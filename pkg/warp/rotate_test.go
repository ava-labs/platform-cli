package warp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils/crypto/bls"
	"github.com/ava-labs/avalanchego/vms/platformvm/warp/message"
	"github.com/ava-labs/avalanchego/vms/platformvm/warp/payload"

	avawarp "github.com/ava-labs/avalanchego/vms/platformvm/warp"
)

var errFakeChain = errors.New("fake P-Chain rejected the transaction")

type fakeValidator struct {
	L1Validator
	publicKey *bls.PublicKey
}

// fakeChain is an in-memory P-Chain. After each transaction, the epoch set
// lags the accepted state for lag reads of ValidatorSet. Like the P-Chain
// after Granite, it verifies each Warp message against the epoch set, not
// the accepted state.
type fakeChain struct {
	now time.Time
	lag int
	l1  map[ids.ID]*fakeValidator
	txs []string
	// emptyOwners makes RegisterL1Validator ignore the message owners.
	emptyOwners bool
	// fail holds the errors that the next calls of a method return, by
	// method name. failAfterApply holds errors that a transaction returns
	// after it takes effect, as when the confirmation is rate limited.
	fail           map[string][]error
	failAfterApply map[string][]error
	calls          map[string]int
	// watch is the node IDs of the plan targets. maxMissing is the largest
	// number of them that were out of the set at the same time.
	watch      []ids.NodeID
	maxMissing int

	stale     map[ids.NodeID]*validators.GetValidatorOutput
	staleLeft int
}

// injected counts a call of method and returns its next error from m.
func (f *fakeChain) injected(m map[string][]error, method string) error {
	if m == nil || len(m[method]) == 0 {
		return nil
	}
	err := m[method][0]
	m[method] = m[method][1:]
	return err
}

func (f *fakeChain) count(method string) {
	if f.calls == nil {
		f.calls = make(map[string]int)
	}
	f.calls[method]++
}

func (f *fakeChain) accepted() map[ids.NodeID]*validators.GetValidatorOutput {
	vdrs := make(map[ids.NodeID]*validators.GetValidatorOutput)
	for _, v := range f.l1 {
		if v.Weight == 0 {
			continue
		}
		vdrs[v.NodeID] = &validators.GetValidatorOutput{
			NodeID:    v.NodeID,
			PublicKey: v.publicKey,
			Weight:    v.Weight,
		}
	}
	return vdrs
}

func (f *fakeChain) proposed() map[ids.NodeID]*validators.GetValidatorOutput {
	if f.staleLeft > 0 {
		return maps.Clone(f.stale)
	}
	return f.accepted()
}

func (f *fakeChain) ValidatorSet(context.Context, ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
	f.count("ValidatorSet")
	if err := f.injected(f.fail, "ValidatorSet"); err != nil {
		return nil, err
	}
	vdrs := f.proposed()
	if f.staleLeft > 0 {
		f.staleLeft--
	}
	return vdrs, nil
}

func (f *fakeChain) L1Validator(_ context.Context, validationID ids.ID) (L1Validator, bool, error) {
	f.count("L1Validator")
	if err := f.injected(f.fail, "L1Validator"); err != nil {
		return L1Validator{}, false, err
	}
	v, ok := f.l1[validationID]
	if !ok {
		return L1Validator{}, false, nil
	}
	return v.L1Validator, true, nil
}

func (f *fakeChain) verify(msgBytes []byte) (message.Payload, error) {
	msg, err := avawarp.ParseMessage(msgBytes)
	if err != nil {
		return nil, err
	}
	vdrs, err := validators.FlattenValidatorSet(f.proposed())
	if err != nil {
		return nil, err
	}
	err = msg.Signature.Verify(&msg.UnsignedMessage, testNetworkID, vdrs, QuorumNumerator, QuorumDenominator)
	if err != nil {
		// The P-Chain error text, which Rotate retries.
		return nil, fmt.Errorf("failed verifying warp messages: %w", err)
	}
	call, err := payload.ParseAddressedCall(msg.Payload)
	if err != nil {
		return nil, err
	}
	if msg.SourceChainID != testChainID || !bytes.Equal(call.SourceAddress, testManager) {
		return nil, fmt.Errorf("%w: wrong source", errFakeChain)
	}
	return message.Parse(call.Payload)
}

