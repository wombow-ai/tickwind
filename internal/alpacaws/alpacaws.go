// Package alpacaws streams real-time US-equity trades from Alpaca's free IEX
// WebSocket feed and republishes them as live store.Quote updates, giving the
// hot/watchlist set sub-second price updates (vs the slower REST poller cadence).
//
// The WS carries only price+time, so prev-close / regular-close are seeded from a
// REST snapshot and each live trade is overlaid on top. Free tier allows one
// connection and ≤30 symbols, so the subscription set is capped; broader coverage
// stays on the REST poller. Quotes flow to the same SSE hub + store as the poller.
package alpacaws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/wombow-ai/tickwind/internal/store"
)

// MaxSymbols is Alpaca's free-tier subscription cap.
const MaxSymbols = 30

// trade is one incoming trade message (message type "t").
type trade struct {
	Type   string    `json:"T"`
	Symbol string    `json:"S"`
	Price  float64   `json:"p"`
	Time   time.Time `json:"t"`
}

// QuoteSeeder provides REST snapshot quotes to seed prev/regular-close baselines
// (satisfied by *alpaca.Client).
type QuoteSeeder interface {
	SnapshotQuotes(ctx context.Context, symbols []string) (map[string]store.Quote, error)
}

// Streamer maintains the Alpaca IEX WS connection and republishes live quotes.
type Streamer struct {
	url      string
	keyID    string
	secret   string
	symbols  []string
	seeder   QuoteSeeder
	classify func(time.Time) string // session classifier (alpaca.Client.SessionAt)
	publish  func(store.Quote)      // SSE hub publish (may be nil)
	store    store.Store            // for throttled UpsertQuote (may be nil)
	log      *slog.Logger

	mu          sync.Mutex
	seed        map[string]store.Quote
	lastPublish map[string]time.Time
	lastUpsert  map[string]time.Time
}

// New builds a Streamer; the symbol set is capped at MaxSymbols.
func New(wsURL, keyID, secret string, symbols []string, seeder QuoteSeeder, classify func(time.Time) string, publish func(store.Quote), st store.Store, log *slog.Logger) *Streamer {
	if len(symbols) > MaxSymbols {
		symbols = symbols[:MaxSymbols]
	}
	return &Streamer{
		url: wsURL, keyID: keyID, secret: secret, symbols: symbols,
		seeder: seeder, classify: classify, publish: publish, store: st, log: log,
		seed:        make(map[string]store.Quote),
		lastPublish: make(map[string]time.Time),
		lastUpsert:  make(map[string]time.Time),
	}
}

// Run connects, subscribes, and streams until ctx is cancelled, reconnecting with
// capped exponential backoff.
func (s *Streamer) Run(ctx context.Context) {
	if len(s.symbols) == 0 {
		s.log.Info("alpacaws: no symbols — not starting")
		return
	}
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		err := s.session(ctx)
		if ctx.Err() != nil {
			return
		}
		s.log.Warn("alpacaws: session ended; reconnecting", "err", err, "in", backoff.String())
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > 60*time.Second {
			backoff = 60 * time.Second
		}
	}
}

// session runs one full connection lifecycle (dial → auth → subscribe → read).
func (s *Streamer) session(parent context.Context) error {
	s.reseed(parent)

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	dialCtx, dialCancel := context.WithTimeout(ctx, 20*time.Second)
	conn, _, err := websocket.Dial(dialCtx, s.url, nil)
	dialCancel()
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	conn.SetReadLimit(8 << 20)

	if err := s.writeJSON(ctx, conn, map[string]any{"action": "auth", "key": s.keyID, "secret": s.secret}); err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	if err := s.writeJSON(ctx, conn, map[string]any{"action": "subscribe", "trades": s.symbols}); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	s.log.Info("alpacaws: connected + subscribed", "symbols", len(s.symbols))

	// Keepalive: ping periodically; on failure cancel so Read unblocks → reconnect.
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pctx, pcancel := context.WithTimeout(ctx, 10*time.Second)
				err := conn.Ping(pctx)
				pcancel()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		s.handle(ctx, data)
	}
}

