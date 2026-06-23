package materialevents

import "github.com/wombow-ai/tickwind/internal/edgar"

// FeedEvent is one NOTABLE recent 8-K on the market-wide material-events feed: the ticker it was
// filed under (known by the aggregating scan) plus the Go-owned event facts — form, dates, the SEC
// filing link, and the filtered NOTABLE item codes with their canonical labels. There is deliberately
// NO LLM summary on the feed (it is a facts-only roll-up; the per-stock material-events view carries
// the optional hedged AI summary). A DISCLOSED corporate-filing fact, never advice. Every field is
// Go-parsed from SEC EDGAR.
type FeedEvent struct {
	Ticker       string            `json:"ticker"`
	Form         string            `json:"form"` // "8-K" / "8-K/A"
	FiledDate    string            `json:"filed_date"`
	ReportDate   string            `json:"report_date,omitempty"`
	AccessionURL string            `json:"accession_url"`
	Items        []edgar.EventItem `json:"items"` // only the NOTABLE items (see NotableItems)
}

// notableItemCodes is the curated set of HIGH-SIGNAL 8-K item codes the market-wide feed surfaces —
// the corporate events a reader actually searches for: material agreements, M&A, bankruptcy, debt
// distress, restructuring/impairment, delisting, auditor change / financial restatement, change of
// control, and officer/director departures & appointments. The routine / administrative codes are
// deliberately EXCLUDED so the feed is SIGNAL, not 8-K noise: 2.02 (earnings — already covered by the
// earnings calendar + reaction stats), 7.01 (Reg FD), 8.01 (Other Events — a catch-all), 9.01
// (Exhibits — administrative), 5.07 (vote results — routine). This is editorial CURATION over the
// Go-owned item-label universe (edgar.itemLabels); every label itself stays Go-owned.
var notableItemCodes = map[string]bool{
	"1.01": true, // Entry into a Material Definitive Agreement
	"1.02": true, // Termination of a Material Definitive Agreement
	"1.03": true, // Bankruptcy or Receivership
	"2.01": true, // Completion of Acquisition or Disposition of Assets
	"2.03": true, // Creation of a Direct Financial Obligation (new material debt)
	"2.04": true, // Triggering Events That Accelerate a Financial Obligation
	"2.05": true, // Costs Associated with Exit or Disposal Activities (restructuring)
	"2.06": true, // Material Impairments
	"3.01": true, // Notice of Delisting / Failure to Satisfy a Continued Listing Rule
	"4.01": true, // Changes in Registrant's Certifying Accountant
	"4.02": true, // Non-Reliance on Previously Issued Financial Statements (restatement)
	"5.01": true, // Changes in Control of Registrant
	"5.02": true, // Departure / Election / Appointment of Directors or Certain Officers
	"5.06": true, // Change in Shell Company Status
}

// NotableItems returns the subset of an 8-K's items that are high-signal (in notableItemCodes), or
// nil when none are — so an event with only routine items (earnings, exhibits, Reg FD) is dropped
// from the feed. The returned items keep their Go-owned canonical labels.
func NotableItems(items []edgar.EventItem) []edgar.EventItem {
	var out []edgar.EventItem
	for _, it := range items {
		if notableItemCodes[it.Code] {
			out = append(out, it)
		}
	}
	return out
}
