// Package memory is a thread-safe in-memory store.Store implementation.
// It lets Tickwind run with zero infra (no Docker/Postgres) during early dev.
package memory

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

type Store struct {
	mu        sync.RWMutex
	secs      map[string]store.Security
	filings   map[string]map[string]store.Filing  // ticker -> accessionNo -> Filing
	quotes    map[string]store.Quote              // ticker -> latest quote
	news      map[string]map[string]store.News    // ticker -> id -> News
	social    map[string]map[string]store.Post    // ticker -> id -> Post
	signals   map[string]map[string]store.Signal  // ticker -> source -> Signal
	hot       map[string][]store.HotStock         // board -> ranked snapshot
	insiders  map[string]store.InsiderBuy         // accession -> insider buy
	earnings  map[string]store.Earning            // "TICKER|YYYY-MM-DD" -> Earning
	seenF4    map[string]time.Time                // form-4 accession -> filed date
	fearGreed map[string]int                      // "YYYY-MM-DD" -> headline F&G score
	aiSum     map[string][]byte                   // "TICKER|DAY|LANG" -> serialized AI digest payload
	deepRpts  map[string]deepRptRow               // "TICKER|LANG" -> persisted deep-research report (Product A)
	watchlist map[string][]string                 // userID -> ordered tickers
	clips     map[string]map[string]store.Clip    // userID -> clipID -> Clip
	notes     map[string]map[string]store.Note    // userID -> noteID -> Note
	alerts    map[string]map[string]store.Alert   // userID -> alertID -> Alert
	holdings  map[string]map[string]store.Holding // userID -> holdingID -> Holding
	prefs     map[string]json.RawMessage          // userID -> opaque JSON prefs blob
	deepQuota map[string]int                      // "userID|PERIOD" (ET month, e.g. 2026-06) -> deep-research generations used
	chatMsgs  map[string][]store.ChatMessage      // "userID|TICKER" -> ordered chat thread (Product B)
	convs     map[string]store.Conversation       // conversationID -> Conversation (Product C hub)
	chatQuota map[string]int                      // "userID|PERIOD" (ET month) -> Product B chat messages used
	comments  map[string]store.Comment            // commentID -> Comment (public)
	cmtLikes  map[string]map[string]bool          // commentID -> set of userIDs who liked
	subs      map[string]store.Subscription       // userID -> Stripe-synced entitlement
	stripeEv  map[string]bool                     // Stripe webhook event id -> seen (idempotency)
}

func New() *Store {
	return &Store{
		secs:      make(map[string]store.Security),
		filings:   make(map[string]map[string]store.Filing),
		quotes:    make(map[string]store.Quote),
		news:      make(map[string]map[string]store.News),
		social:    make(map[string]map[string]store.Post),
		signals:   make(map[string]map[string]store.Signal),
		hot:       make(map[string][]store.HotStock),
		insiders:  make(map[string]store.InsiderBuy),
		earnings:  make(map[string]store.Earning),
		seenF4:    make(map[string]time.Time),
		fearGreed: make(map[string]int),
		aiSum:     make(map[string][]byte),
		deepRpts:  make(map[string]deepRptRow),
		watchlist: make(map[string][]string),
		clips:     make(map[string]map[string]store.Clip),
		notes:     make(map[string]map[string]store.Note),
		alerts:    make(map[string]map[string]store.Alert),
		holdings:  make(map[string]map[string]store.Holding),
		prefs:     make(map[string]json.RawMessage),
		deepQuota: make(map[string]int),
		chatMsgs:  make(map[string][]store.ChatMessage),
		convs:     make(map[string]store.Conversation),
		chatQuota: make(map[string]int),
		comments:  make(map[string]store.Comment),
		cmtLikes:  make(map[string]map[string]bool),
		subs:      make(map[string]store.Subscription),
		stripeEv:  make(map[string]bool),
	}
}

func key(ticker string) string { return strings.ToUpper(strings.TrimSpace(ticker)) }

// Ping always succeeds — the in-memory store is its own process.
func (s *Store) Ping(_ context.Context) error { return nil }

func (s *Store) UpsertSecurity(_ context.Context, sec store.Security) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secs[key(sec.Ticker)] = sec
	return nil
}

func (s *Store) GetSecurity(_ context.Context, ticker string) (store.Security, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sec, ok := s.secs[key(ticker)]
	return sec, ok, nil
}

