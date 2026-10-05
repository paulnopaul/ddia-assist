package mcpserver

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/paulnopaul/ddia-assist/internal/store"
)

func TestServer_Handshake(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "ddia.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ct, stt := mcp.NewInMemoryTransports()
	ss, err := New(st, "test").Connect(ctx, stt, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	if got := cs.InitializeResult().ServerInfo.Name; got != "ddia" {
		t.Fatalf("server name = %q, want ddia", got)
	}
}
