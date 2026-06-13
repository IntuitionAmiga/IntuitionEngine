#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source_dir="${1:-../AROSVision}"
output_dir="${2:-build/arosvision-probe/AROS}"
ie_aros_dir="$root_dir/../AROS-deadw00d/bin/ie-m68k/bin/ie-m68k/AROS"
ie_tools_dir="$root_dir/../AROS-deadw00d/bin/ie-m68k/bin/linux-x86_64/tools"
ie_images_dir="$root_dir/../AROS-deadw00d/images/IconSets/Mason/workbench"
ie_runtime_dir="$root_dir/Systems/AROS"

fail() {
  echo "Error: $*" >&2
  exit 1
}

realpath_m() {
  realpath -m "$1"
}

write_ie_startup_sequence() {
  local source_path="$1"
  local output_path="$2"

  perl -ne '
    if (/^\s*c:apoke\b/i ||
        /^\s*c:vcontrol\b/i) {
      print "; IE disabled: $_";
      next;
    }

    if (/^\s*If EXISTS "SYS:Classes\/USB"/i) {
      $skip_usb = 1;
      print "; IE disabled USB stack block:\n";
      print "; $_";
      next;
    }
    if ($skip_usb) {
      print "; $_";
      $skip_usb = 0 if /^\s*EndIf\b/i;
      next;
    }

    s/"FONTS:__TEST__"/"SYS:Fonts\/__TEST__"/g if /^\s*(IF EXISTS|Delete)\s+"FONTS:__TEST__"/i;

    if (/^\s*setenv\s+workbench\s+save\b/i && !$system_assign_done) {
      print "Assign \"System:\" \"IE:\"\n";
      $system_assign_done = 1;
    }

    if (/^\s*C:FPPrefs\b/i) {
      print "; IE disabled: $_";
      next;
    }

    print;
  ' "$source_path" >"$output_path"
}

write_ie_user_startup() {
  local source_path="$1"
  local output_path="$2"

  perl -ne '
	    if (/^\s*Mount\s+(GOOGLE|DBOX|KCON|KRAW):(?:\s|$)/i ||
	        /^\s*mount\s+(cbm0|apipe|aux|tee|zero):(?:\s|$)/i ||
	        /^\s*exe\s+<nil:\s+>nil:\s+l:fifo-handler\b/i ||
	        /^\s*c:TaskPriHandler\b/i ||
	        /^\s*run\s+>NIL:\s+>NIL:\s+yaws\b/i ||
	        /^\s*ntpSync\b/i ||
	        /^\s*Execute\s+\$\{UHCBIN\}UHC-Startup\b/i ||
	        /^\s*Execute\s+ENV:PathManager\.prefs\b/i ||
	        /^\s*execute\s+JFHD:Extras\/HD-ASSIGNS\b/i ||
	        /^\s*Sys:Prefs\/Assigns\s+USE\b/i ||
	        /^\s*Libs:svppc\/loadppclib\b/i ||
	        /^\s*Run\b.*\bC:AmigaGPTD\b/i ||
	        /^\s*stack\s+999999\b/i ||
	        /^\s*mysql:bin\/mysqld\b/i) {
	      print "; IE disabled: $_";
	      next;
	    }

	    if (/^\s*;BEGIN sofa\b/i) {
	      $skip_sofa = 1;
	      print "; IE disabled unsupported sofa block:\n";
	      print "; $_";
	      next;
	    }
	    if ($skip_sofa) {
	      print "; $_";
	      $skip_sofa = 0 if /^\s*;END sofa\b/i;
	      next;
	    }

	    print;
	  ' "$source_path" >"$output_path"
}

write_ie_default_prefs() {
  local prefs_dir="$1"

  mkdir -p "$prefs_dir"

  # IFF PREF/SCRM: 1920x1080, depth 8, default mode ID.
  perl -e '
    print pack("H*",
      "464f524d0000003650524546" .
      "5052484400000006000000000000" .
      "5343524d0000001c" .
      "00000000000000000000000000000000" .
      "ffffffff0780043800080000"
    );
  ' >"$prefs_dir/screenmode.prefs"

  # IFF PREF/AHIG+AHIU: default all units to the IE AHI mode ID (0x00020009).
  perl -e '
    print pack("H*",
      "464f524d000000f850524546" .
      "5052484400000006000000000000" .
      "4148494700000016000000000000000000000000e6660001000000000002" .
      "4148495500000020ff000000000200090000bb800001000000010000000100000000000000000000" .
      "414849550000002000000001000200090000bb8000010f2b00010000000100000000000000000000" .
      "41484955000000200100000100020009000072d800010f2b00010000000100000000000000000000" .
      "41484955000000200200000100020009000072d800010f2b00010000000100000000000000000000" .
      "41484955000000200300000100020010000072d800010f2b00010000000100000000000000000000"
    );
  ' >"$prefs_dir/ahi.prefs"
}

