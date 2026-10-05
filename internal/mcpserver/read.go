package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/paulnopaul/ddia-assist/internal/store"
)

type unitRef struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

type statusOut struct {
	Book         string   `json:"book"`
	UnitsTotal   int      `json:"units_total"`
	UnitsRead    int      `json:"units_read"`
	UnitsStudied int      `json:"units_studied"`
	NextToStudy  *unitRef `json:"next_to_study,omitempty"`
	NextToRead   *unitRef `json:"next_to_read,omitempty"`
	DueReviews   int      `json:"due_reviews"`
	Weakest      []weak   `json:"weakest_concepts,omitempty"`
}

type weak struct {
	ConceptID int64  `json:"concept_id"`
	Name      string `json:"name"`
	Score     int    `json:"score"`
}

type unitIn struct {
	UnitID int64 `json:"unit_id,omitempty" jsonschema:"unit to fetch; defaults to the next unit that is read but not studied yet"`
}

type sectionIn struct {
	SectionID string `json:"section_id" jsonschema:"section ID such as ch05.s02.s01"`
}

type figureIn struct {
	Name string `json:"name" jsonschema:"figure name from a figure:<name> reference in the unit text"`
}

type searchIn struct {
	Query string `json:"query" jsonschema:"words to search for; a trailing * matches prefixes"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum hits, at most 10"`
}

type searchOut struct {
	Hits []hit `json:"hits"`
}

type hit struct {
	SectionID string `json:"section_id"`
	Heading   string `json:"heading"`
	UnitID    int64  `json:"unit_id"`
	Snippet   string `json:"snippet"`
}

type markReadIn struct {
	UnitID int64 `json:"unit_id"`
}

type okOut struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

func (s *server) addReadTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{Name: "get_status", Description: "Reading progress: units read and studied, the next unit to study, due reviews and weakest concepts."}, s.getStatus)
	mcp.AddTool(srv, &mcp.Tool{Name: "get_unit", Description: "The full Markdown text of a unit the user has read, with section IDs and known concepts. Fails for units not marked read (MCP-2)."}, s.getUnit)
	mcp.AddTool(srv, &mcp.Tool{Name: "get_section", Description: "One section's Markdown by ID. Only sections in units the user has read."}, s.getSection)
	mcp.AddTool(srv, &mcp.Tool{Name: "get_figure", Description: "A figure image by name, from a figure:<name> reference. Only figures in read units."}, s.getFigure)
	mcp.AddTool(srv, &mcp.Tool{Name: "search_book", Description: "Full-text search over the units the user has read. Returns section IDs and snippets."}, s.searchBook)
	mcp.AddTool(srv, &mcp.Tool{Name: "mark_read", Description: "Mark a unit as read. Only call this after the user has confirmed they finished reading it."}, s.markRead)
}

func (s *server) getStatus(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, statusOut, error) {
	var out statusOut
	b, err := s.st.CurrentBook(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return nil, out, errors.New("no book imported yet; ask the user to import their EPUB at http://localhost:8080/import")
	} else if err != nil {
		return nil, out, err
	}
	out.Book = b.Title
	counts, err := s.st.Counts(ctx)
	if err != nil {
		return nil, out, err
	}
	for _, n := range counts {
		out.UnitsTotal += n
	}
	out.UnitsRead = counts[store.StatusRead] + counts[store.StatusStudied]
	out.UnitsStudied = counts[store.StatusStudied]
	if u, err := s.st.NextUnit(ctx, store.StatusRead); err == nil {
		out.NextToStudy = &unitRef{u.ID, u.Title}
	}
	if u, err := s.st.NextUnit(ctx, store.StatusNotStarted, store.StatusReading); err == nil {
		out.NextToRead = &unitRef{u.ID, u.Title}
	}
	if err := s.statusExtras(ctx, &out); err != nil {
		return nil, out, err
	}
	return nil, out, nil
}

