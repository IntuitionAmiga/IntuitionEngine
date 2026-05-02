# Intuition Engine Architecture

*Last updated: 2026-03-01*

Intuition Engine is a multi-CPU retro hardware emulator with 6 heterogeneous CPU cores, 6 video chips, 6 audio engines, a copper coprocessor, DMA blitter, and extensive I/O peripherals — all connected through a unified MachineBus. Total guest RAM is autodetected at boot from host `/proc/meminfo` minus a per-platform reserve (see `memory_sizing.go`); each CPU/profile sees an active visible RAM clamped to its own ceiling. Guest software discovers sizes through the SYSINFO MMIO pairs (`SYSINFO_TOTAL_RAM_LO/HI`, `SYSINFO_ACTIVE_RAM_LO/HI`) and IE64 `CR_RAM_SIZE_BYTES`. This document describes the system architecture with diagrams showing all chips, buses, internal functional units, and data flow paths.

## Platform JIT Matrix

The host-side JIT support is intentionally asymmetric:

| Host platform | JIT-enabled guest cores |
|---------------|-------------------------|
| Linux amd64 | IE64, 6502, M68K, Z80, x86 |
| Linux arm64 | IE64 |
| Windows amd64 | IE64, 6502, M68K, Z80, x86 |
| Windows arm64 | IE64 |
| macOS amd64 | IE64, 6502, M68K, Z80, x86 |
| macOS arm64 | IE64 |

On macOS amd64, the JIT reuses the shared x86-64 host backends. On macOS arm64, executable memory uses the native `MAP_JIT` model with thread-pinned write protection toggles, and non-IE64 guest cores remain interpreter-only on arm64 hosts.

## 1. System Overview

```mermaid
graph TB
    subgraph CPUs["CPU Bank (concurrent via Coprocessor Manager)"]
        IE32["IE32<br/>16 x 32-bit GPR"]
        IE64["IE64<br/>32 x 64-bit GPR"]
        M68K["M68K 68020<br/>D0-D7 / A0-A7"]
        Z80["Z80<br/>Main + Shadow Regs"]
        C6502["6502<br/>A / X / Y"]
        X86["x86<br/>EAX-EDI"]
    end

    BUS["MachineBus (autodetected guest RAM)<br/>MMIO Dispatch via ioPageBitmap"]

    IE32 --> BUS
    IE64 --> BUS
    M68K --> BUS
    Z80 --> BUS
    C6502 --> BUS
    X86 --> BUS

    subgraph MEM["Memory"]
        RAM["Main RAM<br/>636KB"]
        STK["Stack<br/>4KB"]
        VGAW["VGA Windows<br/>128KB"]
        VRAM["Video RAM<br/>5MB"]
        FAST["Fast RAM<br/>22MB (AROS mode)"]
    end

    BUS --> RAM
    BUS --> STK
    BUS --> VGAW
    BUS --> VRAM
    BUS --> FAST

    subgraph VIDEO["Video Subsystem"]
        VC["VideoChip<br/>Layer 0"]
        VGA["VGA<br/>Layer 10"]
        TED_V["TED Video<br/>Layer 12"]
        ANTIC["ANTIC/GTIA<br/>Layer 13"]
        ULA["ULA<br/>Layer 15"]
        VOO["Voodoo 3D<br/>Layer 20"]
        COMP["Compositor<br/>Z-order blend"]
        DISP["Display<br/>Ebiten 60Hz"]
    end

    BUS --> VC
    BUS --> VGA
    BUS --> TED_V
    BUS --> ANTIC
    BUS --> ULA
    BUS --> VOO
    VC --> COMP
    VGA --> COMP
    TED_V --> COMP
    ANTIC --> COMP
    ULA --> COMP
    VOO --> COMP
    COMP --> DISP

    subgraph AUDIO["Audio Subsystem"]
        SC["SoundChip<br/>10 Channels"]
        PSG["PSG<br/>AY-3-8910"]
        SID["SID<br/>6581/8580 x3"]
        POK["POKEY"]
        TED_A["TED Audio"]
        AHX["AHX Replayer"]
        AOUT["Audio Backend<br/>OTO 44.1kHz"]
    end

    BUS --> SC
    BUS --> PSG
    BUS --> SID
    BUS --> POK
    BUS --> TED_A
    BUS --> AHX
    PSG --> SC
    SID --> SC
    POK --> SC
    TED_A --> SC
    AHX --> SC
    SC --> AOUT

    subgraph IO["I/O Peripherals"]
        TERM["Terminal/Serial"]
        FIO["File I/O"]
        PEXEC["Program Executor"]
        COPRO["Coprocessor Manager"]
        CLIP["Clipboard Bridge"]
        MEDIA["Media Loader"]
        LUA["Lua Scripting"]
        DOS["DOS Handler"]
    end

    BUS --> TERM
    BUS --> FIO
    BUS --> PEXEC
    BUS --> COPRO
    BUS --> CLIP
    BUS --> MEDIA
    BUS --> DOS
    LUA -.->|"bus access"| BUS

    classDef cpu fill:#4169E1,stroke:#333,color:#fff
    classDef video fill:#228B22,stroke:#333,color:#fff
    classDef audio fill:#FF8C00,stroke:#333,color:#fff
    classDef mem fill:#708090,stroke:#333,color:#fff
    classDef periph fill:#8B008B,stroke:#333,color:#fff
    classDef bus fill:#DC143C,stroke:#333,color:#fff

    class IE32,IE64,M68K,Z80,C6502,X86 cpu
    class VC,VGA,TED_V,ANTIC,ULA,VOO,COMP,DISP video
    class SC,PSG,SID,POK,TED_A,AHX,AOUT audio
    class RAM,STK,VGAW,VRAM,FAST mem
    class TERM,FIO,PEXEC,COPRO,CLIP,MEDIA,LUA,DOS periph
    class BUS bus
```

**Bus architecture notes:**

- **Concurrent multi-CPU bus** — the Program Executor selects the primary CPU mode, but the Coprocessor Manager can launch additional worker CPUs that run concurrently on the same bus. Lock-free bus design (immutable `MapIO` dispatch, `ioPageBitmap` fast path, I/O callbacks protect their own state) allows safe concurrent access without bus arbitration.
- **No centralised interrupt controller** — each CPU has per-CPU interrupt lines (IRQ/NMI as `atomic.Bool`). Peripherals signal the active CPU directly.
- **MMIO dispatch** — the bus uses an `ioPageBitmap []bool` fast path (page = 256 bytes). Non-I/O pages use direct unsafe pointer access with zero dispatch overhead.

## 2. CPU Subsystem

