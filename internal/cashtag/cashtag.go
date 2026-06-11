// Package cashtag extracts $TICKER stock mentions ("cashtags") from
// user-authored text, powering comment fan-out: a comment that mentions
// $RKLB also appears in RKLB's comment section. The frontend mirrors the
// same rules to linkify tags in rendered Markdown — keep them in sync.
package cashtag

import (
	"regexp"
	"strings"
)

// MaxTags caps mentions per comment so a tag-spam post can't fan out everywhere.
const MaxTags = 8

// re matches "$" + 1-6 alphanumerics + an optional venue suffix (".SA", ".HK",
// ".TW", ".KS"…). The leading class keeps "$$AAPL" and "x$AAPL" from matching;
// the trailing \b drops 7+ character runs ("$AAPLextra").
var re = regexp.MustCompile(`(^|[^A-Za-z0-9$])\$([A-Za-z0-9]{1,6}(?:\.[A-Za-z]{1,3})?)\b`)

// Extract returns the unique cashtags in body — uppercased, in first-mention
// order, capped at MaxTags. Pure-digit tags without a venue suffix ("$100",
// "$420") are dollar amounts, not tickers, and are skipped; digit codes WITH a
// suffix ("$0700.HK") are kept.
func Extract(body string) []string {
	matches := re.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	var out []string
	for _, m := range matches {
		tag := strings.ToUpper(m[2])
		base, _, hasSuffix := strings.Cut(tag, ".")
		if !hasSuffix && isAllDigits(base) {
			continue // a price like "$100", not a ticker
		}
		if seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
		if len(out) == MaxTags {
			break
		}
	}
	return out
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}
