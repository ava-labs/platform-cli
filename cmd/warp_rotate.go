package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/crypto/bls"
	"github.com/ava-labs/avalanchego/vms/platformvm"
	"github.com/ava-labs/avalanchego/vms/platformvm/warp/message"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
	"github.com/ava-labs/libevm/common"
	"github.com/ava-labs/platform-cli/pkg/network"
	"github.com/ava-labs/platform-cli/pkg/pchain"
	"github.com/ava-labs/platform-cli/pkg/wallet"
	"github.com/ava-labs/platform-cli/pkg/warp"
	"github.com/spf13/cobra"

	platformapi "github.com/ava-labs/avalanchego/vms/platformvm/api"
)

const (
	defaultPlanExpiry       = 23 * time.Hour
	defaultWaitTimeout      = 5 * time.Minute
	defaultPollInterval     = 2 * time.Second
	defaultReaddExpiryLimit = 30 * time.Minute

	// A rate limit or a proposed height lag clears within about 30s. These
	// values retry for about 60s, then stop so a rerun resumes.
	retryAttempts   = 6
	retryBackoff    = 2 * time.Second
	retryMaxBackoff = 16 * time.Second
)

var (
	errWarpNoTargets       = errors.New("no validators to rotate")
	errWarpTargetNotFound  = errors.New("validation ID is not a current validator of the subnet")
	errWarpNetworkMismatch = errors.New("network ID does not match the plan")
)

var (
	warpPlanPath      string
	warpPlanOut       string
	warpBundleOut     string
	warpBalance       float64
	warpValidationIDs string
	warpBundlePaths   []string
	warpWaitTimeout   time.Duration
	warpPollInterval  time.Duration
	warpExpiryMargin  time.Duration
)

