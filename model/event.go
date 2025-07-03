// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"github.com/xmidt-org/wrp-go/v3"
)

//go:generate codecgen -st "json" -o event_codec.go event.go

// Event is the extension of wrp message
//
//	https://github.com/xmidt-org/wrp-c/wiki/Web-Routing-Protocol
//
// swagger:response Event
type Event struct {
	wrp.Message

	// BirthDate the time codex received the message
	//
	// required: false
	// example: 1555639704
	BirthDate int64 `json:"birth_date,omitempty"`
}
