//go:build ledger

package wallet

import (
	"fmt"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto/keychain"
	"github.com/ava-labs/avalanchego/utils/crypto/secp256k1"
	"github.com/ava-labs/avalanchego/utils/set"
	ledger "github.com/ava-labs/ledger-avalanche-go"
	"github.com/ava-labs/libevm/common"
)

const (
	// BIP44 root path for Avalanche X/P-Chain: m/44'/9000'/0'
	ledgerRootPath = "m/44'/9000'/0'"

	// BIP44 root path for EVM/C-Chain: m/44'/60'/0'
	// Core wallet uses this path for C-Chain addresses.
	ledgerEVMRootPath = "m/44'/60'/0'"

	// Maximum retries for Ledger operations
	ledgerMaxRetries = 5

	// Initial retry delay
	ledgerRetryDelay = 200 * time.Millisecond
)

// LedgerEnabled indicates whether Ledger support is compiled in.
const LedgerEnabled = true

// LedgerKeychain wraps a Ledger device to implement keychain functionality.
type LedgerKeychain struct {
	device    *ledger.LedgerAvalanche
	index     uint32
	address   ids.ShortID              // P-Chain address (from 9000 path)
	pubKey    *secp256k1.PublicKey      // Public key from m/44'/9000'/0'/0/{index}
	evmPubKey *secp256k1.PublicKey      // Public key from m/44'/60'/0'/0/{index}
	addresses set.Set[ids.ShortID]
}

