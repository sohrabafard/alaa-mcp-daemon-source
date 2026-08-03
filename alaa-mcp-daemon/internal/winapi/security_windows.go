//go:build windows

package winapi

import (
	"fmt"
	"syscall"
	"unsafe"
)

const sddlRevision1 = 1

var (
	advapi32                                                 = syscall.NewLazyDLL("advapi32.dll")
	kernel32                                                 = syscall.NewLazyDLL("kernel32.dll")
	procConvertStringSecurityDescriptorToSecurityDescriptorW = advapi32.NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW")
	procLocalFree                                            = kernel32.NewProc("LocalFree")
)

func CurrentUserSID() (string, error) {
	process, err := syscall.GetCurrentProcess()
	if err != nil {
		return "", fmt.Errorf("GetCurrentProcess: %w", err)
	}
	var token syscall.Token
	if err := syscall.OpenProcessToken(process, syscall.TOKEN_QUERY, &token); err != nil {
		return "", fmt.Errorf("OpenProcessToken: %w", err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("GetTokenUser: %w", err)
	}
	sid, err := user.User.Sid.String()
	if err != nil {
		return "", fmt.Errorf("format user SID: %w", err)
	}
	return sid, nil
}

func CurrentUserSecurityAttributes() (*syscall.SecurityAttributes, func(), error) {
	sid, err := CurrentUserSID()
	if err != nil {
		return nil, nil, err
	}
	// Protect the object and grant full control only to LocalSystem and the current user.
	sddl := `D:P(A;;GA;;;SY)(A;;GA;;;` + sid + `)`
	sddlPtr, err := syscall.UTF16PtrFromString(sddl)
	if err != nil {
		return nil, nil, err
	}
	var descriptor uintptr
	var size uint32
	r1, _, callErr := procConvertStringSecurityDescriptorToSecurityDescriptorW.Call(
		uintptr(unsafe.Pointer(sddlPtr)), sddlRevision1, uintptr(unsafe.Pointer(&descriptor)), uintptr(unsafe.Pointer(&size)),
	)
	if r1 == 0 {
		return nil, nil, fmt.Errorf("ConvertStringSecurityDescriptorToSecurityDescriptorW: %w", callErr)
	}
	attributes := &syscall.SecurityAttributes{
		Length: uint32(unsafe.Sizeof(syscall.SecurityAttributes{})), SecurityDescriptor: descriptor,
	}
	cleanup := func() {
		if descriptor != 0 {
			_, _, _ = procLocalFree.Call(descriptor)
		}
	}
	return attributes, cleanup, nil
}
