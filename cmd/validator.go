package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/vms/platformvm/signer"
	"github.com/ava-labs/platform-cli/pkg/network"
	"github.com/ava-labs/platform-cli/pkg/pchain"
	"github.com/ava-labs/platform-cli/pkg/wallet"
	"github.com/spf13/cobra"
)

var (
	valNodeID        string
	valStakeAmount   float64
	valStartTime     string
	valDuration      string
	valDelegationFee float64
	valRewardAddr    string
	valSubnetID      string
	valWeight        uint64
	valAssetID       string
)

var validatorCmd = &cobra.Command{
	Use:   "validator",
	Short: "Validator and delegation operations",
	Long:  `Add validators and delegators to the primary network or subnets.`,
}

// =============================================================================
// Primary Network
// =============================================================================

var validatorAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a primary network validator",
	Long:  `Add a validator to the Avalanche primary network.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		nodeID, err := ids.NodeIDFromString(valNodeID)
		if err != nil {
			return fmt.Errorf("invalid node ID: %w", err)
		}

		start, end, err := parseTimeRange(valStartTime, valDuration)
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
		w, err := wallet.NewWallet(ctx, key, netConfig)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}

		rewardAddr := w.PChainAddress()
		if valRewardAddr != "" {
			rewardAddr, err = ids.ShortFromString(valRewardAddr)
			if err != nil {
				return fmt.Errorf("invalid reward address: %w", err)
			}
		}

		txID, err := pchain.AddValidator(ctx, w, pchain.AddValidatorConfig{
			NodeID:        nodeID,
			Start:         start,
			End:           end,
			StakeAmt:      uint64(valStakeAmount * 1e9),
			RewardAddr:    rewardAddr,
			DelegationFee: uint32(valDelegationFee * 10000), // Convert to basis points
		})
		if err != nil {
			return err
		}

		fmt.Printf("Add Validator TX: %s\n", txID)
		return nil
	},
}

var validatorDelegateCmd = &cobra.Command{
	Use:   "delegate",
	Short: "Delegate to a primary network validator",
	Long:  `Delegate stake to an existing primary network validator.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		nodeID, err := ids.NodeIDFromString(valNodeID)
		if err != nil {
			return fmt.Errorf("invalid node ID: %w", err)
		}

		start, end, err := parseTimeRange(valStartTime, valDuration)
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
		w, err := wallet.NewWallet(ctx, key, netConfig)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}

		rewardAddr := w.PChainAddress()
		if valRewardAddr != "" {
			rewardAddr, err = ids.ShortFromString(valRewardAddr)
			if err != nil {
				return fmt.Errorf("invalid reward address: %w", err)
			}
		}

		txID, err := pchain.AddDelegator(ctx, w, pchain.AddDelegatorConfig{
			NodeID:     nodeID,
			Start:      start,
			End:        end,
			StakeAmt:   uint64(valStakeAmount * 1e9),
			RewardAddr: rewardAddr,
		})
		if err != nil {
			return err
		}

		fmt.Printf("Add Delegator TX: %s\n", txID)
		return nil
	},
}

// =============================================================================
// Subnet Validators (Legacy Permissioned)
// =============================================================================

