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
	Use:   "platform",
	Short: "Avalanche P-Chain CLI",
	Long: `Avalanche P-Chain operations: staking, subnets, transfers, and L1 validators.

Example usage:
  platform wallet balance --key-name mykey
  platform validator add --node-id NodeID-... --stake 2000
  platform transfer p-to-c --amount 10 --key-name mykey
  platform subnet create --network fuji --key-name mykey`,
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
