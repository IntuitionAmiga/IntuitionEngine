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

5. **Build SDK** (syncs includes and pre-assembles demos):
   ```bash
   make sdk
   ```

6. **Check version output**:
   ```bash
   ./bin/IntuitionEngine -version
   ./bin/IntuitionEngine -features
   ```

## Building Release Artifacts

### Using Makefile Targets (Recommended)

Each release target builds both amd64 and arm64 archives, assembles the EhBASIC ROM, and embeds it in the binary:

```bash
make release-linux      # Linux amd64 + arm64 (.tar.xz)
make release-windows    # Windows amd64 + arm64 (.zip)
make release-macos      # macOS amd64 + arm64 (.tar.xz)
make release-freebsd    # FreeBSD amd64 + arm64 (.tar.xz)
make release-netbsd     # NetBSD amd64 + arm64 (.tar.xz)
make release-openbsd    # OpenBSD amd64 + arm64 (.tar.xz)

make release-all        # All platforms + SHA256SUMS
```

Each archive contains: `IntuitionEngine`, `ie32asm`, `ie64asm`, `ie32to64`, `README.md`, `CHANGELOG.md`, and the full `sdk/` directory with pre-assembled demos.

### Build Details

**Linux (Official)**

The native architecture gets a full CGO build with sstrip/upx compression. The cross architecture uses `CGO_ENABLED=0` with the `novulkan` profile.

**Windows, macOS, FreeBSD, NetBSD, OpenBSD (Experimental)**

Cross-compiled with `CGO_ENABLED=0` and the `novulkan` profile. No UPX compression (cannot compress foreign-architecture binaries).

All release builds include the `embed_basic` tag, embedding the EhBASIC interpreter so the VM starts a BASIC prompt by default.

## Release Artifacts

| Platform | Architecture | Format | Profile |
|----------|-------------|--------|---------|
| Linux | amd64, arm64 | `.tar.xz`, `.AppImage` | full (native) / novulkan (cross) |
| Windows | amd64, arm64 | `.zip` | novulkan |
| macOS | amd64, arm64 | `.tar.xz` | novulkan |
| FreeBSD | amd64, arm64 | `.tar.xz` | novulkan |
| NetBSD | amd64, arm64 | `.tar.xz` | novulkan |
| OpenBSD | amd64, arm64 | `.tar.xz` | novulkan |

## Checksums

`make release-all` generates SHA256 checksums automatically. To generate manually:

```bash
cd release/
sha256sum *.tar.xz *.zip > SHA256SUMS
```

## Tagging

```bash
git tag -a v1.0.0 -m "Intuition Engine v1.0.0"
git push origin v1.0.0
```

## CI/CD

Release builds are triggered by pushing a version tag (`v*`). See `.github/workflows/release.yml` for the automated pipeline.

Test builds run on every push. See `.github/workflows/test.yml` for the CI pipeline.

## Post-Release

1. Create GitHub release with tag, attach artifacts and `SHA256SUMS`
2. Update `sdk/include/` if hardware register maps changed (or run `make sdk`)
3. Announce on project channels
