; ie65.inc - Intuition Engine 6502 (IE65) Macro Library
;
; Hardware definitions and utility macros for writing 6502 assembly
; programs targeting the Intuition Engine virtual machine.
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; MEMORY MAP CONSTANTS
; ============================================================================

; Bank window addresses
BANK1_WINDOW    = $2000         ; Sprite data bank window (8KB)
BANK2_WINDOW    = $4000         ; Font data bank window (8KB)
BANK3_WINDOW    = $6000         ; General data bank window (8KB)
VRAM_WINDOW     = $8000         ; VRAM bank window (16KB)
BANK_SIZE       = $2000         ; 8KB per extended bank
VRAM_BANK_SIZE  = $4000         ; 16KB VRAM bank

; Bank control registers
BANK1_REG_LO    = $F700         ; Bank 1 select (low byte)
BANK1_REG_HI    = $F701         ; Bank 1 select (high byte)
BANK2_REG_LO    = $F702         ; Bank 2 select (low byte)
BANK2_REG_HI    = $F703         ; Bank 2 select (high byte)
BANK3_REG_LO    = $F704         ; Bank 3 select (low byte)
BANK3_REG_HI    = $F705         ; Bank 3 select (high byte)
VRAM_BANK_REG   = $F7F0         ; VRAM bank select

; ============================================================================
; VIDEO HARDWARE REGISTERS (mapped from IE32 $F0000-$F00FF to 6502 $F000-$F0FF)
; ============================================================================

VIDEO_CTRL      = $F000         ; Video control (enable/disable)
VIDEO_MODE      = $F004         ; Video mode (resolution)
VIDEO_STATUS    = $F008         ; Video status (VBlank flag)
STATUS_VBLANK   = 2             ; VBlank status bit

; Copper coprocessor
COPPER_CTRL     = $F00C         ; Copper control
COPPER_PTR_0    = $F010         ; Copper list pointer byte 0 (LSB)
COPPER_PTR_1    = $F011         ; Copper list pointer byte 1
COPPER_PTR_2    = $F012         ; Copper list pointer byte 2
COPPER_PTR_3    = $F013         ; Copper list pointer byte 3 (MSB)

; Blitter hardware
BLT_CTRL        = $F01C         ; Blitter control
BLT_OP          = $F020         ; Blitter operation
BLT_SRC_0       = $F024         ; Source address byte 0 (LSB)
BLT_SRC_1       = $F025         ; Source address byte 1
BLT_SRC_2       = $F026         ; Source address byte 2
BLT_SRC_3       = $F027         ; Source address byte 3 (MSB)
BLT_DST_0       = $F028         ; Dest address byte 0 (LSB)
BLT_DST_1       = $F029         ; Dest address byte 1
BLT_DST_2       = $F02A         ; Dest address byte 2
BLT_DST_3       = $F02B         ; Dest address byte 3 (MSB)
BLT_WIDTH_LO    = $F02C         ; Blit width (low byte)
BLT_WIDTH_HI    = $F02D         ; Blit width (high byte)
BLT_HEIGHT_LO   = $F030         ; Blit height (low byte)
BLT_HEIGHT_HI   = $F031         ; Blit height (high byte)
BLT_SRC_STRIDE_LO = $F034       ; Source stride (low byte)
BLT_SRC_STRIDE_HI = $F035       ; Source stride (high byte)
BLT_DST_STRIDE_LO = $F038       ; Dest stride (low byte)
BLT_DST_STRIDE_HI = $F039       ; Dest stride (high byte)
BLT_COLOR_0     = $F03C         ; Fill color byte 0
BLT_COLOR_1     = $F03D         ; Fill color byte 1
BLT_COLOR_2     = $F03E         ; Fill color byte 2
BLT_COLOR_3     = $F03F         ; Fill color byte 3
BLT_MASK_0      = $F040         ; Mask address byte 0 (LSB)
BLT_MASK_1      = $F041         ; Mask address byte 1
BLT_MASK_2      = $F042         ; Mask address byte 2
BLT_MASK_3      = $F043         ; Mask address byte 3 (MSB)
BLT_STATUS      = $F044         ; Blitter status

; Raster registers
VIDEO_RASTER_Y_LO     = $F048   ; Raster Y position (low)
VIDEO_RASTER_Y_HI     = $F049   ; Raster Y position (high)
VIDEO_RASTER_HEIGHT_LO = $F04C  ; Raster height (low)
VIDEO_RASTER_HEIGHT_HI = $F04D  ; Raster height (high)
VIDEO_RASTER_COLOR_0  = $F050   ; Raster color byte 0
VIDEO_RASTER_COLOR_1  = $F051   ; Raster color byte 1
VIDEO_RASTER_COLOR_2  = $F052   ; Raster color byte 2
VIDEO_RASTER_COLOR_3  = $F053   ; Raster color byte 3
VIDEO_RASTER_CTRL     = $F054   ; Raster control

; ============================================================================
; PSG/AUDIO REGISTERS (mapped from IE32 $F0C00+ to 6502 $FC00+)
; Translation: 6502 $FCxx -> IE32 $F0Cxx via formula $F0000 + (addr - $F000)
; ============================================================================

PSG_PLUS_CTRL   = $FC0E         ; PSG+ mode control (IE32 $F0C0E)

; ----------------------------------------------------------------------------
; PSG Player Registers
;
; The PSG player supports multiple music formats with automatic detection:
;   .ay   - ZX Spectrum format with embedded Z80 code (executed by emulator)
;   .sndh - Atari ST format with embedded M68K code (executed by emulator)
;   .ym   - YM2149 register dump frames (50Hz playback)
;   .vgm  - Video Game Music format with timed PSG events
;
; Usage:
;   1. Embed music data in your program using incbin
;   2. Set PSG_PLAY_PTR bytes to the address of the music data (little-endian)
;   3. Set PSG_PLAY_LEN bytes to the size of the music data (little-endian)
;   4. Write to PSG_PLAY_CTRL to start/stop playback
;
; PSG_PLAY_CTRL bits:
;   bit 0 ($01) - Start playback (write 1 to begin)
;   bit 1 ($02) - Stop playback (write 1 to stop)
;   bit 2 ($04) - Enable looping (set before starting)
;
; PSG_PLAY_STATUS bits:
;   bit 0 ($01) - Busy (1 = playing, 0 = stopped)
;   bit 1 ($02) - Error (1 = load/parse error)
; ----------------------------------------------------------------------------
PSG_PLAY_PTR_0  = $FC10         ; Play pointer byte 0 (IE32 $F0C10)
PSG_PLAY_PTR_1  = $FC11         ; Play pointer byte 1
PSG_PLAY_PTR_2  = $FC12         ; Play pointer byte 2
PSG_PLAY_PTR_3  = $FC13         ; Play pointer byte 3
PSG_PLAY_LEN_0  = $FC14         ; Play length byte 0 (IE32 $F0C14)
PSG_PLAY_LEN_1  = $FC15         ; Play length byte 1
PSG_PLAY_LEN_2  = $FC16         ; Play length byte 2
PSG_PLAY_LEN_3  = $FC17         ; Play length byte 3
PSG_PLAY_CTRL   = $FC18         ; Play control (IE32 $F0C18)
PSG_PLAY_STATUS = $FC1C         ; Play status (IE32 $F0C1C)

; ============================================================================
; POKEY AUDIO REGISTERS (Atari-style addresses $D200-$D209)
; ============================================================================

POKEY_BASE      = $D200         ; POKEY base address
POKEY_AUDF1     = $D200         ; Channel 1 frequency divider
POKEY_AUDC1     = $D201         ; Channel 1 control (distortion + volume)
POKEY_AUDF2     = $D202         ; Channel 2 frequency divider
POKEY_AUDC2     = $D203         ; Channel 2 control
POKEY_AUDF3     = $D204         ; Channel 3 frequency divider
POKEY_AUDC3     = $D205         ; Channel 3 control
POKEY_AUDF4     = $D206         ; Channel 4 frequency divider
POKEY_AUDC4     = $D207         ; Channel 4 control
POKEY_AUDCTL    = $D208         ; Master audio control
POKEY_PLUS_CTRL = $D209         ; POKEY+ mode (0=standard, 1=enhanced)

; AUDCTL bit masks
AUDCTL_CLOCK_15KHZ  = $01       ; Use 15kHz base clock (else 64kHz)
AUDCTL_HIPASS_CH1   = $02       ; High-pass filter ch1 by ch3
AUDCTL_HIPASS_CH2   = $04       ; High-pass filter ch2 by ch4
AUDCTL_CH4_BY_CH3   = $08       ; Ch4 clocked by ch3 (16-bit mode)
AUDCTL_CH2_BY_CH1   = $10       ; Ch2 clocked by ch1 (16-bit mode)
AUDCTL_CH3_179MHZ   = $20       ; Ch3 uses 1.79MHz clock
AUDCTL_CH1_179MHZ   = $40       ; Ch1 uses 1.79MHz clock
AUDCTL_POLY9        = $80       ; Use 9-bit poly instead of 17-bit

; AUDC distortion modes (bits 5-7)
POKEY_DIST_POLY17_POLY5 = $00   ; 17-bit + 5-bit poly
POKEY_DIST_POLY5        = $20   ; 5-bit poly only
POKEY_DIST_POLY17_POLY4 = $40   ; 17-bit + 4-bit poly
POKEY_DIST_POLY5_POLY4  = $60   ; 5-bit + 4-bit poly
POKEY_DIST_POLY17       = $80   ; 17-bit poly only
POKEY_DIST_PURE_TONE    = $A0   ; Pure square wave
POKEY_DIST_POLY4        = $C0   ; 4-bit poly only
POKEY_DIST_POLY17_PULSE = $E0   ; 17-bit + pulse

; ----------------------------------------------------------------------------
; SID AUDIO REGISTERS (MOS 6581/8580 style)
;
; The SID chip provides 3 voices with analog-style synthesis:
; - 16-bit frequency control per voice
; - 12-bit pulse width for duty cycle
; - 4 waveforms: triangle, sawtooth, pulse, noise
; - ADSR envelope per voice
; - Ring modulation and hard sync
; - Programmable resonant filter (LP/BP/HP)
;
; For 6502: SID is memory-mapped at $D500-$D51C
; Note: C64's original SID was at $D400, but PSG uses that range.
;
; SID+ mode (write 1 to SID_PLUS_CTRL) enables enhanced audio
; ----------------------------------------------------------------------------
SID_BASE        = $D500         ; SID base address

; Voice 1 registers
SID_V1_FREQ_LO  = $D500         ; Voice 1 frequency low byte
SID_V1_FREQ_HI  = $D501         ; Voice 1 frequency high byte
SID_V1_PW_LO    = $D502         ; Voice 1 pulse width low byte
SID_V1_PW_HI    = $D503         ; Voice 1 pulse width high (bits 0-3)
SID_V1_CTRL     = $D504         ; Voice 1 control register
SID_V1_AD       = $D505         ; Voice 1 attack/decay
SID_V1_SR       = $D506         ; Voice 1 sustain/release

; Voice 2 registers
SID_V2_FREQ_LO  = $D507         ; Voice 2 frequency low byte
SID_V2_FREQ_HI  = $D508         ; Voice 2 frequency high byte
SID_V2_PW_LO    = $D509         ; Voice 2 pulse width low byte
SID_V2_PW_HI    = $D50A         ; Voice 2 pulse width high
SID_V2_CTRL     = $D50B         ; Voice 2 control register
SID_V2_AD       = $D50C         ; Voice 2 attack/decay
SID_V2_SR       = $D50D         ; Voice 2 sustain/release

; Voice 3 registers
SID_V3_FREQ_LO  = $D50E         ; Voice 3 frequency low byte
SID_V3_FREQ_HI  = $D50F         ; Voice 3 frequency high byte
SID_V3_PW_LO    = $D510         ; Voice 3 pulse width low byte
SID_V3_PW_HI    = $D511         ; Voice 3 pulse width high
SID_V3_CTRL     = $D512         ; Voice 3 control register
SID_V3_AD       = $D513         ; Voice 3 attack/decay
SID_V3_SR       = $D514         ; Voice 3 sustain/release

; Filter and volume registers
SID_FC_LO       = $D515         ; Filter cutoff low (bits 0-2)
SID_FC_HI       = $D516         ; Filter cutoff high byte
SID_RES_FILT    = $D517         ; Resonance (bits 4-7) + routing (bits 0-3)
SID_MODE_VOL    = $D518         ; Volume (bits 0-3) + filter mode (bits 4-7)
SID_PLUS_CTRL   = $D519         ; SID+ mode (0=standard, 1=enhanced)

; Voice control register bits (SID_Vx_CTRL)
SID_CTRL_GATE     = $01         ; Gate bit (trigger envelope)
SID_CTRL_SYNC     = $02         ; Sync with previous voice
SID_CTRL_RINGMOD  = $04         ; Ring modulation
SID_CTRL_TEST     = $08         ; Test bit (resets oscillator)
SID_CTRL_TRIANGLE = $10         ; Triangle waveform
SID_CTRL_SAWTOOTH = $20         ; Sawtooth waveform
SID_CTRL_PULSE    = $40         ; Pulse/square waveform
SID_CTRL_NOISE    = $80         ; Noise waveform

; Filter routing bits (SID_RES_FILT bits 0-3)
SID_FILT_V1       = $01         ; Route voice 1 through filter
SID_FILT_V2       = $02         ; Route voice 2 through filter
SID_FILT_V3       = $04         ; Route voice 3 through filter
SID_FILT_EXT      = $08         ; Route external input through filter

; Filter mode bits (SID_MODE_VOL bits 4-7)
SID_MODE_LP       = $10         ; Low-pass filter
SID_MODE_BP       = $20         ; Band-pass filter
SID_MODE_HP       = $40         ; High-pass filter
SID_MODE_3OFF     = $80         ; Disconnect voice 3 from output

; ----------------------------------------------------------------------------
; SID Player Registers
;
; The SID player handles Commodore 64 music files (.sid) which contain
; embedded 6502 code that drives the SID sound chip. The player executes
; the 6502 init routine once, then calls the play routine each frame.
;
; Usage:
;   1. Embed SID data in your program using .incbin
;   2. Set SID_PLAY_PTR bytes to the address of the SID data (little-endian)
;   3. Set SID_PLAY_LEN bytes to the size of the SID data (little-endian)
;   4. Optionally set SID_SUBSONG to select a subsong (default 0)
;   5. Write to SID_PLAY_CTRL to start/stop playback
;
; SID_PLAY_CTRL bits:
;   bit 0 ($01) - Start playback (write 1 to begin)
;   bit 1 ($02) - Stop playback (write 1 to stop)
;   bit 2 ($04) - Enable looping (set before starting)
;
; SID_PLAY_STATUS bits:
;   bit 0 ($01) - Busy (1 = playing, 0 = stopped)
;   bit 1 ($02) - Error (1 = load/parse error)
; ----------------------------------------------------------------------------
SID_PLAY_PTR_0  = $FE20         ; Play pointer byte 0 (IE32 $F0E20)
SID_PLAY_PTR_1  = $FE21         ; Play pointer byte 1
SID_PLAY_PTR_2  = $FE22         ; Play pointer byte 2
SID_PLAY_PTR_3  = $FE23         ; Play pointer byte 3
SID_PLAY_LEN_0  = $FE24         ; Play length byte 0 (IE32 $F0E24)
SID_PLAY_LEN_1  = $FE25         ; Play length byte 1
SID_PLAY_LEN_2  = $FE26         ; Play length byte 2
SID_PLAY_LEN_3  = $FE27         ; Play length byte 3
SID_PLAY_CTRL   = $FE28         ; Play control (IE32 $F0E28)
SID_PLAY_STATUS = $FE2C         ; Play status (IE32 $F0E2C)
SID_SUBSONG     = $FE2D         ; Subsong selection (0-255)

