include "template.s"

prog_graphics_library:
    .libmanifest name="graphics.library", version=11, revision=0, type=1, flags=2, msg_abi=0
    dc.l    0, 0
    dc.l    prog_gfxlib_code_end - prog_gfxlib_code
    dc.l    prog_gfxlib_data_end - prog_gfxlib_data
    dc.l    0
    ds.b    12
prog_gfxlib_code:

    ; ===== Preamble =====
    m16_lib_preamble 128

    ; ===== M12.5: request CHIP and VRAM grants from hardware.resource =====
    ; Both SYS_MAP_IO calls below are now gated by the kernel grant table.
    ; Spin on FindPort until hardware.resource is up, then send two
    ; HWRES_MSG_REQUEST messages, then call SYS_MAP_IO twice.
.gfx_find_hwres:
    add     r1, r29, #256              ; r1 = "hardware.resource" string (data offset 256)
    syscall #SYS_FIND_PORT
    bnez    r2, .gfx_hwres_retry
    load.q  r29, (sp)
    bra     .gfx_have_hwres
.gfx_hwres_retry:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .gfx_find_hwres
.gfx_have_hwres:
    store.q r1, 288(r29)               ; data[288] = hwres_port

    ; Create anonymous reply port (reused for both requests)
    move.q  r1, r0
    move.q  r2, r0
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    bnez    r2, .gfx_halt
    store.q r1, 296(r29)               ; data[296] = reply_port

    ; --- Request CHIP grant ---
    load.q  r1, 288(r29)
    move.l  r2, #HWRES_MSG_REQUEST
    move.l  r3, #HWRES_TAG_CHIP
    load.q  r4, 128(r29)
    load.q  r5, 296(r29)
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    bnez    r2, .gfx_halt
    load.q  r1, 296(r29)
    syscall #SYS_WAIT_PORT             ; R1=type, R3=err (returns msg data)
    load.q  r29, (sp)
    bnez    r3, .gfx_halt
    move.l  r28, #HWRES_MSG_GRANTED
    bne     r1, r28, .gfx_halt

    ; --- Request VRAM grant ---
    load.q  r1, 288(r29)
    move.l  r2, #HWRES_MSG_REQUEST
    move.l  r3, #HWRES_TAG_VRAM
    load.q  r4, 128(r29)
    load.q  r5, 296(r29)
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    bnez    r2, .gfx_halt
    load.q  r1, 296(r29)
    syscall #SYS_WAIT_PORT             ; R1=type, R3=err
    load.q  r29, (sp)
    bnez    r3, .gfx_halt
    move.l  r28, #HWRES_MSG_GRANTED
    bne     r1, r28, .gfx_halt

    ; ===== SYS_MAP_IO chip register page =====
    move.l  r1, #TERM_IO_PAGE
    move.l  r2, #1
    syscall #SYS_MAP_IO
    load.q  r29, (sp)
    bnez    r2, .gfx_halt
    store.q r1, 152(r29)               ; data[152] = chip_mmio_va

    ; ===== SYS_MAP_IO VRAM (PPN 0x100, 470 pages = 800x600x4 = 1920000 bytes
    ; → 469 pages, rounded up to 470) =====
    move.l  r1, #0x100
    move.l  r2, #470
    syscall #SYS_MAP_IO
    load.q  r29, (sp)
    bnez    r2, .gfx_halt
    store.q r1, 160(r29)               ; data[160] = vram_va

    ; ===== CreatePort("graphics.library") + AddLibrary =====
    m16_lib_register 16, 11, 0, 144, .gfx_addlib_done, .gfx_addlib_done, .gfx_halt
.gfx_addlib_done:

    ; ===== Print banner via SYS_DEBUG_PUTCHAR =====
    m16_lib_print_banner 48, 128, .gfx_ban_loop, .gfx_ban_id

    ; ===== Main loop: WaitPort + dispatch =====
