package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

const (
	defaultBuildServiceURL = "http://127.0.0.1:9090"
	buildTimeout           = 60 * time.Second
)

// NewBuildTools creates the build_and_deploy tool for the coordinator.
func NewBuildTools() ([]tool.Tool, error) {
	var out []tool.Tool

	t, err := functiontool.New(
		functiontool.Config{
			Name:        "build_and_deploy",
			Description: "Compile yourself via the external build service and deploy the new binary. Medic will handle the restart and rollback if the new binary crashes.",
		},
		func(ctx tool.Context, args BuildRequestArgs) (BuildRequestResult, error) {
			return requestBuild(args.Reason)
		},
	)
	if err != nil {
		return nil, err
	}
	out = append(out, t)

	return out, nil
}

func requestBuild(reason string) (BuildRequestResult, error) {
	buildURL := os.Getenv("BUILD_SERVICE_URL")
	if buildURL == "" {
		buildURL = defaultBuildServiceURL
	}

	payload, _ := json.Marshal(map[string]string{
		"reason": reason,
	})

	client := &http.Client{Timeout: buildTimeout}
	resp, err := client.Post(buildURL+"/build", "application/json", bytes.NewReader(payload))
	if err != nil {
		return BuildRequestResult{
			Message: "Build service unreachable",
			Output:  fmt.Sprintf("Error: %v", err),
		}, fmt.Errorf("build service unreachable: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Success bool   `json:"success"`
		Output  string `json:"output"`
		Error   string `json:"error"`
	}
	json.Unmarshal(body, &result)

	if resp.StatusCode != 200 || !result.Success {
		errMsg := result.Error
		if errMsg == "" {
			errMsg = result.Output
		}
		return BuildRequestResult{
			Message: "Build failed",
			Output:  errMsg,
		}, fmt.Errorf("build failed: %s", errMsg)
	}

	return BuildRequestResult{
		Message: "Build succeeded. Medic will restart with the new binary shortly.",
		Output:  result.Output,
	}, nil
}
