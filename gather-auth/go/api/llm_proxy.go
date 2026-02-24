package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// ---------------------------------------------------------------------------
// Tier configuration
// ---------------------------------------------------------------------------

type tierConfig struct {
	MonthlyTokenCap int64
	WindowReqs      int
	WindowHours     int
}

var tiers = map[string]tierConfig{
	"lite": {MonthlyTokenCap: 15_000_000, WindowReqs: 100, WindowHours: 5},
	"pro":  {MonthlyTokenCap: 50_000_000, WindowReqs: 300, WindowHours: 5},
	"max":  {MonthlyTokenCap: 150_000_000, WindowReqs: 1000, WindowHours: 5},
}

// ---------------------------------------------------------------------------
// Route registration (raw HTTP — not Huma, passes body verbatim)
// ---------------------------------------------------------------------------

func RegisterLLMProxyRoutes(mux *http.ServeMux, app *pocketbase.PocketBase) {
	mux.HandleFunc("POST /api/llm/v1/messages", handleLLMProxy(app))
}

// ---------------------------------------------------------------------------
// Proxy handler
// ---------------------------------------------------------------------------

func handleLLMProxy(app *pocketbase.PocketBase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Extract proxy token
		token := r.Header.Get("x-api-key")
		if token == "" {
			writeAnthropicError(w, 401, "authentication_error", "Missing x-api-key header")
			return
		}

		// 2. Look up claw deployment by proxy token
		records, err := app.FindRecordsByFilter("claw_deployments",
			"proxy_token = {:token} && status = 'running'",
			"", 1, 0,
			map[string]any{"token": token})
		if err != nil || len(records) == 0 {
			writeAnthropicError(w, 401, "authentication_error", "Invalid proxy token")
			return
		}
		claw := records[0]
		clawID := claw.Id

		// 3. Determine tier and check quotas
		clawType := claw.GetString("claw_type")
		tier, ok := tiers[clawType]
		if !ok {
			tier = tiers["lite"] // default to lite
		}

		if err := checkQuota(app, clawID, tier); err != nil {
			if err == errMonthlyCapExceeded {
				writeAnthropicError(w, 403, "permission_error",
					"Monthly token quota exceeded. Upgrade your plan or wait for next billing cycle.")
				return
			}
			// Rolling window exceeded
			writeAnthropicError(w, 429, "rate_limit_error",
				"Rate limit exceeded. Please wait before making more requests.")
			return
		}

		// 4. Read request body (limit 1MB)
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeAnthropicError(w, 400, "invalid_request_error", "Failed to read request body")
			return
		}

		// 5. Forward to upstream
		upstreamURL := os.Getenv("LLM_UPSTREAM_URL")
		if upstreamURL == "" {
			app.Logger().Error("LLM_UPSTREAM_URL not configured")
			writeAnthropicError(w, 500, "api_error", "LLM proxy not configured")
			return
		}
		upstreamKey := os.Getenv("LLM_UPSTREAM_KEY")

		// Append /v1/messages — LLM_UPSTREAM_URL is the base (e.g. https://api.minimax.io/anthropic)
		upstreamFull := strings.TrimRight(upstreamURL, "/") + "/v1/messages"
		upReq, err := http.NewRequestWithContext(r.Context(), "POST", upstreamFull,
			bytes.NewReader(body))
		if err != nil {
			writeAnthropicError(w, 500, "api_error", "Failed to create upstream request")
			return
		}

		// Copy relevant headers
		upReq.Header.Set("Content-Type", "application/json")
		upReq.Header.Set("x-api-key", upstreamKey)
		if v := r.Header.Get("anthropic-version"); v != "" {
			upReq.Header.Set("anthropic-version", v)
		}

		resp, err := http.DefaultClient.Do(upReq)
		if err != nil {
			app.Logger().Error("LLM upstream request failed", "claw_id", clawID, "error", err)
			writeAnthropicError(w, 502, "api_error", "Failed to reach LLM upstream")
			return
		}
		defer resp.Body.Close()

		// 6. Read response
		respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10MB limit
		if err != nil {
			writeAnthropicError(w, 502, "api_error", "Failed to read upstream response")
			return
		}

		// 7. Record usage in background (best-effort)
		go recordUsage(app, clawID, respBody)

		// 8. Return response verbatim
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(respBody)
	}
}

