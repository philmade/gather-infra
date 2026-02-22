package anthropicmodel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"log"
	"net/http"
	"strings"
	"time"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// Model implements model.LLM for Anthropic-compatible APIs (z.ai / LiteLLM).
type Model struct {
	name    string
	baseURL string
	apiKey  string
	client  *http.Client
}

// Config for creating an Anthropic model.
type Config struct {
	Model   string // e.g. "anthropic/glm-5"
	BaseURL string // e.g. "https://api.z.ai/api/anthropic"
	APIKey  string
}

func New(cfg Config) *Model {
	return &Model{
		name:    cfg.Model,
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		client:  &http.Client{},
	}
}

func (m *Model) Name() string { return m.name }

func (m *Model) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		resp, err := m.generate(ctx, req)
		yield(resp, err)
	}
}

// --- Anthropic API types ---

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []contentBlock
}

type contentBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   any            `json:"content,omitempty"`
	IsError   bool           `json:"is_error,omitempty"`
}

type anthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema"`
}

type anthropicResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Content      []contentBlock `json:"content"`
	Model        string         `json:"model"`
	StopReason   string         `json:"stop_reason"`
	Usage        *anthropicUsage `json:"usage"`
	Error        *anthropicError `json:"error"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

const (
	maxRetries     = 3
	initialBackoff = 2 * time.Second
	maxBackoff     = 30 * time.Second
)

func (m *Model) generate(ctx context.Context, req *model.LLMRequest) (*model.LLMResponse, error) {
	anthReq := m.convertRequest(req)

	body, err := json.Marshal(anthReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	var lastErr error
	backoff := initialBackoff

	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := m.generateOnce(ctx, body)
		if err == nil {
			return resp, nil
		}
		lastErr = err

		if !isRetryableError(err) {
			return nil, err
		}

		if attempt == maxRetries {
			break
		}

		log.Printf("anthropic: attempt %d/%d failed (%v), retrying in %s", attempt, maxRetries, err, backoff)

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled during retry: %w", ctx.Err())
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	return nil, fmt.Errorf("all %d attempts failed: %w", maxRetries, lastErr)
}

func (m *Model) generateOnce(ctx context.Context, body []byte) (*model.LLMResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", m.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", m.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	httpResp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, &retryableError{err: fmt.Errorf("http request: %w", err)}
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, &retryableError{err: fmt.Errorf("read response: %w", err)}
	}

	if httpResp.StatusCode != 200 {
		apiErr := fmt.Errorf("anthropic API error (HTTP %d): %s", httpResp.StatusCode, string(respBody))
		if httpResp.StatusCode == 429 || httpResp.StatusCode >= 500 {
			return nil, &retryableError{err: apiErr}
		}
		return nil, apiErr // 400, 401, 403 etc — don't retry
	}

	var anthResp anthropicResponse
	if err := json.Unmarshal(respBody, &anthResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if anthResp.Error != nil {
		apiErr := fmt.Errorf("anthropic error: %s: %s", anthResp.Error.Type, anthResp.Error.Message)
		// Overloaded errors are retryable
		if anthResp.Error.Type == "overloaded_error" || anthResp.Error.Type == "rate_limit_error" {
			return nil, &retryableError{err: apiErr}
		}
		return nil, apiErr
	}

	return m.convertResponse(&anthResp), nil
}

// retryableError wraps an error to signal the retry loop should continue.
type retryableError struct {
	err error
}

func (e *retryableError) Error() string { return e.err.Error() }
func (e *retryableError) Unwrap() error { return e.err }

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	var re *retryableError
	if ok := errorAs(err, &re); ok {
		return true
	}
	// Also retry on context deadline exceeded (timeout) but NOT context cancelled (user abort)
	msg := err.Error()
	if strings.Contains(msg, "deadline exceeded") || strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") || strings.Contains(msg, "EOF") {
		return true
	}
	return false
}

// errorAs is a type-assertion helper equivalent to errors.As for our retryableError.
func errorAs(err error, target **retryableError) bool {
	if e, ok := err.(*retryableError); ok {
		*target = e
		return true
	}
	return false
}

func (m *Model) convertRequest(req *model.LLMRequest) *anthropicRequest {
	anthReq := &anthropicRequest{
		Model:     m.name,
		MaxTokens: 8192,
	}

	// System instruction
	if req.Config != nil && req.Config.SystemInstruction != nil {
		for _, p := range req.Config.SystemInstruction.Parts {
			if p.Text != "" {
				anthReq.System += p.Text
			}
		}
	}

	if req.Config != nil && req.Config.MaxOutputTokens > 0 {
		anthReq.MaxTokens = int(req.Config.MaxOutputTokens)
	}

	// Convert tools from genai format
	if req.Config != nil && req.Config.Tools != nil {
		for _, t := range req.Config.Tools {
			if t.FunctionDeclarations != nil {
				for _, fd := range t.FunctionDeclarations {
					at := anthropicTool{
						Name:        fd.Name,
						Description: fd.Description,
					}
					// Use JSON schema if available, else parameters
					if fd.ParametersJsonSchema != nil {
						at.InputSchema = fd.ParametersJsonSchema
					} else if fd.Parameters != nil {
						at.InputSchema = fd.Parameters
					} else {
						at.InputSchema = map[string]any{"type": "object", "properties": map[string]any{}}
					}
					anthReq.Tools = append(anthReq.Tools, at)
				}
			}
		}
	}

	// Convert messages
	for _, content := range req.Contents {
		msg := m.convertContentToMessage(content)
		if msg != nil {
			anthReq.Messages = append(anthReq.Messages, *msg)
		}
	}

	// Anthropic requires messages to start with a user message
	if len(anthReq.Messages) > 0 && anthReq.Messages[0].Role != "user" {
		anthReq.Messages = append([]anthropicMessage{{Role: "user", Content: "Continue."}}, anthReq.Messages...)
	}

	// Anthropic requires alternating user/assistant messages - merge consecutive same-role
	anthReq.Messages = mergeConsecutiveMessages(anthReq.Messages)

	return anthReq
}

func (m *Model) convertContentToMessage(content *genai.Content) *anthropicMessage {
	if content == nil || len(content.Parts) == 0 {
		return nil
	}

	role := "user"
	if content.Role == "model" {
		role = "assistant"
	}

	var blocks []contentBlock
	for _, part := range content.Parts {
		if part.Thought {
			continue // skip thought parts
		}
		if part.Text != "" {
			blocks = append(blocks, contentBlock{Type: "text", Text: part.Text})
		}
		if part.FunctionCall != nil {
			blocks = append(blocks, contentBlock{
				Type:  "tool_use",
				ID:    part.FunctionCall.ID,
				Name:  part.FunctionCall.Name,
				Input: part.FunctionCall.Args,
			})
		}
		if part.FunctionResponse != nil {
			block := contentBlock{
				Type:      "tool_result",
				ToolUseID: part.FunctionResponse.ID,
			}
			resp := part.FunctionResponse.Response
			if len(resp) == 0 {
				block.Content = "Tool returned no response"
				block.IsError = true
			} else if _, hasErr := resp["error"]; hasErr {
				// ADK convention: "error" key signals tool failure
				b, _ := json.Marshal(resp)
				block.Content = string(b)
				block.IsError = true
			} else {
				b, _ := json.Marshal(resp)
				block.Content = string(b)
			}
			blocks = append(blocks, block)
		}
	}

	if len(blocks) == 0 {
		return nil
	}

	// If it's just one text block, simplify
	if len(blocks) == 1 && blocks[0].Type == "text" {
		return &anthropicMessage{Role: role, Content: blocks[0].Text}
	}

	return &anthropicMessage{Role: role, Content: blocks}
}

func (m *Model) convertResponse(resp *anthropicResponse) *model.LLMResponse {
	var parts []*genai.Part

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			parts = append(parts, &genai.Part{Text: block.Text})
		case "tool_use":
			parts = append(parts, &genai.Part{
				FunctionCall: &genai.FunctionCall{
					ID:   block.ID,
					Name: block.Name,
					Args: block.Input,
				},
			})
		}
	}

	llmResp := &model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: parts,
		},
		TurnComplete: true,
	}

	// Map stop reason
	switch resp.StopReason {
	case "end_turn":
		llmResp.FinishReason = genai.FinishReasonStop
	case "tool_use":
		llmResp.FinishReason = genai.FinishReasonStop
		llmResp.TurnComplete = false
	case "max_tokens":
		llmResp.FinishReason = genai.FinishReasonMaxTokens
	}

	// Usage
	if resp.Usage != nil {
		llmResp.UsageMetadata = &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     int32(resp.Usage.InputTokens),
			CandidatesTokenCount: int32(resp.Usage.OutputTokens),
			TotalTokenCount:      int32(resp.Usage.InputTokens + resp.Usage.OutputTokens),
		}
	}

	return llmResp
}

// mergeConsecutiveMessages ensures alternating user/assistant roles.
// For plain text messages, consecutive same-role messages are merged.
// For messages containing tool blocks (tool_use or tool_result), we insert
// synthetic alternation to preserve sequential tool call ordering — merging
// them would make sequential calls look like parallel calls.
func mergeConsecutiveMessages(msgs []anthropicMessage) []anthropicMessage {
	if len(msgs) <= 1 {
		return msgs
	}

	var result []anthropicMessage
	for _, msg := range msgs {
		if len(result) == 0 || result[len(result)-1].Role != msg.Role {
			result = append(result, msg)
			continue
		}
		// Consecutive same-role — check if either contains tool blocks
		if hasToolBlocks(result[len(result)-1].Content) || hasToolBlocks(msg.Content) {
			// Insert synthetic alternation to preserve tool call sequencing
			filler := "user"
			if msg.Role == "user" {
				filler = "assistant"
			}
			result = append(result, anthropicMessage{Role: filler, Content: "Continue."})
			result = append(result, msg)
		} else {
			// Safe to merge plain text messages
			prev := &result[len(result)-1]
			prevBlocks := toBlocks(prev.Content)
			newBlocks := toBlocks(msg.Content)
			prev.Content = append(prevBlocks, newBlocks...)
		}
	}
	return result
}

// hasToolBlocks checks if a message content contains tool_use or tool_result blocks.
func hasToolBlocks(content any) bool {
	blocks, ok := content.([]contentBlock)
	if !ok {
		return false
	}
	for _, b := range blocks {
		if b.Type == "tool_use" || b.Type == "tool_result" {
			return true
		}
	}
	return false
}

func toBlocks(content any) []contentBlock {
	switch v := content.(type) {
	case string:
		return []contentBlock{{Type: "text", Text: v}}
	case []contentBlock:
		return v
	default:
		return nil
	}
}
