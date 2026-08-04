#!/usr/bin/env bash
set -euo pipefail

root=$1
[[ -x "${root}/bin/ie64-cproc" ]] || { echo 'missing installed driver' >&2; exit 1; }
for document in README.md IE64_ISA.md architecture.md; do
    [[ -f "${root}/share/ie64/docs/${document}" ]] || {
        echo "missing installed toolchain documentation: ${document}" >&2
        exit 1
    }
done
for document in IE64_ABI_V3.md IE64_C23_FEATURE_MATRIX.md; do
    [[ ! -e "${root}/share/ie64/docs/${document}" ]] || {
        echo "internal toolchain document is installed: ${document}" >&2
        exit 1
    }
done
grep -q '^## Quick start$' "${root}/share/ie64/docs/README.md" || {
    echo 'installed README has no compiler quick start' >&2
    exit 1
}
grep -q 'ie64-cproc' "${root}/share/ie64/docs/README.md" || {
    echo 'installed README does not describe the compiler driver' >&2
    exit 1
}
tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT
cat >"${tmp}/answer.c" <<'EOF'
int answer(void) { return 42; }
EOF
cat >"${tmp}/main.c" <<'EOF'
extern int answer(void);
int main(void) { return answer() == 42 ? 0 : 1; }
EOF
cat >"${tmp}/optimisation.c" <<'EOF'
int optimisation_slot;
int optimisation_probe(int value) {
    optimisation_slot = value;
    return optimisation_slot;
}
EOF
cat >"${tmp}/start.s" <<'EOF'
include "ie64.inc"
.text
.global packaged_assembly_symbol
packaged_assembly_symbol:
    move.q r1, #PROG_START
    rts
EOF
"${root}/bin/ie64-cproc" --sysroot "${root}" -c -o "${tmp}/answer.o" "${tmp}/answer.c"
"${root}/bin/ie64-ar" rcs "${tmp}/libanswer.a" "${tmp}/answer.o"
"${root}/bin/ie64-ranlib" "${tmp}/libanswer.a"
"${root}/bin/ie64-cproc" --sysroot "${root}" -o "${tmp}/program.ie64" \
    "${tmp}/main.c" "${tmp}/libanswer.a"
for level in 0 1 2 3; do
    "${root}/bin/ie64-cproc" --sysroot "${root}" "-O${level}" -S \
        -o "${tmp}/optimisation-O${level}.s" "${tmp}/optimisation.c"
    test -s "${tmp}/optimisation-O${level}.s"
done
! cmp -s "${tmp}/optimisation-O0.s" "${tmp}/optimisation-O1.s"
"${root}/bin/ie64asm" -I "${root}/include" -c -o "${tmp}/start.o" "${tmp}/start.s"
"${root}/bin/ie64dis" "${tmp}/program.ie64" >"${tmp}/program.dis"
/usr/bin/env -i PATH=/nonexistent TMPDIR="${tmp}" \
    "${root}/bin/ie64-cproc" --sysroot "${root}" -c \
    -o "${tmp}/restricted.o" "${tmp}/answer.c"
/usr/bin/env -i PATH=/nonexistent \
    "${root}/bin/ie64-ar" rcs "${tmp}/restricted.a" "${tmp}/restricted.o"
/usr/bin/env -i PATH=/nonexistent \
    "${root}/bin/ie64-ranlib" "${tmp}/restricted.a"
/usr/bin/env -i PATH=/nonexistent TMPDIR="${tmp}" \
    "${root}/bin/ie64-cproc" --sysroot "${root}" -o \
    "${tmp}/restricted.ie64" "${tmp}/main.c" "${tmp}/restricted.a"
test -s "${tmp}/program.ie64"
test -s "${tmp}/program.dis"
test -s "${tmp}/restricted.ie64"
