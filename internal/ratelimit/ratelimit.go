// Package ratelimit provides an in-memory, concurrency-safe per-client-IP rate
// limiter for the public HTTP API. It defends a small VPS against scraping/bot
// abuse (a single IP flooding the API) while staying generous enough that a
// heavy but legitimate page load — which fans out into dozens of requests for
// news/social/bars/quotes — is never throttled.
//
// The limiter is a classic token bucket per client IP: each IP refills at a
// steady rate (requests-per-minute) up to a burst capacity. Buckets are kept in
// a sharded map (to spread lock contention) and a background sweeper evicts IPs
// that have been idle long enough to have fully refilled, so memory does not
// leak as the set of seen IPs grows.
//
// Design rules baked in:
//   - The REAL client IP comes from Cloudflare's CF-Connecting-IP header (the
//     API sits behind a Cloudflare Tunnel, so RemoteAddr is the loopback/tunnel
//     hop, not the visitor). It falls back to the first X-Forwarded-For hop and
//     finally RemoteAddr.
//   - Fail-open: any internal doubt (no resolvable IP, etc.) ALLOWS the request.
//     The limiter must never break legitimate traffic because of its own state.
//   - Exempt paths (uptime probes, the long-lived SSE stream) bypass the limiter
//     entirely and are never counted.
package ratelimit

import (
	"encoding/json"
	"hash/fnv"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// shardCount is the number of independent bucket maps. Spreading IPs across
// several mutexes keeps lock contention low under load. A power of two keeps the
// hash→shard mapping a cheap mask.
const shardCount = 16

// Config tunes a Limiter. RPM is the sustained per-IP allowance (tokens refilled
// per minute); Burst is the bucket capacity (the most requests an IP may make in
// an instantaneous burst before it must wait for refill). Sensible defaults are
// applied for any non-positive field, so a zero Config is usable.
type Config struct {
	// RPM is the sustained requests-per-minute allowed per client IP.
	RPM int
	// Burst is the token-bucket capacity — the largest burst an idle IP may
	// spend at once. Should comfortably exceed a single page load's fan-out.
	Burst int
	// IdleEviction is how long a bucket may sit untouched before the sweeper
	// removes it (memory hygiene). A bucket that has fully refilled carries no
	// state worth keeping. Defaults to 10 minutes.
	IdleEviction time.Duration
	// SweepEvery is how often the background sweeper runs. Defaults to 5 minutes.
	SweepEvery time.Duration
	// Exempt reports whether a request path bypasses the limiter entirely
	// (uptime probes, the SSE stream, …). It is called on every request; keep it
	// cheap. A nil Exempt means "limit everything".
	Exempt func(path string) bool
	// Logger receives sampled 429 logs so the limits can be tuned. A nil Logger
	// disables logging (the limiter still functions).
	Logger *slog.Logger
}

const (
	defaultRPM          = 300
	defaultBurst        = 60
	defaultIdleEviction = 10 * time.Minute
	defaultSweepEvery   = 5 * time.Minute
)

// bucket is a single IP's token bucket. tokens is a float so fractional refill
// accumulates correctly between requests. It is guarded by its shard's mutex.
type bucket struct {
	tokens float64   // current tokens available (0..burst)
	last   time.Time // last time tokens were refilled
}

type shard struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

// Limiter is a concurrency-safe per-IP token-bucket rate limiter. Construct it
// with New; use Middleware to wrap an http.Handler. The zero value is not
// usable — always go through New.
type Limiter struct {
	rps      float64 // refill rate, tokens per second (RPM/60)
	burst    float64
	idleEv   time.Duration
	sweepInt time.Duration
	exempt   func(path string) bool
	log      *slog.Logger

	shards [shardCount]shard

	// now is the clock, swappable in tests; defaults to time.Now.
	now func() time.Time

	// 429 log sampling: at most one log line per logEvery, counting suppressed
	// rejections so the periodic line can report the true volume.
	logMu       sync.Mutex
	logSuppress int
	logNext     time.Time
}

// logEvery bounds how often a 429 is logged, so a flood does not itself flood
// the logs. The suppressed count is reported on the next emitted line.
const logEvery = 10 * time.Second

// New builds a Limiter from cfg, applying defaults for any non-positive field,
// and starts the background eviction sweeper (it runs for the process lifetime).
func New(cfg Config) *Limiter {
	rpm := cfg.RPM
	if rpm <= 0 {
		rpm = defaultRPM
	}
	burst := cfg.Burst
	if burst <= 0 {
		burst = defaultBurst
	}
	idleEv := cfg.IdleEviction
	if idleEv <= 0 {
		idleEv = defaultIdleEviction
	}
	sweepInt := cfg.SweepEvery
	if sweepInt <= 0 {
		sweepInt = defaultSweepEvery
	}

	l := &Limiter{
		rps:      float64(rpm) / 60.0,
		burst:    float64(burst),
		idleEv:   idleEv,
		sweepInt: sweepInt,
		exempt:   cfg.Exempt,
		log:      cfg.Logger,
		now:      time.Now,
	}
	for i := range l.shards {
		l.shards[i].buckets = make(map[string]*bucket)
	}
	go l.sweepLoop()
	return l
}

// Middleware returns an http.Handler that rate-limits next on a per-client-IP
// basis. Exempt paths and requests with no resolvable client IP pass straight
// through (fail-open). On exceed it replies 429 with a Retry-After header and a
// small JSON body, and does NOT call next.
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Exempt infra paths (uptime probes, the long-lived SSE stream): never
		// limited, never counted.
		if l.exempt != nil && l.exempt(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		ip := ClientIP(r)
		if ip == "" {
			// Fail-open: with no client IP to key on we cannot fairly limit, so
			// we allow rather than risk blocking legitimate traffic.
			next.ServeHTTP(w, r)
			return
		}

		if l.allow(ip) {
			next.ServeHTTP(w, r)
			return
		}

		l.reject(w, ip)
	})
}

