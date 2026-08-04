#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
header_dir="${root_dir}/sdk/include"
tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT

cat >"${tmp}/fixture.c" <<'EOF'
#include <stdint.h>
#include <intuitionengine.h>
#if !defined(IE_HAS_BANK_WINDOWS) || !defined(IE_HAS_X86_PORT_IO) || !defined(IE_HAS_IE64_CONTROL_REGISTERS) || !defined(IE_HAS_IE64_ATOMICS) || !defined(IE_HAS_IE64_FPU)
#error capability macro missing
#endif
#if (IE_HAS_BANK_WINDOWS != 0 && IE_HAS_BANK_WINDOWS != 1) || (IE_HAS_X86_PORT_IO != 0 && IE_HAS_X86_PORT_IO != 1) || (IE_HAS_IE64_CONTROL_REGISTERS != 0 && IE_HAS_IE64_CONTROL_REGISTERS != 1) || (IE_HAS_IE64_ATOMICS != 0 && IE_HAS_IE64_ATOMICS != 1) || (IE_HAS_IE64_FPU != 0 && IE_HAS_IE64_FPU != 1)
#error capability macro is not literal zero or one
#endif
int fixture(void) { return (int)(IE_VIDEO_STATUS + IE_INPUT_TERM_STATUS + IE_FILE_OPEN_READ + IE_NET_CMD_SOCKET); }
EOF

cc="${CC:-gcc}"
"${cc}" -I "${header_dir}" -DIE_TARGET_X86=1 -fsyntax-only "${tmp}/fixture.c"
"${cc}" -I "${header_dir}" -DIE_TARGET_M68K=1 -fsyntax-only "${tmp}/fixture.c"
"${cc}" -I "${header_dir}" -D__VBCC__=1 -DIE_TARGET_Z80=1 -fsyntax-only "${tmp}/fixture.c"
"${cc}" -I "${header_dir}" -D__VBCC__=1 -DIE_TARGET_6502=1 -fsyntax-only "${tmp}/fixture.c"
if "${cc}" -I "${header_dir}" -fsyntax-only "${tmp}/fixture.c"; then
    echo 'header accepted zero target selections' >&2
    exit 1
fi
if "${cc}" -I "${header_dir}" -DIE_TARGET_X86=1 -DIE_TARGET_M68K=1 -fsyntax-only "${tmp}/fixture.c"; then
    echo 'header accepted multiple target selections' >&2
    exit 1
fi
