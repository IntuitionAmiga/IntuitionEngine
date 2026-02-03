; rotozoomer.asm - Maximum Optimized Rotozoomer
;
; Classic demoscene rotozoomer effect with extensive optimizations.
; Renders a rotating and zooming checkerboard texture at 640x480.
;
; =============================================================================
; OPTIMIZATIONS APPLIED
; =============================================================================
;
; 1. INCREMENTAL TEXTURE COORDINATE CALCULATION
;    - Instead of per-pixel: texX = dx*cosA - dy*sinA (4 multiplies)
;    - We use: texU += du_dx, texV += dv_dx (2 additions)
;    - Saves ~1.2 million multiplications per frame
;
; 2. DOUBLE BUFFERING WITH HARDWARE BLITTER
;    - Render to back buffer (0x22C000) while front is displayed
;    - Hardware blitter copies back->front during vblank
;    - Eliminates tearing artifacts
;
; 3. ALL 16 REGISTERS UTILIZED
;    - Hot loop constants cached in registers (no memory reads)
;    - Eliminates ~1.5 million memory accesses per frame
;    - See register allocation table below
;
; 4. 16x LOOP UNROLLING
;    - Process 16 pixels per iteration (40 iterations for 640 pixels)
;    - Reduces loop overhead from 2 instructions/pixel to 0.125/pixel
;    - Saves ~57,600 instructions per frame vs 4x unrolling
;
; 5. DIRECT TEXTURE ADDRESS CALCULATION
;    - Row offset = texY << 10 (texY * 1024)
;    - Eliminated texture row lookup table
;    - Saves 2 instructions per pixel (~614,400/frame)
;
; 6. DIVISION ELIMINATED WITH RECIPROCAL TABLE
;    - scale_inv = recip_table[divisor] instead of 1536/divisor
;    - 8-entry table (32 bytes) replaces slow DIV instruction
;    - Saves ~30-50 cycles per frame
;
; 7. MULTIPLICATION REPLACED WITH SHIFTS (Constants)
;    - 320 * x >> 8 = x + (x >> 2)   [320/256 = 1.25]
;    - 240 * x >> 8 = x - (x >> 4)   [240/256 = 0.9375]
;    - Eliminates 4 MUL operations per frame
;
; 8. PRECOMPUTED DU/DV TABLES
;    - du_table[scale][angle] = scale_inv[scale] * cos[angle]
;    - dv_table[scale][angle] = scale_inv[scale] * sin[angle]
;    - 5 scale values × 256 angles × 2 tables = 2560 entries (10KB)
;    - Eliminates ALL remaining MUL operations
;
; 9. INLINED ARITHMETIC
;    - No subroutine calls in render path
;    - Eliminates JSR/RTS overhead (~10 cycles per call)
;
; 10. SIGNED ARITHMETIC HANDLING
;    - Shift-based multiplication handles signs via negate/compute/negate
;    - Two's complement addition naturally handles signed increments
;
; 11. PRE-COMPUTED SINE TABLE
;    - 256 entries covering full rotation (0-255 = 0-360 degrees)
;    - Values in 8.8 fixed-point: -256 to +256 (-1.0 to +1.0)
;    - cos(x) = sin(x + 64) - no separate cosine table needed
;
; 12. OPTIMIZED TEXTURE ADDRESSING
;    - Combined shift operations where possible
;    - Minimized register moves in inner loop
;
; =============================================================================
; REGISTER ALLOCATION (Inner Loop)
; =============================================================================
;
;   Reg | Contents                  | Eliminates
;   ----+---------------------------+---------------------------
;    A  | scratch/calculations      | -
;    B  | texture pixel load        | -
;    C  | du_dx (U incr per pixel)  | memory read per pixel
;    D  | dv_dx (V incr per pixel)  | memory read per pixel
;    E  | TEXTURE_BASE address      | immediate per pixel
;    F  | texture row base scratch  | memory variable
;    G  | 255 (TEX_MASK)            | immediate 2x per pixel
;    H  | 4 (pixel stride)          | immediate per pixel
;    S  | du_dy (U incr per row)    | memory read per row
;    T  | current texture V coord   | -
;    U  | current texture U coord   | -
;    V  | dv_dy (V incr per row)    | memory read per row
;    W  | 2560 (LINE_BYTES)         | immediate per row
;    X  | VRAM write pointer        | -
;    Y  | row counter (0-479)       | -
;    Z  | column counter (40 iter)  | -
;
; =============================================================================
; PERFORMANCE SUMMARY
; =============================================================================
;
; Per-pixel operations: 17 instructions
; Loop overhead: 0.125 instructions/pixel (2 per 16 pixels)
; Total per frame: ~5.2 million instructions
; Zero MUL/DIV in render path
;
; Memory layout:
;   0x4000    - Texture (256x256x4 = 256KB)
;   0x44000   - Sine table (256x4 = 1KB)
;   0x44400   - Reciprocal table (8x4 = 32 bytes)
;   0x44420   - DU table (5 scales × 256 angles × 4 = 5KB)
;   0x45820   - DV table (5 scales × 256 angles × 4 = 5KB)
;   0x46C20   - Variables
;   0x100000  - Front buffer (VRAM)
;   0x22C000  - Back buffer
;
; =============================================================================
; USAGE
; =============================================================================
;
; Assemble: ./bin/ie32asm assembler/rotozoomer.asm
; Run:      ./bin/IntuitionEngine -ie32 assembler/rotozoomer.iex

