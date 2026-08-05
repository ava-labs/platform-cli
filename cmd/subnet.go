package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/formatting/address"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	ethcommon "github.com/ava-labs/libevm/common"
	"github.com/ava-labs/platform-cli/pkg/pchain"
	"github.com/spf13/cobra"
)

var (
	subnetID               string
	subnetNewOwners        []string
	subnetThreshold        uint32
	subnetOutputTxPath     string
	subnetChainID          string
	subnetManager          string
	subnetValidatorIPs     string
	subnetValidatorIDs     string
	subnetValidatorBLS     string
	subnetValidatorPoP     string
	subnetValBalance       float64
	subnetMockVal          bool
	subnetValidatorWeights string

	subnetValNodeID    string
	subnetValWeight    uint64
	subnetValStartTime string
	subnetValDuration  string
)

var subnetCmd = &cobra.Command{
	Use:   "subnet",
	Short: "Subnet management",
	Long:  `Create and manage subnets on the Avalanche P-Chain.`,
	RunE:  requireSubcommand,
}

var subnetCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new subnet (CreateSubnetTx)",
	Long:  `Create a new subnet on the P-Chain.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

		netConfig, err := getNetworkConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to get network config: %w", err)
		}

		w, cleanup, err := loadPChainWallet(ctx, netConfig)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}
		defer cleanup()

		fmt.Println("Creating new subnet...")
		fmt.Printf("Owner: %s\n", w.FormattedPChainAddress())
		fmt.Println("Submitting transaction...")

		txID, err := pchain.CreateSubnet(ctx, w)
		if err != nil {
			return err
		}

		fmt.Println("Subnet created successfully!")
		fmt.Printf("Subnet ID: %s\n", txID)
		return nil
	},
}

var subnetTransferOwnershipCmd = &cobra.Command{
	Use:   "transfer-ownership",
	Short: "Transfer subnet ownership (TransferSubnetOwnershipTx)",
	Long: `Transfer ownership of a subnet to a new owner.

The new owner is one or more P-Chain addresses with a signature threshold.
Pass --new-owner multiple times (or comma-separated) with --threshold to
set a multisig owner, e.g. 2-of-3.

