-- Tickwind schema. Idempotent: safe to execute on every startup.

CREATE TABLE IF NOT EXISTS securities (
    ticker     text PRIMARY KEY,
    cik        text,
    name       text,
    market     text,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS filings (
    ticker       text NOT NULL,
    accession_no text NOT NULL,
    form         text,
    title        text,
    filed_at     timestamptz,
    url          text,
    PRIMARY KEY (ticker, accession_no)
);

CREATE INDEX IF NOT EXISTS filings_ticker_filed_at_idx
    ON filings (ticker, filed_at DESC);

CREATE TABLE IF NOT EXISTS quotes (
    ticker        text PRIMARY KEY,
    price         double precision NOT NULL,
    prev_close    double precision,
    regular_close double precision,
    session       text,
    source        text,
    at            timestamptz,
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- Add prev_close / regular_close to pre-existing quotes tables (idempotent).
ALTER TABLE quotes ADD COLUMN IF NOT EXISTS prev_close double precision;
ALTER TABLE quotes ADD COLUMN IF NOT EXISTS regular_close double precision;

CREATE TABLE IF NOT EXISTS news (
    ticker    text NOT NULL,
    id        text NOT NULL,
    headline  text,
    summary   text,
    source    text,
    url       text,
    published timestamptz,
    PRIMARY KEY (ticker, id)
);

CREATE INDEX IF NOT EXISTS news_ticker_published_idx
    ON news (ticker, published DESC);

CREATE TABLE IF NOT EXISTS social (
    ticker     text NOT NULL,
    id         text NOT NULL,
    source     text,
    author     text,
    body       text,
    url        text,
    created_at timestamptz,
    PRIMARY KEY (ticker, id)
);

CREATE INDEX IF NOT EXISTS social_ticker_created_at_idx
    ON social (ticker, created_at DESC);

-- Per-ticker numeric pulse (mention-momentum / news sentiment). One row per
-- (ticker, source); a rolled-up snapshot rather than a feed of items.
CREATE TABLE IF NOT EXISTS signals (
    ticker        text NOT NULL,
    source        text NOT NULL,
    kind          text NOT NULL DEFAULT '',
    mentions      integer NOT NULL DEFAULT 0,
    mentions_prev integer NOT NULL DEFAULT 0,
    rank          integer NOT NULL DEFAULT 0,
    rank_prev     integer NOT NULL DEFAULT 0,
    upvotes       integer NOT NULL DEFAULT 0,
    score         double precision NOT NULL DEFAULT 0,
    label         text NOT NULL DEFAULT '',
    sample_size   integer NOT NULL DEFAULT 0,
    updated_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (ticker, source)
);

-- Trending leaderboard boards (hot / surging / …). One ranked snapshot per
-- board, replaced wholesale on each refresh — fully ephemeral (regenerated
-- within one ingest cycle), so we DROP+CREATE to absorb shape changes without
-- migration ceremony rather than ALTER an existing single-board table.
DROP TABLE IF EXISTS hotlist;
CREATE TABLE hotlist (
    board          text NOT NULL,
    ticker         text NOT NULL,
    name           text NOT NULL DEFAULT '',
    rank           integer NOT NULL DEFAULT 0,
    mentions       integer NOT NULL DEFAULT 0,
    mentions_prev  integer NOT NULL DEFAULT 0,
    mention_change double precision NOT NULL DEFAULT 0,
    upvotes        integer NOT NULL DEFAULT 0,
    score          double precision NOT NULL DEFAULT 0,
    updated_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (board, ticker)
);

CREATE INDEX hotlist_board_rank_idx ON hotlist (board, rank);

-- Insider open-market purchases (Form 4, code P), the persistent corpus behind
-- the Opportunity board. Public-domain SEC data; deduped on accession.
CREATE TABLE IF NOT EXISTS insider_buys (
    accession   text PRIMARY KEY,
    ticker      text NOT NULL,
    cik         integer NOT NULL DEFAULT 0,
    company     text NOT NULL DEFAULT '',
    owner_name  text NOT NULL DEFAULT '',
    title       text NOT NULL DEFAULT '',
    is_officer  boolean NOT NULL DEFAULT false,
    is_director boolean NOT NULL DEFAULT false,
    filed_date  timestamptz,
    shares      double precision NOT NULL DEFAULT 0,
    price       double precision NOT NULL DEFAULT 0,
    value       double precision NOT NULL DEFAULT 0,
    filing_url  text NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS insider_buys_filed_idx ON insider_buys (filed_date DESC);

CREATE TABLE IF NOT EXISTS earnings (
    ticker           text NOT NULL,
    edate            date NOT NULL,
    hour             text NOT NULL DEFAULT '',
    eps_estimate     double precision,
    eps_actual       double precision,
    revenue_estimate double precision,
    revenue_actual   double precision,
    PRIMARY KEY (ticker, edate)
);
CREATE INDEX IF NOT EXISTS earnings_date_idx ON earnings (edate);

-- Form-4 accessions already fetched (a buy or not), so a restart skips
-- re-fetching them instead of re-sweeping the SEC index. Public-domain metadata.
CREATE TABLE IF NOT EXISTS seen_form4 (
    accession   text PRIMARY KEY,
    filed_date  timestamptz
);

CREATE INDEX IF NOT EXISTS seen_form4_filed_idx ON seen_form4 (filed_date DESC);

-- Migrate the legacy single-tenant watchlist (ticker PK, no user) to per-user.
-- Runs at most once: the condition is false after user_id exists.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name = 'watchlist' AND column_name = 'ticker')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns
                       WHERE table_name = 'watchlist' AND column_name = 'user_id') THEN
        DROP TABLE watchlist;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS watchlist (
    user_id  uuid NOT NULL,
    ticker   text NOT NULL,
    added_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, ticker)
);

