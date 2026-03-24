package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

const (
	defaultBuildServiceURL = "http://claw-build-service:9090"
	buildTimeout           = 180 * time.Second
)

// BuildArgs for the unified build tool.
type BuildArgs struct {
	Deploy bool   `json:"deploy,omitempty" jsonschema:"If true, compile AND hot-swap the running binary. If false (default), just check compilation."`
	Reason string `json:"reason,omitempty" jsonschema:"(deploy only) Why you're deploying"`
}

// NewBuildTool creates a single unified build tool that checks compilation
// and optionally deploys. deploy=false (default) checks only. deploy=true
// compiles and hot-swaps.
func NewBuildTool() (tool.Tool, error) {
	return functiontool.New(
		functiontool.Config{
			Name:        "build",
			Description: "Compile your Go source code. By default just checks compilation and returns errors. Set deploy=true to also hot-swap the running binary. Always check first (deploy=false), fix errors, then deploy.",
		},
		func(ctx tool.Context, args BuildArgs) (BuildRequestResult, error) {
			if args.Deploy {
				return requestBuild(args.Reason)
			}
			return requestCheck()
		},
	)
}

func requestCheck() (BuildRequestResult, error) {
	buildURL := os.Getenv("BUILD_SERVICE_URL")
	if buildURL == "" {
		buildURL = defaultBuildServiceURL
	}

	srcDir := os.Getenv("CLAY_ROOT")
	if srcDir == "" {
		srcDir = "/app"
	}
	srcDir = srcDir + "/src"

	tarball, err := createTarball(srcDir)
	if err != nil {
		return BuildRequestResult{
			Message: "Failed to create tarball",
			Output:  err.Error(),
		}, fmt.Errorf("tarball failed: %w", err)
	}

	client := &http.Client{Timeout: buildTimeout}
	resp, err := client.Post(buildURL+"/check", "application/gzip", bytes.NewReader(tarball))
	if err != nil {
		return BuildRequestResult{
			Message: "Build service unreachable",
			Output:  fmt.Sprintf("Error: %v\nURL: %s", err, buildURL),
		}, fmt.Errorf("build service unreachable: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Output  string `json:"output"`
		Error   string `json:"error"`
	}
	json.Unmarshal(body, &result)

	if !result.Success {
		return BuildRequestResult{
			Message: "Compilation failed — fix errors and check again",
			Output:  result.Output,
		}, fmt.Errorf("check failed: %s", result.Error)
	}

	return BuildRequestResult{
		Message: "All packages compile successfully. Safe to build_and_deploy.",
		Output:  result.Output,
	}, nil
}

func requestBuild(reason string) (BuildRequestResult, error) {
	buildURL := os.Getenv("BUILD_SERVICE_URL")
	if buildURL == "" {
		buildURL = defaultBuildServiceURL
	}

	srcDir := os.Getenv("CLAY_ROOT")
	if srcDir == "" {
		srcDir = "/app"
	}
	srcDir = srcDir + "/src"

	// 1. Tarball the source directory
	tarball, err := createTarball(srcDir)
	if err != nil {
		return BuildRequestResult{
			Message: "Failed to create tarball",
			Output:  err.Error(),
		}, fmt.Errorf("tarball failed: %w", err)
	}

	// 2. POST tarball to build service
	client := &http.Client{Timeout: buildTimeout}
	resp, err := client.Post(buildURL+"/build", "application/gzip", bytes.NewReader(tarball))
	if err != nil {
		return BuildRequestResult{
			Message: "Build service unreachable",
			Output:  fmt.Sprintf("Error: %v\nURL: %s", err, buildURL),
		}, fmt.Errorf("build service unreachable: %w", err)
	}
	defer resp.Body.Close()

	// 3. Check response type
	contentType := resp.Header.Get("Content-Type")

	if contentType == "application/octet-stream" {
		// Success — response body is the compiled binary
		outDir := os.Getenv("CLAY_ROOT")
		if outDir == "" {
			outDir = "/app"
		}
		outPath := outDir + "/builds/clay.new"

		os.MkdirAll(outDir+"/builds", 0755)
		f, err := os.Create(outPath)
		if err != nil {
			return BuildRequestResult{
				Message: "Build succeeded but failed to save binary",
				Output:  err.Error(),
			}, fmt.Errorf("save binary: %w", err)
		}

		n, err := io.Copy(f, resp.Body)
		f.Close()
		if err != nil {
			os.Remove(outPath)
			return BuildRequestResult{
				Message: "Build succeeded but failed to save binary",
				Output:  err.Error(),
			}, fmt.Errorf("save binary: %w", err)
		}

		os.Chmod(outPath, 0755)

		buildOutput := resp.Header.Get("X-Build-Output")
		return BuildRequestResult{
			Message: fmt.Sprintf("Build succeeded (%d bytes). Medic will hot-swap shortly.", n),
			Output:  buildOutput,
		}, nil
	}

	// Failure — response body is JSON with error details
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Output  string `json:"output"`
		Error   string `json:"error"`
	}
	json.Unmarshal(body, &result)

	errMsg := result.Error
	if errMsg == "" {
		errMsg = result.Output
	}
	return BuildRequestResult{
		Message: "Build failed — fix the errors and retry",
		Output:  errMsg,
	}, fmt.Errorf("build failed: %s", errMsg)
}

func createTarball(srcDir string) ([]byte, error) {
	cmd := exec.Command("tar", "-czf", "-", "-C", srcDir, ".")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%v: %s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}
