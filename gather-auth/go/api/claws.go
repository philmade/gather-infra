package api

import (
	"context"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// -----------------------------------------------------------------------------
// Request / Response types
// -----------------------------------------------------------------------------

type ClawDeployment struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	Instructions string `json:"instructions,omitempty"`
	GithubRepo   string `json:"github_repo,omitempty"`
	ClawType     string `json:"claw_type"`
	UserID       string `json:"user_id"`
	Created      string `json:"created"`
}

type DeployClawInput struct {
	Authorization string `header:"Authorization" doc:"Bearer PocketBase auth token" required:"true"`
	Body          struct {
		Name         string `json:"name" doc:"Claw name (e.g. ResearchClaw)" minLength:"1" maxLength:"50"`
		Instructions string `json:"instructions,omitempty" doc:"Initial instructions for the claw" maxLength:"2000"`
		GithubRepo   string `json:"github_repo,omitempty" doc:"GitHub repo to connect (e.g. acme/repo)" maxLength:"200"`
		ClawType     string `json:"claw_type,omitempty" doc:"Agent type: picoclaw (default)" maxLength:"50"`
	}
}

type DeployClawOutput struct {
	Body ClawDeployment
}

type GetClawInput struct {
	Authorization string `header:"Authorization" doc:"Bearer PocketBase auth token" required:"true"`
	ID            string `path:"id" doc:"Deployment ID"`
}

type GetClawOutput struct {
	Body ClawDeployment
}

type ListClawsInput struct {
	Authorization string `header:"Authorization" doc:"Bearer PocketBase auth token" required:"true"`
}

type ListClawsOutput struct {
	Body struct {
		Claws []ClawDeployment `json:"claws"`
		Total int              `json:"total"`
	}
}

// -----------------------------------------------------------------------------
// Route registration
// -----------------------------------------------------------------------------

func RegisterClawRoutes(api huma.API, app *pocketbase.PocketBase) {
	// POST /api/claws — deploy a new claw
	huma.Register(api, huma.Operation{
		OperationID: "deploy-claw",
		Method:      "POST",
		Path:        "/api/claws",
		Summary:     "Deploy a Claw agent",
		Description: "Queue a new PicoClaw agent deployment for the authenticated user's workspace.",
		Tags:        []string{"Claws"},
	}, func(ctx context.Context, input *DeployClawInput) (*DeployClawOutput, error) {
		userID, err := extractPBUserID(app, input.Authorization)
		if err != nil {
			return nil, huma.Error401Unauthorized("Authentication required")
		}

		name := strings.TrimSpace(input.Body.Name)
		if name == "" {
			return nil, huma.Error422UnprocessableEntity("Name is required")
		}

		clawType := input.Body.ClawType
		if clawType == "" {
			clawType = "picoclaw"
		}

		col, err := app.FindCollectionByNameOrId("claw_deployments")
		if err != nil {
			return nil, huma.Error500InternalServerError("claw_deployments collection not found")
		}

		record := core.NewRecord(col)
		record.Set("name", name)
		record.Set("status", "queued")
		record.Set("instructions", strings.TrimSpace(input.Body.Instructions))
		record.Set("github_repo", strings.TrimSpace(input.Body.GithubRepo))
		record.Set("claw_type", clawType)
		record.Set("user_id", userID)

		if err := app.Save(record); err != nil {
			return nil, huma.Error500InternalServerError("Failed to create deployment")
		}

		out := &DeployClawOutput{}
		out.Body = ClawDeployment{
			ID:           record.Id,
			Name:         name,
			Status:       "queued",
			Instructions: strings.TrimSpace(input.Body.Instructions),
			GithubRepo:   strings.TrimSpace(input.Body.GithubRepo),
			ClawType:     clawType,
			UserID:       userID,
			Created:      record.GetString("created"),
		}
		return out, nil
	})

	// GET /api/claws/{id} — get deployment status
	huma.Register(api, huma.Operation{
		OperationID: "get-claw",
		Method:      "GET",
		Path:        "/api/claws/{id}",
		Summary:     "Get Claw deployment status",
		Description: "Check the status of a claw deployment.",
		Tags:        []string{"Claws"},
	}, func(ctx context.Context, input *GetClawInput) (*GetClawOutput, error) {
		userID, err := extractPBUserID(app, input.Authorization)
		if err != nil {
			return nil, huma.Error401Unauthorized("Authentication required")
		}

		record, err := app.FindRecordById("claw_deployments", input.ID)
		if err != nil {
			return nil, huma.Error404NotFound("Deployment not found")
		}

		if record.GetString("user_id") != userID {
			return nil, huma.Error404NotFound("Deployment not found")
		}

		out := &GetClawOutput{}
		out.Body = ClawDeployment{
			ID:           record.Id,
			Name:         record.GetString("name"),
			Status:       record.GetString("status"),
			Instructions: record.GetString("instructions"),
			GithubRepo:   record.GetString("github_repo"),
			ClawType:     record.GetString("claw_type"),
			UserID:       record.GetString("user_id"),
			Created:      record.GetString("created"),
		}
		return out, nil
	})

	// GET /api/claws — list user's claws
	huma.Register(api, huma.Operation{
		OperationID: "list-claws",
		Method:      "GET",
		Path:        "/api/claws",
		Summary:     "List deployed Claws",
		Description: "List all claw deployments for the authenticated user.",
		Tags:        []string{"Claws"},
	}, func(ctx context.Context, input *ListClawsInput) (*ListClawsOutput, error) {
		userID, err := extractPBUserID(app, input.Authorization)
		if err != nil {
			return nil, huma.Error401Unauthorized("Authentication required")
		}

		records, err := app.FindRecordsByFilter("claw_deployments",
			"user_id = {:uid}", "-created", 50, 0,
			map[string]any{"uid": userID})
		if err != nil {
			records = nil
		}

		out := &ListClawsOutput{}
		for _, r := range records {
			out.Body.Claws = append(out.Body.Claws, ClawDeployment{
				ID:           r.Id,
				Name:         r.GetString("name"),
				Status:       r.GetString("status"),
				Instructions: r.GetString("instructions"),
				GithubRepo:   r.GetString("github_repo"),
				ClawType:     r.GetString("claw_type"),
				UserID:       r.GetString("user_id"),
				Created:      r.GetString("created"),
			})
		}
		out.Body.Total = len(out.Body.Claws)
		return out, nil
	})
}

// extractPBUserID parses a PocketBase auth token and returns the user ID.
func extractPBUserID(app *pocketbase.PocketBase, authHeader string) (string, error) {
	token := strings.TrimPrefix(authHeader, "Bearer ")
	token = strings.TrimPrefix(token, "bearer ")
	token = strings.TrimSpace(token)
	if token == "" {
		return "", huma.Error401Unauthorized("Missing auth token")
	}

	record, err := app.FindAuthRecordByToken(token, "auth")
	if err != nil {
		return "", err
	}
	return record.Id, nil
}
