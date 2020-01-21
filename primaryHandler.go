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
	"encoding/base64"
	"errors"
	"github.com/ugorji/go/codec"
	"github.com/xmidt-org/gungnir/model"
	"github.com/xmidt-org/wrp-go/wrp"
	"net/http"
	"strings"
	"time"

	"github.com/justinas/alice"
	"github.com/xmidt-org/bascule"
	"github.com/xmidt-org/bascule/basculehttp"
	"github.com/xmidt-org/voynicrypto"

	"github.com/go-kit/kit/log"
	kithttp "github.com/go-kit/kit/transport/http"
	"github.com/goph/emperror"
	"github.com/gorilla/mux"
	db "github.com/xmidt-org/codex-db"
	"github.com/xmidt-org/webpa-common/basculechecks"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/xmetrics"
)

//go:generate swagger generate spec -m -o swagger.spec

type App struct {
	eventGetter    db.RecordGetter
	logger         log.Logger
	getEventLimit  int
	getStatusLimit int
	longPollSleep  time.Duration
	decrypters     voynicrypto.Ciphers

	measures *Measures
}

// swagger:parameters getEvents getStatus
type DeviceIdParam struct {
	// device id passed by caller
	//
	// in: path
	// required: true
	DeviceID string `json:"deviceID"`
}

// EventResponse is what is returned on a successful response
//
// swagger:response EventResponse
type EventResponse struct {
	// in:body
	Body []model.Event
}

// ErrResponse is the information passed to the client on an error
//
// swagger:response ErrResponse
type ErrResponse struct {
	// The http code of the response
	//
	// required: true
	Code int `json:"code"`
}

func (app *App) getDeviceInfoAfterHash(deviceID string, requestHash string) ([]model.Event, string, error) {

	records, hErr := app.eventGetter.GetRecords(deviceID, app.getEventLimit, requestHash)
	// if both have errors or are empty, return an error
	if hErr != nil {
		return []model.Event{}, "", serverErr{emperror.WrapWith(hErr, "Failed to get events", "device id", deviceID, "hash", requestHash),
			http.StatusInternalServerError}
	}

	// TODO: improve long poll logic
	for len(records) == 0 {
		time.Sleep(app.longPollSleep)
		records, hErr = app.eventGetter.GetRecords(deviceID, app.getEventLimit, requestHash)
		// if both have errors or are empty, return an error
		if hErr != nil {
			return []model.Event{}, "", serverErr{emperror.WrapWith(hErr, "Failed to get events", "device id", deviceID, "hash", requestHash),
				http.StatusInternalServerError}
		}
	}

	hash, err := app.eventGetter.GetStateHash(records)
	if err != nil {
		logging.Error(app.logger, emperror.Context(err)...).Log(logging.MessageKey(), "Failed to get latest hash from records", logging.ErrorKey(), err.Error())
	}
	events := app.parseRecords(records)

	if len(events) == 0 {
		// TODO: Timeout Error
		return events, "", serverErr{emperror.With(errors.New("No events found for device id after hash"), "device id", deviceID, "hash", requestHash),
			http.StatusNotFound}
	}
	app.measures.EventsReturnedCount.Add(float64(len(events)))

	return events, hash, nil
}

