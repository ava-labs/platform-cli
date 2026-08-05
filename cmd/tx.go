package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/vms/platformvm"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/platform-cli/pkg/network"
	"github.com/ava-labs/platform-cli/pkg/pchain"
	"github.com/ava-labs/platform-cli/pkg/wallet"
	"github.com/spf13/cobra"
)

var txPath string

// txAcceptancePollFreq is how often tx commit polls for acceptance.
const txAcceptancePollFreq = 500 * time.Millisecond

var txCmd = &cobra.Command{
	Use:   "tx",
	Short: "Sign and submit partially signed transactions",
	Long: `Pass a partially signed transaction between machines until a multisig
owner threshold is met, then submit it.

Flow for a 2-of-3 subnet owner split across two machines:
  1. Signer A creates the tx:   subnet transfer-ownership ... --output-tx-path tx.json
  2. Send tx.json to signer B:  tx sign --tx-path tx.json
  3. Either signer submits:     tx commit --tx-path tx.json`,
	RunE: requireSubcommand,
}

var txSignCmd = &cobra.Command{
	Use:   "sign",
	Short: "Add signatures to a partially signed transaction",
	Long: `Sign a transaction file with locally available keys.

Existing signatures are preserved; only empty signature slots matching the
loaded keys are filled. The file is updated in place.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

		tx, subnetID, netConfig, err := loadTxFile(ctx)
		if err != nil {
			return err
		}

		w, cleanup, err := loadPChainWalletWithSubnet(ctx, netConfig, subnetID)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}
		defer cleanup()

		if err := pchain.SignTx(ctx, w, tx); err != nil {
			return err
		}

		data, err := pchain.EncodeTxFile(netConfig.NetworkID, tx)
		if err != nil {
			return err
		}
		if err := os.WriteFile(txPath, data, 0o600); err != nil {
			return fmt.Errorf("failed to write tx file: %w", err)
		}

		return printSubnetAuthProgress(ctx, netConfig, tx, subnetID, txPath)
	},
}

var txCommitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Submit a fully signed transaction",
	Long:  `Submit a fully signed transaction file to the network and wait for acceptance.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

		tx, subnetID, netConfig, err := loadTxFile(ctx)
		if err != nil {
			return err
		}

		client := platformvm.NewClient(netConfig.RPCURL)

		// Refuse to submit if signatures are still missing, with a clear
		// list of who still needs to sign.
		subnet, err := client.GetSubnet(ctx, subnetID)
		if err != nil {
			return fmt.Errorf("failed to fetch subnet owner: %w", err)
		}
		_, remaining, err := pchain.SubnetAuthSigners(tx, subnet.ControlKeys)
		if err != nil {
			return err
		}
		if len(remaining) > 0 {
			return fmt.Errorf("tx is not fully signed; still missing %d signature(s): %s (run: platform-cli tx sign --tx-path %s)",
				len(remaining), formatOwnerAddresses(remaining, netConfig.NetworkID), txPath)
		}

		fmt.Println("Submitting transaction...")
		txID, err := client.IssueTx(ctx, tx.Bytes())
		if err != nil {
			return fmt.Errorf("failed to issue tx: %w", err)
		}
		if err := client.AwaitTxAccepted(ctx, txID, txAcceptancePollFreq); err != nil {
			return fmt.Errorf("tx %s issued but not yet accepted: %w", txID, err)
		}

		fmt.Printf("TX accepted: %s\n", txID)
		return nil
	},
}

// loadTxFile reads and decodes --tx-path, verifies it matches the selected
// network, and returns the tx plus the subnet it modifies.
func loadTxFile(ctx context.Context) (*txs.Tx, ids.ID, network.Config, error) {
	if txPath == "" {
		return nil, ids.Empty, network.Config{}, fmt.Errorf("--tx-path is required")
	}

	data, err := os.ReadFile(txPath)
	if err != nil {
		return nil, ids.Empty, network.Config{}, fmt.Errorf("failed to read tx file: %w", err)
	}
	fileNetworkID, tx, err := pchain.DecodeTxFile(data)
	if err != nil {
		return nil, ids.Empty, network.Config{}, err
	}

	utx, ok := tx.Unsigned.(*txs.TransferSubnetOwnershipTx)
	if !ok {
		return nil, ids.Empty, network.Config{}, fmt.Errorf("unsupported tx type %T (only TransferSubnetOwnershipTx is supported)", tx.Unsigned)
	}

	netConfig, err := getNetworkConfig(ctx)
	if err != nil {
		return nil, ids.Empty, network.Config{}, fmt.Errorf("failed to get network config: %w", err)
	}
	if fileNetworkID != netConfig.NetworkID {
		return nil, ids.Empty, network.Config{}, fmt.Errorf("tx file was built for network ID %d but the selected network is %d", fileNetworkID, netConfig.NetworkID)
	}

	return tx, utx.Subnet, netConfig, nil
}

// printSubnetAuthProgress reports which owner signatures the tx has and which
// are still missing.
func printSubnetAuthProgress(ctx context.Context, netConfig network.Config, tx *txs.Tx, subnetID ids.ID, path string) error {
	client := platformvm.NewClient(netConfig.RPCURL)
	subnet, err := client.GetSubnet(ctx, subnetID)
	if err != nil {
		return fmt.Errorf("failed to fetch subnet owner: %w", err)
	}

	signed, remaining, err := pchain.SubnetAuthSigners(tx, subnet.ControlKeys)
	if err != nil {
		return err
	}

	fmt.Printf("Signatures: %d of %d collected\n", len(signed), len(signed)+len(remaining))
	if len(signed) > 0 {
		fmt.Printf("  Signed:    %s\n", formatOwnerAddresses(signed, netConfig.NetworkID))
	}
	if len(remaining) > 0 {
		fmt.Printf("  Remaining: %s\n", formatOwnerAddresses(remaining, netConfig.NetworkID))
		fmt.Printf("Next signer runs: platform-cli tx sign --tx-path %s\n", path)
		return nil
	}
	fmt.Printf("Fully signed. Submit with: platform-cli tx commit --tx-path %s\n", path)
	return nil
}

// formatOwnerAddresses renders owner addresses as bech32 for the network.
func formatOwnerAddresses(addrs []ids.ShortID, networkID uint32) string {
	out := ""
	for i, addr := range addrs {
		if i > 0 {
			out += ", "
		}
		out += wallet.FormatPChainAddress(addr, networkID)
	}
	return out
}

func init() {
	rootCmd.AddCommand(txCmd)
	txCmd.AddCommand(txSignCmd)
	txCmd.AddCommand(txCommitCmd)

	txSignCmd.Flags().StringVar(&txPath, "tx-path", "", "Path of the transaction file")
	txCommitCmd.Flags().StringVar(&txPath, "tx-path", "", "Path of the transaction file")
}