var warpPlanCmd = &cobra.Command{
	Use:   "plan",
	Short: "Build every removal and re-add message for an L1 validator rotation",
	Long: `Build the rotation plan for the validators of --subnet-id.

The command reads the live validator set and selects every L1 validator whose
deactivation owner is empty, or the validators in --validation-ids. For each
target it builds:

  removal  L1ValidatorWeight, weight 0, nonce MaxUint64
  re-add   RegisterL1Validator with the same node ID, BLS key, and weight,
           and the new --remaining-balance-owner and --deactivation-owner

The plan file also holds a snapshot of the current canonical set. Give the
plan to every signing machine ("warp sign --plan"), then run "warp rotate".
The re-adds expire at --expiry, so finish the rotation before then.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

		if warpSubnetID == "" || warpManagerBlockchainID == "" || warpManagerAddress == "" {
			return fmt.Errorf("%w: --subnet-id, --manager-blockchain-id, and --manager-address", errWarpMissingFlag)
		}
		if warpRemainingBalanceOwner == "" || warpDeactivationOwner == "" {
			return fmt.Errorf("%w: --remaining-balance-owner and --deactivation-owner", errWarpMissingFlag)
		}
		if warpBalance <= 0 {
			return fmt.Errorf("%w: --balance must be positive", errWarpMissingFlag)
		}
		subnetID, err := ids.FromString(warpSubnetID)
		if err != nil {
			return fmt.Errorf("invalid --subnet-id: %w", err)
		}
		managerChainID, err := ids.FromString(warpManagerBlockchainID)
		if err != nil {
			return fmt.Errorf("invalid --manager-blockchain-id: %w", err)
		}
		managerAddress, err := decodeHexExactLength(warpManagerAddress, common.AddressLength)
		if err != nil {
			return fmt.Errorf("invalid --manager-address: %w", err)
		}
		balance, err := avaxToNAVAX(warpBalance)
		if err != nil {
			return fmt.Errorf("invalid --balance: %w", err)
		}

		uri, err := pChainURI()
		if err != nil {
			return err
		}
		networkID := customNetID
		if networkID == 0 {
			networkID, err = network.GetNetworkID(ctx, uri)
			if err != nil {
				return err
			}
		}
		hrp := constants.GetHRP(networkID)
		remainingBalanceOwner, err := parsePChainAddress(warpRemainingBalanceOwner, hrp)
		if err != nil {
			return fmt.Errorf("invalid --remaining-balance-owner: %w", err)
		}
		deactivationOwner, err := parsePChainAddress(warpDeactivationOwner, hrp)
		if err != nil {
			return fmt.Errorf("invalid --deactivation-owner: %w", err)
		}

		now := time.Now()
		expiry := warpExpiry
		if expiry == 0 {
			expiry = uint64(now.Add(defaultPlanExpiry).Unix())
		}
		if err := verifyRegisterExpiry(expiry, now); err != nil {
			return fmt.Errorf("invalid --expiry: %w", err)
		}

		client := platformvm.NewClient(uri)
		height, vdrs, err := proposedValidatorSet(ctx, client, subnetID)
		if err != nil {
			return err
		}
		targets, err := selectTargets(ctx, client, subnetID)
		if err != nil {
			return err
		}

		p, err := warp.NewPlan(warp.PlanConfig{
			NetworkID:             networkID,
			SubnetID:              subnetID,
			ManagerBlockchainID:   managerChainID,
			ManagerAddress:        managerAddress,
			RemainingBalanceOwner: validatorOwner([]ids.ShortID{remainingBalanceOwner}, 0, ids.ShortEmpty),
			DeactivationOwner:     validatorOwner([]ids.ShortID{deactivationOwner}, 0, ids.ShortEmpty),
			Balance:               balance,
			Expiry:                expiry,
			SnapshotHeight:        height,
			Snapshot:              vdrs,
		}, targets)
		if err != nil {
			return fmt.Errorf("failed to build plan: %w", err)
		}
		planned, err := p.Decode()
		if err != nil {
			return fmt.Errorf("failed to decode built plan: %w", err)
		}

		printPlanSummary(p, planned, now)
		if err := writeNewJSONFile(warpPlanOut, p); err != nil {
			return err
		}
		fmt.Printf("\nWrote plan: %s\n", warpPlanOut)
		return nil
	},
}

// selectTargets returns the validators in --validation-ids, or every L1
// validator of subnetID whose deactivation owner is empty.
func selectTargets(ctx context.Context, client *platformvm.Client, subnetID ids.ID) ([]warp.TargetValidator, error) {
	current, err := client.GetCurrentValidators(ctx, subnetID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get current validators: %w", err)
	}
	empty := make(map[ids.ID]bool, len(current))
	for _, v := range current {
		if v.ValidationID == nil {
			continue
		}
		empty[*v.ValidationID] = v.DeactivationOwner == nil || v.DeactivationOwner.Threshold == 0
	}

	var validationIDs []ids.ID
	if strings.TrimSpace(warpValidationIDs) != "" {
		for _, raw := range parseValidatorAddrs(warpValidationIDs) {
			id, err := ids.FromString(raw)
			if err != nil {
				return nil, fmt.Errorf("invalid --validation-ids entry %q: %w", raw, err)
			}
			if _, ok := empty[id]; !ok {
				return nil, fmt.Errorf("%w: %s", errWarpTargetNotFound, id)
			}
			validationIDs = append(validationIDs, id)
		}
	} else {
		// Keep the order of the API response, so a plan is reproducible.
		for _, v := range current {
			if v.ValidationID != nil && empty[*v.ValidationID] {
				validationIDs = append(validationIDs, *v.ValidationID)
			}
		}
	}
	if len(validationIDs) == 0 {
		return nil, errWarpNoTargets
	}

	targets := make([]warp.TargetValidator, len(validationIDs))
	for i, id := range validationIDs {
		v, _, err := client.GetL1Validator(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to get L1 validator %s: %w", id, err)
		}
		targets[i] = warp.TargetValidator{
			ValidationID: id,
			NodeID:       v.NodeID,
			PublicKey:    v.PublicKey,
			Weight:       v.Weight,
		}
	}
	return targets, nil
}

func printPlanSummary(p *warp.Plan, planned []warp.PlannedTarget, now time.Time) {
	fmt.Printf("Network ID:              %d (%s)\n", p.NetworkID, constants.NetworkName(p.NetworkID))
	fmt.Printf("Subnet ID:               %s\n", p.SubnetID)
	fmt.Printf("Manager:                 0x%x on %s\n", []byte(p.ManagerAddress), p.ManagerBlockchainID)
	fmt.Printf("Re-add balance:          %d nAVAX each\n", p.Balance)
	fmt.Printf("Re-add expiry:           %s (in %s)\n", formatUnix(p.Expiry), time.Unix(int64(p.Expiry), 0).Sub(now).Round(time.Minute))
	fmt.Printf("Canonical set:           %d validators, total weight %d, P-Chain proposed height about %d\n",
		len(p.Snapshot.Validators),
		p.Snapshot.TotalWeight,
		p.SnapshotHeight,
	)
	if len(planned) > 0 {
		r := planned[0].ReaddPayload
		fmt.Printf("New remaining balance owner: %s\n", formatPChainOwner(r.RemainingBalanceOwner, p.NetworkID))
		fmt.Printf("New deactivation owner:      %s\n", formatPChainOwner(r.DisableOwner, p.NetworkID))
	}
	fmt.Printf("\nTargets, in rotation order:\n")
	for i, t := range planned {
		share := 0.0
		if p.Snapshot.TotalWeight > 0 {
			share = 100 * float64(t.Weight) / float64(p.Snapshot.TotalWeight)
		}
		fmt.Printf("  [%d] %s weight %d (%.2f%%) validation %s -> %s\n",
			i,
			t.NodeID,
			t.Weight,
			share,
			t.ValidationID,
			t.ReaddValidationID,
		)
	}
}

var warpRotateCmd = &cobra.Command{
	Use:   "rotate",
	Short: "Submit a rotation plan one validator at a time",
	Long: `Submit the removal and re-add of every plan target, in order, one target
at a time:

  1. aggregate and submit the removal (SetL1ValidatorWeightTx)
  2. wait until the target leaves the validator set
  3. aggregate and submit the re-add (RegisterL1ValidatorTx)
  4. wait until the target is back, then check its new owners

Each aggregation uses the bundle signatures of the validators in the live set
and must reach 67% of the weight. Before a removal, the command checks that
the re-add also reaches 67% without the target, that a bundle has the target's
proof of possession, and that the re-add does not expire within
--expiry-margin. It never removes a second validator while one is out of the
set. A rerun skips rotated targets and re-adds a removed one.

Rate limits (HTTP 429, Cloudflare 1015) and a proposed height lag ("failed
verifying warp messages") are retried for about 60s. A transaction is never
issued again if the P-Chain already accepted it. Other errors stop at once.

The --key-name, --ledger, or --private-key wallet pays the fees and the
re-add balances. It asks for a confirm before each target unless --yes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		if warpPlanPath == "" || len(warpBundlePaths) == 0 {
			return fmt.Errorf("%w: --plan and at least one --sigs", errWarpMissingFlag)
		}
		p, planned, err := loadPlan(warpPlanPath)
		if err != nil {
			return err
		}
		bundles := make([]*warp.Bundle, len(warpBundlePaths))
		for i, path := range warpBundlePaths {
			bundles[i] = new(warp.Bundle)
			if err := readJSONFile(path, bundles[i]); err != nil {
				return err
			}
		}
		collected, err := warp.Collect(planned, bundles)
		if err != nil {
			return fmt.Errorf("invalid signature bundles: %w", err)
		}
		for _, skipped := range collected.Skipped {
			fmt.Printf("warning: %v\n", skipped)
		}

		netConfig, err := getNetworkConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to get network config: %w", err)
		}
		if netConfig.NetworkID != p.NetworkID {
			return fmt.Errorf("%w: wallet network %d, plan %d (set --network)", errWarpNetworkMismatch, netConfig.NetworkID, p.NetworkID)
		}
		uri := netConfig.RPCURL
		if warpRPC != "" {
			uri, err = pChainURI()
			if err != nil {
				return err
			}
			rpcNetworkID, err := network.GetNetworkID(ctx, uri)
			if err != nil {
				return err
			}
			if rpcNetworkID != p.NetworkID {
				return fmt.Errorf("%w: --rpc network %d, plan %d", errWarpNetworkMismatch, rpcNetworkID, p.NetworkID)
			}
		}

		w, cleanup, err := loadPChainWallet(ctx, netConfig)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}
		defer cleanup()

		printPlanSummary(p, planned, time.Now())
		fmt.Printf("\nSignature bundles:       %d keys\n", len(collected.PoPs))
		fmt.Printf("Fee payer:               %s\n\n", w.FormattedPChainAddress())

		var confirmTarget func(warp.PlannedTarget) error
		var afterTarget func(warp.PlannedTarget, warp.TargetResult) error
		if !warpYes {
			confirmTarget = func(t warp.PlannedTarget) error {
				return confirm(fmt.Sprintf("Type 'yes' to remove and re-add %s: ", t.NodeID), false)
			}
			afterTarget = func(t warp.PlannedTarget, res warp.TargetResult) error {
				fmt.Printf("\n  %s rotated.\n", t.NodeID)
				fmt.Printf("    new validation ID:   %s\n", t.ReaddValidationID)
				fmt.Printf("    deactivation owner:  threshold %d\n", res.DeactivationOwner.Threshold)
				fmt.Printf("    Confirm this validator is Active with a threshold-%d owner on the explorer before you continue.\n", res.DeactivationOwner.Threshold)
				return pauseContinue("  Press Enter to continue to the next validator, or type 'stop' to halt: ", false)
			}
		}
		results, err := warp.Rotate(ctx, warp.RotateConfig{
			Chain: &pChain{
				client: platformvm.NewClient(uri),
				wallet: w,
			},
			SubnetID:  p.SubnetID,
			Balance:   p.Balance,
			Planned:   planned,
			Collected: collected,
			Confirm:     confirmTarget,
			AfterTarget: afterTarget,
			Log:         os.Stdout,
			Retry: warp.RetryPolicy{
				Attempts:   retryAttempts,
				Backoff:    retryBackoff,
				MaxBackoff: retryMaxBackoff,
			},
			PollInterval: warpPollInterval,
			WaitTimeout:  warpWaitTimeout,
			ExpiryMargin: warpExpiryMargin,
			Now:          time.Now,
		})

		fmt.Printf("\nSummary: %d of %d targets rotated\n", len(results), len(planned))
		for i, res := range results {
			status := "rotated"
			if res.AlreadyRotated {
				status = "already rotated"
			}
			fmt.Printf("  [%d] %s %s: removal %s, re-add %s, deactivation owner %s\n",
				i,
				res.NodeID,
				status,
				res.RemovalTxID,
				res.ReaddTxID,
				formatPChainOwner(res.DeactivationOwner, p.NetworkID),
			)
		}
		return err
	},
}

