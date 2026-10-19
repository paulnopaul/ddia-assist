// Package mcpserver builds the MCP server shared by the stdio and HTTP
// transports (docs/spec/03-mcp.md). Every tool that returns book content
// enforces the read-scope rule (MCP-1) in the store, not in prompts.
package mcpserver

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/paulnopaul/ddia-assist/internal/store"
)

type server struct {
	st *store.Store
}

func New(st *store.Store, version string) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "ddia", Version: version}, &mcp.ServerOptions{
		Instructions: "Study helper for the books the user is reading. Tools work on the active book (see get_status) unless they name a unit. " +
			"Book content is only available for units the user has marked read; never guess book text you can't fetch.",
	})
	s := &server{st: st}
	s.addReadTools(srv)
	s.addLater(srv)
	return srv
}

// activeBook is the book that tools without a unit work on (MCP-9).
func (s *server) activeBook(ctx context.Context) (*store.Book, error) {
	b, err := s.st.ActiveBook(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errors.New("no book imported yet; ask the user to import an EPUB at http://localhost:8080/import")
	}
	return b, err
}
