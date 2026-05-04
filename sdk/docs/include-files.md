# Include File Reference

Hardware definition include files for Intuition Engine programs. Each file provides register constants, memory map definitions, and helper macros for its target CPU architecture.

## File Summary

| File | CPU | Assembler | Description |
|------|-----|-----------|-------------|
| `iexec.inc` | IE64 / IntuitionOS | ie64asm | IntuitionOS kernel ABI, syscall, task/image-layout, and startup-block constants |
| `ie32.inc` | IE32 | ie32asm | Hardware constants (`.equ` directives) |
| `ie64.inc` | IE64 | ie64asm | Hardware constants and macros |
| `ie64_fp.inc` | IE64 | ie64asm | IEEE 754 FP32 math library |
| `ie65.inc` | 6502 | ca65 | Constants, macros, zero page allocation |
| `ie65.cfg` | 6502 | ld65 | Linker configuration |
| `ie68.inc` | M68K | vasmm68k_mot | Constants with M68K macros |
| `ie80.inc` | Z80 | vasmz80_std | Constants with Z80 macros |
| `ie86.inc` | x86 | NASM | Constants, port I/O, VGA registers |

## Assembler Notes

`ie32asm` include recursion is path-stack based, so the same include can be used again after a nested include returns. `.ascii` and `.asciz` process `\n`, `\t`, `\r`, `\\`, `\"`, `\0`, and `\xHH` escapes.

`ie64asm` source strings and character literals preserve semicolons inside quotes. String escapes additionally accept `\xHH` and `\uHHHH`; `\u` is emitted as UTF-8 bytes. Conditional assembly supports `if` / `elseif` / `else` / `endif`.

Both built-in assemblers cache `incbin` payload bytes between layout and emission passes so changing the binary file during one assembly cannot silently corrupt output. `ie64asm` also reruns pass 1 when size-affecting directives contain forward references.

## Common Definitions

All include files provide these categories of definitions:

## IntuitionOS Include

### iexec.inc

`sdk/include/iexec.inc` is the canonical IntuitionOS contract include for IE64 assembly programs and kernel-side service code. It defines:

- syscall numbers and `SYSINFO_*` query IDs
- kernel data structure offsets
- user-space image/PT window constants and legacy slot-layout constants
- `hardware.resource` grant constants
- M13 startup block constants (`TASK_STARTUP_*`, `TASKSB_*`)

The startup block constants describe the 64-byte kernel-populated record written into a dedicated startup page for each launched task. Boot-loaded and `ExecProgram`-launched services discover the startup-page base VA from `0(sp)`, then read this block to find task identity and actual code/data/stack bases without deriving addresses from `CURRENT_TASK * USER_SLOT_STRIDE`.

The M14 native executable contract is documented separately in `sdk/docs/IntuitionOS/ELF.md`. As of M14/M14.2, `iexec.inc` also ships the DOS loader/launch protocol constants for `DOS_LOADSEG`, `DOS_UNLOADSEG`, `DOS_RUNSEG`, the DOS-owned seglist layout (`DOS_SEGLIST_*`, `DOS_SEG_*`), and the descriptor-only `ExecProgram` launch constants (`M14_LDESC_*`, `M14_LDSEG_*`). The old flat-image `ExecProgram` ABI is removed at the public runtime boundary: strict native ELF commands and apps go through the DOS descriptor path, and shipped boot/services use the internal embedded-manifest ELF path. As of M15, `iexec.inc` also exports the DOS assign protocol constants for `DOS_ASSIGN` and its `LIST` / `QUERY` / `SET` operations together with the canonical DOS-layout constants used by the built-in assign table.

As of **M15.3**, `iexec.inc` adds three more `DOS_ASSIGN` sub-ops and their payload sizing constants:

- `DOS_ASSIGN_LAYERED_QUERY` (3) — share-in: name, share-out: `count × target[DOS_ASSIGN_LAYERED_TGT_SZ]` (32-byte slots, NUL-padded). `reply.data0` = effective count. Returns the FULL ordered list: overlay entries first, then the built-in base list, with duplicate targets collapsed.
- `DOS_ASSIGN_ADD` (4) — share-in: `row[name[16], target[16]]`. Appends `target` to the canonical layered overlay; duplicate-add is a no-op. Rejects `RAM:`, `T:`, `SYS:`, `IOSSYS:`, and any non-canonical user assign with `DOS_ERR_BADARG`.
- `DOS_ASSIGN_REMOVE` (5) — share-in: `row[name[16], target[16]]`. Removes one target from the mutable overlay. Built-in base entries cannot be removed (they remain visible through `DOS_ASSIGN_LAYERED_QUERY`).
- `DOS_ASSIGN_LAYERED_TGT_SZ` (32) — bytes per target slot in the LAYERED_QUERY response.
- `DOS_ASSIGN_OVERLAY_MAX` (4) — max overlay entries per canonical layered assign.

