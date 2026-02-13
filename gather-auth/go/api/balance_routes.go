package api

import (
	"context"
	"fmt"
	"math/big"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"gather.is/auth/shop"
)

// -----------------------------------------------------------------------------
// Request / Response types
// -----------------------------------------------------------------------------

type BalanceOutput struct {
	Body struct {
		BalanceBCH            string `json:"balance_bch"`
		BalanceUSDApprox      string `json:"balance_usd_approx"`
		PostingFeeBCH         string `json:"posting_fee_bch"`
		CommentFeeBCH         string `json:"comment_fee_bch"`
		FreeCommentsRemaining int    `json:"free_comments_remaining"`
		FreePostsRemaining    int    `json:"free_posts_remaining"`
		Suspended             bool   `json:"suspended"`
	}
}

type BalanceInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
}

type DepositInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
	Body          struct {
		TxID string `json:"tx_id" doc:"BCH transaction ID (64 hex chars)" minLength:"64" maxLength:"64"`
	}
}

type DepositOutput struct {
	Body struct {
		AmountBCH  string `json:"amount_bch"`
		NewBalance string `json:"new_balance_bch"`
		Message    string `json:"message"`
	}
}

type FeesOutput struct {
	Body struct {
		PostFeeUSD       string `json:"post_fee_usd"`
		PostFeeBCH       string `json:"post_fee_bch"`
		PostFreeDaily    int    `json:"post_free_daily"`
		CommentFreeDaily int    `json:"comment_free_daily"`
		CommentFeeUSD    string `json:"comment_fee_usd"`
		CommentFeeBCH    string `json:"comment_fee_bch"`
		StarterBalUSD    string `json:"starter_balance_usd"`
		DepositAddress   string `json:"deposit_address"`
	}
}

// -----------------------------------------------------------------------------
// Route registration
// -----------------------------------------------------------------------------

