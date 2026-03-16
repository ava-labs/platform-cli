// Package multisig provides utilities for multisig P-Chain transactions,
// including partial signing and file-based transaction exchange between signers.
package multisig

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/crypto/keychain"
	"github.com/ava-labs/avalanchego/utils/crypto/secp256k1"
	"github.com/ava-labs/avalanchego/utils/formatting/address"
	"github.com/ava-labs/avalanchego/vms/components/verify"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
	"github.com/ava-labs/avalanchego/wallet/chain/p/signer"
)

const (
	// TxFileVersion is the current version of the tx file format.
	TxFileVersion = 1
)

// TxFile represents a partially-signed transaction that can be exchanged between signers.
type TxFile struct {
	Version   int           `json:"version"`
	NetworkID uint32        `json:"network_id"`
	TxBytes   string        `json:"tx_bytes"` // hex-encoded codec-marshaled tx
	Owners    *OwnerInfo    `json:"owners,omitempty"`
	Signers   []SignerInfo  `json:"signers"`
}

// OwnerInfo describes the multisig owner configuration (for display purposes).
type OwnerInfo struct {
	Threshold uint32   `json:"threshold"`
	Addresses []string `json:"addresses"` // formatted P-Chain addresses
}

// SignerInfo tracks whether a particular address has signed.
type SignerInfo struct {
	Address string `json:"address"` // formatted P-Chain address
	Signed  bool   `json:"signed"`
}

// NewOutputOwners creates a secp256k1fx.OutputOwners from multiple addresses and a threshold.
// Addresses are sorted for deterministic ordering as required by avalanchego.
func NewOutputOwners(addrs []ids.ShortID, threshold uint32) (*secp256k1fx.OutputOwners, error) {
	if len(addrs) == 0 {
		return nil, fmt.Errorf("at least one address is required")
	}
	if threshold == 0 {
		return nil, fmt.Errorf("threshold must be at least 1")
	}
	if threshold > uint32(len(addrs)) {
		return nil, fmt.Errorf("threshold (%d) exceeds number of addresses (%d)", threshold, len(addrs))
	}

	// Sort addresses for deterministic ordering (required by avalanchego)
	sorted := make([]ids.ShortID, len(addrs))
	copy(sorted, addrs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Compare(sorted[j]) < 0
	})

	// Check for duplicates
	for i := 1; i < len(sorted); i++ {
		if sorted[i] == sorted[i-1] {
			return nil, fmt.Errorf("duplicate address: %s", sorted[i])
		}
	}

	return &secp256k1fx.OutputOwners{
		Threshold: threshold,
		Addrs:     sorted,
	}, nil
}

// NewTxFileFromTx creates a TxFile from a partially-signed transaction.
func NewTxFileFromTx(tx *txs.Tx, networkID uint32, owners *secp256k1fx.OutputOwners) (*TxFile, error) {
	txBytes := tx.Bytes()
	if len(txBytes) == 0 {
		// If tx hasn't been initialized with bytes yet, marshal it
		var err error
		txBytes, err = txs.Codec.Marshal(txs.CodecVersion, tx)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tx: %w", err)
		}
	}

	tf := &TxFile{
		Version:   TxFileVersion,
		NetworkID: networkID,
		TxBytes:   hex.EncodeToString(txBytes),
	}

	if owners != nil {
		hrp := constants.GetHRP(networkID)
		ownerInfo := &OwnerInfo{
			Threshold: owners.Threshold,
			Addresses: make([]string, len(owners.Addrs)),
		}
		signers := make([]SignerInfo, len(owners.Addrs))
		for i, addr := range owners.Addrs {
			formatted, err := address.Format("P", hrp, addr[:])
			if err != nil {
				formatted = addr.String()
			}
			ownerInfo.Addresses[i] = formatted
			signers[i] = SignerInfo{
				Address: formatted,
				Signed:  hasSignatureForAddr(tx, addr, i),
			}
		}
		tf.Owners = ownerInfo
		tf.Signers = signers
	}

	return tf, nil
}

// hasSignatureForAddr checks if a credential contains a non-empty signature at the given index.
func hasSignatureForAddr(tx *txs.Tx, _ ids.ShortID, addrIndex int) bool {
	var emptySig [secp256k1.SignatureLen]byte

	for _, credIntf := range tx.Creds {
		cred, ok := credIntf.(*secp256k1fx.Credential)
		if !ok {
			continue
		}
		if addrIndex < len(cred.Sigs) && cred.Sigs[addrIndex] != emptySig {
			return true
		}
	}
	return false
}

// WriteTxFile writes a TxFile to disk as JSON.
func WriteTxFile(path string, tf *TxFile) error {
	data, err := json.MarshalIndent(tf, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal tx file: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write tx file: %w", err)
	}
	return nil
}

// ReadTxFile reads a TxFile from disk.
func ReadTxFile(path string) (*TxFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read tx file: %w", err)
	}
	var tf TxFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return nil, fmt.Errorf("failed to parse tx file: %w", err)
	}
	if tf.Version != TxFileVersion {
		return nil, fmt.Errorf("unsupported tx file version %d (expected %d)", tf.Version, TxFileVersion)
	}
	return &tf, nil
}

// ParseTx parses the transaction bytes from a TxFile.
func ParseTx(tf *TxFile) (*txs.Tx, error) {
	txBytes, err := hex.DecodeString(tf.TxBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to decode tx bytes: %w", err)
	}
	tx, err := txs.Parse(txs.Codec, txBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse tx: %w", err)
	}
	return tx, nil
}

