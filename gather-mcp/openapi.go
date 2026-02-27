package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// OpenAPI tag → tool category mapping.
var tagToCategory = map[string]string{
	"Social":     "social",
	"Messaging":  "msg",
	"Skills":     "skills",
	"Reviews":    "skills",
	"Proofs":     "skills",
	"Rankings":   "skills",
	"Shop":       "shop",
	"Platform":   "platform",
	"Health":     "platform",
	"Agents":     "platform",
	"Agent Auth": "platform",
	"Claw":       "claw",
	"Balance":    "social",
	"PoW":        "platform",
	"Inbox":      "msg",
	"Discover":   "platform",
	"Email":      "email",
}

// LoadFromOpenAPI fetches the OpenAPI spec from gather-auth and populates the registry.
func LoadFromOpenAPI(reg *Registry, baseURL string) error {
	resp, err := http.Get(baseURL + "/openapi.json")
	if err != nil {
		return fmt.Errorf("fetch openapi: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("openapi returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read openapi: %w", err)
	}

	var spec openAPISpec
	if err := json.Unmarshal(body, &spec); err != nil {
		return fmt.Errorf("parse openapi: %w", err)
	}

	count := 0
	for path, methods := range spec.Paths {
		for method, op := range methods {
			method = strings.ToUpper(method)
			if method == "OPTIONS" || method == "HEAD" {
				continue
			}

			category := categorize(op.Tags)
			if category == "" {
				category = "platform"
			}

			toolName := buildToolName(category, op.OperationID, method, path)
			description := op.Summary
			if description == "" {
				description = op.Description
			}

			params := extractParams(op.Parameters, op.RequestBody)

			tool := &Tool{
				ID:          toolName,
				Category:    category,
				Name:        toolName,
				Description: description,
				Method:      method,
				Endpoint:    path,
				Params:      params,
				Source:       "openapi",
			}
			reg.Register(tool)
			count++
		}
	}

	log.Printf("Loaded %d tools from OpenAPI spec", count)
	return nil
}

func categorize(tags []string) string {
	for _, tag := range tags {
		if cat, ok := tagToCategory[tag]; ok {
			return cat
		}
	}
	return ""
}

func buildToolName(category, operationID, method, path string) string {
	if operationID != "" {
		// Convert kebab-case to snake: "list-channels" → "list_channels"
		name := strings.ReplaceAll(operationID, "-", "_")
		return category + "." + name
	}
	// Fallback: build from method + path
	parts := strings.Split(strings.Trim(path, "/"), "/")
	name := strings.ToLower(method)
	for _, p := range parts {
		if !strings.HasPrefix(p, "{") && p != "api" {
			name += "_" + p
		}
	}
	return category + "." + name
}

func extractParams(params []openAPIParam, reqBody *openAPIRequestBody) []ToolParam {
	var out []ToolParam

	// Path + query params
	for _, p := range params {
		out = append(out, ToolParam{
			Name:        p.Name,
			Type:        schemaType(p.Schema),
			Required:    p.Required,
			Description: p.Description,
		})
	}

	// Request body params
	if reqBody != nil {
		for _, content := range reqBody.Content {
			if content.Schema.Properties != nil {
				for name, prop := range content.Schema.Properties {
					required := false
					for _, r := range content.Schema.Required {
						if r == name {
							required = true
							break
						}
					}
					out = append(out, ToolParam{
						Name:        name,
						Type:        prop.Type,
						Required:    required,
						Description: prop.Description,
					})
				}
			}
		}
	}

	return out
}

func schemaType(s *openAPISchema) string {
	if s == nil {
		return "string"
	}
	if s.Type != "" {
		return s.Type
	}
	return "string"
}

// Minimal OpenAPI spec structures — just enough to extract tools.
type openAPISpec struct {
	Paths map[string]map[string]openAPIOperation `json:"paths"`
}

type openAPIOperation struct {
	OperationID string              `json:"operationId"`
	Summary     string              `json:"summary"`
	Description string              `json:"description"`
	Tags        []string            `json:"tags"`
	Parameters  []openAPIParam      `json:"parameters"`
	RequestBody *openAPIRequestBody `json:"requestBody"`
}

type openAPIParam struct {
	Name        string        `json:"name"`
	In          string        `json:"in"`
	Required    bool          `json:"required"`
	Description string        `json:"description"`
	Schema      *openAPISchema `json:"schema"`
}

type openAPIRequestBody struct {
	Content map[string]openAPIMediaType `json:"content"`
}

type openAPIMediaType struct {
	Schema openAPIBodySchema `json:"schema"`
}

type openAPIBodySchema struct {
	Type       string                    `json:"type"`
	Required   []string                  `json:"required"`
	Properties map[string]openAPISchema  `json:"properties"`
}

type openAPISchema struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}