```mermaid
graph LR
    subgraph IE32S["IE32"]
        IE32_RF["Register File<br/>16 x 32-bit"]
        IE32_ALU["ALU"]
        IE32_TMR["Timer"]
        IE32_IRQ["Interrupt Ctrl"]
        IE32_VDP["VRAM Direct Path"]
    end

    subgraph IE64S["IE64"]
        IE64_RF["Register File<br/>32 x 64-bit (R0=zero)"]
        IE64_ALU["ALU"]
        IE64_FPU["FPU<br/>16 x float32"]
        IE64_JIT["JIT Compiler<br/>ARM64 + x86-64"]
        IE64_TMR["Timer"]
        IE64_IRQ["Interrupt Ctrl"]
    end

    subgraph M68KS["M68K 68020"]
        M68K_DR["Data Regs D0-D7"]
        M68K_AR["Address Regs A0-A7"]
        M68K_SR["Status Register<br/>CCR + SR"]
        M68K_VBR["VBR"]
        M68K_FPU["FPU<br/>68881/68882"]
        M68K_EVT["Exception Vector Table"]
        M68K_LE["LE/BE Bus Adapter"]
    end

    subgraph Z80S["Z80"]
        Z80_MR["Main Regs"]
        Z80_SR["Shadow Regs"]
        Z80_IX["Index Regs IX/IY"]
        Z80_IM["Interrupt Modes<br/>IM0/1/2"]
        Z80_PIO["Port I/O Bridge"]
        Z80_AW["16-to-32-bit<br/>Address Window"]
    end

    subgraph C6502S["6502"]
        C6502_R["A / X / Y / SP / PC"]
        C6502_F["Status NVBDIZC"]
        C6502_VEC["Vectors<br/>NMI/RESET/IRQ"]
        C6502_AW["16-to-32-bit<br/>Address Window"]
    end

    subgraph X86S["x86"]
        X86_GP["General Purpose<br/>EAX-EDI"]
        X86_SEG["Segment Regs"]
        X86_FL["EFLAGS + EIP"]
        X86_FPU["x87 FPU<br/>8 x float64"]
        X86_PIO["Port I/O Bridge"]
    end

    EMUIF["EmulatorCPU Interface<br/>LoadProgram / Reset / Execute / Stop / StartExecution"]
    BUSBAR["MachineBus"]

    IE32S --> EMUIF
    IE64S --> EMUIF
    M68KS --> EMUIF
    Z80S --> EMUIF
    C6502S --> EMUIF
    X86S --> EMUIF
    EMUIF --> BUSBAR

    classDef cpu fill:#4169E1,stroke:#333,color:#fff
    classDef iface fill:#DC143C,stroke:#333,color:#fff
    class IE32_RF,IE32_ALU,IE32_TMR,IE32_IRQ,IE32_VDP cpu
    class IE64_RF,IE64_ALU,IE64_FPU,IE64_JIT,IE64_TMR,IE64_IRQ cpu
    class M68K_DR,M68K_AR,M68K_SR,M68K_VBR,M68K_FPU,M68K_EVT,M68K_LE cpu
    class Z80_MR,Z80_SR,Z80_IX,Z80_IM,Z80_PIO,Z80_AW cpu
    class C6502_R,C6502_F,C6502_VEC,C6502_AW cpu
    class X86_GP,X86_SEG,X86_FL,X86_FPU,X86_PIO cpu
    class EMUIF,BUSBAR iface
```

### CPU Timers (IE32 / IE64)

Both IE32 and IE64 use CPU-internal countdown timers backed by atomic fields on the CPU structs.

- The timer decrements once per `SAMPLE_RATE` instructions.
- On expiry, it sets timer state to expired and can trigger a CPU interrupt.
- If still enabled after interrupt handling, the count auto-reloads from the configured period.

There is no stable bus/MMIO timer control ABI at present. Legacy include symbols such as `TIMER_CTRL/TIMER_COUNT/TIMER_RELOAD` are retained for compatibility but are deprecated.

### CPU Selection by File Extension

| Extension | CPU | Address Space | Notes |
|-----------|-----|---------------|-------|
| `.iex` / `.ie32` | IE32 | 32-bit flat (clamped to active visible RAM) | Native RISC, 8-byte fixed instructions |
| `.ie64` | IE64 | 64-bit (sees full active visible RAM) | 64-bit RISC, R0=zero, JIT on ARM64 + x86-64 |
| `.ie68` | M68K | 32-bit flat (clamped to M68K profile bound) | 68020, big-endian with LE bus adapter |
| `.ie65` | 6502 | 16-bit + bank windows | Bank windows reach the banked-CPU visible ceiling |
| `.ie80` | Z80 | 16-bit + bank windows + port I/O bridge | Bank windows reach the banked-CPU visible ceiling |
| `.ie86` | x86 | 32-bit flat (clamped to active visible RAM) | Flat model, port I/O bridge |
| `.tos` / `.img` | M68K | EmuTOS profile (`EmuTOS_PROFILE_TOP`) | EmuTOS boot with GEMDOS intercept |
| `.ies` | Script | N/A | Lua scripting engine (IE Script) |

AROS boot is selected by CLI mode (`-aros`, optional `-aros-image`), not file extension.

### Z80 Port I/O Bridge

Z80 `IN`/`OUT` instructions are translated to bus MMIO accesses by the `Z80BusAdapter`:

| Z80 Port | Chip | Bus Target | Protocol |
|----------|------|------------|----------|
| `$A0-$AA` | VGA | `0xF1000` | Direct register map (MODE, STATUS, CTRL, SEQ, CRTC, GC, DAC) |
| `$B0-$B7` | Voodoo 3D | `0xF8000` | Addr lo/hi + 4 data bytes (write to `$B5` triggers 32-bit bus write) |
| `$D0/$D1` | POKEY | `0xF0D00` | Register select / data |
| `$D4/$D5` | ANTIC | `0xF2100` | Register select / data (x4 stride) |
| `$D6/$D7` | GTIA | `0xF2140` | Register select / data (x4 stride, collision regs through `0xF21F8`) |
| `$E0/$E1` | SID | `0xF0E00/0xF0E30/0xF0E50` | Register select / data; select bits 5-6 choose SID1/SID2/SID3 |
| `$F0/$F1` | PSG | `0xF0C00` | Register select / data |
| `$F2/$F3` | TED | `0xF0F00` / `0xF0F20` | Register select / data (audio / video x4 stride) |
| `$FE` | ULA | `0xF2000` | Border colour (bits 0-2). Bits 3-4 currently ignored. |

### Z80 16-bit Memory Translation

Z80 memory accesses in `0xF000-0xFFFF` are translated to bus `0xF0000-0xF0FFF`. ANTIC/GTIA access is provided through the Z80 port bridge (`$D4-$D7`) rather than general memory-mapped addressing.

### x86 Port I/O Bridge

