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
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/provider"
	"github.com/xmidt-org/webpa-common/v2/xmetrics"
)

const (
	UnmarshalFailureCounter    = "unmarshal_failure_count"
	DecryptFailureCounter      = "decrypt_failure_count"
	GetDecrypterFailureCounter = "get_decrypter_failure_count"
	EventsReturnedCounter      = "events_returned_counter"
)

func Metrics() []xmetrics.Metric {
	return []xmetrics.Metric{
		{
			Name: UnmarshalFailureCounter,
			Help: "The total number of failures to unmarshal an event",
			Type: "counter",
		},
		{
			Name: DecryptFailureCounter,
			Help: "The total number of failures to decypt an event",
			Type: "counter",
		},
		{
			Name: GetDecrypterFailureCounter,
			Help: "The total number of failures to get the decypter",
			Type: "counter",
		},
		{
			Name: EventsReturnedCounter,
			Help: "The total number of events gungnir has responded with",
			Type: "counter",
		},
	}
}

type Measures struct {
	UnmarshalFailure    metrics.Counter
	DecryptFailure      metrics.Counter
	GetDecryptFailure   metrics.Counter
	EventsReturnedCount metrics.Counter
}

// NewMeasures constructs a Measures given a go-kit metrics Provider
func NewMeasures(p provider.Provider) *Measures {
	return &Measures{
		UnmarshalFailure:    p.NewCounter(UnmarshalFailureCounter),
		DecryptFailure:      p.NewCounter(DecryptFailureCounter),
		GetDecryptFailure:   p.NewCounter(GetDecrypterFailureCounter),
		EventsReturnedCount: p.NewCounter(EventsReturnedCounter),
	}
}
