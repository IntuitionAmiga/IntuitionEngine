prog_hwres:
    dc.l    0, 0
    dc.l    prog_hwres_code_end - prog_hwres_code
    dc.l    prog_hwres_data_end - prog_hwres_data
    dc.l    0
    ds.b    12
prog_hwres_code:

    ; ===== Preamble: compute data page base (preempt-safe) =====
    sub     sp, sp, #16
.hwres_preamble:
    load.q  r30, (sp)
    load.q  r29, 8(sp)
    store.q r29, (sp)
    load.l  r1, TASKSB_TASK_ID(r30)
    store.q r1, 128(r29)               ; data[128] = task_id

    ; ===== CreatePort("hardware.resource", PF_PUBLIC) =====
    move.q  r1, r29                    ; r1 = &data[0] = port name
    move.l  r2, #PF_PUBLIC
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    bnez    r2, .hwres_halt
    store.q r1, 136(r29)               ; data[136] = hwres_port
    move.q  r4, r1
    move.q  r1, r29
    move.l  r2, #1
    move.l  r3, #0
    syscall #SYS_ADD_RESOURCE
    load.q  r29, (sp)
    bnez    r2, .hwres_halt

    ; ===== SYS_HWRES_OP / HWRES_BECOME =====
    move.l  r6, #HWRES_BECOME
    syscall #SYS_HWRES_OP              ; R2 = err
    load.q  r29, (sp)
    bnez    r2, .hwres_halt            ; can't claim broker → give up

    bra     .hwres_main

    ; ===== Legacy boot banner disabled =====
    add     r20, r29, #32              ; r20 = &data[32] = banner
.hwres_ban_loop:
    load.b  r1, (r20)
    beqz    r1, .hwres_ban_id
    store.q r20, 8(sp)
    syscall #SYS_DEBUG_PUTCHAR
    load.q  r29, (sp)
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .hwres_ban_loop
.hwres_ban_id:
    load.q  r29, (sp)
    load.q  r1, 128(r29)
    add     r1, r1, #0x30              ; '0' + task_id
    syscall #SYS_DEBUG_PUTCHAR
    move.l  r1, #0x5D                  ; ']'
    syscall #SYS_DEBUG_PUTCHAR
    move.l  r1, #0x0D
    syscall #SYS_DEBUG_PUTCHAR
    move.l  r1, #0x0A
    syscall #SYS_DEBUG_PUTCHAR

    ; ===== Main loop: WaitPort + dispatch =====
.hwres_main:
    ; SYS_WAIT_PORT atomically blocks AND fetches the message — it returns
    ; (R1=type, R2=data0, R3=err, R4=data1, R5=reply_port, R6=share_handle,
    ; R7=sender_task_id). M12.5 enriches the return with R7 so the broker
    ; can validate the sender against its trust list without trusting
    ; client-supplied identifiers.
.hwres_main:
    load.q  r29, (sp)
    load.q  r1, 136(r29)               ; hwres_port
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r3, .hwres_main            ; spurious wake — go back to wait

    ; r1 = msg type, r2 = tag, r5 = reply_port, r7 = sender public task ID
    move.q  r24, r2                    ; r24 = tag
    move.q  r25, r7                    ; r25 = sender public task ID (TRUSTED)
    move.q  r26, r5                    ; r26 = reply_port

    move.l  r28, #MSG_GET_IOSM
    beq     r1, r28, .hwres_get_iosm
    move.l  r28, #HWRES_MSG_REQUEST
    bne     r1, r28, .hwres_main       ; ignore unknown message types

    ; ===== M12.5 v2 hardening: scrub stale owner slots =====
    ; Walk the broker's per-tag owner lists and clear any slot whose task
    ; has exited (kernel reports state == TASK_FREE). Without this scrub a
    ; recycled task slot would be silently regranted (because the slot ID
    ; matches the dead owner) or a different task would be blocked forever
    ; (because the dead owner still occupies the slot). The kernel grant
    ; chain and KD_HWRES_TASK are cleaned up by kill_task_cleanup, but
    ; the broker's private owner state isn't visible to the kernel.
    ;
    ; CHIP slots: data[144..159] (4 x u32)
    ; VRAM slot:  data[160..163] (1 x u32)
    add     r17, r29, #144              ; r17 = scan cursor
    move.l  r18, #5                     ; total slots to scrub (4 CHIP + 1 VRAM)
.hwres_scrub:
    beqz    r18, .hwres_scrub_done
    load.l  r14, (r17)
    move.l  r15, #HWRES_TASK_FREE
    beq     r14, r15, .hwres_scrub_next
    ; Slot is occupied — query liveness via HWRES_TASK_ALIVE.
    move.q  r1, r14                     ; task_id to query
    move.l  r6, #HWRES_TASK_ALIVE
    push    r17
    push    r18
    push    r24
    push    r25
    push    r26
    push    r29
    syscall #SYS_HWRES_OP               ; R1 = 1 (alive) or 0 (dead), R2 = err
    pop     r29
    pop     r26
    pop     r25
    pop     r24
    pop     r18
    pop     r17
    bnez    r1, .hwres_scrub_next       ; alive — leave as-is
    move.l  r14, #HWRES_TASK_FREE
    store.l r14, (r17)                  ; dead — reclaim slot
