package watcher

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"time"
)

type ReloadFunc func() error
type ErrorFunc func(error)
type SettingsFunc func() (interval, debounce time.Duration)

type Watcher struct {
	Path     string
	Interval time.Duration
	Debounce time.Duration
	Settings SettingsFunc
	Reload   ReloadFunc
	OnError  ErrorFunc
}

func (w Watcher) Run(ctx context.Context) error {
	if w.Path == "" {
		return errors.New("watch path is required")
	}
	if w.Reload == nil {
		return errors.New("reload callback is required")
	}
	lastObserved, _ := digest(w.Path)
	var pending bool
	var changedAt time.Time
	var candidate [32]byte
	for {
		interval, debounce := w.currentSettings()
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
		current, err := digest(w.Path)
		if err != nil {
			current = sha256.Sum256([]byte("error:" + err.Error()))
		}
		if current != lastObserved && (!pending || current != candidate) {
			candidate, changedAt, pending = current, time.Now(), true
		}
		if !pending || time.Since(changedAt) < debounce {
			continue
		}
		err = retryReload(ctx, w.Reload)
		// A rejected candidate becomes the latest observed content. It is retried only
		// after another file change or an explicit `reload` command.
		lastObserved = candidate
		pending = false
		if err != nil && w.OnError != nil {
			w.OnError(err)
		}
	}
}

func (w Watcher) currentSettings() (time.Duration, time.Duration) {
	interval, debounce := w.Interval, w.Debounce
	if w.Settings != nil {
		if dynamicInterval, dynamicDebounce := w.Settings(); dynamicInterval > 0 {
			interval = dynamicInterval
			if dynamicDebounce > 0 {
				debounce = dynamicDebounce
			}
		}
	}
	if interval <= 0 {
		interval = time.Second
	}
	if debounce <= 0 {
		debounce = 250 * time.Millisecond
	}
	return interval, debounce
}

func retryReload(ctx context.Context, reload ReloadFunc) error {
	delays := []time.Duration{0, 100 * time.Millisecond, 250 * time.Millisecond, 500 * time.Millisecond}
	var errs []error
	for _, delay := range delays {
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		if err := reload(); err != nil {
			errs = append(errs, err)
			continue
		}
		return nil
	}
	return fmt.Errorf("reload failed after %d attempts: %w", len(delays), errors.Join(errs...))
}

func digest(path string) ([32]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(data), nil
}
