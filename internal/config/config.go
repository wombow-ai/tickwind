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
}

func Load() Config {
	return Config{
		Port:           env("PORT", "8080"),
		EDGARUserAgent: env("EDGAR_USER_AGENT", "Tickwind/0.1 (contact@tickwind.com)"),
		Watchlist:      splitCSV(env("WATCHLIST", "AAPL,NVDA,TSLA")),
		StoreBackend:   env("STORE_BACKEND", "memory"),
		DatabaseURL:    env("DATABASE_URL", "postgres://tickwind:tickwind@localhost:5432/tickwind?sslmode=disable"),
		IngestEvery:    envDur("INGEST_EVERY", 15*time.Minute),
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
