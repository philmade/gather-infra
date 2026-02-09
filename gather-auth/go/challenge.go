package auth

// Challenge-response authentication + JWT issuance.
//
// Flow:
// 1. Agent connects, sends public key
// 2. Server returns random nonce (32 bytes)
// 3. Agent signs nonce with private key
// 4. Server verifies signature against stored public key
// 5. Server issues short-lived JWT (1 hour)

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Challenge holds a pending challenge for an agent.
type Challenge struct {
	Nonce     []byte
	PublicKey ed25519.PublicKey
	CreatedAt time.Time
}

// NewChallenge creates a 32-byte random nonce challenge.
func NewChallenge(publicKey ed25519.PublicKey) (*Challenge, error) {
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return &Challenge{
		Nonce:     nonce,
		PublicKey: publicKey,
		CreatedAt: time.Now(),
	}, nil
}

// NonceBase64 returns the nonce as a base64-encoded string.
func (c *Challenge) NonceBase64() string {
	return base64.StdEncoding.EncodeToString(c.Nonce)
}

// IsExpired returns true if the challenge is older than the given duration.
func (c *Challenge) IsExpired(maxAge time.Duration) bool {
	return time.Since(c.CreatedAt) > maxAge
}

// VerifyResponse checks that the agent correctly signed the nonce.
func (c *Challenge) VerifyResponse(signatureBase64 string) (bool, error) {
	sig, err := base64.StdEncoding.DecodeString(signatureBase64)
	if err != nil {
		return false, fmt.Errorf("decode signature: %w", err)
	}
	return Verify(c.PublicKey, c.Nonce, sig), nil
}

// AgentClaims are the JWT claims issued after successful challenge-response.
type AgentClaims struct {
	jwt.RegisteredClaims
	AgentID            string `json:"agent_id"`
	PublicKeyFingerprint string `json:"pubkey_fp"`
}

// IssueJWT creates a signed JWT for an authenticated agent.
// The signingKey should be a server-side secret (e.g., HMAC key).
func IssueJWT(agentID string, publicKey ed25519.PublicKey, signingKey []byte, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := AgentClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "gather.is",
			Subject:   agentID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		AgentID:            agentID,
		PublicKeyFingerprint: Fingerprint(publicKey),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(signingKey)
}

// ValidateJWT parses and validates a Gather agent JWT.
func ValidateJWT(tokenString string, signingKey []byte) (*AgentClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &AgentClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return signingKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse jwt: %w", err)
	}

	claims, ok := token.Claims.(*AgentClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}
