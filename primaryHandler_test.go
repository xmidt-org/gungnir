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
	"github.com/xmidt-org/gungnir/model"
	"github.com/xmidt-org/wrp-go/wrp"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/voynicrypto"

	kithttp "github.com/go-kit/kit/transport/http"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/xmidt-org/codex-db"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/xmetrics/xmetricstest"
)

var (
	goodEvent = wrp.Message{
		//ID: 1234,
		//Time:        567890974,
		Source:      "test source",
		Destination: "/test/online",
		PartnerIDs:  []string{"test1", "test2"},
		Payload: []byte(`{
			"id": "mac:48f7c0d79024",
			"ts": "2019-02-14T21:19:02.614191735Z",
			"bytes-sent": 0,
			"messages-sent": 1,
			"bytes-received": 0,
			"messages-received": 0,
			"connected-at": "2018-11-22T21:19:02.614191735Z",
			"up-time": "16m46.6s",
			"reason-for-closure": "ping miss"
		}`),
	}
)

func TestGetDeviceInfo(t *testing.T) {
	getRecordsErr := errors.New("get records test error")

	testassert := assert.New(t)
	futureTime := time.Now().Add(time.Duration(50000) * time.Minute).UnixNano()
	prevTime, err := time.Parse(time.RFC3339Nano, "2019-02-13T21:19:02.614191735Z")
	testassert.Nil(err)
	previousTime := prevTime.UnixNano()

	var goodData []byte
	encoder := wrp.NewEncoderBytes(&goodData, wrp.Msgpack)
	err = encoder.Encode(&goodEvent)
	testassert.Nil(err)
	badData, err := json.Marshal("")
	testassert.Nil(err)

	tests := []struct {
		description           string
		recordsToReturn       []db.Record
		getRecordsErr         error
		decryptErr            error
		expectedFailureMetric float64
		expectedEvents        []model.Event
		expectedErr           error
		expectedStatus        int
	}{
		{
			description:    "Get Records Error",
			getRecordsErr:  getRecordsErr,
			expectedEvents: []model.Event{},
			expectedErr:    getRecordsErr,
			expectedStatus: http.StatusInternalServerError,
		},
		{
			description:    "Empty Records Error",
			expectedEvents: []model.Event{},
			expectedErr:    errors.New("No events found"),
			expectedStatus: http.StatusNotFound,
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
			expectedEvents: []model.Event{},
			expectedErr:    errors.New("No events found"),
			expectedStatus: http.StatusNotFound,
		},
		{
			description: "Decode Event Error",
			recordsToReturn: []db.Record{
				{
					DeathDate: futureTime,
					Data:      badData,
					Alg:       string(voynicrypto.None),
					KID:       "none",
				},
			},
			expectedFailureMetric: 1.0,
			expectedEvents:        []model.Event{model.Event{wrp.Message{Type: 11}, 0}},
		},
		{
			description: "Decrypt Error",
			recordsToReturn: []db.Record{
				{
					DeathDate: futureTime,
					Data:      goodData,
					Alg:       string(voynicrypto.None),
					KID:       "none",
				},
			},
			decryptErr:     errors.New("failed to decrypt"),
			expectedEvents: []model.Event{model.Event{wrp.Message{Type: 11}, 0}},
		},
		{
			description: "No Decrypter",
			recordsToReturn: []db.Record{
				{
					BirthDate: prevTime.UnixNano(),
					DeathDate: futureTime,
					Data:      goodData,
					Alg:       string(voynicrypto.Box),
					KID:       "test",
				},
			},
			expectedEvents: []model.Event{
				model.Event{wrp.Message{Type: 11}, prevTime.UnixNano()},
			},
		},
		{
			description: "Success",
			recordsToReturn: []db.Record{
				{
					BirthDate: prevTime.UnixNano(),
					DeathDate: futureTime,
					Data:      goodData,
					Alg:       string(voynicrypto.None),
					KID:       "none",
				},
			},
			expectedEvents: []model.Event{
				model.Event{goodEvent, prevTime.UnixNano()},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			assert := assert.New(t)
			require := require.New(t)
			mockGetter := new(mockRecordGetter)
			mockGetter.On("GetRecords", "test", 5,"").Return(tc.recordsToReturn, tc.getRecordsErr).Once()
			mockGetter.On("GetStateHash", mock.Anything).Return("123", nil).Once()

			mockDecrypter := new(mockDecrypter)
			mockDecrypter.On("DecryptMessage", mock.Anything, mock.Anything).Return(tc.decryptErr)

			ciphers := voynicrypto.Ciphers{
				Options: map[voynicrypto.AlgorithmType]map[string]voynicrypto.Decrypt{
					voynicrypto.None: map[string]voynicrypto.Decrypt{
						"none": mockDecrypter,
					},
				},
			}

			p := xmetricstest.NewProvider(nil, Metrics)
			m := NewMeasures(p)
			app := App{
				eventGetter:   mockGetter,
				logger:        logging.DefaultLogger(),
				decrypters:    ciphers,
				measures:      m,
				getEventLimit: 5,
			}
			p.Assert(t, UnmarshalFailureCounter)(xmetricstest.Value(0.0))
			events, _, err := app.getDeviceInfo("test")
			p.Assert(t, UnmarshalFailureCounter)(xmetricstest.Value(tc.expectedFailureMetric))
			assert.Equal(tc.expectedEvents, events)

			if tc.expectedErr == nil || err == nil {
				assert.Equal(tc.expectedErr, err)
			} else {
				assert.Contains(err.Error(), tc.expectedErr.Error())
			}
			if tc.expectedStatus > 0 {
				statusCodeErr, ok := err.(kithttp.StatusCoder)
				require.True(ok, "expected error to have a status code")
				assert.Equal(tc.expectedStatus, statusCodeErr.StatusCode())
			}
		})
	}
}

func TestHandleGetEvents(t *testing.T) {
	testassert := assert.New(t)
	futureTime := time.Now().Add(time.Duration(50000) * time.Minute).UnixNano()
	var goodData []byte
	encoder := wrp.NewEncoderBytes(&goodData, wrp.Msgpack)
	err := encoder.Encode(&goodEvent)
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
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			assert := assert.New(t)
			mockGetter := new(mockRecordGetter)
			mockGetter.On("GetRecords", tc.deviceID, 5,"").Return(tc.recordsToReturn, nil).Once()
			mockGetter.On("GetStateHash", mock.Anything).Return("123", nil).Once()

			ciphers := voynicrypto.Ciphers{
				Options: map[voynicrypto.AlgorithmType]map[string]voynicrypto.Decrypt{
					voynicrypto.None: map[string]voynicrypto.Decrypt{
						"none": new(voynicrypto.NOOP),
					},
				},
			}
			p := xmetricstest.NewProvider(nil, Metrics)
			m := NewMeasures(p)

			app := App{
				eventGetter:   mockGetter,
				getEventLimit: 5,
				logger:        logging.DefaultLogger(),
				decrypters:    ciphers,
				measures:      m,
			}
			rr := httptest.NewRecorder()
			request := mux.SetURLVars(
				httptest.NewRequest("GET", "/1234/status", nil),
				map[string]string{"deviceID": tc.deviceID},
			)
			app.handleGetEvents(rr, request)
			assert.Equal(tc.expectedStatusCode, rr.Code)
		})
	}
}
