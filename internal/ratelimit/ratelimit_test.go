package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// okHandler counts how many requests reach the wrapped handler (i.e. were
// allowed through the limiter) and replies 200.
func okHandler(reached *int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(reached, 1)
		w.WriteHeader(http.StatusOK)
	})
}

func reqWithCFIP(ip, path string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.Header.Set("CF-Connecting-IP", ip)
	r.RemoteAddr = "127.0.0.1:54321" // tunnel/loopback hop — must NOT be the key
	return r
}

// TestUnderLimitPasses verifies that a burst within capacity is fully allowed.
func TestUnderLimitPasses(t *testing.T) {
	var reached int64
	l := New(Config{RPM: 300, Burst: 10})
	h := l.Middleware(okHandler(&reached))

	for i := 0; i < 10; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, reqWithCFIP("203.0.113.5", "/v1/news"))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i, rec.Code)
		}
	}
	if reached != 10 {
		t.Fatalf("reached handler %d times, want 10", reached)
	}
}

// TestBurstBeyondLimitGets429 verifies that exceeding the burst yields a 429
// with a sane Retry-After header, and that the wrapped handler is not called.
func TestBurstBeyondLimitGets429(t *testing.T) {
	var reached int64
	l := New(Config{RPM: 60, Burst: 5})
	// Freeze the clock so no refill happens mid-burst.
	now := time.Now()
	l.now = func() time.Time { return now }
	h := l.Middleware(okHandler(&reached))

	// First 5 (burst) pass.
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, reqWithCFIP("203.0.113.6", "/v1/news"))
		if rec.Code != http.StatusOK {
			t.Fatalf("burst request %d: got %d, want 200", i, rec.Code)
		}
	}
	// The 6th is over the burst → 429.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, reqWithCFIP("203.0.113.6", "/v1/news"))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("over-limit request: got %d, want 429", rec.Code)
	}
	ra := rec.Header().Get("Retry-After")
	if ra == "" {
		t.Fatal("429 missing Retry-After header")
	}
	if n, err := strconv.Atoi(ra); err != nil || n < 1 {
		t.Fatalf("Retry-After = %q, want a positive integer", ra)
	}
	if reached != 5 {
		t.Fatalf("handler reached %d times, want 5 (the 429 must not call next)", reached)
	}
}

// TestRefillOverTime verifies tokens accrue at the configured rate so a blocked
// IP recovers after waiting.
func TestRefillOverTime(t *testing.T) {
	var reached int64
	l := New(Config{RPM: 60, Burst: 1}) // 1 token/sec, capacity 1
	now := time.Now()
	l.now = func() time.Time { return now }
	h := l.Middleware(okHandler(&reached))

	// Spend the single token.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, reqWithCFIP("203.0.113.7", "/v1/news"))
	if rec.Code != http.StatusOK {
		t.Fatalf("first request: got %d, want 200", rec.Code)
	}
	// Immediately again → blocked.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, reqWithCFIP("203.0.113.7", "/v1/news"))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("immediate second request: got %d, want 429", rec.Code)
	}
	// Advance 1.1s → one token refilled → allowed again.
	now = now.Add(1100 * time.Millisecond)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, reqWithCFIP("203.0.113.7", "/v1/news"))
	if rec.Code != http.StatusOK {
		t.Fatalf("after refill: got %d, want 200", rec.Code)
	}
}

// TestKeyedByCFConnectingIP verifies the limiter keys on CF-Connecting-IP, not
// RemoteAddr: two different client IPs sharing the same tunnel RemoteAddr get
// independent buckets, and exhausting one does not affect the other.
func TestKeyedByCFConnectingIP(t *testing.T) {
	l := New(Config{RPM: 60, Burst: 2})
	now := time.Now()
	l.now = func() time.Time { return now }
	h := l.Middleware(okHandler(new(int64)))

	exhaust := func(ip string) {
		for i := 0; i < 2; i++ {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, reqWithCFIP(ip, "/v1/news"))
			if rec.Code != http.StatusOK {
				t.Fatalf("%s burst %d: got %d, want 200", ip, i, rec.Code)
			}
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, reqWithCFIP(ip, "/v1/news"))
		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("%s over-limit: got %d, want 429", ip, rec.Code)
		}
	}

	// Exhaust IP A (all share RemoteAddr 127.0.0.1).
	exhaust("198.51.100.1")
	// IP B must be unaffected — proving the key is the CF header, not RemoteAddr.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, reqWithCFIP("198.51.100.2", "/v1/news"))
	if rec.Code != http.StatusOK {
		t.Fatalf("second IP after first exhausted: got %d, want 200 (must be keyed per CF-Connecting-IP)", rec.Code)
	}
}

