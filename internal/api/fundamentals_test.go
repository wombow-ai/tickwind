package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wombow-ai/tickwind/internal/auth"
	"github.com/wombow-ai/tickwind/internal/edgar"
	"github.com/wombow-ai/tickwind/internal/enrich"
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/store/memory"
	"github.com/wombow-ai/tickwind/internal/stream"
)

type fakeFundamentals struct {
	f   edgar.Fundamentals
	err error
}

func (f fakeFundamentals) Fundamentals(context.Context, string) (edgar.Fundamentals, error) {
	return f.f, f.err
}

func fundServer(t *testing.T, q store.Quote, f edgar.Fundamentals) *httptest.Server {
	t.Helper()
	st := memory.New()
	if err := st.UpsertQuote(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	h := New(
		st, stream.NewHub(), enrich.Noop{},
		auth.NewVerifier(testSecret, ""),
		nil, nil, nil, nil, nil, nil, nil, nil, // bars, topics, opps, universe, gurus, ingestor, symbols, events
		fakeFundamentals{f: f},
		nil, // admins
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	return httptest.NewServer(h)
}

type fundJSON struct {
	Ticker    string   `json:"ticker"`
	Revenue   float64  `json:"revenue"`
	NetIncome float64  `json:"net_income"`
	Price     float64  `json:"price"`
	MarketCap *float64 `json:"market_cap"`
	PE        *float64 `json:"pe"`
	PB        *float64 `json:"pb"`
}

func getFund(t *testing.T, srv *httptest.Server, ticker string) (int, fundJSON) {
	t.Helper()
	resp, err := http.Get(srv.URL + "/v1/stocks/" + ticker + "/fundamentals")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got fundJSON
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
	}
	return resp.StatusCode, got
}

func TestGetFundamentals_Profitable(t *testing.T) {
	srv := fundServer(t,
		store.Quote{Ticker: "AAPL", Price: 100},
		edgar.Fundamentals{Ticker: "AAPL", Currency: "USD", Shares: 1000, Revenue: 5000, NetIncome: 800, EPSDiluted: 4, Equity: 2000, Period: "FY2024"},
	)
	defer srv.Close()

	code, got := getFund(t, srv, "AAPL")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if got.Revenue != 5000 || got.Price != 100 {
		t.Errorf("revenue/price = %v/%v, want 5000/100", got.Revenue, got.Price)
	}
	if got.MarketCap == nil || *got.MarketCap != 100000 { // 100 × 1000
		t.Errorf("market_cap = %v, want 100000", got.MarketCap)
	}
	if got.PE == nil || *got.PE != 25 { // 100 / 4
		t.Errorf("pe = %v, want 25", got.PE)
	}
	if got.PB == nil || *got.PB != 50 { // 100 / (2000/1000)
		t.Errorf("pb = %v, want 50", got.PB)
	}
}

func TestGetFundamentals_LossHasNoPE(t *testing.T) {
	srv := fundServer(t,
		store.Quote{Ticker: "MSTR", Price: 120},
		edgar.Fundamentals{Ticker: "MSTR", Currency: "USD", Shares: 350, Revenue: 463, NetIncome: -1200, EPSDiluted: -3.5, Equity: 5000, Period: "FY2024"},
	)
	defer srv.Close()

	code, got := getFund(t, srv, "MSTR")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if got.PE != nil { // negative EPS → P/E omitted (frontend shows 亏损/—)
		t.Errorf("pe = %v, want null for a loss-maker", *got.PE)
	}
	if got.MarketCap == nil || *got.MarketCap != 42000 { // 120 × 350
		t.Errorf("market_cap = %v, want 42000", got.MarketCap)
	}
}

func TestGetFundamentals_NoDataIs404(t *testing.T) {
	srv := fundServer(t, store.Quote{Ticker: "ZZZZ", Price: 10}, edgar.Fundamentals{}) // HasData()==false
	defer srv.Close()
	if code, _ := getFund(t, srv, "ZZZZ"); code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for empty fundamentals", code)
	}
}
