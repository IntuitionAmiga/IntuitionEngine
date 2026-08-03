#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
inputs="${root_dir}/sdk/toolchain/ie64-v2-release-inputs.conf"
qbe_dir="${QBE_SRC:-${root_dir}/../qbe}"
cproc_dir="${CPROC_SRC:-${root_dir}/../cproc}"
picolibc_dir="${PICOLIBC_SRC:-${root_dir}/../picolibc}"
package_name=ie64-toolchain-v2-linux-amd64

fail() {
    echo "dist-ie64-toolchain-linux-amd64: $*" >&2
    exit 1
}

[[ -f "${inputs}" ]] || fail "missing release-input manifest"
# This tracked file contains simple NAME=value records controlled by the
# project. shellcheck disable=SC1090
source "${inputs}"

[[ "$(uname -s)" == Linux && "$(uname -m)" == x86_64 ]] ||
    fail "the release host must be Linux x86-64"
epoch="${SOURCE_DATE_EPOCH:-${RELEASE_EPOCH}}"
[[ "${epoch}" =~ ^[0-9]+$ ]] || fail "SOURCE_DATE_EPOCH must be a non-negative integer"

verify_checkout() {
    local name=$1 directory=$2 revision=$3 actual status
    [[ -d "${directory}/.git" ]] || fail "${name} is not a Git checkout: ${directory}"
    actual="$(git -C "${directory}" rev-parse HEAD)"
    [[ "${actual}" == "${revision}" ]] ||
        fail "${name} revision mismatch: expected ${revision}, found ${actual}"
    # Untracked plans, IDE metadata, and generated files do not enter the
    # package because the staging commands name every input explicitly. Keep
    # the reproducibility guard focused on tracked source changes.
    status="$(git -C "${directory}" status --porcelain --untracked-files=no)"
    [[ -z "${status}" ]] || fail "${name} checkout is not clean"
}

verify_checkout IntuitionEngine "${root_dir}" "$(git -C "${root_dir}" rev-parse HEAD)"
verify_checkout QBE "${qbe_dir}" "${QBE_REVISION}"
verify_checkout cproc "${cproc_dir}" "${CPROC_REVISION}"
verify_checkout Picolibc "${picolibc_dir}" "${PICOLIBC_REVISION}"
[[ "$(git -C "${picolibc_dir}" branch --show-current)" == ie ]] ||
    fail "Picolibc must be on branch ie"
