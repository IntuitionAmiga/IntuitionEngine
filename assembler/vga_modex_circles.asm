; vga_modex_circles.asm - VGA Mode X Demo with Page Flipping (IE32)
; 320x240 256-color mode with double buffering
;
; Assemble: ./bin/ie32asm assembler/vga_modex_circles.asm
; Run: ./bin/IntuitionEngine -ie32 assembler/vga_modex_circles.iex

; Include VGA definitions
.include "ie32.inc"

; Mode X dimensions
.equ WIDTH          320
.equ HEIGHT         240
.equ PAGE_SIZE      19200       ; Mode X: 320 * 240 / 4 (planar)

; Page addresses (Mode X: 320*240/4 = 19200 bytes per page per plane)
.equ PAGE0          0
.equ PAGE1          19200

; Screen center
.equ CENTER_X       160
.equ CENTER_Y       120

; Variables
.equ VAR_FRAME      0x8800
.equ VAR_PAGE       0x8804
.equ VAR_RADIUS     0x8808

.org 0x1000

start:
    ; Enable VGA
    LDA #VGA_CTRL_ENABLE
    STA @VGA_CTRL

    ; Set Mode X (320x240x256)
    LDA #VGA_MODE_X
    STA @VGA_MODE

    ; Setup gradient palette
    JSR setup_palette

    ; Initialize variables
    LDA #0
    STA @VAR_FRAME
    STA @VAR_PAGE

main_loop:
    ; Wait for vsync
    JSR wait_vsync

    ; Clear back buffer
    JSR clear_page

    ; Draw expanding circles
    JSR draw_circles

    ; Flip pages
    JSR flip_page

    ; Increment frame
    LDA @VAR_FRAME
    ADD A, #1
    STA @VAR_FRAME

    JMP main_loop

; -----------------------------------------------------------------------------
; wait_vsync - Wait for vertical sync
; -----------------------------------------------------------------------------
wait_vsync:
.wait:
    LDA @VGA_STATUS
    AND A, #VGA_STATUS_VSYNC
    JZ A, .wait
    RTS

; -----------------------------------------------------------------------------
; setup_palette - Create rainbow gradient palette
; -----------------------------------------------------------------------------
setup_palette:
    LDX #0              ; Palette index

.pal_loop:
    ; Set write index
    LDA X
    STA @VGA_DAC_WINDEX

    ; R = sin-like curve based on index
    LDA X
    AND A, #0x3F        ; 0-63
    STA @VGA_DAC_DATA

    ; G = offset sine
    LDA X
    ADD A, #21
    AND A, #0x3F
    STA @VGA_DAC_DATA

    ; B = offset sine
    LDA X
    ADD A, #42
    AND A, #0x3F
    STA @VGA_DAC_DATA

    ADD X, #1
    LDA #256
    SUB A, X
    JNZ A, .pal_loop

    RTS

; -----------------------------------------------------------------------------
; clear_page - Clear the back buffer (Mode X planar)
; Mode X page size = 320 * 240 / 4 = 19200 bytes per plane
; -----------------------------------------------------------------------------
clear_page:
    ; Enable all planes for clearing
    LDA #0x0F
    STA @VGA_SEQ_MAPMASK

    ; Determine back buffer address
    LDA @VAR_PAGE
    JZ A, .clear_page1
    ; Page 0 is front, clear page 1 (offset 19200)
    LDX #VGA_VRAM
    ADD X, #19200
    JMP .do_clear
.clear_page1:
    ; Page 1 is front, clear page 0 (offset 0)
    LDX #VGA_VRAM

.do_clear:
    ; Mode X page size = 19200 bytes
    LDY #19200

.clr:
    LDA #0
    STA [X]
    ADD X, #1
    SUB Y, #1
    JNZ Y, .clr
    RTS

; -----------------------------------------------------------------------------
; draw_circles - Draw concentric expanding circles
; -----------------------------------------------------------------------------
draw_circles:
    ; Calculate base radius from frame
    LDA @VAR_FRAME
    AND A, #0x7F        ; 0-127
    STA @VAR_RADIUS

    ; Draw 8 circles with different radii
    LDT #0              ; Circle counter

.circle_loop:
    ; Calculate radius for this circle
    LDA @VAR_RADIUS
    LDB T
    MUL B, #16          ; Offset each circle by 16 pixels
    ADD A, B
    AND A, #0x7F        ; Wrap at 128

    ; Skip if radius is 0
    JZ A, .next_circle

    ; Calculate color based on circle index
    LDB T
    MUL B, #32          ; Different color for each circle
    ADD B, @VAR_FRAME
    AND B, #0xFF

    ; Draw circle
    JSR draw_circle     ; A = radius, B = color

.next_circle:
    ADD T, #1
    LDA #8
    SUB A, T
    JNZ A, .circle_loop

    RTS

; -----------------------------------------------------------------------------
; draw_circle - Draw a circle using midpoint algorithm
; Input: A = radius, B = color
; -----------------------------------------------------------------------------
draw_circle:
    PUSH C
    PUSH D
    PUSH E
    PUSH F
    PUSH T
    PUSH U

    LDC A               ; C = radius
    LDD B               ; D = color
    LDE #0              ; E = x = 0
    LDF C               ; F = y = radius
    LDT #1
    SUB T, C            ; T = decision = 1 - radius

