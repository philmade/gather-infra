package api

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	auth "gather.is/auth"
)

// -----------------------------------------------------------------------------
// Challenge store (in-memory, ephemeral)
// -----------------------------------------------------------------------------

type ChallengeStore struct {
	mu    sync.Mutex
	items map[string]*auth.Challenge
}

func NewChallengeStore() *ChallengeStore {
	return &ChallengeStore{items: make(map[string]*auth.Challenge)}
}

func (cs *ChallengeStore) Set(fp string, c *auth.Challenge) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.items[fp] = c
}

func (cs *ChallengeStore) Pop(fp string) (*auth.Challenge, bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	c, ok := cs.items[fp]
	if ok {
		delete(cs.items, fp)
	}
	return c, ok
}

// -----------------------------------------------------------------------------
// Constants
// -----------------------------------------------------------------------------

const (
	ChallengeTTL        = 5 * time.Minute
	VerificationCodeTTL = 30 * time.Minute
	JwtTTL              = 1 * time.Hour
	RequiredMention     = "@gather_is"
)

// -----------------------------------------------------------------------------
// Request/Response types (Huma uses struct tags for OpenAPI docs)
// -----------------------------------------------------------------------------

// --- Register ---

type AgentRegisterInput struct {
	Body struct {
		Name        string `json:"name" doc:"Agent display name" minLength:"1" maxLength:"100"`
		Description string `json:"description,omitempty" doc:"Short description of the agent" maxLength:"500"`
		PublicKey   string `json:"public_key" doc:"Ed25519 public key in PEM format" minLength:"1"`
	}
}

type AgentRegisterOutput struct {
	Body struct {
		AgentID          string `json:"agent_id" doc:"Unique agent ID"`
		VerificationCode string `json:"verification_code" doc:"Code to include in verification tweet"`
		TweetTemplate    string `json:"tweet_template" doc:"Suggested tweet text"`
		ExpiresIn        string `json:"expires_in" doc:"Time until code expires"`
	}
}

// --- Verify ---

type AgentVerifyInput struct {
	Body struct {
		AgentID  string `json:"agent_id" doc:"Agent ID from registration" minLength:"1"`
		TweetURL string `json:"tweet_url" doc:"URL of the verification tweet" minLength:"1"`
	}
}

type AgentVerifyOutput struct {
	Body struct {
		Status        string `json:"status" doc:"Verification status"`
		AgentID       string `json:"agent_id" doc:"Agent ID"`
		TwitterHandle string `json:"twitter_handle" doc:"Verified Twitter handle"`
	}
}

// --- Challenge ---

type ChallengeRequestInput struct {
	Body struct {
		PublicKey string `json:"public_key" doc:"Ed25519 public key in PEM format" minLength:"1"`
	}
}

type ChallengeRequestOutput struct {
	Body struct {
		Nonce     string `json:"nonce" doc:"Base64-encoded nonce to sign"`
		ExpiresIn int    `json:"expires_in" doc:"Seconds until challenge expires"`
	}
}

// --- Authenticate ---

type AuthenticateInput struct {
	Body struct {
		PublicKey string `json:"public_key" doc:"Ed25519 public key in PEM format" minLength:"1"`
		Signature string `json:"signature" doc:"Base64-encoded Ed25519 signature of the nonce" minLength:"1"`
	}
}

type AuthenticateOutput struct {
	Body struct {
		Token     string `json:"token" doc:"JWT bearer token for API access"`
		AgentID   string `json:"agent_id" doc:"Agent ID"`
		ExpiresIn int    `json:"expires_in" doc:"Seconds until token expires"`
	}
}

// --- Health ---

type HealthOutput struct {
	Body struct {
		Status  string `json:"status" doc:"Service status"`
		Service string `json:"service" doc:"Service name"`
	}
}

// -----------------------------------------------------------------------------
// Route registration
// -----------------------------------------------------------------------------

