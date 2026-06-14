// Package config loads runtime configuration from the environment.
package config

import (
	"net/http"
	"net/url"
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
	// AlpacaWSURL is the real-time trade WebSocket endpoint (free IEX feed);
	// AlpacaWSEnabled gates the live streamer (default on when keys are present).
	AlpacaWSURL     string
	AlpacaWSEnabled bool
	// UniverseSweepEvery: how often the whole-US-universe price cache refreshes.
	UniverseSweepEvery time.Duration
	// CongressSweepEvery: how often the congressional-PTR cache refreshes (House
	// Clerk public-domain disclosures; the index updates roughly daily).
	CongressSweepEvery time.Duration
	// InstitutionalSweepEvery: how often the SEC 13D/13G beneficial-ownership
	// cache refreshes (daily index; disseminates next business day).
	InstitutionalSweepEvery time.Duration

	// Finnhub company news. Empty token disables news ingestion.
	FinnhubToken string

	// ResidentialProxyURL routes outbound requests for sources that block
	// datacenter IPs (e.g. the Nasdaq IPO API, HKEXnews, Xueqiu) through a
	// residential egress. Form: http://user:pass@host:port (e.g. a dataimpulse
	// gateway). Empty → those clients use a plain http.Client (no proxy). Read
	// from env only; the credentials are never committed. See ProxyHTTPClient.
	ResidentialProxyURL string

	// Telegram broadcast (optional): an empty TelegramBotToken disables the
	// daily-briefing push. TelegramChannel is the destination (a public
	// @username like "@tickwind" or a numeric chat ID); the bot must be an
	// admin of the channel with permission to post. PublicSiteURL is the
	// public origin used to build OG share-card image URLs that Telegram
	// fetches server-side, so it must be reachable from the public internet.
	TelegramBotToken string
	TelegramChannel  string
	PublicSiteURL    string

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

	// Brazil market (optional). BRAPIKey enables B3 (.SA) delayed quotes via
	// brapi.dev. Free token; empty → Brazil disabled.
	BRAPIKey string

	// OpportunityBackfillDays seeds the Opportunity board with this many days of
	// SEC Form-4 history on startup (it then accumulates forward to a 30d window).
	// Higher = a fuller initial board but a longer startup sweep.
	OpportunityBackfillDays int

	// Optional LLM enrichment (OpenAI-compatible). Empty key disables it.
	LLMAPIKey  string
	LLMBaseURL string // default https://api.openai.com/v1
	LLMModel   string // default gpt-4o-mini
	// LLMDeepModel is the (optionally stronger) model used ONLY by the AI Deep
	// Research compose (depth=deep). It is empty by default → the deep path falls
	// back to LLMModel, so there is ZERO cost/behavior change until the owner sets
	// LLM_DEEP_MODEL (deliberate cost control: the pricey model stays off until the
	// paywall goes live).
	LLMDeepModel string

	// Supabase auth. SupabaseURL (e.g. https://<ref>.supabase.co) enables ES256
	// verification via the project's JWKS — required because Supabase now signs
	// user tokens with asymmetric keys. SupabaseJWTSecret keeps legacy HS256
	// working too. With neither set, all private endpoints 401. Watchlist below
	// is the default ticker set always ingested (so public stock pages have
	// data), unioned with every user's watchlist.
	SupabaseURL       string
	SupabaseJWTSecret string

	// AdminUserIDs are Supabase user UUIDs allowed to delete ANY comment
	// (moderation takedown); everyone else can only delete their own. Comma-
	// separated env ADMIN_USER_IDS.
	AdminUserIDs []string

	// Retention tunes the tiered Pruner (off the request path) that bounds the
	// durable market tables; see RetentionConfig.
	Retention RetentionConfig
}

