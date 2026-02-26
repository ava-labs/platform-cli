package cmd

import (
	"context"
	"crypto/subtle"
	"fmt"
	"os"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/platform-cli/pkg/keystore"
	"github.com/ava-labs/platform-cli/pkg/network"
	"github.com/ava-labs/platform-cli/pkg/wallet"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// clearBytesWallet securely zeros a byte slice to prevent sensitive data from lingering in memory.
func clearBytesWallet(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

var walletCmd = &cobra.Command{
	Use:   "wallet",
	Short: "Wallet operations",
	Long:  `Wallet operations including balance check and address display.`,
}

var balanceCmd = &cobra.Command{
	Use:   "balance",
	Short: "Show P-Chain balance",
	Long:  `Display the P-Chain balance for the specified wallet.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

		netConfig, err := getNetworkConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to get network config: %w", err)
		}

		w, cleanup, err := loadPChainWallet(ctx, netConfig)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}
		defer cleanup()

		balance, err := w.GetPChainBalance(ctx)
		if err != nil {
			return fmt.Errorf("failed to get balance: %w", err)
		}

		fmt.Printf("P-Chain Address: %s\n", w.FormattedPChainAddress())
		fmt.Printf("Balance: %.9f AVAX\n", float64(balance)/1e9)
		return nil
	},
}

var addressCmd = &cobra.Command{
	Use:   "address",
	Short: "Show wallet addresses",
	Long:  `Display P-Chain and EVM addresses for the specified wallet.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

		netConfig, err := getNetworkConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to get network config: %w", err)
		}

		if useLedger {
			if !wallet.LedgerEnabled {
				return fmt.Errorf("ledger support not compiled. Rebuild with: go build -tags ledger")
			}
			kc, err := wallet.NewLedgerKeychain(ledgerIndex)
			if err != nil {
				return err
			}
			defer kc.Close()

			fmt.Printf("P-Chain Address: %s\n", wallet.FormatPChainAddress(kc.GetAddress(), netConfig.NetworkID))
			fmt.Printf("EVM Address:     %s\n", kc.GetEVMPublicKey().EthAddress().Hex())
			return nil
		}

		key, err := loadKey()
		if err != nil {
			return err
		}
		defer clearBytesWallet(key)

		pAddr, evmAddr := wallet.DeriveAddressesFormatted(key, netConfig.NetworkID)

		fmt.Printf("P-Chain Address: %s\n", pAddr)
		fmt.Printf("EVM Address:     %s\n", evmAddr)
		return nil
	},
}

func loadKey() ([]byte, error) {
	// Priority 1: Key from keystore by name
	if keyNameGlobal != "" {
		if privateKey != "" {
			return nil, fmt.Errorf("use either --key-name or --private-key, not both")
		}
		return loadFromKeystore(keyNameGlobal)
	}

	// Priority 2: Direct private key via flag (discouraged; prefer keystore/Ledger)
	if privateKey != "" {
		return wallet.ParsePrivateKey(privateKey)
	}

	// Priority 3: Default key from keystore
	ks, err := keystore.Load()
	if err == nil && ks.GetDefault() != "" {
		return loadFromKeystore(ks.GetDefault())
	}

	// Priority 4: Environment variable
	if envKey := os.Getenv("AVALANCHE_PRIVATE_KEY"); envKey != "" {
		return wallet.ParsePrivateKey(envKey)
	}

	return nil, fmt.Errorf("no key source provided. Use --key-name (preferred), --private-key, or set AVALANCHE_PRIVATE_KEY env var")
}

// ewoqPrivateKey is the well-known ewoq test key used in local/test networks.
// P-Chain: 6Y3kysjF9jnHnYkdS9yGAuoHyae2eNmeV
// EVM: 0x8db97C7cEcE249c2b98bDC0226Cc4C2A57BF52FC
var ewoqPrivateKey = []byte{
	0x56, 0x28, 0x9e, 0x99, 0xc9, 0x4b, 0x69, 0x12,
	0xbf, 0xc1, 0x2a, 0xdc, 0x09, 0x3c, 0x9b, 0x51,
	0x12, 0x4f, 0x0d, 0xc5, 0x4a, 0xc7, 0xa7, 0x66,
	0xb2, 0xbc, 0x5c, 0xcf, 0x55, 0x8d, 0x80, 0x27,
}

// loadFromKeystore loads a key from the keystore by name.
// Special built-in key: "ewoq" returns the well-known test key.
// Note: The returned key bytes should be cleared by the caller when no longer needed.
func loadFromKeystore(name string) ([]byte, error) {
	// Built-in: ewoq test key
	if name == "ewoq" {
		// SECURITY: Prevent accidental use of ewoq key on mainnet
		if networkName == "mainnet" || customNetID == constants.MainnetID {
			return nil, fmt.Errorf("ewoq test key cannot be used on mainnet - this is a well-known key with no security")
		}
		// Return a copy so caller can safely clear it
		keyCopy := make([]byte, len(ewoqPrivateKey))
		copy(keyCopy, ewoqPrivateKey)
		return keyCopy, nil
	}

	ks, err := keystore.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load keystore: %w", err)
	}

	if !ks.HasKey(name) {
		return nil, fmt.Errorf("key %q not found in keystore (built-in: ewoq)", name)
	}

	// Get password if key is encrypted
	var password []byte
	if ks.IsEncrypted(name) {
		// Try environment variable first
		if envPwd := os.Getenv("PLATFORM_CLI_KEY_PASSWORD"); envPwd != "" {
			password = []byte(envPwd)
		} else {
			// Prompt for password
			fmt.Fprintf(os.Stderr, "Key %q is encrypted. Enter password: ", name)
			password, err = term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(os.Stderr)
			if err != nil {
				return nil, fmt.Errorf("failed to read password: %w", err)
			}
		}
		// Clear password after use
		defer clearBytesWallet(password)
	}

	return ks.LoadKey(name, password)
}

