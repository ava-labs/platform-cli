#!/usr/bin/env bash
# Fuji rehearsal of the offline Warp rotation (warp plan -> sign -> rotate).
#
# Creates a throwaway L1 with 3 validators that have EMPTY deactivation and
# remaining balance owners, rotates all 3 to threshold-1 owners with offline
# BLS signatures, and asserts the result. The validators are BLS keys and node
# IDs made locally: no nodes run, so the L1 chain never produces blocks. That
# is enough, because only the P-Chain state matters here.
#
# Usage:
#   KEY_NAME=<funded Fuji key> scripts/warp-rehearsal-fuji.sh
#
# Environment:
#   KEY_NAME                   keystore key that pays every fee and balance (required)
#   PLATFORM_CLI_KEY_PASSWORD  password of KEY_NAME, if it is encrypted
#   BALANCE                    AVAX balance per validator (default 0.1)
#   KEEP_L1=1                  keep the validators at the end (default: disable them,
#                              which refunds their balance and proves the new owner works)
#   WORK_DIR                   working directory (default: a new temp directory)
#
# Cost: about 6 x BALANCE AVAX plus fees; the disable step refunds most of it.
set -euo pipefail

readonly RPC="https://api.avax-test.network"
readonly MANAGER_ADDRESS="0x00000000000000000000000000000000000c0de0"
readonly NUM_VALIDATORS=3
readonly WEIGHT=100
readonly POLL_SECONDS=3
readonly POLL_ATTEMPTS=60

BALANCE="${BALANCE:-0.1}"
REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
WORK_DIR="${WORK_DIR:-$(mktemp -d -t warp-rehearsal)}"
PLATFORM="$WORK_DIR/platform"

# require_bin exits unless every named binary is on PATH.
require_bin() {
	local bin
	for bin in "$@"; do
		command -v "$bin" >/dev/null 2>&1 || { echo "missing required binary: $bin" >&2; exit 1; }
	done
}

# log prints a step header.
log() {
	printf '\n==> %s\n' "$*"
}

# fail prints an assertion failure and exits.
fail() {
	echo "ASSERTION FAILED: $*" >&2
	exit 1
}

# pcall sends a P-Chain JSON-RPC call and prints the JSON reply.
pcall() {
	local method="$1" params="$2"
	curl -sS -X POST -H 'content-type: application/json' "$RPC/ext/bc/P" \
		-d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"$method\",\"params\":$params}"
}

# platform runs the CLI on Fuji with the fee payer key.
platform() {
	"$PLATFORM" --network fuji --key-name "$KEY_NAME" "$@"
}

# l1_validators prints "<validationID> <nodeID> <deactivation threshold>" for
# every L1 validator of the subnet.
l1_validators() {
	pcall platform.getCurrentValidators "{\"subnetID\":\"$SUBNET_ID\"}" |
		jq -r '.result.validators[] | "\(.validationID) \(.nodeID) \(.deactivationOwner.threshold // "0")"'
}

# wait_for_validators waits until the subnet has the expected number of L1
# validators.
wait_for_validators() {
	local want="$1" n i
	for ((i = 0; i < POLL_ATTEMPTS; i++)); do
		n="$(l1_validators | wc -l | tr -d ' ')"
		[[ "$n" == "$want" ]] && return 0
		sleep "$POLL_SECONDS"
	done
	fail "subnet $SUBNET_ID has $n L1 validators, want $want"
}

