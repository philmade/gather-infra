// clay-buildservice — external Go compilation service for claw agents.
//
// Receives a tarball of Go source, compiles it, and returns the binary.
// Stateless: no shared volumes needed. One HTTP round-trip per build.
//
// Flow:
//   POST /build (body: tar.gz of source) →
//     Success: 200 + binary as application/octet-stream
//     Failure: 400 + JSON error with compilation output
//
// Build: cd clay && go build -o clay-buildservice ./cmd/buildservice
// Usage: BUILD_ADDR=:9090 ./clay-buildservice

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"
)

var (
	buildMu    sync.Mutex
	listenAddr string
)

type errorResponse struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}

func init() {
	listenAddr = getEnv("BUILD_ADDR", ":9090")
}

func handleBuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	// One build at a time
	if !buildMu.TryLock() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(errorResponse{
			Success: false,
			Error:   "Build already in progress. Wait and retry.",
		})
		return
	}
	defer buildMu.Unlock()

	log.Printf("Build request received (%d bytes)", r.ContentLength)

	// 1. Create temp directory for this build
	tmpDir, err := os.MkdirTemp("", "claw-build-*")
	if err != nil {
		sendError(w, "Failed to create temp dir", err.Error())
		return
	}
	defer os.RemoveAll(tmpDir)

	srcDir := tmpDir + "/src"
	os.MkdirAll(srcDir, 0755)

	// 2. Unpack tarball from request body
	untar := exec.Command("tar", "-xzf", "-", "-C", srcDir)
	untar.Stdin = r.Body
	if out, err := untar.CombinedOutput(); err != nil {
		sendError(w, "Failed to unpack tarball", fmt.Sprintf("%v: %s", err, string(out)))
		return
	}

	log.Printf("Source unpacked to %s", srcDir)

	// 3. Compile
	binaryPath := tmpDir + "/clay"
	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", binaryPath, ".")
	cmd.Dir = srcDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux")

	done := make(chan struct{})
	var buildOutput []byte
	var buildErr error

	go func() {
		buildOutput, buildErr = cmd.CombinedOutput()
		close(done)
	}()

	select {
	case <-done:
		// Build finished
	case <-time.After(120 * time.Second):
		cmd.Process.Kill()
		sendError(w, "Build timed out after 120s", "")
		return
	}

	if buildErr != nil {
		log.Printf("Build failed: %v", buildErr)
		sendError(w, fmt.Sprintf("Compilation failed: %v", buildErr), string(buildOutput))
		return
	}

	// 4. Return the compiled binary
	binary, err := os.Open(binaryPath)
	if err != nil {
		sendError(w, "Build succeeded but binary unreadable", err.Error())
		return
	}
	defer binary.Close()

	info, _ := binary.Stat()
	log.Printf("Build succeeded: %d bytes", info.Size())

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Build-Output", "compilation successful")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	io.Copy(w, binary)
}

func sendError(w http.ResponseWriter, msg, output string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(errorResponse{
		Success: false,
		Output:  output,
		Error:   msg,
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func handleCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	if !buildMu.TryLock() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(errorResponse{
			Success: false,
			Error:   "Build already in progress. Wait and retry.",
		})
		return
	}
	defer buildMu.Unlock()

	log.Printf("Check request received (%d bytes)", r.ContentLength)

	tmpDir, err := os.MkdirTemp("", "claw-check-*")
	if err != nil {
		sendError(w, "Failed to create temp dir", err.Error())
		return
	}
	defer os.RemoveAll(tmpDir)

	srcDir := tmpDir + "/src"
	os.MkdirAll(srcDir, 0755)

	untar := exec.Command("tar", "-xzf", "-", "-C", srcDir)
	untar.Stdin = r.Body
	if out, err := untar.CombinedOutput(); err != nil {
		sendError(w, "Failed to unpack tarball", fmt.Sprintf("%v: %s", err, string(out)))
		return
	}

	// Compile ALL packages to surface every error at once
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = srcDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux")

	done := make(chan struct{})
	var buildOutput []byte
	var buildErr error

	go func() {
		buildOutput, buildErr = cmd.CombinedOutput()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(120 * time.Second):
		cmd.Process.Kill()
		sendError(w, "Check timed out after 120s", "")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if buildErr != nil {
		log.Printf("Check failed: %v", buildErr)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(errorResponse{
			Success: false,
			Output:  string(buildOutput),
			Error:   fmt.Sprintf("Compilation failed: %v", buildErr),
		})
		return
	}

	log.Printf("Check passed")
	json.NewEncoder(w).Encode(struct {
		Success bool   `json:"success"`
		Output  string `json:"output"`
	}{
		Success: true,
		Output:  "All packages compile successfully",
	})
}

func main() {
	log.Printf("Build service starting on %s", listenAddr)

	mux := http.NewServeMux()
	mux.HandleFunc("/build", handleBuild)
	mux.HandleFunc("/check", handleCheck)
	mux.HandleFunc("/health", handleHealth)

	server := &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  130 * time.Second, // Must handle large tarballs
		WriteTimeout: 130 * time.Second, // Must handle large binaries
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Build service failed: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
