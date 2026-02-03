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

	// Build the CLI if it doesn't exist
	if _, err := os.Stat("../platform"); os.IsNotExist(err) {
		cmd := exec.Command("go", "build", "-o", "platform", ".")
		cmd.Dir = ".."
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("failed to build CLI: %v\n%s", err, out)
		}
	}

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
		// Add network flag
		fullArgs = append([]string{"--network", *networkFlag}, args...)

		// Add private key if available
		if envKey := os.Getenv("PRIVATE_KEY"); envKey != "" {
			fullArgs = append(fullArgs, "--private-key", envKey)
		} else if *networkFlag == "local" {
			fullArgs = append(fullArgs, "--key-name", "ewoq")
		}
	}

	cmd := exec.Command("../platform", fullArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
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
	if os.Getenv("PRIVATE_KEY") == "" && *networkFlag != "local" {
		t.Skip("PRIVATE_KEY required for Fuji")
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
	if os.Getenv("PRIVATE_KEY") == "" && *networkFlag != "local" {
		t.Skip("PRIVATE_KEY required for Fuji")
	}

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
	if os.Getenv("PRIVATE_KEY") == "" && *networkFlag != "local" {
		t.Skip("PRIVATE_KEY required for Fuji")
	}

	stdout, stderr, err := runCLI(t, "transfer", "p-to-c", "--amount", "0.01")
	if err != nil {
		t.Fatalf("transfer p-to-c failed: %v\nstderr: %s", err, stderr)
	}

	if !strings.Contains(stdout, "Export TX ID:") || !strings.Contains(stdout, "Import TX ID:") {
		t.Error("output missing TX IDs")
	}

	t.Logf("Output:\n%s", stdout)
}

func TestCLITransferCToP(t *testing.T) {
	if os.Getenv("PRIVATE_KEY") == "" && *networkFlag != "local" {
		t.Skip("PRIVATE_KEY required for Fuji")
	}

	stdout, stderr, err := runCLI(t, "transfer", "c-to-p", "--amount", "0.01")
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
	if os.Getenv("PRIVATE_KEY") == "" && *networkFlag != "local" {
		t.Skip("PRIVATE_KEY required for Fuji")
	}

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
	_, stderr, err := runCLI(t, "l1", "add-balance")
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
