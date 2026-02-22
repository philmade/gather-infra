package api

import (
	"fmt"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// StartHeartbeat launches a background goroutine that sends periodic heartbeat
// messages to claws that have heartbeat_interval > 0 and status = "running".
func StartHeartbeat(app *pocketbase.PocketBase) {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			processHeartbeats(app)
		}
	}()
	app.Logger().Info("Heartbeat scheduler started (1-minute tick)")
}

func processHeartbeats(app *pocketbase.PocketBase) {
	records, err := app.FindRecordsByFilter("claw_deployments",
		"status = 'running' && heartbeat_interval > 0", "", 100, 0, nil)
	if err != nil || len(records) == 0 {
		return
	}

	now := time.Now().UTC()

	for _, r := range records {
		interval := time.Duration(int(r.GetFloat("heartbeat_interval"))) * time.Minute
		lastStr := r.GetString("last_heartbeat")

		if lastStr != "" {
			last, err := time.Parse(time.RFC3339, lastStr)
			if err == nil && now.Sub(last) < interval {
				continue // not due yet
			}
		}

		sendHeartbeat(app, r, now)
	}
}

func sendHeartbeat(app *pocketbase.PocketBase, r *core.Record, now time.Time) {
	containerID := r.GetString("container_id")
	agentID := r.GetString("agent_id")
	instruction := r.GetString("heartbeat_instruction")
	clawName := r.GetString("name")

	if containerID == "" || agentID == "" {
		return
	}

	msg := "[HEARTBEAT]"
	if instruction != "" {
		msg += " " + instruction
	}

	reply, err := sendToADK(containerID, "heartbeat", msg)
	if err != nil {
		app.Logger().Warn("Heartbeat failed",
			"claw", clawName, "container", containerID, "error", err)
		// Still update last_heartbeat so we don't spam a broken claw every minute
		r.Set("last_heartbeat", now.Format(time.RFC3339))
		app.Save(r)
		return
	}

	// Save reply as channel message (suppress HEARTBEAT_OK idle signals)
	channelID, err := findClawChannel(app, agentID)
	if err == nil && reply != "" && strings.TrimSpace(reply) != "HEARTBEAT_OK" {
		col, err := app.FindCollectionByNameOrId("channel_messages")
		if err == nil {
			rec := core.NewRecord(col)
			rec.Set("channel_id", channelID)
			rec.Set("author_id", agentID)
			rec.Set("body", reply)
			if err := app.Save(rec); err != nil {
				app.Logger().Warn("Failed to save heartbeat reply",
					"claw", clawName, "error", err)
			}
		}
	}

	r.Set("last_heartbeat", now.Format(time.RFC3339))
	if err := app.Save(r); err != nil {
		app.Logger().Warn("Failed to update last_heartbeat",
			"claw", clawName, "error", err)
	}

	app.Logger().Info(fmt.Sprintf("Heartbeat sent to %s", clawName),
		"claw", clawName, "reply_len", len(reply))
}
