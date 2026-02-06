// Package pchain provides all P-Chain transaction operations for Avalanche.
package pchain

import (
	"context"
	"fmt"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto/bls"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/platformvm/signer"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
	"github.com/ava-labs/avalanchego/wallet/subnet/primary/common"
	"github.com/ava-labs/platform-cli/pkg/wallet"
)

// =============================================================================
// Transfers
// =============================================================================

// Send sends AVAX on the P-Chain (IssueBaseTx).
func Send(ctx context.Context, w *wallet.Wallet, to ids.ShortID, amountNAVAX uint64) (ids.ID, error) {
	avaxAssetID := w.PWallet().Builder().Context().AVAXAssetID
	return issueSendTx(w.PWallet().IssueBaseTx, avaxAssetID, to, amountNAVAX)
}

// Export exports AVAX from P-Chain to another chain (IssueExportTx).
func Export(ctx context.Context, w *wallet.Wallet, destChainID ids.ID, amountNAVAX uint64) (ids.ID, error) {
	avaxAssetID := w.PWallet().Builder().Context().AVAXAssetID
	return issueExportTx(w.PWallet().IssueExportTx, destChainID, avaxAssetID, w.PChainAddress(), amountNAVAX)
}

// Import imports AVAX to P-Chain from another chain (IssueImportTx).
func Import(ctx context.Context, w *wallet.Wallet, sourceChainID ids.ID) (ids.ID, error) {
	return issueImportTx(w.PWallet().IssueImportTx, sourceChainID, w.PChainAddress())
}

func issueSendTx(
	issueBaseTx func(outputs []*avax.TransferableOutput, options ...common.Option) (*txs.Tx, error),
	avaxAssetID ids.ID,
	to ids.ShortID,
	amountNAVAX uint64,
) (ids.ID, error) {
	tx, err := issueBaseTx([]*avax.TransferableOutput{{
		Asset: avax.Asset{ID: avaxAssetID},
		Out: &secp256k1fx.TransferOutput{
			Amt: amountNAVAX,
			OutputOwners: secp256k1fx.OutputOwners{
				Threshold: 1,
				Addrs:     []ids.ShortID{to},
			},
		},
	}})
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue BaseTx: %w", err)
	}
	return tx.ID(), nil
}

func issueExportTx(
	issueExportTxFn func(chainID ids.ID, outputs []*avax.TransferableOutput, options ...common.Option) (*txs.Tx, error),
	destChainID ids.ID,
	avaxAssetID ids.ID,
	ownerAddr ids.ShortID,
	amountNAVAX uint64,
) (ids.ID, error) {
	owner := secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs:     []ids.ShortID{ownerAddr},
	}

	tx, err := issueExportTxFn(destChainID, []*avax.TransferableOutput{{
		Asset: avax.Asset{ID: avaxAssetID},
		Out: &secp256k1fx.TransferOutput{
			Amt:          amountNAVAX,
			OutputOwners: owner,
		},
	}})
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue ExportTx: %w", err)
	}
	return tx.ID(), nil
}

func issueImportTx(
	issueImportTxFn func(chainID ids.ID, to *secp256k1fx.OutputOwners, options ...common.Option) (*txs.Tx, error),
	sourceChainID ids.ID,
	ownerAddr ids.ShortID,
) (ids.ID, error) {
	owner := secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs:     []ids.ShortID{ownerAddr},
	}

	tx, err := issueImportTxFn(sourceChainID, &owner)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue ImportTx: %w", err)
	}
	return tx.ID(), nil
}

// =============================================================================
// Primary Network Staking
// =============================================================================

// AddValidatorConfig holds configuration for adding a primary network validator.
type AddValidatorConfig struct {
	NodeID        ids.NodeID
	Start         time.Time
	End           time.Time
	StakeAmt      uint64 // in nAVAX (Fuji: min 1 AVAX, Mainnet: min 2000 AVAX)
	RewardAddr    ids.ShortID
	DelegationFee uint32 // in basis points (10000 = 100%, 200 = 2%)
}

