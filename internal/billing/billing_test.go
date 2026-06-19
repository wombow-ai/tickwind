package billing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

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
