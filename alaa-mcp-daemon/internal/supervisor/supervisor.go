package supervisor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"alaa-mcp-daemon/internal/config"
	"alaa-mcp-daemon/internal/logging"
	"alaa-mcp-daemon/internal/model"
	"alaa-mcp-daemon/internal/probe"
	"alaa-mcp-daemon/internal/process"
)

type ApplyMode string

const (
	ApplyInitial     ApplyMode = "initial"
	ApplyProcess     ApplyMode = "process"
	ApplySupervision ApplyMode = "supervision"
	ApplyMetadata    ApplyMode = "metadata"
)

type commandKind int

const (
	cmdApply commandKind = iota
	cmdStart
	cmdStop
	cmdRestart
	cmdRemove
	cmdShutdown
)

type command struct {
	kind       commandKind
	spec       config.EffectiveService
	mode       ApplyMode
	generation uint64
	configHash string
	resp       chan error
}

type launchResult struct {
	seq        uint64
	handle     process.Handle
	err        error
	blocked    bool
	executable string
	release    func()
	spec       config.EffectiveService
}

type exitEvent struct {
	seq    uint64
	handle process.Handle
}
type readyEvent struct {
	seq, probeSeq uint64
	result        probe.Result
	err           error
}
type healthEvent struct {
	seq, probeSeq uint64
	result        probe.Result
}
type stopEvent struct {
	seq uint64
	err error
}
type backoffEvent struct{ seq uint64 }

type Options struct {
	Service    config.EffectiveService
	Generation uint64
	ConfigHash string
	LogDir     string
	Launcher   process.Launcher
	Limiter    *Limiter
	Logger     *slog.Logger
}

type Supervisor struct {
	commands chan command
	events   chan any
	done     chan struct{}
	status   atomic.Value
	stdout   *logging.RotateWriter
	stderr   *logging.RotateWriter
	logger   *slog.Logger
}

