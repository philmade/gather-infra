package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"

	gatherapi "gather.is/auth/api"
	"gather.is/auth/tinode"
)

func main() {
	app := pocketbase.New()

	challenges := gatherapi.NewChallengeStore()

	jwtKey := []byte(os.Getenv("JWT_SIGNING_KEY"))
	if len(jwtKey) == 0 {
		log.Fatal("JWT_SIGNING_KEY environment variable is required")
	}

	tinodeAddr := os.Getenv("TINODE_ADDR")
	if tinodeAddr == "" {
		tinodeAddr = "tinode:16060"
	}
	apiKey := os.Getenv("TINODE_API_KEY")
	if apiKey == "" {
		apiKey = "AQEAAAABAAD_rAp4DJh05a1HAwFT3A6K"
	}

	// Register PocketBase auth hooks for Tinode user sync
	registerTinodeHooks(app, tinodeAddr, apiKey)

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// Bootstrap admin + collections
		if err := autoBootstrap(app); err != nil {
			app.Logger().Warn("Auto-bootstrap failed", "error", err)
		}
		if err := ensureCollections(app); err != nil {
			app.Logger().Warn("Failed to ensure collections", "error", err)
		}

		// Try to connect to Tinode on startup (non-blocking)
		go func() {
			tc, err := tinode.NewClient(tinodeAddr, apiKey, nil)
			if err != nil {
				app.Logger().Warn("Could not connect to Tinode on startup", "addr", tinodeAddr, "error", err)
			} else {
				tc.Close()
				app.Logger().Info("Tinode is reachable", "addr", tinodeAddr)
			}
		}()

		// --- Huma API (OpenAPI docs + typed handlers) ---

		mux := http.NewServeMux()
		config := huma.DefaultConfig("Gather Platform API", "1.0.0")
		config.Info.Description = "Unified API for the Gather platform. Agent auth, skills marketplace, and shop — all in one place."
		api := humago.New(mux, config)

		// Alias /openapi.yaml → /openapi.json (Stoplight Elements references .yaml)
		mux.HandleFunc("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/openapi.json", http.StatusMovedPermanently)
		})

		gatherapi.RegisterAuthRoutes(api, app, challenges, jwtKey)
		gatherapi.RegisterShopRoutes(api, app)
		gatherapi.RegisterSkillRoutes(api, app, jwtKey)
		gatherapi.RegisterReviewRoutes(api, app, jwtKey)
		gatherapi.RegisterProofRoutes(api, app)
		gatherapi.RegisterRankingRoutes(api, app, jwtKey)
		gatherapi.RegisterHelpRoutes(api)

		// Delegate Huma-managed paths to the Huma mux
		delegate := func(re *core.RequestEvent) error {
			mux.ServeHTTP(re.Response, re.Request)
			return nil
		}
		for _, p := range []string{
			"/docs", "/docs/{path...}",
			"/openapi.json", "/openapi.yaml",
			"/schemas/{path...}",
			"/api/auth/health",
			"/api/agents/{path...}",
			"/help",
			"/api/menu", "/api/menu/{path...}",
			"/api/order/{path...}",
			"/api/products", "/api/products/{path...}",
			"/api/feedback",
			"/api/skills/{path...}",
			"/api/skills",
			"/api/reviews/{path...}",
			"/api/reviews",
			"/api/proofs/{path...}",
			"/api/proofs",
			"/api/rankings/{path...}",
			"/api/rankings",
		} {
			e.Router.Any(p, delegate)
		}

		// --- PocketBase-native routes (require PocketBase auth middleware) ---

		e.Router.GET("/api/tinode/credentials", func(re *core.RequestEvent) error {
			return handleTinodeCredentials(re)
		}).Bind(apis.RequireAuth())

		e.Router.POST("/api/sdk/register-agents", func(re *core.RequestEvent) error {
			return handleSDKRegisterAgents(app, re, tinodeAddr, apiKey)
		}).Bind(apis.RequireAuth())

		e.Router.POST("/api/designs/upload", func(re *core.RequestEvent) error {
			return handleDesignUpload(app, re)
		})

		return e.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

// =============================================================================
// Bootstrap
// =============================================================================

