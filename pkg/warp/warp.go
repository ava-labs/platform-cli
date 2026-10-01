// Package warp builds, signs, and aggregates P-Chain Warp messages offline, so
// L1 validators can authorize RegisterL1Validator and L1ValidatorWeight
// messages with their BLS keys directly.
package warp

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils/crypto/bls"
	"github.com/ava-labs/avalanchego/utils/set"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs/executor"
	"github.com/ava-labs/avalanchego/vms/platformvm/warp/message"
	"github.com/ava-labs/avalanchego/vms/platformvm/warp/payload"

	avawarp "github.com/ava-labs/avalanchego/vms/platformvm/warp"
)

// The P-Chain accepts a Warp message only if signers hold at least
// QuorumNumerator/QuorumDenominator of the source subnet weight.
const (
	QuorumNumerator   = executor.WarpQuorumNumerator
	QuorumDenominator = executor.WarpQuorumDenominator
)

const signatureSeparator = ":"

var (
	errUnsupportedPayload = errors.New("unsupported payload type")
	errInvalidBlob        = errors.New("invalid signature blob")
	errInvalidSignature   = errors.New("signature does not verify against the message")
	errUnknownSigner      = errors.New("signer is not in the canonical validator set")
	errDuplicateSigner    = errors.New("duplicate signer")
	errNoSignatures       = errors.New("no signatures")
)

// NewUnsignedMessage verifies p and wraps it in an AddressedCall from
// sourceAddress on sourceChainID.
func NewUnsignedMessage(
	networkID uint32,
	sourceChainID ids.ID,
	sourceAddress []byte,
	p message.Payload,
) (*avawarp.UnsignedMessage, error) {
	if err := verifyPayload(p); err != nil {
		return nil, err
	}
	call, err := payload.NewAddressedCall(sourceAddress, p.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to build addressed call: %w", err)
	}
	return avawarp.NewUnsignedMessage(networkID, sourceChainID, call.Bytes())
}

// Decoded is an unsigned Warp message with its AddressedCall and L1 payload
// parsed.
type Decoded struct {
	Message       *avawarp.UnsignedMessage
	SourceAddress []byte
	// Payload is either a *message.RegisterL1Validator or a
	// *message.L1ValidatorWeight.
	Payload message.Payload
}

// Decode parses b as an unsigned Warp message that carries a valid
// RegisterL1Validator or L1ValidatorWeight payload. It rejects every other
// payload, so a signer never signs bytes it cannot inspect.
func Decode(b []byte) (*Decoded, error) {
	msg, err := avawarp.ParseUnsignedMessage(b)
	if err != nil {
		return nil, fmt.Errorf("failed to parse unsigned message: %w", err)
	}
	call, err := payload.ParseAddressedCall(msg.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse addressed call: %w", err)
	}
	p, err := message.Parse(call.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse L1 payload: %w", err)
	}
	if err := verifyPayload(p); err != nil {
		return nil, err
	}
	return &Decoded{
		Message:       msg,
		SourceAddress: call.SourceAddress,
		Payload:       p,
	}, nil
}

func verifyPayload(p message.Payload) error {
	var err error
	switch p := p.(type) {
	case *message.RegisterL1Validator:
		err = p.Verify()
	case *message.L1ValidatorWeight:
		err = p.Verify()
	default:
		return fmt.Errorf("%w: %T", errUnsupportedPayload, p)
	}
	if err != nil {
		return fmt.Errorf("invalid %T payload: %w", p, err)
	}
	return nil
}

// Signature is one validator's BLS signature over an unsigned Warp message.
type Signature struct {
	PublicKey *bls.PublicKey
	Signature *bls.Signature
}

// Sign signs msg the same way a node signs a Warp message.
func Sign(s bls.Signer, msg *avawarp.UnsignedMessage) (Signature, error) {
	sig, err := s.Sign(msg.Bytes())
	if err != nil {
		return Signature{}, fmt.Errorf("failed to sign message: %w", err)
	}
	return Signature{
		PublicKey: s.PublicKey(),
		Signature: sig,
	}, nil
}

// String returns the copy-paste form "<public key hex>:<signature hex>".
func (s Signature) String() string {
	return hex.EncodeToString(bls.PublicKeyToCompressedBytes(s.PublicKey)) +
		signatureSeparator +
		hex.EncodeToString(bls.SignatureToBytes(s.Signature))
}

