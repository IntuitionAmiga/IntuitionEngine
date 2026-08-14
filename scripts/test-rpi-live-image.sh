#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
builder="${root_dir}/scripts/build_rpi_live_image.sh"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

[[ -x "$builder" ]] || fail "missing executable Raspberry Pi image builder: $builder"
rg -q 'apparmor=1' "$builder" || fail "builder does not enable AppArmor in the physical Pi kernel command line"
rg -q 'cat /config.txt' "$builder" || fail "builder does not inspect Raspberry Pi boot kernel selection"
rg -Fq 'kernel=kernel8_rt.img' "$builder" || fail "builder does not require the PREEMPT_RT kernel to be selected"
rg -Fq 'initramfs initramfs8_rt followkernel' "$builder" || fail "builder does not require the PREEMPT_RT initramfs to be selected"
rg -q 'security=apparmor' "$builder" || fail "builder does not select AppArmor in the physical Pi kernel command line"
rg -q 'lsm=[^ ]*apparmor' "$builder" || fail "builder does not include AppArmor in the physical Pi LSM list"
rg -Fq "write-append /etc/fstab \$'LABEL=IESHARE /var/ie/share vfat defaults,nofail,x-systemd.device-timeout=10 0 0\\n'" "$builder" || \
    fail "builder does not append the IESHARE fstab entry with a real newline"
if rg -Fq '0 0\n"' "$builder"; then
    fail "builder writes a literal backslash-n into the IESHARE fstab entry"
fi
apparmor="${root_dir}/scripts/rpi-live/opt.ie.IntuitionEngine"
[[ -f "$apparmor" ]] || fail "missing Raspberry Pi AppArmor policy"
[[ "$(sed -n '1p' "$apparmor")" == '#include <tunables/global>' ]] || fail "IE policy lacks the required AppArmor global tunables preamble"
[[ "$(sed -n '1p' "${root_dir}/scripts/rpi-live/usr.libexec.intuitionengine-host-helper")" == '#include <tunables/global>' ]] || fail "HOST-helper policy lacks the required AppArmor global tunables preamble"
if sed -n '1,/^profile /p' "$apparmor" | rg -q 'abstractions/base'; then
    fail "IE policy includes abstractions outside a profile"
fi
rg -q '/usr/bin/jackd Px -> ie-jackd' "$apparmor" || fail "IE policy lacks JACK child transition"
rg -q '/usr/libexec/intuitionengine-host-helper Px' "$apparmor" || fail "IE policy lacks HOST-helper child transition"
rg -q 'aarch64-linux-gnu' "$apparmor" || fail "IE policy lacks ARM64 library paths"
rg -q '^  network unix stream,$' "$apparmor" || fail "IE policy blocks the X11 Unix socket"
rg -q '/tmp/\.X11-unix/\*\* rw,' "$apparmor" || fail "IE policy blocks the X11 socket path"
if rg -q 'x86_64-linux-gnu' "$apparmor"; then
    fail "IE policy contains x86-only library paths"
fi
[[ -f "${root_dir}/scripts/rpi-live/usr.libexec.intuitionengine-host-helper" ]] || fail "missing HOST-helper AppArmor policy"
host_helper_apparmor="${root_dir}/scripts/rpi-live/usr.libexec.intuitionengine-host-helper"
rg -Fq '/usr/bin/systemctl ixr,' "$host_helper_apparmor" || fail "HOST-helper policy blocks reboot and poweroff through systemctl"
rg -Fq '/usr/bin/apt-get Cx -> apt,' "$host_helper_apparmor" || fail "HOST-helper policy does not isolate package updates in an APT child profile"
rg -q '^  profile apt .*\{' "$host_helper_apparmor" || fail "HOST-helper policy lacks its APT child profile"
for path_rule in '/var/lib/apt/** rwkl,' '/var/cache/apt/** rwkl,' '/var/lib/dpkg/** rwkl,' '/var/cache/debconf/** rwkl,' '/var/log/apt/** rw,' '/var/log/dpkg.log rw,' '/etc/** rwkl,' '/usr/** rwkl,' '/boot/** rwkl,'; do
    rg -Fq "$path_rule" "$host_helper_apparmor" || fail "APT child policy lacks required rule: $path_rule"
