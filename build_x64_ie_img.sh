#!/usr/bin/env bash
#
# Ubuntu x64 Intuition Engine live USB image builder.
# Produces build/x64-live/intuition-engine-x64.img and .zip by default.

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
GOLDEN_STAMP_VERSION="x64-live-golden-v42-audio-session-dracut"
GOLDEN_STAMP_PATH="${GOLDEN_IMG_PATH}.stamp"
KERNEL_PKG="linux-lowlatency"
COMPOSITOR_PKGS="cage,seatd,greetd,xwayland,xwayland-run,libgl1,libegl1,libgles2,libwayland-client0,libxkbcommon0,fonts-dejavu-core,kbd"
X11_RUNTIME_PKGS="libxrandr2,libxxf86vm1,libxi6,libxcursor1,libxinerama1,libx11-6,libxext6,libxfixes3,libxrender1"
AUDIO_PKGS="pipewire,pipewire-pulse,wireplumber,pipewire-alsa,alsa-utils,dbus-user-session"
MEDIA_PKGS="ffmpeg"
SECUREBOOT_PKGS="shim-signed,grub-efi-amd64-signed,sbsigntool"
PLYMOUTH_PKGS="plymouth,plymouth-themes"
SHARE_GROW_PKGS="cloud-guest-utils,dosfstools,fatresize,parted"
NETWORK_PKGS="network-manager,wpasupplicant,wireless-regdb,iw"
HOST_HELPER_PKGS="polkitd,pkexec,ufw,apparmor,apparmor-utils"
ALL_PKGS="${KERNEL_PKG},${COMPOSITOR_PKGS},${X11_RUNTIME_PKGS},${AUDIO_PKGS},${MEDIA_PKGS},${SECUREBOOT_PKGS},${PLYMOUTH_PKGS},${SHARE_GROW_PKGS},${NETWORK_PKGS},${HOST_HELPER_PKGS}"
IE_BINARY="${SCRIPT_DIR}/bin/IntuitionEngine_v3"
IE_INSTALL_NAME="IntuitionEngine"
HOST_HELPER_BINARY="${WORK_DIR}/intuitionengine-host-helper"
ROOT_PART_IMG="${WORK_DIR}/root-partition.ext4"
PLYMOUTH_SPLASH="${SCRIPT_DIR}/splash.png"
REFMAN_PDF_DIR="${SCRIPT_DIR}/sdk/docs/refman.publish/pdf"
SDK_TOOLS_BUILD_DIR="${LIVE_OUT_DIR}/sdk-tools"
SDK_TOOLS_README_TEMPLATE="${SCRIPT_DIR}/sdk/tools/README.md"
SDK_COMPANION_PDFS=(
    "${SCRIPT_DIR}/sdk/docs/IE64_ISA.pdf"
    "${SCRIPT_DIR}/sdk/docs/IE32_ISA.pdf"
    "${SCRIPT_DIR}/sdk/docs/iemon.pdf"
    "${SCRIPT_DIR}/sdk/docs/iescript.pdf"
    "${SCRIPT_DIR}/sdk/docs/architecture.pdf"
)
AROS_RELEASE_DIR="${AROS_RELEASE_DIR:-${SCRIPT_DIR}/build/arosvision-probe/AROS}"
AB3D2_EMBED_DIR="${SCRIPT_DIR}/embedded/ab3d2"
CHOCOLATE_DOOM_DIR="${CHOCOLATE_DOOM_DIR:-${SCRIPT_DIR}/../chocolate-doom}"
IEDOOM_IE86="${IEDOOM_IE86:-build/iedoom.ie86}"
IEDOOM_IE68="${IEDOOM_IE68:-build/iedoom.ie68}"
IEDOOM_WAD="${IEDOOM_WAD:-DOOM1.WAD}"
IEDOOM_IE86_PATH="${CHOCOLATE_DOOM_DIR}/${IEDOOM_IE86}"
IEDOOM_IE68_PATH="${CHOCOLATE_DOOM_DIR}/${IEDOOM_IE68}"
IEDOOM_WAD_PATH="${CHOCOLATE_DOOM_DIR}/${IEDOOM_WAD}"
C64_MUSIC_SOURCE="${C64_MUSIC_SOURCE:-${HOME}/Music/C64Music}"
PROJECTAY_MUSIC_SOURCE="${PROJECTAY_MUSIC_SOURCE:-${HOME}/Music/ProjectAY}"
FINAL_IMAGE_SIZE="${FINAL_IMAGE_SIZE:-10G}"
OUTPUT_IMAGE_SIZE="${OUTPUT_IMAGE_SIZE:-20G}"
ROOT_PART_SIZE="6G"
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

configure_guestfs_environment() {
    local guestfs_tmp_dir="${WORK_DIR}/.tmp"
    local guestfs_cache_dir="${WORK_DIR}/.guestfs-cache"
    local guestfs_runtime_dir="${WORK_DIR}/.runtime"

    mkdir -p "$guestfs_tmp_dir" "$guestfs_cache_dir" "$guestfs_runtime_dir"
    chmod 700 "$guestfs_runtime_dir"

    export TMPDIR="$guestfs_tmp_dir"
    export LIBGUESTFS_TMPDIR="$guestfs_tmp_dir"
    export LIBGUESTFS_CACHEDIR="$guestfs_cache_dir"
    export XDG_RUNTIME_DIR="$guestfs_runtime_dir"
}

cleanup() {
    local status=$?
    if [[ $status -ne 0 ]]; then
        log_warn "Build failed at ${BASH_SOURCE[1]:-${BASH_SOURCE[0]}}:${BASH_LINENO[0]} while running: ${BASH_COMMAND}"
        log_warn "Build failed. Work directory kept for inspection: ${WORK_DIR}"
    fi
}

trap cleanup EXIT

