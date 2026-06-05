// Package stream provides a tiny in-process pub/sub hub that fans out quote
// updates to Server-Sent Events subscribers.
package stream

import (
	"sync"

	"github.com/wombow-ai/tickwind/internal/store"
)

// Hub broadcasts quote updates to all current subscribers.
type Hub struct {
	mu   sync.RWMutex
	subs map[chan store.Quote]struct{}
}

// NewHub returns an empty Hub.
func NewHub() *Hub {
	return &Hub{subs: make(map[chan store.Quote]struct{})}
}

// Subscribe registers a new subscriber and returns its update channel plus an
// unsubscribe func that the caller must invoke when done.
func (h *Hub) Subscribe() (<-chan store.Quote, func()) {
	ch := make(chan store.Quote, 16)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs, ch)
			close(ch)
			h.mu.Unlock()
		})
	}
	return ch, unsubscribe
}

// Publish delivers q to every subscriber. Updates for a slow subscriber are
// dropped rather than blocking — the next tick carries the latest price anyway.
func (h *Hub) Publish(q store.Quote) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs {
		select {
		case ch <- q:
		default:
		}
	}
}
