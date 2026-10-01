package warp

import (
	"context"
	"errors"
	"testing"
	"time"
)

var (
	err429            = errors.New("received status code: 429")
	errCloudflare1015 = errors.New("received status code: 403: error code: 1015")
	errWarpLag        = errors.New("failed verifying warp messages: signature is invalid")
	errLogic          = errors.New("validator not found")
)

var testRetryPolicy = RetryPolicy{
	Attempts:   6,
	Backoff:    time.Millisecond,
	MaxBackoff: time.Millisecond,
}

func TestIsTransient(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "rate_limit_429",
			err:  err429,
			want: true,
		},
		{
			name: "cloudflare_1015",
			err:  errCloudflare1015,
			want: true,
		},
		{
			name: "proposed_height_lag",
			err:  errWarpLag,
			want: true,
		},
		{
			name: "logic_error",
			err:  errLogic,
		},
		{
			// A message ID or hex blob can contain the digits of a status
			// code.
			name: "digits_in_an_ID",
			err:  errors.New("unknown validation 2a429b1015c"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransient(tt.err); got != tt.want {
				t.Fatalf("isTransient(%q) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestRetry(t *testing.T) {
	tests := []struct {
		name string
		// errs is the result of each call. A call past the end succeeds.
		errs      []error
		applied   bool
		wantCalls int
		wantErr   error
	}{
		{
			name:      "transient_twice_then_success",
			errs:      []error{err429, errWarpLag},
			wantCalls: 3,
		},
		{
			name:      "non_transient_is_not_retried",
			errs:      []error{errLogic},
			wantCalls: 1,
			wantErr:   errLogic,
		},
		{
			name:      "transient_then_non_transient",
			errs:      []error{errCloudflare1015, errLogic},
			wantCalls: 2,
			wantErr:   errLogic,
		},
		{
			name:      "exhausted",
			errs:      []error{err429, err429, err429, err429, err429, err429},
			wantCalls: 6,
			wantErr:   err429,
		},
		{
			// The call failed with a rate limit after the P-Chain accepted
			// it. The retry must not issue it again.
			name:      "previous_attempt_applied",
			errs:      []error{err429},
			applied:   true,
			wantCalls: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			got, err := retry(
				t.Context(),
				testRetryPolicy,
				t.Logf,
				"test",
				func(context.Context) (bool, error) { return tt.applied, nil },
				func(context.Context) (int, error) {
					calls++
					if calls <= len(tt.errs) {
						return 0, tt.errs[calls-1]
					}
					return calls, nil
				},
			)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("retry() error = %v, want %v", err, tt.wantErr)
			}
			if calls != tt.wantCalls {
				t.Fatalf("retry() calls = %d, want %d", calls, tt.wantCalls)
			}
			if tt.wantErr == nil && !tt.applied && got != tt.wantCalls {
				t.Fatalf("retry() = %d, want %d", got, tt.wantCalls)
			}
		})
	}
}
