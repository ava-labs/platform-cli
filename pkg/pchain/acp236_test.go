package pchain

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/formatting/address"
	"github.com/ava-labs/avalanchego/vms/platformvm/signer"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
	"github.com/ava-labs/avalanchego/wallet/subnet/primary/common"
)

var errAssertable = errors.New("assertable failure")

// =============================================================================
// ACP-236 Issuer Stubs
// =============================================================================

// stubAutoRenewedValidatorTxIssuer implements autoRenewedValidatorTxIssuer.
type stubAutoRenewedValidatorTxIssuer struct {
	tx  *txs.Tx
	err error

	called                    bool
	gotNodeID                 ids.NodeID
	gotWeight                 uint64
	gotSigner                 signer.Signer
	gotAssetID                ids.ID
	gotValidationRewardsOwner *secp256k1fx.OutputOwners
	gotDelegationRewardsOwner *secp256k1fx.OutputOwners
	gotConfigOwner            *secp256k1fx.OutputOwners
	gotDelegationShares       uint32
	gotAutoCompoundShares     uint32
	gotPeriodSeconds          uint64
	gotOpts                   []common.Option
}

func (s *stubAutoRenewedValidatorTxIssuer) IssueAddAutoRenewedValidatorTx(validatorNodeID ids.NodeID, weight uint64, sig signer.Signer, assetID ids.ID, validationRewardsOwner *secp256k1fx.OutputOwners, delegationRewardsOwner *secp256k1fx.OutputOwners, configOwner *secp256k1fx.OutputOwners, delegationShares uint32, autoCompoundRewardShares uint32, periodSeconds uint64, options ...common.Option) (*txs.Tx, error) {
	s.called = true
	s.gotNodeID = validatorNodeID
	s.gotWeight = weight
	s.gotSigner = sig
	s.gotAssetID = assetID
	s.gotValidationRewardsOwner = validationRewardsOwner
	s.gotDelegationRewardsOwner = delegationRewardsOwner
	s.gotConfigOwner = configOwner
	s.gotDelegationShares = delegationShares
	s.gotAutoCompoundShares = autoCompoundRewardShares
	s.gotPeriodSeconds = periodSeconds
	s.gotOpts = options
	return s.tx, s.err
}

// stubSetAutoRenewedValidatorConfigTxIssuer implements setAutoRenewedValidatorConfigTxIssuer.
type stubSetAutoRenewedValidatorConfigTxIssuer struct {
	tx  *txs.Tx
	err error

	called                bool
	gotTxID               ids.ID
	gotAutoCompoundShares uint32
	gotPeriodSeconds      uint64
}

func (s *stubSetAutoRenewedValidatorConfigTxIssuer) IssueSetAutoRenewedValidatorConfigTx(txID ids.ID, autoCompoundRewardShares uint32, periodSeconds uint64, _ ...common.Option) (*txs.Tx, error) {
	s.called = true
	s.gotTxID = txID
	s.gotAutoCompoundShares = autoCompoundRewardShares
	s.gotPeriodSeconds = periodSeconds
	return s.tx, s.err
}

// =============================================================================
// AddAutoRenewedValidatorTx
// =============================================================================

