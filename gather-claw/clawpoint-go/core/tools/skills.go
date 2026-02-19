package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SkillTool handles dynamic skill loading and execution
type SkillTool struct {
	skillsDir string
}

// NewSkillTool creates a new skill tool
func NewSkillTool() *SkillTool {
	return &SkillTool{
		skillsDir: "../skills",
	}
}

// Find lists available skills, optionally filtered by query
func (s *SkillTool) Find(query string) ([]string, error) {
	var skills []string

	entries, err := os.ReadDir(s.skillsDir)
	if err != nil {
		os.MkdirAll(s.skillsDir, 0755)
		return skills, nil
	}

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), ".sh") {
			if query == "" || strings.Contains(strings.ToLower(entry.Name()), strings.ToLower(query)) {
				skills = append(skills, entry.Name())
			}
		}
	}

	return skills, nil
}

// Run executes a skill
func (s *SkillTool) Run(skillName string, args map[string]interface{}) (string, error) {
	skillPath := filepath.Join(s.skillsDir, skillName)

	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		return "", fmt.Errorf("skill not found: %s", skillName)
	}

	if strings.HasSuffix(skillName, ".sh") {
		return s.runShellSkill(skillPath, args)
	} else if strings.HasSuffix(skillName, ".go") {
		return "", fmt.Errorf("Go skills not yet implemented")
	}

	return "", fmt.Errorf("unsupported skill type: %s", skillName)
}

func (s *SkillTool) runShellSkill(skillPath string, args map[string]interface{}) (string, error) {
	env := os.Environ()
	for k, v := range args {
		env = append(env, fmt.Sprintf("SKILL_ARG_%s=%v", strings.ToUpper(k), v))
	}

	cmd := exec.Command("bash", skillPath)
	cmd.Env = env
	cmd.Dir = s.skillsDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}

	return string(output), nil
}
