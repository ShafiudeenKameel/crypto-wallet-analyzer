package provider

import (
	"context"
	"errors"
	"time"
)

// RetryOnRateLimit retries fn with exponential backoff, but only when fn's
// error wraps ErrRateLimited - any other error (including ctx cancellation)
// returns immediately, since retrying wouldn't help. Shared by every
// concrete provider so backoff behavior is defined exactly once.
func RetryOnRateLimit(ctx context.Context, maxRetries int, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<attempt) * 200 * time.Millisecond
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		if !errors.Is(err, ErrRateLimited) {
			return err
		}
	}
	return lastErr
}
