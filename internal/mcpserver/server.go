// Package mcpserver builds the MCP server shared by the stdio and HTTP
// transports (docs/spec/03-mcp.md). Every tool that returns book content
// enforces the read-scope rule (MCP-1) in the store, not in prompts.
package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/paulnopaul/ddia-assist/internal/store"
)

type server struct {
	st *store.Store
}

func New(st *store.Store, version string) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "ddia", Version: version}, &mcp.ServerOptions{
		Instructions: "Study helper for the book the user is reading. Book content is only available for units the user has marked read; never guess book text you can't fetch.",
	})
	s := &server{st: st}
	s.addReadTools(srv)
	s.addLater(srv)
	return srv
}
