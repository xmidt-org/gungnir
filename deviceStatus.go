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
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/xmidt-org/voynicrypto"

	kithttp "github.com/go-kit/kit/transport/http"
	"github.com/goph/emperror"
	"github.com/gorilla/mux"
	db "github.com/xmidt-org/codex-db"
	"github.com/xmidt-org/webpa-common/v2/logging"
	"github.com/xmidt-org/wrp-go/v3"
)

const (
	payloadKey = "reason-for-closure"
)

// note: below may be separated later into a separate service

// Status contains information on the current state of the device, how long it
// has been like that, and the last reason for going offline.
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
		var coder kithttp.StatusCoder
		if errors.As(err, &coder) {
			writer.WriteHeader(coder.StatusCode())
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

		// if the record is expired, don't include it
		if time.Unix(0, record.DeathDate).Before(time.Now()) {
			continue
		}

		item, err := app.parseState(deviceID, record)
		if err != nil {
			logging.Error(app.logger).Log(logging.MessageKey(), err)
		}

		if item.status.State == "offline" {
			if item.status.Since.After(lastOfflineEvent.status.Since) {
				lastOfflineEvent = item
			}
		}
		if item.status.State == "online" {
			if item.status.Since.After(lastOnlineEvent.status.Since) {
				lastOnlineEvent = item
			}
		}
	}

	if lastOfflineEvent.status.State == "" && lastOnlineEvent.status.State == "" {
		return Status{}, serverErr{emperror.With(errors.New("No events found for device id"), "device id", deviceID),
			http.StatusNotFound}
	}

	return determineStatus(lastOnlineEvent, lastOfflineEvent), nil
}

func (app *App) parseState(deviceID string, record db.Record) (eventTuple, error) {
	decrypter, ok := app.decrypters.Get(voynicrypto.ParseAlgorithmType(record.Alg), record.KID)
	if !ok {
		app.measures.GetDecryptFailure.Add(1.0)
		return eventTuple{}, errors.New("failed to find decrypter")
	}
	data, err := decrypter.DecryptMessage(record.Data, record.Nonce)
	if err != nil {
		app.measures.DecryptFailure.Add(1.0)
		return eventTuple{}, fmt.Errorf("failed to decrypt event: %v", err)
	}

	var event wrp.Message
	decoder := wrp.NewDecoderBytes(data, wrp.Msgpack)
	err = decoder.Decode(&event)
	if err != nil {
		app.measures.UnmarshalFailure.Add(1.0)
		return eventTuple{}, fmt.Errorf("failed to decode event: %v", err)
	}
	var payload map[string]interface{}
	err = json.Unmarshal(event.Payload, &payload)
	if err != nil {
		return eventTuple{}, fmt.Errorf("failed to unmarshal payload: %v", err)
	}

	s := Status{
		DeviceID:   deviceID,
		State:      path.Base(event.Destination),
		Since:      time.Unix(0, record.BirthDate),
		Now:        time.Now(),
		PartnerIDs: event.PartnerIDs,
	}

	if value, ok := payload[payloadKey]; ok && s.LastOfflineReason == "" {
		s.LastOfflineReason = value.(string)
	}

	item := eventTuple{
		record:    record,
		status:    s,
		sessionID: event.SessionID,
	}
	return item, nil
}

func determineStatus(lastOnline, lastOffline eventTuple) Status {
	if lastOffline.status.State == "" {
		return lastOnline.status
	}
	if lastOnline.status.State == "" {
		return lastOffline.status
	}
	if (lastOnline.sessionID == "" || lastOffline.sessionID == "") && lastOnline.status.Since.After(lastOffline.status.Since) {
		return lastOnline.status
	}
	if lastOffline.sessionID == lastOnline.sessionID {
		return lastOffline.status
	}
	return lastOnline.status
}
