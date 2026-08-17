#!/usr/bin/env bash
set -euo pipefail
fail(){ echo "stage-intuitionengine-repository: $*" >&2; exit 1; }
private_key_marker="BEGIN PGP PRIVATE KEY"
private_key_marker+=" BLOCK"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
out= home= fp= version= amd64= pi4= pi5=
while (($#)); do
  case "$1" in
    --output|--key-home|--fingerprint|--app-version|--amd64|--pi4|--pi5) (($#>=2))||fail "missing value for $1"; case "$1" in --output) out=$2;;--key-home) home=$2;;--fingerprint) fp=$2;;--app-version) version=$2;;--amd64) amd64=$2;;--pi4) pi4=$2;;--pi5) pi5=$2;;esac; shift 2;;
    *) fail "unknown argument: $1";;
  esac
done
[[ -n "$out" && -n "$home" && -n "$fp" && -n "$version" ]] || fail "output, key home, fingerprint and version are required"
[[ -f "$amd64" && -f "$pi4" && -f "$pi5" ]] || fail "all three packages are required"
"$script_dir/check-intuitionengine-release-secrets.sh" --root "$script_dir/.." --output "$out"
out_real=$(realpath -m "$out"); home_real=$(realpath -m "$home")
case "$home_real/" in "$out_real"/*) fail "private GnuPG home must not be below repository output";; esac
if [[ -d "$out" ]]; then
    while IFS= read -r private_path; do
        fail "private signing material is present below repository output: $private_path"
    done < <(find "$out" -type f \( -name '*.key' -o -name 'private*' -o -name 'secring.gpg' \) -print)
    if rg -a -l --hidden --glob '!.git/**' "$private_key_marker" "$out" >/dev/null 2>&1; then
        fail "private PGP key material is present below repository output"
    fi
fi
for tool in dpkg-deb dpkg-scanpackages gzip gpg realpath stat md5sum sha1sum sha256sum; do command -v "$tool" >/dev/null || fail "$tool is required"; done
tmp=$(mktemp -d "${TMPDIR:-/tmp}/intuitionengine-repo.XXXXXX"); trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/pool/main/i" "$tmp/dists/stable/main/binary-amd64" "$tmp/dists/stable/main/binary-arm64"
for deb in "$amd64" "$pi4" "$pi5"; do
    package=$(dpkg-deb -f "$deb" Package); debver=$(dpkg-deb -f "$deb" Version); arch=$(dpkg-deb -f "$deb" Architecture)
    [[ "$debver" == "$version-1" ]] || fail "$deb has version $debver"
    [[ "$(basename "$deb")" == "${package}_${debver}_${arch}.deb" ]] || fail "$deb filename does not match Debian package identity"
  case "$package:$arch" in intuitionengine-amd64-v3:amd64|intuitionengine-arm64-pi4:arm64|intuitionengine-arm64-pi5:arm64);;*) fail "invalid package identity $package/$arch";;esac
  install -Dm0644 "$deb" "$tmp/pool/main/i/$package/$(basename "$deb")"
done
(
  cd "$tmp"
  dpkg-scanpackages pool/main/i/intuitionengine-amd64-v3 /dev/null
) >"$tmp/dists/stable/main/binary-amd64/Packages"
(
  cd "$tmp"
  dpkg-scanpackages pool/main/i/intuitionengine-arm64-pi4 /dev/null
  dpkg-scanpackages pool/main/i/intuitionengine-arm64-pi5 /dev/null
) >"$tmp/dists/stable/main/binary-arm64/Packages"
gzip -n -9 <"$tmp/dists/stable/main/binary-amd64/Packages" >"$tmp/dists/stable/main/binary-amd64/Packages.gz"
gzip -n -9 <"$tmp/dists/stable/main/binary-arm64/Packages" >"$tmp/dists/stable/main/binary-arm64/Packages.gz"
release="$tmp/dists/stable/Release"
release_paths=(
  main/binary-amd64/Packages
  main/binary-amd64/Packages.gz
  main/binary-arm64/Packages
  main/binary-arm64/Packages.gz
)
write_checksums() {
  local tool="$1" path digest size
  for path in "${release_paths[@]}"; do
    digest=$("$tool" "$tmp/dists/stable/$path" | awk '{print $1}')
    size=$(stat -c '%s' "$tmp/dists/stable/$path")
    printf ' %s %s %s\n' "$digest" "$size" "$path"
  done
}
cat >"$release" <<EOF
Origin: IntuitionEngine
Label: IntuitionEngine
Suite: stable
Codename: stable
Date: $(LC_ALL=C date -Ru)
Architectures: amd64 arm64
Components: main
Description: IntuitionEngine Debian packages
MD5Sum:
EOF
write_checksums md5sum >>"$release"
printf 'SHA1:\n' >>"$release"
write_checksums sha1sum >>"$release"
printf 'SHA256:\n' >>"$release"
write_checksums sha256sum >>"$release"
gpg --homedir "$home" --batch --yes --local-user "$fp" --clearsign --output "$tmp/dists/stable/InRelease" "$tmp/dists/stable/Release"
gpg --homedir "$home" --batch --yes --local-user "$fp" --detach-sign --output "$tmp/dists/stable/Release.gpg" "$tmp/dists/stable/Release"
gpg --homedir "$home" --batch --yes --export "$fp" | gpg --dearmor >"$tmp/repository-keyring.gpg"
cp "$tmp/repository-keyring.gpg" "$tmp/intuitionengine-archive-keyring.gpg"
printf '%s\n' "deb [signed-by=$tmp/repository-keyring.gpg] file://$tmp stable main" >"$tmp/sources.list"
for deb in "$amd64" "$pi4" "$pi5"; do
  package=$(dpkg-deb -f "$deb" Package); candidate="$out/pool/main/i/$package/$(basename "$deb")"
  [[ ! -f "$candidate" ]] || cmp -s "$deb" "$candidate" || fail "immutable package changed for $package"
done
mkdir -p "$out"
rm -rf "$out/pool" "$out/dists" "$out/intuitionengine-archive-keyring.gpg"
cp -a "$tmp/pool" "$out/pool"
cp -a "$tmp/dists" "$out/dists"
cp -a "$tmp/intuitionengine-archive-keyring.gpg" "$out/intuitionengine-archive-keyring.gpg"
