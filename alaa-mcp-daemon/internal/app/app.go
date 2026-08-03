package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"

	"alaa-mcp-daemon/internal/autostart"
	"alaa-mcp-daemon/internal/buildinfo"
	"alaa-mcp-daemon/internal/config"
	"alaa-mcp-daemon/internal/control"
	"alaa-mcp-daemon/internal/instance"
	"alaa-mcp-daemon/internal/logging"
	"alaa-mcp-daemon/internal/manager"
	"alaa-mcp-daemon/internal/model"
	"alaa-mcp-daemon/internal/tail"
	"alaa-mcp-daemon/internal/watcher"
)

const usageText = `alaa-mcp-daemon supervises local MCP server processes.

Usage:
  alaa-mcp-daemon run [--config PATH]
  alaa-mcp-daemon validate [--config PATH] [--json]
  alaa-mcp-daemon status [--config PATH] [--json] [SERVICE]
  alaa-mcp-daemon start|stop|restart [--config PATH] SERVICE
  alaa-mcp-daemon reload [--config PATH]
  alaa-mcp-daemon logs [--config PATH] [--follow] [--lines N] [--stream both|stdout|stderr] SERVICE
  alaa-mcp-daemon install-autostart [--config PATH]
  alaa-mcp-daemon uninstall-autostart [--config PATH]
  alaa-mcp-daemon shutdown [--config PATH]
  alaa-mcp-daemon version
`

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = io.WriteString(stderr, usageText)
		return 2
	}
	command := strings.ToLower(args[0])
	var err error
	switch command {
	case "run":
		err = runCommand(args[1:], stdout, stderr)
	case "validate":
		err = validateCommand(args[1:], stdout, stderr)
	case "status":
		err = statusCommand(args[1:], stdout, stderr)
	case "start", "stop", "restart":
		err = serviceCommand(command, args[1:], stdout, stderr)
	case "reload", "shutdown":
		err = simpleCommand(command, args[1:], stdout, stderr)
	case "logs":
		err = logsCommand(args[1:], stdout, stderr)
	case "install-autostart":
		err = installAutostartCommand(args[1:], stdout, stderr)
	case "uninstall-autostart":
		err = uninstallAutostartCommand(args[1:], stdout, stderr)
	case "version", "--version", "-version":
		_, err = fmt.Fprintf(stdout, "alaa-mcp-daemon %s (commit %s, built %s)\n", buildinfo.Version, buildinfo.Commit, buildinfo.Date)
	case "help", "--help", "-h":
		_, err = io.WriteString(stdout, usageText)
	default:
		err = fmt.Errorf("unknown command %q\n\n%s", args[0], usageText)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func runCommand(args []string, stdout, stderr io.Writer) error {
	fs, configPath := commandFlags("run", stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("run accepts no positional arguments")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return RunDaemon(ctx, *configPath, stdout)
}

func RunDaemon(parent context.Context, configPath string, stdout io.Writer) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	lock, err := instance.Acquire(cfg.Path, cfg.Runtime.StateDir)
	if err != nil {
		return err
	}
	defer lock.Close()
	logger, err := logging.NewDaemonLogger(filepath.Join(cfg.Runtime.LogDir, "daemon.jsonl"))
	if err != nil {
		return err
	}
	defer logger.Close()
	mgr, err := manager.New(cfg, logger.Slog)
	if err != nil {
		return err
	}
	listener, endpoint, err := control.Listen(cfg.Path, cfg.Runtime.StateDir)
	if err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), mgr.ShutdownTimeout())
		defer cancel()
		_ = mgr.Shutdown(ctx)
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	server := control.NewServer(listener, mgr, logger.Slog, cancel)
	fileWatcher := watcher.Watcher{
		Path: cfg.Path, Interval: cfg.Runtime.WatchInterval, Debounce: cfg.Runtime.ReloadDebounce,
		Settings: func() (time.Duration, time.Duration) {
			runtime := mgr.Runtime()
			return runtime.WatchInterval, runtime.ReloadDebounce
		},
		Reload: mgr.Reload, OnError: mgr.SetReloadError,
	}
	logger.Slog.Info("daemon started", "pid", os.Getpid(), "config", cfg.Path, "control", endpoint, "services", len(cfg.Services))
	_, _ = fmt.Fprintf(stdout, "alaa-mcp-daemon is running (PID %d, control %s)\n", os.Getpid(), endpoint)

	results := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); results <- server.Run(ctx) }()
	go func() { defer wg.Done(); results <- fileWatcher.Run(ctx) }()
	var runErr error
	select {
	case <-ctx.Done():
	case err := <-results:
		if err != nil {
			runErr = err
		}
		cancel()
	}
	cancel()
	_ = listener.Close()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), mgr.ShutdownTimeout())
	shutdownErr := mgr.Shutdown(shutdownCtx)
	shutdownCancel()
	wg.Wait()
	logger.Slog.Info("daemon stopped", "error", errors.Join(runErr, shutdownErr))
	return errors.Join(runErr, shutdownErr)
}

