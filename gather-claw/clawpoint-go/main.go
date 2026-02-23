package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"clawpoint-go/core"
	"clawpoint-go/core/connectors"
	"clawpoint-go/extensions"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
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

	// Build the interactive coordinator ("clawpoint" app — Telegram + web UI)
	coordinator, err := core.BuildOrchestrator(ctx, cfg, shared)
	if err != nil {
		log.Fatalf("Failed to build orchestrator: %v", err)
	}

	// Build the autonomous loop agent ("claw" app — generator-reviewer loop)
	clawAgent, err := core.BuildClawAgent(shared, cfg)
	if err != nil {
		log.Fatalf("Failed to build claw agent: %v", err)
	}

	// Multi-loader: both apps available in ADK web UI dropdown
	loader, err := agent.NewMultiLoader(coordinator, clawAgent)
	if err != nil {
		log.Fatalf("Failed to create multi-loader: %v", err)
	}

	// In-memory sessions — compaction stores durable memories to messages.db,
	// and heartbeat injection restores continuity on restart.
	sessionService := session.InMemoryService()
	log.Printf("Session storage: in-memory")

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

	// Start internal heartbeat goroutine — targets "claw" app for autonomous work.
	adkPort := os.Getenv("ADK_PORT")
	if adkPort == "" {
		adkPort = "8080"
	}
	hb := connectors.NewInternalHeartbeat("http://127.0.0.1:"+adkPort, "claw")
	go hb.Start(ctx)

	// Run with ADK launcher
	config := &launcher.Config{
		AgentLoader:    loader,
		SessionService: sessionService,
	}
	l := full.NewLauncher()
	if err = l.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}
