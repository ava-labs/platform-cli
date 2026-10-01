package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/utils/crypto/bls/signer/localsigner"
)

func TestLoadBLSKey(t *testing.T) {
	want, err := localsigner.New()
	if err != nil {
		t.Fatalf("localsigner.New() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "signer.key")
	if err := want.ToFile(path); err != nil {
		t.Fatalf("ToFile() error = %v", err)
	}

	got, err := loadBLSKey(path)
	if err != nil {
		t.Fatalf("loadBLSKey() error = %v", err)
	}
	if !got.PublicKey().Equals(want.PublicKey()) {
		t.Fatal("loadBLSKey() public key does not match the key file")
	}
}

func TestLoadBLSKeyRejectsWrongLength(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signer.key")
	// A hex-encoded key is the most likely wrong format.
	if err := os.WriteFile(path, []byte("0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	_, err := loadBLSKey(path)
	if !errors.Is(err, errWarpBadKeyFile) {
		t.Fatalf("loadBLSKey() error = %v, want %v", err, errWarpBadKeyFile)
	}
}

func TestVerifyRegisterExpiry(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tests := []struct {
		name    string
		expiry  time.Time
		wantErr error
	}{
		{
			name:   "23h_ahead",
			expiry: now.Add(23 * time.Hour),
		},
		{
			name:   "end_of_window",
			expiry: now.Add(registerExpiryWindow),
		},
		{
			name:    "past_window",
			expiry:  now.Add(registerExpiryWindow + time.Second),
			wantErr: errWarpExpiryTooFar,
		},
		{
			name:    "now",
			expiry:  now,
			wantErr: errWarpExpiryPassed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyRegisterExpiry(uint64(tt.expiry.Unix()), now)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("verifyRegisterExpiry() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
