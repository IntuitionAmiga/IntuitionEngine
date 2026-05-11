#!/usr/bin/env bash
#
# kmake.sh — multi-file wrapper for m68kto64 + ie64asm (AB3D2-style projects).
#
# ie64asm is single-file. AB3D2-style codebases rely on cross-unit linkage via
# xdef/xref (which m68kto64 drops) and on `include` directives that the
# m68kto64 preprocessor resolves before assembly. This script concatenates a
# list of input m68k sources into one transpiled IE64 source, then assembles.
#
# Usage:
#   kmake.sh -o out.bin [-I dir]... [-D NAME[=VAL]]... file1.s file2.s ...
#
# -I directories are forwarded to m68kto64 (transpile-time `include`
# resolution, Phase D of the vasm/devpac preprocessor) AND to ie64asm
# (assemble-time `incbin` / native `include` resolution). -D NAME[=VAL] flags
# are forwarded to m68kto64 only. Each input file is transpiled in isolation;
# outputs are concatenated with a banner so assembler errors point back to
# the source file.

set -euo pipefail

usage() {
    echo "Usage: $0 -o <out.bin> [-I <dir>]... [-D <NAME[=VAL]>]... <input.s>..." >&2
    exit 2
}

OUT=""
INCLUDES=()
DEFINES=()
INPUTS=()

while (( $# )); do
    case "$1" in
        -o) OUT="$2"; shift 2;;
        -I) INCLUDES+=("$2"); shift 2;;
        -D) DEFINES+=("$2"); shift 2;;
        -h|--help) usage;;
        --) shift; INPUTS+=("$@"); break;;
        -*) echo "unknown flag: $1" >&2; usage;;
        *)  INPUTS+=("$1"); shift;;
    esac
done

if [[ -z "$OUT" ]]; then usage; fi
if (( ${#INPUTS[@]} == 0 )); then usage; fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# Two install layouts: sdk/scripts/ab3d2/kmake.sh (three levels deep) and
# sdk/bin/m68kto64-kmake (two levels deep). Walk up until sdk/bin lives
# alongside, so both invocation forms find the tools.
REPO_ROOT="$SCRIPT_DIR"
while [[ "$REPO_ROOT" != "/" && ! -d "$REPO_ROOT/sdk/bin" ]]; do
    REPO_ROOT="$(dirname "$REPO_ROOT")"
done
if [[ ! -d "$REPO_ROOT/sdk/bin" ]]; then
    echo "kmake.sh: cannot locate sdk/bin from $SCRIPT_DIR" >&2
    exit 1
fi
M68KTO64="${M68KTO64:-$REPO_ROOT/sdk/bin/m68kto64}"
IE64ASM="${IE64ASM:-$REPO_ROOT/sdk/bin/ie64asm}"

if [[ ! -x "$M68KTO64" ]]; then
    echo "kmake.sh: $M68KTO64 not found; run 'make m68kto64' first" >&2
    exit 1
fi
if [[ ! -x "$IE64ASM" ]]; then
    echo "kmake.sh: $IE64ASM not found; run 'make ie64asm' first" >&2
    exit 1
fi

WORK="$(mktemp -d -t m68kto64.XXXXXX)"
trap 'rm -rf "$WORK"' EXIT

CONCAT="$WORK/concat.ie64.s"
: > "$CONCAT"

# Build m68kto64 invocation prefix with -I / -D flags.
M68K_FLAGS=()
for d in "${INCLUDES[@]}"; do M68K_FLAGS+=( -I "$d" ); done
for s in "${DEFINES[@]}"; do M68K_FLAGS+=( -D "$s" ); done

# Optional extra flags for m68kto64 (e.g. -strip-cond). Provided as a single
# whitespace-separated string in M68KTO64_EXTRA_FLAGS, since this is a thin
# wrapper and a full flag-passthrough syntax would invite ambiguity with
# kmake's own option parsing.
if [[ -n "${M68KTO64_EXTRA_FLAGS:-}" ]]; then
    # shellcheck disable=SC2206  # intentional word-split
    M68K_FLAGS+=( ${M68KTO64_EXTRA_FLAGS} )
fi

for src in "${INPUTS[@]}"; do
    base="$(basename "$src")"
    converted="$WORK/${base}.ie64.s"
    "$M68KTO64" "${M68K_FLAGS[@]}" -o "$converted" "$src"
    {
        echo
        echo "; ========================================================================="
        echo "; --- file: $src"
        echo "; ========================================================================="
        cat "$converted"
    } >> "$CONCAT"
done

# Cross-TU equ/set dedup. Each TU is transpiled with an independent
# symtab, so the same `LABEL equ VAL` line can land in the concat from
# multiple sources. ie64asm rejects a duplicate symbol even when the
# values match for `set`, and rejects mismatched `equ`. Drop second-
# and-later definitions (first one wins) and emit a comment trace so
# the diff is visible in the assembled output.
DEDUP_AWK='
{
    # Only dedup `equ` (immutable). `set` is mutable and is used by the
    # transpilers `__m68kto64_rs` offset counter — dropping later sets
    # would freeze the counter at zero and silently break every rs-
    # bound symbol after the first.
    if (match($0, /^[ \t]*([A-Za-z_.][A-Za-z0-9_.]*)[ \t]+equ[ \t]/, m)) {
        sym = m[1]
        if (sym in seen_equ) {
            print "; m68kto64-kmake: dedup equ " sym " (kept first)"
            next
        }
        seen_equ[sym] = 1
    }
    print
}'
awk "$DEDUP_AWK" "$CONCAT" > "$CONCAT.dedup" && mv "$CONCAT.dedup" "$CONCAT"

CMD=( "$IE64ASM" )
for d in "${INCLUDES[@]}"; do
    CMD+=( -I "$d" )
done
CMD+=( -o "$OUT" "$CONCAT" )

"${CMD[@]}"
