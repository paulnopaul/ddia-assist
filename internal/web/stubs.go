package web

import "net/http"

// The dashboard and concepts pages arrive with the review queue (M4).

func (s *server) dashboard(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/plan", http.StatusSeeOther)
}

func (s *server) concepts(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "concepts", "Concepts", nil)
}
