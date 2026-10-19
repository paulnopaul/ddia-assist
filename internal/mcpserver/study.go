package mcpserver

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/paulnopaul/ddia-assist/internal/store"
)

// Prompt texts are part of the spec (MCP-8) and are compiled in.
//
//go:embed prompts/*.md
var promptFS embed.FS

func promptText(name string) string {
	b, err := promptFS.ReadFile("prompts/" + name + ".md")
	if err != nil {
		panic(err)
	}
	return string(b)
}

type saveConceptsIn struct {
	UnitID   int64             `json:"unit_id"`
	Concepts []store.ConceptIn `json:"concepts" jsonschema:"5-10 key concepts of the unit"`
}

type conceptsOut struct {
	Concepts []store.Concept `json:"concepts"`
}

type weakIn struct {
	Limit int `json:"limit,omitempty" jsonschema:"maximum concepts, default 10"`
}

type recordOut struct {
	SessionID     int64  `json:"session_id"`
	AlreadyStored bool   `json:"already_stored,omitempty"`
	Message       string `json:"message"`
}

func (s *server) addStudy(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{Name: "save_concepts", Description: "Store a unit's 5-10 key concepts. Only allowed once per unit, the first time it is studied."}, s.saveConcepts)
	mcp.AddTool(srv, &mcp.Tool{Name: "get_weak_concepts", Description: "Concepts from read units whose current score is 0 or 1, weakest first."}, s.getWeakConcepts)
	mcp.AddTool(srv, &mcp.Tool{Name: "record_assessment", Description: "Store a finished study session: answers verbatim, a 0-3 score per concept, and gaps citing section IDs. Marks the unit studied. Errors starting with 'fix and retry' list what to correct."}, s.recordAssessment)

	srv.AddPrompt(&mcp.Prompt{
		Name:        "ddia-study",
		Title:       "Study a unit",
		Description: "Explain, probe, apply and verdict for the next read unit, or the one you name.",
		Arguments: []*mcp.PromptArgument{
			{Name: "unit", Description: "Optional. The unit's number from the web UI (#12) or a few words of its title. " +
				"Leave empty to study the next read, unstudied unit."},
			{Name: "book", Description: bookArg},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		book, err := s.resolveBook(ctx, req.Params.Arguments["book"])
		if err != nil {
			return nil, err
		}
		arg := req.Params.Arguments["unit"]
		if arg == "" {
			arg = req.Params.Arguments["unit_id"] // older clients (MCP-6)
		}
		unit, book, err := s.resolveUnit(ctx, book, arg)
		if err != nil {
			return nil, err
		}
		text := strings.NewReplacer("{{UNIT}}", unit, "{{BOOK}}", bookLine(book)).Replace(promptText("ddia-study"))
		return &mcp.GetPromptResult{
			Description: "Study session",
			Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: text}}},
		}, nil
	})
}

// resolveUnit turns the ddia-study argument into the prompt's unit line (MCP-6). It accepts
// a unit number, optionally prefixed with '#', or words from a unit title in book. A unit
// number may belong to any book, which then becomes the active book and is returned.
func (s *server) resolveUnit(ctx context.Context, book *store.Book, arg string) (string, *store.Book, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "the `next_to_study` unit from `get_status`", book, nil
	}
	describe := func(u store.Unit) string { return fmt.Sprintf("unit %d (%q, chapter %d)", u.ID, u.Title, u.Chapter) }
	if id, err := strconv.ParseInt(strings.TrimPrefix(arg, "#"), 10, 64); err == nil {
		u, err := s.st.Unit(ctx, id, false)
		if errors.Is(err, store.ErrNotFound) {
			return "", nil, fmt.Errorf("there is no unit #%d; unit numbers are shown on the reading plan at /plan", id)
		} else if err != nil {
			return "", nil, err
		}
		if book == nil || u.BookID != book.ID {
			if err := s.st.SetActiveBook(ctx, u.BookID); err != nil {
				return "", nil, err
			}
			if book, err = s.st.Book(ctx, u.BookID); err != nil {
				return "", nil, err
			}
		}
		return describe(*u), book, nil
	}
	if book == nil {
		return "", nil, errors.New("no book imported yet; import an EPUB at http://localhost:8080/import")
	}
	units, err := s.st.Units(ctx, book.ID)
	if err != nil {
		return "", nil, err
	}
	var matches []store.Unit
	for _, u := range units {
		if strings.Contains(strings.ToLower(u.Title), strings.ToLower(arg)) {
			matches = append(matches, u)
		}
	}
	switch len(matches) {
	case 0:
		return "", nil, fmt.Errorf("no unit title in %q contains %q; use a unit number from the reading plan at /plan, or leave the argument empty", book.Title, arg)
	case 1:
		return describe(matches[0]), book, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "the one I mean by %q. Several units match, so ask me which one before anything else:", arg)
	for _, u := range matches {
		b.WriteString("\n   - " + describe(u))
	}
	return b.String(), book, nil
}

