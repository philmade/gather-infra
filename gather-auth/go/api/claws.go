package api

import (
	"context"
	"os"
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
	Subdomain    string `json:"subdomain,omitempty"`
	ContainerID  string `json:"container_id,omitempty"`
	URL          string `json:"url,omitempty"`
	Port         int    `json:"port,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	Created      string `json:"created"`
}

func recordToClawDeployment(r *core.Record) ClawDeployment {
	return ClawDeployment{
		ID:           r.Id,
		Name:         r.GetString("name"),
		Status:       r.GetString("status"),
		Instructions: r.GetString("instructions"),
		GithubRepo:   r.GetString("github_repo"),
		ClawType:     r.GetString("claw_type"),
		UserID:       r.GetString("user_id"),
		Subdomain:    r.GetString("subdomain"),
		ContainerID:  r.GetString("container_id"),
		URL:          r.GetString("url"),
		Port:         int(r.GetFloat("port")),
		ErrorMessage: r.GetString("error_message"),
		Created:      r.GetString("created"),
	}
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

// Provisioner-internal types (host-side provisioner calls these)

type PendingClawsInput struct {
	ProvisionerKey string `header:"X-Provisioner-Key" doc:"Provisioner shared secret" required:"true"`
}

type ProvisionResultInput struct {
	ProvisionerKey string `header:"X-Provisioner-Key" doc:"Provisioner shared secret" required:"true"`
	ID             string `path:"id" doc:"Deployment ID"`
	Body           struct {
		Status       string `json:"status" doc:"New status: running or failed" enum:"running,failed"`
		ContainerID  string `json:"container_id,omitempty" doc:"Docker container name/ID"`
		ErrorMessage string `json:"error_message,omitempty" doc:"Error message if failed"`
	}
}

type ProvisionResultOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

type DeleteClawInput struct {
	Authorization string `header:"Authorization" doc:"Bearer PocketBase auth token" required:"true"`
	ID            string `path:"id" doc:"Deployment ID"`
}

type DeleteClawOutput struct {
	Body struct {
		OK bool `json:"ok"`
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
		Description: "Queue a new PicoClaw agent deployment. The hook transitions it to provisioning automatically.",
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

		// Hook fires async — record may still show "queued" here.
		// Client should poll GET /api/claws/{id} to see status progression.
		out := &DeployClawOutput{}
		out.Body = recordToClawDeployment(record)
		return out, nil
	})

	// GET /api/claws/pending — list claws awaiting provisioning (internal)
	huma.Register(api, huma.Operation{
		OperationID: "list-pending-claws",
		Method:      "GET",
		Path:        "/api/claws/pending",
		Summary:     "List claws awaiting provisioning",
		Description: "Internal endpoint for the host-side provisioner. Requires X-Provisioner-Key header.",
		Tags:        []string{"Claws"},
	}, func(ctx context.Context, input *PendingClawsInput) (*ListClawsOutput, error) {
		expected := os.Getenv("CLAW_PROVISIONER_KEY")
		if expected == "" || input.ProvisionerKey != expected {
			return nil, huma.Error401Unauthorized("Invalid provisioner key")
		}

		records, err := app.FindRecordsByFilter("claw_deployments",
			"status = 'provisioning'", "-created", 50, 0, nil)
		if err != nil {
			records = nil
		}

		out := &ListClawsOutput{}
		for _, r := range records {
			out.Body.Claws = append(out.Body.Claws, recordToClawDeployment(r))
		}
		out.Body.Total = len(out.Body.Claws)
		return out, nil
	})

	// POST /api/claws/{id}/provision-result — report provisioning outcome (internal)
	huma.Register(api, huma.Operation{
		OperationID: "provision-result",
		Method:      "POST",
		Path:        "/api/claws/{id}/provision-result",
		Summary:     "Report claw provisioning result",
		Description: "Internal endpoint. Host-side provisioner reports success (running) or failure.",
		Tags:        []string{"Claws"},
	}, func(ctx context.Context, input *ProvisionResultInput) (*ProvisionResultOutput, error) {
		expected := os.Getenv("CLAW_PROVISIONER_KEY")
		if expected == "" || input.ProvisionerKey != expected {
			return nil, huma.Error401Unauthorized("Invalid provisioner key")
		}

		if input.Body.Status != "running" && input.Body.Status != "failed" {
			return nil, huma.Error422UnprocessableEntity("Status must be 'running' or 'failed'")
		}

		record, err := app.FindRecordById("claw_deployments", input.ID)
		if err != nil {
			return nil, huma.Error404NotFound("Deployment not found")
		}

		record.Set("status", input.Body.Status)
		if input.Body.ContainerID != "" {
			record.Set("container_id", input.Body.ContainerID)
		}
		if input.Body.ErrorMessage != "" {
			record.Set("error_message", input.Body.ErrorMessage)
		}

		if err := app.Save(record); err != nil {
			return nil, huma.Error500InternalServerError("Failed to update deployment")
		}

		out := &ProvisionResultOutput{}
		out.Body.OK = true
		return out, nil
	})

	// DELETE /api/claws/{id} — delete a deployment
	huma.Register(api, huma.Operation{
		OperationID: "delete-claw",
		Method:      "DELETE",
		Path:        "/api/claws/{id}",
		Summary:     "Delete a Claw deployment",
		Description: "Delete a claw deployment. Only the owning user can delete.",
		Tags:        []string{"Claws"},
	}, func(ctx context.Context, input *DeleteClawInput) (*DeleteClawOutput, error) {
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

		if err := app.Delete(record); err != nil {
			return nil, huma.Error500InternalServerError("Failed to delete deployment")
		}

		out := &DeleteClawOutput{}
		out.Body.OK = true
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
		out.Body = recordToClawDeployment(record)
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
			out.Body.Claws = append(out.Body.Claws, recordToClawDeployment(r))
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