.hwres_scrub_next:
    add     r17, r17, #4
    sub     r18, r18, #1
    bra     .hwres_scrub
.hwres_scrub_done:

    ; ===== Resolve tag in policy table =====
    ; M12.5 trust gating: each tag has a per-tag owner list. Sender either
    ; finds itself already in the list (idempotent re-grant) or claims a
    ; FREE slot. List full → DENY. Stale slots have already been reclaimed
    ; by the scrub pass above so dead owners do not block live requesters.
    ;
    ; CHIP owners: data[144..159] (4 x u32; chip MMIO is shared)
    ; VRAM owner:  data[160..163] (1 x u32; framebuffer is monopolized)
    move.l  r28, #HWRES_TAG_CHIP
    beq     r24, r28, .hwres_grant_chip
    move.l  r28, #HWRES_TAG_VRAM
    beq     r24, r28, .hwres_grant_vram
    bra     .hwres_deny

.hwres_grant_chip:
    ; CHIP is a SHARED resource — chip MMIO holds terminal/input/video
    ; registers all in the same physical page, so multiple services
    ; legitimately need access (input.device + graphics.library both
    ; want it). The broker keeps a small fixed-size owner list at
    ; data[144..159] (4 x u32, 0xFFFFFFFF = unclaimed). A request is granted
    ; if the sender is already in the list (idempotent re-grant) OR
    ; if there is a free slot to record them. Otherwise DENY.
    add     r28, r29, #144              ; r28 = &owner_list[0]
    move.l  r27, #4                     ; list size
    move.q  r17, r28                    ; r17 = scan cursor
    move.q  r16, r0                     ; r16 = first free slot ptr (0 = none)
.hwres_chip_scan:
    beqz    r27, .hwres_chip_check_free
    load.l  r15, (r17)
    move.l  r14, #HWRES_TASK_FREE
    beq     r15, r14, .hwres_chip_remember_free
    beq     r15, r25, .hwres_chip_set_range  ; sender already in list
    bra     .hwres_chip_scan_next
.hwres_chip_remember_free:
    bnez    r16, .hwres_chip_scan_next
    move.q  r16, r17
.hwres_chip_scan_next:
    add     r17, r17, #4
    sub     r27, r27, #1
    bra     .hwres_chip_scan
.hwres_chip_check_free:
    beqz    r16, .hwres_deny           ; list full → deny
    store.l r25, (r16)                  ; claim free slot
.hwres_chip_set_range:
    move.l  r17, #0xF0                 ; r17 = ppn_lo
    move.l  r18, #0xF0                 ; r18 = ppn_hi
    move.l  r19, #1                    ; r19 = count
    bra     .hwres_do_create

.hwres_grant_vram:
    ; VRAM is a MONOPOLY resource — only one task may own the framebuffer
    ; (M12 single-display-client rule). One owner slot at data[160].
    load.l  r28, 160(r29)              ; r28 = current VRAM owner
    move.l  r27, #HWRES_TASK_FREE
    beq     r28, r27, .hwres_vram_claim
    bne     r28, r25, .hwres_deny
    bra     .hwres_vram_set_range
.hwres_vram_claim:
    store.l r25, 160(r29)
.hwres_vram_set_range:
    move.l  r17, #0x100                ; r17 = ppn_lo
    move.l  r18, #0x2D5                ; r18 = ppn_hi (0x100 + 470 - 1)
    move.l  r19, #470                  ; r19 = count
    bra     .hwres_do_create

.hwres_do_create:
    ; SYS_HWRES_OP / HWRES_CREATE for the validated sender task with the
    ; resolved range. r25 is the kernel-supplied sender ID — never the
    ; client-supplied data1, which is now ignored entirely.
    move.l  r6, #HWRES_CREATE
    move.q  r1, r25                    ; r1 = target task_id (trusted)
    move.q  r2, r24                    ; r2 = tag
    move.q  r3, r17                    ; r3 = ppn_lo
    move.q  r4, r18                    ; r4 = ppn_hi
    syscall #SYS_HWRES_OP              ; R2 = err
    bnez    r2, .hwres_deny            ; if create failed, deny

    ; Reply HWRES_MSG_GRANTED with payload (ppn_lo<<32) | count.
    ; The client uses these values for SYS_MAP_IO.
    move.q  r3, r17
    lsl     r3, r3, #32
    or      r3, r3, r19                ; r3 = (ppn_lo<<32) | count
    move.q  r1, r26                    ; reply_port
    move.l  r2, #HWRES_MSG_GRANTED
    move.q  r4, r0                     ; data1 unused
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    syscall #SYS_REPLY_MSG
    bra     .hwres_main

