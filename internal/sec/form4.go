// Package sec reads public-domain SEC EDGAR data for the Opportunity board:
// Form 4 insider transactions, the CIK↔ticker map, and XBRL shares-outstanding
// frames. All callers must send a descriptive User-Agent and stay under SEC's
// 10 req/s fair-access limit (see Client).
package sec

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// Form4 is the distilled content of one Form 4 ownership filing: the issuer,
// the reporting insider, and any OPEN-MARKET PURCHASES (transaction code "P").
// Non-purchase transactions (awards, option exercises, sales, gifts) are
// dropped — only discretionary open-market buys carry a conviction signal.
type Form4 struct {
	Ticker       string
	IssuerName   string
	OwnerName    string
	IsDirector   bool
	IsOfficer    bool
	OfficerTitle string
	Buys         []Buy
}

// Buy is one open-market purchase line (code P, acquired, priced).
type Buy struct {
	Shares float64
	Price  float64
	Value  float64 // Shares × Price
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
	NonDerivative struct {
		Txns []struct {
			Coding struct {
				Code string `xml:"transactionCode"`
			} `xml:"transactionCoding"`
			Amounts struct {
				Shares  valueFloat `xml:"transactionShares"`
				Price   valueFloat `xml:"transactionPricePerShare"`
				AcqDisp valueStr   `xml:"transactionAcquiredDisposedCode"`
			} `xml:"transactionAmounts"`
		} `xml:"nonDerivativeTransaction"`
	} `xml:"nonDerivativeTable"`
}

type valueFloat struct {
	Value float64 `xml:"value"`
}
type valueStr struct {
	Value string `xml:"value"`
}

// ParseForm4 parses a Form 4 primary XML document into a Form4, keeping only
// open-market purchases (code "P", acquired, with a positive price).
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
	for _, tx := range doc.NonDerivative.Txns {
		if tx.Coding.Code != "P" { // open-market purchase only
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(tx.Amounts.AcqDisp.Value), "A") {
			continue // must be an acquisition
		}
		if tx.Amounts.Price.Value <= 0 || tx.Amounts.Shares.Value <= 0 {
			continue // drop $0 awards/gifts that slipped through as P
		}
		f.Buys = append(f.Buys, Buy{
			Shares: tx.Amounts.Shares.Value,
			Price:  tx.Amounts.Price.Value,
			Value:  tx.Amounts.Shares.Value * tx.Amounts.Price.Value,
		})
	}
	return f, nil
}

// isTrue accepts SEC's boolean conventions ("1" or "true").
func isTrue(s string) bool {
	s = strings.TrimSpace(s)
	return s == "1" || strings.EqualFold(s, "true")
}
