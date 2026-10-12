// Package spec defines the declarative interpretation format "protolab/v1"
// (see docs/interpretation-format.md): how to frame messages out of a TCP
// stream, which message types exist, their fields and assertions against the
// action log.
package spec

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
)

// Format is the only supported value of Spec.Format.
const Format = "protolab/v1"

// Meaning statuses of a field: what backs the claimed meaning.
const (
	MeaningStructural = "structural"
	MeaningHypothesis = "hypothesis"
	MeaningConfirmed  = "confirmed"
)

// Framing kinds.
const (
	FramingLengthPrefixed = "length_prefixed"
	FramingDelimiter      = "delimiter"
	FramingFixed          = "fixed"
)

type Spec struct {
	Format     string        `json:"format"`
	Protocol   string        `json:"protocol"`
	Transport  *Transport    `json:"transport,omitempty"`
	Framing    Framing       `json:"framing"`
	Messages   []MessageRule `json:"messages"`
	Assertions []Assertion   `json:"assertions,omitempty"`
}

type Transport struct {
	Port int `json:"port,omitempty"`
}

// Framing describes how messages are cut out of a directed stream.
// Fields that do not belong to Kind are ignored.
type Framing struct {
	Kind string `json:"kind"`

	// length_prefixed: total = value(LengthField) + LengthAdjust.
	LengthField  *LengthField `json:"lengthField,omitempty"`
	LengthAdjust *int         `json:"lengthAdjust,omitempty"`
	MinLength    *int         `json:"minLength,omitempty"`
	MaxLength    *int         `json:"maxLength,omitempty"`

	// delimiter
	Delimiter string `json:"delimiter,omitempty"`

	// fixed
	Size int `json:"size,omitempty"`

	// Hex pattern used to resynchronise after a gap or a framing error.
	Sync string `json:"sync,omitempty"`
}

type LengthField struct {
	Offset int    `json:"offset"`
	Size   int    `json:"size"`   // 1, 2 or 4
	Endian string `json:"endian"` // "le" | "be"
}

type MessageRule struct {
	ID         string           `json:"id"`
	Title      string           `json:"title,omitempty"`
	Direction  string           `json:"direction,omitempty"` // "c2s" | "s2c" | "any" | ""
	Match      []MatchCondition `json:"match"`
	Fields     []FieldRule      `json:"fields"`
	Hypothesis string           `json:"hypothesis,omitempty"`
}

type MatchCondition struct {
	Offset int    `json:"offset"`
	Hex    string `json:"hex"`
}

// FieldRule describes one field. Negative offsets count from the end of the message.
type FieldRule struct {
	Name   string `json:"name"`
	Offset int    `json:"offset"`
	Type   string `json:"type"` // u8 i8 u16 i16 u32 i32 bytes ascii array
	Endian string `json:"endian,omitempty"`

	// bytes/ascii: fixed length or the name of a previously parsed field.
	Length *Length `json:"length,omitempty"`

	// array: count field name, item size and item layout (offsets relative to the item).
	Count    string      `json:"count,omitempty"`
	ItemSize int         `json:"itemSize,omitempty"`
	Items    []FieldRule `json:"items,omitempty"`

	Scale    float64           `json:"scale,omitempty"`
	Unit     string            `json:"unit,omitempty"`
	Enum     map[string]string `json:"enum,omitempty"`
	Expect   *Expect           `json:"expect,omitempty"`
	Checksum string            `json:"checksum,omitempty"` // "sum8" | "xor8"

	Meaning    string `json:"meaning,omitempty"`
	Status     string `json:"status,omitempty"`
	Hypothesis string `json:"hypothesis,omitempty"`
}

// EffectiveMeaningStatus is what backs the meaning of a field: its status, else
// "hypothesis" when a meaning is claimed, else "structural". The engine, the
// report and the UI all show this value.
func EffectiveMeaningStatus(f *FieldRule) string {
	switch {
	case f.Status != "":
		return f.Status
	case f.Meaning != "":
		return MeaningHypothesis
	default:
		return MeaningStructural
	}
}

// Expect is checked against the raw (unscaled) value.
type Expect struct {
	Equals *float64  `json:"equals,omitempty"`
	Min    *float64  `json:"min,omitempty"`
	Max    *float64  `json:"max,omitempty"`
	OneOf  []float64 `json:"oneOf,omitempty"`
}

// Assertion ties fields of a rule to experiment conditions of a linked action.
type Assertion struct {
	ID         string  `json:"id"`
	Rule       string  `json:"rule"`
	ActionKind string  `json:"actionKind"`
	Checks     []Check `json:"checks"`
	Hypothesis string  `json:"hypothesis,omitempty"`
}

// Check compares a field with "params.<name>" or "observed.<name>" of the action.
type Check struct {
	Field  string `json:"field"`
	Equals string `json:"equals"`
}

// Length is either a fixed number of bytes or a reference to a previous field.
type Length struct {
	N   int
	Ref string
}

func (l Length) MarshalJSON() ([]byte, error) {
	if l.Ref != "" {
		return json.Marshal(l.Ref)
	}
	return []byte(strconv.Itoa(l.N)), nil
}

func (l *Length) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '"' {
		return json.Unmarshal(b, &l.Ref)
	}
	if err := json.Unmarshal(b, &l.N); err != nil {
		return errors.New("length: expected a number or a field name")
	}
	return nil
}

// Hash is the SHA-256 of the canonical JSON encoding of the spec.
// encoding/json writes struct fields in declaration order and map keys sorted,
// so equal specs always produce equal hashes.
func Hash(s *Spec) string {
	b, err := json.Marshal(s)
	if err != nil {
		// Spec contains only JSON-safe types; this cannot happen.
		panic(fmt.Sprintf("spec: marshal: %v", err))
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Decode parses raw JSON strictly: unknown fields and wrong types are errors.
func Decode(raw []byte) (*Spec, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var s Spec
	if err := dec.Decode(&s); err != nil {
		return nil, err
	}
	// More() misses a stray "}" or "]"; only EOF means nothing follows.
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("unexpected data after the JSON object")
	}
	s.Normalize()
	return &s, nil
}

// Normalize turns omitted lists into empty ones: a spec without rules or a rule
// without match/fields is valid (a warning), and the canonical form (and so the
// hash and the API) always carries arrays, never null.
func (s *Spec) Normalize() {
	if s.Messages == nil {
		s.Messages = []MessageRule{}
	}
	for i := range s.Messages {
		r := &s.Messages[i]
		if r.Match == nil {
			r.Match = []MatchCondition{}
		}
		if r.Fields == nil {
			r.Fields = []FieldRule{}
		}
	}
	for i := range s.Assertions {
		if s.Assertions[i].Checks == nil {
			s.Assertions[i].Checks = []Check{}
		}
	}
}
