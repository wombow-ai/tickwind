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

// MaxSymbols is Alpaca's free-tier subscription cap. viewedSlots of those are
// reserved for actively-viewed (non-base) tickers; the rest is the pinned base.
const (
	MaxSymbols  = 30
	viewedSlots = 10
)

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
	base     []string // pinned base set (watchlist∪popular), always subscribed
	seeder   QuoteSeeder
	classify func(time.Time) string // session classifier (alpaca.Client.SessionAt)
	publish  func(store.Quote)      // SSE hub publish (may be nil)
	store    store.Store            // for throttled UpsertQuote (may be nil)
	log      *slog.Logger

	mu          sync.Mutex
	seed        map[string]store.Quote
	lastPublish map[string]time.Time
	lastUpsert  map[string]time.Time

	submu    sync.Mutex    // guards viewed
	viewed   []string      // LRU of actively-viewed tickers (most-recent last), disjoint from base
	resyncCh chan struct{} // nudges the writer goroutine to re-diff subscriptions
}

// New builds a Streamer; the pinned base set is capped at MaxSymbols-viewedSlots,
// leaving room for actively-viewed tickers added via Subscribe.
func New(wsURL, keyID, secret string, base []string, seeder QuoteSeeder, classify func(time.Time) string, publish func(store.Quote), st store.Store, log *slog.Logger) *Streamer {
	return &Streamer{
		url: wsURL, keyID: keyID, secret: secret, base: capBase(base),
		seeder: seeder, classify: classify, publish: publish, store: st, log: log,
		seed:        make(map[string]store.Quote),
		lastPublish: make(map[string]time.Time),
		lastUpsert:  make(map[string]time.Time),
		resyncCh:    make(chan struct{}, 1),
	}
}

// capBase trims the pinned base set to MaxSymbols-viewedSlots, leaving room for
// actively-viewed tickers. Order is preserved (callers front-load the most
// important symbols, e.g. POPULAR_TICKERS).
func capBase(base []string) []string {
	if lim := MaxSymbols - viewedSlots; len(base) > lim {
		return base[:lim]
	}
	return base
}

// RefreshBase replaces the pinned base set (e.g. as watchlists change after boot)
// so the real-time stream isn't frozen to the startup snapshot. Capped like New;
// updates s.base under submu and nudges a resync (so the writer re-diffs the wire
// subscriptions) only when the set actually changed. Safe for concurrent use.
func (s *Streamer) RefreshBase(base []string) {
	nb := capBase(base)
	s.submu.Lock()
	same := len(nb) == len(s.base)
	for i := 0; same && i < len(nb); i++ {
		if nb[i] != s.base[i] {
			same = false
		}
	}
	if !same {
		s.base = nb
	}
	s.submu.Unlock()
	if !same {
		select {
		case s.resyncCh <- struct{}{}:
		default:
		}
	}
}

// Subscribe marks ticker as actively viewed so it joins the live stream (within
// the free-tier cap, evicting the least-recently-viewed). No-op for blank /
// non-US / already-base tickers. Safe for concurrent use (called from handlers).
func (s *Streamer) Subscribe(ticker string) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	if ticker == "" || isForeignSuffix(ticker) {
		return
	}
	s.submu.Lock()
	for _, b := range s.base {
		if b == ticker {
			s.submu.Unlock()
			return // already pinned
		}
	}
	maxViewed := MaxSymbols - len(s.base)
	if maxViewed < 1 {
		s.submu.Unlock()
		return
	}
	before := len(s.viewed)
	s.viewed = lruAdd(s.viewed, ticker, maxViewed)
	// Only nudge a resync when the set actually changed (avoid needless WS churn).
	changed := before != len(s.viewed) || (len(s.viewed) > 0 && s.viewed[len(s.viewed)-1] == ticker)
	s.submu.Unlock()
	if changed {
		select {
		case s.resyncCh <- struct{}{}:
		default:
		}
	}
}

// desired returns the full set to be subscribed (base ∪ viewed, disjoint).
func (s *Streamer) desired() []string {
	s.submu.Lock()
	defer s.submu.Unlock()
	out := make([]string, 0, len(s.base)+len(s.viewed))
	out = append(out, s.base...)
	out = append(out, s.viewed...)
	return out
}

