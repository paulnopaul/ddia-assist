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
	units, _ := st.Units(context.Background(), active(t, st))
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
	units, _ := st.Units(context.Background(), active(t, st))
	for _, u := range units {
		body := do(t, h, httptest.NewRequest(http.MethodGet, "/units/"+itoa(u.ID), nil)).Body.String()
		if strings.Contains(body, "lorem") {
			t.Fatalf("unit %d page shows book text", u.ID)
		}
	}
}

func itoa(i int64) string { return strconv.FormatInt(i, 10) }

func TestUnitPage_UI4_SessionsAndOverride_SES11(t *testing.T) {
	h, st := newServer(t)
	upload(t, h, testbook.HTMLBook())
	ctx := context.Background()
	u := mustUnits(t, st)[0]
	st.SetStatus(ctx, u.ID, store.StatusRead)
	cs, err := st.SaveConcepts(ctx, u.ID, []store.ConceptIn{
		{Name: "Log", Definition: "d", SectionRef: "ch01.s01"},
		{Name: "Compaction", Definition: "d", SectionRef: "ch01.s01.s01"},
		{Name: "Framing", Definition: "d", SectionRef: "ch01"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var scores []store.ScoreIn
	for _, c := range cs {
		scores = append(scores, store.ScoreIn{ConceptID: c.ID, Score: 1})
	}
	if _, _, err := st.RecordAssessment(ctx, store.Assessment{UnitID: u.ID,
		Answers:       []store.AnswerIn{{Step: "explain", Prompt: "Explain", Response: "my <b>answer</b>"}},
		ConceptScores: scores,
		Gaps:          []store.GapIn{{Description: "missed it", SectionRef: "ch01.s01"}}}); err != nil {
		t.Fatal(err)
	}
	body := do(t, h, httptest.NewRequest(http.MethodGet, "/units/"+itoa(u.ID), nil)).Body.String()
	for _, want := range []string{"Session 1", "my &lt;b&gt;answer&lt;/b&gt;", "reread ch01.s01", "Compaction"} {
		if !strings.Contains(body, want) {
			t.Errorf("unit page missing %q", want)
		}
	}
	sessions, _ := st.Sessions(ctx, u.ID)
	rec := post(t, h, "/scores/"+itoa(sessions[0].Scores[0].ID)+"/override", url.Values{"score": {"3"}})
	if strings.Contains(rec.Header().Get("Location"), "error") {
		t.Fatalf("override: %s", rec.Header().Get("Location"))
	}
	after, _ := st.Concepts(ctx, u.ID)
	if *after[0].Score != 3 {
		t.Fatalf("DAT-2: current score = %d, want override 3", *after[0].Score)
	}
}

func mustUnits(t *testing.T, st *store.Store) []store.Unit {
	t.Helper()
	us, err := st.Units(context.Background(), active(t, st))
	if err != nil || len(us) == 0 {
		t.Fatalf("units: %v", err)
	}
	return us
}

func TestDashboardAndConcepts_UI5_UI6(t *testing.T) {
	h, st := newServer(t)
	if rec := do(t, h, httptest.NewRequest(http.MethodGet, "/", nil)); rec.Header().Get("Location") != "/import" {
		t.Fatalf("dashboard without a book should send you to import, got %d %s", rec.Code, rec.Header().Get("Location"))
	}
	upload(t, h, testbook.HTMLBook())
	ctx := context.Background()
	u := mustUnits(t, st)[0]
	st.SetStatus(ctx, u.ID, store.StatusRead)
	cs, _ := st.SaveConcepts(ctx, u.ID, []store.ConceptIn{
		{Name: "Log", Definition: "d", SectionRef: "ch01.s01"},
		{Name: "Compaction", Definition: "d", SectionRef: "ch01.s01.s01"},
		{Name: "Framing", Definition: "d", SectionRef: "ch01"},
	})
	scores := []store.ScoreIn{{ConceptID: cs[0].ID, Score: 0}, {ConceptID: cs[1].ID, Score: 3}, {ConceptID: cs[2].ID, Score: 2}}
	if _, _, err := st.RecordAssessment(ctx, store.Assessment{UnitID: u.ID,
		Answers: []store.AnswerIn{{Step: "explain", Prompt: "p", Response: "r"}}, ConceptScores: scores}); err != nil {
		t.Fatal(err)
	}
	body := do(t, h, httptest.NewRequest(http.MethodGet, "/", nil)).Body.String()
	for _, want := range []string{"Test Book", "1/", "units studied", "Weakest concepts", "Log"} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard missing %q", want)
		}
	}
	body = do(t, h, httptest.NewRequest(http.MethodGet, "/concepts", nil)).Body.String()
	if !strings.Contains(body, "not queued") || !strings.Contains(body, "every 1 d") {
		t.Errorf("concepts page missing review state:\n%s", body)
	}
	if strings.Index(body, "Log") > strings.Index(body, "Compaction") {
		t.Error("UI-5: weakest concept should come first")
	}
}

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
	us, err := st.Units(context.Background(), active(t, st))
	if err != nil || len(us) == 0 {
		t.Fatalf("units: %v", err)
	}
	return us
}

