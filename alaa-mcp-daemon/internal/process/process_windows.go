//go:build windows

package process

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const (
	createSuspended           = 0x00000004
	createNoWindow            = 0x08000000
	startupUseStdHandles      = 0x00000100
	waitObject0               = 0x00000000
	waitFailed                = 0xFFFFFFFF
	infinite                  = 0xFFFFFFFF
	jobObjectExtendedLimit    = 9
	jobObjectLimitKillOnClose = 0x00002000
)

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW         = kernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject  = kernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")
	procTerminateJobObject       = kernel32.NewProc("TerminateJobObject")
	procResumeThread             = kernel32.NewProc("ResumeThread")
	procWaitForSingleObject      = kernel32.NewProc("WaitForSingleObject")
	procGetExitCodeProcess       = kernel32.NewProc("GetExitCodeProcess")
	procGenerateConsoleCtrlEvent = kernel32.NewProc("GenerateConsoleCtrlEvent")
	procSetPriorityClass         = kernel32.NewProc("SetPriorityClass")
)

type jobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type jobObjectExtendedLimitInformation struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

func (DefaultLauncher) Start(spec Spec) (Handle, error) {
	program, err := resolveProgram(spec.Program)
	if err != nil {
		return nil, err
	}
	stdoutR, stdoutW, err := inheritablePipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}
	defer func() {
		if stdoutR != 0 {
			_ = syscall.CloseHandle(stdoutR)
		}
		if stdoutW != 0 {
			_ = syscall.CloseHandle(stdoutW)
		}
	}()
	stderrR, stderrW, err := inheritablePipe()
	if err != nil {
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}
	defer func() {
		if stderrR != 0 {
			_ = syscall.CloseHandle(stderrR)
		}
		if stderrW != 0 {
			_ = syscall.CloseHandle(stderrW)
		}
	}()
	stdin, err := inheritableNullInput()
	if err != nil {
		return nil, fmt.Errorf("open NUL for stdin: %w", err)
	}
	defer func() {
		if stdin != 0 {
			_ = syscall.CloseHandle(stdin)
		}
	}()

	job, err := createKillOnCloseJob()
	if err != nil {
		return nil, err
	}
	cleanupJob := true
	defer func() {
		if cleanupJob {
			_ = syscall.CloseHandle(job)
		}
	}()

	commandLine := makeCommandLine(append([]string{program}, spec.Args...))
	commandPtr, err := syscall.UTF16PtrFromString(commandLine)
	if err != nil {
		return nil, fmt.Errorf("encode command line: %w", err)
	}
	programPtr, err := syscall.UTF16PtrFromString(program)
	if err != nil {
		return nil, fmt.Errorf("encode executable path: %w", err)
	}
	cwdPtr, err := syscall.UTF16PtrFromString(spec.Cwd)
	if err != nil {
		return nil, fmt.Errorf("encode working directory: %w", err)
	}
	envBlock, err := windowsEnvBlock(mergedEnv(spec.Env))
	if err != nil {
		return nil, fmt.Errorf("encode environment: %w", err)
	}
	startup := &syscall.StartupInfo{
		Cb:        uint32(unsafe.Sizeof(syscall.StartupInfo{})),
		Flags:     startupUseStdHandles,
		StdInput:  stdin,
		StdOutput: stdoutW,
		StdErr:    stderrW,
	}
	var info syscall.ProcessInformation
	flags := uint32(createSuspended | syscall.CREATE_NEW_PROCESS_GROUP | syscall.CREATE_UNICODE_ENVIRONMENT | createNoWindow)
	if err := syscall.CreateProcess(programPtr, commandPtr, nil, nil, true, flags, &envBlock[0], cwdPtr, startup, &info); err != nil {
		return nil, fmt.Errorf("CreateProcessW: %w", err)
	}
	processCreated := true
	defer func() {
		if info.Thread != 0 {
			_ = syscall.CloseHandle(info.Thread)
		}
		if processCreated && info.Process != 0 {
			_ = syscall.CloseHandle(info.Process)
		}
	}()

	if err := assignProcess(job, info.Process); err != nil {
		_ = syscall.TerminateProcess(info.Process, 1)
		_ = terminateJob(job, 1)
		return nil, err
	}
	if err := setPriority(info.Process, spec.Priority); err != nil {
		_ = syscall.TerminateProcess(info.Process, 1)
		_ = terminateJob(job, 1)
		return nil, err
	}
	resumeResult, _, callErr := procResumeThread.Call(uintptr(info.Thread))
	if uint32(resumeResult) == 0xFFFFFFFF {
		_ = syscall.TerminateProcess(info.Process, 1)
		_ = terminateJob(job, 1)
		return nil, fmt.Errorf("ResumeThread: %w", callErr)
	}
	_ = syscall.CloseHandle(info.Thread)
	info.Thread = 0
	_ = syscall.CloseHandle(stdoutW)
	stdoutW = 0
	_ = syscall.CloseHandle(stderrW)
	stderrW = 0
	_ = syscall.CloseHandle(stdin)
	stdin = 0

	stdoutFile := os.NewFile(uintptr(stdoutR), "stdout-pipe")
	stderrFile := os.NewFile(uintptr(stderrR), "stderr-pipe")
	stdoutR, stderrR = 0, 0
	h := &windowsHandle{
		pid:          int(info.ProcessId),
		process:      info.Process,
		job:          job,
		stdout:       stdoutFile,
		stderr:       stderrFile,
		stdoutTarget: spec.Stdout,
		stderrTarget: spec.Stderr,
		done:         make(chan struct{}),
	}
	cleanupJob = false
	processCreated = false
	info.Process = 0
	go h.wait()
	return h, nil
}

