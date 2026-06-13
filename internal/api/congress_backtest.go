package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/wombow-ai/tickwind/internal/congressbt"
	"github.com/wombow-ai/tickwind/internal/store"
)

// backtestEntry is a cached follow-trade simulation result, keyed by member slug,
// tagged with the UTC day it was computed for (recomputed when the day rolls over
// or new prices arrive).
type backtestEntry struct {
	day string // YYYY-MM-DD this result was computed for
	bt  congressbt.Backtest
}

// backtestCandleDeadline bounds the per-ticker price fetches for one backtest so
// a slow/unreachable price source can't hang the request indefinitely. The
// simulation degrades gracefully — a ticker that times out is simply skipped.
const backtestCandleDeadline = 20 * time.Second

// getCongressBacktest serves the follow-trade SIMULATION for one member:
// equal-weight virtual buys at each disclosed purchase's disclosure-date close,
// held (or sold on a disclosed sale) to today, vs. an equal-dollar SPY
// buy-and-hold benchmark. See internal/congressbt for the full methodology.
//
// Always 200. When the member is unknown, the price source is unavailable, or
// there isn't enough priced buy history, the body carries insufficient:true (and
// the available coverage) rather than an error status — the frontend shows a
// "not enough data to simulate" notice. Results are cached per slug per UTC day.
func (s *Server) getCongressBacktest(w http.ResponseWriter, r *http.Request) {
	slug := strings.ToLower(strings.TrimSpace(r.PathValue("slug")))

	// No member data or no price source → nothing to simulate (but still 200 with
	// the insufficient shape, never a 500).
	if s.congressTx == nil || s.bars == nil {
		writeJSON(w, http.StatusOK, backtestResponse(slug, "", congressbt.Backtest{Insufficient: true}))
		return
	}
	m, ok := s.congressTx.ByMember(slug)
	if !ok || len(m.Transactions) == 0 {
		writeJSON(w, http.StatusOK, backtestResponse(slug, m.Name, congressbt.Backtest{Insufficient: true}))
		return
	}

	day := time.Now().UTC().Format("2006-01-02")
	s.btMu.Lock()
	if e, hit := s.btCache[slug]; hit && e.day == day {
		s.btMu.Unlock()
		writeJSON(w, http.StatusOK, backtestResponse(m.Slug, m.Name, e.bt))
		return
	}
	s.btMu.Unlock()

	// closes injects daily-close history per ticker via the BarSource (the same
	// cached source the candlestick chart uses). A per-ticker fetch error yields
	// nil candles → that buy is skipped + counted, never failed.
	ctx, cancel := context.WithTimeout(context.Background(), backtestCandleDeadline)
	defer cancel()
	closes := func(ticker string) []store.Candle {
		cs, err := s.bars.DailyCandles(ctx, ticker)
		if err != nil {
			s.log.Debug("backtest candles fetch failed", "slug", slug, "ticker", ticker, "err", err)
			return nil
		}
		return cs
	}

	bt := congressbt.Run(m.Transactions, closes, time.Now())

	s.btMu.Lock()
	s.btCache[slug] = backtestEntry{day: day, bt: bt}
	s.btMu.Unlock()

	writeJSON(w, http.StatusOK, backtestResponse(m.Slug, m.Name, bt))
}

// backtestResponse wraps the simulation with the member identity for the
// frontend (the member-page section header reuses these).
func backtestResponse(slug, name string, bt congressbt.Backtest) map[string]any {
	return map[string]any{
		"slug":     slug,
		"name":     name,
		"backtest": bt,
	}
}
