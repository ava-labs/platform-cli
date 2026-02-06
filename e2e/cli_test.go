//go:build clie2e

package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// runCLI executes the platform CLI with the given arguments.
func runCLI(t *testing.T, args ...string) (string, string, error) {
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

		// Add private key if available
		if envKey := os.Getenv(envPrivateKey); envKey != "" {
			fullArgs = append(fullArgs, "--private-key", envKey)
		} else if *networkFlag == "local" {
			fullArgs = append(fullArgs, "--key-name", "ewoq")
		}
	}

	binPath := cliBinaryPath
	if binPath == "" {
		// Fallback for direct execution without TestMain setup.
		binPath = "../platform"
	}
	cmd := exec.Command(binPath, fullArgs...)

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

	expected := []string{"add", "delegate"}
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

	expected := []string{"create", "transfer-ownership", "convert-l1"}
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

	expected := []string{"register-validator", "set-weight", "add-balance", "disable-validator"}
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
	_, stderr, err := runCLI(t, "validator", "add")
	if err == nil {
		t.Error("expected error when missing required args")
	}

	if !strings.Contains(stderr, "node-id") && !strings.Contains(stderr, "required") {
		t.Logf("stderr: %s", stderr)
	}
}

func TestCLIValidatorDelegateMissingArgs(t *testing.T) {
	_, stderr, err := runCLI(t, "validator", "delegate")
	if err == nil {
		t.Error("expected error when missing required args")
	}

	if !strings.Contains(stderr, "node-id") && !strings.Contains(stderr, "required") {
		t.Logf("stderr: %s", stderr)
	}
}

// =============================================================================
// CLI L1 Command Tests (Error Path - requires valid data)
// =============================================================================

func TestCLIL1AddBalanceMissingArgs(t *testing.T) {
	_, stderr, err := runCLI(t, "l1", "add-balance", "--balance", "1")
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
	_, stderr, err := runCLI(t, "subnet", "convert-l1")
	if err == nil {
		t.Error("expected error when missing required args")
	}

	if !strings.Contains(stderr, "subnet-id") {
		t.Logf("stderr: %s", stderr)
	}
}

func TestCLISubnetConvertL1EmptyValidators(t *testing.T) {
	_, stderr, err := runCLI(t,
		"subnet", "convert-l1",
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
	convertOut, stderr, err := runCLI(t, "subnet", "convert-l1",
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