func (f *fakeChain) apply() {
	f.stale = f.proposed()
	f.staleLeft = f.lag
	vdrs := f.accepted()
	missing := 0
	for _, nodeID := range f.watch {
		if _, ok := vdrs[nodeID]; !ok {
			missing++
		}
	}
	f.maxMissing = max(f.maxMissing, missing)
}

func (f *fakeChain) SetL1ValidatorWeight(_ context.Context, msgBytes []byte) (ids.ID, error) {
	f.count("SetL1ValidatorWeight")
	if err := f.injected(f.fail, "SetL1ValidatorWeight"); err != nil {
		return ids.Empty, err
	}
	p, err := f.verify(msgBytes)
	if err != nil {
		return ids.Empty, err
	}
	w := p.(*message.L1ValidatorWeight)
	v, ok := f.l1[w.ValidationID]
	if !ok {
		return ids.Empty, fmt.Errorf("%w: unknown validation %s", errFakeChain, w.ValidationID)
	}
	f.apply()
	if w.Weight == 0 {
		delete(f.l1, w.ValidationID)
	} else {
		v.Weight = w.Weight
	}
	f.txs = append(f.txs, "remove "+v.NodeID.String())
	if err := f.injected(f.failAfterApply, "SetL1ValidatorWeight"); err != nil {
		return ids.Empty, err
	}
	return ids.GenerateTestID(), nil
}

func (f *fakeChain) RegisterL1Validator(_ context.Context, _ uint64, pop [bls.SignatureLen]byte, msgBytes []byte) (ids.ID, error) {
	f.count("RegisterL1Validator")
	if err := f.injected(f.fail, "RegisterL1Validator"); err != nil {
		return ids.Empty, err
	}
	p, err := f.verify(msgBytes)
	if err != nil {
		return ids.Empty, err
	}
	r := p.(*message.RegisterL1Validator)
	if r.Expiry <= uint64(f.now.Unix()) {
		return ids.Empty, fmt.Errorf("%w: expired", errFakeChain)
	}
	pk, err := bls.PublicKeyFromCompressedBytes(r.BLSPublicKey[:])
	if err != nil {
		return ids.Empty, err
	}
	sig, err := bls.SignatureFromBytes(pop[:])
	if err != nil {
		return ids.Empty, err
	}
	if !bls.VerifyProofOfPossession(pk, sig, r.BLSPublicKey[:]) {
		return ids.Empty, fmt.Errorf("%w: invalid proof of possession", errFakeChain)
	}
	nodeID, err := ids.ToNodeID(r.NodeID)
	if err != nil {
		return ids.Empty, err
	}
	if _, ok := f.accepted()[nodeID]; ok {
		return ids.Empty, fmt.Errorf("%w: %s is already a validator", errFakeChain, nodeID)
	}
	f.apply()
	v := &fakeValidator{
		L1Validator: L1Validator{
			NodeID:                nodeID,
			Weight:                r.Weight,
			RemainingBalanceOwner: r.RemainingBalanceOwner,
			DeactivationOwner:     r.DisableOwner,
		},
		publicKey: pk,
	}
	if f.emptyOwners {
		v.RemainingBalanceOwner = message.PChainOwner{}
		v.DeactivationOwner = message.PChainOwner{}
	}
	f.l1[r.ValidationID()] = v
	f.txs = append(f.txs, "add "+nodeID.String())
	if err := f.injected(f.failAfterApply, "RegisterL1Validator"); err != nil {
		return ids.Empty, err
	}
	return ids.GenerateTestID(), nil
}

type rotationTest struct {
	planned []PlannedTarget
	vdrs    validators.WarpSet
	signers []bls.Signer
	chain   *fakeChain
}

// newRotationTest returns validators with the given unique weights, all with
// empty owners, and a plan that rotates the validators with targetWeights.
func newRotationTest(t *testing.T, weights []uint64, targetWeights ...uint64) *rotationTest {
	t.Helper()
	p, planned, vdrs, localSigners := newTestPlan(t, weights, targetWeights...)
	chain := &fakeChain{
		now: time.Unix(testExpiry, 0).Add(-23 * time.Hour),
		lag: 2,
		l1:  make(map[ids.ID]*fakeValidator),
	}
	targetByNode := make(map[ids.NodeID]ids.ID)
	for _, pt := range p.Targets {
		targetByNode[pt.NodeID] = pt.ValidationID
		chain.watch = append(chain.watch, pt.NodeID)
	}
	signers := make([]bls.Signer, len(localSigners))
	for i, vdr := range vdrs.Validators {
		signers[i] = localSigners[i]
		validationID, ok := targetByNode[vdr.NodeIDs[0]]
		if !ok {
			validationID = ids.GenerateTestID()
		}
		chain.l1[validationID] = &fakeValidator{
			L1Validator: L1Validator{
				NodeID: vdr.NodeIDs[0],
				Weight: vdr.Weight,
			},
			publicKey: vdr.PublicKey,
		}
	}
	return &rotationTest{
		planned: planned,
		vdrs:    vdrs,
		signers: signers,
		chain:   chain,
	}
}

