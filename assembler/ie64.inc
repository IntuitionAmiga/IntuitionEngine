; ie64.inc - Intuition Engine IE64 Macro Library
;
; Hardware definitions for writing IE64 assembly programs targeting
; the Intuition Engine virtual machine.
;
; For use with ie64asm assembler
; Include with: include "ie64.inc"
;
; All macros clobber r1-r4 (caller-saved scratch registers).
;
; RENAMED MACROS (ie68.inc -> ie64.inc):
;   The ie64asm assembler treats equate lines whose name matches a macro
;   (case-insensitive) as macro invocations. The following macros are
;   renamed to avoid shadowing their corresponding hardware equates:
;
;     ie68.inc name     -> ie64.inc name       (conflicting equate)
;     ula_border        -> set_ula_border      (ULA_BORDER)
;     ted_v_enable      -> set_ted_v_enable    (TED_V_ENABLE)
;     ted_v_border      -> set_ted_v_border    (TED_V_BORDER)
;     antic_enable      -> set_antic_enable    (ANTIC_ENABLE)
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; MEMORY MAP CONSTANTS
; ============================================================================

; System vectors
VECTOR_BASE       equ 0x000000
VECTOR_RESET      equ 0x000000
VECTOR_IRQ        equ 0x000004

; Program space
PROGRAM_START     equ 0x001000
STACK_TOP         equ 0x09F000              ; Top of stack (grows downward)

; ============================================================================
; EhBASIC IE64 MEMORY MAP
; ============================================================================

; Interpreter code: placed by assembler from org 0x1000
; Reserve 0x1000-0x20FFF (128KB) for code growth.

; Working buffers
BASIC_LINE_BUF    equ 0x021000              ; Input line buffer (4KB, 1024 chars max)
BASIC_LINE_BUFLEN equ 1024
BASIC_LINE_END    equ 0x021FFF

; Interpreter state variables block (pointer cached in R16)
BASIC_STATE       equ 0x022000              ; State block (4KB)
BASIC_STATE_END   equ 0x022FFF

; State block offsets (add to BASIC_STATE / R16)
ST_PROG_START     equ 0x00                  ; Start of BASIC program text
ST_PROG_END       equ 0x04                  ; End of program text (next free)
ST_VAR_START      equ 0x08                  ; Simple variable storage start
ST_VAR_END        equ 0x0C                  ; Simple variable storage end
ST_ARRAY_START    equ 0x10                  ; Array storage start
ST_ARRAY_END      equ 0x14                  ; Array storage end
ST_HEAP_TOP       equ 0x18                  ; String heap top (grows down)
ST_HEAP_BOTTOM    equ 0x1C                  ; Heap bottom limit
ST_CURRENT_LINE   equ 0x20                  ; Line number being executed
ST_TEXT_PTR       equ 0x24                  ; Execution cursor
ST_DATA_PTR       equ 0x28                  ; DATA/READ pointer
ST_GOSUB_SP       equ 0x2C                  ; GOSUB return stack pointer
ST_FOR_SP         equ 0x30                  ; FOR/NEXT stack pointer
ST_RANDOM_SEED    equ 0x34                  ; RND seed
ST_ERROR_FLAG     equ 0x38                  ; Last error code
ST_TRACE_FLAG     equ 0x3C                  ; TRON/TROFF
ST_DIRECT_MODE    equ 0x40                  ; 1=immediate, 0=running
ST_SVAR_START     equ 0x44                  ; String variable table start
ST_SVAR_END       equ 0x48                  ; String variable table end
ST_TEXT_ATTR      equ 0x4C                  ; Text attribute (COLOR fg,bg)
ST_COPPER_BUILD   equ 0x50                  ; Copper build pointer
ST_ULA_BRIGHT     equ 0x54                  ; ULA bright flag
ST_ULA_INK        equ 0x58                  ; ULA ink colour
ST_ULA_PAPER      equ 0x5C                  ; ULA paper colour
ST_ULA_FLASH      equ 0x60                  ; ULA flash flag
ST_CONT_LINE_PTR  equ 0x64                  ; CONT: saved line pointer (R14)
ST_CONT_TEXT_PTR  equ 0x68                  ; CONT: saved text pointer (R17)

; BASIC program/variable storage
BASIC_PROG_START  equ 0x023000              ; Program text + variables + heap (428KB)
BASIC_PROG_END    equ 0x08BFFF

; String temporary space
BASIC_STR_TEMP    equ 0x08C000              ; String temporaries (16KB)
BASIC_STR_END     equ 0x08FFFF

; Stack regions
BASIC_GOSUB_STACK equ 0x090000              ; GOSUB/FOR stacks (28KB)
BASIC_GOSUB_END   equ 0x096FFF
BASIC_HW_STACK    equ 0x097000              ; Hardware stack (R31, grows down, 32KB)
BASIC_HW_STACK_END equ 0x09EFFF
; STACK_TOP (0x09F000) = initial SP value
; 0x09F000-0x0A0000 = 4KB unmapped guard zone

; Bank window addresses (for 8-bit CPU compatibility)
BANK1_WINDOW      equ 0x002000              ; Sprite data bank window (8KB)
BANK2_WINDOW      equ 0x004000              ; Font data bank window (8KB)
BANK3_WINDOW      equ 0x006000              ; General data bank window (8KB)
VRAM_WINDOW       equ 0x008000              ; VRAM bank window (16KB)
BANK_SIZE         equ 0x002000              ; 8KB per extended bank
VRAM_BANK_SIZE    equ 0x004000              ; 16KB VRAM bank

; Bank control registers
BANK1_REG_LO      equ 0x00F700              ; Bank 1 select (low byte)
BANK1_REG_HI      equ 0x00F701              ; Bank 1 select (high byte)
BANK2_REG_LO      equ 0x00F702              ; Bank 2 select (low byte)
BANK2_REG_HI      equ 0x00F703              ; Bank 2 select (high byte)
BANK3_REG_LO      equ 0x00F704              ; Bank 3 select (low byte)
BANK3_REG_HI      equ 0x00F705              ; Bank 3 select (high byte)
VRAM_BANK_REG     equ 0x00F7F0              ; VRAM bank select

; ============================================================================
; TERMINAL I/O REGISTERS (0xF0700-0xF07FF)
; ============================================================================

TERM_OUT          equ 0xF0700               ; Character output (W)
TERM_STATUS       equ 0xF0704               ; Bit 0: input available, Bit 1: output ready (R)
TERM_IN           equ 0xF0708               ; Read next input char, dequeues (R)
TERM_LINE_STATUS  equ 0xF070C               ; Bit 0: complete line available (R)
TERM_ECHO         equ 0xF0710               ; Bit 0: local echo enable (W, default 1)
TERM_SENTINEL     equ 0xF07F0               ; Write 0xDEAD to stop CPU (via OnSentinel callback)

; ============================================================================
; VIDEO HARDWARE REGISTERS (I/O region at 0xF0000)
; ============================================================================

VIDEO_CTRL        equ 0xF0000               ; Video control (enable/disable)
VIDEO_MODE        equ 0xF0004               ; Video mode (resolution)
VIDEO_STATUS      equ 0xF0008               ; Video status (VBlank flag)
STATUS_VBLANK     equ 2                     ; VBlank status bit

; Copper coprocessor
COPPER_CTRL       equ 0xF000C               ; Copper control
COPPER_PTR        equ 0xF0010               ; Copper list pointer (32-bit)

; Blitter hardware
BLT_CTRL          equ 0xF001C               ; Blitter control
BLT_OP            equ 0xF0020               ; Blitter operation
BLT_SRC           equ 0xF0024               ; Source address (32-bit)
BLT_DST           equ 0xF0028               ; Dest address (32-bit)
BLT_WIDTH         equ 0xF002C               ; Blit width (32-bit)
BLT_HEIGHT        equ 0xF0030               ; Blit height (32-bit)
BLT_SRC_STRIDE    equ 0xF0034               ; Source stride (32-bit)
BLT_DST_STRIDE    equ 0xF0038               ; Dest stride (32-bit)
BLT_COLOR         equ 0xF003C               ; Fill color (32-bit BGRA)
BLT_MASK          equ 0xF0040               ; Mask address (32-bit)
BLT_STATUS        equ 0xF0044               ; Blitter status

; Raster registers
VIDEO_RASTER_Y       equ 0xF0048            ; Raster Y position (32-bit)
VIDEO_RASTER_HEIGHT  equ 0xF004C            ; Raster height (32-bit)
VIDEO_RASTER_COLOR   equ 0xF0050            ; Raster color (32-bit BGRA)
VIDEO_RASTER_CTRL    equ 0xF0054            ; Raster control

; ============================================================================
; PSG/AUDIO REGISTERS (AY-3-8910 style)
; ============================================================================

PSG_BASE          equ 0xF0C00               ; PSG base address (AY-3-8910 registers)
PSG_PLUS_CTRL     equ 0xF0C0E               ; PSG+ mode control

; PSG Player Registers
PSG_PLAY_PTR      equ 0xF0C10               ; Play pointer (32-bit)
PSG_PLAY_LEN      equ 0xF0C14               ; Play length (32-bit)
PSG_PLAY_CTRL     equ 0xF0C18               ; Play control
PSG_PLAY_STATUS   equ 0xF0C1C               ; Play status

; ============================================================================
; POKEY AUDIO REGISTERS (Atari style)
; ============================================================================

