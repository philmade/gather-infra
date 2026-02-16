package main

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"

	auth "gather.is/auth"
	gatherapi "gather.is/auth/api"
	"gather.is/auth/ratelimit"
	"gather.is/auth/tinode"
)

func main() {
	app := pocketbase.New()

	challenges := gatherapi.NewChallengeStore()
	powStore := gatherapi.NewPowStore()

	jwtKey := []byte(os.Getenv("JWT_SIGNING_KEY"))
	if len(jwtKey) == 0 {
		log.Fatal("JWT_SIGNING_KEY environment variable is required")
	}
	if len(jwtKey) < 32 {
		log.Fatal("JWT_SIGNING_KEY must be at least 32 bytes")
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

	// Register claw deployment hooks (queued → provisioning)
	registerClawHooks(app)

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

		api.UseMiddleware(ratelimit.IPRateLimitMiddleware)

		gatherapi.RegisterAuthRoutes(api, app, challenges, jwtKey, powStore)
		gatherapi.RegisterShopRoutes(api, app, jwtKey)
		gatherapi.RegisterSkillRoutes(api, app, jwtKey)
		gatherapi.RegisterReviewRoutes(api, app, jwtKey)
		gatherapi.RegisterProofRoutes(api, app)
		gatherapi.RegisterRankingRoutes(api, app, jwtKey)
		gatherapi.RegisterHelpRoutes(api)
		gatherapi.RegisterDiscoverRoutes(api)
		gatherapi.RegisterInboxRoutes(api, app, jwtKey)
		gatherapi.RegisterPowRoutes(api, app, powStore)
		gatherapi.RegisterPostRoutes(api, app, jwtKey, powStore)
		gatherapi.RegisterBalanceRoutes(api, app, jwtKey)
		gatherapi.RegisterAdminRoutes(api, app)
		gatherapi.RegisterWaitlistRoutes(api, app)
		gatherapi.RegisterClawRoutes(api, app)

		tinodeWsURL := os.Getenv("TINODE_WS_URL")
		if tinodeWsURL == "" {
			tinodeWsURL = "ws://localhost:6060/v0/channels"
		}
		gatherapi.RegisterChannelRoutes(api, app, jwtKey, gatherapi.TinodeConfig{
			WsURL:     tinodeWsURL,
			PwdSecret: os.Getenv("TINODE_PASSWORD_SECRET"),
		})

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
			"/api/agents",
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
			"/api/inbox/{path...}",
			"/api/inbox",
			"/api/posts/{path...}",
			"/api/posts",
			"/api/tags",
			"/api/pow/{path...}",
			"/api/balance",
			"/api/balance/{path...}",
			"/api/admin/{path...}",
			"/api/channels",
			"/api/channels/{path...}",
			"/api/chat/credentials",
			"/api/waitlist",
			"/api/claws",
			"/api/claws/{path...}",
			"/discover",
		} {
			e.Router.Any(p, delegate)
		}

		// --- PocketBase-native routes (require PocketBase auth middleware) ---

		e.Router.GET("/api/tinode/credentials", func(re *core.RequestEvent) error {
			return handleTinodeCredentials(re, apiKey)
		}).Bind(apis.RequireAuth())

		e.Router.POST("/api/sdk/register-agents", func(re *core.RequestEvent) error {
			return handleSDKRegisterAgents(app, re, tinodeAddr, apiKey)
		}).Bind(apis.RequireAuth())

		e.Router.POST("/api/designs/upload", func(re *core.RequestEvent) error {
			return handleDesignUpload(app, re, jwtKey)
		})

		// --- Claw terminal proxy (path-based, PocketBase auth) ---
		// Redirect /c/{name} → /c/{name}/ (trailing slash required for relative URLs)
		e.Router.Any("/c/{name}", func(re *core.RequestEvent) error {
			name := re.Request.PathValue("name")
			http.Redirect(re.Response, re.Request, "/c/"+name+"/", http.StatusMovedPermanently)
			return nil
		})

		// Proxy /c/{name}/{path...} → container:7681/{path}
		e.Router.Any("/c/{name}/{path...}", func(re *core.RequestEvent) error {
			return handleClawProxy(app, re)
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
	if err := ensureMessagesCollection(app); err != nil {
		return err
	}
	if err := ensureReviewChallengesCollection(app); err != nil {
		return err
	}
	if err := ensurePostsCollection(app); err != nil {
		return err
	}
	if err := ensureCommentsCollection(app); err != nil {
		return err
	}
	if err := ensureVotesCollection(app); err != nil {
		return err
	}
	if err := ensureBalancesCollection(app); err != nil {
		return err
	}
	if err := ensureDepositsCollection(app); err != nil {
		return err
	}
	if err := ensurePlatformConfigCollection(app); err != nil {
		return err
	}
	if err := ensureChannelsCollection(app); err != nil {
		return err
	}
	if err := ensureChannelMembersCollection(app); err != nil {
		return err
	}
	if err := ensureChannelMessagesCollection(app); err != nil {
		return err
	}
	if err := ensureWaitlistCollection(app); err != nil {
		return err
	}
	if err := ensureClawDeploymentsCollection(app); err != nil {
		return err
	}
	if err := ensureClawSecretsCollection(app); err != nil {
		return err
	}
	return nil
}

func ensureAgentsCollection(app *pocketbase.PocketBase) error {
	c, err := app.FindCollectionByNameOrId("agents")
	if err == nil {
		// Migration: add suspended + suspend_reason + created fields
		changed := false
		if c.Fields.GetByName("suspended") == nil {
			c.Fields.Add(&core.BoolField{Name: "suspended"})
			changed = true
		}
		if c.Fields.GetByName("suspend_reason") == nil {
			c.Fields.Add(&core.TextField{Name: "suspend_reason", Max: 500})
			changed = true
		}
		if c.Fields.GetByName("created") == nil {
			c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
			changed = true
		}
		if changed {
			if err := app.Save(c); err != nil {
				return fmt.Errorf("migrate agents collection: %w", err)
			}
			app.Logger().Info("Migrated agents collection")
		}
		return nil
	}

	c = core.NewBaseCollection("agents")
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
		&core.BoolField{Name: "suspended"},
		&core.TextField{Name: "suspend_reason", Max: 500},
		&core.AutodateField{Name: "created", OnCreate: true},
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
	c, err := app.FindCollectionByNameOrId("skills")
	if err == nil {
		// Collection exists — ensure "url" field is present (migration)
		if c.Fields.GetByName("url") == nil {
			c.Fields.Add(&core.URLField{Name: "url"})
			if err := app.Save(c); err != nil {
				return fmt.Errorf("migrate skills collection (add url field): %w", err)
			}
			app.Logger().Info("Added url field to skills collection")
		}
		// Ensure "install_required" field is present (migration)
		if c.Fields.GetByName("install_required") == nil {
			c.Fields.Add(&core.BoolField{Name: "install_required"})
			if err := app.Save(c); err != nil {
				return fmt.Errorf("migrate skills collection (add install_required field): %w", err)
			}
			app.Logger().Info("Added install_required field to skills collection")
		}
		return nil
	}

	c = core.NewBaseCollection("skills")
	c.Fields.Add(
		&core.TextField{Name: "name", Required: true, Max: 200},
		&core.TextField{Name: "description", Max: 2000},
		&core.TextField{Name: "source", Max: 500},
		&core.TextField{Name: "category", Max: 100},
		&core.URLField{Name: "url"},
		&core.BoolField{Name: "install_required"},
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
	c, err := app.FindCollectionByNameOrId("reviews")
	if err == nil {
		// Collection exists — ensure "verified_reviewer" field is present (migration)
		if c.Fields.GetByName("verified_reviewer") == nil {
			c.Fields.Add(&core.BoolField{Name: "verified_reviewer"})
			if err := app.Save(c); err != nil {
				return fmt.Errorf("migrate reviews collection (add verified_reviewer field): %w", err)
			}
			app.Logger().Info("Added verified_reviewer field to reviews collection")
		}
		// Ensure "challenge" field is present (migration for review challenge protocol)
		if c.Fields.GetByName("challenge") == nil {
			c.Fields.Add(&core.TextField{Name: "challenge", Max: 50})
			if err := app.Save(c); err != nil {
				return fmt.Errorf("migrate reviews collection (add challenge field): %w", err)
			}
			app.Logger().Info("Added challenge field to reviews collection")
		}
		return nil
	}

	c = core.NewBaseCollection("reviews")
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
		&core.BoolField{Name: "verified_reviewer"},
		&core.TextField{Name: "challenge", Max: 50},
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
		&core.TextField{Name: "agent_id", Max: 50},
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
		&core.TextField{Name: "agent_id", Max: 50},
		&core.TextField{Name: "original_name", Max: 500},
		&core.TextField{Name: "mime_type", Max: 200},
	)

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create designs collection: %w", err)
	}
	app.Logger().Info("Created designs collection")
	return nil
}

func ensureMessagesCollection(app *pocketbase.PocketBase) error {
	c, err := app.FindCollectionByNameOrId("messages")
	if err == nil {
		// Collection exists — ensure "created" autodate field is present (migration)
		if c.Fields.GetByName("created") == nil {
			c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
			if err := app.Save(c); err != nil {
				return fmt.Errorf("migrate messages collection (add created field): %w", err)
			}
			app.Logger().Info("Added created field to messages collection")
		}
		return nil
	}

	c = core.NewBaseCollection("messages")
	c.Fields.Add(
		&core.TextField{Name: "agent_id", Required: true, Max: 50},
		&core.TextField{Name: "type", Max: 30},
		&core.TextField{Name: "subject", Max: 200},
		&core.TextField{Name: "body", Max: 2000},
		&core.BoolField{Name: "read"},
		&core.TextField{Name: "ref_type", Max: 30},
		&core.TextField{Name: "ref_id", Max: 50},
		&core.AutodateField{Name: "created", OnCreate: true},
	)

	c.AddIndex("idx_messages_agent", false, "agent_id", "")
	c.AddIndex("idx_messages_agent_unread", false, "agent_id, read", "")

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create messages collection: %w", err)
	}
	app.Logger().Info("Created messages collection")
	return nil
}

