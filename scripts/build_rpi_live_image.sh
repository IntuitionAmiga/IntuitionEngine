#!/usr/bin/env bash
# Build one Raspberry Pi Intuition Engine appliance image from a validated,
# natively prepared ARM64 golden image.  It never modifies the golden source.
set -euo pipefail

readonly script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly project_dir="$(cd "${script_dir}/.." && pwd)"
readonly appliance_assets="${script_dir}/rpi-live"
readonly guestfish_bin="${RPI_GUESTFISH:-guestfish}"
readonly sfdisk_bin="${RPI_SFDISK:-sfdisk}"

fail() { echo "rpi-live-image: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || fail "required tool not found: $1"; }
guestfish() { "$guestfish_bin" "$@"; }

board=""
binary=""
output=""
golden="${project_dir}/rpi-ie-golden.img"
manifest="${project_dir}/scripts/rpi-ie-golden.manifest"
payload=""
work_dir=""
legacy_assets=""
host_helper="${RPI_HOST_HELPER:-${project_dir}/build/rpi-live/intuitionengine-host-helper}"
check_payload=false
create_share=true

usage() {
    cat >&2 <<'EOF'
Usage: build_rpi_live_image.sh --board pi4|pi400|pi5 --binary FILE --output FILE [options]
Options: --golden FILE --manifest FILE --payload DIR --work-dir DIR
         --check-payload --no-share
EOF
}

while (($#)); do
    case "$1" in
        --board|--binary|--output|--golden|--manifest|--assets|--payload|--work-dir)
            (($# >= 2)) || fail "missing value for $1"
            case "$1" in
                --board) board="$2" ;; --binary) binary="$2" ;; --output) output="$2" ;;
                --golden) golden="$2" ;; --manifest) manifest="$2" ;; --assets) legacy_assets="$2" ;;
                --payload) payload="$2" ;; --work-dir) work_dir="$2" ;;
            esac
            shift 2 ;;
        --check-payload) check_payload=true; shift ;;
        --no-share) create_share=false; shift ;;
        -h|--help) usage; exit 0 ;;
        *) fail "unknown argument: $1" ;;
    esac
done

case "$board" in pi4|pi400|pi5) ;; *) fail "board must be pi4, pi400 or pi5" ;; esac
[[ -n "$binary" && -f "$binary" ]] || fail "missing ARM64 binary"
[[ -n "$payload" && -d "$payload" ]] || fail "missing payload directory"
for required in ie-session.sh ie-launch.sh ie-grow-share.sh ie-grow-share.service greetd-config.toml opt.ie.IntuitionEngine usr.libexec.intuitionengine-host-helper ie-apparmor.service ie-host-helper.service ie-firewall.service ie-no-vt-switch.service ie-no-vt-switch.map; do
    [[ -f "$appliance_assets/$required" ]] || fail "missing Raspberry Pi appliance asset: $required"
done
if ! file -b "$binary" 2>/dev/null | grep -Eqi 'ELF.*(aarch64|ARM aarch64)'; then
    fail "binary is not an ARM64 ELF: $binary"
fi

golden_has_file() {
    guestfish --ro -a "$golden" run : mount-ro /dev/sda2 / : is-file "$1" | grep -qx true || \
        guestfish --ro -a "$golden" run : mount-ro /dev/sda2 / : is-symlink "$1" | grep -qx true
}

golden_boot_has_file() {
    guestfish --ro -a "$golden" run : mount-ro /dev/sda1 / : is-file "$1" | grep -qx true || \
        guestfish --ro -a "$golden" run : mount-ro /dev/sda1 / : is-symlink "$1" | grep -qx true
}

golden_has_any_file() {
    local path
    for path in "$@"; do
        if golden_has_file "$path"; then
            return 0
        fi
    done
    return 1
}

manifest_value() {
    sed -n "s/^$1=//p" "$manifest" | head -n 1
}

