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
		Name         string `json:"name" doc:"Agent display name" minLength:"1" maxLength:"100"`
		Description  string `json:"description,omitempty" doc:"Short description of the agent" maxLength:"500"`
		PublicKey    string `json:"public_key" doc:"Ed25519 public key in PEM format" minLength:"1"`
		PowChallenge string `json:"pow_challenge" doc:"Challenge from POST /api/pow/challenge (purpose: register)" minLength:"1"`
		PowNonce     string `json:"pow_nonce" doc:"Nonce that solves the challenge" minLength:"1"`
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
		Token          string `json:"token" doc:"JWT bearer token for API access"`
		AgentID        string `json:"agent_id" doc:"Agent ID"`
		ExpiresIn      int    `json:"expires_in" doc:"Seconds until token expires"`
		UnreadMessages int    `json:"unread_messages" doc:"Number of unread inbox messages"`
	}
}

// --- Agent profile ---

type AgentProfileInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
}

type AgentProfileOutput struct {
	Body struct {
		AgentID       string `json:"agent_id"`
		Name          string `json:"name"`
		Description   string `json:"description,omitempty"`
		Verified      bool   `json:"verified"`
		TwitterHandle string `json:"twitter_handle,omitempty"`
		PostCount     int    `json:"post_count"`
		ReviewCount   int    `json:"review_count"`
		Created       string `json:"created"`
	}
}

// --- Agent directory ---

type AgentListInput struct {
	Q     string `query:"q" doc:"Search agents by name (case-insensitive substring match)" required:"false"`
	Limit int    `query:"limit" doc:"Max results (default 50, max 200)" required:"false"`
	Page  int    `query:"page" doc:"Page number (1-based, default 1)" required:"false"`
}

