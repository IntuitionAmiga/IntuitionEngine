#!/bin/bash
# Build IE64 assembly examples for IntuitionEngine SDK
# Usage: ./build-ie64.sh [source.asm]
# If no argument given, builds all IE64 examples.

set -euo pipefail

SDK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ASM_DIR="$SDK_DIR/examples/asm"
INCLUDE_DIR="$SDK_DIR/include"
BIN_DIR="${IE_BIN_DIR:-./bin}"
IE64ASM="${BIN_DIR}/ie64asm"

if [ ! -f "$IE64ASM" ]; then
    echo "Error: ie64asm not found at $IE64ASM"
    echo "Build it first with: make ie64asm"
    echo "Or set IE_BIN_DIR to the directory containing ie64asm."
    exit 1
fi

build_one() {
    local src="$1"
    local base
    base="$(basename "$src" .asm)"
    echo "  [IE64] $base.asm -> $base.ie64"
    (cd "$(dirname "$src")" && "$IE64ASM" -I "$INCLUDE_DIR" "$(basename "$src")")
}

if [ $# -gt 0 ]; then
    build_one "$1"
else
    for src in "$ASM_DIR"/rotozoomer_ie64.asm; do
        if [ -f "$src" ]; then
            build_one "$src"
        fi
    done
fi
