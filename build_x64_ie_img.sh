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
GOLDEN_STAMP_VERSION="x64-live-golden-v9-emutos-fat32-drive"
GOLDEN_STAMP_PATH="${GOLDEN_IMG_PATH}.stamp"
KERNEL_PKG="linux-lowlatency"
COMPOSITOR_PKGS="cage,seatd,greetd,xwayland,xwayland-run,mesa-utils,libgl1,libegl1,libgles2,libwayland-client0,libxkbcommon0,fonts-dejavu-core"
X11_RUNTIME_PKGS="libxrandr2,libxxf86vm1,libxi6,libxcursor1,libxinerama1,libx11-6,libxext6,libxfixes3,libxrender1"
AUDIO_PKGS="pipewire,pipewire-pulse,wireplumber,pipewire-alsa,alsa-utils"
SECUREBOOT_PKGS="shim-signed,grub-efi-amd64-signed,sbsigntool"
OVERLAY_PKG="overlayroot"
SHARE_GROW_PKGS="cloud-guest-utils,dosfstools,fatresize,parted"
NETWORK_PKGS="network-manager,wpasupplicant,wireless-regdb,iw"
ALL_PKGS="${KERNEL_PKG},${COMPOSITOR_PKGS},${X11_RUNTIME_PKGS},${AUDIO_PKGS},${SECUREBOOT_PKGS},${OVERLAY_PKG},${SHARE_GROW_PKGS},${NETWORK_PKGS}"
IE_BINARY="${SCRIPT_DIR}/bin/IntuitionEngine_v3"
IE_INSTALL_NAME="IntuitionEngine"
FINAL_IMAGE_SIZE="8G"
ROOT_PART_SIZE="5G"
FATSHARE_LABEL="IESHARE"
OUTPUT_IMG="${X64_LIVE_OUTPUT_IMG:-${LIVE_OUT_DIR}/intuition-engine-x64.img}"

FORCE_REBUILD_GOLDEN=false
CREATE_SHARE=true

usage() {
    cat <<EOF
Usage: $0 [--rebuild-golden] [--no-share]

  --rebuild-golden  Rebuild the cached Ubuntu package/config golden image.
  --no-share        Do not create the host-visible FAT32 IESHARE partition.
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
    local required_cmds=(aria2c curl virt-customize virt-resize virt-filesystems guestfish qemu-img file zstd tar python3)
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

    local available_space
    available_space="$(df -BG "$SCRIPT_DIR" | awk 'NR==2 {print $4}' | sed 's/G//')"
    if [[ "$available_space" -lt 18 ]]; then
        log_error "Insufficient disk space. Need 18GB, have ${available_space}GB"
        exit 1
    fi
    log_success "Sufficient disk space available (${available_space}GB)"
}

stage_share_payload() {
    log_section "Staging IESHARE demo payload"
    local payload_root="${WORK_DIR}/ieshare-payload"
    local demos_dir="${payload_root}/Demos"
    rm -rf "$payload_root"
    mkdir -p "$demos_dir"

    shopt -s nullglob
    local demo_files=(
        "${SCRIPT_DIR}"/sdk/examples/prebuilt/*.ie*
        "${SCRIPT_DIR}"/sdk/examples/prebuilt/*.prg
    )
    shopt -u nullglob

    if [[ ${#demo_files[@]} -eq 0 ]]; then
        log_error "No .ie* or .prg demos found in ${SCRIPT_DIR}/sdk/examples/prebuilt"
        log_error "Run: make x64-live-demos"
        exit 1
    fi
    cp -f "${demo_files[@]}" "$demos_dir/"
    cp -f "${SCRIPT_DIR}/sdk/examples/basic/rotozoomer_basic.bas" "$demos_dir/"

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
        cp -f "$aros_demo" "$demos_dir/"
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
It contains runnable Intuition Engine demo binaries, EmuTOS/AROS demo programs,
and the AB3D2 runtime asset tree extracted from the embedded release payload.
EOF

    find "$demos_dir" -maxdepth 2 -type f | sort | sed "s#^${payload_root}/#  #" | tee -a "$LOG_FILE"
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
cd /var/ie/share
pipewire >/tmp/ie-pipewire.log 2>&1 &
wireplumber >/tmp/ie-wireplumber.log 2>&1 &
pipewire-pulse >/tmp/ie-pipewire-pulse.log 2>&1 &
exec /opt/ie/IntuitionEngine -emutos-drive /var/ie/share
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
        --upload "${WORK_DIR}/launch.sh:/opt/ie/launch.sh" \
        --run-command 'chmod +x /opt/ie/launch.sh' \
        --run-command 'chown 1000:1000 /opt/ie/launch.sh /opt/ie /var/ie /var/ie/share' \
        --upload "${WORK_DIR}/greetd-config.toml:/etc/greetd/config.toml" \
        --upload "${WORK_DIR}/overlayroot.conf:/etc/overlayroot.conf" \
        --upload "${WORK_DIR}/90-ie-networkmanager.yaml:/etc/netplan/90-ie-networkmanager.yaml" \
        --upload "${WORK_DIR}/ie-grow-share.sh:/usr/local/sbin/ie-grow-share.sh" \
        --upload "${WORK_DIR}/ie-grow-share.service:/etc/systemd/system/ie-grow-share.service" \
        --run-command 'chmod +x /usr/local/sbin/ie-grow-share.sh' \
        --run-command 'systemctl mask getty@tty1.service' \
        --run-command 'systemctl enable greetd.service seatd.service' \
        --run-command 'systemctl enable NetworkManager.service' \
        --run-command 'systemctl enable getty@tty2.service' \
        --run-command 'systemctl enable ie-grow-share.service' \
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
        --run-command "chmod +x /opt/ie/${IE_INSTALL_NAME}" \
        --run-command 'chown -R 1000:1000 /var/ie /opt/ie' \
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

    if [[ "${CREATE_SHARE}" == "true" ]]; then
        guestfish -a "${OUTPUT_IMG}" <<GUESTFISH_EOF
run
mount ${os_root_dev} /
mkdir-p /var/ie/share
write-append /etc/fstab "LABEL=IESHARE /var/ie/share vfat defaults,nofail,umask=0022,uid=1000,gid=1000 0 0\n"
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
    mcopy -i "$fat_img" -D A -s "${SHARE_PAYLOAD_ROOT}/Demos" ::/
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
    check_dependencies
    mkdir -p "$WORK_DIR"
    if check_golden_image; then
        log "Using cached golden image"
    else
        download_ubuntu
        generate_support_files
        build_golden_image
    fi
    install_ie_binary
    append_partitions
    write_fstab
    format_share_partition_rootless
    validate_image
    compress_image
    log_success "x64 live image complete: ${OUTPUT_IMG%.img}.tar.zst"
}

main "$@"
