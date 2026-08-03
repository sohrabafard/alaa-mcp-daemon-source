//go:build windows

package control

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"alaa-mcp-daemon/internal/model"
)

type windowsFakeHandler struct{ started string }

func (f *windowsFakeHandler) Status(service string) (model.DaemonStatus, error) {
	return model.DaemonStatus{ProtocolVersion: 1, DaemonVersion: "test", Services: []model.ServiceStatus{{ID: service, State: model.StateReady}}}, nil
}
func (f *windowsFakeHandler) Start(service string) error              { f.started = service; return nil }
func (f *windowsFakeHandler) Stop(string) error                       { return nil }
func (f *windowsFakeHandler) Restart(string) error                    { return nil }
func (f *windowsFakeHandler) Reload() error                           { return nil }
func (f *windowsFakeHandler) LogPaths(string) (string, string, error) { return "stdout", "stderr", nil }

func TestWindowsNamedPipeControlRoundTrip(t *testing.T) {
	state := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "config.json")
	listener, _, err := Listen(configPath, state)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	handler := &windowsFakeHandler{}
	server := NewServer(listener, handler, slog.New(slog.NewTextHandler(io.Discard, nil)), cancel)
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()
	response, err := Call(configPath, state, Request{Command: "start", Service: "svc"}, 2*time.Second)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if !response.OK || handler.started != "svc" {
		cancel()
		t.Fatalf("response=%#v started=%q", response, handler.started)
	}
	response, err = Call(configPath, state, Request{Command: "status", Service: "svc"}, 2*time.Second)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if response.Status == nil || len(response.Status.Services) != 1 || response.Status.Services[0].ID != "svc" {
		cancel()
		t.Fatalf("response=%#v", response)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("control server did not stop")
	}
}