The compatibility ops (`DOS_ASSIGN_LIST`, `DOS_ASSIGN_QUERY`, `DOS_ASSIGN_SET`) keep their M15.2 semantics through a first-effective-target projection.

`sdk/docs/IntuitionOS/Toolchain.md` is the canonical IOS-native codegen contract. `iexec.inc` remains the ABI/include contract, not the full loader/toolchain spec.

As of **M16.4.1**, `MODF_ASLR_CAPABLE` is operational and runtime metadata
lives in `PT_NOTE`. DOS-loaded and protected runtime ELFs must be stripped,
section-header-free self-contained IE64 `ET_DYN` images with zero-relative `PT_LOAD` addresses,
no section header table, one `PT_NOTE` carrying `IOS-MOD` plus optional
`IOS-REL`, class-correct IOSM flags, and local bounded relocation metadata.
`section-header-only` metadata is rejected. Commands use exactly
`MODF_ASLR_CAPABLE`, while libraries, devices, handlers, and resources use
exactly `MODF_COMPAT_PORT | MODF_ASLR_CAPABLE`. The phrase dynamic linking remains
unsupported, protected `.library` use remains message/port based,
host-provided and third-party DOS ELFs have no `ET_EXEC` compatibility
exception, trusted-internal launch requires trusted read-only system source
provenance plus validated IOSM metadata, userland ASLR is enabled, and
M16.4.1 only prepares for KASLR. M16.5 owns fixed kernel VA blockers including
`KERN_PAGE_TABLE`, `KERN_DATA_BASE`, `KERN_STACK_TOP`, supervisor identity
mapping, trap/fault paths, scheduler state, panic/debug paths, and task
page-table kernel mapping copies. W^X, SKEF/SKAC/SUA discipline, bounded
inputs, and shared-memory `MAPF_READ` / `MAPF_WRITE` rules remain mandatory.

As of **M15.6**, `iexec.inc` adds the CPU-level SMEP/SMAP-equivalent controls and the supervisor-user-access latch opcodes so kernel-side assembly can reference them symbolically:

- `MMU_CTRL_ENABLE` (bit 0) — MMU translation enable (already established).
- `MMU_CTRL_SUPER` (bit 1) — supervisor mode, read-only.
- `MMU_CTRL_SKEF` (bit 2) — supervisor-kernel-execute-fault enable. When set, a supervisor instruction fetch from a page with `PTE_U==1` faults with `FAULT_SKEF`.
- `MMU_CTRL_SKAC` (bit 3) — supervisor-kernel-access-check enable. When set, a supervisor read or write on a page with `PTE_U==1` faults with `FAULT_SKAC` unless the `SUA` latch is also set.
- `MMU_CTRL_SUA` (bit 4) — supervisor-user-access latch, mutated only by `SUAEN` / `SUADIS` (ignored by `MTCR CR_MMU_CTRL`).
- `FAULT_SKEF` (9) / `FAULT_SKAC` (10) — new fault cause codes raised by the SKEF / SKAC checks.

Control register 14 holds the `SUA` snapshot taken on trap entry. Kernel assembly accesses it numerically as `cr14` (no mnemonic constant is exported from `iexec.inc`; the semantic name `CR_SAVED_SUA` is used by the CPU and monitor source but the assembly contract is numeric). The CPU's trap-frame stack preserves `cr14` (and `cr3` / `cr13`) across nested traps automatically, so kernel handlers do not need a manual MFCR/MTCR save/restore dance to survive a nested synchronous trap. See `sdk/docs/IE64_ISA.md` §12.14 for the full contract.

The `copy_from_user`, `copy_to_user`, and `copy_cstring_from_user` helpers in `sdk/intuitionos/iexec/iexec.s` wrap their user-memory accesses with `SUAEN` / `SUADIS` and are the only sanctioned way to touch a user pointer from supervisor code. See `sdk/docs/IE64_COOKBOOK.md` and `sdk/docs/IntuitionOS/IExec.md` for the worked calling convention.

The same header now exports the M15.6 shared-memory permission bits used by `SYS_MAP_SHARED`:

- `MAPF_READ` (bit 0) — install a readable user mapping.
- `MAPF_WRITE` (bit 1) — install a writable user mapping.
- `MEMF_GUARD` (bit 1 in the `AllocMem` flags word) — reserve one non-present
  page on each side of the mapped allocation. The returned VA still points at
  the mapped body, not the leading guard.
- `QUOTA_PAGES` / `QUOTA_PORTS` / `QUOTA_WAITERS` / `QUOTA_SHMEM` /
  `QUOTA_GRANTS` — quota-kind constants used by the M15.6 quota inspection and
  limit-setting paths. These name the five kernel-tracked resource classes:
  page allocations, public ports, blocked waiters, shared mappings, and
  grant rows.

