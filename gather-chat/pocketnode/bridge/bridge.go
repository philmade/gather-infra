// Package bridge handles the Tinode message bridge for agent lifecycle events
//
// This is the "app" - all business logic for agent management flows through here.
// The SDK just sends events, this bridge processes them.
//
// Events:
//   - agent:ready   - Agent connected, ensure it's set up properly
//   - agent:leaving - Agent disconnecting, cleanup if needed
//
// Responsibilities:
//   - Create/update bot users with proper metadata (name, handle, bot=true)
//   - Subscribe bots to workspace and ALL channels
//   - Handle new channel creation (add all agents)
//   - Route @mentions if needed (or let SDK handle natively)
package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/tinode/chat/pbx"
)

// AgentEvent represents an event from the SDK
type AgentEvent struct {
	Type      string `json:"type"`      // "agent:ready", "agent:leaving"
	Handle    string `json:"handle"`    // Agent handle (e.g., "hello_agent")
	Name      string `json:"name"`      // Display name
	Workspace string `json:"workspace"` // Workspace topic ID
	BotUID    string `json:"bot_uid"`   // Bot's Tinode user ID
}

// RegisteredAgent tracks an active agent
type RegisteredAgent struct {
	Handle    string
	Name      string
	Workspace string
	BotUID    string
	BotLogin  string
	Channels  []string // Channels the bot is subscribed to
}

// Bridge manages the connection to Tinode and processes events
type Bridge struct {
	conn     *grpc.ClientConn
	stub     pb.NodeClient
	apiKey   string

	// Service account for administrative operations
	serviceLogin    string
	servicePassword string

	// Active agents
	mu     sync.RWMutex
	agents map[string]*RegisteredAgent // handle -> agent

	// Message stream
	stream pb.Node_MessageLoopClient
	msgID  int64

	// Callbacks
	onAgentReady func(agent *RegisteredAgent)
}

