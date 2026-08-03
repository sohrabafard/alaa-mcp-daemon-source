package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadExpandsAndNormalizes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alaa-mcp-daemon.json")
	writeConfig(t, path, `{
  "version": 1,
  "runtime": {"log_dir":"./var/log","state_dir":"./var/state","max_parallel_starts":2},
  "commands": {"tool":{"program":"echo","args":["--port","${port}"],"cwd":"${project}","env":{"TOKEN":"${token}"},"priority":"below_normal"}},
  "services": [{"id":"svc","command":"tool","vars":{"port":"42100","project":"./project","token":"secret"},"claims":[{"type":"tcp","host":"localhost","port":"${port}"}],"exclusive_keys":["project:${project}"],"readiness":{"type":"tcp","host":"127.0.0.1","port":"${port}"}}]
}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	service := cfg.Services["svc"]
	if service.Args[1] != "42100" {
		t.Fatalf("expanded port = %q", service.Args[1])
	}
	if service.Cwd != filepath.Join(dir, "project") {
		t.Fatalf("cwd = %q", service.Cwd)
	}
	if service.Env["TOKEN"] != "secret" {
		t.Fatalf("env expansion failed")
	}
	if service.Claims[0].Host != "127.0.0.1" || service.Claims[0].Port != 42100 {
		t.Fatalf("claim = %#v", service.Claims[0])
	}
	if service.Readiness.Type != "tcp" || service.Readiness.Port != 42100 {
		t.Fatalf("readiness = %#v", service.Readiness)
	}
	if cfg.Runtime.LogDir != filepath.Join(dir, "var", "log") {
		t.Fatalf("log dir = %q", cfg.Runtime.LogDir)
	}
	if len(cfg.Hash) != 64 {
		t.Fatalf("hash length = %d", len(cfg.Hash))
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfig(t, path, `{"version":1,"runtime":{"log_dir":"./logs","state_dir":"./state","max_parallel_starts":1,"unknown":true},"commands":{"x":{"program":"echo"}},"services":[]}`)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRejectsDuplicateClaims(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfig(t, path, `{
"version":1,"runtime":{"log_dir":"./logs","state_dir":"./state","max_parallel_starts":1},
"commands":{"x":{"program":"echo"}},
"services":[
{"id":"a","command":"x","claims":[{"type":"tcp","host":"127.0.0.1","port":"42100"}]},
{"id":"b","command":"x","claims":[{"type":"tcp","host":"localhost","port":"42100"}]}
]}`)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "shared") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRejectsMissingAndNestedVariables(t *testing.T) {
	for name, vars := range map[string]string{
		"missing": `{"value":"ok"}`,
		"nested":  `{"missing":"${other}"}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			writeConfig(t, path, `{"version":1,"runtime":{"log_dir":"./logs","state_dir":"./state","max_parallel_starts":1},"commands":{"x":{"program":"echo","args":["${missing}"]}},"services":[{"id":"a","command":"x","vars":`+vars+`}]}`)
			if _, err := Load(path); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadRejectsNonLoopbackProbe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfig(t, path, `{"version":1,"runtime":{"log_dir":"./logs","state_dir":"./state","max_parallel_starts":1},"commands":{"x":{"program":"echo"}},"services":[{"id":"a","command":"x","readiness":{"type":"tcp","host":"8.8.8.8","port":"53"}}]}`)
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("error = %v", err)
	}
}

func TestDiffRejectsRuntimeDirectoryChange(t *testing.T) {
	base := &Effective{Runtime: EffectiveRuntime{LogDir: "a", StateDir: "b"}}
	next := &Effective{Runtime: EffectiveRuntime{LogDir: "c", StateDir: "b"}}
	if _, err := Diff(base, next); err == nil {
		t.Fatal("expected error")
	}
}

func writeConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRejectsDuplicateJSONKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfig(t, path, `{"version":1,"version":1,"runtime":{"log_dir":"./logs","state_dir":"./state","max_parallel_starts":1},"commands":{"x":{"program":"echo"}},"services":[]}`)
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "duplicate object key") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRejectsExplicitZeroForPositiveDurations(t *testing.T) {
	cases := map[string]string{
		"watch interval":  `"watch_interval":"0s"`,
		"reload debounce": `"reload_debounce":"0s"`,
	}
	for name, field := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			writeConfig(t, path, `{"version":1,"runtime":{"log_dir":"./logs","state_dir":"./state","max_parallel_starts":1,`+field+`},"commands":{"x":{"program":"echo"}},"services":[]}`)
			if _, err := Load(path); err == nil {
				t.Fatal("expected explicit zero duration to fail validation")
			}
		})
	}
}

func TestLoadAcceptsExplicitZeroShutdownGracePeriod(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfig(t, path, `{"version":1,"runtime":{"log_dir":"./logs","state_dir":"./state","max_parallel_starts":1},"defaults":{"shutdown":{"grace_period":"0s"}},"commands":{"x":{"program":"echo"}},"services":[{"id":"a","command":"x","autostart":false}]}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Services["a"].Shutdown.GracePeriod; got != 0 {
		t.Fatalf("grace period = %s, want 0", got)
	}
}

func TestLoadRejectsMissingRequiredTopLevelFields(t *testing.T) {
	cases := map[string]string{
		"version":  `{"runtime":{},"commands":{"x":{"program":"echo"}},"services":[]}`,
		"runtime":  `{"version":1,"commands":{"x":{"program":"echo"}},"services":[]}`,
		"commands": `{"version":1,"runtime":{},"services":[]}`,
		"services": `{"version":1,"runtime":{},"commands":{"x":{"program":"echo"}}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			writeConfig(t, path, body)
			_, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), `required top-level field "`+name+`" is missing`) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestExplicitZeroDefaultShutdownSurvivesEmptyServiceOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfig(t, path, `{"version":1,"runtime":{},"defaults":{"shutdown":{"grace_period":"0s"}},"commands":{"x":{"program":"echo"}},"services":[{"id":"a","command":"x","autostart":false,"shutdown":{}}]}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Services["a"].Shutdown.GracePeriod; got != 0 {
		t.Fatalf("grace period = %s, want inherited 0", got)
	}
}
