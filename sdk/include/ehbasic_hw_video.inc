; ehbasic_hw_video.inc - EhBASIC IE64 VGA Video Commands
;
; Statement handlers for VGA video hardware commands:
;   SCREEN mode / SCREEN ON / SCREEN OFF
;   COLOR fg [,bg]
;   CLS [colour]
;   PLOT x, y [,colour]
;   PALETTE idx, r, g, b
;   VSYNC
;   LOCATE row, col
;
; All handlers are jumped to from exec_line dispatch and
; branch back to .el_next_stmt on completion.
;
; Register conventions:
;   R16 = interpreter state base
;   R17 = text pointer
;   R26 = TERM_OUT address
;   R27 = TERM_STATUS address
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; hw_screen - SCREEN mode / SCREEN ON / SCREEN OFF
; ============================================================================
; SCREEN &H13   → set VGA mode 13h (320x200x256)
; SCREEN &H12   → set VGA mode 12h (640x480x16)
; SCREEN 3      → set VGA text mode (80x25)
; SCREEN ON     → enable VGA
; SCREEN OFF    → disable VGA

hw_screen:
    jsr     exec_skip_spaces
    ; Check for ON token
    load.b  r1, (r17)
    move.q  r2, #TK_ON
    bne     r1, r2, .scr_not_on
    add.q   r17, r17, #1         ; consume ON
    ; Enable VGA: write 1 to VGA_CTRL
    la      r1, VGA_CTRL
    move.q  r2, #VGA_CTRL_ENABLE
    store.l r2, (r1)
    rts
.scr_not_on:
    ; Check for 'O' (start of "OFF")
    move.q  r2, #0x4F            ; 'O'
    bne     r1, r2, .scr_mode
    ; Check "OFF"
    add.q   r3, r17, #1
    load.b  r4, (r3)
    move.q  r2, #0x46            ; 'F'
    bne     r4, r2, .scr_mode
    add.q   r3, r3, #1
    load.b  r4, (r3)
    bne     r4, r2, .scr_mode
    ; Confirmed "OFF"
    add.q   r17, r3, #1          ; consume "OFF"
    ; Disable VGA: write 0 to VGA_CTRL
    la      r1, VGA_CTRL
    store.l r0, (r1)
    rts
.scr_mode:
    ; Evaluate mode number
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix               ; R8 = mode number
    ; Write to VGA_MODE register
    la      r1, VGA_MODE
    store.l r8, (r1)
    ; Enable VGA
    la      r1, VGA_CTRL
    move.q  r2, #VGA_CTRL_ENABLE
    store.l r2, (r1)
    rts

; ============================================================================
; hw_cls - CLS [colour]
; ============================================================================
; Clears the screen using the blitter fill operation.
; In mode 13h: fills VRAM at 0xA0000, 320x200 bytes
; Optional colour argument (default 0).

hw_cls:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    beqz    r1, .cls_default
    move.q  r2, #0x3A            ; ':'
    beq     r1, r2, .cls_default
    ; Has colour argument
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = colour
    bra     .cls_do
.cls_default:
    move.q  r22, r0              ; colour = 0
.cls_do:
    ; Read current VGA mode to determine dimensions
    la      r1, VGA_MODE
    load.l  r1, (r1)
    ; Mode 13h: 320x200, mode 12h: 640x480, text: 80x25x2
    move.q  r2, #VGA_MODE_13H
    beq     r1, r2, .cls_13h
    move.q  r2, #VGA_MODE_12H
    beq     r1, r2, .cls_12h
    ; Default: assume text mode 80x25
    bra     .cls_text
.cls_13h:
    ; Fill 64000 bytes at VGA_VRAM with colour
    la      r1, BLT_COLOR
    store.l r22, (r1)
    la      r1, BLT_DST
    move.l  r2, #VGA_VRAM
    store.l r2, (r1)
    la      r1, BLT_WIDTH
    move.l  r2, #320
    store.l r2, (r1)
    la      r1, BLT_HEIGHT
    move.l  r2, #200
    store.l r2, (r1)
    la      r1, BLT_DST_STRIDE
    move.l  r2, #320
    store.l r2, (r1)
    la      r1, BLT_OP
    move.q  r2, #BLT_OP_FILL
    store.l r2, (r1)
    la      r1, BLT_CTRL
    move.q  r2, #1               ; start
    store.l r2, (r1)
    rts
.cls_12h:
    ; Fill 640x480/8 = 38400 bytes per plane, 4 planes
    ; For simplicity, fill 38400 bytes at VGA_VRAM
    la      r1, BLT_COLOR
    store.l r22, (r1)
    la      r1, BLT_DST
    move.l  r2, #VGA_VRAM
    store.l r2, (r1)
    la      r1, BLT_WIDTH
    move.l  r2, #80              ; 640/8 bytes per row
    store.l r2, (r1)
    la      r1, BLT_HEIGHT
    move.l  r2, #480
    store.l r2, (r1)
    la      r1, BLT_DST_STRIDE
    move.l  r2, #80
    store.l r2, (r1)
    la      r1, BLT_OP
    move.q  r2, #BLT_OP_FILL
    store.l r2, (r1)
    la      r1, BLT_CTRL
    move.q  r2, #1
    store.l r2, (r1)
    rts
