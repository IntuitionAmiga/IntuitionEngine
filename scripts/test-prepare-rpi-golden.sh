#!/usr/bin/env bash
set -euo pipefail
root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
script="$root_dir/scripts/prepare_rpi_golden.sh"
[[ -x "$script" ]] || { echo 'missing native golden preparation script' >&2; exit 1; }
rg -q 'uname -m.*aarch64' "$script" || { echo 'native architecture guard missing' >&2; exit 1; }
rg -q 'apt-get purge -y openbox obconf' "$script" || { echo 'obsolete Openbox purge missing' >&2; exit 1; }
if rg -q 'usermod .* pi|groupmod .* pi' "$script"; then
    echo 'native preparation renames the potentially active pi account' >&2
    exit 1
fi
rg -q 'groupadd ie' "$script" || { echo 'dedicated IE group setup missing' >&2; exit 1; }
rg -q 'useradd -m -g ie -s /bin/sh ie' "$script" || { echo 'dedicated IE account setup missing' >&2; exit 1; }
rg -q 'usermod -s /bin/sh ie' "$script" || { echo 'IE appliance account login shell setup missing' >&2; exit 1; }
rg -q 'polkitd ufw dosfstools' "$script" || { echo 'firewall package setup missing' >&2; exit 1; }
rg -q 'apt-get install -y .*jackd2 rtkit ' "$script" || { echo 'JACK runtime package provisioning missing' >&2; exit 1; }
rg -q 'command -v jackd' "$script" || { echo 'post-install jackd validation missing' >&2; exit 1; }
rg -q 'command -v rtkit-daemon' "$script" || { echo 'post-install rtkit validation missing' >&2; exit 1; }
rg -q 'running kernel is not PREEMPT_RT' "$script" || { echo 'PREEMPT_RT identity validation missing' >&2; exit 1; }
rg -q 'dpkg-query -W linux-image-rpi-v8-rt' "$script" || { echo 'installed RT kernel package validation missing' >&2; exit 1; }
if rg -q 'apt-get install[^\n]*linux-image-rpi-v8-rt' "$script"; then
    echo 'native preparation can upgrade the preserved RT boot stack' >&2
    exit 1
fi
rg -q 'boot_stack_before=' "$script" || { echo 'boot-stack preservation snapshot missing' >&2; exit 1; }
rg -q 'native preparation changed the Raspberry Pi boot stack' "$script" || { echo 'boot-stack preservation validation missing' >&2; exit 1; }
rg -q 'for command in cage Xwayland xwayland-run greetd nmcli ufw apparmor_parser growpart' "$script" || { echo 'appliance-runtime validation missing' >&2; exit 1; }
rg -q 'VERSION_CODENAME.*trixie' "$script" || { echo 'Trixie golden guard missing' >&2; exit 1; }
if rg -q 'rebuild-golden' "$script"; then echo 'native script exposes forbidden rebuild-golden mode' >&2; exit 1; fi
echo 'Raspberry Pi native golden-preparation contracts passed'
