package warp

import (
	"errors"
	"slices"
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils/crypto/bls"
	"github.com/ava-labs/avalanchego/utils/crypto/bls/signer/localsigner"
	"github.com/ava-labs/avalanchego/vms/platformvm/warp/message"
)

const testExpiry = 1_800_000_000

var testSubnetID = ids.GenerateTestID()

// newTestPlan returns a plan that rotates the validators with targetWeights,
// and the signers of vdrs in canonical order. Weights must be unique.
func newTestPlan(t *testing.T, weights []uint64, targetWeights ...uint64) (*Plan, []PlannedTarget, validators.WarpSet, []*localsigner.LocalSigner) {
	t.Helper()
	vdrs, signers := newValidatorSet(t, weights...)
	owner := message.PChainOwner{
		Threshold: 1,
		Addresses: []ids.ShortID{ids.GenerateTestShortID()},
	}
	targets := make([]TargetValidator, len(targetWeights))
	for i, w := range targetWeights {
		idx := indexByWeight(vdrs, w)
		targets[i] = TargetValidator{
			ValidationID: ids.GenerateTestID(),
			NodeID:       vdrs.Validators[idx].NodeIDs[0],
			PublicKey:    vdrs.Validators[idx].PublicKey,
			Weight:       vdrs.Validators[idx].Weight,
		}
	}
	p, err := NewPlan(PlanConfig{
		NetworkID:             testNetworkID,
		SubnetID:              testSubnetID,
		ManagerBlockchainID:   testChainID,
		ManagerAddress:        testManager,
		RemainingBalanceOwner: owner,
		DeactivationOwner:     owner,
		Balance:               1_000_000_000,
		Expiry:                testExpiry,
		Snapshot:              vdrs,
	}, targets)
	if err != nil {
		t.Fatalf("NewPlan() error = %v", err)
	}
	planned, err := p.Decode()
	if err != nil {
		t.Fatalf("%T.Decode() error = %v", p, err)
	}
	return p, planned, vdrs, signers
}

// indexByWeight returns the canonical index of the validator with weight w.
func indexByWeight(vdrs validators.WarpSet, w uint64) int {
	return slices.IndexFunc(vdrs.Validators, func(v *validators.Warp) bool {
		return v.Weight == w
	})
}

func TestPlanDecode(t *testing.T) {
	_, planned, vdrs, _ := newTestPlan(t, []uint64{10, 20, 30}, 10, 30)

	if len(planned) != 2 {
		t.Fatalf("len(planned) = %d, want 2", len(planned))
	}
	for i, w := range []uint64{10, 30} {
		idx := indexByWeight(vdrs, w)
		pt := planned[i]
		removal := pt.Removal.Payload.(*message.L1ValidatorWeight)
		if removal.ValidationID != pt.ValidationID || removal.Nonce != RemovalNonce || removal.Weight != 0 {
			t.Errorf("target %d removal = %+v, want weight 0 of %s at nonce MaxUint64", i, removal, pt.ValidationID)
		}
		readd := pt.ReaddPayload
		if !pt.PublicKey.Equals(vdrs.Validators[idx].PublicKey) || readd.Weight != vdrs.Validators[idx].Weight {
			t.Errorf("target %d re-add key or weight does not match validator %d", i, idx)
		}
		if readd.DisableOwner.Threshold != 1 || readd.RemainingBalanceOwner.Threshold != 1 {
			t.Errorf("target %d re-add owners = %+v / %+v, want threshold 1", i, readd.RemainingBalanceOwner, readd.DisableOwner)
		}
		if readd.ValidationID() != pt.ReaddValidationID {
			t.Errorf("target %d ReaddValidationID = %s, want %s", i, pt.ReaddValidationID, readd.ValidationID())
		}
	}
}

func TestNewPlanRejectsEmptyOwner(t *testing.T) {
	vdrs, _ := newValidatorSet(t, 10)
	_, err := NewPlan(PlanConfig{
		NetworkID:           testNetworkID,
		SubnetID:            testSubnetID,
		ManagerBlockchainID: testChainID,
		ManagerAddress:      testManager,
		RemainingBalanceOwner: message.PChainOwner{
			Threshold: 1,
			Addresses: []ids.ShortID{ids.GenerateTestShortID()},
		},
		Expiry:   testExpiry,
		Snapshot: vdrs,
	}, []TargetValidator{{
		ValidationID: ids.GenerateTestID(),
		NodeID:       vdrs.Validators[0].NodeIDs[0],
		PublicKey:    vdrs.Validators[0].PublicKey,
		Weight:       10,
	}})
	if !errors.Is(err, errEmptyOwner) {
		t.Fatalf("NewPlan() error = %v, want %v", err, errEmptyOwner)
	}
}

// TestPlanDecodeRejectsTampering checks that a signer refuses a plan whose
// messages were changed after the plan was built.
func TestPlanDecodeRejectsTampering(t *testing.T) {
	tests := []struct {
		name    string
		tamper  func(p *Plan)
		wantErr error
	}{
		{
			name: "target_weight",
			tamper: func(p *Plan) {
				p.Targets[0].Weight++
			},
			wantErr: errPlanMismatch,
		},
		{
			name: "removal_of_other_validator",
			tamper: func(p *Plan) {
				p.Targets[0].Removal = p.Targets[1].Removal
			},
			wantErr: errPlanMismatch,
		},
		{
			name: "manager_address",
			tamper: func(p *Plan) {
				p.ManagerAddress = make([]byte, 20)
			},
			wantErr: errPlanMismatch,
		},
		{
			name: "duplicate_target",
			tamper: func(p *Plan) {
				p.Targets[1] = p.Targets[0]
			},
			wantErr: errDuplicateTarget,
		},
		{
			name: "version",
			tamper: func(p *Plan) {
				p.Version++
			},
			wantErr: errPlanVersion,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, _, _, _ := newTestPlan(t, []uint64{10, 20, 30}, 10, 20)
			tt.tamper(p)
			_, err := p.Decode()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("%T.Decode() error = %v, want %v", p, err, tt.wantErr)
			}
		})
	}
}

