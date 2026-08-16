#!/usr/bin/env bash
set -euo pipefail

fail(){ echo "bump-intuitionengine-version: $*" >&2; exit 1; }
version_file=VERSION
while (($#)); do
    case "$1" in
        --file)
            (($# >= 2)) || fail "missing value for --file"
            version_file=$2
            shift 2
            ;;
        *) fail "unknown argument: $1" ;;
    esac
done

[[ -f "$version_file" ]] || fail "missing version file: $version_file"
current="$(tr -d '[:space:]' <"$version_file")"
[[ "$current" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]] || fail "invalid semantic version: $current"
next_patch=$((10#${BASH_REMATCH[3]} + 1))
next="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.$next_patch"
tmp="$(mktemp "${version_file}.tmp.XXXXXX")"
trap 'rm -f "$tmp"' EXIT
printf '%s\n' "$next" >"$tmp"
mv "$tmp" "$version_file"
trap - EXIT
printf '%s\n' "$next"
