package cmd

import (
	"bytes"
	"testing"
)

func TestDecodeHex(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []byte
		wantErr bool
	}{
		{
			name:  "no prefix",
			input: "0a0b0c",
			want:  []byte{0x0a, 0x0b, 0x0c},
		},
		{
			name:  "lowercase prefix",
			input: "0x0a0b0c",
			want:  []byte{0x0a, 0x0b, 0x0c},
		},
		{
			name:  "uppercase prefix",
			input: "0X0A0B0C",
			want:  []byte{0x0a, 0x0b, 0x0c},
		},
		{
			name:  "trim spaces",
			input: "  0x0a0b0c  ",
			want:  []byte{0x0a, 0x0b, 0x0c},
		},
		{
			name:    "invalid hex",
			input:   "0xzz",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeHex(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("decodeHex() expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeHex() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("decodeHex() = %x, want %x", got, tt.want)
			}
		})
	}
}

func TestDecodeHexExactLength(t *testing.T) {
	got, err := decodeHexExactLength("0X001122", 3)
	if err != nil {
		t.Fatalf("decodeHexExactLength() error = %v", err)
	}
	if !bytes.Equal(got, []byte{0x00, 0x11, 0x22}) {
		t.Fatalf("decodeHexExactLength() = %x, want 001122", got)
	}

	_, err = decodeHexExactLength("0x001122", 20)
	if err == nil {
		t.Fatal("decodeHexExactLength() expected length error")
	}
}
