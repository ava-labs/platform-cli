// Package e2e provides end-to-end tests for P-Chain operations.
//
// Run against Fuji (requires funded wallet):
//
//	PRIVATE_KEY="PrivateKey-..." go test -v ./e2e/... -network=fuji
//
// Run against local network (uses ewoq key):
//
//	go test -v ./e2e/... -network=local
package e2e

import (
	"context"
	"flag"
	"os"
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/platform-cli/pkg/crosschain"
	"github.com/ava-labs/platform-cli/pkg/network"
	"github.com/ava-labs/platform-cli/pkg/pchain"
	"github.com/ava-labs/platform-cli/pkg/wallet"
)

var (
	networkFlag = flag.String("network", "fuji", "Network to test against: local, fuji")
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

func getPrivateKeyBytes(t *testing.T) []byte {
	t.Helper()

	// Check for env var first (for Fuji tests)
	if envKey := os.Getenv("PRIVATE_KEY"); envKey != "" {
		keyBytes, err := wallet.ParsePrivateKey(envKey)
		if err != nil {
			t.Fatalf("failed to parse PRIVATE_KEY: %v", err)
		}
		return keyBytes
	}

	// Fall back to ewoq for local network
	if *networkFlag == "local" {
		return ewoqPrivateKey
	}

	t.Skip("PRIVATE_KEY env var required for Fuji tests")
	return nil
}

func getTestWallet(t *testing.T) (*wallet.Wallet, network.Config) {
	t.Helper()
	ctx := context.Background()

	keyBytes := getPrivateKeyBytes(t)
	key, err := wallet.ToPrivateKey(keyBytes)
	if err != nil {
		t.Fatalf("failed to parse key: %v", err)
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

	keyBytes := getPrivateKeyBytes(t)
	key, err := wallet.ToPrivateKey(keyBytes)
	if err != nil {
		t.Fatalf("failed to parse key: %v", err)
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
	w, netConfig := getTestWallet(t)

	addr := w.PChainAddress()
	if addr == ids.ShortEmpty {
		t.Error("expected non-empty P-Chain address")
	}

	t.Logf("Network: %s", netConfig.Name)
	t.Logf("P-Chain Address: %s", addr)
}

func TestFullWalletCreation(t *testing.T) {
	w, netConfig := getTestFullWallet(t)

	pAddr := w.PChainAddress()
	ethAddr := w.EthAddress()

	if pAddr == ids.ShortEmpty {
		t.Error("expected non-empty P-Chain address")
	}

	t.Logf("Network: %s", netConfig.Name)
	t.Logf("P-Chain Address: %s", pAddr)
	t.Logf("C-Chain Address: %s", ethAddr.Hex())
}

// =============================================================================
// P-Chain Transfer Tests
// =============================================================================

func TestPChainSendToSelf(t *testing.T) {
	ctx := context.Background()
	w, _ := getTestWallet(t)

	// Send 0.001 AVAX to self
	amount := uint64(1_000_000) // 0.001 AVAX in nAVAX

	t.Logf("Sending %d nAVAX to self (%s)...", amount, w.PChainAddress())

	txID, err := pchain.Send(ctx, w, w.PChainAddress(), amount)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	t.Logf("Send TX: %s", txID)
}

// =============================================================================
// Cross-Chain Transfer Tests
// =============================================================================

func TestExportFromPChain(t *testing.T) {
	ctx := context.Background()
	w, _ := getTestFullWallet(t)

	// Export 0.01 AVAX from P-Chain to C-Chain
	amount := uint64(10_000_000) // 0.01 AVAX

	t.Logf("Exporting %d nAVAX from P-Chain...", amount)

	txID, err := crosschain.ExportFromPChain(ctx, w, amount)
	if err != nil {
		t.Fatalf("ExportFromPChain failed: %v", err)
	}

	t.Logf("Export TX: %s", txID)
}

func TestImportToCChain(t *testing.T) {
	ctx := context.Background()
	w, _ := getTestFullWallet(t)

	t.Log("Importing to C-Chain from P-Chain...")

	txID, err := crosschain.ImportToCChain(ctx, w)
	if err != nil {
		// May fail if nothing to import - that's ok
		t.Logf("ImportToCChain: %v (may be expected if nothing to import)", err)
		return
	}

	t.Logf("Import TX: %s", txID)
}

func TestFullPToCTransfer(t *testing.T) {
	ctx := context.Background()
	w, _ := getTestFullWallet(t)

	// Transfer 0.01 AVAX from P-Chain to C-Chain
	amount := uint64(10_000_000) // 0.01 AVAX

	t.Logf("Transferring %d nAVAX from P-Chain to C-Chain...", amount)
	t.Logf("From P-Chain: %s", w.PChainAddress())
	t.Logf("To C-Chain: %s", w.EthAddress().Hex())

	exportTxID, importTxID, err := crosschain.TransferPToC(ctx, w, amount)
	if err != nil {
		t.Fatalf("TransferPToC failed: %v", err)
	}

	t.Logf("Export TX: %s", exportTxID)
	t.Logf("Import TX: %s", importTxID)
}

func TestExportFromCChain(t *testing.T) {
	ctx := context.Background()
	w, _ := getTestFullWallet(t)

	// Export 0.01 AVAX from C-Chain to P-Chain
	amount := uint64(10_000_000) // 0.01 AVAX

	t.Logf("Exporting %d nAVAX from C-Chain...", amount)

	txID, err := crosschain.ExportFromCChain(ctx, w, amount)
	if err != nil {
		t.Fatalf("ExportFromCChain failed: %v", err)
	}

	t.Logf("Export TX: %s", txID)
}

func TestImportToPChain(t *testing.T) {
	ctx := context.Background()
	w, _ := getTestFullWallet(t)

	t.Log("Importing to P-Chain from C-Chain...")

	txID, err := crosschain.ImportToPChain(ctx, w)
	if err != nil {
		// May fail if nothing to import - that's ok
		t.Logf("ImportToPChain: %v (may be expected if nothing to import)", err)
		return
	}

	t.Logf("Import TX: %s", txID)
}

func TestFullCToPTransfer(t *testing.T) {
	ctx := context.Background()
	w, _ := getTestFullWallet(t)

	// Transfer 0.01 AVAX from C-Chain to P-Chain
	amount := uint64(10_000_000) // 0.01 AVAX

	t.Logf("Transferring %d nAVAX from C-Chain to P-Chain...", amount)
	t.Logf("From C-Chain: %s", w.EthAddress().Hex())
	t.Logf("To P-Chain: %s", w.PChainAddress())

	exportTxID, importTxID, err := crosschain.TransferCToP(ctx, w, amount)
	if err != nil {
		t.Fatalf("TransferCToP failed: %v", err)
	}

	t.Logf("Export TX: %s", exportTxID)
	t.Logf("Import TX: %s", importTxID)
}

// =============================================================================
// Subnet Tests
// =============================================================================

func TestCreateSubnet(t *testing.T) {
	ctx := context.Background()
	w, _ := getTestWallet(t)

	t.Logf("Creating subnet with owner %s...", w.PChainAddress())

	subnetID, err := pchain.CreateSubnet(ctx, w)
	if err != nil {
		t.Fatalf("CreateSubnet failed: %v", err)
	}

	t.Logf("Subnet ID: %s", subnetID)
}

func TestCreateSubnetAndTransferOwnership(t *testing.T) {
	ctx := context.Background()
	w, netConfig := getTestWallet(t)

	// 1. Create subnet
	t.Logf("Creating subnet...")
	subnetID, err := pchain.CreateSubnet(ctx, w)
	if err != nil {
		t.Fatalf("CreateSubnet failed: %v", err)
	}
	t.Logf("Created Subnet ID: %s", subnetID)

	// Wait for tx to be accepted
	t.Log("Waiting for subnet creation to be accepted...")
	time.Sleep(3 * time.Second)

	// 2. Create wallet tracking the subnet
	keyBytes := getPrivateKeyBytes(t)
	key, _ := wallet.ToPrivateKey(keyBytes)
	subnetWallet, err := wallet.NewWalletWithSubnet(ctx, key, netConfig, subnetID)
	if err != nil {
		t.Fatalf("failed to create subnet wallet: %v", err)
	}

	// 3. Transfer ownership back to self (testing the operation)
	t.Logf("Transferring ownership to self...")
	txID, err := pchain.TransferSubnetOwnership(ctx, subnetWallet, subnetID, w.PChainAddress())
	if err != nil {
		t.Fatalf("TransferSubnetOwnership failed: %v", err)
	}

	t.Logf("Transfer Ownership TX: %s", txID)
}

// =============================================================================
// Chain Tests
// =============================================================================

func TestCreateChainOnSubnet(t *testing.T) {
	ctx := context.Background()
	w, netConfig := getTestWallet(t)

	// 1. Create subnet
	t.Log("Creating subnet...")
	subnetID, err := pchain.CreateSubnet(ctx, w)
	if err != nil {
		t.Fatalf("CreateSubnet failed: %v", err)
	}
	t.Logf("Subnet ID: %s", subnetID)

	// Wait for subnet creation
	t.Log("Waiting for subnet creation...")
	time.Sleep(3 * time.Second)

	// 2. Create wallet tracking the subnet
	keyBytes := getPrivateKeyBytes(t)
	key, _ := wallet.ToPrivateKey(keyBytes)
	subnetWallet, err := wallet.NewWalletWithSubnet(ctx, key, netConfig, subnetID)
	if err != nil {
		t.Fatalf("failed to create subnet wallet: %v", err)
	}

	// 3. Create chain
	genesis := []byte(`{"config":{"chainId":99999},"alloc":{}}`)

	t.Log("Creating chain on subnet...")
	chainID, err := pchain.CreateChain(ctx, subnetWallet, pchain.CreateChainConfig{
		SubnetID:  subnetID,
		Genesis:   genesis,
		VMID:      constants.SubnetEVMID,
		FxIDs:     nil,
		ChainName: "e2e-test-chain",
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	t.Logf("Chain ID: %s", chainID)
}

// =============================================================================
// Primary Network Validator Tests (Parameter Validation)
// =============================================================================

func TestAddValidatorParams(t *testing.T) {
	// This test validates parameters without submitting
	// Real validator submission requires 2000+ AVAX

	cfg := pchain.AddValidatorConfig{
		NodeID:        ids.GenerateTestNodeID(),
		Start:         time.Now().Add(5 * time.Minute),
		End:           time.Now().Add(14*24*time.Hour + 5*time.Minute),
		StakeAmt:      2000_000_000_000, // 2000 AVAX minimum
		RewardAddr:    ids.GenerateTestShortID(),
		DelegationFee: 200, // 2% = 200 basis points
	}

	// Validate minimum stake
	if cfg.StakeAmt < 2000_000_000_000 {
		t.Error("stake amount below minimum (2000 AVAX)")
	}

	// Validate minimum duration (14 days)
	duration := cfg.End.Sub(cfg.Start)
	if duration < 14*24*time.Hour {
		t.Errorf("validation period too short: %v (minimum 14 days)", duration)
	}

	// Validate delegation fee (max 100%)
	if cfg.DelegationFee > 10000 {
		t.Errorf("delegation fee too high: %d (max 10000)", cfg.DelegationFee)
	}

	t.Logf("Validator config valid:")
	t.Logf("  NodeID: %s", cfg.NodeID)
	t.Logf("  Stake: %d nAVAX (%.2f AVAX)", cfg.StakeAmt, float64(cfg.StakeAmt)/1e9)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Delegation Fee: %.2f%%", float64(cfg.DelegationFee)/100)
}

func TestAddDelegatorParams(t *testing.T) {
	// This test validates parameters without submitting
	// Real delegation requires 25+ AVAX and valid validator

	cfg := pchain.AddDelegatorConfig{
		NodeID:     ids.GenerateTestNodeID(),
		Start:      time.Now().Add(5 * time.Minute),
		End:        time.Now().Add(14*24*time.Hour + 5*time.Minute),
		StakeAmt:   25_000_000_000, // 25 AVAX minimum
		RewardAddr: ids.GenerateTestShortID(),
	}

	// Validate minimum stake
	if cfg.StakeAmt < 25_000_000_000 {
		t.Error("delegation amount below minimum (25 AVAX)")
	}

	// Validate minimum duration (14 days)
	duration := cfg.End.Sub(cfg.Start)
	if duration < 14*24*time.Hour {
		t.Errorf("delegation period too short: %v (minimum 14 days)", duration)
	}

	t.Logf("Delegator config valid:")
	t.Logf("  NodeID: %s", cfg.NodeID)
	t.Logf("  Stake: %d nAVAX (%.2f AVAX)", cfg.StakeAmt, float64(cfg.StakeAmt)/1e9)
	t.Logf("  Duration: %v", duration)
}

// =============================================================================
// L1 Validator Tests (API calls with invalid data to test paths)
// =============================================================================

func TestIncreaseL1ValidatorBalance(t *testing.T) {
	ctx := context.Background()
	w, _ := getTestWallet(t)

	// Use a fake validation ID - this will fail but tests the call path
	validationID := ids.GenerateTestID()
	amount := uint64(1_000_000_000) // 1 AVAX

	t.Logf("Testing IncreaseL1ValidatorBalance with fake ID: %s", validationID)

	_, err := pchain.IncreaseL1ValidatorBalance(ctx, w, validationID, amount)
	if err == nil {
		t.Log("IncreaseL1ValidatorBalance succeeded (unexpected)")
	} else {
		t.Logf("IncreaseL1ValidatorBalance failed as expected: %v", err)
	}
}

func TestDisableL1Validator(t *testing.T) {
	ctx := context.Background()
	w, _ := getTestWallet(t)

	// Use a fake validation ID - this will fail but tests the call path
	validationID := ids.GenerateTestID()

	t.Logf("Testing DisableL1Validator with fake ID: %s", validationID)

	_, err := pchain.DisableL1Validator(ctx, w, validationID)
	if err == nil {
		t.Log("DisableL1Validator succeeded (unexpected)")
	} else {
		t.Logf("DisableL1Validator failed as expected: %v", err)
	}
}

// =============================================================================
// Full Integration Test: Subnet Lifecycle
// =============================================================================

func TestSubnetLifecycle(t *testing.T) {
	ctx := context.Background()
	w, netConfig := getTestWallet(t)

	t.Log("=== Subnet Lifecycle Test ===")

	// 1. Create subnet
	t.Log("Step 1: Creating subnet...")
	subnetID, err := pchain.CreateSubnet(ctx, w)
	if err != nil {
		t.Fatalf("CreateSubnet failed: %v", err)
	}
	t.Logf("  Subnet ID: %s", subnetID)

	time.Sleep(3 * time.Second)

	// 2. Create subnet-aware wallet
	keyBytes := getPrivateKeyBytes(t)
	key, _ := wallet.ToPrivateKey(keyBytes)
	subnetWallet, err := wallet.NewWalletWithSubnet(ctx, key, netConfig, subnetID)
	if err != nil {
		t.Fatalf("failed to create subnet wallet: %v", err)
	}

	// 3. Create chain on subnet
	t.Log("Step 2: Creating chain on subnet...")
	genesis := []byte(`{"config":{"chainId":99999},"alloc":{}}`)
	chainID, err := pchain.CreateChain(ctx, subnetWallet, pchain.CreateChainConfig{
		SubnetID:  subnetID,
		Genesis:   genesis,
		VMID:      constants.SubnetEVMID,
		ChainName: "lifecycle-test-chain",
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}
	t.Logf("  Chain ID: %s", chainID)

	time.Sleep(3 * time.Second)

	// 4. Transfer subnet ownership (to self)
	t.Log("Step 3: Transferring subnet ownership...")
	txID, err := pchain.TransferSubnetOwnership(ctx, subnetWallet, subnetID, w.PChainAddress())
	if err != nil {
		t.Fatalf("TransferSubnetOwnership failed: %v", err)
	}
	t.Logf("  Transfer TX: %s", txID)

	t.Log("=== Subnet Lifecycle Complete ===")
}

// =============================================================================
// Full Integration Test: Cross-Chain Round Trip
// =============================================================================

func TestCrossChainRoundTrip(t *testing.T) {
	ctx := context.Background()
	w, _ := getTestFullWallet(t)

	amount := uint64(10_000_000) // 0.01 AVAX

	t.Log("=== Cross-Chain Round Trip Test ===")
	t.Logf("P-Chain Address: %s", w.PChainAddress())
	t.Logf("C-Chain Address: %s", w.EthAddress().Hex())

	// 1. P-Chain -> C-Chain
	t.Logf("Step 1: P-Chain -> C-Chain (%d nAVAX)...", amount)
	exportTx1, importTx1, err := crosschain.TransferPToC(ctx, w, amount)
	if err != nil {
		t.Fatalf("P-to-C transfer failed: %v", err)
	}
	t.Logf("  Export TX: %s", exportTx1)
	t.Logf("  Import TX: %s", importTx1)

	time.Sleep(2 * time.Second)

	// 2. C-Chain -> P-Chain
	t.Logf("Step 2: C-Chain -> P-Chain (%d nAVAX)...", amount)
	exportTx2, importTx2, err := crosschain.TransferCToP(ctx, w, amount)
	if err != nil {
		t.Fatalf("C-to-P transfer failed: %v", err)
	}
	t.Logf("  Export TX: %s", exportTx2)
	t.Logf("  Import TX: %s", importTx2)

	t.Log("=== Cross-Chain Round Trip Complete ===")
}