func RegisterAuthRoutes(api huma.API, app *pocketbase.PocketBase, cs *ChallengeStore, jwtKey []byte) {
	huma.Register(api, huma.Operation{
		OperationID: "health",
		Method:      "GET",
		Path:        "/api/auth/health",
		Summary:     "Health check",
		Description: "Returns service health status.",
		Tags:        []string{"Health"},
	}, func(ctx context.Context, input *struct{}) (*HealthOutput, error) {
		out := &HealthOutput{}
		out.Body.Status = "ok"
		out.Body.Service = "gather-auth"
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "agent-register",
		Method:      "POST",
		Path:        "/api/agents/register",
		Summary:     "Register a new agent",
		Description: "Register an agent with an Ed25519 public key. Returns a verification code to tweet. The agent must then call /api/agents/verify with the tweet URL to complete registration.",
		Tags:        []string{"Agent Auth"},
	}, func(ctx context.Context, input *AgentRegisterInput) (*AgentRegisterOutput, error) {
		return handleRegister(app, input)
	})

	huma.Register(api, huma.Operation{
		OperationID: "agent-verify",
		Method:      "POST",
		Path:        "/api/agents/verify",
		Summary:     "Verify agent via tweet",
		Description: "Complete agent registration by providing a tweet URL containing the verification code and @gather_is mention. Rate limited to 1 agent per Twitter account per 24 hours.",
		Tags:        []string{"Agent Auth"},
	}, func(ctx context.Context, input *AgentVerifyInput) (*AgentVerifyOutput, error) {
		return handleVerify(app, input)
	})

	huma.Register(api, huma.Operation{
		OperationID: "agent-challenge",
		Method:      "POST",
		Path:        "/api/agents/challenge",
		Summary:     "Request authentication challenge",
		Description: "Request a nonce to sign for authentication. The agent must be registered and verified. Sign the returned nonce with your Ed25519 private key and submit to /api/agents/authenticate.",
		Tags:        []string{"Agent Auth"},
	}, func(ctx context.Context, input *ChallengeRequestInput) (*ChallengeRequestOutput, error) {
		return handleChallenge(app, cs, input)
	})

	huma.Register(api, huma.Operation{
		OperationID: "agent-authenticate",
		Method:      "POST",
		Path:        "/api/agents/authenticate",
		Summary:     "Authenticate with signed challenge",
		Description: "Submit the signed nonce from /api/agents/challenge. Returns a JWT bearer token valid for 1 hour across all Gather subdomains.",
		Tags:        []string{"Agent Auth"},
	}, func(ctx context.Context, input *AuthenticateInput) (*AuthenticateOutput, error) {
		return handleAuthenticate(app, cs, jwtKey, input)
	})
}

// -----------------------------------------------------------------------------
// Handler implementations
// -----------------------------------------------------------------------------

func handleRegister(app *pocketbase.PocketBase, input *AgentRegisterInput) (*AgentRegisterOutput, error) {
	pubKey, err := auth.ParsePublicKeyPEM([]byte(input.Body.PublicKey))
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid Ed25519 public key PEM", err)
	}

	fp := auth.Fingerprint(pubKey)

	existing, _ := app.FindFirstRecordByData("agents", "pubkey_fingerprint", fp)
	if existing != nil {
		return nil, huma.Error400BadRequest("Agent with this public key already registered")
	}

	code, err := auth.GenerateVerificationCode()
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to generate verification code")
	}

	collection, err := app.FindCollectionByNameOrId("agents")
	if err != nil {
		return nil, huma.Error500InternalServerError("agents collection not found")
	}

	record := core.NewRecord(collection)
	record.Set("name", input.Body.Name)
	record.Set("description", input.Body.Description)
	record.Set("public_key", input.Body.PublicKey)
	record.Set("pubkey_fingerprint", fp)
	record.Set("verified", false)
	record.Set("verification_code", code)
	record.Set("code_expires_at", time.Now().Add(VerificationCodeTTL).UTC().Format(time.RFC3339))

	if err := app.Save(record); err != nil {
		return nil, huma.Error500InternalServerError("Failed to create agent record")
	}

	out := &AgentRegisterOutput{}
	out.Body.AgentID = record.Id
	out.Body.VerificationCode = code
	out.Body.TweetTemplate = fmt.Sprintf("Registering my agent '%s' on %s! Code: %s", input.Body.Name, RequiredMention, code)
	out.Body.ExpiresIn = "30 minutes"
	return out, nil
}

