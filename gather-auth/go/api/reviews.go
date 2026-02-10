package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"gather.is/auth/skills"
)

// -----------------------------------------------------------------------------
// Request / Response types
// -----------------------------------------------------------------------------

type CreateReviewInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
	Body          struct {
		SkillID string `json:"skill_id" doc:"Skill to review" minLength:"1"`
		Task    string `json:"task" doc:"Task description to execute" minLength:"1"`
	}
}

type CreateReviewOutput struct {
	Status int `header:"Status"`
	Body   struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Message string `json:"message"`
	}
}

type SubmitReviewInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
	RawBody       []byte
	Body          struct {
		SkillID         string                   `json:"skill_id" doc:"Skill that was reviewed" minLength:"1"`
		Task            string                   `json:"task" doc:"Task that was executed" minLength:"1"`
		Score           float64                  `json:"score" doc:"Quality score 1-10" minimum:"1" maximum:"10"`
		WhatWorked      string                   `json:"what_worked,omitempty" doc:"What worked well"`
		WhatFailed      string                   `json:"what_failed,omitempty" doc:"What failed or had issues"`
		SkillFeedback   string                   `json:"skill_feedback,omitempty" doc:"Feedback for the skill author"`
		SecurityScore   *float64                 `json:"security_score,omitempty" doc:"Security score 1-10"`
		SecurityNotes   string                   `json:"security_notes,omitempty" doc:"Security review findings"`
		RunnerType      string                   `json:"runner_type,omitempty" doc:"Executor type (claude, aider, etc.)"`
		PermissionMode  string                   `json:"permission_mode,omitempty" doc:"Permission mode used"`
		ExecutionTimeMs *float64                 `json:"execution_time_ms,omitempty" doc:"Execution time in milliseconds"`
		CLIOutput       string                   `json:"cli_output,omitempty" doc:"Raw CLI output"`
		Proof           *ClientProof             `json:"proof,omitempty" doc:"Client-side execution proof"`
		Artifacts       []ClientArtifact         `json:"artifacts,omitempty" doc:"File artifacts from execution"`
	}
}

type ClientProof struct {
	ID            string                 `json:"id"`
	Signature     string                 `json:"signature"`
	ExecutionHash string                 `json:"execution_hash"`
	PublicKey     string                 `json:"public_key"`
	Payload       map[string]interface{} `json:"payload,omitempty"`
}

type ClientArtifact struct {
	FileName      string `json:"file_name"`
	ContentBase64 string `json:"content_base64"`
	MimeType      string `json:"mime_type,omitempty"`
}

type SubmitReviewOutput struct {
	Status int `header:"Status"`
	Body   struct {
		Message       string `json:"message"`
		ReviewID      string `json:"review_id"`
		SkillID       string `json:"skill_id"`
		Score         float64 `json:"score"`
		ProofID       string `json:"proof_id"`
		ArtifactCount int    `json:"artifact_count"`
	}
}

type GetReviewInput struct {
	ID string `path:"id" doc:"Review ID"`
}

type ReviewArtifactSummary struct {
	ID       string `json:"id"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type,omitempty"`
}

type ReviewProofSummary struct {
	ID       string `json:"id"`
	Verified bool   `json:"verified"`
	Created  string `json:"created"`
}

type GetReviewOutput struct {
	Body struct {
		ID              string                 `json:"id"`
		Skill           string                 `json:"skill"`
		SkillName       string                 `json:"skill_name,omitempty"`
		AgentID         string                 `json:"agent_id,omitempty"`
		Task            string                 `json:"task"`
		Status          string                 `json:"status"`
		Score           *float64               `json:"score"`
		WhatWorked      string                 `json:"what_worked,omitempty"`
		WhatFailed      string                 `json:"what_failed,omitempty"`
		SkillFeedback   string                 `json:"skill_feedback,omitempty"`
		SecurityScore   *float64               `json:"security_score"`
		SecurityNotes   string                 `json:"security_notes,omitempty"`
		RunnerType      string                 `json:"runner_type,omitempty"`
		PermissionMode  string                 `json:"permission_mode,omitempty"`
		AgentModel      string                 `json:"agent_model,omitempty"`
		ExecutionTimeMs *float64               `json:"execution_time_ms"`
		CLIOutput       string                 `json:"cli_output,omitempty"`
		Created         string                 `json:"created"`
		Artifacts       []ReviewArtifactSummary `json:"artifacts,omitempty"`
		Proof           *ReviewProofSummary     `json:"proof,omitempty"`
	}
}

