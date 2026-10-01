package warp

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils/crypto/bls"
	"github.com/ava-labs/avalanchego/utils/crypto/bls/signer/localsigner"
	"github.com/ava-labs/avalanchego/vms/platformvm/warp/message"
	"github.com/ava-labs/avalanchego/vms/platformvm/warp/payload"

	avawarp "github.com/ava-labs/avalanchego/vms/platformvm/warp"
)

const testNetworkID = 5

var (
	testChainID = ids.GenerateTestID()
	testManager = bytes.Repeat([]byte{0x46}, 20)
)

func newWeightMessage(t *testing.T, nonce uint64) *avawarp.UnsignedMessage {
	t.Helper()
	p, err := message.NewL1ValidatorWeight(ids.GenerateTestID(), nonce, 0)
	if err != nil {
		t.Fatalf("message.NewL1ValidatorWeight() error = %v", err)
	}
	msg, err := NewUnsignedMessage(testNetworkID, testChainID, testManager, p)
	if err != nil {
		t.Fatalf("NewUnsignedMessage() error = %v", err)
	}
	return msg
}

func newSigner(t *testing.T) *localsigner.LocalSigner {
	t.Helper()
	s, err := localsigner.New()
	if err != nil {
		t.Fatalf("localsigner.New() error = %v", err)
	}
	return s
}

func TestDecodeRoundTrip(t *testing.T) {
	sk := newSigner(t)
	var pk [bls.PublicKeyLen]byte
	copy(pk[:], bls.PublicKeyToCompressedBytes(sk.PublicKey()))
	owner := message.PChainOwner{
		Threshold: 1,
		Addresses: []ids.ShortID{ids.GenerateTestShortID()},
	}

	register, err := message.NewRegisterL1Validator(
		ids.GenerateTestID(),
		ids.GenerateTestNodeID(),
		pk,
		1_800_000_000,
		owner,
		owner,
		100,
	)
	if err != nil {
		t.Fatalf("message.NewRegisterL1Validator() error = %v", err)
	}
	weight, err := message.NewL1ValidatorWeight(ids.GenerateTestID(), 3, 0)
	if err != nil {
		t.Fatalf("message.NewL1ValidatorWeight() error = %v", err)
	}

	tests := []struct {
		name    string
		payload message.Payload
	}{
		{
			name:    "register",
			payload: register,
		},
		{
			name:    "weight",
			payload: weight,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := NewUnsignedMessage(testNetworkID, testChainID, testManager, tt.payload)
			if err != nil {
				t.Fatalf("NewUnsignedMessage() error = %v", err)
			}

			got, err := Decode(msg.Bytes())
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			if got.Message.NetworkID != testNetworkID {
				t.Errorf("NetworkID = %d, want %d", got.Message.NetworkID, testNetworkID)
			}
			if got.Message.SourceChainID != testChainID {
				t.Errorf("SourceChainID = %s, want %s", got.Message.SourceChainID, testChainID)
			}
			if !bytes.Equal(got.SourceAddress, testManager) {
				t.Errorf("SourceAddress = %x, want %x", got.SourceAddress, testManager)
			}
			if !bytes.Equal(got.Payload.Bytes(), tt.payload.Bytes()) {
				t.Errorf("Payload = %x, want %x", got.Payload.Bytes(), tt.payload.Bytes())
			}
			if !bytes.Equal(got.Message.Bytes(), msg.Bytes()) {
				t.Errorf("Message = %x, want %x", got.Message.Bytes(), msg.Bytes())
			}
		})
	}
}

func TestNewUnsignedMessageRejectsInvalidPayload(t *testing.T) {
	sk := newSigner(t)
	var pk [bls.PublicKeyLen]byte
	copy(pk[:], bls.PublicKeyToCompressedBytes(sk.PublicKey()))
	p, err := message.NewRegisterL1Validator(
		ids.GenerateTestID(),
		ids.GenerateTestNodeID(),
		pk,
		1_800_000_000,
		message.PChainOwner{},
		message.PChainOwner{},
		0,
	)
	if err != nil {
		t.Fatalf("message.NewRegisterL1Validator() error = %v", err)
	}

	_, err = NewUnsignedMessage(testNetworkID, testChainID, testManager, p)
	if !errors.Is(err, message.ErrInvalidWeight) {
		t.Fatalf("NewUnsignedMessage() error = %v, want %v", err, message.ErrInvalidWeight)
	}
}

