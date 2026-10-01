package warp

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto/bls"
	"github.com/ava-labs/avalanchego/vms/platformvm/signer"
	"github.com/ava-labs/avalanchego/vms/types"
)

// BundleVersion is the version of the signature bundle file format.
const BundleVersion = 1

var (
	errBundleVersion      = errors.New("unsupported bundle version")
	errInvalidPoP         = errors.New("invalid proof of possession")
	errNoSigners          = errors.New("no signers")
	errUnknownMessageID   = errors.New("signature for a message that is not in the plan")
	errDuplicateBundleKey = errors.New("BLS key appears in more than one bundle entry")
)

// Bundle is the output of one signing machine: every plan message signed by
// every key on that machine.
type Bundle struct {
	Version uint32         `json:"version"`
	Signers []BundleSigner `json:"signers"`
}

// BundleSigner is the signatures of one BLS key.
type BundleSigner struct {
	PublicKey types.JSONByteSlice `json:"publicKey"`
	// ProofOfPossession lets the coordinator re-add this key with
	// RegisterL1ValidatorTx.
	ProofOfPossession types.JSONByteSlice `json:"proofOfPossession"`
	// Signatures maps an unsigned message ID to the signature of that
	// message.
	Signatures map[ids.ID]types.JSONByteSlice `json:"signatures"`
}

// SignPlan signs every message of planned with every signer.
func SignPlan(planned []PlannedTarget, signers []bls.Signer) (*Bundle, error) {
	if len(signers) == 0 {
		return nil, errNoSigners
	}
	b := &Bundle{
		Version: BundleVersion,
		Signers: make([]BundleSigner, 0, len(signers)),
	}
	for _, s := range signers {
		pop, err := signer.NewProofOfPossession(s)
		if err != nil {
			return nil, fmt.Errorf("failed to build proof of possession: %w", err)
		}
		bs := BundleSigner{
			PublicKey:         pop.PublicKey[:],
			ProofOfPossession: pop.ProofOfPossession[:],
			Signatures:        make(map[ids.ID]types.JSONByteSlice, 2*len(planned)),
		}
		for _, t := range planned {
			for _, d := range []*Decoded{t.Removal, t.Readd} {
				sig, err := Sign(s, d.Message)
				if err != nil {
					return nil, err
				}
				bs.Signatures[d.Message.ID()] = bls.SignatureToBytes(sig.Signature)
			}
		}
		b.Signers = append(b.Signers, bs)
	}
	return b, nil
}

// Collected is the verified content of a set of bundles.
type Collected struct {
	// Signatures maps a message ID to its verified signatures.
	Signatures map[ids.ID][]Signature
	// PoPs maps a compressed BLS public key to its verified proof of
	// possession.
	PoPs map[string][bls.SignatureLen]byte
	// Skipped has one error per message ID that a bundle signs but the plan
	// does not hold, in ID order. Each one wraps errUnknownMessageID.
	Skipped []error
}

// Collect verifies every signature and proof of possession in bundles against
// planned. It rejects an invalid signature and a key that appears twice.
//
// It skips a signature of a message outside the plan and reports the message
// ID in Skipped. After a re-plan, old bundles sign re-adds with a changed
// expiry, but their removal signatures are still valid, because a removal
// message does not change.
func Collect(planned []PlannedTarget, bundles []*Bundle) (*Collected, error) {
	msgs := make(map[ids.ID]*Decoded, 2*len(planned))
	for _, t := range planned {
		msgs[t.Removal.Message.ID()] = t.Removal
		msgs[t.Readd.Message.ID()] = t.Readd
	}

	c := &Collected{
		Signatures: make(map[ids.ID][]Signature, len(msgs)),
		PoPs:       make(map[string][bls.SignatureLen]byte),
	}
	skipped := make(map[ids.ID]int)
	for i, b := range bundles {
		if b.Version != BundleVersion {
			return nil, fmt.Errorf("bundle %d: %w: %d", i, errBundleVersion, b.Version)
		}
		for _, bs := range b.Signers {
			pk, pop, err := parseBundleSigner(bs)
			if err != nil {
				return nil, fmt.Errorf("bundle %d: %w", i, err)
			}
			pkKey := string(bs.PublicKey)
			if _, ok := c.PoPs[pkKey]; ok {
				return nil, fmt.Errorf("bundle %d: %w: 0x%x", i, errDuplicateBundleKey, bs.PublicKey)
			}
			c.PoPs[pkKey] = pop

			for msgID, sigBytes := range bs.Signatures {
				d, ok := msgs[msgID]
				if !ok {
					skipped[msgID]++
					continue
				}
				blsSig, err := bls.SignatureFromBytes(sigBytes)
				if err != nil {
					return nil, fmt.Errorf("bundle %d: %w: key 0x%x: %w", i, errInvalidBlob, bs.PublicKey, err)
				}
				sig := Signature{
					PublicKey: pk,
					Signature: blsSig,
				}
				if !sig.Verify(d.Message) {
					return nil, fmt.Errorf("bundle %d: %w: key 0x%x, message %s", i, errInvalidSignature, bs.PublicKey, msgID)
				}
				c.Signatures[msgID] = append(c.Signatures[msgID], sig)
			}
		}
	}
	for _, msgID := range slices.SortedFunc(maps.Keys(skipped), ids.ID.Compare) {
		c.Skipped = append(c.Skipped, fmt.Errorf("%w: %s (%d signatures skipped)", errUnknownMessageID, msgID, skipped[msgID]))
	}
	return c, nil
}

func parseBundleSigner(bs BundleSigner) (*bls.PublicKey, [bls.SignatureLen]byte, error) {
	var pop signer.ProofOfPossession
	if len(bs.PublicKey) != bls.PublicKeyLen || len(bs.ProofOfPossession) != bls.SignatureLen {
		return nil, pop.ProofOfPossession, fmt.Errorf("%w: key 0x%x", errInvalidPoP, bs.PublicKey)
	}
	copy(pop.PublicKey[:], bs.PublicKey)
	copy(pop.ProofOfPossession[:], bs.ProofOfPossession)
	if err := pop.Verify(); err != nil {
		return nil, pop.ProofOfPossession, fmt.Errorf("%w: key 0x%x: %w", errInvalidPoP, bs.PublicKey, err)
	}
	return pop.Key(), pop.ProofOfPossession, nil
}
