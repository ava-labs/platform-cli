package e2e

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const (
	envPrivateKey      = "PRIVATE_KEY"
	envRunNetworkTests = "RUN_E2E_NETWORK_TESTS"
)

var (
	networkFlag   = flag.String("network", "fuji", "Network to test against: local, fuji")
	localRPCURL   = "http://127.0.0.1:9650" // Default local network RPC URL
	cliBinaryPath string
)

// buildCLIBinaryForE2E builds a fresh CLI binary for this test run.
func buildCLIBinaryForE2E() (string, func(), error) {
	tempDir, err := os.MkdirTemp("", "platform-cli-e2e-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	binPath := filepath.Join(tempDir, "platform")
	cmd := exec.Command("go", "build", "-o", binPath, ".")
	cmd.Dir = ".."
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", nil, fmt.Errorf("failed to build CLI binary: %w\n%s", err, out)
	}

	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}
	return binPath, cleanup, nil
}

func requireNetworkE2ETestsEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv(envRunNetworkTests) != "1" {
		t.Skipf("network e2e tests are disabled; set %s=1 to run", envRunNetworkTests)
	}
}

func requireNetworkKeyForE2E(t *testing.T) {
	t.Helper()
	if os.Getenv(envPrivateKey) == "" && *networkFlag != "local" {
		t.Skipf("%s required for Fuji tests", envPrivateKey)
	}
}