.include "ie32.inc"

; =============================================================================
; CONSTANTS
; =============================================================================

.equ RENDER_W       640
.equ RENDER_H       480
.equ CENTER_X       320
.equ CENTER_Y       240

.equ DISPLAY_W      640
.equ DISPLAY_H      480
.equ FRAME_SIZE     0x12C000

.equ BACK_BUFFER    0x22C000

.equ TEX_SIZE       256
.equ TEX_MASK       255
.equ TEX_ROW_SHIFT  10

.equ FP_SHIFT       8
.equ FP_ONE         256

; Memory layout
.equ TEXTURE_BASE   0x4000
.equ SINE_TABLE     0x44000
.equ RECIP_TABLE    0x44400
.equ DU_TABLE       0x44420         ; 5 scales × 256 angles = 1280 entries
.equ DV_TABLE       0x45820         ; 5 scales × 256 angles = 1280 entries
.equ VAR_BASE       0x46C20

; Variables
.equ VAR_ANGLE      0x46C20
.equ VAR_SCALE_IDX  0x46C24
.equ VAR_SCALE_SEL  0x46C28         ; Current scale selector (0-4)
.equ VAR_DU_DX      0x46C2C
.equ VAR_DV_DX      0x46C30
.equ VAR_DU_DY      0x46C34
.equ VAR_DV_DY      0x46C38
.equ VAR_ROW_U      0x46C3C
.equ VAR_ROW_V      0x46C40
.equ VAR_START_U    0x46C44
.equ VAR_START_V    0x46C48
.equ VAR_TEMP       0x46C4C
.equ VAR_TEMP2      0x46C50
.equ VAR_VRAM_ROW   0x46C54

; =============================================================================
; ENTRY POINT
; =============================================================================
.org 0x1000

start:
    LDA #1
    STA @VIDEO_CTRL

    LDA #0
    STA @VAR_ANGLE
    LDA #192                            ; Start at sine minimum = most zoomed in
    STA @VAR_SCALE_IDX

    JSR generate_texture
    JSR generate_sine_table
    JSR generate_recip_table
    JSR generate_dudv_tables

main_loop:
    JSR update_animation
    JSR render_rotozoomer
    JSR wait_vsync
    JSR blit_to_front
    JMP main_loop

; =============================================================================
; WAIT FOR VSYNC
; =============================================================================
wait_vsync:
.wait:
    LDA @VIDEO_STATUS
    AND A, #STATUS_VBLANK
    JZ A, .wait
    RTS

