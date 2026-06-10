package cmd

import (
	"bytes"
	"context"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto/bls"
	"github.com/ava-labs/avalanchego/utils/crypto/bls/signer/localsigner"
	"github.com/ava-labs/avalanchego/vms/platformvm/signer"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
)

// newTestPoP generates a valid BLS proof of possession for tests.
func newTestPoP(t *testing.T) *signer.ProofOfPossession {
	t.Helper()
	blsSigner, err := localsigner.New()
	if err != nil {
		t.Fatalf("localsigner.New() error = %v", err)
	}
	pop, err := signer.NewProofOfPossession(blsSigner)
	if err != nil {
		t.Fatalf("signer.NewProofOfPossession() error = %v", err)
	}
	return pop
}

func TestParseValidatorAddrs(t *testing.T) {
	got := parseValidatorAddrs(" 127.0.0.1 , node.example.com:9650 ,,https://node.example.com ")
	want := []string{"127.0.0.1", "node.example.com:9650", "https://node.example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseValidatorAddrs() = %#v, want %#v", got, want)
	}

	got = parseValidatorAddrs("")
	if len(got) != 0 {
		t.Fatalf("parseValidatorAddrs(\"\") = %#v, want empty slice", got)
	}

	got = parseValidatorAddrs(" , ,  , ")
	if len(got) != 0 {
		t.Fatalf("parseValidatorAddrs(\" , ,  , \") = %#v, want empty slice", got)
	}
}

