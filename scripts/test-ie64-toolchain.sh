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
if ! make -C "${qbe_dir}" check-ie64; then
  fail "sibling QBE IE64 tests failed"
fi
if ! make -C "${cproc_dir}" all; then
  fail "unable to build sibling cproc checkout; repair its host configuration before IE64 integration"
fi
if ! make -C "${cproc_dir}" check-ie64; then
  fail "sibling cproc IE64 tests failed"
fi

[[ -x "${cproc_dir}/ie64-cproc" ]] || fail \
  "ie64-cproc is not built; implement the cproc IE64 driver before running integration fixtures"

[[ -x "${root_dir}/sdk/bin/ie64asm" ]] || make -C "${root_dir}" ie64asm
"${cproc_dir}/ie64-cproc" --version >/dev/null

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT
image="${tmp_dir}/smoke.ie64"
abi_image="${tmp_dir}/abi.ie64"
lib_image="${tmp_dir}/lib.ie64"
builtin_image="${tmp_dir}/builtin.ie64"
halt_image="${tmp_dir}/halt.ie64"
interrupt_image="${tmp_dir}/interrupt.ie64"
atomic_misaligned_image="${tmp_dir}/atomic-misaligned.ie64"
atomic_aperture_image="${tmp_dir}/atomic-aperture.ie64"
assert_image="${tmp_dir}/assert.ie64"
assert_failure_image="${tmp_dir}/assert-failure.ie64"
host_lib_test="${tmp_dir}/libie64c-host-test"
"${cproc_dir}/ie64-cproc" -o "${image}" "${root_dir}/sdk/tests/ie64-cproc/smoke.c"
"${cproc_dir}/ie64-cproc" -o "${abi_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/abi_runtime.c" \
  "${root_dir}/sdk/tests/ie64-cproc/abi_runtime.s"
"${cproc_dir}/ie64-cproc" -o "${lib_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/lib_runtime.c"
"${cproc_dir}/ie64-cproc" -o "${builtin_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/builtin_runtime.c"
"${cproc_dir}/ie64-cproc" -o "${halt_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/halt_runtime.c"
"${cproc_dir}/ie64-cproc" -o "${interrupt_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/interrupt_runtime.c" \
  "${root_dir}/sdk/tests/ie64-cproc/interrupt_runtime.s"
"${cproc_dir}/ie64-cproc" -DFAULT_ADDRESS=0x82001 \
  -o "${atomic_misaligned_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/atomic_fault_runtime.c"
"${cproc_dir}/ie64-cproc" -DFAULT_ADDRESS=0xa0000 \
  -o "${atomic_aperture_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/atomic_fault_runtime.c"
"${cproc_dir}/ie64-cproc" -o "${assert_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/assert_runtime.c" \
  "${root_dir}/sdk/tests/ie64-cproc/assert_enabled.c" \
  "${root_dir}/sdk/tests/ie64-cproc/assert_disabled.c"
"${cproc_dir}/ie64-cproc" -o "${assert_failure_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/assert_failure_runtime.c"
"${CC:-cc}" -std=c11 -Wall -Wextra -Werror -fno-builtin \
  -o "${host_lib_test}" "${root_dir}/sdk/tests/ie64-cproc/libie64c_host_test.c"
"${host_lib_test}"
IE64_TOOLCHAIN_IMAGE="${image}" \
IE64_TOOLCHAIN_ABI_IMAGE="${abi_image}" \
IE64_TOOLCHAIN_LIB_IMAGE="${lib_image}" \
IE64_TOOLCHAIN_BUILTIN_IMAGE="${builtin_image}" \
IE64_TOOLCHAIN_HALT_IMAGE="${halt_image}" \
IE64_TOOLCHAIN_INTERRUPT_IMAGE="${interrupt_image}" \
IE64_TOOLCHAIN_ATOMIC_MISALIGNED_IMAGE="${atomic_misaligned_image}" \
IE64_TOOLCHAIN_ATOMIC_APERTURE_IMAGE="${atomic_aperture_image}" \
IE64_TOOLCHAIN_ASSERT_IMAGE="${assert_image}" \
IE64_TOOLCHAIN_ASSERT_FAILURE_IMAGE="${assert_failure_image}" \
  go test -tags headless -run '^TestIE64CProc(Smoke|ABI|Library|Builtin|Halt|Interrupt|AtomicMisaligned|AtomicAperture|Assert|AssertFailure)ImageDefaultJIT$|^TestIE64CProcStartupRejectsLowRAMDefaultJIT$' \
  -count=1 "${root_dir}"
echo "test-ie64-toolchain: smoke, ABI, library, builtin, halt, assert and low-RAM images passed through the default IE64 JIT"
