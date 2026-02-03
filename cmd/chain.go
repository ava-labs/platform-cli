package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/platform-cli/pkg/network"
	"github.com/ava-labs/platform-cli/pkg/pchain"
	"github.com/ava-labs/platform-cli/pkg/wallet"
	"github.com/spf13/cobra"
)

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
		ctx := context.Background()

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

		genesis, err := os.ReadFile(chainGenesisFile)
		if err != nil {
			return fmt.Errorf("failed to read genesis file: %w", err)
		}

		// Default to Subnet-EVM
		vmID := constants.SubnetEVMID
		if chainVMID != "" {
			vmID, err = ids.FromString(chainVMID)
			if err != nil {
				return fmt.Errorf("invalid VM ID: %w", err)
			}
		}

		keyBytes, err := loadKey()
		if err != nil {
			return err
		}
		key, err := wallet.ToPrivateKey(keyBytes)
		if err != nil {
			return err
		}

		netConfig := network.GetConfig(networkName)
		w, err := wallet.NewWalletWithSubnet(ctx, key, netConfig, subnetID)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}

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
