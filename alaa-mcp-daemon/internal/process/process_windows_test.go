//go:build windows

package process

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	windowsHelperEnv  = "ALAA_MCP_DAEMON_WINDOWS_HELPER"
	windowsPIDFileEnv = "ALAA_MCP_DAEMON_WINDOWS_PID_FILE"
	synchronizeAccess = 0x00100000
)

func TestWindowsHelperProcess(t *testing.T) {
	mode := os.Getenv(windowsHelperEnv)
	if mode == "" {
		return
	}
	if mode == "parent" {
		command := exec.Command(os.Args[0], "-test.run=^TestWindowsHelperProcess$")
		command.Env = append(os.Environ(), windowsHelperEnv+"=child")
		if err := command.Start(); err != nil {
			os.Exit(91)
		}
		pidPath := os.Getenv(windowsPIDFileEnv)
		if err := os.WriteFile(pidPath, []byte(strconv.Itoa(command.Process.Pid)), 0o600); err != nil {
			_ = command.Process.Kill()
			os.Exit(92)
		}
		for {
			time.Sleep(time.Hour)
		}
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestWindowsJobObjectTerminatesDescendants(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	handle, err := NewLauncher().Start(Spec{
		Program: os.Args[0],
		Args:    []string{"-test.run=^TestWindowsHelperProcess$"},
		Cwd:     t.TempDir(),
		Env: map[string]string{
			windowsHelperEnv:  "parent",
			windowsPIDFileEnv: pidFile,
		},
		Priority: "normal",
		Stdout:   io.Discard,
		Stderr:   io.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	childPID := waitForPIDFile(t, pidFile)
	if err := handle.Stop(0); err != nil {
		t.Fatal(err)
	}
	select {
	case <-handle.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("managed root process did not exit")
	}
	waitForWindowsProcessExit(t, childPID)
}

func waitForPIDFile(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
			if parseErr == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("helper child PID was not written")
	return 0
}

func waitForWindowsProcessExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		handle, err := syscall.OpenProcess(synchronizeAccess, false, uint32(pid))
		if err != nil {
			if err == syscall.Errno(87) {
				return
			}
			t.Fatalf("OpenProcess(%d): %v", pid, err)
		}
		result, _, _ := procWaitForSingleObject.Call(uintptr(handle), 0)
		_ = syscall.CloseHandle(handle)
		if uint32(result) == waitObject0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("descendant process %d survived Job Object termination", pid)
}
