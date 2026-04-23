; ---------------------------------------------------------------------------
; LIST — directory listing command (M15)
; ---------------------------------------------------------------------------

prog_list_cmd:
    dc.l    0, 0
    dc.l    prog_list_cmd_code_end - prog_list_cmd_code
    dc.l    prog_list_cmd_data_end - prog_list_cmd_data
    dc.l    0
    ds.b    12
prog_list_cmd_code:

    sub     sp, sp, #16
.list_preamble:
    load.q  r29, 8(sp)
    store.q r29, (sp)

.list_open_dos_alloc:
    move.l  r1, #16
    syscall #SYS_ALLOC_SIGNAL
    load.q  r29, (sp)
    bnez    r2, .list_open_dos_alloc_wait
    store.q r1, 104(r29)               ; dos_open_sigbit
.list_open_dos_retry:
    load.q  r29, (sp)
    move.l  r14, #0xFFFFFFFF
    store.l r14, 112(r29)              ; waiter status sentinel
    store.l r0, 116(r29)
    store.l r0, 120(r29)
    store.l r0, 124(r29)
    add     r1, r29, #16
    move.l  r2, #0
    load.q  r3, 104(r29)
    add     r4, r29, #112
    syscall #SYS_OPEN_LIBRARY_EX
    load.q  r29, (sp)
    beqz    r2, .list_find_dos
    move.l  r14, #ERR_AGAIN
    bne     r2, r14, .list_open_dos_wait
    load.l  r14, 112(r29)
    move.l  r15, #0xFFFFFFFF
    bne     r14, r15, .list_open_dos_done
    load.q  r14, 104(r29)
    move.q  r1, #1
    lsl     r1, r1, r14
    syscall #SYS_WAIT
    load.q  r29, (sp)
    bnez    r2, .list_open_dos_wait
    load.l  r14, 112(r29)
    move.l  r15, #0xFFFFFFFF
    beq     r14, r15, .list_open_dos_wait
.list_open_dos_done:
    store.q r1, 128(r29)               ; dos_library_token
    bnez    r14, .list_open_dos_wait
.list_find_dos:
    load.q  r29, (sp)
    add     r1, r29, #16
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    bnez    r2, .list_open_dos_wait
.list_open_dos_ok:
    store.q r1, 72(r29)
    load.q  r14, 104(r29)
    beqz    r14, .list_open_dos_sigfree_done
    move.q  r1, r14
    syscall #SYS_FREE_SIGNAL
    load.q  r29, (sp)
    store.q r0, 104(r29)
.list_open_dos_sigfree_done:
    bra     .list_open_dos_ready

.list_open_dos_wait:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .list_open_dos_retry
.list_open_dos_alloc_wait:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .list_open_dos_alloc

.list_open_dos_ready:

    move.q  r1, r0
    move.q  r2, r0
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    store.q r1, 80(r29)

    move.l  r1, #0x1000
    move.l  r2, #0x10001
    syscall #SYS_ALLOC_MEM
    load.q  r29, (sp)
    store.q r1, 88(r29)
    store.l r3, 96(r29)

    load.q  r14, 88(r29)
    move.l  r16, #0x44
    store.b r16, (r14)
    add     r14, r14, #1
    move.l  r16, #0x49
    store.b r16, (r14)
    add     r14, r14, #1
    move.l  r16, #0x52
    store.b r16, (r14)
    add     r14, r14, #1
    store.b r0, (r14)
    add     r14, r14, #1
    add     r15, r29, #DATA_ARGS_OFFSET
.list_copy_args:
    load.b  r16, (r15)
    store.b r16, (r14)
    beqz    r16, .list_args_done
    add     r15, r15, #1
    add     r14, r14, #1
    bra     .list_copy_args
.list_args_done:

    move.l  r2, #DOS_RUN
    move.q  r3, r0
    move.q  r4, r0
    load.q  r5, 80(r29)
    load.l  r6, 96(r29)
    load.q  r1, 72(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)

    load.q  r1, 80(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)

.list_done:
    load.q  r1, 128(r29)               ; dos_library_token
    beqz    r1, .list_exit_task
    syscall #SYS_CLOSE_LIBRARY
    load.q  r29, (sp)
    store.q r0, 128(r29)
.list_exit_task:
    move.q  r1, r0
    syscall #SYS_EXIT_TASK

prog_list_cmd_code_end:

prog_list_cmd_data:
    dc.b    "console.handler", 0
    dc.b    "dos.library", 0, 0, 0, 0, 0
    dc.b    "RAM: is empty", 0x0D, 0x0A, 0
    ds.b    16
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8                           ; 104: dos_open_sigbit
    ds.b    16                          ; 112: dos_open outcome scratch
    ds.b    8                           ; 128: dos_library_token
prog_list_cmd_data_end:
    align   8
prog_list_cmd_end:
