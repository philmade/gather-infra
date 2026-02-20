package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"clawpoint-go/core"
	"clawpoint-go/extensions"

	"github.com/glebarez/sqlite"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
	sessiondb "google.golang.org/adk/session/database"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
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

	// Build the orchestrator with core + extensions
	coordinator, cleanup, err := core.BuildOrchestrator(ctx, core.OrchestratorConfig{
		ExtensionTools:  extTools,
		ExtensionAgents: extAgents,
	})
	if err != nil {
		log.Fatalf("Failed to build orchestrator: %v", err)
	}
	defer cleanup()

	// Set up persistent session storage (SQLite on the data volume)
	dataDir := os.Getenv("CLAWPOINT_ROOT")
	if dataDir == "" {
		dataDir = "."
	}
	sessionsDBPath := filepath.Join(dataDir, "data", "sessions.db")
	sessionService, err := sessiondb.NewSessionService(
		sqlite.Open(sessionsDBPath),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)},
	)
	if err != nil {
		log.Fatalf("Failed to create session service: %v", err)
	}
	if err := sessiondb.AutoMigrate(sessionService); err != nil {
		log.Fatalf("Failed to migrate session database: %v", err)
	}
	log.Printf("Session persistence: %s", sessionsDBPath)

	// Run with ADK launcher
	config := &launcher.Config{
		AgentLoader:    agent.NewSingleLoader(coordinator),
		SessionService: sessionService,
	}
	l := full.NewLauncher()
	if err = l.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}
