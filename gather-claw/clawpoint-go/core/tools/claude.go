package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ClaudeTool shells out to Claude Code CLI for heavy coding tasks.
type ClaudeTool struct {
	timeout time.Duration
	maxOut  int
}

// NewClaudeTool creates a Claude Code tool.
func NewClaudeTool() *ClaudeTool {
	return &ClaudeTool{
		timeout: 600 * time.Second,
		maxOut:  5000,
	}
}

// Run executes a task via Claude Code CLI.
func (c *ClaudeTool) Run(task, workingDir string) (string, error) {
	if task == "" {
		return "", fmt.Errorf("task is required")
	}
	if workingDir == "" {
		workingDir = "."
	}

	// Build environment without CLAUDECODE to avoid recursion
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, "CLAUDECODE=") {
			filtered = append(filtered, e)
		}
	}

	cmd := exec.Command("claude", "-p",
		"--dangerously-skip-permissions",
		"--output-format", "json",
		task,
	)
	cmd.Dir = workingDir
	cmd.Env = filtered

	done := make(chan struct{})
	var out []byte
	var cmdErr error

	go func() {
		out, cmdErr = cmd.CombinedOutput()
		close(done)
	}()

	select {
	case <-done:
		// Command finished
	case <-time.After(c.timeout):
		cmd.Process.Kill()
		return "", fmt.Errorf("claude code timed out after %v", c.timeout)
	}

	// Try to parse JSON output
	var result string
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err == nil {
		if r, ok := parsed["result"].(string); ok {
			result = r
		} else {
			result = string(out)
		}
	} else {
		result = string(out)
	}

	if len(result) > c.maxOut {
		result = result[:c.maxOut] + fmt.Sprintf("\n... truncated (%d chars)", len(result))
	}

	if cmdErr != nil && result == "" {
		return "", fmt.Errorf("claude code failed: %v", cmdErr)
	}

	return result, nil
}
