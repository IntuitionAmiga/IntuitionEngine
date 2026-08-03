#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
qbe_dir="${root_dir}/../qbe"
cproc_dir="${root_dir}/../cproc"
picolibc_dir="${root_dir}/../picolibc"
driver="${root_dir}/sdk/bin/ie64-cproc"

fail() {
  echo "test-ie64-toolchain: $*" >&2
  exit 1
}

[[ -f "${qbe_dir}/Makefile" ]] || fail "missing sibling QBE checkout: ${qbe_dir}"
[[ -f "${cproc_dir}/Makefile" ]] || fail "missing sibling cproc checkout: ${cproc_dir}"
[[ -f "${picolibc_dir}/meson.build" ]] || fail "missing sibling Picolibc checkout: ${picolibc_dir}"
command -v meson >/dev/null || fail "Meson is required to build the Picolibc integration image"

if ! make -C "${qbe_dir}" qbe; then
  fail "unable to build sibling QBE checkout"
fi
if ! make -C "${qbe_dir}" check; then
  fail "sibling QBE upstream tests failed"
fi
if ! make -C "${qbe_dir}" check-ie64; then
  fail "sibling QBE IE64 tests failed"
fi
if ! make -C "${cproc_dir}" all; then
  fail "unable to build sibling cproc checkout; repair its host configuration before IE64 integration"
fi
if ! make -C "${cproc_dir}" check; then
  fail "sibling cproc upstream tests failed"
fi
if ! make -C "${cproc_dir}" check-ie64; then
  fail "sibling cproc IE64 tests failed"
fi
if ! make -C "${cproc_dir}" check-stage2; then
  fail "sibling cproc stage2 tests failed"
fi

if ! make -C "${root_dir}" ie64-cproc ie64asm ie64ld ie64-ar ie64-ranlib; then
  fail "unable to build IntuitionEngine IE64 V2 host tools"
fi

[[ -x "${driver}" ]] || fail \
  "ie64-cproc is not built; implement the cproc IE64 driver before running integration fixtures"

[[ -x "${root_dir}/sdk/bin/ie64asm" ]] || make -C "${root_dir}" ie64asm
"${driver}" --version >/dev/null

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT
image="${tmp_dir}/smoke.ie64"
optimisation_images=()
abi_image="${tmp_dir}/abi.ie64"
cross_image="${tmp_dir}/cross.ie64"
lib_image="${tmp_dir}/lib.ie64"
builtin_image="${tmp_dir}/builtin.ie64"
mmu_image="${tmp_dir}/mmu.ie64"
halt_image="${tmp_dir}/halt.ie64"
interrupt_image="${tmp_dir}/interrupt.ie64"
atomic_misaligned_image="${tmp_dir}/atomic-misaligned.ie64"
atomic_aperture_image="${tmp_dir}/atomic-aperture.ie64"
atomic_image="${tmp_dir}/atomic.ie64"
atomic_interface_image="${tmp_dir}/atomic-interface.ie64"
atomic_collision_image="${tmp_dir}/atomic-collision.ie64"
picolibc_image="${tmp_dir}/picolibc.ie64"
assert_image="${tmp_dir}/assert.ie64"
assert_failure_image="${tmp_dir}/assert-failure.ie64"
host_lib_test="${tmp_dir}/libie64c-host-test"
host_atomic_orders_test="${tmp_dir}/atomic-orders-host-test"
"${driver}" -o "${image}" \
  "${root_dir}/sdk/tests/ie64-cproc/smoke.c" \
  "${root_dir}/sdk/tests/ie64-cproc/smoke_lifecycle.s"
for level in 0 1 2 3; do
  optimisation_images[level]="${tmp_dir}/smoke-O${level}.ie64"
  "${driver}" "-O${level}" -o "${optimisation_images[level]}" \
    "${root_dir}/sdk/tests/ie64-cproc/smoke.c" \
    "${root_dir}/sdk/tests/ie64-cproc/smoke_lifecycle.s"
done
"${driver}" -o "${abi_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/abi_runtime.c" \
  "${root_dir}/sdk/tests/ie64-cproc/abi_runtime.s"
"${driver}" -o "${cross_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/cross_runtime_main.c" \
  "${root_dir}/sdk/tests/ie64-cproc/cross_runtime_a.c" \
  "${root_dir}/sdk/tests/ie64-cproc/cross_runtime_b.c"
"${driver}" -o "${lib_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/lib_runtime.c"
"${driver}" -o "${builtin_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/builtin_runtime.c"
"${driver}" -o "${mmu_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/mmu_runtime.c"
"${driver}" -o "${halt_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/halt_runtime.c"
"${driver}" -o "${interrupt_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/interrupt_runtime.c" \
  "${root_dir}/sdk/tests/ie64-cproc/interrupt_runtime.s"
"${driver}" -DFAULT_ADDRESS=0x82001 \
  -o "${atomic_misaligned_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/atomic_fault_runtime.c"
