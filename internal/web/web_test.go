package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paulnopaul/ddia-assist/internal/store"
)

func TestHealthz(t *testing.T) {
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "ddia.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	rec := httptest.NewRecorder()
	Handler(st, "test").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestIndex_RendersEmbeddedTemplate(t *testing.T) {
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "ddia.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	rec := httptest.NewRecorder()
	Handler(st, "test").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Version test, schema 1.") {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
}
