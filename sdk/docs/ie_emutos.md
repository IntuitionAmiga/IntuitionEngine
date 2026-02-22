# EmuTOS on Intuition Engine

EmuTOS is a free GPLv2 replacement for Atari TOS with GEM desktop support.
Intuition Engine runs EmuTOS directly on the IE M68K core with full access to all IE MMIO devices (no Atari ST chipset emulation). TOS .PRG programs can drive any hardware register including all audio chips (SoundChip, PSG, SID, POKEY, TED, AHX) and video chips (VideoChip, VGA, ULA, TED video, ANTIC/GTIA, Voodoo 3D).

## Current Status

Implemented in IE runtime:
- M68K interrupt pending bitmask with multi-source delivery
- EmuTOS ROM loader (`192K -> 0xFC0000`, `256K+/512K+ -> 0xE00000`)
- Timer IRQ level 5 at 200Hz and VBlank IRQ level 4 at 60Hz
- Mouse MMIO registers in terminal region (`0xF0730` block)
- Raw ST-style scancode + modifier MMIO (`0xF0740` block)
- GEMDOS TRAP #1 filesystem interception — maps a host directory as drive U: in the GEM desktop
- IOREC keyboard buffer detection and initialization from ROM pattern scan
- ProgramExecutor support for `.tos` and `.img`

## Quick Start

External ROM image:

```bash
./IntuitionEngine -emutos-image etos256us.img
```

Embedded ROM build:

```bash
make emutos
./bin/IntuitionEngine -emutos
```

`make emutos` build order:
1. If `sdk/emutos/etos256us.img` exists, use it.
2. Else, clone EmuTOS into `sdk/emutos-src/` (default URL/ref) and build it.
3. Auto-copy first ROM artifact found into `sdk/emutos/etos256us.img`.

You can override source/build:

```bash
make emutos EMUTOS_SRC_DIR=../emutos EMUTOS_BUILD_CMD='make -C ../emutos'
```

You can override clone source/ref:

```bash
make emutos EMUTOS_GIT_URL=https://github.com/<you>/emutos.git EMUTOS_GIT_REF=my-ie-branch
```

Default EmuTOS build target is `256` (`EMUTOS_BUILD_TARGET=256`).
Default build command is `EMUTOS_BUILD_CMD=auto`:
- uses MiNT toolchain when `m68k-atari-mint-gcc` exists
- falls back to ELF toolchain (`ELF=1`) when `m68k-elf-gcc` exists
- also tries GNU/Linux M68K cross (`m68k-linux-gnu-gcc`) with `ELF=1 TOOLCHAIN_PREFIX=m68k-linux-gnu-`

Toolchain requirement:
- `m68k-atari-mint-gcc` (default EmuTOS toolchain), or
- `m68k-elf-gcc` if using an ELF build (`ELF=1` via `EMUTOS_BUILD_CMD`).
- `m68k-linux-gnu-gcc` may also work via auto fallback.

From EhBASIC:

```basic
RUN "emutos.img"
```

## Build EmuTOS for IE (high-level)

1. Build EmuTOS with `-m68020` CPU flags.
2. Add an IE machine target in EmuTOS using `sdk/emutos/*` skeleton files.
3. Bind platform input/output/timer code to the MMIO map below.
4. Produce ROM image (`.img`/`.tos`) and launch via `-emutos-image`.

## Pixel Format

VRAM is RGBA32 packed as:

```c
(R << 24) | (G << 16) | (B << 8) | A
```

Framebuffer base is `0x100000` at `640x480` (`stride=2560`).

## IE Hardware Map (EmuTOS target)

EmuTOS has full access to the complete IE hardware map. The key registers for EmuTOS-specific I/O are listed below; see `registers.go` and `ie68.inc` for the full map.

