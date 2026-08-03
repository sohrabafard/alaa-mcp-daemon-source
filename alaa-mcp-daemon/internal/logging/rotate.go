package logging

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type RotateWriter struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	backups  int
	file     *os.File
	size     int64
	closed   bool
}

func NewRotateWriter(path string, maxBytes int64, backups int) (*RotateWriter, error) {
	if maxBytes <= 0 {
		return nil, errors.New("maxBytes must be positive")
	}
	if backups < 1 {
		return nil, errors.New("backups must be at least 1")
	}
	w := &RotateWriter{path: path, maxBytes: maxBytes, backups: backups}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *RotateWriter) Path() string { return w.path }

func (w *RotateWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, os.ErrClosed
	}
	if w.file == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	if w.size > 0 && w.size+int64(len(p)) > w.maxBytes {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *RotateWriter) Update(maxBytes int64, backups int) error {
	if maxBytes <= 0 || backups < 1 {
		return errors.New("invalid rotation policy")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return os.ErrClosed
	}
	if err := w.pruneTo(backups); err != nil {
		return err
	}
	w.maxBytes = maxBytes
	w.backups = backups
	return nil
}

func (w *RotateWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	return w.file.Sync()
}

func (w *RotateWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if w.file == nil {
		return nil
	}
	return w.file.Close()
}

func (w *RotateWriter) open() error {
	if err := os.MkdirAll(filepath.Dir(w.path), 0o700); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open log %q: %w", w.path, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("stat log %q: %w", w.path, err)
	}
	w.file = f
	w.size = info.Size()
	return nil
}

func (w *RotateWriter) rotate() error {
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return fmt.Errorf("close log before rotation: %w", err)
		}
		w.file = nil
	}
	if err := w.prune(); err != nil {
		return err
	}
	for i := w.backups - 1; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", w.path, i)
		newPath := fmt.Sprintf("%s.%d", w.path, i+1)
		if err := os.Remove(newPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove rotation destination %q: %w", newPath, err)
		}
		if err := os.Rename(oldPath, newPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("rotate %q to %q: %w", oldPath, newPath, err)
		}
	}
	if err := os.Remove(w.path + ".1"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove first rotation destination: %w", err)
	}
	if err := os.Rename(w.path, w.path+".1"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("rotate active log: %w", err)
	}
	w.size = 0
	return w.open()
}

func (w *RotateWriter) prune() error { return w.pruneTo(w.backups) }

func (w *RotateWriter) pruneTo(backups int) error {
	matches, err := filepath.Glob(w.path + ".*")
	if err != nil {
		return fmt.Errorf("enumerate rotated logs: %w", err)
	}
	prefix := w.path + "."
	for _, path := range matches {
		suffix := strings.TrimPrefix(path, prefix)
		index, parseErr := strconv.Atoi(suffix)
		if parseErr != nil || index <= backups {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove old log %q: %w", path, err)
		}
	}
	return nil
}

var _ io.WriteCloser = (*RotateWriter)(nil)
