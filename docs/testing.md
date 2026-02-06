# Testing

## Local Test Commands

```bash
# Full repository tests
go test ./...

# CI-style run with race detector and coverage output
go test -race -coverprofile=coverage.out ./...
```

## End-to-End Tests

```bash
# Build first
go build -o platform .

# Network e2e tests are opt-in to avoid accidental live transactions
# Set RUN_E2E_NETWORK_TESTS=1 when you intentionally want to run them

# Fuji (requires funded wallet)
RUN_E2E_NETWORK_TESTS=1 PRIVATE_KEY="PrivateKey-..." go test -v ./e2e/... -network=fuji

# Specific Fuji test
RUN_E2E_NETWORK_TESTS=1 PRIVATE_KEY="PrivateKey-..." go test -v ./e2e/... -network=fuji -run TestCreateSubnet

# Local network (uses ewoq key, RPC http://127.0.0.1:9650)
RUN_E2E_NETWORK_TESTS=1 go test -v ./e2e/... -network=local

# Help/validation-only subset (no funds needed)
go test -v ./e2e/... -run "Help|Params|MissingArgs"
```

## Integration-Tag Tests (`pkg/pchain`)

```bash
# Compile integration tests only (quick sanity check)
go test -tags=integration -run '^$' ./pkg/pchain/...

# Run integration tests (requires avalanchego binary path)
AVALANCHEGO_PATH=/path/to/avalanchego go test -tags=integration -v ./pkg/pchain/...
```

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
