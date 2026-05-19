package cmd

import (
	"fmt"

	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/platform-cli/pkg/multisig"
	"github.com/spf13/cobra"
)

var (
	txFilePath string
)

var txCmd = &cobra.Command{
	Use:   "tx",
	Short: "Transaction operations",
	Long: `Manage partially-signed transactions for multisig workflows.

Typical multisig workflow:
  1. Create a transaction with --output-tx to write it to a file instead of submitting
     platform subnet create --owners addr1,addr2,addr3 --threshold 2 --output-tx unsigned.json

  2. First signer signs the transaction
     platform tx sign --tx-file unsigned.json --key-name signer1

  3. Second signer signs the same file
     platform tx sign --tx-file unsigned.json --key-name signer2

  4. Once enough signatures are collected, submit the transaction
     platform tx commit --tx-file unsigned.json --network fuji`,
}

var txSignCmd = &cobra.Command{
	Use:   "sign",
	Short: "Add a signature to a transaction file",
	Long: `Sign a partially-signed transaction file with the specified key.

The transaction file is updated in-place with the new signature(s).
Multiple signers can each run this command on the same file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if txFilePath == "" {
			return fmt.Errorf("--tx-file is required")
		}

		ctx, cancel := getOperationContext()
		defer cancel()

		// Read the tx file
		tf, err := multisig.ReadTxFile(txFilePath)
		if err != nil {
			return fmt.Errorf("failed to read tx file: %w", err)
		}

		// Parse the transaction
		tx, err := multisig.ParseTx(tf)
		if err != nil {
			return fmt.Errorf("failed to parse transaction: %w", err)
		}

		// Check if already fully signed
		if multisig.IsFullySigned(tx) {
			fmt.Println("Transaction is already fully signed.")
			fmt.Println("Use 'platform tx commit' to submit it.")
			return nil
		}

		// Load the network config to get the wallet backend
		netConfig, err := getNetworkConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to get network config: %w", err)
		}

		// Load wallet (provides keychain and signer backend)
		w, cleanup, err := loadPChainWallet(ctx, netConfig)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}
		defer cleanup()

		// Get signature counts before signing
		filledBefore, totalBefore := multisig.CredentialSignatureStatus(tx)

		// Sign the transaction using the wallet's signer
		// The avalanchego signer natively supports partial signing —
		// it fills in sigs for keys it has and leaves empty sigs for keys it doesn't.
		s := w.PWallet().Signer()
		if err := s.Sign(ctx, tx); err != nil {
			return fmt.Errorf("failed to sign transaction: %w", err)
		}

		// Get signature counts after signing
		filledAfter, _ := multisig.CredentialSignatureStatus(tx)

		if filledAfter == filledBefore {
			fmt.Printf("Warning: no new signatures were added (key may not be a required signer)\n")
			fmt.Printf("Signature status: %d/%d filled\n", filledBefore, totalBefore)
			return nil
		}

		// Update the tx file with new signatures
		if err := multisig.UpdateTxFileAfterSign(tf, tx); err != nil {
			return fmt.Errorf("failed to update tx file: %w", err)
		}

		// Write back
		if err := multisig.WriteTxFile(txFilePath, tf); err != nil {
			return fmt.Errorf("failed to write tx file: %w", err)
		}

		fmt.Printf("Added %d new signature(s) to %s\n", filledAfter-filledBefore, txFilePath)
		fmt.Printf("Signature status: %d/%d filled\n", filledAfter, totalBefore)

		if multisig.IsFullySigned(tx) {
			fmt.Println("Transaction is now fully signed!")
			fmt.Println("Use 'platform tx commit' to submit it.")
		} else {
			fmt.Println("Transaction still needs more signatures.")
			if tf.Owners != nil {
				fmt.Print(multisig.FormatSignerStatus(tf))
			}
		}

		return nil
	},
}

var txCommitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Submit a fully-signed transaction",
	Long: `Submit a fully-signed transaction from a tx file to the network.

The transaction must have all required signatures before it can be committed.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if txFilePath == "" {
			return fmt.Errorf("--tx-file is required")
		}

		ctx, cancel := getOperationContext()
		defer cancel()

		// Read the tx file
		tf, err := multisig.ReadTxFile(txFilePath)
		if err != nil {
			return fmt.Errorf("failed to read tx file: %w", err)
		}

		// Parse the transaction
		tx, err := multisig.ParseTx(tf)
		if err != nil {
			return fmt.Errorf("failed to parse transaction: %w", err)
		}

		// Check if fully signed
		if !multisig.IsFullySigned(tx) {
			filled, total := multisig.CredentialSignatureStatus(tx)
			fmt.Printf("Transaction is not fully signed (%d/%d signatures)\n", filled, total)
			if tf.Owners != nil {
				fmt.Print(multisig.FormatSignerStatus(tf))
			}
			return fmt.Errorf("all required signatures must be present before committing")
		}

		// Load network config
		netConfig, err := getNetworkConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to get network config: %w", err)
		}

		// Verify network ID matches
		if tf.NetworkID != 0 && tf.NetworkID != netConfig.NetworkID {
			return fmt.Errorf("tx file network ID (%d) does not match current network (%d)", tf.NetworkID, netConfig.NetworkID)
		}

		// We need a wallet just to submit the tx (any key works since tx is already signed)
		w, cleanup, err := loadPChainWallet(ctx, netConfig)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}
		defer cleanup()

		fmt.Println("Submitting transaction...")
		if err := w.PWallet().IssueTx(tx); err != nil {
			return fmt.Errorf("failed to submit transaction: %w", err)
		}

		fmt.Println("Transaction submitted successfully!")
		fmt.Printf("TX ID: %s\n", tx.ID())
		return nil
	},
}

var txInspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "Inspect a transaction file",
	Long:  `Display details about a transaction file including signature status.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if txFilePath == "" {
			return fmt.Errorf("--tx-file is required")
		}

		// Read the tx file
		tf, err := multisig.ReadTxFile(txFilePath)
		if err != nil {
			return fmt.Errorf("failed to read tx file: %w", err)
		}

		// Parse the transaction
		tx, err := multisig.ParseTx(tf)
		if err != nil {
			return fmt.Errorf("failed to parse transaction: %w", err)
		}

		// Display info
		fmt.Printf("File: %s\n", txFilePath)
		fmt.Printf("Version: %d\n", tf.Version)
		fmt.Printf("Network ID: %d\n", tf.NetworkID)
		fmt.Printf("TX ID: %s\n", tx.ID())

		// Transaction type
		fmt.Printf("TX Type: %s\n", txTypeName(tx))

		// Signature status
		filled, total := multisig.CredentialSignatureStatus(tx)
		fmt.Printf("Signatures: %d/%d filled\n", filled, total)
		fmt.Printf("Fully signed: %v\n", multisig.IsFullySigned(tx))

		// Owner info
		if tf.Owners != nil {
			fmt.Println()
			fmt.Print(multisig.FormatSignerStatus(tf))
		}

		return nil
	},
}

// txTypeName returns a human-readable name for a transaction type.
func txTypeName(tx *txs.Tx) string {
	switch tx.Unsigned.(type) {
	case *txs.CreateSubnetTx:
		return "CreateSubnet"
	case *txs.TransferSubnetOwnershipTx:
		return "TransferSubnetOwnership"
	case *txs.AddValidatorTx:
		return "AddValidator"
	case *txs.AddDelegatorTx:
		return "AddDelegator"
	case *txs.AddPermissionlessValidatorTx:
		return "AddPermissionlessValidator"
	case *txs.AddPermissionlessDelegatorTx:
		return "AddPermissionlessDelegator"
	case *txs.CreateChainTx:
		return "CreateChain"
	case *txs.ConvertSubnetToL1Tx:
		return "ConvertSubnetToL1"
	case *txs.BaseTx:
		return "BaseTx"
	case *txs.ExportTx:
		return "ExportTx"
	case *txs.ImportTx:
		return "ImportTx"
	case *txs.RegisterL1ValidatorTx:
		return "RegisterL1Validator"
	case *txs.SetL1ValidatorWeightTx:
		return "SetL1ValidatorWeight"
	case *txs.IncreaseL1ValidatorBalanceTx:
		return "IncreaseL1ValidatorBalance"
	case *txs.DisableL1ValidatorTx:
		return "DisableL1Validator"
	default:
		return "Unknown"
	}
}

func init() {
	rootCmd.AddCommand(txCmd)

	txCmd.AddCommand(txSignCmd)
	txCmd.AddCommand(txCommitCmd)
	txCmd.AddCommand(txInspectCmd)

	// Shared flags
	txSignCmd.Flags().StringVar(&txFilePath, "tx-file", "", "Path to transaction file")
	txCommitCmd.Flags().StringVar(&txFilePath, "tx-file", "", "Path to transaction file")
	txInspectCmd.Flags().StringVar(&txFilePath, "tx-file", "", "Path to transaction file")
}
