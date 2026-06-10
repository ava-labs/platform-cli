package pchain

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/platformvm/signer"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
	"github.com/ava-labs/avalanchego/wallet/subnet/primary/common"
)

type testContextKey string

// =============================================================================
// Issuer Stubs
// =============================================================================

// stubBaseTxIssuer implements baseTxIssuer, recording the arguments it was
// called with and returning the configured tx/err.
type stubBaseTxIssuer struct {
	tx  *txs.Tx
	err error

	gotOutputs []*avax.TransferableOutput
	gotOpts    []common.Option
}

func (s *stubBaseTxIssuer) IssueBaseTx(outputs []*avax.TransferableOutput, options ...common.Option) (*txs.Tx, error) {
	s.gotOutputs = outputs
	s.gotOpts = options
	return s.tx, s.err
}

// stubExportTxIssuer implements exportTxIssuer.
type stubExportTxIssuer struct {
	tx  *txs.Tx
	err error

	gotChainID ids.ID
	gotOutputs []*avax.TransferableOutput
}

func (s *stubExportTxIssuer) IssueExportTx(chainID ids.ID, outputs []*avax.TransferableOutput, _ ...common.Option) (*txs.Tx, error) {
	s.gotChainID = chainID
	s.gotOutputs = outputs
	return s.tx, s.err
}

// stubImportTxIssuer implements importTxIssuer.
type stubImportTxIssuer struct {
	tx  *txs.Tx
	err error

	gotChainID ids.ID
	gotOwners  *secp256k1fx.OutputOwners
}

func (s *stubImportTxIssuer) IssueImportTx(chainID ids.ID, to *secp256k1fx.OutputOwners, _ ...common.Option) (*txs.Tx, error) {
	s.gotChainID = chainID
	s.gotOwners = to
	return s.tx, s.err
}

// stubValidatorTxIssuer implements permissionlessValidatorTxIssuer.
type stubValidatorTxIssuer struct {
	tx  *txs.Tx
	err error

	gotVdr                    *txs.SubnetValidator
	gotSigner                 signer.Signer
	gotAssetID                ids.ID
	gotValidationRewardsOwner *secp256k1fx.OutputOwners
	gotDelegationRewardsOwner *secp256k1fx.OutputOwners
	gotShares                 uint32
}

func (s *stubValidatorTxIssuer) IssueAddPermissionlessValidatorTx(vdr *txs.SubnetValidator, sig signer.Signer, assetID ids.ID, validationRewardsOwner *secp256k1fx.OutputOwners, delegationRewardsOwner *secp256k1fx.OutputOwners, shares uint32, _ ...common.Option) (*txs.Tx, error) {
	s.gotVdr = vdr
	s.gotSigner = sig
	s.gotAssetID = assetID
	s.gotValidationRewardsOwner = validationRewardsOwner
	s.gotDelegationRewardsOwner = delegationRewardsOwner
	s.gotShares = shares
	return s.tx, s.err
}

// stubDelegatorTxIssuer implements permissionlessDelegatorTxIssuer.
type stubDelegatorTxIssuer struct {
	tx  *txs.Tx
	err error

	gotVdr          *txs.SubnetValidator
	gotAssetID      ids.ID
	gotRewardsOwner *secp256k1fx.OutputOwners
}

func (s *stubDelegatorTxIssuer) IssueAddPermissionlessDelegatorTx(vdr *txs.SubnetValidator, assetID ids.ID, rewardsOwner *secp256k1fx.OutputOwners, _ ...common.Option) (*txs.Tx, error) {
	s.gotVdr = vdr
	s.gotAssetID = assetID
	s.gotRewardsOwner = rewardsOwner
	return s.tx, s.err
}

// stubCreateSubnetTxIssuer implements createSubnetTxIssuer.
type stubCreateSubnetTxIssuer struct {
	tx  *txs.Tx
	err error

	gotOwner *secp256k1fx.OutputOwners
}

func (s *stubCreateSubnetTxIssuer) IssueCreateSubnetTx(owner *secp256k1fx.OutputOwners, _ ...common.Option) (*txs.Tx, error) {
	s.gotOwner = owner
	return s.tx, s.err
}

