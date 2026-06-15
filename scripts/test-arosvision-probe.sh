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
test_output="build/arosvision-test"
trap 'rm -rf "$tmp" build/arosvision-test' EXIT

source="$tmp/AROSVision"
ie_aros="$tmp/ie-aros"
ie_tools="$tmp/ie-tools"
ie_images="$tmp/ie-images"
mkdir -p "$source/S" "$source/System/Wanderer" "$source/C" "$source/Prefs/Env-Archive" \
  "$source/WBStartup" "$source/Devs/Midi" "$source/Devs/Drivers"
mkdir -p "$ie_aros/Devs/AHI" "$ie_aros/Utilities" "$ie_tools" "$ie_images/Utilities" "$tmp/Libs"
cat >"$source/S/Startup-Sequence" <<'SRCSTART'
Assign WANDERER: SYS:System/Wanderer DEFER
c:sysvars
Execute S:User-Startup
C:apoke dead beef
C:vcontrol something
Automount >NIL:
Mount >NIL: "DEVS:DOSDrivers/~((.#?)|(#?.info)|(#?.dbg))"
If EXISTS "SYS:Classes/USB"
    C:LoadModule SYS:Classes/USB/foo.class
EndIf
IF EXISTS "FONTS:__TEST__"
    Delete "FONTS:__TEST__"
EndIf
C:FPPrefs >NIL:
setenv workbench save 40.42
Dir >NIL: "PIPE:"
Touch >NIL: "FONTS:__TEST__"
RunFromWB sys:WBStartUp/Additional/WBDock
if $ArosVision eq 1
c:LoadWB
endif
if $ArosVision eq 2
WANDERER:Wanderer
RunFromWB sys:WBStartUp/Additional/WBDock
endif
SRCSTART
cat >"$source/S/User-Startup" <<'SRCUSER'
Assign Work: SYS:Work
Assign Desktop: Sys:Extras/Desktops/Opus5/Desktop
IF EXISTS Devs:Cloud/google.mountlist
Mount GOOGLE: from Devs:Cloud/google.mountlist
ENDIF
Mount DBOX: from Devs:Cloud/cloud.mountlist
Mount KCON: from DEVS:KingCON-mountlist
Mount KRAW: from DEVS:KingCON-mountlist
mount cbm0:
mount apipe:
mount aux:
mount tee:
mount zero:
exe <nil: >nil: l:fifo-handler
c:TaskPriHandler
run >NIL: >NIL: yaws
ntpSync de.pool.ntp.org
Execute ${UHCBIN}UHC-Startup
Execute ENV:PathManager.prefs
execute JFHD:Extras/HD-ASSIGNS
Sys:Prefs/Assigns USE
Libs:svppc/loadppclib
Run <>NIL: C:AmigaGPTD <>NIL:
stack 999999
mysql:bin/mysqld
;BEGIN sofa
Assign sofa: "System:Extras/Developing/sofa"
endif
;END sofa
SRCUSER
touch "$source/System/Wanderer/Wanderer" "$source/C/IPrefs" "$source/C/AddDatatypes"
printf 'arosvision-loadwb\n' >"$source/C/LoadWB"
touch "$source/WBStartup/Clipper" "$source/WBStartup/Clipper.info" \
  "$source/WBStartup/DiskImageGUI" "$source/WBStartup/DiskImageGUI.info"
mkdir -p "$source/WBStartup/Additional"
touch "$source/WBStartup/Additional/WBDock" "$source/WBStartup/Additional/WBDock.info"
touch "$source/Devs/Midi/debugdriver" "$source/Devs/Midi/echo" \
  "$source/Devs/Midi/uaemidi" "$source/Devs/Midi/udp"
printf 'source-iegfx\n' >"$source/Devs/Drivers/iegfx.hidd"
printf 'ie-audio.audio\0$VER: ie-audio.audio 2.0 test\0' >"$ie_aros/Devs/AHI/ie-audio.audio"
printf 'iewarpmon\n' >"$ie_aros/Utilities/IEWarpMon"
printf 'iewarpmon-icon\n' >"$ie_aros/Utilities/IEWarpMon.info"
printf 'worker\n' >"$tmp/Libs/iewarp_service.ie64"

src_start_before="$(sha256sum "$source/S/Startup-Sequence" | awk '{print $1}')"
src_user_before="$(sha256sum "$source/S/User-Startup" | awk '{print $1}')"

IE_AROS_DIR="$ie_aros" IE_RUNTIME_DIR="$tmp" IE_TOOLS_DIR="$ie_tools" IE_IMAGES_DIR="$ie_images" \
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
assert_file "$out/C/LoadWB"
cmp "$source/C/LoadWB" "$out/C/LoadWB" >/dev/null || fail "AROSVision LoadWB was replaced"
assert_file "$out/Storage/IEProbeDisabled/Devs/Drivers/iegfx.hidd"
[[ ! -e "$out/Devs/Drivers/iegfx.hidd" ]] || fail "iegfx.hidd remained active in generated AROSVision"
if [[ -f ../AROS-deadw00d/bin/ie-m68k/bin/ie-m68k/AROS/Devs/Midi/ie ]]; then
  assert_file "$out/Devs/Midi/ie"
