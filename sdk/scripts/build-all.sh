#!/bin/bash
# Build all SDK examples across all CPU targets
# Skips targets whose assembler is not installed.
#
# Usage: ./build-all.sh
#
# Environment variables:
#   IE_BIN_DIR   - Path to IntuitionEngine bin/ directory (default: ./bin)
#   VASM_M68K    - Path to vasmm68k_mot (default: vasmm68k_mot)
#   VASM_Z80     - Path to vasmz80_std (default: vasmz80_std)
#   CA65 / LD65  - Path to cc65 tools (default: ca65 / ld65)
#   NASM         - Path to nasm (default: nasm)

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BUILT=0
SKIPPED=0
FAILED=0

run_builder() {
    local name="$1"
    local script="$2"
    echo "=== Building $name examples ==="
    if bash "$script" 2>&1; then
        ((BUILT++))
    else
        echo "  WARNING: $name build had errors"
        ((FAILED++))
    fi
    echo
}

skip_builder() {
    local name="$1"
    local tool="$2"
    echo "=== Skipping $name examples ($tool not found) ==="
    ((SKIPPED++))
    echo
}

# IE32 (built-in assembler)
BIN_DIR="${IE_BIN_DIR:-./bin}"
if [ -f "$BIN_DIR/ie32asm" ]; then
    run_builder "IE32" "$SCRIPT_DIR/build-ie32.sh"
else
    skip_builder "IE32" "ie32asm"
fi

# IE64 (built-in assembler)
if [ -f "$BIN_DIR/ie64asm" ]; then
    run_builder "IE64" "$SCRIPT_DIR/build-ie64.sh"
else
    skip_builder "IE64" "ie64asm"
fi

# M68K (external: vasmm68k_mot)
if command -v "${VASM_M68K:-vasmm68k_mot}" &>/dev/null; then
    run_builder "M68K" "$SCRIPT_DIR/build-m68k.sh"
else
    skip_builder "M68K" "vasmm68k_mot"
fi

# Z80 (external: vasmz80_std)
if command -v "${VASM_Z80:-vasmz80_std}" &>/dev/null; then
    run_builder "Z80" "$SCRIPT_DIR/build-z80.sh"
else
    skip_builder "Z80" "vasmz80_std"
fi

# 6502 (external: cc65)
if command -v "${CA65:-ca65}" &>/dev/null; then
    run_builder "6502" "$SCRIPT_DIR/build-6502.sh"
else
    skip_builder "6502" "ca65"
fi

# x86 (external: nasm)
if command -v "${NASM:-nasm}" &>/dev/null; then
    run_builder "x86" "$SCRIPT_DIR/build-x86.sh"
else
    skip_builder "x86" "nasm"
fi

echo "==============================="
echo "  Built:   $BUILT target(s)"
echo "  Skipped: $SKIPPED target(s)"
echo "  Failed:  $FAILED target(s)"
echo "==============================="

[ "$FAILED" -eq 0 ]
