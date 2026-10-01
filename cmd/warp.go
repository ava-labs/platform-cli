package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/crypto/bls"
	"github.com/ava-labs/avalanchego/utils/crypto/bls/signer/localsigner"
	"github.com/ava-labs/avalanchego/vms/platformvm"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs/executor"
	"github.com/ava-labs/avalanchego/vms/platformvm/warp/message"
	"github.com/ava-labs/libevm/common"
	"github.com/ava-labs/platform-cli/pkg/network"
	nodeutil "github.com/ava-labs/platform-cli/pkg/node"
	"github.com/ava-labs/platform-cli/pkg/warp"
	"github.com/spf13/cobra"

	platformapi "github.com/ava-labs/avalanchego/vms/platformvm/api"
)

const (
	warpTypeWeight   = "weight"
	warpTypeRegister = "register"

	// blsSecretKeyLen is the size of the raw secret key avalanchego writes to
	// staking/signer.key.
	blsSecretKeyLen = 32

	registerExpiryWindow = executor.RegisterL1ValidatorTxExpiryWindow * time.Second
)

var (
	errWarpMissingFlag  = errors.New("missing required flag")
	errWarpUnknownType  = errors.New("unknown message type")
	errWarpExpiryPassed = errors.New("expiry has passed")
	errWarpExpiryTooFar = errors.New("expiry is beyond the P-Chain expiry window")
	errWarpNotConfirmed = errors.New("not confirmed")
	errWarpBadKeyFile   = errors.New("invalid BLS key file")
)

var (
	warpType                  string
	warpValidationID          string
	warpWeight                uint64
	warpNonce                 uint64
	warpNodeID                string
	warpBLSPublicKey          string
	warpRemainingBalanceOwner string
	warpDeactivationOwner     string
	warpExpiry                uint64
	warpManagerBlockchainID   string
	warpManagerAddress        string
	warpSubnetID              string
	warpMessage               string
	warpBLSKey                string
	warpYes                   bool
	warpRPC                   string
	warpSigs                  []string
)

var warpCmd = &cobra.Command{
	Use:   "warp",
	Short: "Offline BLS signing of P-Chain Warp messages",
	Long: `Build, sign, and aggregate P-Chain Warp messages with validator BLS keys directly.

Use this when the validator manager cannot emit the message, for example to
rotate L1 validator owners. The flow is:

  1. warp build-message   (coordinator) builds the unsigned message
  2. warp sign            (each validator machine) signs it with staking/signer.key
  3. warp aggregate       (coordinator) checks the 67% quorum and builds the signed message
  4. l1 set-validator-weight or l1 register-validator submits it`,
	RunE: requireSubcommand,
}

var warpBuildMessageCmd = &cobra.Command{
	Use:   "build-message",
	Short: "Build an unsigned RegisterL1Validator or L1ValidatorWeight Warp message",
	Long: `Build the canonical unsigned Warp message that every signer signs.

The message is an AddressedCall from --manager-address on --manager-blockchain-id.
This command is offline. It takes the network ID from --network or --network-id.

  --type weight     L1ValidatorWeight: --validation-id, --weight (0 removes), --nonce
  --type register   RegisterL1Validator: --subnet-id, --node-id, --bls-public-key,
                    --weight, --expiry, --remaining-balance-owner, --deactivation-owner

The validator balance is not part of the message. Set it on
"l1 register-validator --balance".`,
	RunE: func(cmd *cobra.Command, args []string) error {
		networkID, err := offlineNetworkID()
		if err != nil {
			return err
		}
		if warpManagerBlockchainID == "" || warpManagerAddress == "" {
			return fmt.Errorf("%w: --manager-blockchain-id and --manager-address", errWarpMissingFlag)
		}
		managerChainID, err := ids.FromString(warpManagerBlockchainID)
		if err != nil {
			return fmt.Errorf("invalid --manager-blockchain-id: %w", err)
		}
		managerAddress, err := decodeHexExactLength(warpManagerAddress, common.AddressLength)
		if err != nil {
			return fmt.Errorf("invalid --manager-address: %w", err)
		}

		var p message.Payload
		switch warpType {
		case warpTypeWeight:
			p, err = buildWeightPayload()
		case warpTypeRegister:
			p, err = buildRegisterPayload(networkID, time.Now())
		default:
			return fmt.Errorf("%w %q (want %q or %q)", errWarpUnknownType, warpType, warpTypeWeight, warpTypeRegister)
		}
		if err != nil {
			return err
		}

		msg, err := warp.NewUnsignedMessage(networkID, managerChainID, managerAddress, p)
		if err != nil {
			return fmt.Errorf("failed to build message: %w", err)
		}
		decoded, err := warp.Decode(msg.Bytes())
		if err != nil {
			return fmt.Errorf("failed to decode built message: %w", err)
		}

		fmt.Print(describeWarpMessage(decoded, time.Now()))
		fmt.Println()
		fmt.Printf("Unsigned message: 0x%x\n", msg.Bytes())
		return nil
	},
}

