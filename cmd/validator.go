package cmd

import (
	"fmt"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/platform-cli/pkg/pchain"
	"github.com/spf13/cobra"
)

var (
	valNodeID        string
	valStakeAmount   float64
	valStartTime     string
	valDuration      string
	valDelegationFee float64
	valRewardAddr    string
)

var validatorCmd = &cobra.Command{
	Use:   "validator",
	Short: "Primary network staking",
	Long:  `Add validators and delegators to the Avalanche primary network.`,
}

var validatorAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a primary network validator",
	Long:  `Add a validator to the Avalanche primary network.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

		if valNodeID == "" {
			return fmt.Errorf("--node-id is required")
		}
		if valStakeAmount <= 0 {
			return fmt.Errorf("--stake is required and must be positive")
		}

		nodeID, err := ids.NodeIDFromString(valNodeID)
		if err != nil {
			return fmt.Errorf("invalid node ID: %w", err)
		}

		start, end, err := parseTimeRange(valStartTime, valDuration)
		if err != nil {
			return err
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

		rewardAddr := w.PChainAddress()
		if valRewardAddr != "" {
			rewardAddr, err = ids.ShortFromString(valRewardAddr)
			if err != nil {
				return fmt.Errorf("invalid reward address: %w", err)
			}
		}

		stakeNAVAX, err := avaxToNAVAX(valStakeAmount)
		if err != nil {
			return fmt.Errorf("invalid stake amount: %w", err)
		}

		delegationFeeBps, err := feeToPercent(valDelegationFee)
		if err != nil {
			return fmt.Errorf("invalid delegation fee: %w", err)
		}

		fmt.Printf("Adding validator %s with %.9f AVAX stake...\n", nodeID, valStakeAmount)

		txID, err := pchain.AddValidator(ctx, w, pchain.AddValidatorConfig{
			NodeID:        nodeID,
			Start:         start,
			End:           end,
			StakeAmt:      stakeNAVAX,
			RewardAddr:    rewardAddr,
			DelegationFee: delegationFeeBps,
		})
		if err != nil {
			return err
		}

		fmt.Printf("TX ID: %s\n", txID)
		return nil
	},
}

var validatorDelegateCmd = &cobra.Command{
	Use:   "delegate",
	Short: "Delegate to a primary network validator",
	Long:  `Delegate stake to an existing primary network validator.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

		if valNodeID == "" {
			return fmt.Errorf("--node-id is required")
		}
		if valStakeAmount <= 0 {
			return fmt.Errorf("--stake is required and must be positive")
		}

		nodeID, err := ids.NodeIDFromString(valNodeID)
		if err != nil {
			return fmt.Errorf("invalid node ID: %w", err)
		}

		start, end, err := parseTimeRange(valStartTime, valDuration)
		if err != nil {
			return err
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

		rewardAddr := w.PChainAddress()
		if valRewardAddr != "" {
			rewardAddr, err = ids.ShortFromString(valRewardAddr)
			if err != nil {
				return fmt.Errorf("invalid reward address: %w", err)
			}
		}

		stakeNAVAX, err := avaxToNAVAX(valStakeAmount)
		if err != nil {
			return fmt.Errorf("invalid stake amount: %w", err)
		}

		fmt.Printf("Delegating %.9f AVAX to validator %s...\n", valStakeAmount, nodeID)

		txID, err := pchain.AddDelegator(ctx, w, pchain.AddDelegatorConfig{
			NodeID:     nodeID,
			Start:      start,
			End:        end,
			StakeAmt:   stakeNAVAX,
			RewardAddr: rewardAddr,
		})
		if err != nil {
			return err
		}

		fmt.Printf("TX ID: %s\n", txID)
		return nil
	},
}

func parseTimeRange(startStr, durationStr string) (time.Time, time.Time, error) {
	var start time.Time
	var err error

	if startStr == "" || startStr == "now" {
		start = time.Now().Add(30 * time.Second)
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
	validatorCmd.AddCommand(validatorAddCmd)
	validatorCmd.AddCommand(validatorDelegateCmd)

	// Add validator flags
	validatorAddCmd.Flags().StringVar(&valNodeID, "node-id", "", "Node ID to validate")
	validatorAddCmd.Flags().Float64Var(&valStakeAmount, "stake", 0, "Stake amount in AVAX (min 2000)")
	validatorAddCmd.Flags().StringVar(&valStartTime, "start", "now", "Start time (RFC3339 or 'now')")
	validatorAddCmd.Flags().StringVar(&valDuration, "duration", "336h", "Validation duration (min 14 days)")
	validatorAddCmd.Flags().Float64Var(&valDelegationFee, "delegation-fee", 0.02, "Delegation fee (0.02 = 2%)")
	validatorAddCmd.Flags().StringVar(&valRewardAddr, "reward-address", "", "Reward address (default: own address)")

	// Delegate flags
	validatorDelegateCmd.Flags().StringVar(&valNodeID, "node-id", "", "Node ID to delegate to")
	validatorDelegateCmd.Flags().Float64Var(&valStakeAmount, "stake", 0, "Stake amount in AVAX (min 25)")
	validatorDelegateCmd.Flags().StringVar(&valStartTime, "start", "now", "Start time (RFC3339 or 'now')")
	validatorDelegateCmd.Flags().StringVar(&valDuration, "duration", "336h", "Delegation duration (min 14 days)")
	validatorDelegateCmd.Flags().StringVar(&valRewardAddr, "reward-address", "", "Reward address (default: own address)")
}
