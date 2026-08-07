#!/usr/bin/env bash
set -u

if [[ "$#" -lt 2 ]]; then
  echo "usage: $0 <empty-inventory message> <go test -list command...>" >&2
  exit 2
fi

empty_message="$1"
shift

output_file="$(mktemp)" || exit 1
trap 'rm -f "$output_file"' EXIT

"$@" >"$output_file" 2>&1
status=$?
if [[ "$status" -ne 0 ]]; then
  cat "$output_file" >&2
  exit "$status"
fi

if ! sed -n '/^Test/p' "$output_file" | grep -q '^Test'; then
  echo "$empty_message" >&2
  exit 1
fi
