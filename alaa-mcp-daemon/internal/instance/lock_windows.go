//go:build windows

package instance

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procCreateMutexW = kernel32.NewProc("CreateMutexW")
)

type mutexLock struct{ handle syscall.Handle }

func Acquire(configPath, stateDir string) (Lock, error) {
	_ = stateDir
	name, err := syscall.UTF16PtrFromString(`Local\alaa-mcp-daemon-` + Key(configPath))
	if err != nil {
		return nil, err
	}
	handle, _, callErr := procCreateMutexW.Call(0, 0, uintptr(unsafePointer(name)))
	if handle == 0 {
		return nil, fmt.Errorf("CreateMutexW: %w", callErr)
	}
	if callErr == syscall.ERROR_ALREADY_EXISTS {
		_ = syscall.CloseHandle(syscall.Handle(handle))
		return nil, errors.New("another daemon instance is already running for this config")
	}
	return &mutexLock{handle: syscall.Handle(handle)}, nil
}

func (l *mutexLock) Close() error {
	if l == nil || l.handle == 0 {
		return nil
	}
	return syscall.CloseHandle(l.handle)
}

func unsafePointer[T any](value *T) uintptr { return uintptr(unsafe.Pointer(value)) }
