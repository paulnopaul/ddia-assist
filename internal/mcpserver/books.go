package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/paulnopaul/ddia-assist/internal/store"
)

// bookArg describes the `book` prompt argument (MCP-10).
const bookArg = "Optional. A few words of a book's title, to switch to that book first. Leave empty for the active book."

// resolveBook applies the `book` prompt argument (MCP-10): a single title match
// becomes the active book. Empty keeps the active book, which may be nil when
// nothing is imported yet; the session then hits get_status's error.
func (s *server) resolveBook(ctx context.Context, arg string) (*store.Book, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		b, err := s.st.ActiveBook(ctx)
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return b, err
	}
	books, err := s.st.Books(ctx)
	if err != nil {
		return nil, err
	}
	var matches []store.BookSummary
	for _, b := range books {
		if strings.Contains(strings.ToLower(b.Title), strings.ToLower(arg)) {
			matches = append(matches, b)
		}
	}
	if len(matches) == 1 {
		if err := s.st.SetActiveBook(ctx, matches[0].ID); err != nil {
			return nil, err
		}
		return &matches[0].Book, nil
	}
	if len(books) == 0 {
		return nil, errors.New("no book imported yet; import an EPUB at http://localhost:8080/import")
	}
	what := "no book title contains"
	if len(matches) > 1 {
		what = "several book titles contain"
		books = matches
	}
	titles := make([]string, len(books))
	for i, b := range books {
		titles[i] = fmt.Sprintf("%q", b.Title)
	}
	return nil, fmt.Errorf("%s %q; pick one of: %s", what, arg, strings.Join(titles, ", "))
}

// bookLine names the session's book in the prompt text (MCP-10).
func bookLine(b *store.Book) string {
	if b == nil {
		return "the active book (see `get_status`)"
	}
	return fmt.Sprintf("%q", b.Title)
}