func validateCommand(args []string, stdout, stderr io.Writer) error {
	fs, configPath := commandFlags("validate", stderr)
	jsonOutput := fs.Bool("json", false, "write machine-readable output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("validate accepts no positional arguments")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	result := struct {
		Valid             bool   `json:"valid"`
		Path              string `json:"path"`
		Hash              string `json:"hash"`
		Services          int    `json:"services"`
		MaxParallelStarts int    `json:"max_parallel_starts"`
	}{true, cfg.Path, cfg.Hash, len(cfg.Services), cfg.Runtime.MaxParallelStarts}
	if *jsonOutput {
		return writeJSON(stdout, result)
	}
	_, err = fmt.Fprintf(stdout, "valid: %s\nhash: %s\nservices: %d\nmax parallel starts: %d\n", cfg.Path, cfg.Hash, len(cfg.Services), cfg.Runtime.MaxParallelStarts)
	return err
}

func statusCommand(args []string, stdout, stderr io.Writer) error {
	parsed, err := parseFlexible(args, map[string]bool{"json": false}, map[string]string{})
	if err != nil {
		return err
	}
	configPath := parsed.configPath
	if configPath == "" {
		configPath, err = defaultConfigPath()
		if err != nil {
			return err
		}
	}
	if len(parsed.positionals) > 1 {
		return errors.New("status accepts at most one service id")
	}
	service := ""
	if len(parsed.positionals) == 1 {
		service = parsed.positionals[0]
	}
	resolvedPath, stateDir, err := clientTarget(configPath)
	if err != nil {
		return err
	}
	response, err := control.Call(resolvedPath, stateDir, control.Request{Command: "status", Service: service}, 5*time.Second)
	if err != nil {
		return err
	}
	if response.Status == nil {
		return errors.New("daemon returned no status")
	}
	if parsed.bools["json"] {
		return writeJSON(stdout, response.Status)
	}
	return writeHumanStatus(stdout, *response.Status)
}

func serviceCommand(command string, args []string, stdout, stderr io.Writer) error {
	fs, configPath := commandFlags(command, stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("%s requires exactly one service id", command)
	}
	resolvedPath, stateDir, err := clientTarget(*configPath)
	if err != nil {
		return err
	}
	_, err = control.Call(resolvedPath, stateDir, control.Request{Command: command, Service: fs.Arg(0)}, 10*time.Second)
	if err == nil {
		_, err = fmt.Fprintf(stdout, "%s accepted for %s\n", command, fs.Arg(0))
	}
	return err
}

func simpleCommand(command string, args []string, stdout, stderr io.Writer) error {
	fs, configPath := commandFlags(command, stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("%s accepts no positional arguments", command)
	}
	resolvedPath, stateDir, err := clientTarget(*configPath)
	if err != nil {
		return err
	}
	_, err = control.Call(resolvedPath, stateDir, control.Request{Command: command}, 10*time.Second)
	if err == nil {
		_, err = fmt.Fprintf(stdout, "%s accepted\n", command)
	}
	return err
}

