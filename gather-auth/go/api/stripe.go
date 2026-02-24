package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pocketbase/pocketbase"
)

// -----------------------------------------------------------------------------
// Request / Response types
// -----------------------------------------------------------------------------

type CreateCheckoutInput struct {
	Authorization string `header:"Authorization" doc:"Bearer PocketBase auth token" required:"true"`
	ID            string `path:"id" doc:"Claw deployment ID"`
}

type CreateCheckoutOutput struct {
	Body struct {
		URL string `json:"url" doc:"Stripe Checkout URL to redirect the user to"`
	}
}

type StripeWebhookInput struct {
	Body struct {
		RawBody string `json:"-"` // not used by huma — we read raw body manually
	}
}

type StripeWebhookOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

// -----------------------------------------------------------------------------
// Route registration
// -----------------------------------------------------------------------------

func RegisterStripeRoutes(api huma.API, app *pocketbase.PocketBase) {
	huma.Register(api, huma.Operation{
		OperationID: "create-claw-checkout",
		Method:      "POST",
		Path:        "/api/claws/{id}/checkout",
		Summary:     "Create Stripe checkout session",
		Description: "Creates a Stripe Checkout Session for a claw subscription. Returns a URL to redirect to.",
		Tags:        []string{"Claws", "Billing"},
	}, CreateCheckoutSession(app))

	huma.Register(api, huma.Operation{
		OperationID: "stripe-webhook",
		Method:      "POST",
		Path:        "/api/stripe/webhook",
		Summary:     "Stripe webhook receiver",
		Description: "Receives Stripe webhook events. Verified via HMAC-SHA256 signature.",
		Tags:        []string{"Billing"},
	}, HandleStripeWebhook(app))
}

// -----------------------------------------------------------------------------
// POST /api/claws/{id}/checkout
// -----------------------------------------------------------------------------

// stripeEnv returns a Stripe env var, using _TEST suffixed vars when STRIPE_MODE=test.
func stripeEnv(key string) string {
	if os.Getenv("STRIPE_MODE") == "test" {
		if v := os.Getenv(key + "_TEST"); v != "" {
			return v
		}
	}
	return os.Getenv(key)
}

// clawPriceID returns the Stripe Price ID for a claw subscription tier.
func clawPriceID(clawType string) string {
	switch clawType {
	case "pro":
		return stripeEnv("STRIPE_PRICE_PRO")
	case "max":
		return stripeEnv("STRIPE_PRICE_MAX")
	default: // "lite" or anything else
		return stripeEnv("STRIPE_PRICE_LITE")
	}
}

func CreateCheckoutSession(app *pocketbase.PocketBase) func(ctx context.Context, input *CreateCheckoutInput) (*CreateCheckoutOutput, error) {
	return func(ctx context.Context, input *CreateCheckoutInput) (*CreateCheckoutOutput, error) {
		stripeKey := stripeEnv("STRIPE_SECRET_KEY")
		if stripeKey == "" {
			return nil, huma.Error500InternalServerError("Stripe not configured")
		}

		userRecord, err := extractPBUserRecord(app, input.Authorization)
		if err != nil {
			return nil, huma.Error401Unauthorized("Authentication required")
		}
		userID := userRecord.Id

		record, err := app.FindRecordById("claw_deployments", input.ID)
		if err != nil {
			return nil, huma.Error404NotFound("Deployment not found")
		}
		if record.GetString("user_id") != userID {
			return nil, huma.Error404NotFound("Deployment not found")
		}

		if record.GetBool("paid") {
			return nil, huma.Error422UnprocessableEntity("This claw is already paid")
		}

		priceID := clawPriceID(record.GetString("claw_type"))
		if priceID == "" {
			return nil, huma.Error500InternalServerError("No Stripe price configured for this claw tier")
		}

		// Build Stripe Checkout Session via form-encoded POST
		baseURL := os.Getenv("GATHER_PUBLIC_URL")
		if baseURL == "" {
			baseURL = "https://gather.is"
		}

		form := url.Values{}
		form.Set("mode", "subscription")
		form.Set("line_items[0][price]", priceID)
		form.Set("line_items[0][quantity]", "1")
		form.Set("success_url", fmt.Sprintf("%s/?stripe=success&claw=%s", baseURL, input.ID))
		form.Set("cancel_url", fmt.Sprintf("%s/?stripe=cancel&claw=%s", baseURL, input.ID))
		form.Set("metadata[claw_id]", input.ID)
		form.Set("metadata[user_id]", userID)
		form.Set("client_reference_id", input.ID)

		// Pre-fill email so user doesn't have to re-enter it
		if email := userRecord.GetString("email"); email != "" {
			form.Set("customer_email", email)
		}

		req, _ := http.NewRequestWithContext(ctx, "POST",
			"https://api.stripe.com/v1/checkout/sessions",
			strings.NewReader(form.Encode()))
		req.Header.Set("Authorization", "Bearer "+stripeKey)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			app.Logger().Error("Stripe API request failed", "error", err)
			return nil, huma.Error502BadGateway("Failed to reach Stripe")
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			app.Logger().Error("Stripe checkout creation failed",
				"status", resp.StatusCode, "body", string(body))
			return nil, huma.Error502BadGateway("Stripe returned an error")
		}

		var stripeResp struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		}
		if err := json.Unmarshal(body, &stripeResp); err != nil {
			return nil, huma.Error500InternalServerError("Failed to parse Stripe response")
		}

		// Save session ID on claw record
		record.Set("stripe_session_id", stripeResp.ID)
		if err := app.Save(record); err != nil {
			app.Logger().Error("Failed to save stripe_session_id", "id", input.ID, "error", err)
		}

		out := &CreateCheckoutOutput{}
		out.Body.URL = stripeResp.URL
		return out, nil
	}
}