done
rg -Fq 'dbus send bus=system,' "$host_helper_apparmor" || fail "HOST-helper policy blocks systemctl communication with systemd"
if [[ -x /usr/sbin/apparmor_parser ]]; then
    /usr/sbin/apparmor_parser -Q -K "$host_helper_apparmor" >/dev/null
fi
launch="${root_dir}/scripts/rpi-live/ie-launch.sh"
session="${root_dir}/scripts/rpi-live/ie-session.sh"
greetd_config="${root_dir}/scripts/rpi-live/greetd-config.toml"
rg -q '^\[initial_session\]$' "$greetd_config" || fail "Pi greetd config does not define an automatic initial appliance session"
rg -q '^command = "/opt/ie/ie-session.sh"$' "$greetd_config" || fail "Pi greetd initial session does not launch the appliance"
rg -q '^\[default_session\]$' "$greetd_config" || fail "Pi greetd config lacks a protocol-speaking fallback greeter"
rg -q '^command = "/usr/sbin/agreety --cmd /opt/ie/ie-session.sh"$' "$greetd_config" || fail "Pi greetd fallback bypasses agreety"
rg -q 'IE_AUDIO_BACKEND=jack' "$launch" || fail "Pi launch script does not select JACK"
rg -q 'IE_JACK_ALSA_DEVICE=.*IE_JACK_ALSA_CARD' "$launch" || fail "Pi launch script does not pass selected ALSA card to JACK"
rg -q '^cage -s -- /opt/ie/ie-launch\.sh$' "$session" || fail "Pi session does not use Trixie Cage's integrated Xwayland path"
if rg -q 'cage .*xwayland-run' "$session"; then
    fail "Pi session starts a second Xwayland server inside Trixie Cage"
fi
rg -q '^export WLR_RENDERER=pixman$' "$session" || fail "Pi session does not use wlroots software rendering"
rg -q '^export WLR_NO_HARDWARE_CURSORS=1$' "$session" || fail "Pi session does not avoid unsupported hardware cursors"
rg -q 'ie-session\.log' "$session" || fail "Pi session does not retain compositor and launch diagnostics"
[[ ! -e "$root_dir/scripts/rpi-live/ie-xwayland-run.sh" ]] || fail "Pi image retains the obsolete custom Xwayland/Openbox shim"
rg -q 'Intuition Engine greetd session started' "$session" || fail "Pi session lacks QEMU-visible greetd marker"
rg -q -- '-aros-drive' "$launch" || fail "Pi launch script does not expose AROS payload"
rg -q -- '-intuitionos-root' "$launch" || fail "Pi launch script does not expose IntuitionOS payload"
rg -q 'IntuitionEngine exited with status' "$launch" || fail "Pi launch script does not retain the engine exit status"
if rg -q '^exec /opt/ie/IntuitionEngine' "$launch"; then
    fail "Pi launch script discards the engine exit status"
fi
firewall="${root_dir}/scripts/rpi-live/ie-firewall.service"
rg -q '^DefaultDependencies=no$' "$firewall" || fail "Pi firewall has unsafe default ordering"
rg -q '^Before=network-pre.target network.target ' "$firewall" || fail "Pi firewall is not ordered before network activation"
rg -q '^WantedBy=network-pre.target$' "$firewall" || fail "Pi firewall is not pulled in by network-pre.target"
apparmor_service="${root_dir}/scripts/rpi-live/ie-apparmor.service"
[[ -f "$apparmor_service" ]] || fail "missing Pi AppArmor enforcement service"
rg -q '^Requires=apparmor.service$' "$apparmor_service" || fail "Pi AppArmor enforcement does not require AppArmor"
rg -q '^Before=greetd.service ie-host-helper.service$' "$apparmor_service" || fail "Pi AppArmor enforcement is not ordered before appliance services"
rg -q 'apparmor_parser -r -W /etc/apparmor.d/opt.ie.IntuitionEngine /etc/apparmor.d/usr.libexec.intuitionengine-host-helper' "$apparmor_service" || fail "Pi AppArmor enforcement does not load both policies"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/tools" "$tmp/work" "$tmp/payload/Systems" "$tmp/payload/Docs/IEProgRefGuide"
printf '\177ELF\002\001\001\000\000\000\000\000\000\000\000\000\002\000\267\000\001\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000' >"$tmp/ie-arm64"
cp "$tmp/ie-arm64" "$tmp/ie-arm64-pi5"
printf 'pi5' >>"$tmp/ie-arm64-pi5"
cp "$tmp/ie-arm64" "$tmp/host-helper"
printf 'payload\n' >"$tmp/payload/Systems/example"
printf 'manual\n' >"$tmp/payload/Docs/IE64_ISA.pdf"
printf 'reference guide\n' >"$tmp/payload/Docs/IEProgRefGuide/00-Preface.pdf"
printf 'golden\n' >"$tmp/golden.img"
golden_sum="$(sha256sum "$tmp/golden.img" | awk '{print $1}')"
cat >"$tmp/golden.manifest" <<EOF
sha256=$golden_sum
architecture=aarch64
partition_table=dos
os_release=Debian GNU/Linux 13 (trixie)
kernel=PREEMPT_RT
kernel_image=kernel8_rt.img
packages=jackd2,rtkit,cage,xwayland,xwayland-run,greetd,network-manager,fonts-dejavu-core,kbd,apparmor,apparmor-utils,polkitd,ufw,dosfstools,cloud-guest-utils
EOF
printf 'sha256=%s\n' "$golden_sum" >"$tmp/incomplete.manifest"

