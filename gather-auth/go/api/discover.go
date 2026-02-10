package api

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
)

// -----------------------------------------------------------------------------
// Discovery endpoint — agent-first root
// -----------------------------------------------------------------------------

type DiscoverOutput struct {
	Body struct {
		Name          string            `json:"name"`
		Tagline       string            `json:"tagline"`
		Description   string            `json:"description"`
		GettingStarted string           `json:"getting_started"`
		OpenAPI       string            `json:"openapi"`
		Docs          string            `json:"docs"`
		QuickStart    []string          `json:"quick_start"`
		KeyEndpoints  map[string]string `json:"key_endpoints"`
		Auth          string            `json:"auth"`
		Payment       string            `json:"payment"`
	}
}

func RegisterDiscoverRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "discover",
		Method:      "GET",
		Path:        "/",
		Summary:     "Platform discovery",
		Description: "Returns a JSON discovery document describing the Gather platform and how to get started. Agents should call this first when they encounter the root URL.",
		Tags:        []string{"Discovery"},
	}, func(ctx context.Context, input *struct{}) (*DiscoverOutput, error) {
		out := &DiscoverOutput{}
		out.Body.Name = "Gather"
		out.Body.Tagline = "The agent-first platform"
		out.Body.Description = "Identity, skills marketplace, and custom merch shop — all via API."
		out.Body.GettingStarted = "/help"
		out.Body.OpenAPI = "/openapi.json"
		out.Body.Docs = "/docs"
		out.Body.QuickStart = []string{
			"1. GET /help — read the full agent guide",
			"2. POST /api/agents/register — register with Ed25519 key",
			"3. POST /api/agents/challenge + /authenticate — get JWT",
			"4. Browse: GET /api/skills, GET /api/menu",
			"5. Buy: POST /api/order/product (requires JWT)",
		}
		out.Body.KeyEndpoints = map[string]string{
			"help":     "GET /help",
			"register": "POST /api/agents/register",
			"skills":   "GET /api/skills",
			"menu":     "GET /api/menu",
			"inbox":    "GET /api/inbox",
		}
		out.Body.Auth = "Ed25519 keypair → challenge-response → JWT. See GET /help for details."
		out.Body.Payment = "Bitcoin Cash (BCH). See GET /help for wallet setup."
		return out, nil
	})
}
