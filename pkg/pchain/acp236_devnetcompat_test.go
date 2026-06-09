//go:build devnetcompat

package pchain

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/utils/constants"
	blslocalsigner "github.com/ava-labs/avalanchego/utils/crypto/bls/signer/localsigner"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/platformvm/reward"
	"github.com/ava-labs/avalanchego/vms/platformvm/signer"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
	"github.com/ava-labs/avalanchego/wallet/subnet/primary/common"
)

func TestDevnetCompatAddAutoRenewedValidatorTx(t *testing.T) {
	ctx := devnetCompatContext()
	nodeID := ids.GenerateTestNodeID()
	rewardAddr := ids.GenerateTestShortID()
	authorityAddr := ids.GenerateTestShortID()
	pop := newDevnetCompatPoP(t)
	stakeOuts := []*avax.TransferableOutput{
		devnetCompatStakeOut(ctx, 2_000_000_000_000, rewardAddr),
	}
	cfg := AddAutoRenewedValidatorConfig{
		NodeID:                   nodeID,
		StakeAmt:                 2_000_000_000_000,
		RewardAddr:               rewardAddr,
		ValidatorAuthorityAddr:   authorityAddr,
		DelegationFee:            20_000,
		AutoCompoundRewardShares: 750_000,
		Period:                   14 * 24 * time.Hour,
		BLSSigner:                pop,
	}
	callCtx := context.WithValue(context.Background(), testContextKey("devnetcompat"), "add")

	var issuedTx *txs.Tx
	gotTxID, err := issueAddAutoRenewedValidatorTx(
		func(
			validatorNodeID ids.NodeID,
			weight uint64,
			gotSigner signer.Signer,
			assetID ids.ID,
			validationRewardsOwner *secp256k1fx.OutputOwners,
			delegationRewardsOwner *secp256k1fx.OutputOwners,
			configOwner *secp256k1fx.OutputOwners,
			shares uint32,
			autoCompoundRewardShares uint32,
			periodSeconds uint64,
			options ...common.Option,
		) (*txs.AddAutoRenewedValidatorTx, error) {
			if common.NewOptions(options).Context().Value(testContextKey("devnetcompat")) != "add" {
				t.Fatal("context option was not passed to add-auto-renewed builder")
			}
			if validatorNodeID != cfg.NodeID {
				t.Fatalf("builder nodeID = %s, want %s", validatorNodeID, cfg.NodeID)
			}
			if weight != cfg.StakeAmt {
				t.Fatalf("builder weight = %d, want %d", weight, cfg.StakeAmt)
			}
			if gotSigner != pop {
				t.Fatal("builder BLS proof pointer mismatch")
			}
			if assetID != ctx.AVAXAssetID {
				t.Fatalf("builder assetID = %s, want %s", assetID, ctx.AVAXAssetID)
			}
			if !ownerHasOnly(validationRewardsOwner, rewardAddr) {
				t.Fatalf("validation reward owner = %#v, want [%s]", validationRewardsOwner, rewardAddr)
			}
			if !ownerHasOnly(delegationRewardsOwner, rewardAddr) {
				t.Fatalf("delegation reward owner = %#v, want [%s]", delegationRewardsOwner, rewardAddr)
			}
			if shares != cfg.DelegationFee {
				t.Fatalf("builder delegation shares = %d, want %d", shares, cfg.DelegationFee)
			}
			if !ownerHasOnly(configOwner, authorityAddr) {
				t.Fatalf("validator authority = %#v, want [%s]", configOwner, authorityAddr)
			}
			if autoCompoundRewardShares != cfg.AutoCompoundRewardShares {
				t.Fatalf("builder auto-compound shares = %d, want %d", autoCompoundRewardShares, cfg.AutoCompoundRewardShares)
			}
			if periodSeconds != uint64(cfg.Period/time.Second) {
				t.Fatalf("builder period seconds = %d, want %d", periodSeconds, uint64(cfg.Period/time.Second))
			}

			return &txs.AddAutoRenewedValidatorTx{
				BaseTx:                   devnetCompatBaseTx(ctx),
				ValidatorNodeID:          validatorNodeID[:],
				Signer:                   gotSigner,
				StakeOuts:                stakeOuts,
				ValidatorRewardsOwner:    validationRewardsOwner,
				DelegatorRewardsOwner:    delegationRewardsOwner,
				ValidatorAuthority:       configOwner,
				DelegationShares:         shares,
				AutoCompoundRewardShares: autoCompoundRewardShares,
				Period:                   periodSeconds,
			}, nil
		},
		func(utx txs.UnsignedTx, options ...common.Option) (*txs.Tx, error) {
			if common.NewOptions(options).Context().Value(testContextKey("devnetcompat")) != "add" {
				t.Fatal("context option was not passed to add-auto-renewed issuer")
			}

			autoTx, ok := utx.(*txs.AddAutoRenewedValidatorTx)
			if !ok {
				t.Fatalf("unsigned tx type = %T, want *txs.AddAutoRenewedValidatorTx", utx)
			}
			autoTx.InitCtx(ctx)
			if err := autoTx.SyntacticVerify(ctx); err != nil {
				t.Fatalf("AddAutoRenewedValidatorTx failed devnet syntactic verify: %v", err)
			}
			if !bytes.Equal(autoTx.ValidatorNodeID, cfg.NodeID.Bytes()) {
				t.Fatalf("nodeID bytes = %x, want %x", []byte(autoTx.ValidatorNodeID), cfg.NodeID.Bytes())
			}
			if autoTx.Signer != pop {
				t.Fatal("auto-renewed signer pointer mismatch")
			}
			if len(autoTx.StakeOuts) != 1 || autoTx.StakeOuts[0] != stakeOuts[0] {
				t.Fatalf("stake outs = %#v, want stake builder outputs", autoTx.StakeOuts)
			}
			if !ownerHasOnly(autoTx.ValidatorRewardsOwner.(*secp256k1fx.OutputOwners), rewardAddr) {
				t.Fatalf("validator reward owner = %#v, want [%s]", autoTx.ValidatorRewardsOwner, rewardAddr)
			}
			if !ownerHasOnly(autoTx.DelegatorRewardsOwner.(*secp256k1fx.OutputOwners), rewardAddr) {
				t.Fatalf("delegator reward owner = %#v, want [%s]", autoTx.DelegatorRewardsOwner, rewardAddr)
			}
			if !ownerHasOnly(autoTx.ValidatorAuthority.(*secp256k1fx.OutputOwners), authorityAddr) {
				t.Fatalf("validator authority = %#v, want [%s]", autoTx.ValidatorAuthority, authorityAddr)
			}
			if autoTx.DelegationShares != cfg.DelegationFee {
				t.Fatalf("delegation shares = %d, want %d", autoTx.DelegationShares, cfg.DelegationFee)
			}
			if autoTx.AutoCompoundRewardShares != cfg.AutoCompoundRewardShares {
				t.Fatalf("auto-compound shares = %d, want %d", autoTx.AutoCompoundRewardShares, cfg.AutoCompoundRewardShares)
			}
			if autoTx.Period != uint64(cfg.Period/time.Second) {
				t.Fatalf("period seconds = %d, want %d", autoTx.Period, uint64(cfg.Period/time.Second))
			}

			issuedTx = initializeDevnetCompatTx(t, autoTx)
			parsed, err := txs.Parse(txs.Codec, issuedTx.Bytes())
			if err != nil {
				t.Fatalf("failed to parse serialized AddAutoRenewedValidatorTx: %v", err)
			}
			if _, ok := parsed.Unsigned.(*txs.AddAutoRenewedValidatorTx); !ok {
				t.Fatalf("parsed unsigned tx type = %T, want *txs.AddAutoRenewedValidatorTx", parsed.Unsigned)
			}
			return issuedTx, nil
		},
		ctx.AVAXAssetID,
		cfg,
		common.WithContext(callCtx),
	)
	if err != nil {
		t.Fatalf("issueAddAutoRenewedValidatorTx returned error: %v", err)
	}
	if gotTxID != issuedTx.ID() {
		t.Fatalf("returned txID = %s, want issued tx ID %s", gotTxID, issuedTx.ID())
	}
}

