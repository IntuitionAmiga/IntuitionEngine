#!/bin/bash
# Build the M68K CPU test suite and shard binaries.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
VASM="${VASM_M68K:-vasmm68k_mot}"
SRC_DIR="$ROOT_DIR/sdk/cputest"
INCLUDE_DIR="$ROOT_DIR/sdk/cputest/include"
OUT_DIR="$ROOT_DIR/sdk/examples/prebuilt"

if ! command -v "$VASM" >/dev/null 2>&1; then
    echo "Error: vasmm68k_mot not found in PATH."
    exit 1
fi

echo "Generating M68K CPU test case includes..."
(cd "$ROOT_DIR" && go run ./cmd/gen_m68k_cputest)

mkdir -p "$OUT_DIR"

build_one() {
    local src="$1"
    local base
    base="$(basename "$src" .asm)"
    echo "  [CPUTEST] $base.asm -> $base"
    (cd "$(dirname "$src")" && "$VASM" -Fhunk -m68020 -m68881 -devpac -I "$INCLUDE_DIR" -o "$OUT_DIR/$base" "$(basename "$src")")
}

build_one "$SRC_DIR/cputest_suite.asm"

echo "CPU test binaries written to $OUT_DIR"
