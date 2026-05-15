# Intuition Engine

![68k VGA SID Copper Cube](68k-VGA-SID-Copper-Cube.png)

Intuition Engine is a bootable retro-computing environment and Go emulator for a custom retro-style computer platform. The x64 live USB image is the primary end-user package: it boots straight into Intuition Engine, starts the BASIC environment, and includes demos plus guest OS payloads on a FAT32 share.

For developers, the same repository builds the emulator, SDK tools, live image, and guest integration assets used by that package.

## Live USB Quick Start

The release archive contains:

- `intuition-engine-x64.img` - raw bootable x64 UEFI image.
- `README.md` - this file.

Write the `.img` file to a USB stick with an image writer, then boot it on an x64 UEFI machine. Writing the image replaces the contents of the selected USB drive. From a repository checkout, this boots the locally built image in QEMU:

```bash
make x64-live
make x64-live-qemu
```

On a normal boot, Intuition Engine starts fullscreen at the EhBASIC `Ready` prompt. The live image mounts its FAT32 share as the runtime file area, so paths in BASIC are relative to that share.

Useful first commands:

```basic
DIR
DIR "Demos"
RUN
EMUTOS
AROS
```

Use the exact filenames shown by `DIR`; demo names vary between builds. `RUN "Demos/name.ie64"` and other typed executable files hand off to the matching guest CPU. `LOAD "Demos/name.bas"` loads a BASIC program, then `RUN` starts it. `EMUTOS` and `AROS` boot the bundled guest OS paths when their assets are present.

Live share layout:

| Folder | Purpose |
|--------|---------|
| `Demos` | Bare-metal Intuition Engine demos and shared runtime assets. |
| `IE` | Runtime support files, including coprocessor worker payloads. |
| `SDK` | Reference include files, selected docs, and source examples. Host tool binaries are not included. |
| `Systems/AROS` | AROS `SYS:` root for the live image, including AROS-native demos under `Systems/AROS/Demos`. |
| `Systems/EmuTOS` | EmuTOS GEMDOS drive root, including GEMDOS demos under `Systems/EmuTOS/Demos`. |
| `Systems/IntuitionOS` | IntuitionOS `SYS:` root for the live image; `IOSSYS` is the read-only system subtree and `Boot/iexec.ie64` is the bootstrap kernel. |

Important keys:

| Key | Action |
|-----|--------|
| `F8` | Toggle the Lua REPL overlay, unless the Machine Monitor is active. |
| `F9` | Toggle the Machine Monitor. |
| `F10` | Hard reset. |
| `F11` | Toggle fullscreen. |
| `Shift+F11` | Toggle fit/stretch scaling when the active native mode is not 16:9. |
| `F12` | Toggle the runtime status bar. |
| `Ctrl+Alt` | Release captured relative mouse mode. |

The live image is intended to be usable without manually copying support files. If a bundled demo, OS mode, or service needs a payload, `make x64-live` should stage it automatically.

M15.2 host-backed boot status: `SYS:` is the mounted host-backed boot volume, and `IOSSYS:` as the built-in system assign rooted at `SYS:IOSSYS` contains the read-only IntuitionOS system subtree. The boot chain loads the shell from `IOSSYS:Tools/Shell`.

The project provides:

- Primary guest CPU modes for IE64, IE32, M68K, Z80, 6502, and 32-bit flat x86.
- Optional coprocessor workers that let supported guest programs use more than one emulated CPU.
- Video devices for IEVideoChip, VGA, ULA, TED video, ANTIC/GTIA, and a Voodoo-style 3D path.
- Audio paths for the custom SoundChip, PSG/AY/YM, SN76489 VGM/VGZ, SID, POKEY/SAP, TED, AHX/THX, MOD, WAV, and AROS Paula-style DMA.
- An SDK with assemblers, include files, examples, prebuilt demo outputs, scripts, and documentation.
- Integration paths for EhBASIC, EmuTOS, AROS, and IntuitionOS development.
- Runtime tooling including the Machine Monitor, Lua automation, REPL overlay, screenshots, recording support, and scripted test harnesses.