// stubTransferSubnetOwnershipTxIssuer implements transferSubnetOwnershipTxIssuer.
type stubTransferSubnetOwnershipTxIssuer struct {
	tx  *txs.Tx
	err error

	gotSubnetID ids.ID
	gotOwner    *secp256k1fx.OutputOwners
}

func (s *stubTransferSubnetOwnershipTxIssuer) IssueTransferSubnetOwnershipTx(subnetID ids.ID, owner *secp256k1fx.OutputOwners, _ ...common.Option) (*txs.Tx, error) {
	s.gotSubnetID = subnetID
	s.gotOwner = owner
	return s.tx, s.err
}

// stubConvertSubnetToL1TxIssuer implements convertSubnetToL1TxIssuer.
type stubConvertSubnetToL1TxIssuer struct {
	tx  *txs.Tx
	err error

	gotSubnetID    ids.ID
	gotChainID     ids.ID
	gotManagerAddr []byte
	gotValidators  []*txs.ConvertSubnetToL1Validator
}

func (s *stubConvertSubnetToL1TxIssuer) IssueConvertSubnetToL1Tx(subnetID ids.ID, chainID ids.ID, address []byte, validators []*txs.ConvertSubnetToL1Validator, _ ...common.Option) (*txs.Tx, error) {
	s.gotSubnetID = subnetID
	s.gotChainID = chainID
	s.gotManagerAddr = address
	s.gotValidators = validators
	return s.tx, s.err
}

// stubCreateChainTxIssuer implements createChainTxIssuer.
type stubCreateChainTxIssuer struct {
	tx  *txs.Tx
	err error

	gotCfg CreateChainConfig
}

func (s *stubCreateChainTxIssuer) IssueCreateChainTx(subnetID ids.ID, genesis []byte, vmID ids.ID, fxIDs []ids.ID, chainName string, _ ...common.Option) (*txs.Tx, error) {
	s.gotCfg = CreateChainConfig{
		SubnetID:  subnetID,
		Genesis:   genesis,
		VMID:      vmID,
		FxIDs:     fxIDs,
		ChainName: chainName,
	}
	return s.tx, s.err
}

// =============================================================================
// Tests
// =============================================================================

func TestIssueSendTx(t *testing.T) {
	assetID := ids.GenerateTestID()
	dest := ids.GenerateTestShortID()
	amount := uint64(42_000)
	txID := ids.GenerateTestID()

	issuer := &stubBaseTxIssuer{tx: &txs.Tx{TxID: txID}}
	gotTxID, err := issueSendTx(issuer, assetID, dest, amount)
	if err != nil {
		t.Fatalf("issueSendTx() returned error: %v", err)
	}
	if gotTxID != txID {
		t.Fatalf("issueSendTx() txID = %s, want %s", gotTxID, txID)
	}

	captured := issuer.gotOutputs
	if len(captured) != 1 {
		t.Fatalf("issueSendTx() output count = %d, want 1", len(captured))
	}
	if captured[0].Asset.ID != assetID {
		t.Fatalf("issueSendTx() assetID = %s, want %s", captured[0].Asset.ID, assetID)
	}
	out, ok := captured[0].Out.(*secp256k1fx.TransferOutput)
	if !ok {
		t.Fatalf("issueSendTx() output type = %T, want *secp256k1fx.TransferOutput", captured[0].Out)
	}
	if out.Amt != amount {
		t.Fatalf("issueSendTx() amount = %d, want %d", out.Amt, amount)
	}
	if out.OutputOwners.Threshold != 1 {
		t.Fatalf("issueSendTx() threshold = %d, want 1", out.OutputOwners.Threshold)
	}
	if len(out.OutputOwners.Addrs) != 1 || out.OutputOwners.Addrs[0] != dest {
		t.Fatalf("issueSendTx() owner addrs = %#v, want [%s]", out.OutputOwners.Addrs, dest)
	}
}

