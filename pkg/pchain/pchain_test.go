package pchain

import (
	"bytes"
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
	stakeOuts := []*avax.TransferableOutput{{
		Asset: avax.Asset{ID: assetID},
		Out: &secp256k1fx.TransferOutput{
			Amt: cfg.StakeAmt,
		},
	}}

	var gotNodeID ids.NodeID
	var gotWeight uint64
	var gotSigner signer.Signer
	var gotAssetID ids.ID
	var gotValidationRewardsOwner *secp256k1fx.OutputOwners
	var gotDelegationRewardsOwner *secp256k1fx.OutputOwners
	var gotConfigOwner *secp256k1fx.OutputOwners
	var gotDelegationShares uint32
	var gotAutoCompoundShares uint32
	var gotPeriodSeconds uint64
	var gotUnsignedTx txs.UnsignedTx
	gotTxID, err := issueAddAutoRenewedValidatorTx(
		func(
			validatorNodeID ids.NodeID,
			weight uint64,
			signer signer.Signer,
			assetID ids.ID,
			validationRewardsOwner *secp256k1fx.OutputOwners,
			delegationRewardsOwner *secp256k1fx.OutputOwners,
			configOwner *secp256k1fx.OutputOwners,
			shares uint32,
			autoCompoundRewardShares uint32,
			periodSeconds uint64,
			_ ...common.Option,
		) (*txs.AddAutoRenewedValidatorTx, error) {
			gotNodeID = validatorNodeID
			gotWeight = weight
			gotSigner = signer
			gotAssetID = assetID
			gotValidationRewardsOwner = validationRewardsOwner
			gotDelegationRewardsOwner = delegationRewardsOwner
			gotConfigOwner = configOwner
			gotDelegationShares = shares
			gotAutoCompoundShares = autoCompoundRewardShares
			gotPeriodSeconds = periodSeconds
			return &txs.AddAutoRenewedValidatorTx{
				BaseTx: txs.BaseTx{BaseTx: avax.BaseTx{
					NetworkID: 1,
				}},
				ValidatorNodeID:          cfg.NodeID[:],
				Signer:                   signer,
				StakeOuts:                stakeOuts,
				ValidatorRewardsOwner:    validationRewardsOwner,
				DelegatorRewardsOwner:    delegationRewardsOwner,
				ValidatorAuthority:       configOwner,
				DelegationShares:         shares,
				AutoCompoundRewardShares: autoCompoundRewardShares,
				Period:                   periodSeconds,
			}, nil
		},
		func(utx txs.UnsignedTx, _ ...common.Option) (*txs.Tx, error) {
			gotUnsignedTx = utx
			return &txs.Tx{TxID: txID}, nil
		},
		assetID,
		cfg,
	)
	if err != nil {
		t.Fatalf("issueAddAutoRenewedValidatorTx() returned error: %v", err)
	}
	if gotTxID != txID {
		t.Fatalf("issueAddAutoRenewedValidatorTx() txID = %s, want %s", gotTxID, txID)
	}
	if gotNodeID != cfg.NodeID {
		t.Fatalf("issueAddAutoRenewedValidatorTx() builder nodeID = %s, want %s", gotNodeID, cfg.NodeID)
	}
	if gotWeight != cfg.StakeAmt {
		t.Fatalf("issueAddAutoRenewedValidatorTx() builder weight = %d, want %d", gotWeight, cfg.StakeAmt)
	}
	gotPop, ok := gotSigner.(*signer.ProofOfPossession)
	if !ok {
		t.Fatalf("issueAddAutoRenewedValidatorTx() builder signer type = %T, want *signer.ProofOfPossession", gotSigner)
	}
	if gotPop != pop {
		t.Fatal("issueAddAutoRenewedValidatorTx() builder signer pointer mismatch")
	}
	if gotAssetID != assetID {
		t.Fatalf("issueAddAutoRenewedValidatorTx() builder assetID = %s, want %s", gotAssetID, assetID)
	}
	if gotValidationRewardsOwner == nil || len(gotValidationRewardsOwner.Addrs) != 1 || gotValidationRewardsOwner.Addrs[0] != rewardAddr {
		t.Fatalf("issueAddAutoRenewedValidatorTx() builder validation owner addrs = %#v, want [%s]", gotValidationRewardsOwner, rewardAddr)
	}
	if gotValidationRewardsOwner.Locktime != 0 || gotValidationRewardsOwner.Threshold != 1 {
		t.Fatalf("issueAddAutoRenewedValidatorTx() builder validation owner locktime/threshold = %d/%d, want 0/1", gotValidationRewardsOwner.Locktime, gotValidationRewardsOwner.Threshold)
	}
	if gotDelegationRewardsOwner == nil || len(gotDelegationRewardsOwner.Addrs) != 1 || gotDelegationRewardsOwner.Addrs[0] != rewardAddr {
		t.Fatalf("issueAddAutoRenewedValidatorTx() builder delegation owner addrs = %#v, want [%s]", gotDelegationRewardsOwner, rewardAddr)
	}
	if gotDelegationRewardsOwner.Locktime != 0 || gotDelegationRewardsOwner.Threshold != 1 {
		t.Fatalf("issueAddAutoRenewedValidatorTx() builder delegation owner locktime/threshold = %d/%d, want 0/1", gotDelegationRewardsOwner.Locktime, gotDelegationRewardsOwner.Threshold)
	}
	if gotDelegationShares != cfg.DelegationFee {
		t.Fatalf("issueAddAutoRenewedValidatorTx() builder delegation shares = %d, want %d", gotDelegationShares, cfg.DelegationFee)
	}
	if gotAutoCompoundShares != cfg.AutoCompoundRewardShares {
		t.Fatalf("issueAddAutoRenewedValidatorTx() builder auto-compound shares = %d, want %d", gotAutoCompoundShares, cfg.AutoCompoundRewardShares)
	}
	if gotPeriodSeconds != uint64(cfg.Period/time.Second) {
		t.Fatalf("issueAddAutoRenewedValidatorTx() builder period seconds = %d, want %d", gotPeriodSeconds, uint64(cfg.Period/time.Second))
	}
	if gotConfigOwner == nil || len(gotConfigOwner.Addrs) != 1 || gotConfigOwner.Addrs[0] != authorityAddr {
		t.Fatalf("issueAddAutoRenewedValidatorTx() builder authority addrs = %#v, want [%s]", gotConfigOwner, authorityAddr)
	}

	autoTx, ok := gotUnsignedTx.(*txs.AddAutoRenewedValidatorTx)
	if !ok {
		t.Fatalf("issueAddAutoRenewedValidatorTx() unsigned type = %T, want *txs.AddAutoRenewedValidatorTx", gotUnsignedTx)
	}
	if !bytes.Equal(autoTx.ValidatorNodeID, cfg.NodeID.Bytes()) {
		t.Fatalf("issueAddAutoRenewedValidatorTx() nodeID bytes = %x, want %x", []byte(autoTx.ValidatorNodeID), cfg.NodeID.Bytes())
	}
	if autoTx.Signer != pop {
		t.Fatal("issueAddAutoRenewedValidatorTx() auto-renew signer pointer mismatch")
	}
	if len(autoTx.StakeOuts) != 1 || autoTx.StakeOuts[0] != stakeOuts[0] {
		t.Fatalf("issueAddAutoRenewedValidatorTx() stake outs = %#v, want builder stake outs", autoTx.StakeOuts)
	}
	if autoTx.ValidatorRewardsOwner != gotValidationRewardsOwner {
		t.Fatal("issueAddAutoRenewedValidatorTx() validation rewards owner was not reused")
	}
	if autoTx.DelegatorRewardsOwner != gotDelegationRewardsOwner {
		t.Fatal("issueAddAutoRenewedValidatorTx() delegation rewards owner was not reused")
	}
	validatorAuthority, ok := autoTx.ValidatorAuthority.(*secp256k1fx.OutputOwners)
	if !ok {
		t.Fatalf("issueAddAutoRenewedValidatorTx() authority type = %T, want *secp256k1fx.OutputOwners", autoTx.ValidatorAuthority)
	}
	if len(validatorAuthority.Addrs) != 1 || validatorAuthority.Addrs[0] != authorityAddr {
		t.Fatalf("issueAddAutoRenewedValidatorTx() authority addrs = %#v, want [%s]", validatorAuthority.Addrs, authorityAddr)
	}
	if validatorAuthority.Locktime != 0 || validatorAuthority.Threshold != 1 {
		t.Fatalf("issueAddAutoRenewedValidatorTx() authority locktime/threshold = %d/%d, want 0/1", validatorAuthority.Locktime, validatorAuthority.Threshold)
	}
	if autoTx.DelegationShares != cfg.DelegationFee {
		t.Fatalf("issueAddAutoRenewedValidatorTx() delegation shares = %d, want %d", autoTx.DelegationShares, cfg.DelegationFee)
	}
	if autoTx.AutoCompoundRewardShares != cfg.AutoCompoundRewardShares {
		t.Fatalf("issueAddAutoRenewedValidatorTx() auto-compound shares = %d, want %d", autoTx.AutoCompoundRewardShares, cfg.AutoCompoundRewardShares)
	}
	if autoTx.Period != uint64(cfg.Period/time.Second) {
		t.Fatalf("issueAddAutoRenewedValidatorTx() period seconds = %d, want %d", autoTx.Period, uint64(cfg.Period/time.Second))
	}

	initializedTx := &txs.Tx{Unsigned: autoTx}
	if err := initializedTx.Initialize(txs.Codec); err != nil {
		t.Fatalf("issueAddAutoRenewedValidatorTx() failed to initialize tx: %v", err)
	}
	parsedTx, err := txs.Parse(txs.Codec, initializedTx.Bytes())
	if err != nil {
		t.Fatalf("issueAddAutoRenewedValidatorTx() failed to parse initialized tx: %v", err)
	}
	if _, ok := parsedTx.Unsigned.(*txs.AddAutoRenewedValidatorTx); !ok {
		t.Fatalf("issueAddAutoRenewedValidatorTx() parsed unsigned type = %T, want *txs.AddAutoRenewedValidatorTx", parsedTx.Unsigned)
	}
}