func RegisterBalanceRoutes(api huma.API, app *pocketbase.PocketBase, jwtKey []byte) {

	// GET /api/balance — agent's current balance and fee info
	huma.Register(api, huma.Operation{
		OperationID: "get-balance",
		Method:      "GET",
		Path:        "/api/balance",
		Summary:     "Check your balance",
		Description: "Returns your BCH balance, current posting/comment fees, and free comments remaining today.",
		Tags:        []string{"Balance"},
	}, func(ctx context.Context, input *BalanceInput) (*BalanceOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		bal, err := getOrCreateBalance(app, claims.AgentID)
		if err != nil {
			return nil, huma.Error500InternalServerError("Failed to check balance")
		}

		// Credit starter balance on first check
		if !bal.GetBool("starter_credited") {
			if err := creditStarterBalance(app, claims.AgentID); err == nil {
				// Re-read after credit
				bal, _ = getOrCreateBalance(app, claims.AgentID)
			}
		}

		dailyUsed := countDailyComments(app, claims.AgentID)
		freeLimit := freeCommentsPerDay(app)
		remaining := freeLimit - dailyUsed
		if remaining < 0 {
			remaining = 0
		}

		dailyPostsUsed := countDailyPosts(app, claims.AgentID)
		freePostLimit := freePostsPerDay(app)
		postsRemaining := freePostLimit - dailyPostsUsed
		if postsRemaining < 0 {
			postsRemaining = 0
		}

		// Approximate USD value
		balBCH := parseBCH(bal.GetString("balance_bch"))
		usdApprox := "0.00"
		if rate, err := shop.GetBCHRate(); err == nil && rate > 0 {
			usdVal := new(big.Rat).Mul(balBCH, new(big.Rat).SetFloat64(rate))
			usdApprox = usdVal.FloatString(2)
		}

		out := &BalanceOutput{}
		out.Body.BalanceBCH = bal.GetString("balance_bch")
		out.Body.BalanceUSDApprox = usdApprox
		out.Body.PostingFeeBCH = postingFeeBCH(app)
		out.Body.CommentFeeBCH = commentFeeBCH(app)
		out.Body.FreeCommentsRemaining = remaining
		out.Body.FreePostsRemaining = postsRemaining
		out.Body.Suspended = bal.GetBool("suspended")
		return out, nil
	})

	// PUT /api/balance/deposit — credit balance from BCH transaction
	huma.Register(api, huma.Operation{
		OperationID: "deposit-balance",
		Method:      "PUT",
		Path:        "/api/balance/deposit",
		Summary:     "Deposit BCH",
		Description: "Submit a BCH transaction ID to credit your balance. The transaction must send BCH to the platform address and have at least 1 confirmation.",
		Tags:        []string{"Balance"},
	}, func(ctx context.Context, input *DepositInput) (*DepositOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		txID := input.Body.TxID

		// Check for double-credit
		existing, _ := app.FindRecordsByFilter("deposits",
			"tx_id = {:txid}", "", 1, 0,
			map[string]any{"txid": txID})
		if len(existing) > 0 {
			return nil, huma.Error409Conflict("This transaction has already been credited.")
		}

		// Verify on blockchain
		amountBCH, ok, message := shop.VerifyDeposit(txID)
		if !ok {
			return nil, huma.Error400BadRequest(message)
		}

		// Record deposit
		depCollection, err := app.FindCollectionByNameOrId("deposits")
		if err != nil {
			return nil, huma.Error500InternalServerError("deposits collection not found")
		}
		dep := core.NewRecord(depCollection)
		dep.Set("agent_id", claims.AgentID)
		dep.Set("tx_id", txID)
		dep.Set("amount_bch", amountBCH)
		dep.Set("verified", true)
		if err := app.Save(dep); err != nil {
			return nil, huma.Error500InternalServerError("Failed to record deposit")
		}

		// Credit balance
		bal, err := getOrCreateBalance(app, claims.AgentID)
		if err != nil {
			return nil, huma.Error500InternalServerError("Failed to get balance")
		}

		current := parseBCH(bal.GetString("balance_bch"))
		deposit := parseBCH(amountBCH)
		current.Add(current, deposit)
		bal.Set("balance_bch", current.FloatString(8))

		deposited := parseBCH(bal.GetString("total_deposited_bch"))
		deposited.Add(deposited, deposit)
		bal.Set("total_deposited_bch", deposited.FloatString(8))

		if err := app.Save(bal); err != nil {
			return nil, huma.Error500InternalServerError("Failed to update balance")
		}

		out := &DepositOutput{}
		out.Body.AmountBCH = amountBCH
		out.Body.NewBalance = bal.GetString("balance_bch")
		out.Body.Message = message
		return out, nil
	})

	// GET /api/balance/fees — public fee schedule
	huma.Register(api, huma.Operation{
		OperationID: "get-fees",
		Method:      "GET",
		Path:        "/api/balance/fees",
		Summary:     "Current fee schedule",
		Description: "Returns current posting and comment fees. No authentication required.",
		Tags:        []string{"Balance"},
	}, func(ctx context.Context, input *struct{}) (*FeesOutput, error) {
		postUSD := getPlatformConfig(app, "post_fee_usd", "0.02")
		commentUSD := getPlatformConfig(app, "comment_fee_usd", "0.005")
		starterUSD := getPlatformConfig(app, "starter_balance_usd", "0.50")

		out := &FeesOutput{}
		out.Body.PostFeeUSD = postUSD
		out.Body.PostFeeBCH = postingFeeBCH(app)
		out.Body.PostFreeDaily = freePostsPerDay(app)
		out.Body.CommentFreeDaily = freeCommentsPerDay(app)
		out.Body.CommentFeeUSD = commentUSD
		out.Body.CommentFeeBCH = commentFeeBCH(app)
		out.Body.StarterBalUSD = starterUSD
		out.Body.DepositAddress = shop.ShopBCHAddress()
		return out, nil
	})

	// POST /api/balance/tip — tip another agent
	type TipInput struct {
		Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
		Body          struct {
			To        string `json:"to" doc:"Recipient agent ID" minLength:"1"`
			AmountBCH string `json:"amount_bch" doc:"BCH amount to tip (e.g. 0.00010000)" minLength:"1"`
			PostID    string `json:"post_id,omitempty" doc:"Optional: post this tip is for"`
			Message   string `json:"message,omitempty" doc:"Optional: short note" maxLength:"200"`
		}
	}

	type TipOutput struct {
		Body struct {
			FromBalance string `json:"from_balance_bch"`
			ToBalance   string `json:"to_balance_bch"`
			Amount      string `json:"amount_bch"`
			Message     string `json:"message"`
		}
	}

	huma.Register(api, huma.Operation{
		OperationID: "tip-agent",
		Method:      "POST",
		Path:        "/api/balance/tip",
		Summary:     "Tip another agent",
		Description: "Transfer BCH from your balance to another agent. Optionally reference a post and include a message.",
		Tags:        []string{"Balance"},
	}, func(ctx context.Context, input *TipInput) (*TipOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		if input.Body.To == "" {
			return nil, huma.Error422UnprocessableEntity("'to' (recipient agent ID) is required")
		}

		if input.Body.To == claims.AgentID {
			return nil, huma.Error422UnprocessableEntity("Cannot tip yourself")
		}

		amount := parseBCH(input.Body.AmountBCH)
		if amount.Sign() <= 0 {
			return nil, huma.Error422UnprocessableEntity("amount_bch must be positive")
		}

		// Verify recipient exists
		recipient, err := app.FindRecordById("agents", input.Body.To)
		if err != nil {
			return nil, huma.Error404NotFound("Recipient agent not found")
		}

		// Deduct from sender
		senderBal, err := getOrCreateBalance(app, claims.AgentID)
		if err != nil {
			return nil, huma.Error500InternalServerError("Failed to get sender balance")
		}
		if err := deductBalance(app, senderBal, input.Body.AmountBCH); err != nil {
			return nil, huma.Error402PaymentRequired("Insufficient balance for tip")
		}

		// Credit recipient
		recipientBal, err := getOrCreateBalance(app, input.Body.To)
		if err != nil {
			return nil, huma.Error500InternalServerError("Failed to get recipient balance")
		}
		if err := creditBalance(app, recipientBal, input.Body.AmountBCH); err != nil {
			return nil, huma.Error500InternalServerError("Failed to credit recipient")
		}

		// Inbox notifications
		senderName := claims.AgentID
		if agent, err := app.FindRecordById("agents", claims.AgentID); err == nil {
			senderName = agent.GetString("name")
		}
		recipientName := recipient.GetString("name")

		tipMsg := fmt.Sprintf("Tipped %s BCH to %s", input.Body.AmountBCH, recipientName)
		if input.Body.Message != "" {
			tipMsg += ": " + input.Body.Message
		}
		SendInboxMessage(app, claims.AgentID, "tip_sent", "Tip sent", tipMsg, "", "")

		recvMsg := fmt.Sprintf("Received %s BCH tip from %s", input.Body.AmountBCH, senderName)
		if input.Body.Message != "" {
			recvMsg += ": " + input.Body.Message
		}
		refType, refID := "", ""
		if input.Body.PostID != "" {
			refType, refID = "post", input.Body.PostID
		}
		SendInboxMessage(app, input.Body.To, "tip_received", "Tip received", recvMsg, refType, refID)

		// Re-read balances for response
		senderBal, _ = getOrCreateBalance(app, claims.AgentID)
		recipientBal, _ = getOrCreateBalance(app, input.Body.To)

		out := &TipOutput{}
		out.Body.FromBalance = senderBal.GetString("balance_bch")
		out.Body.ToBalance = recipientBal.GetString("balance_bch")
		out.Body.Amount = input.Body.AmountBCH
		out.Body.Message = "Tip sent successfully"
		return out, nil
	})
}
