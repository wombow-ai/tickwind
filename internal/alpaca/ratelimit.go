package alpaca

import (
	"context"
	"math"
	"sync"
	"time"
)

// alpacaRatePerSec / alpacaBurst bound outbound requests to Alpaca's free market-data tier (200
// req/min). A sustained 2.5/s (150/min) leaves headroom under the cap; a burst of 15 lets a spike of
// on-demand (user-facing) fetches through immediately while the CONTINUOUS background scans
// (signals/scorecard/relative-strength/earnings-reaction over the broadened analytic universe) queue
// at the sustained rate — so a multi-scan cold-start can no longer storm Alpaca into 429s (which were
// negative-cached and left the leaderboards thin).
const (
	alpacaRatePerSec = 2.5
	alpacaBurst      = 15
)

// tokenBucket is a minimal token-bucket rate limiter (stdlib-only; no golang.org/x/time/rate dep):
// tokens accrue at perSec up to `max`, and Wait blocks until one is available or ctx is cancelled.
// Refill is computed from elapsed time on each call, so it self-corrects and needs no goroutine.
type tokenBucket struct {
	mu     sync.Mutex
	tokens float64
	max    float64
	perSec float64
	last   time.Time
}

func newTokenBucket(perSec, burst float64) *tokenBucket {
	if burst < 1 {
		burst = 1 // capacity < 1 can never accumulate a whole token → Wait would loop forever
	}
	return &tokenBucket{tokens: burst, max: burst, perSec: perSec, last: time.Now()}
}

// Wait blocks until a token is available or ctx is done. Callers pass their LONG-LIVED ctx (the scan
// / request context) — never a short per-HTTP-request deadline — so queuing under load doesn't
// manifest as request timeouts.
func (b *tokenBucket) Wait(ctx context.Context) error {
	for {
		b.mu.Lock()
		now := time.Now()
		b.tokens = math.Min(b.max, b.tokens+now.Sub(b.last).Seconds()*b.perSec)
		b.last = now
		if b.tokens >= 1 {
			b.tokens--
			b.mu.Unlock()
			return nil
		}
		need := (1 - b.tokens) / b.perSec
		b.mu.Unlock()
		t := time.NewTimer(time.Duration(need * float64(time.Second)))
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}
