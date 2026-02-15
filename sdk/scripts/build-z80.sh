#!/bin/bash
# Build Z80 assembly examples for IntuitionEngine SDK
# Requires: vasmz80_std (http://sun.hasenbraten.de/vasm/)
# Usage: ./build-z80.sh [source.asm]
# If no argument given, builds all Z80 examples.

set -euo pipefail

SDK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ASM_DIR="$SDK_DIR/examples/asm"
INCLUDE_DIR="$SDK_DIR/include"
VASM="${VASM_Z80:-vasmz80_std}"

if ! command -v "$VASM" &>/dev/null; then
    echo "Error: vasmz80_std not found in PATH."
    echo "Install VASM from: http://sun.hasenbraten.de/vasm/"
    echo "Or set VASM_Z80 to the path to the assembler."
    exit 1
fi

build_one() {
    local src="$1"
    local base
    base="$(basename "$src" .asm)"
    echo "  [Z80] $base.asm -> $base.ie80"
    (cd "$(dirname "$src")" && "$VASM" -Fbin -I "$INCLUDE_DIR" -o "$base.ie80" "$(basename "$src")")
}

if [ $# -gt 0 ]; then
    build_one "$1"
else
    for src in "$ASM_DIR"/rotozoomer_z80.asm; do
        if [ -f "$src" ]; then
            build_one "$src"
        fi
    done
fi
