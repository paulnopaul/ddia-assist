package web

import "net/http"

// Hooks filled in by later milestones (study sessions, scores, review queue).

func (s *server) routesM3(mux *http.ServeMux) {}

func (s *server) unitScores(r *http.Request) (map[int64]float64, error) { return nil, nil }

func (s *server) unitExtras(r *http.Request, unitID int64, data map[string]any) error { return nil }

func (s *server) dashboard(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/plan", http.StatusSeeOther)
}

func (s *server) concepts(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "concepts", "Concepts", nil)
}
