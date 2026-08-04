#!/usr/bin/env bash
set -euo pipefail
root=$1
[[ "$(find "${root}" -mindepth 1 -maxdepth 1 -printf '%f\n' | LC_ALL=C sort | tr '\n' ' ')" == 'bin examples include lib share ' ]] || { echo 'invalid host SDK top-level layout' >&2; exit 1; }
! find "${root}/examples" -type f ! \( -name '*.asm' -o -name '*.bas' -o -name '*.c' \) -print -quit | grep -q . || { echo 'invalid host SDK example layout' >&2; exit 1; }
expected_bins='cproc-qbe ie32asm ie32to64 ie64-ar ie64-cproc ie64-ranlib ie64asm ie64dis ie64ld qbe '
[[ "$(find "${root}/bin" -maxdepth 1 -type f -printf '%f\n' | LC_ALL=C sort | tr '\n' ' ')" == "${expected_bins}" ]] || { echo 'invalid host SDK bin layout' >&2; exit 1; }
expected_includes='ie32.inc ie64.inc ie65.inc ie68.inc ie80.inc ie86.inc intuitionengine.h '
[[ "$(find "${root}/include" -maxdepth 1 -type f -printf '%f\n' | LC_ALL=C sort | tr '\n' ' ')" == "${expected_includes}" ]] || { echo 'invalid host SDK include layout' >&2; exit 1; }
expected_standard_includes='assert.h ctype.h float.h limits.h stdalign.h stdarg.h stdatomic.h stdbool.h stddef.h stdint.h stdlib.h stdnoreturn.h string.h '
[[ "$(find "${root}/lib/ie64-unknown-none/include" -maxdepth 1 -type f -printf '%f\n' | LC_ALL=C sort | tr '\n' ' ')" == "${expected_standard_includes}" ]] || { echo 'invalid IE64 standard-header layout' >&2; exit 1; }
expected_cc65='ie65.cfg ie65_bindata.cfg ie65_service.cfg '
[[ "$(find "${root}/share/cc65" -maxdepth 1 -type f -printf '%f\n' | LC_ALL=C sort | tr '\n' ' ')" == "${expected_cc65}" ]] || { echo 'invalid host SDK cc65 layout' >&2; exit 1; }
expected_docs='IE32_ISA.md IE64_ISA.md README.md architecture.md iemon.md iescript.md include-files.md toolchain-profiles.md '
[[ "$(find "${root}/share/docs" -maxdepth 1 -type f -printf '%f\n' | LC_ALL=C sort | tr '\n' ' ')" == "${expected_docs}" ]] || { echo 'invalid host SDK documentation layout' >&2; exit 1; }
expected_licences='COPYING.picolibc IntuitionEngine-LICENSE QBE-LICENSE cproc-LICENSE '
[[ "$(find "${root}/share/licenses" -maxdepth 1 -type f -printf '%f\n' | LC_ALL=C sort | tr '\n' ' ')" == "${expected_licences}" ]] || { echo 'invalid host SDK licence layout' >&2; exit 1; }
for file in crt0.o libc.a libm.a libatomic.a; do [[ -f "${root}/lib/ie64-unknown-none/${file}" ]] || { echo "missing runtime ${file}" >&2; exit 1; }; done
! find "${root}" -name m68kto64 -o -name ie64.h -o -path '*/ie64/platform.h' | grep -q . || { echo 'forbidden legacy host SDK file' >&2; exit 1; }
