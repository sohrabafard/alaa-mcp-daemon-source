package supervisor

import (
	"context"
	"sync"
)

type Limiter struct {
	mu     sync.Mutex
	limit  int
	inUse  int
	notify chan struct{}
}

func NewLimiter(limit int) *Limiter {
	if limit < 1 {
		limit = 1
	}
	return &Limiter{limit: limit, notify: make(chan struct{})}
}

func (l *Limiter) SetLimit(limit int) {
	if limit < 1 {
		limit = 1
	}
	l.mu.Lock()
	l.limit = limit
	l.signalLocked()
	l.mu.Unlock()
}

func (l *Limiter) Acquire(ctx context.Context) (func(), error) {
	for {
		l.mu.Lock()
		if l.inUse < l.limit {
			l.inUse++
			l.mu.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() {
					l.mu.Lock()
					l.inUse--
					l.signalLocked()
					l.mu.Unlock()
				})
			}, nil
		}
		notify := l.notify
		l.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-notify:
		}
	}
}

func (l *Limiter) signalLocked() {
	close(l.notify)
	l.notify = make(chan struct{})
}
