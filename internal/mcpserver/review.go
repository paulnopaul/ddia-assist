package mcpserver

import (
	"context"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/paulnopaul/ddia-assist/internal/store"
)

type dueIn struct {
	Limit int `json:"limit,omitempty" jsonschema:"maximum items, default 5"`
}

type dueOut struct {
	Due []store.DueReview `json:"due"`
}

type reviewOut struct {
	NextDueAt    string `json:"next_due_at"`
	IntervalDays int    `json:"interval_days"`
	Streak       int    `json:"streak"`
	Mastered     bool   `json:"mastered"`
}

func (s *server) addReview(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{Name: "get_due_reviews", Description: "Concepts due for spaced review (from read units only), with the last gap and last review question."}, s.getDueReviews)
	mcp.AddTool(srv, &mcp.Tool{Name: "record_review", Description: "Store one review answer with a 0-3 score and reschedule the concept. The question must differ from the concept's last one."}, s.recordReview)

	srv.AddPrompt(&mcp.Prompt{
		Name:        "ddia-review",
		Title:       "Review weak concepts",
		Description: "Short spaced-review questions on concepts that are due.",
		Arguments: []*mcp.PromptArgument{
			{Name: "limit", Description: "How many concepts to review (default 5)."},
			{Name: "book", Description: bookArg},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		book, note, err := s.resolveBook(ctx, req.Params.Arguments["book"])
		if err != nil {
			return nil, err
		}
		limit := 5
		if n, err := strconv.Atoi(strings.TrimSpace(req.Params.Arguments["limit"])); err == nil && n > 0 {
			limit = n
		}
		text := strings.NewReplacer("{{LIMIT}}", strconv.Itoa(limit), "{{BOOK}}", bookLine(book)).Replace(withNote(note, promptText("ddia-review")))
		return &mcp.GetPromptResult{
			Description: "Review session",
			Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: text}}},
		}, nil
	})
}

func (s *server) getDueReviews(ctx context.Context, _ *mcp.CallToolRequest, in dueIn) (*mcp.CallToolResult, dueOut, error) {
	b, err := s.activeBook(ctx)
	if err != nil {
		return nil, dueOut{}, err
	}
	d, err := s.st.DueReviews(ctx, b.ID, in.Limit)
	return nil, dueOut{Due: d}, err
}

func (s *server) recordReview(ctx context.Context, _ *mcp.CallToolRequest, in store.ReviewIn) (*mcp.CallToolResult, reviewOut, error) {
	st, err := s.st.RecordReview(ctx, in)
	if err != nil {
		return nil, reviewOut{}, toolErr(err)
	}
	return nil, reviewOut{NextDueAt: st.DueAt, IntervalDays: st.Interval, Streak: st.Streak, Mastered: st.Mastered}, nil
}
