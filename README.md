# platform-cli

Command-line utilities for Avalanche P-Chain operations.

## Installation

```bash
go install github.com/ava-labs/platform-cli@latest
```

Or build from source:

```bash
git clone https://github.com/ava-labs/platform-cli.git
cd platform-cli
go build -o platform-cli .
```

## Key Management

Keys are stored in `~/.platform-cli/keys.json`. The built-in `ewoq` key is always available for local/test networks.

```bash
# Generate a new key
platform-cli keys generate mykey

# Generate an encrypted key
platform-cli keys generate mykey --encrypt

# Import an existing key
platform-cli keys import mykey --private-key "PrivateKey-..."

# List all keys
platform-cli keys list

# Set default key
platform-cli keys default mykey

# Export a key
platform-cli keys export mykey

# Delete a key
platform-cli keys delete mykey
```

## Wallet Operations

```bash
# Show wallet addresses (uses default key)
platform-cli wallet address

# Show wallet addresses with specific key
platform-cli wallet address --key-name mykey

# Check P-Chain balance
platform-cli wallet balance --key-name mykey --network fuji
```

## Cross-Chain Transfers

Transfer AVAX between P-Chain and C-Chain.

```bash
# Transfer from P-Chain to C-Chain (full transfer)
platform-cli transfer p-to-c --amount 1.0 --key-name mykey --network fuji

# Transfer from C-Chain to P-Chain (full transfer)
platform-cli transfer c-to-p --amount 1.0 --key-name mykey --network fuji

# Manual two-step transfer (export then import)
platform-cli transfer export --from p --to c --amount 1.0 --key-name mykey
platform-cli transfer import --from p --to c --key-name mykey
```

## P-Chain Operations

### Subnet Management

```bash
# Create a new subnet
platform-cli subnet create --network fuji --key-name mykey

# Convert subnet to L1
platform-cli subnet convert \
  --network fuji \
  --key-name mykey \
  --subnet-id "..." \
  --chain-id "..." \
  --validators "10.0.0.1,10.0.0.2"
```

### Chain Management

```bash
# Create a new chain on a subnet
platform-cli chain create \
  --network fuji \
  --key-name mykey \
  --subnet-id "..." \
  --genesis genesis.json \
  --name "mychain"
```

### Node Information

```bash
# Get node ID and BLS key
platform-cli node info --ip 127.0.0.1
```

## Key Loading Priority

1. `--private-key` flag (direct key input)
2. `--key-name` flag (load from keystore)
3. Default key from keystore (if set)
4. `AVALANCHE_PRIVATE_KEY` environment variable

For encrypted keys, set `PLATFORM_CLI_KEY_PASSWORD` environment variable or enter password when prompted.

## Private Key Formats

Supported formats:
- `PrivateKey-ewoqjP7PxY4yr3iLTp...` (Avalanche CB58)
- `0x56289e99c94b6912bfc12adc...` (Ethereum hex)
- Raw CB58 or hex strings
