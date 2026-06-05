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
