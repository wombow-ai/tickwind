// Package postgres provides a PostgreSQL-backed implementation of store.Store
// using the pgx driver. It is used in deployed environments; local development
// can use the in-memory store instead.
package postgres

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wombow-ai/tickwind/internal/store"
)

//go:embed schema.sql
var schema string

// Store is a PostgreSQL-backed store.Store.
type Store struct {
	pool *pgxpool.Pool
}

var _ store.Store = (*Store)(nil)

// New connects to the database at dsn, verifies connectivity, and applies the
// (idempotent) schema migrations. The caller must call Close when finished.
func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	if _, err := pool.Exec(ctx, schema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: migrate: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close releases the underlying connection pool.
func (s *Store) Close() { s.pool.Close() }

// UpsertSecurity inserts or updates a tracked security, keyed by ticker.
func (s *Store) UpsertSecurity(ctx context.Context, sec store.Security) error {
	const q = `
INSERT INTO securities (ticker, cik, name, market, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (ticker) DO UPDATE
SET cik = EXCLUDED.cik, name = EXCLUDED.name, market = EXCLUDED.market, updated_at = now()`
	if _, err := s.pool.Exec(ctx, q, sec.Ticker, sec.CIK, sec.Name, sec.Market); err != nil {
		return fmt.Errorf("postgres: upsert security %s: %w", sec.Ticker, err)
	}
	return nil
}

// GetSecurity returns the security for ticker. The boolean is false when no
// such security is tracked.
func (s *Store) GetSecurity(ctx context.Context, ticker string) (store.Security, bool, error) {
	const q = `SELECT ticker, cik, name, market FROM securities WHERE ticker = $1`
	var sec store.Security
	err := s.pool.QueryRow(ctx, q, ticker).Scan(&sec.Ticker, &sec.CIK, &sec.Name, &sec.Market)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return store.Security{}, false, nil
	case err != nil:
		return store.Security{}, false, fmt.Errorf("postgres: get security %s: %w", ticker, err)
	}
	return sec, true, nil
}

// SaveFilings upserts filings, deduplicating on (ticker, accession_no). It is
// idempotent: re-saving the same filings is a no-op.
func (s *Store) SaveFilings(ctx context.Context, ticker string, filings []store.Filing) error {
	if len(filings) == 0 {
		return nil
	}
	const q = `
INSERT INTO filings (ticker, accession_no, form, title, filed_at, url)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (ticker, accession_no) DO UPDATE
SET form = EXCLUDED.form, title = EXCLUDED.title, filed_at = EXCLUDED.filed_at, url = EXCLUDED.url`
	batch := &pgx.Batch{}
	for _, f := range filings {
		batch.Queue(q, ticker, f.AccessionNo, f.Form, f.Title, f.FiledAt, f.URL)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range filings {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("postgres: save filings %s: %w", ticker, err)
		}
	}
	return nil
}

// ListFilings returns filings for ticker, newest first. A limit <= 0 means no
// limit.
func (s *Store) ListFilings(ctx context.Context, ticker string, limit int) ([]store.Filing, error) {
	q := `
SELECT ticker, form, title, filed_at, accession_no, url
FROM filings WHERE ticker = $1 ORDER BY filed_at DESC`
	args := []any{ticker}
	if limit > 0 {
		q += ` LIMIT $2`
		args = append(args, limit)
	}

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list filings %s: %w", ticker, err)
	}
	defer rows.Close()

	var out []store.Filing
	for rows.Next() {
		var f store.Filing
		if err := rows.Scan(&f.Ticker, &f.Form, &f.Title, &f.FiledAt, &f.AccessionNo, &f.URL); err != nil {
			return nil, fmt.Errorf("postgres: scan filing %s: %w", ticker, err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate filings %s: %w", ticker, err)
	}
	return out, nil
}

// UpsertQuote stores the latest quote for a security, keyed by ticker.
func (s *Store) UpsertQuote(ctx context.Context, q store.Quote) error {
	const query = `
INSERT INTO quotes (ticker, price, session, source, at, updated_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (ticker) DO UPDATE
SET price = EXCLUDED.price, session = EXCLUDED.session, source = EXCLUDED.source, at = EXCLUDED.at, updated_at = now()`
	if _, err := s.pool.Exec(ctx, query, q.Ticker, q.Price, q.Session, q.Source, q.At); err != nil {
		return fmt.Errorf("postgres: upsert quote %s: %w", q.Ticker, err)
	}
	return nil
}

// GetQuote returns the latest quote for ticker. The boolean is false when no
// quote has been stored yet.
func (s *Store) GetQuote(ctx context.Context, ticker string) (store.Quote, bool, error) {
	const query = `SELECT ticker, price, session, source, at FROM quotes WHERE ticker = $1`
	var q store.Quote
	err := s.pool.QueryRow(ctx, query, ticker).Scan(&q.Ticker, &q.Price, &q.Session, &q.Source, &q.At)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return store.Quote{}, false, nil
	case err != nil:
		return store.Quote{}, false, fmt.Errorf("postgres: get quote %s: %w", ticker, err)
	}
	return q, true, nil
}

// SaveNews upserts news items, deduplicating on (ticker, id).
func (s *Store) SaveNews(ctx context.Context, ticker string, items []store.News) error {
	if len(items) == 0 {
		return nil
	}
	const query = `
INSERT INTO news (ticker, id, headline, summary, source, url, published)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (ticker, id) DO UPDATE
SET headline = EXCLUDED.headline, summary = EXCLUDED.summary, source = EXCLUDED.source, url = EXCLUDED.url, published = EXCLUDED.published`
	batch := &pgx.Batch{}
	for _, n := range items {
		batch.Queue(query, ticker, n.ID, n.Headline, n.Summary, n.Source, n.URL, n.Published)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range items {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("postgres: save news %s: %w", ticker, err)
		}
	}
	return nil
}

// ListNews returns news for ticker, newest first. A limit <= 0 means no limit.
func (s *Store) ListNews(ctx context.Context, ticker string, limit int) ([]store.News, error) {
	query := `
SELECT ticker, id, headline, summary, source, url, published
FROM news WHERE ticker = $1 ORDER BY published DESC`
	args := []any{ticker}
	if limit > 0 {
		query += ` LIMIT $2`
		args = append(args, limit)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list news %s: %w", ticker, err)
	}
	defer rows.Close()

	var out []store.News
	for rows.Next() {
		var n store.News
		if err := rows.Scan(&n.Ticker, &n.ID, &n.Headline, &n.Summary, &n.Source, &n.URL, &n.Published); err != nil {
			return nil, fmt.Errorf("postgres: scan news %s: %w", ticker, err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate news %s: %w", ticker, err)
	}
	return out, nil
}
