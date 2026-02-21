package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/server"
)

func main() {
	port := getEnv("MCP_PORT", "9200")
	authURL := getEnv("GATHER_AUTH_URL", "http://gather-auth:8090")

	log.Printf("gather-mcp starting (auth=%s, port=%s)", authURL, port)

	// Build tool registry
	reg := NewRegistry()

	// Load tools from OpenAPI spec (retry — gather-auth may not be ready yet)
	go func() {
		for i := 0; i < 30; i++ {
			if err := LoadFromOpenAPI(reg, authURL); err != nil {
				log.Printf("OpenAPI load attempt %d failed: %v", i+1, err)
				time.Sleep(2 * time.Second)
				continue
			}
			return
		}
		log.Printf("WARNING: Could not load OpenAPI spec after 30 attempts")
	}()

	// Register manual tools (Docker, inter-claw)
	var dockerTools *DockerTools
	dt, err := NewDockerTools()
	if err != nil {
		log.Printf("Docker tools unavailable: %v (claw.* and peer.list tools disabled)", err)
	} else {
		dockerTools = dt
		dockerTools.RegisterTools(reg)
	}
	RegisterInterClawTools(reg)

	// Auth + executor
	auth := NewAuthManager(authURL)
	executor := NewExecutor(authURL, auth, dockerTools)

	// --- MCP transport (Streamable HTTP) ---
	mcpServer := NewMCPServer(reg, executor)
	mcpHTTP := server.NewStreamableHTTPServer(mcpServer)

	// --- Plain HTTP transport (for claws) ---
	mux := http.NewServeMux()

	// MCP endpoint
	mux.Handle("/mcp", mcpHTTP)

	// Plain HTTP: search
	mux.HandleFunc("/tools/search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		category := r.URL.Query().Get("category")

		if query == "" {
			// Return all tools grouped by category
			tools := reg.All(category)
			writeJSON(w, map[string]any{"tools": tools, "count": len(tools)})
			return
		}

		results := reg.Search(query, category)
		writeJSON(w, map[string]any{"tools": results, "count": len(results)})
	})

	// Plain HTTP: execute
	mux.HandleFunc("/tools/execute", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "read body: "+err.Error())
			return
		}

		// Authenticate
		jwt, err := auth.AuthenticateRequest(r, body)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}

		// Parse request
		var req struct {
			Tool   string         `json:"tool"`
			Params map[string]any `json:"params"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}

		if req.Tool == "" {
			writeError(w, http.StatusBadRequest, "tool field required")
			return
		}

		tool := reg.Get(req.Tool)
		if tool == nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("unknown tool: %s", req.Tool))
			return
		}

		if req.Params == nil {
			req.Params = make(map[string]any)
		}

		result, err := executor.Execute(tool, req.Params, jwt)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, map[string]any{"result": result})
	})

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"status":     "ok",
			"service":    "gather-mcp",
			"tools":      reg.Count(),
		})
	})

	log.Printf("Listening on :%s (MCP: /mcp, HTTP: /tools/search, /tools/execute)", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
