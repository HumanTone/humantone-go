package humantone

import (
	"context"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// rngMu guards rng. math/rand's top-level Source is goroutine-safe in modern
// Go, but to keep jitter deterministic-test-friendly we use our own source
// seeded once. The lock cost is negligible for retry use.
var (
	rngMu sync.Mutex
	rng   = rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // jitter, not crypto
)

// jitterFunc returns the per-attempt jitter offset. Indirected so tests can
// replace it with a deterministic value. Default implementation uses rng to
// pick a value in [-jitterMax, +jitterMax].
var jitterFunc = func() time.Duration {
	rngMu.Lock()
	defer rngMu.Unlock()
	return time.Duration(rng.Int63n(int64(2*jitterMax))) - jitterMax
}

// sleepFunc is the indirected sleep implementation used by the retry loop.
// Tests replace it with a no-op or short-circuit to avoid real waits.
var sleepFunc = func(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

const (
	// baseBackoff is the initial backoff for exponential schedules.
	baseBackoff = 500 * time.Millisecond
	// rateBaseBackoff is the initial backoff specifically for 429 without Retry-After.
	rateBaseBackoff = 1 * time.Second
	// jitterMax bounds the symmetric jitter applied to every backoff.
	jitterMax = 200 * time.Millisecond
)

// shouldRetry implements the §7.2 matrix as a single decision function. It
// receives the most recent *Error, the request's HTTP method and path, the
// current attempt index (0 = first attempt, 1 = first retry, ...), and the
// effective Config for this client. It returns true when another attempt
// should be made.
func shouldRetry(method, path string, e *Error, cfg Config, attempt int) bool {
	if e == nil {
		return false
	}
	if attempt >= cfg.MaxRetries {
		return false
	}

	isPost := method == http.MethodPost
	is429 := e.StatusCode == 429
	is5xx := e.StatusCode >= 500 && e.StatusCode < 600

	switch e.sentinel {
	case ErrRateLimit:
		// 429 always retries on every method.
		return true
	case ErrAuthentication, ErrPermission, ErrInsufficientCredits,
		ErrDailyLimitExceeded, ErrInvalidRequest, ErrNotFound,
		ErrTimeout, ErrInvalidAPIKey:
		return false
	case ErrNetwork:
		if !isPost {
			return true
		}
		return cfg.RetryOnPost
	case ErrAPIError:
		// Two cases roll up here:
		//   (a) HTTP 5xx (truly transient).
		//   (b) HTTP 200 + success:false on /v1/detect with no message
		//       (transient detection backend error).
		// (a) and (b) for GET always retry; for POST require RetryOnPost.
		if is429 {
			// Defensive — should already be ErrRateLimit, but if API
			// surfaces 429 as api_error somehow, still retry.
			return true
		}
		if !e.Retryable {
			// e.g., humanize 200+success:false reserved (Retryable=false)
			// or detect 200+success:false with unknown msg (per user #3).
			// Allow upgrade only for the humanize reserved case, signaled
			// by POST + 200 status + path /v1/humanize + RetryOnPost.
			if isPost && e.StatusCode == 200 && cfg.RetryOnPost && path == "/v1/humanize" {
				return true
			}
			return false
		}
		if !isPost {
			return true
		}
		if is5xx {
			return cfg.RetryOnPost
		}
		// 200 detect transient + Retryable=true: retry on POST always per matrix.
		return true
	}

	// Unknown sentinel — be conservative.
	return false
}

// backoffDuration returns the wait duration before the next retry attempt.
// attempt is the just-completed attempt index (0-based); the first retry
// uses attempt=0's backoff.
func backoffDuration(attempt int, e *Error) time.Duration {
	if e == nil {
		return 0
	}

	// 429 with Retry-After: honor the header (with jitter).
	if e.sentinel == ErrRateLimit && e.RetryAfterSeconds > 0 {
		return time.Duration(e.RetryAfterSeconds)*time.Second + jitterFunc()
	}

	// 429 without Retry-After: 1s × 2^attempt with jitter.
	if e.sentinel == ErrRateLimit {
		d := rateBaseBackoff * (1 << attempt)
		return d + jitterFunc()
	}

	// All other retryable conditions: 500ms × 2^attempt with jitter.
	d := baseBackoff * (1 << attempt)
	return d + jitterFunc()
}

// executeWithRetry runs op (a single attempt) up to MaxRetries+1 times,
// applying the §7.2 retry matrix. The caller's context is honored both
// during requests and during backoff; cancellation aborts immediately.
//
// op returns the *Error from the most recent attempt and any opaque success
// payload it wishes to carry across attempts (typically the response body).
func (c *Client) executeWithRetry(ctx context.Context, method, path string, op func(ctx context.Context) ([]byte, http.Header, *Error)) ([]byte, http.Header, *Error) {
	var (
		body    []byte
		headers http.Header
		lastErr *Error
	)
	for attempt := 0; ; attempt++ {
		body, headers, lastErr = op(ctx)
		if lastErr == nil {
			return body, headers, nil
		}
		if !shouldRetry(method, path, lastErr, c.cfg, attempt) {
			return nil, nil, lastErr
		}
		wait := backoffDuration(attempt, lastErr)
		if wait < 0 {
			wait = 0
		}
		if err := sleepFunc(ctx, wait); err != nil {
			// Context canceled or deadline exceeded during backoff —
			// surface as a transport-shaped error.
			return nil, nil, classifyTransportError(err)
		}
	}
}