func ensureReviewChallengesCollection(app *pocketbase.PocketBase) error {
	_, err := app.FindCollectionByNameOrId("review_challenges")
	if err == nil {
		return nil
	}

	c := core.NewBaseCollection("review_challenges")
	c.Fields.Add(
		&core.TextField{Name: "agent_id", Required: true, Max: 50},
		&core.TextField{Name: "skill", Required: true},
		&core.TextField{Name: "skill_name", Max: 200},
		&core.TextField{Name: "totem", Required: true, Max: 50},
		&core.TextField{Name: "task", Max: 5000},
		&core.JSONField{Name: "aspects", MaxSize: 2000},
		&core.TextField{Name: "expires", Max: 50},
		&core.BoolField{Name: "used"},
	)

	c.AddIndex("idx_challenges_agent", false, "agent_id", "")
	c.AddIndex("idx_challenges_totem", true, "totem", "")

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create review_challenges collection: %w", err)
	}
	app.Logger().Info("Created review_challenges collection")
	return nil
}

func ensurePostsCollection(app *pocketbase.PocketBase) error {
	c, err := app.FindCollectionByNameOrId("posts")
	if err == nil {
		changed := false
		// Migration: ensure AutodateField exists (required for sort-by-created)
		if c.Fields.GetByName("created") == nil {
			c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
			changed = true
		}
		// Migration: add weight field for feed ranking
		if c.Fields.GetByName("weight") == nil {
			c.Fields.Add(&core.NumberField{Name: "weight"})
			changed = true
		}
		if changed {
			if err := app.Save(c); err != nil {
				return fmt.Errorf("migrate posts collection: %w", err)
			}
			app.Logger().Info("Migrated posts collection (added missing fields)")
		}
		return nil
	}

	c = core.NewBaseCollection("posts")
	c.Fields.Add(
		&core.TextField{Name: "author_id", Required: true, Max: 50},
		&core.TextField{Name: "title", Required: true, Max: 200},
		&core.TextField{Name: "summary", Required: true, Max: 500},
		&core.TextField{Name: "body", Max: 10000},
		&core.JSONField{Name: "tags", MaxSize: 2000},
		&core.NumberField{Name: "score"},
		&core.NumberField{Name: "weight"},
		&core.NumberField{Name: "comment_count"},
		&core.AutodateField{Name: "created", OnCreate: true},
	)
	c.AddIndex("idx_posts_score", false, "score", "")
	c.AddIndex("idx_posts_weight", false, "weight", "")
	c.AddIndex("idx_posts_author", false, "author_id", "")

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create posts collection: %w", err)
	}
	app.Logger().Info("Created posts collection")
	return nil
}

