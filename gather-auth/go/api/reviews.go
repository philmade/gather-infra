package api

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

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
		ChallengeID     string                   `json:"challenge_id,omitempty" doc:"Challenge ID from POST /api/reviews/challenge"`
		Totem           string                   `json:"totem,omitempty" doc:"Totem from the review challenge"`
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
		Message          string  `json:"message"`
		ReviewID         string  `json:"review_id"`
		SkillID          string  `json:"skill_id"`
		Score            float64 `json:"score"`
		ProofID          string  `json:"proof_id"`
		ArtifactCount    int     `json:"artifact_count"`
		VerifiedReviewer bool    `json:"verified_reviewer"`
		Challenged       bool    `json:"challenged"`
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
		ID               string                  `json:"id"`
		Skill            string                  `json:"skill"`
		SkillName        string                  `json:"skill_name,omitempty"`
		AgentID          string                  `json:"agent_id,omitempty"`
		Task             string                  `json:"task"`
		Status           string                  `json:"status"`
		Score            *float64                `json:"score"`
		WhatWorked       string                  `json:"what_worked,omitempty"`
		WhatFailed       string                  `json:"what_failed,omitempty"`
		SkillFeedback    string                  `json:"skill_feedback,omitempty"`
		SecurityScore    *float64                `json:"security_score"`
		SecurityNotes    string                  `json:"security_notes,omitempty"`
		RunnerType       string                  `json:"runner_type,omitempty"`
		PermissionMode   string                  `json:"permission_mode,omitempty"`
		AgentModel       string                  `json:"agent_model,omitempty"`
		ExecutionTimeMs  *float64                `json:"execution_time_ms"`
		CLIOutput        string                  `json:"cli_output,omitempty"`
		VerifiedReviewer bool                    `json:"verified_reviewer"`
		Challenged       bool                    `json:"challenged"`
		Created          string                  `json:"created"`
		Artifacts        []ReviewArtifactSummary `json:"artifacts,omitempty"`
		Proof            *ReviewProofSummary     `json:"proof,omitempty"`
	}
}

type ListReviewsInput struct {
	Limit  int    `query:"limit" default:"20" minimum:"1" maximum:"100" doc:"Max results"`
	Status string `query:"status" doc:"Filter by status (pending, running, complete, failed)"`
}

type ReviewListItem struct {
	ID               string   `json:"id"`
	Skill            string   `json:"skill"`
	SkillName        string   `json:"skill_name,omitempty"`
	Task             string   `json:"task"`
	Status           string   `json:"status"`
	Score            *float64 `json:"score"`
	VerifiedReviewer bool     `json:"verified_reviewer"`
	Challenged       bool     `json:"challenged"`
	Created          string   `json:"created"`
}

type ListReviewsOutput struct {
	Body struct {
		Reviews []ReviewListItem `json:"reviews"`
	}
}

// Review challenge types

type ChallengeSkillInfo struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Category    string  `json:"category,omitempty"`
	URL         string  `json:"url,omitempty"`
	ReviewCount float64 `json:"review_count"`
}

type RequestChallengeInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
	Body          struct {
		SkillID string `json:"skill_id" doc:"Skill name or ID to review" minLength:"1"`
	}
}

type RequestChallengeOutput struct {
	Status int `header:"Status"`
	Body   struct {
		ChallengeID string             `json:"challenge_id"`
		Totem       string             `json:"totem"`
		Task        string             `json:"task"`
		Aspects     []string           `json:"aspects"`
		ExpiresAt   string             `json:"expires_at"`
		ExpiresIn   string             `json:"expires_in"`
		Skill       ChallengeSkillInfo `json:"skill"`
	}
}

// -----------------------------------------------------------------------------
// Route registration
// -----------------------------------------------------------------------------

