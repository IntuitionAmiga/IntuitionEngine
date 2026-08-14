#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
script="$root_dir/scripts/rpi4_virt_qemu.sh"
prepare_script="$root_dir/scripts/rpi-live/ie-qemu-prepare-golden.sh"
prepare_service="$root_dir/scripts/rpi-live/ie-qemu-prepare-golden.service"
fail() { echo "FAIL: $*" >&2; exit 1; }
[[ -x "$script" ]] || fail "missing executable Pi graphical QEMU script"
[[ -x "$prepare_script" ]] || fail "missing executable QEMU golden-preparation script"
[[ -f "$prepare_service" ]] || fail "missing QEMU golden-preparation service"

rg -q 'cloud\.debian\.org/images/cloud/bookworm/latest/debian-12-genericcloud-arm64\.qcow2' "$script" || fail "visual test lacks an official Debian ARM64 kernel source"
rg -q 'SHA512SUMS' "$script" || fail "visual test does not authenticate the downloaded kernel source"
rg -q 'linux-image-6\.1\.0-50-arm64_6\.1\.176-1_arm64\.deb' "$script" || fail "visual test does not use Debian's generic ARM64 kernel"
rg -q '914f75b57a8e165d85fb910c3e2dcc7000a05a9fea3f42f90b26e8dd590d5f06' "$script" || fail "visual test does not pin the generic kernel package checksum"
rg -q 'kernel_package=.*realpath' "$script" || fail "visual test does not canonicalise the cached kernel package path before changing directory"
rg -q 'depmod_command=/usr/sbin/depmod' "$script" || fail "visual test does not use depmod's standard absolute path"
rg -q '"\$depmod_command" -b .*package_root.*-m /lib/modules.*version' "$script" || fail "visual test does not generate module dependency metadata under the package module root"
for module in virtio_pci virtio_blk ext4 virtio_gpu; do
    rg -q "$module" "$script" || fail "visual test initramfs/module policy lacks $module"
done
rg -q 'qemu-img create.*-b.*image' "$script" || fail "visual test does not use an overlay on the release image"
rg -q 'copy-in.*modules' "$script" || fail "visual test does not inject matching kernel modules into its overlay"
rg -q 'modules-load\.d.*virtio_gpu' "$script" || fail "visual test does not load the virtio GPU module"
rg -Fq "write /etc/modules-load.d/ie-qemu-virt.conf \$'virtio_gpu\\nvirtio_net\\n'" "$script" || fail "visual test does not terminate QEMU module names with real newlines"
if rg -Fq 'write /etc/modules-load.d/ie-qemu-virt.conf "virtio_gpu\n"' "$script"; then
    fail "visual test writes a literal backslash-n into the modules-load file"
fi
rg -q 'StandardError=journal\+console' "$script" || fail "visual test does not expose AppArmor loader errors on serial"
rg -q 'greetd.service.d.*StandardError=journal\+console' "$script" || fail "visual test does not expose greetd session errors on serial"
rg -Fq '/var/lib/ie-qemu-visual' "$script" || fail "visual test does not mark its QEMU-only graphics environment"
for log in ie-session.log intuition-engine.log xwayland.log; do
    rg -Fq "rm-f /var/ie/state/$log" "$script" || fail "visual test does not clear inherited $log before boot"
done
rg -Fq '/var/lib/ie-qemu-visual' "$root_dir/scripts/rpi-live/ie-session.sh" || fail "appliance session does not recognise the QEMU graphics marker"
rg -Fq 'export SteamEnv=1' "$root_dir/scripts/rpi-live/ie-session.sh" || fail "QEMU session does not force Ebiten from Pi GLES to desktop GL"
rg -Fq 'export LIBGL_ALWAYS_SOFTWARE=1' "$root_dir/scripts/rpi-live/ie-session.sh" || fail "QEMU session does not select Mesa software rendering"
rg -q -- '-M virt' "$script" || fail "visual test does not use QEMU virt"
rg -q 'virtio-blk-pci' "$script" || fail "visual test does not expose the Pi image through virtio block"
rg -q 'root_partuuid=.*PART_ENTRY_UUID' "$script" || fail "visual test does not derive the supplied image root PARTUUID"
if rg -q 'root=PARTUUID=ffca5ce2-02' "$script"; then
    fail "visual test is hard-coded to the obsolete Bookworm golden partition UUID"
