//go:build clie2e || networke2e

package e2e

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/vms/platformvm"
	"github.com/ava-labs/platform-cli/pkg/network"
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

	binPath := filepath.Join(tempDir, "platform-cli")
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

// Retry transient RPC rate limits from shared public endpoints.
const (
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

// waitForSubnetOwners polls GetSubnet until the on-chain owner matches the
// expected control keys (order-insensitive) and threshold. Public API
// endpoints are load balanced, so the queried node may briefly lag the node
// that accepted the transfer tx.
func waitForSubnetOwners(t *testing.T, ctx context.Context, client *platformvm.Client, subnetID ids.ID, wantOwners []ids.ShortID, wantThreshold uint32) {
	t.Helper()

	// The public API is load balanced and individual nodes can serve state
	// that lags an accepted tx by tens of seconds, so poll generously.
	const (
		pollAttempts = 20
		pollDelay    = 6 * time.Second
	)

	var subnet platformvm.GetSubnetClientResponse
	for attempt := 1; attempt <= pollAttempts; attempt++ {
		var err error
		subnet, err = retryRateLimitedOperation(t, "GetSubnet", func() (platformvm.GetSubnetClientResponse, error) {
			return client.GetSubnet(ctx, subnetID)
		})
		if err != nil {
			t.Fatalf("GetSubnet failed: %v", err)
		}
		if subnetOwnersMatch(subnet, wantOwners, wantThreshold) {
			return
		}
		t.Logf("owner set not yet visible (attempt %d/%d): keys=%v threshold=%d", attempt, pollAttempts, subnet.ControlKeys, subnet.Threshold)
		time.Sleep(pollDelay)
	}
	t.Fatalf("subnet owner = keys %v threshold %d, want keys %v threshold %d", subnet.ControlKeys, subnet.Threshold, wantOwners, wantThreshold)
}

func subnetOwnersMatch(subnet platformvm.GetSubnetClientResponse, wantOwners []ids.ShortID, wantThreshold uint32) bool {
	if subnet.Threshold != wantThreshold || len(subnet.ControlKeys) != len(wantOwners) {
		return false
	}
	got := make(map[ids.ShortID]bool, len(subnet.ControlKeys))
	for _, addr := range subnet.ControlKeys {
		got[addr] = true
	}
	for _, addr := range wantOwners {
		if !got[addr] {
			return false
		}
	}
	return true
}