func TestDevnetCompatSetAutoRenewedValidatorConfigTx(t *testing.T) {
	ctx := devnetCompatContext()
	validatorTxID := ids.GenerateTestID()
	auth := &secp256k1fx.Input{SigIndices: []uint32{0}}
	cfg := SetAutoRenewedValidatorConfigTxConfig{
		TxID:                     validatorTxID,
		AutoCompoundRewardShares: 250_000,
		Period:                   7 * 24 * time.Hour,
	}
	callCtx := context.WithValue(context.Background(), testContextKey("devnetcompat"), "set")

	var issuedTx *txs.Tx
	gotTxID, err := issueSetAutoRenewedValidatorConfigTx(
		func(txID ids.ID, autoCompoundRewardShares uint32, periodSeconds uint64, options ...common.Option) (*txs.SetAutoRenewedValidatorConfigTx, error) {
			if common.NewOptions(options).Context().Value(testContextKey("devnetcompat")) != "set" {
				t.Fatal("context option was not passed to set-auto-config builder")
			}
			if txID != validatorTxID {
				t.Fatalf("builder txID = %s, want %s", txID, validatorTxID)
			}
			if autoCompoundRewardShares != cfg.AutoCompoundRewardShares {
				t.Fatalf("builder auto-compound shares = %d, want %d", autoCompoundRewardShares, cfg.AutoCompoundRewardShares)
			}
			if periodSeconds != uint64(cfg.Period/time.Second) {
				t.Fatalf("builder period seconds = %d, want %d", periodSeconds, uint64(cfg.Period/time.Second))
			}
			return &txs.SetAutoRenewedValidatorConfigTx{
				BaseTx:                   devnetCompatBaseTx(ctx),
				TxID:                     txID,
				Auth:                     auth,
				AutoCompoundRewardShares: autoCompoundRewardShares,
				Period:                   periodSeconds,
			}, nil
		},
		func(utx txs.UnsignedTx, options ...common.Option) (*txs.Tx, error) {
			if common.NewOptions(options).Context().Value(testContextKey("devnetcompat")) != "set" {
				t.Fatal("context option was not passed to set-auto-config issuer")
			}

			setTx, ok := utx.(*txs.SetAutoRenewedValidatorConfigTx)
			if !ok {
				t.Fatalf("unsigned tx type = %T, want *txs.SetAutoRenewedValidatorConfigTx", utx)
			}
			setTx.InitCtx(ctx)
			if err := setTx.SyntacticVerify(ctx); err != nil {
				t.Fatalf("SetAutoRenewedValidatorConfigTx failed devnet syntactic verify: %v", err)
			}
			if setTx.TxID != validatorTxID {
				t.Fatalf("config txID = %s, want %s", setTx.TxID, validatorTxID)
			}
			if setTx.Auth != auth {
				t.Fatal("auth pointer mismatch")
			}
			if setTx.AutoCompoundRewardShares != cfg.AutoCompoundRewardShares {
				t.Fatalf("auto-compound shares = %d, want %d", setTx.AutoCompoundRewardShares, cfg.AutoCompoundRewardShares)
			}
			if setTx.Period != uint64(cfg.Period/time.Second) {
				t.Fatalf("period seconds = %d, want %d", setTx.Period, uint64(cfg.Period/time.Second))
			}

			issuedTx = initializeDevnetCompatTx(t, setTx)
			parsed, err := txs.Parse(txs.Codec, issuedTx.Bytes())
			if err != nil {
				t.Fatalf("failed to parse serialized SetAutoRenewedValidatorConfigTx: %v", err)
			}
			if _, ok := parsed.Unsigned.(*txs.SetAutoRenewedValidatorConfigTx); !ok {
				t.Fatalf("parsed unsigned tx type = %T, want *txs.SetAutoRenewedValidatorConfigTx", parsed.Unsigned)
			}
			return issuedTx, nil
		},
		cfg,
		common.WithContext(callCtx),
	)
	if err != nil {
		t.Fatalf("issueSetAutoRenewedValidatorConfigTx returned error: %v", err)
	}
	if gotTxID != issuedTx.ID() {
		t.Fatalf("returned txID = %s, want issued tx ID %s", gotTxID, issuedTx.ID())
	}
}

