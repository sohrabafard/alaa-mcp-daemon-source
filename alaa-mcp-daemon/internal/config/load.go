package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxConfigBytes = 1 << 20

var (
	serviceIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)
	varPattern       = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
)

func Load(path string) (*Effective, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", abs, err)
	}
	if len(data) > maxConfigBytes {
		return nil, fmt.Errorf("config exceeds %d bytes", maxConfigBytes)
	}
	if err := rejectDuplicateObjectKeys(data); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	if err := requireTopLevelFields(data); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	var raw File
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("decode config: trailing JSON value")
		}
		return nil, fmt.Errorf("decode config trailing data: %w", err)
	}
	return normalize(abs, data, raw)
}

func requireTopLevelFields(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, name := range []string{"version", "runtime", "commands", "services"} {
		if _, ok := fields[name]; !ok {
			return fmt.Errorf("required top-level field %q is missing", name)
		}
	}
	return nil
}

func normalize(path string, rawData []byte, raw File) (*Effective, error) {
	if raw.Version != 1 {
		return nil, fmt.Errorf("version must be 1, got %d", raw.Version)
	}
	base := filepath.Dir(path)
	runtime, err := normalizeRuntime(base, raw.Runtime)
	if err != nil {
		return nil, err
	}
	defaults := normalizeDefaults(raw.Defaults)
	if len(raw.Commands) == 0 {
		return nil, errors.New("commands must contain at least one template")
	}
	for name, command := range raw.Commands {
		if !serviceIDPattern.MatchString(name) {
			return nil, fmt.Errorf("command name %q is invalid", name)
		}
		if strings.TrimSpace(command.Program) == "" {
			return nil, fmt.Errorf("command %q program is required", name)
		}
		if strings.ContainsRune(command.Program, '\x00') {
			return nil, fmt.Errorf("command %q program contains NUL", name)
		}
	}

	effective := &Effective{
		Path:     path,
		BaseDir:  base,
		Runtime:  runtime,
		Services: make(map[string]EffectiveService, len(raw.Services)),
		Order:    make([]string, 0, len(raw.Services)),
		LoadedAt: time.Now().UTC(),
	}
	seenTCP := make(map[string]string)
	seenExclusive := make(map[string]string)
	for index, service := range raw.Services {
		normalized, err := normalizeService(base, runtime, defaults, raw.Commands, service)
		if err != nil {
			return nil, fmt.Errorf("services[%d]: %w", index, err)
		}
		if _, exists := effective.Services[normalized.ID]; exists {
			return nil, fmt.Errorf("duplicate service id %q", normalized.ID)
		}
		for _, claim := range normalized.Claims {
			key := net.JoinHostPort(canonicalHost(claim.Host), strconv.Itoa(claim.Port))
			if owner, exists := seenTCP[key]; exists {
				return nil, fmt.Errorf("tcp claim %s is shared by %q and %q", key, owner, normalized.ID)
			}
			seenTCP[key] = normalized.ID
		}
		for _, key := range normalized.ExclusiveKeys {
			folded := strings.ToLower(strings.TrimSpace(key))
			if owner, exists := seenExclusive[folded]; exists {
				return nil, fmt.Errorf("exclusive key %q is shared by %q and %q", key, owner, normalized.ID)
			}
			seenExclusive[folded] = normalized.ID
		}
		effective.Services[normalized.ID] = normalized
		effective.Order = append(effective.Order, normalized.ID)
	}

	canonical, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("canonicalize config: %w", err)
	}
	sum := sha256.Sum256(canonical)
	effective.Hash = hex.EncodeToString(sum[:])
	_ = rawData // retained in signature to make the size boundary explicit.
	return effective, nil
}

