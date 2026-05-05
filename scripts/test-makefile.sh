#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root_dir"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

make_db() {
  make -pn help 2>/dev/null
}

make_dry() {
  make -n "$@" 2>&1
}

assert_target_exists() {
  local target="$1"
  rg -q "^${target}:" Makefile || fail "target does not exist: $target"
}

assert_phony() {
  local target="$1"
  rg -q "^\\.PHONY:.*(^|[[:space:]])${target}([[:space:]]|\$)" Makefile || \
    fail "target is not declared .PHONY: $target"
}

assert_recipe_contains() {
  local target="$1"
  local regex="$2"
  shift 2
  make_dry "$target" "$@" | rg -q "$regex" || fail "$target recipe does not match: $regex"
}

assert_recipe_not_contains() {
  local target="$1"
  local regex="$2"
  shift 2
  if make_dry "$target" "$@" | rg -q "$regex"; then
    fail "$target recipe unexpectedly matches: $regex"
  fi
}

assert_var() {
  local name="$1"
  local expected="${2:-}"
  local line
  line="$(make_db | rg "^${name} [?:]?= " | head -n 1 || true)"
  [[ -n "$line" ]] || fail "variable not found: $name"
  local value="${line#*= }"
  if [[ -n "$expected" && "$value" != "$expected" ]]; then
    fail "variable $name expected '$expected', got '$line'"
  fi
  if [[ -z "$expected" && -z "$value" ]]; then
    fail "variable $name is empty"
  fi
}

assert_no_dup_assign() {
  local name="$1"
  local count
  count="$(rg -n "^${name}[[:space:]]*[:?]?=" Makefile | wc -l)"
  [[ "$count" -le 1 ]] || fail "variable $name has duplicate assignments ($count)"
}

assert_set_e_loop() {
  local target="$1"
  assert_recipe_contains "$target" 'set -e;'
}

assert_delete_on_error() {
  rg -q '^\.DELETE_ON_ERROR:' Makefile || fail ".DELETE_ON_ERROR is missing"
}

assert_makefile_contains() {
  local regex="$1"
  rg -q "$regex" Makefile || fail "Makefile does not match: $regex"
}

assert_release_src_pipefail_runtime() {
  local tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN
  cat >"$tmp/git" <<'STUB'
#!/usr/bin/env sh
exit 1
STUB
  chmod +x "$tmp/git"
  if make release-src GIT="$tmp/git" RELEASE_DIR="$tmp/release" >/tmp/make-release-src.out 2>&1; then
    cat /tmp/make-release-src.out >&2
    fail "release-src succeeded with failing git stub"
  fi
}

assert_install_runtime_destdir() {
  local tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN
  mkdir -p "$tmp/bin" "$tmp/sdk/bin"
  touch "$tmp/bin/IntuitionEngine" "$tmp/sdk/bin/ie32asm"
  make install BIN_DIR="$tmp/bin" SDK_BIN_DIR="$tmp/sdk/bin" DESTDIR="$tmp/root" >/tmp/make-install.out 2>&1 || {
    cat /tmp/make-install.out >&2
    fail "install with DESTDIR failed"
  }
  [[ -f "$tmp/root/usr/local/bin/IntuitionEngine" ]] || fail "DESTDIR install did not create IntuitionEngine"
  [[ -f "$tmp/root/usr/local/bin/ie32asm" ]] || fail "DESTDIR install did not create ie32asm"
  if rg -q 'sudo' /tmp/make-install.out; then
    cat /tmp/make-install.out >&2
    fail "DESTDIR install invoked sudo"
  fi
}

assert_rotozoom_single_invocation() {
  local count
  count="$(make -n -B -j4 rotozoom-textures 2>/dev/null | rg -c 'gen_roto_textures.go' || true)"
  [[ "$count" -eq 1 ]] || fail "rotozoom-textures dry-run should invoke generator once, saw $count"
}

assert_sdk_serialized() {
  rg -q '\$\(MAKE\) clean-sdk && \$\(MAKE\) sdk-build' Makefile || \
    fail "sdk target does not serialize clean-sdk before sdk-build via sub-make"
}