.hwres_deny:
    move.q  r1, r26                    ; reply_port
    move.l  r2, #HWRES_MSG_DENIED
    move.q  r3, r0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    syscall #SYS_REPLY_MSG
    bra     .hwres_main

.hwres_get_iosm:
    beqz    r6, .hwres_get_iosm_badarg
    move.q  r1, r6
    move.l  r2, #MAPF_WRITE
    syscall #SYS_MAP_SHARED
    load.q  r29, (sp)
    bnez    r2, .hwres_get_iosm_maperr
    move.q  r23, r1
    move.q  r24, r1
    move.l  r28, #1
    bne     r3, r28, .hwres_get_iosm_badarg_free
    add     r14, r29, #(prog_hwres_iosm - prog_hwres_data)
    move.l  r15, #(IOSM_SIZE / 8)
.hwres_get_iosm_copy:
    load.q  r16, (r14)
    store.q r16, (r24)
    add     r14, r14, #8
    add     r24, r24, #8
    sub     r15, r15, #1
    bnez    r15, .hwres_get_iosm_copy
    move.q  r1, r23
    move.l  r2, #IOSM_SIZE
    syscall #SYS_FREE_MEM
    load.q  r29, (sp)
    move.q  r1, r26
    move.q  r2, r0
    move.q  r3, r0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    syscall #SYS_REPLY_MSG
    bra     .hwres_main
.hwres_get_iosm_badarg_free:
    move.q  r1, r23
    move.q  r2, r3
    lsl     r2, r2, #12
    syscall #SYS_FREE_MEM
    load.q  r29, (sp)
.hwres_get_iosm_badarg:
    move.q  r1, r26
    move.l  r2, #ERR_BADARG
    move.q  r3, r0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    syscall #SYS_REPLY_MSG
    bra     .hwres_main
.hwres_get_iosm_maperr:
    move.q  r1, r26
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    syscall #SYS_REPLY_MSG
    bra     .hwres_main

.hwres_halt:
    syscall #SYS_YIELD
    bra     .hwres_halt
prog_hwres_code_end:

prog_hwres_data:
    dc.b    "hardware.resource", 0     ; 0..17 (port name, padded below)
    ds.b    14                          ; 18..31 (pad to PORT_NAME_LEN=32)
    dc.b    "hardware.resource M12.5 [Task ", 0   ; 32..63 (banner)
    ds.b    1                           ; pad to offset 64
    ds.b    32                          ; 64..95 (pad)
    ds.b    32                          ; 96..127 (pad to data[128])
    ds.b    8                           ; 128: task_id
    ds.b    8                           ; 136: hwres_port
    ; --- M12.5 trust gating: per-tag owner slots ---
    ; 144..159: CHIP owner list (4 x u32, 0xFFFFFFFF = unclaimed). CHIP is shared
    ;           because chip MMIO holds terminal/input/video registers all
    ;           in one physical page; multiple services need it.
    ; 160..163: VRAM owner (1 x u32, 0xFFFFFFFF = unclaimed). VRAM is a monopoly —
    ;           only one task may own the framebuffer per the M12
    ;           single-display-client rule.
    ; All slots default to 0xFFFFFFFF because 0 is a valid task ID (console.handler)
    ; and the broker must distinguish "unclaimed" from "owned by task 0".
    dc.l    0xFFFFFFFF, 0xFFFFFFFF      ; 144..151: CHIP owner list[0..1]
    dc.l    0xFFFFFFFF, 0xFFFFFFFF      ; 152..159: CHIP owner list[2..3]
    dc.l    0xFFFFFFFF                  ; 160..163: VRAM owner
    ds.b    4                           ; 164..167: pad
    align   8
prog_hwres_iosm:
    dc.l    IOSM_MAGIC
    dc.l    IOSM_SCHEMA_VERSION
    dc.b    IOSM_KIND_RESOURCE
    dc.b    0
    dc.w    1
    dc.w    0
    dc.w    0
    dc.b    "hardware.resource", 0
    ds.b    IOSM_NAME_SIZE - 18
    dc.l    MODF_COMPAT_PORT | MODF_ASLR_CAPABLE
    dc.l    0
    dc.b    "2026-04-25", 0
    ds.b    IOSM_BUILD_DATE_SIZE - 11
    dc.b    0x43, 0x6F, 0x70, 0x79, 0x72, 0x69, 0x67, 0x68, 0x74, 0x20, 0xA9, 0x20, 0x32, 0x30, 0x32, 0x36, 0x20, 0x5A, 0x61, 0x79, 0x6E, 0x20, 0x4F, 0x74, 0x6C, 0x65, 0x79, 0
    ds.b    IOSM_COPYRIGHT_SIZE - 28
    ds.b    8
prog_hwres_data_end:
    align   8
prog_hwres_end:
