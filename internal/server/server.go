// Package server exposes the app over HTTP: the REST API under /api/v1 and
// the embedded single-page frontend everywhere else.
package server

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"github.com/AVGsync/protolab/internal/app"
	"github.com/AVGsync/protolab/internal/jobs"
)

// Options configure the HTTP layer.
type Options struct {
	Logger *slog.Logger
	// Static is the built frontend (nil or without index.html shows a hint page).
	Static fs.FS
	// AllowAnyHost disables the Host/Origin loopback checks. Only for an explicit
	// non-loopback listen address chosen by the user.
	AllowAnyHost bool
}

const maxJSONBody = 8 << 20

type server struct {
	app  *app.App
	log  *slog.Logger
	opts Options
}

// New returns the root handler.
func New(a *app.App, opts Options) http.Handler {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	s := &server{app: a, log: opts.Logger, opts: opts}
	mux := http.NewServeMux()
	s.routes(mux)
	mux.Handle("/", s.static())
	return s.recoverer(s.logRequests(s.guard(securityHeaders(mux))))
}

// ------------------------------------------------------------------ middleware

func (s *server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.log.Error("panic in handler", "path", r.URL.Path, "panic", v, "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, "internal", "Внутренняя ошибка сервера", nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (s *server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			return
		}
		level := slog.LevelDebug
		if r.Method != http.MethodGet || rec.status >= 400 {
			level = slog.LevelInfo
		}
		if rec.status >= 500 {
			level = slog.LevelError
		}
		s.log.Log(r.Context(), level, "http", "method", r.Method, "path", r.URL.Path, "status", rec.status, "ms", time.Since(start).Milliseconds())
	})
}

// isLoopbackHost accepts localhost names and loopback IPs (with or without port).
func isLoopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// guard protects the local server against DNS rebinding (foreign Host header)
// and cross-site requests from web pages the user visits (CSRF).
func (s *server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.opts.AllowAnyHost && !isLoopbackHost(r.Host) {
			writeError(w, http.StatusForbidden, "forbidden_host", "Доступ разрешён только через localhost", nil)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			// The UI (and the Vite proxy in dev) sends "same-origin". "same-site" is a
			// page on another port of this host: another local web app must not
			// change projects either, although its loopback Origin passes below.
			if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" || site == "same-site" {
				writeError(w, http.StatusForbidden, "cross_site", "Межсайтовые запросы запрещены", nil)
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" && !s.originAllowed(origin, r.Host) {
				writeError(w, http.StatusForbidden, "cross_site", "Межсайтовые запросы запрещены", nil)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// originAllowed: in loopback mode any loopback origin is fine (the Vite dev
// server proxies from another port); otherwise the origin must be this host.
func (s *server) originAllowed(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if s.opts.AllowAnyHost {
		return u.Host == host
	}
	return isLoopbackHost(u.Host)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

// ------------------------------------------------------------------ responses

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details any    `json:"details,omitempty"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string, details any) {
	var body errorBody
	body.Error.Code = code
	body.Error.Message = message
	body.Error.Details = details
	writeJSON(w, status, body)
}

// fail maps service errors to HTTP responses.
func (s *server) fail(w http.ResponseWriter, r *http.Request, err error) {
	if e, ok := app.AsError(err); ok {
		writeError(w, e.Status, e.Code, e.Message, e.Details)
		return
	}
	var maxErr *http.MaxBytesError
	switch {
	case errors.Is(err, jobs.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Задача не найдена", nil)
	case errors.Is(err, jobs.ErrNotCancellable):
		writeError(w, http.StatusConflict, "not_cancellable", "Задача уже завершена", nil)
	case errors.As(err, &maxErr):
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "Тело запроса слишком большое", nil)
	default:
		s.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
	}
}

// decode reads a JSON body into v. The body must be declared as JSON: an HTML
// form can only send form encodings and text/plain, so a cross-site form post
// is refused even by a browser that sends neither Origin nor Sec-Fetch-Site.
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || (mt != "application/json" && !strings.HasSuffix(mt, "+json")) {
		return &app.Error{Status: http.StatusUnsupportedMediaType, Code: "unsupported_media_type", Message: "Тело запроса должно быть JSON (Content-Type: application/json)"}
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return err
		}
		return &app.Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "Некорректный JSON: " + err.Error()}
	}
	return nil
}
