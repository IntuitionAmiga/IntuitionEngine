#!/usr/bin/env bash
# Run natively on a copied Raspberry Pi OS ARM64 image, never on the build host.
set -euo pipefail

fail() { echo "prepare-rpi-golden: $*" >&2; exit 1; }
[[ "$(uname -m)" == aarch64 ]] || fail "must run natively on Raspberry Pi OS ARM64"
[[ $EUID -eq 0 ]] || fail "must run as root on the copied golden image"

kernel_before="$(uname -r)"
[[ "$kernel_before" == *rt* ]] || fail "running kernel is not PREEMPT_RT: $kernel_before"
boot_stack_before="$(sha256sum /boot/firmware/config.txt /boot/firmware/cmdline.txt /boot/firmware/kernel8_rt.img)"
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y jackd2 rtkit cage xwayland xwayland-run greetd network-manager fonts-dejavu-core kbd apparmor apparmor-utils polkitd ufw dosfstools cloud-guest-utils
DEBIAN_FRONTEND=noninteractive apt-get purge -y 'subsynth*' || true
rm -rf /opt/subsynth /var/lib/subsynth /etc/subsynth
getent group ie >/dev/null || groupadd -g 1000 ie
if ! id ie >/dev/null 2>&1; then
    useradd -m -u 1000 -g ie -G audio,video,input,render,seat -s /bin/sh ie
fi
[[ "$(uname -r)" == "$kernel_before" ]] || fail "native preparation changed the running RT kernel"
[[ "$(sha256sum /boot/firmware/config.txt /boot/firmware/cmdline.txt /boot/firmware/kernel8_rt.img)" == "$boot_stack_before" ]] || \
    fail "native preparation changed the Raspberry Pi boot stack"
dpkg-query -W jackd2 rtkit >/dev/null
command -v jackd >/dev/null || fail "jackd2 did not install jackd"
command -v rtkit-daemon >/dev/null || fail "rtkit did not install rtkit-daemon"
for command in cage Xwayland xwayland-run greetd nmcli ufw apparmor_parser growpart; do
    command -v "$command" >/dev/null || fail "required appliance command is unavailable: $command"
done
if grep -Eq 'video=1920x1080|GRUB|UEFI' /boot/firmware/cmdline.txt; then fail "Pi kernel command line contains x64 boot settings"; fi
shutdown -h now
