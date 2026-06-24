package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/wombow-ai/tickwind/internal/chat"
)

// chatETFHoldings implements chat.ETFHoldingsLister over an ETFHoldingsSource: it fetches a fund/ETF's
// top positions from its latest SEC Form N-PORT-P and formats them as an attributed, Go-OWNED summary
// the model may quote (every percent is parsed verbatim from the filing — the model never derives a
// number). It closes the ETF "dead-zone": the chat can now state what an ETF HOLDS, not merely that
// it is an ETF (the DRAM incident).
type chatETFHoldings struct{ src ETFHoldingsSource }

// NewChatETFHoldings returns the chat.ETFHoldingsLister, or nil when no source is wired (the
// get_etf_holdings tool is then never offered — safe to deploy before wiring).
func NewChatETFHoldings(src ETFHoldingsSource) chat.ETFHoldingsLister {
	if src == nil {
		return nil
	}
	return chatETFHoldings{src: src}
}

// etfHoldingsChatTopN bounds how many holdings the chat summary lists — enough to characterize the
// fund without flooding the model context.
const etfHoldingsChatTopN = 15

// ETFHoldingsText returns a formatted top-holdings summary, or ok=false when the ticker has no
// N-PORT holdings filing (an ordinary stock) so the model answers honestly instead of improvising.
func (c chatETFHoldings) ETFHoldingsText(ctx context.Context, ticker, lang string) (string, bool) {
	holdings, asOf, err := c.src.ETFHoldings(ctx, ticker, etfHoldingsChatTopN)
	if err != nil || len(holdings) == 0 {
		return "", false
	}
	tk := strings.ToUpper(strings.TrimSpace(ticker))
	asOfStr := asOf.Format("2006-01-02")
	var b strings.Builder
	if lang == "en" {
		fmt.Fprintf(&b, "%s — top %d holdings, %% of net assets (SEC Form N-PORT, filed %s):\n", tk, len(holdings), asOfStr)
	} else {
		fmt.Fprintf(&b, "%s — 前 %d 大持仓,占净值百分比(SEC N-PORT 申报,申报日 %s):\n", tk, len(holdings), asOfStr)
	}
	for i, h := range holdings {
		name := h.Name
		if h.Ticker != "" {
			name = h.Name + " (" + h.Ticker + ")"
		}
		fmt.Fprintf(&b, "%d. %s — %.2f%%\n", i+1, name, h.PctVal)
	}
	return strings.TrimRight(b.String(), "\n"), true
}
