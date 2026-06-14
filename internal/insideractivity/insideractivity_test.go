package insideractivity

import (
	"context"
	"errors"
	"testing"

	"github.com/wombow-ai/tickwind/internal/edgar"
)

// fakeFetcher is a controllable Fetcher: it returns the held transactions or err.
type fakeFetcher struct {
	txns []edgar.InsiderTransaction
	err  error
}

func (f *fakeFetcher) InsiderActivity(context.Context, string) ([]edgar.InsiderTransaction, error) {
	return f.txns, f.err
}

// TestReportAggregates: buy/sell counts and net value (buy $ − sell $) are
// computed over the returned transactions, and the ticker is upper-cased.
func TestReportAggregates(t *testing.T) {
	ff := &fakeFetcher{txns: []edgar.InsiderTransaction{
		{Type: "sell", Owner: "Cook Timothy D", Role: "CEO", Shares: 100, Price: 200, Value: 20000, Date: "2026-05-20", Planned10b5_1: true},
		{Type: "buy", Owner: "Doe Jane", Role: "Director", Shares: 50, Price: 100, Value: 5000, Date: "2026-05-18"},
	}}
	svc := NewService(ff)

	rep, err := svc.Report(context.Background(), "aapl")
	if err != nil {
		t.Fatalf("Report err: %v", err)
	}
	if rep.Ticker != "AAPL" {
		t.Errorf("ticker = %q, want AAPL (upper-cased)", rep.Ticker)
	}
	if len(rep.Transactions) != 2 {
		t.Fatalf("got %d transactions, want 2", len(rep.Transactions))
	}
	if rep.BuyCount != 1 || rep.SellCount != 1 {
		t.Errorf("buy_count=%d sell_count=%d, want 1/1", rep.BuyCount, rep.SellCount)
	}
	if rep.NetValue != 5000-20000 {
		t.Errorf("net_value = %v, want %v (buy $ − sell $)", rep.NetValue, 5000.0-20000.0)
	}
	// The 10b5-1 flag is passed through untouched (never altered by the service).
	if !rep.Transactions[0].Planned10b5_1 {
		t.Error("10b5-1 flag was dropped by the service")
	}
}

// TestReportEmptyIsSlice: a company with zero recent Form 4s yields an empty
// (non-nil) Transactions slice and nil error (handler → {"transactions":[]}/200).
func TestReportEmptyIsSlice(t *testing.T) {
	svc := NewService(&fakeFetcher{txns: nil})
	rep, err := svc.Report(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("Report err: %v", err)
	}
	if rep.Transactions == nil {
		t.Error("Transactions is nil (must be a non-nil empty slice)")
	}
	if len(rep.Transactions) != 0 || rep.BuyCount != 0 || rep.SellCount != 0 || rep.NetValue != 0 {
		t.Errorf("non-empty aggregates for an empty timeline: %+v", rep)
	}
}

// TestReportPropagatesFetchError: an unresolved ticker / feed failure propagates
// so the handler can 404.
func TestReportPropagatesFetchError(t *testing.T) {
	svc := NewService(&fakeFetcher{err: errors.New("ticker not found")})
	if _, err := svc.Report(context.Background(), "ZZZZ"); err == nil {
		t.Error("expected a fetch error to propagate")
	}
}

// TestReportNilFetcher: a nil fetcher errors (the handler 404s) rather than
// panicking.
func TestReportNilFetcher(t *testing.T) {
	svc := NewService(nil)
	if _, err := svc.Report(context.Background(), "AAPL"); err == nil {
		t.Error("expected an error with a nil fetcher")
	}
}
