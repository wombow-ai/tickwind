// Package billing is an optional, pluggable Stripe integration for the Pro
// entitlement. Like internal/enrich it is STDLIB-ONLY (no SDK): it speaks Stripe's
// HTTPS form API directly and verifies webhook signatures with crypto/hmac, keeping
// the project's stdlib-first ethos and avoiding an SDK dependency + its CVE surface.
//
// It is DISABLED by default: New returns a Service whose Enabled() is false when no
// secret key is configured, and the API layer registers no behavior / serves 404 in
// that case — so a keyless deployment (the current production state) behaves exactly
// as before. Nothing here touches live money until the owner sets the Stripe env.
package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// stripeAPI is the Stripe REST base. Stripe has no separate test host — test mode is
// selected purely by using a test-mode secret key (sk_test_…), so this is constant.
const stripeAPI = "https://api.stripe.com"

// webhookTolerance bounds the age of a webhook timestamp (replay / clock-skew guard).
const webhookTolerance = 5 * time.Minute

// ErrBadSignature is returned when a webhook signature fails verification.
var ErrBadSignature = errors.New("billing: webhook signature verification failed")

// Config configures the Stripe integration. An empty SecretKey disables it.
type Config struct {
	SecretKey     string // sk_test_… / sk_live_… — empty disables the whole surface
	WebhookSecret string // whsec_… — empty disables ONLY the webhook (signatures unverifiable)
	PriceMonthly  string // price_… for the monthly Pro plan
	PriceAnnual   string // price_… for the annual Pro plan
	PublicSiteURL string // site origin (e.g. https://tickwind.com) for checkout/portal redirects
	APIBaseURL    string // override the Stripe API base (tests/self-host); empty → stripeAPI
}

// Service talks to Stripe over its form API (stdlib net/http).
type Service struct {
	cfg     Config
	http    *http.Client
	baseURL string
}

// New returns a Service. It is always non-nil (so callers can hold a concrete
// pointer), but Enabled() reports false until a secret key is set.
func New(cfg Config) *Service {
	base := cfg.APIBaseURL
	if base == "" {
		base = stripeAPI
	}
	return &Service{cfg: cfg, http: &http.Client{Timeout: 20 * time.Second}, baseURL: base}
}

// Enabled reports whether a Stripe secret key is configured (the whole surface is
// inert otherwise).
func (s *Service) Enabled() bool { return s != nil && s.cfg.SecretKey != "" }

// WebhookEnabled reports whether inbound webhooks can be verified (needs both the
// secret key and the webhook signing secret).
func (s *Service) WebhookEnabled() bool { return s.Enabled() && s.cfg.WebhookSecret != "" }

// PriceID maps a plan interval ("month"|"year"/"annual") to the configured Stripe
// price id; "" for an unknown interval.
func (s *Service) PriceID(interval string) string {
	switch strings.ToLower(interval) {
	case "year", "annual", "yearly":
		return s.cfg.PriceAnnual
	case "month", "monthly":
		return s.cfg.PriceMonthly
	default:
		return ""
	}
}

// VerifyWebhook checks the Stripe-Signature header against the RAW payload using the
// webhook signing secret: HMAC-SHA256 over "t.payload", constant-time compared to a
// v1 signature, within a replay-tolerance window. Returns nil on success. Stdlib
// crypto/hmac — equivalent to stripe-go's webhook.ConstructEvent verification.
func (s *Service) VerifyWebhook(payload []byte, sigHeader string, now time.Time) error {
	if s.cfg.WebhookSecret == "" {
		return ErrBadSignature
	}
	var ts string
	var v1s []string
	for _, part := range strings.Split(sigHeader, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			ts = kv[1]
		case "v1":
			v1s = append(v1s, kv[1])
		}
	}
	if ts == "" || len(v1s) == 0 {
		return ErrBadSignature
	}
	tsec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return ErrBadSignature
	}
	if d := now.Sub(time.Unix(tsec, 0)); d > webhookTolerance || d < -webhookTolerance {
		return ErrBadSignature // stale / future-dated → replay or skew
	}
	mac := hmac.New(sha256.New, []byte(s.cfg.WebhookSecret))
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write(payload)
	expected := mac.Sum(nil)
	for _, v := range v1s {
		sig, err := hex.DecodeString(v)
		if err != nil {
			continue
		}
		if hmac.Equal(sig, expected) {
			return nil
		}
	}
	return ErrBadSignature
}

