package alpaca

import (
	"context"
	"testing"
	"time"
)

func TestTokenBucket(t *testing.T) {
	// Burst tokens are available immediately (no meaningful wait).
	b := newTokenBucket(1000, 3) // fast refill, burst 3
	ctx := context.Background()
	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := b.Wait(ctx); err != nil {
			t.Fatalf("burst Wait %d: %v", i, err)
		}
	}
	if d := time.Since(start); d > 25*time.Millisecond {
		t.Fatalf("burst of 3 should be ~instant, took %v", d)
	}

	// A cancelled ctx makes Wait return promptly with the ctx error while it would otherwise block.
	slow := newTokenBucket(0.0001, 1) // ~never refills
	if err := slow.Wait(context.Background()); err != nil {
		t.Fatalf("draining the initial token: %v", err)
	}
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := slow.Wait(cctx); err == nil { // no token left → blocks → ctx cancelled → error
		t.Fatal("cancelled ctx should make Wait return an error")
	}

	// Sustained rate: capacity 1 at 200/s — the FIRST token is instant (initial), the NEXT is paced
	// (~5ms). Also exercises the burst<1 clamp (newTokenBucket raises 0→1 so Wait can't loop forever).
	paced := newTokenBucket(200, 1)
	if err := paced.Wait(context.Background()); err != nil { // consume the initial token
		t.Fatalf("paced initial Wait: %v", err)
	}
	s2 := time.Now()
	if err := paced.Wait(context.Background()); err != nil { // this one waits for a refill
		t.Fatalf("paced Wait: %v", err)
	}
	if d := time.Since(s2); d < 2*time.Millisecond {
		t.Fatalf("the second token (capacity 1) should be paced (~5ms), took %v", d)
	}
}
