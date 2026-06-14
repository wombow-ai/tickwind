package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestPrefsRequiresAuth: GET /v1/me/prefs without a token → 401.
func TestPrefsRequiresAuth(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/me/prefs")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET /v1/me/prefs without a token = %d; want 401", resp.StatusCode)
	}
}

// TestPrefsEmptyDefault: GET with no stored prefs → 200 {} so the client falls
// back to its default.
func TestPrefsEmptyDefault(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	resp := authed(t, http.MethodGet, srv.URL+"/v1/me/prefs", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET empty prefs = %d; want 200", resp.StatusCode)
	}
	var body map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 0 {
		t.Fatalf("GET empty prefs body = %v; want {}", body)
	}
}

// TestPrefsRoundTrip: PUT then GET round-trips the stored blob per-user.
func TestPrefsRoundTrip(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	put := authed(t, http.MethodPut, srv.URL+"/v1/me/prefs",
		`{"indicators":{"ids":["technical.rsi","technical.macd"]}}`)
	put.Body.Close()
	if put.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT prefs = %d; want 204", put.StatusCode)
	}

	get := authed(t, http.MethodGet, srv.URL+"/v1/me/prefs", "")
	defer get.Body.Close()
	if get.StatusCode != http.StatusOK {
		t.Fatalf("GET prefs = %d; want 200", get.StatusCode)
	}
	var body struct {
		Indicators struct {
			IDs []string `json:"ids"`
		} `json:"indicators"`
	}
	if err := json.NewDecoder(get.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := body.Indicators.IDs; len(got) != 2 || got[0] != "technical.rsi" || got[1] != "technical.macd" {
		t.Fatalf("round-tripped ids = %v; want [technical.rsi technical.macd]", got)
	}
}

// TestPrefsShallowMerge: a PUT that only sets `indicators` must preserve a
// sibling top-level key written earlier (the handler shallow-merges).
func TestPrefsShallowMerge(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	// Seed a sibling pref key + an indicators key.
	seed := authed(t, http.MethodPut, srv.URL+"/v1/me/prefs",
		`{"theme":"dark","indicators":{"ids":["technical.rsi"]}}`)
	seed.Body.Close()
	if seed.StatusCode != http.StatusNoContent {
		t.Fatalf("seed PUT = %d; want 204", seed.StatusCode)
	}

	// A PUT that only updates `indicators` must NOT clobber `theme`.
	upd := authed(t, http.MethodPut, srv.URL+"/v1/me/prefs",
		`{"indicators":{"ids":["technical.macd"]}}`)
	upd.Body.Close()
	if upd.StatusCode != http.StatusNoContent {
		t.Fatalf("update PUT = %d; want 204", upd.StatusCode)
	}

	get := authed(t, http.MethodGet, srv.URL+"/v1/me/prefs", "")
	defer get.Body.Close()
	var body struct {
		Theme      string `json:"theme"`
		Indicators struct {
			IDs []string `json:"ids"`
		} `json:"indicators"`
	}
	if err := json.NewDecoder(get.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Theme != "dark" {
		t.Errorf("sibling key clobbered: theme = %q; want \"dark\"", body.Theme)
	}
	if got := body.Indicators.IDs; len(got) != 1 || got[0] != "technical.macd" {
		t.Errorf("indicators not updated: %v; want [technical.macd]", got)
	}
}

// TestPrefsRejectsNonObject: a non-object JSON body (e.g. an array) → 400.
func TestPrefsRejectsNonObject(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	resp := authed(t, http.MethodPut, srv.URL+"/v1/me/prefs", `["not","an","object"]`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT array body = %d; want 400", resp.StatusCode)
	}
}

// TestPrefsRejectsOversize: a body over the 8 KB cap → 413.
func TestPrefsRejectsOversize(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	big := `{"indicators":{"ids":["` + strings.Repeat("x", 9<<10) + `"]}}`
	resp := authed(t, http.MethodPut, srv.URL+"/v1/me/prefs", big)
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("PUT >8KB body = %d; want 413", resp.StatusCode)
	}
}

// TestPrefsAreUserScoped: user-2 never sees user-1's prefs.
func TestPrefsAreUserScoped(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	// authed() always uses user-1.
	authed(t, http.MethodPut, srv.URL+"/v1/me/prefs",
		`{"indicators":{"ids":["technical.rsi"]}}`).Body.Close()

	// user-2 has no prefs → 200 {}.
	r, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/me/prefs", nil)
	r.Header.Set("Authorization", "Bearer "+token("user-2"))
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]json.RawMessage
	json.NewDecoder(resp.Body).Decode(&body)
	if len(body) != 0 {
		t.Fatalf("user-2 prefs = %v; want {} (user-scoped)", body)
	}
}