validate_golden_image() {
    need "$guestfish_bin"
    local expected_architecture expected_partition_table expected_os_release expected_kernel expected_kernel_image expected_packages guestfish_partition_table actual_partition_table os_release
    expected_architecture="$(manifest_value architecture)"
    expected_partition_table="$(manifest_value partition_table)"
    expected_os_release="$(manifest_value os_release)"
    expected_kernel="$(manifest_value kernel)"
    expected_kernel_image="$(manifest_value kernel_image)"
    expected_packages="$(manifest_value packages)"
    [[ "$expected_architecture" == aarch64 ]] || fail "golden manifest must declare architecture=aarch64"
    [[ "$expected_partition_table" == dos ]] || fail "golden manifest must declare partition_table=dos"
    [[ -n "$expected_os_release" ]] || fail "golden manifest has no os_release entry"
    [[ "$expected_kernel" == PREEMPT_RT ]] || fail "golden manifest must declare kernel=PREEMPT_RT"
    [[ "$expected_kernel_image" == kernel8_rt.img ]] || fail "golden manifest must declare kernel_image=kernel8_rt.img"
    [[ "$expected_packages" == jackd2,rtkit,cage,xwayland,xwayland-run,greetd,network-manager,fonts-dejavu-core,kbd,apparmor,apparmor-utils,polkitd,ufw,dosfstools,cloud-guest-utils ]] || \
        fail "golden manifest package contract is incomplete"
    # The manifest uses the conventional "dos" spelling while libguestfs
    # reports that partition-table type as "msdos".
    guestfish_partition_table="$(guestfish --ro -a "$golden" run : part-get-parttype /dev/sda)"
    case "$guestfish_partition_table" in
        msdos) actual_partition_table=dos ;;
        *) actual_partition_table="$guestfish_partition_table" ;;
    esac
    [[ "$actual_partition_table" == "$expected_partition_table" ]] || fail "golden image partition table is not $expected_partition_table"
    os_release="$(guestfish --ro -a "$golden" run : mount-ro /dev/sda2 / : cat /etc/os-release)"
    grep -Fxq "PRETTY_NAME=\"$expected_os_release\"" <<<"$os_release" || fail "golden image operating-system release differs from manifest"
    # The immutable golden supplies the Raspberry Pi firmware, RT kernel and
    # audio runtime.  The normal builder is rootless and offline: it injects
    # the appliance session, services and policies but never executes an ARM64
    # package manager inside the image.
    for path in \
        /lib/ld-linux-aarch64.so.1 \
        /usr/bin/jackd; do
        golden_has_file "$path" || fail "golden image lacks required file: $path"
    done
    for path in /config.txt /cmdline.txt "/$expected_kernel_image"; do
        golden_boot_has_file "$path" || fail "golden boot partition lacks required file: $path"
    done
    golden_has_any_file /usr/libexec/rtkit-daemon /usr/bin/rtkit-daemon || \
        fail "golden image lacks rtkit-daemon"
    # The copied image is appliance-only. Any residual Subsynth executable,
    # service or private tree is a release blocker, not a cosmetic warning.
    if guestfish --ro -a "$golden" run : mount-ro /dev/sda2 / : find / 2>/dev/null | grep -Eqi '(^|/)(subsynth|intuition[ -]?subsynth)'; then
        fail "golden image contains forbidden Subsynth residue"
    fi
}

# The check is deliberately content-oriented: normal build mode has stronger
# libguestfs checks, while this mode lets Make prove payload inputs without an image write.
[[ -d "$payload/Systems" ]] || fail "payload lacks Systems directory"
[[ -f "$payload/Docs/IE64_ISA.pdf" ]] || fail "payload lacks companion manuals"
[[ -d "$payload/Docs/IEProgRefGuide" ]] || fail "payload lacks Programmer's Reference Guide tree"
if "$check_payload"; then
    exit 0
fi

[[ -n "$output" ]] || fail "missing output path"
[[ -f "$golden" ]] || fail "missing golden image: $golden"
[[ -f "$manifest" ]] || fail "missing golden manifest: $manifest"
golden_canonical="$(readlink -f "$golden")"
output_canonical="$(realpath -m "$output")"
[[ "$output_canonical" != "$golden_canonical" ]] || fail "output path aliases golden image"
[[ -n "$work_dir" ]] || work_dir="$(dirname "$output")/work"
expected_sha="$(sed -n 's/^sha256=//p' "$manifest" | head -n 1)"
[[ "$expected_sha" =~ ^[[:xdigit:]]{64}$ ]] || fail "golden manifest has no sha256= entry"
actual_sha="$(sha256sum "$golden" | awk '{print $1}')"
[[ "$actual_sha" == "$expected_sha" ]] || fail "golden image checksum does not match manifest"

for tool in "$sfdisk_bin" python3; do need "$tool"; done
validate_golden_image
[[ -f "$host_helper" ]] || fail "missing ARM64 host helper: $host_helper"
file -b "$host_helper" 2>/dev/null | grep -Eqi 'ELF.*(aarch64|ARM aarch64)' || fail "host helper is not an ARM64 ELF: $host_helper"
mkdir -p "$work_dir" "$(dirname "$output")"
tmp_output="${work_dir}/$(basename "$output").new"
rm -f "$tmp_output"
cp --reflink=auto "$golden" "$tmp_output"
[[ "$(sha256sum "$golden" | awk '{print $1}')" == "$actual_sha" ]] || fail "golden image changed while copying"

# Image population is intentionally offline.  guestfish obtains no ARM64 execution path.
guestfish --rw -a "$tmp_output" run : mount /dev/sda2 / : mkdir-p /opt/ie : mkdir-p /var/ie/share : mkdir-p /var/ie/state : mkdir-p /var/ie/runtime : mkdir-p /usr/libexec : \
    copy-in "$binary" /opt/ie/ : \
    copy-in "$host_helper" /usr/libexec/ : mv "/opt/ie/$(basename "$binary")" /opt/ie/IntuitionEngine : chmod 0755 /opt/ie/IntuitionEngine : chmod 0755 /usr/libexec/intuitionengine-host-helper : chown 1000 1000 /var/ie : chown 1000 1000 /var/ie/share : chown 1000 1000 /var/ie/state : chown 1000 1000 /var/ie/runtime
guestfish --rw -a "$tmp_output" run : mount /dev/sda2 / : mkdir-p /etc/greetd : mkdir-p /etc/apparmor.d : mkdir-p /usr/local/sbin : mkdir-p /usr/local/share/kbd/keymaps : mkdir-p /etc/systemd/system : mkdir-p /etc/systemd/system/greetd.service.requires : mkdir-p /etc/systemd/system/ie-host-helper.service.requires : mkdir-p /etc/systemd/system/multi-user.target.wants : mkdir-p /etc/systemd/system/network-pre.target.wants : \
    copy-in "$appliance_assets/ie-session.sh" /opt/ie/ : copy-in "$appliance_assets/ie-launch.sh" /opt/ie/ : copy-in "$appliance_assets/ie-grow-share.sh" /usr/local/sbin/ : \
    copy-in "$appliance_assets/ie-grow-share.service" /etc/systemd/system/ : copy-in "$appliance_assets/greetd-config.toml" /etc/greetd/ : mv /etc/greetd/greetd-config.toml /etc/greetd/config.toml : \
    copy-in "$appliance_assets/opt.ie.IntuitionEngine" /etc/apparmor.d/ : copy-in "$appliance_assets/usr.libexec.intuitionengine-host-helper" /etc/apparmor.d/ : copy-in "$appliance_assets/ie-apparmor.service" /etc/systemd/system/ : copy-in "$appliance_assets/ie-host-helper.service" /etc/systemd/system/ : \
    copy-in "$appliance_assets/ie-firewall.service" /etc/systemd/system/ : copy-in "$appliance_assets/ie-no-vt-switch.service" /etc/systemd/system/ : \
    copy-in "$appliance_assets/ie-no-vt-switch.map" /usr/local/share/kbd/keymaps/ : chmod 0755 /opt/ie/ie-session.sh : chmod 0755 /opt/ie/ie-launch.sh : chmod 0755 /usr/local/sbin/ie-grow-share.sh : \
    ln-s /etc/systemd/system/ie-apparmor.service /etc/systemd/system/greetd.service.requires/ie-apparmor.service : \
    ln-s /etc/systemd/system/ie-apparmor.service /etc/systemd/system/ie-host-helper.service.requires/ie-apparmor.service : \
    ln-s /etc/systemd/system/ie-host-helper.service /etc/systemd/system/multi-user.target.wants/ie-host-helper.service : \
    ln-s /etc/systemd/system/ie-firewall.service /etc/systemd/system/network-pre.target.wants/ie-firewall.service : \
    ln-s /etc/systemd/system/ie-no-vt-switch.service /etc/systemd/system/multi-user.target.wants/ie-no-vt-switch.service

if "$create_share"; then
    truncate -s 20G "$tmp_output"
    # The real partition creation is kept as an offline helper so the golden
    # boot and root partitions remain untouched before this appended partition.
    RPI_SFDISK="$sfdisk_bin" RPI_GUESTFISH="$guestfish_bin" "${script_dir}/rpi_append_ieshare.sh" "$tmp_output" "$payload"
    guestfish --rw -a "$tmp_output" run : mount /dev/sda2 / : mkdir-p /etc/systemd/system/multi-user.target.wants : \
        ln-s /etc/systemd/system/ie-grow-share.service /etc/systemd/system/multi-user.target.wants/ie-grow-share.service : \
        write-append /etc/fstab "LABEL=IESHARE /var/ie/share vfat defaults,nofail,x-systemd.device-timeout=10 0 0\n"
fi
mv "$tmp_output" "$output"
verify_args=(--board "$board" --image "$output" --binary "$binary")
if "$create_share"; then
    verify_args+=(--share)
fi
"${script_dir}/verify_rpi_live_image.sh" "${verify_args[@]}"
sha256sum "$output" >"${output}.sha256"
archive_root="${work_dir}/archive-${board}"
archive_name="$(basename "${output%.img}.zip")"
rm -rf "$archive_root"
mkdir -p "$archive_root/Docs/IEProgRefMan"
cp "$output" "$archive_root/$(basename "$output")"
find "$payload/Docs" -maxdepth 1 -type f -name '*.pdf' -exec cp -f {} "$archive_root/Docs/" \;
cp -a "$payload/Docs/IEProgRefGuide/." "$archive_root/Docs/IEProgRefMan/"
python3 - "${output%.img}.zip" "$archive_root" "$(basename "$output")" Docs <<'PY'
import os
import sys
import zipfile

archive_path, archive_root, image_name, docs_name = sys.argv[1:]
with zipfile.ZipFile(archive_path, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=1, allowZip64=True) as archive:
    for entry in (image_name, docs_name):
        entry_path = os.path.join(archive_root, entry)
        if os.path.isdir(entry_path):
            for directory, directories, filenames in os.walk(entry_path):
                directories.sort()
                for filename in sorted(filenames):
                    path = os.path.join(directory, filename)
                    archive.write(path, os.path.relpath(path, archive_root))
        else:
            archive.write(entry_path, entry)
PY
sha256sum "${output%.img}.zip" >"${output%.img}.zip.sha256"
