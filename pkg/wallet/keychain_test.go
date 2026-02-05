package wallet

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/ava-labs/avalanchego/ids"
)

// testKeyBytes is the well-known ewoq test key.
var testKeyBytes = []byte{
	0x56, 0x28, 0x9e, 0x99, 0xc9, 0x4b, 0x69, 0x12,
	0xbf, 0xc1, 0x2a, 0xdc, 0x09, 0x3c, 0x9b, 0x51,
	0x12, 0x4f, 0x0d, 0xc5, 0x4a, 0xc7, 0xa7, 0x66,
	0xb2, 0xbc, 0x5c, 0xcf, 0x55, 0x8d, 0x80, 0x27,
}

// testKeyCB58 is the CB58 encoding of the test key.
const testKeyCB58 = "PrivateKey-ewoqjP7PxY4yr3iLTpLisriqt94hdyDFNgchSxGGztUrTXtNN"

// testKeyHex is the hex encoding of the test key.
const testKeyHex = "0x56289e99c94b6912bfc12adc093c9b51124f0dc54ac7a766b2bc5ccf558d8027"

func TestParsePrivateKey_CB58WithPrefix(t *testing.T) {
	keyBytes, err := ParsePrivateKey(testKeyCB58)
	if err != nil {
		t.Fatalf("ParsePrivateKey() error = %v", err)
	}

	if len(keyBytes) != 32 {
		t.Errorf("ParsePrivateKey() returned %d bytes, want 32", len(keyBytes))
	}
}

func TestParsePrivateKey_HexWithPrefix(t *testing.T) {
	keyBytes, err := ParsePrivateKey(testKeyHex)
	if err != nil {
		t.Fatalf("ParsePrivateKey() error = %v", err)
	}

	if len(keyBytes) != 32 {
		t.Errorf("ParsePrivateKey() returned %d bytes, want 32", len(keyBytes))
	}

	// Verify bytes match expected
	expectedHex := strings.TrimPrefix(testKeyHex, "0x")
	gotHex := hex.EncodeToString(keyBytes)
	if gotHex != expectedHex {
		t.Errorf("ParsePrivateKey() = %s, want %s", gotHex, expectedHex)
	}
}

func TestParsePrivateKey_HexUpperCase(t *testing.T) {
	upperHex := "0X" + strings.ToUpper(strings.TrimPrefix(testKeyHex, "0x"))
	keyBytes, err := ParsePrivateKey(upperHex)
	if err != nil {
		t.Fatalf("ParsePrivateKey() error = %v", err)
	}

	if len(keyBytes) != 32 {
		t.Errorf("ParsePrivateKey() returned %d bytes, want 32", len(keyBytes))
	}
}

func TestParsePrivateKey_RawHex(t *testing.T) {
	rawHex := strings.TrimPrefix(testKeyHex, "0x")
	keyBytes, err := ParsePrivateKey(rawHex)
	if err != nil {
		t.Fatalf("ParsePrivateKey() error = %v", err)
	}

	if len(keyBytes) != 32 {
		t.Errorf("ParsePrivateKey() returned %d bytes, want 32", len(keyBytes))
	}
}

func TestParsePrivateKey_RawCB58(t *testing.T) {
	rawCB58 := strings.TrimPrefix(testKeyCB58, "PrivateKey-")
	keyBytes, err := ParsePrivateKey(rawCB58)
	if err != nil {
		t.Fatalf("ParsePrivateKey() error = %v", err)
	}

	if len(keyBytes) != 32 {
		t.Errorf("ParsePrivateKey() returned %d bytes, want 32", len(keyBytes))
	}
}

func TestParsePrivateKey_WithWhitespace(t *testing.T) {
	keyWithWhitespace := "  " + testKeyCB58 + "  \n"
	keyBytes, err := ParsePrivateKey(keyWithWhitespace)
	if err != nil {
		t.Fatalf("ParsePrivateKey() error = %v", err)
	}

	if len(keyBytes) != 32 {
		t.Errorf("ParsePrivateKey() returned %d bytes, want 32", len(keyBytes))
	}
}

func TestParsePrivateKey_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"invalid string", "not a key"},
		{"invalid hex", "0xZZZZ"},
		{"invalid cb58", "PrivateKey-invalid!!!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParsePrivateKey(tt.input)
			if err == nil {
				t.Errorf("ParsePrivateKey(%q) should fail", tt.input)
			}
		})
	}
}

func TestParsePrivateKey_ShortKeyValidation(t *testing.T) {
	// ParsePrivateKey parses format but doesn't validate key length.
	// ToPrivateKey does the actual secp256k1 validation.
	shortKey := "0x1234"
	keyBytes, err := ParsePrivateKey(shortKey)
	if err != nil {
		// If it fails parsing, that's also acceptable
		return
	}

	// If parsing succeeds, ToPrivateKey should fail due to invalid length
	_, err = ToPrivateKey(keyBytes)
	if err == nil {
		t.Error("ToPrivateKey() with short key should fail")
	}
}

