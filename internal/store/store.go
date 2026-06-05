// Package store defines the domain types and the storage interface.
// v1 ships an in-memory implementation; a Postgres (TimescaleDB + pgvector)
// implementation is added when we deploy to the server.
package store

import (
	"context"
	"time"
)

// Security is a tracked instrument.
type Security struct {
	Ticker string `json:"ticker"`
	CIK    string `json:"cik,omitempty"`
	Name   string `json:"name"`
	Market string `json:"market"` // US | HK | KR
}

// Filing is a regulatory disclosure (e.g. 8-K, 10-Q, Form 4).
type Filing struct {
	Ticker      string    `json:"ticker"`
	Form        string    `json:"form"`
	Title       string    `json:"title"`
	FiledAt     time.Time `json:"filed_at"`
	AccessionNo string    `json:"accession_no"`
	URL         string    `json:"url"`
}

// Quote is the latest traded price for a security. It covers all trading
// sessions — pre-market, regular, after-hours and overnight.
type Quote struct {
	Ticker  string    `json:"ticker"`
	Price   float64   `json:"price"`
	Session string    `json:"session"` // pre | regular | post | overnight | closed
	Source  string    `json:"source"`
	At      time.Time `json:"at"`
}

// News is a company-news article for a security.
type News struct {
	Ticker    string    `json:"ticker"`
	ID        string    `json:"id"` // source-assigned id, used for dedupe
	Headline  string    `json:"headline"`
	Summary   string    `json:"summary"`
	Source    string    `json:"source"`
	URL       string    `json:"url"`
	Published time.Time `json:"published"`
}

// Post is a social-media message about a security (e.g. from StockTwits).
type Post struct {
	Ticker    string    `json:"ticker"`
	ID        string    `json:"id"` // "<source>:<rawid>", used for dedupe
	Source    string    `json:"source"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

// Store is the persistence boundary. Every backend (memory, postgres)
// implements this so the rest of the app never depends on a driver.
type Store interface {
	UpsertSecurity(ctx context.Context, s Security) error
	GetSecurity(ctx context.Context, ticker string) (Security, bool, error)

	SaveFilings(ctx context.Context, ticker string, filings []Filing) error
	ListFilings(ctx context.Context, ticker string, limit int) ([]Filing, error)

	UpsertQuote(ctx context.Context, q Quote) error
	GetQuote(ctx context.Context, ticker string) (Quote, bool, error)

	SaveNews(ctx context.Context, ticker string, items []News) error
	ListNews(ctx context.Context, ticker string, limit int) ([]News, error)

	SaveSocial(ctx context.Context, ticker string, posts []Post) error
	ListSocial(ctx context.Context, ticker string, limit int) ([]Post, error)
}
