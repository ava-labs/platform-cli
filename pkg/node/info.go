// Package node provides utilities for querying Avalanche node information.
package node

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/ava-labs/avalanchego/api/info"
)

// NodeInfo holds information about an Avalanche node.
type NodeInfo struct {
	NodeID               string
	BLSPublicKey         string
	BLSProofOfPossession string
}

// NormalizeNodeURI converts a node address to a base URI suitable for info.NewClient.
// Accepts: "127.0.0.1", "127.0.0.1:9650", "http://127.0.0.1:9650".
//
// The info client appends "/ext/info", so this rejects custom paths except a
// trailing "/ext/info" (which is normalized away).
func NormalizeNodeURI(addr string) (string, error) {
	return NormalizeNodeURIWithInsecureHTTP(addr, false)
}

// NormalizeNodeURIWithInsecureHTTP converts a node address to a base URI
// suitable for info.NewClient.
//
// Security defaults:
//   - host[:port] shorthand defaults to HTTPS for non-local hosts.
//   - localhost / loopback shorthand defaults to HTTP.
//   - explicit HTTP for non-local hosts is rejected unless allowInsecureHTTP is true.
func NormalizeNodeURIWithInsecureHTTP(addr string, allowInsecureHTTP bool) (string, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", fmt.Errorf("node address cannot be empty")
	}

	hasScheme := strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://")

	// Allow host[:port] shorthand.
	if !hasScheme {
		if strings.Contains(addr, "/") {
			return "", fmt.Errorf("invalid node address %q: use host[:port] or http(s)://host[:port]", addr)
		}
		if strings.Contains(addr, ":") {
			addr = "https://" + addr
		} else {
			addr = fmt.Sprintf("https://%s:9650", addr)
		}
	}

	parsed, err := url.Parse(addr)
	if err != nil {
		return "", fmt.Errorf("invalid node URI %q: %w", addr, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported URI scheme %q: use http or https", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("invalid node URI %q: missing host", addr)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("invalid node URI %q: query and fragment are not supported", addr)
	}

	switch trimmedPath := strings.TrimSuffix(parsed.EscapedPath(), "/"); trimmedPath {
	case "", "/":
		parsed.Path = ""
	case "/ext/info":
		parsed.Path = ""
	default:
		return "", fmt.Errorf("invalid node URI %q: use base URI only (without path)", addr)
	}

	hostname := parsed.Hostname()
	if !hasScheme && isLoopbackHost(hostname) {
		// Preserve legacy local-node ergonomics for shorthand inputs.
		parsed.Scheme = "http"
	}

	if parsed.Scheme == "http" && !allowInsecureHTTP && !isLoopbackHost(hostname) {
		return "", fmt.Errorf(
			"insecure HTTP is disabled for non-local host %q (use https:// or enable insecure HTTP explicitly)",
			hostname,
		)
	}

	return parsed.String(), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// GetNodeInfo queries an avalanchego node for its node ID and BLS key.
// Accepts IP, IP:port, or full URI (e.g., "127.0.0.1", "127.0.0.1:9650", "http://127.0.0.1:9650").
func GetNodeInfo(ctx context.Context, addr string) (*NodeInfo, error) {
	return GetNodeInfoWithInsecureHTTP(ctx, addr, false)
}

// GetNodeInfoWithInsecureHTTP queries an avalanchego node for its node ID and BLS key,
// with optional support for insecure HTTP on non-local hosts.
func GetNodeInfoWithInsecureHTTP(ctx context.Context, addr string, allowInsecureHTTP bool) (*NodeInfo, error) {
	uri, err := NormalizeNodeURIWithInsecureHTTP(addr, allowInsecureHTTP)
	if err != nil {
		return nil, err
	}
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
