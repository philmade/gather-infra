package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"agency/pocketnode/hooks"
	"agency/pocketnode/tinode"
)

func main() {
	app := pocketbase.New()

	// Get Tinode configuration from environment
	tinodeAddr := os.Getenv("TINODE_ADDR")
	if tinodeAddr == "" {
		tinodeAddr = "tinode:16060"
	}

	apiKey := os.Getenv("TINODE_API_KEY")
	if apiKey == "" {
		apiKey = "AQEAAAABAAD_rAp4DJh05a1HAwFT3A6K"
	}

	// Track Tinode connection status
	var tc *tinode.Client
	var tinodeConnected bool

	// Register auth hook for Tinode user sync
	hooks.RegisterAuth(app, tc)

	// Auto-bootstrap: create superuser on first startup
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		if err := autoBootstrap(app); err != nil {
			app.Logger().Warn("Auto-bootstrap failed", "error", err)
		}
		return e.Next()
	})

	// Single OnServe hook for all setup
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// Try to connect to Tinode on startup (non-blocking)
		go func() {
			var err error
			tc, err = tinode.NewClient(tinodeAddr, apiKey, nil)
			if err != nil {
				app.Logger().Warn("Could not connect to Tinode on startup - will retry on auth",
					"addr", tinodeAddr,
					"error", err,
				)
			} else {
				tinodeConnected = true
				app.Logger().Info("Connected to Tinode",
					"addr", tinodeAddr,
				)
			}
		}()

		// Health check endpoint (using /api/agency/health to avoid conflict with PocketBase built-in)
		e.Router.GET("/api/agency/health", func(re *core.RequestEvent) error {
			return re.JSON(200, map[string]interface{}{
				"status":           "ok",
				"tinode_connected": tinodeConnected,
			})
		})

		// Tinode credentials endpoint (for authenticated users)
		e.Router.GET("/api/tinode/credentials", func(re *core.RequestEvent) error {
			// Require authentication
			info, _ := re.RequestInfo()
			if info.Auth == nil {
				return apis.NewUnauthorizedError("Authentication required", nil)
			}

			pbUserID := info.Auth.Id
			login, password := hooks.GetTinodeCredentials(pbUserID)

			return re.JSON(200, map[string]interface{}{
				"login":    login,
				"password": password,
			})
		}).Bind(apis.RequireAuth())

		// Bootstrap endpoint for initial setup
		e.Router.POST("/api/bootstrap", func(re *core.RequestEvent) error {
			return handleBootstrap(app, re)
		})

		// SDK agent registration endpoint (requires PocketBase auth)
		e.Router.POST("/api/sdk/register-agents", func(re *core.RequestEvent) error {
			return handleRegisterAgents(app, re, tinodeAddr, apiKey)
		}).Bind(apis.RequireAuth())

		return e.Next()
	})

	// Start PocketBase
	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

// autoBootstrap creates the superuser and required collections on first startup
func autoBootstrap(app *pocketbase.PocketBase) error {
	// Create sdk_tokens collection if it doesn't exist
	if err := ensureSDKTokensCollection(app); err != nil {
		app.Logger().Warn("Failed to create sdk_tokens collection", "error", err)
	}

	// Get admin credentials from environment
	adminEmail := os.Getenv("POCKETBASE_ADMIN_EMAIL")
	adminPassword := os.Getenv("POCKETBASE_ADMIN_PASSWORD")

	if adminEmail == "" || adminPassword == "" {
		app.Logger().Debug("No admin credentials in env, skipping admin creation")
		return nil
	}

	// Check if superuser already exists
	superusersCollection, err := app.FindCollectionByNameOrId("_superusers")
	if err != nil {
		return err
	}

	// Try to find existing admin
	existing, _ := app.FindAuthRecordByEmail(superusersCollection, adminEmail)
	if existing != nil {
		app.Logger().Debug("Superuser already exists", "email", adminEmail)
		return nil
	}

	// Create superuser
	admin := core.NewRecord(superusersCollection)
	admin.Set("email", adminEmail)
	admin.Set("password", adminPassword)

	if err := app.Save(admin); err != nil {
		return err
	}

	app.Logger().Info("Auto-bootstrap: created superuser", "email", adminEmail)
	return nil
}