func normalizeRuntime(base string, raw Runtime) (EffectiveRuntime, error) {
	logDir := strings.TrimSpace(raw.LogDir)
	if logDir == "" {
		logDir = "./logs"
	}
	stateDir := strings.TrimSpace(raw.StateDir)
	if stateDir == "" {
		stateDir = "./state"
	}
	var err error
	logDir, err = resolvePath(base, logDir)
	if err != nil {
		return EffectiveRuntime{}, fmt.Errorf("runtime.log_dir: %w", err)
	}
	stateDir, err = resolvePath(base, stateDir)
	if err != nil {
		return EffectiveRuntime{}, fmt.Errorf("runtime.state_dir: %w", err)
	}
	parallel := raw.MaxParallelStarts
	if parallel == 0 {
		parallel = 2
	}
	if parallel < 1 || parallel > 32 {
		return EffectiveRuntime{}, fmt.Errorf("runtime.max_parallel_starts must be between 1 and 32")
	}
	watch := time.Second
	if raw.WatchInterval.IsSet() {
		watch = raw.WatchInterval.Value()
	}
	if watch < 100*time.Millisecond || watch > time.Minute {
		return EffectiveRuntime{}, fmt.Errorf("runtime.watch_interval must be between 100ms and 1m")
	}
	debounce := 250 * time.Millisecond
	if raw.ReloadDebounce.IsSet() {
		debounce = raw.ReloadDebounce.Value()
	}
	if debounce < 10*time.Millisecond || debounce > 10*time.Second {
		return EffectiveRuntime{}, fmt.Errorf("runtime.reload_debounce must be between 10ms and 10s")
	}
	return EffectiveRuntime{
		LogDir:            logDir,
		StateDir:          stateDir,
		MaxParallelStarts: parallel,
		WatchInterval:     watch,
		ReloadDebounce:    debounce,
	}, nil
}

type effectiveDefaults struct {
	restart  EffectiveRestartPolicy
	shutdown EffectiveShutdownPolicy
	logs     EffectiveLogPolicy
}

func normalizeDefaults(raw Defaults) effectiveDefaults {
	return effectiveDefaults{
		restart: normalizeRestart(raw.Restart, EffectiveRestartPolicy{}),
		shutdown: normalizeShutdown(raw.Shutdown, EffectiveShutdownPolicy{
			GracePeriod: 8 * time.Second,
		}),
		logs: normalizeLogs(raw.Logs, EffectiveLogPolicy{}),
	}
}

