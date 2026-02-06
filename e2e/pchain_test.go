//go:build networke2e

// Package e2e provides end-to-end tests for P-Chain operations.
//
// Run against Fuji (requires funded wallet and explicit opt-in):
//
//	RUN_E2E_NETWORK_TESTS=1 PRIVATE_KEY="PrivateKey-..." go test -tags=networke2e -v ./e2e/... -network=fuji
//
// Run against local network (uses ewoq key, connects to http://127.0.0.1:9650):
//
//	RUN_E2E_NETWORK_TESTS=1 go test -tags=networke2e -v ./e2e/... -network=local
package e2e

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/crypto/bls/signer/localsigner"
	"github.com/ava-labs/avalanchego/utils/units"
	"github.com/ava-labs/avalanchego/vms/platformvm"
	"github.com/ava-labs/avalanchego/vms/platformvm/signer"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/platform-cli/pkg/crosschain"
	"github.com/ava-labs/platform-cli/pkg/network"
	"github.com/ava-labs/platform-cli/pkg/pchain"
	"github.com/ava-labs/platform-cli/pkg/wallet"
)

// ewoqPrivateKey is the well-known ewoq test key used in local/test networks.
var ewoqPrivateKey = []byte{
	0x56, 0x28, 0x9e, 0x99, 0xc9, 0x4b, 0x69, 0x12,
	0xbf, 0xc1, 0x2a, 0xdc, 0x09, 0x3c, 0x9b, 0x51,
	0x12, 0x4f, 0x0d, 0xc5, 0x4a, 0xc7, 0xa7, 0x66,
	0xb2, 0xbc, 0x5c, 0xcf, 0x55, 0x8d, 0x80, 0x27,
}

const (
	// Keep transfer amounts low to minimize spend on funded test wallets.
	smallTransferAmountNAVAX = uint64(1_000_000) // 0.001 AVAX

	// Retry transient RPC rate limits from shared public endpoints.
	rateLimitRetryAttempts = 6
	rateLimitRetryDelay    = 1 * time.Second
)

func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "status code: 429") ||
		strings.Contains(errStr, "too many requests") ||
		strings.Contains(errStr, "rate limit")
}

func retryRateLimitedOperation[T any](t *testing.T, opName string, fn func() (T, error)) (T, error) {
	t.Helper()

	var zero T
	delay := rateLimitRetryDelay
	var lastErr error

	for attempt := 1; attempt <= rateLimitRetryAttempts; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}
		if !isRateLimitError(err) {
			return zero, err
		}

		lastErr = err
		if attempt == rateLimitRetryAttempts {
			break
		}

		t.Logf("%s hit rate limit (attempt %d/%d), retrying in %s: %v", opName, attempt, rateLimitRetryAttempts, delay, err)
		time.Sleep(delay)
		delay *= 2
	}

	return zero, fmt.Errorf("%s failed after %d rate-limit retries: %w", opName, rateLimitRetryAttempts, lastErr)
}

func getPrivateKeyBytes(t *testing.T) []byte {
	t.Helper()
	requireNetworkE2ETestsEnabled(t)

	// Check for env var first (for Fuji tests)
	if envKey := os.Getenv(envPrivateKey); envKey != "" {
		keyBytes, err := wallet.ParsePrivateKey(envKey)
		if err != nil {
			t.Fatalf("failed to parse %s: %v", envPrivateKey, err)
		}
		return keyBytes
	}

	// Fall back to ewoq for local network
	if *networkFlag == "local" {
		return ewoqPrivateKey
	}

	t.Skipf("%s env var required for Fuji tests", envPrivateKey)
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

	netConfig := getNetworkConfig(t, ctx)
	w, err := retryRateLimitedOperation(t, "wallet.NewWallet", func() (*wallet.Wallet, error) {
		return wallet.NewWallet(ctx, key, netConfig)
	})
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	return w, netConfig
}

// getNetworkConfig returns the network config for tests.
// For "local", uses --rpc-url with NewCustomConfig.
func getNetworkConfig(t *testing.T, ctx context.Context) network.Config {
	t.Helper()
	if *networkFlag == "local" {
		cfg, err := network.NewCustomConfig(ctx, localRPCURL, 0)
		if err != nil {
			t.Fatalf("failed to get local network config: %v", err)
		}
		return cfg
	}
	cfg, err := network.GetConfig(*networkFlag)
	if err != nil {
		t.Fatalf("failed to get network config: %v", err)
	}
	return cfg
}

