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

func TestIssueSendTx(t *testing.T) {
	assetID := ids.GenerateTestID()
	dest := ids.GenerateTestShortID()
	amount := uint64(42_000)
	txID := ids.GenerateTestID()

	var captured []*avax.TransferableOutput
	gotTxID, err := issueSendTx(
		func(outputs []*avax.TransferableOutput, _ ...common.Option) (*txs.Tx, error) {
			captured = outputs
			return &txs.Tx{TxID: txID}, nil
		},
		assetID,
		dest,
		amount,
	)
	if err != nil {
		t.Fatalf("issueSendTx() returned error: %v", err)
	}
	if gotTxID != txID {
		t.Fatalf("issueSendTx() txID = %s, want %s", gotTxID, txID)
	}

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
	_, err := issueSendTx(
		func(_ []*avax.TransferableOutput, opts ...common.Option) (*txs.Tx, error) {
			gotCtx := common.NewOptions(opts).Context()
			if gotCtx.Value(testContextKey("key")) != "value" {
				t.Fatalf("issueSendTx() context option not propagated")
			}
			return &txs.Tx{TxID: ids.GenerateTestID()}, nil
		},
		ids.GenerateTestID(),
		ids.GenerateTestShortID(),
		1,
		common.WithContext(ctx),
	)
	if err != nil {
		t.Fatalf("issueSendTx() returned error: %v", err)
	}
}

