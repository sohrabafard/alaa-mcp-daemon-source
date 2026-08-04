package control

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
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

type completedResponseConn struct {
	request   *bytes.Reader
	mu        sync.Mutex
	response  bytes.Buffer
	closed    chan struct{}
	closeOnce sync.Once
	abortive  bool
	graceful  bool
}

func newCompletedResponseConn(request string) *completedResponseConn {
	return &completedResponseConn{request: bytes.NewReader([]byte(request)), closed: make(chan struct{})}
}

func (c *completedResponseConn) Read(p []byte) (int, error) { return c.request.Read(p) }

func (c *completedResponseConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.abortive || c.graceful {
		return 0, os.ErrClosed
	}
	return c.response.Write(p)
}

func (c *completedResponseConn) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.abortive = true
		c.response.Reset()
		c.mu.Unlock()
		close(c.closed)
	})
	return nil
}

func (c *completedResponseConn) CloseGracefully() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.graceful = true
		c.mu.Unlock()
		close(c.closed)
	})
	return nil
}

func (c *completedResponseConn) snapshot() ([]byte, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return bytes.Clone(c.response.Bytes()), c.abortive, c.graceful
}

type singleConnListener struct {
	conn      io.ReadWriteCloser
	mu        sync.Mutex
	accepted  bool
	closed    chan struct{}
	closeOnce sync.Once
}

func newSingleConnListener(conn io.ReadWriteCloser) *singleConnListener {
	return &singleConnListener{conn: conn, closed: make(chan struct{})}
}

func (l *singleConnListener) Accept() (io.ReadWriteCloser, error) {
	l.mu.Lock()
	if !l.accepted {
		l.accepted = true
		l.mu.Unlock()
		return l.conn, nil
	}
	closed := l.closed
	l.mu.Unlock()
	<-closed
	return nil, os.ErrClosed
}

func (l *singleConnListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	return nil
}

type admissionRaceContext struct {
	done        chan struct{}
	errChecked  chan struct{}
	releaseErr  chan struct{}
	cancelOnce  sync.Once
	releaseOnce sync.Once
	errOnce     sync.Once
	mu          sync.Mutex
	err         error
}

func newAdmissionRaceContext() *admissionRaceContext {
	return &admissionRaceContext{
		done:       make(chan struct{}),
		errChecked: make(chan struct{}),
		releaseErr: make(chan struct{}),
	}
}

func (*admissionRaceContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *admissionRaceContext) Done() <-chan struct{}     { return c.done }
func (*admissionRaceContext) Value(any) any               { return nil }

func (c *admissionRaceContext) Err() error {
	first := false
	c.errOnce.Do(func() { first = true })
	if first {
		c.mu.Lock()
		err := c.err
		c.mu.Unlock()
		close(c.errChecked)
		<-c.releaseErr
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *admissionRaceContext) cancel() {
	c.cancelOnce.Do(func() {
		c.mu.Lock()
		c.err = context.Canceled
		c.mu.Unlock()
		close(c.done)
	})
}

func (c *admissionRaceContext) releaseErrCheck() {
	c.releaseOnce.Do(func() { close(c.releaseErr) })
}

func TestServerServeConn_PreservesCompletedResponseUntilClientReads(t *testing.T) {
	conn := newCompletedResponseConn("{\"version\":1,\"command\":\"status\",\"service\":\"svc\"}\n")
	listener := newSingleConnListener(conn)
	ctx, cancel := context.WithCancel(context.Background())
	server := NewServer(listener, shutdownTestHandler{}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		_ = listener.Close()
		<-done
	})

	select {
	case <-conn.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not close the completed response")
	}

	data, abortive, graceful := conn.snapshot()
	var response Response
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatalf("client could not decode completed response after server close: %v", err)
	}
	if abortive || !graceful {
		t.Fatalf("completed response close policy: abortive=%t graceful=%t", abortive, graceful)
	}
	if !response.OK || response.Status == nil {
		t.Fatalf("response=%#v", response)
	}
}

func TestServerShutdown_DoesNotMissAcceptedConnectionDuringAdmission(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	listener := newSingleConnListener(serverConn)
	ctx := newAdmissionRaceContext()
	server := NewServer(listener, shutdownTestHandler{}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	snapshotSentinel := newCompletedResponseConn("")
	server.connections[snapshotSentinel] = struct{}{}
	done := make(chan error, 1)
	serverFinished := false
	go func() { done <- server.Run(ctx) }()
	t.Cleanup(func() {
		ctx.cancel()
		ctx.releaseErrCheck()
		_ = listener.Close()
		_ = clientConn.Close()
		_ = serverConn.Close()
		if !serverFinished {
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Error("control server did not stop during cleanup")
			}
		}
	})

	select {
	case <-ctx.errChecked:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not reach the admission context check")
	}
	ctx.cancel()
	select {
	case <-snapshotSentinel.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not complete its connection snapshot")
	}
	ctx.releaseErrCheck()

	select {
	case err := <-done:
		serverFinished = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("server missed an accepted connection admitted concurrently with shutdown")
	}
}

func TestServerServeConn_ShutdownRespondsBeforeCancellation(t *testing.T) {
	conn := newCompletedResponseConn("{\"version\":1,\"command\":\"shutdown\"}\n")
	listener := newSingleConnListener(conn)
	ctx, cancel := context.WithCancel(context.Background())
	callbackDone := make(chan struct{})
	server := NewServer(listener, shutdownTestHandler{}, slog.New(slog.NewTextHandler(io.Discard, nil)), func() {
		cancel()
		<-conn.closed
		close(callbackDone)
	})
	done := make(chan error, 1)
	serverFinished := false
	go func() { done <- server.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		_ = listener.Close()
		if !serverFinished {
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Error("control server did not stop during cleanup")
			}
		}
	})

	select {
	case <-callbackDone:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown callback did not complete")
	}
	data, abortive, graceful := conn.snapshot()
	var response Response
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatalf("client could not decode shutdown response: %v", err)
	}
	if abortive || !graceful {
		t.Fatalf("shutdown response close policy: abortive=%t graceful=%t", abortive, graceful)
	}
	if !response.OK {
		t.Fatalf("response=%#v", response)
	}

	select {
	case err := <-done:
		serverFinished = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("control server did not stop after shutdown response")
	}
}

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
