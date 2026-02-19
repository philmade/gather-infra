package tools

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// SelfBuildTool handles self-compilation and restart
type SelfBuildTool struct {
	projectRoot string
}

// NewSelfBuildTool creates a new self-build tool
func NewSelfBuildTool() *SelfBuildTool {
	cwd, _ := os.Getwd()
	return &SelfBuildTool{projectRoot: cwd}
}

// Build compiles the agent, returns any errors
func (s *SelfBuildTool) Build() (string, error) {
	cmd := exec.Command("go", "build", "-o", "clawpoint-adk", ".")
	cmd.Dir = s.projectRoot
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// Reboot restarts the agent with the new binary
func (s *SelfBuildTool) Reboot() error {
	// Get the path to the new binary
	binaryPath := s.projectRoot + "/clawpoint-adk"
	
	// Start the new process
	cmd := exec.Command(binaryPath)
	cmd.Dir = s.projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	
	// Fork and exec - replace current process
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start new binary: %v", err)
	}
	
	// Exit current process
	os.Exit(0)
	
	return nil // Never reached
}
