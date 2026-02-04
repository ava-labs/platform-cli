package cmd

import (
	"context"
	"fmt"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/platform-cli/pkg/crosschain"
	"github.com/ava-labs/platform-cli/pkg/pchain"
	"github.com/spf13/cobra"
)

var (
	transferAmount float64
	transferFrom   string
	transferTo     string
	transferDest   string
)

var transferCmd = &cobra.Command{
	Use:   "transfer",
	Short: "Transfer AVAX",
	Long:  `Transfer AVAX on P-Chain or between P-Chain and C-Chain.`,
}

var transferSendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send AVAX on P-Chain",
	Long:  `Send AVAX to another address on the P-Chain.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		if transferAmount <= 0 {
			return fmt.Errorf("--amount is required and must be positive")
		}
		if transferDest == "" {
			return fmt.Errorf("--to is required")
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

		amountNAVAX, err := avaxToNAVAX(transferAmount)
		if err != nil {
			return fmt.Errorf("invalid amount: %w", err)
		}

		fmt.Printf("Sending %.9f AVAX to %s...\n", transferAmount, destAddr)

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
		ctx := context.Background()

		if transferAmount <= 0 {
			return fmt.Errorf("--amount is required and must be positive")
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

		amountNAVAX, err := avaxToNAVAX(transferAmount)
		if err != nil {
			return fmt.Errorf("invalid amount: %w", err)
		}

		fmt.Printf("Transferring %.9f AVAX from P-Chain to C-Chain...\n", transferAmount)
		fmt.Printf("P-Chain Address: %s\n", w.PChainAddress())
		fmt.Printf("C-Chain Address: %s\n", w.EthAddress().Hex())

		exportTxID, importTxID, err := crosschain.TransferPToC(ctx, w, amountNAVAX)
		if err != nil {
			return fmt.Errorf("transfer failed: %w", err)
		}

		fmt.Printf("Export TX ID: %s\n", exportTxID)
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
		ctx := context.Background()

		if transferAmount <= 0 {
			return fmt.Errorf("--amount is required and must be positive")
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

		amountNAVAX, err := avaxToNAVAX(transferAmount)
		if err != nil {
			return fmt.Errorf("invalid amount: %w", err)
		}

		fmt.Printf("Transferring %.9f AVAX from C-Chain to P-Chain...\n", transferAmount)
		fmt.Printf("C-Chain Address: %s\n", w.EthAddress().Hex())
		fmt.Printf("P-Chain Address: %s\n", w.PChainAddress())

		exportTxID, importTxID, err := crosschain.TransferCToP(ctx, w, amountNAVAX)
		if err != nil {
			return fmt.Errorf("transfer failed: %w", err)
		}

		fmt.Printf("Export TX ID: %s\n", exportTxID)
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
		ctx := context.Background()

		if transferAmount <= 0 {
			return fmt.Errorf("--amount is required and must be positive")
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

		amountNAVAX, err := avaxToNAVAX(transferAmount)
		if err != nil {
			return fmt.Errorf("invalid amount: %w", err)
		}

		var txID interface{ String() string }

		switch {
		case transferFrom == "p" && transferTo == "c":
			fmt.Printf("Exporting %.9f AVAX from P-Chain to C-Chain...\n", transferAmount)
			id, err := crosschain.ExportFromPChain(ctx, w, amountNAVAX)
			if err != nil {
				return fmt.Errorf("export failed: %w", err)
			}
			txID = id
		case transferFrom == "c" && transferTo == "p":
			fmt.Printf("Exporting %.9f AVAX from C-Chain to P-Chain...\n", transferAmount)
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
		ctx := context.Background()

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
	transferSendCmd.Flags().StringVar(&transferDest, "to", "", "Destination P-Chain address")

	// Flags for combined transfer commands
	transferPToCCmd.Flags().Float64Var(&transferAmount, "amount", 0, "Amount in AVAX to transfer")
	transferCToPCmd.Flags().Float64Var(&transferAmount, "amount", 0, "Amount in AVAX to transfer")

	// Flags for manual export command
	transferExportCmd.Flags().Float64Var(&transferAmount, "amount", 0, "Amount in AVAX to export")
	transferExportCmd.Flags().StringVar(&transferFrom, "from", "", "Source chain: 'p' or 'c'")
	transferExportCmd.Flags().StringVar(&transferTo, "to", "", "Destination chain: 'p' or 'c'")

	// Flags for manual import command
	transferImportCmd.Flags().StringVar(&transferFrom, "from", "", "Source chain: 'p' or 'c'")
	transferImportCmd.Flags().StringVar(&transferTo, "to", "", "Destination chain: 'p' or 'c'")
}
