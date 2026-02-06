package crosschain

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/ids"
)

func TestIsRetryableImportError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "not found error",
			err:  errors.New("UTXO not found"),
			want: true,
		},
		{
			name: "not found lowercase",
			err:  errors.New("utxo not found in mempool"),
			want: true,
		},
		{
			name: "no utxos error",
			err:  errors.New("no utxos available"),
			want: true,
		},
		{
			name: "insufficient funds",
			err:  errors.New("insufficient funds for transfer"),
			want: true,
		},
		{
			name: "missing utxo",
			err:  errors.New("missing utxo in set"),
			want: true,
		},
		{
			name: "rate limited status code",
			err:  errors.New("received status code: 429"),
			want: true,
		},
		{
			name: "rate limited message",
			err:  errors.New("too many requests"),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("connection refused"),
			want: false,
		},
		{
			name: "permission denied",
			err:  errors.New("permission denied"),
			want: false,
		},
		{
			name: "invalid signature",
			err:  errors.New("invalid signature"),
			want: false,
		},
		{
			name: "mixed case not found",
			err:  errors.New("UTXO NOT FOUND"),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableImportError(tt.err)
			if got != tt.want {
				t.Errorf("isRetryableImportError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestImportWithRetry_Success(t *testing.T) {
	expectedID := ids.GenerateTestID()
	callCount := 0

	importFn := func() (ids.ID, error) {
		callCount++
		return expectedID, nil
	}

	ctx := context.Background()
	result, err := importWithRetry(ctx, importFn)

	if err != nil {
		t.Fatalf("importWithRetry() error = %v", err)
	}
	if result != expectedID {
		t.Errorf("importWithRetry() = %v, want %v", result, expectedID)
	}
	if callCount != 1 {
		t.Errorf("importFn called %d times, want 1", callCount)
	}
}

func TestImportWithRetry_SuccessAfterRetry(t *testing.T) {
	expectedID := ids.GenerateTestID()
	callCount := 0

	importFn := func() (ids.ID, error) {
		callCount++
		if callCount < 3 {
			return ids.Empty, errors.New("UTXO not found")
		}
		return expectedID, nil
	}

	ctx := context.Background()
	result, err := importWithRetry(ctx, importFn)

	if err != nil {
		t.Fatalf("importWithRetry() error = %v", err)
	}
	if result != expectedID {
		t.Errorf("importWithRetry() = %v, want %v", result, expectedID)
	}
	if callCount != 3 {
		t.Errorf("importFn called %d times, want 3", callCount)
	}
}

func TestImportWithRetry_NonRetryableError(t *testing.T) {
	callCount := 0

	importFn := func() (ids.ID, error) {
		callCount++
		return ids.Empty, errors.New("invalid signature")
	}

	ctx := context.Background()
	_, err := importWithRetry(ctx, importFn)

	if err == nil {
		t.Fatal("importWithRetry() should fail with non-retryable error")
	}
	if callCount != 1 {
		t.Errorf("importFn called %d times, want 1 (should not retry)", callCount)
	}
}

func TestImportWithRetry_MaxRetries(t *testing.T) {
	callCount := 0

	importFn := func() (ids.ID, error) {
		callCount++
		return ids.Empty, errors.New("UTXO not found")
	}

	ctx := context.Background()
	_, err := importWithRetry(ctx, importFn)

	if err == nil {
		t.Fatal("importWithRetry() should fail after max retries")
	}
	if callCount != importRetryAttempts {
		t.Errorf("importFn called %d times, want %d", callCount, importRetryAttempts)
	}
}

func TestImportWithRetry_ContextCancelled(t *testing.T) {
	callCount := 0

	importFn := func() (ids.ID, error) {
		callCount++
		return ids.Empty, errors.New("UTXO not found")
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after a short delay
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := importWithRetry(ctx, importFn)

	if err == nil {
		t.Fatal("importWithRetry() should fail when context is cancelled")
	}
	// Should have tried at least once but not necessarily all retries
	if callCount == 0 {
		t.Error("importFn should have been called at least once")
	}
}

func TestImportRetryConstants(t *testing.T) {
	// Verify retry constants are reasonable
	if importRetryAttempts < 3 {
		t.Errorf("importRetryAttempts = %d, should be at least 3", importRetryAttempts)
	}
	if importRetryAttempts > 10 {
		t.Errorf("importRetryAttempts = %d, should be at most 10", importRetryAttempts)
	}

	if importRetryDelay < 100*time.Millisecond {
		t.Errorf("importRetryDelay = %v, should be at least 100ms", importRetryDelay)
	}
	if importRetryDelay > 5*time.Second {
		t.Errorf("importRetryDelay = %v, should be at most 5s", importRetryDelay)
	}
}
