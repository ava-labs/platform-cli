package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/ava-labs/platform-cli/pkg/keystore"
	"github.com/ava-labs/platform-cli/pkg/network"
	"github.com/ava-labs/platform-cli/pkg/wallet"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

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
		ctx := context.Background()

		keyBytes, err := loadKey()
		if err != nil {
			return err
		}

		key, err := wallet.ToPrivateKey(keyBytes)
		if err != nil {
			return err
		}

		netConfig := network.GetConfig(networkName)
		w, err := wallet.NewWallet(ctx, key, netConfig)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}

		balance, err := w.GetPChainBalance(ctx)
		if err != nil {
			return fmt.Errorf("failed to get balance: %w", err)
		}

		fmt.Printf("P-Chain Address: %s\n", w.PChainAddress())
		fmt.Printf("Balance: %.9f AVAX\n", float64(balance)/1e9)
		return nil
	},
}

var addressCmd = &cobra.Command{
	Use:   "address",
	Short: "Show wallet addresses",
	Long:  `Display P-Chain and EVM addresses for the specified wallet.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		key, err := loadKey()
		if err != nil {
			return err
		}

		pAddr, evmAddr := wallet.DeriveAddresses(key)

		fmt.Printf("P-Chain Address: %s\n", pAddr)
		fmt.Printf("EVM Address:     %s\n", evmAddr)
		return nil
	},
}

func loadKey() ([]byte, error) {
	// Priority 1: Direct private key via flag
	if privateKey != "" {
		return wallet.ParsePrivateKey(privateKey)
	}

	// Priority 2: Key from keystore by name
	if keyNameGlobal != "" {
		return loadFromKeystore(keyNameGlobal)
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

	return nil, fmt.Errorf("no private key provided. Use --private-key, --key-name, or set AVALANCHE_PRIVATE_KEY env var")
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
func loadFromKeystore(name string) ([]byte, error) {
	// Built-in: ewoq test key
	if name == "ewoq" {
		return ewoqPrivateKey, nil
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
	}

	return ks.LoadKey(name, password)
}

func init() {
	rootCmd.AddCommand(walletCmd)
	walletCmd.AddCommand(balanceCmd)
	walletCmd.AddCommand(addressCmd)
}
