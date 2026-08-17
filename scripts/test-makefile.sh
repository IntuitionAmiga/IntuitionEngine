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
  make -n MAKE='echo make' EMUTOS_SRC_DIR=/tmp/intuitionengine-makefile-test-no-emutos-source "$@" 2>&1
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
  dry="$(make_dry "$@" "$target")"
  printf '%s\n' "$dry" | rg -q -- "$regex" || fail "$target recipe does not match: $regex"
}

assert_recipe_not_contains() {
  local target="$1"
  local regex="$2"
  shift 2
  local dry
  dry="$(make_dry "$@" "$target")"
  if printf '%s\n' "$dry" | rg -q -- "$regex"; then
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

assert_go127_workflows() {
  local workflows=(.github/workflows/test.yml .github/workflows/release.yml)
  if rg -q 'go-version-file:' "${workflows[@]}"; then
    fail "CI or release workflow still uses go-version-file"
  fi

  local setups versions
  setups="$(rg -c 'uses: actions/setup-go@' "${workflows[@]}" | awk -F: '{sum += $2} END {print sum + 0}')"
  versions="$(rg -c 'go-version: 1\.27\.0-rc\.2' "${workflows[@]}" | awk -F: '{sum += $2} END {print sum + 0}')"
  [[ "$setups" -eq "$versions" ]] || fail "every setup-go step must select Go 1.27rc2"

  for workflow in "${workflows[@]}"; do
    rg -q 'GOTOOLCHAIN: local' "$workflow" || fail "$workflow does not force the local toolchain"
  done

  rg -q 'GOEXPERIMENT: simd' .github/workflows/release.yml || \
    fail "the Linux x64 release build does not enable SIMD"
  local release_matrix
  release_matrix="$(sed -n '/windows:/,/macos:/p' .github/workflows/release.yml)$(sed -n '/macos:/,/release:/p' .github/workflows/release.yml)"
  [[ "$(printf '%s\n' "$release_matrix" | rg -c 'goexperiment: simd')" -eq 2 ]] || \
    fail "Windows and macOS x64 release entries must enable SIMD"
  [[ "$(printf '%s\n' "$release_matrix" | rg -c 'goexperiment: none')" -eq 2 ]] || \
    fail "Windows and macOS ARM64 release entries must remain scalar"
  printf '%s\n' "$release_matrix" | rg -q 'GOEXPERIMENT:.*matrix\.goexperiment' || \
    fail "release matrix does not pass its SIMD selection to Go"

	local cross_compile_job
	cross_compile_job="$(sed -n '/^  cross-compile:/,/^  sdk-smoke:/p' .github/workflows/test.yml)"
	printf '%s\n' "$cross_compile_job" | rg -q 'gcc-aarch64-linux-gnu' || \
		fail "cross-compile CI does not install the ARM64 C compiler"
	printf '%s\n' "$cross_compile_job" | rg -q 'g\+\+-aarch64-linux-gnu' || \
		fail "cross-compile CI does not install the ARM64 C++ compiler"
}

assert_go_test_inventory_guard_runtime() {
  local guard="./scripts/require-go-test-inventory.sh"
  [[ -x "$guard" ]] || fail "missing executable Go test inventory guard"

  if "$guard" "empty test inventory" sh -c 'printf "ok  example/package  0.001s\n"' >/dev/null 2>&1; then
    fail "inventory guard accepted package status without a Test name"
  fi
  "$guard" "empty test inventory" sh -c 'printf "TestRequiredBackend\nok  example/package  0.001s\n"' || \
    fail "inventory guard rejected an actual Test name"
  local status
  set +e
  "$guard" "empty test inventory" sh -c 'exit 7' >/dev/null 2>&1
  status=$?
  set -e
  [[ "$status" -eq 7 ]] || fail "inventory guard changed command failure status 7 to $status"

  local uses
  uses="$(rg -c 'scripts/require-go-test-inventory\.sh "empty (pre-JIT Z80 contract|Z80 native JIT|pre-JIT Z80 ARM64 contract|Z80 ARM64 JIT) inventory"' Makefile)"
  [[ "$uses" -eq 4 ]] || fail "Z80 parity target must guard legacy and backend inventories on native and ARM64"
  rg -q 'require-go-test-inventory\.sh "empty pre-JIT Z80 ARM64 contract inventory".*\|\| exit \$\$\?' Makefile || \
    fail "ARM64 legacy inventory failure does not terminate its multi-command recipe"
  rg -q 'require-go-test-inventory\.sh "empty Z80 ARM64 JIT inventory".*\|\| exit \$\$\?' Makefile || \
    fail "ARM64 backend inventory failure does not terminate its multi-command recipe"
  rg -q 'Z80_INTERPRETER_TEST_REGEX := \$\(shell \./scripts/z80-interpreter-test-regex\.sh\)' Makefile || \
    fail "Z80 parity gate does not derive the pre-existing interpreter inventory"
  rg -q 'z80-interpreter-test-regex\.sh 10 \| while IFS= read -r regex' Makefile || \
    fail "Z80 parity gate does not batch the exact legacy inventory for the wasm argument limit"
  rg -q 'TestAMD64Z80JIT_' Makefile || \
    fail "Z80 parity gate omits the amd64 emitted-path manifest differential"
  rg -q 'TestARM64Z80JIT_' Makefile || \
    fail "Z80 parity gate omits the ARM64 emitted-path manifest differential"
  rg -q "test-wasm-node WASM_NODE_TEST_REGEX='\^\(TestWasmJIT_Z80\|TestZ80Wasm\|TestZ80JIT_Full\|TestAYZ80PlaybackRealProgramsJITParity\)'" Makefile || \
    fail "Z80 parity gate does not select its complete bounded Node inventory"
  local parity_recipe
  parity_recipe="$(make_dry test-z80-jit-parity)"
  printf '%s\n' "$parity_recipe" | rg -q 'TestZ80JIT_Full.*TestARM64Z80JIT_' || \
    fail "Z80 ARM64 gate omits full demo shadow fixtures"
  printf '%s\n' "$parity_recipe" | rg -q 'IE_REQUIRE_Z80_WASM_BROWSER=1.*TestZ80WasmBrowser_InstantiatesAndAccessesMemory' || \
    fail "Z80 parity gate does not require real browser execution"
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
  local dry copy_original copy_high build_original build_high
  dry="$(sed -n '/^ab3d2:/,/^$/p' Makefile)"
  copy_original="$(printf '%s\n' "$dry" | rg -n 'prepare-ab3d2-embed AB3D2_SOURCE=\$\(AB3D2_ORIGINAL_SOURCE\)' | head -n 1 | cut -d: -f1 || true)"
  copy_high="$(printf '%s\n' "$dry" | rg -n 'prepare-ab3d2-embed AB3D2_SOURCE=\$\(AB3D2_SOURCE\)' | head -n 1 | cut -d: -f1 || true)"
  build_original="$(printf '%s\n' "$dry" | rg -n 'test-cross-amd64-binaries CROSS_BUILD_DIR=\$\(AB3D2_BUILD_DIR\) CROSS_BINARY_PREFIX=\$\(AB3D2_ORIGINAL_BINARY_PREFIX\)' | head -n 1 | cut -d: -f1 || true)"
  build_high="$(printf '%s\n' "$dry" | rg -n 'test-cross-amd64-binaries CROSS_BUILD_DIR=\$\(AB3D2_BUILD_DIR\) CROSS_BINARY_PREFIX=\$\(AB3D2_BINARY_PREFIX\)' | head -n 1 | cut -d: -f1 || true)"
  [[ -n "$copy_original" && -n "$copy_high" ]] || fail "ab3d2 dry-run does not refresh both packed AB3D2 images"
  [[ -n "$build_original" && -n "$build_high" ]] || fail "ab3d2 dry-run does not build both AB3D2 variants"
  [[ "$copy_original" -lt "$build_original" && "$build_original" -lt "$copy_high" && "$copy_high" -lt "$build_high" ]] || fail "ab3d2 variants are not staged before their builds"
}

assert_ab3d2_starts_fullscreen() {
  local dry
  dry="$(sed -n '/^ab3d2:/,/^$/p' Makefile)"
  printf '%s\n' "$dry" | rg -q 'test-cross-amd64-binaries .*EMBEDDED_AB3D2_START_FULLSCREEN=\$\(AB3D2_START_FULLSCREEN\)' || \
    fail "AB3D2 package build does not stamp fullscreen startup"
}

assert_ab3d2_target_packages_redux_high() {
  local dry cp_rom build_vm
  dry="$(sed -n '/^ab3d2:/,/^$/p' Makefile)"
  cp_rom="$(printf '%s\n' "$dry" | rg -n 'prepare-ab3d2-embed AB3D2_SOURCE=\$\(AB3D2_SOURCE\)' | head -n 1 | cut -d: -f1 || true)"
  build_vm="$(printf '%s\n' "$dry" | rg -n 'test-cross-amd64-binaries CROSS_BUILD_DIR=\$\(AB3D2_BUILD_DIR\) CROSS_BINARY_PREFIX=\$\(AB3D2_BINARY_PREFIX\) VM_EMBED_TAGS="embed_ab3d2" EMBEDDED_AB3D2_START_FULLSCREEN=\$\(AB3D2_START_FULLSCREEN\)' | head -n 1 | cut -d: -f1 || true)"
  [[ -n "$cp_rom" ]] || fail "ab3d2 dry-run does not embed the Redux High IE68 image"
  [[ -n "$build_vm" ]] || fail "ab3d2 dry-run does not build Redux High binaries with the expected prefix"
  [[ "$cp_rom" -lt "$build_vm" ]] || fail "ab3d2 builds binaries before refreshing the embedded ROM"
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

if [[ "${1:-}" == "--go-test-inventory-guard" ]]; then
  assert_go_test_inventory_guard_runtime
  echo "Go test inventory guard checks passed"
  exit 0
fi

assert_delete_on_error
assert_go127_workflows
rg -q 'runtime_asmcgocall runtime\.asmcgocall' jit_call_darwin_nocgo.go || \
  fail "native JIT bridge does not support cgo-disabled release builds"
[[ "$(rg -c 'GOOS=darwin GOARCH=(amd64|arm64) go (build|test).* -ldflags=-checklinkname=0' scripts/test-cross-compile.sh)" -eq 4 ]] || \
  fail "every Darwin cross-build must permit its cgo-independent JIT bridge"
macos_release_job="$(sed -n '/^  macos:/,/^  release:/p' .github/workflows/release.yml)"
printf '%s\n' "$macos_release_job" | rg -q -- '-checklinkname=0.*main\.Version' || \
  fail "macOS release build does not permit its cgo-independent JIT bridge"

assert_var IEXEC_BUILD_DATE 2026-04-25
assert_no_dup_assign IEXEC_BUILD_DATE
assert_var NCORES
assert_recipe_contains intuition-engine "main\\.Version=$(cat VERSION)"
assert_recipe_not_contains intuition-engine 'go mod tidy'
assert_makefile_contains 'Checking AHI artifacts'
assert_makefile_contains 'Drivers/Makefile\.in" 2>/dev/null \|\| true'

for target in \
  all setup intuition-engine ie32asm ie64asm ie64dis ie32to64 clean clean-sdk distclean \
  rotozoom-textures nocpu-rotozoomer gem-rotozoomer emutos-rom aros-rom aros-ie-live-assets aros-ie-live-inputs aros-ie-toolchain-assets aros-release-assets emutos-probe \
  iewarp-service-worker iewarp-runtime-local-assets iewarp-runtime-assets \
  arosvision-probe-tree arosvision-live-base arosvision-live-components arosvision-live-overlays arosvision-live-tree arosvision-probe-run emutos-release-rom basic basic-emutos cputest-musashi sdk sdk-build test vet tidy \
  test-makefile test-cross test-cross-binaries test-x86-jit-parity test-ie32-jit-parity test-ie32-jit-race ab3d2 prepare-ab3d2-embed compress-ab3d2 check-linux-arm64-cross-prereqs testdata-harte testdata-x86 test-harte test-harte-short \
  test-x86-harte test-x86-harte-short release-verify dist-host-sdk-linux-amd64 dist-host-sdk-linux-arm64 test-host-sdk test-host-sdk-arm64 test-host-sdk-external iedoom iedoom-ie86 iedoom-ie68 x86-bench-baseline x86-bench-after x86-bench-compare ie32-bench-baseline ie32-bench-after ie32-bench-compare x86-iedoom-timedemo; do
  assert_phony "$target"
  assert_target_exists "$target"
done

assert_recipe_contains nocpu-rotozoomer 'rotozoomer_nocpu\.asm'
assert_makefile_not_contains 'gen_nocpu_roto|rotozoomer_nocpu_(copper|state|targets)\.bin|rotozoomer_nocpu\.inc'
assert_makefile_contains 'showreel-ie64:.*nocpu-rotozoomer'

# Pi 4 and Pi 400 share one Cortex-A72 binary and release image. Pi 5 derives
# its image from that completed appliance, replacing only the VM binary.
for target in rpi-4-arm64 rpi-400-arm64 rpi-5-arm64 rpi-arm64-preflight rpi-host-helper-arm64 build-image-pi4 build-image-pi400 build-image-pi5 rpi-live-payload-check rpi4-live-payload-check rpi400-live-payload-check rpi5-live-payload-check rpi-live-images rpi4-live-qemu rpi4-live-hardware-qemu prepare-rpi-cross-overlay validate-rpi-sysroot validate-rpi-sysroot-preflight test-rpi-live-image test-rpi-sysroot test-rpi-binary test-rpi-golden-prepare test-rpi-append-ieshare test-rpi4-live-qemu test-ieshare-payload test-verify-rpi-live-image; do
  assert_phony "$target"
  assert_target_exists "$target"
done
for target in check-intuitionengine-app-version deb-intuitionengine-amd64-v3 deb-intuitionengine-arm64-pi4 deb-intuitionengine-arm64-pi5 deb-intuitionengine release-intuitionengine-repository test-intuitionengine-packages test-intuitionengine-repository test-intuitionengine-release-secrets; do
  assert_phony "$target"
  assert_target_exists "$target"
done
assert_recipe_contains deb-intuitionengine-amd64-v3 'build-intuitionengine-deb\.sh.*intuitionengine-amd64-v3'
assert_recipe_contains deb-intuitionengine-arm64-pi4 'build-intuitionengine-deb\.sh.*intuitionengine-arm64-pi4'
assert_recipe_contains deb-intuitionengine-arm64-pi5 'build-intuitionengine-deb\.sh.*intuitionengine-arm64-pi5'
assert_recipe_contains release-intuitionengine-repository 'stage-intuitionengine-repository\.sh'
assert_makefile_contains '^x64-live: deb-intuitionengine-amd64-v3'
assert_makefile_contains '^x64-live-rebuild-golden: deb-intuitionengine-amd64-v3'
assert_recipe_contains build-image-pi4 'intuitionengine-arm64-pi4_.*\.deb'
assert_recipe_contains build-image-pi5 'intuitionengine-arm64-pi5_.*\.deb'
assert_recipe_contains rpi4-live-qemu 'rpi4_virt_qemu\.sh'
assert_recipe_contains rpi4-live-hardware-qemu 'rpi4_live_qemu\.sh'
assert_recipe_contains rpi-4-arm64 'GOARM64=v8\.0' -o x64-live-embed-assets
assert_recipe_contains rpi-5-arm64 'GOARM64=v8\.2' -o x64-live-embed-assets
assert_recipe_contains rpi-4-arm64 'mcpu=cortex-a72' -o x64-live-embed-assets
assert_recipe_contains rpi-5-arm64 'mcpu=cortex-a76' -o x64-live-embed-assets
assert_makefile_contains '^rpi-400-arm64: rpi-4-arm64'
assert_recipe_not_contains rpi-400-arm64 'go build' -o rpi-4-arm64
assert_makefile_contains '^rpi-live-images: build-image-pi4 build-image-pi5'
assert_makefile_contains '^build-image-pi400: build-image-pi4'
assert_recipe_not_contains build-image-pi400 'build_rpi_live_image\.sh' -o build-image-pi4
assert_makefile_contains '^rpi-live-payload-check:.*rpi-4-arm64.*rpi-5-arm64'
assert_recipe_not_contains rpi-live-payload-check 'stage_ieshare_payload\.sh' -o rpi-4-arm64 -o rpi-5-arm64 -o rpi-host-helper-arm64 -o x64-live-payload-check
assert_recipe_contains rpi-live-payload-check 'payload build/x64-live/work/ieshare-payload' -o rpi-4-arm64 -o rpi-5-arm64 -o rpi-host-helper-arm64 -o x64-live-payload-check
assert_recipe_contains build-image-pi4 'build_rpi_live_image\.sh --board pi4 --binary build/rpi4-live/IntuitionEngine-rpi4' -o rpi-live-payload-check
assert_recipe_contains build-image-pi4 'payload build/x64-live/work/ieshare-payload' -o rpi-live-payload-check
assert_recipe_contains build-image-pi5 'source-image build/rpi4-live/intuition-engine-rpi4\.img' -o build-image-pi4 -o rpi-live-payload-check
assert_recipe_contains build-image-pi5 'binary build/rpi5-live/IntuitionEngine-rpi5' -o build-image-pi4 -o rpi-live-payload-check
assert_recipe_contains build-image-pi5 'payload build/x64-live/work/ieshare-payload' -o build-image-pi4 -o rpi-live-payload-check
assert_makefile_not_contains '^rpi4-live-payload-check:.*wasm'
assert_makefile_contains '^rpi-4-arm64:.*validate-rpi-sysroot'
assert_makefile_contains '^rpi-5-arm64:.*validate-rpi-sysroot'
assert_recipe_contains rpi-host-helper-arm64 'CGO_ENABLED=0 GOOS=linux GOARCH=arm64'
assert_makefile_contains 'RPI_CROSS_SYSROOT.*IntuitionSubtractor/sysroot-arm64'
assert_makefile_contains 'RPI_TOOLCHAIN_SYSROOT.*-print-sysroot'
assert_makefile_contains 'RPI_PKG_CONFIG_LIBDIR.*RPI_CROSS_OVERLAY'
assert_recipe_contains prepare-rpi-cross-overlay 'prepare_rpi_cross_overlay\.sh'
assert_makefile_contains '^rpi-arm64-preflight: validate-rpi-sysroot-preflight'
assert_makefile_contains 'RPI_GO \?= env GOTOOLCHAIN=go1\.27rc2'
assert_recipe_contains headless-novulkan 'CGO_ENABLED=1'
assert_makefile_contains 'ARM64_QEMU_CGO_ENV := .*CGO_ENABLED=1.*CC=\$\(CROSS_CC\)'
assert_makefile_contains 'ARM64_QEMU_EXEC = .* -L \$\(RPI_TOOLCHAIN_SYSROOT\)'
rg -q 'CGO_ENABLED=1 CC="\$ARM64_CC" GOOS=linux GOARCH=arm64 go test' scripts/test-cross-compile.sh || \
  fail "Linux ARM64 cross tests do not enable cgo with the cross compiler"
assert_recipe_contains rpi-4-arm64 'CROSS_SYSROOT=' -o x64-live-embed-assets
assert_makefile_contains '^define build-rpi-binary'
assert_makefile_contains 'build-linux-vm-binary,arm64'
assert_makefile_contains 'validate_rpi_binary\.sh "\$\(4\)"'

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
assert_makefile_contains '^x64-live-payload-check:.*x64-live-refman-pdfs'
assert_makefile_contains '^x64-live-payload-check:.*x64-live-sdk-companion-pdfs'
assert_makefile_contains '^x64-live-payload-check:.*dist-host-sdk-linux-amd64.*dist-host-sdk-linux-arm64'
assert_makefile_not_contains '^x64-live-payload-check:.*x64-live-sdk-tools'
assert_makefile_not_contains '^x64-live-payload-check:.*dist-ie64-toolchain-linux-amd64'
assert_makefile_contains '^x64-live-payload-check:.*iedoom'
assert_makefile_contains '^x64-live-payload-check:.*arosvision-live-tree'
assert_makefile_contains '^x64-live-sdk-companion-pdfs:.*x64-live-refman-pdfs'
assert_recipe_contains x64-live-refman-pdfs 'scripts/refman-publish\.sh --strict'
assert_recipe_contains x64-live-refman-pdfs 'scripts/refman-pdf\.sh'
assert_recipe_contains x64-live-sdk-companion-pdfs 'scripts/sdk-companion-pdf\.sh'
assert_var CHOCOLATE_DOOM_DIR ../chocolate-doom
assert_var IEDOOM_IE86 build/iedoom.ie86
assert_var IEDOOM_IE68 build/iedoom.ie68
assert_var IEDOOM_WAD DOOM1.WAD
assert_var IEDOOM_TIMED_IE86 build/iedoom_timedemo.ie86
assert_var IEDOOM_TIMED_SCRIPT bench/measure_timedemo.ies
assert_makefile_contains '^iedoom: iedoom-ie86 iedoom-ie68'
assert_recipe_contains x86-bench-baseline "BENCH_REGEX='BenchmarkX86JIT_'"
assert_recipe_contains x86-bench-baseline "BENCH_TAGS='headless'"
assert_recipe_contains x86-bench-baseline "BENCH_PKG='\\.'"
assert_recipe_contains x86-bench-after "BENCH_REGEX='BenchmarkX86JIT_'"
assert_recipe_contains x86-bench-after "BENCH_TAGS='headless'"
assert_recipe_contains x86-bench-after "BENCH_PKG='\\.'"
assert_recipe_contains x86-bench-compare "BENCH_REGEX='BenchmarkX86JIT_'"
assert_recipe_contains x86-bench-compare "BENCH_TAGS='headless'"
assert_recipe_contains x86-bench-compare "BENCH_PKG='\\.'"
assert_recipe_contains ie32-bench-baseline "BENCH_REGEX='BenchmarkIE32_\\(ALU\\|Memory\\|Mixed\\|Call\\|VoodooMegaDemo\\)_\\(Interpreter\\|JIT\\)'"
assert_recipe_contains ie32-bench-baseline "BENCH_TAGS='headless'"
assert_recipe_contains ie32-bench-baseline "BENCH_PKG='\\.'"
assert_recipe_contains ie32-bench-after "BENCH_REGEX='BenchmarkIE32_\\(ALU\\|Memory\\|Mixed\\|Call\\|VoodooMegaDemo\\)_\\(Interpreter\\|JIT\\)'"
assert_recipe_contains ie32-bench-after "BENCH_TAGS='headless'"
assert_recipe_contains ie32-bench-after "BENCH_PKG='\\.'"
assert_recipe_contains ie32-bench-compare "BENCH_REGEX='BenchmarkIE32_\\(ALU\\|Memory\\|Mixed\\|Call\\|VoodooMegaDemo\\)_\\(Interpreter\\|JIT\\)'"
assert_recipe_contains ie32-bench-compare "BENCH_TAGS='headless'"
assert_recipe_contains ie32-bench-compare "BENCH_PKG='\\.'"
assert_recipe_contains test-ie32-jit-parity 'require-go-test-inventory\.sh'
assert_recipe_contains test-ie32-jit-parity 'test-ie32-jit-race'
assert_recipe_contains test-ie32-jit-parity 'make test-wasm-build'
assert_recipe_contains test-ie32-jit-parity 'make test-wasm-node'
assert_recipe_contains test-ie32-jit-parity 'GOOS=linux GOARCH=arm64'
assert_var IE32_JIT_TEST_REGEX
assert_recipe_contains test-ie32-jit-parity 'TestIE32'
assert_makefile_contains 'TestIE32\(JIT\|StepOne\|Retired\|Wasm\)\.\*'
assert_recipe_contains test-ie32-jit-race 'go test -race'
assert_recipe_contains x86-iedoom-timedemo 'IE_NO_IPC=1'
assert_recipe_contains x86-iedoom-timedemo '-script-owned-term'
assert_recipe_contains x86-iedoom-timedemo '-script "bench/measure_timedemo\.ies"'
assert_recipe_contains test-x86-jit-parity 'go test -tags headless -run'
assert_recipe_contains test-x86-jit-parity 'make test-wasm-build'
assert_recipe_contains test-x86-jit-parity 'make test-wasm-node'
assert_recipe_contains test-x86-jit-parity 'GOOS=linux GOARCH=arm64'
assert_var AROS_LIVE_DIR '$(AROSVISION_PROBE_DIR)'
assert_recipe_contains x64-live 'AROS_RELEASE_DIR="build/arosvision".*CHOCOLATE_DOOM_DIR="\.\./chocolate-doom" IEDOOM_IE86="build/iedoom\.ie86" IEDOOM_IE68="build/iedoom\.ie68" IEDOOM_WAD="DOOM1\.WAD" \./build_x64_ie_img\.sh'
assert_recipe_contains x64-live-rebuild-golden 'AROS_RELEASE_DIR="build/arosvision".*CHOCOLATE_DOOM_DIR="\.\./chocolate-doom" IEDOOM_IE86="build/iedoom\.ie86" IEDOOM_IE68="build/iedoom\.ie68" IEDOOM_WAD="DOOM1\.WAD" \./build_x64_ie_img\.sh --rebuild-golden'
assert_recipe_contains x64-live-payload-check 'AROS_RELEASE_DIR="build/arosvision".*CHOCOLATE_DOOM_DIR="\.\./chocolate-doom" IEDOOM_IE86="build/iedoom\.ie86" IEDOOM_IE68="build/iedoom\.ie68" IEDOOM_WAD="DOOM1\.WAD" \./build_x64_ie_img\.sh --check-payload'
assert_recipe_contains dist-host-sdk-linux-amd64 'dist-host-sdk-linux-amd64\.sh'
assert_recipe_contains dist-host-sdk-linux-arm64 'dist-host-sdk-linux-arm64\.sh'
assert_recipe_contains dist-host-sdk-linux-arm64 'HOST_SDK_CC=.*HOST_SDK_SYSROOT=.*dist-host-sdk-linux-arm64\.sh'
assert_recipe_contains web-demos "! -name 'intuition-engine-host-sdk-linux-amd64\\.tar\\.xz\\*'"
assert_recipe_contains web-demos "! -name 'intuition-engine-host-sdk-linux-arm64\\.tar\\.xz\\*'"
assert_recipe_contains iedoom-ie86 'cd "\.\./chocolate-doom" && sh src/iedoom_build\.sh "build/iedoom\.ie86"'
assert_recipe_contains iedoom-ie68 'cd "\.\./chocolate-doom" && sh src/iedoom_build_m68k\.sh "build/iedoom\.ie68"'
assert_var AROSVISION_SOURCE ../AROSVision
assert_var AROSVISION_PROBE_DIR build/arosvision
assert_makefile_contains '^iewarp-service-worker:'
assert_recipe_contains iewarp-service-worker 'iewarp_service\.asm'
assert_recipe_contains iewarp-service-worker 'sdk/examples/prebuilt/iewarp_service\.ie64'
assert_makefile_contains '^iewarp-runtime-local-assets: iewarp-service-worker'
assert_makefile_not_contains '^iewarp-runtime-local-assets:.*sdk-build'
assert_makefile_contains '^iewarp-runtime-assets: iewarp-runtime-local-assets'
assert_recipe_contains iewarp-runtime-local-assets 'cp -f sdk/examples/prebuilt/iewarp_service\.ie64 Systems/AROS/Libs/iewarp_service\.ie64'
assert_recipe_contains iewarp-runtime-assets 'if \[ -d "\.\./AROS-deadw00d/bin/ie-m68k/bin/ie-m68k/AROS" \]; then'
assert_makefile_contains '^x64-live-embed-assets:.*aros-ie-live-inputs'
assert_makefile_not_contains '^x64-live-embed-assets:.*aros-ie-live-assets'
assert_makefile_not_contains '^x64-live-embed-assets:.*aros-release-assets'
assert_makefile_contains '^x64-live-aros-demos: aros-ie-toolchain-assets rotozoom-textures'
assert_makefile_not_contains '^x64-live-aros-demos:.*aros-release-assets'
assert_target_exists aros-ie-live-assets
assert_target_exists aros-ie-live-inputs
assert_target_exists aros-ie-toolchain-assets
assert_recipe_contains aros-ie-live-assets 'kernel-iewarp'
assert_recipe_contains aros-ie-live-assets 'kernel-ie-m68k-rom'
assert_recipe_contains aros-ie-live-assets 'kernel-ie-m68k-ahidrv'
assert_recipe_not_contains aros-ie-live-assets 'arch-ie-m68k-utilities-iewarpmon'
assert_recipe_not_contains aros-ie-live-assets 'IEWarpMon'
assert_recipe_contains aros-ie-live-assets 'IEGfx HIDD staged only in local AROS build tree'
assert_recipe_not_contains aros-ie-live-assets 'workbench-fonts'
assert_recipe_not_contains aros-ie-live-assets 'workbench-system-wanderer'
assert_recipe_not_contains aros-ie-live-assets 'workbench-tools'
assert_recipe_not_contains aros-ie-live-assets 'workbench-utilities'
assert_recipe_contains aros-ie-toolchain-assets 'missing IE AROS compiler'
assert_recipe_contains aros-ie-toolchain-assets 'missing IE AROS toolchain directory'
assert_recipe_not_contains aros-ie-toolchain-assets 'git clone'
assert_recipe_not_contains aros-ie-toolchain-assets 'git -C .*fetch'
assert_recipe_not_contains aros-ie-toolchain-assets 'git -C .*checkout'
assert_recipe_not_contains aros-ie-toolchain-assets 'submodule update'
assert_recipe_not_contains aros-ie-toolchain-assets 'configure'
assert_recipe_not_contains aros-ie-toolchain-assets '\$\(MAKE\).*kernel-'
assert_recipe_contains aros-ie-live-inputs 'missing embedded AROS ROM'
assert_recipe_contains aros-ie-live-inputs 'missing IE AHI driver'
assert_recipe_not_contains aros-ie-live-inputs '\$\(MAKE\).*kernel-'
assert_makefile_contains '^aros-ie-live-inputs: iewarp-runtime-local-assets'
assert_makefile_not_contains '^aros-ie-live-inputs:.*iewarp-runtime-assets'
assert_recipe_contains arosvision-probe-tree 'scripts/prepare-arosvision-probe\.sh "\.\./AROSVision" "build/arosvision"'
assert_makefile_contains '^arosvision-live-base:'
assert_recipe_contains arosvision-live-base 'scripts/prepare-arosvision-probe\.sh --base "\.\./AROSVision" "build/arosvision"'
assert_makefile_contains '^arosvision-live-components: arosvision-live-base'
assert_recipe_contains arosvision-live-components '(\$\(MAKE\)|make) aros-ie-live-inputs'
assert_makefile_contains '^arosvision-live-overlays: arosvision-live-components'
assert_recipe_contains arosvision-live-overlays 'scripts/prepare-arosvision-probe\.sh --overlay "\.\./AROSVision" "build/arosvision"'
assert_recipe_contains arosvision-live-overlays 'IE_AROS_DIR=".*AROS"'
assert_makefile_contains '^arosvision-live-tree: arosvision-live-overlays'
assert_makefile_not_contains '^arosvision-live-tree:.*aros-ie-live-assets'
assert_makefile_not_contains '^arosvision-live-tree:.*iewarp-runtime-assets'
assert_recipe_contains arosvision-probe-run 'go run \. -aros -aros-drive "build/arosvision"'
assert_recipe_contains arosvision-probe-run 'missing AROS ROM'
assert_makefile_contains '^arosvision-probe-run: arosvision-probe-tree'
assert_makefile_not_contains '^arosvision-probe-run:.*(intuition-engine|aros-rom|aros-release-assets)'
assert_recipe_not_contains arosvision-probe-run '\$\(MAKE\).*aros-rom'
assert_recipe_not_contains arosvision-probe-run '\$\(MAKE\).*intuition-engine'
assert_recipe_not_contains arosvision-probe-run '\$\(MAKE\).*aros-release-assets'
assert_makefile_contains 'define build-linux-vm-binary'
assert_makefile_contains 'define build-purego-novulkan-vm-binary'
assert_makefile_contains '/opt/ie-sysroots/tumbleweed-aarch64/usr'
assert_makefile_contains 'test-cross-binaries:'
assert_makefile_contains 'test-cross-amd64-binaries:'
assert_makefile_contains 'CROSS_BINARY_PREFIX \?= IntuitionEngine'
assert_makefile_contains 'AB3D2_BINARY_PREFIX \?= IntuitionEngine-AB3D2-Karlos-TKG-High'
assert_makefile_contains 'AB3D2_ORIGINAL_BINARY_PREFIX \?= IntuitionEngine-AB3D2'
assert_makefile_contains '\$\(call build-linux-vm-binary,amd64'
assert_makefile_contains '\$\(call build-linux-vm-binary,arm64'
assert_makefile_contains '\$\(call build-purego-novulkan-vm-binary,\$\$goos,\$\$goarch'
assert_makefile_contains '\$\(call build-purego-novulkan-vm-binary,windows,\$\$goarch'
assert_makefile_contains '\$\(call build-purego-novulkan-vm-binary,darwin,amd64'
assert_makefile_contains '\$\(call build-purego-novulkan-vm-binary,darwin,arm64'
assert_makefile_contains 'AB3D2_SOURCE \?= \.\./alienbreed3d2/ab3d2_source/ie/bin/ab3d2_ie68_redux_high\.ie68'
assert_makefile_contains 'AB3D2_ORIGINAL_SOURCE \?= \.\./alienbreed3d2/ab3d2_source/ie/bin/ab3d2_ie68\.ie68'
assert_makefile_contains 'AB3D2_README_SOURCE := AB3D2_README\.md'
assert_makefile_contains 'AB3D2_ARCHIVE := \$\(AB3D2_BUILD_DIR\)/IntuitionEngine-AB3D2-x64\.zip'
assert_makefile_contains 'python3 scripts/package-ab3d2\.py'
assert_makefile_contains 'AB3D2_START_FULLSCREEN \?= 1'
assert_makefile_contains 'cp "\$\(AB3D2_SOURCE\)" "\$\(AB3D2_EMBED_FILE\)"'
assert_makefile_not_contains 'AB3D2_EMBED_ZIP'
assert_makefile_not_contains 'AB3D2_ASSET_ROOT'
assert_makefile_not_contains 'AB3D2_ASSET_TREE'
assert_makefile_not_contains 'BSDTAR'
assert_makefile_contains 'test-cross-amd64-binaries CROSS_BUILD_DIR=\$\(AB3D2_BUILD_DIR\) CROSS_BINARY_PREFIX=\$\(AB3D2_BINARY_PREFIX\) VM_EMBED_TAGS="embed_ab3d2" EMBEDDED_AB3D2_START_FULLSCREEN=\$\(AB3D2_START_FULLSCREEN\)'
assert_makefile_contains 'test-cross-amd64-binaries CROSS_BUILD_DIR=\$\(AB3D2_BUILD_DIR\) CROSS_BINARY_PREFIX=\$\(AB3D2_ORIGINAL_BINARY_PREFIX\) VM_EMBED_TAGS="embed_ab3d2" EMBEDDED_AB3D2_START_FULLSCREEN=\$\(AB3D2_START_FULLSCREEN\)'
assert_makefile_not_contains '\$\(MAKE\) compress-ab3d2'
assert_makefile_not_contains 'AB3D2_OVERDRIVE_'
assert_makefile_not_contains '^ab3d2-overdrive:'
assert_makefile_not_contains '^ab3d2-all:'
assert_makefile_not_contains 'UPX'
assert_makefile_not_contains 'AB3D2_UPX_FLAGS'
assert_recipe_contains compress-ab3d2 'Skipping AB3D2 binary compression'
assert_ab3d2_starts_fullscreen
assert_ab3d2_target_packages_redux_high
assert_no_nested_external_git_checkouts
assert_ab3d2_prepares_embed_before_build
assert_install_runtime_destdir
assert_dist_layout_skips_non_runtime_archives
assert_phony ie64-cproc
assert_makefile_contains '^all:.*ie64ld.*ie64-cproc.*ie64-ar.*ie64-ranlib'
assert_go_test_inventory_guard_runtime

echo "Makefile checks passed"
