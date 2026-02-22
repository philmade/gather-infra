package connectors

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
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
	middleware    *Middleware // Token estimation + compaction pipeline
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
		middleware:    NewMiddleware(adkURL),
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
				// Send a friendly error message back instead of silent failure
				if friendly := friendlyError(err); friendly != "" {
					if sendErr := m.SendMessage(friendly); sendErr != nil {
						fmt.Printf("  error reply failed: %v\n", sendErr)
					}
				}
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

// routeToADK sends a message to the ADK agent via the middleware pipeline.
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

	text := msg.Text

	// Route through middleware (token estimation + compaction + ADK call)
	result, err := m.middleware.ProcessMessage(ctx, userID, sessionID, text)
	if err != nil {
		// Session lost (ADK restart / hot-swap) — invalidate cache and retry once
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
			fmt.Printf("  session %s lost, creating new one\n", truncateStr(sessionID, 8))
			m.mu.Lock()
			delete(m.sessions, userID)
			m.mu.Unlock()

			newSessionID, createErr := m.getOrCreateSession(userID)
			if createErr != nil {
				return "", fmt.Errorf("retry session: %w", createErr)
			}

			result, err = m.middleware.ProcessMessage(ctx, userID, newSessionID, text)
			if err != nil {
				return "", err
			}
			sessionID = newSessionID
		} else {
			return "", err
		}
	}

	// If middleware compacted and created a new session, update our mapping
	if result.SessionID != sessionID {
		m.mu.Lock()
		m.sessions[userID] = result.SessionID
		m.mu.Unlock()
		fmt.Printf("  session updated: %s → %s (compacted)\n", truncateStr(sessionID, 8), truncateStr(result.SessionID, 8))
	}

	return result.Text, nil
}

// getOrCreateSession finds the most recent existing session for this user,
// or creates a new one if none exist. Sessions are in-memory — on restart,
// a new session is created and heartbeat injection restores continuity.
func (m *MatterbridgeConnector) getOrCreateSession(userID string) (string, error) {
	m.mu.Lock()
	if sid, ok := m.sessions[userID]; ok {
		m.mu.Unlock()
		return sid, nil
	}
	m.mu.Unlock()

	// Try to find an existing session for this user
	listURL := fmt.Sprintf("%s/api/apps/%s/users/%s/sessions", m.adkURL, m.appName, userID)
	resp, err := m.httpClient.Get(listURL)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			body, _ := io.ReadAll(resp.Body)
			var sessions []map[string]any
			if err := json.Unmarshal(body, &sessions); err == nil && len(sessions) > 0 {
				// Use the most recently updated session
				var bestID string
				var bestTime string
				for _, s := range sessions {
					sid, _ := s["id"].(string)
					updated, _ := s["lastUpdateTime"].(string)
					if sid != "" && updated >= bestTime {
						bestID = sid
						bestTime = updated
					}
				}
				if bestID != "" {
					m.mu.Lock()
					m.sessions[userID] = bestID
					m.mu.Unlock()
					fmt.Printf("  session resumed: %s (user: %s)\n", bestID[:8], userID)
					return bestID, nil
				}
			}
		}
	}

	// No existing session — create a new one
	createURL := fmt.Sprintf("%s/api/apps/%s/users/%s/sessions", m.adkURL, m.appName, userID)
	resp2, err := m.httpClient.Post(createURL, "application/json", strings.NewReader("{}"))
	if err != nil {
		return "", err
	}
	defer resp2.Body.Close()

	body, _ := io.ReadAll(resp2.Body)
	if resp2.StatusCode != 200 {
		return "", fmt.Errorf("create session HTTP %d: %s", resp2.StatusCode, string(body))
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

// StartHeartbeat runs an internal heartbeat loop. The agent controls its own
// wake-up interval via NEXT_HEARTBEAT: directives in its responses.
func (m *MatterbridgeConnector) StartHeartbeat(ctx context.Context) {
	intervalStr := os.Getenv("HEARTBEAT_INTERVAL")
	if intervalStr == "" {
		intervalStr = "15m"
	}
	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		log.Printf("heartbeat: invalid HEARTBEAT_INTERVAL %q, using 15m", intervalStr)
		interval = 15 * time.Minute
	}
	if interval <= 0 {
		log.Printf("heartbeat: disabled (HEARTBEAT_INTERVAL=0)")
		return
	}
	interval = clampHeartbeatInterval(interval)

	log.Printf("heartbeat: starting with interval %s", interval)

	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("heartbeat: stopped")
			return
		case <-timer.C:
			log.Printf("heartbeat: tick")

			response, err := m.routeToADK(ctx, MBMessage{
				Text:     "[HEARTBEAT]",
				Username: "heartbeat",
				UserID:   "heartbeat",
				Protocol: "internal",
			})
			if err != nil {
				log.Printf("heartbeat: error: %v", err)
				timer.Reset(interval)
				continue
			}

			// Check for NEXT_HEARTBEAT directive
			if next, stripped, found := parseNextHeartbeat(response); found {
				interval = next
				log.Printf("heartbeat: agent requested next in %s", interval)
				response = stripped
			}

			// Suppress HEARTBEAT_OK and empty responses — don't relay to Telegram
			if response == "HEARTBEAT_OK" || strings.TrimSpace(response) == "" {
				timer.Reset(interval)
				continue
			}

			// Relay non-trivial heartbeat responses to Telegram
			if err := m.SendMessage(response); err != nil {
				log.Printf("heartbeat: send failed: %v", err)
			} else {
				log.Printf("heartbeat: relayed %d chars", len(response))
			}

			timer.Reset(interval)
		}
	}
}

