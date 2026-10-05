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
		Arguments:   []*mcp.PromptArgument{{Name: "limit", Description: "How many concepts to review (default 5)."}},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		limit := 5
		if n, err := strconv.Atoi(strings.TrimSpace(req.Params.Arguments["limit"])); err == nil && n > 0 {
			limit = n
		}
		return &mcp.GetPromptResult{
			Description: "DDIA review session",
			Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: strings.ReplaceAll(promptText("ddia-review"), "{{LIMIT}}", strconv.Itoa(limit))}}},
		}, nil
	})
}

func (s *server) getDueReviews(ctx context.Context, _ *mcp.CallToolRequest, in dueIn) (*mcp.CallToolResult, dueOut, error) {
	d, err := s.st.DueReviews(ctx, in.Limit)
	return nil, dueOut{Due: d}, err
}

func (s *server) recordReview(ctx context.Context, _ *mcp.CallToolRequest, in store.ReviewIn) (*mcp.CallToolResult, reviewOut, error) {
	st, err := s.st.RecordReview(ctx, in)
	if err != nil {
		return nil, reviewOut{}, toolErr(err)
	}
	return nil, reviewOut{NextDueAt: st.DueAt, IntervalDays: st.Interval, Streak: st.Streak, Mastered: st.Mastered}, nil
}

func (s *server) dueCount(ctx context.Context) (int, error) { return s.st.DueCount(ctx) }