// AddValidator adds a validator to the primary network (IssueAddValidatorTx).
// NOTE: This uses the legacy AddValidatorTx which is deprecated post-Etna.
// Use AddPermissionlessValidator for post-Etna networks.
func AddValidator(ctx context.Context, w *wallet.Wallet, cfg AddValidatorConfig) (ids.ID, error) {
	rewardsOwner := &secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs:     []ids.ShortID{cfg.RewardAddr},
	}

	tx, err := w.PWallet().IssueAddValidatorTx(
		&txs.Validator{
			NodeID: cfg.NodeID,
			Start:  uint64(cfg.Start.Unix()),
			End:    uint64(cfg.End.Unix()),
			Wght:   cfg.StakeAmt,
		},
		rewardsOwner,
		cfg.DelegationFee,
	)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue AddValidatorTx: %w", err)
	}
	return tx.ID(), nil
}

// AddPermissionlessValidatorConfig holds configuration for adding a permissionless validator.
type AddPermissionlessValidatorConfig struct {
	NodeID        ids.NodeID
	Start         time.Time
	End           time.Time
	StakeAmt      uint64 // in nAVAX (Fuji: min 1 AVAX, Mainnet: min 2000 AVAX for primary network)
	RewardAddr    ids.ShortID
	DelegationFee uint32                    // in parts per million (1_000_000 = 100%)
	BLSSigner     *signer.ProofOfPossession // BLS proof of possession for the validator (required for primary network)
}

// AddPermissionlessValidator adds a permissionless validator to the primary network.
// This is the post-Etna method for staking on the primary network.
func AddPermissionlessValidator(ctx context.Context, w *wallet.Wallet, cfg AddPermissionlessValidatorConfig) (ids.ID, error) {
	avaxAssetID := w.PWallet().Builder().Context().AVAXAssetID
	return issueAddPermissionlessValidatorTx(
		w.PWallet().IssueAddPermissionlessValidatorTx,
		avaxAssetID,
		cfg,
	)
}

func issueAddPermissionlessValidatorTx(
	issueTxFn func(
		vdr *txs.SubnetValidator,
		signer signer.Signer,
		assetID ids.ID,
		validationRewardsOwner *secp256k1fx.OutputOwners,
		delegationRewardsOwner *secp256k1fx.OutputOwners,
		shares uint32,
		options ...common.Option,
	) (*txs.Tx, error),
	avaxAssetID ids.ID,
	cfg AddPermissionlessValidatorConfig,
) (ids.ID, error) {
	rewardsOwner := &secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs:     []ids.ShortID{cfg.RewardAddr},
	}

	tx, err := issueTxFn(
		&txs.SubnetValidator{
			Validator: txs.Validator{
				NodeID: cfg.NodeID,
				Start:  uint64(cfg.Start.Unix()),
				End:    uint64(cfg.End.Unix()),
				Wght:   cfg.StakeAmt,
			},
			Subnet: ids.Empty, // Empty = Primary Network
		},
		cfg.BLSSigner,
		avaxAssetID,
		rewardsOwner,
		rewardsOwner, // delegation rewards go to same owner
		cfg.DelegationFee,
	)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue AddPermissionlessValidatorTx: %w", err)
	}
	return tx.ID(), nil
}

// AddDelegatorConfig holds configuration for adding a delegator.
type AddDelegatorConfig struct {
	NodeID     ids.NodeID
	Start      time.Time
	End        time.Time
	StakeAmt   uint64 // in nAVAX (Fuji: min 1 AVAX, Mainnet: min 25 AVAX)
	RewardAddr ids.ShortID
}

// AddDelegator adds a delegator to the primary network (IssueAddDelegatorTx).
// NOTE: This uses the legacy AddDelegatorTx which is deprecated post-Etna.
// Use AddPermissionlessDelegator for post-Etna networks.
func AddDelegator(ctx context.Context, w *wallet.Wallet, cfg AddDelegatorConfig) (ids.ID, error) {
	rewardsOwner := &secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs:     []ids.ShortID{cfg.RewardAddr},
	}

	tx, err := w.PWallet().IssueAddDelegatorTx(
		&txs.Validator{
			NodeID: cfg.NodeID,
			Start:  uint64(cfg.Start.Unix()),
			End:    uint64(cfg.End.Unix()),
			Wght:   cfg.StakeAmt,
		},
		rewardsOwner,
	)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue AddDelegatorTx: %w", err)
	}
	return tx.ID(), nil
}