; ============================================================================
; TED AUDIO REGISTERS (Commodore Plus/4 style)
;
; The TED chip provides 2 voices with simple square wave synthesis:
; - 10-bit frequency control per voice (0-1023)
; - Voice 2 can optionally produce white noise
; - Global 4-bit volume (0-8, where 8 is maximum)
;
; For 6502: TED is memory-mapped at $D600-$D605
;
; TED+ mode (write 1 to TED_PLUS_CTRL) enables enhanced audio:
; - 4x oversampling for cleaner waveforms
; - Soft low-pass filtering for smoother output
; - Subtle saturation for analog warmth
; - Room reverb for spatial depth
; - Logarithmic volume curve for more musical response
; - Per-voice stereo separation
;
; Frequency formula: freq_hz = clock/8 / (1024 - register_value)
; Clock: 886724 Hz (PAL), 894886 Hz (NTSC)
; ============================================================================
TED_BASE        = $D600         ; TED base address
TED_FREQ1_LO    = $D600         ; Voice 1 frequency low byte
TED_FREQ2_LO    = $D601         ; Voice 2 frequency low byte
TED_FREQ2_HI    = $D602         ; Voice 2 frequency high (bits 0-1)
TED_SND_CTRL    = $D603         ; Sound control register
TED_FREQ1_HI    = $D604         ; Voice 1 frequency high (bits 0-1)
TED_PLUS_CTRL   = $D605         ; TED+ mode (0=standard, 1=enhanced)

; TED Control register bits (TED_SND_CTRL)
TED_CTRL_SNDDC     = $80        ; D/A mode (bit 7)
TED_CTRL_SND2NOISE = $40        ; Voice 2 noise enable (bit 6)
TED_CTRL_SND2ON    = $20        ; Voice 2 enable (bit 5)
TED_CTRL_SND1ON    = $10        ; Voice 1 enable (bit 4)
TED_CTRL_VOLUME    = $0F        ; Volume mask (bits 0-3, max=8)

; ----------------------------------------------------------------------------
; TED Player Registers
;
; The TED player handles Commodore Plus/4 music files (.ted) which contain
; embedded 6502 code that drives the TED sound chip. The player executes
; the 6502 init routine once, then calls the play routine each frame.
;
; Usage:
;   1. Embed TED data in your program using .incbin
;   2. Set TED_PLAY_PTR bytes to the address of the TED data (little-endian)
;   3. Set TED_PLAY_LEN bytes to the size of the TED data (little-endian)
;   4. Write to TED_PLAY_CTRL to start/stop playback
;
; TED_PLAY_CTRL bits:
;   bit 0 ($01) - Start playback (write 1 to begin)
;   bit 1 ($02) - Stop playback (write 1 to stop)
;   bit 2 ($04) - Enable looping (set before starting)
;
; TED_PLAY_STATUS bits:
;   bit 0 ($01) - Busy (1 = playing, 0 = stopped)
;   bit 1 ($02) - Error (1 = load/parse error)
; ----------------------------------------------------------------------------
TED_PLAY_PTR_0  = $FF10         ; Play pointer byte 0 (IE32 $F0F10)
TED_PLAY_PTR_1  = $FF11         ; Play pointer byte 1
TED_PLAY_PTR_2  = $FF12         ; Play pointer byte 2
TED_PLAY_PTR_3  = $FF13         ; Play pointer byte 3
TED_PLAY_LEN_0  = $FF14         ; Play length byte 0 (IE32 $F0F14)
TED_PLAY_LEN_1  = $FF15         ; Play length byte 1
TED_PLAY_LEN_2  = $FF16         ; Play length byte 2
TED_PLAY_LEN_3  = $FF17         ; Play length byte 3
TED_PLAY_CTRL   = $FF18         ; Play control (IE32 $F0F18)
TED_PLAY_STATUS = $FF1C         ; Play status (IE32 $F0F1C)

; ============================================================================
; TED VIDEO REGISTERS (Commodore Plus/4 style)
;
; The TED chip also provides video capabilities:
; - 40x25 text mode (8x8 character cells)
; - 320x200 pixel resolution (384x272 with border)
; - 121 colors (16 hues × 8 luminances)
; - Hardware cursor support
; - Compositor layer 12 (between VGA=10 and ULA=15)
;
; For 6502: TED video registers are memory-mapped at $D620-$D62F
; IE32/M68K address: $F0F20-$F0F5F (4-byte aligned)
; ============================================================================
TED_V_BASE      = $D620         ; TED video register base

; Control registers
TED_V_CTRL1     = $D620         ; Control 1 (ECM/BMM/DEN/RSEL/YSCROLL)
TED_V_CTRL2     = $D621         ; Control 2 (RES/MCM/CSEL/XSCROLL)
TED_V_CHAR_BASE = $D622         ; Character/bitmap base address
TED_V_VID_BASE  = $D623         ; Video matrix base address

; Color registers
TED_V_BG0       = $D624         ; Background color 0
TED_V_BG1       = $D625         ; Background color 1 (multicolor)
TED_V_BG2       = $D626         ; Background color 2 (multicolor)
TED_V_BG3       = $D627         ; Background color 3 (multicolor)
TED_V_BORDER    = $D628         ; Border color

; Cursor registers
TED_V_CURSOR_HI = $D629         ; Cursor position high byte
TED_V_CURSOR_LO = $D62A         ; Cursor position low byte
TED_V_CURSOR_C  = $D62B         ; Cursor color

; Status registers
TED_V_RASTER_LO = $D62C         ; Raster line low (read-only)
TED_V_RASTER_HI = $D62D         ; Raster line high (read-only)
TED_V_ENABLE    = $D62E         ; Video enable (bit 0)
TED_V_STATUS    = $D62F         ; Status (bit 0 = VBlank)

; TED_V_CTRL1 bits
TED_V_CTRL1_ECM     = $40       ; Bit 6: Extended Color Mode
TED_V_CTRL1_BMM     = $20       ; Bit 5: Bitmap Mode
TED_V_CTRL1_DEN     = $10       ; Bit 4: Display Enable
TED_V_CTRL1_RSEL    = $08       ; Bit 3: Row Select (0=24, 1=25)
TED_V_CTRL1_YSCROLL = $07       ; Bits 0-2: Vertical scroll mask

; TED_V_CTRL2 bits
TED_V_CTRL2_RES     = $20       ; Bit 5: Reset
TED_V_CTRL2_MCM     = $10       ; Bit 4: Multicolor Mode
TED_V_CTRL2_CSEL    = $08       ; Bit 3: Column Select (0=38, 1=40)
TED_V_CTRL2_XSCROLL = $07       ; Bits 0-2: Horizontal scroll mask

; Enable/Status bits
TED_V_ENABLE_VIDEO  = $01       ; Bit 0: Video enable
TED_V_STATUS_VBLANK = $01       ; Bit 0: VBlank active

; ----------------------------------------------------------------------------
; SAP Player Registers
;
; The SAP player handles Atari 8-bit music files (.sap) which contain
; embedded 6502 code that drives the POKEY sound chip. Supports TYPE B files
; where INIT is called once and PLAYER is called each frame.
;
; Usage:
;   1. Embed SAP data in your program using .incbin
;   2. Set SAP_PLAY_PTR bytes to the address of the SAP data (little-endian)
;   3. Set SAP_PLAY_LEN bytes to the size of the SAP data (little-endian)
;   4. Optionally set SAP_SUBSONG to select a subsong (default 0)
;   5. Write to SAP_PLAY_CTRL to start/stop playback
;
; SAP_PLAY_CTRL bits:
;   bit 0 ($01) - Start playback (write 1 to begin)
;   bit 1 ($02) - Stop playback (write 1 to stop)
;   bit 2 ($04) - Enable looping (set before starting)
;
; SAP_PLAY_STATUS bits:
;   bit 0 ($01) - Busy (1 = playing, 0 = stopped)
;   bit 1 ($02) - Error (1 = load/parse error)
; ----------------------------------------------------------------------------
SAP_PLAY_PTR_0  = $FD10         ; Play pointer byte 0 (IE32 $F0D10)
SAP_PLAY_PTR_1  = $FD11         ; Play pointer byte 1
SAP_PLAY_PTR_2  = $FD12         ; Play pointer byte 2
SAP_PLAY_PTR_3  = $FD13         ; Play pointer byte 3
SAP_PLAY_LEN_0  = $FD14         ; Play length byte 0 (IE32 $F0D14)
SAP_PLAY_LEN_1  = $FD15         ; Play length byte 1
SAP_PLAY_LEN_2  = $FD16         ; Play length byte 2
SAP_PLAY_LEN_3  = $FD17         ; Play length byte 3
SAP_PLAY_CTRL   = $FD18         ; Play control (IE32 $F0D18)
SAP_PLAY_STATUS = $FD1C         ; Play status (IE32 $F0D1C)
SAP_SUBSONG     = $FD1D         ; Subsong selection (0-255)

; ============================================================================
; AHX AUDIO REGISTERS (Amiga AHX module player)
;
; The AHX engine provides Amiga AHX/THX module playback:
; - 4 channels with waveform synthesis (triangle, sawtooth, square, noise)
; - Per-channel filter modulation
; - Instrument-based sequencing with effects
;
; AHX+ mode (write 1 to AHX_PLUS_CTRL) enables enhanced audio:
; - 4x oversampling for cleaner waveforms
; - Soft low-pass filtering for smoother output
; - Subtle saturation for analog warmth
; - Room reverb for Amiga-style ambience
; - Authentic stereo panning (L-R-R-L pattern)
; - Hardware PWM for square wave duty cycle modulation
;
; For 6502: AHX registers are at $FB80-$FB91 (IE32 $F0B80-$F0B91)
; ============================================================================
AHX_PLUS_CTRL   = $FB80         ; AHX+ mode (0=standard, 1=enhanced)

; ----------------------------------------------------------------------------
; AHX Player Registers
;
; The AHX player handles Amiga AHX/THX modules which contain instrument
; definitions and pattern data for 4-channel waveform synthesis.
;
; Usage:
;   1. Embed AHX data in your program using .incbin
;   2. Set AHX_PLAY_PTR bytes to the address of the AHX data (little-endian)
;   3. Set AHX_PLAY_LEN bytes to the size of the AHX data (little-endian)
;   4. Optionally set AHX_SUBSONG to select a subsong (default 0)
;   5. Optionally write 1 to AHX_PLUS_CTRL to enable enhanced mode
;   6. Write to AHX_PLAY_CTRL to start/stop playback
;
; AHX_PLAY_CTRL bits:
;   bit 0 ($01) - Start playback (write 1 to begin)
;   bit 1 ($02) - Stop playback (write 1 to stop)
;   bit 2 ($04) - Enable looping (set before starting)
;
; AHX_PLAY_STATUS bits:
;   bit 0 ($01) - Busy (1 = playing, 0 = stopped)
;   bit 1 ($02) - Error (1 = load/parse error)
; ----------------------------------------------------------------------------
AHX_PLAY_PTR_0  = $FB84         ; Play pointer byte 0 (IE32 $F0B84)
AHX_PLAY_PTR_1  = $FB85         ; Play pointer byte 1
AHX_PLAY_PTR_2  = $FB86         ; Play pointer byte 2
AHX_PLAY_PTR_3  = $FB87         ; Play pointer byte 3
AHX_PLAY_LEN_0  = $FB88         ; Play length byte 0 (IE32 $F0B88)
AHX_PLAY_LEN_1  = $FB89         ; Play length byte 1
AHX_PLAY_LEN_2  = $FB8A         ; Play length byte 2
AHX_PLAY_LEN_3  = $FB8B         ; Play length byte 3
AHX_PLAY_CTRL   = $FB8C         ; Play control (IE32 $F0B8C)
AHX_PLAY_STATUS = $FB90         ; Play status (IE32 $F0B90)
AHX_SUBSONG     = $FB91         ; Subsong selection (0-255)

; ----------------------------------------------------------------------------
; AHX Player Macros
; ----------------------------------------------------------------------------

; Set AHX data pointer (32-bit address)
.macro SET_AHX_PTR addr
    lda #<(addr)
    sta AHX_PLAY_PTR_0
    lda #>(addr)
    sta AHX_PLAY_PTR_1
    lda #^(addr)
    sta AHX_PLAY_PTR_2
    lda #.lobyte(.hiword(addr) >> 8)
    sta AHX_PLAY_PTR_3
.endmacro

; Set AHX data length (32-bit)
.macro SET_AHX_LEN len
    lda #<(len)
    sta AHX_PLAY_LEN_0
    lda #>(len)
    sta AHX_PLAY_LEN_1
    lda #^(len)
    sta AHX_PLAY_LEN_2
    lda #.lobyte(.hiword(len) >> 8)
    sta AHX_PLAY_LEN_3
.endmacro

; Start AHX playback (no loop)
.macro START_AHX_PLAY
    lda #1
    sta AHX_PLAY_CTRL
.endmacro

; Start AHX playback with looping
.macro START_AHX_LOOP
    lda #5
    sta AHX_PLAY_CTRL
.endmacro

; Stop AHX playback
.macro STOP_AHX_PLAY
    lda #2
    sta AHX_PLAY_CTRL
.endmacro

; Enable AHX+ enhanced mode
.macro ENABLE_AHX_PLUS
    lda #1
    sta AHX_PLUS_CTRL
.endmacro

; ----------------------------------------------------------------------------
; PSG Player Macros
; ----------------------------------------------------------------------------

; Set PSG play pointer (32-bit address)
.macro SET_PSG_PTR addr
    lda #<(addr)
    sta PSG_PLAY_PTR_0
    lda #>(addr)
    sta PSG_PLAY_PTR_1
    lda #^(addr)
    sta PSG_PLAY_PTR_2
    lda #.lobyte(.hiword(addr) >> 8)
    sta PSG_PLAY_PTR_3
.endmacro

; Set PSG play length (32-bit)
.macro SET_PSG_LEN len
    lda #<(len)
    sta PSG_PLAY_LEN_0
    lda #>(len)
    sta PSG_PLAY_LEN_1
    lda #^(len)
    sta PSG_PLAY_LEN_2
    lda #.lobyte(.hiword(len) >> 8)
    sta PSG_PLAY_LEN_3
.endmacro

; Start PSG playback (no loop)
.macro START_PSG_PLAY
    lda #1
    sta PSG_PLAY_CTRL
.endmacro

; Start PSG playback with looping
.macro START_PSG_LOOP
    lda #5
    sta PSG_PLAY_CTRL
.endmacro

; Stop PSG playback
.macro STOP_PSG_PLAY
    lda #2
    sta PSG_PLAY_CTRL
.endmacro

