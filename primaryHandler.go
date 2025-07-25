// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cast"
	"github.com/ugorji/go/codec"
	"github.com/xmidt-org/clortho"
	"github.com/xmidt-org/clortho/clorthometrics"
	"github.com/xmidt-org/clortho/clorthozap"
	"github.com/xmidt-org/gungnir/model"
	"github.com/xmidt-org/sallust"
	"github.com/xmidt-org/touchstone"
	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap"

	"github.com/justinas/alice"
	"github.com/xmidt-org/bascule"
	"github.com/xmidt-org/bascule/basculehttp"
	"github.com/xmidt-org/voynicrypto"

	kithttp "github.com/go-kit/kit/transport/http"
	"github.com/go-kit/log"
	"github.com/goph/emperror"
	"github.com/gorilla/mux"
	newchecks "github.com/xmidt-org/bascule/basculechecks"
	db "github.com/xmidt-org/codex-db"
	"github.com/xmidt-org/webpa-common/v2/basculechecks"  //nolint: staticcheck
	"github.com/xmidt-org/webpa-common/v2/basculemetrics" //nolint: staticcheck
	"github.com/xmidt-org/webpa-common/v2/logging"        //nolint: staticcheck
	"github.com/xmidt-org/webpa-common/v2/xmetrics"       //nolint: staticcheck
)

type App struct {
	eventGetter     db.RecordGetter
	logger          log.Logger
	getEventLimit   int
	getStatusLimit  int
	longPollSleep   time.Duration
	longPollTimeout time.Duration
	decrypters      voynicrypto.Ciphers

	measures                    *Measures
	basicAuthPartnerIDHeaderKey string
}

var (
	errGettingPartnerIDs         = errors.New("unable to retrieve PartnerIDs")
	errAuthIsNotOfTypeBasicOrJWT = errors.New("auth is not of type Basic of JWT")
	basicType                    = "basic"
)

func (app *App) getDeviceInfoAfterHash(deviceID string, requestHash string, ctx context.Context) ([]model.Event, string, error) {
	var (
		hash string
		err  error
	)

	records, hErr := app.eventGetter.GetRecords(deviceID, app.getEventLimit, requestHash)
	// if both have errors or are empty, return an error
	if hErr != nil {
		return []model.Event{}, "", serverErr{emperror.WrapWith(hErr, "Failed to get events", "device id", deviceID, "hash", requestHash),
			http.StatusInternalServerError}
	}

	hash, err = app.eventGetter.GetStateHash(records)
	if err != nil {
		logging.Error(app.logger, emperror.Context(err)...).Log(logging.MessageKey(), "Failed to get latest hash from records", logging.ErrorKey(), err.Error())
	}
	events := app.parseRecords(records)

	after := time.After(app.longPollTimeout)
	// TODO: improve long poll logic
	for len(events) == 0 {
		select {
		case <-ctx.Done():
			// request was canceled.
			// 499 Client Closed Request (from nginx)
			return []model.Event{}, "", serverErr{emperror.With(ctx.Err(), "device id", deviceID, "hash", requestHash),
				499}
		case <-after:
			return []model.Event{}, "", serverErr{emperror.With(fmt.Errorf("long poll timeout expired after %s", app.longPollTimeout), "device id", deviceID, "hash", requestHash),
				http.StatusNoContent}

		default:
			time.Sleep(app.longPollSleep)
			records, hErr = app.eventGetter.GetRecords(deviceID, app.getEventLimit, requestHash)
			if len(records) == 0 {
				continue
			}
			// if both have errors or are empty, return an error
			if hErr != nil {
				return []model.Event{}, "", serverErr{emperror.WrapWith(hErr, "Failed to get events", "device id", deviceID, "hash", requestHash),
					http.StatusInternalServerError}
			}
			hash, err = app.eventGetter.GetStateHash(records)
			if err != nil {
				logging.Error(app.logger, emperror.Context(err)...).Log(logging.MessageKey(), "Failed to get latest hash from records", logging.ErrorKey(), err.Error())
			}
			events = app.parseRecords(records)
		}
	}

	app.measures.EventsReturnedCount.Add(float64(len(events)))

	return events, hash, nil
}

