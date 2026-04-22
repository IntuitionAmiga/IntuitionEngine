prog_about:
    dc.l    0, 0
    dc.l    prog_about_code_end - prog_about_code
    dc.l    prog_about_data_end - prog_about_data
    dc.l    0
    ds.b    12
prog_about_code:
    sub     sp, sp, #16
.ab_preamble:
    load.q  r30, (sp)
    load.q  r29, 8(sp)
    store.q r29, (sp)
    load.l  r1, TASKSB_TASK_ID(r30)
    store.q r1, 128(r29)

    ; FindPort("intuition.library") first. If absent, use OpenLibraryEx so
    ; exec/dos owns autoload and then resolve the compat port transport.
.ab_findi:
    load.q  r29, (sp)
    add     r1, r29, #256
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    beqz    r2, .ab_findi_ok

    move.l  r1, #16
    syscall #SYS_ALLOC_SIGNAL
    load.q  r29, (sp)
    bnez    r2, .ab_halt
    store.q r1, 184(r29)               ; intuition_open_sigbit
.ab_openi_retry:
    load.q  r29, (sp)
    move.l  r14, #0xFFFFFFFF
    store.l r14, 192(r29)              ; waiter status sentinel
    store.l r0, 196(r29)
    store.l r0, 200(r29)
    store.l r0, 204(r29)
    add     r1, r29, #256              ; "intuition.library"
    move.q  r2, r0
    load.q  r3, 184(r29)               ; waiter-owned signal bit
    add     r4, r29, #192              ; 16-byte outcome scratch
    syscall #SYS_OPEN_LIBRARY_EX
    load.q  r29, (sp)
    beqz    r2, .ab_findi
    move.l  r14, #ERR_AGAIN
    bne     r2, r14, .ab_openi_fail
    load.l  r14, 192(r29)              ; waiter status
    move.l  r15, #0xFFFFFFFF
    bne     r14, r15, .ab_openi_done
    load.q  r14, 184(r29)
    move.q  r1, #1
    lsl     r1, r1, r14
    syscall #SYS_WAIT
    load.q  r29, (sp)
    bnez    r2, .ab_openi_yield
    load.l  r14, 192(r29)              ; waiter status
    move.l  r15, #0xFFFFFFFF
    beq     r14, r15, .ab_openi_yield
.ab_openi_done:
    beqz    r14, .ab_findi
.ab_openi_fail:
    load.q  r14, 184(r29)              ; intuition_open_sigbit
    beqz    r14, .ab_halt
    move.q  r1, r14
    syscall #SYS_FREE_SIGNAL
    load.q  r29, (sp)
    store.q r0, 184(r29)
    bra     .ab_halt
.ab_openi_yield:
    syscall #SYS_YIELD
    bra     .ab_openi_retry
.ab_findi_ok:
    store.q r1, 136(r29)               ; intuition_port
    load.q  r14, 184(r29)              ; intuition_open_sigbit
    beqz    r14, .ab_findi_sigfree_done
    move.q  r1, r14
    syscall #SYS_FREE_SIGNAL
    load.q  r29, (sp)
    store.q r0, 184(r29)
.ab_findi_sigfree_done:

    ; CreatePort(NULL) → reply_port
    move.q  r1, r0
    move.l  r2, #0
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    bnez    r2, .ab_halt
    store.q r1, 144(r29)               ; reply_port

    ; CreatePort(NULL) → idcmp_port
    move.q  r1, r0
    move.l  r2, #0
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    bnez    r2, .ab_halt
    store.q r1, 152(r29)               ; idcmp_port

    ; AllocMem(320*200*4 = 256000, MEMF_PUBLIC|MEMF_CLEAR)
    ; M12: window is 320x200 to fit nicely under the 16-pixel title bar
    ; with room for several lines of about text in the content area.
    move.l  r1, #256000
    move.l  r2, #0x10001
    syscall #SYS_ALLOC_MEM             ; R1=va R2=err R3=share
    load.q  r29, (sp)
    bnez    r2, .ab_halt
    store.q r1, 160(r29)               ; surface_va
    store.l r3, 168(r29)               ; surface_share

    ; Fill window backing buffer with the recessed-panel grey colour
    ; COL_PANEL_BG = 0xFFDCD8D0 (AmigaOS 3.9 / ReAction recessed panel).
    ; intuition.library overpaints the outer frame, blue title bar, and
    ; recessed-panel border on top, but the interior of the panel keeps
    ; these pixels — so the user sees a uniform recessed grey body with
    ; black Topaz text on top.
    load.q  r14, 160(r29)
    move.l  r15, #256000               ; bytes
    move.l  r16, #0xFFDCD8D0           ; COL_PANEL_BG
.ab_fill:
    beqz    r15, .ab_fill_done
    store.l r16, (r14)
    add     r14, r14, #4
    sub     r15, r15, #4
    bra     .ab_fill