func buildWeightPayload() (*message.L1ValidatorWeight, error) {
	if warpValidationID == "" {
		return nil, fmt.Errorf("%w: --validation-id", errWarpMissingFlag)
	}
	validationID, err := ids.FromString(warpValidationID)
	if err != nil {
		return nil, fmt.Errorf("invalid --validation-id: %w", err)
	}
	return message.NewL1ValidatorWeight(validationID, warpNonce, warpWeight)
}

func buildRegisterPayload(networkID uint32, now time.Time) (*message.RegisterL1Validator, error) {
	if warpSubnetID == "" || warpNodeID == "" || warpBLSPublicKey == "" || warpExpiry == 0 {
		return nil, fmt.Errorf("%w: --subnet-id, --node-id, --bls-public-key, and --expiry", errWarpMissingFlag)
	}
	if warpRemainingBalanceOwner == "" || warpDeactivationOwner == "" {
		return nil, fmt.Errorf("%w: --remaining-balance-owner and --deactivation-owner", errWarpMissingFlag)
	}
	subnetID, err := ids.FromString(warpSubnetID)
	if err != nil {
		return nil, fmt.Errorf("invalid --subnet-id: %w", err)
	}
	nodeID, err := ids.NodeIDFromString(warpNodeID)
	if err != nil {
		return nil, fmt.Errorf("invalid --node-id: %w", err)
	}
	pkBytes, err := decodeHexExactLength(warpBLSPublicKey, bls.PublicKeyLen)
	if err != nil {
		return nil, fmt.Errorf("invalid --bls-public-key: %w", err)
	}
	if _, err := bls.PublicKeyFromCompressedBytes(pkBytes); err != nil {
		return nil, fmt.Errorf("invalid --bls-public-key: %w", err)
	}
	if err := verifyRegisterExpiry(warpExpiry, now); err != nil {
		return nil, err
	}
	hrp := constants.GetHRP(networkID)
	remainingBalanceOwner, err := parsePChainAddress(warpRemainingBalanceOwner, hrp)
	if err != nil {
		return nil, fmt.Errorf("invalid --remaining-balance-owner: %w", err)
	}
	deactivationOwner, err := parsePChainAddress(warpDeactivationOwner, hrp)
	if err != nil {
		return nil, fmt.Errorf("invalid --deactivation-owner: %w", err)
	}

	var pk [bls.PublicKeyLen]byte
	copy(pk[:], pkBytes)
	return message.NewRegisterL1Validator(
		subnetID,
		nodeID,
		pk,
		warpExpiry,
		validatorOwner([]ids.ShortID{remainingBalanceOwner}, 0, ids.ShortEmpty),
		validatorOwner([]ids.ShortID{deactivationOwner}, 0, ids.ShortEmpty),
		warpWeight,
	)
}

// verifyRegisterExpiry checks expiry against the local clock. The P-Chain
// rejects a RegisterL1Validator message whose expiry is not after its chain
// time, or more than [registerExpiryWindow] after it.
func verifyRegisterExpiry(expiry uint64, now time.Time) error {
	if expiry <= uint64(now.Unix()) {
		return fmt.Errorf("%w: %s", errWarpExpiryPassed, formatUnix(expiry))
	}
	if maxExpiry := uint64(now.Add(registerExpiryWindow).Unix()); expiry > maxExpiry {
		return fmt.Errorf("%w: %s is after %s", errWarpExpiryTooFar, formatUnix(expiry), formatUnix(maxExpiry))
	}
	return nil
}