func (s *Store) SaveFilings(_ context.Context, ticker string, filings []store.Filing) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(ticker)
	m := s.filings[k]
	if m == nil {
		m = make(map[string]store.Filing)
		s.filings[k] = m
	}
	for _, f := range filings {
		m[f.AccessionNo] = f // dedupe by accession number
	}
	return nil
}

func (s *Store) ListFilings(_ context.Context, ticker string, limit int) ([]store.Filing, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.filings[key(ticker)]
	out := make([]store.Filing, 0, len(m))
	for _, f := range m {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FiledAt.After(out[j].FiledAt) })
	return limited(out, limit), nil
}

func (s *Store) UpsertQuote(_ context.Context, q store.Quote) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quotes[key(q.Ticker)] = q
	return nil
}

func (s *Store) GetQuote(_ context.Context, ticker string) (store.Quote, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q, ok := s.quotes[key(ticker)]
	return q, ok, nil
}

func (s *Store) SaveNews(_ context.Context, ticker string, items []store.News) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(ticker)
	m := s.news[k]
	if m == nil {
		m = make(map[string]store.News)
		s.news[k] = m
	}
	for _, n := range items {
		// Re-saving a known item must not wipe its translation (postgres keeps
		// the column too — its upsert doesn't touch headline_zh).
		if old, ok := m[n.ID]; ok && n.HeadlineZH == "" {
			n.HeadlineZH = old.HeadlineZH
		}
		m[n.ID] = n // dedupe by id
	}
	return nil
}

func (s *Store) ListUntranslatedNews(_ context.Context, limit int) ([]store.News, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]store.News, 0)
	for _, m := range s.news {
		for _, n := range m {
			if n.HeadlineZH == "" && n.Headline != "" {
				out = append(out, n)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Published.After(out[j].Published) })
	return limited(out, limit), nil
}

func (s *Store) SetNewsTranslation(_ context.Context, ticker, id, headlineZH string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m := s.news[key(ticker)]; m != nil {
		if n, ok := m[id]; ok {
			n.HeadlineZH = headlineZH
			m[id] = n
		}
	}
	return nil
}

func (s *Store) ListNews(_ context.Context, ticker string, limit int) ([]store.News, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.news[key(ticker)]
	out := make([]store.News, 0, len(m))
	for _, n := range m {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Published.After(out[j].Published) })
	return limited(out, limit), nil
}

func (s *Store) SaveSocial(_ context.Context, ticker string, posts []store.Post) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(ticker)
	m := s.social[k]
	if m == nil {
		m = make(map[string]store.Post)
		s.social[k] = m
	}
	for _, p := range posts {
		m[p.ID] = p // dedupe by id
	}
	return nil
}

func (s *Store) ListSocial(_ context.Context, ticker string, limit int) ([]store.Post, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.social[key(ticker)]
	out := make([]store.Post, 0, len(m))
	for _, p := range m {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return limited(out, limit), nil
}

func (s *Store) SaveSignals(_ context.Context, signals []store.Signal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sig := range signals {
		k := key(sig.Ticker)
		if k == "" || sig.Source == "" {
			continue
		}
		m := s.signals[k]
		if m == nil {
			m = make(map[string]store.Signal)
			s.signals[k] = m
		}
		m[sig.Source] = sig // one row per (ticker, source)
	}
	return nil
}

func (s *Store) ListSignals(_ context.Context, ticker string) ([]store.Signal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.signals[key(ticker)]
	out := make([]store.Signal, 0, len(m))
	for _, sig := range m {
		out = append(out, sig)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Source < out[j].Source })
	return out, nil
}

func (s *Store) SaveHotList(_ context.Context, board string, stocks []store.HotStock) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hot[board] = append([]store.HotStock(nil), stocks...) // replace this board's snapshot
	return nil
}

func (s *Store) HotList(_ context.Context, board string, limit int) ([]store.HotStock, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]store.HotStock(nil), s.hot[board]...)
	sort.Slice(out, func(i, j int) bool { return out[i].Rank < out[j].Rank })
	return limited(out, limit), nil
}

func (s *Store) SaveInsiderBuys(_ context.Context, buys []store.InsiderBuy) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range buys {
		if b.Accession == "" {
			continue
		}
		s.insiders[b.Accession] = b // upsert by accession
	}
	return nil
}

func (s *Store) RecentInsiderBuys(_ context.Context, since time.Time) ([]store.InsiderBuy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]store.InsiderBuy, 0)
	for _, b := range s.insiders {
		if !b.FiledDate.Before(since) {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FiledDate.After(out[j].FiledDate) })
	return out, nil
}