.gfx_main:
    load.q  r29, (sp)
    load.q  r1, 144(r29)               ; port_id
    syscall #SYS_WAIT_PORT             ; R1=type R2=data0 R3=err R4=data1 R5=reply R6=share
    load.q  r29, (sp)
    bnez    r3, .gfx_main              ; error → loop

    ; Save message fields to scratch (200..239)
    store.q r1, 200(r29)               ; type
    store.q r2, 208(r29)               ; data0
    store.q r4, 216(r29)               ; data1
    store.q r5, 224(r29)               ; reply_port
    store.q r6, 232(r29)               ; share_handle

    ; Dispatch
    move.l  r28, #MSG_GET_IOSM
    beq     r1, r28, .gfx_h_get_iosm
    move.l  r28, #LIB_OP_EXPUNGE
    beq     r1, r28, .gfx_h_expunge
    move.l  r28, #GFX_ENUMERATE_ADAPTERS
    beq     r1, r28, .gfx_h_enum_adapt
    move.l  r28, #GFX_GET_ADAPTER_INFO
    beq     r1, r28, .gfx_h_get_adapt
    move.l  r28, #GFX_ENUMERATE_MODES
    beq     r1, r28, .gfx_h_enum_modes
    move.l  r28, #GFX_GET_MODE_INFO
    beq     r1, r28, .gfx_h_get_mode
    move.l  r28, #GFX_OPEN_DISPLAY
    beq     r1, r28, .gfx_h_open_disp
    move.l  r28, #GFX_CLOSE_DISPLAY
    beq     r1, r28, .gfx_h_close_disp
    move.l  r28, #GFX_REGISTER_SURFACE
    beq     r1, r28, .gfx_h_reg_surf
    move.l  r28, #GFX_UNREGISTER_SURFACE
    beq     r1, r28, .gfx_h_unreg_surf
    move.l  r28, #GFX_PRESENT
    beq     r1, r28, .gfx_h_present
    bra     .gfx_reply_bad_handle

    ; ----- ENUMERATE_ADAPTERS: data0=1 -----
.gfx_h_enum_adapt:
    move.l  r2, #GFX_ERR_OK
    move.l  r3, #1                     ; 1 adapter
    move.q  r4, r0
    bra     .gfx_reply

    ; ----- GET_ADAPTER_INFO: data0=(1<<16), data1=CAP_RGBA32 -----
.gfx_h_get_adapt:
    ; Validate adapter_id == 0
    load.q  r14, 208(r29)
    bnez    r14, .gfx_reply_bad_handle
    move.l  r2, #GFX_ERR_OK
    move.l  r3, #0x10000               ; version 1.0 (major<<16)
    move.l  r4, #GFX_CAP_RGBA32
    bra     .gfx_reply

    ; ----- ENUMERATE_MODES: data0=1 -----
.gfx_h_enum_modes:
    load.q  r14, 208(r29)
    bnez    r14, .gfx_reply_bad_handle
    move.l  r2, #GFX_ERR_OK
    move.l  r3, #1                     ; 1 mode
    move.q  r4, r0
    bra     .gfx_reply

    ; ----- GET_MODE_INFO: data0=(800<<16)|600, data1=(1<<32)|3200 -----
    ; M12: bumped from 640x480 to 800x600. Stride = 800*4 = 3200 bytes.
.gfx_h_get_mode:
    load.q  r14, 208(r29)              ; adapter_id
    bnez    r14, .gfx_reply_bad_handle
    load.q  r14, 216(r29)              ; mode_id
    bnez    r14, .gfx_reply_bad_handle
    ; data0 = (800<<16) | 600
    move.l  r3, #800
    lsl     r3, r3, #16
    or      r3, r3, #600
    ; data1 = (FMT_RGBA32 << 32) | 3200
    move.l  r4, #GFX_FMT_RGBA32
    lsl     r4, r4, #32
    or      r4, r4, #3200
    move.l  r2, #GFX_ERR_OK
    bra     .gfx_reply

    ; ----- OPEN_DISPLAY(0, 0): set chip mode, enable chip, mark open -----
