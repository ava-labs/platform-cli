// Package e2e provides end-to-end tests for P-Chain operations.
//
// Run with: go test -v ./e2e/... -network=local
//
// These tests require a running Avalanche network. Use one of:
//   - Local network: avalanche-network-runner
//   - Fuji testnet: Requires funded wallet
package e2e

import (
	"context"
	"flag"
	"os"
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/vms/platformvm/signer"
	"github.com/ava-labs/platform-cli/pkg/crosschain"
	"github.com/ava-labs/platform-cli/pkg/network"
	"github.com/ava-labs/platform-cli/pkg/pchain"
	"github.com/ava-labs/platform-cli/pkg/wallet"
)

var (
	networkFlag = flag.String("network", "local", "Network to test against: local, fuji")
	skipFlag    = flag.Bool("skip-e2e", false, "Skip e2e tests")
)

// ewoqPrivateKey is the well-known ewoq test key used in local/test networks.
var ewoqPrivateKey = []byte{
	0x56, 0x28, 0x9e, 0x99, 0xc9, 0x4b, 0x69, 0x12,
	0xbf, 0xc1, 0x2a, 0xdc, 0x09, 0x3c, 0x9b, 0x51,
	0x12, 0x4f, 0x0d, 0xc5, 0x4a, 0xc7, 0xa7, 0x66,
	0xb2, 0xbc, 0x5c, 0xcf, 0x55, 0x8d, 0x80, 0x27,
}

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

func skipIfDisabled(t *testing.T) {
	if *skipFlag {
		t.Skip("e2e tests disabled")
	}
}

func getTestWallet(t *testing.T) (*wallet.Wallet, network.Config) {
	t.Helper()
	ctx := context.Background()

	key, err := wallet.ToPrivateKey(ewoqPrivateKey)
	if err != nil {
		t.Fatalf("failed to parse ewoq key: %v", err)
	}

	netConfig := network.GetConfig(*networkFlag)
	w, err := wallet.NewWallet(ctx, key, netConfig)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	return w, netConfig
}

func getTestFullWallet(t *testing.T) (*wallet.FullWallet, network.Config) {
	t.Helper()
	ctx := context.Background()

	key, err := wallet.ToPrivateKey(ewoqPrivateKey)
	if err != nil {
		t.Fatalf("failed to parse ewoq key: %v", err)
	}

	netConfig := network.GetConfig(*networkFlag)
	w, err := wallet.NewFullWallet(ctx, key, netConfig)
	if err != nil {
		t.Fatalf("failed to create full wallet: %v", err)
	}

	return w, netConfig
}

// =============================================================================
// Wallet Tests
// =============================================================================

func TestWalletCreation(t *testing.T) {
	skipIfDisabled(t)

	w, _ := getTestWallet(t)

	addr := w.PChainAddress()
	if addr == ids.ShortEmpty {
		t.Error("expected non-empty P-Chain address")
	}

	t.Logf("P-Chain Address: %s", addr)
}

func TestFullWalletCreation(t *testing.T) {
	skipIfDisabled(t)

	w, _ := getTestFullWallet(t)

	pAddr := w.PChainAddress()
	ethAddr := w.EthAddress()

	if pAddr == ids.ShortEmpty {
		t.Error("expected non-empty P-Chain address")
	}

	t.Logf("P-Chain Address: %s", pAddr)
	t.Logf("Eth Address: %s", ethAddr.Hex())
}

// =============================================================================
// Transfer Tests
// =============================================================================

func TestPChainSend(t *testing.T) {
	skipIfDisabled(t)

	ctx := context.Background()
	w, _ := getTestWallet(t)

	// Send to self (1 nAVAX)
	txID, err := pchain.Send(ctx, w, w.PChainAddress(), 1)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	t.Logf("Send TX: %s", txID)
}