func earningsKey(ticker string, d time.Time) string {
	return key(ticker) + "|" + d.Format("2006-01-02")
}

func (s *Store) SaveEarnings(_ context.Context, es []store.Earning) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range es {
		if e.Ticker == "" || e.Date.IsZero() {
			continue
		}
		s.earnings[earningsKey(e.Ticker, e.Date)] = e // upsert by (ticker, date)
	}
	return nil
}

func (s *Store) ListEarnings(_ context.Context, from, to time.Time) ([]store.Earning, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]store.Earning, 0)
	for _, e := range s.earnings {
		if !e.Date.Before(from) && !e.Date.After(to) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out, nil
}

func (s *Store) ListEarningsForTicker(_ context.Context, ticker string, limit int) ([]store.Earning, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tk := key(ticker)
	out := make([]store.Earning, 0)
	for _, e := range s.earnings {
		if key(e.Ticker) == tk {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return limited(out, limit), nil
}

func (s *Store) MarkForm4Seen(_ context.Context, accessions []string, filedDate time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range accessions {
		if a != "" {
			s.seenF4[a] = filedDate
		}
	}
	return nil
}

func (s *Store) SeenForm4Since(_ context.Context, since time.Time) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0)
	for a, d := range s.seenF4 {
		if !d.Before(since) {
			out = append(out, a)
		}
	}
	return out, nil
}

func (s *Store) SaveFearGreed(_ context.Context, date string, score int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if date != "" {
		s.fearGreed[date] = score // upsert by date
	}
	return nil
}

func (s *Store) FearGreedHistory(_ context.Context, limit int) ([]store.FearGreedPoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]store.FearGreedPoint, 0, len(s.fearGreed))
	for d, sc := range s.fearGreed {
		out = append(out, store.FearGreedPoint{Date: d, Score: sc})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	// Apply the limit as a tail: keep the most recent `limit` days, still in
	// chronological order.
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

// aiSumKey builds the (ticker, day, lang) map key, mirroring the API's cache key.
func aiSumKey(ticker, day, lang string) string {
	return key(ticker) + "|" + day + "|" + lang
}

// deepRptRow is a persisted deep-research report payload + when it was generated.
type deepRptRow struct {
	payload []byte
	at      time.Time
}

// SaveDeepReport upserts the prose'd deep-research FactSheet for (ticker, lang),
// stamping generated_at now (a defensive copy of the payload is stored).
func (s *Store) SaveDeepReport(_ context.Context, ticker, lang string, payload []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(payload))
	copy(cp, payload)
	s.deepRpts[key(ticker)+"|"+lang] = deepRptRow{payload: cp, at: time.Now().UTC()}
	return nil
}

// GetDeepReport returns the persisted report payload + its generated_at for (ticker,
// lang), or ok=false when there's none.
func (s *Store) GetDeepReport(_ context.Context, ticker, lang string) ([]byte, time.Time, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	row, ok := s.deepRpts[key(ticker)+"|"+lang]
	if !ok {
		return nil, time.Time{}, false, nil
	}
	cp := make([]byte, len(row.payload))
	copy(cp, row.payload)
	return cp, row.at, true, nil
}

// SaveAISummary upserts the serialized AI digest for (ticker, day, lang),
// storing a copy so a later mutation of the caller's slice can't corrupt the map.
func (s *Store) SaveAISummary(_ context.Context, ticker, day, lang string, payload []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(payload))
	copy(cp, payload)
	s.aiSum[aiSumKey(ticker, day, lang)] = cp
	return nil
}

// GetAISummary returns the stored digest payload (a defensive copy) for
// (ticker, day, lang), or ok=false when there's no entry.
func (s *Store) GetAISummary(_ context.Context, ticker, day, lang string) ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	blob, ok := s.aiSum[aiSumKey(ticker, day, lang)]
	if !ok {
		return nil, false, nil
	}
	cp := make([]byte, len(blob))
	copy(cp, blob)
	return cp, true, nil
}

func (s *Store) Watchlist(_ context.Context, userID string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.watchlist[userID]...), nil
}

func (s *Store) AllWatchlistTickers(_ context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := make(map[string]struct{})
	var out []string
	for _, tickers := range s.watchlist {
		for _, t := range tickers {
			if _, ok := seen[t]; !ok {
				seen[t] = struct{}{}
				out = append(out, t)
			}
		}
	}
	return out, nil
}

