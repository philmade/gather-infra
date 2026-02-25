package api

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	gatheremail "gather.is/auth/email"
	"gather.is/auth/ratelimit"
)

// -----------------------------------------------------------------------------
// Types
// -----------------------------------------------------------------------------

type EmailItem struct {
	ID        string `json:"id"`
	Direction string `json:"direction"`
	FromAddr  string `json:"from_addr"`
	ToAddr    string `json:"to_addr"`
	Subject   string `json:"subject"`
	BodyText  string `json:"body_text,omitempty"`
	Read      bool   `json:"read"`
	Created   string `json:"created"`
}

type EmailDetail struct {
	EmailItem
	BodyHTML  string `json:"body_html,omitempty"`
	MessageID string `json:"message_id,omitempty"`
	InReplyTo string `json:"in_reply_to,omitempty"`
}

// --- List ---

type EmailListInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
	Direction     string `query:"direction" doc:"Filter: inbound, outbound, or blank for all"`
	UnreadOnly    bool   `query:"unread_only" default:"false" doc:"Only return unread messages"`
	Limit         int    `query:"limit" default:"20" minimum:"1" maximum:"100"`
	Offset        int    `query:"offset" default:"0" minimum:"0"`
}

type EmailListOutput struct {
	Body struct {
		Emails []EmailItem `json:"emails"`
		Total  int         `json:"total"`
		Unread int         `json:"unread"`
	}
}

// --- Detail ---

type EmailDetailInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
	ID            string `path:"id" doc:"Email record ID"`
}

type EmailDetailOutput struct {
	Body EmailDetail
}

// --- Mark read ---

type EmailMarkReadInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
	ID            string `path:"id" doc:"Email record ID"`
}

type EmailMarkReadOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}

// --- Delete ---

type EmailDeleteInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
	ID            string `path:"id" doc:"Email record ID"`
}

type EmailDeleteOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}

// --- Send ---

type EmailSendInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
	Body          struct {
		To        string `json:"to" required:"true" doc:"Recipient email address"`
		Subject   string `json:"subject" required:"true" doc:"Email subject line"`
		BodyHTML  string `json:"body_html" required:"true" doc:"HTML body content"`
		InReplyTo string `json:"in_reply_to,omitempty" doc:"Message-ID being replied to"`
	}
}

type EmailSendOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}

// --- Inbound (internal) ---

type EmailInboundInput struct {
	Body struct {
		Secret    string `json:"secret" required:"true"`
		FromAddr  string `json:"from_addr" required:"true"`
		ToAddr    string `json:"to_addr" required:"true"`
		Subject   string `json:"subject"`
		BodyHTML  string `json:"body_html"`
		BodyText  string `json:"body_text"`
		MessageID string `json:"message_id"`
		InReplyTo string `json:"in_reply_to"`
	}
}

type EmailInboundOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}

// -----------------------------------------------------------------------------
// Route registration
// -----------------------------------------------------------------------------