.dc_loop:
    ; Draw 8 octants
    ; Point (cx+x, cy+y)
    LDA #CENTER_X
    ADD A, E
    LDB #CENTER_Y
    ADD B, F
    JSR plot_pixel

    ; Point (cx-x, cy+y)
    LDA #CENTER_X
    SUB A, E
    LDB #CENTER_Y
    ADD B, F
    JSR plot_pixel

    ; Point (cx+x, cy-y)
    LDA #CENTER_X
    ADD A, E
    LDB #CENTER_Y
    SUB B, F
    JSR plot_pixel

    ; Point (cx-x, cy-y)
    LDA #CENTER_X
    SUB A, E
    LDB #CENTER_Y
    SUB B, F
    JSR plot_pixel

    ; Point (cx+y, cy+x)
    LDA #CENTER_X
    ADD A, F
    LDB #CENTER_Y
    ADD B, E
    JSR plot_pixel

    ; Point (cx-y, cy+x)
    LDA #CENTER_X
    SUB A, F
    LDB #CENTER_Y
    ADD B, E
    JSR plot_pixel

    ; Point (cx+y, cy-x)
    LDA #CENTER_X
    ADD A, F
    LDB #CENTER_Y
    SUB B, E
    JSR plot_pixel

    ; Point (cx-y, cy-x)
    LDA #CENTER_X
    SUB A, F
    LDB #CENTER_Y
    SUB B, E
    JSR plot_pixel

    ; Update decision and coordinates
    LDA T
    AND A, #0x80000000
    JZ A, .dec_positive

    ; decision < 0: decision += 2*x + 3
    LDA E
    MUL A, #2
    ADD A, #3
    ADD T, A
    ADD E, #1
    JMP .dc_check

.dec_positive:
    ; decision >= 0: decision += 2*(x-y) + 5
    LDA E
    SUB A, F
    MUL A, #2
    ADD A, #5
    ADD T, A
    ADD E, #1
    SUB F, #1

.dc_check:
    ; Continue while x <= y
    LDA F
    SUB A, E
    AND A, #0x80000000
    JZ A, .dc_loop

    POP U
    POP T
    POP F
    POP E
    POP D
    POP C
    RTS

; -----------------------------------------------------------------------------
; plot_pixel - Plot a pixel at (A, B) with color D using Mode X planar addressing
; Mode X: plane = x & 3, offset = y * 80 + x / 4
; -----------------------------------------------------------------------------
plot_pixel:
    PUSH F
    PUSH E

    ; Check bounds
    LDF A
    AND F, #0x80000000
    JNZ F, .pp_skip     ; x < 0
    LDF A
    SUB F, #WIDTH
    AND F, #0x80000000
    JZ F, .pp_skip      ; x >= 320

    LDF B
    AND F, #0x80000000
    JNZ F, .pp_skip     ; y < 0
    LDF B
    SUB F, #HEIGHT
    AND F, #0x80000000
    JZ F, .pp_skip      ; y >= 240

    ; Calculate plane from X coordinate: plane = x & 3
    LDE A               ; E = x
    AND E, #3           ; E = x & 3
    LDF #1
    SHL F, E            ; F = 1 << plane (map mask)
    PUSH A              ; Save x
    LDA F
    STA @VGA_SEQ_MAPMASK
    POP A               ; Restore x

    ; Calculate planar offset: offset = y * 80 + x / 4
    LDF B
    MUL F, #80          ; F = y * 80 (bytes per scanline in Mode X)
    LDE A
    SHR E, #2           ; E = x / 4
    ADD F, E            ; F = y * 80 + x / 4

    ; Add page offset (back buffer)
    ; Page size in Mode X = 320 * 240 / 4 = 19200 bytes per plane
    LDA @VAR_PAGE
    JZ A, .pp_page1
    ; Page 0 is front, draw to page 1 (offset 19200)
    ADD F, #19200
    JMP .pp_addr
.pp_page1:
    ; Page 1 is front, draw to page 0 (offset 0)

.pp_addr:
    ADD F, #VGA_VRAM

    ; Write pixel
    LDA D
    STA [F]

.pp_skip:
    POP E
    POP F
    RTS

; -----------------------------------------------------------------------------
; flip_page - Switch display page
; -----------------------------------------------------------------------------
flip_page:
    ; Toggle page variable
    LDA @VAR_PAGE
    XOR A, #1
    STA @VAR_PAGE

    ; Set CRTC start address
    JZ A, .show_page0
    ; Show page 1
    LDA #PAGE1
    SHR A, #8
    STA @VGA_CRTC_STARTHI
    LDA #PAGE1
    AND A, #0xFF
    STA @VGA_CRTC_STARTLO
    RTS

.show_page0:
    ; Show page 0
    LDA #0
    STA @VGA_CRTC_STARTHI
    STA @VGA_CRTC_STARTLO
    RTS
