package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type KeyPair struct {
	PublicKey  string
	PrivateKey string
}

// LoadOrGenerate loads existing keys from keyDir, or generates new ones.
func LoadOrGenerate(keyDir string) (*KeyPair, error) {
	pubPath := filepath.Join(keyDir, "node.pub")
	privPath := filepath.Join(keyDir, "node.key")

	pubBytes, pubErr := os.ReadFile(pubPath)
	privBytes, privErr := os.ReadFile(privPath)

	if pubErr == nil && privErr == nil {
		return &KeyPair{
			PublicKey:  string(pubBytes),
			PrivateKey: string(privBytes),
		}, nil
	}

	// Generate new pair
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, fmt.Errorf("generate key pair: %w", err)
	}

	pubB64 := base64.StdEncoding.EncodeToString(pub)
	privB64 := base64.StdEncoding.EncodeToString(priv)

	if err := os.MkdirAll(keyDir, 0700); err != nil {
		return nil, fmt.Errorf("create key dir: %w", err)
	}
	if err := os.WriteFile(pubPath, []byte(pubB64), 0644); err != nil {
		return nil, fmt.Errorf("write public key: %w", err)
	}
	if err := os.WriteFile(privPath, []byte(privB64), 0600); err != nil {
		return nil, fmt.Errorf("write private key: %w", err)
	}

	return &KeyPair{PublicKey: pubB64, PrivateKey: privB64}, nil
}

// Sign creates a signature for the given request parameters.
func Sign(privateKeyB64, method, path, timestamp string) (string, error) {
	privBytes, err := base64.StdEncoding.DecodeString(privateKeyB64)
	if err != nil || len(privBytes) != ed25519.PrivateKeySize {
		return "", errors.New("invalid private key")
	}

	message := []byte(method + path + timestamp)
	sig := ed25519.Sign(privBytes, message)
	return base64.StdEncoding.EncodeToString(sig), nil
}

// Timestamp returns the current UTC time in RFC3339 format.
func Timestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}
