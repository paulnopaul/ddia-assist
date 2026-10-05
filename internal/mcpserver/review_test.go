package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestReviewQueue_REV1_to_REV5(t *testing.T) {
	e, u, cs := studyReady(t)
	// scores: cs[0]=0, cs[1]=1, cs[2]=2 → all three enter the queue (REV-1).
	if out, isErr := e.call("record_assessment", assessment(u, cs)); isErr {
		t.Fatal(out)
	}
	ctx := context.Background()
	states, _ := e.st.ReviewStates(ctx)
	for i, want := range []int{1, 2, 7} { // REV-2
		if got := states[cs[i].ID].Interval; got != want {
			t.Errorf("concept %d interval = %d, want %d", i, got, want)
		}
	}
	// Make everything due now.
	e.st.DB.Exec(`UPDATE review_item SET due_at = datetime('now', '-1 minute')`)
	out, _ := e.call("get_due_reviews", map[string]any{"limit": 10})
	var due dueOut
	if err := json.Unmarshal([]byte(out), &due); err != nil || len(due.Due) != 3 {
		t.Fatalf("due: %s", out)
	}
	if due.Due[0].LastGap == "" && due.Due[1].LastGap == "" && due.Due[2].LastGap == "" {
		t.Error("last gap not surfaced")
	}

	review := func(prompt string, score int) (string, bool) {
		return e.call("record_review", map[string]any{"concept_id": cs[2].ID, "prompt": prompt, "response": "answer", "score": score, "task_shape": "predict_outcome"})
	}
	// REV-3: a 3 doubles the interval (7 → 14) and bumps the streak.
	out, isErr := review("Q1", 3)
	var r reviewOut
	if json.Unmarshal([]byte(out), &r); isErr || r.IntervalDays != 14 || r.Streak != 1 || r.Mastered {
		t.Fatalf("first review: %s", out)
	}
	// REV-5: same question twice is rejected.
	if out, isErr := review("Q1", 3); !isErr || !strings.Contains(out, "different question") {
		t.Fatalf("repeat question accepted: %s", out)
	}
	// REV-4: streak 2 → mastered, leaves the queue.
	out, _ = review("Q2", 3)
	if json.Unmarshal([]byte(out), &r); !r.Mastered {
		t.Fatalf("second 3 should master: %s", out)
	}
	if n, _ := e.st.DueCount(ctx); n != 2 {
		t.Fatalf("due after mastering = %d, want 2", n)
	}
	// A low review score resets per REV-2.
	out, _ = e.call("record_review", map[string]any{"concept_id": cs[0].ID, "prompt": "Q", "response": "?", "score": 1})
	if json.Unmarshal([]byte(out), &r); r.IntervalDays != 2 || r.Streak != 0 {
		t.Fatalf("reset: %s", out)
	}
}

func TestPrompt_MCP7_DDIAReview(t *testing.T) {
	e := setup(t, false)
	res, err := e.cs.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: "ddia-review", Arguments: map[string]string{"limit": "3"}})
	if err != nil {
		t.Fatal(err)
	}
	if txt := res.Messages[0].Content.(*mcp.TextContent).Text; !strings.Contains(txt, "limit 3") || !strings.Contains(txt, "record_review") {
		t.Fatalf("prompt: %s", txt)
	}
}
