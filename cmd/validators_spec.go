package cmd

// Shared validator-spec parsing and building helpers used by the subnet
// convert-l1 and primary-network validator commands. These functions parse
// CLI-provided validator data (addresses, weights, BLS credentials) and build
// L1 conversion validators. Keep them free of command/flag state so they stay
// independently testable.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ava-labs/avalanchego/api/info"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto/bls"
	"github.com/ava-labs/avalanchego/utils/crypto/bls/signer/localsigner"
	"github.com/ava-labs/avalanchego/vms/platformvm/signer"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	nodeutil "github.com/ava-labs/platform-cli/pkg/node"
)

const defaultValidatorWeight uint64 = 100

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

		weight := uint64(defaultValidatorWeight)
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

		weight := uint64(defaultValidatorWeight)
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

// parseManualPoP parses and verifies a BLS public key and proof of possession
// provided as hex strings (optional 0x/0X prefix).
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

func normalizeNodeURI(addr string) (string, error) {
	return nodeutil.NormalizeNodeURIWithInsecureHTTP(addr, allowInsecureHTTP)
}

// generateMockValidator creates a mock validator with valid BLS credentials for testing.
// If weight is 0, defaultValidatorWeight (100) is used as the default.
func generateMockValidator(balance float64, weight uint64) (*txs.ConvertSubnetToL1Validator, error) {
	// Validate balance to prevent overflow
	balanceNAVAX, err := avaxToNAVAX(balance)
	if err != nil {
		return nil, fmt.Errorf("invalid validator balance: %w", err)
	}

	if weight == 0 {
		weight = defaultValidatorWeight
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
