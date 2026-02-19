// clawpoint-medic — process supervisor that auto-recovers crashed agents.
//
// Tails agent log files, detects death signatures, confirms via health check,
// diagnoses via Claude Code CLI, restarts the agent, and verifies recovery.
//
// Build: cd clawpoint-go && go build -o clawpoint-medic ./cmd/medic
// Usage: ./clawpoint-medic

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

type agentConfig struct {
	LogFile        string
	WorkingDir     string
	HealthURL      string
	ProcessPattern string
	RestartCmd     string
}

func adkPort() string {
	if p := os.Getenv("ADK_PORT"); p != "" {
		return p
	}
	return "8081"
}

var agents = map[string]agentConfig{
	"clawpoint-go": {
		LogFile:        "/tmp/adk-go.log",
		WorkingDir:     projectRoot() + "/clawpoint-go",
		HealthURL:      "http://127.0.0.1:" + adkPort(),
		ProcessPattern: "clawpoint-go web",
		RestartCmd:     "cd " + projectRoot() + " && PORT=" + adkPort() + " ./clawpoint-go web api webui > /tmp/adk-go.log 2>&1",
	},
	"clawpoint-bridge": {
		LogFile:        "/tmp/bridge.log",
		WorkingDir:     projectRoot() + "/clawpoint-go",
		ProcessPattern: "clawpoint-bridge",
		RestartCmd:     "cd " + projectRoot() + " && ADK_URL=http://127.0.0.1:" + adkPort() + " ./clawpoint-bridge > /tmp/bridge.log 2>&1",
	},
}

// Death signatures — any match in a log line triggers investigation.
var deathSignatures = []string{
	// Go
	`panic:`,
	`fatal error:`,
	`server failed:`,
	// General
	`FATAL`,
	`Segmentation fault`,
}

var deathPatterns []*regexp.Regexp

const (
	cooldownSeconds      = 90
	healthCheckInterval  = 60 * time.Second
	maxFixAttempts       = 3
	claudeTimeout        = 180 * time.Second
	healthTimeout        = 5 * time.Second
	logContextLines      = 80
	startupWait          = 8 * time.Second
	initialHealthDelay   = 30 * time.Second
)

func init() {
	deathPatterns = make([]*regexp.Regexp, len(deathSignatures))
	for i, sig := range deathSignatures {
		deathPatterns[i] = regexp.MustCompile(sig)
	}
}

func projectRoot() string {
	// Use CLAWPOINT_ROOT env var if set (for container deployment)
	if root := os.Getenv("CLAWPOINT_ROOT"); root != "" {
		return root
	}
	// Medic binary lives in clawpoint-go/cmd/medic, project root is two up from there.
	// But we resolve from the working directory since the binary is invoked from project root.
	home, _ := os.UserHomeDir()
	return home + "/gather-claw"
}

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

var (
	lastAction   = make(map[string]time.Time)
	lastActionMu sync.Mutex
)

// ---------------------------------------------------------------------------
// Logging
// ---------------------------------------------------------------------------

func logMsg(format string, args ...any) {
	ts := time.Now().Format("15:04:05")
	fmt.Printf("[%s] [MEDIC] %s\n", ts, fmt.Sprintf(format, args...))
}

// ---------------------------------------------------------------------------
// Detection
// ---------------------------------------------------------------------------

func detectCrash(line string) bool {
	for _, p := range deathPatterns {
		if p.MatchString(line) {
			return true
		}
	}
	return false
}

func checkHealth(cfg agentConfig) bool {
	if cfg.HealthURL == "" {
		return false // No health endpoint — can't verify
	}
	client := &http.Client{Timeout: healthTimeout}
	resp, err := client.Get(cfg.HealthURL)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode > 0 && resp.StatusCode < 500
}

// ---------------------------------------------------------------------------
// Diagnosis & Fix
// ---------------------------------------------------------------------------

func captureContext(logFile string) string {
	f, err := os.Open(logFile)
	if err != nil {
		return fmt.Sprintf("Failed to read log: %v", err)
	}
	defer f.Close()

	// Read all lines, keep last N
	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	start := 0
	if len(lines) > logContextLines {
		start = len(lines) - logContextLines
	}
	return strings.Join(lines[start:], "\n")
}

