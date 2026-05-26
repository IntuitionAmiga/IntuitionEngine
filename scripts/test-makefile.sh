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
  local dry
  dry="$(make_dry "$target" "$@")"
  printf '%s\n' "$dry" | rg -q "$regex" || fail "$target recipe does not match: $regex"
}

assert_recipe_not_contains() {
  local target="$1"
  local regex="$2"
  shift 2
  local dry
  dry="$(make_dry "$target" "$@")"
  if printf '%s\n' "$dry" | rg -q "$regex"; then
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

assert_makefile_not_contains() {
  local regex="$1"
  if rg -q "$regex" Makefile; then
    fail "Makefile unexpectedly matches: $regex"
  fi
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

assert_no_nested_external_git_checkouts() {
  if find testdata/external -mindepth 2 -maxdepth 2 -type d -name .git 2>/dev/null | rg -q .; then
    fail "testdata/external contains a nested Git checkout; vendor only fixture files or add a real submodule"
  fi
}

assert_ab3d2_prepares_embed_before_build() {
 local dry cp_zip build_vm
 dry="$(make_dry ab3d2)"
 cp_zip="$(printf '%s\n' "$dry" | rg -n 'bsdtar.*ab3d2_source/_build' | head -n 1 | cut -d: -f1 || true)"
  build_vm="$(printf '%s\n' "$dry" | rg -n 'test-cross-binaries CROSS_BUILD_DIR=\./bin/ab3d2 CROSS_BINARY_PREFIX=IntuitionEngine-AB3D2-Karlos-TKG-High VM_EMBED_TAGS="embed_ab3d2" EMBEDDED_AB3D2_START_FULLSCREEN=0' | head -n 1 | cut -d: -f1 || true)"
  [[ -n "$cp_zip" ]] || fail "ab3d2 dry-run does not package AB3D2 asset tree"
  [[ -n "$build_vm" ]] || fail "ab3d2 dry-run does not build AB3D2 binaries"
  [[ "$cp_zip" -lt "$build_vm" ]] || fail "ab3d2 builds binaries before refreshing embedded AB3D2 zip"
}

assert_ab3d2_overdrive_starts_fullscreen() {
  local dry
  dry="$(make -n -B ab3d2 AB3D2_SOURCE=../alienbreed3d2/ab3d2_source/ab3d2_ie68_overdrive.ie68 2>/dev/null)"
  printf '%s\n' "$dry" | rg -q 'test-cross-binaries .*EMBEDDED_AB3D2_START_FULLSCREEN=1' || \
    fail "AB3D2 Overdrive package build does not stamp fullscreen startup"
}

assert_ab3d2_overdrive_target_packages_overdrive() {
  local dry cp_rom build_vm
  dry="$(make_dry ab3d2-overdrive)"
  cp_rom="$(printf '%s\n' "$dry" | rg -n 'cp "\.\./alienbreed3d2/ab3d2_source/ie/bin/ab3d2_ie68_redux_high_overdrive\.ie68" "embedded/ab3d2/ab3d2_ie68_redux_high\.ie68"' | head -n 1 | cut -d: -f1 || true)"
  build_vm="$(printf '%s\n' "$dry" | rg -n 'test-cross-binaries CROSS_BUILD_DIR=\./bin/ab3d2 CROSS_BINARY_PREFIX=IntuitionEngine-AB3D2-Karlos-TKG-High-Overdrive VM_EMBED_TAGS="embed_ab3d2" EMBEDDED_AB3D2_START_FULLSCREEN=1' | head -n 1 | cut -d: -f1 || true)"
  [[ -n "$cp_rom" ]] || fail "ab3d2-overdrive dry-run does not embed the Overdrive IE68 ROM"
  [[ -n "$build_vm" ]] || fail "ab3d2-overdrive dry-run does not build Overdrive binaries with the expected prefix"
  [[ "$cp_rom" -lt "$build_vm" ]] || fail "ab3d2-overdrive builds binaries before refreshing the embedded ROM"
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
  test-makefile test-cross test-cross-binaries ab3d2 ab3d2-overdrive ab3d2-all prepare-ab3d2-embed compress-ab3d2 check-linux-arm64-cross-prereqs testdata-harte testdata-x86 test-harte test-harte-short \
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
assert_target_exists x64-live-refman-pdfs
assert_phony x64-live-refman-pdfs
assert_target_exists x64-live-sdk-companion-pdfs
assert_phony x64-live-sdk-companion-pdfs
assert_target_exists x64-live-sdk-tools
assert_phony x64-live-sdk-tools
assert_makefile_contains '^x64-live-payload-check:.*x64-live-sdk-tools'
assert_makefile_contains '^x64-live-payload-check:.*x64-live-refman-pdfs'
assert_makefile_contains '^x64-live-payload-check:.*x64-live-sdk-companion-pdfs'
assert_recipe_contains x64-live-sdk-tools 'GOOS=\$goos GOARCH=\$goarch.*ie32asm'
assert_recipe_contains x64-live-sdk-tools 'GOOS=\$goos GOARCH=\$goarch.*ie64asm'
assert_recipe_contains x64-live-sdk-tools 'GOOS=\$goos GOARCH=\$goarch.*ie64dis'
assert_recipe_contains x64-live-sdk-tools 'GOOS=\$goos GOARCH=\$goarch.*ie32to64'
assert_recipe_contains x64-live-sdk-tools 'GOOS=\$goos GOARCH=\$goarch.*m68kto64'
assert_recipe_contains x64-live-sdk-tools 'SHA256SUMS\.txt'
assert_recipe_contains x64-live-refman-pdfs 'scripts/refman-publish\.sh --strict'
assert_recipe_contains x64-live-refman-pdfs 'scripts/refman-pdf\.sh'
assert_recipe_contains x64-live-sdk-companion-pdfs 'IE64_ISA\.pdf'
assert_recipe_contains x64-live-sdk-companion-pdfs 'architecture\.pdf'
rg -q 'local archive_docs_dir="\$\{archive_root\}/Docs"' build_x64_ie_img.sh || fail "x64 live archive does not create Docs directory"
rg -q 'local archive_refman_docs_dir="\$\{archive_docs_dir\}/IEProgRefMan"' build_x64_ie_img.sh || fail "x64 live archive does not create Docs/IEProgRefMan directory"
rg -q 'cp -f "\$\{SDK_COMPANION_PDFS\[@\]\}" "\$archive_docs_dir/"' build_x64_ie_img.sh || fail "x64 live archive does not copy SDK companion PDFs into Docs"
rg -q 'cp -f "\$\{REFMAN_PDF_DIR\}"/\*\.pdf "\$archive_refman_docs_dir/"' build_x64_ie_img.sh || fail "x64 live archive does not copy refman PDFs into Docs/IEProgRefMan"
rg -q '"\$\(basename "\$OUTPUT_IMG"\)" Docs <<' build_x64_ie_img.sh || fail "x64 live archive zip entries do not include Docs"
! rg -q 'cp "\$\{SCRIPT_DIR\}/README\.md" "\$archive_root/README\.md"|README\.md Docs <<' build_x64_ie_img.sh || fail "x64 live archive must not include README.md"
assert_makefile_contains 'define build-linux-vm-binary'
assert_makefile_contains 'define build-purego-novulkan-vm-binary'
assert_makefile_contains '/opt/ie-sysroots/tumbleweed-aarch64/usr'
assert_makefile_contains 'test-cross-binaries:'
assert_makefile_contains 'CROSS_BINARY_PREFIX \?= IntuitionEngine'
assert_makefile_contains 'AB3D2_BINARY_PREFIX \?= IntuitionEngine-AB3D2-Karlos-TKG-High'
assert_makefile_contains 'AB3D2_OVERDRIVE_BINARY_PREFIX \?= IntuitionEngine-AB3D2-Karlos-TKG-High-Overdrive'
assert_makefile_contains '\$\(call build-linux-vm-binary,amd64'
assert_makefile_contains '\$\(call build-linux-vm-binary,arm64'
assert_makefile_contains '\$\(call build-purego-novulkan-vm-binary,\$\$goos,\$\$goarch'
assert_makefile_contains '\$\(call build-purego-novulkan-vm-binary,windows,\$\$goarch'
assert_makefile_contains '\$\(call build-purego-novulkan-vm-binary,darwin,amd64'
assert_makefile_contains '\$\(call build-purego-novulkan-vm-binary,darwin,arm64'
assert_makefile_contains 'AB3D2_SOURCE \?= \.\./alienbreed3d2/ab3d2_source/ie/bin/ab3d2_ie68_redux_high\.ie68'
assert_makefile_contains 'AB3D2_OVERDRIVE_SOURCE \?= \.\./alienbreed3d2/ab3d2_source/ie/bin/ab3d2_ie68_redux_high_overdrive\.ie68'
assert_makefile_contains 'AB3D2_ASSET_ROOT \?= \.\./alienbreed3d2'
assert_makefile_contains 'AB3D2_ASSET_TREE \?= ab3d2_source/_build'
assert_makefile_contains 'AB3D2_START_FULLSCREEN \?= \$\(if \$\(findstring overdrive,\$\(notdir \$\(AB3D2_SOURCE\)\)\),1,0\)'
assert_makefile_contains 'cp "\$\(AB3D2_SOURCE\)" "\$\(AB3D2_EMBED_FILE\)"'
assert_makefile_contains '\$\(BSDTAR\) -c -L --format zip'
assert_makefile_contains 'test-cross-binaries CROSS_BUILD_DIR=\$\(AB3D2_BUILD_DIR\) CROSS_BINARY_PREFIX=\$\(AB3D2_BINARY_PREFIX\) VM_EMBED_TAGS="embed_ab3d2" EMBEDDED_AB3D2_START_FULLSCREEN=\$\(AB3D2_START_FULLSCREEN\)'
assert_makefile_not_contains '\$\(MAKE\) compress-ab3d2'
assert_makefile_contains 'ab3d2 AB3D2_SOURCE=\$\(AB3D2_OVERDRIVE_SOURCE\) AB3D2_BINARY_PREFIX=\$\(AB3D2_OVERDRIVE_BINARY_PREFIX\) AB3D2_START_FULLSCREEN=1'
assert_makefile_contains '\$\(MAKE\) ab3d2-overdrive'
assert_makefile_not_contains 'UPX'
assert_makefile_not_contains 'AB3D2_UPX_FLAGS'
assert_recipe_contains compress-ab3d2 'Skipping AB3D2 binary compression'
assert_ab3d2_overdrive_starts_fullscreen
assert_ab3d2_overdrive_target_packages_overdrive
assert_no_nested_external_git_checkouts
assert_ab3d2_prepares_embed_before_build
assert_install_runtime_destdir
assert_dist_layout_skips_non_runtime_archives

echo "Makefile checks passed"