func TestIssueSendTxError(t *testing.T) {
	expectedErr := errors.New("boom")
	_, err := issueSendTx(
		func(_ []*avax.TransferableOutput, _ ...common.Option) (*txs.Tx, error) {
			return nil, expectedErr
		},
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

	var gotChainID ids.ID
	var captured []*avax.TransferableOutput
	gotTxID, err := issueExportTx(
		func(chainID ids.ID, outputs []*avax.TransferableOutput, _ ...common.Option) (*txs.Tx, error) {
			gotChainID = chainID
			captured = outputs
			return &txs.Tx{TxID: txID}, nil
		},
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
	if gotChainID != destChainID {
		t.Fatalf("issueExportTx() chainID = %s, want %s", gotChainID, destChainID)
	}
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

	var gotChainID ids.ID
	var gotOwners *secp256k1fx.OutputOwners
	gotTxID, err := issueImportTx(
		func(chainID ids.ID, to *secp256k1fx.OutputOwners, _ ...common.Option) (*txs.Tx, error) {
			gotChainID = chainID
			gotOwners = to
			return &txs.Tx{TxID: txID}, nil
		},
		sourceChainID,
		owner,
	)
	if err != nil {
		t.Fatalf("issueImportTx() returned error: %v", err)
	}
	if gotTxID != txID {
		t.Fatalf("issueImportTx() txID = %s, want %s", gotTxID, txID)
	}
	if gotChainID != sourceChainID {
		t.Fatalf("issueImportTx() chainID = %s, want %s", gotChainID, sourceChainID)
	}
	if gotOwners == nil {
		t.Fatal("issueImportTx() owners is nil")
	}
	if len(gotOwners.Addrs) != 1 || gotOwners.Addrs[0] != owner {
		t.Fatalf("issueImportTx() owner addrs = %#v, want [%s]", gotOwners.Addrs, owner)
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

	var gotVdr *txs.SubnetValidator
	var gotSigner signer.Signer
	var gotAssetID ids.ID
	var gotValidationRewardsOwner *secp256k1fx.OutputOwners
	var gotDelegationRewardsOwner *secp256k1fx.OutputOwners
	var gotShares uint32
	gotTxID, err := issueAddPermissionlessValidatorTx(
		func(
			vdr *txs.SubnetValidator,
			signer signer.Signer,
			assetID ids.ID,
			validationRewardsOwner *secp256k1fx.OutputOwners,
			delegationRewardsOwner *secp256k1fx.OutputOwners,
			shares uint32,
			_ ...common.Option,
		) (*txs.Tx, error) {
			gotVdr = vdr
			gotSigner = signer
			gotAssetID = assetID
			gotValidationRewardsOwner = validationRewardsOwner
			gotDelegationRewardsOwner = delegationRewardsOwner
			gotShares = shares
			return &txs.Tx{TxID: txID}, nil
		},
		assetID,
		cfg,
	)
	if err != nil {
		t.Fatalf("issueAddPermissionlessValidatorTx() returned error: %v", err)
	}
	if gotTxID != txID {
		t.Fatalf("issueAddPermissionlessValidatorTx() txID = %s, want %s", gotTxID, txID)
	}
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
	gotPop, ok := gotSigner.(*signer.ProofOfPossession)
	if !ok {
		t.Fatalf("issueAddPermissionlessValidatorTx() signer type = %T, want *signer.ProofOfPossession", gotSigner)
	}
	if gotPop != pop {
		t.Fatal("issueAddPermissionlessValidatorTx() signer pointer mismatch")
	}
	if gotAssetID != assetID {
		t.Fatalf("issueAddPermissionlessValidatorTx() assetID = %s, want %s", gotAssetID, assetID)
	}
	if gotValidationRewardsOwner == nil || len(gotValidationRewardsOwner.Addrs) != 1 || gotValidationRewardsOwner.Addrs[0] != rewardAddr {
		t.Fatalf("issueAddPermissionlessValidatorTx() validation owner addrs = %#v, want [%s]", gotValidationRewardsOwner, rewardAddr)
	}
	if gotDelegationRewardsOwner == nil || len(gotDelegationRewardsOwner.Addrs) != 1 || gotDelegationRewardsOwner.Addrs[0] != rewardAddr {
		t.Fatalf("issueAddPermissionlessValidatorTx() delegation owner addrs = %#v, want [%s]", gotDelegationRewardsOwner, rewardAddr)
	}
	if gotShares != cfg.DelegationFee {
		t.Fatalf("issueAddPermissionlessValidatorTx() shares = %d, want %d", gotShares, cfg.DelegationFee)
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

	var gotVdr *txs.SubnetValidator
	var gotAssetID ids.ID
	var gotRewardsOwner *secp256k1fx.OutputOwners
	gotTxID, err := issueAddPermissionlessDelegatorTx(
		func(
			vdr *txs.SubnetValidator,
			assetID ids.ID,
			rewardsOwner *secp256k1fx.OutputOwners,
			_ ...common.Option,
		) (*txs.Tx, error) {
			gotVdr = vdr
			gotAssetID = assetID
			gotRewardsOwner = rewardsOwner
			return &txs.Tx{TxID: txID}, nil
		},
		assetID,
		cfg,
	)
	if err != nil {
		t.Fatalf("issueAddPermissionlessDelegatorTx() returned error: %v", err)
	}
	if gotTxID != txID {
		t.Fatalf("issueAddPermissionlessDelegatorTx() txID = %s, want %s", gotTxID, txID)
	}
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
	if gotAssetID != assetID {
		t.Fatalf("issueAddPermissionlessDelegatorTx() assetID = %s, want %s", gotAssetID, assetID)
	}
	if gotRewardsOwner == nil || len(gotRewardsOwner.Addrs) != 1 || gotRewardsOwner.Addrs[0] != rewardAddr {
		t.Fatalf("issueAddPermissionlessDelegatorTx() rewards owner addrs = %#v, want [%s]", gotRewardsOwner, rewardAddr)
	}
}

func TestIssueCreateSubnetTx(t *testing.T) {
	owner := ids.GenerateTestShortID()
	txID := ids.GenerateTestID()

	var gotOwner *secp256k1fx.OutputOwners
	gotTxID, err := issueCreateSubnetTx(
		func(owner *secp256k1fx.OutputOwners, _ ...common.Option) (*txs.Tx, error) {
			gotOwner = owner
			return &txs.Tx{TxID: txID}, nil
		},
		owner,
	)
	if err != nil {
		t.Fatalf("issueCreateSubnetTx() returned error: %v", err)
	}
	if gotTxID != txID {
		t.Fatalf("issueCreateSubnetTx() txID = %s, want %s", gotTxID, txID)
	}
	if gotOwner == nil || len(gotOwner.Addrs) != 1 || gotOwner.Addrs[0] != owner {
		t.Fatalf("issueCreateSubnetTx() owner addrs = %#v, want [%s]", gotOwner, owner)
	}
}

func TestIssueTransferSubnetOwnershipTx(t *testing.T) {
	subnetID := ids.GenerateTestID()
	newOwner := ids.GenerateTestShortID()
	txID := ids.GenerateTestID()

	var gotSubnetID ids.ID
	var gotOwner *secp256k1fx.OutputOwners
	gotTxID, err := issueTransferSubnetOwnershipTx(
		func(subnetID ids.ID, owner *secp256k1fx.OutputOwners, _ ...common.Option) (*txs.Tx, error) {
			gotSubnetID = subnetID
			gotOwner = owner
			return &txs.Tx{TxID: txID}, nil
		},
		subnetID,
		newOwner,
	)
	if err != nil {
		t.Fatalf("issueTransferSubnetOwnershipTx() returned error: %v", err)
	}
	if gotTxID != txID {
		t.Fatalf("issueTransferSubnetOwnershipTx() txID = %s, want %s", gotTxID, txID)
	}
	if gotSubnetID != subnetID {
		t.Fatalf("issueTransferSubnetOwnershipTx() subnetID = %s, want %s", gotSubnetID, subnetID)
	}
	if gotOwner == nil || len(gotOwner.Addrs) != 1 || gotOwner.Addrs[0] != newOwner {
		t.Fatalf("issueTransferSubnetOwnershipTx() owner addrs = %#v, want [%s]", gotOwner, newOwner)
	}
}

func TestIssueConvertSubnetToL1Tx(t *testing.T) {
	subnetID := ids.GenerateTestID()
	chainID := ids.GenerateTestID()
	managerAddr := []byte{0x01, 0x02}
	validators := []*txs.ConvertSubnetToL1Validator{{NodeID: []byte{0xAA}, Weight: 1}}
	txID := ids.GenerateTestID()

	var gotSubnetID ids.ID
	var gotChainID ids.ID
	var gotManagerAddr []byte
	var gotValidators []*txs.ConvertSubnetToL1Validator
	gotTxID, err := issueConvertSubnetToL1Tx(
		func(
			subnetID ids.ID,
			chainID ids.ID,
			address []byte,
			validators []*txs.ConvertSubnetToL1Validator,
			_ ...common.Option,
		) (*txs.Tx, error) {
			gotSubnetID = subnetID
			gotChainID = chainID
			gotManagerAddr = address
			gotValidators = validators
			return &txs.Tx{TxID: txID}, nil
		},
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
	if gotSubnetID != subnetID || gotChainID != chainID {
		t.Fatalf("issueConvertSubnetToL1Tx() IDs = (%s,%s), want (%s,%s)", gotSubnetID, gotChainID, subnetID, chainID)
	}
	if len(gotManagerAddr) != len(managerAddr) || gotManagerAddr[0] != managerAddr[0] || gotManagerAddr[1] != managerAddr[1] {
		t.Fatalf("issueConvertSubnetToL1Tx() managerAddr = %x, want %x", gotManagerAddr, managerAddr)
	}
	if len(gotValidators) != 1 || gotValidators[0] != validators[0] {
		t.Fatalf("issueConvertSubnetToL1Tx() validators = %#v, want %#v", gotValidators, validators)
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

	var gotCfg CreateChainConfig
	gotTxID, err := issueCreateChainTx(
		func(
			subnetID ids.ID,
			genesis []byte,
			vmID ids.ID,
			fxIDs []ids.ID,
			chainName string,
			_ ...common.Option,
		) (*txs.Tx, error) {
			gotCfg = CreateChainConfig{
				SubnetID:  subnetID,
				Genesis:   genesis,
				VMID:      vmID,
				FxIDs:     fxIDs,
				ChainName: chainName,
			}
			return &txs.Tx{TxID: txID}, nil
		},
		cfg,
	)
	if err != nil {
		t.Fatalf("issueCreateChainTx() returned error: %v", err)
	}
	if gotTxID != txID {
		t.Fatalf("issueCreateChainTx() txID = %s, want %s", gotTxID, txID)
	}
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
