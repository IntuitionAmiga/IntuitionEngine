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

Each release target builds the platform archives, embeds the ROM images, and stages the runtime data tree:

```bash
make release-linux      # Linux amd64 + arm64 (.tar.xz)
make release-windows    # Windows amd64 + arm64 (.zip)
make release-macos      # macOS amd64 + arm64 (.tar.xz)

make release-src        # Source archive via git archive (.tar.xz)
make release-sdk        # Standalone SDK archive (.zip)

make release-all        # All of the above + SHA256SUMS
```

Each platform archive contains:
- `IntuitionEngine` or `IntuitionEngine.exe` at the archive root
- `README.md`, `CHANGELOG.md`, `DEVELOPERS.md`
- the full `sdk/` tree, including `sdk/intuitionos/system/SYS`
- `sdk/bin/` with `ie32asm`, `ie64asm`, `ie32to64`, `ie64dis`
- an `AROS/` directory copied from the staged AROS build tree

All release binaries are built with `-tags "novulkan embed_basic embed_emutos embed_aros"` on Windows and macOS, and `embed_basic embed_emutos embed_aros` on Linux.

### Build Details

**Linux**

Builds `amd64` and `arm64` `.tar.xz` archives. Linux keeps the full CGO desktop stack and can use Vulkan in non-`novulkan` builds.

**Windows**

Builds `amd64` and `arm64` `.zip` archives with `CGO_ENABLED=0` and the `novulkan` profile. Windows `amd64` has full guest JIT parity with Linux `amd64`; Windows `arm64` ships the IE64 arm64 JIT.

**macOS**

Builds `amd64` and `arm64` `.tar.xz` archives with `CGO_ENABLED=0` and the `novulkan` profile. The `amd64` archive ships the shared x86-64 guest JIT backends; the `arm64` archive uses the native IE64 arm64 JIT.

## Release Artifacts

| Platform | Architecture | Format | Profile |
|----------|-------------|--------|---------|
| Linux | amd64 | `.tar.xz` | full |
| Linux | arm64 | `.tar.xz` | full |
| Windows | amd64 | `.zip` | novulkan |
| Windows | arm64 | `.zip` | novulkan |
| macOS | amd64 | `.tar.xz` | novulkan |
| macOS | arm64 | `.tar.xz` | novulkan |

## Checksums

`make release-all` generates SHA256 checksums automatically (covering `.tar.xz` and `.zip` artifacts). To generate manually:

```bash
cd release/
sha256sum *.tar.xz *.zip 2>/dev/null > SHA256SUMS
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
3. For macOS testers, document the quarantine workaround: `xattr -dr com.apple.quarantine .`
4. Announce on project channels
