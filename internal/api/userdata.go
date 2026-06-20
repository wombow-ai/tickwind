package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/wombow-ai/tickwind/internal/chat"
	"github.com/wombow-ai/tickwind/internal/store"
)

// chatUserData implements chat.UserData over the store: it reads the AUTHENTICATED user's
// OWN watchlist / holdings / notes, pre-formatted by Go (every number is Go-computed —
// the anti-hallucination contract holds). PRIVACY: every store call is scoped to userID
// (the store queries WHERE user_id = $1), so it can never return another user's data.
type chatUserData struct {
	store store.Store
}

// NewChatUserData builds the chat.UserData source for the personalized chat.
func NewChatUserData(st store.Store) chat.UserData { return chatUserData{store: st} }

func pick(lang, zh, en string) string {
	if lang == "en" {
		return en
	}
	return zh
}

// Watchlist returns the user's tracked tickers with their live prices + day change.
func (d chatUserData) Watchlist(ctx context.Context, userID, lang string) string {
	tickers, err := d.store.Watchlist(ctx, userID)
	if err != nil || len(tickers) == 0 {
		return pick(lang, "用户的自选股为空。", "The user's watchlist is empty.")
	}
	var b strings.Builder
	b.WriteString(pick(lang, fmt.Sprintf("用户自选股(%d 只):\n", len(tickers)), fmt.Sprintf("The user's watchlist (%d):\n", len(tickers))))
	for _, t := range tickers {
		q, ok, _ := d.store.GetQuote(ctx, t)
		if !ok || q.Price <= 0 {
			b.WriteString("- " + t + pick(lang, "(暂无报价)\n", " (no quote)\n"))
			continue
		}
		chg := ""
		if q.PrevClose > 0 {
			chg = fmt.Sprintf(" (%+.1f%%)", (q.Price/q.PrevClose-1)*100)
		}
		b.WriteString(fmt.Sprintf("- %s: $%.2f%s\n", t, q.Price, chg))
	}
	return strings.TrimRight(b.String(), "\n")
}

// Holdings returns the user's positions with Go-computed gain/loss + portfolio weight.
func (d chatUserData) Holdings(ctx context.Context, userID, lang string) string {
	holds, err := d.store.ListHoldings(ctx, userID)
	if err != nil || len(holds) == 0 {
		return pick(lang, "用户没有记录持仓。", "The user has no recorded holdings.")
	}
	type row struct {
		h        store.Holding
		price    float64
		value    float64
		gain     float64
		gainPct  float64
		hasPrice bool
	}
	rows := make([]row, 0, len(holds))
	total := 0.0
	for _, h := range holds {
		r := row{h: h}
		if q, ok, _ := d.store.GetQuote(ctx, h.Ticker); ok && q.Price > 0 {
			r.hasPrice = true
			r.price = q.Price
			r.value = q.Price * h.Shares
			r.gain = (q.Price - h.AvgCost) * h.Shares
			if h.AvgCost > 0 {
				r.gainPct = (q.Price/h.AvgCost - 1) * 100
			}
			total += r.value
		}
		rows = append(rows, r)
	}
	var b strings.Builder
	b.WriteString(pick(lang, fmt.Sprintf("用户持仓(%d 只", len(rows)), fmt.Sprintf("The user's holdings (%d position(s)", len(rows))))
	if total > 0 {
		b.WriteString(fmt.Sprintf(pick(lang, ",组合市值 $%.0f):\n", ", portfolio value $%.0f):\n"), total))
	} else {
		b.WriteString("):\n")
	}
	for _, r := range rows {
		if !r.hasPrice {
			b.WriteString(fmt.Sprintf("- %s: %g %s @ $%.2f "+pick(lang, "(暂无报价)\n", "(no quote)\n"), r.h.Ticker, r.h.Shares, pick(lang, "股,成本", "sh, cost"), r.h.AvgCost))
			continue
		}
		weight := ""
		if total > 0 {
			weight = fmt.Sprintf(pick(lang, ",占组合 %.0f%%", ", %.0f%% of portfolio"), r.value/total*100)
		}
		b.WriteString(fmt.Sprintf("- %s: %g %s @ $%.2f → $%.2f, %s $%.0f (%+.1f%%)%s\n",
			r.h.Ticker, r.h.Shares, pick(lang, "股,成本", "sh @ cost"), r.h.AvgCost, r.price,
			pick(lang, "市值", "value"), r.value, r.gainPct, weight))
	}
	return strings.TrimRight(b.String(), "\n")
}

// Notes returns the user's private notes (optionally scoped to a ticker), truncated.
func (d chatUserData) Notes(ctx context.Context, userID, ticker, lang string) string {
	notes, err := d.store.ListNotes(ctx, store.NoteFilter{UserID: userID, Ticker: ticker, Limit: 20})
	if err != nil || len(notes) == 0 {
		return pick(lang, "用户没有相关私人笔记。", "The user has no matching private notes.")
	}
	var b strings.Builder
	b.WriteString(pick(lang, "用户的私人笔记:\n", "The user's private notes:\n"))
	for _, n := range notes {
		tag := ""
		if n.Ticker != "" {
			tag = "[" + n.Ticker + "] "
		}
		body := strings.TrimSpace(n.Body)
		if len(body) > 400 {
			body = body[:400] + "…"
		}
		b.WriteString("- " + tag + body + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
