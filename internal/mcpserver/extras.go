package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// addLater registers the study (M3) and review (M4) tools.
func (s *server) addLater(srv *mcp.Server) {
	s.addStudy(srv)
	s.addReview(srv)
}

func (s *server) statusExtras(ctx context.Context, bookID int64, out *statusOut) error {
	cs, err := s.st.WeakConcepts(ctx, bookID, 5)
	if err != nil {
		return err
	}
	for _, c := range cs {
		out.Weakest = append(out.Weakest, weak{ConceptID: c.ID, Name: c.Name, Score: *c.Score})
	}
	due, err := s.st.DueCount(ctx, bookID)
	out.DueReviews = due
	return err
}

func (s *server) unitExtras(ctx context.Context, unitID int64) (string, error) {
	return s.conceptsSection(ctx, unitID)
}
