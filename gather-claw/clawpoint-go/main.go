package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"clawpoint-go/core"
	"clawpoint-go/core/connectors"
	"clawpoint-go/core/plugins"
	"clawpoint-go/extensions"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/plugin"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
)

func main() {
	ctx := context.Background()

	// Load extension tools and agents
	extTools, err := extensions.RegisterTools()
	if err != nil {
		log.Fatalf("Failed to load extension tools: %v", err)
	}
	extAgents, err := extensions.RegisterAgents()
	if err != nil {
		log.Fatalf("Failed to load extension agents: %v", err)
	}

	cfg := core.OrchestratorConfig{
		ExtensionTools:  extTools,
		ExtensionAgents: extAgents,
	}

	// Build shared resources (model, memory, soul, tasks)
	shared, err := core.BuildSharedResources(ctx)
	if err != nil {
		log.Fatalf("Failed to build shared resources: %v", err)
	}
	defer shared.Cleanup()

	// Build the clay agent (orchestrator → build_loop + ops_loop)
	clayAgent, err := core.BuildClayAgent(shared, cfg)
	if err != nil {
		log.Fatalf("Failed to build clay agent: %v", err)
	}

	loader, err := agent.NewMultiLoader(clayAgent)
	if err != nil {
		log.Fatalf("Failed to create agent loader: %v", err)
	}

	// In-memory sessions — compaction stores durable memories to messages.db,
	// and heartbeat injection restores continuity on restart.
	sessionService := session.InMemoryService()
	log.Printf("Session storage: in-memory")

	// Build plugins — memory injection + lazy compaction run inside ADK runner
	// for ALL requests (browser, Telegram, heartbeat, ADK web UI).
	dbPath := os.Getenv("CLAWPOINT_DB")
	if dbPath == "" {
		dbPath = "../messages.db"
	}
	soulRoot := os.Getenv("CLAWPOINT_ROOT")
	if soulRoot == "" {
		soulRoot = "."
	}

	memPlugin, err := plugins.NewMemoryPlugin(plugins.MemoryPluginConfig{
		DBPath:   dbPath,
		SoulRoot: soulRoot,
		TaskDB:   shared.MemTool.DB(),
	})
	if err != nil {
		log.Fatalf("Failed to create memory plugin: %v", err)
	}

	llmBase := os.Getenv("ANTHROPIC_API_BASE")
	if llmBase == "" {
		llmBase = "https://api.z.ai/api/anthropic"
	}
	llmModel := os.Getenv("ANTHROPIC_MODEL")
	if llmModel == "" {
		llmModel = "glm-5"
	}

	compactPlugin, err := plugins.NewCompactionPlugin(plugins.CompactionPluginConfig{
		SessionService: sessionService,
		DBPath:         dbPath,
		LLMBaseURL:     llmBase,
		LLMAPIKey:      os.Getenv("ANTHROPIC_API_KEY"),
		LLMModel:       llmModel,
	})
	if err != nil {
		log.Fatalf("Failed to create compaction plugin: %v", err)
	}

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Printf("Shutting down...")
		cancel()
	}()

	// Start internal heartbeat goroutine — targets "clay" app for autonomous work.
	adkPort := os.Getenv("ADK_PORT")
	if adkPort == "" {
		adkPort = "8080"
	}
	hb := connectors.NewInternalHeartbeat("http://127.0.0.1:"+adkPort, "clay")
	go hb.Start(ctx)

	// Run with ADK launcher — plugins fire for ALL requests through the runner.
	config := &launcher.Config{
		AgentLoader:    loader,
		SessionService: sessionService,
		PluginConfig: runner.PluginConfig{
			Plugins: []*plugin.Plugin{memPlugin, compactPlugin},
		},
	}
	l := full.NewLauncher()
	if err = l.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}
