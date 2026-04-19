#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

AROS_SRC_DIR="${AROS_SRC_DIR:-../AROS}"
AROS_BUILD_DIR="${AROS_BUILD_DIR:-$AROS_SRC_DIR/bin/ie-m68k}"
AROS_TREE="$AROS_BUILD_DIR/bin/ie-m68k/AROS"
AROS_ROM="${AROS_ROM:-./sdk/examples/prebuilt/aros-ie.rom}"

test -s "$AROS_TREE/S/Startup-Sequence"
test -s "$AROS_TREE/Libs/iffparse.library"
test -s "$AROS_TREE/Libs/kms.library"
test -s "$AROS_TREE/Libs/locale.library"
test -d "$AROS_TREE/Fonts"
test -s "$AROS_ROM"
