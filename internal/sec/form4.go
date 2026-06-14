// Package sec reads public-domain SEC EDGAR data for the Opportunity board:
// Form 4 insider transactions, the CIK↔ticker map, and XBRL shares-outstanding
// frames. All callers must send a descriptive User-Agent and stay under SEC's
// 10 req/s fair-access limit (see Client).
package sec

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"
)

// rule10b5One matches the plan reference "10b5-1" only when it is NOT immediately
// followed by another digit, so noise like "10b5-10" or "...110b5-1234" can't
// false-positive the anti-hallucination-governed planned-sale flag. Footnote text
// is lower-cased and space-stripped before matching, so only a trailing digit
// needs rejecting.
var rule10b5One = regexp.MustCompile(`10b5-1([^0-9]|$)`)

// Form4 is the distilled content of one Form 4 ownership filing: the issuer,
// the reporting insider, any OPEN-MARKET PURCHASES (transaction code "P"), and
// any OPEN-MARKET SALES (transaction code "S"). Awards, option exercises, and
// gifts are dropped — only discretionary open-market buys/sells carry a signal.
// The Opportunity board consumes only Buys/BuyValue()/HasBuys(); the per-ticker
// insider-activity timeline additionally consumes Sells.
type Form4 struct {
	Ticker       string
	IssuerName   string
	OwnerName    string
	IsDirector   bool
	IsOfficer    bool
	OfficerTitle string
	Buys         []Buy
	Sells        []Sale
}

// Buy is one open-market purchase line (code P, acquired, priced). Date is the
// transaction date (YYYY-MM-DD) from the filing, "" when absent.
type Buy struct {
	Shares float64
	Price  float64
	Value  float64 // Shares × Price
	Date   string  // transaction date (YYYY-MM-DD), "" when absent
}

// Sale is one open-market sale line (code S, disposed, priced). Date is the
// transaction date (YYYY-MM-DD) from the filing, "" when absent. Planned10b5_1
// reports whether the filing affirms the trade was made under a Rule 10b5-1
// trading plan (from the document-level <aff10b5One> checkbox, with a
// conservative footnote backstop for pre-2023 filings) — such sales carry far
// less signal than discretionary sells.
type Sale struct {
	Shares        float64
	Price         float64
	Value         float64 // Shares × Price
	Date          string  // transaction date (YYYY-MM-DD), "" when absent
	Planned10b5_1 bool    // affirmed Rule 10b5-1 planned sale (document-level)
}

// BuyValue is the total dollar value of the open-market buys in the filing.
func (f Form4) BuyValue() float64 {
	var sum float64
	for _, b := range f.Buys {
		sum += b.Value
	}
	return sum
}

// HasBuys reports whether the filing contains any open-market purchase.
func (f Form4) HasBuys() bool { return len(f.Buys) > 0 }

// SellValue is the total dollar value of the open-market sales in the filing.
func (f Form4) SellValue() float64 {
	var sum float64
	for _, s := range f.Sells {
		sum += s.Value
	}
	return sum
}

// HasSells reports whether the filing contains any open-market sale.
func (f Form4) HasSells() bool { return len(f.Sells) > 0 }

// ownershipDoc mirrors the Form 4 primary XML. Leaf amounts are <value>-wrapped;
// the transaction code is not. Only the fields we map are declared.
type ownershipDoc struct {
	XMLName xml.Name `xml:"ownershipDocument"`
	Issuer  struct {
		TradingSymbol string `xml:"issuerTradingSymbol"`
		Name          string `xml:"issuerName"`
	} `xml:"issuer"`
	ReportingOwner struct {
		ID struct {
			Name string `xml:"rptOwnerName"`
		} `xml:"reportingOwnerId"`
		Rel struct {
			IsDirector   string `xml:"isDirector"`
			IsOfficer    string `xml:"isOfficer"`
			OfficerTitle string `xml:"officerTitle"`
		} `xml:"reportingOwnerRelationship"`
	} `xml:"reportingOwner"`
	// Aff10b5One is the document-level affirmation (the post-2023 Form 4 checkbox)
	// that the reported transaction(s) were made pursuant to a Rule 10b5-1(c)
	// trading plan. "true"/"1" → planned. Absent (pre-2023 filings) → "".
	Aff10b5One    string `xml:"aff10b5One"`
	NonDerivative struct {
		Txns []struct {
			Coding struct {
				Code string `xml:"transactionCode"`
			} `xml:"transactionCoding"`
			Date    valueStr `xml:"transactionDate"`
			Amounts struct {
				Shares  valueFloat `xml:"transactionShares"`
				Price   valueFloat `xml:"transactionPricePerShare"`
				AcqDisp valueStr   `xml:"transactionAcquiredDisposedCode"`
			} `xml:"transactionAmounts"`
		} `xml:"nonDerivativeTransaction"`
	} `xml:"nonDerivativeTable"`
	Footnotes struct {
		Items []struct {
			Text string `xml:",chardata"`
		} `xml:"footnote"`
	} `xml:"footnotes"`
}