func RegisterEmailRoutes(api huma.API, app *pocketbase.PocketBase, jwtKey []byte) {

	// GET /api/email — list emails
	huma.Register(api, huma.Operation{
		OperationID: "list-emails",
		Method:      "GET",
		Path:        "/api/email",
		Summary:     "List emails",
		Description: "Returns emails for the authenticated agent, newest first.",
		Tags:        []string{"Email"},
	}, func(ctx context.Context, input *EmailListInput) (*EmailListOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}
		if err := ratelimit.CheckAgent(claims.AgentID, false); err != nil {
			return nil, err
		}

		filter := "agent_id = {:aid}"
		params := map[string]any{"aid": claims.AgentID}

		if input.Direction != "" {
			filter += " && direction = {:dir}"
			params["dir"] = input.Direction
		}
		if input.UnreadOnly {
			filter += " && read = false"
		}

		allMatching, _ := app.FindRecordsByFilter("emails", filter, "", 0, 0, params)
		total := len(allMatching)

		unreadRecs, _ := app.FindRecordsByFilter("emails", "agent_id = {:aid} && read = false", "", 0, 0, map[string]any{"aid": claims.AgentID})
		unread := len(unreadRecs)

		records, _ := app.FindRecordsByFilter("emails", filter, "-created", input.Limit, input.Offset, params)

		emails := make([]EmailItem, 0, len(records))
		for _, r := range records {
			emails = append(emails, EmailItem{
				ID:        r.Id,
				Direction: r.GetString("direction"),
				FromAddr:  r.GetString("from_addr"),
				ToAddr:    r.GetString("to_addr"),
				Subject:   r.GetString("subject"),
				BodyText:  truncate(r.GetString("body_text"), 200),
				Read:      r.GetBool("read"),
				Created:   r.GetString("created"),
			})
		}

		out := &EmailListOutput{}
		out.Body.Emails = emails
		out.Body.Total = total
		out.Body.Unread = unread
		return out, nil
	})

	// GET /api/email/{id} — single email detail
	huma.Register(api, huma.Operation{
		OperationID: "get-email",
		Method:      "GET",
		Path:        "/api/email/{id}",
		Summary:     "Get email detail",
		Description: "Returns full email content including HTML body.",
		Tags:        []string{"Email"},
	}, func(ctx context.Context, input *EmailDetailInput) (*EmailDetailOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		record, err := app.FindRecordById("emails", input.ID)
		if err != nil {
			return nil, huma.Error404NotFound("Email not found.")
		}
		if record.GetString("agent_id") != claims.AgentID {
			return nil, huma.Error403Forbidden("You can only access your own emails.")
		}

		out := &EmailDetailOutput{}
		out.Body = EmailDetail{
			EmailItem: EmailItem{
				ID:        record.Id,
				Direction: record.GetString("direction"),
				FromAddr:  record.GetString("from_addr"),
				ToAddr:    record.GetString("to_addr"),
				Subject:   record.GetString("subject"),
				BodyText:  record.GetString("body_text"),
				Read:      record.GetBool("read"),
				Created:   record.GetString("created"),
			},
			BodyHTML:  record.GetString("body_html"),
			MessageID: record.GetString("message_id"),
			InReplyTo: record.GetString("in_reply_to"),
		}
		return out, nil
	})

	// PUT /api/email/{id}/read — mark as read
	huma.Register(api, huma.Operation{
		OperationID: "mark-email-read",
		Method:      "PUT",
		Path:        "/api/email/{id}/read",
		Summary:     "Mark email as read",
		Tags:        []string{"Email"},
	}, func(ctx context.Context, input *EmailMarkReadInput) (*EmailMarkReadOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		record, err := app.FindRecordById("emails", input.ID)
		if err != nil {
			return nil, huma.Error404NotFound("Email not found.")
		}
		if record.GetString("agent_id") != claims.AgentID {
			return nil, huma.Error403Forbidden("You can only access your own emails.")
		}

		record.Set("read", true)
		if err := app.Save(record); err != nil {
			return nil, huma.Error500InternalServerError("Failed to update email")
		}

		out := &EmailMarkReadOutput{}
		out.Body.Status = "read"
		return out, nil
	})

	// DELETE /api/email/{id} — delete email
	huma.Register(api, huma.Operation{
		OperationID: "delete-email",
		Method:      "DELETE",
		Path:        "/api/email/{id}",
		Summary:     "Delete an email",
		Tags:        []string{"Email"},
	}, func(ctx context.Context, input *EmailDeleteInput) (*EmailDeleteOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		record, err := app.FindRecordById("emails", input.ID)
		if err != nil {
			return nil, huma.Error404NotFound("Email not found.")
		}
		if record.GetString("agent_id") != claims.AgentID {
			return nil, huma.Error403Forbidden("You can only delete your own emails.")
		}

		if err := app.Delete(record); err != nil {
			return nil, huma.Error500InternalServerError("Failed to delete email")
		}

		out := &EmailDeleteOutput{}
		out.Body.Status = "deleted"
		return out, nil
	})

	// POST /api/email/send — send email (agent-authenticated)
	huma.Register(api, huma.Operation{
		OperationID: "send-email",
		Method:      "POST",
		Path:        "/api/email/send",
		Summary:     "Send an email",
		Description: "Sends an email as <agent-name>@gather.is. Agent can only send from their own address.",
		Tags:        []string{"Email"},
	}, func(ctx context.Context, input *EmailSendInput) (*EmailSendOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}
		if err := ratelimit.CheckAgent(claims.AgentID, false); err != nil {
			return nil, err
		}

		// Look up agent name to construct from address
		agent, err := app.FindRecordById("agents", claims.AgentID)
		if err != nil {
			return nil, huma.Error404NotFound("Agent not found.")
		}
		agentName := agent.GetString("name")
		if agentName == "" {
			return nil, huma.Error422UnprocessableEntity("Agent has no name set.")
		}

		fromAddr := agentName + "@gather.is"

		// Store outbound record
		col, err := app.FindCollectionByNameOrId("emails")
		if err != nil {
			return nil, huma.Error500InternalServerError("Emails collection not found")
		}
		record := core.NewRecord(col)
		record.Set("agent_id", claims.AgentID)
		record.Set("direction", "outbound")
		record.Set("from_addr", fromAddr)
		record.Set("to_addr", input.Body.To)
		record.Set("subject", input.Body.Subject)
		record.Set("body_html", input.Body.BodyHTML)
		record.Set("in_reply_to", input.Body.InReplyTo)
		record.Set("read", true)
		if err := app.Save(record); err != nil {
			return nil, huma.Error500InternalServerError("Failed to save email record")
		}

		// Send via Cloudflare worker
		if err := gatheremail.SendAs(input.Body.To, input.Body.Subject, input.Body.BodyHTML, fromAddr, agentName); err != nil {
			log.Printf("[EMAIL] Send failed for %s: %v", fromAddr, err)
			// Don't fail the request — email is stored, delivery failed
		}

		out := &EmailSendOutput{}
		out.Body.Status = "sent"
		return out, nil
	})

	// POST /api/email/inbound — receive email (internal, secret-authenticated)
	huma.Register(api, huma.Operation{
		OperationID: "receive-inbound-email",
		Method:      "POST",
		Path:        "/api/email/inbound",
		Summary:     "Deliver inbound email",
		Description: "Internal endpoint called by Cloudflare Email Worker to deliver inbound mail.",
		Tags:        []string{"Email"},
	}, func(ctx context.Context, input *EmailInboundInput) (*EmailInboundOutput, error) {
		// Validate internal secret
		secret := os.Getenv("EMAIL_INBOUND_SECRET")
		if secret == "" || input.Body.Secret != secret {
			return nil, huma.Error401Unauthorized("Invalid secret.")
		}

		// Extract username from To address
		username := extractLocalPart(input.Body.ToAddr)
		if username == "" {
			return nil, huma.Error400BadRequest("Invalid to_addr.")
		}

		// Find agent by name (claw deployments use name as username)
		agent, err := app.FindFirstRecordByFilter("agents", "name = {:n}", map[string]any{"n": username})
		if err != nil {
			// Try case-insensitive
			agent, err = app.FindFirstRecordByFilter("agents", "name = {:n}", map[string]any{"n": strings.ToLower(username)})
			if err != nil {
				return nil, huma.Error404NotFound(fmt.Sprintf("No agent found for %s@gather.is", username))
			}
		}
		agentID := agent.Id

		// Store inbound email
		col, err := app.FindCollectionByNameOrId("emails")
		if err != nil {
			return nil, huma.Error500InternalServerError("Emails collection not found")
		}
		record := core.NewRecord(col)
		record.Set("agent_id", agentID)
		record.Set("direction", "inbound")
		record.Set("from_addr", input.Body.FromAddr)
		record.Set("to_addr", input.Body.ToAddr)
		record.Set("subject", input.Body.Subject)
		record.Set("body_html", input.Body.BodyHTML)
		record.Set("body_text", input.Body.BodyText)
		record.Set("message_id", input.Body.MessageID)
		record.Set("in_reply_to", input.Body.InReplyTo)
		record.Set("read", false)
		if err := app.Save(record); err != nil {
			return nil, huma.Error500InternalServerError("Failed to save email record")
		}

		// Send inbox notification
		SendInboxMessage(app, agentID, "email", "New email from "+input.Body.FromAddr, "Subject: "+input.Body.Subject, "email", record.Id)

		// Wake up the claw if it has a running container
		go wakeClawForEmail(app, agentID, input.Body.FromAddr, input.Body.Subject, input.Body.BodyText)

		out := &EmailInboundOutput{}
		out.Body.Status = "delivered"
		return out, nil
	})
}

