package engine

import (
	"context"
	"testing"

	"github.com/AVGsync/protolab/internal/model"
)

// largeSession is a 64 MiB client stream of 1Mi ThermoLink frames of 64 bytes,
// delivered in 1460-byte segments, with a lost segment every ~10 MiB.
func largeSession(b *testing.B) SessionInput {
	b.Helper()
	const frameLen, frames = 64, 1 << 20
	data := make([]byte, 0, frameLen*frames)
	payload := make([]byte, frameLen-6)
	for i := range frames {
		payload[0] = byte(i)
		data = append(data, tlFrame(0x02, byte(i), payload...)...)
	}
	var segs []model.StreamSegment
	var gaps []model.StreamGap
	for off, n := 0, 0; off < len(data); off, n = off+1460, n+1 {
		length := min(1460, len(data)-off)
		if n%7000 == 6999 {
			gaps = append(gaps, model.StreamGap{Offset: off, Length: length, Reason: model.GapLost})
			continue
		}
		segs = append(segs, seg(off, length, n+1, n))
	}
	return newInput("big", StreamInput{Bytes: data, Gaps: gaps, Segments: segs}, StreamInput{})
}

const benchSpec = `{"format":"protolab/v1","protocol":"ThermoLink",
	"framing":{"kind":"length_prefixed","lengthField":{"offset":3,"size":2,"endian":"le"},"lengthAdjust":6,"maxLength":4096,"sync":"A5"},
	"messages":[{"id":"set","match":[{"offset":0,"hex":"A5"},{"offset":1,"hex":"02"}],"fields":[
		{"name":"magic","offset":0,"type":"u8","expect":{"equals":165}},
		{"name":"seq","offset":2,"type":"u8"},
		{"name":"length","offset":3,"type":"u16"},
		{"name":"param","offset":5,"type":"u8"},
		{"name":"value","offset":6,"type":"i16","scale":0.1,"unit":"°C"},
		{"name":"checksum","offset":-1,"type":"u8","checksum":"sum8"}
	]}]}`

func BenchmarkParseSession64MiB(b *testing.B) {
	s := mustSpec(b, benchSpec)
	in := largeSession(b)
	b.SetBytes(int64(len(in.Streams[0].Bytes)))
	b.ResetTimer()
	for range b.N {
		got := ParseSession(s, in)
		if len(got.Messages) < 1<<20-200 {
			b.Fatalf("parsed %d messages", len(got.Messages))
		}
	}
}

func BenchmarkVerify64MiB(b *testing.B) {
	s := mustSpec(b, benchSpec)
	in := largeSession(b)
	b.SetBytes(int64(len(in.Streams[0].Bytes)))
	b.ResetTimer()
	for range b.N {
		res, err := Verify(context.Background(), s, []SessionInput{in}, nil)
		if err != nil || res.Summary.Messages < 1<<20-200 {
			b.Fatalf("verify: %v, %+v", err, res.Summary)
		}
	}
}
