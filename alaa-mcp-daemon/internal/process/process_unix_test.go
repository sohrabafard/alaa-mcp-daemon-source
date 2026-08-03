//go:build !windows

package process

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestUnixStopTerminatesProcessGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	var output bytes.Buffer
	handle, err := NewLauncher().Start(Spec{
		Program: "/bin/sh", Args: []string{"-c", "sleep 30 & child=$!; echo $child > \"$PID_FILE\"; wait"},
		Cwd: t.TempDir(), Env: map[string]string{"PID_FILE": pidFile}, Priority: "normal", Stdout: &output, Stderr: &output,
	})
	if err != nil {
		t.Fatal(err)
	}
	var childPID int
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		data, readErr := os.ReadFile(pidFile)
		if readErr == nil {
			childPID, _ = strconv.Atoi(strings.TrimSpace(string(data)))
			if childPID > 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if childPID == 0 {
		_ = handle.Kill()
		t.Fatal("child pid was not written")
	}
	if err := handle.Stop(100 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	select {
	case <-handle.Done():
	case <-time.After(time.Second):
		t.Fatal("process did not stop")
	}
	for i := 0; i < 50; i++ {
		err := syscall.Kill(childPID, 0)
		if errors.Is(err, syscall.ESRCH) || isZombie(childPID) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("child process %d is still running", childPID)
}

func isZombie(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	fields := strings.Fields(string(data))
	return len(fields) > 2 && fields[2] == "Z"
}
