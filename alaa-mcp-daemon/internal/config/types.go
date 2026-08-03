package config

import "time"

type File struct {
	Schema   string                     `json:"$schema,omitempty"`
	Version  int                        `json:"version"`
	Runtime  Runtime                    `json:"runtime"`
	Defaults Defaults                   `json:"defaults,omitempty"`
	Commands map[string]CommandTemplate `json:"commands"`
	Services []Service                  `json:"services"`
}

type Runtime struct {
	LogDir            string   `json:"log_dir"`
	StateDir          string   `json:"state_dir"`
	MaxParallelStarts int      `json:"max_parallel_starts"`
	WatchInterval     Duration `json:"watch_interval,omitempty"`
	ReloadDebounce    Duration `json:"reload_debounce,omitempty"`
}

type Defaults struct {
	Restart  RestartPolicy  `json:"restart,omitempty"`
	Shutdown ShutdownPolicy `json:"shutdown,omitempty"`
	Logs     LogPolicy      `json:"logs,omitempty"`
}

type CommandTemplate struct {
	Program  string            `json:"program"`
	Args     []string          `json:"args,omitempty"`
	Cwd      string            `json:"cwd,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	Priority string            `json:"priority,omitempty"`
}

type Service struct {
	ID            string            `json:"id"`
	Description   string            `json:"description,omitempty"`
	Command       string            `json:"command"`
	Enabled       *bool             `json:"enabled,omitempty"`
	Autostart     *bool             `json:"autostart,omitempty"`
	Vars          map[string]string `json:"vars,omitempty"`
	Claims        []Claim           `json:"claims,omitempty"`
	ExclusiveKeys []string          `json:"exclusive_keys,omitempty"`
	Readiness     *Probe            `json:"readiness,omitempty"`
	Restart       *RestartPolicy    `json:"restart,omitempty"`
	Shutdown      *ShutdownPolicy   `json:"shutdown,omitempty"`
	Logs          *LogPolicy        `json:"logs,omitempty"`
}

type Claim struct {
	Type string `json:"type"`
	Host string `json:"host,omitempty"`
	Port string `json:"port,omitempty"`
}

type Probe struct {
	Type             string   `json:"type"`
	Host             string   `json:"host,omitempty"`
	Port             string   `json:"port,omitempty"`
	URL              string   `json:"url,omitempty"`
	Method           string   `json:"method,omitempty"`
	AllowedStatuses  []int    `json:"allowed_statuses,omitempty"`
	StartupTimeout   Duration `json:"startup_timeout,omitempty"`
	Interval         Duration `json:"interval,omitempty"`
	Timeout          Duration `json:"timeout,omitempty"`
	FailureThreshold int      `json:"failure_threshold,omitempty"`
}

type RestartPolicy struct {
	Policy         string   `json:"policy,omitempty"`
	MaxAttempts    int      `json:"max_attempts,omitempty"`
	Window         Duration `json:"window,omitempty"`
	BackoffInitial Duration `json:"backoff_initial,omitempty"`
	BackoffMax     Duration `json:"backoff_max,omitempty"`
}

type ShutdownPolicy struct {
	GracePeriod Duration `json:"grace_period,omitempty"`
}

type LogPolicy struct {
	MaxBytes int64 `json:"max_bytes,omitempty"`
	Backups  int   `json:"backups,omitempty"`
}

type Effective struct {
	Path     string
	BaseDir  string
	Runtime  EffectiveRuntime
	Services map[string]EffectiveService
	Order    []string
	Hash     string
	LoadedAt time.Time
}

type EffectiveRuntime struct {
	LogDir            string
	StateDir          string
	MaxParallelStarts int
	WatchInterval     time.Duration
	ReloadDebounce    time.Duration
}

type EffectiveService struct {
	ID              string
	Description     string
	Program         string
	Args            []string
	Cwd             string
	Env             map[string]string
	Priority        string
	Enabled         bool
	Autostart       bool
	Claims          []TCPClaim
	ExclusiveKeys   []string
	Readiness       EffectiveProbe
	Restart         EffectiveRestartPolicy
	Shutdown        EffectiveShutdownPolicy
	Logs            EffectiveLogPolicy
	ProcessHash     string
	SupervisionHash string
	MetadataHash    string
}

type TCPClaim struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type EffectiveProbe struct {
	Type             string
	Host             string
	Port             int
	URL              string
	Method           string
	AllowedStatuses  map[int]struct{}
	StartupTimeout   time.Duration
	Interval         time.Duration
	Timeout          time.Duration
	FailureThreshold int
}

type EffectiveRestartPolicy struct {
	Policy         string
	MaxAttempts    int
	Window         time.Duration
	BackoffInitial time.Duration
	BackoffMax     time.Duration
}

type EffectiveShutdownPolicy struct {
	GracePeriod time.Duration
}

type EffectiveLogPolicy struct {
	MaxBytes int64
	Backups  int
}