.ab_fill_done:

    ; ===== Render about text into the window =====
    ; The window's content area starts below intuition.library's 16-pixel
    ; title bar plus the 1-pixel outer + 1-pixel inner frame, so visible
    ; window. The recessed content panel painted by intuition.library
    ; starts at window-local (8, 24) and extends to (312, 168). All text
    ; lives inside that panel area, indented 8 px (so x = 16) and spaced
    ; 18 px apart (16-px glyph height + 2 px leading).
    ;
    ; Text color = black (0xFF000000) on COL_PANEL_BG (0xFFDCD8D0).

    ; Line 1 (y=32): "About IntuitionOS"
    move.l  r10, #16                   ; x
    move.l  r11, #32                   ; y
    add     r12, r29, #288             ; r12 = string ptr (data offset 288)
    jsr     .ab_draw_string
    ; Line 2 (y=56): "Protected Exec-inspired kernel"
    move.l  r10, #16
    move.l  r11, #56
    add     r12, r29, #320             ; data offset 320
    jsr     .ab_draw_string
    ; Line 3 (y=80): "intuition.library demonstration"
    move.l  r10, #16
    move.l  r11, #80
    add     r12, r29, #352             ; data offset 352
    jsr     .ab_draw_string
    ; Line 4 (y=104): "All services run in user space"
    move.l  r10, #16
    move.l  r11, #104
    add     r12, r29, #384             ; data offset 384
    jsr     .ab_draw_string
    ; Line 5 (y=152): "Press Esc to close"
    move.l  r10, #16
    move.l  r11, #152
    add     r12, r29, #416             ; data offset 416
    jsr     .ab_draw_string

    ; Send INTUITION_OPEN_WINDOW
    ; data0 = (320<<48)|(200<<32)|(240<<16)|200  (w/h/x/y)
    ; M12: window centered on 800x600 screen at (240, 200), size 320x200.
    load.q  r1, 136(r29)               ; intuition_port
    move.l  r2, #INTUITION_OPEN_WINDOW
    move.q  r3, #320
    lsl     r3, r3, #48
    move.q  r14, #200
    lsl     r14, r14, #32
    or      r3, r3, r14
    move.q  r14, #240
    lsl     r14, r14, #16
    or      r3, r3, r14
    or      r3, r3, #200
    load.q  r4, 152(r29)               ; data1 = idcmp_port
    load.q  r5, 144(r29)               ; reply_port
    load.l  r6, 168(r29)               ; share = surface_share
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    bnez    r2, .ab_halt
    load.q  r1, 144(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r3, .ab_halt
    bnez    r1, .ab_halt               ; r1 = INTUI_ERR_*
    store.q r2, 176(r29)               ; window_handle

    ; Send INTUITION_DAMAGE (full window)
    load.q  r1, 136(r29)
    move.l  r2, #INTUITION_DAMAGE
    load.q  r3, 176(r29)
    move.q  r4, r0
    load.q  r5, 144(r29)
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    load.q  r1, 144(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)

    ; ----- IDCMP wait loop -----
.ab_idcmp:
    load.q  r29, (sp)
    load.q  r1, 152(r29)               ; idcmp_port
    syscall #SYS_WAIT_PORT             ; R1=type R2=data0 R3=err
    load.q  r29, (sp)
    bnez    r3, .ab_idcmp
    move.l  r28, #IDCMP_CLOSEWINDOW
    beq     r1, r28, .ab_close
    bra     .ab_idcmp                  ; ignore other classes for M12 demo

.ab_close:
    ; Send INTUITION_CLOSE_WINDOW
    load.q  r1, 136(r29)
    move.l  r2, #INTUITION_CLOSE_WINDOW
    load.q  r3, 176(r29)
    move.q  r4, r0
    load.q  r5, 144(r29)
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    load.q  r1, 144(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)

    move.q  r1, r0
    syscall #SYS_EXIT_TASK

.ab_halt:
    syscall #SYS_YIELD
    bra     .ab_halt

; ----------------------------------------------------------------
; .ab_draw_char — render a single 8x16 topaz glyph into the surface
; ----------------------------------------------------------------
; Inputs:
;   r10 = x (column in pixels, must be >= 0 and <= 312)
;   r11 = y (row in pixels, must be >= 0 and <= 184)
;   r3  = character byte (ASCII; lookup is `font[ch * 16 + row]`)
;   r29 = data base (so r29+1024 is font[0])
; Output: nothing
; Clobbers: r4..r9, r14..r19
; Preserves: r10, r11, r29
; ----------------------------------------------------------------
.ab_draw_char:
    ; Load surface_va, compute pixel base for this glyph cell:
    ;   base = surface_va + y*1280 + x*4    (320-stride = 320*4 = 1280)
    load.q  r4, 160(r29)               ; surface_va
    move.l  r14, #1280
    mulu    r14, r11, r14
    add     r4, r4, r14
    lsl     r14, r10, #2
    add     r4, r4, r14                ; r4 = top-left pixel of cell
    ; Compute glyph pointer: r5 = font_base + ch * 16
    add     r5, r29, #1024             ; font_base
    lsl     r6, r3, #4                 ; ch * 16
    add     r5, r5, r6                 ; r5 = &font[ch][0]
    ; Loop 16 rows
    move.l  r6, #0                     ; row index 0..15
.ab_dc_row:
    move.l  r14, #16
    bge     r6, r14, .ab_dc_done
    load.b  r7, (r5)                   ; r7 = glyph row bits
    add     r5, r5, #1
    ; Render 8 pixels left-to-right
    move.q  r8, r4                     ; pixel cursor
    move.l  r9, #0                     ; col index 0..7
.ab_dc_col:
    move.l  r14, #8
    bge     r9, r14, .ab_dc_col_done
    ; bit = (r7 >> (7 - col)) & 1
    move.l  r14, #7
    sub     r14, r14, r9
    lsr     r15, r7, r14
    and     r15, r15, #1
    beqz    r15, .ab_dc_skip
    ; M12 redesign: black text on grey panel (was white text on teal).
    ; COL_TEXT = 0xFF000000 (RGBA bytes 00,00,00,FF).
    move.l  r16, #0xFF000000
    store.l r16, (r8)
.ab_dc_skip:
    add     r8, r8, #4
    add     r9, r9, #1
    bra     .ab_dc_col
.ab_dc_col_done:
    add     r4, r4, #1280              ; next surface row
    add     r6, r6, #1
    bra     .ab_dc_row
.ab_dc_done:
    rts

; ----------------------------------------------------------------
; .ab_draw_string — render a null-terminated string at (x, y)
; ----------------------------------------------------------------
; Inputs:
;   r10 = x (initial column)
;   r11 = y (row)
;   r12 = string pointer
;   r29 = data base
; Output: r10 advanced past the last drawn glyph
; Clobbers: r3..r9, r14..r19
; ----------------------------------------------------------------
.ab_draw_string:
    sub     sp, sp, #24
    store.q r10, (sp)                  ; save x
    store.q r11, 8(sp)                 ; save y
    store.q r12, 16(sp)                ; save str ptr
.ab_ds_loop:
    load.q  r12, 16(sp)
    load.b  r3, (r12)
    beqz    r3, .ab_ds_done
    ; Draw the char
    load.q  r10, (sp)
    load.q  r11, 8(sp)
    jsr     .ab_draw_char
    ; Advance x by glyph width (8) and string ptr
    load.q  r10, (sp)
    add     r10, r10, #8
    store.q r10, (sp)
    load.q  r12, 16(sp)
    add     r12, r12, #1
    store.q r12, 16(sp)
    bra     .ab_ds_loop
.ab_ds_done:
    add     sp, sp, #24
    rts
prog_about_code_end:

prog_about_data:
    ; offsets 0..127: convention/scratch, unused. Strings live at 192+ to keep
    ; field offsets stable.
    ds.b    128                          ; pad 0..128
    ds.b    8                            ; 128: task_id
    ds.b    8                            ; 136: intuition_port
    ds.b    8                            ; 144: reply_port
    ds.b    8                            ; 152: idcmp_port
    ds.b    8                            ; 160: surface_va
    ds.b    8                            ; 168: surface_share
    ds.b    8                            ; 176: window_handle
    ds.b    8                            ; 184: intuition_open_sigbit
    ds.b    16                           ; 192: OpenLibraryEx waiter outcome scratch
    ds.b    16                           ; 208: pad
    ; offset 224: "About M12 ready" + pad to 32 (test marker)
    dc.b    "About M12 ready", 0
    ds.b    16
    ; offset 256: "intuition.library" + pad to 32 (port name for FindPort/OpenLibraryEx)
    dc.b    "intuition.library", 0
    ds.b    14
    ; offset 288: line 1 — "About IntuitionOS" + pad to 32
    dc.b    "About IntuitionOS", 0
    ds.b    14
    ; offset 320: line 2 — "Protected Exec-inspired kernel" + pad to 32
    dc.b    "Protected Exec-inspired kernel", 0
    ds.b    1
    ; offset 352: line 3 — "intuition.library demonstration" + pad to 32
    dc.b    "intuition.library demonstration", 0
    ; offset 384: line 4 — "All visible services run in user space" — too long
    ; for a 32-byte slot; truncate to fit. Use a 64-byte slot here.
    dc.b    "All services run in user space", 0
    ds.b    1
    ; offset 416: line 5 — "Press Esc to close" + pad to 32
    dc.b    "Press Esc to close", 0
    ds.b    13
    ; offset 448: pad to 1024 (font lives at 1024 for round offsets)
    ds.b    576
    ; offset 1024: embedded Topaz 8x16 font (256 glyphs × 16 bytes = 4096 bytes)
    incbin  "topaz.raw"
prog_about_data_end:
    align   8
prog_about_end:
