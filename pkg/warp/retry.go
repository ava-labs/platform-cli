package warp

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Transient error signatures. Each one is matched as text, because JSON-RPC
// and the HTTP client drop the error chain.
var transientErrors = []string{
	// The public API rate limit, as returned by the avalanchego RPC client.
	"status code: 429",
	"Too Many Requests",
	// The Cloudflare rate limit page body.
	"error code: 1015",
	// The P-Chain verified the Warp message against a set other than the one
	// it was aggregated against, as when the epoch changes between the read
	// and the submit. The retry aggregates again.
	"failed verifying warp messages",
}

// isTransient reports whether err is a rate limit or an epoch change,
// which clear on their own. Every other error is a logic error.
func isTransient(err error) bool {
	msg := err.Error()
	for _, s := range transientErrors {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// RetryPolicy bounds the retries of a transient P-Chain error.
type RetryPolicy struct {
	// Attempts is the maximum number of calls. A value below 1 means 1.
	Attempts int
	// Backoff is the wait after the first failure. It doubles after each
	// failure, up to MaxBackoff.
	Backoff    time.Duration
	MaxBackoff time.Duration
}

// retry calls f until it succeeds, returns a non-transient error, or uses
// all attempts. Before each retry, it calls applied, if not nil. If applied
// reports true, the failed call took effect after all, and retry returns the
// zero value and no error, so a transaction is never issued twice.
func retry[T any](
	ctx context.Context,
	p RetryPolicy,
	logf func(format string, args ...any),
	what string,
	applied func(context.Context) (bool, error),
	f func(context.Context) (T, error),
) (T, error) {
	var zero T
	backoff := p.Backoff
	for attempt := 1; ; attempt++ {
		v, err := f(ctx)
		if err == nil {
			return v, nil
		}
		if !isTransient(err) || attempt >= p.Attempts {
			return zero, err
		}
		logf("%s: transient error (attempt %d of %d), retrying in %s: %v", what, attempt, p.Attempts, backoff, err)
		select {
		case <-ctx.Done():
			return zero, fmt.Errorf("%s: %w (last error: %w)", what, ctx.Err(), err)
		case <-time.After(backoff):
		}
		backoff = min(2*backoff, p.MaxBackoff)

		if applied == nil {
			continue
		}
		done, checkErr := applied(ctx)
		if checkErr != nil {
			return zero, fmt.Errorf("%s: failed to check the previous attempt: %w (previous error: %w)", what, checkErr, err)
		}
		if done {
			logf("%s: the previous attempt was accepted", what)
			return zero, nil
		}
	}
}
