#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
inputs="${root_dir}/sdk/toolchain/ie64-v3-release-inputs.conf"
qbe_dir="${QBE_SRC:-${root_dir}/../qbe}"
cproc_dir="${CPROC_SRC:-${root_dir}/../cproc}"
picolibc_dir="${PICOLIBC_SRC:-${root_dir}/../picolibc}"
host_arch="${HOST_SDK_ARCH:-amd64}"
package_name="${HOST_SDK_NAME:-intuition-engine-host-sdk-linux-${host_arch}}"
target_goarch="${HOST_SDK_GOARCH:-${host_arch}}"
host_cc="${HOST_SDK_CC:-cc}"
host_cc_flags="${HOST_SDK_CC_FLAGS:-}"
host_make_cc="${host_cc}"
qemu_aarch64="${HOST_SDK_QEMU_AARCH64:-}"
host_sysroot="${HOST_SDK_SYSROOT:-}"
host_cflags=''
host_ldflags=''

fail() { echo "dist-host-sdk-${host_arch}: $*" >&2; exit 1; }
[[ -f "${inputs}" ]] || fail "missing release-input manifest"
# shellcheck disable=SC1090
source "${inputs}"
[[ "$(uname -s)" == Linux && "$(uname -m)" == x86_64 ]] || fail "the release host must be Linux x86-64"
if [[ "${host_arch}" == arm64 ]]; then
    [[ "${target_goarch}" == arm64 ]] || fail "ARM64 distribution requires GOARCH=arm64"
    command -v "${host_cc}" >/dev/null 2>&1 || fail "ARM64 compiler is unavailable: ${host_cc}"
    [[ -n "${host_sysroot}" && -d "${host_sysroot}" ]] || fail "ARM64 sysroot is unavailable: ${host_sysroot:-unset}"
    [[ -n "${qemu_aarch64}" ]] || qemu_aarch64="$(command -v qemu-aarch64-static || command -v qemu-aarch64 || true)"
    [[ -x "${qemu_aarch64}" ]] || fail "QEMU AArch64 is unavailable"
    host_cflags="-D_GNU_SOURCE --sysroot=${host_sysroot}"
    if [[ "$(basename "${host_cc}")" == clang* ]]; then
        host_cc_flags="--target=aarch64-linux-gnu ${host_cc_flags}"
    fi
    host_make_cc="${host_cc} ${host_cc_flags}"
    host_ldflags="--sysroot=${host_sysroot}"
    libc_development="$(find "${host_sysroot}" -type f -o -type l 2>/dev/null | grep -E '/libc\.so$' | head -n 1 || true)"
    [[ -n "${libc_development}" ]] || fail "ARM64 sysroot lacks libc.so development linker input: ${host_sysroot}"
fi
epoch="${SOURCE_DATE_EPOCH:-${RELEASE_EPOCH}}"
[[ "${epoch}" =~ ^[0-9]+$ ]] || fail "SOURCE_DATE_EPOCH must be a non-negative integer"

verify_checkout() {
    local name=$1 directory=$2 revision=$3 actual
    [[ -d "${directory}/.git" ]] || fail "${name} is not a Git checkout: ${directory}"
    actual="$(git -C "${directory}" rev-parse HEAD)"
    [[ "${actual}" == "${revision}" ]] || fail "${name} revision mismatch: expected ${revision}, found ${actual}"
}
verify_checkout IntuitionEngine "${root_dir}" "$(git -C "${root_dir}" rev-parse HEAD)"
verify_checkout QBE "${qbe_dir}" "${QBE_REVISION}"
verify_checkout cproc "${cproc_dir}" "${CPROC_REVISION}"
verify_checkout Picolibc "${picolibc_dir}" "${PICOLIBC_REVISION}"

work_dir="$(mktemp -d)"
trap 'rm -rf "${work_dir}"' EXIT
stage="${work_dir}/${package_name}"

go_build=(env GOOS=linux GOARCH="${target_goarch}" go build)
run_host_tool() {
    if [[ "${host_arch}" == arm64 ]]; then
        "${qemu_aarch64}" -L "${host_sysroot}" "$@"
    else
        "$@"
    fi
}

