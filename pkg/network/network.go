// Package network provides Avalanche network configuration utilities.
package network

import (
	"context"
	"fmt"
	"time"

	"github.com/ava-labs/avalanchego/api/info"
	"github.com/ava-labs/avalanchego/utils/constants"
	nodeutil "github.com/ava-labs/platform-cli/pkg/node"
)

// Config holds network-specific configuration.
type Config struct {
	Name      string
	NetworkID uint32
	RPCURL    string

	// Staking parameters
	MinValidatorStake uint64        // Minimum stake to become a validator (in nAVAX)
	MinDelegatorStake uint64        // Minimum stake to delegate (in nAVAX)
	MinStakeDuration  time.Duration // Minimum staking duration
}

// Fuji testnet configuration
var Fuji = Config{
	Name:              "fuji",
	NetworkID:         5,
	RPCURL:            "https://api.avax-test.network",
	MinValidatorStake: 1_000_000_000,  // 1 AVAX
	MinDelegatorStake: 1_000_000_000,  // 1 AVAX
	MinStakeDuration:  24 * time.Hour, // 24 hours
}

// Mainnet configuration
var Mainnet = Config{
	Name:              "mainnet",
	NetworkID:         1,
	RPCURL:            "https://api.avax.network",
	MinValidatorStake: 2000_000_000_000,    // 2000 AVAX
	MinDelegatorStake: 25_000_000_000,      // 25 AVAX
	MinStakeDuration:  14 * 24 * time.Hour, // 14 days
}

// GetConfig returns the network configuration for the given network name.
// For local/custom networks, use --rpc-url instead.
func GetConfig(name string) (Config, error) {
	switch name {
	case "mainnet":
		return Mainnet, nil
	case "fuji":
		return Fuji, nil
	default:
		return Config{}, fmt.Errorf("unsupported network %q (supported: fuji, mainnet)", name)
	}
}

// GetNetworkIDAndRPC is a convenience function that returns both networkID and RPC URL.
func GetNetworkIDAndRPC(name string) (uint32, string, error) {
	config, err := GetConfig(name)
	if err != nil {
		return 0, "", err
	}
	return config.NetworkID, config.RPCURL, nil
}

// GetNetworkID queries the network ID from an RPC endpoint.
func GetNetworkID(ctx context.Context, rpcURL string) (uint32, error) {
	client := info.NewClient(rpcURL)
	networkID, err := client.GetNetworkID(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get network ID from %s: %w", rpcURL, err)
	}
	return networkID, nil
}

// GetHRP returns the Human-Readable Part (HRP) for bech32 addresses based on network ID.
func GetHRP(networkID uint32) string {
	return constants.GetHRP(networkID)
}

// NewCustomConfig creates a config for a custom network (devnet).
// If networkID is 0, it will be queried from the node.
// If the node doesn't expose /ext/info, use --network-id flag.
func NewCustomConfig(ctx context.Context, rpcURL string, networkID uint32) (Config, error) {
	return NewCustomConfigWithInsecureHTTP(ctx, rpcURL, networkID, false)
}

// NewCustomConfigWithInsecureHTTP creates a config for a custom network (devnet)
// and validates/normalizes the RPC URL using the same URI policy used for node endpoints.
// If networkID is 0, it will be queried from the node.
// If the node doesn't expose /ext/info, use --network-id flag.
func NewCustomConfigWithInsecureHTTP(ctx context.Context, rpcURL string, networkID uint32, allowInsecureHTTP bool) (Config, error) {
	normalizedRPCURL, err := nodeutil.NormalizeNodeURIWithInsecureHTTP(rpcURL, allowInsecureHTTP)
	if err != nil {
		return Config{}, fmt.Errorf("invalid --rpc-url: %w", err)
	}

	if networkID == 0 {
		networkID, err = GetNetworkID(ctx, normalizedRPCURL)
		if err != nil {
			return Config{}, fmt.Errorf("%w\n\nUse --network-id to specify the network ID manually:\n  --network-id 1     (mainnet)\n  --network-id 5     (fuji)\n  --network-id 12345 (custom)", err)
		}
	}

	// Determine reasonable defaults based on network ID
	var minValidatorStake, minDelegatorStake uint64
	var minStakeDuration time.Duration

	switch networkID {
	case constants.MainnetID:
		// Mainnet parameters
		minValidatorStake = 2000_000_000_000   // 2000 AVAX
		minDelegatorStake = 25_000_000_000     // 25 AVAX
		minStakeDuration = 14 * 24 * time.Hour // 14 days
	case constants.FujiID:
		// Fuji parameters
		minValidatorStake = 1_000_000_000 // 1 AVAX
		minDelegatorStake = 1_000_000_000 // 1 AVAX
		minStakeDuration = 24 * time.Hour // 24 hours
	default:
		// Default devnet/local parameters (permissive)
		minValidatorStake = 1_000_000_000 // 1 AVAX
		minDelegatorStake = 1_000_000_000 // 1 AVAX
		minStakeDuration = 24 * time.Hour // 24 hours
	}

	hrp := GetHRP(networkID)

	return Config{
		Name:              fmt.Sprintf("custom-%s", hrp),
		NetworkID:         networkID,
		RPCURL:            normalizedRPCURL,
		MinValidatorStake: minValidatorStake,
		MinDelegatorStake: minDelegatorStake,
		MinStakeDuration:  minStakeDuration,
	}, nil
}