func (s *Store) AddToWatchlist(_ context.Context, userID, ticker string) error {
	t := key(ticker)
	if t == "" || userID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.watchlist[userID] {
		if x == t {
			return nil
		}
	}
	s.watchlist[userID] = append(s.watchlist[userID], t)
	return nil
}

func (s *Store) RemoveFromWatchlist(_ context.Context, userID, ticker string) error {
	t := key(ticker)
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.watchlist[userID]
	next := make([]string, 0, len(cur))
	for _, x := range cur {
		if x != t {
			next = append(next, x)
		}
	}
	s.watchlist[userID] = next
	return nil
}

func (s *Store) SaveClip(_ context.Context, c store.Clip) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.clips[c.UserID]
	if m == nil {
		m = make(map[string]store.Clip)
		s.clips[c.UserID] = m
	}
	m[c.ID] = c
	return nil
}

func (s *Store) ListClips(_ context.Context, userID, ticker string, limit int) ([]store.Clip, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k := key(ticker)
	out := make([]store.Clip, 0)
	for _, c := range s.clips[userID] {
		if key(c.Ticker) == k {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return limited(out, limit), nil
}

func (s *Store) SaveNote(_ context.Context, n store.Note) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.notes[n.UserID]
	if m == nil {
		m = make(map[string]store.Note)
		s.notes[n.UserID] = m
	}
	m[n.ID] = n
	return nil
}

func (s *Store) ListNotes(_ context.Context, f store.NoteFilter) ([]store.Note, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tk := key(f.Ticker)
	out := make([]store.Note, 0)
	for _, n := range s.notes[f.UserID] {
		if f.Ticker != "" && key(n.Ticker) != tk {
			continue
		}
		if f.From != "" && (n.Date == "" || n.Date < f.From) { // YYYY-MM-DD sorts lexically
			continue
		}
		if f.To != "" && (n.Date == "" || n.Date > f.To) {
			continue
		}
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned // pinned first
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return limited(out, f.Limit), nil
}

func (s *Store) UpdateNote(_ context.Context, userID, id string, body *string, pinned *bool) (store.Note, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.notes[userID][id]
	if !ok {
		return store.Note{}, false, nil
	}
	if body != nil {
		n.Body = *body
	}
	if pinned != nil {
		n.Pinned = *pinned
	}
	n.UpdatedAt = time.Now().UTC()
	s.notes[userID][id] = n
	return n, true, nil
}

func (s *Store) DeleteNote(_ context.Context, userID, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.notes[userID][id]; !ok {
		return false, nil
	}
	delete(s.notes[userID], id)
	return true, nil
}

func (s *Store) SaveAlert(_ context.Context, a store.Alert) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.alerts[a.UserID]
	if m == nil {
		m = make(map[string]store.Alert)
		s.alerts[a.UserID] = m
	}
	m[a.ID] = a
	return nil
}

func (s *Store) ListAlerts(_ context.Context, userID string) ([]store.Alert, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]store.Alert, 0)
	for _, a := range s.alerts[userID] {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) DeleteAlert(_ context.Context, userID, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.alerts[userID][id]; !ok {
		return false, nil
	}
	delete(s.alerts[userID], id)
	return true, nil
}

func (s *Store) ReactivateAlert(_ context.Context, userID, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.alerts[userID][id]
	if !ok {
		return false, nil
	}
	a.Active = true
	a.TriggeredAt = time.Time{} // re-armed
	s.alerts[userID][id] = a
	return true, nil
}

func (s *Store) SaveHolding(_ context.Context, h store.Holding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.holdings[h.UserID]
	if m == nil {
		m = make(map[string]store.Holding)
		s.holdings[h.UserID] = m
	}
	// Upsert by ticker: re-saving a held ticker overwrites it (keep id + created).
	for id, existing := range m {
		if existing.Ticker == h.Ticker {
			h.ID = id
			h.CreatedAt = existing.CreatedAt
			m[id] = h
			return nil
		}
	}
	m[h.ID] = h
	return nil
}

func (s *Store) ListHoldings(_ context.Context, userID string) ([]store.Holding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]store.Holding, 0)
	for _, h := range s.holdings[userID] {
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ticker < out[j].Ticker })
	return out, nil
}

