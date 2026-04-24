include "template.s"

prog_intuition_library:
    .libmanifest name="intuition.library", version=12, revision=0, type=1, flags=2, msg_abi=0
    dc.l    0, 0
    dc.l    prog_intui_code_end - prog_intui_code
    dc.l    prog_intui_data_end - prog_intui_data
    dc.l    0
    ds.b    12
prog_intui_code:

    ; ===== Preamble: compute data page base (preempt-safe) =====
    ; M12 (Amiga borders / fillrect helper / extended compositor): the
    ; intuition.library code section grew past 4096 bytes and now needs
    ; 2 code pages. The loader places data at code_base + (code_pages+1)
    ; * 4096 = code_base + 0x3000 for 2 code pages, NOT at USER_DATA_BASE
    ; (which is code_base + 0x2000 — only correct for 1 code page).
    ; This matches dos.library's 2-code-page preamble pattern.
    m16_lib_preamble 128

    ; ===== CreatePort("intuition.library") + AddLibrary =====
    m16_lib_register 320, 12, 0, 136, .intui_addlib_done, .intui_exit, .intui_halt
.intui_addlib_done:

    ; ===== CreatePort(NULL) → reply_port (anonymous) =====
    move.q  r1, r0
    move.l  r2, #0
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    bnez    r2, .intui_halt
    store.q r1, 160(r29)               ; data[160] = reply_port

    ; ===== CreatePort(NULL) → my_input_port (anonymous) =====
    move.q  r1, r0
    move.l  r2, #0
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    bnez    r2, .intui_halt
    store.q r1, 168(r29)               ; data[168] = my_input_port

    ; ===== Print banner =====
    m16_lib_print_banner 416, 128, .intui_ban_loop, .intui_ban_id

    ; ===== Main loop =====
.intui_main:
    load.q  r29, (sp)

    ; --- Try to dequeue an intuition.library control message ---
    load.q  r1, 136(r29)               ; intuition_port
    syscall #SYS_GET_MSG
    bnez    r3, .intui_poll_input      ; ERR_AGAIN → no msg

    ; Save fields
    load.q  r29, (sp)
    store.q r1, 272(r29)               ; type
    store.q r2, 280(r29)               ; data0
    store.q r4, 288(r29)               ; data1
    store.q r5, 296(r29)               ; reply_port
    store.q r6, 304(r29)               ; share_handle

    move.l  r28, #MSG_GET_IOSM
    beq     r1, r28, .intui_do_get_iosm
    move.l  r28, #LIB_OP_EXPUNGE
    beq     r1, r28, .intui_do_expunge
    move.l  r28, #INTUITION_OPEN_WINDOW
    beq     r1, r28, .intui_do_open
    move.l  r28, #INTUITION_DAMAGE
    beq     r1, r28, .intui_do_damage
    move.l  r28, #INTUITION_CLOSE_WINDOW
    beq     r1, r28, .intui_do_close
    bra     .intui_reply_badarg

    ; ----- OPEN_WINDOW -----
.intui_do_open:
    load.b  r14, 216(r29)              ; win_in_use
    bnez    r14, .intui_reply_busy

    ; Decode geometry from saved data0: (w<<48)|(h<<32)|(x<<16)|y
    load.q  r14, 280(r29)
    move.q  r15, r14
    lsr     r15, r15, #48
    and     r15, r15, #0xFFFF
    store.l r15, 232(r29)              ; win_w
    move.q  r15, r14
    lsr     r15, r15, #32
    and     r15, r15, #0xFFFF
    store.l r15, 236(r29)              ; win_h
    move.q  r15, r14
    lsr     r15, r15, #16
    and     r15, r15, #0xFFFF
    store.l r15, 224(r29)              ; win_x
    move.q  r15, r14
    and     r15, r15, #0xFFFF
    store.l r15, 228(r29)              ; win_y

    ; ----- Security: bounds-check the rect against the 800x600 screen -----
    ; The DAMAGE handler blits win_w*win_h*4 bytes from win_mapped_va into
    ; screen_va + win_y*3200 + win_x*4. Without this check a malicious
    ; client could supply a rect that walks the destination cursor past
    ; the end of the 1.92 MB screen surface and clobber whatever sits in
    ; intuition.library's address space after it. Reject any geometry
    ; that doesn't lie strictly inside the 800x600 frame, or that has a
    ; zero dimension. Values are already 16-bit unsigned (masked above).
    load.l  r14, 232(r29)              ; win_w
    beqz    r14, .intui_reply_badarg
    move.l  r28, #800
    bgt     r14, r28, .intui_reply_badarg
    load.l  r15, 224(r29)              ; win_x
    add     r14, r14, r15              ; r14 = win_x + win_w
    bgt     r14, r28, .intui_reply_badarg
    load.l  r14, 236(r29)              ; win_h
    beqz    r14, .intui_reply_badarg
    move.l  r28, #600
    bgt     r14, r28, .intui_reply_badarg
    load.l  r15, 228(r29)              ; win_y
    add     r14, r14, r15              ; r14 = win_y + win_h
    bgt     r14, r28, .intui_reply_badarg

    ; idcmp_port = saved data1
    load.q  r14, 288(r29)
    store.q r14, 256(r29)

    ; ----- Lazy display open: only on first OPEN_WINDOW -----
    load.b  r14, 176(r29)              ; display_open
    bnez    r14, .intui_skip_display_init

    ; OpenLibrary("graphics.library", 0) first so exec/dos owns the
    ; lifecycle and autoload, then resolve the compat port transport.
    move.l  r1, #16
    syscall #SYS_ALLOC_SIGNAL
    load.q  r29, (sp)
    bnez    r2, .intui_reply_nomem
    store.q r1, 312(r29)               ; graphics_open_sigbit
.intui_open_gfx_retry:
    load.q  r29, (sp)
    move.l  r14, #0xFFFFFFFF
    store.l r14, 0(r29)               ; waiter status sentinel
    store.l r0, 4(r29)
    store.l r0, 8(r29)
    store.l r0, 12(r29)
    add     r1, r29, #352              ; "graphics.library"
    move.q  r2, r0
    load.q  r3, 312(r29)               ; waiter-owned signal bit
    add     r4, r29, #0                ; 16-byte outcome scratch
    syscall #SYS_OPEN_LIBRARY_EX
    load.q  r29, (sp)
    beqz    r2, .intui_findgfx
    move.l  r14, #ERR_AGAIN
    bne     r2, r14, .intui_open_gfx_fail
    load.l  r14, 0(r29)                ; waiter status
    move.l  r15, #0xFFFFFFFF
    bne     r14, r15, .intui_open_gfx_done
    load.q  r14, 312(r29)
    move.q  r1, #1
    lsl     r1, r1, r14
    syscall #SYS_WAIT
    load.q  r29, (sp)
    bnez    r2, .intui_open_gfx_yield
    load.l  r14, 0(r29)                ; waiter status
    move.l  r15, #0xFFFFFFFF
    beq     r14, r15, .intui_open_gfx_yield
    ; fall through once waiter status is real