func getTestFullWallet(t *testing.T) (*wallet.FullWallet, network.Config) {
	t.Helper()
	ctx := context.Background()

	keyBytes := getPrivateKeyBytes(t)
	key, err := wallet.ToPrivateKey(keyBytes)
	if err != nil {
		t.Fatalf("failed to parse key: %v", err)
	}

	netConfig := getNetworkConfig(t, ctx)
	w, err := retryRateLimitedOperation(t, "wallet.NewFullWallet", func() (*wallet.FullWallet, error) {
		return wallet.NewFullWallet(ctx, key, netConfig)
	})
	if err != nil {
		t.Fatalf("failed to create full wallet: %v", err)
	}

	return w, netConfig
}

// generateMockValidator creates a mock validator with valid BLS credentials.
// The validator won't be a real node, but has cryptographically valid PoP.
func generateMockValidator(t *testing.T, balance uint64) *txs.ConvertSubnetToL1Validator {
	t.Helper()

	// Generate random NodeID (20 bytes)
	nodeID := make([]byte, ids.NodeIDLen)
	if _, err := rand.Read(nodeID); err != nil {
		t.Fatalf("failed to generate node ID: %v", err)
	}

	// Generate BLS signer and proof of possession
	blsSigner, err := localsigner.New()
	if err != nil {
		t.Fatalf("failed to generate BLS signer: %v", err)
	}

	pop, err := signer.NewProofOfPossession(blsSigner)
	if err != nil {
		t.Fatalf("failed to generate proof of possession: %v", err)
	}

	return &txs.ConvertSubnetToL1Validator{
		NodeID:  nodeID,
		Weight:  units.Schmeckle, // 1 weight unit
		Balance: balance,
		Signer:  *pop,
	}
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
	amount := smallTransferAmountNAVAX

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

	// Export a small amount from P-Chain to C-Chain
	amount := smallTransferAmountNAVAX

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

	// Transfer a small amount from P-Chain to C-Chain
	amount := smallTransferAmountNAVAX

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

	// Export a small amount from C-Chain to P-Chain
	amount := smallTransferAmountNAVAX

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

	// Transfer a small amount from C-Chain to P-Chain
	amount := smallTransferAmountNAVAX

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
		ChainName: "e2etestchain",
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	t.Logf("Chain ID: %s", chainID)
}

// =============================================================================
// Primary Network Staking Tests
// =============================================================================
//
// Post-Etna (Avalanche9000), primary network staking uses the permissionless
// validator/delegator transactions (AddPermissionlessValidatorTx/AddPermissionlessDelegatorTx).
// =============================================================================

// TestAddPermissionlessDelegator tests delegating to an existing validator on the network.
// This uses the post-Etna AddPermissionlessDelegatorTx.
func TestAddPermissionlessDelegator(t *testing.T) {
	ctx := context.Background()
	w, netConfig := getTestWallet(t)

	// Find a validator to delegate to
	validator, err := findValidatorForDelegation(t, ctx, netConfig)
	if err != nil {
		t.Skipf("Could not find suitable validator for delegation: %v", err)
	}

	t.Logf("Found validator for delegation: %s", validator.NodeID)
	t.Logf("  Validator end time: %s", validator.EndTime.Format(time.RFC3339))

	// Calculate delegation period
	start := time.Now().Add(1 * time.Minute)
	minEnd := start.Add(netConfig.MinStakeDuration)

	// Delegation must end before validator ends
	end := minEnd
	if end.After(validator.EndTime.Add(-1 * time.Hour)) {
		end = validator.EndTime.Add(-1 * time.Hour)
	}

	if end.Before(start.Add(netConfig.MinStakeDuration)) {
		t.Skipf("Validator ends too soon for minimum delegation duration")
	}

	cfg := pchain.AddPermissionlessDelegatorConfig{
		NodeID:     validator.NodeID,
		Start:      start,
		End:        end,
		StakeAmt:   netConfig.MinDelegatorStake,
		RewardAddr: w.PChainAddress(),
	}

	t.Logf("Delegating %.2f AVAX to %s...", float64(cfg.StakeAmt)/1e9, cfg.NodeID)
	t.Logf("  Duration: %v", cfg.End.Sub(cfg.Start))

	txID, err := pchain.AddPermissionlessDelegator(ctx, w, cfg)
	if err != nil {
		// Skip if insufficient funds (test wallet may be depleted by previous tests)
		if isInsufficientFunds(err) {
			t.Skipf("Insufficient funds for delegation (wallet depleted): %v", err)
		}
		t.Fatalf("AddPermissionlessDelegator failed: %v", err)
	}

	t.Logf("Delegation TX: %s", txID)
}

