// Package auth implements the Phase 7 password/session primitives: bcrypt
// password hashing and a signed, stateless session token. There is no
// server-side session table, a deliberate choice — a session is just an
// expiry timestamp plus an HMAC of it, keyed by a secret stored in
// config.json.
// Rotating that secret (on a password change) invalidates every existing
// session at once, with no revocation list to maintain.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// SessionLifetime is deliberately long: this gates casual access on a
// shared home network, not a high-security system, and being logged out
// every few hours would be pure friction with no real security benefit
// (see ARCHITECTURE.md).
const SessionLifetime = 90 * 24 * time.Hour

// HashPassword bcrypt-hashes a plaintext password for storage.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword reports whether password matches hash. Any error from
// bcrypt (including a malformed hash) is treated as "doesn't match" —
// callers never need to distinguish a wrong password from a corrupt
// stored hash.
func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// GenerateSecret returns a random, base64url-encoded 32-byte secret,
// suitable for storage as config.json's sessionSecret.
func GenerateSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

var errMalformedToken = errors.New("malformed session token")

// SignSession produces a session token good until expiry: the expiry
// encoded as an 8-byte big-endian unix timestamp, base64url, a ".", and a
// base64url HMAC-SHA256 of that payload keyed by secret (itself
// base64url, as returned by GenerateSecret).
func SignSession(secret string, expiry time.Time) (string, error) {
	key, err := base64.RawURLEncoding.DecodeString(secret)
	if err != nil {
		return "", err
	}

	var payload [8]byte
	binary.BigEndian.PutUint64(payload[:], uint64(expiry.Unix()))
	payloadEnc := base64.RawURLEncoding.EncodeToString(payload[:])

	mac := hmac.New(sha256.New, key)
	mac.Write(payload[:])
	sigEnc := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return payloadEnc + "." + sigEnc, nil
}

// VerifySession reports whether token is a well-formed, correctly-signed,
// not-yet-expired session token for secret.
func VerifySession(secret, token string) (bool, error) {
	key, err := base64.RawURLEncoding.DecodeString(secret)
	if err != nil {
		return false, err
	}

	payloadEnc, sigEnc, ok := strings.Cut(token, ".")
	if !ok {
		return false, errMalformedToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(payloadEnc)
	if err != nil || len(payload) != 8 {
		return false, errMalformedToken
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigEnc)
	if err != nil {
		return false, errMalformedToken
	}

	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return false, nil
	}

	expiry := time.Unix(int64(binary.BigEndian.Uint64(payload)), 0)
	return time.Now().Before(expiry), nil
}
