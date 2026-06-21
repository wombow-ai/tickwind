package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCORSMiddleware_PreservesACAOOn429 is the regression guard for the intermittent
// blank-page bug: when the inner handler (e.g. the rate limiter) returns a 429, the
// outermost CORSMiddleware must still have set Access-Control-Allow-Origin, otherwise
// the browser reports a misleading CORS error and blanks the page.
func TestCORSMiddleware_PreservesACAOOn429(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests) // mimic the limiter reject path
	})
	rec := httptest.NewRecorder()
	CORSMiddleware(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/holdings", nil))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin on 429 = %q, want \"*\"", got)
	}
}

// TestCORSMiddleware_PreflightShortCircuits verifies an OPTIONS preflight is answered
// 204 with CORS headers WITHOUT reaching the inner handler — so preflights are never
// counted/throttled by the limiter sitting inside.
func TestCORSMiddleware_PreflightShortCircuits(t *testing.T) {
	reached := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true })
	rec := httptest.NewRecorder()
	CORSMiddleware(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/v1/holdings", nil))

	if reached {
		t.Fatal("inner handler was reached on an OPTIONS preflight (should short-circuit)")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("preflight Access-Control-Allow-Origin = %q, want \"*\"", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatal("preflight missing Access-Control-Allow-Methods")
	}
	// Every method the API routes must be allowed, or its browser preflight fails with a
	// CORS error (the PUT /v1/me/prefs settings-toggle bug: PUT was missing from the list).
	allow := rec.Header().Get("Access-Control-Allow-Methods")
	for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"} {
		if !strings.Contains(allow, m) {
			t.Errorf("Access-Control-Allow-Methods %q is missing %s (its preflight would CORS-fail)", allow, m)
		}
	}
}