.cls_text:
    ; Fill text buffer: 80x25x2 = 4000 bytes with spaces + attribute
    la      r1, BLT_COLOR
    ; Space (0x20) + default attribute (0x07 = light grey on black)
    move.l  r2, #0x07200720      ; two character cells
    store.l r2, (r1)
    la      r1, BLT_DST
    move.l  r2, #0xB8000         ; VGA text buffer
    store.l r2, (r1)
    la      r1, BLT_WIDTH
    move.l  r2, #160             ; 80 cols * 2 bytes
    store.l r2, (r1)
    la      r1, BLT_HEIGHT
    move.l  r2, #25
    store.l r2, (r1)
    la      r1, BLT_DST_STRIDE
    move.l  r2, #160
    store.l r2, (r1)
    la      r1, BLT_OP
    move.q  r2, #BLT_OP_FILL
    store.l r2, (r1)
    la      r1, BLT_CTRL
    move.q  r2, #1
    store.l r2, (r1)
    rts

; ============================================================================
; hw_plot - PLOT x, y [,colour]
; ============================================================================
; In mode 13h: writes pixel at VRAM[y*320+x]
; Optional colour (default 15 = white)

hw_plot:
    jsr     exec_skip_spaces
    jsr     expr_eval            ; x
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = x
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .plot_done
    add.q   r17, r17, #1         ; consume ','
    jsr     exec_skip_spaces
    jsr     expr_eval            ; y
    jsr     fp_int
    jsr     fp_fix
    move.q  r23, r8              ; R23 = y
    ; Check for optional colour
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .plot_default_col
    add.q   r17, r17, #1         ; consume ','
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r24, r8              ; R24 = colour
    bra     .plot_pixel
.plot_default_col:
    move.q  r24, #15             ; default white
.plot_pixel:
    ; Read VGA mode
    la      r1, VGA_MODE
    load.l  r1, (r1)
    move.q  r2, #VGA_MODE_13H
    bne     r1, r2, .plot_done   ; only mode 13h for now
    ; offset = y * 320 + x
    mulu.l  r1, r23, #320
    add.l   r1, r1, r22
    ; Write pixel
    la      r2, VGA_VRAM
    add.q   r1, r1, r2
    store.b r24, (r1)
.plot_done:
    rts

; ============================================================================
; hw_palette - PALETTE idx, r, g, b
; ============================================================================
; Sets VGA DAC palette entry.

hw_palette:
    jsr     exec_skip_spaces
    jsr     expr_eval            ; idx
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = palette index
    ; Expect comma
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .pal_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval            ; r
    jsr     fp_int
    jsr     fp_fix
    move.q  r23, r8              ; R23 = red
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .pal_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval            ; g
    jsr     fp_int
    jsr     fp_fix
    move.q  r24, r8              ; R24 = green
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .pal_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval            ; b
    jsr     fp_int
    jsr     fp_fix
    move.q  r25, r8              ; R25 = blue
    ; Write to DAC: set write index, then write R, G, B
    la      r1, VGA_DAC_WINDEX
    store.l r22, (r1)
    la      r1, VGA_DAC_DATA
    store.l r23, (r1)            ; red
    store.l r24, (r1)            ; green
    store.l r25, (r1)            ; blue
.pal_done:
    rts

; ============================================================================
; hw_vsync - VSYNC
; ============================================================================
; Waits for vertical blank by polling VGA_STATUS.

hw_vsync:
    la      r1, VGA_STATUS
    move.q  r3, #0x100000           ; timeout counter (~1M iterations)
.vsync_wait:
    load.l  r2, (r1)
    and.l   r2, r2, #VGA_STATUS_VSYNC
    bnez    r2, .vsync_done
    sub.q   r3, r3, #1
    bnez    r3, .vsync_wait
.vsync_done:
    rts

; ============================================================================
; hw_locate - LOCATE row, col
; ============================================================================
; Sets text cursor position via CRTC registers.

hw_locate:
    jsr     exec_skip_spaces
    jsr     expr_eval            ; row
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = row (0-based)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .loc_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval            ; col
    jsr     fp_int
    jsr     fp_fix
    move.q  r23, r8              ; R23 = col (0-based)
    ; Cursor offset = row * 80 + col
    mulu.l  r1, r22, #80
    add.l   r1, r1, r23
    ; Write cursor position to CRTC registers
    ; High byte to CRTC register 0x0E
    lsr.l   r2, r1, #8
    la      r3, VGA_CRTC_INDEX
    move.q  r4, #0x0E
    store.l r4, (r3)
    la      r3, VGA_CRTC_DATA
    store.l r2, (r3)
    ; Low byte to CRTC register 0x0F
    and.l   r2, r1, #0xFF
    la      r3, VGA_CRTC_INDEX
    move.q  r4, #0x0F
    store.l r4, (r3)
    la      r3, VGA_CRTC_DATA
    store.l r2, (r3)
.loc_done:
    rts

; ============================================================================
; hw_color - COLOR fg [,bg]
; ============================================================================
; Sets text attribute colour. Stored in a state variable for PRINT to use.
; For now, this stores the attribute at ST_TEXT_ATTR (offset 0x4C in state).

hw_color:
    jsr     exec_skip_spaces
    jsr     expr_eval            ; fg
    jsr     fp_int
    jsr     fp_fix
    and.l   r22, r8, #0x0F       ; fg is lower nibble
    ; Check for bg
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .color_store
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval            ; bg
    jsr     fp_int
    jsr     fp_fix
    and.l   r1, r8, #0x0F
    lsl.l   r1, r1, #4
    or.l    r22, r22, r1         ; combine fg + bg*16
.color_store:
    ; Store attribute at state + 0x4C
    add.q   r1, r16, #0x4C
    store.l r22, (r1)
    rts

; ============================================================================
; hw_scroll - SCROLL dx, dy
; ============================================================================
; Adjusts VGA CRTC start address for hardware scrolling.
; dx and dy are pixel offsets (text mode: character positions).

hw_scroll:
    jsr     exec_skip_spaces
    jsr     expr_eval            ; dx
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = dx
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .scrl_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval            ; dy
    jsr     fp_int
    jsr     fp_fix
    move.q  r23, r8              ; R23 = dy
    ; Calculate start address offset = dy * 80 + dx (text mode)
    mulu.l  r1, r23, #80
    add.l   r1, r1, r22
    ; Write to CRTC start address registers
    lsr.l   r2, r1, #8
    la      r3, VGA_CRTC_STARTHI
    store.l r2, (r3)
    and.l   r2, r1, #0xFF
    la      r3, VGA_CRTC_STARTLO
    store.l r2, (r3)
.scrl_done:
    rts

; ============================================================================
; hw_line - LINE x1, y1, x2, y2 [,colour]
; ============================================================================
; Draws a line using the blitter LINE operation.
; BLT_SRC = x0 | (y0 << 16), BLT_DST = x1 | (y1 << 16)

hw_line:
    jsr     exec_skip_spaces
    ; Parse x1
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = x1
    ; Parse comma + y1
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .line_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r23, r8              ; R23 = y1
    ; Parse comma + x2
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .line_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r24, r8              ; R24 = x2
    ; Parse comma + y2
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .line_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r25, r8              ; R25 = y2
    ; Check for optional colour
    move.q  r28, #15             ; default white
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .line_draw
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r28, r8              ; R28 = colour
.line_draw:
    ; Set BLT_SRC = x1 | (y1 << 16)
    lsl.l   r1, r23, #16
    or.l    r1, r1, r22
    la      r3, BLT_SRC
    store.l r1, (r3)
    ; Set BLT_DST = x2 | (y2 << 16)
    lsl.l   r1, r25, #16
    or.l    r1, r1, r24
    la      r3, BLT_DST
    store.l r1, (r3)
    ; Set colour
    la      r3, BLT_COLOR
    store.l r28, (r3)
    ; Set BLT_DST_STRIDE = screen width (320 for mode 13h)
    la      r3, BLT_DST_STRIDE
    move.l  r1, #320
    store.l r1, (r3)
    ; Set operation to LINE
    la      r3, BLT_OP
    move.q  r1, #BLT_OP_LINE
    store.l r1, (r3)
    ; Start blitter
    la      r3, BLT_CTRL
    move.q  r1, #1
    store.l r1, (r3)
.line_done:
    rts

; ============================================================================
; hw_box - BOX x1, y1, x2, y2 [,colour] [,FILL]
; ============================================================================
; Draws a rectangle. If FILL keyword follows, uses blitter fill.
; Otherwise draws 4 lines.

hw_box:
    jsr     exec_skip_spaces
    ; Parse x1
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = x1
    ; Parse y1
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .box_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r23, r8              ; R23 = y1
    ; Parse x2
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .box_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r24, r8              ; R24 = x2
    ; Parse y2
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .box_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r25, r8              ; R25 = y2
    ; Check for optional colour
    move.q  r28, #15             ; default white
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .box_fill
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r28, r8              ; R28 = colour
.box_fill:
    ; Filled rectangle via blitter
    ; Calculate width and height
    sub.l   r1, r24, r22         ; width = x2 - x1
    add.l   r1, r1, #1           ; inclusive
    sub.l   r2, r25, r23         ; height = y2 - y1
    add.l   r2, r2, #1
    ; Set colour
    la      r3, BLT_COLOR
    store.l r28, (r3)
    ; Calculate dest address: VGA_VRAM + y1*320 + x1
    mulu.l  r4, r23, #320
    add.l   r4, r4, r22
    move.l  r5, #VGA_VRAM
    add.l   r4, r4, r5
    la      r3, BLT_DST
    store.l r4, (r3)
    ; Set dimensions
    la      r3, BLT_WIDTH
    store.l r1, (r3)
    la      r3, BLT_HEIGHT
    store.l r2, (r3)
    la      r3, BLT_DST_STRIDE
    move.l  r1, #320
    store.l r1, (r3)
    ; Fill op
    la      r3, BLT_OP
    move.q  r1, #BLT_OP_FILL
    store.l r1, (r3)
    ; Start
    la      r3, BLT_CTRL
    move.q  r1, #1
    store.l r1, (r3)
