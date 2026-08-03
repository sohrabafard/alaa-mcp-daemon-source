//go:build windows

package control

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"alaa-mcp-daemon/internal/winapi"
)

const (
	pipeAccessDuplex        = 0x00000003
	pipeTypeByte            = 0x00000000
	pipeReadModeByte        = 0x00000000
	pipeWait                = 0x00000000
	pipeRejectRemoteClients = 0x00000008
	pipeUnlimitedInstances  = 255
	openExisting            = 3
	errorSemTimeout         = syscall.Errno(121)
	errorPipeBusy           = syscall.Errno(231)
	errorPipeConnected      = syscall.Errno(535)
)

var (
	pipeKernel32            = syscall.NewLazyDLL("kernel32.dll")
	procCreateNamedPipeW    = pipeKernel32.NewProc("CreateNamedPipeW")
	procConnectNamedPipe    = pipeKernel32.NewProc("ConnectNamedPipe")
	procDisconnectNamedPipe = pipeKernel32.NewProc("DisconnectNamedPipe")
	procWaitNamedPipeW      = pipeKernel32.NewProc("WaitNamedPipeW")
)

type namedPipeListener struct {
	name      string
	namePtr   *uint16
	security  *syscall.SecurityAttributes
	cleanup   func()
	mu        sync.Mutex
	cond      *sync.Cond
	closed    bool
	accepting int
	closeOnce sync.Once
}

type pipeConn struct {
	file   *os.File
	server bool
}

func Listen(configPath, stateDir string) (Listener, string, error) {
	_ = stateDir
	name := `\\.\pipe\alaa-mcp-daemon-` + endpointKey(configPath)
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, "", err
	}
	security, cleanup, err := winapi.CurrentUserSecurityAttributes()
	if err != nil {
		return nil, "", fmt.Errorf("build named-pipe ACL: %w", err)
	}
	listener := &namedPipeListener{name: name, namePtr: namePtr, security: security, cleanup: cleanup}
	listener.cond = sync.NewCond(&listener.mu)
	return listener, name, nil
}

func (l *namedPipeListener) Accept() (io.ReadWriteCloser, error) {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil, os.ErrClosed
	}
	l.accepting++
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		l.accepting--
		l.cond.Broadcast()
		l.mu.Unlock()
	}()

	handle, err := createPipeInstance(l.namePtr, l.security)
	if err != nil {
		return nil, err
	}
	r1, _, callErr := procConnectNamedPipe.Call(uintptr(handle), 0)
	if r1 == 0 && callErr != errorPipeConnected {
		_ = syscall.CloseHandle(handle)
		return nil, fmt.Errorf("ConnectNamedPipe: %w", callErr)
	}
	l.mu.Lock()
	closed := l.closed
	l.mu.Unlock()
	if closed {
		_, _, _ = procDisconnectNamedPipe.Call(uintptr(handle))
		_ = syscall.CloseHandle(handle)
		return nil, os.ErrClosed
	}
	return &pipeConn{file: os.NewFile(uintptr(handle), l.name), server: true}, nil
}

func (l *namedPipeListener) Close() error {
	l.closeOnce.Do(func() {
		l.mu.Lock()
		l.closed = true
		l.mu.Unlock()
		// Connect once to release an Accept blocked in ConnectNamedPipe.
		if conn, err := openNamedPipe(l.namePtr, 250*time.Millisecond); err == nil {
			_ = conn.Close()
		}
		l.mu.Lock()
		for l.accepting > 0 {
			l.cond.Wait()
		}
		l.mu.Unlock()
		if l.cleanup != nil {
			l.cleanup()
		}
	})
	return nil
}

func (c *pipeConn) Read(p []byte) (int, error)  { return c.file.Read(p) }
func (c *pipeConn) Write(p []byte) (int, error) { return c.file.Write(p) }
func (c *pipeConn) Close() error {
	if c == nil || c.file == nil {
		return nil
	}
	if c.server {
		_, _, _ = procDisconnectNamedPipe.Call(c.file.Fd())
	}
	return c.file.Close()
}

func createPipeInstance(name *uint16, security *syscall.SecurityAttributes) (syscall.Handle, error) {
	r1, _, callErr := procCreateNamedPipeW.Call(
		uintptr(unsafe.Pointer(name)), pipeAccessDuplex,
		pipeTypeByte|pipeReadModeByte|pipeWait|pipeRejectRemoteClients,
		pipeUnlimitedInstances, 64*1024, 64*1024, 0, uintptr(unsafe.Pointer(security)),
	)
	handle := syscall.Handle(r1)
	if handle == syscall.InvalidHandle || handle == 0 {
		return 0, fmt.Errorf("CreateNamedPipeW: %w", callErr)
	}
	return handle, nil
}

func dial(configPath, stateDir string, options DialOptions) (io.ReadWriteCloser, error) {
	_ = stateDir
	name := `\\.\pipe\alaa-mcp-daemon-` + endpointKey(configPath)
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	conn, err := openNamedPipe(namePtr, options.Timeout)
	if err != nil {
		return nil, fmt.Errorf("connect to daemon at %q: %w", name, err)
	}
	return conn, nil
}

func openNamedPipe(name *uint16, timeout time.Duration) (io.ReadWriteCloser, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		handle, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, openExisting, syscall.FILE_ATTRIBUTE_NORMAL, 0)
		if err == nil {
			return &pipeConn{file: os.NewFile(uintptr(handle), "named-pipe")}, nil
		}
		if !errors.Is(err, errorPipeBusy) && !errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
			return nil, err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, fmt.Errorf("timed out after %s", timeout)
		}
		waitMS := uint32(100)
		if remaining < 100*time.Millisecond {
			waitMS = uint32(remaining.Milliseconds())
			if waitMS == 0 {
				waitMS = 1
			}
		}
		r1, _, waitErr := procWaitNamedPipeW.Call(uintptr(unsafe.Pointer(name)), uintptr(waitMS))
		if r1 == 0 && waitErr != syscall.ERROR_FILE_NOT_FOUND && waitErr != errorSemTimeout && waitErr != errorPipeBusy {
			return nil, waitErr
		}
		// WaitNamedPipe returns immediately while no instance exists. Bound the
		// retry rate so a stopped daemon cannot cause a client-side busy loop.
		if r1 == 0 && waitErr == syscall.ERROR_FILE_NOT_FOUND {
			delay := 25 * time.Millisecond
			if remaining < delay {
				delay = remaining
			}
			time.Sleep(delay)
		}
	}
}