"${driver}" -DFAULT_ADDRESS=0xa0000 \
  -o "${atomic_aperture_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/atomic_fault_runtime.c"
"${driver}" -o "${atomic_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/atomic_runtime.c" \
  "${root_dir}/sdk/lib/ie64-cproc/libatomic.c"
"${driver}" -o "${atomic_interface_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/atomic_interface_runtime.c" \
  "${root_dir}/sdk/lib/ie64-cproc/libatomic.c"
"${driver}" -o "${atomic_collision_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/atomic_collision_runtime.c" \
  "${root_dir}/sdk/lib/ie64-cproc/libatomic.c"

picolibc_sysroot="${tmp_dir}/picolibc-sysroot"
picolibc_build="${tmp_dir}/picolibc-build"
mkdir -p "${picolibc_sysroot}/bin" "${picolibc_sysroot}/include" \
  "${picolibc_sysroot}/lib/ie64-unknown-none"
cp -a "${root_dir}/sdk/include/." "${picolibc_sysroot}/include/"
install -m 0755 "${driver}" "${picolibc_sysroot}/bin/ie64-cproc"
install -m 0755 "${qbe_dir}/qbe" "${picolibc_sysroot}/bin/qbe"
install -m 0755 "${cproc_dir}/cproc-qbe" "${picolibc_sysroot}/bin/cproc-qbe"
for tool in ie64asm ie64ld ie64-ar ie64-ranlib; do
  install -m 0755 "${root_dir}/sdk/bin/${tool}" "${picolibc_sysroot}/bin/${tool}"
done
cat >"${tmp_dir}/picolibc-cc" <<EOF
#!/usr/bin/env bash
if [[ \$# -eq 1 && \$1 == --version ]]; then echo 'gcc (Free Software Foundation) 15.0.0'; exit 0; fi
if [[ \$# -eq 1 && \$1 == -Wl,--version ]]; then echo 'GNU ld (GNU Binutils) 2.40'; exit 0; fi
if [[ \$# -eq 1 && \$1 == -print-search-dirs ]]; then echo 'install: ${picolibc_sysroot}'; exit 0; fi
for arg in "\$@"; do
  if [[ \$arg == -dM ]]; then
    printf '%s\n' '#define __GNUC__ 15' '#define __GNUC_MINOR__ 0' \
      '#define __GNUC_PATCHLEVEL__ 0' '#define __STDC__ 1' \
      '#define __STDC_VERSION__ 202311L' '#define __SIZEOF_POINTER__ 8' \
      '#define __SIZEOF_LONG__ 8'
    exit 0
  fi
done
args=()
while [[ \$# -gt 0 ]]; do
  case \$1 in
    -P|-pipe|-xc|-Winvalid-pch|-g|-fdiagnostics-color=*) ;;
    -O0|-O1|-O2|-O3) args+=("\$1") ;;
    -Og) args+=(-O1) ;;
    -Os|-Oz) args+=(-O2) ;;
    -MQ) shift; args+=(-MT "\$1") ;;
    -Werror) args+=(-Werror) ;;
    -W*|-Wl,*) ;;
    *) args+=("\$1") ;;
  esac
  shift
done
exec "${picolibc_sysroot}/bin/ie64-cproc" --sysroot "${picolibc_sysroot}" "\${args[@]}"
EOF
cat >"${tmp_dir}/picolibc-ar" <<EOF
#!/usr/bin/env sh
if [ "\$#" -eq 1 ] && [ "\$1" = --version ]; then echo 'ie64-ar 2.0.0'; exit 0; fi
exec "${picolibc_sysroot}/bin/ie64-ar" "\$@"
EOF
cat >"${tmp_dir}/picolibc-cross.ini" <<EOF
[binaries]
c = '${tmp_dir}/picolibc-cc'
ar = '${tmp_dir}/picolibc-ar'
ranlib = '${picolibc_sysroot}/bin/ie64-ranlib'
strip = '/bin/true'
[host_machine]
system = 'none'
cpu_family = 'ie64'
cpu = 'ie64'
endian = 'little'
[properties]
needs_exe_wrapper = true
[built-in options]
c_std = 'c23'
c_args = ['-ffreestanding', '-fno-builtin']
EOF
chmod 0755 "${tmp_dir}/picolibc-cc" "${tmp_dir}/picolibc-ar"
# shellcheck disable=SC1091
source "${root_dir}/sdk/toolchain/ie64-v2-release-inputs.conf"
read -r -a picolibc_options <<<"${PICOLIBC_OPTIONS}"
meson setup "${picolibc_build}" "${picolibc_dir}" --prefix=/ \
  --cross-file="${tmp_dir}/picolibc-cross.ini" "${picolibc_options[@]}"