func TestDevnetCompatSetAutoRenewedValidatorConfigTxExitCycle(t *testing.T) {
	ctx := devnetCompatContext()
	cfg := SetAutoRenewedValidatorConfigTxConfig{
		TxID:                     ids.GenerateTestID(),
		AutoCompoundRewardShares: 0,
		Period:                   0,
	}

	_, err := issueSetAutoRenewedValidatorConfigTx(
		func(txID ids.ID, autoCompoundRewardShares uint32, periodSeconds uint64, _ ...common.Option) (*txs.SetAutoRenewedValidatorConfigTx, error) {
			return &txs.SetAutoRenewedValidatorConfigTx{
				BaseTx:                   devnetCompatBaseTx(ctx),
				TxID:                     txID,
				Auth:                     &secp256k1fx.Input{SigIndices: []uint32{0}},
				AutoCompoundRewardShares: autoCompoundRewardShares,
				Period:                   periodSeconds,
			}, nil
		},
		func(utx txs.UnsignedTx, _ ...common.Option) (*txs.Tx, error) {
			setTx, ok := utx.(*txs.SetAutoRenewedValidatorConfigTx)
			if !ok {
				t.Fatalf("unsigned tx type = %T, want *txs.SetAutoRenewedValidatorConfigTx", utx)
			}
			if setTx.Period != 0 {
				t.Fatalf("exit-cycle period = %d, want 0", setTx.Period)
			}
			setTx.InitCtx(ctx)
			if err := setTx.SyntacticVerify(ctx); err != nil {
				t.Fatalf("exit-cycle SetAutoRenewedValidatorConfigTx failed devnet syntactic verify: %v", err)
			}
			return initializeDevnetCompatTx(t, setTx), nil
		},
		cfg,
	)
	if err != nil {
		t.Fatalf("exit-cycle issueSetAutoRenewedValidatorConfigTx returned error: %v", err)
	}
}