.intui_open_gfx_done:
    store.q r1, 264(r29)               ; graphics_library_token
    beqz    r14, .intui_findgfx
.intui_open_gfx_fail:
    load.q  r14, 312(r29)              ; graphics_open_sigbit
    beqz    r14, .intui_reply_nomem
    move.q  r1, r14
    syscall #SYS_FREE_SIGNAL
    load.q  r29, (sp)
    store.q r0, 312(r29)
    bra     .intui_reply_nomem
.intui_open_gfx_yield:
    syscall #SYS_YIELD
    bra     .intui_open_gfx_retry
.intui_findgfx:
    load.q  r29, (sp)
    add     r1, r29, #352              ; "graphics.library"
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    beqz    r2, .intui_findgfx_ok
    syscall #SYS_YIELD
    bra     .intui_findgfx
.intui_findgfx_ok:
    store.q r1, 144(r29)               ; graphics_port
    load.q  r14, 312(r29)              ; graphics_open_sigbit
    beqz    r14, .intui_findgfx_sigfree_done
    move.q  r1, r14
    syscall #SYS_FREE_SIGNAL
    load.q  r29, (sp)
    store.q r0, 312(r29)
.intui_findgfx_sigfree_done:

    ; FindPort("input.device")
.intui_findin:
    load.q  r29, (sp)
    add     r1, r29, #384              ; "input.device"
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    beqz    r2, .intui_findin_ok
    syscall #SYS_YIELD
    bra     .intui_findin
.intui_findin_ok:
    store.q r1, 152(r29)               ; input_port

    ; AllocMem(1920000, MEMF_PUBLIC|MEMF_CLEAR) — 800x600 RGBA32 screen surface
    ; (M12: bumped from 1228800 / 640x480 to 1920000 / 800x600 to give clients
    ; more screen real estate. Stride = 800*4 = 3200.)
    move.l  r1, #1920000
    move.l  r2, #0x10001
    syscall #SYS_ALLOC_MEM             ; R1=va R2=err R3=share
    load.q  r29, (sp)
    bnez    r2, .intui_display_init_fail
    store.q r1, 200(r29)               ; screen_va
    store.l r3, 208(r29)               ; screen_share

    ; M12 redesign: fill the entire 800x600 screen surface with the
    ; AmigaOS 3.9 / ReAction prefs grey (COL_SCREEN_BG = 0xFFD4D0C8)
    ; so the desktop reads as a system surface, not a black void.
    load.q  r14, 200(r29)              ; r14 = screen_va cursor
    move.l  r15, #1920000              ; r15 = remaining bytes
    move.l  r16, #0xFFD4D0C8           ; COL_SCREEN_BG (RGBA bytes C8,D0,D4,FF)
.intui_screen_bg:
    beqz    r15, .intui_screen_bg_done
    store.l r16, (r14)
    add     r14, r14, #4
    sub     r15, r15, #4
    bra     .intui_screen_bg