func TestIssueSetAutoRenewedValidatorConfigTx(t *testing.T) {
	validatorTxID := ids.GenerateTestID()
	issuedTxID := ids.GenerateTestID()
	auth := &secp256k1fx.Input{SigIndices: []uint32{0}}
	cfg := SetAutoRenewedValidatorConfigTxConfig{
		TxID:                     validatorTxID,
		AutoCompoundRewardShares: 250_000,
		Period:                   7 * 24 * time.Hour,
	}

	var gotBuilderTxID ids.ID
	var gotBuilderAutoCompoundShares uint32
	var gotBuilderPeriodSeconds uint64
	var gotUnsignedTx txs.UnsignedTx
	gotTxID, err := issueSetAutoRenewedValidatorConfigTx(
		func(txID ids.ID, autoCompoundRewardShares uint32, periodSeconds uint64, _ ...common.Option) (*txs.SetAutoRenewedValidatorConfigTx, error) {
			gotBuilderTxID = txID
			gotBuilderAutoCompoundShares = autoCompoundRewardShares
			gotBuilderPeriodSeconds = periodSeconds
			return &txs.SetAutoRenewedValidatorConfigTx{
				BaseTx: txs.BaseTx{BaseTx: avax.BaseTx{
					NetworkID: 1,
				}},
				TxID:                     txID,
				Auth:                     auth,
				AutoCompoundRewardShares: autoCompoundRewardShares,
				Period:                   periodSeconds,
			}, nil
		},
		func(utx txs.UnsignedTx, _ ...common.Option) (*txs.Tx, error) {
			gotUnsignedTx = utx
			return &txs.Tx{TxID: issuedTxID}, nil
		},
		cfg,
	)
	if err != nil {
		t.Fatalf("issueSetAutoRenewedValidatorConfigTx() returned error: %v", err)
	}
	if gotTxID != issuedTxID {
		t.Fatalf("issueSetAutoRenewedValidatorConfigTx() txID = %s, want %s", gotTxID, issuedTxID)
	}
	if gotBuilderTxID != validatorTxID {
		t.Fatalf("issueSetAutoRenewedValidatorConfigTx() builder txID = %s, want %s", gotBuilderTxID, validatorTxID)
	}
	if gotBuilderAutoCompoundShares != cfg.AutoCompoundRewardShares {
		t.Fatalf("issueSetAutoRenewedValidatorConfigTx() builder auto-compound shares = %d, want %d", gotBuilderAutoCompoundShares, cfg.AutoCompoundRewardShares)
	}
	if gotBuilderPeriodSeconds != uint64(cfg.Period/time.Second) {
		t.Fatalf("issueSetAutoRenewedValidatorConfigTx() builder period seconds = %d, want %d", gotBuilderPeriodSeconds, uint64(cfg.Period/time.Second))
	}
	setTx, ok := gotUnsignedTx.(*txs.SetAutoRenewedValidatorConfigTx)
	if !ok {
		t.Fatalf("issueSetAutoRenewedValidatorConfigTx() unsigned type = %T, want *txs.SetAutoRenewedValidatorConfigTx", gotUnsignedTx)
	}
	if setTx.TxID != validatorTxID {
		t.Fatalf("issueSetAutoRenewedValidatorConfigTx() validator txID = %s, want %s", setTx.TxID, validatorTxID)
	}
	if setTx.Auth != auth {
		t.Fatal("issueSetAutoRenewedValidatorConfigTx() auth pointer mismatch")
	}
	if setTx.AutoCompoundRewardShares != cfg.AutoCompoundRewardShares {
		t.Fatalf("issueSetAutoRenewedValidatorConfigTx() auto-compound shares = %d, want %d", setTx.AutoCompoundRewardShares, cfg.AutoCompoundRewardShares)
	}
	if setTx.Period != uint64(cfg.Period/time.Second) {
		t.Fatalf("issueSetAutoRenewedValidatorConfigTx() period seconds = %d, want %d", setTx.Period, uint64(cfg.Period/time.Second))
	}

	initializedTx := &txs.Tx{Unsigned: setTx}
	if err := initializedTx.Initialize(txs.Codec); err != nil {
		t.Fatalf("issueSetAutoRenewedValidatorConfigTx() failed to initialize tx: %v", err)
	}
	parsedTx, err := txs.Parse(txs.Codec, initializedTx.Bytes())
	if err != nil {
		t.Fatalf("issueSetAutoRenewedValidatorConfigTx() failed to parse initialized tx: %v", err)
	}
	if _, ok := parsedTx.Unsigned.(*txs.SetAutoRenewedValidatorConfigTx); !ok {
		t.Fatalf("issueSetAutoRenewedValidatorConfigTx() parsed unsigned type = %T, want *txs.SetAutoRenewedValidatorConfigTx", parsedTx.Unsigned)
	}
}

