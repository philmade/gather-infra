package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SoulTool manages soul identity files.
type SoulTool struct {
	soulDir string
	allowed map[string]bool
}

// NewSoulTool creates a soul tool pointing at the soul directory.
func NewSoulTool() *SoulTool {
	// Use CLAWPOINT_ROOT env var if set (for container deployment)
	soulDir := os.Getenv("CLAWPOINT_ROOT")
	if soulDir == "" {
		// Fallback: soul dir is at ../soul relative to clawpoint-go
		dir, _ := os.Getwd()
		soulDir = filepath.Join(dir, "..", "soul")
	} else {
		soulDir = filepath.Join(soulDir, "soul")
	}
	return &SoulTool{
		soulDir: soulDir,
		allowed: map[string]bool{
			"SOUL.md":      true,
			"IDENTITY.md":  true,
			"USER.md":      true,
			"HEARTBEAT.md": true,
			"BOOTSTRAP.md": true,
		},
	}
}

// Read reads a soul file.
func (s *SoulTool) Read(filename string) (string, error) {
	if !s.allowed[filename] {
		return "", fmt.Errorf("invalid filename %q, valid: %s", filename, s.validFiles())
	}
	data, err := os.ReadFile(filepath.Join(s.soulDir, filename))
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%s does not exist yet", filename)
		}
		return "", err
	}
	return string(data), nil
}

// Write updates a soul file (BOOTSTRAP.md is read-only).
func (s *SoulTool) Write(filename, content string) error {
	if filename == "BOOTSTRAP.md" {
		return fmt.Errorf("BOOTSTRAP.md is read-only")
	}
	if !s.allowed[filename] {
		return fmt.Errorf("invalid filename %q, valid: %s", filename, s.validFiles())
	}
	return os.WriteFile(filepath.Join(s.soulDir, filename), []byte(content), 0644)
}

func (s *SoulTool) validFiles() string {
	var names []string
	for k := range s.allowed {
		names = append(names, k)
	}
	return strings.Join(names, ", ")
}

// LoadSection reads a soul file and returns it as a markdown section.
func (s *SoulTool) LoadSection(filename string) string {
	content, err := s.Read(filename)
	if err != nil {
		return ""
	}
	name := strings.TrimSuffix(filename, ".md")
	return fmt.Sprintf("## %s\n\n%s\n", name, strings.TrimSpace(content))
}
