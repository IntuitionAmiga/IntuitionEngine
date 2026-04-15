#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${ROOT_DIR}/testdata/external/opl"
ADPLUG_COMMIT="101e7503d91fc783f5152d374a5837a13082ff1b"
BASE_URL="https://raw.githubusercontent.com/adplug/adplug/${ADPLUG_COMMIT}/test/testmus"

download_file() {
	local url="$1"
	local out="$2"

	if command -v curl >/dev/null 2>&1; then
		curl --fail --location --silent --show-error --output "$out" "$url"
		return
	fi
	if command -v wget >/dev/null 2>&1; then
		wget -qO "$out" "$url"
		return
	fi

	echo "error: need curl or wget to fetch OPL test fixtures" >&2
	exit 1
}

verify_sha256() {
	local expected="$1"
	local path="$2"
	local actual

	if command -v sha256sum >/dev/null 2>&1; then
		actual="$(sha256sum "$path" | awk '{print $1}')"
	elif command -v shasum >/dev/null 2>&1; then
		actual="$(shasum -a 256 "$path" | awk '{print $1}')"
	else
		echo "error: need sha256sum or shasum to verify OPL test fixtures" >&2
		exit 1
	fi

	if [[ "$actual" != "$expected" ]]; then
		echo "error: sha256 mismatch for $path" >&2
		echo "expected: $expected" >&2
		echo "actual:   $actual" >&2
		exit 1
	fi
}

fetch_one() {
	local name="$1"
	local sha="$2"
	local url="${BASE_URL}/${name}"
	local tmp

	tmp="$(mktemp)"
	trap 'rm -f "$tmp"' RETURN

	echo "  - ${name}"
	download_file "$url" "$tmp"
	verify_sha256 "$sha" "$tmp"
	mv "$tmp" "${OUT_DIR}/${name}"
	trap - RETURN
}

mkdir -p "$OUT_DIR"

echo "Fetching OPL fixture corpus from adplug/adplug@${ADPLUG_COMMIT}"

fetch_one "YsBattle.vgm" "16fe0bb506eb60ad1e4630d53fb8d5f97367325d91cbb37524aaf7eeed768462"
fetch_one "BeyondSN.vgm" "2aa1a7d5685ecd730e267e166659034601cabbe3a15e967c582a782f3cd63566"
fetch_one "WONDERIN.WLF" "f8e689a3bd1d0602a758857e277b72229c9e6bbd718eeb46a81081b9ab51525b"
fetch_one "dro_v2.dro" "997fad83fb55370663db5baf9878617d91e3b31c99eb7343e45efecda84f55c9"
fetch_one "fm-troni.a2m" "89b0b1678757ef078b77d1a56dd96220ed7df84e068d77868fc6aa324b33f83e"
fetch_one "HIP_D.ROL" "727e361038cf38b3a4f91385420a9cf91e095fdf683cf3aa443ab7f2c6be601e"
fetch_one "standard.bnk" "18bef4a289ee3751abfda8ab3b9958e5b8e8a3b734a7389256630978e5b4197f"

echo "Fetched $(find "$OUT_DIR" -maxdepth 1 -type f ! -name '.gitkeep' | wc -l | tr -d ' ') fixture files into ${OUT_DIR}"
