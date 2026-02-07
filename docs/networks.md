# Networks and Custom RPC

## Built-in Networks

| Network | Flag | RPC URL |
|---------|------|---------|
| Fuji | `--network fuji` | `https://api.avax-test.network` |
| Mainnet | `--network mainnet` | `https://api.avax.network` |

## Local and Custom RPC

For local networks, devnets, or custom RPC endpoints, use `--rpc-url`:

```bash
# Local network (e.g. avalanche-network-runner)
platform wallet balance --rpc-url http://127.0.0.1:9650 --key-name ewoq

# Custom devnet (auto-detects network ID)
platform wallet balance --rpc-url https://my-devnet:9650 --key-name mykey

# Explicit network ID if auto-detection fails
platform wallet balance --rpc-url https://my-devnet:9650 --network-id 12345 --key-name mykey

# Works across commands
platform subnet create --rpc-url https://my-devnet:9650 --key-name mykey
platform transfer send --rpc-url https://my-devnet:9650 --to <address> --amount 1.0

# Non-local HTTP is blocked by default; enable only on trusted networks
platform wallet balance --rpc-url http://my-devnet:9650 --allow-insecure-http --key-name mykey
```

When using `--rpc-url`:
- Non-local `http://` endpoints are rejected unless `--allow-insecure-http` is set.
- Network ID is auto-detected from `/ext/info` when available.
- Use `--network-id` if auto-detection is unavailable.
- Address HRP is derived from network ID.
- Common IDs: `1` (mainnet / `avax`), `5` (fuji).
