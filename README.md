# platform

Minimal CLI for Avalanche P-Chain operations.

## Installation

Requirements:
- Go 1.24.13+

```bash
go install github.com/ava-labs/platform-cli@latest
```

Or build from source:

```bash
git clone https://github.com/ava-labs/platform-cli.git
cd platform-cli
go build -o platform .
```

For Ledger support, build with:

```bash
go build -tags ledger -o platform .
```

## Quick Start

```bash
# Generate a key
platform keys generate --name mykey

# Check your address
platform wallet address --key-name mykey

# Check balance on Fuji
platform wallet balance --network fuji --key-name mykey
```

## Documentation

Detailed docs live in [`docs/`](docs/README.md):

- [Usage and Commands](docs/usage.md)
- [Networks and Custom RPC](docs/networks.md)
- [Testing](docs/testing.md)
- [P-Chain Operations Reference](docs/pchain-operations.md)

## CLI Help

```bash
platform --help
platform <command> --help
```
