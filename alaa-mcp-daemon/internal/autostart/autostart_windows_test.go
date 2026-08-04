//go:build windows

package autostart

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestBuildTaskXML_UsesUTF16LEBOMAndMatchingDeclaration(t *testing.T) {
	executable := `C:\ابزار\آلاء MCP\alaa-mcp-daemon.exe`
	configPath := `C:\ابزار\آلاء MCP\config.json`
	data, err := buildTaskXML(executable, configPath, "S-1-5-21-123-456-789-1001")
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodeUTF16LETaskXML(t, data)
	if !strings.HasPrefix(decoded, `<?xml version="1.0" encoding="UTF-16"?>`) {
		t.Fatalf("XML declaration = %q", strings.SplitN(decoded, "\n", 2)[0])
	}
	if !strings.Contains(decoded, executable) || !strings.Contains(decoded, configPath) {
		t.Fatalf("Unicode paths did not round-trip: %s", decoded)
	}
}

func TestBuildTaskXMLUsesCurrentUserLeastPrivilegeAndExactPaths(t *testing.T) {
	executable := `C:\Tools\Alaa MCP\alaa-mcp-daemon.exe`
	configPath := `C:\Tools\Alaa MCP\alaa-mcp-daemon.json`
	sid := "S-1-5-21-123-456-789-1001"
	data, err := buildTaskXML(executable, configPath, sid)
	if err != nil {
		t.Fatal(err)
	}
	decodedXML := decodeUTF16LETaskXML(t, data)
	_, xmlBody, found := strings.Cut(decodedXML, "\n")
	if !found {
		t.Fatal("generated task XML is missing a document body")
	}
	var decoded taskXML
	if err := xml.Unmarshal([]byte(xmlBody), &decoded); err != nil {
		t.Fatalf("generated task XML is invalid: %v\n%s", err, decodedXML)
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

func decodeUTF16LETaskXML(t *testing.T, data []byte) string {
	t.Helper()
	if !bytes.HasPrefix(data, []byte{0xff, 0xfe}) {
		t.Fatalf("task XML does not start with a UTF-16LE BOM: % x", data[:min(8, len(data))])
	}
	if len(data)%2 != 0 {
		t.Fatalf("task XML length = %d, want even UTF-16LE byte count", len(data))
	}

	units := make([]uint16, (len(data)-2)/2)
	for i := range units {
		units[i] = uint16(data[2+i*2]) | uint16(data[3+i*2])<<8
	}
	return string(utf16.Decode(units))
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
