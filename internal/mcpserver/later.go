package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *server) addLater(srv *mcp.Server) {
	s.addStudy(srv)
}

func (s *server) statusExtras(ctx context.Context, out *statusOut) error {
	cs, err := s.st.WeakConcepts(ctx, 5)
	if err != nil {
		return err
	}
	for _, c := range cs {
		out.Weakest = append(out.Weakest, weak{ConceptID: c.ID, Name: c.Name, Score: *c.Score})
	}
	due, err := s.dueCount(ctx)
	out.DueReviews = due
	return err
}

func (s *server) unitExtras(ctx context.Context, unitID int64) (string, error) {
	return s.conceptsSection(ctx, unitID)
}

// dueCount is the number of due reviews; the review queue arrives in M4.
func (s *server) dueCount(ctx context.Context) (int, error) { return 0, nil }