for tool in guestfish virt-copy-out virt-customize sfdisk mkfs.fat; do
    cat >"$tmp/tools/$tool" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
    chmod +x "$tmp/tools/$tool"
done

# Administrative tools are commonly installed outside an unprivileged user's
# PATH. The builder must find sfdisk in its standard system-tool directories.
mkdir -p "$tmp/admin-tools" "$tmp/path-without-sfdisk"
cp "$tmp/tools/sfdisk" "$tmp/admin-tools/sfdisk"
for tool in bash cat dirname readlink; do
    path="$(command -v "$tool")"
    ln -s "$path" "$tmp/path-without-sfdisk/$tool"
done
env PATH="$tmp/path-without-sfdisk" RPI_SYSTEM_TOOL_DIRS="$tmp/admin-tools" /bin/bash "$builder" --help >/dev/null

cat >"$tmp/tools/guestfish" <<'EOF'
#!/usr/bin/env bash
[[ -z "${GUESTFISH_LOG:-}" ]] || printf '%s\n' "$*" >>"$GUESTFISH_LOG"
if [[ "$*" == *part-get-parttype* ]]; then
    echo msdos
elif [[ "$*" == *'cat /etc/os-release'* ]]; then
    printf '%s\n' 'PRETTY_NAME="Debian GNU/Linux 13 (trixie)"'
elif [[ "$*" == *'cat /var/lib/dpkg/status'* ]]; then
    for package in jackd2 rtkit cage xwayland xwayland-run greetd network-manager fonts-dejavu-core kbd apparmor apparmor-utils polkitd ufw dosfstools cloud-guest-utils; do
        printf 'Package: %s\nStatus: install ok installed\n\n' "$package"
    done
elif [[ "$*" == *'cat /etc/passwd'* ]]; then
    ie_uid="${GUESTFISH_IE_UID:-1000}"
    ie_gid="${GUESTFISH_IE_GID:-$ie_uid}"
    if [[ "${GUESTFISH_BAD_IE_SHELL:-0}" == 1 ]]; then
        printf 'ie:x:%s:%s::/home/ie:/usr/sbin/nologin\n' "$ie_uid" "$ie_gid"
    else
        printf 'ie:x:%s:%s::/home/ie:/bin/sh\n' "$ie_uid" "$ie_gid"
    fi
elif [[ "$*" == *'cat /etc/group'* ]]; then
    printf 'ie:x:%s:\n' "${GUESTFISH_IE_GID:-${GUESTFISH_IE_UID:-1000}}"
elif [[ "$*" == *'cat /config.txt'* ]]; then
    if [[ "${GUESTFISH_BAD_BOOT:-0}" == 1 ]]; then
        printf '%s\n' 'kernel=kernel8.img' 'initramfs initramfs8 followkernel'
    else
        printf '%s\n' 'kernel=kernel8_rt.img' 'initramfs initramfs8_rt followkernel'
    fi
elif [[ "$*" == *'is-file '* ]]; then
    if [[ -n "${GUESTFISH_MISSING_PATH:-}" && " $* " == *" ${GUESTFISH_MISSING_PATH} "* ]]; then
        echo false
    else
        echo true
    fi
elif [[ "$*" == *'is-symlink '* ]]; then
    echo true