func (app *App) getDeviceInfo(deviceID string) ([]model.Event, string, error) {

	records, hErr := app.eventGetter.GetRecords(deviceID, app.getEventLimit, "")
	// if both have errors or are empty, return an error
	if hErr != nil && len(records) == 0 {
		return []model.Event{}, "", serverErr{emperror.WrapWith(hErr, "Failed to get events", "device id", deviceID),
			http.StatusInternalServerError}
	}

	hash, err := app.eventGetter.GetStateHash(records)
	if err != nil {
		logging.Error(app.logger, emperror.Context(err)...).Log(logging.MessageKey(), "Failed to get latest hash from records", logging.ErrorKey(), err.Error())
	}
	events := app.parseRecords(records)

	if len(events) == 0 {
		return events, "", serverErr{emperror.With(errors.New("No events found for device id"), "device id", deviceID),
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
			continue
		}

		event := model.Event{
			BirthDate: record.BirthDate,
		}
		decrypter, ok := app.decrypters.Get(voynicrypto.ParseAlgorithmType(record.Alg), record.KID)
		if !ok {
			app.measures.GetDecryptFailure.Add(1.0)
			logging.Error(app.logger).Log(logging.MessageKey(), "Failed to get decrypter", logging.ErrorKey())
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
 *    404: ErrResponse
 *    500: ErrResponse
 *
 */
func (app *App) handleGetEvents(writer http.ResponseWriter, request *http.Request) {
	var (
		d    []model.Event
		hash string
		err  error
	)
	vars := mux.Vars(request)
	id := strings.ToLower(vars["deviceID"])
	if id == "" {
		writer.WriteHeader(http.StatusNotFound)
		return
	}

	if requestHash := request.FormValue("after"); requestHash != "" {
		if d, hash, err = app.getDeviceInfoAfterHash(id, requestHash); err != nil {
			logging.Error(app.logger, emperror.Context(err)...).Log(logging.MessageKey(),
				"Failed to get status info", logging.ErrorKey(), err.Error())
			writer.Header().Add("X-Codex-Error", err.Error())

			if val, ok := err.(kithttp.StatusCoder); ok {
				writer.WriteHeader(val.StatusCode())
				return
			}
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
	} else if d, hash, err = app.getDeviceInfo(id); err != nil {
		logging.Error(app.logger, emperror.Context(err)...).Log(logging.MessageKey(),
			"Failed to get status info", logging.ErrorKey(), err.Error())
		writer.Header().Add("X-Codex-Error", err.Error())

		if val, ok := err.(kithttp.StatusCoder); ok {
			writer.WriteHeader(val.StatusCode())
			return
		}
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	var data []byte
	// TODO: revert to json spec, aka encode integers > 2^53 as a json string
	err = codec.NewEncoderBytes(&data, &codec.JsonHandle{
		BasicHandle: codec.BasicHandle{
			TypeInfos: codec.NewTypeInfos([]string{"wrp"}),
		},
	}).Encode(d)
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

func authChain(basicAuth []string, jwtVal JWTValidator, capabilityCheck CapabilityConfig, logger log.Logger, registry xmetrics.Registry) (alice.Chain, error) {
	var m *basculechecks.JWTValidationMeasures

	if registry != nil {
		m = basculechecks.NewJWTValidationMeasures(registry)
	}
	listener := basculechecks.NewMetricListener(m)

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
		basculehttp.WithParseURLFunc(basculehttp.CreateRemovePrefixURLFunc(apiBase, basculehttp.DefaultParseURLFunc)),
	}
	if len(basicAllowed) > 0 {
		options = append(options, basculehttp.WithTokenFactory("Basic", basculehttp.BasicTokenFactory(basicAllowed)))
	}

	if jwtVal.Keys.URI != "" {
		resolver, err := jwtVal.Keys.NewResolver()
		if err != nil {
			return alice.Chain{}, emperror.With(err, "failed to create resolver")
		}

		options = append(options, basculehttp.WithTokenFactory("Bearer", basculehttp.BearerTokenFactory{
			DefaultKeyId: DEFAULT_KEY_ID,
			Resolver:     resolver,
			Parser:       bascule.DefaultJWTParser,
			Leeway:       jwtVal.Leeway,
		}))
	}

	authConstructor := basculehttp.NewConstructor(options...)

	bearerRules := bascule.Validators{
		bascule.CreateNonEmptyPrincipalCheck(),
		bascule.CreateNonEmptyTypeCheck(),
		bascule.CreateValidTypeCheck([]string{"jwt"}),
	}

	// only add capability check if the configuration is set
	if capabilityCheck.Prefix != "" {
		check, err := basculechecks.CreateValidCapabilityCheck(capabilityCheck.Prefix, capabilityCheck.AcceptAllMethod)
		if err != nil {
			return alice.Chain{}, emperror.With(err, "failed to create capability check")
		}
		bearerRules = append(bearerRules, bascule.CreateListAttributeCheck("capabilities", check))
	}

	authEnforcer := basculehttp.NewEnforcer(
		basculehttp.WithELogger(GetLogger),
		basculehttp.WithRules("Basic", bascule.Validators{
			bascule.CreateAllowAllCheck(),
		}),
		basculehttp.WithRules("Bearer", bearerRules),
		basculehttp.WithEErrorResponseFunc(listener.OnErrorResponse),
	)

	return alice.New(SetLogger(logger), authConstructor, authEnforcer, basculehttp.NewListenerDecorator(listener)), nil
}
