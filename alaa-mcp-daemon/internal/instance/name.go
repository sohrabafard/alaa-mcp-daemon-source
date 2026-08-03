package instance

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
)

func Key(configPath string) string {
	clean := strings.ToLower(filepath.Clean(configPath))
	sum := sha256.Sum256([]byte(clean))
	return hex.EncodeToString(sum[:8])
}

type Lock interface{ Close() error }