var warpSignCmd = &cobra.Command{
	Use:   "sign",
	Short: "Sign an unsigned Warp message with a validator BLS key",
	Long: `Sign an unsigned Warp message with the BLS key of a validator.

Run this on the validator machine. --bls-key is the avalanchego
staking/signer.key file (the 32-byte raw secret key). The key never leaves
this process: it is never printed or written.

Before it signs, the command prints every field of the message and the signer
public key, and asks you to type 'yes'. A BLS signature over these bytes
authorizes the change on the P-Chain, so read the fields first. --yes skips
the prompt.

Output: one "<public key hex>:<signature hex>" blob for "warp aggregate --sig".`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if warpBLSKey == "" || warpMessage == "" {
			return fmt.Errorf("%w: --bls-key and --message", errWarpMissingFlag)
		}
		msgBytes, err := decodeHex(warpMessage)
		if err != nil {
			return fmt.Errorf("invalid --message: %w", err)
		}
		decoded, err := warp.Decode(msgBytes)
		if err != nil {
			return fmt.Errorf("refusing to sign: %w", err)
		}
		now := time.Now()
		if err := verifyDecodedExpiry(decoded, now); err != nil {
			return fmt.Errorf("refusing to sign: %w", err)
		}

		signer, err := loadBLSKey(warpBLSKey)
		if err != nil {
			return err
		}

		fmt.Print(describeWarpMessage(decoded, now))
		fmt.Printf("Signer BLS public key: 0x%x\n\n", bls.PublicKeyToCompressedBytes(signer.PublicKey()))
		if err := confirm("Type 'yes' to sign this message: ", warpYes); err != nil {
			return err
		}

		sig, err := warp.Sign(signer, decoded.Message)
		if err != nil {
			return err
		}
		fmt.Printf("Signature: %s\n", sig)
		return nil
	},
}

// loadBLSKey reads an avalanchego signer.key file. The file bytes are cleared
// after the key is parsed.
func loadBLSKey(path string) (*localsigner.LocalSigner, error) {
	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read BLS key: %w", err)
	}
	defer clearBytes(keyBytes)

	if len(keyBytes) != blsSecretKeyLen {
		return nil, fmt.Errorf("%w %s: want the %d-byte raw key from staking/signer.key, got %d bytes",
			errWarpBadKeyFile,
			path,
			blsSecretKeyLen,
			len(keyBytes),
		)
	}
	signer, err := localsigner.FromBytes(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("%w %s: %w", errWarpBadKeyFile, path, err)
	}
	return signer, nil
}

var warpAggregateCmd = &cobra.Command{
	Use:   "aggregate",
	Short: "Aggregate BLS signatures into a signed Warp message",
	Long: `Aggregate the "warp sign" blobs into a signed Warp message.

The command reads the canonical validator set of --subnet-id from the P-Chain
at its proposed height, the height the P-Chain uses to verify new transactions.
It places each signer at its canonical index and refuses to output a message
unless the signers hold at least 67% of the subnet weight.

Submit the output with "l1 set-validator-weight --message" or
"l1 register-validator --message".`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

		if warpMessage == "" || warpSubnetID == "" || len(warpSigs) == 0 {
			return fmt.Errorf("%w: --message, --subnet-id, and at least one --sig", errWarpMissingFlag)
		}
		msgBytes, err := decodeHex(warpMessage)
		if err != nil {
			return fmt.Errorf("invalid --message: %w", err)
		}
		decoded, err := warp.Decode(msgBytes)
		if err != nil {
			return err
		}
		subnetID, err := ids.FromString(warpSubnetID)
		if err != nil {
			return fmt.Errorf("invalid --subnet-id: %w", err)
		}
		sigs := make([]warp.Signature, len(warpSigs))
		for i, blob := range warpSigs {
			sigs[i], err = warp.ParseSignature(blob)
			if err != nil {
				return fmt.Errorf("invalid --sig %d: %w", i+1, err)
			}
		}

		client, err := pChainClient()
		if err != nil {
			return err
		}
		height, vdrs, err := proposedValidatorSet(ctx, client, subnetID)
		if err != nil {
			return err
		}

		fmt.Print(describeWarpMessage(decoded, time.Now()))
		fmt.Println()
		fmt.Printf("Canonical set: %d validators, total weight %d, P-Chain proposed height about %d\n",
			len(vdrs.Validators),
			vdrs.TotalWeight,
			height,
		)

		agg, err := warp.Aggregate(decoded.Message, vdrs, sigs)
		if err != nil {
			return fmt.Errorf("refusing to output a signed message: %w", err)
		}
		printAggregation(agg, vdrs)
		fmt.Printf("\nSigned message: 0x%x\n", agg.Message.Bytes())
		return nil
	},
}

