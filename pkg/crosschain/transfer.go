// Package crosschain provides cross-chain transfer utilities for Avalanche.
package crosschain

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
	"github.com/ava-labs/avalanchego/wallet/subnet/primary/common"
	"github.com/ava-labs/platform-cli/pkg/wallet"
)

const (
	// importRetryAttempts is the number of times to retry import after export
	importRetryAttempts = 5
	// importRetryDelay is the initial delay between import retries
	importRetryDelay = 500 * time.Millisecond
)

// ExportFromPChain exports AVAX from P-Chain to C-Chain.
// Returns the export transaction ID.
func ExportFromPChain(ctx context.Context, w *wallet.FullWallet, amountNAVAX uint64) (ids.ID, error) {
	pWallet := w.PWallet()
	cWallet := w.CWallet()

	// Get C-Chain blockchain ID and AVAX asset ID from the C-Chain wallet context
	cChainID := cWallet.Builder().Context().BlockchainID
	avaxAssetID := cWallet.Builder().Context().AVAXAssetID

	// Create owner for the exported funds
	owner := secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs:     []ids.ShortID{w.PChainAddress()},
	}

	// Issue the export transaction
	exportTx, err := pWallet.IssueExportTx(cChainID, []*avax.TransferableOutput{{
		Asset: avax.Asset{ID: avaxAssetID},
		Out: &secp256k1fx.TransferOutput{
			Amt:          amountNAVAX,
			OutputOwners: owner,
		},
	}}, common.WithContext(ctx))
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue P-Chain export tx: %w", err)
	}

	return exportTx.TxID, nil
}

// ImportToCChain imports AVAX to C-Chain from P-Chain.
// Returns the import transaction ID.
func ImportToCChain(ctx context.Context, w *wallet.FullWallet) (ids.ID, error) {
	cWallet := w.CWallet()
	ethAddr := w.EthAddress()

	// Issue the import transaction
	importTx, err := cWallet.IssueImportTx(constants.PlatformChainID, ethAddr, common.WithContext(ctx))
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue C-Chain import tx: %w", err)
	}

	return importTx.ID(), nil
}

// ExportFromCChain exports AVAX from C-Chain to P-Chain.
// Returns the export transaction ID.
func ExportFromCChain(ctx context.Context, w *wallet.FullWallet, amountNAVAX uint64) (ids.ID, error) {
	cWallet := w.CWallet()

	// Create owner for the exported funds
	owner := secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs:     []ids.ShortID{w.PChainAddress()},
	}

	// Issue the export transaction
	exportTx, err := cWallet.IssueExportTx(constants.PlatformChainID, []*secp256k1fx.TransferOutput{{
		Amt:          amountNAVAX,
		OutputOwners: owner,
	}}, common.WithContext(ctx))
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue C-Chain export tx: %w", err)
	}

	return exportTx.ID(), nil
}

// ImportToPChain imports AVAX to P-Chain from C-Chain.
// Returns the import transaction ID.
func ImportToPChain(ctx context.Context, w *wallet.FullWallet) (ids.ID, error) {
	pWallet := w.PWallet()
	cWallet := w.CWallet()

	// Get C-Chain blockchain ID
	cChainID := cWallet.Builder().Context().BlockchainID

	// Create owner for the imported funds
	owner := secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs:     []ids.ShortID{w.PChainAddress()},
	}

	// Issue the import transaction
	importTx, err := pWallet.IssueImportTx(cChainID, &owner, common.WithContext(ctx))
	if err != nil {
		return ids.Empty, fmt.Errorf("failed to issue P-Chain import tx: %w", err)
	}

	return importTx.TxID, nil
}

// TransferPToC performs a complete transfer from P-Chain to C-Chain.
// This is a convenience function that exports from P-Chain and imports to C-Chain.
// Returns both transaction IDs.
func TransferPToC(ctx context.Context, w *wallet.FullWallet, amountNAVAX uint64) (exportTxID, importTxID ids.ID, err error) {
	// Step 1: Export from P-Chain
	exportTxID, err = ExportFromPChain(ctx, w, amountNAVAX)
	if err != nil {
		return ids.Empty, ids.Empty, fmt.Errorf("export failed: %w", err)
	}

	// Step 2: Import to C-Chain with retry
	// Atomic UTXOs may not be immediately visible after export
	importTxID, err = importWithRetry(ctx, func() (ids.ID, error) {
		return ImportToCChain(ctx, w)
	})
	if err != nil {
		return exportTxID, ids.Empty, fmt.Errorf("import failed: %w", err)
	}

	return exportTxID, importTxID, nil
}

// TransferCToP performs a complete transfer from C-Chain to P-Chain.
// This is a convenience function that exports from C-Chain and imports to P-Chain.
// Returns both transaction IDs.
func TransferCToP(ctx context.Context, w *wallet.FullWallet, amountNAVAX uint64) (exportTxID, importTxID ids.ID, err error) {
	// Step 1: Export from C-Chain
	exportTxID, err = ExportFromCChain(ctx, w, amountNAVAX)
	if err != nil {
		return ids.Empty, ids.Empty, fmt.Errorf("export failed: %w", err)
	}

	// Step 2: Import to P-Chain with retry
	// Atomic UTXOs may not be immediately visible after export
	importTxID, err = importWithRetry(ctx, func() (ids.ID, error) {
		return ImportToPChain(ctx, w)
	})
	if err != nil {
		return exportTxID, ids.Empty, fmt.Errorf("import failed: %w", err)
	}

	return exportTxID, importTxID, nil
}

// isRetryableImportError checks if an import error is retryable.
// These errors typically indicate UTXOs aren't visible yet after export.
func isRetryableImportError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	// Common patterns for transient UTXO visibility issues
	retryablePatterns := []string{
		"not found",
		"no utxos",
		"insufficient funds", // May occur if UTXOs haven't propagated
		"missing utxo",
	}
	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
}

// importWithRetry attempts an import operation with retries.
// This handles the case where atomic UTXOs aren't immediately visible after export.
func importWithRetry(ctx context.Context, importFn func() (ids.ID, error)) (ids.ID, error) {
	var lastErr error
	delay := importRetryDelay

	for attempt := 0; attempt < importRetryAttempts; attempt++ {
		txID, err := importFn()
		if err == nil {
			return txID, nil
		}

		// Only retry on transient UTXO visibility errors
		if !isRetryableImportError(err) {
			return ids.Empty, err
		}

		lastErr = err

		if attempt == importRetryAttempts-1 {
			break
		}

		// Wait before retrying (with exponential backoff)
		select {
		case <-ctx.Done():
			return ids.Empty, ctx.Err()
		case <-time.After(delay):
			delay *= 2 // exponential backoff
		}
	}

	return ids.Empty, fmt.Errorf("import failed after %d attempts: %w", importRetryAttempts, lastErr)
}
