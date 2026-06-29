# platform-cli

Minimal CLI for Avalanche P-Chain operations.

## Installation

Requirements:
- Go 1.24.13+

```bash
go install github.com/ava-labs/platform-cli@main
```

Or build from source:

```bash
git clone https://github.com/ava-labs/platform-cli.git
cd platform-cli
go build -o platform-cli .
```

For Ledger support, build with:

```bash
go build -tags ledger -o platform-cli .
```

## Quick Start

```bash
# Generate a key
platform-cli keys generate --name mykey

# Check your address
platform-cli wallet address --key-name mykey

# Check balance on Fuji
platform-cli wallet balance --network fuji --key-name mykey
```

## Documentation

Detailed docs live in [`docs/`](docs/README.md):

- [Usage and Commands](docs/usage.md)
- [Networks and Custom RPC](docs/networks.md)
- [Testing](docs/testing.md)
- [P-Chain Operations Reference](docs/pchain-operations.md)

## CLI Help

```bash
platform-cli --help
platform-cli <command> --help
```