; Enable PSG+ enhanced mode
.macro ENABLE_PSG_PLUS
    lda #1
    sta PSG_PLUS_CTRL
.endmacro

; ----------------------------------------------------------------------------
; SID Player Macros
; ----------------------------------------------------------------------------

; Set SID play pointer (32-bit address)
.macro SET_SID_PTR addr
    lda #<(addr)
    sta SID_PLAY_PTR_0
    lda #>(addr)
    sta SID_PLAY_PTR_1
    lda #^(addr)
    sta SID_PLAY_PTR_2
    lda #.lobyte(.hiword(addr) >> 8)
    sta SID_PLAY_PTR_3
.endmacro

; Set SID play length (32-bit)
.macro SET_SID_LEN len
    lda #<(len)
    sta SID_PLAY_LEN_0
    lda #>(len)
    sta SID_PLAY_LEN_1
    lda #^(len)
    sta SID_PLAY_LEN_2
    lda #.lobyte(.hiword(len) >> 8)
    sta SID_PLAY_LEN_3
.endmacro

; Set SID subsong (0-255)
.macro SET_SID_SUBSONG num
    lda #num
    sta SID_SUBSONG
.endmacro

; Start SID playback (no loop)
.macro START_SID_PLAY
    lda #1
    sta SID_PLAY_CTRL
.endmacro

; Start SID playback with looping
.macro START_SID_LOOP
    lda #5
    sta SID_PLAY_CTRL
.endmacro

; Stop SID playback
.macro STOP_SID_PLAY
    lda #2
    sta SID_PLAY_CTRL
.endmacro

; Enable SID+ enhanced mode
.macro ENABLE_SID_PLUS
    lda #1
    sta SID_PLUS_CTRL
.endmacro

; ----------------------------------------------------------------------------
; TED Player Macros
; ----------------------------------------------------------------------------

; Set TED play pointer (32-bit address)
.macro SET_TED_PTR addr
    lda #<(addr)
    sta TED_PLAY_PTR_0
    lda #>(addr)
    sta TED_PLAY_PTR_1
    lda #^(addr)
    sta TED_PLAY_PTR_2
    lda #.lobyte(.hiword(addr) >> 8)
    sta TED_PLAY_PTR_3
.endmacro

; Set TED play length (32-bit)
.macro SET_TED_LEN len
    lda #<(len)
    sta TED_PLAY_LEN_0
    lda #>(len)
    sta TED_PLAY_LEN_1
    lda #^(len)
    sta TED_PLAY_LEN_2
    lda #.lobyte(.hiword(len) >> 8)
    sta TED_PLAY_LEN_3
.endmacro

; Start TED playback (no loop)
.macro START_TED_PLAY
    lda #1
    sta TED_PLAY_CTRL
.endmacro

; Start TED playback with looping
.macro START_TED_LOOP
    lda #5
    sta TED_PLAY_CTRL
.endmacro

; Stop TED playback
.macro STOP_TED_PLAY
    lda #2
    sta TED_PLAY_CTRL
.endmacro

; Enable TED+ enhanced mode
.macro ENABLE_TED_PLUS
    lda #1
    sta TED_PLUS_CTRL
.endmacro

; ----------------------------------------------------------------------------
; TED Video Macros
; ----------------------------------------------------------------------------

; Enable TED video
.macro TED_V_ENABLE_ON
    lda #TED_V_ENABLE_VIDEO
    sta TED_V_ENABLE
.endmacro

; Disable TED video
.macro TED_V_DISABLE
    lda #0
    sta TED_V_ENABLE
.endmacro

; Set TED border color (color byte = (lum<<4)|hue)
.macro TED_V_SET_BORDER color
    lda #color
    sta TED_V_BORDER
.endmacro

; Set TED background color 0 (color byte = (lum<<4)|hue)
.macro TED_V_SET_BG0 color
    lda #color
    sta TED_V_BG0
.endmacro

; Wait for TED VBlank
.macro TED_V_WAIT_VBLANK
:   lda TED_V_STATUS
    and #TED_V_STATUS_VBLANK
    beq :-
.endmacro

; ============================================================================
; VGA VIDEO REGISTERS (IBM VGA compatible)
;
; The VGA chip provides IBM PC-compatible graphics modes:
; - Mode $03: 80x25 text mode, 16 colors
; - Mode $12: 640x480, 16 colors, planar
; - Mode $13: 320x200, 256 colors, linear (Mode 13h)
; - Mode $14: 320x240, 256 colors, planar (Mode X)
;
; For 6502: VGA registers are memory-mapped at $D700-$D70A
; VGA VRAM is banked at $A000 (use VGA_VRAM_BANK register)
; ============================================================================

VGA_BASE        = $D700
VGA_MODE        = $D700         ; Mode control
VGA_STATUS      = $D701         ; Status register
VGA_CTRL        = $D702         ; Control register
VGA_SEQ_IDX     = $D703         ; Sequencer index
VGA_SEQ_DATA    = $D704         ; Sequencer data
VGA_CRTC_IDX    = $D705         ; CRTC index
VGA_CRTC_DATA   = $D706         ; CRTC data
VGA_GC_IDX      = $D707         ; Graphics controller index
VGA_GC_DATA     = $D708         ; Graphics controller data
VGA_DAC_WIDX    = $D709         ; DAC write index
VGA_DAC_DATA    = $D70A         ; DAC data (R,G,B sequence)
VGA_DAC_RIDX    = $D70B         ; DAC read index
VGA_DAC_MASK    = $D70C         ; DAC pixel mask
VGA_VRAM_BANK   = $D70D         ; VRAM bank select

; VGA VRAM window (banked at $A000)
VGA_VRAM        = $A000         ; VRAM window (16KB)
VGA_VRAM_SIZE   = $4000         ; 16KB per bank

; Mode constants
VGA_MODE_TEXT   = $03           ; 80x25 text mode
VGA_MODE_12H    = $12           ; 640x480, 16 colors
VGA_MODE_13H    = $13           ; 320x200, 256 colors
VGA_MODE_X      = $14           ; 320x240, 256 colors

; Status bits
VGA_STATUS_VSYNC  = $01         ; VSync active
VGA_STATUS_RETRACE = $08        ; Vertical retrace

; Control bits
VGA_CTRL_ENABLE = $01           ; VGA enable

; ============================================================================
; CUSTOM AUDIO CHIP REGISTERS
; ============================================================================
;
; The Intuition Engine features a powerful custom 4-channel audio synthesizer
; with additional flexible synthesis channels. This chip provides:
;
;   - 5 dedicated waveform channels: Square, Triangle, Sine, Noise, Sawtooth
;   - 4 flexible synthesis channels with selectable waveforms
;   - Per-voice ADSR envelope generators (16-bit parameters)
;   - Pulse width modulation with LFO
;   - Frequency sweep (portamento/pitch bend)
;   - Hard sync between oscillators
;   - Ring modulation
;   - Global resonant filter (low-pass, high-pass, band-pass)
;   - Effects: overdrive distortion, reverb
;
; Memory Map (6502 addresses, mapped from IE32 $F08xx-$F0Bxx to $F8xx-$FBxx):
;   $F800-$F807   Global audio control
;   $F820-$F830   Filter parameters
;   $F900-$F93F   Square wave channel
;   $F940-$F97F   Triangle wave channel
;   $F980-$F9BF   Sine wave channel
;   $F9C0-$F9FF   Noise channel
;   $FA00-$FA0F   Sync sources (per channel)
;   $FA10-$FA1F   Ring mod sources (per channel)
;   $FA20-$FA5F   Sawtooth wave channel
;   $FA40-$FA5F   Effects (overdrive, reverb)
;   $FA80-$FB3F   Flexible synth channels (4 channels × $30 bytes each)
;
; Note: All 32-bit values are stored in LITTLE-ENDIAN byte order.
; ----------------------------------------------------------------------------

; ----------------------------------------------------------------------------
; Global Audio Control
; ----------------------------------------------------------------------------
AUDIO_CTRL_0    = $F800         ; Audio control byte 0 (IE32 $F0800)
AUDIO_CTRL_1    = $F801         ; Audio control byte 1
AUDIO_CTRL_2    = $F802         ; Audio control byte 2
AUDIO_CTRL_3    = $F803         ; Audio control byte 3
ENV_SHAPE_0     = $F804         ; Envelope shape byte 0 (IE32 $F0804)
ENV_SHAPE_1     = $F805         ; Envelope shape byte 1
ENV_SHAPE_2     = $F806         ; Envelope shape byte 2
ENV_SHAPE_3     = $F807         ; Envelope shape byte 3

; ----------------------------------------------------------------------------
; Global Filter
; ----------------------------------------------------------------------------
FILTER_CUTOFF_0   = $F820       ; Filter cutoff byte 0 (IE32 $F0820)
FILTER_CUTOFF_1   = $F821       ; Filter cutoff byte 1
FILTER_CUTOFF_2   = $F822       ; Filter cutoff byte 2
FILTER_CUTOFF_3   = $F823       ; Filter cutoff byte 3
FILTER_RESONANCE_0 = $F824      ; Filter resonance byte 0 (IE32 $F0824)
FILTER_RESONANCE_1 = $F825      ; Filter resonance byte 1
FILTER_RESONANCE_2 = $F826      ; Filter resonance byte 2
FILTER_RESONANCE_3 = $F827      ; Filter resonance byte 3
FILTER_TYPE_0     = $F828       ; Filter type byte 0 (IE32 $F0828)
FILTER_TYPE_1     = $F829       ; Filter type byte 1
FILTER_TYPE_2     = $F82A       ; Filter type byte 2
FILTER_TYPE_3     = $F82B       ; Filter type byte 3
FILTER_MOD_SRC_0  = $F82C       ; Filter mod source byte 0 (IE32 $F082C)
FILTER_MOD_SRC_1  = $F82D       ; Filter mod source byte 1
FILTER_MOD_SRC_2  = $F82E       ; Filter mod source byte 2
FILTER_MOD_SRC_3  = $F82F       ; Filter mod source byte 3
FILTER_MOD_AMT_0  = $F830       ; Filter mod amount byte 0 (IE32 $F0830)
FILTER_MOD_AMT_1  = $F831       ; Filter mod amount byte 1
FILTER_MOD_AMT_2  = $F832       ; Filter mod amount byte 2
FILTER_MOD_AMT_3  = $F833       ; Filter mod amount byte 3

; Filter type values
FILT_OFF        = 0             ; Filter disabled
FILT_LOWPASS    = 1             ; Low-pass filter
FILT_HIGHPASS   = 2             ; High-pass filter
FILT_BANDPASS   = 3             ; Band-pass filter

; ----------------------------------------------------------------------------
; Square Wave Channel ($F900-$F93F, maps from IE32 $F0900)
; ----------------------------------------------------------------------------
SQ_FREQ_0       = $F900         ; Square frequency byte 0
SQ_FREQ_1       = $F901         ; Square frequency byte 1
SQ_FREQ_2       = $F902         ; Square frequency byte 2
SQ_FREQ_3       = $F903         ; Square frequency byte 3
SQ_VOL_0        = $F904         ; Square volume byte 0
SQ_VOL_1        = $F905         ; Square volume byte 1
SQ_VOL_2        = $F906         ; Square volume byte 2
SQ_VOL_3        = $F907         ; Square volume byte 3
SQ_CTRL_0       = $F908         ; Square control byte 0
SQ_CTRL_1       = $F909         ; Square control byte 1
SQ_CTRL_2       = $F90A         ; Square control byte 2
SQ_CTRL_3       = $F90B         ; Square control byte 3
SQ_ATTACK_0     = $F90C         ; Square attack byte 0 (ms)
SQ_ATTACK_1     = $F90D         ; Square attack byte 1
SQ_DECAY_0      = $F910         ; Square decay byte 0 (ms)
SQ_DECAY_1      = $F911         ; Square decay byte 1
SQ_SUSTAIN_0    = $F914         ; Square sustain byte 0 (0-255 level)
SQ_SUSTAIN_1    = $F915         ; Square sustain byte 1
SQ_RELEASE_0    = $F918         ; Square release byte 0 (ms)
SQ_RELEASE_1    = $F919         ; Square release byte 1
SQ_DUTY_0       = $F91C         ; Square duty cycle byte 0 (0-65535)
SQ_DUTY_1       = $F91D         ; Square duty cycle byte 1
SQ_PWM_RATE_0   = $F920         ; Square PWM rate byte 0
SQ_PWM_RATE_1   = $F921         ; Square PWM rate byte 1
SQ_PWM_RATE_2   = $F922         ; Square PWM rate byte 2
SQ_PWM_RATE_3   = $F923         ; Square PWM rate byte 3
SQ_PWM_DEPTH_0  = $F924         ; Square PWM depth byte 0
SQ_PWM_DEPTH_1  = $F925         ; Square PWM depth byte 1
SQ_PWM_DEPTH_2  = $F926         ; Square PWM depth byte 2
SQ_PWM_DEPTH_3  = $F927         ; Square PWM depth byte 3
SQ_SWEEP_RATE_0 = $F928         ; Square sweep rate byte 0
SQ_SWEEP_RATE_1 = $F929         ; Square sweep rate byte 1
SQ_SWEEP_RATE_2 = $F92A         ; Square sweep rate byte 2
SQ_SWEEP_RATE_3 = $F92B         ; Square sweep rate byte 3
SQ_SWEEP_DIR_0  = $F92C         ; Square sweep direction byte 0
SQ_SWEEP_DIR_1  = $F92D         ; Square sweep direction byte 1
SQ_SWEEP_DIR_2  = $F92E         ; Square sweep direction byte 2
SQ_SWEEP_DIR_3  = $F92F         ; Square sweep direction byte 3
SQ_SWEEP_AMT_0  = $F930         ; Square sweep amount byte 0
SQ_SWEEP_AMT_1  = $F931         ; Square sweep amount byte 1
SQ_SWEEP_AMT_2  = $F932         ; Square sweep amount byte 2
SQ_SWEEP_AMT_3  = $F933         ; Square sweep amount byte 3
SQ_TARGET_0     = $F934         ; Square target freq byte 0
SQ_TARGET_1     = $F935         ; Square target freq byte 1
SQ_TARGET_2     = $F936         ; Square target freq byte 2
SQ_TARGET_3     = $F937         ; Square target freq byte 3

