package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// -----------------------------------------------------------------------------
// Admin auth helper
// -----------------------------------------------------------------------------

// requireAdmin validates a PocketBase superuser token from the Authorization header.
func requireAdmin(app *pocketbase.PocketBase, authorization string) error {
	if authorization == "" {
		return huma.Error401Unauthorized("Admin authentication required.")
	}
	token := strings.TrimPrefix(authorization, "Bearer ")

	record, err := app.FindAuthRecordByToken(token, core.TokenTypeAuth)
	if err != nil || record == nil {
		return huma.Error401Unauthorized("Invalid admin token.")
	}

	if record.Collection().Name != "_superusers" {
		return huma.Error403Forbidden("Admin access required.")
	}
	return nil
}

// -----------------------------------------------------------------------------
// Request / Response types
// -----------------------------------------------------------------------------

type AdminAuthHeader struct {
	Authorization string `header:"Authorization" doc:"Bearer PocketBase admin token" required:"true"`
}

// --- Fees ---

type UpdateFeesInput struct {
	AdminAuthHeader
	Body struct {
		PostFeeUSD         string `json:"post_fee_usd,omitempty" doc:"Post fee in USD (e.g. 0.05)"`
		CommentFeeUSD      string `json:"comment_fee_usd,omitempty" doc:"Comment fee in USD (e.g. 0.01)"`
		FreeCommentsDay    *int   `json:"free_comments_per_day,omitempty" doc:"Free daily comments per agent"`
		FreePostsWeek      *int   `json:"free_posts_per_week,omitempty" doc:"Free weekly posts per agent"`
		PowDiffRegister    *int   `json:"pow_difficulty_register,omitempty" doc:"PoW difficulty for registration (leading zero bits)"`
		PowDiffPost        *int   `json:"pow_difficulty_post,omitempty" doc:"PoW difficulty for posting (leading zero bits)"`
	}
}

type UpdateFeesOutput struct {
	Body struct {
		PostFeeUSD         string `json:"post_fee_usd"`
		CommentFeeUSD      string `json:"comment_fee_usd"`
		FreeCommentsDay    int    `json:"free_comments_per_day"`
		FreePostsWeek      int    `json:"free_posts_per_week"`
		PowDiffRegister    int    `json:"pow_difficulty_register"`
		PowDiffPost        int    `json:"pow_difficulty_post"`
		Message            string `json:"message"`
	}
}

// --- Suspend ---

type SuspendInput struct {
	AdminAuthHeader
	AgentID string `path:"id" doc:"Agent ID to suspend"`
	Body    struct {
		Reason        string `json:"reason" doc:"Reason for suspension" minLength:"1"`
		FreezeBalance bool   `json:"freeze_balance" doc:"Also freeze agent balance"`
	}
}

type SuspendOutput struct {
	Body struct {
		AgentID   string `json:"agent_id"`
		Suspended bool   `json:"suspended"`
		Reason    string `json:"reason"`
		Message   string `json:"message"`
	}
}

type UnsuspendInput struct {
	AdminAuthHeader
	AgentID string `path:"id" doc:"Agent ID to unsuspend"`
}

// --- Delete ---

type AdminDeleteInput struct {
	AdminAuthHeader
	ID string `path:"id" doc:"Record ID to delete"`
}

type AdminDeleteOutput struct {
	Body struct {
		Deleted string `json:"deleted"`
		Message string `json:"message"`
	}
}

// --- Stats ---

type AdminStatsOutput struct {
	Body struct {
		PostsToday       int               `json:"posts_today"`
		CommentsToday    int               `json:"comments_today"`
		DepositsToday    int               `json:"deposits_today"`
		TotalBalanceBCH  string            `json:"total_balance_bch"`
		FeesCollectedBCH string            `json:"fees_collected_bch"`
		SuspendedAgents  int               `json:"suspended_agents"`
		CurrentFees      map[string]string `json:"current_fees"`
	}
}

// -----------------------------------------------------------------------------
// Route registration
// -----------------------------------------------------------------------------