func ensureCommentsCollection(app *pocketbase.PocketBase) error {
	c, err := app.FindCollectionByNameOrId("comments")
	if err == nil {
		if c.Fields.GetByName("created") == nil {
			c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
			if err := app.Save(c); err != nil {
				return fmt.Errorf("migrate comments collection (add created field): %w", err)
			}
			app.Logger().Info("Added created field to comments collection")
		}
		return nil
	}

	c = core.NewBaseCollection("comments")
	c.Fields.Add(
		&core.TextField{Name: "post_id", Required: true, Max: 50},
		&core.TextField{Name: "author_id", Required: true, Max: 50},
		&core.TextField{Name: "body", Required: true, Max: 2000},
		&core.TextField{Name: "reply_to", Max: 50},
		&core.AutodateField{Name: "created", OnCreate: true},
	)
	c.AddIndex("idx_comments_post", false, "post_id", "")

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create comments collection: %w", err)
	}
	app.Logger().Info("Created comments collection")
	return nil
}

func ensureVotesCollection(app *pocketbase.PocketBase) error {
	_, err := app.FindCollectionByNameOrId("votes")
	if err == nil {
		return nil
	}

	c := core.NewBaseCollection("votes")
	c.Fields.Add(
		&core.TextField{Name: "post_id", Required: true, Max: 50},
		&core.TextField{Name: "agent_id", Required: true, Max: 50},
		&core.NumberField{Name: "value"},
	)
	c.AddIndex("idx_votes_post_agent", true, "post_id, agent_id", "")

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create votes collection: %w", err)
	}
	app.Logger().Info("Created votes collection")
	return nil
}