type ListReviewsInput struct {
	Limit  int    `query:"limit" default:"20" minimum:"1" maximum:"100" doc:"Max results"`
	Status string `query:"status" doc:"Filter by status (pending, running, complete, failed)"`
}

type ReviewListItem struct {
	ID        string   `json:"id"`
	Skill     string   `json:"skill"`
	SkillName string   `json:"skill_name,omitempty"`
	Task      string   `json:"task"`
	Status    string   `json:"status"`
	Score     *float64 `json:"score"`
	Created   string   `json:"created"`
}

type ListReviewsOutput struct {
	Body struct {
		Reviews []ReviewListItem `json:"reviews"`
	}
}

// -----------------------------------------------------------------------------
// Route registration
// -----------------------------------------------------------------------------

func RegisterReviewRoutes(api huma.API, app *pocketbase.PocketBase, jwtKey []byte) {
	// Create review (starts async execution)
	huma.Register(api, huma.Operation{
		OperationID:   "create-review",
		Method:        "POST",
		Path:          "/api/reviews",
		Summary:       "Create a review (async)",
		Description:   "Creates a new review and starts skill execution in the background. Returns immediately with review ID.",
		Tags:          []string{"Reviews"},
		DefaultStatus: 202,
	}, func(ctx context.Context, input *CreateReviewInput) (*CreateReviewOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		// Ensure skill exists, auto-create if not
		ensureSkillExists(app, input.Body.SkillID)

		// Find the skill
		skill, _ := app.FindFirstRecordByData("skills", "name", input.Body.SkillID)
		skillRef := ""
		if skill != nil {
			skillRef = skill.Id
		}

		agentID := claims.AgentID

		collection, err := app.FindCollectionByNameOrId("reviews")
		if err != nil {
			return nil, huma.Error500InternalServerError("reviews collection not found")
		}

		record := core.NewRecord(collection)
		record.Set("skill", skillRef)
		record.Set("agent_id", agentID)
		record.Set("task", input.Body.Task)
		record.Set("status", "pending")

		if err := app.Save(record); err != nil {
			return nil, huma.Error500InternalServerError("Failed to create review")
		}

		// Start execution in background
		go skills.ExecuteReview(app, record.Id, input.Body.SkillID, input.Body.Task)

		out := &CreateReviewOutput{}
		out.Status = 202
		out.Body.ID = record.Id
		out.Body.Status = "pending"
		out.Body.Message = "Review started"
		return out, nil
	})

	// Submit completed review from CLI
	huma.Register(api, huma.Operation{
		OperationID:   "submit-review",
		Method:        "POST",
		Path:          "/api/reviews/submit",
		Summary:       "Submit a completed review",
		Description:   "Submit a review that was executed locally by the CLI. Requires JWT authentication.",
		Tags:          []string{"Reviews"},
		DefaultStatus: 201,
	}, func(ctx context.Context, input *SubmitReviewInput) (*SubmitReviewOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		// Ensure skill exists
		ensureSkillExists(app, input.Body.SkillID)

		skill, _ := app.FindFirstRecordByData("skills", "name", input.Body.SkillID)
		skillRef := ""
		if skill != nil {
			skillRef = skill.Id
		}

		runnerType := input.Body.RunnerType
		if runnerType == "" {
			runnerType = "claude"
		}
		permissionMode := input.Body.PermissionMode
		if permissionMode == "" {
			permissionMode = "default"
		}

		collection, err := app.FindCollectionByNameOrId("reviews")
		if err != nil {
			return nil, huma.Error500InternalServerError("reviews collection not found")
		}

		record := core.NewRecord(collection)
		record.Set("skill", skillRef)
		record.Set("agent_id", claims.AgentID)
		record.Set("task", input.Body.Task)
		record.Set("status", "complete")
		record.Set("score", input.Body.Score)
		record.Set("what_worked", input.Body.WhatWorked)
		record.Set("what_failed", input.Body.WhatFailed)
		record.Set("skill_feedback", input.Body.SkillFeedback)
		if input.Body.SecurityScore != nil {
			record.Set("security_score", *input.Body.SecurityScore)
		}
		record.Set("security_notes", input.Body.SecurityNotes)
		record.Set("runner_type", runnerType)
		record.Set("permission_mode", permissionMode)
		record.Set("agent_model", "claude-sonnet")
		if input.Body.ExecutionTimeMs != nil {
			record.Set("execution_time_ms", *input.Body.ExecutionTimeMs)
		}
		record.Set("cli_output", input.Body.CLIOutput)

		if err := app.Save(record); err != nil {
			return nil, huma.Error500InternalServerError("Failed to create review")
		}

		// Handle proof
		proofID := ""
		if p := input.Body.Proof; p != nil && p.ID != "" && p.Signature != "" {
			proofID = createClientProof(app, record.Id, p)
		}
		if proofID == "" {
			// Generate server-side attestation
			proofID = createServerProof(app, record.Id, input.Body.SkillID, input.Body.Task, input.Body.CLIOutput, input.Body.Score, input.Body.WhatWorked, input.Body.WhatFailed, input.Body.ExecutionTimeMs)
		}

		if proofID != "" {
			record.Set("proof", proofID)
			app.Save(record)
		}

		// Update skill stats
		if skill != nil {
			updateSkillStatsFromAPI(app, skill.Id)
		}

		out := &SubmitReviewOutput{}
		out.Status = 201
		out.Body.Message = "Review submitted successfully"
		out.Body.ReviewID = record.Id
		out.Body.SkillID = input.Body.SkillID
		out.Body.Score = input.Body.Score
		out.Body.ProofID = proofID
		return out, nil
	})

	// Get review by ID
	huma.Register(api, huma.Operation{
		OperationID: "get-review",
		Method:      "GET",
		Path:        "/api/reviews/{id}",
		Summary:     "Get review details",
		Description: "Returns review details with artifacts and proof info.",
		Tags:        []string{"Reviews"},
	}, func(ctx context.Context, input *GetReviewInput) (*GetReviewOutput, error) {
		review, err := app.FindRecordById("reviews", input.ID)
		if err != nil {
			return nil, huma.Error404NotFound("Review not found")
		}

		out := &GetReviewOutput{}
		out.Body.ID = review.Id
		out.Body.Skill = review.GetString("skill")
		out.Body.AgentID = review.GetString("agent_id")
		out.Body.Task = review.GetString("task")
		out.Body.Status = review.GetString("status")
		out.Body.WhatWorked = review.GetString("what_worked")
		out.Body.WhatFailed = review.GetString("what_failed")
		out.Body.SkillFeedback = review.GetString("skill_feedback")
		out.Body.SecurityNotes = review.GetString("security_notes")
		out.Body.RunnerType = review.GetString("runner_type")
		out.Body.PermissionMode = review.GetString("permission_mode")
		out.Body.AgentModel = review.GetString("agent_model")
		out.Body.CLIOutput = review.GetString("cli_output")
		out.Body.Created = fmt.Sprintf("%v", review.GetDateTime("created"))

		if v := review.GetFloat("score"); v > 0 {
			out.Body.Score = &v
		}
		if v := review.GetFloat("security_score"); v > 0 {
			out.Body.SecurityScore = &v
		}
		if v := review.GetFloat("execution_time_ms"); v > 0 {
			out.Body.ExecutionTimeMs = &v
		}

		// Get skill name
		if skillID := review.GetString("skill"); skillID != "" {
			if skillRec, err := app.FindRecordById("skills", skillID); err == nil {
				out.Body.SkillName = skillRec.GetString("name")
			}
		}

		// Get artifacts
		artifacts, _ := app.FindRecordsByFilter("artifacts",
			"review = {:rid}", "", 0, 0,
			map[string]any{"rid": review.Id})
		for _, a := range artifacts {
			out.Body.Artifacts = append(out.Body.Artifacts, ReviewArtifactSummary{
				ID:       a.Id,
				FileName: a.GetString("file_name"),
				MimeType: a.GetString("mime_type"),
			})
		}

		// Get proof
		if proofID := review.GetString("proof"); proofID != "" {
			if proof, err := app.FindRecordById("proofs", proofID); err == nil {
				out.Body.Proof = &ReviewProofSummary{
					ID:       proof.Id,
					Verified: proof.GetBool("verified"),
					Created:  fmt.Sprintf("%v", proof.GetDateTime("created")),
				}
			}
		}

		return out, nil
	})

	// List reviews
	huma.Register(api, huma.Operation{
		OperationID: "list-reviews",
		Method:      "GET",
		Path:        "/api/reviews",
		Summary:     "List recent reviews",
		Description: "Returns recent reviews, optionally filtered by status.",
		Tags:        []string{"Reviews"},
	}, func(ctx context.Context, input *ListReviewsInput) (*ListReviewsOutput, error) {
		filter := "id != ''"
		params := map[string]any{}

		if input.Status != "" {
			filter += " && status = {:status}"
			params["status"] = input.Status
		}

		records, err := app.FindRecordsByFilter("reviews", filter, "", input.Limit, 0, params)
		if err != nil {
			records = nil
		}

		items := make([]ReviewListItem, 0, len(records))
		for _, r := range records {
			item := ReviewListItem{
				ID:      r.Id,
				Skill:   r.GetString("skill"),
				Task:    r.GetString("task"),
				Status:  r.GetString("status"),
				Created: fmt.Sprintf("%v", r.GetDateTime("created")),
			}
			if v := r.GetFloat("score"); v > 0 {
				item.Score = &v
			}
			// Get skill name
			if skillID := r.GetString("skill"); skillID != "" {
				if skillRec, err := app.FindRecordById("skills", skillID); err == nil {
					item.SkillName = skillRec.GetString("name")
				}
			}
			items = append(items, item)
		}

		out := &ListReviewsOutput{}
		out.Body.Reviews = items
		return out, nil
	})
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func ensureSkillExists(app *pocketbase.PocketBase, skillName string) {
	existing, _ := app.FindFirstRecordByData("skills", "name", skillName)
	if existing != nil {
		return
	}

	collection, err := app.FindCollectionByNameOrId("skills")
	if err != nil {
		return
	}

	record := core.NewRecord(collection)
	record.Set("name", skillName)
	record.Set("source", "github")
	app.Save(record)
}

func createClientProof(app *pocketbase.PocketBase, reviewID string, p *ClientProof) string {
	collection, err := app.FindCollectionByNameOrId("proofs")
	if err != nil {
		return ""
	}

	payloadJSON, _ := json.Marshal(p.Payload)
	sigJSON, _ := json.Marshal([]string{p.Signature})
	witJSON, _ := json.Marshal([]map[string]string{{"type": "ed25519", "public_key": p.PublicKey}})

	record := core.NewRecord(collection)
	record.Set("review", reviewID)
	record.Set("claim_data", string(payloadJSON))
	record.Set("identifier", p.ExecutionHash)
	record.Set("signatures", string(sigJSON))
	record.Set("witnesses", string(witJSON))
	record.Set("verified", true)

	if err := app.Save(record); err != nil {
		return ""
	}
	return record.Id
}

func createServerProof(app *pocketbase.PocketBase, reviewID, skillID, task, cliOutput string, score float64, whatWorked, whatFailed string, execTimeMs *float64) string {
	attestation, err := skills.CreateAttestation(skills.ExecutionData{
		SkillID:         skillID,
		Task:            task,
		CLIOutput:       cliOutput,
		Score:           &score,
		WhatWorked:      whatWorked,
		WhatFailed:      whatFailed,
		ExecutionTimeMs: execTimeMs,
	})
	if err != nil {
		return ""
	}

	collection, err := app.FindCollectionByNameOrId("proofs")
	if err != nil {
		return ""
	}

	payloadJSON, _ := json.Marshal(attestation.Payload)
	sigJSON, _ := json.Marshal([]string{attestation.Signature})
	witJSON, _ := json.Marshal([]map[string]string{{"type": "ed25519", "public_key": attestation.PublicKey}})

	record := core.NewRecord(collection)
	record.Set("review", reviewID)
	record.Set("claim_data", string(payloadJSON))
	record.Set("identifier", attestation.ExecutionHash)
	record.Set("signatures", string(sigJSON))
	record.Set("witnesses", string(witJSON))
	record.Set("verified", true)

	if err := app.Save(record); err != nil {
		return ""
	}
	return record.Id
}

func updateSkillStatsFromAPI(app *pocketbase.PocketBase, skillID string) {
	skill, err := app.FindRecordById("skills", skillID)
	if err != nil {
		return
	}

	reviews, err := app.FindRecordsByFilter("reviews",
		"skill = {:sid} && status = 'complete' && score > 0", "", 0, 0,
		map[string]any{"sid": skillID})
	if err != nil {
		return
	}

	var totalScore, totalSecScore float64
	var secCount int
	for _, r := range reviews {
		totalScore += r.GetFloat("score")
		if s := r.GetFloat("security_score"); s > 0 {
			totalSecScore += s
			secCount++
		}
	}

	reviewCount := len(reviews)
	var avgScore, avgSecScore float64
	if reviewCount > 0 {
		avgScore = totalScore / float64(reviewCount)
	}
	if secCount > 0 {
		avgSecScore = totalSecScore / float64(secCount)
	}

	skill.Set("review_count", reviewCount)
	skill.Set("avg_score", avgScore)
	skill.Set("avg_security_score", avgSecScore)
	app.Save(skill)

	skills.UpdateSkillRanking(app, skillID)
}
