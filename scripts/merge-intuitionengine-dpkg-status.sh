#!/usr/bin/env bash
set -euo pipefail

fail(){ echo "merge-intuitionengine-dpkg-status: $*" >&2; exit 1; }
input= output=
while (($#)); do
    case "$1" in
        --input|--output)
            (($# >= 2)) || fail "missing value for $1"
            if [[ "$1" == --input ]]; then input=$2; else output=$2; fi
            shift 2
            ;;
        *) fail "unknown argument: $1" ;;
    esac
done
[[ -f "$input" && -n "$output" ]] || fail "input and output are required"

awk 'BEGIN { RS=""; ORS="\n\n" }
    $0 !~ /(^|\n)Package: intuitionengine-(amd64-v3|arm64-pi4|arm64-pi5)(\n|$)/ { print }' "$input" >"$output"