elif [[ "$*" == *list-partitions* && "${FAKE_SHARE:-0}" == "1" ]]; then
    echo /dev/sda3
elif [[ "$*" == *'download '* ]]; then
    if [[ "$*" == *'/etc/greetd/config.toml'* ]]; then
        printf '%s\n' '[initial_session]' 'command = "/opt/ie/ie-session.sh"' 'user = "ie"' '' \
            '[default_session]' 'command = "/usr/sbin/agreety --cmd /opt/ie/ie-session.sh"' 'user = "_greetd"' >"${!#}"
    elif [[ -n "${FAKE_BINARY:-}" ]]; then cp "$FAKE_BINARY" "${!#}"; else touch "${!#}"; fi
fi
exit 0
EOF
chmod +x "$tmp/tools/guestfish"
expect_fail() {
    if "$@" >/dev/null 2>&1; then
        fail "unexpected success: $*"
    fi
}

expect_fail env PATH="$tmp/tools:$PATH" "$builder"
expect_fail env PATH="$tmp/tools:$PATH" "$builder" --board wrong --binary "$tmp/ie-arm64" --output "$tmp/out.img" --golden "$tmp/golden.img" --manifest "$tmp/golden.manifest" --payload "$tmp/payload"
expect_fail env PATH="$tmp/tools:$PATH" RPI_GUESTFISH="$tmp/tools/guestfish" RPI_HOST_HELPER="$tmp/host-helper" "$builder" --board pi4 --binary "$tmp/ie-arm64" --output "$tmp/incomplete-manifest.img" --golden "$tmp/golden.img" --manifest "$tmp/incomplete.manifest" --payload "$tmp/payload" --work-dir "$tmp/work" --no-share
expect_fail env PATH="$tmp/tools:$PATH" "$builder" --board pi4 --binary "$tmp/ie-arm64" --output "$tmp/golden.img" --golden "$tmp/golden.img" --manifest "$tmp/golden.manifest" --payload "$tmp/payload"
ln -s "$tmp/golden.img" "$tmp/golden-alias.img"
expect_fail env PATH="$tmp/tools:$PATH" "$builder" --board pi4 --binary "$tmp/ie-arm64" --output "$tmp/golden-alias.img" --golden "$tmp/golden.img" --manifest "$tmp/golden.manifest" --payload "$tmp/payload"
printf 'tampered\n' >>"$tmp/golden.img"
expect_fail env PATH="$tmp/tools:$PATH" "$builder" --board pi4 --binary "$tmp/ie-arm64" --output "$tmp/out.img" --golden "$tmp/golden.img" --manifest "$tmp/golden.manifest" --payload "$tmp/payload"
printf 'golden\n' >"$tmp/golden.img"
expect_fail env PATH="$tmp/tools:$PATH" GUESTFISH_MISSING_PATH=/usr/bin/cage RPI_GUESTFISH="$tmp/tools/guestfish" RPI_HOST_HELPER="$tmp/host-helper" "$builder" --board pi4 --binary "$tmp/ie-arm64" --output "$tmp/missing-cage.img" --golden "$tmp/golden.img" --manifest "$tmp/golden.manifest" --payload "$tmp/payload" --work-dir "$tmp/work" --no-share
expect_fail env PATH="$tmp/tools:$PATH" GUESTFISH_BAD_BOOT=1 RPI_GUESTFISH="$tmp/tools/guestfish" RPI_HOST_HELPER="$tmp/host-helper" "$builder" --board pi4 --binary "$tmp/ie-arm64" --output "$tmp/non-rt-boot.img" --golden "$tmp/golden.img" --manifest "$tmp/golden.manifest" --payload "$tmp/payload" --work-dir "$tmp/work" --no-share
expect_fail env PATH="$tmp/tools:$PATH" GUESTFISH_BAD_IE_SHELL=1 RPI_GUESTFISH="$tmp/tools/guestfish" RPI_HOST_HELPER="$tmp/host-helper" "$builder" --board pi4 --binary "$tmp/ie-arm64" --output "$tmp/nologin-ie.img" --golden "$tmp/golden.img" --manifest "$tmp/golden.manifest" --payload "$tmp/payload" --work-dir "$tmp/work" --no-share
printf 'not an arm64 executable\n' >"$tmp/bad-binary"
expect_fail env PATH="$tmp/tools:$PATH" "$builder" --board pi4 --binary "$tmp/bad-binary" --output "$tmp/out.img" --golden "$tmp/golden.img" --manifest "$tmp/golden.manifest" --payload "$tmp/payload" --check-payload
if env PATH="$tmp/tools:$PATH" "$builder" --board pi5 --binary "$tmp/ie-arm64-pi5" --source-image "$tmp/golden.img" --output "$tmp/derived-no-share.img" --payload "$tmp/payload" --no-share 2>"$tmp/derived-no-share.err"; then
    fail "derived builder accepted --source-image with --no-share"
