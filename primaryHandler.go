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
	"fmt"
	"net/http"
	"sort"

	"github.com/Comcast/codex/db"
	"github.com/Comcast/webpa-common/xhttp"
	"github.com/go-kit/kit/log"
	"github.com/goph/emperror"
	"github.com/gorilla/mux"
)

//go:generate swagger generate spec -m -o swagger.spec

type App struct {
	tombstoneGetter db.TombstoneGetter
	historyGetter   db.HistoryGetter
	logger          log.Logger
}

// swagger:parameters getAll getLastState getHardware
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
type EventResponse []db.Event

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
	history, hErr := app.historyGetter.GetHistory(id)
	tombstone, tErr := app.tombstoneGetter.GetTombstone(id)

	// if both have errors or are empty, return an error
	if hErr != nil && tErr != nil {
		xhttp.WriteError(writer, 404, hErr)
		return []db.Event{}, false
	}
	if len(history.Events) == 0 && len(tombstone) == 0 {
		xhttp.WriteError(writer, 500, fmt.Errorf("recieved an empty object"))
		return []db.Event{}, false
	}

	// if all is good, combine tombstone into history, sort the list, and remove any duplicates.
	sortedHistory := combineIntoSortedList(history, tombstone)

	writer.Header().Set("X-Codex-Device-Id", id)
	return sortedHistory, true
}

func combineIntoSortedList(history db.History, tombstone db.Tombstone) []db.Event {
	if len(tombstone) == 0 {
		return removeDuplicates(sortEvents(history.Events))
	}
	eventList := history.Events
	for _, val := range tombstone {
		eventList = append(eventList, val)
	}
	return removeDuplicates(sortEvents(eventList))
}

func sortEvents(events []db.Event) []db.Event {
	sortedEvents := events
	sort.Slice(sortedEvents, func(i, j int) bool {
		return sortedEvents[i].Time < sortedEvents[j].Time
	})
	return sortedEvents
}

func removeDuplicates(events []db.Event) []db.Event {
	if len(events) == 0 {
		return events
	}
	uniqueEvents := events[:1]
	for i := 1; i < len(events); i++ {
		if events[i].ID != events[i-1].ID {
			uniqueEvents = append(uniqueEvents, events[i])
		}
	}
	return uniqueEvents
}

/*
 * swagger:route GET /device/{deviceID} device getAll
 *
 * Gets all of the information related to a specific device id.
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
func (app *App) handleGetAll(writer http.ResponseWriter, request *http.Request) {
	var (
		d  []db.Event
		ok bool
	)
	if d, ok = app.getDeviceInfo(writer, request); !ok {
		return
	}

	data, err := json.Marshal(&d)
	if err != nil {
		xhttp.WriteError(writer, 500, emperror.WrapWith(err, "failed to marshal db.Device"))
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(200)
	writer.Write(data)
}
