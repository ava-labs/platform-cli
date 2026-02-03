// Package network provides Avalanche network configuration utilities.
package network

import "time"

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
