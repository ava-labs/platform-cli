package pchain

import (
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto/secp256k1"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
)

// makePartialTx builds a TransferSubnetOwnershipTx whose subnet auth expects
// sigIndices signatures, with signedSlots of them filled with a fake sig.
func makePartialTx(t *testing.T, sigIndices []uint32, signedSlots []int) *txs.Tx {
	t.Helper()

	utx := &txs.TransferSubnetOwnershipTx{
		Subnet: ids.GenerateTestID(),
		SubnetAuth: &secp256k1fx.Input{
			SigIndices: sigIndices,
		},
		Owner: &secp256k1fx.OutputOwners{
			Threshold: 1,
			Addrs:     []ids.ShortID{ids.GenerateTestShortID()},
		},
	}
	tx, err := txs.NewSigned(utx, txs.Codec, nil)
	if err != nil {
		t.Fatalf("failed to create tx: %v", err)
	}

	cred := &secp256k1fx.Credential{
		Sigs: make([][secp256k1.SignatureLen]byte, len(sigIndices)),
	}
	for _, slot := range signedSlots {
		cred.Sigs[slot][0] = 0xFF // any non-zero byte marks the slot signed
	}
	tx.Creds = append(tx.Creds, cred)
	return tx
}

func TestSubnetAuthSigners(t *testing.T) {
	controlKeys := []ids.ShortID{
		ids.GenerateTestShortID(),
		ids.GenerateTestShortID(),
		ids.GenerateTestShortID(),
	}

	// 2-of-3 auth over control keys 0 and 2; only slot 0 signed
	tx := makePartialTx(t, []uint32{0, 2}, []int{0})

	signed, remaining, err := SubnetAuthSigners(tx, controlKeys)
	if err != nil {
		t.Fatalf("SubnetAuthSigners() returned error: %v", err)
	}
	if len(signed) != 1 || signed[0] != controlKeys[0] {
		t.Fatalf("signed = %v, want [%s]", signed, controlKeys[0])
	}
	if len(remaining) != 1 || remaining[0] != controlKeys[2] {
		t.Fatalf("remaining = %v, want [%s]", remaining, controlKeys[2])
	}

	// All slots signed
	tx = makePartialTx(t, []uint32{0, 2}, []int{0, 1})
	_, remaining, err = SubnetAuthSigners(tx, controlKeys)
	if err != nil {
		t.Fatalf("SubnetAuthSigners() returned error: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining = %v, want none", remaining)
	}
}

func TestSubnetAuthSignersIndexOutOfRange(t *testing.T) {
	// Auth references control key index 5 but the owner has 1 key: the tx was
	// built against a different owner set.
	tx := makePartialTx(t, []uint32{5}, nil)
	if _, _, err := SubnetAuthSigners(tx, []ids.ShortID{ids.GenerateTestShortID()}); err == nil {
		t.Fatal("SubnetAuthSigners() expected error for out-of-range index, got nil")
	}
}

func TestSelectAuthSigners(t *testing.T) {
	owners := []ids.ShortID{
		{0x01}, {0x02}, {0x03},
	}

	tests := []struct {
		name      string
		threshold uint32
		local     []ids.ShortID
		want      []ids.ShortID
		wantErr   bool
	}{
		{
			name:      "local owner preferred over on-chain order",
			threshold: 2,
			local:     []ids.ShortID{{0x03}},
			want:      []ids.ShortID{{0x03}, {0x01}},
		},
		{
			name:      "no local owners takes first threshold",
			threshold: 2,
			local:     []ids.ShortID{{0xAA}},
			want:      []ids.ShortID{{0x01}, {0x02}},
		},
		{
			name:      "local keys cover threshold",
			threshold: 2,
			local:     []ids.ShortID{{0x02}, {0x03}},
			want:      []ids.ShortID{{0x02}, {0x03}},
		},
		{
			name:      "zero threshold rejected",
			threshold: 0,
			wantErr:   true,
		},
		{
			name:      "threshold above owner count rejected",
			threshold: 4,
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SelectAuthSigners(owners, tt.threshold, tt.local)
			if tt.wantErr {
				if err == nil {
					t.Fatal("SelectAuthSigners() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("SelectAuthSigners() returned error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("SelectAuthSigners() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("SelectAuthSigners() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestTxFileRoundTrip(t *testing.T) {
	tx := makePartialTx(t, []uint32{0, 1}, []int{1})

	data, err := EncodeTxFile(5, tx)
	if err != nil {
		t.Fatalf("EncodeTxFile() returned error: %v", err)
	}

	networkID, parsed, err := DecodeTxFile(data)
	if err != nil {
		t.Fatalf("DecodeTxFile() returned error: %v", err)
	}
	if networkID != 5 {
		t.Fatalf("networkID = %d, want 5", networkID)
	}
	if parsed.ID() != tx.ID() {
		t.Fatalf("parsed tx ID = %s, want %s", parsed.ID(), tx.ID())
	}
}

func TestDecodeTxFileCorrupted(t *testing.T) {
	tx := makePartialTx(t, []uint32{0}, nil)
	data, err := EncodeTxFile(5, tx)
	if err != nil {
		t.Fatalf("EncodeTxFile() returned error: %v", err)
	}
	// Flip a hex character inside the tx payload; the checksum must catch it.
	corrupted := []byte(string(data))
	for i := range corrupted {
		if corrupted[i] == '7' {
			corrupted[i] = '8'
			break
		}
	}
	if _, _, err := DecodeTxFile(corrupted); err == nil {
		t.Fatal("DecodeTxFile() expected error for corrupted file, got nil")
	}
}
