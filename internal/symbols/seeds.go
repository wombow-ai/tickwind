package symbols

// ForeignSeeds returns the hand-curated non-US symbols Tickwind actively tracks
// (the Taiwan + Hong Kong seed names). The SEC directory is US-only, so these are
// merged into the search index by the ingestor — otherwise the foreign names we
// price wouldn't be findable in autocomplete. Country carries the market; Korea
// is intentionally omitted until KR data is live (no point surfacing a name with
// no page data).
func ForeignSeeds() []Symbol {
	return []Symbol{
		// Taiwan (TWSE) — live via the TW adapter.
		{Ticker: "2330.TW", Name: "Taiwan Semiconductor (TSMC)", Exchange: "TWSE", Country: "TW"},
		{Ticker: "2317.TW", Name: "Hon Hai Precision (Foxconn)", Exchange: "TWSE", Country: "TW"},
		{Ticker: "2454.TW", Name: "MediaTek", Exchange: "TWSE", Country: "TW"},
		{Ticker: "2308.TW", Name: "Delta Electronics", Exchange: "TWSE", Country: "TW"},
		{Ticker: "2412.TW", Name: "Chunghwa Telecom", Exchange: "TWSE", Country: "TW"},
		{Ticker: "2303.TW", Name: "United Microelectronics (UMC)", Exchange: "TWSE", Country: "TW"},
		// Hong Kong (HKEX) — live via the Yahoo delayed-quote adapter.
		{Ticker: "0700.HK", Name: "Tencent Holdings", Exchange: "HKEX", Country: "HK"},
		{Ticker: "2513.HK", Name: "Zhipu AI (Knowledge Atlas / Z.ai)", Exchange: "HKEX", Country: "HK"},
		{Ticker: "0100.HK", Name: "MiniMax", Exchange: "HKEX", Country: "HK"},
	}
}
