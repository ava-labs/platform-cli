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

## Quick Start

```bash
# Generate a key
platform keys generate mykey

# Check your address
platform wallet address --key-name mykey

# Create a subnet
platform subnet create --network fuji --key-name mykey
```

## Commands

### Key Management

```bash
platform keys generate <name> [--encrypt]
platform keys import <name> --private-key "PrivateKey-..."
platform keys list [--show-addresses]
platform keys export <name> [--format cb58|hex]
platform keys delete <name> [--force]
platform keys default [<name>]
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
platform l1 add-balance --validation-id <ID> --amount <AVAX>
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
| Local | `--network local` | `http://127.0.0.1:9650` |
| Fuji | `--network fuji` | `https://api.avax-test.network` |
| Mainnet | `--network mainnet` | `https://api.avax.network` |

## Key Loading Priority

1. `--private-key` flag
2. `--key-name` flag
3. Default key from keystore
4. `AVALANCHE_PRIVATE_KEY` env var

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

# Run against local network (uses ewoq key)
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
| Add Validator | `validator add` | `IssueAddValidatorTx` |
| Add Delegator | `validator delegate` | `IssueAddDelegatorTx` |
| Create Subnet | `subnet create` | `IssueCreateSubnetTx` |
| Transfer Subnet Ownership | `subnet transfer-ownership` | `IssueTransferSubnetOwnershipTx` |
| Convert to L1 | `subnet convert-l1` | `IssueConvertSubnetToL1Tx` |
| Register L1 Validator | `l1 register-validator` | `IssueRegisterL1ValidatorTx` |
| Set L1 Validator Weight | `l1 set-weight` | `IssueSetL1ValidatorWeightTx` |
| Increase L1 Balance | `l1 add-balance` | `IssueIncreaseL1ValidatorBalanceTx` |
| Disable L1 Validator | `l1 disable-validator` | `IssueDisableL1ValidatorTx` |
| Create Chain | `chain create` | `IssueCreateChainTx` |
