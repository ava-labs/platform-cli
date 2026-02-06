//go:build clie2e

package e2e

import (
	"flag"
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	flag.Parse()

	binPath, cleanup, err := buildCLIBinaryForE2E()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to set up e2e CLI binary: %v\n", err)
		os.Exit(1)
	}
	cliBinaryPath = binPath

	code := m.Run()
	cleanup()
	os.Exit(code)
}
