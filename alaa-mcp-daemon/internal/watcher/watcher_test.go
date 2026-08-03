package watcher

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatcherDetectsAtomicReplacementAndRetries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	var attempts atomic.Int32
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watch := Watcher{Path: path, Interval: 10 * time.Millisecond, Debounce: 10 * time.Millisecond, Reload: func() error {
		if attempts.Add(1) < 3 {
			return os.ErrInvalid
		}
		select {
		case <-done:
		default:
			close(done)
		}
		return nil
	}}
	go func() { _ = watch.Run(ctx) }()
	time.Sleep(30 * time.Millisecond)
	temp := filepath.Join(dir, "replacement")
	if err := os.WriteFile(temp, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temp, path); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("reload was not observed")
	}
	if got := attempts.Load(); got != 3 {
		t.Fatalf("attempts = %d", got)
	}
}
