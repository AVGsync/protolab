package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/AVGsync/protolab/internal/app"
)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a, err := app.New(app.Config{DataDir: t.TempDir(), Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	static := fstest.MapFS{
		"index.html":      {Data: []byte("<!doctype html><title>ProtoLab</title>")},
		"assets/app-1.js": {Data: []byte("console.log(1)")},
		"favicon.svg":     {Data: []byte("<svg/>")},
	}
	return New(a, Options{Logger: logger, Static: static})
}

func do(h http.Handler, method, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Host = "127.0.0.1:7420"
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("not an error envelope: %s", rec.Body.String())
	}
	return body.Error.Code
}

func TestGuardRejectsForeignHostAndCrossSite(t *testing.T) {
	h := newTestServer(t)
	if rec := do(h, "GET", "/api/v1/projects", "", map[string]string{"Host": "evil.example:7420"}); rec.Code != 403 || errorCode(t, rec) != "forbidden_host" {
		t.Fatalf("DNS rebinding not blocked: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "POST", "/api/v1/projects", `{"name":"x"}`, map[string]string{"Origin": "https://evil.example"}); rec.Code != 403 {
		t.Fatalf("cross-site POST not blocked: %d", rec.Code)
	}
	if rec := do(h, "POST", "/api/v1/projects", `{"name":"x"}`, map[string]string{"Sec-Fetch-Site": "cross-site"}); rec.Code != 403 {
		t.Fatalf("Sec-Fetch-Site cross-site not blocked: %d", rec.Code)
	}
	// The Vite dev server proxies from another loopback port.
	if rec := do(h, "POST", "/api/v1/projects", `{"name":"dev"}`, map[string]string{"Origin": "http://localhost:5173"}); rec.Code != 201 {
		t.Fatalf("loopback origin rejected: %d %s", rec.Code, rec.Body)
	}
	for _, host := range []string{"localhost:7420", "[::1]:7420", "127.0.0.1"} {
		if rec := do(h, "GET", "/api/v1/projects", "", map[string]string{"Host": host}); rec.Code != 200 {
			t.Fatalf("host %s rejected: %d", host, rec.Code)
		}
	}
}

func TestProjectCRUDAndErrors(t *testing.T) {
	h := newTestServer(t)
	rec := do(h, "POST", "/api/v1/projects", `{"name":"  Стенд  ","targetPorts":[5020,5020,502]}`, nil)
	if rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var p struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Settings struct {
			TargetPorts []int `json:"targetPorts"`
		} `json:"settings"`
	}
	json.Unmarshal(rec.Body.Bytes(), &p)
	if p.Name != "Стенд" || len(p.Settings.TargetPorts) != 2 {
		t.Fatalf("project: %+v", p)
	}

	rec = do(h, "GET", "/api/v1/projects", "", nil)
	var list struct {
		Items []json.RawMessage `json:"items"`
		Total int               `json:"total"`
	}
	json.Unmarshal(rec.Body.Bytes(), &list)
	if list.Total != 1 || len(list.Items) != 1 {
		t.Fatalf("list: %s", rec.Body)
	}

	if rec := do(h, "PATCH", "/api/v1/projects/"+p.ID, `{"settings":{"overlapPolicy":"middle"}}`, nil); rec.Code != 422 || errorCode(t, rec) != "validation_error" {
		t.Fatalf("invalid patch: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "POST", "/api/v1/projects", `{"name":""}`, nil); rec.Code != 422 {
		t.Fatalf("empty name: %d", rec.Code)
	}
	if rec := do(h, "POST", "/api/v1/projects", `{bad json`, nil); rec.Code != 400 || errorCode(t, rec) != "bad_request" {
		t.Fatalf("bad json: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "GET", "/api/v1/projects/prj-missing", "", nil); rec.Code != 404 || errorCode(t, rec) != "not_found" {
		t.Fatalf("missing project: %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/v1/nope", "", nil); rec.Code != 404 || errorCode(t, rec) != "not_found" {
		t.Fatalf("unknown api path: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "GET", "/api/v1/jobs/job-x", "", nil); rec.Code != 404 {
		t.Fatalf("unknown job: %d", rec.Code)
	}
	if rec := do(h, "POST", "/api/v1/interpretations/validate", `{"spec":{"format":"other"}}`, nil); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"valid":false`) {
		t.Fatalf("validate: %d %s", rec.Code, rec.Body)
	}
}

func TestUploadRejectsNonCapture(t *testing.T) {
	h := newTestServer(t)
	rec := do(h, "POST", "/api/v1/projects", `{"name":"u"}`, nil)
	var p struct{ ID string }
	json.Unmarshal(rec.Body.Bytes(), &p)

	body := "--b\r\nContent-Disposition: form-data; name=\"file\"; filename=\"notes.txt\"\r\nContent-Type: text/plain\r\n\r\nhello\r\n--b--\r\n"
	req := httptest.NewRequest("POST", "/api/v1/projects/"+p.ID+"/captures", strings.NewReader(body))
	req.Host = "127.0.0.1:7420"
	req.Header.Set("Content-Type", "multipart/form-data; boundary=b")
	out := httptest.NewRecorder()
	h.ServeHTTP(out, req)
	if out.Code != 422 || errorCode(t, out) != "unsupported_format" {
		t.Fatalf("upload: %d %s", out.Code, out.Body)
	}
}

func TestStaticSPA(t *testing.T) {
	h := newTestServer(t)
	rec := do(h, "GET", "/projects/prj-1/sessions/x", "", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "ProtoLab") || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("SPA fallback: %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	rec = do(h, "GET", "/assets/app-1.js", "", nil)
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("asset: %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if rec := do(h, "GET", "/favicon.svg", "", nil); rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("unhashed file must revalidate: %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if rec := do(h, "GET", "/assets/missing.js", "", nil); rec.Code != 404 {
		t.Fatalf("missing asset must 404, got %d", rec.Code)
	}
	if rec := do(h, "GET", "/assets/", "", nil); strings.Contains(rec.Body.String(), "app-1.js") {
		t.Fatalf("directory listing of the embedded build: %s", rec.Body.String())
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("security headers missing: %v", rec.Header())
	}
}

// A page on another port of this host is "same-site", not "cross-site", and
// its loopback Origin passes the origin check: it must still not change data.
// The UI and the Vite dev proxy send "same-origin".
func TestGuardRejectsSameSitePages(t *testing.T) {
	h := newTestServer(t)
	rec := do(h, "POST", "/api/v1/projects", `{"name":"x"}`, map[string]string{"Sec-Fetch-Site": "same-site", "Origin": "http://127.0.0.1:8080"})
	if rec.Code != 403 || errorCode(t, rec) != "cross_site" {
		t.Fatalf("same-site POST from another local port: %d %s", rec.Code, rec.Body)
	}
	rec = do(h, "POST", "/api/v1/projects", `{"name":"dev"}`, map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "http://localhost:5173"})
	if rec.Code != 201 {
		t.Fatalf("Vite proxy request rejected: %d %s", rec.Code, rec.Body)
	}
}

// JSON endpoints accept only a JSON body: an HTML form (text/plain or form
// encodings) cannot produce one.
func TestJSONBodyRequiresJSONContentType(t *testing.T) {
	h := newTestServer(t)
	for _, ct := range []string{"text/plain", "application/x-www-form-urlencoded", "multipart/form-data; boundary=x", ""} {
		req := httptest.NewRequest("POST", "/api/v1/projects", strings.NewReader(`{"name":"form"}`))
		req.Host = "127.0.0.1:7420"
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 415 || errorCode(t, rec) != "unsupported_media_type" {
			t.Fatalf("Content-Type %q: %d %s", ct, rec.Code, rec.Body)
		}
	}
	if rec := do(h, "POST", "/api/v1/projects", `{"name":"ok"}`, map[string]string{"Content-Type": "application/json; charset=utf-8"}); rec.Code != 201 {
		t.Fatalf("JSON with charset: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "GET", "/api/v1/projects", "", nil); rec.Code != 200 {
		t.Fatalf("GET without a body: %d", rec.Code)
	}
}
