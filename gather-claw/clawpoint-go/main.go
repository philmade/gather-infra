package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"

	"clawpoint-go/core"
	"clawpoint-go/extensions"

	// modernc.org/sqlite registers the "sqlite" driver via memory.go's blank import.
	// Do NOT also import glebarez/sqlite — it pulls in glebarez/go-sqlite which
	// double-registers the same driver name and panics.

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

	// Set up persistent session storage (SQLite on the data volume).
	// We open the sql.DB ourselves using the already-registered modernc.org/sqlite
	// driver, then pass it to GORM via the Conn field to avoid driver conflicts.
	dataDir := os.Getenv("CLAWPOINT_ROOT")
	if dataDir == "" {
		dataDir = "."
	}
	sessionsDBPath := filepath.Join(dataDir, "data", "sessions.db")
	sessionsDB, err := sql.Open("sqlite", sessionsDBPath)
	if err != nil {
		log.Fatalf("Failed to open sessions database: %v", err)
	}
	sessionService, err := sessiondb.NewSessionService(
		&sqliteDialector{conn: sessionsDB},
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
