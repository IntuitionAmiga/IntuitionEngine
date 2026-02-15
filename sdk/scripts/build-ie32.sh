#!/bin/bash
# Build IE32 assembly examples for IntuitionEngine SDK
# Usage: ./build-ie32.sh [source.asm]
# If no argument given, builds all IE32 examples.

set -euo pipefail

SDK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ASM_DIR="$SDK_DIR/examples/asm"
INCLUDE_DIR="$SDK_DIR/include"
BIN_DIR="${IE_BIN_DIR:-./sdk/bin}"
IE32ASM="${BIN_DIR}/ie32asm"

if [ ! -f "$IE32ASM" ]; then
    echo "Error: ie32asm not found at $IE32ASM"
    echo "Build it first with: make ie32asm"
    echo "Or set IE_BIN_DIR to the directory containing ie32asm."
    exit 1
fi

build_one() {
    local src="$1"
    local base
    base="$(basename "$src" .asm)"
    echo "  [IE32] $base.asm -> $base.iex"
    (cd "$(dirname "$src")" && "$IE32ASM" -I "$INCLUDE_DIR" "$(basename "$src")")
}

if [ $# -gt 0 ]; then
    build_one "$1"
else
    for src in "$ASM_DIR"/rotozoomer.asm \
               "$ASM_DIR"/vga_text_hello.asm \
               "$ASM_DIR"/vga_mode13h_fire.asm \
               "$ASM_DIR"/copper_vga_bands.asm \
               "$ASM_DIR"/coproc_caller_ie32.asm \
               "$ASM_DIR"/coproc_service_ie32.asm; do
        if [ -f "$src" ]; then
            build_one "$src"
        fi
    done
fi