; =============================================================================
; GENERATE TEXTURE (256x256 checkerboard)
; =============================================================================
generate_texture:
    LDX #TEXTURE_BASE
    LDY #0

.row:
    LDZ #0

.col:
    LDA Z
    XOR A, Y
    AND A, #128
    JNZ A, .dark

    LDA #0xFFFFFFFF
    JMP .store

.dark:
    LDA #0xFF000000

.store:
    STA [X]
    ADD X, #4

    ADD Z, #1
    LDA #TEX_SIZE
    SUB A, Z
    JNZ A, .col

    ADD Y, #1
    LDA #TEX_SIZE
    SUB A, Y
    JNZ A, .row
    RTS

; =============================================================================
; GENERATE SINE TABLE
; =============================================================================
generate_sine_table:
    LDX #SINE_TABLE
    LDY #0

.loop:
    LDA Y
    AND A, #63
    STA @VAR_TEMP

    LDA Y
    AND A, #64
    JZ A, .rising

    LDA #64
    SUB A, @VAR_TEMP
    STA @VAR_TEMP

.rising:
    LDA @VAR_TEMP
    SHL A, #2

    LDB #256
    LDC A
    SUB C, B
    AND C, #0x80000000
    JNZ C, .clamp_ok
    LDA #256
.clamp_ok:
    STA @VAR_TEMP2

    LDB Y
    AND B, #128
    JZ B, .positive

    LDA #0
    SUB A, @VAR_TEMP2
    JMP .store_sine

.positive:
    LDA @VAR_TEMP2

.store_sine:
    STA [X]
    ADD X, #4

    ADD Y, #1
    LDA #256
    SUB A, Y
    JNZ A, .loop
    RTS

; =============================================================================
; GENERATE RECIPROCAL TABLE
; recip_table[x] = 1536 / x for x = 0..7
; =============================================================================
generate_recip_table:
    LDX #RECIP_TABLE

    LDA #0
    STA [X]
    ADD X, #4

    LDA #1536
    STA [X]
    ADD X, #4

    LDA #768
    STA [X]
    ADD X, #4

    LDA #512
    STA [X]
    ADD X, #4

    LDA #384
    STA [X]
    ADD X, #4

    LDA #307
    STA [X]
    ADD X, #4

    LDA #256
    STA [X]
    ADD X, #4

    LDA #219
    STA [X]

    RTS

; =============================================================================
; GENERATE DU/DV TABLES
; Precompute scale_inv * cos/sin for all scale and angle combinations
; DU_TABLE[scale * 256 + angle] = recip[scale+2] * cos[angle] >> 8
; DV_TABLE[scale * 256 + angle] = recip[scale+2] * sin[angle] >> 8
; scale = 0..4 (maps to divisor 2..6)
; =============================================================================
generate_dudv_tables:
    ; For each scale (0-4)
    LDY #0                          ; Y = scale index

.scale_loop:
    ; Get reciprocal value for this scale
    ; recip_table[scale + 2] (since divisor = scale_shifted + 1, and scale_shifted ranges 1-5)
    LDA Y
    ADD A, #2                       ; divisor index
    SHL A, #2
    ADD A, #RECIP_TABLE
    LDB A
    LDA [B]
    STA @VAR_TEMP                   ; VAR_TEMP = scale_inv for this scale

    ; Calculate table base for this scale
    ; DU base = DU_TABLE + scale * 1024 (256 entries * 4 bytes)
    LDA Y
    SHL A, #10
    ADD A, #DU_TABLE
    STA @VAR_TEMP2                  ; DU table pointer for this scale

    ; For each angle (0-255)
    LDZ #0

.angle_loop:
    ; Get cos[angle] = sin[angle + 64]
    LDA Z
    ADD A, #64
    AND A, #255
    SHL A, #2
    ADD A, #SINE_TABLE
    LDB A
    LDA [B]                         ; A = cos[angle]

    ; Multiply by scale_inv (inline signed multiply)
    ; Result = (scale_inv * cosA) >> 8
    STA @0x46C60                    ; temp storage for cosA

    ; Check sign of cosA
    AND A, #0x80000000
    JNZ A, .cos_neg

    ; Positive cosA
    LDA @0x46C60
    MUL A, @VAR_TEMP
    SHR A, #8
    JMP .store_du