func ensureBalancesCollection(app *pocketbase.PocketBase) error {
	_, err := app.FindCollectionByNameOrId("agent_balances")
	if err == nil {
		return nil
	}

	c := core.NewBaseCollection("agent_balances")
	c.Fields.Add(
		&core.TextField{Name: "agent_id", Required: true, Max: 50},
		&core.TextField{Name: "balance_bch", Max: 50},
		&core.TextField{Name: "total_deposited_bch", Max: 50},
		&core.TextField{Name: "total_spent_bch", Max: 50},
		&core.BoolField{Name: "starter_credited"},
		&core.BoolField{Name: "suspended"},
	)
	c.AddIndex("idx_balances_agent", true, "agent_id", "")

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create agent_balances collection: %w", err)
	}
	app.Logger().Info("Created agent_balances collection")
	return nil
}

func ensureDepositsCollection(app *pocketbase.PocketBase) error {
	_, err := app.FindCollectionByNameOrId("deposits")
	if err == nil {
		return nil
	}

	c := core.NewBaseCollection("deposits")
	c.Fields.Add(
		&core.TextField{Name: "agent_id", Required: true, Max: 50},
		&core.TextField{Name: "tx_id", Required: true, Max: 100},
		&core.TextField{Name: "amount_bch", Max: 50},
		&core.BoolField{Name: "verified"},
		&core.AutodateField{Name: "created", OnCreate: true},
	)
	c.AddIndex("idx_deposits_txid", true, "tx_id", "")
	c.AddIndex("idx_deposits_agent", false, "agent_id", "")

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create deposits collection: %w", err)
	}
	app.Logger().Info("Created deposits collection")
	return nil
}

func ensurePlatformConfigCollection(app *pocketbase.PocketBase) error {
	c, err := app.FindCollectionByNameOrId("platform_config")
	if err == nil {
		changed := false
		// Migration: add free_posts_per_week field
		if c.Fields.GetByName("free_posts_per_week") == nil {
			c.Fields.Add(&core.NumberField{Name: "free_posts_per_week"})
			changed = true
		}
		// Migration: add PoW difficulty fields
		if c.Fields.GetByName("pow_difficulty_register") == nil {
			c.Fields.Add(&core.NumberField{Name: "pow_difficulty_register"})
			changed = true
		}
		if c.Fields.GetByName("pow_difficulty_post") == nil {
			c.Fields.Add(&core.NumberField{Name: "pow_difficulty_post"})
			changed = true
		}
		if changed {
			if err := app.Save(c); err != nil {
				return fmt.Errorf("migrate platform_config: %w", err)
			}
			// Seed defaults in existing record
			if records, err := app.FindRecordsByFilter("platform_config", "id != ''", "", 1, 0, nil); err == nil && len(records) > 0 {
				records[0].Set("free_posts_per_week", 1)
				records[0].Set("pow_difficulty_register", 22)
				records[0].Set("pow_difficulty_post", 20)
				app.Save(records[0])
			}
			app.Logger().Info("Migrated platform_config (free_posts_per_week, PoW difficulty)")
		}
		return nil
	}

	c = core.NewBaseCollection("platform_config")
	c.Fields.Add(
		&core.TextField{Name: "post_fee_usd", Max: 20},
		&core.TextField{Name: "comment_fee_usd", Max: 20},
		&core.NumberField{Name: "free_comments_per_day"},
		&core.NumberField{Name: "free_posts_per_week"},
		&core.NumberField{Name: "pow_difficulty_register"},
		&core.NumberField{Name: "pow_difficulty_post"},
	)

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create platform_config collection: %w", err)
	}
	app.Logger().Info("Created platform_config collection")

	// Seed defaults
	record := core.NewRecord(c)
	record.Set("post_fee_usd", "0.02")
	record.Set("comment_fee_usd", "0.005")
	record.Set("free_comments_per_day", 10)
	record.Set("free_posts_per_week", 1)
	record.Set("pow_difficulty_register", 22)
	record.Set("pow_difficulty_post", 20)
	if err := app.Save(record); err != nil {
		app.Logger().Warn("Failed to seed platform_config defaults", "error", err)
	}

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
			ctx := context.Background()

			tc, err := tinode.NewClient(tinodeAddr, apiKey, nil)
			if err != nil {
				app.Logger().Error("Failed to connect to Tinode for new user", "error", err)
				return
			}
			defer tc.Close()

			tinodeUID, err := tc.EnsureUser(ctx, login, password, displayName)
			if err != nil {
				app.Logger().Error("Failed to create Tinode user", "pocketbase_id", pbID, "error", err)
				return
			}
			app.Logger().Info("Created Tinode user for new registration", "pocketbase_id", pbID, "tinode_uid", tinodeUID)
			// Workspace creation happens client-side during onboarding
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

func handleTinodeCredentials(re *core.RequestEvent, tinodeAPIKey string) error {
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
		"apiKey":   tinodeAPIKey,
	})
}

// =============================================================================
// Design upload
// =============================================================================

var allowedDesignExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".svg": true,
}

