package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Executor dispatches tool calls to the appropriate backend.
type Executor struct {
	authURL     string
	auth        *AuthManager
	client      *http.Client
	dockerTools *DockerTools
}

func NewExecutor(authURL string, auth *AuthManager, docker *DockerTools) *Executor {
	return &Executor{
		authURL:     authURL,
		auth:        auth,
		client:      &http.Client{},
		dockerTools: docker,
	}
}

// Execute runs a tool and returns the JSON result.
func (e *Executor) Execute(tool *Tool, params map[string]any, jwt string) (any, error) {
	switch tool.Source {
	case "openapi":
		return e.executeOpenAPI(tool, params, jwt)
	case "docker":
		return e.executeDocker(tool, params)
	case "interclaw":
		return e.executeInterClaw(tool, params, jwt)
	default:
		return nil, fmt.Errorf("unknown tool source: %s", tool.Source)
	}
}

func (e *Executor) executeOpenAPI(tool *Tool, params map[string]any, jwt string) (any, error) {
	// Build URL, substituting path params
	path := tool.Endpoint
	queryParams := make(map[string]string)
	bodyParams := make(map[string]any)

	for _, p := range tool.Params {
		val, ok := params[p.Name]
		if !ok {
			continue
		}
		valStr := fmt.Sprintf("%v", val)

		// Check if this is a path param
		placeholder := "{" + p.Name + "}"
		if strings.Contains(path, placeholder) {
			path = strings.ReplaceAll(path, placeholder, valStr)
			continue
		}

		// For GET/DELETE → query params, for POST/PUT/PATCH → body
		if tool.Method == "GET" || tool.Method == "DELETE" {
			queryParams[p.Name] = valStr
		} else {
			bodyParams[p.Name] = val
		}
	}

	// Also handle params that weren't in the tool definition (passthrough)
	for k, v := range params {
		found := false
		for _, p := range tool.Params {
			if p.Name == k {
				found = true
				break
			}
		}
		if !found {
			if tool.Method == "GET" || tool.Method == "DELETE" {
				queryParams[k] = fmt.Sprintf("%v", v)
			} else {
				bodyParams[k] = v
			}
		}
	}

	url := e.authURL + path

	// Build query string
	if len(queryParams) > 0 {
		parts := make([]string, 0, len(queryParams))
		for k, v := range queryParams {
			parts = append(parts, k+"="+v)
		}
		url += "?" + strings.Join(parts, "&")
	}

	// Build body
	var bodyReader io.Reader
	if len(bodyParams) > 0 {
		bodyJSON, err := json.Marshal(bodyParams)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyJSON)
	}

	req, err := http.NewRequest(tool.Method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	ForwardAuth(req, jwt)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Try to parse as JSON
	var result any
	if err := json.Unmarshal(respBody, &result); err != nil {
		// Return raw text
		return map[string]any{
			"status": resp.StatusCode,
			"body":   string(respBody),
		}, nil
	}

	if resp.StatusCode >= 400 {
		return map[string]any{
			"error":  true,
			"status": resp.StatusCode,
			"body":   result,
		}, nil
	}

	return result, nil
}

func (e *Executor) executeDocker(tool *Tool, params map[string]any) (any, error) {
	if e.dockerTools == nil {
		return nil, fmt.Errorf("docker tools unavailable (no Docker socket)")
	}
	return e.dockerTools.Execute(tool.ID, params)
}

func (e *Executor) executeInterClaw(tool *Tool, params map[string]any, jwt string) (any, error) {
	switch tool.ID {
	case "peer.message":
		return e.sendPeerMessage(params, jwt)
	case "peer.page":
		return e.fetchPeerPage(params)
	case "peer.list":
		return e.listPeers()
	default:
		return nil, fmt.Errorf("unknown interclaw tool: %s", tool.ID)
	}
}

func (e *Executor) sendPeerMessage(params map[string]any, jwt string) (any, error) {
	clawName, _ := params["claw"].(string)
	text, _ := params["text"].(string)
	if clawName == "" || text == "" {
		return nil, fmt.Errorf("'claw' and 'text' params required")
	}

	url := fmt.Sprintf("http://claw-%s:8080/message", clawName)
	body, _ := json.Marshal(map[string]string{"text": text})

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	ForwardAuth(req, jwt)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("peer message failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return map[string]any{"status": resp.StatusCode, "body": string(respBody)}, nil
	}
	return result, nil
}

func (e *Executor) fetchPeerPage(params map[string]any) (any, error) {
	clawName, _ := params["claw"].(string)
	if clawName == "" {
		return nil, fmt.Errorf("'claw' param required")
	}

	url := fmt.Sprintf("http://claw-%s:8080/", clawName)
	resp, err := e.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch peer page: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return map[string]any{"html": string(body)}, nil
}

func (e *Executor) listPeers() (any, error) {
	// This would use Docker API to list claw containers.
	// For now, return a placeholder — docker.go will provide the real implementation.
	return map[string]any{"error": "peer.list requires Docker socket access"}, nil
}
