#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
qbe_dir="${root_dir}/../qbe"
cproc_dir="${root_dir}/../cproc"

fail() {
  echo "test-ie64-toolchain: $*" >&2
  exit 1
}

[[ -f "${qbe_dir}/Makefile" ]] || fail "missing sibling QBE checkout: ${qbe_dir}"
[[ -f "${cproc_dir}/Makefile" ]] || fail "missing sibling cproc checkout: ${cproc_dir}"

if ! make -C "${qbe_dir}" qbe; then
  fail "unable to build sibling QBE checkout"
fi
if ! make -C "${cproc_dir}" all; then
  fail "unable to build sibling cproc checkout; repair its host configuration before IE64 integration"
fi

[[ -x "${cproc_dir}/ie64-cproc" ]] || fail \
  "ie64-cproc is not built; implement the cproc IE64 driver before running integration fixtures"

[[ -x "${root_dir}/sdk/bin/ie64asm" ]] || make -C "${root_dir}" ie64asm
"${cproc_dir}/ie64-cproc" --version >/dev/null

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT
image="${tmp_dir}/smoke.ie64"
"${cproc_dir}/ie64-cproc" -o "${image}" "${root_dir}/sdk/tests/ie64-cproc/smoke.c"
IE64_TOOLCHAIN_IMAGE="${image}" go test -tags headless -run '^TestIE64CProcSmokeImageDefaultJIT$' -count=1 "${root_dir}"
echo "test-ie64-toolchain: smoke image passed through the default IE64 JIT"
