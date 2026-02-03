package wallet

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/cb58"
	"github.com/ava-labs/avalanchego/utils/crypto/secp256k1"
)

// ParsePrivateKey parses a private key from various formats.
// Supported formats:
//   - PrivateKey-... (Avalanche CB58 format)
//   - 0x... (hex format)
//   - Raw CB58 string
//   - Raw hex string
func ParsePrivateKey(keyStr string) ([]byte, error) {
	keyStr = strings.TrimSpace(keyStr)

	var keyBytes []byte
	var err error

	// Check format and parse accordingly
	if strings.HasPrefix(keyStr, "0x") || strings.HasPrefix(keyStr, "0X") {
		// Hex format (0x...)
		keyBytes, err = hex.DecodeString(strings.TrimPrefix(strings.TrimPrefix(keyStr, "0x"), "0X"))
		if err != nil {
			return nil, fmt.Errorf("failed to decode hex private key: %w", err)
		}
	} else if strings.HasPrefix(keyStr, "PrivateKey-") {
		// Avalanche CB58 format (PrivateKey-...)
		keyStr = strings.TrimPrefix(keyStr, "PrivateKey-")
		keyBytes, err = cb58.Decode(keyStr)
		if err != nil {
			return nil, fmt.Errorf("failed to decode CB58 private key: %w", err)
		}
	} else {
		// Try CB58 without prefix
		keyBytes, err = cb58.Decode(keyStr)
		if err != nil {
			// Try hex without prefix
			keyBytes, err = hex.DecodeString(keyStr)
			if err != nil {
				return nil, fmt.Errorf("failed to decode private key (tried CB58 and hex): %w", err)
			}
		}
	}

	return keyBytes, nil
}

// ToPrivateKey converts raw key bytes to a secp256k1 private key.
func ToPrivateKey(keyBytes []byte) (*secp256k1.PrivateKey, error) {
	key, err := secp256k1.ToPrivateKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}
	return key, nil
}

// DeriveAddresses derives both P-Chain and EVM addresses from a private key.
func DeriveAddresses(keyBytes []byte) (pAddr string, evmAddr string) {
	key, err := ToPrivateKey(keyBytes)
	if err != nil {
		return "", ""
	}

	// P-Chain address
	pAddr = key.Address().String()

	// EVM address
	evmAddr = deriveEthAddress(key)

	return pAddr, evmAddr
}

// DeriveAddressesFromKey derives addresses from a secp256k1 private key.
func DeriveAddressesFromKey(key *secp256k1.PrivateKey) (pAddr ids.ShortID, evmAddr string) {
	return key.Address(), deriveEthAddress(key)
}

// deriveEthAddress derives an Ethereum address from a secp256k1 private key.
func deriveEthAddress(key *secp256k1.PrivateKey) string {
	// Use the SDK's proper EVM address derivation
	return key.PublicKey().EthAddress().Hex()
}


// KeyToHex converts a private key to hex format with 0x prefix.
func KeyToHex(key *secp256k1.PrivateKey) string {
	return "0x" + hex.EncodeToString(key.Bytes())
}
