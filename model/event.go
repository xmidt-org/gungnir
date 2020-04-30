package model

import (
	"github.com/xmidt-org/wrp-go/v2"
)

//go:generate codecgen -st "wrp" -o event_codec.go event.go

// Event is the extension of wrp message
//     https://github.com/xmidt-org/wrp-c/wiki/Web-Routing-Protocol
//
// swagger:response Event
type Event struct {
	wrp.Message

	// BirthDate the time codex received the message
	//
	// required: false
	// example: 1555639704
	BirthDate int64 `wrp:"birth_date,omitempty"`
}
