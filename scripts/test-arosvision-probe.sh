#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root_dir"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_file() {
  [[ -f "$1" ]] || fail "missing file: $1"
}

assert_contains() {
  local path="$1"
  local pattern="$2"
  rg -q "$pattern" "$path" || fail "$path does not match: $pattern"
}

assert_not_contains() {
  local path="$1"
  local pattern="$2"
  if rg -q "$pattern" "$path"; then
    fail "$path unexpectedly matches: $pattern"
  fi
}

tmp="$(mktemp -d)"
test_output="build/arosvision-probe/test/AROS"
trap 'rm -rf "$tmp" build/arosvision-probe/test' EXIT

source="$tmp/AROSVision"
mkdir -p "$source/S" "$source/System/Wanderer" "$source/C" "$source/Prefs/Env-Archive" "$source/WBStartup" "$source/Devs/Midi"
cat >"$source/S/Startup-Sequence" <<'SRCSTART'
Assign WANDERER: SYS:System/Wanderer DEFER
Execute S:User-Startup
C:apoke dead beef
C:vcontrol something
If EXISTS "SYS:Classes/USB"
    C:LoadModule SYS:Classes/USB/foo.class
EndIf
IF EXISTS "FONTS:__TEST__"
    Delete "FONTS:__TEST__"
EndIf
C:FPPrefs >NIL:
setenv workbench save 40.42
WANDERER:Wanderer
SRCSTART
cat >"$source/S/User-Startup" <<'SRCUSER'
Assign Work: SYS:Work
Mount GOOGLE:
Mount KCON: from DEVS:KingCON-mountlist
Mount KRAW: from DEVS:KingCON-mountlist
mount apipe:
exe <nil: >nil: l:fifo-handler
c:TaskPriHandler
run >NIL: >NIL: yaws
ntpSync de.pool.ntp.org
SRCUSER
touch "$source/System/Wanderer/Wanderer" "$source/C/IPrefs" "$source/C/AddDatatypes"
touch "$source/WBStartup/Clipper" "$source/WBStartup/Clipper.info" \
  "$source/WBStartup/DiskImageGUI" "$source/WBStartup/DiskImageGUI.info"
touch "$source/Devs/Midi/debugdriver" "$source/Devs/Midi/echo" \
  "$source/Devs/Midi/uaemidi" "$source/Devs/Midi/udp"

src_start_before="$(sha256sum "$source/S/Startup-Sequence" | awk '{print $1}')"
src_user_before="$(sha256sum "$source/S/User-Startup" | awk '{print $1}')"

scripts/prepare-arosvision-probe.sh "$source" "$test_output" >/tmp/arosvision-probe-test.out

[[ "$(sha256sum "$source/S/Startup-Sequence" | awk '{print $1}')" == "$src_start_before" ]] || \
  fail "source Startup-Sequence was modified"
[[ "$(sha256sum "$source/S/User-Startup" | awk '{print $1}')" == "$src_user_before" ]] || \
  fail "source User-Startup was modified"
[[ ! -f "$source/S/Startup-Sequence.AROSVision.orig" ]] || fail "source gained Startup-Sequence backup"
[[ ! -f "$source/S/User-Startup.AROSVision.orig" ]] || fail "source gained User-Startup backup"

out="$test_output"
assert_file "$out/S/Startup-Sequence"
assert_file "$out/S/Startup-Sequence.AROSVision.orig"
assert_file "$out/S/User-Startup"
assert_file "$out/S/User-Startup.AROSVision.orig"
assert_file "$out/Prefs/Env-Archive/SYS/screenmode.prefs"
assert_file "$out/Prefs/Env-Archive/SYS/ahi.prefs"
assert_file "$out/Storage/IEProbeDisabled/Devs/Midi/debugdriver"
assert_file "$out/Storage/IEProbeDisabled/Devs/Midi/echo"
assert_file "$out/Storage/IEProbeDisabled/Devs/Midi/uaemidi"
assert_file "$out/Storage/IEProbeDisabled/Devs/Midi/udp"
assert_file "$out/Devs/AudioModes/IE"
if [[ -f ../AROS-deadw00d/bin/ie-m68k/bin/ie-m68k/AROS/Devs/Midi/ie ]]; then
  assert_file "$out/Devs/Midi/ie"