.box_done:
    rts

; ============================================================================
; hw_circle - CIRCLE x, y, r [,colour]
; ============================================================================
; Draws a circle using the midpoint algorithm, plotting pixels directly.
; Mode 13h only (320x200).

hw_circle:
    jsr     exec_skip_spaces
    ; Parse cx
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = cx
    ; Parse cy
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .circ_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r23, r8              ; R23 = cy
    ; Parse radius
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .circ_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r24, r8              ; R24 = radius
    ; Check for optional colour
    move.q  r25, #15             ; default white
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .circ_draw
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r25, r8              ; R25 = colour
.circ_draw:
    ; Midpoint circle algorithm
    ; x=r, y=0, d=1-r
    push    r14
    push    r15
    move.q  r14, r24             ; R14 = x (starts at radius)
    move.q  r15, r0              ; R15 = y (starts at 0)
    move.q  r1, #1
    sub.l   r28, r1, r24         ; R28 = d = 1 - radius
.circ_loop:
    ; Plot 8 symmetric points
    ; (cx+x, cy+y), (cx-x, cy+y), (cx+x, cy-y), (cx-x, cy-y)
    ; (cx+y, cy+x), (cx-y, cy+x), (cx+y, cy-x), (cx-y, cy-x)
    ; Plot (cx+x, cy+y)
    add.l   r1, r22, r14
    add.l   r2, r23, r15
    jsr     .circ_plot
    ; Plot (cx-x, cy+y)
    sub.l   r1, r22, r14
    add.l   r2, r23, r15
    jsr     .circ_plot
    ; Plot (cx+x, cy-y)
    add.l   r1, r22, r14
    sub.l   r2, r23, r15
    jsr     .circ_plot
    ; Plot (cx-x, cy-y)
    sub.l   r1, r22, r14
    sub.l   r2, r23, r15
    jsr     .circ_plot
    ; Plot (cx+y, cy+x)
    add.l   r1, r22, r15
    add.l   r2, r23, r14
    jsr     .circ_plot
    ; Plot (cx-y, cy+x)
    sub.l   r1, r22, r15
    add.l   r2, r23, r14
    jsr     .circ_plot
    ; Plot (cx+y, cy-x)
    add.l   r1, r22, r15
    sub.l   r2, r23, r14
    jsr     .circ_plot
    ; Plot (cx-y, cy-x)
    sub.l   r1, r22, r15
    sub.l   r2, r23, r14
    jsr     .circ_plot
    ; Advance
    add.q   r15, r15, #1         ; y++
    ; If d < 0: d += 2*y + 1
    ; Else: x--, d += 2*(y-x) + 1
    ; Check sign of d (d is signed 32-bit in lower 32)
    ; Since .l ops zero-extend, we need to check bit 31
    move.l  r1, #0x80000000
    and.l   r3, r28, r1
    bnez    r3, .circ_d_neg
    ; d >= 0: x--, d += 2*(y-x)+1
    sub.q   r14, r14, #1         ; x--
    sub.l   r1, r15, r14         ; y - x
    lsl.l   r1, r1, #1           ; 2*(y-x)
    add.l   r1, r1, #1           ; +1
    add.l   r28, r28, r1
    bra     .circ_check
.circ_d_neg:
    ; d < 0: d += 2*y + 1
    lsl.l   r1, r15, #1          ; 2*y
    add.l   r1, r1, #1           ; +1
    add.l   r28, r28, r1
.circ_check:
    ; Continue while y <= x
    ble     r15, r14, .circ_loop
    pop     r15
    pop     r14
.circ_done:
    rts

; Helper: plot pixel at (R1, R2) with colour R25
; Bounds-check for mode 13h (320x200)
.circ_plot:
    ; Bounds check
    move.l  r3, #320
    bge     r1, r3, .cp_skip
    move.l  r3, #200
    bge     r2, r3, .cp_skip
    ; Check negative (unsigned comparison handles this since .l zero-extends)
    move.l  r3, #0x80000000
    and.l   r4, r1, r3
    bnez    r4, .cp_skip
    and.l   r4, r2, r3
    bnez    r4, .cp_skip
    ; offset = y*320 + x
    mulu.l  r3, r2, #320
    add.l   r3, r3, r1
    la      r4, VGA_VRAM
    add.q   r3, r3, r4
    store.b r25, (r3)
.cp_skip:
    rts

; ============================================================================
; hw_ula - ULA ON/OFF/BORDER/INK/PAPER/BRIGHT/FLASH/PLOT/CLS/ATTR
; ============================================================================