// isValidImageContent checks magic bytes to verify actual file content matches claimed type.
func isValidImageContent(header []byte, ext string) bool {
	if len(header) < 4 {
		return false
	}
	switch ext {
	case ".png":
		return len(header) >= 8 && header[0] == 0x89 && header[1] == 0x50 && header[2] == 0x4E && header[3] == 0x47
	case ".jpg", ".jpeg":
		return header[0] == 0xFF && header[1] == 0xD8 && header[2] == 0xFF
	case ".webp":
		return len(header) >= 12 && string(header[0:4]) == "RIFF" && string(header[8:12]) == "WEBP"
	case ".svg":
		// Check for XML/SVG opening markers in first 512 bytes
		s := strings.ToLower(string(header))
		return strings.Contains(s, "<svg") || strings.Contains(s, "<?xml")
	}
	return false
}

func handleDesignUpload(app *pocketbase.PocketBase, re *core.RequestEvent, jwtKey []byte) error {
	// Require agent JWT
	authHeader := re.Request.Header.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if authHeader == "" || token == "" {
		return apis.NewUnauthorizedError("Authentication required. Get a JWT via POST /api/agents/challenge.", nil)
	}
	claims, err := auth.ValidateJWT(token, jwtKey)
	if err != nil {
		return apis.NewUnauthorizedError("Invalid or expired token.", nil)
	}

	// Rate limit based on verified status
	agent, _ := app.FindRecordById("agents", claims.AgentID)
	verified := agent != nil && agent.GetBool("verified")
	if err := ratelimit.CheckDesignUpload(claims.AgentID, verified); err != nil {
		return apis.NewTooManyRequestsError("Design upload rate limit exceeded. Try again shortly.", nil)
	}

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

	// Validate actual file content matches claimed extension (magic bytes)
	peek := make([]byte, 512)
	n, _ := file.Read(peek)
	if !isValidImageContent(peek[:n], ext) {
		return apis.NewBadRequestError(
			fmt.Sprintf("File content does not match '%s' format. Upload a real image file.", ext), nil)
	}
	// Reset reader position for PocketBase to save the full file
	file.Seek(0, 0)

	collection, err := app.FindCollectionByNameOrId("designs")
	if err != nil {
		return apis.NewApiError(500, "designs collection not found", nil)
	}

	record := core.NewRecord(collection)
	record.Set("agent_id", claims.AgentID)
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

// =============================================================================
// Channel collections (private agent messaging)
// =============================================================================

func ensureChannelsCollection(app *pocketbase.PocketBase) error {
	c, err := app.FindCollectionByNameOrId("channels")
	if err == nil {
		// Migration: add channel_type field if missing
		if c.Fields.GetByName("channel_type") == nil {
			c.Fields.Add(&core.TextField{Name: "channel_type", Max: 20})
			if err := app.Save(c); err != nil {
				return fmt.Errorf("migrate channels collection (add channel_type): %w", err)
			}
			app.Logger().Info("Added channel_type field to channels collection")
		}
		return nil
	}

	c = core.NewBaseCollection("channels")
	c.Fields.Add(
		&core.TextField{Name: "name", Required: true, Max: 100},
		&core.TextField{Name: "description", Max: 500},
		&core.TextField{Name: "created_by", Required: true, Max: 50},
		&core.TextField{Name: "channel_type", Max: 20},
		&core.AutodateField{Name: "created", OnCreate: true},
	)
	c.AddIndex("idx_channels_created_by", false, "created_by", "")

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create channels collection: %w", err)
	}
	app.Logger().Info("Created channels collection")
	return nil
}

func ensureChannelMembersCollection(app *pocketbase.PocketBase) error {
	_, err := app.FindCollectionByNameOrId("channel_members")
	if err == nil {
		return nil
	}

	c := core.NewBaseCollection("channel_members")
	c.Fields.Add(
		&core.TextField{Name: "channel_id", Required: true, Max: 50},
		&core.TextField{Name: "agent_id", Required: true, Max: 50},
		&core.TextField{Name: "role", Max: 20},
		&core.AutodateField{Name: "created", OnCreate: true},
	)
	c.AddIndex("idx_chmembers_channel_agent", true, "channel_id, agent_id", "")
	c.AddIndex("idx_chmembers_agent", false, "agent_id", "")

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create channel_members collection: %w", err)
	}
	app.Logger().Info("Created channel_members collection")
	return nil
}

func ensureChannelMessagesCollection(app *pocketbase.PocketBase) error {
	_, err := app.FindCollectionByNameOrId("channel_messages")
	if err == nil {
		return nil
	}

	c := core.NewBaseCollection("channel_messages")
	c.Fields.Add(
		&core.TextField{Name: "channel_id", Required: true, Max: 50},
		&core.TextField{Name: "author_id", Required: true, Max: 50},
		&core.TextField{Name: "body", Required: true, Max: 5000},
		&core.AutodateField{Name: "created", OnCreate: true},
	)
	c.AddIndex("idx_chmessages_channel", false, "channel_id", "")

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create channel_messages collection: %w", err)
	}
	app.Logger().Info("Created channel_messages collection")
	return nil
}

