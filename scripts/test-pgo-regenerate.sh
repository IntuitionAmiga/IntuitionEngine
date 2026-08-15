#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

cat >"$tmp_dir/capture" <<'CAPTURE'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$IE_CPUPROFILE" == *complete.pprof ]]; then
  printf 'profile' >"$IE_CPUPROFILE"
fi
CAPTURE
chmod +x "$tmp_dir/capture"

set +e
output="$({
  PGO_CAPTURE_BIN="$tmp_dir/capture" \
  PGO_WORKLOADS=$'complete|1|\nmissing|1|' \
    bash "$root_dir/scripts/pgo-regenerate.sh" "$tmp_dir/default.pgo.new"
} 2>&1)"
status=$?
set -e

if [[ "$status" -eq 0 ]]; then
  echo "FAIL: regeneration accepted a missing workload profile" >&2
  exit 1
fi
if [[ "$output" != *"missing produced no profile"* ]]; then
  printf '%s\n' "$output" >&2
  echo "FAIL: regeneration did not identify the missing workload" >&2
  exit 1
fi

echo "PGO regeneration failure checks passed"
