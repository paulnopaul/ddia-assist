// Package web serves the tracker UI (docs/spec/06-web-ui.md). Pages are
// server-rendered; book text is never shown (UI-7).
package web

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/paulnopaul/ddia-assist/internal/ingest"
	"github.com/paulnopaul/ddia-assist/internal/store"
)

// Templates live in templates/*.html and are compiled into the binary.
// Every page is layout.html plus the page's own "content" block.
//
//go:embed templates/*.html
var templateFS embed.FS

var funcs = template.FuncMap{
	"pages":            func(words int) int { return int(math.Round(float64(words) / 400)) },
	"indent":           func(level int) int { return level * 16 },
	"round":            func(f float64) int { return int(math.Round(f)) },
	"settableStatuses": func() []string { return settable },
	"statusAction": func(s string) string {
		switch s {
		case store.StatusNotStarted:
			return "Mark unread"
		case store.StatusReading:
			return "Reading now"
		case store.StatusRead:
			return "Mark read"
		}
		return s
	},
	"statusLabel": func(s string) string {
		switch s {
		case store.StatusNotStarted:
			return "not started"
		case store.StatusReading:
			return "reading"
		case store.StatusRead:
			return "read"
		case store.StatusStudied:
			return "studied"
		}
		return s
	},
}

func page(name string) *template.Template {
	return template.Must(template.New("").Funcs(funcs).ParseFS(templateFS, "templates/layout.html", "templates/"+name+".html"))
}

// settable are the statuses you can set by hand (UI-3). "studied" is only
// set by a recorded study session.
var settable = []string{store.StatusNotStarted, store.StatusReading, store.StatusRead}

type server struct {
	st      *store.Store
	version string
	dataDir string
	pages   map[string]*template.Template
}

type view struct {
	Title   string
	Version string
	Flash   string
	Error   string
	Data    any
}

