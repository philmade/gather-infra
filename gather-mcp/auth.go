package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// AuthManager handles claw authentication — validates Ed25519 signatures on
// incoming requests and obtains/caches JWTs from gather-auth.
type AuthManager struct {
	authURL string

	mu       sync.RWMutex
	jwtCache map[string]cachedJWT // agentID → cached JWT
}

type cachedJWT struct {
	Token     string
	ExpiresAt time.Time
}

func NewAuthManager(authURL string) *AuthManager {
	return &AuthManager{
		authURL:  authURL,
		jwtCache: make(map[string]cachedJWT),
	}
}

// Authenticate extracts agent identity from the request.
// For claw requests: X-Agent-ID header + X-Agent-Signature (Ed25519 sig of request body).
// For external MCP clients: Authorization: Bearer <jwt> header (pass-through).
// Returns (agentID, jwt, error).
func (a *AuthManager) Authenticate(r *http.Request, body []byte) (string, string, error) {
	// Check for direct JWT pass-through (external MCP clients)
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		return "", token, nil
	}

	// Claw-style auth: agent signs the request
	agentID := r.Header.Get("X-Agent-ID")
	sig := r.Header.Get("X-Agent-Signature")
	pubKeyB64 := r.Header.Get("X-Agent-Public-Key")

	if agentID == "" || sig == "" || pubKeyB64 == "" {
		return "", "", fmt.Errorf("missing auth headers (need X-Agent-ID, X-Agent-Signature, X-Agent-Public-Key)")
	}

	// Decode public key
	pubKeyPEM, err := base64.StdEncoding.DecodeString(pubKeyB64)
	if err != nil {
		return "", "", fmt.Errorf("decode public key: %w", err)
	}

	pubKey, err := parsePublicKeyPEM(pubKeyPEM)
	if err != nil {
		return "", "", fmt.Errorf("parse public key: %w", err)
	}

	// Verify signature over request body
	sigBytes, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return "", "", fmt.Errorf("decode signature: %w", err)
	}

	if !ed25519.Verify(pubKey, body, sigBytes) {
		return "", "", fmt.Errorf("signature verification failed")
	}

	// Get or refresh JWT
	jwt, err := a.getJWT(agentID, pubKeyPEM)
	if err != nil {
		return "", "", fmt.Errorf("obtain JWT: %w", err)
	}

	return agentID, jwt, nil
}

// getJWT returns a cached JWT or obtains a new one via challenge-response.
func (a *AuthManager) getJWT(agentID string, pubKeyPEM []byte) (string, error) {
	a.mu.RLock()
	cached, ok := a.jwtCache[agentID]
	a.mu.RUnlock()

	if ok && time.Now().Before(cached.ExpiresAt.Add(-5*time.Minute)) {
		return cached.Token, nil
	}

	// Need fresh JWT — do challenge-response with gather-auth.
	// This is a server-to-server call, not subject to the same rate limits.
	token, expiresIn, err := a.challengeResponse(pubKeyPEM)
	if err != nil {
		return "", err
	}

	a.mu.Lock()
	a.jwtCache[agentID] = cachedJWT{
		Token:     token,
		ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second),
	}
	a.mu.Unlock()

	log.Printf("Cached JWT for agent %s (expires in %ds)", agentID, expiresIn)
	return token, nil
}

