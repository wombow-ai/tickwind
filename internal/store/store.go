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

// Store is the persistence boundary. Every backend (memory, postgres)
// implements this so the rest of the app never depends on a driver.
type Store interface {
	UpsertSecurity(ctx context.Context, s Security) error
	GetSecurity(ctx context.Context, ticker string) (Security, bool, error)

	SaveFilings(ctx context.Context, ticker string, filings []Filing) error
	ListFilings(ctx context.Context, ticker string, limit int) ([]Filing, error)
}
