//go:build clie2e

package e2e

import (
	"os"
	"strings"
	"testing"

	"github.com/ava-labs/avalanchego/ids"
)

// outputField returns the value of the first "<prefix> <value>" line in out.
func outputField(t *testing.T, out, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), prefix); ok {
			return strings.TrimSpace(value)
		}
	}
	t.Fatalf("output has no %q line:\n%s", prefix, out)
	return ""
}

// TestCLIL1DisableValidator converts a subnet with one validator and disables
// that validator with the key that did the conversion, which is the default
// deactivation owner.
func TestCLIL1DisableValidator(t *testing.T) {
	requireStateChangingCLITest(t)

	out, stderr, err := runCLI(t, "subnet", "create")
	if err != nil {
		t.Fatalf("subnet create failed: %v\nstderr: %s", err, stderr)
	}
	subnetID, err := ids.FromString(outputField(t, out, "Subnet ID:"))
	if err != nil {
		t.Fatalf("invalid subnet ID: %v", err)
	}

	genesis, err := os.CreateTemp(t.TempDir(), "genesis-*.json")
	if err != nil {
		t.Fatalf("failed to create genesis file: %v", err)
	}
	if _, err := genesis.WriteString(`{"config":{"chainId":99997},"alloc":{}}`); err != nil {
		t.Fatalf("failed to write genesis file: %v", err)
	}
	genesis.Close()

	out, stderr, err = runCLI(t, "chain", "create",
		"--subnet-id", subnetID.String(),
		"--genesis", genesis.Name(),
		"--name", "disabletest")
	if err != nil {
		t.Fatalf("chain create failed: %v\nstderr: %s", err, stderr)
	}
	chainID := outputField(t, out, "Chain ID:")

	_, stderr, err = runCLI(t, "subnet", "convert-to-l1",
		"--subnet-id", subnetID.String(),
		"--chain-id", chainID,
		"--mock-validator")
	if err != nil {
		t.Fatalf("subnet convert-to-l1 failed: %v\nstderr: %s", err, stderr)
	}

	// The P-Chain derives the validation ID of the i-th conversion validator
	// as subnetID.Append(i).
	validationID := subnetID.Append(0).String()
	out, stderr, err = runCLI(t, "l1", "disable-validator", "--validation-id", validationID)
	if err != nil {
		t.Fatalf("l1 disable-validator failed: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(out, "Disable L1 Validator TX:") {
		t.Fatalf("output has no TX ID:\n%s", out)
	}

	_, stderr, err = runCLI(t, "l1", "disable-validator", "--validation-id", validationID)
	if err == nil {
		t.Fatal("second l1 disable-validator succeeded, want an error for an inactive validator")
	}
	if !strings.Contains(stderr, "already inactive") {
		t.Fatalf("stderr = %q, want an \"already inactive\" error", stderr)
	}
}