func TestIssueSendTxPassesOptions(t *testing.T) {
	ctx := context.WithValue(context.Background(), testContextKey("key"), "value")
	issuer := &stubBaseTxIssuer{tx: &txs.Tx{TxID: ids.GenerateTestID()}}
	_, err := issueSendTx(
		issuer,
		ids.GenerateTestID(),
		ids.GenerateTestShortID(),
		1,
		common.WithContext(ctx),
	)
	if err != nil {
		t.Fatalf("issueSendTx() returned error: %v", err)
	}
	gotCtx := common.NewOptions(issuer.gotOpts).Context()
	if gotCtx.Value(testContextKey("key")) != "value" {
		t.Fatalf("issueSendTx() context option not propagated")
	}
}

func TestIssueSendTxError(t *testing.T) {
	expectedErr := errors.New("boom")
	issuer := &stubBaseTxIssuer{err: expectedErr}
	_, err := issueSendTx(
		issuer,
		ids.GenerateTestID(),
		ids.GenerateTestShortID(),
		1,
	)
	if err == nil {
		t.Fatal("issueSendTx() expected error")
	}
	if !strings.Contains(err.Error(), "failed to issue BaseTx") {
		t.Fatalf("issueSendTx() error = %v, want wrapped BaseTx message", err)
	}
}

func TestIssueExportTx(t *testing.T) {
	destChainID := ids.GenerateTestID()
	assetID := ids.GenerateTestID()
	owner := ids.GenerateTestShortID()
	amount := uint64(7)
	txID := ids.GenerateTestID()

	issuer := &stubExportTxIssuer{tx: &txs.Tx{TxID: txID}}
	gotTxID, err := issueExportTx(
		issuer,
		destChainID,
		assetID,
		owner,
		amount,
	)
	if err != nil {
		t.Fatalf("issueExportTx() returned error: %v", err)
	}
	if gotTxID != txID {
		t.Fatalf("issueExportTx() txID = %s, want %s", gotTxID, txID)
	}
	if issuer.gotChainID != destChainID {
		t.Fatalf("issueExportTx() chainID = %s, want %s", issuer.gotChainID, destChainID)
	}
	captured := issuer.gotOutputs
	if len(captured) != 1 {
		t.Fatalf("issueExportTx() output count = %d, want 1", len(captured))
	}
	out, ok := captured[0].Out.(*secp256k1fx.TransferOutput)
	if !ok {
		t.Fatalf("issueExportTx() output type = %T, want *secp256k1fx.TransferOutput", captured[0].Out)
	}
	if out.Amt != amount {
		t.Fatalf("issueExportTx() amount = %d, want %d", out.Amt, amount)
	}
	if len(out.OutputOwners.Addrs) != 1 || out.OutputOwners.Addrs[0] != owner {
		t.Fatalf("issueExportTx() owner addrs = %#v, want [%s]", out.OutputOwners.Addrs, owner)
	}
}

func TestIssueImportTx(t *testing.T) {
	sourceChainID := ids.GenerateTestID()
	owner := ids.GenerateTestShortID()
	txID := ids.GenerateTestID()

	issuer := &stubImportTxIssuer{tx: &txs.Tx{TxID: txID}}
	gotTxID, err := issueImportTx(
		issuer,
		sourceChainID,
		owner,
	)
	if err != nil {
		t.Fatalf("issueImportTx() returned error: %v", err)
	}
	if gotTxID != txID {
		t.Fatalf("issueImportTx() txID = %s, want %s", gotTxID, txID)
	}
	if issuer.gotChainID != sourceChainID {
		t.Fatalf("issueImportTx() chainID = %s, want %s", issuer.gotChainID, sourceChainID)
	}
	if issuer.gotOwners == nil {
		t.Fatal("issueImportTx() owners is nil")
	}
	if len(issuer.gotOwners.Addrs) != 1 || issuer.gotOwners.Addrs[0] != owner {
		t.Fatalf("issueImportTx() owner addrs = %#v, want [%s]", issuer.gotOwners.Addrs, owner)
	}
}