func toolErr(err error) error {
	switch {
	case errors.Is(err, store.ErrOutOfScope):
		return errors.New("that unit isn't marked read yet; ask the user to confirm, then call mark_read")
	case errors.Is(err, store.ErrNotFound):
		return errors.New("no such unit")
	}
	return err
}

func (s *server) saveConcepts(ctx context.Context, _ *mcp.CallToolRequest, in saveConceptsIn) (*mcp.CallToolResult, conceptsOut, error) {
	cs, err := s.st.SaveConcepts(ctx, in.UnitID, in.Concepts)
	if err != nil {
		return nil, conceptsOut{}, toolErr(err)
	}
	return nil, conceptsOut{Concepts: cs}, nil
}

func (s *server) getWeakConcepts(ctx context.Context, _ *mcp.CallToolRequest, in weakIn) (*mcp.CallToolResult, conceptsOut, error) {
	if in.Limit <= 0 {
		in.Limit = 10
	}
	b, err := s.activeBook(ctx)
	if err != nil {
		return nil, conceptsOut{}, err
	}
	cs, err := s.st.WeakConcepts(ctx, b.ID, in.Limit)
	if err != nil {
		return nil, conceptsOut{}, err
	}
	if cs == nil {
		cs = []store.Concept{}
	}
	return nil, conceptsOut{Concepts: cs}, nil
}

func (s *server) recordAssessment(ctx context.Context, _ *mcp.CallToolRequest, in store.Assessment) (*mcp.CallToolResult, recordOut, error) {
	id, existed, err := s.st.RecordAssessment(ctx, in)
	if err != nil {
		return nil, recordOut{}, toolErr(err)
	}
	msg := fmt.Sprintf("Session %d recorded; unit %d is now studied.", id, in.UnitID)
	if u, err := s.st.Unit(ctx, in.UnitID, false); err != nil {
		return nil, recordOut{}, err
	} else if due, err := s.st.DueCount(ctx, u.BookID); err == nil && due > 0 {
		msg += fmt.Sprintf(" %d concept reviews are due: suggest /ddia-review.", due)
	}
	return nil, recordOut{SessionID: id, AlreadyStored: existed, Message: msg}, nil
}

// conceptsSection lists a unit's known concepts for get_unit.
func (s *server) conceptsSection(ctx context.Context, unitID int64) (string, error) {
	cs, err := s.st.Concepts(ctx, unitID)
	if err != nil {
		return "", err
	}
	if len(cs) == 0 {
		return "\n---\nKnown concepts: none yet. Pick 5-10 and call save_concepts before scoring.\n", nil
	}
	var b strings.Builder
	b.WriteString("\n---\nKnown concepts (use these IDs in record_assessment):\n")
	for _, c := range cs {
		score := "never scored"
		if c.Score != nil {
			score = fmt.Sprintf("current score %d", *c.Score)
		}
		fmt.Fprintf(&b, "- %d: %s [%s] (%s): %s\n", c.ID, c.Name, c.SectionRef, score, c.Definition)
	}
	return b.String(), nil
}