func TestIssueSetAutoRenewedValidatorConfigTxAllowsZeroPeriod(t *testing.T) {
	cfg := SetAutoRenewedValidatorConfigTxConfig{
		TxID:                     ids.GenerateTestID(),
		AutoCompoundRewardShares: 0,
		Period:                   0,
	}

	var gotPeriodSeconds uint64
	_, err := issueSetAutoRenewedValidatorConfigTx(
		func(txID ids.ID, autoCompoundRewardShares uint32, periodSeconds uint64, _ ...common.Option) (*txs.SetAutoRenewedValidatorConfigTx, error) {
			return &txs.SetAutoRenewedValidatorConfigTx{
				TxID:                     txID,
				Auth:                     &secp256k1fx.Input{},
				AutoCompoundRewardShares: autoCompoundRewardShares,
				Period:                   periodSeconds,
			}, nil
		},
		func(utx txs.UnsignedTx, _ ...common.Option) (*txs.Tx, error) {
			setTx, ok := utx.(*txs.SetAutoRenewedValidatorConfigTx)
			if !ok {
				t.Fatalf("issueSetAutoRenewedValidatorConfigTx() unsigned type = %T, want *txs.SetAutoRenewedValidatorConfigTx", utx)
			}
			gotPeriodSeconds = setTx.Period
			return &txs.Tx{TxID: ids.GenerateTestID()}, nil
		},
		cfg,
	)
	if err != nil {
		t.Fatalf("issueSetAutoRenewedValidatorConfigTx() returned error: %v", err)
	}
	if gotPeriodSeconds != 0 {
		t.Fatalf("issueSetAutoRenewedValidatorConfigTx() period seconds = %d, want 0", gotPeriodSeconds)
	}
}

