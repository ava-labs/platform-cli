package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils/crypto/bls/signer/localsigner"
	"github.com/ava-labs/avalanchego/vms/platformvm/warp/message"
	"github.com/ava-labs/platform-cli/pkg/warp"
)

func TestLoadBLSKey(t *testing.T) {
	want, err := localsigner.New()
	if err != nil {
		t.Fatalf("localsigner.New() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "signer.key")
	if err := want.ToFile(path); err != nil {
		t.Fatalf("ToFile() error = %v", err)
	}

	got, err := loadBLSKey(path)
	if err != nil {
		t.Fatalf("loadBLSKey() error = %v", err)
	}
	if !got.PublicKey().Equals(want.PublicKey()) {
		t.Fatal("loadBLSKey() public key does not match the key file")
	}
}

func TestLoadBLSKeyRejectsWrongLength(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signer.key")
	// A hex-encoded key is the most likely wrong format.
	if err := os.WriteFile(path, []byte("0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	_, err := loadBLSKey(path)
	if !errors.Is(err, errWarpBadKeyFile) {
		t.Fatalf("loadBLSKey() error = %v, want %v", err, errWarpBadKeyFile)
	}
}

func TestVerifyRegisterExpiry(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tests := []struct {
		name    string
		expiry  time.Time
		wantErr error
	}{
		{
			name:   "23h_ahead",
			expiry: now.Add(23 * time.Hour),
		},
		{
			name:   "end_of_window",
			expiry: now.Add(registerExpiryWindow),
		},
		{
			name:    "past_window",
			expiry:  now.Add(registerExpiryWindow + time.Second),
			wantErr: errWarpExpiryTooFar,
		},
		{
			name:    "now",
			expiry:  now,
			wantErr: errWarpExpiryPassed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyRegisterExpiry(uint64(tt.expiry.Unix()), now)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("verifyRegisterExpiry() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestWarpFlagDefaults checks that commands that share a flag variable keep
// their own defaults.
func TestWarpFlagDefaults(t *testing.T) {
	if warpBundleOut != "bundle.json" {
		t.Errorf("warp sign --out default = %q, want %q", warpBundleOut, "bundle.json")
	}
	if warpPlanOut != "plan.json" {
		t.Errorf("warp plan --out default = %q, want %q", warpPlanOut, "plan.json")
	}
}

func TestSignPlan(t *testing.T) {
	dir := t.TempDir()
	vdrSet := make(map[ids.NodeID]*validators.GetValidatorOutput)
	var targets []warp.TargetValidator
	for i := range 2 {
		s, err := localsigner.New()
		if err != nil {
			t.Fatalf("localsigner.New() error = %v", err)
		}
		path := filepath.Join(dir, strconv.Itoa(i), blsKeyFileName)
		if err := s.ToFile(path); err != nil {
			t.Fatalf("ToFile() error = %v", err)
		}
		nodeID := ids.GenerateTestNodeID()
		vdrSet[nodeID] = &validators.GetValidatorOutput{
			NodeID:    nodeID,
			PublicKey: s.PublicKey(),
			Weight:    100,
		}
		targets = append(targets, warp.TargetValidator{
			ValidationID: ids.GenerateTestID(),
			NodeID:       nodeID,
			PublicKey:    s.PublicKey(),
			Weight:       100,
		})
	}
	vdrs, err := validators.FlattenValidatorSet(vdrSet)
	if err != nil {
		t.Fatalf("FlattenValidatorSet() error = %v", err)
	}
	owner := message.PChainOwner{
		Threshold: 1,
		Addresses: []ids.ShortID{ids.GenerateTestShortID()},
	}
	p, err := warp.NewPlan(warp.PlanConfig{
		NetworkID:             5,
		SubnetID:              ids.GenerateTestID(),
		ManagerBlockchainID:   ids.GenerateTestID(),
		ManagerAddress:        make([]byte, 20),
		RemainingBalanceOwner: owner,
		DeactivationOwner:     owner,
		Balance:               1,
		Expiry:                uint64(time.Now().Add(time.Hour).Unix()),
		Snapshot:              vdrs,
	}, targets)
	if err != nil {
		t.Fatalf("warp.NewPlan() error = %v", err)
	}
	planPath := filepath.Join(dir, "plan.json")
	if err := writeNewJSONFile(planPath, p); err != nil {
		t.Fatalf("writeNewJSONFile() error = %v", err)
	}

	// --bls-key-dir finds both keys.
	paths, err := blsKeyPaths(nil, dir)
	if err != nil {
		t.Fatalf("blsKeyPaths() error = %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("blsKeyPaths() = %v, want 2 paths", paths)
	}

	warpYes = true
	warpBundleOut = filepath.Join(dir, "bundle.json")
	t.Cleanup(func() {
		warpYes = false
		warpBundleOut = "bundle.json"
	})
	if err := signPlan(planPath, paths); err != nil {
		t.Fatalf("signPlan() error = %v", err)
	}
	// The bundle file is never overwritten.
	if err := signPlan(planPath, paths); !errors.Is(err, os.ErrExist) {
		t.Fatalf("second signPlan() error = %v, want %v", err, os.ErrExist)
	}

	_, planned, err := loadPlan(planPath)
	if err != nil {
		t.Fatalf("loadPlan() error = %v", err)
	}
	b := new(warp.Bundle)
	if err := readJSONFile(warpBundleOut, b); err != nil {
		t.Fatalf("readJSONFile() error = %v", err)
	}
	c, err := warp.Collect(planned, []*warp.Bundle{b})
	if err != nil {
		t.Fatalf("warp.Collect() error = %v", err)
	}
	if len(c.PoPs) != 2 {
		t.Errorf("len(PoPs) = %d, want 2", len(c.PoPs))
	}
	for _, msgID := range warp.MessageIDs(planned) {
		if got := len(c.Signatures[msgID]); got != 2 {
			t.Errorf("len(Signatures[%s]) = %d, want 2", msgID, got)
		}
	}
}
