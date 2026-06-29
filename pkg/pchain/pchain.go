// Package pchain provides all P-Chain transaction operations for Avalanche.
package pchain

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto/bls"
	"github.com/ava-labs/avalanchego/utils/formatting/address"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/platformvm"
	"github.com/ava-labs/avalanchego/vms/platformvm/signer"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
	"github.com/ava-labs/avalanchego/wallet/subnet/primary/common"
	"github.com/ava-labs/platform-cli/pkg/wallet"
)

// =============================================================================
// Issuer Seams
// =============================================================================
//
// Each unexported issue*Tx helper depends on a small, single-method interface
// rather than the full avalanchego wallet. The avalanchego P-Chain wallet
// (pwallet.Wallet, returned by w.PWallet()) satisfies all of them, and tests
// supply small stub implementations.

// baseTxIssuer issues a P-Chain BaseTx.
type baseTxIssuer interface {
	IssueBaseTx(outputs []*avax.TransferableOutput, options ...common.Option) (*txs.Tx, error)
}

// exportTxIssuer issues a P-Chain ExportTx.
type exportTxIssuer interface {
	IssueExportTx(chainID ids.ID, outputs []*avax.TransferableOutput, options ...common.Option) (*txs.Tx, error)
}

// importTxIssuer issues a P-Chain ImportTx.
type importTxIssuer interface {
	IssueImportTx(chainID ids.ID, to *secp256k1fx.OutputOwners, options ...common.Option) (*txs.Tx, error)
}

// permissionlessValidatorTxIssuer issues an AddPermissionlessValidatorTx.
type permissionlessValidatorTxIssuer interface {
	IssueAddPermissionlessValidatorTx(vdr *txs.SubnetValidator, signer signer.Signer, assetID ids.ID, validationRewardsOwner *secp256k1fx.OutputOwners, delegationRewardsOwner *secp256k1fx.OutputOwners, shares uint32, options ...common.Option) (*txs.Tx, error)
}

// permissionlessDelegatorTxIssuer issues an AddPermissionlessDelegatorTx.
type permissionlessDelegatorTxIssuer interface {
	IssueAddPermissionlessDelegatorTx(vdr *txs.SubnetValidator, assetID ids.ID, rewardsOwner *secp256k1fx.OutputOwners, options ...common.Option) (*txs.Tx, error)
}

// autoRenewedValidatorTxIssuer issues an AddAutoRenewedValidatorTx (ACP-236).
type autoRenewedValidatorTxIssuer interface {
	IssueAddAutoRenewedValidatorTx(validatorNodeID ids.NodeID, weight uint64, signer signer.Signer, assetID ids.ID, validationRewardsOwner *secp256k1fx.OutputOwners, delegationRewardsOwner *secp256k1fx.OutputOwners, configOwner *secp256k1fx.OutputOwners, delegationShares uint32, autoCompoundRewardShares uint32, periodSeconds uint64, options ...common.Option) (*txs.Tx, error)
}

// setAutoRenewedValidatorConfigTxIssuer issues a SetAutoRenewedValidatorConfigTx (ACP-236).
type setAutoRenewedValidatorConfigTxIssuer interface {
	IssueSetAutoRenewedValidatorConfigTx(txID ids.ID, autoCompoundRewardShares uint32, periodSeconds uint64, options ...common.Option) (*txs.Tx, error)
}

// createSubnetTxIssuer issues a CreateSubnetTx.
type createSubnetTxIssuer interface {
	IssueCreateSubnetTx(owner *secp256k1fx.OutputOwners, options ...common.Option) (*txs.Tx, error)
}

// transferSubnetOwnershipTxIssuer issues a TransferSubnetOwnershipTx.
type transferSubnetOwnershipTxIssuer interface {
	IssueTransferSubnetOwnershipTx(subnetID ids.ID, owner *secp256k1fx.OutputOwners, options ...common.Option) (*txs.Tx, error)
}

// convertSubnetToL1TxIssuer issues a ConvertSubnetToL1Tx.
type convertSubnetToL1TxIssuer interface {
	IssueConvertSubnetToL1Tx(subnetID ids.ID, chainID ids.ID, address []byte, validators []*txs.ConvertSubnetToL1Validator, options ...common.Option) (*txs.Tx, error)
}

// addSubnetValidatorTxIssuer issues an AddSubnetValidatorTx.
type addSubnetValidatorTxIssuer interface {
	IssueAddSubnetValidatorTx(vdr *txs.SubnetValidator, options ...common.Option) (*txs.Tx, error)
}

