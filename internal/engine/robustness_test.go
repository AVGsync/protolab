package engine

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/AVGsync/protolab/internal/model"
)

func TestUnsortedAndOverlappingSegments(t *testing.T) {
	s := mustSpec(t, `{"format":"protolab/v1","framing":{"kind":"fixed","size":5},"messages":[]}`)
	data := make([]byte, 20)
	in := newInput("s", StreamInput{Bytes: data, Segments: []model.StreamSegment{
		seg(10, 10, 3, 30),
		seg(0, 12, 1, 10), // overlaps the first one
		seg(5, 3, 2, 20),  // nested inside the second one
	}}, StreamInput{})
	got := ParseSession(s, in).Messages
	type brief struct {
		offset  int
		ts      string
		packets []int
	}
	want := []brief{
		{0, at(10), []int{1}},
		{5, at(10), []int{1, 2}}, // the earliest listed segment holding byte 5
		{10, at(30), []int{1, 3}},
		{15, at(30), []int{3}},
	}
	for i, m := range got {
		if b := (brief{m.Offset, m.Timestamp, m.PacketNumbers}); !reflect.DeepEqual(b, want[i]) {
			t.Errorf("message %d = %+v, want %+v", i, b, want[i])
		}
	}
}

// Specs are user input: absurd numbers must yield violations, never panics
// or runaway allocations.
func TestAdversarialSpecValues(t *testing.T) {
	s := mustSpec(t, `{"format":"protolab/v1",
		"framing":{"kind":"length_prefixed","lengthField":{"offset":0,"size":4,"endian":"le"},"lengthAdjust":9223372036854775000,"maxLength":9223372036854775807},
		"messages":[{"id":"m","match":[],"fields":[
			{"name":"far","offset":9223372036854775807,"type":"u8"},
			{"name":"before","offset":-9223372036854775808,"type":"u8"},
			{"name":"n","offset":0,"type":"u32"},
			{"name":"arr","offset":0,"type":"array","count":"n","itemSize":9223372036854775807,"items":[]},
			{"name":"neg","offset":0,"type":"array","count":"n","itemSize":-9223372036854775807,"items":[]},
			{"name":"blob","offset":0,"type":"bytes","length":9223372036854775807}
		]}]}`)
	data := []byte{0xFF, 0xFF, 0xFF, 0xFF, 1, 2, 3}
	in := newInput("s", StreamInput{
		Bytes:    data,
		Gaps:     []model.StreamGap{{Offset: -5, Length: 6}, {Offset: 1000, Length: 5}, {Offset: 3, Length: -2}},
		Segments: []model.StreamSegment{seg(0, -3, 1, 0), seg(2, 100, 2, 1)},
	}, StreamInput{})
	// The first gap is clamped to byte 0. The next header gives a length that
	// saturates at maxLength instead of wrapping around, so the message is
	// cut short by the end of the stream.
	got := ParseSession(s, in)
	if len(got.Unframed) != 0 || len(got.Messages) != 1 {
		t.Fatalf("parsed = %+v", got)
	}
	m := got.Messages[0]
	wantIssues := []string{"Ожидалось 9223372036854775807 Б, в потоке 6 Б"}
	if m.Offset != 1 || m.Status != model.MsgPartial || !reflect.DeepEqual(m.Issues, wantIssues) || !reflect.DeepEqual(m.PacketNumbers, []int{2}) {
		t.Errorf("message = %+v", m)
	}

	// The same fields on a fixed frame reach the field parser.
	s.Framing.Kind, s.Framing.Size = "fixed", len(data)
	m = ParseSession(s, newInput("s", oneSegment(data, 1, 0), StreamInput{})).Messages[0]
	for _, name := range []string{"far", "before", "arr", "neg", "blob"} {
		if f := mustField(t, m, name); f.Status != model.FieldViolation || f.Issue != issueOutOfBounds {
			t.Errorf("%s = %+v", name, f)
		}
	}
	res, err := Verify(context.Background(), s, []SessionInput{newInput("s", oneSegment(data, 1, 0), StreamInput{})}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Findings {
		for _, ex := range f.Examples {
			if ex.Offset < 0 || ex.Offset+ex.Length > len(data) {
				t.Errorf("finding %q points outside the stream: %+v", f.Message, ex)
			}
		}
	}
}

func TestVerifyAmbiguousAndUnknownHead(t *testing.T) {
	s := mustSpec(t, `{"format":"protolab/v1","framing":{"kind":"fixed","size":2},"messages":[
		{"id":"a","match":[{"offset":0,"hex":"A5"}],"fields":[]},
		{"id":"b","match":[{"offset":0,"hex":"A5"}],"fields":[]}
	]}`)
	data := []byte{0xA5, 1, 0, 7, 0x10, 0x20}
	in := newInput("s", StreamInput{
		Bytes:    data,
		Gaps:     []model.StreamGap{{Offset: 2, Length: 1, Reason: model.GapLost}},
		Segments: []model.StreamSegment{seg(0, 2, 1, 0), seg(3, 3, 2, 1)},
	}, StreamInput{})
	res, err := Verify(context.Background(), s, []SessionInput{in}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range res.Findings {
		if f.Kind == model.FindingAmbiguous || f.Kind == model.FindingUnknownType {
			got = append(got, f.Message)
		}
	}
	want := []string{
		"Подходят несколько правил: a, b",
		"C→S: сообщения «?? 07 …» не описаны ни одним правилом",
		"C→S: сообщения «10 20 …» не описаны ни одним правилом",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings = %q, want %q", got, want)
	}
	if res.Summary.Ambiguous != 1 || res.Summary.UnknownType != 2 || res.Summary.SessionsWithErrors != 1 {
		t.Errorf("summary = %+v", res.Summary)
	}
}

func TestToNumberTypes(t *testing.T) {
	for _, v := range []any{int8(5), int16(5), int32(5), int64(5), uint(5), uint8(5), uint16(5), uint32(5), uint64(5), float32(5), json.Number("5")} {
		if got, ok := toNumber(v); !ok || got != 5 {
			t.Errorf("toNumber(%T) = %v, %v", v, got, ok)
		}
	}
	if _, ok := toNumber(json.Number("x")); ok {
		t.Error("toNumber accepted an invalid json.Number")
	}
}
