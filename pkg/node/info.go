// Package node provides utilities for querying Avalanche node information.
package node

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ava-labs/avalanchego/api/info"
)

// NodeInfo holds information about an Avalanche node.
type NodeInfo struct {
	NodeID               string
	BLSPublicKey         string
	BLSProofOfPossession string
}

// normalizeNodeURI converts a node address to a full URI.
// Accepts: "127.0.0.1", "127.0.0.1:9650", "http://127.0.0.1:9650"
func normalizeNodeURI(addr string) string {
	// Already a full URI
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	// Has port but no scheme
	if strings.Contains(addr, ":") {
		return "http://" + addr
	}
	// Just IP/hostname, add default port
	return fmt.Sprintf("http://%s:9650", addr)
}

// GetNodeInfo queries an avalanchego node for its node ID and BLS key.
// Accepts IP, IP:port, or full URI (e.g., "127.0.0.1", "127.0.0.1:9650", "http://127.0.0.1:9650").
func GetNodeInfo(ctx context.Context, addr string) (*NodeInfo, error) {
	uri := normalizeNodeURI(addr)
	infoClient := info.NewClient(uri)

	nodeID, nodePoP, err := infoClient.GetNodeID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get node info from %s: %w", uri, err)
	}

	blsPubKey := ""
	blsPoP := ""
	if nodePoP != nil {
		blsPubKey = hex.EncodeToString(nodePoP.PublicKey[:])
		blsPoP = hex.EncodeToString(nodePoP.ProofOfPossession[:])
	}

	return &NodeInfo{
		NodeID:               nodeID.String(),
		BLSPublicKey:         blsPubKey,
		BLSProofOfPossession: blsPoP,
	}, nil
}

// GetNodeIDs queries multiple nodes and returns their NodeIDs.
func GetNodeIDs(ctx context.Context, ips []string) ([]string, error) {
	nodeIDs := make([]string, 0, len(ips))

	for _, ip := range ips {
		info, err := GetNodeInfo(ctx, ip)
		if err != nil {
			return nil, err
		}
		nodeIDs = append(nodeIDs, info.NodeID)
	}

	return nodeIDs, nil
}
