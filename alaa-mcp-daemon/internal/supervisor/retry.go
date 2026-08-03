package supervisor

import (
	"time"

	"alaa-mcp-daemon/internal/config"
)

type retryWindow struct{ failures []time.Time }

func (r *retryWindow) Reset() { r.failures = nil }

func (r *retryWindow) Record(now time.Time, policy config.EffectiveRestartPolicy) (count int, exhausted bool, backoff time.Duration) {
	cutoff := now.Add(-policy.Window)
	kept := r.failures[:0]
	for _, failure := range r.failures {
		if !failure.Before(cutoff) {
			kept = append(kept, failure)
		}
	}
	r.failures = append(kept, now)
	count = len(r.failures)
	exhausted = count >= policy.MaxAttempts
	backoff = policy.BackoffInitial
	for i := 1; i < count && backoff < policy.BackoffMax; i++ {
		if backoff > policy.BackoffMax/2 {
			backoff = policy.BackoffMax
			break
		}
		backoff *= 2
	}
	if backoff > policy.BackoffMax {
		backoff = policy.BackoffMax
	}
	return count, exhausted, backoff
}

func (r *retryWindow) Count(now time.Time, window time.Duration) int {
	cutoff := now.Add(-window)
	count := 0
	for _, failure := range r.failures {
		if !failure.Before(cutoff) {
			count++
		}
	}
	return count
}
