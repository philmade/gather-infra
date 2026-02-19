// clawpoint-buildservice — external Go build service for claw agents.
//
// Compiles clawpoint-go source and writes the binary to a shared volume.
// The claw's medic process detects the new binary and performs a hot swap.
//
// Build: cd clawpoint-go && go build -o clawpoint-buildservice ./cmd/buildservice
// Usage: BUILD_SRC=/src BUILD_OUT=/builds ./clawpoint-buildservice

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"
)

var (
	buildMu   sync.Mutex
	srcDir    string
	outDir    string
	listenAddr string
)

type buildRequest struct {
	Reason string `json:"reason"`
}

type buildResponse struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}

func init() {
	srcDir = getEnv("BUILD_SRC", "/src")
	outDir = getEnv("BUILD_OUT", "/builds")
	listenAddr = getEnv("BUILD_ADDR", ":9090")
}

func handleBuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	// Only one build at a time
	if !buildMu.TryLock() {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(buildResponse{
			Success: false,
			Error:   "Build already in progress. Wait and retry.",
		})
		return
	}
	defer buildMu.Unlock()

	var req buildRequest
	json.NewDecoder(r.Body).Decode(&req)

	log.Printf("Build requested: %s", req.Reason)

	// Run go build with a timeout
	outPath := outDir + "/clawpoint-go.new"
	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", outPath, ".")
	cmd.Dir = srcDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux")

	done := make(chan error, 1)
	var output []byte

	go func() {
		var err error
		output, err = cmd.CombinedOutput()
		done <- err
	}()

	select {
	case err := <-done:
		w.Header().Set("Content-Type", "application/json")

		if err != nil {
			log.Printf("Build failed: %v", err)
			json.NewEncoder(w).Encode(buildResponse{
				Success: false,
				Output:  string(output),
				Error:   fmt.Sprintf("Build failed: %v", err),
			})
			return
		}

		log.Printf("Build succeeded: %s", outPath)
		json.NewEncoder(w).Encode(buildResponse{
			Success: true,
			Output:  string(output),
		})

	case <-time.After(120 * time.Second):
		cmd.Process.Kill()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(buildResponse{
			Success: false,
			Error:   "Build timed out after 120s",
		})
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func main() {
	log.Printf("Build service starting on %s", listenAddr)
	log.Printf("  Source: %s", srcDir)
	log.Printf("  Output: %s", outDir)

	// Ensure output dir exists
	os.MkdirAll(outDir, 0755)

	mux := http.NewServeMux()
	mux.HandleFunc("/build", handleBuild)
	mux.HandleFunc("/health", handleHealth)

	server := &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 130 * time.Second, // Must exceed build timeout
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