func TestDevnetCompatACP236ErrorWrapping(t *testing.T) {
	ctx := devnetCompatContext()
	expectedErr := errors.New("devnet compat failure")

	_, err := issueAddAutoRenewedValidatorTx(
		func(ids.NodeID, uint64, signer.Signer, ids.ID, *secp256k1fx.OutputOwners, *secp256k1fx.OutputOwners, *secp256k1fx.OutputOwners, uint32, uint32, uint64, ...common.Option) (*txs.AddAutoRenewedValidatorTx, error) {
			return nil, expectedErr
		},
		func(txs.UnsignedTx, ...common.Option) (*txs.Tx, error) {
			t.Fatal("issuer should not be called after add-auto-renewed builder failure")
			return nil, nil
		},
		ctx.AVAXAssetID,
		AddAutoRenewedValidatorConfig{Period: time.Second},
	)
	assertWrapped(t, err, expectedErr, "failed to build AddAutoRenewedValidatorTx")

	_, err = issueAddAutoRenewedValidatorTx(
		func(ids.NodeID, uint64, signer.Signer, ids.ID, *secp256k1fx.OutputOwners, *secp256k1fx.OutputOwners, *secp256k1fx.OutputOwners, uint32, uint32, uint64, ...common.Option) (*txs.AddAutoRenewedValidatorTx, error) {
			return &txs.AddAutoRenewedValidatorTx{BaseTx: devnetCompatBaseTx(ctx)}, nil
		},
		func(txs.UnsignedTx, ...common.Option) (*txs.Tx, error) {
			return nil, expectedErr
		},
		ctx.AVAXAssetID,
		AddAutoRenewedValidatorConfig{Period: time.Second},
	)
	assertWrapped(t, err, expectedErr, "failed to issue AddAutoRenewedValidatorTx")

	_, err = issueSetAutoRenewedValidatorConfigTx(
		func(ids.ID, uint32, uint64, ...common.Option) (*txs.SetAutoRenewedValidatorConfigTx, error) {
			return nil, expectedErr
		},
		func(txs.UnsignedTx, ...common.Option) (*txs.Tx, error) {
			t.Fatal("issuer should not be called after set-auto-config auth builder failure")
			return nil, nil
		},
		SetAutoRenewedValidatorConfigTxConfig{TxID: ids.GenerateTestID()},
	)
	assertWrapped(t, err, expectedErr, "failed to build SetAutoRenewedValidatorConfigTx")

	_, err = issueSetAutoRenewedValidatorConfigTx(
		func(txID ids.ID, autoCompoundRewardShares uint32, periodSeconds uint64, _ ...common.Option) (*txs.SetAutoRenewedValidatorConfigTx, error) {
			return &txs.SetAutoRenewedValidatorConfigTx{
				BaseTx:                   devnetCompatBaseTx(ctx),
				TxID:                     txID,
				Auth:                     &secp256k1fx.Input{},
				AutoCompoundRewardShares: autoCompoundRewardShares,
				Period:                   periodSeconds,
			}, nil
		},
		func(txs.UnsignedTx, ...common.Option) (*txs.Tx, error) {
			return nil, expectedErr
		},
		SetAutoRenewedValidatorConfigTxConfig{TxID: ids.GenerateTestID()},
	)
	assertWrapped(t, err, expectedErr, "failed to issue SetAutoRenewedValidatorConfigTx")
}

