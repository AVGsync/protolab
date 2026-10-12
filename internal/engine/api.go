// Package engine applies an interpretation spec to reassembled streams:
// framing, rule matching, field parsing, assertions against the action log,
// and corpus-wide verification.
package engine

import (
	"context"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/spec"
)

// Version identifies the engine semantics; stored with every verification run.
const Version = "protolab-engine/1.1" // 1.1: framing syncs first when the connection start was not captured

// StreamInput is one reassembled direction.
type StreamInput struct {
	Bytes    []byte
	Gaps     []model.StreamGap // unknown ranges; their bytes must never be interpreted
	Segments []model.StreamSegment
	// UnknownStart: the start of the connection was not captured
	// (DirectedStream.StartKnown is false), so offset 0 is just the first
	// byte seen, not a message boundary. Length-prefixed framing looks for
	// the sync pattern first; with a delimiter the first message is partial.
	UnknownStart bool
}

// SessionInput is everything needed to parse one session.
type SessionInput struct {
	Session model.TcpSession
	Streams [2]StreamInput      // [0] c2s, [1] s2c
	Actions []model.ActionEntry // actions linked to the session (experiment conditions)
	// MaxMessages, when positive, bounds ParseSession: framing stops counting
	// past it and a session with more messages is not built (TooMany), so a
	// framing that cuts a long stream into tiny messages cannot exhaust memory.
	// Verify ignores it.
	MaxMessages int
}

// Unframed is a range of a stream that does not belong to any message
// (lost synchronisation, missing connection start, garbage).
type Unframed struct {
	Direction model.Direction
	Offset    int
	Length    int
}

// ParsedSession is the result of ParseSession. Both slices are never nil.
type ParsedSession struct {
	Messages []model.Message // both directions, ordered by time; Index assigned
	Unframed []Unframed
	// TooMany: the session has more than SessionInput.MaxMessages messages;
	// Messages and Unframed are empty then.
	TooMany bool
}

// ParseSession frames and parses both directions of a session. Messages own
// a copy of their bytes; integer field values are int64, bytes/ascii values
// are strings. A nil or invalid spec yields no messages and leaves the whole
// stream unframed instead of failing.
func ParseSession(s *spec.Spec, in SessionInput) ParsedSession { return parseSession(s, in) }

// Result of a verification run. Finding.ID and Finding.RunID are left empty.
type Result struct {
	Summary  model.RunSummary
	Findings []model.Finding
}

// Verify applies the spec to every session of a corpus. progress may be nil.
// It returns ctx.Err() if cancelled. A panic while processing one session is
// recovered and reported as a capture_defect finding; the run continues.
func Verify(ctx context.Context, s *spec.Spec, inputs []SessionInput, progress func(done, total int)) (Result, error) {
	return verify(ctx, s, inputs, progress)
}