// TestDecodeRejectsUnsupportedPayload checks that a signer cannot be asked to
// sign a valid Warp message that carries some other payload.
func TestDecodeRejectsUnsupportedPayload(t *testing.T) {
	p, err := message.NewL1ValidatorRegistration(ids.GenerateTestID(), true)
	if err != nil {
		t.Fatalf("message.NewL1ValidatorRegistration() error = %v", err)
	}
	_, err = NewUnsignedMessage(testNetworkID, testChainID, testManager, p)
	if !errors.Is(err, errUnsupportedPayload) {
		t.Fatalf("NewUnsignedMessage() error = %v, want %v", err, errUnsupportedPayload)
	}

	// Build the same message without the payload check, as a hostile
	// coordinator could.
	call, err := payload.NewAddressedCall(testManager, p.Bytes())
	if err != nil {
		t.Fatalf("payload.NewAddressedCall() error = %v", err)
	}
	msg, err := avawarp.NewUnsignedMessage(testNetworkID, testChainID, call.Bytes())
	if err != nil {
		t.Fatalf("avawarp.NewUnsignedMessage() error = %v", err)
	}
	_, err = Decode(msg.Bytes())
	if !errors.Is(err, errUnsupportedPayload) {
		t.Fatalf("Decode() error = %v, want %v", err, errUnsupportedPayload)
	}
}

func TestSignatureRoundTrip(t *testing.T) {
	sk := newSigner(t)
	msg := newWeightMessage(t, 1)

	sig, err := Sign(sk, msg)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	got, err := ParseSignature(sig.String())
	if err != nil {
		t.Fatalf("ParseSignature() error = %v", err)
	}
	if !got.PublicKey.Equals(sk.PublicKey()) {
		t.Fatal("ParseSignature() public key does not match the signer")
	}
	if !got.Verify(msg) {
		t.Fatal("Verify() = false for the signed message")
	}
	if got.Verify(newWeightMessage(t, 2)) {
		t.Fatal("Verify() = true for a different message")
	}

	other := got
	other.PublicKey = newSigner(t).PublicKey()
	if other.Verify(msg) {
		t.Fatal("Verify() = true for a different public key")
	}
}

func TestParseSignatureRejectsMalformedBlob(t *testing.T) {
	sig, err := Sign(newSigner(t), newWeightMessage(t, 1))
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	blob := sig.String()

	tests := []struct {
		name string
		blob string
	}{
		{
			name: "no_separator",
			blob: "abcd",
		},
		{
			name: "truncated_signature",
			blob: blob[:len(blob)-2],
		},
		{
			name: "not_hex",
			blob: "zz" + blob[2:],
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSignature(tt.blob)
			if !errors.Is(err, errInvalidBlob) {
				t.Fatalf("ParseSignature() error = %v, want %v", err, errInvalidBlob)
			}
		})
	}
}

// newValidatorSet returns the canonical set for validators with the given
// weights, and the signers in canonical order.
func newValidatorSet(t *testing.T, weights ...uint64) (validators.WarpSet, []*localsigner.LocalSigner) {
	t.Helper()
	byKey := make(map[string]*localsigner.LocalSigner, len(weights))
	vdrs := make(map[ids.NodeID]*validators.GetValidatorOutput, len(weights))
	for _, w := range weights {
		sk := newSigner(t)
		nodeID := ids.GenerateTestNodeID()
		vdrs[nodeID] = &validators.GetValidatorOutput{
			NodeID:    nodeID,
			PublicKey: sk.PublicKey(),
			Weight:    w,
		}
		byKey[string(bls.PublicKeyToUncompressedBytes(sk.PublicKey()))] = sk
	}

	set, err := validators.FlattenValidatorSet(vdrs)
	if err != nil {
		t.Fatalf("validators.FlattenValidatorSet() error = %v", err)
	}
	signers := make([]*localsigner.LocalSigner, len(set.Validators))
	for i, vdr := range set.Validators {
		signers[i] = byKey[string(vdr.PublicKeyBytes)]
	}
	return set, signers
}