func TestIssueAddPermissionlessValidatorTx(t *testing.T) {
	nodeID := ids.GenerateTestNodeID()
	rewardAddr := ids.GenerateTestShortID()
	assetID := ids.GenerateTestID()
	pop := &signer.ProofOfPossession{}
	start := time.Unix(1_700_000_000, 0).UTC()
	end := start.Add(24 * time.Hour)
	cfg := AddPermissionlessValidatorConfig{
		NodeID:        nodeID,
		Start:         start,
		End:           end,
		StakeAmt:      123,
		RewardAddr:    rewardAddr,
		DelegationFee: 20_000,
		BLSSigner:     pop,
	}
	txID := ids.GenerateTestID()

	issuer := &stubValidatorTxIssuer{tx: &txs.Tx{TxID: txID}}
	gotTxID, err := issueAddPermissionlessValidatorTx(
		issuer,
		assetID,
		cfg,
	)
	if err != nil {
		t.Fatalf("issueAddPermissionlessValidatorTx() returned error: %v", err)
	}
	if gotTxID != txID {
		t.Fatalf("issueAddPermissionlessValidatorTx() txID = %s, want %s", gotTxID, txID)
	}
	gotVdr := issuer.gotVdr
	if gotVdr == nil {
		t.Fatal("issueAddPermissionlessValidatorTx() validator is nil")
	}
	if gotVdr.Validator.NodeID != cfg.NodeID {
		t.Fatalf("issueAddPermissionlessValidatorTx() nodeID = %s, want %s", gotVdr.Validator.NodeID, cfg.NodeID)
	}
	if gotVdr.Validator.Start != uint64(cfg.Start.Unix()) || gotVdr.Validator.End != uint64(cfg.End.Unix()) {
		t.Fatalf("issueAddPermissionlessValidatorTx() time range = [%d,%d], want [%d,%d]",
			gotVdr.Validator.Start, gotVdr.Validator.End, uint64(cfg.Start.Unix()), uint64(cfg.End.Unix()))
	}
	if gotVdr.Validator.Wght != cfg.StakeAmt {
		t.Fatalf("issueAddPermissionlessValidatorTx() stake = %d, want %d", gotVdr.Validator.Wght, cfg.StakeAmt)
	}
	if gotVdr.Subnet != ids.Empty {
		t.Fatalf("issueAddPermissionlessValidatorTx() subnet = %s, want Primary Network (ids.Empty)", gotVdr.Subnet)
	}
	gotPop, ok := issuer.gotSigner.(*signer.ProofOfPossession)
	if !ok {
		t.Fatalf("issueAddPermissionlessValidatorTx() signer type = %T, want *signer.ProofOfPossession", issuer.gotSigner)
	}
	if gotPop != pop {
		t.Fatal("issueAddPermissionlessValidatorTx() signer pointer mismatch")
	}
	if issuer.gotAssetID != assetID {
		t.Fatalf("issueAddPermissionlessValidatorTx() assetID = %s, want %s", issuer.gotAssetID, assetID)
	}
	if issuer.gotValidationRewardsOwner == nil || len(issuer.gotValidationRewardsOwner.Addrs) != 1 || issuer.gotValidationRewardsOwner.Addrs[0] != rewardAddr {
		t.Fatalf("issueAddPermissionlessValidatorTx() validation owner addrs = %#v, want [%s]", issuer.gotValidationRewardsOwner, rewardAddr)
	}
	if issuer.gotDelegationRewardsOwner == nil || len(issuer.gotDelegationRewardsOwner.Addrs) != 1 || issuer.gotDelegationRewardsOwner.Addrs[0] != rewardAddr {
		t.Fatalf("issueAddPermissionlessValidatorTx() delegation owner addrs = %#v, want [%s]", issuer.gotDelegationRewardsOwner, rewardAddr)
	}
	if issuer.gotShares != cfg.DelegationFee {
		t.Fatalf("issueAddPermissionlessValidatorTx() shares = %d, want %d", issuer.gotShares, cfg.DelegationFee)
	}
}

