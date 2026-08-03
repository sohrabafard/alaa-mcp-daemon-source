package control

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"alaa-mcp-daemon/internal/model"
)

const (
	maxMessageBytes    = 1 << 20
	requestReadTimeout = 15 * time.Second
)

type Handler interface {
	Status(serviceID string) (model.DaemonStatus, error)
	Start(serviceID string) error
	Stop(serviceID string) error
	Restart(serviceID string) error
	Reload() error
	LogPaths(serviceID string) (string, string, error)
}

type Server struct {
	listener    Listener
	handler     Handler
	logger      *slog.Logger
	onShutdown  func()
	wg          sync.WaitGroup
	connMu      sync.Mutex
	connections map[io.ReadWriteCloser]struct{}
}

func NewServer(listener Listener, handler Handler, logger *slog.Logger, onShutdown func()) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		listener: listener, handler: handler, logger: logger, onShutdown: onShutdown,
		connections: make(map[io.ReadWriteCloser]struct{}),
	}
}

func (s *Server) Run(ctx context.Context) error {
	if s.listener == nil || s.handler == nil {
		return errors.New("control server requires listener and handler")
	}
	go func() {
		<-ctx.Done()
		_ = s.listener.Close()
		s.closeConnections()
	}()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			return fmt.Errorf("accept control connection: %w", err)
		}
		if ctx.Err() != nil {
			_ = conn.Close()
			break
		}
		s.trackConnection(conn)
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer s.untrackConnection(conn)
			defer conn.Close()
			s.serveConn(ctx, conn)
		}()
	}
	s.wg.Wait()
	return nil
}

func (s *Server) serveConn(ctx context.Context, conn io.ReadWriteCloser) {
	timedOut := make(chan struct{})
	timer := time.AfterFunc(requestReadTimeout, func() {
		close(timedOut)
		_ = conn.Close()
	})
	reader := bufio.NewReader(io.LimitReader(conn, maxMessageBytes+1))
	line, err := reader.ReadBytes('\n')
	if !timer.Stop() {
		select {
		case <-timedOut:
			return
		default:
		}
	}
	if err != nil && !errors.Is(err, io.EOF) {
		if ctx.Err() != nil {
			return
		}
		s.write(conn, Response{Version: ProtocolVersion, Error: "invalid request: " + err.Error()})
		return
	}
	if len(line) > maxMessageBytes {
		s.write(conn, Response{Version: ProtocolVersion, Error: "invalid request: message exceeds size limit"})
		return
	}
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.DisallowUnknownFields()
	var request Request
	if err := dec.Decode(&request); err != nil {
		s.write(conn, Response{Version: ProtocolVersion, Error: "invalid request: " + err.Error()})
		return
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("trailing JSON value")
		}
		s.write(conn, Response{Version: ProtocolVersion, Error: "invalid request: " + err.Error()})
		return
	}
	s.write(conn, s.handle(request))
}

func (s *Server) handle(request Request) Response {
	response := Response{Version: ProtocolVersion}
	if request.Version != ProtocolVersion {
		response.Error = fmt.Sprintf("unsupported protocol version %d", request.Version)
		return response
	}
	command := strings.ToLower(strings.TrimSpace(request.Command))
	var err error
	switch command {
	case "status":
		var status model.DaemonStatus
		status, err = s.handler.Status(request.Service)
		if err == nil {
			response.Status = &status
		}
	case "start":
		err = requireService(request.Service, s.handler.Start)
	case "stop":
		err = requireService(request.Service, s.handler.Stop)
	case "restart":
		err = requireService(request.Service, s.handler.Restart)
	case "reload":
		err = s.handler.Reload()
	case "log_paths":
		if strings.TrimSpace(request.Service) == "" {
			err = errors.New("service is required")
		} else {
			var stdout, stderr string
			stdout, stderr, err = s.handler.LogPaths(request.Service)
			if err == nil {
				response.Logs = &LogPaths{Stdout: stdout, Stderr: stderr}
			}
		}
	case "shutdown":
		if s.onShutdown == nil {
			err = errors.New("shutdown is unavailable")
		} else {
			s.onShutdown()
		}
	default:
		err = fmt.Errorf("unknown command %q", request.Command)
	}
	if err != nil {
		response.Error = err.Error()
		return response
	}
	response.OK = true
	return response
}

func requireService(service string, fn func(string) error) error {
	if strings.TrimSpace(service) == "" {
		return errors.New("service is required")
	}
	return fn(service)
}

func (s *Server) trackConnection(conn io.ReadWriteCloser) {
	s.connMu.Lock()
	s.connections[conn] = struct{}{}
	s.connMu.Unlock()
}

func (s *Server) untrackConnection(conn io.ReadWriteCloser) {
	s.connMu.Lock()
	delete(s.connections, conn)
	s.connMu.Unlock()
}

func (s *Server) closeConnections() {
	s.connMu.Lock()
	connections := make([]io.ReadWriteCloser, 0, len(s.connections))
	for conn := range s.connections {
		connections = append(connections, conn)
	}
	s.connMu.Unlock()
	for _, conn := range connections {
		_ = conn.Close()
	}
}

func (s *Server) write(w io.Writer, response Response) {
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Warn("write control response", "error", err)
	}
}
