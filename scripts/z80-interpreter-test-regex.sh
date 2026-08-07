#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

names="$({ rg --no-filename '^func Test[[:alnum:]_]+' cpu_z80*_test.go || true; } \
	| sed -E 's/^func (Test[[:alnum:]_]+).*/\1/' \
	| grep -v '^TestCPUZ80CacheLineLayout$' \
	| sort -u)"

if [[ -z "$names" ]]; then
	echo "empty pre-JIT Z80 test inventory" >&2
	exit 1
fi

batch_size="${1:-0}"
if ! [[ "$batch_size" =~ ^[0-9]+$ ]]; then
	echo "batch size must be a non-negative integer" >&2
	exit 1
fi

if (( batch_size == 0 )); then
	printf '^(%s)$\n' "$(printf '%s\n' "$names" | paste -sd '|' -)"
	exit 0
fi

printf '%s\n' "$names" | awk -v size="$batch_size" '
	{ if (count == 0) regex = "^(" $0; else regex = regex "|" $0; count++ }
	count == size { print regex ")$"; regex = ""; count = 0 }
	END { if (regex != "") print regex ")$" }
'