; ----------------------------------------------------------------------------
; Triangle Wave Channel ($F940-$F97F, maps from IE32 $F0940)
; ----------------------------------------------------------------------------
TRI_FREQ_0      = $F940         ; Triangle frequency byte 0
TRI_FREQ_1      = $F941         ; Triangle frequency byte 1
TRI_FREQ_2      = $F942         ; Triangle frequency byte 2
TRI_FREQ_3      = $F943         ; Triangle frequency byte 3
TRI_VOL_0       = $F944         ; Triangle volume byte 0
TRI_VOL_1       = $F945         ; Triangle volume byte 1
TRI_VOL_2       = $F946         ; Triangle volume byte 2
TRI_VOL_3       = $F947         ; Triangle volume byte 3
TRI_CTRL_0      = $F948         ; Triangle control byte 0
TRI_CTRL_1      = $F949         ; Triangle control byte 1
TRI_CTRL_2      = $F94A         ; Triangle control byte 2
TRI_CTRL_3      = $F94B         ; Triangle control byte 3
TRI_ATTACK_0    = $F94C         ; Triangle attack byte 0 (ms)
TRI_ATTACK_1    = $F94D         ; Triangle attack byte 1
TRI_DECAY_0     = $F950         ; Triangle decay byte 0 (ms)
TRI_DECAY_1     = $F951         ; Triangle decay byte 1
TRI_SUSTAIN_0   = $F954         ; Triangle sustain byte 0
TRI_SUSTAIN_1   = $F955         ; Triangle sustain byte 1
TRI_RELEASE_0   = $F958         ; Triangle release byte 0 (ms)
TRI_RELEASE_1   = $F959         ; Triangle release byte 1
TRI_SWEEP_RATE_0 = $F968        ; Triangle sweep rate byte 0
TRI_SWEEP_RATE_1 = $F969        ; Triangle sweep rate byte 1
TRI_SWEEP_RATE_2 = $F96A        ; Triangle sweep rate byte 2
TRI_SWEEP_RATE_3 = $F96B        ; Triangle sweep rate byte 3
TRI_SWEEP_DIR_0 = $F96C         ; Triangle sweep direction byte 0
TRI_SWEEP_DIR_1 = $F96D         ; Triangle sweep direction byte 1
TRI_SWEEP_DIR_2 = $F96E         ; Triangle sweep direction byte 2
TRI_SWEEP_DIR_3 = $F96F         ; Triangle sweep direction byte 3
TRI_SWEEP_AMT_0 = $F970         ; Triangle sweep amount byte 0
TRI_SWEEP_AMT_1 = $F971         ; Triangle sweep amount byte 1
TRI_SWEEP_AMT_2 = $F972         ; Triangle sweep amount byte 2
TRI_SWEEP_AMT_3 = $F973         ; Triangle sweep amount byte 3
TRI_TARGET_0    = $F974         ; Triangle target freq byte 0
TRI_TARGET_1    = $F975         ; Triangle target freq byte 1
TRI_TARGET_2    = $F976         ; Triangle target freq byte 2
TRI_TARGET_3    = $F977         ; Triangle target freq byte 3

; ----------------------------------------------------------------------------
; Sine Wave Channel ($F980-$F9BF, maps from IE32 $F0980)
; ----------------------------------------------------------------------------
SINE_FREQ_0     = $F980         ; Sine frequency byte 0
SINE_FREQ_1     = $F981         ; Sine frequency byte 1
SINE_FREQ_2     = $F982         ; Sine frequency byte 2
SINE_FREQ_3     = $F983         ; Sine frequency byte 3
SINE_VOL_0      = $F984         ; Sine volume byte 0
SINE_VOL_1      = $F985         ; Sine volume byte 1
SINE_VOL_2      = $F986         ; Sine volume byte 2
SINE_VOL_3      = $F987         ; Sine volume byte 3
SINE_CTRL_0     = $F988         ; Sine control byte 0
SINE_CTRL_1     = $F989         ; Sine control byte 1
SINE_CTRL_2     = $F98A         ; Sine control byte 2
SINE_CTRL_3     = $F98B         ; Sine control byte 3
SINE_ATTACK_0   = $F98C         ; Sine attack byte 0 (ms)
SINE_ATTACK_1   = $F98D         ; Sine attack byte 1
SINE_DECAY_0    = $F990         ; Sine decay byte 0 (ms)
SINE_DECAY_1    = $F991         ; Sine decay byte 1
SINE_SUSTAIN_0  = $F994         ; Sine sustain byte 0
SINE_SUSTAIN_1  = $F995         ; Sine sustain byte 1
SINE_RELEASE_0  = $F998         ; Sine release byte 0 (ms)
SINE_RELEASE_1  = $F999         ; Sine release byte 1
SINE_SWEEP_RATE_0 = $F9A8       ; Sine sweep rate byte 0
SINE_SWEEP_RATE_1 = $F9A9       ; Sine sweep rate byte 1
SINE_SWEEP_RATE_2 = $F9AA       ; Sine sweep rate byte 2
SINE_SWEEP_RATE_3 = $F9AB       ; Sine sweep rate byte 3
SINE_SWEEP_DIR_0 = $F9AC        ; Sine sweep direction byte 0
SINE_SWEEP_DIR_1 = $F9AD        ; Sine sweep direction byte 1
SINE_SWEEP_DIR_2 = $F9AE        ; Sine sweep direction byte 2
SINE_SWEEP_DIR_3 = $F9AF        ; Sine sweep direction byte 3
SINE_SWEEP_AMT_0 = $F9B0        ; Sine sweep amount byte 0
SINE_SWEEP_AMT_1 = $F9B1        ; Sine sweep amount byte 1
SINE_SWEEP_AMT_2 = $F9B2        ; Sine sweep amount byte 2
SINE_SWEEP_AMT_3 = $F9B3        ; Sine sweep amount byte 3
SINE_TARGET_0   = $F9B4         ; Sine target freq byte 0
SINE_TARGET_1   = $F9B5         ; Sine target freq byte 1
SINE_TARGET_2   = $F9B6         ; Sine target freq byte 2
SINE_TARGET_3   = $F9B7         ; Sine target freq byte 3

; ----------------------------------------------------------------------------
; Noise Channel ($F9C0-$F9FF, maps from IE32 $F09C0)
; ----------------------------------------------------------------------------
NOISE_FREQ_0    = $F9C0         ; Noise frequency byte 0
NOISE_FREQ_1    = $F9C1         ; Noise frequency byte 1
NOISE_FREQ_2    = $F9C2         ; Noise frequency byte 2
NOISE_FREQ_3    = $F9C3         ; Noise frequency byte 3
NOISE_VOL_0     = $F9C4         ; Noise volume byte 0
NOISE_VOL_1     = $F9C5         ; Noise volume byte 1
NOISE_VOL_2     = $F9C6         ; Noise volume byte 2
NOISE_VOL_3     = $F9C7         ; Noise volume byte 3
NOISE_CTRL_0    = $F9C8         ; Noise control byte 0
NOISE_CTRL_1    = $F9C9         ; Noise control byte 1
NOISE_CTRL_2    = $F9CA         ; Noise control byte 2
NOISE_CTRL_3    = $F9CB         ; Noise control byte 3
NOISE_ATTACK_0  = $F9CC         ; Noise attack byte 0 (ms)
NOISE_ATTACK_1  = $F9CD         ; Noise attack byte 1
NOISE_DECAY_0   = $F9D0         ; Noise decay byte 0 (ms)
NOISE_DECAY_1   = $F9D1         ; Noise decay byte 1
NOISE_SUSTAIN_0 = $F9D4         ; Noise sustain byte 0
NOISE_SUSTAIN_1 = $F9D5         ; Noise sustain byte 1
NOISE_RELEASE_0 = $F9D8         ; Noise release byte 0 (ms)
NOISE_RELEASE_1 = $F9D9         ; Noise release byte 1
NOISE_MODE_0    = $F9DC         ; Noise mode byte 0
NOISE_MODE_1    = $F9DD         ; Noise mode byte 1
NOISE_MODE_2    = $F9DE         ; Noise mode byte 2
NOISE_MODE_3    = $F9DF         ; Noise mode byte 3

; Noise mode values
NOISE_WHITE     = 0             ; White noise (random)
NOISE_PERIODIC  = 1             ; Periodic noise (regular pattern)
NOISE_METALLIC  = 2             ; Metallic noise (harsh)
NOISE_PSG       = 3             ; PSG-style noise (AY/SN76489)

; ----------------------------------------------------------------------------
; Sync and Ring Modulation Sources ($FA00-$FA1F, maps from IE32 $F0A00)
; ----------------------------------------------------------------------------
SYNC_SRC_CH0_0  = $FA00         ; Sync source channel 0 byte 0
SYNC_SRC_CH0_1  = $FA01         ; Sync source channel 0 byte 1
SYNC_SRC_CH0_2  = $FA02         ; Sync source channel 0 byte 2
SYNC_SRC_CH0_3  = $FA03         ; Sync source channel 0 byte 3
SYNC_SRC_CH1_0  = $FA04         ; Sync source channel 1 byte 0
SYNC_SRC_CH1_1  = $FA05         ; Sync source channel 1 byte 1
SYNC_SRC_CH1_2  = $FA06         ; Sync source channel 1 byte 2
SYNC_SRC_CH1_3  = $FA07         ; Sync source channel 1 byte 3
SYNC_SRC_CH2_0  = $FA08         ; Sync source channel 2 byte 0
SYNC_SRC_CH2_1  = $FA09         ; Sync source channel 2 byte 1
SYNC_SRC_CH2_2  = $FA0A         ; Sync source channel 2 byte 2
SYNC_SRC_CH2_3  = $FA0B         ; Sync source channel 2 byte 3
SYNC_SRC_CH3_0  = $FA0C         ; Sync source channel 3 byte 0
SYNC_SRC_CH3_1  = $FA0D         ; Sync source channel 3 byte 1
SYNC_SRC_CH3_2  = $FA0E         ; Sync source channel 3 byte 2
SYNC_SRC_CH3_3  = $FA0F         ; Sync source channel 3 byte 3

RING_SRC_CH0_0  = $FA10         ; Ring mod source channel 0 byte 0
RING_SRC_CH0_1  = $FA11         ; Ring mod source channel 0 byte 1
RING_SRC_CH0_2  = $FA12         ; Ring mod source channel 0 byte 2
RING_SRC_CH0_3  = $FA13         ; Ring mod source channel 0 byte 3
RING_SRC_CH1_0  = $FA14         ; Ring mod source channel 1 byte 0
RING_SRC_CH1_1  = $FA15         ; Ring mod source channel 1 byte 1
RING_SRC_CH1_2  = $FA16         ; Ring mod source channel 1 byte 2
RING_SRC_CH1_3  = $FA17         ; Ring mod source channel 1 byte 3
RING_SRC_CH2_0  = $FA18         ; Ring mod source channel 2 byte 0
RING_SRC_CH2_1  = $FA19         ; Ring mod source channel 2 byte 1
RING_SRC_CH2_2  = $FA1A         ; Ring mod source channel 2 byte 2
RING_SRC_CH2_3  = $FA1B         ; Ring mod source channel 2 byte 3
RING_SRC_CH3_0  = $FA1C         ; Ring mod source channel 3 byte 0
RING_SRC_CH3_1  = $FA1D         ; Ring mod source channel 3 byte 1
RING_SRC_CH3_2  = $FA1E         ; Ring mod source channel 3 byte 2
RING_SRC_CH3_3  = $FA1F         ; Ring mod source channel 3 byte 3

; ----------------------------------------------------------------------------
; Sawtooth Wave Channel ($FA20-$FA5F, maps from IE32 $F0A20)
; ----------------------------------------------------------------------------
SAW_FREQ_0      = $FA20         ; Sawtooth frequency byte 0
SAW_FREQ_1      = $FA21         ; Sawtooth frequency byte 1
SAW_FREQ_2      = $FA22         ; Sawtooth frequency byte 2
SAW_FREQ_3      = $FA23         ; Sawtooth frequency byte 3
SAW_VOL_0       = $FA24         ; Sawtooth volume byte 0
SAW_VOL_1       = $FA25         ; Sawtooth volume byte 1
SAW_VOL_2       = $FA26         ; Sawtooth volume byte 2
SAW_VOL_3       = $FA27         ; Sawtooth volume byte 3
SAW_CTRL_0      = $FA28         ; Sawtooth control byte 0
SAW_CTRL_1      = $FA29         ; Sawtooth control byte 1
SAW_CTRL_2      = $FA2A         ; Sawtooth control byte 2
SAW_CTRL_3      = $FA2B         ; Sawtooth control byte 3
SAW_ATTACK_0    = $FA2C         ; Sawtooth attack byte 0 (ms)
SAW_ATTACK_1    = $FA2D         ; Sawtooth attack byte 1
SAW_DECAY_0     = $FA30         ; Sawtooth decay byte 0 (ms)
SAW_DECAY_1     = $FA31         ; Sawtooth decay byte 1
SAW_SUSTAIN_0   = $FA34         ; Sawtooth sustain byte 0
SAW_SUSTAIN_1   = $FA35         ; Sawtooth sustain byte 1
SAW_RELEASE_0   = $FA38         ; Sawtooth release byte 0 (ms)
SAW_RELEASE_1   = $FA39         ; Sawtooth release byte 1
SAW_SWEEP_RATE_0 = $FA48        ; Sawtooth sweep rate byte 0
SAW_SWEEP_RATE_1 = $FA49        ; Sawtooth sweep rate byte 1
SAW_SWEEP_RATE_2 = $FA4A        ; Sawtooth sweep rate byte 2
SAW_SWEEP_RATE_3 = $FA4B        ; Sawtooth sweep rate byte 3
SAW_SWEEP_DIR_0 = $FA4C         ; Sawtooth sweep direction byte 0
SAW_SWEEP_DIR_1 = $FA4D         ; Sawtooth sweep direction byte 1
SAW_SWEEP_DIR_2 = $FA4E         ; Sawtooth sweep direction byte 2
SAW_SWEEP_DIR_3 = $FA4F         ; Sawtooth sweep direction byte 3
SAW_SWEEP_AMT_0 = $FA50         ; Sawtooth sweep amount byte 0
SAW_SWEEP_AMT_1 = $FA51         ; Sawtooth sweep amount byte 1
SAW_SWEEP_AMT_2 = $FA52         ; Sawtooth sweep amount byte 2
SAW_SWEEP_AMT_3 = $FA53         ; Sawtooth sweep amount byte 3
SAW_TARGET_0    = $FA54         ; Sawtooth target freq byte 0
SAW_TARGET_1    = $FA55         ; Sawtooth target freq byte 1
SAW_TARGET_2    = $FA56         ; Sawtooth target freq byte 2
SAW_TARGET_3    = $FA57         ; Sawtooth target freq byte 3

; ----------------------------------------------------------------------------
; Effects ($FA40-$FA5F, maps from IE32 $F0A40)
; Note: Overlaps with sawtooth sweep - sawtooth sweep addresses above are
; the actual mappings; these effect registers share space
; ----------------------------------------------------------------------------
OVERDRIVE_CTRL_0 = $FA40        ; Overdrive control byte 0 (0=off, 1-255=amount)
OVERDRIVE_CTRL_1 = $FA41        ; Overdrive control byte 1
OVERDRIVE_CTRL_2 = $FA42        ; Overdrive control byte 2
OVERDRIVE_CTRL_3 = $FA43        ; Overdrive control byte 3

REVERB_MIX_0    = $FA50         ; Reverb wet/dry mix byte 0 (0-255)
REVERB_MIX_1    = $FA51         ; Reverb wet/dry mix byte 1
REVERB_MIX_2    = $FA52         ; Reverb wet/dry mix byte 2
REVERB_MIX_3    = $FA53         ; Reverb wet/dry mix byte 3
REVERB_DECAY_0  = $FA54         ; Reverb decay time byte 0 (0-65535 ms)
REVERB_DECAY_1  = $FA55         ; Reverb decay time byte 1
REVERB_DECAY_2  = $FA56         ; Reverb decay time byte 2
REVERB_DECAY_3  = $FA57         ; Reverb decay time byte 3