// RetentionConfig tunes the tiered Pruner (internal/ingest/prune.go). A *Days
// value <= 0 disables that table's age-based prune; a cap <= 0 disables that
// per-ticker cap. Hot-list tickers keep the *Hot (longer) window, and
// ProtectSocialSources (e.g. the 大V / Serenity "substack" rail) are never pruned.
type RetentionConfig struct {
	NewsDays             int
	NewsHotDays          int
	SocialDays           int
	SocialHotDays        int
	FilingsDays          int
	InsiderDays          int
	SeenForm4Days        int
	CapNewsPerTicker     int
	CapSocialPerTicker   int
	ProtectSocialSources []string
	Every                time.Duration
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
		AlpacaWSURL:             env("ALPACA_WS_URL", "wss://stream.data.alpaca.markets/v2/iex"),
		AlpacaWSEnabled:         envBool("ALPACA_WS_ENABLED", true),
		PricePollEvery:          envDur("PRICE_POLL_EVERY", 10*time.Second),
		UniverseSweepEvery:      envDur("UNIVERSE_SWEEP_EVERY", 5*time.Minute),
		CongressSweepEvery:      envDur("CONGRESS_SWEEP_EVERY", 8*time.Hour),
		InstitutionalSweepEvery: envDur("INSTITUTIONAL_SWEEP_EVERY", 8*time.Hour),
		FinnhubToken:            env("FINNHUB_TOKEN", ""),
		ResidentialProxyURL:     env("RESIDENTIAL_PROXY_URL", ""),
		TelegramBotToken:        env("TELEGRAM_BOT_TOKEN", ""),
		TelegramChannel:         env("TELEGRAM_CHANNEL", ""),
		PublicSiteURL:           strings.TrimRight(env("PUBLIC_SITE_URL", "https://tickwind.com"), "/"),
		RedditClientID:          env("REDDIT_CLIENT_ID", ""),
		RedditSecret:            env("REDDIT_CLIENT_SECRET", ""),
		RedditUsername:          env("REDDIT_USERNAME", ""),
		RedditPassword:          env("REDDIT_PASSWORD", ""),
		BlueskyHandle:           env("BLUESKY_HANDLE", ""),
		BlueskyAppPassword:      env("BLUESKY_APP_PASSWORD", ""),
		AlphaVantageKey:         env("ALPHAVANTAGE_API_KEY", ""),
		KRXAPIKey:               env("KRX_API_KEY", ""),
		OpenDARTKey:             env("OPENDART_API_KEY", ""),
		BRAPIKey:                env("BRAPI_API_KEY", ""),
		OpportunityBackfillDays: envInt("OPPORTUNITY_BACKFILL_DAYS", 3),
		LLMAPIKey:               env("LLM_API_KEY", ""),
		LLMBaseURL:              env("LLM_BASE_URL", ""),
		LLMModel:                env("LLM_MODEL", ""),
		LLMDeepModel:            env("LLM_DEEP_MODEL", ""),
		SupabaseURL:             strings.TrimRight(env("SUPABASE_URL", ""), "/"),
		SupabaseJWTSecret:       env("SUPABASE_JWT_SECRET", ""),
		AdminUserIDs:            splitCSVRaw(env("ADMIN_USER_IDS", "")),
		Retention: RetentionConfig{
			NewsDays:             envInt("RETAIN_NEWS_DAYS", 60),
			NewsHotDays:          envInt("RETAIN_NEWS_HOT_DAYS", 120),
			SocialDays:           envInt("RETAIN_SOCIAL_DAYS", 30),
			SocialHotDays:        envInt("RETAIN_SOCIAL_HOT_DAYS", 90),
			FilingsDays:          envInt("RETAIN_FILINGS_DAYS", 730),
			InsiderDays:          envInt("RETAIN_INSIDER_DAYS", 90),
			SeenForm4Days:        envInt("RETAIN_SEEN_FORM4_DAYS", 60),
			CapNewsPerTicker:     envInt("CAP_NEWS_PER_TICKER", 500),
			CapSocialPerTicker:   envInt("CAP_SOCIAL_PER_TICKER", 500),
			ProtectSocialSources: splitCSVRaw(env("PROTECT_SOCIAL_SOURCES", "substack")),
			Every:                envDur("PRUNE_EVERY", 6*time.Hour),
		},
	}
}

// splitCSVRaw splits a comma list, trimming spaces but preserving case (unlike
// splitCSV, which upper-cases tickers) — used for source names like "substack".
func splitCSVRaw(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envBool reads a boolean env var; "0"/"false"/"no"/"off" (case-insensitive) are
// false, any other non-empty value is true, and unset uses def.
func envBool(key string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return def
	}
	return v != "0" && v != "false" && v != "no" && v != "off"
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

// ProxyHTTPClient returns an *http.Client whose transport routes requests
// through the configured ResidentialProxyURL, with the given timeout. When the
// proxy URL is empty (or unparseable), it returns a plain timeout-only client —
// so callers transparently fall back to a direct connection. This is the single
// place that wires the residential egress used by datacenter-IP-blocked sources
// (e.g. the Nasdaq IPO API).
func (c Config) ProxyHTTPClient(timeout time.Duration) *http.Client {
	if c.ResidentialProxyURL == "" {
		return &http.Client{Timeout: timeout}
	}
	parsed, err := url.Parse(c.ResidentialProxyURL)
	if err != nil || parsed.Host == "" {
		return &http.Client{Timeout: timeout}
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{Proxy: http.ProxyURL(parsed)},
	}
}
