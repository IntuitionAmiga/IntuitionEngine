#!/usr/bin/env bash
set -euo pipefail
root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fail(){ echo "FAIL: $*" >&2; exit 1; }
cd "$root_dir"
[[ -f RELEASE_INTUITIONENGINE_PACKAGES.md ]] || fail "missing package release documentation"
[[ -f VERSION ]] || fail "missing canonical VERSION file"
[[ "$(cat VERSION)" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "VERSION is not semantic"
rg -q 'IE_RELEASE_KEY_FINGERPRINT' RELEASE_INTUITIONENGINE_PACKAGES.md || fail "release documentation does not require a fingerprint"
for target in bump-intuitionengine-version deb-intuitionengine-amd64-v3 deb-intuitionengine-arm64-pi4 deb-intuitionengine-arm64-pi5 deb-intuitionengine release-intuitionengine release-intuitionengine-repository test-intuitionengine-release-secrets; do
    rg -q "^${target}:" Makefile || fail "missing Make target: $target"
done
[[ -x scripts/bump-intuitionengine-version.sh ]] || fail "missing version bump script"
bump_tmp="$(mktemp -d)"
trap 'rm -rf "$bump_tmp"' EXIT
version_tmp="$bump_tmp/VERSION"
html_tmp="$bump_tmp/index.html"
printf '%s\n' 2.3.9 >"$version_tmp"
printf '%s\n' \
    '<a>Download the Intuition Engine 2.3.9 live image</a>' \
    '<a>Download the Intuition Engine 2.3.9 live image</a>' >"$html_tmp"
scripts/bump-intuitionengine-version.sh --file "$version_tmp" --html-file "$html_tmp" >/dev/null
[[ "$(cat "$version_tmp")" == 2.3.10 ]] || fail "version bump did not increment the patch version"
[[ "$(rg -o 'Download the Intuition Engine 2\.3\.10 live image' "$html_tmp" | wc -l)" == 2 ]] || fail "version bump did not update both live-image links"
current_version="$(cat VERSION)"
[[ "$(rg -o "Download the Intuition Engine ${current_version} live image" intuitionengine.com/index.html | wc -l)" == 2 ]] || fail "website live-image links do not match VERSION"
make check-intuitionengine-app-version >/dev/null || fail "the Makefile default APP_VERSION was rejected"
make APP_VERSION=1.2.3 check-intuitionengine-app-version >/dev/null || fail "valid APP_VERSION was rejected"
for script in build-intuitionengine-deb.sh stage-intuitionengine-repository.sh install-intuitionengine-package.sh merge-intuitionengine-dpkg-status.sh; do
    [[ -x "scripts/$script" ]] || fail "missing executable package script: $script"
done
[[ -x scripts/check-intuitionengine-release-secrets.sh ]] || fail "missing executable release secret preflight"
rg -q 'dpkg-deb --build' scripts/build-intuitionengine-deb.sh || fail "package build does not use dpkg-deb"
rg -q 'GOOS=linux GOARCH=amd64 GOAMD64=v3' Makefile || fail "x64 package input is not built as amd64 v3"
rg -q 'build-rpi-binary,pi4,v8\.0,cortex-a72' Makefile || fail "Pi 4 package input is not Cortex-A72 ARM64 v8.0"
rg -q 'build-rpi-binary,pi5,v8\.2,cortex-a76' Makefile || fail "Pi 5 package input is not Cortex-A76 ARM64 v8.2"
rg -q 'dpkg-scanpackages' scripts/stage-intuitionengine-repository.sh || fail "repository indexes are not generated"
rg -q -- '--clearsign' scripts/stage-intuitionengine-repository.sh || fail "InRelease is not signed"
rg -q -- '--detach-sign' scripts/stage-intuitionengine-repository.sh || fail "Release.gpg is not signed"
rg -q 'check-intuitionengine-release-secrets\.sh' scripts/stage-intuitionengine-repository.sh || fail "repository staging does not run release secret preflight"
rg -q 'IntuitionEngine.previous' scripts/build-intuitionengine-deb.sh || fail "upgrade backup is missing"
if rg -q 'previous' scripts/build-intuitionengine-deb.sh && rg -q 'IntuitionEngine\.previous' scripts/build-intuitionengine-deb.sh; then :; else fail "upgrade backup contract is missing"; fi
rg -q 'cp -p /opt/ie/IntuitionEngine /opt/ie/IntuitionEngine.previous' scripts/build-intuitionengine-deb.sh || fail "preinst does not save the active binary"
rg -q 'exec /usr/lib/intuitionengine/package-check' scripts/build-intuitionengine-deb.sh || fail "postinst does not invoke package-check"
rg -q 'systemctl restart greetd\.service' scripts/build-intuitionengine-deb.sh || fail "package-check does not restart greetd"
rg -q 'systemctl is-active --quiet greetd\.service' scripts/build-intuitionengine-deb.sh || fail "package-check does not check greetd state"
rg -q 'gpg --show-keys --with-colons' build_x64_ie_img.sh || fail "x64 builder does not validate the public keyring"
rg -q 'gpg --show-keys --with-colons' scripts/build_rpi_live_image.sh || fail "Pi builder does not validate the public keyring"
rg -q 'IE_PACKAGE_FILE=.*intuitionengine-amd64-v3' build_x64_ie_img.sh || fail "x64 builder does not select the x64 package"
rg -q 'install-intuitionengine-package\.sh' build_x64_ie_img.sh || fail "x64 builder does not extract the package"
rg -q -- '--app-version "\$IE_APP_VERSION"' build_x64_ie_img.sh || fail "x64 builder does not enforce package version"
rg -q 'cmp -s "\$package_root/opt/ie/IntuitionEngine" "\$IE_BINARY"' build_x64_ie_img.sh || fail "x64 builder does not enforce package binary identity"
rg -q -- '--base-status "\$base_status"' build_x64_ie_img.sh || fail "x64 builder does not preserve the golden dpkg database"
rg -q -- '--app-version "\$app_version"' scripts/build_rpi_live_image.sh || fail "Pi builder does not enforce package version"
rg -q -- '--base-status "\$base_status"' scripts/build_rpi_live_image.sh || fail "Pi builder does not preserve the source dpkg database"
rg -q 'merge-intuitionengine-dpkg-status\.sh' scripts/install-intuitionengine-package.sh || fail "installer does not replace stale engine package records"
rg -q 'actual_checksum=.*sha256sum "\$staged_binary"' scripts/install-intuitionengine-package.sh || fail "installer does not verify the staged binary checksum"
if rg -q 'sha256sum -c "\$root/usr/share/intuitionengine/IntuitionEngine\.sha256"' scripts/install-intuitionengine-package.sh; then
    fail "installer still verifies the checksum manifest against the host root"
fi
rg -q -- '--package .*intuitionengine-arm64-pi4' Makefile || fail "Pi 4 image target does not select the Pi 4 package"
rg -q -- '--package .*intuitionengine-arm64-pi5' Makefile || fail "Pi 5 image target does not select the Pi 5 package"
rg -q 'https://intuitionengine\.io stable main' scripts/install-intuitionengine-package.sh || fail "source list is not fixed to live HTTPS repository"
rg -Fq 'Disallow: /pool/' intuitionengine.com/robots.txt || fail "pool is crawlable"
rg -Fq 'Disallow: /dists/' intuitionengine.com/robots.txt || fail "dists is crawlable"
if find intuitionengine.com -type f -print | rg -q '(^|/)(private|secret|.*\.key$)'; then fail "private release key material is below website root"; fi
if command -v dpkg-deb >/dev/null 2>&1; then
    tmp="$(mktemp -d)"
    trap 'rm -rf "$tmp"' EXIT
    cp /bin/true "$tmp/IntuitionEngine"
    printf '\n1.0.0\n' >>"$tmp/IntuitionEngine"
    scripts/build-intuitionengine-deb.sh --target intuitionengine-amd64-v3 --app-version 1.0.0 --binary "$tmp/IntuitionEngine" --output-dir "$tmp/deb" >/dev/null
    contents="$(dpkg-deb -c "$tmp/deb"/*.deb)"
    for path in /opt/ie/IntuitionEngine /usr/lib/intuitionengine/package-check /usr/share/intuitionengine/IntuitionEngine.sha256; do
        printf '%s\n' "$contents" | rg -q "${path//\//\\/}$" || fail "package lacks $path"
    done
    printf '%s\n' "$contents" | rg -q '/opt/ie/IntuitionEngine.previous' && fail "package owns the previous binary"
    extract="$tmp/extract"
    dpkg-deb --extract "$tmp/deb"/*.deb "$extract"
    [[ "$(find "$extract" -type f | sort | sed "s#^$extract##")" == $'/opt/ie/IntuitionEngine\n/usr/lib/intuitionengine/package-check\n/usr/share/intuitionengine/IntuitionEngine.sha256' ]] || fail "package contains unexpected data files"
    expected_checksum="$(awk 'NF == 2 && $2 == "/opt/ie/IntuitionEngine" { print $1 }' "$extract/usr/share/intuitionengine/IntuitionEngine.sha256")"
    actual_checksum="$(sha256sum "$extract/opt/ie/IntuitionEngine" | awk '{print $1}')"
    [[ "$expected_checksum" == "$actual_checksum" ]] || fail "package checksum does not match installed binary"
else
    echo "dpkg-deb unavailable: artefact build check skipped"
fi
status_tmp="$(mktemp -d)"
trap 'rm -rf "$status_tmp"' EXIT
cat >"$status_tmp/status" <<'EOF'
Package: libc6
Status: install ok installed
Version: 2.0

Package: intuitionengine-arm64-pi4
Status: install ok installed
Version: 1.0.0-1

Package: systemd
Status: install ok installed
Version: 1.0
EOF
scripts/merge-intuitionengine-dpkg-status.sh --input "$status_tmp/status" --output "$status_tmp/filtered"
rg -q '^Package: libc6$' "$status_tmp/filtered" || fail "dpkg status merge removed libc6"
rg -q '^Package: systemd$' "$status_tmp/filtered" || fail "dpkg status merge removed systemd"
if rg -q '^Package: intuitionengine-' "$status_tmp/filtered"; then fail "dpkg status merge retained an old engine record"; fi
echo "IntuitionEngine package contracts passed"