func diagnoseAndFix(ctx context.Context, agentName string, cfg agentConfig, errorContext string) string {
	// Truncate context to last 4000 chars
	if len(errorContext) > 4000 {
		errorContext = errorContext[len(errorContext)-4000:]
	}

	task := fmt.Sprintf(
		"The agent '%s' has crashed. Here is the error from the logs:\n\n"+
			"```\n%s\n```\n\n"+
			"The agent code is in %s.\n"+
			"Diagnose the error and make the minimum surgical fix to resolve the crash. "+
			"Do NOT refactor, improve, or add features — just fix the crash. "+
			"Report exactly what you changed.",
		agentName, errorContext, cfg.WorkingDir,
	)

	// Filter out CLAUDECODE env var to avoid recursion
	env := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "CLAUDECODE=") {
			env = append(env, e)
		}
	}

	claudeCtx, cancel := context.WithTimeout(ctx, claudeTimeout)
	defer cancel()

	cmd := exec.CommandContext(claudeCtx, "claude", "-p",
		"--dangerously-skip-permissions",
		"--output-format", "json",
		task,
	)
	cmd.Dir = cfg.WorkingDir
	cmd.Env = env

	out, err := cmd.CombinedOutput()
	if claudeCtx.Err() == context.DeadlineExceeded {
		return "Claude Code timed out after 180s"
	}
	if err != nil && len(out) == 0 {
		return fmt.Sprintf("Claude Code failed: %v", err)
	}

	// Try to parse JSON output
	var parsed map[string]any
	if jsonErr := json.Unmarshal(out, &parsed); jsonErr == nil {
		if r, ok := parsed["result"].(string); ok {
			if len(r) > 3000 {
				return r[:3000]
			}
			return r
		}
	}

	result := string(out)
	if len(result) > 3000 {
		return result[:3000]
	}
	if result == "" {
		return "(no output)"
	}
	return result
}

// ---------------------------------------------------------------------------
// Restart
// ---------------------------------------------------------------------------

func killAgent(cfg agentConfig) {
	out, err := exec.Command("pgrep", "-f", cfg.ProcessPattern).Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pid := strings.TrimSpace(line)
		if pid != "" {
			exec.Command("kill", pid).Run()
		}
	}
	time.Sleep(2 * time.Second) // Wait for graceful shutdown
}

func startAgent(agentName string, cfg agentConfig) bool {
	// Append restart marker
	if f, err := os.OpenFile(cfg.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		fmt.Fprintf(f, "\n--- MEDIC RESTART at %s ---\n", time.Now().Format(time.RFC3339))
		f.Close()
	}

	cmd := exec.Command("bash", "-c", cfg.RestartCmd)
	cmd.Dir = cfg.WorkingDir
	// Detach: start in new session
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		logMsg("Failed to start %s: %v", agentName, err)
		return false
	}
	// Release — don't wait for it
	cmd.Process.Release()
	return true
}

// ---------------------------------------------------------------------------
// Crash Handler
// ---------------------------------------------------------------------------

func handleCrash(ctx context.Context, agentName string, cfg agentConfig, trigger string) {
	now := time.Now()
	lastActionMu.Lock()
	if last, ok := lastAction[agentName]; ok && now.Sub(last).Seconds() < cooldownSeconds {
		lastActionMu.Unlock()
		return // Cooldown active
	}
	lastAction[agentName] = now
	lastActionMu.Unlock()

	logMsg("CRASH DETECTED: %s", agentName)
	trimmed := trigger
	if len(trimmed) > 200 {
		trimmed = trimmed[:200]
	}
	logMsg("Trigger: %s", strings.TrimSpace(trimmed))

	// 1. Confirm dead (avoid false positives from error logging)
	if cfg.HealthURL != "" && checkHealth(cfg) {
		logMsg("False alarm — %s is still responding", agentName)
		return
	}

	logMsg("Confirmed dead. Capturing error context...")

	// 2. Capture error context
	errContext := captureContext(cfg.LogFile)

	// 3. Diagnose and fix (up to maxFixAttempts)
	for attempt := 1; attempt <= maxFixAttempts; attempt++ {
		logMsg("Fix attempt %d/%d...", attempt, maxFixAttempts)

		report := diagnoseAndFix(ctx, agentName, cfg, errContext)
		if len(report) > 300 {
			logMsg("Fix report: %s", report[:300])
		} else {
			logMsg("Fix report: %s", report)
		}

		// 4. Restart
		logMsg("Restarting %s...", agentName)
		killAgent(cfg)
		if !startAgent(agentName, cfg) {
			logMsg("Failed to restart %s", agentName)
			continue
		}

		// 5. Wait for startup
		time.Sleep(startupWait)

		// 6. Verify
		if cfg.HealthURL != "" && checkHealth(cfg) {
			logMsg("SUCCESS: %s is back up (attempt %d)", agentName, attempt)
			return
		}
		// For agents without health URL, assume success if start didn't fail
		if cfg.HealthURL == "" {
			logMsg("SUCCESS: %s restarted (no health endpoint to verify)", agentName)
			return
		}

		logMsg("Still dead after attempt %d", attempt)
		// Re-capture context for next attempt
		errContext = captureContext(cfg.LogFile)
	}

	logMsg("FAILED: Could not recover %s after %d attempts", agentName, maxFixAttempts)
}

