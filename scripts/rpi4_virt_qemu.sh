#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
appliance_assets="$script_dir/rpi-live"
image="${1:?Pi 4 image path is required}"
timeout_seconds="${RPI_QEMU_TIMEOUT_SECONDS:-600}"
prepare_output="${RPI_QEMU_PREPARE_GOLDEN_OUTPUT:-}"
cache_dir="${RPI_QEMU_CACHE_DIR:-build/rpi-qemu-cache}"
cloud_url="https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-genericcloud-arm64.qcow2"
sums_url="https://cloud.debian.org/images/cloud/bookworm/latest/SHA512SUMS"
cloud_image="$cache_dir/debian-12-genericcloud-arm64.qcow2"
cloud_sums="$cache_dir/SHA512SUMS"
kernel_package_name="linux-image-6.1.0-50-arm64_6.1.176-1_arm64.deb"
kernel_package_url="https://deb.debian.org/debian/pool/main/l/linux-signed-arm64/$kernel_package_name"
kernel_package_sha256="914f75b57a8e165d85fb910c3e2dcc7000a05a9fea3f42f90b26e8dd590d5f06"
kernel_package="$cache_dir/$kernel_package_name"
depmod_command=/usr/sbin/depmod

fail() { echo "rpi4-live-qemu: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || fail "required tool is unavailable: $1"; }
[[ "$timeout_seconds" =~ ^[1-9][0-9]*$ ]] || fail 'RPI_QEMU_TIMEOUT_SECONDS must be a positive integer'
[[ -f "$image" ]] || fail "missing image: $image"
for tool in ar cpio curl guestfish qemu-img qemu-system-aarch64 sha256sum sha512sum tar timeout rg xz zstd; do need "$tool"; done
root_partuuid="$(guestfish --ro -a "$image" run : blkid /dev/sda2 | awk '$1 == "PART_ENTRY_UUID:" { print $2; exit }')"
[[ "$root_partuuid" =~ ^[0-9a-fA-F]+-[0-9]+$ ]] || fail 'image root partition lacks a usable PARTUUID'
[[ -x "$depmod_command" ]] || fail "required tool is unavailable: $depmod_command"
if [[ -z "$prepare_output" ]]; then
    for runtime_path in \
        /usr/bin/cage \
        /usr/bin/Xwayland \
        /usr/bin/xwayland-run \
        /usr/sbin/greetd \
        /usr/lib/systemd/system/greetd.service; do
        guestfish --ro -a "$image" run : mount-ro /dev/sda2 / : is-file "$runtime_path" | grep -qx true ||
            fail "image lacks required graphical runtime: $runtime_path"
    done
else
    timeout_seconds="${RPI_QEMU_PREPARE_TIMEOUT_SECONDS:-1800}"
    [[ "$(realpath -m "$prepare_output")" != "$(realpath "$image")" ]] || fail 'prepared output must differ from its source image'
fi

mkdir -p "$cache_dir"
if [[ ! -s "$cloud_sums" ]]; then
    curl -fL "$sums_url" -o "$cloud_sums"
fi
if [[ ! -s "$kernel_package" ]] || [[ "$(sha256sum "$kernel_package" | awk '{print $1}')" != "$kernel_package_sha256" ]]; then
    tmp_package="$kernel_package.new"
    rm -f "$tmp_package"
    curl -fL "$kernel_package_url" -o "$tmp_package"
    [[ "$(sha256sum "$tmp_package" | awk '{print $1}')" == "$kernel_package_sha256" ]] || fail 'downloaded Debian generic ARM64 kernel failed SHA-256 verification'
    mv "$tmp_package" "$kernel_package"
fi
kernel_package="$(realpath "$kernel_package")"
expected_sum="$(awk '$2 == "debian-12-genericcloud-arm64.qcow2" || $2 == "*debian-12-genericcloud-arm64.qcow2" { print $1; exit }' "$cloud_sums")"
[[ "$expected_sum" =~ ^[0-9a-fA-F]{128}$ ]] || fail 'official SHA512SUMS lacks the ARM64 generic cloud image'
if [[ ! -s "$cloud_image" ]] || [[ "$(sha512sum "$cloud_image" | awk '{print $1}')" != "$expected_sum" ]]; then
    tmp_download="$cloud_image.new"
    rm -f "$tmp_download"
    curl -fL "$cloud_url" -o "$tmp_download"
    [[ "$(sha512sum "$tmp_download" | awk '{print $1}')" == "$expected_sum" ]] || fail 'downloaded Debian ARM64 cloud image failed SHA-512 verification'
    mv "$tmp_download" "$cloud_image"
fi

workdir="$(mktemp -d)"
cleanup() {
    rm -rf "$workdir"
}
trap cleanup EXIT INT TERM

mapfile -t boot_files < <(guestfish --ro -a "$cloud_image" -i find /boot)
cloud_initrd_rel="$(printf '%s\n' "${boot_files[@]}" | awk '/^\/?initrd\.img-[^/]+$/ { sub(/^\//, ""); print; exit }')"
[[ -n "$cloud_initrd_rel" ]] || fail 'Debian ARM64 cloud image lacks a versioned initramfs'
version="6.1.0-50-arm64"
package_root="$workdir/kernel-package"
mkdir -p "$package_root"
(cd "$package_root" && ar p "$kernel_package" data.tar.xz | xz -dc | tar -xf -)
kernel="$package_root/boot/vmlinuz-$version"
modules="$package_root/lib/modules"
[[ -f "$kernel" && -d "$modules/$version" ]] || fail 'generic ARM64 kernel package lacks its kernel or module tree'
"$depmod_command" -b "$package_root" -m /lib/modules "$version"

cloud_initrd="$workdir/cloud-initrd"
initrd_root="$workdir/initrd-root"
initrd="$workdir/initrd.img-$version"
mkdir -p "$initrd_root"
guestfish --ro -a "$cloud_image" -i download "/boot/$cloud_initrd_rel" "$cloud_initrd"
(cd "$initrd_root" && zstd -q -dc "$cloud_initrd" | cpio --quiet -idmu)
rm -rf "$initrd_root/lib/modules"
mkdir -p "$initrd_root/lib/modules"
cp -a "$modules/$version" "$initrd_root/lib/modules/"
mkdir -p "$initrd_root/conf"
printf '%s\n' virtio_pci virtio_blk ext4 >"$initrd_root/conf/modules"
(cd "$initrd_root" && find . -print0 | cpio --quiet --null -o -H newc | zstd -q -19 -o "$initrd")

overlay="$workdir/pi-visual.qcow2"
qemu-img create -q -f qcow2 -F raw -b "$(realpath "$image")" "$overlay"
if [[ -n "$prepare_output" ]]; then
    qemu-img resize -q "$overlay" +4G
    guestfish --rw -a "$overlay" run : part-resize /dev/sda 2 -1
fi
guestfish --rw -a "$overlay" run : mount /dev/sda2 / : copy-in "$modules/$version" /lib/modules
guestfish --rw -a "$overlay" run : mount /dev/sda2 / : \
    mkdir-p /etc/modules-load.d : write /etc/modules-load.d/ie-qemu-virt.conf $'virtio_gpu\nvirtio_net\n' : \
    mkdir-p /etc/systemd/system/apparmor.service.d : \
    write /etc/systemd/system/apparmor.service.d/ie-qemu-console.conf $'[Service]\nStandardOutput=journal+console\nStandardError=journal+console\n' : \
    mkdir-p /etc/systemd/system/greetd.service.d : \
    write /etc/systemd/system/greetd.service.d/ie-qemu-console.conf $'[Service]\nStandardOutput=journal+console\nStandardError=journal+console\n' : \
    touch /var/lib/ie-qemu-visual : \
    rm-f /var/ie/state/ie-session.log : \
    rm-f /var/ie/state/intuition-engine.log : \
    rm-f /var/ie/state/xwayland.log
if [[ -n "$prepare_output" ]]; then
    guestfish --rw -a "$overlay" run : mount /dev/sda2 / : \
        copy-in "$appliance_assets/ie-qemu-prepare-golden.sh" /usr/local/sbin/ : \
        chmod 0755 /usr/local/sbin/ie-qemu-prepare-golden.sh : \
        copy-in "$appliance_assets/ie-qemu-prepare-golden.service" /etc/systemd/system/ : \
        mkdir-p /etc/systemd/system/multi-user.target.wants : \
        rm-f /var/lib/ie-qemu-golden-prepared : \
        rm-f /etc/systemd/system/cloud-init-main.service : \
        rm-f /etc/systemd/system/cloud-init-local.service : \
        rm-f /etc/systemd/system/cloud-init-network.service : \
        rm-f /etc/systemd/system/cloud-init-hotplugd.socket : \
        ln-s /dev/null /etc/systemd/system/cloud-init-main.service : \
        ln-s /dev/null /etc/systemd/system/cloud-init-local.service : \
        ln-s /dev/null /etc/systemd/system/cloud-init-network.service : \
        ln-s /dev/null /etc/systemd/system/cloud-init-hotplugd.socket : \
        rm-f /etc/systemd/system/multi-user.target.wants/ie-qemu-prepare-golden.service : \
        ln-s /etc/systemd/system/ie-qemu-prepare-golden.service /etc/systemd/system/multi-user.target.wants/ie-qemu-prepare-golden.service
fi
log="${image}.qemu-serial.log"
stderr_log="${image}.qemu-stderr.log"
monitor_socket="${image}.qemu-monitor.sock"
: >"$log"
: >"$stderr_log"
rm -f "$monitor_socket"
if [[ -n "$prepare_output" ]]; then
    echo "rpi4-live-qemu: preparing ARM64 golden image under QEMU; timeout ${timeout_seconds} seconds"
    display_args=(-display none)
else
    echo "rpi4-live-qemu: opening graphical ARM64 appliance console; waiting up to ${timeout_seconds} seconds for IE64 BASIC"
    display_args=(-display gtk)
fi
set +e
timeout "${timeout_seconds}s" qemu-system-aarch64 \
    -M virt -cpu cortex-a72 -smp 4 -m 2G \
    -blockdev "driver=file,filename=$(realpath "$image"),node-name=backing,read-only=on" \
    -blockdev "driver=raw,file=backing,node-name=backing-raw,read-only=on" \
    -blockdev "driver=file,filename=$overlay,node-name=overlay-file" \
    -blockdev "driver=qcow2,file=overlay-file,backing=backing-raw,node-name=system" \
    -device virtio-blk-pci,drive=system \
    -device virtio-gpu-pci \
    -netdev user,id=net0 -device virtio-net-pci,netdev=net0 \
    -device qemu-xhci -device usb-kbd -device usb-tablet \
    -kernel "$kernel" -initrd "$initrd" \
    -append "console=ttyAMA0,115200 root=PARTUUID=$root_partuuid rootfstype=ext4 rootwait rw apparmor=1 security=apparmor lsm=landlock,lockdown,yama,integrity,apparmor,bpf" \
    "${display_args[@]}" -monitor "unix:$monitor_socket,server,nowait" \
    -serial "file:$log" 2>"$stderr_log"
status=$?
set -e
rm -f "$monitor_socket"
session_log="$workdir/ie-session.log"
if guestfish --ro -a "$overlay" run : mount-ro /dev/sda2 / : is-file /var/ie/state/ie-session.log | grep -qx true; then
    guestfish --ro -a "$overlay" run : mount-ro /dev/sda2 / : download /var/ie/state/ie-session.log "$session_log"
    echo 'rpi4-live-qemu: appliance session log follows:' >&2
    tail -n 120 "$session_log" >&2
fi
engine_log="$workdir/intuition-engine.log"
if guestfish --ro -a "$overlay" run : mount-ro /dev/sda2 / : is-file /var/ie/state/intuition-engine.log | grep -qx true; then
    guestfish --ro -a "$overlay" run : mount-ro /dev/sda2 / : download /var/ie/state/intuition-engine.log "$engine_log"
    echo 'rpi4-live-qemu: IntuitionEngine log follows:' >&2
    tail -n 120 "$engine_log" >&2
fi
xwayland_log="$workdir/xwayland.log"
if guestfish --ro -a "$overlay" run : mount-ro /dev/sda2 / : is-file /var/ie/state/xwayland.log | grep -qx true; then
    guestfish --ro -a "$overlay" run : mount-ro /dev/sda2 / : download /var/ie/state/xwayland.log "$xwayland_log"
    echo 'rpi4-live-qemu: Xwayland log follows:' >&2
    tail -n 120 "$xwayland_log" >&2
fi

if [[ -n "$prepare_output" ]]; then
    [[ "$status" -eq 0 ]] || fail "golden preparation QEMU exited with status $status; logs retained at $log and $stderr_log"
    guestfish --ro -a "$overlay" run : mount-ro /dev/sda2 / : is-file /var/lib/ie-qemu-golden-prepared | grep -qx true ||
        fail "golden preparation did not complete; logs retained at $log and $stderr_log"
    guestfish --rw -a "$overlay" run : mount /dev/sda2 / : \
        rm-f /etc/systemd/system/multi-user.target.wants/ie-qemu-prepare-golden.service : \
        rm-f /etc/systemd/system/ie-qemu-prepare-golden.service : \
        rm-f /usr/local/sbin/ie-qemu-prepare-golden.sh : \
        rm-f /var/lib/ie-qemu-golden-prepared : \
        rm-f /var/lib/ie-qemu-visual : \
        rm-f /etc/modules-load.d/ie-qemu-virt.conf : \
        rm-f /etc/systemd/system/apparmor.service.d/ie-qemu-console.conf : \
        rm-f /etc/systemd/system/greetd.service.d/ie-qemu-console.conf : \
        rm-rf "/lib/modules/$version"
    mkdir -p "$(dirname "$prepare_output")"
    qemu-img convert -q -f qcow2 -O raw "$overlay" "$prepare_output"
    echo "rpi4-live-qemu: prepared golden image written to $prepare_output"
    exit 0
fi

if [[ -f "$session_log" ]] && rg -q 'Intuition Engine greetd session started' "$session_log" &&
    [[ -f "$engine_log" ]] && rg -q 'Starting IE64 BASIC' "$engine_log"; then
    echo "rpi4-live-qemu: IntuitionEngine reached IE64 BASIC; graphical framebuffer confirmation remains visible in the QEMU window"
    exit 0
fi
if [[ "$status" -eq 124 ]]; then
    fail "timeout before the appliance session marker; logs retained at $log and $stderr_log"
fi
fail "QEMU exited with status $status before the appliance session marker; logs retained at $log and $stderr_log"
