package multisig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/crypto/secp256k1"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
)

func TestNewOutputOwners(t *testing.T) {
	addr1 := ids.GenerateTestShortID()
	addr2 := ids.GenerateTestShortID()
	addr3 := ids.GenerateTestShortID()

	tests := []struct {
		name      string
		addrs     []ids.ShortID
		threshold uint32
		wantErr   bool
	}{
		{
			name:      "single address threshold 1",
			addrs:     []ids.ShortID{addr1},
			threshold: 1,
		},
		{
			name:      "2-of-3 multisig",
			addrs:     []ids.ShortID{addr1, addr2, addr3},
			threshold: 2,
		},
		{
			name:      "3-of-3 multisig",
			addrs:     []ids.ShortID{addr1, addr2, addr3},
			threshold: 3,
		},
		{
			name:      "empty addresses",
			addrs:     []ids.ShortID{},
			threshold: 1,
			wantErr:   true,
		},
		{
			name:      "threshold 0",
			addrs:     []ids.ShortID{addr1},
			threshold: 0,
			wantErr:   true,
		},
		{
			name:      "threshold exceeds addresses",
			addrs:     []ids.ShortID{addr1, addr2},
			threshold: 3,
			wantErr:   true,
		},
		{
			name:      "duplicate addresses",
			addrs:     []ids.ShortID{addr1, addr1},
			threshold: 1,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, err := NewOutputOwners(tt.addrs, tt.threshold)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if owner.Threshold != tt.threshold {
				t.Errorf("threshold = %d, want %d", owner.Threshold, tt.threshold)
			}
			if len(owner.Addrs) != len(tt.addrs) {
				t.Errorf("addrs len = %d, want %d", len(owner.Addrs), len(tt.addrs))
			}
			// Verify addresses are sorted
			for i := 1; i < len(owner.Addrs); i++ {
				if owner.Addrs[i].Compare(owner.Addrs[i-1]) <= 0 {
					t.Errorf("addresses not sorted at index %d", i)
				}
			}
		})
	}
}

func TestTxFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	tf := &TxFile{
		Version:   TxFileVersion,
		NetworkID: constants.FujiID,
		TxBytes:   "deadbeef",
		Owners: &OwnerInfo{
			Threshold: 2,
			Addresses: []string{"P-fuji1abc", "P-fuji1def"},
		},
		Signers: []SignerInfo{
			{Address: "P-fuji1abc", Signed: true},
			{Address: "P-fuji1def", Signed: false},
		},
	}

	// Write
	if err := WriteTxFile(path, tf); err != nil {
		t.Fatalf("WriteTxFile() error: %v", err)
	}

	// Verify file permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat error: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("file permissions = %o, want 0600", info.Mode().Perm())
	}

	// Read back
	got, err := ReadTxFile(path)
	if err != nil {
		t.Fatalf("ReadTxFile() error: %v", err)
	}

	if got.Version != tf.Version {
		t.Errorf("version = %d, want %d", got.Version, tf.Version)
	}
	if got.NetworkID != tf.NetworkID {
		t.Errorf("network ID = %d, want %d", got.NetworkID, tf.NetworkID)
	}
	if got.TxBytes != tf.TxBytes {
		t.Errorf("tx bytes = %s, want %s", got.TxBytes, tf.TxBytes)
	}
	if got.Owners == nil {
		t.Fatal("owners is nil")
	}
	if got.Owners.Threshold != tf.Owners.Threshold {
		t.Errorf("owners threshold = %d, want %d", got.Owners.Threshold, tf.Owners.Threshold)
	}
	if len(got.Signers) != len(tf.Signers) {
		t.Fatalf("signers len = %d, want %d", len(got.Signers), len(tf.Signers))
	}
	if got.Signers[0].Signed != true {
		t.Error("signer[0] should be signed")
	}
	if got.Signers[1].Signed != false {
		t.Error("signer[1] should not be signed")
	}
}

func TestReadTxFileVersionMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")

	if err := os.WriteFile(path, []byte(`{"version": 99, "network_id": 5, "tx_bytes": "aa"}`), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := ReadTxFile(path)
	if err == nil {
		t.Fatal("expected version mismatch error")
	}
}

func TestIsFullySigned(t *testing.T) {
	// Create a tx with credentials
	tx := &txs.Tx{}

	// Empty credential (not signed)
	var emptySig [secp256k1.SignatureLen]byte
	var filledSig [secp256k1.SignatureLen]byte
	filledSig[0] = 0x01 // non-zero = signed

	// Not fully signed
	tx.Creds = append(tx.Creds, &secp256k1fx.Credential{
		Sigs: [][secp256k1.SignatureLen]byte{emptySig},
	})
	if IsFullySigned(tx) {
		t.Error("expected not fully signed")
	}

	// Fully signed
	tx.Creds[0] = &secp256k1fx.Credential{
		Sigs: [][secp256k1.SignatureLen]byte{filledSig},
	}
	if !IsFullySigned(tx) {
		t.Error("expected fully signed")
	}
}

func TestCredentialSignatureStatus(t *testing.T) {
	var emptySig [secp256k1.SignatureLen]byte
	var filledSig [secp256k1.SignatureLen]byte
	filledSig[0] = 0x01

	tx := &txs.Tx{}
	tx.Creds = append(tx.Creds,
		&secp256k1fx.Credential{
			Sigs: [][secp256k1.SignatureLen]byte{filledSig, emptySig},
		},
		&secp256k1fx.Credential{
			Sigs: [][secp256k1.SignatureLen]byte{filledSig},
		},
	)

	filled, total := CredentialSignatureStatus(tx)
	if filled != 2 {
		t.Errorf("filled = %d, want 2", filled)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
}

func TestSignatureStatus(t *testing.T) {
	tf := &TxFile{
		Owners: &OwnerInfo{
			Threshold: 2,
			Addresses: []string{"a", "b", "c"},
		},
		Signers: []SignerInfo{
			{Address: "a", Signed: true},
			{Address: "b", Signed: false},
			{Address: "c", Signed: true},
		},
	}

	got := SignatureStatus(tf)
	want := "2/3 signatures (2 required)"
	if got != want {
		t.Errorf("SignatureStatus() = %q, want %q", got, want)
	}
}

func TestParseAddresses(t *testing.T) {
	addr1 := ids.GenerateTestShortID()

	// Test with raw short ID
	addrs, err := ParseAddresses(addr1.String(), "fuji")
	if err != nil {
		t.Fatalf("ParseAddresses() error: %v", err)
	}
	if len(addrs) != 1 {
		t.Fatalf("expected 1 address, got %d", len(addrs))
	}
	if addrs[0] != addr1 {
		t.Errorf("address mismatch: got %s, want %s", addrs[0], addr1)
	}

	// Test empty
	_, err = ParseAddresses("", "fuji")
	if err == nil {
		t.Error("expected error for empty address list")
	}

	// Test comma-separated
	addr2 := ids.GenerateTestShortID()
	addrs, err = ParseAddresses(addr1.String()+","+addr2.String(), "fuji")
	if err != nil {
		t.Fatalf("ParseAddresses() error: %v", err)
	}
	if len(addrs) != 2 {
		t.Fatalf("expected 2 addresses, got %d", len(addrs))
	}
}

func TestFormatSignerStatus(t *testing.T) {
	tf := &TxFile{
		Owners: &OwnerInfo{
			Threshold: 2,
			Addresses: []string{"P-fuji1abc", "P-fuji1def"},
		},
		Signers: []SignerInfo{
			{Address: "P-fuji1abc", Signed: true},
			{Address: "P-fuji1def", Signed: false},
		},
	}

	got := FormatSignerStatus(tf)
	if got == "" {
		t.Error("expected non-empty status")
	}
	// Should contain threshold info and addresses
	if len(got) < 20 {
		t.Errorf("status too short: %q", got)
	}
}
