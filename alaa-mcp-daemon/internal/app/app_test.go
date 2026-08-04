package app

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateCommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := `{"version":1,"runtime":{"log_dir":"./logs","state_dir":"./state","max_parallel_starts":1},"commands":{"noop":{"program":"echo"}},"services":[]}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"validate", "--config", path, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"valid": true`) {
		t.Fatalf("stdout=%s", stdout.String())
	}
}

func TestUnknownCommandFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"unknown"}, &stdout, &stderr); code == 0 {
		t.Fatal("expected failure")
	}
}

func TestValidateUsesExecutableAdjacentDefaultConfig(t *testing.T) {
	executable := copyTestExecutable(t)
	adjacentConfig := filepath.Join(filepath.Dir(executable), "alaa-mcp-daemon.json")
	writeTestConfig(t, adjacentConfig, "adjacent")

	result := runValidateProcess(t, executable, t.TempDir())
	if !result.Valid {
		t.Fatalf("result=%#v", result)
	}
	if result.Path != adjacentConfig {
		t.Fatalf("config path=%q, want executable-adjacent %q", result.Path, adjacentConfig)
	}
}

func TestValidateExplicitConfigOverridesExecutableAdjacentDefault(t *testing.T) {
	executable := copyTestExecutable(t)
	adjacentConfig := filepath.Join(filepath.Dir(executable), "alaa-mcp-daemon.json")
	writeTestConfig(t, adjacentConfig, "adjacent")
	explicitConfig := filepath.Join(t.TempDir(), "explicit.json")
	writeTestConfig(t, explicitConfig, "explicit")

	result := runValidateProcess(t, executable, t.TempDir(), "--config", explicitConfig)
	if !result.Valid {
		t.Fatalf("result=%#v", result)
	}
	if result.Path != explicitConfig {
		t.Fatalf("config path=%q, want explicit %q", result.Path, explicitConfig)
	}
}

func TestValidateConfigHelperProcess(t *testing.T) {
	if os.Getenv("ALAA_MCP_DAEMON_TEST_HELPER") != "1" {
		return
	}
	separator := -1
	for index, arg := range os.Args {
		if arg == "--" {
			separator = index
			break
		}
	}
	if separator == -1 {
		os.Exit(2)
	}
	os.Exit(Run(os.Args[separator+1:], os.Stdout, os.Stderr))
}

type validateResult struct {
	Valid bool   `json:"valid"`
	Path  string `json:"path"`
}

func copyTestExecutable(t *testing.T) string {
	t.Helper()
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "alaa-mcp-daemon-test"+filepath.Ext(source))
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, data, 0o700); err != nil {
		t.Fatal(err)
	}
	return destination
}

func writeTestConfig(t *testing.T, path, label string) {
	t.Helper()
	body := `{"version":1,"runtime":{"log_dir":"./logs-` + label + `","state_dir":"./state-` + label + `","max_parallel_starts":1},"commands":{"noop":{"program":"echo"}},"services":[]}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runValidateProcess(t *testing.T, executable, workingDirectory string, args ...string) validateResult {
	t.Helper()
	commandArgs := []string{"-test.run=^TestValidateConfigHelperProcess$", "--", "validate", "--json"}
	commandArgs = append(commandArgs, args...)
	command := exec.Command(executable, commandArgs...)
	command.Dir = workingDirectory
	command.Env = append(os.Environ(), "ALAA_MCP_DAEMON_TEST_HELPER=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("validate process: %v\noutput: %s", err, output)
	}
	var result validateResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode validate output: %v\noutput: %s", err, output)
	}
	return result
}