fi
rg -q 'virtio-gpu-pci' "$script" || fail "visual test does not expose DRM/KMS graphics"
rg -q 'usb-kbd' "$script" || fail "visual test does not expose keyboard input"
rg -q -- '-monitor "unix:\$monitor_socket,server,nowait"' "$script" || fail "visual test does not expose a private monitor for framebuffer capture"
rg -q 'node-name=backing,read-only=on' "$script" || fail "visual test does not open the release image read-only"
if rg -q 'source_sum=.*sha256sum.*image|sha256sum.*image.*source_sum' "$script"; then
    fail "visual test hashes the entire release image despite using a read-only base and disposable overlay"
fi
rg -q 'RPI_QEMU_PREPARE_GOLDEN_OUTPUT' "$script" || fail "QEMU launcher lacks golden-preparation mode"
rg -q 'rm-f /etc/systemd/system/multi-user.target.wants/ie-qemu-prepare-golden.service' "$script" || fail "golden preparation does not remove an existing service link"
rg -q 'ln-s /etc/systemd/system/ie-qemu-prepare-golden.service' "$script" || fail "golden preparation does not install its service link"
rg -q 'ln-s /dev/null /etc/systemd/system/cloud-init-main.service' "$script" || fail "golden preparation permits cloud-init to start before provisioning"
rg -q 'rm-f /var/lib/ie-qemu-golden-prepared' "$script" || fail "golden preparation accepts a stale success marker"
rg -q 'qemu-img resize.*overlay.*\+4G' "$script" || fail "golden preparation does not provide Trixie upgrade working space"
rg -q 'part-resize /dev/sda 2 -1' "$script" || fail "golden preparation does not expose the enlarged overlay partition before boot"
rg -Fq "awk '/^\\/?initrd\\.img-[^/]+$/ { sub(/^\\//, \"\"); print; exit }'" "$script" || fail "visual test does not normalise guestfish initramfs names with or without a leading slash"
rg -Fq 'download "/boot/$cloud_initrd_rel"' "$script" || fail "visual test does not reconstruct the initramfs path under /boot"
normalise_initrd() { awk '/^\/?initrd\.img-[^/]+$/ { sub(/^\//, ""); print; exit }'; }
[[ "$(printf '%s\n' '/initrd.img-test-arm64' | normalise_initrd)" == initrd.img-test-arm64 ]] || fail "initramfs normalisation rejects guestfish's leading-slash form"
[[ "$(printf '%s\n' 'initrd.img-test-arm64' | normalise_initrd)" == initrd.img-test-arm64 ]] || fail "initramfs normalisation rejects guestfish's prefix-free form"
rg -q 'virtio_net' "$script" || fail "golden-preparation mode lacks network module loading"
rg -q 'virtio-net-pci' "$script" || fail "golden-preparation mode lacks a network device"
rg -q 'ie-qemu-golden-prepared' "$script" || fail "golden-preparation mode does not require its success marker"
rg -q 'rm-f /etc/systemd/system/multi-user.target.wants/ie-qemu-prepare-golden.service' "$script" || fail "prepared golden retains the QEMU preparation service link"
rg -q 'rm-f /etc/systemd/system/ie-qemu-prepare-golden.service' "$script" || fail "prepared golden retains the QEMU preparation service"
rg -q 'rm-f /usr/local/sbin/ie-qemu-prepare-golden.sh' "$script" || fail "prepared golden retains the QEMU preparation helper"
rg -q 'rm-f /var/lib/ie-qemu-visual' "$script" || fail "prepared golden retains QEMU-only graphics policy"
rg -q 'rm-f /etc/modules-load.d/ie-qemu-virt.conf' "$script" || fail "prepared golden retains QEMU-only module loading policy"
rg -q 'rm-f /etc/systemd/system/apparmor.service.d/ie-qemu-console.conf' "$script" || fail "prepared golden retains the QEMU AppArmor console override"
rg -q 'rm-f /etc/systemd/system/greetd.service.d/ie-qemu-console.conf' "$script" || fail "prepared golden retains the QEMU greetd console override"
rg -Fq 'rm-rf "/lib/modules/$version"' "$script" || fail "prepared golden retains the injected generic QEMU module tree"
rg -q 'ie-session\.log.*session_log' "$script" || fail "visual test does not recover retained appliance-session diagnostics"
rg -q 'intuition-engine\.log.*engine_log' "$script" || fail "visual test does not recover retained IntuitionEngine diagnostics"
rg -q 'xwayland\.log.*xwayland_log' "$script" || fail "visual test does not recover retained Xwayland diagnostics"
rg -q 'qemu-img convert.*prepare_output' "$script" || fail "golden-preparation mode does not materialise the prepared image"
for package in linux-image-rpi-v8-rt jackd2 rtkit cage xwayland xwayland-run greetd network-manager fonts-dejavu-core kbd apparmor apparmor-utils polkitd ufw dosfstools cloud-guest-utils; do
    rg -qw "$package" "$prepare_script" || fail "golden preparation does not install declared runtime package: $package"
done
rg -q 'usermod -l ie -d /home/ie -m pi' "$prepare_script" || fail "QEMU golden preparation does not migrate the Trixie pi account"
rg -q 'groupmod -n ie pi' "$prepare_script" || fail "QEMU golden preparation does not migrate the Trixie pi group"
rg -q 'UID 1000 is owned by unexpected account' "$prepare_script" || fail "QEMU golden preparation can overwrite an unexpected UID 1000 account"
rg -q 'id -u ie' "$prepare_script" || fail "QEMU golden preparation does not validate the IE appliance account"
rg -q 'usermod -s /bin/sh ie' "$prepare_script" || fail "QEMU golden preparation leaves the IE appliance account unable to start a greetd session"
rg -Fq 'usermod -G "$supplementary_groups" ie' "$prepare_script" || fail "QEMU golden preparation does not replace inherited administrative groups"
if rg -q 'usermod -a -G.*ie' "$prepare_script"; then
    fail "QEMU golden preparation preserves inherited administrative groups"
fi
rg -Fq 'passwd -l ie' "$prepare_script" || fail "QEMU golden preparation preserves the migrated account password"
rg -Fq 'rm -rf /home/ie/.ssh' "$prepare_script" || fail "QEMU golden preparation preserves migrated SSH credentials"
rg -Fq 'rm -f /etc/sudoers.d/010_pi-nopasswd' "$prepare_script" || fail "QEMU golden preparation preserves Raspberry Pi passwordless sudo policy"
if sed -n '1,/apt-get install/p' "$prepare_script" | rg -q 'growpart'; then
    fail "Trixie preparation uses growpart before cloud-guest-utils is installed"
fi
rg -q 'resize2fs.*root_device' "$prepare_script" || fail "Trixie preparation does not grow the copied root filesystem"
if rg -q "VERSION_CODENAME=bookworm|s/bookworm/trixie|full-upgrade" "$prepare_script"; then
    fail "golden preparation retains the unsupported in-place Bookworm upgrade path"