## Repository Quick Start

Build the VM and start the default BASIC environment:

```bash
make
./bin/IntuitionEngine
```

Build the SDK assets and run a shipped demo:

```bash
make sdk
./bin/IntuitionEngine sdk/examples/prebuilt/vga_text_hello.iex
```

In standard builds, launching without a mode flag and without a filename starts EhBASIC on IE64.

## Contents

1. [Live USB Quick Start](#live-usb-quick-start)
2. [Repository Quick Start](#repository-quick-start)
3. [Current Scope](#current-scope)
4. [Build](#build)
5. [Run](#run)
6. [Runtime Controls](#runtime-controls)
7. [Architecture Summary](#architecture-summary)
8. [SDK and Toolchains](#sdk-and-toolchains)
9. [Testing](#testing)
10. [Platform Support](#platform-support)
11. [Documentation](#documentation)
12. [Licence](#licence)

## Current Scope

### CPU and OS Modes

| Mode flag | Guest/runtime | Positional file support | Notes |
|-----------|---------------|-------------------------|-------|
| `-ie64` | IE64 | `.ie64` | 64-bit RISC core. Used by EhBASIC and IntuitionOS work. |
| `-ie32` | IE32 | `.iex`, `.ie32` | 32-bit RISC core and the original `.iex` executable format. |
| `-m68k` | Motorola 68020-oriented M68K | `.ie68` | Used by native demos, EmuTOS, and AROS work. |
| `-z80` | Z80 | `.ie80` | Z80 guest mode with AY/PSG-oriented examples. |
| `-m6502` | 6502 | `.ie65` | 6502 guest mode with cc65-style SDK support. |
| `-x86` | 32-bit flat x86 | `.ie86` | Flat 32-bit x86 guest mode. |
| `-basic` | EhBASIC on IE64 | none | Uses the embedded BASIC image unless `-basic-image` is supplied. |
| `-emutos` | EmuTOS on M68K | optional ROM via flag | Uses embedded, discovered, or `-emutos-image` ROM assets. |
| `-aros` | AROS on M68K | optional ROM via flag | Uses embedded or `-aros-image` ROM assets and an AROS host drive. |

JIT availability depends on the host OS, host architecture, and guest CPU. Check [Platform Compatibility](sdk/docs/platform-compatibility.md) before relying on JIT for a specific host and guest combination.

### Audio and Video

Supported audio modes are exposed through `-psg`, `-sid`, `-pokey`, `-ted`, `-ahx`, `-mod`, and `-wav`. Enhanced player paths are enabled with `-psg+`, `-sid+`, `-pokey+`, `-ted+`, and `-ahx+`. SID playback also accepts `-sid-pal` and `-sid-ntsc`.

The desktop video path uses Ebiten. The default native video mode is 960x540 and the default presentation frame is 1920x1080 fullscreen. Guest code can select other supported modes through the video MMIO interface. Tests and batch workflows can use the headless backend.

Detailed audio and video references:

- [Architecture](sdk/docs/architecture.md)
- [Sound MMIO](sdk/docs/ie_sfx_mmio.md)
- [WAV Player](sdk/docs/wav_player.md)
- [Compositor](sdk/docs/compositor.md)
- [Voodoo ABI](sdk/docs/ie_voodoo_abi.md)

### Scripting and Debugging

IEScript uses Lua 5.1-compatible semantics through GopherLua. Script modules include `sys`, `cpu`, `mem`, `term`, `audio`, `video`, `repl`, `rec`, `dbg`, `sym`, `regions`, `coproc`, and `media`; scripts also receive `bit32` and `keys` globals.

The Machine Monitor is available with `F9` in desktop builds. It provides CPU, memory, breakpoint, watchpoint, trace, I/O view, and scripting facilities. In desktop builds, guests can request captured relative mouse mode; press `Ctrl+Alt` to release the host mouse and left-click the IE window to recapture while the guest still requests relative mode.

References:

- [IEScript](sdk/docs/iescript.md)
- [Machine Monitor](sdk/docs/iemon.md)
- [Coprocessor](sdk/docs/Coprocessor.md)

## Build

Go 1.26 or later is required. The default Linux desktop build uses CGO and native display, audio, and Vulkan-capable dependencies. Use `novulkan`, `headless`, or `headless-novulkan` when those dependencies are not wanted.

```bash
# Build the VM and core SDK tools
make

# Build only the VM
make intuition-engine

# Build without Vulkan
make novulkan

# Build with stub display and audio backends
make headless

# Build a portable headless binary without CGO
make headless-novulkan
```

Build outputs:

| Output | Produced by |
|--------|-------------|
| `bin/IntuitionEngine` | `make`, `make intuition-engine`, and VM profile targets |
| `sdk/bin/ie32asm` | `make`, `make ie32asm`, `make sdk`, `make sdk-build` |
| `sdk/bin/ie64asm` | `make`, `make ie64asm`, `make sdk`, `make sdk-build` |
| `sdk/bin/ie32to64` | `make`, `make ie32to64`, `make sdk`, `make sdk-build` |
| `sdk/bin/m68kto64` | `make`, `make m68kto64`, `make sdk`, `make sdk-build` |
| `sdk/bin/ie64dis` | `make`, `make ie64dis`, `make sdk`, `make sdk-build` |

Useful build targets:

```bash
make sdk
make sdk-build
make players
make basic
make basic-emutos
make emutos
make aros
make list
make clean
make distclean
```

### x64 Live Image

The x64 live-image workflow builds a bootable raw image and a compressed archive:

```bash
make x64-live
make x64-live-qemu
```

Default outputs:

| Output | Path |
|--------|------|
| Raw image | `build/x64-live/intuition-engine-x64.img` |
| Archive | `build/x64-live/intuition-engine-x64.tar.zst` |

The live-image script requires host image-building tools such as `libguestfs-tools`, `aria2`, `curl`, `qemu-utils`, `mtools`, and `zstd`. It stages the live runtime, SDK/demo payload, EmuTOS assets, AROS assets, IntuitionOS assets, and the files required by bundled services as part of `make x64-live`; these bundled payloads should not require manual copying. Host SDK tool binaries are not bundled on the FAT32 share.

Use [DEVELOPERS.md](DEVELOPERS.md) for full build, release, and contribution details.

## Run

Typed Intuition Engine binaries and IEScript files can be launched directly by extension. Raw binaries, extensionless files, ROM images, and media files require an explicit CPU, OS, or media flag.

### CPU and BASIC Modes

```bash
# Default: start EhBASIC on IE64
./bin/IntuitionEngine

# Run typed guest programs
./bin/IntuitionEngine program.ie64
./bin/IntuitionEngine program.iex
./bin/IntuitionEngine program.ie68
./bin/IntuitionEngine program.ie80
./bin/IntuitionEngine program.ie65
./bin/IntuitionEngine program.ie86

# Run EhBASIC
./bin/IntuitionEngine -basic
./bin/IntuitionEngine -basic -term
./bin/IntuitionEngine -basic-image path/to/ehbasic_ie64.ie64
```

### OS Modes

```bash
# Boot EmuTOS
./bin/IntuitionEngine -emutos
./bin/IntuitionEngine -emutos -emutos-image path/to/emutos.img
./bin/IntuitionEngine -emutos -emutos-drive ~/gemdos-root

# Boot AROS
./bin/IntuitionEngine -aros
./bin/IntuitionEngine -aros -aros-image path/to/aros-ie-m68k.rom
./bin/IntuitionEngine -aros -aros-drive ~/aros-root
```

EmuTOS and AROS availability depends on embedded assets, local default ROM paths, or explicit image paths. See [EmuTOS Integration](sdk/docs/ie_emutos.md) and the AROS sections in [DEVELOPERS.md](DEVELOPERS.md).

### Audio Playback

```bash
./bin/IntuitionEngine -psg music.ym
./bin/IntuitionEngine -sid music.sid
./bin/IntuitionEngine -pokey music.sap
./bin/IntuitionEngine -ted music.ted
./bin/IntuitionEngine -ahx music.ahx
./bin/IntuitionEngine -mod music.mod
./bin/IntuitionEngine -wav sound.wav
```

The media loader recognises additional tracker and chiptune extensions internally, but the CLI does not auto-detect media files. Use the relevant media flag.

### Scripting, Performance, and Display Options

```bash
# Script with no program file starts the default BASIC runtime
./bin/IntuitionEngine -script script.ies

# Script alongside a guest program
./bin/IntuitionEngine -script script.ies program.ie64
./bin/IntuitionEngine program.ie64 -script script.ies

# Performance and interpreter-only runs
./bin/IntuitionEngine -perf program.ie64
./bin/IntuitionEngine -nojit program.ie64

# Display options
./bin/IntuitionEngine -scale 2 program.iex
./bin/IntuitionEngine -fullscreen program.ie68
./bin/IntuitionEngine -width 800 -height 600 program.ie64

# Runtime information
./bin/IntuitionEngine -version
./bin/IntuitionEngine -features
```

### File Opening and Desktop Integration

CLI auto-detection supports these executable extensions:

| Extension | Mode |
|-----------|------|
| `.iex`, `.ie32` | IE32 |
| `.ie64` | IE64 |
| `.ie65` | 6502 |
| `.ie68` | M68K |
| `.ie80` | Z80 |
| `.ie86` | x86 |
| `.ies` | IEScript |

If a desktop build is already running, the runtime helper can also receive `.tos` and `.img` as EmuTOS images through the single-instance path. Desktop file association targets are Linux-oriented:

```bash
sudo make install-desktop-entry
make set-default-handler
```

## Runtime Controls

| Key | Action |
|-----|--------|
| `F8` | Toggle the Lua REPL overlay, unless the Machine Monitor is active. |
| `F9` | Toggle the Machine Monitor. |
| `F10` | Hard reset. |
| `F11` | Toggle fullscreen. |
| `Shift+F11` | Toggle fit/stretch scaling when the active native mode is not 16:9. |
| `F12` | Toggle the runtime status bar. |
| `Page Up` / `Page Down` | Scroll terminal scrollback where supported. |
| Mouse wheel | Scroll terminal scrollback where supported. |

See [Machine Monitor](sdk/docs/iemon.md) for monitor commands, breakpoint/watchpoint syntax, reverse debugging, and multi-CPU debugging.

## Architecture Summary

The central runtime components are:

- `MachineBus`: guest RAM, MMIO dispatch, I/O page fast paths, and guest RAM sizing.
- CPU runners: IE64, IE32, M68K, Z80, 6502, and x86.
- JIT infrastructure: host-specific JIT backends where available.
- Audio engines and players: SoundChip plus chip-specific parsers, players, and MMIO handlers.
- Video engines: IEVideoChip, VGA, ULA, TED video, ANTIC/GTIA, Voodoo, and compositor.
- Debugging: Machine Monitor, disassemblers, breakpoints, watchpoints, traces, and CPU-local snapshots.
- Scripting: IEScript Lua automation and REPL overlay.
- OS integration: EmuTOS loader, AROS loader, GEMDOS/AROS DOS paths, and IntuitionOS runtime work.

Guest RAM is sized at boot from host availability and profile constraints. Guest software can query memory sizing through the SYSINFO MMIO registers and, on IE64, `CR_RAM_SIZE_BYTES`. Detailed memory and MMIO layout information belongs in:

- [Architecture](sdk/docs/architecture.md)
- [IE64 ISA](sdk/docs/IE64_ISA.md)
- [Sound MMIO](sdk/docs/ie_sfx_mmio.md)
- [Voodoo ABI](sdk/docs/ie_voodoo_abi.md)
- [IntuitionOS IExec](sdk/docs/IntuitionOS/IExec.md)

## SDK and Toolchains

The SDK contains include files, examples, prebuilt demo outputs, assets, and scripts under [sdk/](sdk/).

| Guest | Main toolchain | Typical output |
|-------|----------------|----------------|
| IE32 | `sdk/bin/ie32asm` | `.iex` |
| IE64 | `sdk/bin/ie64asm` | `.ie64` |
| M68K | `vasmm68k_mot` | `.ie68` |
| Z80 | `vasmz80_std` | `.ie80` |
| 6502 | `ca65` / `ld65` | `.ie65` |
| x86 | `nasm` | `.ie86` |

Common SDK commands:

```bash
make sdk
make sdk-build
./sdk/scripts/build-all.sh
./bin/IntuitionEngine sdk/examples/prebuilt/vga_text_hello.iex
```

Core SDK references:

- [SDK README](sdk/README.md)
- [SDK Getting Started](sdk/docs/sdk-getting-started.md)
- [Toolchains](sdk/docs/toolchains.md)
- [Include Files](sdk/docs/include-files.md)
- [Demo Matrix](sdk/docs/demo-matrix.md)
- [Tutorial](sdk/docs/TUTORIAL.md)

## Testing

Use the `headless` build tag for normal test runs.

```bash
go test -tags headless ./...
go test -tags headless -run TestName ./...
go test -tags headless -timeout 10m -count=1 ./...
```

Makefile test and check targets:

```bash
make test
make vet
make tidy
make test-makefile
make check-docs
make test-race
make test-harte-short
make test-x86-harte-short
```

Long-running or external-data test suites are opt-in:

```bash
make testdata-harte
make test-harte
make testdata-x86
make test-x86-harte
go test -tags audiolong -run TestSineWave_BasicWaveforms
go test -tags videolong -run TestFireEffect
```

For broad code changes, prefer the verification guidance in [DEVELOPERS.md](DEVELOPERS.md).

## Platform Support

Summary from [Platform Compatibility](sdk/docs/platform-compatibility.md):

| Platform | Architecture | Maintained profiles |
|----------|--------------|---------------------|
| Linux | x86_64 | `full`, `novulkan`, `headless`, `headless-novulkan` |
| Linux | aarch64 | `full`, `novulkan`, `headless`, `headless-novulkan` |
| Windows | x86_64 | `novulkan` |
| Windows | ARM64 | `novulkan` |
| macOS | x86_64 | `novulkan` |
| macOS | ARM64 | `novulkan` |

Linux amd64 and arm64 support the full profile, including the Vulkan-backed Voodoo HLE path. The full Linux profile is a CGO build and depends on native display, audio, C runtime, and Vulkan libraries.

Windows and macOS release builds are maintained on amd64 and arm64 as Pure Go `novulkan` builds. These builds have no CGO or third-party native runtime dependencies, but the Vulkan Voodoo path is disabled and the software Voodoo rasteriser is used instead. Use `headless-novulkan` for a portable no-CGO build with no display or audio backend.

## Documentation

- [Developer Guide](DEVELOPERS.md)
- [Platform Compatibility](sdk/docs/platform-compatibility.md)
- [Architecture](sdk/docs/architecture.md)
- [SDK README](sdk/README.md)
- [SDK Getting Started](sdk/docs/sdk-getting-started.md)
- [Toolchains](sdk/docs/toolchains.md)
- [Include Files](sdk/docs/include-files.md)
- [Demo Matrix](sdk/docs/demo-matrix.md)
- [IEScript](sdk/docs/iescript.md)
- [Machine Monitor](sdk/docs/iemon.md)
- [Coprocessor](sdk/docs/Coprocessor.md)
- [IE64 ISA](sdk/docs/IE64_ISA.md)
- [IE64 ABI](sdk/docs/IE64_ABI.md)
- [EmuTOS Integration](sdk/docs/ie_emutos.md)
- [IntuitionOS IExec](sdk/docs/IntuitionOS/IExec.md)

Additional hardware, JIT, OS, and tutorial references live under [sdk/docs/](sdk/docs/).

## Licence

Intuition Engine is distributed under GPLv3 or later. See [LICENSE](LICENSE).

Project links:

- <https://github.com/intuitionamiga/IntuitionEngine>
- <https://www.youtube.com/@IntuitionAmiga/>
