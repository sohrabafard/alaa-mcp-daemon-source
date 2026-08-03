package process

import (
	"io"
	"time"
)

type Spec struct {
	Program  string
	Args     []string
	Cwd      string
	Env      map[string]string
	Priority string
	Stdout   io.Writer
	Stderr   io.Writer
}

type Result struct {
	ExitCode int
	Err      error
	ExitedAt time.Time
}

type Handle interface {
	PID() int
	Done() <-chan struct{}
	Result() Result
	Stop(grace time.Duration) error
	Kill() error
}

type Launcher interface {
	Start(spec Spec) (Handle, error)
}

type DefaultLauncher struct{}

func NewLauncher() Launcher { return DefaultLauncher{} }