// Handler returns the UI. dataDir is where imports put extracted figures.
func Handler(st *store.Store, version, dataDir string) http.Handler {
	s := &server{st: st, version: version, dataDir: dataDir, pages: map[string]*template.Template{}}
	for _, name := range []string{"dashboard", "import", "plan", "unit", "concepts"} {
		s.pages[name] = page(name)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /{$}", s.dashboard)
	mux.HandleFunc("GET /import", s.importPage)
	mux.HandleFunc("POST /import", s.importPost)
	mux.HandleFunc("POST /book/delete", s.deleteBook)
	mux.HandleFunc("GET /plan", s.plan)
	mux.HandleFunc("GET /concepts", s.concepts)
	mux.HandleFunc("GET /units/{id}", s.unit)
	mux.HandleFunc("POST /units/{id}/status", s.unitStatus)
	mux.HandleFunc("POST /units/{id}/merge", s.unitMerge)
	mux.HandleFunc("POST /units/{id}/split", s.unitSplit)
	mux.HandleFunc("POST /units/{id}/rename", s.unitRename)
	s.routesM3(mux)
	return mux
}

func (s *server) render(w http.ResponseWriter, r *http.Request, name, title string, data any) {
	v := view{Title: title, Version: s.version, Data: data, Flash: r.URL.Query().Get("flash"), Error: r.URL.Query().Get("error")}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.pages[name].ExecuteTemplate(w, "layout", v); err != nil {
		slog.Error("render", "page", name, "err", err)
	}
}

func (s *server) fail(w http.ResponseWriter, err error) {
	slog.Error("request failed", "err", err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// back redirects to target with a flash or error message.
func back(w http.ResponseWriter, r *http.Request, target string, err error, flash string) {
	q := url.Values{}
	if err != nil {
		q.Set("error", err.Error())
	} else if flash != "" {
		q.Set("flash", flash)
	}
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func unitID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func (s *server) healthz(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DB.PingContext(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.Write([]byte("ok\n"))
}

func (s *server) importPage(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{}
	if b, err := s.st.CurrentBook(r.Context()); err == nil {
		data["Book"] = b
	}
	s.render(w, r, "import", "Import", data)
}

const maxUpload = 200 << 20

func (s *server) importPost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	f, _, err := r.FormFile("epub")
	if err != nil {
		back(w, r, "/import", fmt.Errorf("upload: %w", err), "")
		return
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		back(w, r, "/import", err, "")
		return
	}
	res, err := ingest.Import(r.Context(), s.st, s.dataDir, data)
	if err != nil {
		back(w, r, "/import", err, "")
		return
	}
	if res.Unchanged {
		back(w, r, "/plan", nil, "This book is already imported; nothing changed.")
		return
	}
	data2 := map[string]any{"Result": res}
	if b, err := s.st.CurrentBook(r.Context()); err == nil {
		data2["Book"] = b
	}
	s.render(w, r, "import", "Import", data2)
}

func (s *server) deleteBook(w http.ResponseWriter, r *http.Request) {
	b, err := s.st.CurrentBook(r.Context())
	if err == nil {
		err = s.st.DeleteBook(r.Context(), b.ID)
	}
	if errors.Is(err, store.ErrNotFound) {
		err = nil
	}
	back(w, r, "/import", err, "Book deleted.")
}

type chapterView struct {
	Number int
	Units  []store.Unit
}

func (s *server) plan(w http.ResponseWriter, r *http.Request) {
	units, err := s.st.Units(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	var chapters []chapterView
	for _, u := range units {
		if len(chapters) == 0 || chapters[len(chapters)-1].Number != u.Chapter {
			chapters = append(chapters, chapterView{Number: u.Chapter})
		}
		chapters[len(chapters)-1].Units = append(chapters[len(chapters)-1].Units, u)
	}
	scores, err := s.unitScores(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.render(w, r, "plan", "Reading plan", map[string]any{"Units": units, "Chapters": chapters, "Scores": scores})
}

func (s *server) unit(w http.ResponseWriter, r *http.Request) {
	id, err := unitID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	u, err := s.st.Unit(r.Context(), id, false)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		s.fail(w, err)
		return
	}
	data := map[string]any{"Unit": u}
	if err := s.unitExtras(r, id, data); err != nil {
		s.fail(w, err)
		return
	}
	s.render(w, r, "unit", u.Title, data)
}

func (s *server) unitStatus(w http.ResponseWriter, r *http.Request) {
	id, err := unitID(r)
	if err == nil {
		status := r.FormValue("status")
		if !slices.Contains(settable, status) {
			err = fmt.Errorf("status %q can't be set by hand", status)
		} else {
			err = s.st.SetStatus(r.Context(), id, status)
		}
	}
	flash := ""
	switch r.FormValue("status") {
	case store.StatusRead:
		flash = "Marked read. Open Claude Desktop and run /ddia-study."
	case store.StatusNotStarted:
		flash = "Marked unread. Claude can't see this unit until you mark it read again."
	}
	back(w, r, referer(r, fmt.Sprintf("/units/%d", id)), err, flash)
}

func (s *server) unitMerge(w http.ResponseWriter, r *http.Request) {
	id, err := unitID(r)
	if err == nil {
		err = s.st.MergeWithNext(r.Context(), id)
	}
	back(w, r, "/plan", err, "Units merged.")
}

func (s *server) unitSplit(w http.ResponseWriter, r *http.Request) {
	id, err := unitID(r)
	if err == nil {
		err = s.st.SplitAt(r.Context(), id, r.FormValue("section"))
	}
	back(w, r, "/plan", err, "Unit split.")
}

func (s *server) unitRename(w http.ResponseWriter, r *http.Request) {
	id, err := unitID(r)
	if err == nil {
		err = s.st.Rename(r.Context(), id, r.FormValue("title"))
	}
	back(w, r, fmt.Sprintf("/units/%d", id), err, "Renamed.")
}

// referer returns the same-site path the form came from, or fallback.
func referer(r *http.Request, fallback string) string {
	if u, err := url.Parse(r.Referer()); err == nil && u.Path != "" && (u.Host == "" || u.Host == r.Host) {
		return u.Path
	}
	return fallback
}
