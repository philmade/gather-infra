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
		Message ClawMessage `json:"message"`
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

		// Return the claw's reply
		out := &SendClawMsgOutput{}
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
// ADK proxy — forward user messages to the claw container's ADK API
// ---------------------------------------------------------------------------

// adkRunRequest is the JSON body for POST /api/run on the ADK server.
type adkRunRequest struct {
	AppName    string     `json:"appName"`
	UserID     string     `json:"userId"`
	SessionID  string     `json:"sessionId"`
	NewMessage adkMessage `json:"newMessage"`
}

type adkMessage struct {
	Role  string    `json:"role"`
	Parts []adkPart `json:"parts"`
}

type adkPart struct {
	Text string `json:"text"`
}

type adkEvent struct {
	Content *adkMessage `json:"content,omitempty"`
	Author  string      `json:"author,omitempty"`
}

var adkClient = &http.Client{Timeout: 120 * time.Second}

// sendToADK forwards a user message to the claw's ADK API and returns the agent's text response.
func sendToADK(containerName, userID, text string) (string, error) {
	base := fmt.Sprintf("http://%s:8080", containerName)
	sessionID := fmt.Sprintf("chat-%s", userID)

	// Ensure session exists (idempotent — ADK returns existing if already created)
	sessURL := fmt.Sprintf("%s/api/apps/clawpoint/users/%s/sessions/%s", base, userID, sessionID)
	sessReq, _ := http.NewRequest("POST", sessURL, bytes.NewBufferString("{}"))
	sessReq.Header.Set("Content-Type", "application/json")
	sessResp, err := adkClient.Do(sessReq)
	if err != nil {
		return "", fmt.Errorf("ADK session create failed: %w", err)
	}
	sessResp.Body.Close()

	// Send the message
	body, _ := json.Marshal(adkRunRequest{
		AppName:   "clawpoint",
		UserID:    userID,
		SessionID: sessionID,
		NewMessage: adkMessage{
			Role:  "user",
			Parts: []adkPart{{Text: text}},
		},
	})

	runResp, err := adkClient.Post(base+"/api/run", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ADK run failed: %w", err)
	}
	defer runResp.Body.Close()

	respBody, err := io.ReadAll(runResp.Body)
	if err != nil {
		return "", fmt.Errorf("ADK response read failed: %w", err)
	}

	if runResp.StatusCode != 200 {
		return "", fmt.Errorf("ADK returned %d: %s", runResp.StatusCode, string(respBody))
	}

	// Parse ADK response — array of events, find the last model response
	var events []adkEvent
	if err := json.Unmarshal(respBody, &events); err != nil {
		return "", fmt.Errorf("ADK response parse failed: %w", err)
	}

	// Walk events in reverse to find the last model content
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.Content != nil && ev.Content.Role == "model" && len(ev.Content.Parts) > 0 {
			var parts []string
			for _, p := range ev.Content.Parts {
				if p.Text != "" {
					parts = append(parts, p.Text)
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, "\n"), nil
			}
		}
	}

	return "", fmt.Errorf("no model response in ADK events")
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
