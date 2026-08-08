# Intuition Engine

![Intuition Engine splash](splash.png)

Intuition Engine is a multi-CPU fantasy computer implemented in Go. It reimagines 1980s and 1990s home-computer ideas as one machine, not as a clone of any single system. Six heterogeneous processors share one MachineBus: IE64, IE32, M68K, Z80, 6502, and x86. The BASIC prompt, display chips, sound engines, DMA hardware, input devices, file devices, and monitor use that same machine architecture.

It can be run as a desktop emulator or booted as an x64 live USB appliance. For programmers, it is a bare-metal target with an SDK, examples, and maintained reference documentation for writing directly against the Intuition Engine hardware.

Native builds derive guest RAM from host memory, then apply a platform reserve
and the selected profile's active-visible ceiling. Browser builds use a fixed
256 MiB heap backing. Guest software discovers total and active-visible RAM
through SYSINFO and CPU-specific paths.

[Try Intuition Engine in a browser](https://intuitionengine.io) | [Download releases](https://github.com/IntuitionAmiga/IntuitionEngine/releases) | [Read the architecture guide](sdk/docs/architecture.md) | [Watch demonstrations on YouTube](https://www.youtube.com/@IntuitionAmiga/)

## Quick Start

Build the emulator and core SDK tools:

```bash
make
./bin/IntuitionEngine
```

Launching with no mode flag and no filename starts EhBASIC on IE64.

The default build uses the Ebiten display, Oto audio, and Vulkan-backed Voodoo renderer. It requires the Go toolchain selected by `go.mod` and the native development libraries required by those backends. If the Vulkan SDK is unavailable, build the emulator with the software Voodoo renderer instead:

```bash
make novulkan
./bin/IntuitionEngine
```

Build SDK examples and run a demo:

```bash
make sdk
./bin/IntuitionEngine sdk/examples/prebuilt/vga_text_hello.iex
```

## Features

- Guest CPU modes: IE64, IE32, Motorola 68020-oriented M68K, Z80, 6502, and 32-bit flat x86.
- JIT backends for all six guest CPUs, with availability determined by the host OS and architecture.
- A Coprocessor Manager that launches additional CPU workers to run concurrently with the primary CPU on the shared MachineBus.
- Video systems: VideoChip, VGA, TED video, ANTIC/GTIA, ULA, and Voodoo 3D, combined through a layered compositor.
- Audio and music paths: custom SoundChip, PSG/AY/YM/SN76489, SID, POKEY/SAP, TED, AHX/THX, MOD, WAV, MIDI/MUS, and AROS Paula-style DMA.
- Guest environments: EhBASIC, EmuTOS, AROS, and IntuitionOS.
- Runtime tooling: Machine Monitor, Lua/IEScript automation, REPL overlay, screenshots, recording support, and scripted test harnesses.
- SDK tools: IE32 and IE64 assemblers, IE64 disassembler, IE32-to-IE64 converter, M68K-to-IE64 transpiler, IE64 C compiler, static linker and archive tools, include files, examples, and documentation.

## Build

Requires Go 1.26.0 or later. The default amd64 build enables the experimental `simd/archsimd` API.

The default build shown in the quick start produces the emulator at `bin/IntuitionEngine` and core SDK tools under `sdk/bin/`.

On amd64, `make` builds enable SIMD acceleration and target the x86-64-v3 baseline
by default. The live image inherits both settings. `IE_SIMD=0` selects the
bit-exact scalar span kernels at runtime, but it does not lower the binary's
x86-64-v3 CPU requirement. For an older x86-64 host, build outside the Makefile
with a suitable lower `GOAMD64` value. For `go run .` outside `make`, enable the
SIMD experiment once with `go env -w GOEXPERIMENT=simd`, or leave it unset to use
the scalar kernels.

## Run

<details>
<summary>Command-line formats, playback modes, and runtime flags</summary>

Typed Intuition Engine binaries and IEScript files can be launched directly by extension:

```bash
./bin/IntuitionEngine program.ie64
./bin/IntuitionEngine program.iex
./bin/IntuitionEngine program.ie68
./bin/IntuitionEngine program.ie80
./bin/IntuitionEngine program.ie65
./bin/IntuitionEngine program.ie86
./bin/IntuitionEngine demo.ies
```

CLI auto-detection supports:

| Extension | Mode |
|-----------|------|
| `.iex`, `.ie32` | IE32 |
| `.ie64` | IE64 |
| `.ie65` | 6502 |
| `.ie68` | M68K |
| `.ie80` | Z80 |
| `.ie86` | x86 |
| `.ies` | IEScript |
| `.sid` | SID player |
| `.ym`, `.ay`, `.sndh`, `.vtx`, `.vt`, `.pt3`, `.pt2`, `.pt1`, `.stc`, `.sqt`, `.asc`, `.ftc`, `.vgm`, `.vgz`, `.snd` | PSG player |
| `.ted`, `.prg` | TED player |
| `.ahx` | AHX player |
| `.sap` | POKEY player |
| `.mod` | MOD player |
| `.wav` | WAV player |
| `.mid`, `.midi`, `.mus` | MIDI/MUS player |

Raw binaries, ROM images, and EmuTOS `.tos`/`.img` files require an explicit flag:

```bash
./bin/IntuitionEngine -basic
./bin/IntuitionEngine -basic -term
./bin/IntuitionEngine -basic-image path/to/ehbasic_ie64.ie64

./bin/IntuitionEngine -emutos
./bin/IntuitionEngine -emutos-image path/to/emutos.img
./bin/IntuitionEngine -emutos -emutos-drive path/to/gemdos-root

./bin/IntuitionEngine -aros
./bin/IntuitionEngine -aros-image path/to/aros-ie-m68k.rom
./bin/IntuitionEngine -aros -aros-drive path/to/aros-root

./bin/IntuitionEngine -intuitionos
./bin/IntuitionEngine -intuitionos -intuitionos-root sdk/intuitionos/system/SYS
./bin/IntuitionEngine -intuitionos -intuitionos-image sdk/intuitionos/iexec/iexec.ie64
```

Audio playback examples:

```bash
./bin/IntuitionEngine music.ym
./bin/IntuitionEngine music.sid
./bin/IntuitionEngine music.sap
./bin/IntuitionEngine music.ted
./bin/IntuitionEngine music.ahx
./bin/IntuitionEngine music.mod
./bin/IntuitionEngine sound.wav
./bin/IntuitionEngine song.mid

# Enhanced playback remains opt-in
./bin/IntuitionEngine -psg+ music.ym
./bin/IntuitionEngine -sid+ music.sid
./bin/IntuitionEngine -pokey+ music.sap
./bin/IntuitionEngine -ted+ music.ted
./bin/IntuitionEngine -ahx+ music.ahx
```

Useful runtime flags:

```bash
./bin/IntuitionEngine -script script.ies program.ie64
./bin/IntuitionEngine -perf program.ie64
./bin/IntuitionEngine -nojit program.ie64
./bin/IntuitionEngine -fullscreen program.ie68
./bin/IntuitionEngine -width 800 -height 600 program.ie64
./bin/IntuitionEngine -version
./bin/IntuitionEngine -features
```

</details>

## Runtime Controls

| Key | Action |
|-----|--------|
| `F7` | Cycle the default-on CRT presentation filter through flat, curved, and off. The key also reaches the guest when no host overlay is consuming keyboard input. |
| `F8` | Toggle the Lua REPL overlay, unless the Machine Monitor is active. |
| `F9` | Toggle the Machine Monitor. |
| `F10` | Hard reset to the configured boot profile; normal BASIC-launched sessions return to BASIC. |
| `F11` | Toggle fit/stretch scaling when the active native mode is not 16:9. |
| `Shift+F11` | Toggle fullscreen/windowed mode outside locked live-image sessions. |
| `F12` | Toggle the runtime status bar. |
| `Ctrl+Alt` | Release captured relative mouse mode. |

## SDK

The SDK lives under `sdk/` and includes toolchains, include files, examples, prebuilt demo outputs, scripts, and maintained reference documentation.

Core SDK tool outputs:

| Tool | Purpose |
|------|---------|
| `sdk/bin/ie32asm` | IE32 assembler |
| `sdk/bin/ie64asm` | IE64 assembler |
| `sdk/bin/ie32to64` | IE32-to-IE64 converter |
| `sdk/bin/m68kto64` | M68K-to-IE64 transpiler |
| `sdk/bin/ie64dis` | IE64 disassembler |
| `sdk/bin/ie64-cproc` | IE64 freestanding C compiler driver |
| `sdk/bin/ie64ld` | IE64 static linker |
| `sdk/bin/ie64-ar` | IE64 static archive tool |
| `sdk/bin/ie64-ranlib` | IE64 static archive indexer |

The main output formats are `.iex` for IE32, `.ie64` for IE64, `.ie68` for M68K, `.ie80` for Z80, `.ie65` for 6502, and `.ie86` for x86.

The [Linux x86-64 Host SDK](sdk/docs/host-sdk-README.md) packages these tools with QBE, cproc-qbe, the IE64 runtime and libraries, public assembly includes, the target-selected C hardware header, and user documentation. Build its distributable archive with `make dist-host-sdk-linux-amd64`.

## Live USB Image

The optional x64 live-image workflow builds a bootable raw UEFI image and a compressed archive:

```bash
make x64-live
```

Default outputs:

| Output | Path |
|--------|------|
| Raw image | `build/x64-live/intuition-engine-x64.img` |
| Archive | `build/x64-live/intuition-engine-x64.zip` |

The image boots into Intuition Engine, starts the BASIC environment, and stages demos plus guest OS payloads on a FAT32 share. The image builder needs host image-building tools such as libguestfs, QEMU utilities, mtools, rsync, curl/aria2, and enough free disk space for the build workspace.

## Platform Support

Maintained profiles:

| Platform | Architecture | Maintained profiles | JIT-enabled guest CPUs |
|----------|--------------|---------------------|------------------------|
| Linux | x86_64 | `full`, `novulkan`, `headless`, `headless-novulkan` | IE32, IE64, M68K, 6502, Z80 and x86 |
| Linux | aarch64 | `full`, `novulkan`, `headless`, `headless-novulkan` | IE32, IE64, M68K, 6502, Z80 and x86 |
| Windows | x86_64 | `novulkan` | IE64, M68K, Z80 and x86 |
| Windows | ARM64 | `novulkan` | IE64 and M68K |
| macOS | x86_64 | `novulkan` | IE64, M68K, Z80 and x86 |
| macOS | ARM64 | `novulkan` | IE64 and M68K |
| Browser | WebAssembly | `make wasm` | IE32, IE64, M68K, 6502, Z80 and x86 |

SIMD span acceleration (`simd/archsimd`) is amd64 only and default-on for `make`
builds; every other architecture falls back to the bit-exact scalar kernels
automatically.

Unsupported JIT operations and host platforms retain interpreter fallback. See
the [architecture guide](sdk/docs/architecture.md#platform-jit-matrix) for the
backend and dispatch details.

## Documentation

- [Architecture](sdk/docs/architecture.md)
- [IE64 ISA](sdk/docs/IE64_ISA.md)
- [IE32 ISA](sdk/docs/IE32_ISA.md)
- [Machine Monitor](sdk/docs/iemon.md)
- [IEScript](sdk/docs/iescript.md)
- [Intuition Engine Programmer's Reference Guide](sdk/docs/refman.publish/)
- [Developer guide](DEVELOPERS.md)

## Licence

Intuition Engine is distributed under GPLv3 or later. See `LICENSE`.
