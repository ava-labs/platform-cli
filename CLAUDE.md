# CLAUDE.md

This file provides context for AI assistants working on this codebase.

## Project Overview

**Platform CLI** is a minimal command-line interface for Avalanche P-Chain operations. It provides functionality for:
- Key management (generate, import, export, encrypt)
- P-Chain transfers and cross-chain (P↔C) transfers
- Primary network staking (validators and delegators)
- Subnet creation and L1 conversion
- L1 validator management
- Chain creation on subnets

## Architecture

```
cmd/               - Cobra CLI commands (user-facing interface)
├── root.go        - Root command, global flags (--network, --private-key, --key-name)
├── keys.go        - Key management: generate, import, export, delete, default
├── wallet.go      - Wallet info: address, balance
├── transfer.go    - Transfers: send, p-to-c, c-to-p, export, import
├── validator.go   - Staking: add-permissionless, add-permissionless-delegator
├── subnet.go      - Subnets: create, transfer-ownership, convert-to-l1, add-validator
├── l1.go          - L1 validators: register-validator, set-validator-weight, increase-validator-balance, disable-validator
├── chain.go       - Chains: create chain on subnet
└── node.go        - Node utilities: info

pkg/               - Core business logic (importable as library)
├── pchain/        - P-Chain transaction building (DO NOT modify crypto)
├── wallet/        - Wallet abstraction over avalanchego SDK
├── keystore/      - Encrypted key storage (SECURITY CRITICAL)
├── crosschain/    - P↔C chain transfer logic with retry
├── network/       - Network configuration (local, fuji, mainnet)
└── node/          - Node info utilities

e2e/               - End-to-end tests (run against real networks)
```

## Critical Rules for AI Development

### Security Rules (MUST follow)
1. **NEVER modify cryptographic code** in `pkg/keystore/` without explicit security review
2. **NEVER commit private keys**, test credentials, or secrets
3. **NEVER log or print private keys** - even in debug/error messages
4. **NEVER hardcode** network URLs, timeouts, or magic numbers - use constants/config
5. **ALWAYS validate** user input before passing to transaction builders

### Code Quality Rules
6. **ALWAYS run `go vet ./...` and `staticcheck ./...`** before committing
7. **ALWAYS add tests** for new functionality - unit tests preferred, e2e for integration
8. **ALWAYS wrap errors** with context: `fmt.Errorf("failed to X: %w", err)`
9. **NEVER use `panic()`** in library code - return errors instead
10. **FOLLOW existing patterns** - check similar code before writing new code

### Domain Knowledge
- Transaction amounts are in **nAVAX** (1 AVAX = 1,000,000,000 nAVAX)
- Address formats differ: P-Chain uses `P-avax1...`, C-Chain uses `0x...`
- Subnet IDs and Chain IDs are different concepts (subnet contains chains)
- L1 validators require BLS keys (not just secp256k1)
- The `ewoq` key is a well-known test key - NEVER use on mainnet

## Build & Test Commands

```bash
# Build
go build -o platform .

# Lint & vet (run before committing)
go vet ./...
staticcheck ./...

# Unit tests (when they exist)
go test ./pkg/...

# E2E tests - no funds required
go test -v ./e2e/... -run "Help|Params|MissingArgs"

# E2E tests - local network (uses ewoq key)
go test -v ./e2e/... -network=local

# E2E tests - Fuji testnet (requires funded wallet)
PRIVATE_KEY="PrivateKey-..." go test -v ./e2e/... -network=fuji
```

## Common Pitfalls

1. **Address derivation differs between chains** - P-Chain and C-Chain derive different addresses from the same private key
2. **Cross-chain transfers are two-step** - export then import, with network confirmation between
3. **Subnet creation requires P-Chain balance** - not C-Chain
4. **L1 conversion is irreversible** - converts permissioned subnet to L1
5. **Stake duration has minimums** - 14 days for validators, 14 days for delegators on mainnet
6. **Delegation fee is a percentage** - 0.02 = 2%, not 2 AVAX

## Key Files to Understand

| File | Purpose | Modification Risk |
|------|---------|-------------------|
| `pkg/keystore/keystore.go` | Key encryption/storage | HIGH - security critical |
| `pkg/pchain/pchain.go` | Transaction building | MEDIUM - affects all ops |
| `pkg/wallet/wallet.go` | Wallet interface | MEDIUM - core abstraction |
| `cmd/root.go` | Global flags | LOW - UI only |
| `pkg/network/network.go` | Network config | LOW - constants only |

## Dependencies

Core dependencies (from go.mod):
- `github.com/ava-labs/avalanchego` - Avalanche node SDK
- `github.com/ava-labs/libevm` - EVM/C-Chain operations
- `github.com/spf13/cobra` - CLI framework
- `golang.org/x/crypto` - Cryptographic primitives

## Network Configuration

| Network | Chain ID | Min Stake (Validator) | Min Stake (Delegator) |
|---------|----------|----------------------|----------------------|
| Local | 1337 | 1 AVAX | 1 AVAX |
| Fuji | 5 | 1 AVAX | 1 AVAX |
| Mainnet | 1 | 2000 AVAX | 25 AVAX |

## Error Handling Patterns

```go
// Good - wrap with context
if err != nil {
    return fmt.Errorf("failed to create subnet: %w", err)
}

// Bad - loses context
if err != nil {
    return err
}

// Bad - panics in library code
if err != nil {
    panic(err)
}
```

## Transaction Flow Example

```
User runs: platform subnet create --network fuji

1. cmd/subnet.go: Parse flags, load key
2. pkg/wallet/wallet.go: Create wallet from private key
3. pkg/pchain/pchain.go: Build CreateSubnetTx
4. avalanchego SDK: Sign and submit transaction
5. cmd/subnet.go: Print result (subnet ID)
```
