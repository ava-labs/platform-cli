package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ava-labs/avalanchego/api/info"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto/bls/signer/localsigner"
	"github.com/ava-labs/avalanchego/utils/units"
	"github.com/ava-labs/avalanchego/vms/platformvm/signer"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/platform-cli/pkg/pchain"
	"github.com/spf13/cobra"
)

var (
	subnetID           string
	subnetNewOwner     string
	subnetChainID      string
	subnetManager      string
	subnetValidatorIPs string
	subnetValBalance   float64
	subnetMockVal      bool
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

		// Gather validator info from IPs or generate mock
		var validators []*txs.ConvertSubnetToL1Validator
		if subnetMockVal {
			// Generate a mock validator with valid BLS credentials for testing
			mockVal, err := generateMockValidator(subnetValBalance)
			if err != nil {
				return fmt.Errorf("failed to generate mock validator: %w", err)
			}
			validators = []*txs.ConvertSubnetToL1Validator{mockVal}
			fmt.Printf("Using mock validator (NodeID: %x)\n", mockVal.NodeID)
		} else {
			validators, err = gatherL1Validators(ctx, subnetValidatorIPs, subnetValBalance)
			if err != nil {
				return err
			}
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

		txID, err := pchain.ConvertSubnetToL1(ctx, w, sid, cid, managerAddr, validators)
		if err != nil {
			return err
		}

		fmt.Printf("Convert Subnet to L1 TX: %s\n", txID)
		return nil
	},
}

// gatherL1Validators queries validator nodes and builds conversion validators.
func gatherL1Validators(ctx context.Context, validatorAddrs string, balance float64) ([]*txs.ConvertSubnetToL1Validator, error) {
	addrs := parseValidatorAddrs(validatorAddrs)
	if len(addrs) == 0 {
		return nil, nil // No validators is valid
	}

	// Validate balance to prevent overflow
	balanceNAVAX, err := avaxToNAVAX(balance)
	if err != nil {
		return nil, fmt.Errorf("invalid validator balance: %w", err)
	}

	validators := make([]*txs.ConvertSubnetToL1Validator, 0, len(addrs))

	for _, addr := range addrs {
		uri := normalizeNodeURI(addr)
		infoClient := info.NewClient(uri)

		nodeID, nodePoP, err := infoClient.GetNodeID(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get node info from %s: %w", uri, err)
		}

		validators = append(validators, &txs.ConvertSubnetToL1Validator{
			NodeID:  nodeID.Bytes(),
			Weight:  units.Schmeckle,
			Balance: balanceNAVAX,
			Signer:  *nodePoP,
		})
	}

	return validators, nil
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

// normalizeNodeURI converts a node address to a full URI.
// Accepts: "127.0.0.1", "127.0.0.1:9650", "http://127.0.0.1:9650"
func normalizeNodeURI(addr string) string {
	// Already a full URI
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	// Has port but no scheme
	if strings.Contains(addr, ":") {
		return "http://" + addr
	}
	// Just IP/hostname, add default port
	return fmt.Sprintf("http://%s:9650", addr)
}

// generateMockValidator creates a mock validator with valid BLS credentials for testing.
func generateMockValidator(balance float64) (*txs.ConvertSubnetToL1Validator, error) {
	// Validate balance to prevent overflow
	balanceNAVAX, err := avaxToNAVAX(balance)
	if err != nil {
		return nil, fmt.Errorf("invalid validator balance: %w", err)
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
		Weight:  units.Schmeckle,
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
	subnetConvertL1Cmd.Flags().StringVar(&subnetChainID, "chain-id", "", "Chain ID for the L1")
	subnetConvertL1Cmd.Flags().StringVar(&subnetManager, "manager", "", "Validator manager address (hex)")
	subnetConvertL1Cmd.Flags().StringVar(&subnetValidatorIPs, "validators", "", "Comma-separated validator IPs")
	subnetConvertL1Cmd.Flags().Float64Var(&subnetValBalance, "validator-balance", 1.0, "Balance per validator in AVAX")
	subnetConvertL1Cmd.Flags().BoolVar(&subnetMockVal, "mock-validator", false, "Use a mock validator (for testing)")
}
