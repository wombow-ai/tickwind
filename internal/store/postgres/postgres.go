// Package postgres provides a PostgreSQL-backed implementation of store.Store
// using the pgx driver. It is used in deployed environments; local development
// can use the in-memory store instead.
package postgres

import (
	"context"
	_ "embed"
	"encoding/json"
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
// Ping verifies the connection pool can reach Postgres.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

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
INSERT INTO quotes (ticker, price, prev_close, regular_close, session, source, at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, now())
ON CONFLICT (ticker) DO UPDATE
SET price = EXCLUDED.price, prev_close = EXCLUDED.prev_close, regular_close = EXCLUDED.regular_close, session = EXCLUDED.session, source = EXCLUDED.source, at = EXCLUDED.at, updated_at = now()`
	if _, err := s.pool.Exec(ctx, query, q.Ticker, q.Price, q.PrevClose, q.RegularClose, q.Session, q.Source, q.At); err != nil {
		return fmt.Errorf("postgres: upsert quote %s: %w", q.Ticker, err)
	}
	return nil
}

// GetQuote returns the latest quote for ticker. The boolean is false when no
// quote has been stored yet.
func (s *Store) GetQuote(ctx context.Context, ticker string) (store.Quote, bool, error) {
	const query = `SELECT ticker, price, COALESCE(prev_close, 0), COALESCE(regular_close, 0), session, source, at FROM quotes WHERE ticker = $1`
	var q store.Quote
	err := s.pool.QueryRow(ctx, query, ticker).Scan(&q.Ticker, &q.Price, &q.PrevClose, &q.RegularClose, &q.Session, &q.Source, &q.At)
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
SELECT ticker, id, headline, COALESCE(headline_zh,''), summary, source, url, published
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
		if err := rows.Scan(&n.Ticker, &n.ID, &n.Headline, &n.HeadlineZH, &n.Summary, &n.Source, &n.URL, &n.Published); err != nil {
			return nil, fmt.Errorf("postgres: scan news %s: %w", ticker, err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate news %s: %w", ticker, err)
	}
	return out, nil
}

// ListUntranslatedNews returns up to limit recent rows lacking a Chinese
// headline, newest first (fresh news gets translated before the backlog).
func (s *Store) ListUntranslatedNews(ctx context.Context, limit int) ([]store.News, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
SELECT ticker, id, headline, COALESCE(headline_zh,''), summary, source, url, published
FROM news
WHERE (headline_zh IS NULL OR headline_zh = '') AND headline <> ''
ORDER BY published DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("postgres: list untranslated news: %w", err)
	}
	defer rows.Close()
	var out []store.News
	for rows.Next() {
		var n store.News
		if err := rows.Scan(&n.Ticker, &n.ID, &n.Headline, &n.HeadlineZH, &n.Summary, &n.Source, &n.URL, &n.Published); err != nil {
			return nil, fmt.Errorf("postgres: scan untranslated news: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// SetNewsTranslation stores the translated headline for one news row.
func (s *Store) SetNewsTranslation(ctx context.Context, ticker, id, headlineZH string) error {
	if _, err := s.pool.Exec(ctx,
		`UPDATE news SET headline_zh = $3 WHERE ticker = $1 AND id = $2`,
		ticker, id, headlineZH); err != nil {
		return fmt.Errorf("postgres: set news translation: %w", err)
	}
	return nil
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

const earningCols = `ticker, edate, hour, eps_estimate, eps_actual, revenue_estimate, revenue_actual`

func scanEarning(row interface{ Scan(...any) error }) (store.Earning, error) {
	var e store.Earning
	err := row.Scan(&e.Ticker, &e.Date, &e.Hour, &e.EPSEstimate, &e.EPSActual, &e.RevenueEstimate, &e.RevenueActual)
	return e, err
}

func (s *Store) SaveEarnings(ctx context.Context, es []store.Earning) error {
	if len(es) == 0 {
		return nil
	}
	const q = `
INSERT INTO earnings (ticker, edate, hour, eps_estimate, eps_actual, revenue_estimate, revenue_actual)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (ticker, edate) DO UPDATE SET
  hour = EXCLUDED.hour, eps_estimate = EXCLUDED.eps_estimate, eps_actual = EXCLUDED.eps_actual,
  revenue_estimate = EXCLUDED.revenue_estimate, revenue_actual = EXCLUDED.revenue_actual`
	batch := &pgx.Batch{}
	for _, e := range es {
		batch.Queue(q, e.Ticker, e.Date, e.Hour, e.EPSEstimate, e.EPSActual, e.RevenueEstimate, e.RevenueActual)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range es {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("postgres: save earnings: %w", err)
		}
	}
	return nil
}

func (s *Store) ListEarnings(ctx context.Context, from, to time.Time) ([]store.Earning, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+earningCols+` FROM earnings WHERE edate >= $1 AND edate <= $2 ORDER BY edate`, from, to)
	if err != nil {
		return nil, fmt.Errorf("postgres: list earnings: %w", err)
	}
	defer rows.Close()
	var out []store.Earning
	for rows.Next() {
		e, err := scanEarning(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan earning: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) ListEarningsForTicker(ctx context.Context, ticker string, limit int) ([]store.Earning, error) {
	if limit <= 0 {
		limit = 12
	}
	rows, err := s.pool.Query(ctx, `SELECT `+earningCols+` FROM earnings WHERE ticker = $1 ORDER BY edate LIMIT $2`, ticker, limit)
	if err != nil {
		return nil, fmt.Errorf("postgres: list earnings for ticker: %w", err)
	}
	defer rows.Close()
	var out []store.Earning
	for rows.Next() {
		e, err := scanEarning(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan earning: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// MarkForm4Seen records Form-4 accessions as already fetched, deduped on
// accession (existing rows keep their original filed_date).
func (s *Store) MarkForm4Seen(ctx context.Context, accessions []string, filedDate time.Time) error {
	const q = `INSERT INTO seen_form4 (accession, filed_date) VALUES ($1, $2)
ON CONFLICT (accession) DO NOTHING`
	batch := &pgx.Batch{}
	for _, a := range accessions {
		if a != "" {
			batch.Queue(q, a, filedDate)
		}
	}
	if batch.Len() == 0 {
		return nil
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < batch.Len(); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("postgres: mark form4 seen: %w", err)
		}
	}
	return nil
}

// SeenForm4Since returns Form-4 accessions seen on/after since.
func (s *Store) SeenForm4Since(ctx context.Context, since time.Time) ([]string, error) {
	const q = `SELECT accession FROM seen_form4 WHERE filed_date >= $1`
	rows, err := s.pool.Query(ctx, q, since)
	if err != nil {
		return nil, fmt.Errorf("postgres: seen form4: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, fmt.Errorf("postgres: scan seen form4: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate seen form4: %w", err)
	}
	return out, nil
}

// SaveFearGreed upserts one day's headline Fear & Greed score, idempotent on the
// day ("2006-01-02") — a same-day re-save replaces the score and bumps updated_at.
func (s *Store) SaveFearGreed(ctx context.Context, date string, score int) error {
	const q = `INSERT INTO fear_greed (day, score) VALUES ($1, $2)
ON CONFLICT (day) DO UPDATE SET score = $2, updated_at = now()`
	if _, err := s.pool.Exec(ctx, q, date, score); err != nil {
		return fmt.Errorf("postgres: save fear_greed: %w", err)
	}
	return nil
}

// FearGreedHistory returns the daily scores in CHRONOLOGICAL order (oldest→newest).
// When limit>0 it returns only the most recent `limit` days (still chronological);
// limit<=0 returns all days. Always a non-nil (possibly empty) slice.
func (s *Store) FearGreedHistory(ctx context.Context, limit int) ([]store.FearGreedPoint, error) {
	// Fetch the most recent `limit` days (DESC + LIMIT), then reverse to
	// chronological order; limit<=0 fetches everything.
	q := `SELECT day::text, score FROM fear_greed ORDER BY day DESC`
	args := []any{}
	if limit > 0 {
		q += ` LIMIT $1`
		args = append(args, limit)
	}
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: fear_greed history: %w", err)
	}
	defer rows.Close()
	out := make([]store.FearGreedPoint, 0)
	for rows.Next() {
		var p store.FearGreedPoint
		if err := rows.Scan(&p.Date, &p.Score); err != nil {
			return nil, fmt.Errorf("postgres: scan fear_greed: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate fear_greed: %w", err)
	}
	// Reverse DESC → chronological (oldest→newest).
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// SaveAISummary upserts the serialized AI digest for (ticker, day, lang),
// idempotent on the composite key — a same-day re-save replaces the payload and
// bumps created_at.
func (s *Store) SaveAISummary(ctx context.Context, ticker, day, lang string, payload []byte) error {
	const q = `INSERT INTO ai_summary (ticker, day, lang, payload) VALUES ($1, $2, $3, $4)
ON CONFLICT (ticker, day, lang) DO UPDATE SET payload = $4, created_at = now()`
	if _, err := s.pool.Exec(ctx, q, ticker, day, lang, payload); err != nil {
		return fmt.Errorf("postgres: save ai_summary: %w", err)
	}
	return nil
}

// GetAISummary returns the stored digest payload for (ticker, day, lang), or
// ok=false when there's no row (pgx.ErrNoRows → the caller generates).
func (s *Store) GetAISummary(ctx context.Context, ticker, day, lang string) ([]byte, bool, error) {
	var payload []byte
	err := s.pool.QueryRow(ctx,
		`SELECT payload FROM ai_summary WHERE ticker = $1 AND day = $2 AND lang = $3`,
		ticker, day, lang).Scan(&payload)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, false, nil
	case err != nil:
		return nil, false, fmt.Errorf("postgres: get ai_summary: %w", err)
	}
	return payload, true, nil
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

// noteCols is the SELECT list returning every Note field with NULLs folded to
// "" (ticker) / "" (note_date as YYYY-MM-DD) so scans go straight into strings.
const noteCols = `id, user_id, COALESCE(ticker,''), COALESCE(to_char(note_date,'YYYY-MM-DD'),''), body, pinned, created_at, updated_at`

func scanNote(row interface{ Scan(...any) error }) (store.Note, error) {
	var n store.Note
	err := row.Scan(&n.ID, &n.UserID, &n.Ticker, &n.Date, &n.Body, &n.Pinned, &n.CreatedAt, &n.UpdatedAt)
	return n, err
}

// SaveNote upserts a user's note (by id). Empty ticker/date map to SQL NULL.
func (s *Store) SaveNote(ctx context.Context, n store.Note) error {
	const query = `
INSERT INTO notes (id, user_id, ticker, note_date, body, pinned, created_at, updated_at)
VALUES ($1, $2, NULLIF($3,''), NULLIF($4,'')::date, $5, $6, $7, $8)
ON CONFLICT (id) DO UPDATE SET
  ticker = EXCLUDED.ticker, note_date = EXCLUDED.note_date,
  body = EXCLUDED.body, pinned = EXCLUDED.pinned, updated_at = EXCLUDED.updated_at`
	if _, err := s.pool.Exec(ctx, query, n.ID, n.UserID, n.Ticker, n.Date, n.Body, n.Pinned, n.CreatedAt, n.UpdatedAt); err != nil {
		return fmt.Errorf("postgres: save note: %w", err)
	}
	return nil
}

// ListNotes returns a user's notes, optionally filtered by ticker and/or date
// range, pinned first then newest.
func (s *Store) ListNotes(ctx context.Context, f store.NoteFilter) ([]store.Note, error) {
	query := `SELECT ` + noteCols + ` FROM notes WHERE user_id = $1`
	args := []any{f.UserID}
	if f.Ticker != "" {
		args = append(args, f.Ticker)
		query += fmt.Sprintf(" AND ticker = $%d", len(args))
	}
	if f.From != "" {
		args = append(args, f.From)
		query += fmt.Sprintf(" AND note_date >= $%d::date", len(args))
	}
	if f.To != "" {
		args = append(args, f.To)
		query += fmt.Sprintf(" AND note_date <= $%d::date", len(args))
	}
	query += " ORDER BY pinned DESC, created_at DESC"
	if f.Limit > 0 {
		args = append(args, f.Limit)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list notes: %w", err)
	}
	defer rows.Close()
	var out []store.Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan note: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// UpdateNote patches body and/or pinned for the caller's note (nil = unchanged),
// returning found=false if the id isn't this user's.
func (s *Store) UpdateNote(ctx context.Context, userID, id string, body *string, pinned *bool) (store.Note, bool, error) {
	const query = `
UPDATE notes SET body = COALESCE($3, body), pinned = COALESCE($4, pinned), updated_at = now()
WHERE id = $1 AND user_id = $2
RETURNING ` + noteCols
	n, err := scanNote(s.pool.QueryRow(ctx, query, id, userID, body, pinned))
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Note{}, false, nil
	}
	if err != nil {
		return store.Note{}, false, fmt.Errorf("postgres: update note: %w", err)
	}
	return n, true, nil
}

// DeleteNote removes the caller's note, returning false if it wasn't theirs.
func (s *Store) DeleteNote(ctx context.Context, userID, id string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM notes WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return false, fmt.Errorf("postgres: delete note: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

const alertCols = `id, user_id, ticker, kind, threshold, active, created_at, triggered_at`

func scanAlert(row interface{ Scan(...any) error }) (store.Alert, error) {
	var a store.Alert
	var trig *time.Time // triggered_at is NULL until fired
	err := row.Scan(&a.ID, &a.UserID, &a.Ticker, &a.Kind, &a.Threshold, &a.Active, &a.CreatedAt, &trig)
	if trig != nil {
		a.TriggeredAt = *trig
	}
	return a, err
}

func (s *Store) ListActiveAlerts(ctx context.Context) ([]store.Alert, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+alertCols+` FROM alerts WHERE active AND triggered_at IS NULL ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list active alerts: %w", err)
	}
	defer rows.Close()
	var out []store.Alert
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan alert: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) MarkAlertTriggered(ctx context.Context, id string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE alerts SET triggered_at = $2 WHERE id = $1 AND triggered_at IS NULL`, id, at)
	if err != nil {
		return fmt.Errorf("postgres: mark alert triggered: %w", err)
	}
	return nil
}

func (s *Store) SaveAlert(ctx context.Context, a store.Alert) error {
	const query = `
INSERT INTO alerts (id, user_id, ticker, kind, threshold, active, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (id) DO UPDATE SET
  ticker = EXCLUDED.ticker, kind = EXCLUDED.kind,
  threshold = EXCLUDED.threshold, active = EXCLUDED.active`
	if _, err := s.pool.Exec(ctx, query, a.ID, a.UserID, a.Ticker, a.Kind, a.Threshold, a.Active, a.CreatedAt); err != nil {
		return fmt.Errorf("postgres: save alert: %w", err)
	}
	return nil
}

func (s *Store) ListAlerts(ctx context.Context, userID string) ([]store.Alert, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+alertCols+` FROM alerts WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list alerts: %w", err)
	}
	defer rows.Close()
	var out []store.Alert
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan alert: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) DeleteAlert(ctx context.Context, userID, id string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM alerts WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return false, fmt.Errorf("postgres: delete alert: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (s *Store) ReactivateAlert(ctx context.Context, userID, id string) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE alerts SET active = true, triggered_at = NULL WHERE id = $1 AND user_id = $2`,
		id, userID)
	if err != nil {
		return false, fmt.Errorf("postgres: reactivate alert: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

const holdingCols = `id, user_id, ticker, shares, avg_cost, created_at, updated_at`

func scanHolding(row interface{ Scan(...any) error }) (store.Holding, error) {
	var h store.Holding
	err := row.Scan(&h.ID, &h.UserID, &h.Ticker, &h.Shares, &h.AvgCost, &h.CreatedAt, &h.UpdatedAt)
	return h, err
}

func (s *Store) SaveHolding(ctx context.Context, h store.Holding) error {
	const query = `
INSERT INTO holdings (id, user_id, ticker, shares, avg_cost, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (user_id, ticker) DO UPDATE SET
  shares = EXCLUDED.shares, avg_cost = EXCLUDED.avg_cost, updated_at = EXCLUDED.updated_at`
	if _, err := s.pool.Exec(ctx, query, h.ID, h.UserID, h.Ticker, h.Shares, h.AvgCost, h.CreatedAt, h.UpdatedAt); err != nil {
		return fmt.Errorf("postgres: save holding: %w", err)
	}
	return nil
}

func (s *Store) ListHoldings(ctx context.Context, userID string) ([]store.Holding, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+holdingCols+` FROM holdings WHERE user_id = $1 ORDER BY ticker`, userID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list holdings: %w", err)
	}
	defer rows.Close()
	var out []store.Holding
	for rows.Next() {
		h, err := scanHolding(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan holding: %w", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) DeleteHolding(ctx context.Context, userID, id string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM holdings WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return false, fmt.Errorf("postgres: delete holding: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// GetPrefs returns the user's prefs blob (jsonb), or ok=false when the user has
// no row yet (sql.ErrNoRows → the caller falls back to defaults).
func (s *Store) GetPrefs(ctx context.Context, userID string) (json.RawMessage, bool, error) {
	var blob json.RawMessage
	err := s.pool.QueryRow(ctx, `SELECT prefs FROM user_prefs WHERE user_id = $1`, userID).Scan(&blob)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, false, nil
	case err != nil:
		return nil, false, fmt.Errorf("postgres: get prefs: %w", err)
	}
	return blob, true, nil
}

// PutPrefs upserts the user's prefs blob, replacing it wholesale (the API does
// the shallow-merge before calling this).
func (s *Store) PutPrefs(ctx context.Context, userID string, blob json.RawMessage) error {
	const query = `
INSERT INTO user_prefs (user_id, prefs, updated_at)
VALUES ($1, $2, now())
ON CONFLICT (user_id) DO UPDATE SET prefs = $2, updated_at = now()`
	if _, err := s.pool.Exec(ctx, query, userID, blob); err != nil {
		return fmt.Errorf("postgres: put prefs: %w", err)
	}
	return nil
}

// GetDeepQuotaUsed returns the user's deep-research generation count for the
// period (ET month, e.g. "2026-06"); 0 when there's no row yet (pgx.ErrNoRows is
// a clean zero). The `day` column is reused for the period key — old per-day rows
// never match a month key, so they are harmless.
func (s *Store) GetDeepQuotaUsed(ctx context.Context, userID, period string) (int, error) {
	var used int
	err := s.pool.QueryRow(ctx, `SELECT used FROM deep_research_quota WHERE user_id = $1 AND day = $2`, userID, period).Scan(&used)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return 0, nil
	case err != nil:
		return 0, fmt.Errorf("postgres: get deep-research quota: %w", err)
	}
	return used, nil
}

// IncrDeepQuotaUsed upserts the user's deep-research generation count for the
// period (ET month) by one (insert with used=1, or increment an existing row's
// used). The `day` column carries the month key (e.g. "2026-06").
func (s *Store) IncrDeepQuotaUsed(ctx context.Context, userID, period string) error {
	const query = `
INSERT INTO deep_research_quota (user_id, day, used, updated_at)
VALUES ($1, $2, 1, now())
ON CONFLICT (user_id, day) DO UPDATE SET used = deep_research_quota.used + 1, updated_at = now()`
	if _, err := s.pool.Exec(ctx, query, userID, period); err != nil {
		return fmt.Errorf("postgres: incr deep-research quota: %w", err)
	}
	return nil
}

// AppendChatMessage appends one Product B chat turn for the (user, ticker) thread.
func (s *Store) AppendChatMessage(ctx context.Context, m store.ChatMessage) error {
	const q = `INSERT INTO chat_message (user_id, ticker, role, content, created_at) VALUES ($1, $2, $3, $4, now())`
	if _, err := s.pool.Exec(ctx, q, m.UserID, m.Ticker, m.Role, m.Content); err != nil {
		return fmt.Errorf("postgres: append chat message: %w", err)
	}
	return nil
}

// ListChatMessages returns the most recent `limit` messages for the (user, ticker)
// thread in CHRONOLOGICAL order (oldest first). limit<=0 defaults to 100.
func (s *Store) ListChatMessages(ctx context.Context, userID, ticker string, limit int) ([]store.ChatMessage, error) {
	if limit <= 0 {
		limit = 100
	}
	const q = `SELECT user_id, ticker, role, content, created_at FROM chat_message WHERE user_id = $1 AND ticker = $2 ORDER BY id DESC LIMIT $3`
	rows, err := s.pool.Query(ctx, q, userID, ticker, limit)
	if err != nil {
		return nil, fmt.Errorf("postgres: list chat messages: %w", err)
	}
	defer rows.Close()
	var out []store.ChatMessage
	for rows.Next() {
		var m store.ChatMessage
		if err := rows.Scan(&m.UserID, &m.Ticker, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan chat message: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list chat messages rows: %w", err)
	}
	// Fetched newest-first (to honor LIMIT); reverse to chronological.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// GetChatMsgUsed returns the user's Product B chat-message count for the period (ET
// month); 0 when there's no row (pgx.ErrNoRows is a clean zero).
func (s *Store) GetChatMsgUsed(ctx context.Context, userID, period string) (int, error) {
	var used int
	err := s.pool.QueryRow(ctx, `SELECT used FROM chat_msg_quota WHERE user_id = $1 AND period = $2`, userID, period).Scan(&used)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return 0, nil
	case err != nil:
		return 0, fmt.Errorf("postgres: get chat-msg quota: %w", err)
	}
	return used, nil
}

// IncrChatMsgUsed upserts the user's Product B chat-message count for the period (ET
// month) by one.
func (s *Store) IncrChatMsgUsed(ctx context.Context, userID, period string) error {
	const query = `
INSERT INTO chat_msg_quota (user_id, period, used, updated_at)
VALUES ($1, $2, 1, now())
ON CONFLICT (user_id, period) DO UPDATE SET used = chat_msg_quota.used + 1, updated_at = now()`
	if _, err := s.pool.Exec(ctx, query, userID, period); err != nil {
		return fmt.Errorf("postgres: incr chat-msg quota: %w", err)
	}
	return nil
}

// GetSubscription returns the user's Stripe-synced entitlement (found=false when
// the user has no row → the caller treats them as free).
func (s *Store) GetSubscription(ctx context.Context, userID string) (store.Subscription, bool, error) {
	return s.scanSubscription(ctx, `WHERE user_id = $1`, userID)
}

// GetSubscriptionByCustomer maps a Stripe customer id back to its entitlement row
// (the webhook resolves customer → user). found=false when unknown.
func (s *Store) GetSubscriptionByCustomer(ctx context.Context, customerID string) (store.Subscription, bool, error) {
	return s.scanSubscription(ctx, `WHERE stripe_customer_id = $1`, customerID)
}

func (s *Store) scanSubscription(ctx context.Context, where, arg string) (store.Subscription, bool, error) {
	const cols = `SELECT user_id, stripe_customer_id, stripe_subscription_id, status, tier, price_id, plan_interval, current_period_end, cancel_at_period_end, updated_at FROM subscriptions `
	var sub store.Subscription
	err := s.pool.QueryRow(ctx, cols+where, arg).Scan(
		&sub.UserID, &sub.StripeCustomerID, &sub.StripeSubscriptionID, &sub.Status,
		&sub.Tier, &sub.PriceID, &sub.Interval, &sub.CurrentPeriodEnd,
		&sub.CancelAtPeriodEnd, &sub.UpdatedAt,
	)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return store.Subscription{}, false, nil
	case err != nil:
		return store.Subscription{}, false, fmt.Errorf("postgres: get subscription: %w", err)
	}
	return sub, true, nil
}

// UpsertSubscription writes the full entitlement row, keyed by user_id (the Stripe
// webhook re-derives the whole row from the self-describing Subscription object, so
// this is order-independent).
func (s *Store) UpsertSubscription(ctx context.Context, sub store.Subscription) error {
	const query = `
INSERT INTO subscriptions (user_id, stripe_customer_id, stripe_subscription_id, status, tier, price_id, plan_interval, current_period_end, cancel_at_period_end, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9, now())
ON CONFLICT (user_id) DO UPDATE SET
  stripe_customer_id = EXCLUDED.stripe_customer_id,
  stripe_subscription_id = EXCLUDED.stripe_subscription_id,
  status = EXCLUDED.status,
  tier = EXCLUDED.tier,
  price_id = EXCLUDED.price_id,
  plan_interval = EXCLUDED.plan_interval,
  current_period_end = EXCLUDED.current_period_end,
  cancel_at_period_end = EXCLUDED.cancel_at_period_end,
  updated_at = now()`
	cpe := sub.CurrentPeriodEnd
	if cpe.IsZero() {
		cpe = time.Now()
	}
	if _, err := s.pool.Exec(ctx, query, sub.UserID, sub.StripeCustomerID, sub.StripeSubscriptionID,
		sub.Status, sub.Tier, sub.PriceID, sub.Interval, cpe, sub.CancelAtPeriodEnd); err != nil {
		return fmt.Errorf("postgres: upsert subscription: %w", err)
	}
	return nil
}

// MarkStripeEventSeen records a webhook event id; fresh=true the first time (process
// it), false if already recorded (Stripe re-delivers at-least-once — skip).
func (s *Store) MarkStripeEventSeen(ctx context.Context, eventID, eventType string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `INSERT INTO stripe_events (event_id, type) VALUES ($1,$2) ON CONFLICT (event_id) DO NOTHING`, eventID, eventType)
	if err != nil {
		return false, fmt.Errorf("postgres: mark stripe event: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// SaveComment inserts a public comment (empty ticker → NULL = global board)
// together with its cashtag mention rows ($TICKER fan-out).
func (s *Store) SaveComment(ctx context.Context, c store.Comment) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: save comment begin: %w", err)
	}
	defer tx.Rollback(ctx)
	const query = `
INSERT INTO comments (id, user_id, author, ticker, body, ip, created_at)
VALUES ($1, $2, $3, NULLIF($4,''), $5, NULLIF($6,''), $7)`
	if _, err := tx.Exec(ctx, query, c.ID, c.UserID, c.Author, c.Ticker, c.Body, c.IP, c.CreatedAt); err != nil {
		return fmt.Errorf("postgres: save comment: %w", err)
	}
	for _, m := range c.Mentions {
		if _, err := tx.Exec(ctx,
			`INSERT INTO comment_mentions (comment_id, ticker) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			c.ID, m); err != nil {
			return fmt.Errorf("postgres: save comment mention: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: save comment commit: %w", err)
	}
	return nil
}

// ListComments returns non-deleted comments for a ticker ("" = the global board,
// i.e. ticker IS NULL), newest first. A non-empty ticker also includes comments
// posted elsewhere that cashtag-mention it ($TICKER fan-out).
func (s *Store) ListComments(ctx context.Context, ticker string, limit int, viewerID string) ([]store.Comment, error) {
	// $1 is the viewer. Anon → NULL (not ""), because comment_likes.user_id is a
	// uuid column and binding "" errors with 22P02 (invalid uuid); NULL simply
	// never matches, so liked=false.
	var viewer any = viewerID
	if viewerID == "" {
		viewer = nil
	}
	args := []any{viewer}
	const cols = `id, user_id, author, COALESCE(ticker,''), body, created_at, edited_at,
		(SELECT count(*) FROM comment_likes cl WHERE cl.comment_id = comments.id) AS likes,
		EXISTS(SELECT 1 FROM comment_likes clv WHERE clv.comment_id = comments.id AND clv.user_id = $1::uuid) AS liked`
	var where string
	if ticker == "" {
		where = `WHERE ticker IS NULL AND NOT deleted`
	} else {
		args = append(args, ticker) // $2
		where = `WHERE (ticker = $2 OR id IN (SELECT comment_id FROM comment_mentions WHERE ticker = $2)) AND NOT deleted`
	}
	query := `SELECT ` + cols + ` FROM comments ` + where + ` ORDER BY created_at DESC`
	if limit > 0 {
		args = append(args, limit)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list comments: %w", err)
	}
	defer rows.Close()
	var out []store.Comment
	for rows.Next() {
		var c store.Comment
		if err := rows.Scan(&c.ID, &c.UserID, &c.Author, &c.Ticker, &c.Body, &c.CreatedAt, &c.EditedAt, &c.Likes, &c.Liked); err != nil {
			return nil, fmt.Errorf("postgres: scan comment: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteComment soft-deletes a comment (kept for moderation audit). admin=true
// skips the author check; otherwise only the author can delete. found=false when
// the id is unknown or not permitted.
func (s *Store) DeleteComment(ctx context.Context, id, userID string, admin bool) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE comments SET deleted = true WHERE id = $1 AND NOT deleted AND ($3 OR user_id = $2)`,
		id, userID, admin)
	if err != nil {
		return false, fmt.Errorf("postgres: delete comment: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// UpdateComment edits a comment's body and replaces its cashtag mentions with
// the new body's set. Only the author may edit (user_id match); sets edited_at.
// ok=false when the id is unknown, deleted, or not the author's.
func (s *Store) UpdateComment(ctx context.Context, id, userID, body string, mentions []string) (store.Comment, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.Comment{}, false, fmt.Errorf("postgres: update comment begin: %w", err)
	}
	defer tx.Rollback(ctx)
	var c store.Comment
	err = tx.QueryRow(ctx,
		`UPDATE comments SET body = $3, edited_at = now() WHERE id = $1 AND user_id = $2 AND NOT deleted
		 RETURNING id, user_id, author, COALESCE(ticker,''), body, created_at, edited_at`,
		id, userID, body).
		Scan(&c.ID, &c.UserID, &c.Author, &c.Ticker, &c.Body, &c.CreatedAt, &c.EditedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Comment{}, false, nil
	}
	if err != nil {
		return store.Comment{}, false, fmt.Errorf("postgres: update comment: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM comment_mentions WHERE comment_id = $1`, id); err != nil {
		return store.Comment{}, false, fmt.Errorf("postgres: update comment mentions: %w", err)
	}
	for _, m := range mentions {
		if _, err := tx.Exec(ctx,
			`INSERT INTO comment_mentions (comment_id, ticker) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			id, m); err != nil {
			return store.Comment{}, false, fmt.Errorf("postgres: update comment mention: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return store.Comment{}, false, fmt.Errorf("postgres: update comment commit: %w", err)
	}
	c.Mentions = mentions
	return c, true, nil
}

// LikeComment toggles userID's like on a comment (one like per user), returning
// the new liked state for the user and the total like count. ok=false when the
// comment is unknown or deleted.
func (s *Store) LikeComment(ctx context.Context, id, userID string) (bool, int, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, 0, false, fmt.Errorf("postgres: like comment begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM comments WHERE id = $1 AND NOT deleted)`, id).Scan(&exists); err != nil {
		return false, 0, false, fmt.Errorf("postgres: like comment exists: %w", err)
	}
	if !exists {
		return false, 0, false, nil
	}
	var alreadyLiked bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM comment_likes WHERE comment_id = $1 AND user_id = $2)`, id, userID).Scan(&alreadyLiked); err != nil {
		return false, 0, false, fmt.Errorf("postgres: like comment check: %w", err)
	}
	liked := !alreadyLiked
	if liked {
		_, err = tx.Exec(ctx, `INSERT INTO comment_likes (comment_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, id, userID)
	} else {
		_, err = tx.Exec(ctx, `DELETE FROM comment_likes WHERE comment_id = $1 AND user_id = $2`, id, userID)
	}
	if err != nil {
		return false, 0, false, fmt.Errorf("postgres: like comment toggle: %w", err)
	}
	var likes int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM comment_likes WHERE comment_id = $1`, id).Scan(&likes); err != nil {
		return false, 0, false, fmt.Errorf("postgres: like comment count: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, 0, false, fmt.Errorf("postgres: like comment commit: %w", err)
	}
	return liked, likes, true, nil
}

// ReportComment flags a comment for moderation (increments its report count).
func (s *Store) ReportComment(ctx context.Context, id string) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE comments SET reports = reports + 1, flagged = true WHERE id = $1 AND NOT deleted`, id)
	if err != nil {
		return false, fmt.Errorf("postgres: report comment: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
