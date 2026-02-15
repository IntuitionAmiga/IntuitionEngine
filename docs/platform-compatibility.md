# Platform Compatibility

Supported platforms, build profiles, and known limitations for Intuition Engine v1.0.

## Platform Matrix

| Platform | Architecture | Status | Build Profile | Notes |
|----------|-------------|--------|---------------|-------|
| Linux | x86_64 | **Official** | `full` | Primary development platform |
| Linux | aarch64 | **Official** | `full` | |
| macOS | x86_64 | Experimental | `novulkan` | No Vulkan support |
| macOS | ARM64 | Experimental | `novulkan` | No Vulkan support |
| Windows | x86_64 | Experimental | `novulkan` | No Vulkan support |
| Windows | ARM64 | Experimental | `novulkan` | No Vulkan support |
| FreeBSD | x86_64 | Experimental | `novulkan` | No Vulkan support |
| FreeBSD | ARM64 | Experimental | `novulkan` | No Vulkan support |
| NetBSD | x86_64 | Experimental | `novulkan` | No Vulkan support |
| NetBSD | ARM64 | Experimental | `novulkan` | No Vulkan support |
| OpenBSD | x86_64 | Experimental | `novulkan` | No Vulkan support |
| OpenBSD | ARM64 | Experimental | `novulkan` | No Vulkan support |

**Official** platforms are fully tested and supported. **Experimental** platforms compile and run but may have untested edge cases.

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
- CGO enabled
- C compiler

**Features:** Ebiten display, Oto audio, software Voodoo rasteriser.

**Use this for:** macOS, Windows, and Linux systems without Vulkan.

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
| Ebiten | Linux, macOS, Windows | OpenGL / Metal / DirectX | Default, hardware-accelerated |
| Headless | All | None | Stub for testing |

Ebiten provides:
- Hardware-accelerated rendering
- Automatic display scaling
- VSync synchronisation
- Cross-platform window management

## Audio Backends

| Backend | Platforms | Output | Notes |
|---------|-----------|--------|-------|
| Oto | Linux, macOS, Windows | 44.1kHz stereo | Default, low-latency (~20ms) |
| Headless | All | None | Stub for testing |

## Known Limitations

### macOS (Experimental)
- Vulkan Voodoo path not available (use `novulkan`)
- LHA decompression uses pure-Go fallback

### Windows (Experimental)
- Vulkan Voodoo path not available (use `novulkan`)
- LHA decompression uses pure-Go fallback
- Desktop integration (`.desktop` files, MIME types) is Linux-only

### BSD (FreeBSD, NetBSD, OpenBSD)
- Vulkan Voodoo path not available (use `novulkan`)
- Cross-compiled with `CGO_ENABLED=0`
- Desktop integration (`.desktop` files, MIME types) is Linux-only

### Cross-Compilation
- Release targets use `CGO_ENABLED=0` with the `novulkan` profile for cross-architecture builds
- Use `make release-linux`, `make release-windows`, etc. for automated cross-compilation
- Full and novulkan profiles require CGO and may need platform-specific sysroot for native builds

## Runtime Feature Detection

```bash
# Print compiled-in features
./bin/IntuitionEngine -features

# Print version and build metadata
./bin/IntuitionEngine -version
```