func (s *Store) DeleteHolding(_ context.Context, userID, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.holdings[userID][id]; !ok {
		return false, nil
	}
	delete(s.holdings[userID], id)
	return true, nil
}

// GetPrefs returns the user's stored prefs blob (a defensive copy, so a later
// mutation of the returned slice can't corrupt the map) and ok=false when the
// user has none.
func (s *Store) GetPrefs(_ context.Context, userID string) (json.RawMessage, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	blob, ok := s.prefs[userID]
	if !ok {
		return nil, false, nil
	}
	cp := make(json.RawMessage, len(blob))
	copy(cp, blob)
	return cp, true, nil
}

// PutPrefs overwrites the user's prefs blob. It stores a copy (not the caller's
// slice) so a later mutation by the caller can't leak into the map.
func (s *Store) PutPrefs(_ context.Context, userID string, blob json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make(json.RawMessage, len(blob))
	copy(cp, blob)
	s.prefs[userID] = cp
	return nil
}

// deepQuotaKey is the per-(user, ET month) deep-research quota counter key.
func deepQuotaKey(userID, period string) string { return userID + "|" + period }

// GetDeepQuotaUsed returns how many deep-research generations the user has used
// in the given period (ET month, e.g. "2026-06"); 0 when there's no row.
func (s *Store) GetDeepQuotaUsed(_ context.Context, userID, period string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.deepQuota[deepQuotaKey(userID, period)], nil
}

// IncrDeepQuotaUsed increments the user's deep-research generation count for the
// given period (ET month) by one.
func (s *Store) IncrDeepQuotaUsed(_ context.Context, userID, period string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deepQuota[deepQuotaKey(userID, period)]++
	return nil
}

// chatKey is the per-(user, ticker) implicit-thread key for Product B chat history.
func chatKey(userID, ticker string) string { return userID + "|" + ticker }

// AppendChatMessage appends one turn to the (user, ticker) thread, stamping CreatedAt.
func (s *Store) AppendChatMessage(_ context.Context, m store.ChatMessage) error {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := chatKey(m.UserID, m.Ticker)
	s.chatMsgs[k] = append(s.chatMsgs[k], m)
	return nil
}

// ListChatMessages returns the most recent `limit` messages for the (user, ticker)
// thread in chronological order (oldest first). limit<=0 returns all.
func (s *Store) ListChatMessages(_ context.Context, userID, ticker string, limit int) ([]store.ChatMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := s.chatMsgs[chatKey(userID, ticker)]
	start := 0
	if limit > 0 && len(all) > limit {
		start = len(all) - limit
	}
	out := make([]store.ChatMessage, len(all)-start)
	copy(out, all[start:])
	return out, nil
}

// ClearChatMessages deletes the user's whole (user, ticker) chat thread.
func (s *Store) ClearChatMessages(_ context.Context, userID, ticker string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.chatMsgs, chatKey(userID, ticker))
	return nil
}

// CreateConversation adds a new conversation owned by userID and returns its id.
func (s *Store) CreateConversation(_ context.Context, userID, title, anchorTicker string) (string, error) {
	id := store.NewID()
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.convs[id] = store.Conversation{ID: id, UserID: userID, Title: title, AnchorTicker: anchorTicker, CreatedAt: now, UpdatedAt: now}
	return id, nil
}