build_external_tool() {
    local name=$1 source=$2 target=$3 build_dir
    build_dir="${work_dir}/${name}"
    cp -a "${source}" "${build_dir}"
    if [[ "${name}" == qbe ]]; then
        make -C "${build_dir}" clean-gen >/dev/null
        if [[ "${host_arch}" == arm64 ]]; then
            make -C "${build_dir}" CC="${host_make_cc}" CFLAGS="${host_cflags}" LDFLAGS="${host_ldflags}" qbe
        else
            make -C "${build_dir}" CC="${host_make_cc}" qbe
        fi
    else
        make -C "${build_dir}" clean >/dev/null
        if [[ "${host_arch}" == arm64 ]]; then
            rm -f "${build_dir}/config.h" "${build_dir}/config.mk"
            (cd "${build_dir}" && CC="${host_make_cc}" ./configure --host=aarch64-linux-gnu --target=aarch64-linux-gnu >/dev/null)
        fi
        if [[ "${host_arch}" == arm64 ]]; then
            make -C "${build_dir}" CC="${host_make_cc}" CFLAGS="${host_cflags}" LDFLAGS="${host_ldflags}" all
        else
            make -C "${build_dir}" CC="${host_make_cc}" all
        fi
    fi
    install -m 0755 "${build_dir}/${target}" "${stage}/bin/${target}"
}

prepare_nested_qemu_tools() {
    [[ "${host_arch}" == arm64 ]] || return 0
    for executable in qbe cproc-qbe; do
        mv "${stage}/bin/${executable}" "${stage}/bin/${executable}.arm64"
        printf '%s\n' '#!/bin/sh' "exec \"${qemu_aarch64}\" -L \"${host_sysroot}\" \"${stage}/bin/${executable}.arm64\" \"\$@\"" >"${stage}/bin/${executable}"
        chmod 0755 "${stage}/bin/${executable}"
    done
}

restore_nested_qemu_tools() {
    [[ "${host_arch}" == arm64 ]] || return 0
    for executable in qbe cproc-qbe; do
        rm -f "${stage}/bin/${executable}"
        mv "${stage}/bin/${executable}.arm64" "${stage}/bin/${executable}"
    done
}

mkdir -p "${stage}/bin" "${stage}/examples" "${stage}/include" "${stage}/lib/ie64-unknown-none/include" \
    "${stage}/share/cc65" "${stage}/share/docs" "${stage}/share/licenses"

# The Host SDK ships example source only. Assets, prebuilt guest binaries and
# editorial files belong to the repository, not the cross-development package.
while IFS= read -r -d '' source; do
    relative="${source#"${root_dir}/sdk/examples/"}"
    install -D -m 0644 "${source}" "${stage}/examples/${relative}"
done < <(find "${root_dir}/sdk/examples" -type f \( -name '*.asm' -o -name '*.bas' -o -name '*.c' \) -print0)

for include in ie32.inc ie64.inc ie68.inc ie80.inc ie65.inc ie86.inc; do
    install -m 0644 "${root_dir}/sdk/include/${include}" "${stage}/include/${include}"
done
for header in assert.h ctype.h float.h limits.h stdalign.h stdarg.h stdatomic.h stdbool.h stddef.h stdint.h stdlib.h stdnoreturn.h string.h; do
    install -m 0644 "${root_dir}/sdk/include/${header}" "${stage}/lib/ie64-unknown-none/include/${header}"
done
install -m 0644 "${root_dir}/sdk/include/intuitionengine.h" "${stage}/include/intuitionengine.h"
for config in ie65.cfg ie65_bindata.cfg ie65_service.cfg; do
    install -m 0644 "${root_dir}/sdk/include/${config}" "${stage}/share/cc65/${config}"