POKEY_BASE        equ 0xF0D00               ; POKEY base address
POKEY_AUDF1       equ 0xF0D00               ; Channel 1 frequency divider
POKEY_AUDC1       equ 0xF0D01               ; Channel 1 control (distortion + volume)
POKEY_AUDF2       equ 0xF0D02               ; Channel 2 frequency divider
POKEY_AUDC2       equ 0xF0D03               ; Channel 2 control
POKEY_AUDF3       equ 0xF0D04               ; Channel 3 frequency divider
POKEY_AUDC3       equ 0xF0D05               ; Channel 3 control
POKEY_AUDF4       equ 0xF0D06               ; Channel 4 frequency divider
POKEY_AUDC4       equ 0xF0D07               ; Channel 4 control
POKEY_AUDCTL      equ 0xF0D08               ; Master audio control
POKEY_PLUS_CTRL   equ 0xF0D09               ; POKEY+ mode (0=standard, 1=enhanced)

; AUDCTL bit masks
AUDCTL_CLOCK_15KHZ   equ 0x01               ; Use 15kHz base clock (else 64kHz)
AUDCTL_HIPASS_CH1    equ 0x02               ; High-pass filter ch1 by ch3
AUDCTL_HIPASS_CH2    equ 0x04               ; High-pass filter ch2 by ch4
AUDCTL_CH4_BY_CH3    equ 0x08               ; Ch4 clocked by ch3 (16-bit mode)
AUDCTL_CH2_BY_CH1    equ 0x10               ; Ch2 clocked by ch1 (16-bit mode)
AUDCTL_CH3_179MHZ    equ 0x20               ; Ch3 uses 1.79MHz clock
AUDCTL_CH1_179MHZ    equ 0x40               ; Ch1 uses 1.79MHz clock
AUDCTL_POLY9         equ 0x80               ; Use 9-bit poly instead of 17-bit

; AUDC distortion modes (bits 5-7)
POKEY_DIST_POLY17_POLY5  equ 0x00           ; 17-bit + 5-bit poly
POKEY_DIST_POLY5         equ 0x20           ; 5-bit poly only
POKEY_DIST_POLY17_POLY4  equ 0x40           ; 17-bit + 4-bit poly
POKEY_DIST_POLY5_POLY4   equ 0x60           ; 5-bit + 4-bit poly
POKEY_DIST_POLY17        equ 0x80           ; 17-bit poly only
POKEY_DIST_PURE_TONE     equ 0xA0           ; Pure square wave
POKEY_DIST_POLY4         equ 0xC0           ; 4-bit poly only
POKEY_DIST_POLY17_PULSE  equ 0xE0           ; 17-bit + pulse

; ============================================================================
; SID AUDIO REGISTERS (MOS 6581/8580 style)
; ============================================================================

SID_BASE          equ 0xF0E00               ; SID base address

; Voice 1 registers
SID_V1_FREQ_LO    equ 0xF0E00               ; Voice 1 frequency low byte
SID_V1_FREQ_HI    equ 0xF0E01               ; Voice 1 frequency high byte
SID_V1_PW_LO      equ 0xF0E02               ; Voice 1 pulse width low byte
SID_V1_PW_HI      equ 0xF0E03               ; Voice 1 pulse width high (bits 0-3)
SID_V1_CTRL       equ 0xF0E04               ; Voice 1 control register
SID_V1_AD         equ 0xF0E05               ; Voice 1 attack/decay
SID_V1_SR         equ 0xF0E06               ; Voice 1 sustain/release

; Voice 2 registers
SID_V2_FREQ_LO    equ 0xF0E07               ; Voice 2 frequency low byte
SID_V2_FREQ_HI    equ 0xF0E08               ; Voice 2 frequency high byte
SID_V2_PW_LO      equ 0xF0E09               ; Voice 2 pulse width low byte
SID_V2_PW_HI      equ 0xF0E0A               ; Voice 2 pulse width high
SID_V2_CTRL       equ 0xF0E0B               ; Voice 2 control register
SID_V2_AD         equ 0xF0E0C               ; Voice 2 attack/decay
SID_V2_SR         equ 0xF0E0D               ; Voice 2 sustain/release

; Voice 3 registers
SID_V3_FREQ_LO    equ 0xF0E0E               ; Voice 3 frequency low byte
SID_V3_FREQ_HI    equ 0xF0E0F               ; Voice 3 frequency high byte
SID_V3_PW_LO      equ 0xF0E10               ; Voice 3 pulse width low byte
SID_V3_PW_HI      equ 0xF0E11               ; Voice 3 pulse width high
SID_V3_CTRL       equ 0xF0E12               ; Voice 3 control register
SID_V3_AD         equ 0xF0E13               ; Voice 3 attack/decay
SID_V3_SR         equ 0xF0E14               ; Voice 3 sustain/release

; Filter and volume registers
SID_FC_LO         equ 0xF0E15               ; Filter cutoff low (bits 0-2)
SID_FC_HI         equ 0xF0E16               ; Filter cutoff high byte
SID_RES_FILT      equ 0xF0E17               ; Resonance (bits 4-7) + routing (bits 0-3)
SID_MODE_VOL      equ 0xF0E18               ; Volume (bits 0-3) + filter mode (bits 4-7)
SID_PLUS_CTRL     equ 0xF0E19               ; SID+ mode (0=standard, 1=enhanced)

; Voice control register bits (SID_Vx_CTRL)
SID_CTRL_GATE     equ 0x01                  ; Gate bit (trigger envelope)
SID_CTRL_SYNC     equ 0x02                  ; Sync with previous voice
SID_CTRL_RINGMOD  equ 0x04                  ; Ring modulation
SID_CTRL_TEST     equ 0x08                  ; Test bit (resets oscillator)
SID_CTRL_TRIANGLE equ 0x10                  ; Triangle waveform
SID_CTRL_SAWTOOTH equ 0x20                  ; Sawtooth waveform
SID_CTRL_PULSE    equ 0x40                  ; Pulse/square waveform
SID_CTRL_NOISE    equ 0x80                  ; Noise waveform

; Filter routing bits (SID_RES_FILT bits 0-3)
SID_FILT_V1       equ 0x01                  ; Route voice 1 through filter
SID_FILT_V2       equ 0x02                  ; Route voice 2 through filter
SID_FILT_V3       equ 0x04                  ; Route voice 3 through filter
SID_FILT_EXT      equ 0x08                  ; Route external input through filter

; Filter mode bits (SID_MODE_VOL bits 4-7)
SID_MODE_LP       equ 0x10                  ; Low-pass filter
SID_MODE_BP       equ 0x20                  ; Band-pass filter
SID_MODE_HP       equ 0x40                  ; High-pass filter
SID_MODE_3OFF     equ 0x80                  ; Disconnect voice 3 from output

; SID Player Registers
SID_PLAY_PTR      equ 0xF0E20               ; Play pointer (32-bit)
SID_PLAY_LEN      equ 0xF0E24               ; Play length (32-bit)
SID_PLAY_CTRL     equ 0xF0E28               ; Play control
SID_PLAY_STATUS   equ 0xF0E2C               ; Play status
SID_SUBSONG       equ 0xF0E2D               ; Subsong selection (0-255)

; SAP Player Registers
SAP_PLAY_PTR      equ 0xF0D10               ; Play pointer (32-bit)
SAP_PLAY_LEN      equ 0xF0D14               ; Play length (32-bit)
SAP_PLAY_CTRL     equ 0xF0D18               ; Play control
SAP_PLAY_STATUS   equ 0xF0D1C               ; Play status
SAP_SUBSONG       equ 0xF0D1D               ; Subsong selection (0-255)

; ============================================================================
; TED AUDIO REGISTERS (Commodore Plus/4 style)
; ============================================================================

TED_BASE          equ 0xF0F00               ; TED base address
TED_FREQ1_LO      equ 0xF0F00               ; Voice 1 frequency low byte
TED_FREQ2_LO      equ 0xF0F01               ; Voice 2 frequency low byte
TED_FREQ2_HI      equ 0xF0F02               ; Voice 2 frequency high (bits 0-1)
TED_SND_CTRL      equ 0xF0F03               ; Sound control register
TED_FREQ1_HI      equ 0xF0F04               ; Voice 1 frequency high (bits 0-1)
TED_PLUS_CTRL     equ 0xF0F05               ; TED+ mode (0=standard, 1=enhanced)

; TED Control register bits (TED_SND_CTRL)
TED_CTRL_SNDDC     equ 0x80                 ; D/A mode (bit 7)
TED_CTRL_SND2NOISE equ 0x40                 ; Voice 2 noise enable (bit 6)
TED_CTRL_SND2ON    equ 0x20                 ; Voice 2 enable (bit 5)
TED_CTRL_SND1ON    equ 0x10                 ; Voice 1 enable (bit 4)
TED_CTRL_VOLUME    equ 0x0F                 ; Volume mask (bits 0-3, max=8)

; TED Player Registers
TED_PLAY_PTR      equ 0xF0F10               ; Play pointer (32-bit)
TED_PLAY_LEN      equ 0xF0F14               ; Play length (32-bit)
TED_PLAY_CTRL     equ 0xF0F18               ; Play control
TED_PLAY_STATUS   equ 0xF0F1C               ; Play status

; ============================================================================
; TED VIDEO REGISTERS (Commodore Plus/4 style)
; ============================================================================

TED_VIDEO_BASE    equ 0xF0F20               ; TED video register base

; Control registers
TED_V_CTRL1       equ 0xF0F20               ; Control 1 (ECM/BMM/DEN/RSEL/YSCROLL)
TED_V_CTRL2       equ 0xF0F24               ; Control 2 (RES/MCM/CSEL/XSCROLL)
TED_V_CHAR_BASE   equ 0xF0F28               ; Character/bitmap base address
TED_V_VIDEO_BASE  equ 0xF0F2C               ; Video matrix base address

; Color registers
TED_V_BG_COLOR0   equ 0xF0F30               ; Background color 0
TED_V_BG_COLOR1   equ 0xF0F34               ; Background color 1 (multicolor)
TED_V_BG_COLOR2   equ 0xF0F38               ; Background color 2 (multicolor)
TED_V_BG_COLOR3   equ 0xF0F3C               ; Background color 3 (multicolor)
TED_V_BORDER      equ 0xF0F40               ; Border color