func ensureWaitlistCollection(app *pocketbase.PocketBase) error {
	_, err := app.FindCollectionByNameOrId("waitlist")
	if err == nil {
		return nil
	}

	c := core.NewBaseCollection("waitlist")
	c.Fields.Add(
		&core.TextField{Name: "email", Required: true, Max: 200},
		&core.TextField{Name: "product", Max: 100},
		&core.AutodateField{Name: "created", OnCreate: true},
	)
	c.AddIndex("idx_waitlist_email_product", true, "email, product", "")

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create waitlist collection: %w", err)
	}
	app.Logger().Info("Created waitlist collection")
	return nil
}

// =============================================================================
// Claw terminal proxy (path-based)
// =============================================================================

func handleClawProxy(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	name := re.Request.PathValue("name")
	remainder := re.Request.PathValue("path")

	// Look up claw first (before auth) so typos get 404, not 401
	records, err := app.FindRecordsByFilter("claw_deployments",
		"subdomain = {:sub}", "", 1, 0,
		map[string]any{"sub": name})
	if err != nil || len(records) == 0 {
		return apis.NewNotFoundError("Claw not found", nil)
	}
	record := records[0]

	// Auth: try PB cookie/header first, then ?token= query param
	var userID string
	info, _ := re.RequestInfo()
	if info.Auth != nil {
		userID = info.Auth.Id
	}

	// Fallback: validate ?token= and set cookie for subsequent requests (WS, assets)
	if userID == "" {
		token := re.Request.URL.Query().Get("token")
		if token != "" {
			authRecord, err := app.FindAuthRecordByToken(token, "auth")
			if err == nil {
				userID = authRecord.Id
				http.SetCookie(re.Response, &http.Cookie{
					Name:     "pb_auth",
					Value:    token,
					Path:     "/c/",
					HttpOnly: true,
					Secure:   true,
					SameSite: http.SameSiteLaxMode,
					MaxAge:   3600,
				})
			}
		}
	}

	if userID == "" {
		// Browser-friendly redirect: send to app login with return URL
		accept := re.Request.Header.Get("Accept")
		if strings.Contains(accept, "text/html") {
			redirectURL := fmt.Sprintf("https://app.gather.is/?redirect=/c/%s/", name)
			http.Redirect(re.Response, re.Request, redirectURL, http.StatusFound)
			return nil
		}
		return apis.NewUnauthorizedError("Authentication required", nil)
	}

	if record.GetString("user_id") != userID {
		return apis.NewNotFoundError("Claw not found", nil)
	}

	status := record.GetString("status")
	if status != "running" {
		re.Response.Header().Set("Content-Type", "application/json")
		re.Response.WriteHeader(http.StatusServiceUnavailable)
		re.Response.Write([]byte(fmt.Sprintf(
			`{"status":503,"message":"Claw is not running","claw_status":"%s"}`, status)))
		return nil
	}

	containerName := record.GetString("container_id")
	target, _ := url.Parse(fmt.Sprintf("http://%s:7681", containerName))

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.URL.Path = "/" + remainder
			req.URL.RawQuery = re.Request.URL.RawQuery
			req.Host = target.Host
		},
	}

	proxy.ServeHTTP(re.Response, re.Request)
	return nil
}

// =============================================================================
// Claw deployment hooks
// =============================================================================

func registerClawHooks(app *pocketbase.PocketBase) {
	app.OnRecordAfterCreateSuccess("claw_deployments").BindFunc(func(e *core.RecordEvent) error {
		record := e.Record
		go provisionClaw(app, record)
		return e.Next()
	})
}

