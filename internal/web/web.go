// Package web serves the tracker UI (docs/spec/06-web-ui.md). In M0 it only
// has a placeholder page and a health check.
package web

import (
	"embed"
	"html/template"
	"net/http"

	"github.com/paulnopaul/ddia-assist/internal/store"
)

// Templates live in templates/*.html and are compiled into the binary, so the
// image stays a single file. Each page is executed by its file name.
//
//go:embed templates/*.html
var templateFS embed.FS

var templates = template.Must(template.ParseFS(templateFS, "templates/*.html"))

func Handler(st *store.Store, version string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := st.DB.PingContext(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		v, err := st.SchemaVersion(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		templates.ExecuteTemplate(w, "index.html", struct {
			Version string
			Schema  int64
		}{version, v})
	})
	return mux
}
