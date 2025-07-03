// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/provider"
	"github.com/xmidt-org/webpa-common/v2/xmetrics" //nolint: staticcheck
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
