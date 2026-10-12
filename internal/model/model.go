// Package model holds the JSON data transfer objects of the HTTP API.
//
// The structs mirror the TypeScript types in frontend/src/entities/*/model/types.ts
// field by field; docs/api.md describes the semantics. Keep both in sync.
package model

import "time"

// Direction inside a TCP session.
type Direction string

const (
	C2S Direction = "c2s" // client -> server
	S2C Direction = "s2c" // server -> client
)

// Directions lists both directions in a stable order.
var Directions = [2]Direction{C2S, S2C}

// Opposite returns the other direction.
func (d Direction) Opposite() Direction {
	if d == C2S {
		return S2C
	}
	return C2S
}

// Valid reports whether d is c2s or s2c.
func (d Direction) Valid() bool { return d == C2S || d == S2C }

const (
	// PacketTimeLayout keeps nanoseconds: capture timestamps may have them.
	PacketTimeLayout = "2006-01-02T15:04:05.000000000Z07:00"
	// EntityTimeLayout is used for creation/update times of stored objects.
	EntityTimeLayout = "2006-01-02T15:04:05.000Z07:00"
)

// PacketTime formats a capture timestamp (UTC, fixed nanosecond precision).
func PacketTime(t time.Time) string { return t.UTC().Format(PacketTimeLayout) }

// EntityTime formats the time an object was created or changed.
func EntityTime(t time.Time) string { return t.UTC().Format(EntityTimeLayout) }

// Now is the current entity time.
func Now() string { return EntityTime(time.Now()) }

// ListResponse is the envelope of every list endpoint.
type ListResponse[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// List wraps items into a ListResponse, never encoding a nil slice as null.
func List[T any](items []T) ListResponse[T] {
	if items == nil {
		items = []T{}
	}
	return ListResponse[T]{Items: items, Total: len(items)}
}

// Endpoint is an IPv4 address and TCP port.
type Endpoint struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

// ByteRange is a half-open range [Offset, Offset+Length).
type ByteRange struct {
	Offset int `json:"offset"`
	Length int `json:"length"`
}

// End returns the first offset after the range.
func (r ByteRange) End() int { return r.Offset + r.Length }

// SourceRef points at the exact bytes of an immutable capture a derived result came from.
type SourceRef struct {
	CaptureID     string    `json:"captureId"`
	CaptureSHA256 string    `json:"captureSha256"`
	SessionID     string    `json:"sessionId"`
	Direction     Direction `json:"direction"`
	Offset        int       `json:"offset"`
	Length        int       `json:"length"`
	PacketNumbers []int     `json:"packetNumbers"`
}

// Diagnostic levels.
const (
	LevelInfo    = "info"
	LevelWarning = "warning"
	LevelError   = "error"
)

// Diagnostic is an explicit report about unsupported, incomplete or ambiguous input.
type Diagnostic struct {
	Level         string `json:"level"`
	Code          string `json:"code"`
	Message       string `json:"message"`
	SessionID     string `json:"sessionId,omitempty"`
	PacketNumbers []int  `json:"packetNumbers,omitempty"`
	Count         int    `json:"count,omitempty"`
}
