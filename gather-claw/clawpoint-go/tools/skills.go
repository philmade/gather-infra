package tools

import (
	"fmt"
	"io/ioutil"
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

// Skill represents a loaded skill
func (s *SkillTool) Find(query string) ([]string, error) {
	var skills []string
	
	files, err := ioutil.ReadDir(s.skillsDir)
	if err != nil {
		// Try creating the directory
		os.MkdirAll(s.skillsDir, 0755)
		return skills, nil
	}
	
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".go") || strings.HasSuffix(file.Name(), ".sh") {
			if query == "" || strings.Contains(strings.ToLower(file.Name()), strings.ToLower(query)) {
				skills = append(skills, file.Name())
			}
		}
	}
	
	return skills, nil
}

// Run executes a skill
func (s *SkillTool) Run(skillName string, args map[string]interface{}) (string, error) {
	skillPath := filepath.Join(s.skillsDir, skillName)
	
	// Check if skill exists
	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		return "", fmt.Errorf("skill not found: %s", skillName)
	}
	
	// Execute based on file type
	if strings.HasSuffix(skillName, ".sh") {
		return s.runShellSkill(skillPath, args)
	} else if strings.HasSuffix(skillName, ".go") {
		return s.runGoSkill(skillPath, args)
	}
	
	return "", fmt.Errorf("unsupported skill type: %s", skillName)
}

func (s *SkillTool) runShellSkill(skillPath string, args map[string]interface{}) (string, error) {
	// Build args as environment variables
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

func (s *SkillTool) runGoSkill(skillPath string, args map[string]interface{}) (string, error) {
	// For now, just compile and run
	// TODO: Build skills as plugins or separate binaries
	return "", fmt.Errorf("Go skills not yet implemented")
}
