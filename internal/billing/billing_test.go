package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestSubscriptionPeriodEnd checks the item-level current_period_end fallback (newer
// Stripe API versions carry it on the line item, not the subscription).
func TestSubscriptionPeriodEnd(t *testing.T) {
	var top Subscription
	top.CurrentPeriodEnd = 100
	if top.PeriodEnd() != 100 {
		t.Errorf("top-level period end = %d, want 100", top.PeriodEnd())
	}
	var item Subscription
	item.Items.Data = append(item.Items.Data, struct {
		CurrentPeriodEnd int64 `json:"current_period_end"`
		Price            struct {
			ID        string `json:"id"`
			Recurring struct {
				Interval string `json:"interval"`
			} `json:"recurring"`
		} `json:"price"`
	}{CurrentPeriodEnd: 200})
	if item.PeriodEnd() != 200 {
		t.Errorf("item-level period end = %d, want 200", item.PeriodEnd())
	}
	var none Subscription
	if none.PeriodEnd() != 0 {
		t.Errorf("no period end = %d, want 0", none.PeriodEnd())
	}
}

// TestListSubscriptionsRefusesTruncation ensures a never-ending has_more (more than
// maxPages) returns an ERROR rather than a silently-partial slice — the reconciler must
// never reverse-revoke users off a truncated list.
func TestListSubscriptionsRefusesTruncation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Always claim more pages with a stable cursor id → forces the page cap.
		io.WriteString(w, `{"object":"list","has_more":true,"data":[{"id":"sub_x","customer":"cus_x","status":"active"}]}`)
	}))
	defer srv.Close()
	s := New(Config{SecretKey: "sk_test_x", APIBaseURL: srv.URL})
	if _, err := s.ListSubscriptions(context.Background()); err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("want a truncation error, got %v", err)
	}
}

// sign produces a valid Stripe-Signature header for a payload at time t.
func sign(secret string, payload []byte, t time.Time) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.", t.Unix())
	mac.Write(payload)
	return fmt.Sprintf("t=%d,v1=%s", t.Unix(), hex.EncodeToString(mac.Sum(nil)))
}

func TestVerifyWebhook(t *testing.T) {
	const secret = "whsec_test_abc123"
	svc := New(Config{SecretKey: "sk_test_x", WebhookSecret: secret})
	payload := []byte(`{"id":"evt_1","type":"checkout.session.completed"}`)
	now := time.Unix(1_700_000_000, 0)

	// Valid signature within tolerance → nil.
	if err := svc.VerifyWebhook(payload, sign(secret, payload, now), now); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}

	// Tampered payload → ErrBadSignature (signature no longer matches).
	if err := svc.VerifyWebhook([]byte(`{"id":"evt_evil"}`), sign(secret, payload, now), now); err == nil {
		t.Fatal("tampered payload accepted")
	}

	// Wrong secret → rejected.
	if err := svc.VerifyWebhook(payload, sign("whsec_wrong", payload, now), now); err == nil {
		t.Fatal("wrong-secret signature accepted")
	}

	// Stale timestamp (beyond tolerance) → rejected (replay guard).
	stale := now.Add(-webhookTolerance - time.Minute)
	if err := svc.VerifyWebhook(payload, sign(secret, payload, stale), now); err == nil {
		t.Fatal("stale signature accepted")
	}

	// Malformed header → rejected.
	if err := svc.VerifyWebhook(payload, "garbage", now); err == nil {
		t.Fatal("malformed header accepted")
	}

	// No webhook secret configured → always rejected.
	noSecret := New(Config{SecretKey: "sk_test_x"})
	if err := noSecret.VerifyWebhook(payload, sign(secret, payload, now), now); err == nil {
		t.Fatal("verified with no webhook secret configured")
	}
}

func TestEnabledGating(t *testing.T) {
	if New(Config{}).Enabled() {
		t.Fatal("empty config should be disabled")
	}
	if !New(Config{SecretKey: "sk_test_x"}).Enabled() {
		t.Fatal("secret key should enable the service")
	}
	if New(Config{SecretKey: "sk_test_x"}).WebhookEnabled() {
		t.Fatal("webhook should be disabled without the webhook secret")
	}
	if !New(Config{SecretKey: "sk_test_x", WebhookSecret: "whsec_x"}).WebhookEnabled() {
		t.Fatal("webhook should be enabled with both secrets")
	}
	svc := New(Config{PriceMonthly: "price_m", PriceAnnual: "price_a"})
	if svc.PriceID("month") != "price_m" || svc.PriceID("year") != "price_a" || svc.PriceID("bogus") != "" {
		t.Fatalf("PriceID mapping wrong: %q %q %q", svc.PriceID("month"), svc.PriceID("year"), svc.PriceID("bogus"))
	}
}
