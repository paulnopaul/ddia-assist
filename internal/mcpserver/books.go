package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/paulnopaul/ddia-assist/internal/store"
)

// bookArg describes the `book` prompt argument (MCP-10).
const bookArg = "Optional. A few words of a book's title, or its initials (ddia), to switch to that book first. Leave empty for the active book."

// resolveBook applies the `book` prompt argument (MCP-10): a single match on a
// title's words or its initials ("ddia") becomes the active book. Empty keeps the
// active book, which may be nil when nothing is imported yet; the session then hits
// get_status's error. An argument that matches no book, or several, keeps the active
// book and returns a note telling Claude to ask, because Claude Desktop hides prompt
// errors behind "failed to attach prompt".
func (s *server) resolveBook(ctx context.Context, arg string) (*store.Book, string, error) {
	active, err := s.st.ActiveBook(ctx)
	if errors.Is(err, store.ErrNotFound) {
		active, err = nil, nil
	}
	arg = strings.TrimSpace(arg)
	if arg == "" || err != nil {
		return active, "", err
	}
	books, err := s.st.Books(ctx)
	if err != nil {
		return nil, "", err
	}
	var matches []store.BookSummary
	for _, b := range books {
		if titleMatches(b.Title, arg) {
			matches = append(matches, b)
		}
	}
	if len(matches) == 1 {
		if err := s.st.SetActiveBook(ctx, matches[0].ID); err != nil {
			return nil, "", err
		}
		return &matches[0].Book, "", nil
	}
	if len(books) == 0 {
		return nil, fmt.Sprintf("I asked for the book %q, but no book is imported yet. Tell me to import an EPUB at http://localhost:8080/import, and stop.", arg), nil
	}
	what := "no imported book matches it"
	if len(matches) > 1 {
		what = "several books match it"
		books = matches
	}
	titles := make([]string, len(books))
	for i, b := range books {
		titles[i] = fmt.Sprintf("%q", b.Title)
	}
	return active, fmt.Sprintf("I asked for the book %q, but %s. Tell me that, list these books: %s, and ask me to start the prompt again "+
		"with a few words of the title I want. Don't start the session.", arg, what, strings.Join(titles, ", ")), nil
}

// titleMatches reports whether arg is part of title or equals its initials, both
// case-insensitive: "data-intensive" and "ddia" both match "Designing Data-Intensive Applications".
func titleMatches(title, arg string) bool {
	title, arg = strings.ToLower(title), strings.ToLower(arg)
	if strings.Contains(title, arg) {
		return true
	}
	var initials strings.Builder
	for _, w := range strings.FieldsFunc(title, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		initials.WriteRune([]rune(w)[0])
	}
	return initials.String() == strings.Join(strings.Fields(arg), "")
}

// withNote puts a note about a prompt argument in front of the prompt text (MCP-10).
func withNote(note, text string) string {
	if note == "" {
		return text
	}
	return "**Before anything else:** " + note + "\n\n" + text
}

// bookLine names the session's book in the prompt text (MCP-10).
func bookLine(b *store.Book) string {
	if b == nil {
		return "the active book (see `get_status`)"
	}
	return fmt.Sprintf("%q", b.Title)
}
