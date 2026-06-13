package ingest

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/wombow-ai/tickwind/internal/telegram"
)

// briefingTelegram is the slice of *telegram.Client the broadcaster needs: a
// send-only surface plus the Enabled gate. Narrowing it keeps the broadcaster
// unit-testable with a fake that points at no real network.
type briefingTelegram interface {
	Enabled() bool
	SendPhoto(ctx context.Context, photoURL, caption string, opts ...telegram.Option) (int, error)
	SendMessage(ctx context.Context, text string, opts ...telegram.Option) (int, error)
}

// briefingReader is the slice of *BriefingCache the broadcaster reads: the
// latest Chinese briefing's ET date and text. ok=false before the first
// generation. Satisfied by *BriefingCache.Get.
type briefingReader interface {
	Get(lang string) (date, text string, at time.Time, ok bool)
}

// broadcastCheckEvery is how often the broadcaster polls for a fresh briefing
// to post. The briefing is generated at most once per ET day, so a ~30-minute
// cadence posts it promptly after generation without busy-waiting.
const broadcastCheckEvery = 30 * time.Minute

// broadcastCaptionLimit bounds how many runes of the briefing body go into the
// Telegram caption/message, leaving headroom under Telegram's 1024-rune photo
// caption cap once the HTML header/footer are added.
const broadcastCaptionLimit = 600

// BriefingBroadcaster pushes the day's Chinese pre-market briefing to a Telegram
// channel once per ET day. It is send-only: it never reads or processes incoming
// Telegram messages. Delivery is best-effort — any failure is logged and retried
// on the next tick (a new ET day's briefing always supersedes an undelivered one),
// and a disabled Telegram client makes Run a graceful no-op.
type BriefingBroadcaster struct {
	tg       briefingTelegram
	briefing briefingReader
	siteURL  string
	log      *slog.Logger

	// lastPostedDate is the ET date ("2006-01-02") of the most recently posted
	// briefing, used to post each day's briefing at most once. Touched only from
	// Run's single goroutine, so it needs no lock.
	lastPostedDate string
}

// NewBriefingBroadcaster builds the broadcaster. tg is the Telegram client
// (disabled clients make Run a no-op); briefing is the source of the latest
// Chinese briefing (e.g. *BriefingCache); siteURL is the public origin used to
// build the OG share-card image URL (must be publicly reachable, as Telegram
// fetches the photo server-side). A nil log defaults to slog.Default.
func NewBriefingBroadcaster(tg *telegram.Client, briefing briefingReader, siteURL string, log *slog.Logger) *BriefingBroadcaster {
	if log == nil {
		log = slog.Default()
	}
	return &BriefingBroadcaster{
		tg:       tg,
		briefing: briefing,
		siteURL:  strings.TrimRight(strings.TrimSpace(siteURL), "/"),
		log:      log,
	}
}

// Run posts the day's briefing once at startup and then on every tick until ctx
// is cancelled. When the Telegram client is disabled it returns immediately, so
// callers can wire it unconditionally and gate real delivery on configuration.
func (b *BriefingBroadcaster) Run(ctx context.Context) {
	if b.tg == nil || !b.tg.Enabled() {
		return // no token configured — graceful skip
	}
	b.maybeBroadcast(ctx)
	t := time.NewTicker(broadcastCheckEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			b.maybeBroadcast(ctx)
		}
	}
}

// maybeBroadcast posts the latest Chinese briefing if it exists and its ET date
// has not been posted yet. A failed send leaves lastPostedDate unchanged so the
// next tick retries the same day.
func (b *BriefingBroadcaster) maybeBroadcast(ctx context.Context) {
	date, text, _, ok := b.briefing.Get("zh")
	if !ok || strings.TrimSpace(text) == "" {
		return // no briefing generated yet — retry next tick
	}
	if date == b.lastPostedDate {
		return // already posted today's briefing
	}

	cardURL := b.cardURL(date, text)
	caption := b.caption(date, text)

	if err := b.send(ctx, cardURL, caption); err != nil {
		b.log.Warn("briefing broadcast failed", "date", date, "err", err)
		return // keep lastPostedDate so the next tick retries this day
	}
	b.lastPostedDate = date
	b.log.Info("morning briefing broadcast to telegram", "date", date)
}

