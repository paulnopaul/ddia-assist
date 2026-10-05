// Package mcpserver builds the MCP server shared by the stdio and HTTP
// transports (docs/spec/03-mcp.md). Tools and prompts arrive from M2 on.
package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/paulnopaul/ddia-assist/internal/store"
)

func New(st *store.Store, version string) *mcp.Server {
	_ = st // used by the tools added in M2
	return mcp.NewServer(&mcp.Implementation{Name: "ddia", Version: version}, nil)
}
