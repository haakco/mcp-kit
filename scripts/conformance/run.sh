#!/usr/bin/env bash
#
# Run the official MCP conformance suite against a 2026-07-28 server.
#
# The kit ships middleware, not tools: it owns the transport envelope, Origin
# allowlisting, OAuth, and discovery. The conformance suite's tool/resource/
# prompt scenarios therefore belong to the servers that own those handlers.
# This runner is the one command both halves use:
#
#   * CI runs it with no target, so it starts the SDK's own reference server and
#     proves the pinned CLI, the revision profile, and the baseline still agree.
#   * A consumer runs it with CONFORMANCE_URL pointed at their server to prove
#     their tools, resources, and prompts on the 2026-07-28 wire. Note the CLI
#     sends no credentials, so a bearer-protected /mcp must be reached through a
#     token-injecting bridge; see docs/conformance.md, "Authenticated servers
#     need a bridge".
#
# Usage:
#   scripts/conformance/run.sh [--url <url>] [--results <dir>] [--verbose]
#
# Environment:
#   CONFORMANCE_URL      Target server URL. When unset the SDK reference server
#                        is started on CONFORMANCE_PORT.
#   CONFORMANCE_PORT     Port for the built-in reference server. Default 3111.
#   CONFORMANCE_VERSION  Conformance CLI version. Default 0.2.0-alpha.11.
#   CONFORMANCE_SPEC     Spec revision to require. Default 2026-07-28.
#
# The CLI version matters: 0.1.16 does not know 2026-07-28 at all and rejects
# the revision with "Unknown spec version". Only the 0.2.x line implements it,
# and only the 0.2.x line accepts --requirements.

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"

conformance_version="${CONFORMANCE_VERSION:-0.2.0-alpha.11}"
spec_revision="${CONFORMANCE_SPEC:-2026-07-28}"
port="${CONFORMANCE_PORT:-3111}"
baseline="$repo_root/conformance-baseline.yml"
target_url="${CONFORMANCE_URL:-}"
results_dir=""
verbose=""

while [[ $# -gt 0 ]]; do
	case "$1" in
	--url)
		target_url="$2"
		shift 2
		;;
	--results)
		results_dir="$2"
		shift 2
		;;
	--verbose)
		verbose="--verbose"
		shift
		;;
	*)
		echo "unknown option: $1" >&2
		exit 2
		;;
	esac
done

if [[ ! -f "$baseline" ]]; then
	echo "missing baseline file: $baseline" >&2
	exit 1
fi

server_pid=""
workdir=""
cleanup() {
	if [[ -n "$server_pid" ]]; then
		kill "$server_pid" 2>/dev/null || true
		wait "$server_pid" 2>/dev/null || true
	fi
	if [[ -n "$workdir" && -z "$results_dir" ]]; then
		rm -rf "$workdir"
	fi
}
trap cleanup EXIT

if [[ -z "$target_url" ]]; then
	workdir="$(mktemp -d)"
	echo "Building the MCP Go SDK reference server..."
	if ! go build -o "$workdir/conformance-server" github.com/modelcontextprotocol/go-sdk/conformance/everything-server; then
		echo "failed to build the reference server" >&2
		exit 1
	fi

	echo "Starting reference server on 127.0.0.1:${port}..."
	"$workdir/conformance-server" -http="127.0.0.1:${port}" -stateless=true >"$workdir/server.log" 2>&1 &
	server_pid=$!

	# Bounded readiness loop: fail loudly rather than letting the suite report
	# every scenario as a connection error.
	ready=""
	for _ in $(seq 1 60); do
		if ! kill -0 "$server_pid" 2>/dev/null; then
			echo "reference server exited before becoming ready:" >&2
			cat "$workdir/server.log" >&2
			exit 1
		fi
		# The server answers 405 to a bodiless GET in stateless mode, so a
		# successful connection is the readiness signal, not a 2xx status.
		if curl -s -o /dev/null "http://127.0.0.1:${port}" 2>/dev/null; then
			ready=1
			break
		fi
		sleep 0.5
	done
	if [[ -z "$ready" ]]; then
		echo "reference server did not become ready within 30s:" >&2
		cat "$workdir/server.log" >&2
		exit 1
	fi

	target_url="http://127.0.0.1:${port}"
fi

echo "Running MCP conformance ${conformance_version} against ${target_url}"
echo "Requiring revision ${spec_revision}"

run_dir="$(mktemp -d)"
suite_args=(server --url "$target_url" --requirements "$spec_revision" --expected-failures "$baseline")
if [[ -n "$results_dir" ]]; then
	mkdir -p "$results_dir"
	suite_args+=(--output-dir "$results_dir")
fi
if [[ -n "$verbose" ]]; then
	suite_args+=("$verbose")
fi

# Run from a scratch directory so a consumer's repository is never written to.
set +e
(
	cd "$run_dir" && npx --yes "@modelcontextprotocol/conformance@${conformance_version}" "${suite_args[@]}"
)
exit_code=$?
set -e
rm -rf "$run_dir"

if [[ $exit_code -ne 0 ]]; then
	echo ""
	echo "Conformance failed. Every failure above must either be fixed or recorded in" >&2
	echo "conformance-baseline.yml with a reason, owner, and removal condition." >&2
fi
exit $exit_code