// NewLedgerKeychain creates a new keychain backed by a Ledger device.
func NewLedgerKeychain(addressIndex uint32) (*LedgerKeychain, error) {
	fmt.Println("  Connecting to Ledger device...")

	device, err := findLedgerWithRetry()
	if err != nil {
		return nil, fmt.Errorf("failed to find Ledger Avalanche app: %w\n\nMake sure:\n  1. Ledger is connected and unlocked\n  2. Avalanche app is open on the device\n  3. Ledger Live is NOT running", err)
	}

	fmt.Println("  Ledger connected successfully")

	// Derive the P-Chain/X-Chain address at the specified index (coin type 9000)
	avaxPath := fmt.Sprintf("%s/0/%d", ledgerRootPath, addressIndex)
	fmt.Printf("  Deriving P-Chain address at path: %s\n", avaxPath)

	addrResp, err := getPublicKeyWithRetry(device, avaxPath)
	if err != nil {
		device.Close()
		return nil, fmt.Errorf("failed to get public key from Ledger: %w", err)
	}

	pubKey, err := secp256k1.ToPublicKey(addrResp.PublicKey)
	if err != nil {
		device.Close()
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	address := pubKey.Address()
	fmt.Printf("  P-Chain address: %s\n", address)

	// Derive the C-Chain/EVM address at the specified index (coin type 60)
	evmPath := fmt.Sprintf("%s/0/%d", ledgerEVMRootPath, addressIndex)
	fmt.Printf("  Deriving C-Chain address at path: %s\n", evmPath)

	evmAddrResp, err := getPublicKeyWithRetry(device, evmPath)
	if err != nil {
		device.Close()
		return nil, fmt.Errorf("failed to get EVM public key from Ledger: %w", err)
	}

	evmPubKey, err := secp256k1.ToPublicKey(evmAddrResp.PublicKey)
	if err != nil {
		device.Close()
		return nil, fmt.Errorf("failed to parse EVM public key: %w", err)
	}

	fmt.Printf("  C-Chain address: %s\n", evmPubKey.EthAddress().Hex())

	addresses := set.NewSet[ids.ShortID](1)
	addresses.Add(address)

	kc := &LedgerKeychain{
		device:    device,
		index:     addressIndex,
		address:   address,
		pubKey:    pubKey,
		evmPubKey: evmPubKey,
		addresses: addresses,
	}

	return kc, nil
}

// Close closes the Ledger device connection.
func (kc *LedgerKeychain) Close() {
	if kc.device != nil {
		kc.device.Close()
	}
}

// Addresses returns the set of addresses managed by this keychain.
func (kc *LedgerKeychain) Addresses() set.Set[ids.ShortID] {
	return kc.addresses
}

// GetAddress returns the primary address of the keychain.
func (kc *LedgerKeychain) GetAddress() ids.ShortID {
	return kc.address
}

// GetPublicKey returns the public key.
func (kc *LedgerKeychain) GetPublicKey() *secp256k1.PublicKey {
	return kc.pubKey
}

// Get returns a signer for the given address.
// Implements keychain.Keychain interface.
func (kc *LedgerKeychain) Get(addr ids.ShortID) (keychain.Signer, bool) {
	if !kc.addresses.Contains(addr) {
		return nil, false
	}
	return &LedgerSigner{kc: kc, addr: addr}, true
}

// EthAddresses returns the set of Ethereum addresses managed by this keychain.
// Uses the m/44'/60'/0' derivation path to match Core wallet.
func (kc *LedgerKeychain) EthAddresses() set.Set[common.Address] {
	addrs := set.NewSet[common.Address](1)
	addrs.Add(kc.evmPubKey.EthAddress())
	return addrs
}

// GetEth returns a signer for the given Ethereum address.
// The signer uses the m/44'/60'/0' path for signing.
func (kc *LedgerKeychain) GetEth(addr common.Address) (keychain.Signer, bool) {
	if addr != kc.evmPubKey.EthAddress() {
		return nil, false
	}
	return &LedgerEVMSigner{kc: kc, addr: kc.address}, true
}

// GetEVMPublicKey returns the EVM public key (from m/44'/60' path).
func (kc *LedgerKeychain) GetEVMPublicKey() *secp256k1.PublicKey {
	return kc.evmPubKey
}

// LedgerSigner implements keychain.Signer using a Ledger device.
// Signs using the Avalanche path (m/44'/9000'/0') for P-Chain operations.
type LedgerSigner struct {
	kc   *LedgerKeychain
	addr ids.ShortID
}

// SignHash signs a 32-byte hash using the Ledger device.
func (s *LedgerSigner) SignHash(hash []byte) ([]byte, error) {
	return s.kc.SignHash(hash)
}

// Sign signs a message (full transaction) using the Ledger device.
func (s *LedgerSigner) Sign(msg []byte) ([]byte, error) {
	return s.kc.Sign(msg)
}

// Address returns the address associated with this signer.
func (s *LedgerSigner) Address() ids.ShortID {
	return s.addr
}

// LedgerEVMSigner implements keychain.Signer using a Ledger device.
// Signs using the EVM path (m/44'/60'/0') for C-Chain operations.
type LedgerEVMSigner struct {
	kc   *LedgerKeychain
	addr ids.ShortID
}

// SignHash signs a 32-byte hash using the Ledger device with the EVM path.
func (s *LedgerEVMSigner) SignHash(hash []byte) ([]byte, error) {
	return s.kc.SignHashEVM(hash)
}

// Sign signs a message using the Ledger device with the EVM path.
func (s *LedgerEVMSigner) Sign(msg []byte) ([]byte, error) {
	return s.kc.SignEVM(msg)
}

// Address returns the address associated with this signer.
func (s *LedgerEVMSigner) Address() ids.ShortID {
	return s.addr
}

// SignHash signs a 32-byte hash using the Ledger device (Avalanche path).
func (kc *LedgerKeychain) SignHash(hash []byte) ([]byte, error) {
	signerPath := fmt.Sprintf("0/%d", kc.index)

	fmt.Printf("\n  >>> Please confirm the transaction on your Ledger device <<<\n\n")

	sig, err := signHashWithRetry(kc.device, ledgerRootPath, []string{signerPath}, hash)
	if err != nil {
		return nil, fmt.Errorf("Ledger signing failed: %w", err)
	}

	return sig, nil
}

// Sign signs a full transaction message using the Ledger device (Avalanche path).
func (kc *LedgerKeychain) Sign(msg []byte) ([]byte, error) {
	signerPath := fmt.Sprintf("0/%d", kc.index)

	fmt.Printf("\n  >>> Please confirm the transaction on your Ledger device <<<\n\n")

	sig, err := signWithRetry(kc.device, ledgerRootPath, []string{signerPath}, msg)
	if err != nil {
		return nil, fmt.Errorf("Ledger signing failed: %w", err)
	}

	return sig, nil
}

// SignHashEVM signs a 32-byte hash using the Ledger device (EVM path).
func (kc *LedgerKeychain) SignHashEVM(hash []byte) ([]byte, error) {
	signerPath := fmt.Sprintf("0/%d", kc.index)

	fmt.Printf("\n  >>> Please confirm the transaction on your Ledger device <<<\n\n")

	sig, err := signHashWithRetry(kc.device, ledgerEVMRootPath, []string{signerPath}, hash)
	if err != nil {
		return nil, fmt.Errorf("Ledger EVM signing failed: %w", err)
	}

	return sig, nil
}

// SignEVM signs a full transaction message using the Ledger device (EVM path).
func (kc *LedgerKeychain) SignEVM(msg []byte) ([]byte, error) {
	signerPath := fmt.Sprintf("0/%d", kc.index)

	fmt.Printf("\n  >>> Please confirm the transaction on your Ledger device <<<\n\n")

	sig, err := signWithRetry(kc.device, ledgerEVMRootPath, []string{signerPath}, msg)
	if err != nil {
		return nil, fmt.Errorf("Ledger EVM signing failed: %w", err)
	}

	return sig, nil
}

func findLedgerWithRetry() (*ledger.LedgerAvalanche, error) {
	var device *ledger.LedgerAvalanche
	var err error

	delay := ledgerRetryDelay
	for i := 0; i < ledgerMaxRetries; i++ {
		device, err = ledger.FindLedgerAvalancheApp()
		if err == nil {
			return device, nil
		}

		if i < ledgerMaxRetries-1 {
			time.Sleep(delay)
			delay *= 2
		}
	}

	return nil, err
}

func getPublicKeyWithRetry(device *ledger.LedgerAvalanche, path string) (*ledger.ResponseAddr, error) {
	var resp *ledger.ResponseAddr
	var err error

	delay := ledgerRetryDelay
	for i := 0; i < ledgerMaxRetries; i++ {
		// Use empty strings for hrp and chainID to get raw public key
		// Address derivation is done on our side using secp256k1.PublicKey.Address()
		resp, err = device.GetPubKey(path, false, "", "")
		if err == nil {
			return resp, nil
		}

		if i < ledgerMaxRetries-1 {
			time.Sleep(delay)
			delay *= 2
		}
	}

	return nil, err
}

func signHashWithRetry(device *ledger.LedgerAvalanche, rootPath string, signerPaths []string, hash []byte) ([]byte, error) {
	var err error

	delay := ledgerRetryDelay
	for i := 0; i < ledgerMaxRetries; i++ {
		response, signErr := device.SignHash(rootPath, signerPaths, hash)
		if signErr == nil && response != nil {
			// Return the first signature found
			for _, sig := range response.Signature {
				return sig, nil
			}
		}
		err = signErr

		if i < ledgerMaxRetries-1 {
			time.Sleep(delay)
			delay *= 2
		}
	}

	return nil, err
}

func signWithRetry(device *ledger.LedgerAvalanche, rootPath string, signerPaths []string, msg []byte) ([]byte, error) {
	var err error

	delay := ledgerRetryDelay
	for i := 0; i < ledgerMaxRetries; i++ {
		response, signErr := device.Sign(rootPath, signerPaths, msg, nil)
		if signErr == nil && response != nil {
			// Return the first signature found
			for _, sig := range response.Signature {
				return sig, nil
			}
		}
		err = signErr

		if i < ledgerMaxRetries-1 {
			time.Sleep(delay)
			delay *= 2
		}
	}

	return nil, err
}
