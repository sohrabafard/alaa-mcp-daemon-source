package manager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"sync"
	"time"

	"alaa-mcp-daemon/internal/buildinfo"
	"alaa-mcp-daemon/internal/config"
	"alaa-mcp-daemon/internal/model"
	"alaa-mcp-daemon/internal/process"
	"alaa-mcp-daemon/internal/supervisor"
)

type Manager struct {
	mu              sync.RWMutex
	config          *config.Effective
	generation      uint64
	supervisors     map[string]*supervisor.Supervisor
	limiter         *supervisor.Limiter
	launcher        process.Launcher
	logger          *slog.Logger
	startedAt       time.Time
	lastReloadError string
	closed          bool
}

func New(cfg *config.Effective, logger *slog.Logger) (*Manager, error) {
	if cfg == nil {
		return nil, errors.New("configuration is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if err := os.MkdirAll(cfg.Runtime.LogDir, 0o700); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	if err := os.MkdirAll(cfg.Runtime.StateDir, 0o700); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}
	m := &Manager{
		config: cfg, generation: 1, supervisors: make(map[string]*supervisor.Supervisor),
		limiter: supervisor.NewLimiter(cfg.Runtime.MaxParallelStarts), launcher: process.NewLauncher(),
		logger: logger, startedAt: time.Now().UTC(),
	}
	for _, id := range cfg.Order {
		service := cfg.Services[id]
		sup, err := supervisor.New(supervisor.Options{
			Service: service, Generation: m.generation, ConfigHash: cfg.Hash, LogDir: cfg.Runtime.LogDir,
			Launcher: m.launcher, Limiter: m.limiter, Logger: logger,
		})
		if err != nil {
			ctx, cancel := context.WithTimeout(context.Background(), m.shutdownTimeoutLocked())
			_ = m.shutdownLocked(ctx)
			cancel()
			return nil, fmt.Errorf("create supervisor %q: %w", id, err)
		}
		m.supervisors[id] = sup
	}
	return m, nil
}

func (m *Manager) Apply(next *config.Effective) error {
	if next == nil {
		return errors.New("configuration is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("manager is closed")
	}
	if m.config.Hash == next.Hash {
		m.lastReloadError = ""
		return nil
	}
	changes, err := config.Diff(m.config, next)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(next.Runtime.LogDir, 0o700); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}
	if err := os.MkdirAll(next.Runtime.StateDir, 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	generation := m.generation + 1
	oldLimit := m.config.Runtime.MaxParallelStarts
	m.limiter.SetLimit(next.Runtime.MaxParallelStarts)
	committed := false
	defer func() {
		if !committed {
			m.limiter.SetLimit(oldLimit)
		}
	}()

	// Stage added supervisors in a disabled state before stopping any existing
	// service. A log/open failure therefore cannot destroy the last-known-good
	// service set merely because the new config also renamed a service.
	staged := make(map[string]*supervisor.Supervisor)
	cleanupStaged := func() {
		for _, sup := range staged {
			_ = sup.Shutdown()
		}
		deadline := time.NewTimer(30 * time.Second)
		defer deadline.Stop()
		for _, sup := range staged {
			select {
			case <-sup.Done():
			case <-deadline.C:
				return
			}
		}
	}
	defer func() {
		if !committed {
			cleanupStaged()
		}
	}()
	for _, change := range changes {
		if change.Kind != config.ChangeAdd {
			continue
		}
		prepared := *change.New
		prepared.Enabled = false
		prepared.Autostart = false
		sup, createErr := supervisor.New(supervisor.Options{
			Service: prepared, Generation: generation, ConfigHash: next.Hash, LogDir: next.Runtime.LogDir,
			Launcher: m.launcher, Limiter: m.limiter, Logger: m.logger,
		})
		if createErr != nil {
			return fmt.Errorf("stage service %q: %w", change.ID, createErr)
		}
		staged[change.ID] = sup
	}

	// Removals complete before additions are activated so a renamed service can
	// safely reuse the old service's endpoint.
	for _, change := range changes {
		if change.Kind != config.ChangeRemove {
			continue
		}
		sup := m.supervisors[change.ID]
		if sup == nil {
			continue
		}
		if err := sup.Remove(); err != nil {
			return fmt.Errorf("remove service %q: %w", change.ID, err)
		}
		wait := 15 * time.Second
		if change.Old != nil {
			wait = change.Old.Shutdown.GracePeriod + 15*time.Second
		}
		select {
		case <-sup.Done():
		case <-time.After(wait):
			return fmt.Errorf("remove service %q: supervisor did not stop within %s", change.ID, wait)
		}
		delete(m.supervisors, change.ID)
	}

	// Existing supervisors are updated before staged additions are allowed to
	// start. Config validation has already classified every change mode.
	for _, change := range changes {
		switch change.Kind {
		case config.ChangeRemove, config.ChangeAdd:
			continue
		case config.ChangeProcessRestart:
			if err := m.supervisors[change.ID].Apply(*change.New, supervisor.ApplyProcess, generation, next.Hash); err != nil {
				return fmt.Errorf("apply process change to %q: %w", change.ID, err)
			}
		case config.ChangeSupervision:
			if err := m.supervisors[change.ID].Apply(*change.New, supervisor.ApplySupervision, generation, next.Hash); err != nil {
				return fmt.Errorf("apply supervision change to %q: %w", change.ID, err)
			}
		case config.ChangeMetadata, config.ChangeNone:
			if err := m.supervisors[change.ID].Apply(*change.New, supervisor.ApplyMetadata, generation, next.Hash); err != nil {
				return fmt.Errorf("apply metadata change to %q: %w", change.ID, err)
			}
		default:
			return fmt.Errorf("unsupported change kind %q", change.Kind)
		}
	}

	for _, change := range changes {
		if change.Kind != config.ChangeAdd {
			continue
		}
		sup := staged[change.ID]
		if err := sup.Apply(*change.New, supervisor.ApplySupervision, generation, next.Hash); err != nil {
			return fmt.Errorf("activate service %q: %w", change.ID, err)
		}
	}
	for id, sup := range staged {
		m.supervisors[id] = sup
	}

	m.config = next
	m.generation = generation
	m.lastReloadError = ""
	committed = true
	m.logger.Info("configuration applied", "generation", generation, "hash", next.Hash, "services", len(next.Services))
	return nil
}

func (m *Manager) Reload() error {
	m.mu.RLock()
	path := m.config.Path
	m.mu.RUnlock()
	cfg, err := config.Load(path)
	if err != nil {
		m.SetReloadError(err)
		return err
	}
	if err := m.Apply(cfg); err != nil {
		m.SetReloadError(err)
		return err
	}
	return nil
}

func (m *Manager) SetReloadError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err == nil {
		m.lastReloadError = ""
		return
	}
	m.lastReloadError = err.Error()
	m.logger.Error("configuration reload rejected; retaining last-known-good state", "error", err)
}

func (m *Manager) Status(serviceID string) (model.DaemonStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := model.DaemonStatus{
		ProtocolVersion: 1, DaemonVersion: buildinfo.Version, PID: os.Getpid(), StartedAt: m.startedAt,
		ConfigPath: m.config.Path, ConfigGeneration: m.generation, ConfigHash: m.config.Hash,
		LastReloadError: m.lastReloadError,
	}
	if serviceID != "" {
		sup := m.supervisors[serviceID]
		if sup == nil {
			return model.DaemonStatus{}, fmt.Errorf("unknown service %q", serviceID)
		}
		status.Services = []model.ServiceStatus{sup.Status()}
		return status, nil
	}
	status.Services = make([]model.ServiceStatus, 0, len(m.supervisors))
	seen := make(map[string]struct{}, len(m.supervisors))
	for _, id := range m.config.Order {
		if sup := m.supervisors[id]; sup != nil {
			status.Services = append(status.Services, sup.Status())
			seen[id] = struct{}{}
		}
	}
	var extras []string
	for id := range m.supervisors {
		if _, ok := seen[id]; !ok {
			extras = append(extras, id)
		}
	}
	sort.Strings(extras)
	for _, id := range extras {
		status.Services = append(status.Services, m.supervisors[id].Status())
	}
	return status, nil
}

func (m *Manager) Start(id string) error { return m.withService(id, (*supervisor.Supervisor).Start) }
func (m *Manager) Stop(id string) error  { return m.withService(id, (*supervisor.Supervisor).Stop) }
func (m *Manager) Restart(id string) error {
	return m.withService(id, (*supervisor.Supervisor).Restart)
}

func (m *Manager) LogPaths(id string) (string, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sup := m.supervisors[id]
	if sup == nil {
		return "", "", fmt.Errorf("unknown service %q", id)
	}
	stdout, stderr := sup.LogPaths()
	return stdout, stderr, nil
}

func (m *Manager) ShutdownTimeout() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.shutdownTimeoutLocked()
}

func (m *Manager) shutdownTimeoutLocked() time.Duration {
	maximum := time.Duration(0)
	for _, service := range m.config.Services {
		if service.Shutdown.GracePeriod > maximum {
			maximum = service.Shutdown.GracePeriod
		}
	}
	timeout := maximum + 15*time.Second
	if timeout < 30*time.Second {
		timeout = 30 * time.Second
	}
	return timeout
}

func (m *Manager) ConfigPath() string { m.mu.RLock(); defer m.mu.RUnlock(); return m.config.Path }
func (m *Manager) Runtime() config.EffectiveRuntime {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config.Runtime
}

func (m *Manager) withService(id string, fn func(*supervisor.Supervisor) error) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return errors.New("manager is closed")
	}
	sup := m.supervisors[id]
	m.mu.RUnlock()
	if sup == nil {
		return fmt.Errorf("unknown service %q", id)
	}
	return fn(sup)
}

func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shutdownLocked(ctx)
}

func (m *Manager) shutdownLocked(ctx context.Context) error {
	if m.closed {
		return nil
	}
	m.closed = true
	var errs []error
	for id, sup := range m.supervisors {
		if err := sup.Shutdown(); err != nil {
			errs = append(errs, fmt.Errorf("shutdown %q: %w", id, err))
		}
	}
	for id, sup := range m.supervisors {
		select {
		case <-sup.Done():
		case <-ctx.Done():
			errs = append(errs, fmt.Errorf("shutdown %q: %w", id, ctx.Err()))
		}
	}
	return errors.Join(errs...)
}