main() {
	require_bin go jq curl
	[[ -n "${KEY_NAME:-}" ]] || { echo "set KEY_NAME to a funded Fuji keystore key" >&2; exit 1; }
	echo "Work directory: $WORK_DIR"

	log "Build the CLI"
	(cd "$REPO_DIR" && go build -o "$PLATFORM" .)

	ADDRESS="$(platform wallet address | grep -o 'P-fuji1[0-9a-z]*' | head -1)"
	[[ -n "$ADDRESS" ]] || fail "could not read the fee payer address"
	echo "Fee payer and new owner: $ADDRESS"

	log "Create $NUM_VALIDATORS throwaway validator identities"
	(cd "$REPO_DIR" && go run ./scripts/warp-testkeys "$WORK_DIR/keys" "$NUM_VALIDATORS") >"$WORK_DIR/validators.txt"
	cat "$WORK_DIR/validators.txt"
	local node_ids bls_keys bls_pops weights
	node_ids="$(awk '{print $1}' "$WORK_DIR/validators.txt" | paste -sd, -)"
	bls_keys="$(awk '{print $2}' "$WORK_DIR/validators.txt" | paste -sd, -)"
	bls_pops="$(awk '{print $3}' "$WORK_DIR/validators.txt" | paste -sd, -)"
	weights="$(yes "$WEIGHT" | head -n "$NUM_VALIDATORS" | paste -sd, -)"

	log "Create the subnet and the manager chain"
	SUBNET_ID="$(platform subnet create | awk '/^Subnet ID:/ {print $3}')"
	[[ -n "$SUBNET_ID" ]] || fail "subnet create printed no subnet ID"
	echo '{"config":{"chainId":99999}}' >"$WORK_DIR/genesis.json"
	CHAIN_ID="$(platform chain create --subnet-id "$SUBNET_ID" --name warprehearsal --genesis "$WORK_DIR/genesis.json" | awk '/^Chain ID:/ {print $3}')"
	[[ -n "$CHAIN_ID" ]] || fail "chain create printed no chain ID"
	echo "Subnet: $SUBNET_ID  Chain: $CHAIN_ID"

	log "Convert to an L1 with EMPTY validator owners"
	platform subnet convert-to-l1 \
		--subnet-id "$SUBNET_ID" \
		--chain-id "$CHAIN_ID" \
		--manager "$MANAGER_ADDRESS" \
		--validator-node-ids "$node_ids" \
		--validator-bls-public-keys "$bls_keys" \
		--validator-bls-pops "$bls_pops" \
		--validator-weights "$weights" \
		--validator-balance "$BALANCE" \
		--allow-empty-owners
	wait_for_validators "$NUM_VALIDATORS"
	l1_validators | tee "$WORK_DIR/before.txt"
	awk '$3 != 0 {exit 1}' "$WORK_DIR/before.txt" || fail "a validator already has a deactivation owner"

	log "warp plan"
	platform warp plan \
		--subnet-id "$SUBNET_ID" \
		--rpc "$RPC" \
		--manager-blockchain-id "$CHAIN_ID" \
		--manager-address "$MANAGER_ADDRESS" \
		--remaining-balance-owner "$ADDRESS" \
		--deactivation-owner "$ADDRESS" \
		--balance "$BALANCE" \
		--out "$WORK_DIR/plan.json"
	[[ "$(jq '.targets | length' "$WORK_DIR/plan.json")" == "$NUM_VALIDATORS" ]] ||
		fail "plan does not have $NUM_VALIDATORS targets"

	log "warp sign: one bundle per validator machine"
	local i sigs=()
	for ((i = 0; i < NUM_VALIDATORS; i++)); do
		"$PLATFORM" warp sign \
			--plan "$WORK_DIR/plan.json" \
			--bls-key "$WORK_DIR/keys/$i/signer.key" \
			--out "$WORK_DIR/bundle-$i.json" \
			--yes >/dev/null
		sigs+=(--sigs "$WORK_DIR/bundle-$i.json")
	done

	log "warp rotate"
	platform warp rotate --plan "$WORK_DIR/plan.json" "${sigs[@]}" --rpc "$RPC" --yes

	log "Assert the rotation"
	wait_for_validators "$NUM_VALIDATORS"
	l1_validators | tee "$WORK_DIR/after.txt"
	local old_id new_id node reply
	while read -r old_id new_id node; do
		grep -q "^$old_id " "$WORK_DIR/after.txt" && fail "original validation $old_id of $node is still active"
		grep -q "^$new_id $node " "$WORK_DIR/after.txt" || fail "re-added validation $new_id of $node is not active"
		reply="$(pcall platform.getL1Validator "{\"validationID\":\"$new_id\"}")"
		[[ "$(jq -r '.result.deactivationOwner.threshold' <<<"$reply")" == "1" ]] ||
			fail "$node deactivation owner threshold is not 1"
		[[ "$(jq -r '.result.deactivationOwner.addresses[0]' <<<"$reply")" == "$ADDRESS" ]] ||
			fail "$node deactivation owner is not $ADDRESS"
		[[ "$(jq -r '.result.remainingBalanceOwner.threshold' <<<"$reply")" == "1" ]] ||
			fail "$node remaining balance owner threshold is not 1"
		echo "OK $node: $old_id -> $new_id, deactivation owner threshold 1 ($ADDRESS)"
	done < <(jq -r '.targets[] | "\(.validationID) \(.readdValidationID) \(.nodeID)"' "$WORK_DIR/plan.json")

	log "Rerun rotate: every target must be already rotated"
	platform warp rotate --plan "$WORK_DIR/plan.json" "${sigs[@]}" --rpc "$RPC" --yes |
		grep -c "already rotated" | grep -qx "$NUM_VALIDATORS" || fail "rerun did not skip every target"

	if [[ "${KEEP_L1:-}" == "1" ]]; then
		echo "KEEP_L1=1: validators keep running and paying the continuous fee"
	else
		log "Disable the re-added validators with the new deactivation owner (refunds balance)"
		while read -r new_id; do
			platform l1 disable-validator --validation-id "$new_id"
		done < <(jq -r '.targets[].readdValidationID' "$WORK_DIR/plan.json")
	fi

	log "PASS: $NUM_VALIDATORS validators rotated from empty to threshold-1 owners on $SUBNET_ID"
}

main "$@"
