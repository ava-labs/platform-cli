package cmd

import (
	"fmt"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/platform-cli/pkg/crosschain"
	"github.com/ava-labs/platform-cli/pkg/pchain"
	"github.com/spf13/cobra"
)

var (
	transferAmount      float64
	transferAmountNAVAX uint64 // Direct nAVAX amount for precision-sensitive operations
	transferFrom        string
	transferTo          string
	transferDest        string
)

var transferCmd = &cobra.Command{
	Use:   "transfer",
	Short: "Transfer AVAX",
	Long: `Transfer AVAX on P-Chain or between P-Chain and C-Chain.

Amount Precision:
  Use --amount for human-readable AVAX amounts (e.g., --amount 10.5).
  Use --amount-navax for exact nAVAX amounts when precision matters
  (1 AVAX = 1,000,000,000 nAVAX).

  Warning: Float amounts may lose precision for values > 9007199254740992 nAVAX (~9M AVAX).
  For large transfers, use --amount-navax for guaranteed precision.`,
}

// getTransferAmountNAVAX returns the transfer amount in nAVAX.
// Prefers --amount-navax if set, otherwise converts --amount from AVAX.
func getTransferAmountNAVAX() (uint64, error) {
	if transferAmount > 0 && transferAmountNAVAX > 0 {
		return 0, fmt.Errorf("use either --amount or --amount-navax, not both")
	}
	if transferAmountNAVAX > 0 {
		return transferAmountNAVAX, nil
	}
	if transferAmount <= 0 {
		return 0, fmt.Errorf("--amount or --amount-navax is required and must be positive")
	}
	return avaxToNAVAX(transferAmount)
}

var transferSendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send AVAX on P-Chain",
	Long:  `Send AVAX to another address on the P-Chain.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

		if transferDest == "" {
			return fmt.Errorf("--to is required")
		}

		amountNAVAX, err := getTransferAmountNAVAX()
		if err != nil {
			return fmt.Errorf("invalid amount: %w", err)
		}

		destAddr, err := ids.ShortFromString(transferDest)
		if err != nil {
			return fmt.Errorf("invalid destination address: %w", err)
		}

		netConfig, err := getNetworkConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to get network config: %w", err)
		}

		w, cleanup, err := loadPChainWallet(ctx, netConfig)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}
		defer cleanup()

		fmt.Printf("Sending %d nAVAX (%.9f AVAX) to %s...\n", amountNAVAX, float64(amountNAVAX)/1e9, destAddr)

		txID, err := pchain.Send(ctx, w, destAddr, amountNAVAX)
		if err != nil {
			return fmt.Errorf("transfer failed: %w", err)
		}

		fmt.Printf("TX ID: %s\n", txID)
		return nil
	},
}

var transferPToCCmd = &cobra.Command{
	Use:   "p-to-c",
	Short: "Transfer AVAX from P-Chain to C-Chain",
	Long:  `Transfer AVAX from P-Chain to C-Chain (export + import in one step).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

		amountNAVAX, err := getTransferAmountNAVAX()
		if err != nil {
			return fmt.Errorf("invalid amount: %w", err)
		}

		netConfig, err := getNetworkConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to get network config: %w", err)
		}

		w, cleanup, err := loadFullWallet(ctx, netConfig)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}
		defer cleanup()

		fmt.Printf("Transferring %d nAVAX (%.9f AVAX) from P-Chain to C-Chain...\n", amountNAVAX, float64(amountNAVAX)/1e9)
		fmt.Printf("P-Chain Address: %s\n", w.PChainAddress())
		fmt.Printf("C-Chain Address: %s\n", w.EthAddress().Hex())
		fmt.Println("Step 1/2: Exporting from P-Chain...")

		exportTxID, importTxID, err := crosschain.TransferPToC(ctx, w, amountNAVAX)
		if err != nil {
			return fmt.Errorf("transfer failed: %w", err)
		}

		fmt.Printf("Export TX ID: %s\n", exportTxID)
		fmt.Println("Step 2/2: Importing to C-Chain...")
		fmt.Printf("Import TX ID: %s\n", importTxID)
		fmt.Println("Transfer complete!")
		return nil
	},
}

