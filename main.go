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
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"time"

	"github.com/Comcast/webpa-common/secure"

	"github.com/go-kit/kit/log"
	"github.com/goph/emperror"
	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/Comcast/codex/db"
	"github.com/Comcast/webpa-common/bookkeeping"
	"github.com/Comcast/webpa-common/concurrent"
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/secure/handler"
	"github.com/Comcast/webpa-common/server"
)

const (
	applicationName, apiBase = "gungnir", "/api/v1"
	DEFAULT_KEY_ID           = "current"
	applicationVersion       = "0.2.1"
)

type Config struct {
	Db            db.Config
	GetRetries    int
	RetryInterval time.Duration
}

func SetLogger(logger log.Logger) func(delegate http.Handler) http.Handler {
	return func(delegate http.Handler) http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				r.WithContext(logging.WithLogger(r.Context(), logger))
				delegate.ServeHTTP(w, r.WithContext(logging.WithLogger(r.Context(), logger)))
			})
	}
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

	// add GetValidator function (originally from caduceus)
	//validator, err := server.GetValidator(v, DEFAULT_KEY_ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Validator error: %v\n", err)
		return 1
	}

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

	database, err := db.CreateDbConnection(dbConfig, metricsRegistry)
	if err != nil {
		logging.Error(logger, emperror.Context(err)...).Log(logging.MessageKey(), "Failed to initialize database connection",
			logging.ErrorKey(), err.Error())
		fmt.Fprintf(os.Stderr, "Database Initialize Failed: %#v\n", err)
		return 2
	}
	retryService := db.CreateRetryRGService(database, config.GetRetries, config.RetryInterval, metricsRegistry)

	authHandler := handler.AuthorizationHandler{
		HeaderName:          "Authorization",
		ForbiddenStatusCode: 403,
		//Validator:           validator,
		Logger: logger,
	}
	// TODO: fix bookkeeping, add a decorator to add the bookkeeping requests and logger
	bookkeeper := bookkeeping.New(bookkeeping.WithResponses(bookkeeping.Code))

	gungnirHandler := alice.New(SetLogger(logger), authHandler.Decorate, bookkeeper)
	router := mux.NewRouter()
	measures := NewMeasures(metricsRegistry)
	// MARK: Actual server logic
	app := &App{
		eventGetter: retryService,
		logger:      logger,
		measures:    measures,
	}
	logging.GetLogger(context.Background())

	router.Handle(apiBase+"/device/{deviceID}/events", gungnirHandler.ThenFunc(app.handleGetEvents))
	router.Handle(apiBase+"/device/{deviceID}/status", gungnirHandler.ThenFunc(app.handleGetStatus))
	// router.Handle(apiBase+"/device/{deviceID}/last", gungnirHandler.ThenFunc(app.handleGetLastState))

	serverHealth := codex.Health.NewHealth(logger)

	// MARK: Starting the server
	_, runnable, done := codex.Prepare(logger, serverHealth, metricsRegistry, router)

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
