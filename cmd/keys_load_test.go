package cmd

import (
	"encoding/hex"
	"testing"

	"github.com/ava-labs/avalanchego/utils/crypto/secp256k1"
)

// resetKeySources clears the global key-source state and restores it after
// the test, so loadKeys resolution is deterministic.
func resetKeySources(t *testing.T) {
	t.Helper()
	prevNames := keyNamesGlobal
	prevPrivateKey := privateKey
	keyNamesGlobal = nil
	privateKey = ""
	t.Setenv("AVALANCHE_PRIVATE_KEY", "")
	// Point the keystore at an empty directory so a developer's default key
	// cannot shadow the env-based sources under test.
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(func() {
		keyNamesGlobal = prevNames
		privateKey = prevPrivateKey
	})
}

func testKeyHex(t *testing.T) string {
	t.Helper()
	key, err := secp256k1.NewPrivateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	return "0x" + hex.EncodeToString(key.Bytes())
}

func TestLoadKeysEnvSingle(t *testing.T) {
	resetKeySources(t)
	t.Setenv("AVALANCHE_PRIVATE_KEY", testKeyHex(t))

	allKeys, err := loadKeys()
	if err != nil {
		t.Fatalf("loadKeys() returned error: %v", err)
	}
	if len(allKeys) != 1 {
		t.Fatalf("loadKeys() returned %d keys, want 1", len(allKeys))
	}
}

func TestLoadKeysEnvMultiple(t *testing.T) {
	resetKeySources(t)
	t.Setenv("AVALANCHE_PRIVATE_KEY", testKeyHex(t)+", "+testKeyHex(t))

	allKeys, err := loadKeys()
	if err != nil {
		t.Fatalf("loadKeys() returned error: %v", err)
	}
	if len(allKeys) != 2 {
		t.Fatalf("loadKeys() returned %d keys, want 2", len(allKeys))
	}
}

func TestLoadKeysEnvInvalidEntry(t *testing.T) {
	resetKeySources(t)
	t.Setenv("AVALANCHE_PRIVATE_KEY", testKeyHex(t)+",not-a-key")

	if _, err := loadKeys(); err == nil {
		t.Fatal("loadKeys() expected error for invalid key entry, got nil")
	}
}

func TestLoadKeysNameAndPrivateKeyConflict(t *testing.T) {
	resetKeySources(t)
	keyNamesGlobal = []string{"somekey"}
	privateKey = testKeyHex(t)

	if _, err := loadKeys(); err == nil {
		t.Fatal("loadKeys() expected error for --key-name with --private-key, got nil")
	}
}

func TestLoadKeySingleRejectsMultiple(t *testing.T) {
	resetKeySources(t)
	t.Setenv("AVALANCHE_PRIVATE_KEY", testKeyHex(t)+","+testKeyHex(t))

	if _, err := loadKey(); err == nil {
		t.Fatal("loadKey() expected error for multiple keys, got nil")
	}
}
