package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	transporthttp "github.com/StephenQiu30/hotkey-server/internal/transport/http"
)

func TestHealthz(t *testing.T) {
	router := transporthttp.NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %q", rec.Code, rec.Body.String())
	}

	want := `{"status":"ok"}`
	if rec.Body.String() != want {
		t.Fatalf("expected body %s, got %s", want, rec.Body.String())
	}
}