func New(opts Options) (*Supervisor, error) {
	if opts.Launcher == nil {
		opts.Launcher = process.NewLauncher()
	}
	if opts.Limiter == nil {
		opts.Limiter = NewLimiter(1)
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	serviceDir := filepath.Join(opts.LogDir, opts.Service.ID)
	stdout, err := logging.NewRotateWriter(filepath.Join(serviceDir, "stdout.log"), opts.Service.Logs.MaxBytes, opts.Service.Logs.Backups)
	if err != nil {
		return nil, err
	}
	stderr, err := logging.NewRotateWriter(filepath.Join(serviceDir, "stderr.log"), opts.Service.Logs.MaxBytes, opts.Service.Logs.Backups)
	if err != nil {
		_ = stdout.Close()
		return nil, err
	}
	s := &Supervisor{
		commands: make(chan command), events: make(chan any, 32), done: make(chan struct{}),
		stdout: stdout, stderr: stderr, logger: opts.Logger.With("service", opts.Service.ID),
	}
	s.status.Store(model.ServiceStatus{ID: opts.Service.ID, State: model.StateStopped})
	go s.loop(opts, opts.Launcher, opts.Limiter)
	return s, nil
}

func (s *Supervisor) Apply(spec config.EffectiveService, mode ApplyMode, generation uint64, configHash string) error {
	return s.send(command{kind: cmdApply, spec: spec, mode: mode, generation: generation, configHash: configHash})
}
func (s *Supervisor) Start() error                { return s.send(command{kind: cmdStart}) }
func (s *Supervisor) Stop() error                 { return s.send(command{kind: cmdStop}) }
func (s *Supervisor) Restart() error              { return s.send(command{kind: cmdRestart}) }
func (s *Supervisor) Remove() error               { return s.send(command{kind: cmdRemove}) }
func (s *Supervisor) Shutdown() error             { return s.send(command{kind: cmdShutdown}) }
func (s *Supervisor) Done() <-chan struct{}       { return s.done }
func (s *Supervisor) Status() model.ServiceStatus { return s.status.Load().(model.ServiceStatus) }
func (s *Supervisor) LogPaths() (string, string)  { return s.stdout.Path(), s.stderr.Path() }

func (s *Supervisor) send(cmd command) error {
	cmd.resp = make(chan error, 1)
	select {
	case <-s.done:
		return errors.New("service supervisor is closed")
	case s.commands <- cmd:
	}
	select {
	case <-s.done:
		select {
		case err := <-cmd.resp:
			return err
		default:
			return errors.New("service supervisor closed")
		}
	case err := <-cmd.resp:
		return err
	}
}

type runtimeState struct {
	spec          config.EffectiveService
	generation    uint64
	configHash    string
	state         model.State
	manualHold    bool
	manualStarted bool
	closing       bool

	handle           process.Handle
	launching        bool
	stopping         bool
	stopIssued       bool
	restartAfter     bool
	restartAsFailure bool
	stopTarget       model.State
	seq              uint64
	probeSeq         uint64
	probeCancel      context.CancelFunc
	backoffCancel    context.CancelFunc
	startRelease     func()
	launchCancel     context.CancelFunc
	stopCompleted    bool
	activeClaims     []config.TCPClaim
	activeGrace      time.Duration

	startedAt    *time.Time
	readyAt      *time.Time
	readiness    string
	restartCount int
	lastExitCode *int
	lastError    string
	backoffUntil *time.Time
	executable   string
	retries      retryWindow
}

func (s *Supervisor) loop(opts Options, launcher process.Launcher, limiter *Limiter) {
	st := runtimeState{spec: opts.Service, generation: opts.Generation, configHash: opts.ConfigHash, state: model.StateStopped}
	st.publish(s)
	st.reconcile(s, launcher, limiter)
	for {
		select {
		case cmd := <-s.commands:
			err := st.handleCommand(s, launcher, limiter, cmd)
			cmd.resp <- err
		case event := <-s.events:
			st.handleEvent(s, launcher, limiter, event)
		}
		st.publish(s)
		if st.closing && st.handle == nil && !st.launching && !st.stopping {
			if st.probeCancel != nil {
				st.probeCancel()
			}
			if st.backoffCancel != nil {
				st.backoffCancel()
			}
			if st.startRelease != nil {
				st.startRelease()
				st.startRelease = nil
			}
			if st.launchCancel != nil {
				st.launchCancel()
				st.launchCancel = nil
			}
			_ = s.stdout.Close()
			_ = s.stderr.Close()
			close(s.done)
			return
		}
	}
}

func (st *runtimeState) handleCommand(s *Supervisor, launcher process.Launcher, limiter *Limiter, cmd command) error {
	switch cmd.kind {
	case cmdApply:
		old := st.spec
		if old.ID != "" && old.ID != cmd.spec.ID {
			return errors.New("service id cannot change")
		}
		if old.Logs != cmd.spec.Logs {
			if err := s.stdout.Update(cmd.spec.Logs.MaxBytes, cmd.spec.Logs.Backups); err != nil {
				return fmt.Errorf("update stdout log policy: %w", err)
			}
			if err := s.stderr.Update(cmd.spec.Logs.MaxBytes, cmd.spec.Logs.Backups); err != nil {
				rollbackErr := s.stdout.Update(old.Logs.MaxBytes, old.Logs.Backups)
				return errors.Join(fmt.Errorf("update stderr log policy: %w", err), rollbackErr)
			}
		}
		st.spec, st.generation, st.configHash = cmd.spec, cmd.generation, cmd.configHash
		switch cmd.mode {
		case ApplyProcess:
			st.resetRetries()
			st.forceReconcile(s, launcher, limiter)
		case ApplySupervision:
			st.resetRetries()
			if st.state == model.StateFailed {
				st.state = model.StateStopped
			}
			if st.handle != nil {
				st.activeGrace = cmd.spec.Shutdown.GracePeriod
				switch st.state {
				case model.StateReady:
					st.startHealth(s)
				case model.StateStarting:
					st.startReadiness(s)
				}
			}
			st.reconcile(s, launcher, limiter)
		case ApplyMetadata, ApplyInitial:
			st.reconcile(s, launcher, limiter)
		default:
			return fmt.Errorf("unsupported apply mode %q", cmd.mode)
		}
		return nil
	case cmdStart:
		if !st.spec.Enabled {
			return errors.New("service is disabled in configuration")
		}
		st.manualHold = false
		st.manualStarted = true
		st.resetRetries()
		if st.state == model.StateBlocked || st.state == model.StateFailed {
			st.state = model.StateStopped
			st.lastError = ""
		}
		st.reconcile(s, launcher, limiter)
		return nil
	case cmdStop:
		st.manualHold = true
		st.manualStarted = false
		st.reconcile(s, launcher, limiter)
		return nil
	case cmdRestart:
		if !st.spec.Enabled {
			return errors.New("service is disabled in configuration")
		}
		st.manualHold = false
		st.manualStarted = true
		st.resetRetries()
		st.forceReconcile(s, launcher, limiter)
		return nil
	case cmdRemove:
		st.closing = true
		st.manualHold = true
		st.reconcile(s, launcher, limiter)
		return nil
	case cmdShutdown:
		st.closing = true
		st.manualHold = true
		st.reconcile(s, launcher, limiter)
		return nil
	default:
		return errors.New("unknown supervisor command")
	}
}

func (st *runtimeState) handleEvent(s *Supervisor, launcher process.Launcher, limiter *Limiter, event any) {
	switch ev := event.(type) {
	case launchResult:
		if ev.seq != st.seq {
			if ev.release != nil {
				ev.release()
			}
			if ev.handle != nil {
				go func() { _ = ev.handle.Stop(0) }()
			}
			return
		}
		st.launching = false
		if st.launchCancel != nil {
			st.launchCancel()
			st.launchCancel = nil
		}
		st.executable = ev.executable
		if ev.err != nil {
			if ev.release != nil {
				ev.release()
			}
			st.startRelease = nil
			st.lastError = ev.err.Error()
			if st.stopping || st.closing || !st.desiredRunning() {
				st.finishIntentionalStop(s, launcher, limiter)
				return
			}
			if ev.blocked {
				st.state = model.StateBlocked
				s.logger.Warn("service start blocked", "error", ev.err)
				return
			}
			s.logger.Error("service launch failed", "error", ev.err)
			st.scheduleFailure(s, launcher, limiter)
			return
		}
		st.handle = ev.handle
		st.startRelease = ev.release
		st.activeClaims = append(st.activeClaims[:0], ev.spec.Claims...)
		st.activeGrace = st.spec.Shutdown.GracePeriod
		now := time.Now().UTC()
		st.startedAt, st.readyAt = &now, nil
		st.lastError, st.readiness = "", "starting"
		go func(seq uint64, h process.Handle) { <-h.Done(); s.events <- exitEvent{seq: seq, handle: h} }(ev.seq, ev.handle)
		if st.stopping || st.closing || !st.desiredRunning() {
			if st.startRelease != nil {
				st.startRelease()
				st.startRelease = nil
			}
			st.beginHandleStop(s)
			return
		}
		st.startReadiness(s)
	case readyEvent:
		if ev.seq != st.seq || ev.probeSeq != st.probeSeq || st.handle == nil {
			return
		}
		if st.startRelease != nil {
			st.startRelease()
			st.startRelease = nil
		}
		if ev.err != nil {
			st.lastError = ev.err.Error()
			st.readiness = ev.result.Message
			s.logger.Error("service did not become ready", "error", ev.err)
			st.failRunningProcess(s, launcher, limiter)
			return
		}
		now := time.Now().UTC()
		st.readyAt = &now
		st.state = model.StateReady
		st.readiness = ev.result.Message
		st.lastError = ""
		s.logger.Info("service ready", "pid", st.handle.PID(), "readiness", ev.result.Message)
		st.startHealth(s)
	case healthEvent:
		if ev.seq != st.seq || ev.probeSeq != st.probeSeq || st.handle == nil {
			return
		}
		st.state = model.StateUnhealthy
		st.readiness = ev.result.Message
		st.lastError = ev.result.Message
		s.logger.Error("service health threshold exceeded", "reason", ev.result.Message)
		st.failRunningProcess(s, launcher, limiter)
	case exitEvent:
		if ev.seq != st.seq || st.handle != ev.handle {
			return
		}
		if st.probeCancel != nil {
			st.probeCancel()
			st.probeCancel = nil
		}
		if st.startRelease != nil {
			st.startRelease()
			st.startRelease = nil
		}
		result := ev.handle.Result()
		code := result.ExitCode
		st.lastExitCode = &code
		st.handle = nil
		st.startedAt, st.readyAt = nil, nil
		if st.stopping {
			if st.stopCompleted {
				st.finishIntentionalStop(s, launcher, limiter)
			}
			return
		}
		st.activeClaims = nil
		st.activeGrace = 0
		st.state = model.StateStopped
		if result.Err != nil {
			st.lastError = result.Err.Error()
		}
		s.logger.Warn("service exited", "exit_code", result.ExitCode, "error", result.Err)
		if st.desiredRunning() {
			if st.shouldRestart(result.ExitCode) {
				st.scheduleFailure(s, launcher, limiter)
			} else if result.ExitCode != 0 {
				st.state = model.StateFailed
			}
		}
	case stopEvent:
		if ev.seq != st.seq || !st.stopping {
			return
		}
		st.stopCompleted = true
		if ev.err != nil {
			st.lastError = ev.err.Error()
			s.logger.Warn("service stop completed with finding", "error", ev.err)
		}
		// A restart or final stopped state is published only after both the root
		// process has exited and the process backend has confirmed cleanup.
		if st.handle == nil {
			st.finishIntentionalStop(s, launcher, limiter)
		}
	case backoffEvent:
		if ev.seq != st.seq {
			return
		}
		st.backoffCancel = nil
		st.backoffUntil = nil
		if st.desiredRunning() {
			st.state = model.StateStopped
			st.start(s, launcher, limiter)
		} else {
			st.reconcile(s, launcher, limiter)
		}
	}
}

func (st *runtimeState) desiredRunning() bool {
	return st.spec.Enabled && !st.manualHold && (st.spec.Autostart || st.manualStarted) && !st.closing
}

func (st *runtimeState) desiredLabel() string {
	if !st.spec.Enabled {
		return "disabled"
	}
	if st.closing || st.manualHold {
		return "stopped"
	}
	if st.spec.Autostart || st.manualStarted {
		return "running"
	}
	return "stopped"
}

func (st *runtimeState) reconcile(s *Supervisor, launcher process.Launcher, limiter *Limiter) {
	if !st.desiredRunning() {
		if st.backoffCancel != nil {
			st.backoffCancel()
			st.backoffCancel = nil
			st.backoffUntil = nil
		}
		if st.probeCancel != nil {
			st.probeCancel()
			st.probeCancel = nil
		}
		target := model.StateStopped
		if !st.spec.Enabled {
			target = model.StateDisabled
		}
		if st.handle != nil || st.launching {
			st.initiateStop(s, launcher, limiter, target, false, false)
		} else if !st.stopping {
			st.state = target
		}
		return
	}
	if st.handle == nil && !st.launching && !st.stopping && st.backoffCancel == nil {
		if st.state == model.StateBlocked || st.state == model.StateFailed {
			return
		}
		st.start(s, launcher, limiter)
	}
}

func (st *runtimeState) forceReconcile(s *Supervisor, launcher process.Launcher, limiter *Limiter) {
	if st.backoffCancel != nil {
		st.backoffCancel()
		st.backoffCancel = nil
		st.backoffUntil = nil
	}
	if st.handle != nil || st.launching || st.stopping {
		st.initiateStop(s, launcher, limiter, model.StateStopped, st.desiredRunning(), false)
		return
	}
	st.state = model.StateStopped
	st.reconcile(s, launcher, limiter)
}

func (st *runtimeState) start(s *Supervisor, launcher process.Launcher, limiter *Limiter) {
	st.seq++
	seq := st.seq
	st.launching = true
	st.state = model.StateStarting
	st.readiness = "waiting for launch"
	st.lastError = ""
	spec := st.spec
	ctx, cancel := context.WithCancel(context.Background())
	st.launchCancel = cancel
	go func() {
		defer cancel()
		release, err := limiter.Acquire(ctx)
		if err != nil {
			s.events <- launchResult{seq: seq, err: err, spec: spec}
			return
		}
		executable, blocked, err := preflight(spec)
		if err != nil {
			s.events <- launchResult{seq: seq, err: err, blocked: blocked, executable: executable, release: release, spec: spec}
			return
		}
		handle, err := launcher.Start(process.Spec{Program: executable, Args: spec.Args, Cwd: spec.Cwd, Env: spec.Env, Priority: spec.Priority, Stdout: s.stdout, Stderr: s.stderr})
		s.events <- launchResult{seq: seq, handle: handle, err: err, executable: executable, release: release, spec: spec}
	}()
}

func (st *runtimeState) initiateStop(s *Supervisor, launcher process.Launcher, limiter *Limiter, target model.State, restart, asFailure bool) {
	if st.probeCancel != nil {
		st.probeCancel()
		st.probeCancel = nil
	}
	if st.backoffCancel != nil {
		st.backoffCancel()
		st.backoffCancel = nil
		st.backoffUntil = nil
	}
	if st.stopping {
		st.stopTarget = target
		st.restartAfter = restart
		st.restartAsFailure = asFailure
		return
	}
	st.stopTarget = target
	st.restartAfter = restart
	st.restartAsFailure = asFailure
	st.state = model.StateStopping
	st.stopping = true
	st.stopCompleted = false
	if st.launchCancel != nil {
		st.launchCancel()
	}
	if st.startRelease != nil {
		st.startRelease()
		st.startRelease = nil
	}
	if st.launching {
		return
	}
	if st.handle == nil {
		st.finishIntentionalStop(s, launcher, limiter)
		return
	}
	st.beginHandleStop(s)
}

func (st *runtimeState) beginHandleStop(s *Supervisor) {
	if st.handle == nil || st.stopIssued {
		return
	}
	st.stopIssued = true
	seq := st.seq
	handle := st.handle
	grace := st.activeGrace
	claims := append([]config.TCPClaim(nil), st.activeClaims...)
	go func() {
		err := handle.Stop(grace)
		if releaseErr := waitClaimsFree(claims, 3*time.Second); err == nil {
			err = releaseErr
		}
		s.events <- stopEvent{seq: seq, err: err}
	}()
}

func (st *runtimeState) finishIntentionalStop(s *Supervisor, launcher process.Launcher, limiter *Limiter) {
	target, restart, asFailure := st.stopTarget, st.restartAfter, st.restartAsFailure
	st.stopping = false
	st.stopIssued = false
	st.stopCompleted = false
	st.activeClaims = nil
	st.activeGrace = 0
	st.restartAfter = false
	st.restartAsFailure = false
	st.state = target
	if restart && st.desiredRunning() {
		if asFailure {
			st.scheduleFailure(s, launcher, limiter)
		} else if launcher != nil && limiter != nil {
			st.start(s, launcher, limiter)
		}
		return
	}
	if launcher != nil && limiter != nil {
		st.reconcile(s, launcher, limiter)
	}
}

func (st *runtimeState) startReadiness(s *Supervisor) {
	if st.probeCancel != nil {
		st.probeCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	st.probeCancel = cancel
	st.probeSeq++
	probeSeq, seq, p, handle := st.probeSeq, st.seq, st.spec.Readiness, st.handle
	go func() {
		deadline := time.Now().Add(p.StartupTimeout)
		interval := p.Interval
		if interval > 500*time.Millisecond {
			interval = 500 * time.Millisecond
		}
		for {
			checkCtx, checkCancel := context.WithTimeout(ctx, p.Timeout)
			result := probe.Check(checkCtx, p, func() bool {
				select {
				case <-handle.Done():
					return false
				default:
					return true
				}
			})
			checkCancel()
			if result.OK {
				s.events <- readyEvent{seq: seq, probeSeq: probeSeq, result: result}
				return
			}
			if time.Now().After(deadline) {
				s.events <- readyEvent{seq: seq, probeSeq: probeSeq, result: result, err: fmt.Errorf("startup readiness timed out after %s: %s", p.StartupTimeout, result.Message)}
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}
		}
	}()
}

func (st *runtimeState) startHealth(s *Supervisor) {
	if st.probeCancel != nil {
		st.probeCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	st.probeCancel = cancel
	st.probeSeq++
	probeSeq, seq, p, handle := st.probeSeq, st.seq, st.spec.Readiness, st.handle
	go func() {
		failures := 0
		ticker := time.NewTicker(p.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				checkCtx, checkCancel := context.WithTimeout(ctx, p.Timeout)
				result := probe.Check(checkCtx, p, func() bool {
					select {
					case <-handle.Done():
						return false
					default:
						return true
					}
				})
				checkCancel()
				if result.OK {
					failures = 0
					continue
				}
				failures++
				if failures >= p.FailureThreshold {
					s.events <- healthEvent{seq: seq, probeSeq: probeSeq, result: result}
					return
				}
			}
		}
	}()
}

func (st *runtimeState) failRunningProcess(s *Supervisor, launcher process.Launcher, limiter *Limiter) {
	st.initiateStop(s, launcher, limiter, model.StateStopped, true, true)
}

func (st *runtimeState) shouldRestart(exitCode int) bool {
	switch st.spec.Restart.Policy {
	case "always":
		return true
	case "on_failure":
		return exitCode != 0
	default:
		return false
	}
}

func (st *runtimeState) scheduleFailure(s *Supervisor, launcher process.Launcher, limiter *Limiter) {
	if !st.desiredRunning() {
		st.reconcile(s, launcher, limiter)
		return
	}
	if st.spec.Restart.Policy == "never" {
		st.state = model.StateFailed
		return
	}
	now := time.Now().UTC()
	count, exhausted, delay := st.retries.Record(now, st.spec.Restart)
	if exhausted {
		st.state = model.StateFailed
		cause := st.lastError
		st.lastError = fmt.Sprintf("restart budget exhausted after %d failures in %s", count, st.spec.Restart.Window)
		if cause != "" {
			st.lastError += "; last failure: " + cause
		}
		s.logger.Error("restart budget exhausted", "failures", count, "window", st.spec.Restart.Window)
		return
	}
	st.restartCount++
	until := now.Add(delay)
	st.backoffUntil = &until
	st.state = model.StateBackoff
	ctx, cancel := context.WithCancel(context.Background())
	st.backoffCancel = cancel
	seq := st.seq
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.events <- backoffEvent{seq: seq}
		}
	}()
}

