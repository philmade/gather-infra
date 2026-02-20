package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// -----------------------------------------------------------------------------
// Request / Response types
// -----------------------------------------------------------------------------

type ClawDeployment struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Status               string `json:"status"`
	Instructions         string `json:"instructions,omitempty"`
	GithubRepo           string `json:"github_repo,omitempty"`
	ClawType             string `json:"claw_type"`
	UserID               string `json:"user_id"`
	Subdomain            string `json:"subdomain,omitempty"`
	ContainerID          string `json:"container_id,omitempty"`
	URL                  string `json:"url,omitempty"`
	Port                 int    `json:"port,omitempty"`
	ErrorMessage         string `json:"error_message,omitempty"`
	IsPublic             bool   `json:"is_public"`
	HeartbeatInterval    int    `json:"heartbeat_interval"`
	HeartbeatInstruction string `json:"heartbeat_instruction,omitempty"`
	Paid                 bool   `json:"paid"`
	TrialEndsAt          string `json:"trial_ends_at,omitempty"`
	StripeSessionID      string `json:"stripe_session_id,omitempty"`
	Created              string `json:"created"`
}

func recordToClawDeployment(r *core.Record) ClawDeployment {
	return ClawDeployment{
		ID:                   r.Id,
		Name:                 r.GetString("name"),
		Status:               r.GetString("status"),
		Instructions:         r.GetString("instructions"),
		GithubRepo:           r.GetString("github_repo"),
		ClawType:             r.GetString("claw_type"),
		UserID:               r.GetString("user_id"),
		Subdomain:            r.GetString("subdomain"),
		ContainerID:          r.GetString("container_id"),
		URL:                  r.GetString("url"),
		Port:                 int(r.GetFloat("port")),
		ErrorMessage:         r.GetString("error_message"),
		IsPublic:             r.GetBool("is_public"),
		HeartbeatInterval:    int(r.GetFloat("heartbeat_interval")),
		HeartbeatInstruction: r.GetString("heartbeat_instruction"),
		Paid:                 r.GetBool("paid"),
		TrialEndsAt:          r.GetString("trial_ends_at"),
		StripeSessionID:      r.GetString("stripe_session_id"),
		Created:              r.GetString("created"),
	}
}

