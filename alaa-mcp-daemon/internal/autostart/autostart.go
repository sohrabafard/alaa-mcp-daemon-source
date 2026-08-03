package autostart

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
)

func taskKey(configPath string) string {
	clean := strings.ToLower(filepath.Clean(configPath))
	sum := sha256.Sum256([]byte(clean))
	return hex.EncodeToString(sum[:8])
}

func TaskName(configPath string) string { return "Alaa MCP Daemon " + taskKey(configPath) }
