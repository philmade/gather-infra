package tools

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

// PlatformTool provides access to the Gather platform via the gather-mcp service.
type PlatformTool struct {
	mcpURL     string
	agentID    string
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	pubKeyB64  string // base64-encoded PEM of public key
	jwt        string // cached JWT from gather-auth
	client     *http.Client
}

// NewPlatformTool creates a platform tool from environment variables.
// Returns nil if GATHER_PRIVATE_KEY is not set (platform tools disabled).
func NewPlatformTool() *PlatformTool {
	mcpURL := os.Getenv("GATHER_MCP_URL")
	if mcpURL == "" {
		mcpURL = "http://gather-mcp:9200"
	}

	agentID := os.Getenv("GATHER_AGENT_ID")
	privKeyB64 := os.Getenv("GATHER_PRIVATE_KEY")
	pubKeyB64 := os.Getenv("GATHER_PUBLIC_KEY")

	if privKeyB64 == "" {
		return nil
	}

	privPEM, err := base64.StdEncoding.DecodeString(privKeyB64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "platform: decode private key: %v\n", err)
		return nil
	}

	privKey, err := parsePrivKey(privPEM)
	if err != nil {
		fmt.Fprintf(os.Stderr, "platform: parse private key: %v\n", err)
		return nil
	}

	var pubKey ed25519.PublicKey
	if pubKeyB64 != "" {
		pubPEM, err := base64.StdEncoding.DecodeString(pubKeyB64)
		if err == nil {
			pubKey, _ = parsePubKey(pubPEM)
		}
	}
	if pubKey == nil {
		// Derive from private key
		pubKey = privKey.Public().(ed25519.PublicKey)
	}

	// Encode public key as PEM then base64 for headers
	pubPEMBytes, err := marshalPubKey(pubKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "platform: marshal public key: %v\n", err)
		return nil
	}
	pubKeyHeaderB64 := base64.StdEncoding.EncodeToString(pubPEMBytes)

	return &PlatformTool{
		mcpURL:     mcpURL,
		agentID:    agentID,
		privateKey: privKey,
		publicKey:  pubKey,
		pubKeyB64:  pubKeyHeaderB64,
		client:     &http.Client{},
	}
}

// Search finds platform tools matching a query.
func (p *PlatformTool) Search(query, category string) (string, error) {
	u := p.mcpURL + "/tools/search?q=" + url.QueryEscape(query)
	if category != "" {
		u += "&category=" + url.QueryEscape(category)
	}

	resp, err := p.client.Get(u)
	if err != nil {
		return "", fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	return string(body), nil
}

// Call executes a platform tool by ID with the given params.
func (p *PlatformTool) Call(toolID string, params map[string]any) (string, error) {
	reqBody := map[string]any{
		"tool":   toolID,
		"params": params,
	}
	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", p.mcpURL+"/tools/execute", bytes.NewReader(bodyJSON))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Sign the request body with our Ed25519 key
	sig := ed25519.Sign(p.privateKey, bodyJSON)
	req.Header.Set("X-Agent-ID", p.agentID)
	req.Header.Set("X-Agent-Signature", base64.StdEncoding.EncodeToString(sig))
	req.Header.Set("X-Agent-Public-Key", p.pubKeyB64)

	// Include cached JWT if we have one
	if p.jwt != "" {
		req.Header.Set("X-Agent-JWT", p.jwt)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	// Cache JWT if returned in response headers
	if jwt := resp.Header.Get("X-JWT"); jwt != "" {
		p.jwt = jwt
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("execute failed (%d): %s", resp.StatusCode, string(respBody))
	}

	return string(respBody), nil
}

func parsePrivKey(pemData []byte) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	priv, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	edPriv, ok := priv.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not Ed25519")
	}
	return edPriv, nil
}

func parsePubKey(pemData []byte) (ed25519.PublicKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	edPub, ok := pub.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not Ed25519")
	}
	return edPub, nil
}

func marshalPubKey(pub ed25519.PublicKey) ([]byte, error) {
	pubBytes, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes}), nil
}
