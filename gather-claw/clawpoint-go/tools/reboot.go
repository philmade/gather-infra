package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const PARAMEDIC_URL = "http://127.0.0.1:8001"

// RebootTool handles self-restart via paramedic agent
type RebootTool struct{}

// NewRebootTool creates a new reboot tool
func NewRebootTool() *RebootTool {
	return &RebootTool{}
}

// Reboot requests the paramedic agent to restart this agent
func (r *RebootTool) Reboot(reason string) (string, error) {
	agentName := "clawpoint-adk"
	
	// Create a session for the paramedic
	sessionData := map[string]interface{}{}
	sessionJSON, _ := json.Marshal(sessionData)
	
	req, err := http.NewRequest("POST", 
		fmt.Sprintf("%s/apps/paramedic/users/paramedic/sessions", PARAMEDIC_URL),
		bytes.NewBuffer(sessionJSON))
	if err != nil {
		return "", fmt.Errorf("failed to create session request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to connect to paramedic: %v", err)
	}
	defer resp.Body.Close()
	
	var session map[string]interface{}
	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &session)
	
	sessionID, ok := session["id"].(string)
	if !ok {
		return "", fmt.Errorf("failed to get session ID from paramedic")
	}
	
	// Send resuscitate message
	payload := map[string]interface{}{
		"app_name":   "paramedic",
		"user_id":    "paramedic",
		"session_id": sessionID,
		"new_message": map[string]interface{}{
			"role": "user",
			"parts": []map[string]string{{
				"text": fmt.Sprintf(
					"Resuscitate agent '%s'. Reason: %s. "+
						"Workflow: check health, if dead read the error, fix the code, "+
						"restart, verify alive. Max 3 attempts.",
					agentName, reason),
			}},
		},
	}
	
	payloadJSON, _ := json.Marshal(payload)
	req, err = http.NewRequest("POST",
		fmt.Sprintf("%s/run", PARAMEDIC_URL),
		bytes.NewBuffer(payloadJSON))
	if err != nil {
		return "", fmt.Errorf("failed to create reboot request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	
	resp, err = client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send reboot request: %v", err)
	}
	defer resp.Body.Close()
	
	result, _ := io.ReadAll(resp.Body)
	
	return fmt.Sprintf("Reboot signal sent to Paramedic (reason: %s). Response: %s", 
		reason, string(result)), nil
}