func RegisterReviewRoutes(api huma.API, app *pocketbase.PocketBase, jwtKey []byte) {
	// Create review (server-side execution — disabled)
	huma.Register(api, huma.Operation{
		OperationID:   "create-review",
		Method:        "POST",
		Path:          "/api/reviews",
		Summary:       "Create a review (async) — currently disabled",
		Description:   "Server-side review execution is not yet available. Use POST /api/reviews/submit to submit your own review.",
		Tags:          []string{"Reviews"},
		DefaultStatus: 501,
	}, func(ctx context.Context, input *CreateReviewInput) (*CreateReviewOutput, error) {
		return nil, huma.Error501NotImplemented(
			"Server-side review execution is not available yet. " +
				"Use POST /api/reviews/submit to submit your own review with an optional Ed25519 proof.")
	})

	// Submit completed review from CLI
	huma.Register(api, huma.Operation{
		OperationID:   "submit-review",
		Method:        "POST",
		Path:          "/api/reviews/submit",
		Summary:       "Submit a completed review",
		Description:   "Submit a review with optional Ed25519 cryptographic proof. Requires JWT authentication.",
		Tags:          []string{"Reviews"},
		DefaultStatus: 201,
	}, func(ctx context.Context, input *SubmitReviewInput) (*SubmitReviewOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		// Look up skill by name or by ID (matching challenge handler logic)
		skill, _ := app.FindFirstRecordByData("skills", "name", input.Body.SkillID)
		if skill == nil {
			skill, _ = app.FindRecordById("skills", input.Body.SkillID)
		}
		if skill == nil {
			// Auto-create only if not found by name or ID
			ensureSkillExists(app, input.Body.SkillID)
			skill, _ = app.FindFirstRecordByData("skills", "name", input.Body.SkillID)
		}
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

		// Look up agent to get registered public key and verification status
		agent, _ := app.FindRecordById("agents", claims.AgentID)
		agentPubKey := ""
		if agent != nil {
			agentPubKey = agent.GetString("public_key")
		}
		isVerified := agent != nil && agent.GetBool("verified")

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
		record.Set("verified_reviewer", isVerified)

		// Validate review challenge if provided
		challenged := false
		if input.Body.ChallengeID != "" && input.Body.Totem != "" {
			challenge, err := app.FindRecordById("review_challenges", input.Body.ChallengeID)
			if err != nil {
				return nil, huma.Error400BadRequest("Challenge not found")
			}
			if challenge.GetString("totem") != input.Body.Totem {
				return nil, huma.Error400BadRequest("Totem does not match challenge")
			}
			if challenge.GetString("agent_id") != claims.AgentID {
				return nil, huma.Error400BadRequest("Challenge was issued to a different agent")
			}
			if challenge.GetBool("used") {
				return nil, huma.Error400BadRequest("Challenge has already been used")
			}
			expiresStr := challenge.GetString("expires")
			if expiresStr != "" {
				if expires, err := time.Parse(time.RFC3339, expiresStr); err == nil {
					if time.Now().After(expires) {
						return nil, huma.Error400BadRequest("Challenge has expired")
					}
				}
			}
			// Mark challenge as used
			challenge.Set("used", true)
			app.Save(challenge)
			record.Set("challenge", challenge.Id)
			challenged = true

			// Security score is mandatory for challenge-verified reviews
			if input.Body.SecurityScore == nil {
				return nil, huma.Error400BadRequest(
					"security_score is required for challenge-verified reviews")
			}
		}

		if err := app.Save(record); err != nil {
			return nil, huma.Error500InternalServerError("Failed to create review")
		}

		// Handle proof — verify against agent's registered key
		proofID := ""
		if p := input.Body.Proof; p != nil && p.Signature != "" && p.ExecutionHash != "" {
			proofVerified := false
			if agentPubKey != "" && p.PublicKey == agentPubKey {
				proofVerified = skills.VerifyAttestation(p.ExecutionHash, p.Signature, p.PublicKey)
			}
			proofID = createClientProof(app, record.Id, p, proofVerified)
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
		out.Body.VerifiedReviewer = isVerified
		out.Body.Challenged = challenged
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
		out.Body.VerifiedReviewer = review.GetBool("verified_reviewer")
		out.Body.Challenged = review.GetString("challenge") != ""
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
				ID:               r.Id,
				Skill:            r.GetString("skill"),
				Task:             r.GetString("task"),
				Status:           r.GetString("status"),
				VerifiedReviewer: r.GetBool("verified_reviewer"),
				Challenged:       r.GetString("challenge") != "",
				Created:          fmt.Sprintf("%v", r.GetDateTime("created")),
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

	// Request a review challenge
	huma.Register(api, huma.Operation{
		OperationID:   "request-review-challenge",
		Method:        "POST",
		Path:          "/api/reviews/challenge",
		Summary:       "Request a review challenge",
		Description:   "Get a unique totem and targeted review task for a skill. The challenge must be completed within 15 minutes. Challenge-verified reviews carry more weight in the marketplace.",
		Tags:          []string{"Reviews"},
		DefaultStatus: 201,
	}, func(ctx context.Context, input *RequestChallengeInput) (*RequestChallengeOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		// Look up skill by name or by ID
		var skill *core.Record
		skill, _ = app.FindFirstRecordByData("skills", "name", input.Body.SkillID)
		if skill == nil {
			skill, _ = app.FindRecordById("skills", input.Body.SkillID)
		}
		if skill == nil {
			return nil, huma.Error404NotFound("Skill not found")
		}

		totem := generateTotem()
		existingReviews, _ := app.FindRecordsByFilter("reviews",
			"skill = {:sid} && status = 'complete'", "", 0, 0,
			map[string]any{"sid": skill.Id})
		task, aspects := generateReviewTask(app, skill, existingReviews)
		expiresAt := time.Now().Add(15 * time.Minute).UTC().Format(time.RFC3339)

		// Persist challenge
		collection, err := app.FindCollectionByNameOrId("review_challenges")
		if err != nil {
			return nil, huma.Error500InternalServerError("review_challenges collection not found")
		}

		aspectsJSON, _ := json.Marshal(aspects)

		record := core.NewRecord(collection)
		record.Set("agent_id", claims.AgentID)
		record.Set("skill", skill.Id)
		record.Set("skill_name", skill.GetString("name"))
		record.Set("totem", totem)
		record.Set("task", task)
		record.Set("aspects", string(aspectsJSON))
		record.Set("expires", expiresAt)
		record.Set("used", false)

		if err := app.Save(record); err != nil {
			return nil, huma.Error500InternalServerError("Failed to create challenge")
		}

		out := &RequestChallengeOutput{}
		out.Status = 201
		out.Body.ChallengeID = record.Id
		out.Body.Totem = totem
		out.Body.Task = task
		out.Body.Aspects = aspects
		out.Body.ExpiresAt = expiresAt
		out.Body.ExpiresIn = "15 minutes"
		out.Body.Skill = ChallengeSkillInfo{
			ID:          skill.Id,
			Name:        skill.GetString("name"),
			Description: skill.GetString("description"),
			Category:    skill.GetString("category"),
			URL:         skill.GetString("url"),
			ReviewCount: skill.GetFloat("review_count"),
		}
		return out, nil
	})
}

// -----------------------------------------------------------------------------
// Totem + review task generation
// -----------------------------------------------------------------------------

func generateTotem() string {
	b := make([]byte, 4)
	crand.Read(b)
	return "GATHER-" + hex.EncodeToString(b)
}

type reviewAspect struct {
	Name   string
	Prompt string
}

var generalAspects = []reviewAspect{
	{"functionality", "Test the core functionality — does the skill do what it claims?"},
	{"error handling", "Try edge cases and invalid inputs — how does it handle errors?"},
	{"output quality", "Evaluate the quality and usefulness of the output."},
	{"ease of use", "Assess how easy the skill is to set up and use as an agent."},
	{"documentation", "Check if the skill has clear documentation and examples."},
}

var apiAspects = []reviewAspect{
	{"response format", "Examine the response format — is it well-structured and parseable?"},
	{"error responses", "Test error responses — are error codes and messages helpful?"},
	{"latency", "Measure response time — is performance acceptable for agent workflows?"},
	{"authentication", "Evaluate the authentication flow — is it straightforward for agents?"},
}

var securityAspects = []reviewAspect{
	{"security", "Evaluate security: Does it require installing code? What permissions does it need? Does it phone home? Is source code auditable?"},
	{"installation safety", "Assess installation safety: What does setup require? Are dependencies auditable? Does it modify system files?"},
	{"data handling", "Review data handling: Does it send data to external servers? What info does it collect? Is data encrypted in transit?"},
}

// generateReviewTask builds a targeted review task for a skill.
// With GEMINI_API_KEY set, it uses Gemini to generate contextual tasks.
// Without it, falls back to template-based generation with mandatory security aspect.
func generateReviewTask(app *pocketbase.PocketBase, skill *core.Record, existingReviews []*core.Record) (task string, aspects []string) {
	// Try AI-driven generation first
	if t, a, err := generateReviewTaskAI(skill, existingReviews); err == nil {
		return t, a
	} else if err.Error() != "GOOGLE_API_KEY not set" {
		log.Printf("WARNING: AI task generation failed, using template fallback: %v", err)
	}

	return generateReviewTaskTemplate(skill)
}

// generateReviewTaskAI uses Claude to create a contextual review task.
func generateReviewTaskAI(skill *core.Record, existingReviews []*core.Record) (string, []string, error) {
	name := skill.GetString("name")
	desc := skill.GetString("description")
	category := skill.GetString("category")
	url := skill.GetString("url")
	source := skill.GetString("source")
	installRequired := skill.GetBool("install_required")

	// Build existing coverage summary
	var coveredAspects []string
	var scoreSum float64
	var securityCount int
	for _, r := range existingReviews {
		if v := r.GetFloat("score"); v > 0 {
			scoreSum += v
		}
		if r.GetFloat("security_score") > 0 {
			securityCount++
		}
	}
	reviewCount := len(existingReviews)

	var coverageSummary string
	if reviewCount == 0 {
		coverageSummary = "No existing reviews. This will be the first."
	} else {
		avgScore := scoreSum / float64(reviewCount)
		coverageSummary = fmt.Sprintf("%d existing reviews (avg score: %.1f). ", reviewCount, avgScore)
		if securityCount == 0 {
			coverageSummary += "No security reviews yet — security evaluation is especially needed."
		} else {
			coverageSummary += fmt.Sprintf("%d include security scores.", securityCount)
		}
	}

	systemPrompt := `You generate targeted review tasks for an AI skills marketplace.
Return JSON only: {"task": "...", "aspects": ["...", "..."]}
Rules:
- Always include exactly one security-focused aspect (e.g. "security", "installation safety", "data handling")
- Choose one additional aspect that complements existing coverage — don't repeat well-covered areas
- Make the task specific, actionable, and completable in 15 minutes
- If the skill requires installation, emphasize installation safety, dependency auditing, filesystem access
- If the skill is an API/service, emphasize data handling, auth flow, network behavior
- The task description should reference the specific skill and what to test
- Return ONLY valid JSON, no markdown, no explanation`

	var userPromptParts []string
	userPromptParts = append(userPromptParts, fmt.Sprintf("Skill: %s", name))
	if desc != "" {
		userPromptParts = append(userPromptParts, fmt.Sprintf("Description: %s", desc))
	}
	if category != "" {
		userPromptParts = append(userPromptParts, fmt.Sprintf("Category: %s", category))
	}
	if url != "" {
		userPromptParts = append(userPromptParts, fmt.Sprintf("URL: %s", url))
	}
	if source != "" {
		userPromptParts = append(userPromptParts, fmt.Sprintf("Source: %s", source))
	}
	userPromptParts = append(userPromptParts, fmt.Sprintf("Install required: %v", installRequired))
	userPromptParts = append(userPromptParts, fmt.Sprintf("Coverage: %s", coverageSummary))
	if len(coveredAspects) > 0 {
		userPromptParts = append(userPromptParts, fmt.Sprintf("Already covered: %s", strings.Join(coveredAspects, ", ")))
	}

	raw, err := callLLM(systemPrompt, strings.Join(userPromptParts, "\n"))
	if err != nil {
		return "", nil, err
	}

	cleaned := stripCodeFences(raw)
	var result struct {
		Task    string   `json:"task"`
		Aspects []string `json:"aspects"`
	}
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return "", nil, fmt.Errorf("parse AI response: %w", err)
	}
	if result.Task == "" || len(result.Aspects) == 0 {
		return "", nil, fmt.Errorf("AI returned empty task or aspects")
	}

	return result.Task, result.Aspects, nil
}

// generateReviewTaskTemplate is the fallback when AI generation is unavailable.
// Always picks 1 security aspect + 1 general/api aspect.
func generateReviewTaskTemplate(skill *core.Record) (task string, aspects []string) {
	category := skill.GetString("category")

	// Pick 1 security aspect
	secChosen := pickRandomAspects(securityAspects, 1)

	// Pick 1 general/api aspect
	generalPool := make([]reviewAspect, len(generalAspects))
	copy(generalPool, generalAspects)
	if category == "api" || category == "service" {
		generalPool = append(generalPool, apiAspects...)
	}
	genChosen := pickRandomAspects(generalPool, 1)

	chosen := append(secChosen, genChosen...)
	for _, a := range chosen {
		aspects = append(aspects, a.Name)
	}

	// Build the task
	name := skill.GetString("name")
	desc := skill.GetString("description")
	url := skill.GetString("url")

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Review the skill '%s'", name))
	if desc != "" {
		b.WriteString(fmt.Sprintf(" (%s)", desc))
	}
	if url != "" {
		b.WriteString(fmt.Sprintf(" at %s", url))
	}
	b.WriteString(". Focus on these aspects:\n")
	for i, a := range chosen {
		b.WriteString(fmt.Sprintf("%d. %s: %s\n", i+1, a.Name, a.Prompt))
	}
	b.WriteString("Include your findings in what_worked and what_failed.")

	return b.String(), aspects
}

func pickRandomAspects(pool []reviewAspect, n int) []reviewAspect {
	if n >= len(pool) {
		return pool
	}
	// Fisher-Yates partial shuffle using math/rand (aspects are not security-sensitive)
	picked := make([]reviewAspect, len(pool))
	copy(picked, pool)
	for i := 0; i < n; i++ {
		j := i + rand.Intn(len(picked)-i)
		picked[i], picked[j] = picked[j], picked[i]
	}
	return picked[:n]
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

	if strings.HasPrefix(skillName, "http://") || strings.HasPrefix(skillName, "https://") {
		record.Set("source", "url")
		record.Set("url", skillName)
		record.Set("category", "api")
	} else {
		record.Set("source", "github")
	}

	app.Save(record)
}

func createClientProof(app *pocketbase.PocketBase, reviewID string, p *ClientProof, verified bool) string {
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
	record.Set("verified", verified)

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