// createChainTxIssuer issues a CreateChainTx.
type createChainTxIssuer interface {
	IssueCreateChainTx(subnetID ids.ID, genesis []byte, vmID ids.ID, fxIDs []ids.ID, chainName string, options ...common.Option) (*txs.Tx, error)
}

// =============================================================================
// Transfers
// =============================================================================

// Send sends AVAX on the P-Chain (IssueBaseTx).
func Send(ctx context.Context, w *wallet.Wallet, to ids.ShortID, amountNAVAX uint64) (ids.ID, error) {
	avaxAssetID := w.PWallet().Builder().Context().AVAXAssetID
	return issueSendTx(w.PWallet(), avaxAssetID, to, amountNAVAX, common.WithContext(ctx))
}

// Export exports AVAX from P-Chain to another chain (IssueExportTx).
func Export(ctx context.Context, w *wallet.Wallet, destChainID ids.ID, amountNAVAX uint64) (ids.ID, error) {
	avaxAssetID := w.PWallet().Builder().Context().AVAXAssetID
	return issueExportTx(w.PWallet(), destChainID, avaxAssetID, w.PChainAddress(), amountNAVAX, common.WithContext(ctx))
}

// Import imports AVAX to P-Chain from another chain (IssueImportTx).
func Import(ctx context.Context, w *wallet.Wallet, sourceChainID ids.ID) (ids.ID, error) {
	return issueImportTx(w.PWallet(), sourceChainID, w.PChainAddress(), common.WithContext(ctx))
}

func issueSendTx(
	issuer baseTxIssuer,
	avaxAssetID ids.ID,
	to ids.ShortID,
	amountNAVAX uint64,
	options ...common.Option,
) (ids.ID, error) {
	tx, err := issuer.IssueBaseTx([]*avax.TransferableOutput{{
		Asset: avax.Asset{ID: avaxAssetID},
		Out: &secp256k1fx.TransferOutput{
			Amt: amountNAVAX,
			OutputOwners: secp256k1fx.OutputOwners{
				Threshold: 1,
				Addrs:     []ids.ShortID{to},
			},
		},
	}}, options...)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue BaseTx: %w", err)
	}
	return tx.ID(), nil
}

func issueExportTx(
	issuer exportTxIssuer,
	destChainID ids.ID,
	avaxAssetID ids.ID,
	ownerAddr ids.ShortID,
	amountNAVAX uint64,
	options ...common.Option,
) (ids.ID, error) {
	owner := secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs:     []ids.ShortID{ownerAddr},
	}

	tx, err := issuer.IssueExportTx(destChainID, []*avax.TransferableOutput{{
		Asset: avax.Asset{ID: avaxAssetID},
		Out: &secp256k1fx.TransferOutput{
			Amt:          amountNAVAX,
			OutputOwners: owner,
		},
	}}, options...)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue ExportTx: %w", err)
	}
	return tx.ID(), nil
}

func issueImportTx(
	issuer importTxIssuer,
	sourceChainID ids.ID,
	ownerAddr ids.ShortID,
	options ...common.Option,
) (ids.ID, error) {
	owner := secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs:     []ids.ShortID{ownerAddr},
	}

	tx, err := issuer.IssueImportTx(sourceChainID, &owner, options...)
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
		common.WithContext(ctx),
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
		w.PWallet(),
		avaxAssetID,
		cfg,
		common.WithContext(ctx),
	)
}

func issueAddPermissionlessValidatorTx(
	issuer permissionlessValidatorTxIssuer,
	avaxAssetID ids.ID,
	cfg AddPermissionlessValidatorConfig,
	options ...common.Option,
) (ids.ID, error) {
	rewardsOwner := &secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs:     []ids.ShortID{cfg.RewardAddr},
	}

	tx, err := issuer.IssueAddPermissionlessValidatorTx(
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
		options...,
	)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue AddPermissionlessValidatorTx: %w", err)
	}
	return tx.ID(), nil
}

// =============================================================================
// ACP-236 Auto-Renewed Staking
// =============================================================================

// AddAutoRenewedValidatorConfig holds configuration for adding an auto-renewed
// primary network validator.
type AddAutoRenewedValidatorConfig struct {
	NodeID                   ids.NodeID
	StakeAmt                 uint64 // in nAVAX
	RewardAddr               ids.ShortID
	ValidatorAuthorityAddr   ids.ShortID
	DelegationFee            uint32                    // in parts per million (1_000_000 = 100%)
	AutoCompoundRewardShares uint32                    // in parts per million (1_000_000 = 100%)
	Period                   time.Duration             // auto-renewal cycle duration
	BLSSigner                *signer.ProofOfPossession // BLS proof of possession for the validator
}

