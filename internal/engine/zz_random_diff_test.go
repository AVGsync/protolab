package engine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/spec"
)

func TestZZRandomDiff(t *testing.T) {
	path := os.Getenv("PROTOLAB_TS_RAND")
	if path == "" {
		t.Skip()
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var ref struct {
		Cases []struct {
			Spec     json.RawMessage `json:"spec"`
			Sessions []struct {
				Session model.TcpSession    `json:"session"`
				Actions []model.ActionEntry `json:"actions"`
				Streams []struct {
					Bytes    string                `json:"bytes"`
					Gaps     []model.StreamGap     `json:"gaps"`
					Segments []model.StreamSegment `json:"segments"`
				} `json:"streams"`
			} `json:"sessions"`
			Parsed []struct {
				Messages json.RawMessage `json:"messages"`
				Unframed json.RawMessage `json:"unframed"`
			} `json:"parsed"`
			Verify json.RawMessage `json:"verify"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	fails := 0
	for ci, c := range ref.Cases {
		s, res := spec.Parse(c.Spec)
		if s == nil || !res.Valid {
			t.Fatalf("case %d spec rejected: %+v", ci, res.Issues)
		}
		var inputs []SessionInput
		for _, ss := range c.Sessions {
			in := SessionInput{Session: ss.Session, Actions: ss.Actions}
			for d := range 2 {
				b, _ := base64.StdEncoding.DecodeString(ss.Streams[d].Bytes)
				in.Streams[d] = StreamInput{Bytes: b, Gaps: ss.Streams[d].Gaps, Segments: ss.Streams[d].Segments}
			}
			inputs = append(inputs, in)
		}
		for si, in := range inputs {
			got := ParseSession(s, in)
			unframed := make([]map[string]any, len(got.Unframed))
			for i, u := range got.Unframed {
				unframed[i] = map[string]any{"direction": u.Direction, "offset": u.Offset, "length": u.Length}
			}
			before := t.Failed()
			assertSameJSON(t, fmt.Sprintf("case %d session %d messages", ci, si), got.Messages, c.Parsed[si].Messages)
			assertSameJSON(t, fmt.Sprintf("case %d session %d unframed", ci, si), unframed, c.Parsed[si].Unframed)
			if !before && t.Failed() {
				fails++
			}
		}
		got, err := Verify(context.Background(), s, inputs, nil)
		if err != nil {
			t.Fatal(err)
		}
		assertSameJSON(t, fmt.Sprintf("case %d verify", ci), map[string]any{"summary": got.Summary, "findings": got.Findings}, c.Verify)
		if t.Failed() {
			fails++
		}
		if fails > 10 {
			t.Fatalf("too many differences; spec: %s", c.Spec)
		}
	}
	t.Logf("compared %d cases", len(ref.Cases))
}
