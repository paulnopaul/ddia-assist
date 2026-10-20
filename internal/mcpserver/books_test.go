package mcpserver

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/paulnopaul/ddia-assist/internal/ingest"
	"github.com/paulnopaul/ddia-assist/internal/store"
	"github.com/paulnopaul/ddia-assist/internal/testbook"
)

// twoBooks imports "Test Book" (book 1) and "Second Book" (active), which share section IDs.
func twoBooks(t *testing.T) (*env, int64, int64) {
	t.Helper()
	e := setup(t, true)
	first := active(t, e.st)
	res, err := ingest.Import(context.Background(), e.st, t.TempDir(), testbook.HTMLBookTitled("Second Book"))
	if err != nil {
		t.Fatal(err)
	}
	return e, first, res.BookID
}

func (e *env) prompt(name string, args map[string]string) (string, error) {
	e.t.Helper()
	res, err := e.cs.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: name, Arguments: args})
	if err != nil {
		return "", err
	}
	return res.Messages[0].Content.(*mcp.TextContent).Text, nil
}

func TestTools_MCP9_UseActiveBookOrUnitsBook(t *testing.T) {
	e, first, _ := twoBooks(t)
	ctx := context.Background()
	firstUnits, _ := e.st.Units(ctx, first)
	// mark_read and get_unit name a unit, so they work on book 1 while book 2 is active.
	if _, isErr := e.call("mark_read", map[string]any{"unit_id": firstUnits[0].ID}); isErr {
		t.Fatal("mark_read on a unit of the other book failed")
	}
	if out, isErr := e.call("get_unit", map[string]any{"unit_id": firstUnits[0].ID}); isErr || !strings.Contains(out, "introword") {
		t.Fatalf("get_unit of book 1: %.200s", out)
	}
	// Everything else uses the active book, where nothing is read yet.
	if out, isErr := e.call("get_section", map[string]any{"section_id": "ch01.s01"}); !isErr {
		t.Fatalf("get_section read book 1 while book 2 is active: %s", out)
	}
	if out, _ := e.call("search_book", map[string]any{"query": "appendonly"}); strings.Contains(out, "ch01.s01") {
		t.Fatalf("search leaked book 1: %s", out)
	}
	out, _ := e.call("get_status", map[string]any{})
	var st statusOut
	if json.Unmarshal([]byte(out), &st) != nil || st.Book != "Second Book" || st.UnitsRead != 0 || len(st.OtherBooks) != 1 || st.OtherBooks[0] != "Test Book" {
		t.Fatalf("status: %s", out)
	}
	if err := e.st.SetActiveBook(ctx, first); err != nil {
		t.Fatal(err)
	}
	if out, isErr := e.call("get_section", map[string]any{"section_id": "ch01.s01"}); isErr || !strings.Contains(out, "appendonly") {
		t.Fatalf("get_section in book 1: %s", out)
	}
}

func TestPrompt_MCP10_BookArgSwitchesActiveBook(t *testing.T) {
	e, first, second := twoBooks(t)
	ctx := context.Background()
	txt, err := e.prompt("ddia-study", map[string]string{"book": " test BOOK "})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(txt, `"Test Book"`) || active(t, e.st) != first {
		t.Fatalf("book arg did not switch to book 1: %.120s", txt)
	}
	txt, err = e.prompt("ddia-review", map[string]string{"book": "second"})
	if err != nil || !strings.Contains(txt, `"Second Book"`) || active(t, e.st) != second {
		t.Fatalf("review book arg: %v %.120s", err, txt)
	}
	// MCP-6: a unit number from another book switches to that book.
	firstUnits, _ := e.st.Units(ctx, first)
	txt, err = e.prompt("ddia-study", map[string]string{"unit": "#" + strconv.FormatInt(firstUnits[1].ID, 10)})
	if err != nil || !strings.Contains(txt, `"Test Book"`) || active(t, e.st) != first {
		t.Fatalf("unit number from book 1: %v %.120s", err, txt)
	}
	// A unit title is looked up in the book the arg picks.
	if _, err := e.prompt("ddia-study", map[string]string{"book": "second", "unit": firstUnits[0].Title}); err != nil {
		t.Fatalf("title in book 2: %v", err)
	}
	// A book arg never fails: the closest title wins, a typo included, and a tie keeps the active book.
	if txt, err := e.prompt("ddia-study", map[string]string{"book": "secnd bok"}); err != nil || !strings.Contains(txt, `"Second Book"`) || active(t, e.st) != second {
		t.Errorf("typo: err = %v, prompt = %.120s", err, txt)
	}
	if txt, err := e.prompt("ddia-review", map[string]string{"book": "book"}); err != nil || strings.Contains(txt, "Before anything else") || active(t, e.st) != second {
		t.Errorf("tie should keep the active book: err = %v, active = %d", err, active(t, e.st))
	}
}