hw_ula:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    ; Check for ON token
    move.q  r2, #TK_ON
    beq     r1, r2, .ula_on
    ; Check for token bytes first
    move.q  r2, #0xE4            ; TK_PLOT
    beq     r1, r2, .ula_plot
    move.q  r2, #0xE3            ; TK_CLS
    beq     r1, r2, .ula_cls
    ; Check first letter
    move.q  r2, #0x4F            ; 'O' (OFF)
    beq     r1, r2, .ula_off_check
    move.q  r2, #0x42            ; 'B' (BORDER/BRIGHT)
    beq     r1, r2, .ula_b_cmd
    move.q  r2, #0x49            ; 'I' (INK)
    beq     r1, r2, .ula_ink
    move.q  r2, #0x50            ; 'P' (PAPER/PLOT)
    beq     r1, r2, .ula_p_cmd
    move.q  r2, #0x46            ; 'F' (FLASH)
    beq     r1, r2, .ula_flash
    move.q  r2, #0x43            ; 'C' (CLS)
    beq     r1, r2, .ula_cls
    move.q  r2, #0x41            ; 'A' (ATTR)
    beq     r1, r2, .ula_attr
    rts
.ula_on:
    add.q   r17, r17, #1
    la      r1, ULA_CTRL
    move.q  r2, #ULA_CTRL_ENABLE
    store.l r2, (r1)
    rts
.ula_off_check:
    add.q   r3, r17, #1
    load.b  r4, (r3)
    move.q  r2, #0x46
    bne     r4, r2, .ula_done
    add.q   r3, r3, #1
    load.b  r4, (r3)
    bne     r4, r2, .ula_done
    add.q   r17, r3, #1
    la      r1, ULA_CTRL
    store.l r0, (r1)
    rts
.ula_b_cmd:
    ; Check second char: 'O'=BORDER, 'R'=BRIGHT
    add.q   r3, r17, #1
    load.b  r4, (r3)
    move.q  r2, #0x52            ; 'R'
    beq     r4, r2, .ula_bright
    ; BORDER - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, ULA_BORDER
    store.l r8, (r1)
    rts
.ula_bright:
    ; Skip "BRIGHT" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Store bright flag at state+0x54
    add.q   r1, r16, #0x54
    store.l r8, (r1)
    rts
.ula_ink:
    ; Skip "INK" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Store ink at state+0x58
    add.q   r1, r16, #0x58
    store.l r8, (r1)
    rts
.ula_p_cmd:
    ; Check 'A'=PAPER, 'L'=PLOT
    add.q   r3, r17, #1
    load.b  r4, (r3)
    move.q  r2, #0x4C            ; 'L'
    beq     r4, r2, .ula_plot
    ; PAPER - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Store paper at state+0x5C
    add.q   r1, r16, #0x5C
    store.l r8, (r1)
    rts
.ula_plot:
    ; Skip PLOT token or "PLOT" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse x
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .ula_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse y
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r23, r8
    ; ZX Spectrum VRAM addressing: non-linear
    ; byte = VRAM + ((y&0xC0)<<5) + ((y&7)<<8) + ((y&0x38)<<2) + (x>>3)
    and.l   r1, r23, #0xC0
    lsl.l   r1, r1, #5
    and.l   r2, r23, #7
    lsl.l   r2, r2, #8
    or.l    r1, r1, r2
    and.l   r2, r23, #0x38
    lsl.l   r2, r2, #2
    or.l    r1, r1, r2
    lsr.l   r2, r22, #3
    or.l    r1, r1, r2
    la      r3, ULA_VRAM
    add.q   r1, r1, r3
    ; Set bit within byte (bit 7 - (x & 7))
    and.l   r2, r22, #7
    move.q  r3, #7
    sub.l   r2, r3, r2
    move.q  r3, #1
    lsl.l   r3, r3, r2
    ; OR into VRAM byte
    load.b  r4, (r1)
    or.l    r4, r4, r3
    store.b r4, (r1)
    rts
.ula_flash:
    ; Skip "FLASH" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Store flash flag at state+0x60
    add.q   r1, r16, #0x60
    store.l r8, (r1)
    rts