func TestIssueAddAutoRenewedValidatorTx(t *testing.T) {
	nodeID := ids.GenerateTestNodeID()
	rewardAddr := ids.GenerateTestShortID()
	authorityAddr := ids.GenerateTestShortID()
	assetID := ids.GenerateTestID()
	pop := &signer.ProofOfPossession{}
	cfg := AddAutoRenewedValidatorConfig{
		NodeID:                   nodeID,
		StakeAmt:                 123,
		RewardAddr:               rewardAddr,
		ValidatorAuthorityAddr:   authorityAddr,
		DelegationFee:            20_000,
		AutoCompoundRewardShares: 500_000,
		Period:                   14 * 24 * time.Hour,
		BLSSigner:                pop,
	}
	txID := ids.GenerateTestID()

	stub := &stubAutoRenewedValidatorTxIssuer{tx: &txs.Tx{TxID: txID}}
	ctxKey := testContextKey("auto")
	gotTxID, err := issueAddAutoRenewedValidatorTx(stub, assetID, cfg, common.WithContext(context.WithValue(context.Background(), ctxKey, "v")))
	if err != nil {
		t.Fatalf("issueAddAutoRenewedValidatorTx() returned error: %v", err)
	}
	if gotTxID != txID {
		t.Fatalf("issueAddAutoRenewedValidatorTx() txID = %s, want %s", gotTxID, txID)
	}
	if stub.gotNodeID != nodeID {
		t.Fatalf("nodeID = %s, want %s", stub.gotNodeID, nodeID)
	}
	if stub.gotWeight != cfg.StakeAmt {
		t.Fatalf("weight = %d, want %d", stub.gotWeight, cfg.StakeAmt)
	}
	gotPop, ok := stub.gotSigner.(*signer.ProofOfPossession)
	if !ok || gotPop != pop {
		t.Fatalf("signer = %T (%p), want %p", stub.gotSigner, gotPop, pop)
	}
	if stub.gotAssetID != assetID {
		t.Fatalf("assetID = %s, want %s", stub.gotAssetID, assetID)
	}
	// Validation and delegation rewards go to the same owner.
	if stub.gotValidationRewardsOwner != stub.gotDelegationRewardsOwner {
		t.Fatal("validation and delegation rewards owners should be the same pointer")
	}
	assertSingleOwner(t, "validation rewards owner", stub.gotValidationRewardsOwner, rewardAddr)
	assertSingleOwner(t, "config owner", stub.gotConfigOwner, authorityAddr)
	if stub.gotDelegationShares != cfg.DelegationFee {
		t.Fatalf("delegation shares = %d, want %d", stub.gotDelegationShares, cfg.DelegationFee)
	}
	if stub.gotAutoCompoundShares != cfg.AutoCompoundRewardShares {
		t.Fatalf("auto-compound shares = %d, want %d", stub.gotAutoCompoundShares, cfg.AutoCompoundRewardShares)
	}
	if want := uint64(cfg.Period / time.Second); stub.gotPeriodSeconds != want {
		t.Fatalf("period seconds = %d, want %d", stub.gotPeriodSeconds, want)
	}
	if len(stub.gotOpts) == 0 {
		t.Fatal("expected options to be forwarded to the issuer")
	}
}

