// Package auth verifies Supabase-issued JWTs (HS256) and exposes the
// authenticated user via request context. Stdlib only — verification is
// hardcoded to HS256 to prevent algorithm-confusion attacks.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

type ctxKey int

const userKey ctxKey = iota

// User is the authenticated principal from a Supabase JWT.
type User struct {
	ID    string // Supabase auth user UUID (the `sub` claim)
	Email string
}

// Verifier verifies tokens with a project's HS256 JWT secret.
type Verifier struct {
	secret []byte
}

// NewVerifier returns a Verifier. An empty secret yields a disabled verifier
// (Enabled reports false; every token is rejected).
func NewVerifier(secret string) *Verifier {
	return &Verifier{secret: []byte(secret)}
}

// Enabled reports whether a secret is configured.
func (v *Verifier) Enabled() bool { return len(v.secret) > 0 }

// ErrInvalid is returned for any malformed, mis-signed, or expired token.
var ErrInvalid = errors.New("auth: invalid token")

var b64 = base64.RawURLEncoding

type claims struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Exp   int64  `json:"exp"`
}

// Verify checks a JWT's HS256 signature and expiry and returns the user.
func (v *Verifier) Verify(token string) (User, error) {
	if !v.Enabled() {
		return User{}, ErrInvalid
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return User{}, ErrInvalid
	}

	// Header: require alg HS256 (reject "none"/asymmetric to block confusion).
	hdrJSON, err := b64.DecodeString(parts[0])
	if err != nil {
		return User{}, ErrInvalid
	}
	var hdr struct {
		Alg string `json:"alg"`
	}
	if json.Unmarshal(hdrJSON, &hdr) != nil || hdr.Alg != "HS256" {
		return User{}, ErrInvalid
	}

	// Signature: constant-time compare.
	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return User{}, ErrInvalid
	}
	mac := hmac.New(sha256.New, v.secret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return User{}, ErrInvalid
	}

	// Claims.
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

// Middleware attaches the authenticated user to the context when a valid
// bearer token is present. It does NOT reject anonymous requests — handlers
// decide via UserFrom, so public endpoints stay open.
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
