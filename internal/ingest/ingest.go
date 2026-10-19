// Package ingest imports an EPUB: parse, store sections and figures, plan units.
package ingest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/paulnopaul/ddia-assist/internal/epub"
	"github.com/paulnopaul/ddia-assist/internal/plan"
	"github.com/paulnopaul/ddia-assist/internal/store"
)

// Result summarises an import for the confirmation page (UI-1).
type Result struct {
	BookID    int64
	Title     string
	Chapters  int
	Sections  int
	Figures   int
	Words     int
	Units     int
	Unchanged bool
}

// Import parses data and stores it as a new book, which becomes the active
// book (ING-13). Re-importing the same file is a no-op (ING-8).
func Import(ctx context.Context, st *store.Store, dataDir string, data []byte) (*Result, error) {
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	if cur, err := st.BookBySHA(ctx, sha); err == nil {
		return &Result{BookID: cur.ID, Title: cur.Title, Unchanged: true}, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	book, err := epub.Parse(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	// One folder per book, so figure names can repeat across books (ING-5, DAT-6).
	figDir := filepath.Join(dataDir, "figures", sha[:12])
	if err := os.MkdirAll(figDir, 0o755); err != nil {
		return nil, err
	}
	var figs []store.Figure
	for _, f := range book.Figures {
		p := filepath.Join(figDir, filepath.Base(f.Name))
		if err := os.WriteFile(p, f.Data, 0o644); err != nil {
			return nil, err
		}
		figs = append(figs, store.Figure{Name: f.Name, SectionID: f.SectionID, Path: p, Caption: f.Caption})
	}
	var ps []plan.Section
	res := &Result{Title: book.Title, Sections: len(book.Sections), Figures: len(figs)}
	for _, s := range book.Sections {
		ps = append(ps, plan.Section{ID: s.ID, ParentID: s.ParentID, Chapter: s.Chapter, Level: s.Level, Heading: s.Heading, Words: s.Words, IsReference: s.IsReference})
		if s.Level == 0 {
			res.Chapters++
		}
		if !s.IsReference {
			res.Words += s.Words
		}
	}
	units := plan.Plan(ps)
	res.Units = len(units)
	if res.BookID, err = st.SaveBook(ctx, book.Title, sha, book.Sections, figs, units); err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}
	return res, nil
}