// send delivers the briefing as an OG-card photo with an HTML caption, falling
// back to a plain HTML message (no link preview) if the photo send fails — most
// often because Telegram could not fetch the card image. It retries once after a
// rate-limit, honoring Telegram's requested back-off.
func (b *BriefingBroadcaster) send(ctx context.Context, cardURL, caption string) error {
	_, err := b.tg.SendPhoto(ctx, cardURL, caption, telegram.WithHTML())
	if err == nil {
		return nil
	}
	if rl := backoff(ctx, err); rl {
		if _, err = b.tg.SendPhoto(ctx, cardURL, caption, telegram.WithHTML()); err == nil {
			return nil
		}
	}
	// Photo failed (likely the card image was unreachable) — fall back to a
	// plain text message so the briefing still goes out.
	b.log.Warn("briefing photo send failed; falling back to text", "err", err)
	_, err = b.tg.SendMessage(ctx, caption, telegram.WithHTML(), telegram.WithoutPreview())
	if err == nil {
		return nil
	}
	if rl := backoff(ctx, err); rl {
		_, err = b.tg.SendMessage(ctx, caption, telegram.WithHTML(), telegram.WithoutPreview())
	}
	return err
}

// backoff waits out a Telegram rate-limit (honoring RetryAfter, default 1s) and
// reports whether the caller should retry. It returns false for non-rate-limit
// errors and when ctx is cancelled during the wait.
func backoff(ctx context.Context, err error) bool {
	var rl *telegram.RateLimitError
	if !errors.As(err, &rl) {
		return false
	}
	wait := time.Duration(rl.RetryAfter) * time.Second
	if wait <= 0 {
		wait = time.Second
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// cardURL builds the public OG share-card image URL for the briefing. The card
// is rendered by the frontend's /api/og/page route from URL-encoded params, and
// must be reachable from the public internet because Telegram fetches it
// server-side.
func (b *BriefingBroadcaster) cardURL(date, text string) string {
	q := url.Values{}
	q.Set("eyebrow", "每日美股晨报")
	q.Set("title", date)
	q.Set("subtitle", teaser(text))
	return b.siteURL + "/api/og/page?" + q.Encode()
}

// caption builds the HTML message body: a bold title line, then the escaped
// briefing body (truncated for length), then a link back to the site. The body
// is HTML-escaped so stray < > & in the briefing can't break Telegram's HTML
// parse_mode.
func (b *BriefingBroadcaster) caption(date, text string) string {
	body := truncateRunes(strings.TrimSpace(text), broadcastCaptionLimit)
	var sb strings.Builder
	sb.WriteString("<b>🌊 Tickwind 每日美股晨报 · ")
	sb.WriteString(telegram.EscapeHTML(date))
	sb.WriteString("</b>\n\n")
	sb.WriteString(telegram.EscapeHTML(body))
	sb.WriteString("\n\n全文 → tickwind.com")
	return sb.String()
}

// teaser returns the briefing's first sentence/line for the card subtitle,
// bounded to a card-friendly length. It stops at the first newline or Chinese/
// ASCII full stop so the card shows a clean opening line rather than a hard cut.
func teaser(text string) string {
	s := strings.TrimSpace(text)
	if i := strings.IndexAny(s, "\n。\r"); i > 0 {
		s = s[:i]
	}
	return truncateRunes(strings.TrimSpace(s), 90)
}

// truncateRunes shortens s to at most n runes (not bytes, so it never splits a
// multi-byte CJK character), appending an ellipsis when it cuts.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