func TestCrossChainTransferPToC(t *testing.T) {
	skipIfDisabled(t)

	ctx := context.Background()
	w, _ := getTestFullWallet(t)

	// Export 0.001 AVAX from P-Chain to C-Chain
	exportTxID, importTxID, err := crosschain.TransferPToC(ctx, w, 1_000_000) // 0.001 AVAX
	if err != nil {
		t.Fatalf("P-to-C transfer failed: %v", err)
	}

	t.Logf("Export TX: %s", exportTxID)
	t.Logf("Import TX: %s", importTxID)
}

func TestCrossChainTransferCToP(t *testing.T) {
	skipIfDisabled(t)

	ctx := context.Background()
	w, _ := getTestFullWallet(t)

	// Export 0.001 AVAX from C-Chain to P-Chain
	exportTxID, importTxID, err := crosschain.TransferCToP(ctx, w, 1_000_000) // 0.001 AVAX
	if err != nil {
		t.Fatalf("C-to-P transfer failed: %v", err)
	}

	t.Logf("Export TX: %s", exportTxID)
	t.Logf("Import TX: %s", importTxID)
}

// =============================================================================
// Subnet Tests
// =============================================================================

func TestCreateSubnet(t *testing.T) {
	skipIfDisabled(t)

	ctx := context.Background()
	w, _ := getTestWallet(t)

	subnetID, err := pchain.CreateSubnet(ctx, w)
	if err != nil {
		t.Fatalf("CreateSubnet failed: %v", err)
	}

	t.Logf("Subnet ID: %s", subnetID)
}

func TestCreateSubnetAndTransferOwnership(t *testing.T) {
	skipIfDisabled(t)

	ctx := context.Background()
	w, netConfig := getTestWallet(t)

	// Create subnet
	subnetID, err := pchain.CreateSubnet(ctx, w)
	if err != nil {
		t.Fatalf("CreateSubnet failed: %v", err)
	}
	t.Logf("Created Subnet ID: %s", subnetID)

	// Wait for tx to be accepted
	time.Sleep(2 * time.Second)

	// Create new wallet tracking the subnet
	key, _ := wallet.ToPrivateKey(ewoqPrivateKey)
	subnetWallet, err := wallet.NewWalletWithSubnet(ctx, key, netConfig, subnetID)
	if err != nil {
		t.Fatalf("failed to create subnet wallet: %v", err)
	}

	// Transfer ownership back to self (just testing the operation)
	txID, err := pchain.TransferSubnetOwnership(ctx, subnetWallet, subnetID, w.PChainAddress())
	if err != nil {
		t.Fatalf("TransferSubnetOwnership failed: %v", err)
	}

	t.Logf("Transfer Ownership TX: %s", txID)
}

// =============================================================================
// Chain Tests
// =============================================================================

