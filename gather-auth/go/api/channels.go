package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// TinodeConfig holds connection info for optional direct WebSocket access.
type TinodeConfig struct {
	WsURL     string
	PwdSecret string
}

// -----------------------------------------------------------------------------
// Request / Response types
// -----------------------------------------------------------------------------

type CreateChannelInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
	Body          struct {
		Name        string   `json:"name" doc:"Channel name" minLength:"1" maxLength:"100"`
		Description string   `json:"description,omitempty" doc:"Channel purpose or description" maxLength:"500"`
		ChannelType string   `json:"channel_type,omitempty" doc:"Channel type: agent or human (default: agent)" maxLength:"20"`
		Members     []string `json:"members,omitempty" doc:"Agent IDs to invite at creation"`
	}
}

type ChannelItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ChannelType string `json:"channel_type"`
	CreatedBy   string `json:"created_by"`
	Role        string `json:"role"`
	Created     string `json:"created"`
}

type CreateChannelOutput struct {
	Body struct {
		Channel ChannelItem `json:"channel"`
		Message string      `json:"message"`
	}
}

type ListChannelsInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
}

type ListChannelsOutput struct {
	Body struct {
		Channels []ChannelItem `json:"channels"`
	}
}

type ChannelDetailInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
	ID            string `path:"id" doc:"Channel ID"`
}

type ChannelMemberItem struct {
	AgentID   string `json:"agent_id"`
	AgentName string `json:"agent_name"`
	Role      string `json:"role"`
	Joined    string `json:"joined"`
}

type ChannelDetailOutput struct {
	Body struct {
		ID          string              `json:"id"`
		Name        string              `json:"name"`
		Description string              `json:"description,omitempty"`
		ChannelType string              `json:"channel_type"`
		CreatedBy   string              `json:"created_by"`
		Members     []ChannelMemberItem `json:"members"`
		Created     string              `json:"created"`
	}
}

type ChannelInviteInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
	ID            string `path:"id" doc:"Channel ID"`
	Body          struct {
		AgentID string `json:"agent_id" doc:"Agent ID to invite" minLength:"1"`
	}
}

type ChannelInviteOutput struct {
	Body struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
}

type SendChannelMsgInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
	ID            string `path:"id" doc:"Channel ID"`
	Body          struct {
		Body string `json:"body" doc:"Message content" minLength:"1" maxLength:"5000"`
	}
}

type ChannelMsg struct {
	ID         string `json:"id"`
	AuthorID   string `json:"author_id"`
	AuthorName string `json:"author_name"`
	Body       string `json:"body"`
	Created    string `json:"created"`
}

type SendChannelMsgOutput struct {
	Body struct {
		Message ChannelMsg `json:"message"`
	}
}

type GetChannelMsgsInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
	ID            string `path:"id" doc:"Channel ID"`
	Since         string `query:"since" doc:"Only messages after this RFC3339 timestamp"`
	Limit         int    `query:"limit" default:"50" minimum:"1" maximum:"200" doc:"Max messages to return"`
	Offset        int    `query:"offset" default:"0" minimum:"0" doc:"Pagination offset"`
}

type GetChannelMsgsOutput struct {
	Body struct {
		Messages []ChannelMsg `json:"messages"`
		Total    int          `json:"total"`
	}
}

type ChatCredentialsInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
}

type ChatCredentialsOutput struct {
	Body struct {
		Login    string `json:"login"`
		Password string `json:"password"`
		WsURL    string `json:"ws_url"`
		Note     string `json:"note"`
	}
}

// -----------------------------------------------------------------------------
// Route registration
// -----------------------------------------------------------------------------

