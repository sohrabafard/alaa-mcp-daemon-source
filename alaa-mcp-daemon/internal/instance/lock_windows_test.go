//go:build windows

package instance

import (
	"path/filepath"
	"testing"
)

func TestWindowsMutexRejectsSecondInstance(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	first, err := Acquire(configPath, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(configPath, t.TempDir()); err == nil {
		_ = first.Close()
		t.Fatal("expected second lock to fail")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	third, err := Acquire(configPath, t.TempDir())
	if err != nil {
		t.Fatalf("mutex was not released: %v", err)
	}
	_ = third.Close()
}