func TestAggregate(t *testing.T) {
	msg := newWeightMessage(t, 1)

	tests := []struct {
		name    string
		weights []uint64
		// signerWeights selects the signers by weight, in the order the
		// coordinator passes them. Weights are unique within a test.
		signerWeights []uint64
		wantErr       error
	}{
		{
			name:          "exactly_67_percent",
			weights:       []uint64{67, 33},
			signerWeights: []uint64{67},
		},
		{
			name:          "below_67_percent",
			weights:       []uint64{66, 34},
			signerWeights: []uint64{66},
			wantErr:       avawarp.ErrInsufficientWeight,
		},
		{
			name:          "all_signers",
			weights:       []uint64{10, 20, 30, 40},
			signerWeights: []uint64{10, 20, 30, 40},
		},
		{
			name:          "unordered_subset",
			weights:       []uint64{10, 20, 30, 40},
			signerWeights: []uint64{40, 10, 30},
		},
		{
			name:          "duplicate_signer",
			weights:       []uint64{10, 20, 30, 40},
			signerWeights: []uint64{40, 40, 30},
			wantErr:       errDuplicateSigner,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vdrs, signers := newValidatorSet(t, tt.weights...)

			var (
				sigs        = make([]Signature, len(tt.signerWeights))
				wantSigners []int
				wantWeight  uint64
			)
			for i, w := range tt.signerWeights {
				idx := slices.IndexFunc(vdrs.Validators, func(v *validators.Warp) bool {
					return v.Weight == w
				})
				sig, err := Sign(signers[idx], msg)
				if err != nil {
					t.Fatalf("Sign() error = %v", err)
				}
				sigs[i] = sig
				wantSigners = append(wantSigners, idx)
				wantWeight += w
			}
			slices.Sort(wantSigners)

			got, err := Aggregate(msg, vdrs, sigs)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Aggregate() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if !slices.Equal(got.Signers, wantSigners) {
				t.Errorf("Signers = %v, want %v", got.Signers, wantSigners)
			}
			if got.SignedWeight != wantWeight {
				t.Errorf("SignedWeight = %d, want %d", got.SignedWeight, wantWeight)
			}
			if got.TotalWeight != vdrs.TotalWeight {
				t.Errorf("TotalWeight = %d, want %d", got.TotalWeight, vdrs.TotalWeight)
			}

			// Verify the output exactly as the P-Chain does.
			parsed, err := avawarp.ParseMessage(got.Message.Bytes())
			if err != nil {
				t.Fatalf("avawarp.ParseMessage() error = %v", err)
			}
			err = parsed.Signature.Verify(
				&parsed.UnsignedMessage,
				testNetworkID,
				vdrs,
				QuorumNumerator,
				QuorumDenominator,
			)
			if err != nil {
				t.Fatalf("%T.Verify() error = %v", parsed.Signature, err)
			}
		})
	}
}

func TestAggregateRejectsBadSigners(t *testing.T) {
	msg := newWeightMessage(t, 1)
	vdrs, signers := newValidatorSet(t, 10, 20, 30, 40)

	sign := func(s *localsigner.LocalSigner, m *avawarp.UnsignedMessage) Signature {
		sig, err := Sign(s, m)
		if err != nil {
			t.Fatalf("Sign() error = %v", err)
		}
		return sig
	}

	tests := []struct {
		name    string
		sigs    []Signature
		wantErr error
	}{
		{
			name:    "no_signatures",
			wantErr: errNoSignatures,
		},
		{
			name: "unknown_signer",
			sigs: []Signature{
				sign(signers[3], msg),
				sign(signers[2], msg),
				sign(newSigner(t), msg),
			},
			wantErr: errUnknownSigner,
		},
		{
			name: "signature_over_other_message",
			sigs: []Signature{
				sign(signers[3], msg),
				sign(signers[2], newWeightMessage(t, 2)),
			},
			wantErr: errInvalidSignature,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Aggregate(msg, vdrs, tt.sigs)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Aggregate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
