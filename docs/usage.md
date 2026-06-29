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
platform-cli wallet address --ledger

# Check balance
platform-cli wallet balance --ledger --network fuji

# Send AVAX (requires device confirmation)
platform-cli transfer send --ledger --to <address> --amount 1.0

# Cross-chain transfer
platform-cli transfer p-to-c --ledger --amount 0.5

# Create subnet
platform-cli subnet create --ledger --network fuji

# Use a different address index (default: 0)
platform-cli wallet address --ledger --ledger-index 2
```

## Command Reference

### Key Management

```bash
platform-cli keys generate --name <name> [--encrypt]
platform-cli keys import --name <name> --private-key "PrivateKey-..."
platform-cli keys list [--show-addresses]
platform-cli keys export --name <name> --output-file <path> [--format cb58|hex]
platform-cli keys export --name <name> --unsafe-stdout [--format cb58|hex]  # discouraged
platform-cli keys delete --name <name> [--force]
platform-cli keys default [--name <name>]
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

### Primary Network Staking

```bash
# Add validator (mainnet minimum: 2000 AVAX, 14 days)
platform-cli validator add-permissionless \
  --node-id NodeID-... \
  --bls-public-key <hex> \
  --bls-pop <hex> \
  --stake 2000 \
  --duration 336h \
  --delegation-fee 0.02

# Delegate to validator (mainnet minimum: 25 AVAX)
platform-cli validator add-permissionless-delegator \
  --node-id NodeID-... \
  --stake 100 \
  --duration 336h
```

> **Breaking (v2.0.0):** command names now mirror the avalanchego transaction
> they issue, and the old names were removed (no aliases):
> `validator add` → `validator add-permissionless`,
> `validator delegate` → `validator add-permissionless-delegator`,
> `subnet convert-l1` → `subnet convert-to-l1`,
> `l1 set-weight` → `l1 set-validator-weight`,
> `l1 add-balance` → `l1 increase-validator-balance`.

### Subnets

```bash
platform-cli subnet create
platform-cli subnet transfer-ownership --subnet-id <ID> --new-owner <address>
platform-cli subnet convert-to-l1 --subnet-id <ID> --chain-id <manager-chain-id> --validators <nodes> [--manager <hex>]
platform-cli subnet convert-to-l1 --subnet-id <ID> --chain-id <manager-chain-id> --validators <nodes> [--contract-address <hex>]
platform-cli subnet convert-to-l1 --subnet-id <ID> --chain-id <manager-chain-id> \
  --validator-node-ids NodeID-...,NodeID-... \
  --validator-bls-public-keys <hex>,<hex> \
  --validator-bls-pops <hex>,<hex> \
  [--manager <hex>]
platform-cli subnet convert-to-l1 --subnet-id <ID> --chain-id <manager-chain-id> --mock-validator
platform-cli subnet add-validator --subnet-id <ID> --node-id NodeID-... --weight <uint> [--start <RFC3339|now>] [--duration <dur>]
```

`add-validator` notes:
- Adds a validator to a **permissioned** subnet (`AddSubnetValidatorTx`).
- The node must already validate the primary network, and the validation period
  must fall within its primary network validation window.
- The subnet owner key authorizes the tx (subnet auth), so load the owner key via
  `--key-name` or `--ledger`.

`convert-to-l1` notes:
- `--manager` / `--contract-address` is the validator manager contract address (hex).
- `--chain-id` is the chain where the validator manager contract is deployed.
  In many setups, this is the same as the new L1 chain ID.
- `--validators` accepts comma-separated node addresses (`IP`, `host:port`, or base `http(s)://host:port` URI).
  Non-local shorthand addresses default to `https://`.
- Plain `http://` for non-local validator/node endpoints is blocked by default.
  Use `--allow-insecure-http` only on trusted networks.
- For each validator address, the CLI auto-queries `/ext/info` and reads:
  - `NodeID`
  - BLS public key + proof of possession (PoP)
- If validator info endpoints are not reachable, use manual flags:
  - `--validator-node-ids`
  - `--validator-bls-public-keys`
  - `--validator-bls-pops`

### L1 Validators

```bash
platform-cli l1 register-validator --balance <AVAX> --pop <hex> --message <hex>   # balance > 0
platform-cli l1 set-validator-weight --message <hex>
platform-cli l1 increase-validator-balance --validation-id <ID> --balance <AVAX>   # balance > 0
platform-cli l1 disable-validator --validation-id <ID>
```

### Chains

```bash
platform-cli chain create --subnet-id <ID> --genesis <file> --name <name>
```

### Node Info

```bash
platform-cli node info --ip <IP-or-URI> [--allow-insecure-http]
```

## Key Loading Priority

1. `--ledger`
2. `--key-name`
3. `--private-key` (deprecated; prefer keystore/Ledger)
4. Default key from keystore
5. `AVALANCHE_PRIVATE_KEY`

For encrypted keys, use `PLATFORM_CLI_KEY_PASSWORD` or the interactive prompt.

## Built-in Keys

- `ewoq` (pre-funded test key for local networks)