func TestIssueAutoRenewedValidatorTxRejectsInvalidPeriods(t *testing.T) {
	_, err := issueAddAutoRenewedValidatorTx(
		func(ids.NodeID, uint64, signer.Signer, ids.ID, *secp256k1fx.OutputOwners, *secp256k1fx.OutputOwners, *secp256k1fx.OutputOwners, uint32, uint32, uint64, ...common.Option) (*txs.AddAutoRenewedValidatorTx, error) {
			t.Fatal("builder should not be called for invalid add-auto period")
			return nil, nil
		},
		func(txs.UnsignedTx, ...common.Option) (*txs.Tx, error) {
			t.Fatal("issuer should not be called for invalid add-auto period")
			return nil, nil
		},
		ids.GenerateTestID(),
		AddAutoRenewedValidatorConfig{Period: 1500 * time.Millisecond},
	)
	if err == nil || !strings.Contains(err.Error(), "period must be a whole number of seconds") {
		t.Fatalf("add-auto invalid period error = %v", err)
	}

	_, err = issueSetAutoRenewedValidatorConfigTx(
		func(ids.ID, uint32, uint64, ...common.Option) (*txs.SetAutoRenewedValidatorConfigTx, error) {
			t.Fatal("builder should not be called for invalid set-auto period")
			return nil, nil
		},
		func(txs.UnsignedTx, ...common.Option) (*txs.Tx, error) {
			t.Fatal("issuer should not be called for invalid set-auto period")
			return nil, nil
		},
		SetAutoRenewedValidatorConfigTxConfig{
			TxID:   ids.GenerateTestID(),
			Period: -time.Second,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "period cannot be negative") {
		t.Fatalf("set-auto invalid period error = %v", err)
	}
}

func TestGetAutoRenewedValidatorAuthorityUsesCurrentValidatorAuthority(t *testing.T) {
	targetTxID := ids.GenerateTestID()
	otherTxID := ids.GenerateTestID()
	validationRewardAddr := ids.GenerateTestShortID()
	delegationRewardAddr := ids.GenerateTestShortID()
	authorityAddr := ids.GenerateTestShortID()

	server := newCurrentValidatorsServer(t, []map[string]any{
		{
			"txID":               otherTxID.String(),
			"validatorAuthority": testAPIOwner(t, ids.GenerateTestShortID(), "0", "1"),
		},
		{
			"txID":                  targetTxID.String(),
			"validationRewardOwner": testAPIOwner(t, validationRewardAddr, "0", "1"),
			"delegationRewardOwner": testAPIOwner(t, delegationRewardAddr, "0", "1"),
			"validatorAuthority":    testAPIOwner(t, authorityAddr, "0", "1"),
		},
	})
	defer server.Close()

	owner, err := GetAutoRenewedValidatorAuthority(context.Background(), server.URL, targetTxID)
	if err != nil {
		t.Fatalf("GetAutoRenewedValidatorAuthority() returned error: %v", err)
	}
	if owner.Locktime != 0 || owner.Threshold != 1 {
		t.Fatalf("authority locktime/threshold = %d/%d, want 0/1", owner.Locktime, owner.Threshold)
	}
	if len(owner.Addrs) != 1 || owner.Addrs[0] != authorityAddr {
		t.Fatalf("authority addrs = %#v, want [%s]", owner.Addrs, authorityAddr)
	}
	if owner.Addrs[0] == validationRewardAddr || owner.Addrs[0] == delegationRewardAddr {
		t.Fatal("authority lookup used a reward owner instead of validatorAuthority")
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
			server := newCurrentValidatorsServer(t, tt.validators)
			defer server.Close()

			_, err := GetAutoRenewedValidatorAuthority(context.Background(), server.URL, targetTxID)
			if err == nil {
				t.Fatalf("GetAutoRenewedValidatorAuthority() expected error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func newCurrentValidatorsServer(t *testing.T, validators []map[string]any) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ext/P" {
			t.Fatalf("request path = %q, want /ext/P", r.URL.Path)
		}
		var req struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			ID     any             `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode JSON-RPC request: %v", err)
		}
		if req.Method != "platform.getCurrentValidators" {
			t.Fatalf("method = %q, want platform.getCurrentValidators", req.Method)
		}
		if string(req.Params) != "{}" {
			t.Fatalf("params = %s, want {}", req.Params)
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