meson compile -C "${picolibc_build}"
DESTDIR="${picolibc_sysroot}" meson install -C "${picolibc_build}"
install -m 0644 "${picolibc_sysroot}/lib/libc.a" \
  "${picolibc_sysroot}/lib/ie64-unknown-none/libc.a"
install -m 0644 "${picolibc_sysroot}/lib/libm.a" \
  "${picolibc_sysroot}/lib/ie64-unknown-none/libm.a"
install -m 0644 "${root_dir}/sdk/include/ie64.h" "${picolibc_sysroot}/include/ie64.h"
"${picolibc_sysroot}/bin/ie64asm" -c -Werror \
  -o "${picolibc_sysroot}/lib/ie64-unknown-none/crt0.o" \
  "${root_dir}/sdk/lib/ie64-cproc/crt0.s"
"${picolibc_sysroot}/bin/ie64-cproc" --sysroot "${picolibc_sysroot}" -c \
  -o "${tmp_dir}/libatomic.o" "${root_dir}/sdk/lib/ie64-cproc/libatomic.c"
"${picolibc_sysroot}/bin/ie64-ar" rcs \
  "${picolibc_sysroot}/lib/ie64-unknown-none/libatomic.a" "${tmp_dir}/libatomic.o"
"${picolibc_sysroot}/bin/ie64-cproc" --sysroot "${picolibc_sysroot}" \
  -o "${picolibc_image}" "${root_dir}/sdk/tests/ie64-cproc/picolibc_runtime.c"
"${driver}" -o "${assert_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/assert_runtime.c" \
  "${root_dir}/sdk/tests/ie64-cproc/assert_enabled.c" \
  "${root_dir}/sdk/tests/ie64-cproc/assert_disabled.c"
"${driver}" -o "${assert_failure_image}" \
  "${root_dir}/sdk/tests/ie64-cproc/assert_failure_runtime.c"
"${CC:-cc}" -std=c11 -Wall -Wextra -Werror -Wno-unused-function -fno-builtin \
  -I"${root_dir}/sdk/include" \
  -o "${host_lib_test}" "${root_dir}/sdk/tests/ie64-cproc/libie64c_host_test.c"
"${host_lib_test}"
"${CC:-cc}" -std=c11 -Wall -Wextra -Werror \
  -o "${host_atomic_orders_test}" \
  "${root_dir}/sdk/tests/ie64-cproc/atomic_orders_host_test.c"
"${host_atomic_orders_test}"
IE64_TOOLCHAIN_IMAGE="${image}" \
IE64_TOOLCHAIN_O0_IMAGE="${optimisation_images[0]}" \
IE64_TOOLCHAIN_O1_IMAGE="${optimisation_images[1]}" \
IE64_TOOLCHAIN_O2_IMAGE="${optimisation_images[2]}" \
IE64_TOOLCHAIN_O3_IMAGE="${optimisation_images[3]}" \
IE64_TOOLCHAIN_ABI_IMAGE="${abi_image}" \
IE64_TOOLCHAIN_CROSS_IMAGE="${cross_image}" \
IE64_TOOLCHAIN_LIB_IMAGE="${lib_image}" \
IE64_TOOLCHAIN_BUILTIN_IMAGE="${builtin_image}" \
IE64_TOOLCHAIN_MMU_IMAGE="${mmu_image}" \
IE64_TOOLCHAIN_HALT_IMAGE="${halt_image}" \
IE64_TOOLCHAIN_INTERRUPT_IMAGE="${interrupt_image}" \
IE64_TOOLCHAIN_ATOMIC_MISALIGNED_IMAGE="${atomic_misaligned_image}" \
IE64_TOOLCHAIN_ATOMIC_APERTURE_IMAGE="${atomic_aperture_image}" \
IE64_TOOLCHAIN_ATOMIC_IMAGE="${atomic_image}" \
IE64_TOOLCHAIN_ATOMIC_INTERFACE_IMAGE="${atomic_interface_image}" \
IE64_TOOLCHAIN_ATOMIC_COLLISION_IMAGE="${atomic_collision_image}" \
IE64_TOOLCHAIN_PICOLIBC_IMAGE="${picolibc_image}" \
IE64_TOOLCHAIN_ASSERT_IMAGE="${assert_image}" \
IE64_TOOLCHAIN_ASSERT_FAILURE_IMAGE="${assert_failure_image}" \
  go test -tags headless -run '^TestIE64CProc(Smoke|Optimisation|ABI|CrossUnit|Library|Builtin|MMU|Halt|Interrupt|AtomicMisaligned|AtomicAperture|Atomic|AtomicInterface|AtomicCollisionAndReadOnly|Picolibc|Assert|AssertFailure)ImageDefaultJIT$|^TestIE64CProcStartupRejectsLowRAMDefaultJIT$' \
  -count=1 "${root_dir}"
echo "test-ie64-toolchain: ABI, cross-unit, library, machine-facility, MMU, interrupt, atomic, Picolibc, assert and low-RAM images passed through the default IE64 JIT"
