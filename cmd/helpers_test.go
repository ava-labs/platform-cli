package cmd

import (
	"math"
	"reflect"
	"testing"
	"time"
)

func TestAvaxToNAVAX(t *testing.T) {
	tests := []struct {
		name    string
		input   float64
		want    uint64
		wantErr bool
	}{
		{
			name:  "whole avax",
			input: 1,
			want:  1_000_000_000,
		},
		{
			name:  "fractional avax",
			input: 1.5,
			want:  1_500_000_000,
		},
		{
			name:  "smallest unit",
			input: 0.000000001,
			want:  1,
		},
		{
			name:    "negative amount",
			input:   -1,
			wantErr: true,
		},
		{
			name:    "overflow",
			input:   float64(math.MaxUint64)/1e9 + 1,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := avaxToNAVAX(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("avaxToNAVAX(%f) expected error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("avaxToNAVAX(%f) returned error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("avaxToNAVAX(%f) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestFeeToShares(t *testing.T) {
	tests := []struct {
		name    string
		input   float64
		want    uint32
		wantErr bool
	}{
		{
			name:  "zero",
			input: 0,
			want:  0,
		},
		{
			name:  "two percent",
			input: 0.02,
			want:  20_000,
		},
		{
			name:  "hundred percent",
			input: 1,
			want:  1_000_000,
		},
		{
			name:    "negative",
			input:   -0.01,
			wantErr: true,
		},
		{
			name:    "above one",
			input:   1.01,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := feeToShares(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("feeToShares(%f) expected error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("feeToShares(%f) returned error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("feeToShares(%f) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetTransferAmountNAVAX(t *testing.T) {
	origAmount := transferAmount
	origAmountNAVAX := transferAmountNAVAX
	defer func() {
		transferAmount = origAmount
		transferAmountNAVAX = origAmountNAVAX
	}()

	transferAmount = 1
	transferAmountNAVAX = 123
	got, err := getTransferAmountNAVAX()
	if err != nil {
		t.Fatalf("getTransferAmountNAVAX() returned error: %v", err)
	}
	if got != 123 {
		t.Fatalf("getTransferAmountNAVAX() = %d, want 123", got)
	}

	transferAmount = 1.5
	transferAmountNAVAX = 0
	got, err = getTransferAmountNAVAX()
	if err != nil {
		t.Fatalf("getTransferAmountNAVAX() returned error: %v", err)
	}
	if got != 1_500_000_000 {
		t.Fatalf("getTransferAmountNAVAX() = %d, want 1500000000", got)
	}

	transferAmount = 0
	transferAmountNAVAX = 0
	_, err = getTransferAmountNAVAX()
	if err == nil {
		t.Fatal("getTransferAmountNAVAX() expected error for missing amount")
	}
}

func TestParseTimeRange(t *testing.T) {
	before := time.Now()
	start, end, err := parseTimeRange("now", "1h")
	after := time.Now()
	if err != nil {
		t.Fatalf("parseTimeRange(now, 1h) returned error: %v", err)
	}

	// now mode should default to ~30s in the future.
	if start.Before(before.Add(25*time.Second)) || start.After(after.Add(35*time.Second)) {
		t.Fatalf("parseTimeRange(now, 1h) start=%v outside expected now+30s window", start)
	}
	if end.Sub(start) != time.Hour {
		t.Fatalf("parseTimeRange(now, 1h) duration=%v, want 1h", end.Sub(start))
	}

	fixedStart := "2026-01-01T00:00:00Z"
	start, end, err = parseTimeRange(fixedStart, "2h")
	if err != nil {
		t.Fatalf("parseTimeRange(fixed, 2h) returned error: %v", err)
	}
	if start.Format(time.RFC3339) != fixedStart {
		t.Fatalf("parseTimeRange(fixed, 2h) start=%s, want %s", start.Format(time.RFC3339), fixedStart)
	}
	if end.Sub(start) != 2*time.Hour {
		t.Fatalf("parseTimeRange(fixed, 2h) duration=%v, want 2h", end.Sub(start))
	}

	_, _, err = parseTimeRange("bad-time", "1h")
	if err == nil {
		t.Fatal("parseTimeRange() expected error for invalid start time")
	}

	_, _, err = parseTimeRange("now", "bad-duration")
	if err == nil {
		t.Fatal("parseTimeRange() expected error for invalid duration")
	}
}

func TestParseValidatorAddrs(t *testing.T) {
	got := parseValidatorAddrs(" 127.0.0.1 , node.example.com:9650 ,,https://node.example.com ")
	want := []string{"127.0.0.1", "node.example.com:9650", "https://node.example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseValidatorAddrs() = %#v, want %#v", got, want)
	}

	got = parseValidatorAddrs("")
	if len(got) != 0 {
		t.Fatalf("parseValidatorAddrs(\"\") = %#v, want empty slice", got)
	}
}

func TestNormalizeNodeURI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"ip only", "127.0.0.1", "http://127.0.0.1:9650"},
		{"ip with port", "127.0.0.1:9650", "http://127.0.0.1:9650"},
		{"http uri", "http://127.0.0.1:9650", "http://127.0.0.1:9650"},
		{"https uri", "https://example.com", "https://example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeNodeURI(tt.input)
			if got != tt.want {
				t.Fatalf("normalizeNodeURI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeValidatorNodeURI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"ip only", "127.0.0.1", "http://127.0.0.1:9650"},
		{"ip with port", "127.0.0.1:9650", "http://127.0.0.1:9650"},
		{"http uri", "http://127.0.0.1:9650", "http://127.0.0.1:9650"},
		{"https uri", "https://example.com", "https://example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeValidatorNodeURI(tt.input)
			if got != tt.want {
				t.Fatalf("normalizeValidatorNodeURI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsEwoqKey(t *testing.T) {
	key := make([]byte, len(ewoqPrivateKey))
	copy(key, ewoqPrivateKey)
	if !isEwoqKey(key) {
		t.Fatal("isEwoqKey() expected true for ewoq key bytes")
	}

	key[0] ^= 0xFF
	if isEwoqKey(key) {
		t.Fatal("isEwoqKey() expected false for modified key bytes")
	}

	if isEwoqKey([]byte{1, 2, 3}) {
		t.Fatal("isEwoqKey() expected false for wrong length")
	}
}