// ---------------------------------------------------------------------------
// Watchers
// ---------------------------------------------------------------------------

func watchLogs(ctx context.Context, agentName string, cfg agentConfig) {
	// Touch the log file so tail doesn't fail
	if f, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
		f.Close()
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		cmd := exec.CommandContext(ctx, "tail", "-f", "-n", "0", cfg.LogFile)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			logMsg("Log watcher pipe error for %s: %v", agentName, err)
			time.Sleep(5 * time.Second)
			continue
		}

		if err := cmd.Start(); err != nil {
			logMsg("Log watcher start error for %s: %v", agentName, err)
			time.Sleep(5 * time.Second)
			continue
		}

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if detectCrash(line) {
				handleCrash(ctx, agentName, cfg, line)
			}
		}

		// If we get here, tail exited — wait and retry
		cmd.Wait()
		if ctx.Err() != nil {
			return
		}
		logMsg("Log watcher for %s exited, restarting...", agentName)
		time.Sleep(5 * time.Second)
	}
}

func periodicHealthCheck(ctx context.Context) {
	// Initial delay — let agents finish starting
	select {
	case <-time.After(initialHealthDelay):
	case <-ctx.Done():
		return
	}

	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for name, cfg := range agents {
				if cfg.HealthURL == "" {
					continue // No health endpoint
				}
				if !checkHealth(cfg) {
					// Only act if not in cooldown
					lastActionMu.Lock()
					inCooldown := time.Since(lastAction[name]).Seconds() < cooldownSeconds
					lastActionMu.Unlock()
					if inCooldown {
						continue
					}

					logMsg("Health check failed for %s", name)
					handleCrash(ctx, name, cfg, "periodic health check — no response")
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Status reporting
// ---------------------------------------------------------------------------

func printStatus() {
	logMsg("Agent status:")
	for name, cfg := range agents {
		status := "unknown"
		if cfg.HealthURL != "" {
			if checkHealth(cfg) {
				status = "UP"
			} else {
				status = "DOWN"
			}
		} else {
			// Check by process pattern
			if err := exec.Command("pgrep", "-f", cfg.ProcessPattern).Run(); err == nil {
				status = "running (no health URL)"
			} else {
				status = "not found"
			}
		}
		logMsg("  %s: %s (log: %s)", name, status, cfg.LogFile)
	}
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logMsg("Starting medic watcher")
	logMsg("Watching %d agents: %s", len(agents), agentNames())
	logMsg("Death signatures: %d", len(deathSignatures))
	logMsg("Cooldown: %ds | Health check every: %v", cooldownSeconds, healthCheckInterval)

	// Start log watcher goroutines
	for name, cfg := range agents {
		go watchLogs(ctx, name, cfg)
		logMsg("Watching logs: %s -> %s", name, cfg.LogFile)
	}

	// Start periodic health checker
	go periodicHealthCheck(ctx)
	logMsg("Periodic health checker started")

	// Quick status report
	printStatus()

	logMsg("Medic is ready.")

	// Wait for signal, then read from stdout for status reporting
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					return
				}
				continue
			}
			if strings.TrimSpace(line) == "status" {
				printStatus()
			}
		}
	}()

	<-sigChan
	logMsg("Shutting down...")
	cancel()
	// Give goroutines a moment to clean up
	time.Sleep(500 * time.Millisecond)
}

func agentNames() string {
	names := make([]string, 0, len(agents))
	for name := range agents {
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}
