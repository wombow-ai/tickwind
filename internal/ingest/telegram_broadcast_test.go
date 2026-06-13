package ingest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/telegram"
)

// fakeBriefing is a static briefingReader: it returns one fixed (date, text).
type fakeBriefing struct {
	date, text string
	ok         bool
}

func (f fakeBriefing) Get(string) (string, string, time.Time, bool) {
	return f.date, f.text, time.Time{}, f.ok
}

// recordingTelegram is a briefingTelegram fake that counts photo/message sends
// and can be programmed to fail. It points at no network.
type recordingTelegram struct {
	mu          sync.Mutex
	enabled     bool
	photoErr    error // returned by SendPhoto until photos==0 budget runs out
	messageErr  error
	photoCalls  int
	msgCalls    int
	lastCaption string
	lastPhoto   string
}

func (t *recordingTelegram) Enabled() bool { return t.enabled }

func (t *recordingTelegram) SendPhoto(_ context.Context, photoURL, caption string, _ ...telegram.Option) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.photoCalls++
	t.lastPhoto = photoURL
	t.lastCaption = caption
	if t.photoErr != nil {
		return 0, t.photoErr
	}
	return 1, nil
}

func (t *recordingTelegram) SendMessage(_ context.Context, text string, _ ...telegram.Option) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.msgCalls++
	t.lastCaption = text
	if t.messageErr != nil {
		return 0, t.messageErr
	}
	return 2, nil
}

// newBroadcaster builds a broadcaster wired to fakes (no network).
func newBroadcaster(tg briefingTelegram, br briefingReader) *BriefingBroadcaster {
	return &BriefingBroadcaster{tg: tg, briefing: br, siteURL: "https://tickwind.com", log: slog.Default()}
}

func TestBroadcastPostsOncePerDay(t *testing.T) {
	tg := &recordingTelegram{enabled: true}
	br := fakeBriefing{date: "2026-06-13", text: "今日大盘高开。科技股领涨。", ok: true}
	b := newBroadcaster(tg, br)

	ctx := context.Background()
	b.maybeBroadcast(ctx) // first: should post
	b.maybeBroadcast(ctx) // same day: should NOT post again
	b.maybeBroadcast(ctx)

	if tg.photoCalls != 1 {
		t.Errorf("photoCalls = %d, want 1 (post once per ET day)", tg.photoCalls)
	}
	if tg.msgCalls != 0 {
		t.Errorf("msgCalls = %d, want 0 (photo succeeded, no fallback)", tg.msgCalls)
	}
	if b.lastPostedDate != "2026-06-13" {
		t.Errorf("lastPostedDate = %q, want 2026-06-13", b.lastPostedDate)
	}
	if !strings.Contains(tg.lastCaption, "每日美股晨报") {
		t.Errorf("caption %q missing title", tg.lastCaption)
	}
	if !strings.Contains(tg.lastPhoto, "/api/og/page?") {
		t.Errorf("card URL %q not the OG page route", tg.lastPhoto)
	}
	if !strings.Contains(tg.lastPhoto, "eyebrow=") || !strings.Contains(tg.lastPhoto, "title=") {
		t.Errorf("card URL %q missing encoded params", tg.lastPhoto)
	}
}

func TestBroadcastPostsAgainOnNewDay(t *testing.T) {
	tg := &recordingTelegram{enabled: true}
	b := newBroadcaster(tg, nil)

	b.briefing = fakeBriefing{date: "2026-06-13", text: "第一天。", ok: true}
	b.maybeBroadcast(context.Background())
	b.briefing = fakeBriefing{date: "2026-06-14", text: "第二天。", ok: true}
	b.maybeBroadcast(context.Background())

	if tg.photoCalls != 2 {
		t.Errorf("photoCalls = %d, want 2 (a new ET day re-posts)", tg.photoCalls)
	}
	if b.lastPostedDate != "2026-06-14" {
		t.Errorf("lastPostedDate = %q, want 2026-06-14", b.lastPostedDate)
	}
}

