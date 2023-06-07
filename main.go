/**
 * Copyright 2019 Comcast Cable Communications Management, LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package main

import (
	"context"
	"fmt"
	"io"
	olog "log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/xmidt-org/clortho"
	dbretry "github.com/xmidt-org/codex-db/retry"

	"github.com/xmidt-org/codex-db/cassandra"

	"github.com/xmidt-org/voynicrypto"

	"github.com/xmidt-org/bascule"

	"github.com/go-kit/log"
	"github.com/goph/emperror"
	"github.com/gorilla/mux"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/xmidt-org/codex-db/healthlogger"
	"github.com/xmidt-org/webpa-common/v2/basculechecks"  //nolint: staticcheck
	"github.com/xmidt-org/webpa-common/v2/basculemetrics" //nolint: staticcheck
	"github.com/xmidt-org/webpa-common/v2/concurrent"     //nolint: staticcheck
	"github.com/xmidt-org/webpa-common/v2/logging"        //nolint: staticcheck
	"github.com/xmidt-org/webpa-common/v2/server"         //nolint: staticcheck

	"github.com/InVisionApp/go-health/v2"
	"github.com/InVisionApp/go-health/v2/handlers"
)

const (
	applicationName, apiBase = "gungnir", "/api/v1"
	DEFAULT_KEY_ID           = "current"
)

var (
	GitCommit = "undefined"
	Version   = "undefined"
	BuildTime = "undefined"
)

type Config struct {
	Db                          cassandra.Config
	GetEventsLimit              int
	GetStatusLimit              int
	Health                      HealthConfig
	AuthHeader                  []string
	JwtValidator                JWTValidator
	CapabilityCheck             CapabilityConfig
	LongPollSleep               time.Duration
	LongPollTimeout             time.Duration
	BasicAuthPartnerIDHeaderKey string
}

type HealthConfig struct {
	Port     string
	Endpoint string
}

type CapabilityConfig struct {
	Type            string
	Prefix          string
	AcceptAllMethod string
	EndpointBuckets []string
}

type JWTValidator struct {
	// Config is used to create the clortho Resolver & Refresher for JWT verification keys
	Config clortho.Config

	// Leeway is used to set the amount of time buffer should be given to JWT
	// time values, such as nbf
	Leeway bascule.Leeway
}

func SetLogger(logger log.Logger) func(delegate http.Handler) http.Handler {
	return func(delegate http.Handler) http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				ctx := r.WithContext(logging.WithLogger(r.Context(),
					log.With(logger, "requestHeaders", r.Header, "requestURL", r.URL.EscapedPath(), "method", r.Method)))
				delegate.ServeHTTP(w, ctx)
			})
	}
}

func GetLogger(ctx context.Context) log.Logger {
	return log.With(logging.GetLogger(ctx), "ts", log.DefaultTimestampUTC, "caller", log.DefaultCaller)
}

//nolint:funlen // this will be fixed with uber fx
func gungnir(arguments []string) {
	start := time.Now()

	var (
		f, v                                = pflag.NewFlagSet(applicationName, pflag.ContinueOnError), viper.New()
		logger, metricsRegistry, codex, err = server.Initialize(applicationName, arguments, f, v, cassandra.Metrics, dbretry.Metrics, basculechecks.Metrics, basculemetrics.Metrics)
	)

	if parseErr, done := printVersion(f, arguments); done {
		// if we're done, we're exiting no matter what
		exitIfError(logger, emperror.Wrap(parseErr, "failed to parse arguments"))
		os.Exit(0)
	}

	exitIfError(logger, emperror.Wrap(err, "unable to initialize viper"))
	logging.Info(logger).Log(logging.MessageKey(), "Successfully loaded config file", "configurationFile", v.ConfigFileUsed())

	serverHealth := health.New()
	serverHealth.Logger = healthlogger.NewHealthLogger(logger)

	config := new(Config)
	v.Unmarshal(config)
	validateConfig(config)

	database, err := cassandra.CreateDbConnection(config.Db, metricsRegistry, serverHealth)
	exitIfError(logger, emperror.Wrap(err, "failed to initialize database connection"))

	cipherOptions, err := voynicrypto.FromViper(v)
	exitIfError(logger, emperror.Wrap(err, "failed to initialize cipher config"))
	decrypters := voynicrypto.PopulateCiphers(cipherOptions, logger)

	gungnirHandler, err := authChain(v, config.AuthHeader, config.CapabilityCheck, logger, metricsRegistry)
	exitIfError(logger, emperror.Wrap(err, "failed to setup auth chain"))

	router := mux.NewRouter()
	measures := NewMeasures(metricsRegistry)
	// MARK: Actual server logic
	app := &App{
		eventGetter:                 database,
		logger:                      logger,
		getEventLimit:               config.GetEventsLimit,
		getStatusLimit:              config.GetStatusLimit,
		longPollSleep:               config.LongPollSleep,
		longPollTimeout:             config.LongPollTimeout,
		decrypters:                  decrypters,
		measures:                    measures,
		basicAuthPartnerIDHeaderKey: config.BasicAuthPartnerIDHeaderKey,
	}

	router.Handle(apiBase+"/device/{deviceID}/events", gungnirHandler.ThenFunc(app.handleGetEvents))
	router.Handle(apiBase+"/device/{deviceID}/status", gungnirHandler.ThenFunc(app.handleGetStatus))

	if config.Health.Endpoint != "" && config.Health.Port != "" {
		err = serverHealth.Start()
		if err != nil {
			logging.Error(logger).Log(logging.MessageKey(), "failed to start health", logging.ErrorKey(), err)
		}
		http.HandleFunc(config.Health.Endpoint, handlers.NewJSONHandlerFunc(serverHealth, nil))
		server := &http.Server{
			Addr:              config.Health.Port,
			ReadHeaderTimeout: 3 * time.Second,
		}
		go func() {

			olog.Fatal(server.ListenAndServe())
		}()
	}

	// MARK: Starting the server
	_, runnable, done := codex.Prepare(logger, nil, metricsRegistry, router)

	waitGroup, shutdown, err := concurrent.Execute(runnable)
	exitIfError(logger, emperror.Wrap(err, "unable to start device manager"))

	logging.Info(logger).Log(logging.MessageKey(), fmt.Sprintf("%s is up and running!", applicationName), "elapsedTime", time.Since(start))
	signals := make(chan os.Signal, 10)
	signal.Notify(signals)
	for exit := false; !exit; {
		select {
		case s := <-signals:
			if s != os.Kill && s != os.Interrupt {
				logging.Info(logger).Log(logging.MessageKey(), "ignoring signal", "signal", s)
			} else {
				logging.Error(logger).Log(logging.MessageKey(), "exiting due to signal", "signal", s)
				exit = true
			}
		case <-done:
			logging.Error(logger).Log(logging.MessageKey(), "one or more servers exited")
			exit = true
		}
	}

	err = database.Close()
	if err != nil {
		logging.Error(logger, emperror.Context(err)...).Log(logging.MessageKey(), "closing database threads failed",
			logging.ErrorKey(), err.Error())
	}
	close(shutdown)
	waitGroup.Wait()
	logging.Info(logger).Log(logging.MessageKey(), "Gungnir has shut down")
}

func printVersion(f *pflag.FlagSet, arguments []string) (error, bool) {
	printVer := f.BoolP("version", "v", false, "displays the version number")
	if err := f.Parse(arguments); err != nil {
		return err, true
	}

	if *printVer {
		printVersionInfo(os.Stdout)
		return nil, true
	}
	return nil, false
}

func printVersionInfo(writer io.Writer) {
	fmt.Fprintf(writer, "%s:\n", applicationName)
	fmt.Fprintf(writer, "  version: \t%s\n", Version)
	fmt.Fprintf(writer, "  go version: \t%s\n", runtime.Version())
	fmt.Fprintf(writer, "  built time: \t%s\n", BuildTime)
	fmt.Fprintf(writer, "  git commit: \t%s\n", GitCommit)
	fmt.Fprintf(writer, "  os/arch: \t%s/%s\n", runtime.GOOS, runtime.GOARCH)
}

func exitIfError(logger log.Logger, err error) {
	if err != nil {
		if logger != nil {
			logging.Error(logger, emperror.Context(err)...).Log(logging.ErrorKey(), err.Error())
		}
		fmt.Fprintf(os.Stderr, "Error: %#v\n", err.Error())
		os.Exit(1)
	}
}

const (
	defaultGetEventsLimit  = 50
	defaultGetStatusLimit  = 10
	defaultLongPollSleep   = time.Second
	defaultLongPollTimeout = time.Minute
)

func validateConfig(config *Config) {
	var emptyDuration time.Duration
	if config.GetEventsLimit < 1 {
		config.GetEventsLimit = defaultGetEventsLimit
	}
	if config.GetStatusLimit < 1 {
		config.GetStatusLimit = defaultGetStatusLimit
	}
	if config.LongPollSleep == emptyDuration {
		config.LongPollSleep = defaultLongPollSleep
	}
	if config.LongPollTimeout == emptyDuration {
		config.LongPollTimeout = defaultLongPollTimeout
	}
}

func main() {
	gungnir(os.Args)
}