func autoBootstrap(app *pocketbase.PocketBase) error {
	adminEmail := os.Getenv("POCKETBASE_ADMIN_EMAIL")
	adminPassword := os.Getenv("POCKETBASE_ADMIN_PASSWORD")
	if adminEmail == "" || adminPassword == "" {
		return nil
	}

	superusers, err := app.FindCollectionByNameOrId("_superusers")
	if err != nil {
		return err
	}

	existing, _ := app.FindAuthRecordByEmail(superusers, adminEmail)
	if existing != nil {
		return nil
	}

	admin := core.NewRecord(superusers)
	admin.Set("email", adminEmail)
	admin.Set("password", adminPassword)

	if err := app.Save(admin); err != nil {
		return err
	}

	app.Logger().Info("Created superuser", "email", adminEmail)
	return nil
}

// ensureCollections creates all PocketBase collections if they don't exist.
func ensureCollections(app *pocketbase.PocketBase) error {
	if err := ensureAgentsCollection(app); err != nil {
		return err
	}
	if err := ensureSDKTokensCollection(app); err != nil {
		return err
	}
	if err := ensureSkillsCollection(app); err != nil {
		return err
	}
	if err := ensureReviewsCollection(app); err != nil {
		return err
	}
	if err := ensureProofsCollection(app); err != nil {
		return err
	}
	if err := ensureArtifactsCollection(app); err != nil {
		return err
	}
	if err := ensureOrdersCollection(app); err != nil {
		return err
	}
	if err := ensureFeedbackCollection(app); err != nil {
		return err
	}
	if err := ensureDesignsCollection(app); err != nil {
		return err
	}
	return nil
}

func ensureAgentsCollection(app *pocketbase.PocketBase) error {
	_, err := app.FindCollectionByNameOrId("agents")
	if err == nil {
		return nil
	}

	c := core.NewBaseCollection("agents")
	c.Fields.Add(
		&core.TextField{Name: "name", Required: true, Max: 100},
		&core.TextField{Name: "description", Max: 500},
		&core.TextField{Name: "public_key", Required: true},
		&core.TextField{Name: "pubkey_fingerprint", Required: true, Max: 128},
		&core.TextField{Name: "twitter_handle", Max: 50},
		&core.BoolField{Name: "verified"},
		&core.TextField{Name: "verification_code", Max: 20},
		&core.TextField{Name: "code_expires_at"},
		&core.SelectField{
			Name:   "agent_type",
			Values: []string{"service", "autonomous"},
		},
	)

	c.AddIndex("idx_agents_pubkey_fp", true, "pubkey_fingerprint", "")
	c.AddIndex("idx_agents_twitter", false, "twitter_handle", "")

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create agents collection: %w", err)
	}
	app.Logger().Info("Created agents collection")
	return nil
}

func ensureSDKTokensCollection(app *pocketbase.PocketBase) error {
	_, err := app.FindCollectionByNameOrId("sdk_tokens")
	if err == nil {
		return nil
	}

	collection := core.NewBaseCollection("sdk_tokens")
	collection.Fields.Add(
		&core.TextField{Name: "token", Required: true},
		&core.TextField{Name: "workspace", Required: true},
		&core.TextField{Name: "user", Required: true},
	)
	collection.AddIndex("idx_sdk_tokens_token", true, "token", "")

	if err := app.Save(collection); err != nil {
		return err
	}
	app.Logger().Info("Created sdk_tokens collection")
	return nil
}

func ensureSkillsCollection(app *pocketbase.PocketBase) error {
	_, err := app.FindCollectionByNameOrId("skills")
	if err == nil {
		return nil
	}

	c := core.NewBaseCollection("skills")
	c.Fields.Add(
		&core.TextField{Name: "name", Required: true, Max: 200},
		&core.TextField{Name: "description", Max: 2000},
		&core.TextField{Name: "source", Max: 500},
		&core.TextField{Name: "category", Max: 100},
		&core.NumberField{Name: "installs"},
		&core.NumberField{Name: "review_count"},
		&core.NumberField{Name: "avg_score"},
		&core.NumberField{Name: "avg_security_score"},
		&core.NumberField{Name: "rank_score"},
	)
	c.AddIndex("idx_skills_category", false, "category", "")
	c.AddIndex("idx_skills_rank", false, "rank_score", "")

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create skills collection: %w", err)
	}
	app.Logger().Info("Created skills collection")
	return nil
}