// ParseSignature parses the form returned by [Signature.String].
func ParseSignature(blob string) (Signature, error) {
	pkHex, sigHex, ok := strings.Cut(strings.TrimSpace(blob), signatureSeparator)
	if !ok {
		return Signature{}, fmt.Errorf("%w: want <public key hex>%s<signature hex>", errInvalidBlob, signatureSeparator)
	}
	pkBytes, err := hex.DecodeString(strings.TrimPrefix(pkHex, "0x"))
	if err != nil {
		return Signature{}, fmt.Errorf("%w: public key: %w", errInvalidBlob, err)
	}
	pk, err := bls.PublicKeyFromCompressedBytes(pkBytes)
	if err != nil {
		return Signature{}, fmt.Errorf("%w: public key: %w", errInvalidBlob, err)
	}
	sigBytes, err := hex.DecodeString(strings.TrimPrefix(sigHex, "0x"))
	if err != nil {
		return Signature{}, fmt.Errorf("%w: signature: %w", errInvalidBlob, err)
	}
	sig, err := bls.SignatureFromBytes(sigBytes)
	if err != nil {
		return Signature{}, fmt.Errorf("%w: signature: %w", errInvalidBlob, err)
	}
	return Signature{
		PublicKey: pk,
		Signature: sig,
	}, nil
}

// Verify reports whether s is a valid signature of msg by s.PublicKey.
func (s Signature) Verify(msg *avawarp.UnsignedMessage) bool {
	return bls.Verify(s.PublicKey, s.Signature, msg.Bytes())
}

// Aggregation is a signed Warp message and the weight that signed it.
type Aggregation struct {
	Message *avawarp.Message
	// Signers are the canonical indices of the signers, in ascending order.
	Signers      []int
	SignedWeight uint64
	TotalWeight  uint64
}

// Aggregate places each signature at its signer's index in vdrs, checks that
// the signers reach the P-Chain quorum, and returns the signed message.
//
// Every signature must verify against msg, and every signer must be in vdrs.
// If the signed weight is below quorum, the returned error wraps
// [avawarp.ErrInsufficientWeight].
func Aggregate(
	msg *avawarp.UnsignedMessage,
	vdrs validators.WarpSet,
	sigs []Signature,
) (*Aggregation, error) {
	if len(sigs) == 0 {
		return nil, errNoSignatures
	}

	var (
		signerBits = set.NewBits()
		indices    = make([]int, 0, len(sigs))
		signers    = make([]*validators.Warp, 0, len(sigs))
		blsSigs    = make([]*bls.Signature, 0, len(sigs))
	)
	for _, sig := range sigs {
		pkBytes := bls.PublicKeyToUncompressedBytes(sig.PublicKey)
		pkHex := hex.EncodeToString(bls.PublicKeyToCompressedBytes(sig.PublicKey))
		i, ok := canonicalIndex(vdrs.Validators, pkBytes)
		if !ok {
			return nil, fmt.Errorf("%w: %s", errUnknownSigner, pkHex)
		}
		if signerBits.Contains(i) {
			return nil, fmt.Errorf("%w: %s", errDuplicateSigner, pkHex)
		}
		if !sig.Verify(msg) {
			return nil, fmt.Errorf("%w: signer %s", errInvalidSignature, pkHex)
		}
		signerBits.Add(i)
		indices = append(indices, i)
		signers = append(signers, vdrs.Validators[i])
		blsSigs = append(blsSigs, sig.Signature)
	}

	signedWeight, err := avawarp.SumWeight(signers)
	if err != nil {
		return nil, fmt.Errorf("failed to sum signer weight: %w", err)
	}
	err = avawarp.VerifyWeight(
		signedWeight,
		vdrs.TotalWeight,
		QuorumNumerator,
		QuorumDenominator,
	)
	if err != nil {
		return nil, fmt.Errorf("signed weight %d of %d is below the %d%% quorum: %w",
			signedWeight,
			vdrs.TotalWeight,
			QuorumNumerator,
			err,
		)
	}

	aggSig, err := bls.AggregateSignatures(blsSigs)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate signatures: %w", err)
	}
	bitSetSig := &avawarp.BitSetSignature{
		Signers: signerBits.Bytes(),
	}
	copy(bitSetSig.Signature[:], bls.SignatureToBytes(aggSig))

	signed, err := avawarp.NewMessage(msg, bitSetSig)
	if err != nil {
		return nil, fmt.Errorf("failed to build signed message: %w", err)
	}

	// Run the P-Chain verification so a bad aggregate never leaves this
	// function.
	err = signed.Signature.Verify(
		msg,
		msg.NetworkID,
		vdrs,
		QuorumNumerator,
		QuorumDenominator,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to verify signed message: %w", err)
	}

	slices.Sort(indices)
	return &Aggregation{
		Message:      signed,
		Signers:      indices,
		SignedWeight: signedWeight,
		TotalWeight:  vdrs.TotalWeight,
	}, nil
}

// SelectSigners returns the signatures in sigs whose signer is in vdrs.
func SelectSigners(vdrs validators.WarpSet, sigs []Signature) []Signature {
	return slices.DeleteFunc(slices.Clone(sigs), func(s Signature) bool {
		_, ok := canonicalIndex(vdrs.Validators, bls.PublicKeyToUncompressedBytes(s.PublicKey))
		return !ok
	})
}

func canonicalIndex(vdrs []*validators.Warp, pkBytes []byte) (int, bool) {
	for i, vdr := range vdrs {
		if bytes.Equal(vdr.PublicKeyBytes, pkBytes) {
			return i, true
		}
	}
	return 0, false
}