// allow consumes one token for ip, refilling the bucket for elapsed time first.
// It reports whether a token was available. This is the hot path; it touches
// only the IP's shard.
func (l *Limiter) allow(ip string) bool {
	now := l.now()
	sh := &l.shards[l.shardIndex(ip)]

	sh.mu.Lock()
	defer sh.mu.Unlock()

	b := sh.buckets[ip]
	if b == nil {
		// First sighting: full bucket, spend one token.
		sh.buckets[ip] = &bucket{tokens: l.burst - 1, last: now}
		return true
	}

	// Refill for the elapsed time, capped at burst.
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * l.rps
		if b.tokens > l.burst {
			b.tokens = l.burst
		}
		b.last = now
	}

	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// reject writes the 429 response (Retry-After + small JSON body) and records a
// sampled log line so the limits can be tuned without flooding the log.
func (l *Limiter) reject(w http.ResponseWriter, ip string) {
	// Retry-After (seconds): time to accrue one token at the refill rate, at
	// least 1s. Communicates a sane back-off to well-behaved clients.
	retry := 1
	if l.rps > 0 {
		if s := int(1.0/l.rps + 0.999); s > retry {
			retry = s
		}
	}
	w.Header().Set("Retry-After", strconv.Itoa(retry))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":       "rate limit exceeded",
		"retry_after": retry,
	})
	l.sampleLog(ip)
}

// sampleLog emits at most one warning per logEvery, reporting how many further
// rejections were suppressed since the last line (so a flood is visible without
// drowning the log).
func (l *Limiter) sampleLog(ip string) {
	if l.log == nil {
		return
	}
	now := l.now()
	l.logMu.Lock()
	if now.Before(l.logNext) {
		l.logSuppress++
		l.logMu.Unlock()
		return
	}
	suppressed := l.logSuppress
	l.logSuppress = 0
	l.logNext = now.Add(logEvery)
	l.logMu.Unlock()

	l.log.Warn("rate limit exceeded", "ip", ip, "suppressed_since_last", suppressed)
}

// shardIndex maps an IP string to one of the shards via FNV-1a, masked to the
// (power-of-two) shard count.
func (l *Limiter) shardIndex(ip string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(ip))
	return int(h.Sum32() & (shardCount - 1))
}

// sweepLoop periodically evicts idle buckets so the map does not grow without
// bound as new IPs are seen. It runs for the process lifetime.
func (l *Limiter) sweepLoop() {
	t := time.NewTicker(l.sweepInt)
	defer t.Stop()
	for range t.C {
		l.sweep()
	}
}

// sweep removes buckets untouched for longer than idleEv. A bucket that has been
// idle that long has fully refilled, so dropping it loses no state (a returning
// IP simply gets a fresh full bucket).
func (l *Limiter) sweep() {
	cutoff := l.now().Add(-l.idleEv)
	for i := range l.shards {
		sh := &l.shards[i]
		sh.mu.Lock()
		for ip, b := range sh.buckets {
			if b.last.Before(cutoff) {
				delete(sh.buckets, ip)
			}
		}
		sh.mu.Unlock()
	}
}

// size reports the total number of tracked buckets across all shards (test
// helper; not part of the public surface beyond observability).
func (l *Limiter) size() int {
	n := 0
	for i := range l.shards {
		sh := &l.shards[i]
		sh.mu.Lock()
		n += len(sh.buckets)
		sh.mu.Unlock()
	}
	return n
}

// ClientIP resolves the real client IP for a request behind a Cloudflare Tunnel.
// Order: CF-Connecting-IP (set by Cloudflare, the authoritative visitor IP) →
// first hop of X-Forwarded-For → RemoteAddr (host part). It returns "" only when
// nothing resolves, which the middleware treats as fail-open. Values are
// validated as IPs where possible so a spoofed/garbage header cannot create an
// odd bucket key, but a non-IP CF/XFF value is still preferred over RemoteAddr
// to avoid keying every visitor onto the shared tunnel address.
func ClientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); ip != "" {
		return normalizeIP(ip)
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first := xff
		if i := strings.IndexByte(xff, ','); i >= 0 {
			first = xff[:i]
		}
		if ip := strings.TrimSpace(first); ip != "" {
			return normalizeIP(ip)
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

// normalizeIP returns the canonical form of a parseable IP, or the trimmed input
// unchanged when it does not parse (so a malformed header still yields a stable,
// distinct key rather than collapsing onto another IP's bucket).
func normalizeIP(s string) string {
	if ip := net.ParseIP(s); ip != nil {
		return ip.String()
	}
	return s
}