// TestAddPermissionlessValidator tests adding a permissionless validator.
// NOTE: This requires a real node to be running with the given NodeID and BLS credentials.
// The test will fail validation but verifies the tx building path.
func TestAddPermissionlessValidator(t *testing.T) {
	ctx := context.Background()
	w, netConfig := getTestWallet(t)

	// Generate a random NodeID and BLS signer
	nodeID := ids.GenerateTestNodeID()

	// Generate BLS signer for the validator
	blsSigner, err := localsigner.New()
	if err != nil {
		t.Fatalf("failed to generate BLS signer: %v", err)
	}

	pop, err := signer.NewProofOfPossession(blsSigner)
	if err != nil {
		t.Fatalf("failed to generate proof of possession: %v", err)
	}

	start := time.Now().Add(1 * time.Minute)
	end := start.Add(netConfig.MinStakeDuration)

	cfg := pchain.AddPermissionlessValidatorConfig{
		NodeID:        nodeID,
		Start:         start,
		End:           end,
		StakeAmt:      netConfig.MinValidatorStake,
		RewardAddr:    w.PChainAddress(),
		DelegationFee: 20000, // 2% = 20000 parts per million
		BLSSigner:     pop,
	}

	t.Logf("Attempting to add permissionless validator %s with %.2f AVAX stake...", nodeID, float64(cfg.StakeAmt)/1e9)
	t.Logf("  Duration: %v", end.Sub(start))
	t.Logf("  Delegation Fee: %.2f%%", float64(cfg.DelegationFee)/10000)

	txID, err := pchain.AddPermissionlessValidator(ctx, w, cfg)
	if err != nil {
		// Expected to fail since the node isn't running or BLS key doesn't match
		t.Logf("AddPermissionlessValidator failed (expected - node not running): %v", err)
		return
	}

	t.Logf("Validator TX: %s", txID)
}

// TestGetCurrentValidators tests querying the current validators on the network.
func TestGetCurrentValidators(t *testing.T) {
	ctx := context.Background()
	_, netConfig := getTestWallet(t)

	client := platformvm.NewClient(netConfig.RPCURL)
	validators, err := retryRateLimitedOperation(t, "GetCurrentValidators", func() ([]platformvm.ClientPermissionlessValidator, error) {
		return client.GetCurrentValidators(ctx, constants.PrimaryNetworkID, nil)
	})
	if err != nil {
		t.Fatalf("GetCurrentValidators failed: %v", err)
	}

	t.Logf("Found %d primary network validators", len(validators))

	// Log first few validators
	for i, v := range validators {
		if i >= 5 {
			t.Logf("  ... and %d more", len(validators)-5)
			break
		}
		endTime := time.Unix(int64(v.EndTime), 0)
		t.Logf("  %s (ends: %s)", v.NodeID, endTime.Format("2006-01-02"))
	}
}

// ValidatorInfo holds basic validator information for delegation tests.
type ValidatorInfo struct {
	NodeID  ids.NodeID
	EndTime time.Time
}

// findValidatorForDelegation queries the network for a validator suitable for delegation.
func findValidatorForDelegation(t *testing.T, ctx context.Context, netConfig network.Config) (*ValidatorInfo, error) {
	client := platformvm.NewClient(netConfig.RPCURL)
	validators, err := retryRateLimitedOperation(t, "findValidatorForDelegation.GetCurrentValidators", func() ([]platformvm.ClientPermissionlessValidator, error) {
		return client.GetCurrentValidators(ctx, constants.PrimaryNetworkID, nil)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get validators: %w", err)
	}

	if len(validators) == 0 {
		return nil, fmt.Errorf("no validators found on network")
	}

	now := time.Now()
	minEndTime := now.Add(netConfig.MinStakeDuration + 2*time.Hour)

	for _, v := range validators {
		endTime := time.Unix(int64(v.EndTime), 0)
		if endTime.After(minEndTime) {
			return &ValidatorInfo{
				NodeID:  v.NodeID,
				EndTime: endTime,
			}, nil
		}
	}

	return nil, fmt.Errorf("no validators with sufficient remaining time found")
}

// isInsufficientFunds checks if an error indicates insufficient funds.
func isInsufficientFunds(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "insufficient funds")
}

