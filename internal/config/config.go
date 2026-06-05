// Package config loads runtime configuration from the environment.
package config

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	Port           string
	EDGARUserAgent string
	Watchlist      []string
	StoreBackend   string // memory | postgres
	DatabaseURL    string // used when StoreBackend == "postgres"
	IngestEvery    time.Duration

	// Alpaca market data (US prices, all sessions incl. overnight). Empty keys
	// disable price polling. Use an unfunded/paper account — data only.
	AlpacaKeyID    string
	AlpacaSecret   string
	AlpacaDataURL  string // default https://data.alpaca.markets
	AlpacaFeed     string // iex (free) | sip | overnight
	PricePollEvery time.Duration

	// Finnhub company news. Empty token disables news ingestion.
	FinnhubToken string

	// Optional LLM enrichment (OpenAI-compatible). Empty key disables it.
	LLMAPIKey  string
	LLMBaseURL string // default https://api.openai.com/v1
	LLMModel   string // default gpt-4o-mini
}

func Load() Config {
	return Config{
		Port:           env("PORT", "8080"),
		EDGARUserAgent: env("EDGAR_USER_AGENT", "Tickwind/0.1 (contact@tickwind.com)"),
		Watchlist:      splitCSV(env("WATCHLIST", "AAPL,NVDA,TSLA")),
		StoreBackend:   env("STORE_BACKEND", "memory"),
		DatabaseURL:    env("DATABASE_URL", "postgres://tickwind:tickwind@localhost:5432/tickwind?sslmode=disable"),
		IngestEvery:    envDur("INGEST_EVERY", 15*time.Minute),
		AlpacaKeyID:    env("ALPACA_API_KEY", ""),
		AlpacaSecret:   env("ALPACA_API_SECRET", ""),
		AlpacaDataURL:  env("ALPACA_DATA_URL", ""),
		AlpacaFeed:     env("ALPACA_FEED", "iex"),
		PricePollEvery: envDur("PRICE_POLL_EVERY", 10*time.Second),
		FinnhubToken:   env("FINNHUB_TOKEN", ""),
		LLMAPIKey:      env("LLM_API_KEY", ""),
		LLMBaseURL:     env("LLM_BASE_URL", ""),
		LLMModel:       env("LLM_MODEL", ""),
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
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