// SignTx adds signatures from the provided keychain to the transaction.
// It uses the avalanchego signer which natively supports partial signing —
// it fills in signatures for keys it has and leaves empty sigs for keys it doesn't.
func SignTx(ctx context.Context, tx *txs.Tx, kc keychain.Keychain, backend signer.Backend) error {
	s := signer.New(kc, backend)
	return s.Sign(ctx, tx)
}

// UpdateTxFileAfterSign re-encodes the tx and updates signer status after signing.
func UpdateTxFileAfterSign(tf *TxFile, tx *txs.Tx) error {
	txBytes := tx.Bytes()
	if len(txBytes) == 0 {
		var err error
		txBytes, err = txs.Codec.Marshal(txs.CodecVersion, tx)
		if err != nil {
			return fmt.Errorf("failed to marshal signed tx: %w", err)
		}
	}
	tf.TxBytes = hex.EncodeToString(txBytes)

	// Update signer status
	if tf.Owners != nil {
		for i := range tf.Signers {
			tf.Signers[i].Signed = hasNonEmptySigAtIndex(tx, i)
		}
	}

	return nil
}

// hasNonEmptySigAtIndex checks if any credential has a non-empty signature at the given index.
func hasNonEmptySigAtIndex(tx *txs.Tx, sigIndex int) bool {
	var emptySig [secp256k1.SignatureLen]byte

	for _, credIntf := range tx.Creds {
		cred, ok := credIntf.(*secp256k1fx.Credential)
		if !ok {
			continue
		}
		if sigIndex < len(cred.Sigs) && cred.Sigs[sigIndex] != emptySig {
			return true
		}
	}
	return false
}

// IsFullySigned checks if all required signatures are present.
// It checks all credentials for empty signature slots.
func IsFullySigned(tx *txs.Tx) bool {
	var emptySig [secp256k1.SignatureLen]byte

	for _, credIntf := range tx.Creds {
		cred, ok := credIntf.(*secp256k1fx.Credential)
		if !ok {
			continue
		}
		for _, sig := range cred.Sigs {
			if sig == emptySig {
				return false
			}
		}
	}
	return true
}

// SignatureStatus returns a human-readable summary of signature status.
func SignatureStatus(tf *TxFile) string {
	if tf.Owners == nil || len(tf.Signers) == 0 {
		return "unknown"
	}

	signed := 0
	for _, s := range tf.Signers {
		if s.Signed {
			signed++
		}
	}

	return fmt.Sprintf("%d/%d signatures (%d required)",
		signed, len(tf.Signers), tf.Owners.Threshold)
}

// CredentialSignatureStatus returns the number of non-empty signatures across all credentials.
func CredentialSignatureStatus(tx *txs.Tx) (filled, total int) {
	var emptySig [secp256k1.SignatureLen]byte

	for _, credIntf := range tx.Creds {
		cred, ok := credIntf.(*secp256k1fx.Credential)
		if !ok {
			continue
		}
		for _, sig := range cred.Sigs {
			total++
			if sig != emptySig {
				filled++
			}
		}
	}
	return filled, total
}

// ParseAddresses parses comma-separated P-Chain addresses to ShortIDs.
// Accepts formats: "P-fuji1...", "P-avax1...", or raw short ID strings.
func ParseAddresses(addrList string, expectedHRP string) ([]ids.ShortID, error) {
	if addrList == "" {
		return nil, fmt.Errorf("address list cannot be empty")
	}

	var addrs []ids.ShortID
	for _, raw := range splitAndTrim(addrList) {
		addr, err := parseAddress(raw, expectedHRP)
		if err != nil {
			return nil, fmt.Errorf("invalid address %q: %w", raw, err)
		}
		addrs = append(addrs, addr)
	}

	if len(addrs) == 0 {
		return nil, fmt.Errorf("no valid addresses found")
	}
	return addrs, nil
}

// parseAddress parses a single P-Chain address.
func parseAddress(raw string, expectedHRP string) (ids.ShortID, error) {
	// Try parsing as "P-<hrp>1..." bech32 format
	_, _, addrBytes, err := address.Parse(raw)
	if err == nil {
		addr, err := ids.ToShortID(addrBytes)
		if err != nil {
			return ids.ShortEmpty, fmt.Errorf("invalid address bytes: %w", err)
		}
		return addr, nil
	}

	// Try parsing as raw short ID
	addr, err := ids.ShortFromString(raw)
	if err != nil {
		return ids.ShortEmpty, fmt.Errorf("could not parse as P-Chain address or short ID: %s", raw)
	}
	return addr, nil
}

// splitAndTrim splits a comma-separated string and trims whitespace.
func splitAndTrim(s string) []string {
	var result []string
	for _, part := range splitComma(s) {
		trimmed := trimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func splitComma(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// FormatSignerStatus returns a formatted table of signer addresses and their status.
func FormatSignerStatus(tf *TxFile) string {
	if tf.Owners == nil || len(tf.Signers) == 0 {
		return ""
	}

	result := fmt.Sprintf("Threshold: %d of %d\n", tf.Owners.Threshold, len(tf.Signers))
	for _, s := range tf.Signers {
		status := "[ ]"
		if s.Signed {
			status = "[x]"
		}
		result += fmt.Sprintf("  %s %s\n", status, s.Address)
	}
	return result
}

// EmptyCredentials returns a slice of empty credentials matching the expected count.
// This is used when building an unsigned transaction that needs credential placeholders.
func EmptyCredentials(count int, sigsPerCred []int) []verify.Verifiable {
	creds := make([]verify.Verifiable, count)
	for i := 0; i < count; i++ {
		sigs := 1
		if i < len(sigsPerCred) {
			sigs = sigsPerCred[i]
		}
		creds[i] = &secp256k1fx.Credential{
			Sigs: make([][secp256k1.SignatureLen]byte, sigs),
		}
	}
	return creds
}
