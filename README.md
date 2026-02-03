# platform-cli

Minimal CLI for Avalanche P-Chain operations.

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

## Quick Start

```bash
# Generate a key
platform-cli keys generate mykey

# Check your address
platform-cli wallet address --key-name mykey

# Create a subnet
platform-cli subnet create --network fuji --key-name mykey
```

## Commands

### Key Management

```bash
platform-cli keys generate <name> [--encrypt]
platform-cli keys import <name> --private-key "PrivateKey-..."
platform-cli keys list [--show-addresses]
platform-cli keys export <name> [--format cb58|hex]
platform-cli keys delete <name> [--force]
platform-cli keys default [<name>]
```

### Wallet

```bash
platform-cli wallet address
platform-cli wallet balance
```

### Transfers

```bash
# P-Chain to P-Chain
platform-cli transfer send --to <address> --amount <AVAX>

# Cross-chain (P <-> C)
platform-cli transfer p-to-c --amount <AVAX>
platform-cli transfer c-to-p --amount <AVAX>

# Manual export/import
platform-cli transfer export --from p --to c --amount <AVAX>
platform-cli transfer import --from p --to c
```

### Validators

```bash
# Primary network
platform-cli validator add --node-id <ID> --stake <AVAX> --duration 336h
platform-cli validator delegate --node-id <ID> --stake <AVAX> --duration 336h

# Subnet validators (permissioned)
platform-cli validator add-subnet --node-id <ID> --subnet-id <ID> --duration 336h
platform-cli validator remove-subnet --node-id <ID> --subnet-id <ID>

# Elastic subnet validators (permissionless)
platform-cli validator add-permissionless --node-id <ID> --subnet-id <ID> --asset-id <ID> --stake <amount>
platform-cli validator delegate-permissionless --node-id <ID> --subnet-id <ID> --asset-id <ID> --stake <amount>
```

### Subnets

```bash
platform-cli subnet create
platform-cli subnet transfer-ownership --subnet-id <ID> --new-owner <address>
platform-cli subnet convert-l1 --subnet-id <ID> --chain-id <ID> [--validators <IPs>]
```

### L1 Validators

```bash
platform-cli l1 register-validator --balance <AVAX> --pop <hex> --message <hex>
platform-cli l1 set-weight --message <hex>
platform-cli l1 add-balance --validation-id <ID> --amount <AVAX>
platform-cli l1 disable-validator --validation-id <ID>
```

### Chains

```bash
platform-cli chain create --subnet-id <ID> --genesis <file> --name <name>
```

### Node Info

```bash
platform-cli node info --ip <IP>
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
# Run e2e tests against local network
go test -v ./e2e/... -network=local

# Run e2e tests against Fuji
go test -v ./e2e/... -network=fuji

# Skip e2e tests
go test -v ./e2e/... -skip-e2e
```

## P-Chain Operations Reference

| Operation | Command | SDK Method |
|-----------|---------|------------|
| Send AVAX | `transfer send` | `IssueBaseTx` |
| Export | `transfer export` | `IssueExportTx` |
| Import | `transfer import` | `IssueImportTx` |
| Add Validator | `validator add` | `IssueAddValidatorTx` |
| Add Delegator | `validator delegate` | `IssueAddDelegatorTx` |
| Add Subnet Validator | `validator add-subnet` | `IssueAddSubnetValidatorTx` |
| Remove Subnet Validator | `validator remove-subnet` | `IssueRemoveSubnetValidatorTx` |
| Add Permissionless Validator | `validator add-permissionless` | `IssueAddPermissionlessValidatorTx` |
| Add Permissionless Delegator | `validator delegate-permissionless` | `IssueAddPermissionlessDelegatorTx` |
| Create Subnet | `subnet create` | `IssueCreateSubnetTx` |
| Transfer Subnet Ownership | `subnet transfer-ownership` | `IssueTransferSubnetOwnershipTx` |
| Convert to L1 | `subnet convert-l1` | `IssueConvertSubnetToL1Tx` |
| Register L1 Validator | `l1 register-validator` | `IssueRegisterL1ValidatorTx` |
| Set L1 Validator Weight | `l1 set-weight` | `IssueSetL1ValidatorWeightTx` |
| Increase L1 Balance | `l1 add-balance` | `IssueIncreaseL1ValidatorBalanceTx` |
| Disable L1 Validator | `l1 disable-validator` | `IssueDisableL1ValidatorTx` |
| Create Chain | `chain create` | `IssueCreateChainTx` |
