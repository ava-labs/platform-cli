package cmd

import (
	"bufio"
	"crypto/subtle"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/ava-labs/platform-cli/pkg/keystore"
	"github.com/ava-labs/platform-cli/pkg/wallet"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// clearBytes securely zeros a byte slice to prevent sensitive data from lingering in memory.
func clearBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

var (
	// keys flags
	keyName    string
	keyEncrypt bool
	keyFormat  string
	keyForce   bool
	showAddrs  bool
)

var keysCmd = &cobra.Command{
	Use:   "keys",
	Short: "Key management operations",
	Long: `Manage persistent keys stored in ~/.platform/keys/

Keys can be stored encrypted (with a password) or unencrypted.
Encrypted keys use Argon2id for key derivation and AES-256-GCM for encryption.

Subcommands:
  import    Import a private key
  generate  Generate a new random key
  list      List all stored keys
  export    Export a key (show private key)
  delete    Remove a stored key`,
}

var keysImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import a private key",
	Long: `Import a private key into the keystore.

If --private-key is not provided, you will be prompted to enter it (hidden input).
Keys are encrypted by default. Use --encrypt=false to store unencrypted keys (unsafe).
When encryption is enabled, set PLATFORM_CLI_KEY_PASSWORD for non-interactive use
or follow the password prompt.

Examples:
  platform keys import --name mykey --private-key "PrivateKey-..."
  platform keys import --name mykey
  platform keys import --name mykey --encrypt=false`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if keyName == "" {
			return fmt.Errorf("--name is required")
		}
		if err := keystore.ValidateKeyName(keyName); err != nil {
			return err
		}

		ks, err := keystore.Load()
		if err != nil {
			return fmt.Errorf("failed to load keystore: %w", err)
		}

		// Check if key already exists
		if ks.HasKey(keyName) {
			return fmt.Errorf("key %q already exists. Use a different name or delete the existing key first", keyName)
		}

		// Get private key
		keyStr := privateKey
		if keyStr == "" {
			keyStr = os.Getenv("AVALANCHE_PRIVATE_KEY")
		}
		if keyStr == "" {
			// Prompt for key (hidden input)
			fmt.Print("Enter private key: ")
			inputBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println()
			if err != nil {
				return fmt.Errorf("failed to read private key: %w", err)
			}
			keyStr = string(inputBytes)
			clearBytes(inputBytes)
		}

		keyBytes, err := wallet.ParsePrivateKey(keyStr)
		if err != nil {
			return fmt.Errorf("invalid private key: %w", err)
		}
		// Clear key bytes when done
		defer clearBytes(keyBytes)

		// Get password if encrypting
		var password []byte
		if keyEncrypt {
			if envPwd := os.Getenv("PLATFORM_CLI_KEY_PASSWORD"); envPwd != "" {
				password = []byte(envPwd)
				if len(password) < 8 {
					clearBytes(password)
					return fmt.Errorf("PLATFORM_CLI_KEY_PASSWORD must be at least 8 characters")
				}
			} else {
				password, err = promptPassword(true)
				if err != nil {
					return err
				}
			}
			defer clearBytes(password)
		}

		// Import the key
		if err := ks.ImportKey(keyName, keyBytes, password); err != nil {
			return err
		}

		entry, _ := ks.GetKey(keyName)
		fmt.Printf("Key imported successfully!\n")
		fmt.Printf("  Name:          %s\n", keyName)
		fmt.Printf("  P-Chain:       %s\n", entry.PChainAddress)
		fmt.Printf("  EVM:           %s\n", entry.EVMAddress)
		fmt.Printf("  Encrypted:     %v\n", entry.Encrypted)

		if ks.GetDefault() == keyName {
			fmt.Printf("  Default:       yes\n")
		}

		return nil
	},
}

var keysGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate a new random key",
	Long: `Generate a new random secp256k1 private key.

Keys are encrypted by default. Use --encrypt=false to store unencrypted keys (unsafe).
When encryption is enabled, set PLATFORM_CLI_KEY_PASSWORD for non-interactive use
or follow the password prompt.

Examples:
  platform keys generate --name mykey
  platform keys generate --name mykey --encrypt=false`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if keyName == "" {
			return fmt.Errorf("--name is required")
		}
		if err := keystore.ValidateKeyName(keyName); err != nil {
			return err
		}

		ks, err := keystore.Load()
		if err != nil {
			return fmt.Errorf("failed to load keystore: %w", err)
		}

		// Check if key already exists
		if ks.HasKey(keyName) {
			return fmt.Errorf("key %q already exists. Use a different name or delete the existing key first", keyName)
		}

		// Get password if encrypting
		var password []byte
		if keyEncrypt {
			if envPwd := os.Getenv("PLATFORM_CLI_KEY_PASSWORD"); envPwd != "" {
				password = []byte(envPwd)
				if len(password) < 8 {
					clearBytes(password)
					return fmt.Errorf("PLATFORM_CLI_KEY_PASSWORD must be at least 8 characters")
				}
			} else {
				password, err = promptPassword(true)
				if err != nil {
					return err
				}
			}
			defer clearBytes(password)
		}

		// Generate the key
		keyBytes, err := ks.GenerateKey(keyName, password)
		if err != nil {
			return err
		}
		// Clear key bytes when done (important: derive addresses before clearing)
		defer clearBytes(keyBytes)

		entry, _ := ks.GetKey(keyName)
		pAddr, evmAddr := wallet.DeriveAddresses(keyBytes)

		fmt.Printf("Key generated successfully!\n")
		fmt.Printf("  Name:          %s\n", keyName)
		fmt.Printf("  P-Chain:       %s\n", pAddr)
		fmt.Printf("  EVM:           %s\n", evmAddr)
		fmt.Printf("  Encrypted:     %v\n", entry.Encrypted)

		if ks.GetDefault() == keyName {
			fmt.Printf("  Default:       yes\n")
		}

		fmt.Println()
		fmt.Println("WARNING: Back up your key! Use 'platform keys export' to view the private key.")

		return nil
	},
}

var keysListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all stored keys",
	Long: `List all keys stored in the keystore.

Use --show-addresses to display P-Chain and EVM addresses.

Examples:
  platform keys list
  platform keys list --show-addresses`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ks, err := keystore.Load()
		if err != nil {
			return fmt.Errorf("failed to load keystore: %w", err)
		}

		entries := ks.ListKeys()
		if len(entries) == 0 {
			fmt.Println("No keys found. Use 'platform keys import' or 'platform keys generate' to add a key.")
			return nil
		}

		// Sort by name
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Name < entries[j].Name
		})

		defaultKey := ks.GetDefault()

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

		if showAddrs {
			fmt.Fprintln(w, "NAME\tENCRYPTED\tDEFAULT\tP-CHAIN\tEVM\tCREATED")
			for _, e := range entries {
				isDefault := ""
				if e.Name == defaultKey {
					isDefault = "*"
				}
				encrypted := "no"
				if e.Encrypted {
					encrypted = "yes"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
					e.Name, encrypted, isDefault, e.PChainAddress, e.EVMAddress, e.CreatedAt.Format("2006-01-02"))
			}
		} else {
			fmt.Fprintln(w, "NAME\tENCRYPTED\tDEFAULT\tCREATED")
			for _, e := range entries {
				isDefault := ""
				if e.Name == defaultKey {
					isDefault = "*"
				}
				encrypted := "no"
				if e.Encrypted {
					encrypted = "yes"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					e.Name, encrypted, isDefault, e.CreatedAt.Format("2006-01-02"))
			}
		}

		w.Flush()

		fmt.Printf("\nTotal: %d key(s)\n", len(entries))
		return nil
	},
}

var keysExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export a key (show private key)",
	Long: `Export a key by displaying its private key.

WARNING: This will display your private key in plaintext!

If the key is encrypted, you will be prompted for the password.

Examples:
  platform keys export --name mykey
  platform keys export --name mykey --format hex`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if keyName == "" {
			return fmt.Errorf("--name is required")
		}
		if err := keystore.ValidateKeyName(keyName); err != nil {
			return err
		}

		ks, err := keystore.Load()
		if err != nil {
			return fmt.Errorf("failed to load keystore: %w", err)
		}

		if !ks.HasKey(keyName) {
			return fmt.Errorf("key %q not found", keyName)
		}

		// Get password if encrypted
		var password []byte
		if ks.IsEncrypted(keyName) {
			// Support non-interactive usage in scripts/CI.
			if envPwd := os.Getenv("PLATFORM_CLI_KEY_PASSWORD"); envPwd != "" {
				password = []byte(envPwd)
			} else {
				password, err = promptPassword(false)
				if err != nil {
					return err
				}
			}
			defer clearBytes(password)
		}

		// Export the key
		exported, err := ks.ExportKey(keyName, password, keyFormat)
		if err != nil {
			return err
		}

		fmt.Println(exported)
		return nil
	},
}

var keysDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Remove a stored key",
	Long: `Delete a key from the keystore.

This action is irreversible! Make sure you have a backup of your key.

Examples:
  platform keys delete --name mykey
  platform keys delete --name mykey --force`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if keyName == "" {
			return fmt.Errorf("--name is required")
		}
		if err := keystore.ValidateKeyName(keyName); err != nil {
			return err
		}

		ks, err := keystore.Load()
		if err != nil {
			return fmt.Errorf("failed to load keystore: %w", err)
		}

		if !ks.HasKey(keyName) {
			return fmt.Errorf("key %q not found", keyName)
		}

		// Confirm deletion
		if !keyForce {
			fmt.Printf("Are you sure you want to delete key %q? This cannot be undone.\n", keyName)
			fmt.Print("Type 'yes' to confirm: ")

			reader := bufio.NewReader(os.Stdin)
			response, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read response: %w", err)
			}

			if strings.TrimSpace(strings.ToLower(response)) != "yes" {
				fmt.Println("Deletion cancelled.")
				return nil
			}
		}

		if err := ks.DeleteKey(keyName); err != nil {
			return err
		}

		fmt.Printf("Key %q deleted successfully.\n", keyName)
		return nil
	},
}

var keysDefaultCmd = &cobra.Command{
	Use:   "default",
	Short: "Set or show the default key",
	Long: `Set or show the default key.

When no --name is provided, shows the current default key.
When --name is provided, sets that key as the default.

Examples:
  platform keys default
  platform keys default --name mykey`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ks, err := keystore.Load()
		if err != nil {
			return fmt.Errorf("failed to load keystore: %w", err)
		}

		if keyName == "" {
			// Show current default
			defaultKey := ks.GetDefault()
			if defaultKey == "" {
				fmt.Println("No default key set.")
			} else {
				fmt.Printf("Default key: %s\n", defaultKey)
			}
			return nil
		}

		// Set new default
		if err := keystore.ValidateKeyName(keyName); err != nil {
			return err
		}

		if err := ks.SetDefault(keyName); err != nil {
			return err
		}

		fmt.Printf("Default key set to: %s\n", keyName)
		return nil
	},
}

// promptPassword prompts for a password. If confirm is true, asks for confirmation.
// The returned password must be cleared by the caller when no longer needed.
func promptPassword(confirm bool) ([]byte, error) {
	fmt.Print("Enter password: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return nil, fmt.Errorf("failed to read password: %w", err)
	}

	if len(password) < 8 {
		clearBytes(password)
		return nil, fmt.Errorf("password must be at least 8 characters")
	}

	if confirm {
		fmt.Print("Confirm password: ")
		confirmPwd, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			clearBytes(password)
			return nil, fmt.Errorf("failed to read password confirmation: %w", err)
		}

		// Use constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare(password, confirmPwd) != 1 {
			clearBytes(password)
			clearBytes(confirmPwd)
			return nil, fmt.Errorf("passwords do not match")
		}
		clearBytes(confirmPwd)
	}

	return password, nil
}

func init() {
	rootCmd.AddCommand(keysCmd)
	keysCmd.AddCommand(keysImportCmd)
	keysCmd.AddCommand(keysGenerateCmd)
	keysCmd.AddCommand(keysListCmd)
	keysCmd.AddCommand(keysExportCmd)
	keysCmd.AddCommand(keysDeleteCmd)
	keysCmd.AddCommand(keysDefaultCmd)

	// Import flags
	keysImportCmd.Flags().StringVar(&keyName, "name", "", "Name for the key (required)")
	keysImportCmd.Flags().BoolVar(&keyEncrypt, "encrypt", true, "Encrypt the key with a password (default true)")

	// Generate flags
	keysGenerateCmd.Flags().StringVar(&keyName, "name", "", "Name for the key (required)")
	keysGenerateCmd.Flags().BoolVar(&keyEncrypt, "encrypt", true, "Encrypt the key with a password (default true)")

	// List flags
	keysListCmd.Flags().BoolVar(&showAddrs, "show-addresses", false, "Show P-Chain and EVM addresses")

	// Export flags
	keysExportCmd.Flags().StringVar(&keyName, "name", "", "Name of the key to export (required)")
	keysExportCmd.Flags().StringVar(&keyFormat, "format", "cb58", "Output format: cb58 or hex")

	// Delete flags
	keysDeleteCmd.Flags().StringVar(&keyName, "name", "", "Name of the key to delete (required)")
	keysDeleteCmd.Flags().BoolVar(&keyForce, "force", false, "Skip confirmation prompt")

	// Default flags
	keysDefaultCmd.Flags().StringVar(&keyName, "name", "", "Name of the key to set as default")
}
