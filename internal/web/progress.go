package web

import (
	"errors"
	"net/http"

	"github.com/paulnopaul/ddia-assist/internal/store"
)

// dashboard shows progress per chapter, due reviews and the weakest concepts (UI-6).
func (s *server) dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	book, err := s.st.CurrentBook(ctx)
	if errors.Is(err, store.ErrNotFound) {
		http.Redirect(w, r, "/import", http.StatusSeeOther)
		return
	} else if err != nil {
		s.fail(w, err)
		return
	}
	chapters, err := s.st.ChapterProgress(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	due, err := s.st.DueCount(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	weak, err := s.st.WeakConcepts(ctx, 5)
	if err != nil {
		s.fail(w, err)
		return
	}
	var total, read, studied int
	for _, c := range chapters {
		total += c.Units
		read += c.Read
		studied += c.Studied
	}
	var nextRead, nextStudy *store.Unit
	if u, err := s.st.NextUnit(ctx, store.StatusRead); err == nil {
		nextStudy = u
	}
	if u, err := s.st.NextUnit(ctx, store.StatusNotStarted, store.StatusReading); err == nil {
		nextRead = u
	}
	s.render(w, r, "dashboard", "Dashboard", map[string]any{
		"Book": book, "Chapters": chapters, "Due": due, "Weak": weak,
		"Total": total, "Read": read, "Studied": studied, "NextRead": nextRead, "NextStudy": nextStudy,
	})
}

type conceptRow struct {
	store.Concept
	Review *store.ReviewState
}

// concepts lists every concept in read scope with score and review state (UI-5).
func (s *server) concepts(w http.ResponseWriter, r *http.Request) {
	cs, err := s.st.ConceptsInScope(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	states, err := s.st.ReviewStates(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	rows := make([]conceptRow, 0, len(cs))
	for _, c := range cs {
		row := conceptRow{Concept: c}
		if st, ok := states[c.ID]; ok {
			row.Review = &st
		}
		rows = append(rows, row)
	}
	s.render(w, r, "concepts", "Concepts", map[string]any{"Concepts": rows})
}
