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
-- AI-translated Chinese headline (translate ingestor); idempotent add for
-- existing deployments. Written once per row — headlines are immutable.
ALTER TABLE news ADD COLUMN IF NOT EXISTS headline_zh text;

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

-- Daily headline Fear & Greed score, persisted so the sentiment-history curve
-- survives redeploys (the live index is computed into an in-memory cache).
-- Public market data → durable Market store. One row per calendar day.
CREATE TABLE IF NOT EXISTS fear_greed (
    day        date PRIMARY KEY,
    score      int NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Per-stock AI digest (the LLM summary at GET /v1/stocks/{ticker}/summary),
-- persisted so it survives redeploys (the in-memory cache is wiped on restart →
-- the next visitor would otherwise pay a fresh LLM generation). Keyed by
-- (ticker, ET trading day, lang); the day is the ~1-day TTL boundary, so a
-- new day generates afresh and yesterday's rows go stale (pruned/ignored).
-- Costs tokens to regenerate → durable (Market store). One row per key.
CREATE TABLE IF NOT EXISTS ai_summary (
    ticker     text NOT NULL,
    day        date NOT NULL,
    lang       text NOT NULL,
    payload    bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (ticker, day, lang)
);

CREATE INDEX IF NOT EXISTS ai_summary_day_idx ON ai_summary (day);

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

-- Per-user opaque JSON preferences blob (selected indicators, future UI prefs).
-- The API owns the shape (namespaced top-level keys) and caps the size; the
-- store treats it as opaque jsonb. One row per user, upserted on the PK.
CREATE TABLE IF NOT EXISTS user_prefs (
    user_id    text PRIMARY KEY,
    prefs      jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Per-user, per-MONTH generation quota for the AI Deep Research report
-- (depth=deep): each user gets a small number of NEW deep-report generations
-- per ET CALENDAR MONTH, site-wide (not per stock; free = 1 report/user/month).
-- The `day` column is reused as the PERIOD key and now holds an ET-month string
-- ("2026-06" style, America/New_York) rather than a per-day date — so one row per
-- (user, month), upserted/incremented on a genuinely-new generation; viewing a
-- globally cached report never touches this. Old per-day rows ("2026-06-15") from
-- the previous daily scheme never match a month key, so they are harmless dead
-- weight. Cheap to rebuild (User store) — losing it just resets the period's
-- quota. Old periods accumulate harmlessly (pruneable later).
CREATE TABLE IF NOT EXISTS deep_research_quota (
    user_id    text NOT NULL,
    day        text NOT NULL, -- reused as the ET-month period key (e.g. "2026-06")
    used       integer NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, day)
);

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

-- Stripe-synced per-user Pro entitlement — the SINGLE SOURCE OF TRUTH for a user's
-- tier, written by the Stripe webhook and read O(1) on the gate hot path (never a
-- Stripe API call per request). DURABLE (Market store): billing is NOT cheap to
-- rebuild like watchlist/quota. One row per Supabase user (the JWT sub). `tier` is
-- the derived entitlement ("pro" when status is active/trialing, else "free");
-- current_period_end powers a small renewal grace window. Inert until Stripe is
-- configured (no rows are written without a webhook).
CREATE TABLE IF NOT EXISTS subscriptions (
    user_id                uuid PRIMARY KEY,
    stripe_customer_id     text NOT NULL DEFAULT '',
    stripe_subscription_id text NOT NULL DEFAULT '',
    status                 text NOT NULL DEFAULT '',
    tier                   text NOT NULL DEFAULT 'free',
    price_id               text NOT NULL DEFAULT '',
    plan_interval          text NOT NULL DEFAULT '',
    current_period_end     timestamptz NOT NULL DEFAULT now(),
    cancel_at_period_end   boolean NOT NULL DEFAULT false,
    updated_at             timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS subscriptions_customer_idx ON subscriptions (stripe_customer_id);

-- Stripe webhook idempotency ledger. Stripe delivers events at-least-once and out
-- of order; MarkStripeEventSeen INSERTs the id and a conflict means "already
-- processed -> skip". Durable (Market store) so a redeploy never reprocesses one.
CREATE TABLE IF NOT EXISTS stripe_events (
    event_id text PRIMARY KEY,
    type     text NOT NULL DEFAULT '',
    seen_at  timestamptz NOT NULL DEFAULT now()
);
