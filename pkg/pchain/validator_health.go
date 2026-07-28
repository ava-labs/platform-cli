package pchain

import (
	"context"
	"fmt"
	"strconv"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/vms/platformvm"
)

// ValidatorHealth is how the network currently sees a primary network validator.
//
// It matters most for auto-renewed validators: renewal is conditioned on reward
// eligibility, so a validator the network cannot reach is removed at the next
// cycle boundary rather than renewed, and forfeits that cycle's rewards.
type ValidatorHealth struct {
	// Found is false when nodeID is not in the current validator set.
	Found     bool
	Connected bool
	// UptimePercent is the queried node's view of this validator's uptime. It is
	// 0 for a validator that has only just started validating, so it is only
	// meaningful once the validator has been in the set for a while.
	UptimePercent float64
}

// GetValidatorHealth reports the connectivity and uptime that
// platform.getCurrentValidators returns for nodeID on the primary network.
func GetValidatorHealth(ctx context.Context, rpcURL string, nodeID ids.NodeID) (*ValidatorHealth, error) {
	client := platformvm.NewClient(rpcURL)
	args := &platformvm.GetCurrentValidatorsArgs{
		SubnetID: constants.PrimaryNetworkID,
		NodeIDs:  []ids.NodeID{nodeID},
	}
	reply := &getCurrentValidatorsHealthReply{}
	if err := client.Requester.SendRequest(ctx, "platform.getCurrentValidators", args, reply); err != nil {
		return nil, fmt.Errorf("failed to fetch current validators: %w", err)
	}

	for _, validator := range reply.Validators {
		if validator.NodeID != nodeID.String() {
			continue
		}
		health := &ValidatorHealth{
			Found:     true,
			Connected: validator.Connected,
		}
		// Uptime is omitted for subnet validators and may be absent on nodes that
		// do not track it; treat a missing value as 0 rather than an error.
		if validator.Uptime != "" {
			uptime, err := strconv.ParseFloat(validator.Uptime, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid uptime %q for %s: %w", validator.Uptime, nodeID, err)
			}
			health.UptimePercent = uptime
		}
		return health, nil
	}

	return &ValidatorHealth{}, nil
}

type getCurrentValidatorsHealthReply struct {
	Validators []validatorHealthEntry `json:"validators"`
}

type validatorHealthEntry struct {
	NodeID    string `json:"nodeID"`
	Connected bool   `json:"connected"`
	Uptime    string `json:"uptime"`
}