func TestBroadcastSkipsWhenNoBriefing(t *testing.T) {
	tg := &recordingTelegram{enabled: true}
	b := newBroadcaster(tg, fakeBriefing{ok: false})
	b.maybeBroadcast(context.Background())
	if tg.photoCalls != 0 || tg.msgCalls != 0 {
		t.Errorf("sent with no briefing: photo=%d msg=%d, want 0/0", tg.photoCalls, tg.msgCalls)
	}
}

func TestBroadcastFallsBackToMessageOnPhotoFailure(t *testing.T) {
	tg := &recordingTelegram{enabled: true, photoErr: errors.New("telegram: sendPhoto: failed to get HTTP URL content")}
	br := fakeBriefing{date: "2026-06-13", text: "图卡抓取失败也要播。", ok: true}
	b := newBroadcaster(tg, br)

	b.maybeBroadcast(context.Background())

	if tg.photoCalls != 1 {
		t.Errorf("photoCalls = %d, want 1", tg.photoCalls)
	}
	if tg.msgCalls != 1 {
		t.Errorf("msgCalls = %d, want 1 (fallback to text)", tg.msgCalls)
	}
	if b.lastPostedDate != "2026-06-13" {
		t.Errorf("lastPostedDate = %q, want 2026-06-13 (fallback succeeded)", b.lastPostedDate)
	}
}

func TestBroadcastRetriesOnceAfterRateLimit(t *testing.T) {
	// Photo: rate-limited then OK on retry → exactly one fallback-free success.
	tg := &flakyTelegram{recordingTelegram: recordingTelegram{enabled: true}, photoFailUntil: 1, photoErr: &telegram.RateLimitError{RetryAfter: 0}}
	br := fakeBriefing{date: "2026-06-13", text: "限流后重试一次。", ok: true}
	b := newBroadcaster(tg, br)

	b.maybeBroadcast(context.Background())

	if tg.photoCalls != 2 {
		t.Errorf("photoCalls = %d, want 2 (one 429 + one retry)", tg.photoCalls)
	}
	if tg.msgCalls != 0 {
		t.Errorf("msgCalls = %d, want 0 (retry succeeded, no text fallback)", tg.msgCalls)
	}
	if b.lastPostedDate != "2026-06-13" {
		t.Errorf("lastPostedDate = %q, want set after successful retry", b.lastPostedDate)
	}
}

func TestBroadcastDisabledIsNoOp(t *testing.T) {
	tg := &recordingTelegram{enabled: false} // disabled client
	b := newBroadcaster(tg, fakeBriefing{date: "2026-06-13", text: "x", ok: true})
	// Run must return immediately and never touch the client.
	b.Run(context.Background())
	if tg.photoCalls != 0 || tg.msgCalls != 0 {
		t.Errorf("disabled client sent: photo=%d msg=%d, want 0/0", tg.photoCalls, tg.msgCalls)
	}
}

// flakyTelegram fails SendPhoto for the first photoFailUntil calls (with
// photoErr), then succeeds — used to exercise the rate-limit retry path.
type flakyTelegram struct {
	recordingTelegram
	photoFailUntil int
	photoErr       error
}

func (t *flakyTelegram) SendPhoto(ctx context.Context, photoURL, caption string, opts ...telegram.Option) (int, error) {
	t.mu.Lock()
	t.photoCalls++
	n := t.photoCalls
	t.lastPhoto = photoURL
	t.lastCaption = caption
	t.mu.Unlock()
	if n <= t.photoFailUntil {
		return 0, t.photoErr
	}
	return 1, nil
}

// Ensure the real client also satisfies the briefingTelegram interface (so the
// constructor's *telegram.Client argument is interface-compatible) and that a
// disabled real client's Run is a no-op against an httptest server that would
// otherwise record hits.
func TestRealClientDisabledRunNoOp(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":1}}`)
	}))
	defer srv.Close()

	// Empty token → disabled real client; Run returns immediately.
	tg := telegram.New("", "@tickwind", srv.Client())
	var _ briefingTelegram = tg // compile-time interface check
	b := NewBriefingBroadcaster(tg, fakeBriefing{date: "2026-06-13", text: "x", ok: true}, "https://tickwind.com", nil)
	b.Run(context.Background())
	if hits != 0 {
		t.Errorf("disabled real client made %d HTTP calls, want 0", hits)
	}
}
