package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidSignature = errors.New("invalid signature")
	ErrInvalidPublicKey = errors.New("invalid public key")
	ErrExpiredTimestamp  = errors.New("request timestamp expired")
)

const MaxTimestampSkew = 5 * time.Minute

// VerifyRequest checks that a request was signed by the holder of the given public key.
// The message is: method + path + timestamp (RFC3339).
func VerifyRequest(publicKeyB64 string, method, path, timestamp, signatureB64 string) error {
	pubKeyBytes, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		return ErrInvalidPublicKey
	}

	ts, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}
	if time.Since(ts).Abs() > MaxTimestampSkew {
		return ErrExpiredTimestamp
	}

	sig, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return ErrInvalidSignature
	}

	message := []byte(method + path + timestamp)
	if !ed25519.Verify(pubKeyBytes, message, sig) {
		return ErrInvalidSignature
	}

	return nil
}

// GenerateKeyPair creates a new Ed25519 key pair, returning base64-encoded strings.
func GenerateKeyPair() (publicKeyB64, privateKeyB64 string, err error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(pub), base64.StdEncoding.EncodeToString(priv), nil
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