func (app *App) getDeviceInfo(deviceID string) ([]model.Event, string, error) {

	records, hErr := app.eventGetter.GetRecords(deviceID, app.getEventLimit, "")
	// if both have errors or are empty, return an error
	if hErr != nil {
		return []model.Event{}, "", serverErr{emperror.WrapWith(hErr, "Failed to get events", "device id", deviceID),
			http.StatusInternalServerError}
	}
	if len(records) == 0 {
		return []model.Event{}, "", serverErr{emperror.WrapWith(fmt.Errorf("no events found for %s", deviceID), "Failed to get events", "deviceID", deviceID),
			http.StatusNotFound}
	}

	hash, err := app.eventGetter.GetStateHash(records)
	if err != nil {
		logging.Warn(app.logger, emperror.Context(err)...).Log(logging.MessageKey(), "Failed to get latest hash from records", logging.ErrorKey(), err.Error(), "hash", hash)
	}
	events := app.parseRecords(records)

	if len(events) == 0 {
		return events, "", serverErr{emperror.With(errors.New("no events found for device id"), "device id", deviceID),
			http.StatusNotFound}
	}
	app.measures.EventsReturnedCount.Add(float64(len(events)))

	return events, hash, nil
}

func (app *App) parseRecords(records []db.Record) []model.Event {
	events := []model.Event{}
	// if all is good, unmarshal everything
	for _, record := range records {
		// if the record is expired, don't include it
		if time.Unix(0, record.DeathDate).Before(time.Now()) {
			logging.Debug(app.logger).Log(logging.MessageKey(), "the record is expired", "timesince", time.Since(time.Unix(0, record.DeathDate)))
			continue
		}

		event := model.Event{
			BirthDate: record.BirthDate,
		}
		decrypter, ok := app.decrypters.Get(voynicrypto.ParseAlgorithmType(record.Alg), record.KID)
		if !ok {
			app.measures.GetDecryptFailure.Add(1.0)
			logging.Error(app.logger).Log(logging.MessageKey(), "Failed to get decrypter")
			event.Type = wrp.UnknownMessageType
			events = append(events, event)
			continue
		}
		data, err := decrypter.DecryptMessage(record.Data, record.Nonce)
		if err != nil {
			app.measures.DecryptFailure.Add(1.0)
			logging.Error(app.logger).Log(logging.MessageKey(), "Failed to decrypt event", logging.ErrorKey(), err.Error())
			event.Type = wrp.UnknownMessageType
			events = append(events, event)
			continue
		}

		decoder := wrp.NewDecoderBytes(data, wrp.Msgpack)
		err = decoder.Decode(&event)
		if err != nil {
			app.measures.UnmarshalFailure.Add(1.0)
			logging.Error(app.logger, emperror.Context(err)...).Log(logging.MessageKey(), "Failed to decode decrypted event", logging.ErrorKey(), err.Error())
			event.Type = wrp.UnknownMessageType
			events = append(events, event)
			continue
		}

		events = append(events, event)
	}
	return events
}

/*
 * swagger:route GET /device/{deviceID}/events device getEvents
 *
 * Get all of the events related to a specific device id.
 *
 * Parameters: deviceID
 *
 * Produces:
 *    - application/json
 *
 * Schemes: https
 *
 * Security:
 *    bearer_token:
 *
 * Responses:
 *    200: EventResponse
 *	  400: ErrResponse
 *    404: ErrResponse
 *    500: ErrResponse
 *
 */
