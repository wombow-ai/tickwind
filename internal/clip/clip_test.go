package clip

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCleanTitle(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"entities", "Foo &amp; Bar &quot;x&quot;", `Foo & Bar "x"`},
		{"collapse whitespace", "  hello\n\t world  ", "hello world"},
		{"trim", "   spaced   ", "spaced"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanTitle(tc.in); got != tc.want {
				t.Fatalf("cleanTitle(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCleanTitleTruncates(t *testing.T) {
	if got := cleanTitle(strings.Repeat("a", 300)); len(got) != 200 {
		t.Fatalf("len = %d; want 200", len(got))
	}
}

func TestTitlePrefersOpenGraph(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `<meta property="og:title" content="OG Title"><title>Plain</title>`)
	}))
	defer srv.Close()

	got, err := NewFetcher().Title(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got != "OG Title" {
		t.Fatalf("Title = %q; want og:title", got)
	}
}

func TestTitleFallsBackToTitleTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `<html><head><title>Hello &amp; World</title></head></html>`)
	}))
	defer srv.Close()

	got, err := NewFetcher().Title(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Hello & World" {
		t.Fatalf("Title = %q", got)
	}
}

func TestTitleRejectsNonHTTP(t *testing.T) {
	if _, err := NewFetcher().Title(context.Background(), "ftp://example.com"); err == nil {
		t.Fatal("want error for non-http(s) url")
	}
}
