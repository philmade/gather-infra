package connectors

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// MatterbridgeConnector handles bidirectional messaging via Matterbridge + ADK API.
type MatterbridgeConnector struct {
	mbURL         string // Matterbridge API
	adkURL        string // ADK API
	appName       string
	gateway       string
	botName       string
	telegramToken string // For typing indicators
	httpClient    *http.Client
	sessions      map[string]string // userID -> sessionID
	mu            sync.Mutex
}

// MBMessage represents a Matterbridge message.
type MBMessage struct {
	Text     string `json:"text"`
	Username string `json:"username"`
	UserID   string `json:"userid"`
	Channel  string `json:"channel"`
	Protocol string `json:"protocol"`
	Event    string `json:"event"`
	Gateway  string `json:"gateway"`
}

// NewMatterbridgeConnector creates a new connector.
func NewMatterbridgeConnector(adkURL string) *MatterbridgeConnector {
	if adkURL == "" {
		adkURL = "http://127.0.0.1:8080"
	}
	return &MatterbridgeConnector{
		mbURL:         "http://localhost:4242",
		adkURL:        adkURL,
		appName:       "clawpoint",
		gateway:       "clawpoint",
		botName:       "ClawPoint-Go",
		telegramToken: os.Getenv("TELEGRAM_BOT"),
		httpClient:    &http.Client{Timeout: 120 * time.Second},
		sessions:      make(map[string]string),
	}
}

// Start begins streaming messages from Matterbridge and routing to ADK.
func (m *MatterbridgeConnector) Start(ctx context.Context) error {
	fmt.Println("Matterbridge connector starting...")
	fmt.Printf("  Matterbridge: %s\n", m.mbURL)
	fmt.Printf("  ADK API:      %s\n", m.adkURL)
	fmt.Printf("  App:          %s\n", m.appName)

	msgChan := make(chan MBMessage, 100)
	go m.readStream(ctx, msgChan)

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg := <-msgChan:
			if msg.Text == "" {
				continue
			}

			fmt.Printf("[%s] %s: %s\n", msg.Protocol, msg.Username, truncateStr(msg.Text, 80))

			response, err := m.routeToADK(ctx, msg)
			if err != nil {
				fmt.Printf("  error: %v\n", err)
				continue
			}

			if response != "" {
				if err := m.SendMessage(response); err != nil {
					fmt.Printf("  send failed: %v\n", err)
				} else {
					fmt.Printf("  -> sent %d chars\n", len(response))
				}
			}
		}
	}
}

// readStream continuously reads from Matterbridge /api/stream.
func (m *MatterbridgeConnector) readStream(ctx context.Context, msgChan chan<- MBMessage) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		resp, err := http.Get(m.mbURL + "/api/stream")
		if err != nil {
			fmt.Printf("stream connect error: %v, retrying in 5s...\n", err)
			time.Sleep(5 * time.Second)
			continue
		}

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			var msg MBMessage
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				continue
			}

			if msg.Event == "api_connected" {
				fmt.Println("connected to Matterbridge stream")
				continue
			}

			if msg.Username == m.botName {
				continue
			}

			msgChan <- msg
		}

		resp.Body.Close()
		fmt.Println("stream disconnected, reconnecting in 2s...")
		time.Sleep(2 * time.Second)
	}
}

// sendTypingIndicator sends a "typing..." action to Telegram for the given chat.
func (m *MatterbridgeConnector) sendTypingIndicator(chatID string) {
	if m.telegramToken == "" {
		return
	}
	payload, _ := json.Marshal(map[string]string{
		"chat_id": chatID,
		"action":  "typing",
	})
	resp, err := m.httpClient.Post(
		"https://api.telegram.org/bot"+m.telegramToken+"/sendChatAction",
		"application/json",
		bytes.NewReader(payload),
	)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// startTypingLoop sends typing indicators every 4s until the context is cancelled.
func (m *MatterbridgeConnector) startTypingLoop(chatID string) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		m.sendTypingIndicator(chatID)
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.sendTypingIndicator(chatID)
			}
		}
	}()
	return cancel
}

// routeToADK sends a message to the ADK agent and returns the response.
func (m *MatterbridgeConnector) routeToADK(ctx context.Context, msg MBMessage) (string, error) {
	userID := msg.UserID
	if userID == "" {
		userID = msg.Username
	}

	sessionID, err := m.getOrCreateSession(userID)
	if err != nil {
		return "", fmt.Errorf("session: %w", err)
	}

	stopTyping := m.startTypingLoop(msg.Channel)
	defer stopTyping()

	proto := msg.Protocol
	if proto == "" {
		proto = "chat"
	}
	text := fmt.Sprintf("MESSAGE from %s via %s:\n%s", msg.Username, proto, msg.Text)

	return m.sendRunSSE(ctx, userID, sessionID, text)
}

// getOrCreateSession gets an existing session or creates one.
func (m *MatterbridgeConnector) getOrCreateSession(userID string) (string, error) {
	m.mu.Lock()
	if sid, ok := m.sessions[userID]; ok {
		m.mu.Unlock()
		return sid, nil
	}
	m.mu.Unlock()

	url := fmt.Sprintf("%s/api/apps/%s/users/%s/sessions", m.adkURL, m.appName, userID)
	resp, err := m.httpClient.Post(url, "application/json", strings.NewReader("{}"))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("create session HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse session response: %w", err)
	}

	sessionID, ok := result["id"].(string)
	if !ok {
		return "", fmt.Errorf("no session id in response: %s", string(body))
	}

	m.mu.Lock()
	m.sessions[userID] = sessionID
	m.mu.Unlock()

	fmt.Printf("  session created: %s (user: %s)\n", sessionID[:8], userID)
	return sessionID, nil
}

// sendRunSSE sends a message via the ADK run_sse endpoint and collects the response.
func (m *MatterbridgeConnector) sendRunSSE(ctx context.Context, userID, sessionID, text string) (string, error) {
	payload := map[string]any{
		"appName":   m.appName,
		"userId":    userID,
		"sessionId": sessionID,
		"newMessage": map[string]any{
			"role": "user",
			"parts": []map[string]any{
				{"text": text},
			},
		},
	}

	jsonPayload, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", m.adkURL+"/api/run_sse", bytes.NewReader(jsonPayload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("run_sse HTTP %d: %s", resp.StatusCode, string(body))
	}

	return m.parseSSEResponse(resp.Body)
}

// parseSSEResponse reads SSE events and extracts the agent's text response.
func (m *MatterbridgeConnector) parseSSEResponse(r io.Reader) (string, error) {
	scanner := bufio.NewScanner(r)
	var lastText string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "Error while running agent:") {
			return "", fmt.Errorf("%s", line)
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := line[6:]
		var event map[string]any
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		content, ok := event["content"].(map[string]any)
		if !ok {
			continue
		}

		parts, ok := content["parts"].([]any)
		if !ok {
			continue
		}

		for _, p := range parts {
			part, ok := p.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := part["text"].(string); ok && text != "" {
				lastText = text
			}
		}
	}

	return lastText, nil
}

// SendMessage sends a message back to Matterbridge.
func (m *MatterbridgeConnector) SendMessage(text string) error {
	payload := map[string]string{
		"text":     text,
		"username": m.botName,
		"gateway":  m.gateway,
	}

	jsonPayload, _ := json.Marshal(payload)

	resp, err := m.httpClient.Post(
		m.mbURL+"/api/message",
		"application/json",
		bytes.NewReader(jsonPayload),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