; Cursor registers
TED_V_CURSOR_HI   equ 0xF0F44               ; Cursor position high byte
TED_V_CURSOR_LO   equ 0xF0F48               ; Cursor position low byte
TED_V_CURSOR_CLR  equ 0xF0F4C               ; Cursor color

; Status registers
TED_V_RASTER_LO   equ 0xF0F50               ; Raster line low (read-only)
TED_V_RASTER_HI   equ 0xF0F54               ; Raster line high (read-only)
TED_V_ENABLE      equ 0xF0F58               ; Video enable (bit 0)
TED_V_STATUS      equ 0xF0F5C               ; Status (bit 0 = VBlank)

; TED_V_CTRL1 bits
TED_V_CTRL1_ECM      equ 0x40               ; Bit 6: Extended Color Mode
TED_V_CTRL1_BMM      equ 0x20               ; Bit 5: Bitmap Mode
TED_V_CTRL1_DEN      equ 0x10               ; Bit 4: Display Enable
TED_V_CTRL1_RSEL     equ 0x08               ; Bit 3: Row Select (0=24, 1=25)
TED_V_CTRL1_YSCROLL  equ 0x07               ; Bits 0-2: Vertical scroll mask

; TED_V_CTRL2 bits
TED_V_CTRL2_RES      equ 0x20               ; Bit 5: Reset
TED_V_CTRL2_MCM      equ 0x10               ; Bit 4: Multicolor Mode
TED_V_CTRL2_CSEL     equ 0x08               ; Bit 3: Column Select (0=38, 1=40)
TED_V_CTRL2_XSCROLL  equ 0x07               ; Bits 0-2: Horizontal scroll mask

; Enable/Status bits
TED_V_ENABLE_VIDEO   equ 0x01               ; Bit 0: Video enable
TED_V_STATUS_VBLANK  equ 0x01               ; Bit 0: VBlank active

; Display dimensions
TED_V_DISPLAY_W   equ 320                   ; Display width in pixels
TED_V_DISPLAY_H   equ 200                   ; Display height in pixels
TED_V_FRAME_W     equ 384                   ; Total frame width (with border)
TED_V_FRAME_H     equ 272                   ; Total frame height (with border)
TED_V_CELLS_X     equ 40                    ; Text columns
TED_V_CELLS_Y     equ 25                    ; Text rows

; Copper SETBASE for TED video
COP_SETBASE_TED_VIDEO equ 0x8003C3C8        ; TED_VIDEO_BASE (0xF0F20 >> 2)

; ============================================================================
; AHX AUDIO REGISTERS (Amiga AHX module player)
; ============================================================================

AHX_BASE          equ 0xF0B80               ; AHX base address
AHX_PLUS_CTRL     equ 0xF0B80               ; AHX+ mode (0=standard, 1=enhanced)

; AHX Player Registers
AHX_PLAY_PTR      equ 0xF0B84               ; Play pointer (32-bit)
AHX_PLAY_LEN      equ 0xF0B88               ; Play length (32-bit)
AHX_PLAY_CTRL     equ 0xF0B8C               ; Play control
AHX_PLAY_STATUS   equ 0xF0B90               ; Play status
AHX_SUBSONG       equ 0xF0B91               ; Subsong selection (0-255)

; ============================================================================
; VGA VIDEO REGISTERS (IBM VGA compatible)
; ============================================================================

VGA_BASE          equ 0xF1000
VGA_MODE          equ 0xF1000               ; Mode control
VGA_STATUS        equ 0xF1004               ; Status register
VGA_CTRL          equ 0xF1008               ; Control register

; Sequencer (0x3C4/0x3C5 equivalent)
VGA_SEQ_INDEX     equ 0xF1010               ; Sequencer index
VGA_SEQ_DATA      equ 0xF1014               ; Sequencer data
VGA_SEQ_MAPMASK   equ 0xF1018               ; Plane write mask (direct)

; CRTC (0x3D4/0x3D5 equivalent)
VGA_CRTC_INDEX    equ 0xF1020               ; CRTC index
VGA_CRTC_DATA     equ 0xF1024               ; CRTC data
VGA_CRTC_STARTHI  equ 0xF1028               ; Start address high
VGA_CRTC_STARTLO  equ 0xF102C               ; Start address low

; Graphics Controller (0x3CE/0x3CF equivalent)
VGA_GC_INDEX      equ 0xF1030               ; GC index
VGA_GC_DATA       equ 0xF1034               ; GC data
VGA_GC_READMAP    equ 0xF1038               ; Read plane select (direct)
VGA_GC_BITMASK    equ 0xF103C               ; Bit mask (direct)

; Attribute Controller
VGA_ATTR_INDEX    equ 0xF1040               ; Attribute index
VGA_ATTR_DATA     equ 0xF1044               ; Attribute data

; DAC/Palette (0x3C6-0x3C9 equivalent)
VGA_DAC_MASK      equ 0xF1050               ; Pixel mask
VGA_DAC_RINDEX    equ 0xF1054               ; Read index
VGA_DAC_WINDEX    equ 0xF1058               ; Write index
VGA_DAC_DATA      equ 0xF105C               ; DAC data (R,G,B sequence)

; Palette RAM (256 x 3 bytes)
VGA_PALETTE       equ 0xF1100               ; Palette base (768 bytes)

; VGA VRAM (64KB window)
VGA_VRAM          equ 0xA0000               ; VRAM window (64KB)

; Mode constants
VGA_MODE_TEXT     equ 0x03                   ; 80x25 text mode
VGA_MODE_12H      equ 0x12                  ; 640x480, 16 colors
VGA_MODE_13H      equ 0x13                  ; 320x200, 256 colors
VGA_MODE_X        equ 0x14                  ; 320x240, 256 colors

; Status bits
VGA_STATUS_VSYNC  equ 0x01                  ; VSync active
VGA_STATUS_RETRACE equ 0x08                 ; Vertical retrace

; Control bits
VGA_CTRL_ENABLE   equ 0x01                  ; VGA enable

; ============================================================================
; CUSTOM AUDIO CHIP REGISTERS
; ============================================================================

; Global Audio Control (0xF0800-0xF083F)
AUDIO_CTRL        equ 0xF0800               ; Master audio control
ENV_SHAPE         equ 0xF0804               ; Global envelope shape

; Filter registers
FILTER_CUTOFF     equ 0xF0820               ; Cutoff frequency (0-255, exponential 20Hz-20kHz)
FILTER_RESONANCE  equ 0xF0824               ; Resonance/Q (0-255)
FILTER_TYPE       equ 0xF0828               ; Filter type (0=off, 1=LP, 2=HP, 3=BP)
FILTER_MOD_SOURCE equ 0xF082C               ; Modulation source channel (0-3)
FILTER_MOD_AMOUNT equ 0xF0830               ; Modulation depth (0-255)

; Filter type constants
FILTER_OFF        equ 0                     ; Filter bypassed
FILTER_LOWPASS    equ 1                     ; Low-pass filter
FILTER_HIGHPASS   equ 2                     ; High-pass filter
FILTER_BANDPASS   equ 3                     ; Band-pass filter

; Envelope shape constants
ENV_SHAPE_ADSR      equ 0                   ; Standard ADSR
ENV_SHAPE_SAW_UP    equ 1                   ; Linear rise to max, then hold
ENV_SHAPE_SAW_DOWN  equ 2                   ; Linear fall to zero, then hold
ENV_SHAPE_LOOP      equ 3                   ; ADSR but loops after release
ENV_SHAPE_SID       equ 4                   ; SID-style exponential ADSR

; Square Wave Channel (0xF0900-0xF093F)
SQUARE_FREQ       equ 0xF0900               ; Frequency (16.8 fixed-point Hz, value = Hz * 256)
SQUARE_VOL        equ 0xF0904               ; Volume (0-255)
SQUARE_CTRL       equ 0xF0908               ; Control (bit 1 = gate)
SQUARE_DUTY       equ 0xF090C               ; Duty cycle (0-255, 128 = 50%)
SQUARE_SWEEP      equ 0xF0910               ; Sweep control
SQUARE_PWM_CTRL   equ 0xF0922               ; PWM control (bit 7 = enable, bits 0-6 = rate)
SQUARE_ATK        equ 0xF0930               ; Attack time (ms)
SQUARE_DEC        equ 0xF0934               ; Decay time (ms)
SQUARE_SUS        equ 0xF0938               ; Sustain level (0-255)
SQUARE_REL        equ 0xF093C               ; Release time (ms)

; Triangle Wave Channel (0xF0940-0xF097F)
TRI_FREQ          equ 0xF0940               ; Frequency (16.8 fixed-point Hz, value = Hz * 256)
TRI_VOL           equ 0xF0944               ; Volume (0-255)
TRI_CTRL          equ 0xF0948               ; Control (bit 1 = gate)
TRI_SWEEP         equ 0xF0914               ; Sweep control
TRI_ATK           equ 0xF0960               ; Attack time (ms)
TRI_DEC           equ 0xF0964               ; Decay time (ms)
TRI_SUS           equ 0xF0968               ; Sustain level (0-255)
TRI_REL           equ 0xF096C               ; Release time (ms)

; Sine Wave Channel (0xF0980-0xF09BF)
SINE_FREQ         equ 0xF0980               ; Frequency (16.8 fixed-point Hz, value = Hz * 256)
SINE_VOL          equ 0xF0984               ; Volume (0-255)
SINE_CTRL         equ 0xF0988               ; Control (bit 1 = gate)
SINE_SWEEP        equ 0xF0918               ; Sweep control
SINE_ATK          equ 0xF0990               ; Attack time (ms)
SINE_DEC          equ 0xF0994               ; Decay time (ms)
SINE_SUS          equ 0xF0998               ; Sustain level (0-255)
SINE_REL          equ 0xF099C               ; Release time (ms)

