package network

import (
	"testing"
	"time"
)

func TestGetConfig_Fuji(t *testing.T) {
	cfg := GetConfig("fuji")

	if cfg.Name != "fuji" {
		t.Errorf("GetConfig(fuji).Name = %s, want fuji", cfg.Name)
	}
	if cfg.NetworkID != 5 {
		t.Errorf("GetConfig(fuji).NetworkID = %d, want 5", cfg.NetworkID)
	}
	if cfg.RPCURL != "https://api.avax-test.network" {
		t.Errorf("GetConfig(fuji).RPCURL = %s, want https://api.avax-test.network", cfg.RPCURL)
	}
	if cfg.MinValidatorStake != 1_000_000_000 {
		t.Errorf("GetConfig(fuji).MinValidatorStake = %d, want 1000000000", cfg.MinValidatorStake)
	}
	if cfg.MinDelegatorStake != 1_000_000_000 {
		t.Errorf("GetConfig(fuji).MinDelegatorStake = %d, want 1000000000", cfg.MinDelegatorStake)
	}
	if cfg.MinStakeDuration != 24*time.Hour {
		t.Errorf("GetConfig(fuji).MinStakeDuration = %v, want 24h", cfg.MinStakeDuration)
	}
}

func TestGetConfig_Mainnet(t *testing.T) {
	cfg := GetConfig("mainnet")

	if cfg.Name != "mainnet" {
		t.Errorf("GetConfig(mainnet).Name = %s, want mainnet", cfg.Name)
	}
	if cfg.NetworkID != 1 {
		t.Errorf("GetConfig(mainnet).NetworkID = %d, want 1", cfg.NetworkID)
	}
	if cfg.RPCURL != "https://api.avax.network" {
		t.Errorf("GetConfig(mainnet).RPCURL = %s, want https://api.avax.network", cfg.RPCURL)
	}
	if cfg.MinValidatorStake != 2000_000_000_000 {
		t.Errorf("GetConfig(mainnet).MinValidatorStake = %d, want 2000000000000", cfg.MinValidatorStake)
	}
	if cfg.MinDelegatorStake != 25_000_000_000 {
		t.Errorf("GetConfig(mainnet).MinDelegatorStake = %d, want 25000000000", cfg.MinDelegatorStake)
	}
	if cfg.MinStakeDuration != 14*24*time.Hour {
		t.Errorf("GetConfig(mainnet).MinStakeDuration = %v, want 336h", cfg.MinStakeDuration)
	}
}

func TestGetConfig_Default(t *testing.T) {
	// Unknown network should default to Fuji
	cfg := GetConfig("unknown")

	if cfg.Name != "fuji" {
		t.Errorf("GetConfig(unknown) should default to fuji, got %s", cfg.Name)
	}
}

func TestGetNetworkIDAndRPC(t *testing.T) {
	tests := []struct {
		name          string
		network       string
		wantNetworkID uint32
		wantRPC       string
	}{
		{"fuji", "fuji", 5, "https://api.avax-test.network"},
		{"mainnet", "mainnet", 1, "https://api.avax.network"},
		{"unknown", "unknown", 5, "https://api.avax-test.network"}, // defaults to fuji
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			networkID, rpc := GetNetworkIDAndRPC(tt.network)
			if networkID != tt.wantNetworkID {
				t.Errorf("GetNetworkIDAndRPC(%s) networkID = %d, want %d", tt.network, networkID, tt.wantNetworkID)
			}
			if rpc != tt.wantRPC {
				t.Errorf("GetNetworkIDAndRPC(%s) rpc = %s, want %s", tt.network, rpc, tt.wantRPC)
			}
		})
	}
}

func TestGetHRP(t *testing.T) {
	tests := []struct {
		networkID uint32
		wantHRP   string
	}{
		{1, "avax"},  // mainnet
		{5, "fuji"},  // fuji
		{1337, "custom"}, // local/custom
	}

	for _, tt := range tests {
		t.Run(tt.wantHRP, func(t *testing.T) {
			hrp := GetHRP(tt.networkID)
			if hrp != tt.wantHRP {
				t.Errorf("GetHRP(%d) = %s, want %s", tt.networkID, hrp, tt.wantHRP)
			}
		})
	}
}

func TestFujiConfig(t *testing.T) {
	// Verify Fuji config is properly initialized
	if Fuji.NetworkID != 5 {
		t.Errorf("Fuji.NetworkID = %d, want 5", Fuji.NetworkID)
	}
	if Fuji.Name != "fuji" {
		t.Errorf("Fuji.Name = %s, want fuji", Fuji.Name)
	}
}

func TestMainnetConfig(t *testing.T) {
	// Verify Mainnet config is properly initialized
	if Mainnet.NetworkID != 1 {
		t.Errorf("Mainnet.NetworkID = %d, want 1", Mainnet.NetworkID)
	}
	if Mainnet.Name != "mainnet" {
		t.Errorf("Mainnet.Name = %s, want mainnet", Mainnet.Name)
	}

	// Mainnet should have higher staking requirements
	if Mainnet.MinValidatorStake <= Fuji.MinValidatorStake {
		t.Error("Mainnet.MinValidatorStake should be greater than Fuji")
	}
	if Mainnet.MinDelegatorStake <= Fuji.MinDelegatorStake {
		t.Error("Mainnet.MinDelegatorStake should be greater than Fuji")
	}
	if Mainnet.MinStakeDuration <= Fuji.MinStakeDuration {
		t.Error("Mainnet.MinStakeDuration should be greater than Fuji")
	}
}