`SYS_MAP_SHARED` requires a non-zero subset of those bits in `R2`; omitted masks and unknown bits fail with `ERR_BADARG`. The kernel never sets `PTE_X` for shared mappings.

`iexec.inc` also adds two `BOOT_HOSTFS_*` commands that back the writable `SYS:` overlay:

- `BOOT_HOSTFS_CREATE_WRITE` (6) — `arg1 = path ptr`. Opens (or creates+truncates) the file for writing; returns a host handle in `res1`. The host device rejects any path whose first component is `IOSSYS` (case-insensitive), enforcing the read-only IOSSYS namespace.
- `BOOT_HOSTFS_WRITE` (7) — `arg1 = handle, arg2 = src ptr, arg3 = byte_count`. Writes bytes to an open hostfs handle; returns the byte count actually written in `res1`.

As of **M16**, `iexec.inc` also exports the protected-module/library lifecycle ABI used by shipped IntuitionOS libraries:

- `SYS_OPEN_LIBRARY_EX` — registry-backed `OpenLibrary` for online/loading libraries with waiter completion
- `SYS_CLOSE_LIBRARY` — generation-checked `CloseLibrary` for opaque library-base tokens
- `SYS_ADD_LIBRARY` — trusted internal library registration path (`RegisterModule`/`AddLibrary` for class `library`)
- `SYS_SET_RESIDENT` — toggle `MODF_RESIDENT` on a library row by name (`RESIDENT` shell command uses this)
- `SYS_M16_EXPUNGE_RESULT` — trusted internal reply path for library accept/refuse of an exec-managed expunge
- `SIGF_MODDEAD` — signal bit delivered to opener tasks when an online library owner dies
- `MODF_RESIDENT` — runtime pin bit; close-to-zero keeps the row online at the resident floor
- `MODF_COMPAT_PORT` — manifest/runtime bit indicating the library publishes a FindPort-visible compat public port
- `LIB_OP_EXPUNGE` — control opcode queued by exec when a non-resident close-to-zero enters expunge

This M16 surface is intentionally narrow. `iexec.inc` documents the shipped ABI constants; the larger manifest/schema/state-machine discussion lives in `sdk/docs/IntuitionOS/IExec.md` and `sdk/docs/IntuitionOS/M16-plan.md`. M16.2 extends the same internal lifecycle model to handlers, devices, and resources with `MODCLASS_HANDLER`, `MODCLASS_DEVICE`, `MODCLASS_RESOURCE`, plus trusted-internal `SYS_ADD_HANDLER`, `SYS_ADD_DEVICE`, and `SYS_ADD_RESOURCE` aliases. M16.2.1 freezes public non-library acquisition as `exec.library` IPC, not new public syscalls: `EXEC_MSG_ATTACH_HANDLER` / `DETACH_HANDLER`, `EXEC_MSG_OPEN_DEVICE` / `CLOSE_DEVICE`, `EXEC_MSG_OPEN_RESOURCE` / `CLOSE_RESOURCE`, and `EXEC_REPLY_FLAG`. Acquire/open requests pass a one-page shared request object whose first 64 bytes contain version/reserved fields and a 32-byte NUL-terminated module name; replies use `opcode | EXEC_REPLY_FLAG`, token-or-zero in `data0`, low-32-bit `ERR_*` in `data1`, and no reply share handle. M16.2.1 is ONLINE-only for non-library rows, returns opaque generation-aware tokens, and leaves PIE enforcement to M16.3 and relocation/ASLR to M16.4.

As of M15.1, `sdk/intuitionos/iexec/iexec.s` remains the top-level kernel image/layout source, while `sdk/intuitionos/iexec/runtime_builder.s` assembles the standalone hostfs runtime artifacts. Command bodies live under `sdk/intuitionos/iexec/cmd/`, the non-DOS boot services now live under `sdk/intuitionos/iexec/handler/`, `sdk/intuitionos/iexec/dev/`, `sdk/intuitionos/iexec/resource/`, and `sdk/intuitionos/iexec/lib/`, the interactive shell itself lives in `sdk/intuitionos/iexec/handler/shell.s`, and the DOS-owned block lives under `sdk/intuitionos/iexec/lib/dos_library.s`. The remaining subordinate runtime files now include `sdk/intuitionos/iexec/assets/elfseg_fixture.s`, `sdk/intuitionos/iexec/cmd/gfxdemo.s`, and `sdk/intuitionos/iexec/cmd/about.s`. The last root boot/image wiring blocks now live in `sdk/intuitionos/iexec/boot/bootstrap.s` and `sdk/intuitionos/iexec/boot/strings.s`. This does not change the role of `iexec.inc`: it remains the shared contract include for both the kernel image source and the split IntuitionOS component files.

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
- `BLT_OP_COLOR_EXPAND` - 1-bit template to colored pixels (text rendering)

