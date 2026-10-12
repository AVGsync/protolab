package engine

import (
	"os"
	"testing"
	"time"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/spec"
)

// mustSpec decodes a spec without validating it, so tests can also feed
// specs that validation would reject.
func mustSpec(t testing.TB, js string) *spec.Spec {
	t.Helper()
	s, err := spec.Decode([]byte(js))
	if err != nil {
		t.Fatalf("decode spec: %v\n%s", err, js)
	}
	return s
}

// thermoLink is the v3 interpretation of the demo protocol:
// A5 | type | seq | len u16le | payload | sum8.
func thermoLink(t testing.TB) *spec.Spec {
	t.Helper()
	raw, err := os.ReadFile("testdata/thermolink_v3.json")
	if err != nil {
		t.Fatal(err)
	}
	s, res := spec.Parse(raw)
	if s == nil || !res.Valid {
		t.Fatalf("thermolink spec: %+v", res.Issues)
	}
	return s
}

// tlFrame builds a ThermoLink frame with a correct checksum.
func tlFrame(typ, seq byte, payload ...byte) []byte {
	b := []byte{0xA5, typ, seq, byte(len(payload)), byte(len(payload) >> 8)}
	b = append(b, payload...)
	var sum byte
	for _, c := range b {
		sum += c
	}
	return append(b, sum)
}

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

var baseTime = time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)

// at returns the capture timestamp ms milliseconds after baseTime.
func at(ms int) string { return model.PacketTime(baseTime.Add(time.Duration(ms) * time.Millisecond)) }

func seg(offset, length, packet, ms int) model.StreamSegment {
	return model.StreamSegment{Offset: offset, Length: length, PacketNumber: packet, Timestamp: at(ms), Flags: []string{}}
}

// oneSegment covers a whole stream with a single packet.
func oneSegment(data []byte, packet, ms int) StreamInput {
	return StreamInput{Bytes: data, Segments: []model.StreamSegment{seg(0, len(data), packet, ms)}}
}

func newInput(id string, c2s, s2c StreamInput, actions ...model.ActionEntry) SessionInput {
	return SessionInput{
		Session: model.TcpSession{
			ID:        id,
			CaptureID: "cap-1",
			Capture:   model.CaptureRef{FileName: "test.pcapng", SHA256: "sha-1"},
		},
		Streams: [2]StreamInput{c2s, s2c},
		Actions: actions,
	}
}

func fieldByName(fields []model.ParsedField, name string) *model.ParsedField {
	for i := range fields {
		if fields[i].Name == name {
			return &fields[i]
		}
	}
	return nil
}

func mustField(t testing.TB, m model.Message, name string) model.ParsedField {
	t.Helper()
	f := fieldByName(m.Fields, name)
	if f == nil {
		t.Fatalf("message %d has no field %q (rule %v)", m.Index, name, m.RuleID)
	}
	return *f
}