// ensureSDKTokensCollection creates the sdk_tokens collection if it doesn't exist
func ensureSDKTokensCollection(app *pocketbase.PocketBase) error {
	// Check if collection exists
	_, err := app.FindCollectionByNameOrId("sdk_tokens")
	if err == nil {
		return nil // Already exists
	}

	// Create the collection
	collection := core.NewBaseCollection("sdk_tokens")
	collection.Fields.Add(
		&core.TextField{
			Name:     "token",
			Required: true,
		},
		&core.TextField{
			Name:     "workspace",
			Required: true,
		},
		&core.TextField{
			Name:     "user",
			Required: true,
		},
	)

	// Add unique index on token
	collection.AddIndex("idx_sdk_tokens_token", true, "token", "")

	if err := app.Save(collection); err != nil {
		return err
	}

	app.Logger().Info("Auto-bootstrap: created sdk_tokens collection")
	return nil
}

// handleBootstrap handles first-time setup via API
func handleBootstrap(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	// Check if already bootstrapped
	admins, err := app.FindAllRecords("_superusers")
	if err == nil && len(admins) > 0 {
		return re.JSON(200, map[string]interface{}{
			"status":  "already_bootstrapped",
			"message": "System has already been set up",
		})
	}

	// Get admin credentials from environment
	adminEmail := os.Getenv("POCKETBASE_ADMIN_EMAIL")
	adminPassword := os.Getenv("POCKETBASE_ADMIN_PASSWORD")

	if adminEmail == "" || adminPassword == "" {
		return apis.NewBadRequestError("POCKETBASE_ADMIN_EMAIL and POCKETBASE_ADMIN_PASSWORD must be set", nil)
	}

	// Create superuser
	superusersCollection, err := app.FindCollectionByNameOrId("_superusers")
	if err != nil {
		return apis.NewBadRequestError("Failed to find _superusers collection", err)
	}

	admin := core.NewRecord(superusersCollection)
	admin.Set("email", adminEmail)
	admin.Set("password", adminPassword)

	if err := app.Save(admin); err != nil {
		return apis.NewBadRequestError("Failed to create admin user", err)
	}

	app.Logger().Info("Bootstrap complete",
		"admin_email", adminEmail,
	)

	return re.JSON(200, map[string]interface{}{
		"status":      "success",
		"message":     "System bootstrapped successfully",
		"admin_email": adminEmail,
	})
}

// RegisterAgentsRequest is the request body for /api/sdk/register-agents
type RegisterAgentsRequest struct {
	Workspace string   `json:"workspace"` // Workspace ID or slug
	Channels  []string `json:"channels"`  // Channel IDs to subscribe agents to
	Handles   []string `json:"handles"`
}

// AgentCredentials holds the credentials for a single agent
type AgentCredentials struct {
	Handle      string `json:"handle"`
	BotLogin    string `json:"bot_login"`
	BotPassword string `json:"bot_password"`
	BotUID      string `json:"bot_uid,omitempty"`
}