source_abs="$(realpath_m "$source_dir")"
output_abs="$(realpath_m "$output_dir")"
probe_root_abs="$(realpath_m "$root_dir/build/arosvision-probe")"

[[ -d "$source_abs" ]] || fail "AROSVision source not found: $source_dir"
[[ -f "$source_abs/S/Startup-Sequence" ]] || fail "source is not an AROSVision system tree: missing S/Startup-Sequence"
[[ "$source_abs" != "$output_abs" ]] || fail "output must not equal source"
case "$output_abs" in
  "$probe_root_abs"/*) ;;
  *) fail "output must be inside build/arosvision-probe" ;;
esac

cd "$root_dir"
rm -rf "$output_abs"
mkdir -p "$(dirname "$output_abs")"
cp -a "$source_abs" "$output_abs"

startup_dir="$output_abs/S"
cp -p "$startup_dir/Startup-Sequence" "$startup_dir/Startup-Sequence.AROSVision.orig"

if [[ -f "$startup_dir/User-Startup" ]]; then
  cp -p "$startup_dir/User-Startup" "$startup_dir/User-Startup.AROSVision.orig"
  write_ie_user_startup \
    "$startup_dir/User-Startup.AROSVision.orig" \
    "$startup_dir/User-Startup"
fi

disabled_dir="$output_abs/Storage/IEProbeDisabled/WBStartup"
mkdir -p "$disabled_dir"

disabled_monitors_dir="$output_abs/Storage/IEProbeDisabled/Devs/Monitors"
mkdir -p "$disabled_monitors_dir"
for name in SAGA SAGA.info; do
  if [[ -e "$output_abs/Devs/Monitors/$name" ]]; then
    mv "$output_abs/Devs/Monitors/$name" "$disabled_monitors_dir/$name"
  fi
done

for rel in Devs/DOSDrivers/CD0 Devs/DOSDrivers/CD0.info; do
  if [[ -e "$output_abs/$rel" ]]; then
    rm -f "$output_abs/$rel"
  fi
done

for rel in \
  Devs/DOSDrivers/ICD0 \
  Devs/DOSDrivers/ICD0.info \
  Devs/DOSDrivers/ICD1 \
  Devs/DOSDrivers/ICD1.info; do
  if [[ -f "$output_abs/$rel" ]]; then
    perl -0pi -e 's/Activate\s*=\s*1/Activate       = 0/g; s/ACTIVATE=1/ACTIVATE=0/g' "$output_abs/$rel"
  fi
done

if [[ -f "$ie_aros_dir/C/LoadWB" ]]; then
  if [[ -f "$output_abs/C/LoadWB" ]]; then
    mkdir -p "$output_abs/Storage/IEProbeDisabled/C"
    mv "$output_abs/C/LoadWB" "$output_abs/Storage/IEProbeDisabled/C/LoadWB.Scalos"
  fi
  cp -p "$ie_aros_dir/C/LoadWB" "$output_abs/C/LoadWB"
fi

for rel in \
  L/iehandler-handler \
  Devs/audio.device \
  Devs/Drivers/iegfx.hidd \
  Devs/Drivers/inputclass.hidd \
  Devs/Drivers/keyboard.hidd \
  Devs/Drivers/mouse.hidd \
  Devs/Drivers/iekbd.hidd \
  Devs/Drivers/iemouse.hidd \
  Devs/input.device \
  Devs/keyboard.device \
  Devs/gameport.device; do
  if [[ -e "$output_abs/$rel" ]]; then
    mkdir -p "$output_abs/Storage/IEProbeDisabled/$(dirname "$rel")"
    mv "$output_abs/$rel" "$output_abs/Storage/IEProbeDisabled/$rel"
  fi
done

for rel in \
  Devs/AHI/ac97.audio \
  Devs/AHI/cmi8738.audio \
  Devs/AHI/emu10kx.audio \
  Devs/AHI/envy24.audio \
  Devs/AHI/envy24ht.audio \
  Devs/AHI/hdaudio.audio \
  Devs/AHI/sb128.audio \
  Devs/AHI/via-ac97.audio \
  Devs/AudioModes/CMI8738 \
  Devs/AudioModes/EMU10KX \
  Devs/AudioModes/ENVY24 \
  Devs/AudioModes/ENVY24HT \
  Devs/AudioModes/HDAUDIO \
  Devs/AudioModes/SB128 \
  Devs/AudioModes/VIA-AC97 \
  Devs/AudioModes/ac97; do
  if [[ -e "$output_abs/$rel" ]]; then
    mkdir -p "$output_abs/Storage/IEProbeDisabled/$(dirname "$rel")"
    mv "$output_abs/$rel" "$output_abs/Storage/IEProbeDisabled/$rel"
  fi
done

for rel in \
  Devs/Midi/debugdriver \
  Devs/Midi/echo \
  Devs/Midi/uaemidi \
  Devs/Midi/udp; do
  if [[ -e "$output_abs/$rel" ]]; then
    mkdir -p "$output_abs/Storage/IEProbeDisabled/$(dirname "$rel")"
    mv "$output_abs/$rel" "$output_abs/Storage/IEProbeDisabled/$rel"
  fi
done

if [[ -f "$ie_aros_dir/Devs/Midi/ie" ]]; then
  mkdir -p "$output_abs/Devs/Midi"
  cp -p "$ie_aros_dir/Devs/Midi/ie" "$output_abs/Devs/Midi/ie"
fi

if [[ -f "$ie_aros_dir/Devs/AHI/ie-audio.audio" ]]; then
  mkdir -p "$output_abs/Devs/AHI"
  cp -p "$ie_aros_dir/Devs/AHI/ie-audio.audio" "$output_abs/Devs/AHI/ie-audio.audio"
fi

rm -f "$output_abs/Libs/ie-audio.library"

iewarp_service_src=""
for candidate in \
  "$ie_aros_dir/Libs/iewarp_service.ie64" \
  "$ie_runtime_dir/Libs/iewarp_service.ie64" \
  "$root_dir/sdk/examples/prebuilt/iewarp_service.ie64"; do
  if [[ -f "$candidate" ]]; then
    iewarp_service_src="$candidate"
    break
  fi
done
if [[ -n "$iewarp_service_src" ]]; then
  mkdir -p "$output_abs/Systems/AROS/Libs"
  cp -p "$iewarp_service_src" "$output_abs/Systems/AROS/Libs/iewarp_service.ie64"
fi

if [[ -f "$ie_aros_dir/Utilities/IEWarpMon" ]]; then
  mkdir -p "$output_abs/Utilities"
  cp -p "$ie_aros_dir/Utilities/IEWarpMon" "$output_abs/Utilities/IEWarpMon"
fi

if [[ -x "$ie_tools_dir/ilbmtoicon" && \
      -f "$ie_images_dir/Utilities/IEWarpMon.info.src" && \
      -f "$ie_images_dir/Utilities/IEWarpMon.png" ]]; then
  mkdir -p "$output_abs/Utilities"
  "$ie_tools_dir/ilbmtoicon" \
    "$ie_images_dir/Utilities/IEWarpMon.info.src" \
    "$ie_images_dir/Utilities/IEWarpMon.png" \
    "$output_abs/Utilities/IEWarpMon.info"
fi

mkdir -p "$output_abs/Devs/AudioModes"
perl -e '
use strict; use warnings;
sub u32 { pack("N", shift) }
sub pad { my ($s)=@_; return length($s) % 2 ? $s . "\0" : $s }
sub chunk { my ($id, $data)=@_; return $id . u32(length($data)) . pad($data) }
my $TAG_USER = 0x80000000;
my $TAG_PTR = 0x00008000;
my $AHIDB_AudioID = $TAG_USER + 100;
my $AHIDB_Volume = $TAG_USER + 103;
my $AHIDB_Panning = $TAG_USER + 104;
my $AHIDB_Stereo = $TAG_USER + 105;
my $AHIDB_HiFi = $TAG_USER + 106;
my $AHIDB_PingPong = $TAG_USER + 107;
my $AHIDB_Name = $TAG_USER + $TAG_PTR + 109;
my $AHIDB_MaxChannels = $TAG_USER + 111;
my $name = "IE:16 bit stereo HiFi\0";
my $tags = "";
$tags .= u32($AHIDB_AudioID) . u32(0x00020040);
$tags .= u32($AHIDB_Volume) . u32(1);
$tags .= u32($AHIDB_Panning) . u32(1);
$tags .= u32($AHIDB_Stereo) . u32(1);
$tags .= u32($AHIDB_HiFi) . u32(1);
$tags .= u32($AHIDB_PingPong) . u32(0);
$tags .= u32($AHIDB_MaxChannels) . u32(32);
$tags .= u32($AHIDB_Name) . u32(length($tags) + 16);
$tags .= u32(0) . u32(0);
my $body = chunk("AUDN", "ie-audio\0") . chunk("AUDM", $tags . $name);
my $form = "FORM" . u32(4 + length($body)) . "AHIM" . $body;
open my $fh, ">:raw", $ARGV[0] or die "$ARGV[0]: $!\n";
print $fh $form;
close $fh;
' "$output_abs/Devs/AudioModes/IE"

write_ie_default_prefs "$output_abs/Prefs/Env-Archive/SYS"

write_ie_startup_sequence \
  "$startup_dir/Startup-Sequence.AROSVision.orig" \
  "$startup_dir/Startup-Sequence"

echo "Prepared AROSVision probe tree: $output_abs"
echo "Boot with: go run . -aros -aros-drive $output_dir"