func normalizeService(base string, runtime EffectiveRuntime, defaults effectiveDefaults, commands map[string]CommandTemplate, raw Service) (EffectiveService, error) {
	if !serviceIDPattern.MatchString(raw.ID) {
		return EffectiveService{}, fmt.Errorf("id %q is invalid", raw.ID)
	}
	template, ok := commands[raw.Command]
	if !ok {
		return EffectiveService{}, fmt.Errorf("service %q references unknown command %q", raw.ID, raw.Command)
	}
	vars := make(map[string]string, len(raw.Vars)+5)
	for key, value := range raw.Vars {
		if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(key) {
			return EffectiveService{}, fmt.Errorf("service %q has invalid variable name %q", raw.ID, key)
		}
		if strings.ContainsRune(value, '\x00') {
			return EffectiveService{}, fmt.Errorf("service %q variable %q contains NUL", raw.ID, key)
		}
		if strings.Contains(value, "${") {
			return EffectiveService{}, fmt.Errorf("service %q variable %q may not reference another variable", raw.ID, key)
		}
		vars[key] = value
	}
	reserved := map[string]string{
		"service_id": raw.ID,
		"config_dir": base,
		"log_dir":    runtime.LogDir,
		"state_dir":  runtime.StateDir,
	}
	for key, value := range reserved {
		if _, exists := vars[key]; exists {
			return EffectiveService{}, fmt.Errorf("service %q may not override reserved variable %q", raw.ID, key)
		}
		vars[key] = value
	}

	program, err := expand(template.Program, vars)
	if err != nil {
		return EffectiveService{}, fmt.Errorf("service %q program: %w", raw.ID, err)
	}
	args := make([]string, len(template.Args))
	for i, arg := range template.Args {
		args[i], err = expand(arg, vars)
		if err != nil {
			return EffectiveService{}, fmt.Errorf("service %q args[%d]: %w", raw.ID, i, err)
		}
		if strings.ContainsRune(args[i], '\x00') {
			return EffectiveService{}, fmt.Errorf("service %q args[%d] contains NUL", raw.ID, i)
		}
	}
	cwd, err := expand(template.Cwd, vars)
	if err != nil {
		return EffectiveService{}, fmt.Errorf("service %q cwd: %w", raw.ID, err)
	}
	if strings.TrimSpace(cwd) == "" {
		cwd = base
	} else {
		cwd, err = resolvePath(base, cwd)
		if err != nil {
			return EffectiveService{}, fmt.Errorf("service %q cwd: %w", raw.ID, err)
		}
	}
	if hasPathSeparator(program) && !filepath.IsAbs(program) {
		program, err = resolvePath(base, program)
		if err != nil {
			return EffectiveService{}, fmt.Errorf("service %q program: %w", raw.ID, err)
		}
	}
	env := make(map[string]string, len(template.Env))
	envKeys := make(map[string]string, len(template.Env))
	for key, value := range template.Env {
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "=\x00") {
			return EffectiveService{}, fmt.Errorf("service %q has invalid environment key %q", raw.ID, key)
		}
		foldedKey := strings.ToUpper(key)
		if previous, exists := envKeys[foldedKey]; exists {
			return EffectiveService{}, fmt.Errorf("service %q environment keys %q and %q collide on Windows", raw.ID, previous, key)
		}
		envKeys[foldedKey] = key
		expanded, err := expand(value, vars)
		if err != nil {
			return EffectiveService{}, fmt.Errorf("service %q env[%q]: %w", raw.ID, key, err)
		}
		if strings.ContainsRune(expanded, '\x00') {
			return EffectiveService{}, fmt.Errorf("service %q env[%q] contains NUL", raw.ID, key)
		}
		env[key] = expanded
	}
	priority := strings.ToLower(strings.TrimSpace(template.Priority))
	if priority == "" {
		priority = "normal"
	}
	switch priority {
	case "idle", "below_normal", "normal", "above_normal", "high":
	default:
		return EffectiveService{}, fmt.Errorf("service %q priority must be idle, below_normal, normal, above_normal, or high", raw.ID)
	}
	enabled := boolOrDefault(raw.Enabled, true)
	autostart := boolOrDefault(raw.Autostart, true)
	if !enabled {
		autostart = false
	}

	claims := make([]TCPClaim, 0, len(raw.Claims))
	for index, claim := range raw.Claims {
		if strings.ToLower(strings.TrimSpace(claim.Type)) != "tcp" {
			return EffectiveService{}, fmt.Errorf("service %q claims[%d].type must be tcp", raw.ID, index)
		}
		host, err := expand(defaultString(claim.Host, "127.0.0.1"), vars)
		if err != nil {
			return EffectiveService{}, fmt.Errorf("service %q claims[%d].host: %w", raw.ID, index, err)
		}
		if err := validateLoopbackHost(host); err != nil {
			return EffectiveService{}, fmt.Errorf("service %q claims[%d].host: %w", raw.ID, index, err)
		}
		portRaw, err := expand(claim.Port, vars)
		if err != nil {
			return EffectiveService{}, fmt.Errorf("service %q claims[%d].port: %w", raw.ID, index, err)
		}
		port, err := parsePort(portRaw)
		if err != nil {
			return EffectiveService{}, fmt.Errorf("service %q claims[%d].port: %w", raw.ID, index, err)
		}
		claims = append(claims, TCPClaim{Host: canonicalHost(host), Port: port})
	}

	exclusiveKeys := make([]string, 0, len(raw.ExclusiveKeys))
	localKeys := make(map[string]struct{})
	for index, keyTemplate := range raw.ExclusiveKeys {
		key, err := expand(keyTemplate, vars)
		if err != nil {
			return EffectiveService{}, fmt.Errorf("service %q exclusive_keys[%d]: %w", raw.ID, index, err)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return EffectiveService{}, fmt.Errorf("service %q exclusive_keys[%d] is empty", raw.ID, index)
		}
		folded := strings.ToLower(key)
		if _, exists := localKeys[folded]; exists {
			return EffectiveService{}, fmt.Errorf("service %q repeats exclusive key %q", raw.ID, key)
		}
		localKeys[folded] = struct{}{}
		exclusiveKeys = append(exclusiveKeys, key)
	}

	readiness, err := normalizeProbe(raw.ID, raw.Readiness, vars)
	if err != nil {
		return EffectiveService{}, err
	}
	restart := defaults.restart
	if raw.Restart != nil {
		restart = normalizeRestart(*raw.Restart, restart)
	}
	if err := validateRestart(raw.ID, restart); err != nil {
		return EffectiveService{}, err
	}
	shutdown := defaults.shutdown
	if raw.Shutdown != nil {
		shutdown = normalizeShutdown(*raw.Shutdown, shutdown)
	}
	if shutdown.GracePeriod < 0 || shutdown.GracePeriod > 5*time.Minute {
		return EffectiveService{}, fmt.Errorf("service %q shutdown.grace_period must be between 0 and 5m", raw.ID)
	}
	logs := defaults.logs
	if raw.Logs != nil {
		logs = normalizeLogs(*raw.Logs, logs)
	}
	if logs.MaxBytes < 64*1024 || logs.MaxBytes > 10*1024*1024*1024 {
		return EffectiveService{}, fmt.Errorf("service %q logs.max_bytes must be between 64KiB and 10GiB", raw.ID)
	}
	if logs.Backups < 1 || logs.Backups > 100 {
		return EffectiveService{}, fmt.Errorf("service %q logs.backups must be between 1 and 100", raw.ID)
	}

	out := EffectiveService{
		ID: raw.ID, Description: strings.TrimSpace(raw.Description), Program: program, Args: args,
		Cwd: cwd, Env: env, Priority: priority, Enabled: enabled, Autostart: autostart,
		Claims: claims, ExclusiveKeys: exclusiveKeys, Readiness: readiness, Restart: restart,
		Shutdown: shutdown, Logs: logs,
	}
	out.ProcessHash = hashValue(struct {
		Program   string
		Args      []string
		Cwd       string
		Env       map[string]string
		Priority  string
		Claims    []TCPClaim
		Exclusive []string
	}{program, args, cwd, env, priority, claims, exclusiveKeys})
	out.SupervisionHash = hashValue(struct {
		Enabled   bool
		Autostart bool
		Readiness EffectiveProbe
		Restart   EffectiveRestartPolicy
		Shutdown  EffectiveShutdownPolicy
	}{enabled, autostart, readiness, restart, shutdown})
	out.MetadataHash = hashValue(struct {
		Description string
		Logs        EffectiveLogPolicy
	}{out.Description, logs})
	return out, nil
}