func ensureReviewsCollection(app *pocketbase.PocketBase) error {
	_, err := app.FindCollectionByNameOrId("reviews")
	if err == nil {
		return nil
	}

	c := core.NewBaseCollection("reviews")
	c.Fields.Add(
		&core.TextField{Name: "skill", Required: true},
		&core.TextField{Name: "agent_id"},
		&core.TextField{Name: "task", Max: 5000},
		&core.SelectField{
			Name:     "status",
			Values:   []string{"pending", "running", "complete", "failed"},
			Required: true,
		},
		&core.NumberField{Name: "score"},
		&core.TextField{Name: "what_worked", Max: 10000},
		&core.TextField{Name: "what_failed", Max: 10000},
		&core.TextField{Name: "skill_feedback", Max: 10000},
		&core.NumberField{Name: "security_score"},
		&core.TextField{Name: "security_notes", Max: 10000},
		&core.TextField{Name: "runner_type", Max: 50},
		&core.TextField{Name: "permission_mode", Max: 50},
		&core.TextField{Name: "agent_model", Max: 100},
		&core.NumberField{Name: "execution_time_ms"},
		&core.TextField{Name: "cli_output", Max: 100000},
		&core.TextField{Name: "proof"},
	)
	c.AddIndex("idx_reviews_skill", false, "skill", "")
	c.AddIndex("idx_reviews_status", false, "status", "")

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create reviews collection: %w", err)
	}
	app.Logger().Info("Created reviews collection")
	return nil
}

func ensureProofsCollection(app *pocketbase.PocketBase) error {
	_, err := app.FindCollectionByNameOrId("proofs")
	if err == nil {
		return nil
	}

	c := core.NewBaseCollection("proofs")
	c.Fields.Add(
		&core.TextField{Name: "review", Required: true},
		&core.JSONField{Name: "claim_data", MaxSize: 100000},
		&core.TextField{Name: "identifier", Max: 500},
		&core.JSONField{Name: "signatures", MaxSize: 10000},
		&core.JSONField{Name: "witnesses", MaxSize: 10000},
		&core.BoolField{Name: "verified"},
	)
	c.AddIndex("idx_proofs_review", false, "review", "")

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create proofs collection: %w", err)
	}
	app.Logger().Info("Created proofs collection")
	return nil
}

func ensureArtifactsCollection(app *pocketbase.PocketBase) error {
	_, err := app.FindCollectionByNameOrId("artifacts")
	if err == nil {
		return nil
	}

	c := core.NewBaseCollection("artifacts")
	c.Fields.Add(
		&core.TextField{Name: "review", Required: true},
		&core.FileField{
			Name:      "file",
			MaxSelect: 1,
			MaxSize:   10 * 1024 * 1024, // 10MB
		},
		&core.TextField{Name: "file_name", Max: 500},
		&core.TextField{Name: "mime_type", Max: 200},
		&core.NumberField{Name: "size_bytes"},
	)
	c.AddIndex("idx_artifacts_review", false, "review", "")

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create artifacts collection: %w", err)
	}
	app.Logger().Info("Created artifacts collection")
	return nil
}

func ensureOrdersCollection(app *pocketbase.PocketBase) error {
	_, err := app.FindCollectionByNameOrId("orders")
	if err == nil {
		return nil
	}

	c := core.NewBaseCollection("orders")
	c.Fields.Add(
		&core.SelectField{
			Name:     "order_type",
			Values:   []string{"product"},
			Required: true,
		},
		&core.SelectField{
			Name:     "status",
			Values:   []string{"awaiting_payment", "confirmed", "fulfilling", "shipped"},
			Required: true,
		},
		&core.TextField{Name: "product_id", Max: 100},
		&core.JSONField{Name: "product_options", MaxSize: 10000},
		&core.JSONField{Name: "shipping_address", MaxSize: 5000},
		&core.URLField{Name: "design_url"},
		&core.TextField{Name: "gelato_product_uid", Max: 200},
		&core.TextField{Name: "total_bch", Max: 50},
		&core.TextField{Name: "payment_address", Max: 100},
		&core.BoolField{Name: "paid"},
		&core.TextField{Name: "tx_id", Max: 100},
		&core.TextField{Name: "gelato_order_id", Max: 100},
		&core.URLField{Name: "tracking_url"},
	)

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create orders collection: %w", err)
	}
	app.Logger().Info("Created orders collection")
	return nil
}

func ensureFeedbackCollection(app *pocketbase.PocketBase) error {
	_, err := app.FindCollectionByNameOrId("feedback")
	if err == nil {
		return nil
	}

	c := core.NewBaseCollection("feedback")
	c.Fields.Add(
		&core.NumberField{Name: "rating"},
		&core.TextField{Name: "message", Max: 5000},
		&core.TextField{Name: "agent_name", Max: 200},
	)

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create feedback collection: %w", err)
	}
	app.Logger().Info("Created feedback collection")
	return nil
}

