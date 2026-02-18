# Security Best Practices Review Report

Date: 2026-02-07  
Repository: `platform-cli`

## Executive Summary

I reviewed the Go CLI codebase with focus on key handling, endpoint validation, keystore storage, and CI supply-chain posture.

- Critical findings: 0
- High findings: 0
- Medium findings: 2
- Low findings: 4

The codebase already has strong controls (Argon2id + AES-GCM encryption, secure keystore write permissions, endpoint normalization with insecure-HTTP gating, and `govulncheck` in CI). The main remaining risk is secret exposure through CLI argument usage and URL credential handling.

## Medium Severity

### [SBP-001] Sensitive private keys can still be provided via process arguments

- Rule ID: `SBP-001`
- Severity: Medium
- Location: `cmd/root.go:63`, `cmd/wallet.go:104`, `docs/usage.md:41`, `docs/usage.md:146`
- Evidence:
  - Global `--private-key` flag remains available (`cmd/root.go:63`), even though deprecated (`cmd/root.go:70`).
  - Runtime key loading still accepts `--private-key` before environment fallback (`cmd/wallet.go:104`).
  - Docs still show key import via command-line private key argument (`docs/usage.md:41`).
- Impact: CLI arguments are commonly exposed through shell history, process listings, debugging output, and CI logs.
- Secure-by-default improvement:
  - Remove or hard-gate `--private-key` behind an explicit unsafe flag (for example `--unsafe-private-key-arg`).
  - Add safer non-interactive inputs (`--private-key-file`, `--private-key-stdin`).
  - Update docs to prefer prompt, keystore, or hardware wallet flows.

### [SBP-002] URL userinfo is accepted and may be printed, risking credential disclosure

- Rule ID: `SBP-002`
- Severity: Medium
- Location: `pkg/node/info.go:58`, `pkg/node/info.go:94`, `cmd/wallet.go:190`, `cmd/validator.go:106`, `pkg/network/network.go:73`
- Evidence:
  - URI normalization does not reject `userinfo` (username/password in URL), and returns `parsed.String()` (`pkg/node/info.go:58`, `pkg/node/info.go:94`).
  - Normalized URLs are printed to stdout in command flows (`cmd/wallet.go:190`, `cmd/validator.go:106`).
  - Error paths include full endpoint string (`pkg/network/network.go:73`).
- Impact: Embedded RPC credentials can be leaked to terminal output, logs, and CI artifacts.
- Secure-by-default improvement:
  - Reject URLs with `parsed.User != nil` during normalization.
  - Redact credentials from any endpoint string shown to users/logs.
  - Prefer auth headers or external credential providers over URL-embedded credentials.

## Low Severity

### [SBP-003] Password policy minimum for encrypted keys is weak (8 characters)

- Rule ID: `SBP-003`
- Severity: Low
- Location: `cmd/keys.go:113`, `cmd/keys.go:181`, `cmd/keys.go:467`
- Evidence:
  - Password validation enforces only length >= 8 for both environment and prompt paths.
- Impact: Weak user-selected passwords reduce practical resistance if keystore files are exfiltrated.
- Secure-by-default improvement:
  - Raise minimum to at least 12-14 characters.
  - Add passphrase quality guidance in prompts/docs.
  - Optionally add entropy/strength checks for interactive mode.

### [SBP-004] Built-in known `ewoq` test key is blocked only on mainnet

- Rule ID: `SBP-004`
- Severity: Low
- Location: `cmd/wallet.go:140`, `cmd/wallet.go:221`, `cmd/wallet.go:260`, `cmd/wallet.go:300`
- Evidence:
  - Guard prevents `ewoq` usage on mainnet, but allows use on other non-local networks.
- Impact: Users can accidentally operate with a publicly known key on shared/public test environments.
- Secure-by-default improvement:
  - Restrict `ewoq` to local loopback/dev contexts by default.
  - Require explicit override flag for any non-local usage.

### [SBP-005] Existing key/index file permissions are not validated on load

- Rule ID: `SBP-005`
- Severity: Low
- Location: `pkg/keystore/keystore.go:117`, `pkg/keystore/keystore.go:165`, `pkg/keystore/keystore.go:325`
- Evidence:
  - Reads enforce file type/size but do not enforce restrictive mode bits for existing keystore files.
- Impact: If permissions drift (for example `0644`), sensitive material may become readable by other local users.
- Secure-by-default improvement:
  - Validate key/index file mode on read.
  - Refuse unsafe modes or auto-repair to `0600` with warning.

### [SBP-006] GitHub Actions supply-chain hardening gaps

- Rule ID: `SBP-006`
- Severity: Low
- Location: `.github/workflows/ci.yml:14`, `.github/workflows/ci.yml:17`, `.github/workflows/ci.yml:79`, `.github/workflows/pr.yml:12`, `.github/workflows/pr.yml:17`, `.github/workflows/nightly-network-e2e.yml:18`, `.github/workflows/nightly-network-e2e.yml:21`
- Evidence:
  - Third-party actions are pinned by mutable tags (for example `@v4`, `@v5`) rather than immutable commit SHAs.
  - Workflows do not explicitly define least-privilege `permissions`.
- Impact: Higher risk from action supply-chain compromise and broader-than-needed token scope.
- Secure-by-default improvement:
  - Pin third-party actions to full commit SHAs.
  - Add explicit minimal `permissions` at workflow/job scope.

## Positive Security Controls Observed

- Keystore encryption uses Argon2id + AES-256-GCM (`pkg/keystore/crypto.go:22`, `pkg/keystore/crypto.go:55`).
- Key material files are written atomically with restrictive perms (`pkg/keystore/keystore.go:62`, `pkg/keystore/keystore.go:194`, `pkg/keystore/keystore.go:252`).
- Keystore directory permissions are actively enforced (`pkg/keystore/keystore.go:155`, `pkg/keystore/keystore.go:159`).
- Node/custom RPC normalization blocks insecure non-local HTTP by default (`pkg/node/info.go:87`).
- CI includes `govulncheck` (`.github/workflows/ci.yml:39`).
- `keys export` defaults to file output with `0600`, requiring explicit unsafe stdout opt-in (`cmd/keys.go:295`, `cmd/keys.go:353`).

## Recommended Remediation Order

1. Remove/hard-gate `--private-key` argument path and update docs (`SBP-001`).
2. Reject/redact URL credentials in endpoint handling (`SBP-002`).
3. Tighten password defaults and known-test-key scope (`SBP-003`, `SBP-004`).
4. Enforce permission checks on existing keystore files (`SBP-005`).
5. Harden GitHub Actions pinning and token permissions (`SBP-006`).