assert_dist_layout_skips_non_runtime_archives() {
  local tmp runtime source sdk_archive
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN

  runtime="$tmp/IntuitionEngine-1.0.0-linux-amd64"
  mkdir -p "$runtime/sdk/intuitionos/system/SYS/IOSSYS/C" \
    "$runtime/sdk/intuitionos/system/SYS/IOSSYS/LIBS" \
    "$runtime/sdk/bin" \
    "$runtime/AROS/C" \
    "$runtime/AROS/Libs" \
    "$runtime/AROS/S"
  cp README.md "$runtime/README.md"
  touch "$runtime/IntuitionEngine" \
    "$runtime/sdk/bin/ie64asm" \
    "$runtime/sdk/intuitionos/system/SYS/IOSSYS/C/Version" \
    "$runtime/sdk/intuitionos/system/SYS/IOSSYS/LIBS/dos.library" \
    "$runtime/AROS/S/Startup-Sequence"
  tar -C "$tmp" -cJf "$tmp/IntuitionEngine-1.0.0-linux-amd64.tar.xz" \
    IntuitionEngine-1.0.0-linux-amd64

  source="$tmp/IntuitionEngine-1.0.0"
  mkdir -p "$source"
  echo "source archive placeholder" >"$source/README.md"
  tar -C "$tmp" -cJf "$tmp/IntuitionEngine-1.0.0-src.tar.xz" \
    IntuitionEngine-1.0.0

  touch "$tmp/IntuitionEngine-SDK-1.0.0.zip"

  bash scripts/test-dist-layout.sh "$tmp" >/tmp/test-dist-layout.out 2>&1 || {
    cat /tmp/test-dist-layout.out >&2
    fail "dist layout check failed for mixed runtime/source/SDK archives"
  }
  rg -q 'skipping non-runtime archive: IntuitionEngine-1.0.0-src.tar.xz' /tmp/test-dist-layout.out || \
    fail "dist layout check did not skip source archive"
  rg -q 'skipping non-runtime archive: IntuitionEngine-SDK-1.0.0.zip' /tmp/test-dist-layout.out || \
    fail "dist layout check did not skip SDK archive"
}

assert_delete_on_error

assert_var IEXEC_BUILD_DATE 2026-04-25
assert_no_dup_assign IEXEC_BUILD_DATE
assert_var NCORES
assert_recipe_contains intuition-engine 'main\.Version=1\.0\.0'
assert_recipe_not_contains intuition-engine 'go mod tidy'
assert_makefile_contains 'Checking AHI artifacts'
assert_makefile_contains 'Drivers/Makefile\.in" 2>/dev/null \|\| true'

for target in \
  all setup intuition-engine ie32asm ie64asm ie64dis ie32to64 clean clean-sdk distclean \
  rotozoom-textures gem-rotozoomer emutos-rom aros-rom aros-release-assets emutos-probe \
  emutos-release-rom basic basic-emutos cputest-musashi sdk sdk-build test vet tidy \
  test-makefile testdata-harte testdata-x86 test-harte test-harte-short \
  test-x86-harte test-x86-harte-short release-verify; do
  assert_phony "$target"
  assert_target_exists "$target"
done

assert_set_e_loop release-windows
assert_recipe_contains release-src 'pipefail'
assert_release_src_pipefail_runtime
assert_recipe_contains sdk-build 'if \[ "\$SDK_FAILED" -gt 0 \]; then exit 1; fi'
assert_recipe_contains tidy 'go mod tidy -v'
assert_recipe_contains test '^go test -tags headless \./\.\.\.'
assert_recipe_contains vet '^go vet -tags headless -unsafeptr=false \./\.\.\.'
assert_recipe_contains testdata-x86 'SingleStepTests/8088|8088'
assert_recipe_contains test-harte 'go test -tags headless .* -count=1'
assert_recipe_contains test-harte-short 'go test -tags headless .* -count=1'
assert_recipe_contains test-x86-harte 'go test -tags headless .*TestHarte8086.* -count=1'
assert_recipe_contains cputest-musashi 'go test -tags "headless musashi m68k_test".* -count=1'
assert_recipe_contains clean 'IntuitionEngine\.exe'
assert_recipe_not_contains clean 'intuitionos-clean'
assert_recipe_not_contains clean 'clean-testdata'
assert_makefile_contains '^distclean:.*intuitionos-clean'
assert_makefile_contains '^distclean:.*clean-testdata'
assert_rotozoom_single_invocation
assert_sdk_serialized
assert_recipe_contains install '/tmp/x/usr/local/bin' DESTDIR=/tmp/x
assert_recipe_not_contains install 'sudo' DESTDIR=/tmp/x
assert_recipe_contains install 'sudo' PREFIX=/root/intuition-engine-test
assert_recipe_contains release-verify 'scripts/test-dist-layout\.sh'
assert_install_runtime_destdir
assert_dist_layout_skips_non_runtime_archives

echo "Makefile checks passed"
