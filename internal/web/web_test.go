package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