func TestIssueAddAutoRenewedValidatorTxRejectsInvalidPeriod(t *testing.T) {
	stub := &stubAutoRenewedValidatorTxIssuer{tx: &txs.Tx{TxID: ids.GenerateTestID()}}
	_, err := issueAddAutoRenewedValidatorTx(stub, ids.GenerateTestID(), AddAutoRenewedValidatorConfig{Period: 1500 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "period must be a whole number of seconds") {
		t.Fatalf("error = %v, want whole-seconds error", err)
	}
	if stub.called {
		t.Fatal("issuer should not be called for an invalid period")
	}

	_, err = issueAddAutoRenewedValidatorTx(stub, ids.GenerateTestID(), AddAutoRenewedValidatorConfig{Period: 0})
	if err == nil || !strings.Contains(err.Error(), "period must be positive") {
		t.Fatalf("error = %v, want positive-period error", err)
	}
}

func TestIssueAddAutoRenewedValidatorTxWrapsIssuerError(t *testing.T) {
	stub := &stubAutoRenewedValidatorTxIssuer{err: errAssertable}
	_, err := issueAddAutoRenewedValidatorTx(stub, ids.GenerateTestID(), AddAutoRenewedValidatorConfig{Period: time.Second})
	if err == nil || !strings.Contains(err.Error(), "failed to issue AddAutoRenewedValidatorTx") {
		t.Fatalf("error = %v, want wrapped issue error", err)
	}
}

// =============================================================================
// SetAutoRenewedValidatorConfigTx
// =============================================================================

func TestIssueSetAutoRenewedValidatorConfigTx(t *testing.T) {
	validatorTxID := ids.GenerateTestID()
	issuedTxID := ids.GenerateTestID()
	cfg := SetAutoRenewedValidatorConfigTxConfig{
		TxID:                     validatorTxID,
		AutoCompoundRewardShares: 250_000,
		Period:                   7 * 24 * time.Hour,
	}

	stub := &stubSetAutoRenewedValidatorConfigTxIssuer{tx: &txs.Tx{TxID: issuedTxID}}
	gotTxID, err := issueSetAutoRenewedValidatorConfigTx(stub, cfg)
	if err != nil {
		t.Fatalf("issueSetAutoRenewedValidatorConfigTx() returned error: %v", err)
	}
	if gotTxID != issuedTxID {
		t.Fatalf("txID = %s, want %s", gotTxID, issuedTxID)
	}
	if stub.gotTxID != validatorTxID {
		t.Fatalf("builder txID = %s, want %s", stub.gotTxID, validatorTxID)
	}
	if stub.gotAutoCompoundShares != cfg.AutoCompoundRewardShares {
		t.Fatalf("auto-compound shares = %d, want %d", stub.gotAutoCompoundShares, cfg.AutoCompoundRewardShares)
	}
	if want := uint64(cfg.Period / time.Second); stub.gotPeriodSeconds != want {
		t.Fatalf("period seconds = %d, want %d", stub.gotPeriodSeconds, want)
	}
}

func TestIssueSetAutoRenewedValidatorConfigTxAllowsZeroPeriod(t *testing.T) {
	stub := &stubSetAutoRenewedValidatorConfigTxIssuer{tx: &txs.Tx{TxID: ids.GenerateTestID()}}
	cfg := SetAutoRenewedValidatorConfigTxConfig{
		TxID:   ids.GenerateTestID(),
		Period: 0,
	}
	if _, err := issueSetAutoRenewedValidatorConfigTx(stub, cfg); err != nil {
		t.Fatalf("issueSetAutoRenewedValidatorConfigTx() returned error: %v", err)
	}
	if !stub.called {
		t.Fatal("issuer should be called for a zero (exit-after-cycle) period")
	}
	if stub.gotPeriodSeconds != 0 {
		t.Fatalf("period seconds = %d, want 0", stub.gotPeriodSeconds)
	}
}

func TestIssueSetAutoRenewedValidatorConfigTxRejectsNegativePeriod(t *testing.T) {
	stub := &stubSetAutoRenewedValidatorConfigTxIssuer{tx: &txs.Tx{TxID: ids.GenerateTestID()}}
	_, err := issueSetAutoRenewedValidatorConfigTx(stub, SetAutoRenewedValidatorConfigTxConfig{
		TxID:   ids.GenerateTestID(),
		Period: -time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "period cannot be negative") {
		t.Fatalf("error = %v, want negative-period error", err)
	}
	if stub.called {
		t.Fatal("issuer should not be called for a negative period")
	}
}

func TestIssueSetAutoRenewedValidatorConfigTxWrapsIssuerError(t *testing.T) {
	stub := &stubSetAutoRenewedValidatorConfigTxIssuer{err: errAssertable}
	_, err := issueSetAutoRenewedValidatorConfigTx(stub, SetAutoRenewedValidatorConfigTxConfig{
		TxID:   ids.GenerateTestID(),
		Period: time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "failed to issue SetAutoRenewedValidatorConfigTx") {
		t.Fatalf("error = %v, want wrapped issue error", err)
	}
}

// =============================================================================
// GetAutoRenewedValidatorAuthority
// =============================================================================

func TestGetAutoRenewedValidatorAuthorityUsesValidatorAuthority(t *testing.T) {
	targetTxID := ids.GenerateTestID()
	otherTxID := ids.GenerateTestID()
	rewardAddr := ids.GenerateTestShortID()
	authorityAddr := ids.GenerateTestShortID()

	var gotParams string
	server := newCurrentValidatorsServer(t, &gotParams, []map[string]any{
		{
			"txID":               otherTxID.String(),
			"validatorAuthority": testAPIOwner(t, ids.GenerateTestShortID(), "0", "1"),
		},
		{
			"txID":                  targetTxID.String(),
			"validationRewardOwner": testAPIOwner(t, rewardAddr, "0", "1"),
			"validatorAuthority":    testAPIOwner(t, authorityAddr, "0", "1"),
		},
	})
	defer server.Close()

	owner, err := GetAutoRenewedValidatorAuthority(context.Background(), server.URL, ids.EmptyNodeID, targetTxID)
	if err != nil {
		t.Fatalf("GetAutoRenewedValidatorAuthority() returned error: %v", err)
	}
	assertSingleOwner(t, "authority", owner, authorityAddr)
	if owner.Addrs[0] == rewardAddr {
		t.Fatal("authority lookup returned a reward owner instead of validatorAuthority")
	}
	// With no nodeID the request must not narrow by nodeIDs.
	if strings.Contains(gotParams, "NodeID-") {
		t.Fatalf("params = %s, expected no nodeIDs filter", gotParams)
	}
}

func TestGetAutoRenewedValidatorAuthorityFiltersByNodeID(t *testing.T) {
	targetTxID := ids.GenerateTestID()
	nodeID := ids.GenerateTestNodeID()
	authorityAddr := ids.GenerateTestShortID()

	var gotParams string
	server := newCurrentValidatorsServer(t, &gotParams, []map[string]any{
		{
			"txID":               targetTxID.String(),
			"validatorAuthority": testAPIOwner(t, authorityAddr, "0", "1"),
		},
	})
	defer server.Close()

	if _, err := GetAutoRenewedValidatorAuthority(context.Background(), server.URL, nodeID, targetTxID); err != nil {
		t.Fatalf("GetAutoRenewedValidatorAuthority() returned error: %v", err)
	}
	if !strings.Contains(gotParams, nodeID.String()) {
		t.Fatalf("params = %s, want nodeIDs filter containing %s", gotParams, nodeID)
	}
}

func TestGetAutoRenewedValidatorAuthorityErrors(t *testing.T) {
	targetTxID := ids.GenerateTestID()

	tests := []struct {
		name       string
		validators []map[string]any
		wantErr    string
	}{
		{
			name:       "not found",
			validators: []map[string]any{},
			wantErr:    "not found in current validators",
		},
		{
			name: "missing validatorAuthority",
			validators: []map[string]any{{
				"txID": targetTxID.String(),
			}},
			wantErr: "did not include validatorAuthority",
		},
		{
			name: "bad threshold",
			validators: []map[string]any{{
				"txID":               targetTxID.String(),
				"validatorAuthority": testAPIOwner(t, ids.GenerateTestShortID(), "0", "not-a-number"),
			}},
			wantErr: "invalid validatorAuthority threshold",
		},
		{
			name: "bad address",
			validators: []map[string]any{{
				"txID": targetTxID.String(),
				"validatorAuthority": map[string]any{
					"locktime":  "0",
					"threshold": "1",
					"addresses": []string{"not-an-address"},
				},
			}},
			wantErr: "invalid validatorAuthority addresses",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotParams string
			server := newCurrentValidatorsServer(t, &gotParams, tt.validators)
			defer server.Close()

			_, err := GetAutoRenewedValidatorAuthority(context.Background(), server.URL, ids.EmptyNodeID, targetTxID)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

// =============================================================================
// Helpers
// =============================================================================

func assertSingleOwner(t *testing.T, label string, owner *secp256k1fx.OutputOwners, addr ids.ShortID) {
	t.Helper()
	if owner == nil || owner.Locktime != 0 || owner.Threshold != 1 || len(owner.Addrs) != 1 || owner.Addrs[0] != addr {
		t.Fatalf("%s = %#v, want locktime 0, threshold 1, addrs [%s]", label, owner, addr)
	}
}

func newCurrentValidatorsServer(t *testing.T, gotParams *string, validators []map[string]any) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ext/P" {
			t.Errorf("request path = %q, want /ext/P", r.URL.Path)
		}
		var req struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			ID     any             `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode JSON-RPC request: %v", err)
		}
		if req.Method != "platform.getCurrentValidators" {
			t.Errorf("method = %q, want platform.getCurrentValidators", req.Method)
		}
		if gotParams != nil {
			*gotParams = string(req.Params)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"result": map[string]any{
				"validators": validators,
			},
			"id": req.ID,
		})
	}))
}

func testAPIOwner(t *testing.T, addr ids.ShortID, locktime string, threshold string) map[string]any {
	t.Helper()

	formattedAddr, err := address.Format("P", constants.GetHRP(constants.UnitTestID), addr.Bytes())
	if err != nil {
		t.Fatalf("address.Format() error = %v", err)
	}
	return map[string]any{
		"locktime":  locktime,
		"threshold": threshold,
		"addresses": []string{formattedAddr},
	}
}