.ula_cls:
    ; Skip CLS token or "CLS" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Optional attr value (default 0x38 = white paper, black ink)
    move.q  r22, #0x38
    load.b  r1, (r17)
    beqz    r1, .ula_cls_do
    move.q  r2, #0x3A
    beq     r1, r2, .ula_cls_do
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8
.ula_cls_do:
    ; Clear bitmap (6144 bytes at ULA_VRAM)
    la      r1, BLT_COLOR
    store.l r0, (r1)
    la      r1, BLT_DST
    move.l  r2, #ULA_VRAM
    store.l r2, (r1)
    la      r1, BLT_WIDTH
    move.l  r2, #32              ; 256/8 = 32 bytes per row
    store.l r2, (r1)
    la      r1, BLT_HEIGHT
    move.l  r2, #192
    store.l r2, (r1)
    la      r1, BLT_DST_STRIDE
    move.l  r2, #32
    store.l r2, (r1)
    la      r1, BLT_OP
    move.q  r2, #BLT_OP_FILL
    store.l r2, (r1)
    la      r1, BLT_CTRL
    move.q  r2, #1
    store.l r2, (r1)
    ; Set attributes (768 bytes at ULA_VRAM + 0x1800)
    la      r1, BLT_COLOR
    store.l r22, (r1)
    la      r1, BLT_DST
    move.l  r2, #ULA_VRAM
    add.l   r2, r2, #ULA_ATTR_OFFSET
    store.l r2, (r1)
    la      r1, BLT_WIDTH
    move.l  r2, #32
    store.l r2, (r1)
    la      r1, BLT_HEIGHT
    move.l  r2, #24
    store.l r2, (r1)
    la      r1, BLT_DST_STRIDE
    move.l  r2, #32
    store.l r2, (r1)
    la      r1, BLT_OP
    move.q  r2, #BLT_OP_FILL
    store.l r2, (r1)
    la      r1, BLT_CTRL
    move.q  r2, #1
    store.l r2, (r1)
    rts
.ula_attr:
    ; Skip "ATTR" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse x (column 0-31)
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .ula_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse y (row 0-23)
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r23, r8
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .ula_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse attr value
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Write to attr area: ULA_VRAM + 0x1800 + y*32 + x
    mulu.l  r1, r23, #32
    add.l   r1, r1, r22
    move.l  r2, #ULA_VRAM
    add.l   r2, r2, #ULA_ATTR_OFFSET
    add.q   r1, r1, r2
    store.b r8, (r1)
.ula_done:
    rts

; ============================================================================
; hw_ted_cmd - TED ON/OFF/MODE/COLOR/CHAR/VIDEO/PLOT/CLS/SCROLL
; ============================================================================

hw_ted_cmd:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_ON
    beq     r1, r2, .ted_on
    ; Check for token bytes first
    move.q  r2, #0xE8            ; TK_COLOR
    beq     r1, r2, .ted_color
    move.q  r2, #0xE3            ; TK_CLS
    beq     r1, r2, .ted_cls
    move.q  r2, #0xEC            ; TK_SCROLL_CMD
    beq     r1, r2, .ted_scroll
    ; Check first letter
    move.q  r2, #0x4F            ; 'O' (OFF)
    beq     r1, r2, .ted_off_check
    move.q  r2, #0x4D            ; 'M' (MODE)
    beq     r1, r2, .ted_mode
    move.q  r2, #0x43            ; 'C' (COLOR/CHAR/CLS)
    beq     r1, r2, .ted_c_cmd
    move.q  r2, #0x56            ; 'V' (VIDEO/VOL)
    beq     r1, r2, .ted_v_cmd
    move.q  r2, #0x53            ; 'S' (SCROLL)
    beq     r1, r2, .ted_scroll
    ; TED TONE/VOL/NOISE/PLUS/PLAY/STOP - audio sub-commands
    bra     hw_ted_audio
.ted_v_cmd:
    ; Disambiguate VIDEO (VI...) vs VOL (VO...) → audio
    add.q   r3, r17, #1
    load.b  r4, (r3)
    move.q  r2, #0x49            ; 'I' → VIDEO
    beq     r4, r2, .ted_video
    ; VOL or other V-word → route to TED audio handler
    bra     hw_ted_audio
.ted_on:
    add.q   r17, r17, #1
    la      r1, TED_V_ENABLE
    move.q  r2, #TED_V_ENABLE_VIDEO
    store.b r2, (r1)
    rts
.ted_off_check:
    add.q   r3, r17, #1
    load.b  r4, (r3)
    move.q  r2, #0x46
    bne     r4, r2, .ted_done
    add.q   r3, r3, #1
    load.b  r4, (r3)
    bne     r4, r2, .ted_done
    add.q   r17, r3, #1
    la      r1, TED_V_ENABLE
    store.b r0, (r1)
    rts
.ted_mode:
    ; Skip "MODE" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; mode 0=text, 1=bitmap (BMM), 2=multicolour (MCM+BMM)
    la      r3, TED_V_CTRL1
    load.l  r1, (r3)
    ; Clear BMM bit (bit 5) and set DEN bit (bit 4)
    and.l   r1, r1, #0xDF       ; clear bit 5
    or.l    r1, r1, #0x10       ; set DEN
    ; If mode >= 1, set BMM
    move.q  r2, #1
    blt     r8, r2, .ted_mode_write
    or.l    r1, r1, #0x20       ; set BMM
.ted_mode_write:
    store.l r1, (r3)
    ; Handle MCM in CTRL2
    la      r3, TED_V_CTRL2
    load.l  r1, (r3)
    and.l   r1, r1, #0xEF       ; clear MCM bit 4
    move.q  r2, #2
    blt     r8, r2, .ted_mode_ctrl2
    or.l    r1, r1, #0x10       ; set MCM