fi
rg -q -- '--source-image cannot be combined with --no-share' "$tmp/derived-no-share.err" || fail "derived builder did not explain rejected --no-share combination"

printf '\177ELF\002\001\001\000\000\000\000\000\000\000\000\000\002\000\267\000\001\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000' >"$tmp/ie-arm64"
env PATH="$tmp/tools:$PATH" "$builder" --board pi4 --binary "$tmp/ie-arm64" --output "$tmp/out.img" --golden "$tmp/golden.img" --manifest "$tmp/golden.manifest" --payload "$tmp/payload" --work-dir "$tmp/work" --check-payload
[[ "$(sha256sum "$tmp/golden.img" | awk '{print $1}')" == "$golden_sum" ]] || fail "builder modified its source golden image"
env PATH="$tmp/tools:$PATH" "$builder" --board pi4 --binary "$tmp/ie-arm64" --payload "$tmp/payload" --check-payload

# A normal no-share build exercises the mutating path with small fixtures. It
# must need the host helper, preserve its source image and use the requested
# output name. The guestfish fixture makes no filesystem changes by design.
expect_fail env PATH="$tmp/tools:$PATH" RPI_GUESTFISH="$tmp/tools/guestfish" RPI_HOST_HELPER="$tmp/missing-helper" "$builder" --board pi4 --binary "$tmp/ie-arm64" --output "$tmp/no-helper.img" --golden "$tmp/golden.img" --manifest "$tmp/golden.manifest" --payload "$tmp/payload" --work-dir "$tmp/work" --no-share
guestfish_log="$tmp/guestfish.log"
env PATH="$tmp/tools:$PATH" RPI_GUESTFISH="$tmp/tools/guestfish" GUESTFISH_LOG="$guestfish_log" FAKE_BINARY="$tmp/ie-arm64" RPI_HOST_HELPER="$tmp/host-helper" "$builder" --board pi4 --binary "$tmp/ie-arm64" --output "$tmp/board-output.img" --golden "$tmp/golden.img" --manifest "$tmp/golden.manifest" --payload "$tmp/payload" --work-dir "$tmp/work" --no-share
[[ -f "$tmp/board-output.img" && -f "$tmp/board-output.zip" ]] || fail "builder did not produce requested image and ZIP names"
rg -q 'mv /opt/ie/ie-arm64 /opt/ie/IntuitionEngine' "$guestfish_log" || fail "builder does not install the board binary under the appliance launch name"
rg -q 'mv /etc/greetd/greetd-config.toml /etc/greetd/config.toml' "$guestfish_log" || fail "builder does not install greetd's required config.toml"
rg -q 'greetd.service.requires/ie-apparmor.service' "$guestfish_log" || fail "builder does not enable AppArmor enforcement for greetd"
rg -q 'graphical.target.wants/greetd.service' "$guestfish_log" || fail "builder does not enable greetd in graphical.target"
rg -q 'rm-f /etc/systemd/system/default.target.*ln-s /usr/lib/systemd/system/graphical.target /etc/systemd/system/default.target' "$guestfish_log" || fail "builder does not select graphical.target as the default"
rg -q 'getty.target.wants/getty@tty1.service.*ln-s /dev/null /etc/systemd/system/getty@tty1.service' "$guestfish_log" || fail "builder does not prevent tty1 getty from competing with greetd"
rg -q 'ie-host-helper.service.requires/ie-apparmor.service' "$guestfish_log" || fail "builder does not enable AppArmor enforcement for the host helper"
rg -q 'golden_has_any_file /usr/libexec/rtkit-daemon /usr/bin/rtkit-daemon' "$builder" || fail "builder does not accept Debian RTKit's libexec path"
rg -q 'mount-ro /dev/sda1' "$builder" || fail "builder does not validate Raspberry Pi firmware from the boot partition"
if rg -q 'mkdir-p /opt/ie /' "$builder" || rg -q 'mkdir-p /etc/greetd /' "$builder"; then
    fail "builder passes multiple paths to guestfish mkdir-p"
