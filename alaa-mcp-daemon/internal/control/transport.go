package control

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"path/filepath"
	"strings"
	"time"
)

type Listener interface {
	Accept() (io.ReadWriteCloser, error)
	Close() error
}

func endpointKey(configPath string) string {
	clean := strings.ToLower(filepath.Clean(configPath))
	sum := sha256.Sum256([]byte(clean))
	return hex.EncodeToString(sum[:8])
}

type DialOptions struct{ Timeout time.Duration }
