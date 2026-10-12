package spec

import (
	"os"
	"testing"
)

func FuzzParse(f *testing.F) {
	for _, name := range []string{"testdata/thermolink_v1.json", "testdata/thermolink_v3.json"} {
		if raw, err := os.ReadFile(name); err == nil {
			f.Add(raw)
		}
	}
	f.Add([]byte(`{"messages":[{"fields":[{"length":"x","items":[{}]}]}]}`))
	f.Add([]byte(`[`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		s, res := Parse(raw)
		if res.Issues == nil {
			t.Fatal("Issues is nil")
		}
		if s == nil && (res.Valid || len(res.Issues) != 1) {
			t.Fatalf("undecodable input must give exactly one error: %+v", res)
		}
		for _, is := range res.Issues {
			if is.Path == "" || is.Message == "" || (is.Severity != SeverityError && is.Severity != SeverityWarning) {
				t.Fatalf("malformed issue %+v", is)
			}
		}
	})
}