func ensureDesignsCollection(app *pocketbase.PocketBase) error {
	_, err := app.FindCollectionByNameOrId("designs")
	if err == nil {
		return nil
	}

	c := core.NewBaseCollection("designs")
	c.Fields.Add(
		&core.FileField{
			Name:      "file",
			MaxSelect: 1,
			MaxSize:   20 * 1024 * 1024, // 20MB
		},
		&core.TextField{Name: "original_name", Max: 500},
		&core.TextField{Name: "mime_type", Max: 200},
	)

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create designs collection: %w", err)
	}
	app.Logger().Info("Created designs collection")
	return nil
}

// =============================================================================
// Tinode user sync hooks (from gather-chat/pocketnode/hooks/auth.go)
// =============================================================================

func registerTinodeHooks(app *pocketbase.PocketBase, tinodeAddr, apiKey string) {
	app.OnRecordAuthRequest("users").BindFunc(func(e *core.RecordAuthRequestEvent) error {
		user := e.Record
		pbID := user.Id
		login := fmt.Sprintf("pb_%s", pbID)
		password := generateTinodePassword(pbID)
		displayName := user.GetString("name")
		if displayName == "" {
			displayName = user.GetString("email")
		}

		go func() {
			tc, err := tinode.NewClient(tinodeAddr, apiKey, nil)
			if err != nil {
				app.Logger().Error("Failed to connect to Tinode for user sync", "error", err)
				return
			}
			defer tc.Close()

			tinodeUID, err := tc.EnsureUser(context.Background(), login, password, displayName)
			if err != nil {
				app.Logger().Error("Failed to sync user to Tinode", "pocketbase_id", pbID, "error", err)
			} else {
				app.Logger().Info("User synced to Tinode", "pocketbase_id", pbID, "tinode_uid", tinodeUID)
			}
		}()

		return e.Next()
	})

	app.OnRecordAfterCreateSuccess("users").BindFunc(func(e *core.RecordEvent) error {
		user := e.Record
		pbID := user.Id
		login := fmt.Sprintf("pb_%s", pbID)
		password := generateTinodePassword(pbID)
		displayName := user.GetString("name")
		if displayName == "" {
			displayName = user.GetString("email")
		}

		go func() {
			tc, err := tinode.NewClient(tinodeAddr, apiKey, nil)
			if err != nil {
				app.Logger().Error("Failed to connect to Tinode for new user", "error", err)
				return
			}
			defer tc.Close()

			tinodeUID, err := tc.EnsureUser(context.Background(), login, password, displayName)
			if err != nil {
				app.Logger().Error("Failed to create Tinode user", "pocketbase_id", pbID, "error", err)
			} else {
				app.Logger().Info("Created Tinode user for new registration", "pocketbase_id", pbID, "tinode_uid", tinodeUID)
			}
		}()

		return e.Next()
	})
}

func generateTinodePassword(seed string) string {
	secret := os.Getenv("TINODE_PASSWORD_SECRET")
	if secret == "" {
		secret = "agency_tinode_sync_v1"
	}
	hash := sha256.Sum256([]byte(seed + "_" + secret))
	return hex.EncodeToString(hash[:])[:24]
}

// =============================================================================
// Tinode credentials endpoint (for authenticated users)
// =============================================================================

func handleTinodeCredentials(re *core.RequestEvent) error {
	info, _ := re.RequestInfo()
	if info.Auth == nil {
		return apis.NewUnauthorizedError("Authentication required", nil)
	}

	pbUserID := info.Auth.Id
	login := fmt.Sprintf("pb_%s", pbUserID)
	password := generateTinodePassword(pbUserID)

	return re.JSON(200, map[string]interface{}{
		"login":    login,
		"password": password,
	})
}

// =============================================================================
// Design upload
// =============================================================================

var allowedDesignExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".svg": true,
}