; Noise Channel (0xF09C0-0xF09FF)
NOISE_FREQ        equ 0xF09C0               ; Frequency/pitch (16.8 fixed-point Hz, value = Hz * 256)
NOISE_VOL         equ 0xF09C4               ; Volume (0-255)
NOISE_CTRL        equ 0xF09C8               ; Control (bit 1 = gate)
NOISE_ATK         equ 0xF09D0               ; Attack time (ms)
NOISE_DEC         equ 0xF09D4               ; Decay time (ms)
NOISE_SUS         equ 0xF09D8               ; Sustain level (0-255)
NOISE_REL         equ 0xF09DC               ; Release time (ms)
NOISE_MODE        equ 0xF09E0               ; Noise type

; Noise mode constants
NOISE_MODE_WHITE    equ 0                   ; White noise (LFSR)
NOISE_MODE_PERIODIC equ 1                   ; Periodic/looping noise
NOISE_MODE_METALLIC equ 2                   ; Metallic noise
NOISE_MODE_PSG      equ 3                   ; PSG-style (AY/YM) LFSR

; Modulation Registers (0xF0A00-0xF0A1F)
SYNC_SOURCE_CH0   equ 0xF0A00               ; Sync source for channel 0 (0-3, or 0xFF=off)
SYNC_SOURCE_CH1   equ 0xF0A04               ; Sync source for channel 1
SYNC_SOURCE_CH2   equ 0xF0A08               ; Sync source for channel 2
SYNC_SOURCE_CH3   equ 0xF0A0C               ; Sync source for channel 3

RING_MOD_SOURCE_CH0 equ 0xF0A10             ; Ring mod source for channel 0
RING_MOD_SOURCE_CH1 equ 0xF0A14             ; Ring mod source for channel 1
RING_MOD_SOURCE_CH2 equ 0xF0A18             ; Ring mod source for channel 2
RING_MOD_SOURCE_CH3 equ 0xF0A1C             ; Ring mod source for channel 3

; Sawtooth Wave Channel (0xF0A20-0xF0A6F)
SAW_FREQ          equ 0xF0A20               ; Frequency (16.8 fixed-point Hz, value = Hz * 256)
SAW_VOL           equ 0xF0A24               ; Volume (0-255)
SAW_CTRL          equ 0xF0A28               ; Control (bit 1 = gate)
SAW_SWEEP         equ 0xF0A2C               ; Sweep control
SAW_ATK           equ 0xF0A30               ; Attack time (ms)
SAW_DEC           equ 0xF0A34               ; Decay time (ms)
SAW_SUS           equ 0xF0A38               ; Sustain level (0-255)
SAW_REL           equ 0xF0A3C               ; Release time (ms)
SYNC_SOURCE_CH4   equ 0xF0A60               ; Sync source for sawtooth
RING_MOD_SOURCE_CH4 equ 0xF0A64             ; Ring mod source for sawtooth

; Effects Registers (0xF0A40-0xF0A5F)
OVERDRIVE_CTRL    equ 0xF0A40               ; Drive amount (0-255 -> 0.0-4.0x gain)
REVERB_MIX        equ 0xF0A50               ; Dry/wet mix (0-255, 0=dry, 255=wet)
REVERB_DECAY      equ 0xF0A54               ; Decay time (0-255 -> 0.1-0.99)

; Flexible 4-Channel Synth Block (0xF0A80-0xF0B7F)
FLEX_CH_BASE      equ 0xF0A80               ; Base address of flexible block
FLEX_CH_STRIDE    equ 0x40                   ; Bytes per channel (64)

FLEX_CH0_BASE     equ 0xF0A80               ; Channel 0 base
FLEX_CH1_BASE     equ 0xF0AC0               ; Channel 1 base
FLEX_CH2_BASE     equ 0xF0B00               ; Channel 2 base
FLEX_CH3_BASE     equ 0xF0B40               ; Channel 3 base

; Register offsets within each flexible channel (add to FLEX_CHx_BASE)
FLEX_OFF_FREQ       equ 0x00                ; Frequency (16.8 fixed-point Hz, value = Hz * 256)
FLEX_OFF_VOL        equ 0x04                ; Volume (0-255)
FLEX_OFF_CTRL       equ 0x08                ; Control (bit 1 = gate)
FLEX_OFF_DUTY       equ 0x0C                ; Duty cycle for square wave
FLEX_OFF_SWEEP      equ 0x10                ; Sweep control
FLEX_OFF_ATK        equ 0x14                ; Attack time (ms)
FLEX_OFF_DEC        equ 0x18                ; Decay time (ms)
FLEX_OFF_SUS        equ 0x1C                ; Sustain level (0-255)
FLEX_OFF_REL        equ 0x20                ; Release time (ms)
FLEX_OFF_WAVE_TYPE  equ 0x24                ; Waveform type (0-4)
FLEX_OFF_PWM_CTRL   equ 0x28                ; PWM control
FLEX_OFF_NOISEMODE  equ 0x2C                ; Noise mode (if wave type is noise)

; Waveform type constants (for FLEX_OFF_WAVE_TYPE)
WAVE_SQUARE       equ 0                     ; Square wave with PWM
WAVE_TRIANGLE     equ 1                     ; Triangle wave
WAVE_SINE         equ 2                     ; Sine wave
WAVE_NOISE        equ 3                     ; Noise
WAVE_SAWTOOTH     equ 4                     ; Sawtooth wave

; Control Register Bit Masks
CTRL_GATE         equ 0x02                  ; Gate bit (trigger envelope)
PWM_ENABLE        equ 0x80                  ; PWM enable (bit 7)
PWM_RATE_MASK     equ 0x7F                  ; PWM rate (bits 0-6)
SWEEP_ENABLE      equ 0x80                  ; Sweep enable (bit 7)
SWEEP_DIR_UP      equ 0x00                  ; Sweep direction up
SWEEP_DIR_DOWN    equ 0x08                  ; Sweep direction down (bit 3)
SWEEP_PERIOD_MASK equ 0x70                  ; Sweep period (bits 4-6)
SWEEP_SHIFT_MASK  equ 0x07                  ; Sweep shift (bits 0-2)

; ============================================================================
; VIDEO CONSTANTS
; ============================================================================

VRAM_START        equ 0x100000              ; VRAM starts at 1MB
SCREEN_W          equ 640                   ; Default screen width
SCREEN_H          equ 480                   ; Default screen height
LINE_BYTES        equ 2560                  ; Bytes per scanline (640 * 4)

; Blitter operations
BLT_OP_COPY       equ 0                     ; Copy source to dest
BLT_OP_FILL       equ 1                     ; Fill rect with color
BLT_OP_LINE       equ 2                     ; Draw line
BLT_OP_MASKED     equ 3                     ; Copy with mask
BLT_OP_ALPHA      equ 4                     ; Alpha blending

; Background color (black with alpha)
BACKGROUND        equ 0xFF000000            ; BGRA: Black, full alpha

; ============================================================================
; COPPER OPCODES
; ============================================================================

COP_WAIT_MASK         equ 0x00000000
COP_WAIT_SCALE        equ 0x1000            ; Multiply Y position by this for WAIT

; Copper MOVE opcodes
COP_MOVE_RASTER_Y     equ 0x40120000
COP_MOVE_RASTER_H     equ 0x40130000
COP_MOVE_RASTER_COLOR equ 0x40140000
COP_MOVE_RASTER_CTRL  equ 0x40150000

; Copper END opcode
COP_END               equ 0xC0000000

; Copper SETBASE opcode
COP_SETBASE_OP        equ 0x80000000
COP_SETBASE_SHIFT     equ 2

; Pre-computed SETBASE instructions for common devices
COP_SETBASE_VIDEO     equ 0x8003C000        ; VIDEO_REG_BASE (0xF0000 >> 2)
COP_SETBASE_VGA       equ 0x8003C400        ; VGA_BASE (0xF1000 >> 2)
COP_SETBASE_VGA_DAC   equ 0x8003C416        ; VGA_DAC_WINDEX (0xF1058 >> 2)

; MOVE register indices after SETBASE to VGA_DAC_WINDEX
COP_MOVE_VGA_WINDEX   equ 0x40000000        ; Offset 0: DAC write index
COP_MOVE_VGA_DATA     equ 0x40010000        ; Offset 1: DAC data (R,G,B seq)

; ============================================================================
; TIMER REGISTERS
; ============================================================================

TIMER_CTRL        equ 0x00F800              ; Timer control
TIMER_COUNT       equ 0x00F804              ; Timer count
TIMER_RELOAD      equ 0x00F808              ; Timer reload value

; ============================================================================
; ZX SPECTRUM ULA VIDEO REGISTERS
; ============================================================================

ULA_BASE          equ 0xF2000               ; ULA register base
ULA_BORDER        equ 0xF2000               ; Border color (bits 0-2)
ULA_CTRL          equ 0xF2004               ; Control register
ULA_STATUS        equ 0xF2008               ; Status register

; Control register bits
ULA_CTRL_ENABLE   equ 0x01                  ; ULA enable bit

; VRAM layout (same as authentic ZX Spectrum)
ULA_VRAM          equ 0x4000                ; VRAM base address
ULA_BITMAP_SIZE   equ 6144                  ; Bitmap: 256x192/8 = 6144 bytes
ULA_ATTR_OFFSET   equ 0x1800               ; Attribute offset from VRAM base
ULA_ATTR_SIZE     equ 768                   ; Attributes: 32x24 = 768 bytes
ULA_VRAM_SIZE     equ 6912                  ; Total: 6144 + 768 bytes

