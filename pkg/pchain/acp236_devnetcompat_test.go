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

// devnetAutoRenewedValidatorTxIssuer adapts a func to autoRenewedValidatorTxIssuer
// so each test can build, verify and serialize a real ACP-236 tx inline.
type devnetAutoRenewedValidatorTxIssuer func(validatorNodeID ids.NodeID, weight uint64, sig signer.Signer, assetID ids.ID, validationRewardsOwner *secp256k1fx.OutputOwners, delegationRewardsOwner *secp256k1fx.OutputOwners, configOwner *secp256k1fx.OutputOwners, delegationShares uint32, autoCompoundRewardShares uint32, periodSeconds uint64, options ...common.Option) (*txs.Tx, error)

func (f devnetAutoRenewedValidatorTxIssuer) IssueAddAutoRenewedValidatorTx(validatorNodeID ids.NodeID, weight uint64, sig signer.Signer, assetID ids.ID, validationRewardsOwner *secp256k1fx.OutputOwners, delegationRewardsOwner *secp256k1fx.OutputOwners, configOwner *secp256k1fx.OutputOwners, delegationShares uint32, autoCompoundRewardShares uint32, periodSeconds uint64, options ...common.Option) (*txs.Tx, error) {
	return f(validatorNodeID, weight, sig, assetID, validationRewardsOwner, delegationRewardsOwner, configOwner, delegationShares, autoCompoundRewardShares, periodSeconds, options...)
}

// devnetSetAutoRenewedValidatorConfigTxIssuer adapts a func to
// setAutoRenewedValidatorConfigTxIssuer.
type devnetSetAutoRenewedValidatorConfigTxIssuer func(txID ids.ID, autoCompoundRewardShares uint32, periodSeconds uint64, options ...common.Option) (*txs.Tx, error)

