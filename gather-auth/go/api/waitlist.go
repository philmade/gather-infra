package api

import (
	"context"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// -----------------------------------------------------------------------------
// Request / Response types
// -----------------------------------------------------------------------------

type WaitlistInput struct {
	Body struct {
		Email   string `json:"email" doc:"Email address" minLength:"5" maxLength:"200"`
		Product string `json:"product,omitempty" doc:"Product interest (e.g. 'openclaw')" maxLength:"100"`
	}
}

type WaitlistOutput struct {
	Body struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
}

// -----------------------------------------------------------------------------
// Route registration
// -----------------------------------------------------------------------------

func RegisterWaitlistRoutes(api huma.API, app *pocketbase.PocketBase) {
	huma.Register(api, huma.Operation{
		OperationID: "join-waitlist",
		Method:      "POST",
		Path:        "/api/waitlist",
		Summary:     "Join the waitlist",
		Description: "Register interest in upcoming products. No authentication required.",
		Tags:        []string{"Waitlist"},
	}, func(ctx context.Context, input *WaitlistInput) (*WaitlistOutput, error) {
		email := strings.TrimSpace(input.Body.Email)
		if email == "" || !strings.Contains(email, "@") {
			return nil, huma.Error422UnprocessableEntity("Valid email address required")
		}

		product := input.Body.Product
		if product == "" {
			product = "openclaw"
		}

		// Check for duplicate
		existing, _ := app.FindRecordsByFilter("waitlist",
			"email = {:email} && product = {:product}", "", 1, 0,
			map[string]any{"email": email, "product": product})
		if len(existing) > 0 {
			out := &WaitlistOutput{}
			out.Body.Status = "already_registered"
			out.Body.Message = "You're already on the waitlist. We'll be in touch."
			return out, nil
		}

		col, err := app.FindCollectionByNameOrId("waitlist")
		if err != nil {
			return nil, huma.Error500InternalServerError("waitlist collection not found")
		}

		record := core.NewRecord(col)
		record.Set("email", email)
		record.Set("product", product)
		if err := app.Save(record); err != nil {
			return nil, huma.Error500InternalServerError("Failed to save")
		}

		out := &WaitlistOutput{}
		out.Body.Status = "registered"
		out.Body.Message = "You're on the list. We'll email you when it's ready."
		return out, nil
	})
}
