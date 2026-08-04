package supervisor

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"alaa-mcp-daemon/internal/config"
	"alaa-mcp-daemon/internal/model"
	"alaa-mcp-daemon/internal/process"
)

type fakeLauncher struct {
	starts    atomic.Int32
	mu        sync.Mutex
	handles   []*fakeHandle
	autoCrash bool
}

func (f *fakeLauncher) Start(process.Spec) (process.Handle, error) {
	f.starts.Add(1)
	h := newFakeHandle(int(f.starts.Load()) + 1000)
	f.mu.Lock()
	f.handles = append(f.handles, h)
	f.mu.Unlock()
	if f.autoCrash {
		go func() { time.Sleep(15 * time.Millisecond); h.exit(1, errors.New("crash")) }()
	}
	return h, nil
}

type fakeHandle struct {
	pid    int
	done   chan struct{}
	once   sync.Once
	mu     sync.RWMutex
	result process.Result
}

func newFakeHandle(pid int) *fakeHandle        { return &fakeHandle{pid: pid, done: make(chan struct{})} }
func (h *fakeHandle) PID() int                 { return h.pid }
func (h *fakeHandle) Done() <-chan struct{}    { return h.done }
func (h *fakeHandle) Result() process.Result   { h.mu.RLock(); defer h.mu.RUnlock(); return h.result }
func (h *fakeHandle) Stop(time.Duration) error { h.exit(0, nil); return nil }
func (h *fakeHandle) Kill() error              { h.exit(1, errors.New("killed")); return nil }
func (h *fakeHandle) exit(code int, err error) {
	h.once.Do(func() {
		h.mu.Lock()
		h.result = process.Result{ExitCode: code, Err: err, ExitedAt: time.Now().UTC()}
		h.mu.Unlock()
		close(h.done)
	})
}