If the current owner is a multisig whose keys live on different machines,
pass --output-tx-path to write a partially signed tx file instead of
submitting. The remaining owners add signatures with "tx sign" and anyone
submits with "tx commit".`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

		if subnetID == "" {
			return fmt.Errorf("--subnet-id is required")
		}
		if len(subnetNewOwners) == 0 {
			return fmt.Errorf("--new-owner is required")
		}
		if subnetOutputTxPath != "" {
			if _, err := os.Stat(subnetOutputTxPath); err == nil {
				return fmt.Errorf("output tx path %q already exists", subnetOutputTxPath)
			}
		}

		sid, err := ids.FromString(subnetID)
		if err != nil {
			return fmt.Errorf("invalid subnet ID: %w", err)
		}

		newOwners := make([]ids.ShortID, 0, len(subnetNewOwners))
		for _, addr := range subnetNewOwners {
			owner, err := parseOwnerAddress(addr)
			if err != nil {
				return err
			}
			newOwners = append(newOwners, owner)
		}

		// Validate before loading the wallet so bad input fails fast.
		if _, err := pchain.NewOwner(newOwners, subnetThreshold); err != nil {
			return fmt.Errorf("invalid new owner: %w", err)
		}

		netConfig, err := getNetworkConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to get network config: %w", err)
		}

		// Partial-sign mode: build the tx with signature slots for the
		// expected owner signers, sign what local keys allow, and write it
		// to a file for the other signers instead of submitting.
		if subnetOutputTxPath != "" {
			w, cleanup, err := loadPartialSignWalletWithSubnet(ctx, netConfig, sid)
			if err != nil {
				return fmt.Errorf("failed to create wallet: %w", err)
			}
			defer cleanup()

			fmt.Printf("New owner: %d address(es), threshold %d\n", len(newOwners), subnetThreshold)

			tx, err := pchain.BuildTransferSubnetOwnershipTx(ctx, w, sid, newOwners, subnetThreshold)
			if err != nil {
				return err
			}
			data, err := pchain.EncodeTxFile(netConfig.NetworkID, tx)
			if err != nil {
				return err
			}
			if err := os.WriteFile(subnetOutputTxPath, data, 0o600); err != nil {
				return fmt.Errorf("failed to write tx file: %w", err)
			}
			fmt.Printf("Wrote partially signed tx to %s\n", subnetOutputTxPath)
			return printSubnetAuthProgress(ctx, netConfig, tx, sid, subnetOutputTxPath)
		}

		w, cleanup, err := loadPChainWalletWithSubnet(ctx, netConfig, sid)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}
		defer cleanup()

		fmt.Printf("New owner: %d address(es), threshold %d\n", len(newOwners), subnetThreshold)

		txID, err := pchain.TransferSubnetOwnership(ctx, w, sid, newOwners, subnetThreshold)
		if err != nil {
			return err
		}

		fmt.Printf("Transfer Subnet Ownership TX: %s\n", txID)
		return nil
	},
}

// parseOwnerAddress accepts a bech32 P-Chain address (e.g. P-avax1... or
// P-fuji1...) or a CB58-encoded short ID.
func parseOwnerAddress(s string) (ids.ShortID, error) {
	if id, err := address.ParseToID(s); err == nil {
		return id, nil
	}
	if id, err := ids.ShortFromString(s); err == nil {
		return id, nil
	}
	return ids.ShortEmpty, fmt.Errorf("invalid owner address %q: expected a bech32 P-Chain address (P-avax1.../P-fuji1...) or CB58 short ID", s)
}

var subnetConvertL1Cmd = &cobra.Command{
	Use:   "convert-to-l1",
	Short: "Convert subnet to L1 (ConvertSubnetToL1Tx)",
	Long:  `Convert a permissioned subnet to an L1 blockchain.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

		if subnetID == "" {
			return fmt.Errorf("--subnet-id is required")
		}
		if subnetChainID == "" {
			return fmt.Errorf("--chain-id is required")
		}
		validatorAddrs := parseValidatorAddrs(subnetValidatorIPs)
		hasValidatorIPs := len(validatorAddrs) > 0
		hasManualValidators := strings.TrimSpace(subnetValidatorIDs) != "" ||
			strings.TrimSpace(subnetValidatorBLS) != "" ||
			strings.TrimSpace(subnetValidatorPoP) != ""
		hasValidatorFlag := strings.TrimSpace(subnetValidatorIPs) != ""
		switch {
		case subnetMockVal && hasValidatorIPs:
			return fmt.Errorf("--mock-validator cannot be used with --validators")
		case subnetMockVal && hasManualValidators:
			return fmt.Errorf("--mock-validator cannot be used with manual validator flags")
		case hasValidatorFlag && !hasValidatorIPs:
			return fmt.Errorf("--validators must include at least one non-empty validator address")
		case hasValidatorIPs && hasManualValidators:
			return fmt.Errorf("use either --validators (auto-discovery) or manual validator flags, not both")
		case !subnetMockVal && !hasValidatorIPs && !hasManualValidators:
			return fmt.Errorf("at least one validator is required: provide --validators, manual validator flags, or use --mock-validator for testing")
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
			managerAddr, err = decodeHexExactLength(subnetManager, ethcommon.AddressLength)
			if err != nil {
				return fmt.Errorf("invalid manager address: %w", err)
			}
		}

		// Parse optional per-validator weights
		var weights []uint64
		if strings.TrimSpace(subnetValidatorWeights) != "" {
			weights, err = parseValidatorWeights(subnetValidatorWeights)
			if err != nil {
				return fmt.Errorf("invalid --validator-weights: %w", err)
			}
		}

		// Gather validator info from IPs or generate mock
		var validators []*txs.ConvertSubnetToL1Validator
		if subnetMockVal {
			// For mock, use the first weight if provided, otherwise 0 (default)
			var mockWeight uint64
			if weights != nil {
				if len(weights) != 1 {
					return fmt.Errorf("--validator-weights must have exactly 1 value when using --mock-validator, got %d", len(weights))
				}
				mockWeight = weights[0]
			}
			mockVal, err := generateMockValidator(subnetValBalance, mockWeight)
			if err != nil {
				return fmt.Errorf("failed to generate mock validator: %w", err)
			}
			validators = []*txs.ConvertSubnetToL1Validator{mockVal}
			fmt.Printf("Using mock validator (NodeID: %x)\n", mockVal.NodeID)
		} else if hasManualValidators {
			validators, err = buildManualL1Validators(
				subnetValidatorIDs,
				subnetValidatorBLS,
				subnetValidatorPoP,
				subnetValBalance,
				weights,
			)
			if err != nil {
				return err
			}
		} else {
			validators, err = gatherL1Validators(ctx, validatorAddrs, subnetValBalance, weights)
			if err != nil {
				return err
			}
		}
		if err := sortAndValidateL1Validators(validators); err != nil {
			return err
		}

		netConfig, err := getNetworkConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to get network config: %w", err)
		}

		w, cleanup, err := loadPChainWalletWithSubnet(ctx, netConfig, sid)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}
		defer cleanup()

		fmt.Println("Converting subnet to L1...")
		fmt.Printf("  Subnet ID: %s\n", sid)
		fmt.Printf("  Chain ID: %s\n", cid)
		fmt.Printf("  Validators: %d\n", len(validators))
		fmt.Println("Submitting transaction...")

		txID, err := pchain.ConvertSubnetToL1(ctx, w, sid, cid, managerAddr, validators)
		if err != nil {
			return err
		}

		fmt.Println("Subnet converted to L1 successfully!")
		fmt.Printf("TX ID: %s\n", txID)
		return nil
	},
}

