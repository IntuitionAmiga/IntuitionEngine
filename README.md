# Intuition Engine

![Intuition Engine splash](splash.png)

Intuition Engine is a modern 64-bit RISC fantasy computer implemented in Go. It reimagines 1980s and 1990s home-computer ideas as one machine, not as a clone of any single system: the BASIC prompt, IE64 and IE32 processors, M68K, Z80, 6502, x86, display chips, sound engines, DMA hardware, input devices, file devices, and monitor all sit on one shared MachineBus.

It can be run as a desktop emulator or booted as an x64 live USB appliance. For programmers, it is a bare-metal target with an SDK, examples, and maintained reference documentation for writing directly against the Intuition Engine hardware.

## Quick Start

Build the emulator and core SDK tools:

```bash
make
./bin/IntuitionEngine
```

Launching with no mode flag and no filename starts EhBASIC on IE64.

Build SDK examples and run a demo:

```bash
make sdk
./bin/IntuitionEngine sdk/examples/prebuilt/vga_text_hello.iex
```

## Features

- Guest CPU modes: IE64, IE32, Motorola 68020-oriented M68K, Z80, 6502, and 32-bit flat x86.
- JIT backends where supported by host OS and architecture.
- Video devices: IEVideoChip, VGA, ULA, TED video, ANTIC/GTIA, compositor, and a Voodoo-style 3D path.
- Audio and music paths: custom SoundChip, PSG/AY/YM/SN76489, SID, POKEY/SAP, TED, AHX/THX, MOD, WAV, MIDI/MUS, and AROS Paula-style DMA.
- Guest environments: EhBASIC, EmuTOS, AROS, and IntuitionOS.
- Runtime tooling: Machine Monitor, Lua/IEScript automation, REPL overlay, screenshots, recording support, and scripted test harnesses.
- SDK tools: IE32/IE64 assemblers, IE64 disassembler, IE32-to-IE64 converter, M68K-to-IE64 transpiler, include files, examples, and documentation.

## Build

Go 1.26 or later is required.

The default build shown in the quick start produces the emulator at `bin/IntuitionEngine` and core SDK tools under `sdk/bin/`.

## Run

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
| `.mid`, `.midi`, `.mus` | MIDI/MUS player |

Raw binaries, ROM images, EmuTOS `.tos`/`.img` files, and most audio formats require an explicit flag:

```bash
./bin/IntuitionEngine -basic
./bin/IntuitionEngine -basic -term
./bin/IntuitionEngine -basic-image path/to/ehbasic_ie64.ie64

./bin/IntuitionEngine -emutos
./bin/IntuitionEngine -emutos-image path/to/emutos.img
./bin/IntuitionEngine -emutos-drive path/to/gemdos-root

./bin/IntuitionEngine -aros
./bin/IntuitionEngine -aros-image path/to/aros-ie-m68k.rom
./bin/IntuitionEngine -aros-drive path/to/aros-root

./bin/IntuitionEngine -intuitionos
./bin/IntuitionEngine -intuitionos-root sdk/intuitionos/system/SYS
./bin/IntuitionEngine -intuitionos-image sdk/intuitionos/iexec/iexec.ie64
```

Audio playback examples:

```bash
./bin/IntuitionEngine -psg music.ym
./bin/IntuitionEngine -sid music.sid
./bin/IntuitionEngine -pokey music.sap
./bin/IntuitionEngine -ted music.ted
./bin/IntuitionEngine -ahx music.ahx
./bin/IntuitionEngine -mod music.mod
./bin/IntuitionEngine -wav sound.wav
./bin/IntuitionEngine -midi song.mid
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

## Runtime Controls

| Key | Action |
|-----|--------|
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

The main output formats are `.iex` for IE32, `.ie64` for IE64, `.ie68` for M68K, `.ie80` for Z80, `.ie65` for 6502, and `.ie86` for x86.

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

| Platform | Architecture | Maintained profiles |
|----------|--------------|---------------------|
| Linux | x86_64 | `full`, `novulkan`, `headless`, `headless-novulkan` |
| Linux | aarch64 | `full`, `novulkan`, `headless`, `headless-novulkan` |
| Windows | x86_64 | `novulkan` |
| Windows | ARM64 | `novulkan` |
| macOS | x86_64 | `novulkan` |
| macOS | ARM64 | `novulkan` |

JIT availability depends on host OS, host architecture, and guest CPU.

## Documentation

- [Architecture](sdk/docs/architecture.md)
- [IE64 ISA](sdk/docs/IE64_ISA.md)
- [IE32 ISA](sdk/docs/IE32_ISA.md)
- [Machine Monitor](sdk/docs/iemon.md)
- [IEScript](sdk/docs/iescript.md)
- [Intuition Engine Programmer's Reference Guide](sdk/docs/refman.publish/)

## License

Intuition Engine is distributed under GPLv3 or later. See `LICENSE`.

YouTube: <https://www.youtube.com/@IntuitionAmiga/>