// AddPermissionlessDelegatorConfig holds configuration for adding a permissionless delegator.
type AddPermissionlessDelegatorConfig struct {
	NodeID     ids.NodeID
	Start      time.Time
	End        time.Time
	StakeAmt   uint64 // in nAVAX (Fuji: min 1 AVAX, Mainnet: min 25 AVAX)
	RewardAddr ids.ShortID
}

// AddPermissionlessDelegator adds a permissionless delegator to the primary network.
// This is the post-Etna method for delegating on the primary network.
func AddPermissionlessDelegator(ctx context.Context, w *wallet.Wallet, cfg AddPermissionlessDelegatorConfig) (ids.ID, error) {
	avaxAssetID := w.PWallet().Builder().Context().AVAXAssetID
	return issueAddPermissionlessDelegatorTx(
		w.PWallet().IssueAddPermissionlessDelegatorTx,
		avaxAssetID,
		cfg,
	)
}

func issueAddPermissionlessDelegatorTx(
	issueTxFn func(
		vdr *txs.SubnetValidator,
		assetID ids.ID,
		rewardsOwner *secp256k1fx.OutputOwners,
		options ...common.Option,
	) (*txs.Tx, error),
	avaxAssetID ids.ID,
	cfg AddPermissionlessDelegatorConfig,
) (ids.ID, error) {
	rewardsOwner := &secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs:     []ids.ShortID{cfg.RewardAddr},
	}

	tx, err := issueTxFn(
		&txs.SubnetValidator{
			Validator: txs.Validator{
				NodeID: cfg.NodeID,
				Start:  uint64(cfg.Start.Unix()),
				End:    uint64(cfg.End.Unix()),
				Wght:   cfg.StakeAmt,
			},
			Subnet: ids.Empty, // Empty = Primary Network
		},
		avaxAssetID,
		rewardsOwner,
	)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue AddPermissionlessDelegatorTx: %w", err)
	}
	return tx.ID(), nil
}

// =============================================================================
// Subnet Management
// =============================================================================

// CreateSubnet creates a new subnet (IssueCreateSubnetTx).
func CreateSubnet(ctx context.Context, w *wallet.Wallet) (ids.ID, error) {
	return issueCreateSubnetTx(w.PWallet().IssueCreateSubnetTx, w.PChainAddress())
}

func issueCreateSubnetTx(
	issueTxFn func(owner *secp256k1fx.OutputOwners, options ...common.Option) (*txs.Tx, error),
	ownerAddr ids.ShortID,
) (ids.ID, error) {
	owner := &secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs:     []ids.ShortID{ownerAddr},
	}

	tx, err := issueTxFn(owner)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue CreateSubnetTx: %w", err)
	}
	return tx.ID(), nil
}

// TransferSubnetOwnership transfers subnet ownership (IssueTransferSubnetOwnershipTx).
func TransferSubnetOwnership(ctx context.Context, w *wallet.Wallet, subnetID ids.ID, newOwner ids.ShortID) (ids.ID, error) {
	return issueTransferSubnetOwnershipTx(w.PWallet().IssueTransferSubnetOwnershipTx, subnetID, newOwner)
}

func issueTransferSubnetOwnershipTx(
	issueTxFn func(subnetID ids.ID, owner *secp256k1fx.OutputOwners, options ...common.Option) (*txs.Tx, error),
	subnetID ids.ID,
	newOwner ids.ShortID,
) (ids.ID, error) {
	owner := &secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs:     []ids.ShortID{newOwner},
	}

	tx, err := issueTxFn(subnetID, owner)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue TransferSubnetOwnershipTx: %w", err)
	}
	return tx.ID(), nil
}