func TestIssueAddPermissionlessDelegatorTx(t *testing.T) {
	nodeID := ids.GenerateTestNodeID()
	rewardAddr := ids.GenerateTestShortID()
	assetID := ids.GenerateTestID()
	start := time.Unix(1_700_000_100, 0).UTC()
	end := start.Add(12 * time.Hour)
	cfg := AddPermissionlessDelegatorConfig{
		NodeID:     nodeID,
		Start:      start,
		End:        end,
		StakeAmt:   222,
		RewardAddr: rewardAddr,
	}
	txID := ids.GenerateTestID()

	issuer := &stubDelegatorTxIssuer{tx: &txs.Tx{TxID: txID}}
	gotTxID, err := issueAddPermissionlessDelegatorTx(
		issuer,
		assetID,
		cfg,
	)
	if err != nil {
		t.Fatalf("issueAddPermissionlessDelegatorTx() returned error: %v", err)
	}
	if gotTxID != txID {
		t.Fatalf("issueAddPermissionlessDelegatorTx() txID = %s, want %s", gotTxID, txID)
	}
	gotVdr := issuer.gotVdr
	if gotVdr == nil {
		t.Fatal("issueAddPermissionlessDelegatorTx() validator is nil")
	}
	if gotVdr.Validator.NodeID != cfg.NodeID {
		t.Fatalf("issueAddPermissionlessDelegatorTx() nodeID = %s, want %s", gotVdr.Validator.NodeID, cfg.NodeID)
	}
	if gotVdr.Validator.Wght != cfg.StakeAmt {
		t.Fatalf("issueAddPermissionlessDelegatorTx() stake = %d, want %d", gotVdr.Validator.Wght, cfg.StakeAmt)
	}
	if gotVdr.Subnet != ids.Empty {
		t.Fatalf("issueAddPermissionlessDelegatorTx() subnet = %s, want Primary Network (ids.Empty)", gotVdr.Subnet)
	}
	if issuer.gotAssetID != assetID {
		t.Fatalf("issueAddPermissionlessDelegatorTx() assetID = %s, want %s", issuer.gotAssetID, assetID)
	}
	if issuer.gotRewardsOwner == nil || len(issuer.gotRewardsOwner.Addrs) != 1 || issuer.gotRewardsOwner.Addrs[0] != rewardAddr {
		t.Fatalf("issueAddPermissionlessDelegatorTx() rewards owner addrs = %#v, want [%s]", issuer.gotRewardsOwner, rewardAddr)
	}
}

func TestIssueCreateSubnetTx(t *testing.T) {
	owner := ids.GenerateTestShortID()
	txID := ids.GenerateTestID()

	issuer := &stubCreateSubnetTxIssuer{tx: &txs.Tx{TxID: txID}}
	gotTxID, err := issueCreateSubnetTx(
		issuer,
		owner,
	)
	if err != nil {
		t.Fatalf("issueCreateSubnetTx() returned error: %v", err)
	}
	if gotTxID != txID {
		t.Fatalf("issueCreateSubnetTx() txID = %s, want %s", gotTxID, txID)
	}
	if issuer.gotOwner == nil || len(issuer.gotOwner.Addrs) != 1 || issuer.gotOwner.Addrs[0] != owner {
		t.Fatalf("issueCreateSubnetTx() owner addrs = %#v, want [%s]", issuer.gotOwner, owner)
	}
}

func TestIssueTransferSubnetOwnershipTx(t *testing.T) {
	subnetID := ids.GenerateTestID()
	newOwner := ids.GenerateTestShortID()
	txID := ids.GenerateTestID()

	issuer := &stubTransferSubnetOwnershipTxIssuer{tx: &txs.Tx{TxID: txID}}
	gotTxID, err := issueTransferSubnetOwnershipTx(
		issuer,
		subnetID,
		newOwner,
	)
	if err != nil {
		t.Fatalf("issueTransferSubnetOwnershipTx() returned error: %v", err)
	}
	if gotTxID != txID {
		t.Fatalf("issueTransferSubnetOwnershipTx() txID = %s, want %s", gotTxID, txID)
	}
	if issuer.gotSubnetID != subnetID {
		t.Fatalf("issueTransferSubnetOwnershipTx() subnetID = %s, want %s", issuer.gotSubnetID, subnetID)
	}
	if issuer.gotOwner == nil || len(issuer.gotOwner.Addrs) != 1 || issuer.gotOwner.Addrs[0] != newOwner {
		t.Fatalf("issueTransferSubnetOwnershipTx() owner addrs = %#v, want [%s]", issuer.gotOwner, newOwner)
	}
}