// TestClientIPResolutionOrder checks CF-Connecting-IP > X-Forwarded-For first
// hop > RemoteAddr.
func TestClientIPResolutionOrder(t *testing.T) {
	tests := []struct {
		name   string
		cf     string
		xff    string
		remote string
		want   string
	}{
		{"cf wins", "203.0.113.9", "10.0.0.1, 10.0.0.2", "127.0.0.1:1", "203.0.113.9"},
		{"xff first hop when no cf", "", "198.51.100.7, 10.0.0.2", "127.0.0.1:1", "198.51.100.7"},
		{"xff single", "", "198.51.100.8", "127.0.0.1:1", "198.51.100.8"},
		{"remote when no headers", "", "", "192.0.2.4:5050", "192.0.2.4"},
		{"cf canonicalized", "::ffff:203.0.113.9", "", "127.0.0.1:1", "203.0.113.9"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/v1/news", nil)
			r.RemoteAddr = tc.remote
			if tc.cf != "" {
				r.Header.Set("CF-Connecting-IP", tc.cf)
			}
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			if got := ClientIP(r); got != tc.want {
				t.Fatalf("ClientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestExemptPathsNeverLimited verifies that /healthz and /v1/stream pass through
// unlimited even under a flood that would otherwise 429.
func TestExemptPathsNeverLimited(t *testing.T) {
	var reached int64
	l := New(Config{
		RPM:   60,
		Burst: 1,
		Exempt: func(p string) bool {
			return p == "/healthz" || p == "/v1/stream"
		},
	})
	now := time.Now()
	l.now = func() time.Time { return now }
	h := l.Middleware(okHandler(&reached))

	for _, path := range []string{"/healthz", "/v1/stream"} {
		for i := 0; i < 50; i++ {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, reqWithCFIP("203.0.113.10", path))
			if rec.Code != http.StatusOK {
				t.Fatalf("%s request %d: got %d, want 200 (exempt path must never be limited)", path, i, rec.Code)
			}
		}
	}
	if reached != 100 {
		t.Fatalf("exempt handler reached %d times, want 100", reached)
	}
	// And exempt requests must not have created any bucket state.
	if got := l.size(); got != 0 {
		t.Fatalf("exempt traffic created %d buckets, want 0", got)
	}
}

// TestFailOpenNoClientIP verifies that a request with no resolvable client IP is
// allowed (fail-open) rather than blocked.
func TestFailOpenNoClientIP(t *testing.T) {
	var reached int64
	l := New(Config{RPM: 60, Burst: 1})
	h := l.Middleware(okHandler(&reached))

	for i := 0; i < 20; i++ {
		r := httptest.NewRequest(http.MethodGet, "/v1/news", nil)
		r.RemoteAddr = "" // no headers, no parseable remote → "" → fail-open
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusOK {
			t.Fatalf("no-IP request %d: got %d, want 200 (must fail open)", i, rec.Code)
		}
	}
	if reached != 20 {
		t.Fatalf("handler reached %d times, want 20", reached)
	}
}

// TestEviction verifies idle buckets are swept and that the eviction loses no
// state (an evicted IP returns to a full bucket).
func TestEviction(t *testing.T) {
	l := New(Config{RPM: 60, Burst: 5, IdleEviction: time.Minute})
	now := time.Now()
	l.now = func() time.Time { return now }
	h := l.Middleware(okHandler(new(int64)))

	// Touch an IP, creating a bucket.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, reqWithCFIP("203.0.113.20", "/v1/news"))
	if l.size() != 1 {
		t.Fatalf("after one request, size = %d, want 1", l.size())
	}

	// Not yet idle long enough → kept.
	now = now.Add(30 * time.Second)
	l.sweep()
	if l.size() != 1 {
		t.Fatalf("after 30s, size = %d, want 1 (not yet idle)", l.size())
	}

	// Past the idle window → evicted.
	now = now.Add(2 * time.Minute)
	l.sweep()
	if l.size() != 0 {
		t.Fatalf("after idle window, size = %d, want 0 (should be evicted)", l.size())
	}
}

// TestConcurrentAccess hammers the limiter from many goroutines across many IPs
// to surface data races under -race. It asserts only that the limiter neither
// over-allows nor panics; correctness of counts is covered by the deterministic
// tests above.
func TestConcurrentAccess(t *testing.T) {
	l := New(Config{RPM: 6000, Burst: 100})
	h := l.Middleware(okHandler(new(int64)))

	const goroutines = 32
	const perGoroutine = 200
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			ip := "203.0.113." + strconv.Itoa(g%64)
			for i := 0; i < perGoroutine; i++ {
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, reqWithCFIP(ip, "/v1/news"))
				if rec.Code != http.StatusOK && rec.Code != http.StatusTooManyRequests {
					t.Errorf("unexpected status %d", rec.Code)
					return
				}
			}
		}(g)
	}
	wg.Wait()
}

// TestSweepConcurrentWithTraffic runs the sweeper alongside live traffic under
// -race to catch lock-ordering / map-access races between the hot path and
// eviction.
func TestSweepConcurrentWithTraffic(t *testing.T) {
	l := New(Config{RPM: 6000, Burst: 100, IdleEviction: time.Millisecond})
	h := l.Middleware(okHandler(new(int64)))

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				l.sweep()
			}
		}
	}()

	for i := 0; i < 2000; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, reqWithCFIP("203.0.113."+strconv.Itoa(i%50), "/v1/news"))
	}
	close(done)
	wg.Wait()
}