func (st *runtimeState) resetRetries() {
	st.retries.Reset()
	st.backoffUntil = nil
	if st.backoffCancel != nil {
		st.backoffCancel()
		st.backoffCancel = nil
	}
}

func (st *runtimeState) publish(s *Supervisor) {
	claims := make([]model.Endpoint, len(st.spec.Claims))
	for i, claim := range st.spec.Claims {
		claims[i] = model.Endpoint{Type: "tcp", Host: claim.Host, Port: claim.Port}
	}
	status := model.ServiceStatus{
		ID: st.spec.ID, Description: st.spec.Description, Desired: st.desiredLabel(), State: st.state,
		StartedAt: st.startedAt, ReadyAt: st.readyAt, Readiness: st.readiness, Claims: claims,
		RestartCount: st.restartCount, FailuresInWindow: st.retries.Count(time.Now(), st.spec.Restart.Window),
		LastExitCode: st.lastExitCode, LastError: st.lastError, BackoffUntil: st.backoffUntil,
		Executable: st.executable, ConfigGeneration: st.generation, ConfigHash: st.configHash, ManualHold: st.manualHold,
	}
	if st.handle != nil {
		status.PID = st.handle.PID()
	}
	s.status.Store(status)
}

func preflight(spec config.EffectiveService) (string, bool, error) {
	info, err := os.Stat(spec.Cwd)
	if err != nil {
		return "", true, fmt.Errorf("working directory %q is unavailable: %w", spec.Cwd, err)
	}
	if !info.IsDir() {
		return "", true, fmt.Errorf("working directory %q is not a directory", spec.Cwd)
	}
	executable := spec.Program
	if !filepath.IsAbs(executable) {
		executable, err = exec.LookPath(executable)
		if err != nil {
			return "", true, fmt.Errorf("executable %q cannot be resolved: %w", spec.Program, err)
		}
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", true, fmt.Errorf("resolve executable: %w", err)
	}
	executableInfo, err := os.Stat(executable)
	if err != nil {
		return executable, true, fmt.Errorf("executable %q is unavailable: %w", executable, err)
	}
	if executableInfo.IsDir() {
		return executable, true, fmt.Errorf("executable %q is a directory", executable)
	}
	for _, claim := range spec.Claims {
		listener, listenErr := net.Listen("tcp", net.JoinHostPort(claim.Host, strconv.Itoa(claim.Port)))
		if listenErr != nil {
			return executable, true, fmt.Errorf("claimed endpoint %s:%d is occupied: %w", claim.Host, claim.Port, listenErr)
		}
		_ = listener.Close()
	}
	return executable, false, nil
}

func waitClaimsFree(claims []config.TCPClaim, timeout time.Duration) error {
	if len(claims) == 0 {
		return nil
	}
	deadline := time.Now().Add(timeout)
	var last error
	for {
		allFree := true
		for _, claim := range claims {
			listener, err := net.Listen("tcp", net.JoinHostPort(claim.Host, strconv.Itoa(claim.Port)))
			if err != nil {
				allFree = false
				last = err
				break
			}
			_ = listener.Close()
		}
		if allFree {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("claimed port remained occupied after process stop: %w", last)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