; Display dimensions
ULA_DISPLAY_W     equ 256                   ; Display width in pixels
ULA_DISPLAY_H     equ 192                   ; Display height in pixels
ULA_BORDER_SIZE   equ 32                    ; Border size in pixels
ULA_FRAME_W       equ 320                   ; Total frame width (256+64)
ULA_FRAME_H       equ 256                   ; Total frame height (192+64)
ULA_CELLS_X       equ 32                    ; Character cells horizontally
ULA_CELLS_Y       equ 24                    ; Character cells vertically

; ============================================================================
; ANTIC VIDEO REGISTERS (Atari 8-bit style)
; ============================================================================

ANTIC_BASE        equ 0xF2100               ; ANTIC register base
ANTIC_DMACTL      equ 0xF2100               ; DMA control
ANTIC_CHACTL      equ 0xF2104               ; Character control
ANTIC_DLISTL      equ 0xF2108               ; Display list pointer low
ANTIC_DLISTH      equ 0xF210C               ; Display list pointer high
ANTIC_HSCROL      equ 0xF2110               ; Horizontal scroll (0-15)
ANTIC_VSCROL      equ 0xF2114               ; Vertical scroll (0-15)
ANTIC_PMBASE      equ 0xF2118               ; Player-missile base address
ANTIC_CHBASE      equ 0xF211C               ; Character set base address
ANTIC_WSYNC       equ 0xF2120               ; Wait for sync (write only)
ANTIC_VCOUNT      equ 0xF2124               ; Vertical counter (read only)
ANTIC_PENH        equ 0xF2128               ; Light pen H (read only)
ANTIC_PENV        equ 0xF212C               ; Light pen V (read only)
ANTIC_NMIEN       equ 0xF2130               ; NMI enable
ANTIC_NMIST       equ 0xF2134               ; NMI status (read) / NMIRES (write)
ANTIC_ENABLE      equ 0xF2138               ; Video enable (IE-specific)
ANTIC_STATUS      equ 0xF213C               ; Status (VBlank flag)

; DMACTL bits
ANTIC_DMA_NARROW  equ 0x01                  ; Narrow playfield
ANTIC_DMA_NORMAL  equ 0x02                  ; Normal playfield
ANTIC_DMA_WIDE    equ 0x03                  ; Wide playfield
ANTIC_DMA_MISSILE equ 0x04                  ; Enable missile DMA
ANTIC_DMA_PLAYER  equ 0x08                  ; Enable player DMA
ANTIC_DMA_PMRES   equ 0x10                  ; Player/missile resolution
ANTIC_DMA_DL      equ 0x20                  ; Enable display list DMA

; CHACTL bits
ANTIC_CHACTL_BLANK   equ 0x01               ; Blank instead of inverse
ANTIC_CHACTL_INVERT  equ 0x02               ; Invert video
ANTIC_CHACTL_REFLECT equ 0x04               ; Reflect characters

; NMIEN bits
ANTIC_NMIEN_DLI   equ 0x80                  ; Display list interrupt enable
ANTIC_NMIEN_VBI   equ 0x40                  ; Vertical blank interrupt enable

; Display List Instructions
DL_BLANK1         equ 0x00                  ; 1 blank scanline
DL_BLANK2         equ 0x10                  ; 2 blank scanlines
DL_BLANK3         equ 0x20                  ; 3 blank scanlines
DL_BLANK4         equ 0x30                  ; 4 blank scanlines
DL_BLANK5         equ 0x40                  ; 5 blank scanlines
DL_BLANK6         equ 0x50                  ; 6 blank scanlines
DL_BLANK7         equ 0x60                  ; 7 blank scanlines
DL_BLANK8         equ 0x70                  ; 8 blank scanlines

; Jump instructions
DL_JMP            equ 0x01                  ; Jump to address (2 bytes follow)
DL_JVB            equ 0x41                  ; Jump and wait for Vertical Blank

; Graphics modes
DL_MODE2          equ 0x02                  ; 40 col text, 8 scanlines/row
DL_MODE3          equ 0x03                  ; 40 col text, 10 scanlines/row
DL_MODE4          equ 0x04                  ; 40 col text, 8 scanlines, multicolor
DL_MODE5          equ 0x05                  ; 40 col text, 16 scanlines, multicolor
DL_MODE6          equ 0x06                  ; 20 col text, 8 scanlines
DL_MODE7          equ 0x07                  ; 20 col text, 16 scanlines
DL_MODE8          equ 0x08                  ; 40 pixels, 8 scanlines/row (GRAPHICS 3)
DL_MODE9          equ 0x09                  ; 80 pixels, 4 scanlines (GRAPHICS 4)
DL_MODE10         equ 0x0A                  ; 80 pixels, 2 scanlines (GRAPHICS 5)
DL_MODE11         equ 0x0B                  ; 160 pixels, 1 scanline (GRAPHICS 6)
DL_MODE12         equ 0x0C                  ; 160 pixels, 1 scanline (GRAPHICS 6+)
DL_MODE13         equ 0x0D                  ; 160 pixels, 2 scanlines (GRAPHICS 7)
DL_MODE14         equ 0x0E                  ; 160 pixels, 1 scanline, 4 colors
DL_MODE15         equ 0x0F                  ; 320 pixels, 1 scanline (GRAPHICS 8)

; Display list modifiers (OR with mode)
DL_LMS            equ 0x40                  ; Load Memory Scan (2 addr bytes follow)
DL_DLI            equ 0x80                  ; Display List Interrupt at end of line
DL_HSCROL         equ 0x10                  ; Horizontal scroll enable
DL_VSCROL         equ 0x20                  ; Vertical scroll enable

; Enable/Status bits
ANTIC_ENABLE_VIDEO   equ 0x01               ; Bit 0: Video enable
ANTIC_STATUS_VBLANK  equ 0x01               ; Bit 0: VBlank active

; Display dimensions
ANTIC_DISPLAY_W   equ 320                   ; Display width in pixels
ANTIC_DISPLAY_H   equ 192                   ; Display height in pixels
ANTIC_FRAME_W     equ 384                   ; Total frame width (with border)
ANTIC_FRAME_H     equ 240                   ; Total frame height (with border)

; Copper SETBASE for ANTIC video
COP_SETBASE_ANTIC equ 0x8003C840            ; ANTIC_BASE (0xF2100 >> 2)

; ============================================================================
; GTIA VIDEO REGISTERS (Atari 8-bit color control)
; ============================================================================

GTIA_BASE         equ 0xF2140               ; GTIA register base

; Playfield color registers
GTIA_COLPF0       equ 0xF2140               ; Playfield color 0
GTIA_COLPF1       equ 0xF2144               ; Playfield color 1
GTIA_COLPF2       equ 0xF2148               ; Playfield color 2
GTIA_COLPF3       equ 0xF214C               ; Playfield color 3
GTIA_COLBK        equ 0xF2150               ; Background/border color

; Player color registers
GTIA_COLPM0       equ 0xF2154               ; Player/missile 0 color
GTIA_COLPM1       equ 0xF2158               ; Player/missile 1 color
GTIA_COLPM2       equ 0xF215C               ; Player/missile 2 color
GTIA_COLPM3       equ 0xF2160               ; Player/missile 3 color

; GTIA control registers
GTIA_PRIOR        equ 0xF2164               ; Priority and GTIA modes
GTIA_GRACTL       equ 0xF2168               ; Graphics control
GTIA_CONSOL       equ 0xF216C               ; Console switches (read)

; Player horizontal position registers
GTIA_HPOSP0       equ 0xF2170               ; Player 0 horizontal position
GTIA_HPOSP1       equ 0xF2174               ; Player 1 horizontal position
GTIA_HPOSP2       equ 0xF2178               ; Player 2 horizontal position
GTIA_HPOSP3       equ 0xF217C               ; Player 3 horizontal position

; Missile horizontal position registers
GTIA_HPOSM0       equ 0xF2180               ; Missile 0 horizontal position
GTIA_HPOSM1       equ 0xF2184               ; Missile 1 horizontal position
GTIA_HPOSM2       equ 0xF2188               ; Missile 2 horizontal position
GTIA_HPOSM3       equ 0xF218C               ; Missile 3 horizontal position

; Player/missile size registers
GTIA_SIZEP0       equ 0xF2190               ; Player 0 size (0=normal,1=double,3=quad)
GTIA_SIZEP1       equ 0xF2194               ; Player 1 size
GTIA_SIZEP2       equ 0xF2198               ; Player 2 size
GTIA_SIZEP3       equ 0xF219C               ; Player 3 size
GTIA_SIZEM        equ 0xF21A0               ; All missile sizes (2 bits each)

; Player graphics registers (directly writable)
GTIA_GRAFP0       equ 0xF21A4               ; Player 0 graphics (8 pixels)
GTIA_GRAFP1       equ 0xF21A8               ; Player 1 graphics
GTIA_GRAFP2       equ 0xF21AC               ; Player 2 graphics
GTIA_GRAFP3       equ 0xF21B0               ; Player 3 graphics
GTIA_GRAFM        equ 0xF21B4               ; Missile graphics (2 bits each)

; PRIOR register bits
GTIA_PRIOR_P03    equ 0x01                  ; Players 0-3 have priority over PF
GTIA_PRIOR_P01    equ 0x02                  ; Players 0-1 have priority
GTIA_PRIOR_P23    equ 0x04                  ; Players 2-3 have priority
GTIA_PRIOR_MULTI  equ 0x10                  ; Enable multicolor players
GTIA_PRIOR_FIFTH  equ 0x20                  ; Enable fifth player
GTIA_PRIOR_GTIA1  equ 0x40                  ; GTIA mode bit 0
GTIA_PRIOR_GTIA2  equ 0x80                  ; GTIA mode bit 1

