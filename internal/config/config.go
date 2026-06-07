// Package config loads runtime configuration from the environment.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port           string
	EDGARUserAgent string
	Watchlist      []string
	StoreBackend   string // memory | postgres
	DatabaseURL    string // single-store fallback (StoreBackend == "postgres")
	// Split storage (optional): when both are set, collected/market data
	// (securities, filings, quotes, news, social) goes to the durable
	// MarketDatabaseURL (e.g. managed/backed-up Postgres) and per-user data
	// (watchlist, clips) goes to the local UserDatabaseURL (cheap/ephemeral —
	// losing it just means users re-add their tickers). Empty → use DatabaseURL.
	MarketDatabaseURL string
	UserDatabaseURL   string
	IngestEvery       time.Duration

	// Alpaca market data (US prices, all sessions incl. overnight). Empty keys
	// disable price polling. Use an unfunded/paper account — data only.
	AlpacaKeyID    string
	AlpacaSecret   string
	AlpacaDataURL  string // default https://data.alpaca.markets
	AlpacaFeed     string // iex (free) | sip | overnight
	PricePollEvery time.Duration

	// Finnhub company news. Empty token disables news ingestion.
	FinnhubToken string

	// Social sources (optional; empty disables that source). Reddit needs a
	// "script" app + a bot account; Bluesky needs a handle + an app password.
	RedditClientID     string
	RedditSecret       string
	RedditUsername     string
	RedditPassword     string
	BlueskyHandle      string
	BlueskyAppPassword string

	// Alpha Vantage NEWS_SENTIMENT (per-ticker news sentiment). Free tier is
	// 25 requests/day, so the client self-budgets + caches. Empty disables it.
	AlphaVantageKey string

	// Korea markets (optional). KRXAPIKey enables KOSPI/KOSDAQ EOD prices via the
	// KRX Open API; OpenDARTKey adds KR filings. Both free; KRX is required for
	// Korea, DART is an add-on. Empty → Korea disabled.
	KRXAPIKey   string
	OpenDARTKey string

	// OpportunityBackfillDays seeds the Opportunity board with this many days of
	// SEC Form-4 history on startup (it then accumulates forward to a 30d window).
	// Higher = a fuller initial board but a longer startup sweep.
	OpportunityBackfillDays int

	// Optional LLM enrichment (OpenAI-compatible). Empty key disables it.
	LLMAPIKey  string
	LLMBaseURL string // default https://api.openai.com/v1
	LLMModel   string // default gpt-4o-mini

	// Supabase auth. SupabaseURL (e.g. https://<ref>.supabase.co) enables ES256
	// verification via the project's JWKS — required because Supabase now signs
	// user tokens with asymmetric keys. SupabaseJWTSecret keeps legacy HS256
	// working too. With neither set, all private endpoints 401. Watchlist below
	// is the default ticker set always ingested (so public stock pages have
	// data), unioned with every user's watchlist.
	SupabaseURL       string
	SupabaseJWTSecret string
}

func Load() Config {
	return Config{
		Port:                    env("PORT", "8080"),
		EDGARUserAgent:          env("EDGAR_USER_AGENT", "Tickwind/0.1 (contact@tickwind.com)"),
		Watchlist:               splitCSV(env("WATCHLIST", "AAPL,NVDA,TSLA,MSFT,AMZN,GOOGL,META,AMD,NFLX,AVGO")),
		StoreBackend:            env("STORE_BACKEND", "memory"),
		DatabaseURL:             env("DATABASE_URL", "postgres://tickwind:tickwind@localhost:5432/tickwind?sslmode=disable"),
		MarketDatabaseURL:       env("MARKET_DATABASE_URL", ""),
		UserDatabaseURL:         env("USER_DATABASE_URL", ""),
		IngestEvery:             envDur("INGEST_EVERY", 15*time.Minute),
		AlpacaKeyID:             env("ALPACA_API_KEY", ""),
		AlpacaSecret:            env("ALPACA_API_SECRET", ""),
		AlpacaDataURL:           env("ALPACA_DATA_URL", ""),
		AlpacaFeed:              env("ALPACA_FEED", "iex"),
		PricePollEvery:          envDur("PRICE_POLL_EVERY", 10*time.Second),
		FinnhubToken:            env("FINNHUB_TOKEN", ""),
		RedditClientID:          env("REDDIT_CLIENT_ID", ""),
		RedditSecret:            env("REDDIT_CLIENT_SECRET", ""),
		RedditUsername:          env("REDDIT_USERNAME", ""),
		RedditPassword:          env("REDDIT_PASSWORD", ""),
		BlueskyHandle:           env("BLUESKY_HANDLE", ""),
		BlueskyAppPassword:      env("BLUESKY_APP_PASSWORD", ""),
		AlphaVantageKey:         env("ALPHAVANTAGE_API_KEY", ""),
		KRXAPIKey:               env("KRX_API_KEY", ""),
		OpenDARTKey:             env("OPENDART_API_KEY", ""),
		OpportunityBackfillDays: envInt("OPPORTUNITY_BACKFILL_DAYS", 3),
		LLMAPIKey:               env("LLM_API_KEY", ""),
		LLMBaseURL:              env("LLM_BASE_URL", ""),
		LLMModel:                env("LLM_MODEL", ""),
		SupabaseURL:             strings.TrimRight(env("SUPABASE_URL", ""), "/"),
		SupabaseJWTSecret:       env("SUPABASE_JWT_SECRET", ""),
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envDur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, strings.ToUpper(p))
		}
	}
	return out
}