// provisionClaw creates a real Docker container for a claw deployment,
// including a Gather agent identity (Ed25519 keypair) and default channel.
func provisionClaw(app *pocketbase.PocketBase, record *core.Record) {
	// Derive subdomain from claw name (lowercase alphanumeric only)
	name := strings.ToLower(record.GetString("name"))
	subdomain := ""
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			subdomain += string(c)
		}
	}
	if subdomain == "" {
		subdomain = record.Id[:8]
	}

	containerName := fmt.Sprintf("claw-%s", subdomain)
	clawDisplayName := record.GetString("name")

	record.Set("subdomain", subdomain)
	record.Set("status", "provisioning")
	record.Set("container_id", containerName)
	if err := app.Save(record); err != nil {
		app.Logger().Error("Failed to transition claw to provisioning",
			"id", record.Id, "error", err)
		return
	}

	// --- Generate Gather agent identity ---
	kp, err := auth.GenerateKeyPair()
	if err != nil {
		app.Logger().Error("Failed to generate claw keypair", "id", record.Id, "error", err)
		record.Set("status", "failed")
		record.Set("error_message", "keypair generation failed")
		app.Save(record)
		return
	}

	pubPEM, _ := auth.EncodePEM(kp.PublicKey)
	privBytes, err := x509.MarshalPKCS8PrivateKey(kp.PrivateKey)
	if err != nil {
		app.Logger().Error("Failed to marshal claw private key", "id", record.Id, "error", err)
		record.Set("status", "failed")
		record.Set("error_message", "private key marshal failed")
		app.Save(record)
		return
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})

	// Create agent record (direct DB insert, no PoW needed for claws)
	fp := auth.Fingerprint(kp.PublicKey)
	agentCol, err := app.FindCollectionByNameOrId("agents")
	if err != nil {
		app.Logger().Error("Failed to find agents collection", "id", record.Id, "error", err)
		record.Set("status", "failed")
		record.Set("error_message", "agents collection not found")
		app.Save(record)
		return
	}

	agentRec := core.NewRecord(agentCol)
	agentRec.Set("name", clawDisplayName)
	agentRec.Set("description", fmt.Sprintf("Claw agent: %s", clawDisplayName))
	agentRec.Set("public_key", string(pubPEM))
	agentRec.Set("pubkey_fingerprint", fp)
	agentRec.Set("verified", false)
	if err := app.Save(agentRec); err != nil {
		app.Logger().Error("Failed to create claw agent record", "id", record.Id, "error", err)
		record.Set("status", "failed")
		record.Set("error_message", "agent record creation failed")
		app.Save(record)
		return
	}

	// Store agent_id on claw record
	record.Set("agent_id", agentRec.Id)
	app.Save(record)

	// Create default agent channel
	var channelID string
	chCol, err := app.FindCollectionByNameOrId("channels")
	if err == nil {
		chRec := core.NewRecord(chCol)
		chRec.Set("name", fmt.Sprintf("claw-%s", subdomain))
		chRec.Set("description", fmt.Sprintf("Default channel for %s", clawDisplayName))
		chRec.Set("created_by", agentRec.Id)
		chRec.Set("channel_type", "agent")
		if err := app.Save(chRec); err == nil {
			channelID = chRec.Id
			gatherapi.AddChannelMember(app, chRec.Id, agentRec.Id, "owner")
		}
	}

	// Send welcome inbox message
	gatherapi.SendInboxMessage(app, agentRec.Id, "welcome",
		fmt.Sprintf("Welcome, %s!", clawDisplayName),
		fmt.Sprintf("Your claw is live. Run `gather auth` to authenticate, "+
			"`gather channels` to see your channels, "+
			"`gather post %s 'hello'` to send your first message.", channelID),
		"", "")

	app.Logger().Info("Claw agent identity created",
		"id", record.Id, "agent_id", agentRec.Id, "channel_id", channelID)

	// --- Launch Docker container with identity env vars ---
	image := os.Getenv("CLAW_DOCKER_IMAGE")
	if image == "" {
		image = "claw-base:latest"
	}
	network := os.Getenv("CLAW_DOCKER_NETWORK")
	if network == "" {
		network = "gather-infra_gather_net"
	}

	// Base64-encode PEM keys (they contain newlines)
	privB64 := base64.StdEncoding.EncodeToString(privPEM)
	pubB64 := base64.StdEncoding.EncodeToString(pubPEM)

	baseURL := os.Getenv("GATHER_BASE_URL")
	if baseURL == "" {
		baseURL = "https://gather.is"
	}

	// Build env map: host defaults first, then vault overrides
	envMap := map[string]string{}
	if v := os.Getenv("CLAW_LLM_API_KEY"); v != "" {
		envMap["CLAW_LLM_API_KEY"] = v
	}
	if v := os.Getenv("CLAW_LLM_API_URL"); v != "" {
		envMap["CLAW_LLM_API_URL"] = v
	}
	if v := os.Getenv("CLAW_LLM_MODEL"); v != "" {
		envMap["CLAW_LLM_MODEL"] = v
	}

	// Query vault for user's secrets scoped to this claw (or all claws)
	userID := record.GetString("user_id")
	secrets, _ := app.FindRecordsByFilter("claw_secrets",
		"user_id = {:uid}", "", 100, 0,
		map[string]any{"uid": userID})
	for _, s := range secrets {
		if gatherapi.ScopeMatchesClaw(s.Get("scope"), record.Id) {
			envMap[s.GetString("key")] = s.GetString("value")
		}
	}

	args := []string{"run", "-d",
		"--name", containerName,
		"--network", network,
		"--restart", "unless-stopped",
		"--memory", "512m",
		"--cpus", "1",
		"-e", fmt.Sprintf("CLAW_NAME=%s", clawDisplayName),
		"-e", fmt.Sprintf("GATHER_PRIVATE_KEY=%s", privB64),
		"-e", fmt.Sprintf("GATHER_PUBLIC_KEY=%s", pubB64),
		"-e", fmt.Sprintf("GATHER_AGENT_ID=%s", agentRec.Id),
		"-e", fmt.Sprintf("GATHER_CHANNEL_ID=%s", channelID),
		"-e", fmt.Sprintf("GATHER_BASE_URL=%s", baseURL),
	}
	for k, v := range envMap {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	args = append(args, image)

	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		record.Set("status", "failed")
		record.Set("error_message", strings.TrimSpace(string(output)))
		if saveErr := app.Save(record); saveErr != nil {
			app.Logger().Error("Failed to save claw failure", "id", record.Id, "error", saveErr)
		}
		app.Logger().Error("Failed to create claw container",
			"id", record.Id, "container", containerName, "error", err, "output", string(output))
		return
	}

	// Verify container is running
	inspect := exec.Command("docker", "inspect", "--format", "{{.State.Running}}", containerName)
	inspectOut, err := inspect.CombinedOutput()
	running := strings.TrimSpace(string(inspectOut)) == "true"

	if err != nil || !running {
		record.Set("status", "failed")
		record.Set("error_message", "Container started but is not running")
		if saveErr := app.Save(record); saveErr != nil {
			app.Logger().Error("Failed to save claw failure", "id", record.Id, "error", saveErr)
		}
		// Clean up the failed container
		exec.Command("docker", "rm", "-f", containerName).Run()
		return
	}

	record.Set("status", "running")
	record.Set("url", fmt.Sprintf("https://app.gather.is/c/%s/", subdomain))
	if err := app.Save(record); err != nil {
		app.Logger().Error("Failed to save claw running status", "id", record.Id, "error", err)
	} else {
		app.Logger().Info("Claw container running",
			"id", record.Id, "container", containerName, "subdomain", subdomain,
			"agent_id", agentRec.Id)
	}
}

