package hooks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"agency/pocketnode/tinode"
)

// TinodeSync handles synchronization of PocketBase users to Tinode
type TinodeSync struct {
	tc     *tinode.Client
	logger *pocketbase.PocketBase
}

// RegisterAuth sets up authentication hooks for Tinode user sync
func RegisterAuth(app *pocketbase.PocketBase, tc *tinode.Client) {
	sync := &TinodeSync{tc: tc}

	// When user authenticates via OAuth, create/sync Tinode user
	app.OnRecordAuthRequest("users").BindFunc(func(e *core.RecordAuthRequestEvent) error {
		user := e.Record

		// Generate deterministic Tinode credentials
		pbID := user.Id
		login := fmt.Sprintf("pb_%s", pbID)
		password := generatePassword(pbID)
		displayName := user.GetString("name")
		if displayName == "" {
			displayName = user.GetString("email")
		}

		// Ensure Tinode user exists (in background, don't block auth)
		go func() {
			bgCtx := context.Background()
			tinodeUID, err := sync.ensureTinodeUser(bgCtx, login, password, displayName)
			if err != nil {
				app.Logger().Error("Failed to sync user to Tinode",
					"pocketbase_id", pbID,
					"error", err,
				)
			} else {
				app.Logger().Info("User synced to Tinode",
					"pocketbase_id", pbID,
					"tinode_uid", tinodeUID,
				)

				// Update the user record with Tinode UID (optional)
				// This is done in background, failures are logged but not fatal
				if tinodeUID != "" {
					updateCtx := context.Background()
					if err := sync.updateUserTinodeUID(updateCtx, app, user.Id, tinodeUID); err != nil {
						app.Logger().Warn("Failed to store Tinode UID in user record",
							"error", err,
						)
					}
				}
			}
		}()

		return e.Next()
	})

	// When a new user is created (via registration or OAuth), ensure Tinode user
	app.OnRecordAfterCreateSuccess("users").BindFunc(func(e *core.RecordEvent) error {
		user := e.Record
		ctx := context.Background()

		pbID := user.Id
		login := fmt.Sprintf("pb_%s", pbID)
		password := generatePassword(pbID)
		displayName := user.GetString("name")
		if displayName == "" {
			displayName = user.GetString("email")
		}

		tinodeUID, err := sync.ensureTinodeUser(ctx, login, password, displayName)
		if err != nil {
			app.Logger().Error("Failed to create Tinode user on registration",
				"pocketbase_id", pbID,
				"error", err,
			)
			// Don't fail the registration
			return e.Next()
		}

		app.Logger().Info("Created Tinode user for new registration",
			"pocketbase_id", pbID,
			"tinode_uid", tinodeUID,
		)

		return e.Next()
	})
}

// ensureTinodeUser creates a new Tinode client connection and ensures user exists
func (s *TinodeSync) ensureTinodeUser(ctx context.Context, login, password, displayName string) (string, error) {
	// Create a fresh client for this operation
	addr := os.Getenv("TINODE_ADDR")
	if addr == "" {
		addr = "tinode:16060"
	}

	apiKey := os.Getenv("TINODE_API_KEY")
	if apiKey == "" {
		apiKey = "AQEAAAABAAD_rAp4DJh05a1HAwFT3A6K"
	}

	tc, err := tinode.NewClient(addr, apiKey, nil)
	if err != nil {
		return "", fmt.Errorf("failed to connect to Tinode: %w", err)
	}
	defer tc.Close()

	return tc.EnsureUser(ctx, login, password, displayName)
}

// updateUserTinodeUID updates the user record with their Tinode UID
func (s *TinodeSync) updateUserTinodeUID(ctx context.Context, app *pocketbase.PocketBase, userID, tinodeUID string) error {
	record, err := app.FindRecordById("users", userID)
	if err != nil {
		return err
	}

	record.Set("tinode_uid", tinodeUID)
	return app.Save(record)
}

// generatePassword creates a deterministic password from PocketBase user ID
// This allows us to always compute the Tinode password without storing it
func generatePassword(seed string) string {
	secret := os.Getenv("TINODE_PASSWORD_SECRET")
	if secret == "" {
		secret = "agency_tinode_sync_v1"
	}
	hash := sha256.Sum256([]byte(seed + "_" + secret))
	return hex.EncodeToString(hash[:])[:24]
}

// GetTinodeCredentials returns the Tinode login credentials for a PocketBase user
// This can be used by the frontend to authenticate with Tinode
func GetTinodeCredentials(pbUserID string) (login, password string) {
	login = fmt.Sprintf("pb_%s", pbUserID)
	password = generatePassword(pbUserID)
	return
}
