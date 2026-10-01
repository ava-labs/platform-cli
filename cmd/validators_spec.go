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
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/ava-labs/avalanchego/api/info"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto/bls"
	"github.com/ava-labs/avalanchego/utils/crypto/bls/signer/localsigner"
	"github.com/ava-labs/avalanchego/utils/formatting/address"
	"github.com/ava-labs/avalanchego/vms/platformvm/signer"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/avalanchego/vms/platformvm/warp/message"
	nodeutil "github.com/ava-labs/platform-cli/pkg/node"
	"github.com/ava-labs/platform-cli/pkg/wallet"
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

var (
	errNoOwnerAddresses    = errors.New("no owner addresses provided")
	errOwnerCountMismatch  = errors.New("owner count must be 1 or match validator count")
	errNotPChainAddress    = errors.New("not a P-Chain address")
	errWrongNetworkAddress = errors.New("address is for a different network")
	errZeroOwnerAddress    = errors.New("owner address is the zero address")
	errEmptyValidatorOwner = errors.New("validator owner is empty")
)

// parseValidatorOwners parses a comma-separated list of P-Chain addresses for
// the network with the given HRP. It returns one owner per validator: a single
// address applies to every validator, otherwise the list must align with the
// validators by index. An unset list returns nil.
func parseValidatorOwners(list, hrp string, numValidators int) ([]ids.ShortID, error) {
	if strings.TrimSpace(list) == "" {
		return nil, nil
	}
	addrs := parseValidatorAddrs(list)
	if len(addrs) == 0 {
		return nil, errNoOwnerAddresses
	}
	if len(addrs) != 1 && len(addrs) != numValidators {
		return nil, fmt.Errorf("%w: got %d, validators %d", errOwnerCountMismatch, len(addrs), numValidators)
	}

	owners := make([]ids.ShortID, len(addrs))
	for i, addr := range addrs {
		owner, err := parsePChainAddress(addr, hrp)
		if err != nil {
			return nil, fmt.Errorf("invalid owner address %q: %w", addr, err)
		}
		// validatorOwner treats the zero address as unset.
		if owner == ids.ShortEmpty {
			return nil, fmt.Errorf("%w: %q", errZeroOwnerAddress, addr)
		}
		owners[i] = owner
	}
	if len(owners) == 1 {
		return slices.Repeat(owners, numValidators), nil
	}
	return owners, nil
}

// parsePChainAddress parses a bech32 P-Chain address ("P-<hrp>1...") and
// rejects addresses for other chains or networks.
func parsePChainAddress(addr, hrp string) (ids.ShortID, error) {
	chain, addrHRP, addrBytes, err := address.Parse(addr)
	if err != nil {
		return ids.ShortEmpty, err
	}
	if chain != "P" {
		return ids.ShortEmpty, fmt.Errorf("%w: chain %q", errNotPChainAddress, chain)
	}
	if addrHRP != hrp {
		return ids.ShortEmpty, fmt.Errorf("%w: got HRP %q, want %q", errWrongNetworkAddress, addrHRP, hrp)
	}
	return ids.ToShortID(addrBytes)
}

// setL1ValidatorOwners sets the remaining balance owner and the deactivation
// owner of each validator. remainingBalanceOwners and deactivationOwners are
// either nil or aligned with validators by index.
//
// A nil list defaults to defaultOwner with threshold 1. If allowEmpty is set, a
// nil list leaves the owner empty instead. An empty owner has threshold 0, so
// any P-Chain key can disable the validator and spend its remaining balance.
// Unless allowEmpty is set, an empty owner returns errEmptyValidatorOwner.
func setL1ValidatorOwners(
	validators []*txs.ConvertSubnetToL1Validator,
	remainingBalanceOwners []ids.ShortID,
	deactivationOwners []ids.ShortID,
	defaultOwner ids.ShortID,
	allowEmpty bool,
) error {
	if allowEmpty {
		defaultOwner = ids.ShortEmpty
	}
	for i, v := range validators {
		v.RemainingBalanceOwner = validatorOwner(remainingBalanceOwners, i, defaultOwner)
		v.DeactivationOwner = validatorOwner(deactivationOwners, i, defaultOwner)
		if allowEmpty {
			continue
		}
		if v.RemainingBalanceOwner.Threshold == 0 {
			return fmt.Errorf("%w: remaining balance owner of validator %d", errEmptyValidatorOwner, i)
		}
		if v.DeactivationOwner.Threshold == 0 {
			return fmt.Errorf("%w: deactivation owner of validator %d", errEmptyValidatorOwner, i)
		}
	}
	return nil
}

// finalizeL1Validators sets the validator owners with setL1ValidatorOwners and
// then sorts the validators with sortAndValidateL1Validators. The owner lists
// align with validators in input order, so the owners must be set before the
// sort.
func finalizeL1Validators(
	validators []*txs.ConvertSubnetToL1Validator,
	remainingBalanceOwners []ids.ShortID,
	deactivationOwners []ids.ShortID,
	defaultOwner ids.ShortID,
	allowEmpty bool,
) error {
	err := setL1ValidatorOwners(
		validators,
		remainingBalanceOwners,
		deactivationOwners,
		defaultOwner,
		allowEmpty,
	)
	if err != nil {
		return err
	}
	return sortAndValidateL1Validators(validators)
}

// validatorOwner returns owners[i], or defaultOwner if owners is nil, as a
// threshold 1 owner. An empty address returns the empty owner.
func validatorOwner(owners []ids.ShortID, i int, defaultOwner ids.ShortID) message.PChainOwner {
	owner := defaultOwner
	if owners != nil {
		owner = owners[i]
	}
	if owner == ids.ShortEmpty {
		return message.PChainOwner{}
	}
	return message.PChainOwner{
		Threshold: 1,
		Addresses: []ids.ShortID{owner},
	}
}

// formatPChainOwner returns a readable form of owner for the given network.
func formatPChainOwner(owner message.PChainOwner, networkID uint32) string {
	if owner.Threshold == 0 {
		return "EMPTY (threshold 0: any P-Chain key can use it)"
	}
	addrs := make([]string, len(owner.Addresses))
	for i, addr := range owner.Addresses {
		addrs[i] = wallet.FormatPChainAddress(addr, networkID)
	}
	return fmt.Sprintf("threshold %d of [%s]", owner.Threshold, strings.Join(addrs, ", "))
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
