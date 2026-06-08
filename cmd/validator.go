package cmd

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ava-labs/avalanchego/api/info"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto/bls"
	"github.com/ava-labs/avalanchego/vms/platformvm/signer"
	nodeutil "github.com/ava-labs/platform-cli/pkg/node"
	"github.com/ava-labs/platform-cli/pkg/pchain"
	"github.com/spf13/cobra"
)

var (
	valNodeID          string
	valStakeAmount     float64
	valStartTime       string
	valDuration        string
	valDelegationFee   float64
	valRewardAddr      string
	valNodeEndpoint    string
	valBLSPublicKey    string
	valBLSPoP          string
	valAutoPeriod      string
	valAutoCompound    float64
	valOwnerAddr       string
	valSetAutoTxID     string
	valSetAutoPeriod   string
	valSetAutoCompound float64
	valRewardAutoTxID  string
	valRewardAutoTime  string
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

		if valStakeAmount <= 0 {
			return fmt.Errorf("--stake is required and must be positive")
		}
		if valNodeID == "" {
			return fmt.Errorf("--node-id is required")
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
		if end.Sub(start) < netConfig.MinStakeDuration {
			return fmt.Errorf("duration too short for %s: minimum is %s", netConfig.Name, netConfig.MinStakeDuration)
		}

		nodePoP, nodeURI, err := getValidatorPoP(ctx, nodeID)
		if err != nil {
			return err
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
		if stakeNAVAX < netConfig.MinValidatorStake {
			return fmt.Errorf("stake too low for %s: minimum is %.9f AVAX", netConfig.Name, float64(netConfig.MinValidatorStake)/1e9)
		}

		delegationFeeShares, err := feeToShares(valDelegationFee)
		if err != nil {
			return fmt.Errorf("invalid delegation fee: %w", err)
		}

		fmt.Printf("Adding validator %s with %.9f AVAX stake...\n", nodeID, valStakeAmount)
		fmt.Printf("  Start: %s\n", start.UTC().Format("2006-01-02 15:04:05 MST"))
		fmt.Printf("  End: %s\n", end.UTC().Format("2006-01-02 15:04:05 MST"))
		fmt.Printf("  Delegation Fee: %.2f%%\n", valDelegationFee*100)
		if nodeURI != "" {
			fmt.Printf("  Node Endpoint: %s\n", nodeURI)
		} else {
			fmt.Println("  BLS PoP Source: --bls-public-key/--bls-pop flags")
		}
		fmt.Println("Submitting transaction...")

		txID, err := pchain.AddPermissionlessValidator(ctx, w, pchain.AddPermissionlessValidatorConfig{
			NodeID:        nodeID,
			Start:         start,
			End:           end,
			StakeAmt:      stakeNAVAX,
			RewardAddr:    rewardAddr,
			DelegationFee: delegationFeeShares,
			BLSSigner:     nodePoP,
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
		if end.Sub(start) < netConfig.MinStakeDuration {
			return fmt.Errorf("duration too short for %s: minimum is %s", netConfig.Name, netConfig.MinStakeDuration)
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
		if stakeNAVAX < netConfig.MinDelegatorStake {
			return fmt.Errorf("stake too low for %s: minimum is %.9f AVAX", netConfig.Name, float64(netConfig.MinDelegatorStake)/1e9)
		}

		fmt.Printf("Delegating %.9f AVAX to validator %s...\n", valStakeAmount, nodeID)
		fmt.Printf("  Start: %s\n", start.UTC().Format("2006-01-02 15:04:05 MST"))
		fmt.Printf("  End: %s\n", end.UTC().Format("2006-01-02 15:04:05 MST"))
		fmt.Println("Submitting transaction...")

		txID, err := pchain.AddPermissionlessDelegator(ctx, w, pchain.AddPermissionlessDelegatorConfig{
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

var validatorAddAutoRenewedCmd = &cobra.Command{
	Use:   "add-auto-renewed",
	Short: "Add an auto-renewed primary network validator",
	Long:  `Add an auto-renewed validator to the Avalanche primary network.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

		if valStakeAmount <= 0 {
			return fmt.Errorf("--stake is required and must be positive")
		}
		if valNodeID == "" {
			return fmt.Errorf("--node-id is required")
		}
		nodeID, err := ids.NodeIDFromString(valNodeID)
		if err != nil {
			return fmt.Errorf("invalid node ID: %w", err)
		}

		period, err := parseAutoRenewPeriod(valAutoPeriod)
		if err != nil {
			return err
		}

		netConfig, err := getNetworkConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to get network config: %w", err)
		}
		if period < netConfig.MinStakeDuration {
			return fmt.Errorf("period too short for %s: minimum is %s", netConfig.Name, netConfig.MinStakeDuration)
		}

		nodePoP, nodeURI, err := getValidatorPoP(ctx, nodeID)
		if err != nil {
			return err
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

		authorityAddr := w.PChainAddress()
		if valOwnerAddr != "" {
			authorityAddr, err = ids.ShortFromString(valOwnerAddr)
			if err != nil {
				return fmt.Errorf("invalid owner address: %w", err)
			}
		}

		stakeNAVAX, err := avaxToNAVAX(valStakeAmount)
		if err != nil {
			return fmt.Errorf("invalid stake amount: %w", err)
		}
		if stakeNAVAX < netConfig.MinValidatorStake {
			return fmt.Errorf("stake too low for %s: minimum is %.9f AVAX", netConfig.Name, float64(netConfig.MinValidatorStake)/1e9)
		}

		delegationFeeShares, err := feeToShares(valDelegationFee)
		if err != nil {
			return fmt.Errorf("invalid delegation fee: %w", err)
		}
		autoCompoundShares, err := fractionToShares("auto-compound", valAutoCompound)
		if err != nil {
			return fmt.Errorf("invalid auto-compound: %w", err)
		}

		fmt.Printf("Adding auto-renewed validator %s with %.9f AVAX stake...\n", nodeID, valStakeAmount)
		fmt.Printf("  Period: %s\n", period)
		fmt.Printf("  Delegation Fee: %.2f%%\n", valDelegationFee*100)
		fmt.Printf("  Auto-Compound Rewards: %.2f%%\n", valAutoCompound*100)
		fmt.Printf("  Validator Authority: %s\n", authorityAddr)
		if nodeURI != "" {
			fmt.Printf("  Node Endpoint: %s\n", nodeURI)
		} else {
			fmt.Println("  BLS PoP Source: --bls-public-key/--bls-pop flags")
		}
		fmt.Println("Submitting transaction...")

		txID, err := pchain.AddAutoRenewedValidator(ctx, w, pchain.AddAutoRenewedValidatorConfig{
			NodeID:                   nodeID,
			StakeAmt:                 stakeNAVAX,
			RewardAddr:               rewardAddr,
			ValidatorAuthorityAddr:   authorityAddr,
			DelegationFee:            delegationFeeShares,
			AutoCompoundRewardShares: autoCompoundShares,
			Period:                   period,
			BLSSigner:                nodePoP,
		})
		if err != nil {
			return err
		}

		fmt.Printf("TX ID: %s\n", txID)
		return nil
	},
}

var validatorSetAutoConfigCmd = &cobra.Command{
	Use:   "set-auto-config",
	Short: "Set auto-renewed validator config",
	Long:  `Set the next-cycle configuration for an auto-renewed validator.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

		if valSetAutoTxID == "" {
			return fmt.Errorf("--tx-id is required")
		}
		autoRenewedTxID, err := ids.FromString(valSetAutoTxID)
		if err != nil {
			return fmt.Errorf("invalid tx ID: %w", err)
		}

		if !cmd.Flags().Changed("period") {
			return fmt.Errorf("--period is required")
		}
		period, err := parseAutoRenewConfigPeriod(valSetAutoPeriod)
		if err != nil {
			return err
		}

		if !cmd.Flags().Changed("auto-compound") {
			return fmt.Errorf("--auto-compound is required")
		}
		autoCompoundShares, err := fractionToShares("auto-compound", valSetAutoCompound)
		if err != nil {
			return fmt.Errorf("invalid auto-compound: %w", err)
		}

		netConfig, err := getNetworkConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to get network config: %w", err)
		}
		if period > 0 && period < netConfig.MinStakeDuration {
			return fmt.Errorf("period too short for %s: minimum is %s", netConfig.Name, netConfig.MinStakeDuration)
		}

		validatorAuthority, err := pchain.GetAutoRenewedValidatorAuthority(ctx, netConfig.RPCURL, autoRenewedTxID)
		if err != nil {
			return err
		}

		w, cleanup, err := loadPChainWallet(ctx, netConfig)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}
		defer cleanup()

		fmt.Printf("Setting auto-renewed validator config for %s...\n", autoRenewedTxID)
		if period == 0 {
			fmt.Println("  Period: 0s (exit after current cycle)")
		} else {
			fmt.Printf("  Period: %s\n", period)
		}
		fmt.Printf("  Auto-Compound Rewards: %.2f%%\n", valSetAutoCompound*100)
		fmt.Println("Submitting transaction...")

		txID, err := pchain.SetAutoRenewedValidatorConfig(ctx, w, pchain.SetAutoRenewedValidatorConfigTxConfig{
			TxID:                     autoRenewedTxID,
			AutoCompoundRewardShares: autoCompoundShares,
			Period:                   period,
			ValidatorAuthority:       validatorAuthority,
		})
		if err != nil {
			return err
		}

		fmt.Printf("TX ID: %s\n", txID)
		return nil
	},
}

var validatorRewardAutoCmd = &cobra.Command{
	Use:   "reward-auto",
	Short: "Reward an auto-renewed validator cycle",
	Long:  `Reward or exit an auto-renewed validator at the end of a validation cycle.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

		if valRewardAutoTxID == "" {
			return fmt.Errorf("--tx-id is required")
		}
		autoRenewedTxID, err := ids.FromString(valRewardAutoTxID)
		if err != nil {
			return fmt.Errorf("invalid tx ID: %w", err)
		}

		if valRewardAutoTime == "" {
			return fmt.Errorf("--timestamp is required")
		}
		timestamp, timestampTime, err := parseRewardAutoTimestamp(valRewardAutoTime)
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

		fmt.Printf("Rewarding auto-renewed validator cycle for %s...\n", autoRenewedTxID)
		fmt.Printf("  Timestamp: %s (%d)\n", timestampTime.UTC().Format(time.RFC3339), timestamp)
		fmt.Println("Submitting transaction...")

		txID, err := pchain.RewardAutoRenewedValidator(ctx, w, pchain.RewardAutoRenewedValidatorConfig{
			TxID:      autoRenewedTxID,
			Timestamp: timestamp,
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
		offset := 30 * time.Second
		if useLedger {
			offset = 5 * time.Minute
		}
		start = time.Now().Add(offset)
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

func parseAutoRenewPeriod(periodStr string) (time.Duration, error) {
	period, err := time.ParseDuration(periodStr)
	if err != nil {
		return 0, fmt.Errorf("invalid period: %w", err)
	}
	if period <= 0 {
		return 0, fmt.Errorf("period must be positive")
	}
	if period%time.Second != 0 {
		return 0, fmt.Errorf("period must be a whole number of seconds")
	}
	return period, nil
}

func parseAutoRenewConfigPeriod(periodStr string) (time.Duration, error) {
	if strings.TrimSpace(periodStr) == "0" {
		return 0, nil
	}
	period, err := time.ParseDuration(periodStr)
	if err != nil {
		return 0, fmt.Errorf("invalid period: %w", err)
	}
	if period < 0 {
		return 0, fmt.Errorf("period cannot be negative")
	}
	if period%time.Second != 0 {
		return 0, fmt.Errorf("period must be a whole number of seconds")
	}
	return period, nil
}

func parseRewardAutoTimestamp(timestampStr string) (uint64, time.Time, error) {
	timestampStr = strings.TrimSpace(timestampStr)
	if timestampStr == "" {
		return 0, time.Time{}, fmt.Errorf("timestamp is required")
	}

	if timestamp, err := strconv.ParseUint(timestampStr, 10, 64); err == nil {
		if timestamp == 0 {
			return 0, time.Time{}, fmt.Errorf("timestamp must be positive")
		}
		if timestamp > uint64(1<<63-1) {
			return 0, time.Time{}, fmt.Errorf("timestamp is too large")
		}
		return timestamp, time.Unix(int64(timestamp), 0).UTC(), nil
	}

	timestampTime, err := time.Parse(time.RFC3339, timestampStr)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("invalid timestamp (use RFC3339 or Unix seconds): %w", err)
	}
	if timestampTime.Unix() <= 0 {
		return 0, time.Time{}, fmt.Errorf("timestamp must be positive")
	}
	return uint64(timestampTime.Unix()), timestampTime.UTC(), nil
}

// getValidatorPoP returns a BLS proof of possession for validator registration.
// Manual mode (default): use --bls-public-key and --bls-pop.
// Fallback mode: fetch from --node-endpoint.
func getValidatorPoP(ctx context.Context, nodeID ids.NodeID) (*signer.ProofOfPossession, string, error) {
	hasManualPub := strings.TrimSpace(valBLSPublicKey) != ""
	hasManualPoP := strings.TrimSpace(valBLSPoP) != ""

	switch {
	case hasManualPub && hasManualPoP:
		pop, err := parseManualPoP(valBLSPublicKey, valBLSPoP)
		if err != nil {
			return nil, "", err
		}
		return pop, "", nil
	case hasManualPub != hasManualPoP:
		return nil, "", fmt.Errorf("manual BLS mode requires both --bls-public-key and --bls-pop")
	case valNodeEndpoint != "":
		nodeURI, err := normalizeValidatorNodeURI(valNodeEndpoint)
		if err != nil {
			return nil, "", err
		}
		nodeInfoClient := info.NewClient(nodeURI)
		fetchedNodeID, fetchedPoP, err := nodeInfoClient.GetNodeID(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("failed to query node endpoint %s: %w", nodeURI, err)
		}
		if fetchedPoP == nil {
			return nil, "", fmt.Errorf("node endpoint %s did not return BLS proof of possession", nodeURI)
		}
		if fetchedNodeID != nodeID {
			return nil, "", fmt.Errorf("--node-id (%s) does not match node endpoint identity (%s)", nodeID, fetchedNodeID)
		}
		return fetchedPoP, nodeURI, nil
	default:
		return nil, "", fmt.Errorf("missing BLS proof of possession: provide --bls-public-key and --bls-pop (recommended), or use --node-endpoint")
	}
}

func parseManualPoP(pubKeyHex, popHex string) (*signer.ProofOfPossession, error) {
	pubKeyBytes, err := hex.DecodeString(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(pubKeyHex), "0x"), "0X"))
	if err != nil {
		return nil, fmt.Errorf("invalid --bls-public-key: %w", err)
	}
	if len(pubKeyBytes) != bls.PublicKeyLen {
		return nil, fmt.Errorf("invalid --bls-public-key length: expected %d bytes, got %d", bls.PublicKeyLen, len(pubKeyBytes))
	}

	popBytes, err := hex.DecodeString(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(popHex), "0x"), "0X"))
	if err != nil {
		return nil, fmt.Errorf("invalid --bls-pop: %w", err)
	}
	if len(popBytes) != bls.SignatureLen {
		return nil, fmt.Errorf("invalid --bls-pop length: expected %d bytes, got %d", bls.SignatureLen, len(popBytes))
	}

	pop := &signer.ProofOfPossession{}
	copy(pop.PublicKey[:], pubKeyBytes)
	copy(pop.ProofOfPossession[:], popBytes)
	if err := pop.Verify(); err != nil {
		return nil, fmt.Errorf("invalid BLS proof of possession: %w", err)
	}

	return pop, nil
}

func normalizeValidatorNodeURI(addr string) (string, error) {
	return nodeutil.NormalizeNodeURIWithInsecureHTTP(addr, allowInsecureHTTP)
}

func init() {
	rootCmd.AddCommand(validatorCmd)
	validatorCmd.AddCommand(validatorAddCmd)
	validatorCmd.AddCommand(validatorAddAutoRenewedCmd)
	validatorCmd.AddCommand(validatorSetAutoConfigCmd)
	validatorCmd.AddCommand(validatorRewardAutoCmd)
	validatorCmd.AddCommand(validatorDelegateCmd)

	// Add validator flags
	validatorAddCmd.Flags().StringVar(&valNodeID, "node-id", "", "Node ID to validate (required)")
	validatorAddCmd.Flags().StringVar(&valNodeEndpoint, "node-endpoint", "", "Validator node endpoint (fallback mode) to fetch BLS proof of possession")
	validatorAddCmd.Flags().StringVar(&valBLSPublicKey, "bls-public-key", "", "Validator BLS public key (hex, recommended/manual mode)")
	validatorAddCmd.Flags().StringVar(&valBLSPoP, "bls-pop", "", "Validator BLS proof of possession signature (hex, recommended/manual mode)")
	validatorAddCmd.Flags().Float64Var(&valStakeAmount, "stake", 0, "Stake amount in AVAX (min 2000)")
	validatorAddCmd.Flags().StringVar(&valStartTime, "start", "now", "Start time (RFC3339 or 'now')")
	validatorAddCmd.Flags().StringVar(&valDuration, "duration", "336h", "Validation duration (min 14 days)")
	validatorAddCmd.Flags().Float64Var(&valDelegationFee, "delegation-fee", 0.02, "Delegation fee (0.02 = 2%)")
	validatorAddCmd.Flags().StringVar(&valRewardAddr, "reward-address", "", "Reward address (default: own address)")

	// Add auto-renewed validator flags
	validatorAddAutoRenewedCmd.Flags().StringVar(&valNodeID, "node-id", "", "Node ID to validate (required)")
	validatorAddAutoRenewedCmd.Flags().StringVar(&valNodeEndpoint, "node-endpoint", "", "Validator node endpoint (fallback mode) to fetch BLS proof of possession")
	validatorAddAutoRenewedCmd.Flags().StringVar(&valBLSPublicKey, "bls-public-key", "", "Validator BLS public key (hex, recommended/manual mode)")
	validatorAddAutoRenewedCmd.Flags().StringVar(&valBLSPoP, "bls-pop", "", "Validator BLS proof of possession signature (hex, recommended/manual mode)")
	validatorAddAutoRenewedCmd.Flags().Float64Var(&valStakeAmount, "stake", 0, "Stake amount in AVAX (min 2000)")
	validatorAddAutoRenewedCmd.Flags().StringVar(&valAutoPeriod, "period", "336h", "Auto-renewal cycle duration (for example, 336h for 14 days)")
	validatorAddAutoRenewedCmd.Flags().Float64Var(&valDelegationFee, "delegation-fee", 0.02, "Delegation fee (0.02 = 2%)")
	validatorAddAutoRenewedCmd.Flags().Float64Var(&valAutoCompound, "auto-compound", 1, "Fraction of rewards to auto-compound (0.3 = 30%, 1 = 100%)")
	validatorAddAutoRenewedCmd.Flags().StringVar(&valRewardAddr, "reward-address", "", "Reward address (default: own address)")
	validatorAddAutoRenewedCmd.Flags().StringVar(&valOwnerAddr, "owner-address", "", "Address authorized to update auto-renew config (default: own address)")

	// Set auto-renewed validator config flags
	validatorSetAutoConfigCmd.Flags().StringVar(&valSetAutoTxID, "tx-id", "", "Original AddAutoRenewedValidatorTx ID (required)")
	validatorSetAutoConfigCmd.Flags().StringVar(&valSetAutoPeriod, "period", "", "Next auto-renewal cycle duration, or 0 to exit after the current cycle (required)")
	validatorSetAutoConfigCmd.Flags().Float64Var(&valSetAutoCompound, "auto-compound", 0, "Fraction of rewards to auto-compound (0.3 = 30%, 1 = 100%) (required)")

	// Reward auto-renewed validator flags
	validatorRewardAutoCmd.Flags().StringVar(&valRewardAutoTxID, "tx-id", "", "Original AddAutoRenewedValidatorTx ID (required)")
	validatorRewardAutoCmd.Flags().StringVar(&valRewardAutoTime, "timestamp", "", "Cycle end timestamp as RFC3339 or Unix seconds (required)")

	// Delegate flags
	validatorDelegateCmd.Flags().StringVar(&valNodeID, "node-id", "", "Node ID to delegate to")
	validatorDelegateCmd.Flags().Float64Var(&valStakeAmount, "stake", 0, "Stake amount in AVAX (min 25)")
	validatorDelegateCmd.Flags().StringVar(&valStartTime, "start", "now", "Start time (RFC3339 or 'now')")
	validatorDelegateCmd.Flags().StringVar(&valDuration, "duration", "336h", "Delegation duration (min 14 days)")
	validatorDelegateCmd.Flags().StringVar(&valRewardAddr, "reward-address", "", "Reward address (default: own address)")
}
