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
	"github.com/Comcast/codex/db"
	"github.com/Comcast/webpa-common/xhttp"
	"github.com/go-kit/kit/log"
	"github.com/goph/emperror"
	"github.com/gorilla/mux"
	"gopkg.in/couchbase/gocb.v1"
	"net/http"
)

//go:generate swagger generate spec -m -o swagger.spec

type App struct {
	db     db.Connection
	logger log.Logger
}

// swagger:parameters getAll getLastState getHardware
type DeviceIdParam struct {
	// device id passed by caller
	//
	// in: path
	// required: true
	DeviceID string `json:"deviceID"`
}

// ErrResponse is the information passed to the client on an error
//
// swagger:model ErrResponse
type ErrResponse struct {
	// The http code of the response
	//
	// required: true
	Code int `json:"code"`

	// the error message
	//
	//required: true
	Message interface{} `json:"message"`
}

func (app *App) getDeviceInfo(writer http.ResponseWriter, request *http.Request) (interface{}, bool) {
	vars := mux.Vars(request)
	id := vars["deviceID"]
	history, err := app.db.GetHistory(id)
	if err != nil {

		// if there is no history, get the tombstone
		keyNotFound := false
		emperror.ForEachCause(err, func(errPart error) bool {
			if errPart == gocb.ErrKeyNotFound {
				keyNotFound = true
				return false
			}
			return true
		})
		if keyNotFound {
			return app.getTombstone(writer, id)
		}

		xhttp.WriteError(writer, 404, err)
		return nil, false
	} else if len(history.Events) == 0 {
		xhttp.WriteError(writer, 500, fmt.Errorf("recieved an empty object"))
		return nil, false
	}
	writer.Header().Set("X-Codex-Device-Id", id)
	return history, true
}

func (app *App) getTombstone(writer http.ResponseWriter, id string) (interface{}, bool) {
	d, err := app.db.GetTombstone(id)
	if err != nil {
		xhttp.WriteError(writer, 404, err)
		return nil, false
	} else if len(d) == 0 {
		xhttp.WriteError(writer, 500, fmt.Errorf("recieved an empty object"))
		return nil, false
	}
	writer.Header().Set("X-Codex-Device-Id", id)
	return d, true
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
 *    200: Device
 *    404: ErrResponse
 *    500: ErrResponse
 *
 */
func (app *App) handleGetAll(writer http.ResponseWriter, request *http.Request) {
	var (
		d  interface{}
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

/*
 * swagger:route GET /device/{deviceID}/last device getLastState
 *
 * Gets information on the last state for a particular device
 *
 * Parameters: deviceID
 *
 * Produces:
 *    - application/json
 *
 * Schemes: http
 *
 * Responses:
 *    200: State
 *    404: ErrResponse
 *    500: ErrResponse
 */
// func (app *App) handleGetLastState(writer http.ResponseWriter, request *http.Request) {
// 	var (
// 		d  db.Device
// 		ok bool
// 	)
// 	if d, ok = app.getDevice(writer, request); !ok {
// 		return
// 	}

// 	if len(d.States) > 0 {
// 		lastState := d.States[0]
// 		data, err := json.Marshal(&lastState)
// 		if err != nil {
// 			xhttp.WriteError(writer, 500, err)
// 			return
// 		}
// 		writer.Header().Set("Content-Type", "application/json")
// 		writer.WriteHeader(200)
// 		writer.Write(data)
// 	} else {
// 		xhttp.WriteError(writer, 500, fmt.Errorf("no states found"))
// 	}
// }
