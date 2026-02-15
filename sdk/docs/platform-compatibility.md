# Platform Compatibility

Supported platforms, build profiles, and known limitations for Intuition Engine v1.0.

## Platform Matrix

| Platform | Architecture | Status | Build Profile | Notes |
|----------|-------------|--------|---------------|-------|
| Linux | x86_64 | **Official** | `full` | Primary development platform |
| Linux | aarch64 | **Official** | `full` | |
| Windows | x86_64 | Experimental | `novulkan` | No Vulkan support |
| Windows | ARM64 | Experimental | `novulkan` | No Vulkan support |

**Official** platforms are fully tested and supported. **Experimental** platforms compile and run but may have untested edge cases.

macOS and BSD variants (FreeBSD, NetBSD, OpenBSD) are not currently supported for release builds because ebiten and oto require CGO on those platforms, preventing cross-compilation. They may compile from source on native hardware with the `novulkan` profile.

## Build Profile Requirements

### full (default)

The complete build with all features enabled.

**Requirements:**
- Go 1.26+
- CGO enabled
- C compiler (gcc or clang)
- Vulkan SDK and drivers (for Voodoo GPU path)
- liblhasa development headers (for LHA decompression on Linux)
- `sstrip` and `upx` (for binary optimisation, optional)

**Features:** Ebiten display, Oto audio, Vulkan Voodoo rasteriser, software Voodoo rasteriser, LHA decompression.

### novulkan

Software-only Voodoo rasteriser. Removes the Vulkan SDK dependency.

**Requirements:**
- Go 1.26+
- CGO enabled (Linux) / auto-disabled (Windows cross-compile)
- C compiler (Linux native builds)

**Features:** Ebiten display, Oto audio, software Voodoo rasteriser.

**Use this for:** Windows and Linux systems without Vulkan.

### headless

No display, no audio. For CI/testing and batch processing.

**Requirements:**
- Go 1.26+
- CGO enabled
- C compiler

**Features:** Stub display/audio backends, software Voodoo rasteriser.

### headless-novulkan

Fully portable build with no CGO dependencies. Cross-compile safe.

**Requirements:**
- Go 1.26+

**Features:** Stub display/audio backends, software Voodoo rasteriser. No native dependencies.

**Use this for:** Cross-compilation, CI environments without C toolchains, embedded deployment.

```bash
CGO_ENABLED=0 go build -tags "novulkan headless" .
```

## Graphics Backends

| Backend | Platforms | Rendering | Notes |
|---------|-----------|-----------|-------|
| Ebiten | Linux, Windows | OpenGL / DirectX | Default, hardware-accelerated |
| Headless | All | None | Stub for testing |

Ebiten provides:
- Hardware-accelerated rendering
- Automatic display scaling
- VSync synchronisation
- Cross-platform window management

## Audio Backends

| Backend | Platforms | Output | Notes |
|---------|-----------|--------|-------|
| Oto | Linux, Windows | 44.1kHz stereo | Default, low-latency (~20ms) |
| Headless | All | None | Stub for testing |

## Known Limitations

### Windows (Experimental)
- Vulkan Voodoo path not available (use `novulkan`)
- LHA decompression uses pure-Go fallback
- Desktop integration (`.desktop` files, MIME types) is Linux-only

### Cross-Compilation
- Linux release builds are native-arch only (ebiten/oto require CGO for GLFW/X11/ALSA)
- Windows cross-compilation works from Linux (ebiten/oto are pure Go on Windows)
- Use `make release-linux`, `make release-windows`, or `make release-all` for automated builds

## Runtime Feature Detection

```bash
# Print compiled-in features
./bin/IntuitionEngine -features

# Print version and build metadata
./bin/IntuitionEngine -version
```
