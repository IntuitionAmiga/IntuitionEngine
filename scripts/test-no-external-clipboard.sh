#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

if rg -n "golang\\.design/x/clipboard" . >/dev/null; then
	echo "found forbidden external clipboard dependency"
	exit 1
fi