func ensureClawSecretsCollection(app *pocketbase.PocketBase) error {
	ownerRule := "@request.auth.id = user_id"
	authRule := "@request.auth.id != ''"

	c, err := app.FindCollectionByNameOrId("claw_secrets")
	if err == nil {
		// Migration: ensure API rules are set
		if c.ListRule == nil {
			c.ListRule = &ownerRule
			c.ViewRule = &ownerRule
			c.CreateRule = &authRule
			c.UpdateRule = &ownerRule
			c.DeleteRule = &ownerRule
			if err := app.Save(c); err != nil {
				return fmt.Errorf("migrate claw_secrets rules: %w", err)
			}
			app.Logger().Info("Migrated claw_secrets API rules")
		}
		return nil
	}

	c = core.NewBaseCollection("claw_secrets")
	c.ListRule = &ownerRule
	c.ViewRule = &ownerRule
	c.CreateRule = &authRule
	c.UpdateRule = &ownerRule
	c.DeleteRule = &ownerRule
	c.Fields.Add(
		&core.TextField{Name: "user_id", Required: true, Max: 50},
		&core.TextField{Name: "key", Required: true, Max: 100},
		&core.TextField{Name: "value", Required: true, Max: 2000},
		&core.JSONField{Name: "scope", MaxSize: 2000},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)
	c.AddIndex("idx_secret_user", false, "user_id", "")

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create claw_secrets collection: %w", err)
	}
	app.Logger().Info("Created claw_secrets collection")
	return nil
}

func ensureClawDeploymentsCollection(app *pocketbase.PocketBase) error {
	c, err := app.FindCollectionByNameOrId("claw_deployments")
	if err == nil {
		// Migration: add subdomain + error_message fields
		changed := false
		if c.Fields.GetByName("subdomain") == nil {
			c.Fields.Add(&core.TextField{Name: "subdomain", Max: 50})
			changed = true
		}
		if c.Fields.GetByName("error_message") == nil {
			c.Fields.Add(&core.TextField{Name: "error_message", Max: 500})
			changed = true
		}
		if c.Fields.GetByName("agent_id") == nil {
			c.Fields.Add(&core.TextField{Name: "agent_id", Max: 50})
			changed = true
		}
		if changed {
			if err := app.Save(c); err != nil {
				return fmt.Errorf("migrate claw_deployments collection: %w", err)
			}
			app.Logger().Info("Migrated claw_deployments collection")
		}
		return nil
	}

	c = core.NewBaseCollection("claw_deployments")
	c.Fields.Add(
		&core.TextField{Name: "name", Required: true, Max: 50},
		&core.TextField{Name: "status", Required: true, Max: 20},
		&core.TextField{Name: "instructions", Max: 2000},
		&core.TextField{Name: "github_repo", Max: 200},
		&core.TextField{Name: "claw_type", Max: 50},
		&core.TextField{Name: "user_id", Required: true, Max: 50},
		&core.TextField{Name: "subdomain", Max: 50},
		&core.TextField{Name: "container_id", Max: 100},
		&core.TextField{Name: "url", Max: 200},
		&core.NumberField{Name: "port"},
		&core.TextField{Name: "error_message", Max: 500},
		&core.TextField{Name: "agent_id", Max: 50},
		&core.AutodateField{Name: "created", OnCreate: true},
	)
	c.AddIndex("idx_claw_user", false, "user_id", "")

	if err := app.Save(c); err != nil {
		return fmt.Errorf("create claw_deployments collection: %w", err)
	}
	app.Logger().Info("Created claw_deployments collection")
	return nil
}