.gfx_h_open_disp:
    load.q  r14, 208(r29)              ; adapter_id
    bnez    r14, .gfx_reply_bad_mode
    load.q  r14, 216(r29)              ; mode_id
    bnez    r14, .gfx_reply_bad_mode
    load.b  r14, 168(r29)              ; display_open
    bnez    r14, .gfx_reply_busy
    ; M12: write VIDEO_MODE = 1 (MODE_800x600 = chip's DEFAULT_VIDEO_MODE).
    ; This is a no-op when the chip is already in 800x600 (the chip skips
    ; reallocating its frontBuffer when len matches), so VideoTerminal's
    ; cached pixel dimensions stay valid. The protocol still allows other
    ; modes — graphics.library just defaults to the chip's native mode.
    load.q  r15, 152(r29)              ; chip_mmio_va
    add     r16, r15, #4               ; VIDEO_MODE
    move.l  r17, #1                    ; MODE_800x600
    store.l r17, (r16)
    ; Set VIDEO_CTRL = 1 to ENABLE the chip. Writing 0 to VIDEO_CTRL
    ; DISABLES the chip per video_chip.go:2653 (the constant name
    ; CTRL_DISABLE_FLAG=0 is misleading — non-zero enables, zero disables).
    move.l  r17, #1
    store.l r17, (r15)
    ; Mark display open
    move.b  r14, #1
    store.b r14, 168(r29)
    move.l  r2, #GFX_ERR_OK
    move.l  r3, #1                     ; display_handle = 1
    move.q  r4, r0
    bra     .gfx_reply

    ; ----- CLOSE_DISPLAY(handle): clear flag, drop surface, disable chip -----
.gfx_h_close_disp:
    load.q  r14, 208(r29)              ; display_handle
    move.l  r28, #1
    bne     r14, r28, .gfx_reply_bad_handle
    store.b r0, 168(r29)               ; display_open = 0
    store.b r0, 176(r29)               ; surface_in_use = 0 (drop on close)
    ; Reset chip mode to 800x600 default and disable scanout. The next
    ; OpenDisplay will re-enable with VIDEO_CTRL=1. This makes CloseDisplay
    ; observable on the chip and mitigates the M11 wart (crashed
    ; graphics.library leaving graphics mode active) for the clean-exit path.
    load.q  r15, 152(r29)              ; chip_mmio_va
    add     r16, r15, #4               ; VIDEO_MODE
    move.l  r17, #1                    ; MODE_800x600 (DEFAULT_VIDEO_MODE)
    store.l r17, (r16)
    ; VIDEO_CTRL = 0 disables the chip (CTRL_DISABLE_FLAG = 0).
    store.l r0, (r15)
    move.l  r2, #GFX_ERR_OK
    move.q  r3, r0
    move.q  r4, r0
    bra     .gfx_reply

    ; ----- REGISTER_SURFACE: MapShared, store, return handle=1 -----
.gfx_h_reg_surf:
    load.q  r14, 208(r29)              ; display_handle
    move.l  r28, #1
    bne     r14, r28, .gfx_reply_bad_handle
    load.b  r14, 168(r29)              ; display_open
    beqz    r14, .gfx_reply_bad_handle
    load.b  r14, 176(r29)              ; surface_in_use
    bnez    r14, .gfx_reply_busy       ; already have one (single surface for M11)
    ; MapShared(share_handle)
    load.l  r1, 232(r29)               ; share_handle
    move.l  r2, #MAPF_READ
    syscall #SYS_MAP_SHARED            ; R1=mapped_va R2=err
    load.q  r29, (sp)
    bnez    r2, .gfx_reply_bad_format
    store.q r1, 184(r29)               ; surface_mapped_va
    load.l  r14, 232(r29)              ; share_handle
    store.l r14, 180(r29)              ; surface_share_handle
    store.l r0, 192(r29)               ; present_seq = 0
    ; Unpack dimensions from saved data1: (w<<48)|(h<<32)|(fmt<<16)|stride
    load.q  r14, 216(r29)              ; saved data1
    move.q  r15, r14
    lsr     r15, r15, #48
    and     r15, r15, #0xFFFF          ; width
    store.l r15, 240(r29)
    move.q  r15, r14
    lsr     r15, r15, #32
    and     r15, r15, #0xFFFF          ; height
    store.l r15, 244(r29)
    move.q  r15, r14
    lsr     r15, r15, #16
    and     r15, r15, #0xFFFF          ; format
    store.l r15, 248(r29)
    move.q  r15, r14
    and     r15, r15, #0xFFFF          ; stride (bytes)
    store.l r15, 252(r29)
    move.b  r14, #1
    store.b r14, 176(r29)              ; surface_in_use
    move.l  r2, #GFX_ERR_OK
    move.l  r3, #1                     ; surface_handle = 1
    move.q  r4, r0
    bra     .gfx_reply

    ; ----- UNREGISTER_SURFACE -----
    ; M12 fix: also FREE_MEM the mapped client surface, otherwise the
    ; shared object's refcount stays > 0 forever and the backing pages
    ; never get released — even after the client side calls FreeMem.
    ; The original M11 path just cleared in_use and leaked the mapping.
.gfx_h_unreg_surf:
    load.q  r14, 208(r29)              ; surface_handle
    move.l  r28, #1
    bne     r14, r28, .gfx_reply_bad_handle
    load.b  r14, 176(r29)              ; surface_in_use
    beqz    r14, .gfx_unreg_skip_free  ; nothing mapped — defensive
    ; FreeMem(surface_mapped_va, stride * height)
    load.l  r14, 252(r29)              ; stride bytes
    load.l  r15, 244(r29)              ; height
    mulu    r14, r14, r15
    load.q  r1, 184(r29)               ; surface_mapped_va
    move.q  r2, r14
    syscall #SYS_FREE_MEM
    load.q  r29, (sp)
    ; Best-effort: ignore FreeMem errors. Clear cached fields so a future
    ; REGISTER_SURFACE starts from a clean slate.
    store.q r0, 184(r29)               ; surface_mapped_va = 0
    store.l r0, 180(r29)               ; surface_share_handle = 0
.gfx_unreg_skip_free:
    store.b r0, 176(r29)               ; clear in_use
    move.l  r2, #GFX_ERR_OK
    move.q  r3, r0
    move.q  r4, r0
    bra     .gfx_reply

    ; ----- PRESENT: memcpy surface → VRAM, return present_seq -----
.gfx_h_present:
    load.q  r14, 208(r29)              ; surface_handle
    move.l  r28, #1
    bne     r14, r28, .gfx_reply_bad_handle
    load.b  r14, 176(r29)              ; surface_in_use
    beqz    r14, .gfx_reply_bad_handle
    ; Compute byte count = stride * height (per stored surface dims)
    load.l  r17, 252(r29)              ; stride (bytes)
    load.l  r18, 244(r29)              ; height
    mulu    r16, r17, r18              ; r16 = byte count
    load.q  r14, 184(r29)              ; src = surface_mapped_va
    load.q  r15, 160(r29)              ; dst = vram_va
.gfx_present_copy:
    beqz    r16, .gfx_present_done
    load.q  r17, (r14)
    store.q r17, (r15)
    add     r14, r14, #8
    add     r15, r15, #8
    sub     r16, r16, #8
    bra     .gfx_present_copy
.gfx_present_done:
    ; Increment present_seq, reply with new value
    load.l  r14, 192(r29)
    add     r14, r14, #1
    store.l r14, 192(r29)
    move.l  r2, #GFX_ERR_OK
    move.q  r3, r14                    ; reply data0 = present_seq
    move.q  r4, r0
    bra     .gfx_reply

    ; ----- Common reply paths -----
.gfx_reply_bad_handle:
    move.l  r2, #GFX_ERR_BAD_HANDLE
    move.q  r3, r0
    move.q  r4, r0
    bra     .gfx_reply
.gfx_reply_bad_mode:
    move.l  r2, #GFX_ERR_BAD_MODE
    move.q  r3, r0
    move.q  r4, r0
    bra     .gfx_reply
.gfx_reply_bad_format:
    move.l  r2, #GFX_ERR_BAD_FORMAT
    move.q  r3, r0
    move.q  r4, r0
    bra     .gfx_reply
.gfx_reply_busy:
    move.l  r2, #GFX_ERR_BUSY
    move.q  r3, r0
    move.q  r4, r0
    bra     .gfx_reply

.gfx_h_expunge:
    load.b  r14, 168(r29)              ; display_open
    bnez    r14, .gfx_expunge_refuse
    load.b  r14, 176(r29)              ; surface_in_use
    bnez    r14, .gfx_expunge_refuse
    m16_lib_accept_expunge 208, 216, .gfx_main
.gfx_expunge_refuse:
    m16_lib_refuse_expunge 208, 216, .gfx_main

.gfx_h_get_iosm:
    load.q  r14, 232(r29)
    beqz    r14, .gfx_get_iosm_badarg
    move.q  r25, r14                   ; share_handle
    load.q  r26, 224(r29)              ; reply_port
    move.q  r1, r25
    move.l  r2, #MAPF_WRITE
    syscall #SYS_MAP_SHARED
    load.q  r29, (sp)
    bnez    r2, .gfx_get_iosm_maperr
    move.q  r23, r1
    move.q  r24, r1
    move.l  r28, #1
    bne     r3, r28, .gfx_get_iosm_badarg_free
    add     r14, r29, #(prog_gfxlib_iosm - prog_gfxlib_data)
    move.l  r15, #(IOSM_SIZE / 8)
.gfx_get_iosm_copy:
    load.q  r16, (r14)
    store.q r16, (r24)
    add     r14, r14, #8
    add     r24, r24, #8
    sub     r15, r15, #1
    bnez    r15, .gfx_get_iosm_copy
    move.q  r1, r23
    move.l  r2, #IOSM_SIZE
    syscall #SYS_FREE_MEM
    load.q  r29, (sp)
    move.q  r1, r26
    move.q  r2, r0
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    bra     .gfx_main
.gfx_get_iosm_badarg_free:
    move.q  r1, r23
    move.q  r2, r3
    lsl     r2, r2, #12
    syscall #SYS_FREE_MEM
    load.q  r29, (sp)
.gfx_get_iosm_badarg:
    load.q  r1, 224(r29)
    move.l  r2, #ERR_BADARG
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    bra     .gfx_main
.gfx_get_iosm_maperr:
    load.q  r1, 224(r29)
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    bra     .gfx_main

.gfx_reply:
    ; R2 = err code (used as msg_type per project convention)
    ; R3 = data0, R4 = data1
    load.q  r1, 224(r29)               ; reply_port
    move.q  r5, r0                     ; share_handle = 0
    syscall #SYS_REPLY_MSG
    bra     .gfx_main

.gfx_halt:
    syscall #SYS_YIELD
    bra     .gfx_halt
prog_gfxlib_code_end:

prog_gfxlib_data:
    ; offset 0:  "console.handler\0" (16) — unused, kept for convention
    dc.b    "console.handler", 0
    ; offset 16: port name "graphics.library" + null (M12: PORT_NAME_LEN bumped
    ; from 16 to 32, so the kernel reads up to 32 bytes — the name MUST be
    ; null-terminated within the first 32 bytes from this address).
    dc.b    "graphics.library", 0
    ds.b    15                          ; pad to offset 48
    ; offset 48: banner "graphics.library M11 [Task " + null + pad to 80
    dc.b    "graphics.library M11 [Task ", 0
    ds.b    4                           ; pad to offset 80
    ds.b    48                          ; pad to offset 128
    ds.b    8                           ; 128: task_id
    ds.b    8                           ; 136: (unused)
    ds.b    8                           ; 144: port_id
    ds.b    8                           ; 152: chip_mmio_va
    ds.b    8                           ; 160: vram_va
    ds.b    8                           ; 168: display_open (1) + pad
    ds.b    4                           ; 176: surface_in_use (1) + pad (3)
    ds.b    4                           ; 180: surface_share_handle (4)
    ds.b    8                           ; 184: surface_mapped_va (8)
    ds.b    8                           ; 192: present_seq (4) + pad
    ds.b    8                           ; 200: msg type
    ds.b    8                           ; 208: msg data0
    ds.b    8                           ; 216: msg data1
    ds.b    8                           ; 224: msg reply_port
    ds.b    8                           ; 232: msg share_handle
    ds.b    4                           ; 240: surface_width
    ds.b    4                           ; 244: surface_height
    ds.b    4                           ; 248: surface_format
    ds.b    4                           ; 252: surface_stride
    ; --- M12.5 additions ---
    dc.b    "hardware.resource", 0      ; 256: broker port name
    ds.b    14                          ; pad to offset 288 (256+32)
    ds.b    8                           ; 288: hwres_port
    ds.b    8                           ; 296: reply_port
    align   8
prog_graphics_library_iosm:
prog_gfxlib_iosm:
    dc.l    IOSM_MAGIC
    dc.l    IOSM_SCHEMA_VERSION
    dc.b    IOSM_KIND_LIBRARY
    dc.b    0
    dc.w    11
    dc.w    0
    dc.w    0
    dc.b    "graphics.library", 0
    ds.b    IOSM_NAME_SIZE - 17
    dc.l    MODF_COMPAT_PORT
    dc.l    0
    dc.b    "2026-04-22", 0
    ds.b    IOSM_BUILD_DATE_SIZE - 11
    dc.b    0x43, 0x6F, 0x70, 0x79, 0x72, 0x69, 0x67, 0x68, 0x74, 0x20, 0xA9, 0x20, 0x32, 0x30, 0x32, 0x36, 0x20, 0x5A, 0x61, 0x79, 0x6E, 0x20, 0x4F, 0x74, 0x6C, 0x65, 0x79, 0
    ds.b    IOSM_COPYRIGHT_SIZE - 28
    ds.b    8
prog_gfxlib_data_end:
    align   8
prog_gfxlib_end:
