# Platform Compatibility

Supported platforms, build profiles, and known limitations for Intuition Engine v1.0.

## Platform Matrix

| Platform | Architecture | Status | Build Profile | Notes |
|----------|-------------|--------|---------------|-------|
| Linux | x86_64 | **Official** | `full`, `novulkan`, `headless`, `headless-novulkan` | Primary development platform |
| Linux | aarch64 | **Official** | `full`, `novulkan`, `headless`, `headless-novulkan` | Native JITs and Linux ARM64 SIMD kernels |
| Windows | x86_64 | **Official** | `novulkan` | Pure-Go release build, full guest JIT parity with Linux x64 |
| Windows | ARM64 | **Official** | `novulkan` | Pure-Go release build, IE64 and M68K native JITs |
| macOS | x86_64 | **Official** | `novulkan` | Pure-Go release build, full guest JIT parity with Linux and Windows x64 |
| macOS | ARM64 | **Official** | `novulkan` | Pure-Go release build, IE64 and M68K native JITs via `MAP_JIT` |

**Official** platforms are built in CI and have maintained release packaging targets. BSD variants remain out of scope.

## Build Profile Requirements

### full (default)

The complete build with all features enabled.

**Requirements:**
- Go 1.27rc2+
- CGO enabled on Linux
- C compiler (gcc or clang) for Linux native builds
- Vulkan SDK and drivers (for Voodoo GPU path)
- `sstrip` and `upx` (for binary optimisation, optional)

**Features:** Ebiten display, Oto audio, Vulkan Voodoo rasteriser, software Voodoo rasteriser.

### novulkan

Software-only Voodoo rasteriser. Removes the Vulkan SDK dependency.

**Requirements:**
- Go 1.27rc2+
- CGO enabled on Linux native builds
- C compiler for Linux native builds
- No CGO toolchain required for Windows or macOS cross-builds

**Features:** Ebiten display, Oto audio, software Voodoo rasteriser.

**Use this for:** Windows releases, macOS releases, and Linux systems without Vulkan.

### headless

No display, no audio. For CI/testing and batch processing.

**Requirements:**
- Go 1.27rc2+
- CGO enabled on Linux
- C compiler for Linux native runs

**Features:** Stub display/audio backends, software Voodoo rasteriser. The IE Voodoo bus ABI and software backend feature contract are covered by headless tests.

### headless-novulkan

Headless build with the software Voodoo renderer and native JITs.

**Requirements:**
- Go 1.27rc2+

**Features:** Stub display and audio backends, software Voodoo rasteriser and native JITs.

**Use this for:** Headless testing without Vulkan.

```bash
CGO_ENABLED=1 go build -tags "novulkan headless" .
```

## Graphics Backends

| Backend | Platforms | Rendering | Notes |
|---------|-----------|-----------|-------|
| Ebiten | Linux, Windows, macOS | OpenGL / DirectX / Metal | Default, hardware-accelerated |
| Headless | All | None | Stub for testing |
| Voodoo software | All profiles | CPU rasteriser | Contract reference for Voodoo HLE |
| Voodoo Vulkan | Linux full profile | Vulkan | Must match software output for ABI-implemented features |

Ebiten provides:
- Hardware-accelerated rendering
- Automatic display scaling
- VSync synchronisation
- Cross-platform window management

## Audio Backends

| Backend | Platforms | Output | Notes |
|---------|-----------|--------|-------|
| Oto | Linux, Windows, macOS | 44.1kHz stereo | Default, low-latency (~20ms) |
| Headless | All | None | Stub for testing |

## Known Limitations

### Windows
- Vulkan Voodoo path not available (use `novulkan`)
- Desktop integration (`.desktop` files, MIME types) is Linux-only

### macOS
- `novulkan` profile only
- Release artifacts are ad-hoc binaries; testers may need `xattr -dr com.apple.quarantine .` after download
- amd64 builds have full guest JIT parity with Linux/Windows amd64
- arm64 builds use the native IE64 and M68K JIT backends; other guest cores remain interpreter-only on arm64

### Cross-Compilation
- Linux release builds remain native-arch or Linux-cross-toolchain builds because the full profile still uses CGO
- Windows amd64, Windows arm64, macOS amd64, and macOS arm64 cross-compilation work from Linux with `CGO_ENABLED=0` under `novulkan`
- Use `make release-linux`, `make release-windows`, `make release-macos`, or `make release-all` for automated builds

## Runtime Feature Detection

```bash
# Print compiled-in features
./bin/IntuitionEngine -features

# Print version and build metadata
./bin/IntuitionEngine -version
```
