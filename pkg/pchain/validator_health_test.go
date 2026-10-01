package pchain

import (
	"context"
	"strings"
	"testing"

	"github.com/ava-labs/avalanchego/ids"
)

func TestGetValidatorHealth(t *testing.T) {
	nodeID := ids.GenerateTestNodeID()
	otherNodeID := ids.GenerateTestNodeID()

	tests := []struct {
		name          string
		validators    []map[string]any
		wantFound     bool
		wantConnected bool
		wantUptime    float64
	}{
		{
			name: "connected validator",
			validators: []map[string]any{{
				"nodeID":    nodeID.String(),
				"connected": true,
				"uptime":    "99.5000",
			}},
			wantFound:     true,
			wantConnected: true,
			wantUptime:    99.5,
		},
		{
			name: "disconnected validator",
			validators: []map[string]any{{
				"nodeID":    nodeID.String(),
				"connected": false,
				"uptime":    "0.0000",
			}},
			wantFound:     true,
			wantConnected: false,
		},
		{
			name: "missing uptime is treated as zero",
			validators: []map[string]any{{
				"nodeID":    nodeID.String(),
				"connected": true,
			}},
			wantFound:     true,
			wantConnected: true,
		},
		{
			name:       "not in the validator set",
			validators: []map[string]any{},
		},
		{
			name: "other node ids are ignored",
			validators: []map[string]any{{
				"nodeID":    otherNodeID.String(),
				"connected": true,
				"uptime":    "100.0000",
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotParams string
			server := newCurrentValidatorsServer(t, &gotParams, tt.validators)
			defer server.Close()

			health, err := GetValidatorHealth(context.Background(), server.URL, nodeID)
			if err != nil {
				t.Fatalf("GetValidatorHealth() error = %v", err)
			}
			if health.Found != tt.wantFound {
				t.Errorf("Found = %v, want %v", health.Found, tt.wantFound)
			}
			if health.Connected != tt.wantConnected {
				t.Errorf("Connected = %v, want %v", health.Connected, tt.wantConnected)
			}
			if health.UptimePercent != tt.wantUptime {
				t.Errorf("UptimePercent = %v, want %v", health.UptimePercent, tt.wantUptime)
			}
			// The query must be scoped to the node and the primary network, so the
			// reply cannot be diluted by the whole validator set.
			if !strings.Contains(gotParams, nodeID.String()) {
				t.Errorf("request params = %s, want it to contain %s", gotParams, nodeID)
			}
		})
	}
}

func TestGetValidatorHealthInvalidUptime(t *testing.T) {
	nodeID := ids.GenerateTestNodeID()
	server := newCurrentValidatorsServer(t, nil, []map[string]any{{
		"nodeID":    nodeID.String(),
		"connected": true,
		"uptime":    "not-a-number",
	}})
	defer server.Close()

	if _, err := GetValidatorHealth(context.Background(), server.URL, nodeID); err == nil {
		t.Fatal("GetValidatorHealth() error = nil, want an error for an unparseable uptime")
	}
}