// handleRegisterAgents creates bot users in Tinode for the given agent handles
func handleRegisterAgents(app *pocketbase.PocketBase, re *core.RequestEvent, tinodeAddr, apiKey string) error {
	// Get authenticated user from PocketBase auth context
	info, _ := re.RequestInfo()
	if info.Auth == nil {
		return apis.NewUnauthorizedError("Authentication required", nil)
	}
	userID := info.Auth.Id

	// Parse request body
	var req RegisterAgentsRequest
	if err := json.NewDecoder(re.Request.Body).Decode(&req); err != nil {
		return apis.NewBadRequestError("Invalid request body", err)
	}

	if req.Workspace == "" {
		return apis.NewBadRequestError("Workspace is required", nil)
	}

	if len(req.Handles) == 0 {
		return apis.NewBadRequestError("At least one agent handle is required", nil)
	}

	// Use the workspace from the request
	workspaceID := req.Workspace

	app.Logger().Info("SDK registration request",
		"workspace", workspaceID,
		"user", userID,
		"handles", req.Handles,
	)

	// Connect to Tinode
	log.Printf("Connecting to Tinode at %s...", tinodeAddr)
	tc, err := tinode.NewClient(tinodeAddr, apiKey, nil)
	if err != nil {
		log.Printf("ERROR: Failed to connect to Tinode: %v", err)
		app.Logger().Error("Failed to connect to Tinode", "error", err)
		return apis.NewApiError(500, "Failed to connect to chat server", nil)
	}
	log.Printf("Connected to Tinode successfully")
	defer tc.Close()

	// Create bot users for each handle
	ctx := context.Background()
	agents := make([]AgentCredentials, 0, len(req.Handles))

	// Use channels passed in the request
	channels := req.Channels
	log.Printf("Received %d channel(s) to subscribe agents to", len(channels))

	for _, handle := range req.Handles {
		// Generate deterministic credentials for this bot
		login := generateBotLogin(workspaceID, handle)
		password := generateBotPassword(workspaceID, handle)
		displayName := formatDisplayName(handle)

		log.Printf("Creating bot user: handle=%s, login=%s", handle, login)

		// Create or get existing bot user with proper metadata including handle
		uid, err := tc.EnsureBotUser(ctx, login, password, displayName, handle)
		if err != nil {
			log.Printf("ERROR: Failed to create bot user %s: %v", handle, err)
			app.Logger().Warn("Failed to create bot user",
				"handle", handle,
				"error", err,
			)
			// Continue with other agents
			continue
		}
		log.Printf("Created bot user: handle=%s, uid=%s", handle, uid)

		// Subscribe bot to workspace so it appears in workspace members
		log.Printf("Subscribing bot %s to workspace %s", handle, workspaceID)
		if err := tc.Subscribe(ctx, workspaceID); err != nil {
			log.Printf("WARNING: Failed to subscribe bot to workspace: %v", err)
		} else {
			log.Printf("Bot %s subscribed to workspace %s", handle, workspaceID)
		}

		// Subscribe bot to all channels in the workspace
		for _, channel := range channels {
			log.Printf("Subscribing bot %s to channel %s", handle, channel)
			if err := tc.Subscribe(ctx, channel); err != nil {
				log.Printf("WARNING: Failed to subscribe bot to channel %s: %v", channel, err)
			}
		}

		agents = append(agents, AgentCredentials{
			Handle:      handle,
			BotLogin:    login,
			BotPassword: password,
			BotUID:      uid,
		})

		app.Logger().Info("Registered agent",
			"handle", handle,
			"uid", uid,
		)
	}

	// Return the Tinode WebSocket URL and credentials
	tinodeWsURL := os.Getenv("TINODE_WS_URL")
	if tinodeWsURL == "" {
		// Default for local development
		tinodeWsURL = "ws://localhost:6060/v0/channels"
	}

	return re.JSON(200, map[string]interface{}{
		"success":   true,
		"server":    tinodeWsURL,
		"workspace": workspaceID,
		"agents":    agents,
	})
}

// generateBotLogin creates a deterministic login for a bot
func generateBotLogin(workspaceID, handle string) string {
	// Format: bot_{workspace_hash}_{handle}
	// Use hash to avoid special characters and length issues
	wsHash := sha256.Sum256([]byte(workspaceID))
	wsShort := hex.EncodeToString(wsHash[:])[:8]
	// Replace any non-alphanumeric chars in handle
	cleanHandle := ""
	for _, c := range handle {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			cleanHandle += string(c)
		}
	}
	return "bot" + wsShort + cleanHandle
}

// generateBotPassword creates a deterministic password for a bot
func generateBotPassword(workspaceID, handle string) string {
	secret := os.Getenv("TINODE_PASSWORD_SECRET")
	if secret == "" {
		secret = "agency_bot_password_v1"
	}
	data := workspaceID + "_" + handle + "_" + secret
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])[:24]
}

// formatDisplayName converts handle to display name
func formatDisplayName(handle string) string {
	// Convert snake_case to Title Case
	result := ""
	capitalize := true
	for _, c := range handle {
		if c == '_' {
			result += " "
			capitalize = true
		} else if capitalize {
			result += string(c - 32) // uppercase
			capitalize = false
		} else {
			result += string(c)
		}
	}
	return result
}
