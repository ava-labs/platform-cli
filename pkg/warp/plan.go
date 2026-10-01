package warp

import (
	"bytes"
	"errors"
	"fmt"
	"math"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils/crypto/bls"
	"github.com/ava-labs/avalanchego/utils/set"
	"github.com/ava-labs/avalanchego/vms/platformvm/warp/message"
	"github.com/ava-labs/avalanchego/vms/types"
)

// PlanVersion is the version of the plan file format.
const PlanVersion = 1

// RemovalNonce is the nonce of every removal message. The P-Chain accepts any
// nonce at or above the validator's minimum nonce, and reserves MaxUint64 for
// weight 0, so a removal built before signing never goes stale.
const RemovalNonce = math.MaxUint64

var (
	errNoTargets       = errors.New("no targets")
	errDuplicateTarget = errors.New("duplicate target")
	errEmptyOwner      = errors.New("owner is empty")
	errPlanVersion     = errors.New("unsupported plan version")
	errPlanMismatch    = errors.New("plan message does not match its target")
)

// Plan is the full set of messages that rotates the targets of one L1: one
// removal and one re-add per target, in submission order.
type Plan struct {
	Version             uint32              `json:"version"`
	NetworkID           uint32              `json:"networkID"`
	SubnetID            ids.ID              `json:"subnetID"`
	ManagerBlockchainID ids.ID              `json:"managerBlockchainID"`
	ManagerAddress      types.JSONByteSlice `json:"managerAddress"`
	// Balance is the nAVAX balance of each re-added validator.
	Balance uint64 `json:"balance"`
	// Expiry is the Unix time after which the P-Chain rejects the re-adds.
	Expiry uint64 `json:"expiry"`
	// SnapshotHeight and Snapshot are the canonical validator set when the
	// plan was built, for review. Rotation reads the live set.
	SnapshotHeight uint64             `json:"snapshotHeight"`
	Snapshot       validators.WarpSet `json:"snapshot"`
	Targets        []Target           `json:"targets"`
}

// Target is one validator to rotate.
type Target struct {
	ValidationID ids.ID              `json:"validationID"`
	NodeID       ids.NodeID          `json:"nodeID"`
	BLSPublicKey types.JSONByteSlice `json:"blsPublicKey"`
	Weight       uint64              `json:"weight"`
	// Removal is the unsigned L1ValidatorWeight message that sets the weight
	// to 0.
	Removal types.JSONByteSlice `json:"removal"`
	// Readd is the unsigned RegisterL1Validator message that re-adds the same
	// node ID and BLS key with the new owners.
	Readd types.JSONByteSlice `json:"readd"`
	// ReaddValidationID is the validation ID that Readd creates.
	ReaddValidationID ids.ID `json:"readdValidationID"`
}

// PlanConfig is the input of [NewPlan].
type PlanConfig struct {
	NetworkID             uint32
	SubnetID              ids.ID
	ManagerBlockchainID   ids.ID
	ManagerAddress        []byte
	RemainingBalanceOwner message.PChainOwner
	DeactivationOwner     message.PChainOwner
	Balance               uint64
	Expiry                uint64
	SnapshotHeight        uint64
	Snapshot              validators.WarpSet
}

// TargetValidator is a validator that [NewPlan] rotates.
type TargetValidator struct {
	ValidationID ids.ID
	NodeID       ids.NodeID
	PublicKey    *bls.PublicKey
	Weight       uint64
}

// NewPlan builds the removal and re-add messages of every target. Every
// Both owners must be non-empty.
func NewPlan(cfg PlanConfig, targets []TargetValidator) (*Plan, error) {
	if len(targets) == 0 {
		return nil, errNoTargets
	}
	if cfg.RemainingBalanceOwner.Threshold == 0 {
		return nil, fmt.Errorf("%w: remaining balance owner", errEmptyOwner)
	}
	if cfg.DeactivationOwner.Threshold == 0 {
		return nil, fmt.Errorf("%w: deactivation owner", errEmptyOwner)
	}

	p := &Plan{
		Version:             PlanVersion,
		NetworkID:           cfg.NetworkID,
		SubnetID:            cfg.SubnetID,
		ManagerBlockchainID: cfg.ManagerBlockchainID,
		ManagerAddress:      cfg.ManagerAddress,
		Balance:             cfg.Balance,
		Expiry:              cfg.Expiry,
		SnapshotHeight:      cfg.SnapshotHeight,
		Snapshot:            cfg.Snapshot,
		Targets:             make([]Target, 0, len(targets)),
	}
	seen := set.NewSet[ids.ID](len(targets))
	for _, t := range targets {
		if seen.Contains(t.ValidationID) {
			return nil, fmt.Errorf("%w: %s", errDuplicateTarget, t.ValidationID)
		}
		seen.Add(t.ValidationID)

		removal, err := message.NewL1ValidatorWeight(t.ValidationID, RemovalNonce, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to build removal of %s: %w", t.NodeID, err)
		}
		removalMsg, err := NewUnsignedMessage(cfg.NetworkID, cfg.ManagerBlockchainID, cfg.ManagerAddress, removal)
		if err != nil {
			return nil, fmt.Errorf("failed to build removal of %s: %w", t.NodeID, err)
		}

		var pk [bls.PublicKeyLen]byte
		copy(pk[:], bls.PublicKeyToCompressedBytes(t.PublicKey))
		readd, err := message.NewRegisterL1Validator(
			cfg.SubnetID,
			t.NodeID,
			pk,
			cfg.Expiry,
			cfg.RemainingBalanceOwner,
			cfg.DeactivationOwner,
			t.Weight,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to build re-add of %s: %w", t.NodeID, err)
		}
		readdMsg, err := NewUnsignedMessage(cfg.NetworkID, cfg.ManagerBlockchainID, cfg.ManagerAddress, readd)
		if err != nil {
			return nil, fmt.Errorf("failed to build re-add of %s: %w", t.NodeID, err)
		}

		p.Targets = append(p.Targets, Target{
			ValidationID:      t.ValidationID,
			NodeID:            t.NodeID,
			BLSPublicKey:      pk[:],
			Weight:            t.Weight,
			Removal:           removalMsg.Bytes(),
			Readd:             readdMsg.Bytes(),
			ReaddValidationID: readd.ValidationID(),
		})
	}
	return p, nil
}

