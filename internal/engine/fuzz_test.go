package engine

import (
	"context"
	"fmt"
	"testing"

	"github.com/AVGsync/protolab/internal/model"
)

// FuzzParseSession feeds arbitrary streams, gaps and framing parameters to the
// engine and checks the invariants every caller relies on.
func FuzzParseSession(f *testing.F) {
	f.Add([]byte{0xA5, 0x01, 0x01, 0x00, 0x00, 0xA7}, uint8(0), uint8(3), uint8(2), int16(6), uint16(0), uint8(0))
	f.Add([]byte{1, 2, 0x7E, 3, 4}, uint8(1), uint8(0), uint8(1), int16(0), uint16(2), uint8(3))
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7}, uint8(2), uint8(0), uint8(4), int16(-1), uint16(5), uint8(1))
	f.Fuzz(func(t *testing.T, data []byte, kind, lenOffset, lenSize uint8, adjust int16, gapAt uint16, gapLen uint8) {
		framing := [...]string{
			fmt.Sprintf(`{"kind":"length_prefixed","lengthField":{"offset":%d,"size":%d,"endian":"be"},"lengthAdjust":%d,"sync":"A5"}`, lenOffset%8, lenSize%5, adjust),
			`{"kind":"delimiter","delimiter":"7E"}`,
			fmt.Sprintf(`{"kind":"fixed","size":%d}`, lenSize%9),
		}[kind%3]
		s := mustSpec(t, `{"format":"protolab/v1","framing":`+framing+`,"messages":[
			{"id":"a","match":[{"offset":0,"hex":"A5"}],"fields":[
				{"name":"n","offset":1,"type":"u8"},
				{"name":"items","offset":2,"type":"array","count":"n","itemSize":2,"items":[
					{"name":"v","offset":0,"type":"i16","scale":0.1,"expect":{"min":0}},
					{"name":"tail","offset":-1,"type":"u8","enum":{"1":"x"}}
				]},
				{"name":"text","offset":2,"type":"ascii","length":"n"},
				{"name":"sum","offset":-1,"type":"u8","checksum":"sum8"}
			]},
			{"id":"b","direction":"s2c","match":[],"fields":[{"name":"w","offset":-4,"type":"u32","endian":"be"}]}
		],"assertions":[{"id":"x","rule":"a","actionKind":"k","checks":[{"field":"n","equals":"params.n"}]}]}`)

		gaps := []model.StreamGap{{Offset: int(gapAt) % (len(data) + 1), Length: int(gapLen % 16), Reason: model.GapLost}}
		var segs []model.StreamSegment
		for off := 0; off < len(data); off += 3 {
			segs = append(segs, seg(off, min(3, len(data)-off), off/3+1, (off*7)%11))
		}
		action := model.ActionEntry{Kind: "k", Label: "k", Params: map[string]any{"n": "2"}}
		in := newInput("f", StreamInput{Bytes: data, Gaps: gaps, Segments: segs}, StreamInput{Bytes: data, Segments: segs}, action)

		got := ParseSession(s, in)
		for i, m := range got.Messages {
			if m.Index != i || m.Issues == nil || m.Fields == nil || m.Unknown == nil || m.PacketNumbers == nil {
				t.Fatalf("message %d breaks the invariants: %+v", i, m)
			}
			if m.Offset < 0 || m.Length <= 0 || m.Offset+m.Length > len(data) || len(m.Data) != m.Length {
				t.Fatalf("message %d out of the stream: %+v", i, m)
			}
			if i > 0 && got.Messages[i-1].Timestamp > m.Timestamp {
				t.Fatalf("messages out of time order at %d", i)
			}
		}
		res, err := Verify(context.Background(), s, []SessionInput{in}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if res.Summary.Messages != len(got.Messages) {
			t.Fatalf("verify saw %d messages, parse %d", res.Summary.Messages, len(got.Messages))
		}
		for _, f := range res.Findings {
			if f.Kind == model.FindingCaptureDefect && f.Severity == model.LevelError {
				t.Fatalf("engine failure: %s", f.Message)
			}
		}
	})
}
