package tools

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.starlark.net/starlark"
)

const defaultExtensionsDir = "/app/data/extensions"

// StarlarkRunner executes .star extension scripts with sandboxed builtins.
type StarlarkRunner struct {
	dir string
}

// NewStarlarkRunner creates a runner scanning the extensions directory.
func NewStarlarkRunner() *StarlarkRunner {
	dir := os.Getenv("EXTENSIONS_DIR")
	if dir == "" {
		dir = defaultExtensionsDir
	}
	os.MkdirAll(dir, 0755)
	return &StarlarkRunner{dir: dir}
}

// List returns available .star scripts with their descriptions.
func (s *StarlarkRunner) List() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var scripts []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".star") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			scripts = append(scripts, e.Name()+" — (unreadable)")
			continue
		}
		desc := extractStarlarkMeta(string(content))
		scripts = append(scripts, e.Name()+" — "+desc)
	}
	return scripts, nil
}

func extractStarlarkMeta(content string) string {
	for _, line := range strings.SplitN(content, "\n", 10) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# DESCRIPTION:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# DESCRIPTION:"))
		}
	}
	return "(no description)"
}

// Run executes a .star script by name with the given arguments.
func (s *StarlarkRunner) Run(name string, args map[string]string) (string, error) {
	if !strings.HasSuffix(name, ".star") {
		name = name + ".star"
	}
	path := filepath.Join(s.dir, name)

	// Prevent path traversal
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid extension name")
	}
	dirAbs, _ := filepath.Abs(s.dir)
	if !strings.HasPrefix(abs, dirAbs+"/") {
		return "", fmt.Errorf("invalid extension name: path traversal blocked")
	}

	src, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("extension %q not found: %v", name, err)
	}

	var output strings.Builder
	predeclared := starlarkBuiltins(&output)

	thread := &starlark.Thread{
		Name: "ext-" + name,
		Print: func(_ *starlark.Thread, msg string) {
			output.WriteString(msg + "\n")
		},
	}

	globals, err := starlark.ExecFile(thread, path, src, predeclared)
	if err != nil {
		return "", fmt.Errorf("starlark error: %v", err)
	}

	// Call run(args) if defined
	runFn, ok := globals["run"]
	if !ok {
		if output.Len() > 0 {
			return output.String(), nil
		}
		return "Script executed (no run() function defined)", nil
	}

	argsDict := starlark.NewDict(len(args))
	for k, v := range args {
		argsDict.SetKey(starlark.String(k), starlark.String(v))
	}

	result, err := starlark.Call(thread, runFn, starlark.Tuple{argsDict}, nil)
	if err != nil {
		return "", fmt.Errorf("run() error: %v", err)
	}

	prefix := output.String()
	if str, ok := result.(starlark.String); ok {
		return prefix + string(str), nil
	}
	return prefix + result.String(), nil
}

func starlarkBuiltins(output *strings.Builder) starlark.StringDict {
	return starlark.StringDict{
		"http_get": starlark.NewBuiltin("http_get", builtinHTTPGet),
		"http_post": starlark.NewBuiltin("http_post", builtinHTTPPost),
		"read_file": starlark.NewBuiltin("read_file", builtinReadFile),
		"write_file": starlark.NewBuiltin("write_file", builtinWriteFile),
		"log": starlark.NewBuiltin("log", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			parts := make([]string, len(args))
			for i, a := range args {
				parts[i] = a.String()
			}
			output.WriteString(strings.Join(parts, " ") + "\n")
			return starlark.None, nil
		}),
	}
}

func builtinHTTPGet(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url string
	if err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &url); err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return starlark.String("ERROR: " + err.Error()), nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	return starlark.String(string(body)), nil
}

func builtinHTTPPost(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url, body string
	contentType := "application/json"
	if err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 2, &url, &body, &contentType); err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(url, contentType, strings.NewReader(body))
	if err != nil {
		return starlark.String("ERROR: " + err.Error()), nil
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	return starlark.String(string(respBody)), nil
}

func builtinReadFile(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return starlark.String("ERROR: " + err.Error()), nil
	}
	return starlark.String(string(data)), nil
}

func builtinWriteFile(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path, content string
	if err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 2, &path, &content); err != nil {
		return nil, err
	}
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return starlark.String("ERROR: " + err.Error()), nil
	}
	return starlark.String(fmt.Sprintf("Wrote %d bytes to %s", len(content), path)), nil
}