func TestToPrivateKey(t *testing.T) {
	key, err := ToPrivateKey(testKeyBytes)
	if err != nil {
		t.Fatalf("ToPrivateKey() error = %v", err)
	}

	if key == nil {
		t.Fatal("ToPrivateKey() returned nil")
	}

	// Key bytes should match
	if len(key.Bytes()) != 32 {
		t.Errorf("ToPrivateKey().Bytes() length = %d, want 32", len(key.Bytes()))
	}
}

func TestToPrivateKey_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"empty", []byte{}},
		{"too short", []byte{1, 2, 3}},
		{"too long", make([]byte, 64)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ToPrivateKey(tt.input)
			if err == nil {
				t.Errorf("ToPrivateKey() with %s should fail", tt.name)
			}
		})
	}
}

func TestDeriveAddresses(t *testing.T) {
	pAddr, evmAddr := DeriveAddresses(testKeyBytes)

	if pAddr == "" {
		t.Error("DeriveAddresses() returned empty P-Chain address")
	}

	if evmAddr == "" {
		t.Error("DeriveAddresses() returned empty EVM address")
	}

	// EVM address should start with 0x
	if !strings.HasPrefix(evmAddr, "0x") {
		t.Errorf("DeriveAddresses() EVM address should start with 0x, got %s", evmAddr)
	}

	// EVM address should be 42 chars (0x + 40 hex chars)
	if len(evmAddr) != 42 {
		t.Errorf("DeriveAddresses() EVM address length = %d, want 42", len(evmAddr))
	}

	// Verify known ewoq addresses
	expectedEVM := "0x8db97C7cEcE249c2b98bDC0226Cc4C2A57BF52FC"
	if !strings.EqualFold(evmAddr, expectedEVM) {
		t.Errorf("DeriveAddresses() EVM = %s, want %s", evmAddr, expectedEVM)
	}
}

func TestDeriveAddresses_InvalidKey(t *testing.T) {
	pAddr, evmAddr := DeriveAddresses([]byte{1, 2, 3})

	if pAddr != "" || evmAddr != "" {
		t.Error("DeriveAddresses() with invalid key should return empty strings")
	}
}

func TestDeriveAddressesFromKey(t *testing.T) {
	key, err := ToPrivateKey(testKeyBytes)
	if err != nil {
		t.Fatalf("ToPrivateKey() error = %v", err)
	}

	pAddr, evmAddr := DeriveAddressesFromKey(key)

	// Check that P-Chain address is not empty (all zeros)
	var emptyAddr ids.ShortID
	if pAddr == emptyAddr {
		t.Error("DeriveAddressesFromKey() returned zero P-Chain address")
	}

	if evmAddr == "" {
		t.Error("DeriveAddressesFromKey() returned empty EVM address")
	}
}

func TestKeyToHex(t *testing.T) {
	key, err := ToPrivateKey(testKeyBytes)
	if err != nil {
		t.Fatalf("ToPrivateKey() error = %v", err)
	}

	hexKey := KeyToHex(key)

	if !strings.HasPrefix(hexKey, "0x") {
		t.Errorf("KeyToHex() should start with 0x, got %s", hexKey[:2])
	}

	// Should be 66 chars (0x + 64 hex chars)
	if len(hexKey) != 66 {
		t.Errorf("KeyToHex() length = %d, want 66", len(hexKey))
	}

	// Parse it back and verify
	parsed, err := ParsePrivateKey(hexKey)
	if err != nil {
		t.Fatalf("ParsePrivateKey(KeyToHex()) error = %v", err)
	}

	if len(parsed) != len(testKeyBytes) {
		t.Errorf("Round-trip failed: got %d bytes, want %d", len(parsed), len(testKeyBytes))
	}
}

func TestParseAndDeriveConsistency(t *testing.T) {
	// Parse from different formats and verify they derive the same addresses
	formats := []string{
		testKeyCB58,
		testKeyHex,
		strings.TrimPrefix(testKeyCB58, "PrivateKey-"),
		strings.TrimPrefix(testKeyHex, "0x"),
	}

	var expectedPAddr, expectedEVMAddr string

	for i, format := range formats {
		keyBytes, err := ParsePrivateKey(format)
		if err != nil {
			t.Fatalf("ParsePrivateKey(%d) error = %v", i, err)
		}

		pAddr, evmAddr := DeriveAddresses(keyBytes)

		if i == 0 {
			expectedPAddr = pAddr
			expectedEVMAddr = evmAddr
		} else {
			if pAddr != expectedPAddr {
				t.Errorf("Format %d: P-Chain address = %s, want %s", i, pAddr, expectedPAddr)
			}
			if !strings.EqualFold(evmAddr, expectedEVMAddr) {
				t.Errorf("Format %d: EVM address = %s, want %s", i, evmAddr, expectedEVMAddr)
			}
		}
	}
}
