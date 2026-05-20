#!/usr/bin/env bash
#
# Ubuntu x64 Intuition Engine live USB image builder.
# Produces build/x64-live/intuition-engine-x64.img and .tar.zst by default.

set -euo pipefail

COLOR_RESET='\033[0m'
COLOR_CYAN='\033[36m'
COLOR_GREEN='\033[32m'
COLOR_YELLOW='\033[33m'
COLOR_RED='\033[31m'
COLOR_BOLD_CYAN='\033[1;36m'
COLOR_BOLD_GREEN='\033[1;32m'
COLOR_BOLD_RED='\033[1;31m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TIMESTAMP="$(date +%Y%m%d%H%M)"
LIVE_OUT_DIR="${X64_LIVE_OUT_DIR:-${SCRIPT_DIR}/build/x64-live}"
WORK_DIR="${X64_LIVE_WORK_DIR:-${LIVE_OUT_DIR}/work}"
LOG_FILE="${X64_LIVE_LOG_FILE:-${LIVE_OUT_DIR}/build-x64-live-${TIMESTAMP}.log}"
mkdir -p "$LIVE_OUT_DIR"

UBUNTU_VERSION="26.04"
UBUNTU_CLOUD_IMG_URL="https://cloud-images.ubuntu.com/releases/26.04/release/ubuntu-26.04-server-cloudimg-amd64.img"
UBUNTU_CLOUD_IMG="ubuntu-26.04-server-cloudimg-amd64.img"
UBUNTU_CLOUD_IMG_PATH="${WORK_DIR}/${UBUNTU_CLOUD_IMG}"
EXPANDED_IMG="${WORK_DIR}/ubuntu-26.04-ie-expanded.img"
GOLDEN_IMG="ubuntu-26.04-lowlatency-cage-golden.img"
GOLDEN_IMG_PATH="${WORK_DIR}/${GOLDEN_IMG}"
GOLDEN_IMG_MAX_AGE_DAYS=30
GOLDEN_STAMP_VERSION="x64-live-golden-v17-basic-host-apparmor"
GOLDEN_STAMP_PATH="${GOLDEN_IMG_PATH}.stamp"
KERNEL_PKG="linux-lowlatency"
COMPOSITOR_PKGS="cage,seatd,greetd,xwayland,xwayland-run,mesa-utils,libgl1,libegl1,libgles2,libwayland-client0,libxkbcommon0,fonts-dejavu-core"
X11_RUNTIME_PKGS="libxrandr2,libxxf86vm1,libxi6,libxcursor1,libxinerama1,libx11-6,libxext6,libxfixes3,libxrender1"
AUDIO_PKGS="pipewire,pipewire-pulse,wireplumber,pipewire-alsa,alsa-utils,pulseaudio-utils,dbus-user-session"
SECUREBOOT_PKGS="shim-signed,grub-efi-amd64-signed,sbsigntool"
OVERLAY_PKG="overlayroot"
SHARE_GROW_PKGS="cloud-guest-utils,dosfstools,fatresize,parted"
NETWORK_PKGS="network-manager,wpasupplicant,wireless-regdb,iw"
HOST_HELPER_PKGS="policykit-1,ufw,apparmor,apparmor-utils"
ALL_PKGS="${KERNEL_PKG},${COMPOSITOR_PKGS},${X11_RUNTIME_PKGS},${AUDIO_PKGS},${SECUREBOOT_PKGS},${OVERLAY_PKG},${SHARE_GROW_PKGS},${NETWORK_PKGS},${HOST_HELPER_PKGS}"
IE_BINARY="${SCRIPT_DIR}/bin/IntuitionEngine_v3"
IE_INSTALL_NAME="IntuitionEngine"
HOST_HELPER_BINARY="${WORK_DIR}/intuitionengine-host-helper"
AROS_RELEASE_DIR="${AROS_RELEASE_DIR:-${SCRIPT_DIR}/../AROS/bin/ie-m68k/bin/ie-m68k/AROS}"
FINAL_IMAGE_SIZE="8G"
ROOT_PART_SIZE="5G"
FATSHARE_LABEL="IESHARE"
OUTPUT_IMG="${X64_LIVE_OUTPUT_IMG:-${LIVE_OUT_DIR}/intuition-engine-x64.img}"

FORCE_REBUILD_GOLDEN=false
CREATE_SHARE=true
PAYLOAD_CHECK_ONLY=false

usage() {
    cat <<EOF
Usage: $0 [--rebuild-golden] [--no-share] [--check-payload]

  --rebuild-golden  Rebuild the cached Ubuntu package/config golden image.
  --no-share        Do not create the host-visible FAT32 IESHARE partition.
  --check-payload   Validate live-image payload inputs without building an image.
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --rebuild-golden)
            FORCE_REBUILD_GOLDEN=true
            ;;
        --no-share)
            CREATE_SHARE=false
            ;;
        --check-payload)
            PAYLOAD_CHECK_ONLY=true
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "unknown argument: $1" >&2
            usage >&2
            exit 2
            ;;
    esac
    shift
done

log() {
    echo -e "${COLOR_CYAN}>${COLOR_RESET} $*" | tee -a "$LOG_FILE"
}

log_success() {
    echo -e "${COLOR_BOLD_GREEN}OK${COLOR_RESET} ${COLOR_GREEN}$*${COLOR_RESET}" | tee -a "$LOG_FILE"
}

log_warn() {
    echo -e "${COLOR_YELLOW}WARN${COLOR_RESET} ${COLOR_YELLOW}$*${COLOR_RESET}" | tee -a "$LOG_FILE"
}

log_error() {
    echo -e "${COLOR_BOLD_RED}ERR${COLOR_RESET} ${COLOR_RED}$*${COLOR_RESET}" | tee -a "$LOG_FILE"
}

log_section() {
    echo "" | tee -a "$LOG_FILE"
    echo -e "${COLOR_BOLD_CYAN}== $* ==${COLOR_RESET}" | tee -a "$LOG_FILE"
}

cleanup() {
    local status=$?
    if [[ $status -ne 0 ]]; then
        log_warn "Build failed. Work directory kept for inspection: ${WORK_DIR}"
    fi
}

trap cleanup EXIT

