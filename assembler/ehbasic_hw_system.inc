; ehbasic_hw_system.inc - EhBASIC IE64 System Commands
;
; Statement handlers for system-level BASIC commands:
;   WAIT addr, mask [,xor] - Poll memory until condition met
;   COPPER LIST/WAIT/MOVE/END/ON/OFF - Copper coprocessor
;   BLIT COPY/FILL/LINE/WAIT - Blitter operations
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; skip_alpha - Skip remaining alphabetic characters of a sub-command keyword
; ============================================================================
; After consuming the first byte of a sub-command (either a token byte or
; an ASCII letter), call this to skip any remaining ASCII alpha chars.
; If the sub-command was a token (1 byte), the next byte is non-alpha so
; this returns immediately. If it was ASCII "LIST", it skips "IST".
; Input:  R17 = text pointer (past first byte)
; Output: R17 advanced past all [A-Za-z] chars
; Clobbers: R1, R2

skip_alpha:
    load.b  r1, (r17)
    move.q  r2, #0x41
    blt     r1, r2, .sa_done
    move.q  r2, #0x5A
    ble     r1, r2, .sa_next
    move.q  r2, #0x61
    blt     r1, r2, .sa_done
    move.q  r2, #0x7A
    bgt     r1, r2, .sa_done
.sa_next:
    add.q   r17, r17, #1
    bra     skip_alpha
.sa_done:
    rts

; ============================================================================
; hw_wait - WAIT addr, mask [,xor]
; ============================================================================
; Polls PEEK(addr) until ((value XOR xor) AND mask) != 0.
; If xor is omitted, defaults to 0.

hw_wait:
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = address
    ; Parse comma + mask
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .wait_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r23, r8              ; R23 = mask
    ; Check for optional xor argument
    move.q  r24, r0              ; default xor = 0
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .wait_poll
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r24, r8              ; R24 = xor value
.wait_poll:
    move.q  r25, #0x100000          ; timeout counter (~1M iterations)
.wait_loop:
    ; Read value at address
    load.l  r1, (r22)
    ; Apply XOR
    eor.l   r1, r1, r24
    ; Apply AND mask
    and.l   r1, r1, r23
    ; Loop until non-zero or timeout
    bnez    r1, .wait_done
    sub.q   r25, r25, #1
    bnez    r25, .wait_loop
.wait_done:
    rts

; ============================================================================
; hw_copper - COPPER LIST/WAIT/MOVE/END/ON/OFF
; ============================================================================
; COPPER LIST addr    → set COPPER_PTR
; COPPER ON / OFF     → set/clear COPPER_CTRL bit 0
; COPPER WAIT scan    → (0<<30)|(scan<<12), 4 bytes
; COPPER MOVE addr,val→ SETBASE(addr) + MOVE(regIdx=0) + DATA(val), 12 bytes
; COPPER END          → 0xC0000000, 4 bytes
;
; Copper build pointer stored at state + 0x50

hw_copper:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    ; Check for ON token
    move.q  r2, #TK_ON
    beq     r1, r2, .cop_on
    ; Check for 'O' (OFF)
    move.q  r2, #0x4F
    beq     r1, r2, .cop_check_off
    ; Check for LIST (token or ASCII)
    move.q  r2, #TK_LIST
    beq     r1, r2, .cop_list
    move.q  r2, #0x4C
    beq     r1, r2, .cop_list
    ; Check for WAIT (token or ASCII)
    move.q  r2, #TK_WAIT
    beq     r1, r2, .cop_wait
    move.q  r2, #0x57
    beq     r1, r2, .cop_wait
    ; Check for MOVE (ASCII only, not a token)
    move.q  r2, #0x4D
    beq     r1, r2, .cop_move
    ; Check for END (token or ASCII)
    move.q  r2, #TK_END
    beq     r1, r2, .cop_end
    move.q  r2, #0x45
    beq     r1, r2, .cop_end
    rts