fi
if rg -q 'chmod 0755 /opt/ie/IntuitionEngine /' "$builder" || rg -q 'chown 1000 1000 /var/ie /' "$builder"; then
    fail "builder passes multiple paths to single-path guestfish operations"
fi
dynamic_uid_log="$tmp/dynamic-uid-guestfish.log"
env PATH="$tmp/tools:$PATH" RPI_GUESTFISH="$tmp/tools/guestfish" GUESTFISH_LOG="$dynamic_uid_log" GUESTFISH_IE_UID=1001 GUESTFISH_IE_GID=1002 FAKE_BINARY="$tmp/ie-arm64" RPI_HOST_HELPER="$tmp/host-helper" "$builder" --board pi4 --binary "$tmp/ie-arm64" --output "$tmp/dynamic-uid.img" --golden "$tmp/golden.img" --manifest "$tmp/golden.manifest" --payload "$tmp/payload" --work-dir "$tmp/dynamic-work" --no-share
rg -q 'chown 1001 1002 /var/ie' "$dynamic_uid_log" || fail "builder hard-codes the IE appliance account ownership"
if rg -q 'LABEL=IESHARE' "$guestfish_log"; then
    fail "--no-share build configured a nonexistent IESHARE filesystem"
fi
[[ "$(sha256sum "$tmp/golden.img" | awk '{print $1}')" == "$golden_sum" ]] || fail "normal builder modified its source golden image"

# A derived image must clone the completed Pi 4/Pi 400 appliance, preserve its
# source image and replace only the board-specific Intuition Engine binary.
derived_source_sum="$(sha256sum "$tmp/board-output.img" | awk '{print $1}')"
derived_log="$tmp/derived-guestfish.log"
env PATH="$tmp/tools:$PATH" RPI_GUESTFISH="$tmp/tools/guestfish" GUESTFISH_LOG="$derived_log" FAKE_BINARY="$tmp/ie-arm64-pi5" FAKE_SHARE=1 "$builder" --board pi5 --binary "$tmp/ie-arm64-pi5" --source-image "$tmp/board-output.img" --output "$tmp/derived-output.img" --payload "$tmp/payload" --work-dir "$tmp/derived-work"
[[ -f "$tmp/derived-output.img" && -f "$tmp/derived-output.zip" ]] || fail "derived builder did not produce Pi 5 image and ZIP"
[[ "$(sha256sum "$tmp/board-output.img" | awk '{print $1}')" == "$derived_source_sum" ]] || fail "derived builder modified its source appliance image"
rg -q 'rm /opt/ie/IntuitionEngine.*copy-in .*ie-arm64-pi5.*mv /opt/ie/ie-arm64-pi5 /opt/ie/IntuitionEngine' "$derived_log" || fail "derived builder does not replace the board binary"
if rg -q 'copy-in .*ie-session\.sh|copy-in .*greetd-config\.toml|write-append /etc/fstab' "$derived_log"; then
    fail "derived builder rebuilt common appliance or IESHARE content"
fi

# Leave RPI_GUESTFISH unset so the production PATH lookup cannot recurse into
# the shell wrapper with the same name.
env PATH="$tmp/tools:$PATH" FAKE_BINARY="$tmp/ie-arm64" RPI_HOST_HELPER="$tmp/host-helper" "$builder" --board pi4 --binary "$tmp/ie-arm64" --output "$tmp/archive-layout.img" --golden "$tmp/golden.img" --manifest "$tmp/golden.manifest" --payload "$tmp/payload" --work-dir "$tmp/work" --no-share
python3 - "$tmp/archive-layout.zip" <<'PY' || fail "ZIP layout differs from x64 release layout"
import sys
import zipfile

with zipfile.ZipFile(sys.argv[1]) as archive:
    names = set(archive.namelist())
for required in {
    "archive-layout.img",
    "Docs/IE64_ISA.pdf",
    "Docs/IEProgRefMan/00-Preface.pdf",
}:
    if required not in names:
        raise SystemExit(f"missing ZIP entry: {required}")
if any(name.startswith("Docs/IEProgRefGuide/") for name in names):
    raise SystemExit("ZIP exposes staging-only IEProgRefGuide path")
PY

echo "Raspberry Pi image-builder contracts passed"
