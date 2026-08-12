#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
stage="$root_dir/scripts/stage_ieshare_payload.sh"
[[ -x "$stage" ]] || { echo 'missing shared payload entrypoint' >&2; exit 1; }
if "$stage" >/dev/null 2>&1; then
    echo 'payload entrypoint accepted no destination' >&2; exit 1
fi
if "$stage" --destination /tmp/not-payload >/dev/null 2>&1; then
    echo 'payload entrypoint accepted an unsafe destination name' >&2; exit 1
fi
rg -q 'IE_PAYLOAD_LIBRARY_ONLY=1' "$stage" || { echo 'payload entrypoint does not load shared implementation' >&2; exit 1; }
rg -q 'verify_staged_share_payload' "$stage" || { echo 'payload entrypoint does not validate staging output' >&2; exit 1; }
rg -q 'source.*build_x64_ie_img\.sh' "$stage" || { echo 'payload entrypoint does not use the x64 payload implementation' >&2; exit 1; }
echo 'Shared IESHARE payload contracts passed'