func (app *App) handleGetEvents(writer http.ResponseWriter, request *http.Request) {
	var (
		d        []model.Event
		filtered []model.Event
		hash     string
		err      error
		coder    kithttp.StatusCoder
	)
	vars := mux.Vars(request)
	id := strings.ToLower(vars["deviceID"])
	if id == "" {
		writer.WriteHeader(http.StatusNotFound)
		return
	}

	requestPartnerIDs, err := extractPartnerIDs(request, app.basicAuthPartnerIDHeaderKey)
	if err != nil || len(requestPartnerIDs) == 0 {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	if requestHash := request.FormValue("after"); requestHash != "" {
		if d, hash, err = app.getDeviceInfoAfterHash(id, requestHash, request.Context()); err != nil {
			logging.Error(app.logger, emperror.Context(err)...).Log(logging.MessageKey(),
				"Failed to get status info", logging.ErrorKey(), err.Error())
			writer.Header().Add("X-Codex-Error", err.Error())

			if errors.As(err, &coder) {
				writer.WriteHeader(coder.StatusCode())
				return
			}
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
	} else if d, hash, err = app.getDeviceInfo(id); err != nil {
		logging.Error(app.logger, emperror.Context(err)...).Log(logging.MessageKey(),
			"Failed to get status info", logging.ErrorKey(), err.Error())
		writer.Header().Add("X-Codex-Error", err.Error())

		if errors.As(err, &coder) {
			writer.WriteHeader(coder.StatusCode())
			return
		}
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	// if partners contains wildcard, do not filter and send all events
	if contains(requestPartnerIDs, "*") {
		filtered = d
	} else {
		for _, event := range d {
			if overlaps(event.PartnerIDs, requestPartnerIDs) {
				filtered = append(filtered, event)
			}
		}
	}

	var data []byte
	// TODO: revert to json spec, aka encode integers > 2^53 as a json string
	err = codec.NewEncoderBytes(&data, &codec.JsonHandle{
		BasicHandle: codec.BasicHandle{ //nolint: staticcheck
			TypeInfos: codec.NewTypeInfos([]string{"wrp"}),
		},
	}).Encode(filtered)
	if err != nil {
		writer.Header().Add("X-Codex-Error", err.Error())
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	if hash != "" {
		writer.Header().Add("X-Codex-Hash", hash)
	}
	writer.WriteHeader(http.StatusOK)
	writer.Write(data)
}

//nolint:funlen // this will be fixed with uber fx
func authChain(basicAuth []string, jwtConfig JWTValidator, tsConfig touchstone.Config, zConfig sallust.Config, capabilityCheck CapabilityConfig, logger log.Logger, registry xmetrics.Registry) (alice.Chain, error) {
	if registry == nil {
		return alice.Chain{}, errors.New("nil registry")
	}

	basculeMeasures := basculemetrics.NewAuthValidationMeasures(registry)
	capabilityCheckMeasures := basculechecks.NewAuthCapabilityCheckMeasures(registry)
	listener := basculemetrics.NewMetricListener(basculeMeasures)

	basicAllowed := make(map[string]string)
	for _, a := range basicAuth {
		decoded, err := base64.StdEncoding.DecodeString(a)
		if err != nil {
			logging.Info(logger).Log(logging.MessageKey(), "failed to decode auth header", "authHeader", a, logging.ErrorKey(), err.Error())
		}

		if i := bytes.IndexByte(decoded, ':'); i > 0 {
			basicAllowed[string(decoded[:i])] = string(decoded[i+1:])
			logging.Debug(logger).Log(logging.MessageKey(), "decoded string", "string", decoded, "i", i)
		}
	}
	logging.Debug(logger).Log(logging.MessageKey(), "Created list of allowed basic auths", "allowed", basicAllowed, "config", basicAuth)

	options := []basculehttp.COption{
		basculehttp.WithCLogger(GetLogger),
		basculehttp.WithCErrorResponseFunc(listener.OnErrorResponse),
		basculehttp.WithParseURLFunc(basculehttp.CreateRemovePrefixURLFunc(apiBase+"/", basculehttp.DefaultParseURLFunc)),
	}
	if len(basicAllowed) > 0 {
		options = append(options, basculehttp.WithTokenFactory("Basic", basculehttp.BasicTokenFactory(basicAllowed)))
	}

	// Instantiate a keyring for refresher and resolver to share
	kr := clortho.NewKeyRing()

	// Instantiate a fetcher for refresher and resolver to share
	f, err := clortho.NewFetcher()
	if err != nil {
		return alice.Chain{}, emperror.With(err, "failed to create clortho fetcher")
	}

	ref, err := clortho.NewRefresher(
		clortho.WithConfig(jwtConfig.Config),
		clortho.WithFetcher(f),
	)
	if err != nil {
		return alice.Chain{}, emperror.With(err, "failed to create clortho refresher")
	}

	resolver, err := clortho.NewResolver(
		clortho.WithConfig(jwtConfig.Config),
		clortho.WithKeyRing(kr),
		clortho.WithFetcher(f),
	)
	if err != nil {
		return alice.Chain{}, emperror.With(err, "failed to create clortho resolver")
	}

	promReg, ok := registry.(prometheus.Registerer)
	if !ok {
		return alice.Chain{}, errors.New("failed to get prometheus registerer")
	}

	zlogger := zap.Must(zConfig.Build())
	tf := touchstone.NewFactory(tsConfig, zlogger, promReg)
	// Instantiate a metric listener for refresher and resolver to share
	cml, err := clorthometrics.NewListener(clorthometrics.WithFactory(tf))
	if err != nil {
		return alice.Chain{}, emperror.With(err, "failed to create clortho metrics listener")
	}

	// Instantiate a logging listener for refresher and resolver to share
	czl, err := clorthozap.NewListener(
		clorthozap.WithLogger(zlogger),
	)
	if err != nil {
		return alice.Chain{}, emperror.With(err, "failed to create clortho zap logger listener")
	}

	resolver.AddListener(cml)
	resolver.AddListener(czl)
	ref.AddListener(cml)
	ref.AddListener(czl)
	ref.AddListener(kr)
	// context.Background() is for the unused `context.Context` argument in refresher.Start
	ref.Start(context.Background())
	// Shutdown refresher's goroutines when SIGTERM
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)
	go func() {
		<-sigs
		// context.Background() is for the unused `context.Context` argument in refresher.Stop
		ref.Stop(context.Background())
	}()

	options = append(options, basculehttp.WithTokenFactory("Bearer", basculehttp.BearerTokenFactory{
		DefaultKeyID: DEFAULT_KEY_ID,
		Resolver:     resolver,
		Parser:       bascule.DefaultJWTParser,
		Leeway:       jwtConfig.Leeway,
	}))

	authConstructor := basculehttp.NewConstructor(options...)

	bearerRules := bascule.Validators{
		newchecks.NonEmptyPrincipal(),
		newchecks.NonEmptyType(),
		newchecks.ValidType([]string{"jwt"}),
	}

	// only add capability check if the configuration is set
	if capabilityCheck.Type == "enforce" || capabilityCheck.Type == "monitor" {
		var endpoints []*regexp.Regexp
		c, err := basculechecks.NewEndpointRegexCheck(capabilityCheck.Prefix, capabilityCheck.AcceptAllMethod)
		if err != nil {
			return alice.Chain{}, emperror.With(err, "failed to create capability check")
		}
		for _, e := range capabilityCheck.EndpointBuckets {
			r, err := regexp.Compile(e)
			if err != nil {
				logging.Error(logger).Log(logging.MessageKey(), "failed to compile regular expression", "regex", e, logging.ErrorKey(), err.Error())
				continue
			}
			endpoints = append(endpoints, r)
		}
		m := basculechecks.MetricValidator{
			C:         basculechecks.CapabilitiesValidator{Checker: c},
			Measures:  capabilityCheckMeasures,
			Endpoints: endpoints,
		}
		bearerRules = append(bearerRules, m.CreateValidator(capabilityCheck.Type == "enforce"))
	}

	authEnforcer := basculehttp.NewEnforcer(
		basculehttp.WithELogger(GetLogger),
		basculehttp.WithRules("Basic", bascule.Validators{
			newchecks.AllowAll(),
		}),
		basculehttp.WithRules("Bearer", bearerRules),
		basculehttp.WithEErrorResponseFunc(listener.OnErrorResponse),
	)

	return alice.New(SetLogger(logger), authConstructor, authEnforcer, basculehttp.NewListenerDecorator(listener)), nil
}

func extractPartnerIDs(r *http.Request, basicAuth string) ([]string, error) {
	auth, present := bascule.FromContext(r.Context())
	if !present || auth.Token == nil {
		return nil, errGettingPartnerIDs
	}
	var partners []string

	switch auth.Token.Type() {
	case basicType:
		authHeader := r.Header[basicAuth]
		for _, value := range authHeader {
			fields := strings.Split(value, ",")
			for i := 0; i < len(fields); i++ {
				fields[i] = strings.TrimSpace(fields[i])
			}
			partners = append(partners, fields...)
		}
		return partners, nil
	case "jwt":
		authToken := auth.Token
		partnersInterface, attrExist := bascule.GetNestedAttribute(authToken.Attributes(), basculechecks.PartnerKeys()...)
		if !attrExist {
			return nil, errGettingPartnerIDs
		}
		vals, err := cast.ToStringSliceE(partnersInterface)
		if err != nil {
			return nil, errGettingPartnerIDs
		}
		partners = vals
		return partners, nil
	}
	return nil, errAuthIsNotOfTypeBasicOrJWT
}

func overlaps(sl1 []string, sl2 []string) bool {
	for _, s1 := range sl1 {
		for _, s2 := range sl2 {
			if s1 == s2 {
				return true
			}
		}
	}
	return false
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}