// nextHeartbeatRe matches "NEXT_HEARTBEAT: <duration>" lines in agent responses.
var nextHeartbeatRe = regexp.MustCompile(`(?m)^NEXT_HEARTBEAT:\s*(\S+)\s*$`)

// parseNextHeartbeat scans response text for a NEXT_HEARTBEAT directive.
// Returns the parsed duration (clamped to [1m, 24h]), the response with the
// directive stripped, and whether a directive was found.
func parseNextHeartbeat(response string) (time.Duration, string, bool) {
	match := nextHeartbeatRe.FindStringSubmatch(response)
	if match == nil {
		return 0, response, false
	}

	d, err := time.ParseDuration(match[1])
	if err != nil {
		return 0, response, false
	}

	d = clampHeartbeatInterval(d)

	// Strip the NEXT_HEARTBEAT line from the response
	stripped := strings.TrimSpace(nextHeartbeatRe.ReplaceAllString(response, ""))
	return d, stripped, true
}

// clampHeartbeatInterval clamps a duration to [1m, 24h].
func clampHeartbeatInterval(d time.Duration) time.Duration {
	const (
		minInterval = 1 * time.Minute
		maxInterval = 24 * time.Hour
	)
	if d < minInterval {
		d = minInterval
	}
	if d > maxInterval {
		d = maxInterval
	}
	return d
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

// BridgeRequest is the JSON body for POST /message on the bridge HTTP server.
type BridgeRequest struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Text     string `json:"text"`
	Protocol string `json:"protocol"`
}

// BridgeResponse is the JSON response from the bridge HTTP server.
type BridgeResponse struct {
	Text  string `json:"text"`
	Error string `json:"error,omitempty"`
}

// ServeHTTP starts an HTTP server for receiving messages from external sources.
// POST /message accepts a BridgeRequest and returns the agent's response synchronously.
func (m *MatterbridgeConnector) ServeHTTP(ctx context.Context, addr string) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/message", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}

		var req BridgeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(BridgeResponse{Error: "invalid JSON"})
			return
		}

		if req.Text == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(BridgeResponse{Error: "text is required"})
			return
		}

		userID := req.UserID
		if userID == "" {
			userID = req.Username
		}
		if userID == "" {
			userID = "anonymous"
		}

		fmt.Printf("[%s] %s: %s\n", req.Protocol, req.Username, truncateStr(req.Text, 80))

		sessionID, err := m.getOrCreateSession(userID)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(BridgeResponse{Error: fmt.Sprintf("session: %v", err)})
			return
		}

		text := req.Text

		// Route through middleware (token estimation + compaction + ADK call)
		result, err := m.middleware.ProcessMessage(ctx, userID, sessionID, text)
		if err != nil {
			// Session lost (ADK restart / hot-swap) — invalidate cache and retry once
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
				fmt.Printf("  session %s lost, creating new one\n", truncateStr(sessionID, 8))
				m.mu.Lock()
				delete(m.sessions, userID)
				m.mu.Unlock()

				newSessionID, createErr := m.getOrCreateSession(userID)
				if createErr == nil {
					result, err = m.middleware.ProcessMessage(ctx, userID, newSessionID, text)
					if err == nil {
						sessionID = newSessionID
					}
				}
			}

			// If still erroring after retry
			if err != nil {
				fmt.Printf("  error: %v\n", err)
				if friendly := friendlyError(err); friendly != "" {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(BridgeResponse{Text: friendly})
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadGateway)
				json.NewEncoder(w).Encode(BridgeResponse{Error: fmt.Sprintf("adk: %v", err)})
				return
			}
		}

		// Update session mapping if compaction occurred
		if result.SessionID != sessionID {
			m.mu.Lock()
			m.sessions[userID] = result.SessionID
			m.mu.Unlock()
		}

		fmt.Printf("  -> sent %d chars\n", len(result.Text))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(BridgeResponse{Text: result.Text})
	})

	server := &http.Server{Addr: addr, Handler: mux}

	go func() {
		<-ctx.Done()
		server.Close()
	}()

	fmt.Printf("Bridge HTTP server listening on %s\n", addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// friendlyError converts known ADK/LLM errors into user-facing messages.
// Returns empty string if the error isn't recognized.
func friendlyError(err error) string {
	msg := err.Error()

	// Rate limit (HTTP 429)
	if strings.Contains(msg, "429") || strings.Contains(msg, "Limit Exhausted") || strings.Contains(msg, "rate_limit") {
		// Try to extract reset time from the error
		if idx := strings.Index(msg, "reset at "); idx != -1 {
			resetTime := msg[idx+9:]
			if end := strings.IndexAny(resetTime, "\"}"); end != -1 {
				resetTime = resetTime[:end]
			}
			return fmt.Sprintf("I'm temporarily out of API credits — my limit resets at %s. Try again after that!", resetTime)
		}
		return "I'm temporarily out of API credits. My rate limit will reset soon — try again in a bit!"
	}

	// Connection refused / service down
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host") {
		return "I'm having trouble reaching my brain (LLM service is down). Give me a minute to reconnect."
	}

	// Timeout
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded") {
		return "That took too long and timed out. Try sending a shorter message, or try again in a moment."
	}

	// Generic server error
	if strings.Contains(msg, "500") || strings.Contains(msg, "502") || strings.Contains(msg, "503") {
		return "Something went wrong on the backend. Try again in a moment."
	}

	return ""
}
