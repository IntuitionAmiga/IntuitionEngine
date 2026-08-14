#!/bin/sh
set -eu

export DEBIAN_FRONTEND=noninteractive
export APT_LISTCHANGES_FRONTEND=none
export DEBCONF_DB_REPLACE='File{filename:/var/cache/debconf/ie-golden.dat}'
systemctl mask --now apt-listchanges.service apt-listchanges.timer 2>/dev/null || true
rm -f /etc/apt/apt.conf.d/20listchanges
systemctl mask --now cloud-init-main.service cloud-init-hotplugd.socket \
    cloud-init-local.service cloud-init-network.service cloud-config.service cloud-final.service \
    2>/dev/null || true
systemctl mask --now userconfig.service 2>/dev/null || true
pkill -f '/usr/sbin/dpkg-reconfigure -p critical keyboard-configuration' 2>/dev/null || true
root_device="$(findmnt -n -o SOURCE /)"
resize2fs "$root_device"
. /etc/os-release
[ "${VERSION_CODENAME:-}" = trixie ] || { echo 'golden preparation requires Debian Trixie' >&2; exit 1; }
apt-get update
apt-get install -y \
    linux-image-rpi-v8-rt jackd2 rtkit cage xwayland xwayland-run greetd network-manager fonts-dejavu-core kbd \
    apparmor apparmor-utils polkitd ufw dosfstools cloud-guest-utils
sed -i '/^kernel=/d; /^initramfs[[:space:]]/d' /boot/firmware/config.txt
printf '\nkernel=kernel8_rt.img\ninitramfs initramfs8_rt followkernel\n' >>/boot/firmware/config.txt
apt-get purge -y openbox obconf || true
apt-get clean
rm -rf /var/lib/apt/lists/*

unset DEBCONF_DB_REPLACE
debconf_lock=/var/cache/debconf/config.dat
debconf_wait=0
while debconf_lock_pids="$(lslocks --notruncate -n -o PID,PATH | awk -v path="$debconf_lock" '$2 == path { print $1 }')" &&
    [ -n "$debconf_lock_pids" ]; do
    debconf_wait=$((debconf_wait + 1))
    echo "waiting for debconf database lock held by PID(s): $debconf_lock_pids" >&2
    ps -fp $debconf_lock_pids >&2 || true
    pkill -f '/usr/sbin/dpkg-reconfigure -p critical keyboard-configuration' 2>/dev/null || true
    [ "$debconf_wait" -lt 150 ] || { echo 'debconf database remained locked for five minutes' >&2; exit 1; }
    sleep 2
done
printf '%s\n' \
    'keyboard-configuration keyboard-configuration/layout select English (UK)' \
    'keyboard-configuration keyboard-configuration/layoutcode string gb' \
    'keyboard-configuration keyboard-configuration/model select Generic 105-key (Intl) PC' \
    'keyboard-configuration keyboard-configuration/modelcode string pc105' \
    'keyboard-configuration keyboard-configuration/variant select English (UK)' \
    'keyboard-configuration keyboard-configuration/variantcode string' \
    'keyboard-configuration keyboard-configuration/optionscode string' \
    'keyboard-configuration keyboard-configuration/xkb-keymap select gb' \
    'keyboard-configuration keyboard-configuration/store_defaults_in_debconf_db boolean true' |
    debconf-set-selections
DEBIAN_FRONTEND=noninteractive dpkg-reconfigure keyboard-configuration

uid_1000_owner="$(getent passwd 1000 | cut -d: -f1 || true)"
gid_1000_owner="$(getent group 1000 | cut -d: -f1 || true)"
if id ie >/dev/null 2>&1; then
    :
elif [ "$uid_1000_owner" = pi ] && [ "$gid_1000_owner" = pi ]; then
    usermod -l ie -d /home/ie -m pi
    groupmod -n ie pi
elif [ -n "$uid_1000_owner" ]; then
    echo "UID 1000 is owned by unexpected account: $uid_1000_owner" >&2
    exit 1
else
    [ -z "$gid_1000_owner" ] || { echo "GID 1000 is owned by unexpected group: $gid_1000_owner" >&2; exit 1; }
    groupadd -g 1000 ie
    useradd -m -u 1000 -g ie -s /bin/sh ie
fi
supplementary_groups=""
for group in audio video input render seat; do
    if getent group "$group" >/dev/null; then
        supplementary_groups="${supplementary_groups:+$supplementary_groups,}$group"
    fi
done
usermod -G "$supplementary_groups" ie
usermod -s /bin/sh ie
passwd -l ie
rm -rf /home/ie/.ssh
rm -f /etc/sudoers.d/010_pi-nopasswd
test "$(id -u ie)" = 1000
test "$(id -g ie)" = 1000

for path in \
    /usr/bin/jackd /usr/bin/cage /usr/bin/Xwayland /usr/bin/xwayland-run /usr/sbin/greetd \
    /usr/lib/systemd/system/greetd.service /usr/sbin/apparmor_parser \
    /usr/sbin/ufw /usr/bin/growpart; do
    [ -f "$path" ] || { echo "missing prepared runtime: $path" >&2; exit 1; }
done
touch /var/lib/ie-qemu-golden-prepared
systemctl poweroff
