package cmd

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/ava-labs/avalanchego/api/info"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/crypto/bls"
	"github.com/ava-labs/avalanchego/vms/platformvm/signer"
	"github.com/ava-labs/platform-cli/pkg/multisig"
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
	valRewardAddresses string // comma-separated reward addresses for multisig
	valRewardThreshold uint32 // threshold for multisig reward owner
	valNodeEndpoint    string
	valBLSPublicKey    string
	valBLSPoP          string
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
		if valRewardAddr != "" && valRewardAddresses != "" {
			return fmt.Errorf("use either --reward-address (single) or --reward-addresses (multisig), not both")
		}
		if valRewardAddr != "" {
			rewardAddr, err = ids.ShortFromString(valRewardAddr)
			if err != nil {
				return fmt.Errorf("invalid reward address: %w", err)
			}
		}

		// Build reward owner (multisig or single)
		cfg := pchain.AddPermissionlessValidatorConfig{
			NodeID:    nodeID,
			Start:     start,
			End:       end,
			RewardAddr: rewardAddr,
		}

		if valRewardAddresses != "" {
			hrp := constants.GetHRP(netConfig.NetworkID)
			addrs, err := multisig.ParseAddresses(valRewardAddresses, hrp)
			if err != nil {
				return fmt.Errorf("invalid --reward-addresses: %w", err)
			}
			threshold := valRewardThreshold
			if threshold == 0 {
				threshold = uint32(len(addrs))
			}
			rewardOwner, err := multisig.NewOutputOwners(addrs, threshold)
			if err != nil {
				return fmt.Errorf("invalid multisig reward config: %w", err)
			}
			cfg.RewardOwner = rewardOwner
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

		cfg.StakeAmt = stakeNAVAX
		cfg.DelegationFee = delegationFeeShares
		cfg.BLSSigner = nodePoP

		fmt.Printf("Adding validator %s with %.9f AVAX stake...\n", nodeID, valStakeAmount)
		fmt.Printf("  Start: %s\n", start.UTC().Format("2006-01-02 15:04:05 MST"))
		fmt.Printf("  End: %s\n", end.UTC().Format("2006-01-02 15:04:05 MST"))
		fmt.Printf("  Delegation Fee: %.2f%%\n", valDelegationFee*100)
		if cfg.RewardOwner != nil {
			fmt.Printf("  Reward Owner: %d-of-%d multisig\n", cfg.RewardOwner.Threshold, len(cfg.RewardOwner.Addrs))
		}
		if nodeURI != "" {
			fmt.Printf("  Node Endpoint: %s\n", nodeURI)
		} else {
			fmt.Println("  BLS PoP Source: --bls-public-key/--bls-pop flags")
		}
		fmt.Println("Submitting transaction...")

		txID, err := pchain.AddPermissionlessValidator(ctx, w, cfg)
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
		if valRewardAddr != "" && valRewardAddresses != "" {
			return fmt.Errorf("use either --reward-address (single) or --reward-addresses (multisig), not both")
		}
		if valRewardAddr != "" {
			rewardAddr, err = ids.ShortFromString(valRewardAddr)
			if err != nil {
				return fmt.Errorf("invalid reward address: %w", err)
			}
		}

		cfg := pchain.AddPermissionlessDelegatorConfig{
			NodeID:     nodeID,
			Start:      start,
			End:        end,
			RewardAddr: rewardAddr,
		}

		if valRewardAddresses != "" {
			hrp := constants.GetHRP(netConfig.NetworkID)
			addrs, err := multisig.ParseAddresses(valRewardAddresses, hrp)
			if err != nil {
				return fmt.Errorf("invalid --reward-addresses: %w", err)
			}
			threshold := valRewardThreshold
			if threshold == 0 {
				threshold = uint32(len(addrs))
			}
			rewardOwner, err := multisig.NewOutputOwners(addrs, threshold)
			if err != nil {
				return fmt.Errorf("invalid multisig reward config: %w", err)
			}
			cfg.RewardOwner = rewardOwner
		}

		stakeNAVAX, err := avaxToNAVAX(valStakeAmount)
		if err != nil {
			return fmt.Errorf("invalid stake amount: %w", err)
		}
		if stakeNAVAX < netConfig.MinDelegatorStake {
			return fmt.Errorf("stake too low for %s: minimum is %.9f AVAX", netConfig.Name, float64(netConfig.MinDelegatorStake)/1e9)
		}
		cfg.StakeAmt = stakeNAVAX

		fmt.Printf("Delegating %.9f AVAX to validator %s...\n", valStakeAmount, nodeID)
		fmt.Printf("  Start: %s\n", start.UTC().Format("2006-01-02 15:04:05 MST"))
		fmt.Printf("  End: %s\n", end.UTC().Format("2006-01-02 15:04:05 MST"))
		if cfg.RewardOwner != nil {
			fmt.Printf("  Reward Owner: %d-of-%d multisig\n", cfg.RewardOwner.Threshold, len(cfg.RewardOwner.Addrs))
		}
		fmt.Println("Submitting transaction...")

		txID, err := pchain.AddPermissionlessDelegator(ctx, w, cfg)
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
	validatorAddCmd.Flags().StringVar(&valRewardAddresses, "reward-addresses", "", "Comma-separated reward addresses (for multisig)")
	validatorAddCmd.Flags().Uint32Var(&valRewardThreshold, "reward-threshold", 0, "Reward address signature threshold (default: all must sign)")

	// Delegate flags
	validatorDelegateCmd.Flags().StringVar(&valNodeID, "node-id", "", "Node ID to delegate to")
	validatorDelegateCmd.Flags().Float64Var(&valStakeAmount, "stake", 0, "Stake amount in AVAX (min 25)")
	validatorDelegateCmd.Flags().StringVar(&valStartTime, "start", "now", "Start time (RFC3339 or 'now')")
	validatorDelegateCmd.Flags().StringVar(&valDuration, "duration", "336h", "Delegation duration (min 14 days)")
	validatorDelegateCmd.Flags().StringVar(&valRewardAddr, "reward-address", "", "Reward address (default: own address)")
	validatorDelegateCmd.Flags().StringVar(&valRewardAddresses, "reward-addresses", "", "Comma-separated reward addresses (for multisig)")
	validatorDelegateCmd.Flags().Uint32Var(&valRewardThreshold, "reward-threshold", 0, "Reward address signature threshold (default: all must sign)")
}
