// Package postgres provides a PostgreSQL-backed implementation of store.Store
// using the pgx driver. It is used in deployed environments; local development
// can use the in-memory store instead.
package postgres

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

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
INSERT INTO quotes (ticker, price, prev_close, session, source, at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, now())
ON CONFLICT (ticker) DO UPDATE
SET price = EXCLUDED.price, prev_close = EXCLUDED.prev_close, session = EXCLUDED.session, source = EXCLUDED.source, at = EXCLUDED.at, updated_at = now()`
	if _, err := s.pool.Exec(ctx, query, q.Ticker, q.Price, q.PrevClose, q.Session, q.Source, q.At); err != nil {
		return fmt.Errorf("postgres: upsert quote %s: %w", q.Ticker, err)
	}
	return nil
}

// GetQuote returns the latest quote for ticker. The boolean is false when no
// quote has been stored yet.
func (s *Store) GetQuote(ctx context.Context, ticker string) (store.Quote, bool, error) {
	const query = `SELECT ticker, price, COALESCE(prev_close, 0), session, source, at FROM quotes WHERE ticker = $1`
	var q store.Quote
	err := s.pool.QueryRow(ctx, query, ticker).Scan(&q.Ticker, &q.Price, &q.PrevClose, &q.Session, &q.Source, &q.At)
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

// SaveSocial upserts social posts, deduplicating on (ticker, id).
func (s *Store) SaveSocial(ctx context.Context, ticker string, posts []store.Post) error {
	if len(posts) == 0 {
		return nil
	}
	const query = `
INSERT INTO social (ticker, id, source, author, body, url, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (ticker, id) DO UPDATE
SET source = EXCLUDED.source, author = EXCLUDED.author, body = EXCLUDED.body, url = EXCLUDED.url, created_at = EXCLUDED.created_at`
	batch := &pgx.Batch{}
	for _, p := range posts {
		batch.Queue(query, ticker, p.ID, p.Source, p.Author, p.Body, p.URL, p.CreatedAt)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range posts {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("postgres: save social %s: %w", ticker, err)
		}
	}
	return nil
}

// ListSocial returns social posts for ticker, newest first. limit <= 0 = no limit.
func (s *Store) ListSocial(ctx context.Context, ticker string, limit int) ([]store.Post, error) {
	query := `
SELECT ticker, id, source, author, body, url, created_at
FROM social WHERE ticker = $1 ORDER BY created_at DESC`
	args := []any{ticker}
	if limit > 0 {
		query += ` LIMIT $2`
		args = append(args, limit)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list social %s: %w", ticker, err)
	}
	defer rows.Close()

	var out []store.Post
	for rows.Next() {
		var p store.Post
		if err := rows.Scan(&p.Ticker, &p.ID, &p.Source, &p.Author, &p.Body, &p.URL, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan social %s: %w", ticker, err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate social %s: %w", ticker, err)
	}
	return out, nil
}

// SaveSignals upserts per-ticker signals, deduplicating on (ticker, source).
func (s *Store) SaveSignals(ctx context.Context, signals []store.Signal) error {
	if len(signals) == 0 {
		return nil
	}
	const query = `
INSERT INTO signals (ticker, source, kind, mentions, mentions_prev, rank, rank_prev, upvotes, score, label, sample_size, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now())
ON CONFLICT (ticker, source) DO UPDATE
SET kind = EXCLUDED.kind, mentions = EXCLUDED.mentions, mentions_prev = EXCLUDED.mentions_prev,
    rank = EXCLUDED.rank, rank_prev = EXCLUDED.rank_prev, upvotes = EXCLUDED.upvotes,
    score = EXCLUDED.score, label = EXCLUDED.label, sample_size = EXCLUDED.sample_size, updated_at = now()`
	batch := &pgx.Batch{}
	for _, sig := range signals {
		batch.Queue(query, sig.Ticker, sig.Source, sig.Kind, sig.Mentions, sig.MentionsPrev,
			sig.Rank, sig.RankPrev, sig.Upvotes, sig.Score, sig.Label, sig.SampleSize)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range signals {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("postgres: save signals: %w", err)
		}
	}
	return nil
}

// ListSignals returns every source's latest signal for ticker, ordered by source.
func (s *Store) ListSignals(ctx context.Context, ticker string) ([]store.Signal, error) {
	const query = `
SELECT ticker, source, kind, mentions, mentions_prev, rank, rank_prev, upvotes, score, label, sample_size, updated_at
FROM signals WHERE ticker = $1 ORDER BY source`
	rows, err := s.pool.Query(ctx, query, ticker)
	if err != nil {
		return nil, fmt.Errorf("postgres: list signals %s: %w", ticker, err)
	}
	defer rows.Close()

	var out []store.Signal
	for rows.Next() {
		var sig store.Signal
		if err := rows.Scan(&sig.Ticker, &sig.Source, &sig.Kind, &sig.Mentions, &sig.MentionsPrev,
			&sig.Rank, &sig.RankPrev, &sig.Upvotes, &sig.Score, &sig.Label, &sig.SampleSize, &sig.UpdatedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan signal %s: %w", ticker, err)
		}
		out = append(out, sig)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate signals %s: %w", ticker, err)
	}
	return out, nil
}

// SaveHotList replaces one board's leaderboard snapshot atomically (clear that
// board + re-insert in one transaction), leaving other boards untouched.
func (s *Store) SaveHotList(ctx context.Context, board string, stocks []store.HotStock) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: hotlist begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM hotlist WHERE board = $1`, board); err != nil {
		return fmt.Errorf("postgres: hotlist clear %s: %w", board, err)
	}
	if len(stocks) > 0 {
		const q = `
INSERT INTO hotlist (board, ticker, name, rank, mentions, mentions_prev, mention_change, upvotes, score, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
ON CONFLICT (board, ticker) DO UPDATE
SET name = EXCLUDED.name, rank = EXCLUDED.rank, mentions = EXCLUDED.mentions,
    mentions_prev = EXCLUDED.mentions_prev, mention_change = EXCLUDED.mention_change,
    upvotes = EXCLUDED.upvotes, score = EXCLUDED.score, updated_at = now()`
		batch := &pgx.Batch{}
		for _, h := range stocks {
			batch.Queue(q, board, h.Ticker, h.Name, h.Rank, h.Mentions, h.MentionsPrev, h.Change, h.Upvotes, h.Score)
		}
		br := tx.SendBatch(ctx, batch)
		for range stocks {
			if _, err := br.Exec(); err != nil {
				br.Close()
				return fmt.Errorf("postgres: hotlist insert %s: %w", board, err)
			}
		}
		if err := br.Close(); err != nil {
			return fmt.Errorf("postgres: hotlist batch close: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: hotlist commit: %w", err)
	}
	return nil
}

