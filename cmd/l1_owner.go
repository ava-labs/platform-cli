package cmd

import (
	"errors"
	"fmt"
	"slices"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/vms/platformvm"
	"github.com/ava-labs/avalanchego/vms/platformvm/warp/message"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
)

var (
	errL1ValidatorInactive       = errors.New("L1 validator is already inactive")
	errNotDeactivationOwner      = errors.New("signer is not the deactivation owner")
	errMultisigDeactivationOwner = errors.New("deactivation owner needs more than one signature")
)

// checkCanDisableL1Validator returns an error if signer alone cannot disable
// validator, or if disabling it would only spend the fee.
func checkCanDisableL1Validator(validator platformvm.L1Validator, signer ids.ShortID, networkID uint32) error {
	// An inactive validator has no balance left to refund.
	if validator.Balance == 0 {
		return errL1ValidatorInactive
	}
	owner := validator.DeactivationOwner
	switch {
	case owner.Threshold == 0:
		return nil
	case owner.Threshold > 1:
		return fmt.Errorf("%w: %s", errMultisigDeactivationOwner, formatOutputOwners(owner, networkID))
	case !slices.Contains(owner.Addrs, signer):
		return fmt.Errorf("%w: deactivation owner is %s", errNotDeactivationOwner, formatOutputOwners(owner, networkID))
	default:
		return nil
	}
}

// formatOutputOwners returns a readable form of owner for the given network.
func formatOutputOwners(owner *secp256k1fx.OutputOwners, networkID uint32) string {
	return formatPChainOwner(message.PChainOwner{
		Threshold: owner.Threshold,
		Addresses: owner.Addrs,
	}, networkID)
}