// challengeResponse performs the full challenge-response flow against gather-auth.
// Note: We don't actually sign — gather-auth needs the PEM to look up the agent,
// and we pass the JWT back. For server-to-server, we use the stored public key
// to authenticate. The MCP service is trusted (same Docker network).
//
// Actually, the MCP service needs the private key to sign challenges.
// But the MCP service doesn't HAVE the private key — claws do.
// So instead, the claw provides a pre-signed challenge OR the MCP service
// caches JWTs that the claw initially provides.
//
// Simplest approach: The first time a claw calls us, it must include a valid JWT
// OR we do the full flow using the claw's signature.
// Let's keep it simple: the claw includes its public key PEM in headers,
// we do challenge-response on its behalf.
//
// Wait — we can't sign without the private key. The claw needs to provide a JWT.
// OR: the claw signs a challenge we give it.
//
// Simplest correct approach:
// 1. Claw calls /tools/execute with X-Agent-ID + X-Agent-Public-Key + X-Agent-Signature
// 2. We call gather-auth challenge endpoint with the public key
// 3. We get a nonce back
// 4. We can't sign it — we don't have the private key
//
// So the auth flow must be:
// Option A: Claw pre-authenticates with gather-auth and passes JWT directly
// Option B: Claw signs a nonce we provide (two-step)
//
// Going with Option A — it's simpler and the claw already knows how to get JWTs.
// The claw includes "Authorization: Bearer <jwt>" on every request.
// If it's expired, the claw re-authenticates directly with gather-auth.
//
// For claws, we just validate the signature over the body to confirm identity,
// then use their JWT for downstream calls.
func (a *AuthManager) challengeResponse(pubKeyPEM []byte) (string, int, error) {
	// This won't work without the private key. See comment above.
	// Leaving as a no-op — the real flow uses JWT pass-through.
	return "", 0, fmt.Errorf("challenge-response requires private key; use JWT pass-through instead")
}

// AuthenticateRequest is a simpler auth flow:
// - If Authorization header present → extract JWT, use it for downstream
// - If X-Agent-ID + X-Agent-Signature → verify body signature, require JWT in X-Agent-JWT
// Returns (jwt for downstream use, error).
func (a *AuthManager) AuthenticateRequest(r *http.Request, body []byte) (string, error) {
	// Direct JWT (external clients or claws with JWT)
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer "), nil
	}

	// Claw-style: verify signature + use provided JWT
	agentID := r.Header.Get("X-Agent-ID")
	sig := r.Header.Get("X-Agent-Signature")
	pubKeyB64 := r.Header.Get("X-Agent-Public-Key")
	jwt := r.Header.Get("X-Agent-JWT")

	if agentID == "" {
		return "", fmt.Errorf("missing authentication: provide Authorization header or X-Agent-ID")
	}

	// If claw provides a JWT, verify identity via signature and use that JWT
	if jwt != "" && sig != "" && pubKeyB64 != "" {
		pubKeyPEM, err := base64.StdEncoding.DecodeString(pubKeyB64)
		if err != nil {
			return "", fmt.Errorf("decode public key: %w", err)
		}
		pubKey, err := parsePublicKeyPEM(pubKeyPEM)
		if err != nil {
			return "", fmt.Errorf("parse public key: %w", err)
		}
		sigBytes, err := base64.StdEncoding.DecodeString(sig)
		if err != nil {
			return "", fmt.Errorf("decode signature: %w", err)
		}
		if !ed25519.Verify(pubKey, body, sigBytes) {
			return "", fmt.Errorf("signature verification failed")
		}

		// Cache the JWT
		a.mu.Lock()
		a.jwtCache[agentID] = cachedJWT{
			Token:     jwt,
			ExpiresAt: time.Now().Add(50 * time.Minute), // assume ~1hr validity
		}
		a.mu.Unlock()
		return jwt, nil
	}

	// Check cache for previously provided JWT
	a.mu.RLock()
	cached, ok := a.jwtCache[agentID]
	a.mu.RUnlock()
	if ok && time.Now().Before(cached.ExpiresAt) {
		return cached.Token, nil
	}

	return "", fmt.Errorf("no JWT available for agent %s: provide Authorization header or X-Agent-JWT", agentID)
}

// ForwardAuth creates an http.Request with the JWT attached as Authorization header.
func ForwardAuth(req *http.Request, jwt string) {
	if jwt != "" {
		req.Header.Set("Authorization", "Bearer "+jwt)
	}
}

func parsePublicKeyPEM(pemData []byte) (ed25519.PublicKey, error) {
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

// readBody reads and returns the request body, then replaces the body reader
// so downstream handlers can read it again.
func readBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	return body, nil
}
