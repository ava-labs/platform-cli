# Testing

## Local Test Commands

```bash
# Fast default tests (unit and package-level tests)
# Note: CLI and network e2e tests are behind build tags
go test ./...

# CI-style run with race detector and coverage output
go test -race -coverprofile=coverage.out ./...

# CLI smoke e2e tests (help/arg validation, no live funds)
go test -tags=clie2e -v ./e2e/... -run "Help|Params|MissingArgs"
```

## End-to-End Tests

```bash
# Build first
go build -o platform .

# Network e2e tests are opt-in to avoid accidental live transactions
# Set RUN_E2E_NETWORK_TESTS=1 when you intentionally want to run them

# Compile network e2e tests (quick sanity check)
go test -tags=networke2e -run '^$' ./e2e/...

# Fuji (requires funded wallet)
RUN_E2E_NETWORK_TESTS=1 PRIVATE_KEY="PrivateKey-..." go test -tags=networke2e -v ./e2e/... -network=fuji

# Specific Fuji test
RUN_E2E_NETWORK_TESTS=1 PRIVATE_KEY="PrivateKey-..." go test -tags=networke2e -v ./e2e/... -network=fuji -run TestCreateSubnet

# Local network (uses ewoq key, RPC http://127.0.0.1:9650)
RUN_E2E_NETWORK_TESTS=1 go test -tags=networke2e -v ./e2e/... -network=local

# Help/validation-only subset (no funds needed)
go test -tags=clie2e -v ./e2e/... -run "Help|Params|MissingArgs"
```

## Integration-Tag Tests (`pkg/pchain`)

```bash
# Compile integration tests only (quick sanity check)
go test -tags=integration -run '^$' ./pkg/pchain/...

# Run integration tests (requires avalanchego binary path)
AVALANCHEGO_PATH=/path/to/avalanchego go test -tags=integration -v ./pkg/pchain/...
```

## Nightly Network E2E (GitHub Actions)

`nightly-network-e2e.yml` runs the Fuji network suite nightly and on manual dispatch.
It requires the `E2E_FUJI_PRIVATE_KEY` repository secret to be configured.
The nightly job runs a low-spend smoke subset and uses small transfer amounts.

## Test Categories

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