| Register | Address | Purpose |
|---|---:|---|
| `VIDEO_CTRL` | `0xF0000` | video enable |
| `VIDEO_MODE` | `0xF0004` | `0=640x480` |
| `VIDEO_STATUS` | `0xF0008` | bit 1 `in_vblank` |
| `BLT_OP` | `0xF0020` | blitter operation (fill, copy, Mode7) |
| `TERM_OUT` | `0xF0700` | debug output |
| `MOUSE_X` | `0xF0730` | absolute X |
| `MOUSE_Y` | `0xF0734` | absolute Y |
| `MOUSE_BUTTONS` | `0xF0738` | bit0=left bit1=right bit2=middle |
| `MOUSE_STATUS` | `0xF073C` | bit0 changed since last read |
| `SCAN_CODE` | `0xF0740` | dequeues make/break scancode |
| `SCAN_STATUS` | `0xF0744` | bit0 available |
| `SCAN_MODIFIERS` | `0xF0748` | bit0 shift bit1 ctrl bit2 alt bit3 caps |
| `AUDIO_CTRL` | `0xF0800` | SoundChip registers |
| `PSG` | `0xF0C00+` | YM2149 PSG registers |
| `SID` | `0xF0C40+` | SID registers |
| `TED` | `0xF0C80+` | TED audio registers |
| `POKEY` | `0xF0CC0+` | POKEY registers |
| `AHX_PLAY_PTR` | `0xF0B84` | AHX module playback pointer |
| `AHX_PLAY_LEN` | `0xF0B88` | AHX module data length |
| `AHX_PLAY_CTRL` | `0xF0B8C` | AHX play control (bit0=start, bit1=stop, bit2=loop) |
| `VGA_BASE` | `0xF1000+` | VGA registers |
| `ULA_BASE` | `0xF2000+` | ULA registers |
| `ANTIC_BASE` | `0xF2100+` | ANTIC/GTIA registers |
| `VOODOO_BASE` | `0xF4000+` | Voodoo 3D registers |
| `VRAM` | `0x100000+` | RGBA32 framebuffer (4MB) |

## GEM Application Programming (.PRG)

TOS .PRG executables can be built with `vasmm68k_mot -Ftos` and run from the GEM desktop.

### Build a GEM .PRG

```bash
vasmm68k_mot -Ftos -m68020 -devpac -Isdk/include \
  -o myapp.prg myapp.asm
```

The `-Ftos` output format produces a proper TOS executable with header and relocation table. The `-Isdk/include` flag gives access to `ie68.inc` for blitter/video MMIO constants.

A convenience Makefile target is available for the example rotozoomer:

```bash
make gem-rotozoomer
```

### Run a .PRG under EmuTOS

```bash
mkdir -p /tmp/tos_drive
cp myapp.prg /tmp/tos_drive/MYAPP.PRG
./bin/IntuitionEngine -emutos -emutos-drive /tmp/tos_drive/
```

Navigate to drive U: in the GEM desktop and double-click the .PRG file.

### Blitter Access from .PRG Programs

The IE hardware blitter is fully accessible from GEM applications via the standard MMIO registers (`BLT_OP` at `$F0020`, `BLT_CTRL` at `$F001C`, etc.). In EmuTOS mode, the blitter writes directly to bus memory (the same memory backing the VRAM framebuffer), so blitter output is immediately visible.

Key points for GEM blitter usage:
- Enable the VideoChip first: `move.l #1,VIDEO_CTRL`
- VRAM base is `$100000`, stride is `2560` bytes (640 pixels x 4 bytes)
- Mode7 affine texture mapping works identically to bare-metal mode
- Use `wind_update(BEG_UPDATE/END_UPDATE)` to prevent AES conflicts during rendering
- Iterate the rectangle list (`WF_FIRSTXYWH/WF_NEXTXYWH`) for proper GEM redraw

### Example: GEM Windowed Rotozoomer

See `sdk/examples/asm/rotozoomer_gem.asm` for a complete GEM application that opens a window and renders an animated Mode7 rotozoomer with AHX music. It demonstrates:
- TOS .PRG crt0 startup (basepage, Mshrink, stack setup)
- GEM AES/VDI initialization (appl_init, v_opnvwk, wind_create/open)
- evnt_multi event loop with MU_MESAG + MU_TIMER
- WM_REDRAW rectangle list clipping protocol
- WM_MOVED window repositioning
- Hardware Mode7 blitter rendering into a GEM window
- AHX music playback via MMIO (incbin + PLAY_AHX_LOOP macro)

## Monitor and Debugging

- Press `F9` to toggle monitor.
- Timer IRQ goroutines are pause-safe: monitor freeze (`cpu.Running()==false`) skips ticks but keeps goroutines alive.
- On resume, timer/vblank assertions continue.

## Troubleshooting

- `make emutos` fails with missing ROM:
  - Place ROM at `sdk/emutos/etos256us.img` for embedded builds.
- Boot fails with `-emutos` and no embedded image:
  - Use `-emutos-image <path>`.
- Mouse/keyboard not responding:
  - Verify you are using Ebiten output (not headless).
- `.img` launched from BASIC but no switch:
  - Ensure extension is `.img` or `.tos` and file is in allowed run path.
