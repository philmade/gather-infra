package tinode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/tinode/chat/pbx"
)

// Client wraps the Tinode gRPC connection
type Client struct {
	conn   *grpc.ClientConn
	stub   pb.NodeClient
	apiKey string

	// Session management
	mu       sync.Mutex
	stream   pb.Node_MessageLoopClient
	msgID    int64
	rootAuth *AuthCredentials
}

// AuthCredentials holds login credentials
type AuthCredentials struct {
	Login    string
	Password string
}

// WorkspaceMetadata holds workspace public data
type WorkspaceMetadata struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	Owner string `json:"owner"`
}

// ChannelMetadata holds channel public data
type ChannelMetadata struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Parent string `json:"parent"`
}

// BotMetadata holds agent bot public data
type BotMetadata struct {
	Fn        string `json:"fn"`
	Bot       bool   `json:"bot"`
	Handle    string `json:"handle"`
	Owner     string `json:"owner"`
	Workspace string `json:"workspace,omitempty"`
}

// NewClient creates a new Tinode gRPC client
func NewClient(addr, apiKey string, rootAuth *AuthCredentials) (*Client, error) {
	conn, err := grpc.Dial(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(10*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Tinode at %s: %w", addr, err)
	}

	return &Client{
		conn:     conn,
		stub:     pb.NewNodeClient(conn),
		apiKey:   apiKey,
		rootAuth: rootAuth,
	}, nil
}

// Close closes the gRPC connection
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stream != nil {
		c.stream.CloseSend()
		c.stream = nil
	}
	return c.conn.Close()
}

// nextMsgID generates the next message ID
func (c *Client) nextMsgID() string {
	c.msgID++
	return fmt.Sprintf("%d", c.msgID)
}

// ensureStream creates a message loop stream if not exists
func (c *Client) ensureStream(ctx context.Context) (pb.Node_MessageLoopClient, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stream != nil {
		return c.stream, nil
	}

	stream, err := c.stub.MessageLoop(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create message loop: %w", err)
	}

	c.stream = stream
	return stream, nil
}