func TestManualStopHoldSurvivesApply(t *testing.T) {
	launcher := &fakeLauncher{}
	sup := newTestSupervisor(t, launcher, baseService(t))
	defer shutdownTestSupervisor(t, sup)
	waitState(t, sup, model.StateReady)
	if err := sup.Stop(); err != nil {
		t.Fatal(err)
	}
	status := waitState(t, sup, model.StateStopped)
	if !status.ManualHold {
		t.Fatal("manual hold was not set")
	}
	spec := baseService(t)
	spec.Description = "changed"
	if err := sup.Apply(spec, ApplyMetadata, 2, "hash-2"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	if got := launcher.starts.Load(); got != 1 {
		t.Fatalf("service restarted during manual hold; starts=%d", got)
	}
	if err := sup.Start(); err != nil {
		t.Fatal(err)
	}
	waitState(t, sup, model.StateReady)
	if got := launcher.starts.Load(); got != 2 {
		t.Fatalf("starts=%d", got)
	}
}

func TestCrashLoopExhaustsRetryBudget(t *testing.T) {
	launcher := &fakeLauncher{autoCrash: true}
	spec := baseService(t)
	spec.Restart = config.EffectiveRestartPolicy{Policy: "always", MaxAttempts: 3, Window: time.Second, BackoffInitial: time.Millisecond, BackoffMax: 2 * time.Millisecond}
	sup := newTestSupervisor(t, launcher, spec)
	defer shutdownTestSupervisor(t, sup)
	status := waitState(t, sup, model.StateFailed)
	if status.FailuresInWindow != 3 {
		t.Fatalf("failures=%d", status.FailuresInWindow)
	}
	if got := launcher.starts.Load(); got != 3 {
		t.Fatalf("starts=%d", got)
	}
	if status.LastError == "" {
		t.Fatal("expected retry budget error")
	}
}

func TestPermanentForeignPortExhaustsRetryBudgetWithoutLaunching(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	launcher := &fakeLauncher{}
	spec := baseService(t)
	spec.Claims = []config.TCPClaim{{Host: "127.0.0.1", Port: port}}
	spec.Restart = config.EffectiveRestartPolicy{
		Policy:         "always",
		MaxAttempts:    3,
		Window:         time.Second,
		BackoffInitial: time.Millisecond,
		BackoffMax:     2 * time.Millisecond,
	}
	sup := newTestSupervisor(t, launcher, spec)
	defer shutdownTestSupervisor(t, sup)
	status := waitState(t, sup, model.StateFailed)
	if got := launcher.starts.Load(); got != 0 {
		t.Fatalf("launcher called %d times", got)
	}
	if status.FailuresInWindow != 3 {
		t.Fatalf("preflight failures=%d want=3", status.FailuresInWindow)
	}
	if !strings.Contains(status.LastError, "claimed endpoint") {
		t.Fatalf("missing occupied-endpoint cause: %s", status.LastError)
	}
	conn, err := net.DialTimeout("tcp", listener.Addr().String(), 100*time.Millisecond)
	if err != nil {
		t.Fatalf("foreign listener was disturbed: %v", err)
	}
	_ = conn.Close()
}

func TestMissingWorkingDirectoryRemainsBlockedWithoutRetry(t *testing.T) {
	launcher := &fakeLauncher{}
	spec := baseService(t)
	spec.Cwd = filepath.Join(t.TempDir(), "missing")
	sup := newTestSupervisor(t, launcher, spec)
	defer shutdownTestSupervisor(t, sup)

	status := waitState(t, sup, model.StateBlocked)
	if got := launcher.starts.Load(); got != 0 {
		t.Fatalf("launcher called for a missing working directory; starts=%d", got)
	}
	if status.FailuresInWindow != 0 || status.RestartCount != 0 || status.BackoffUntil != nil {
		t.Fatalf("deterministic preflight failure was retried: %#v", status)
	}
	if !strings.Contains(status.LastError, "working directory") {
		t.Fatalf("missing working-directory cause: %s", status.LastError)
	}
}

func TestTransientForeignPortIsRetriedAfterRelease(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	launcher := &fakeLauncher{}
	spec := baseService(t)
	spec.Claims = []config.TCPClaim{{Host: "127.0.0.1", Port: port}}
	spec.Restart = config.EffectiveRestartPolicy{
		Policy:         "always",
		MaxAttempts:    5,
		Window:         time.Second,
		BackoffInitial: 5 * time.Millisecond,
		BackoffMax:     20 * time.Millisecond,
	}
	sup := newTestSupervisor(t, launcher, spec)
	defer shutdownTestSupervisor(t, sup)

	preflightFailure := waitFor(t, sup, func(status model.ServiceStatus) bool {
		return strings.Contains(status.LastError, "claimed endpoint")
	})
	if got := launcher.starts.Load(); got != 0 {
		_ = listener.Close()
		t.Fatalf("launcher called while the claimed port was occupied; starts=%d status=%#v", got, preflightFailure)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	ready := waitFor(t, sup, func(status model.ServiceStatus) bool {
		return status.State == model.StateReady && launcher.starts.Load() == 1
	})
	if ready.LastError != "" {
		t.Fatalf("transient claim error survived successful recovery: %s", ready.LastError)
	}
	if got := launcher.starts.Load(); got != 1 {
		t.Fatalf("transient claim recovery launched %d processes; want exactly 1", got)
	}
}

func TestProcessApplyRestartsWithoutConsumingFailureBudget(t *testing.T) {
	launcher := &fakeLauncher{}
	spec := baseService(t)
	sup := newTestSupervisor(t, launcher, spec)
	defer shutdownTestSupervisor(t, sup)
	waitState(t, sup, model.StateReady)
	spec.Args = []string{"changed"}
	spec.ProcessHash = "changed"
	if err := sup.Apply(spec, ApplyProcess, 2, "hash-2"); err != nil {
		t.Fatal(err)
	}
	status := waitFor(t, sup, func(s model.ServiceStatus) bool { return s.State == model.StateReady && launcher.starts.Load() == 2 })
	if status.FailuresInWindow != 0 {
		t.Fatalf("intentional restart consumed budget: %d", status.FailuresInWindow)
	}
}

func TestProcessApplyWaitsForOldClaimReleaseBeforeRestart(t *testing.T) {
	launcher := &fakeLauncher{}
	oldListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	oldPort := oldListener.Addr().(*net.TCPAddr).Port
	if err := oldListener.Close(); err != nil {
		t.Fatal(err)
	}
	newListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	newPort := newListener.Addr().(*net.TCPAddr).Port
	_ = newListener.Close()

	spec := baseService(t)
	spec.Claims = []config.TCPClaim{{Host: "127.0.0.1", Port: oldPort}}
	sup := newTestSupervisor(t, launcher, spec)
	defer shutdownTestSupervisor(t, sup)
	waitState(t, sup, model.StateReady)

	// Simulate the old process still owning its declared endpoint after the root
	// process has exited. A replacement must not launch until this old claim is free.
	oldListener, err = net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(oldPort)))
	if err != nil {
		t.Fatal(err)
	}
	next := spec
	next.Claims = []config.TCPClaim{{Host: "127.0.0.1", Port: newPort}}
	next.ProcessHash = "new-process"
	if err := sup.Apply(next, ApplyProcess, 2, "hash-2"); err != nil {
		_ = oldListener.Close()
		t.Fatal(err)
	}
	waitState(t, sup, model.StateStopping)
	time.Sleep(120 * time.Millisecond)
	if got := launcher.starts.Load(); got != 1 {
		_ = oldListener.Close()
		t.Fatalf("replacement launched before old claim was released; starts=%d", got)
	}
	if err := oldListener.Close(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, sup, func(status model.ServiceStatus) bool {
		return status.State == model.StateReady && launcher.starts.Load() == 2
	})
}

