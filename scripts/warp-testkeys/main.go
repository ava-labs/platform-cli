// Command warp-testkeys creates throwaway L1 validator identities for the
// warp rehearsal: a random node ID and a BLS key per validator. It writes
// each key to <dir>/<i>/signer.key and prints one line per validator:
//
//	<node ID> <BLS public key hex> <BLS proof of possession hex>
//
// Never use these keys outside a test network.
package main

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto/bls/signer/localsigner"
	"github.com/ava-labs/avalanchego/vms/platformvm/signer"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "warp-testkeys:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: warp-testkeys <dir> <count>")
	}
	dir := args[0]
	count, err := strconv.Atoi(args[1])
	if err != nil || count <= 0 {
		return fmt.Errorf("invalid count %q", args[1])
	}

	for i := range count {
		var nodeID ids.NodeID
		if _, err := rand.Read(nodeID[:]); err != nil {
			return fmt.Errorf("failed to generate node ID: %w", err)
		}
		s, err := localsigner.New()
		if err != nil {
			return fmt.Errorf("failed to generate BLS key: %w", err)
		}
		if err := s.ToFile(filepath.Join(dir, strconv.Itoa(i), "signer.key")); err != nil {
			return fmt.Errorf("failed to write BLS key: %w", err)
		}
		pop, err := signer.NewProofOfPossession(s)
		if err != nil {
			return fmt.Errorf("failed to build proof of possession: %w", err)
		}
		fmt.Printf("%s 0x%x 0x%x\n", nodeID, pop.PublicKey, pop.ProofOfPossession)
	}
	return nil
}
