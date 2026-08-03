//go:build windows

package autostart

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildTaskXMLUsesCurrentUserLeastPrivilegeAndExactPaths(t *testing.T) {
	executable := `C:\Tools\Alaa MCP\alaa-mcp-daemon.exe`
	configPath := `C:\Tools\Alaa MCP\alaa-mcp-daemon.json`
	sid := "S-1-5-21-123-456-789-1001"
	data, err := buildTaskXML(executable, configPath, sid)
	if err != nil {
		t.Fatal(err)
	}
	var decoded taskXML
	if err := xml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("generated task XML is invalid: %v\n%s", err, data)
	}
	if decoded.Principals.Principal.UserID != sid || decoded.Principals.Principal.LogonType != "InteractiveToken" || decoded.Principals.Principal.RunLevel != "LeastPrivilege" {
		t.Fatalf("principal = %#v", decoded.Principals.Principal)
	}
	if decoded.Settings.MultipleInstancesPolicy != "IgnoreNew" || decoded.Settings.ExecutionTimeLimit != "PT0S" {
		t.Fatalf("settings = %#v", decoded.Settings)
	}
	if decoded.Actions.Exec.Command != executable || decoded.Actions.Exec.WorkingDirectory != filepath.Dir(executable) {
		t.Fatalf("action = %#v", decoded.Actions.Exec)
	}
	if !strings.Contains(decoded.Actions.Exec.Arguments, "run --config") || !strings.Contains(decoded.Actions.Exec.Arguments, configPath) {
		t.Fatalf("arguments = %q", decoded.Actions.Exec.Arguments)
	}
}

func TestTaskSchedulerInstallRemoveOptIn(t *testing.T) {
	if os.Getenv("ALAA_MCP_DAEMON_RUN_SCHEDULER_TESTS") != "1" {
		t.Skip("set ALAA_MCP_DAEMON_RUN_SCHEDULER_TESTS=1 to exercise Task Scheduler")
	}
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	taskName, err := Install(os.Args[0], configPath, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = Uninstall(configPath) }()
	if taskName != TaskName(configPath) {
		t.Fatalf("task name = %q", taskName)
	}
	if _, err := Uninstall(configPath); err != nil {
		t.Fatal(err)
	}
}
