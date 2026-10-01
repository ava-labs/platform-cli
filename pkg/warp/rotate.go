package warp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils/crypto/bls"
	"github.com/ava-labs/avalanchego/vms/platformvm/warp/message"
)

var (
	errAnotherTargetRemoved = errors.New("another target is removed and not re-added")
	errExpiryTooSoon        = errors.New("re-add expires too soon")
	errMissingPoP           = errors.New("no bundle has a proof of possession for the target key")
	errOwnerMismatch        = errors.New("re-added validator owner does not match the plan")
)

// Chain is the P-Chain access that [Rotate] needs.
type Chain interface {
	// ValidatorSet returns the validators of subnetID at the P-Chain proposed
	// height, the height the P-Chain verifies new Warp messages against.
	ValidatorSet(ctx context.Context, subnetID ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error)
	// L1Validator returns the L1 validator with validationID from the last
	// accepted state. found is false if the validator does not exist.
	L1Validator(ctx context.Context, validationID ids.ID) (v L1Validator, found bool, err error)
	// SetL1ValidatorWeight issues a SetL1ValidatorWeightTx and returns after
	// it is accepted.
	SetL1ValidatorWeight(ctx context.Context, message []byte) (ids.ID, error)
	// RegisterL1Validator issues a RegisterL1ValidatorTx and returns after it
	// is accepted.
	RegisterL1Validator(ctx context.Context, balance uint64, pop [bls.SignatureLen]byte, message []byte) (ids.ID, error)
}

// L1Validator is the part of an L1 validator that [Rotate] checks.
type L1Validator struct {
	NodeID                ids.NodeID
	Weight                uint64
	RemainingBalanceOwner message.PChainOwner
	DeactivationOwner     message.PChainOwner
}

// RotateConfig is the input of [Rotate].
type RotateConfig struct {
	Chain     Chain
	SubnetID  ids.ID
	Balance   uint64
	Planned   []PlannedTarget
	Collected *Collected
	// Confirm is called before the first transaction of each target. An error
	// stops the rotation. Nil skips the confirmation.
	Confirm func(PlannedTarget) error
	Log     io.Writer
	// Retry bounds the retries of each P-Chain read and transaction after a
	// rate limit or a proposed height lag.
	Retry RetryPolicy
	// PollInterval and WaitTimeout control the wait for the validator set to
	// update after each transaction.
	PollInterval time.Duration
	WaitTimeout  time.Duration
	// ExpiryMargin is the minimum time between now and the re-add expiry for
	// a target to start.
	ExpiryMargin time.Duration
	Now          func() time.Time
}

// TargetResult is the outcome of one target.
type TargetResult struct {
	NodeID            ids.NodeID
	AlreadyRotated    bool
	RemovalTxID       ids.ID
	ReaddTxID         ids.ID
	DeactivationOwner message.PChainOwner
}

type targetState int

const (
	// stateActive: the original validation is active.
	stateActive targetState = iota
	// stateRemoved: the original validation is gone and the re-add is not on
	// chain.
	stateRemoved
	// stateRotated: the re-add is on chain.
	stateRotated
)

// Rotate removes and re-adds each target in order, one target at a time:
//
//  1. aggregate and submit the removal, then wait until the target leaves
//     the validator set
//  2. aggregate and submit the re-add, then wait until the target is back
//  3. check that the re-added validator has the planned owners
//
// Before a target's removal, Rotate checks that the removal and the re-add
// can both reach quorum with the collected signatures, that a proof of
// possession exists, and that the re-add does not expire within
// ExpiryMargin. So it never removes a validator it cannot re-add. It refuses
// to start a target while another target is removed and not re-added.
//
// Rotate resumes: a target that is already rotated is checked and skipped,
// and a removed target is re-added.
func Rotate(ctx context.Context, cfg RotateConfig) ([]TargetResult, error) {
	r := &rotator{cfg: cfg}
	results := make([]TargetResult, 0, len(cfg.Planned))
	for i, t := range cfg.Planned {
		res, err := r.rotateTarget(ctx, i, t)
		if err != nil {
			return results, fmt.Errorf("target %d (%s): %w", i, t.NodeID, err)
		}
		results = append(results, res)
	}
	return results, nil
}

type rotator struct {
	cfg RotateConfig
}

func (r *rotator) logf(format string, args ...any) {
	fmt.Fprintf(r.cfg.Log, format+"\n", args...)
}