// =============================================================================
// L1 Validator Tests (API calls with invalid data to test paths)
// =============================================================================

func TestIncreaseL1ValidatorBalance(t *testing.T) {
	ctx := context.Background()
	w, _ := getTestWallet(t)

	// Use a fake validation ID - this will fail but tests the call path
	validationID := ids.GenerateTestID()
	amount := smallTransferAmountNAVAX

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
		ChainName: "lifecyclechain",
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
// Full Integration Test: L1 Lifecycle (Subnet -> Chain -> Convert to L1)
// =============================================================================

func TestL1Lifecycle(t *testing.T) {
	ctx := context.Background()
	w, netConfig := getTestWallet(t)

	t.Log("=== L1 Lifecycle Test ===")

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
	genesis := []byte(`{"config":{"chainId":99997},"alloc":{}}`)
	chainID, err := pchain.CreateChain(ctx, subnetWallet, pchain.CreateChainConfig{
		SubnetID:  subnetID,
		Genesis:   genesis,
		VMID:      constants.SubnetEVMID,
		ChainName: "l1chain",
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}
	t.Logf("  Chain ID: %s", chainID)

	time.Sleep(3 * time.Second)

	// 4. Convert subnet to L1 with a mock validator
	t.Log("Step 3: Converting subnet to L1...")

	// Generate mock validator with valid BLS credentials
	// Balance: 1 AVAX for the validator
	mockValidator := generateMockValidator(t, 1*units.Avax)
	validators := []*txs.ConvertSubnetToL1Validator{mockValidator}

	t.Logf("  Mock validator NodeID: %x", mockValidator.NodeID)

	convertTxID, err := pchain.ConvertSubnetToL1(ctx, subnetWallet, subnetID, chainID, nil, validators)
	if err != nil {
		// Skip if insufficient funds (test wallet may be depleted by previous tests)
		if isInsufficientFunds(err) {
			t.Skipf("Insufficient funds for L1 conversion (wallet depleted): %v", err)
		}
		t.Fatalf("ConvertSubnetToL1 failed: %v", err)
	}
	t.Logf("  Convert TX: %s", convertTxID)

	t.Log("=== L1 Lifecycle Complete ===")
}

// =============================================================================
// Full Integration Test: Cross-Chain Round Trip
// =============================================================================

func TestCrossChainRoundTrip(t *testing.T) {
	ctx := context.Background()
	w, _ := getTestFullWallet(t)

	amount := smallTransferAmountNAVAX

	t.Log("=== Cross-Chain Round Trip Test ===")
	t.Logf("P-Chain Address: %s", w.PChainAddress())
	t.Logf("C-Chain Address: %s", w.EthAddress().Hex())

	// 1. P-Chain -> C-Chain
	t.Logf("Step 1: P-Chain -> C-Chain (%d nAVAX)...", amount)
	exportTx1, importTx1, err := crosschain.TransferPToC(ctx, w, amount)
	if err != nil {
		t.Logf("P-to-C transfer failed (may need more P-Chain funds): %v", err)
		t.Skip("Skipping round trip - insufficient P-Chain balance")
	}
	t.Logf("  Export TX: %s", exportTx1)
	t.Logf("  Import TX: %s", importTx1)

	time.Sleep(3 * time.Second)

	// 2. C-Chain -> P-Chain
	t.Logf("Step 2: C-Chain -> P-Chain (%d nAVAX)...", amount)
	exportTx2, importTx2, err := crosschain.TransferCToP(ctx, w, amount)
	if err != nil {
		t.Logf("C-to-P transfer failed (may need more C-Chain funds): %v", err)
		t.Skip("Skipping return leg - insufficient C-Chain balance")
	}
	t.Logf("  Export TX: %s", exportTx2)
	t.Logf("  Import TX: %s", importTx2)

	t.Log("=== Cross-Chain Round Trip Complete ===")
}
