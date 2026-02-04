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
	// BIP44 root path for Avalanche: m/44'/9000'/0'
	ledgerRootPath = "m/44'/9000'/0'"

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
	address   ids.ShortID
	pubKey    *secp256k1.PublicKey
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

	// Derive the address at the specified index
	path := fmt.Sprintf("%s/0/%d", ledgerRootPath, addressIndex)
	fmt.Printf("  Deriving address at path: %s\n", path)

	addrResp, err := getPublicKeyWithRetry(device, path)
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
	fmt.Printf("  Ledger address: %s\n", address)

	addresses := set.NewSet[ids.ShortID](1)
	addresses.Add(address)

	kc := &LedgerKeychain{
		device:    device,
		index:     addressIndex,
		address:   address,
		pubKey:    pubKey,
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
func (kc *LedgerKeychain) EthAddresses() set.Set[common.Address] {
	addrs := set.NewSet[common.Address](1)
	addrs.Add(kc.pubKey.EthAddress())
	return addrs
}

// GetEth returns a signer for the given Ethereum address.
func (kc *LedgerKeychain) GetEth(addr common.Address) (keychain.Signer, bool) {
	if addr != kc.pubKey.EthAddress() {
		return nil, false
	}
	return &LedgerSigner{kc: kc, addr: kc.address}, true
}

// LedgerSigner implements keychain.Signer using a Ledger device.
type LedgerSigner struct {
	kc   *LedgerKeychain
	addr ids.ShortID
}

// SignHash signs a 32-byte hash using the Ledger device.
// This is used by the SDK when signHash=true.
func (s *LedgerSigner) SignHash(hash []byte) ([]byte, error) {
	return s.kc.SignHash(hash)
}

// Sign signs a message (full transaction) using the Ledger device.
// The message is hashed before signing.
func (s *LedgerSigner) Sign(msg []byte) ([]byte, error) {
	return s.kc.Sign(msg)
}

// Address returns the address associated with this signer.
func (s *LedgerSigner) Address() ids.ShortID {
	return s.addr
}

// SignHash signs a 32-byte hash using the Ledger device (blind signing).
func (kc *LedgerKeychain) SignHash(hash []byte) ([]byte, error) {
	signerPath := fmt.Sprintf("0/%d", kc.index)

	fmt.Printf("\n  >>> Please confirm the transaction on your Ledger device <<<\n\n")

	sig, err := signHashWithRetry(kc.device, ledgerRootPath, []string{signerPath}, hash)
	if err != nil {
		return nil, fmt.Errorf("Ledger signing failed: %w", err)
	}

	return sig, nil
}

// Sign signs a full transaction message using the Ledger device.
// The Ledger will parse and display the transaction details.
func (kc *LedgerKeychain) Sign(msg []byte) ([]byte, error) {
	signerPath := fmt.Sprintf("0/%d", kc.index)

	fmt.Printf("\n  >>> Please confirm the transaction on your Ledger device <<<\n\n")

	sig, err := signWithRetry(kc.device, ledgerRootPath, []string{signerPath}, msg)
	if err != nil {
		return nil, fmt.Errorf("Ledger signing failed: %w", err)
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
