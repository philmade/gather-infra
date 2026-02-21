package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

// DockerTools registers claw management tools that use the Docker API.
type DockerTools struct {
	cli *client.Client
}

func NewDockerTools() (*DockerTools, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &DockerTools{cli: cli}, nil
}

// RegisterTools adds Docker-based tools to the registry.
func (d *DockerTools) RegisterTools(reg *Registry) {
	reg.Register(&Tool{
		ID:          "claw.list_all",
		Category:    "claw",
		Name:        "claw.list_all",
		Description: "List all active claw containers (names only)",
		Source:      "docker",
	})
	reg.Register(&Tool{
		ID:          "claw.logs",
		Category:    "claw",
		Name:        "claw.logs",
		Description: "Get recent container logs for a claw (default: own container)",
		Params: []ToolParam{
			{Name: "claw", Type: "string", Required: false, Description: "Claw name (default: self)"},
			{Name: "lines", Type: "integer", Required: false, Description: "Number of lines (default: 100)"},
		},
		Source: "docker",
	})
	reg.Register(&Tool{
		ID:          "claw.stats",
		Category:    "claw",
		Name:        "claw.stats",
		Description: "Get container resource stats (CPU, memory) for a claw",
		Params: []ToolParam{
			{Name: "claw", Type: "string", Required: false, Description: "Claw name (default: self)"},
		},
		Source: "docker",
	})
	reg.Register(&Tool{
		ID:          "peer.list",
		Category:    "peer",
		Name:        "peer.list",
		Description: "List active claw names and URLs for inter-claw communication",
		Source:      "docker",
	})
}

// Execute runs a Docker-based tool.
func (d *DockerTools) Execute(toolID string, params map[string]any) (any, error) {
	switch toolID {
	case "claw.list_all", "peer.list":
		return d.listClaws()
	case "claw.logs":
		return d.getClawLogs(params)
	case "claw.stats":
		return d.getClawStats(params)
	default:
		return nil, fmt.Errorf("unknown docker tool: %s", toolID)
	}
}

func (d *DockerTools) listClaws() (any, error) {
	ctx := context.Background()

	// List containers with claw- prefix
	containers, err := d.cli.ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("name", "claw-"),
			filters.Arg("status", "running"),
		),
	})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	type clawInfo struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}

	var claws []clawInfo
	for _, c := range containers {
		for _, name := range c.Names {
			name = strings.TrimPrefix(name, "/")
			if strings.HasPrefix(name, "claw-") {
				clawName := strings.TrimPrefix(name, "claw-")
				claws = append(claws, clawInfo{
					Name: clawName,
					URL:  fmt.Sprintf("http://%s:8080", name),
				})
			}
		}
	}

	return map[string]any{"claws": claws, "count": len(claws)}, nil
}

func (d *DockerTools) getClawLogs(params map[string]any) (any, error) {
	ctx := context.Background()

	clawName, _ := params["claw"].(string)
	if clawName == "" {
		clawName = "self" // placeholder — caller should set this
	}

	lines := "100"
	if l, ok := params["lines"]; ok {
		lines = fmt.Sprintf("%v", l)
	}

	containerName := "claw-" + clawName
	reader, err := d.cli.ContainerLogs(ctx, containerName, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       lines,
	})
	if err != nil {
		return nil, fmt.Errorf("get logs: %w", err)
	}
	defer reader.Close()

	logBytes, _ := io.ReadAll(reader)
	return map[string]any{"logs": string(logBytes)}, nil
}

func (d *DockerTools) getClawStats(params map[string]any) (any, error) {
	ctx := context.Background()

	clawName, _ := params["claw"].(string)
	if clawName == "" {
		clawName = "self"
	}

	containerName := "claw-" + clawName
	stats, err := d.cli.ContainerStatsOneShot(ctx, containerName)
	if err != nil {
		return nil, fmt.Errorf("get stats: %w", err)
	}
	defer stats.Body.Close()

	body, _ := io.ReadAll(stats.Body)
	return map[string]any{"stats": string(body)}, nil
}
