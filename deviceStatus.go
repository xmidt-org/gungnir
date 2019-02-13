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
	"path"
	"time"

	"github.com/Comcast/codex/db"
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/xhttp"
	"github.com/goph/emperror"
	"github.com/gorilla/mux"
)

// note: below may be separated later into a separate service

type Status struct {
	DeviceId          string    `json:"deviceid"`
	State             string    `json:"state"`
	Since             time.Time `json:"since"`
	Now               time.Time `json:"now"`
	LastOfflineReason string    `json:"last_offline_reason,omitempty"`
}

func (app *App) handleGetStatus(writer http.ResponseWriter, request *http.Request) {
	var (
		s  Status
		ok bool
	)
	if s, ok = app.getStatusInfo(writer, request); !ok {
		return
	}

	data, err := json.Marshal(&s)
	if err != nil {
		xhttp.WriteError(writer, 500, nil)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(200)
	writer.Write(data)
}

func (app *App) getStatusInfo(writer http.ResponseWriter, request *http.Request) (Status, bool) {
	var (
		s Status
	)

	vars := mux.Vars(request)
	id := vars["deviceID"]
	stateInfo, hErr := app.eventGetter.GetRecordsOfType(id, db.EventState)

	if hErr != nil {
		logging.Error(app.logger, emperror.Context(hErr)...).Log(logging.MessageKey(), "Failed to get state records",
			logging.ErrorKey(), hErr.Error(), "device id", id)
		xhttp.WriteError(writer, 500, nil)
		return s, false
	}

	// if all is good, create our Status record
	for _, record := range stateInfo {
		// if we have state and last offline reason, we don't need to search anymore
		if s.State != "" && s.LastOfflineReason != "" {
			break
		}
		// if the record is expired, don't include it
		if record.DeathDate.Before(time.Now()) {
			continue
		}

		var event db.Event
		err := json.Unmarshal(record.Data, &event)
		if err != nil {
			logging.Error(app.logger).Log(logging.MessageKey(), "Failed to unmarshal event", logging.ErrorKey(), err.Error())
			break
		}
		var payload map[string]interface{}
		err = json.Unmarshal(event.Payload, &payload)
		if err != nil {
			logging.Error(app.logger).Log(logging.MessageKey(), "Failed to unmarshal payload",
				logging.ErrorKey(), hErr.Error())
			break
		}

		if value, ok := payload["reason-for-close"]; ok && s.LastOfflineReason == "" {
			s.LastOfflineReason = value.(string)
		}

		if s.State == "" {
			s.DeviceId = id
			s.State = path.Base(event.Destination)
			s.Since = record.BirthDate
			s.Now = time.Now()
		}
	}

	if s.State == "" {
		logging.Error(app.logger).Log(logging.MessageKey(), "No events founds for device id",
			"device id", id)
		xhttp.WriteError(writer, 404, nil)
		return Status{}, false
	}

	writer.Header().Set("X-Codex-Device-Id", id)
	return s, true
}
