#!/usr/bin/env bash
set -euo pipefail

fail(){ echo "bump-intuitionengine-version: $*" >&2; exit 1; }
version_file=VERSION
html_file=
while (($#)); do
    case "$1" in
        --file)
            (($# >= 2)) || fail "missing value for --file"
            version_file=$2
            shift 2
            ;;
        --html-file)
            (($# >= 2)) || fail "missing value for --html-file"
            html_file=$2
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
html_tmp=
if [[ -n "$html_file" ]]; then
    [[ -f "$html_file" ]] || fail "missing HTML file: $html_file"
    matches="$(rg -o 'Download the Intuition Engine [0-9]+\.[0-9]+\.[0-9]+ live image' "$html_file" | wc -l)"
    [[ "$matches" == 2 ]] || fail "HTML file has $matches versioned live-image links, want 2: $html_file"
    html_tmp="$(mktemp "${html_file}.tmp.XXXXXX")"
    trap 'rm -f "$html_tmp"' EXIT
    sed -E "s/(Download the Intuition Engine )[0-9]+\.[0-9]+\.[0-9]+( live image)/\1${next}\2/g" "$html_file" >"$html_tmp"
fi
tmp="$(mktemp "${version_file}.tmp.XXXXXX")"
trap 'rm -f "$tmp" "$html_tmp"' EXIT
printf '%s\n' "$next" >"$tmp"
mv "$tmp" "$version_file"
if [[ -n "$html_file" ]]; then
    mv "$html_tmp" "$html_file"
fi
trap - EXIT
printf '%s\n' "$next"
