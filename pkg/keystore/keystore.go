package keystore

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ava-labs/avalanchego/utils/cb58"
	"github.com/ava-labs/avalanchego/utils/crypto/secp256k1"
	"github.com/ava-labs/platform-cli/pkg/wallet"
)

// clearKeyBytes securely zeros a byte slice to prevent sensitive data from lingering in memory.
func clearKeyBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

const (
	keystoreDir  = ".platform"
	keysDir      = "keys"
	indexFile    = "keys.json"
	keyExtension = ".key"
)

var keyNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// ValidateKeyName validates a key name for safe filesystem usage.
func ValidateKeyName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("key name cannot be empty")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid key name %q", name)
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("invalid key name %q: path separators are not allowed", name)
	}
	if !keyNamePattern.MatchString(name) {
		return fmt.Errorf("invalid key name %q: use 1-64 characters [a-zA-Z0-9._-], starting with alphanumeric", name)
	}
	return nil
}

// KeyStore manages persistent key storage.
type KeyStore struct {
	basePath string
	index    *KeyIndex
}

// DefaultPath returns the default keystore path (~/.platform/keys).
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, keystoreDir, keysDir), nil
}

// Load loads the keystore from the default location.
func Load() (*KeyStore, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return LoadFrom(path)
}

// LoadFrom loads the keystore from a specific path.
func LoadFrom(basePath string) (*KeyStore, error) {
	ks := &KeyStore{
		basePath: basePath,
	}

	// Ensure directory exists
	if err := os.MkdirAll(basePath, 0700); err != nil {
		return nil, fmt.Errorf("failed to create keystore directory: %w", err)
	}

	// Load or create index
	indexPath := filepath.Join(basePath, indexFile)
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			ks.index = NewKeyIndex()
			return ks, nil
		}
		return nil, fmt.Errorf("failed to read index: %w", err)
	}

	ks.index = &KeyIndex{}
	if err := json.Unmarshal(data, ks.index); err != nil {
		return nil, fmt.Errorf("failed to parse index: %w", err)
	}

	// Initialize map if nil (for older versions)
	if ks.index.Keys == nil {
		ks.index.Keys = make(map[string]KeyEntry)
	}

	return ks, nil
}

// Save persists the keystore index to disk.
func (ks *KeyStore) Save() error {
	indexPath := filepath.Join(ks.basePath, indexFile)
	data, err := json.MarshalIndent(ks.index, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}
	if err := os.WriteFile(indexPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}
	return nil
}

// ImportKey imports a private key with the given name.
// If password is provided, the key will be encrypted.
func (ks *KeyStore) ImportKey(name string, keyBytes []byte, password []byte) error {
	if err := ValidateKeyName(name); err != nil {
		return err
	}

	// Check if name already exists
	if _, exists := ks.index.Keys[name]; exists {
		return fmt.Errorf("key with name %q already exists", name)
	}

	// Validate key by parsing it
	key, err := secp256k1.ToPrivateKey(keyBytes)
	if err != nil {
		return fmt.Errorf("invalid private key: %w", err)
	}

	// Derive addresses
	pAddr, evmAddr := wallet.DeriveAddresses(keyBytes)

	// Create key file
	keyFile := &KeyFile{
		Version: 1,
	}

	if len(password) > 0 {
		// Encrypt the key
		salt, nonce, ciphertext, err := Encrypt(keyBytes, password)
		if err != nil {
			return fmt.Errorf("failed to encrypt key: %w", err)
		}
		keyFile.Encrypted = true
		keyFile.Salt = salt
		keyFile.Nonce = nonce
		keyFile.Ciphertext = ciphertext
	} else {
		// Store in CB58 format
		keyFile.Format = "cb58"
		encoded, err := cb58.Encode(key.Bytes())
		if err != nil {
			return fmt.Errorf("failed to encode key: %w", err)
		}
		keyFile.Key = "PrivateKey-" + encoded
	}

	// Write key file
	keyPath := filepath.Join(ks.basePath, name+keyExtension)
	data, err := json.MarshalIndent(keyFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal key file: %w", err)
	}
	if err := os.WriteFile(keyPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write key file: %w", err)
	}

	// Update index
	ks.index.Keys[name] = KeyEntry{
		Name:          name,
		Encrypted:     len(password) > 0,
		PChainAddress: pAddr,
		EVMAddress:    evmAddr,
		CreatedAt:     time.Now().UTC(),
	}

	// Set as default if it's the first key
	if len(ks.index.Keys) == 1 {
		ks.index.Default = name
	}

	return ks.Save()
}

