#!/bin/bash
# Build M68K assembly examples for IntuitionEngine SDK
# Requires: vasmm68k_mot (http://sun.hasenbraten.de/vasm/)
# Usage: ./build-m68k.sh [source.asm]
# If no argument given, builds all M68K examples.

set -euo pipefail

SDK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ASM_DIR="$SDK_DIR/examples/asm"
INCLUDE_DIR="$SDK_DIR/include"
VASM="${VASM_M68K:-vasmm68k_mot}"

if ! command -v "$VASM" &>/dev/null; then
    echo "Error: vasmm68k_mot not found in PATH."
    echo "Install VASM from: http://sun.hasenbraten.de/vasm/"
    echo "Or set VASM_M68K to the path to the assembler."
    exit 1
fi

build_one() {
    local src="$1"
    local base
    base="$(basename "$src" .asm)"
    echo "  [M68K] $base.asm -> $base.ie68"
    (cd "$(dirname "$src")" && "$VASM" -Fbin -m68020 -devpac -I "$INCLUDE_DIR" -o "$base.ie68" "$(basename "$src")")
}

if [ $# -gt 0 ]; then
    build_one "$1"
else
    for src in "$ASM_DIR"/rotozoomer_68k.asm \
               "$ASM_DIR"/ted_121_colors_68k.asm \
               "$ASM_DIR"/voodoo_cube_68k.asm; do
        if [ -f "$src" ]; then
            build_one "$src"
        fi
    done
fi