.cos_neg:
    ; Negative cosA: negate, multiply, negate
    LDA #0
    SUB A, @0x46C60
    MUL A, @VAR_TEMP
    SHR A, #8
    LDB #0
    SUB B, A
    LDA B

.store_du:
    ; Store in DU table
    LDB @VAR_TEMP2
    STA [B]

    ; Now compute DV = scale_inv * sin[angle] >> 8
    LDA Z
    SHL A, #2
    ADD A, #SINE_TABLE
    LDB A
    LDA [B]                         ; A = sin[angle]
    STA @0x46C60

    AND A, #0x80000000
    JNZ A, .sin_neg

    LDA @0x46C60
    MUL A, @VAR_TEMP
    SHR A, #8
    JMP .store_dv

.sin_neg:
    LDA #0
    SUB A, @0x46C60
    MUL A, @VAR_TEMP
    SHR A, #8
    LDB #0
    SUB B, A
    LDA B

.store_dv:
    ; Store in DV table - BUG FIX: preserve dv value before computing address
    STA @0x46C64                    ; Save dv value temporarily
    LDA @VAR_TEMP2
    ADD A, #5120                    ; Offset to DV_TABLE
    LDB A
    LDA @0x46C64                    ; Restore dv value
    STA [B]

    ; Advance to next angle
    LDA @VAR_TEMP2
    ADD A, #4
    STA @VAR_TEMP2

    ADD Z, #1
    LDA #256
    SUB A, Z
    JNZ A, .angle_loop

    ; Next scale
    ADD Y, #1
    LDA #5
    SUB A, Y
    JNZ A, .scale_loop

    RTS

; =============================================================================
; UPDATE ANIMATION (No MUL - uses precomputed tables)
; =============================================================================
update_animation:
    ; Increment angle
    LDA @VAR_ANGLE
    ADD A, #1
    AND A, #TEX_MASK
    STA @VAR_ANGLE

    ; Increment scale index
    LDA @VAR_SCALE_IDX
    ADD A, #2
    AND A, #TEX_MASK
    STA @VAR_SCALE_IDX

    ; Calculate scale selector (0-4) from scale value
    ; scale = 205 + (sin[scale_idx] >> 1), range ~77-333
    ; scale >> 6 gives 1-5, so (scale >> 6) - 1 gives 0-4
    LDA @VAR_SCALE_IDX
    SHL A, #2
    ADD A, #SINE_TABLE
    LDB A
    LDA [B]                         ; sin value

    ; Signed shift right by 1
    LDB A
    AND B, #0x80000000
    JZ B, .shr_pos
    SHR A, #1
    OR A, #0x80000000
    JMP .shr_done
.shr_pos:
    SHR A, #1
.shr_done:
    ADD A, #205                     ; scale in 8.8

    ; Clamp to positive
    LDB A
    AND B, #0x80000000
    JZ B, .scale_ok
    LDA #77
.scale_ok:
    ; Convert to scale selector: (scale >> 6) - 1, clamped to 0-4
    SHR A, #6                       ; 1-5
    SUB A, #1                       ; 0-4
    AND A, #0x80000000
    JNZ A, .clamp_low
    LDB A
    LDA #4
    SUB A, B
    AND A, #0x80000000
    JZ A, .clamp_high
    LDA B
    JMP .scale_sel_done
.clamp_low:
    LDA #0
    JMP .scale_sel_done
.clamp_high:
    LDA #4
