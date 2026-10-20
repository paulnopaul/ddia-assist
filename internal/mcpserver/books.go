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
const bookArg = "Optional. Part of a book's title or its initials (ddia); the closest title wins. Leave empty for the active book."

// resolveBook applies the `book` prompt argument (MCP-10): the book whose title is
// closest to arg (titleDistance) becomes the active book, so the argument never
// fails; Claude Desktop would only show "failed to attach prompt". Empty keeps the
// active book, which may be nil when nothing is imported yet; the session then hits
// get_status's error. With no book imported, a non-empty arg returns a note.
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
	if len(books) == 0 {
		return nil, fmt.Sprintf("I asked for the book %q, but no book is imported yet. Tell me to import an EPUB at http://localhost:8080/import, and stop.", arg), nil
	}
	// Closest title wins; on a tie the active book, then the oldest.
	best, bestDist := 0, -1
	for i, b := range books {
		d := titleDistance(b.Title, arg)
		if bestDist < 0 || d < bestDist || (d == bestDist && b.Active) {
			best, bestDist = i, d
		}
	}
	if err := s.st.SetActiveBook(ctx, books[best].ID); err != nil {
		return nil, "", err
	}
	return &books[best].Book, "", nil
}

// titleDistance is the Levenshtein distance from arg to the closest of: the
// title's initials, and every run of consecutive words in the title. Case and
// punctuation are ignored, so "ddia" and "data-intensive" are both 0 from
// "Designing Data-Intensive Applications".
func titleDistance(title, arg string) int {
	words, want := normWords(title), strings.Join(normWords(arg), " ")
	var initials strings.Builder
	for _, w := range words {
		initials.WriteRune([]rune(w)[0])
	}
	best := levenshtein(initials.String(), strings.ReplaceAll(want, " ", ""))
	for i := range words {
		for j := i + 1; j <= len(words); j++ {
			best = min(best, levenshtein(strings.Join(words[i:j], " "), want))
		}
	}
	return best
}

func normWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
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