func normalizeProbe(serviceID string, raw *Probe, vars map[string]string) (EffectiveProbe, error) {
	probe := Probe{Type: "process"}
	if raw != nil {
		probe = *raw
	}
	typ := strings.ToLower(strings.TrimSpace(probe.Type))
	if typ == "" {
		typ = "process"
	}
	startup := 30 * time.Second
	if probe.StartupTimeout.IsSet() {
		startup = probe.StartupTimeout.Value()
	}
	interval := 10 * time.Second
	if probe.Interval.IsSet() {
		interval = probe.Interval.Value()
	}
	timeout := 2 * time.Second
	if probe.Timeout.IsSet() {
		timeout = probe.Timeout.Value()
	}
	threshold := probe.FailureThreshold
	if threshold == 0 {
		threshold = 3
	}
	if startup <= 0 || startup > 30*time.Minute {
		return EffectiveProbe{}, fmt.Errorf("service %q readiness.startup_timeout must be between 1ns and 30m", serviceID)
	}
	if interval < 100*time.Millisecond || interval > 10*time.Minute {
		return EffectiveProbe{}, fmt.Errorf("service %q readiness.interval must be between 100ms and 10m", serviceID)
	}
	if timeout <= 0 || timeout > interval {
		return EffectiveProbe{}, fmt.Errorf("service %q readiness.timeout must be positive and not exceed interval", serviceID)
	}
	if threshold < 1 || threshold > 100 {
		return EffectiveProbe{}, fmt.Errorf("service %q readiness.failure_threshold must be between 1 and 100", serviceID)
	}
	out := EffectiveProbe{Type: typ, StartupTimeout: startup, Interval: interval, Timeout: timeout, FailureThreshold: threshold}
	switch typ {
	case "process":
	case "tcp":
		host, err := expand(defaultString(probe.Host, "127.0.0.1"), vars)
		if err != nil {
			return EffectiveProbe{}, fmt.Errorf("service %q readiness.host: %w", serviceID, err)
		}
		if err := validateLoopbackHost(host); err != nil {
			return EffectiveProbe{}, fmt.Errorf("service %q readiness.host: %w", serviceID, err)
		}
		portRaw, err := expand(probe.Port, vars)
		if err != nil {
			return EffectiveProbe{}, fmt.Errorf("service %q readiness.port: %w", serviceID, err)
		}
		port, err := parsePort(portRaw)
		if err != nil {
			return EffectiveProbe{}, fmt.Errorf("service %q readiness.port: %w", serviceID, err)
		}
		out.Host, out.Port = canonicalHost(host), port
	case "http":
		rawURL, err := expand(probe.URL, vars)
		if err != nil {
			return EffectiveProbe{}, fmt.Errorf("service %q readiness.url: %w", serviceID, err)
		}
		u, err := url.Parse(rawURL)
		if err != nil || u.Scheme == "" || u.Hostname() == "" {
			return EffectiveProbe{}, fmt.Errorf("service %q readiness.url must be an absolute HTTP URL", serviceID)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return EffectiveProbe{}, fmt.Errorf("service %q readiness.url scheme must be http or https", serviceID)
		}
		if u.User != nil {
			return EffectiveProbe{}, fmt.Errorf("service %q readiness.url may not contain user information", serviceID)
		}
		if err := validateLoopbackHost(u.Hostname()); err != nil {
			return EffectiveProbe{}, fmt.Errorf("service %q readiness.url host: %w", serviceID, err)
		}
		canonical := canonicalHost(u.Hostname())
		if port := u.Port(); port != "" {
			u.Host = net.JoinHostPort(canonical, port)
		} else if strings.Contains(canonical, ":") {
			u.Host = "[" + canonical + "]"
		} else {
			u.Host = canonical
		}
		method := strings.ToUpper(strings.TrimSpace(probe.Method))
		if method == "" {
			method = "GET"
		}
		if method != "GET" && method != "HEAD" {
			return EffectiveProbe{}, fmt.Errorf("service %q readiness.method must be GET or HEAD", serviceID)
		}
		statuses := probe.AllowedStatuses
		if len(statuses) == 0 {
			statuses = []int{200, 204}
		}
		allowed := make(map[int]struct{}, len(statuses))
		for _, status := range statuses {
			if status < 100 || status > 599 {
				return EffectiveProbe{}, fmt.Errorf("service %q readiness.allowed_statuses contains invalid status %d", serviceID, status)
			}
			allowed[status] = struct{}{}
		}
		out.URL, out.Method, out.AllowedStatuses = u.String(), method, allowed
	default:
		return EffectiveProbe{}, fmt.Errorf("service %q readiness.type must be process, tcp, or http", serviceID)
	}
	return out, nil
}