// -----------------------------------------------------------------------------
// POST /api/stripe/webhook
// -----------------------------------------------------------------------------

func HandleStripeWebhook(app *pocketbase.PocketBase) func(ctx context.Context, input *StripeWebhookInput) (*StripeWebhookOutput, error) {
	return func(ctx context.Context, input *StripeWebhookInput) (*StripeWebhookOutput, error) {
		// Huma doesn't give us raw body access easily, so this handler is
		// registered as a raw HTTP handler wrapper. However, for the Huma
		// registration to work, we return a basic OK here. The actual webhook
		// logic is in HandleStripeWebhookRaw which is wired as a PocketBase route.
		out := &StripeWebhookOutput{}
		out.Body.OK = true
		return out, nil
	}
}

// HandleStripeWebhookRaw is the actual webhook handler that reads the raw body
// and verifies the Stripe signature. Wired as a PocketBase-native route.
func HandleStripeWebhookRaw(app *pocketbase.PocketBase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		webhookSecret := stripeEnv("STRIPE_WEBHOOK_SECRET")
		if webhookSecret == "" {
			app.Logger().Warn("STRIPE_WEBHOOK_SECRET not set, rejecting webhook")
			http.Error(w, "Webhook not configured", http.StatusInternalServerError)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB max
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}

		sigHeader := r.Header.Get("Stripe-Signature")
		if !verifyStripeSignature(body, sigHeader, webhookSecret) {
			app.Logger().Warn("Stripe webhook signature verification failed")
			http.Error(w, "Invalid signature", http.StatusBadRequest)
			return
		}

		// Parse event
		var event struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(body, &event); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		switch event.Type {
		case "checkout.session.completed":
			var data struct {
				Object struct {
					ClientReferenceID string `json:"client_reference_id"`
					Metadata          struct {
						ClawID string `json:"claw_id"`
						UserID string `json:"user_id"`
					} `json:"metadata"`
				} `json:"object"`
			}
			if err := json.Unmarshal(event.Data, &data); err != nil {
				app.Logger().Error("Failed to parse checkout.session.completed", "error", err)
				w.WriteHeader(http.StatusOK)
				return
			}

			clawID := data.Object.ClientReferenceID
			if clawID == "" {
				clawID = data.Object.Metadata.ClawID
			}
			if clawID == "" {
				app.Logger().Warn("Stripe webhook: no claw_id in checkout session")
				w.WriteHeader(http.StatusOK)
				return
			}

			record, err := app.FindRecordById("claw_deployments", clawID)
			if err != nil {
				app.Logger().Warn("Stripe webhook: claw not found", "claw_id", clawID)
				w.WriteHeader(http.StatusOK)
				return
			}

			record.Set("paid", true)
			record.Set("trial_ends_at", "") // no longer relevant
			if err := app.Save(record); err != nil {
				app.Logger().Error("Failed to mark claw as paid", "claw_id", clawID, "error", err)
			} else {
				app.Logger().Info("Claw marked as paid via Stripe", "claw_id", clawID)
			}

		default:
			// Ignore other event types
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}
}

// verifyStripeSignature checks the Stripe-Signature header using HMAC-SHA256.
// Stripe signature format: t=<timestamp>,v1=<signature>[,v0=<test_sig>]
func verifyStripeSignature(payload []byte, sigHeader, secret string) bool {
	if sigHeader == "" {
		return false
	}

	var timestamp string
	var signatures []string

	for _, part := range strings.Split(sigHeader, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			timestamp = kv[1]
		case "v1":
			signatures = append(signatures, kv[1])
		}
	}

	if timestamp == "" || len(signatures) == 0 {
		return false
	}

	// Check timestamp is within 5 minutes
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if time.Since(time.Unix(ts, 0)).Abs() > 5*time.Minute {
		return false
	}

	// Compute expected signature
	signedPayload := timestamp + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signedPayload))
	expected := hex.EncodeToString(mac.Sum(nil))

	for _, sig := range signatures {
		if hmac.Equal([]byte(expected), []byte(sig)) {
			return true
		}
	}
	return false
}
