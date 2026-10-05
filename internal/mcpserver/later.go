package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Hooks for the study loop (M3) and review queue (M4).

func (s *server) addLater(srv *mcp.Server) {}

func (s *server) statusExtras(ctx context.Context, out *statusOut) error { return nil }

func (s *server) unitExtras(ctx context.Context, unitID int64) (string, error) { return "", nil }
