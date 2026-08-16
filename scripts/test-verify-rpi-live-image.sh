#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
verifier="$root_dir/scripts/verify_rpi_live_image.sh"
fail() { echo "FAIL: $*" >&2; exit 1; }
[[ -x "$verifier" ]] || fail "missing executable verifier"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/tools"
printf '\177ELF\002\001\001\000\000\000\000\000\000\000\000\000\002\000\267\000\001\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000' >"$tmp/ie-arm64"
printf 'image\n' >"$tmp/image.img"

cat >"$tmp/tools/guestfish" <<'EOF'
#!/usr/bin/env bash
case " $* " in
  *' is-file '*) echo true ;;
  *' is-symlink '*) echo true ;;
  *' list-partitions '*) printf '%s\n' /dev/sda1 /dev/sda2 /dev/sda3 ;;
  *' download /etc/greetd/config.toml '*) printf '%s\n' '[initial_session]' 'command = "/opt/ie/ie-session.sh"' '[default_session]' 'command = "/usr/sbin/agreety --cmd /opt/ie/ie-session.sh"' >"${!#}" ;;
  *' download '*) cp "$FAKE_BINARY" "${!#}" ;;
esac
EOF
chmod +x "$tmp/tools/guestfish"

expect_fail() {
    if "$@" >/dev/null 2>&1; then fail "unexpected success: $*"; fi
}

expect_fail env PATH="$tmp/tools:$PATH" "$verifier"
expect_fail env PATH="$tmp/tools:$PATH" "$verifier" --board wrong --image "$tmp/image.img" --binary "$tmp/ie-arm64"
env PATH="$tmp/tools:$PATH" RPI_TEST_MODE=1 RPI_SKIP_PACKAGE_VERIFY=1 FAKE_BINARY="$tmp/ie-arm64" "$verifier" --board pi4 --image "$tmp/image.img" --binary "$tmp/ie-arm64" --share
printf 'other\n' >"$tmp/not-the-binary"
expect_fail env PATH="$tmp/tools:$PATH" RPI_TEST_MODE=1 RPI_SKIP_PACKAGE_VERIFY=1 FAKE_BINARY="$tmp/not-the-binary" "$verifier" --board pi4 --image "$tmp/image.img" --binary "$tmp/ie-arm64" --share

echo "Raspberry Pi post-build verification contracts passed"
