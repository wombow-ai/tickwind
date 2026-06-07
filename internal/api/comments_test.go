package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestCommentsPublicReadAuthedWrite(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	// Reading is public (no token).
	resp, err := http.Get(srv.URL + "/v1/comments?ticker=AAPL")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("public GET = %d; want 200", resp.StatusCode)
	}

	// Posting without a token → 401.
	r, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/comments", strings.NewReader(`{"ticker":"AAPL","body":"hi"}`))
	r.Header.Set("Content-Type", "application/json")
	rr, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	rr.Body.Close()
	if rr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon post = %d; want 401", rr.StatusCode)
	}
}

func TestCommentsCRUD(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	resp := authed(t, http.MethodPost, srv.URL+"/v1/comments", `{"ticker":"aapl","body":"strong quarter"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("post = %d; want 201", resp.StatusCode)
	}
	var c struct {
		ID     string `json:"id"`
		Ticker string `json:"ticker"`
		Author string `json:"author"`
	}
	json.NewDecoder(resp.Body).Decode(&c)
	resp.Body.Close()
	if !strings.HasPrefix(c.ID, "cmt:") || c.Ticker != "AAPL" || c.Author == "" {
		t.Fatalf("created = %+v; want cmt:* / AAPL / non-empty author", c)
	}

	lr, _ := http.Get(srv.URL + "/v1/comments?ticker=AAPL")
	var list struct {
		Count int `json:"count"`
	}
	json.NewDecoder(lr.Body).Decode(&list)
	lr.Body.Close()
	if list.Count != 1 {
		t.Fatalf("list count = %d; want 1", list.Count)
	}

	// Report (auth required) → 200.
	rep := authed(t, http.MethodPost, srv.URL+"/v1/comments/"+c.ID+"/report", "")
	rep.Body.Close()
	if rep.StatusCode != http.StatusOK {
		t.Fatalf("report = %d; want 200", rep.StatusCode)
	}

	// Delete own, then confirm gone.
	dr := authed(t, http.MethodDelete, srv.URL+"/v1/comments/"+c.ID, "")
	dr.Body.Close()
	if dr.StatusCode != http.StatusOK {
		t.Fatalf("delete = %d; want 200", dr.StatusCode)
	}
	lr2, _ := http.Get(srv.URL + "/v1/comments?ticker=AAPL")
	var list2 struct {
		Count int `json:"count"`
	}
	json.NewDecoder(lr2.Body).Decode(&list2)
	lr2.Body.Close()
	if list2.Count != 0 {
		t.Fatalf("after delete count = %d; want 0", list2.Count)
	}
}

func TestCommentsDeleteIsolation(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	resp := authed(t, http.MethodPost, srv.URL+"/v1/comments", `{"ticker":"AAPL","body":"mine"}`)
	var c struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&c)
	resp.Body.Close()

	// user-2 (not author, not admin) must not delete it → 404.
	r, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/comments/"+c.ID, nil)
	r.Header.Set("Authorization", "Bearer "+token("user-2"))
	rr, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	rr.Body.Close()
	if rr.StatusCode != http.StatusNotFound {
		t.Errorf("user-2 delete = %d; want 404", rr.StatusCode)
	}
}

func TestCommentsRejectsEmpty(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	resp := authed(t, http.MethodPost, srv.URL+"/v1/comments", `{"body":"   "}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty body = %d; want 400", resp.StatusCode)
	}
}

func TestCommentsRateLimit(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	// newRateLimiter(10, 10min): the first 10 are allowed, the 11th is throttled.
	last := 0
	for i := 0; i < 11; i++ {
		resp := authed(t, http.MethodPost, srv.URL+"/v1/comments", `{"body":"spam"}`)
		last = resp.StatusCode
		resp.Body.Close()
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("11th post = %d; want 429 (rate-limited)", last)
	}
}
