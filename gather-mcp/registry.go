package main

import (
	"strings"
	"sync"
)

// Tool describes a single platform operation.
type Tool struct {
	ID          string      `json:"id"`
	Category    string      `json:"category"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Method      string      `json:"method,omitempty"`
	Endpoint    string      `json:"endpoint,omitempty"`
	Params      []ToolParam `json:"params,omitempty"`
	Source      string      `json:"source"` // "openapi", "docker", "interclaw"
}

// ToolParam describes a tool parameter.
type ToolParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
}

// Registry holds all available tools and provides search.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]*Tool // keyed by ID
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]*Tool)}
}

func (r *Registry) Register(t *Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.ID] = t
}

func (r *Registry) Get(id string) *Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[id]
}

func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// Search finds tools matching a query string and optional category filter.
// Returns up to 5 results, ranked by relevance (ID match > name match > description match).
func (r *Registry) Search(query, category string) []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query = strings.ToLower(query)
	words := strings.Fields(query)

	type scored struct {
		tool  *Tool
		score int
	}
	var results []scored

	for _, t := range r.tools {
		if category != "" && t.Category != category {
			continue
		}

		score := 0
		id := strings.ToLower(t.ID)
		name := strings.ToLower(t.Name)
		desc := strings.ToLower(t.Description)

		for _, w := range words {
			if strings.Contains(id, w) {
				score += 3
			}
			if strings.Contains(name, w) {
				score += 2
			}
			if strings.Contains(desc, w) {
				score += 1
			}
		}

		if score > 0 {
			results = append(results, scored{tool: t, score: score})
		}
	}

	// Sort by score descending (simple insertion sort for small N)
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].score > results[j-1].score; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}

	limit := 5
	if len(results) < limit {
		limit = len(results)
	}

	out := make([]*Tool, limit)
	for i := 0; i < limit; i++ {
		out[i] = results[i].tool
	}
	return out
}

// All returns all tools, optionally filtered by category.
func (r *Registry) All(category string) []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []*Tool
	for _, t := range r.tools {
		if category == "" || t.Category == category {
			out = append(out, t)
		}
	}
	return out
}
