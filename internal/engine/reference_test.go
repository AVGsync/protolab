package engine

import (
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/spec"
)

// testdata/reference.json.gz holds the output of the TypeScript reference
// engine (frontend/src/mocks/engine) on the mock ThermoLink corpus: every
// session parsed with interpretations v1–v3 and every corpus verified. Inputs
// and outputs are both stored, so the test does not depend on the frontend.
//
// The reference was run with the engine's deliberate deviations applied: a
// checksum is not verified over gap bytes (the reference sums the zero fill
// and reports a false violation in a partial message), unknown bytes never
// match rules or delimiters, and packet numbers of examples are sorted.
type reference struct {
	Specs    map[string]json.RawMessage `json:"specs"`
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
		Spec      string `json:"spec"`
		SessionID string `json:"sessionId"`
		Result    struct {
			Messages json.RawMessage `json:"messages"`
			Unframed json.RawMessage `json:"unframed"`
		} `json:"result"`
	} `json:"parsed"`
	Corpora []struct {
		ID         string   `json:"id"`
		SessionIDs []string `json:"sessionIds"`
	} `json:"corpora"`
	Runs []struct {
		Spec       string          `json:"spec"`
		Corpus     string          `json:"corpus"`
		SessionIDs []string        `json:"sessionIds"`
		Result     json.RawMessage `json:"result"`
	} `json:"runs"`
}

func loadReference(t *testing.T) reference {
	t.Helper()
	f, err := os.Open("testdata/reference.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var ref reference
	if err := json.NewDecoder(zr).Decode(&ref); err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestMatchesReferenceImplementation(t *testing.T) {
	ref := loadReference(t)

	specs := map[string]*spec.Spec{}
	for name, b := range ref.Specs {
		s, res := spec.Parse(b)
		if s == nil || !res.Valid {
			t.Fatalf("spec %s: %+v", name, res.Issues)
		}
		specs[name] = s
	}
	inputs := map[string]SessionInput{}
	for _, s := range ref.Sessions {
		in := SessionInput{Session: s.Session, Actions: s.Actions}
		for d := range 2 {
			b, err := base64.StdEncoding.DecodeString(s.Streams[d].Bytes)
			if err != nil {
				t.Fatal(err)
			}
			in.Streams[d] = StreamInput{Bytes: b, Gaps: s.Streams[d].Gaps, Segments: s.Streams[d].Segments}
		}
		inputs[s.Session.ID] = in
	}
	if len(ref.Parsed) == 0 || len(ref.Runs) == 0 {
		t.Fatal("empty reference")
	}

	for _, p := range ref.Parsed {
		got := ParseSession(specs[p.Spec], inputs[p.SessionID])
		unframed := make([]map[string]any, len(got.Unframed))
		for i, u := range got.Unframed {
			unframed[i] = map[string]any{"direction": u.Direction, "offset": u.Offset, "length": u.Length}
		}
		name := p.Spec + "/" + p.SessionID
		assertSameJSON(t, name+" messages", got.Messages, p.Result.Messages)
		assertSameJSON(t, name+" unframed", unframed, p.Result.Unframed)
	}

	for _, r := range ref.Runs {
		ids := r.SessionIDs
		for _, c := range ref.Corpora {
			if c.ID == r.Corpus {
				ids = c.SessionIDs
			}
		}
		var in []SessionInput
		for _, id := range ids {
			in = append(in, inputs[id])
		}
		got, err := Verify(context.Background(), specs[r.Spec], in, nil)
		if err != nil {
			t.Fatal(err)
		}
		result := map[string]any{"summary": got.Summary, "findings": got.Findings}
		assertSameJSON(t, r.Spec+"/"+r.Corpus+" verify", result, r.Result)
	}
}

// assertSameJSON compares the JSON encoding of got with want, ignoring what
// the two encoders legitimately disagree on: empty children arrays (omitted
// by encoding/json), empty finding ids and the order of packet numbers.
func assertSameJSON(t *testing.T, name string, got any, want json.RawMessage) {
	t.Helper()
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var g, w any
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &w); err != nil {
		t.Fatal(err)
	}
	if path, ok := firstDiff("$", normalizeJSON(g), normalizeJSON(w)); !ok {
		t.Errorf("%s: %s", name, path)
	}
}

func normalizeJSON(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if arr, ok := val.([]any); ok && k == "children" && len(arr) == 0 {
				delete(x, k)
				continue
			}
			if (k == "id" || k == "runId") && val == "" {
				delete(x, k)
				continue
			}
			if arr, ok := val.([]any); ok && k == "packetNumbers" {
				sort.Slice(arr, func(i, j int) bool { return arr[i].(float64) < arr[j].(float64) })
			}
			x[k] = normalizeJSON(val)
		}
	case []any:
		for i := range x {
			x[i] = normalizeJSON(x[i])
		}
	}
	return v
}

// firstDiff reports the path of the first difference between two decoded
// JSON values, visiting object keys in sorted order.
func firstDiff(path string, got, want any) (string, bool) {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return fmt.Sprintf("%s: got %s, want an object", path, brief(got)), false
		}
		keys := make([]string, 0, len(w)+len(g))
		for k := range w {
			keys = append(keys, k)
		}
		for k := range g {
			if _, dup := w[k]; !dup {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			if p, ok := firstDiff(path+"."+k, g[k], w[k]); !ok {
				return p, false
			}
		}
		return "", true
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return fmt.Sprintf("%s: got %s, want %s", path, brief(got), brief(want)), false
		}
		for i := range w {
			if p, ok := firstDiff(fmt.Sprintf("%s[%d]", path, i), g[i], w[i]); !ok {
				return p, false
			}
		}
		return "", true
	}
	if !reflect.DeepEqual(got, want) {
		return fmt.Sprintf("%s: got %s, want %s", path, brief(got), brief(want)), false
	}
	return "", true
}

func brief(v any) string {
	b, _ := json.Marshal(v)
	if len(b) > 300 {
		return string(b[:300]) + "..."
	}
	return string(b)
}
