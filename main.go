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
	olog "log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"time"

	"github.com/xmidt-org/codex-db/retry"

	"github.com/xmidt-org/codex-db/postgresql"

	"github.com/xmidt-org/voynicrypto"

	"github.com/xmidt-org/bascule"
	"github.com/xmidt-org/bascule/key"

	"github.com/xmidt-org/webpa-common/basculechecks"
	"github.com/xmidt-org/webpa-common/secure"

	"github.com/go-kit/kit/log"
	"github.com/goph/emperror"
	"github.com/gorilla/mux"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/xmidt-org/codex-db/healthlogger"
	"github.com/xmidt-org/webpa-common/concurrent"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/server"

	"github.com/InVisionApp/go-health"
	"github.com/InVisionApp/go-health/handlers"
)

const (
	applicationName, apiBase = "gungnir", "/api/v1"
	DEFAULT_KEY_ID           = "current"
	applicationVersion       = "0.9.0"
)

type Config struct {
	Db               postgresql.Config
	GetLimit         int
	GetRetries       int
	RetryInterval    time.Duration
	Health           HealthConfig
	AuthHeader       []string
	JwtValidator     JWTValidator
	CapabilityConfig basculechecks.CapabilityConfig
}

type HealthConfig struct {
	Port     string
	Endpoint string
}

type JWTValidator struct {
	// JWTKeys is used to create the key.Resolver for JWT verification keys
	Keys key.ResolverFactory

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

func GetLogger(ctx context.Context) bascule.Logger {
	return log.With(logging.GetLogger(ctx), "ts", log.DefaultTimestampUTC, "caller", log.DefaultCaller)
}

func gungnir(arguments []string) {
	start := time.Now()

	var (
		f, v                                = pflag.NewFlagSet(applicationName, pflag.ContinueOnError), viper.New()
		logger, metricsRegistry, codex, err = server.Initialize(applicationName, arguments, f, v, secure.Metrics, postgresql.Metrics, dbretry.Metrics)
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
	dbConfig := config.Db

	//vaultClient, err := xvault.Initialize(v)
	//if err != nil {
	//	fmt.Fprintf(os.Stderr, "Vauilt Initialize error: %v\n", err)
	//	return 3
	//}
	//usr, pwd := vaultClient.GetUsernamePassword("dev", "couchbase")
	//if usr == "" || pwd == "" {
	//	fmt.Fprintf(os.Stderr, "Failed to get Login credientals to couchbase")
	//	return 3
	//}
	//database.Username = usr
	//database.Password = pwd

	database, err := postgresql.CreateDbConnection(dbConfig, metricsRegistry, serverHealth)
	exitIfError(logger, emperror.Wrap(err, "failed to initialize database connection"))
	retryService := dbretry.CreateRetryRGService(
		database,
		dbretry.WithRetries(config.GetRetries),
		dbretry.WithInterval(config.RetryInterval),
		dbretry.WithMeasures(metricsRegistry),
	)

	cipherOptions, err := voynicrypto.FromViper(v)
	exitIfError(logger, emperror.Wrap(err, "failed to initialize cipher config"))
	decrypters := voynicrypto.PopulateCiphers(cipherOptions, logger)

	gungnirHandler, err := authChain(config.AuthHeader, config.JwtValidator, config.CapabilityConfig, logger, metricsRegistry)
	exitIfError(logger, emperror.Wrap(err, "failed to setup auth chain"))

	router := mux.NewRouter()
	measures := NewMeasures(metricsRegistry)
	// MARK: Actual server logic
	app := &App{
		eventGetter: retryService,
		logger:      logger,
		getLimit:    config.GetLimit,
		decrypters:  decrypters,
		measures:    measures,
	}

	logging.GetLogger(context.Background())

	router.Handle(apiBase+"/device/{deviceID}/events", gungnirHandler.ThenFunc(app.handleGetEvents))
	router.Handle(apiBase+"/device/{deviceID}/status", gungnirHandler.ThenFunc(app.handleGetStatus))
	// router.Handle(apiBase+"/device/{deviceID}/last", gungnirHandler.ThenFunc(app.handleGetLastState))

	if config.Health.Endpoint != "" && config.Health.Port != "" {
		err = serverHealth.Start()
		if err != nil {
			logging.Error(logger).Log(logging.MessageKey(), "failed to start health", logging.ErrorKey(), err)
		}
		//router.Handler(config.Health.Address, handlers)
		http.HandleFunc(config.Health.Endpoint, handlers.NewJSONHandlerFunc(serverHealth, nil))
		go func() {
			olog.Fatal(http.ListenAndServe(config.Health.Port, nil))
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
		fmt.Println(applicationVersion)
		return nil, true
	}
	return nil, false
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

func main() {
	gungnir(os.Args)
}
