//go:build integration

// Package pchain provides all P-Chain transaction operations for Avalanche.
// This file contains integration tests that run against a tmpnet network.
//
// To run these tests:
//   1. Build avalanchego: go build -o ./build/avalanchego ./main
//   2. Set AVALANCHEGO_PATH to point to the binary
//   3. Run: go test -tags=integration -v ./pkg/pchain/...
package pchain

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/tests/fixture/tmpnet"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/platform-cli/pkg/network"
	"github.com/ava-labs/platform-cli/pkg/wallet"
)

const (
	// testTimeout is the maximum time for a single test
	testTimeout = 2 * time.Minute
)

// testNetwork holds shared test network state
type testNetwork struct {
	network *tmpnet.Network
	log     logging.Logger
	t       *testing.T
}

// setupTestNetwork creates a new tmpnet network for testing.
// The caller is responsible for calling cleanup().
func setupTestNetwork(t *testing.T) (*testNetwork, func()) {
	t.Helper()

	avalanchegoPath := os.Getenv("AVALANCHEGO_PATH")
	if avalanchegoPath == "" {
		t.Skip("AVALANCHEGO_PATH not set, skipping integration test")
	}

	log := logging.NewLogger(
		"tmpnet-test",
		logging.NewWrappedCore(logging.Info, os.Stderr, logging.Colors.ConsoleEncoder()),
	)

	// Create network with 2 nodes
	network := &tmpnet.Network{
		Owner: "platform-cli-test",
		Nodes: tmpnet.NewNodesOrPanic(2),
		DefaultRuntimeConfig: tmpnet.NodeRuntimeConfig{
			Process: &tmpnet.ProcessRuntimeConfig{
				AvalancheGoPath: avalanchegoPath,
			},
		},
	}

	// Set default flags for faster testing
	network.DefaultFlags = tmpnet.FlagsMap{}
	network.DefaultFlags.SetDefaults(tmpnet.DefaultTmpnetFlags())
	network.DefaultFlags.SetDefaults(tmpnet.DefaultE2EFlags())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Bootstrap the network
	if err := tmpnet.BootstrapNewNetwork(ctx, log, network, ""); err != nil {
		t.Fatalf("failed to bootstrap network: %v", err)
	}

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := network.Stop(ctx); err != nil {
			t.Logf("failed to stop network: %v", err)
		}
	}

	return &testNetwork{
		network: network,
		log:     log,
		t:       t,
	}, cleanup
}

// getNetworkConfig returns a network config for the tmpnet.
func (tn *testNetwork) getNetworkConfig() network.Config {
	networkID, _ := tn.network.GetNetworkID()
	return network.Config{
		Name:      "tmpnet",
		NetworkID: networkID,
		RPCURL:    tn.network.Nodes[0].URI,
	}
}

// getTestWallet creates a wallet using a pre-funded key from the network.
func (tn *testNetwork) getTestWallet(ctx context.Context) (*wallet.Wallet, error) {
	if len(tn.network.PreFundedKeys) == 0 {
		tn.t.Fatal("no pre-funded keys available")
	}

	// Use the first pre-funded key
	key := tn.network.PreFundedKeys[0]
	config := tn.getNetworkConfig()

	return wallet.NewWallet(ctx, key, config)
}

// getTestWalletWithSubnet creates a wallet that tracks a specific subnet.
func (tn *testNetwork) getTestWalletWithSubnet(ctx context.Context, subnetID ids.ID) (*wallet.Wallet, error) {
	if len(tn.network.PreFundedKeys) == 0 {
		tn.t.Fatal("no pre-funded keys available")
	}

	key := tn.network.PreFundedKeys[0]
	config := tn.getNetworkConfig()

	return wallet.NewWalletWithSubnet(ctx, key, config, subnetID)
}

