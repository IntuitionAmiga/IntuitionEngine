# Include File Reference

Hardware definition include files for Intuition Engine programs. Each file provides register constants, memory map definitions, and helper macros for its target CPU architecture.

## File Summary

| File | CPU | Assembler | Description |
|------|-----|-----------|-------------|
| `ie32.inc` | IE32 | ie32asm | Hardware constants (`.equ` directives) |
| `ie64.inc` | IE64 | ie64asm | Hardware constants and macros |
| `ie64_fp.inc` | IE64 | ie64asm | IEEE 754 FP32 math library |
| `ie65.inc` | 6502 | ca65 | Constants, macros, zero page allocation |
| `ie65.cfg` | 6502 | ld65 | Linker configuration |
| `ie68.inc` | M68K | vasmm68k_mot | Constants with M68K macros |
| `ie80.inc` | Z80 | vasmz80_std | Constants with Z80 macros |
| `ie86.inc` | x86 | NASM | Constants, port I/O, VGA registers |

## Common Definitions

All include files provide these categories of definitions:

### Video Registers
- `VIDEO_CTRL` / `VIDEO_MODE` / `VIDEO_STATUS` - Display control
- `BLT_OP` / `BLT_SRC` / `BLT_DST` / `BLT_WIDTH` / `BLT_HEIGHT` - Blitter
- `COP_PTR` / `COP_CTRL` - Copper coprocessor

### Blitter Operations
- `BLT_OP_COPY` - Rectangular copy
- `BLT_OP_FILL` - Rectangular fill
- `BLT_OP_LINE` - Line draw
- `BLT_OP_MASKED` - Masked copy (transparency)
- `BLT_OP_ALPHA` - Alpha-blended copy
- `BLT_OP_MODE7` - SNES-style rotation/scaling

### Audio Registers
- PSG: `PSG_REG_*`, `PSG_PLAY_*`
- SID: `SID_*`, `SID_PLAY_*`
- POKEY: `POKEY_*`, `SAP_PLAY_*`
- TED: `TED_*`
- AHX: `AHX_*`

### Memory Constants
- `VRAM_START` - Start of video RAM
- `SCREEN_W` / `SCREEN_H` - Display dimensions
- `LINE_BYTES` - Bytes per scanline

### Timer
- `TIMER_CTRL` / `TIMER_COUNT` / `TIMER_RELOAD`

## Per-CPU Details

### ie32.inc

Uses `.equ` directives. No macros (IE32 assembler has limited macro support).

```assembly
.include "ie32.inc"

start:
    LOAD A, #1
    STORE A, @VIDEO_CTRL
    LOAD A, #BLT_OP_FILL
    STORE A, @BLT_OP
```

### ie64.inc

Uses `equ` constants and extensive macros.

```assembly
    include "ie64.inc"

start:
    move.l r2, #1
    store.l r2, VIDEO_CTRL(r0)
    wait_vblank
    set_blt_color $FF00FF00
    start_blit
```

**Macros:** `wait_vblank`, `wait_blit`, `start_blit`, `set_blt_color`, `set_blt_src`, `set_blt_dst`, `set_blt_size`, `set_blt_strides`, `set_copper_ptr`, `enable_copper`, `disable_copper`, VGA (`vga_setmode`, `vga_enable`, `vga_setpalette`, etc.), ULA (`set_ula_border`, `ula_enable`), TED video, ANTIC/GTIA, PSG/SID/SAP/AHX/POKEY player control, audio channels, Voodoo 3D, coprocessor helpers.

### ie68.inc

Uses `equ` constants and M68K macros.

```assembly
    include "ie68.inc"

start:
    move.l  #1,VIDEO_CTRL.l
    wait_vblank
    set_blt_color $FF00FF00
    start_blit
```

**Macros:** `wait_vblank`, `wait_blit`, `start_blit`, `set_blt_color`, `set_blt_src`, `set_blt_dst`, `set_blt_size`, `set_blt_strides`, `set_copper_ptr`, `enable_copper`, `disable_copper`, PSG/SID/SAP/AHX player macros, coprocessor helpers.

### ie65.inc

The most comprehensive include file. Uses `.define` and ca65 macros. Provides zero page allocation.

```assembly
.include "ie65.inc"

.segment "CODE"
start:
    lda  #1
    sta  VIDEO_CTRL
    WAIT_VBLANK
    SET_BLT_OP BLT_OP_FILL
    SET_BLT_COLOR $FF00FF00
    START_BLIT
```

**Zero page layout:**
```
zp_ptr0    .res 2    ; General purpose pointer 0
zp_ptr1    .res 2    ; General purpose pointer 1
zp_tmp0    .res 4    ; 32-bit temporary 0
zp_frame   .res 2    ; Frame counter
zp_scratch .res 8    ; Scratch space
```

**Macros:** `SET_BANK1`..`SET_BANK3`, `SET_VRAM_BANK`, `STORE16`, `STORE32`, `STORE32_ZP`, `WAIT_VBLANK`, `WAIT_BLIT`, `START_BLIT`, `SET_BLT_OP/WIDTH/HEIGHT/COLOR`, `SET_SRC_STRIDE`, `SET_DST_STRIDE`, `ADD16`, `INC16`, `CMP16`, AHX player macros, coprocessor helpers.

### ie80.inc

Uses `.set` constants and Z80 macros.

```assembly
    .include "ie80.inc"

start:
    ld   sp,STACK_TOP
    ld   a,1
    ld   (VIDEO_CTRL),a
    WAIT_VBLANK
    SET_BLT_OP BLT_OP_FILL
    START_BLIT
```

**Macros:** `SET_BANK1`..`SET_BANK3`, `SET_VRAM_BANK`, `STORE16`, `STORE32`, `WAIT_VBLANK`, `WAIT_BLIT`, `START_BLIT`, `SET_BLT_*`, `SET_COPPER_PTR`, PSG/SID/SAP/AHX player macros, `SID_WRITE`, `ADD_HL_IMM`, `CP_HL_IMM`, `INC16`, coprocessor helpers.

### ie86.inc

Uses `equ` constants and NASM macros. Unique: supports both memory-mapped and port I/O access.

```assembly
%include "ie86.inc"

section .text
start:
    mov     eax, 1
    mov     [VIDEO_CTRL], eax
    wait_vblank
    psg_write PSG_REG_MIXER, 0x38
    vga_wait_vsync
```

**Port I/O addresses:**
- PSG: ports `0xF0`-`0xF1`
- POKEY: ports `0xD0`-`0xDF`
- SID: ports `0xE0`-`0xE1`
- TED: ports `0xF2`-`0xF3`
- VGA: standard PC ports (`0x3C4`-`0x3DA`)

**Macros:** `wait_vblank`, `vga_wait_vsync`, `psg_write`, `sid_write`, `pokey_write`, coprocessor helpers.

## Stability Policy

The `sdk/include/` directory is the canonical location for all include files. Hardware register definitions (`ie*.inc`) and EhBASIC modules (`ehbasic_*.inc`) live here.
