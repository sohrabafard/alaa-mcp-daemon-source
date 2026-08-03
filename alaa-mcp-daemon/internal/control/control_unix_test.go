//go:build !windows

package control

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"alaa-mcp-daemon/internal/model"
)

type fakeHandler struct{ started string }

func (f *fakeHandler) Status(service string) (model.DaemonStatus, error) {
	return model.DaemonStatus{ProtocolVersion: 1, DaemonVersion: "test", Services: []model.ServiceStatus{{ID: service, State: model.StateReady}}}, nil
}
func (f *fakeHandler) Start(service string) error              { f.started = service; return nil }
func (f *fakeHandler) Stop(string) error                       { return nil }
func (f *fakeHandler) Restart(string) error                    { return nil }
func (f *fakeHandler) Reload() error                           { return nil }
func (f *fakeHandler) LogPaths(string) (string, string, error) { return "stdout", "stderr", nil }

func TestUnixControlRoundTrip(t *testing.T) {
	state := t.TempDir()
	listener, _, err := Listen("config.json", state)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	handler := &fakeHandler{}
	server := NewServer(listener, handler, slog.New(slog.NewTextHandler(io.Discard, nil)), cancel)
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()
	response, err := Call("config.json", state, Request{Command: "start", Service: "svc"}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !response.OK || handler.started != "svc" {
		t.Fatalf("response=%#v started=%q", response, handler.started)
	}
	response, err = Call("config.json", state, Request{Command: "status", Service: "svc"}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if response.Status == nil || response.Status.Services[0].ID != "svc" {
		t.Fatalf("response=%#v", response)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop")
	}
}