// ConvertSubnetToL1 converts a subnet to L1 (IssueConvertSubnetToL1Tx).
func ConvertSubnetToL1(ctx context.Context, w *wallet.Wallet, subnetID, chainID ids.ID, managerAddr []byte, validators []*txs.ConvertSubnetToL1Validator) (ids.ID, error) {
	return issueConvertSubnetToL1Tx(w.PWallet().IssueConvertSubnetToL1Tx, subnetID, chainID, managerAddr, validators)
}

func issueConvertSubnetToL1Tx(
	issueTxFn func(subnetID ids.ID, chainID ids.ID, address []byte, validators []*txs.ConvertSubnetToL1Validator, options ...common.Option) (*txs.Tx, error),
	subnetID ids.ID,
	chainID ids.ID,
	managerAddr []byte,
	validators []*txs.ConvertSubnetToL1Validator,
) (ids.ID, error) {
	tx, err := issueTxFn(subnetID, chainID, managerAddr, validators)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue ConvertSubnetToL1Tx: %w", err)
	}
	return tx.ID(), nil
}

// =============================================================================
// L1 Validator Operations
// =============================================================================

// RegisterL1Validator registers a new L1 validator (IssueRegisterL1ValidatorTx).
func RegisterL1Validator(ctx context.Context, w *wallet.Wallet, balance uint64, pop [bls.SignatureLen]byte, message []byte) (ids.ID, error) {
	tx, err := w.PWallet().IssueRegisterL1ValidatorTx(balance, pop, message)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue RegisterL1ValidatorTx: %w", err)
	}
	return tx.ID(), nil
}

// SetL1ValidatorWeight sets the weight of an L1 validator (IssueSetL1ValidatorWeightTx).
func SetL1ValidatorWeight(ctx context.Context, w *wallet.Wallet, message []byte) (ids.ID, error) {
	tx, err := w.PWallet().IssueSetL1ValidatorWeightTx(message)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue SetL1ValidatorWeightTx: %w", err)
	}
	return tx.ID(), nil
}

// IncreaseL1ValidatorBalance increases the balance of an L1 validator (IssueIncreaseL1ValidatorBalanceTx).
func IncreaseL1ValidatorBalance(ctx context.Context, w *wallet.Wallet, validationID ids.ID, amount uint64) (ids.ID, error) {
	tx, err := w.PWallet().IssueIncreaseL1ValidatorBalanceTx(validationID, amount)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue IncreaseL1ValidatorBalanceTx: %w", err)
	}
	return tx.ID(), nil
}

// DisableL1Validator disables an L1 validator (IssueDisableL1ValidatorTx).
func DisableL1Validator(ctx context.Context, w *wallet.Wallet, validationID ids.ID) (ids.ID, error) {
	tx, err := w.PWallet().IssueDisableL1ValidatorTx(validationID)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue DisableL1ValidatorTx: %w", err)
	}
	return tx.ID(), nil
}

// =============================================================================
// Chain Management
// =============================================================================

// CreateChainConfig holds configuration for creating a chain.
type CreateChainConfig struct {
	SubnetID  ids.ID
	Genesis   []byte
	VMID      ids.ID
	FxIDs     []ids.ID
	ChainName string
}

// CreateChain creates a new chain on a subnet (IssueCreateChainTx).
func CreateChain(ctx context.Context, w *wallet.Wallet, cfg CreateChainConfig) (ids.ID, error) {
	return issueCreateChainTx(w.PWallet().IssueCreateChainTx, cfg)
}

func issueCreateChainTx(
	issueTxFn func(
		subnetID ids.ID,
		genesis []byte,
		vmID ids.ID,
		fxIDs []ids.ID,
		chainName string,
		options ...common.Option,
	) (*txs.Tx, error),
	cfg CreateChainConfig,
) (ids.ID, error) {
	tx, err := issueTxFn(
		cfg.SubnetID,
		cfg.Genesis,
		cfg.VMID,
		cfg.FxIDs,
		cfg.ChainName,
	)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue CreateChainTx: %w", err)
	}
	return tx.ID(), nil
}
