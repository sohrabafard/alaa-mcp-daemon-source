package tail

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLastLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\nfour\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := LastLines(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != "three\nfour" {
		t.Fatalf("got %q", got)
	}
}