func (r *rotator) rotateTarget(ctx context.Context, i int, t PlannedTarget) (TargetResult, error) {
	res := TargetResult{NodeID: t.NodeID}
	state, err := r.state(ctx, t)
	if err != nil {
		return res, err
	}
	if state == stateRotated {
		res.AlreadyRotated = true
		res.DeactivationOwner, err = r.checkOwners(ctx, t)
		if err != nil {
			return res, err
		}
		r.logf("[%d] %s already rotated (validation %s)", i, t.NodeID, t.ReaddValidationID)
		return res, nil
	}

	for j, other := range r.cfg.Planned {
		if j == i {
			continue
		}
		otherState, err := r.state(ctx, other)
		if err != nil {
			return res, err
		}
		if otherState == stateRemoved {
			return res, fmt.Errorf("%w: target %d (%s)", errAnotherTargetRemoved, j, other.NodeID)
		}
	}

	if err := r.checkExpiry(t); err != nil {
		return res, err
	}
	pop, ok := r.cfg.Collected.PoPs[string(t.BLSPublicKey)]
	if !ok {
		return res, fmt.Errorf("%w: 0x%x", errMissingPoP, t.BLSPublicKey)
	}

	vdrSet, err := r.validatorSet(ctx)
	if err != nil {
		return res, err
	}
	readdSet := vdrSet
	var removal *Aggregation
	if state == stateActive {
		removal, err = r.aggregate(t.Removal, vdrSet)
		if err != nil {
			return res, fmt.Errorf("removal: %w", err)
		}
		readdSet = maps.Clone(vdrSet)
		delete(readdSet, t.NodeID)
	}
	// The re-add is aggregated again after the removal. This is the check
	// that it can reach quorum once the target is gone.
	readd, err := r.aggregate(t.Readd, readdSet)
	if err != nil {
		return res, fmt.Errorf("re-add, checked before the removal: %w", err)
	}

	if state == stateActive {
		r.logf("[%d] %s: removal signed by %.2f%% of weight, re-add by %.2f%% of the remaining weight",
			i,
			t.NodeID,
			percent(removal),
			percent(readd),
		)
	} else {
		r.logf("[%d] %s is removed, re-add signed by %.2f%% of weight", i, t.NodeID, percent(readd))
	}
	if r.cfg.Confirm != nil {
		if err := r.cfg.Confirm(t); err != nil {
			return res, err
		}
	}

	if state == stateActive {
		res.RemovalTxID, err = retry(
			ctx,
			r.cfg.Retry,
			r.logf,
			"submit removal",
			func(ctx context.Context) (bool, error) {
				s, err := r.state(ctx, t)
				return s != stateActive, err
			},
			func(ctx context.Context) (ids.ID, error) {
				return r.submitRemoval(ctx, t)
			},
		)
		if err != nil {
			return res, fmt.Errorf("failed to submit removal: %w", err)
		}
		r.logf("[%d] %s: removal accepted: %s", i, t.NodeID, res.RemovalTxID)
		err = r.waitFor(ctx, "the removal to reach the validator set", func(ctx context.Context) (bool, error) {
			s, err := r.state(ctx, t)
			if err != nil || s != stateRemoved {
				return false, err
			}
			return r.inSet(ctx, t, false)
		})
		if err != nil {
			return res, err
		}
	}

	res.ReaddTxID, err = retry(
		ctx,
		r.cfg.Retry,
		r.logf,
		"submit re-add",
		func(ctx context.Context) (bool, error) {
			s, err := r.state(ctx, t)
			return s == stateRotated, err
		},
		func(ctx context.Context) (ids.ID, error) {
			return r.submitReadd(ctx, t, pop)
		},
	)
	if err != nil {
		return res, fmt.Errorf("failed to submit re-add: %w", err)
	}
	r.logf("[%d] %s: re-add accepted: %s", i, t.NodeID, res.ReaddTxID)
	err = r.waitFor(ctx, "the re-add to reach the validator set", func(ctx context.Context) (bool, error) {
		return r.inSet(ctx, t, true)
	})
	if err != nil {
		return res, err
	}

	res.DeactivationOwner, err = r.checkOwners(ctx, t)
	if err != nil {
		return res, err
	}
	r.logf("[%d] %s rotated: validation %s, deactivation owner threshold %d",
		i,
		t.NodeID,
		t.ReaddValidationID,
		res.DeactivationOwner.Threshold,
	)
	return res, nil
}

// submitRemoval aggregates the removal against the current set and submits
// it. Each retry aggregates again, so a retry after a proposed height lag
// uses the set that the P-Chain verifies against.
func (r *rotator) submitRemoval(ctx context.Context, t PlannedTarget) (ids.ID, error) {
	// The submit retry covers this read, so it is not retried on its own.
	vdrSet, err := r.cfg.Chain.ValidatorSet(ctx, r.cfg.SubnetID)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to read validator set: %w", err)
	}
	agg, err := r.aggregate(t.Removal, vdrSet)
	if err != nil {
		return ids.Empty, fmt.Errorf("removal: %w", err)
	}
	return r.cfg.Chain.SetL1ValidatorWeight(ctx, agg.Message.Bytes())
}

// submitReadd aggregates the re-add against the current set and submits it.
func (r *rotator) submitReadd(ctx context.Context, t PlannedTarget, pop [bls.SignatureLen]byte) (ids.ID, error) {
	vdrSet, err := r.cfg.Chain.ValidatorSet(ctx, r.cfg.SubnetID)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to read validator set: %w", err)
	}
	agg, err := r.aggregate(t.Readd, vdrSet)
	if err != nil {
		return ids.Empty, fmt.Errorf("re-add: %w", err)
	}
	return r.cfg.Chain.RegisterL1Validator(ctx, r.cfg.Balance, pop, agg.Message.Bytes())
}

