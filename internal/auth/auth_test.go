package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

const testSecret = "test-secret-0123456789"

func mint(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	enc := base64.RawURLEncoding
	hdr, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	pl, _ := json.Marshal(claims)
	signing := enc.EncodeToString(hdr) + "." + enc.EncodeToString(pl)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signing))
	return signing + "." + enc.EncodeToString(mac.Sum(nil))
}

func TestVerifyValid(t *testing.T) {
	v := NewVerifier(testSecret)
	tok := mint(t, testSecret, map[string]any{
		"sub": "user-123", "email": "a@b.com",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	u, err := v.Verify(tok)
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != "user-123" || u.Email != "a@b.com" {
		t.Fatalf("got %+v", u)
	}
}

func TestVerifyRejects(t *testing.T) {
	v := NewVerifier(testSecret)
	cases := map[string]string{
		"wrong secret": mint(t, "other-secret", map[string]any{"sub": "u", "exp": time.Now().Add(time.Hour).Unix()}),
		"expired":      mint(t, testSecret, map[string]any{"sub": "u", "exp": time.Now().Add(-time.Hour).Unix()}),
		"no sub":       mint(t, testSecret, map[string]any{"exp": time.Now().Add(time.Hour).Unix()}),
		"malformed":    "not.a.jwt.x",
		"empty":        "",
	}
	for name, tok := range cases {
		if _, err := v.Verify(tok); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
}

func TestVerifyRejectsNoneAlg(t *testing.T) {
	v := NewVerifier(testSecret)
	enc := base64.RawURLEncoding
	hdr, _ := json.Marshal(map[string]string{"alg": "none", "typ": "JWT"})
	pl, _ := json.Marshal(map[string]any{"sub": "u", "exp": time.Now().Add(time.Hour).Unix()})
	tok := enc.EncodeToString(hdr) + "." + enc.EncodeToString(pl) + "."
	if _, err := v.Verify(tok); err == nil {
		t.Fatal("expected rejection of alg=none")
	}
}

func TestDisabledVerifierRejectsAll(t *testing.T) {
	v := NewVerifier("")
	if v.Enabled() {
		t.Fatal("want disabled")
	}
	tok := mint(t, "x", map[string]any{"sub": "u", "exp": time.Now().Add(time.Hour).Unix()})
	if _, err := v.Verify(tok); err == nil {
		t.Fatal("disabled verifier must reject")
	}
}
