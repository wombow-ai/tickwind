package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

// Compile-time guard: *postgres.Store satisfies the optional Pruner capability.
var _ store.Pruner = (*Store)(nil)

// PruneNews deletes news older than `before`, keeping hot-list tickers until the
// (earlier) `hotBefore`. A row survives the cut if it's recent enough OR its
// ticker is currently on a hot board and still within the longer window.
func (s *Store) PruneNews(ctx context.Context, before, hotBefore time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM news
		 WHERE published < $1
		   AND NOT (ticker IN (SELECT ticker FROM hotlist) AND published >= $2)`,
		before, hotBefore)
	if err != nil {
		return 0, fmt.Errorf("prune news: %w", err)
	}
	return tag.RowsAffected(), nil
}

// PruneSocial deletes posts older than `before`, never touching posts whose
// source is in `protect` (the 大V / KOL rail), and keeping hot-list tickers until
// the (earlier) `hotBefore`. COALESCE folds a NULL source to ” so untagged rows
// are still eligible.
func (s *Store) PruneSocial(ctx context.Context, before, hotBefore time.Time, protect []string) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM social
		 WHERE created_at < $1
		   AND NOT (COALESCE(source, '') = ANY($3))
		   AND NOT (ticker IN (SELECT ticker FROM hotlist) AND created_at >= $2)`,
		before, hotBefore, protect)
	if err != nil {
		return 0, fmt.Errorf("prune social: %w", err)
	}
	return tag.RowsAffected(), nil
}

// PruneFilings deletes filings filed before `before`.
func (s *Store) PruneFilings(ctx context.Context, before time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM filings WHERE filed_at < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("prune filings: %w", err)
	}
	return tag.RowsAffected(), nil
}

// PruneInsiderBuys deletes insider buys filed before `before`.
func (s *Store) PruneInsiderBuys(ctx context.Context, before time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM insider_buys WHERE filed_date < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("prune insider_buys: %w", err)
	}
	return tag.RowsAffected(), nil
}

// PruneSeenForm4 deletes seen-Form-4 markers filed before `before` (this is the
// fastest-churning table — one row per scanned filing).
func (s *Store) PruneSeenForm4(ctx context.Context, before time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM seen_form4 WHERE filed_date < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("prune seen_form4: %w", err)
	}
	return tag.RowsAffected(), nil
}

// capColumns whitelists the cappable tables and their recency column. The table
// and column names are interpolated into SQL, so they MUST come from this map
// (never from caller input) to stay injection-safe.
var capColumns = map[string]string{"news": "published", "social": "created_at"}

// CapPerTicker keeps only the newest n rows per ticker, deleting the overflow.
func (s *Store) CapPerTicker(ctx context.Context, table string, n int) (int64, error) {
	col, ok := capColumns[table]
	if !ok {
		return 0, fmt.Errorf("cap per ticker: unsupported table %q", table)
	}
	if n <= 0 {
		return 0, nil
	}
	q := fmt.Sprintf(`
		DELETE FROM %[1]s WHERE (ticker, id) IN (
			SELECT ticker, id FROM (
				SELECT ticker, id,
				       row_number() OVER (PARTITION BY ticker ORDER BY %[2]s DESC NULLS LAST) AS rn
				  FROM %[1]s
			) ranked WHERE ranked.rn > $1
		)`, table, col)
	tag, err := s.pool.Exec(ctx, q, n)
	if err != nil {
		return 0, fmt.Errorf("cap %s per ticker: %w", table, err)
	}
	return tag.RowsAffected(), nil
}