func normalizeRestart(raw RestartPolicy, base EffectiveRestartPolicy) EffectiveRestartPolicy {
	if base.Policy == "" {
		base.Policy = "always"
		base.MaxAttempts = 5
		base.Window = 60 * time.Second
		base.BackoffInitial = time.Second
		base.BackoffMax = 30 * time.Second
	}
	if strings.TrimSpace(raw.Policy) != "" {
		base.Policy = strings.ToLower(strings.TrimSpace(raw.Policy))
	}
	if raw.MaxAttempts != 0 {
		base.MaxAttempts = raw.MaxAttempts
	}
	if raw.Window.IsSet() {
		base.Window = raw.Window.Value()
	}
	if raw.BackoffInitial.IsSet() {
		base.BackoffInitial = raw.BackoffInitial.Value()
	}
	if raw.BackoffMax.IsSet() {
		base.BackoffMax = raw.BackoffMax.Value()
	}
	return base
}

func validateRestart(serviceID string, p EffectiveRestartPolicy) error {
	switch p.Policy {
	case "always", "on_failure", "never":
	default:
		return fmt.Errorf("service %q restart.policy must be always, on_failure, or never", serviceID)
	}
	if p.MaxAttempts < 1 || p.MaxAttempts > 1000 {
		return fmt.Errorf("service %q restart.max_attempts must be between 1 and 1000", serviceID)
	}
	if p.Window <= 0 || p.Window > 24*time.Hour {
		return fmt.Errorf("service %q restart.window must be between 1ns and 24h", serviceID)
	}
	if p.BackoffInitial <= 0 || p.BackoffInitial > 10*time.Minute {
		return fmt.Errorf("service %q restart.backoff_initial must be between 1ns and 10m", serviceID)
	}
	if p.BackoffMax < p.BackoffInitial || p.BackoffMax > time.Hour {
		return fmt.Errorf("service %q restart.backoff_max must be at least backoff_initial and no more than 1h", serviceID)
	}
	return nil
}