var transferCToPCmd = &cobra.Command{
	Use:   "c-to-p",
	Short: "Transfer AVAX from C-Chain to P-Chain",
	Long:  `Transfer AVAX from C-Chain to P-Chain (export + import in one step).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

		amountNAVAX, err := getTransferAmountNAVAX()
		if err != nil {
			return fmt.Errorf("invalid amount: %w", err)
		}

		netConfig, err := getNetworkConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to get network config: %w", err)
		}

		w, cleanup, err := loadFullWallet(ctx, netConfig)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}
		defer cleanup()

		fmt.Printf("Transferring %d nAVAX (%.9f AVAX) from C-Chain to P-Chain...\n", amountNAVAX, float64(amountNAVAX)/1e9)
		fmt.Printf("C-Chain Address: %s\n", w.EthAddress().Hex())
		fmt.Printf("P-Chain Address: %s\n", w.PChainAddress())
		fmt.Println("Step 1/2: Exporting from C-Chain...")

		exportTxID, importTxID, err := crosschain.TransferCToP(ctx, w, amountNAVAX)
		if err != nil {
			return fmt.Errorf("transfer failed: %w", err)
		}

		fmt.Printf("Export TX ID: %s\n", exportTxID)
		fmt.Println("Step 2/2: Importing to P-Chain...")
		fmt.Printf("Import TX ID: %s\n", importTxID)
		fmt.Println("Transfer complete!")
		return nil
	},
}

var transferExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export AVAX from one chain (step 1 of manual transfer)",
	Long:  `Export AVAX from P-Chain or C-Chain. Use this for manual two-step transfers.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

		amountNAVAX, err := getTransferAmountNAVAX()
		if err != nil {
			return fmt.Errorf("invalid amount: %w", err)
		}

		if transferFrom == "" || transferTo == "" {
			return fmt.Errorf("--from and --to are required (use 'p' or 'c')")
		}

		netConfig, err := getNetworkConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to get network config: %w", err)
		}

		w, cleanup, err := loadFullWallet(ctx, netConfig)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}
		defer cleanup()

		var txID interface{ String() string }

		switch {
		case transferFrom == "p" && transferTo == "c":
			fmt.Printf("Exporting %d nAVAX (%.9f AVAX) from P-Chain to C-Chain...\n", amountNAVAX, float64(amountNAVAX)/1e9)
			id, err := crosschain.ExportFromPChain(ctx, w, amountNAVAX)
			if err != nil {
				return fmt.Errorf("export failed: %w", err)
			}
			txID = id
		case transferFrom == "c" && transferTo == "p":
			fmt.Printf("Exporting %d nAVAX (%.9f AVAX) from C-Chain to P-Chain...\n", amountNAVAX, float64(amountNAVAX)/1e9)
			id, err := crosschain.ExportFromCChain(ctx, w, amountNAVAX)
			if err != nil {
				return fmt.Errorf("export failed: %w", err)
			}
			txID = id
		default:
			return fmt.Errorf("invalid --from/--to combination: must be p->c or c->p")
		}

		fmt.Printf("Export TX ID: %s\n", txID)
		fmt.Println("Export complete! Run 'transfer import' to complete the transfer.")
		return nil
	},
}

var transferImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import AVAX to one chain (step 2 of manual transfer)",
	Long:  `Import AVAX to P-Chain or C-Chain. Use this after 'transfer export'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

		if transferFrom == "" || transferTo == "" {
			return fmt.Errorf("--from and --to are required (use 'p' or 'c')")
		}

		netConfig, err := getNetworkConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to get network config: %w", err)
		}

		w, cleanup, err := loadFullWallet(ctx, netConfig)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}
		defer cleanup()

		var txID interface{ String() string }

		switch {
		case transferFrom == "p" && transferTo == "c":
			fmt.Println("Importing AVAX to C-Chain from P-Chain...")
			id, err := crosschain.ImportToCChain(ctx, w)
			if err != nil {
				return fmt.Errorf("import failed: %w", err)
			}
			txID = id
		case transferFrom == "c" && transferTo == "p":
			fmt.Println("Importing AVAX to P-Chain from C-Chain...")
			id, err := crosschain.ImportToPChain(ctx, w)
			if err != nil {
				return fmt.Errorf("import failed: %w", err)
			}
			txID = id
		default:
			return fmt.Errorf("invalid --from/--to combination: must be p->c or c->p")
		}

		fmt.Printf("Import TX ID: %s\n", txID)
		fmt.Println("Import complete!")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(transferCmd)
	transferCmd.AddCommand(transferSendCmd)
	transferCmd.AddCommand(transferPToCCmd)
	transferCmd.AddCommand(transferCToPCmd)
	transferCmd.AddCommand(transferExportCmd)
	transferCmd.AddCommand(transferImportCmd)

	// Flags for P-Chain send
	transferSendCmd.Flags().Float64Var(&transferAmount, "amount", 0, "Amount in AVAX to send")
	transferSendCmd.Flags().Uint64Var(&transferAmountNAVAX, "amount-navax", 0, "Amount in nAVAX (for precision-sensitive transfers)")
	transferSendCmd.Flags().StringVar(&transferDest, "to", "", "Destination P-Chain address")
	transferSendCmd.MarkFlagsMutuallyExclusive("amount", "amount-navax")

	// Flags for combined transfer commands
	transferPToCCmd.Flags().Float64Var(&transferAmount, "amount", 0, "Amount in AVAX to transfer")
	transferPToCCmd.Flags().Uint64Var(&transferAmountNAVAX, "amount-navax", 0, "Amount in nAVAX (for precision-sensitive transfers)")
	transferCToPCmd.Flags().Float64Var(&transferAmount, "amount", 0, "Amount in AVAX to transfer")
	transferCToPCmd.Flags().Uint64Var(&transferAmountNAVAX, "amount-navax", 0, "Amount in nAVAX (for precision-sensitive transfers)")
	transferPToCCmd.MarkFlagsMutuallyExclusive("amount", "amount-navax")
	transferCToPCmd.MarkFlagsMutuallyExclusive("amount", "amount-navax")

	// Flags for manual export command
	transferExportCmd.Flags().Float64Var(&transferAmount, "amount", 0, "Amount in AVAX to export")
	transferExportCmd.Flags().Uint64Var(&transferAmountNAVAX, "amount-navax", 0, "Amount in nAVAX (for precision-sensitive transfers)")
	transferExportCmd.Flags().StringVar(&transferFrom, "from", "", "Source chain: 'p' or 'c'")
	transferExportCmd.Flags().StringVar(&transferTo, "to", "", "Destination chain: 'p' or 'c'")
	transferExportCmd.MarkFlagsMutuallyExclusive("amount", "amount-navax")

	// Flags for manual import command
	transferImportCmd.Flags().StringVar(&transferFrom, "from", "", "Source chain: 'p' or 'c'")
	transferImportCmd.Flags().StringVar(&transferTo, "to", "", "Destination chain: 'p' or 'c'")
}