// ListConversations returns the user's conversations, newest-updated first.
func (s *Store) ListConversations(_ context.Context, userID string) ([]store.Conversation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []store.Conversation
	for _, c := range s.convs {
		if c.UserID == userID {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

// GetConversation returns the conversation if owned by userID.
func (s *Store) GetConversation(_ context.Context, userID, id string) (store.Conversation, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.convs[id]
	if !ok || c.UserID != userID {
		return store.Conversation{}, false, nil
	}
	return c, true, nil
}

// RenameConversation sets the title (no-op if not owned).
func (s *Store) RenameConversation(_ context.Context, userID, id, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.convs[id]; ok && c.UserID == userID {
		c.Title = title
		c.UpdatedAt = time.Now().UTC()
		s.convs[id] = c
	}
	return nil
}

// DeleteConversation removes the conversation (no-op if not owned).
func (s *Store) DeleteConversation(_ context.Context, userID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.convs[id]; ok && c.UserID == userID {
		delete(s.convs, id)
	}
	return nil
}

// GetChatMsgUsed returns the user's Product B chat-message count for the period (ET
// month); 0 when there's no row. (Reuses the deepQuotaKey shape.)
func (s *Store) GetChatMsgUsed(_ context.Context, userID, period string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.chatQuota[deepQuotaKey(userID, period)], nil
}

// IncrChatMsgUsed increments the user's Product B chat-message count for the period
// (ET month) by one.
func (s *Store) IncrChatMsgUsed(_ context.Context, userID, period string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chatQuota[deepQuotaKey(userID, period)]++
	return nil
}

// GetSubscription returns the user's Stripe-synced entitlement (found=false when none).
func (s *Store) GetSubscription(_ context.Context, userID string) (store.Subscription, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sub, ok := s.subs[userID]
	return sub, ok, nil
}

// GetSubscriptionByCustomer scans for the entitlement matching a Stripe customer id.
func (s *Store) GetSubscriptionByCustomer(_ context.Context, customerID string) (store.Subscription, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sub := range s.subs {
		if sub.StripeCustomerID == customerID && customerID != "" {
			return sub, true, nil
		}
	}
	return store.Subscription{}, false, nil
}

// UpsertSubscription writes the full entitlement row, keyed by user_id.
func (s *Store) UpsertSubscription(_ context.Context, sub store.Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub.UpdatedAt = time.Now()
	if sub.CurrentPeriodEnd.IsZero() {
		sub.CurrentPeriodEnd = time.Now()
	}
	s.subs[sub.UserID] = sub
	return nil
}

// MarkStripeEventSeen records a webhook event id; fresh=true the first time, false if seen.
func (s *Store) MarkStripeEventSeen(_ context.Context, eventID, _ string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stripeEv[eventID] {
		return false, nil
	}
	s.stripeEv[eventID] = true
	return true, nil
}

func (s *Store) ListActiveAlerts(_ context.Context) ([]store.Alert, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]store.Alert, 0)
	for _, m := range s.alerts {
		for _, a := range m {
			if a.Active && a.TriggeredAt.IsZero() {
				out = append(out, a)
			}
		}
	}
	return out, nil
}

func (s *Store) MarkAlertTriggered(_ context.Context, id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.alerts {
		if a, ok := m[id]; ok {
			a.TriggeredAt = at
			m[id] = a
			return nil
		}
	}
	return nil
}

func (s *Store) SaveComment(_ context.Context, c store.Comment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.comments[c.ID] = c
	return nil
}

func (s *Store) ListComments(_ context.Context, ticker string, limit int, viewerID string) ([]store.Comment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tk := key(ticker) // "" stays "" → matches the global board
	out := make([]store.Comment, 0)
	for _, c := range s.comments {
		// A stock's list = comments posted on it ∪ comments that cashtag it.
		match := key(c.Ticker) == tk
		if !match && tk != "" {
			for _, m := range c.Mentions {
				if m == tk {
					match = true
					break
				}
			}
		}
		if match {
			likes := s.cmtLikes[c.ID]
			c.Likes = len(likes)
			c.Liked = viewerID != "" && likes[viewerID]
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return limited(out, limit), nil
}

func (s *Store) DeleteComment(_ context.Context, id, userID string, admin bool) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.comments[id]
	if !ok || (!admin && c.UserID != userID) {
		return false, nil
	}
	delete(s.comments, id) // memory hard-deletes; postgres soft-deletes for audit
	return true, nil
}

func (s *Store) ReportComment(_ context.Context, id string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.comments[id]
	return ok, nil
}

func (s *Store) UpdateComment(_ context.Context, id, userID, body string, mentions []string) (store.Comment, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.comments[id]
	if !ok || c.UserID != userID { // only the author may edit
		return store.Comment{}, false, nil
	}
	now := time.Now().UTC()
	c.Body = body
	c.EditedAt = &now
	c.Mentions = mentions // the edited body's cashtags replace the old set
	s.comments[id] = c
	return c, true, nil
}

func (s *Store) LikeComment(_ context.Context, id, userID string) (bool, int, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.comments[id]; !ok {
		return false, 0, false, nil
	}
	set := s.cmtLikes[id]
	if set == nil {
		set = make(map[string]bool)
		s.cmtLikes[id] = set
	}
	liked := !set[userID]
	if liked {
		set[userID] = true // toggle on
	} else {
		delete(set, userID) // toggle off
	}
	return liked, len(set), true, nil
}

// limited returns the first limit elements (limit <= 0 means all).
func limited[T any](s []T, limit int) []T {
	if limit > 0 && len(s) > limit {
		return s[:limit]
	}
	return s
}