// GenerateKey generates a new random key with the given name.
// If password is provided, the key will be encrypted.
// Note: The returned key bytes should be cleared by the caller when no longer needed.
func (ks *KeyStore) GenerateKey(name string, password []byte) ([]byte, error) {
	if err := ValidateKeyName(name); err != nil {
		return nil, err
	}

	// Generate random key bytes
	keyBytes := make([]byte, secp256k1.PrivateKeyLen)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}

	// Import it (which validates and stores it)
	if err := ks.ImportKey(name, keyBytes, password); err != nil {
		// Clear key bytes on error before returning
		clearKeyBytes(keyBytes)
		return nil, err
	}

	return keyBytes, nil
}

// LoadKey loads a key by name. If the key is encrypted, password must be provided.
// Note: The returned key bytes should be cleared by the caller when no longer needed.
func (ks *KeyStore) LoadKey(name string, password []byte) ([]byte, error) {
	if err := ValidateKeyName(name); err != nil {
		return nil, err
	}

	entry, exists := ks.index.Keys[name]
	if !exists {
		return nil, fmt.Errorf("key %q not found", name)
	}

	// Read key file
	keyPath := filepath.Join(ks.basePath, name+keyExtension)
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	var keyFile KeyFile
	if err := json.Unmarshal(data, &keyFile); err != nil {
		return nil, fmt.Errorf("failed to parse key file: %w", err)
	}

	if keyFile.Encrypted || entry.Encrypted {
		if len(password) == 0 {
			return nil, fmt.Errorf("key %q is encrypted, password required", name)
		}
		return Decrypt(keyFile.Salt, keyFile.Nonce, keyFile.Ciphertext, password)
	}

	// Parse unencrypted key
	return wallet.ParsePrivateKey(keyFile.Key)
}

// DeleteKey removes a key by name.
func (ks *KeyStore) DeleteKey(name string) error {
	if err := ValidateKeyName(name); err != nil {
		return err
	}

	if _, exists := ks.index.Keys[name]; !exists {
		return fmt.Errorf("key %q not found", name)
	}

	// Remove key file
	keyPath := filepath.Join(ks.basePath, name+keyExtension)
	if err := os.Remove(keyPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove key file: %w", err)
	}

	// Update index
	delete(ks.index.Keys, name)

	// Clear default if it was the deleted key
	if ks.index.Default == name {
		ks.index.Default = ""
		// Set a new default if there are remaining keys
		for k := range ks.index.Keys {
			ks.index.Default = k
			break
		}
	}

	return ks.Save()
}

// ListKeys returns all key entries.
func (ks *KeyStore) ListKeys() []KeyEntry {
	entries := make([]KeyEntry, 0, len(ks.index.Keys))
	for _, entry := range ks.index.Keys {
		entries = append(entries, entry)
	}
	return entries
}

// GetKey returns metadata for a specific key.
func (ks *KeyStore) GetKey(name string) (KeyEntry, bool) {
	entry, exists := ks.index.Keys[name]
	return entry, exists
}

// SetDefault sets the default key name.
func (ks *KeyStore) SetDefault(name string) error {
	if err := ValidateKeyName(name); err != nil {
		return err
	}

	if _, exists := ks.index.Keys[name]; !exists {
		return fmt.Errorf("key %q not found", name)
	}
	ks.index.Default = name
	return ks.Save()
}

// GetDefault returns the default key name.
func (ks *KeyStore) GetDefault() string {
	return ks.index.Default
}

// HasKey checks if a key with the given name exists.
func (ks *KeyStore) HasKey(name string) bool {
	_, exists := ks.index.Keys[name]
	return exists
}

// IsEncrypted checks if a key is encrypted.
func (ks *KeyStore) IsEncrypted(name string) bool {
	entry, exists := ks.index.Keys[name]
	if !exists {
		return false
	}
	return entry.Encrypted
}

// ExportKey exports a key in the specified format.
// If the key is encrypted, password must be provided.
func (ks *KeyStore) ExportKey(name string, password []byte, format string) (string, error) {
	if err := ValidateKeyName(name); err != nil {
		return "", err
	}

	keyBytes, err := ks.LoadKey(name, password)
	if err != nil {
		return "", err
	}
	// Clear key bytes after encoding
	defer clearKeyBytes(keyBytes)

	switch format {
	case "cb58", "":
		encoded, err := cb58.Encode(keyBytes)
		if err != nil {
			return "", fmt.Errorf("failed to encode key: %w", err)
		}
		return "PrivateKey-" + encoded, nil
	case "hex":
		return fmt.Sprintf("0x%x", keyBytes), nil
	default:
		return "", fmt.Errorf("unsupported format: %s (use cb58 or hex)", format)
	}
}

// KeyCount returns the number of stored keys.
func (ks *KeyStore) KeyCount() int {
	return len(ks.index.Keys)
}
