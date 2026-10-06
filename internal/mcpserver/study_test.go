package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/paulnopaul/ddia-assist/internal/store"
)

// studyReady marks the first unit read and saves three concepts for it.
func studyReady(t *testing.T) (*env, store.Unit, []store.Concept) {
	e := setup(t, true)
	u := e.units()[0]
	e.call("mark_read", map[string]any{"unit_id": u.ID})
	out, isErr := e.call("save_concepts", map[string]any{"unit_id": u.ID, "concepts": []map[string]any{
		{"name": "Append-only log", "definition": "Writes only go to the end.", "section_ref": "ch01.s01"},
		{"name": "Compaction", "definition": "Throwing away overwritten keys.", "section_ref": "ch01.s01.s01"},
		{"name": "Chapter framing", "definition": "Why storage matters.", "section_ref": "ch01"},
	}})
	if isErr {
		t.Fatalf("save_concepts: %s", out)
	}
	var co conceptsOut
	if err := json.Unmarshal([]byte(out), &co); err != nil || len(co.Concepts) != 3 {
		t.Fatalf("save_concepts out: %s", out)
	}
	return e, u, co.Concepts
}

func assessment(u store.Unit, cs []store.Concept) map[string]any {
	scores := []map[string]any{}
	for i, c := range cs {
		scores = append(scores, map[string]any{"concept_id": c.ID, "score": i % 4})
	}
	return map[string]any{
		"unit_id": u.ID,
		"answers": []map[string]any{
			{"step": "explain", "prompt": "Explain the unit", "response": "Logs are appended…"},
			{"step": "probe", "prompt": "Why compact?", "response": "To save space", "score": 2, "feedback": "Yes."},
			{"step": "apply", "task_shape": "predict_outcome", "prompt": "What happens if…", "response": "It grows", "score": 1},
		},
		"concept_scores": scores,
		"gaps":           []map[string]any{{"concept_id": cs[0].ID, "description": "Missed tombstones", "section_ref": "ch01.s01.s01"}},
	}
}

func TestSaveConcepts_OnlyOnce(t *testing.T) {
	e, u, _ := studyReady(t)
	out, isErr := e.call("save_concepts", map[string]any{"unit_id": u.ID, "concepts": []map[string]any{{"name": "x", "definition": "y", "section_ref": "ch01"}}})
	if !isErr || !strings.Contains(out, "already has") {
		t.Fatalf("second save_concepts: %s", out)
	}
	if out, _ := e.call("get_unit", map[string]any{"unit_id": u.ID}); !strings.Contains(out, "Append-only log") {
		t.Fatal("get_unit should list known concepts")
	}
}

func TestRecordAssessment_MCP3_MCP4_Validation(t *testing.T) {
	e, u, cs := studyReady(t)
	bad := assessment(u, cs)
	bad["answers"] = []map[string]any{{"step": "probe", "prompt": "q", "response": "a", "score": 7}}
	bad["concept_scores"] = []map[string]any{{"concept_id": cs[0].ID, "score": 1}}
	bad["gaps"] = []map[string]any{{"description": "x", "section_ref": "ch01.s02"}, {"description": "y", "section_ref": ""}}
	out, isErr := e.call("record_assessment", bad)
	if !isErr {
		t.Fatal("invalid assessment accepted")
	}
	for _, want := range []string{"fix and retry", "explain answer is required", "score must be 0-3", "needs a score (SES-10)", "not a section the user has read", "section_ref is required"} {
		if !strings.Contains(out, want) {
			t.Errorf("error missing %q: %s", want, out)
		}
	}
	if got, _ := e.st.Unit(context.Background(), u.ID, false); got.Status != store.StatusRead {
		t.Fatalf("failed assessment changed status to %s", got.Status)
	}
}

func TestRecordAssessment_SES12_MarksStudied_MCP5_Idempotent(t *testing.T) {
	e, u, cs := studyReady(t)
	a := assessment(u, cs)
	a["client_session_id"] = "abc"
	out, isErr := e.call("record_assessment", a)
	if isErr {
		t.Fatalf("record_assessment: %s", out)
	}
	var r1 recordOut
	json.Unmarshal([]byte(out), &r1)
	got, _ := e.st.Unit(context.Background(), u.ID, false)
	if got.Status != store.StatusStudied {
		t.Fatalf("status = %s", got.Status)
	}
	out, _ = e.call("record_assessment", a)
	var r2 recordOut
	json.Unmarshal([]byte(out), &r2)
	if !r2.AlreadyStored || r2.SessionID != r1.SessionID {
		t.Fatalf("repeat call: %+v vs %+v", r2, r1)
	}
	sessions, _ := e.st.Sessions(context.Background(), u.ID)
	if len(sessions) != 1 || len(sessions[0].Answers) != 3 || sessions[0].Answers[0].Response != "Logs are appended…" || len(sessions[0].Gaps) != 1 {
		t.Fatalf("sessions = %+v", sessions)
	}
	// cs[1] scored 1 → weak.
	out, _ = e.call("get_weak_concepts", map[string]any{})
	if !strings.Contains(out, "Compaction") || strings.Contains(out, "Chapter framing") {
		t.Fatalf("weak concepts: %s", out)
	}
}

func TestPrompt_MCP6_DDIAStudy(t *testing.T) {
	e := setup(t, false)
	res, err := e.cs.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: "ddia-study", Arguments: map[string]string{"unit_id": "7"}})
	if err != nil {
		t.Fatal(err)
	}
	txt := res.Messages[0].Content.(*mcp.TextContent).Text
	for _, want := range []string{"unit 7", "## 1. Explain", "record_assessment", "predict_outcome"} {
		if !strings.Contains(txt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}
