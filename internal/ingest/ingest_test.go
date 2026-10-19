package ingest

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paulnopaul/ddia-assist/internal/store"
	"github.com/paulnopaul/ddia-assist/internal/testbook"
)

func setup(t *testing.T) (*store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dir, "ddia.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st, dir
}

func TestImport_HTMLBook(t *testing.T) {
	st, dir := setup(t)
	ctx := context.Background()
	res, err := Import(ctx, st, dir, testbook.HTMLBook())
	if err != nil {
		t.Fatal(err)
	}
	if res.Chapters != 2 || res.Figures != 1 || res.Units < 2 {
		t.Fatalf("result = %+v", res)
	}
	units, err := st.Units(ctx, active(t, st))
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range units {
		if u.Status != store.StatusNotStarted {
			t.Errorf("unit %d status %s", u.ID, u.Status)
		}
		for _, s := range u.Sections {
			if strings.HasSuffix(s.ID, ".refs") {
				t.Errorf("ING-6: reference section %s in unit %d", s.ID, u.ID)
			}
		}
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "figures", "*", "fig1.png")); len(m) != 1 {
		t.Errorf("ING-5: figure not extracted to a folder per book: %v", m)
	}
}

func TestImport_ING8_SameFileIsNoop(t *testing.T) {
	st, dir := setup(t)
	ctx := context.Background()
	data := testbook.HTMLBook()
	if _, err := Import(ctx, st, dir, data); err != nil {
		t.Fatal(err)
	}
	res, err := Import(ctx, st, dir, data)
	if err != nil || !res.Unchanged {
		t.Fatalf("second import = %+v, %v", res, err)
	}
}

func TestImport_ING13_DAT6_DAT7_SecondBookIsSeparate(t *testing.T) {
	st, dir := setup(t)
	ctx := context.Background()
	first, err := Import(ctx, st, dir, testbook.HTMLBook())
	if err != nil {
		t.Fatal(err)
	}
	firstUnits, _ := st.Units(ctx, first.BookID)
	if err := st.SetStatus(ctx, firstUnits[0].ID, store.StatusRead); err != nil {
		t.Fatal(err)
	}
	// Same section IDs and figure names, different file.
	second, err := Import(ctx, st, dir, testbook.HTMLBookTitled("Second Book"))
	if err != nil {
		t.Fatalf("ING-13: second book: %v", err)
	}
	if second.Unchanged || second.BookID == first.BookID {
		t.Fatalf("ING-13: second import = %+v", second)
	}
	if active(t, st) != second.BookID {
		t.Error("ING-13: the new book should become active")
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "figures", "*", "fig1.png")); len(m) != 2 {
		t.Errorf("ING-5: want one figure folder per book, got %v", m)
	}
	books, _ := st.Books(ctx)
	if len(books) != 2 || books[0].Read != 1 || books[1].Read != 0 {
		t.Fatalf("books = %+v", books)
	}
	// DAT-6: the same section ID is in scope in one book and not the other.
	sid := firstUnits[0].Sections[0].ID
	if _, err := st.Section(ctx, first.BookID, sid); err != nil {
		t.Errorf("book 1 %s: %v", sid, err)
	}
	if _, err := st.Section(ctx, second.BookID, sid); err != store.ErrOutOfScope {
		t.Errorf("book 2 %s: err = %v, want ErrOutOfScope", sid, err)
	}
	if hits, _ := st.Search(ctx, first.BookID, "introword", 5); len(hits) != 1 {
		t.Errorf("book 1 search: %v", hits)
	}
	if hits, _ := st.Search(ctx, second.BookID, "introword", 5); len(hits) != 0 {
		t.Errorf("book 2 search leaks book 1: %v", hits)
	}
	if _, err := st.FigureInScope(ctx, second.BookID, "fig1.png"); err != store.ErrOutOfScope {
		t.Errorf("book 2 figure: err = %v, want ErrOutOfScope", err)
	}
	// DAT-7: deleting the active book makes the remaining one active, with its data intact.
	if err := st.DeleteBook(ctx, second.BookID); err != nil {
		t.Fatal(err)
	}
	if active(t, st) != first.BookID {
		t.Error("DAT-7: book 1 should be active after deleting book 2")
	}
	if after, _ := st.Units(ctx, first.BookID); len(after) != len(firstUnits) || after[0].Status != store.StatusRead {
		t.Errorf("book 1 changed by deleting book 2: %+v", after[0])
	}
	if hits, _ := st.Search(ctx, first.BookID, "introword", 5); len(hits) != 1 {
		t.Errorf("book 1 search after delete: %v", hits)
	}
	if err := st.DeleteBook(ctx, first.BookID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ActiveBook(ctx); err != store.ErrNotFound {
		t.Errorf("no books left: active err = %v", err)
	}
}

func TestUnitEditing_ING11(t *testing.T) {
	st, dir := setup(t)
	ctx := context.Background()
	if _, err := Import(ctx, st, dir, testbook.HTMLBook()); err != nil {
		t.Fatal(err)
	}
	units, _ := st.Units(ctx, active(t, st))
	first := units[0]
	if len(first.Sections) < 2 {
		t.Fatalf("first unit too small to split: %+v", first)
	}
	splitAt := first.Sections[len(first.Sections)-1].ID
	if err := st.SplitAt(ctx, first.ID, splitAt); err != nil {
		t.Fatal(err)
	}
	after, _ := st.Units(ctx, active(t, st))
	if len(after) != len(units)+1 || after[1].Sections[0].ID != splitAt {
		t.Fatalf("split: %d units, second starts at %s", len(after), after[1].Sections[0].ID)
	}
	if err := st.MergeWithNext(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	merged, _ := st.Units(ctx, active(t, st))
	if len(merged) != len(units) || len(merged[0].Sections) != len(first.Sections) || merged[0].Words != first.Words {
		t.Fatalf("merge did not restore the unit: %+v", merged[0])
	}
	if err := st.Rename(ctx, first.ID, "My title"); err != nil {
		t.Fatal(err)
	}
}

// active returns the active book's ID.
func active(t *testing.T, st *store.Store) int64 {
	t.Helper()
	b, err := st.ActiveBook(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return b.ID
}
