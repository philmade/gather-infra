// Package auth provides Ed25519 keypair identity for Gather agents.
//
// Port of reskill's src/lib/attestation.ts to Go.
// Agents generate Ed25519 keypairs locally. The private key never leaves the
// agent's machine. The server stores only the public key.
package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
)

// KeyPair holds an Ed25519 public/private key pair.
type KeyPair struct {
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

// GenerateKeyPair creates a new Ed25519 keypair.
func GenerateKeyPair() (*KeyPair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519 key: %w", err)
	}
	return &KeyPair{PublicKey: pub, PrivateKey: priv}, nil
}

// SaveKeyPair writes the keypair to disk at ~/.gather/keys/{name}.key and .pub
// with restrictive permissions (0600 for private, 0644 for public).
func SaveKeyPair(name string, kp *KeyPair) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	dir := filepath.Join(home, ".gather", "keys")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create keys dir: %w", err)
	}

	// Marshal private key to PKCS8 PEM
	privBytes, err := x509.MarshalPKCS8PrivateKey(kp.PrivateKey)
	if err != nil {
		return fmt.Errorf("marshal private key: %w", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})

	if err := os.WriteFile(filepath.Join(dir, name+".key"), privPEM, 0600); err != nil {
		return fmt.Errorf("write private key: %w", err)
	}

	// Marshal public key to SPKI PEM
	pubBytes, err := x509.MarshalPKIXPublicKey(kp.PublicKey)
	if err != nil {
		return fmt.Errorf("marshal public key: %w", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})

	if err := os.WriteFile(filepath.Join(dir, name+".pub"), pubPEM, 0644); err != nil {
		return fmt.Errorf("write public key: %w", err)
	}

	return nil
}

// LoadKeyPair reads a keypair from ~/.gather/keys/{name}.key and .pub.
func LoadKeyPair(name string) (*KeyPair, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}

	dir := filepath.Join(home, ".gather", "keys")

	privPEM, err := os.ReadFile(filepath.Join(dir, name+".key"))
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}

	pubPEM, err := os.ReadFile(filepath.Join(dir, name+".pub"))
	if err != nil {
		return nil, fmt.Errorf("read public key: %w", err)
	}

	priv, err := ParsePrivateKeyPEM(privPEM)
	if err != nil {
		return nil, err
	}

	pub, err := ParsePublicKeyPEM(pubPEM)
	if err != nil {
		return nil, err
	}

	return &KeyPair{PublicKey: pub, PrivateKey: priv}, nil
}

// ParsePublicKeyPEM decodes a PEM-encoded Ed25519 public key.
func ParsePublicKeyPEM(pemData []byte) (ed25519.PublicKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}

	edPub, ok := pub.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("key is not Ed25519")
	}

	return edPub, nil
}

// ParsePrivateKeyPEM decodes a PEM-encoded Ed25519 private key.
func ParsePrivateKeyPEM(pemData []byte) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}

	priv, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	edPriv, ok := priv.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is not Ed25519")
	}

	return edPriv, nil
}

// Sign signs a message with the private key.
func Sign(privateKey ed25519.PrivateKey, message []byte) []byte {
	return ed25519.Sign(privateKey, message)
}

// Verify checks a signature against a public key and message.
func Verify(publicKey ed25519.PublicKey, message, signature []byte) bool {
	return ed25519.Verify(publicKey, message, signature)
}

// Fingerprint returns the SHA-256 hex fingerprint of a public key (for JWTs).
func Fingerprint(publicKey ed25519.PublicKey) string {
	hash := sha256.Sum256(publicKey)
	return hex.EncodeToString(hash[:])
}

// EncodePEM returns the PEM-encoded form of a public key.
func EncodePEM(publicKey ed25519.PublicKey) ([]byte, error) {
	pubBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return nil, fmt.Errorf("marshal public key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes}), nil
}
