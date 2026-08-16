#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fail(){ echo "FAIL: $*" >&2; exit 1; }
missing=()
for tool in dpkg-deb dpkg-scanpackages gzip gpg gpgv gpg-agent gpg-connect-agent gpgconf realpath stat md5sum sha1sum sha256sum; do
    command -v "$tool" >/dev/null 2>&1 || missing+=("$tool")
done
if ((${#missing[@]})); then
    echo "repository integration test skipped; missing: ${missing[*]}"
    exit 0
fi

tmp="$(mktemp -d)"
home="$tmp/gnupg"; mkdir -m0700 "$home"
cleanup_gpg() {
    gpgconf --homedir "$home" --kill gpg-agent >/dev/null 2>&1 || true
    rm -rf "$tmp"
}
trap cleanup_gpg EXIT
agent_log="$tmp/gpg-agent.log"
if ! gpgconf --homedir "$home" --launch gpg-agent >"$agent_log" 2>&1; then
    cat "$agent_log" >&2
    fail "could not launch temporary gpg-agent"
fi
if ! gpg-connect-agent --homedir "$home" /bye >"$agent_log" 2>&1; then
    cat "$agent_log" >&2
    gpg-agent --homedir "$home" --daemon --allow-loopback-pinentry >>"$agent_log" 2>&1 || {
        cat "$agent_log" >&2
        fail "could not start a usable temporary gpg-agent"
    }
    gpg-connect-agent --homedir "$home" /bye >"$agent_log" 2>&1 || {
        cat "$agent_log" >&2
        fail "temporary gpg-agent is not reachable"
    }
fi
if ! gpg --homedir "$home" --batch --pinentry-mode loopback --passphrase '' --quick-generate-key 'IntuitionEngine test <test@example.invalid>' rsa2048 sign 0 >"$agent_log" 2>&1; then
    cat "$agent_log" >&2
    fail "could not generate temporary repository test key"
fi
fingerprint="$(gpg --homedir "$home" --with-colons --list-secret-keys | awk -F: '$1 == "fpr" { print $10; exit }')"
[[ "$fingerprint" =~ ^[[:xdigit:]]{40}$ ]] || fail "test key fingerprint was not created"

make_deb(){
    local package="$1" arch="$2"
    local dir="$tmp/$package"
    rm -rf "$dir"
    mkdir -p "$dir/DEBIAN"
    cat >"$dir/DEBIAN/control" <<EOF
Package: $package
Version: 1.0.0-1
Architecture: $arch
Maintainer: test <test@example.invalid>
Description: test package
 test package for repository integration
EOF
    mkdir -p "$dir/opt/ie" "$dir/usr/share/intuitionengine"
    cp /bin/true "$dir/opt/ie/IntuitionEngine"
    printf '%s  /opt/ie/IntuitionEngine\n' "$(sha256sum "$dir/opt/ie/IntuitionEngine" | awk '{print $1}')" >"$dir/usr/share/intuitionengine/IntuitionEngine.sha256"
    dpkg-deb --build --root-owner-group "$dir" "$tmp/${package}_1.0.0-1_${arch}.deb" >/dev/null
}
make_deb intuitionengine-amd64-v3 amd64
make_deb intuitionengine-arm64-pi4 arm64
make_deb intuitionengine-arm64-pi5 arm64
make_deb intuitionengine-amd64-v3 arm64
wrong_arch_deb="$tmp/intuitionengine-amd64-v3_1.0.0-1_arm64.deb"
make_deb intuitionengine-amd64-v3 amd64

repo="$tmp/repository"
stage(){
    scripts/stage-intuitionengine-repository.sh \
        --output "$repo" --key-home "$home" --fingerprint "$fingerprint" --app-version 1.0.0 \
        --amd64 "$tmp/intuitionengine-amd64-v3_1.0.0-1_amd64.deb" \
        --pi4 "$tmp/intuitionengine-arm64-pi4_1.0.0-1_arm64.deb" \
        --pi5 "$tmp/intuitionengine-arm64-pi5_1.0.0-1_arm64.deb"
}
stage
for path in \
    pool/main/i/intuitionengine-amd64-v3/intuitionengine-amd64-v3_1.0.0-1_amd64.deb \
    pool/main/i/intuitionengine-arm64-pi4/intuitionengine-arm64-pi4_1.0.0-1_arm64.deb \
    pool/main/i/intuitionengine-arm64-pi5/intuitionengine-arm64-pi5_1.0.0-1_arm64.deb \
    dists/stable/InRelease dists/stable/Release dists/stable/Release.gpg \
    dists/stable/main/binary-amd64/Packages dists/stable/main/binary-amd64/Packages.gz \
    dists/stable/main/binary-arm64/Packages dists/stable/main/binary-arm64/Packages.gz \
    intuitionengine-archive-keyring.gpg; do
    [[ -f "$repo/$path" ]] || fail "repository lacks $path"
done
rg -q '^Package: intuitionengine-amd64-v3$' "$repo/dists/stable/main/binary-amd64/Packages" || fail "amd64 index lacks x64 package"
if rg -q '^Package: intuitionengine-arm64-' "$repo/dists/stable/main/binary-amd64/Packages"; then fail "amd64 index contains ARM packages"; fi
[[ "$(dpkg-deb -f "$repo/pool/main/i/intuitionengine-amd64-v3"/*.deb Architecture)" == amd64 ]] || fail "x64 package architecture changed"
mkdir -p "$tmp/wrong-root"
printf 'public test key\n' >"$tmp/test-keyring.gpg"
if scripts/install-intuitionengine-package.sh --package "$tmp/intuitionengine-amd64-v3_1.0.0-1_amd64.deb" --root "$tmp/wrong-root" --target intuitionengine-arm64-pi5 --app-version 1.0.0 --keyring "$tmp/test-keyring.gpg" >/dev/null 2>&1; then
    fail "installer accepted a wrong-target package"
fi
if scripts/install-intuitionengine-package.sh --package "$wrong_arch_deb" --root "$tmp/wrong-root" --target intuitionengine-amd64-v3 --app-version 1.0.0 --keyring "$tmp/test-keyring.gpg" >/dev/null 2>&1; then
    fail "installer accepted a wrong-architecture package"
fi
install_root="$tmp/install-root"
mkdir -p "$install_root"
scripts/install-intuitionengine-package.sh --package "$tmp/intuitionengine-amd64-v3_1.0.0-1_amd64.deb" --root "$install_root" --target intuitionengine-amd64-v3 --app-version 1.0.0 --keyring "$tmp/test-keyring.gpg" >/dev/null
[[ "$(cat "$install_root/etc/apt/sources.list.d/intuitionengine.list")" == *"https://intuitionengine.io stable main" ]] || fail "installer points at the wrong repository domain"
printf '%s\n' '-----BEGIN PGP PRIVATE KEY BLOCK-----' >"$repo/accidental-private.key"
if stage >/dev/null 2>&1; then fail "repository accepted a private key below output"; fi
rm -f "$repo/accidental-private.key"
gpg --homedir "$home" --batch --yes --export "$fingerprint" | gpg --dearmor >"$tmp/public.gpg"
gpgv --keyring "$tmp/public.gpg" "$repo/dists/stable/InRelease" >/dev/null 2>&1 || fail "InRelease signature does not verify"
gpgv --keyring "$tmp/public.gpg" "$repo/dists/stable/Release.gpg" "$repo/dists/stable/Release" >/dev/null 2>&1 || fail "Release.gpg signature does not verify"
for path in main/binary-amd64/Packages main/binary-amd64/Packages.gz main/binary-arm64/Packages main/binary-arm64/Packages.gz; do
    grep -Fq " $path" "$repo/dists/stable/Release" || fail "Release lacks checksum for $path"
done
if command -v apt-get >/dev/null 2>&1; then
    printf '%s\n' "deb [signed-by=$tmp/public.gpg] file://$repo stable main" >"$tmp/sources.list"
    lists="$tmp/lists"; mkdir -p "$lists/partial" "$tmp/archives/partial"
    apt-get -o Dir::Etc::sourcelist="$tmp/sources.list" -o Dir::Etc::sourceparts=- -o Dir::State::lists="$lists" -o Dir::Cache::archives="$tmp/archives" update >/dev/null 2>&1 || fail "apt-get could not consume repository"
fi

stable_sha="$(sha256sum "$repo/pool/main/i/intuitionengine-amd64-v3"/*.deb | awk '{print $1}')"
printf 'changed package bytes\n' >>"$tmp/intuitionengine-amd64-v3_1.0.0-1_amd64.deb"
if stage >/dev/null 2>&1; then fail "changed package under existing version was published"; fi
[[ "$(sha256sum "$repo/pool/main/i/intuitionengine-amd64-v3"/*.deb | awk '{print $1}')" == "$stable_sha" ]] || fail "stable repository changed after rejected release"
if scripts/stage-intuitionengine-repository.sh --output "$tmp/incomplete" --key-home "$home" --fingerprint "$fingerprint" --app-version 1.0.0 --amd64 "$tmp/intuitionengine-amd64-v3_1.0.0-1_amd64.deb" --pi4 "$tmp/intuitionengine-arm64-pi4_1.0.0-1_arm64.deb" --pi5 "$tmp/missing.deb" >/dev/null 2>&1; then
    fail "incomplete release was accepted"
fi
echo "IntuitionEngine repository integration contracts passed"
