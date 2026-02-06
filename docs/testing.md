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

# Fuji (requires funded wallet)
PRIVATE_KEY="PrivateKey-..." go test -v ./e2e/... -network=fuji

# Specific Fuji test
PRIVATE_KEY="PrivateKey-..." go test -v ./e2e/... -network=fuji -run TestCreateSubnet

# Local network (uses ewoq key, RPC http://127.0.0.1:9650)
go test -v ./e2e/... -network=local

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