var validatorAddSubnetCmd = &cobra.Command{
	Use:   "add-subnet",
	Short: "Add a subnet validator",
	Long:  `Add a validator to a permissioned subnet.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		nodeID, err := ids.NodeIDFromString(valNodeID)
		if err != nil {
			return fmt.Errorf("invalid node ID: %w", err)
		}

		subnetID, err := ids.FromString(valSubnetID)
		if err != nil {
			return fmt.Errorf("invalid subnet ID: %w", err)
		}

		start, end, err := parseTimeRange(valStartTime, valDuration)
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
		w, err := wallet.NewWalletWithSubnet(ctx, key, netConfig, subnetID)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}

		weight := valWeight
		if weight == 0 {
			weight = 1
		}

		txID, err := pchain.AddSubnetValidator(ctx, w, pchain.AddSubnetValidatorConfig{
			NodeID:   nodeID,
			SubnetID: subnetID,
			Start:    start,
			End:      end,
			Weight:   weight,
		})
		if err != nil {
			return err
		}

		fmt.Printf("Add Subnet Validator TX: %s\n", txID)
		return nil
	},
}

var validatorRemoveSubnetCmd = &cobra.Command{
	Use:   "remove-subnet",
	Short: "Remove a subnet validator",
	Long:  `Remove a validator from a permissioned subnet.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		nodeID, err := ids.NodeIDFromString(valNodeID)
		if err != nil {
			return fmt.Errorf("invalid node ID: %w", err)
		}

		subnetID, err := ids.FromString(valSubnetID)
		if err != nil {
			return fmt.Errorf("invalid subnet ID: %w", err)
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

		txID, err := pchain.RemoveSubnetValidator(ctx, w, nodeID, subnetID)
		if err != nil {
			return err
		}

		fmt.Printf("Remove Subnet Validator TX: %s\n", txID)
		return nil
	},
}

// =============================================================================
// Permissionless Validators (Elastic Subnets)
// =============================================================================

var validatorAddPermissionlessCmd = &cobra.Command{
	Use:   "add-permissionless",
	Short: "Add a permissionless validator (elastic subnet)",
	Long:  `Add a permissionless validator to an elastic subnet.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		nodeID, err := ids.NodeIDFromString(valNodeID)
		if err != nil {
			return fmt.Errorf("invalid node ID: %w", err)
		}

		subnetID, err := ids.FromString(valSubnetID)
		if err != nil {
			return fmt.Errorf("invalid subnet ID: %w", err)
		}

		assetID, err := ids.FromString(valAssetID)
		if err != nil {
			return fmt.Errorf("invalid asset ID: %w", err)
		}

		start, end, err := parseTimeRange(valStartTime, valDuration)
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
		w, err := wallet.NewWallet(ctx, key, netConfig)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}

		rewardAddr := w.PChainAddress()
		if valRewardAddr != "" {
			rewardAddr, err = ids.ShortFromString(valRewardAddr)
			if err != nil {
				return fmt.Errorf("invalid reward address: %w", err)
			}
		}

		txID, err := pchain.AddPermissionlessValidator(ctx, w, pchain.AddPermissionlessValidatorConfig{
			NodeID:        nodeID,
			SubnetID:      subnetID,
			Start:         start,
			End:           end,
			StakeAmt:      uint64(valStakeAmount * 1e9),
			AssetID:       assetID,
			RewardAddr:    rewardAddr,
			DelegationFee: uint32(valDelegationFee * 10000),
			Signer:        &signer.Empty{},
		})
		if err != nil {
			return err
		}

		fmt.Printf("Add Permissionless Validator TX: %s\n", txID)
		return nil
	},
}

var validatorDelegatePermissionlessCmd = &cobra.Command{
	Use:   "delegate-permissionless",
	Short: "Delegate to a permissionless validator (elastic subnet)",
	Long:  `Delegate stake to a permissionless validator on an elastic subnet.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		nodeID, err := ids.NodeIDFromString(valNodeID)
		if err != nil {
			return fmt.Errorf("invalid node ID: %w", err)
		}

		subnetID, err := ids.FromString(valSubnetID)
		if err != nil {
			return fmt.Errorf("invalid subnet ID: %w", err)
		}

		assetID, err := ids.FromString(valAssetID)
		if err != nil {
			return fmt.Errorf("invalid asset ID: %w", err)
		}

		start, end, err := parseTimeRange(valStartTime, valDuration)
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
		w, err := wallet.NewWallet(ctx, key, netConfig)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}

		rewardAddr := w.PChainAddress()
		if valRewardAddr != "" {
			rewardAddr, err = ids.ShortFromString(valRewardAddr)
			if err != nil {
				return fmt.Errorf("invalid reward address: %w", err)
			}
		}

		txID, err := pchain.AddPermissionlessDelegator(ctx, w, pchain.AddPermissionlessDelegatorConfig{
			NodeID:     nodeID,
			SubnetID:   subnetID,
			Start:      start,
			End:        end,
			StakeAmt:   uint64(valStakeAmount * 1e9),
			AssetID:    assetID,
			RewardAddr: rewardAddr,
		})
		if err != nil {
			return err
		}

		fmt.Printf("Add Permissionless Delegator TX: %s\n", txID)
		return nil
	},
}