; ----------------------------------------------------------------------------
; Flexible Synth Channels ($FA80-$FB7F, maps from IE32 $F0A80)
;
; 4 flexible channels, each with $40 (64) bytes of registers.
; Each channel can be set to any waveform type and has full ADSR + effects.
;
; Channel base addresses:
;   Flex 0: $FA80
;   Flex 1: $FAC0
;   Flex 2: $FB00
;   Flex 3: $FB40
; ----------------------------------------------------------------------------
FLEX_CH_STRIDE  = $40           ; Bytes per flexible channel (64)

; Flex Channel 0 ($FA80-$FAAF)
; Frequency is 16.8 fixed-point: value = Hz * 256 (e.g., 440 Hz = 440*256 = 112640)
FLEX_CH0_FREQ_0 = $FA80         ; Flex 0 frequency byte 0 (low byte)
FLEX_CH0_FREQ_1 = $FA81         ; Flex 0 frequency byte 1
FLEX_CH0_FREQ_2 = $FA82         ; Flex 0 frequency byte 2
FLEX_CH0_FREQ_3 = $FA83         ; Flex 0 frequency byte 3 (high byte)
FLEX_CH0_VOL_0  = $FA84         ; Flex 0 volume byte 0
FLEX_CH0_VOL_1  = $FA85         ; Flex 0 volume byte 1
FLEX_CH0_VOL_2  = $FA86         ; Flex 0 volume byte 2
FLEX_CH0_VOL_3  = $FA87         ; Flex 0 volume byte 3
FLEX_CH0_WAVE_0 = $FA88         ; Flex 0 waveform byte 0
FLEX_CH0_WAVE_1 = $FA89         ; Flex 0 waveform byte 1
FLEX_CH0_WAVE_2 = $FA8A         ; Flex 0 waveform byte 2
FLEX_CH0_WAVE_3 = $FA8B         ; Flex 0 waveform byte 3
FLEX_CH0_CTRL_0 = $FA8C         ; Flex 0 control byte 0
FLEX_CH0_CTRL_1 = $FA8D         ; Flex 0 control byte 1
FLEX_CH0_CTRL_2 = $FA8E         ; Flex 0 control byte 2
FLEX_CH0_CTRL_3 = $FA8F         ; Flex 0 control byte 3
FLEX_CH0_ATK_0  = $FA90         ; Flex 0 attack byte 0 (ms)
FLEX_CH0_ATK_1  = $FA91         ; Flex 0 attack byte 1
FLEX_CH0_DEC_0  = $FA94         ; Flex 0 decay byte 0 (ms)
FLEX_CH0_DEC_1  = $FA95         ; Flex 0 decay byte 1
FLEX_CH0_SUS_0  = $FA98         ; Flex 0 sustain byte 0
FLEX_CH0_SUS_1  = $FA99         ; Flex 0 sustain byte 1
FLEX_CH0_REL_0  = $FA9C         ; Flex 0 release byte 0 (ms)
FLEX_CH0_REL_1  = $FA9D         ; Flex 0 release byte 1
FLEX_CH0_DUTY_0 = $FAA0         ; Flex 0 duty cycle byte 0
FLEX_CH0_DUTY_1 = $FAA1         ; Flex 0 duty cycle byte 1
FLEX_CH0_PAN_0  = $FAA4         ; Flex 0 pan byte 0 (-128 to 127)
FLEX_CH0_PAN_1  = $FAA5         ; Flex 0 pan byte 1
FLEX_CH0_PAN_2  = $FAA6         ; Flex 0 pan byte 2
FLEX_CH0_PAN_3  = $FAA7         ; Flex 0 pan byte 3

; Flex Channel 1 ($FAC0-$FAFF)
FLEX_CH1_FREQ_0 = $FAC0         ; Flex 1 frequency byte 0
FLEX_CH1_FREQ_1 = $FAC1         ; Flex 1 frequency byte 1
FLEX_CH1_FREQ_2 = $FAC2         ; Flex 1 frequency byte 2
FLEX_CH1_FREQ_3 = $FAC3         ; Flex 1 frequency byte 3
FLEX_CH1_VOL_0  = $FAC4         ; Flex 1 volume byte 0
FLEX_CH1_VOL_1  = $FAC5         ; Flex 1 volume byte 1
FLEX_CH1_VOL_2  = $FAC6         ; Flex 1 volume byte 2
FLEX_CH1_VOL_3  = $FAC7         ; Flex 1 volume byte 3
FLEX_CH1_WAVE_0 = $FAC8         ; Flex 1 waveform byte 0
FLEX_CH1_WAVE_1 = $FAC9         ; Flex 1 waveform byte 1
FLEX_CH1_WAVE_2 = $FACA         ; Flex 1 waveform byte 2
FLEX_CH1_WAVE_3 = $FACB         ; Flex 1 waveform byte 3
FLEX_CH1_CTRL_0 = $FACC         ; Flex 1 control byte 0
FLEX_CH1_CTRL_1 = $FACD         ; Flex 1 control byte 1
FLEX_CH1_CTRL_2 = $FACE         ; Flex 1 control byte 2
FLEX_CH1_CTRL_3 = $FACF         ; Flex 1 control byte 3
FLEX_CH1_ATK_0  = $FAD0         ; Flex 1 attack byte 0 (ms)
FLEX_CH1_ATK_1  = $FAD1         ; Flex 1 attack byte 1
FLEX_CH1_DEC_0  = $FAD4         ; Flex 1 decay byte 0 (ms)
FLEX_CH1_DEC_1  = $FAD5         ; Flex 1 decay byte 1
FLEX_CH1_SUS_0  = $FAD8         ; Flex 1 sustain byte 0
FLEX_CH1_SUS_1  = $FAD9         ; Flex 1 sustain byte 1
FLEX_CH1_REL_0  = $FADC         ; Flex 1 release byte 0 (ms)
FLEX_CH1_REL_1  = $FADD         ; Flex 1 release byte 1
FLEX_CH1_DUTY_0 = $FAE0         ; Flex 1 duty cycle byte 0
FLEX_CH1_DUTY_1 = $FAE1         ; Flex 1 duty cycle byte 1
FLEX_CH1_PAN_0  = $FAE4         ; Flex 1 pan byte 0
FLEX_CH1_PAN_1  = $FAE5         ; Flex 1 pan byte 1
FLEX_CH1_PAN_2  = $FAE6         ; Flex 1 pan byte 2
FLEX_CH1_PAN_3  = $FAE7         ; Flex 1 pan byte 3

; Flex Channel 2 ($FB00-$FB3F)
FLEX_CH2_FREQ_0 = $FB00         ; Flex 2 frequency byte 0
FLEX_CH2_FREQ_1 = $FB01         ; Flex 2 frequency byte 1
FLEX_CH2_FREQ_2 = $FB02         ; Flex 2 frequency byte 2
FLEX_CH2_FREQ_3 = $FB03         ; Flex 2 frequency byte 3
FLEX_CH2_VOL_0  = $FB04         ; Flex 2 volume byte 0
FLEX_CH2_VOL_1  = $FB05         ; Flex 2 volume byte 1
FLEX_CH2_VOL_2  = $FB06         ; Flex 2 volume byte 2
FLEX_CH2_VOL_3  = $FB07         ; Flex 2 volume byte 3
FLEX_CH2_WAVE_0 = $FB08         ; Flex 2 waveform byte 0
FLEX_CH2_WAVE_1 = $FB09         ; Flex 2 waveform byte 1
FLEX_CH2_WAVE_2 = $FB0A         ; Flex 2 waveform byte 2
FLEX_CH2_WAVE_3 = $FB0B         ; Flex 2 waveform byte 3
FLEX_CH2_CTRL_0 = $FB0C         ; Flex 2 control byte 0
FLEX_CH2_CTRL_1 = $FB0D         ; Flex 2 control byte 1
FLEX_CH2_CTRL_2 = $FB0E         ; Flex 2 control byte 2
FLEX_CH2_CTRL_3 = $FB0F         ; Flex 2 control byte 3
FLEX_CH2_ATK_0  = $FB10         ; Flex 2 attack byte 0 (ms)
FLEX_CH2_ATK_1  = $FB11         ; Flex 2 attack byte 1
FLEX_CH2_DEC_0  = $FB14         ; Flex 2 decay byte 0 (ms)
FLEX_CH2_DEC_1  = $FB15         ; Flex 2 decay byte 1
FLEX_CH2_SUS_0  = $FB18         ; Flex 2 sustain byte 0
FLEX_CH2_SUS_1  = $FB19         ; Flex 2 sustain byte 1
FLEX_CH2_REL_0  = $FB1C         ; Flex 2 release byte 0 (ms)
FLEX_CH2_REL_1  = $FB1D         ; Flex 2 release byte 1
FLEX_CH2_DUTY_0 = $FB20         ; Flex 2 duty cycle byte 0
FLEX_CH2_DUTY_1 = $FB21         ; Flex 2 duty cycle byte 1
FLEX_CH2_PAN_0  = $FB24         ; Flex 2 pan byte 0
FLEX_CH2_PAN_1  = $FB25         ; Flex 2 pan byte 1
FLEX_CH2_PAN_2  = $FB26         ; Flex 2 pan byte 2
FLEX_CH2_PAN_3  = $FB27         ; Flex 2 pan byte 3

; Flex Channel 3 ($FB40-$FB7F)
FLEX_CH3_FREQ_0 = $FB40         ; Flex 3 frequency byte 0
FLEX_CH3_FREQ_1 = $FB41         ; Flex 3 frequency byte 1
FLEX_CH3_FREQ_2 = $FB42         ; Flex 3 frequency byte 2
FLEX_CH3_FREQ_3 = $FB43         ; Flex 3 frequency byte 3
FLEX_CH3_VOL_0  = $FB44         ; Flex 3 volume byte 0
FLEX_CH3_VOL_1  = $FB45         ; Flex 3 volume byte 1
FLEX_CH3_VOL_2  = $FB46         ; Flex 3 volume byte 2
FLEX_CH3_VOL_3  = $FB47         ; Flex 3 volume byte 3
FLEX_CH3_WAVE_0 = $FB48         ; Flex 3 waveform byte 0
FLEX_CH3_WAVE_1 = $FB49         ; Flex 3 waveform byte 1
FLEX_CH3_WAVE_2 = $FB4A         ; Flex 3 waveform byte 2
FLEX_CH3_WAVE_3 = $FB4B         ; Flex 3 waveform byte 3
FLEX_CH3_CTRL_0 = $FB4C         ; Flex 3 control byte 0
FLEX_CH3_CTRL_1 = $FB4D         ; Flex 3 control byte 1
FLEX_CH3_CTRL_2 = $FB4E         ; Flex 3 control byte 2
FLEX_CH3_CTRL_3 = $FB4F         ; Flex 3 control byte 3
FLEX_CH3_ATK_0  = $FB50         ; Flex 3 attack byte 0 (ms)
FLEX_CH3_ATK_1  = $FB51         ; Flex 3 attack byte 1
FLEX_CH3_DEC_0  = $FB54         ; Flex 3 decay byte 0 (ms)
FLEX_CH3_DEC_1  = $FB55         ; Flex 3 decay byte 1
FLEX_CH3_SUS_0  = $FB58         ; Flex 3 sustain byte 0
FLEX_CH3_SUS_1  = $FB59         ; Flex 3 sustain byte 1
FLEX_CH3_REL_0  = $FB5C         ; Flex 3 release byte 0 (ms)
FLEX_CH3_REL_1  = $FB5D         ; Flex 3 release byte 1
FLEX_CH3_DUTY_0 = $FB60         ; Flex 3 duty cycle byte 0
FLEX_CH3_DUTY_1 = $FB61         ; Flex 3 duty cycle byte 1
FLEX_CH3_PAN_0  = $FB64         ; Flex 3 pan byte 0
FLEX_CH3_PAN_1  = $FB65         ; Flex 3 pan byte 1
FLEX_CH3_PAN_2  = $FB66         ; Flex 3 pan byte 2
FLEX_CH3_PAN_3  = $FB67         ; Flex 3 pan byte 3

; ----------------------------------------------------------------------------
; Waveform Types (for flexible channels)
; ----------------------------------------------------------------------------
WAVE_SQUARE     = 0             ; Square/pulse wave
WAVE_TRIANGLE   = 1             ; Triangle wave
WAVE_SINE       = 2             ; Sine wave
WAVE_NOISE      = 3             ; Noise
WAVE_SAWTOOTH   = 4             ; Sawtooth wave

; ----------------------------------------------------------------------------
; Channel Control Bits (for SQ_CTRL, TRI_CTRL, etc.)
; ----------------------------------------------------------------------------
CTRL_GATE       = $01           ; Gate on (bit 0) - trigger envelope
CTRL_PWM_EN     = $02           ; PWM enable (bit 1)
CTRL_SWEEP_EN   = $04           ; Sweep enable (bit 2)
CTRL_SYNC_EN    = $08           ; Hard sync enable (bit 3)
CTRL_RING_EN    = $10           ; Ring modulation enable (bit 4)
CTRL_FILTER_EN  = $20           ; Route through filter (bit 5)

; Sweep direction values
SWEEP_DOWN      = 0             ; Sweep frequency down
SWEEP_UP        = 1             ; Sweep frequency up

; ============================================================================
; VIDEO CONSTANTS
; ============================================================================

VRAM_START      = $100000       ; VRAM starts at 1MB in 32-bit space
SCREEN_W        = 640           ; Default screen width
SCREEN_H        = 480           ; Default screen height
LINE_BYTES      = 2560          ; Bytes per scanline (640 * 4)

; Blitter operations
BLT_OP_COPY     = 0             ; Copy source to dest
BLT_OP_FILL     = 1             ; Fill rect with color
BLT_OP_LINE     = 2             ; Draw line
BLT_OP_MASKED   = 3             ; Copy with mask
BLT_OP_ALPHA    = 4             ; Alpha blending

; Background color (black with alpha)
BACKGROUND_0    = $00           ; Blue
BACKGROUND_1    = $00           ; Green
BACKGROUND_2    = $00           ; Red
BACKGROUND_3    = $FF           ; Alpha

; ============================================================================
; COPPER OPCODES
; ============================================================================

; Copper WAIT opcode: wait for raster position
; Format: $0YYY_HHHH where YYY = Y position (0-4095), HHHH = H position
; Usage: COP_WAIT(y) = y * COP_WAIT_SCALE
COP_WAIT_MASK   = $00000000
COP_WAIT_SCALE  = $1000                     ; Multiply Y position by this for WAIT

; Copper MOVE opcodes
COP_MOVE_RASTER_Y     = $40120000
COP_MOVE_RASTER_H     = $40130000
COP_MOVE_RASTER_COLOR = $40140000
COP_MOVE_RASTER_CTRL  = $40150000

; Copper END opcode
COP_END         = $C0000000

; Copper SETBASE opcode - set I/O base for subsequent MOVE operations
; Format: [10][6-bit unused][24-bit base >> 2]
; Addresses are 4-byte aligned, so we encode (addr >> 2) in 24 bits
COP_SETBASE_OP        = $80000000
COP_SETBASE_SHIFT     = 2

; Pre-computed SETBASE instructions for common devices
COP_SETBASE_VIDEO     = $8003C000           ; VIDEO_REG_BASE ($F0000 >> 2)
COP_SETBASE_VGA       = $8003C400           ; VGA_BASE ($F1000 >> 2)
COP_SETBASE_VGA_DAC   = $8003C416           ; VGA_DAC_WINDEX ($F1058 >> 2)