var subnetAddValidatorCmd = &cobra.Command{
	Use:   "add-validator",
	Short: "Add a validator to a permissioned subnet (AddSubnetValidatorTx)",
	Long: `Add a validator to a permissioned subnet (AddSubnetValidatorTx).

The node must already be a primary network validator, and the validation period
must fall within its primary network validation window. The subnet owner key
authorizes the transaction, so load the owner key via --key-name or --ledger.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

		if subnetID == "" {
			return fmt.Errorf("--subnet-id is required")
		}
		if subnetValNodeID == "" {
			return fmt.Errorf("--node-id is required")
		}
		if subnetValWeight == 0 {
			return fmt.Errorf("--weight is required and must be positive")
		}

		sid, err := ids.FromString(subnetID)
		if err != nil {
			return fmt.Errorf("invalid subnet ID: %w", err)
		}

		nodeID, err := ids.NodeIDFromString(subnetValNodeID)
		if err != nil {
			return fmt.Errorf("invalid node ID: %w", err)
		}

		start, end, err := parseTimeRange(subnetValStartTime, subnetValDuration)
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

		w, cleanup, err := loadPChainWalletWithSubnet(ctx, netConfig, sid)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}
		defer cleanup()

		fmt.Printf("Adding validator %s to subnet %s...\n", nodeID, sid)
		fmt.Printf("  Weight: %d\n", subnetValWeight)
		fmt.Printf("  Start: %s\n", start.UTC().Format("2006-01-02 15:04:05 MST"))
		fmt.Printf("  End: %s\n", end.UTC().Format("2006-01-02 15:04:05 MST"))
		fmt.Println("Submitting transaction...")

		txID, err := pchain.AddSubnetValidator(ctx, w, pchain.AddSubnetValidatorConfig{
			SubnetID: sid,
			NodeID:   nodeID,
			Start:    start,
			End:      end,
			Weight:   subnetValWeight,
		})
		if err != nil {
			return err
		}

		fmt.Printf("TX ID: %s\n", txID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(subnetCmd)

	subnetCmd.AddCommand(subnetCreateCmd)
	subnetCmd.AddCommand(subnetTransferOwnershipCmd)
	subnetCmd.AddCommand(subnetConvertL1Cmd)
	subnetCmd.AddCommand(subnetAddValidatorCmd)

	// Transfer ownership flags
	subnetTransferOwnershipCmd.Flags().StringVar(&subnetID, "subnet-id", "", "Subnet ID")
	subnetTransferOwnershipCmd.Flags().StringSliceVar(&subnetNewOwners, "new-owner", nil, "New owner P-Chain address (repeat or comma-separate for multisig)")
	subnetTransferOwnershipCmd.Flags().Uint32Var(&subnetThreshold, "threshold", 1, "Number of owner signatures required to authorize future subnet changes")
	subnetTransferOwnershipCmd.Flags().StringVar(&subnetOutputTxPath, "output-tx-path", "", "Write a partially signed tx to this file instead of submitting (complete with 'tx sign' and 'tx commit')")

	// Convert L1 flags
	subnetConvertL1Cmd.Flags().StringVar(&subnetID, "subnet-id", "", "Subnet ID to convert")
	subnetConvertL1Cmd.Flags().StringVar(&subnetChainID, "chain-id", "", "Chain ID where the validator manager contract lives (often the L1 chain ID)")
	subnetConvertL1Cmd.Flags().StringVar(&subnetManager, "manager", "", "Validator manager contract address (hex)")
	subnetConvertL1Cmd.Flags().StringVar(&subnetManager, "contract-address", "", "Alias for --manager")
	subnetConvertL1Cmd.Flags().StringVar(&subnetValidatorIPs, "validators", "", "Comma-separated validator node addresses (auto-fetches NodeID + BLS PoP from /ext/info)")
	subnetConvertL1Cmd.Flags().StringVar(&subnetValidatorIDs, "validator-node-ids", "", "Manual mode: comma-separated validator NodeIDs (must align with --validator-bls-public-keys and --validator-bls-pops)")
	subnetConvertL1Cmd.Flags().StringVar(&subnetValidatorBLS, "validator-bls-public-keys", "", "Manual mode: comma-separated validator BLS public keys (hex)")
	subnetConvertL1Cmd.Flags().StringVar(&subnetValidatorPoP, "validator-bls-pops", "", "Manual mode: comma-separated validator BLS proofs of possession (hex)")
	subnetConvertL1Cmd.Flags().Float64Var(&subnetValBalance, "validator-balance", 1.0, "Balance per validator in AVAX")
	subnetConvertL1Cmd.Flags().StringVar(&subnetValidatorWeights, "validator-weights", "", "Comma-separated validator weights (uint64). Must match validator count. Defaults to 100 per validator if omitted.")
	subnetConvertL1Cmd.Flags().BoolVar(&subnetMockVal, "mock-validator", false, "Use a mock validator (for testing)")

	// Add validator flags
	subnetAddValidatorCmd.Flags().StringVar(&subnetID, "subnet-id", "", "Subnet ID")
	subnetAddValidatorCmd.Flags().StringVar(&subnetValNodeID, "node-id", "", "Validator node ID (must already validate the primary network)")
	subnetAddValidatorCmd.Flags().Uint64Var(&subnetValWeight, "weight", 0, "Validator sampling weight on the subnet")
	subnetAddValidatorCmd.Flags().StringVar(&subnetValStartTime, "start", "now", "Start time (RFC3339 or 'now'). Post-Durango networks ignore this; validation begins at tx acceptance")
	subnetAddValidatorCmd.Flags().StringVar(&subnetValDuration, "duration", "336h", "Validation duration (must fall within the node's primary network validation period)")
}