// sendAndReceive sends a client message and waits for response
func (c *Client) sendAndReceive(ctx context.Context, msg *pb.ClientMsg) (*pb.ServerMsg, error) {
	stream, err := c.ensureStream(ctx)
	if err != nil {
		return nil, err
	}

	if err := stream.Send(msg); err != nil {
		// Reset stream on error
		c.mu.Lock()
		c.stream = nil
		c.mu.Unlock()
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	resp, err := stream.Recv()
	if err != nil {
		c.mu.Lock()
		c.stream = nil
		c.mu.Unlock()
		if err == io.EOF {
			return nil, fmt.Errorf("stream closed by server")
		}
		return nil, fmt.Errorf("failed to receive response: %w", err)
	}

	return resp, nil
}

// Hello sends the initial handshake with API key
func (c *Client) Hello(ctx context.Context) error {
	msg := &pb.ClientMsg{
		Message: &pb.ClientMsg_Hi{
			Hi: &pb.ClientHi{
				Id:        c.nextMsgID(),
				UserAgent: "PocketNode/1.0; gRPC",
				Ver:       "0.22",
				Lang:      "en",
			},
		},
	}

	resp, err := c.sendAndReceive(ctx, msg)
	if err != nil {
		return err
	}

	if ctrl := resp.GetCtrl(); ctrl != nil {
		if ctrl.Code >= 200 && ctrl.Code < 300 {
			return nil
		}
		return fmt.Errorf("hello failed: %d %s", ctrl.Code, ctrl.Text)
	}

	return fmt.Errorf("unexpected response to hello")
}

// Login authenticates with basic credentials
func (c *Client) Login(ctx context.Context, login, password string) (string, error) {
	secret := []byte(login + ":" + password)

	msg := &pb.ClientMsg{
		Message: &pb.ClientMsg_Login{
			Login: &pb.ClientLogin{
				Id:     c.nextMsgID(),
				Scheme: "basic",
				Secret: secret,
			},
		},
	}

	resp, err := c.sendAndReceive(ctx, msg)
	if err != nil {
		return "", err
	}

	if ctrl := resp.GetCtrl(); ctrl != nil {
		if ctrl.Code >= 200 && ctrl.Code < 300 {
			// Extract user ID from params
			if params := ctrl.Params; params != nil {
				if uidBytes, ok := params["user"]; ok {
					var uid string
					if err := json.Unmarshal(uidBytes, &uid); err == nil {
						return uid, nil
					}
				}
			}
			return "", nil
		}
		return "", fmt.Errorf("login failed: %d %s", ctrl.Code, ctrl.Text)
	}

	return "", fmt.Errorf("unexpected response to login")
}

// CreateAccount creates a new user account
func (c *Client) CreateAccount(ctx context.Context, login, password, displayName string, isBotUser bool, publicData interface{}) (string, error) {
	secret := []byte(login + ":" + password)

	// Build public data
	var pubBytes []byte
	var err error
	if publicData != nil {
		pubBytes, err = json.Marshal(publicData)
		if err != nil {
			return "", fmt.Errorf("failed to marshal public data: %w", err)
		}
	} else {
		pub := map[string]interface{}{
			"fn": displayName,
		}
		// Set bot flag in public data so clients can identify bots
		if isBotUser {
			pub["bot"] = true
		}
		pubBytes, _ = json.Marshal(pub)
	}

	// Build tags
	tags := []string{}
	if isBotUser {
		tags = append(tags, "bot")
	}

	msg := &pb.ClientMsg{
		Message: &pb.ClientMsg_Acc{
			Acc: &pb.ClientAcc{
				Id:     c.nextMsgID(),
				UserId: "new" + c.apiKey,
				Scheme: "basic",
				Secret: secret,
				Login:  true,
				Tags:   tags,
				Desc: &pb.SetDesc{
					Public: pubBytes,
				},
			},
		},
	}

	resp, err := c.sendAndReceive(ctx, msg)
	if err != nil {
		return "", err
	}

	if ctrl := resp.GetCtrl(); ctrl != nil {
		if ctrl.Code >= 200 && ctrl.Code < 300 {
			// Extract user ID from params
			if params := ctrl.Params; params != nil {
				if uidBytes, ok := params["user"]; ok {
					var uid string
					if err := json.Unmarshal(uidBytes, &uid); err == nil {
						return uid, nil
					}
				}
			}
			return "", nil
		}
		// 409 = already exists
		if ctrl.Code == 409 {
			return "", fmt.Errorf("account already exists")
		}
		return "", fmt.Errorf("create account failed: %d %s", ctrl.Code, ctrl.Text)
	}

	return "", fmt.Errorf("unexpected response to account creation")
}

// EnsureUser creates a Tinode user if it doesn't exist, returns user ID
// This is for regular (non-bot) users synced from PocketBase
func (c *Client) EnsureUser(ctx context.Context, login, password, displayName string) (string, error) {
	// Reset stream for fresh connection
	c.mu.Lock()
	if c.stream != nil {
		c.stream.CloseSend()
		c.stream = nil
	}
	c.mu.Unlock()

	// Always send Hello first (Tinode protocol requirement)
	if err := c.Hello(ctx); err != nil {
		return "", fmt.Errorf("hello failed: %w", err)
	}

	// Try to login first
	uid, err := c.Login(ctx, login, password)
	if err == nil {
		// User exists, return their ID
		return uid, nil
	}

	// If login failed, try to create account
	// Need fresh stream for account creation
	c.mu.Lock()
	if c.stream != nil {
		c.stream.CloseSend()
		c.stream = nil
	}
	c.mu.Unlock()

	// Send hello again
	if err := c.Hello(ctx); err != nil {
		return "", fmt.Errorf("hello failed: %w", err)
	}

	// Create account (as regular user, NOT a bot)
	uid, err = c.CreateAccount(ctx, login, password, displayName, false, nil)
	if err != nil {
		// Account might already exist (race condition)
		if err.Error() == "account already exists" {
			// Reset and try login again
			c.mu.Lock()
			if c.stream != nil {
				c.stream.CloseSend()
				c.stream = nil
			}
			c.mu.Unlock()

			if err := c.Hello(ctx); err != nil {
				return "", err
			}
			return c.Login(ctx, login, password)
		}
		return "", fmt.Errorf("create account failed: %w", err)
	}

	return uid, nil
}

// updateBotMetadata updates the current user's public data to include bot=true
func (c *Client) updateBotMetadata(ctx context.Context, displayName string) error {
	pub := map[string]interface{}{
		"fn":  displayName,
		"bot": true,
	}
	pubBytes, _ := json.Marshal(pub)

	msg := &pb.ClientMsg{
		Message: &pb.ClientMsg_Set{
			Set: &pb.ClientSet{
				Id:    c.nextMsgID(),
				Topic: "me",
				Query: &pb.SetQuery{
					Desc: &pb.SetDesc{
						Public: pubBytes,
					},
				},
			},
		},
	}

	resp, err := c.sendAndReceive(ctx, msg)
	if err != nil {
		return err
	}

	if ctrl := resp.GetCtrl(); ctrl != nil {
		if ctrl.Code >= 200 && ctrl.Code < 300 {
			return nil
		}
		return fmt.Errorf("set failed: %d %s", ctrl.Code, ctrl.Text)
	}

	return nil
}

// CreateGroup creates a new group topic
func (c *Client) CreateGroup(ctx context.Context, name string, publicData interface{}, tags []string) (string, error) {
	pubBytes, err := json.Marshal(publicData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal public data: %w", err)
	}

	msg := &pb.ClientMsg{
		Message: &pb.ClientMsg_Sub{
			Sub: &pb.ClientSub{
				Id:    c.nextMsgID(),
				Topic: "new",
				SetQuery: &pb.SetQuery{
					Desc: &pb.SetDesc{
						Public: pubBytes,
					},
				},
			},
		},
	}

	resp, err := c.sendAndReceive(ctx, msg)
	if err != nil {
		return "", err
	}

	if ctrl := resp.GetCtrl(); ctrl != nil {
		if ctrl.Code >= 200 && ctrl.Code < 300 {
			// Extract topic from params
			if params := ctrl.Params; params != nil {
				if topicBytes, ok := params["topic"]; ok {
					var topic string
					if err := json.Unmarshal(topicBytes, &topic); err == nil {
						return topic, nil
					}
				}
			}
			return "", nil
		}
		return "", fmt.Errorf("create group failed: %d %s", ctrl.Code, ctrl.Text)
	}

	return "", fmt.Errorf("unexpected response to group creation")
}

// CreateWorkspace creates a grp topic with workspace metadata
func (c *Client) CreateWorkspace(ctx context.Context, name, slug, ownerUID string) (string, error) {
	meta := WorkspaceMetadata{
		Type:  "workspace",
		Name:  name,
		Slug:  slug,
		Owner: ownerUID,
	}

	tags := []string{
		"workspace",
		fmt.Sprintf("slug:%s", slug),
	}

	return c.CreateGroup(ctx, name, meta, tags)
}

// CreateChannel creates a grp topic with channel metadata
func (c *Client) CreateChannel(ctx context.Context, name, parentWorkspace string) (string, error) {
	meta := ChannelMetadata{
		Type:   "channel",
		Name:   name,
		Parent: parentWorkspace,
	}

	tags := []string{
		"channel",
		fmt.Sprintf("parent:%s", parentWorkspace),
	}

	return c.CreateGroup(ctx, name, meta, tags)
}

// CreateBot creates a bot user account
func (c *Client) CreateBot(ctx context.Context, login, password, displayName, handle, ownerUID, workspace string) (string, error) {
	meta := BotMetadata{
		Fn:        displayName,
		Bot:       true,
		Handle:    handle,
		Owner:     ownerUID,
		Workspace: workspace,
	}

	// Need fresh stream
	c.mu.Lock()
	if c.stream != nil {
		c.stream.CloseSend()
		c.stream = nil
	}
	c.mu.Unlock()

	if err := c.Hello(ctx); err != nil {
		return "", err
	}

	return c.CreateAccount(ctx, login, password, displayName, true, meta)
}

// EnsureBotUser creates a bot user if it doesn't exist, returns user ID
// Sets proper bot metadata including handle for @mentions
func (c *Client) EnsureBotUser(ctx context.Context, login, password, displayName, handle string) (string, error) {
	// Reset stream for fresh connection
	c.mu.Lock()
	if c.stream != nil {
		c.stream.CloseSend()
		c.stream = nil
	}
	c.mu.Unlock()

	// Always send Hello first (Tinode protocol requirement)
	if err := c.Hello(ctx); err != nil {
		return "", fmt.Errorf("hello failed: %w", err)
	}

	// Try to login first
	uid, err := c.Login(ctx, login, password)
	if err == nil {
		// User exists - ensure bot metadata is set with handle
		if updateErr := c.updateBotMetadataWithHandle(ctx, displayName, handle); updateErr != nil {
			log.Printf("Warning: could not update bot metadata: %v", updateErr)
		}
		return uid, nil
	}

	// If login failed, try to create account
	c.mu.Lock()
	if c.stream != nil {
		c.stream.CloseSend()
		c.stream = nil
	}
	c.mu.Unlock()

	if err := c.Hello(ctx); err != nil {
		return "", fmt.Errorf("hello failed: %w", err)
	}

	// Create bot account with proper metadata
	meta := BotMetadata{
		Fn:     displayName,
		Bot:    true,
		Handle: handle,
	}

	uid, err = c.CreateAccount(ctx, login, password, displayName, true, meta)
	if err != nil {
		if err.Error() == "account already exists" {
			// Reset and try login again
			c.mu.Lock()
			if c.stream != nil {
				c.stream.CloseSend()
				c.stream = nil
			}
			c.mu.Unlock()

			if err := c.Hello(ctx); err != nil {
				return "", err
			}
			return c.Login(ctx, login, password)
		}
		return "", fmt.Errorf("create account failed: %w", err)
	}

	return uid, nil
}

// updateBotMetadataWithHandle updates the current user's public data with bot flag and handle
func (c *Client) updateBotMetadataWithHandle(ctx context.Context, displayName, handle string) error {
	pub := map[string]interface{}{
		"fn":     displayName,
		"bot":    true,
		"handle": handle,
	}
	pubBytes, _ := json.Marshal(pub)

	msg := &pb.ClientMsg{
		Message: &pb.ClientMsg_Set{
			Set: &pb.ClientSet{
				Id:    c.nextMsgID(),
				Topic: "me",
				Query: &pb.SetQuery{
					Desc: &pb.SetDesc{
						Public: pubBytes,
					},
				},
			},
		},
	}

	resp, err := c.sendAndReceive(ctx, msg)
	if err != nil {
		return err
	}

	if ctrl := resp.GetCtrl(); ctrl != nil {
		if ctrl.Code >= 200 && ctrl.Code < 400 {
			return nil
		}
		return fmt.Errorf("set failed: %d %s", ctrl.Code, ctrl.Text)
	}

	return nil
}

// InviteUserToTopic invites a different user (by Tinode UID) to a group topic.
// The current session user must have admin (A) access on the topic.
// mode is the access mode string, e.g. "JRWPS".
func (c *Client) InviteUserToTopic(ctx context.Context, topic, targetUID, mode string) error {
	msg := &pb.ClientMsg{
		Message: &pb.ClientMsg_Sub{
			Sub: &pb.ClientSub{
				Id:    c.nextMsgID(),
				Topic: topic,
				SetQuery: &pb.SetQuery{
					Sub: &pb.SetSub{
						UserId: targetUID,
						Mode:   mode,
					},
				},
			},
		},
	}

	resp, err := c.sendAndReceive(ctx, msg)
	if err != nil {
		return err
	}

	if ctrl := resp.GetCtrl(); ctrl != nil {
		if ctrl.Code >= 200 && ctrl.Code < 400 {
			// 2xx = success, 304 = already subscribed — both are fine
			return nil
		}
		return fmt.Errorf("invite to topic failed: %d %s", ctrl.Code, ctrl.Text)
	}

	return fmt.Errorf("unexpected response to invite")
}

// Subscribe subscribes a user to a topic
func (c *Client) Subscribe(ctx context.Context, topic string) error {
	msg := &pb.ClientMsg{
		Message: &pb.ClientMsg_Sub{
			Sub: &pb.ClientSub{
				Id:    c.nextMsgID(),
				Topic: topic,
			},
		},
	}

	resp, err := c.sendAndReceive(ctx, msg)
	if err != nil {
		return err
	}

	if ctrl := resp.GetCtrl(); ctrl != nil {
		if ctrl.Code >= 200 && ctrl.Code < 400 {
			// 2xx = success, 304 = already subscribed — both are fine
			return nil
		}
		return fmt.Errorf("subscribe failed: %d %s", ctrl.Code, ctrl.Text)
	}

	return fmt.Errorf("unexpected response to subscribe")
}

// AddUserToWorkspace adds a user to a workspace group
func (c *Client) AddUserToWorkspace(ctx context.Context, userUID, workspaceTopic string) error {
	// This requires admin permissions on the topic
	// The user creating the subscription must have 'A' access
	return c.Subscribe(ctx, workspaceTopic)
}

// GetWorkspaceChannels returns the channel topic IDs for a workspace
// by querying the fnd topic for channels with parent:<workspaceID>
func (c *Client) GetWorkspaceChannels(ctx context.Context, workspaceID string) ([]string, error) {
	// Subscribe to fnd topic for search
	fndMsg := &pb.ClientMsg{
		Message: &pb.ClientMsg_Sub{
			Sub: &pb.ClientSub{
				Id:    c.nextMsgID(),
				Topic: "fnd",
			},
		},
	}

	resp, err := c.sendAndReceive(ctx, fndMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to fnd: %w", err)
	}

	if ctrl := resp.GetCtrl(); ctrl != nil && ctrl.Code >= 300 {
		return nil, fmt.Errorf("subscribe to fnd failed: %d %s", ctrl.Code, ctrl.Text)
	}

	// Search for channels with parent tag
	searchQuery := fmt.Sprintf("parent:%s", workspaceID)
	setMsg := &pb.ClientMsg{
		Message: &pb.ClientMsg_Set{
			Set: &pb.ClientSet{
				Id:    c.nextMsgID(),
				Topic: "fnd",
				Query: &pb.SetQuery{
					Desc: &pb.SetDesc{
						Public: []byte(fmt.Sprintf(`"%s"`, searchQuery)),
					},
				},
			},
		},
	}

	resp, err = c.sendAndReceive(ctx, setMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to set search query: %w", err)
	}

	// Get search results
	getMsg := &pb.ClientMsg{
		Message: &pb.ClientMsg_Get{
			Get: &pb.ClientGet{
				Id:    c.nextMsgID(),
				Topic: "fnd",
				Query: &pb.GetQuery{
					Sub: &pb.GetOpts{},
				},
			},
		},
	}

	channels := []string{}

	// Send get and collect meta responses
	stream, err := c.ensureStream(ctx)
	if err != nil {
		return nil, err
	}

	if err := stream.Send(getMsg); err != nil {
		return nil, fmt.Errorf("failed to send get: %w", err)
	}

	// Read responses until we get ctrl
	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}

		if meta := resp.GetMeta(); meta != nil {
			for _, sub := range meta.Sub {
				if sub.Topic != "" {
					channels = append(channels, sub.Topic)
				}
			}
		}

		if ctrl := resp.GetCtrl(); ctrl != nil {
			break
		}
	}

	// Leave fnd topic
	leaveMsg := &pb.ClientMsg{
		Message: &pb.ClientMsg_Leave{
			Leave: &pb.ClientLeave{
				Id:    c.nextMsgID(),
				Topic: "fnd",
			},
		},
	}
	c.sendAndReceive(ctx, leaveMsg)

	return channels, nil
}