[[ "$(git -C "${picolibc_dir}" remote get-url origin)" == \
    https://github.com/IntuitionAmiga/picolibc ]] ||
    fail "Picolibc origin is not the canonical repository"

work_dir="$(mktemp -d)"
trap 'rm -rf "${work_dir}"' EXIT
stage="${work_dir}/${package_name}"
build_dir="${work_dir}/picolibc-build"
mkdir -p "${stage}/bin" "${stage}/include" \
    "${stage}/lib/ie64-unknown-none" "${stage}/share/ie64/docs" \
    "${stage}/share/ie64/licenses"
cp -a "${root_dir}/sdk/include/." "${stage}/include/"

go build -trimpath -o "${stage}/bin/ie64-cproc" "${root_dir}/cmd/ie64-cproc"
go build -trimpath -tags ie64 -o "${stage}/bin/ie64asm" "${root_dir}/assembler"
go build -trimpath -tags ie64dis -o "${stage}/bin/ie64dis" "${root_dir}/assembler"
go build -trimpath -o "${stage}/bin/ie64ld" "${root_dir}/cmd/ie64ld"
go build -trimpath -o "${stage}/bin/ie64-ar" "${root_dir}/cmd/ie64-ar"
go build -trimpath -o "${stage}/bin/ie64-ranlib" "${root_dir}/cmd/ie64-ranlib"
make -C "${qbe_dir}" qbe
make -C "${cproc_dir}" all
install -m 0755 "${qbe_dir}/qbe" "${stage}/bin/qbe"
install -m 0755 "${cproc_dir}/cproc-qbe" "${stage}/bin/cproc-qbe"

cat >"${work_dir}/meson-cc" <<EOF
#!/usr/bin/env bash
if [[ \$# -eq 1 && \$1 == --version ]]; then echo 'gcc (Free Software Foundation) 15.0.0'; exit 0; fi
if [[ \$# -eq 1 && \$1 == -Wl,--version ]]; then echo 'GNU ld (GNU Binutils) 2.40'; exit 0; fi
if [[ \$# -eq 1 && \$1 == -print-search-dirs ]]; then echo 'install: ${stage}'; exit 0; fi
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
exec "${stage}/bin/ie64-cproc" --sysroot "${stage}" "\${args[@]}"
EOF
cat >"${work_dir}/meson-ar" <<EOF
#!/usr/bin/env sh
if [ "\$#" -eq 1 ] && [ "\$1" = --version ]; then echo 'ie64-ar 2.0.0'; exit 0; fi
exec "${stage}/bin/ie64-ar" "\$@"
EOF
cat >"${work_dir}/cross.ini" <<EOF
[binaries]
c = '${work_dir}/meson-cc'
ar = '${work_dir}/meson-ar'
ranlib = '${stage}/bin/ie64-ranlib'
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
chmod 0755 "${work_dir}/meson-cc" "${work_dir}/meson-ar"

read -r -a picolibc_options <<<"${PICOLIBC_OPTIONS}"
meson setup "${build_dir}" "${picolibc_dir}" --prefix=/ \
    --cross-file="${work_dir}/cross.ini" "${picolibc_options[@]}"
meson compile -C "${build_dir}"
DESTDIR="${stage}" meson install -C "${build_dir}"
install -m 0644 "${stage}/lib/libc.a" "${stage}/lib/ie64-unknown-none/libc.a"
install -m 0644 "${stage}/lib/libm.a" "${stage}/lib/ie64-unknown-none/libm.a"
rm -f "${stage}/lib/libc.a" "${stage}/lib/libm.a" "${stage}/lib/libg.a" \
    "${stage}/lib/libnosys.a" "${stage}/lib/libdummyhost.a" \
    "${stage}/lib/picolibc.ld" "${stage}/lib/picolibcpp.ld" \
    "${stage}/lib/picolibc_noflash.ld" "${stage}/lib/picolibcpp_noflash.ld" \
    "${stage}/lib/picolibc_linux.ld" "${stage}/lib/picolibcpp_linux.ld"

"${stage}/bin/ie64asm" -c -Werror -o \
    "${stage}/lib/ie64-unknown-none/crt0.o" \
    "${root_dir}/sdk/lib/ie64-cproc/crt0.s"
"${stage}/bin/ie64-cproc" --sysroot "${stage}" -c -o "${work_dir}/libatomic.o" \
    "${root_dir}/sdk/lib/ie64-cproc/libatomic.c"
"${stage}/bin/ie64-ar" rcs "${stage}/lib/ie64-unknown-none/libatomic.a" \
    "${work_dir}/libatomic.o"
install -m 0644 "${root_dir}/sdk/include/ie64.inc" "${stage}/include/ie64.inc"
install -m 0644 "${root_dir}/sdk/toolchain/README.md" "${stage}/share/ie64/docs/README.md"
install -m 0644 "${root_dir}/sdk/docs/IE64_ABI_V2.md" \
    "${stage}/share/ie64/docs/IE64_ABI_V2.md"
install -m 0644 "${root_dir}/sdk/docs/IE64_ISA.md" \
    "${stage}/share/ie64/docs/IE64_ISA.md"
install -m 0644 "${root_dir}/sdk/docs/architecture.md" \
    "${stage}/share/ie64/docs/architecture.md"
install -m 0644 "${root_dir}/sdk/toolchain/PICOLIBC_SOURCE_AUDIT.md" \
    "${stage}/share/ie64/docs/PICOLIBC_SOURCE_AUDIT.md"
install -m 0644 "${root_dir}/LICENSE" "${stage}/share/ie64/licenses/IntuitionEngine-LICENSE"
install -m 0644 "${qbe_dir}/LICENSE" "${stage}/share/ie64/licenses/QBE-LICENSE"
install -m 0644 "${cproc_dir}/LICENSE" "${stage}/share/ie64/licenses/cproc-LICENSE"
install -m 0644 "${picolibc_dir}/COPYING.picolibc" \
    "${stage}/share/ie64/licenses/COPYING.picolibc"

"${root_dir}/scripts/test-installed-ie64-toolchain.sh" "${stage}"

manifest="${stage}/share/ie64/build-manifest.txt"
{
    printf 'format=ie64-toolchain-v2-manifest-1\n'
    printf 'target=%s\n' "${TARGET}"
    printf 'host=%s\n' "${HOST}"
    printf 'release_epoch=%s\n' "${epoch}"
    printf 'intuitionengine_revision=%s\n' "$(git -C "${root_dir}" rev-parse HEAD)"
    printf 'qbe_revision=%s\n' "${QBE_REVISION}"
    printf 'cproc_revision=%s\n' "${CPROC_REVISION}"
    printf 'picolibc_revision=%s\n' "${PICOLIBC_REVISION}"
    printf 'picolibc_options=%s\n' "${PICOLIBC_OPTIONS}"
    printf 'host_abi=Linux x86-64 with glibc 2.38 or newer\n'
    printf 'ie64-cproc_version=2\nqbe_version=recorded revision\n'
    printf 'cproc-qbe_version=recorded revision\nie64asm_version=2\n'
    printf 'ie64dis_version=2\nie64ld_version=2\nie64-ar_version=2\n'
    printf 'ie64-ranlib_version=2\n'
    while IFS= read -r path; do
        relative=${path#"${stage}/"}
        [[ "${relative}" == share/ie64/build-manifest.txt ]] && continue
        printf 'file=%s  %s\n' "$(sha256sum "${path}" | cut -d' ' -f1)" "${relative}"
    done < <(find "${stage}" -type f -print | LC_ALL=C sort)
} >"${manifest}"
"${root_dir}/scripts/verify-ie64-toolchain-manifest.sh" "${stage}"

find "${stage}" -type d -exec chmod 0755 {} +
find "${stage}" -type f -exec chmod 0644 {} +
find "${stage}/bin" -type f -exec chmod 0755 {} +
touch -d "@${epoch}" "${manifest}"
find "${stage}" -exec touch -h -d "@${epoch}" {} +

mkdir -p "${root_dir}/dist"
archive="${root_dir}/dist/${package_name}.tar.xz"
rm -f "${archive}" "${archive}.sha256"
if find "${root_dir}/dist" -mindepth 1 -maxdepth 1 -print -quit | grep -q .; then
    fail "dist contains an undeclared file or directory"
fi
LC_ALL=C tar --sort=name --format=ustar --owner=0 --group=0 --numeric-owner \
    --mtime="@${epoch}" --mode='u+rwX,go+rX,go-w' -C "${work_dir}" \
    -cf - "${package_name}" | xz -9e --threads=1 --check=crc64 >"${archive}"
sha256sum "${archive}" | sed "s#  ${root_dir}/dist/#  #" >"${archive}.sha256"
touch -d "@${epoch}" "${archive}" "${archive}.sha256"

repro="${work_dir}/repro.tar.xz"
LC_ALL=C tar --sort=name --format=ustar --owner=0 --group=0 --numeric-owner \
    --mtime="@${epoch}" --mode='u+rwX,go+rX,go-w' -C "${work_dir}" \
    -cf - "${package_name}" | xz -9e --threads=1 --check=crc64 >"${repro}"
cmp "${archive}" "${repro}"

mkdir "${work_dir}/extract-a" "${work_dir}/extract-b"
tar -xf "${archive}" -C "${work_dir}/extract-a"
tar -xf "${archive}" -C "${work_dir}/extract-b"
for extracted in "${work_dir}/extract-a/${package_name}" \
                 "${work_dir}/extract-b/${package_name}"; do
    "${root_dir}/scripts/verify-ie64-toolchain-manifest.sh" "${extracted}"
    "${root_dir}/scripts/test-installed-ie64-toolchain.sh" "${extracted}"
done
"${work_dir}/extract-a/${package_name}/bin/ie64-cproc" \
    --sysroot "${work_dir}/extract-a/${package_name}" \
    -o "${work_dir}/smoke-a.ie64" \
    "${root_dir}/sdk/tests/ie64-cproc/smoke.c" \
    "${root_dir}/sdk/tests/ie64-cproc/smoke_lifecycle.s"
"${work_dir}/extract-b/${package_name}/bin/ie64-cproc" \
    --sysroot "${work_dir}/extract-b/${package_name}" \
    -o "${work_dir}/smoke-b.ie64" \
    "${root_dir}/sdk/tests/ie64-cproc/smoke.c" \
    "${root_dir}/sdk/tests/ie64-cproc/smoke_lifecycle.s"
cmp "${work_dir}/smoke-a.ie64" "${work_dir}/smoke-b.ie64"
IE64_TOOLCHAIN_IMAGE="${work_dir}/smoke-a.ie64" \
    go test -tags headless -run '^TestIE64CProcSmokeImageDefaultJIT$' \
    -count=1 "${root_dir}"
verify_checkout IntuitionEngine "${root_dir}" "$(git -C "${root_dir}" rev-parse HEAD)"
verify_checkout QBE "${qbe_dir}" "${QBE_REVISION}"
verify_checkout cproc "${cproc_dir}" "${CPROC_REVISION}"
verify_checkout Picolibc "${picolibc_dir}" "${PICOLIBC_REVISION}"
echo "created ${archive}"