.cop_on:
    add.q   r17, r17, #1
    la      r1, COPPER_CTRL
    move.q  r2, #1
    store.l r2, (r1)
    rts
.cop_check_off:
    add.q   r3, r17, #1
    load.b  r4, (r3)
    move.q  r2, #0x46            ; 'F'
    bne     r4, r2, .cop_done
    add.q   r3, r3, #1
    load.b  r4, (r3)
    bne     r4, r2, .cop_done
    add.q   r17, r3, #1
    la      r1, COPPER_CTRL
    store.l r0, (r1)
    rts
.cop_list:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Set COPPER_PTR
    la      r1, COPPER_PTR
    store.l r8, (r1)
    ; Also save as build pointer at state+0x50
    add.q   r1, r16, #0x50
    store.l r8, (r1)
    rts
.cop_wait:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Build WAIT opcode: (0<<30) | (scanline << 12)
    lsl.l   r1, r8, #12
    ; Write at build pointer
    add.q   r3, r16, #0x50
    load.l  r4, (r3)
    store.l r1, (r4)
    ; Advance build pointer by 4
    add.q   r4, r4, #4
    store.l r4, (r3)
    rts
.cop_move:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = absolute address
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .cop_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r23, r8              ; R23 = value

    ; Guard: addr < 0xA0000 → ?FC ERROR
    move.l  r1, #0xA0000
    blt     r22, r1, .cop_move_err

    ; Emit SETBASE: 0x80000000 | (addr >> 2)
    lsr.l   r1, r22, #2
    or.l    r1, r1, #0x80000000
    add.q   r3, r16, #0x50
    load.l  r4, (r3)
    store.l r1, (r4)
    add.q   r4, r4, #4

    ; Emit MOVE opcode: 0x40000000 (regIndex=0)
    move.l  r1, #0x40000000
    store.l r1, (r4)
    add.q   r4, r4, #4

    ; Emit MOVE data
    store.l r23, (r4)
    add.q   r4, r4, #4

    store.l r4, (r3)            ; update build pointer (+12)
    rts

.cop_move_err:
    la      r8, cop_err_fc
    jsr     print_string
    jsr     print_crlf
    rts
.cop_end:
    add.q   r17, r17, #1
    jsr     skip_alpha
    ; Write end opcode: 0xC0000000 (opcode 3 = END)
    add.q   r3, r16, #0x50
    load.l  r4, (r3)
    move.l  r1, #0xC0000000
    store.l r1, (r4)
    add.q   r4, r4, #4
    store.l r4, (r3)
    rts
.cop_done:
    rts

; ============================================================================
; hw_blit - BLIT COPY/FILL/LINE/WAIT
; ============================================================================
; BLIT COPY src, dst, w, h [,srcstride, dststride]
; BLIT FILL dst, w, h, colour [,stride]
; BLIT LINE x1, y1, x2, y2, colour, stride
; BLIT WAIT

hw_blit:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    ; Check first letter/token to determine sub-command
    move.q  r2, #0x43            ; 'C' (COPY)
    beq     r1, r2, .blt_copy
    move.q  r2, #0x46            ; 'F' (FILL)
    beq     r1, r2, .blt_fill
    ; LINE (token or ASCII)
    move.q  r2, #TK_LINE_CMD
    beq     r1, r2, .blt_line
    move.q  r2, #0x4C
    beq     r1, r2, .blt_line
    ; WAIT (token or ASCII)
    move.q  r2, #TK_WAIT
    beq     r1, r2, .blt_wait
    move.q  r2, #0x57
    beq     r1, r2, .blt_wait
    ; MEMCOPY (ASCII)
    move.q  r2, #0x4D            ; 'M'
    beq     r1, r2, .blt_memcopy
    rts