.scale_sel_done:
    STA @VAR_SCALE_SEL

    ; Look up du_dx from precomputed table
    ; DU_TABLE[scale_sel * 256 + angle]
    LDA @VAR_SCALE_SEL
    SHL A, #10                      ; * 1024 (256 entries * 4 bytes)
    LDB @VAR_ANGLE
    SHL B, #2                       ; * 4
    ADD A, B
    ADD A, #DU_TABLE
    LDB A
    LDA [B]
    STA @VAR_DU_DX

    ; Look up dv_dx from precomputed table
    LDA @VAR_SCALE_SEL
    SHL A, #10
    LDB @VAR_ANGLE
    SHL B, #2
    ADD A, B
    ADD A, #DV_TABLE
    LDB A
    LDA [B]
    STA @VAR_DV_DX

    ; du_dy = -dv_dx
    LDA #0
    SUB A, @VAR_DV_DX
    STA @VAR_DU_DY

    ; dv_dy = du_dx
    LDA @VAR_DU_DX
    STA @VAR_DV_DY

    ; Set texture center
    LDA #0x8000
    STA @VAR_START_U
    STA @VAR_START_V

    RTS

; =============================================================================
; RENDER ROTOZOOMER
; 16x loop unrolling, all arithmetic inlined
; =============================================================================
render_rotozoomer:
    ; Calculate starting row U,V using shifts (no MUL)
    ; 320 * x >> 8 = x + (x >> 2)
    ; 240 * x >> 8 = x - (x >> 4)

    ; === 320 * du_dx >> 8 ===
    LDA @VAR_DU_DX
    STA @VAR_TEMP2
    AND A, #0x80000000
    JNZ A, .neg_320_dudx

    LDA @VAR_TEMP2
    LDB A
    SHR B, #2
    ADD A, B
    JMP .done_320_dudx

.neg_320_dudx:
    LDA #0
    SUB A, @VAR_TEMP2
    LDB A
    SHR B, #2
    ADD A, B
    LDB #0
    SUB B, A
    LDA B

.done_320_dudx:
    STA @VAR_TEMP

    ; === 240 * du_dy >> 8 ===
    LDA @VAR_DU_DY
    STA @VAR_TEMP2
    AND A, #0x80000000
    JNZ A, .neg_240_dudy

    LDA @VAR_TEMP2
    LDB A
    SHR B, #4
    SUB A, B
    JMP .done_240_dudy

.neg_240_dudy:
    LDA #0
    SUB A, @VAR_TEMP2
    LDB A
    SHR B, #4
    SUB A, B
    LDB #0
    SUB B, A
    LDA B

.done_240_dudy:
    ADD A, @VAR_TEMP

    LDB @VAR_START_U
    SUB B, A
    STB @VAR_ROW_U

    ; === 320 * dv_dx >> 8 ===
    LDA @VAR_DV_DX
    STA @VAR_TEMP2
    AND A, #0x80000000
    JNZ A, .neg_320_dvdx

    LDA @VAR_TEMP2
    LDB A
    SHR B, #2
    ADD A, B
    JMP .done_320_dvdx

.neg_320_dvdx:
    LDA #0
    SUB A, @VAR_TEMP2
    LDB A
    SHR B, #2
    ADD A, B
    LDB #0
    SUB B, A
    LDA B

.done_320_dvdx:
    STA @VAR_TEMP

    ; === 240 * dv_dy >> 8 ===
    LDA @VAR_DV_DY
    STA @VAR_TEMP2
    AND A, #0x80000000
    JNZ A, .neg_240_dvdy

    LDA @VAR_TEMP2
    LDB A
    SHR B, #4
    SUB A, B
    JMP .done_240_dvdy

.neg_240_dvdy:
    LDA #0
    SUB A, @VAR_TEMP2
    LDB A
    SHR B, #4
    SUB A, B
    LDB #0
    SUB B, A
    LDA B

.done_240_dvdy:
    ADD A, @VAR_TEMP

    LDB @VAR_START_V
    SUB B, A
    STB @VAR_ROW_V

    LDA #BACK_BUFFER
    STA @VAR_VRAM_ROW

    ; Load constants into registers
    LDC @VAR_DU_DX
    LDD @VAR_DV_DX
    LDE #TEXTURE_BASE
    LDG #TEX_MASK
    LDH #4
    LDS @VAR_DU_DY
    LDV @VAR_DV_DY
    LDW #LINE_BYTES

    LDY #0