.ted_mode_ctrl2:
    store.l r1, (r3)
    rts
.ted_c_cmd:
    ; Disambiguate: CO=COLOR, CH=CHAR, CL=CLS
    add.q   r3, r17, #1
    load.b  r4, (r3)
    move.q  r2, #0x4F            ; 'O' → COLOR
    beq     r4, r2, .ted_color
    move.q  r2, #0x48            ; 'H' → CHAR
    beq     r4, r2, .ted_char
    move.q  r2, #0x4C            ; 'L' → CLS
    beq     r4, r2, .ted_cls
    rts
.ted_color:
    ; Skip COLOR token or "COLOR" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, TED_V_BG_COLOR0
    store.l r8, (r1)
    ; Parse border
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .ted_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, TED_V_BORDER
    store.l r8, (r1)
    rts
.ted_char:
    ; Skip "CHAR" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, TED_V_CHAR_BASE
    store.l r8, (r1)
    rts
.ted_video:
    ; Skip "VIDEO" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, TED_V_VIDEO_BASE
    store.l r8, (r1)
    rts
.ted_cls:
    ; Skip CLS token or "CLS" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    ; Fill 1000 bytes of video matrix with spaces (0x20)
    la      r1, BLT_COLOR
    move.l  r2, #0x20
    store.l r2, (r1)
    la      r1, BLT_DST
    la      r2, TED_V_VIDEO_BASE
    load.l  r2, (r2)             ; read configured video base
    store.l r2, (r1)
    la      r1, BLT_WIDTH
    move.l  r2, #40
    store.l r2, (r1)
    la      r1, BLT_HEIGHT
    move.l  r2, #25
    store.l r2, (r1)
    la      r1, BLT_DST_STRIDE
    move.l  r2, #40
    store.l r2, (r1)
    la      r1, BLT_OP
    move.q  r2, #BLT_OP_FILL
    store.l r2, (r1)
    la      r1, BLT_CTRL
    move.q  r2, #1
    store.l r2, (r1)
    rts
.ted_scroll:
    ; Skip SCROLL token or "SCROLL" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse dx
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .ted_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse dy
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Set XSCROLL (bits 0-2 of CTRL2)
    la      r3, TED_V_CTRL2
    load.l  r1, (r3)
    and.l   r1, r1, #0xF8       ; clear bits 0-2
    and.l   r2, r22, #7
    or.l    r1, r1, r2
    store.l r1, (r3)
    ; Set YSCROLL (bits 0-2 of CTRL1)
    la      r3, TED_V_CTRL1
    load.l  r1, (r3)
    and.l   r1, r1, #0xF8
    and.l   r2, r8, #7
    or.l    r1, r1, r2
    store.l r1, (r3)
    rts
.ted_done:
    rts

; ============================================================================
; hw_antic - ANTIC ON/OFF/DLIST/MODE/SCROLL/CHBASE/PMBASE/NMI
; ============================================================================

hw_antic:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_ON
    beq     r1, r2, .antic_on
    ; Check for token bytes first
    move.q  r2, #0xEC            ; TK_SCROLL_CMD
    beq     r1, r2, .antic_scroll
    ; Check first letter
    move.q  r2, #0x4F            ; 'O' (OFF)
    beq     r1, r2, .antic_off_check
    move.q  r2, #0x44            ; 'D' (DLIST)
    beq     r1, r2, .antic_dlist
    move.q  r2, #0x4D            ; 'M' (MODE)
    beq     r1, r2, .antic_mode
    move.q  r2, #0x53            ; 'S' (SCROLL)
    beq     r1, r2, .antic_scroll
    move.q  r2, #0x43            ; 'C' (CHBASE)
    beq     r1, r2, .antic_chbase
    move.q  r2, #0x50            ; 'P' (PMBASE)
    beq     r1, r2, .antic_pmbase
    move.q  r2, #0x4E            ; 'N' (NMI)
    beq     r1, r2, .antic_nmi
    rts
.antic_on:
    add.q   r17, r17, #1
    la      r1, ANTIC_ENABLE
    move.q  r2, #ANTIC_ENABLE_VIDEO
    store.b r2, (r1)
    rts
.antic_off_check:
    add.q   r3, r17, #1
    load.b  r4, (r3)
    move.q  r2, #0x46
    bne     r4, r2, .antic_done
    add.q   r3, r3, #1
    load.b  r4, (r3)
    bne     r4, r2, .antic_done
    add.q   r17, r3, #1
    la      r1, ANTIC_ENABLE
    store.b r0, (r1)
    rts
.antic_dlist:
    ; Skip 'D' then handle TK_LIST token or ASCII "LIST"
    add.q   r17, r17, #1         ; skip 'D'
    load.b  r1, (r17)
    move.q  r2, #0xA0            ; TK_LIST
    bne     r1, r2, .ant_dlist_ascii
    add.q   r17, r17, #1         ; skip TK_LIST token
    bra     .ant_dlist_parse
