package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestNotesRequiresAuth(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/notes")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET /v1/notes without a token = %d; want 401", resp.StatusCode)
	}
}

func TestNotesCRUD(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	// Create (ticker-scoped).
	resp := authed(t, http.MethodPost, srv.URL+"/v1/notes", `{"ticker":"aapl","body":"thesis: strong"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d; want 201", resp.StatusCode)
	}
	var created struct {
		ID     string `json:"id"`
		Ticker string `json:"ticker"`
	}
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if !strings.HasPrefix(created.ID, "note:") || created.Ticker != "AAPL" {
		t.Fatalf("created = %+v; want note:* / AAPL (uppercased)", created)
	}

	// List by ticker.
	lr := authed(t, http.MethodGet, srv.URL+"/v1/notes?ticker=AAPL", "")
	var list struct {
		Count int `json:"count"`
	}
	json.NewDecoder(lr.Body).Decode(&list)
	lr.Body.Close()
	if list.Count != 1 {
		t.Fatalf("list count = %d; want 1", list.Count)
	}

	// Patch (edit body + pin).
	pr := authed(t, http.MethodPatch, srv.URL+"/v1/notes/"+created.ID, `{"body":"updated","pinned":true}`)
	var patched struct {
		Body   string `json:"body"`
		Pinned bool   `json:"pinned"`
	}
	json.NewDecoder(pr.Body).Decode(&patched)
	pr.Body.Close()
	if pr.StatusCode != http.StatusOK || patched.Body != "updated" || !patched.Pinned {
		t.Fatalf("patch = %d %+v; want 200 updated/pinned", pr.StatusCode, patched)
	}

	// Delete, then confirm gone.
	dr := authed(t, http.MethodDelete, srv.URL+"/v1/notes/"+created.ID, "")
	dr.Body.Close()
	if dr.StatusCode != http.StatusOK {
		t.Fatalf("delete = %d; want 200", dr.StatusCode)
	}
	lr2 := authed(t, http.MethodGet, srv.URL+"/v1/notes", "")
	var list2 struct {
		Count int `json:"count"`
	}
	json.NewDecoder(lr2.Body).Decode(&list2)
	lr2.Body.Close()
	if list2.Count != 0 {
		t.Fatalf("after delete count = %d; want 0", list2.Count)
	}
}

func TestNotesDateRange(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	authed(t, http.MethodPost, srv.URL+"/v1/notes", `{"note_date":"2026-06-07","body":"FOMC week"}`).Body.Close()
	authed(t, http.MethodPost, srv.URL+"/v1/notes", `{"note_date":"2026-07-15","body":"earnings"}`).Body.Close()

	lr := authed(t, http.MethodGet, srv.URL+"/v1/notes?from=2026-06-01&to=2026-06-30", "")
	var list struct {
		Count int `json:"count"`
	}
	json.NewDecoder(lr.Body).Decode(&list)
	lr.Body.Close()
	if list.Count != 1 {
		t.Fatalf("date-range count = %d; want 1 (only the June note)", list.Count)
	}
}

func TestNotesRejectsEmptyBody(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	resp := authed(t, http.MethodPost, srv.URL+"/v1/notes", `{"ticker":"AAPL","body":"   "}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty body = %d; want 400", resp.StatusCode)
	}
}

func TestNotesOwnershipIsolation(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	// user-1 creates a note (authed() always uses user-1).
	resp := authed(t, http.MethodPost, srv.URL+"/v1/notes", `{"body":"private"}`)
	var n struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&n)
	resp.Body.Close()

	// user-2 must not be able to patch or delete it → 404 (don't leak existence).
	for _, m := range []string{http.MethodPatch, http.MethodDelete} {
		body := ""
		if m == http.MethodPatch {
			body = `{"pinned":true}`
		}
		var r *http.Request
		if body != "" {
			r, _ = http.NewRequest(m, srv.URL+"/v1/notes/"+n.ID, strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
		} else {
			r, _ = http.NewRequest(m, srv.URL+"/v1/notes/"+n.ID, nil)
		}
		r.Header.Set("Authorization", "Bearer "+token("user-2"))
		rr, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		rr.Body.Close()
		if rr.StatusCode != http.StatusNotFound {
			t.Errorf("user-2 %s another user's note = %d; want 404", m, rr.StatusCode)
		}
	}
}