func TestLimiterBoundsConcurrency(t *testing.T) {
	limiter := NewLimiter(1)
	release, err := limiter.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan struct{})
	go func() { second, _ := limiter.Acquire(context.Background()); close(acquired); second() }()
	select {
	case <-acquired:
		t.Fatal("second acquire should block")
	case <-time.After(30 * time.Millisecond):
	}
	release()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("second acquire did not resume")
	}
}

func baseService(t *testing.T) config.EffectiveService {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return config.EffectiveService{
		ID: "service", Program: executable, Cwd: t.TempDir(), Priority: "normal", Enabled: true, Autostart: true,
		Readiness: config.EffectiveProbe{Type: "process", StartupTimeout: time.Second, Interval: 20 * time.Millisecond, Timeout: 10 * time.Millisecond, FailureThreshold: 2},
		Restart:   config.EffectiveRestartPolicy{Policy: "always", MaxAttempts: 5, Window: time.Second, BackoffInitial: 5 * time.Millisecond, BackoffMax: 20 * time.Millisecond},
		Shutdown:  config.EffectiveShutdownPolicy{GracePeriod: 10 * time.Millisecond},
		Logs:      config.EffectiveLogPolicy{MaxBytes: 64 * 1024, Backups: 2},
	}
}

func newTestSupervisor(t *testing.T, launcher process.Launcher, spec config.EffectiveService) *Supervisor {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sup, err := New(Options{Service: spec, Generation: 1, ConfigHash: "hash-1", LogDir: filepath.Join(t.TempDir(), "logs"), Launcher: launcher, Limiter: NewLimiter(2), Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	return sup
}

func shutdownTestSupervisor(t *testing.T, sup *Supervisor) {
	t.Helper()
	if err := sup.Shutdown(); err != nil {
		return
	}
	select {
	case <-sup.Done():
	case <-time.After(time.Second):
		t.Error("supervisor did not stop")
	}
}

func waitState(t *testing.T, sup *Supervisor, state model.State) model.ServiceStatus {
	t.Helper()
	return waitFor(t, sup, func(status model.ServiceStatus) bool { return status.State == state })
}

func waitFor(t *testing.T, sup *Supervisor, predicate func(model.ServiceStatus) bool) model.ServiceStatus {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		status := sup.Status()
		if predicate(status) {
			return status
		}
		select {
		case <-deadline.C:
			t.Fatalf("timed out; final status=%#v", status)
		case <-ticker.C:
		}
	}
}