fi
if [[ -f ../AROS-deadw00d/bin/ie-m68k/bin/ie-m68k/AROS/Devs/AHI/ie-audio.audio ]]; then
  assert_file "$out/Devs/AHI/ie-audio.audio"
  strings -a "$out/Devs/AHI/ie-audio.audio" | rg -q '^ie-audio\.audio$' || \
    fail "IE AHI driver resident name was not patched to ie-audio.audio"
  xxd -g1 -s 0x0f5c -l 16 "$out/Devs/AHI/ie-audio.audio" | rg -q '81 02 09' || \
    fail "IE AHI driver resident version was not patched to 2"
  xxd -g1 -s 0x02ee -l 4 "$out/Devs/AHI/ie-audio.audio" | rg -q '70 05 4e 75' || \
    fail "IE AHI driver playback guard was not applied"
fi
if [[ -f ../AROS-deadw00d/bin/ie-m68k/bin/ie-m68k/AROS/Libs/ie-audio.library ]]; then
  assert_file "$out/Libs/ie-audio.library"
  xxd -g1 -s 0x0f5c -l 16 "$out/Libs/ie-audio.library" | rg -q '81 02 09' || \
    fail "IE audio library resident version was not patched to 2"
  xxd -g1 -s 0x02ee -l 4 "$out/Libs/ie-audio.library" | rg -q '70 05 4e 75' || \
    fail "IE audio library playback guard was not applied"
fi
if [[ -f ../AROS-deadw00d/bin/ie-m68k/bin/ie-m68k/AROS/Libs/iewarp_service.ie64 ]]; then
  assert_file "$out/Systems/AROS/Libs/iewarp_service.ie64"
fi
if [[ -f ../AROS-deadw00d/bin/ie-m68k/bin/ie-m68k/AROS/Utilities/IEWarpMon ]]; then
  assert_file "$out/Utilities/IEWarpMon"
fi
if [[ -x ../AROS-deadw00d/bin/ie-m68k/bin/linux-x86_64/tools/ilbmtoicon && \
      -f ../AROS-deadw00d/images/IconSets/Mason/workbench/Utilities/IEWarpMon.info.src && \
      -f ../AROS-deadw00d/images/IconSets/Mason/workbench/Utilities/IEWarpMon.png ]]; then
  assert_file "$out/Utilities/IEWarpMon.info"
fi
assert_file "$out/WBStartup/Clipper"
assert_file "$out/WBStartup/Clipper.info"
assert_file "$out/WBStartup/DiskImageGUI"
assert_file "$out/WBStartup/DiskImageGUI.info"
[[ ! -e "$out/Devs/Midi/debugdriver" ]] || fail "debugdriver remained active in Devs/Midi"
[[ ! -e "$out/Devs/Midi/echo" ]] || fail "echo remained active in Devs/Midi"
[[ ! -e "$out/Devs/Midi/uaemidi" ]] || fail "uaemidi remained active in Devs/Midi"
[[ ! -e "$out/Devs/Midi/udp" ]] || fail "udp remained active in Devs/Midi"

cmp "$source/S/Startup-Sequence" "$out/S/Startup-Sequence.AROSVision.orig" >/dev/null || \
  fail "Startup-Sequence original was not preserved"
cmp "$source/S/User-Startup" "$out/S/User-Startup.AROSVision.orig" >/dev/null || \
  fail "User-Startup original was not preserved"

