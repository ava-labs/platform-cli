package keystore

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// testKeyBytes is a valid secp256k1 private key for testing.
var testKeyBytes = []byte{
	0x56, 0x28, 0x9e, 0x99, 0xc9, 0x4b, 0x69, 0x12,
	0xbf, 0xc1, 0x2a, 0xdc, 0x09, 0x3c, 0x9b, 0x51,
	0x12, 0x4f, 0x0d, 0xc5, 0x4a, 0xc7, 0xa7, 0x66,
	0xb2, 0xbc, 0x5c, 0xcf, 0x55, 0x8d, 0x80, 0x27,
}

func setupTestKeystore(t *testing.T) (*KeyStore, string) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "keystore-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	ks, err := LoadFrom(tempDir)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("failed to load keystore: %v", err)
	}

	return ks, tempDir
}

func TestLoadFrom_CreatesDirectory(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "keystore-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	newPath := filepath.Join(tempDir, "new", "keystore")
	ks, err := LoadFrom(newPath)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}

	if ks == nil {
		t.Fatal("LoadFrom() returned nil")
	}

	// Directory should exist
	if _, err := os.Stat(newPath); os.IsNotExist(err) {
		t.Error("LoadFrom() did not create directory")
	}
}

func TestLoadFrom_EnforcesSecureDirectoryPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not portable on windows")
	}

	tempDir, err := os.MkdirTemp("", "keystore-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ksPath := filepath.Join(tempDir, "keys")
	if err := os.MkdirAll(ksPath, 0755); err != nil {
		t.Fatalf("failed to create keystore dir: %v", err)
	}

	if _, err := LoadFrom(ksPath); err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}

	info, err := os.Stat(ksPath)
	if err != nil {
		t.Fatalf("failed to stat keystore dir: %v", err)
	}
	if got := info.Mode().Perm(); got != 0700 {
		t.Fatalf("keystore dir perms = %o, want 700", got)
	}
}

