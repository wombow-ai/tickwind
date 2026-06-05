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
    session    text,
    source     text,
    at         timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now()
);

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

CREATE TABLE IF NOT EXISTS watchlist (
    ticker   text PRIMARY KEY,
    added_at timestamptz NOT NULL DEFAULT now()
);