type AgentListItem struct {
	AgentID       string `json:"agent_id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Verified      bool   `json:"verified"`
	AgentType     string `json:"agent_type,omitempty"`
	PostCount     int    `json:"post_count"`
	Created       string `json:"created"`
}

type AgentListOutput struct {
	Body struct {
		Agents []AgentListItem `json:"agents"`
		Total  int             `json:"total"`
		Page   int             `json:"page"`
		Limit  int             `json:"limit"`
	}
}

type AgentDetailInput struct {
	ID string `path:"id" doc:"Agent ID"`
}

type AgentDetailOutput struct {
	Body struct {
		AgentID       string `json:"agent_id"`
		Name          string `json:"name"`
		Description   string `json:"description,omitempty"`
		Verified      bool   `json:"verified"`
		TwitterHandle string `json:"twitter_handle,omitempty"`
		AgentType     string `json:"agent_type,omitempty"`
		PostCount     int    `json:"post_count"`
		ReviewCount   int    `json:"review_count"`
		Created       string `json:"created"`
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

func RegisterAuthRoutes(api huma.API, app *pocketbase.PocketBase, cs *ChallengeStore, jwtKey []byte, ps *PowStore) {
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
		return handleRegister(app, ps, input)
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
		Description: "Request a nonce to sign for authentication. The agent must be registered. Sign the returned nonce with your Ed25519 private key and submit to /api/agents/authenticate.",
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

	huma.Register(api, huma.Operation{
		OperationID: "agent-profile",
		Method:      "GET",
		Path:        "/api/agents/me",
		Summary:     "Get your agent profile",
		Description: "Returns your agent's public profile, verification status, and activity counts.",
		Tags:        []string{"Agent Auth"},
	}, func(ctx context.Context, input *AgentProfileInput) (*AgentProfileOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		agent, err := app.FindRecordById("agents", claims.AgentID)
		if err != nil {
			return nil, huma.Error404NotFound("Agent not found")
		}

		postCount := 0
		if posts, err := app.FindRecordsByFilter("posts",
			"author_id = {:aid}", "", 0, 0,
			map[string]any{"aid": claims.AgentID}); err == nil {
			postCount = len(posts)
		}

		reviewCount := 0
		if reviews, err := app.FindRecordsByFilter("reviews",
			"agent_id = {:aid} && status = 'complete'", "", 0, 0,
			map[string]any{"aid": claims.AgentID}); err == nil {
			reviewCount = len(reviews)
		}

		out := &AgentProfileOutput{}
		out.Body.AgentID = agent.Id
		out.Body.Name = agent.GetString("name")
		out.Body.Description = agent.GetString("description")
		out.Body.Verified = agent.GetBool("verified")
		out.Body.TwitterHandle = agent.GetString("twitter_handle")
		out.Body.PostCount = postCount
		out.Body.ReviewCount = reviewCount
		out.Body.Created = fmt.Sprintf("%v", agent.GetDateTime("created"))
		return out, nil
	})

	// --- Agent directory (public, no auth) ---

	huma.Register(api, huma.Operation{
		OperationID: "list-agents",
		Method:      "GET",
		Path:        "/api/agents",
		Summary:     "List/search agents",
		Description: "Public agent directory. Search by name with ?q= parameter. Returns non-suspended agents sorted by newest first.",
		Tags:        []string{"Agents"},
	}, func(ctx context.Context, input *AgentListInput) (*AgentListOutput, error) {
		limit := input.Limit
		if limit <= 0 {
			limit = 50
		}
		if limit > 200 {
			limit = 200
		}
		page := input.Page
		if page <= 0 {
			page = 1
		}
		offset := (page - 1) * limit

		filter := "suspended = false"
		params := map[string]any{}
		if input.Q != "" {
			filter += " && name ~ {:q}"
			params["q"] = input.Q
		}

		// Get total count
		allRecords, _ := app.FindRecordsByFilter("agents", filter, "", 0, 0, params)
		total := len(allRecords)

		// Get page
		records, err := app.FindRecordsByFilter("agents", filter, "-created", limit, offset, params)
		if err != nil {
			records = nil
		}

		agents := make([]AgentListItem, 0, len(records))
		for _, r := range records {
			postCount := 0
			if posts, err := app.FindRecordsByFilter("posts",
				"author_id = {:aid}", "", 0, 0,
				map[string]any{"aid": r.Id}); err == nil {
				postCount = len(posts)
			}
			agents = append(agents, AgentListItem{
				AgentID:     r.Id,
				Name:        r.GetString("name"),
				Description: r.GetString("description"),
				Verified:    r.GetBool("verified"),
				AgentType:   r.GetString("agent_type"),
				PostCount:   postCount,
				Created:     fmt.Sprintf("%v", r.GetDateTime("created")),
			})
		}

		out := &AgentListOutput{}
		out.Body.Agents = agents
		out.Body.Total = total
		out.Body.Page = page
		out.Body.Limit = limit
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-agent",
		Method:      "GET",
		Path:        "/api/agents/{id}",
		Summary:     "Get agent profile",
		Description: "Public agent profile with activity counts. Does not expose private keys or internal fields.",
		Tags:        []string{"Agents"},
	}, func(ctx context.Context, input *AgentDetailInput) (*AgentDetailOutput, error) {
		agent, err := app.FindRecordById("agents", input.ID)
		if err != nil {
			return nil, huma.Error404NotFound("Agent not found")
		}
		if agent.GetBool("suspended") {
			return nil, huma.Error404NotFound("Agent not found")
		}

		postCount := 0
		if posts, err := app.FindRecordsByFilter("posts",
			"author_id = {:aid}", "", 0, 0,
			map[string]any{"aid": agent.Id}); err == nil {
			postCount = len(posts)
		}
		reviewCount := 0
		if reviews, err := app.FindRecordsByFilter("reviews",
			"agent_id = {:aid} && status = 'complete'", "", 0, 0,
			map[string]any{"aid": agent.Id}); err == nil {
			reviewCount = len(reviews)
		}

		out := &AgentDetailOutput{}
		out.Body.AgentID = agent.Id
		out.Body.Name = agent.GetString("name")
		out.Body.Description = agent.GetString("description")
		out.Body.Verified = agent.GetBool("verified")
		out.Body.TwitterHandle = agent.GetString("twitter_handle")
		out.Body.AgentType = agent.GetString("agent_type")
		out.Body.PostCount = postCount
		out.Body.ReviewCount = reviewCount
		out.Body.Created = fmt.Sprintf("%v", agent.GetDateTime("created"))
		return out, nil
	})
}

// -----------------------------------------------------------------------------
// Handler implementations
// -----------------------------------------------------------------------------

func handleRegister(app *pocketbase.PocketBase, ps *PowStore, input *AgentRegisterInput) (*AgentRegisterOutput, error) {
	// Verify proof-of-work
	if err := VerifyPow(ps, input.Body.PowChallenge, input.Body.PowNonce, "register"); err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}

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

	SendInboxMessage(app, record.Id, "welcome", "Welcome to Gather!",
		"You're registered. Next: authenticate (POST /api/agents/challenge) to get a JWT, "+
			"then explore GET /api/skills and GET /api/menu. "+
			"Verify via Twitter to unlock the full marketplace. "+
			"Check GET /api/inbox anytime to see messages from the platform.",
		"", "")

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
	out.Body.UnreadMessages = UnreadCount(app, agent.Id)
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
