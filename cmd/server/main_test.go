package main

import (
	"reflect"
	"testing"
)

// TestWithIndexProxies covers the WS-base pinning of the homepage index proxies:
// the proxies are prepended (so capBase never trims them), order is preserved, and
// duplicates (a proxy that is also watchlisted, or repeats in the input) collapse.
func TestWithIndexProxies(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "empty base → just the proxies",
			in:   nil,
			want: []string{"SPY", "DIA", "QQQ"},
		},
		{
			name: "proxies prepended, base order preserved",
			in:   []string{"AAPL", "MSFT", "NVDA"},
			want: []string{"SPY", "DIA", "QQQ", "AAPL", "MSFT", "NVDA"},
		},
		{
			name: "a proxy already in the base is not duplicated (stays pinned at front)",
			in:   []string{"AAPL", "SPY", "MSFT"},
			want: []string{"SPY", "DIA", "QQQ", "AAPL", "MSFT"},
		},
		{
			name: "repeats in the base collapse",
			in:   []string{"AAPL", "AAPL", "MSFT"},
			want: []string{"SPY", "DIA", "QQQ", "AAPL", "MSFT"},
		},
		{
			name: "empty strings skipped",
			in:   []string{"", "AAPL", ""},
			want: []string{"SPY", "DIA", "QQQ", "AAPL"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := withIndexProxies(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("withIndexProxies(%v) = %v; want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestUsSymbols covers the US-only filter feeding the WS base: foreign suffixes are
// dropped, symbols are upper-cased, and blanks are skipped.
func TestUsSymbols(t *testing.T) {
	in := []string{"aapl", " msft ", "0700.HK", "2330.TW", "005930.KS", "", "BRK.B"}
	want := []string{"AAPL", "MSFT", "BRK.B"} // dotted US class share kept; foreign suffixes dropped
	if got := usSymbols(in); !reflect.DeepEqual(got, want) {
		t.Errorf("usSymbols(%v) = %v; want %v", in, got, want)
	}
}
