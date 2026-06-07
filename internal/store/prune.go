package store

import (
	"context"
	"time"
)

// Pruner is an optional capability a Store may implement to bound the growth of
// the durable, append-mostly market tables (news, social, filings, insider_buys,
// seen_form4). It is type-asserted at runtime: a Store that does not implement it
// is simply never pruned. Every method returns the number of rows removed.
//
// Two protect-rules thread through the windowed deletes, per the owner's intent
// ("淘汰老的非重点数据 … 但是大V、reddit高热的部分信息可以长久存储"):
//   - hot-list tickers keep a LONGER window (hotBefore is an earlier instant than
//     before), so a name the market currently cares about isn't trimmed early;
//   - protected social sources (e.g. "substack" — the curated 大V / Serenity rail)
//     are NEVER pruned, regardless of age.
type Pruner interface {
	// PruneNews removes news published before `before`, except hot-list tickers,
	// which are retained until `hotBefore`.
	PruneNews(ctx context.Context, before, hotBefore time.Time) (int64, error)
	// PruneSocial removes posts created before `before`, except (a) posts whose
	// source is in protectSources (never pruned) and (b) hot-list tickers, which
	// are retained until `hotBefore`.
	PruneSocial(ctx context.Context, before, hotBefore time.Time, protectSources []string) (int64, error)
	// PruneFilings removes filings filed before `before`.
	PruneFilings(ctx context.Context, before time.Time) (int64, error)
	// PruneInsiderBuys removes insider buys filed before `before`.
	PruneInsiderBuys(ctx context.Context, before time.Time) (int64, error)
	// PruneSeenForm4 removes seen-Form-4 markers filed before `before`.
	PruneSeenForm4(ctx context.Context, before time.Time) (int64, error)
	// CapPerTicker keeps only the newest n rows per ticker in the given table — a
	// backstop so one perpetually-hot ticker can't accumulate without bound. Rows
	// whose source is in protectSources (e.g. the 大V rail) are never counted
	// toward the cap nor evicted by it, so guru posts survive even on a 500+-post
	// ticker. table must be "news" or "social"; anything else returns an error.
	CapPerTicker(ctx context.Context, table string, n int, protectSources []string) (int64, error)
}