.blt_copy:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse src
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, BLT_SRC
    store.l r8, (r3)
    ; Parse dst
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .blt_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, BLT_DST
    store.l r8, (r3)
    ; Parse w
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .blt_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, BLT_WIDTH
    store.l r8, (r3)
    ; Parse h
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .blt_copy_start
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, BLT_HEIGHT
    store.l r8, (r3)
    ; Optional srcstride
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .blt_copy_start
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, BLT_SRC_STRIDE
    store.l r8, (r3)
    ; Optional dststride
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .blt_copy_start
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, BLT_DST_STRIDE
    store.l r8, (r3)
.blt_copy_start:
    la      r3, BLT_OP
    move.q  r1, #BLT_OP_COPY
    store.l r1, (r3)
    la      r3, BLT_CTRL
    move.q  r1, #1
    store.l r1, (r3)
    rts
.blt_fill:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse dst
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, BLT_DST
    store.l r8, (r3)
    ; Parse w
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .blt_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, BLT_WIDTH
    store.l r8, (r3)
    ; Parse h
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .blt_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, BLT_HEIGHT
    store.l r8, (r3)
    ; Parse colour
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .blt_fill_start
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, BLT_COLOR
    store.l r8, (r3)
    ; Optional stride
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .blt_fill_start
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, BLT_DST_STRIDE
    store.l r8, (r3)
.blt_fill_start:
    la      r3, BLT_OP
    move.q  r1, #BLT_OP_FILL
    store.l r1, (r3)
    la      r3, BLT_CTRL
    move.q  r1, #1
    store.l r1, (r3)
    rts
.blt_line:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse x1
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8
    ; Parse y1
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .blt_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r23, r8
    ; Parse x2
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .blt_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r24, r8
    ; Parse y2
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .blt_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r25, r8
    ; Parse colour
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .blt_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, BLT_COLOR
    store.l r8, (r3)
    ; Parse stride
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .blt_line_start
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, BLT_DST_STRIDE
    store.l r8, (r3)
.blt_line_start:
    ; BLT_SRC = x1 | (y1 << 16)
    lsl.l   r1, r23, #16
    or.l    r1, r1, r22
    la      r3, BLT_SRC
    store.l r1, (r3)
    ; BLT_DST = x2 | (y2 << 16)
    lsl.l   r1, r25, #16
    or.l    r1, r1, r24
    la      r3, BLT_DST
    store.l r1, (r3)
    la      r3, BLT_OP
    move.q  r1, #BLT_OP_LINE
    store.l r1, (r3)
    la      r3, BLT_CTRL
    move.q  r1, #1
    store.l r1, (r3)
    rts
.blt_wait:
    add.q   r17, r17, #1
    jsr     skip_alpha
.blt_wait_poll:
    move.q  r3, #0x100000           ; timeout counter (~1M iterations)
.blt_wait_loop:
    la      r1, BLT_CTRL
    load.l  r2, (r1)
    beqz    r2, .blt_wait_exit
    sub.q   r3, r3, #1
    bnez    r3, .blt_wait_loop
.blt_wait_exit:
    rts
.blt_memcopy:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     hw_memcopy
    rts
.blt_done:
    rts

; ============================================================================
; hw_memcopy - MEMCOPY src, dst, len
; ============================================================================
; Blitter-accelerated memory copy.

hw_memcopy:
    jsr     exec_skip_spaces
    ; Parse src
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, BLT_SRC
    store.l r8, (r3)
    ; Parse dst
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .mc_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, BLT_DST
    store.l r8, (r3)
    ; Parse len
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .mc_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, BLT_WIDTH
    store.l r8, (r3)
    la      r3, BLT_HEIGHT
    move.q  r1, #1
    store.l r1, (r3)
    la      r3, BLT_SRC_STRIDE
    store.l r8, (r3)
    la      r3, BLT_DST_STRIDE
    store.l r8, (r3)
    la      r3, BLT_OP
    move.q  r1, #BLT_OP_COPY
    store.l r1, (r3)
    la      r3, BLT_CTRL
    move.q  r1, #1
    store.l r1, (r3)
.mc_done:
    rts

cop_err_fc:
    dc.b    "?FC ERROR", 0
    align 8

; ============================================================================
; EOF
; ============================================================================
