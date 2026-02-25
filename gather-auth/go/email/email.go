// Package email sends transactional emails via the Cloudflare Email Worker.
//
// Configure with EMAIL_WORKER_URL and EMAIL_WORKER_TOKEN env vars.
// When EMAIL_WORKER_URL is empty, emails are logged instead of sent (dev mode).
package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

var client = &http.Client{Timeout: 10 * time.Second}

type payload struct {
	To       string `json:"to"`
	Subject  string `json:"subject"`
	HTML     string `json:"html"`
	From     string `json:"from,omitempty"`
	FromName string `json:"fromName,omitempty"`
}

// Send sends an email via the Cloudflare Email Worker as noreply@gather.is.
// Falls back to logging in dev mode when EMAIL_WORKER_URL is not set.
func Send(to, subject, html string) error {
	return SendAs(to, subject, html, "noreply@gather.is", "Gather")
}

// SendAs sends an email via the Cloudflare Email Worker with a custom from address.
func SendAs(to, subject, html, from, fromName string) error {
	workerURL := os.Getenv("EMAIL_WORKER_URL")
	workerToken := os.Getenv("EMAIL_WORKER_TOKEN")

	if workerURL == "" {
		log.Printf("[EMAIL DEV] From: %s <%s> | To: %s | Subject: %s", fromName, from, to, subject)
		return nil
	}

	body, _ := json.Marshal(payload{
		To:       to,
		Subject:  subject,
		HTML:     html,
		From:     from,
		FromName: fromName,
	})

	req, err := http.NewRequest("POST", workerURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("email: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if workerToken != "" {
		req.Header.Set("Authorization", "Bearer "+workerToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("email: send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("email: worker returned %d", resp.StatusCode)
	}

	return nil
}