.ant_dlist_ascii:
    jsr     skip_alpha           ; skip "LIST" remaining chars
.ant_dlist_parse:
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Split into low and high bytes
    la      r1, ANTIC_DLISTL
    and.l   r2, r8, #0xFF
    store.b r2, (r1)
    la      r1, ANTIC_DLISTH
    lsr.l   r2, r8, #8
    and.l   r2, r2, #0xFF
    store.b r2, (r1)
    rts
.antic_mode:
    ; Skip "MODE" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, ANTIC_DMACTL
    store.b r8, (r1)
    rts
.antic_scroll:
    ; Skip SCROLL token or "SCROLL" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, ANTIC_HSCROL
    store.b r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .antic_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, ANTIC_VSCROL
    store.b r8, (r1)
    rts
.antic_chbase:
    ; Skip "CHBASE" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, ANTIC_CHBASE
    store.b r8, (r1)
    rts
.antic_pmbase:
    ; Skip "PMBASE" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, ANTIC_PMBASE
    store.b r8, (r1)
    rts
.antic_nmi:
    ; Skip "NMI" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, ANTIC_NMIEN
    store.b r8, (r1)
    rts
.antic_done:
    rts

; ============================================================================
; hw_gtia - GTIA COLOR/PRIOR/PLAYER/MISSILE/GRAFP/GRAFM/GRACTL
; ============================================================================

hw_gtia:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    ; Check for token bytes first
    move.q  r2, #0xE8            ; TK_COLOR
    beq     r1, r2, .gtia_color
    ; Check first letter
    move.q  r2, #0x43            ; 'C' (COLOR)
    beq     r1, r2, .gtia_color
    move.q  r2, #0x50            ; 'P' (PRIOR/PLAYER)
    beq     r1, r2, .gtia_p_cmd
    move.q  r2, #0x4D            ; 'M' (MISSILE)
    beq     r1, r2, .gtia_missile
    move.q  r2, #0x47            ; 'G' (GRAFP/GRAFM/GRACTL)
    beq     r1, r2, .gtia_g_cmd
    rts
.gtia_color:
    ; Skip COLOR token or "COLOR" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse register index (0=COLPF0..4=COLBK, 5-8=COLPM0-3)
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = reg index
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .gtia_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Base = GTIA_COLPF0, stride = 4
    mulu.l  r1, r22, #4
    la      r2, GTIA_COLPF0
    add.q   r1, r1, r2
    store.b r8, (r1)
    rts
.gtia_p_cmd:
    ; Disambiguate: PR=PRIOR, PL=PLAYER
    add.q   r3, r17, #1
    load.b  r4, (r3)
    move.q  r2, #0x52            ; 'R' → PRIOR
    beq     r4, r2, .gtia_prior
    ; PLAYER - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse player num (0-3)
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8
    ; Parse x position
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .gtia_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r23, r8              ; x position
    ; Parse size
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .gtia_player_nosize
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Write size: GTIA_SIZEP0 + player*4
    mulu.l  r1, r22, #4
    la      r2, GTIA_SIZEP0
    add.q   r1, r1, r2
    store.b r8, (r1)
.gtia_player_nosize:
    ; Write position: GTIA_HPOSP0 + player*4
    mulu.l  r1, r22, #4
    la      r2, GTIA_HPOSP0
    add.q   r1, r1, r2
    store.b r23, (r1)
    rts
.gtia_prior:
    ; Skip "PRIOR" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, GTIA_PRIOR
    store.b r8, (r1)
    rts
.gtia_missile:
    ; Skip "MISSILE" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse missile num (0-3)
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .gtia_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse x position
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Write position: GTIA_HPOSM0 + missile*4
    mulu.l  r1, r22, #4
    la      r2, GTIA_HPOSM0
    add.q   r1, r1, r2
    store.b r8, (r1)
    rts
.gtia_g_cmd:
    ; Disambiguate: GR=GRAFP/GRAFM/GRACTL
    add.q   r3, r17, #4
    load.b  r4, (r3)
    move.q  r2, #0x50            ; 'P' → GRAFP
    beq     r4, r2, .gtia_grafp
    move.q  r2, #0x4D            ; 'M' → GRAFM
    beq     r4, r2, .gtia_grafm
    move.q  r2, #0x54            ; 'T' → GRACTL
    beq     r4, r2, .gtia_gractl
    rts
.gtia_grafp:
    ; Skip "GRAFP" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse player num
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .gtia_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; GTIA_GRAFP0 + n*4
    mulu.l  r1, r22, #4
    la      r2, GTIA_GRAFP0
    add.q   r1, r1, r2
    store.b r8, (r1)
    rts
.gtia_grafm:
    ; Skip "GRAFM" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, GTIA_GRAFM
    store.b r8, (r1)
    rts
.gtia_gractl:
    ; Skip "GRACTL" - skip first char + remaining alpha
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, GTIA_GRACTL
    store.b r8, (r1)
    rts
.gtia_done:
    rts

; ============================================================================
; EOF
; ============================================================================
