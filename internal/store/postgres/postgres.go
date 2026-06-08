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

// SaveComment inserts a public comment (empty ticker → NULL = global board).
func (s *Store) SaveComment(ctx context.Context, c store.Comment) error {
	const query = `
INSERT INTO comments (id, user_id, author, ticker, body, ip, created_at)
VALUES ($1, $2, $3, NULLIF($4,''), $5, NULLIF($6,''), $7)`
	if _, err := s.pool.Exec(ctx, query, c.ID, c.UserID, c.Author, c.Ticker, c.Body, c.IP, c.CreatedAt); err != nil {
		return fmt.Errorf("postgres: save comment: %w", err)
	}
	return nil
}

// ListComments returns non-deleted comments for a ticker ("" = the global board,
// i.e. ticker IS NULL), newest first.
func (s *Store) ListComments(ctx context.Context, ticker string, limit int) ([]store.Comment, error) {
	var query string
	var args []any
	if ticker == "" {
		query = `SELECT id, user_id, author, COALESCE(ticker,''), body, created_at FROM comments WHERE ticker IS NULL AND NOT deleted ORDER BY created_at DESC`
	} else {
		query = `SELECT id, user_id, author, COALESCE(ticker,''), body, created_at FROM comments WHERE ticker = $1 AND NOT deleted ORDER BY created_at DESC`
		args = append(args, ticker)
	}
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
		if err := rows.Scan(&c.ID, &c.UserID, &c.Author, &c.Ticker, &c.Body, &c.CreatedAt); err != nil {
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

// ReportComment flags a comment for moderation (increments its report count).
func (s *Store) ReportComment(ctx context.Context, id string) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE comments SET reports = reports + 1, flagged = true WHERE id = $1 AND NOT deleted`, id)
	if err != nil {
		return false, fmt.Errorf("postgres: report comment: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
