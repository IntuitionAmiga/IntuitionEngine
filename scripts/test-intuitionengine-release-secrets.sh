#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fail(){ echo "FAIL: $*" >&2; exit 1; }
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
private_key_marker="-----BEGIN PGP PRIVATE KEY"
private_key_marker+=" BLOCK-----"

repo="$tmp/repo"
mkdir -p "$repo/intuitionengine.com"
git -C "$repo" init -q
printf 'safe\n' >"$repo/README"
git -C "$repo" add README
"$root_dir/scripts/check-intuitionengine-release-secrets.sh" --root "$repo" --output "$repo/intuitionengine.com" >/dev/null || fail "clean repository was rejected"

printf '%s\n' "$private_key_marker" >"$repo/tracked-private.key"
git -C "$repo" add tracked-private.key
if "$root_dir/scripts/check-intuitionengine-release-secrets.sh" --root "$repo" --output "$repo/intuitionengine.com" >/dev/null 2>&1; then
    fail "tracked private key was accepted"
fi
rm -f "$repo/tracked-private.key"
git -C "$repo" add -u

printf '%s\n' "$private_key_marker" >"$repo/staged-material.txt"
git -C "$repo" add staged-material.txt
if "$root_dir/scripts/check-intuitionengine-release-secrets.sh" --root "$repo" --output "$repo/intuitionengine.com" >/dev/null 2>&1; then
    fail "staged private key material was accepted"
fi
rm -f "$repo/staged-material.txt"
git -C "$repo" add -u

printf '%s\n' "$private_key_marker" >"$repo/intuitionengine.com/release.asc"
if "$root_dir/scripts/check-intuitionengine-release-secrets.sh" --root "$repo" --output "$repo/intuitionengine.com" >/dev/null 2>&1; then
    fail "private key below release output was accepted"
fi

echo "IntuitionEngine release secret contracts passed"