func TestDevnetCompatACP236DevnetSyntacticRejections(t *testing.T) {
	ctx := devnetCompatContext()

	tooManySharesTx := &txs.AddAutoRenewedValidatorTx{
		BaseTx:                   devnetCompatBaseTx(ctx),
		ValidatorNodeID:          ids.GenerateTestNodeID().Bytes(),
		Signer:                   newDevnetCompatPoP(t),
		StakeOuts:                []*avax.TransferableOutput{devnetCompatStakeOut(ctx, 1, ids.GenerateTestShortID())},
		ValidatorRewardsOwner:    devnetCompatOwner(ids.GenerateTestShortID()),
		DelegatorRewardsOwner:    devnetCompatOwner(ids.GenerateTestShortID()),
		ValidatorAuthority:       devnetCompatOwner(ids.GenerateTestShortID()),
		AutoCompoundRewardShares: reward.PercentDenominator + 1,
		Period:                   1,
	}
	tooManySharesTx.InitCtx(ctx)
	if err := tooManySharesTx.SyntacticVerify(ctx); err == nil {
		t.Fatal("AddAutoRenewedValidatorTx expected devnet syntactic rejection for too many auto-compound shares")
	}

}

func newDevnetCompatPoP(t *testing.T) *signer.ProofOfPossession {
	t.Helper()

	sk, err := blslocalsigner.New()
	if err != nil {
		t.Fatalf("failed to create BLS secret key: %v", err)
	}
	pop, err := signer.NewProofOfPossession(sk)
	if err != nil {
		t.Fatalf("failed to create BLS proof of possession: %v", err)
	}
	return pop
}

func devnetCompatBaseTx(ctx *snow.Context) txs.BaseTx {
	return txs.BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    ctx.NetworkID,
		BlockchainID: ctx.ChainID,
	}}
}

func devnetCompatContext() *snow.Context {
	return &snow.Context{
		NetworkID:   constants.UnitTestID,
		ChainID:     constants.PlatformChainID,
		AVAXAssetID: ids.GenerateTestID(),
	}
}

func devnetCompatStakeOut(ctx *snow.Context, amount uint64, owner ids.ShortID) *avax.TransferableOutput {
	return &avax.TransferableOutput{
		Asset: avax.Asset{ID: ctx.AVAXAssetID},
		Out: &secp256k1fx.TransferOutput{
			Amt:          amount,
			OutputOwners: *devnetCompatOwner(owner),
		},
	}
}

func devnetCompatOwner(addr ids.ShortID) *secp256k1fx.OutputOwners {
	return &secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs:     []ids.ShortID{addr},
	}
}

func ownerHasOnly(owner *secp256k1fx.OutputOwners, addr ids.ShortID) bool {
	return owner != nil &&
		owner.Threshold == 1 &&
		len(owner.Addrs) == 1 &&
		owner.Addrs[0] == addr
}

func initializeDevnetCompatTx(t *testing.T, utx txs.UnsignedTx) *txs.Tx {
	t.Helper()

	tx := &txs.Tx{Unsigned: utx}
	if err := tx.Initialize(txs.Codec); err != nil {
		t.Fatalf("failed to initialize tx: %v", err)
	}
	return tx
}

func assertWrapped(t *testing.T, err, target error, message string) {
	t.Helper()

	if !errors.Is(err, target) {
		t.Fatalf("error = %v, want wrapped %v", err, target)
	}
	if !strings.Contains(err.Error(), message) {
		t.Fatalf("error = %v, want message containing %q", err, message)
	}
}