check_dependencies() {
    log_section "Checking dependencies"
    local required_cmds=(aria2c curl virt-customize virt-resize virt-filesystems guestfish qemu-img file python3 go sha256sum /sbin/debugfs)
    if [[ "${CREATE_SHARE}" == "true" ]]; then
        required_cmds+=(mformat mcopy rsync)
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
        log_error "Debian/Ubuntu packages: libguestfs-tools aria2 curl qemu-utils mtools"
        log_error "openSUSE packages: libguestfs guestfs-tools aria2 curl qemu-tools mtools"
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
        log_error "AROS system tree not found: ${AROS_RELEASE_DIR}"
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
    for payload_cmd in file python3 rsync sha256sum; do
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
    payload_require_file "$PLYMOUTH_SPLASH" "restore splash.png" "Plymouth splash image"

    payload_require_file "${AB3D2_EMBED_DIR}/_build.zip" "make x64-live-ab3d2-assets" "AB3D2 embedded runtime asset zip"
    shopt -s nullglob
    local ab3d2_demo_inputs=("${AB3D2_EMBED_DIR}"/ab3d2_*.ie68)
    shopt -u nullglob
    if [[ ${#ab3d2_demo_inputs[@]} -eq 0 ]]; then
        log_error "AB3D2 IE68 demos not found: ${AB3D2_EMBED_DIR}/ab3d2_*.ie68"
        log_error "Producer: make x64-live-ab3d2-assets"
        exit 1
    fi
    payload_require_file "${AROS_RELEASE_DIR}/S/Startup-Sequence" "make arosvision-live-tree" "AROS Startup-Sequence"
    payload_require_file "${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Tool.info" "make arosvision-live-tree" "AROS default tool icon"
    payload_require_file "${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Drawer.info" "make arosvision-live-tree" "AROS default drawer icon"
    payload_require_file "${SCRIPT_DIR}/sdk/examples/prebuilt/iewarp_service.ie64" "make iewarp-runtime-assets" "IEWarp IE64 worker"
    payload_require_file "${AROS_RELEASE_DIR}/Systems/AROS/Libs/iewarp_service.ie64" "make arosvision-live-tree" "AROSVision IEWarp worker copy"
    payload_require_file "${SCRIPT_DIR}/sdk/intuitionos/iexec/iexec.ie64" "make intuitionos" "IntuitionOS IExec kernel"
    payload_require_file "${SCRIPT_DIR}/sdk/intuitionos/system/SYS/IOSSYS/S/Startup-Sequence" "make intuitionos" "IntuitionOS Startup-Sequence"
    payload_require_file "${SCRIPT_DIR}/sdk/intuitionos/system/SYS/IOSSYS/Tools/Shell" "make intuitionos" "IntuitionOS Shell"
    payload_require_file "${SCRIPT_DIR}/sdk/intuitionos/system/SYS/IOSSYS/LIBS/dos.library" "make intuitionos" "IntuitionOS dos.library"
    payload_require_file "${SCRIPT_DIR}/sdk/intuitionos/system/SYS/IOSSYS/L/console.handler" "make intuitionos" "IntuitionOS console.handler"

    payload_require_file "${SCRIPT_DIR}/sdk/examples/basic/rotozoomer_basic.bas" "make sdk-build" "EhBASIC live demo source"
    payload_require_file "${SCRIPT_DIR}/sdk/examples/assets/music/Yummy_Pizza.sid" "make sdk-build" "EhBASIC rotozoomer SID asset"
    payload_require_file "${SCRIPT_DIR}/sdk/examples/assets/rotozoomtexture_ehbasic.raw" "make rotozoom-textures" "EhBASIC rotozoomer texture asset"
    payload_require_file "${SCRIPT_DIR}/sdk/examples/assets/music/enjoythesilence.mid" "make sdk-build" "EhBASIC wobble MIDI asset"
    payload_require_file "${SCRIPT_DIR}/sdk/examples/assets/splash_640x92.rgba" "make sdk-build" "EhBASIC splash image asset"
    payload_require_file "${SCRIPT_DIR}/sdk/examples/assets/music/adagioforstrings.mid" "make sdk-build" "EhBASIC resonance MIDI asset"
    payload_require_file "${SCRIPT_DIR}/sdk/examples/assets/resonance_scroll.rgba" "make sdk-build" "EhBASIC resonance scroll asset"
    payload_require_file "${SCRIPT_DIR}/sdk/examples/prebuilt/rotozoomer_gem.prg" "make gem-rotozoomer" "EmuTOS GEM rotozoomer demo"
    payload_require_file "${SCRIPT_DIR}/sdk/examples/asm/RotoAPI" "make x64-live-aros-demos" "AROS RotoAPI demo"
    payload_require_file "${SCRIPT_DIR}/sdk/examples/asm/RotoHW" "make x64-live-aros-demos" "AROS RotoHW demo"
    payload_require_file "${SCRIPT_DIR}/sdk/examples/c/RotoAPIc" "make x64-live-aros-demos" "AROS RotoAPIc demo"
    payload_require_file "${SCRIPT_DIR}/sdk/examples/c/RotoHWc" "make x64-live-aros-demos" "AROS RotoHWc demo"
    payload_require_file "${SCRIPT_DIR}/sdk/examples/assets/rotozoomtexture_api_c.raw" "make rotozoom-textures" "AROS API C rotozoomer texture"
    payload_require_file "${SCRIPT_DIR}/sdk/examples/assets/rotozoomtexture_hw_c.raw" "make rotozoom-textures" "AROS HW C rotozoomer texture"
    payload_require_file "${IEDOOM_IE86_PATH}" "make iedoom-ie86" "IEDoom x86 guest image"
    payload_require_file "${IEDOOM_IE68_PATH}" "make iedoom-ie68" "IEDoom M68K guest image"
    payload_require_file "${IEDOOM_WAD_PATH}" "copy DOOM1.WAD into CHOCOLATE_DOOM_DIR" "IEDoom shareware WAD"

    payload_require_glob "${SCRIPT_DIR}/sdk/examples/prebuilt/*.ie*" "make sdk-build" "prebuilt Intuition Engine binaries"
    payload_require_glob "${SCRIPT_DIR}/sdk/examples/asm/*.asm" "make sdk-build" "SDK assembly source examples"
    payload_require_glob "${SCRIPT_DIR}/sdk/examples/basic/*.bas" "make sdk-build" "SDK BASIC source examples"
    payload_require_glob "${SCRIPT_DIR}/sdk/examples/c/*.c" "make sdk-build" "SDK C source examples"
    payload_require_glob "${REFMAN_PDF_DIR}/*.pdf" "make x64-live-refman-pdfs" "Programmer's Reference Guide PDFs"
    payload_require_file "${REFMAN_PDF_DIR}/00-Preface.pdf" "make x64-live-refman-pdfs" "Programmer's Reference Guide preface PDF"
    payload_require_file "${REFMAN_PDF_DIR}/39-whole-machine-capstone.pdf" "make x64-live-refman-pdfs" "Programmer's Reference Guide final chapter PDF"
    payload_require_file "${REFMAN_PDF_DIR}/appK-block-diagrams.pdf" "make x64-live-refman-pdfs" "Programmer's Reference Guide final appendix PDF"
    if [[ -e "${REFMAN_PDF_DIR}/README.pdf" || -e "${REFMAN_PDF_DIR}/17-pokey-sap.pdf" ]]; then
        log_error "Stale refman PDF output found under ${REFMAN_PDF_DIR}"
        log_error "Producer: make x64-live-refman-pdfs"
        exit 1
    fi
    local companion_pdf
    for companion_pdf in "${SDK_COMPANION_PDFS[@]}"; do
        payload_require_file "$companion_pdf" "make x64-live-sdk-companion-pdfs" "SDK companion PDF $(basename "$companion_pdf")"
    done

    local include
    for include in ie32.inc ie64.inc ie64_fp.inc ie65.inc ie65.cfg ie65_bindata.cfg ie65_service.cfg ie68.inc ie80.inc ie86.inc; do
        payload_require_file "${SCRIPT_DIR}/sdk/include/${include}" "make sdk-build" "SDK include ${include}"
    done

    payload_require_file "$SDK_TOOLS_README_TEMPLATE" "tracked source file" "SDK tools README template"
    payload_require_file "${SDK_TOOLS_BUILD_DIR}/SHA256SUMS.txt" "make x64-live-sdk-tools" "SDK host tools checksum manifest"
    local sdk_tool_platform sdk_tool_name sdk_tool_ext
    for sdk_tool_platform in linux-x64 linux-arm64 macos-x64 macos-arm64 windows-x64 windows-arm64; do
        if [[ ! -d "${SDK_TOOLS_BUILD_DIR}/${sdk_tool_platform}" ]]; then
            log_error "Required SDK host tools directory missing: ${SDK_TOOLS_BUILD_DIR}/${sdk_tool_platform}"
            log_error "Producer: make x64-live-sdk-tools"
            exit 1
        fi
        sdk_tool_ext=""
        if [[ "$sdk_tool_platform" == windows-* ]]; then
            sdk_tool_ext=".exe"
        fi
        for sdk_tool_name in ie32asm ie64asm ie64dis ie32to64 m68kto64; do
            payload_require_file "${SDK_TOOLS_BUILD_DIR}/${sdk_tool_platform}/${sdk_tool_name}${sdk_tool_ext}" "make x64-live-sdk-tools" "SDK host tool ${sdk_tool_platform}/${sdk_tool_name}${sdk_tool_ext}"
        done
    done
    if ! (cd "$SDK_TOOLS_BUILD_DIR" && sha256sum -c SHA256SUMS.txt); then
        log_error "SDK host tools checksum validation failed: ${SDK_TOOLS_BUILD_DIR}/SHA256SUMS.txt"
        log_error "Producer: make x64-live-sdk-tools"
        exit 1
    fi

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
    if [[ ! -d "${payload_root}/Music" ]]; then
        log_error "Required staged payload directory missing: ${payload_root}/Music"
        log_error "Producer: build_x64_ie_img.sh stage_share_payload"
        exit 1
    fi
    payload_require_file "${payload_root}/Systems/README.TXT" "build_x64_ie_img.sh stage_share_payload" "Systems README"
    payload_require_file "${payload_root}/Docs/IEProgRefGuide/00-Preface.pdf" "make x64-live-refman-pdfs" "staged Programmer's Reference Guide preface"
    payload_require_file "${payload_root}/Docs/IEProgRefGuide/39-whole-machine-capstone.pdf" "make x64-live-refman-pdfs" "staged Programmer's Reference Guide final chapter"
    payload_require_file "${payload_root}/Docs/IEProgRefGuide/appK-block-diagrams.pdf" "make x64-live-refman-pdfs" "staged Programmer's Reference Guide final appendix"
    payload_require_file "${payload_root}/Docs/IE64_ISA.pdf" "make x64-live-sdk-companion-pdfs" "staged IE64 ISA companion PDF"
    payload_require_file "${payload_root}/Docs/IE32_ISA.pdf" "make x64-live-sdk-companion-pdfs" "staged IE32 ISA companion PDF"
    payload_require_file "${payload_root}/Docs/iemon.pdf" "make x64-live-sdk-companion-pdfs" "staged IEMon companion PDF"
    payload_require_file "${payload_root}/Docs/iescript.pdf" "make x64-live-sdk-companion-pdfs" "staged IEScript companion PDF"
    payload_require_file "${payload_root}/Docs/architecture.pdf" "make x64-live-sdk-companion-pdfs" "staged architecture companion PDF"

    payload_require_file "${payload_root}/Systems/AROS/S/Startup-Sequence" "make aros-release-assets" "staged AROS Startup-Sequence"
    payload_require_file "${payload_root}/Systems/AROS/Libs/iewarp_service.ie64" "make iewarp-runtime-assets" "staged AROS IEWarp worker"
    payload_require_file "${payload_root}/Systems/AROS/Demos/RotoAPI" "make x64-live-aros-demos" "staged AROS RotoAPI demo"
    payload_require_file "${payload_root}/Systems/AROS/Demos/RotoHW" "make x64-live-aros-demos" "staged AROS RotoHW demo"
    payload_require_file "${payload_root}/Systems/AROS/Demos/RotoAPIc" "make x64-live-aros-demos" "staged AROS RotoAPIc demo"
    payload_require_file "${payload_root}/Systems/AROS/Demos/RotoHWc" "make x64-live-aros-demos" "staged AROS RotoHWc demo"
    payload_require_file "${payload_root}/Systems/AROS/Demos/rotozoomtexture_api_c.raw" "make rotozoom-textures" "staged AROS API C texture"
    payload_require_file "${payload_root}/Systems/AROS/Demos/rotozoomtexture_hw_c.raw" "make rotozoom-textures" "staged AROS HW C texture"
    payload_require_file "${payload_root}/Systems/EmuTOS/Demos/rotozoomer_gem.prg" "make gem-rotozoomer" "staged EmuTOS GEM demo"
    payload_require_file "${payload_root}/Systems/IntuitionOS/Boot/iexec.ie64" "make intuitionos" "staged IntuitionOS IExec kernel"
    payload_require_file "${payload_root}/Systems/IntuitionOS/IOSSYS/S/Startup-Sequence" "make intuitionos" "staged IntuitionOS Startup-Sequence"
    payload_require_file "${payload_root}/Systems/IntuitionOS/IOSSYS/Tools/Shell" "make intuitionos" "staged IntuitionOS Shell"
    payload_require_file "${payload_root}/Systems/IntuitionOS/IOSSYS/LIBS/dos.library" "make intuitionos" "staged IntuitionOS dos.library"
    payload_require_file "${payload_root}/Systems/IntuitionOS/IOSSYS/L/console.handler" "make intuitionos" "staged IntuitionOS console.handler"
    payload_require_file "${payload_root}/Demos/m68k/ab3d2_ie68_redux_high.ie68" "make x64-live-ab3d2-assets" "staged AB3D2 IE68 demo"
    payload_require_file "${payload_root}/Demos/m68k/iedoom.ie68" "make iedoom-ie68" "staged IEDoom M68K guest image"
    payload_require_file "${payload_root}/Demos/x86/iedoom.ie86" "make iedoom-ie86" "staged IEDoom x86 guest image"
    payload_require_file "${payload_root}/doom1.wad" "copy DOOM1.WAD into CHOCOLATE_DOOM_DIR" "staged IEDoom shareware WAD"
    payload_require_file "${payload_root}/_build/ie_media/redux-high/boot.dat" "make x64-live-ab3d2-assets" "staged AB3D2 runtime media"
    if find "${payload_root}/Demos/m68k" -maxdepth 1 -type f -name 'ab3d2_*.ie68' ! -name '*redux_high*' ! -name '*redux_low*' | grep -q .; then
        payload_require_file "${payload_root}/_build/ie_unpacked/media/includes/test.lnk" "make x64-live-ab3d2-assets" "staged AB3D2 unpacked media"
    fi
    shopt -s nullglob
    local ab3d2_low_demos=("${payload_root}"/Demos/m68k/ab3d2_ie68_redux_low*.ie68)
    shopt -u nullglob
    if [[ ${#ab3d2_low_demos[@]} -gt 0 ]]; then
        payload_require_file "${payload_root}/_build/ie_media/redux-low/boot.dat" "make x64-live-ab3d2-assets" "staged AB3D2 redux-low runtime media"
    fi
    payload_require_file "${payload_root}/SDK/Examples/basic/rotozoomer_basic.bas" "make sdk-build" "staged BASIC example"
    payload_require_file "${payload_root}/SDK/Examples/assets/music/Yummy_Pizza.sid" "make sdk-build" "staged EhBASIC rotozoomer SID asset"
    payload_require_file "${payload_root}/SDK/Examples/assets/rotozoomtexture_ehbasic.raw" "make rotozoom-textures" "staged EhBASIC rotozoomer texture asset"
    payload_require_file "${payload_root}/SDK/Examples/assets/music/enjoythesilence.mid" "make sdk-build" "staged EhBASIC wobble MIDI asset"
    payload_require_file "${payload_root}/SDK/Examples/assets/splash_640x92.rgba" "make sdk-build" "staged EhBASIC splash image asset"
    payload_require_file "${payload_root}/SDK/Examples/assets/music/adagioforstrings.mid" "make sdk-build" "staged EhBASIC resonance MIDI asset"
    payload_require_file "${payload_root}/SDK/Examples/assets/resonance_scroll.rgba" "make sdk-build" "staged EhBASIC resonance scroll asset"
    payload_require_file "${payload_root}/SDK/Include/ie64.inc" "make sdk-build" "staged IE64 include"
    payload_require_file "${payload_root}/SDK/Include/ie64_fp.inc" "make sdk-build" "staged IE64 floating-point include"
    payload_require_file "${payload_root}/SDK/Include/ie65.cfg" "make sdk-build" "staged 6502 linker configuration"
    payload_require_file "${payload_root}/SDK/Tools/README.md" "build_x64_ie_img.sh stage_share_payload" "staged SDK tools README"
    payload_require_file "${payload_root}/SDK/Tools/SHA256SUMS.txt" "make x64-live-sdk-tools" "staged SDK host tools checksum manifest"
    local sdk_tool_platform sdk_tool_name sdk_tool_ext
    for sdk_tool_platform in linux-x64 linux-arm64 macos-x64 macos-arm64 windows-x64 windows-arm64; do
        if [[ ! -d "${payload_root}/SDK/Tools/${sdk_tool_platform}" ]]; then
            log_error "Required staged SDK host tools directory missing: ${payload_root}/SDK/Tools/${sdk_tool_platform}"
            log_error "Producer: build_x64_ie_img.sh stage_share_payload"
            exit 1
        fi
        sdk_tool_ext=""
        if [[ "$sdk_tool_platform" == windows-* ]]; then
            sdk_tool_ext=".exe"
        fi
        for sdk_tool_name in ie32asm ie64asm ie64dis ie32to64 m68kto64; do
            payload_require_file "${payload_root}/SDK/Tools/${sdk_tool_platform}/${sdk_tool_name}${sdk_tool_ext}" "make x64-live-sdk-tools" "staged SDK host tool ${sdk_tool_platform}/${sdk_tool_name}${sdk_tool_ext}"
        done
    done
    if ! (cd "${payload_root}/SDK/Tools" && sha256sum -c SHA256SUMS.txt); then
        log_error "Staged SDK host tools checksum validation failed: ${payload_root}/SDK/Tools/SHA256SUMS.txt"
        log_error "Producer: build_x64_ie_img.sh stage_share_payload"
        exit 1
    fi
    if find "${payload_root}/Docs" -type f -name '*.md' | grep -q .; then
        log_error "Forbidden live payload content: Markdown staged under Docs"
        log_error "Expected: Docs contains PDFs only"
        exit 1
    fi
    if [[ -e "${payload_root}/SDK/README.TXT" || -e "${payload_root}/SDK/README.md" ]]; then
        log_error "Forbidden live payload content: SDK root README staged"
        log_error "Expected: SDK contains include files and source examples only"
        exit 1
    fi
    if find "${payload_root}/SDK" -type f -name '*.md' ! -path "${payload_root}/SDK/Tools/README.md" | grep -q .; then
        log_error "Forbidden live payload content: Markdown staged under SDK"
        log_error "Expected: only SDK/Tools/README.md is staged as Markdown under SDK"
        exit 1
    fi
    if find "${payload_root}/SDK" -type f -name 'SHA256SUMS.txt' ! -path "${payload_root}/SDK/Tools/SHA256SUMS.txt" | grep -q .; then
        log_error "Forbidden live payload content: unexpected SHA256SUMS.txt staged under SDK"
        log_error "Expected: only SDK/Tools/SHA256SUMS.txt is staged as a checksum manifest under SDK"
        exit 1
    fi

    if find "${payload_root}/Demos" -type f -name 'coproc_*.ie*' | grep -q .; then
        log_error "Forbidden live payload location: coproc worker staged under Demos"
        log_error "Producer: build_x64_ie_img.sh stage_share_payload"
        exit 1
    fi
    if find "${payload_root}" -type f \( \
        -name 'coproc_*.ie*' -o \
        -name 'rotozoomer_aros_api.ie68' -o \
        -name 'rotozoomer_aros_hw.ie68' -o \
        -name 'voodoo_smoketest_*.ie*' \
    \) | grep -q .; then
        log_error "Forbidden live payload content: non-user-facing prebuilt .ie* staged under IESHARE"
        log_error "Expected: coprocessor examples, bare-metal AROS duplicate demos, and smoke tests stay out of IESHARE"
        exit 1
    fi
    if find "${payload_root}/_build" -maxdepth 1 -type f \( -name '*.o' -o -name '*.map' -o -name 'diag_symbols_*.lua' \) | grep -q .; then
        log_error "Forbidden live payload content: AB3D2 build intermediates staged in runtime asset root"
        log_error "Producer: build_x64_ie_img.sh stage_share_payload"
        exit 1
    fi
    if find "${payload_root}/Demos" -type f \( -name 'RotoAPI' -o -name 'RotoHW' -o -name 'RotoAPIc' -o -name 'RotoHWc' -o -name 'iewarp_service.ie64' \) | grep -q .; then
        log_error "Forbidden live payload location: AROS-only payload staged under Demos"
        log_error "Producer: build_x64_ie_img.sh stage_share_payload"
        exit 1
    fi
    if [[ -e "${payload_root}/IE/Warp" || -e "${payload_root}/sdk/examples/asm/iewarp_service.ie64" ]]; then
        log_error "Forbidden legacy IEWarp worker path staged in live payload"
        log_error "Expected: Systems/AROS/Libs/iewarp_service.ie64"
        exit 1
    fi
    if find "${payload_root}" -type f \( \
        -name 'ehbasic_ie64.ie64' -o \
        -name 'etos256us.img' -o \
        -name 'emutos.img' -o \
        -name 'aros-ie-m68k.rom' \
    \) | grep -q .; then
        log_error "Forbidden live payload content: embedded boot image staged under IESHARE"
        log_error "Expected: BASIC, EmuTOS, and AROS boot images are embedded in IntuitionEngine"
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
    if [[ -e "${payload_root}/Docs/IEProgRefGuide/README.pdf" ||
          -e "${payload_root}/Docs/IEProgRefGuide/17-pokey-sap.pdf" ]]; then
        log_error "Forbidden stale Programmer's Reference Guide PDF staged in live payload"
        log_error "Expected: Docs/IEProgRefGuide/00-Preface.pdf and 17-pokey.pdf"
        exit 1
    fi
    log_success "Staged IESHARE payload matches the live manifest"
}

stage_share_payload() {
    log_section "Staging IESHARE demo payload"
    local payload_root="${WORK_DIR}/ieshare-payload"
    local demos_dir="${payload_root}/Demos"
    local demos_ie32_dir="${demos_dir}/ie32"
    local demos_ie64_dir="${demos_dir}/ie64"
    local demos_m68k_dir="${demos_dir}/m68k"
    local demos_z80_dir="${demos_dir}/z80"
    local demos_m6502_dir="${demos_dir}/m6502"
    local demos_x86_dir="${demos_dir}/x86"
    local coproc_dir="${payload_root}/IE/Coproc"
    local music_dir="${payload_root}/Music"
    local docs_dir="${payload_root}/Docs"
    local refman_docs_dir="${docs_dir}/IEProgRefGuide"
    local sdk_dir="${payload_root}/SDK"
    local sdk_tools_dir="${sdk_dir}/Tools"
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
embedded_boot_images = {
    "aros-ie-m68k.rom",
    "emutos.img",
    "etos256us.img",
    "ehbasic_ie64.ie64",
}

def choose_name(entries):
    names = [entry.name for entry in entries]
    exact_lower = [name for name in names if name == name.lower()]
    if exact_lower:
        return sorted(exact_lower)[0]
    return sorted(names, key=lambda name: (sum(1 for ch in name if ch.isupper()), name.lower(), name))[0]

def copy_group(entries, dst_dir):
    entries = [entry for entry in entries if entry.name.lower() not in embedded_boot_images]
    if not entries:
        return
    dirs = [entry for entry in entries if entry.is_dir(follow_symlinks=False)]
    files = [entry for entry in entries if entry.is_file(follow_symlinks=False)]
    links = [entry for entry in entries if entry.is_symlink()]
    if links:
        raise SystemExit(f"AROS system tree contains unsupported symlink case collision: {links[0].path}")
    if dirs and files:
        raise SystemExit(f"AROS system tree contains file/directory case collision: {entries[0].path}")
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
    mkdir -p "$demos_dir" "$demos_ie32_dir" "$demos_ie64_dir" \
        "$demos_m68k_dir" "$demos_z80_dir" "$demos_m6502_dir" \
        "$demos_x86_dir" "$coproc_dir" "$music_dir" \
        "$aros_system_dir/Libs" "$aros_demos_dir" "$emutos_demos_dir" \
        "$intuitionos_system_dir/Boot" \
        "$docs_dir" "$refman_docs_dir" "$sdk_dir/Include" "$sdk_dir/Examples/asm" \
        "$sdk_dir/Examples/basic" "$sdk_dir/Examples/c" "$sdk_tools_dir"
    if [[ -d "$C64_MUSIC_SOURCE" ]]; then
        rsync -a --delete "${C64_MUSIC_SOURCE}/" "${music_dir}/C64Music/"
    else
        log_warn "C64 music source not found; leaving Music/C64Music absent: ${C64_MUSIC_SOURCE}"
    fi
    if [[ -d "$PROJECTAY_MUSIC_SOURCE" ]]; then
        rsync -a --delete "${PROJECTAY_MUSIC_SOURCE}/" "${music_dir}/ProjectAY/"
    else
        log_warn "ProjectAY music source not found; leaving Music/ProjectAY absent: ${PROJECTAY_MUSIC_SOURCE}"
    fi
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
    local emutos_demo_files=()
    local iewarp_worker_files=()
    local prebuilt_file
    for prebuilt_file in "${prebuilt_files[@]}"; do
        case "$(basename "$prebuilt_file")" in
            ehbasic_ie64.ie64) ;;
            coproc_*.ie*) ;;
            iewarp_service.ie64) iewarp_worker_files+=("$prebuilt_file") ;;
            rotozoomer_aros_api.ie68|rotozoomer_aros_hw.ie68) ;;
            voodoo_smoketest_*.ie*) ;;
            *.prg) emutos_demo_files+=("$prebuilt_file") ;;
            *.iex|*.ie32) cp -f "$prebuilt_file" "$demos_ie32_dir/" ;;
            *.ie64) cp -f "$prebuilt_file" "$demos_ie64_dir/" ;;
            *.ie68) cp -f "$prebuilt_file" "$demos_m68k_dir/" ;;
            *.ie80) cp -f "$prebuilt_file" "$demos_z80_dir/" ;;
            *.ie65) cp -f "$prebuilt_file" "$demos_m6502_dir/" ;;
            *.ie86) cp -f "$prebuilt_file" "$demos_x86_dir/" ;;
            *)
                log_error "Unsupported prebuilt demo extension: $prebuilt_file"
                exit 1
                ;;
        esac
    done
    if [[ ${#emutos_demo_files[@]} -gt 0 ]]; then
        cp -f "${emutos_demo_files[@]}" "$emutos_demos_dir/"
    fi
    if [[ ${#iewarp_worker_files[@]} -gt 0 ]]; then
        cp -f "${iewarp_worker_files[@]}" "$aros_system_dir/Libs/"
    fi
    cp -f "${IEDOOM_IE86_PATH}" "$demos_x86_dir/iedoom.ie86"
    cp -f "${IEDOOM_IE68_PATH}" "$demos_m68k_dir/iedoom.ie68"
    cp -f "${IEDOOM_WAD_PATH}" "$payload_root/doom1.wad"
    python3 - "${AB3D2_EMBED_DIR}/_build.zip" "${AB3D2_EMBED_DIR}" "$demos_m68k_dir" <<'PY'
import os
import shutil
import sys
import zipfile

zip_path, source_dir, demos_dir = sys.argv[1], sys.argv[2], sys.argv[3]
with zipfile.ZipFile(zip_path) as zf:
    names = zf.namelist()

available_roots = {
    "original": any(name.startswith("ab3d2_source/_build/ie_unpacked/") for name in names),
    "redux-high": any(name.startswith("ab3d2_source/_build/ie_media/redux-high/") for name in names),
    "redux-low": any(name.startswith("ab3d2_source/_build/ie_media/redux-low/") for name in names),
}

def profile_for(filename):
    if "redux_low" in filename:
        return "redux-low"
    if "redux_high" in filename:
        return "redux-high"
    return "original"

copied = 0
for filename in sorted(os.listdir(source_dir)):
    if not (filename.startswith("ab3d2_") and filename.endswith(".ie68")):
        continue
    profile = profile_for(filename)
    if not available_roots[profile]:
        print(f"Skipping AB3D2 {filename}: missing {profile} runtime media in {zip_path}", file=sys.stderr)
        continue
    shutil.copy2(os.path.join(source_dir, filename), os.path.join(demos_dir, filename))
    copied += 1

if copied == 0:
    raise SystemExit(f"no AB3D2 IE68 demos matched available runtime media in {zip_path}")
PY
    cp -f \
        "${SCRIPT_DIR}/sdk/include/ie32.inc" \
        "${SCRIPT_DIR}/sdk/include/ie64.inc" \
        "${SCRIPT_DIR}/sdk/include/ie64_fp.inc" \
        "${SCRIPT_DIR}/sdk/include/ie65.inc" \
        "${SCRIPT_DIR}/sdk/include/ie65.cfg" \
        "${SCRIPT_DIR}/sdk/include/ie65_bindata.cfg" \
        "${SCRIPT_DIR}/sdk/include/ie65_service.cfg" \
        "${SCRIPT_DIR}/sdk/include/ie68.inc" \
        "${SCRIPT_DIR}/sdk/include/ie80.inc" \
        "${SCRIPT_DIR}/sdk/include/ie86.inc" \
        "$sdk_dir/Include/"
    cp -f "${SCRIPT_DIR}"/sdk/examples/asm/*.asm "$sdk_dir/Examples/asm/"
    cp -f "${SCRIPT_DIR}"/sdk/examples/basic/*.bas "$sdk_dir/Examples/basic/"
    cp -f "${SCRIPT_DIR}"/sdk/examples/c/*.c "$sdk_dir/Examples/c/"
    cp -a "${SCRIPT_DIR}/sdk/examples/assets" "$sdk_dir/Examples/"
    cp -a "${SDK_TOOLS_BUILD_DIR}/." "$sdk_tools_dir/"
    cp -f "$SDK_TOOLS_README_TEMPLATE" "$sdk_tools_dir/README.md"
    cp -f "${REFMAN_PDF_DIR}"/*.pdf "$refman_docs_dir/"
    cp -f "${SDK_COMPANION_PDFS[@]}" "$docs_dir/"

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
    cp -f \
        "${SCRIPT_DIR}/sdk/examples/assets/rotozoomtexture_api_c.raw" \
        "${SCRIPT_DIR}/sdk/examples/assets/rotozoomtexture_hw_c.raw" \
        "$aros_demos_dir/"

    python3 - "${AB3D2_EMBED_DIR}/_build.zip" "$payload_root" <<'PY'
import hashlib
import os
import sys
import zipfile

zip_path, dest = sys.argv[1], sys.argv[2]
dest_real = os.path.realpath(dest)
runtime_roots = (
    "ab3d2_source/_build/ie_media/redux-high/",
    "ab3d2_source/_build/ie_media/redux-low/",
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
        fat_name = name.removeprefix("ab3d2_source/").lower()
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
It contains runnable Intuition Engine demos grouped by guest CPU:

ie32
ie64
m68k
z80
m6502
x86

AB3D2 IE68 demos live under m68k.

OS-specific payloads live under Systems:

Systems/AROS/Demos
Systems/EmuTOS/Demos
Systems/IntuitionOS
EOF

    cat > "${payload_root}/README.TXT" <<'EOF'
Intuition Engine Live USB share

Top-level folders:

Demos    Bare-metal Intuition Engine demos.
IE       Intuition Engine runtime support files.
Music    Music collections copied from the build host when available.
Docs     Printable Programmer's Reference Guide PDFs.
SDK      Reference include files, source examples, and host SDK tools.
Systems  Guest OS payloads.
_build   AB3D2 runtime assets used by the AB3D2 IE68 demos.

AROS files live under Systems/AROS.
EmuTOS/GEMDOS demo files live under Systems/EmuTOS.
IntuitionOS SYS: lives under Systems/IntuitionOS.
EOF

    cat > "${systems_dir}/README.TXT" <<'EOF'
Guest OS payloads

Systems/AROS is the AROS SYS: root used by the live image.
Systems/AROS/Demos contains AROS-native demo programs.
Systems/AROS/Libs contains private AROS resources such as iewarp_service.ie64.

Systems/EmuTOS is the EmuTOS GEMDOS drive root used by the live image.
Systems/EmuTOS/Demos contains GEMDOS demo programs.

Systems/IntuitionOS is the IntuitionOS SYS: root used by the live image.
Systems/IntuitionOS/Boot/iexec.ie64 is the host bootstrap kernel image.
Systems/IntuitionOS/IOSSYS is the read-only system subtree visible inside
IntuitionOS as IOSSYS: through SYS:IOSSYS.
EOF

    cat > "${coproc_dir}/README.TXT" <<'EOF'
Intuition Engine coprocessor support images

No coprocessor service images are staged here by default. Coprocessor caller
and service examples are included as source under SDK/Examples/asm.
EOF

    find "$demos_dir" -maxdepth 2 -type f | sort | sed "s#^${payload_root}/#  #" | tee -a "$LOG_FILE"
    find "$systems_dir" -maxdepth 3 -type f | sort | sed "s#^${payload_root}/#  #" | tee -a "$LOG_FILE"
    find "$coproc_dir" -maxdepth 1 -type f | sort | sed "s#^${payload_root}/#  #" | tee -a "$LOG_FILE"
    find "${payload_root}/Docs" -maxdepth 3 -type f | sort | sed "s#^${payload_root}/#  #" | tee -a "$LOG_FILE"
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
    set -- -ehbasic-host -ehbasic-host-appliance
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
    set -- -ehbasic-host -ehbasic-host-appliance
fi
if [ -z "${DBUS_SESSION_BUS_ADDRESS:-}" ] && [ -z "${IE_LIVE_DBUS_SESSION:-}" ] && command -v dbus-run-session >/dev/null 2>&1; then
    export IE_LIVE_IMAGE=1
    export IE_LIVE_DBUS_SESSION=1
    exec dbus-run-session -- "$0" "$@"
fi
export IE_LIVE_IMAGE=1
if [ -z "${XDG_RUNTIME_DIR:-}" ] || [ ! -d "$XDG_RUNTIME_DIR" ]; then
    if [ -d "/run/user/$(id -u)" ]; then
        export XDG_RUNTIME_DIR="/run/user/$(id -u)"
    else
        export XDG_RUNTIME_DIR="/tmp/ie-runtime-$(id -u)"
    fi
fi
export PIPEWIRE_RUNTIME_DIR="$XDG_RUNTIME_DIR"
export PULSE_RUNTIME_PATH="$XDG_RUNTIME_DIR/pulse"
export PULSE_SERVER="unix:${XDG_RUNTIME_DIR}/pulse/native"
mkdir -p "$XDG_RUNTIME_DIR" "$XDG_RUNTIME_DIR/pulse" /var/ie/state
chmod 700 "$XDG_RUNTIME_DIR"
audio_log=/tmp/ie-pipewire-ready.log
ie_log=/var/ie/state/intuition-engine.log
: > "$audio_log"
: > "$ie_log"
chmod 0644 "$audio_log" "$ie_log" 2>/dev/null || true
{
    echo "XDG_RUNTIME_DIR=${XDG_RUNTIME_DIR}"
    echo "PULSE_SERVER=${PULSE_SERVER}"
    if [ -n "${DBUS_SESSION_BUS_ADDRESS:-}" ]; then
        echo "DBUS_SESSION_BUS_ADDRESS is set"
    else
        echo "DBUS_SESSION_BUS_ADDRESS is not set"
    fi
} >>"$audio_log" 2>&1

if command -v systemctl >/dev/null 2>&1; then
    systemctl --user start pipewire.service wireplumber.service pipewire-pulse.service >>"$audio_log" 2>&1 || true
fi

if [ ! -S "${XDG_RUNTIME_DIR}/pipewire-0" ]; then
    pipewire >/tmp/ie-pipewire.log 2>&1 &
else
    echo "pipewire socket already present at ${XDG_RUNTIME_DIR}/pipewire-0" >>"$audio_log"
fi
for _ in $(seq 1 100); do
    [ -S "${XDG_RUNTIME_DIR}/pipewire-0" ] && break
    sleep 0.05
done
if ! command -v systemctl >/dev/null 2>&1 || ! systemctl --user is-active --quiet wireplumber.service; then
    wireplumber >/tmp/ie-wireplumber.log 2>&1 &
else
    echo "wireplumber.service already active" >>"$audio_log"
fi
if [ ! -S "${XDG_RUNTIME_DIR}/pulse/native" ]; then
    pipewire-pulse >/tmp/ie-pipewire-pulse.log 2>&1 &
else
    echo "pipewire-pulse socket already present at ${XDG_RUNTIME_DIR}/pulse/native" >>"$audio_log"
fi
for _ in $(seq 1 100); do
    [ -S "${XDG_RUNTIME_DIR}/pulse/native" ] && break
    sleep 0.05
done
if [ ! -S "${XDG_RUNTIME_DIR}/pulse/native" ]; then
    {
        echo "pipewire-pulse did not become ready at ${XDG_RUNTIME_DIR}/pulse/native"
        echo "PULSE_SERVER=${PULSE_SERVER}"
        echo "--- pipewire-pulse log ---"
        cat /tmp/ie-pipewire-pulse.log 2>/dev/null || true
        echo "--- pipewire log ---"
        cat /tmp/ie-pipewire.log 2>/dev/null || true
    } >>"$audio_log"
else
    echo "pipewire-pulse socket ready at ${XDG_RUNTIME_DIR}/pulse/native" >>"$audio_log"
fi
{
    command -v aplay >/dev/null 2>&1 && aplay -l || true
    if command -v wpctl >/dev/null 2>&1; then
        wpctl status || true
        wpctl set-mute @DEFAULT_AUDIO_SINK@ 0 || true
        wpctl set-volume @DEFAULT_AUDIO_SINK@ 0.90 || true
    fi
} >>"$audio_log" 2>&1
{
    echo "launching IntuitionEngine at $(date -Is)"
    echo "args: $*"
    echo "PULSE_SERVER=${PULSE_SERVER}"
    echo "audio readiness log: ${audio_log}"
} >>"$ie_log"
exec /opt/ie/IntuitionEngine "$@" >>"$ie_log" 2>&1
EOF
    chmod +x "${WORK_DIR}/launch.sh"

    cat > "${WORK_DIR}/session.sh" <<'EOF'
#!/bin/sh
exec cage -s -- xwayland-run -- /opt/ie/launch.sh
EOF
    chmod +x "${WORK_DIR}/session.sh"

    cat > "${WORK_DIR}/greetd-config.toml" <<'EOF'
[terminal]
vt = 1

[default_session]
command = "/opt/ie/session.sh"
user = "ie"
EOF

    cat > "${WORK_DIR}/ie-logind.conf" <<'EOF'
[Login]
NAutoVTs=0
ReserveVT=0
EOF

    cat > "${WORK_DIR}/ie-no-vt-switch.map" <<'EOF'
# Disable virtual terminal switching chords in the live appliance.
alt keycode  59 = VoidSymbol
alt keycode  60 = VoidSymbol
alt keycode  61 = VoidSymbol
alt keycode  62 = VoidSymbol
alt keycode  63 = VoidSymbol
alt keycode  64 = VoidSymbol
alt keycode  65 = VoidSymbol
alt keycode  66 = VoidSymbol
alt keycode  67 = VoidSymbol
alt keycode  68 = VoidSymbol
alt keycode  87 = VoidSymbol
alt keycode  88 = VoidSymbol
control alt keycode  59 = VoidSymbol
control alt keycode  60 = VoidSymbol
control alt keycode  61 = VoidSymbol
control alt keycode  62 = VoidSymbol
control alt keycode  63 = VoidSymbol
control alt keycode  64 = VoidSymbol
control alt keycode  65 = VoidSymbol
control alt keycode  66 = VoidSymbol
control alt keycode  67 = VoidSymbol
control alt keycode  68 = VoidSymbol
control alt keycode  87 = VoidSymbol
control alt keycode  88 = VoidSymbol
alt keycode 105 = VoidSymbol
alt keycode 106 = VoidSymbol
EOF

    cat > "${WORK_DIR}/90-ie-networkmanager.yaml" <<'EOF'
network:
  version: 2
  renderer: NetworkManager
EOF

    cat > "${WORK_DIR}/intuition-engine.plymouth" <<'EOF'
[Plymouth Theme]
Name=Intuition Engine
Description=Intuition Engine live boot splash
ModuleName=script

[script]
ImageDir=/usr/share/plymouth/themes/intuition-engine
ScriptFile=/usr/share/plymouth/themes/intuition-engine/intuition-engine.script
EOF

    cat > "${WORK_DIR}/intuition-engine.script" <<'EOF'
Window.SetBackgroundTopColor(0, 0, 0);
Window.SetBackgroundBottomColor(0, 0, 0);

logo = Image("splash.png");
logo_sprite = Sprite(logo);

fun refresh_callback() {
    logo_sprite.SetX(Window.GetWidth() / 2 - logo.GetWidth() / 2);
    logo_sprite.SetY(Window.GetHeight() / 2 - logo.GetHeight() / 2);
}

Plymouth.SetRefreshFunction(refresh_callback);
EOF

    cat > "${WORK_DIR}/zz-intuition-engine-grub.cfg" <<'EOF'
GRUB_CMDLINE_LINUX_DEFAULT="quiet splash loglevel=0 vt.global_cursor_default=0 fbcon=nodefer video=1920x1080 rd.driver.export=0 mitigations=off"
GRUB_CMDLINE_LINUX=""
GRUB_TIMEOUT_STYLE=hidden
GRUB_TIMEOUT=0
GRUB_RECORDFAIL_TIMEOUT=0
unset GRUB_TERMINAL
EOF

    cat > "${WORK_DIR}/ie-grow-share.sh" <<'EOF'
#!/bin/sh
set -eu

log() {
    printf '%s\n' "ie-grow-share: $*"
}

udevadm settle --timeout=10 2>/dev/null || true

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
After=systemd-udevd.service
Before=local-fs-pre.target

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/ie-grow-share.sh

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

[Install]
WantedBy=multi-user.target
WantedBy=network-pre.target
EOF

    cat > "${WORK_DIR}/ie-no-vt-switch.service" <<'EOF'
[Unit]
Description=Disable Intuition Engine live image virtual terminal switch keys
DefaultDependencies=no
After=local-fs.target systemd-vconsole-setup.service
Before=greetd.service

[Service]
Type=oneshot
ExecStart=/usr/bin/loadkeys /usr/local/share/kbd/keymaps/ie-no-vt-switch.map

[Install]
WantedBy=multi-user.target
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

[Install]
WantedBy=multi-user.target
EOF

    cat > "${WORK_DIR}/ie-host-helper.service" <<'EOF'
[Unit]
Description=Intuition Engine HOST helper broker
After=ie-apparmor.service dbus.service NetworkManager.service
Requires=ie-apparmor.service
Before=greetd.service

[Service]
Type=simple
ExecStart=/usr/libexec/intuitionengine-host-helper serve
Restart=on-failure
RestartSec=1

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
        subject.user === "ie") {
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

  network unix stream,

  /opt/ie/ r,
  /opt/ie/** r,
  /opt/ie/IntuitionEngine mr,
  /var/ie/ r,
  /var/ie/share/ rw,
  /var/ie/share/** rwk,
  /var/ie/state/ rw,
  /var/ie/state/** rwk,

  /dev/dri/** rw,
  /dev/input/** r,
  /dev/shm/** rwk,
  /dev/snd/** rw,
  /tmp/.X11-unix/** rw,
  /tmp/.X[0-9]*-lock r,
  /tmp/dbus-* rw,
  /tmp/tmp*/ rw,
  /tmp/tmp*/** rwk,
  /tmp/xwayland-run*/** rwk,
  /tmp/ie-runtime-[0-9]*/** rw,
  /tmp/ie-*.log rw,
  /run/dbus/system_bus_socket rw,
  /run/intuitionengine-host-helper.sock rw,
  /run/user/[0-9]*/** rw,
  /run/seatd.sock rw,
  /sys/class/drm/** r,
  /sys/devices/** r,

  /etc/group r,
  /etc/login.defs r,
  /etc/nsswitch.conf r,
  /etc/passwd r,
  /etc/pam.d/** r,
  /etc/polkit-1/** r,
  /etc/security/** r,
  /etc/shells r,
  /home/ r,
  /home/ie/ r,
  /home/ie/.config/ rw,
  /home/ie/.config/pulse/ rw,
  /home/ie/.config/pulse/** rwk,
  /proc/[0-9]*/cgroup r,
  /proc/[0-9]*/stat r,
  /proc/[0-9]*/status r,
  /proc/sys/net/core/somaxconn r,
  /usr/lib/x86_64-linux-gnu/security/** r,
  /usr/lib/x86_64-linux-gnu/dri/** mr,
  /usr/share/drirc.d/** r,
  /usr/share/glvnd/** r,
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

  capability chown,
  capability fowner,

  network unix stream,

  /usr/libexec/intuitionengine-host-helper mr,
  /usr/bin/apt-get Ux,
  /usr/bin/systemctl Cx -> systemctl,

  /etc/NetworkManager/** r,
  /etc/apt/** r,
  /etc/dpkg/** r,
  /etc/resolv.conf r,
  /etc/ssl/** r,
  /run/dbus/system_bus_socket rw,
  /run/intuitionengine-host-helper.sock rw,
  /run/NetworkManager/** rw,
  /run/systemd/** rw,
  /var/cache/apt/** rwk,
  /var/lib/apt/** rwk,
  /var/lib/dpkg/** rwk,
  /var/log/apt/** rwk,

  dbus send
       bus=system
       peer=(name=org.freedesktop.NetworkManager),

  profile systemctl flags=(attach_disconnected) {
    #include <abstractions/base>
    #include <abstractions/dbus>
    #include <abstractions/nameservice>

    capability sys_boot,

    network unix stream,

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
    local plymouth_splash_sha256
    plymouth_splash_sha256="$(sha256sum "$PLYMOUTH_SPLASH")"
    plymouth_splash_sha256="${plymouth_splash_sha256%% *}"

    cat <<EOF
version=${GOLDEN_STAMP_VERSION}
ubuntu=${UBUNTU_CLOUD_IMG_URL}
kernel=${KERNEL_PKG}
packages=${ALL_PKGS}
root_part_size=${ROOT_PART_SIZE}
final_image_size=${FINAL_IMAGE_SIZE}
share_grow=ie-grow-share-v1
session=greetd-cage-default-basic-v3-systemd-user-audio
persistent_root=ext4
network=${NETWORK_PKGS}
audio=${AUDIO_PKGS}
media=${MEDIA_PKGS}
plymouth=${PLYMOUTH_PKGS}
plymouth_splash=splash.png
plymouth_splash_sha256=${plymouth_splash_sha256}
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
        --run-command 'set -e; export DEBIAN_FRONTEND=noninteractive; apt-get update; apt-get purge -y overlayroot || true; apt-get upgrade -y -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold; apt-get purge -y overlayroot || true; apt-get autoremove -y; apt-get clean' \
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
        --upload "${WORK_DIR}/session.sh:/opt/ie/session.sh" \
        --run-command 'chmod 0755 /opt/ie /opt/ie/launch.sh /opt/ie/session.sh' \
        --run-command 'chown root:root /opt/ie /opt/ie/launch.sh /opt/ie/session.sh' \
        --run-command 'chown 1000:1000 /var/ie /var/ie/share /var/ie/state' \
        --upload "${WORK_DIR}/greetd-config.toml:/etc/greetd/config.toml" \
        --mkdir /etc/systemd/logind.conf.d \
        --upload "${WORK_DIR}/ie-logind.conf:/etc/systemd/logind.conf.d/10-ie-live.conf" \
        --mkdir /usr/local/share/kbd \
        --mkdir /usr/local/share/kbd/keymaps \
        --upload "${WORK_DIR}/ie-no-vt-switch.map:/usr/local/share/kbd/keymaps/ie-no-vt-switch.map" \
        --mkdir /usr/share/plymouth/themes/intuition-engine \
        --upload "${WORK_DIR}/intuition-engine.plymouth:/usr/share/plymouth/themes/intuition-engine/intuition-engine.plymouth" \
        --upload "${WORK_DIR}/intuition-engine.script:/usr/share/plymouth/themes/intuition-engine/intuition-engine.script" \
        --upload "${PLYMOUTH_SPLASH}:/usr/share/plymouth/themes/intuition-engine/splash.png" \
        --mkdir /etc/default/grub.d \
        --upload "${WORK_DIR}/zz-intuition-engine-grub.cfg:/etc/default/grub.d/zz-intuition-engine.cfg" \
        --upload "${WORK_DIR}/90-ie-networkmanager.yaml:/etc/netplan/90-ie-networkmanager.yaml" \
        --upload "${WORK_DIR}/ie-grow-share.sh:/usr/local/sbin/ie-grow-share.sh" \
        --upload "${WORK_DIR}/ie-grow-share.service:/etc/systemd/system/ie-grow-share.service" \
        --upload "${WORK_DIR}/ie-firewall.service:/etc/systemd/system/ie-firewall.service" \
        --upload "${WORK_DIR}/ie-no-vt-switch.service:/etc/systemd/system/ie-no-vt-switch.service" \
        --upload "${WORK_DIR}/ie-apparmor.service:/etc/systemd/system/ie-apparmor.service" \
        --upload "${WORK_DIR}/ie-host-helper.service:/etc/systemd/system/ie-host-helper.service" \
        --upload "${WORK_DIR}/org.intuitionengine.host.policy:/usr/share/polkit-1/actions/org.intuitionengine.host.policy" \
        --upload "${WORK_DIR}/49-intuitionengine.rules:/etc/polkit-1/rules.d/49-intuitionengine.rules" \
        --upload "${WORK_DIR}/opt.ie.IntuitionEngine:/etc/apparmor.d/opt.ie.IntuitionEngine" \
        --upload "${WORK_DIR}/usr.libexec.intuitionengine-host-helper:/etc/apparmor.d/usr.libexec.intuitionengine-host-helper" \
        --run-command 'chmod +x /usr/local/sbin/ie-grow-share.sh' \
        --run-command 'chmod 0644 /etc/systemd/logind.conf.d/10-ie-live.conf /etc/default/grub.d/zz-intuition-engine.cfg /usr/local/share/kbd/keymaps/ie-no-vt-switch.map /usr/share/plymouth/themes/intuition-engine/intuition-engine.plymouth /usr/share/plymouth/themes/intuition-engine/intuition-engine.script /usr/share/plymouth/themes/intuition-engine/splash.png' \
        --run-command 'chmod 0600 /etc/netplan/90-ie-networkmanager.yaml' \
        --run-command 'chmod 0644 /etc/systemd/system/ie-grow-share.service /etc/systemd/system/ie-firewall.service /etc/systemd/system/ie-no-vt-switch.service /etc/systemd/system/ie-apparmor.service /etc/systemd/system/ie-host-helper.service' \
        --run-command 'chmod 0644 /usr/share/polkit-1/actions/org.intuitionengine.host.policy /etc/polkit-1/rules.d/49-intuitionengine.rules' \
        --run-command 'chmod 0644 /etc/apparmor.d/opt.ie.IntuitionEngine /etc/apparmor.d/usr.libexec.intuitionengine-host-helper' \
        --run-command 'chown root:root /etc/systemd/logind.conf.d/10-ie-live.conf /etc/default/grub.d/zz-intuition-engine.cfg /etc/netplan/90-ie-networkmanager.yaml /usr/local/share/kbd/keymaps/ie-no-vt-switch.map /usr/share/plymouth/themes/intuition-engine /usr/share/plymouth/themes/intuition-engine/intuition-engine.plymouth /usr/share/plymouth/themes/intuition-engine/intuition-engine.script /usr/share/plymouth/themes/intuition-engine/splash.png /etc/systemd/system/ie-grow-share.service /etc/systemd/system/ie-firewall.service /etc/systemd/system/ie-no-vt-switch.service /etc/systemd/system/ie-apparmor.service /etc/systemd/system/ie-host-helper.service /usr/share/polkit-1/actions/org.intuitionengine.host.policy /etc/polkit-1/rules.d/49-intuitionengine.rules /etc/apparmor.d/opt.ie.IntuitionEngine /etc/apparmor.d/usr.libexec.intuitionengine-host-helper' \
        --run-command 'for n in $(seq 1 12); do systemctl mask "getty@tty${n}.service"; done' \
        --run-command 'systemctl enable greetd.service seatd.service' \
        --run-command 'systemctl enable NetworkManager.service' \
        --run-command 'systemctl enable ie-grow-share.service ie-firewall.service ie-no-vt-switch.service ie-apparmor.service ie-host-helper.service' \
        --run-command 'update-alternatives --install /usr/share/plymouth/themes/default.plymouth default.plymouth /usr/share/plymouth/themes/intuition-engine/intuition-engine.plymouth 100' \
        --run-command 'update-alternatives --set default.plymouth /usr/share/plymouth/themes/intuition-engine/intuition-engine.plymouth' \
        --run-command 'update-initramfs -u -k all' \
        --run-command 'grep -q "^GRUB_CMDLINE_LINUX_DEFAULT=" /etc/default/grub && sed -i "s/^GRUB_CMDLINE_LINUX_DEFAULT=.*/GRUB_CMDLINE_LINUX_DEFAULT=\"quiet splash loglevel=0 vt.global_cursor_default=0 fbcon=nodefer video=1920x1080 rd.driver.export=0 mitigations=off\"/" /etc/default/grub || echo "GRUB_CMDLINE_LINUX_DEFAULT=\"quiet splash loglevel=0 vt.global_cursor_default=0 fbcon=nodefer video=1920x1080 rd.driver.export=0 mitigations=off\"" >> /etc/default/grub' \
        --run-command 'grep -q "^GRUB_CMDLINE_LINUX=" /etc/default/grub && sed -i "s/^GRUB_CMDLINE_LINUX=.*/GRUB_CMDLINE_LINUX=\"\"/" /etc/default/grub || echo "GRUB_CMDLINE_LINUX=\"\"" >> /etc/default/grub' \
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
    qemu-img resize -f raw "$OUTPUT_IMG" "$OUTPUT_IMAGE_SIZE" 2>&1 | tee -a "$LOG_FILE"
    extract_root_partition_image
    local commands_file="${WORK_DIR}/debugfs-install-ie.cmds"
    cat > "$commands_file" <<EOF
mkdir /opt
mkdir /opt/ie
mkdir /var
mkdir /var/ie
mkdir /var/ie/share
mkdir /var/ie/state
rm /opt/ie/${IE_INSTALL_NAME}
write ${IE_BINARY} /opt/ie/${IE_INSTALL_NAME}
sif /opt/ie/${IE_INSTALL_NAME} mode 0100755
sif /opt/ie/${IE_INSTALL_NAME} uid 0
sif /opt/ie/${IE_INSTALL_NAME} gid 0
sif /opt/ie mode 040755
sif /opt/ie uid 0
sif /opt/ie gid 0
sif /var/ie uid 1000
sif /var/ie gid 1000
sif /var/ie/share uid 1000
sif /var/ie/share gid 1000
sif /var/ie/state uid 1000
sif /var/ie/state gid 1000
EOF
    debugfs_apply "$commands_file"
}

build_host_helper_binary() {
    log_section "Building HOST helper binary"
    mkdir -p "$WORK_DIR"
    (
        cd "$SCRIPT_DIR"
        GOAMD64=v3 CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -pgo=off -ldflags "-s -w" -o "$HOST_HELPER_BINARY" ./cmd/host-helper
    ) 2>&1 | tee -a "$LOG_FILE"
    chmod 0755 "$HOST_HELPER_BINARY"
    if ! file "$HOST_HELPER_BINARY" | grep -q "x86-64"; then
        log_error "HOST helper binary is not x86-64: $HOST_HELPER_BINARY"
        exit 1
    fi
}

discover_root_partition_geometry() {
    eval "$(python3 - "$OUTPUT_IMG" <<'PY'
import struct
import sys

image_path = sys.argv[1]
sector_size = 512
with open(image_path, "rb") as f:
    f.seek(sector_size)
    header = f.read(sector_size)
    if header[:8] != b"EFI PART":
        raise SystemExit(f"{image_path}: missing GPT header")
    entries_lba = struct.unpack_from("<Q", header, 72)[0]
    entry_count = struct.unpack_from("<I", header, 80)[0]
    entry_size = struct.unpack_from("<I", header, 84)[0]
    f.seek(entries_lba * sector_size)
    for index in range(entry_count):
        entry = f.read(entry_size)
        if entry[:16] == b"\x00" * 16:
            continue
        first_lba, last_lba = struct.unpack_from("<QQ", entry, 32)
        name = entry[56:128].decode("utf-16le", errors="ignore").rstrip("\x00")
        if name == "cloudimg-rootfs":
            print(f"ROOT_PART_NUM={index + 1}")
            print(f"ROOT_START_B={first_lba * sector_size}")
            print(f"ROOT_SIZE_B={(last_lba - first_lba + 1) * sector_size}")
            raise SystemExit(0)
raise SystemExit(f"{image_path}: cloudimg-rootfs partition not found")
PY
)"
    export ROOT_PART_NUM ROOT_START_B ROOT_SIZE_B
}

extract_root_partition_image() {
    discover_root_partition_geometry
    log "Root partition: /dev/sda${ROOT_PART_NUM}, start=${ROOT_START_B}, size=${ROOT_SIZE_B}"
    rm -f "$ROOT_PART_IMG"
    dd if="$OUTPUT_IMG" of="$ROOT_PART_IMG" bs=4M \
        iflag=skip_bytes,count_bytes skip="$ROOT_START_B" count="$ROOT_SIZE_B" \
        status=progress 2>&1 | tee -a "$LOG_FILE"
}

flush_root_partition_image() {
    if [[ ! -f "$ROOT_PART_IMG" ]]; then
        log_error "Root partition work image missing: $ROOT_PART_IMG"
        exit 1
    fi
    log_section "Writing root partition updates"
    dd if="$ROOT_PART_IMG" of="$OUTPUT_IMG" bs=4M \
        oflag=seek_bytes seek="$ROOT_START_B" conv=notrunc \
        status=progress 2>&1 | tee -a "$LOG_FILE"
}

debugfs_apply() {
    local commands_file="$1"
    /sbin/debugfs -w -f "$commands_file" "$ROOT_PART_IMG" 2>&1 | tee -a "$LOG_FILE"
}

install_host_helper_binary() {
    log_section "Installing HOST helper binary"
    local commands_file="${WORK_DIR}/debugfs-install-host-helper.cmds"
    cat > "$commands_file" <<EOF
mkdir /usr/libexec
rm /usr/libexec/intuitionengine-host-helper
write ${HOST_HELPER_BINARY} /usr/libexec/intuitionengine-host-helper
sif /usr/libexec/intuitionengine-host-helper mode 0100755
sif /usr/libexec/intuitionengine-host-helper uid 0
sif /usr/libexec/intuitionengine-host-helper gid 0
EOF
    debugfs_apply "$commands_file"
}

append_partitions() {
    log_section "Appending persistent FAT32 partition"
    if [[ "${CREATE_SHARE}" != "true" ]]; then
        log_warn "Skipping IESHARE partition because --no-share was passed"
        return 0
    fi

    eval "$(python3 - "$OUTPUT_IMG" "$FATSHARE_LABEL" <<'PY'
import binascii
import os
import struct
import sys
import uuid

image_path, share_label = sys.argv[1], sys.argv[2]
sector_size = 512
header_struct = struct.Struct("<8sIIIIQQQQ16sQIII")
basic_data_guid = uuid.UUID("EBD0A0A2-B9E5-4433-87C0-68B6B72699C7").bytes_le
zero_guid = b"\x00" * 16

def align_up(value, align):
    return ((value + align - 1) // align) * align

def read_header(f, lba):
    f.seek(lba * sector_size)
    raw = bytearray(f.read(sector_size))
    fields = header_struct.unpack(bytes(raw[:header_struct.size]))
    if fields[0] != b"EFI PART":
        raise SystemExit(f"{image_path}: missing GPT header at LBA {lba}")
    return raw, fields

def header_crc(raw, header_size):
    data = bytearray(raw[:header_size])
    struct.pack_into("<I", data, 16, 0)
    return binascii.crc32(data) & 0xffffffff

def write_header(f, current_lba, backup_lba, first_usable, last_usable, disk_guid,
                 entries_lba, entry_count, entry_size, entries_crc):
    header_size = 92
    raw = bytearray(sector_size)
    header_struct.pack_into(
        raw,
        0,
        b"EFI PART",
        0x00010000,
        header_size,
        0,
        0,
        current_lba,
        backup_lba,
        first_usable,
        last_usable,
        disk_guid,
        entries_lba,
        entry_count,
        entry_size,
        entries_crc,
    )
    crc = header_crc(raw, header_size)
    struct.pack_into("<I", raw, 16, crc)
    f.seek(current_lba * sector_size)
    f.write(raw)

def entry_used(entry):
    return entry[:16] != zero_guid

with open(image_path, "r+b") as f:
    image_size = os.fstat(f.fileno()).st_size
    if image_size % sector_size != 0:
        raise SystemExit(f"{image_path}: size is not sector-aligned")
    total_lbas = image_size // sector_size
    primary_raw, h = read_header(f, 1)
    (
        _sig,
        _rev,
        header_size,
        expected_crc,
        _reserved,
        current_lba,
        _old_backup_lba,
        first_usable,
        _old_last_usable,
        disk_guid,
        entries_lba,
        entry_count,
        entry_size,
        _entries_crc,
    ) = h
    if current_lba != 1:
        raise SystemExit(f"{image_path}: primary GPT is not at LBA 1")
    actual_crc = header_crc(primary_raw, header_size)
    if actual_crc != expected_crc:
        raise SystemExit(f"{image_path}: primary GPT header CRC mismatch")

    entries_bytes = entry_count * entry_size
    entry_sectors = align_up(entries_bytes, sector_size) // sector_size
    f.seek(entries_lba * sector_size)
    entries = bytearray(f.read(entry_sectors * sector_size))
    active = []
    empty_index = None
    for index in range(entry_count):
        start = index * entry_size
        entry = entries[start:start + entry_size]
        if entry_used(entry):
            first_lba, last_lba = struct.unpack_from("<QQ", entry, 32)
            active.append((index + 1, first_lba, last_lba))
        elif empty_index is None:
            empty_index = index
    if empty_index is None:
        raise SystemExit(f"{image_path}: no free GPT partition entry for {share_label}")
    if not active:
        raise SystemExit(f"{image_path}: no existing GPT partitions found")

    backup_lba = total_lbas - 1
    backup_entries_lba = backup_lba - entry_sectors
    last_usable = backup_entries_lba - 1
    share_start = align_up(max(last for _num, _first, last in active) + 1, 2048)
    share_end = last_usable
    if share_end <= share_start:
        raise SystemExit(
            f"{image_path}: not enough free space for {share_label}: "
            f"start={share_start}, end={share_end}"
        )

    name = share_label.encode("utf-16le")[:72]
    name += b"\x00" * (72 - len(name))
    new_entry = bytearray(entry_size)
    struct.pack_into("<16s16sQQQ72s", new_entry, 0,
                     basic_data_guid, uuid.uuid4().bytes_le,
                     share_start, share_end, 0, name)
    entries[empty_index * entry_size:(empty_index + 1) * entry_size] = new_entry
    entries_crc = binascii.crc32(entries[:entries_bytes]) & 0xffffffff

    f.seek(entries_lba * sector_size)
    f.write(entries)
    f.seek(backup_entries_lba * sector_size)
    f.write(entries)
    write_header(f, 1, backup_lba, first_usable, last_usable, disk_guid,
                 entries_lba, entry_count, entry_size, entries_crc)
    write_header(f, backup_lba, 1, first_usable, last_usable, disk_guid,
                 backup_entries_lba, entry_count, entry_size, entries_crc)

share_size_b = (share_end - share_start + 1) * sector_size
print(f"SECTOR_SIZE={sector_size}")
print(f"SHARE_START={share_start}")
print(f"SHARE_END={share_end}")
print(f"SHARE_START_B={share_start * sector_size}")
print(f"IESHARE_NUM={empty_index + 1}")
print(f"IESHARE_SIZE_B={share_size_b}")
print(f"IESHARE_DEV=/dev/sda{empty_index + 1}")
PY
)"
    export SECTOR_SIZE SHARE_START SHARE_END SHARE_START_B IESHARE_NUM IESHARE_SIZE_B IESHARE_DEV
    log "Sector size: ${SECTOR_SIZE} bytes"
    log "IESHARE device: ${IESHARE_DEV}"
    log "IESHARE byte range: start=${SHARE_START_B}, size=${IESHARE_SIZE_B}"
}

write_fstab() {
    log_section "Writing fstab entries"
    local fstab_host="${WORK_DIR}/fstab"
    local fstab_new="${WORK_DIR}/fstab.new"
    local commands_file="${WORK_DIR}/debugfs-fstab.cmds"
    /sbin/debugfs -R "dump /etc/fstab ${fstab_host}" "$ROOT_PART_IMG" 2>&1 | tee -a "$LOG_FILE"
    python3 - "$fstab_host" "$fstab_new" "$CREATE_SHARE" <<'PY'
import sys

src, dst, create_share = sys.argv[1], sys.argv[2], sys.argv[3] == "true"
lines = open(src, "r", encoding="utf-8").read().splitlines()
out = []
seen_share = False
for line in lines:
    parts = line.split()
    if parts and not line.lstrip().startswith("#"):
        if len(parts) >= 4 and parts[1] in ("/", "/boot"):
            opts = parts[3].split(",")
            if "relatime" not in opts:
                opts.append("relatime")
                parts[3] = ",".join(opts)
                line = "\t".join(parts)
        if parts[0] == "LABEL=IESHARE":
            seen_share = True
    out.append(line)
if create_share and not seen_share:
    out.append("LABEL=IESHARE /var/ie/share vfat defaults,relatime,nofail,umask=0022,uid=1000,gid=1000 0 0")
with open(dst, "w", encoding="utf-8") as f:
    f.write("\n".join(out) + "\n")
PY
    cat > "$commands_file" <<EOF
rm /etc/fstab
write ${fstab_new} /etc/fstab
sif /etc/fstab mode 0100644
sif /etc/fstab uid 0
sif /etc/fstab gid 0
EOF
    debugfs_apply "$commands_file"
}

format_share_partition_rootless() {
    if [[ "${CREATE_SHARE}" != "true" ]]; then
        log_warn "Skipping IESHARE FAT32 partition because --no-share was passed"
        return 0
    fi

    log_section "Formatting IESHARE FAT32 partition rootlessly"
    local fat_img="${WORK_DIR}/ieshare-fat32.img"
    rm -f "$fat_img"
    truncate -s "$IESHARE_SIZE_B" "$fat_img"
    mformat -i "$fat_img" -F -v "${FATSHARE_LABEL}" ::
    stage_share_payload
    local payload_entries=("${SHARE_PAYLOAD_ROOT}"/*)
    mcopy -i "$fat_img" -D A -s "${payload_entries[@]}" ::/
    dd if="$fat_img" of="$OUTPUT_IMG" bs=1M seek="$(( SHARE_START_B / 1048576 ))" conv=notrunc status=progress 2>&1 | tee -a "$LOG_FILE"
}

validate_image() {
    log_section "Validating image layout"
    python3 - "$OUTPUT_IMG" <<'PY' 2>&1 | tee -a "$LOG_FILE"
import os
import struct
import sys

image_path = sys.argv[1]
sector_size = 512
entry_size = 128
with open(image_path, "rb") as f:
    f.seek(sector_size)
    header = f.read(sector_size)
    if header[:8] != b"EFI PART":
        raise SystemExit(f"{image_path}: missing GPT header")
    entries_lba = struct.unpack_from("<Q", header, 72)[0]
    entry_count = struct.unpack_from("<I", header, 80)[0]
    entry_size = struct.unpack_from("<I", header, 84)[0]
    f.seek(entries_lba * sector_size)
    print("Name      Type       VFS     Label           MBR Size  Parent")
    for index in range(entry_count):
        entry = f.read(entry_size)
        if entry[:16] == b"\x00" * 16:
            continue
        first_lba, last_lba = struct.unpack_from("<QQ", entry, 32)
        raw_name = entry[56:128]
        name = raw_name.decode("utf-16le", errors="ignore").rstrip("\x00") or "-"
        size_b = (last_lba - first_lba + 1) * sector_size
        if size_b >= 1024 ** 3:
            size = f"{size_b // (1024 ** 3)}G"
        elif size_b >= 1024 ** 2:
            size = f"{size_b // (1024 ** 2)}M"
        else:
            size = f"{size_b}B"
        print(f"/dev/sda{index + 1:<2} partition  -       {name:<15} -   {size:<5} /dev/sda")
PY
}

compress_image() {
    log_section "Creating compressed release archive"
    local archive_path="${OUTPUT_IMG%.img}.zip"
    local archive_root="${WORK_DIR}/x64-live-archive"
    local archive_docs_dir="${archive_root}/Docs"
    local archive_refman_docs_dir="${archive_docs_dir}/IEProgRefMan"
    rm -rf "$archive_root"
    mkdir -p "$archive_root" "$archive_docs_dir" "$archive_refman_docs_dir"
    cp "$OUTPUT_IMG" "$archive_root/$(basename "$OUTPUT_IMG")"
    local companion_pdf
    for companion_pdf in "${SDK_COMPANION_PDFS[@]}"; do
        payload_require_file "$companion_pdf" "make x64-live-sdk-companion-pdfs" "SDK companion PDF $(basename "$companion_pdf")"
    done
    payload_require_glob "${REFMAN_PDF_DIR}/*.pdf" "make x64-live-refman-pdfs" "Programmer's Reference Guide PDFs"
    payload_require_file "${REFMAN_PDF_DIR}/00-Preface.pdf" "make x64-live-refman-pdfs" "Programmer's Reference Guide preface PDF"
    payload_require_file "${REFMAN_PDF_DIR}/39-whole-machine-capstone.pdf" "make x64-live-refman-pdfs" "Programmer's Reference Guide final chapter PDF"
    payload_require_file "${REFMAN_PDF_DIR}/appK-block-diagrams.pdf" "make x64-live-refman-pdfs" "Programmer's Reference Guide final appendix PDF"
    cp -f "${SDK_COMPANION_PDFS[@]}" "$archive_docs_dir/"
    cp -f "${REFMAN_PDF_DIR}"/*.pdf "$archive_refman_docs_dir/"
    rm -f "$archive_path"
    python3 - "$archive_path" "$archive_root" "$(basename "$OUTPUT_IMG")" Docs <<'PY' 2>&1 | tee -a "$LOG_FILE"
import os
import sys
import zipfile

archive_path, archive_root = sys.argv[1], sys.argv[2]
entries = sys.argv[3:]

with zipfile.ZipFile(archive_path, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=1, allowZip64=True) as zf:
    for entry in entries:
        entry_path = os.path.join(archive_root, entry)
        if os.path.isdir(entry_path):
            for dirpath, dirnames, filenames in os.walk(entry_path):
                dirnames.sort()
                for filename in sorted(filenames):
                    path = os.path.join(dirpath, filename)
                    zf.write(path, os.path.relpath(path, archive_root))
        else:
            zf.write(entry_path, entry)
PY
    log_success "Created ${archive_path}"
}

main() {
    configure_guestfs_environment
    if [[ "$PAYLOAD_CHECK_ONLY" == "true" ]]; then
        check_live_payload_inputs
        stage_share_payload
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
    flush_root_partition_image
    format_share_partition_rootless
    validate_image
    compress_image
    log_success "x64 live image complete: ${OUTPUT_IMG%.img}.zip"
}

main "$@"