func handleDesignUpload(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	// 20MB limit
	if err := re.Request.ParseMultipartForm(20 << 20); err != nil {
		return apis.NewBadRequestError("Failed to parse multipart form (max 20MB)", err)
	}

	file, header, err := re.Request.FormFile("file")
	if err != nil {
		return apis.NewBadRequestError("Missing 'file' field in multipart form", err)
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedDesignExts[ext] {
		return apis.NewBadRequestError(
			fmt.Sprintf("File type '%s' not allowed. Accepted: png, jpg, jpeg, webp, svg", ext), nil)
	}

	collection, err := app.FindCollectionByNameOrId("designs")
	if err != nil {
		return apis.NewApiError(500, "designs collection not found", nil)
	}

	record := core.NewRecord(collection)
	record.Set("original_name", header.Filename)
	record.Set("mime_type", header.Header.Get("Content-Type"))

	f, err := filesystem.NewFileFromMultipart(header)
	if err != nil {
		return apis.NewApiError(500, "Failed to process uploaded file", err)
	}
	record.Set("file", f)

	if err := app.Save(record); err != nil {
		return apis.NewApiError(500, "Failed to save design", err)
	}

	// PocketBase serves files at /api/files/{collection}/{record_id}/{filename}
	filename := record.GetString("file")
	designURL := fmt.Sprintf("/api/files/designs/%s/%s", record.Id, filename)

	return re.JSON(http.StatusCreated, map[string]string{
		"design_id":  record.Id,
		"design_url": designURL,
	})
}

// =============================================================================
// SDK agent registration (moved from gather-chat PocketNode)
// =============================================================================

type sdkRegisterRequest struct {
	Workspace string   `json:"workspace"`
	Channels  []string `json:"channels"`
	Handles   []string `json:"handles"`
}

type agentCredentials struct {
	Handle      string `json:"handle"`
	BotLogin    string `json:"bot_login"`
	BotPassword string `json:"bot_password"`
	BotUID      string `json:"bot_uid,omitempty"`
}

func handleSDKRegisterAgents(app *pocketbase.PocketBase, re *core.RequestEvent, tinodeAddr, apiKey string) error {
	info, _ := re.RequestInfo()
	if info.Auth == nil {
		return apis.NewUnauthorizedError("Authentication required", nil)
	}

	var req sdkRegisterRequest
	if err := json.NewDecoder(re.Request.Body).Decode(&req); err != nil {
		return apis.NewBadRequestError("Invalid request body", err)
	}

	if req.Workspace == "" {
		return apis.NewBadRequestError("workspace is required", nil)
	}
	if len(req.Handles) == 0 {
		return apis.NewBadRequestError("At least one agent handle is required", nil)
	}

	tc, err := tinode.NewClient(tinodeAddr, apiKey, nil)
	if err != nil {
		app.Logger().Error("Failed to connect to Tinode", "error", err)
		return apis.NewApiError(500, "Failed to connect to chat server", nil)
	}
	defer tc.Close()

	agents := make([]agentCredentials, 0, len(req.Handles))

	for _, handle := range req.Handles {
		login := generateBotLogin(req.Workspace, handle)
		password := generateBotPassword(req.Workspace, handle)
		displayName := formatDisplayName(handle)

		uid, err := tc.EnsureBotUser(context.Background(), login, password, displayName, handle)
		if err != nil {
			app.Logger().Warn("Failed to create bot user", "handle", handle, "error", err)
			continue
		}

		if err := tc.Subscribe(context.Background(), req.Workspace); err != nil {
			app.Logger().Warn("Failed to subscribe bot to workspace", "handle", handle, "error", err)
		}

		for _, channel := range req.Channels {
			if err := tc.Subscribe(context.Background(), channel); err != nil {
				app.Logger().Warn("Failed to subscribe bot to channel", "handle", handle, "channel", channel, "error", err)
			}
		}

		agents = append(agents, agentCredentials{
			Handle:      handle,
			BotLogin:    login,
			BotPassword: password,
			BotUID:      uid,
		})
	}

	tinodeWsURL := os.Getenv("TINODE_WS_URL")
	if tinodeWsURL == "" {
		tinodeWsURL = "ws://localhost:6060/v0/channels"
	}

	return re.JSON(200, map[string]interface{}{
		"success":   true,
		"server":    tinodeWsURL,
		"workspace": req.Workspace,
		"agents":    agents,
	})
}

func generateBotLogin(workspaceID, handle string) string {
	wsHash := sha256.Sum256([]byte(workspaceID))
	wsShort := hex.EncodeToString(wsHash[:])[:8]
	cleanHandle := ""
	for _, c := range handle {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			cleanHandle += string(c)
		}
	}
	return "bot" + wsShort + cleanHandle
}

func generateBotPassword(workspaceID, handle string) string {
	secret := os.Getenv("TINODE_PASSWORD_SECRET")
	if secret == "" {
		secret = "agency_bot_password_v1"
	}
	data := workspaceID + "_" + handle + "_" + secret
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])[:24]
}

func formatDisplayName(handle string) string {
	result := ""
	capitalize := true
	for _, c := range handle {
		if c == '_' {
			result += " "
			capitalize = true
		} else if capitalize && c >= 'a' && c <= 'z' {
			result += string(c - 32)
			capitalize = false
		} else {
			result += string(c)
			capitalize = false
		}
	}
	return result
}

