package cmd

import (
	"bytes"
	"testing"
)

// TestUniqueMemo checks that two submits never share a memo, so their
// transaction IDs differ even with the same Warp message and UTXOs.
func TestUniqueMemo(t *testing.T) {
	a, err := uniqueMemo()
	if err != nil {
		t.Fatalf("uniqueMemo() error = %v", err)
	}
	b, err := uniqueMemo()
	if err != nil {
		t.Fatalf("uniqueMemo() error = %v", err)
	}
	if len(a) != memoLen || len(b) != memoLen {
		t.Fatalf("memo lengths = %d, %d, want %d", len(a), len(b), memoLen)
	}
	if bytes.Equal(a, b) {
		t.Fatalf("uniqueMemo() returned %x twice", a)
	}
}
