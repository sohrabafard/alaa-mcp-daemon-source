//go:build !windows

package process

import (
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

func (DefaultLauncher) Start(spec Spec) (Handle, error) {
	cmd := exec.Command(spec.Program, spec.Args...)
	cmd.Dir = spec.Cwd
	cmd.Env = mergedEnv(spec.Env)
	cmd.Stdout = spec.Stdout
	cmd.Stderr = spec.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start process: %w", err)
	}
	h := &unixHandle{cmd: cmd, pid: cmd.Process.Pid, done: make(chan struct{})}
	go h.wait()
	return h, nil
}

type unixHandle struct {
	cmd    *exec.Cmd
	pid    int
	done   chan struct{}
	mu     sync.RWMutex
	result Result
}

func (h *unixHandle) PID() int              { return h.pid }
func (h *unixHandle) Done() <-chan struct{} { return h.done }
func (h *unixHandle) Result() Result        { h.mu.RLock(); defer h.mu.RUnlock(); return h.result }

func (h *unixHandle) wait() {
	err := h.cmd.Wait()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	h.mu.Lock()
	h.result = Result{ExitCode: exitCode, Err: err, ExitedAt: time.Now().UTC()}
	h.mu.Unlock()
	close(h.done)
}

func (h *unixHandle) Stop(grace time.Duration) error {
	select {
	case <-h.done:
		return nil
	default:
	}
	_ = syscall.Kill(-h.pid, syscall.SIGTERM)
	if grace > 0 {
		timer := time.NewTimer(grace)
		defer timer.Stop()
		select {
		case <-h.done:
			return nil
		case <-timer.C:
		}
	}
	return h.Kill()
}

func (h *unixHandle) Kill() error {
	select {
	case <-h.done:
		return nil
	default:
	}
	if err := syscall.Kill(-h.pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("kill process group: %w", err)
	}
	select {
	case <-h.done:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("process did not exit after SIGKILL")
	}
}
