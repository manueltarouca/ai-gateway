package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrGenerateCreatesKeys(t *testing.T) {
	dir := t.TempDir()

	kp, err := LoadOrGenerate(dir)
	if err != nil {
		t.Fatalf("LoadOrGenerate failed: %v", err)
	}
	if kp.PublicKey == "" || kp.PrivateKey == "" {
		t.Fatal("expected non-empty keys")
	}

	// Files should exist
	if _, err := os.Stat(filepath.Join(dir, "node.pub")); err != nil {
		t.Fatalf("public key file not found: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "node.key")); err != nil {
		t.Fatalf("private key file not found: %v", err)
	}
}

func TestLoadOrGenerateReusesExisting(t *testing.T) {
	dir := t.TempDir()

	kp1, _ := LoadOrGenerate(dir)
	kp2, _ := LoadOrGenerate(dir)

	if kp1.PublicKey != kp2.PublicKey {
		t.Fatal("expected same public key on second load")
	}
	if kp1.PrivateKey != kp2.PrivateKey {
		t.Fatal("expected same private key on second load")
	}
}

func TestSignProducesValidSignature(t *testing.T) {
	dir := t.TempDir()
	kp, _ := LoadOrGenerate(dir)

	ts := Timestamp()
	sig, err := Sign(kp.PrivateKey, "GET", "/api/tasks/next", ts)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	if sig == "" {
		t.Fatal("expected non-empty signature")
	}
}

func TestKeyFilePermissions(t *testing.T) {
	dir := t.TempDir()
	LoadOrGenerate(dir)

	info, _ := os.Stat(filepath.Join(dir, "node.key"))
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Fatalf("expected private key permissions 0600, got %o", perm)
	}
}
