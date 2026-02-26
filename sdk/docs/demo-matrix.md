# Demo Matrix

Complete list of SDK example programs with their CPU, video, and audio combinations.

## Rotozoomer Series

The rotozoomer is the canonical "hello world" demo: a hardware-accelerated rotating/zooming texture using the Mode7 blitter (same technique as SNES F-Zero and Mario Kart). Each version is heavily commented to teach both the algorithm and CPU-specific programming patterns. The EhBASIC version (`rotozoomer_basic.bas`) is the primary entry point in the automated product demo — it is loaded, listed, and run first to showcase the BASIC interpreter with hardware blitter and SID music.

| Example | CPU | Video | Audio | Description |
|---------|-----|-------|-------|-------------|
| `rotozoomer.asm` | IE32 | IEVideoChip | AHX | Mode7 blitter + Amiga tracker music |
| `rotozoomer_ie64.asm` | IE64 | IEVideoChip | SAP/POKEY | Mode7 blitter + Atari 8-bit music |
| `rotozoomer_68k.asm` | M68K | IEVideoChip | TED audio | Mode7 blitter + C264 music |
| `rotozoomer_z80.asm` | Z80 | IEVideoChip | SID | Mode7 blitter + C64 music |
| `rotozoomer_65.asm` | 6502 | IEVideoChip | AHX | Mode7 blitter + Amiga tracker music |
| `rotozoomer_x86.asm` | x86 | IEVideoChip | PSG | Mode7 blitter + AY-3-8910 music |
| `rotozoomer_basic.bas` | IE64 (BASIC) | IEVideoChip | SID | Mode7 blitter from EhBASIC |

## Video Chip Showcases

| Example | CPU | Video | Audio | Description |
|---------|-----|-------|-------|-------------|
| `vga_text_hello.asm` | IE32 | VGA (text) | -- | Simplest demo: coloured text on 80x25 screen |
| `vga_mode13h_fire.asm` | IE32 | VGA (Mode 13h) | -- | Classic DOS-era 256-colour fire effect |
| `copper_vga_bands.asm` | IE32 | VGA + Copper | -- | Amiga-style per-scanline palette manipulation |
| `ula_rotating_cube_65.asm` | 6502 | ULA (Spectrum) | AHX | Wireframe 3D cube on ZX Spectrum display |
| `ted_121_colors_68k.asm` | M68K | TED (Plus/4) | PSG | Full-screen plasma using all 121 TED colours |
| `antic_plasma_x86.asm` | x86 | ANTIC/GTIA | SID | Atari 8-bit display list + Player/Missile graphics |
| `vga_text_sap_demo.asm` | Z80 | VGA (text) | POKEY/SAP | VGA text mode with SAP music playback |
| `voodoo_cube_68k.asm` | M68K | Voodoo 3D | -- | Z-buffered 3D cube on 3DFX Voodoo hardware |
| `voodoo_triangle_68k.asm` | M68K | Voodoo 3D | -- | Flat-shaded triangle on 3DFX Voodoo |
| `voodoo_3dfx_logo_68k.asm` | M68K | Voodoo 3D | -- | Textured 3DFX logo flyby with fog |
| `voodoo_mega_demo.asm` | IE32 | Voodoo 3D | SID | Textured 3D scenes with SID music |
| `voodoo_tunnel_z80.asm` | Z80 | Voodoo 3D | -- | Texture-mapped tunnel effect |
| `robocop_intro.asm` | IE32 | IEVideoChip + Copper | -- | Copper rasterbars + blitter sprite |
| `robocop_intro_68k.asm` | M68K | IEVideoChip + Copper | -- | Copper rasterbars + blitter sprite |
| `robocop_intro_65.asm` | 6502 | IEVideoChip + Copper | -- | Copper rasterbars + blitter sprite |
| `robocop_intro_z80.asm` | Z80 | IEVideoChip + Copper | -- | Copper rasterbars + blitter sprite |

## Coprocessor Communication

| Example | CPU | Description |
|---------|-----|-------------|
| `coproc_caller_ie32.asm` | IE32 | Launches a worker, sends a request, reads the result |
| `coproc_service_ie32.asm` | IE32 | Worker that polls a ring buffer and processes requests |

## Coverage Summary

### CPU Cores Covered
IE32, IE64, M68020, Z80, 6502, x86 (32-bit)

### Video Chips Covered
- **IEVideoChip** (up to 1280x960 true colour) - rotozoomer series
- **VGA** (text / Mode 13h) - text hello, fire effect, copper bands
- **ULA** (ZX Spectrum 256x192) - rotating cube
- **TED** (Commodore Plus/4 121 colours) - plasma
- **ANTIC/GTIA** (Atari 8-bit display lists) - plasma
- **Voodoo SST-1** (3DFX hardware 3D) - 3D cube
- **Copper coprocessor** - VGA band manipulation

### Audio Engines Covered
- **IESoundChip** (custom synthesiser) - available in all demos
- **PSG/AY-3-8910** - x86 rotozoomer, TED plasma
- **SID** (Commodore 64) - Z80 rotozoomer, ANTIC plasma, BASIC rotozoomer
- **POKEY/SAP** (Atari 8-bit) - IE64 rotozoomer
- **TED audio** (Commodore Plus/4) - M68K rotozoomer
- **AHX** (Amiga tracker) - IE32 rotozoomer, 6502 rotozoomer, ULA cube

### Audio File Formats (20 formats)
- `.ym` - Atari ST YM format (PSG)
- `.ay` - ZXAYEMUL with embedded Z80 player (PSG; ZX Spectrum/Amstrad CPC/MSX auto-detected)
- `.vgm` / `.vgz` - Video Game Music streams (PSG, SN76489)
- `.vtx` - VTX AY-3-8910 frame dump (PSG)
- `.sndh` - Atari ST SNDH with embedded M68K code (PSG)
- `.pt3` / `.pt2` / `.pt1` - ZX Spectrum ProTracker 3/2/1 (PSG; PT1 native Go, PT2/PT3 via Z80)
- `.stc` - Sound Tracker Compiled (PSG, Z80)
- `.sqt` - SQ-Tracker (PSG, Z80)
- `.asc` - ASC Sound Master (PSG, native Go)
- `.ftc` - Fast Tracker ZX (PSG, native Go)
- `.sid` - Commodore 64 SID tunes (PSID v1-v4, RSID)
- `.ted` - Commodore Plus/4 TED music
- `.sap` - Atari 8-bit SAP files (POKEY)
- `.ahx` - Amiga AHX/THX tracker modules
- `.mod` - ProTracker MOD modules (Amiga 4-channel)
- `.wav` - PCM WAV audio via SoundChip FLEX DAC mode
