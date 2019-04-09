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
	"encoding/json"
	"errors"
	"github.com/Comcast/codex/cipher"
	"net/http"
	"strings"
	"time"

	"github.com/Comcast/webpa-common/logging"
	"github.com/goph/emperror"

	"github.com/Comcast/codex/db"
	"github.com/go-kit/kit/log"
	kithttp "github.com/go-kit/kit/transport/http"
	"github.com/gorilla/mux"
)

//go:generate swagger generate spec -m -o swagger.spec

type App struct {
	eventGetter db.RecordGetter
	logger      log.Logger
	getLimit    int
	decrypter   cipher.Decrypt

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
	Body []db.Event
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

func (app *App) getDeviceInfo(deviceID string) ([]db.Event, error) {
	records, hErr := app.eventGetter.GetRecords(deviceID, app.getLimit)
	events := []db.Event{}

	// if both have errors or are empty, return an error
	if hErr != nil {
		return events, serverErr{emperror.WrapWith(hErr, "Failed to get events", "device id", deviceID),
			http.StatusInternalServerError}
	}

	// if all is good, unmarshal everything
	for _, record := range records {
		// if the record is expired, don't include it
		if time.Unix(record.DeathDate, 0).Before(time.Now()) {
			continue
		}

		var event db.Event
		data, err := app.decrypter.DecryptMessage(record.Data)

		if err != nil {
			app.measures.DecryptFailure.Add(1.0)
			logging.Error(app.logger).Log(logging.MessageKey(), "Failed to decode event", logging.ErrorKey(), err.Error())
			continue
		}

		err = json.Unmarshal(data, &event)
		if err != nil {
			app.measures.DecryptFailure.Add(1.0)
			logging.Error(app.logger).Log(logging.MessageKey(), "Failed to unmarshal decode event", logging.ErrorKey(), err.Error())
			err = json.Unmarshal(record.Data, &event)
			if err != nil {
				app.measures.UnmarshalFailure.Add(1.0)
				logging.Error(app.logger).Log(logging.MessageKey(), "Failed to unmarshal event", logging.ErrorKey(), err.Error())
				continue
			}
		}
		event.ID = record.ID
		events = append(events, event)
	}

	if len(events) == 0 {
		return events, serverErr{emperror.With(errors.New("No events found for device id"), "device id", deviceID),
			http.StatusNotFound}
	}
	return events, nil
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
		d   []db.Event
		err error
	)
	vars := mux.Vars(request)
	id := strings.ToLower(vars["deviceID"])
	if id == "" {
		writer.WriteHeader(http.StatusNotFound)
		return
	}
	if d, err = app.getDeviceInfo(id); err != nil {
		logging.Error(app.logger, emperror.Context(err)...).Log(logging.MessageKey(),
			"Failed to get status info", logging.ErrorKey(), err.Error())
		if val, ok := err.(kithttp.StatusCoder); ok {
			writer.WriteHeader(val.StatusCode())
			return
		}
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	data, err := json.Marshal(&d)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	writer.Write(data)
}
