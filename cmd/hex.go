package cmd

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// decodeHex decodes a hex string while accepting optional 0x/0X prefix.
func decodeHex(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")

	decoded, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	return decoded, nil
}

// decodeHexExactLength decodes hex and enforces exact decoded byte length.
func decodeHexExactLength(s string, expected int) ([]byte, error) {
	decoded, err := decodeHex(s)
	if err != nil {
		return nil, err
	}
	if len(decoded) != expected {
		return nil, fmt.Errorf("invalid length: expected %d bytes, got %d", expected, len(decoded))
	}
	return decoded, nil
}