; GRACTL register bits
GTIA_GRACTL_MISSLE equ 0x01                 ; Enable missile graphics
GTIA_GRACTL_PLAYER equ 0x02                 ; Enable player graphics
GTIA_GRACTL_LATCH  equ 0x04                 ; Latch trigger inputs

; GTIA register end
GTIA_END          equ 0xF21B7

; Copper SETBASE for GTIA
COP_SETBASE_GTIA  equ 0x8003C850            ; GTIA_BASE (0xF2140 >> 2)

; ============================================================================
; 3DFX VOODOO GRAPHICS (SST-1) REGISTERS
; ============================================================================

; Base address
VOODOO_BASE           equ 0xF4000

; Status register (read-only)
VOODOO_STATUS         equ 0xF4000

; Enable register (write-only)
VOODOO_ENABLE         equ 0xF4004

; Vertex coordinate registers (12.4 fixed-point)
VOODOO_VERTEX_AX      equ 0xF4008
VOODOO_VERTEX_AY      equ 0xF400C
VOODOO_VERTEX_BX      equ 0xF4010
VOODOO_VERTEX_BY      equ 0xF4014
VOODOO_VERTEX_CX      equ 0xF4018
VOODOO_VERTEX_CY      equ 0xF401C

; Vertex attribute start values (12.12 fixed-point)
VOODOO_START_R        equ 0xF4020
VOODOO_START_G        equ 0xF4024
VOODOO_START_B        equ 0xF4028
VOODOO_START_Z        equ 0xF402C
VOODOO_START_A        equ 0xF4030
VOODOO_START_S        equ 0xF4034
VOODOO_START_T        equ 0xF4038
VOODOO_START_W        equ 0xF403C

; Vertex attribute gradients (per-scanline deltas for interpolation)
VOODOO_DRDX           equ 0xF4040     ; dR/dX
VOODOO_DGDX           equ 0xF4044     ; dG/dX
VOODOO_DBDX           equ 0xF4048     ; dB/dX
VOODOO_DZDX           equ 0xF404C     ; dZ/dX
VOODOO_DADX           equ 0xF4050     ; dA/dX
VOODOO_DSDX           equ 0xF4054     ; dS/dX
VOODOO_DTDX           equ 0xF4058     ; dT/dX
VOODOO_DWDX           equ 0xF405C     ; dW/dX
VOODOO_DRDY           equ 0xF4060     ; dR/dY
VOODOO_DGDY           equ 0xF4064     ; dG/dY
VOODOO_DBDY           equ 0xF4068     ; dB/dY
VOODOO_DZDY           equ 0xF406C     ; dZ/dY
VOODOO_DADY           equ 0xF4070     ; dA/dY
VOODOO_DSDY           equ 0xF4074     ; dS/dY
VOODOO_DTDY           equ 0xF4078     ; dT/dY
VOODOO_DWDY           equ 0xF407C     ; dW/dY

; Command registers
VOODOO_TRIANGLE_CMD   equ 0xF4080
VOODOO_COLOR_SELECT   equ 0xF4088
VOODOO_FBZCOLOR_PATH  equ 0xF4104
VOODOO_FOG_MODE       equ 0xF4108
VOODOO_ALPHA_MODE     equ 0xF410C
VOODOO_FBZ_MODE       equ 0xF4110
VOODOO_LFB_MODE       equ 0xF4114
VOODOO_CLIP_LEFT_RIGHT equ 0xF4118
VOODOO_CLIP_LOW_Y_HIGH equ 0xF411C
VOODOO_NOP_CMD        equ 0xF4120
VOODOO_FAST_FILL_CMD  equ 0xF4124
VOODOO_SWAP_BUFFER_CMD equ 0xF4128

; Configuration registers
VOODOO_FOG_COLOR      equ 0xF41C4
VOODOO_ZA_COLOR       equ 0xF41C8
VOODOO_CHROMA_KEY     equ 0xF41CC
VOODOO_COLOR0         equ 0xF41D8
VOODOO_COLOR1         equ 0xF41DC
VOODOO_VIDEO_DIM      equ 0xF4214

; Texture registers
VOODOO_TEXTURE_MODE   equ 0xF4300
VOODOO_TEX_BASE0      equ 0xF430C
VOODOO_TEX_WIDTH      equ 0xF4330
VOODOO_TEX_HEIGHT     equ 0xF4334
VOODOO_TEX_UPLOAD     equ 0xF4338
VOODOO_TEXMEM_BASE    equ 0xF5000

; Status register bits
VOODOO_STATUS_FBI_BUSY    equ 0x01
VOODOO_STATUS_TMU_BUSY    equ 0x02
VOODOO_STATUS_SST_BUSY    equ 0x04
VOODOO_STATUS_VRETRACE    equ 0x40
VOODOO_STATUS_SWAPBUF     equ 0x80

; fbzMode bits
VOODOO_FBZ_CLIPPING     equ 0x0001
VOODOO_FBZ_CHROMAKEY    equ 0x0002
VOODOO_FBZ_DEPTH_ENABLE equ 0x0010
VOODOO_FBZ_DEPTH_FUNC   equ 0x00E0
VOODOO_FBZ_RGB_WRITE    equ 0x0200
VOODOO_FBZ_DEPTH_WRITE  equ 0x0400

; Depth compare functions (shift left 5 to use in FBZ_MODE)
VOODOO_DEPTH_NEVER        equ 0
VOODOO_DEPTH_LESS         equ 1
VOODOO_DEPTH_EQUAL        equ 2
VOODOO_DEPTH_LESSEQUAL    equ 3
VOODOO_DEPTH_GREATER      equ 4
VOODOO_DEPTH_NOTEQUAL     equ 5
VOODOO_DEPTH_GREATEREQUAL equ 6
VOODOO_DEPTH_ALWAYS       equ 7

; alphaMode bits
VOODOO_ALPHA_TEST_EN    equ 0x0001
VOODOO_ALPHA_FUNC_MASK  equ 0x000E
VOODOO_ALPHA_FUNC_SHIFT equ 1
VOODOO_ALPHA_BLEND_EN   equ 0x0010
VOODOO_ALPHA_SRC_RGB    equ 0x0F00
VOODOO_ALPHA_DST_RGB    equ 0xF000
VOODOO_ALPHA_REF_SHIFT  equ 24

; Alpha test functions
VOODOO_ALPHA_NEVER        equ 0
VOODOO_ALPHA_LESS         equ 1
VOODOO_ALPHA_EQUAL        equ 2
VOODOO_ALPHA_LESSEQUAL    equ 3
VOODOO_ALPHA_GREATER      equ 4
VOODOO_ALPHA_NOTEQUAL     equ 5
VOODOO_ALPHA_GREATEREQUAL equ 6
VOODOO_ALPHA_ALWAYS       equ 7

; Alpha blend factors
VOODOO_BLEND_ZERO       equ 0
VOODOO_BLEND_SRC_ALPHA  equ 1
VOODOO_BLEND_COLOR      equ 2
VOODOO_BLEND_DST_ALPHA  equ 3
VOODOO_BLEND_ONE        equ 4
VOODOO_BLEND_INV_SRC_A  equ 5
VOODOO_BLEND_INV_COLOR  equ 6
VOODOO_BLEND_INV_DST_A  equ 7
VOODOO_BLEND_SATURATE   equ 15

; swapbufferCMD bits
VOODOO_SWAP_VSYNC       equ 0x01
VOODOO_SWAP_CLEAR       equ 0x02

; textureMode bits
VOODOO_TEX_ENABLE       equ 0x0001
VOODOO_TEX_MINIFY       equ 0x000E
VOODOO_TEX_MAGNIFY      equ 0x0010
VOODOO_TEX_CLAMP_S      equ 0x0020
VOODOO_TEX_CLAMP_T      equ 0x0040
VOODOO_TEX_FORMAT       equ 0x0F00
VOODOO_TEX_FORMAT_SHIFT equ 8

; Texture format constants
VOODOO_TEX_FMT_PALETTE  equ 0
VOODOO_TEX_FMT_P8       equ 5
VOODOO_TEX_FMT_ARGB1555 equ 8
VOODOO_TEX_FMT_ARGB4444 equ 9
VOODOO_TEX_FMT_ARGB8888 equ 10

; Clip register packing
VOODOO_CLIP_MASK        equ 0x3FF

; Fixed-point shift amounts
VOODOO_FIXED_12_4       equ 4
VOODOO_FIXED_12_12      equ 12

; fbzColorPath bit fields
VOODOO_FCP_RGB_SELECT_MASK  equ 0x03
VOODOO_FCP_RGB_SELECT_SHIFT equ 0
VOODOO_FCP_A_SELECT_MASK    equ 0x0C
VOODOO_FCP_A_SELECT_SHIFT   equ 2
VOODOO_FCP_CC_MSELECT_MASK  equ 0x70
VOODOO_FCP_CC_MSELECT_SHIFT equ 4
VOODOO_FCP_TEXTURE_ENABLE   equ 0x08000000

; Color source select values
VOODOO_CC_ITERATED      equ 0
VOODOO_CC_TEXTURE       equ 1
VOODOO_CC_COLOR1        equ 2
VOODOO_CC_LFB           equ 3

; Color combine function modes
VOODOO_CC_ZERO          equ 0
VOODOO_CC_CSUB_CL       equ 1
VOODOO_CC_ALOCAL        equ 2
VOODOO_CC_AOTHER        equ 3
VOODOO_CC_CLOCAL        equ 4
VOODOO_CC_ALOCAL_T      equ 5
VOODOO_CC_CLOC_MUL      equ 6
VOODOO_CC_AOTHER_T      equ 7

; Simplified color combine modes
VOODOO_COMBINE_UNSET    equ 0xFFFFFFFF
VOODOO_COMBINE_ITERATED equ 0x00
VOODOO_COMBINE_TEXTURE  equ 0x01
VOODOO_COMBINE_MODULATE equ 0x61
VOODOO_COMBINE_ADD      equ 0x81
VOODOO_COMBINE_DECAL    equ 0x41