// wakeClawForEmail finds the claw container for an agent and sends it the email as a message.
func wakeClawForEmail(app *pocketbase.PocketBase, agentID, fromAddr, subject, bodyText string) {
	// Find claw deployment for this agent
	deployment, err := app.FindFirstRecordByFilter("claw_deployments", "agent_id = {:aid}", map[string]any{"aid": agentID})
	if err != nil {
		return // No claw deployed — that's fine, email is stored
	}

	containerID := deployment.GetString("container_id")
	if containerID == "" {
		return // Container not running
	}

	// Compose a concise message for the claw
	text := fmt.Sprintf("[EMAIL from %s] Subject: %s\n\n%s", fromAddr, subject, truncate(bodyText, 2000))

	result, err := sendToADK(containerID, "email:"+fromAddr, text)
	if err != nil {
		log.Printf("[EMAIL] Failed to wake claw %s: %v", containerID, err)
		return
	}
	log.Printf("[EMAIL] Claw %s woke up and replied: %s", containerID, truncate(result.Text, 100))
}

// extractLocalPart returns the local part of an email address (before @).
func extractLocalPart(addr string) string {
	// Handle "Name <user@domain>" format
	if idx := strings.Index(addr, "<"); idx >= 0 {
		addr = addr[idx+1:]
		if idx := strings.Index(addr, ">"); idx >= 0 {
			addr = addr[:idx]
		}
	}
	parts := strings.SplitN(addr, "@", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parts[0]))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
