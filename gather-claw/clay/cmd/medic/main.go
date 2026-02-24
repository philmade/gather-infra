// clay-medic — process supervisor with hot-swap and rollback.
//
// Watches agent logs for crash signatures, performs health checks, and
// manages binary hot-swaps from the external build service.
//
// Hot-swap flow:
//   1. Build service writes new binary to /app/builds/clay.new
//   2. Medic detects the file, backs up current binary to .prev
//   3. Medic replaces current binary and restarts the agent
//   4. If new binary crashes within 30s: revert to .prev, log failure
//   5. Agent reads failure logs on next startup to learn what went wrong
//
// Build: cd clay && go build -o clay-medic ./cmd/medic
// Usage: ./clay-medic

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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
	"clay": {
		LogFile:        "/tmp/adk-go.log",
		WorkingDir:     projectRoot(),
		HealthURL:      "http://127.0.0.1:" + adkPort(),
		ProcessPattern: "clay web",
		RestartCmd:     "cd " + projectRoot() + " && ./clay web -port " + adkPort() + " api webui > /tmp/adk-go.log 2>&1",
	},
	"clay-bridge": {
		LogFile:        "/tmp/bridge.log",
		WorkingDir:     projectRoot(),
		ProcessPattern: "clay-bridge",
		RestartCmd:     "cd " + projectRoot() + " && ADK_URL=http://127.0.0.1:" + adkPort() + " ./clay-bridge > /tmp/bridge.log 2>&1",
	},
}

// Death signatures — any match in a log line triggers investigation.
var deathSignatures = []string{
	`panic:`,
	`fatal error:`,
	`server failed:`,
	`FATAL`,
	`Segmentation fault`,
}

var deathPatterns []*regexp.Regexp

const (
	cooldownSeconds     = 90
	healthCheckInterval = 60 * time.Second
	maxRestartAttempts  = 3
	healthTimeout       = 5 * time.Second
	logContextLines     = 80
	startupWait         = 8 * time.Second
	initialHealthDelay  = 30 * time.Second
	hotSwapCheckInterval = 5 * time.Second
	hotSwapStabilityWait = 30 * time.Second
)

// Hot-swap paths
var (
	binaryPath    = projectRoot() + "/clay"
	newBinaryPath = projectRoot() + "/builds/clay.new"
	prevBinaryPath = projectRoot() + "/clay.prev"
	failureLogDir  = projectRoot() + "/data/build-failures"
)

func init() {
	deathPatterns = make([]*regexp.Regexp, len(deathSignatures))
	for i, sig := range deathSignatures {
		deathPatterns[i] = regexp.MustCompile(sig)
	}
}

func projectRoot() string {
	if root := os.Getenv("CLAY_ROOT"); root != "" {
		return root
	}
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
		return false
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
// Error capture (replaces Claude Code diagnosis)
// ---------------------------------------------------------------------------

func captureContext(logFile string) string {
	f, err := os.Open(logFile)
	if err != nil {
		return fmt.Sprintf("Failed to read log: %v", err)
	}
	defer f.Close()

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
	time.Sleep(2 * time.Second)
}

func startAgent(agentName string, cfg agentConfig) bool {
	if f, err := os.OpenFile(cfg.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		fmt.Fprintf(f, "\n--- MEDIC RESTART at %s ---\n", time.Now().Format(time.RFC3339))
		f.Close()
	}

	cmd := exec.Command("bash", "-c", cfg.RestartCmd)
	cmd.Dir = cfg.WorkingDir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		logMsg("Failed to start %s: %v", agentName, err)
		return false
	}
	cmd.Process.Release()
	return true
}

// ---------------------------------------------------------------------------
// Crash Handler (simplified — no Claude Code, just restart)
// ---------------------------------------------------------------------------

func handleCrash(ctx context.Context, agentName string, cfg agentConfig, trigger string) {
	now := time.Now()
	lastActionMu.Lock()
	if last, ok := lastAction[agentName]; ok && now.Sub(last).Seconds() < cooldownSeconds {
		lastActionMu.Unlock()
		return
	}
	lastAction[agentName] = now
	lastActionMu.Unlock()

	logMsg("CRASH DETECTED: %s", agentName)
	trimmed := trigger
	if len(trimmed) > 200 {
		trimmed = trimmed[:200]
	}
	logMsg("Trigger: %s", strings.TrimSpace(trimmed))

	// Confirm dead
	if cfg.HealthURL != "" && checkHealth(cfg) {
		logMsg("False alarm — %s is still responding", agentName)
		return
	}

	logMsg("Confirmed dead. Capturing error context...")
	errContext := captureContext(cfg.LogFile)

	// Log the crash for the agent to learn from on next startup
	writeFailureLog(agentName, "crash", errContext)

	// Simple restart (up to maxRestartAttempts)
	for attempt := 1; attempt <= maxRestartAttempts; attempt++ {
		logMsg("Restart attempt %d/%d for %s...", attempt, maxRestartAttempts, agentName)

		killAgent(cfg)
		if !startAgent(agentName, cfg) {
			logMsg("Failed to start %s", agentName)
			continue
		}

		time.Sleep(startupWait)

		if cfg.HealthURL != "" && checkHealth(cfg) {
			logMsg("SUCCESS: %s is back up (attempt %d)", agentName, attempt)
			return
		}
		if cfg.HealthURL == "" {
			logMsg("SUCCESS: %s restarted (no health endpoint to verify)", agentName)
			return
		}

		logMsg("Still dead after attempt %d", attempt)
	}

	logMsg("FAILED: Could not recover %s after %d attempts", agentName, maxRestartAttempts)
}

