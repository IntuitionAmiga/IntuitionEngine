prog_gfxdemo:
    dc.l    0, 0
    dc.l    prog_gfxdemo_code_end - prog_gfxdemo_code
    dc.l    prog_gfxdemo_data_end - prog_gfxdemo_data
    dc.l    0
    ds.b    12
prog_gfxdemo_code:

    ; ===== Preamble =====
    sub     sp, sp, #16
.gd_preamble:
    load.q  r30, (sp)
    load.q  r29, 8(sp)
    store.q r29, (sp)
    load.l  r1, TASKSB_TASK_ID(r30)
    store.q r1, 128(r29)               ; data[128] = task_id

    ; ===== Acquire graphics.library through OpenLibraryEx, then resolve
    ;       the compat port transport =====
    move.l  r1, #16
    syscall #SYS_ALLOC_SIGNAL
    load.q  r29, (sp)
    bnez    r2, .gd_halt
    store.q r1, 240(r29)               ; graphics_open_sigbit
.gd_open_gfx_retry:
    load.q  r29, (sp)
    move.l  r14, #0xFFFFFFFF
    store.l r14, 248(r29)              ; waiter status sentinel
    store.l r0, 252(r29)
    store.l r0, 256(r29)
    store.l r0, 260(r29)
    add     r1, r29, #16               ; "graphics.library"
    move.q  r2, r0
    load.q  r3, 240(r29)               ; waiter-owned signal bit
    add     r4, r29, #248              ; 16-byte outcome scratch
    syscall #SYS_OPEN_LIBRARY_EX
    load.q  r29, (sp)
    beqz    r2, .gd_find_gfx
    move.l  r14, #ERR_AGAIN
    bne     r2, r14, .gd_open_gfx_yield
    load.l  r14, 248(r29)              ; waiter status
    move.l  r15, #0xFFFFFFFF
    bne     r14, r15, .gd_open_gfx_done
    load.q  r14, 240(r29)
    move.q  r1, #1
    lsl     r1, r1, r14
    syscall #SYS_WAIT
    load.q  r29, (sp)
    bnez    r2, .gd_open_gfx_yield
    load.l  r14, 248(r29)
    move.l  r15, #0xFFFFFFFF
    beq     r14, r15, .gd_open_gfx_yield
.gd_open_gfx_done:
    store.q r1, 232(r29)               ; graphics_library_token
    bnez    r14, .gd_open_gfx_yield
.gd_find_gfx:
    load.q  r29, (sp)
    add     r1, r29, #16               ; "graphics.library"
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    bnez    r2, .gd_open_gfx_yield
.gd_gfx_ok:
    store.q r1, 136(r29)               ; data[136] = graphics_port
    load.q  r14, 240(r29)
    beqz    r14, .gd_open_gfx_sigfree_done
    move.q  r1, r14
    syscall #SYS_FREE_SIGNAL
    load.q  r29, (sp)
    store.q r0, 240(r29)
.gd_open_gfx_sigfree_done:
    bra     .gd_open_in

.gd_open_gfx_yield:
    syscall #SYS_YIELD
    bra     .gd_open_gfx_retry
    ; ===== OpenLibrary("input.device") with retry =====
.gd_open_in:
    load.q  r29, (sp)
    add     r1, r29, #48               ; "input.device" (M12: shifted from 32 after PORT_NAME_LEN bump)
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    beqz    r2, .gd_in_ok
    syscall #SYS_YIELD
    bra     .gd_open_in
