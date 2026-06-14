// Package insideractivity assembles a company's recent insider-activity timeline
// from SEC Form 4 filings — open-market buys (code P) AND sells (code S), newest
// first, each with shares/price/value/date, the insider's name + role, and a
// best-effort Rule 10b5-1 planned-sale flag. Mirrors internal/materialevents'
// per-ticker structure, but it is PURE STRUCTURED DATA: there is NO LLM in this
// feature at all. Go owns every fact (shares, price, value = shares×price,
// transaction date, name, role, buy/sell, the 10b5-1 flag) — all parsed straight
// from the Form 4 XML (internal/sec.ParseForm4), never computed or guessed by a
// model. An absent field is omitted (empty/zero), never fabricated.
package insideractivity

import (
	"context"
	"fmt"
	"strings"

	"github.com/wombow-ai/tickwind/internal/edgar"
)

// Report is the assembled insider-activity response for one ticker. Transactions
// is newest-first and is ALWAYS non-nil (an existing company with no recent
// Form 4s yields an empty slice — never null). BuyCount/SellCount/NetValue are
// cheap Go-owned aggregates over the returned transactions (net = buy $ − sell $).
type Report struct {
	Ticker       string                     `json:"ticker"`
	Transactions []edgar.InsiderTransaction `json:"transactions"`
	BuyCount     int                        `json:"buy_count"`
	SellCount    int                        `json:"sell_count"`
	NetValue     float64                    `json:"net_value"`
}

// Fetcher is the narrow EDGAR slice the service needs: list a ticker's recent
// open-market Form 4 transactions (facts only), newest first. Satisfied by
// *edgar.Client. Returns an error only when the ticker/CIK can't be resolved or
// the feed fetch fails; an existing company with zero recent Form 4s returns an
// empty slice and nil error.
type Fetcher interface {
	InsiderActivity(ctx context.Context, ticker string) ([]edgar.InsiderTransaction, error)
}

// Service is the insider-activity façade the API handler holds. It owns only the
// EDGAR fetcher — there is no LLM anywhere in this feature.
type Service struct {
	edgar Fetcher
}

// NewService builds an insider-activity Service over the given EDGAR fetcher.
func NewService(f Fetcher) *Service {
	return &Service{edgar: f}
}

// Report assembles the insider-activity timeline for a ticker: the Go-owned
// Form 4 buy/sell transactions, newest first, plus cheap aggregates. It returns
// an error only when the ticker/CIK can't be resolved or the feed fetch fails
// (the handler 404s on that); an existing company with zero recent insider
// activity yields an empty (non-nil) Transactions slice and nil error.
func (s *Service) Report(ctx context.Context, ticker string) (Report, error) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	rep := Report{Ticker: ticker, Transactions: []edgar.InsiderTransaction{}}
	if s.edgar == nil {
		return Report{}, fmt.Errorf("insideractivity: no edgar fetcher")
	}
	txns, err := s.edgar.InsiderActivity(ctx, ticker)
	if err != nil {
		return Report{}, err
	}
	if txns != nil {
		rep.Transactions = txns
	}
	for _, t := range rep.Transactions {
		switch t.Type {
		case "buy":
			rep.BuyCount++
			rep.NetValue += t.Value
		case "sell":
			rep.SellCount++
			rep.NetValue -= t.Value
		}
	}
	return rep, nil
}