fi
assert_file "$out/Devs/AHI/ie-audio.audio"
strings -a "$out/Devs/AHI/ie-audio.audio" | rg -q '^ie-audio\.audio$' || \
  fail "IE AHI driver resident name is not ie-audio.audio"
strings -a "$out/Devs/AHI/ie-audio.audio" | rg -q '^\$VER: ie-audio\.audio 2\.0 ' || \
  fail "IE AHI driver resident version is not 2.0"
strings -a "$out/Devs/AHI/ie-audio.audio" | rg -q 'ie-audio\.library' && \
  fail "IE AHI driver still contains ie-audio.library metadata"
[[ ! -e "$out/Libs/ie-audio.library" ]] || fail "ie-audio.library was staged"
assert_file "$out/Systems/AROS/Libs/iewarp_service.ie64"
assert_file "$out/Utilities/IEWarpMon"
assert_file "$out/Utilities/IEWarpMon.info"
[[ ! -e "$out/Libs/iewarp.library" ]] || fail "iewarp.library was staged"

missing_overlay_out="build/arosvision-missing-overlays-test"
rm -rf "$missing_overlay_out"
mkdir -p "$missing_overlay_out"
if ! IE_AROS_DIR="$tmp/missing-ie-aros" IE_RUNTIME_DIR="$tmp/missing-runtime" IE_TOOLS_DIR="$ie_tools" IE_IMAGES_DIR="$ie_images" \
  scripts/prepare-arosvision-probe.sh "$source" "$missing_overlay_out" >/tmp/arosvision-probe-missing-overlays.out 2>&1; then
  fail "default probe tree should tolerate missing live IE overlays"
fi
if IE_AROS_DIR="$tmp/missing-ie-aros" IE_RUNTIME_DIR="$tmp/missing-runtime" IE_TOOLS_DIR="$ie_tools" IE_IMAGES_DIR="$ie_images" \
  scripts/prepare-arosvision-probe.sh --overlay "$source" "$missing_overlay_out" >/tmp/arosvision-live-missing-overlays.out 2>&1; then
  fail "live overlay mode should require IE overlays"
fi