.gd_in_ok:
    store.q r1, 144(r29)               ; data[144] = input_port

    ; ===== AllocMem(1920000, MEMF_PUBLIC|MEMF_CLEAR) — 800x600 RGBA32 =====
    ; M12: bumped from 1228800 (640x480) to 1920000 (800x600) to match
    ; graphics.library's M12 default mode.
    move.l  r1, #1920000
    move.l  r2, #0x10001               ; MEMF_PUBLIC | MEMF_CLEAR
    syscall #SYS_ALLOC_MEM             ; R1=va R2=err R3=share_handle
    load.q  r29, (sp)
    bnez    r2, .gd_halt
    store.q r1, 152(r29)               ; data[152] = surface_va
    store.l r3, 160(r29)               ; data[160] = surface_share_handle

    ; ===== CreatePort(NULL) → reply_port (anonymous) =====
    move.q  r1, r0
    move.l  r2, #0
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    bnez    r2, .gd_halt
    store.q r1, 168(r29)               ; data[168] = reply_port

    ; ===== CreatePort(NULL) → my_input_port (anonymous) =====
    move.q  r1, r0
    move.l  r2, #0
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    bnez    r2, .gd_halt
    store.q r1, 176(r29)               ; data[176] = my_input_port

    ; ===== Send GFX_OPEN_DISPLAY(adapter=0, mode=0) =====
    load.q  r1, 136(r29)               ; graphics_port
    move.l  r2, #GFX_OPEN_DISPLAY
    move.q  r3, r0                     ; data0 = adapter_id 0
    move.q  r4, r0                     ; data1 = mode_id 0
    load.q  r5, 168(r29)               ; reply_port
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    bnez    r2, .gd_halt
    ; WaitPort for reply
    load.q  r1, 168(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r3, .gd_halt
    bnez    r1, .gd_halt               ; r1 = err code (must be GFX_ERR_OK = 0)
    store.q r2, 184(r29)               ; data[184] = display_handle

    ; ===== Send GFX_REGISTER_SURFACE =====
    ; data1 = (800<<48) | (600<<32) | (1<<16) | 3200 — M12: 800x600 stride 3200
    load.q  r1, 136(r29)
    move.l  r2, #GFX_REGISTER_SURFACE
    load.q  r3, 184(r29)               ; data0 = display_handle
    move.q  r4, #800
    lsl     r4, r4, #48
    move.q  r14, #600
    lsl     r14, r14, #32
    or      r4, r4, r14
    move.q  r14, #1                    ; format
    lsl     r14, r14, #16
    or      r4, r4, r14
    or      r4, r4, #3200              ; stride bytes (800 * 4 = 3200)
    load.q  r5, 168(r29)               ; reply_port
    load.l  r6, 160(r29)               ; share_handle
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    bnez    r2, .gd_halt
    load.q  r1, 168(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r3, .gd_halt
    bnez    r1, .gd_halt
    store.q r2, 192(r29)               ; data[192] = surface_handle

    ; ===== Send INPUT_OPEN(my_input_port) =====
    load.q  r1, 144(r29)               ; input_port
    move.l  r2, #INPUT_OPEN
    load.q  r3, 176(r29)               ; data0 = my_input_port
    move.q  r4, r0
    load.q  r5, 168(r29)               ; reply_port
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    bnez    r2, .gd_halt
    load.q  r1, 168(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r3, .gd_halt
    bnez    r1, .gd_halt

    ; ===== Initialize bouncing rect + mouse pointer state =====
    move.l  r14, #100
    store.l r14, 208(r29)              ; rect_x = 100
    move.l  r14, #80
    store.l r14, 212(r29)              ; rect_y = 80
    move.l  r14, #4
    store.l r14, 216(r29)              ; rect_vx = +4
    move.l  r14, #3
    store.l r14, 220(r29)              ; rect_vy = +3
    move.l  r14, #320
    store.l r14, 224(r29)              ; mouse_x = 320 (center)
    move.l  r14, #240
    store.l r14, 228(r29)              ; mouse_y = 240

    ; ===== Animation loop: backdrop + rect + pointer + present + drain =====
.gd_frame:
    load.q  r29, (sp)

    ; --- Fill backdrop (dark navy) ---
    ; M12: bumped to 800x600 = 1920000 bytes. Color is 0xFF602020 in RGBA byte
    ; order (LE bytes 20,20,60,FF → R=0x20 G=0x20 B=0x60 = dark navy).
    load.q  r14, 152(r29)              ; surface_va
    move.l  r15, #1920000
    move.l  r16, #0xFF602020
.gd_fill_bg:
    beqz    r15, .gd_fill_bg_done
    store.l r16, (r14)
    add     r14, r14, #4
    sub     r15, r15, #4
    bra     .gd_fill_bg
.gd_fill_bg_done:

    ; --- Draw 32x32 bouncing rectangle (white) ---
    load.q  r14, 152(r29)              ; surface_va base
    load.l  r15, 208(r29)              ; rect_x
    load.l  r16, 212(r29)              ; rect_y
    move.l  r17, #0xFFFFFFFF           ; white
    ; pixel addr = surface + y*2560 + x*4
    move.l  r18, #3200
    mulu    r19, r16, r18
    lsl     r20, r15, #2
    add     r19, r19, r20
    add     r19, r19, r14              ; r19 = top-left pixel addr
    move.l  r20, #32                   ; rows
.gd_rect_row:
    beqz    r20, .gd_rect_done
    move.q  r22, r19                   ; row start
    move.l  r21, #32                   ; cols
.gd_rect_col:
    beqz    r21, .gd_rect_row_done
    store.l r17, (r22)
    add     r22, r22, #4
    sub     r21, r21, #1
    bra     .gd_rect_col
.gd_rect_row_done:
    add     r19, r19, #3200            ; next row
    sub     r20, r20, #1
    bra     .gd_rect_row
.gd_rect_done:

    ; --- Draw 16x16 mouse pointer (green) ---
    load.q  r14, 152(r29)
    load.l  r15, 224(r29)              ; mouse_x
    load.l  r16, 228(r29)              ; mouse_y
    ; Clamp mouse to surface bounds [0, 784] / [0, 584] (M12: 800x600 - 16)
    move.l  r28, #784
    ble     r15, r28, .gd_mp_x_ok
    move.l  r15, #784
.gd_mp_x_ok:
    bgez    r15, .gd_mp_x_pos
    move.l  r15, #0
.gd_mp_x_pos:
    move.l  r28, #584
    ble     r16, r28, .gd_mp_y_ok
    move.l  r16, #584
.gd_mp_y_ok:
    bgez    r16, .gd_mp_y_pos
    move.l  r16, #0
.gd_mp_y_pos:
    move.l  r17, #0xFF00FF00           ; green
    move.l  r18, #3200
    mulu    r19, r16, r18
    lsl     r20, r15, #2
    add     r19, r19, r20
    add     r19, r19, r14
    move.l  r20, #16
.gd_mp_row:
    beqz    r20, .gd_mp_done
    move.q  r22, r19
    move.l  r21, #16
.gd_mp_col:
    beqz    r21, .gd_mp_row_done
    store.l r17, (r22)
    add     r22, r22, #4
    sub     r21, r21, #1
    bra     .gd_mp_col
.gd_mp_row_done:
    add     r19, r19, #3200
    sub     r20, r20, #1
    bra     .gd_mp_row
.gd_mp_done:

    ; --- Send GFX_PRESENT and wait for reply ---
    load.q  r1, 136(r29)
    move.l  r2, #GFX_PRESENT
    load.q  r3, 192(r29)
    move.q  r4, r0
    load.q  r5, 168(r29)
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    load.q  r1, 168(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)

    ; Mark "frame presented" in data[200] for tests
    move.l  r14, #1
    store.l r14, 200(r29)

    ; --- Update bouncing rect: x += vx, bounce at [0, 768] (M12: 800-32) ---
    load.l  r14, 208(r29)
    load.l  r15, 216(r29)
    add     r14, r14, r15
    move.l  r28, #768
    ble     r14, r28, .gd_x_low_check
    move.l  r14, #768
    sub     r15, r0, r15
    bra     .gd_x_save
.gd_x_low_check:
    bgez    r14, .gd_x_save
    move.l  r14, #0
    sub     r15, r0, r15
.gd_x_save:
    store.l r14, 208(r29)
    store.l r15, 216(r29)

    load.l  r14, 212(r29)
    load.l  r15, 220(r29)
    add     r14, r14, r15
    move.l  r28, #568
    ble     r14, r28, .gd_y_low_check
    move.l  r14, #568
    sub     r15, r0, r15
    bra     .gd_y_save
.gd_y_low_check:
    bgez    r14, .gd_y_save
    move.l  r14, #0
    sub     r15, r0, r15
.gd_y_save:
    store.l r14, 212(r29)
    store.l r15, 220(r29)

    ; --- Drain input events until queue empty ---
.gd_drain:
    load.q  r29, (sp)
    load.q  r1, 176(r29)               ; my_input_port
    syscall #SYS_GET_MSG
    load.q  r29, (sp)
    bnez    r3, .gd_drain_done         ; ERR_AGAIN → no more events

    move.l  r28, #INPUT_EVENT
    bne     r1, r28, .gd_drain         ; ignore non-events

    move.q  r14, r2
    lsr     r14, r14, #24
    and     r14, r14, #0xFF            ; event type

    move.l  r28, #IE_KEY_DOWN
    beq     r14, r28, .gd_handle_key
    move.l  r28, #IE_MOUSE_MOVE
    beq     r14, r28, .gd_handle_move
    bra     .gd_drain                  ; ignore other event types

.gd_handle_key:
    move.q  r14, r2
    lsr     r14, r14, #16
    and     r14, r14, #0xFF            ; scancode
    move.l  r28, #1                    ; Escape
    beq     r14, r28, .gd_exit
    bra     .gd_drain

.gd_handle_move:
    ; mn_Data1 (R4) = (mx16<<48)|(my16<<32)|seq32
    move.q  r14, r4
    lsr     r14, r14, #48
    and     r14, r14, #0xFFFF
    store.l r14, 224(r29)              ; mouse_x
    move.q  r14, r4
    lsr     r14, r14, #32
    and     r14, r14, #0xFFFF
    store.l r14, 228(r29)              ; mouse_y
    bra     .gd_drain

.gd_drain_done:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .gd_frame

.gd_exit:
    ; ===== Cleanup: INPUT_CLOSE → UNREGISTER → CLOSE_DISPLAY → ExitTask =====
    load.q  r1, 144(r29)
    move.l  r2, #INPUT_CLOSE
    move.q  r3, r0
    move.q  r4, r0
    load.q  r5, 168(r29)
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    load.q  r1, 168(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)

    load.q  r1, 136(r29)
    move.l  r2, #GFX_UNREGISTER_SURFACE
    load.q  r3, 192(r29)
    move.q  r4, r0
    load.q  r5, 168(r29)
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    load.q  r1, 168(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)

    load.q  r1, 136(r29)
    move.l  r2, #GFX_CLOSE_DISPLAY
    load.q  r3, 184(r29)
    move.q  r4, r0
    load.q  r5, 168(r29)
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    load.q  r1, 168(r29)
    syscall #SYS_WAIT_PORT

.gd_cleanup_exit:
    load.q  r1, 232(r29)               ; graphics_library_token
    beqz    r1, .gd_exit_task
    syscall #SYS_CLOSE_LIBRARY
    load.q  r29, (sp)
    store.q r0, 232(r29)
.gd_exit_task:
    move.q  r1, r0
    syscall #SYS_EXIT_TASK

.gd_halt:
    bra     .gd_cleanup_exit
prog_gfxdemo_code_end:

prog_gfxdemo_data:
    dc.b    "console.handler", 0
    ; offset 16: "graphics.library" + null + pad to 32 (M12: needs null
    ; terminator within the first PORT_NAME_LEN=32 bytes for FindPort)
    dc.b    "graphics.library", 0
    ds.b    15                          ; pad to offset 48
    ; offset 48: "input.device" + null + pad to 32
    dc.b    "input.device", 0
    ds.b    19                          ; pad to offset 80
    ; offset 80: "GfxDemo M11"
    dc.b    "GfxDemo M11", 0            ; offset 80
    ds.b    36                          ; pad to 128 (was 64)
    ds.b    64                          ; 64-127: padding
    ds.b    8                           ; 128: task_id
    ds.b    8                           ; 136: graphics_port
    ds.b    8                           ; 144: input_port
    ds.b    8                           ; 152: surface_va
    ds.b    8                           ; 160: surface_share_handle (4) + pad
    ds.b    8                           ; 168: reply_port
    ds.b    8                           ; 176: my_input_port
    ds.b    8                           ; 184: display_handle
    ds.b    8                           ; 192: surface_handle
    ds.b    8                           ; 200: presented_flag (4) + pad
    ds.b    4                           ; 208: rect_x
    ds.b    4                           ; 212: rect_y
    ds.b    4                           ; 216: rect_vx
    ds.b    4                           ; 220: rect_vy
    ds.b    4                           ; 224: mouse_x
    ds.b    4                           ; 228: mouse_y
    ds.b    8                           ; 232: graphics_library_token
    ds.b    8                           ; 240: graphics_open_sigbit
    ds.b    16                          ; 248: graphics_open outcome scratch
    align   8
prog_gfxdemo_iosm:
    dc.l    IOSM_MAGIC
    dc.l    IOSM_SCHEMA_VERSION
    dc.b    IOSM_KIND_COMMAND
    dc.b    0
    dc.w    1
    dc.w    0
    dc.w    0
    dc.b    "GfxDemo", 0
    ds.b    IOSM_NAME_SIZE - 9
    dc.l    0
    dc.l    0
    dc.b    "2026-04-22", 0
    ds.b    IOSM_BUILD_DATE_SIZE - 11
    dc.b    0x43, 0x6F, 0x70, 0x79, 0x72, 0x69, 0x67, 0x68, 0x74, 0x20, 0xA9, 0x20, 0x32, 0x30, 0x32, 0x36, 0x20, 0x5A, 0x61, 0x79, 0x6E, 0x20, 0x4F, 0x74, 0x6C, 0x65, 0x79, 0
    ds.b    IOSM_COPYRIGHT_SIZE - 28
    ds.b    8
prog_gfxdemo_data_end:
    align   8
prog_gfxdemo_end:
