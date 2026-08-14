#!/usr/bin/env bash
# Build one Raspberry Pi Intuition Engine appliance image from a validated,
# natively prepared ARM64 golden image.  It never modifies the golden source.
set -euo pipefail

readonly script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly project_dir="$(cd "${script_dir}/.." && pwd)"
readonly appliance_assets="${script_dir}/rpi-live"

fail() { echo "rpi-live-image: $*" >&2; exit 1; }
resolve_host_tool() {
    local requested="$1" candidate directory
    if [[ "$requested" == */* ]]; then
        [[ -x "$requested" ]] || return 1
        printf '%s\n' "$requested"
        return 0
    fi
    if candidate="$(command -v -- "$requested" 2>/dev/null)"; then
        printf '%s\n' "$candidate"
        return 0
    fi
    IFS=: read -r -a directories <<<"${RPI_SYSTEM_TOOL_DIRS:-/usr/sbin:/sbin}"
    for directory in "${directories[@]}"; do
        candidate="${directory%/}/$requested"
        if [[ -x "$candidate" ]]; then
            printf '%s\n' "$candidate"
            return 0
        fi
    done
    return 1
}
readonly guestfish_bin="$(resolve_host_tool "${RPI_GUESTFISH:-guestfish}")" || fail "required tool not found: ${RPI_GUESTFISH:-guestfish}"
readonly sfdisk_bin="$(resolve_host_tool "${RPI_SFDISK:-sfdisk}")" || fail "required tool not found: ${RPI_SFDISK:-sfdisk}"
need() { command -v "$1" >/dev/null 2>&1 || fail "required tool not found: $1"; }
guestfish() { "$guestfish_bin" "$@"; }

board=""
binary=""
output=""
source_image=""
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
Options: --golden FILE --manifest FILE --source-image FILE --payload DIR --work-dir DIR
         --check-payload --no-share
EOF
}

while (($#)); do
    case "$1" in
        --board|--binary|--output|--golden|--manifest|--source-image|--assets|--payload|--work-dir)
            (($# >= 2)) || fail "missing value for $1"
            case "$1" in
                --board) board="$2" ;; --binary) binary="$2" ;; --output) output="$2" ;;
                --golden) golden="$2" ;; --manifest) manifest="$2" ;; --assets) legacy_assets="$2" ;;
                --source-image) source_image="$2" ;;
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
if [[ -n "$source_image" ]] && ! "$create_share"; then
    fail "--source-image cannot be combined with --no-share"
fi
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
    local expected_architecture expected_partition_table expected_os_release expected_kernel expected_kernel_image expected_packages guestfish_partition_table actual_partition_table os_release package package_status passwd_file group_file boot_config ie_entry
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
    package_status="$(guestfish --ro -a "$golden" run : mount-ro /dev/sda2 / : cat /var/lib/dpkg/status)"
    for package in ${expected_packages//,/ }; do
        awk -v package="$package" 'BEGIN { RS=""; FS="\n" }
            $0 ~ "(^|\\n)Package: " package "(\\n|$)" &&
            $0 ~ "(^|\\n)Status: install ok installed(\\n|$)" { found=1 }
            END { exit !found }' <<<"$package_status" ||
            fail "golden image lacks required installed package: $package"
    done
    passwd_file="$(guestfish --ro -a "$golden" run : mount-ro /dev/sda2 / : cat /etc/passwd)"
    group_file="$(guestfish --ro -a "$golden" run : mount-ro /dev/sda2 / : cat /etc/group)"
    ie_entry="$(awk -F: '$1 == "ie" { print; exit }' <<<"$passwd_file")"
    IFS=: read -r _ _ ie_uid ie_gid _ ie_home ie_shell <<<"$ie_entry"
    [[ "$ie_uid" =~ ^[1-9][0-9]*$ && "$ie_gid" =~ ^[1-9][0-9]*$ && "$ie_home" == /home/ie && "$ie_shell" == /bin/sh ]] || \
        fail 'golden image lacks a login-capable IE appliance user'
    awk -F: -v gid="$ie_gid" '$1 == "ie" && $3 == gid { found=1 } END { exit !found }' <<<"$group_file" || \
        fail 'golden image lacks the IE appliance primary group'
    # The immutable golden supplies the Raspberry Pi firmware, RT kernel and
    # audio runtime.  The normal builder is rootless and offline: it injects
    # the appliance session, services and policies but never executes an ARM64
    # package manager inside the image.
    for path in \
        /lib/ld-linux-aarch64.so.1 \
        /usr/bin/jackd \
        /usr/bin/cage \
        /usr/bin/Xwayland \
        /usr/sbin/greetd \
        /usr/lib/systemd/system/greetd.service; do
        golden_has_file "$path" || fail "golden image lacks required file: $path"
    done
    for path in /config.txt /cmdline.txt "/$expected_kernel_image"; do
        golden_boot_has_file "$path" || fail "golden boot partition lacks required file: $path"
    done
    boot_config="$(guestfish --ro -a "$golden" run : mount-ro /dev/sda1 / : cat /config.txt)"
    grep -Fxq 'kernel=kernel8_rt.img' <<<"$boot_config" || fail 'golden boot configuration does not select kernel8_rt.img'
    grep -Fxq 'initramfs initramfs8_rt followkernel' <<<"$boot_config" || fail 'golden boot configuration does not select initramfs8_rt'
    golden_has_any_file /usr/libexec/rtkit-daemon /usr/bin/rtkit-daemon || \
        fail "golden image lacks rtkit-daemon"
}

finalise_output() {
    local -a verify_args
    verify_args=(--board "$board" --image "$output" --binary "$binary")
    if "$create_share"; then
        verify_args+=(--share)
    fi
    "${script_dir}/verify_rpi_live_image.sh" "${verify_args[@]}"
    sha256sum "$output" >"${output}.sha256"

    local archive_root
    archive_root="${work_dir}/archive-${board}"
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
[[ -n "$work_dir" ]] || work_dir="$(dirname "$output")/work"
output_canonical="$(realpath -m "$output")"

if [[ -n "$source_image" ]]; then
    [[ -f "$source_image" ]] || fail "missing source appliance image: $source_image"
    source_canonical="$(readlink -f "$source_image")"
    [[ "$output_canonical" != "$source_canonical" ]] || fail "output path aliases source appliance image"
    for tool in "$guestfish_bin" python3; do need "$tool"; done
    mkdir -p "$work_dir" "$(dirname "$output")"
    source_sha="$(sha256sum "$source_image" | awk '{print $1}')"
    tmp_output="${work_dir}/$(basename "$output").new"
    rm -f "$tmp_output"
    cp --reflink=auto --sparse=always "$source_image" "$tmp_output"
    [[ "$(sha256sum "$source_image" | awk '{print $1}')" == "$source_sha" ]] || fail "source appliance image changed while copying"
    guestfish --rw -a "$tmp_output" run : mount /dev/sda2 / : rm /opt/ie/IntuitionEngine : copy-in "$binary" /opt/ie/ : mv "/opt/ie/$(basename "$binary")" /opt/ie/IntuitionEngine : chmod 0755 /opt/ie/IntuitionEngine
    mv "$tmp_output" "$output"
    finalise_output
    exit 0
fi

[[ -f "$golden" ]] || fail "missing golden image: $golden"
[[ -f "$manifest" ]] || fail "missing golden manifest: $manifest"
golden_canonical="$(readlink -f "$golden")"
[[ "$output_canonical" != "$golden_canonical" ]] || fail "output path aliases golden image"
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

# AppArmor must be selected at kernel initialisation time. Installing profiles
# and enabling their loader service is insufficient when the Pi kernel starts
# with only the capability LSM active.
boot_cmdline="$work_dir/cmdline.txt"
guestfish --ro -a "$tmp_output" run : mount-ro /dev/sda1 / : cat /cmdline.txt >"$boot_cmdline"
read -r boot_args <"$boot_cmdline" || true
for required_arg in \
    apparmor=1 \
    security=apparmor \
    lsm=landlock,lockdown,yama,integrity,apparmor,bpf; do
    case " $boot_args " in
        *" $required_arg "*) ;;
        *) boot_args="${boot_args:+$boot_args }$required_arg" ;;
    esac
done
printf '%s\n' "$boot_args" >"$boot_cmdline"
guestfish --rw -a "$tmp_output" run : mount /dev/sda1 / : upload "$boot_cmdline" /cmdline.txt

# Image population is intentionally offline.  guestfish obtains no ARM64 execution path.
guestfish --rw -a "$tmp_output" run : mount /dev/sda2 / : mkdir-p /opt/ie : mkdir-p /var/ie/share : mkdir-p /var/ie/state : mkdir-p /var/ie/runtime : mkdir-p /usr/libexec : \
    copy-in "$binary" /opt/ie/ : \
    copy-in "$host_helper" /usr/libexec/ : mv "/opt/ie/$(basename "$binary")" /opt/ie/IntuitionEngine : chmod 0755 /opt/ie/IntuitionEngine : chmod 0755 /usr/libexec/intuitionengine-host-helper : chown "$ie_uid" "$ie_gid" /var/ie : chown "$ie_uid" "$ie_gid" /var/ie/share : chown "$ie_uid" "$ie_gid" /var/ie/state : chown "$ie_uid" "$ie_gid" /var/ie/runtime
guestfish --rw -a "$tmp_output" run : mount /dev/sda2 / : mkdir-p /etc/greetd : mkdir-p /etc/apparmor.d : mkdir-p /usr/local/sbin : mkdir-p /usr/local/share/kbd/keymaps : mkdir-p /etc/systemd/system : mkdir-p /etc/systemd/system/greetd.service.requires : mkdir-p /etc/systemd/system/ie-host-helper.service.requires : mkdir-p /etc/systemd/system/graphical.target.wants : mkdir-p /etc/systemd/system/multi-user.target.wants : mkdir-p /etc/systemd/system/network-pre.target.wants : \
    copy-in "$appliance_assets/ie-session.sh" /opt/ie/ : copy-in "$appliance_assets/ie-launch.sh" /opt/ie/ : copy-in "$appliance_assets/ie-grow-share.sh" /usr/local/sbin/ : \
    copy-in "$appliance_assets/ie-grow-share.service" /etc/systemd/system/ : copy-in "$appliance_assets/greetd-config.toml" /etc/greetd/ : mv /etc/greetd/greetd-config.toml /etc/greetd/config.toml : \
    copy-in "$appliance_assets/opt.ie.IntuitionEngine" /etc/apparmor.d/ : copy-in "$appliance_assets/usr.libexec.intuitionengine-host-helper" /etc/apparmor.d/ : copy-in "$appliance_assets/ie-apparmor.service" /etc/systemd/system/ : copy-in "$appliance_assets/ie-host-helper.service" /etc/systemd/system/ : \
    copy-in "$appliance_assets/ie-firewall.service" /etc/systemd/system/ : copy-in "$appliance_assets/ie-no-vt-switch.service" /etc/systemd/system/ : \
    copy-in "$appliance_assets/ie-no-vt-switch.map" /usr/local/share/kbd/keymaps/ : chmod 0755 /opt/ie/ie-session.sh : chmod 0755 /opt/ie/ie-launch.sh : chmod 0755 /usr/local/sbin/ie-grow-share.sh : \
    ln-s /etc/systemd/system/ie-apparmor.service /etc/systemd/system/greetd.service.requires/ie-apparmor.service : \
    ln-s /usr/lib/systemd/system/greetd.service /etc/systemd/system/graphical.target.wants/greetd.service : \
    rm-f /etc/systemd/system/default.target : ln-s /usr/lib/systemd/system/graphical.target /etc/systemd/system/default.target : \
    rm-f /etc/systemd/system/getty.target.wants/getty@tty1.service : ln-s /dev/null /etc/systemd/system/getty@tty1.service : \
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
        write-append /etc/fstab $'LABEL=IESHARE /var/ie/share vfat defaults,nofail,x-systemd.device-timeout=10 0 0\n'
fi
mv "$tmp_output" "$output"
finalise_output