// parseTimeRange parses start time and duration into start/end times.
func parseTimeRange(startStr, durationStr string) (time.Time, time.Time, error) {
	var start time.Time
	var err error

	if startStr == "" || startStr == "now" {
		start = time.Now().Add(30 * time.Second) // Start 30 seconds from now
	} else {
		start, err = time.Parse(time.RFC3339, startStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid start time (use RFC3339 format): %w", err)
		}
	}

	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid duration: %w", err)
	}

	end := start.Add(duration)
	return start, end, nil
}

func init() {
	rootCmd.AddCommand(validatorCmd)

	// Primary network commands
	validatorCmd.AddCommand(validatorAddCmd)
	validatorCmd.AddCommand(validatorDelegateCmd)

	// Subnet validator commands
	validatorCmd.AddCommand(validatorAddSubnetCmd)
	validatorCmd.AddCommand(validatorRemoveSubnetCmd)

	// Permissionless commands
	validatorCmd.AddCommand(validatorAddPermissionlessCmd)
	validatorCmd.AddCommand(validatorDelegatePermissionlessCmd)

	// Common flags for primary network
	for _, cmd := range []*cobra.Command{validatorAddCmd, validatorDelegateCmd} {
		cmd.Flags().StringVar(&valNodeID, "node-id", "", "Node ID to validate/delegate to")
		cmd.Flags().Float64Var(&valStakeAmount, "stake", 0, "Stake amount in AVAX")
		cmd.Flags().StringVar(&valStartTime, "start", "now", "Start time (RFC3339 or 'now')")
		cmd.Flags().StringVar(&valDuration, "duration", "336h", "Validation duration (e.g., 336h for 14 days)")
		cmd.Flags().StringVar(&valRewardAddr, "reward-address", "", "Reward address (default: own address)")
	}
	validatorAddCmd.Flags().Float64Var(&valDelegationFee, "delegation-fee", 0.02, "Delegation fee (0.02 = 2%)")

	// Subnet validator flags
	for _, cmd := range []*cobra.Command{validatorAddSubnetCmd, validatorRemoveSubnetCmd} {
		cmd.Flags().StringVar(&valNodeID, "node-id", "", "Node ID")
		cmd.Flags().StringVar(&valSubnetID, "subnet-id", "", "Subnet ID")
	}
	validatorAddSubnetCmd.Flags().StringVar(&valStartTime, "start", "now", "Start time (RFC3339 or 'now')")
	validatorAddSubnetCmd.Flags().StringVar(&valDuration, "duration", "336h", "Validation duration")
	validatorAddSubnetCmd.Flags().Uint64Var(&valWeight, "weight", 1, "Validator weight")

	// Permissionless flags
	for _, cmd := range []*cobra.Command{validatorAddPermissionlessCmd, validatorDelegatePermissionlessCmd} {
		cmd.Flags().StringVar(&valNodeID, "node-id", "", "Node ID")
		cmd.Flags().StringVar(&valSubnetID, "subnet-id", "", "Subnet ID")
		cmd.Flags().StringVar(&valAssetID, "asset-id", "", "Staking asset ID")
		cmd.Flags().Float64Var(&valStakeAmount, "stake", 0, "Stake amount")
		cmd.Flags().StringVar(&valStartTime, "start", "now", "Start time (RFC3339 or 'now')")
		cmd.Flags().StringVar(&valDuration, "duration", "336h", "Validation duration")
		cmd.Flags().StringVar(&valRewardAddr, "reward-address", "", "Reward address")
	}
	validatorAddPermissionlessCmd.Flags().Float64Var(&valDelegationFee, "delegation-fee", 0.02, "Delegation fee (0.02 = 2%)")
}