assert_file "$out/Storage/IEProbeDisabled/WBStartup/Clipper"
assert_file "$out/Storage/IEProbeDisabled/WBStartup/Clipper.info"
assert_file "$out/WBStartup/Additional/WBDock"
assert_file "$out/WBStartup/Additional/WBDock.info"
assert_file "$out/WBStartup/DiskImageGUI"
assert_file "$out/WBStartup/DiskImageGUI.info"
[[ ! -e "$out/WBStartup/Clipper" ]] || fail "Clipper remained active in WBStartup"
[[ ! -e "$out/WBStartup/Clipper.info" ]] || fail "Clipper.info remained active in WBStartup"
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
assert_contains "$out/S/Startup-Sequence" '^[[:space:]]*Automount >NIL:'
assert_contains "$out/S/Startup-Sequence" '^[[:space:]]*Mount >NIL: "DEVS:DOSDrivers/~\(\(\.#\?\)\|\(#\?\.info\)\|\(#\?\.dbg\)\)"'
assert_contains "$out/S/Startup-Sequence" '^[[:space:]]*Dir >NIL: "PIPE:"'
assert_contains "$out/S/Startup-Sequence" '^[[:space:]]*Touch >NIL: "FONTS:__TEST__"'
assert_contains "$out/S/Startup-Sequence" '^[[:space:]]*c:sysvars'
assert_contains "$out/S/Startup-Sequence" '^[[:space:]]*set ArosVision 2'
assert_not_contains "$out/S/Startup-Sequence" '^[[:space:]]*setenv ArosVision 2'
assert_contains "$out/S/Startup-Sequence" '^[[:space:]]*WANDERER:Wanderer'
assert_contains "$out/S/Startup-Sequence" '^[[:space:]]*RunFromWB sys:WBStartUp/Additional/WBDock'
assert_contains "$out/S/Startup-Sequence" '; IE disabled: RunFromWB sys:WBStartUp/Additional/WBDock'
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
assert_contains "$out/S/User-Startup" '^[[:space:]]*Assign Desktop: SYS:System/Wanderer'
assert_not_contains "$out/S/User-Startup" '^[[:space:]]*Assign Desktop: Sys:Extras/Desktops/Opus5/Desktop'
assert_contains "$out/S/User-Startup" '; IE disabled: exe <nil: >nil: l:fifo-handler'
assert_contains "$out/S/User-Startup" '; IE disabled: ntpSync de.pool.ntp.org'
assert_contains "$out/S/User-Startup" '; IE disabled: Libs:svppc/loadppclib'
assert_contains "$out/S/User-Startup" '^[[:space:]]*Mount KCON: from DEVS:KingCON-mountlist'
assert_contains "$out/S/User-Startup" '^[[:space:]]*Mount KRAW: from DEVS:KingCON-mountlist'
assert_contains "$out/S/User-Startup" '^[[:space:]]*Mount GOOGLE: from Devs:Cloud/google.mountlist'
assert_contains "$out/S/User-Startup" '^[[:space:]]*Mount DBOX: from Devs:Cloud/cloud.mountlist'
assert_contains "$out/S/User-Startup" '^[[:space:]]*run >NIL: >NIL: yaws'
assert_contains "$out/S/User-Startup" '^[[:space:]]*Run <>NIL: C:AmigaGPTD <>NIL:'
assert_contains "$out/S/User-Startup" '^[[:space:]]*stack 999999'
assert_contains "$out/S/User-Startup" '^[[:space:]]*mysql:bin/mysqld'
assert_not_contains "$out/S/User-Startup" '^[[:space:]]*; IE disabled:.*C:AmigaGPTD'
assert_not_contains "$out/S/User-Startup" '^[[:space:]]*; IE disabled: Mount KCON:'
assert_not_contains "$out/S/User-Startup" '^[[:space:]]*; IE disabled: Mount KRAW:'
assert_not_contains "$out/S/User-Startup" '^[[:space:]]*; IE disabled: Mount GOOGLE:'
assert_not_contains "$out/S/User-Startup" '^[[:space:]]*; IE disabled: Mount DBOX:'
assert_contains "$out/S/User-Startup" '^[[:space:]]*Execute \$\{UHCBIN\}UHC-Startup'
assert_not_contains "$out/S/User-Startup" '^[[:space:]]*; IE disabled: Execute \$\{UHCBIN\}UHC-Startup'
assert_contains "$out/S/User-Startup" '^[[:space:]]*mount cbm0:'
assert_contains "$out/S/User-Startup" '^[[:space:]]*mount apipe:'
assert_contains "$out/S/User-Startup" '^[[:space:]]*mount aux:'
assert_contains "$out/S/User-Startup" '^[[:space:]]*mount tee:'
assert_contains "$out/S/User-Startup" '^[[:space:]]*mount zero:'
assert_not_contains "$out/S/User-Startup" '^[[:space:]]*exe <nil: >nil: l:fifo-handler'
assert_contains "$out/S/User-Startup" '^[[:space:]]*c:TaskPriHandler'
assert_contains "$out/S/User-Startup" '^[[:space:]]*Sys:Prefs/Assigns USE'
assert_contains "$out/S/User-Startup" '^[[:space:]]*Execute ENV:PathManager\.prefs'
assert_contains "$out/S/User-Startup" '^[[:space:]]*execute JFHD:Extras/HD-ASSIGNS'
assert_contains "$out/S/User-Startup" '^[[:space:]]*Assign sofa: "System:Extras/Developing/sofa"'
assert_not_contains "$out/S/User-Startup" 'IE disabled unsupported sofa block'
assert_not_contains "$out/S/User-Startup" '^[[:space:]]*Libs:svppc/loadppclib'
assert_not_contains "$out/S/User-Startup" '^[[:space:]]*; IE disabled: stack 999999'
assert_not_contains "$out/S/User-Startup" '^[[:space:]]*; IE disabled: mysql:bin/mysqld'

assert_not_contains "$out/S/Startup-Sequence" '^[[:space:]]*C:apoke'
assert_not_contains "$out/S/Startup-Sequence" '^[[:space:]]*C:vcontrol'
assert_not_contains "$out/S/Startup-Sequence" '^[[:space:]]*C:LoadModule SYS:Classes/USB'
assert_not_contains "$out/S/Startup-Sequence" '^[[:space:]]*C:FPPrefs'

if IE_AROS_DIR="$ie_aros" IE_RUNTIME_DIR="$tmp" scripts/prepare-arosvision-probe.sh "$source" "$source" >/tmp/arosvision-probe-unsafe.out 2>&1; then
  fail "script accepted output equal to source"
fi
if IE_AROS_DIR="$ie_aros" IE_RUNTIME_DIR="$tmp" scripts/prepare-arosvision-probe.sh "$source" "$tmp/outside" >/tmp/arosvision-probe-outside.out 2>&1; then
  fail "script accepted output outside build"
fi

missing="$tmp/missing"
if IE_AROS_DIR="$ie_aros" IE_RUNTIME_DIR="$tmp" scripts/prepare-arosvision-probe.sh "$missing" "$test_output" >/tmp/arosvision-probe-missing.out 2>&1; then
  fail "script accepted missing source"
fi

echo "AROSVision probe script tests passed."
