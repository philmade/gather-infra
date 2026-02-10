package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pocketbase/pocketbase"

	"gather.is/auth/skills"
)

// -----------------------------------------------------------------------------
// Request / Response types
// -----------------------------------------------------------------------------

type GetProofInput struct {
	ID string `path:"id" doc:"Proof ID"`
}

type GetProofOutput struct {
	Body struct {
		ID         string      `json:"id"`
		ReviewID   string      `json:"review_id"`
		SkillID    string      `json:"skill_id,omitempty"`
		Task       string      `json:"task,omitempty"`
		ClaimData  interface{} `json:"claim_data"`
		Identifier string      `json:"identifier"`
		Signatures interface{} `json:"signatures"`
		Witnesses  interface{} `json:"witnesses"`
		Verified   bool        `json:"verified"`
		Created    string      `json:"created"`
	}
}

type VerifyProofInput struct {
	ID string `path:"id" doc:"Proof ID to verify"`
}

type VerifyProofOutput struct {
	Body struct {
		ID       string `json:"id"`
		Verified bool   `json:"verified"`
		Message  string `json:"message"`
	}
}

type ListProofsInput struct {
	Limit    int    `query:"limit" default:"20" minimum:"1" maximum:"100" doc:"Max results"`
	Verified string `query:"verified" doc:"Filter by verified status (true/false)"`
}

type ProofListItem struct {
	ID       string `json:"id"`
	ReviewID string `json:"review_id"`
	SkillID  string `json:"skill_id,omitempty"`
	Verified bool   `json:"verified"`
	Created  string `json:"created"`
}

type ListProofsOutput struct {
	Body struct {
		Proofs []ProofListItem `json:"proofs"`
	}
}

// -----------------------------------------------------------------------------
// Route registration
// -----------------------------------------------------------------------------

func RegisterProofRoutes(api huma.API, app *pocketbase.PocketBase) {
	// Get proof details
	huma.Register(api, huma.Operation{
		OperationID: "get-proof",
		Method:      "GET",
		Path:        "/api/proofs/{id}",
		Summary:     "Get proof details",
		Description: "Returns full proof details including claim data, signatures, and witnesses.",
		Tags:        []string{"Proofs"},
	}, func(ctx context.Context, input *GetProofInput) (*GetProofOutput, error) {
		proof, err := app.FindRecordById("proofs", input.ID)
		if err != nil {
			return nil, huma.Error404NotFound("Proof not found")
		}

		out := &GetProofOutput{}
		out.Body.ID = proof.Id
		out.Body.ReviewID = proof.GetString("review")
		out.Body.Identifier = proof.GetString("identifier")
		out.Body.Verified = proof.GetBool("verified")
		out.Body.Created = fmt.Sprintf("%v", proof.GetDateTime("created"))

		// Parse JSON fields
		if raw := proof.GetString("claim_data"); raw != "" {
			var v interface{}
			json.Unmarshal([]byte(raw), &v)
			out.Body.ClaimData = v
		}
		if raw := proof.GetString("signatures"); raw != "" {
			var v interface{}
			json.Unmarshal([]byte(raw), &v)
			out.Body.Signatures = v
		}
		if raw := proof.GetString("witnesses"); raw != "" {
			var v interface{}
			json.Unmarshal([]byte(raw), &v)
			out.Body.Witnesses = v
		}

		// Get skill info from the linked review
		if reviewID := proof.GetString("review"); reviewID != "" {
			if review, err := app.FindRecordById("reviews", reviewID); err == nil {
				if skillID := review.GetString("skill"); skillID != "" {
					if skillRec, err := app.FindRecordById("skills", skillID); err == nil {
						out.Body.SkillID = skillRec.GetString("name")
					}
				}
				out.Body.Task = review.GetString("task")
			}
		}

		return out, nil
	})

	// Verify proof
	huma.Register(api, huma.Operation{
		OperationID: "verify-proof",
		Method:      "POST",
		Path:        "/api/proofs/{id}/verify",
		Summary:     "Verify a proof",
		Description: "Re-verifies the Ed25519 signature on a proof and updates the verified status.",
		Tags:        []string{"Proofs"},
	}, func(ctx context.Context, input *VerifyProofInput) (*VerifyProofOutput, error) {
		proof, err := app.FindRecordById("proofs", input.ID)
		if err != nil {
			return nil, huma.Error404NotFound("Proof not found")
		}

		// Parse witnesses and signatures
		var witnesses []struct {
			Type      string `json:"type"`
			PublicKey string `json:"public_key"`
		}
		var signatures []string

		if raw := proof.GetString("witnesses"); raw != "" {
			json.Unmarshal([]byte(raw), &witnesses)
		}
		if raw := proof.GetString("signatures"); raw != "" {
			json.Unmarshal([]byte(raw), &signatures)
		}

		if len(witnesses) == 0 || len(signatures) == 0 {
			out := &VerifyProofOutput{}
			out.Body.ID = proof.Id
			out.Body.Verified = false
			out.Body.Message = "No signatures found"
			return out, nil
		}

		executionHash := proof.GetString("identifier")
		isValid := skills.VerifyAttestation(executionHash, signatures[0], witnesses[0].PublicKey)

		proof.Set("verified", isValid)
		app.Save(proof)

		out := &VerifyProofOutput{}
		out.Body.ID = proof.Id
		out.Body.Verified = isValid
		if isValid {
			out.Body.Message = "Signature verified successfully"
		} else {
			out.Body.Message = "Signature verification failed"
		}
		return out, nil
	})

	// List proofs
	huma.Register(api, huma.Operation{
		OperationID: "list-proofs",
		Method:      "GET",
		Path:        "/api/proofs",
		Summary:     "List proofs",
		Description: "Returns recent proofs, optionally filtered by verified status.",
		Tags:        []string{"Proofs"},
	}, func(ctx context.Context, input *ListProofsInput) (*ListProofsOutput, error) {
		filter := "id != ''"
		params := map[string]any{}

		if input.Verified == "true" {
			filter += " && verified = true"
		} else if input.Verified == "false" {
			filter += " && verified = false"
		}

		records, err := app.FindRecordsByFilter("proofs", filter, "", input.Limit, 0, params)
		if err != nil {
			records = nil
		}

		items := make([]ProofListItem, 0, len(records))
		for _, r := range records {
			item := ProofListItem{
				ID:       r.Id,
				ReviewID: r.GetString("review"),
				Verified: r.GetBool("verified"),
				Created:  fmt.Sprintf("%v", r.GetDateTime("created")),
			}
			// Get skill ID from review
			if reviewID := r.GetString("review"); reviewID != "" {
				if review, err := app.FindRecordById("reviews", reviewID); err == nil {
					if skillID := review.GetString("skill"); skillID != "" {
						if skillRec, err := app.FindRecordById("skills", skillID); err == nil {
							item.SkillID = skillRec.GetString("name")
						}
					}
				}
			}
			items = append(items, item)
		}

		out := &ListProofsOutput{}
		out.Body.Proofs = items
		return out, nil
	})
}
