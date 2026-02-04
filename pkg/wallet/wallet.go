// Package wallet provides P-Chain wallet utilities for Avalanche.
package wallet

import (
	"context"
	"fmt"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto/keychain"
	"github.com/ava-labs/avalanchego/utils/crypto/secp256k1"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
	"github.com/ava-labs/avalanchego/wallet/chain/c"
	pwallet "github.com/ava-labs/avalanchego/wallet/chain/p/wallet"
	"github.com/ava-labs/avalanchego/wallet/subnet/primary"
	"github.com/ava-labs/libevm/common"
	"github.com/ava-labs/platform-cli/pkg/network"
)

// Wallet wraps the avalanchego wallet for P-Chain operations.
type Wallet struct {
	key       *secp256k1.PrivateKey   // nil for Ledger
	keychain  *secp256k1fx.Keychain   // nil for Ledger
	pWallet   pwallet.Wallet
	config    network.Config
	address   ids.ShortID             // used when key is nil (Ledger mode)
}

// NewWallet creates a new wallet for P-Chain operations.
func NewWallet(ctx context.Context, key *secp256k1.PrivateKey, config network.Config) (*Wallet, error) {
	kc := secp256k1fx.NewKeychain(key)

	pWallet, err := primary.MakePWallet(ctx, config.RPCURL, kc, primary.WalletConfig{})
	if err != nil {
		return nil, fmt.Errorf("failed to create P-Chain wallet: %w", err)
	}

	return &Wallet{
		key:      key,
		keychain: kc,
		pWallet:  pWallet,
		config:   config,
	}, nil
}

// NewWalletWithSubnet creates a wallet that tracks a specific subnet.
func NewWalletWithSubnet(ctx context.Context, key *secp256k1.PrivateKey, config network.Config, subnetID ids.ID) (*Wallet, error) {
	kc := secp256k1fx.NewKeychain(key)

	pWallet, err := primary.MakePWallet(ctx, config.RPCURL, kc, primary.WalletConfig{
		SubnetIDs: []ids.ID{subnetID},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create P-Chain wallet: %w", err)
	}

	return &Wallet{
		key:      key,
		keychain: kc,
		pWallet:  pWallet,
		config:   config,
	}, nil
}

// NewWalletFromKeychain creates a wallet from any keychain implementation (e.g., Ledger).
func NewWalletFromKeychain(ctx context.Context, kc keychain.Keychain, address ids.ShortID, config network.Config) (*Wallet, error) {
	pWallet, err := primary.MakePWallet(ctx, config.RPCURL, kc, primary.WalletConfig{})
	if err != nil {
		return nil, fmt.Errorf("failed to create P-Chain wallet: %w", err)
	}

	return &Wallet{
		pWallet: pWallet,
		config:  config,
		address: address,
	}, nil
}

// NewWalletFromKeychainWithSubnet creates a wallet from any keychain with subnet tracking.
func NewWalletFromKeychainWithSubnet(ctx context.Context, kc keychain.Keychain, address ids.ShortID, config network.Config, subnetID ids.ID) (*Wallet, error) {
	pWallet, err := primary.MakePWallet(ctx, config.RPCURL, kc, primary.WalletConfig{
		SubnetIDs: []ids.ID{subnetID},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create P-Chain wallet: %w", err)
	}

	return &Wallet{
		pWallet: pWallet,
		config:  config,
		address: address,
	}, nil
}

// PWallet returns the underlying P-Chain wallet.
func (w *Wallet) PWallet() pwallet.Wallet {
	return w.pWallet
}

// Key returns the private key.
func (w *Wallet) Key() *secp256k1.PrivateKey {
	return w.key
}

// Keychain returns the keychain.
func (w *Wallet) Keychain() *secp256k1fx.Keychain {
	return w.keychain
}

// PChainAddress returns the P-Chain address.
func (w *Wallet) PChainAddress() ids.ShortID {
	if w.key != nil {
		return w.key.Address()
	}
	return w.address
}

// OwnerAddress returns the owner address for subnet operations.
func (w *Wallet) OwnerAddress() ids.ShortID {
	if w.key != nil {
		return w.key.Address()
	}
	return w.address
}

// GetPChainBalance returns the P-Chain balance in nAVAX.
func (w *Wallet) GetPChainBalance(ctx context.Context) (uint64, error) {
	balances, err := w.pWallet.Builder().GetBalance()
	if err != nil {
		return 0, fmt.Errorf("failed to get balance: %w", err)
	}
	avaxAssetID := w.pWallet.Builder().Context().AVAXAssetID
	return balances[avaxAssetID], nil
}

// Config returns the network configuration.
func (w *Wallet) Config() network.Config {
	return w.config
}

// FullWallet wraps the avalanchego primary.Wallet for multi-chain operations.
type FullWallet struct {
	key       *secp256k1.PrivateKey   // nil for Ledger
	keychain  *secp256k1fx.Keychain   // nil for Ledger
	wallet    *primary.Wallet
	config    network.Config
	address   ids.ShortID             // P-Chain address (used when key is nil)
	ethAddr   common.Address          // C-Chain address (used when key is nil)
}

// NewFullWallet creates a new wallet for multi-chain operations (P-Chain and C-Chain).
func NewFullWallet(ctx context.Context, key *secp256k1.PrivateKey, config network.Config) (*FullWallet, error) {
	kc := secp256k1fx.NewKeychain(key)

	wallet, err := primary.MakeWallet(ctx, config.RPCURL, kc, kc, primary.WalletConfig{})
	if err != nil {
		return nil, fmt.Errorf("failed to create multi-chain wallet: %w", err)
	}

	return &FullWallet{
		key:      key,
		keychain: kc,
		wallet:   wallet,
		config:   config,
	}, nil
}

// PWallet returns the P-Chain wallet.
func (w *FullWallet) PWallet() pwallet.Wallet {
	return w.wallet.P()
}

// CWallet returns the C-Chain wallet.
func (w *FullWallet) CWallet() c.Wallet {
	return w.wallet.C()
}

// Key returns the private key.
func (w *FullWallet) Key() *secp256k1.PrivateKey {
	return w.key
}

// Keychain returns the keychain.
func (w *FullWallet) Keychain() *secp256k1fx.Keychain {
	return w.keychain
}

// PChainAddress returns the P-Chain address.
func (w *FullWallet) PChainAddress() ids.ShortID {
	if w.key != nil {
		return w.key.Address()
	}
	return w.address
}

// EthAddress returns the Ethereum/C-Chain address.
func (w *FullWallet) EthAddress() common.Address {
	if w.key != nil {
		return w.key.PublicKey().EthAddress()
	}
	return w.ethAddr
}

// Config returns the network configuration.
func (w *FullWallet) Config() network.Config {
	return w.config
}

// FullKeychain combines P-Chain and C-Chain keychain interfaces.
// Used for keychains like Ledger that implement both.
type FullKeychain interface {
	keychain.Keychain
	c.EthKeychain
}

// NewFullWalletFromKeychain creates a multi-chain wallet from any keychain implementation.
func NewFullWalletFromKeychain(ctx context.Context, kc FullKeychain, address ids.ShortID, ethAddr common.Address, config network.Config) (*FullWallet, error) {
	wallet, err := primary.MakeWallet(ctx, config.RPCURL, kc, kc, primary.WalletConfig{})
	if err != nil {
		return nil, fmt.Errorf("failed to create multi-chain wallet: %w", err)
	}

	return &FullWallet{
		wallet:  wallet,
		config:  config,
		address: address,
		ethAddr: ethAddr,
	}, nil
}