// getNetworkConfig returns the network configuration, handling custom RPC URLs.
// If customRPCURL is set, it creates a custom config (querying network ID if needed).
// Otherwise, it uses the standard named network config.
func getNetworkConfig(ctx context.Context) (network.Config, error) {
	if customRPCURL != "" {
		config, err := network.NewCustomConfigWithInsecureHTTP(ctx, customRPCURL, customNetID, allowInsecureHTTP)
		if err != nil {
			return network.Config{}, err
		}
		hrp := constants.GetHRP(config.NetworkID)
		fmt.Printf("Using custom RPC: %s (network ID: %d, HRP: %s)\n", config.RPCURL, config.NetworkID, hrp)
		return config, nil
	}
	return network.GetConfig(networkName)
}

// loadPChainWallet creates a P-Chain wallet from either Ledger or private key.
// Returns the wallet and a cleanup function that must be called when done.
func loadPChainWallet(ctx context.Context, netConfig network.Config) (*wallet.Wallet, func(), error) {
	if useLedger {
		if !wallet.LedgerEnabled {
			return nil, nil, fmt.Errorf("ledger support not compiled. Rebuild with: go build -tags ledger")
		}
		kc, err := wallet.NewLedgerKeychain(ledgerIndex)
		if err != nil {
			return nil, nil, err
		}
		w, err := wallet.NewWalletFromKeychain(ctx, kc, kc.GetAddress(), netConfig)
		if err != nil {
			kc.Close()
			return nil, nil, err
		}
		return w, kc.Close, nil
	}

	keyBytes, err := loadKey()
	if err != nil {
		return nil, nil, err
	}
	// Clear key bytes after wallet creation
	defer clearBytesWallet(keyBytes)
	if netConfig.NetworkID == constants.MainnetID && isEwoqKey(keyBytes) {
		return nil, nil, fmt.Errorf("ewoq test key cannot be used on mainnet - this is a well-known key with no security")
	}

	key, err := wallet.ToPrivateKey(keyBytes)
	if err != nil {
		return nil, nil, err
	}
	w, err := wallet.NewWallet(ctx, key, netConfig)
	if err != nil {
		return nil, nil, err
	}
	return w, func() {}, nil
}

// loadPChainWalletWithSubnet creates a P-Chain wallet that tracks a subnet.
func loadPChainWalletWithSubnet(ctx context.Context, netConfig network.Config, subnetID ids.ID) (*wallet.Wallet, func(), error) {
	if useLedger {
		if !wallet.LedgerEnabled {
			return nil, nil, fmt.Errorf("ledger support not compiled. Rebuild with: go build -tags ledger")
		}
		kc, err := wallet.NewLedgerKeychain(ledgerIndex)
		if err != nil {
			return nil, nil, err
		}
		w, err := wallet.NewWalletFromKeychainWithSubnet(ctx, kc, kc.GetAddress(), netConfig, subnetID)
		if err != nil {
			kc.Close()
			return nil, nil, err
		}
		return w, kc.Close, nil
	}

	keyBytes, err := loadKey()
	if err != nil {
		return nil, nil, err
	}
	// Clear key bytes after wallet creation
	defer clearBytesWallet(keyBytes)
	if netConfig.NetworkID == constants.MainnetID && isEwoqKey(keyBytes) {
		return nil, nil, fmt.Errorf("ewoq test key cannot be used on mainnet - this is a well-known key with no security")
	}

	key, err := wallet.ToPrivateKey(keyBytes)
	if err != nil {
		return nil, nil, err
	}
	w, err := wallet.NewWalletWithSubnet(ctx, key, netConfig, subnetID)
	if err != nil {
		return nil, nil, err
	}
	return w, func() {}, nil
}

// loadFullWallet creates a multi-chain wallet (P-Chain + C-Chain).
func loadFullWallet(ctx context.Context, netConfig network.Config) (*wallet.FullWallet, func(), error) {
	if useLedger {
		if !wallet.LedgerEnabled {
			return nil, nil, fmt.Errorf("ledger support not compiled. Rebuild with: go build -tags ledger")
		}
		kc, err := wallet.NewLedgerKeychain(ledgerIndex)
		if err != nil {
			return nil, nil, err
		}
		ethAddr := kc.GetEVMPublicKey().EthAddress()
		w, err := wallet.NewFullWalletFromKeychain(ctx, kc, kc.GetAddress(), ethAddr, netConfig)
		if err != nil {
			kc.Close()
			return nil, nil, err
		}
		return w, kc.Close, nil
	}

	keyBytes, err := loadKey()
	if err != nil {
		return nil, nil, err
	}
	// Clear key bytes after wallet creation
	defer clearBytesWallet(keyBytes)
	if netConfig.NetworkID == constants.MainnetID && isEwoqKey(keyBytes) {
		return nil, nil, fmt.Errorf("ewoq test key cannot be used on mainnet - this is a well-known key with no security")
	}

	key, err := wallet.ToPrivateKey(keyBytes)
	if err != nil {
		return nil, nil, err
	}
	w, err := wallet.NewFullWallet(ctx, key, netConfig)
	if err != nil {
		return nil, nil, err
	}
	return w, func() {}, nil
}

func isEwoqKey(keyBytes []byte) bool {
	if len(keyBytes) != len(ewoqPrivateKey) {
		return false
	}
	return subtle.ConstantTimeCompare(keyBytes, ewoqPrivateKey) == 1
}

func init() {
	rootCmd.AddCommand(walletCmd)
	walletCmd.AddCommand(balanceCmd)
	walletCmd.AddCommand(addressCmd)
}