func normalizeShutdown(raw ShutdownPolicy, base EffectiveShutdownPolicy) EffectiveShutdownPolicy {
	if raw.GracePeriod.IsSet() {
		base.GracePeriod = raw.GracePeriod.Value()
	}
	return base
}

func normalizeLogs(raw LogPolicy, base EffectiveLogPolicy) EffectiveLogPolicy {
	if base.MaxBytes == 0 {
		base.MaxBytes = 10 * 1024 * 1024
	}
	if base.Backups == 0 {
		base.Backups = 5
	}
	if raw.MaxBytes != 0 {
		base.MaxBytes = raw.MaxBytes
	}
	if raw.Backups != 0 {
		base.Backups = raw.Backups
	}
	return base
}

func expand(input string, vars map[string]string) (string, error) {
	var missing []string
	out := varPattern.ReplaceAllStringFunc(input, func(match string) string {
		parts := varPattern.FindStringSubmatch(match)
		value, ok := vars[parts[1]]
		if !ok {
			missing = append(missing, parts[1])
			return match
		}
		return value
	})
	if len(missing) > 0 {
		sort.Strings(missing)
		return "", fmt.Errorf("missing variable(s): %s", strings.Join(unique(missing), ", "))
	}
	if strings.Contains(out, "${") {
		return "", errors.New("contains malformed or unresolved variable expression")
	}
	return out, nil
}

func parsePort(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, errors.New("port is required")
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("must be an integer between 1 and 65535, got %q", raw)
	}
	return port, nil
}

func validateLoopbackHost(host string) error {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("must be a loopback address, got %q", host)
	}
	return nil
}

func canonicalHost(host string) string {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if strings.EqualFold(host, "localhost") {
		return "127.0.0.1"
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return strings.ToLower(host)
}

func resolvePath(base, value string) (string, error) {
	if strings.ContainsRune(value, '\x00') {
		return "", errors.New("path contains NUL")
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(base, value)
	}
	return filepath.Clean(value), nil
}

func hasPathSeparator(value string) bool { return strings.ContainsAny(value, `/\\`) }
func boolOrDefault(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}
func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
func unique(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := []string{values[0]}
	for _, v := range values[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}

func hashValue(v any) string {
	data, _ := json.Marshal(v)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
