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
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	kithttp "github.com/go-kit/kit/transport/http"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/xmidt-org/voynicrypto"
	"github.com/xmidt-org/webpa-common/v2/logging"
	"github.com/xmidt-org/webpa-common/v2/xmetrics/xmetricstest"
	"github.com/xmidt-org/wrp-go/v3"

	db "github.com/xmidt-org/codex-db"
)

func TestGetStatusInfo(t *testing.T) {
	getRecordsErr := errors.New("get records of type test error")

	testassert := assert.New(t)
	futureTime := time.Now().Add(time.Duration(50000) * time.Minute).UnixNano()
	prevTime, err := time.Parse(time.RFC3339Nano, "2019-02-13T21:19:02.614191735Z")
	testassert.Nil(err)
	previousTime := prevTime.UnixNano()

	var goodData []byte
	encoder := wrp.NewEncoderBytes(&goodData, wrp.Msgpack)
	err = encoder.Encode(&goodOnlineEvent)
	testassert.Nil(err)
	event := goodOnlineEvent
	event.Payload = []byte("")
	var emptyPayloadData []byte
	encoder = wrp.NewEncoderBytes(&emptyPayloadData, wrp.Msgpack)
	err = encoder.Encode(&event)
	testassert.Nil(err)
	badData, err := json.Marshal("")
	testassert.Nil(err)

	// create offline event
	var offlineData []byte
	offlineEncoder := wrp.NewEncoderBytes(&offlineData, wrp.Msgpack)
	err = offlineEncoder.Encode(&goodOfflineEvent)
	testassert.Nil(err)

	tests := []struct {
		description          string
		recordsToReturn      []db.Record
		getRecordsErr        error
		decryptErr           error
		expectedStatus       Status
		expectedErr          error
		expectedServerStatus int
	}{
		{
			description:          "Get Records Error",
			getRecordsErr:        getRecordsErr,
			expectedStatus:       Status{},
			expectedErr:          getRecordsErr,
			expectedServerStatus: http.StatusInternalServerError,
		},
		{
			description:          "Empty Records Error",
			expectedStatus:       Status{},
			expectedErr:          errors.New("No events found"),
			expectedServerStatus: http.StatusNotFound,
		},
		{
			description: "Expired Records Error",
			recordsToReturn: []db.Record{
				db.Record{
					DeathDate: previousTime,
					Alg:       string(voynicrypto.None),
					KID:       "none",
				},
			},
			expectedStatus:       Status{},
			expectedErr:          errors.New("No events found"),
			expectedServerStatus: http.StatusNotFound,
		},
		{
			description: "Unmarshal Event Error",
			recordsToReturn: []db.Record{
				{
					DeathDate: futureTime,
					Data:      badData,
					Alg:       string(voynicrypto.None),
					KID:       "none",
				},
			},
			expectedStatus:       Status{},
			expectedErr:          errors.New("No events found"),
			expectedServerStatus: http.StatusNotFound,
		},
		{
			description: "Unmarshal Payload Error",
			recordsToReturn: []db.Record{
				{
					Type:      db.State,
					DeathDate: futureTime,
					Data:      emptyPayloadData,
					Alg:       string(voynicrypto.None),
					KID:       "none",
				},
			},
			expectedStatus:       Status{},
			expectedErr:          errors.New("No events found"),
			expectedServerStatus: http.StatusNotFound,
		},
		{
			description: "Decrypt Error",
			recordsToReturn: []db.Record{
				{
					Type:      db.State,
					BirthDate: futureTime - 500,
					DeathDate: futureTime,
					Data:      goodData,
					Alg:       string(voynicrypto.None),
					KID:       "none",
				},
				{
					Type:      db.State,
					DeathDate: futureTime,
					Data:      goodData,
					Alg:       string(voynicrypto.None),
					KID:       "none",
				},
			},
			expectedStatus:       Status{},
			decryptErr:           errors.New("failed to decrypt"),
			expectedErr:          errors.New("No events found"),
			expectedServerStatus: http.StatusNotFound,
		},
		{
			description: "Success-Online",
			recordsToReturn: []db.Record{
				{
					Type:      db.State,
					BirthDate: futureTime - 500,
					DeathDate: futureTime,
					Data:      goodData,
					Alg:       string(voynicrypto.None),
					KID:       "none",
				},
				{
					Type:      db.State,
					DeathDate: futureTime,
					Data:      goodData,
					Alg:       string(voynicrypto.None),
					KID:       "none",
				},
			},
			expectedStatus: Status{
				DeviceID:          "test",
				State:             "online",
				Since:             time.Unix(0, futureTime-500),
				Now:               time.Now(),
				LastOfflineReason: "ping miss",
			},
		},
		{
			description: "Success-Offline",
			recordsToReturn: []db.Record{
				{
					Type:      db.State,
					BirthDate: futureTime - 500,
					DeathDate: futureTime,
					Data:      offlineData,
					Alg:       string(voynicrypto.None),
					KID:       "none",
				},
			},
			expectedStatus: Status{
				DeviceID:          "test",
				State:             "offline",
				Since:             time.Unix(0, futureTime-500),
				Now:               time.Now(),
				LastOfflineReason: "ping miss",
			},
		},
		{
			description: "Success-Out-of-Order",
			recordsToReturn: []db.Record{
				{
					Type:      db.State,
					BirthDate: futureTime - 500,
					DeathDate: futureTime,
					Data:      offlineData,
					Alg:       string(voynicrypto.None),
					KID:       "none",
				},
				{
					Type:      db.State,
					BirthDate: futureTime - 700,
					DeathDate: futureTime - 200,
					Data:      goodData,
					Alg:       string(voynicrypto.None),
					KID:       "none",
				},
			},
			expectedStatus: Status{
				DeviceID:          "test",
				State:             "online",
				Since:             time.Unix(0, futureTime-700),
				Now:               time.Now(),
				LastOfflineReason: "ping miss",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			assert := assert.New(t)
			mockGetter := new(mockRecordGetter)
			mockGetter.On("GetRecordsOfType", "test", 5, db.State, "").Return(tc.recordsToReturn, tc.getRecordsErr).Once()

			p := xmetricstest.NewProvider(nil, Metrics)
			m := NewMeasures(p)

			mockDecrypter := new(mockDecrypter)
			mockDecrypter.On("DecryptMessage", mock.Anything, mock.Anything).Return(tc.decryptErr)

			ciphers := voynicrypto.Ciphers{
				Options: map[voynicrypto.AlgorithmType]map[string]voynicrypto.Decrypt{
					voynicrypto.None: map[string]voynicrypto.Decrypt{
						"none": mockDecrypter,
					},
				},
			}

			app := App{
				eventGetter:    mockGetter,
				getStatusLimit: 5,
				logger:         logging.DefaultLogger(),
				decrypters:     ciphers,
				measures:       m,
			}
			status, err := app.getStatusInfo("test")

			// can't assert over the full status, since we can't check Now
			assert.Equal(tc.expectedStatus.DeviceID, status.DeviceID)
			assert.Equal(tc.expectedStatus.State, status.State)
			assert.Equal(tc.expectedStatus.Since, status.Since)
			assert.Equal(tc.expectedStatus.LastOfflineReason, status.LastOfflineReason)

			if tc.expectedErr == nil || err == nil {
				assert.Equal(tc.expectedErr, err)
			} else {
				assert.Contains(err.Error(), tc.expectedErr.Error())
			}
			if tc.expectedServerStatus > 0 {
				var coder kithttp.StatusCoder
				ok := errors.As(err, &coder)
				assert.True(ok, "expected error to have a status code")
				assert.Equal(tc.expectedServerStatus, coder.StatusCode())
			}
		})
	}
}

