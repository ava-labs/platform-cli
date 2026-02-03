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

// loadFromKeystore loads a key from the keystore by name.
func loadFromKeystore(name string) ([]byte, error) {
	ks, err := keystore.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load keystore: %w", err)
	}

	if !ks.HasKey(name) {
		return nil, fmt.Errorf("key %q not found in keystore", name)
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
