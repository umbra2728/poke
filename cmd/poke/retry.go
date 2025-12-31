package main

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

type retryConfig struct {
	MaxRetries int
	BackoffMin time.Duration
	BackoffMax time.Duration
}

func (c retryConfig) enabled() bool {
	return c.MaxRetries > 0
}

func (c retryConfig) validate() error {
	if c.MaxRetries < 0 {
		return errors.New("-retries must be >= 0")
	}
	if c.BackoffMin < 0 {
		return errors.New("-backoff-min must be >= 0")
	}
	if c.BackoffMax < 0 {
		return errors.New("-backoff-max must be >= 0")
	}
	if c.BackoffMax > 0 && c.BackoffMax < c.BackoffMin {
		return errors.New("-backoff-max must be >= -backoff-min")
	}
	return nil
}

func isRetryableHTTPStatus(code int) bool {
	return code == http.StatusTooManyRequests || (code >= 500 && code <= 599)
}

func isRetryableDoError(err error) bool {
	if err == nil {
		return false
	}
	// Context cancellation/timeouts should surface immediately to callers.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return true
}

func parseRetryAfter(h string, now time.Time) (time.Duration, bool) {
	if h == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(h); err == nil {
		if secs <= 0 {
			return 0, false
		}
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(h); err == nil {
		d := t.Sub(now)
		if d <= 0 {
			return 0, false
		}
		return d, true
	}
	return 0, false
}

func nextBackoffDelay(cfg retryConfig, retryNumber int, retryAfter time.Duration) time.Duration {
	if retryNumber <= 0 {
		return 0
	}

	min := cfg.BackoffMin
	max := cfg.BackoffMax

	// Compute exponential backoff starting at min.
	base := min
	if retryNumber > 1 && base > 0 {
		exp := float64(base) * math.Pow(2, float64(retryNumber-1))
		if exp > float64(math.MaxInt64) {
			exp = float64(math.MaxInt64)
		}
		if exp > float64(max) && max > 0 {
			base = max
		} else {
			base = time.Duration(exp)
		}
	}

	if retryAfter > base {
		base = retryAfter
	}
	if base < min {
		base = min
	}
	if max > 0 && base > max {
		base = max
	}

	// Equal jitter: pick in [base/2, base], then clamp to bounds.
	low := base / 2
	j := low
	if base > low {
		j = low + time.Duration(rand.Int64N(int64(base-low)+1))
	}
	if j < min {
		j = min
	}
	if max > 0 && j > max {
		j = max
	}
	return j
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