// PlannedTarget is a [Target] with its messages decoded.
type PlannedTarget struct {
	Target
	PublicKey *bls.PublicKey
	Removal   *Decoded
	Readd     *Decoded
	// ReaddPayload is Readd.Payload.
	ReaddPayload *message.RegisterL1Validator
}

// Decode decodes every message of p and checks that each one matches its
// target and the plan. Signers and the coordinator both run it, so a plan
// file that was edited by hand is rejected before anyone signs or submits.
func (p *Plan) Decode() ([]PlannedTarget, error) {
	if p.Version != PlanVersion {
		return nil, fmt.Errorf("%w: %d", errPlanVersion, p.Version)
	}
	if len(p.Targets) == 0 {
		return nil, errNoTargets
	}

	planned := make([]PlannedTarget, len(p.Targets))
	seen := set.NewSet[ids.ID](2 * len(p.Targets))
	for i, t := range p.Targets {
		pt, err := p.decodeTarget(t)
		if err != nil {
			return nil, fmt.Errorf("target %d (%s): %w", i, t.NodeID, err)
		}
		for _, id := range []ids.ID{t.ValidationID, pt.Removal.Message.ID(), pt.Readd.Message.ID()} {
			if seen.Contains(id) {
				return nil, fmt.Errorf("target %d (%s): %w: %s", i, t.NodeID, errDuplicateTarget, id)
			}
			seen.Add(id)
		}
		planned[i] = pt
	}
	return planned, nil
}

func (p *Plan) decodeTarget(t Target) (PlannedTarget, error) {
	pk, err := bls.PublicKeyFromCompressedBytes(t.BLSPublicKey)
	if err != nil {
		return PlannedTarget{}, fmt.Errorf("invalid BLS public key: %w", err)
	}
	removal, err := Decode(t.Removal)
	if err != nil {
		return PlannedTarget{}, fmt.Errorf("invalid removal: %w", err)
	}
	readd, err := Decode(t.Readd)
	if err != nil {
		return PlannedTarget{}, fmt.Errorf("invalid re-add: %w", err)
	}
	for _, d := range []*Decoded{removal, readd} {
		if d.Message.NetworkID != p.NetworkID ||
			d.Message.SourceChainID != p.ManagerBlockchainID ||
			!bytes.Equal(d.SourceAddress, p.ManagerAddress) {
			return PlannedTarget{}, fmt.Errorf("%w: source is %d/%s/0x%x", errPlanMismatch, d.Message.NetworkID, d.Message.SourceChainID, d.SourceAddress)
		}
	}

	weight, ok := removal.Payload.(*message.L1ValidatorWeight)
	if !ok || weight.ValidationID != t.ValidationID || weight.Nonce != RemovalNonce || weight.Weight != 0 {
		return PlannedTarget{}, fmt.Errorf("%w: removal", errPlanMismatch)
	}
	register, ok := readd.Payload.(*message.RegisterL1Validator)
	if !ok ||
		register.SubnetID != p.SubnetID ||
		!bytes.Equal(register.NodeID, t.NodeID[:]) ||
		!bytes.Equal(register.BLSPublicKey[:], t.BLSPublicKey) ||
		register.Weight != t.Weight ||
		register.Expiry != p.Expiry ||
		register.ValidationID() != t.ReaddValidationID {
		return PlannedTarget{}, fmt.Errorf("%w: re-add", errPlanMismatch)
	}
	return PlannedTarget{
		Target:       t,
		PublicKey:    pk,
		Removal:      removal,
		Readd:        readd,
		ReaddPayload: register,
	}, nil
}

// MessageIDs returns the ID of every message in planned, in submission
// order.
func MessageIDs(planned []PlannedTarget) []ids.ID {
	msgIDs := make([]ids.ID, 0, 2*len(planned))
	for _, t := range planned {
		msgIDs = append(msgIDs, t.Removal.Message.ID(), t.Readd.Message.ID())
	}
	return msgIDs
}