done
"${go_build[@]}" -trimpath -o "${stage}/bin/ie32asm" "${root_dir}/assembler/ie32asm.go"
"${go_build[@]}" -trimpath -o "${stage}/bin/ie32to64" "${root_dir}/cmd/ie32to64"
"${go_build[@]}" -trimpath -tags ie64 -o "${stage}/bin/ie64asm" "${root_dir}/assembler"
"${go_build[@]}" -trimpath -tags ie64dis -o "${stage}/bin/ie64dis" "${root_dir}/assembler"
"${go_build[@]}" -trimpath -o "${stage}/bin/ie64-cproc" "${root_dir}/cmd/ie64-cproc"
"${go_build[@]}" -trimpath -o "${stage}/bin/ie64ld" "${root_dir}/cmd/ie64ld"
"${go_build[@]}" -trimpath -o "${stage}/bin/ie64-ar" "${root_dir}/cmd/ie64-ar"
"${go_build[@]}" -trimpath -o "${stage}/bin/ie64-ranlib" "${root_dir}/cmd/ie64-ranlib"
build_external_tool qbe "${qbe_dir}" qbe
build_external_tool cproc "${cproc_dir}" cproc-qbe
for executable in "${stage}/bin"/*; do
    if [[ "${host_arch}" == arm64 ]]; then
        file "${executable}" | grep -Eq 'ELF.*(aarch64|ARM aarch64)' || fail "expected ARM64 ELF: ${executable}"
    else
        file "${executable}" | grep -Eq 'ELF.*(x86-64|x86_64)' || fail "expected x64 ELF: ${executable}"
    fi
done
prepare_nested_qemu_tools
"${stage}/bin/qbe" -h >/dev/null 2>&1 || fail "qbe smoke test failed"
"${stage}/bin/cproc-qbe" </dev/null >/dev/null 2>&1 || fail "cproc-qbe smoke test failed"
run_host_tool "${stage}/bin/ie32to64" -h >/dev/null 2>&1 || fail "ie32to64 smoke test failed"
for executable in ie64ld ie64-ar ie64-ranlib; do
    probe_args=(-h)
    [[ "${executable}" == ie64-ranlib ]] && probe_args=()
    if run_host_tool "${stage}/bin/${executable}" "${probe_args[@]}" >/dev/null 2>&1; then
        fail "${executable} smoke test unexpectedly succeeded without input"
    else
        status=$?
        [[ "${status}" -eq 2 ]] || fail "${executable} smoke test failed with status ${status}"
    fi
done

run_host_tool "${stage}/bin/ie64asm" -c -Werror -o "${stage}/lib/ie64-unknown-none/crt0.o" "${root_dir}/sdk/lib/ie64-cproc/crt0.s"
# Reuse the pinned IE64 toolchain release input for Picolibc. It is temporary
# input only and is never copied into this SDK.
legacy_archive="${root_dir}/dist/ie64-toolchain-v3-linux-amd64.tar.xz"
legacy_manifest_value() {
    tar -xJOf "${legacy_archive}" \
        ie64-toolchain-v3-linux-amd64/share/ie64/build-manifest.txt 2>/dev/null |
        awk -F= -v key="$1" '$1 == key { print substr($0, length(key) + 2); exit }'
}
legacy_archive_matches_inputs() {
    [[ -f "${legacy_archive}" ]] || return 1
    [[ "$(legacy_manifest_value format)" == ie64-toolchain-v3-manifest-1 ]] || return 1
    [[ "$(legacy_manifest_value target)" == "${TARGET}" ]] || return 1
    [[ "$(legacy_manifest_value host)" == "${HOST}" ]] || return 1
    [[ "$(legacy_manifest_value picolibc_revision)" == "${PICOLIBC_REVISION}" ]] || return 1
    [[ "$(legacy_manifest_value picolibc_options)" == "${PICOLIBC_OPTIONS}" ]] || return 1
}
if ! legacy_archive_matches_inputs; then
    "${root_dir}/scripts/dist-ie64-toolchain-linux-amd64.sh"
fi
legacy_archive_matches_inputs || fail "legacy IE64 toolchain input does not match pinned Picolibc revision and options"
legacy_root="${work_dir}/legacy/ie64-toolchain-v3-linux-amd64"
mkdir -p "${work_dir}/legacy"
tar -xf "${legacy_archive}" -C "${work_dir}/legacy"
for library in libc.a libm.a; do
    install -m 0644 "${legacy_root}/lib/ie64-unknown-none/${library}" "${stage}/lib/ie64-unknown-none/${library}"
done
run_host_tool "${stage}/bin/ie64-cproc" --sysroot "${root_dir}/sdk" -c -o "${work_dir}/libatomic.o" "${root_dir}/sdk/lib/ie64-cproc/libatomic.c"
run_host_tool "${stage}/bin/ie64-ar" rcs "${stage}/lib/ie64-unknown-none/libatomic.a" "${work_dir}/libatomic.o"
restore_nested_qemu_tools

for document in README.md architecture.md IE64_ISA.md IE32_ISA.md iescript.md iemon.md; do
    source="${root_dir}/sdk/docs/${document}"
    [[ "${document}" == README.md ]] && source="${root_dir}/sdk/docs/host-sdk-README.md"
    install -m 0644 "${source}" "${stage}/share/docs/${document}"
done
install -m 0644 "${root_dir}/sdk/docs/include-files-host-sdk.md" "${stage}/share/docs/include-files.md"
install -m 0644 "${root_dir}/sdk/docs/toolchain-profiles.md" "${stage}/share/docs/toolchain-profiles.md"
install -m 0644 "${root_dir}/LICENSE" "${stage}/share/licenses/IntuitionEngine-LICENSE"
install -m 0644 "${qbe_dir}/LICENSE" "${stage}/share/licenses/QBE-LICENSE"
install -m 0644 "${cproc_dir}/LICENSE" "${stage}/share/licenses/cproc-LICENSE"
install -m 0644 "${picolibc_dir}/COPYING.picolibc" "${stage}/share/licenses/COPYING.picolibc"

bash "${root_dir}/scripts/test-installed-host-sdk.sh" "${stage}"
cat >"${work_dir}/host-sdk-smoke.c" <<'EOF'
#include <assert.h>
#include <stdatomic.h>
#include <stdint.h>
#include <intuitionengine.h>
int main(void) {
    atomic_flag flag = ATOMIC_FLAG_INIT;
    volatile uint32_t *status = (volatile uint32_t *)(uintptr_t)IE_INPUT_TERM_STATUS;
    assert(status != 0);
    (void)atomic_flag_test_and_set(&flag);
    atomic_flag_clear(&flag);
    return (int)*status;
}
EOF
prepare_nested_qemu_tools
run_host_tool "${stage}/bin/ie64-cproc" --sysroot "${stage}" -c \
    -o "${work_dir}/host-sdk-smoke.o" "${work_dir}/host-sdk-smoke.c"
"${CC:-cc}" -std=c11 -I "${stage}/include" -DIE_TARGET_X86=1 -fsyntax-only \
    "${work_dir}/host-sdk-smoke.c"
if run_host_tool "${stage}/bin/ie64-cproc" -DIE_TARGET_X86=1 -c -o "${work_dir}/invalid.o" \
    "${work_dir}/host-sdk-smoke.c"; then
    fail "ie64-cproc accepted a user IE_TARGET_* definition"
fi
restore_nested_qemu_tools
inventory="${work_dir}/release-validation.json"
python3 "${root_dir}/scripts/write-host-sdk-validation.py" "${stage}" "${inventory}" "$(git -C "${root_dir}" rev-parse HEAD)" "${epoch}" "${QBE_REVISION}" "${CPROC_REVISION}" "${PICOLIBC_REVISION}"
python3 "${root_dir}/scripts/verify-host-sdk-validation.py" "${stage}" "${inventory}"

find "${stage}" -type d -exec chmod 0755 {} +
find "${stage}" -type f -exec chmod 0644 {} +
find "${stage}/bin" -type f -exec chmod 0755 {} +
find "${stage}" -exec touch -h -d "@${epoch}" {} +
mkdir -p "${root_dir}/dist"
archive="${root_dir}/dist/${package_name}.tar.xz"
rm -f "${archive}" "${archive}.sha256"
archive_cmd=(tar --sort=name --format=ustar --owner=0 --group=0 --numeric-owner --mtime="@${epoch}" --mode='u+rwX,go+rX,go-w' -C "${work_dir}" -cf - "${package_name}")
LC_ALL=C "${archive_cmd[@]}" | xz -0 --threads=1 --check=crc64 >"${archive}"
sha256sum "${archive}" | sed "s#  ${root_dir}/dist/#  #" >"${archive}.sha256"
LC_ALL=C "${archive_cmd[@]}" | xz -0 --threads=1 --check=crc64 >"${work_dir}/repro.tar.xz"
cmp "${archive}" "${work_dir}/repro.tar.xz"
mkdir "${work_dir}/extract"
tar -xf "${archive}" -C "${work_dir}/extract"
(cd "${root_dir}/dist" && sha256sum -c "$(basename "${archive}.sha256")" --status)
bash "${root_dir}/scripts/test-installed-host-sdk.sh" "${work_dir}/extract/${package_name}"
if [[ "${host_arch}" == arm64 ]]; then
    extracted_stage="${work_dir}/extract/${package_name}"
    for executable in qbe cproc-qbe; do
        mv "${extracted_stage}/bin/${executable}" "${extracted_stage}/bin/${executable}.arm64"
        printf '%s\n' '#!/bin/sh' "exec \"${qemu_aarch64}\" -L \"${host_sysroot}\" \"${extracted_stage}/bin/${executable}.arm64\" \"\$@\"" >"${extracted_stage}/bin/${executable}"
        chmod 0755 "${extracted_stage}/bin/${executable}"
    done
fi
run_host_tool "${work_dir}/extract/${package_name}/bin/ie64-cproc" \
    --sysroot "${work_dir}/extract/${package_name}" -c \
    -o "${work_dir}/extracted-smoke.o" "${work_dir}/host-sdk-smoke.c"
"${CC:-cc}" -std=c11 -I "${work_dir}/extract/${package_name}/include" \
    -DIE_TARGET_X86=1 -fsyntax-only "${work_dir}/host-sdk-smoke.c"
if [[ "${host_arch}" == arm64 ]]; then
    for executable in qbe cproc-qbe; do
        rm -f "${work_dir}/extract/${package_name}/bin/${executable}"
        mv "${work_dir}/extract/${package_name}/bin/${executable}.arm64" "${work_dir}/extract/${package_name}/bin/${executable}"
    done
fi
cmp "${archive}" "${root_dir}/dist/${package_name}.tar.xz"
mkdir -p "${root_dir}/intuitionengine.com/assets"
install -m 0644 "${archive}" "${root_dir}/intuitionengine.com/assets/$(basename "${archive}")"
install -m 0644 "${archive}.sha256" "${root_dir}/intuitionengine.com/assets/$(basename "${archive}.sha256")"
echo "created ${archive}"