fi
rg -Fq "kernel=kernel8_rt.img" "$prepare_script" || fail "QEMU golden preparation does not select the PREEMPT_RT kernel"
rg -Fq "initramfs initramfs8_rt followkernel" "$prepare_script" || fail "QEMU golden preparation does not select the PREEMPT_RT initramfs"
rg -q 'systemctl mask --now apt-listchanges.service apt-listchanges.timer' "$prepare_script" || fail "golden preparation permits apt-listchanges to race for the debconf database"
rg -q 'APT_LISTCHANGES_FRONTEND=none' "$prepare_script" || fail "golden preparation does not disable interactive apt changelogs"
rg -Fq "DEBCONF_DB_REPLACE='File{filename:/var/cache/debconf/ie-golden.dat}'" "$prepare_script" || fail "golden preparation shares Raspberry Pi OS first-boot debconf state"
rg -Fq 'keyboard-configuration/layout select English (UK)' "$prepare_script" || fail "golden preparation leaves keyboard layout interactive on first appliance boot"
rg -q 'debconf_lock=/var/cache/debconf/config.dat' "$prepare_script" || fail "golden preparation does not identify the system debconf database lock"
rg -q 'lslocks.*PID,PATH' "$prepare_script" || fail "golden preparation does not wait for the system debconf database lock"
rg -q 'debconf database remained locked' "$prepare_script" || fail "golden preparation has no bounded failure for a persistent debconf lock"
rg -q 'dpkg-reconfigure.*keyboard-configuration' "$prepare_script" || fail "golden preparation does not finalise keyboard configuration noninteractively"
rg -q 'rm -f /etc/apt/apt.conf.d/20listchanges' "$prepare_script" || fail "golden preparation leaves the apt-listchanges APT hook active during provisioning"
rg -q 'systemctl mask --now cloud-init-main.service cloud-init-hotplugd.socket' "$prepare_script" || fail "golden preparation permits cloud-init to race for the debconf database"
rg -q 'systemctl mask --now userconfig.service' "$prepare_script" || fail "golden preparation permits the Raspberry Pi first-boot wizard to respawn interactive configuration"
rg -Fq "pkill -f '/usr/sbin/dpkg-reconfigure -p critical keyboard-configuration'" "$prepare_script" || fail "golden preparation permits Raspberry Pi first-boot keyboard configuration to remain interactive"
[[ "$(rg -Fc "pkill -f '/usr/sbin/dpkg-reconfigure -p critical keyboard-configuration'" "$prepare_script")" -ge 2 ]] || fail "golden preparation does not stop a late-launched interactive keyboard configuration"
rg -q 'apt-get purge -y openbox obconf' "$prepare_script" || fail "Trixie golden preparation retains the obsolete Openbox detour"
rg -qw 'xwayland-run' "$prepare_script" || fail "Trixie golden preparation omits xwayland-run"
rg -q '^WantedBy=multi-user.target$' "$prepare_service" || fail "golden preparation is not enabled at boot"
rg -q '^TimeoutStartSec=30min$' "$prepare_service" || fail "golden preparation service timeout is too short for emulated RT-kernel installation"
rg -q 'RPI_QEMU_PREPARE_TIMEOUT_SECONDS:-1800' "$script" || fail "golden preparation QEMU timeout is too short for emulated package installation"
rg -q 'RPI_QEMU_TIMEOUT_SECONDS:-600' "$script" || fail "graphical smoke timeout is too short for the emulated first boot and JACK fallback"
for path in /usr/bin/cage /usr/bin/Xwayland /usr/bin/xwayland-run /usr/sbin/greetd /usr/lib/systemd/system/greetd.service; do
    rg -Fq "$path" "$script" || fail "visual test does not preflight required appliance runtime: $path"
done
rg -q 'Intuition Engine greetd session started' "$script" || fail "visual test lacks the appliance-session marker"
rg -q 'rg -q.*Intuition Engine greetd session started.*session_log' "$script" || fail "visual test does not recognise the retained session marker"
rg -q 'rg -q.*Starting IE64 BASIC.*engine_log' "$script" || fail "visual test can pass before IntuitionEngine reaches IE64 BASIC"
rg -q '^cage -s -- /opt/ie/ie-launch\.sh$' "$root_dir/scripts/rpi-live/ie-session.sh" || fail "QEMU appliance session bypasses Trixie Cage integrated Xwayland"
if rg -q 'cage .*xwayland-run' "$root_dir/scripts/rpi-live/ie-session.sh"; then
    fail "QEMU appliance session starts a conflicting second Xwayland server"
fi

echo 'Raspberry Pi graphical QEMU contracts passed'
