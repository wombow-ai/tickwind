package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/congress"
	"github.com/wombow-ai/tickwind/internal/sec"
	"github.com/wombow-ai/tickwind/internal/store"
)

// briefGenAfterET is the ET hour from which a day's briefing may generate —
// early enough to read before the open, late enough that overnight data is in.
const briefGenAfterET = 7

// Narrow dependency slices so the cache is unit-testable without the world.
type (
	// BriefEnricher is the slice of enrich.Enricher the briefing needs.
	BriefEnricher interface {
		Enabled() bool
		Brief(ctx context.Context, material string) (string, error)
	}
	briefIndices interface{ Indices() []store.IndexQuote }
	briefQuotes  interface{ Snapshot() map[string]store.Quote }
	briefEarner  interface {
		ListEarnings(ctx context.Context, from, to time.Time) ([]store.Earning, error)
	}
	briefCongress interface{ Get() []congress.Filing }
	briefInst     interface{ Get() []sec.OwnershipRef }
)

// BriefingCache generates and holds the day's Chinese pre-market briefing —
// ONE LLM call per day serves every visitor. All material comes from caches
// already in memory (indices, universe, earnings, smart money): zero extra
// upstream requests.
type BriefingCache struct {
	enr      BriefEnricher
	indices  briefIndices
	quotes   briefQuotes
	earnings briefEarner
	congress briefCongress
	inst     briefInst
	log      *slog.Logger

	mu       sync.RWMutex
	date     string // ET day the text belongs to
	text     string
	at       time.Time
	complete bool // material had the [指数] section (caches were warm) → locked for the day
	attempts int  // generations this ET day (bounds incomplete re-tries)
}

// briefMaxAttempts caps generations per day so a missing data source can't
// trigger endless regeneration — a few tries cover the post-restart warm-up.
const briefMaxAttempts = 6

// NewBriefingCache builds the cache; call Run to start the daily generation.
func NewBriefingCache(enr BriefEnricher, idx briefIndices, quotes briefQuotes, earnings briefEarner, cg briefCongress, inst briefInst, log *slog.Logger) *BriefingCache {
	if log == nil {
		log = slog.Default()
	}
	return &BriefingCache{enr: enr, indices: idx, quotes: quotes, earnings: earnings, congress: cg, inst: inst, log: log}
}

// Get returns the current briefing (ok=false before the first generation).
func (b *BriefingCache) Get() (date, text string, at time.Time, ok bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.date, b.text, b.at, b.text != ""
}

// Run checks every 30 minutes (and once at startup) whether today's briefing
// still needs generating, until ctx is cancelled.
func (b *BriefingCache) Run(ctx context.Context) {
	b.maybeGenerate(ctx)
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			b.maybeGenerate(ctx)
		}
	}
}

func etNow() time.Time {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		loc = time.UTC
	}
	return time.Now().In(loc)
}

// maybeGenerate produces the briefing once per ET day, no earlier than
// briefGenAfterET. A failed generation just logs — the next tick retries.
func (b *BriefingCache) maybeGenerate(ctx context.Context) {
	if b.enr == nil || !b.enr.Enabled() {
		return
	}
	now := etNow()
	if now.Hour() < briefGenAfterET {
		return
	}
	day := now.Format("2006-01-02")
	b.mu.Lock()
	if b.date != day {
		b.attempts = 0 // new ET day → reset the retry budget
	}
	if b.date == day && (b.complete || b.attempts >= briefMaxAttempts) {
		b.mu.Unlock()
		return // today's briefing is locked (complete, or out of retries)
	}
	b.mu.Unlock()

	material := b.buildMaterial(ctx, now)
	if strings.TrimSpace(material) == "" {
		return // caches still warming up after a restart — retry next tick
	}
	// On a fresh restart the index/universe caches may not be warm yet; a
	// briefing without the [指数] section is provisional — keep it servable but
	// re-generate (overwrite) on a later tick once the caches fill.
	complete := strings.Contains(material, "[指数]")
	cctx, cancel := context.WithTimeout(ctx, 85*time.Second)
	defer cancel()
	text, err := b.enr.Brief(cctx, material)
	if err != nil {
		b.log.Warn("briefing generation failed", "err", err)
		return
	}
	b.mu.Lock()
	if b.date != day {
		b.attempts = 0
	}
	b.date, b.text, b.at, b.complete = day, text, time.Now().UTC(), complete
	b.attempts++
	b.mu.Unlock()
	b.log.Info("morning briefing generated", "date", day, "chars", len(text), "complete", complete)
}

