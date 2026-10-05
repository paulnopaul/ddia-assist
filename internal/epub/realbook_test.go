package epub

import (
	"os"
	"testing"
)

// TestRealBook parses a real EPUB when DDIA_EPUB points at one. It never runs
// in CI, and it prints only structure, never book text.
func TestRealBook(t *testing.T) {
	p := os.Getenv("DDIA_EPUB")
	if p == "" {
		t.Skip("DDIA_EPUB not set")
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	b, err := Parse(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	words := map[int]int{}
	for _, s := range b.Sections {
		if !s.IsReference {
			words[s.Chapter] += s.Words
		}
		if os.Getenv("DDIA_VERBOSE") != "" {
			t.Logf("%-16s L%d %6d ref=%v %q", s.ID, s.Level, s.Words, s.IsReference, s.Heading)
		}
	}
	t.Logf("title=%q sections=%d figures=%d words/chapter=%v", b.Title, len(b.Sections), len(b.Figures), words)
}
