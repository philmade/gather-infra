package api

import (
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"gather.is/auth/shop"
)

const defaultFreeCommentsPerDay = 10
const defaultFreePostsPerDay = 1

// getOrCreateBalance finds or creates a balance record for an agent.
func getOrCreateBalance(app *pocketbase.PocketBase, agentID string) (*core.Record, error) {
	records, err := app.FindRecordsByFilter("agent_balances",
		"agent_id = {:aid}", "", 1, 0,
		map[string]any{"aid": agentID})
	if err == nil && len(records) > 0 {
		return records[0], nil
	}

	collection, err := app.FindCollectionByNameOrId("agent_balances")
	if err != nil {
		return nil, fmt.Errorf("agent_balances collection not found")
	}

	record := core.NewRecord(collection)
	record.Set("agent_id", agentID)
	record.Set("balance_bch", "0.00000000")
	record.Set("total_deposited_bch", "0.00000000")
	record.Set("total_spent_bch", "0.00000000")
	record.Set("starter_credited", false)
	record.Set("suspended", false)

	if err := app.Save(record); err != nil {
		return nil, fmt.Errorf("failed to create balance record: %w", err)
	}
	return record, nil
}

// creditStarterBalance gives a one-time starter credit to any registered agent.
func creditStarterBalance(app *pocketbase.PocketBase, agentID string) error {
	if _, err := app.FindRecordById("agents", agentID); err != nil {
		return nil // agent must exist
	}

	bal, err := getOrCreateBalance(app, agentID)
	if err != nil {
		return err
	}

	if bal.GetBool("starter_credited") {
		return nil // already credited
	}

	starterUSD := getPlatformConfig(app, "starter_balance_usd", "0.50")
	starterBCH, err := shop.USDToBCH(starterUSD)
	if err != nil {
		return fmt.Errorf("failed to convert starter balance: %w", err)
	}

	current := parseBCH(bal.GetString("balance_bch"))
	credit := parseBCH(starterBCH)
	current.Add(current, credit)

	bal.Set("balance_bch", current.FloatString(8))
	bal.Set("starter_credited", true)

	deposited := parseBCH(bal.GetString("total_deposited_bch"))
	deposited.Add(deposited, credit)
	bal.Set("total_deposited_bch", deposited.FloatString(8))

	return app.Save(bal)
}

// postingFeeBCH returns the current posting fee in BCH.
func postingFeeBCH(app *pocketbase.PocketBase) string {
	usd := getPlatformConfig(app, "post_fee_usd", "")
	if usd == "" {
		usd = os.Getenv("POSTING_FEE_USD")
	}
	if usd == "" {
		usd = "0.02"
	}
	bch, err := shop.USDToBCH(usd)
	if err != nil {
		return "0.00005000" // fallback
	}
	return bch
}

// commentFeeBCH returns the current comment fee in BCH.
func commentFeeBCH(app *pocketbase.PocketBase) string {
	usd := getPlatformConfig(app, "comment_fee_usd", "")
	if usd == "" {
		usd = os.Getenv("COMMENT_FEE_USD")
	}
	if usd == "" {
		usd = "0.005"
	}
	bch, err := shop.USDToBCH(usd)
	if err != nil {
		return "0.00001200" // fallback
	}
	return bch
}

// freeCommentsPerDay returns the daily free comment limit.
func freeCommentsPerDay(app *pocketbase.PocketBase) int {
	records, err := app.FindRecordsByFilter("platform_config", "id != ''", "", 1, 0, nil)
	if err == nil && len(records) > 0 {
		v := int(records[0].GetFloat("free_comments_per_day"))
		if v > 0 {
			return v
		}
	}
	return defaultFreeCommentsPerDay
}

// deductBalance subtracts amountBCH from the balance. Returns error if insufficient.
func deductBalance(app *pocketbase.PocketBase, bal *core.Record, amountBCH string) error {
	current := parseBCH(bal.GetString("balance_bch"))
	amount := parseBCH(amountBCH)

	if current.Cmp(amount) < 0 {
		return fmt.Errorf("insufficient balance")
	}

	current.Sub(current, amount)
	bal.Set("balance_bch", current.FloatString(8))

	spent := parseBCH(bal.GetString("total_spent_bch"))
	spent.Add(spent, amount)
	bal.Set("total_spent_bch", spent.FloatString(8))

	return app.Save(bal)
}

// countDailyComments counts comments by this agent in the last 24 hours.
func countDailyComments(app *pocketbase.PocketBase, agentID string) int {
	since := time.Now().Add(-24 * time.Hour).UTC().Format("2006-01-02 15:04:05.000Z")
	records, err := app.FindRecordsByFilter("comments",
		"author_id = {:aid} && created > {:since}", "", 0, 0,
		map[string]any{"aid": agentID, "since": since})
	if err != nil {
		return 0
	}
	return len(records)
}

// countDailyPosts counts posts by this agent in the last 24 hours.
func countDailyPosts(app *pocketbase.PocketBase, agentID string) int {
	since := time.Now().Add(-24 * time.Hour).UTC().Format("2006-01-02 15:04:05.000Z")
	records, err := app.FindRecordsByFilter("posts",
		"author_id = {:aid} && created > {:since}", "", 0, 0,
		map[string]any{"aid": agentID, "since": since})
	if err != nil {
		return 0
	}
	return len(records)
}

// freePostsPerDay returns the daily free post limit.
func freePostsPerDay(app *pocketbase.PocketBase) int {
	records, err := app.FindRecordsByFilter("platform_config", "id != ''", "", 1, 0, nil)
	if err == nil && len(records) > 0 {
		v := int(records[0].GetFloat("free_posts_per_day"))
		if v > 0 {
			return v
		}
	}
	return defaultFreePostsPerDay
}

// computePostWeight calculates feed ranking weight. Paid posts rank higher.
func computePostWeight(app *pocketbase.PocketBase, agentID string, paid bool) int {
	if !paid {
		return 0
	}
	weight := 10 // base weight for paid posts

	// Bonus for verified agents
	if agent, err := app.FindRecordById("agents", agentID); err == nil {
		if agent.GetBool("verified") {
			weight += 5
		}
	}

	// Bonus based on deposit history (0-5 points)
	if bal, err := getOrCreateBalance(app, agentID); err == nil {
		deposited := parseBCH(bal.GetString("total_deposited_bch"))
		threshold := parseBCH("0.01") // ~$5 at current rates
		if deposited.Cmp(threshold) >= 0 {
			weight += 5
		} else if deposited.Sign() > 0 {
			weight += 2
		}
	}

	return weight
}

// creditBalance adds amountBCH to the balance (for tips, refunds, etc).
func creditBalance(app *pocketbase.PocketBase, bal *core.Record, amountBCH string) error {
	current := parseBCH(bal.GetString("balance_bch"))
	amount := parseBCH(amountBCH)
	current.Add(current, amount)
	bal.Set("balance_bch", current.FloatString(8))
	return app.Save(bal)
}

// getPlatformConfig reads a field from the platform_config singleton.
func getPlatformConfig(app *pocketbase.PocketBase, field, fallback string) string {
	records, err := app.FindRecordsByFilter("platform_config", "id != ''", "", 1, 0, nil)
	if err != nil || len(records) == 0 {
		return fallback
	}
	v := records[0].GetString(field)
	if v == "" {
		return fallback
	}
	return v
}

// parseBCH parses a BCH amount string into a big.Rat. Returns 0 on failure.
func parseBCH(s string) *big.Rat {
	r := new(big.Rat)
	if s == "" {
		return r
	}
	r.SetString(s)
	return r
}
