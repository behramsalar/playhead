package auth_test

import (
	"testing"
	"time"

	"playhead/internal/auth"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := auth.HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	if !auth.VerifyPassword(hash, "correct horse battery staple") {
		t.Fatal("expected the correct password to verify")
	}
	if auth.VerifyPassword(hash, "wrong password") {
		t.Fatal("expected an incorrect password to fail verification")
	}
}

func TestVerifyPasswordAgainstMalformedHash(t *testing.T) {
	if auth.VerifyPassword("not-a-real-bcrypt-hash", "anything") {
		t.Fatal("expected a malformed hash to never verify")
	}
}

func TestSignAndVerifySession(t *testing.T) {
	secret, err := auth.GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret error: %v", err)
	}

	token, err := auth.SignSession(secret, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("SignSession error: %v", err)
	}

	ok, err := auth.VerifySession(secret, token)
	if err != nil {
		t.Fatalf("VerifySession error: %v", err)
	}
	if !ok {
		t.Fatal("expected a freshly-signed, unexpired token to verify")
	}
}

func TestVerifySessionRejectsExpiredToken(t *testing.T) {
	secret, err := auth.GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret error: %v", err)
	}

	token, err := auth.SignSession(secret, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("SignSession error: %v", err)
	}

	ok, err := auth.VerifySession(secret, token)
	if err != nil {
		t.Fatalf("VerifySession error: %v", err)
	}
	if ok {
		t.Fatal("expected an expired token to fail verification")
	}
}

func TestVerifySessionRejectsTamperedToken(t *testing.T) {
	secret, err := auth.GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret error: %v", err)
	}
	token, err := auth.SignSession(secret, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("SignSession error: %v", err)
	}

	// Flip the last character of the signature.
	tampered := token[:len(token)-1] + "x"
	if tampered == token {
		tampered = token[:len(token)-1] + "y"
	}

	ok, err := auth.VerifySession(secret, tampered)
	if err != nil {
		t.Fatalf("VerifySession error: %v", err)
	}
	if ok {
		t.Fatal("expected a tampered token to fail verification")
	}
}

func TestVerifySessionRejectsWrongSecret(t *testing.T) {
	secretA, err := auth.GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	secretB, err := auth.GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.SignSession(secretA, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("SignSession error: %v", err)
	}

	ok, err := auth.VerifySession(secretB, token)
	if err != nil {
		t.Fatalf("VerifySession error: %v", err)
	}
	if ok {
		t.Fatal("expected a token signed with a different (rotated) secret to fail verification")
	}
}

func TestVerifySessionRejectsMalformedToken(t *testing.T) {
	secret, err := auth.GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "no-dot-here", "a.b.c", "not-base64!.also-not-base64!"} {
		if ok, _ := auth.VerifySession(secret, bad); ok {
			t.Fatalf("expected malformed token %q to fail verification", bad)
		}
	}
}
