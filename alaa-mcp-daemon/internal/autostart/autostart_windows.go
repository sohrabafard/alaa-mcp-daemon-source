//go:build windows

package autostart

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"alaa-mcp-daemon/internal/winapi"
)

func Install(executable, configPath, stateDir string) (string, error) {
	executable, err := filepath.Abs(executable)
	if err != nil {
		return "", err
	}
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return "", err
	}
	stateDir, err = filepath.Abs(stateDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return "", fmt.Errorf("create state directory: %w", err)
	}
	sid, err := winapi.CurrentUserSID()
	if err != nil {
		return "", err
	}
	taskName := TaskName(configPath)
	xmlPath := filepath.Join(stateDir, "autostart-"+taskKey(configPath)+".xml")
	xmlData, err := buildTaskXML(executable, configPath, sid)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(xmlPath, xmlData, 0o600); err != nil {
		return "", fmt.Errorf("write task XML: %w", err)
	}
	cmd := exec.Command("schtasks.exe", "/Create", "/TN", taskName, "/XML", xmlPath, "/F")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("register scheduled task: %w: %s", err, bytes.TrimSpace(output))
	}
	return taskName, nil
}

func buildTaskXML(executable, configPath, sid string) ([]byte, error) {
	arguments := "run --config " + syscall.EscapeArg(configPath)
	doc := taskXML{
		Version: "1.4", Namespace: "http://schemas.microsoft.com/windows/2004/02/mit/task",
		RegistrationInfo: registrationInfo{Description: "Keeps local MCP server processes supervised for the current user."},
		Triggers:         triggers{Logon: logonTrigger{Enabled: true, UserID: sid}},
		Principals:       principals{Principal: principal{ID: "Author", UserID: sid, LogonType: "InteractiveToken", RunLevel: "LeastPrivilege"}},
		Settings: settings{
			MultipleInstancesPolicy: "IgnoreNew", DisallowStartIfOnBatteries: false, StopIfGoingOnBatteries: false,
			AllowHardTerminate: true, StartWhenAvailable: true, RunOnlyIfNetworkAvailable: false,
			AllowStartOnDemand: true, Enabled: true, Hidden: false, RunOnlyIfIdle: false,
			WakeToRun: false, ExecutionTimeLimit: "PT0S", Priority: 7,
			RestartOnFailure: restartOnFailure{Interval: "PT1M", Count: 3},
		},
		Actions: actions{Context: "Author", Exec: execAction{Command: executable, Arguments: arguments, WorkingDirectory: filepath.Dir(executable)}},
	}
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	encoder := xml.NewEncoder(&buf)
	encoder.Indent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		return nil, fmt.Errorf("encode task XML: %w", err)
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

func Uninstall(configPath string) (string, error) {
	configPath, err := filepath.Abs(configPath)
	if err != nil {
		return "", err
	}
	taskName := TaskName(configPath)
	cmd := exec.Command("schtasks.exe", "/Delete", "/TN", taskName, "/F")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("remove scheduled task: %w: %s", err, bytes.TrimSpace(output))
	}
	return taskName, nil
}

type taskXML struct {
	XMLName          xml.Name         `xml:"Task"`
	Version          string           `xml:"version,attr"`
	Namespace        string           `xml:"xmlns,attr"`
	RegistrationInfo registrationInfo `xml:"RegistrationInfo"`
	Triggers         triggers         `xml:"Triggers"`
	Principals       principals       `xml:"Principals"`
	Settings         settings         `xml:"Settings"`
	Actions          actions          `xml:"Actions"`
}
type registrationInfo struct {
	Description string `xml:"Description"`
}
type triggers struct {
	Logon logonTrigger `xml:"LogonTrigger"`
}
type logonTrigger struct {
	Enabled bool   `xml:"Enabled"`
	UserID  string `xml:"UserId"`
}
type principals struct {
	Principal principal `xml:"Principal"`
}
type principal struct {
	ID        string `xml:"id,attr"`
	UserID    string `xml:"UserId"`
	LogonType string `xml:"LogonType"`
	RunLevel  string `xml:"RunLevel"`
}
type settings struct {
	MultipleInstancesPolicy    string           `xml:"MultipleInstancesPolicy"`
	DisallowStartIfOnBatteries bool             `xml:"DisallowStartIfOnBatteries"`
	StopIfGoingOnBatteries     bool             `xml:"StopIfGoingOnBatteries"`
	AllowHardTerminate         bool             `xml:"AllowHardTerminate"`
	StartWhenAvailable         bool             `xml:"StartWhenAvailable"`
	RunOnlyIfNetworkAvailable  bool             `xml:"RunOnlyIfNetworkAvailable"`
	AllowStartOnDemand         bool             `xml:"AllowStartOnDemand"`
	Enabled                    bool             `xml:"Enabled"`
	Hidden                     bool             `xml:"Hidden"`
	RunOnlyIfIdle              bool             `xml:"RunOnlyIfIdle"`
	WakeToRun                  bool             `xml:"WakeToRun"`
	ExecutionTimeLimit         string           `xml:"ExecutionTimeLimit"`
	Priority                   int              `xml:"Priority"`
	RestartOnFailure           restartOnFailure `xml:"RestartOnFailure"`
}
type restartOnFailure struct {
	Interval string `xml:"Interval"`
	Count    int    `xml:"Count"`
}
type actions struct {
	Context string     `xml:"Context,attr"`
	Exec    execAction `xml:"Exec"`
}
type execAction struct {
	Command          string `xml:"Command"`
	Arguments        string `xml:"Arguments"`
	WorkingDirectory string `xml:"WorkingDirectory"`
}