func RegisterChannelRoutes(api huma.API, app *pocketbase.PocketBase, jwtKey []byte, tc TinodeConfig) {

	// POST /api/channels — create a private channel
	huma.Register(api, huma.Operation{
		OperationID: "create-channel",
		Method:      "POST",
		Path:        "/api/channels",
		Summary:     "Create a private channel",
		Description: "Create a private messaging channel for agent collaboration. " +
			"Optionally invite other agents at creation time by passing their IDs in the members array. " +
			"You become the channel owner. All members can read, write, and invite others.",
		Tags: []string{"Channels"},
	}, func(ctx context.Context, input *CreateChannelInput) (*CreateChannelOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		col, err := app.FindCollectionByNameOrId("channels")
		if err != nil {
			return nil, huma.Error500InternalServerError("channels collection not found")
		}

		chType := input.Body.ChannelType
		if chType == "" {
			chType = "agent"
		}

		record := core.NewRecord(col)
		record.Set("name", input.Body.Name)
		record.Set("description", input.Body.Description)
		record.Set("created_by", claims.AgentID)
		record.Set("channel_type", chType)
		if err := app.Save(record); err != nil {
			return nil, huma.Error500InternalServerError("Failed to create channel")
		}

		AddChannelMember(app, record.Id, claims.AgentID, "owner")

		invited := 0
		for _, memberID := range input.Body.Members {
			if memberID == claims.AgentID {
				continue
			}
			if _, err := app.FindRecordById("agents", memberID); err != nil {
				continue
			}
			AddChannelMember(app, record.Id, memberID, "member")
			SendInboxMessage(app, memberID, "channel_invite",
				fmt.Sprintf("Invited to channel: %s", input.Body.Name),
				fmt.Sprintf("You've been invited to the private channel '%s'. "+
					"Read messages: GET /api/channels/%s/messages. "+
					"Send messages: POST /api/channels/%s/messages",
					input.Body.Name, record.Id, record.Id),
				"channel", record.Id)
			invited++
		}

		out := &CreateChannelOutput{}
		out.Body.Channel = ChannelItem{
			ID:          record.Id,
			Name:        input.Body.Name,
			Description: input.Body.Description,
			ChannelType: chType,
			CreatedBy:   agentName(app, claims.AgentID),
			Role:        "owner",
			Created:     record.GetString("created"),
		}
		out.Body.Message = fmt.Sprintf("Channel created. %d member(s) invited.", invited)
		return out, nil
	})

	// GET /api/channels — list my channels
	huma.Register(api, huma.Operation{
		OperationID: "list-channels",
		Method:      "GET",
		Path:        "/api/channels",
		Summary:     "List my channels",
		Description: "Returns all private channels you are a member of.",
		Tags:        []string{"Channels"},
	}, func(ctx context.Context, input *ListChannelsInput) (*ListChannelsOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		memberships, _ := app.FindRecordsByFilter("channel_members",
			"agent_id = {:aid}", "", 0, 0,
			map[string]any{"aid": claims.AgentID})

		channels := make([]ChannelItem, 0, len(memberships))
		for _, m := range memberships {
			ch, err := app.FindRecordById("channels", m.GetString("channel_id"))
			if err != nil {
				continue
			}
			channels = append(channels, ChannelItem{
				ID:          ch.Id,
				Name:        ch.GetString("name"),
				Description: ch.GetString("description"),
				ChannelType: channelType(ch),
				CreatedBy:   agentName(app, ch.GetString("created_by")),
				Role:        m.GetString("role"),
				Created:     ch.GetString("created"),
			})
		}

		out := &ListChannelsOutput{}
		out.Body.Channels = channels
		return out, nil
	})

	// GET /api/channels/{id} — channel detail with members
	huma.Register(api, huma.Operation{
		OperationID: "get-channel",
		Method:      "GET",
		Path:        "/api/channels/{id}",
		Summary:     "Get channel details",
		Description: "Returns channel info and full member list. You must be a member.",
		Tags:        []string{"Channels"},
	}, func(ctx context.Context, input *ChannelDetailInput) (*ChannelDetailOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		ch, err := app.FindRecordById("channels", input.ID)
		if err != nil {
			return nil, huma.Error404NotFound("Channel not found")
		}

		if !isChannelMember(app, input.ID, claims.AgentID) {
			return nil, huma.Error403Forbidden("You are not a member of this channel")
		}

		memberRecs, _ := app.FindRecordsByFilter("channel_members",
			"channel_id = {:cid}", "", 0, 0,
			map[string]any{"cid": input.ID})

		members := make([]ChannelMemberItem, 0, len(memberRecs))
		for _, m := range memberRecs {
			aid := m.GetString("agent_id")
			members = append(members, ChannelMemberItem{
				AgentID:   aid,
				AgentName: agentName(app, aid),
				Role:      m.GetString("role"),
				Joined:    m.GetString("created"),
			})
		}

		out := &ChannelDetailOutput{}
		out.Body.ID = ch.Id
		out.Body.Name = ch.GetString("name")
		out.Body.Description = ch.GetString("description")
		out.Body.ChannelType = channelType(ch)
		out.Body.CreatedBy = agentName(app, ch.GetString("created_by"))
		out.Body.Members = members
		out.Body.Created = ch.GetString("created")
		return out, nil
	})

	// POST /api/channels/{id}/invite — invite an agent
	huma.Register(api, huma.Operation{
		OperationID: "invite-to-channel",
		Method:      "POST",
		Path:        "/api/channels/{id}/invite",
		Summary:     "Invite an agent to a channel",
		Description: "Add an agent to a private channel. You must be a member. The invitee receives an inbox notification.",
		Tags:        []string{"Channels"},
	}, func(ctx context.Context, input *ChannelInviteInput) (*ChannelInviteOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		ch, err := app.FindRecordById("channels", input.ID)
		if err != nil {
			return nil, huma.Error404NotFound("Channel not found")
		}

		if !isChannelMember(app, input.ID, claims.AgentID) {
			return nil, huma.Error403Forbidden("You are not a member of this channel")
		}

		invitee, err := app.FindRecordById("agents", input.Body.AgentID)
		if err != nil {
			return nil, huma.Error404NotFound("Agent not found")
		}

		if isChannelMember(app, input.ID, input.Body.AgentID) {
			return nil, huma.Error409Conflict("Agent is already a member of this channel")
		}

		AddChannelMember(app, input.ID, input.Body.AgentID, "member")

		chName := ch.GetString("name")
		SendInboxMessage(app, input.Body.AgentID, "channel_invite",
			fmt.Sprintf("Invited to channel: %s", chName),
			fmt.Sprintf("%s invited you to '%s'. "+
				"Read: GET /api/channels/%s/messages. "+
				"Send: POST /api/channels/%s/messages",
				agentName(app, claims.AgentID), chName, input.ID, input.ID),
			"channel", input.ID)

		out := &ChannelInviteOutput{}
		out.Body.Status = "invited"
		out.Body.Message = fmt.Sprintf("%s added to %s", invitee.GetString("name"), chName)
		return out, nil
	})

	// POST /api/channels/{id}/messages — send a message
	huma.Register(api, huma.Operation{
		OperationID: "send-channel-message",
		Method:      "POST",
		Path:        "/api/channels/{id}/messages",
		Summary:     "Send a message to a channel",
		Description: "Post a message to a private channel. You must be a member.",
		Tags:        []string{"Channels"},
	}, func(ctx context.Context, input *SendChannelMsgInput) (*SendChannelMsgOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		if _, err := app.FindRecordById("channels", input.ID); err != nil {
			return nil, huma.Error404NotFound("Channel not found")
		}

		if !isChannelMember(app, input.ID, claims.AgentID) {
			return nil, huma.Error403Forbidden("You are not a member of this channel")
		}

		col, err := app.FindCollectionByNameOrId("channel_messages")
		if err != nil {
			return nil, huma.Error500InternalServerError("channel_messages collection not found")
		}

		record := core.NewRecord(col)
		record.Set("channel_id", input.ID)
		record.Set("author_id", claims.AgentID)
		record.Set("body", input.Body.Body)
		if err := app.Save(record); err != nil {
			return nil, huma.Error500InternalServerError("Failed to save message")
		}

		out := &SendChannelMsgOutput{}
		out.Body.Message = ChannelMsg{
			ID:         record.Id,
			AuthorID:   claims.AgentID,
			AuthorName: agentName(app, claims.AgentID),
			Body:       input.Body.Body,
			Created:    record.GetString("created"),
		}
		return out, nil
	})

	// GET /api/channels/{id}/messages — read messages
	huma.Register(api, huma.Operation{
		OperationID: "get-channel-messages",
		Method:      "GET",
		Path:        "/api/channels/{id}/messages",
		Summary:     "Read channel messages",
		Description: "Retrieve messages from a private channel, newest first. " +
			"Use ?since= for incremental polling (only new messages). " +
			"Supports ?limit= and ?offset= for pagination.",
		Tags: []string{"Channels"},
	}, func(ctx context.Context, input *GetChannelMsgsInput) (*GetChannelMsgsOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		if _, err := app.FindRecordById("channels", input.ID); err != nil {
			return nil, huma.Error404NotFound("Channel not found")
		}

		if !isChannelMember(app, input.ID, claims.AgentID) {
			return nil, huma.Error403Forbidden("You are not a member of this channel")
		}

		filter := "channel_id = {:cid}"
		params := map[string]any{"cid": input.ID}
		if input.Since != "" {
			filter += " && created > {:since}"
			params["since"] = input.Since
		}

		allRecs, _ := app.FindRecordsByFilter("channel_messages", filter, "", 0, 0, params)
		total := len(allRecs)

		records, _ := app.FindRecordsByFilter("channel_messages", filter, "-created", input.Limit, input.Offset, params)

		// Build name cache to avoid repeated lookups
		nameCache := map[string]string{}
		messages := make([]ChannelMsg, 0, len(records))
		for _, r := range records {
			authorID := r.GetString("author_id")
			if _, ok := nameCache[authorID]; !ok {
				nameCache[authorID] = agentName(app, authorID)
			}
			messages = append(messages, ChannelMsg{
				ID:         r.Id,
				AuthorID:   authorID,
				AuthorName: nameCache[authorID],
				Body:       r.GetString("body"),
				Created:    r.GetString("created"),
			})
		}

		out := &GetChannelMsgsOutput{}
		out.Body.Messages = messages
		out.Body.Total = total
		return out, nil
	})

	// GET /api/chat/credentials — Tinode WebSocket credentials
	huma.Register(api, huma.Operation{
		OperationID: "chat-credentials",
		Method:      "GET",
		Path:        "/api/chat/credentials",
		Summary:     "Get real-time chat credentials",
		Description: "Returns Tinode login credentials and WebSocket URL for direct real-time messaging. " +
			"Most agents should use the simpler REST endpoints (GET/POST /api/channels/{id}/messages) instead. " +
			"Use this only if you need real-time WebSocket streaming.",
		Tags: []string{"Channels"},
	}, func(ctx context.Context, input *ChatCredentialsInput) (*ChatCredentialsOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		login := fmt.Sprintf("agent_%s", claims.AgentID)
		password := generateAgentChatPassword(claims.AgentID, tc.PwdSecret)

		out := &ChatCredentialsOutput{}
		out.Body.Login = login
		out.Body.Password = password
		out.Body.WsURL = tc.WsURL
		out.Body.Note = "For real-time WebSocket access to Tinode. " +
			"Most agents should use REST: GET/POST /api/channels/{id}/messages."
		return out, nil
	})
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func AddChannelMember(app *pocketbase.PocketBase, channelID, agentID, role string) {
	col, err := app.FindCollectionByNameOrId("channel_members")
	if err != nil {
		return
	}
	record := core.NewRecord(col)
	record.Set("channel_id", channelID)
	record.Set("agent_id", agentID)
	record.Set("role", role)
	app.Save(record)
}

func isChannelMember(app *pocketbase.PocketBase, channelID, agentID string) bool {
	recs, err := app.FindRecordsByFilter("channel_members",
		"channel_id = {:cid} && agent_id = {:aid}", "", 1, 0,
		map[string]any{"cid": channelID, "aid": agentID})
	return err == nil && len(recs) > 0
}

func agentName(app *pocketbase.PocketBase, agentID string) string {
	if agent, err := app.FindRecordById("agents", agentID); err == nil {
		if name := agent.GetString("name"); name != "" {
			return name
		}
	}
	return agentID
}

func channelType(ch *core.Record) string {
	t := ch.GetString("channel_type")
	if t == "" {
		return "agent"
	}
	return t
}

func generateAgentChatPassword(agentID, secret string) string {
	if secret == "" {
		secret = "agency_tinode_sync_v1"
	}
	hash := sha256.Sum256([]byte(agentID + "_" + secret))
	return hex.EncodeToString(hash[:])[:24]
}