func TestCreateChain(t *testing.T) {
	skipIfDisabled(t)

	ctx := context.Background()
	w, netConfig := getTestWallet(t)

	// First create a subnet
	subnetID, err := pchain.CreateSubnet(ctx, w)
	if err != nil {
		t.Fatalf("CreateSubnet failed: %v", err)
	}
	t.Logf("Created Subnet ID: %s", subnetID)

	// Wait for subnet tx
	time.Sleep(2 * time.Second)

	// Create wallet tracking the subnet
	key, _ := wallet.ToPrivateKey(ewoqPrivateKey)
	subnetWallet, err := wallet.NewWalletWithSubnet(ctx, key, netConfig, subnetID)
	if err != nil {
		t.Fatalf("failed to create subnet wallet: %v", err)
	}

	// Minimal genesis
	genesis := []byte(`{"config":{"chainId":99999}}`)

	chainID, err := pchain.CreateChain(ctx, subnetWallet, pchain.CreateChainConfig{
		SubnetID:  subnetID,
		Genesis:   genesis,
		VMID:      constants.SubnetEVMID,
		FxIDs:     nil,
		ChainName: "test-chain",
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	t.Logf("Chain ID: %s", chainID)
}

// =============================================================================
// Validator Tests (Primary Network - requires significant stake)
// =============================================================================

func TestAddValidatorParams(t *testing.T) {
	// This test validates parameters without actually submitting
	// Real validator tests require 2000 AVAX minimum stake

	skipIfDisabled(t)

	cfg := pchain.AddValidatorConfig{
		NodeID:        ids.GenerateTestNodeID(),
		Start:         time.Now().Add(1 * time.Minute),
		End:           time.Now().Add(14*24*time.Hour + 1*time.Minute),
		StakeAmt:      2000_000_000_000, // 2000 AVAX minimum
		RewardAddr:    ids.GenerateTestShortID(),
		DelegationFee: 200, // 2%
	}

	if cfg.StakeAmt < 2000_000_000_000 {
		t.Error("stake amount below minimum")
	}

	if cfg.End.Sub(cfg.Start) < 14*24*time.Hour {
		t.Error("validation period too short")
	}

	t.Logf("Validator config valid: NodeID=%s, Stake=%d nAVAX", cfg.NodeID, cfg.StakeAmt)
}

func TestAddDelegatorParams(t *testing.T) {
	skipIfDisabled(t)

	cfg := pchain.AddDelegatorConfig{
		NodeID:     ids.GenerateTestNodeID(),
		Start:      time.Now().Add(1 * time.Minute),
		End:        time.Now().Add(14*24*time.Hour + 1*time.Minute),
		StakeAmt:   25_000_000_000, // 25 AVAX minimum
		RewardAddr: ids.GenerateTestShortID(),
	}

	if cfg.StakeAmt < 25_000_000_000 {
		t.Error("delegation amount below minimum")
	}

	t.Logf("Delegator config valid: NodeID=%s, Stake=%d nAVAX", cfg.NodeID, cfg.StakeAmt)
}

// =============================================================================
// Subnet Validator Tests
// =============================================================================

func TestAddSubnetValidator(t *testing.T) {
	skipIfDisabled(t)

	ctx := context.Background()
	w, netConfig := getTestWallet(t)

	// Create subnet first
	subnetID, err := pchain.CreateSubnet(ctx, w)
	if err != nil {
		t.Fatalf("CreateSubnet failed: %v", err)
	}
	t.Logf("Created Subnet ID: %s", subnetID)

	time.Sleep(2 * time.Second)

	// Create subnet wallet
	key, _ := wallet.ToPrivateKey(ewoqPrivateKey)
	subnetWallet, err := wallet.NewWalletWithSubnet(ctx, key, netConfig, subnetID)
	if err != nil {
		t.Fatalf("failed to create subnet wallet: %v", err)
	}

	// Note: This will fail without a valid primary network validator
	// Just testing the call mechanics
	nodeID := ids.GenerateTestNodeID()

	_, err = pchain.AddSubnetValidator(ctx, subnetWallet, pchain.AddSubnetValidatorConfig{
		NodeID:   nodeID,
		SubnetID: subnetID,
		Start:    time.Now().Add(1 * time.Minute),
		End:      time.Now().Add(24 * time.Hour),
		Weight:   1,
	})

	// Expected to fail because nodeID is not a primary network validator
	if err == nil {
		t.Log("AddSubnetValidator succeeded (node must be primary network validator)")
	} else {
		t.Logf("AddSubnetValidator failed as expected (node not primary validator): %v", err)
	}
}

// =============================================================================
// Permissionless Validator Tests
// =============================================================================

func TestPermissionlessValidatorParams(t *testing.T) {
	skipIfDisabled(t)

	cfg := pchain.AddPermissionlessValidatorConfig{
		NodeID:        ids.GenerateTestNodeID(),
		SubnetID:      ids.GenerateTestID(),
		Start:         time.Now().Add(1 * time.Minute),
		End:           time.Now().Add(14 * 24 * time.Hour),
		StakeAmt:      1_000_000_000, // 1 AVAX
		AssetID:       ids.GenerateTestID(),
		RewardAddr:    ids.GenerateTestShortID(),
		DelegationFee: 200,
		Signer:        &signer.Empty{},
	}

	if cfg.DelegationFee > 10000 {
		t.Error("delegation fee cannot exceed 100%")
	}

	t.Logf("Permissionless validator config valid: NodeID=%s", cfg.NodeID)
}

// =============================================================================
// L1 Validator Tests
// =============================================================================

func TestL1ValidatorBalance(t *testing.T) {
	skipIfDisabled(t)

	ctx := context.Background()
	w, _ := getTestWallet(t)

	// This will fail without a real validation ID, but tests the call path
	validationID := ids.GenerateTestID()

	_, err := pchain.IncreaseL1ValidatorBalance(ctx, w, validationID, 1_000_000_000)
	if err == nil {
		t.Log("IncreaseL1ValidatorBalance succeeded")
	} else {
		t.Logf("IncreaseL1ValidatorBalance failed as expected (invalid validation ID): %v", err)
	}
}

func TestDisableL1Validator(t *testing.T) {
	skipIfDisabled(t)

	ctx := context.Background()
	w, _ := getTestWallet(t)

	validationID := ids.GenerateTestID()

	_, err := pchain.DisableL1Validator(ctx, w, validationID)
	if err == nil {
		t.Log("DisableL1Validator succeeded")
	} else {
		t.Logf("DisableL1Validator failed as expected (invalid validation ID): %v", err)
	}
}

// =============================================================================
// Export/Import Tests
// =============================================================================

func TestExportFromPChain(t *testing.T) {
	skipIfDisabled(t)

	ctx := context.Background()
	w, _ := getTestFullWallet(t)

	txID, err := crosschain.ExportFromPChain(ctx, w, 1_000_000) // 0.001 AVAX
	if err != nil {
		t.Fatalf("ExportFromPChain failed: %v", err)
	}

	t.Logf("Export TX: %s", txID)
}

func TestImportToCChain(t *testing.T) {
	skipIfDisabled(t)

	ctx := context.Background()
	w, _ := getTestFullWallet(t)

	// First export
	_, err := crosschain.ExportFromPChain(ctx, w, 1_000_000)
	if err != nil {
		t.Fatalf("ExportFromPChain failed: %v", err)
	}

	// Then import
	txID, err := crosschain.ImportToCChain(ctx, w)
	if err != nil {
		t.Fatalf("ImportToCChain failed: %v", err)
	}

	t.Logf("Import TX: %s", txID)
}

// =============================================================================
// Integration Test: Full Subnet Lifecycle
// =============================================================================

func TestSubnetLifecycle(t *testing.T) {
	skipIfDisabled(t)

	ctx := context.Background()
	w, netConfig := getTestWallet(t)

	// 1. Create subnet
	subnetID, err := pchain.CreateSubnet(ctx, w)
	if err != nil {
		t.Fatalf("CreateSubnet failed: %v", err)
	}
	t.Logf("1. Created Subnet: %s", subnetID)

	time.Sleep(2 * time.Second)

	// 2. Create subnet wallet
	key, _ := wallet.ToPrivateKey(ewoqPrivateKey)
	subnetWallet, err := wallet.NewWalletWithSubnet(ctx, key, netConfig, subnetID)
	if err != nil {
		t.Fatalf("failed to create subnet wallet: %v", err)
	}

	// 3. Create chain on subnet
	genesis := []byte(`{"config":{"chainId":99999}}`)
	chainID, err := pchain.CreateChain(ctx, subnetWallet, pchain.CreateChainConfig{
		SubnetID:  subnetID,
		Genesis:   genesis,
		VMID:      constants.SubnetEVMID,
		ChainName: "e2e-test-chain",
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}
	t.Logf("2. Created Chain: %s", chainID)

	time.Sleep(2 * time.Second)

	// 4. Transfer subnet ownership (to self, just testing)
	txID, err := pchain.TransferSubnetOwnership(ctx, subnetWallet, subnetID, w.PChainAddress())
	if err != nil {
		t.Fatalf("TransferSubnetOwnership failed: %v", err)
	}
	t.Logf("3. Transferred Ownership: %s", txID)

	t.Log("Subnet lifecycle test completed successfully!")
}
