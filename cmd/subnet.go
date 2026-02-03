package cmd

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ava-labs/avalanchego/api/info"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/units"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/platform-cli/pkg/network"
	"github.com/ava-labs/platform-cli/pkg/pchain"
	"github.com/ava-labs/platform-cli/pkg/wallet"
	"github.com/spf13/cobra"
)

var (
	subnetID           string
	subnetNewOwner     string
	subnetChainID      string
	subnetManager      string
	subnetValidatorIPs string
	subnetValBalance   float64
)

var subnetCmd = &cobra.Command{
	Use:   "subnet",
	Short: "Subnet management",
	Long:  `Create and manage subnets on the Avalanche P-Chain.`,
}

var subnetCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new subnet",
	Long:  `Create a new subnet on the P-Chain.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		keyBytes, err := loadKey()
		if err != nil {
			return err
		}
		key, err := wallet.ToPrivateKey(keyBytes)
		if err != nil {
			return err
		}

		netConfig := network.GetConfig(networkName)
		w, err := wallet.NewWallet(ctx, key, netConfig)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}

		txID, err := pchain.CreateSubnet(ctx, w)
		if err != nil {
			return err
		}

		fmt.Printf("Subnet ID: %s\n", txID)
		return nil
	},
}

var subnetTransferOwnershipCmd = &cobra.Command{
	Use:   "transfer-ownership",
	Short: "Transfer subnet ownership",
	Long:  `Transfer ownership of a subnet to a new address.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		if subnetID == "" {
			return fmt.Errorf("--subnet-id is required")
		}
		if subnetNewOwner == "" {
			return fmt.Errorf("--new-owner is required")
		}

		sid, err := ids.FromString(subnetID)
		if err != nil {
			return fmt.Errorf("invalid subnet ID: %w", err)
		}

		newOwner, err := ids.ShortFromString(subnetNewOwner)
		if err != nil {
			return fmt.Errorf("invalid new owner address: %w", err)
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
		w, err := wallet.NewWalletWithSubnet(ctx, key, netConfig, sid)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}

		txID, err := pchain.TransferSubnetOwnership(ctx, w, sid, newOwner)
		if err != nil {
			return err
		}

		fmt.Printf("Transfer Subnet Ownership TX: %s\n", txID)
		return nil
	},
}

var subnetConvertL1Cmd = &cobra.Command{
	Use:   "convert-l1",
	Short: "Convert subnet to L1",
	Long:  `Convert a permissioned subnet to an L1 blockchain.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		if subnetID == "" {
			return fmt.Errorf("--subnet-id is required")
		}
		if subnetChainID == "" {
			return fmt.Errorf("--chain-id is required")
		}

		sid, err := ids.FromString(subnetID)
		if err != nil {
			return fmt.Errorf("invalid subnet ID: %w", err)
		}

		cid, err := ids.FromString(subnetChainID)
		if err != nil {
			return fmt.Errorf("invalid chain ID: %w", err)
		}

		var managerAddr []byte
		if subnetManager != "" {
			addrStr := strings.TrimPrefix(subnetManager, "0x")
			managerAddr, err = hex.DecodeString(addrStr)
			if err != nil {
				return fmt.Errorf("invalid manager address: %w", err)
			}
		}

		// Gather validator info from IPs
		validators, err := gatherL1Validators(ctx, subnetValidatorIPs, subnetValBalance)
		if err != nil {
			return err
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
		w, err := wallet.NewWalletWithSubnet(ctx, key, netConfig, sid)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}

		txID, err := pchain.ConvertSubnetToL1(ctx, w, sid, cid, managerAddr, validators)
		if err != nil {
			return err
		}

		fmt.Printf("Convert Subnet to L1 TX: %s\n", txID)
		return nil
	},
}

// gatherL1Validators queries validator nodes and builds conversion validators.
func gatherL1Validators(ctx context.Context, validatorIPs string, balance float64) ([]*txs.ConvertSubnetToL1Validator, error) {
	ips := parseValidatorIPs(validatorIPs)
	if len(ips) == 0 {
		return nil, nil // No validators is valid
	}

	validators := make([]*txs.ConvertSubnetToL1Validator, 0, len(ips))

	for _, ip := range ips {
		uri := fmt.Sprintf("http://%s:9650", ip)
		infoClient := info.NewClient(uri)

		nodeID, nodePoP, err := infoClient.GetNodeID(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get node info from %s: %w", uri, err)
		}

		validators = append(validators, &txs.ConvertSubnetToL1Validator{
			NodeID:  nodeID.Bytes(),
			Weight:  units.Schmeckle,
			Balance: uint64(balance * float64(units.Avax)),
			Signer:  *nodePoP,
		})
	}

	return validators, nil
}

func parseValidatorIPs(ipList string) []string {
	var ips []string
	for _, ip := range strings.Split(ipList, ",") {
		ip = strings.TrimSpace(ip)
		if ip != "" {
			ips = append(ips, ip)
		}
	}
	return ips
}

func init() {
	rootCmd.AddCommand(subnetCmd)

	subnetCmd.AddCommand(subnetCreateCmd)
	subnetCmd.AddCommand(subnetTransferOwnershipCmd)
	subnetCmd.AddCommand(subnetConvertL1Cmd)

	// Transfer ownership flags
	subnetTransferOwnershipCmd.Flags().StringVar(&subnetID, "subnet-id", "", "Subnet ID")
	subnetTransferOwnershipCmd.Flags().StringVar(&subnetNewOwner, "new-owner", "", "New owner P-Chain address")

	// Convert L1 flags
	subnetConvertL1Cmd.Flags().StringVar(&subnetID, "subnet-id", "", "Subnet ID to convert")
	subnetConvertL1Cmd.Flags().StringVar(&subnetChainID, "chain-id", "", "Chain ID for the L1")
	subnetConvertL1Cmd.Flags().StringVar(&subnetManager, "manager", "", "Validator manager address (hex)")
	subnetConvertL1Cmd.Flags().StringVar(&subnetValidatorIPs, "validators", "", "Comma-separated validator IPs")
	subnetConvertL1Cmd.Flags().Float64Var(&subnetValBalance, "validator-balance", 1.0, "Balance per validator in AVAX")
}
