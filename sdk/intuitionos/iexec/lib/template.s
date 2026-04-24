; Shared M16 library-side boilerplate macros.
; These keep entry/register/banner/expunge plumbing consistent across
; protected libraries while leaving each library's service logic local.

m16_lib_preamble macro
    sub     sp, sp, #16
    load.q  r30, (sp)
    load.q  r29, 8(sp)
    store.q r29, (sp)
    load.l  r1, TASKSB_TASK_ID(r30)
    store.q r1, \1(r29)
endm

m16_lib_register macro
    add     r1, r29, #\1
    move.l  r2, #PF_PUBLIC
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    bnez    r2, \7
    store.q r1, \4(r29)
    add     r1, r29, #\1
    move.l  r2, #\2
    move.l  r3, #\3
    load.q  r4, \4(r29)
    syscall #SYS_ADD_LIBRARY
    load.q  r29, (sp)
    beqz    r2, \5
    move.l  r1, #ERR_PERM
    beq     r2, r1, \6
    bra     \7
endm

m16_lib_print_banner macro
\3:
\4:
endm

m16_lib_accept_expunge macro
    move.l  r1, #1
    load.q  r2, \1(r29)
    load.q  r3, \2(r29)
    syscall #SYS_M16_EXPUNGE_RESULT
    load.q  r29, (sp)
    bnez    r2, \3
    syscall #SYS_EXIT_TASK
endm

m16_lib_refuse_expunge macro
    move.q  r1, r0
    load.q  r2, \1(r29)
    load.q  r3, \2(r29)
    syscall #SYS_M16_EXPUNGE_RESULT
    load.q  r29, (sp)
    bra     \3
endm