// Event is the envelope of a Stripe webhook event (only the fields we consume).
type Event struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		Object json.RawMessage `json:"object"`
	} `json:"data"`
}

// ParseEvent unmarshals a verified webhook payload into the Event envelope.
func ParseEvent(payload []byte) (Event, error) {
	var e Event
	if err := json.Unmarshal(payload, &e); err != nil {
		return Event{}, fmt.Errorf("billing: parse event: %w", err)
	}
	return e, nil
}

// CheckoutSession is the subset of a checkout.session object we read (to bind a
// Supabase user — client_reference_id — to a Stripe customer).
type CheckoutSession struct {
	ID                string `json:"id"`
	ClientReferenceID string `json:"client_reference_id"`
	Customer          string `json:"customer"`
	Subscription      string `json:"subscription"`
}

// Subscription is the subset of a Stripe subscription object we read.
type Subscription struct {
	ID                string `json:"id"`
	Customer          string `json:"customer"`
	Status            string `json:"status"`
	CancelAtPeriodEnd bool   `json:"cancel_at_period_end"`
	CurrentPeriodEnd  int64  `json:"current_period_end"`
	Items             struct {
		Data []struct {
			// Newer Stripe API versions moved current_period_end OFF the subscription
			// onto the line item; we read both and prefer the top-level (PeriodEnd).
			CurrentPeriodEnd int64 `json:"current_period_end"`
			Price            struct {
				ID        string `json:"id"`
				Recurring struct {
					Interval string `json:"interval"`
				} `json:"recurring"`
			} `json:"price"`
		} `json:"data"`
	} `json:"items"`
}

// PeriodEnd returns the subscription's current-period-end unix ts, falling back to the
// first line item's value (newer Stripe API versions carry it there, not on the sub). 0
// if neither is set.
func (sub Subscription) PeriodEnd() int64 {
	if sub.CurrentPeriodEnd > 0 {
		return sub.CurrentPeriodEnd
	}
	if len(sub.Items.Data) > 0 {
		return sub.Items.Data[0].CurrentPeriodEnd
	}
	return 0
}

// PriceID returns the subscription's first line-item price id (the plan), "" if none.
func (sub Subscription) PriceID() string {
	if len(sub.Items.Data) > 0 {
		return sub.Items.Data[0].Price.ID
	}
	return ""
}

// Interval returns the subscription's billing interval ("month"|"year"), "" if none.
func (sub Subscription) Interval() string {
	if len(sub.Items.Data) > 0 {
		return sub.Items.Data[0].Price.Recurring.Interval
	}
	return ""
}