func (r *rotator) validatorSet(ctx context.Context) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
	vdrSet, err := retry(ctx, r.cfg.Retry, r.logf, "read validator set", nil, func(ctx context.Context) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
		return r.cfg.Chain.ValidatorSet(ctx, r.cfg.SubnetID)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to read validator set: %w", err)
	}
	return vdrSet, nil
}

type foundL1Validator struct {
	v     L1Validator
	found bool
}

func (r *rotator) l1Validator(ctx context.Context, validationID ids.ID) (L1Validator, bool, error) {
	res, err := retry(ctx, r.cfg.Retry, r.logf, "read L1 validator", nil, func(ctx context.Context) (foundL1Validator, error) {
		v, found, err := r.cfg.Chain.L1Validator(ctx, validationID)
		return foundL1Validator{v: v, found: found}, err
	})
	return res.v, res.found, err
}

func (r *rotator) state(ctx context.Context, t PlannedTarget) (targetState, error) {
	_, found, err := r.l1Validator(ctx, t.ReaddValidationID)
	if err != nil {
		return 0, fmt.Errorf("failed to read re-added validator: %w", err)
	}
	if found {
		return stateRotated, nil
	}
	// The P-Chain deletes a validator when its weight is set to 0.
	_, found, err = r.l1Validator(ctx, t.ValidationID)
	if err != nil {
		return 0, fmt.Errorf("failed to read validator: %w", err)
	}
	if found {
		return stateActive, nil
	}
	return stateRemoved, nil
}

// inSet reports whether the target's presence in the validator set at the
// proposed height equals want. A present target must have its planned
// weight.
func (r *rotator) inSet(ctx context.Context, t PlannedTarget, want bool) (bool, error) {
	vdrSet, err := r.validatorSet(ctx)
	if err != nil {
		return false, err
	}
	vdr, ok := vdrSet[t.NodeID]
	if !want {
		return !ok, nil
	}
	return ok && vdr.Weight == t.Weight, nil
}

func (r *rotator) checkExpiry(t PlannedTarget) error {
	deadline := r.cfg.Now().Add(r.cfg.ExpiryMargin)
	expiry := time.Unix(int64(t.ReaddPayload.Expiry), 0)
	if !expiry.After(deadline) {
		return fmt.Errorf("%w: expiry %s, need after %s", errExpiryTooSoon, expiry.UTC().Format(time.RFC3339), deadline.UTC().Format(time.RFC3339))
	}
	return nil
}

func (r *rotator) aggregate(d *Decoded, vdrSet map[ids.NodeID]*validators.GetValidatorOutput) (*Aggregation, error) {
	vdrs, err := validators.FlattenValidatorSet(vdrSet)
	if err != nil {
		return nil, fmt.Errorf("failed to build canonical validator set: %w", err)
	}
	sigs := SelectSigners(vdrs, r.cfg.Collected.Signatures[d.Message.ID()])
	return Aggregate(d.Message, vdrs, sigs)
}

func (r *rotator) checkOwners(ctx context.Context, t PlannedTarget) (message.PChainOwner, error) {
	v, found, err := r.l1Validator(ctx, t.ReaddValidationID)
	if err != nil {
		return message.PChainOwner{}, fmt.Errorf("failed to read re-added validator: %w", err)
	}
	if !found {
		return message.PChainOwner{}, fmt.Errorf("%w: validation %s not found", errOwnerMismatch, t.ReaddValidationID)
	}
	want := t.ReaddPayload
	if !equalOwners(v.DeactivationOwner, want.DisableOwner) {
		return message.PChainOwner{}, fmt.Errorf("%w: deactivation owner %+v, want %+v", errOwnerMismatch, v.DeactivationOwner, want.DisableOwner)
	}
	if !equalOwners(v.RemainingBalanceOwner, want.RemainingBalanceOwner) {
		return message.PChainOwner{}, fmt.Errorf("%w: remaining balance owner %+v, want %+v", errOwnerMismatch, v.RemainingBalanceOwner, want.RemainingBalanceOwner)
	}
	return v.DeactivationOwner, nil
}

func equalOwners(a, b message.PChainOwner) bool {
	return a.Threshold == b.Threshold && slices.Equal(a.Addresses, b.Addresses)
}

func (r *rotator) waitFor(ctx context.Context, what string, done func(context.Context) (bool, error)) error {
	ctx, cancel := context.WithTimeout(ctx, r.cfg.WaitTimeout)
	defer cancel()
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()
	for {
		ok, err := done(ctx)
		if err != nil && !isTransient(err) {
			return fmt.Errorf("failed waiting for %s: %w", what, err)
		}
		if err != nil {
			r.logf("waiting for %s: transient error, polling again: %v", what, err)
		}
		if err == nil && ok {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("failed waiting for %s: %w", what, ctx.Err())
		case <-ticker.C:
		}
	}
}

func percent(a *Aggregation) float64 {
	return 100 * float64(a.SignedWeight) / float64(a.TotalWeight)
}
