package cmd

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ava-labs/avalanchego/api/info"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto/bls/signer/localsigner"
	"github.com/ava-labs/avalanchego/utils/units"
	"github.com/ava-labs/avalanchego/vms/platformvm/signer"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	ethcommon "github.com/ava-labs/libevm/common"
	nodeutil "github.com/ava-labs/platform-cli/pkg/node"
	"github.com/ava-labs/platform-cli/pkg/pchain"
	"github.com/spf13/cobra"
)

var (
	subnetID           string
	subnetNewOwner     string
	subnetChainID      string
	subnetManager      string
	subnetValidatorIPs string
	subnetValidatorIDs string
	subnetValidatorBLS string
	subnetValidatorPoP string
	subnetValBalance        float64
	subnetMockVal           bool
	subnetValidatorWeights  string
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
		fmt.Printf("Owner: %s\n", w.PChainAddress())
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
	Short: "Transfer subnet ownership",
	Long:  `Transfer ownership of a subnet to a new address.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

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

		netConfig, err := getNetworkConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to get network config: %w", err)
		}

		w, cleanup, err := loadPChainWalletWithSubnet(ctx, netConfig, sid)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}
		defer cleanup()

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

// gatherL1Validators queries validator nodes and builds conversion validators.
// If weights is non-nil, it must have the same length as validatorAddrs.
func gatherL1Validators(ctx context.Context, validatorAddrs []string, balance float64, weights []uint64) ([]*txs.ConvertSubnetToL1Validator, error) {
	if len(validatorAddrs) == 0 {
		return nil, fmt.Errorf("no validator addresses provided")
	}
	if weights != nil && len(weights) != len(validatorAddrs) {
		return nil, fmt.Errorf("validator-weights count (%d) must match validators count (%d)", len(weights), len(validatorAddrs))
	}

	// Validate balance to prevent overflow
	balanceNAVAX, err := avaxToNAVAX(balance)
	if err != nil {
		return nil, fmt.Errorf("invalid validator balance: %w", err)
	}

	validators := make([]*txs.ConvertSubnetToL1Validator, 0, len(validatorAddrs))
	for i, addr := range validatorAddrs {
		uri, err := normalizeNodeURI(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid validator address %q: %w", addr, err)
		}
		infoClient := info.NewClient(uri)

		nodeID, nodePoP, err := infoClient.GetNodeID(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get node info from %s: %w", uri, err)
		}
		if nodePoP == nil {
			return nil, fmt.Errorf("node %s did not return BLS proof of possession from /ext/info", uri)
		}

		weight := uint64(units.Schmeckle)
		if weights != nil {
			weight = weights[i]
		}

		validators = append(validators, &txs.ConvertSubnetToL1Validator{
			NodeID:  nodeID.Bytes(),
			Weight:  weight,
			Balance: balanceNAVAX,
			Signer:  *nodePoP,
		})
	}

	return validators, nil
}

// buildManualL1Validators builds conversion validators from manually provided data.
// All inputs are comma-separated lists and must be aligned by index.
// If weights is non-nil, it must have the same length as the other lists.
func buildManualL1Validators(nodeIDs, blsPubKeys, blsPoPs string, balance float64, weights []uint64) ([]*txs.ConvertSubnetToL1Validator, error) {
	if strings.TrimSpace(nodeIDs) == "" || strings.TrimSpace(blsPubKeys) == "" || strings.TrimSpace(blsPoPs) == "" {
		return nil, fmt.Errorf("manual validator mode requires --validator-node-ids, --validator-bls-public-keys, and --validator-bls-pops")
	}

	idsList := parseValidatorAddrs(nodeIDs)
	blsList := parseValidatorAddrs(blsPubKeys)
	popList := parseValidatorAddrs(blsPoPs)
	if len(idsList) == 0 {
		return nil, fmt.Errorf("no validator node IDs provided")
	}
	if len(idsList) != len(blsList) || len(idsList) != len(popList) {
		return nil, fmt.Errorf(
			"manual validator lists must have matching lengths (node-ids=%d, bls-public-keys=%d, bls-pops=%d)",
			len(idsList), len(blsList), len(popList),
		)
	}
	if weights != nil && len(weights) != len(idsList) {
		return nil, fmt.Errorf("validator-weights count (%d) must match validator count (%d)", len(weights), len(idsList))
	}

	balanceNAVAX, err := avaxToNAVAX(balance)
	if err != nil {
		return nil, fmt.Errorf("invalid validator balance: %w", err)
	}

	validators := make([]*txs.ConvertSubnetToL1Validator, 0, len(idsList))
	for i := range idsList {
		nodeID, err := ids.NodeIDFromString(idsList[i])
		if err != nil {
			return nil, fmt.Errorf("invalid validator node ID at index %d: %w", i, err)
		}
		pop, err := parseManualPoP(blsList[i], popList[i])
		if err != nil {
			return nil, fmt.Errorf("invalid validator BLS data at index %d: %w", i, err)
		}

		weight := uint64(units.Schmeckle)
		if weights != nil {
			weight = weights[i]
		}

		validators = append(validators, &txs.ConvertSubnetToL1Validator{
			NodeID:  nodeID.Bytes(),
			Weight:  weight,
			Balance: balanceNAVAX,
			Signer:  *pop,
		})
	}

	return validators, nil
}

// sortAndValidateL1Validators sorts validators by NodeID bytes and rejects duplicates.
func sortAndValidateL1Validators(validators []*txs.ConvertSubnetToL1Validator) error {
	sort.Slice(validators, func(i, j int) bool {
		return bytes.Compare(validators[i].NodeID, validators[j].NodeID) < 0
	})
	for i := 1; i < len(validators); i++ {
		if bytes.Equal(validators[i-1].NodeID, validators[i].NodeID) {
			if nodeID, err := ids.ToNodeID(validators[i].NodeID); err == nil {
				return fmt.Errorf("duplicate validator node ID: %s", nodeID)
			}
			return fmt.Errorf("duplicate validator node ID bytes: %x", validators[i].NodeID)
		}
	}
	return nil
}

// parseValidatorAddrs splits a comma-separated list of validator addresses.
func parseValidatorAddrs(addrList string) []string {
	var addrs []string
	for _, addr := range strings.Split(addrList, ",") {
		addr = strings.TrimSpace(addr)
		if addr != "" {
			addrs = append(addrs, addr)
		}
	}
	return addrs
}

// parseValidatorWeights splits a comma-separated list of uint64 weights.
func parseValidatorWeights(weightList string) ([]uint64, error) {
	var weights []uint64
	for _, raw := range strings.Split(weightList, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		w, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid weight %q: %w", raw, err)
		}
		if w == 0 {
			return nil, fmt.Errorf("weight must be greater than 0, got %q", raw)
		}
		weights = append(weights, w)
	}
	return weights, nil
}

func normalizeNodeURI(addr string) (string, error) {
	return nodeutil.NormalizeNodeURIWithInsecureHTTP(addr, allowInsecureHTTP)
}

// generateMockValidator creates a mock validator with valid BLS credentials for testing.
// If weight is 0, units.Schmeckle is used as the default.
func generateMockValidator(balance float64, weight uint64) (*txs.ConvertSubnetToL1Validator, error) {
	// Validate balance to prevent overflow
	balanceNAVAX, err := avaxToNAVAX(balance)
	if err != nil {
		return nil, fmt.Errorf("invalid validator balance: %w", err)
	}

	if weight == 0 {
		weight = units.Schmeckle
	}

	// Generate random NodeID (20 bytes)
	nodeID := make([]byte, ids.NodeIDLen)
	if _, err := rand.Read(nodeID); err != nil {
		return nil, fmt.Errorf("failed to generate node ID: %w", err)
	}

	// Generate BLS signer and proof of possession
	blsSigner, err := localsigner.New()
	if err != nil {
		return nil, fmt.Errorf("failed to generate BLS signer: %w", err)
	}

	pop, err := signer.NewProofOfPossession(blsSigner)
	if err != nil {
		return nil, fmt.Errorf("failed to generate proof of possession: %w", err)
	}

	return &txs.ConvertSubnetToL1Validator{
		NodeID:  nodeID,
		Weight:  weight,
		Balance: balanceNAVAX,
		Signer:  *pop,
	}, nil
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
	subnetConvertL1Cmd.Flags().StringVar(&subnetChainID, "chain-id", "", "Chain ID where the validator manager contract lives (often the L1 chain ID)")
	subnetConvertL1Cmd.Flags().StringVar(&subnetManager, "manager", "", "Validator manager contract address (hex)")
	subnetConvertL1Cmd.Flags().StringVar(&subnetManager, "contract-address", "", "Alias for --manager")
	subnetConvertL1Cmd.Flags().StringVar(&subnetValidatorIPs, "validators", "", "Comma-separated validator node addresses (auto-fetches NodeID + BLS PoP from /ext/info)")
	subnetConvertL1Cmd.Flags().StringVar(&subnetValidatorIDs, "validator-node-ids", "", "Manual mode: comma-separated validator NodeIDs (must align with --validator-bls-public-keys and --validator-bls-pops)")
	subnetConvertL1Cmd.Flags().StringVar(&subnetValidatorBLS, "validator-bls-public-keys", "", "Manual mode: comma-separated validator BLS public keys (hex)")
	subnetConvertL1Cmd.Flags().StringVar(&subnetValidatorPoP, "validator-bls-pops", "", "Manual mode: comma-separated validator BLS proofs of possession (hex)")
	subnetConvertL1Cmd.Flags().Float64Var(&subnetValBalance, "validator-balance", 1.0, "Balance per validator in AVAX")
	subnetConvertL1Cmd.Flags().StringVar(&subnetValidatorWeights, "validator-weights", "", "Comma-separated validator weights (uint64). Must match validator count. Defaults to units.Schmeckle (49463) per validator if omitted.")
	subnetConvertL1Cmd.Flags().BoolVar(&subnetMockVal, "mock-validator", false, "Use a mock validator (for testing)")
}
