package skills

import (
	"math"

	"github.com/pocketbase/pocketbase"
)

// RankingWeights controls the weighted rank formula.
type RankingWeights struct {
	Reviews  float64
	Installs float64
	Proofs   float64
}

var DefaultWeights = RankingWeights{
	Reviews:  0.40,
	Installs: 0.25,
	Proofs:   0.35,
}

// CalculateRankScore computes a 0-100 rank score for a skill.
func CalculateRankScore(avgScore *float64, reviewCount, installs, proofCount, totalReviews int, w RankingWeights) float64 {
	if avgScore == nil || reviewCount == 0 {
		return 0
	}

	// Log-scale normalization to prevent dominance by very popular skills
	normalizedReviewCount := math.Log10(float64(reviewCount)+1) / math.Log10(float64(totalReviews)+10)
	normalizedInstalls := math.Log10(float64(installs)+1) / math.Log10(10000)

	// Proof ratio: what percentage of reviews have verified proofs
	proofRatio := 0.0
	if reviewCount > 0 {
		proofRatio = float64(proofCount) / float64(reviewCount)
	}

	score := (w.Reviews * *avgScore * normalizedReviewCount) +
		(w.Installs * normalizedInstalls) +
		(w.Proofs * proofRatio * *avgScore)

	return math.Min(100, math.Max(0, score*10))
}

// UpdateSkillRanking recalculates the rank_score for a single skill.
func UpdateSkillRanking(app *pocketbase.PocketBase, skillID string) {
	skill, err := app.FindRecordById("skills", skillID)
	if err != nil {
		return
	}

	reviewCount := int(skill.GetFloat("review_count"))
	installs := int(skill.GetFloat("installs"))

	var avgScore *float64
	if v := skill.GetFloat("avg_score"); v > 0 {
		avgScore = &v
	}

	// Count verified proofs for this skill's reviews
	proofCount := 0
	reviews, err := app.FindRecordsByFilter("reviews", "skill = {:sid} && status = 'complete'", "", 0, 0,
		map[string]any{"sid": skillID})
	if err == nil {
		for _, r := range reviews {
			if r.GetString("proof") != "" {
				proofID := r.GetString("proof")
				proof, err := app.FindRecordById("proofs", proofID)
				if err == nil && proof.GetBool("verified") {
					proofCount++
				}
			}
		}
	}

	// Get total reviews across all skills for normalization
	totalReviews := 0
	allSkills, err := app.FindRecordsByFilter("skills", "1=1", "", 0, 0, nil)
	if err == nil {
		for _, s := range allSkills {
			totalReviews += int(s.GetFloat("review_count"))
		}
	}

	rankScore := CalculateRankScore(avgScore, reviewCount, installs, proofCount, totalReviews, DefaultWeights)

	skill.Set("rank_score", rankScore)
	app.Save(skill)
}

// UpdateAllRankings recalculates rank_score for all skills.
func UpdateAllRankings(app *pocketbase.PocketBase) {
	allSkills, err := app.FindRecordsByFilter("skills", "1=1", "", 0, 0, nil)
	if err != nil {
		return
	}
	for _, s := range allSkills {
		UpdateSkillRanking(app, s.Id)
	}
}
