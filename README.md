# platform

Minimal CLI for Avalanche P-Chain operations.

## Installation

```bash
go install github.com/ava-labs/platform-cli@latest
```

Or build from source:

```bash
git clone https://github.com/ava-labs/platform-cli.git
cd platform-cli
go build -o platform .
```

### Build with Ledger Support

To enable Ledger hardware wallet support, build with the `ledger` tag:

```bash
go build -tags ledger -o platform .
```

## Quick Start

```bash
# Generate a key
platform keys generate --name mykey

# Check your address
platform wallet address --key-name mykey

# Create a subnet
platform subnet create --network fuji --key-name mykey
```

## Ledger Hardware Wallet

Use `--ledger` flag with any command to sign transactions with your Ledger device.

**Requirements:**
- Ledger device connected via USB
- Device unlocked
- Avalanche app open
- Ledger Live closed

```bash
# Check Ledger address
platform wallet address --ledger

# Check balance
platform wallet balance --ledger --network fuji

# Send AVAX (requires confirmation on device)
platform transfer send --ledger --to <address> --amount 1.0

# Cross-chain transfer
platform transfer p-to-c --ledger --amount 0.5

# Create subnet
platform subnet create --ledger --network fuji

# Use different address index (default: 0)
platform wallet address --ledger --ledger-index 2
```

**Supported Operations:** All P-Chain and cross-chain operations support Ledger signing.

## Commands

### Key Management

```bash
platform keys generate --name <name> [--encrypt]
platform keys import --name <name> --private-key "PrivateKey-..."
platform keys list [--show-addresses]
platform keys export --name <name> [--format cb58|hex]
platform keys delete --name <name> [--force]
platform keys default [--name <name>]
```

### Wallet

```bash
platform wallet address
platform wallet balance
```

### Transfers

```bash
# P-Chain to P-Chain
platform transfer send --to <address> --amount <AVAX>

# Cross-chain (P <-> C)
platform transfer p-to-c --amount <AVAX>
platform transfer c-to-p --amount <AVAX>

# Manual export/import
platform transfer export --from p --to c --amount <AVAX>
platform transfer import --from p --to c
```

### Primary Network Staking

```bash
# Add validator (min 2000 AVAX, min 14 days)
platform validator add \
  --node-id NodeID-... \
  --bls-public-key <hex> \
  --bls-pop <hex> \
  --stake 2000 \
  --duration 336h \
  --delegation-fee 0.02

# Delegate to validator (min 25 AVAX)
platform validator delegate \
  --node-id NodeID-... \
  --stake 100 \
  --duration 336h
```

### Subnets

```bash
platform subnet create
platform subnet transfer-ownership --subnet-id <ID> --new-owner <address>
platform subnet convert-l1 --subnet-id <ID> --chain-id <ID> [--validators <IPs>]
```

### L1 Validators

```bash
platform l1 register-validator --balance <AVAX> --pop <hex> --message <hex>
platform l1 set-weight --message <hex>
platform l1 add-balance --validation-id <ID> --balance <AVAX>
platform l1 disable-validator --validation-id <ID>
```

### Chains

```bash
platform chain create --subnet-id <ID> --genesis <file> --name <name>
```

### Node Info

```bash
platform node info --ip <IP>
```

## Networks

| Network | Flag | RPC URL |
|---------|------|---------|
| Fuji | `--network fuji` | `https://api.avax-test.network` |
| Mainnet | `--network mainnet` | `https://api.avax.network` |

### Local / Custom RPC / Devnets

For local networks, devnets, or custom networks, use the `--rpc-url` flag:

```bash
# Connect to local network (avalanche-network-runner)
platform wallet balance --rpc-url http://127.0.0.1:9650 --key-name ewoq

# Connect to a custom devnet (auto-detects network ID)
platform wallet balance --rpc-url http://my-devnet:9650 --key-name mykey

# Specify network ID explicitly if auto-detection fails
platform wallet balance --rpc-url http://my-devnet:9650 --network-id 12345 --key-name mykey

# Works with all commands
platform subnet create --rpc-url http://my-devnet:9650 --key-name mykey
platform transfer send --rpc-url http://my-devnet:9650 --to <address> --amount 1.0
```

When using `--rpc-url`:
- Network ID is auto-detected from the node (queries `/ext/info` endpoint)
- If auto-detection fails (e.g., public RPCs without `/ext/info`), use `--network-id`
- Address HRP (Human-Readable Part) is derived from network ID
- Standard networks: `--network-id 1` (mainnet/avax), `--network-id 5` (fuji)

## Key Loading Priority

1. `--ledger` flag (hardware wallet)
2. `--private-key` flag
3. `--key-name` flag
4. Default key from keystore
5. `AVALANCHE_PRIVATE_KEY` env var

For encrypted keys: `PLATFORM_CLI_KEY_PASSWORD` env var or interactive prompt.

## Built-in Keys

- `ewoq` - Pre-funded test key for local networks

## Testing

```bash
# Build first
go build -o platform .

# Run e2e tests against Fuji (requires funded wallet)
PRIVATE_KEY="PrivateKey-..." go test -v ./e2e/... -network=fuji

# Run specific test
PRIVATE_KEY="PrivateKey-..." go test -v ./e2e/... -network=fuji -run TestCreateSubnet

# Run against local network (uses ewoq key, connects to http://127.0.0.1:9650)
go test -v ./e2e/... -network=local

# Run only help/validation tests (no funds needed)
go test -v ./e2e/... -run "Help|Params|MissingArgs"
```

### Test Categories

| Test | Description | Funds Required |
|------|-------------|----------------|
| `TestCLI*Help` | CLI help output | No |
| `Test*Params` | Parameter validation | No |
| `Test*MissingArgs` | Error handling | No |
| `TestWallet*` | Wallet creation | No |
| `TestPChainSend*` | P-Chain transfers | Yes |
| `TestCrossChain*` | P<->C transfers | Yes |
| `TestCreateSubnet*` | Subnet creation | Yes |
| `TestCreateChain*` | Chain creation | Yes |
| `TestSubnetLifecycle` | Full subnet flow | Yes |

## P-Chain Operations Reference

| Operation | Command | SDK Method |
|-----------|---------|------------|
| Send AVAX | `transfer send` | `IssueBaseTx` |
| Export | `transfer export` | `IssueExportTx` |
| Import | `transfer import` | `IssueImportTx` |
| Add Validator | `validator add` | `IssueAddPermissionlessValidatorTx` |
| Add Delegator | `validator delegate` | `IssueAddPermissionlessDelegatorTx` |
| Create Subnet | `subnet create` | `IssueCreateSubnetTx` |
| Transfer Subnet Ownership | `subnet transfer-ownership` | `IssueTransferSubnetOwnershipTx` |
| Convert to L1 | `subnet convert-l1` | `IssueConvertSubnetToL1Tx` |
| Register L1 Validator | `l1 register-validator` | `IssueRegisterL1ValidatorTx` |
| Set L1 Validator Weight | `l1 set-weight` | `IssueSetL1ValidatorWeightTx` |
| Increase L1 Balance | `l1 add-balance` | `IssueIncreaseL1ValidatorBalanceTx` |
| Disable L1 Validator | `l1 disable-validator` | `IssueDisableL1ValidatorTx` |
| Create Chain | `chain create` | `IssueCreateChainTx` |
