package auth

import (
	"testing"
	"time"
)

func TestGenerateKeyPair(t *testing.T) {
	pub, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}
	if pub == "" || priv == "" {
		t.Fatal("expected non-empty keys")
	}
}

func TestSignAndVerify(t *testing.T) {
	pub, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	method := "POST"
	path := "/api/tasks/next"
	timestamp := time.Now().UTC().Format(time.RFC3339)

	sig, err := Sign(priv, method, path, timestamp)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	err = VerifyRequest(pub, method, path, timestamp, sig)
	if err != nil {
		t.Fatalf("VerifyRequest failed: %v", err)
	}
}

func TestVerifyRejectsWrongKey(t *testing.T) {
	_, priv, _ := GenerateKeyPair()
	otherPub, _, _ := GenerateKeyPair()

	timestamp := time.Now().UTC().Format(time.RFC3339)
	sig, _ := Sign(priv, "GET", "/test", timestamp)

	err := VerifyRequest(otherPub, "GET", "/test", timestamp, sig)
	if err != ErrInvalidSignature {
		t.Fatalf("expected ErrInvalidSignature, got: %v", err)
	}
}

func TestVerifyRejectsTamperedPath(t *testing.T) {
	pub, priv, _ := GenerateKeyPair()

	timestamp := time.Now().UTC().Format(time.RFC3339)
	sig, _ := Sign(priv, "GET", "/original", timestamp)

	err := VerifyRequest(pub, "GET", "/tampered", timestamp, sig)
	if err != ErrInvalidSignature {
		t.Fatalf("expected ErrInvalidSignature, got: %v", err)
	}
}

func TestVerifyRejectsExpiredTimestamp(t *testing.T) {
	pub, priv, _ := GenerateKeyPair()

	old := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	sig, _ := Sign(priv, "GET", "/test", old)

	err := VerifyRequest(pub, "GET", "/test", old, sig)
	if err != ErrExpiredTimestamp {
		t.Fatalf("expected ErrExpiredTimestamp, got: %v", err)
	}
}

func TestVerifyRejectsInvalidPublicKey(t *testing.T) {
	err := VerifyRequest("not-a-key", "GET", "/test", time.Now().UTC().Format(time.RFC3339), "sig")
	if err != ErrInvalidPublicKey {
		t.Fatalf("expected ErrInvalidPublicKey, got: %v", err)
	}
}

func TestVerifyRejectsFutureTimestamp(t *testing.T) {
	pub, priv, _ := GenerateKeyPair()

	future := time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339)
	sig, _ := Sign(priv, "GET", "/test", future)

	err := VerifyRequest(pub, "GET", "/test", future, sig)
	if err != ErrExpiredTimestamp {
		t.Fatalf("expected ErrExpiredTimestamp, got: %v", err)
	}
}