func (s *server) getUnit(ctx context.Context, _ *mcp.CallToolRequest, in unitIn) (*mcp.CallToolResult, any, error) {
	id := in.UnitID
	if id == 0 {
		u, err := s.st.NextUnit(ctx, store.StatusRead)
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, errors.New("no unit is waiting to be studied: every read unit is already studied. Ask the user which unit they just finished, confirm, then call mark_read")
		} else if err != nil {
			return nil, nil, err
		}
		id = u.ID
	}
	u, err := s.st.Unit(ctx, id, true)
	if err != nil {
		return nil, nil, err
	}
	if !u.InScope() {
		return nil, nil, fmt.Errorf("unit %d (%q) is not marked read yet (MCP-2). Ask the user whether they have finished reading it; only if they confirm, call mark_read and try again", u.ID, u.Title)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Unit %d: %s\n\nChapter %d · %d words · status: %s\n", u.ID, u.Title, u.Chapter, u.Words, u.Status)
	for _, sc := range u.Sections {
		fmt.Fprintf(&b, "\n%s [%s] %s\n\n%s\n", strings.Repeat("#", min(sc.Level+2, 6)), sc.ID, sc.Heading, sc.Markdown)
	}
	extra, err := s.unitExtras(ctx, u.ID)
	if err != nil {
		return nil, nil, err
	}
	b.WriteString(extra)
	return text(b.String()), nil, nil
}

func (s *server) getSection(ctx context.Context, _ *mcp.CallToolRequest, in sectionIn) (*mcp.CallToolResult, any, error) {
	sc, err := s.st.Section(ctx, in.SectionID)
	if errors.Is(err, store.ErrOutOfScope) {
		return nil, nil, fmt.Errorf("section %s is in a unit the user hasn't read yet; don't use it", in.SectionID)
	} else if err != nil {
		return nil, nil, fmt.Errorf("section %s: %w", in.SectionID, err)
	}
	return text(fmt.Sprintf("## [%s] %s\n\n%s\n", sc.ID, sc.Heading, sc.Markdown)), nil, nil
}

func (s *server) getFigure(ctx context.Context, _ *mcp.CallToolRequest, in figureIn) (*mcp.CallToolResult, any, error) {
	f, err := s.st.FigureInScope(ctx, filepath.Base(in.Name))
	if errors.Is(err, store.ErrOutOfScope) {
		return nil, nil, fmt.Errorf("figure %s is in a unit the user hasn't read yet", in.Name)
	} else if err != nil {
		return nil, nil, fmt.Errorf("figure %s: %w", in.Name, err)
	}
	data, err := os.ReadFile(f.Path)
	if err != nil {
		return nil, nil, err
	}
	mt := mime.TypeByExtension(filepath.Ext(f.Name))
	if mt == "" {
		mt = "image/png"
	}
	return &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.TextContent{Text: f.Caption},
		&mcp.ImageContent{Data: data, MIMEType: mt},
	}}, nil, nil
}

func (s *server) searchBook(ctx context.Context, _ *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, searchOut, error) {
	hits, err := s.st.Search(ctx, in.Query, in.Limit)
	if err != nil {
		return nil, searchOut{}, err
	}
	out := searchOut{Hits: []hit{}}
	for _, h := range hits {
		out.Hits = append(out.Hits, hit{h.SectionID, h.Heading, h.UnitID, h.Snippet})
	}
	return nil, out, nil
}

func (s *server) markRead(ctx context.Context, _ *mcp.CallToolRequest, in markReadIn) (*mcp.CallToolResult, okOut, error) {
	u, err := s.st.Unit(ctx, in.UnitID, false)
	if err != nil {
		return nil, okOut{}, fmt.Errorf("unit %d: %w", in.UnitID, err)
	}
	if u.InScope() {
		return nil, okOut{OK: true, Message: "already " + u.Status}, nil
	}
	if err := s.st.SetStatus(ctx, u.ID, store.StatusRead); err != nil {
		return nil, okOut{}, err
	}
	return nil, okOut{OK: true, Message: fmt.Sprintf("unit %d marked read", u.ID)}, nil
}
