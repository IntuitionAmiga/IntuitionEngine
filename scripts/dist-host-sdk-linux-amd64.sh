#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
inputs="${root_dir}/sdk/toolchain/ie64-v3-release-inputs.conf"
qbe_dir="${QBE_SRC:-${root_dir}/../qbe}"
cproc_dir="${CPROC_SRC:-${root_dir}/../cproc}"
picolibc_dir="${PICOLIBC_SRC:-${root_dir}/../picolibc}"
package_name=intuition-engine-host-sdk-linux-amd64

fail() { echo "dist-host-sdk-linux-amd64: $*" >&2; exit 1; }
[[ -f "${inputs}" ]] || fail "missing release-input manifest"
# shellcheck disable=SC1090
source "${inputs}"
[[ "$(uname -s)" == Linux && "$(uname -m)" == x86_64 ]] || fail "the release host must be Linux x86-64"
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
mkdir -p "${stage}/bin" "${stage}/include" "${stage}/lib/ie64-unknown-none/include" \
    "${stage}/share/cc65" "${stage}/share/docs" "${stage}/share/licenses"

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
go build -trimpath -o "${stage}/bin/ie32asm" "${root_dir}/assembler/ie32asm.go"
go build -trimpath -o "${stage}/bin/ie32to64" "${root_dir}/cmd/ie32to64"
go build -trimpath -tags ie64 -o "${stage}/bin/ie64asm" "${root_dir}/assembler"
go build -trimpath -tags ie64dis -o "${stage}/bin/ie64dis" "${root_dir}/assembler"
go build -trimpath -o "${stage}/bin/ie64-cproc" "${root_dir}/cmd/ie64-cproc"
go build -trimpath -o "${stage}/bin/ie64ld" "${root_dir}/cmd/ie64ld"
go build -trimpath -o "${stage}/bin/ie64-ar" "${root_dir}/cmd/ie64-ar"
go build -trimpath -o "${stage}/bin/ie64-ranlib" "${root_dir}/cmd/ie64-ranlib"
make -C "${qbe_dir}" qbe
make -C "${cproc_dir}" all
install -m 0755 "${qbe_dir}/qbe" "${stage}/bin/qbe"
install -m 0755 "${cproc_dir}/cproc-qbe" "${stage}/bin/cproc-qbe"

"${stage}/bin/ie64asm" -c -Werror -o "${stage}/lib/ie64-unknown-none/crt0.o" "${root_dir}/sdk/lib/ie64-cproc/crt0.s"
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
"${stage}/bin/ie64-cproc" --sysroot "${root_dir}/sdk" -c -o "${work_dir}/libatomic.o" "${root_dir}/sdk/lib/ie64-cproc/libatomic.c"
"${stage}/bin/ie64-ar" rcs "${stage}/lib/ie64-unknown-none/libatomic.a" "${work_dir}/libatomic.o"

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
"${stage}/bin/ie64-cproc" --sysroot "${stage}" -c \
    -o "${work_dir}/host-sdk-smoke.o" "${work_dir}/host-sdk-smoke.c"
"${CC:-cc}" -std=c11 -I "${stage}/include" -DIE_TARGET_X86=1 -fsyntax-only \
    "${work_dir}/host-sdk-smoke.c"
if "${stage}/bin/ie64-cproc" -DIE_TARGET_X86=1 -c -o "${work_dir}/invalid.o" \
    "${work_dir}/host-sdk-smoke.c"; then
    fail "ie64-cproc accepted a user IE_TARGET_* definition"
fi
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
"${work_dir}/extract/${package_name}/bin/ie64-cproc" \
    --sysroot "${work_dir}/extract/${package_name}" -c \
    -o "${work_dir}/extracted-smoke.o" "${work_dir}/host-sdk-smoke.c"
"${CC:-cc}" -std=c11 -I "${work_dir}/extract/${package_name}/include" \
    -DIE_TARGET_X86=1 -fsyntax-only "${work_dir}/host-sdk-smoke.c"
cmp "${archive}" "${root_dir}/dist/${package_name}.tar.xz"
mkdir -p "${root_dir}/intuitionengine.com/assets"
install -m 0644 "${archive}" "${root_dir}/intuitionengine.com/assets/$(basename "${archive}")"
install -m 0644 "${archive}.sha256" "${root_dir}/intuitionengine.com/assets/$(basename "${archive}.sha256")"
echo "created ${archive}"
