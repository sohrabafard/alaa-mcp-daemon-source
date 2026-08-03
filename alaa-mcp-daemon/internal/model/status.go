package model

import "time"

type State string

const (
	StateDisabled  State = "disabled"
	StateStopped   State = "stopped"
	StateStarting  State = "starting"
	StateReady     State = "ready"
	StateUnhealthy State = "unhealthy"
	StateBackoff   State = "backoff"
	StateStopping  State = "stopping"
	StateBlocked   State = "blocked"
	StateFailed    State = "failed"
)

type Endpoint struct {
	Type string `json:"type"`
	Host string `json:"host,omitempty"`
	Port int    `json:"port,omitempty"`
}

type ServiceStatus struct {
	ID               string     `json:"id"`
	Description      string     `json:"description,omitempty"`
	Desired          string     `json:"desired"`
	State            State      `json:"state"`
	PID              int        `json:"pid,omitempty"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	ReadyAt          *time.Time `json:"ready_at,omitempty"`
	Readiness        string     `json:"readiness,omitempty"`
	Claims           []Endpoint `json:"claims,omitempty"`
	RestartCount     int        `json:"restart_count"`
	FailuresInWindow int        `json:"failures_in_window"`
	LastExitCode     *int       `json:"last_exit_code,omitempty"`
	LastError        string     `json:"last_error,omitempty"`
	BackoffUntil     *time.Time `json:"backoff_until,omitempty"`
	Executable       string     `json:"executable,omitempty"`
	ConfigGeneration uint64     `json:"config_generation"`
	ConfigHash       string     `json:"config_hash"`
	ManualHold       bool       `json:"manual_hold"`
}

type DaemonStatus struct {
	ProtocolVersion  int             `json:"protocol_version"`
	DaemonVersion    string          `json:"daemon_version"`
	PID              int             `json:"pid"`
	StartedAt        time.Time       `json:"started_at"`
	ConfigPath       string          `json:"config_path"`
	ConfigGeneration uint64          `json:"config_generation"`
	ConfigHash       string          `json:"config_hash"`
	LastReloadError  string          `json:"last_reload_error,omitempty"`
	Services         []ServiceStatus `json:"services"`
}
