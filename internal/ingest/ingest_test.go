package ingest

import (
	"context"
	"os"
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
	units, err := st.Units(ctx)
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
	if _, err := os.Stat(filepath.Join(dir, "figures", "fig1.png")); err != nil {
		t.Errorf("ING-5: figure not extracted: %v", err)
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
	if _, err := Import(ctx, st, dir, testbook.Flat()); err != ErrOtherBook {
		t.Fatalf("different book: err = %v, want ErrOtherBook", err)
	}
}

func TestUnitEditing_ING11(t *testing.T) {
	st, dir := setup(t)
	ctx := context.Background()
	if _, err := Import(ctx, st, dir, testbook.HTMLBook()); err != nil {
		t.Fatal(err)
	}
	units, _ := st.Units(ctx)
	first := units[0]
	if len(first.Sections) < 2 {
		t.Fatalf("first unit too small to split: %+v", first)
	}
	splitAt := first.Sections[len(first.Sections)-1].ID
	if err := st.SplitAt(ctx, first.ID, splitAt); err != nil {
		t.Fatal(err)
	}
	after, _ := st.Units(ctx)
	if len(after) != len(units)+1 || after[1].Sections[0].ID != splitAt {
		t.Fatalf("split: %d units, second starts at %s", len(after), after[1].Sections[0].ID)
	}
	if err := st.MergeWithNext(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	merged, _ := st.Units(ctx)
	if len(merged) != len(units) || len(merged[0].Sections) != len(first.Sections) || merged[0].Words != first.Words {
		t.Fatalf("merge did not restore the unit: %+v", merged[0])
	}
	if err := st.Rename(ctx, first.ID, "My title"); err != nil {
		t.Fatal(err)
	}
}
