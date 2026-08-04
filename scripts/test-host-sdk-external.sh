#!/usr/bin/env bash
set -euo pipefail

# External toolchains are intentionally integration-only.  A developer gets an
# explicit unrun result unless a configured tool path is present; CI sets every
# IE_SDK_* variable and treats any missing configured tool as a failure.
configured=0
root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT
cat >"${tmp}/fixture.c" <<'EOF'
#include <stdint.h>
#include <intuitionengine.h>
int main(void) { return (int)(IE_VIDEO_STATUS + IE_INPUT_TERM_STATUS + IE_FILE_OPEN_READ + IE_NET_CMD_SOCKET); }
EOF
for spec in M68K_GCC M68K_VBCC Z80_VBCC 6502_SDCC 6502_VBCC X86_GCC; do
    variable="IE_SDK_${spec}"
    tool="${!variable:-}"
    if [[ -z "${tool}" ]]; then
        printf 'UNRUN %s: %s is not configured\n' "${spec}" "${variable}"
        continue
    fi
    configured=1
    [[ -x "${tool}" || -n "$(command -v "${tool}" 2>/dev/null || true)" ]] || {
        echo "FAIL ${spec}: configured compiler is unavailable: ${tool}" >&2
        exit 1
    }
    printf 'CONFIGURED %s: %s\n' "${spec}" "${tool}"
    "${tool}" --version | head -n 1
    case "${spec}" in
        M68K_*) target=IE_TARGET_M68K=1 ;;
        Z80_*) target=IE_TARGET_Z80=1 ;;
        6502_*) target=IE_TARGET_6502=1 ;;
        X86_*) target=IE_TARGET_X86=1 ;;
    esac
    "${tool}" -I "${root_dir}/sdk/include" -D"${target}" -c "${tmp}/fixture.c" -o "${tmp}/${spec}.o"
done
[[ "${configured}" -eq 0 ]] && exit 0
