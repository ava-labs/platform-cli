//go:build !ledger

package wallet

import (
	"fmt"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto/keychain"
	"github.com/ava-labs/avalanchego/utils/crypto/secp256k1"
	"github.com/ava-labs/avalanchego/utils/set"
	"github.com/ava-labs/libevm/common"
)

// LedgerEnabled indicates whether Ledger support is compiled in.
const LedgerEnabled = false

// LedgerKeychain is a stub when Ledger support is not compiled.
type LedgerKeychain struct{}

// NewLedgerKeychain returns an error when Ledger support is not compiled.
func NewLedgerKeychain(addressIndex uint32) (*LedgerKeychain, error) {
	return nil, fmt.Errorf("ledger support not compiled. Rebuild with: go build -tags ledger")
}

// Close is a no-op for the stub.
func (kc *LedgerKeychain) Close() {}

// Addresses returns empty set for stub.
func (kc *LedgerKeychain) Addresses() set.Set[ids.ShortID] {
	return set.Set[ids.ShortID]{}
}

// Get returns nil for stub.
func (kc *LedgerKeychain) Get(addr ids.ShortID) (keychain.Signer, bool) {
	return nil, false
}

// GetAddress returns empty address for stub.
func (kc *LedgerKeychain) GetAddress() ids.ShortID {
	return ids.ShortID{}
}

// GetPublicKey returns nil for stub.
func (kc *LedgerKeychain) GetPublicKey() *secp256k1.PublicKey {
	return nil
}

// GetEVMPublicKey returns nil for stub.
func (kc *LedgerKeychain) GetEVMPublicKey() *secp256k1.PublicKey {
	return nil
}

// EthAddresses returns empty set for stub.
func (kc *LedgerKeychain) EthAddresses() set.Set[common.Address] {
	return set.Set[common.Address]{}
}

// GetEth returns nil for stub.
func (kc *LedgerKeychain) GetEth(addr common.Address) (keychain.Signer, bool) {
	return nil, false
}

// SignHash returns error for stub.
func (kc *LedgerKeychain) SignHash(hash []byte) ([]byte, error) {
	return nil, fmt.Errorf("ledger support not compiled")
}

// Sign returns error for stub.
func (kc *LedgerKeychain) Sign(msg []byte) ([]byte, error) {
	return nil, fmt.Errorf("ledger support not compiled")
}
