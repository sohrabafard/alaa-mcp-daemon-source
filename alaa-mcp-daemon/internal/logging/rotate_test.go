package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotateWriterRotatesAndPrunes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.log")
	writer, err := NewRotateWriter(path, 64*1024, 2)
	if err != nil {
		t.Fatal(err)
	}
	payload := strings.Repeat("x", 40*1024)
	for range 4 {
		if _, err := writer.Write([]byte(payload)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", ".1", ".2"} {
		if _, err := os.Stat(path + suffix); err != nil {
			t.Fatalf("missing %s: %v", suffix, err)
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatalf("unexpected third backup: %v", err)
	}
}
