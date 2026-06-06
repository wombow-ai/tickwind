package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	v := NewVerifier(testSecret, "")
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
	v := NewVerifier(testSecret, "")
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
	v := NewVerifier(testSecret, "")
	enc := base64.RawURLEncoding
	hdr, _ := json.Marshal(map[string]string{"alg": "none", "typ": "JWT"})
	pl, _ := json.Marshal(map[string]any{"sub": "u", "exp": time.Now().Add(time.Hour).Unix()})
	tok := enc.EncodeToString(hdr) + "." + enc.EncodeToString(pl) + "."
	if _, err := v.Verify(tok); err == nil {
		t.Fatal("expected rejection of alg=none")
	}
}

func TestDisabledVerifierRejectsAll(t *testing.T) {
	v := NewVerifier("", "")
	if v.Enabled() {
		t.Fatal("want disabled")
	}
	tok := mint(t, "x", map[string]any{"sub": "u", "exp": time.Now().Add(time.Hour).Unix()})
	if _, err := v.Verify(tok); err == nil {
		t.Fatal("disabled verifier must reject")
	}
}

// mintES256 signs a JWT with an ECDSA P-256 key (the format Supabase uses).
func mintES256(t *testing.T, key *ecdsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	enc := base64.RawURLEncoding
	hdr, _ := json.Marshal(map[string]string{"alg": "ES256", "typ": "JWT", "kid": kid})
	pl, _ := json.Marshal(claims)
	signing := enc.EncodeToString(hdr) + "." + enc.EncodeToString(pl)
	h := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, key, h[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signing + "." + enc.EncodeToString(sig)
}

// jwksServer serves a JWKS document exposing pub under kid.
func jwksServer(t *testing.T, kid string, pub *ecdsa.PublicKey) *httptest.Server {
	t.Helper()
	enc := base64.RawURLEncoding
	xb := make([]byte, 32)
	yb := make([]byte, 32)
	pub.X.FillBytes(xb)
	pub.Y.FillBytes(yb)
	body := map[string]any{"keys": []map[string]string{{
		"kty": "EC", "crv": "P-256", "use": "sig", "alg": "ES256",
		"kid": kid, "x": enc.EncodeToString(xb), "y": enc.EncodeToString(yb),
	}}}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func TestVerifyES256(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "kid-1"
	srv := jwksServer(t, kid, &key.PublicKey)
	defer srv.Close()

	v := NewVerifier("", srv.URL)
	if !v.Enabled() {
		t.Fatal("verifier with JWKS should be enabled")
	}

	tok := mintES256(t, key, kid, map[string]any{
		"sub": "u-es", "email": "e@s.com",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	u, err := v.Verify(tok)
	if err != nil {
		t.Fatalf("valid ES256 token rejected: %v", err)
	}
	if u.ID != "u-es" || u.Email != "e@s.com" {
		t.Fatalf("got %+v", u)
	}

	// Token signed by a different key, same kid → reject.
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	bad := mintES256(t, other, kid, map[string]any{"sub": "u", "exp": time.Now().Add(time.Hour).Unix()})
	if _, err := v.Verify(bad); err == nil {
		t.Error("expected rejection of token signed by the wrong key")
	}

	// Unknown kid → reject.
	bad2 := mintES256(t, key, "unknown", map[string]any{"sub": "u", "exp": time.Now().Add(time.Hour).Unix()})
	if _, err := v.Verify(bad2); err == nil {
		t.Error("expected rejection of unknown kid")
	}
}