type valueFloat struct {
	Value float64 `xml:"value"`
}
type valueStr struct {
	Value string `xml:"value"`
}

// ParseForm4 parses a Form 4 primary XML document into a Form4, keeping
// open-market purchases (code "P", acquired, positive price) in Buys and
// open-market sales (code "S", disposed, positive price) in Sells. Each line
// carries its transaction date; sales additionally carry the document-level
// Rule 10b5-1 planned-sale flag. Awards, option exercises, and gifts are dropped.
func ParseForm4(data []byte) (Form4, error) {
	var doc ownershipDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		return Form4{}, fmt.Errorf("sec: parse form4: %w", err)
	}
	f := Form4{
		Ticker:       strings.ToUpper(strings.TrimSpace(doc.Issuer.TradingSymbol)),
		IssuerName:   strings.TrimSpace(doc.Issuer.Name),
		OwnerName:    strings.TrimSpace(doc.ReportingOwner.ID.Name),
		IsDirector:   isTrue(doc.ReportingOwner.Rel.IsDirector),
		IsOfficer:    isTrue(doc.ReportingOwner.Rel.IsOfficer),
		OfficerTitle: strings.TrimSpace(doc.ReportingOwner.Rel.OfficerTitle),
	}
	// 10b5-1 planned-sale detection is document-level: the structured post-2023
	// <aff10b5One> affirmation is primary; a conservative footnote scan is a
	// best-effort backstop for older filings that predate the checkbox. Never
	// guessed — false unless an affirmation or an explicit "10b5-1" footnote.
	planned := planned10b5One(doc)
	for _, tx := range doc.NonDerivative.Txns {
		code := strings.TrimSpace(tx.Coding.Code)
		acqDisp := strings.TrimSpace(tx.Amounts.AcqDisp.Value)
		date := strings.TrimSpace(tx.Date.Value)
		switch {
		case code == "P" && strings.EqualFold(acqDisp, "A"): // open-market purchase
			if tx.Amounts.Price.Value <= 0 || tx.Amounts.Shares.Value <= 0 {
				continue // drop $0 awards/gifts that slipped through as P
			}
			f.Buys = append(f.Buys, Buy{
				Shares: tx.Amounts.Shares.Value,
				Price:  tx.Amounts.Price.Value,
				Value:  tx.Amounts.Shares.Value * tx.Amounts.Price.Value,
				Date:   date,
			})
		case code == "S" && strings.EqualFold(acqDisp, "D"): // open-market sale
			if tx.Amounts.Price.Value <= 0 || tx.Amounts.Shares.Value <= 0 {
				continue // drop $0 dispositions (gifts/transfers mis-coded as S)
			}
			f.Sells = append(f.Sells, Sale{
				Shares:        tx.Amounts.Shares.Value,
				Price:         tx.Amounts.Price.Value,
				Value:         tx.Amounts.Shares.Value * tx.Amounts.Price.Value,
				Date:          date,
				Planned10b5_1: planned,
			})
		}
	}
	return f, nil
}

// planned10b5One reports whether the filing affirms its transactions were made
// under a Rule 10b5-1 trading plan. The structured document-level <aff10b5One>
// checkbox (added to Form 4 in 2023) is the reliable primary signal; for older
// filings that predate the checkbox, a conservative scan of the footnote text
// for "10b5-1" is the best-effort backstop. Returns false when neither is
// present — the flag is NEVER guessed.
func planned10b5One(doc ownershipDoc) bool {
	if isTrue(doc.Aff10b5One) {
		return true
	}
	for _, fn := range doc.Footnotes.Items {
		// Normalize common dash/spacing variants ("10b5-1", "10b5–1", "10b 5-1"),
		// then require a non-digit boundary so "10b5-10" can't false-positive.
		t := strings.ToLower(fn.Text)
		t = strings.NewReplacer("–", "-", "—", "-", " ", "").Replace(t)
		if rule10b5One.MatchString(t) {
			return true
		}
	}
	return false
}

// isTrue accepts SEC's boolean conventions ("1" or "true").
func isTrue(s string) bool {
	s = strings.TrimSpace(s)
	return s == "1" || strings.EqualFold(s, "true")
}
