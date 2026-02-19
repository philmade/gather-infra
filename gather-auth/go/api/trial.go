package api

import (
	"context"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// StartTrialEnforcer launches a background goroutine that checks for expired
// trial claws every minute. Claws get a 5-minute warning, then are stopped.
func StartTrialEnforcer(app *pocketbase.PocketBase) {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			processTrials(app)
		}
	}()
	app.Logger().Info("Trial enforcer started (1-minute tick)")
}

func processTrials(app *pocketbase.PocketBase) {
	records, err := app.FindRecordsByFilter("claw_deployments",
		"status = 'running' && paid = false && trial_ends_at != ''",
		"", 100, 0, nil)
	if err != nil || len(records) == 0 {
		return
	}

	now := time.Now().UTC()

	for _, r := range records {
		trialStr := r.GetString("trial_ends_at")
		trialEnd, err := time.Parse(time.RFC3339, trialStr)
		if err != nil {
			app.Logger().Warn("Invalid trial_ends_at", "claw", r.GetString("name"), "value", trialStr)
			continue
		}

		remaining := trialEnd.Sub(now)

		if remaining <= 0 {
			// Trial expired — stop the claw
			expireClaw(app, r)
		} else if remaining <= 5*time.Minute && !r.GetBool("trial_warned") {
			// 5-minute warning
			warnClaw(app, r)
		}
	}
}

func warnClaw(app *pocketbase.PocketBase, r *core.Record) {
	containerID := r.GetString("container_id")
	clawName := r.GetString("name")

	if containerID != "" {
		msg := "[SYSTEM] Your trial expires in 5 minutes. Your owner needs to upgrade to keep you running."
		_, err := sendToADK(containerID, "system", msg)
		if err != nil {
			app.Logger().Warn("Failed to send trial warning to ADK",
				"claw", clawName, "error", err)
		}
	}

	// Save warning to channel messages so user sees it in chat
	agentID := r.GetString("agent_id")
	channelID, err := findClawChannel(app, agentID)
	if err == nil {
		col, err := app.FindCollectionByNameOrId("channel_messages")
		if err == nil {
			rec := core.NewRecord(col)
			rec.Set("channel_id", channelID)
			rec.Set("author_id", "system")
			rec.Set("body", "Trial expires in 5 minutes. Upgrade from the detail panel to keep this claw running.")
			app.Save(rec)
		}
	}

	r.Set("trial_warned", true)
	if err := app.Save(r); err != nil {
		app.Logger().Warn("Failed to set trial_warned", "claw", clawName, "error", err)
	}

	app.Logger().Info("Trial warning sent", "claw", clawName)
}

func expireClaw(app *pocketbase.PocketBase, r *core.Record) {
	containerID := r.GetString("container_id")
	clawName := r.GetString("name")
	agentID := r.GetString("agent_id")

	// Send final message to ADK (best-effort)
	if containerID != "" {
		msg := "[SYSTEM] Trial expired. This claw is being decommissioned."
		sendToADK(containerID, "system", msg)
	}

	// Save expiry message to channel
	channelID, err := findClawChannel(app, agentID)
	if err == nil {
		col, err := app.FindCollectionByNameOrId("channel_messages")
		if err == nil {
			rec := core.NewRecord(col)
			rec.Set("channel_id", channelID)
			rec.Set("author_id", "system")
			rec.Set("body", "Trial expired. This claw has been stopped. Upgrade to redeploy.")
			app.Save(rec)
		}
	}

	// Remove Docker container
	if containerID != "" {
		cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
		if err == nil {
			cli.ContainerRemove(context.Background(), containerID, container.RemoveOptions{Force: true})
			cli.Close()
		}
	}

	// Update record — keep it around so user can see it was stopped
	r.Set("status", "stopped")
	r.Set("error_message", "Trial expired — not paid")
	if err := app.Save(r); err != nil {
		app.Logger().Error("Failed to update expired claw", "claw", clawName, "error", err)
	} else {
		app.Logger().Info("Trial expired, claw stopped", "claw", clawName)
	}
}