// rotate runs Rotate with a bundle from each validator with signerWeights.
func (rt *rotationTest) rotate(t *testing.T, signerWeights ...uint64) ([]TargetResult, error) {
	t.Helper()
	signers := make([]bls.Signer, len(signerWeights))
	for i, w := range signerWeights {
		signers[i] = rt.signers[indexByWeight(rt.vdrs, w)]
	}
	// One bundle per signer, as with one validator per machine.
	bundles := make([]*Bundle, len(signers))
	for i, s := range signers {
		b, err := SignPlan(rt.planned, []bls.Signer{s})
		if err != nil {
			t.Fatalf("SignPlan() error = %v", err)
		}
		bundles[i] = b
	}
	c, err := Collect(rt.planned, bundles)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	return Rotate(t.Context(), RotateConfig{
		Chain:        rt.chain,
		SubnetID:     testSubnetID,
		Balance:      1_000_000_000,
		Planned:      rt.planned,
		Collected:    c,
		Log:          io.Discard,
		Retry:        testRetryPolicy,
		PollInterval: time.Millisecond,
		WaitTimeout:  time.Second,
		ExpiryMargin: time.Hour,
		Now:          func() time.Time { return rt.chain.now },
	})
}

func TestRotate(t *testing.T) {
	// Ten validators with about 10% of the weight each, as on Beam.
	weights := []uint64{100, 101, 102, 103, 104, 105, 106, 107, 108, 109}
	rt := newRotationTest(t, weights, 100, 104, 109)

	results, err := rt.rotate(t, weights...)
	if err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}

	var wantTxs []string
	for _, pt := range rt.planned {
		wantTxs = append(wantTxs, "remove "+pt.NodeID.String(), "add "+pt.NodeID.String())
	}
	if !slices.Equal(rt.chain.txs, wantTxs) {
		t.Errorf("transactions = %v, want %v", rt.chain.txs, wantTxs)
	}
	if rt.chain.maxMissing != 1 {
		t.Errorf("max targets out of the set at once = %d, want 1", rt.chain.maxMissing)
	}
	if len(results) != len(rt.planned) {
		t.Fatalf("len(results) = %d, want %d", len(results), len(rt.planned))
	}
	for i, pt := range rt.planned {
		v, found := rt.chain.l1[pt.ReaddValidationID]
		if !found {
			t.Fatalf("target %d: re-added validation %s not found", i, pt.ReaddValidationID)
		}
		if v.DeactivationOwner.Threshold != 1 || v.Weight != pt.Weight {
			t.Errorf("target %d: re-added validator = %+v, want threshold 1 deactivation owner and weight %d", i, v.L1Validator, pt.Weight)
		}
		if _, found := rt.chain.l1[pt.ValidationID]; found {
			t.Errorf("target %d: original validation %s still exists", i, pt.ValidationID)
		}
	}

	// A second run finds every target rotated and submits nothing.
	results, err = rt.rotate(t, weights...)
	if err != nil {
		t.Fatalf("Rotate() rerun error = %v", err)
	}
	if len(rt.chain.txs) != len(wantTxs) {
		t.Errorf("rerun submitted %v", rt.chain.txs[len(wantTxs):])
	}
	for i, res := range results {
		if !res.AlreadyRotated {
			t.Errorf("rerun target %d AlreadyRotated = false", i)
		}
	}
}