assert_contains "$out/S/Startup-Sequence" 'Assign WANDERER: SYS:System/Wanderer DEFER'
assert_contains "$out/S/Startup-Sequence" 'Assign "System:" "IE:"'
assert_contains "$out/S/Startup-Sequence" '; IE disabled: C:apoke dead beef'
assert_contains "$out/S/Startup-Sequence" '; IE disabled: C:vcontrol something'
assert_contains "$out/S/Startup-Sequence" '; IE disabled USB stack block:'
assert_contains "$out/S/Startup-Sequence" '; If EXISTS "SYS:Classes/USB"'
assert_contains "$out/S/Startup-Sequence" ';     C:LoadModule SYS:Classes/USB/foo.class'
assert_contains "$out/S/Startup-Sequence" 'IF EXISTS "SYS:Fonts/__TEST__"'
assert_contains "$out/S/Startup-Sequence" 'Delete "SYS:Fonts/__TEST__"'
assert_contains "$out/S/Startup-Sequence" 'WANDERER:Wanderer'
strings -a "$out/Devs/AudioModes/IE" | rg -q 'ie-audio' || fail "IE AudioModes file does not name ie-audio"
strings -a "$out/Devs/AudioModes/IE" | rg -q 'IE:16 bit stereo HiFi' || fail "IE AudioModes file lacks IE mode name"
xxd -g4 "$out/Devs/AudioModes/IE" | rg -q '00640002 0040' || fail "IE AudioModes file uses unexpected hidden/non-music AudioID"
xxd -p "$out/Prefs/Env-Archive/SYS/screenmode.prefs" | tr -d '\n' | \
  rg -q 'ffffffff0780043800080000$' || \
  fail "screenmode.prefs is not 1920x1080 CLUT8"
xxd -p "$out/Prefs/Env-Archive/SYS/ahi.prefs" | tr -d '\n' | \
  rg -q '4148494700000016000000000000000000000000e6660001000000000002' || \
  fail "ahi.prefs global chunk is malformed"
xxd -p "$out/Prefs/Env-Archive/SYS/ahi.prefs" | tr -d '\n' | \
  rg -q 'ff00000000020009' || \
  fail "ahi.prefs all-units default is not IE AHI"
xxd -p "$out/Prefs/Env-Archive/SYS/ahi.prefs" | tr -d '\n' | \
  rg -q '0000000100020009' || \
  fail "ahi.prefs music unit default is not IE AHI"
assert_contains "$out/S/Startup-Sequence" 'Execute S:User-Startup'
assert_contains "$out/S/User-Startup" 'Assign Work: SYS:Work'
assert_contains "$out/S/User-Startup" '; IE disabled: Mount GOOGLE:'
assert_contains "$out/S/User-Startup" '; IE disabled: Mount KCON: from DEVS:KingCON-mountlist'
assert_contains "$out/S/User-Startup" '; IE disabled: Mount KRAW: from DEVS:KingCON-mountlist'
assert_contains "$out/S/User-Startup" '; IE disabled: mount apipe:'
assert_contains "$out/S/User-Startup" '; IE disabled: exe <nil: >nil: l:fifo-handler'
assert_contains "$out/S/User-Startup" '; IE disabled: c:TaskPriHandler'
assert_contains "$out/S/User-Startup" '; IE disabled: run >NIL: >NIL: yaws'
assert_contains "$out/S/User-Startup" '; IE disabled: ntpSync de.pool.ntp.org'
assert_not_contains "$out/S/User-Startup" '^[[:space:]]*Mount KCON:'
assert_not_contains "$out/S/User-Startup" '^[[:space:]]*Mount KRAW:'
assert_not_contains "$out/S/User-Startup" '^[[:space:]]*mount apipe:'
assert_not_contains "$out/S/User-Startup" '^[[:space:]]*exe <nil: >nil: l:fifo-handler'

assert_not_contains "$out/S/Startup-Sequence" '^[[:space:]]*C:apoke'
assert_not_contains "$out/S/Startup-Sequence" '^[[:space:]]*C:vcontrol'
assert_not_contains "$out/S/Startup-Sequence" '^[[:space:]]*C:LoadModule SYS:Classes/USB'
assert_not_contains "$out/S/Startup-Sequence" '^[[:space:]]*C:FPPrefs'

if scripts/prepare-arosvision-probe.sh "$source" "$source" >/tmp/arosvision-probe-unsafe.out 2>&1; then
  fail "script accepted output equal to source"
fi
if scripts/prepare-arosvision-probe.sh "$source" "$tmp/outside" >/tmp/arosvision-probe-outside.out 2>&1; then
  fail "script accepted output outside build/arosvision-probe"
fi

missing="$tmp/missing"
if scripts/prepare-arosvision-probe.sh "$missing" "$test_output" >/tmp/arosvision-probe-missing.out 2>&1; then
  fail "script accepted missing source"
fi

echo "AROSVision probe script tests passed."