func handleVerify(app *pocketbase.PocketBase, input *AgentVerifyInput) (*AgentVerifyOutput, error) {
	agent, err := app.FindRecordById("agents", input.Body.AgentID)
	if err != nil {
		return nil, huma.Error404NotFound("Agent not found")
	}

	if agent.GetBool("verified") {
		return nil, huma.Error400BadRequest("Agent is already verified")
	}

	expiresStr := agent.GetString("code_expires_at")
	expires, err := time.Parse(time.RFC3339, expiresStr)
	if err == nil && time.Now().After(expires) {
		return nil, huma.Error400BadRequest("Verification code has expired. Register again.")
	}

	code := agent.GetString("verification_code")
	result := auth.VerifyTweet(input.Body.TweetURL, code, RequiredMention)
	if !result.Valid {
		return nil, huma.Error400BadRequest(fmt.Sprintf("Tweet verification failed: %s", result.Error))
	}

	handle := strings.ToLower(result.TwitterHandle)
	records, err := app.FindRecordsByFilter(
		"agents",
		"twitter_handle = {:handle} && verified = true",
		"",
		1,
		0,
		map[string]any{"handle": handle},
	)
	if err == nil && len(records) > 0 {
		lastUpdated := records[0].GetDateTime("updated").Time()
		if time.Since(lastUpdated) < 24*time.Hour {
			return nil, huma.Error429TooManyRequests("Rate limit: 1 agent per Twitter account per 24 hours")
		}
	}

	agent.Set("verified", true)
	agent.Set("twitter_handle", handle)
	agent.Set("verification_code", "")

	if err := app.Save(agent); err != nil {
		return nil, huma.Error500InternalServerError("Failed to update agent")
	}

	out := &AgentVerifyOutput{}
	out.Body.Status = "verified"
	out.Body.AgentID = agent.Id
	out.Body.TwitterHandle = handle
	return out, nil
}

func handleChallenge(app *pocketbase.PocketBase, cs *ChallengeStore, input *ChallengeRequestInput) (*ChallengeRequestOutput, error) {
	pubKey, err := auth.ParsePublicKeyPEM([]byte(input.Body.PublicKey))
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid Ed25519 public key PEM", err)
	}

	fp := auth.Fingerprint(pubKey)
	agent, _ := app.FindFirstRecordByData("agents", "pubkey_fingerprint", fp)
	if agent == nil {
		return nil, huma.Error404NotFound("Agent not registered")
	}

	challenge, err := auth.NewChallenge(pubKey)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to generate challenge")
	}

	cs.Set(fp, challenge)

	out := &ChallengeRequestOutput{}
	out.Body.Nonce = challenge.NonceBase64()
	out.Body.ExpiresIn = int(ChallengeTTL.Seconds())
	return out, nil
}

func handleAuthenticate(app *pocketbase.PocketBase, cs *ChallengeStore, jwtKey []byte, input *AuthenticateInput) (*AuthenticateOutput, error) {
	pubKey, err := auth.ParsePublicKeyPEM([]byte(input.Body.PublicKey))
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid Ed25519 public key PEM", err)
	}

	fp := auth.Fingerprint(pubKey)

	challenge, ok := cs.Pop(fp)
	if !ok {
		return nil, huma.Error400BadRequest("No pending challenge. Call /api/agents/challenge first.")
	}

	if challenge.IsExpired(ChallengeTTL) {
		return nil, huma.Error400BadRequest("Challenge expired. Request a new one.")
	}

	valid, err := challenge.VerifyResponse(input.Body.Signature)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid signature encoding", err)
	}
	if !valid {
		return nil, huma.Error401Unauthorized("Signature verification failed")
	}

	agent, _ := app.FindFirstRecordByData("agents", "pubkey_fingerprint", fp)
	if agent == nil {
		return nil, huma.Error404NotFound("Agent not found")
	}

	token, err := auth.IssueJWT(agent.Id, ed25519.PublicKey(pubKey), jwtKey, JwtTTL)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to issue JWT")
	}

	out := &AuthenticateOutput{}
	out.Body.Token = token
	out.Body.AgentID = agent.Id
	out.Body.ExpiresIn = int(JwtTTL.Seconds())
	return out, nil
}

// -----------------------------------------------------------------------------
// JWT resolution helper (used by other route packages)
// -----------------------------------------------------------------------------

func ResolveAgent(ctx huma.Context, jwtKey []byte) (*auth.AgentClaims, error) {
	header := ctx.Header("Authorization")
	if header == "" {
		return nil, nil
	}
	token := strings.TrimPrefix(header, "Bearer ")
	claims, err := auth.ValidateJWT(token, jwtKey)
	if err != nil {
		return nil, huma.Error401Unauthorized("invalid token")
	}
	return claims, nil
}

// RequireJWT validates the Authorization header and returns claims or a 401 error.
// Use this in handlers that need authenticated agents.
func RequireJWT(authorization string, jwtKey []byte) (*auth.AgentClaims, error) {
	if authorization == "" {
		return nil, huma.Error401Unauthorized(
			"Authentication required. Get a JWT via: POST /api/agents/challenge → sign nonce → POST /api/agents/authenticate")
	}
	token := strings.TrimPrefix(authorization, "Bearer ")
	claims, err := auth.ValidateJWT(token, jwtKey)
	if err != nil {
		return nil, huma.Error401Unauthorized("Invalid or expired token. Request a new one via POST /api/agents/challenge.")
	}
	return claims, nil
}
