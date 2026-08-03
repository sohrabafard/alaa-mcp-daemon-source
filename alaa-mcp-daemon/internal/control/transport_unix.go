//go:build !windows

package control

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

type unixListener struct {
	listener *net.UnixListener
	path     string
}

func Listen(configPath, stateDir string) (Listener, string, error) {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, "", err
	}
	path := filepath.Join(stateDir, "control-"+endpointKey(configPath)+".sock")
	_ = os.Remove(path)
	addr := &net.UnixAddr{Name: path, Net: "unix"}
	listener, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, "", fmt.Errorf("listen on Unix control socket: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, "", err
	}
	return &unixListener{listener: listener, path: path}, path, nil
}

func (l *unixListener) Accept() (io.ReadWriteCloser, error) { return l.listener.AcceptUnix() }
func (l *unixListener) Close() error                        { err := l.listener.Close(); _ = os.Remove(l.path); return err }

func dial(configPath, stateDir string, options DialOptions) (io.ReadWriteCloser, error) {
	path := filepath.Join(stateDir, "control-"+endpointKey(configPath)+".sock")
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	conn, err := net.DialTimeout("unix", path, timeout)
	if err != nil {
		return nil, fmt.Errorf("connect to daemon at %q: %w", path, err)
	}
	return conn, nil
}
