package keystore

import "time"

// KeyIndex represents the main index file that tracks all stored keys.
type KeyIndex struct {
	Version int                 `json:"version"`
	Default string              `json:"default,omitempty"`
	Keys    map[string]KeyEntry `json:"keys"`
}

// KeyEntry represents metadata about a stored key in the index.
type KeyEntry struct {
	Name          string    `json:"name"`
	Encrypted     bool      `json:"encrypted"`
	PChainAddress string    `json:"p_chain_address"`
	EVMAddress    string    `json:"evm_address"`
	CreatedAt     time.Time `json:"created_at"`
}

// KeyFile represents an individual key file (encrypted or plain).
type KeyFile struct {
	Version int `json:"version"`

	// For unencrypted keys
	Format string `json:"format,omitempty"` // "cb58" or "hex"
	Key    string `json:"key,omitempty"`

	// For encrypted keys
	Encrypted  bool   `json:"encrypted,omitempty"`
	Salt       string `json:"salt,omitempty"`       // Base64-encoded
	Nonce      string `json:"nonce,omitempty"`      // Base64-encoded
	Ciphertext string `json:"ciphertext,omitempty"` // Base64-encoded
}

// NewKeyIndex creates an empty key index.
func NewKeyIndex() *KeyIndex {
	return &KeyIndex{
		Version: 1,
		Keys:    make(map[string]KeyEntry),
	}
}
