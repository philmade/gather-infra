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
	"strings"
)

// Middleware wraps ADK calls with SSE forwarding and event parsing.
// Memory injection and session compaction are handled by ADK plugins
// (see core/plugins/) — the middleware just forwards messages.
type Middleware struct {
	adkURL  string
	appName string
}

// NewMiddleware creates a middleware instance targeting the "clawpoint" app.
func NewMiddleware(adkURL string) *Middleware {
	return NewMiddlewareForApp(adkURL, "clawpoint")
}

// NewMiddlewareForApp creates a middleware instance targeting the specified ADK app.
func NewMiddlewareForApp(adkURL string, appName string) *Middleware {
	return &Middleware{
		adkURL:  adkURL,
		appName: appName,
	}
}

// ADKEvent represents a single event from the ADK SSE stream (text chunk, tool call, or tool result).
type ADKEvent struct {
	Type     string `json:"type"`                // "text", "tool_call", "tool_result"
	Author   string `json:"author,omitempty"`     // agent that produced this event
	Text     string `json:"text,omitempty"`       // for type=text
	ToolName string `json:"tool_name,omitempty"`  // for type=tool_call / tool_result
	ToolID   string `json:"tool_id,omitempty"`    // for tool_call + tool_result
	ToolArgs any    `json:"tool_args,omitempty"`  // for type=tool_call
	Result   any    `json:"result,omitempty"`     // for type=tool_result
}

// ProcessResult holds the response text and the session ID that was used.
type ProcessResult struct {
	Text      string
	SessionID string     // The session ID used
	Events    []ADKEvent // Captured ADK events (tool calls, tool results, text chunks)
}

// ProcessMessage is the sync path — buffers all events, returns at the end.
func (mw *Middleware) ProcessMessage(ctx context.Context, userID, sessionID, text string) (*ProcessResult, error) {
	return mw.processMessageInternal(ctx, userID, sessionID, text, nil)
}

// ProcessMessageStream emits events via onEvent as they arrive from ADK.
// Still returns the final ProcessResult with all events collected.
func (mw *Middleware) ProcessMessageStream(ctx context.Context, userID, sessionID, text string,
	onEvent func(ADKEvent)) (*ProcessResult, error) {
	return mw.processMessageInternal(ctx, userID, sessionID, text, onEvent)
}

// processMessageInternal forwards the message to ADK and returns the response.
// Memory injection and compaction are handled by ADK plugins — we just forward.
func (mw *Middleware) processMessageInternal(ctx context.Context, userID, sessionID, text string,
	onEvent func(ADKEvent)) (*ProcessResult, error) {

	response, events, err := mw.sendRunSSEInternal(ctx, userID, sessionID, text, onEvent)
	if err != nil {
		return nil, err
	}

	// HEARTBEAT_OK suppression: if the agent said nothing needs attention,
	// mark the result so callers can skip relaying/saving.
	if isHeartbeatOK(response) {
		log.Printf("  HEARTBEAT_OK — suppressing response")
		return &ProcessResult{Text: "HEARTBEAT_OK", SessionID: sessionID}, nil
	}

	return &ProcessResult{Text: response, SessionID: sessionID, Events: events}, nil
}

// isHeartbeatOK checks if the agent's response is a HEARTBEAT_OK idle signal.
// Uses HasSuffix to handle agents that emit analysis text before the HEARTBEAT_OK marker.
func isHeartbeatOK(response string) bool {
	trimmed := strings.TrimSpace(response)
	return trimmed == "HEARTBEAT_OK" || strings.HasSuffix(trimmed, "HEARTBEAT_OK")
}

// sendRunSSEInternal forwards a message to ADK via run_sse and returns the response text + captured events.
// If onEvent is non-nil, each parsed event is emitted immediately via the callback.
func (mw *Middleware) sendRunSSEInternal(ctx context.Context, userID, sessionID, text string,
	onEvent func(ADKEvent)) (string, []ADKEvent, error) {
	payload := map[string]any{
		"appName":   mw.appName,
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

	req, err := http.NewRequestWithContext(ctx, "POST", mw.adkURL+"/api/run_sse", bytes.NewReader(jsonPayload))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	// No client-level timeout — SSE streams stay open for the entire agent run,
	// streaming events tool-by-tool. The request context handles cancellation.
	sseClient := &http.Client{}
	resp, err := sseClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", nil, fmt.Errorf("run_sse HTTP %d: %s", resp.StatusCode, string(body))
	}

	return parseSSEResponseFull(resp.Body, onEvent)
}

// parseSSEResponse reads SSE events and extracts the agent's text response.
func parseSSEResponse(r io.Reader) (string, error) {
	text, _, err := parseSSEResponseFull(r, nil)
	return text, err
}

// parseSSEResponseFull reads SSE events and extracts the agent's text response
// plus all ADK events (tool calls, tool results, text chunks).
// If onEvent is non-nil, each parsed event is emitted immediately via the callback.
func parseSSEResponseFull(r io.Reader, onEvent func(ADKEvent)) (string, []ADKEvent, error) {
	scanner := bufio.NewScanner(r)
	// ADK SSE events can be very large (tool results with full file contents, memory
	// recalls, etc.). Default bufio.Scanner buffer is 64KB — if any single SSE line
	// exceeds that, scanner.Scan() silently returns false and we lose the rest of the
	// agent's execution. 2MB handles even the largest tool results.
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	var lastText string
	var events []ADKEvent

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "Error while running agent:") {
			return "", nil, fmt.Errorf("%s", line)
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := line[6:]
		var event map[string]any
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		author, _ := event["author"].(string)

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

			// Text part
			if text, ok := part["text"].(string); ok && text != "" {
				lastText = text
				adkEvt := ADKEvent{
					Type:   "text",
					Author: author,
					Text:   text,
				}
				events = append(events, adkEvt)
				if onEvent != nil {
					onEvent(adkEvt)
				}
			}

			// Function call
			if fc, ok := part["functionCall"].(map[string]any); ok {
				name, _ := fc["name"].(string)
				id, _ := fc["id"].(string)
				args := fc["args"]
				adkEvt := ADKEvent{
					Type:     "tool_call",
					Author:   author,
					ToolName: name,
					ToolID:   id,
					ToolArgs: args,
				}
				events = append(events, adkEvt)
				if onEvent != nil {
					onEvent(adkEvt)
				}
			}

			// Function response
			if fr, ok := part["functionResponse"].(map[string]any); ok {
				name, _ := fr["name"].(string)
				id, _ := fr["id"].(string)
				result := fr["response"]
				adkEvt := ADKEvent{
					Type:     "tool_result",
					Author:   author,
					ToolName: name,
					ToolID:   id,
					Result:   result,
				}
				events = append(events, adkEvt)
				if onEvent != nil {
					onEvent(adkEvt)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		// Partial results are still useful — log the error but return what we have
		log.Printf("  SSE scanner error (got %d events before failure): %v", len(events), err)
		if lastText != "" {
			return lastText, events, nil
		}
		return "", events, fmt.Errorf("SSE stream read error: %w", err)
	}

	return lastText, events, nil
}

func truncSID(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
