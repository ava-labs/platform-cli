package crosschain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ava-labs/avalanchego/api"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/json"
	"github.com/ava-labs/avalanchego/utils/rpc"
)

// cChainAtomicPollFrequency is the interval between C-Chain atomic tx acceptance checks.
const cChainAtomicPollFrequency = 250 * time.Millisecond

// errIssuedNotConfirmed marks an atomic tx that the node accepted for issuance
// but whose acceptance we could not confirm. Callers must not re-issue it.
var errIssuedNotConfirmed = errors.New("issued but acceptance not confirmed")

// getAtomicTxReply is the subset of the avax.getAtomicTx reply that we read.
// Both the legacy coreth C-Chain and the SAE (Helicon) C-Chain set blockHeight
// only after the tx is accepted.
type getAtomicTxReply struct {
	BlockHeight *json.Uint64 `json:"blockHeight,omitempty"`
}

// awaitCChainAtomicTx polls avax.getAtomicTx until txID is accepted on the C-Chain.
//
// The avalanchego C-Chain wallet waits with avax.getAtomicTxStatus. The SAE
// C-Chain deprecates that method and public API endpoints do not serve it, so
// the wallet returns an error after the tx is already issued. We issue with
// common.WithAssumeDecided() and wait here instead.
func awaitCChainAtomicTx(ctx context.Context, rpcURL string, txID ids.ID) error {
	requester := rpc.NewEndpointRequester(rpcURL + "/ext/bc/C/avax")
	return awaitAtomicTx(ctx, requester, txID, cChainAtomicPollFrequency)
}

func awaitAtomicTx(ctx context.Context, requester rpc.EndpointRequester, txID ids.ID, freq time.Duration) error {
	ticker := time.NewTicker(freq)
	defer ticker.Stop()

	for {
		reply := &getAtomicTxReply{}
		err := requester.SendRequest(ctx, "avax.getAtomicTx", &api.GetTxArgs{
			TxID:     txID,
			Encoding: formatting.Hex,
		}, reply)
		switch {
		case err == nil && reply.BlockHeight != nil:
			return nil
		case err != nil && ctx.Err() == nil && !isTransientPollError(err):
			return fmt.Errorf("tx %s %w: %v", txID, errIssuedNotConfirmed, err)
		}

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return fmt.Errorf("tx %s %w: %v", txID, errIssuedNotConfirmed, ctx.Err())
		}
	}
}

// isTransientPollError reports whether the poll should continue after err.
// SAE returns "fetching tx: reading tx: not found" and legacy coreth returns
// "could not find tx <id>" until the tx is accepted. Rate limits also pass.
func isTransientPollError(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, pattern := range []string{"not found", "could not find", "status code: 429", "too many requests"} {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}
