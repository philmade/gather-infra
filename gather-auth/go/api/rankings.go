package api

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pocketbase/pocketbase"

	"gather.is/auth/skills"
)

// -----------------------------------------------------------------------------
// Request / Response types
// -----------------------------------------------------------------------------

type ListRankingsInput struct {
	Limit int `query:"limit" default:"20" minimum:"1" maximum:"100" doc:"Max results"`
}

type RankedSkill struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description,omitempty"`
	Installs       float64  `json:"installs"`
	ReviewCount    float64  `json:"review_count"`
	AvgScore       *float64 `json:"avg_score"`
	RankScore      *float64 `json:"rank_score"`
	VerifiedProofs int      `json:"verified_proofs"`
}

type ListRankingsOutput struct {
	Body struct {
		Rankings []RankedSkill `json:"rankings"`
		Count    int           `json:"count"`
	}
}

type RefreshRankingsInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
}

type RefreshRankingsOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

// -----------------------------------------------------------------------------
// Route registration
// -----------------------------------------------------------------------------

func RegisterRankingRoutes(api huma.API, app *pocketbase.PocketBase, jwtKey []byte) {
	huma.Register(api, huma.Operation{
		OperationID: "list-rankings",
		Method:      "GET",
		Path:        "/api/rankings",
		Summary:     "Get ranked skills",
		Description: "Returns skills ranked by a composite score factoring reviews, installs, and verified proofs.",
		Tags:        []string{"Rankings"},
	}, func(ctx context.Context, input *ListRankingsInput) (*ListRankingsOutput, error) {
		records, err := app.FindRecordsByFilter("skills",
			"review_count > 0", "-rank_score,-review_count", input.Limit, 0, nil)
		if err != nil {
			records = nil
		}

		ranked := make([]RankedSkill, 0, len(records))
		for _, r := range records {
			item := RankedSkill{
				ID:          r.Id,
				Name:        r.GetString("name"),
				Description: r.GetString("description"),
				Installs:    r.GetFloat("installs"),
				ReviewCount: r.GetFloat("review_count"),
			}
			if v := r.GetFloat("avg_score"); v > 0 {
				item.AvgScore = &v
			}
			if v := r.GetFloat("rank_score"); v > 0 {
				item.RankScore = &v
			}

			// Count verified proofs for this skill
			reviews, _ := app.FindRecordsByFilter("reviews",
				"skill = {:sid} && status = 'complete'", "", 0, 0,
				map[string]any{"sid": r.Id})
			for _, rev := range reviews {
				if proofID := rev.GetString("proof"); proofID != "" {
					if proof, err := app.FindRecordById("proofs", proofID); err == nil && proof.GetBool("verified") {
						item.VerifiedProofs++
					}
				}
			}

			ranked = append(ranked, item)
		}

		out := &ListRankingsOutput{}
		out.Body.Rankings = ranked
		out.Body.Count = len(ranked)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "refresh-rankings",
		Method:      "POST",
		Path:        "/api/rankings/refresh",
		Summary:     "Recalculate all rankings",
		Description: "Triggers a full recalculation of rank scores for all skills.",
		Tags:        []string{"Rankings"},
	}, func(ctx context.Context, input *RefreshRankingsInput) (*RefreshRankingsOutput, error) {
		if _, err := RequireJWT(input.Authorization, jwtKey); err != nil {
			return nil, err
		}

		skills.UpdateAllRankings(app)

		out := &RefreshRankingsOutput{}
		out.Body.Message = fmt.Sprintf("Rankings refreshed")
		return out, nil
	})
}
