#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root_dir"

declare -a files=(
  README.md
  CHANGELOG.md
  DEVELOPERS.md
  CLAUDE.md
  AGENTS.md
  sdk/README.md
  sdk/docs/*.md
)

check_forbidden() {
  local pattern="$1"
  local message="$2"
  if rg -n --glob '!sdk/docs/IntuitionOS/*' "$pattern" "${files[@]}" >/tmp/doc-consistency.out 2>/dev/null; then
    echo "$message" >&2
    cat /tmp/doc-consistency.out >&2
    exit 1
  fi
}

check_make_target_exists() {
  local target="$1"
  if ! make -n "$target" >/dev/null 2>&1; then
    echo "documented make target is missing or has invalid dry-run: $target" >&2
    exit 1
  fi
}

check_make_target_documented() {
  local target="$1"
  local pattern="make[[:space:]]+${target}\\b|[[:space:]]${target}[[:space:]]+-"
  if ! rg -q --glob '!sdk/docs/IntuitionOS/*' "$pattern" "${files[@]}"; then
    echo "make target is not documented: $target" >&2
    exit 1
  fi
}

check_forbidden 'macOS and BSD variants .* not currently supported' \
  'stale platform claim found: macOS is now a supported release target on arm64'
check_forbidden 'ebiten and oto require CGO|release builds require CGO|cross-compile.*require CGO|macOS.*require CGO|Windows.*require CGO' \
  'stale platform-level CGO claim found in docs'
check_forbidden 'cross-compile (is )?not possible' \
  'stale cross-compile claim found in docs'
check_forbidden 'osxcross' \
  'stale osxcross reference found in docs'

new_make_targets=(
  test
  vet
  tidy
  test-makefile
  testdata-x86
  test-x86-harte
  test-x86-harte-short
  release-verify
  distclean
)

for target in "${new_make_targets[@]}"; do
  check_make_target_exists "$target"
  check_make_target_documented "$target"
done

echo "documentation consistency checks passed"