func (s *Streamer) writeJSON(ctx context.Context, conn *websocket.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return conn.Write(wctx, websocket.MessageText, b)
}

// handle parses a message batch (Alpaca sends JSON arrays) and republishes trades,
// throttled per symbol (publish ≤2/s, store upsert ≤1/5s).
func (s *Streamer) handle(ctx context.Context, data []byte) {
	trades, note := parseTrades(data)
	if note != "" {
		s.log.Info("alpacaws: server message", "msg", note)
	}
	now := time.Now()
	for _, tr := range trades {
		if tr.Symbol == "" || tr.Price <= 0 {
			continue
		}
		q := s.merge(tr)
		s.mu.Lock()
		pub := now.Sub(s.lastPublish[tr.Symbol]) >= 500*time.Millisecond
		if pub {
			s.lastPublish[tr.Symbol] = now
		}
		ups := now.Sub(s.lastUpsert[tr.Symbol]) >= 5*time.Second
		if ups {
			s.lastUpsert[tr.Symbol] = now
		}
		s.mu.Unlock()
		if pub && s.publish != nil {
			s.publish(q)
		}
		if ups && s.store != nil {
			uctx, ucancel := context.WithTimeout(ctx, 5*time.Second)
			_ = s.store.UpsertQuote(uctx, q)
			ucancel()
		}
	}
}

// merge overlays a live trade onto the seeded quote. During regular hours the
// regular-close tracks the live price; in pre/post it stays the seeded close so
// the extended-hours change references the right baseline.
func (s *Streamer) merge(tr trade) store.Quote {
	s.mu.Lock()
	base := s.seed[tr.Symbol]
	s.mu.Unlock()
	session := s.classify(tr.Time)
	q := store.Quote{
		Ticker:       tr.Symbol,
		Price:        tr.Price,
		PrevClose:    base.PrevClose,
		RegularClose: base.RegularClose,
		Session:      session,
		Source:       "alpaca",
		At:           tr.Time,
	}
	if session == "regular" {
		q.RegularClose = tr.Price
	}
	return q
}

// reseed refreshes prev/regular-close baselines from a REST snapshot (on each
// (re)connect — these change at most daily, so per-connect is enough).
func (s *Streamer) reseed(ctx context.Context) {
	if s.seeder == nil {
		return
	}
	rctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	quotes, err := s.seeder.SnapshotQuotes(rctx, s.symbols)
	if err != nil {
		s.log.Warn("alpacaws: reseed failed", "err", err)
		return
	}
	s.mu.Lock()
	for sym, q := range quotes {
		s.seed[sym] = q
	}
	s.mu.Unlock()
}

// parseTrades extracts trade messages from a batch (Alpaca frames are JSON arrays
// of objects). Non-trade control messages (success/error/subscription) are joined
// into a note string for logging. Pure — unit-tested.
//
// The row struct deliberately carries BOTH the "T" (type) and "t" (timestamp)
// fields: trade messages contain both keys, and encoding/json only does
// case-insensitive fallback when there's no exact-case field — so omitting the
// "t" field would let the timestamp clobber Type. With both present each key
// exact-matches its own field.
func parseTrades(data []byte) ([]trade, string) {
	var rows []struct {
		Type   string    `json:"T"`
		Symbol string    `json:"S"`
		Price  float64   `json:"p"`
		Time   time.Time `json:"t"`
		Msg    string    `json:"msg"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, ""
	}
	var trades []trade
	var notes []string
	for _, r := range rows {
		switch r.Type {
		case "t":
			trades = append(trades, trade{Type: r.Type, Symbol: r.Symbol, Price: r.Price, Time: r.Time})
		case "error", "success", "subscription":
			if r.Msg != "" {
				notes = append(notes, r.Type+":"+r.Msg)
			} else {
				notes = append(notes, r.Type)
			}
		}
	}
	return trades, strings.Join(notes, ", ")
}