type DeployClawInput struct {
	Authorization string `header:"Authorization" doc:"Bearer PocketBase auth token" required:"true"`
	Body          struct {
		Name         string `json:"name" doc:"Claw name (e.g. ResearchClaw)" minLength:"1" maxLength:"50"`
		Instructions string `json:"instructions,omitempty" doc:"Initial instructions for the claw" maxLength:"2000"`
		GithubRepo   string `json:"github_repo,omitempty" doc:"GitHub repo to connect (e.g. acme/repo)" maxLength:"200"`
		ClawType     string `json:"claw_type,omitempty" doc:"Tier: lite (default), pro, max" maxLength:"50"`
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

type UpdateClawSettingsInput struct {
	Authorization string `header:"Authorization" doc:"Bearer PocketBase auth token" required:"true"`
	ID            string `path:"id" doc:"Deployment ID"`
	Body          struct {
		IsPublic             *bool   `json:"is_public,omitempty" doc:"Whether subdomain page is public"`
		HeartbeatInterval    *int    `json:"heartbeat_interval,omitempty" doc:"Minutes between heartbeats (0=off, 15, 30, 60, 360, 1440)"`
		HeartbeatInstruction *string `json:"heartbeat_instruction,omitempty" doc:"Instruction sent with each heartbeat" maxLength:"2000"`
	}
}

type UpdateClawSettingsOutput struct {
	Body ClawDeployment
}

type ClawMessagesInput struct {
	Authorization string `header:"Authorization" doc:"Bearer PocketBase auth token" required:"true"`
	ID            string `path:"id" doc:"Claw deployment ID"`
	Since         string `query:"since" doc:"Only messages after this timestamp"`
	Limit         int    `query:"limit" default:"50" minimum:"1" maximum:"200" doc:"Max messages"`
}

type ClawMessage struct {
	ID         string `json:"id"`
	AuthorID   string `json:"author_id"`
	AuthorName string `json:"author_name"`
	Body       string `json:"body"`
	Created    string `json:"created"`
}

type ClawMessagesOutput struct {
	Body struct {
		Messages []ClawMessage `json:"messages"`
	}
}

type SendClawMsgInput struct {
	Authorization string `header:"Authorization" doc:"Bearer PocketBase auth token" required:"true"`
	ID            string `path:"id" doc:"Claw deployment ID"`
	Body          struct {
		Body string `json:"body" doc:"Message content" minLength:"1" maxLength:"5000"`
	}
}

type SendClawMsgOutput struct {
	Body struct {
		Message       ClawMessage `json:"message"`
		UserMessageID string      `json:"user_message_id"`
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
		if clawType == "" || clawType == "picoclaw" {
			clawType = "lite"
		}
		if clawType != "lite" && clawType != "pro" && clawType != "max" {
			return nil, huma.Error422UnprocessableEntity("claw_type must be lite, pro, or max")
		}

		col, err := app.FindCollectionByNameOrId("claw_deployments")
		if err != nil {
			return nil, huma.Error500InternalServerError("claw_deployments collection not found")
		}
		record := core.NewRecord(col)
		record.Set("user_id", userID)

		record.Set("name", name)
		record.Set("status", "queued")
		record.Set("instructions", strings.TrimSpace(input.Body.Instructions))
		record.Set("github_repo", strings.TrimSpace(input.Body.GithubRepo))
		record.Set("claw_type", clawType)

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

		// Remove the Docker container if it exists
		containerID := record.GetString("container_id")
		if containerID != "" {
			cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
			if err == nil {
				cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
				cli.Close()
			}
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

	// PATCH /api/claws/{id} — update claw settings
	huma.Register(api, huma.Operation{
		OperationID: "update-claw-settings",
		Method:      "PATCH",
		Path:        "/api/claws/{id}",
		Summary:     "Update Claw settings",
		Description: "Update claw settings (heartbeat, public page). Only the owning user can update.",
		Tags:        []string{"Claws"},
	}, func(ctx context.Context, input *UpdateClawSettingsInput) (*UpdateClawSettingsOutput, error) {
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

		if input.Body.IsPublic != nil {
			record.Set("is_public", *input.Body.IsPublic)
		}
		if input.Body.HeartbeatInterval != nil {
			v := *input.Body.HeartbeatInterval
			allowed := map[int]bool{0: true, 15: true, 30: true, 60: true, 360: true, 1440: true}
			if !allowed[v] {
				return nil, huma.Error422UnprocessableEntity("heartbeat_interval must be 0, 15, 30, 60, 360, or 1440")
			}
			record.Set("heartbeat_interval", v)
		}
		if input.Body.HeartbeatInstruction != nil {
			record.Set("heartbeat_instruction", *input.Body.HeartbeatInstruction)
		}

		if err := app.Save(record); err != nil {
			return nil, huma.Error500InternalServerError("Failed to update settings")
		}

		out := &UpdateClawSettingsOutput{}
		out.Body = recordToClawDeployment(record)
		return out, nil
	})

	// GET /api/claws/{id}/messages — read messages from claw's channel
	huma.Register(api, huma.Operation{
		OperationID: "get-claw-messages",
		Method:      "GET",
		Path:        "/api/claws/{id}/messages",
		Summary:     "Read claw messages",
		Description: "Read messages from a claw's default channel. Only the claw owner can access.",
		Tags:        []string{"Claws"},
	}, func(ctx context.Context, input *ClawMessagesInput) (*ClawMessagesOutput, error) {
		userID, err := extractPBUserID(app, input.Authorization)
		if err != nil {
			return nil, huma.Error401Unauthorized("Authentication required")
		}

		record, err := app.FindRecordById("claw_deployments", input.ID)
		if err != nil || record.GetString("user_id") != userID {
			return nil, huma.Error404NotFound("Claw not found")
		}

		channelID, err := findClawChannel(app, record.GetString("agent_id"))
		if err != nil {
			return nil, huma.Error404NotFound("Claw channel not found")
		}

		filter := "channel_id = {:cid}"
		params := map[string]any{"cid": channelID}
		if input.Since != "" {
			filter += " && created > {:since}"
			params["since"] = input.Since
		}

		records, _ := app.FindRecordsByFilter("channel_messages", filter, "-created", input.Limit, 0, params)

		nameCache := map[string]string{}
		messages := make([]ClawMessage, 0, len(records))
		for _, r := range records {
			authorID := r.GetString("author_id")
			if _, ok := nameCache[authorID]; !ok {
				nameCache[authorID] = resolveAuthorName(app, authorID)
			}
			messages = append(messages, ClawMessage{
				ID:         r.Id,
				AuthorID:   authorID,
				AuthorName: nameCache[authorID],
				Body:       r.GetString("body"),
				Created:    r.GetString("created"),
			})
		}

		out := &ClawMessagesOutput{}
		out.Body.Messages = messages
		return out, nil
	})

	// POST /api/claws/{id}/messages — send message to claw's channel
	huma.Register(api, huma.Operation{
		OperationID: "send-claw-message",
		Method:      "POST",
		Path:        "/api/claws/{id}/messages",
		Summary:     "Send message to claw",
		Description: "Send a message to a claw's default channel. Only the claw owner can send.",
		Tags:        []string{"Claws"},
	}, func(ctx context.Context, input *SendClawMsgInput) (*SendClawMsgOutput, error) {
		userID, err := extractPBUserID(app, input.Authorization)
		if err != nil {
			return nil, huma.Error401Unauthorized("Authentication required")
		}

		record, err := app.FindRecordById("claw_deployments", input.ID)
		if err != nil || record.GetString("user_id") != userID {
			return nil, huma.Error404NotFound("Claw not found")
		}

		agentID := record.GetString("agent_id")
		channelID, err := findClawChannel(app, agentID)
		if err != nil {
			return nil, huma.Error404NotFound("Claw channel not found")
		}

		col, err := app.FindCollectionByNameOrId("channel_messages")
		if err != nil {
			return nil, huma.Error500InternalServerError("channel_messages collection not found")
		}

		// Save user's message
		userAuthorID := "user:" + userID
		msgRec := core.NewRecord(col)
		msgRec.Set("channel_id", channelID)
		msgRec.Set("author_id", userAuthorID)
		msgRec.Set("body", input.Body.Body)
		if err := app.Save(msgRec); err != nil {
			return nil, huma.Error500InternalServerError("Failed to save message")
		}

		// Forward to claw container's ADK API
		containerID := record.GetString("container_id")
		if containerID == "" {
			return nil, huma.Error422UnprocessableEntity("Claw container not running")
		}

		adkReply, err := sendToADK(containerID, userID, input.Body.Body)
		if err != nil {
			app.Logger().Error("ADK proxy failed", "claw", containerID, "error", err)
			return nil, huma.NewError(http.StatusBadGateway, fmt.Sprintf("Claw did not respond: %v", err))
		}

		// Save the claw's response as a channel message
		replyRec := core.NewRecord(col)
		replyRec.Set("channel_id", channelID)
		replyRec.Set("author_id", agentID)
		replyRec.Set("body", adkReply)
		if err := app.Save(replyRec); err != nil {
			app.Logger().Error("Failed to save claw reply", "claw", containerID, "error", err)
		}

		// Return the claw's reply + user message ID (so frontend can de-dupe polls)
		out := &SendClawMsgOutput{}
		out.Body.UserMessageID = msgRec.Id
		out.Body.Message = ClawMessage{
			ID:         replyRec.Id,
			AuthorID:   agentID,
			AuthorName: resolveAuthorName(app, agentID),
			Body:       adkReply,
			Created:    replyRec.GetString("created"),
		}
		return out, nil
	})

	// =========================================================================
	// Vault endpoints — per-user secrets for claw env injection
	// =========================================================================
}

// ---------------------------------------------------------------------------
// Bridge proxy — forward user messages to the claw's bridge middleware
// ---------------------------------------------------------------------------

// bridgeRequest is the JSON body for POST /msg on the claw proxy (→ bridge).
type bridgeRequest struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Text     string `json:"text"`
	Protocol string `json:"protocol"`
}

// bridgeResponse is the JSON response from the bridge.
type bridgeResponse struct {
	Text  string `json:"text"`
	Error string `json:"error,omitempty"`
}

var adkClient = &http.Client{Timeout: 120 * time.Second}

// sendToADK forwards a user message to the claw's bridge middleware and returns the agent's text response.
// The bridge handles session management, token estimation, and compaction.
func sendToADK(containerName, userID, text string) (string, error) {
	base := fmt.Sprintf("http://%s:8080", containerName)

	body, _ := json.Marshal(bridgeRequest{
		UserID:   userID,
		Username: userID,
		Text:     text,
		Protocol: "gather-ui",
	})

	resp, err := adkClient.Post(base+"/msg", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("bridge request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("bridge response read failed: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("bridge returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result bridgeResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("bridge response parse failed: %w", err)
	}

	if result.Error != "" {
		return "", fmt.Errorf("bridge error: %s", result.Error)
	}

	if result.Text == "" {
		return "", fmt.Errorf("no response from agent")
	}

	return result.Text, nil
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

// extractPBUserRecord parses a PocketBase auth token and returns the full record.
func extractPBUserRecord(app *pocketbase.PocketBase, authHeader string) (*core.Record, error) {
	token := strings.TrimPrefix(authHeader, "Bearer ")
	token = strings.TrimPrefix(token, "bearer ")
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, huma.Error401Unauthorized("Missing auth token")
	}
	return app.FindAuthRecordByToken(token, "auth")
}

// findClawChannel finds the default channel for a claw agent.
func findClawChannel(app *pocketbase.PocketBase, agentID string) (string, error) {
	if agentID == "" {
		return "", fmt.Errorf("no agent_id")
	}
	members, err := app.FindRecordsByFilter("channel_members",
		"agent_id = {:aid} && role = 'owner'", "", 1, 0,
		map[string]any{"aid": agentID})
	if err != nil || len(members) == 0 {
		return "", fmt.Errorf("no channel found for agent %s", agentID)
	}
	return members[0].GetString("channel_id"), nil
}


// resolveAuthorName resolves a display name for a message author.
// Handles both agent IDs and "user:{pbId}" format.
func resolveAuthorName(app *pocketbase.PocketBase, authorID string) string {
	if strings.HasPrefix(authorID, "user:") {
		pbID := strings.TrimPrefix(authorID, "user:")
		rec, err := app.FindRecordById("users", pbID)
		if err == nil {
			if name := rec.GetString("name"); name != "" {
				return name
			}
			if email := rec.GetString("email"); email != "" {
				return email
			}
		}
		return "You"
	}
	return agentName(app, authorID)
}