// HotList returns one board's leaderboard, top first. limit <= 0 = all.
func (s *Store) HotList(ctx context.Context, board string, limit int) ([]store.HotStock, error) {
	query := `
SELECT board, ticker, name, rank, mentions, mentions_prev, mention_change, upvotes, score, updated_at
FROM hotlist WHERE board = $1 ORDER BY rank`
	args := []any{board}
	if limit > 0 {
		query += ` LIMIT $2`
		args = append(args, limit)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: hotlist %s: %w", board, err)
	}
	defer rows.Close()

	var out []store.HotStock
	for rows.Next() {
		var h store.HotStock
		if err := rows.Scan(&h.Board, &h.Ticker, &h.Name, &h.Rank, &h.Mentions, &h.MentionsPrev,
			&h.Change, &h.Upvotes, &h.Score, &h.UpdatedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan hotlist: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate hotlist: %w", err)
	}
	return out, nil
}

// SaveInsiderBuys upserts insider open-market purchases, deduped on accession.
func (s *Store) SaveInsiderBuys(ctx context.Context, buys []store.InsiderBuy) error {
	if len(buys) == 0 {
		return nil
	}
	const q = `
INSERT INTO insider_buys (accession, ticker, cik, company, owner_name, title, is_officer, is_director, filed_date, shares, price, value, filing_url)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (accession) DO UPDATE
SET ticker = EXCLUDED.ticker, cik = EXCLUDED.cik, company = EXCLUDED.company,
    owner_name = EXCLUDED.owner_name, title = EXCLUDED.title, is_officer = EXCLUDED.is_officer,
    is_director = EXCLUDED.is_director, filed_date = EXCLUDED.filed_date, shares = EXCLUDED.shares,
    price = EXCLUDED.price, value = EXCLUDED.value, filing_url = EXCLUDED.filing_url`
	batch := &pgx.Batch{}
	for _, b := range buys {
		batch.Queue(q, b.Accession, b.Ticker, b.CIK, b.Company, b.OwnerName, b.Title,
			b.IsOfficer, b.IsDirector, b.FiledDate, b.Shares, b.Price, b.Value, b.FilingURL)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range buys {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("postgres: save insider buys: %w", err)
		}
	}
	return nil
}

// RecentInsiderBuys returns buys filed on/after since, newest first.
func (s *Store) RecentInsiderBuys(ctx context.Context, since time.Time) ([]store.InsiderBuy, error) {
	const q = `
SELECT accession, ticker, cik, company, owner_name, title, is_officer, is_director, filed_date, shares, price, value, filing_url
FROM insider_buys WHERE filed_date >= $1 ORDER BY filed_date DESC`
	rows, err := s.pool.Query(ctx, q, since)
	if err != nil {
		return nil, fmt.Errorf("postgres: recent insider buys: %w", err)
	}
	defer rows.Close()

	var out []store.InsiderBuy
	for rows.Next() {
		var b store.InsiderBuy
		if err := rows.Scan(&b.Accession, &b.Ticker, &b.CIK, &b.Company, &b.OwnerName, &b.Title,
			&b.IsOfficer, &b.IsDirector, &b.FiledDate, &b.Shares, &b.Price, &b.Value, &b.FilingURL); err != nil {
			return nil, fmt.Errorf("postgres: scan insider buy: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate insider buys: %w", err)
	}
	return out, nil
}

// Watchlist returns one user's tracked tickers, in insertion order.
func (s *Store) Watchlist(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT ticker FROM watchlist WHERE user_id = $1 ORDER BY added_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("postgres: watchlist: %w", err)
	}
	defer rows.Close()
	return scanTickers(rows)
}

// AllWatchlistTickers returns the de-duplicated union across all users.
func (s *Store) AllWatchlistTickers(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT ticker FROM watchlist`)
	if err != nil {
		return nil, fmt.Errorf("postgres: all watchlist tickers: %w", err)
	}
	defer rows.Close()
	return scanTickers(rows)
}

func scanTickers(rows pgx.Rows) ([]string, error) {
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("postgres: scan ticker: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate tickers: %w", err)
	}
	return out, nil
}

// AddToWatchlist adds a ticker to a user's watchlist if not present.
func (s *Store) AddToWatchlist(ctx context.Context, userID, ticker string) error {
	const query = `INSERT INTO watchlist (user_id, ticker) VALUES ($1, $2) ON CONFLICT (user_id, ticker) DO NOTHING`
	if _, err := s.pool.Exec(ctx, query, userID, ticker); err != nil {
		return fmt.Errorf("postgres: add watchlist %s: %w", ticker, err)
	}
	return nil
}

// RemoveFromWatchlist removes a ticker from a user's watchlist.
func (s *Store) RemoveFromWatchlist(ctx context.Context, userID, ticker string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM watchlist WHERE user_id = $1 AND ticker = $2`, userID, ticker); err != nil {
		return fmt.Errorf("postgres: remove watchlist %s: %w", ticker, err)
	}
	return nil
}

