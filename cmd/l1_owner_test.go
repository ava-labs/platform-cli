package cmd

import (
	"errors"
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/vms/platformvm"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
)

func TestCheckCanDisableL1Validator(t *testing.T) {
	signer := ids.GenerateTestShortID()
	other := ids.GenerateTestShortID()

	tests := []struct {
		name    string
		balance uint64
		owner   *secp256k1fx.OutputOwners
		wantErr error
	}{
		{
			name:    "signer_is_owner",
			balance: 1,
			owner: &secp256k1fx.OutputOwners{
				Threshold: 1,
				Addrs:     []ids.ShortID{signer},
			},
		},
		{
			name:    "empty_owner",
			balance: 1,
			owner:   &secp256k1fx.OutputOwners{},
		},
		{
			name:    "signer_is_not_owner",
			balance: 1,
			owner: &secp256k1fx.OutputOwners{
				Threshold: 1,
				Addrs:     []ids.ShortID{other},
			},
			wantErr: errNotDeactivationOwner,
		},
		{
			name:    "multisig_owner",
			balance: 1,
			owner: &secp256k1fx.OutputOwners{
				Threshold: 2,
				Addrs:     []ids.ShortID{signer, other},
			},
			wantErr: errMultisigDeactivationOwner,
		},
		{
			name: "inactive_validator",
			owner: &secp256k1fx.OutputOwners{
				Threshold: 1,
				Addrs:     []ids.ShortID{signer},
			},
			wantErr: errL1ValidatorInactive,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := platformvm.L1Validator{
				DeactivationOwner: tt.owner,
				Balance:           tt.balance,
			}
			err := checkCanDisableL1Validator(validator, signer, constants.FujiID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("checkCanDisableL1Validator() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
