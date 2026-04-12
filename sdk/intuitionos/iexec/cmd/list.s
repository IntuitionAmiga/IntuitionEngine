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

.list_open_dos_retry:
    load.q  r29, (sp)
    add     r1, r29, #16
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    beqz    r2, .list_open_dos_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .list_open_dos_retry
.list_open_dos_ok:
    store.q r1, 72(r29)

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
prog_list_cmd_data_end:
    align   8
prog_list_cmd_end:
