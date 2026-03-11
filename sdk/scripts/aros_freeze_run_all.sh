#!/bin/bash
# Run all 4 freeze harness scenarios sequentially.
# Each run gets its own emulator instance (fresh boot).
set -e
DIR="$(cd "$(dirname "$0")/../.." && pwd)"
BIN="$DIR/bin/IntuitionEngine"
SCRIPT="$DIR/sdk/scripts/aros_freeze_harness.ies"

run_scenario() {
    local scenario="$1"
    local timeout="$2"
    echo "=== Running scenario: $scenario (timeout ${timeout}s) ==="
    # Patch SCENARIO in a temp copy
    local tmp
    tmp=$(mktemp /tmp/harness_XXXXXX.ies)
    sed "s/^local SCENARIO.*=.*/local SCENARIO           = \"$scenario\"/" "$SCRIPT" > "$tmp"
    timeout "$timeout" "$BIN" -aros -script "$tmp" > /dev/null 2>&1 || true
    rm -f "$tmp"
    echo "=== $scenario complete ==="
    echo ""
}

# idle:  15s boot + 30s sample + 10s margin = 55s
run_scenario "idle"  55

# mouse: 15s boot + 30s sample + 10s margin = 55s
run_scenario "mouse" 55

# click: 15s boot + 30s sample + 10s margin = 55s
run_scenario "click" 55

# mixed: 15s boot + 60s sample + 10s margin = 85s
run_scenario "mixed" 85

echo "All scenarios complete. Logs:"
ls -la "$DIR"/sdk/scripts/aros_freeze_*.log 2>/dev/null