func TestIssueConvertSubnetToL1Tx(t *testing.T) {
	subnetID := ids.GenerateTestID()
	chainID := ids.GenerateTestID()
	managerAddr := []byte{0x01, 0x02}
	validators := []*txs.ConvertSubnetToL1Validator{{NodeID: []byte{0xAA}, Weight: 1}}
	txID := ids.GenerateTestID()

	issuer := &stubConvertSubnetToL1TxIssuer{tx: &txs.Tx{TxID: txID}}
	gotTxID, err := issueConvertSubnetToL1Tx(
		issuer,
		subnetID,
		chainID,
		managerAddr,
		validators,
	)
	if err != nil {
		t.Fatalf("issueConvertSubnetToL1Tx() returned error: %v", err)
	}
	if gotTxID != txID {
		t.Fatalf("issueConvertSubnetToL1Tx() txID = %s, want %s", gotTxID, txID)
	}
	if issuer.gotSubnetID != subnetID || issuer.gotChainID != chainID {
		t.Fatalf("issueConvertSubnetToL1Tx() IDs = (%s,%s), want (%s,%s)", issuer.gotSubnetID, issuer.gotChainID, subnetID, chainID)
	}
	gotManagerAddr := issuer.gotManagerAddr
	if len(gotManagerAddr) != len(managerAddr) || gotManagerAddr[0] != managerAddr[0] || gotManagerAddr[1] != managerAddr[1] {
		t.Fatalf("issueConvertSubnetToL1Tx() managerAddr = %x, want %x", gotManagerAddr, managerAddr)
	}
	if len(issuer.gotValidators) != 1 || issuer.gotValidators[0] != validators[0] {
		t.Fatalf("issueConvertSubnetToL1Tx() validators = %#v, want %#v", issuer.gotValidators, validators)
	}
}

func TestIssueCreateChainTx(t *testing.T) {
	cfg := CreateChainConfig{
		SubnetID:  ids.GenerateTestID(),
		Genesis:   []byte{0x01, 0x02, 0x03},
		VMID:      ids.GenerateTestID(),
		FxIDs:     []ids.ID{ids.GenerateTestID(), ids.GenerateTestID()},
		ChainName: "unit-chain",
	}
	txID := ids.GenerateTestID()

	issuer := &stubCreateChainTxIssuer{tx: &txs.Tx{TxID: txID}}
	gotTxID, err := issueCreateChainTx(
		issuer,
		cfg,
	)
	if err != nil {
		t.Fatalf("issueCreateChainTx() returned error: %v", err)
	}
	if gotTxID != txID {
		t.Fatalf("issueCreateChainTx() txID = %s, want %s", gotTxID, txID)
	}
	gotCfg := issuer.gotCfg
	if gotCfg.SubnetID != cfg.SubnetID || gotCfg.VMID != cfg.VMID || gotCfg.ChainName != cfg.ChainName {
		t.Fatalf("issueCreateChainTx() config mismatch: got %#v, want %#v", gotCfg, cfg)
	}
	if len(gotCfg.Genesis) != len(cfg.Genesis) || gotCfg.Genesis[0] != cfg.Genesis[0] {
		t.Fatalf("issueCreateChainTx() genesis mismatch: got %x, want %x", gotCfg.Genesis, cfg.Genesis)
	}
	if len(gotCfg.FxIDs) != len(cfg.FxIDs) || gotCfg.FxIDs[0] != cfg.FxIDs[0] || gotCfg.FxIDs[1] != cfg.FxIDs[1] {
		t.Fatalf("issueCreateChainTx() fxIDs mismatch: got %#v, want %#v", gotCfg.FxIDs, cfg.FxIDs)
	}
}