// AddAutoRenewedValidator adds an auto-renewed validator to the primary network.
func AddAutoRenewedValidator(ctx context.Context, w *wallet.Wallet, cfg AddAutoRenewedValidatorConfig) (ids.ID, error) {
	avaxAssetID := w.PWallet().Builder().Context().AVAXAssetID
	return issueAddAutoRenewedValidatorTx(w.PWallet(), avaxAssetID, cfg, common.WithContext(ctx))
}

func issueAddAutoRenewedValidatorTx(
	issuer autoRenewedValidatorTxIssuer,
	avaxAssetID ids.ID,
	cfg AddAutoRenewedValidatorConfig,
	options ...common.Option,
) (ids.ID, error) {
	periodSeconds, err := durationToWholeSeconds("period", cfg.Period, false)
	if err != nil {
		return ids.Empty, err
	}

	rewardsOwner := &secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs:     []ids.ShortID{cfg.RewardAddr},
	}
	validatorAuthority := &secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs:     []ids.ShortID{cfg.ValidatorAuthorityAddr},
	}

	tx, err := issuer.IssueAddAutoRenewedValidatorTx(
		cfg.NodeID,
		cfg.StakeAmt,
		cfg.BLSSigner,
		avaxAssetID,
		rewardsOwner,
		rewardsOwner, // delegation rewards go to same owner
		validatorAuthority,
		cfg.DelegationFee,
		cfg.AutoCompoundRewardShares,
		periodSeconds,
		options...,
	)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue AddAutoRenewedValidatorTx: %w", err)
	}
	return tx.ID(), nil
}

// SetAutoRenewedValidatorConfigTxConfig holds configuration for updating an
// auto-renewed validator's next-cycle configuration.
type SetAutoRenewedValidatorConfigTxConfig struct {
	TxID                     ids.ID
	AutoCompoundRewardShares uint32        // in parts per million (1_000_000 = 100%)
	Period                   time.Duration // 0 means exit after the current cycle
}

// SetAutoRenewedValidatorConfig updates an auto-renewed validator's next-cycle
// configuration.
//
// The wallet must be created with the validator's config-authority owner mapped
// to cfg.TxID (see wallet.NewWalletFromKeychainWithOwner), because the public
// builder resolves the authorizing owner from the wallet backend's owners map
// (builder.authorize -> backend.GetOwner) rather than from chain state.
func SetAutoRenewedValidatorConfig(ctx context.Context, w *wallet.Wallet, cfg SetAutoRenewedValidatorConfigTxConfig) (ids.ID, error) {
	return issueSetAutoRenewedValidatorConfigTx(w.PWallet(), cfg, common.WithContext(ctx))
}

func issueSetAutoRenewedValidatorConfigTx(
	issuer setAutoRenewedValidatorConfigTxIssuer,
	cfg SetAutoRenewedValidatorConfigTxConfig,
	options ...common.Option,
) (ids.ID, error) {
	periodSeconds, err := durationToWholeSeconds("period", cfg.Period, true)
	if err != nil {
		return ids.Empty, err
	}

	tx, err := issuer.IssueSetAutoRenewedValidatorConfigTx(
		cfg.TxID,
		cfg.AutoCompoundRewardShares,
		periodSeconds,
		options...,
	)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue SetAutoRenewedValidatorConfigTx: %w", err)
	}
	return tx.ID(), nil
}

// GetAutoRenewedValidatorAuthority returns the config-authority owner for an
// accepted AddAutoRenewedValidatorTx.
//
// When nodeID is non-empty the lookup is narrowed to that node via the
// platform.getCurrentValidators nodeIDs filter, avoiding a full validator-set
// fetch. The typed client does not yet surface validatorAuthority, so the
// reply is decoded with purpose-built structs.
func GetAutoRenewedValidatorAuthority(ctx context.Context, rpcURL string, nodeID ids.NodeID, txID ids.ID) (*secp256k1fx.OutputOwners, error) {
	client := platformvm.NewClient(rpcURL)
	args := &platformvm.GetCurrentValidatorsArgs{}
	if nodeID != ids.EmptyNodeID {
		args.NodeIDs = []ids.NodeID{nodeID}
	}
	reply := &getCurrentValidatorsWithAuthorityReply{}
	if err := client.Requester.SendRequest(ctx, "platform.getCurrentValidators", args, reply); err != nil {
		return nil, fmt.Errorf("failed to fetch current validators: %w", err)
	}

	for _, validator := range reply.Validators {
		if validator.TxID != txID.String() {
			continue
		}
		if validator.ValidatorAuthority == nil {
			return nil, fmt.Errorf("validator %s did not include validatorAuthority", txID)
		}
		return validator.ValidatorAuthority.toOutputOwners()
	}
	return nil, fmt.Errorf("auto-renewed validator %s not found in current validators", txID)
}

type getCurrentValidatorsWithAuthorityReply struct {
	Validators []autoRenewedValidatorWithAuthority `json:"validators"`
}

type autoRenewedValidatorWithAuthority struct {
	TxID               string               `json:"txID"`
	ValidatorAuthority *autoRenewedAPIOwner `json:"validatorAuthority"`
}

type autoRenewedAPIOwner struct {
	Locktime  string   `json:"locktime"`
	Threshold string   `json:"threshold"`
	Addresses []string `json:"addresses"`
}

func (o *autoRenewedAPIOwner) toOutputOwners() (*secp256k1fx.OutputOwners, error) {
	locktime, err := strconv.ParseUint(o.Locktime, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid validatorAuthority locktime: %w", err)
	}
	threshold, err := strconv.ParseUint(o.Threshold, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid validatorAuthority threshold: %w", err)
	}
	addrs, err := address.ParseToIDs(o.Addresses)
	if err != nil {
		return nil, fmt.Errorf("invalid validatorAuthority addresses: %w", err)
	}
	return &secp256k1fx.OutputOwners{
		Locktime:  locktime,
		Threshold: uint32(threshold),
		Addrs:     addrs,
	}, nil
}

func durationToWholeSeconds(name string, duration time.Duration, allowZero bool) (uint64, error) {
	if duration < 0 {
		return 0, fmt.Errorf("%s cannot be negative", name)
	}
	if !allowZero && duration == 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	if duration%time.Second != 0 {
		return 0, fmt.Errorf("%s must be a whole number of seconds", name)
	}
	return uint64(duration / time.Second), nil
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
		common.WithContext(ctx),
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
		w.PWallet(),
		avaxAssetID,
		cfg,
		common.WithContext(ctx),
	)
}

func issueAddPermissionlessDelegatorTx(
	issuer permissionlessDelegatorTxIssuer,
	avaxAssetID ids.ID,
	cfg AddPermissionlessDelegatorConfig,
	options ...common.Option,
) (ids.ID, error) {
	rewardsOwner := &secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs:     []ids.ShortID{cfg.RewardAddr},
	}

	tx, err := issuer.IssueAddPermissionlessDelegatorTx(
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
		options...,
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
	return issueCreateSubnetTx(w.PWallet(), w.PChainAddress(), common.WithContext(ctx))
}

func issueCreateSubnetTx(
	issuer createSubnetTxIssuer,
	ownerAddr ids.ShortID,
	options ...common.Option,
) (ids.ID, error) {
	owner := &secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs:     []ids.ShortID{ownerAddr},
	}

	tx, err := issuer.IssueCreateSubnetTx(owner, options...)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue CreateSubnetTx: %w", err)
	}
	return tx.ID(), nil
}

// TransferSubnetOwnership transfers subnet ownership (IssueTransferSubnetOwnershipTx).
func TransferSubnetOwnership(ctx context.Context, w *wallet.Wallet, subnetID ids.ID, newOwner ids.ShortID) (ids.ID, error) {
	return issueTransferSubnetOwnershipTx(w.PWallet(), subnetID, newOwner, common.WithContext(ctx))
}

func issueTransferSubnetOwnershipTx(
	issuer transferSubnetOwnershipTxIssuer,
	subnetID ids.ID,
	newOwner ids.ShortID,
	options ...common.Option,
) (ids.ID, error) {
	owner := &secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs:     []ids.ShortID{newOwner},
	}

	tx, err := issuer.IssueTransferSubnetOwnershipTx(subnetID, owner, options...)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue TransferSubnetOwnershipTx: %w", err)
	}
	return tx.ID(), nil
}

// ConvertSubnetToL1 converts a subnet to L1 (IssueConvertSubnetToL1Tx).
func ConvertSubnetToL1(ctx context.Context, w *wallet.Wallet, subnetID, chainID ids.ID, managerAddr []byte, validators []*txs.ConvertSubnetToL1Validator) (ids.ID, error) {
	return issueConvertSubnetToL1Tx(w.PWallet(), subnetID, chainID, managerAddr, validators, common.WithContext(ctx))
}

func issueConvertSubnetToL1Tx(
	issuer convertSubnetToL1TxIssuer,
	subnetID ids.ID,
	chainID ids.ID,
	managerAddr []byte,
	validators []*txs.ConvertSubnetToL1Validator,
	options ...common.Option,
) (ids.ID, error) {
	tx, err := issuer.IssueConvertSubnetToL1Tx(subnetID, chainID, managerAddr, validators, options...)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue ConvertSubnetToL1Tx: %w", err)
	}
	return tx.ID(), nil
}

// AddSubnetValidatorConfig holds configuration for adding a validator to a
// permissioned subnet.
type AddSubnetValidatorConfig struct {
	SubnetID ids.ID
	NodeID   ids.NodeID
	Start    time.Time
	End      time.Time
	Weight   uint64 // sampling weight on the subnet (not a stake amount)
}

// AddSubnetValidator adds a validator to a permissioned subnet
// (IssueAddSubnetValidatorTx). The node must already validate the primary
// network, and the subnet owner authorizes the tx via subnet auth (resolved by
// the wallet backend, so the wallet must track the subnet).
func AddSubnetValidator(ctx context.Context, w *wallet.Wallet, cfg AddSubnetValidatorConfig) (ids.ID, error) {
	return issueAddSubnetValidatorTx(w.PWallet(), cfg, common.WithContext(ctx))
}

func issueAddSubnetValidatorTx(
	issuer addSubnetValidatorTxIssuer,
	cfg AddSubnetValidatorConfig,
	options ...common.Option,
) (ids.ID, error) {
	tx, err := issuer.IssueAddSubnetValidatorTx(
		&txs.SubnetValidator{
			Validator: txs.Validator{
				NodeID: cfg.NodeID,
				Start:  uint64(cfg.Start.Unix()),
				End:    uint64(cfg.End.Unix()),
				Wght:   cfg.Weight,
			},
			Subnet: cfg.SubnetID,
		},
		options...,
	)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue AddSubnetValidatorTx: %w", err)
	}
	return tx.ID(), nil
}

// =============================================================================
// L1 Validator Operations
// =============================================================================

// RegisterL1Validator registers a new L1 validator (IssueRegisterL1ValidatorTx).
func RegisterL1Validator(ctx context.Context, w *wallet.Wallet, balance uint64, pop [bls.SignatureLen]byte, message []byte) (ids.ID, error) {
	tx, err := w.PWallet().IssueRegisterL1ValidatorTx(balance, pop, message, common.WithContext(ctx))
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue RegisterL1ValidatorTx: %w", err)
	}
	return tx.ID(), nil
}

// SetL1ValidatorWeight sets the weight of an L1 validator (IssueSetL1ValidatorWeightTx).
func SetL1ValidatorWeight(ctx context.Context, w *wallet.Wallet, message []byte) (ids.ID, error) {
	tx, err := w.PWallet().IssueSetL1ValidatorWeightTx(message, common.WithContext(ctx))
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue SetL1ValidatorWeightTx: %w", err)
	}
	return tx.ID(), nil
}

// IncreaseL1ValidatorBalance increases the balance of an L1 validator (IssueIncreaseL1ValidatorBalanceTx).
func IncreaseL1ValidatorBalance(ctx context.Context, w *wallet.Wallet, validationID ids.ID, amount uint64) (ids.ID, error) {
	tx, err := w.PWallet().IssueIncreaseL1ValidatorBalanceTx(validationID, amount, common.WithContext(ctx))
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue IncreaseL1ValidatorBalanceTx: %w", err)
	}
	return tx.ID(), nil
}

// DisableL1Validator disables an L1 validator (IssueDisableL1ValidatorTx).
func DisableL1Validator(ctx context.Context, w *wallet.Wallet, validationID ids.ID) (ids.ID, error) {
	tx, err := w.PWallet().IssueDisableL1ValidatorTx(validationID, common.WithContext(ctx))
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
	return issueCreateChainTx(w.PWallet(), cfg, common.WithContext(ctx))
}

func issueCreateChainTx(
	issuer createChainTxIssuer,
	cfg CreateChainConfig,
	options ...common.Option,
) (ids.ID, error) {
	tx, err := issuer.IssueCreateChainTx(
		cfg.SubnetID,
		cfg.Genesis,
		cfg.VMID,
		cfg.FxIDs,
		cfg.ChainName,
		options...,
	)
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue CreateChainTx: %w", err)
	}
	return tx.ID(), nil
}
