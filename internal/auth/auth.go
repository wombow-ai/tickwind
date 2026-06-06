// Package auth verifies Supabase-issued JWTs and exposes the authenticated user
// via request context. Stdlib only. It verifies ES256 tokens against the
// project's published JWKS (Supabase's current asymmetric signing keys) and
// also HS256 tokens against the legacy shared secret. Verification dispatches on
// the token's `alg` using the matching key type, so there is no algorithm
// confusion (an HS256 token is never checked against an ECDSA public key).
package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ctxKey int

const userKey ctxKey = iota

// User is the authenticated principal from a Supabase JWT.
type User struct {
	ID    string // Supabase auth user UUID (the `sub` claim)
	Email string
}

// Verifier verifies tokens using a project's JWKS (ES256) and/or its legacy
// HS256 secret.
type Verifier struct {
	secret  []byte // legacy HS256 secret (optional)
	jwksURL string // ES256 public keys endpoint (optional)
	http    *http.Client

	mu        sync.Mutex
	keys      map[string]*ecdsa.PublicKey // by kid
	fetchedAt time.Time
}

// NewVerifier returns a Verifier. `secret` enables HS256 verification; `jwksURL`
// enables ES256 verification (e.g. https://<ref>.supabase.co/auth/v1/.well-known/jwks.json).
// With neither set the verifier is disabled and rejects every token.
func NewVerifier(secret, jwksURL string) *Verifier {
	return &Verifier{
		secret:  []byte(secret),
		jwksURL: strings.TrimSpace(jwksURL),
		http:    &http.Client{Timeout: 5 * time.Second},
		keys:    map[string]*ecdsa.PublicKey{},
	}
}

// Enabled reports whether any verification method is configured.
func (v *Verifier) Enabled() bool { return len(v.secret) > 0 || v.jwksURL != "" }

// ErrInvalid is returned for any malformed, mis-signed, or expired token.
var ErrInvalid = errors.New("auth: invalid token")

var b64 = base64.RawURLEncoding

type claims struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Exp   int64  `json:"exp"`
}

// Verify checks a JWT's signature (ES256 via JWKS, or HS256 via the secret) and
// its expiry, returning the authenticated user.
func (v *Verifier) Verify(token string) (User, error) {
	if !v.Enabled() {
		return User{}, ErrInvalid
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return User{}, ErrInvalid
	}

	hdrJSON, err := b64.DecodeString(parts[0])
	if err != nil {
		return User{}, ErrInvalid
	}
	var hdr struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if json.Unmarshal(hdrJSON, &hdr) != nil {
		return User{}, ErrInvalid
	}

	signing := parts[0] + "." + parts[1]
	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return User{}, ErrInvalid
	}

	switch hdr.Alg {
	case "HS256":
		if len(v.secret) == 0 {
			return User{}, ErrInvalid
		}
		mac := hmac.New(sha256.New, v.secret)
		mac.Write([]byte(signing))
		if !hmac.Equal(sig, mac.Sum(nil)) {
			return User{}, ErrInvalid
		}
	case "ES256":
		pub, err := v.es256Key(hdr.Kid)
		if err != nil || pub == nil {
			return User{}, ErrInvalid
		}
		if len(sig) != 64 { // r || s, 32 bytes each for P-256
			return User{}, ErrInvalid
		}
		r := new(big.Int).SetBytes(sig[:32])
		s := new(big.Int).SetBytes(sig[32:])
		h := sha256.Sum256([]byte(signing))
		if !ecdsa.Verify(pub, h[:], r, s) {
			return User{}, ErrInvalid
		}
	default:
		return User{}, ErrInvalid
	}

	clmJSON, err := b64.DecodeString(parts[1])
	if err != nil {
		return User{}, ErrInvalid
	}
	var c claims
	if json.Unmarshal(clmJSON, &c) != nil || c.Sub == "" {
		return User{}, ErrInvalid
	}
	if c.Exp != 0 && time.Now().Unix() >= c.Exp {
		return User{}, ErrInvalid
	}
	return User{ID: c.Sub, Email: c.Email}, nil
}

// es256Key returns the cached ES256 public key for kid, fetching the JWKS when
// the key is unknown. Refetches are rate-limited so unknown kids can't trigger
// repeated network calls.
func (v *Verifier) es256Key(kid string) (*ecdsa.PublicKey, error) {
	if v.jwksURL == "" {
		return nil, ErrInvalid
	}
	v.mu.Lock()
	pub, ok := v.keys[kid]
	stale := time.Since(v.fetchedAt) > time.Minute
	v.mu.Unlock()
	if ok {
		return pub, nil
	}
	if !stale {
		return nil, ErrInvalid
	}
	if err := v.refreshJWKS(); err != nil {
		return nil, err
	}
	v.mu.Lock()
	pub = v.keys[kid]
	v.mu.Unlock()
	if pub == nil {
		return nil, ErrInvalid
	}
	return pub, nil
}

// refreshJWKS fetches and caches the project's EC P-256 public keys.
func (v *Verifier) refreshJWKS() error {
	resp, err := v.http.Get(v.jwksURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ErrInvalid
	}
	var doc struct {
		Keys []struct {
			Kty string `json:"kty"`
			Crv string `json:"crv"`
			Kid string `json:"kid"`
			X   string `json:"x"`
			Y   string `json:"y"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return err
	}
	keys := make(map[string]*ecdsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Kty != "EC" || k.Crv != "P-256" || k.Kid == "" {
			continue
		}
		xb, err := b64.DecodeString(k.X)
		if err != nil {
			continue
		}
		yb, err := b64.DecodeString(k.Y)
		if err != nil {
			continue
		}
		keys[k.Kid] = &ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     new(big.Int).SetBytes(xb),
			Y:     new(big.Int).SetBytes(yb),
		}
	}
	v.mu.Lock()
	v.keys = keys
	v.fetchedAt = time.Now()
	v.mu.Unlock()
	return nil
}

// Middleware attaches the authenticated user to the context when a valid bearer
// token is present. It does NOT reject anonymous requests — handlers decide via
// UserFrom, so public endpoints stay open.
func (v *Verifier) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tok := bearer(r); tok != "" {
			if u, err := v.Verify(tok); err == nil {
				r = r.WithContext(context.WithValue(r.Context(), userKey, u))
			}
		}
		next.ServeHTTP(w, r)
	})
}

func bearer(r *http.Request) string {
	if after, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
}

// UserFrom returns the authenticated user attached to ctx, if any.
func UserFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey).(User)
	return u, ok
}
