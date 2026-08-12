#!/usr/bin/env bash
# Shared architecture-neutral IESHARE staging entrypoint.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "${script_dir}/.." && pwd)"
destination=""
while (($#)); do
    case "$1" in
        --destination) (($# >= 2)) || { echo 'missing destination' >&2; exit 2; }; destination="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 2 ;;
    esac
done
[[ -n "$destination" ]] || { echo '--destination is required' >&2; exit 2; }
[[ "$(basename "$destination")" == ieshare-payload ]] || { echo 'destination must end in ieshare-payload' >&2; exit 2; }
mkdir -p "$(dirname "$destination")"
export X64_LIVE_WORK_DIR="$(dirname "$destination")"
export X64_LIVE_OUT_DIR="$(dirname "$X64_LIVE_WORK_DIR")"
export IE_PAYLOAD_LIBRARY_ONLY=1
# shellcheck source=../build_x64_ie_img.sh
source "${project_dir}/build_x64_ie_img.sh"
check_live_payload_inputs
stage_share_payload
verify_staged_share_payload "$destination"