func logsCommand(args []string, stdout, stderr io.Writer) error {
	parsed, err := parseFlexible(args, map[string]bool{"follow": false}, map[string]string{"lines": "100", "stream": "both"})
	if err != nil {
		return err
	}
	if len(parsed.positionals) != 1 {
		return errors.New("logs requires exactly one service id")
	}
	lines, err := strconv.Atoi(parsed.values["lines"])
	if err != nil || lines < 0 || lines > 100000 {
		return errors.New("--lines must be an integer between 0 and 100000")
	}
	stream := strings.ToLower(parsed.values["stream"])
	if stream != "both" && stream != "stdout" && stream != "stderr" {
		return errors.New("--stream must be both, stdout, or stderr")
	}
	configPath := parsed.configPath
	if configPath == "" {
		configPath, err = defaultConfigPath()
		if err != nil {
			return err
		}
	}
	resolvedPath, stateDir, err := clientTarget(configPath)
	if err != nil {
		return err
	}
	response, err := control.Call(resolvedPath, stateDir, control.Request{Command: "log_paths", Service: parsed.positionals[0]}, 5*time.Second)
	if err != nil {
		return err
	}
	if response.Logs == nil {
		return errors.New("daemon returned no log paths")
	}
	var sources []tail.Source
	if stream == "both" || stream == "stdout" {
		sources = append(sources, tail.Source{Label: "stdout", Path: response.Logs.Stdout})
	}
	if stream == "both" || stream == "stderr" {
		sources = append(sources, tail.Source{Label: "stderr", Path: response.Logs.Stderr})
	}
	for _, source := range sources {
		if err := tail.PrintLast(stdout, source, lines); err != nil {
			return err
		}
	}
	if !parsed.bools["follow"] {
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return tail.Follow(ctx, stdout, sources)
}

func installAutostartCommand(args []string, stdout, stderr io.Writer) error {
	fs, configPath := commandFlags("install-autostart", stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("install-autostart accepts no positional arguments")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	taskName, err := autostart.Install(executable, cfg.Path, cfg.Runtime.StateDir)
	if err == nil {
		_, err = fmt.Fprintf(stdout, "installed Task Scheduler entry: %s\n", taskName)
	}
	return err
}

func uninstallAutostartCommand(args []string, stdout, stderr io.Writer) error {
	fs, configPath := commandFlags("uninstall-autostart", stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("uninstall-autostart accepts no positional arguments")
	}
	path, err := filepath.Abs(*configPath)
	if err != nil {
		return err
	}
	taskName, err := autostart.Uninstall(path)
	if err == nil {
		_, err = fmt.Fprintf(stdout, "removed Task Scheduler entry: %s\n", taskName)
	}
	return err
}

func clientTarget(configPath string) (string, string, error) {
	resolvedPath, err := filepath.Abs(configPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve config path: %w", err)
	}
	if cfg, loadErr := config.Load(resolvedPath); loadErr == nil {
		return cfg.Path, cfg.Runtime.StateDir, nil
	}
	stateDir := filepath.Join(filepath.Dir(resolvedPath), "state")
	data, readErr := os.ReadFile(resolvedPath)
	if readErr == nil {
		var partial struct {
			Runtime struct {
				StateDir string `json:"state_dir"`
			} `json:"runtime"`
		}
		if json.Unmarshal(data, &partial) == nil && strings.TrimSpace(partial.Runtime.StateDir) != "" {
			stateDir = partial.Runtime.StateDir
			if !filepath.IsAbs(stateDir) {
				stateDir = filepath.Join(filepath.Dir(resolvedPath), stateDir)
			}
			stateDir = filepath.Clean(stateDir)
		}
	}
	return resolvedPath, stateDir, nil
}

func commandFlags(name string, output io.Writer) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(output)
	path, _ := defaultConfigPath()
	return fs, fs.String("config", path, "path to alaa-mcp-daemon.json")
}

func defaultConfigPath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(executable), "alaa-mcp-daemon.json"), nil
}

type flexibleArgs struct {
	configPath  string
	bools       map[string]bool
	values      map[string]string
	positionals []string
}

func parseFlexible(args []string, boolDefaults map[string]bool, valueDefaults map[string]string) (flexibleArgs, error) {
	out := flexibleArgs{bools: make(map[string]bool), values: make(map[string]string)}
	for key, value := range boolDefaults {
		out.bools[key] = value
	}
	for key, value := range valueDefaults {
		out.values[key] = value
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			out.positionals = append(out.positionals, arg)
			continue
		}
		nameValue := strings.TrimPrefix(arg, "--")
		name, value, hasValue := strings.Cut(nameValue, "=")
		if name == "config" {
			if !hasValue {
				i++
				if i >= len(args) {
					return out, errors.New("--config requires a value")
				}
				value = args[i]
			}
			out.configPath = value
			continue
		}
		if _, ok := out.bools[name]; ok {
			if hasValue {
				parsed, err := strconv.ParseBool(value)
				if err != nil {
					return out, fmt.Errorf("--%s: %w", name, err)
				}
				out.bools[name] = parsed
			} else {
				out.bools[name] = true
			}
			continue
		}
		if _, ok := out.values[name]; ok {
			if !hasValue {
				i++
				if i >= len(args) {
					return out, fmt.Errorf("--%s requires a value", name)
				}
				value = args[i]
			}
			out.values[name] = value
			continue
		}
		return out, fmt.Errorf("unknown option --%s", name)
	}
	return out, nil
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeHumanStatus(w io.Writer, status model.DaemonStatus) error {
	if _, err := fmt.Fprintf(w, "daemon: %s  pid: %d  started: %s\nconfig: %s\ngeneration: %d  hash: %s\n", status.DaemonVersion, status.PID, status.StartedAt.Format(time.RFC3339), status.ConfigPath, status.ConfigGeneration, status.ConfigHash); err != nil {
		return err
	}
	if status.LastReloadError != "" {
		if _, err := fmt.Fprintf(w, "last reload error: %s\n", status.LastReloadError); err != nil {
			return err
		}
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SERVICE\tDESIRED\tSTATE\tPID\tRESTARTS\tREADINESS / LAST ERROR")
	for _, service := range status.Services {
		message := service.Readiness
		if service.LastError != "" {
			message = service.LastError
		}
		pid := "-"
		if service.PID != 0 {
			pid = strconv.Itoa(service.PID)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n", service.ID, service.Desired, service.State, pid, service.RestartCount, message)
	}
	return tw.Flush()
}