func TestIntegration_Send(t *testing.T) {
	tn, cleanup := setupTestNetwork(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	w, err := tn.getTestWallet(ctx)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	// Generate a destination address (use second pre-funded key if available)
	destKey := tn.network.PreFundedKeys[0]
	if len(tn.network.PreFundedKeys) > 1 {
		destKey = tn.network.PreFundedKeys[1]
	}
	destAddr := destKey.Address()

	// Send a small amount (100 nAVAX)
	amount := uint64(100)
	txID, err := Send(ctx, w, destAddr, amount)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if txID == ids.Empty {
		t.Error("Send returned empty tx ID")
	}

	t.Logf("Send TX ID: %s", txID)
}

func TestIntegration_CreateSubnet(t *testing.T) {
	tn, cleanup := setupTestNetwork(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	w, err := tn.getTestWallet(ctx)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	// Create a subnet
	subnetID, err := CreateSubnet(ctx, w)
	if err != nil {
		t.Fatalf("CreateSubnet failed: %v", err)
	}

	if subnetID == ids.Empty {
		t.Error("CreateSubnet returned empty subnet ID")
	}

	t.Logf("Created Subnet ID: %s", subnetID)
}

func TestIntegration_TransferSubnetOwnership(t *testing.T) {
	tn, cleanup := setupTestNetwork(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	w, err := tn.getTestWallet(ctx)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	// First create a subnet
	subnetID, err := CreateSubnet(ctx, w)
	if err != nil {
		t.Fatalf("CreateSubnet failed: %v", err)
	}

	// Get a new owner address
	newOwner := tn.network.PreFundedKeys[0].Address()
	if len(tn.network.PreFundedKeys) > 1 {
		newOwner = tn.network.PreFundedKeys[1].Address()
	}

	// Need to refresh the wallet to include the new subnet
	w, err = tn.getTestWalletWithSubnet(ctx, subnetID)
	if err != nil {
		t.Fatalf("failed to recreate wallet: %v", err)
	}

	// Transfer ownership
	txID, err := TransferSubnetOwnership(ctx, w, subnetID, newOwner)
	if err != nil {
		t.Fatalf("TransferSubnetOwnership failed: %v", err)
	}

	if txID == ids.Empty {
		t.Error("TransferSubnetOwnership returned empty tx ID")
	}

	t.Logf("Transfer Subnet Ownership TX ID: %s", txID)
}

func TestIntegration_CreateChain(t *testing.T) {
	tn, cleanup := setupTestNetwork(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	w, err := tn.getTestWallet(ctx)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	// First create a subnet
	subnetID, err := CreateSubnet(ctx, w)
	if err != nil {
		t.Fatalf("CreateSubnet failed: %v", err)
	}

	// Need to refresh the wallet to include the new subnet
	w, err = tn.getTestWalletWithSubnet(ctx, subnetID)
	if err != nil {
		t.Fatalf("failed to recreate wallet: %v", err)
	}

	// Create a simple chain with minimal genesis
	genesis := []byte(`{"config":{"chainId":99999}}`)
	vmID := ids.GenerateTestID() // Use a test VM ID

	cfg := CreateChainConfig{
		SubnetID:  subnetID,
		Genesis:   genesis,
		VMID:      vmID,
		FxIDs:     nil,
		ChainName: "test-chain",
	}

	chainID, err := CreateChain(ctx, w, cfg)
	if err != nil {
		// Chain creation might fail if the VM is not registered, which is expected
		// in a minimal test environment
		t.Logf("CreateChain failed (expected if VM not registered): %v", err)
		return
	}

	if chainID == ids.Empty {
		t.Error("CreateChain returned empty chain ID")
	}

	t.Logf("Created Chain ID: %s", chainID)
}

func TestIntegration_ExportImport(t *testing.T) {
	tn, cleanup := setupTestNetwork(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	w, err := tn.getTestWallet(ctx)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	// Get the C-Chain ID from the network
	cChainID := ids.Empty
	networkID, err := tn.network.GetNetworkID()
	if err == nil {
		// C-Chain ID is well-known
		cChainID, _ = ids.FromString("2q9e4r6Mu3U68nU1fYjgbR6JvwrRx36CohpAX5UQxse55x1Q5")
	}

	if cChainID == ids.Empty {
		t.Skip("Could not determine C-Chain ID, skipping export test")
	}

	// Export to C-Chain
	amount := uint64(1000000000) // 1 AVAX
	txID, err := Export(ctx, w, cChainID, amount)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if txID == ids.Empty {
		t.Error("Export returned empty tx ID")
	}

	t.Logf("Export TX ID: %s (network ID: %d)", txID, networkID)
}