func TestWriteFileAtomic(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "keystore-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	path := filepath.Join(tempDir, "atomic.json")
	first := []byte("first")
	second := []byte("second")

	if err := writeFileAtomic(path, first, 0600); err != nil {
		t.Fatalf("writeFileAtomic(first) error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(got) != string(first) {
		t.Fatalf("file content = %q, want %q", got, first)
	}

	if err := writeFileAtomic(path, second, 0600); err != nil {
		t.Fatalf("writeFileAtomic(second) error = %v", err)
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file after overwrite: %v", err)
	}
	if string(got) != string(second) {
		t.Fatalf("file content after overwrite = %q, want %q", got, second)
	}
}

func TestKeyStore_ImportKey_Unencrypted(t *testing.T) {
	ks, tempDir := setupTestKeystore(t)
	defer os.RemoveAll(tempDir)

	err := ks.ImportKey("testkey", testKeyBytes, nil)
	if err != nil {
		t.Fatalf("ImportKey() error = %v", err)
	}

	// Key should exist
	if !ks.HasKey("testkey") {
		t.Error("HasKey() = false after import")
	}

	// Should not be encrypted
	if ks.IsEncrypted("testkey") {
		t.Error("IsEncrypted() = true for unencrypted key")
	}

	// Load the key back
	loaded, err := ks.LoadKey("testkey", nil)
	if err != nil {
		t.Fatalf("LoadKey() error = %v", err)
	}

	if len(loaded) != len(testKeyBytes) {
		t.Errorf("LoadKey() returned %d bytes, want %d", len(loaded), len(testKeyBytes))
	}

	// Verify it's the first key and set as default
	if ks.GetDefault() != "testkey" {
		t.Error("First key should be set as default")
	}
}

func TestKeyStore_ImportKey_Encrypted(t *testing.T) {
	ks, tempDir := setupTestKeystore(t)
	defer os.RemoveAll(tempDir)

	password := []byte("testpassword123")
	err := ks.ImportKey("encryptedkey", testKeyBytes, password)
	if err != nil {
		t.Fatalf("ImportKey() error = %v", err)
	}

	// Key should exist and be encrypted
	if !ks.HasKey("encryptedkey") {
		t.Error("HasKey() = false after import")
	}
	if !ks.IsEncrypted("encryptedkey") {
		t.Error("IsEncrypted() = false for encrypted key")
	}

	// Loading without password should fail
	_, err = ks.LoadKey("encryptedkey", nil)
	if err == nil {
		t.Error("LoadKey() without password should fail for encrypted key")
	}

	// Loading with wrong password should fail
	_, err = ks.LoadKey("encryptedkey", []byte("wrongpassword"))
	if err == nil {
		t.Error("LoadKey() with wrong password should fail")
	}

	// Loading with correct password should succeed
	loaded, err := ks.LoadKey("encryptedkey", password)
	if err != nil {
		t.Fatalf("LoadKey() with correct password error = %v", err)
	}

	if len(loaded) != len(testKeyBytes) {
		t.Errorf("LoadKey() returned %d bytes, want %d", len(loaded), len(testKeyBytes))
	}
}

func TestKeyStore_ImportKey_Duplicate(t *testing.T) {
	ks, tempDir := setupTestKeystore(t)
	defer os.RemoveAll(tempDir)

	err := ks.ImportKey("testkey", testKeyBytes, nil)
	if err != nil {
		t.Fatalf("ImportKey() first call error = %v", err)
	}

	// Importing with same name should fail
	err = ks.ImportKey("testkey", testKeyBytes, nil)
	if err == nil {
		t.Error("ImportKey() with duplicate name should fail")
	}
}

func TestKeyStore_GenerateKey(t *testing.T) {
	ks, tempDir := setupTestKeystore(t)
	defer os.RemoveAll(tempDir)

	keyBytes, err := ks.GenerateKey("generated", nil)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	if len(keyBytes) != 32 {
		t.Errorf("GenerateKey() returned %d bytes, want 32", len(keyBytes))
	}

	// Key should exist
	if !ks.HasKey("generated") {
		t.Error("HasKey() = false after generate")
	}

	// Load and verify
	loaded, err := ks.LoadKey("generated", nil)
	if err != nil {
		t.Fatalf("LoadKey() error = %v", err)
	}

	if len(loaded) != 32 {
		t.Errorf("LoadKey() returned %d bytes, want 32", len(loaded))
	}
}

func TestKeyStore_DeleteKey(t *testing.T) {
	ks, tempDir := setupTestKeystore(t)
	defer os.RemoveAll(tempDir)

	err := ks.ImportKey("todelete", testKeyBytes, nil)
	if err != nil {
		t.Fatalf("ImportKey() error = %v", err)
	}

	if !ks.HasKey("todelete") {
		t.Fatal("HasKey() = false after import")
	}

	err = ks.DeleteKey("todelete")
	if err != nil {
		t.Fatalf("DeleteKey() error = %v", err)
	}

	if ks.HasKey("todelete") {
		t.Error("HasKey() = true after delete")
	}

	// Deleting non-existent key should fail
	err = ks.DeleteKey("nonexistent")
	if err == nil {
		t.Error("DeleteKey() on non-existent key should fail")
	}
}

func TestKeyStore_SetDefault(t *testing.T) {
	ks, tempDir := setupTestKeystore(t)
	defer os.RemoveAll(tempDir)

	// Import two keys
	err := ks.ImportKey("key1", testKeyBytes, nil)
	if err != nil {
		t.Fatalf("ImportKey() key1 error = %v", err)
	}

	// Modify key slightly for second import
	key2Bytes := make([]byte, len(testKeyBytes))
	copy(key2Bytes, testKeyBytes)
	key2Bytes[0] ^= 0xFF // Flip bits to make it different but still valid

	err = ks.ImportKey("key2", key2Bytes, nil)
	if err != nil {
		t.Fatalf("ImportKey() key2 error = %v", err)
	}

	// First key should be default
	if ks.GetDefault() != "key1" {
		t.Errorf("GetDefault() = %s, want key1", ks.GetDefault())
	}

	// Set key2 as default
	err = ks.SetDefault("key2")
	if err != nil {
		t.Fatalf("SetDefault() error = %v", err)
	}

	if ks.GetDefault() != "key2" {
		t.Errorf("GetDefault() = %s, want key2", ks.GetDefault())
	}

	// Setting non-existent key as default should fail
	err = ks.SetDefault("nonexistent")
	if err == nil {
		t.Error("SetDefault() on non-existent key should fail")
	}
}

func TestKeyStore_ListKeys(t *testing.T) {
	ks, tempDir := setupTestKeystore(t)
	defer os.RemoveAll(tempDir)

	// Empty keystore
	entries := ks.ListKeys()
	if len(entries) != 0 {
		t.Errorf("ListKeys() on empty keystore returned %d entries", len(entries))
	}

	// Add a key
	err := ks.ImportKey("testkey", testKeyBytes, nil)
	if err != nil {
		t.Fatalf("ImportKey() error = %v", err)
	}

	entries = ks.ListKeys()
	if len(entries) != 1 {
		t.Errorf("ListKeys() returned %d entries, want 1", len(entries))
	}

	if entries[0].Name != "testkey" {
		t.Errorf("ListKeys()[0].Name = %s, want testkey", entries[0].Name)
	}
}

func TestKeyStore_GetKey(t *testing.T) {
	ks, tempDir := setupTestKeystore(t)
	defer os.RemoveAll(tempDir)

	err := ks.ImportKey("testkey", testKeyBytes, nil)
	if err != nil {
		t.Fatalf("ImportKey() error = %v", err)
	}

	entry, exists := ks.GetKey("testkey")
	if !exists {
		t.Fatal("GetKey() exists = false")
	}

	if entry.Name != "testkey" {
		t.Errorf("GetKey().Name = %s, want testkey", entry.Name)
	}

	if entry.PChainAddress == "" {
		t.Error("GetKey().PChainAddress is empty")
	}

	if entry.EVMAddress == "" {
		t.Error("GetKey().EVMAddress is empty")
	}

	// Non-existent key
	_, exists = ks.GetKey("nonexistent")
	if exists {
		t.Error("GetKey() exists = true for non-existent key")
	}
}

func TestKeyStore_ExportKey(t *testing.T) {
	ks, tempDir := setupTestKeystore(t)
	defer os.RemoveAll(tempDir)

	err := ks.ImportKey("testkey", testKeyBytes, nil)
	if err != nil {
		t.Fatalf("ImportKey() error = %v", err)
	}

	// Export as CB58
	cb58Export, err := ks.ExportKey("testkey", nil, "cb58")
	if err != nil {
		t.Fatalf("ExportKey() cb58 error = %v", err)
	}

	if cb58Export == "" {
		t.Error("ExportKey() cb58 returned empty string")
	}

	if cb58Export[:11] != "PrivateKey-" {
		t.Errorf("ExportKey() cb58 should start with 'PrivateKey-', got %s", cb58Export[:11])
	}

	// Export as hex
	hexExport, err := ks.ExportKey("testkey", nil, "hex")
	if err != nil {
		t.Fatalf("ExportKey() hex error = %v", err)
	}

	if hexExport == "" {
		t.Error("ExportKey() hex returned empty string")
	}

	if hexExport[:2] != "0x" {
		t.Errorf("ExportKey() hex should start with '0x', got %s", hexExport[:2])
	}

	// Invalid format
	_, err = ks.ExportKey("testkey", nil, "invalid")
	if err == nil {
		t.Error("ExportKey() with invalid format should fail")
	}
}

func TestKeyStore_KeyCount(t *testing.T) {
	ks, tempDir := setupTestKeystore(t)
	defer os.RemoveAll(tempDir)

	if ks.KeyCount() != 0 {
		t.Errorf("KeyCount() on empty keystore = %d, want 0", ks.KeyCount())
	}

	err := ks.ImportKey("key1", testKeyBytes, nil)
	if err != nil {
		t.Fatalf("ImportKey() error = %v", err)
	}

	if ks.KeyCount() != 1 {
		t.Errorf("KeyCount() = %d, want 1", ks.KeyCount())
	}
}

func TestKeyStore_Persistence(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "keystore-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create keystore and add a key
	ks1, err := LoadFrom(tempDir)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}

	err = ks1.ImportKey("persistent", testKeyBytes, nil)
	if err != nil {
		t.Fatalf("ImportKey() error = %v", err)
	}

	// Load keystore again (simulating restart)
	ks2, err := LoadFrom(tempDir)
	if err != nil {
		t.Fatalf("LoadFrom() second time error = %v", err)
	}

	// Key should still exist
	if !ks2.HasKey("persistent") {
		t.Error("Key not persisted after reload")
	}

	loaded, err := ks2.LoadKey("persistent", nil)
	if err != nil {
		t.Fatalf("LoadKey() after reload error = %v", err)
	}

	if len(loaded) != len(testKeyBytes) {
		t.Errorf("LoadKey() after reload returned %d bytes, want %d", len(loaded), len(testKeyBytes))
	}
}