// TestRotateWithLaggingEpoch checks that a rotation does not wait for the
// epoch set to catch up. The epoch set holds the pre-rotation set for the
// whole run, as when an epoch lasts longer than the rotation. Each message is
// aggregated against the epoch set, which still has the target, so the P-Chain
// accepts it at once.
func TestRotateWithLaggingEpoch(t *testing.T) {
	weights := []uint64{100, 101, 102, 103, 104, 105, 106, 107, 108, 109}
	rt := newRotationTest(t, weights, 100, 104, 109)
	rt.chain.stale = rt.chain.accepted()
	rt.chain.lag = 1_000_000
	rt.chain.staleLeft = rt.chain.lag

	if _, err := rt.rotate(t, weights...); err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}
	var wantTxs []string
	for _, pt := range rt.planned {
		wantTxs = append(wantTxs, "remove "+pt.NodeID.String(), "add "+pt.NodeID.String())
	}
	if !slices.Equal(rt.chain.txs, wantTxs) {
		t.Errorf("transactions = %v, want %v", rt.chain.txs, wantTxs)
	}
	if got := rt.chain.calls["RegisterL1Validator"]; got != len(rt.planned) {
		t.Errorf("RegisterL1Validator calls = %d, want %d", got, len(rt.planned))
	}
}

// TestRotateRefusesBeforeRemoval checks the conditions under which Rotate
// must not submit any transaction, so a validator is never removed without a
// way to re-add it.
func TestRotateRefusesBeforeRemoval(t *testing.T) {
	tests := []struct {
		name          string
		weights       []uint64
		targets       []uint64
		signerWeights []uint64
		setup         func(rt *rotationTest)
		wantErr       error
	}{
		{
			name:          "removal_below_67_percent",
			weights:       []uint64{10, 20, 30, 40},
			targets:       []uint64{10, 20},
			signerWeights: []uint64{10, 20, 30},
			wantErr:       avawarp.ErrInsufficientWeight,
		},
		{
			// The removal has 70%. Without the target, the re-add has 31
			// of 60.
			name:          "readd_below_67_percent_without_the_target",
			weights:       []uint64{40, 29, 31},
			targets:       []uint64{40},
			signerWeights: []uint64{40, 31},
			wantErr:       avawarp.ErrInsufficientWeight,
		},
		{
			name:          "no_proof_of_possession_for_the_target",
			weights:       []uint64{10, 20, 30, 40},
			targets:       []uint64{10, 20},
			signerWeights: []uint64{20, 30, 40},
			wantErr:       errMissingPoP,
		},
		{
			name:          "readd_expires_within_margin",
			weights:       []uint64{10, 20, 30, 40},
			targets:       []uint64{10, 20},
			signerWeights: []uint64{10, 20, 30, 40},
			setup: func(rt *rotationTest) {
				rt.chain.now = time.Unix(testExpiry, 0).Add(-30 * time.Minute)
			},
			wantErr: errExpiryTooSoon,
		},
		{
			name:          "other_target_already_removed",
			weights:       []uint64{10, 20, 30, 40},
			targets:       []uint64{10, 20},
			signerWeights: []uint64{10, 20, 30, 40},
			setup: func(rt *rotationTest) {
				delete(rt.chain.l1, rt.planned[1].ValidationID)
			},
			wantErr: errAnotherTargetRemoved,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newRotationTest(t, tt.weights, tt.targets...)
			if tt.setup != nil {
				tt.setup(rt)
			}
			_, err := rt.rotate(t, tt.signerWeights...)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Rotate() error = %v, want %v", err, tt.wantErr)
			}
			if len(rt.chain.txs) != 0 {
				t.Fatalf("Rotate() submitted %v", rt.chain.txs)
			}
		})
	}
}

// TestRotateResumesRemovedTarget checks that a run that stopped after a
// removal re-adds that target and continues.
func TestRotateResumesRemovedTarget(t *testing.T) {
	rt := newRotationTest(t, []uint64{10, 20, 30, 40}, 10, 20)
	delete(rt.chain.l1, rt.planned[0].ValidationID)

	_, err := rt.rotate(t, 10, 20, 30, 40)
	if err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}
	want := []string{
		"add " + rt.planned[0].NodeID.String(),
		"remove " + rt.planned[1].NodeID.String(),
		"add " + rt.planned[1].NodeID.String(),
	}
	if !slices.Equal(rt.chain.txs, want) {
		t.Fatalf("transactions = %v, want %v", rt.chain.txs, want)
	}
}

// TestRotateChecksOwners checks that Rotate stops when the re-added validator
// does not have the planned owners.
func TestRotateChecksOwners(t *testing.T) {
	rt := newRotationTest(t, []uint64{10, 20, 30, 40}, 10, 20)
	rt.chain.emptyOwners = true

	_, err := rt.rotate(t, 10, 20, 30, 40)
	if !errors.Is(err, errOwnerMismatch) {
		t.Fatalf("Rotate() error = %v, want %v", err, errOwnerMismatch)
	}
	want := []string{
		"remove " + rt.planned[0].NodeID.String(),
		"add " + rt.planned[0].NodeID.String(),
	}
	if !slices.Equal(rt.chain.txs, want) {
		t.Fatalf("transactions = %v, want %v", rt.chain.txs, want)
	}
}