; MOVE register indices after SETBASE to VGA_DAC_WINDEX
COP_MOVE_VGA_WINDEX   = $40000000           ; Offset 0: DAC write index
COP_MOVE_VGA_DATA     = $40010000           ; Offset 1: DAC data (R,G,B seq)

; ============================================================================
; MACROS
; ============================================================================

; Set bank 1 (sprite data) - 16-bit bank number
.macro SET_BANK1 bank
    lda #<(bank)
    sta BANK1_REG_LO
    lda #>(bank)
    sta BANK1_REG_HI
.endmacro

; Set bank 2 (font data) - 16-bit bank number
.macro SET_BANK2 bank
    lda #<(bank)
    sta BANK2_REG_LO
    lda #>(bank)
    sta BANK2_REG_HI
.endmacro

; Set bank 3 (general data) - 16-bit bank number
.macro SET_BANK3 bank
    lda #<(bank)
    sta BANK3_REG_LO
    lda #>(bank)
    sta BANK3_REG_HI
.endmacro

; Set VRAM bank
.macro SET_VRAM_BANK bank
    lda #bank
    sta VRAM_BANK_REG
.endmacro

; Store 16-bit value to address
.macro STORE16 addr, value
    lda #<(value)
    sta addr
    lda #>(value)
    sta addr+1
.endmacro

; Store 32-bit value to I/O register (little-endian)
.macro STORE32 addr, value
    lda #<(value)
    sta addr
    lda #>(value)
    sta addr+1
    lda #^(value)
    sta addr+2
    lda #.lobyte(.hiword(value) >> 8)
    sta addr+3
.endmacro

; Store 32-bit value from zero page pointers
.macro STORE32_ZP addr, zp_ptr
    ldy #0
    lda (zp_ptr),y
    sta addr
    iny
    lda (zp_ptr),y
    sta addr+1
    iny
    lda (zp_ptr),y
    sta addr+2
    iny
    lda (zp_ptr),y
    sta addr+3
.endmacro

; Wait for VBlank
.macro WAIT_VBLANK
:   lda VIDEO_STATUS
    and #STATUS_VBLANK
    beq :-
.endmacro

; Wait for blitter to finish
.macro WAIT_BLIT
:   lda BLT_CTRL
    and #2
    bne :-
.endmacro

; Start a blitter operation (write 1 to control)
.macro START_BLIT
    lda #1
    sta BLT_CTRL
.endmacro

; Set blitter operation type
.macro SET_BLT_OP op
    lda #op
    sta BLT_OP
.endmacro

; Set blitter width (16-bit)
.macro SET_BLT_WIDTH width
    lda #<(width)
    sta BLT_WIDTH_LO
    lda #>(width)
    sta BLT_WIDTH_HI
.endmacro

; Set blitter height (16-bit)
.macro SET_BLT_HEIGHT height
    lda #<(height)
    sta BLT_HEIGHT_LO
    lda #>(height)
    sta BLT_HEIGHT_HI
.endmacro

; Set source stride (16-bit)
.macro SET_SRC_STRIDE stride
    lda #<(stride)
    sta BLT_SRC_STRIDE_LO
    lda #>(stride)
    sta BLT_SRC_STRIDE_HI
.endmacro

; Set dest stride (16-bit)
.macro SET_DST_STRIDE stride
    lda #<(stride)
    sta BLT_DST_STRIDE_LO
    lda #>(stride)
    sta BLT_DST_STRIDE_HI
.endmacro

; Set blitter fill color (32-bit BGRA)
.macro SET_BLT_COLOR color
    lda #<(color)
    sta BLT_COLOR_0
    lda #>(color)
    sta BLT_COLOR_1
    lda #^(color)
    sta BLT_COLOR_2
    lda #.lobyte(.hiword(color) >> 8)
    sta BLT_COLOR_3
.endmacro

; Add 16-bit value to zero page 16-bit variable
.macro ADD16 zp_var, value
    clc
    lda zp_var
    adc #<(value)
    sta zp_var
    lda zp_var+1
    adc #>(value)
    sta zp_var+1
.endmacro

; ============================================================================
; CUSTOM AUDIO CHIP MACROS
; ============================================================================

; Set square channel frequency (16.8 fixed-point Hz, macro handles *256)
.macro SET_SQ_FREQ freq
    ; freq * 256: byte 0 = 0, bytes 1-2 = freq, byte 3 = 0
    lda #0
    sta SQ_FREQ_0
    lda #<(freq)
    sta SQ_FREQ_1
    lda #>(freq)
    sta SQ_FREQ_2
    lda #0
    sta SQ_FREQ_3
.endmacro

; Set square channel volume (8-bit, 0-255)
.macro SET_SQ_VOL vol
    lda #vol
    sta SQ_VOL_0
    lda #0
    sta SQ_VOL_1
    sta SQ_VOL_2
    sta SQ_VOL_3
.endmacro

; Set square channel duty cycle (16-bit, 0-65535; 32768 = 50%)
.macro SET_SQ_DUTY duty
    lda #<(duty)
    sta SQ_DUTY_0
    lda #>(duty)
    sta SQ_DUTY_1
.endmacro

; Set square channel ADSR (all 16-bit ms values except sustain which is 0-255)
.macro SET_SQ_ADSR atk, dec, sus, rel
    lda #<(atk)
    sta SQ_ATTACK_0
    lda #>(atk)
    sta SQ_ATTACK_1
    lda #<(dec)
    sta SQ_DECAY_0
    lda #>(dec)
    sta SQ_DECAY_1
    lda #<(sus)
    sta SQ_SUSTAIN_0
    lda #>(sus)
    sta SQ_SUSTAIN_1
    lda #<(rel)
    sta SQ_RELEASE_0
    lda #>(rel)
    sta SQ_RELEASE_1
.endmacro

; Gate square channel on (trigger envelope)
.macro GATE_SQ_ON
    lda SQ_CTRL_0
    ora #CTRL_GATE
    sta SQ_CTRL_0
.endmacro

; Gate square channel off (release envelope)
.macro GATE_SQ_OFF
    lda SQ_CTRL_0
    and #<(~CTRL_GATE)
    sta SQ_CTRL_0
.endmacro

; Set triangle channel frequency (16.8 fixed-point Hz, macro handles *256)
.macro SET_TRI_FREQ freq
    ; freq * 256: byte 0 = 0, bytes 1-2 = freq, byte 3 = 0
    lda #0
    sta TRI_FREQ_0
    lda #<(freq)
    sta TRI_FREQ_1
    lda #>(freq)
    sta TRI_FREQ_2
    lda #0
    sta TRI_FREQ_3
.endmacro

; Set triangle channel volume
.macro SET_TRI_VOL vol
    lda #vol
    sta TRI_VOL_0
    lda #0
    sta TRI_VOL_1
    sta TRI_VOL_2
    sta TRI_VOL_3
.endmacro

; Set triangle channel ADSR
.macro SET_TRI_ADSR atk, dec, sus, rel
    lda #<(atk)
    sta TRI_ATTACK_0
    lda #>(atk)
    sta TRI_ATTACK_1
    lda #<(dec)
    sta TRI_DECAY_0
    lda #>(dec)
    sta TRI_DECAY_1
    lda #<(sus)
    sta TRI_SUSTAIN_0
    lda #>(sus)
    sta TRI_SUSTAIN_1
    lda #<(rel)
    sta TRI_RELEASE_0
    lda #>(rel)
    sta TRI_RELEASE_1
.endmacro

; Gate triangle channel on
.macro GATE_TRI_ON
    lda TRI_CTRL_0
    ora #CTRL_GATE
    sta TRI_CTRL_0
.endmacro

; Gate triangle channel off
.macro GATE_TRI_OFF
    lda TRI_CTRL_0
    and #<(~CTRL_GATE)
    sta TRI_CTRL_0
.endmacro

; Set sine channel frequency (16.8 fixed-point Hz, macro handles *256)
.macro SET_SINE_FREQ freq
    ; freq * 256: byte 0 = 0, bytes 1-2 = freq, byte 3 = 0
    lda #0
    sta SINE_FREQ_0
    lda #<(freq)
    sta SINE_FREQ_1
    lda #>(freq)
    sta SINE_FREQ_2
    lda #0
    sta SINE_FREQ_3
.endmacro

; Set sine channel volume
.macro SET_SINE_VOL vol
    lda #vol
    sta SINE_VOL_0
    lda #0
    sta SINE_VOL_1
    sta SINE_VOL_2
    sta SINE_VOL_3
.endmacro

; Set sine channel ADSR
.macro SET_SINE_ADSR atk, dec, sus, rel
    lda #<(atk)
    sta SINE_ATTACK_0
    lda #>(atk)
    sta SINE_ATTACK_1
    lda #<(dec)
    sta SINE_DECAY_0
    lda #>(dec)
    sta SINE_DECAY_1
    lda #<(sus)
    sta SINE_SUSTAIN_0
    lda #>(sus)
    sta SINE_SUSTAIN_1
    lda #<(rel)
    sta SINE_RELEASE_0
    lda #>(rel)
    sta SINE_RELEASE_1
.endmacro

; Gate sine channel on
.macro GATE_SINE_ON
    lda SINE_CTRL_0
    ora #CTRL_GATE
    sta SINE_CTRL_0
.endmacro

; Gate sine channel off
.macro GATE_SINE_OFF
    lda SINE_CTRL_0
    and #<(~CTRL_GATE)
    sta SINE_CTRL_0
.endmacro

; Set noise channel frequency (16.8 fixed-point Hz, macro handles *256)
.macro SET_NOISE_FREQ freq
    ; freq * 256: byte 0 = 0, bytes 1-2 = freq, byte 3 = 0
    lda #0
    sta NOISE_FREQ_0
    lda #<(freq)
    sta NOISE_FREQ_1
    lda #>(freq)
    sta NOISE_FREQ_2
    lda #0
    sta NOISE_FREQ_3
.endmacro

; Set noise channel volume
.macro SET_NOISE_VOL vol
    lda #vol
    sta NOISE_VOL_0
    lda #0
    sta NOISE_VOL_1
    sta NOISE_VOL_2
    sta NOISE_VOL_3
.endmacro

; Set noise mode
.macro SET_NOISE_MODE mode
    lda #mode
    sta NOISE_MODE_0
    lda #0
    sta NOISE_MODE_1
    sta NOISE_MODE_2
    sta NOISE_MODE_3
.endmacro

; Set noise channel ADSR
.macro SET_NOISE_ADSR atk, dec, sus, rel
    lda #<(atk)
    sta NOISE_ATTACK_0
    lda #>(atk)
    sta NOISE_ATTACK_1
    lda #<(dec)
    sta NOISE_DECAY_0
    lda #>(dec)
    sta NOISE_DECAY_1
    lda #<(sus)
    sta NOISE_SUSTAIN_0
    lda #>(sus)
    sta NOISE_SUSTAIN_1
    lda #<(rel)
    sta NOISE_RELEASE_0
    lda #>(rel)
    sta NOISE_RELEASE_1
.endmacro

; Gate noise channel on
.macro GATE_NOISE_ON
    lda NOISE_CTRL_0
    ora #CTRL_GATE
    sta NOISE_CTRL_0
.endmacro

; Gate noise channel off
.macro GATE_NOISE_OFF
    lda NOISE_CTRL_0
    and #<(~CTRL_GATE)
    sta NOISE_CTRL_0
.endmacro

; Set sawtooth channel frequency (16.8 fixed-point Hz, macro handles *256)
.macro SET_SAW_FREQ freq
    ; freq * 256: byte 0 = 0, bytes 1-2 = freq, byte 3 = 0
    lda #0
    sta SAW_FREQ_0
    lda #<(freq)
    sta SAW_FREQ_1
    lda #>(freq)
    sta SAW_FREQ_2
    lda #0
    sta SAW_FREQ_3
.endmacro

; Set sawtooth channel volume
.macro SET_SAW_VOL vol
    lda #vol
    sta SAW_VOL_0
    lda #0
    sta SAW_VOL_1
    sta SAW_VOL_2
    sta SAW_VOL_3
.endmacro

; Set sawtooth channel ADSR
.macro SET_SAW_ADSR atk, dec, sus, rel
    lda #<(atk)
    sta SAW_ATTACK_0
    lda #>(atk)
    sta SAW_ATTACK_1
    lda #<(dec)
    sta SAW_DECAY_0
    lda #>(dec)
    sta SAW_DECAY_1
    lda #<(sus)
    sta SAW_SUSTAIN_0
    lda #>(sus)
    sta SAW_SUSTAIN_1
    lda #<(rel)
    sta SAW_RELEASE_0
    lda #>(rel)
    sta SAW_RELEASE_1
.endmacro

; Gate sawtooth channel on
.macro GATE_SAW_ON
    lda SAW_CTRL_0
    ora #CTRL_GATE
    sta SAW_CTRL_0
.endmacro

; Gate sawtooth channel off
.macro GATE_SAW_OFF
    lda SAW_CTRL_0
    and #<(~CTRL_GATE)
    sta SAW_CTRL_0
.endmacro

; Set global filter parameters
; type: FILT_OFF, FILT_LOWPASS, FILT_HIGHPASS, FILT_BANDPASS
; cutoff: 0-65535 (maps to 20Hz-20kHz)
; resonance: 0-255
.macro SET_FILTER type, cutoff, resonance
    lda #type
    sta FILTER_TYPE_0
    lda #0
    sta FILTER_TYPE_1
    sta FILTER_TYPE_2
    sta FILTER_TYPE_3
    lda #<(cutoff)
    sta FILTER_CUTOFF_0
    lda #>(cutoff)
    sta FILTER_CUTOFF_1
    lda #0
    sta FILTER_CUTOFF_2
    sta FILTER_CUTOFF_3
    lda #resonance
    sta FILTER_RESONANCE_0
    lda #0
    sta FILTER_RESONANCE_1
    sta FILTER_RESONANCE_2
    sta FILTER_RESONANCE_3
.endmacro

; Enable filter on square channel
.macro FILTER_SQ_ON
    lda SQ_CTRL_0
    ora #CTRL_FILTER_EN
    sta SQ_CTRL_0
.endmacro

; Disable filter on square channel
.macro FILTER_SQ_OFF
    lda SQ_CTRL_0
    and #<(~CTRL_FILTER_EN)
    sta SQ_CTRL_0
.endmacro

; Set reverb parameters
; mix: 0-255 (wet/dry ratio)
; decay: 0-65535 (decay time in ms)
.macro SET_REVERB mix, decay
    lda #mix
    sta REVERB_MIX_0
    lda #0
    sta REVERB_MIX_1
    sta REVERB_MIX_2
    sta REVERB_MIX_3
    lda #<(decay)
    sta REVERB_DECAY_0
    lda #>(decay)
    sta REVERB_DECAY_1
    lda #0
    sta REVERB_DECAY_2
    sta REVERB_DECAY_3
.endmacro

; Set overdrive amount (0=off, 1-255=distortion amount)
.macro SET_OVERDRIVE amount
    lda #amount
    sta OVERDRIVE_CTRL_0
    lda #0
    sta OVERDRIVE_CTRL_1
    sta OVERDRIVE_CTRL_2
    sta OVERDRIVE_CTRL_3