func TestParseValidatorWeights(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []uint64
		wantErr bool
	}{
		{
			name:  "single weight",
			input: "100",
			want:  []uint64{100},
		},
		{
			name:  "multiple weights",
			input: "100,200,300",
			want:  []uint64{100, 200, 300},
		},
		{
			name:  "weights with spaces",
			input: " 100 , 200 , 300 ",
			want:  []uint64{100, 200, 300},
		},
		{
			name:  "minimum weight",
			input: "1",
			want:  []uint64{1},
		},
		{
			name:  "large weight",
			input: "18446744073709551615",
			want:  []uint64{18446744073709551615},
		},
		{
			name:    "uint64 overflow",
			input:   "18446744073709551616",
			wantErr: true,
		},
		{
			name:    "zero weight",
			input:   "100,0,300",
			wantErr: true,
		},
		{
			name:    "non-numeric",
			input:   "100,abc,300",
			wantErr: true,
		},
		{
			name:    "negative value",
			input:   "100,-1,300",
			wantErr: true,
		},
		{
			name:    "decimal value",
			input:   "1.5",
			wantErr: true,
		},
		{
			name:  "empty string returns nil",
			input: "",
			want:  nil,
		},
		{
			name:  "only commas returns nil",
			input: " , , ",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseValidatorWeights(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseValidatorWeights(%q) expected error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseValidatorWeights(%q) returned error: %v", tt.input, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseValidatorWeights(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseManualPoP(t *testing.T) {
	pop := newTestPoP(t)
	pubHex := hex.EncodeToString(pop.PublicKey[:])
	popHex := hex.EncodeToString(pop.ProofOfPossession[:])

	got, err := parseManualPoP(pubHex, popHex)
	if err != nil {
		t.Fatalf("parseManualPoP() error = %v", err)
	}
	if !bytes.Equal(got.PublicKey[:], pop.PublicKey[:]) {
		t.Fatalf("parseManualPoP() public key mismatch: got %x, want %x", got.PublicKey, pop.PublicKey)
	}
	if !bytes.Equal(got.ProofOfPossession[:], pop.ProofOfPossession[:]) {
		t.Fatalf("parseManualPoP() proof mismatch: got %x, want %x", got.ProofOfPossession, pop.ProofOfPossession)
	}
}

func TestParseManualPoP_AcceptsPrefixAndWhitespace(t *testing.T) {
	pop := newTestPoP(t)
	pubHex := " 0x" + hex.EncodeToString(pop.PublicKey[:]) + " "
	popHex := " 0X" + hex.EncodeToString(pop.ProofOfPossession[:]) + " "

	if _, err := parseManualPoP(pubHex, popHex); err != nil {
		t.Fatalf("parseManualPoP() with prefix/whitespace error = %v", err)
	}
}

func TestParseManualPoP_Invalid(t *testing.T) {
	pop := newTestPoP(t)
	pubHex := hex.EncodeToString(pop.PublicKey[:])
	popHex := hex.EncodeToString(pop.ProofOfPossession[:])

	tests := []struct {
		name   string
		pubKey string
		pop    string
	}{
		{
			name:   "invalid public key hex",
			pubKey: "zz",
			pop:    popHex,
		},
		{
			name:   "invalid pop hex",
			pubKey: pubHex,
			pop:    "zz",
		},
		{
			name:   "short public key",
			pubKey: strings.Repeat("ab", bls.PublicKeyLen-1),
			pop:    popHex,
		},
		{
			name:   "short pop",
			pubKey: pubHex,
			pop:    strings.Repeat("ab", bls.SignatureLen-1),
		},
		{
			name:   "verification failure",
			pubKey: pubHex,
			pop:    strings.Repeat("ab", bls.SignatureLen),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseManualPoP(tt.pubKey, tt.pop); err == nil {
				t.Fatalf("parseManualPoP(%q, %q) expected error", tt.pubKey, tt.pop)
			}
		})
	}
}

func TestNormalizeNodeURI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"ip only", "127.0.0.1", "http://127.0.0.1:9650"},
		{"ip with port", "127.0.0.1:9650", "http://127.0.0.1:9650"},
		{"hostname shorthand defaults https", "mynode.example.com:9650", "https://mynode.example.com:9650"},
		{"http uri", "http://127.0.0.1:9650", "http://127.0.0.1:9650"},
		{"https uri", "https://example.com", "https://example.com"},
		{"ext info path is stripped", "http://127.0.0.1:9650/ext/info", "http://127.0.0.1:9650"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeNodeURI(tt.input)
			if err != nil {
				t.Fatalf("normalizeNodeURI(%q) returned error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("normalizeNodeURI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeNodeURI_InvalidPath(t *testing.T) {
	_, err := normalizeNodeURI("http://127.0.0.1:9650/custom/path")
	if err == nil {
		t.Fatal("normalizeNodeURI() expected error for custom path")
	}
}

func TestNormalizeNodeURI_InsecureHTTPOverride(t *testing.T) {
	origAllow := allowInsecureHTTP
	defer func() {
		allowInsecureHTTP = origAllow
	}()

	allowInsecureHTTP = false
	_, err := normalizeNodeURI("http://mynode.example.com:9650")
	if err == nil {
		t.Fatal("normalizeNodeURI() expected error for insecure non-local HTTP when override is disabled")
	}

	allowInsecureHTTP = true
	got, err := normalizeNodeURI("http://mynode.example.com:9650")
	if err != nil {
		t.Fatalf("normalizeNodeURI() returned error with override enabled: %v", err)
	}
	if got != "http://mynode.example.com:9650" {
		t.Fatalf("normalizeNodeURI() = %q, want %q", got, "http://mynode.example.com:9650")
	}
}

func TestBuildManualL1Validators(t *testing.T) {
	pop := newTestPoP(t)

	nodeID := ids.GenerateTestNodeID()
	pubHex := hex.EncodeToString(pop.PublicKey[:])
	popHex := hex.EncodeToString(pop.ProofOfPossession[:])

	validators, err := buildManualL1Validators(
		nodeID.String(),
		pubHex,
		popHex,
		1.5,
		nil,
	)
	if err != nil {
		t.Fatalf("buildManualL1Validators() error = %v", err)
	}
	if len(validators) != 1 {
		t.Fatalf("buildManualL1Validators() validators length = %d, want 1", len(validators))
	}
	if !bytes.Equal(validators[0].NodeID, nodeID.Bytes()) {
		t.Fatalf("validator NodeID mismatch: got %x, want %x", validators[0].NodeID, nodeID.Bytes())
	}
	if validators[0].Balance != 1_500_000_000 {
		t.Fatalf("validator balance = %d, want 1500000000", validators[0].Balance)
	}
	if validators[0].Weight != defaultValidatorWeight {
		t.Fatalf("validator weight = %d, want default %d", validators[0].Weight, defaultValidatorWeight)
	}
}

func TestBuildManualL1Validators_MissingInputs(t *testing.T) {
	_, err := buildManualL1Validators("", "deadbeef", "beadfeed", 1, nil)
	if err == nil {
		t.Fatal("buildManualL1Validators() expected error for empty node IDs")
	}

	_, err = buildManualL1Validators("NodeID-1", "", "beadfeed", 1, nil)
	if err == nil {
		t.Fatal("buildManualL1Validators() expected error for empty BLS public keys")
	}

	_, err = buildManualL1Validators("NodeID-1", "deadbeef", "", 1, nil)
	if err == nil {
		t.Fatal("buildManualL1Validators() expected error for empty BLS PoPs")
	}
}

func TestBuildManualL1Validators_MismatchLengths(t *testing.T) {
	_, err := buildManualL1Validators(
		"NodeID-1,NodeID-2",
		"deadbeef",
		"beadfeed",
		1,
		nil,
	)
	if err == nil {
		t.Fatal("buildManualL1Validators() expected error for mismatched list lengths")
	}
}

func TestBuildManualL1Validators_InvalidNodeID(t *testing.T) {
	pop := newTestPoP(t)

	pubHex := hex.EncodeToString(pop.PublicKey[:])
	popHex := hex.EncodeToString(pop.ProofOfPossession[:])

	_, err := buildManualL1Validators("NodeID-not-real", pubHex, popHex, 1, nil)
	if err == nil {
		t.Fatal("buildManualL1Validators() expected error for invalid NodeID")
	}
}

func TestBuildManualL1Validators_InvalidBalance(t *testing.T) {
	pop := newTestPoP(t)

	nodeID := ids.GenerateTestNodeID()
	pubHex := hex.EncodeToString(pop.PublicKey[:])
	popHex := hex.EncodeToString(pop.ProofOfPossession[:])

	_, err := buildManualL1Validators(nodeID.String(), pubHex, popHex, -1, nil)
	if err == nil {
		t.Fatal("buildManualL1Validators() expected error for negative balance")
	}
}

func TestBuildManualL1Validators_WithWeights(t *testing.T) {
	// Create two validators with different weights
	pop1 := newTestPoP(t)
	pop2 := newTestPoP(t)

	nodeID1 := ids.GenerateTestNodeID()
	nodeID2 := ids.GenerateTestNodeID()
	pubHex1 := hex.EncodeToString(pop1.PublicKey[:])
	popHex1 := hex.EncodeToString(pop1.ProofOfPossession[:])
	pubHex2 := hex.EncodeToString(pop2.PublicKey[:])
	popHex2 := hex.EncodeToString(pop2.ProofOfPossession[:])

	nodeIDs := nodeID1.String() + "," + nodeID2.String()
	blsPubs := pubHex1 + "," + pubHex2
	blsPops := popHex1 + "," + popHex2

	validators, err := buildManualL1Validators(nodeIDs, blsPubs, blsPops, 1.0, []uint64{1000, 2000})
	if err != nil {
		t.Fatalf("buildManualL1Validators() error = %v", err)
	}
	if len(validators) != 2 {
		t.Fatalf("buildManualL1Validators() validators length = %d, want 2", len(validators))
	}
	if validators[0].Weight != 1000 {
		t.Fatalf("validator[0].Weight = %d, want 1000", validators[0].Weight)
	}
	if validators[1].Weight != 2000 {
		t.Fatalf("validator[1].Weight = %d, want 2000", validators[1].Weight)
	}
}

func TestBuildManualL1Validators_WeightsMismatch(t *testing.T) {
	pop := newTestPoP(t)

	nodeID := ids.GenerateTestNodeID()
	pubHex := hex.EncodeToString(pop.PublicKey[:])
	popHex := hex.EncodeToString(pop.ProofOfPossession[:])

	// 1 validator but 2 weights => error
	_, err := buildManualL1Validators(nodeID.String(), pubHex, popHex, 1.0, []uint64{1000, 2000})
	if err == nil {
		t.Fatal("buildManualL1Validators() expected error for weights count mismatch")
	}
}

func TestGatherL1Validators_InputValidation(t *testing.T) {
	ctx := context.Background()

	// No addresses provided.
	_, err := gatherL1Validators(ctx, nil, 1, nil)
	if err == nil {
		t.Fatal("gatherL1Validators() expected error for empty validator addresses")
	}

	// Weights count mismatch.
	_, err = gatherL1Validators(ctx, []string{"127.0.0.1", "127.0.0.2"}, 1, []uint64{100})
	if err == nil {
		t.Fatal("gatherL1Validators() expected error for weights count mismatch")
	}

	// Negative balance.
	_, err = gatherL1Validators(ctx, []string{"127.0.0.1"}, -1, nil)
	if err == nil {
		t.Fatal("gatherL1Validators() expected error for negative balance")
	}

	// Invalid validator address (rejected before any network call).
	_, err = gatherL1Validators(ctx, []string{"http://127.0.0.1:9650/custom/path"}, 1, nil)
	if err == nil {
		t.Fatal("gatherL1Validators() expected error for invalid validator address")
	}
}

func TestSortAndValidateL1Validators_SortsByNodeID(t *testing.T) {
	v1 := &txs.ConvertSubnetToL1Validator{NodeID: []byte{0x02}, Weight: 1}
	v2 := &txs.ConvertSubnetToL1Validator{NodeID: []byte{0x01}, Weight: 1}
	validators := []*txs.ConvertSubnetToL1Validator{v1, v2}

	if err := sortAndValidateL1Validators(validators); err != nil {
		t.Fatalf("sortAndValidateL1Validators() error = %v", err)
	}
	if !bytes.Equal(validators[0].NodeID, []byte{0x01}) || !bytes.Equal(validators[1].NodeID, []byte{0x02}) {
		t.Fatalf("validators not sorted by NodeID bytes: got [%x, %x]", validators[0].NodeID, validators[1].NodeID)
	}
}

func TestSortAndValidateL1Validators_DuplicateNodeID(t *testing.T) {
	validators := []*txs.ConvertSubnetToL1Validator{
		{NodeID: []byte{0x01}, Weight: 1},
		{NodeID: []byte{0x01}, Weight: 1},
	}

	err := sortAndValidateL1Validators(validators)
	if err == nil {
		t.Fatal("sortAndValidateL1Validators() expected duplicate NodeID error")
	}
}

func TestGenerateMockValidator(t *testing.T) {
	// Explicit weight is used as-is.
	v, err := generateMockValidator(1.5, 42)
	if err != nil {
		t.Fatalf("generateMockValidator() error = %v", err)
	}
	if v.Weight != 42 {
		t.Fatalf("generateMockValidator() weight = %d, want 42", v.Weight)
	}
	if v.Balance != 1_500_000_000 {
		t.Fatalf("generateMockValidator() balance = %d, want 1500000000", v.Balance)
	}
	if len(v.NodeID) != ids.NodeIDLen {
		t.Fatalf("generateMockValidator() NodeID length = %d, want %d", len(v.NodeID), ids.NodeIDLen)
	}
	if err := v.Signer.Verify(); err != nil {
		t.Fatalf("generateMockValidator() produced invalid proof of possession: %v", err)
	}

	// Zero weight falls back to the default.
	v, err = generateMockValidator(1, 0)
	if err != nil {
		t.Fatalf("generateMockValidator() error = %v", err)
	}
	if v.Weight != defaultValidatorWeight {
		t.Fatalf("generateMockValidator() weight = %d, want default %d", v.Weight, defaultValidatorWeight)
	}

	// Negative balance is rejected.
	if _, err := generateMockValidator(-1, 0); err == nil {
		t.Fatal("generateMockValidator() expected error for negative balance")
	}
}
