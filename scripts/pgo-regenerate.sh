#!/usr/bin/env bash
# pgo-regenerate.sh - capture the x64 default.pgo from the manifest workloads.
#
# Implements the procedure documented in default.pgo.manifest and
# sdk/docs/architecture.md: build a -pgo=off capture binary, run each workload
# for its stated duration with IE_CPUPROFILE pointed at a separate file, and
# merge the profiles with `go tool pprof -proto`.
#
# The workloads are GUI demos and need a display (DISPLAY / Wayland). The result
# is written to default.pgo.new. Move it to default.pgo after the native and
# wasm build checks pass, then update default.pgo.manifest in the same change
# set. Nothing is overwritten in place.
#
# (c) 2024 - 2026 Zayn Otley
# https://github.com/IntuitionAmiga/IntuitionEngine
# License: GPLv3 or later
set -euo pipefail

GO="${GO:-go}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

OUT="${1:-default.pgo.new}"
WORKDIR="$(mktemp -d)"
CAPTURE_BIN="${PGO_CAPTURE_BIN:-$WORKDIR/ie-pgo-capture}"
trap 'rm -rf "$WORKDIR"' EXIT

if [[ -z "${PGO_CAPTURE_BIN:-}" ]]; then
  echo "==> Building capture binary (-pgo=off)"
  CGO_ENABLED=1 "$GO" build -pgo=off -tags "embed_basic embed_emutos embed_aros" -o "$CAPTURE_BIN" .
fi

# Each entry: name|duration_seconds|space-separated VM args.
WORKLOADS=(
  "compositor_rotozoomer|30|-ie64 sdk/examples/prebuilt/rotozoomer_ie64.ie64"
  "voodoo_megademo|30|-ie32 sdk/examples/prebuilt/voodoo_mega_demo.iex"
  "audio_wav|30|-wav DoctorWhoSID.wav"
  "emutos_desktop|35|-emutos -emutos-image sdk/examples/prebuilt/etos256us.img"
  "iescript_emutos|15|-emutos -emutos-image sdk/examples/prebuilt/etos256us.img -script emutos_jit_probe.ies"
  "robocop_m68k|30|-m68k sdk/examples/prebuilt/robocop_intro_68k.ie68"
)
if [[ -n "${PGO_WORKLOADS:-}" ]]; then
  mapfile -t WORKLOADS <<<"$PGO_WORKLOADS"
fi

PROFILES=()
missing=0
for entry in "${WORKLOADS[@]}"; do
  IFS='|' read -r name dur args <<<"$entry"
  prof="$WORKDIR/$name.pprof"
  echo "==> Capturing $name (${dur}s): $args"
  # SIGINT so the interrupt handler flushes the in-progress profile cleanly.
  IE_CPUPROFILE="$prof" timeout -s INT "$dur" "$CAPTURE_BIN" $args || true
  if [ -s "$prof" ]; then
    PROFILES+=("$prof")
  else
    echo "ERROR: $name produced no profile (missing demo or no display?)" >&2
    missing=1
  fi
done

if [[ "$missing" -ne 0 || "${#PROFILES[@]}" -ne "${#WORKLOADS[@]}" ]]; then
  echo "ERROR: every declared workload must produce a non-empty profile" >&2
  exit 1
fi

echo "==> Merging ${#PROFILES[@]} profile(s) -> $OUT"
"$GO" tool pprof -proto "${PROFILES[@]}" >"$OUT"

echo "==> Verifying the merged profile builds without fallback"
CGO_ENABLED=1 "$GO" build -pgo="$OUT" -o /dev/null .

echo
echo "Wrote $OUT from ${#PROFILES[@]} workload(s)."
echo "Next: verify native and wasm builds, then 'mv $OUT default.pgo' and"
echo "update default.pgo.manifest."