.endmacro

; Set flexible channel 0 parameters
; Frequency is 16.8 fixed-point (Hz * 256), macro handles conversion
.macro SET_FLEX0 wave, freq, vol
    lda #wave
    sta FLEX_CH0_WAVE_0
    lda #0
    sta FLEX_CH0_WAVE_1
    sta FLEX_CH0_WAVE_2
    sta FLEX_CH0_WAVE_3
    ; freq * 256: byte 0 = 0, bytes 1-2 = freq, byte 3 = 0
    lda #0
    sta FLEX_CH0_FREQ_0
    lda #<(freq)
    sta FLEX_CH0_FREQ_1
    lda #>(freq)
    sta FLEX_CH0_FREQ_2
    lda #0
    sta FLEX_CH0_FREQ_3
    lda #vol
    sta FLEX_CH0_VOL_0
    lda #0
    sta FLEX_CH0_VOL_1
    sta FLEX_CH0_VOL_2
    sta FLEX_CH0_VOL_3
.endmacro

; Gate flexible channel 0 on
.macro GATE_FLEX0_ON
    lda FLEX_CH0_CTRL_0
    ora #CTRL_GATE
    sta FLEX_CH0_CTRL_0
.endmacro

; Gate flexible channel 0 off
.macro GATE_FLEX0_OFF
    lda FLEX_CH0_CTRL_0
    and #<(~CTRL_GATE)
    sta FLEX_CH0_CTRL_0
.endmacro

; Set flexible channel 0 ADSR
.macro SET_FLEX0_ADSR atk, dec, sus, rel
    lda #<(atk)
    sta FLEX_CH0_ATK_0
    lda #>(atk)
    sta FLEX_CH0_ATK_1
    lda #<(dec)
    sta FLEX_CH0_DEC_0
    lda #>(dec)
    sta FLEX_CH0_DEC_1
    lda #<(sus)
    sta FLEX_CH0_SUS_0
    lda #>(sus)
    sta FLEX_CH0_SUS_1
    lda #<(rel)
    sta FLEX_CH0_REL_0
    lda #>(rel)
    sta FLEX_CH0_REL_1
.endmacro

; Set PWM parameters for square channel
; rate: PWM oscillation rate (Hz * 256)
; depth: PWM depth (0-65535)
.macro SET_SQ_PWM rate, depth
    lda #<(rate)
    sta SQ_PWM_RATE_0
    lda #>(rate)
    sta SQ_PWM_RATE_1
    lda #0
    sta SQ_PWM_RATE_2
    sta SQ_PWM_RATE_3
    lda #<(depth)
    sta SQ_PWM_DEPTH_0
    lda #>(depth)
    sta SQ_PWM_DEPTH_1
    lda #0
    sta SQ_PWM_DEPTH_2
    sta SQ_PWM_DEPTH_3
    ; Enable PWM
    lda SQ_CTRL_0
    ora #CTRL_PWM_EN
    sta SQ_CTRL_0
.endmacro

; Enable hard sync on square channel (sync to another channel)
; src: source channel (0-3)
.macro SET_SQ_SYNC src
    lda #src
    sta SYNC_SRC_CH0_0
    lda #0
    sta SYNC_SRC_CH0_1
    sta SYNC_SRC_CH0_2
    sta SYNC_SRC_CH0_3
    ; Enable sync
    lda SQ_CTRL_0
    ora #CTRL_SYNC_EN
    sta SQ_CTRL_0
.endmacro

; Enable ring modulation on square channel
; src: source channel (0-3)
.macro SET_SQ_RING src
    lda #src
    sta RING_SRC_CH0_0
    lda #0
    sta RING_SRC_CH0_1
    sta RING_SRC_CH0_2
    sta RING_SRC_CH0_3
    ; Enable ring mod
    lda SQ_CTRL_0
    ora #CTRL_RING_EN
    sta SQ_CTRL_0
.endmacro

; Increment 16-bit zero page variable
.macro INC16 zp_var
    inc zp_var
    bne :+
    inc zp_var+1
:
.endmacro

; Compare 16-bit zero page variable with immediate
.macro CMP16 zp_var, value
    lda zp_var
    cmp #<(value)
    bne :+
    lda zp_var+1
    cmp #>(value)
:
.endmacro

