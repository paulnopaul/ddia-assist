package mcpserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/paulnopaul/ddia-assist/internal/ingest"
	"github.com/paulnopaul/ddia-assist/internal/store"
	"github.com/paulnopaul/ddia-assist/internal/testbook"
)

type env struct {
	t  *testing.T
	st *store.Store
	cs *mcp.ClientSession
}

func setup(t *testing.T, withBook bool) *env {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dir, "ddia.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if withBook {
		if _, err := ingest.Import(ctx, st, dir, testbook.HTMLBook()); err != nil {
			t.Fatal(err)
		}
	}
	ct, stt := mcp.NewInMemoryTransports()
	ss, err := New(st, "test").Connect(ctx, stt, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return &env{t: t, st: st, cs: cs}
}

// call invokes a tool and returns its text content and whether it errored.
func (e *env) call(name string, args any) (string, bool) {
	e.t.Helper()
	res, err := e.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		e.t.Fatalf("%s: %v", name, err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String(), res.IsError
}

func (e *env) units() []store.Unit {
	us, err := e.st.Units(context.Background())
	if err != nil {
		e.t.Fatal(err)
	}
	return us
}

func TestServer_Handshake(t *testing.T) {
	e := setup(t, false)
	if got := e.cs.InitializeResult().ServerInfo.Name; got != "ddia" {
		t.Fatalf("server name = %q", got)
	}
	if _, isErr := e.call("get_status", map[string]any{}); !isErr {
		t.Fatal("get_status without a book should fail with a hint")
	}
}

func TestGetUnit_MCP2_RequiresRead(t *testing.T) {
	e := setup(t, true)
	u := e.units()[0]
	if out, isErr := e.call("get_unit", map[string]any{"unit_id": u.ID}); !isErr || !strings.Contains(out, "not marked read") {
		t.Fatalf("unread unit: isErr=%v out=%s", isErr, out)
	}
	if _, isErr := e.call("mark_read", map[string]any{"unit_id": u.ID}); isErr {
		t.Fatal("mark_read failed")
	}
	out, isErr := e.call("get_unit", map[string]any{})
	if isErr || !strings.Contains(out, "[ch01] Chapter 1. Storage") || !strings.Contains(out, "introword") {
		t.Fatalf("get_unit default: isErr=%v out=%.200s", isErr, out)
	}
}

func TestReadScope_MCP1(t *testing.T) {
	e := setup(t, true)
	units := e.units()
	first := units[0]
	e.call("mark_read", map[string]any{"unit_id": first.ID})

	// "btree" lives in a later unit: invisible to search, section and unit fetch.
	var later store.Unit
	for _, u := range units[1:] {
		for _, s := range u.Sections {
			if s.ID == "ch01.s02" {
				later = u
			}
		}
	}
	if later.ID == 0 {
		t.Fatal("test book layout changed: ch01.s02 not in a later unit")
	}
	if out, _ := e.call("search_book", map[string]any{"query": "btree"}); strings.Contains(out, "ch01.s02") {
		t.Fatalf("search leaked an unread section: %s", out)
	}
	if out, isErr := e.call("get_section", map[string]any{"section_id": "ch01.s02"}); !isErr || strings.Contains(out, "btree") {
		t.Fatalf("get_section leaked an unread section: %s", out)
	}
	out, _ := e.call("search_book", map[string]any{"query": "appendonly"})
	var res searchOut
	if err := json.Unmarshal([]byte(out), &res); err != nil || len(res.Hits) != 1 || res.Hits[0].SectionID != "ch01.s01" {
		t.Fatalf("search in scope: %s (%v)", out, err)
	}
	if _, isErr := e.call("search_book", map[string]any{"query": `"unbalanced (quote*`}); isErr {
		t.Fatal("punctuation in a query must not break FTS")
	}
	if out, isErr := e.call("get_section", map[string]any{"section_id": "ch01.refs"}); isErr || !strings.Contains(out, "A paper") {
		t.Fatalf("references of a read chapter should be readable: %s", out)
	}

	e.call("mark_read", map[string]any{"unit_id": later.ID})
	if out, _ := e.call("search_book", map[string]any{"query": "btree"}); !strings.Contains(out, "ch01.s02") {
		t.Fatalf("search after marking read: %s", out)
	}
}

func TestGetFigure(t *testing.T) {
	e := setup(t, true)
	if _, isErr := e.call("get_figure", map[string]any{"name": "fig1.png"}); !isErr {
		t.Fatal("figure of an unread unit must not be returned")
	}
	e.call("mark_read", map[string]any{"unit_id": e.units()[0].ID})
	res, err := e.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_figure", Arguments: map[string]any{"name": "fig1.png"}})
	if err != nil || res.IsError || len(res.Content) != 2 {
		t.Fatalf("get_figure: %v %+v", err, res)
	}
	if img, ok := res.Content[1].(*mcp.ImageContent); !ok || img.MIMEType != "image/png" || len(img.Data) == 0 {
		t.Fatalf("not an image: %+v", res.Content[1])
	}
}

func TestGetStatus(t *testing.T) {
	e := setup(t, true)
	e.call("mark_read", map[string]any{"unit_id": e.units()[0].ID})
	out, isErr := e.call("get_status", map[string]any{})
	var st statusOut
	if isErr || json.Unmarshal([]byte(out), &st) != nil || st.UnitsRead != 1 || st.NextToStudy == nil || st.NextToRead == nil {
		t.Fatalf("status: %s", out)
	}
}