func TestRotateStopsWhenNotConfirmed(t *testing.T) {
	rt := newRotationTest(t, []uint64{10, 20, 30, 40}, 10)
	b, err := SignPlan(rt.planned, rt.signers)
	if err != nil {
		t.Fatalf("SignPlan() error = %v", err)
	}
	c, err := Collect(rt.planned, []*Bundle{b})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	errDeclined := errors.New("declined")
	_, err = Rotate(t.Context(), RotateConfig{
		Chain:        rt.chain,
		SubnetID:     testSubnetID,
		Planned:      rt.planned,
		Collected:    c,
		Confirm:      func(PlannedTarget) error { return errDeclined },
		Log:          io.Discard,
		PollInterval: time.Millisecond,
		WaitTimeout:  time.Second,
		Now:          func() time.Time { return rt.chain.now },
	})
	if !errors.Is(err, errDeclined) {
		t.Fatalf("Rotate() error = %v, want %v", err, errDeclined)
	}
	if len(rt.chain.txs) != 0 {
		t.Fatalf("Rotate() submitted %v", rt.chain.txs)
	}
}

// TestRotateRetriesTransientErrors checks that rate limits and an epoch
// change during submit do not stop a rotation.
func TestRotateRetriesTransientErrors(t *testing.T) {
	rt := newRotationTest(t, []uint64{10, 20, 30, 40}, 10, 20)
	// No epoch lag, so the only retries are the injected errors.
	rt.chain.lag = 0
	rt.chain.fail = map[string][]error{
		"ValidatorSet":         {err429},
		"L1Validator":          {errCloudflare1015},
		"SetL1ValidatorWeight": {err429},
		"RegisterL1Validator":  {errWarpLag, errWarpLag},
	}

	_, err := rt.rotate(t, 10, 20, 30, 40)
	if err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}
	want := []string{
		"remove " + rt.planned[0].NodeID.String(),
		"add " + rt.planned[0].NodeID.String(),
		"remove " + rt.planned[1].NodeID.String(),
		"add " + rt.planned[1].NodeID.String(),
	}
	if !slices.Equal(rt.chain.txs, want) {
		t.Fatalf("transactions = %v, want %v", rt.chain.txs, want)
	}
	// 2 targets, 1 extra removal call and 2 extra re-add calls.
	if got := rt.chain.calls["SetL1ValidatorWeight"]; got != 3 {
		t.Errorf("SetL1ValidatorWeight calls = %d, want 3", got)
	}
	if got := rt.chain.calls["RegisterL1Validator"]; got != 4 {
		t.Errorf("RegisterL1Validator calls = %d, want 4", got)
	}
}

// TestRotateDoesNotResubmitAcceptedTx checks that a transaction that took
// effect but returned a rate limit error is not issued again.
func TestRotateDoesNotResubmitAcceptedTx(t *testing.T) {
	rt := newRotationTest(t, []uint64{10, 20, 30, 40}, 10)
	rt.chain.failAfterApply = map[string][]error{
		"SetL1ValidatorWeight": {err429},
		"RegisterL1Validator":  {errCloudflare1015},
	}

	_, err := rt.rotate(t, 10, 20, 30, 40)
	if err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}
	if got := rt.chain.calls["SetL1ValidatorWeight"]; got != 1 {
		t.Errorf("SetL1ValidatorWeight calls = %d, want 1", got)
	}
	if got := rt.chain.calls["RegisterL1Validator"]; got != 1 {
		t.Errorf("RegisterL1Validator calls = %d, want 1", got)
	}
}

func TestRotateDoesNotRetryLogicError(t *testing.T) {
	rt := newRotationTest(t, []uint64{10, 20, 30, 40}, 10)
	rt.chain.fail = map[string][]error{
		"RegisterL1Validator": {errLogic},
	}

	_, err := rt.rotate(t, 10, 20, 30, 40)
	if !errors.Is(err, errLogic) {
		t.Fatalf("Rotate() error = %v, want %v", err, errLogic)
	}
	if got := rt.chain.calls["RegisterL1Validator"]; got != 1 {
		t.Errorf("RegisterL1Validator calls = %d, want 1", got)
	}
}