; ============================================================================
; ZX SPECTRUM ULA VIDEO REGISTERS
;
; The ULA chip provides ZX Spectrum compatible video output:
; - 256x192 pixel display with 32-pixel border (320x256 total)
; - Attribute-based coloring: 8x8 pixel cells share INK/PAPER colors
; - 15 unique colors (8 base + 8 bright, black can't brighten)
; - FLASH attribute: swaps INK/PAPER at ~1.6Hz
;
; For 6502: ULA registers are memory-mapped at $D800-$D80F
; VRAM is at $4000 (banked, same as authentic ZX Spectrum)
;
; Non-linear bitmap addressing formula:
;   addr = ((y & $C0) << 5) + ((y & $07) << 8) + ((y & $38) << 2) + (x >> 3)
;
; Attribute byte format: FBPPPIII
;   F (bit 7): FLASH - swap INK/PAPER when set
;   B (bit 6): BRIGHT - intensify both colors
;   PPP (bits 5-3): PAPER (background) color 0-7
;   III (bits 2-0): INK (foreground) color 0-7
;
; Color palette:
;   0=Black, 1=Blue, 2=Red, 3=Magenta, 4=Green, 5=Cyan, 6=Yellow, 7=White
; ============================================================================

ULA_BASE        = $D800         ; ULA register base (6502 mapping)
ULA_BORDER      = $D800         ; Border color (bits 0-2)
ULA_CTRL        = $D804         ; Control register
ULA_STATUS      = $D808         ; Status register

; Control register bits
ULA_CTRL_ENABLE = $01           ; ULA enable bit

; Status register bits
ULA_STATUS_VBLANK = $01         ; VBlank active (cleared on read)

; VRAM layout (banked at $4000, same as authentic ZX Spectrum)
ULA_VRAM        = $4000         ; VRAM base address
ULA_BITMAP_SIZE = 6144          ; Bitmap: 256x192/8 = 6144 bytes
ULA_ATTR_OFFSET = $1800         ; Attribute offset from VRAM base
ULA_ATTR_SIZE   = 768           ; Attributes: 32x24 = 768 bytes
ULA_VRAM_SIZE   = 6912          ; Total: 6144 + 768 bytes

; Display dimensions
ULA_DISPLAY_W   = 256           ; Display width in pixels
ULA_DISPLAY_H   = 192           ; Display height in pixels
ULA_BORDER_SIZE = 32            ; Border size in pixels
ULA_FRAME_W     = 320           ; Total frame width (256+64)
ULA_FRAME_H     = 256           ; Total frame height (192+64)
ULA_CELLS_X     = 32            ; Character cells horizontally
ULA_CELLS_Y     = 24            ; Character cells vertically

; ----- ULA helper macros -----

; Set ULA border color (0-7)
.macro ULA_SET_BORDER color
    lda #color
    sta ULA_BORDER
.endmacro

; Enable ULA
.macro ULA_ENABLE
    lda #ULA_CTRL_ENABLE
    sta ULA_CTRL
.endmacro

; Disable ULA
.macro ULA_DISABLE
    lda #0
    sta ULA_CTRL
.endmacro

; Wait for ULA VBlank
; The status register VBlank bit is cleared on read, so we wait until it's set
.macro ULA_WAIT_VBLANK
:   lda ULA_STATUS
    and #ULA_STATUS_VBLANK
    beq :-
.endmacro

; ============================================================================
; ANTIC VIDEO REGISTERS (Atari 8-bit style)
;
; The ANTIC chip uses authentic Atari addresses for 6502 compatibility.
; This allows running original Atari software with minimal modification.
;
; Features:
; - Display list-driven graphics (14 modes)
; - 320x192 active display (384x240 with borders)
; - 128 colors (16 hues × 8 luminances)
; - Hardware fine scrolling
; - WSYNC for precise raster synchronization
; ============================================================================

; ANTIC at authentic Atari addresses
ANTIC_DMACTL    = $D400         ; DMA control
ANTIC_CHACTL    = $D401         ; Character control
ANTIC_DLISTL    = $D402         ; Display list pointer low
ANTIC_DLISTH    = $D403         ; Display list pointer high
ANTIC_HSCROL    = $D404         ; Horizontal scroll (0-15)
ANTIC_VSCROL    = $D405         ; Vertical scroll (0-15)
ANTIC_PMBASE    = $D407         ; Player-missile base address
ANTIC_CHBASE    = $D409         ; Character set base address
ANTIC_WSYNC     = $D40A         ; Wait for sync (write only)
ANTIC_VCOUNT    = $D40B         ; Vertical counter (read only, /2)
ANTIC_PENH      = $D40C         ; Light pen H (read only)
ANTIC_PENV      = $D40D         ; Light pen V (read only)
ANTIC_NMIEN     = $D40E         ; NMI enable
ANTIC_NMIST     = $D40F         ; NMI status (read) / NMIRES (write)

; DMACTL bits
ANTIC_DMA_NARROW  = $01         ; Narrow playfield
ANTIC_DMA_NORMAL  = $02         ; Normal playfield
ANTIC_DMA_WIDE    = $03         ; Wide playfield
ANTIC_DMA_MISSILE = $04         ; Enable missile DMA
ANTIC_DMA_PLAYER  = $08         ; Enable player DMA
ANTIC_DMA_PMRES   = $10         ; Player/missile resolution
ANTIC_DMA_DL      = $20         ; Enable display list DMA

; CHACTL bits
ANTIC_CHACTL_BLANK   = $01      ; Blank instead of inverse
ANTIC_CHACTL_INVERT  = $02      ; Invert video
ANTIC_CHACTL_REFLECT = $04      ; Reflect characters

; NMIEN bits
ANTIC_NMIEN_DLI   = $80         ; Display list interrupt enable
ANTIC_NMIEN_VBI   = $40         ; Vertical blank interrupt enable

; Display List Instructions
; Blank lines (1-8 scanlines)
DL_BLANK1         = $00         ; 1 blank scanline
DL_BLANK2         = $10         ; 2 blank scanlines
DL_BLANK3         = $20         ; 3 blank scanlines
DL_BLANK4         = $30         ; 4 blank scanlines
DL_BLANK5         = $40         ; 5 blank scanlines
DL_BLANK6         = $50         ; 6 blank scanlines
DL_BLANK7         = $60         ; 7 blank scanlines
DL_BLANK8         = $70         ; 8 blank scanlines

; Jump instructions
DL_JMP            = $01         ; Jump to address (2 bytes follow)
DL_JVB            = $41         ; Jump and wait for Vertical Blank

; Graphics modes
DL_MODE2          = $02         ; 40 col text, 8 scanlines/row
DL_MODE3          = $03         ; 40 col text, 10 scanlines/row
DL_MODE4          = $04         ; 40 col text, 8 scanlines, multicolor
DL_MODE5          = $05         ; 40 col text, 16 scanlines, multicolor
DL_MODE6          = $06         ; 20 col text, 8 scanlines
DL_MODE7          = $07         ; 20 col text, 16 scanlines
DL_MODE8          = $08         ; 40 pixels, 8 scanlines/row (GRAPHICS 3)
DL_MODE9          = $09         ; 80 pixels, 4 scanlines (GRAPHICS 4)
DL_MODE10         = $0A         ; 80 pixels, 2 scanlines (GRAPHICS 5)
DL_MODE11         = $0B         ; 160 pixels, 1 scanline (GRAPHICS 6)
DL_MODE12         = $0C         ; 160 pixels, 1 scanline (GRAPHICS 6+)
DL_MODE13         = $0D         ; 160 pixels, 2 scanlines (GRAPHICS 7)
DL_MODE14         = $0E         ; 160 pixels, 1 scanline, 4 colors
DL_MODE15         = $0F         ; 320 pixels, 1 scanline (GRAPHICS 8)

; Display list modifiers (OR with mode)
DL_LMS            = $40         ; Load Memory Scan (2 addr bytes follow)
DL_DLI            = $80         ; Display List Interrupt at end of line
DL_HSCROL         = $10         ; Horizontal scroll enable
DL_VSCROL         = $20         ; Vertical scroll enable

; NMIST bits
ANTIC_NMIST_RESET = $20         ; Reset button pressed
ANTIC_NMIST_DLI   = $80         ; DLI pending
ANTIC_NMIST_VBI   = $40         ; VBI pending

; ----- ANTIC helper macros -----

; Set display list pointer
.macro ANTIC_SET_DLIST addr
    lda #<addr
    sta ANTIC_DLISTL
    lda #>addr
    sta ANTIC_DLISTH
.endmacro

; Enable display list DMA with normal playfield
.macro ANTIC_ENABLE_DL
    lda #ANTIC_DMA_NORMAL | ANTIC_DMA_DL
    sta ANTIC_DMACTL
.endmacro

; Disable ANTIC display
.macro ANTIC_DISABLE
    lda #0
    sta ANTIC_DMACTL
.endmacro

; Wait for horizontal sync
.macro ANTIC_WAIT_HSYNC
    sta ANTIC_WSYNC
.endmacro

; ============================================================================
; GTIA VIDEO REGISTERS (Atari 8-bit color control)
;
; GTIA (Graphics Television Interface Adapter) handles color generation
; and player-missile graphics for Atari 8-bit computers. While ANTIC
; controls the display list and timing, GTIA controls colors and sprites.
;
; Color format: HHHHLLLL (hue in high nibble, luminance in low nibble)
; - 16 hues (0-15) x 8 luminances (0,2,4,6,8,10,12,14) = 128 colors
; - Hue 0 = grayscale, others are chromatic
;
; GTIA uses authentic Atari addresses for 6502 compatibility.
; ============================================================================

; Playfield color registers
GTIA_COLPF0     = $D016         ; Playfield color 0
GTIA_COLPF1     = $D017         ; Playfield color 1
GTIA_COLPF2     = $D018         ; Playfield color 2
GTIA_COLPF3     = $D019         ; Playfield color 3
GTIA_COLBK      = $D01A         ; Background/border color

; Player color registers
GTIA_COLPM0     = $D012         ; Player/missile 0 color
GTIA_COLPM1     = $D013         ; Player/missile 1 color
GTIA_COLPM2     = $D014         ; Player/missile 2 color
GTIA_COLPM3     = $D015         ; Player/missile 3 color

; GTIA control registers
GTIA_PRIOR      = $D01B         ; Priority and GTIA modes
GTIA_GRACTL     = $D01D         ; Graphics control
GTIA_CONSOL     = $D01F         ; Console switches (read)

; Player horizontal position registers (authentic Atari addresses)
GTIA_HPOSP0     = $D000         ; Player 0 horizontal position
GTIA_HPOSP1     = $D001         ; Player 1 horizontal position
GTIA_HPOSP2     = $D002         ; Player 2 horizontal position
GTIA_HPOSP3     = $D003         ; Player 3 horizontal position

; Missile horizontal position registers
GTIA_HPOSM0     = $D004         ; Missile 0 horizontal position
GTIA_HPOSM1     = $D005         ; Missile 1 horizontal position
GTIA_HPOSM2     = $D006         ; Missile 2 horizontal position
GTIA_HPOSM3     = $D007         ; Missile 3 horizontal position

; Player/missile size registers
GTIA_SIZEP0     = $D008         ; Player 0 size (0=normal,1=double,3=quad)
GTIA_SIZEP1     = $D009         ; Player 1 size
GTIA_SIZEP2     = $D00A         ; Player 2 size
GTIA_SIZEP3     = $D00B         ; Player 3 size
GTIA_SIZEM      = $D00C         ; All missile sizes (2 bits each)

; Player graphics registers (directly writable)
GTIA_GRAFP0     = $D00D         ; Player 0 graphics (8 pixels)
GTIA_GRAFP1     = $D00E         ; Player 1 graphics
GTIA_GRAFP2     = $D00F         ; Player 2 graphics
GTIA_GRAFP3     = $D010         ; Player 3 graphics
GTIA_GRAFM      = $D011         ; Missile graphics (2 bits each)

; PRIOR register bits
GTIA_PRIOR_P03    = $01         ; Players 0-3 have priority over PF
GTIA_PRIOR_P01    = $02         ; Players 0-1 have priority
GTIA_PRIOR_P23    = $04         ; Players 2-3 have priority
GTIA_PRIOR_MULTI  = $10         ; Enable multicolor players
GTIA_PRIOR_FIFTH  = $20         ; Enable fifth player
GTIA_PRIOR_GTIA1  = $40         ; GTIA mode bit 0
GTIA_PRIOR_GTIA2  = $80         ; GTIA mode bit 1

; GRACTL register bits
GTIA_GRACTL_MISSLE = $01        ; Enable missile graphics
GTIA_GRACTL_PLAYER = $02        ; Enable player graphics
GTIA_GRACTL_LATCH  = $04        ; Latch trigger inputs

; ----- GTIA helper macros -----

; Set background color
.macro GTIA_SET_BK color
    lda #color
    sta GTIA_COLBK
.endmacro

; Set playfield color 0
.macro GTIA_SET_PF0 color
    lda #color
    sta GTIA_COLPF0
.endmacro

; Set playfield color 1
.macro GTIA_SET_PF1 color
    lda #color
    sta GTIA_COLPF1
.endmacro

; Set playfield color 2
.macro GTIA_SET_PF2 color
    lda #color
    sta GTIA_COLPF2
.endmacro

; Set playfield color 3
.macro GTIA_SET_PF3 color
    lda #color
    sta GTIA_COLPF3
.endmacro

; ============================================================================
; 3DFX VOODOO GRAPHICS (SST-1) REGISTERS
; ============================================================================
; Vulkan-accelerated 3D graphics. See ie68.inc for detailed documentation.
; Note: 6502 cannot directly address 32-bit registers, use indirect addressing.

; Base address (high byte only for bank switching)
VOODOO_BASE_HI      = $F4

; Status register (read-only)
VOODOO_STATUS       = $F400

; Enable register (write-only) - enables the Voodoo engine for rendering
VOODOO_ENABLE       = $F400         ; Page offset $04 - Write 1 to enable, 0 to disable

; Vertex coordinate registers (12.4 fixed-point)
; For 6502, these require 32-bit writes via indirect addressing
VOODOO_VERTEX_AX    = $F400         ; Page offset $08
VOODOO_VERTEX_AY    = $F400         ; Page offset $0C
VOODOO_VERTEX_BX    = $F401         ; Page offset $10
VOODOO_VERTEX_BY    = $F401         ; Page offset $14
VOODOO_VERTEX_CX    = $F401         ; Page offset $18
VOODOO_VERTEX_CY    = $F401         ; Page offset $1C

; Vertex attribute start values (12.12 fixed-point)
VOODOO_START_R      = $F402         ; Page offset $20
VOODOO_START_G      = $F402         ; Page offset $24
VOODOO_START_B      = $F402         ; Page offset $28
VOODOO_START_Z      = $F402         ; Page offset $2C
VOODOO_START_A      = $F403         ; Page offset $30
VOODOO_START_S      = $F403         ; Page offset $34
VOODOO_START_T      = $F403         ; Page offset $38
VOODOO_START_W      = $F403         ; Page offset $3C

; Command registers
VOODOO_TRIANGLE_CMD = $F408         ; Page offset $80
VOODOO_COLOR_SELECT = $F408         ; Page offset $88 - Select vertex (0/1/2) for Gouraud shading
VOODOO_ALPHA_MODE   = $F410         ; Page offset $0C
VOODOO_FBZ_MODE     = $F411         ; Page offset $10
VOODOO_FAST_FILL_CMD = $F412        ; Page offset $24
VOODOO_SWAP_BUFFER_CMD = $F412      ; Page offset $28

; Configuration registers
VOODOO_COLOR0       = $F41D         ; Page offset $D8
VOODOO_VIDEO_DIM    = $F421         ; Page offset $14

; Texture registers
VOODOO_TEXTURE_MODE = $F430         ; Texture mode configuration
VOODOO_TEX_BASE0    = $F430         ; Page offset $0C - Texture base address
VOODOO_TEX_WIDTH    = $F433         ; Page offset $30 - Texture width for upload (IE extension)
VOODOO_TEX_HEIGHT   = $F433         ; Page offset $34 - Texture height for upload (IE extension)
VOODOO_TEX_UPLOAD   = $F433         ; Page offset $38 - Write to trigger texture upload (IE extension)
VOODOO_TEXMEM_BASE  = $F500         ; Texture memory base (64KB)
VOODOO_FBZCOLOR_PATH = $F410        ; Page offset $04 - Color combine mode

; fbzMode bits
VOODOO_FBZ_CLIPPING     = $01
VOODOO_FBZ_DEPTH_ENABLE = $10
VOODOO_FBZ_RGB_WRITE    = $02       ; Bit 9 in low byte of high word
VOODOO_FBZ_DEPTH_WRITE  = $04       ; Bit 10 in low byte of high word

; Depth compare functions (shift left 5 to use in FBZ_MODE)
VOODOO_DEPTH_NEVER        = 0
VOODOO_DEPTH_LESS         = 1
VOODOO_DEPTH_EQUAL        = 2
VOODOO_DEPTH_LESSEQUAL    = 3
VOODOO_DEPTH_GREATER      = 4
VOODOO_DEPTH_NOTEQUAL     = 5
VOODOO_DEPTH_GREATEREQUAL = 6
VOODOO_DEPTH_ALWAYS       = 7

; alphaMode bits
; Bit 0:     ALPHA_TEST_EN - Enable alpha test
; Bits 1-3:  ALPHA_FUNC - Alpha test function (0-7, same as depth functions)
; Bit 4:     ALPHA_BLEND_EN - Enable alpha blending
; Bits 8-11: SRC_BLEND - Source blend factor
; Bits 12-15: DST_BLEND - Destination blend factor
; Bits 24-31: ALPHA_REF - Alpha reference value (0-255)
VOODOO_ALPHA_TEST_EN    = $01           ; Enable alpha test
VOODOO_ALPHA_FUNC_MASK  = $0E           ; Alpha function mask (bits 1-3)
VOODOO_ALPHA_FUNC_SHIFT = 1             ; Shift for alpha function
VOODOO_ALPHA_BLEND_EN   = $10           ; Enable alpha blending
VOODOO_ALPHA_REF_SHIFT  = 24            ; Shift for alpha reference value

; Alpha test functions (shift left 1 to position in alphaMode)
; Same comparison semantics as depth functions
VOODOO_ALPHA_NEVER        = 0           ; Never pass
VOODOO_ALPHA_LESS         = 1           ; Pass if alpha < reference
VOODOO_ALPHA_EQUAL        = 2           ; Pass if alpha == reference
VOODOO_ALPHA_LESSEQUAL    = 3           ; Pass if alpha <= reference
VOODOO_ALPHA_GREATER      = 4           ; Pass if alpha > reference
VOODOO_ALPHA_NOTEQUAL     = 5           ; Pass if alpha != reference
VOODOO_ALPHA_GREATEREQUAL = 6           ; Pass if alpha >= reference
VOODOO_ALPHA_ALWAYS       = 7           ; Always pass

; Alpha blend factors (for src/dst blend settings)
; Use these values shifted to bits 8-11 for source, bits 12-15 for dest
VOODOO_BLEND_ZERO       = 0             ; 0
VOODOO_BLEND_SRC_ALPHA  = 1             ; src.A
VOODOO_BLEND_COLOR      = 2             ; constant color
VOODOO_BLEND_DST_ALPHA  = 3             ; dst.A
VOODOO_BLEND_ONE        = 4             ; 1
VOODOO_BLEND_INV_SRC_A  = 5             ; 1 - src.A
VOODOO_BLEND_INV_COLOR  = 6             ; 1 - constant color
VOODOO_BLEND_INV_DST_A  = 7             ; 1 - dst.A
VOODOO_BLEND_SATURATE   = 15            ; min(src.A, 1-dst.A)

; textureMode bits
; Bit layout:
;   Bit 0:     TEX_ENABLE - Enable texture mapping
;   Bits 1-3:  TEX_MINIFY - Minification filter (0=point)
;   Bit 4:     TEX_MAGNIFY - Magnification filter (0=point, 1=bilinear)
;   Bit 5:     TEX_CLAMP_S - Clamp S coordinate (vs wrap)
;   Bit 6:     TEX_CLAMP_T - Clamp T coordinate (vs wrap)
;   Bits 8-11: TEX_FORMAT - Texture format
VOODOO_TEX_ENABLE       = $0001         ; Enable texture mapping
VOODOO_TEX_MINIFY       = $000E         ; Minification filter mask (bits 1-3)
VOODOO_TEX_MAGNIFY      = $0010         ; Magnification filter (0=point, 1=bilinear)
VOODOO_TEX_CLAMP_S      = $0020         ; Clamp S coordinate
VOODOO_TEX_CLAMP_T      = $0040         ; Clamp T coordinate
VOODOO_TEX_FORMAT       = $0F00         ; Texture format mask (bits 8-11)
VOODOO_TEX_FORMAT_SHIFT = 8             ; Shift for texture format

; Texture format constants (for TEX_FORMAT field)
VOODOO_TEX_FMT_PALETTE  = 0             ; 8-bit paletted
VOODOO_TEX_FMT_P8       = 5             ; 8-bit palette (alternative)
VOODOO_TEX_FMT_ARGB1555 = 8             ; ARGB 1555
VOODOO_TEX_FMT_ARGB4444 = 9             ; ARGB 4444
VOODOO_TEX_FMT_ARGB8888 = 10            ; ARGB 8888 (default)

; Fixed-point shift amounts
VOODOO_FIXED_12_4       = 4
VOODOO_FIXED_12_12      = 12

; fbzColorPath bit fields (Color Combine)
; The fbzColorPath register controls how texture and iterated (vertex) colors are combined
VOODOO_FCP_RGB_SELECT_MASK  = $03           ; Bits 0-1: RGB source select
VOODOO_FCP_RGB_SELECT_SHIFT = 0             ; Shift for RGB select
VOODOO_FCP_A_SELECT_MASK    = $0C           ; Bits 2-3: Alpha source select
VOODOO_FCP_A_SELECT_SHIFT   = 2             ; Shift for alpha select
VOODOO_FCP_CC_MSELECT_MASK  = $70           ; Bits 4-6: Color combine mode select
VOODOO_FCP_CC_MSELECT_SHIFT = 4             ; Shift for CC mode
VOODOO_FCP_TEXTURE_ENABLE   = $08000000     ; Bit 27: Enable texture in color path

; Color source select values (for RGB_SELECT and A_SELECT)
VOODOO_CC_ITERATED      = 0                 ; Use iterated (vertex) color
VOODOO_CC_TEXTURE       = 1                 ; Use texture color
VOODOO_CC_COLOR1        = 2                 ; Use constant color1
VOODOO_CC_LFB           = 3                 ; Use linear framebuffer color

; Color combine function modes (for CC_MSELECT)
; These define how the two color sources are combined
VOODOO_CC_ZERO          = 0                 ; Output zero (black)
VOODOO_CC_CSUB_CL       = 1                 ; cother - clocal (subtract)
VOODOO_CC_ALOCAL        = 2                 ; clocal * alocal (modulate by local alpha)
VOODOO_CC_AOTHER        = 3                 ; clocal * aother (modulate by other alpha)
VOODOO_CC_CLOCAL        = 4                 ; clocal only (pass through)
VOODOO_CC_ALOCAL_T      = 5                 ; alocal * texture (alpha * texture)
VOODOO_CC_CLOC_MUL      = 6                 ; clocal * cother (multiply/modulate)
VOODOO_CC_AOTHER_T      = 7                 ; aother * texture

; Simplified color combine modes for common operations
; These are convenience values that combine select + mode bits
VOODOO_COMBINE_UNSET    = $FFFFFFFF         ; Not explicitly set (use defaults)
VOODOO_COMBINE_ITERATED = $00               ; Vertex color only (default when no texture)
VOODOO_COMBINE_TEXTURE  = $01               ; Texture color only
VOODOO_COMBINE_MODULATE = $61               ; tex * vert (texture modulated by vertex color)
VOODOO_COMBINE_ADD      = $81               ; tex + vert (additive blend, clamped)
VOODOO_COMBINE_DECAL    = $41               ; texture with vertex alpha

; Phase 6: fogMode bit fields
; The fogMode register controls depth-based fog blending
VOODOO_FOG_ENABLE       = $0001             ; Enable fog processing
VOODOO_FOG_ADD          = $0002             ; Add fog color to output (vs blend)
VOODOO_FOG_MULT         = $0004             ; Multiply fog factor by alpha
VOODOO_FOG_ZALPHA       = $0008             ; Use Z alpha for fog (vs iterated)
VOODOO_FOG_CONSTANT     = $0010             ; Use constant fog alpha
VOODOO_FOG_DITHER       = $0020             ; Apply dithering to fog
VOODOO_FOG_ZONES        = $0040             ; Enable fog zones (table-based fog)
VOODOO_FOG_TABLE_SHIFT  = 8                 ; Shift for fog table index
VOODOO_FOG_TABLE_MASK   = $3F               ; 6-bit fog table index mask

; Phase 6: Fog table constants
VOODOO_FOG_TABLE_BASE   = $F4140            ; Fog table base address (64 entries)
VOODOO_FOG_TABLE_SIZE   = 64                ; Number of fog table entries

; Phase 6: fbzMode dithering bits (add to fbzMode)
VOODOO_FBZ_DITHER       = $0100             ; Enable 4x4 ordered dithering
VOODOO_FBZ_DITHER_2X2   = $0800             ; Use 2x2 dither pattern (vs 4x4)

; ============================================================================
; ZERO PAGE ALLOCATION HELPER
; ============================================================================

; Define commonly used zero page variables
.zeropage
    zp_ptr0:    .res 2          ; General purpose pointer 0
    zp_ptr1:    .res 2          ; General purpose pointer 1
    zp_ptr2:    .res 2          ; General purpose pointer 2
    zp_ptr3:    .res 2          ; General purpose pointer 3
    zp_tmp0:    .res 4          ; 32-bit temporary 0
    zp_tmp1:    .res 4          ; 32-bit temporary 1
    zp_frame:   .res 2          ; Frame counter
    zp_scratch: .res 8          ; Scratch space
