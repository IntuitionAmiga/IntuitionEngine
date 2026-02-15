# Release Process

How to build, package, and publish Intuition Engine releases.

## Version Scheme

Intuition Engine uses [Semantic Versioning](https://semver.org/):
- **Major**: Breaking changes to the hardware register map or binary format
- **Minor**: New features, new CPU cores, new hardware chips
- **Patch**: Bug fixes, performance improvements, documentation updates

The version is defined in `Makefile` as `APP_VERSION` and injected at build time via ldflags.

## Pre-Release Checklist

1. **Update version** in `Makefile`:
   ```makefile
   APP_VERSION := 1.0.0
   ```

2. **Update CHANGELOG.md** with release notes

3. **Verify all build profiles**:
   ```bash
   make clean
   make                                    # Full build
   make novulkan                          # No Vulkan
   make headless                          # Headless
   make headless-novulkan                 # Portable
   ```

4. **Run tests**:
   ```bash
   go test -tags headless ./...
   ```

5. **Verify SDK examples build**:
   ```bash
   ./sdk/scripts/build-all.sh
   ```

6. **Sync SDK includes** from `assembler/*.inc` to `sdk/include/`

7. **Check version output**:
   ```bash
   ./bin/IntuitionEngine -version
   ./bin/IntuitionEngine -features
   ```

## Building Release Artifacts

### Linux (Official)

```bash
# x86_64
make clean && make
# Produces: bin/IntuitionEngine, bin/ie32asm, bin/ie64asm

# AppImage
make appimage
# Produces: IntuitionEngine-1.0.0-x86_64.AppImage

# Archive
tar -cJf IntuitionEngine-1.0.0-linux-amd64.tar.xz \
  -C bin IntuitionEngine ie32asm ie64asm
```

For aarch64, build on an ARM64 host or cross-compile with appropriate sysroot.

### Windows (Experimental)

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -tags "novulkan headless" \
  -ldflags "-s -w -X main.Version=1.0.0 -X main.Commit=$(git rev-parse --short HEAD) -X main.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o IntuitionEngine.exe .
```

Package as `.zip`.

### macOS (Experimental)

```bash
GOOS=darwin GOARCH=arm64 \
  go build -tags novulkan \
  -ldflags "-s -w -X main.Version=1.0.0 -X main.Commit=$(git rev-parse --short HEAD) -X main.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o IntuitionEngine .
```

Package as `.tar.xz`.

## Release Artifacts

| Platform | Architecture | Format | Profile |
|----------|-------------|--------|---------|
| Linux | x86_64 | `.AppImage`, `.tar.xz` | full |
| Linux | aarch64 | `.AppImage`, `.tar.xz` | full |
| Windows | x86_64 | `.zip` | novulkan |
| Windows | aarch64 | `.zip` | novulkan |
| macOS | ARM64 | `.tar.xz` | novulkan |

## Checksums

Generate SHA256 checksums for all release artifacts:

```bash
sha256sum IntuitionEngine-*.AppImage IntuitionEngine-*.tar.xz IntuitionEngine-*.zip \
  > SHA256SUMS
```

## Tagging

```bash
git tag -a v1.0.0 -m "Intuition Engine v1.0.0"
git push origin v1.0.0
```

## CI/CD

Release builds are triggered by pushing a version tag (`v*`). See `.github/workflows/release.yml` for the automated pipeline (Phase 7 of the release plan).

Test builds run on every push. See `.github/workflows/test.yml` for the CI pipeline.

## Post-Release

1. Create GitHub release with tag, attach artifacts and `SHA256SUMS`
2. Update `sdk/include/` if hardware register maps changed
3. Announce on project channels