// buildMaterial assembles the day's structured Chinese source material from
// the in-memory caches. Sections with no data are omitted.
func (b *BriefingCache) buildMaterial(ctx context.Context, now time.Time) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "日期: %s (美东)\n", now.Format("2006-01-02"))

	if b.indices != nil {
		if idx := b.indices.Indices(); len(idx) > 0 {
			sb.WriteString("\n[指数]\n")
			for _, q := range idx {
				if q.PrevClose > 0 {
					chg := (q.Price - q.PrevClose) / q.PrevClose * 100
					fmt.Fprintf(&sb, "- %s: %.2f (%+.2f%%)\n", q.Name, q.Price, chg)
				} else {
					fmt.Fprintf(&sb, "- %s: %.2f\n", q.Name, q.Price)
				}
			}
		}
	}

	if b.quotes != nil {
		type mover struct {
			t   string
			chg float64
		}
		var movers []mover
		for tk, q := range b.quotes.Snapshot() {
			if q.Price <= 1 || q.PrevClose <= 0 {
				continue // sub-$1 noise / unknown reference
			}
			chg := (q.Price - q.PrevClose) / q.PrevClose * 100
			if chg > 300 || chg < -95 {
				continue // delayed-data split artifacts
			}
			movers = append(movers, mover{tk, chg})
		}
		sort.Slice(movers, func(i, j int) bool { return movers[i].chg > movers[j].chg })
		if len(movers) >= 10 {
			sb.WriteString("\n[涨幅前5]\n")
			for _, m := range movers[:5] {
				fmt.Fprintf(&sb, "- %s %+.1f%%\n", m.t, m.chg)
			}
			sb.WriteString("\n[跌幅前5]\n")
			for i := len(movers) - 5; i < len(movers); i++ {
				fmt.Fprintf(&sb, "- %s %+.1f%%\n", movers[i].t, movers[i].chg)
			}
		}
	}

	if b.earnings != nil {
		day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		if es, err := b.earnings.ListEarnings(ctx, day, day.Add(24*time.Hour)); err == nil && len(es) > 0 {
			sb.WriteString("\n[今日财报]\n")
			for i, e := range es {
				if i >= 10 {
					break
				}
				hour := map[string]string{"bmo": "盘前", "amc": "盘后", "dmh": "盘中"}[e.Hour]
				fmt.Fprintf(&sb, "- %s %s\n", e.Ticker, hour)
			}
		}
	}

	smart := false
	if b.congress != nil {
		if fs := b.congress.Get(); len(fs) > 0 {
			sb.WriteString("\n[聪明钱-国会交易披露]\n")
			smart = true
			for i, f := range fs {
				if i >= 3 {
					break
				}
				fmt.Fprintf(&sb, "- 议员 %s (%s) 提交交易披露 %s\n", f.Name, f.State, f.FiledDate.Format("01-02"))
			}
		}
	}
	if b.inst != nil {
		if refs := b.inst.Get(); len(refs) > 0 {
			if !smart {
				sb.WriteString("\n")
			}
			sb.WriteString("[聪明钱-机构举牌]\n")
			for i, ref := range refs {
				if i >= 3 {
					break
				}
				kind := "13G(被动)"
				if ref.Activist {
					kind = "13D(主动)"
				}
				filer := ref.Filer
				if filer == "" {
					filer = "机构"
				}
				fmt.Fprintf(&sb, "- %s 对 %s 提交 %s\n", filer, ref.Company, kind)
			}
		}
	}

	// Indices alone aren't a briefing; require at least one more section.
	if strings.Count(sb.String(), "[") < 2 {
		return ""
	}
	return sb.String()
}
