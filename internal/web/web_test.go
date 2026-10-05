package web

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/paulnopaul/ddia-assist/internal/store"
	"github.com/paulnopaul/ddia-assist/internal/testbook"
)

func newServer(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dir, "ddia.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return Handler(st, "test", dir), st
}

func do(t *testing.T, h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func upload(t *testing.T, h http.Handler, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("epub", "book.epub")
	fw.Write(data)
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/import", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return do(t, h, req)
}

func post(t *testing.T, h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return do(t, h, req)
}

func TestHealthz(t *testing.T) {
	h, _ := newServer(t)
	if rec := do(t, h, httptest.NewRequest(http.MethodGet, "/healthz", nil)); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestImportAndPlan_UI1_UI2(t *testing.T) {
	h, _ := newServer(t)
	rec := upload(t, h, testbook.HTMLBook())
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Proposed units") {
		t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, httptest.NewRequest(http.MethodGet, "/plan", nil))
	body := rec.Body.String()
	for _, want := range []string{"Chapter 1", "Chapter 2", "Mark read", "not started"} {
		if !strings.Contains(body, want) {
			t.Errorf("plan page missing %q", want)
		}
	}
}

func TestMarkRead_UI3(t *testing.T) {
	h, st := newServer(t)
	upload(t, h, testbook.HTMLBook())
	units, _ := st.Units(context.Background())
	rec := post(t, h, "/units/"+itoa(units[0].ID)+"/status", url.Values{"status": {"read"}})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ddia-study") {
		t.Fatalf("mark read: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	u, _ := st.Unit(context.Background(), units[0].ID, false)
	if u.Status != store.StatusRead {
		t.Fatalf("status = %s", u.Status)
	}
	if rec := post(t, h, "/units/"+itoa(units[0].ID)+"/status", url.Values{"status": {"studied"}}); !strings.Contains(rec.Header().Get("Location"), "error") {
		t.Fatal("studied must not be settable by hand")
	}
}

func TestUnitPage_UI7_NoBookText(t *testing.T) {
	h, st := newServer(t)
	upload(t, h, testbook.HTMLBook())
	units, _ := st.Units(context.Background())
	for _, u := range units {
		body := do(t, h, httptest.NewRequest(http.MethodGet, "/units/"+itoa(u.ID), nil)).Body.String()
		if strings.Contains(body, "lorem") {
			t.Fatalf("unit %d page shows book text", u.ID)
		}
	}
}

func itoa(i int64) string { return strconv.FormatInt(i, 10) }

func TestMarkUnread_UI3(t *testing.T) {
	h, st := newServer(t)
	upload(t, h, testbook.HTMLBook())
	ctx := context.Background()
	u := mustUnitsM1(t, st)[0]
	post(t, h, "/units/"+itoa(u.ID)+"/status", url.Values{"status": {"read"}})
	if body := do(t, h, httptest.NewRequest(http.MethodGet, "/plan", nil)).Body.String(); !strings.Contains(body, "Mark unread") {
		t.Fatal("plan page should offer Mark unread for a read unit")
	}
	rec := post(t, h, "/units/"+itoa(u.ID)+"/status", url.Values{"status": {"not_started"}})
	if strings.Contains(rec.Header().Get("Location"), "error") {
		t.Fatalf("unread: %s", rec.Header().Get("Location"))
	}
	got, _ := st.Unit(ctx, u.ID, false)
	if got.Status != store.StatusNotStarted || got.ReadAt != "" || got.InScope() {
		t.Fatalf("after unread: status=%s read_at=%q", got.Status, got.ReadAt)
	}
}

func mustUnitsM1(t *testing.T, st *store.Store) []store.Unit {
	t.Helper()
	us, err := st.Units(context.Background())
	if err != nil || len(us) == 0 {
		t.Fatalf("units: %v", err)
	}
	return us
}