func TestCollect(t *testing.T) {
	_, planned, _, signers := newTestPlan(t, []uint64{10, 20, 30}, 10)

	b, err := SignPlan(planned, []bls.Signer{signers[0], signers[1]})
	if err != nil {
		t.Fatalf("SignPlan() error = %v", err)
	}
	c, err := Collect(planned, []*Bundle{b})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	for _, msgID := range MessageIDs(planned) {
		if got := len(c.Signatures[msgID]); got != 2 {
			t.Errorf("len(Signatures[%s]) = %d, want 2", msgID, got)
		}
	}
	for _, s := range signers[:2] {
		if _, ok := c.PoPs[string(bls.PublicKeyToCompressedBytes(s.PublicKey()))]; !ok {
			t.Errorf("PoPs is missing signer 0x%x", bls.PublicKeyToCompressedBytes(s.PublicKey()))
		}
	}
}

func TestCollectRejectsBadBundles(t *testing.T) {
	_, planned, _, signers := newTestPlan(t, []uint64{10, 20, 30}, 10)

	signPlan := func(planned []PlannedTarget, s ...bls.Signer) *Bundle {
		b, err := SignPlan(planned, s)
		if err != nil {
			t.Fatalf("SignPlan() error = %v", err)
		}
		return b
	}

	tests := []struct {
		name    string
		bundles func() []*Bundle
		wantErr error
	}{
		{
			name: "swapped_signatures",
			bundles: func() []*Bundle {
				b := signPlan(planned, signers[0])
				sigs := b.Signers[0].Signatures
				removalID, readdID := planned[0].Removal.Message.ID(), planned[0].Readd.Message.ID()
				sigs[removalID], sigs[readdID] = sigs[readdID], sigs[removalID]
				return []*Bundle{b}
			},
			wantErr: errInvalidSignature,
		},
		{
			name: "proof_of_possession_of_other_key",
			bundles: func() []*Bundle {
				b := signPlan(planned, signers[0], signers[1])
				b.Signers[0].ProofOfPossession = b.Signers[1].ProofOfPossession
				return []*Bundle{b}
			},
			wantErr: errInvalidPoP,
		},
		{
			name: "same_key_in_two_bundles",
			bundles: func() []*Bundle {
				return []*Bundle{
					signPlan(planned, signers[0]),
					signPlan(planned, signers[0]),
				}
			},
			wantErr: errDuplicateBundleKey,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Collect(planned, tt.bundles())
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Collect() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// replan returns p rebuilt with a new re-add expiry, as after the old re-adds
// expire. The removal messages do not change.
func replan(t *testing.T, p *Plan, planned []PlannedTarget) []PlannedTarget {
	t.Helper()
	targets := make([]TargetValidator, len(planned))
	for i, pt := range planned {
		targets[i] = TargetValidator{
			ValidationID: pt.ValidationID,
			NodeID:       pt.NodeID,
			PublicKey:    pt.PublicKey,
			Weight:       pt.Weight,
		}
	}
	readd := planned[0].ReaddPayload
	newPlan, err := NewPlan(PlanConfig{
		NetworkID:             p.NetworkID,
		SubnetID:              p.SubnetID,
		ManagerBlockchainID:   p.ManagerBlockchainID,
		ManagerAddress:        p.ManagerAddress,
		RemainingBalanceOwner: readd.RemainingBalanceOwner,
		DeactivationOwner:     readd.DisableOwner,
		Balance:               p.Balance,
		Expiry:                p.Expiry + 3600,
		Snapshot:              p.Snapshot,
	}, targets)
	if err != nil {
		t.Fatalf("NewPlan() error = %v", err)
	}
	newPlanned, err := newPlan.Decode()
	if err != nil {
		t.Fatalf("%T.Decode() error = %v", newPlan, err)
	}
	return newPlanned
}

// TestCollectSkipsUnknownMessages checks that a bundle from an earlier plan
// still gives its removal signature after a re-plan changes the re-add.
func TestCollectSkipsUnknownMessages(t *testing.T) {
	p, planned, _, signers := newTestPlan(t, []uint64{10, 20, 30}, 10)
	oldBundle, err := SignPlan(planned, []bls.Signer{signers[0]})
	if err != nil {
		t.Fatalf("SignPlan() error = %v", err)
	}
	newPlanned := replan(t, p, planned)

	removalID := newPlanned[0].Removal.Message.ID()
	oldReaddID := planned[0].Readd.Message.ID()
	newReaddID := newPlanned[0].Readd.Message.ID()
	if removalID != planned[0].Removal.Message.ID() || oldReaddID == newReaddID {
		t.Fatal("re-plan must keep the removal and change the re-add")
	}

	c, err := Collect(newPlanned, []*Bundle{oldBundle})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if got := len(c.Signatures[removalID]); got != 1 {
		t.Errorf("len(Signatures[removal]) = %d, want 1", got)
	}
	if got := len(c.Signatures[newReaddID]); got != 0 {
		t.Errorf("len(Signatures[new re-add]) = %d, want 0", got)
	}
	if _, ok := c.Signatures[oldReaddID]; ok {
		t.Error("Signatures holds the old re-add")
	}
	if len(c.Skipped) != 1 || !errors.Is(c.Skipped[0], errUnknownMessageID) {
		t.Fatalf("Skipped = %v, want 1 error wrapping %v", c.Skipped, errUnknownMessageID)
	}
}
