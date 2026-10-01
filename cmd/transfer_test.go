package cmd

import (
	"errors"
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/formatting/address"
)

func TestParseDestinationAddress(t *testing.T) {
	hrp := constants.FujiHRP
	addr := ids.GenerateTestShortID()
	format := func(chain, hrp string, addr ids.ShortID) string {
		formatted, err := address.Format(chain, hrp, addr[:])
		if err != nil {
			t.Fatalf("address.Format() error = %v", err)
		}
		return formatted
	}

	tests := []struct {
		name    string
		input   string
		want    ids.ShortID
		wantErr error
	}{
		{
			name:  "p_chain_address",
			input: format("P", hrp, addr),
			want:  addr,
		},
		{
			name:  "raw_short_id",
			input: addr.String(),
			want:  addr,
		},
		{
			name:    "x_chain_address",
			input:   format("X", hrp, addr),
			wantErr: errNotPChainAddress,
		},
		{
			name:    "other_network",
			input:   format("P", constants.MainnetHRP, addr),
			wantErr: errWrongNetworkAddress,
		},
		{
			name:    "zero_address",
			input:   format("P", hrp, ids.ShortEmpty),
			wantErr: errZeroDestinationAddress,
		},
		{
			name:    "zero_short_id",
			input:   ids.ShortEmpty.String(),
			wantErr: errZeroDestinationAddress,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDestinationAddress(tt.input, hrp)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("parseDestinationAddress() error = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("parseDestinationAddress() = %s, want %s", got, tt.want)
			}
		})
	}
}