.intui_screen_bg_done:

    ; GFX_OPEN_DISPLAY(0, 0)
    load.q  r1, 144(r29)
    move.l  r2, #GFX_OPEN_DISPLAY
    move.q  r3, r0
    move.q  r4, r0
    load.q  r5, 160(r29)               ; reply_port
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    bnez    r2, .intui_display_init_fail
    load.q  r1, 160(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r3, .intui_display_init_fail
    bnez    r1, .intui_display_init_fail ; r1 = err code from gfx
    store.q r2, 184(r29)               ; display_handle

    ; GFX_REGISTER_SURFACE — width=800, height=600, format=RGBA32, stride=3200
    load.q  r1, 144(r29)
    move.l  r2, #GFX_REGISTER_SURFACE
    load.q  r3, 184(r29)               ; display_handle
    move.q  r4, #800
    lsl     r4, r4, #48
    move.q  r14, #600
    lsl     r14, r14, #32
    or      r4, r4, r14
    move.q  r14, #1
    lsl     r14, r14, #16
    or      r4, r4, r14
    or      r4, r4, #3200              ; stride bytes (800 * 4 = 3200)
    load.q  r5, 160(r29)
    load.l  r6, 208(r29)               ; share = own screen_share
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    bnez    r2, .intui_display_init_fail
    load.q  r1, 160(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r3, .intui_display_init_fail
    bnez    r1, .intui_display_init_fail
    store.q r2, 192(r29)               ; surface_handle

    ; INPUT_OPEN(my_input_port). M12 fix: input.device is single-subscriber,
    ; so INPUT_OPEN may legitimately return INPUT_ERR_BUSY when another
    ; client (e.g. a test rig) already owns the subscription. We must track
    ; whether WE actually acquired it — otherwise the close path would
    ; unconditionally INPUT_CLOSE and clear the OTHER subscriber's slot.
    load.q  r1, 152(r29)
    move.l  r2, #INPUT_OPEN
    load.q  r3, 168(r29)               ; my_input_port
    move.q  r4, r0
    load.q  r5, 160(r29)
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    bnez    r2, .intui_display_init_fail ; kernel-level PutMsg failure
    load.q  r1, 160(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r3, .intui_display_init_fail ; kernel-level WaitPort failure
    ; r1 = mn_Type from input.device's reply: 0 = INPUT_ERR_OK, non-0 = busy/badarg.
    ; We default input_subscribed to 0, then set to 1 only on confirmed OK.
    store.b r0, 177(r29)               ; input_subscribed = 0 (defensive)
    bnez    r1, .intui_input_done      ; INPUT_ERR_BUSY (or other) — DON'T own it
    move.b  r14, #1
    store.b r14, 177(r29)              ; input_subscribed = 1
.intui_input_done:
    move.b  r14, #1
    store.b r14, 176(r29)              ; display_open = 1

.intui_skip_display_init:
    ; MapShared(client window buffer share)
    load.l  r14, 304(r29)              ; saved share_handle
    store.l r14, 240(r29)              ; win_share
    move.q  r1, r14
    move.l  r2, #MAPF_READ
    syscall #SYS_MAP_SHARED            ; R1=va R2=err
    load.q  r29, (sp)
    bnez    r2, .intui_reply_badarg
    store.q r1, 248(r29)               ; win_mapped_va

    move.b  r14, #1
    store.b r14, 216(r29)              ; win_in_use = 1

    ; Reply OK with window_handle = 1
    load.q  r1, 296(r29)               ; reply_port
    move.l  r2, #INTUI_ERR_OK
    move.l  r3, #1                     ; window_handle
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    bra     .intui_main

    ; ----- DAMAGE -----
.intui_do_damage:
    load.b  r14, 216(r29)              ; win_in_use
    beqz    r14, .intui_reply_badhandle
    load.q  r14, 280(r29)              ; saved data0 = window_handle
    move.l  r28, #1
    bne     r14, r28, .intui_reply_badhandle

    ; Composite the entire window into the screen.
    ; Source: win_mapped_va (win_w * win_h * 4 bytes, stride = win_w * 4)
    ; Dest:   screen_va + win_y*3200 + win_x*4
    ; (M12: ignore the per-call dirty rect; do a full window blit. The rect
    ; argument is reserved for M12.1 once GFX_PRESENT carries it through.)
    load.q  r20, 248(r29)              ; src
    load.q  r21, 200(r29)              ; screen_va base
    load.l  r22, 224(r29)              ; win_x
    load.l  r23, 228(r29)              ; win_y
    load.l  r24, 232(r29)              ; win_w
    load.l  r25, 236(r29)              ; win_h
    ; dst = screen_va + win_y*3200 + win_x*4 (M12: 800x600 stride = 3200)
    move.l  r14, #3200
    mulu    r14, r23, r14
    add     r21, r21, r14
    lsl     r14, r22, #2
    add     r21, r21, r14
    ; src_stride = win_w*4, dst_stride = 3200
.intui_blit_row:
    beqz    r25, .intui_blit_done
    move.q  r14, r20                   ; src cursor
    move.q  r15, r21                   ; dst cursor
    move.l  r16, r24                   ; cols
.intui_blit_col:
    beqz    r16, .intui_blit_row_done
    load.l  r17, (r14)
    store.l r17, (r15)
    add     r14, r14, #4
    add     r15, r15, #4
    sub     r16, r16, #1
    bra     .intui_blit_col
.intui_blit_row_done:
    lsl     r14, r24, #2
    add     r20, r20, r14              ; advance src by win_w*4
    add     r21, r21, #3200            ; advance dst by screen stride
    sub     r25, r25, #1
    bra     .intui_blit_row
.intui_blit_done:

    ; ============================================================
    ; AmigaOS 3.9 / ReAction-style window decoration (M12 redesign)
    ; ------------------------------------------------------------
    ; Faithful OS 3.9 palette: blue title furniture, grey body. All
    ; constants are in RGBA byte order (chip is byte[0]=R, so an asm
    ; literal 0xAARRGGBB stores in memory as BB,GG,RR,AA):
    ;
    ;   COL_SCREEN_BG       0xFFD4D0C8  desktop grey (filled at display open)
    ;   COL_WIN_FACE        0xFFD4D0C8  window face / gadget body
    ;   COL_PANEL_BG        0xFFDCD8D0  recessed content panel interior
    ;   COL_HILITE          0xFFFFFFFF  raised highlight (top + left bevel)
    ;   COL_SHADOW          0xFF808080  raised shadow (bottom + right bevel)
    ;   COL_DARK            0xFF000000  outer outline + close gadget mark
    ;   COL_TITLE_BLUE      0xFFCC7A2C  title bar main fill
    ;   COL_TITLE_BLUE_LIGHT 0xFFE6A25A title bar top highlight (1 px)
    ;   COL_TITLE_BLUE_DARK  0xFF9A4E16 title bar bottom shadow (1 px)
    ;
    ; Layered draw order (the user buffer has already been blitted):
    ;   1. Raised window bevel at the very window edge (white top+left
    ;      at (x, y, w, 1) / (x, y, 1, h); grey shadow bottom+right at
    ;      (x, y+h-1, w, 1) / (x+w-1, y, 1, h))
    ;   2. Title bar BLUE fill at (x+2, y+2, w-4, 16)
    ;   3. Title bar top highlight (light blue, 1 px) at (x+2, y+2, w-4, 1)
    ;   4. Title bar bottom shadow (dark blue, 1 px) at (x+2, y+17, w-4, 1)
    ;   5. Close gadget (top-left, flush with title bar) — bevel + grey
    ;      face + black centre mark
    ;   6. Depth gadget (top-right) — bevel + grey face + back/front icon
    ;   7. Title text "About IntuitionOS" in black Topaz, left of the
    ;      close gadget
    ;   8. Recessed content-panel BORDER (shadow top+left, highlight
    ;      bottom+right). The interior is NOT filled — that area stays
    ;      as the user buffer's pixels (the About app fills its buffer
    ;      with COL_PANEL_BG so the panel interior reads as grey).
    ;
    ; The blit loop above destroys r25 (decrements win_h to 0), so
    ; we reload all four window dimensions from the data page before
    ; using them as fillrect inputs.
    ; ============================================================
    load.l  r22, 224(r29)              ; win_x
    load.l  r23, 228(r29)              ; win_y
    load.l  r24, 232(r29)              ; win_w
    load.l  r25, 236(r29)              ; win_h

    ; --- 1. Raised window bevel at the very window edge ---
    ; White hilite top + left, grey shadow bottom + right. No outer
    ; black border — the bevel is the entire frame, OS 3.9 style.
    move.q  r6, r22                    ; top hilite: (x, y, w, 1)
    move.q  r7, r23
    move.q  r8, r24
    move.l  r9, #1
    move.l  r17, #0xFFFFFFFF           ; COL_HILITE
    jsr     .intui_fillrect
    move.q  r6, r22                    ; left hilite: (x, y, 1, h)
    move.q  r7, r23
    move.l  r8, #1
    move.q  r9, r25
    move.l  r17, #0xFFFFFFFF
    jsr     .intui_fillrect
    move.q  r6, r22                    ; bottom shadow: (x, y+h-1, w, 1)
    add     r7, r23, r25
    sub     r7, r7, #1
    move.q  r8, r24
    move.l  r9, #1
    move.l  r17, #0xFF808080           ; COL_SHADOW
    jsr     .intui_fillrect
    add     r6, r22, r24               ; right shadow: (x+w-1, y, 1, h)
    sub     r6, r6, #1
    move.q  r7, r23
    move.l  r8, #1
    move.q  r9, r25
    move.l  r17, #0xFF808080
    jsr     .intui_fillrect

    ; --- 3. Title bar BLUE main fill (rows y+2 .. y+17, 16 px tall) ---
    ; Sits inside the bevel at (x+2, y+2, w-4, 16). The body of the
    ; window below the title bar is the user buffer's pixels (the
    ; About app fills its buffer with COL_PANEL_BG so the body reads
    ; as recessed grey).
    add     r6, r22, #2
    add     r7, r23, #2
    sub     r8, r24, #4
    move.l  r9, #16                    ; title_h
    move.l  r17, #0xFFCC7A2C           ; COL_TITLE_BLUE
    jsr     .intui_fillrect

    ; --- 4. Title bar top highlight (light blue, 1 px) ---
    add     r6, r22, #2
    add     r7, r23, #2
    sub     r8, r24, #4
    move.l  r9, #1
    move.l  r17, #0xFFE6A25A           ; COL_TITLE_BLUE_LIGHT
    jsr     .intui_fillrect

    ; --- 5. Title bar bottom shadow (dark blue, 1 px) ---
    add     r6, r22, #2
    add     r7, r23, #17               ; y + 17 = last row of 16-px title bar
    sub     r8, r24, #4
    move.l  r9, #1
    move.l  r17, #0xFF9A4E16           ; COL_TITLE_BLUE_DARK
    jsr     .intui_fillrect

    ; --- 6. Close gadget (top-left, flush with title bar) ---
    ; Sits inside the inner bevel at (gx=x+2, gy=y+2), 18x16. Bevel
    ; outline (white top+left, grey shadow bottom+right), grey face,
    ; centred black mark. No outer black border — the inner bevel IS
    ; the gadget frame, and it sits ON the blue title bar.
    add     r6, r22, #2                ; gx = x+2
    add     r7, r23, #2                ; gy = y+2
    move.l  r8, #18
    move.l  r9, #1
    move.l  r17, #0xFFFFFFFF           ; bevel top hilite (gx, gy, 18, 1)
    jsr     .intui_fillrect
    add     r6, r22, #2
    add     r7, r23, #2
    move.l  r8, #1
    move.l  r9, #16
    move.l  r17, #0xFFFFFFFF           ; bevel left hilite (gx, gy, 1, 16)
    jsr     .intui_fillrect
    add     r6, r22, #2                ; bevel bottom shadow (gx, gy+15, 18, 1)
    add     r7, r23, #17
    move.l  r8, #18
    move.l  r9, #1
    move.l  r17, #0xFF808080
    jsr     .intui_fillrect
    add     r6, r22, #19               ; bevel right shadow (gx+17, gy, 1, 16)
    add     r7, r23, #2
    move.l  r8, #1
    move.l  r9, #16
    move.l  r17, #0xFF808080
    jsr     .intui_fillrect
    add     r6, r22, #3                ; face fill (gx+1, gy+1, 16, 14)
    add     r7, r23, #3
    move.l  r8, #16
    move.l  r9, #14
    move.l  r17, #0xFFD4D0C8           ; COL_WIN_FACE
    jsr     .intui_fillrect
    ; Centre mark: 4x4 black square. Sized so that the gadget detail
    ; sample at screen (248, 208) — window-rel (8, 8), gadget-rel
    ; (6, 6) — lands inside the mark.
    add     r6, r22, #6                ; mark (gx+4, gy+5, 6, 6)
    add     r7, r23, #7
    move.l  r8, #6
    move.l  r9, #6
    move.l  r17, #0xFF000000
    jsr     .intui_fillrect

    ; --- 7. Depth gadget (top-right, flush with title bar) ---
    ; gx = win_x + win_w - 20, gy = win_y + 2, 18x16. Same bevel
    ; treatment as the close gadget, with the AmigaOS depth icon
    ; (two overlapping rectangles) drawn in the centre.
    add     r6, r22, r24
    sub     r6, r6, #20                ; gx = x + w - 20
    add     r7, r23, #2                ; gy = y + 2
    move.l  r8, #18
    move.l  r9, #1
    move.l  r17, #0xFFFFFFFF           ; bevel top hilite
    jsr     .intui_fillrect
    add     r6, r22, r24
    sub     r6, r6, #20
    add     r7, r23, #2
    move.l  r8, #1
    move.l  r9, #16
    move.l  r17, #0xFFFFFFFF           ; bevel left hilite
    jsr     .intui_fillrect
    add     r6, r22, r24
    sub     r6, r6, #20
    add     r7, r23, #17
    move.l  r8, #18
    move.l  r9, #1
    move.l  r17, #0xFF808080           ; bevel bottom shadow
    jsr     .intui_fillrect
    add     r6, r22, r24
    sub     r6, r6, #3                 ; gx+17 = x + w - 3
    add     r7, r23, #2
    move.l  r8, #1
    move.l  r9, #16
    move.l  r17, #0xFF808080           ; bevel right shadow
    jsr     .intui_fillrect
    add     r6, r22, r24
    sub     r6, r6, #19                ; gx+1
    add     r7, r23, #3                ; gy+1
    move.l  r8, #16
    move.l  r9, #14
    move.l  r17, #0xFFD4D0C8           ; COL_WIN_FACE
    jsr     .intui_fillrect
    ; Depth icon: "back" rectangle outline (gx+4, gy+5, 7, 5)
    add     r6, r22, r24
    sub     r6, r6, #16                ; gx+4
    add     r7, r23, #7                ; gy+5
    move.l  r8, #7
    move.l  r9, #1
    move.l  r17, #0xFF000000           ; top
    jsr     .intui_fillrect
    add     r6, r22, r24
    sub     r6, r6, #16
    add     r7, r23, #11               ; gy+9
    move.l  r8, #7
    move.l  r9, #1
    move.l  r17, #0xFF000000           ; bottom
    jsr     .intui_fillrect
    add     r6, r22, r24
    sub     r6, r6, #16
    add     r7, r23, #7
    move.l  r8, #1
    move.l  r9, #5
    move.l  r17, #0xFF000000           ; left
    jsr     .intui_fillrect
    add     r6, r22, r24
    sub     r6, r6, #10                ; gx+10
    add     r7, r23, #7
    move.l  r8, #1
    move.l  r9, #5
    move.l  r17, #0xFF000000           ; right
    jsr     .intui_fillrect
    ; Depth icon: "front" rectangle outline (gx+7, gy+3, 7, 5)
    add     r6, r22, r24
    sub     r6, r6, #13                ; gx+7
    add     r7, r23, #5                ; gy+3
    move.l  r8, #7
    move.l  r9, #1
    move.l  r17, #0xFF000000           ; top
    jsr     .intui_fillrect
    add     r6, r22, r24
    sub     r6, r6, #13
    add     r7, r23, #9                ; gy+7
    move.l  r8, #7
    move.l  r9, #1
    move.l  r17, #0xFF000000           ; bottom
    jsr     .intui_fillrect
    add     r6, r22, r24
    sub     r6, r6, #13
    add     r7, r23, #5
    move.l  r8, #1
    move.l  r9, #5
    move.l  r17, #0xFF000000           ; left
    jsr     .intui_fillrect
    add     r6, r22, r24
    sub     r6, r6, #7                 ; gx+13
    add     r7, r23, #5
    move.l  r8, #1
    move.l  r9, #5
    move.l  r17, #0xFF000000           ; right
    jsr     .intui_fillrect

    ; --- 8. Title text "About IntuitionOS" in black Topaz ---
    ; tx = win_x + 24 (after close gadget at x+2..x+19, with 4px gap).
    ; The title string is 17 chars × 8 px = 136 px wide, so it spans
    ; columns x+24 .. x+159. With win_x = 240 that ends at col 399,
    ; leaving the title-bar drag/sample column 400 free of glyph
    ; pixels for the (400, 210) / (400, 217) test samples.
    ; ty = win_y + 4 (top of title bar interior, 2px below highlight)
    add     r10, r22, #24              ; r10 = x  (input to .intui_draw_string)
    add     r11, r23, #4               ; r11 = y
    add     r12, r29, #460             ; r12 = string ptr (data offset 460)
    jsr     .intui_draw_string

    ; --- 9. Recessed content panel border ---
    ; px = x+8, py = y+24, pw = w-16, ph = h-32
    ; Top shadow line: (px, py, pw, 1)
    add     r6, r22, #8
    add     r7, r23, #24
    sub     r8, r24, #16
    move.l  r9, #1
    move.l  r17, #0xFF808080           ; COL_SHADOW
    jsr     .intui_fillrect
    ; Left shadow line: (px, py, 1, ph)
    add     r6, r22, #8
    add     r7, r23, #24
    move.l  r8, #1
    sub     r9, r25, #32
    move.l  r17, #0xFF808080
    jsr     .intui_fillrect
    ; Bottom highlight line: (px, py+ph-1, pw, 1)
    add     r6, r22, #8
    add     r7, r23, r25
    sub     r7, r7, #9                 ; y + h - 9
    sub     r8, r24, #16
    move.l  r9, #1
    move.l  r17, #0xFFFFFFFF           ; COL_HILITE
    jsr     .intui_fillrect
    ; Right highlight line: (px+pw-1, py, 1, ph)
    add     r6, r22, r24
    sub     r6, r6, #9                 ; x + w - 9
    add     r7, r23, #24
    move.l  r8, #1
    sub     r9, r25, #32
    move.l  r17, #0xFFFFFFFF
    jsr     .intui_fillrect
.intui_cg_done:

    ; GFX_PRESENT(surface_handle)
    load.q  r1, 144(r29)
    move.l  r2, #GFX_PRESENT
    load.q  r3, 192(r29)
    move.q  r4, r0
    load.q  r5, 160(r29)
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    load.q  r1, 160(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)

    ; Reply OK
    load.q  r1, 296(r29)
    move.l  r2, #INTUI_ERR_OK
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    bra     .intui_main

    ; ----- CLOSE_WINDOW -----
    ;
    ; M12 fix: this path was leaking pages and shared-memory refs across
    ; repeated open/close cycles. The original M12 close only cleared the
    ; display_open flag and dropped INPUT_CLOSE/GFX_UNREGISTER/GFX_CLOSE,
    ; never touching SYS_FREE_MEM on either the AllocMem'd screen surface
    ; or the SYS_MAP_SHARED'd client window buffer. Both are now released.
    ;
    ; Order matters:
    ;   1. Free the mapped CLIENT window buffer (decrements that share's
    ;      refcount; the client task still holds its own private mapping).
    ;   2. Send INPUT_CLOSE (only if WE own the subscription — see the
    ;      input_subscribed flag set in the open path).
    ;   3. Send GFX_UNREGISTER_SURFACE so graphics.library stops using
    ;      our screen surface as the scanout source.
    ;   4. Send GFX_CLOSE_DISPLAY so graphics.library returns the chip to
    ;      text mode.
    ;   5. Free our OWN screen surface (final share refcount decrement).
.intui_do_close:
    load.b  r14, 216(r29)
    beqz    r14, .intui_reply_badhandle
    load.q  r14, 280(r29)              ; window_handle
    move.l  r28, #1
    bne     r14, r28, .intui_reply_badhandle

    store.b r0, 216(r29)               ; win_in_use = 0
    store.q r0, 256(r29)               ; idcmp_port = 0

    ; --- 1. FreeMem the mapped client window buffer ---
    ; Size = win_w * win_h * 4 bytes (RGBA32). do_free_mem rounds up to a
    ; page count and matches against the region table. The client allocated
    ; the buffer with AllocMem(MEMF_PUBLIC), so this is a SHARED region —
    ; FreeMem decrements the shared object's refcount; the client side's
    ; mapping survives.
    load.l  r14, 232(r29)              ; win_w
    load.l  r15, 236(r29)              ; win_h
    mulu    r14, r14, r15
    lsl     r14, r14, #2               ; * 4 bytes per pixel
    load.q  r1, 248(r29)               ; win_mapped_va
    move.q  r2, r14
    syscall #SYS_FREE_MEM
    load.q  r29, (sp)
    ; Best-effort: even if FreeMem failed (size mismatch), proceed with
    ; teardown. Clear the cached fields so a re-open computes fresh.
    store.q r0, 248(r29)               ; clear win_mapped_va
    store.q r0, 240(r29)               ; clear win_share

    ; --- 2. INPUT_CLOSE (only if we own the subscription) ---
    load.b  r14, 177(r29)              ; input_subscribed
    beqz    r14, .intui_close_skip_input
    load.q  r1, 152(r29)
    move.l  r2, #INPUT_CLOSE
    move.q  r3, r0
    move.q  r4, r0
    load.q  r5, 160(r29)
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    load.q  r1, 160(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    store.b r0, 177(r29)               ; input_subscribed = 0
.intui_close_skip_input:

    ; --- 3. GFX_UNREGISTER_SURFACE ---
    load.q  r1, 144(r29)
    move.l  r2, #GFX_UNREGISTER_SURFACE
    load.q  r3, 192(r29)
    move.q  r4, r0
    load.q  r5, 160(r29)
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    load.q  r1, 160(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)

    ; --- 4. GFX_CLOSE_DISPLAY ---
    load.q  r1, 144(r29)
    move.l  r2, #GFX_CLOSE_DISPLAY
    load.q  r3, 184(r29)
    move.q  r4, r0
    load.q  r5, 160(r29)
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    load.q  r1, 160(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)

    ; --- 5. Close tracked graphics.library opener ---
    load.q  r1, 264(r29)               ; graphics_library_token
    beqz    r1, .intui_close_graphics_done
    syscall #SYS_CLOSE_LIBRARY
    load.q  r29, (sp)
    store.q r0, 264(r29)
.intui_close_graphics_done:

    ; --- 6. FreeMem our own screen surface ---
    ; The screen surface was AllocMem(1920000, MEMF_PUBLIC|MEMF_CLEAR) so
    ; it lives in the SHARED region table. FreeMem decrements the shared
    ; object refcount; if graphics.library has already released its
    ; mapping (M12: it does in the gfx_h_unreg_surf companion fix below),
    ; the backing pages are released here.
    load.q  r1, 200(r29)               ; screen_va
    move.l  r2, #1920000               ; 800 * 600 * 4 bytes (M12: was 1228800)
    syscall #SYS_FREE_MEM
    load.q  r29, (sp)

    ; Clear all display state so a future OPEN_WINDOW lazily re-acquires
    ; from a clean slate.
    store.q r0, 200(r29)               ; screen_va = 0
    store.q r0, 208(r29)               ; screen_share = 0
    store.q r0, 184(r29)               ; display_handle = 0
    store.q r0, 192(r29)               ; surface_handle = 0
    store.b r0, 176(r29)               ; display_open = 0

    ; Reply OK
    load.q  r1, 296(r29)
    move.l  r2, #INTUI_ERR_OK
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    bra     .intui_main

    ; ----- Common error replies -----
.intui_reply_busy:
    load.q  r1, 296(r29)
    move.l  r2, #INTUI_ERR_BUSY
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    bra     .intui_main
.intui_reply_badarg:
    load.q  r1, 296(r29)
    move.l  r2, #INTUI_ERR_BADARG
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    bra     .intui_main
.intui_reply_badhandle:
    load.q  r1, 296(r29)
    move.l  r2, #INTUI_ERR_BADHANDLE
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    bra     .intui_main
.intui_display_init_fail:
    load.q  r14, 312(r29)              ; graphics_open_sigbit
    beqz    r14, .intui_reply_nomem
    move.q  r1, r14
    syscall #SYS_FREE_SIGNAL
    load.q  r29, (sp)
    store.q r0, 312(r29)
    load.q  r1, 264(r29)               ; graphics_library_token
    beqz    r1, .intui_reply_nomem
    syscall #SYS_CLOSE_LIBRARY
    load.q  r29, (sp)
    store.q r0, 264(r29)
    bra     .intui_reply_nomem
.intui_reply_nomem:
    load.q  r1, 296(r29)
    move.l  r2, #INTUI_ERR_NOMEM
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    bra     .intui_main

.intui_do_expunge:
    load.b  r14, 176(r29)              ; display_open
    bnez    r14, .intui_expunge_refuse
    load.b  r14, 216(r29)              ; win_in_use
    bnez    r14, .intui_expunge_refuse
    m16_lib_accept_expunge 280, 288, .intui_main
.intui_expunge_refuse:
    m16_lib_refuse_expunge 280, 288, .intui_main

.intui_do_get_iosm:
    load.q  r14, 304(r29)
    beqz    r14, .intui_get_iosm_reply_badarg
    move.q  r25, r14
    load.q  r26, 296(r29)
    move.q  r1, r25
    move.l  r2, #MAPF_WRITE
    syscall #SYS_MAP_SHARED
    load.q  r29, (sp)
    bnez    r2, .intui_get_iosm_maperr
    move.q  r23, r1
    move.q  r24, r1
    move.l  r28, #1
    bne     r3, r28, .intui_get_iosm_badarg_free
    add     r14, r29, #(prog_intui_iosm - prog_intui_data)
    move.l  r15, #(IOSM_SIZE / 8)
.intui_get_iosm_copy:
    load.q  r16, (r14)
    store.q r16, (r24)
    add     r14, r14, #8
    add     r24, r24, #8
    sub     r15, r15, #1
    bnez    r15, .intui_get_iosm_copy
    move.q  r1, r23
    move.l  r2, #IOSM_SIZE
    syscall #SYS_FREE_MEM
    load.q  r29, (sp)
    load.q  r1, 296(r29)
    move.q  r2, r0
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    bra     .intui_main
.intui_get_iosm_badarg_free:
    move.q  r1, r23
    move.q  r2, r3
    lsl     r2, r2, #12
    syscall #SYS_FREE_MEM
    load.q  r29, (sp)
    bra     .intui_get_iosm_reply_badarg
.intui_get_iosm_reply_badarg:
    load.q  r1, 296(r29)
    move.l  r2, #ERR_BADARG
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    bra     .intui_main
.intui_get_iosm_maperr:
    load.q  r1, 296(r29)
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    bra     .intui_main

    ; --- Poll input.device for raw events; route as IDCMP-* to idcmp_port ---
.intui_poll_input:
    load.q  r29, (sp)
    load.b  r14, 216(r29)              ; win_in_use
    beqz    r14, .intui_yield
    load.q  r24, 256(r29)              ; idcmp_port
    beqz    r24, .intui_yield

    load.q  r1, 168(r29)               ; my_input_port
    syscall #SYS_GET_MSG
    bnez    r3, .intui_yield           ; ERR_AGAIN → nothing

    move.l  r28, #INPUT_EVENT
    bne     r1, r28, .intui_yield      ; unknown opcode → drop

    ; r2 = data0 = (event_type<<24)|(scancode<<16)|(modifiers<<8)
    ;            for IE_MOUSE_BTN: also (buttons<<16)
    ; r4 = data1 = (mx<<48)|(my<<32)|seq32
    move.q  r14, r2
    lsr     r14, r14, #24
    and     r14, r14, #0xFF            ; event type

    move.l  r28, #IE_KEY_DOWN
    beq     r14, r28, .intui_ev_key
    move.l  r28, #IE_MOUSE_MOVE
    beq     r14, r28, .intui_ev_move
    move.l  r28, #IE_MOUSE_BTN
    beq     r14, r28, .intui_ev_btn
    bra     .intui_yield

.intui_ev_key:
    ; scancode = (data0 >> 16) & 0xFF
    move.q  r14, r2
    lsr     r14, r14, #16
    and     r14, r14, #0xFF
    ; If scancode == 0x01 (Esc): IDCMP_CLOSEWINDOW
    move.l  r28, #1
    beq     r14, r28, .intui_ev_close

    ; Else IDCMP_RAWKEY: data0 = (scancode<<8)|mods, data1 = seq32
    move.q  r15, r2
    lsr     r15, r15, #8
    and     r15, r15, #0xFF            ; mods
    lsl     r14, r14, #8
    or      r14, r14, r15
    load.q  r29, (sp)
    load.q  r1, 256(r29)
    move.l  r2, #IDCMP_RAWKEY
    move.q  r3, r14
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    bra     .intui_yield

.intui_ev_close:
    load.q  r29, (sp)
    load.q  r1, 256(r29)
    move.l  r2, #IDCMP_CLOSEWINDOW
    move.l  r3, #1                     ; window_handle
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    bra     .intui_yield

.intui_ev_move:
    ; data1 contains (mx<<48)|(my<<32)|seq
    move.q  r14, r4
    lsr     r14, r14, #48
    and     r14, r14, #0xFFFF          ; mx
    move.q  r15, r4
    lsr     r15, r15, #32
    and     r15, r15, #0xFFFF          ; my
    ; Translate to window-local coords (signed: 32-bit)
    load.q  r29, (sp)
    load.l  r16, 224(r29)              ; win_x
    load.l  r17, 228(r29)              ; win_y
    sub     r14, r14, r16              ; lx
    sub     r15, r15, r17              ; ly
    ; data0 = (lx<<32) | (ly & 0xFFFFFFFF)
    lsl     r14, r14, #32
    and     r15, r15, #0xFFFFFFFF
    or      r14, r14, r15
    load.q  r1, 256(r29)
    move.l  r2, #IDCMP_MOUSEMOVE
    move.q  r3, r14
    move.q  r4, r4                     ; data1 untouched (seq high bits)
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    bra     .intui_yield

.intui_ev_btn:
    ; data0 = (IE_MOUSE_BTN<<24)|(buttons<<16)
    move.q  r14, r2
    lsr     r14, r14, #16
    and     r14, r14, #0xFF            ; buttons
    move.q  r15, r4                    ; mx in high 16 bits
    lsr     r15, r15, #48
    and     r15, r15, #0xFFFF
    move.q  r16, r4
    lsr     r16, r16, #32
    and     r16, r16, #0xFFFF
    ; Window-local coords (need win_x/y)
    load.q  r29, (sp)
    load.l  r17, 224(r29)
    load.l  r18, 228(r29)
    sub     r15, r15, r17              ; lx
    sub     r16, r16, r18              ; ly

    ; If button-down (bit 0 set) AND inside close gadget rect, send CLOSEWINDOW
    ; Close gadget = [0..16) × [0..16)
    and     r19, r14, #1
    beqz    r19, .intui_ev_btn_send
    bgez    r15, .intui_ev_btn_lx_ok
    bra     .intui_ev_btn_send
.intui_ev_btn_lx_ok:
    move.l  r28, #INTUI_CLOSE_GADGET_W
    bge     r15, r28, .intui_ev_btn_send
    bgez    r16, .intui_ev_btn_ly_ok
    bra     .intui_ev_btn_send
.intui_ev_btn_ly_ok:
    move.l  r28, #INTUI_WIN_TITLE_H
    bge     r16, r28, .intui_ev_btn_send
    ; Inside close gadget
    load.q  r1, 256(r29)
    move.l  r2, #IDCMP_CLOSEWINDOW
    move.l  r3, #1
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    bra     .intui_yield

.intui_ev_btn_send:
    ; data0 = (buttons<<32)|(state) — keep simple: state = buttons (no edge tracking)
    lsl     r14, r14, #32
    or      r14, r14, r14
    ; data1 = (lx<<32)|(ly)
    lsl     r15, r15, #32
    and     r16, r16, #0xFFFFFFFF
    or      r15, r15, r16
    load.q  r1, 256(r29)
    move.l  r2, #IDCMP_MOUSEBUTTONS
    move.q  r3, r14
    move.q  r4, r15
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    bra     .intui_yield

.intui_yield:
    syscall #SYS_YIELD
    bra     .intui_main

.intui_halt:
    syscall #SYS_YIELD
    bra     .intui_halt

.intui_exit:
    move.q  r1, r0
    syscall #SYS_EXIT_TASK

; ----------------------------------------------------------------
; .intui_fillrect — fill a screen-space rectangle with a color
; ----------------------------------------------------------------
; Inputs:
;   r6  = rect_x   (column, in pixels, on the 800-wide screen)
;   r7  = rect_y   (row)
;   r8  = rect_w   (width in pixels)
;   r9  = rect_h   (height in pixels — rows)
;   r17 = color    (RGBA32, byte[0]=R per chip ordering)
;   r29 = data base (intuition.library data page)
; Output: nothing
; Clobbers: r5, r11, r12, r13, r14, r15, r16
; Preserves: r6..r9, r17, r29 (caller's loop variables stay live)
;
; Stride is hardcoded to 3200 (800 pixels × 4 bytes/pixel) which matches
; the M12 intuition.library screen surface dimensions. Bounds-checks the
; rect against the 800x600 frame and silently clips zero-or-negative
; sizes so the caller can pass arbitrary expressions.
; ----------------------------------------------------------------
.intui_fillrect:
    ; Reject empty rects
    blez    r8, .intui_fr_ret
    blez    r9, .intui_fr_ret
    ; Compute base address: screen_va + r7*3200 + r6*4
    load.q  r5, 200(r29)               ; screen_va
    move.l  r12, #3200
    mulu    r12, r7, r12
    add     r5, r5, r12
    lsl     r12, r6, #2
    add     r5, r5, r12                ; r5 = top-left pixel of rect
    move.q  r13, r9                    ; r13 = remaining rows
.intui_fr_row:
    beqz    r13, .intui_fr_ret
    move.q  r14, r5                    ; col cursor
    move.q  r15, r8                    ; remaining cols
.intui_fr_col:
    beqz    r15, .intui_fr_row_done
    store.l r17, (r14)
    add     r14, r14, #4
    sub     r15, r15, #1
    bra     .intui_fr_col
.intui_fr_row_done:
    add     r5, r5, #3200              ; next row
    sub     r13, r13, #1
    bra     .intui_fr_row
.intui_fr_ret:
    rts

; ----------------------------------------------------------------
; .intui_draw_char — render one Topaz 8x16 glyph into the screen surface
; ----------------------------------------------------------------
; Inputs:
;   r10 = x  (screen column, must be a valid position on the 800-wide screen)
;   r11 = y  (screen row)
;   r3  = character byte (ASCII; lookup is `font[ch * 16 + row]`)
;   r29 = data base (font lives at offset 768)
; Output: nothing
; Clobbers: r4..r9, r14..r19
; Preserves: r10, r11, r29
; Glyph pixels are written in COL_TEXT (0xFF000000 — black on grey).
; ----------------------------------------------------------------
.intui_draw_char:
    load.q  r4, 200(r29)               ; r4 = screen_va
    move.l  r14, #3200                 ; screen stride
    mulu    r14, r11, r14
    add     r4, r4, r14
    lsl     r14, r10, #2
    add     r4, r4, r14                ; r4 = top-left pixel of glyph cell
    add     r5, r29, #768              ; r5 = font_base
    lsl     r6, r3, #4                 ; ch * 16
    add     r5, r5, r6                 ; r5 = &font[ch][0]
    move.l  r6, #0                     ; row index 0..15
.intui_dc_row:
    move.l  r14, #16
    bge     r6, r14, .intui_dc_done
    load.b  r7, (r5)
    add     r5, r5, #1
    move.q  r8, r4                     ; pixel cursor for this row
    move.l  r9, #0                     ; col index 0..7
.intui_dc_col:
    move.l  r14, #8
    bge     r9, r14, .intui_dc_col_done
    move.l  r14, #7
    sub     r14, r14, r9
    lsr     r15, r7, r14
    and     r15, r15, #1
    beqz    r15, .intui_dc_skip
    move.l  r16, #0xFF000000           ; COL_TEXT (black)
    store.l r16, (r8)
.intui_dc_skip:
    add     r8, r8, #4
    add     r9, r9, #1
    bra     .intui_dc_col
.intui_dc_col_done:
    add     r4, r4, #3200              ; next row
    add     r6, r6, #1
    bra     .intui_dc_row
.intui_dc_done:
    rts

; ----------------------------------------------------------------
; .intui_draw_string — render a null-terminated string at (x, y)
; ----------------------------------------------------------------
; Inputs:
;   r10 = x (initial screen column)
;   r11 = y (screen row)
;   r12 = string pointer (in caller's data section)
;   r29 = data base
; Clobbers: r3..r9, r14..r19
; ----------------------------------------------------------------
.intui_draw_string:
    sub     sp, sp, #24
    store.q r10, (sp)                  ; save x
    store.q r11, 8(sp)                 ; save y
    store.q r12, 16(sp)                ; save str ptr
.intui_ds_loop:
    load.q  r12, 16(sp)
    load.b  r3, (r12)
    beqz    r3, .intui_ds_done
    load.q  r10, (sp)
    load.q  r11, 8(sp)
    jsr     .intui_draw_char
    load.q  r10, (sp)
    add     r10, r10, #8               ; advance x by glyph width
    store.q r10, (sp)
    load.q  r12, 16(sp)
    add     r12, r12, #1
    store.q r12, 16(sp)
    bra     .intui_ds_loop
.intui_ds_done:
    add     sp, sp, #24
    rts
prog_intui_code_end:

prog_intui_data:
    ; offsets 0..127: convention/scratch, unused. Strings live at 320+ so the
    ; field block at 128..319 keeps the same shape as the other services.
    ds.b    128                         ; pad 0..128
    ds.b    8                           ; 128: task_id
    ds.b    8                           ; 136: intuition_port
    ds.b    8                           ; 144: graphics_port
    ds.b    8                           ; 152: input_port
    ds.b    8                           ; 160: reply_port
    ds.b    8                           ; 168: my_input_port
    ds.b    8                           ; 176: display_open (1) + pad
    ds.b    8                           ; 184: display_handle
    ds.b    8                           ; 192: surface_handle
    ds.b    8                           ; 200: screen_va
    ds.b    8                           ; 208: screen_share (4) + pad
    ds.b    8                           ; 216: win_in_use (1) + pad
    ds.b    4                           ; 224: win_x
    ds.b    4                           ; 228: win_y
    ds.b    4                           ; 232: win_w
    ds.b    4                           ; 236: win_h
    ds.b    8                           ; 240: win_share (4) + pad
    ds.b    8                           ; 248: win_mapped_va
    ds.b    8                           ; 256: idcmp_port
    ds.b    8                           ; 264: graphics_library_token
    ds.b    8                           ; 272: msg_type
    ds.b    8                           ; 280: msg_data0
    ds.b    8                           ; 288: msg_data1
    ds.b    8                           ; 296: msg_reply
    ds.b    8                           ; 304: msg_share
    ds.b    8                           ; 312: graphics_open_sigbit
    ; Strings (32-byte slots, PORT_NAME_LEN-aligned)
    ; offset 320: "intuition.library" + pad to 32 — own port name
    dc.b    "intuition.library", 0
    ds.b    14
    ; offset 352: "graphics.library" + pad to 32 — for FindPort
    dc.b    "graphics.library", 0
    ds.b    15
    ; offset 384: "input.device" + pad to 32 — for FindPort
    dc.b    "input.device", 0
    ds.b    19
    ; offset 416: banner string "intuition.library M12 [Task " + null + pad
    dc.b    "intuition.library M12 [Task ", 0
    ds.b    15                          ; pad to offset 460
    ; offset 460: window title text rendered by .intui_draw_string in the
    ; title bar block of the M12 decoration
    dc.b    "About IntuitionOS", 0
    ds.b    290                         ; pad to offset 768
    ; offset 768: embedded Topaz 8x16 bitmap font (256 glyphs x 16 bytes
    ; = 4096 bytes) — used by .intui_draw_char to render title text
    incbin  "topaz.raw"
    align   8
prog_intuition_library_iosm:
prog_intui_iosm:
    dc.l    IOSM_MAGIC
    dc.l    IOSM_SCHEMA_VERSION
    dc.b    IOSM_KIND_LIBRARY
    dc.b    0
    dc.w    12
    dc.w    0
    dc.w    0
    dc.b    "intuition.library", 0
    ds.b    IOSM_NAME_SIZE - 18
    dc.l    MODF_COMPAT_PORT
    dc.l    0
    dc.b    "2026-04-22", 0
    ds.b    IOSM_BUILD_DATE_SIZE - 11
    dc.b    0x43, 0x6F, 0x70, 0x79, 0x72, 0x69, 0x67, 0x68, 0x74, 0x20, 0xA9, 0x20, 0x32, 0x30, 0x32, 0x36, 0x20, 0x5A, 0x61, 0x79, 0x6E, 0x20, 0x4F, 0x74, 0x6C, 0x65, 0x79, 0
    ds.b    IOSM_COPYRIGHT_SIZE - 28
    ds.b    8
prog_intui_data_end:
    align   8
prog_intui_end:
