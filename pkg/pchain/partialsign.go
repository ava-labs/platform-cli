package pchain

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto/secp256k1"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
	walletsigner "github.com/ava-labs/avalanchego/wallet/chain/p/signer"
	"github.com/ava-labs/avalanchego/wallet/subnet/primary/common"
	"github.com/ava-labs/platform-cli/pkg/wallet"
)

// Partial signing support for multisig-owned subnets: one signer builds a tx
// and signs it with locally available keys, the serialized tx is passed to
// the remaining signers (each running platform-cli on their own machine),
// and any party submits it once the owner threshold is met.

// BuildTransferSubnetOwnershipTx builds a TransferSubnetOwnershipTx and signs
// it with the wallet's keys WITHOUT issuing it. Signature slots for keys the
// wallet does not hold are left empty for other signers to fill.
func BuildTransferSubnetOwnershipTx(ctx context.Context, w *wallet.Wallet, subnetID ids.ID, newOwners []ids.ShortID, threshold uint32) (*txs.Tx, error) {
	owner, err := NewOwner(newOwners, threshold)
	if err != nil {
		return nil, fmt.Errorf("invalid new owner: %w", err)
	}
	utx, err := w.PWallet().Builder().NewTransferSubnetOwnershipTx(subnetID, owner, common.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to build TransferSubnetOwnershipTx: %w", err)
	}
	tx, err := walletsigner.SignUnsigned(ctx, w.PWallet().Signer(), utx)
	if err != nil {
		return nil, fmt.Errorf("failed to sign TransferSubnetOwnershipTx: %w", err)
	}
	return tx, nil
}

// SignTx adds as many missing signatures to tx as the wallet's keys allow.
// Existing signatures are preserved; slots the wallet cannot sign are left
// untouched.
func SignTx(ctx context.Context, w *wallet.Wallet, tx *txs.Tx) error {
	if err := w.PWallet().Signer().Sign(ctx, tx); err != nil {
		return fmt.Errorf("failed to sign tx: %w", err)
	}
	return nil
}

// IssueSignedTx submits an already-signed tx and waits for acceptance.
func IssueSignedTx(ctx context.Context, w *wallet.Wallet, tx *txs.Tx) (ids.ID, error) {
	if err := w.PWallet().IssueTx(tx, common.WithContext(ctx)); err != nil {
		return ids.Empty, fmt.Errorf("failed to issue tx: %w", err)
	}
	return tx.ID(), nil
}

// SubnetAuthSigners splits the subnet-auth signers of a partially signed
// TransferSubnetOwnershipTx into addresses that have already signed and
// addresses whose signature slots are still empty. controlKeys must be the
// subnet's current on-chain owner addresses (sorted, as returned by
// platform.getSubnet).
func SubnetAuthSigners(tx *txs.Tx, controlKeys []ids.ShortID) (signed, remaining []ids.ShortID, err error) {
	utx, ok := tx.Unsigned.(*txs.TransferSubnetOwnershipTx)
	if !ok {
		return nil, nil, fmt.Errorf("unsupported tx type %T (only TransferSubnetOwnershipTx is supported)", tx.Unsigned)
	}
	subnetAuth, ok := utx.SubnetAuth.(*secp256k1fx.Input)
	if !ok {
		return nil, nil, fmt.Errorf("unexpected subnet auth type %T", utx.SubnetAuth)
	}
	// The wallet signer orders credentials as [inputs..., subnetAuth], so the
	// subnet auth credential is always last.
	if len(tx.Creds) == 0 {
		return nil, nil, fmt.Errorf("tx has no credentials")
	}
	cred, ok := tx.Creds[len(tx.Creds)-1].(*secp256k1fx.Credential)
	if !ok {
		return nil, nil, fmt.Errorf("unexpected subnet auth credential type %T", tx.Creds[len(tx.Creds)-1])
	}
	if len(cred.Sigs) != len(subnetAuth.SigIndices) {
		return nil, nil, fmt.Errorf("subnet auth credential has %d signature slots, want %d", len(cred.Sigs), len(subnetAuth.SigIndices))
	}

	var emptySig [secp256k1.SignatureLen]byte
	for i, sigIdx := range subnetAuth.SigIndices {
		if int(sigIdx) >= len(controlKeys) {
			return nil, nil, fmt.Errorf("subnet auth signature index %d out of range (%d control keys); is the tx built against the current owner set?", sigIdx, len(controlKeys))
		}
		addr := controlKeys[sigIdx]
		if cred.Sigs[i] == emptySig {
			remaining = append(remaining, addr)
		} else {
			signed = append(signed, addr)
		}
	}
	return signed, remaining, nil
}

// SelectAuthSigners picks which owner addresses are expected to sign an
// owner-gated tx: owners whose keys are held locally first, then the
// remaining owners in on-chain order, up to threshold. The result feeds the
// tx builder's address set so it allocates signature slots for exactly these
// owners.
func SelectAuthSigners(controlKeys []ids.ShortID, threshold uint32, localAddrs []ids.ShortID) ([]ids.ShortID, error) {
	if threshold == 0 || int(threshold) > len(controlKeys) {
		return nil, fmt.Errorf("invalid owner threshold %d for %d control keys", threshold, len(controlKeys))
	}
	local := make(map[ids.ShortID]bool, len(localAddrs))
	for _, addr := range localAddrs {
		local[addr] = true
	}
	selected := make([]ids.ShortID, 0, threshold)
	for _, ck := range controlKeys {
		if local[ck] && len(selected) < int(threshold) {
			selected = append(selected, ck)
		}
	}
	for _, ck := range controlKeys {
		if !local[ck] && len(selected) < int(threshold) {
			selected = append(selected, ck)
		}
	}
	return selected, nil
}

// txFileEnvelope is the JSON envelope used to pass a partially signed tx
// between machines. The tx bytes are hex encoded with a checksum so file
// corruption is detected on load.
type txFileEnvelope struct {
	NetworkID uint32 `json:"networkID"`
	TxHex     string `json:"txHex"`
}

// EncodeTxFile serializes a (partially) signed tx for transport.
func EncodeTxFile(networkID uint32, tx *txs.Tx) ([]byte, error) {
	txHex, err := formatting.Encode(formatting.Hex, tx.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to encode tx bytes: %w", err)
	}
	data, err := json.MarshalIndent(txFileEnvelope{
		NetworkID: networkID,
		TxHex:     txHex,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tx file: %w", err)
	}
	return append(data, '\n'), nil
}

// DecodeTxFile parses a tx file produced by EncodeTxFile and returns the
// network it was built for along with the tx.
func DecodeTxFile(data []byte) (uint32, *txs.Tx, error) {
	var envelope txFileEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return 0, nil, fmt.Errorf("failed to parse tx file: %w", err)
	}
	txBytes, err := formatting.Decode(formatting.Hex, envelope.TxHex)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to decode tx hex: %w", err)
	}
	tx, err := txs.Parse(txs.Codec, txBytes)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to parse tx: %w", err)
	}
	return envelope.NetworkID, tx, nil
}
