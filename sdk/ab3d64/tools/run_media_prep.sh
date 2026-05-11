#!/usr/bin/env bash
# Phase 2 media-prep wrapper.
#
# Usage: run_media_prep.sh <profile> <build_dir>
#   <profile>   redux-high | redux-low
#   <build_dir> destination for media_profile.i (and asset overlay tree)
#
# Always emits <build_dir>/media_profile.i so ie/ie_file_io_runtime.i
# resolves its include even when no runtime assets are available.
#
# If AB3D64_MEDIA_SRC is set and points at an alienbreed3d2 repo root
# (i.e. a tree containing karlos-tkg-main/Game), this script also runs the
# vendored prepare_media_profile.py + unpack_sb_assets.py helpers to
# stage assets under <build_dir>/<profile>/. Absent that env var,
# media_profile.i is emitted but asset prep is skipped with a warning.

set -euo pipefail

profile="${1:?profile required (redux-high|redux-low)}"
build_dir="${2:?build_dir required}"

case "$profile" in
    redux-high|redux-low) ;;
    *) echo "run_media_prep.sh: unknown profile '$profile'" >&2; exit 2 ;;
esac

mkdir -p "$build_dir"

# media_profile.i — runtime path-prefix tables consumed by
# ie/ie_file_io_runtime.i. Matches upstream `build.mk` exactly:
# assets are read from `_build/ie_media/${profile}/` relative to
# wherever the engine is launched from. Operator stages the asset
# tree manually (e.g. by launching from a directory whose `_build/`
# already holds the upstream pre-built assets).
media_root="_build/ie_media/${profile}/"
sound_root="_build/ie_media/${profile}/soundfx/"
{
    printf '%s\n' '.ie_media_prefix:'
    printf "\t\t\t\tdc.b\t'%s',0\n" "$media_root"
    printf '%s\n' '.ie_sfx_prefix:'
    printf "\t\t\t\tdc.b\t'%s',0\n" "$sound_root"
} > "$build_dir/media_profile.i"

if [[ -z "${AB3D64_MEDIA_SRC:-}" ]]; then
    cat >&2 <<EOF
run_media_prep.sh: AB3D64_MEDIA_SRC unset.
  media_profile.i emitted; runtime asset staging skipped.
  Build will succeed but the resulting .ie64 will not find media at
  ${media_root}. To stage assets, set
  AB3D64_MEDIA_SRC=/path/to/alienbreed3d2 and re-run.
EOF
    exit 0
fi

src_root="${AB3D64_MEDIA_SRC}"
if [[ ! -d "$src_root" ]]; then
    echo "run_media_prep.sh: AB3D64_MEDIA_SRC=$src_root not a directory" >&2
    exit 1
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
python3 "$script_dir/prepare_media_profile.py" \
    --profile "$profile" \
    --repo-root "$src_root" \
    --out "$build_dir/$profile"