// ---------------------------------------------------------------------------
// Quota enforcement
// ---------------------------------------------------------------------------

var errMonthlyCapExceeded = fmt.Errorf("monthly cap exceeded")
var errWindowExceeded = fmt.Errorf("rolling window exceeded")

func checkQuota(app *pocketbase.PocketBase, clawID string, tier tierConfig) error {
	// Rolling window: count requests in the last N hours
	windowStart := time.Now().UTC().Add(-time.Duration(tier.WindowHours) * time.Hour).Format("2006-01-02 15:04:05.000Z")
	windowRecords, err := app.FindRecordsByFilter("claw_usage",
		"claw_id = {:cid} && created >= {:start}",
		"", 0, 0,
		map[string]any{"cid": clawID, "start": windowStart})
	if err == nil && len(windowRecords) >= tier.WindowReqs {
		return errWindowExceeded
	}

	// Monthly cap: sum tokens this month
	monthStart := time.Now().UTC().Format("2006-01") + "-01 00:00:00.000Z"
	var totalTokens float64
	err = app.DB().NewQuery("SELECT COALESCE(SUM(input_tokens + output_tokens), 0) as total FROM claw_usage WHERE claw_id = {:cid} AND created >= {:start}").
		Bind(map[string]any{"cid": clawID, "start": monthStart}).
		Row(&totalTokens)
	if err == nil && int64(totalTokens) >= tier.MonthlyTokenCap {
		return errMonthlyCapExceeded
	}

	return nil
}

// ---------------------------------------------------------------------------
// Usage recording (runs in goroutine, best-effort)
// ---------------------------------------------------------------------------

type llmUsageResponse struct {
	Usage struct {
		InputTokens  int    `json:"input_tokens"`
		OutputTokens int    `json:"output_tokens"`
	} `json:"usage"`
	Model string `json:"model"`
}

func recordUsage(app *pocketbase.PocketBase, clawID string, respBody []byte) {
	var resp llmUsageResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return // non-JSON response, skip
	}
	if resp.Usage.InputTokens == 0 && resp.Usage.OutputTokens == 0 {
		return // no usage to record (likely an error response)
	}

	col, err := app.FindCollectionByNameOrId("claw_usage")
	if err != nil {
		return
	}

	rec := core.NewRecord(col)
	rec.Set("claw_id", clawID)
	rec.Set("input_tokens", resp.Usage.InputTokens)
	rec.Set("output_tokens", resp.Usage.OutputTokens)
	rec.Set("model", resp.Model)
	if err := app.Save(rec); err != nil {
		app.Logger().Warn("Failed to record LLM usage", "claw_id", clawID, "error", err)
	}
}

// ---------------------------------------------------------------------------
// Usage cleanup (90-day retention)
// ---------------------------------------------------------------------------

func StartUsageCleanup(app *pocketbase.PocketBase) {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		// Run once on startup too
		cleanOldUsage(app)

		for range ticker.C {
			cleanOldUsage(app)
		}
	}()
	app.Logger().Info("Usage cleanup started (daily tick, 90-day retention)")
}

func cleanOldUsage(app *pocketbase.PocketBase) {
	cutoff := time.Now().UTC().Add(-90 * 24 * time.Hour).Format("2006-01-02 15:04:05.000Z")
	_, err := app.DB().NewQuery("DELETE FROM claw_usage WHERE created < {:cutoff}").
		Bind(map[string]any{"cutoff": cutoff}).Execute()
	if err != nil {
		app.Logger().Warn("Failed to clean old usage records", "error", err)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeAnthropicError(w http.ResponseWriter, status int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    errType,
			"message": message,
		},
	})
}

