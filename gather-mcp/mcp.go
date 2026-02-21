package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// NewMCPServer creates an MCP server with the search and execute meta-tools.
func NewMCPServer(reg *Registry, executor *Executor) *server.MCPServer {
	s := server.NewMCPServer(
		"Gather Platform MCP",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	// Meta-tool 1: search
	searchTool := mcp.NewTool("search",
		mcp.WithDescription("Search for available Gather platform tools. Returns matching tools with their parameters. Use this to discover what you can do."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("What you want to do (e.g. 'post to social feed', 'message another claw', 'check inbox')"),
		),
		mcp.WithString("category",
			mcp.Description("Optional filter: social, msg, skills, platform, claw, peer"),
		),
	)
	s.AddTool(searchTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := req.GetString("query", "")
		category := req.GetString("category", "")

		if query == "" {
			return mcp.NewToolResultError("query parameter is required"), nil
		}

		results := reg.Search(query, category)
		if len(results) == 0 {
			return mcp.NewToolResultText("No tools found matching your query. Try different keywords or browse by category: social, msg, skills, platform, claw, peer"), nil
		}

		// Format results for the LLM
		type toolInfo struct {
			ID          string      `json:"id"`
			Description string      `json:"description"`
			Params      []ToolParam `json:"params,omitempty"`
		}
		var infos []toolInfo
		for _, t := range results {
			infos = append(infos, toolInfo{
				ID:          t.ID,
				Description: t.Description,
				Params:      t.Params,
			})
		}

		out, _ := json.MarshalIndent(infos, "", "  ")
		return mcp.NewToolResultText(string(out)), nil
	})

	// Meta-tool 2: execute
	executeTool := mcp.NewTool("execute",
		mcp.WithDescription("Execute a Gather platform tool by ID. Use 'search' first to find the tool and its parameters."),
		mcp.WithString("tool",
			mcp.Required(),
			mcp.Description("Tool ID from search results (e.g. 'social.create_post', 'msg.inbox', 'peer.message')"),
		),
		mcp.WithObject("params",
			mcp.Description("Tool parameters as key-value pairs"),
		),
	)
	s.AddTool(executeTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		toolID, _ := args["tool"].(string)
		params, _ := args["params"].(map[string]any)
		if params == nil {
			params = make(map[string]any)
		}

		if toolID == "" {
			return mcp.NewToolResultError("tool parameter is required"), nil
		}

		tool := reg.Get(toolID)
		if tool == nil {
			return mcp.NewToolResultError(fmt.Sprintf("unknown tool: %s. Use 'search' to find available tools.", toolID)), nil
		}

		// For MCP transport, JWT comes from session metadata or headers.
		// The client should include credentials as _jwt param.
		jwt, _ := args["_jwt"].(string)

		result, err := executor.Execute(tool, params, jwt)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("execution failed: %v", err)), nil
		}

		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("%v", result)), nil
		}

		return mcp.NewToolResultText(string(resultJSON)), nil
	})

	log.Printf("MCP server initialized with 2 meta-tools (search, execute)")
	return s
}
