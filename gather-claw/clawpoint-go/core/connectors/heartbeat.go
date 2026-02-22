package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// InternalHeartbeat runs a self-managed heartbeat loop as a goroutine.
// It sends [HEARTBEAT] messages through the middleware pipeline (which handles
// task injection, memory injection, HEARTBEAT_OK suppression) to the ADK API.
// Works standalone — no matterbridge or Telegram required.
type InternalHeartbeat struct {
	adkURL     string
	middleware *Middleware

	sessionID string
	mu        sync.Mutex
}

// NewInternalHeartbeat creates a heartbeat that talks to the ADK API at adkURL.
func NewInternalHeartbeat(adkURL string) *InternalHeartbeat {
	return &InternalHeartbeat{
		adkURL:     adkURL,
		middleware: NewMiddleware(adkURL),
	}
}

// Start runs the heartbeat loop. It blocks until ctx is cancelled.
// Call this as a goroutine: go hb.Start(ctx)
func (h *InternalHeartbeat) Start(ctx context.Context) {
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
	// Don't clamp the operator-set interval — only clamp agent NEXT_HEARTBEAT directives.
	// This allows short intervals for testing (e.g. 30s) while keeping the 1m floor
	// for agent-requested scheduling.

	// Wait for ADK server to be ready
	if !h.waitForReady(ctx) {
		return
	}

	log.Printf("heartbeat: starting internal heartbeat (interval: %s)", interval)

	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("heartbeat: stopped")
			return
		case <-timer.C:
			log.Printf("heartbeat: tick")

			response, err := h.sendHeartbeat(ctx)
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

			// Log non-trivial responses (HEARTBEAT_OK is already suppressed by middleware)
			if isHeartbeatOK(response) || strings.TrimSpace(response) == "" {
				log.Printf("heartbeat: idle (HEARTBEAT_OK)")
			} else {
				log.Printf("heartbeat: agent responded (%d chars)", len(response))
			}

			timer.Reset(interval)
		}
	}
}

// waitForReady polls the ADK API until it responds or ctx is cancelled.
func (h *InternalHeartbeat) waitForReady(ctx context.Context) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	checkURL := h.adkURL + "/api/list-apps"

	for {
		select {
		case <-ctx.Done():
			return false
		default:
		}

		resp, err := client.Get(checkURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				log.Printf("heartbeat: ADK server ready at %s", h.adkURL)
				return true
			}
		}

		time.Sleep(1 * time.Second)
	}
}

// sendHeartbeat sends a [HEARTBEAT] message through the middleware pipeline.
func (h *InternalHeartbeat) sendHeartbeat(ctx context.Context) (string, error) {
	sessionID, err := h.getOrCreateSession()
	if err != nil {
		return "", fmt.Errorf("session: %w", err)
	}

	result, err := h.middleware.ProcessMessage(ctx, "heartbeat", sessionID, "[HEARTBEAT]")
	if err != nil {
		// Session lost (ADK restart / hot-swap) — invalidate and retry once
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
			log.Printf("heartbeat: session lost, creating new one")
			h.mu.Lock()
			h.sessionID = ""
			h.mu.Unlock()

			sessionID, err = h.getOrCreateSession()
			if err != nil {
				return "", fmt.Errorf("retry session: %w", err)
			}
			result, err = h.middleware.ProcessMessage(ctx, "heartbeat", sessionID, "[HEARTBEAT]")
			if err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}

	// Update session if compaction occurred
	if result.SessionID != sessionID {
		h.mu.Lock()
		h.sessionID = result.SessionID
		h.mu.Unlock()
		log.Printf("heartbeat: session compacted → %s", truncSID(result.SessionID))
	}

	return result.Text, nil
}

// getOrCreateSession returns the cached session or creates a new one via ADK API.
func (h *InternalHeartbeat) getOrCreateSession() (string, error) {
	h.mu.Lock()
	if h.sessionID != "" {
		sid := h.sessionID
		h.mu.Unlock()
		return sid, nil
	}
	h.mu.Unlock()

	// Try to find an existing heartbeat session
	client := &http.Client{Timeout: 10 * time.Second}
	listURL := fmt.Sprintf("%s/api/apps/clawpoint/users/heartbeat/sessions", h.adkURL)
	resp, err := client.Get(listURL)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var sessions []map[string]any
			if parseJSONBody(resp, &sessions) == nil && len(sessions) > 0 {
				// Use the most recent session
				var bestID, bestTime string
				for _, s := range sessions {
					sid, _ := s["id"].(string)
					updated, _ := s["lastUpdateTime"].(string)
					if sid != "" && updated >= bestTime {
						bestID = sid
						bestTime = updated
					}
				}
				if bestID != "" {
					h.mu.Lock()
					h.sessionID = bestID
					h.mu.Unlock()
					log.Printf("heartbeat: resumed session %s", truncSID(bestID))
					return bestID, nil
				}
			}
		}
	}

	// Create new session
	createURL := fmt.Sprintf("%s/api/apps/clawpoint/users/heartbeat/sessions", h.adkURL)
	resp, err = client.Post(createURL, "application/json", strings.NewReader("{}"))
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := parseJSONBody(resp, &result); err != nil {
		return "", fmt.Errorf("parse session: %w", err)
	}

	sessionID, ok := result["id"].(string)
	if !ok {
		return "", fmt.Errorf("no session id in response")
	}

	h.mu.Lock()
	h.sessionID = sessionID
	h.mu.Unlock()

	log.Printf("heartbeat: created session %s", truncSID(sessionID))
	return sessionID, nil
}

// parseJSONBody reads and unmarshals a JSON response body.
func parseJSONBody(resp *http.Response, v any) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}