; fogMode bit fields
VOODOO_FOG_ENABLE       equ 0x0001
VOODOO_FOG_ADD          equ 0x0002
VOODOO_FOG_MULT         equ 0x0004
VOODOO_FOG_ZALPHA       equ 0x0008
VOODOO_FOG_CONSTANT     equ 0x0010
VOODOO_FOG_DITHER       equ 0x0020
VOODOO_FOG_ZONES        equ 0x0040
VOODOO_FOG_TABLE_SHIFT  equ 8
VOODOO_FOG_TABLE_MASK   equ 0x3F

; Fog table constants
VOODOO_FOG_TABLE_BASE   equ 0xF4140
VOODOO_FOG_TABLE_SIZE   equ 64

; fbzMode dithering bits
VOODOO_FBZ_DITHER       equ 0x0100
VOODOO_FBZ_DITHER_2X2   equ 0x0800

; ============================================================================
; MACROS - Video helpers
; ============================================================================

; Wait for VBlank (matches ie68.inc wait_vblank - polls VIDEO_STATUS .l)
wait_vblank macro
.wait_not_vblank:
    la      r1, VIDEO_STATUS
    load.l  r2, (r1)
    and.l   r2, r2, #STATUS_VBLANK
    bnez    r2, .wait_not_vblank
.wait_vblank_poll:
    load.l  r2, (r1)
    and.l   r2, r2, #STATUS_VBLANK
    beqz    r2, .wait_vblank_poll
    endm

; Wait for blitter to finish (polls BLT_CTRL .l)
wait_blit macro
.wait_blit_poll:
    la      r1, BLT_CTRL
    load.l  r2, (r1)
    and.l   r2, r2, #2
    bnez    r2, .wait_blit_poll
    endm

; Start a blitter operation (.l)
start_blit macro
    la      r1, BLT_CTRL
    li      r2, #1
    store.l r2, (r1)
    endm

; Set blitter fill color (.l)
set_blt_color macro
    la      r1, BLT_COLOR
    li      r2, #\1
    store.l r2, (r1)
    endm

; Set blitter source address (.l)
set_blt_src macro
    la      r1, BLT_SRC
    li      r2, #\1
    store.l r2, (r1)
    endm

; Set blitter dest address (.l)
set_blt_dst macro
    la      r1, BLT_DST
    li      r2, #\1
    store.l r2, (r1)
    endm

; Set blitter dimensions (.l)
set_blt_size macro
    la      r1, BLT_WIDTH
    li      r2, #\1
    store.l r2, (r1)
    la      r1, BLT_HEIGHT
    li      r2, #\2
    store.l r2, (r1)
    endm

; Set blitter strides (.l)
set_blt_strides macro
    la      r1, BLT_SRC_STRIDE
    li      r2, #\1
    store.l r2, (r1)
    la      r1, BLT_DST_STRIDE
    li      r2, #\2
    store.l r2, (r1)
    endm

; ============================================================================
; MACROS - VGA helpers
; ============================================================================

; Set VGA mode (.b)
vga_setmode macro
    la      r1, VGA_MODE
    li      r2, #\1
    store.b r2, (r1)
    endm

; Enable VGA (.b)
vga_enable macro
    la      r1, VGA_CTRL
    li      r2, #VGA_CTRL_ENABLE
    store.b r2, (r1)
    endm

; Set palette write index (.b)
vga_setpalette macro
    la      r1, VGA_DAC_WINDEX
    li      r2, #\1
    store.b r2, (r1)
    endm

; Write palette RGB values (.b x3)
vga_palette_rgb macro
    la      r1, VGA_DAC_DATA
    li      r2, #\1
    store.b r2, (r1)
    li      r2, #\2
    store.b r2, (r1)
    li      r2, #\3
    store.b r2, (r1)
    endm

; Wait for VSync (.b)
vga_wait_vsync macro
.wait_vga_vs:
    la      r1, VGA_STATUS
    load.b  r2, (r1)
    and.b   r2, r2, #VGA_STATUS_VSYNC
    beqz    r2, .wait_vga_vs
    endm

; Set map mask (planes to write) (.b)
vga_mapmask macro
    la      r1, VGA_SEQ_MAPMASK
    li      r2, #\1
    store.b r2, (r1)
    endm

; Set read map (plane to read) (.b)
vga_readmap macro
    la      r1, VGA_GC_READMAP
    li      r2, #\1
    store.b r2, (r1)
    endm

; ============================================================================
; MACROS - ULA helpers
; ============================================================================

; Set ULA border color (.b)
set_ula_border macro
    la      r1, ULA_BORDER
    li      r2, #\1
    store.b r2, (r1)
    endm

; Enable ULA (.b)
ula_enable macro
    la      r1, ULA_CTRL
    li      r2, #ULA_CTRL_ENABLE
    store.b r2, (r1)
    endm

; Disable ULA (.b)
ula_disable macro
    la      r1, ULA_CTRL
    store.b r0, (r1)
    endm

; ============================================================================
; MACROS - TED video helpers
; ============================================================================

; Enable TED video (.b)
set_ted_v_enable macro
    la      r1, TED_V_ENABLE
    li      r2, #TED_V_ENABLE_VIDEO
    store.b r2, (r1)
    endm

; Disable TED video (.b)
ted_v_disable macro
    la      r1, TED_V_ENABLE
    store.b r0, (r1)
    endm

; Set TED border color (.b) - hue 0-15, luminance 0-7
set_ted_v_border macro
    la      r1, TED_V_BORDER
    li      r2, #((\2&7)<<4)|(\1&0x0F)
    store.b r2, (r1)
    endm

; Set TED background color 0 (.b) - hue 0-15, luminance 0-7
ted_v_bgcolor macro
    la      r1, TED_V_BG_COLOR0
    li      r2, #((\2&7)<<4)|(\1&0x0F)
    store.b r2, (r1)
    endm

; Wait for TED VBlank (.b)
ted_v_wait_vblank macro
.wait_ted_vb:
    la      r1, TED_V_STATUS
    load.b  r2, (r1)
    and.b   r2, r2, #TED_V_STATUS_VBLANK
    beqz    r2, .wait_ted_vb
    endm

; ============================================================================
; MACROS - ANTIC/GTIA helpers
; ============================================================================

; Enable ANTIC video (.b)
set_antic_enable macro
    la      r1, ANTIC_ENABLE
    li      r2, #ANTIC_ENABLE_VIDEO
    store.b r2, (r1)
    endm

; Disable ANTIC video (.b)
antic_disable macro
    la      r1, ANTIC_ENABLE
    store.b r0, (r1)
    endm

; Wait for VBlank (.b)
antic_wait_vblank macro
.wvb:
    la      r1, ANTIC_STATUS
    load.b  r2, (r1)
    and.b   r2, r2, #ANTIC_STATUS_VBLANK
    beqz    r2, .wvb
    endm

; Set display list pointer (addr split into low/high bytes)
antic_set_dlist macro
    la      r1, ANTIC_DLISTL
    li      r2, #(\1)&0xFF
    store.b r2, (r1)
    la      r1, ANTIC_DLISTH
    li      r2, #((\1)>>8)&0xFF
    store.b r2, (r1)
    endm

; Set GTIA background color (.b)
gtia_setbk macro
    la      r1, GTIA_COLBK
    li      r2, #\1
    store.b r2, (r1)
    endm

; Set playfield color 0 (.b)
gtia_setpf0 macro
    la      r1, GTIA_COLPF0
    li      r2, #\1
    store.b r2, (r1)
    endm

; Set playfield color 1 (.b)
gtia_setpf1 macro
    la      r1, GTIA_COLPF1
    li      r2, #\1
    store.b r2, (r1)
    endm

; Set playfield color 2 (.b)
gtia_setpf2 macro
    la      r1, GTIA_COLPF2
    li      r2, #\1
    store.b r2, (r1)
    endm

; Set playfield color 3 (.b)
gtia_setpf3 macro
    la      r1, GTIA_COLPF3
    li      r2, #\1
    store.b r2, (r1)
    endm

; ============================================================================
; MACROS - Copper helpers
; ============================================================================

; Set copper list pointer (.l)
set_copper_ptr macro
    la      r1, COPPER_PTR
    li      r2, #\1
    store.l r2, (r1)
    endm

; Enable copper (.l)
enable_copper macro
    la      r1, COPPER_CTRL
    li      r2, #1
    store.l r2, (r1)
    endm

; Disable copper (.l)
disable_copper macro
    la      r1, COPPER_CTRL
    store.l r0, (r1)
    endm

; ============================================================================
; MACROS - PSG player helpers
; ============================================================================

; Enable PSG+ enhanced mode (.b)
enable_psg_plus macro
    la      r1, PSG_PLUS_CTRL
    li      r2, #1
    store.b r2, (r1)
    endm

; Set PSG play pointer and length (.l)
set_psg_play macro
    la      r1, PSG_PLAY_PTR
    li      r2, #\1
    store.l r2, (r1)
    la      r1, PSG_PLAY_LEN
    li      r2, #\2
    store.l r2, (r1)
    endm

; Start PSG playback (.l)
start_psg_play macro
    la      r1, PSG_PLAY_CTRL
    li      r2, #1
    store.l r2, (r1)
    endm

; Stop PSG playback (.l)
stop_psg_play macro
    la      r1, PSG_PLAY_CTRL
    store.l r0, (r1)
    endm

; ============================================================================
; MACROS - SID player helpers
; ============================================================================

; Enable SID+ enhanced mode (.b)
enable_sid_plus macro
    la      r1, SID_PLUS_CTRL
    li      r2, #1
    store.b r2, (r1)
    endm

