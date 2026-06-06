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
    ticker     text PRIMARY KEY,
    price      double precision NOT NULL,
    prev_close double precision,
    session    text,
    source     text,
    at         timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Add prev_close to pre-existing quotes tables (idempotent).
ALTER TABLE quotes ADD COLUMN IF NOT EXISTS prev_close double precision;

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