func printAggregation(agg *warp.Aggregation, vdrs validators.WarpSet) {
	for _, i := range agg.Signers {
		vdr := vdrs.Validators[i]
		fmt.Printf("  signer [%d] %v weight %d\n", i, vdr.NodeIDs, vdr.Weight)
	}
	fmt.Printf("Signed weight: %d of %d (%.2f%%), quorum %d%%: reached\n",
		agg.SignedWeight,
		agg.TotalWeight,
		100*float64(agg.SignedWeight)/float64(agg.TotalWeight),
		warp.QuorumNumerator,
	)
}

// pChainClient returns a P-Chain client for --rpc, or for the --network
// endpoint if --rpc is unset. --rpc accepts a node URL with or without the
// /ext/bc/P suffix.
func pChainClient() (*platformvm.Client, error) {
	uri := warpRPC
	if uri == "" {
		config, err := network.GetConfig(networkName)
		if err != nil {
			return nil, err
		}
		uri = config.RPCURL
	}
	uri = strings.TrimRight(uri, "/")
	uri = strings.TrimSuffix(uri, "/ext/bc/P")
	uri = strings.TrimSuffix(uri, "/ext/P")
	uri, err := nodeutil.NormalizeNodeURIWithInsecureHTTP(uri, allowInsecureHTTP)
	if err != nil {
		return nil, fmt.Errorf("invalid --rpc: %w", err)
	}
	return platformvm.NewClient(uri), nil
}

// proposedValidatorSet returns the canonical validator set of subnetID at the
// P-Chain proposed height. The P-Chain verifies the Warp messages of a new
// transaction against this height. The returned height is for display: the
// set is read with the "proposed" height parameter, because public API nodes
// reject numeric heights.
func proposedValidatorSet(
	ctx context.Context,
	client *platformvm.Client,
	subnetID ids.ID,
) (uint64, validators.WarpSet, error) {
	height, err := client.GetProposedHeight(ctx)
	if err != nil {
		return 0, validators.WarpSet{}, fmt.Errorf("failed to get P-Chain proposed height: %w", err)
	}
	vdrSet, err := client.GetValidatorsAt(ctx, subnetID, platformapi.ProposedHeight)
	if err != nil {
		return 0, validators.WarpSet{}, fmt.Errorf("failed to get validator set: %w", err)
	}
	vdrs, err := validators.FlattenValidatorSet(vdrSet)
	if err != nil {
		return 0, validators.WarpSet{}, fmt.Errorf("failed to get canonical validator set: %w", err)
	}
	return height, vdrs, nil
}

// offlineNetworkID returns --network-id, or the ID of --network.
func offlineNetworkID() (uint32, error) {
	if customNetID != 0 {
		return customNetID, nil
	}
	config, err := network.GetConfig(networkName)
	if err != nil {
		return 0, fmt.Errorf("%w (use --network-id for custom networks)", err)
	}
	return config.NetworkID, nil
}

func verifyDecodedExpiry(d *warp.Decoded, now time.Time) error {
	register, ok := d.Payload.(*message.RegisterL1Validator)
	if !ok || register.Expiry > uint64(now.Unix()) {
		return nil
	}
	return fmt.Errorf("%w: %s", errWarpExpiryPassed, formatUnix(register.Expiry))
}

// describeWarpMessage returns every field of d, one per line.
func describeWarpMessage(d *warp.Decoded, now time.Time) string {
	var b strings.Builder
	networkID := d.Message.NetworkID
	fmt.Fprintf(&b, "Network ID:              %d (%s)\n", networkID, constants.NetworkName(networkID))
	fmt.Fprintf(&b, "Source blockchain ID:    %s\n", d.Message.SourceChainID)
	fmt.Fprintf(&b, "Source address:          0x%x\n", d.SourceAddress)
	fmt.Fprintf(&b, "Message ID:              %s\n", d.Message.ID())

	switch p := d.Payload.(type) {
	case *message.L1ValidatorWeight:
		fmt.Fprintf(&b, "Type:                    L1ValidatorWeight\n")
		fmt.Fprintf(&b, "Validation ID:           %s\n", p.ValidationID)
		nonce := fmt.Sprintf("%d", p.Nonce)
		if p.Nonce == math.MaxUint64 {
			nonce += " (MaxUint64, final removal)"
		}
		fmt.Fprintf(&b, "Nonce:                   %s\n", nonce)
		weight := fmt.Sprintf("%d", p.Weight)
		if p.Weight == 0 {
			weight += " (REMOVES the validator)"
		}
		fmt.Fprintf(&b, "Weight:                  %s\n", weight)
	case *message.RegisterL1Validator:
		nodeID, _ := ids.ToNodeID(p.NodeID) // Decode verified the node ID.
		fmt.Fprintf(&b, "Type:                    RegisterL1Validator\n")
		fmt.Fprintf(&b, "Validation ID:           %s\n", p.ValidationID())
		fmt.Fprintf(&b, "Subnet ID:               %s\n", p.SubnetID)
		fmt.Fprintf(&b, "Node ID:                 %s\n", nodeID)
		fmt.Fprintf(&b, "BLS public key:          0x%x\n", p.BLSPublicKey)
		fmt.Fprintf(&b, "Weight:                  %d\n", p.Weight)
		fmt.Fprintf(&b, "Expiry:                  %s (in %s)\n", formatUnix(p.Expiry), time.Unix(int64(p.Expiry), 0).Sub(now).Round(time.Minute))
		fmt.Fprintf(&b, "Remaining balance owner: %s\n", formatPChainOwner(p.RemainingBalanceOwner, networkID))
		fmt.Fprintf(&b, "Deactivation owner:      %s\n", formatPChainOwner(p.DisableOwner, networkID))
	}
	return b.String()
}

