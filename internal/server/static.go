package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

const notBuiltPage = `<!doctype html><html lang="ru"><meta charset="utf-8"><title>ProtoLab</title>
<body style="font:14px/1.5 system-ui;background:#0e1110;color:#e2e6e0;padding:48px">
<h1 style="font-size:18px">Веб-интерфейс не собран</h1>
<p>Бинарь собран без фронтенда. Выполните <code>make web</code> (или <code>npm run build</code> в каталоге frontend) и пересоберите программу.</p>
<p>API доступно по адресу <code>/api/v1</code>.</p></body></html>`

// static serves the embedded single-page app. Unknown paths fall back to
// index.html so client-side routes survive a reload.
func (s *server) static() http.Handler {
	if s.opts.Static == nil {
		return notBuilt()
	}
	if _, err := fs.Stat(s.opts.Static, "index.html"); err != nil {
		return notBuilt()
	}
	files := http.FileServerFS(s.opts.Static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || name == "index.html" {
			s.serveIndex(w, r)
			return
		}
		info, err := fs.Stat(s.opts.Static, name)
		if err != nil {
			if strings.HasPrefix(name, "assets/") {
				http.NotFound(w, r) // a missing hashed asset must not turn into HTML
				return
			}
			s.serveIndex(w, r)
			return
		}
		if info.IsDir() {
			s.serveIndex(w, r) // no directory listings of the embedded build
			return
		}
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}

func (s *server) serveIndex(w http.ResponseWriter, r *http.Request) {
	data, err := fs.ReadFile(s.opts.Static, "index.html")
	if err != nil {
		http.Error(w, "index.html missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

func notBuilt() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(notBuiltPage))
	})
}
