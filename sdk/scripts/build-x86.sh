#!/bin/bash
# Build x86 (32-bit) assembly examples for IntuitionEngine SDK
# Requires: nasm (https://www.nasm.us/)
# Usage: ./build-x86.sh [source.asm]
# If no argument given, builds all x86 examples.

set -euo pipefail

SDK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ASM_DIR="$SDK_DIR/examples/asm"
INCLUDE_DIR="$SDK_DIR/include"
NASM="${NASM:-nasm}"

if ! command -v "$NASM" &>/dev/null; then
    echo "Error: nasm not found in PATH."
    echo "Install NASM from: https://www.nasm.us/"
    exit 1
fi

build_one() {
    local src="$1"
    local base
    base="$(basename "$src" .asm)"
    echo "  [x86] $base.asm -> $base.ie86"
    (cd "$(dirname "$src")" && "$NASM" -f bin -I "$INCLUDE_DIR/" -o "$base.ie86" "$(basename "$src")")
}

if [ $# -gt 0 ]; then
    build_one "$1"
else
    for src in "$ASM_DIR"/rotozoomer_x86.asm \
               "$ASM_DIR"/antic_plasma_x86.asm; do
        if [ -f "$src" ]; then
            build_one "$src"
        fi
    done
fi
