// Package web serves the tracker UI (docs/spec/06-web-ui.md). In M0 it only
// has a placeholder page and a health check.
package web

import (
	"html/template"
	"net/http"

	"github.com/paulnopaul/ddia-assist/internal/store"
)

var index = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>ddia-assist</title></head>
<body><h1>ddia-assist</h1><p>Version {{.Version}}, schema {{.Schema}}. Nothing to track yet: EPUB import arrives in M1.</p></body></html>
`))

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
		index.Execute(w, struct {
			Version string
			Schema  int
		}{version, v})
	})
	return mux
}