func TestHandleGetStatus(t *testing.T) {
	testassert := assert.New(t)
	futureTime := time.Now().Add(time.Duration(50000) * time.Minute).UnixNano()
	var goodData []byte
	encoder := wrp.NewEncoderBytes(&goodData, wrp.Msgpack)
	err := encoder.Encode(&goodOnlineEvent)
	testassert.Nil(err)

	tests := []struct {
		description        string
		deviceID           string
		recordsToReturn    []db.Record
		expectedStatusCode int
		expectedBody       []byte
	}{
		{
			description:        "Empty Device ID Error",
			deviceID:           "",
			expectedStatusCode: http.StatusNotFound,
		},
		{
			description:        "Get Device Info Error",
			deviceID:           "1234",
			expectedStatusCode: http.StatusNotFound,
		},
		{
			description: "Success",
			deviceID:    "1234",
			recordsToReturn: []db.Record{
				{
					DeathDate: futureTime,
					Data:      goodData,
					Alg:       string(voynicrypto.None),
					KID:       "none",
				},
			},
			expectedStatusCode: http.StatusOK,
			expectedBody:       goodData,
		},
		{
			description: "No Decrypter",
			deviceID:    "1234",
			recordsToReturn: []db.Record{
				{
					DeathDate: futureTime,
					Data:      goodData,
					Alg:       string(voynicrypto.Box),
					KID:       "test",
				},
			},
			expectedStatusCode: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			assert := assert.New(t)
			mockGetter := new(mockRecordGetter)
			mockGetter.On("GetRecordsOfType", tc.deviceID, 5, db.State, "").Return(tc.recordsToReturn, nil).Once()

			p := xmetricstest.NewProvider(nil, Metrics)
			m := NewMeasures(p)

			mockDecrypter := new(mockDecrypter)
			mockDecrypter.On("DecryptMessage", mock.Anything, mock.Anything).Return(nil)

			ciphers := voynicrypto.Ciphers{
				Options: map[voynicrypto.AlgorithmType]map[string]voynicrypto.Decrypt{
					voynicrypto.None: map[string]voynicrypto.Decrypt{
						"none": mockDecrypter,
					},
				},
			}

			app := App{
				eventGetter:    mockGetter,
				getStatusLimit: 5,
				logger:         logging.DefaultLogger(),
				decrypters:     ciphers,
				measures:       m,
			}
			rr := httptest.NewRecorder()
			request := mux.SetURLVars(
				httptest.NewRequest("GET", "/1234/status", nil),
				map[string]string{"deviceID": tc.deviceID},
			)
			app.handleGetStatus(rr, request)
			assert.Equal(tc.expectedStatusCode, rr.Code)
		})
	}
}