// lruAdd moves ticker to most-recent (end), dropping any prior occurrence, and
// trims the oldest (front) so the slice is at most max long. Pure — unit-tested.
func lruAdd(lru []string, ticker string, max int) []string {
	out := make([]string, 0, len(lru)+1)
	for _, t := range lru {
		if t != ticker {
			out = append(out, t)
		}
	}
	out = append(out, ticker)
	if max > 0 && len(out) > max {
		out = out[len(out)-max:]
	}
	return out
}

// isForeignSuffix reports whether a ticker is non-US (Alpaca IEX is US-only).
func isForeignSuffix(t string) bool {
	for _, sfx := range []string{".HK", ".TW", ".TWO", ".KS", ".KQ"} {
		if strings.HasSuffix(t, sfx) {
			return true
		}
	}
	return false
}

// Run connects, subscribes, and streams until ctx is cancelled, reconnecting with
// capped exponential backoff.
func (s *Streamer) Run(ctx context.Context) {
	if len(s.desired()) == 0 { // submu-guarded read (base is refreshed concurrently)
		s.log.Info("alpacaws: no base symbols — not starting")
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
	s.reseed(parent, s.desired())

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

	// The writer goroutine owns ALL WS writes (auth, (un)subscribe, ping) —
	// coder/websocket forbids concurrent writes, and Subscribe() fires from
	// request goroutines. Read happens here; Read+Write concurrency is allowed.
	go func() {
		if err := s.writer(ctx, conn); err != nil && ctx.Err() == nil {
			s.log.Warn("alpacaws: writer ended", "err", err)
			cancel() // unblock Read → reconnect
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

// writer authenticates, subscribes the desired set, then keeps subscriptions in
// sync (on resync nudges) and the connection alive (ping). Sole WS writer.
func (s *Streamer) writer(ctx context.Context, conn *websocket.Conn) error {
	if err := s.writeJSON(ctx, conn, map[string]any{"action": "auth", "key": s.keyID, "secret": s.secret}); err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	subscribed := make(map[string]bool)
	if err := s.sync(ctx, conn, subscribed); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	s.log.Info("alpacaws: connected + subscribed", "base", len(s.base), "subscribed", len(subscribed))

	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.resyncCh:
			if err := s.sync(ctx, conn, subscribed); err != nil {
				return err
			}
		case <-ping.C:
			pctx, pcancel := context.WithTimeout(ctx, 10*time.Second)
			err := conn.Ping(pctx)
			pcancel()
			if err != nil {
				return fmt.Errorf("ping: %w", err)
			}
		}
	}
}

// sync diffs the desired subscription set against what's on the wire and sends
// the necessary subscribe/unsubscribe messages (owned by the writer goroutine).
func (s *Streamer) sync(ctx context.Context, conn *websocket.Conn, subscribed map[string]bool) error {
	want := make(map[string]bool)
	for _, t := range s.desired() {
		want[t] = true
	}
	var add, rem []string
	for t := range want {
		if !subscribed[t] {
			add = append(add, t)
		}
	}
	for t := range subscribed {
		if !want[t] {
			rem = append(rem, t)
		}
	}
	if len(rem) > 0 {
		if err := s.writeJSON(ctx, conn, map[string]any{"action": "unsubscribe", "trades": rem}); err != nil {
			return err
		}
		for _, t := range rem {
			delete(subscribed, t)
		}
	}
	if len(add) > 0 {
		s.reseed(ctx, add) // seed prev/regular-close before the live price streams in
		if err := s.writeJSON(ctx, conn, map[string]any{"action": "subscribe", "trades": add}); err != nil {
			return err
		}
		for _, t := range add {
			subscribed[t] = true
		}
	}
	return nil
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

// reseed refreshes prev/regular-close baselines for the given symbols from a REST
// snapshot — called for the full set on (re)connect and for newly-viewed tickers
// as they're added (so a freshly-streamed ticker has a correct day-change base).
func (s *Streamer) reseed(ctx context.Context, symbols []string) {
	if s.seeder == nil || len(symbols) == 0 {
		return
	}
	rctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	quotes, err := s.seeder.SnapshotQuotes(rctx, symbols)
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
