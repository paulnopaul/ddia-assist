package web

import (
	"fmt"
	"net/http"
	"strconv"
)

func (s *server) routesM3(mux *http.ServeMux) {
	mux.HandleFunc("POST /scores/{id}/override", s.overrideScore)
}

func (s *server) unitScores(r *http.Request) (map[int64]float64, error) {
	return s.st.UnitScores(r.Context())
}

// unitExtras adds the unit's concepts and recorded sessions (UI-4).
func (s *server) unitExtras(r *http.Request, unitID int64, data map[string]any) error {
	cs, err := s.st.Concepts(r.Context(), unitID)
	if err != nil {
		return err
	}
	sessions, err := s.st.Sessions(r.Context(), unitID)
	if err != nil {
		return err
	}
	data["Concepts"] = cs
	data["Sessions"] = sessions
	return nil
}

func (s *server) overrideScore(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err == nil {
		var score int
		score, err = strconv.Atoi(r.FormValue("score"))
		if err == nil {
			err = s.st.OverrideScore(r.Context(), id, score)
		}
	}
	back(w, r, referer(r, "/plan"), err, fmt.Sprintf("Score set to %s.", r.FormValue("score")))
}