func (f devnetSetAutoRenewedValidatorConfigTxIssuer) IssueSetAutoRenewedValidatorConfigTx(txID ids.ID, autoCompoundRewardShares uint32, periodSeconds uint64, options ...common.Option) (*txs.Tx, error) {
	return f(txID, autoCompoundRewardShares, periodSeconds, options...)
}

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
	issuer := devnetAutoRenewedValidatorTxIssuer(func(
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
	) (*txs.Tx, error) {
		if common.NewOptions(options).Context().Value(testContextKey("devnetcompat")) != "add" {
			t.Fatal("context option was not passed to add-auto-renewed issuer")
		}
		if validatorNodeID != cfg.NodeID {
			t.Fatalf("issuer nodeID = %s, want %s", validatorNodeID, cfg.NodeID)
		}
		if weight != cfg.StakeAmt {
			t.Fatalf("issuer weight = %d, want %d", weight, cfg.StakeAmt)
		}
		if gotSigner != pop {
			t.Fatal("issuer BLS proof pointer mismatch")
		}
		if assetID != ctx.AVAXAssetID {
			t.Fatalf("issuer assetID = %s, want %s", assetID, ctx.AVAXAssetID)
		}
		if !ownerHasOnly(validationRewardsOwner, rewardAddr) {
			t.Fatalf("validation reward owner = %#v, want [%s]", validationRewardsOwner, rewardAddr)
		}
		if !ownerHasOnly(delegationRewardsOwner, rewardAddr) {
			t.Fatalf("delegation reward owner = %#v, want [%s]", delegationRewardsOwner, rewardAddr)
		}
		if shares != cfg.DelegationFee {
			t.Fatalf("issuer delegation shares = %d, want %d", shares, cfg.DelegationFee)
		}
		if !ownerHasOnly(configOwner, authorityAddr) {
			t.Fatalf("validator authority = %#v, want [%s]", configOwner, authorityAddr)
		}
		if autoCompoundRewardShares != cfg.AutoCompoundRewardShares {
			t.Fatalf("issuer auto-compound shares = %d, want %d", autoCompoundRewardShares, cfg.AutoCompoundRewardShares)
		}
		if periodSeconds != uint64(cfg.Period/time.Second) {
			t.Fatalf("issuer period seconds = %d, want %d", periodSeconds, uint64(cfg.Period/time.Second))
		}

		autoTx := &txs.AddAutoRenewedValidatorTx{
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
		}
		autoTx.InitCtx(ctx)
		if err := autoTx.SyntacticVerify(ctx); err != nil {
			t.Fatalf("AddAutoRenewedValidatorTx failed devnet syntactic verify: %v", err)
		}
		if !bytes.Equal(autoTx.ValidatorNodeID, cfg.NodeID.Bytes()) {
			t.Fatalf("nodeID bytes = %x, want %x", []byte(autoTx.ValidatorNodeID), cfg.NodeID.Bytes())
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
	})

	gotTxID, err := issueAddAutoRenewedValidatorTx(issuer, ctx.AVAXAssetID, cfg, common.WithContext(callCtx))
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
	issuer := devnetSetAutoRenewedValidatorConfigTxIssuer(func(txID ids.ID, autoCompoundRewardShares uint32, periodSeconds uint64, options ...common.Option) (*txs.Tx, error) {
		if common.NewOptions(options).Context().Value(testContextKey("devnetcompat")) != "set" {
			t.Fatal("context option was not passed to set-auto-config issuer")
		}
		if txID != validatorTxID {
			t.Fatalf("issuer txID = %s, want %s", txID, validatorTxID)
		}
		if autoCompoundRewardShares != cfg.AutoCompoundRewardShares {
			t.Fatalf("issuer auto-compound shares = %d, want %d", autoCompoundRewardShares, cfg.AutoCompoundRewardShares)
		}
		if periodSeconds != uint64(cfg.Period/time.Second) {
			t.Fatalf("issuer period seconds = %d, want %d", periodSeconds, uint64(cfg.Period/time.Second))
		}

		setTx := &txs.SetAutoRenewedValidatorConfigTx{
			BaseTx:                   devnetCompatBaseTx(ctx),
			TxID:                     txID,
			Auth:                     auth,
			AutoCompoundRewardShares: autoCompoundRewardShares,
			Period:                   periodSeconds,
		}
		setTx.InitCtx(ctx)
		if err := setTx.SyntacticVerify(ctx); err != nil {
			t.Fatalf("SetAutoRenewedValidatorConfigTx failed devnet syntactic verify: %v", err)
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
	})

	gotTxID, err := issueSetAutoRenewedValidatorConfigTx(issuer, cfg, common.WithContext(callCtx))
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

	issuer := devnetSetAutoRenewedValidatorConfigTxIssuer(func(txID ids.ID, autoCompoundRewardShares uint32, periodSeconds uint64, _ ...common.Option) (*txs.Tx, error) {
		if periodSeconds != 0 {
			t.Fatalf("exit-cycle period = %d, want 0", periodSeconds)
		}
		setTx := &txs.SetAutoRenewedValidatorConfigTx{
			BaseTx:                   devnetCompatBaseTx(ctx),
			TxID:                     txID,
			Auth:                     &secp256k1fx.Input{SigIndices: []uint32{0}},
			AutoCompoundRewardShares: autoCompoundRewardShares,
			Period:                   periodSeconds,
		}
		setTx.InitCtx(ctx)
		if err := setTx.SyntacticVerify(ctx); err != nil {
			t.Fatalf("exit-cycle SetAutoRenewedValidatorConfigTx failed devnet syntactic verify: %v", err)
		}
		return initializeDevnetCompatTx(t, setTx), nil
	})

	if _, err := issueSetAutoRenewedValidatorConfigTx(issuer, cfg); err != nil {
		t.Fatalf("exit-cycle issueSetAutoRenewedValidatorConfigTx returned error: %v", err)
	}
}

func TestDevnetCompatACP236ErrorWrapping(t *testing.T) {
	expectedErr := errors.New("devnet compat failure")

	addIssuer := devnetAutoRenewedValidatorTxIssuer(func(ids.NodeID, uint64, signer.Signer, ids.ID, *secp256k1fx.OutputOwners, *secp256k1fx.OutputOwners, *secp256k1fx.OutputOwners, uint32, uint32, uint64, ...common.Option) (*txs.Tx, error) {
		return nil, expectedErr
	})
	_, err := issueAddAutoRenewedValidatorTx(addIssuer, ids.GenerateTestID(), AddAutoRenewedValidatorConfig{Period: time.Second})
	assertWrapped(t, err, expectedErr, "failed to issue AddAutoRenewedValidatorTx")

	setIssuer := devnetSetAutoRenewedValidatorConfigTxIssuer(func(ids.ID, uint32, uint64, ...common.Option) (*txs.Tx, error) {
		return nil, expectedErr
	})
	_, err = issueSetAutoRenewedValidatorConfigTx(setIssuer, SetAutoRenewedValidatorConfigTxConfig{TxID: ids.GenerateTestID(), Period: time.Second})
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
