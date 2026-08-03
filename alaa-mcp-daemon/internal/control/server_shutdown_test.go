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

type shutdownTestHandler struct{}

func (shutdownTestHandler) Status(string) (model.DaemonStatus, error) {
	return model.DaemonStatus{}, nil
}
func (shutdownTestHandler) Start(string) error                      { return nil }
func (shutdownTestHandler) Stop(string) error                       { return nil }
func (shutdownTestHandler) Restart(string) error                    { return nil }
func (shutdownTestHandler) Reload() error                           { return nil }
func (shutdownTestHandler) LogPaths(string) (string, string, error) { return "", "", nil }

func TestServerShutdownClosesStalledConnection(t *testing.T) {
	state := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "config.json")
	listener, _, err := Listen(configPath, state)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	server := NewServer(listener, shutdownTestHandler{}, slog.New(slog.NewTextHandler(io.Discard, nil)), cancel)
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()

	conn, err := dial(configPath, state, DialOptions{Timeout: time.Second})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	defer conn.Close()
	// Keep the request incomplete. Shutdown must still close this connection and
	// allow the server wait group to reach zero.
	if _, err := conn.Write([]byte(`{"version":1`)); err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server shutdown was blocked by a stalled control client")
	}
}