.row_loop:
    LDX @VAR_VRAM_ROW
    LDU @VAR_ROW_U
    LDT @VAR_ROW_V

    ; 16x unrolled: 40 iterations for 640 pixels
    LDZ #40

.col_loop:
    ; ===== PIXEL 0 =====
    LDA T
    SHR A, #8
    AND A, G
    SHL A, #TEX_ROW_SHIFT
    ADD A, E
    LDF A
    LDA U
    SHR A, #8
    AND A, G
    SHL A, #2
    ADD A, F
    LDB A
    LDA [B]
    STA [X]
    ADD X, H
    ADD U, C
    ADD T, D

    ; ===== PIXEL 1 =====
    LDA T
    SHR A, #8
    AND A, G
    SHL A, #TEX_ROW_SHIFT
    ADD A, E
    LDF A
    LDA U
    SHR A, #8
    AND A, G
    SHL A, #2
    ADD A, F
    LDB A
    LDA [B]
    STA [X]
    ADD X, H
    ADD U, C
    ADD T, D

    ; ===== PIXEL 2 =====
    LDA T
    SHR A, #8
    AND A, G
    SHL A, #TEX_ROW_SHIFT
    ADD A, E
    LDF A
    LDA U
    SHR A, #8
    AND A, G
    SHL A, #2
    ADD A, F
    LDB A
    LDA [B]
    STA [X]
    ADD X, H
    ADD U, C
    ADD T, D

    ; ===== PIXEL 3 =====
    LDA T
    SHR A, #8
    AND A, G
    SHL A, #TEX_ROW_SHIFT
    ADD A, E
    LDF A
    LDA U
    SHR A, #8
    AND A, G
    SHL A, #2
    ADD A, F
    LDB A
    LDA [B]
    STA [X]
    ADD X, H
    ADD U, C
    ADD T, D

    ; ===== PIXEL 4 =====
    LDA T
    SHR A, #8
    AND A, G
    SHL A, #TEX_ROW_SHIFT
    ADD A, E
    LDF A
    LDA U
    SHR A, #8
    AND A, G
    SHL A, #2
    ADD A, F
    LDB A
    LDA [B]
    STA [X]
    ADD X, H
    ADD U, C
    ADD T, D

    ; ===== PIXEL 5 =====
    LDA T
    SHR A, #8
    AND A, G
    SHL A, #TEX_ROW_SHIFT
    ADD A, E
    LDF A
    LDA U
    SHR A, #8
    AND A, G
    SHL A, #2
    ADD A, F
    LDB A
    LDA [B]
    STA [X]
    ADD X, H
    ADD U, C
    ADD T, D

    ; ===== PIXEL 6 =====
    LDA T
    SHR A, #8
    AND A, G
    SHL A, #TEX_ROW_SHIFT
    ADD A, E
    LDF A
    LDA U
    SHR A, #8
    AND A, G
    SHL A, #2
    ADD A, F
    LDB A
    LDA [B]
    STA [X]
    ADD X, H
    ADD U, C
    ADD T, D

    ; ===== PIXEL 7 =====
    LDA T
    SHR A, #8
    AND A, G
    SHL A, #TEX_ROW_SHIFT
    ADD A, E
    LDF A
    LDA U
    SHR A, #8
    AND A, G
    SHL A, #2
    ADD A, F
    LDB A
    LDA [B]
    STA [X]
    ADD X, H
    ADD U, C
    ADD T, D

    ; ===== PIXEL 8 =====
    LDA T
    SHR A, #8
    AND A, G
    SHL A, #TEX_ROW_SHIFT
    ADD A, E
    LDF A
    LDA U
    SHR A, #8
    AND A, G
    SHL A, #2
    ADD A, F
    LDB A
    LDA [B]
    STA [X]
    ADD X, H
    ADD U, C
    ADD T, D

    ; ===== PIXEL 9 =====
    LDA T
    SHR A, #8
    AND A, G
    SHL A, #TEX_ROW_SHIFT
    ADD A, E
    LDF A
    LDA U
    SHR A, #8
    AND A, G
    SHL A, #2
    ADD A, F
    LDB A
    LDA [B]
    STA [X]
    ADD X, H
    ADD U, C
    ADD T, D

    ; ===== PIXEL 10 =====
    LDA T
    SHR A, #8
    AND A, G
    SHL A, #TEX_ROW_SHIFT
    ADD A, E
    LDF A
    LDA U
    SHR A, #8
    AND A, G
    SHL A, #2
    ADD A, F
    LDB A
    LDA [B]
    STA [X]
    ADD X, H
    ADD U, C
    ADD T, D

    ; ===== PIXEL 11 =====
    LDA T
    SHR A, #8
    AND A, G
    SHL A, #TEX_ROW_SHIFT
    ADD A, E
    LDF A
    LDA U
    SHR A, #8
    AND A, G
    SHL A, #2
    ADD A, F
    LDB A
    LDA [B]
    STA [X]
    ADD X, H
    ADD U, C
    ADD T, D

    ; ===== PIXEL 12 =====
    LDA T
    SHR A, #8
    AND A, G
    SHL A, #TEX_ROW_SHIFT
    ADD A, E
    LDF A
    LDA U
    SHR A, #8
    AND A, G
    SHL A, #2
    ADD A, F
    LDB A
    LDA [B]
    STA [X]
    ADD X, H
    ADD U, C
    ADD T, D

    ; ===== PIXEL 13 =====
    LDA T
    SHR A, #8
    AND A, G
    SHL A, #TEX_ROW_SHIFT
    ADD A, E
    LDF A
    LDA U
    SHR A, #8
    AND A, G
    SHL A, #2
    ADD A, F
    LDB A
    LDA [B]
    STA [X]
    ADD X, H
    ADD U, C
    ADD T, D

    ; ===== PIXEL 14 =====
    LDA T
    SHR A, #8
    AND A, G
    SHL A, #TEX_ROW_SHIFT
    ADD A, E
    LDF A
    LDA U
    SHR A, #8
    AND A, G
    SHL A, #2
    ADD A, F
    LDB A
    LDA [B]
    STA [X]
    ADD X, H
    ADD U, C
    ADD T, D

    ; ===== PIXEL 15 =====
    LDA T
    SHR A, #8
    AND A, G
    SHL A, #TEX_ROW_SHIFT
    ADD A, E
    LDF A
    LDA U
    SHR A, #8
    AND A, G
    SHL A, #2
    ADD A, F
    LDB A
    LDA [B]
    STA [X]
    ADD X, H
    ADD U, C
    ADD T, D

    ; Loop: 40 iterations
    SUB Z, #1
    JNZ Z, .col_loop

    ; Advance to next row
    LDA @VAR_VRAM_ROW
    ADD A, W
    STA @VAR_VRAM_ROW

    LDA @VAR_ROW_U
    ADD A, S
    STA @VAR_ROW_U

    LDA @VAR_ROW_V
    ADD A, V
    STA @VAR_ROW_V

    ADD Y, #1
    LDA #RENDER_H
    SUB A, Y
    JNZ A, .row_loop

    RTS

; =============================================================================
; BLIT BACK BUFFER TO FRONT BUFFER
; =============================================================================
blit_to_front:
    LDA #BLT_OP_COPY
    STA @BLT_OP

    LDA #BACK_BUFFER
    STA @BLT_SRC

    LDA #VRAM_START
    STA @BLT_DST

    LDA #DISPLAY_W
    STA @BLT_WIDTH

    LDA #DISPLAY_H
    STA @BLT_HEIGHT

    LDA #LINE_BYTES
    STA @BLT_SRC_STRIDE
    STA @BLT_DST_STRIDE

    LDA #1
    STA @BLT_CTRL

.wait_blit:
    LDA @BLT_STATUS
    AND A, #2
    JNZ A, .wait_blit

    RTS

; =============================================================================
; EOF
; =============================================================================
