package cmd

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	blslocalsigner "github.com/ava-labs/avalanchego/utils/crypto/bls/signer/localsigner"
	"github.com/ava-labs/avalanchego/vms/platformvm/signer"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
	"github.com/ava-labs/platform-cli/pkg/network"
	"github.com/ava-labs/platform-cli/pkg/pchain"
	walletpkg "github.com/ava-labs/platform-cli/pkg/wallet"
	"github.com/spf13/cobra"
)

func TestValidatorAddAutoRenewedCommandBuildsConfigAndPrintsSummary(t *testing.T) {
	resetValidatorAutoCommandState(t)
	pubHex, popHex, pop := newValidatorTestPoP(t)
	nodeID := ids.GenerateTestNodeID()
	rewardAddr := ids.GenerateTestShortID()
	authorityAddr := ids.GenerateTestShortID()
	issuedTxID := ids.GenerateTestID()

	valNodeID = nodeID.String()
	valStakeAmount = 1.25
	valAutoPeriod = "2s"
	valDelegationFee = 0.025
	valAutoCompound = 0.3
	valRewardAddr = rewardAddr.String()
	valOwnerAddr = authorityAddr.String()
	valBLSPublicKey = pubHex
	valBLSPoP = popHex

	var gotCfg pchain.AddAutoRenewedValidatorConfig
	validatorGetNetworkConfigFn = func(context.Context) (network.Config, error) {
		return validatorTestNetworkConfig(), nil
	}
	validatorLoadPChainWalletFn = func(context.Context, network.Config) (*walletpkg.Wallet, func(), error) {
		return &walletpkg.Wallet{}, func() {}, nil
	}
	validatorAddAutoRenewedValidatorFn = func(_ context.Context, _ *walletpkg.Wallet, cfg pchain.AddAutoRenewedValidatorConfig) (ids.ID, error) {
		gotCfg = cfg
		return issuedTxID, nil
	}

	out, err := captureStdout(t, func() error {
		return validatorAddAutoRenewedCmd.RunE(validatorAddAutoRenewedCmd, nil)
	})
	if err != nil {
		t.Fatalf("validator add-auto-renewed returned error: %v", err)
	}
	if gotCfg.NodeID != nodeID {
		t.Fatalf("NodeID = %s, want %s", gotCfg.NodeID, nodeID)
	}
	if gotCfg.StakeAmt != 1_250_000_000 {
		t.Fatalf("StakeAmt = %d, want 1250000000", gotCfg.StakeAmt)
	}
	if gotCfg.Period != 2*time.Second {
		t.Fatalf("Period = %s, want 2s", gotCfg.Period)
	}
	if gotCfg.DelegationFee != 25_000 {
		t.Fatalf("DelegationFee = %d, want 25000", gotCfg.DelegationFee)
	}
	if gotCfg.AutoCompoundRewardShares != 300_000 {
		t.Fatalf("AutoCompoundRewardShares = %d, want 300000", gotCfg.AutoCompoundRewardShares)
	}
	if gotCfg.RewardAddr != rewardAddr {
		t.Fatalf("RewardAddr = %s, want %s", gotCfg.RewardAddr, rewardAddr)
	}
	if gotCfg.ValidatorAuthorityAddr != authorityAddr {
		t.Fatalf("ValidatorAuthorityAddr = %s, want %s", gotCfg.ValidatorAuthorityAddr, authorityAddr)
	}
	if gotCfg.BLSSigner == nil ||
		!bytes.Equal(gotCfg.BLSSigner.PublicKey[:], pop.PublicKey[:]) ||
		!bytes.Equal(gotCfg.BLSSigner.ProofOfPossession[:], pop.ProofOfPossession[:]) {
		t.Fatal("BLSSigner was not decoded from the manual BLS public key and PoP")
	}

	for _, want := range []string{
		"Adding auto-renewed validator " + nodeID.String(),
		"with 1.250000000 AVAX stake",
		"Period: 2s",
		"Delegation Fee: 2.50%",
		"Auto-Compound Rewards: 30.00%",
		"Validator Authority: " + authorityAddr.String(),
		"BLS PoP Source: --bls-public-key/--bls-pop flags",
		"TX ID: " + issuedTxID.String(),
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestValidatorAddAutoRenewedCommandValidation(t *testing.T) {
	pubHex, popHex, _ := newValidatorTestPoP(t)
	validNodeID := ids.GenerateTestNodeID().String()
	validRewardAddr := ids.GenerateTestShortID().String()
	validAuthorityAddr := ids.GenerateTestShortID().String()

	tests := []struct {
		name    string
		mutate  func()
		wantErr string
	}{
		{
			name: "missing node id",
			mutate: func() {
				valNodeID = ""
			},
			wantErr: "--node-id is required",
		},
		{
			name: "bad node id",
			mutate: func() {
				valNodeID = "NodeID-not-real"
			},
			wantErr: "invalid node ID",
		},
		{
			name: "missing stake",
			mutate: func() {
				valStakeAmount = 0
			},
			wantErr: "--stake is required and must be positive",
		},
		{
			name: "negative stake",
			mutate: func() {
				valStakeAmount = -1
			},
			wantErr: "--stake is required and must be positive",
		},
		{
			name: "bad period",
			mutate: func() {
				valAutoPeriod = "not-a-period"
			},
			wantErr: "invalid period",
		},
		{
			name: "zero period",
			mutate: func() {
				valAutoPeriod = "0s"
			},
			wantErr: "period must be positive",
		},
		{
			name: "sub-second period",
			mutate: func() {
				valAutoPeriod = "1.5s"
			},
			wantErr: "whole number of seconds",
		},
		{
			name: "missing BLS",
			mutate: func() {
				valBLSPublicKey = ""
				valBLSPoP = ""
			},
			wantErr: "missing BLS proof of possession",
		},
		{
			name: "partial BLS",
			mutate: func() {
				valBLSPoP = ""
			},
			wantErr: "manual BLS mode requires both",
		},
		{
			name: "bad BLS public key hex",
			mutate: func() {
				valBLSPublicKey = "zz"
			},
			wantErr: "invalid --bls-public-key",
		},
		{
			name: "bad BLS PoP hex",
			mutate: func() {
				valBLSPoP = "zz"
			},
			wantErr: "invalid --bls-pop",
		},
		{
			name: "delegation fee below zero",
			mutate: func() {
				valDelegationFee = -0.01
			},
			wantErr: "invalid delegation fee",
		},
		{
			name: "delegation fee above one",
			mutate: func() {
				valDelegationFee = 1.01
			},
			wantErr: "invalid delegation fee",
		},
		{
			name: "auto compound below zero",
			mutate: func() {
				valAutoCompound = -0.01
			},
			wantErr: "invalid auto-compound",
		},
		{
			name: "auto compound above one",
			mutate: func() {
				valAutoCompound = 1.01
			},
			wantErr: "invalid auto-compound",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetValidatorAutoCommandState(t)
			valNodeID = validNodeID
			valStakeAmount = 1
			valAutoPeriod = "2s"
			valDelegationFee = 0.02
			valAutoCompound = 1
			valRewardAddr = validRewardAddr
			valOwnerAddr = validAuthorityAddr
			valBLSPublicKey = pubHex
			valBLSPoP = popHex
			tt.mutate()

			validatorGetNetworkConfigFn = func(context.Context) (network.Config, error) {
				return validatorTestNetworkConfig(), nil
			}
			validatorLoadPChainWalletFn = func(context.Context, network.Config) (*walletpkg.Wallet, func(), error) {
				return &walletpkg.Wallet{}, func() {}, nil
			}
			validatorAddAutoRenewedValidatorFn = func(context.Context, *walletpkg.Wallet, pchain.AddAutoRenewedValidatorConfig) (ids.ID, error) {
				return ids.Empty, errors.New("issue should not be called")
			}

			_, err := captureStdout(t, func() error {
				return validatorAddAutoRenewedCmd.RunE(validatorAddAutoRenewedCmd, nil)
			})
			if err == nil {
				t.Fatalf("validator add-auto-renewed expected error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidatorSetAutoConfigCommandUsesValidatorAuthorityAndPrintsSummary(t *testing.T) {
	resetValidatorAutoCommandState(t)
	validatorTxID := ids.GenerateTestID()
	issuedTxID := ids.GenerateTestID()
	authority := &secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs:     []ids.ShortID{ids.GenerateTestShortID()},
	}

	valSetAutoTxID = validatorTxID.String()
	valSetAutoPeriod = "0"
	valSetAutoCompound = 0.4
	setFlagChanged(t, validatorSetAutoConfigCmd, "period", true)
	setFlagChanged(t, validatorSetAutoConfigCmd, "auto-compound", true)

	var gotCfg pchain.SetAutoRenewedValidatorConfigTxConfig
	validatorGetNetworkConfigFn = func(context.Context) (network.Config, error) {
		return validatorTestNetworkConfig(), nil
	}
	validatorGetAutoRenewedValidatorAuthorityFn = func(_ context.Context, rpcURL string, txID ids.ID) (*secp256k1fx.OutputOwners, error) {
		if rpcURL != validatorTestNetworkConfig().RPCURL {
			t.Fatalf("rpcURL = %q, want %q", rpcURL, validatorTestNetworkConfig().RPCURL)
		}
		if txID != validatorTxID {
			t.Fatalf("authority lookup txID = %s, want %s", txID, validatorTxID)
		}
		return authority, nil
	}
	validatorLoadPChainWalletFn = func(context.Context, network.Config) (*walletpkg.Wallet, func(), error) {
		return &walletpkg.Wallet{}, func() {}, nil
	}
	validatorSetAutoRenewedValidatorConfigFn = func(_ context.Context, _ *walletpkg.Wallet, cfg pchain.SetAutoRenewedValidatorConfigTxConfig) (ids.ID, error) {
		gotCfg = cfg
		return issuedTxID, nil
	}

	out, err := captureStdout(t, func() error {
		return validatorSetAutoConfigCmd.RunE(validatorSetAutoConfigCmd, nil)
	})
	if err != nil {
		t.Fatalf("validator set-auto-config returned error: %v", err)
	}
	if gotCfg.TxID != validatorTxID {
		t.Fatalf("TxID = %s, want %s", gotCfg.TxID, validatorTxID)
	}
	if gotCfg.Period != 0 {
		t.Fatalf("Period = %s, want 0", gotCfg.Period)
	}
	if gotCfg.AutoCompoundRewardShares != 400_000 {
		t.Fatalf("AutoCompoundRewardShares = %d, want 400000", gotCfg.AutoCompoundRewardShares)
	}
	if gotCfg.ValidatorAuthority != authority {
		t.Fatal("ValidatorAuthority was not passed from current-validator lookup")
	}

	for _, want := range []string{
		"Setting auto-renewed validator config for " + validatorTxID.String(),
		"Period: 0s (exit after current cycle)",
		"Auto-Compound Rewards: 40.00%",
		"TX ID: " + issuedTxID.String(),
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestValidatorSetAutoConfigCommandValidation(t *testing.T) {
	validTxID := ids.GenerateTestID().String()

	tests := []struct {
		name    string
		mutate  func()
		wantErr string
	}{
		{
			name: "missing tx id",
			mutate: func() {
				valSetAutoTxID = ""
			},
			wantErr: "--tx-id is required",
		},
		{
			name: "bad tx id",
			mutate: func() {
				valSetAutoTxID = "not-a-tx-id"
			},
			wantErr: "invalid tx ID",
		},
		{
			name: "missing period",
			mutate: func() {
				setFlagChanged(t, validatorSetAutoConfigCmd, "period", false)
			},
			wantErr: "--period is required",
		},
		{
			name: "bad period",
			mutate: func() {
				valSetAutoPeriod = "bad-period"
			},
			wantErr: "invalid period",
		},
		{
			name: "negative period",
			mutate: func() {
				valSetAutoPeriod = "-1s"
			},
			wantErr: "period cannot be negative",
		},
		{
			name: "sub-second period",
			mutate: func() {
				valSetAutoPeriod = "1.5s"
			},
			wantErr: "whole number of seconds",
		},
		{
			name: "missing auto compound",
			mutate: func() {
				setFlagChanged(t, validatorSetAutoConfigCmd, "auto-compound", false)
			},
			wantErr: "--auto-compound is required",
		},
		{
			name: "auto compound below zero",
			mutate: func() {
				valSetAutoCompound = -0.01
			},
			wantErr: "invalid auto-compound",
		},
		{
			name: "auto compound above one",
			mutate: func() {
				valSetAutoCompound = 1.01
			},
			wantErr: "invalid auto-compound",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetValidatorAutoCommandState(t)
			valSetAutoTxID = validTxID
			valSetAutoPeriod = "2s"
			valSetAutoCompound = 0.5
			setFlagChanged(t, validatorSetAutoConfigCmd, "period", true)
			setFlagChanged(t, validatorSetAutoConfigCmd, "auto-compound", true)
			tt.mutate()

			validatorGetNetworkConfigFn = func(context.Context) (network.Config, error) {
				return validatorTestNetworkConfig(), nil
			}
			validatorGetAutoRenewedValidatorAuthorityFn = func(context.Context, string, ids.ID) (*secp256k1fx.OutputOwners, error) {
				return &secp256k1fx.OutputOwners{Threshold: 1, Addrs: []ids.ShortID{ids.GenerateTestShortID()}}, nil
			}
			validatorLoadPChainWalletFn = func(context.Context, network.Config) (*walletpkg.Wallet, func(), error) {
				return &walletpkg.Wallet{}, func() {}, nil
			}
			validatorSetAutoRenewedValidatorConfigFn = func(context.Context, *walletpkg.Wallet, pchain.SetAutoRenewedValidatorConfigTxConfig) (ids.ID, error) {
				return ids.Empty, errors.New("issue should not be called")
			}

			_, err := captureStdout(t, func() error {
				return validatorSetAutoConfigCmd.RunE(validatorSetAutoConfigCmd, nil)
			})
			if err == nil {
				t.Fatalf("validator set-auto-config expected error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func resetValidatorAutoCommandState(t *testing.T) {
	t.Helper()

	origGetNetworkConfigFn := validatorGetNetworkConfigFn
	origLoadPChainWalletFn := validatorLoadPChainWalletFn
	origAddAutoRenewedValidatorFn := validatorAddAutoRenewedValidatorFn
	origGetAutoRenewedValidatorAuthorityFn := validatorGetAutoRenewedValidatorAuthorityFn
	origSetAutoRenewedValidatorConfigFn := validatorSetAutoRenewedValidatorConfigFn
	origRewardAutoRenewedValidatorFn := validatorRewardAutoRenewedValidatorFn

	t.Cleanup(func() {
		validatorGetNetworkConfigFn = origGetNetworkConfigFn
		validatorLoadPChainWalletFn = origLoadPChainWalletFn
		validatorAddAutoRenewedValidatorFn = origAddAutoRenewedValidatorFn
		validatorGetAutoRenewedValidatorAuthorityFn = origGetAutoRenewedValidatorAuthorityFn
		validatorSetAutoRenewedValidatorConfigFn = origSetAutoRenewedValidatorConfigFn
		validatorRewardAutoRenewedValidatorFn = origRewardAutoRenewedValidatorFn
		valNodeID = ""
		valStakeAmount = 0
		valNodeEndpoint = ""
		valBLSPublicKey = ""
		valBLSPoP = ""
		valAutoPeriod = "336h"
		valDelegationFee = 0.02
		valAutoCompound = 1
		valRewardAddr = ""
		valOwnerAddr = ""
		valSetAutoTxID = ""
		valSetAutoPeriod = ""
		valSetAutoCompound = 0
		valRewardAutoTxID = ""
		valRewardAutoTime = ""
		setFlagChanged(t, validatorSetAutoConfigCmd, "period", false)
		setFlagChanged(t, validatorSetAutoConfigCmd, "auto-compound", false)
	})

	valNodeID = ""
	valStakeAmount = 0
	valNodeEndpoint = ""
	valBLSPublicKey = ""
	valBLSPoP = ""
	valAutoPeriod = "336h"
	valDelegationFee = 0.02
	valAutoCompound = 1
	valRewardAddr = ""
	valOwnerAddr = ""
	valSetAutoTxID = ""
	valSetAutoPeriod = ""
	valSetAutoCompound = 0
	valRewardAutoTxID = ""
	valRewardAutoTime = ""
	setFlagChanged(t, validatorSetAutoConfigCmd, "period", false)
	setFlagChanged(t, validatorSetAutoConfigCmd, "auto-compound", false)
}

func setFlagChanged(t *testing.T, cmd *cobra.Command, name string, changed bool) {
	t.Helper()
	cmd.Flags().Lookup(name).Changed = changed
}

func validatorTestNetworkConfig() network.Config {
	return network.Config{
		Name:              "unit",
		NetworkID:         12345,
		RPCURL:            "http://127.0.0.1:9650",
		MinValidatorStake: 1,
		MinDelegatorStake: 1,
		MinStakeDuration:  time.Second,
	}
}

func newValidatorTestPoP(t *testing.T) (string, string, *signer.ProofOfPossession) {
	t.Helper()

	blsSigner, err := blslocalsigner.New()
	if err != nil {
		t.Fatalf("localsigner.New() error = %v", err)
	}
	pop, err := signer.NewProofOfPossession(blsSigner)
	if err != nil {
		t.Fatalf("signer.NewProofOfPossession() error = %v", err)
	}
	return hex.EncodeToString(pop.PublicKey[:]), hex.EncodeToString(pop.ProofOfPossession[:]), pop
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	runErr := fn()
	_ = w.Close()
	os.Stdout = oldStdout
	out := <-done
	_ = r.Close()
	return out, runErr
}
