package tools

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FSTool handles filesystem operations
type FSTool struct {
	projectRoot string
}

// NewFSTool creates a new filesystem tool
func NewFSTool() *FSTool {
	cwd, _ := os.Getwd()
	return &FSTool{projectRoot: cwd}
}

// Read reads a file or lists a directory
func (f *FSTool) Read(path string) (string, error) {
	fullPath := filepath.Join(f.projectRoot, path)
	
	info, err := os.Stat(fullPath)
	if err != nil {
		return "", err
	}
	
	if info.IsDir() {
		// List directory
		files, err := ioutil.ReadDir(fullPath)
		if err != nil {
			return "", err
		}
		
		var lines []string
		for _, file := range files {
			prefix := "[d] "
			if !file.IsDir() {
				prefix = "[f] "
			}
			lines = append(lines, prefix+file.Name())
		}
		return strings.Join(lines, "\n"), nil
	}
	
	// Read file
	content, err := ioutil.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	
	return string(content), nil
}

// Write creates or overwrites a file
func (f *FSTool) Write(path, content string) error {
	fullPath := filepath.Join(f.projectRoot, path)
	
	// Create parent directories if needed
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}
	
	return ioutil.WriteFile(fullPath, []byte(content), 0644)
}

// Edit finds and replaces text in a file
func (f *FSTool) Edit(path, oldText, newText string) error {
	fullPath := filepath.Join(f.projectRoot, path)
	
	content, err := ioutil.ReadFile(fullPath)
	if err != nil {
		return err
	}
	
	str := string(content)
	if !strings.Contains(str, oldText) {
		return fmt.Errorf("text not found in file")
	}
	
	newContent := strings.Replace(str, oldText, newText, 1)
	return ioutil.WriteFile(fullPath, []byte(newContent), 0644)
}

// Bash runs a shell command
func (f *FSTool) Bash(command string) (string, error) {
	cmd := exec.Command("bash", "-c", command)
	cmd.Dir = f.projectRoot
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	
	return string(output), nil
}

// Search finds files matching a pattern
func (f *FSTool) Search(pattern string) ([]string, error) {
	var matches []string
	
	err := filepath.Walk(f.projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		matched, err := filepath.Match(pattern, filepath.Base(path))
		if err != nil {
			return err
		}
		
		if matched {
			rel, _ := filepath.Rel(f.projectRoot, path)
			matches = append(matches, rel)
		}
		
		return nil
	})
	
	return matches, err
}
