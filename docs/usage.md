# Usage and Commands

## Ledger Hardware Wallet

Use `--ledger` with commands that require signing.

Requirements:
- Ledger device connected via USB
- Device unlocked
- Avalanche app open
- Ledger Live closed

Examples:

```bash
# Check Ledger address
platform wallet address --ledger

# Check balance
platform wallet balance --ledger --network fuji

# Send AVAX (requires device confirmation)
platform transfer send --ledger --to <address> --amount 1.0

# Cross-chain transfer
platform transfer p-to-c --ledger --amount 0.5

# Create subnet
platform subnet create --ledger --network fuji

# Use a different address index (default: 0)
platform wallet address --ledger --ledger-index 2
```

## Command Reference

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
# Add validator (mainnet minimum: 2000 AVAX, 14 days)
platform validator add \
  --node-id NodeID-... \
  --bls-public-key <hex> \
  --bls-pop <hex> \
  --stake 2000 \
  --duration 336h \
  --delegation-fee 0.02

# Delegate to validator (mainnet minimum: 25 AVAX)
platform validator delegate \
  --node-id NodeID-... \
  --stake 100 \
  --duration 336h
```

### Subnets

```bash
platform subnet create
platform subnet transfer-ownership --subnet-id <ID> --new-owner <address>
platform subnet convert-l1 --subnet-id <ID> --chain-id <manager-chain-id> --validators <nodes> [--manager <hex>]
platform subnet convert-l1 --subnet-id <ID> --chain-id <manager-chain-id> --validators <nodes> [--contract-address <hex>]
platform subnet convert-l1 --subnet-id <ID> --chain-id <manager-chain-id> \
  --validator-node-ids NodeID-...,NodeID-... \
  --validator-bls-public-keys <hex>,<hex> \
  --validator-bls-pops <hex>,<hex> \
  [--manager <hex>]
platform subnet convert-l1 --subnet-id <ID> --chain-id <manager-chain-id> --mock-validator
```

`convert-l1` notes:
- `--manager` / `--contract-address` is the validator manager contract address (hex).
- `--chain-id` is the chain where the validator manager contract is deployed.
  In many setups, this is the same as the new L1 chain ID.
- `--validators` accepts comma-separated node addresses (`IP`, `host:port`, or full `http(s)://...` URI).
- For each validator address, the CLI auto-queries `/ext/info` and reads:
  - `NodeID`
  - BLS public key + proof of possession (PoP)
- If validator info endpoints are not reachable, use manual flags:
  - `--validator-node-ids`
  - `--validator-bls-public-keys`
  - `--validator-bls-pops`

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

## Key Loading Priority

1. `--ledger`
2. `--private-key`
3. `--key-name`
4. Default key from keystore
5. `AVALANCHE_PRIVATE_KEY`

For encrypted keys, use `PLATFORM_CLI_KEY_PASSWORD` or the interactive prompt.

## Built-in Keys

- `ewoq` (pre-funded test key for local networks)
