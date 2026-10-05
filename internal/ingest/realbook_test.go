package ingest

import (
	"context"
	"os"
	"testing"
)

// TestRealBook imports a real EPUB when DDIA_EPUB is set. It never runs in CI
// and logs only structure (unit titles and sizes), never book text.
func TestRealBook(t *testing.T) {
	p := os.Getenv("DDIA_EPUB")
	if p == "" {
		t.Skip("DDIA_EPUB not set")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	st, dir := setup(t)
	res, err := Import(context.Background(), st, dir, data)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", *res)
	units, _ := st.Units(context.Background())
	for _, u := range units {
		t.Logf("ch%02d  %5d words  %2d sections  %s", u.Chapter, u.Words, len(u.Sections), u.Title)
	}
}