func TestReviewQueue_REV9_PerBook(t *testing.T) {
	e, first, second := twoBooks(t)
	ctx := context.Background()
	u := mustUnits(t, e.st, first)[0]
	e.st.SetStatus(ctx, u.ID, store.StatusRead)
	cs, err := e.st.SaveConcepts(ctx, u.ID, []store.ConceptIn{
		{Name: "Log", Definition: "d", SectionRef: "ch01.s01"},
		{Name: "Compaction", Definition: "d", SectionRef: "ch01.s01.s01"},
		{Name: "Framing", Definition: "d", SectionRef: "ch01"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var scores []store.ScoreIn
	for _, c := range cs {
		scores = append(scores, store.ScoreIn{ConceptID: c.ID, Score: 0})
	}
	if _, _, err := e.st.RecordAssessment(ctx, store.Assessment{UnitID: u.ID,
		Answers: []store.AnswerIn{{Step: "explain", Prompt: "p", Response: "r"}}, ConceptScores: scores}); err != nil {
		t.Fatal(err)
	}
	e.st.DB.Exec(`UPDATE review_item SET due_at = datetime('now', '-1 day')`)
	if n, _ := e.st.DueCount(ctx, first); n != 3 {
		t.Fatalf("book 1 due = %d, want 3", n)
	}
	if n, _ := e.st.DueCount(ctx, second); n != 0 {
		t.Fatalf("book 2 due = %d, want 0", n)
	}
	if out, _ := e.call("get_due_reviews", map[string]any{}); strings.Contains(out, "Compaction") {
		t.Fatalf("book 2 is active but got book 1's reviews: %s", out)
	}
	if out, _ := e.call("get_weak_concepts", map[string]any{}); strings.Contains(out, "Compaction") {
		t.Fatalf("book 2 is active but got book 1's weak concepts: %s", out)
	}
}

func mustUnits(t *testing.T, st *store.Store, bookID int64) []store.Unit {
	t.Helper()
	us, err := st.Units(context.Background(), bookID)
	if err != nil || len(us) == 0 {
		t.Fatalf("units: %v", err)
	}
	return us
}

func TestPrompt_MCP10_BookArgMatchesInitials(t *testing.T) {
	e := setup(t, true)
	ctx := context.Background()
	res, err := ingest.Import(ctx, e.st, t.TempDir(), testbook.HTMLBookTitled("Designing Data-Intensive Applications"))
	if err != nil {
		t.Fatal(err)
	}
	e.st.SetActiveBook(ctx, res.BookID-1)
	for _, arg := range []string{"ddia", "DDIA", "data-intensive"} {
		txt, err := e.prompt("ddia-review", map[string]string{"limit": "3", "book": arg})
		if err != nil || strings.Contains(txt, "Before anything else") || active(t, e.st) != res.BookID {
			t.Errorf("book=%q: err = %v, prompt = %.200s", arg, err, txt)
		}
		e.st.SetActiveBook(ctx, res.BookID-1)
	}
}

func TestTitleDistance_MCP10(t *testing.T) {
	const ddia = "Designing Data-Intensive Applications"
	for _, c := range []struct {
		title, arg string
		want       int
	}{
		{ddia, "ddia", 0},
		{ddia, "DDIA", 0},
		{ddia, "data-intensive", 0},
		{ddia, "Intensive Applications", 0},
		{ddia, "dda", 1},
		{ddia, "desinging", 2},
		{"Database Internals", "di", 0},
	} {
		if got := titleDistance(c.title, c.arg); got != c.want {
			t.Errorf("titleDistance(%q, %q) = %d, want %d", c.title, c.arg, got, c.want)
		}
	}
	if titleDistance(ddia, "ddia") >= titleDistance("Database Internals", "ddia") {
		t.Error("ddia should be closer to DDIA than to Database Internals")
	}
}