func TestKeyStore_DeleteDefaultKey(t *testing.T) {
	ks, tempDir := setupTestKeystore(t)
	defer os.RemoveAll(tempDir)

	// Import two keys
	err := ks.ImportKey("key1", testKeyBytes, nil)
	if err != nil {
		t.Fatalf("ImportKey() key1 error = %v", err)
	}

	key2Bytes := make([]byte, len(testKeyBytes))
	copy(key2Bytes, testKeyBytes)
	key2Bytes[0] ^= 0xFF

	err = ks.ImportKey("key2", key2Bytes, nil)
	if err != nil {
		t.Fatalf("ImportKey() key2 error = %v", err)
	}

	// Delete the default key
	err = ks.DeleteKey("key1")
	if err != nil {
		t.Fatalf("DeleteKey() error = %v", err)
	}

	// Default should be updated to remaining key
	if ks.GetDefault() != "key2" {
		t.Errorf("GetDefault() after deleting default = %s, want key2", ks.GetDefault())
	}
}

func TestNewKeyIndex(t *testing.T) {
	idx := NewKeyIndex()
	if idx == nil {
		t.Fatal("NewKeyIndex() returned nil")
	}
	if idx.Version != 1 {
		t.Errorf("NewKeyIndex().Version = %d, want 1", idx.Version)
	}
	if idx.Keys == nil {
		t.Error("NewKeyIndex().Keys is nil")
	}
	if len(idx.Keys) != 0 {
		t.Errorf("NewKeyIndex().Keys has %d entries, want 0", len(idx.Keys))
	}
}

func TestValidateKeyName_Invalid(t *testing.T) {
	tests := []string{
		"",
		".",
		"..",
		"../../evil",
		"..\\..\\evil",
		"/tmp/key",
		"spaces not allowed",
		"-starts-with-dash",
	}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			if err := ValidateKeyName(name); err == nil {
				t.Fatalf("ValidateKeyName(%q) should fail", name)
			}
		})
	}
}

func TestKeyStore_ImportKey_RejectsUnsafeName(t *testing.T) {
	ks, tempDir := setupTestKeystore(t)
	defer os.RemoveAll(tempDir)

	err := ks.ImportKey("../../outside", testKeyBytes, nil)
	if err == nil {
		t.Fatal("ImportKey should fail for unsafe key name")
	}
}