// pChain is the live P-Chain for [warp.Rotate].
type pChain struct {
	client *platformvm.Client
	wallet *wallet.Wallet
}

func (c *pChain) ValidatorSet(ctx context.Context, subnetID ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
	return c.client.GetValidatorsAt(ctx, subnetID, platformapi.ProposedHeight)
}

func (c *pChain) L1Validator(ctx context.Context, validationID ids.ID) (warp.L1Validator, bool, error) {
	v, _, err := c.client.GetL1Validator(ctx, validationID)
	// JSON-RPC drops the error chain, so match the text of
	// database.ErrNotFound, which the API wraps for a missing validator.
	if err != nil && strings.Contains(err.Error(), database.ErrNotFound.Error()) {
		return warp.L1Validator{}, false, nil
	}
	if err != nil {
		return warp.L1Validator{}, false, err
	}
	return warp.L1Validator{
		NodeID:                v.NodeID,
		Weight:                v.Weight,
		RemainingBalanceOwner: pChainOwner(v.RemainingBalanceOwner),
		DeactivationOwner:     pChainOwner(v.DeactivationOwner),
	}, true, nil
}

func pChainOwner(o *secp256k1fx.OutputOwners) message.PChainOwner {
	if o == nil {
		return message.PChainOwner{}
	}
	return message.PChainOwner{
		Threshold: o.Threshold,
		Addresses: o.Addrs,
	}
}

