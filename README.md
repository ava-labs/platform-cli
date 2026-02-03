# platform-cli

Command-line utilities for Avalanche P-Chain operations.

## Overview

`platform-cli` provides a set of commands for working with the Avalanche P-Chain:

- **wallet** - Wallet operations (balance, address)
- **subnet** - Subnet management (create, convert)
- **chain** - Chain management (create)
- **node** - Node information (info)

This tool can be used standalone or as a library by other tools like `create-l1`.

## Installation

```bash
# Install directly
go install github.com/ava-labs/platform-cli@latest

# Or clone and build
git clone https://github.com/ava-labs/platform-cli.git
cd platform-cli
go build -o platform-cli .

# With Ledger support
go build -tags ledger -o platform-cli .
```

## Usage

### Wallet Commands

```bash
# Show wallet addresses
platform-cli wallet address --private-key "PrivateKey-..."

# Check P-Chain balance
platform-cli wallet balance --private-key "PrivateKey-..." --network fuji
```

### Node Commands

```bash
# Get node ID and BLS key
platform-cli node info --ip 127.0.0.1
```

### Subnet Commands

```bash
# Create a new subnet
platform-cli subnet create --network fuji --private-key "PrivateKey-..."

# Convert subnet to L1
platform-cli subnet convert \
  --network fuji \
  --private-key "PrivateKey-..." \
  --subnet-id "..." \
  --chain-id "..." \
  --validators "10.0.0.1,10.0.0.2"
```

### Chain Commands

```bash
# Create a new chain on a subnet
platform-cli chain create \
  --network fuji \
  --private-key "PrivateKey-..." \
  --subnet-id "..." \
  --genesis genesis.json \
  --name "mychain"
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `AVALANCHE_PRIVATE_KEY` | P-Chain private key (alternative to --private-key flag) |

## Private Key Formats

The following private key formats are supported:

- `PrivateKey-ewoqjP7PxY4yr3iLTp...` (Avalanche CB58 format)
- `0x56289e99c94b6912bfc12adc...` (Ethereum hex format)
- Raw CB58 or hex strings

## Using as a Library

```go
import (
    "github.com/ava-labs/platform-cli/pkg/wallet"
    "github.com/ava-labs/platform-cli/pkg/pchain"
    "github.com/ava-labs/platform-cli/pkg/network"
    "github.com/ava-labs/platform-cli/pkg/node"
)

// Parse a private key
keyBytes, _ := wallet.ParsePrivateKey("PrivateKey-...")
key, _ := wallet.ToPrivateKey(keyBytes)

// Create a wallet
config := network.GetConfig("fuji")
w, _ := wallet.NewWallet(ctx, key, config)

// Create a subnet
subnetID, _ := pchain.CreateSubnet(ctx, w)

// Get node info
info, _ := node.GetNodeInfo(ctx, "10.0.0.1")
```