func formatUnix(t uint64) string {
	return fmt.Sprintf("%d (%s)", t, time.Unix(int64(t), 0).UTC().Format(time.RFC3339))
}

// confirm asks the operator to type "yes" unless skip is set.
func confirm(prompt string, skip bool) error {
	if skip {
		return nil
	}
	fmt.Print(prompt)
	response, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read confirmation: %w", err)
	}
	if strings.TrimSpace(strings.ToLower(response)) != "yes" {
		return errWarpNotConfirmed
	}
	return nil
}

func init() {
	rootCmd.AddCommand(warpCmd)
	warpCmd.AddCommand(warpBuildMessageCmd)
	warpCmd.AddCommand(warpSignCmd)
	warpCmd.AddCommand(warpAggregateCmd)

	f := warpBuildMessageCmd.Flags()
	f.StringVar(&warpType, "type", "", "Message type: weight or register")
	f.StringVar(&warpManagerBlockchainID, "manager-blockchain-id", "", "Blockchain ID of the validator manager (Warp source chain)")
	f.StringVar(&warpManagerAddress, "manager-address", "", "Validator manager contract address (0x..., Warp source address)")
	f.StringVar(&warpSubnetID, "subnet-id", "", "L1 subnet ID (register)")
	f.StringVar(&warpValidationID, "validation-id", "", "Validation ID to change (weight)")
	f.Uint64Var(&warpNonce, "nonce", 0, "Message nonce, at least the current minNonce from platform.getL1Validator (weight)")
	f.Uint64Var(&warpWeight, "weight", 0, "Validator weight; 0 removes the validator (weight, register)")
	f.StringVar(&warpNodeID, "node-id", "", "Validator node ID (register)")
	f.StringVar(&warpBLSPublicKey, "bls-public-key", "", "Validator BLS public key, 48 bytes hex (register)")
	f.StringVar(&warpRemainingBalanceOwner, "remaining-balance-owner", "", "P-Chain address that receives the remaining balance (register)")
	f.StringVar(&warpDeactivationOwner, "deactivation-owner", "", "P-Chain address that can disable the validator (register)")
	f.Uint64Var(&warpExpiry, "expiry", 0, "Unix time after which the P-Chain rejects the message, at most 24h ahead (register)")
	_ = warpBuildMessageCmd.MarkFlagRequired("type")

	f = warpSignCmd.Flags()
	f.StringVar(&warpBLSKey, "bls-key", "", "Path to the avalanchego staking/signer.key file")
	f.StringVar(&warpMessage, "message", "", "Unsigned Warp message (hex) from warp build-message")
	f.BoolVar(&warpYes, "yes", false, "Sign without the interactive confirmation")

	f = warpAggregateCmd.Flags()
	f.StringVar(&warpMessage, "message", "", "Unsigned Warp message (hex) from warp build-message")
	f.StringVar(&warpSubnetID, "subnet-id", "", "L1 subnet ID whose validators signed the message")
	f.StringArrayVar(&warpSigs, "sig", nil, "Signature blob from warp sign (repeat once per signer)")
	f.StringVar(&warpRPC, "rpc", "", "Node URL for P-Chain reads, e.g. https://api.avax.network (default: the --network endpoint)")
}