// NewBridge creates a new Tinode bridge
func NewBridge(addr, apiKey, serviceLogin, servicePassword string) (*Bridge, error) {
	conn, err := grpc.Dial(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(10*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Tinode at %s: %w", addr, err)
	}

	return &Bridge{
		conn:            conn,
		stub:            pb.NewNodeClient(conn),
		apiKey:          apiKey,
		serviceLogin:    serviceLogin,
		servicePassword: servicePassword,
		agents:          make(map[string]*RegisteredAgent),
	}, nil
}

// Close closes the bridge connection
func (b *Bridge) Close() error {
	if b.stream != nil {
		b.stream.CloseSend()
	}
	return b.conn.Close()
}

// nextMsgID generates the next message ID
func (b *Bridge) nextMsgID() string {
	b.msgID++
	return fmt.Sprintf("bridge_%d", b.msgID)
}

// Start connects to Tinode and starts processing events
func (b *Bridge) Start(ctx context.Context) error {
	log.Println("[Bridge] Starting Tinode message bridge...")

	// Create message stream
	stream, err := b.stub.MessageLoop(ctx)
	if err != nil {
		return fmt.Errorf("failed to create message loop: %w", err)
	}
	b.stream = stream

	// Send hello
	if err := b.hello(); err != nil {
		return fmt.Errorf("hello failed: %w", err)
	}

	// Login as service account
	if err := b.login(); err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	log.Println("[Bridge] Connected to Tinode as service account")

	// Start message processing loop
	go b.processMessages(ctx)

	return nil
}

func (b *Bridge) hello() error {
	msg := &pb.ClientMsg{
		Message: &pb.ClientMsg_Hi{
			Hi: &pb.ClientHi{
				Id:        b.nextMsgID(),
				UserAgent: "PocketNode-Bridge/1.0",
				Ver:       "0.22",
				Lang:      "en",
			},
		},
	}

	if err := b.stream.Send(msg); err != nil {
		return err
	}

	resp, err := b.stream.Recv()
	if err != nil {
		return err
	}

	if ctrl := resp.GetCtrl(); ctrl != nil && ctrl.Code >= 300 {
		return fmt.Errorf("hello failed: %d %s", ctrl.Code, ctrl.Text)
	}

	return nil
}

func (b *Bridge) login() error {
	secret := []byte(b.serviceLogin + ":" + b.servicePassword)

	msg := &pb.ClientMsg{
		Message: &pb.ClientMsg_Login{
			Login: &pb.ClientLogin{
				Id:     b.nextMsgID(),
				Scheme: "basic",
				Secret: secret,
			},
		},
	}

	if err := b.stream.Send(msg); err != nil {
		return err
	}

	resp, err := b.stream.Recv()
	if err != nil {
		return err
	}

	if ctrl := resp.GetCtrl(); ctrl != nil && ctrl.Code >= 300 {
		return fmt.Errorf("login failed: %d %s", ctrl.Code, ctrl.Text)
	}

	return nil
}

// processMessages is the main event loop
func (b *Bridge) processMessages(ctx context.Context) {
	log.Println("[Bridge] Starting message processing loop...")

	for {
		select {
		case <-ctx.Done():
			log.Println("[Bridge] Context cancelled, stopping")
			return
		default:
		}

		resp, err := b.stream.Recv()
		if err != nil {
			if err == io.EOF {
				log.Println("[Bridge] Stream closed")
				return
			}
			log.Printf("[Bridge] Error receiving message: %v", err)
			continue
		}

		// Process different message types
		if data := resp.GetData(); data != nil {
			b.handleDataMessage(data)
		}

		if pres := resp.GetPres(); pres != nil {
			b.handlePresence(pres)
		}
	}
}

// handleDataMessage processes incoming chat messages
func (b *Bridge) handleDataMessage(data *pb.ServerData) {
	topic := data.Topic
	content := string(data.Content)

	// Check if this is an agent event (special JSON format)
	if strings.HasPrefix(content, "{\"type\":\"agent:") {
		var event AgentEvent
		if err := json.Unmarshal(data.Content, &event); err == nil {
			b.handleAgentEvent(&event)
			return
		}
	}

	// Regular message - check for @mentions
	// (SDK handles this natively since it's subscribed to topics)
	log.Printf("[Bridge] Message in %s: %s", topic, content[:min(50, len(content))])
}

// handleAgentEvent processes agent lifecycle events
func (b *Bridge) handleAgentEvent(event *AgentEvent) {
	log.Printf("[Bridge] Agent event: %s from @%s", event.Type, event.Handle)

	switch event.Type {
	case "agent:ready":
		b.onAgentReadyEvent(event)
	case "agent:leaving":
		b.onAgentLeavingEvent(event)
	default:
		log.Printf("[Bridge] Unknown event type: %s", event.Type)
	}
}

// onAgentReadyEvent handles an agent connecting
func (b *Bridge) onAgentReadyEvent(event *AgentEvent) {
	log.Printf("[Bridge] Agent @%s is ready in workspace %s", event.Handle, event.Workspace)

	// Register the agent
	agent := &RegisteredAgent{
		Handle:    event.Handle,
		Name:      event.Name,
		Workspace: event.Workspace,
		BotUID:    event.BotUID,
	}

	b.mu.Lock()
	b.agents[event.Handle] = agent
	b.mu.Unlock()

	// Ensure agent is properly set up
	go b.setupAgent(agent)
}

// onAgentLeavingEvent handles an agent disconnecting
func (b *Bridge) onAgentLeavingEvent(event *AgentEvent) {
	log.Printf("[Bridge] Agent @%s is leaving", event.Handle)

	b.mu.Lock()
	delete(b.agents, event.Handle)
	b.mu.Unlock()
}

// setupAgent ensures the agent is properly configured in Tinode
func (b *Bridge) setupAgent(agent *RegisteredAgent) {
	ctx := context.Background()

	log.Printf("[Bridge] Setting up agent @%s...", agent.Handle)

	// 1. Update bot user metadata (name, handle, bot=true)
	if err := b.updateBotMetadata(ctx, agent); err != nil {
		log.Printf("[Bridge] Failed to update bot metadata: %v", err)
	}

	// 2. Get all channels in the workspace
	channels, err := b.getWorkspaceChannels(ctx, agent.Workspace)
	if err != nil {
		log.Printf("[Bridge] Failed to get workspace channels: %v", err)
	} else {
		log.Printf("[Bridge] Found %d channel(s) in workspace", len(channels))
	}

	// 3. Subscribe bot to each channel
	for _, channel := range channels {
		if err := b.subscribeBotToChannel(ctx, agent, channel); err != nil {
			log.Printf("[Bridge] Failed to subscribe bot to %s: %v", channel, err)
		} else {
			log.Printf("[Bridge] Subscribed @%s to channel %s", agent.Handle, channel)
			agent.Channels = append(agent.Channels, channel)
		}
	}

	log.Printf("[Bridge] Agent @%s setup complete", agent.Handle)

	// Callback
	if b.onAgentReady != nil {
		b.onAgentReady(agent)
	}
}

// updateBotMetadata updates the bot's public data in Tinode
func (b *Bridge) updateBotMetadata(ctx context.Context, agent *RegisteredAgent) error {
	// This needs to be done as the bot user, not service account
	// For now, we'll rely on the SDK to do this after receiving confirmation
	// TODO: Implement admin-level metadata update
	log.Printf("[Bridge] TODO: Update metadata for @%s", agent.Handle)
	return nil
}

// getWorkspaceChannels returns all channels in a workspace
func (b *Bridge) getWorkspaceChannels(ctx context.Context, workspaceID string) ([]string, error) {
	// Subscribe to the workspace to get its metadata
	subMsg := &pb.ClientMsg{
		Message: &pb.ClientMsg_Sub{
			Sub: &pb.ClientSub{
				Id:    b.nextMsgID(),
				Topic: workspaceID,
				GetQuery: &pb.GetQuery{
					Sub: &pb.GetOpts{},
				},
			},
		},
	}

	if err := b.stream.Send(subMsg); err != nil {
		return nil, err
	}

	// Collect channels from subscription responses
	channels := []string{}

	// Read responses until we get ctrl
	for i := 0; i < 10; i++ {
		resp, err := b.stream.Recv()
		if err != nil {
			break
		}

		if meta := resp.GetMeta(); meta != nil {
			// Check each subscription for channel type
			for _, sub := range meta.Sub {
				if sub.Topic != "" && strings.HasPrefix(sub.Topic, "grp") {
					// Check if it's a channel (has parent = this workspace)
					var pubData map[string]interface{}
					if err := json.Unmarshal(sub.Public, &pubData); err == nil {
						if pubData["type"] == "channel" && pubData["parent"] == workspaceID {
							channels = append(channels, sub.Topic)
						}
					}
				}
			}
		}

		if ctrl := resp.GetCtrl(); ctrl != nil {
			break
		}
	}

	return channels, nil
}

// subscribeBotToChannel subscribes a bot to a channel
// This needs admin privileges on the channel
func (b *Bridge) subscribeBotToChannel(ctx context.Context, agent *RegisteredAgent, channelID string) error {
	// Use set message to add the bot as a subscriber
	// This requires the service account to have admin rights on the channel

	// For now, we'll send a subscription request as the service account
	// and hope it has rights to add members

	// TODO: Implement proper admin subscription
	log.Printf("[Bridge] TODO: Subscribe %s to %s", agent.BotUID, channelID)
	return nil
}

// handlePresence processes presence notifications
func (b *Bridge) handlePresence(pres *pb.ServerPres) {
	// Handle new channel creation - add all agents
	if pres.What == pb.ServerPres_TERM {
		// Topic deleted
		log.Printf("[Bridge] Topic %s deleted", pres.Topic)
	}

	// TODO: Handle new channel creation and add all workspace agents
}

// GetAgent returns a registered agent by handle
func (b *Bridge) GetAgent(handle string) *RegisteredAgent {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.agents[handle]
}

// GetAgents returns all registered agents
func (b *Bridge) GetAgents() []*RegisteredAgent {
	b.mu.RLock()
	defer b.mu.RUnlock()

	agents := make([]*RegisteredAgent, 0, len(b.agents))
	for _, agent := range b.agents {
		agents = append(agents, agent)
	}
	return agents
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