// ---------------------------------------------------------------------------
// Hot-swap: watch for new binary from build service
// ---------------------------------------------------------------------------

func watchForNewBinary(ctx context.Context) {
	logMsg("Hot-swap watcher started (checking %s)", newBinaryPath)

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(hotSwapCheckInterval):
		}

		// Check if new binary exists
		info, err := os.Stat(newBinaryPath)
		if err != nil || info.Size() == 0 {
			continue
		}

		logMsg("New binary detected: %s (%d bytes)", newBinaryPath, info.Size())
		performHotSwap(ctx)
	}
}

func performHotSwap(ctx context.Context) {
	cfg := agents["clay"]

	// 1. Backup current binary
	logMsg("Backing up current binary to %s", prevBinaryPath)
	if err := copyFile(binaryPath, prevBinaryPath); err != nil {
		logMsg("Failed to backup binary: %v", err)
		os.Remove(newBinaryPath)
		return
	}

	// 2. Stop current agent
	logMsg("Stopping current clay...")
	killAgent(cfg)

	// 3. Replace binary
	logMsg("Replacing binary with new version...")
	if err := copyFile(newBinaryPath, binaryPath); err != nil {
		logMsg("Failed to replace binary: %v — reverting", err)
		copyFile(prevBinaryPath, binaryPath)
		os.Remove(newBinaryPath)
		startAgent("clay", cfg)
		return
	}
	os.Chmod(binaryPath, 0755)
	os.Remove(newBinaryPath)

	// 4. Start new binary
	logMsg("Starting new binary...")
	if !startAgent("clay", cfg) {
		logMsg("Failed to start new binary — reverting")
		copyFile(prevBinaryPath, binaryPath)
		startAgent("clay", cfg)
		writeFailureLog("clay", "hot-swap", "Failed to start new binary")
		return
	}

	// 5. Wait for stability period
	logMsg("Watching for stability (%v)...", hotSwapStabilityWait)
	stableUntil := time.Now().Add(hotSwapStabilityWait)

	for time.Now().Before(stableUntil) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}

		if cfg.HealthURL != "" && !checkHealth(cfg) {
			// Might just be starting up — give it more time if within first 10s
			if time.Until(stableUntil) > 20*time.Second {
				continue
			}

			logMsg("New binary appears dead during stability check — reverting")
			errContext := captureContext(cfg.LogFile)
			writeFailureLog("clay", "hot-swap-crash", errContext)

			killAgent(cfg)
			logMsg("Restoring previous binary...")
			copyFile(prevBinaryPath, binaryPath)
			os.Chmod(binaryPath, 0755)
			startAgent("clay", cfg)
			logMsg("Reverted to previous binary")
			return
		}
	}

	logMsg("Hot-swap SUCCESS: new binary is stable")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}

	return out.Close()
}

func writeFailureLog(agentName, category, content string) {
	os.MkdirAll(failureLogDir, 0755)
	ts := time.Now().Format("2006-01-02T15-04-05")
	filename := filepath.Join(failureLogDir, fmt.Sprintf("%s_%s_%s.log", ts, agentName, category))

	header := fmt.Sprintf("Agent: %s\nCategory: %s\nTime: %s\n---\n\n",
		agentName, category, time.Now().Format(time.RFC3339))

	os.WriteFile(filename, []byte(header+content), 0644)
	logMsg("Failure log written: %s", filename)
}

// ---------------------------------------------------------------------------
// Watchers
// ---------------------------------------------------------------------------

func watchLogs(ctx context.Context, agentName string, cfg agentConfig) {
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

		cmd.Wait()
		if ctx.Err() != nil {
			return
		}
		logMsg("Log watcher for %s exited, restarting...", agentName)
		time.Sleep(5 * time.Second)
	}
}

func periodicHealthCheck(ctx context.Context) {
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
					continue
				}
				if !checkHealth(cfg) {
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

	// Ensure failure log dir exists
	os.MkdirAll(failureLogDir, 0755)

	// Start log watcher goroutines
	for name, cfg := range agents {
		go watchLogs(ctx, name, cfg)
		logMsg("Watching logs: %s -> %s", name, cfg.LogFile)
	}

	// Start periodic health checker
	go periodicHealthCheck(ctx)
	logMsg("Periodic health checker started")

	// Start hot-swap watcher
	go watchForNewBinary(ctx)
	logMsg("Hot-swap watcher started")

	// Quick status report
	printStatus()

	logMsg("Medic is ready.")

	// Stdin commands
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
	time.Sleep(500 * time.Millisecond)
}

func agentNames() string {
	names := make([]string, 0, len(agents))
	for name := range agents {
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}
