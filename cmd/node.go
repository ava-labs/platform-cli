package cmd

import (
	"fmt"

	"github.com/ava-labs/platform-cli/pkg/node"
	"github.com/spf13/cobra"
)

var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Node information",
	Long:  `Node information operations including getting node ID and BLS key.`,
}

var nodeIP string

var nodeInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Get node information",
	Long: `Get node ID and BLS public key from an avalanchego node.

Accepts --ip (IP or IP:port) or the global --rpc-url flag.
When both are provided, --ip takes precedence.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := getOperationContext()
		defer cancel()

		addr := nodeIP
		if addr == "" && customRPCURL != "" {
			addr = customRPCURL
		}
		if addr == "" {
			return fmt.Errorf("--ip or --rpc-url is required")
		}

		info, err := node.GetNodeInfoWithInsecureHTTP(ctx, addr, allowInsecureHTTP)
		if err != nil {
			return fmt.Errorf("failed to get node info: %w", err)
		}

		fmt.Printf("Node ID:        %s\n", info.NodeID)
		fmt.Printf("BLS Public Key: %s\n", info.BLSPublicKey)
		fmt.Printf("BLS PoP:        %s\n", info.BLSProofOfPossession)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(nodeCmd)
	nodeCmd.AddCommand(nodeInfoCmd)

	nodeInfoCmd.Flags().StringVar(&nodeIP, "ip", "", "Node IP or IP:port (also accepts --rpc-url)")
}