// SaveClip upserts a user's saved link (deduped by id).
func (s *Store) SaveClip(ctx context.Context, c store.Clip) error {
	const query = `
INSERT INTO clips (id, user_id, ticker, title, url, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (id) DO UPDATE SET title = EXCLUDED.title, url = EXCLUDED.url`
	if _, err := s.pool.Exec(ctx, query, c.ID, c.UserID, c.Ticker, c.Title, c.URL, c.CreatedAt); err != nil {
		return fmt.Errorf("postgres: save clip: %w", err)
	}
	return nil
}

// ListClips returns a user's saved links for a ticker, newest first.
func (s *Store) ListClips(ctx context.Context, userID, ticker string, limit int) ([]store.Clip, error) {
	query := `SELECT id, user_id, ticker, title, url, created_at FROM clips WHERE user_id = $1 AND ticker = $2 ORDER BY created_at DESC`
	args := []any{userID, ticker}
	if limit > 0 {
		query += ` LIMIT $3`
		args = append(args, limit)
	}
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list clips %s: %w", ticker, err)
	}
	defer rows.Close()

	var out []store.Clip
	for rows.Next() {
		var c store.Clip
		if err := rows.Scan(&c.ID, &c.UserID, &c.Ticker, &c.Title, &c.URL, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan clip %s: %w", ticker, err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate clips %s: %w", ticker, err)
	}
	return out, nil
}
