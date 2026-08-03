package manager

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"alaa-mcp-daemon/internal/config"
)

func TestReloadRetainsLastKnownGoodConfiguration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	writeManagerConfig(t, path, "first")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	mgr, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = mgr.Shutdown(ctx)
	}()
	before, _ := mgr.Status("")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Reload(); err == nil {
		t.Fatal("expected reload error")
	}
	afterInvalid, _ := mgr.Status("")
	if afterInvalid.ConfigGeneration != before.ConfigGeneration || afterInvalid.ConfigHash != before.ConfigHash {
		t.Fatalf("last-known-good state changed: before=%#v after=%#v", before, afterInvalid)
	}
	if afterInvalid.LastReloadError == "" {
		t.Fatal("reload finding was not exposed")
	}
	writeManagerConfig(t, path, "second")
	if err := mgr.Reload(); err != nil {
		t.Fatal(err)
	}
	afterValid, _ := mgr.Status("")
	if afterValid.ConfigGeneration != before.ConfigGeneration+1 {
		t.Fatalf("generation=%d", afterValid.ConfigGeneration)
	}
	if afterValid.LastReloadError != "" {
		t.Fatalf("reload error was not cleared: %s", afterValid.LastReloadError)
	}
}

func writeManagerConfig(t *testing.T, path, description string) {
	t.Helper()
	body := `{
"version":1,
"runtime":{"log_dir":"./logs","state_dir":"./state","max_parallel_starts":1,"watch_interval":"100ms","reload_debounce":"20ms"},
"commands":{"noop":{"program":"echo"}},
"services":[{"id":"manual","description":"` + description + `","command":"noop","autostart":false}]
}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestApplyStagesAdditionsBeforeRemovingLastKnownGoodService(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	initial := `{
"version":1,
"runtime":{"log_dir":"./logs","state_dir":"./state","max_parallel_starts":1,"watch_interval":"100ms","reload_debounce":"20ms"},
"commands":{"noop":{"program":"echo"}},
"services":[{"id":"old","command":"noop","autostart":false}]
}`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	mgr, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = mgr.Shutdown(ctx)
	}()
	before, err := mgr.Status("")
	if err != nil {
		t.Fatal(err)
	}
	// Make the new service's log directory impossible to create.
	if err := os.WriteFile(filepath.Join(dir, "logs", "new"), []byte("collision"), 0o600); err != nil {
		t.Fatal(err)
	}
	next := `{
"version":1,
"runtime":{"log_dir":"./logs","state_dir":"./state","max_parallel_starts":1,"watch_interval":"100ms","reload_debounce":"20ms"},
"commands":{"noop":{"program":"echo"}},
"services":[{"id":"new","command":"noop","autostart":false}]
}`
	if err := os.WriteFile(path, []byte(next), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Reload(); err == nil {
		t.Fatal("expected staged addition to fail")
	}
	after, err := mgr.Status("")
	if err != nil {
		t.Fatal(err)
	}
	if after.ConfigGeneration != before.ConfigGeneration || after.ConfigHash != before.ConfigHash {
		t.Fatalf("last-known-good generation changed: before=%#v after=%#v", before, after)
	}
	if len(after.Services) != 1 || after.Services[0].ID != "old" {
		t.Fatalf("old service was removed despite staging failure: %#v", after.Services)
	}
}

func TestApplyIdenticalConfigIsNoOpAndClearsReloadError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	writeManagerConfig(t, path, "same")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	mgr, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = mgr.Shutdown(ctx)
	}()
	mgr.SetReloadError(context.DeadlineExceeded)
	before, err := mgr.Status("")
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Apply(cfg); err != nil {
		t.Fatal(err)
	}
	after, err := mgr.Status("")
	if err != nil {
		t.Fatal(err)
	}
	if after.ConfigGeneration != before.ConfigGeneration {
		t.Fatalf("generation changed from %d to %d", before.ConfigGeneration, after.ConfigGeneration)
	}
	if after.LastReloadError != "" {
		t.Fatalf("reload error was not cleared: %s", after.LastReloadError)
	}
}

func TestShutdownTimeoutCoversLongestGracePeriod(t *testing.T) {
	mgr := &Manager{config: &config.Effective{Services: map[string]config.EffectiveService{
		"a": {Shutdown: config.EffectiveShutdownPolicy{GracePeriod: 5 * time.Minute}},
		"b": {Shutdown: config.EffectiveShutdownPolicy{GracePeriod: time.Second}},
	}}}
	if got, want := mgr.ShutdownTimeout(), 5*time.Minute+15*time.Second; got != want {
		t.Fatalf("timeout=%s want=%s", got, want)
	}
}
