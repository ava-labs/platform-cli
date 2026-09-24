package crosschain

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/json"
	"github.com/ava-labs/avalanchego/utils/rpc"
)

// fakeRequester returns one scripted response per avax.getAtomicTx call.
type fakeRequester struct {
	responses []fakeResponse
	calls     int
}

type fakeResponse struct {
	height *uint64
	err    error
}

func (f *fakeRequester) SendRequest(_ context.Context, method string, _ interface{}, reply interface{}, _ ...rpc.Option) error {
	if method != "avax.getAtomicTx" {
		return errors.New("unexpected method " + method)
	}
	r := f.responses[min(f.calls, len(f.responses)-1)]
	f.calls++
	if r.err != nil {
		return r.err
	}
	if r.height != nil {
		h := json.Uint64(*r.height)
		reply.(*getAtomicTxReply).BlockHeight = &h
	}
	return nil
}

func TestAwaitAtomicTx(t *testing.T) {
	height := uint64(42)
	tests := []struct {
		name          string
		responses     []fakeResponse
		wantErr       bool
		wantNotIssued bool
		wantCalls     int
	}{
		{
			name:      "accepted on first poll",
			responses: []fakeResponse{{height: &height}},
			wantCalls: 1,
		},
		{
			name: "SAE not found then accepted",
			responses: []fakeResponse{
				{err: errors.New("fetching tx: reading tx: not found")},
				{height: &height},
			},
			wantCalls: 2,
		},
		{
			name: "legacy could not find, processing, then accepted",
			responses: []fakeResponse{
				{err: errors.New("could not find tx 2QouvFWUbjuySRxeX5xMbNCuAaKWfbk5FeEa2JmoF85RKLk2dD")},
				{},
				{height: &height},
			},
			wantCalls: 3,
		},
		{
			name: "rate limit then accepted",
			responses: []fakeResponse{
				{err: errors.New("received status code: 429")},
				{height: &height},
			},
			wantCalls: 2,
		},
		{
			name:      "hard error stops poll",
			responses: []fakeResponse{{err: errors.New("the method avax.getAtomicTx is not available")}},
			wantErr:   true,
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeRequester{responses: tt.responses}
			err := awaitAtomicTx(context.Background(), f, ids.GenerateTestID(), time.Millisecond)
			if (err != nil) != tt.wantErr {
				t.Fatalf("awaitAtomicTx() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, errIssuedNotConfirmed) {
				t.Errorf("awaitAtomicTx() error = %v, want errIssuedNotConfirmed", err)
			}
			if f.calls != tt.wantCalls {
				t.Errorf("calls = %d, want %d", f.calls, tt.wantCalls)
			}
		})
	}
}

func TestAwaitAtomicTxContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	f := &fakeRequester{responses: []fakeResponse{{err: errors.New("fetching tx: reading tx: not found")}}}
	err := awaitAtomicTx(ctx, f, ids.GenerateTestID(), time.Millisecond)
	if !errors.Is(err, errIssuedNotConfirmed) {
		t.Fatalf("awaitAtomicTx() error = %v, want errIssuedNotConfirmed", err)
	}
}

func TestImportWithRetryDoesNotReissue(t *testing.T) {
	calls := 0
	_, err := importWithRetry(context.Background(), func() (ids.ID, error) {
		calls++
		return ids.Empty, errors.Join(errIssuedNotConfirmed, errors.New("not found"))
	})
	if !errors.Is(err, errIssuedNotConfirmed) {
		t.Fatalf("importWithRetry() error = %v, want errIssuedNotConfirmed", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (must not re-issue)", calls)
	}
}
