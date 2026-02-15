#!/bin/bash
# Build 6502 assembly examples for IntuitionEngine SDK
# Requires: ca65/ld65 (cc65 toolchain, https://cc65.github.io/)
# Usage: ./build-6502.sh [source.asm]
# If no argument given, builds all 6502 examples.

set -euo pipefail

SDK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ASM_DIR="$SDK_DIR/examples/asm"
INCLUDE_DIR="$SDK_DIR/include"
CFG="$INCLUDE_DIR/ie65.cfg"
CA65="${CA65:-ca65}"
LD65="${LD65:-ld65}"

if ! command -v "$CA65" &>/dev/null; then
    echo "Error: ca65 not found in PATH."
    echo "Install cc65 from: https://cc65.github.io/"
    exit 1
fi

build_one() {
    local src="$1"
    local base
    base="$(basename "$src" .asm)"
    local dir
    dir="$(dirname "$src")"
    echo "  [6502] $base.asm -> $base.ie65"
    (cd "$dir" && \
     "$CA65" --cpu 6502 -I "$INCLUDE_DIR" -o "$base.o" "$(basename "$src")" && \
     "$LD65" -C "$CFG" -o "$base.ie65" "$base.o" && \
     rm -f "$base.o")
}

if [ $# -gt 0 ]; then
    build_one "$1"
else
    for src in "$ASM_DIR"/rotozoomer_65.asm \
               "$ASM_DIR"/ula_rotating_cube_65.asm; do
        if [ -f "$src" ]; then
            build_one "$src"
        fi
    done
fi
