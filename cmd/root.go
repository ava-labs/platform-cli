package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Global flags
	networkName    string
	privateKey     string
	useLedger      bool
	keyNameGlobal  string // Key name for loading from keystore
	keyPassword    string // Password for encrypted keys (env var only for security)
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "platform-cli",
	Short: "Avalanche P-Chain command-line utilities",
	Long: `platform-cli provides utilities for working with the Avalanche P-Chain.

Commands include:
  wallet  - Wallet operations (balance, address)
  subnet  - Subnet management (create)
  chain   - Chain management (create)
  node    - Node information (info)
  keys    - Key management (import, generate, list, export, delete)

Key loading priority:
  1. --private-key flag (direct key input)
  2. --key-name flag (load from keystore)
  3. Default key from keystore (if set)
  4. AVALANCHE_PRIVATE_KEY environment variable

For encrypted keys, set PLATFORM_CLI_KEY_PASSWORD environment variable.

Example usage:
  # Check wallet balance using keystore
  platform-cli wallet balance --key-name mykey

  # Check wallet balance with direct key
  platform-cli wallet balance --private-key "PrivateKey-..."

  # Get node ID and BLS key
  platform-cli node info --ip 127.0.0.1

  # Create a subnet
  platform-cli subnet create --network fuji --key-name mykey`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&networkName, "network", "n", "fuji", "Network: fuji or mainnet")
	rootCmd.PersistentFlags().StringVarP(&privateKey, "private-key", "k", "", "Private key (PrivateKey-... or 0x... format)")
	rootCmd.PersistentFlags().BoolVar(&useLedger, "ledger", false, "Use Ledger hardware wallet")
	rootCmd.PersistentFlags().StringVar(&keyNameGlobal, "key-name", "", "Name of key to load from keystore")
}