; Set SID play pointer and length (.l)
set_sid_play macro
    la      r1, SID_PLAY_PTR
    li      r2, #\1
    store.l r2, (r1)
    la      r1, SID_PLAY_LEN
    li      r2, #\2
    store.l r2, (r1)
    endm

; Set SID subsong (.b)
set_sid_subsong macro
    la      r1, SID_SUBSONG
    li      r2, #\1
    store.b r2, (r1)
    endm

; Start SID playback (.l)
start_sid_play macro
    la      r1, SID_PLAY_CTRL
    li      r2, #1
    store.l r2, (r1)
    endm

; Start SID playback with looping (.l)
start_sid_loop macro
    la      r1, SID_PLAY_CTRL
    li      r2, #5
    store.l r2, (r1)
    endm

; Stop SID playback (.l)
stop_sid_play macro
    la      r1, SID_PLAY_CTRL
    li      r2, #2
    store.l r2, (r1)
    endm

; ============================================================================
; MACROS - SAP player helpers
; ============================================================================

; Set SAP play pointer and length (.l)
set_sap_play macro
    la      r1, SAP_PLAY_PTR
    li      r2, #\1
    store.l r2, (r1)
    la      r1, SAP_PLAY_LEN
    li      r2, #\2
    store.l r2, (r1)
    endm

; Set SAP subsong (.b)
set_sap_subsong macro
    la      r1, SAP_SUBSONG
    li      r2, #\1
    store.b r2, (r1)
    endm

; Start SAP playback (.l)
start_sap_play macro
    la      r1, SAP_PLAY_CTRL
    li      r2, #1
    store.l r2, (r1)
    endm

; Start SAP playback with looping (.l)
start_sap_loop macro
    la      r1, SAP_PLAY_CTRL
    li      r2, #5
    store.l r2, (r1)
    endm

; Stop SAP playback (.l)
stop_sap_play macro
    la      r1, SAP_PLAY_CTRL
    li      r2, #2
    store.l r2, (r1)
    endm

; ============================================================================
; MACROS - AHX player helpers
; ============================================================================

; Load and start AHX playback (.l)
PLAY_AHX macro
    la      r1, AHX_PLAY_PTR
    li      r2, #\1
    store.l r2, (r1)
    la      r1, AHX_PLAY_LEN
    li      r2, #\2
    store.l r2, (r1)
    la      r1, AHX_PLAY_CTRL
    li      r2, #1
    store.l r2, (r1)
    endm

; Load and start AHX playback with looping (.l)
PLAY_AHX_LOOP macro
    la      r1, AHX_PLAY_PTR
    li      r2, #\1
    store.l r2, (r1)
    la      r1, AHX_PLAY_LEN
    li      r2, #\2
    store.l r2, (r1)
    la      r1, AHX_PLAY_CTRL
    li      r2, #5
    store.l r2, (r1)
    endm

; Load and start AHX+ enhanced playback (.l)
PLAY_AHX_PLUS macro
    la      r1, AHX_PLUS_CTRL
    li      r2, #1
    store.l r2, (r1)
    la      r1, AHX_PLAY_PTR
    li      r2, #\1
    store.l r2, (r1)
    la      r1, AHX_PLAY_LEN
    li      r2, #\2
    store.l r2, (r1)
    la      r1, AHX_PLAY_CTRL
    li      r2, #1
    store.l r2, (r1)
    endm

; Load and start AHX+ enhanced playback with looping (.l)
PLAY_AHX_PLUS_LOOP macro
    la      r1, AHX_PLUS_CTRL
    li      r2, #1
    store.l r2, (r1)
    la      r1, AHX_PLAY_PTR
    li      r2, #\1
    store.l r2, (r1)
    la      r1, AHX_PLAY_LEN
    li      r2, #\2
    store.l r2, (r1)
    la      r1, AHX_PLAY_CTRL
    li      r2, #5
    store.l r2, (r1)
    endm

; Stop AHX playback (.l)
STOP_AHX macro
    la      r1, AHX_PLAY_CTRL
    li      r2, #2
    store.l r2, (r1)
    endm

; ============================================================================
; MACROS - POKEY helpers
; ============================================================================

; Enable POKEY+ enhanced mode (.b)
enable_pokey_plus macro
    la      r1, POKEY_PLUS_CTRL
    li      r2, #1
    store.b r2, (r1)
    endm

; Set POKEY master control (.b)
set_pokey_audctl macro
    la      r1, POKEY_AUDCTL
    li      r2, #\1
    store.b r2, (r1)
    endm

; Set POKEY channel (freq, control) (.b)
set_pokey_ch macro
    li      r1, #POKEY_AUDF1+(\1*2)
    li      r2, #\2
    store.b r2, (r1)
    li      r1, #POKEY_AUDC1+(\1*2)
    li      r2, #\3
    store.b r2, (r1)
    endm

; ============================================================================
; MACROS - Custom audio helpers
; ============================================================================

; Set channel frequency (16.8 fixed-point: Hz * 256) (.l)
set_ch_freq macro
    li      r1, #FLEX_CH0_BASE+(\1*FLEX_CH_STRIDE)+FLEX_OFF_FREQ
    li      r2, #(\2)*256
    store.l r2, (r1)
    endm

; Set channel volume (.l)
set_ch_vol macro
    li      r1, #FLEX_CH0_BASE+(\1*FLEX_CH_STRIDE)+FLEX_OFF_VOL
    li      r2, #\2
    store.l r2, (r1)
    endm

; Set channel waveform (.l)
set_ch_wave macro
    li      r1, #FLEX_CH0_BASE+(\1*FLEX_CH_STRIDE)+FLEX_OFF_WAVE_TYPE
    li      r2, #\2
    store.l r2, (r1)
    endm

; Set channel envelope (.l)
set_ch_env macro
    li      r1, #FLEX_CH0_BASE+(\1*FLEX_CH_STRIDE)+FLEX_OFF_ATK
    li      r2, #\2
    store.l r2, (r1)
    li      r1, #FLEX_CH0_BASE+(\1*FLEX_CH_STRIDE)+FLEX_OFF_DEC
    li      r2, #\3
    store.l r2, (r1)
    li      r1, #FLEX_CH0_BASE+(\1*FLEX_CH_STRIDE)+FLEX_OFF_SUS
    li      r2, #\4
    store.l r2, (r1)
    li      r1, #FLEX_CH0_BASE+(\1*FLEX_CH_STRIDE)+FLEX_OFF_REL
    li      r2, #\5
    store.l r2, (r1)
    endm

; Gate channel on (.l)
gate_ch_on macro
    li      r1, #FLEX_CH0_BASE+(\1*FLEX_CH_STRIDE)+FLEX_OFF_CTRL
    li      r2, #CTRL_GATE
    store.l r2, (r1)
    endm

; Gate channel off (.l)
gate_ch_off macro
    li      r1, #FLEX_CH0_BASE+(\1*FLEX_CH_STRIDE)+FLEX_OFF_CTRL
    store.l r0, (r1)
    endm

; Set filter parameters (.l)
set_filter macro
    la      r1, FILTER_CUTOFF
    li      r2, #\1
    store.l r2, (r1)
    la      r1, FILTER_RESONANCE
    li      r2, #\2
    store.l r2, (r1)
    la      r1, FILTER_TYPE
    li      r2, #\3
    store.l r2, (r1)
    endm

; Set reverb parameters (.l)
set_reverb macro
    la      r1, REVERB_MIX
    li      r2, #\1
    store.l r2, (r1)
    la      r1, REVERB_DECAY
    li      r2, #\2
    store.l r2, (r1)
    endm

; ============================================================================
; MACROS - Voodoo helpers
; ============================================================================

; Set vertex A coordinates (.l, 12.4 fixed-point)
voodoo_vertex_a macro
    la      r1, VOODOO_VERTEX_AX
    li      r2, #(\1<<4)
    store.l r2, (r1)
    la      r1, VOODOO_VERTEX_AY
    li      r2, #(\2<<4)
    store.l r2, (r1)
    endm

; Set vertex B coordinates (.l)
voodoo_vertex_b macro
    la      r1, VOODOO_VERTEX_BX
    li      r2, #(\1<<4)
    store.l r2, (r1)
    la      r1, VOODOO_VERTEX_BY
    li      r2, #(\2<<4)
    store.l r2, (r1)
    endm

; Set vertex C coordinates (.l)
voodoo_vertex_c macro
    la      r1, VOODOO_VERTEX_CX
    li      r2, #(\1<<4)
    store.l r2, (r1)
    la      r1, VOODOO_VERTEX_CY
    li      r2, #(\2<<4)
    store.l r2, (r1)
    endm

; Set color (.l, 0-255 -> 12.4 fixed-point)
voodoo_color_rgb macro
    la      r1, VOODOO_START_R
    li      r2, #(\1<<4)
    store.l r2, (r1)
    la      r1, VOODOO_START_G
    li      r2, #(\2<<4)
    store.l r2, (r1)
    la      r1, VOODOO_START_B
    li      r2, #(\3<<4)
    store.l r2, (r1)
    la      r1, VOODOO_START_A
    li      r2, #0x1000
    store.l r2, (r1)
    endm

; Submit triangle (.l)
voodoo_triangle macro
    la      r1, VOODOO_TRIANGLE_CMD
    store.l r0, (r1)
    endm

; Swap buffers (.l)
voodoo_swap macro
    la      r1, VOODOO_SWAP_BUFFER_CMD
    store.l r0, (r1)
    endm

; Clear framebuffer with color (.l)
voodoo_clear macro
    la      r1, VOODOO_COLOR0
    li      r2, #\1
    store.l r2, (r1)
    la      r1, VOODOO_FAST_FILL_CMD
    store.l r0, (r1)
    endm

; ============================================================================
; EOF
; ============================================================================