### Extended Blitter Registers
- `BLT_FLAGS` / `BLT_FG` / `BLT_BG` / `BLT_MASK_MOD` / `BLT_MASK_SRCX` - BPP mode, draw modes, color expansion

### BLT_FLAGS Bit Definitions
- `BLT_FLAGS_BPP_RGBA32` / `BLT_FLAGS_BPP_CLUT8` / `BLT_FLAGS_BPP_MASK` - Pixel format (bits 0-1)
- `BLT_FLAGS_DRAWMODE_SHIFT` / `BLT_FLAGS_DRAWMODE_MASK` - Raster draw mode (bits 4-7)
- `BLT_FLAGS_JAM1` - Color expand: skip BG pixels (bit 8)
- `BLT_FLAGS_INVERT_TMPL` - Invert template bits (bit 9)
- `BLT_FLAGS_INVERT_MODE` - XOR destination for set template bits (bit 10)

### Audio Registers
- PSG: `PSG_REG_*`, `PSG_PLAY_*`
- SID: `SID_*`, `SID2_BASE`, `SID3_BASE`, `SID_PLAY_*`; Z80/x86 also define `SID_CHIP_SID1`, `SID_CHIP_SID2`, `SID_CHIP_SID3`
- POKEY: `POKEY_*`, `SAP_PLAY_*`
- TED: `TED_*`
- AHX: `AHX_*`
- MOD: `MOD_PLAY_PTR`, `MOD_PLAY_LEN`, `MOD_PLAY_CTRL`, `MOD_PLAY_STATUS`, `MOD_FILTER_MODEL`, `MOD_POSITION`

### Memory Constants
- `VRAM_START` - Start of video RAM
- `SCREEN_W` / `SCREEN_H` - Display dimensions
- `LINE_BYTES` - Bytes per scanline

### Timer
- `TIMER_CTRL` / `TIMER_COUNT` / `TIMER_RELOAD`

### File I/O
- `FILE_NAME_PTR` / `FILE_DATA_PTR` / `FILE_DATA_LEN` - Pointers and length (32-bit CPUs)
- `FILE_CTRL` - Control register (write `FILE_OP_READ` or `FILE_OP_WRITE`)
- `FILE_STATUS` / `FILE_RESULT_LEN` / `FILE_ERROR_CODE` - Result registers
- 8-bit CPUs (Z80/6502) use byte-addressable variants via bank3 window: `FIO_NAME_PTR_0`..`FIO_NAME_PTR_3`, `FIO_DATA_PTR_0`..`FIO_DATA_PTR_3`, `FIO_CTRL`, etc.

### System Control
- `SYS_GC_TRIGGER` - Write any value to trigger garbage collection at a safe point

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

**Macros:** `SET_BANK1`..`SET_BANK3`, `SET_VRAM_BANK`, `STORE16`, `STORE32`, `STORE32_ZP`, `WAIT_VBLANK`, `WAIT_BLIT`, `START_BLIT`, `SET_BLT_OP/WIDTH/HEIGHT/COLOR`, `SET_SRC_STRIDE`, `SET_DST_STRIDE`, `ADD16`, `INC16`, `CMP16`, AHX player macros, File I/O (`SET_FILE_IO_BANK`, `SET_FIO_PTR`, `FILE_READ`, `FILE_WRITE`), coprocessor helpers.

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

**Macros:** `SET_BANK1`..`SET_BANK3`, `SET_VRAM_BANK`, `STORE16`, `STORE32`, `WAIT_VBLANK`, `WAIT_BLIT`, `START_BLIT`, `SET_BLT_*`, `SET_COPPER_PTR`, PSG/SID/SAP/AHX player macros, `SID_WRITE`, `ADD_HL_IMM`, `CP_HL_IMM`, `INC16`, File I/O (`SET_FILE_IO_BANK`, `SET_FIO_PTR`, `FILE_READ`, `FILE_WRITE`), coprocessor helpers.

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
- POKEY: ports `0x60`-`0x69`
- SID: ports `0xE0`-`0xE1`
- TED: ports `0xF2`-`0xF3`
- VGA: standard PC ports (`0x3C4`-`0x3DA`)

**Macros:** `wait_vblank`, `vga_wait_vsync`, `psg_write`, `sid_write`, `pokey_write`, coprocessor helpers.

## Stability Policy

The `sdk/include/` directory is the canonical location for all include files. Hardware register definitions (`ie*.inc`) and EhBASIC modules (`ehbasic_*.inc`) live here.
