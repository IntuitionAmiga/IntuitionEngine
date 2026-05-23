#!/usr/bin/env bash
# PRM doc-as-test orchestrator.
#
# Usage:
#   scripts/run_prm_tests.sh                # all chapters
#   scripts/run_prm_tests.sh '01-*'         # one chapter / glob
#
# Phases run sequentially, but each is wrapped in `set +e` so the
# renderer always gets a chance to produce report.md even when earlier
# phases fail. The final exit code is non-zero if any phase failed.

set -eu

glob="${1:-*}"
ie_binary="${IE_BINARY:-./bin/IntuitionEngine}"

if [ ! -x "$ie_binary" ]; then
  echo "ERROR: $ie_binary not found or not executable" >&2
  echo "       Build with: make intuition-engine" >&2
  exit 2
fi

go run ./tools/prm-extract \
  -glob "$glob" \
  -out  sdk/scripts/prm-runner/cases.json \
  -build sdk/scripts/prm-runner/build

set +e
"$ie_binary" -basic -term -script-owned-term \
  -script sdk/scripts/prm-runner/prm_runner.ies
runner_rc=$?

go run ./tools/prm-extract -run-ies \
  -in    sdk/scripts/prm-runner/cases.json \
  -build sdk/scripts/prm-runner/build \
  -append sdk/scripts/prm-runner/report.json \
  -ie-binary "$ie_binary"
ies_rc=$?

go run ./tools/prm-extract -run-iemon \
  -in    sdk/scripts/prm-runner/cases.json \
  -build sdk/scripts/prm-runner/build \
  -append sdk/scripts/prm-runner/report.json \
  -ie-binary "$ie_binary"
iemon_rc=$?

go run ./tools/prm-extract -render \
  -in  sdk/scripts/prm-runner/report.json \
  -out tools/prm-extract/report.md
render_rc=$?
set -e

echo "phase exit codes: runner=$runner_rc ies=$ies_rc iemon=$iemon_rc render=$render_rc"
if [ "$runner_rc" -ne 0 ] || [ "$ies_rc" -ne 0 ] || \
   [ "$iemon_rc" -ne 0 ] || [ "$render_rc" -ne 0 ]; then
  exit 1
fi
exit 0
