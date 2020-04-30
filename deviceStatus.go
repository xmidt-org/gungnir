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
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/xmidt-org/voynicrypto"

	kithttp "github.com/go-kit/kit/transport/http"
	"github.com/goph/emperror"
	"github.com/gorilla/mux"
	"github.com/xmidt-org/codex-db"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/wrp-go/v2"
)

const (
	payloadKey = "reason-for-closure"
)

// note: below may be separated later into a separate service

// StatusResponse is what is returned for a successful getStatus call.
//
// swagger:response StatusResponse
type StatusResponse struct {
	// in:body
	Body Status
}

// Status contains information on the current state of the device, how long it
// has been like that, and the last reason for going offline
//
// swagger:model Status
type Status struct {
	// the device id
	//
	// required: true
	// example: 5
	DeviceID string `json:"deviceid"`

	// State of the device. Ex: online, offline
	//
	// required: true
	// example: online
	State string `json:"state"`

	// The time the device state event was created by talaria
	//
	// required: true
	// example: 2019-02-26T20:18:15.188881748Z
	Since time.Time `json:"since"`

	// the current time
	//
	// required: true
	// example: 2019-02-26T20:18:15.188881748Z
	Now time.Time `json:"now"`

	// the last reason the device went offline.
	//
	// required: true
	// example: ping miss
	LastOfflineReason string `json:"last_offline_reason"`

	// the partner ids used by the device.  Determined from the same event that
	// provides the state
	//
	// required: true
	// example: [".*", "example partner"]
	PartnerIDs []string `json:"partner_ids"`
}

/*
 * swagger:route GET /device/{deviceID}/status device getStatus
 *
 * Get the status information for a specified device.
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
 *    200: StatusResponse
 *    404: ErrResponse
 *    500: ErrResponse
 *
 */
func (app *App) handleGetStatus(writer http.ResponseWriter, request *http.Request) {
	var (
		s   Status
		err error
	)
	vars := mux.Vars(request)
	id := strings.ToLower(vars["deviceID"])
	if id == "" {
		writer.WriteHeader(http.StatusNotFound)
		return
	}
	if s, err = app.getStatusInfo(id); err != nil {
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

	data, err := json.Marshal(&s)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	writer.Write(data)
}

type eventTuple struct {
	record    db.Record
	status    Status
	sessionID string
}

func (app *App) getStatusInfo(deviceID string) (Status, error) {

	stateInfo, hErr := app.eventGetter.GetRecordsOfType(deviceID, app.getStatusLimit, db.State, "")
	if hErr != nil {
		return Status{}, serverErr{emperror.WrapWith(hErr, "Failed to get state records", "device id", deviceID),
			http.StatusInternalServerError}
	}

	var (
		lastOfflineEvent eventTuple
		lastOnlineEvent  eventTuple
	)
	for _, record := range stateInfo {
		// if all is good, create our status record
		var (
			s Status
		)
		// if we have state and last offline reason, we don't need to search anymore
		if s.State != "" && s.LastOfflineReason != "" {
			break
		}
		// if the record is expired, don't include it
		if time.Unix(0, record.DeathDate).Before(time.Now()) {
			continue
		}

		var event wrp.Message
		decrypter, ok := app.decrypters.Get(voynicrypto.ParseAlgorithmType(record.Alg), record.KID)
		if !ok {
			app.measures.GetDecryptFailure.Add(1.0)
			logging.Error(app.logger).Log(logging.MessageKey(), "Failed to find decrypter")
			continue
		}
		data, err := decrypter.DecryptMessage(record.Data, record.Nonce)
		if err != nil {
			app.measures.DecryptFailure.Add(1.0)
			logging.Error(app.logger).Log(logging.MessageKey(), "Failed to decrypt event", logging.ErrorKey(), err.Error())
			continue
		}

		decoder := wrp.NewDecoderBytes(data, wrp.Msgpack)
		err = decoder.Decode(&event)
		if err != nil {
			app.measures.UnmarshalFailure.Add(1.0)
			logging.Error(app.logger).Log(logging.MessageKey(), "Failed to decode event", logging.ErrorKey(), err.Error())
			continue
		}
		var payload map[string]interface{}
		err = json.Unmarshal(event.Payload, &payload)
		if err != nil {
			logging.Error(app.logger).Log(logging.MessageKey(), "Failed to unmarshal payload",
				logging.ErrorKey(), err.Error())
			continue
		}

		if value, ok := payload[payloadKey]; ok && s.LastOfflineReason == "" {
			s.LastOfflineReason = value.(string)
		}

		if s.State == "" {
			s.DeviceID = deviceID
			s.State = path.Base(event.Destination)
			s.Since = time.Unix(0, record.BirthDate)
			s.Now = time.Now()
			s.PartnerIDs = event.PartnerIDs
		}
		if s.State == "offline" {
			if s.Since.After(lastOfflineEvent.status.Since) {
				lastOfflineEvent = eventTuple{
					record:    record,
					status:    s,
					sessionID: event.SessionID,
				}
			}
		}
		if s.State == "online" {
			if s.Since.After(lastOnlineEvent.status.Since) {
				lastOnlineEvent = eventTuple{
					record:    record,
					status:    s,
					sessionID: event.SessionID,
				}
			}
		}
	}

	if lastOfflineEvent.status.State == "" && lastOnlineEvent.status.State == "" {
		return Status{}, serverErr{emperror.With(errors.New("No events found for device id"), "device id", deviceID),
			http.StatusNotFound}
	} else if lastOfflineEvent.status.State == "" && lastOnlineEvent.status.State != "" {
		return lastOnlineEvent.status, nil
	} else if lastOfflineEvent.status.State != "" && lastOnlineEvent.status.State == "" {
		return lastOfflineEvent.status, nil
	}

	if lastOfflineEvent.sessionID == lastOnlineEvent.sessionID {
		return lastOfflineEvent.status, nil
	}
	return lastOnlineEvent.status, nil
}