// CreateCheckoutSession creates a subscription Checkout Session for the given user +
// price and returns the hosted-checkout URL. customerID may be "" (Stripe creates a
// new customer); client_reference_id carries the Supabase user id so the webhook can
// bind it. successURL/cancelURL are absolute.
func (s *Service) CreateCheckoutSession(ctx context.Context, userID, customerID, priceID, successURL, cancelURL string) (string, error) {
	form := url.Values{}
	form.Set("mode", "subscription")
	form.Set("line_items[0][price]", priceID)
	form.Set("line_items[0][quantity]", "1")
	form.Set("success_url", successURL)
	form.Set("cancel_url", cancelURL)
	form.Set("client_reference_id", userID)
	form.Set("allow_promotion_codes", "true")
	if customerID != "" {
		form.Set("customer", customerID)
	}
	var out struct {
		URL string `json:"url"`
	}
	if err := s.post(ctx, "/v1/checkout/sessions", form, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

// CreatePortalSession creates a Billing Portal session (manage/cancel) for a customer
// and returns its URL.
func (s *Service) CreatePortalSession(ctx context.Context, customerID, returnURL string) (string, error) {
	form := url.Values{}
	form.Set("customer", customerID)
	form.Set("return_url", returnURL)
	var out struct {
		URL string `json:"url"`
	}
	if err := s.post(ctx, "/v1/billing_portal/sessions", form, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

// Checkout is the high-level entry the API layer calls: it maps the plan interval to
// the configured price, builds the success/cancel redirects from PublicSiteURL, and
// returns the hosted-checkout URL. Returns an error for an unknown interval.
func (s *Service) Checkout(ctx context.Context, userID, customerID, interval string) (string, error) {
	priceID := s.PriceID(interval)
	if priceID == "" {
		return "", fmt.Errorf("billing: unknown plan interval %q", interval)
	}
	base := strings.TrimRight(s.cfg.PublicSiteURL, "/")
	success := base + "/pro/success?session_id={CHECKOUT_SESSION_ID}"
	cancel := base + "/pro"
	return s.CreateCheckoutSession(ctx, userID, customerID, priceID, success, cancel)
}

// Portal is the high-level entry: it builds the return URL from PublicSiteURL and
// opens the Stripe Billing Portal for managing/cancelling the subscription.
func (s *Service) Portal(ctx context.Context, customerID string) (string, error) {
	base := strings.TrimRight(s.cfg.PublicSiteURL, "/")
	return s.CreatePortalSession(ctx, customerID, base+"/me")
}

// post sends a form-encoded POST to the Stripe API with the secret-key bearer and
// decodes the JSON response. A non-2xx status returns an error including Stripe's
// body (truncated) for diagnosis.
func (s *Service) post(ctx context.Context, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return s.do(req, path, out)
}

// GetSubscription fetches a subscription's CURRENT authoritative state from Stripe by id.
// Used to recover entitlement when a subscription.* webhook arrived out of order (before
// the checkout that binds the user↔customer) — the checkout handler then re-pulls the
// subscription rather than defaulting the user to free.
func (s *Service) GetSubscription(ctx context.Context, subID string) (Subscription, error) {
	var sub Subscription
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/v1/subscriptions/"+url.PathEscape(subID), nil)
	if err != nil {
		return Subscription{}, err
	}
	if err := s.do(req, "/v1/subscriptions", &sub); err != nil {
		return Subscription{}, err
	}
	return sub, nil
}

// ListSubscriptions fetches ALL subscriptions from Stripe (any status), following
// pagination via the starting_after cursor. The reconciler uses this to re-sync our
// stored tiers against Stripe's authoritative state. Bounded by maxPages so a runaway
// cursor (or an enormous account) can never loop forever.
func (s *Service) ListSubscriptions(ctx context.Context) ([]Subscription, error) {
	const pageSize = 100
	const maxPages = 100 // 10k subscriptions hard cap — far beyond any realistic size
	var out []Subscription
	startingAfter := ""
	for page := 0; page < maxPages; page++ {
		q := url.Values{}
		q.Set("status", "all")
		q.Set("limit", strconv.Itoa(pageSize))
		if startingAfter != "" {
			q.Set("starting_after", startingAfter)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/v1/subscriptions?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		var resp struct {
			Data    []Subscription `json:"data"`
			HasMore bool           `json:"has_more"`
		}
		if err := s.do(req, "/v1/subscriptions", &resp); err != nil {
			return nil, err
		}
		out = append(out, resp.Data...)
		if !resp.HasMore || len(resp.Data) == 0 {
			return out, nil
		}
		startingAfter = resp.Data[len(resp.Data)-1].ID
	}
	// Hit the page cap while Stripe still has more: a PARTIAL list. Refuse it rather than
	// return silently truncated data — the reconciler must never reverse-revoke users off
	// an incomplete snapshot.
	return nil, fmt.Errorf("billing: subscription list exceeded %d pages — refusing a truncated result", maxPages)
}

// do sets the secret-key bearer, executes the request, and decodes the JSON response.
// A non-2xx status returns an error including Stripe's body (truncated) for diagnosis.
func (s *Service) do(req *http.Request, path string, out any) error {
	req.Header.Set("Authorization", "Bearer "+s.cfg.SecretKey)
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("billing: stripe %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode/100 != 2 {
		snippet := string(body)
		if len(snippet) > 300 {
			snippet = snippet[:300]
		}
		return fmt.Errorf("billing: stripe %s status %d: %s", path, resp.StatusCode, snippet)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("billing: stripe %s decode: %w", path, err)
	}
	return nil
}
