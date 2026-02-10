package api

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"gather.is/auth/ratelimit"
)

// -----------------------------------------------------------------------------
// Request / Response types
// -----------------------------------------------------------------------------

type InboxMessage struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	Read    bool   `json:"read"`
	RefType string `json:"ref_type,omitempty"`
	RefID   string `json:"ref_id,omitempty"`
	Created string `json:"created"`
}

type InboxListInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
	UnreadOnly    bool   `query:"unread_only" default:"false" doc:"Only return unread messages"`
	Limit         int    `query:"limit" default:"20" minimum:"1" maximum:"100" doc:"Max messages to return"`
	Offset        int    `query:"offset" default:"0" minimum:"0" doc:"Number of messages to skip"`
}

type InboxListOutput struct {
	Body struct {
		Messages []InboxMessage `json:"messages"`
		Total    int            `json:"total"`
		Unread   int            `json:"unread"`
	}
}

type InboxUnreadInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
}

type InboxUnreadOutput struct {
	Body struct {
		Unread int `json:"unread"`
	}
}

type InboxMarkReadInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
	ID            string `path:"id" doc:"Message ID"`
}

type InboxMarkReadOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}

type InboxDeleteInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
	ID            string `path:"id" doc:"Message ID"`
}

type InboxDeleteOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}

// -----------------------------------------------------------------------------
// Route registration
// -----------------------------------------------------------------------------

func RegisterInboxRoutes(api huma.API, app *pocketbase.PocketBase, jwtKey []byte) {
	huma.Register(api, huma.Operation{
		OperationID: "list-inbox",
		Method:      "GET",
		Path:        "/api/inbox",
		Summary:     "List inbox messages",
		Description: "Returns messages for the authenticated agent, newest first. Use ?unread_only=true to filter.",
		Tags:        []string{"Inbox"},
	}, func(ctx context.Context, input *InboxListInput) (*InboxListOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}
		if err := ratelimit.CheckAgent(claims.AgentID, false); err != nil {
			return nil, err
		}

		filter := "agent_id = {:aid}"
		if input.UnreadOnly {
			filter += " && read = false"
		}
		params := map[string]any{"aid": claims.AgentID}

		// Get total matching count
		allMatching, _ := app.FindRecordsByFilter("messages", filter, "", 0, 0, params)
		total := len(allMatching)

		// Get unread count
		unreadRecs, _ := app.FindRecordsByFilter("messages", "agent_id = {:aid} && read = false", "", 0, 0, params)
		unread := len(unreadRecs)

		// Get paginated results (newest first via slice reversal;
		// PocketBase's FindRecordsByFilter sort with "-created" fails silently on base collections)
		records, _ := app.FindRecordsByFilter("messages", filter, "", input.Limit, input.Offset, params)

		// Reverse for newest-first ordering
		for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
			records[i], records[j] = records[j], records[i]
		}

		messages := make([]InboxMessage, 0, len(records))
		for _, r := range records {
			messages = append(messages, InboxMessage{
				ID:      r.Id,
				Type:    r.GetString("type"),
				Subject: r.GetString("subject"),
				Body:    r.GetString("body"),
				Read:    r.GetBool("read"),
				RefType: r.GetString("ref_type"),
				RefID:   r.GetString("ref_id"),
				Created: r.GetString("created"),
			})
		}

		out := &InboxListOutput{}
		out.Body.Messages = messages
		out.Body.Total = total
		out.Body.Unread = unread
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "inbox-unread-count",
		Method:      "GET",
		Path:        "/api/inbox/unread",
		Summary:     "Get unread message count",
		Description: "Fast endpoint for polling. Returns just the unread count.",
		Tags:        []string{"Inbox"},
	}, func(ctx context.Context, input *InboxUnreadInput) (*InboxUnreadOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		unreadRecs, _ := app.FindRecordsByFilter("messages", "agent_id = {:aid} && read = false", "", 0, 0, map[string]any{"aid": claims.AgentID})

		out := &InboxUnreadOutput{}
		out.Body.Unread = len(unreadRecs)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "mark-message-read",
		Method:      "PUT",
		Path:        "/api/inbox/{id}/read",
		Summary:     "Mark message as read",
		Description: "Marks a single inbox message as read. You can only mark your own messages.",
		Tags:        []string{"Inbox"},
	}, func(ctx context.Context, input *InboxMarkReadInput) (*InboxMarkReadOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		record, err := app.FindRecordById("messages", input.ID)
		if err != nil {
			return nil, huma.Error404NotFound("Message not found.")
		}
		if record.GetString("agent_id") != claims.AgentID {
			return nil, huma.Error403Forbidden("You can only access your own messages.")
		}

		record.Set("read", true)
		if err := app.Save(record); err != nil {
			return nil, huma.Error500InternalServerError("Failed to update message")
		}

		out := &InboxMarkReadOutput{}
		out.Body.Status = "read"
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-message",
		Method:      "DELETE",
		Path:        "/api/inbox/{id}",
		Summary:     "Delete a message",
		Description: "Permanently deletes an inbox message. You can only delete your own messages.",
		Tags:        []string{"Inbox"},
	}, func(ctx context.Context, input *InboxDeleteInput) (*InboxDeleteOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		record, err := app.FindRecordById("messages", input.ID)
		if err != nil {
			return nil, huma.Error404NotFound("Message not found.")
		}
		if record.GetString("agent_id") != claims.AgentID {
			return nil, huma.Error403Forbidden("You can only delete your own messages.")
		}

		if err := app.Delete(record); err != nil {
			return nil, huma.Error500InternalServerError("Failed to delete message")
		}

		out := &InboxDeleteOutput{}
		out.Body.Status = "deleted"
		return out, nil
	})
}

// SendInboxMessage creates a message in an agent's inbox.
// Exported so shop.go and auth.go can call it.
func SendInboxMessage(app *pocketbase.PocketBase, agentID, msgType, subject, body, refType, refID string) {
	collection, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		app.Logger().Warn("Cannot send inbox message: messages collection not found", "error", err)
		return
	}

	record := core.NewRecord(collection)
	record.Set("agent_id", agentID)
	record.Set("type", msgType)
	record.Set("subject", subject)
	record.Set("body", body)
	record.Set("read", false)
	record.Set("ref_type", refType)
	record.Set("ref_id", refID)

	if err := app.Save(record); err != nil {
		app.Logger().Warn("Failed to save inbox message",
			"agent_id", agentID,
			"type", msgType,
			"error", err,
		)
	}
}

// UnreadCount returns the number of unread messages for an agent.
func UnreadCount(app *pocketbase.PocketBase, agentID string) int {
	recs, _ := app.FindRecordsByFilter("messages", "agent_id = {:aid} && read = false", "", 0, 0, map[string]any{"aid": agentID})
	return len(recs)
}

// formatOrderID returns a short display form like "ORD-abc123".
func formatOrderID(id string) string {
	short := id
	if len(short) > 8 {
		short = short[:8]
	}
	return fmt.Sprintf("ORD-%s", short)
}
