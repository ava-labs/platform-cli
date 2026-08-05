//go:build clie2e

package e2e

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/crypto/secp256k1"
	"github.com/ava-labs/avalanchego/utils/formatting/address"
	"github.com/ava-labs/avalanchego/vms/platformvm"
)

// runCLI executes the platform CLI with the given arguments.
func runCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	return runCLIWithEnvKey(t, os.Getenv(envPrivateKey), args...)
}

// runCLIWithEnvKey executes the platform CLI with an explicit
// AVALANCHE_PRIVATE_KEY value (comma-separated for multi-key signing).
func runCLIWithEnvKey(t *testing.T, envKeyValue string, args ...string) (string, string, error) {
	t.Helper()

	// For help commands, don't add extra flags
	isHelpCmd := false
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			isHelpCmd = true
			break
		}
	}

	var fullArgs []string
	var cliPrivateKeyEnv string
	if isHelpCmd {
		fullArgs = args
	} else {
		// Add network/RPC flag
		if *networkFlag == "local" {
			// Use --rpc-url for local network instead of --network local
			fullArgs = append([]string{"--rpc-url", "http://127.0.0.1:9650"}, args...)
		} else {
			fullArgs = append([]string{"--network", *networkFlag}, args...)
		}

		// Pass private key via environment to avoid exposing it in process args.
		if envKeyValue != "" {
			cliPrivateKeyEnv = envKeyValue
		} else if *networkFlag == "local" {
			fullArgs = append(fullArgs, "--key-name", "ewoq")
		}
	}

	binPath := cliBinaryPath
	if binPath == "" {
		// Fallback for direct execution without TestMain setup.
		binPath = "../platform-cli"
	}
	cmd := exec.Command(binPath, fullArgs...)
	cmd.Env = os.Environ()
	if cliPrivateKeyEnv != "" {
		cmd.Env = append(cmd.Env, "AVALANCHE_PRIVATE_KEY="+cliPrivateKeyEnv)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func requireStateChangingCLITest(t *testing.T) {
	t.Helper()
	requireNetworkE2ETestsEnabled(t)
	requireNetworkKeyForE2E(t)
}

// =============================================================================
// CLI Help Tests
// =============================================================================

func TestCLIHelp(t *testing.T) {
	stdout, _, err := runCLI(t, "--help")
	if err != nil {
		t.Fatalf("CLI help failed: %v", err)
	}

	// Check that help contains expected commands
	expectedCommands := []string{"wallet", "transfer", "validator", "subnet", "l1", "chain", "keys", "node"}
	for _, cmd := range expectedCommands {
		if !strings.Contains(stdout, cmd) {
			t.Errorf("help output missing command: %s", cmd)
		}
	}

	t.Logf("Help output:\n%s", stdout)
}

func TestCLIWalletHelp(t *testing.T) {
	stdout, _, err := runCLI(t, "wallet", "--help")
	if err != nil {
		t.Fatalf("wallet help failed: %v", err)
	}

	if !strings.Contains(stdout, "address") || !strings.Contains(stdout, "balance") {
		t.Error("wallet help missing expected subcommands")
	}

	t.Logf("Wallet help:\n%s", stdout)
}

func TestCLITransferHelp(t *testing.T) {
	stdout, _, err := runCLI(t, "transfer", "--help")
	if err != nil {
		t.Fatalf("transfer help failed: %v", err)
	}

	expected := []string{"send", "p-to-c", "c-to-p", "export", "import"}
	for _, cmd := range expected {
		if !strings.Contains(stdout, cmd) {
			t.Errorf("transfer help missing subcommand: %s", cmd)
		}
	}

	t.Logf("Transfer help:\n%s", stdout)
}

func TestCLIValidatorHelp(t *testing.T) {
	stdout, _, err := runCLI(t, "validator", "--help")
	if err != nil {
		t.Fatalf("validator help failed: %v", err)
	}

	expected := []string{"add-permissionless", "add-permissionless-delegator", "add-auto-renewed", "set-auto-renewed-config"}
	for _, cmd := range expected {
		if !strings.Contains(stdout, cmd) {
			t.Errorf("validator help missing subcommand: %s", cmd)
		}
	}

	t.Logf("Validator help:\n%s", stdout)
}

func TestCLISubnetHelp(t *testing.T) {
	stdout, _, err := runCLI(t, "subnet", "--help")
	if err != nil {
		t.Fatalf("subnet help failed: %v", err)
	}

	expected := []string{"create", "transfer-ownership", "convert-to-l1", "add-validator"}
	for _, cmd := range expected {
		if !strings.Contains(stdout, cmd) {
			t.Errorf("subnet help missing subcommand: %s", cmd)
		}
	}

	t.Logf("Subnet help:\n%s", stdout)
}

func TestCLIL1Help(t *testing.T) {
	stdout, _, err := runCLI(t, "l1", "--help")
	if err != nil {
		t.Fatalf("l1 help failed: %v", err)
	}

	expected := []string{"register-validator", "set-validator-weight", "increase-validator-balance", "disable-validator"}
	for _, cmd := range expected {
		if !strings.Contains(stdout, cmd) {
			t.Errorf("l1 help missing subcommand: %s", cmd)
		}
	}

	t.Logf("L1 help:\n%s", stdout)
}

// =============================================================================
// CLI Wallet Command Tests
// =============================================================================

func TestCLIWalletAddress(t *testing.T) {
	if os.Getenv(envPrivateKey) == "" && *networkFlag != "local" {
		t.Skipf("%s required for Fuji", envPrivateKey)
	}

	stdout, stderr, err := runCLI(t, "wallet", "address")
	if err != nil {
		t.Fatalf("wallet address failed: %v\nstderr: %s", err, stderr)
	}

	if !strings.Contains(stdout, "P-Chain Address:") {
		t.Error("output missing P-Chain address")
	}
	if !strings.Contains(stdout, "EVM Address:") {
		t.Error("output missing EVM address")
	}

	t.Logf("Output:\n%s", stdout)
}

// =============================================================================
// CLI Transfer Command Tests
// =============================================================================

func TestCLITransferSend(t *testing.T) {
	requireStateChangingCLITest(t)

	// First get our address
	addrOut, _, err := runCLI(t, "wallet", "address")
	if err != nil {
		t.Fatalf("failed to get address: %v", err)
	}

	// Parse P-Chain address from output
	lines := strings.Split(addrOut, "\n")
	var pAddr string
	for _, line := range lines {
		if strings.HasPrefix(line, "P-Chain Address:") {
			pAddr = strings.TrimSpace(strings.TrimPrefix(line, "P-Chain Address:"))
			break
		}
	}

	if pAddr == "" {
		t.Fatal("could not parse P-Chain address")
	}

	// Send to self
	stdout, stderr, err := runCLI(t, "transfer", "send", "--to", pAddr, "--amount", "0.001")
	if err != nil {
		t.Fatalf("transfer send failed: %v\nstderr: %s", err, stderr)
	}

	if !strings.Contains(stdout, "TX ID:") {
		t.Error("output missing TX ID")
	}

	t.Logf("Output:\n%s", stdout)
}

func TestCLITransferPToC(t *testing.T) {
	requireStateChangingCLITest(t)

	stdout, stderr, err := runCLI(t, "transfer", "p-to-c", "--amount", "0.001")
	if err != nil {
		t.Fatalf("transfer p-to-c failed: %v\nstderr: %s", err, stderr)
	}

	if !strings.Contains(stdout, "Export TX ID:") || !strings.Contains(stdout, "Import TX ID:") {
		t.Error("output missing TX IDs")
	}

	t.Logf("Output:\n%s", stdout)
}

func TestCLITransferCToP(t *testing.T) {
	requireStateChangingCLITest(t)

	stdout, stderr, err := runCLI(t, "transfer", "c-to-p", "--amount", "0.001")
	if err != nil {
		t.Fatalf("transfer c-to-p failed: %v\nstderr: %s", err, stderr)
	}

	if !strings.Contains(stdout, "Export TX ID:") || !strings.Contains(stdout, "Import TX ID:") {
		t.Error("output missing TX IDs")
	}

	t.Logf("Output:\n%s", stdout)
}

// =============================================================================
// CLI Subnet Command Tests
// =============================================================================

func TestCLISubnetCreate(t *testing.T) {
	requireStateChangingCLITest(t)

	stdout, stderr, err := runCLI(t, "subnet", "create")
	if err != nil {
		t.Fatalf("subnet create failed: %v\nstderr: %s", err, stderr)
	}

	if !strings.Contains(stdout, "Subnet ID:") {
		t.Error("output missing Subnet ID")
	}

	t.Logf("Output:\n%s", stdout)
}

// =============================================================================
// CLI Validator Command Tests (Error Path - requires stake)
// =============================================================================

func TestCLIValidatorAddMissingArgs(t *testing.T) {
	_, stderr, err := runCLI(t, "validator", "add-permissionless")
	if err == nil {
		t.Error("expected error when missing required args")
	}

	if !strings.Contains(stderr, "node-id") && !strings.Contains(stderr, "required") {
		t.Logf("stderr: %s", stderr)
	}
}

func TestCLIValidatorDelegateMissingArgs(t *testing.T) {
	_, stderr, err := runCLI(t, "validator", "add-permissionless-delegator")
	if err == nil {
		t.Error("expected error when missing required args")
	}

	if !strings.Contains(stderr, "node-id") && !strings.Contains(stderr, "required") {
		t.Logf("stderr: %s", stderr)
	}
}

func TestCLIValidatorAddAutoRenewedHelp(t *testing.T) {
	stdout, _, err := runCLI(t, "validator", "add-auto-renewed", "--help")
	if err != nil {
		t.Fatalf("add-auto-renewed help failed: %v", err)
	}

	expected := []string{"node-id", "stake", "period", "auto-compound", "owner-address"}
	for _, flag := range expected {
		if !strings.Contains(stdout, flag) {
			t.Errorf("add-auto-renewed help missing flag: %s", flag)
		}
	}
}

func TestCLIValidatorSetAutoConfigHelp(t *testing.T) {
	stdout, _, err := runCLI(t, "validator", "set-auto-renewed-config", "--help")
	if err != nil {
		t.Fatalf("set-auto-renewed-config help failed: %v", err)
	}

	expected := []string{"tx-id", "node-id", "period", "auto-compound"}
	for _, flag := range expected {
		if !strings.Contains(stdout, flag) {
			t.Errorf("set-auto-renewed-config help missing flag: %s", flag)
		}
	}
}

func TestCLIValidatorAddAutoRenewedMissingArgs(t *testing.T) {
	_, stderr, err := runCLI(t, "validator", "add-auto-renewed")
	if err == nil {
		t.Error("expected error when missing required args")
	}

	if !strings.Contains(stderr, "stake") && !strings.Contains(stderr, "required") {
		t.Logf("stderr: %s", stderr)
	}
}

func TestCLIValidatorSetAutoConfigMissingArgs(t *testing.T) {
	_, stderr, err := runCLI(t, "validator", "set-auto-renewed-config")
	if err == nil {
		t.Error("expected error when missing required args")
	}

	if !strings.Contains(stderr, "tx-id") && !strings.Contains(stderr, "required") {
		t.Logf("stderr: %s", stderr)
	}
}

// =============================================================================
// CLI L1 Command Tests (Error Path - requires valid data)
// =============================================================================

func TestCLIL1AddBalanceMissingArgs(t *testing.T) {
	_, stderr, err := runCLI(t, "l1", "increase-validator-balance", "--balance", "1")
	if err == nil {
		t.Error("expected error when missing required args")
	}

	if !strings.Contains(stderr, "validation-id") {
		t.Logf("stderr: %s", stderr)
	}
}

func TestCLIL1DisableValidatorMissingArgs(t *testing.T) {
	_, stderr, err := runCLI(t, "l1", "disable-validator")
	if err == nil {
		t.Error("expected error when missing required args")
	}

	if !strings.Contains(stderr, "validation-id") {
		t.Logf("stderr: %s", stderr)
	}
}

// =============================================================================
// CLI Chain Command Tests (Error Path - requires subnet)
// =============================================================================

func TestCLIChainCreateMissingArgs(t *testing.T) {
	_, stderr, err := runCLI(t, "chain", "create")
	if err == nil {
		t.Error("expected error when missing required args")
	}

	if !strings.Contains(stderr, "subnet-id") {
		t.Logf("stderr: %s", stderr)
	}
}

func TestCLISubnetConvertL1MissingArgs(t *testing.T) {
	_, stderr, err := runCLI(t, "subnet", "convert-to-l1")
	if err == nil {
		t.Error("expected error when missing required args")
	}

	if !strings.Contains(stderr, "subnet-id") {
		t.Logf("stderr: %s", stderr)
	}
}

func TestCLISubnetTransferOwnershipMissingArgs(t *testing.T) {
	_, stderr, err := runCLI(t, "subnet", "transfer-ownership")
	if err == nil {
		t.Error("expected error when missing required args")
	}

	if !strings.Contains(stderr, "subnet-id") {
		t.Logf("stderr: %s", stderr)
	}
}

func TestCLISubnetTransferOwnershipInvalidAddress(t *testing.T) {
	_, stderr, err := runCLI(t,
		"subnet", "transfer-ownership",
		"--subnet-id", "11111111111111111111111111111111LpoYY",
		"--new-owner", "not-an-address",
	)
	if err == nil {
		t.Error("expected error for invalid owner address")
	}

	if !strings.Contains(stderr, "invalid owner address") {
		t.Errorf("expected invalid owner address error, got stderr: %s", stderr)
	}
}

// TestCLISubnetTransferOwnershipInvalidThreshold verifies that a threshold
// larger than the owner set is rejected before any wallet or network access.
func TestCLISubnetTransferOwnershipInvalidThreshold(t *testing.T) {
	_, stderr, err := runCLI(t,
		"subnet", "transfer-ownership",
		"--subnet-id", "11111111111111111111111111111111LpoYY",
		"--new-owner", "P-fuji18jma8ppw3nhx5r4ap8clazz0dps7rv5u6wmu4t",
		"--threshold", "2",
	)
	if err == nil {
		t.Error("expected error for threshold exceeding owner count")
	}

	if !strings.Contains(stderr, "threshold") {
		t.Errorf("expected threshold error, got stderr: %s", stderr)
	}
}

func TestCLISubnetTransferOwnershipDuplicateOwners(t *testing.T) {
	addr := "P-fuji18jma8ppw3nhx5r4ap8clazz0dps7rv5u6wmu4t"
	_, stderr, err := runCLI(t,
		"subnet", "transfer-ownership",
		"--subnet-id", "11111111111111111111111111111111LpoYY",
		"--new-owner", addr,
		"--new-owner", addr,
	)
	if err == nil {
		t.Error("expected error for duplicate owner addresses")
	}

	if !strings.Contains(stderr, "duplicate") {
		t.Errorf("expected duplicate owner error, got stderr: %s", stderr)
	}
}

// TestCLISubnetTransferOwnershipMultisigSigning drives the CLI binary through
// a multisig signing round trip:
//
//  1. create a subnet (1-of-1)
//  2. transfer it to a 2-of-2 multisig (funded key + generated key)
//  3. verify a single key can no longer move it
//  4. move it back to 1-of-1 with both keys via comma-separated
//     AVALANCHE_PRIVATE_KEY
func TestCLISubnetTransferOwnershipMultisigSigning(t *testing.T) {
	requireStateChangingCLITest(t)
	payerEnvKey := os.Getenv(envPrivateKey)
	if payerEnvKey == "" {
		t.Skipf("%s required: this test signs with explicit env keys", envPrivateKey)
	}

	ctx := context.Background()
	netConfig := getNetworkConfig(t, ctx)
	hrp := constants.GetHRP(netConfig.NetworkID)

	// 1. Create subnet
	stdout, stderr, err := runCLI(t, "subnet", "create")
	if err != nil {
		t.Fatalf("subnet create failed: %v\nstderr: %s", err, stderr)
	}
	subnetIDStr := valueAfterPrefix(t, stdout, "Subnet ID: ")
	payerAddrStr := valueAfterPrefix(t, stdout, "Owner: ")
	t.Logf("Created subnet %s owned by %s", subnetIDStr, payerAddrStr)

	subnetID, err := ids.FromString(subnetIDStr)
	if err != nil {
		t.Fatalf("failed to parse subnet ID %q: %v", subnetIDStr, err)
	}
	payerAddr, err := address.ParseToID(payerAddrStr)
	if err != nil {
		t.Fatalf("failed to parse owner address %q: %v", payerAddrStr, err)
	}

	// 2. Generate the second owner key
	key2, err := secp256k1.NewPrivateKey()
	if err != nil {
		t.Fatalf("failed to generate second owner key: %v", err)
	}
	key2Hex := "0x" + hex.EncodeToString(key2.Bytes())
	addr2Str, err := address.Format("P", hrp, key2.Address().Bytes())
	if err != nil {
		t.Fatalf("failed to format second owner address: %v", err)
	}

	time.Sleep(3 * time.Second)

	// 3. Transfer to a 2-of-2 multisig, authorized by the single current owner
	_, stderr, err = runCLI(t,
		"subnet", "transfer-ownership",
		"--subnet-id", subnetIDStr,
		"--new-owner", payerAddrStr,
		"--new-owner", addr2Str,
		"--threshold", "2",
	)
	if err != nil {
		t.Fatalf("transfer to 2-of-2 failed: %v\nstderr: %s", err, stderr)
	}

	client := platformvm.NewClient(netConfig.RPCURL)
	waitForSubnetOwners(t, ctx, client, subnetID, []ids.ShortID{payerAddr, key2.Address()}, 2)

	// 4. Negative: a single key cannot meet the 2-of-2 threshold
	_, stderr, err = runCLI(t,
		"subnet", "transfer-ownership",
		"--subnet-id", subnetIDStr,
		"--new-owner", payerAddrStr,
	)
	if err == nil {
		t.Fatal("expected single-key transfer out of 2-of-2 to fail, but it succeeded")
	}
	t.Logf("single-key transfer rejected as expected: %s", strings.TrimSpace(stderr))

	// 5. Move back to 1-of-1 signing with both keys
	_, stderr, err = runCLIWithEnvKey(t, payerEnvKey+","+key2Hex,
		"subnet", "transfer-ownership",
		"--subnet-id", subnetIDStr,
		"--new-owner", payerAddrStr,
	)
	if err != nil {
		t.Fatalf("multi-key transfer back to 1-of-1 failed: %v\nstderr: %s", err, stderr)
	}
	waitForSubnetOwners(t, ctx, client, subnetID, []ids.ShortID{payerAddr}, 1)

	t.Log("=== CLI Multisig Signing Round Trip Complete ===")
}

// TestCLISubnetTransferOwnershipPartialSign simulates a 2-of-2 owner whose
// keys live on two machines, coordinating through a tx file:
//
//  1. create a subnet and move it to a 2-of-2 multisig (payer + generated key)
//  2. "machine A" (payer key only) builds and partially signs a transfer back
//     to 1-of-1 with --output-tx-path
//  3. committing the half-signed tx is refused
//  4. "machine B" (generated key ONLY, no payer key) completes it via tx sign
//  5. tx commit submits it; the owner change is verified on-chain
func TestCLISubnetTransferOwnershipPartialSign(t *testing.T) {
	requireStateChangingCLITest(t)
	payerEnvKey := os.Getenv(envPrivateKey)
	if payerEnvKey == "" {
		t.Skipf("%s required: this test signs with explicit env keys", envPrivateKey)
	}

	ctx := context.Background()
	netConfig := getNetworkConfig(t, ctx)
	hrp := constants.GetHRP(netConfig.NetworkID)

	// 1. Create subnet and move it to a 2-of-2 multisig
	stdout, stderr, err := runCLI(t, "subnet", "create")
	if err != nil {
		t.Fatalf("subnet create failed: %v\nstderr: %s", err, stderr)
	}
	subnetIDStr := valueAfterPrefix(t, stdout, "Subnet ID: ")
	payerAddrStr := valueAfterPrefix(t, stdout, "Owner: ")
	subnetID, err := ids.FromString(subnetIDStr)
	if err != nil {
		t.Fatalf("failed to parse subnet ID %q: %v", subnetIDStr, err)
	}
	payerAddr, err := address.ParseToID(payerAddrStr)
	if err != nil {
		t.Fatalf("failed to parse owner address %q: %v", payerAddrStr, err)
	}

	key2, err := secp256k1.NewPrivateKey()
	if err != nil {
		t.Fatalf("failed to generate second owner key: %v", err)
	}
	key2Hex := "0x" + hex.EncodeToString(key2.Bytes())
	addr2Str, err := address.Format("P", hrp, key2.Address().Bytes())
	if err != nil {
		t.Fatalf("failed to format second owner address: %v", err)
	}

	time.Sleep(3 * time.Second)

	_, stderr, err = runCLI(t,
		"subnet", "transfer-ownership",
		"--subnet-id", subnetIDStr,
		"--new-owner", payerAddrStr,
		"--new-owner", addr2Str,
		"--threshold", "2",
	)
	if err != nil {
		t.Fatalf("transfer to 2-of-2 failed: %v\nstderr: %s", err, stderr)
	}
	client := platformvm.NewClient(netConfig.RPCURL)
	waitForSubnetOwners(t, ctx, client, subnetID, []ids.ShortID{payerAddr, key2.Address()}, 2)

	// 2. Machine A: build + partially sign the transfer back to 1-of-1
	txFile := filepath.Join(t.TempDir(), "transfer.json")
	stdout, stderr, err = runCLI(t,
		"subnet", "transfer-ownership",
		"--subnet-id", subnetIDStr,
		"--new-owner", payerAddrStr,
		"--output-tx-path", txFile,
	)
	if err != nil {
		t.Fatalf("partial-sign transfer failed: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Signatures: 1 of 2") {
		t.Fatalf("expected 1 of 2 signatures after machine A, got output:\n%s", stdout)
	}

	// 3. Committing a half-signed tx must be refused
	_, stderr, err = runCLI(t, "tx", "commit", "--tx-path", txFile)
	if err == nil {
		t.Fatal("expected commit of half-signed tx to fail, but it succeeded")
	}
	if !strings.Contains(stderr, "not fully signed") {
		t.Fatalf("expected 'not fully signed' error, got stderr: %s", stderr)
	}

	// 4. Machine B: only key2, no payer key
	stdout, stderr, err = runCLIWithEnvKey(t, key2Hex, "tx", "sign", "--tx-path", txFile)
	if err != nil {
		t.Fatalf("tx sign on machine B failed: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Fully signed") {
		t.Fatalf("expected fully signed after machine B, got output:\n%s", stdout)
	}

	// 5. Submit and verify on-chain
	stdout, stderr, err = runCLI(t, "tx", "commit", "--tx-path", txFile)
	if err != nil {
		t.Fatalf("tx commit failed: %v\nstderr: %s", err, stderr)
	}
	t.Logf("commit output:\n%s", strings.TrimSpace(stdout))
	waitForSubnetOwners(t, ctx, client, subnetID, []ids.ShortID{payerAddr}, 1)

	t.Log("=== CLI Partial Signing Round Trip Complete ===")
}

// valueAfterPrefix returns the trimmed remainder of the first stdout line
// starting with prefix.
func valueAfterPrefix(t *testing.T, stdout, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	t.Fatalf("output missing %q line:\n%s", prefix, stdout)
	return ""
}

// TestCLIRemovedOldNamesRejected verifies the v2.0.0 hard cutover: the old
// command names were removed (no aliases) and are now rejected as unknown.
func TestCLIRemovedOldNamesRejected(t *testing.T) {
	cases := [][]string{
		{"validator", "add"},
		{"validator", "delegate"},
		{"subnet", "convert-l1"},
		{"l1", "set-weight"},
		{"l1", "add-balance"},
	}
	for _, args := range cases {
		_, stderr, err := runCLI(t, args...)
		if err == nil {
			t.Errorf("%v: expected error for removed command name", args)
		}
		if !strings.Contains(stderr, "unknown command") {
			t.Errorf("%v: expected \"unknown command\", got stderr: %s", args, stderr)
		}
	}
}

// TestCLIUnknownSubcommandRejected verifies the root command and every command
// group reject an unknown subcommand with an error instead of silently printing
// help and exiting 0 (the footgun requireSubcommand closes).
func TestCLIUnknownSubcommandRejected(t *testing.T) {
	cases := [][]string{
		{"definitely-not-a-command"}, // root
		{"chain", "bogus"},
		{"transfer", "bogus"},
		{"keys", "bogus"},
		{"wallet", "bogus"},
		{"node", "bogus"},
	}
	for _, args := range cases {
		_, stderr, err := runCLI(t, args...)
		if err == nil {
			t.Errorf("%v: expected error for unknown subcommand", args)
		}
		if !strings.Contains(stderr, "unknown command") {
			t.Errorf("%v: expected \"unknown command\", got stderr: %s", args, stderr)
		}
	}
}

func TestCLISubnetConvertL1EmptyValidators(t *testing.T) {
	_, stderr, err := runCLI(t,
		"subnet", "convert-to-l1",
		"--subnet-id", "2ebCneQ9z9v56N6sryhU6P8L3s1f6BDoed6ox2q6iM8Qv7w6s",
		"--chain-id", "2ebCneQ9z9v56N6sryhU6P8L3s1f6BDoed6ox2q6iM8Qv7w6s",
		"--validators", ", , ,",
	)
	if err == nil {
		t.Error("expected error when --validators has no valid addresses")
	}

	if !strings.Contains(stderr, "--validators must include at least one non-empty validator address") {
		t.Logf("stderr: %s", stderr)
	}
}

func TestCLISubnetAddValidatorMissingArgs(t *testing.T) {
	_, stderr, err := runCLI(t, "subnet", "add-validator")
	if err == nil {
		t.Error("expected error when missing required args")
	}

	if !strings.Contains(stderr, "subnet-id") {
		t.Errorf("expected error to mention subnet-id, got stderr: %s", stderr)
	}
}

// =============================================================================
// CLI Full L1 Lifecycle Test
// =============================================================================

func TestCLIL1Lifecycle(t *testing.T) {
	requireStateChangingCLITest(t)

	t.Log("=== L1 Lifecycle CLI Test ===")

	// Step 1: Create subnet
	t.Log("Step 1: Creating subnet...")
	subnetOut, stderr, err := runCLI(t, "subnet", "create")
	if err != nil {
		t.Fatalf("subnet create failed: %v\nstderr: %s", err, stderr)
	}

	// Parse subnet ID from output
	var subnetID string
	for _, line := range strings.Split(subnetOut, "\n") {
		if strings.HasPrefix(line, "Subnet ID:") {
			subnetID = strings.TrimSpace(strings.TrimPrefix(line, "Subnet ID:"))
			break
		}
	}
	if subnetID == "" {
		t.Fatal("could not parse Subnet ID from output")
	}
	t.Logf("  Subnet ID: %s", subnetID)

	// Step 2: Create genesis file
	genesisFile, err := os.CreateTemp("", "genesis-*.json")
	if err != nil {
		t.Fatalf("failed to create temp genesis file: %v", err)
	}
	defer os.Remove(genesisFile.Name())

	genesis := `{"config":{"chainId":99998},"alloc":{}}`
	if _, err := genesisFile.WriteString(genesis); err != nil {
		t.Fatalf("failed to write genesis: %v", err)
	}
	genesisFile.Close()

	// Step 3: Create chain on subnet
	t.Log("Step 2: Creating chain on subnet...")
	chainOut, stderr, err := runCLI(t, "chain", "create",
		"--subnet-id", subnetID,
		"--genesis", genesisFile.Name(),
		"--name", "l1testchain")
	if err != nil {
		t.Fatalf("chain create failed: %v\nstderr: %s", err, stderr)
	}

	// Parse chain ID from output
	var chainID string
	for _, line := range strings.Split(chainOut, "\n") {
		if strings.HasPrefix(line, "Chain ID:") {
			chainID = strings.TrimSpace(strings.TrimPrefix(line, "Chain ID:"))
			break
		}
	}
	if chainID == "" {
		t.Fatal("could not parse Chain ID from output")
	}
	t.Logf("  Chain ID: %s", chainID)

	// Step 4: Convert subnet to L1 using mock validator
	t.Log("Step 3: Converting subnet to L1...")
	convertOut, stderr, err := runCLI(t, "subnet", "convert-to-l1",
		"--subnet-id", subnetID,
		"--chain-id", chainID,
		"--mock-validator")
	if err != nil {
		// Skip if insufficient funds (test wallet may be depleted by previous tests)
		if strings.Contains(stderr, "insufficient funds") {
			t.Skipf("Insufficient funds for L1 conversion (wallet depleted): %s", stderr)
		}
		t.Fatalf("subnet convert-l1 failed: %v\nstderr: %s", err, stderr)
	}

	if !strings.Contains(convertOut, "TX ID:") {
		t.Error("output missing conversion TX ID")
	}
	t.Logf("Output:\n%s", convertOut)

	t.Log("=== L1 Lifecycle Complete ===")
}
