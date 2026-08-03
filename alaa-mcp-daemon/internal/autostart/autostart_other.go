//go:build !windows

package autostart

import "errors"

var errUnsupported = errors.New("Task Scheduler autostart is supported only on Windows")

func Install(executable, configPath, stateDir string) (string, error) { return "", errUnsupported }
func Uninstall(configPath string) (string, error)                     { return "", errUnsupported }