check_dependencies() {
    log_section "Checking dependencies"
    local required_cmds=(aria2c curl virt-customize virt-resize virt-filesystems guestfish qemu-img file zstd tar python3 go)
    if [[ "${CREATE_SHARE}" == "true" ]]; then
        required_cmds+=(mformat mcopy)
    fi

    local missing_deps=()
    local cmd
    for cmd in "${required_cmds[@]}"; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            missing_deps+=("$cmd")
        else
            log_success "$cmd found"
        fi
    done
    if [[ ${#missing_deps[@]} -gt 0 ]]; then
        log_error "Missing required dependencies: ${missing_deps[*]}"
        log_error "Debian/Ubuntu packages: libguestfs-tools aria2 curl qemu-utils mtools zstd"
        log_error "openSUSE packages: libguestfs guestfs-tools aria2 curl qemu-tools mtools zstd"
        exit 1
    fi

    if [[ ! -f "$IE_BINARY" ]]; then
        log_error "Intuition Engine x86-64-v3 binary not found: $IE_BINARY"
        log_error "Run: make x86-64-v3"
        exit 1
    fi
    if ! file "$IE_BINARY" | grep -q "x86-64"; then
        log_error "Binary is not x86-64: $IE_BINARY"
        exit 1
    fi

    if [[ "${CREATE_SHARE}" == "true" && ! -f "${SCRIPT_DIR}/embedded/ab3d2/_build.zip" ]]; then
        log_error "AB3D2 embedded asset zip not found: ${SCRIPT_DIR}/embedded/ab3d2/_build.zip"
        log_error "Run: make x64-live-demos"
        exit 1
    fi
    if [[ "${CREATE_SHARE}" == "true" && ! -f "${AROS_RELEASE_DIR}/S/Startup-Sequence" ]]; then
        log_error "AROS release tree not found: ${AROS_RELEASE_DIR}"
        log_error "Run: make x64-live-demos"
        exit 1
    fi
    if [[ "${CREATE_SHARE}" == "true" && ! -f "${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Tool.info" ]]; then
        log_error "AROS default tool icon not found: ${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Tool.info"
        log_error "Run: make x64-live-demos"
        exit 1
    fi
    if [[ "${CREATE_SHARE}" == "true" && ! -f "${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Drawer.info" ]]; then
        log_error "AROS default drawer icon not found: ${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Drawer.info"
        log_error "Run: make x64-live-demos"
        exit 1
    fi
    if [[ "${CREATE_SHARE}" == "true" && ! -f "${AROS_RELEASE_DIR}/Libs/iewarp.library" ]]; then
        log_error "AROS IEWarp library not found: ${AROS_RELEASE_DIR}/Libs/iewarp.library"
        log_error "Run: make x64-live-demos"
        exit 1
    fi

    local available_space
    available_space="$(df -BG "$SCRIPT_DIR" | awk 'NR==2 {print $4}' | sed 's/G//')"
    if [[ "$available_space" -lt 18 ]]; then
        log_error "Insufficient disk space. Need 18GB, have ${available_space}GB"
        exit 1
    fi
    log_success "Sufficient disk space available (${available_space}GB)"
}

payload_require_file() {
    local path="$1"
    local producer="$2"
    local label="$3"
    if [[ ! -f "$path" ]]; then
        log_error "Missing live payload input: $label"
        log_error "Path: $path"
        log_error "Producer: $producer"
        exit 1
    fi
}

payload_require_glob() {
    local pattern="$1"
    local producer="$2"
    local label="$3"
    if ! compgen -G "$pattern" >/dev/null; then
        log_error "Missing live payload input set: $label"
        log_error "Pattern: $pattern"
        log_error "Producer: $producer"
        exit 1
    fi
}

check_live_payload_inputs() {
    log_section "Checking live payload manifest inputs"
    local payload_cmd
    for payload_cmd in file python3; do
        if ! command -v "$payload_cmd" >/dev/null 2>&1; then
            log_error "$payload_cmd is required to validate the live payload manifest"
            exit 1
        fi
    done

    payload_require_file "$IE_BINARY" "make x86-64-v3" "x86-64-v3 Intuition Engine binary"
    if ! file "$IE_BINARY" | grep -q "x86-64"; then
        log_error "Live payload binary is not x86-64: $IE_BINARY"
        log_error "Producer: make x86-64-v3"
        exit 1
    fi

    payload_require_file "${SCRIPT_DIR}/embedded/ab3d2/_build.zip" "make x64-live-ab3d2-assets" "AB3D2 embedded runtime asset zip"
    payload_require_file "${AROS_RELEASE_DIR}/S/Startup-Sequence" "make aros-release-assets" "AROS Startup-Sequence"
    payload_require_file "${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Tool.info" "make aros-release-assets" "AROS default tool icon"
    payload_require_file "${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Drawer.info" "make aros-release-assets" "AROS default drawer icon"
    payload_require_file "${AROS_RELEASE_DIR}/Libs/iewarp.library" "make aros-iewarp-library" "AROS IEWarp library"
    if ! grep -a -q 'Systems/AROS/Libs/iewarp_service.ie64' "${AROS_RELEASE_DIR}/Libs/iewarp.library"; then
        log_error "AROS IEWarp library does not contain Systems/AROS/Libs/iewarp_service.ie64"
        log_error "Producer: make aros-iewarp-library"
        exit 1
    fi
    payload_require_file "${SCRIPT_DIR}/sdk/examples/prebuilt/iewarp_service.ie64" "make iewarp-runtime-assets" "IEWarp IE64 worker"
    payload_require_file "${AROS_RELEASE_DIR}/Libs/iewarp_service.ie64" "make iewarp-runtime-assets" "AROS release IEWarp worker copy"
    payload_require_file "${SCRIPT_DIR}/sdk/intuitionos/iexec/iexec.ie64" "make intuitionos" "IntuitionOS IExec kernel"
    payload_require_file "${SCRIPT_DIR}/sdk/intuitionos/system/SYS/IOSSYS/S/Startup-Sequence" "make intuitionos" "IntuitionOS Startup-Sequence"
    payload_require_file "${SCRIPT_DIR}/sdk/intuitionos/system/SYS/IOSSYS/Tools/Shell" "make intuitionos" "IntuitionOS Shell"
    payload_require_file "${SCRIPT_DIR}/sdk/intuitionos/system/SYS/IOSSYS/LIBS/dos.library" "make intuitionos" "IntuitionOS dos.library"
    payload_require_file "${SCRIPT_DIR}/sdk/intuitionos/system/SYS/IOSSYS/L/console.handler" "make intuitionos" "IntuitionOS console.handler"

    payload_require_file "${SCRIPT_DIR}/sdk/examples/basic/rotozoomer_basic.bas" "make sdk-build" "EhBASIC live demo source"
    payload_require_file "${SCRIPT_DIR}/sdk/examples/prebuilt/rotozoomer_gem.prg" "make gem-rotozoomer" "EmuTOS GEM rotozoomer demo"
    payload_require_file "${SCRIPT_DIR}/sdk/examples/asm/RotoAPI" "make x64-live-aros-demos" "AROS RotoAPI demo"
    payload_require_file "${SCRIPT_DIR}/sdk/examples/asm/RotoHW" "make x64-live-aros-demos" "AROS RotoHW demo"
    payload_require_file "${SCRIPT_DIR}/sdk/examples/c/RotoAPIc" "make x64-live-aros-demos" "AROS RotoAPIc demo"
    payload_require_file "${SCRIPT_DIR}/sdk/examples/c/RotoHWc" "make x64-live-aros-demos" "AROS RotoHWc demo"

    payload_require_glob "${SCRIPT_DIR}/sdk/examples/prebuilt/*.ie*" "make sdk-build" "prebuilt Intuition Engine binaries"
    payload_require_glob "${SCRIPT_DIR}/sdk/examples/prebuilt/coproc_*.ie*" "make sdk-build" "coprocessor support worker binaries"
    payload_require_glob "${SCRIPT_DIR}/sdk/examples/asm/*.asm" "make sdk-build" "SDK assembly source examples"
    payload_require_glob "${SCRIPT_DIR}/sdk/examples/basic/*.bas" "make sdk-build" "SDK BASIC source examples"
    payload_require_glob "${SCRIPT_DIR}/sdk/examples/c/*.c" "make sdk-build" "SDK C source examples"

    local include
    for include in ie32.inc ie64.inc ie65.inc ie68.inc ie80.inc ie86.inc; do
        payload_require_file "${SCRIPT_DIR}/sdk/include/${include}" "make sdk-build" "SDK include ${include}"
    done

    local doc
    for doc in Coprocessor.md demo-matrix.md ehbasic_ie64.md ie_emutos.md iescript.md iewarp.md iemon.md include-files.md sdk-getting-started.md toolchains.md; do
        payload_require_file "${SCRIPT_DIR}/sdk/docs/${doc}" "make sdk-build" "SDK doc ${doc}"
    done

    python3 - "${SCRIPT_DIR}/embedded/ab3d2/_build.zip" <<'PY'
import sys
import zipfile

zip_path = sys.argv[1]
roots = (
    "ab3d2_source/_build/ie_media/redux-high/",
    "ab3d2_source/_build/ie_unpacked/",
)
with zipfile.ZipFile(zip_path) as zf:
    names = [info.filename for info in zf.infolist() if not info.is_dir()]
    for name in names:
        if name.startswith("/") or ".." in name.split("/"):
            raise SystemExit(f"unsafe AB3D2 zip entry path: {name}")
    if not any(any(name.startswith(root) for root in roots) for name in names):
        raise SystemExit("AB3D2 zip does not contain live runtime roots")
PY

    log_success "Live payload manifest inputs are ready"
}

verify_staged_share_payload() {
    local payload_root="$1"
    log_section "Verifying staged IESHARE payload"

    payload_require_file "${payload_root}/README.TXT" "build_x64_ie_img.sh stage_share_payload" "IESHARE root README"
    payload_require_file "${payload_root}/Demos/README.TXT" "build_x64_ie_img.sh stage_share_payload" "Demos README"
    payload_require_file "${payload_root}/IE/Coproc/README.TXT" "build_x64_ie_img.sh stage_share_payload" "IE/Coproc README"
    payload_require_file "${payload_root}/SDK/README.TXT" "build_x64_ie_img.sh stage_share_payload" "SDK README"
    payload_require_file "${payload_root}/Systems/README.TXT" "build_x64_ie_img.sh stage_share_payload" "Systems README"

    payload_require_file "${payload_root}/Systems/AROS/S/Startup-Sequence" "make aros-release-assets" "staged AROS Startup-Sequence"
    payload_require_file "${payload_root}/Systems/AROS/Libs/iewarp.library" "make aros-iewarp-library" "staged AROS IEWarp library"
    payload_require_file "${payload_root}/Systems/AROS/Libs/iewarp_service.ie64" "make iewarp-runtime-assets" "staged AROS IEWarp worker"
    payload_require_file "${payload_root}/Systems/AROS/Demos/RotoAPI" "make x64-live-aros-demos" "staged AROS RotoAPI demo"
    payload_require_file "${payload_root}/Systems/AROS/Demos/RotoHW" "make x64-live-aros-demos" "staged AROS RotoHW demo"
    payload_require_file "${payload_root}/Systems/AROS/Demos/RotoAPIc" "make x64-live-aros-demos" "staged AROS RotoAPIc demo"
    payload_require_file "${payload_root}/Systems/AROS/Demos/RotoHWc" "make x64-live-aros-demos" "staged AROS RotoHWc demo"
    payload_require_file "${payload_root}/Systems/EmuTOS/Demos/rotozoomer_gem.prg" "make gem-rotozoomer" "staged EmuTOS GEM demo"
    payload_require_file "${payload_root}/Systems/IntuitionOS/Boot/iexec.ie64" "make intuitionos" "staged IntuitionOS IExec kernel"
    payload_require_file "${payload_root}/Systems/IntuitionOS/IOSSYS/S/Startup-Sequence" "make intuitionos" "staged IntuitionOS Startup-Sequence"
    payload_require_file "${payload_root}/Systems/IntuitionOS/IOSSYS/Tools/Shell" "make intuitionos" "staged IntuitionOS Shell"
    payload_require_file "${payload_root}/Systems/IntuitionOS/IOSSYS/LIBS/dos.library" "make intuitionos" "staged IntuitionOS dos.library"
    payload_require_file "${payload_root}/Systems/IntuitionOS/IOSSYS/L/console.handler" "make intuitionos" "staged IntuitionOS console.handler"
    payload_require_file "${payload_root}/Demos/rotozoomer_basic.bas" "make sdk-build" "staged BASIC demo"
    payload_require_file "${payload_root}/IE/Coproc/coproc_service_ie32.iex" "make sdk-build" "staged IE32 coprocessor worker"
    payload_require_file "${payload_root}/SDK/Docs/iewarp.md" "make sdk-build" "staged IEWarp documentation"
    payload_require_file "${payload_root}/SDK/Include/ie64.inc" "make sdk-build" "staged IE64 include"

    if find "${payload_root}/Demos" -maxdepth 1 -type f -name 'coproc_*.ie*' | grep -q .; then
        log_error "Forbidden live payload location: coproc worker staged under Demos"
        log_error "Producer: build_x64_ie_img.sh stage_share_payload"
        exit 1
    fi
    if find "${payload_root}/Demos" -maxdepth 1 -type f \( -name 'RotoAPI' -o -name 'RotoHW' -o -name 'RotoAPIc' -o -name 'RotoHWc' -o -name 'iewarp_service.ie64' \) | grep -q .; then
        log_error "Forbidden live payload location: AROS-only payload staged under Demos"
        log_error "Producer: build_x64_ie_img.sh stage_share_payload"
        exit 1
    fi
    if [[ -e "${payload_root}/IE/Warp" || -e "${payload_root}/sdk/examples/asm/iewarp_service.ie64" ]]; then
        log_error "Forbidden legacy IEWarp worker path staged in live payload"
        log_error "Expected: Systems/AROS/Libs/iewarp_service.ie64"
        exit 1
    fi
    if [[ -e "${payload_root}/Demos/iexec.ie64" ||
          -e "${payload_root}/Demos/IOSSYS" ||
          -e "${payload_root}/SDK/iexec.ie64" ||
          -e "${payload_root}/SDK/IOSSYS" ||
          -e "${payload_root}/Systems/AROS/Boot/iexec.ie64" ||
          -e "${payload_root}/Systems/AROS/IOSSYS" ]]; then
        log_error "Forbidden live payload location: IntuitionOS files staged outside Systems/IntuitionOS"
        log_error "Expected: Systems/IntuitionOS"
        exit 1
    fi
    if ! grep -a -q 'Systems/AROS/Libs/iewarp_service.ie64' "${payload_root}/Systems/AROS/Libs/iewarp.library"; then
        log_error "Staged AROS iewarp.library does not contain Systems/AROS/Libs/iewarp_service.ie64"
        log_error "Producer: make aros-iewarp-library"
        exit 1
    fi

    log_success "Staged IESHARE payload matches the live manifest"
}

stage_share_payload() {
    log_section "Staging IESHARE demo payload"
    local payload_root="${WORK_DIR}/ieshare-payload"
    local demos_dir="${payload_root}/Demos"
    local coproc_dir="${payload_root}/IE/Coproc"
    local sdk_dir="${payload_root}/SDK"
    local systems_dir="${payload_root}/Systems"
    local aros_system_dir="${systems_dir}/AROS"
    local aros_demos_dir="${aros_system_dir}/Demos"
    local emutos_system_dir="${systems_dir}/EmuTOS"
    local emutos_demos_dir="${emutos_system_dir}/Demos"
    local intuitionos_system_dir="${systems_dir}/IntuitionOS"
    rm -rf "$payload_root"
    mkdir -p "$payload_root" "$aros_system_dir"
    python3 - "${AROS_RELEASE_DIR}" "$aros_system_dir" <<'PY'
import os
import shutil
import stat
import sys

src_root, dst_root = sys.argv[1], sys.argv[2]

def choose_name(entries):
    names = [entry.name for entry in entries]
    exact_lower = [name for name in names if name == name.lower()]
    if exact_lower:
        return sorted(exact_lower)[0]
    return sorted(names, key=lambda name: (sum(1 for ch in name if ch.isupper()), name.lower(), name))[0]

def copy_group(entries, dst_dir):
    dirs = [entry for entry in entries if entry.is_dir(follow_symlinks=False)]
    files = [entry for entry in entries if entry.is_file(follow_symlinks=False)]
    links = [entry for entry in entries if entry.is_symlink()]
    if links:
        raise SystemExit(f"AROS release tree contains unsupported symlink case collision: {links[0].path}")
    if dirs and files:
        raise SystemExit(f"AROS release tree contains file/directory case collision: {entries[0].path}")
    dst_name = choose_name(entries)
    dst_path = os.path.join(dst_dir, dst_name)
    if dirs:
        os.makedirs(dst_path, exist_ok=True)
        merged = {}
        for directory in dirs:
            for child in os.scandir(directory.path):
                merged.setdefault(child.name.lower(), []).append(child)
        for key in sorted(merged):
            copy_group(merged[key], dst_path)
        return
    if not files:
        return
    selected = next(entry for entry in files if entry.name == dst_name)
    if len(files) > 1:
        skipped = ", ".join(entry.path for entry in files if entry.path != selected.path)
        print(f"case-colliding AROS file: keeping {selected.path}; skipping {skipped}", file=sys.stderr)
    shutil.copy2(selected.path, dst_path)
    mode = stat.S_IMODE(os.stat(selected.path, follow_symlinks=False).st_mode)
    os.chmod(dst_path, mode)

top = {}
for entry in os.scandir(src_root):
    top.setdefault(entry.name.lower(), []).append(entry)
for key in sorted(top):
    copy_group(top[key], dst_root)
PY
    mkdir -p "$demos_dir" "$coproc_dir" \
        "$aros_system_dir/Libs" "$aros_demos_dir" "$emutos_demos_dir" \
        "$intuitionos_system_dir/Boot" \
        "$sdk_dir/Include" "$sdk_dir/Docs" "$sdk_dir/Examples/asm" \
        "$sdk_dir/Examples/basic" "$sdk_dir/Examples/c"
    cp -f "${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Drawer.info" "${payload_root}/Demos.info"
    cp -f "${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Drawer.info" "${systems_dir}/AROS.info"
    cp -f "${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Drawer.info" "${systems_dir}/EmuTOS.info"
    cp -f "${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Drawer.info" "${systems_dir}/IntuitionOS.info"
    cp -a "${SCRIPT_DIR}/sdk/intuitionos/system/SYS/." "$intuitionos_system_dir/"
    cp -f "${SCRIPT_DIR}/sdk/intuitionos/iexec/iexec.ie64" "$intuitionos_system_dir/Boot/iexec.ie64"

    shopt -s nullglob
    local prebuilt_files=(
        "${SCRIPT_DIR}"/sdk/examples/prebuilt/*.ie*
        "${SCRIPT_DIR}"/sdk/examples/prebuilt/*.prg
    )
    shopt -u nullglob

    if [[ ${#prebuilt_files[@]} -eq 0 ]]; then
        log_error "No .ie* or .prg demos found in ${SCRIPT_DIR}/sdk/examples/prebuilt"
        log_error "Run: make x64-live-demos"
        exit 1
    fi
    local demo_files=()
    local coproc_files=()
    local emutos_demo_files=()
    local iewarp_worker_files=()
    local prebuilt_file
    for prebuilt_file in "${prebuilt_files[@]}"; do
        case "$(basename "$prebuilt_file")" in
            coproc_*.ie*) coproc_files+=("$prebuilt_file") ;;
            iewarp_service.ie64) iewarp_worker_files+=("$prebuilt_file") ;;
            *.prg) emutos_demo_files+=("$prebuilt_file") ;;
            *) demo_files+=("$prebuilt_file") ;;
        esac
    done
    if [[ ${#demo_files[@]} -gt 0 ]]; then
        cp -f "${demo_files[@]}" "$demos_dir/"
    fi
    if [[ ${#coproc_files[@]} -gt 0 ]]; then
        cp -f "${coproc_files[@]}" "$coproc_dir/"
    fi
    if [[ ${#emutos_demo_files[@]} -gt 0 ]]; then
        cp -f "${emutos_demo_files[@]}" "$emutos_demos_dir/"
    fi
    if [[ ${#iewarp_worker_files[@]} -gt 0 ]]; then
        cp -f "${iewarp_worker_files[@]}" "$aros_system_dir/Libs/"
    fi
    cp -f "${SCRIPT_DIR}/sdk/examples/basic/rotozoomer_basic.bas" "$demos_dir/"

    cp -f \
        "${SCRIPT_DIR}/sdk/include/ie32.inc" \
        "${SCRIPT_DIR}/sdk/include/ie64.inc" \
        "${SCRIPT_DIR}/sdk/include/ie65.inc" \
        "${SCRIPT_DIR}/sdk/include/ie68.inc" \
        "${SCRIPT_DIR}/sdk/include/ie80.inc" \
        "${SCRIPT_DIR}/sdk/include/ie86.inc" \
        "$sdk_dir/Include/"
    cp -f "${SCRIPT_DIR}/sdk/README.md" "$sdk_dir/README.md"
    cp -f \
        "${SCRIPT_DIR}/sdk/docs/Coprocessor.md" \
        "${SCRIPT_DIR}/sdk/docs/demo-matrix.md" \
        "${SCRIPT_DIR}/sdk/docs/ehbasic_ie64.md" \
        "${SCRIPT_DIR}/sdk/docs/ie_emutos.md" \
        "${SCRIPT_DIR}/sdk/docs/iescript.md" \
        "${SCRIPT_DIR}/sdk/docs/iewarp.md" \
        "${SCRIPT_DIR}/sdk/docs/iemon.md" \
        "${SCRIPT_DIR}/sdk/docs/include-files.md" \
        "${SCRIPT_DIR}/sdk/docs/sdk-getting-started.md" \
        "${SCRIPT_DIR}/sdk/docs/toolchains.md" \
        "$sdk_dir/Docs/"
    cp -f "${SCRIPT_DIR}"/sdk/examples/asm/*.asm "$sdk_dir/Examples/asm/"
    cp -f "${SCRIPT_DIR}"/sdk/examples/basic/*.bas "$sdk_dir/Examples/basic/"
    cp -f "${SCRIPT_DIR}"/sdk/examples/c/*.c "$sdk_dir/Examples/c/"

    local aros_demo
    for aros_demo in \
        "${SCRIPT_DIR}/sdk/examples/asm/RotoAPI" \
        "${SCRIPT_DIR}/sdk/examples/asm/RotoHW" \
        "${SCRIPT_DIR}/sdk/examples/c/RotoAPIc" \
        "${SCRIPT_DIR}/sdk/examples/c/RotoHWc"; do
        if [[ ! -f "$aros_demo" ]]; then
            log_error "Missing AROS demo executable: $aros_demo"
            log_error "Run: make x64-live-demos"
            exit 1
        fi
        cp -f "$aros_demo" "$aros_demos_dir/"
        cp -f "${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Tool.info" "${aros_demos_dir}/$(basename "$aros_demo").info"
    done

    python3 - "${SCRIPT_DIR}/embedded/ab3d2/_build.zip" "$demos_dir" <<'PY'
import hashlib
import os
import sys
import zipfile

zip_path, dest = sys.argv[1], sys.argv[2]
dest_real = os.path.realpath(dest)
runtime_roots = (
    "ab3d2_source/_build/ie_media/redux-high/",
    "ab3d2_source/_build/ie_unpacked/",
)
seen = {}
with zipfile.ZipFile(zip_path) as zf:
    for info in zf.infolist():
        name = info.filename
        if name.startswith("/") or ".." in name.split("/"):
            raise SystemExit(f"unsafe zip entry path: {name}")
        if info.is_dir() or not name.startswith(runtime_roots):
            continue
        fat_name = name.lower()
        data = zf.read(info)
        digest = hashlib.sha256(data).hexdigest()
        existing = seen.get(fat_name)
        if existing is not None:
            if existing != digest:
                raise SystemExit(f"case-colliding AB3D2 asset differs: {name}")
            continue
        seen[fat_name] = digest
        target = os.path.realpath(os.path.join(dest, fat_name))
        if target != dest_real and not target.startswith(dest_real + os.sep):
            raise SystemExit(f"zip entry escapes destination: {name}")
        os.makedirs(os.path.dirname(target), exist_ok=True)
        with open(target, "wb") as out:
            out.write(data)
PY

    cat > "${demos_dir}/README.TXT" <<'EOF'
Intuition Engine Live USB demos

This folder is populated during make x64-live.
It contains runnable Intuition Engine demo binaries and the AB3D2 runtime asset
tree extracted from the embedded release payload.

OS-specific payloads live under Systems:

Systems/AROS/Demos
Systems/EmuTOS/Demos
Systems/IntuitionOS
EOF

    cat > "${payload_root}/README.TXT" <<'EOF'
Intuition Engine Live USB share

Top-level folders:

Demos    Bare-metal Intuition Engine demos and shared runtime assets.
IE       Intuition Engine runtime support files.
SDK      Reference include files, docs, and source examples.
Systems  Guest OS payloads.

AROS files live under Systems/AROS.
EmuTOS/GEMDOS demo files live under Systems/EmuTOS.
IntuitionOS SYS: lives under Systems/IntuitionOS.
EOF

    cat > "${systems_dir}/README.TXT" <<'EOF'
Guest OS payloads

Systems/AROS is the AROS SYS: root used by the live image.
Systems/AROS/Demos contains AROS-native demo programs.
Systems/AROS/Libs contains AROS libraries and private library resources,
including iewarp_service.ie64 for iewarp.library.

Systems/EmuTOS is the EmuTOS GEMDOS drive root used by the live image.
Systems/EmuTOS/Demos contains GEMDOS demo programs.

Systems/IntuitionOS is the IntuitionOS SYS: root used by the live image.
Systems/IntuitionOS/Boot/iexec.ie64 is the host bootstrap kernel image.
Systems/IntuitionOS/IOSSYS is the read-only system subtree visible inside
IntuitionOS as IOSSYS: through SYS:IOSSYS.
EOF

    cat > "${coproc_dir}/README.TXT" <<'EOF'
Intuition Engine coprocessor support images

These binaries are support payloads for software that starts coprocessor
workers. They are resolved relative to the IESHARE root, for example:

IE/Coproc/coproc_service_ie32.iex
EOF

    cat > "${sdk_dir}/README.TXT" <<'EOF'
Intuition Engine reference SDK

This is a documentation and source-reference snapshot for the live image. It
contains include files, selected SDK docs, and source examples. It intentionally
does not include sdk/bin or other host tool binaries; build programs on the
host SDK, then copy runnable outputs to IESHARE.

The live image keeps OS payloads under Systems/AROS, Systems/EmuTOS, and
Systems/IntuitionOS.
The AROS IEWarp worker is a private runtime resource at
Systems/AROS/Libs/iewarp_service.ie64, visible inside AROS as
SYS:Libs/iewarp_service.ie64.
EOF

    find "$demos_dir" -maxdepth 2 -type f | sort | sed "s#^${payload_root}/#  #" | tee -a "$LOG_FILE"
    find "$systems_dir" -maxdepth 3 -type f | sort | sed "s#^${payload_root}/#  #" | tee -a "$LOG_FILE"
    find "$coproc_dir" -maxdepth 1 -type f | sort | sed "s#^${payload_root}/#  #" | tee -a "$LOG_FILE"
    find "$sdk_dir" -maxdepth 3 -type f | sort | sed "s#^${payload_root}/#  #" | tee -a "$LOG_FILE"
    verify_staged_share_payload "$payload_root"
    SHARE_PAYLOAD_ROOT="$payload_root"
    export SHARE_PAYLOAD_ROOT
}

download_ubuntu() {
    log_section "Downloading Ubuntu ${UBUNTU_VERSION} cloud image"
    mkdir -p "$WORK_DIR"

    if [[ -f "$UBUNTU_CLOUD_IMG_PATH" ]]; then
        log_success "Cloud image already present: $UBUNTU_CLOUD_IMG_PATH"
        return 0
    fi
    if [[ -f "${SCRIPT_DIR}/${UBUNTU_CLOUD_IMG}" ]]; then
        log "Using cloud image from repository root"
        cp "${SCRIPT_DIR}/${UBUNTU_CLOUD_IMG}" "$UBUNTU_CLOUD_IMG_PATH"
        return 0
    fi

    if ! curl -fsSI "$UBUNTU_CLOUD_IMG_URL" >/dev/null; then
        log_error "Cloud image URL is not reachable: ${UBUNTU_CLOUD_IMG_URL}"
        log_error "Verify the Ubuntu ${UBUNTU_VERSION} release path."
        exit 1
    fi

    aria2c -x8 -s8 -k1M \
        --continue=true \
        --max-connection-per-server=8 \
        --min-split-size=1M \
        --file-allocation=falloc \
        --max-tries=5 \
        --retry-wait=3 \
        -d "$WORK_DIR" \
        -o "$UBUNTU_CLOUD_IMG" \
        "$UBUNTU_CLOUD_IMG_URL" 2>&1 | tee -a "$LOG_FILE"
}

generate_support_files() {
    log_section "Generating image support files"
cat > "${WORK_DIR}/launch.sh" <<'EOF'
#!/bin/sh
if [ -d /var/ie/share ]; then
    cd /var/ie/share || cd /opt/ie
    set --
    if [ -d /var/ie/share/Systems/EmuTOS ]; then
        set -- "$@" -emutos-drive /var/ie/share/Systems/EmuTOS
    fi
    if [ -d /var/ie/share/Systems/AROS ]; then
        set -- "$@" -aros-drive /var/ie/share/Systems/AROS
    fi
    if [ -d /var/ie/share/Systems/IntuitionOS ]; then
        set -- "$@" -intuitionos-root /var/ie/share/Systems/IntuitionOS
    fi
    if [ -f /var/ie/share/Systems/IntuitionOS/Boot/iexec.ie64 ]; then
        set -- "$@" -intuitionos-image /var/ie/share/Systems/IntuitionOS/Boot/iexec.ie64
    fi
else
    cd /opt/ie
    set --
fi
if [ -z "${IE_LIVE_DBUS_SESSION:-}" ] && command -v dbus-run-session >/dev/null 2>&1; then
    export IE_LIVE_DBUS_SESSION=1
    exec dbus-run-session -- "$0" "$@"
fi
if [ -z "${XDG_RUNTIME_DIR:-}" ] || [ ! -d "$XDG_RUNTIME_DIR" ]; then
    export XDG_RUNTIME_DIR="/tmp/ie-runtime-$(id -u)"
fi
export PIPEWIRE_RUNTIME_DIR="$XDG_RUNTIME_DIR"
export PULSE_RUNTIME_PATH="$XDG_RUNTIME_DIR/pulse"
export PULSE_SERVER="unix:${XDG_RUNTIME_DIR}/pulse/native"
mkdir -p "$XDG_RUNTIME_DIR" "$XDG_RUNTIME_DIR/pulse"
chmod 700 "$XDG_RUNTIME_DIR"
pipewire >/tmp/ie-pipewire.log 2>&1 &
for _ in $(seq 1 100); do
    [ -S "${XDG_RUNTIME_DIR}/pipewire-0" ] && break
    sleep 0.05
done
wireplumber >/tmp/ie-wireplumber.log 2>&1 &
pipewire-pulse >/tmp/ie-pipewire-pulse.log 2>&1 &
for _ in $(seq 1 100); do
    [ -S "${XDG_RUNTIME_DIR}/pulse/native" ] && pactl info >/tmp/ie-pactl-info.log 2>&1 && break
    sleep 0.05
done
if ! pactl info >/tmp/ie-pactl-info.log 2>&1; then
    {
        echo "pipewire-pulse did not become ready at ${XDG_RUNTIME_DIR}/pulse/native"
        echo "PULSE_SERVER=${PULSE_SERVER}"
        echo "--- pactl info ---"
        cat /tmp/ie-pactl-info.log 2>/dev/null || true
        echo "--- pipewire-pulse log ---"
        cat /tmp/ie-pipewire-pulse.log 2>/dev/null || true
        echo "--- pipewire log ---"
        cat /tmp/ie-pipewire.log 2>/dev/null || true
    } >/tmp/ie-pipewire-ready.log
else
    echo "pipewire-pulse ready at ${XDG_RUNTIME_DIR}/pulse/native" >/tmp/ie-pipewire-ready.log
fi
exec /opt/ie/IntuitionEngine -basic -ehbasic-host -ehbasic-host-appliance "$@"
EOF
    chmod +x "${WORK_DIR}/launch.sh"

    cat > "${WORK_DIR}/greetd-config.toml" <<'EOF'
[terminal]
vt = 1

[default_session]
command = "cage -s -- xwayland-run -- /opt/ie/launch.sh"
user = "ie"
EOF

    cat > "${WORK_DIR}/overlayroot.conf" <<'EOF'
overlayroot="tmpfs"
EOF

    cat > "${WORK_DIR}/90-ie-networkmanager.yaml" <<'EOF'
network:
  version: 2
  renderer: NetworkManager
EOF

    cat > "${WORK_DIR}/ie-grow-share.sh" <<'EOF'
#!/bin/sh
set -eu

log() {
    printf '%s\n' "ie-grow-share: $*"
}

share="$(blkid -L IESHARE 2>/dev/null || true)"
if [ -z "$share" ]; then
    log "IESHARE label not found; skipping"
    exit 0
fi

part="$(readlink -f "$share")"
disk_name="$(lsblk -no PKNAME "$part" | head -n1 | tr -d '[:space:]')"
part_num="$(lsblk -no PARTN "$part" | head -n1 | tr -d '[:space:]')"
if [ -z "$disk_name" ] || [ -z "$part_num" ]; then
    log "could not resolve parent disk/partition number for $part; skipping"
    exit 0
fi
disk="/dev/${disk_name}"

before="$(blockdev --getsize64 "$part" 2>/dev/null || echo 0)"
if growpart "$disk" "$part_num"; then
    partprobe "$disk" || true
    udevadm settle || true
else
    log "growpart reported no growth or failed; continuing to filesystem check"
fi
after="$(blockdev --getsize64 "$part" 2>/dev/null || echo "$before")"

tmp="$(mktemp -d /run/ie-share-grow.XXXXXX)"
cleanup() {
    umount "$tmp" 2>/dev/null || true
    rmdir "$tmp" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

if [ "$after" != "$before" ]; then
    if ! command -v fatresize >/dev/null 2>&1; then
        log "fatresize unavailable; preserving existing FAT contents at original size"
        exit 0
    fi
    fatresize -s "$after" "$part" || {
        log "fatresize failed; preserving existing FAT contents without reformatting"
        exit 0
    }
fi

mount -o rw "$part" "$tmp"
if [ -e "$tmp/.ie-share-initialized" ]; then
    log "IESHARE already initialized"
    exit 0
fi
touch "$tmp/.ie-share-initialized"
chown 1000:1000 "$tmp" "$tmp/.ie-share-initialized" 2>/dev/null || true
umount "$tmp"
log "IESHARE ready on $part"
EOF
    chmod +x "${WORK_DIR}/ie-grow-share.sh"

    cat > "${WORK_DIR}/ie-grow-share.service" <<'EOF'
[Unit]
Description=Grow Intuition Engine host share partition
DefaultDependencies=no
Wants=systemd-udev-settle.service
After=systemd-udev-settle.service
Before=local-fs-pre.target

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/ie-grow-share.sh
RemainAfterExit=yes

[Install]
WantedBy=sysinit.target
EOF

    cat > "${WORK_DIR}/ie-firewall.service" <<'EOF'
[Unit]
Description=Apply Intuition Engine live image firewall baseline
Wants=network-pre.target
Before=network-pre.target

[Service]
Type=oneshot
ExecStart=/usr/sbin/ufw default deny incoming
ExecStart=/usr/sbin/ufw default allow outgoing
ExecStart=/usr/sbin/ufw --force enable
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
WantedBy=network-pre.target
EOF

    cat > "${WORK_DIR}/ie-apparmor.service" <<'EOF'
[Unit]
Description=Enforce Intuition Engine AppArmor profiles
After=apparmor.service
Wants=apparmor.service
Before=greetd.service

[Service]
Type=oneshot
ExecStart=/usr/sbin/apparmor_parser -r /etc/apparmor.d/opt.ie.IntuitionEngine /etc/apparmor.d/usr.libexec.intuitionengine-host-helper
ExecStart=/usr/sbin/aa-enforce /etc/apparmor.d/opt.ie.IntuitionEngine /etc/apparmor.d/usr.libexec.intuitionengine-host-helper
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF

    cat > "${WORK_DIR}/org.intuitionengine.host.policy" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE policyconfig PUBLIC
 "-//freedesktop//DTD PolicyKit Policy Configuration 1.0//EN"
 "http://www.freedesktop.org/standards/PolicyKit/1/policyconfig.dtd">
<policyconfig>
  <vendor>Intuition Engine</vendor>
  <vendor_url>https://github.com/intuitionamiga/IntuitionEngine</vendor_url>
  <action id="org.intuitionengine.host.run">
    <description>Run Intuition Engine host helper</description>
    <message>Authentication is required to run the Intuition Engine host helper</message>
    <defaults>
      <allow_any>no</allow_any>
      <allow_inactive>no</allow_inactive>
      <allow_active>auth_self</allow_active>
    </defaults>
    <annotate key="org.freedesktop.policykit.exec.path">/usr/libexec/intuitionengine-host-helper</annotate>
  </action>
</policyconfig>
EOF

    cat > "${WORK_DIR}/49-intuitionengine.rules" <<'EOF'
polkit.addRule(function(action, subject) {
    if (action.id === "org.intuitionengine.host.run" &&
        subject.user === "ie" &&
        subject.active === true &&
        subject.local === true) {
        return polkit.Result.YES;
    }
});
EOF

    cat > "${WORK_DIR}/opt.ie.IntuitionEngine" <<'EOF'
#include <tunables/global>

profile opt.ie.IntuitionEngine /opt/ie/IntuitionEngine flags=(attach_disconnected) {
  #include <abstractions/base>
  #include <abstractions/dbus-session>
  #include <abstractions/fonts>
  #include <abstractions/nameservice>

  capability setgid,
  capability setuid,

  /opt/ie/ r,
  /opt/ie/** r,
  /opt/ie/IntuitionEngine mr,
  /var/ie/ r,
  /var/ie/share/** rwk,
  /var/ie/state/** rwk,

  /dev/dri/** rw,
  /dev/input/** r,
  /dev/snd/** rw,
  /run/dbus/system_bus_socket rw,
  /run/user/[0-9]*/** rw,
  /run/seatd.sock rw,

  /etc/group r,
  /etc/login.defs r,
  /etc/nsswitch.conf r,
  /etc/passwd r,
  /etc/pam.d/** r,
  /etc/polkit-1/** r,
  /etc/security/** r,
  /proc/[0-9]*/stat r,
  /proc/[0-9]*/status r,
  /usr/lib/x86_64-linux-gnu/security/** r,
  /usr/share/polkit-1/** r,

  /usr/bin/pkexec ix,
  /usr/libexec/intuitionengine-host-helper Px,

  dbus send
       bus=system
       path=/org/freedesktop/PolicyKit1/Authority
       interface=org.freedesktop.PolicyKit1.Authority
       peer=(name=org.freedesktop.PolicyKit1),

  deny /etc/shadow r,
  deny /root/** rwklx,
}
EOF

    cat > "${WORK_DIR}/usr.libexec.intuitionengine-host-helper" <<'EOF'
#include <tunables/global>

profile usr.libexec.intuitionengine-host-helper /usr/libexec/intuitionengine-host-helper flags=(attach_disconnected) {
  #include <abstractions/base>
  #include <abstractions/dbus>
  #include <abstractions/nameservice>
  #include <abstractions/ssl_certs>

  /usr/libexec/intuitionengine-host-helper mr,
  /usr/bin/apt-get Cx -> apt,
  /usr/bin/dpkg Cx -> apt,
  /usr/bin/gpg Cx -> apt,
  /usr/bin/gpgv Cx -> apt,
  /usr/bin/nmcli Cx -> nmcli,
  /bin/dash Cx -> apt,
  /bin/sh Cx -> apt,
  /usr/lib/apt/methods/* Cx -> apt,
  /usr/bin/systemctl Cx -> systemctl,
  /bin/gunzip Cx -> apt,
  /usr/bin/lzma Cx -> apt,
  /usr/bin/xz Cx -> apt,
  /usr/bin/zstd Cx -> apt,

  /etc/NetworkManager/** r,
  /etc/apt/** r,
  /etc/dpkg/** r,
  /etc/resolv.conf r,
  /etc/ssl/** r,
  /run/NetworkManager/** rw,
  /run/systemd/** rw,
  /var/cache/apt/** rwk,
  /var/lib/apt/** rwk,
  /var/lib/dpkg/** rwk,
  /var/log/apt/** rwk,

  dbus send
       bus=system
       peer=(name=org.freedesktop.NetworkManager),

  profile apt flags=(attach_disconnected) {
    #include <abstractions/base>
    #include <abstractions/nameservice>
    #include <abstractions/ssl_certs>

    capability chown,
    capability dac_override,
    capability fowner,
    capability fsetid,
    capability setgid,
    capability setuid,

    network inet stream,
    network inet6 stream,

    / r,
    /** r,
    /bin/** mixr,
    /sbin/** mixr,
    /usr/bin/** mixr,
    /usr/sbin/** mixr,
    /usr/lib/** mixr,
    /usr/lib/apt/methods/* mixr,
    /usr/lib/dpkg/** r,
    /usr/lib/x86_64-linux-gnu/** mr,
    /usr/share/** r,
    /boot/** rwkl,
    /etc/** rwkl,
    /lib/** rwkl,
    /lib64/** rwkl,
    /opt/** rwkl,
    /usr/** rwkl,
    /etc/resolv.conf r,
    /etc/ssl/** r,
    /run/** rw,
    /tmp/** rwk,
    /var/backups/** rwk,
    /var/cache/apt/** rwk,
    /var/lib/apt/** rwk,
    /var/lib/dpkg/info/* ixr,
    /var/lib/dpkg/** rwk,
    /var/log/apt/** rwk,
    /var/log/dpkg.log rwk,
    /var/tmp/** rwk,
  }

  profile nmcli flags=(attach_disconnected) {
    #include <abstractions/base>
    #include <abstractions/dbus>
    #include <abstractions/nameservice>

    /usr/bin/nmcli mr,
    /etc/NetworkManager/** r,
    /etc/resolv.conf r,
    /run/dbus/system_bus_socket rw,
    /run/NetworkManager/** rw,
    /usr/share/NetworkManager/** r,

    dbus send
         bus=system
         peer=(name=org.freedesktop.NetworkManager),
  }

  profile systemctl flags=(attach_disconnected) {
    #include <abstractions/base>
    #include <abstractions/dbus>
    #include <abstractions/nameservice>

    capability sys_boot,

    /usr/bin/systemctl mr,
    /run/dbus/system_bus_socket rw,
    /run/systemd/** rw,
    /etc/systemd/** r,
    /usr/lib/systemd/** r,

    dbus send
         bus=system
         peer=(name=org.freedesktop.systemd1),
  }
}
EOF
}

discover_root_device() {
    local image_path="$1"
    local csv root_dev ext4_devs count
    if [[ "$image_path" == "$UBUNTU_CLOUD_IMG_PATH" ]]; then
        csv="$(virt-filesystems -a "${UBUNTU_CLOUD_IMG_PATH}" --filesystems --long --csv)"
    else
        csv="$(virt-filesystems -a "${image_path}" --filesystems --long --csv)"
    fi

    root_dev="$(printf '%s\n' "$csv" | python3 -c '
import csv, sys
r = csv.reader(sys.stdin)
header = next(r)
try:
    i_name, i_type, i_vfs, i_label = header.index("Name"), header.index("Type"), header.index("VFS"), header.index("Label")
except ValueError as e:
    sys.stderr.write(f"virt-filesystems CSV missing expected column: {e}\n")
    sys.exit(2)
labeled = [row[i_name] for row in r if row[i_type] == "filesystem" and row[i_vfs] == "ext4" and row[i_label] == "cloudimg-rootfs"]
if labeled:
    print(labeled[0])
')"

    if [[ -z "$root_dev" ]]; then
        ext4_devs="$(printf '%s\n' "$csv" | python3 -c '
import csv, sys
r = csv.reader(sys.stdin)
header = next(r)
i_name, i_type, i_vfs = header.index("Name"), header.index("Type"), header.index("VFS")
for row in r:
    if row[i_type] == "filesystem" and row[i_vfs] == "ext4":
        print(row[i_name])
')"
        count="$(printf '%s\n' "$ext4_devs" | grep -c . || true)"
        if [[ "$count" -eq 1 ]]; then
            root_dev="$ext4_devs"
            log_warn "Root discovered by single-ext4 fallback: ${root_dev}" >/dev/null
        else
            log_error "Cannot uniquely identify root partition (found ${count} ext4 filesystems, no cloudimg-rootfs label)."
            exit 1
        fi
    fi

    printf '%s\n' "$root_dev"
}

preflight_pristine_cloud_image_only() {
    local expected_source="${WORK_DIR}/${UBUNTU_CLOUD_IMG}"
    if [[ "${EXPANDED_IMG}" != "${WORK_DIR}/"* ]]; then
        log_error "Refusing destructive user deletion: EXPANDED_IMG (${EXPANDED_IMG}) is outside WORK_DIR (${WORK_DIR})."
        exit 1
    fi
    if [[ "${UBUNTU_CLOUD_IMG_PATH:-}" != "${expected_source}" ]]; then
        log_error "Refusing destructive user deletion: source image (${UBUNTU_CLOUD_IMG_PATH:-<unset>}) is not the downloaded cloud image (${expected_source})."
        exit 1
    fi
}

check_golden_image() {
    if [[ "$FORCE_REBUILD_GOLDEN" == "true" ]]; then
        log_warn "Golden image rebuild forced"
        return 1
    fi
    if [[ ! -f "$GOLDEN_IMG_PATH" ]]; then
        log "Golden image not found"
        return 1
    fi
    if [[ ! -f "$GOLDEN_STAMP_PATH" ]]; then
        log_warn "Golden image stamp missing; rebuilding"
        return 1
    fi

    local expected_stamp actual_stamp
    expected_stamp="$(expected_golden_stamp)"
    actual_stamp="$(cat "$GOLDEN_STAMP_PATH")"
    if [[ "$actual_stamp" != "$expected_stamp" ]]; then
        log_warn "Golden image stamp mismatch; rebuilding"
        return 1
    fi

    local age_days
    age_days="$(( ($(date +%s) - $(stat -c %Y "$GOLDEN_IMG_PATH")) / 86400 ))"
    if [[ "$age_days" -gt "$GOLDEN_IMG_MAX_AGE_DAYS" ]]; then
        log_warn "Golden image is ${age_days} days old; rebuilding"
        return 1
    fi
    log_success "Golden image valid (${age_days} days old)"
    return 0
}

expected_golden_stamp() {
    cat <<EOF
version=${GOLDEN_STAMP_VERSION}
ubuntu=${UBUNTU_CLOUD_IMG_URL}
kernel=${KERNEL_PKG}
packages=${ALL_PKGS}
root_part_size=${ROOT_PART_SIZE}
final_image_size=${FINAL_IMAGE_SIZE}
share_grow=ie-grow-share-v1
session=greetd-cage-default-basic-v1
overlayroot=tmpfs
network=${NETWORK_PKGS}
audio=${AUDIO_PKGS}
host_helper=${HOST_HELPER_PKGS}
EOF
}

write_golden_stamp() {
    expected_golden_stamp > "$GOLDEN_STAMP_PATH"
}

create_resized_base() {
    log_section "Creating fixed-size base image"
    ROOT_DEV="$(discover_root_device "$UBUNTU_CLOUD_IMG_PATH")"
    log "Source root partition: ${ROOT_DEV}"

    rm -f "$EXPANDED_IMG"
    qemu-img create -f raw "$EXPANDED_IMG" "$FINAL_IMAGE_SIZE" 2>&1 | tee -a "$LOG_FILE"
    virt-resize --resize "${ROOT_DEV}=${ROOT_PART_SIZE}" --no-extra-partition \
        "$UBUNTU_CLOUD_IMG_PATH" "$EXPANDED_IMG" 2>&1 | tee -a "$LOG_FILE"
}

build_golden_image() {
    log_section "Building golden image"
    preflight_pristine_cloud_image_only
    create_resized_base

    virt-customize -a "$EXPANDED_IMG" \
        --install "${ALL_PKGS}" \
        --run-command 'set -e; dpkg-query -W linux-lowlatency >/dev/null; KIMG=$(find /boot -maxdepth 1 -type f -name "vmlinuz-*" | sort -V | tail -n1); test -n "$KIMG"; sbverify --list "$KIMG" | grep -q "image signature issuer" || (echo "FATAL: installed kernel is not signed; switch KERNEL_PKG to linux-generic"; exit 1)' \
        --run-command 'existing=$(getent passwd 1000 | cut -d: -f1); if [ -n "$existing" ]; then case " ubuntu cloud-user " in *" $existing "*) userdel -r "$existing" || { echo "FATAL: userdel $existing failed"; exit 1; } ;; *) echo "FATAL: UID 1000 occupied by unexpected user $existing; refusing to delete. Use a pristine Ubuntu cloud image."; exit 1 ;; esac; fi' \
        --run-command 'existing_g=$(getent group 1000 | cut -d: -f1); if [ -n "$existing_g" ]; then case " ubuntu cloud-user " in *" $existing_g "*) groupdel "$existing_g" || { echo "FATAL: groupdel $existing_g failed"; exit 1; } ;; *) echo "FATAL: GID 1000 occupied by unexpected group $existing_g"; exit 1 ;; esac; fi' \
        --run-command 'groupadd -g 1000 ie' \
        --run-command 'for g in video audio input render seat tty; do getent group "$g" >/dev/null || groupadd -r "$g"; done' \
        --run-command 'useradd -m -u 1000 -g 1000 -s /bin/bash -G video,audio,input,render,seat,tty ie' \
        --run-command 'passwd -d ie' \
        --mkdir /opt/ie \
        --mkdir /var/ie \
        --mkdir /var/ie/share \
        --mkdir /var/ie/state \
        --upload "${WORK_DIR}/launch.sh:/opt/ie/launch.sh" \
        --run-command 'chmod 0755 /opt/ie /opt/ie/launch.sh' \
        --run-command 'chown root:root /opt/ie /opt/ie/launch.sh' \
        --run-command 'chown 1000:1000 /var/ie /var/ie/share /var/ie/state' \
        --upload "${WORK_DIR}/greetd-config.toml:/etc/greetd/config.toml" \
        --upload "${WORK_DIR}/overlayroot.conf:/etc/overlayroot.conf" \
        --upload "${WORK_DIR}/90-ie-networkmanager.yaml:/etc/netplan/90-ie-networkmanager.yaml" \
        --upload "${WORK_DIR}/ie-grow-share.sh:/usr/local/sbin/ie-grow-share.sh" \
        --upload "${WORK_DIR}/ie-grow-share.service:/etc/systemd/system/ie-grow-share.service" \
        --upload "${WORK_DIR}/ie-firewall.service:/etc/systemd/system/ie-firewall.service" \
        --upload "${WORK_DIR}/ie-apparmor.service:/etc/systemd/system/ie-apparmor.service" \
        --upload "${WORK_DIR}/org.intuitionengine.host.policy:/usr/share/polkit-1/actions/org.intuitionengine.host.policy" \
        --upload "${WORK_DIR}/49-intuitionengine.rules:/etc/polkit-1/rules.d/49-intuitionengine.rules" \
        --upload "${WORK_DIR}/opt.ie.IntuitionEngine:/etc/apparmor.d/opt.ie.IntuitionEngine" \
        --upload "${WORK_DIR}/usr.libexec.intuitionengine-host-helper:/etc/apparmor.d/usr.libexec.intuitionengine-host-helper" \
        --run-command 'chmod +x /usr/local/sbin/ie-grow-share.sh' \
        --run-command 'chmod 0644 /etc/systemd/system/ie-grow-share.service /etc/systemd/system/ie-firewall.service /etc/systemd/system/ie-apparmor.service' \
        --run-command 'chmod 0644 /usr/share/polkit-1/actions/org.intuitionengine.host.policy /etc/polkit-1/rules.d/49-intuitionengine.rules' \
        --run-command 'chmod 0644 /etc/apparmor.d/opt.ie.IntuitionEngine /etc/apparmor.d/usr.libexec.intuitionengine-host-helper' \
        --run-command 'chown root:root /etc/systemd/system/ie-grow-share.service /etc/systemd/system/ie-firewall.service /etc/systemd/system/ie-apparmor.service /usr/share/polkit-1/actions/org.intuitionengine.host.policy /etc/polkit-1/rules.d/49-intuitionengine.rules /etc/apparmor.d/opt.ie.IntuitionEngine /etc/apparmor.d/usr.libexec.intuitionengine-host-helper' \
        --run-command 'systemctl mask getty@tty1.service getty@tty2.service' \
        --run-command 'systemctl enable greetd.service seatd.service' \
        --run-command 'systemctl enable NetworkManager.service' \
        --run-command 'systemctl enable ie-grow-share.service ie-firewall.service ie-apparmor.service' \
        --run-command 'sed -i "s/^GRUB_CMDLINE_LINUX_DEFAULT=.*/GRUB_CMDLINE_LINUX_DEFAULT=\"quiet splash=0 loglevel=0 vt.global_cursor_default=0 fbcon=nodefer video=1920x1080 mitigations=off\"/" /etc/default/grub' \
        --run-command 'sed -i "s/^GRUB_TIMEOUT=.*/GRUB_TIMEOUT=0/" /etc/default/grub' \
        --run-command 'grep -q "^GRUB_RECORDFAIL_TIMEOUT=" /etc/default/grub && sed -i "s/^GRUB_RECORDFAIL_TIMEOUT=.*/GRUB_RECORDFAIL_TIMEOUT=0/" /etc/default/grub || echo "GRUB_RECORDFAIL_TIMEOUT=0" >> /etc/default/grub' \
        --run-command 'mkdir -p /boot/efi/EFI/BOOT' \
        --run-command 'grub-install --target=x86_64-efi --efi-directory=/boot/efi --removable --uefi-secure-boot' \
        --run-command 'cp /usr/lib/shim/shimx64.efi.signed /boot/efi/EFI/BOOT/BOOTX64.EFI' \
        --run-command 'cp /usr/lib/grub/x86_64-efi-signed/grubx64.efi.signed /boot/efi/EFI/BOOT/grubx64.efi' \
        --run-command 'update-grub' \
        --run-command 'systemctl mask cloud-init.service cloud-init-local.service cloud-config.service cloud-final.service apt-daily.service apt-daily.timer apt-daily-upgrade.service apt-daily-upgrade.timer man-db.service man-db.timer motd-news.timer motd-news.service apport.service apport-autoreport.service unattended-upgrades.service systemd-networkd-wait-online.service NetworkManager-wait-online.service' \
        --run-command 'test "$(id -u ie)" = "1000" && test "$(id -g ie)" = "1000"' \
        2>&1 | tee -a "$LOG_FILE"

    mv "$EXPANDED_IMG" "$GOLDEN_IMG_PATH"
    write_golden_stamp
    log_success "Golden image created: $GOLDEN_IMG_PATH"
}

install_ie_binary() {
    log_section "Installing Intuition Engine binary"
    cp "$GOLDEN_IMG_PATH" "$OUTPUT_IMG"
    virt-customize -a "$OUTPUT_IMG" \
        --copy-in "${IE_BINARY}:/opt/ie/" \
        --run-command "mv /opt/ie/$(basename "${IE_BINARY}") /opt/ie/${IE_INSTALL_NAME}" \
        --run-command "chmod 0755 /opt/ie/${IE_INSTALL_NAME}" \
        --run-command "chown root:root /opt/ie /opt/ie/${IE_INSTALL_NAME}" \
        --run-command 'chown -R 1000:1000 /var/ie' \
        2>&1 | tee -a "$LOG_FILE"
}

build_host_helper_binary() {
    log_section "Building HOST helper binary"
    mkdir -p "$WORK_DIR"
    (
        cd "$SCRIPT_DIR"
        CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -pgo=off -ldflags "-s -w" -o "$HOST_HELPER_BINARY" ./cmd/host-helper
    ) 2>&1 | tee -a "$LOG_FILE"
    chmod 0755 "$HOST_HELPER_BINARY"
    if ! file "$HOST_HELPER_BINARY" | grep -q "x86-64"; then
        log_error "HOST helper binary is not x86-64: $HOST_HELPER_BINARY"
        exit 1
    fi
}

install_host_helper_binary() {
    log_section "Installing HOST helper binary"
    virt-customize -a "$OUTPUT_IMG" \
        --mkdir /usr/libexec \
        --copy-in "${HOST_HELPER_BINARY}:/usr/libexec/" \
        --run-command 'chown root:root /usr/libexec/intuitionengine-host-helper' \
        --run-command 'chmod 0755 /usr/libexec/intuitionengine-host-helper' \
        --run-command 'test -x /usr/libexec/intuitionengine-host-helper' \
        2>&1 | tee -a "$LOG_FILE"
}

compute_partition_sectors() {
    local sector_size total_sectors part_info last_end_bytes align_bytes
    sector_size="$(guestfish --ro -a "${OUTPUT_IMG}" run : blockdev-getss /dev/sda | tr -d '[:space:]')"
    [[ "$sector_size" =~ ^[0-9]+$ ]] || { log_error "could not read sector size"; exit 1; }
    log "Sector size: ${sector_size} bytes"
    total_sectors="$(guestfish --ro -a "${OUTPUT_IMG}" run : blockdev-getsz /dev/sda | tr -d '[:space:]')"
    [[ "$total_sectors" =~ ^[0-9]+$ ]] || { log_error "could not read total sector count"; exit 1; }

    part_info="$(guestfish --ro -a "${OUTPUT_IMG}" run : part-list /dev/sda)"
    last_end_bytes="$(printf '%s\n' "$part_info" | python3 -c '
import re, sys
text = sys.stdin.read()
ends = [int(m.group(1)) for m in re.finditer(r"part_end:\s*(\d+)", text)]
if not ends:
    sys.stderr.write("no partitions found\n")
    sys.exit(2)
print(max(ends))
')"

    align_bytes=$((1024 * 1024))
    SHARE_START_B=$(( ((last_end_bytes + 1 + align_bytes - 1) / align_bytes) * align_bytes ))

    local label_val name val
    for label_val in "SHARE_START_B:${SHARE_START_B}"; do
        name="${label_val%%:*}"
        val="${label_val##*:}"
        if (( val % sector_size != 0 )); then
            log_error "Partition offset ${name}=${val} not divisible by sector size ${sector_size}; alignment math broken."
            exit 1
        fi
    done

    SHARE_START=$(( SHARE_START_B / sector_size ))
    SHARE_END=$(( total_sectors - 34 ))
    if (( SHARE_END <= SHARE_START )); then
        log_error "Not enough free space for IESHARE: start=${SHARE_START}, end=${SHARE_END}"
        exit 1
    fi
    export SECTOR_SIZE="$sector_size"
    export SHARE_START SHARE_END SHARE_START_B
}

find_partition_num_by_start() {
    local target_start="$1"
    local part_info
    part_info="$(guestfish --ro -a "${OUTPUT_IMG}" run : part-list /dev/sda)"
    printf '%s\n' "$part_info" | python3 -c '
import re
import sys

target_sector = int(sys.argv[1])
sector_size = int(sys.argv[2])
text = sys.stdin.read()

for entry in re.finditer(r"\{[^}]+\}", text, re.S):
    block = entry.group(0)
    m_num = re.search(r"part_num:\s*(\d+)", block)
    m_start = re.search(r"part_start:\s*(\d+)", block)
    if not m_num or not m_start:
        continue
    start = int(m_start.group(1)) // sector_size
    if start == target_sector:
        print(m_num.group(1))
        sys.exit(0)

sys.stderr.write(f"no partition starts at sector {target_sector}\n")
sys.exit(2)
' "$target_start" "$SECTOR_SIZE"
}

find_partition_size_by_start() {
    local target_start="$1"
    local part_info
    part_info="$(guestfish --ro -a "${OUTPUT_IMG}" run : part-list /dev/sda)"
    printf '%s\n' "$part_info" | python3 -c '
import re
import sys

target_sector = int(sys.argv[1])
sector_size = int(sys.argv[2])
text = sys.stdin.read()

for entry in re.finditer(r"\{[^}]+\}", text, re.S):
    block = entry.group(0)
    m_start = re.search(r"part_start:\s*(\d+)", block)
    m_size = re.search(r"part_size:\s*(\d+)", block)
    if not m_start or not m_size:
        continue
    start = int(m_start.group(1)) // sector_size
    if start == target_sector:
        print(m_size.group(1))
        sys.exit(0)

sys.stderr.write(f"no partition starts at sector {target_sector}\n")
sys.exit(2)
' "$target_start" "$SECTOR_SIZE"
}

discover_appended_partition_devices() {
    if [[ "${CREATE_SHARE}" == "true" ]]; then
        IESHARE_NUM="$(find_partition_num_by_start "$SHARE_START")"
        IESHARE_SIZE_B="$(find_partition_size_by_start "$SHARE_START")"
        IESHARE_DEV="/dev/sda${IESHARE_NUM}"
    else
        IESHARE_NUM=""
        IESHARE_DEV=""
        IESHARE_SIZE_B=""
    fi
    export IESHARE_NUM IESHARE_DEV IESHARE_SIZE_B
    if [[ "${CREATE_SHARE}" == "true" ]]; then
        log "IESHARE device: ${IESHARE_DEV}"
    fi
}

set_share_partition_type() {
    if [[ "${CREATE_SHARE}" != "true" ]]; then
        return 0
    fi

    local part_table_type
    part_table_type="$(guestfish --ro -a "${OUTPUT_IMG}" run : part-get-parttype /dev/sda | tr -d '[:space:]')"
    case "$part_table_type" in
        gpt)
            guestfish -a "${OUTPUT_IMG}" <<GUESTFISH_EOF
run
part-set-gpt-type /dev/sda ${IESHARE_NUM} EBD0A0A2-B9E5-4433-87C0-68B6B72699C7
part-set-name /dev/sda ${IESHARE_NUM} IESHARE
GUESTFISH_EOF
            ;;
        msdos)
            guestfish -a "${OUTPUT_IMG}" <<GUESTFISH_EOF
run
part-set-mbr-id /dev/sda ${IESHARE_NUM} 0x0c
GUESTFISH_EOF
            ;;
        *)
            log_warn "Unknown partition table type ${part_table_type}; leaving IESHARE partition type unchanged"
            ;;
    esac
}

append_partitions() {
    log_section "Appending persistent FAT32 partition"
    compute_partition_sectors

    if [[ "${CREATE_SHARE}" == "true" ]]; then
        guestfish -a "${OUTPUT_IMG}" <<GUESTFISH_EOF
run
part-add /dev/sda p ${SHARE_START} ${SHARE_END}
GUESTFISH_EOF
    fi

    discover_appended_partition_devices
    set_share_partition_type
}

write_fstab() {
    log_section "Writing fstab entries"
    local os_root_dev
    os_root_dev="$(discover_root_device "$OUTPUT_IMG")"
    log "OS root partition: ${os_root_dev}"

    virt-customize -a "$OUTPUT_IMG" \
        --run-command "awk 'BEGIN{OFS=\"\t\"} /^#/ || NF < 4 {print; next} (\$2 == \"/\" || \$2 == \"/boot\") && \$4 !~ /(^|,)relatime(,|$)/ {\$4 = \$4 \",relatime\"} {print}' /etc/fstab > /etc/fstab.ie && mv /etc/fstab.ie /etc/fstab" \
        2>&1 | tee -a "$LOG_FILE"

    if [[ "${CREATE_SHARE}" == "true" ]]; then
        guestfish -a "${OUTPUT_IMG}" <<GUESTFISH_EOF
run
mount ${os_root_dev} /
mkdir-p /var/ie/share
write-append /etc/fstab "LABEL=IESHARE /var/ie/share vfat defaults,relatime,nofail,umask=0022,uid=1000,gid=1000 0 0\n"
umount /
GUESTFISH_EOF
    fi
}

format_share_partition_rootless() {
    if [[ "${CREATE_SHARE}" != "true" ]]; then
        log_warn "Skipping IESHARE FAT32 partition because --no-share was passed"
        return 0
    fi

    log_section "Formatting IESHARE FAT32 partition rootlessly"
    IESHARE_SIZE_B="$(find_partition_size_by_start "$SHARE_START")"
    local fat_img="${WORK_DIR}/ieshare-fat32.img"
    rm -f "$fat_img"
    truncate -s "$IESHARE_SIZE_B" "$fat_img"
    mformat -i "$fat_img" -F -v "${FATSHARE_LABEL}" ::
    stage_share_payload
    local payload_entries=("${SHARE_PAYLOAD_ROOT}"/*)
    mcopy -i "$fat_img" -D A -s "${payload_entries[@]}" ::/
    guestfish -a "${OUTPUT_IMG}" <<GUESTFISH_EOF
run
upload "$fat_img" ${IESHARE_DEV}
GUESTFISH_EOF
}

validate_image() {
    log_section "Validating image layout"
    virt-filesystems --all --long -h -a "${OUTPUT_IMG}" 2>&1 | tee -a "$LOG_FILE"
}

compress_image() {
    log_section "Creating compressed release archive"
    local archive_path="${OUTPUT_IMG%.img}.tar.zst"
    local archive_root="${WORK_DIR}/x64-live-archive"
    rm -rf "$archive_root"
    mkdir -p "$archive_root"
    cp "$OUTPUT_IMG" "$archive_root/$(basename "$OUTPUT_IMG")"
    cp "${SCRIPT_DIR}/README.md" "$archive_root/README.md"
    rm -f "$archive_path"
    tar -C "$archive_root" -I 'zstd --fast=31 -T0' -cf "$archive_path" "$(basename "$OUTPUT_IMG")" README.md 2>&1 | tee -a "$LOG_FILE"
    log_success "Created ${archive_path}"
}

main() {
    if [[ "$PAYLOAD_CHECK_ONLY" == "true" ]]; then
        check_live_payload_inputs
        return 0
    fi
    check_dependencies
    if [[ "${CREATE_SHARE}" == "true" ]]; then
        check_live_payload_inputs
    fi
    mkdir -p "$WORK_DIR"
    if check_golden_image; then
        log "Using cached golden image"
    else
        download_ubuntu
        generate_support_files
        build_golden_image
    fi
    build_host_helper_binary
    install_ie_binary
    install_host_helper_binary
    append_partitions
    write_fstab
    format_share_partition_rootless
    validate_image
    compress_image
    log_success "x64 live image complete: ${OUTPUT_IMG%.img}.tar.zst"
}

main "$@"