CREATE TABLE IF NOT EXISTS clips (
    id         text PRIMARY KEY,
    user_id    uuid NOT NULL,
    ticker     text NOT NULL,
    title      text,
    url        text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS clips_user_ticker_created_idx
    ON clips (user_id, ticker, created_at DESC);

-- Private per-user notes/opinions, attached to a stock (ticker), a calendar date
-- (note_date), both, or neither. Per-user → routed to the User (local) store by
-- store.Split; keyed by the Supabase JWT sub. Editable, so updated_at is tracked.
CREATE TABLE IF NOT EXISTS notes (
    id         text PRIMARY KEY,
    user_id    uuid NOT NULL,
    ticker     text,
    note_date  date,
    body       text NOT NULL DEFAULT '',
    pinned     boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS notes_user_ticker_created_idx
    ON notes (user_id, ticker, created_at DESC);
CREATE INDEX IF NOT EXISTS notes_user_date_idx
    ON notes (user_id, note_date) WHERE note_date IS NOT NULL;
CREATE INDEX IF NOT EXISTS notes_user_created_idx
    ON notes (user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS alerts (
    id         text PRIMARY KEY,
    user_id    uuid NOT NULL,
    ticker     text NOT NULL,
    kind       text NOT NULL,
    threshold  double precision NOT NULL DEFAULT 0,
    active     boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    triggered_at timestamptz
);
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS triggered_at timestamptz;
CREATE INDEX IF NOT EXISTS alerts_user_created_idx
    ON alerts (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS alerts_active_idx
    ON alerts (ticker) WHERE active AND triggered_at IS NULL;

CREATE TABLE IF NOT EXISTS holdings (
    id         text PRIMARY KEY,
    user_id    uuid NOT NULL,
    ticker     text NOT NULL,
    shares     double precision NOT NULL DEFAULT 0,
    avg_cost   double precision NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, ticker)
);
CREATE INDEX IF NOT EXISTS holdings_user_idx ON holdings (user_id);

-- Public user comments on a stock (ticker) or the global community board
-- (ticker IS NULL). Durable (Market store). Soft-deleted (deleted=true) for
-- moderation audit; ip + flagged/reports support takedown/abuse handling.
CREATE TABLE IF NOT EXISTS comments (
    id         text PRIMARY KEY,
    user_id    uuid NOT NULL,
    author     text NOT NULL DEFAULT '',
    ticker     text,
    body       text NOT NULL,
    ip         text,
    flagged    boolean NOT NULL DEFAULT false,
    reports    integer NOT NULL DEFAULT 0,
    deleted    boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    edited_at  timestamptz
);
-- Idempotent add for existing deployments (CREATE TABLE IF NOT EXISTS won't alter).
ALTER TABLE comments ADD COLUMN IF NOT EXISTS edited_at timestamptz;

CREATE INDEX IF NOT EXISTS comments_ticker_created_idx
    ON comments (ticker, created_at DESC) WHERE NOT deleted;
CREATE INDEX IF NOT EXISTS comments_global_created_idx
    ON comments (created_at DESC) WHERE ticker IS NULL AND NOT deleted;

-- One like per (comment, user); toggled by LikeComment. Count is derived.
CREATE TABLE IF NOT EXISTS comment_likes (
    comment_id text NOT NULL,
    user_id    uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (comment_id, user_id)
);
CREATE INDEX IF NOT EXISTS comment_likes_comment_idx ON comment_likes (comment_id);

-- $TICKER cashtags extracted from a comment's body at write time. A comment
-- mentioning $RKLB also shows in RKLB's comment list (ListComments unions
-- direct ticker + mentions). Replaced wholesale on edit.
CREATE TABLE IF NOT EXISTS comment_mentions (
    comment_id text NOT NULL,
    ticker     text NOT NULL,
    PRIMARY KEY (comment_id, ticker)
);
CREATE INDEX IF NOT EXISTS comment_mentions_ticker_idx ON comment_mentions (ticker);
