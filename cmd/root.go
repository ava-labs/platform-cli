package cmd

import (
	"fmt"
	"math"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Global flags
	networkName    string
	privateKey     string
	useLedger      bool
	ledgerIndex    uint32 // Ledger address index (BIP44)
	keyNameGlobal  string // Key name for loading from keystore
	keyPassword    string // Password for encrypted keys (env var only for security)
	customRPCURL   string // Custom RPC URL for devnets
	customNetID    uint32 // Optional network ID for custom RPC (auto-detected if not set)
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
	rootCmd.PersistentFlags().StringVarP(&networkName, "network", "n", "fuji", "Network: local, fuji, or mainnet")
	rootCmd.PersistentFlags().StringVarP(&privateKey, "private-key", "k", "", "Private key (PrivateKey-... or 0x... format)")
	rootCmd.PersistentFlags().BoolVar(&useLedger, "ledger", false, "Use Ledger hardware wallet")
	rootCmd.PersistentFlags().Uint32Var(&ledgerIndex, "ledger-index", 0, "Ledger address index (BIP44 path: m/44'/9000'/0'/0/{index})")
	rootCmd.PersistentFlags().StringVar(&keyNameGlobal, "key-name", "", "Name of key to load from keystore")
	rootCmd.PersistentFlags().StringVar(&customRPCURL, "rpc-url", "", "Custom RPC URL (overrides --network)")
	rootCmd.PersistentFlags().Uint32Var(&customNetID, "network-id", 0, "Network ID for custom RPC (1=mainnet, 5=fuji, auto-detected if not set)")
}

// avaxToNAVAX converts AVAX amount to nAVAX with validation.
// Returns error if amount is negative or would overflow.
func avaxToNAVAX(avax float64) (uint64, error) {
	if avax < 0 {
		return 0, fmt.Errorf("amount cannot be negative: %.9f", avax)
	}
	// Max safe value: ~18.4 billion AVAX (uint64 max / 1e9)
	const maxAVAX = float64(math.MaxUint64) / 1e9
	if avax > maxAVAX {
		return 0, fmt.Errorf("amount too large: %.9f AVAX (max: %.0f)", avax, maxAVAX)
	}
	return uint64(math.Round(avax * 1e9)), nil
}

// feeToPercent converts a decimal fee (0.02 = 2%) to basis points (200).
// Uses rounding to avoid float precision issues.
func feeToPercent(fee float64) (uint32, error) {
	if fee < 0 || fee > 1 {
		return 0, fmt.Errorf("delegation fee must be between 0 and 1 (got %.4f)", fee)
	}
	return uint32(math.Round(fee * 10000)), nil
}