func resolveProgram(program string) (string, error) {
	resolved := program
	if !filepath.IsAbs(program) {
		var err error
		resolved, err = exec.LookPath(program)
		if err != nil {
			return "", fmt.Errorf("resolve executable %q: %w", program, err)
		}
	}
	resolved, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat executable %q: %w", resolved, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("executable %q is a directory", resolved)
	}
	return resolved, nil
}

func inheritablePipe() (syscall.Handle, syscall.Handle, error) {
	security := syscall.SecurityAttributes{Length: uint32(unsafe.Sizeof(syscall.SecurityAttributes{})), InheritHandle: 1}
	var read, write syscall.Handle
	if err := syscall.CreatePipe(&read, &write, &security, 0); err != nil {
		return 0, 0, err
	}
	if err := syscall.SetHandleInformation(read, syscall.HANDLE_FLAG_INHERIT, 0); err != nil {
		_ = syscall.CloseHandle(read)
		_ = syscall.CloseHandle(write)
		return 0, 0, err
	}
	return read, write, nil
}

func inheritableNullInput() (syscall.Handle, error) {
	name, _ := syscall.UTF16PtrFromString("NUL")
	security := syscall.SecurityAttributes{Length: uint32(unsafe.Sizeof(syscall.SecurityAttributes{})), InheritHandle: 1}
	return syscall.CreateFile(name, syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, &security, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
}

func makeCommandLine(args []string) string {
	escaped := make([]string, len(args))
	for i, arg := range args {
		escaped[i] = syscall.EscapeArg(arg)
	}
	return strings.Join(escaped, " ")
}

func windowsEnvBlock(env []string) ([]uint16, error) {
	block := make([]uint16, 0, 4096)
	for _, item := range env {
		if strings.ContainsRune(item, '\x00') {
			return nil, errors.New("environment contains NUL")
		}
		block = append(block, utf16.Encode([]rune(item))...)
		block = append(block, 0)
	}
	block = append(block, 0)
	if len(env) == 0 {
		block = append(block, 0)
	}
	return block, nil
}

func createKillOnCloseJob() (syscall.Handle, error) {
	r1, _, callErr := procCreateJobObjectW.Call(0, 0)
	if r1 == 0 {
		return 0, fmt.Errorf("CreateJobObjectW: %w", callErr)
	}
	job := syscall.Handle(r1)
	info := jobObjectExtendedLimitInformation{}
	info.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnClose
	r1, _, callErr = procSetInformationJobObject.Call(
		uintptr(job),
		jobObjectExtendedLimit,
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
	)
	if r1 == 0 {
		_ = syscall.CloseHandle(job)
		return 0, fmt.Errorf("SetInformationJobObject: %w", callErr)
	}
	return job, nil
}

func assignProcess(job, process syscall.Handle) error {
	r1, _, callErr := procAssignProcessToJobObject.Call(uintptr(job), uintptr(process))
	if r1 == 0 {
		return fmt.Errorf("AssignProcessToJobObject: %w", callErr)
	}
	return nil
}

func terminateJob(job syscall.Handle, code uint32) error {
	r1, _, callErr := procTerminateJobObject.Call(uintptr(job), uintptr(code))
	if r1 == 0 {
		return fmt.Errorf("TerminateJobObject: %w", callErr)
	}
	return nil
}

func setPriority(process syscall.Handle, priority string) error {
	classes := map[string]uintptr{
		"idle": 0x00000040, "below_normal": 0x00004000, "normal": 0x00000020,
		"above_normal": 0x00008000, "high": 0x00000080,
	}
	class, ok := classes[priority]
	if !ok {
		return fmt.Errorf("unsupported priority %q", priority)
	}
	r1, _, callErr := procSetPriorityClass.Call(uintptr(process), class)
	if r1 == 0 {
		return fmt.Errorf("SetPriorityClass: %w", callErr)
	}
	return nil
}

type windowsHandle struct {
	pid          int
	process      syscall.Handle
	job          syscall.Handle
	stdout       *os.File
	stderr       *os.File
	stdoutTarget io.Writer
	stderrTarget io.Writer
	done         chan struct{}
	mu           sync.RWMutex
	result       Result
	closeOnce    sync.Once
}

func (h *windowsHandle) PID() int              { return h.pid }
func (h *windowsHandle) Done() <-chan struct{} { return h.done }
func (h *windowsHandle) Result() Result        { h.mu.RLock(); defer h.mu.RUnlock(); return h.result }

func (h *windowsHandle) wait() {
	var copies sync.WaitGroup
	copies.Add(2)
	go func() {
		defer copies.Done()
		if h.stdoutTarget != nil {
			_, _ = io.Copy(h.stdoutTarget, h.stdout)
		}
		_ = h.stdout.Close()
	}()
	go func() {
		defer copies.Done()
		if h.stderrTarget != nil {
			_, _ = io.Copy(h.stderrTarget, h.stderr)
		}
		_ = h.stderr.Close()
	}()

	waitResult, _, waitErr := procWaitForSingleObject.Call(uintptr(h.process), infinite)
	var result Result
	result.ExitedAt = time.Now().UTC()
	if uint32(waitResult) == waitFailed {
		result.ExitCode = -1
		result.Err = fmt.Errorf("WaitForSingleObject: %w", waitErr)
	} else if uint32(waitResult) != waitObject0 {
		result.ExitCode = -1
		result.Err = fmt.Errorf("WaitForSingleObject returned %d", waitResult)
	} else {
		var code uint32
		r1, _, callErr := procGetExitCodeProcess.Call(uintptr(h.process), uintptr(unsafe.Pointer(&code)))
		if r1 == 0 {
			result.ExitCode = -1
			result.Err = fmt.Errorf("GetExitCodeProcess: %w", callErr)
		} else {
			result.ExitCode = int(int32(code))
			if code != 0 {
				result.Err = fmt.Errorf("process exited with code %d", code)
			}
		}
	}
	h.closeKernelHandles()
	copies.Wait()
	h.mu.Lock()
	h.result = result
	h.mu.Unlock()
	close(h.done)
}

func (h *windowsHandle) closeKernelHandles() {
	h.closeOnce.Do(func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		// Closing the last job handle enforces KILL_ON_JOB_CLOSE for descendants.
		if h.job != 0 {
			_ = syscall.CloseHandle(h.job)
			h.job = 0
		}
		if h.process != 0 {
			_ = syscall.CloseHandle(h.process)
			h.process = 0
		}
	})
}

func (h *windowsHandle) Stop(grace time.Duration) error {
	select {
	case <-h.done:
		return nil
	default:
	}
	// Best effort: a detached process may not share a console with the daemon.
	// Do not spend the grace period waiting when Windows rejects the signal.
	signaled, _, _ := procGenerateConsoleCtrlEvent.Call(syscall.CTRL_BREAK_EVENT, uintptr(uint32(h.pid)))
	if signaled != 0 && grace > 0 {
		timer := time.NewTimer(grace)
		defer timer.Stop()
		select {
		case <-h.done:
			return nil
		case <-timer.C:
		}
	}
	return h.Kill()
}

func (h *windowsHandle) Kill() error {
	select {
	case <-h.done:
		return nil
	default:
	}
	h.mu.RLock()
	job := h.job
	if job == 0 {
		h.mu.RUnlock()
		return nil
	}
	err := terminateJob(job, 1)
	h.mu.RUnlock()
	if err != nil {
		return err
	}
	select {
	case <-h.done:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("process tree did not exit after TerminateJobObject")
	}
}
