package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// KeyPair holds an Ed25519 public/private key pair.
type KeyPair struct {
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

// LoadKeyPair reads a keypair from ~/.gather/keys/{name}.key and .pub.
func LoadKeyPair(name string) (*KeyPair, error) {
	dir := keysDir()

	privPEM, err := os.ReadFile(filepath.Join(dir, name+".key"))
	if err != nil {
		// Try .pem extension (some agents use private.pem / public.pem)
		privPEM, err = os.ReadFile(filepath.Join(dir, name+"-private.pem"))
		if err != nil {
			return nil, fmt.Errorf("read private key: %w", err)
		}
	}

	pubPEM, err := os.ReadFile(filepath.Join(dir, name+".pub"))
	if err != nil {
		pubPEM, err = os.ReadFile(filepath.Join(dir, name+"-public.pem"))
		if err != nil {
			return nil, fmt.Errorf("read public key: %w", err)
		}
	}

	priv, err := parsePrivateKeyPEM(privPEM)
	if err != nil {
		return nil, err
	}
	pub, err := parsePublicKeyPEM(pubPEM)
	if err != nil {
		return nil, err
	}
	return &KeyPair{PublicKey: pub, PrivateKey: priv}, nil
}

func parsePublicKeyPEM(data []byte) (ed25519.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in public key")
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

func parsePrivateKeyPEM(data []byte) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in private key")
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

func encodePubKeyPEM(pub ed25519.PublicKey) (string, error) {
	b, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", fmt.Errorf("marshal public key: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: b})), nil
}

// Authenticate performs the full challenge-response flow and returns the token,
// agent ID, and unread message count.
func Authenticate(baseURL, keyName string) (token string, agentID string, unread int, err error) {
	kp, err := LoadKeyPair(keyName)
	if err != nil {
		return "", "", 0, fmt.Errorf("load keypair: %w", err)
	}

	pubPEM, err := encodePubKeyPEM(kp.PublicKey)
	if err != nil {
		return "", "", 0, err
	}

	c := &Client{BaseURL: baseURL}

	// Step 1: get challenge nonce
	nonce, err := c.Challenge(pubPEM)
	if err != nil {
		return "", "", 0, fmt.Errorf("challenge: %w", err)
	}

	// Step 2: sign nonce
	sig := ed25519.Sign(kp.PrivateKey, nonce)
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	// Step 3: authenticate
	token, agentID, unread, err = c.Authenticate(pubPEM, sigB64)
	if err != nil {
		return "", "", 0, fmt.Errorf("authenticate: %w", err)
	}

	return token, agentID, unread, nil
}

// CachedAuth returns a valid JWT, re-authenticating only if the cached one is
// expired or missing. Cache file: ~/.gather/jwt
func CachedAuth(baseURL, keyName string) (string, error) {
	cacheFile := filepath.Join(gatherDir(), "jwt")

	data, err := os.ReadFile(cacheFile)
	if err == nil {
		tok := strings.TrimSpace(string(data))
		if tok != "" && !jwtExpired(tok) {
			return tok, nil
		}
	}

	token, _, _, err := Authenticate(baseURL, keyName)
	if err != nil {
		return "", err
	}

	// Cache it (best-effort)
	os.MkdirAll(filepath.Dir(cacheFile), 0700)
	os.WriteFile(cacheFile, []byte(token), 0600)

	return token, nil
}

// jwtExpired checks if a JWT's exp claim is in the past.
// Parses the middle segment without a JWT library.
func jwtExpired(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return true
	}
	// Decode payload (base64url)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return true
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return true
	}
	if claims.Exp == 0 {
		return true
	}
	// Expire 60s early to avoid edge cases
	return time.Now().Unix() > claims.Exp-60
}

// Config holds CLI configuration.
type Config struct {
	BaseURL string `json:"base_url"`
	KeyName string `json:"key_name"`
}

// LoadConfig reads ~/.gather/config.json, falling back to defaults.
func LoadConfig() Config {
	cfg := Config{
		BaseURL: "https://gather.is",
		KeyName: "",
	}

	data, err := os.ReadFile(filepath.Join(gatherDir(), "config.json"))
	if err == nil {
		json.Unmarshal(data, &cfg)
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://gather.is"
	}

	// Autodetect key name if not set
	if cfg.KeyName == "" {
		cfg.KeyName = detectKeyName()
	}

	return cfg
}

// detectKeyName finds the first keypair in ~/.gather/keys/.
func detectKeyName() string {
	dir := keysDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	// Look for .key files first
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".key") {
			return strings.TrimSuffix(e.Name(), ".key")
		}
	}
	// Look for *-private.pem
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "-private.pem") {
			return strings.TrimSuffix(e.Name(), "-private.pem")
		}
	}
	return ""
}

func gatherDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".gather")
}

func keysDir() string {
	return filepath.Join(gatherDir(), "keys")
}