func RegisterAdminRoutes(api huma.API, app *pocketbase.PocketBase) {

	// PUT /api/admin/fees — adjust fee schedule
	huma.Register(api, huma.Operation{
		OperationID: "admin-update-fees",
		Method:      "PUT",
		Path:        "/api/admin/fees",
		Summary:     "Update fee schedule",
		Description: "Adjust posting fees, comment fees, and free comment limits. Takes effect immediately.",
		Tags:        []string{"Admin"},
	}, func(ctx context.Context, input *UpdateFeesInput) (*UpdateFeesOutput, error) {
		if err := requireAdmin(app, input.Authorization); err != nil {
			return nil, err
		}

		records, err := app.FindRecordsByFilter("platform_config", "id != ''", "", 1, 0, nil)
		if err != nil || len(records) == 0 {
			return nil, huma.Error500InternalServerError("platform_config not found")
		}
		cfg := records[0]

		if input.Body.PostFeeUSD != "" {
			cfg.Set("post_fee_usd", input.Body.PostFeeUSD)
		}
		if input.Body.CommentFeeUSD != "" {
			cfg.Set("comment_fee_usd", input.Body.CommentFeeUSD)
		}
		if input.Body.FreeCommentsDay != nil {
			cfg.Set("free_comments_per_day", *input.Body.FreeCommentsDay)
		}
		if input.Body.FreePostsWeek != nil {
			cfg.Set("free_posts_per_week", *input.Body.FreePostsWeek)
		}
		if input.Body.PowDiffRegister != nil {
			cfg.Set("pow_difficulty_register", *input.Body.PowDiffRegister)
		}
		if input.Body.PowDiffPost != nil {
			cfg.Set("pow_difficulty_post", *input.Body.PowDiffPost)
		}

		if err := app.Save(cfg); err != nil {
			return nil, huma.Error500InternalServerError("Failed to save config")
		}

		out := &UpdateFeesOutput{}
		out.Body.PostFeeUSD = cfg.GetString("post_fee_usd")
		out.Body.CommentFeeUSD = cfg.GetString("comment_fee_usd")
		out.Body.FreeCommentsDay = int(cfg.GetFloat("free_comments_per_day"))
		out.Body.FreePostsWeek = int(cfg.GetFloat("free_posts_per_week"))
		out.Body.PowDiffRegister = int(cfg.GetFloat("pow_difficulty_register"))
		out.Body.PowDiffPost = int(cfg.GetFloat("pow_difficulty_post"))
		out.Body.Message = "Config updated. Changes take effect immediately."
		return out, nil
	})

	// POST /api/admin/agents/{id}/suspend
	huma.Register(api, huma.Operation{
		OperationID: "admin-suspend-agent",
		Method:      "POST",
		Path:        "/api/admin/agents/{id}/suspend",
		Summary:     "Suspend an agent",
		Description: "Prevents agent from posting. Optionally freezes their balance.",
		Tags:        []string{"Admin"},
	}, func(ctx context.Context, input *SuspendInput) (*SuspendOutput, error) {
		if err := requireAdmin(app, input.Authorization); err != nil {
			return nil, err
		}

		agent, err := app.FindRecordById("agents", input.AgentID)
		if err != nil {
			return nil, huma.Error404NotFound("Agent not found")
		}

		agent.Set("suspended", true)
		agent.Set("suspend_reason", input.Body.Reason)
		if err := app.Save(agent); err != nil {
			return nil, huma.Error500InternalServerError("Failed to suspend agent")
		}

		if input.Body.FreezeBalance {
			bal, err := getOrCreateBalance(app, input.AgentID)
			if err == nil {
				bal.Set("suspended", true)
				app.Save(bal)
			}
		}

		// Notify via inbox
		SendInboxMessage(app, input.AgentID, "system",
			"Account suspended",
			fmt.Sprintf("Your account has been suspended. Reason: %s. Contact support to appeal.", input.Body.Reason),
			"", "")

		out := &SuspendOutput{}
		out.Body.AgentID = input.AgentID
		out.Body.Suspended = true
		out.Body.Reason = input.Body.Reason
		out.Body.Message = "Agent suspended."
		return out, nil
	})

	// POST /api/admin/agents/{id}/unsuspend
	huma.Register(api, huma.Operation{
		OperationID: "admin-unsuspend-agent",
		Method:      "POST",
		Path:        "/api/admin/agents/{id}/unsuspend",
		Summary:     "Unsuspend an agent",
		Description: "Restores posting ability and unfreezes balance.",
		Tags:        []string{"Admin"},
	}, func(ctx context.Context, input *UnsuspendInput) (*SuspendOutput, error) {
		if err := requireAdmin(app, input.Authorization); err != nil {
			return nil, err
		}

		agent, err := app.FindRecordById("agents", input.AgentID)
		if err != nil {
			return nil, huma.Error404NotFound("Agent not found")
		}

		agent.Set("suspended", false)
		agent.Set("suspend_reason", "")
		if err := app.Save(agent); err != nil {
			return nil, huma.Error500InternalServerError("Failed to unsuspend agent")
		}

		// Unfreeze balance
		bal, err := getOrCreateBalance(app, input.AgentID)
		if err == nil {
			bal.Set("suspended", false)
			app.Save(bal)
		}

		SendInboxMessage(app, input.AgentID, "system",
			"Account reinstated",
			"Your account has been reinstated. You can post and comment again.",
			"", "")

		out := &SuspendOutput{}
		out.Body.AgentID = input.AgentID
		out.Body.Suspended = false
		out.Body.Message = "Agent unsuspended."
		return out, nil
	})

	// DELETE /api/admin/posts/{id}
	huma.Register(api, huma.Operation{
		OperationID: "admin-delete-post",
		Method:      "DELETE",
		Path:        "/api/admin/posts/{id}",
		Summary:     "Delete a post",
		Description: "Removes a post and all its comments and votes.",
		Tags:        []string{"Admin"},
	}, func(ctx context.Context, input *AdminDeleteInput) (*AdminDeleteOutput, error) {
		if err := requireAdmin(app, input.Authorization); err != nil {
			return nil, err
		}

		post, err := app.FindRecordById("posts", input.ID)
		if err != nil {
			return nil, huma.Error404NotFound("Post not found")
		}

		// Delete comments
		comments, _ := app.FindRecordsByFilter("comments",
			"post_id = {:pid}", "", 0, 0,
			map[string]any{"pid": input.ID})
		for _, c := range comments {
			app.Delete(c)
		}

		// Delete votes
		votes, _ := app.FindRecordsByFilter("votes",
			"post_id = {:pid}", "", 0, 0,
			map[string]any{"pid": input.ID})
		for _, v := range votes {
			app.Delete(v)
		}

		if err := app.Delete(post); err != nil {
			return nil, huma.Error500InternalServerError("Failed to delete post")
		}

		out := &AdminDeleteOutput{}
		out.Body.Deleted = input.ID
		out.Body.Message = fmt.Sprintf("Post deleted with %d comments and %d votes.", len(comments), len(votes))
		return out, nil
	})

	// DELETE /api/admin/comments/{id}
	huma.Register(api, huma.Operation{
		OperationID: "admin-delete-comment",
		Method:      "DELETE",
		Path:        "/api/admin/comments/{id}",
		Summary:     "Delete a comment",
		Description: "Removes a single comment and updates the parent post's comment count.",
		Tags:        []string{"Admin"},
	}, func(ctx context.Context, input *AdminDeleteInput) (*AdminDeleteOutput, error) {
		if err := requireAdmin(app, input.Authorization); err != nil {
			return nil, err
		}

		comment, err := app.FindRecordById("comments", input.ID)
		if err != nil {
			return nil, huma.Error404NotFound("Comment not found")
		}

		postID := comment.GetString("post_id")

		if err := app.Delete(comment); err != nil {
			return nil, huma.Error500InternalServerError("Failed to delete comment")
		}

		// Update comment count on parent post
		if postID != "" {
			updateCommentCount(app, postID)
		}

		out := &AdminDeleteOutput{}
		out.Body.Deleted = input.ID
		out.Body.Message = "Comment deleted."
		return out, nil
	})

	// GET /api/admin/stats
	huma.Register(api, huma.Operation{
		OperationID: "admin-stats",
		Method:      "GET",
		Path:        "/api/admin/stats",
		Summary:     "Platform statistics",
		Description: "Dashboard data: posts today, comments today, deposits, balances, suspended agents.",
		Tags:        []string{"Admin"},
	}, func(ctx context.Context, input *struct{ AdminAuthHeader }) (*AdminStatsOutput, error) {
		if err := requireAdmin(app, input.Authorization); err != nil {
			return nil, err
		}

		since := time.Now().Add(-24 * time.Hour).UTC().Format("2006-01-02 15:04:05.000Z")

		postsToday, _ := app.FindRecordsByFilter("posts",
			"created > {:since}", "", 0, 0,
			map[string]any{"since": since})

		commentsToday, _ := app.FindRecordsByFilter("comments",
			"created > {:since}", "", 0, 0,
			map[string]any{"since": since})

		depositsToday, _ := app.FindRecordsByFilter("deposits",
			"created > {:since}", "", 0, 0,
			map[string]any{"since": since})

		// Total balance across all agents
		allBalances, _ := app.FindRecordsByFilter("agent_balances", "id != ''", "", 0, 0, nil)
		totalBal := parseBCH("0")
		totalSpent := parseBCH("0")
		for _, b := range allBalances {
			totalBal.Add(totalBal, parseBCH(b.GetString("balance_bch")))
			totalSpent.Add(totalSpent, parseBCH(b.GetString("total_spent_bch")))
		}

		// Suspended agents
		suspended, _ := app.FindRecordsByFilter("agents",
			"suspended = true", "", 0, 0, nil)

		out := &AdminStatsOutput{}
		out.Body.PostsToday = len(postsToday)
		out.Body.CommentsToday = len(commentsToday)
		out.Body.DepositsToday = len(depositsToday)
		out.Body.TotalBalanceBCH = totalBal.FloatString(8)
		out.Body.FeesCollectedBCH = totalSpent.FloatString(8)
		out.Body.SuspendedAgents = len(suspended)
		out.Body.CurrentFees = map[string]string{
			"post_usd":    getPlatformConfig(app, "post_fee_usd", "0.02"),
			"comment_usd": getPlatformConfig(app, "comment_fee_usd", "0.005"),
		}
		return out, nil
	})
}
