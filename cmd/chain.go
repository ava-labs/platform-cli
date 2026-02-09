package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/units"
	"github.com/ava-labs/platform-cli/pkg/pchain"
	"github.com/spf13/cobra"
)

// maxGenesisLen is the maximum allowed genesis file size (matches P-Chain limit).
const maxGenesisLen = units.MiB // 1 MB

var (
	chainSubnetID    string
	chainGenesisFile string
	chainName        string
	chainVMID        string
)

var chainCmd = &cobra.Command{
	Use:   "chain",
	Short: "Chain management",
	Long:  `Create and manage chains on subnets.`,
}

var chainCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new chain",
	Long:  `Create a new blockchain on a subnet.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

		if chainSubnetID == "" {
			return fmt.Errorf("--subnet-id is required")
		}
		if chainGenesisFile == "" {
			return fmt.Errorf("--genesis is required")
		}

		subnetID, err := ids.FromString(chainSubnetID)
		if err != nil {
			return fmt.Errorf("invalid subnet ID: %w", err)
		}

		// Validate genesis file size before reading to prevent memory issues
		fileInfo, err := os.Stat(chainGenesisFile)
		if err != nil {
			return fmt.Errorf("failed to stat genesis file: %w", err)
		}
		if fileInfo.Size() > maxGenesisLen {
			return fmt.Errorf("genesis file too large: %d bytes (max: %d bytes / 1 MB)", fileInfo.Size(), maxGenesisLen)
		}

		genesis, err := os.ReadFile(chainGenesisFile)
		if err != nil {
			return fmt.Errorf("failed to read genesis file: %w", err)
		}
		if !json.Valid(genesis) {
			return fmt.Errorf("genesis file contains invalid JSON")
		}

		// Default to Subnet-EVM
		vmID := constants.SubnetEVMID
		if chainVMID != "" {
			vmID, err = ids.FromString(chainVMID)
			if err != nil {
				return fmt.Errorf("invalid VM ID: %w", err)
			}
		}

		netConfig, err := getNetworkConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to get network config: %w", err)
		}

		w, cleanup, err := loadPChainWalletWithSubnet(ctx, netConfig, subnetID)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}
		defer cleanup()

		txID, err := pchain.CreateChain(ctx, w, pchain.CreateChainConfig{
			SubnetID:  subnetID,
			Genesis:   genesis,
			VMID:      vmID,
			FxIDs:     nil,
			ChainName: chainName,
		})
		if err != nil {
			return err
		}

		fmt.Printf("Chain ID: %s\n", txID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(chainCmd)
	chainCmd.AddCommand(chainCreateCmd)

	chainCreateCmd.Flags().StringVar(&chainSubnetID, "subnet-id", "", "Subnet ID to create chain on")
	chainCreateCmd.Flags().StringVar(&chainGenesisFile, "genesis", "", "Genesis file path")
	chainCreateCmd.Flags().StringVar(&chainName, "name", "mychain", "Chain name")
	chainCreateCmd.Flags().StringVar(&chainVMID, "vm-id", "", "VM ID (default: Subnet-EVM)")
}