func (c *pChain) SetL1ValidatorWeight(ctx context.Context, msg []byte) (ids.ID, error) {
	return pchain.SetL1ValidatorWeight(ctx, c.wallet, msg)
}

func (c *pChain) RegisterL1Validator(ctx context.Context, balance uint64, pop [bls.SignatureLen]byte, msg []byte) (ids.ID, error) {
	return pchain.RegisterL1Validator(ctx, c.wallet, balance, pop, msg)
}

// loadPlan reads and decodes a plan file.
func loadPlan(path string) (*warp.Plan, []warp.PlannedTarget, error) {
	p := new(warp.Plan)
	if err := readJSONFile(path, p); err != nil {
		return nil, nil, err
	}
	planned, err := p.Decode()
	if err != nil {
		return nil, nil, fmt.Errorf("invalid plan %s: %w", path, err)
	}
	return p, planned, nil
}

func readJSONFile(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", path, err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return nil
}

// writeNewJSONFile writes v to path. It refuses to overwrite a file, so a
// signed bundle or a plan in use is never replaced by mistake.
func writeNewJSONFile(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode %s: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", path, err)
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}

func init() {
	warpCmd.AddCommand(warpPlanCmd)
	warpCmd.AddCommand(warpRotateCmd)

	f := warpPlanCmd.Flags()
	f.StringVar(&warpSubnetID, "subnet-id", "", "L1 subnet ID")
	f.StringVar(&warpRPC, "rpc", "", "Node URL for P-Chain reads, e.g. https://api.avax.network (default: the --network endpoint)")
	f.StringVar(&warpManagerBlockchainID, "manager-blockchain-id", "", "Blockchain ID of the validator manager (Warp source chain)")
	f.StringVar(&warpManagerAddress, "manager-address", "", "Validator manager contract address (0x..., Warp source address)")
	f.StringVar(&warpRemainingBalanceOwner, "remaining-balance-owner", "", "P-Chain address that receives each re-added validator's remaining balance")
	f.StringVar(&warpDeactivationOwner, "deactivation-owner", "", "P-Chain address that can disable each re-added validator")
	f.Float64Var(&warpBalance, "balance", 0, "Balance in AVAX of each re-added validator, for the continuous fee")
	f.Uint64Var(&warpExpiry, "expiry", 0, "Unix time after which the P-Chain rejects the re-adds, at most 24h ahead (default: 23h from now)")
	f.StringVar(&warpValidationIDs, "validation-ids", "", "Comma-separated validation IDs to rotate (default: every validator with an empty deactivation owner)")
	f.StringVar(&warpPlanOut, "out", "plan.json", "Plan file to create (never overwritten)")

	f = warpRotateCmd.Flags()
	f.StringVar(&warpPlanPath, "plan", "", "Plan file from warp plan")
	f.StringArrayVar(&warpBundlePaths, "sigs", nil, "Signature bundle file from warp sign --plan (repeat once per machine)")
	f.StringVar(&warpRPC, "rpc", "", "Node URL for P-Chain reads (default: the --network or --rpc-url endpoint)")
	f.BoolVar(&warpYes, "yes", false, "Rotate every target without a confirm")
	f.DurationVar(&warpWaitTimeout, "wait-timeout", defaultWaitTimeout, "Maximum wait for the validator set to update after each transaction")
	f.DurationVar(&warpPollInterval, "poll-interval", defaultPollInterval, "Interval between validator set reads while waiting")
	f.DurationVar(&warpExpiryMargin, "expiry-margin", defaultReaddExpiryLimit, "Do not start a target whose re-add expires within this time")
}
