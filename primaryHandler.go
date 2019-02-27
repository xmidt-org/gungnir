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
	"net/http"
	"time"

	"github.com/Comcast/webpa-common/logging"
	"github.com/goph/emperror"

	"github.com/Comcast/codex/db"
	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
)

//go:generate swagger generate spec -m -o swagger.spec

type App struct {
	eventGetter db.RecordGetter
	logger      log.Logger
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

func (app *App) getDeviceInfo(writer http.ResponseWriter, request *http.Request) ([]db.Event, bool) {
	vars := mux.Vars(request)
	id := vars["deviceID"]
	records, hErr := app.eventGetter.GetRecords(id)

	// if both have errors or are empty, return an error
	if hErr != nil {
		logging.Error(app.logger, emperror.Context(hErr)...).Log(logging.MessageKey(), "Failed to get events",
			logging.ErrorKey(), hErr.Error(), "device id", id)
		writer.WriteHeader(500)
		return []db.Event{}, false
	}

	// if all is good, unmarshal everything
	events := []db.Event{}
	for _, record := range records {
		// if the record is expired, don't include it
		if record.DeathDate.Before(time.Now()) {
			continue
		}

		var event db.Event
		err := json.Unmarshal(record.Data, &event)
		if err != nil {
			logging.Error(app.logger).Log(logging.MessageKey(), "Failed to unmarshal event", logging.ErrorKey(), err.Error())
		} else {
			event.ID = record.ID
			events = append(events, event)
		}
	}

	if len(events) == 0 {
		logging.Error(app.logger).Log(logging.MessageKey(), "No events founds for device id",
			"device id", id)
		writer.WriteHeader(404)
		return []db.Event{}, false
	}

	writer.Header().Set("X-Codex-Device-Id", id)
	return events, true
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
 * Schemes: http
 *
 * Responses:
 *    200: EventResponse
 *    404: ErrResponse
 *    500: ErrResponse
 *
 */
func (app *App) handleGetEvents(writer http.ResponseWriter, request *http.Request) {
	var (
		d  []db.Event
		ok bool
	)
	if d, ok = app.getDeviceInfo(writer, request); !ok {
		return
	}

	data, err := json.Marshal(&d)
	if err != nil {
		writer.WriteHeader(500)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(200)
	writer.Write(data)
}
