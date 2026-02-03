package cmd

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto/bls"
	"github.com/ava-labs/platform-cli/pkg/network"
	"github.com/ava-labs/platform-cli/pkg/pchain"
	"github.com/ava-labs/platform-cli/pkg/wallet"
	"github.com/spf13/cobra"
)

var (
	l1ValidationID string
	l1Balance      float64
	l1Message      string
	l1PoP          string
)

var l1Cmd = &cobra.Command{
	Use:   "l1",
	Short: "L1 validator operations",
	Long:  `Manage validators on Avalanche L1 blockchains.`,
}

var l1RegisterValidatorCmd = &cobra.Command{
	Use:   "register-validator",
	Short: "Register a new L1 validator",
	Long:  `Register a new validator on an L1 blockchain.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		if l1Message == "" {
			return fmt.Errorf("--message is required (hex-encoded Warp message)")
		}
		if l1PoP == "" {
			return fmt.Errorf("--pop is required (hex-encoded BLS proof of possession)")
		}

		message, err := hex.DecodeString(strings.TrimPrefix(l1Message, "0x"))
		if err != nil {
			return fmt.Errorf("invalid message: %w", err)
		}

		popBytes, err := hex.DecodeString(strings.TrimPrefix(l1PoP, "0x"))
		if err != nil {
			return fmt.Errorf("invalid PoP: %w", err)
		}
		if len(popBytes) != bls.SignatureLen {
			return fmt.Errorf("invalid PoP length: expected %d bytes, got %d", bls.SignatureLen, len(popBytes))
		}

		var pop [bls.SignatureLen]byte
		copy(pop[:], popBytes)

		keyBytes, err := loadKey()
		if err != nil {
			return err
		}
		key, err := wallet.ToPrivateKey(keyBytes)
		if err != nil {
			return err
		}

		netConfig := network.GetConfig(networkName)
		w, err := wallet.NewWallet(ctx, key, netConfig)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}

		txID, err := pchain.RegisterL1Validator(ctx, w, uint64(l1Balance*1e9), pop, message)
		if err != nil {
			return err
		}

		fmt.Printf("Register L1 Validator TX: %s\n", txID)
		return nil
	},
}

var l1SetWeightCmd = &cobra.Command{
	Use:   "set-weight",
	Short: "Set L1 validator weight",
	Long:  `Set the weight of a validator on an L1 blockchain.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		if l1Message == "" {
			return fmt.Errorf("--message is required (hex-encoded Warp message)")
		}

		message, err := hex.DecodeString(strings.TrimPrefix(l1Message, "0x"))
		if err != nil {
			return fmt.Errorf("invalid message: %w", err)
		}

		keyBytes, err := loadKey()
		if err != nil {
			return err
		}
		key, err := wallet.ToPrivateKey(keyBytes)
		if err != nil {
			return err
		}

		netConfig := network.GetConfig(networkName)
		w, err := wallet.NewWallet(ctx, key, netConfig)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}

		txID, err := pchain.SetL1ValidatorWeight(ctx, w, message)
		if err != nil {
			return err
		}

		fmt.Printf("Set L1 Validator Weight TX: %s\n", txID)
		return nil
	},
}

var l1AddBalanceCmd = &cobra.Command{
	Use:   "add-balance",
	Short: "Increase L1 validator balance",
	Long:  `Increase the balance of a validator on an L1 blockchain for continuous fees.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		if l1ValidationID == "" {
			return fmt.Errorf("--validation-id is required")
		}

		validationID, err := ids.FromString(l1ValidationID)
		if err != nil {
			return fmt.Errorf("invalid validation ID: %w", err)
		}

		keyBytes, err := loadKey()
		if err != nil {
			return err
		}
		key, err := wallet.ToPrivateKey(keyBytes)
		if err != nil {
			return err
		}

		netConfig := network.GetConfig(networkName)
		w, err := wallet.NewWallet(ctx, key, netConfig)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}

		txID, err := pchain.IncreaseL1ValidatorBalance(ctx, w, validationID, uint64(l1Balance*1e9))
		if err != nil {
			return err
		}

		fmt.Printf("Increase L1 Validator Balance TX: %s\n", txID)
		return nil
	},
}

var l1DisableValidatorCmd = &cobra.Command{
	Use:   "disable-validator",
	Short: "Disable an L1 validator",
	Long:  `Disable a validator on an L1 blockchain and return remaining funds.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		if l1ValidationID == "" {
			return fmt.Errorf("--validation-id is required")
		}

		validationID, err := ids.FromString(l1ValidationID)
		if err != nil {
			return fmt.Errorf("invalid validation ID: %w", err)
		}

		keyBytes, err := loadKey()
		if err != nil {
			return err
		}
		key, err := wallet.ToPrivateKey(keyBytes)
		if err != nil {
			return err
		}

		netConfig := network.GetConfig(networkName)
		w, err := wallet.NewWallet(ctx, key, netConfig)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}

		txID, err := pchain.DisableL1Validator(ctx, w, validationID)
		if err != nil {
			return err
		}

		fmt.Printf("Disable L1 Validator TX: %s\n", txID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(l1Cmd)

	l1Cmd.AddCommand(l1RegisterValidatorCmd)
	l1Cmd.AddCommand(l1SetWeightCmd)
	l1Cmd.AddCommand(l1AddBalanceCmd)
	l1Cmd.AddCommand(l1DisableValidatorCmd)

	// Register validator flags
	l1RegisterValidatorCmd.Flags().Float64Var(&l1Balance, "balance", 0, "Initial balance in AVAX for continuous fees")
	l1RegisterValidatorCmd.Flags().StringVar(&l1PoP, "pop", "", "BLS proof of possession (hex)")
	l1RegisterValidatorCmd.Flags().StringVar(&l1Message, "message", "", "Warp message authorizing the validator (hex)")

	// Set weight flags
	l1SetWeightCmd.Flags().StringVar(&l1Message, "message", "", "Warp message authorizing the weight change (hex)")

	// Add balance flags
	l1AddBalanceCmd.Flags().StringVar(&l1ValidationID, "validation-id", "", "Validation ID")
	l1AddBalanceCmd.Flags().Float64Var(&l1Balance, "amount", 0, "Amount in AVAX to add")

	// Disable validator flags
	l1DisableValidatorCmd.Flags().StringVar(&l1ValidationID, "validation-id", "", "Validation ID to disable")
}
