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
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	olog "log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"time"

	"github.com/Comcast/codex/blacklist"
	"github.com/Comcast/codex/cipher"

	"github.com/Comcast/comcast-bascule/bascule"
	"github.com/Comcast/comcast-bascule/bascule/basculehttp"
	"github.com/Comcast/comcast-bascule/bascule/key"
	"github.com/SermoDigital/jose/jwt"

	"github.com/Comcast/webpa-common/secure"

	"github.com/go-kit/kit/log"
	"github.com/goph/emperror"
	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/Comcast/codex/db"
	"github.com/Comcast/codex/healthlogger"
	"github.com/Comcast/webpa-common/bookkeeping"
	"github.com/Comcast/webpa-common/concurrent"
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/server"

	"github.com/InVisionApp/go-health"
	"github.com/InVisionApp/go-health/handlers"
)

const (
	applicationName, apiBase = "gungnir", "/api/v1"
	DEFAULT_KEY_ID           = "current"
	applicationVersion       = "0.4.1"
)

type Config struct {
	Db                db.Config
	GetLimit          int
	GetRetries        int
	RetryInterval     time.Duration
	Health            HealthConfig
	AuthHeader        []string
	JwtValidator      JWTValidator
	BlacklistInterval time.Duration
}

type HealthConfig struct {
	Port     string
	Endpoint string
}

type JWTValidator struct {
	// JWTKeys is used to create the key.Resolver for JWT verification keys
	Keys key.ResolverFactory

	// Custom is an optional configuration section that defines
	// custom rules for validation over and above the standard RFC rules.
	Custom secure.JWTValidatorFactory
}

func SetLogger(logger log.Logger) func(delegate http.Handler) http.Handler {
	return func(delegate http.Handler) http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				ctx := r.WithContext(logging.WithLogger(r.Context(),
					log.With(logger, "request headers", r.Header, "request URL", r.URL.EscapedPath(), "method", r.Method)))
				delegate.ServeHTTP(w, ctx)
			})
	}
}

func GetLogger(ctx context.Context) bascule.Logger {
	return log.With(logging.GetLogger(ctx), "ts", log.DefaultTimestampUTC, "caller", log.DefaultCaller)
}

func gungnir(arguments []string) int {
	start := time.Now()

	var (
		f, v                                = pflag.NewFlagSet(applicationName, pflag.ContinueOnError), viper.New()
		logger, metricsRegistry, codex, err = server.Initialize(applicationName, arguments, f, v, secure.Metrics, db.Metrics)
	)

	printVer := f.BoolP("version", "v", false, "displays the version number")
	if err := f.Parse(arguments); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse arguments: %s\n", err.Error())
		return 1
	}

	if *printVer {
		fmt.Println(applicationVersion)
		return 0
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to initialize viper: %s\n", err.Error())
		return 1
	}
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

	database, err := db.CreateDbConnection(dbConfig, metricsRegistry, serverHealth)
	if err != nil {
		logging.Error(logger, emperror.Context(err)...).Log(logging.MessageKey(), "Failed to initialize database connection",
			logging.ErrorKey(), err.Error())
		fmt.Fprintf(os.Stderr, "Database Initialize Failed: %#v\n", err)
		return 2
	}
	retryService := db.CreateRetryRGService(database, config.GetRetries, config.RetryInterval, metricsRegistry)

	cipherOptions, err := cipher.FromViper(cipher.Sub(v))
	cipherOptions.Logger = logger
	if err != nil {
		logging.Error(logger, emperror.Context(err)...).Log(logging.MessageKey(), "Failed to initialize cipher options",
			logging.ErrorKey(), err.Error())
		fmt.Fprintf(os.Stderr, "Cipher Options Initialize Failed: %#v\n", err)
		return 2
	}

	decrypter, err := cipherOptions.LoadDecrypt()
	if err != nil {
		logging.Error(logger, emperror.Context(err)...).Log(logging.MessageKey(), "Failed to load cipher decrypter",
			logging.ErrorKey(), err.Error())
		fmt.Fprintf(os.Stderr, "Cipher decrypter Initialize Failed: %#v\n", err)
		return 2
	}

	basicAllowed := make(map[string]string)
	for _, a := range config.AuthHeader {
		decoded, err := base64.StdEncoding.DecodeString(a)
		if err != nil {
			logging.Info(logger).Log(logging.MessageKey(), "failed to decode auth header", "auth header", a, logging.ErrorKey(), err.Error())
		}

		i := bytes.IndexByte(decoded, ':')
		logging.Debug(logger).Log(logging.MessageKey(), "decoded string", "string", decoded, "i", i)
		if i > 0 {
			basicAllowed[string(decoded[:i])] = string(decoded[i+1:])
		}
	}
	logging.Debug(logger).Log(logging.MessageKey(), "Created list of allowed basic auths", "allowed list", basicAllowed, "config", config.AuthHeader)

	options := []basculehttp.COption{basculehttp.WithCLogger(GetLogger)}
	if len(basicAllowed) > 0 {
		options = append(options, basculehttp.WithTokenFactory("Basic", basculehttp.BasicTokenFactory(basicAllowed)))
	}
	if config.JwtValidator.Keys.URI != "" {
		resolver, err := config.JwtValidator.Keys.NewResolver()
		if err != nil {
			logging.Error(logger, emperror.Context(err)...).Log(logging.MessageKey(), "Failed to create resolver",
				logging.ErrorKey(), err.Error())
			fmt.Fprintf(os.Stderr, "New resolver failed: %#v\n", err)
			return 2
		}

		options = append(options, basculehttp.WithTokenFactory("Bearer", basculehttp.BearerTokenFactory{
			DefaultKeyId:  DEFAULT_KEY_ID,
			Resolver:      resolver,
			Parser:        bascule.DefaultJWSParser,
			JWTValidators: []*jwt.Validator{config.JwtValidator.Custom.New()},
		}))
	}

	authConstructor := basculehttp.NewConstructor(options...)

	authEnforcer := basculehttp.NewEnforcer(
		basculehttp.WithELogger(GetLogger),
		basculehttp.WithRules("Basic", []bascule.Validator{
			bascule.CreateAllowAllCheck(),
		}),
		basculehttp.WithRules("Bearer", []bascule.Validator{
			bascule.CreateNonEmptyPrincipalCheck(),
			bascule.CreateNonEmptyTypeCheck(),
			bascule.CreateValidTypeCheck([]string{"jwt"}),
			bascule.CreateListAttributeCheck("capabilities", CreateValidCapabilityCheck("x1", "codex", "api", "all")),
		}),
	)

	// TODO: fix bookkeeping, add a decorator to add the bookkeeping requests and logger
	bookkeeper := bookkeeping.New(bookkeeping.WithResponses(bookkeeping.Code))

	stopUpdateBlackList := make(chan struct{}, 1)
	blacklistConfig := blacklist.RefresherConfig{
		Logger:         logger,
		UpdateInterval: config.BlacklistInterval,
	}

	gungnirHandler := alice.New(SetLogger(logger), authConstructor, authEnforcer, bookkeeper)
	router := mux.NewRouter()
	measures := NewMeasures(metricsRegistry)
	// MARK: Actual server logic
	app := &App{
		eventGetter: retryService,
		logger:      logger,
		getLimit:    config.GetLimit,
		decrypter:   decrypter,
		measures:    measures,
		blacklist:   blacklist.NewListRefresher(blacklistConfig, database, stopUpdateBlackList),
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
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to start device manager: %s\n", err)
		return 1
	}

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
	return 0
}

func main() {
	os.Exit(gungnir(os.Args))
}
