package cmd

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

const (
	// defaultOperationTimeout is the default timeout for network operations.
	// Can be overridden via PLATFORM_CLI_TIMEOUT environment variable.
	defaultOperationTimeout = 2 * time.Minute
)

var (
	// Global flags
	networkName       string
	privateKey        string
	useLedger         bool
	allowInsecureHTTP bool   // Allow plain HTTP for non-local node endpoint discovery
	ledgerIndex       uint32 // Ledger address index (BIP44)
	keyNameGlobal     string // Key name for loading from keystore
	customRPCURL      string // Custom RPC URL for devnets
	customNetID       uint32 // Optional network ID for custom RPC (auto-detected if not set)
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:           "platform",
	Short:         "Avalanche P-Chain CLI",
	SilenceErrors: true,
	SilenceUsage:  true,
	Long: `Avalanche P-Chain operations: staking, subnets, transfers, and L1 validators.

Example usage:
  platform wallet balance --key-name mykey
  platform validator add --node-id NodeID-... --stake 2000
  platform transfer p-to-c --amount 10 --key-name mykey
  platform subnet create --network fuji --key-name mykey

Environment Variables:
  AVALANCHE_PRIVATE_KEY      Private key fallback (prefer --key-name or --ledger)
  PLATFORM_CLI_KEY_PASSWORD  Password for encrypted keys (safer than prompting in scripts)
  PLATFORM_CLI_TIMEOUT       Operation timeout duration (e.g., "5m", "30s", default: 2m)`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&networkName, "network", "n", "fuji", "Network: fuji or mainnet (use --rpc-url for local/custom)")
	rootCmd.PersistentFlags().StringVarP(&privateKey, "private-key", "k", "", "Private key (PrivateKey-... or 0x... format; discouraged, prefer --key-name)")
	rootCmd.PersistentFlags().BoolVar(&useLedger, "ledger", false, "Use Ledger hardware wallet")
	rootCmd.PersistentFlags().BoolVar(&allowInsecureHTTP, "allow-insecure-http", false, "Allow plain HTTP for non-local node/custom RPC endpoint discovery (unsafe; use only on trusted networks)")
	rootCmd.PersistentFlags().Uint32Var(&ledgerIndex, "ledger-index", 0, "Ledger address index (BIP44 path: m/44'/9000'/0'/0/{index})")
	rootCmd.PersistentFlags().StringVar(&keyNameGlobal, "key-name", "", "Name of key to load from keystore")
	rootCmd.PersistentFlags().StringVar(&customRPCURL, "rpc-url", "", "Custom RPC URL (overrides --network)")
	rootCmd.PersistentFlags().Uint32Var(&customNetID, "network-id", 0, "Network ID for custom RPC (1=mainnet, 5=fuji, auto-detected if not set)")
	_ = rootCmd.PersistentFlags().MarkDeprecated("private-key", "prefer --key-name (keystore) or --ledger to avoid exposing secrets in process arguments")
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

// feeToShares converts a decimal fee (0.02 = 2%) to shares (20,000 out of 1,000,000).
// Uses rounding to avoid float precision issues.
func feeToShares(fee float64) (uint32, error) {
	if fee < 0 || fee > 1 {
		return 0, fmt.Errorf("delegation fee must be between 0 and 1 (got %.4f)", fee)
	}
	return uint32(math.Round(fee * 1_000_000)), nil
}

// getOperationContext returns a context with timeout and signal handling.
// The context will be cancelled on SIGINT/SIGTERM or when the timeout expires.
// The returned cancel function must be called to release resources.
func getOperationContext() (context.Context, context.CancelFunc) {
	// Determine timeout from environment or use default
	timeout := defaultOperationTimeout
	if envTimeout := os.Getenv("PLATFORM_CLI_TIMEOUT"); envTimeout != "" {
		if d, err := time.ParseDuration(envTimeout); err == nil && d > 0 {
			timeout = d
		}
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	// Set up signal handling for graceful cancellation
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		select {
		case <-sigChan:
			fmt.Fprintln(os.Stderr, "\nOperation cancelled by user")
			cancel()
		case <-ctx.Done():
			// Context cancelled or timed out, clean up signal handler
		}
		signal.Stop(sigChan)
	}()

	return ctx, cancel
}
