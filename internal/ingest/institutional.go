package ingest

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/wombow-ai/tickwind/internal/institutional"
	"github.com/wombow-ai/tickwind/internal/sec"
)

// OwnershipFetcher fetches Schedule 13D/13G filings for a day and resolves the
// reporting institution for a filing (satisfied by *sec.Client).
type OwnershipFetcher interface {
	DailyBeneficialOwnership(ctx context.Context, date time.Time) ([]sec.OwnershipRef, error)
	FetchFiler(ctx context.Context, ref sec.OwnershipRef) (string, error)
}

// InstitutionalIngestor refreshes the in-memory cache of recent Schedule 13D/13G
// beneficial-ownership filings from the SEC daily index, scanning the last few
// days and deduping by accession. Own goroutine, off the request path;
// memory-only + rebuildable (no DB writes), keyless (public SEC data).
type InstitutionalIngestor struct {
	sec   OwnershipFetcher
	cache *institutional.Cache
	every time.Duration
	max   int
	log   *slog.Logger
}

// institutionalSweepDays is how many calendar days back the refresh scans the SEC daily index.
// It must comfortably clear a holiday CLUSTER (e.g. a Thu/Fri federal holiday + the weekend + a
// not-yet-disseminated Monday) so the board never empties when no business day with filings happens
// to fall in a short window — the 2026 Juneteenth weekend (06-19 holiday → 06-20/21 weekend) left a
// 4-day window with zero 13D/13G rows while 06-18 had 84. 10 days spans any single such cluster; the
// cache keeps-last-good across emptier refreshes and caps at `max` newest, so a wider scan only adds a
// few cheap daily-index fetches.
const institutionalSweepDays = 10

// NewInstitutionalIngestor builds the ingestor; every is the refresh cadence.
func NewInstitutionalIngestor(secClient OwnershipFetcher, cache *institutional.Cache, every time.Duration, log *slog.Logger) *InstitutionalIngestor {
	return &InstitutionalIngestor{sec: secClient, cache: cache, every: every, max: 150, log: log}
}

// Run refreshes once on startup, then on the cadence, until ctx is cancelled.
func (i *InstitutionalIngestor) Run(ctx context.Context) {
	i.refresh(ctx)
	t := time.NewTicker(i.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			i.refresh(ctx)
		}
	}
}

func (i *InstitutionalIngestor) refresh(ctx context.Context) {
	now := time.Now().UTC()
	seen := make(map[string]struct{})
	var all []sec.OwnershipRef
	// Filings disseminate the next business day; scan a window of calendar days wide enough to clear a
	// holiday cluster (weekend/holiday days simply return nothing) and dedupe by accession.
	for d := 0; d < institutionalSweepDays; d++ {
		refs, err := i.sec.DailyBeneficialOwnership(ctx, now.AddDate(0, 0, -d))
		if err != nil {
			i.log.Warn("institutional: fetch failed", "day_offset", -d, "err", err)
			continue
		}
		for _, r := range refs {
			if _, ok := seen[r.Accession]; ok {
				continue
			}
			seen[r.Accession] = struct{}{}
			all = append(all, r)
		}
	}
	if len(all) == 0 {
		return
	}
	// Newest filing date first (FiledDate is YYYY-MM-DD, lexically sortable).
	sort.SliceStable(all, func(x, y int) bool { return all[x].FiledDate > all[y].FiledDate })
	if len(all) > i.max {
		all = all[:i.max]
	}
	// Enrich the newest filings with the reporting institution (the "FILED BY"
	// party), bounded to keep SEC fetches modest. Failures leave Filer empty.
	const maxFiler = 60
	for k := range all {
		if k >= maxFiler {
			break
		}
		if filer, err := i.sec.FetchFiler(ctx, all[k]); err == nil && filer != "" {
			all[k].Filer = filer
		}
	}
	i.cache.Set(all)
	i.log.Info("institutional: refreshed", "filings", len(all))
}
