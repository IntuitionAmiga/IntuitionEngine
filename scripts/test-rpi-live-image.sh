#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
builder="${root_dir}/scripts/build_rpi_live_image.sh"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

[[ -x "$builder" ]] || fail "missing executable Raspberry Pi image builder: $builder"
apparmor="${root_dir}/scripts/rpi-live/opt.ie.IntuitionEngine"
[[ -f "$apparmor" ]] || fail "missing Raspberry Pi AppArmor policy"
rg -q '/usr/bin/jackd Px -> ie-jackd' "$apparmor" || fail "IE policy lacks JACK child transition"
rg -q '/usr/libexec/intuitionengine-host-helper Px' "$apparmor" || fail "IE policy lacks HOST-helper child transition"
rg -q 'aarch64-linux-gnu' "$apparmor" || fail "IE policy lacks ARM64 library paths"
if rg -q 'x86_64-linux-gnu' "$apparmor"; then
    fail "IE policy contains x86-only library paths"
fi
[[ -f "${root_dir}/scripts/rpi-live/usr.libexec.intuitionengine-host-helper" ]] || fail "missing HOST-helper AppArmor policy"
launch="${root_dir}/scripts/rpi-live/ie-launch.sh"
session="${root_dir}/scripts/rpi-live/ie-session.sh"
rg -q 'IE_AUDIO_BACKEND=jack' "$launch" || fail "Pi launch script does not select JACK"
rg -q 'IE_JACK_ALSA_DEVICE=.*IE_JACK_ALSA_CARD' "$launch" || fail "Pi launch script does not pass selected ALSA card to JACK"
rg -q 'xwayland-run' "$session" || fail "Pi session does not start Xwayland"
rg -q 'Intuition Engine greetd session started' "$session" || fail "Pi session lacks QEMU-visible greetd marker"
rg -q -- '-aros-drive' "$launch" || fail "Pi launch script does not expose AROS payload"
rg -q -- '-intuitionos-root' "$launch" || fail "Pi launch script does not expose IntuitionOS payload"
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
os_release=Debian GNU/Linux 12 (bookworm)
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
cat >"$tmp/tools/guestfish" <<'EOF'
#!/usr/bin/env bash
[[ -z "${GUESTFISH_LOG:-}" ]] || printf '%s\n' "$*" >>"$GUESTFISH_LOG"
if [[ "$*" == *part-get-parttype* ]]; then
    echo msdos
elif [[ "$*" == *'cat /etc/os-release'* ]]; then
    printf '%s\n' 'PRETTY_NAME="Debian GNU/Linux 12 (bookworm)"'
elif [[ "$*" == *'is-file '* ]]; then
    if [[ -n "${GUESTFISH_MISSING_PATH:-}" && " $* " == *" ${GUESTFISH_MISSING_PATH} "* ]]; then
        echo false
    else
        echo true
    fi
elif [[ "$*" == *'is-symlink '* ]]; then
    echo true
elif [[ "$*" == *'download '* ]]; then
    if [[ -n "${FAKE_BINARY:-}" ]]; then cp "$FAKE_BINARY" "${!#}"; else touch "${!#}"; fi
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
printf 'not an arm64 executable\n' >"$tmp/bad-binary"
expect_fail env PATH="$tmp/tools:$PATH" "$builder" --board pi4 --binary "$tmp/bad-binary" --output "$tmp/out.img" --golden "$tmp/golden.img" --manifest "$tmp/golden.manifest" --payload "$tmp/payload" --check-payload

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
rg -q 'ie-host-helper.service.requires/ie-apparmor.service' "$guestfish_log" || fail "builder does not enable AppArmor enforcement for the host helper"
rg -q 'golden_has_any_file /usr/libexec/rtkit-daemon /usr/bin/rtkit-daemon' "$builder" || fail "builder does not accept Debian RTKit's libexec path"
rg -q 'mount-ro /dev/sda1' "$builder" || fail "builder does not validate Raspberry Pi firmware from the boot partition"
if rg -q 'mkdir-p /opt/ie /' "$builder" || rg -q 'mkdir-p /etc/greetd /' "$builder"; then
    fail "builder passes multiple paths to guestfish mkdir-p"
fi
if rg -q 'chmod 0755 /opt/ie/IntuitionEngine /' "$builder" || rg -q 'chown 1000 1000 /var/ie /' "$builder"; then
    fail "builder passes multiple paths to single-path guestfish operations"
fi
if rg -q 'LABEL=IESHARE' "$guestfish_log"; then
    fail "--no-share build configured a nonexistent IESHARE filesystem"
fi
[[ "$(sha256sum "$tmp/golden.img" | awk '{print $1}')" == "$golden_sum" ]] || fail "normal builder modified its source golden image"

env PATH="$tmp/tools:$PATH" RPI_GUESTFISH="$tmp/tools/guestfish" FAKE_BINARY="$tmp/ie-arm64" RPI_HOST_HELPER="$tmp/host-helper" "$builder" --board pi4 --binary "$tmp/ie-arm64" --output "$tmp/archive-layout.img" --golden "$tmp/golden.img" --manifest "$tmp/golden.manifest" --payload "$tmp/payload" --work-dir "$tmp/work" --no-share
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
