package skills

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

const defaultTimeoutSec = 300 // 5 minutes

var reviewPrompt = `You are reviewing a Claude Code skill. Your review has two parts: a quality review and a security review.

## Part 1: Install and Test

1. Install the skill using: claude skill add {skill_id}
2. Execute the skill with the given task
3. Evaluate how well it worked

## Part 2: Security Review

After installing, READ every file that was installed as part of the skill. Then assess security:

Start at a security_score of 10 (clean) and subtract based on what you find:

- File writes outside project: -2 to -4
- Credential access: -4 to -8
- Network calls: -1 to -3
- Shell commands that install system-wide software: -2 to -4
- Persistence (cron jobs, PATH modifications): -3 to -5
- Obfuscation: -5 to -8

## Output

Provide your assessment in this exact JSON format:
{
  "score": <number 1-10>,
  "what_worked": "<description>",
  "what_failed": "<issues or 'Nothing'>",
  "skill_feedback": "<feedback for the author>",
  "security_score": <number 1-10>,
  "security_notes": "<findings or 'No security concerns found'>"
}`

// ReviewResult is the parsed JSON from claude's output.
type ReviewResult struct {
	Score         float64  `json:"score"`
	WhatWorked    string   `json:"what_worked"`
	WhatFailed    string   `json:"what_failed"`
	SkillFeedback string   `json:"skill_feedback"`
	SecurityScore *float64 `json:"security_score"`
	SecurityNotes string   `json:"security_notes"`
}

// ExecuteReview runs a skill review asynchronously, updating the review record in PocketBase.
func ExecuteReview(app *pocketbase.PocketBase, reviewID, skillID, task string) {
	review, err := app.FindRecordById("reviews", reviewID)
	if err != nil {
		return
	}

	review.Set("status", "running")
	app.Save(review)

	startTime := time.Now()

	fullPrompt := fmt.Sprintf("%s\n\nSkill to review: %s\nTask: %s\n\nBegin by installing the skill and then executing the task.",
		strings.Replace(reviewPrompt, "{skill_id}", skillID, 1), skillID, task)

	output, timedOut := runClaude(fullPrompt)
	executionTime := time.Since(startTime).Milliseconds()

	if timedOut {
		review.Set("status", "failed")
		review.Set("cli_output", output)
		review.Set("execution_time_ms", executionTime)
		app.Save(review)
		return
	}

	result := parseReviewResult(output)
	if result == nil {
		review.Set("status", "failed")
		review.Set("cli_output", output)
		review.Set("execution_time_ms", executionTime)
		app.Save(review)
		return
	}

	review.Set("status", "complete")
	review.Set("score", result.Score)
	review.Set("what_worked", result.WhatWorked)
	review.Set("what_failed", result.WhatFailed)
	review.Set("skill_feedback", result.SkillFeedback)
	if result.SecurityScore != nil {
		review.Set("security_score", *result.SecurityScore)
	}
	review.Set("security_notes", result.SecurityNotes)
	review.Set("agent_model", "claude-sonnet")
	review.Set("execution_time_ms", executionTime)
	review.Set("cli_output", output)

	if err := app.Save(review); err != nil {
		return
	}

	// Generate attestation/proof
	attestation, err := CreateAttestation(ExecutionData{
		SkillID:    skillID,
		Task:       task,
		CLIOutput:  output,
		Score:      &result.Score,
		WhatWorked: result.WhatWorked,
		WhatFailed: result.WhatFailed,
	})
	if err == nil {
		proofColl, err := app.FindCollectionByNameOrId("proofs")
		if err == nil {
			sigJSON, _ := json.Marshal([]string{attestation.Signature})
			witJSON, _ := json.Marshal([]map[string]string{{"type": "ed25519", "public_key": attestation.PublicKey}})
			payloadJSON, _ := json.Marshal(attestation.Payload)

			proof := core.NewRecord(proofColl)
			proof.Set("review", reviewID)
			proof.Set("claim_data", string(payloadJSON))
			proof.Set("identifier", attestation.ExecutionHash)
			proof.Set("signatures", string(sigJSON))
			proof.Set("witnesses", string(witJSON))
			proof.Set("verified", true)
			if err := app.Save(proof); err == nil {
				review.Set("proof", proof.Id)
				app.Save(review)
			}
		}
	}

	// Update skill stats
	updateSkillStats(app, skillID)
}

func runClaude(prompt string) (string, bool) {
	timeout := defaultTimeoutSec
	if envTimeout := os.Getenv("RESKILL_REVIEW_TIMEOUT_MS"); envTimeout != "" {
		var ms int
		if _, err := fmt.Sscanf(envTimeout, "%d", &ms); err == nil && ms > 0 {
			timeout = ms / 1000
		}
	}

	// Mock mode for testing
	if os.Getenv("RESKILL_MOCK") != "" {
		return mockResponse, false
	}

	cmd := exec.Command("claude", "-p", "-")
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()

	select {
	case err := <-done:
		output := stdout.String() + stderr.String()
		if err != nil {
			return output + "\nError: " + err.Error(), false
		}
		return output, false
	case <-time.After(time.Duration(timeout) * time.Second):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return stdout.String() + fmt.Sprintf("\n\n[TIMEOUT] Review killed after %ds", timeout), true
	}
}

var jsonResultPattern = regexp.MustCompile(`\{[\s\S]*"score"[\s\S]*"what_worked"[\s\S]*"what_failed"[\s\S]*"skill_feedback"[\s\S]*\}`)

func parseReviewResult(output string) *ReviewResult {
	match := jsonResultPattern.FindString(output)
	if match == "" {
		return nil
	}

	var result ReviewResult
	if err := json.Unmarshal([]byte(match), &result); err != nil {
		return nil
	}

	if result.Score < 1 {
		result.Score = 1
	}
	if result.Score > 10 {
		result.Score = 10
	}
	if result.SecurityScore != nil {
		v := *result.SecurityScore
		if v < 1 {
			v = 1
		}
		if v > 10 {
			v = 10
		}
		result.SecurityScore = &v
	}

	if result.WhatWorked == "" || result.WhatFailed == "" || result.SkillFeedback == "" {
		return nil
	}

	return &result
}

func updateSkillStats(app *pocketbase.PocketBase, skillID string) {
	skill, err := app.FindRecordById("skills", skillID)
	if err != nil {
		return
	}

	// Count completed reviews with scores
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

	UpdateSkillRanking(app, skillID)
}

const mockResponse = `
## Mock Review

Installing skill... done.
Executing task... done.

{
  "score": 8,
  "what_worked": "Clean installation, good documentation.",
  "what_failed": "Minor issue with error handling.",
  "skill_feedback": "Consider adding more examples.",
  "security_score": 9,
  "security_notes": "No security concerns found. All operations stay within project scope."
}
`
