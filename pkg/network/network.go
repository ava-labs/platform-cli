// Package network provides Avalanche network configuration utilities.
package network

import (
	"context"
	"fmt"
	"time"

	"github.com/ava-labs/avalanchego/api/info"
	"github.com/ava-labs/avalanchego/utils/constants"
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
	MinValidatorStake: 1_000_000_000,      // 1 AVAX
	MinDelegatorStake: 1_000_000_000,      // 1 AVAX
	MinStakeDuration:  24 * time.Hour,     // 24 hours
}

// Mainnet configuration
var Mainnet = Config{
	Name:              "mainnet",
	NetworkID:         1,
	RPCURL:            "https://api.avax.network",
	MinValidatorStake: 2000_000_000_000,   // 2000 AVAX
	MinDelegatorStake: 25_000_000_000,     // 25 AVAX
	MinStakeDuration:  14 * 24 * time.Hour, // 14 days
}

// Local network configuration (avalanche-network-runner default)
var Local = Config{
	Name:              "local",
	NetworkID:         1337,
	RPCURL:            "http://127.0.0.1:9650",
	MinValidatorStake: 1_000_000_000,      // 1 AVAX
	MinDelegatorStake: 1_000_000_000,      // 1 AVAX
	MinStakeDuration:  24 * time.Hour,     // 24 hours
}

// GetConfig returns the network configuration for the given network name.
func GetConfig(name string) Config {
	switch name {
	case "mainnet":
		return Mainnet
	case "fuji":
		return Fuji
	case "local":
		return Local
	default:
		// Default to Fuji
		return Fuji
	}
}

// GetNetworkIDAndRPC is a convenience function that returns both networkID and RPC URL.
func GetNetworkIDAndRPC(name string) (uint32, string) {
	config := GetConfig(name)
	return config.NetworkID, config.RPCURL
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
func NewCustomConfig(ctx context.Context, rpcURL string, networkID uint32) (Config, error) {
	var err error
	if networkID == 0 {
		networkID, err = GetNetworkID(ctx, rpcURL)
		if err != nil {
			return Config{}, err
		}
	}

	// Determine reasonable defaults based on network ID
	var minValidatorStake, minDelegatorStake uint64
	var minStakeDuration time.Duration

	switch networkID {
	case constants.MainnetID:
		// Mainnet parameters
		minValidatorStake = 2000_000_000_000  // 2000 AVAX
		minDelegatorStake = 25_000_000_000    // 25 AVAX
		minStakeDuration = 14 * 24 * time.Hour // 14 days
	case constants.FujiID:
		// Fuji parameters
		minValidatorStake = 1_000_000_000     // 1 AVAX
		minDelegatorStake = 1_000_000_000     // 1 AVAX
		minStakeDuration = 24 * time.Hour     // 24 hours
	default:
		// Default devnet/local parameters (permissive)
		minValidatorStake = 1_000_000_000     // 1 AVAX
		minDelegatorStake = 1_000_000_000     // 1 AVAX
		minStakeDuration = 24 * time.Hour     // 24 hours
	}

	hrp := GetHRP(networkID)

	return Config{
		Name:              fmt.Sprintf("custom-%s", hrp),
		NetworkID:         networkID,
		RPCURL:            rpcURL,
		MinValidatorStake: minValidatorStake,
		MinDelegatorStake: minDelegatorStake,
		MinStakeDuration:  minStakeDuration,
	}, nil
}