func TestUnitNumbers_UI8(t *testing.T) {
	h, st := newServer(t)
	upload(t, h, testbook.HTMLBook())
	u := mustUnits(t, st)[0]
	if body := do(t, h, httptest.NewRequest(http.MethodGet, "/plan", nil)).Body.String(); !strings.Contains(body, "#"+itoa(u.ID)+"</span>") {
		t.Error("plan page should show each unit's number")
	}
	body := do(t, h, httptest.NewRequest(http.MethodGet, "/units/"+itoa(u.ID), nil)).Body.String()
	if !strings.Contains(body, "#"+itoa(u.ID)+"</span>") || !strings.Contains(body, "<code>"+itoa(u.ID)+"</code> as the unit") {
		t.Error("unit page should show the unit number and how to pass it to ddia-study")
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

func TestBooks_UI9_UI10(t *testing.T) {
	h, st := newServer(t)
	upload(t, h, testbook.HTMLBook())
	ctx := context.Background()
	first := active(t, st)
	if rec := upload(t, h, testbook.HTMLBookTitled("Second Book")); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Proposed units") {
		t.Fatalf("ING-13: second import: %d", rec.Code)
	}
	second := active(t, st)
	if second == first {
		t.Fatal("ING-13: the new book should be active")
	}
	body := do(t, h, httptest.NewRequest(http.MethodGet, "/import", nil)).Body.String()
	for _, want := range []string{"Test Book", "Second Book", "active</span>", "Make active", `action="/books/active"`, "/books/" + itoa(first) + "/delete"} {
		if !strings.Contains(body, want) {
			t.Errorf("books page missing %q", want)
		}
	}
	// UI-10: plan shows the active book only; the header switches.
	if body := do(t, h, httptest.NewRequest(http.MethodGet, "/plan", nil)).Body.String(); !strings.Contains(body, "· Second Book") {
		t.Error("plan should name the active book")
	}
	req := httptest.NewRequest(http.MethodPost, "/books/active", strings.NewReader(url.Values{"book": {itoa(first)}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "http://example.com/plan")
	req.Host = "example.com"
	if rec := do(t, h, req); rec.Header().Get("Location") != "/plan" || active(t, st) != first {
		t.Fatalf("switch: %s, active %d", rec.Header().Get("Location"), active(t, st))
	}
	firstUnits, _ := st.Units(ctx, first)
	st.SetStatus(ctx, firstUnits[0].ID, store.StatusRead)
	if body := do(t, h, httptest.NewRequest(http.MethodGet, "/", nil)).Body.String(); !strings.Contains(body, "<h1>Test Book</h1>") || !strings.Contains(body, "1/") {
		t.Error("dashboard should cover the active book")
	}
	// A unit page of the other book says so.
	secondUnits, _ := st.Units(ctx, second)
	if body := do(t, h, httptest.NewRequest(http.MethodGet, "/units/"+itoa(secondUnits[0].ID), nil)).Body.String(); !strings.Contains(body, "Second Book (not the active book)") {
		t.Error("unit page should name its book")
	}
	if rec := post(t, h, "/books/"+itoa(first)+"/delete", nil); strings.Contains(rec.Header().Get("Location"), "error") {
		t.Fatalf("delete: %s", rec.Header().Get("Location"))
	}
	if active(t, st) != second {
		t.Error("DAT-7: the remaining book should become active")
	}
	if rec := post(t, h, "/books/active", url.Values{"book": {"999"}}); !strings.Contains(rec.Header().Get("Location"), "error") {
		t.Error("switching to an unknown book should fail")
	}
}