x86 shares the Z80 Voodoo port mapping (`$B0-$B7`) and most sound-chip port mappings, but **POKEY uses direct port-to-register mapping** (not the Z80's select/data protocol). It also adds standard PC VGA ports:

| x86 Port | Chip | Bus Target | Protocol |
|----------|------|------------|----------|
| `$3C4` | VGA Sequencer index | `VGA_SEQ_INDEX` | Standard PC port |
| `$3C5` | VGA Sequencer data | `VGA_SEQ_DATA` | Standard PC port |
| `$3C6-$3C9` | VGA DAC | mask / read idx / write idx / data | R/G/B cycled in sequence |
| `$3CE` | VGA GC index | `VGA_GC_INDEX` | Standard PC port |
| `$3CF` | VGA GC data | `VGA_GC_DATA` | Standard PC port |
| `$3D4` | VGA CRTC index | `VGA_CRTC_INDEX` | Standard PC port |
| `$3D5` | VGA CRTC data | `VGA_CRTC_DATA` | Standard PC port |
| `$3DA` | VGA Status | Returns `0x08` if vsync | Standard PC port |
| `$B0-$B7` | Voodoo 3D | `0xF8000` | Addr lo/hi + 4 data bytes |
| `$60-$69` | POKEY | `0xF0D00+(port-0x60)` | **Direct** — port offset maps to writable register |
| `$D4/$D5` | ANTIC | `0xF2100` | Register select / data (x4 stride) |
| `$D6/$D7` | GTIA | `0xF2140` | Register select / data (x4 stride, collision regs through `0xF21F8`) |
| `$E0/$E1` | SID | `0xF0E00/0xF0E30/0xF0E50` | Register select / data; select bits 5-6 choose SID1/SID2/SID3 |
| `$F0/$F1` | PSG | `0xF0C00` | Register select / data |
| `$F2/$F3` | TED | `0xF0F00` / `0xF0F20` | Register select / data (audio / video x4 stride) |
| `$FE` | ULA | `0xF2000` | Border colour (bits 0-2) |

**Key difference from Z80**: x86 POKEY access is direct — ports `$60-$69` map one-to-one onto writable POKEY registers at `0xF0D00+(port-0x60)`. ANTIC (`$D4/$D5`) and GTIA (`$D6/$D7`) keep their own select/data pairs.

x86 also directly accesses VGA VRAM at `$A0000-$AFFFF` in the memory path (no port translation needed).

### Bank Windows (Z80 / 6502 / x86)

All three 8/16-bit CPUs share identical bank window architecture for accessing the active visible RAM from a 16-bit address space; bank translation rejects addresses above the banked-CPU visible ceiling:

| CPU Address | Size | Purpose | Bank Select Register |
|-------------|------|---------|---------------------|
| `$2000-$3FFF` | 8KB | Bank 1 (sprite data) | `$F700/$F701` (lo/hi) |
| `$4000-$5FFF` | 8KB | Bank 2 (font data) | `$F702/$F703` (lo/hi) |
| `$6000-$7FFF` | 8KB | Bank 3 (general) | `$F704/$F705` (lo/hi) |
| `$8000-$BFFF` | 16KB | VRAM window | `$F7F0` (bank number) |
| `$F000-$FFF9` | 4KB | I/O window | Hardwired: `$Fxxx` -> bus `$F0xxx` |
| `$F200-$F23F` | 64B | Coprocessor gateway | Hardwired: -> bus `$F2340+offset` |
| `$FFFA-$FFFF` | 6B | 6502 vectors (NMI/RESET/IRQ) | Identity mapped |

### 6502 I/O Chip Page Dispatch

The 6502 uses `ioTable[page]` to route memory-mapped I/O through the bus:

| 6502 Address | Chip | Bus Target | Notes |
|--------------|------|------------|-------|
| `$D200-$D209` | POKEY | `0xF0D00+offset` | |
| `$D400-$D40F` | PSG | `0xF0C00+offset` | |
| `$D500-$D51F` | SID1 | `0xF0E00+offset` | 32-byte window |
| `$D520-$D53F` | SID2 | `0xF0E30+offset` | 32-byte window |
| `$D540-$D55F` | SID3 | `0xF0E50+offset` | 32-byte window |
| `$D600-$D605` | TED Audio | `0xF0F00+offset` | |
| `$D620-$D62F` | TED Video | `0xF0F20+offset x4` | Stride-4 register mapping |
| `$D700-$D70A` | VGA | `0xF1000` | Direct handler call |
| `$D800-$D80F` | ULA | `0xF2000+offset` | |

ANTIC/GTIA intentionally has no 6502 `$D400/$D000` compatibility surface; `$D400-$D40F` is PSG on the 6502 map.

## 3. Video Subsystem

```mermaid
graph TB
    subgraph VCS["VideoChip (Layer 0, 0xF0000-0xF0487)"]
        VC_FB["Framebuffer Manager<br/>640x480 / 800x600 / 1024x768 / 1280x960"]
        VC_COP["Copper Coprocessor<br/>WAIT / MOVE / SETBASE / END"]
        VC_BLT["DMA Blitter<br/>Copy / Fill / Line / Masked / Alpha"]
        VC_M7["Mode7 Affine Texture Unit<br/>16.16 fixed-point UV<br/>per-col + per-row deltas"]
        VC_RST["Raster Effects Unit"]
        VC_PAL["Palette RAM<br/>256-entry CLUT8"]
    end

    subgraph VGAS["VGA (Layer 10, 0xF1000-0xF13FF)"]
        VGA_SEQ["Sequencer<br/>Plane control"]
        VGA_CRTC["CRTC<br/>Timing / page flip"]
        VGA_GC["Graphics Controller<br/>Bit operations"]
        VGA_AC["Attribute Controller<br/>Palette mapping"]
        VGA_DAC["DAC<br/>256-colour palette"]
        VGA_VRAM["VRAM Windows<br/>0xA0000 graphics / 0xB8000 text"]
    end

    subgraph TEDS["TED Video (Layer 12, 0xF0F20-0xF0F5F)"]
        TED_TXT["Text Display<br/>40x25"]
        TED_CHR["Character Set"]
        TED_COL["121 Colours<br/>16 hue x 8 luminance"]
        TED_CUR["Cursor Engine<br/>Blink"]
    end

    subgraph ANTICS["ANTIC/GTIA (Layer 13, 0xF2100-0xF21FB)"]
        ANT_DLP["ANTIC Display List<br/>Processor + DMA"]
        ANT_SCR["Fine Scroll H/V<br/>+ WSYNC"]
        GTIA_C["GTIA Colours<br/>COLPF0-3 / COLBK"]
        GTIA_PM["Player/Missile<br/>Graphics"]
        GTIA_PR["Priority + Collisions"]
        ANT_COL["128 Colours"]
    end

    subgraph ULAS["ULA (Layer 15, 0xF2000-0xF200B)"]
        ULA_BMP["Bitmap Unit<br/>256x192"]
        ULA_ATR["Attribute Unit<br/>32x24 cells"]
        ULA_BDR["Border Colour"]
        ULA_COL["16 Colours<br/>8 normal + 8 bright<br/>(15 visually distinct)"]
    end

    subgraph VOOS["Voodoo 3D (Layer 20, 0xF8000-0xF87FF)"]
        VOO_VTX["Vertex Assembly<br/>12.4 X/Y, 12.12 colour, 20.12 Z<br/>14.18 S/T, 2.30 W"]
        VOO_TRI["Triangle Rasteriser<br/>Gouraud shading"]
        VOO_TEX["Texture Unit<br/>Perspective-correct S/T/W<br/>64KB texture memory (0xD0000)"]
        VOO_ZB["Z-Buffer<br/>Configurable depth test"]
        VOO_AB["Alpha Blender"]
        VOO_FOG["Fog Unit<br/>64-entry fog table (0xF8140-0xF823F)"]
        VOO_CK["Chroma Key"]
        VOO_BE["Vulkan / Software Backend"]
    end

    COMPS["Compositor<br/>Z-order: 0 - 10 - 12 - 13 - 15 - 20<br/>60Hz output"]
    DISPLAY["Display<br/>Ebiten VideoOutput"]

    VCS -->|"GetFrame()"| COMPS
    VGAS -->|"GetFrame()"| COMPS
    TEDS -->|"GetFrame()"| COMPS
    ANTICS -->|"GetFrame()"| COMPS
    ULAS -->|"GetFrame()"| COMPS
    VOOS -->|"GetFrame()"| COMPS
    COMPS --> DISPLAY

    VC_COP -.->|"SETBASE + bus.Write32()"| VGAS
    VC_COP -.->|"cross-chip register writes"| ULAS

    classDef video fill:#228B22,stroke:#333,color:#fff
    classDef comp fill:#2E8B57,stroke:#333,color:#fff
    class VC_FB,VC_COP,VC_BLT,VC_M7,VC_RST,VC_PAL video
    class VGA_SEQ,VGA_CRTC,VGA_GC,VGA_AC,VGA_DAC,VGA_VRAM video
    class TED_TXT,TED_CHR,TED_COL,TED_CUR video
    class ANT_DLP,ANT_SCR,GTIA_C,GTIA_PM,GTIA_PR,ANT_COL video
    class ULA_BMP,ULA_ATR,ULA_BDR,ULA_COL video
    class VOO_VTX,VOO_TRI,VOO_TEX,VOO_ZB,VOO_AB,VOO_FOG,VOO_CK,VOO_BE video
    class COMPS,DISPLAY comp
```

### Copper Cross-Chip Bus Access

The copper coprocessor is internal to VideoChip but can write to any MMIO-mapped chip on the bus:

- **Default**: MOVE targets VideoChip's own registers (`copperIOBase = VIDEO_REG_BASE`)
- **SETBASE opcode** changes `copperIOBase` to any MMIO address (e.g. `VGA_BASE 0xF1000`)
- Subsequent MOVE instructions compute `regAddr = copperIOBase + (regIndex * 4)` and route via `bus.Write32()` to VGA, ULA, or any other chip's MMIO
- This enables per-scanline palette changes, mode switches, and register manipulation on any video chip
- `copperIOBase` resets to `VIDEO_REG_BASE` at the start of each frame
- The compositor's `ScanlineAware` interface orchestrates this: `StartFrame()` -> `ProcessScanline(y)` -> `FinishFrame()`

### Extended Blitter: BPP Modes, Draw Modes, and Color Expansion

The blitter supports two pixel formats via `BLT_FLAGS` (`0xF0488`): RGBA32 (4 bpp, default) and CLUT8 (1 bpp). Bits 4-7 select one of 16 raster draw modes (Clear, And, Copy, Xor, Invert, etc.) applied per pixel during FILL and COPY operations. When `BLT_FLAGS=0`, the blitter defaults to Copy mode with RGBA32 for full backward compatibility.

The color expansion operation (`BLT_OP=6`) renders 1-bit glyph templates into colored pixels for hardware-accelerated text. It reads a template from `BLT_MASK`, uses `BLT_FG`/`BLT_BG` (`0xF048C`/`0xF0490`) as foreground/background colors, and supports three modes: JAM2 (opaque — set bits write FG, clear bits write BG), JAM1 (transparent — only set bits write FG), and Invert (set bits XOR the destination). `BLT_MASK_MOD` (`0xF0494`) sets the template row stride and `BLT_MASK_SRCX` (`0xF0498`) provides sub-byte bit alignment for glyph fragments. Template bits are MSB-first (Amiga convention).

Line drawing (`BLT_OP=2`) supports an extended mode when `BLT_FLAGS != 0`: `BLT_DST` becomes the framebuffer base address, `BLT_WIDTH` holds the packed endpoint coordinates `(y1<<16)|x1`, and `BLT_DST_STRIDE` sets the row stride. This allows line drawing into arbitrary bitmaps (not just the active framebuffer) with BPP awareness and all 16 draw modes. When `BLT_FLAGS=0`, legacy behavior is preserved (endpoint in `BLT_DST`, base at `VRAM_START`). In extended mode the blitter does not clip — callers must provide pre-clipped coordinates (the AROS driver uses Cohen-Sutherland clipping before calling the blitter).

| Register | Address | Description |
|----------|---------|-------------|
| `BLT_FLAGS` | `0xF0488` | BPP (bits 0-1), draw mode (bits 4-7), JAM1/invert flags (bits 8-10) |
| `BLT_FG` | `0xF048C` | Foreground color for color expansion |
| `BLT_BG` | `0xF0490` | Background color for color expansion |
| `BLT_MASK_MOD` | `0xF0494` | Template row modulo (bytes per row) |
| `BLT_MASK_SRCX` | `0xF0498` | Starting X bit offset in template |

### Mode7 Affine Texture Unit

The blitter's Mode7 operation (`bltOpMode7`) implements SNES-style affine texture mapping as a DMA blitter mode within VideoChip. It uses 8 dedicated MMIO registers (`0xF0058-0xF0074`):

| Register | Address | Description |
|----------|---------|-------------|
| `BLT_MODE7_U0` | `0xF0058` | Starting U coordinate (16.16 fixed-point) |
| `BLT_MODE7_V0` | `0xF005C` | Starting V coordinate (16.16 fixed-point) |
| `BLT_MODE7_DU_COL` | `0xF0060` | U increment per column (16.16) |
| `BLT_MODE7_DV_COL` | `0xF0064` | V increment per column (16.16) |
| `BLT_MODE7_DU_ROW` | `0xF0068` | U increment per row (16.16) |
| `BLT_MODE7_DV_ROW` | `0xF006C` | V increment per row (16.16) |
| `BLT_MODE7_TEX_W` | `0xF0070` | Texture width mask (must be power-of-2 minus 1) |
| `BLT_MODE7_TEX_H` | `0xF0074` | Texture height mask (must be power-of-2 minus 1) |

The rasteriser walks each destination pixel, computes the source UV from the affine matrix (origin + column delta + row delta), wraps via power-of-2 bitmask, and samples the source texture. This enables rotation, scaling, and perspective-like effects on tiled backgrounds — the same technique used by the SNES PPU2 for its Mode 7 background layer.

### Video Compositor

The compositor collects frames from all enabled video sources via `GetFrame()` (lock-free atomic swap) and blends them in Z-order (layer 0 at the back, layer 20 at the front).

Two rendering paths:

1. **Scanline-aware path** — used when all sources implement `ScanlineAware`. The compositor sorts sources by layer ascending and processes them in that order for each scanline. This guarantees that VideoChip (layer 0) processes its copper display list first — which may write to VGA registers via `bus.Write32()` — before VGA (layer 10) renders that same scanline using the updated state. This ordering is what makes per-scanline palette changes and cross-chip effects work correctly.
2. **Full-frame fallback** — collects complete frames and blends them. Frame blending uses parallel goroutines with 60-line strips via `sync.WaitGroup`.

### Triple-Buffer Protocol

All video chips except VideoChip use a lock-free triple-buffer protocol for `GetFrame()`:

```text
Slots:  writeIdx = 0 (producer-owned)
        sharedIdx = 1 (atomic, in-transit)
        readingIdx = 2 (consumer-owned)

Producer (render goroutine, after rendering into frameBufs[writeIdx]):
    writeIdx = sharedIdx.Swap(writeIdx)

Consumer (compositor, GetFrame):
    readIdx = sharedIdx.Swap(readIdx)
    return frameBufs[readIdx]
```

**Important**: `GetFrame()` performs an atomic Swap — calling it twice in a row swaps back to the previous buffer. Always call once and save the result.

On resolution change, all 3 buffer slots are reallocated and indices reset to `writeIdx=0`, `sharedIdx=1`, `readingIdx=2`.

## 4. Audio Subsystem

### Synthesis Pipeline

```mermaid
graph LR
    subgraph SCS["SoundChip (0xF0800-0xF0B7F)"]
        direction TB
        CH["10 Channels<br/>Ch0-3 base + Ch4-6 SID2 + Ch7-9 SID3"]
        OSC["Oscillators<br/>Square / Triangle / Sine / Noise / Sawtooth"]
        ENV["Per-Channel ADSR<br/>Envelope"]
        SWP["Frequency Sweep"]
        SYNC["Hard Sync +<br/>Ring Modulation"]
        DAC_IN["DAC Input<br/>Per-channel bypass"]
        MIX["Channel Mixer<br/>10 to 1, dynamic averaging"]
        FLT["Resonant Filter<br/>LP / BP / HP"]
        OVR["Overdrive"]
        REV["Reverb"]
        LIM["Output Limiter"]
    end

    CH --> OSC --> ENV --> MIX
    SWP --> OSC
    SYNC --> OSC
    DAC_IN --> MIX
    MIX --> FLT --> OVR --> REV --> LIM

    OTO["OTO Backend<br/>44.1kHz float32"]
    SPK["Speakers"]

    LIM --> OTO --> SPK

    classDef audio fill:#FF8C00,stroke:#333,color:#fff
    classDef out fill:#B8860B,stroke:#333,color:#fff
    class CH,OSC,ENV,SWP,SYNC,DAC_IN,MIX,FLT,OVR,REV,LIM audio
    class OTO,SPK out
```

### SoundChip Dual-Interface Architecture

The SoundChip exposes two register interfaces for its channels:

**Legacy per-waveform interface** (`0xF0900-0xF09FF`) — 5 dedicated register blocks, each hardwired to one waveform type:

| Range | Channel | Default Waveform |
|-------|---------|-----------------|
| `0xF0900-0xF093F` | Ch 0 | Square |
| `0xF0940-0xF097F` | Ch 1 | Triangle |
| `0xF0980-0xF09BF` | Ch 2 | Sine |
| `0xF09C0-0xF09FF` | Ch 3 | Noise |
| `0xF0A00-0xF0A6F` | — | Sawtooth + modulation/effects |

**FLEX unified interface** (`0xF0A80-0xF0B7F`) — 4 channels with identical 64-byte register blocks. Each channel can be any waveform type:

| Offset | Register | Description |
|--------|----------|-------------|
| `+0x00` | `FREQ` | 16.8 fixed-point Hz (value / 256.0) |
| `+0x04` | `VOL` | Volume (0-255) |
| `+0x08` | `CTRL` | Enable (bit 0), gate (bit 1) |
| `+0x0C` | `DUTY` | Pulse width duty cycle |
| `+0x10` | `SWEEP` | Frequency sweep rate |
| `+0x14-0x20` | `ATK/DEC/SUS/REL` | ADSR envelope |
| `+0x24` | `WAVE_TYPE` | Waveform selection |
| `+0x28` | `PWM_CTRL` | Pulse width modulation |
| `+0x2C` | `NOISEMODE` | Noise mode (0=white, 1=periodic, 2=metallic, 3=psg) |
| `+0x30` | `PHASE` | Phase reset |
| `+0x34` | `RINGMOD` | Ring modulation (bit 7=enable, 0-2=source) |
| `+0x38` | `SYNC` | Hard sync (bit 7=enable, 0-2=source) |
| `+0x3C` | `DAC` | DAC mode bypass (signed 8-bit sample) |

FLEX channels are at `FLEX_CH0_BASE = 0xF0A80`, stride = `0x40`. Both interfaces write to the same underlying 10-channel mixer. The FLEX interface is preferred for new code — the legacy interface exists for backward compatibility.

### Filter and Modulation

The SoundChip's global resonant filter (`0xF0A00-0xF0A30`) supports low-pass, band-pass, and high-pass modes with cutoff frequency, resonance, and optional filter modulation source/amount registers. The filter model can be switched between classic and SVF via the `FILTER_MODEL` register.

### Engine and Player Routing

```mermaid
graph LR
    subgraph ENGINES["Sound Engines"]
        PSG_E["PSG / AY-3-8910<br/>0xF0C00-0xF0C0F<br/>3 Tone + Noise + Envelope"]
        SID_E["SID 6581/8580 x3<br/>0xF0E00-0xF0E6C<br/>3 Voices x 3 Chips = 9 Voices"]
        POK_E["POKEY<br/>0xF0D00-0xF0D0A<br/>4 Channels + Poly Counters"]
        TED_E["TED Audio<br/>0xF0F00-0xF0F05<br/>2 Voices (square + noise)"]
        AHX_E["AHX Replayer<br/>0xF0B80-0xF0B94<br/>4 Channels"]
    end

    subgraph PLAYERS["Music Players (PTR/LEN/CTRL/STATUS/POSITION protocol)"]
        MOD_P["MOD Player<br/>0xF0BC0<br/>+ FILTER_MODEL"]
        WAV_P["WAV Player<br/>0xF0BD8<br/>+ POSITION"]
        AY_P["AY Player<br/>0xF0C10<br/>VGM/VGZ auto-convert"]
        SID_P["SID Player<br/>0xF0E20"]
        SAP_P["SAP Player<br/>0xF0D10"]
        TED_P["TED Player<br/>0xF0F10"]
        AHX_P["AHX Player<br/>0xF0B84"]
    end

    subgraph PAULA["Paula DMA (0xF2260-0xF22AF)"]
        PDMA["4 DMA Audio Channels<br/>Amiga-compatible"]
    end

    SC["SoundChip<br/>10 Channels"]

    PSG_E -->|"Ch 0-2"| SC
    SID_E -->|"SID1:Ch0-2 SID2:Ch4-6 SID3:Ch7-9"| SC
    POK_E -->|"Ch 0-3"| SC
    TED_E -->|"Ch 0-1"| SC
    AHX_E -->|"Ch 0-3"| SC
    PDMA -->|"DAC bypass"| SC

    MOD_P --> SC
    WAV_P --> SC
    AY_P --> PSG_E
    SID_P --> SID_E
    SAP_P --> POK_E
    TED_P --> TED_E
    AHX_P --> AHX_E

    classDef audio fill:#FF8C00,stroke:#333,color:#fff
    classDef player fill:#CD853F,stroke:#333,color:#fff
    classDef chip fill:#B8860B,stroke:#333,color:#fff
    class PSG_E,SID_E,POK_E,TED_E,AHX_E audio
    class MOD_P,WAV_P,AY_P,SID_P,SAP_P,TED_P,AHX_P player
    class SC,PDMA chip
```

### Audio Engine Plus Enhanced Mode

All five retro sound engines have a "Plus" enhanced mode, activated by writing `1` to their respective `PLUS_CTRL` register:

| Engine | PLUS_CTRL Address | Enhancements |
|--------|------------------|--------------|
| PSG+ | `0xF0C20` | Enhanced render path: oversampling, filtering, drive/room shaping, stereo voicing |

The PSG uses the AY/YM logarithmic 16-step volume curve by default. A legacy linear curve is retained only for compatibility audits.
| SID+ | `0xF0E19` | Enhanced render path for SID voices (oversampling/filter/drive/room shaping) |
| POKEY+ | `0xF0D09` | Enhanced render path (oversampling/filter/drive/room shaping) |
| TED+ | `0xF0F05` | Enhanced render path plus TED-specific response shaping |
| AHX+ | `0xF0B80` | AHX voice-state mapping with stereo spread/panning and room processing |

When Plus mode is enabled, the engine retains full backward compatibility with the standard register set while exposing additional capabilities. AHX maps tracker state to native SoundChip channels instead of producing a Paula DMA stream; AHX+ uses a 64-sample crossfade when enabling/disabling to prevent audio glitches.

### Subsong Selection

SID, SAP, and AHX players support subsong selection for multi-tune files. Each player has a subsong register that selects which tune to play from a multi-song file.

SID PSID playback captures CIA1 timer-A latch writes at `$DC04/$DC05`; when non-zero, the player uses that latch as `cyclesPerTick`, so multispeed tunes run at `clockHz / latch`. SID MMIO opts into wide-write fanout: 16-bit and 32-bit writes are decomposed into little-endian byte register writes.

## 5. Memory Map

| Range | Size | Device |
|-------|------|--------|
| `0x00000-0x9EFFF` | 636KB | Main RAM |
| `0x9F000-0x9FFFF` | 4KB | Stack |
| `0xA0000-0xAFFFF` | 64KB | VGA VRAM Window |
| `0xB8000-0xBFFFF` | 32KB | VGA Text Buffer |
| `0xF0000-0xF0487` | 1160B | VideoChip + Copper + Blitter + Palette |
| `0xF0700-0xF07FF` | 256B | Terminal / Serial / Mouse / Keyboard / RTC_EPOCH (0xF0750) |
| `0xF0800-0xF0B7F` | 896B | SoundChip (10 channels, incl. FLEX) |
| `0xF0B80-0xF0B91` | 18B | AHX Engine / Player |
| `0xF0BC0-0xF0BD7` | 24B | MOD Player |
| `0xF0BD8-0xF0BEB` | 20B | WAV Player |
| `0xF0C00-0xF0C0F` | 16B | PSG Engine (AY-3-8910/YM2149 registers) |
| `0xF0C20` | 1B | PSG+ control |
| `0xF0C30-0xF0C3F` | 16B | Native SN76489 latch/data, ready, and LFSR mode registers |
| `0xF0C40-0xF0C4F` | 16B | Reserved for future SN76489 extensions |
| `0xF0C10-0xF0C1F` | 16B | PSG / AY Player |
| `0xF0D00-0xF0D0A` | 11B | POKEY Engine |
| `0xF0D10-0xF0D20` | 17B | SAP Player |
| `0xF0E00-0xF0E19` | 26B | SID1 Engine (6581/8580) |
| `0xF0E20-0xF0E2D` | 14B | SID Player |
| `0xF0E30-0xF0E6C` | 61B | SID2 + SID3 (Multi-SID) |
| `0xF0F00-0xF0F05` | 6B | TED Audio Engine |
| `0xF0F10-0xF0F1C` | 13B | TED Player |
| `0xF0F20-0xF0F5F` | 64B | TED Video |
| `0xF1000-0xF13FF` | 1KB | VGA Registers |
| `0xF2000-0xF200B` | 12B | ULA (ZX Spectrum) |
| `0xF2100-0xF213F` | 64B | ANTIC |
| `0xF2140-0xF21FB` | 188B | GTIA |
| `0xF2200-0xF221F` | 32B | File I/O |
| `0xF2220-0xF225F` | 64B | AROS DOS Handler |
| `0xF2260-0xF22AF` | 80B | AROS Audio DMA (Paula) |
| `0xF2300-0xF231F` | 32B | Media Loader |
| `0xF2320-0xF233F` | 32B | Program Executor |
| `0xF2340-0xF238F` | 80B | Coprocessor Manager |
| `0xF2390-0xF23AF` | 32B | Clipboard Bridge |
| `0xF23B0-0xF23BF` | 16B | Coprocessor Extended (monitor registers) |
| `0xF23E0-0xF23FF` | 32B | Bootstrap HostFS |
| `0xF8000-0xF87FF` | 2KB | Voodoo 3D Registers + palette |
| `0xF8140-0xF823F` | 256B | Voodoo Fog Table (64 entries × 4B) |
| `0xD0000-0xDFFFF` | 64KB | Voodoo Texture Memory |
| `0x100000-0x5FFFFF` | 5MB | Video RAM |
| `0x800000-0x1DFFFFF` | 22MB | AROS Fast Memory |
| `0x1E00000-0x1FFFFFF` | 2MB | AROS Video RAM |

Additional special regions used by the coprocessor subsystem:

| Range | Size | Purpose |
|-------|------|---------|
| `0x200000-0x27FFFF` | 512KB | Coprocessor: IE32 worker memory |
| `0x280000-0x2FFFFF` | 512KB | Coprocessor: M68K worker memory |
| `0x300000-0x30FFFF` | 64KB | Coprocessor: 6502 worker memory |
| `0x310000-0x31FFFF` | 64KB | Coprocessor: Z80 worker memory |
| `0x320000-0x39FFFF` | 512KB | Coprocessor: x86 worker memory |
| `0x3A0000-0x41FFFF` | 512KB | Coprocessor: IE64 worker memory |
| `0x800000` | 64KB | Media loader staging buffer |
| `0x790000-0x7917FF` | 6KB | Coprocessor mailbox ring buffers |

## 6. I/O Peripherals

```mermaid
graph LR
    CPU["Active CPU"]
    BUS["MachineBus"]
    CPU --> BUS

    subgraph DEVS["I/O Devices"]
        TERM["Terminal/Serial<br/>0xF0700-0xF07FF<br/>Input ring buffer, echo,<br/>raw keys, mouse, scancodes"]
        FIO["File I/O<br/>0xF2200-0xF221F<br/>Name/data ptrs, R/W ops,<br/>sandboxed"]
        PEXEC["Program Executor<br/>0xF2320-0xF233F<br/>CPU mode detect,<br/>full reset orchestration<br/>EXEC_OP: 1=Execute, 2=EmuTOS, 3=AROS"]
        COPRO["Coprocessor Manager<br/>0xF2340-0xF238F + 0xF23B0-0xF23BF<br/>6 worker CPU types,<br/>ticket-based dispatch + monitor"]
        CLIP["Clipboard Bridge<br/>0xF2390-0xF23AF<br/>Data ptr/len, get/put"]
        DOS["DOS Handler<br/>0xF2220-0xF225F<br/>AmigaDOS packet protocol,<br/>lock/file handles,<br/>ACTION_SAME_LOCK,<br/>DupLock of root (key=0)"]
        MEDIA["Media Loader<br/>0xF2300-0xF231F<br/>Format detection,<br/>player dispatch"]
        LUA["Lua Scripting<br/>F8 REPL, bus access,<br/>video recording"]
    end

    BUS --> TERM
    BUS --> FIO
    BUS --> PEXEC
    BUS --> COPRO
    BUS --> CLIP
    BUS --> DOS
    BUS --> MEDIA
    LUA -.->|"host-side"| BUS

    subgraph EXT["External World"]
        HOST_KB["Host Keyboard/Mouse"]
        HOST_FS["Host Filesystem"]
        HOST_CB["Host Clipboard"]
        CPUSUB["Worker CPUs"]
        AUDIO_E["Audio Engines"]
    end

    TERM --> HOST_KB
    FIO --> HOST_FS
    DOS --> HOST_FS
    CLIP --> HOST_CB
    PEXEC --> CPU
    COPRO --> CPUSUB
    MEDIA --> AUDIO_E

    classDef periph fill:#8B008B,stroke:#333,color:#fff
    classDef ext fill:#483D8B,stroke:#333,color:#fff
    classDef bus fill:#DC143C,stroke:#333,color:#fff
    class TERM,FIO,PEXEC,COPRO,CLIP,DOS,MEDIA,LUA periph
    class HOST_KB,HOST_FS,HOST_CB,CPUSUB,AUDIO_E ext
    class CPU,BUS bus
```

### Coprocessor Worker Dispatch

The coprocessor manager supports 6 worker CPU types (IE32, IE64, 6502, M68K, Z80, x86) with ticket-based job dispatch and mailbox ring buffers at `0x790000`. Each worker type has its own dedicated memory region (see memory map above). The main CPU enqueues work via MMIO writes; workers execute independently and post results back through their mailbox slots. When `COPROC_IRQ_CTRL` bit 0 is set, the coprocessor fires a Level 6 completion interrupt (INTB_COPER) on job completion, with the finished ticket ID readable from `COPROC_COMPLETED_TICKET`.

### Lua Scripting

The Lua scripting engine (`script_engine.go`) runs in its own goroutine and provides host-side access to the entire bus. It supports an F8 REPL for interactive debugging, video recording, and direct chip register manipulation. Scripts use the `.ies` extension and are loaded via the IE Script Engine.

## 7. Data Flow

### Video Pipeline

```mermaid
graph LR
    EX["CPU Execute"]
    BW["Bus Write<br/>VRAM / MMIO"]
    VR["Video Chip Render<br/>+ Copper display list<br/>+ Blitter DMA"]
    TB["Triple Buffer<br/>Publish"]
    GF["Compositor<br/>GetFrame() atomic swap"]
    ZO["Z-order Compositing<br/>Layers 0-10-12-13-15-20"]
    EB["Ebiten UpdateFrame<br/>RGBA"]
    DSP["Display @ 60Hz"]

    EX --> BW --> VR --> TB --> GF --> ZO --> EB --> DSP

    classDef flow fill:#228B22,stroke:#333,color:#fff
    class EX,BW,VR,TB,GF,ZO,EB,DSP flow
```

### Audio Pipeline

```mermaid
graph LR
    EX["CPU Execute"]
    BW["Bus Write<br/>Audio MMIO"]
    ET["Engine Translate<br/>PSG/SID/POKEY/TED/AHX"]
    SC["SoundChip Params<br/>Updated"]
    OTO["OTO Callback<br/>ReadSample()"]
    TS["TickSample()<br/>All engines"]
    GS["GenerateSample()<br/>Per channel"]
    MIX["Channel Mix<br/>Filter - Overdrive<br/>Reverb - Limiter"]
    DAC["Float32 - OTO<br/>DAC - Speakers<br/>@ 44.1kHz"]

    EX --> BW --> ET --> SC
    OTO --> TS --> GS --> MIX --> DAC

    classDef flow fill:#FF8C00,stroke:#333,color:#fff
    class EX,BW,ET,SC,OTO,TS,GS,MIX,DAC flow
```

### CPU Mode Switching

```mermaid
graph LR
    EXT["File Extension"]
    DET["modeFromExtension()"]
    CRE["createCPURunner()"]
    STOP["Stop CPU +<br/>Stop Compositor"]
    RST["Reset All Chips"]
    LOAD["Load Program"]
    START["Start Compositor +<br/>Start CPU"]

    EXT --> DET --> CRE --> STOP --> RST --> LOAD --> START

    classDef flow fill:#DC143C,stroke:#333,color:#fff
    class EXT,DET,CRE,STOP,RST,LOAD,START flow
```

## 8. Concurrency Model and System Timing

### Goroutine Inventory

| Goroutine | Clock Source | Rate | Synchronisation |
|-----------|-------------|------|-----------------|
| CPU execution | Free-running `for running.Load()` loop | As fast as Go scheduler allows | MMIO writes invoke I/O callbacks under per-chip `mu` |
| VideoChip refresh | `time.NewTicker` | 60Hz | Copper + blitter + dirty-tile copy + buffer swap |
| VGA render | `time.NewTicker` | 60Hz | Renders into triple-buffer slot, publishes via `sharedIdx.Swap()` |
| ULA render | `time.NewTicker` | 60Hz | Same triple-buffer pattern |
| TED render | `time.NewTicker` | 60Hz | Same triple-buffer pattern |
| ANTIC render | `time.NewTicker` | 60Hz | Same triple-buffer pattern |
| Compositor | `time.NewTicker` | 60Hz | Calls `GetFrame()` (atomic swap) on each source, blends, sends to display |
| OTO audio thread | Host audio hardware callback | 44.1kHz | Calls `SoundChip.ReadSample()` -> `GenerateSample()` directly |
| Terminal output | `time.NewTicker` | 100Hz | Flushes output buffer to host terminal |
| Coprocessor workers | On-demand | Varies | Independent CPUs with mailbox ring buffers |
| Script engine | Event-driven | Varies | Lua goroutine with frame-sync channel |

### Three Independent Clocks

```text
CPU:   Free-running (no fixed clock -- instruction throughput varies with host speed)
Video: 6 x 60Hz tickers (one per video chip + compositor), lock-free triple-buffer handoff
Audio: OTO hardware callback drives sample generation at 44.1kHz -- no IE-owned goroutine
```

### Synchronisation Model

**Hot paths** (lock-free):
- `atomic.Bool` — CPU `running`, chip `enabled`, `compositorManaged`
- `atomic.Int32` — triple-buffer `sharedIdx`
- `atomic.Pointer` — selected CPU pointer

**Cold paths** (mutex-protected):
- `sync.Mutex` (`chip.mu`, `video.mu`) — configuration changes, setup, stop

**CPU-to-Video**:
- CPU writes to VRAM/MMIO invoke I/O callbacks under per-chip `mu`
- Video chips read VRAM lock-free (snapshot-render pattern: copy under lock, render without lock)

**CPU-to-Audio**:
- CPU writes to audio MMIO update channel parameters under `chip.mu`
- OTO callback reads parameters atomically or under `chip.mu` for `GenerateSample()`

**No CPU-video vsync coupling**:
- CPU runs ahead freely — no wait-for-vblank blocking
- WAIT register polls `vblankActive atomic.Bool` without blocking the CPU

## Appendix: Key Source Files

| File | Role |
|------|------|
| `registers.go` | Master I/O address map — all region boundaries |
| `machine_bus.go` | MachineBus: autodetected guest RAM, MapIO, ioPageBitmap, Read/Write, SYSINFO accessors |
| `memory_sizing.go` | Boot-time guest RAM autodetection: total guest RAM + active visible RAM with platform reserves |
| `profile_bounds.go` | Source-owned profile bounds for EmuTOS, AROS, EhBASIC |
| `emulator_cpu.go` | EmulatorCPU interface definition |
| `video_interface.go` | VideoSource, VideoOutput, ScanlineAware interfaces |
| `video_compositor.go` | Compositor pipeline, Z-order blending |
| `video_chip.go` | VideoChip + Copper + Blitter + Palette |
| `video_vga.go` | VGA engine (Sequencer, CRTC, GC, AC, DAC) |
| `video_ula.go` | ULA engine (Spectrum display) |
| `video_ted.go` | TED video engine |
| `video_antic.go` | ANTIC + GTIA engines |
| `video_voodoo.go` | Voodoo 3D pipeline |
| `audio_chip.go` | SoundChip (10-channel synthesis) |
| `psg_engine.go` | PSG / AY-3-8910 |
| `sid_engine.go` | SID 6581/8580 |
| `pokey_engine.go` | POKEY |
| `ted_engine.go` | TED audio |
| `ahx_engine.go` | AHX replayer |
| `cpu_ie32.go` | IE32 CPU |
| `cpu_ie64.go` | IE64 CPU + interpreter |
| `fpu_ie64.go` | IE64 FPU (16 x float32) |
| `jit_emit_arm64.go` | IE64 JIT ARM64 emitter |
| `jit_emit_amd64.go` | IE64 JIT x86-64 emitter |
| `cpu_m68k.go` | M68K 68020 |
| `fpu_m68881.go` | M68881 FPU (8 x float64) |
| `cpu_z80.go` | Z80 |
| `cpu_z80_runner.go` | Z80 bus adapter, port I/O bridge, bank windows |
| `cpu_six5go2.go` | 6502 + bus adapter, ioTable dispatch, bank windows |
| `cpu_x86.go` | x86 |
| `cpu_x86_runner.go` | x86 bus adapter, port I/O bridge (PC VGA + shared ports) |
| `fpu_x87.go` | x87 FPU (8 x float64 stack) |
| `terminal_io.go` | Terminal / Serial / Mouse / Keyboard |
| `file_io.go` | File I/O |
| `program_executor.go` | Program Executor |
| `coprocessor_manager.go` | Coprocessor Manager |
| `clipboard_bridge.go` | Clipboard Bridge |
| `media_loader.go` | Media format detection + player dispatch |
| `script_engine.go` | Lua scripting engine |
| `aros_loader.go` | AROS ROM boot manager |
| `aros_dos_intercept.go` | AmigaDOS packet handler (MMIO) |
| `aros_audio_dma.go` | Paula DMA (audio hardware) |

## 9. IntuitionOS Hardening Story

IntuitionOS runs inside the IE64 emulator, so the security model has
two sides: the **guest** (IE64 MMU + `iexec.library` microkernel) and
the **host** (the Go emulator process that executes the guest and
its JIT output). M15.4 closed the major guest-side gaps; M15.6
extends both sides so nothing the kernel relies on is contradicted
by the emulator one layer down.

- **Guest W^X** — every page is exclusively writable or executable
  (see `IE64_ISA.md` §12.10). Code pages map `P|R|X`, data pages
  map `P|R|W`, `PTE.X` and `PTE.W` are never set together.
- **Host W^X** — as of M15.6 the JIT memory region is dual-mapped
  (see `IE64_JIT.md`). The writable view (`PROT_READ|PROT_WRITE`)
  is where `ExecMem.Write` and `PatchRel32At` emit bytes; the
  execution view (`PROT_READ|PROT_EXEC`) is the VA the JIT
  dispatcher jumps to. At no point does any host mapping hold
  both write and execute permission. Prior releases mapped the
  region RWX permanently; that contradiction with the guest W^X
  story is resolved.
- **SMEP / SMAP equivalent** — the IE64 MMU gained `SKEF` and
  `SKAC` bits in M15.6. `SKEF` stops supervisor instruction fetch
  from user-accessible pages (`FAULT_SKEF`). `SKAC` stops
  supervisor data access to user pages (`FAULT_SKAC`) unless the
  kernel has explicitly opened a supervisor-user-access window
  via the `SUAEN` privileged opcode. See `IE64_ISA.md` §12.2.1
  for the complete model.
- **User↔kernel copy contract** — kernel touches of user memory
  are funnelled through named helpers (`copy_from_user`,
  `copy_to_user`, `copy_cstring_from_user`) that bracket every
  access in `SUAEN` / `SUADIS`. Any missed call site faults with
  `FAULT_SKAC` and is loud rather than silent. See
  `IE64_COOKBOOK.md` for the worked idiom and anti-patterns.
- **Trap-frame stack** — nested-trap state (`CR_FAULT_PC`,
  `CR_FAULT_ADDR`, `CR_FAULT_CAUSE`, `CR_PREV_MODE`,
  `CR_SAVED_SUA`) is preserved by the CPU across trap entry and
  ERET so kernel handlers survive a nested synchronous trap
  without a manual MFCR/MTCR save/restore dance. See
  `IE64_ISA.md` §12.14.
- **Stack guard pages** — as of R1 in M15.6, both user stacks and
  the kernel stack reserve one unmapped page below the downward-growing
  stack floor. Overflow is therefore a deterministic `FAULT_NOT_PRESENT`
  instead of silent adjacent-page corruption.
- **Heap guard pages** — as of R2 in M15.6, `AllocMem(MEMF_GUARD)`
  reserves one unmapped page on each side of the mapped allocation.
  The per-task VA allocator treats those guard slots as occupied for the
  life of the region, so a neighboring allocation cannot consume them.
- **Cross-task confidentiality** — private and shared allocator
  pages are zeroed on free before release, so a later owner cannot
  observe prior-task bytes. `MapShared` then narrows consumer-side
  access further with an explicit permission bitmask. See
  `sdk/docs/IntuitionOS/M15.6-plan.md`.

The hardening story is layered rather than siloed: what the guest
kernel enforces (W^X, per-task quotas, exit-time sweeps,
permission-preserving shared mappings) and what the host enforces
(JIT W^X, bootstrap hostfs confinement) are complementary, and
each item has a regression test that fails loudly if the invariant
is dropped.
