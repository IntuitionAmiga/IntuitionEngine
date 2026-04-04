; ============================================================================
; IExec.library - IE64 Microkernel Nucleus (M5: Dynamic Tasks)
; ============================================================================
;
; Amiga Exec-inspired protected microkernel for the IE64 CPU.
; M5: Dynamic task creation/exit, 8-task round-robin scheduler, fault cleanup.
;
; Build:    sdk/bin/ie64asm -I sdk/include sdk/intuitionos/iexec/iexec.s
; Run:      bin/IntuitionEngine -ie64 iexec.ie64
;
; ============================================================================

include "iexec.inc"
include "ie64.inc"

; ============================================================================
; Entry Point ($1000)
; ============================================================================

iexec_start:
    ; ---------------------------------------------------------------
    ; 1. Set trap and interrupt vectors
    ; ---------------------------------------------------------------
    move.l  r1, #trap_handler
    mtcr    cr4, r1
    move.l  r1, #intr_handler
    mtcr    cr7, r1

    ; ---------------------------------------------------------------
    ; 2. Set kernel stack pointer
    ; ---------------------------------------------------------------
    move.l  r1, #KERN_STACK_TOP
    mtcr    cr8, r1

    ; ---------------------------------------------------------------
    ; 3. Build kernel page table (pages 0-383, supervisor only)
    ; ---------------------------------------------------------------
    move.l  r2, #KERN_PAGE_TABLE
    move.l  r4, #0
.kern_pt_loop:
    lsl     r3, r4, #13
    or      r3, r3, #0x0F              ; P|R|W|X (supervisor only)
    lsl     r5, r4, #3
    add     r5, r5, r2
    store.q r3, (r5)
    add     r4, r4, #1
    move.l  r6, #KERN_PAGES
    blt     r4, r6, .kern_pt_loop

    ; 3b. Add supervisor-only mappings for all user pages (8 tasks × 3 pages)
    ; This lets the kernel access any task's code/stack/data for CreateTask etc.
    move.l  r4, #0                      ; task counter
.kern_user_map:
    move.l  r6, #MAX_TASKS
    bge     r4, r6, .kern_user_map_done
    ; Compute base VPN for task i: USER_CODE_VPN_BASE + i * USER_VPN_STRIDE
    move.l  r7, #USER_VPN_STRIDE
    mulu    r7, r4, r7
    add     r7, r7, #USER_CODE_VPN_BASE ; r7 = base VPN for this task
    ; Code page (VPN+0): P|R|W = 0x07 (supervisor, for copy)
    lsl     r3, r7, #13
    or      r3, r3, #0x07
    lsl     r5, r7, #3
    add     r5, r5, r2
    store.q r3, (r5)
    ; Stack page (VPN+1): P|R|W = 0x07
    add     r8, r7, #1
    lsl     r3, r8, #13
    or      r3, r3, #0x07
    lsl     r5, r8, #3
    add     r5, r5, r2
    store.q r3, (r5)
    ; Data page (VPN+2): P|R|W = 0x07
    add     r8, r7, #2
    lsl     r3, r8, #13
    or      r3, r3, #0x07
    lsl     r5, r8, #3
    add     r5, r5, r2
    store.q r3, (r5)
    add     r4, r4, #1
    bra     .kern_user_map
.kern_user_map_done:

    ; ---------------------------------------------------------------
    ; 4. Build user page tables for boot tasks (0 and 1)
    ;    Copy kernel PT entries (pages 0-383 + user supervisor pages)
    ;    then add user-accessible entries for this task's 3 pages.
    ; ---------------------------------------------------------------
    move.l  r10, #0
    jsr     build_user_pt               ; build task 0 PT
    move.l  r10, #1
    jsr     build_user_pt               ; build task 1 PT

    ; ---------------------------------------------------------------
    ; 5. Copy user task code to physical pages (MMU still off)
    ; ---------------------------------------------------------------
    move.l  r1, #user_task0_template
    move.l  r2, #USER_CODE_BASE         ; 0x600000
    move.l  r3, #USER_CODE_SIZE
    move.l  r4, #0
.copy_task0:
    add     r5, r1, r4
    load.q  r6, (r5)
    add     r5, r2, r4
    store.q r6, (r5)
    add     r4, r4, #8
    blt     r4, r3, .copy_task0

    move.l  r1, #user_task1_template
    move.l  r2, #USER_CODE_BASE
    add     r2, r2, #USER_SLOT_STRIDE   ; 0x610000
    move.l  r4, #0
.copy_task1:
    add     r5, r1, r4
    load.q  r6, (r5)
    add     r5, r2, r4
    store.q r6, (r5)
    add     r4, r4, #8
    blt     r4, r3, .copy_task1

    ; Copy child task template to task 0's data page (for CreateTask demo)
    move.l  r1, #child_task_template
    move.l  r2, #USER_DATA_BASE         ; 0x602000 (task 0 data page)
    move.l  r3, #32                      ; 4 instructions = 32 bytes
    move.l  r4, #0
.copy_child:
    add     r5, r1, r4
    load.q  r6, (r5)
    add     r5, r2, r4
    store.q r6, (r5)
    add     r4, r4, #8
    blt     r4, r3, .copy_child

    ; ---------------------------------------------------------------
    ; 6. Initialize task state at KERN_DATA_BASE (MMU still off)
    ; ---------------------------------------------------------------
    move.l  r12, #KERN_DATA_BASE
    store.q r0, (r12)                   ; current_task = 0
    store.q r0, KD_TICK_COUNT(r12)
    move.q  r1, #2
    store.q r1, KD_NUM_TASKS(r12)       ; 2 boot tasks

    ; --- Init boot tasks (0 and 1) as READY ---
    move.l  r2, #KERN_DATA_BASE
    add     r2, r2, #KD_TASK_BASE       ; r2 = &TCB[0]

    ; Task 0
    move.l  r1, #USER_CODE_BASE         ; PC = 0x600000
    store.q r1, KD_TASK_PC(r2)
    move.l  r1, #USER_STACK_BASE
    add     r1, r1, #MMU_PAGE_SIZE      ; USP = stack page top
    store.q r1, KD_TASK_USP(r2)
    move.l  r1, #SIG_SYSTEM_MASK
    store.l r1, KD_TASK_SIG_ALLOC(r2)
    store.l r0, KD_TASK_SIG_WAIT(r2)
    store.l r0, KD_TASK_SIG_RECV(r2)
    store.b r0, KD_TASK_STATE(r2)       ; READY = 0
    move.b  r1, #WAITPORT_NONE
    store.b r1, KD_TASK_WAITPORT(r2)

    ; Task 1
    add     r2, r2, #KD_TASK_STRIDE     ; r2 = &TCB[1]
    move.l  r1, #USER_CODE_BASE
    add     r1, r1, #USER_SLOT_STRIDE   ; PC = 0x610000
    store.q r1, KD_TASK_PC(r2)
    move.l  r1, #USER_STACK_BASE
    add     r1, r1, #USER_SLOT_STRIDE
    add     r1, r1, #MMU_PAGE_SIZE      ; USP = 0x612000
    store.q r1, KD_TASK_USP(r2)
    move.l  r1, #SIG_SYSTEM_MASK
    store.l r1, KD_TASK_SIG_ALLOC(r2)
    store.l r0, KD_TASK_SIG_WAIT(r2)
    store.l r0, KD_TASK_SIG_RECV(r2)
    store.b r0, KD_TASK_STATE(r2)       ; READY = 0
    move.b  r1, #WAITPORT_NONE
    store.b r1, KD_TASK_WAITPORT(r2)

    ; --- Init tasks 2-7 as FREE ---
    move.l  r4, #2                      ; start from task 2
.init_free_tasks:
    move.l  r6, #MAX_TASKS
    bge     r4, r6, .init_free_done
    lsl     r5, r4, #5                  ; task * 32
    add     r5, r5, #KD_TASK_BASE
    move.l  r2, #KERN_DATA_BASE
    add     r5, r5, r2                  ; r5 = &TCB[i]
    store.q r0, KD_TASK_PC(r5)
    store.q r0, KD_TASK_USP(r5)
    store.l r0, KD_TASK_SIG_ALLOC(r5)
    store.l r0, KD_TASK_SIG_WAIT(r5)
    store.l r0, KD_TASK_SIG_RECV(r5)
    move.b  r1, #TASK_FREE
    store.b r1, KD_TASK_STATE(r5)
    move.b  r1, #WAITPORT_NONE
    store.b r1, KD_TASK_WAITPORT(r5)
    add     r4, r4, #1
    bra     .init_free_tasks
.init_free_done:

    ; --- PTBR array (at new offset KD_PTBR_BASE = 320) ---
    move.l  r12, #KERN_DATA_BASE
    move.l  r1, #USER_PT_BASE           ; 0x100000 (task 0 PT)
    store.q r1, KD_PTBR_BASE(r12)
    move.l  r1, #USER_PT_BASE
    add     r1, r1, #USER_SLOT_STRIDE   ; 0x110000 (task 1 PT)
    move.l  r5, #KD_PTBR_BASE
    add     r5, r5, #8
    add     r5, r5, r12
    store.q r1, (r5)                    ; PTBR[1]

    ; ---------------------------------------------------------------
    ; 6b. Initialize port slots (all invalid)
    ; ---------------------------------------------------------------
    move.l  r2, #KERN_DATA_BASE
    add     r2, r2, #KD_PORT_BASE       ; R2 = &port[0]
    move.l  r4, #0
.port_init_loop:
    store.b r0, KD_PORT_VALID(r2)       ; valid = 0
    add     r2, r2, #KD_PORT_STRIDE
    add     r4, r4, #1
    move.l  r5, #KD_PORT_MAX
    blt     r4, r5, .port_init_loop

    ; ---------------------------------------------------------------
    ; 7. Enable MMU
    ; ---------------------------------------------------------------
    move.l  r1, #KERN_PAGE_TABLE
    mtcr    cr0, r1
    move.q  r1, #1
    mtcr    cr5, r1

    ; ---------------------------------------------------------------
    ; 8. Print boot banner (TERM_OUT at page 0xF0 is within kernel PT)
    ; ---------------------------------------------------------------
    la      r8, boot_banner
    jsr     kern_puts

    ; ---------------------------------------------------------------
    ; 9. Program timer
    ; ---------------------------------------------------------------
    move.l  r1, #10000
    mtcr    cr9, r1
    move.l  r1, #10000
    mtcr    cr10, r1
    move.q  r1, #3
    mtcr    cr11, r1

    ; ---------------------------------------------------------------
    ; 10. Enter first user task (task 0)
    ; ---------------------------------------------------------------
    move.l  r12, #KERN_DATA_BASE
    ; Load task 0 USP from TCB[0]
    move.l  r15, #KERN_DATA_BASE
    add     r15, r15, #KD_TASK_BASE     ; &TCB[0]
    load.q  r1, KD_TASK_USP(r15)
    mtcr    cr12, r1
    load.q  r1, KD_TASK_PC(r15)
    mtcr    cr3, r1
    ; Load PTBR[0]
    load.q  r1, KD_PTBR_BASE(r12)
    mtcr    cr0, r1
    tlbflush
    eret

; ============================================================================
; Kernel Output Helpers
; ============================================================================

; kern_put_char: print byte in R8 to TERM_OUT
kern_put_char:
    la      r28, TERM_OUT
    store.b r8, (r28)
    rts

; kern_puts: print null-terminated string at R8 to TERM_OUT
kern_puts:
    la      r28, TERM_OUT
.puts_loop:
    load.b  r29, (r8)
    beqz    r29, .puts_done
    store.b r29, (r28)
    add     r8, r8, #1
    bra     .puts_loop
.puts_done:
    rts

; kern_put_hex: print 64-bit value in R8 as 16-digit hex to TERM_OUT
kern_put_hex:
    push    r20
    push    r21
    push    r22
    la      r28, TERM_OUT
    move.q  r20, r8             ; value to print
    move.q  r21, #60            ; shift amount (start from top nibble)
.hex_loop:
    lsr     r22, r20, r21       ; shift right
    and     r22, r22, #0x0F     ; mask nibble
    ; Convert to ASCII: 0-9 -> '0'-'9', 10-15 -> 'A'-'F'
    move.q  r29, #10
    blt     r22, r29, .hex_digit
    add     r22, r22, #55       ; 'A' - 10 = 55
    bra     .hex_emit
.hex_digit:
    add     r22, r22, #48       ; '0' = 48
.hex_emit:
    store.b r22, (r28)
    sub     r21, r21, #4
    move.q  r29, #0
    bge     r21, r29, .hex_loop
    pop     r22
    pop     r21
    pop     r20
    rts

; ============================================================================
; Round-Robin Scheduler
; ============================================================================
; find_next_runnable: scan for the next task that is not WAITING or FREE.
; Input:  R13 = current task index, R12 = KERN_DATA_BASE
; Output: R13 = next runnable task index
; If no runnable task found, prints DEADLOCK and halts.
; Clobbers: R16-R21

find_next_runnable:
    move.q  r16, r13                    ; save starting point
    move.l  r17, #0                     ; iteration counter
.fnr_loop:
    add     r13, r13, #1
    move.l  r18, #MAX_TASKS
    blt     r13, r18, .fnr_nowrap
    move.l  r13, #0                     ; wrap around
.fnr_nowrap:
    add     r17, r17, #1
    move.l  r18, #MAX_TASKS
    bge     r17, r18, .fnr_deadlock     ; scanned all slots — none runnable
    ; Check task[r13].state
    lsl     r19, r13, #5
    add     r19, r19, #KD_TASK_BASE
    add     r19, r19, r12
    load.b  r20, KD_TASK_STATE(r19)
    move.l  r21, #TASK_WAITING
    beq     r20, r21, .fnr_loop         ; skip WAITING
    move.l  r21, #TASK_FREE
    beq     r20, r21, .fnr_loop         ; skip FREE
    rts                                 ; R13 = next runnable task
.fnr_deadlock:
    la      r8, deadlock_msg
    jsr     kern_puts
    halt

; ============================================================================
; Page Table Builder (used by boot init and CreateTask)
; ============================================================================
; build_user_pt: Build a complete user page table for task r10.
; Input: r10 = task_id
; Clobbers: r2-r9, r10

build_user_pt:
    push    r10
    ; Compute PT base address: USER_PT_BASE + task_id * USER_SLOT_STRIDE
    move.l  r7, #USER_SLOT_STRIDE
    mulu    r7, r10, r7
    add     r7, r7, #USER_PT_BASE      ; r7 = this task's PT base

    ; Copy kernel PT entries for pages 0..KERN_PAGES-1
    move.l  r2, #KERN_PAGE_TABLE
    move.l  r4, #0
.bup_copy_kern:
    move.l  r6, #KERN_PAGES
    bge     r4, r6, .bup_copy_user
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r7
    store.q r3, (r9)
    add     r4, r4, #1
    bra     .bup_copy_kern

.bup_copy_user:
    ; Copy supervisor-only user page entries (VPN 0x600..0x600+MAX_TASKS*0x10)
    move.l  r4, #USER_CODE_VPN_BASE
    move.l  r6, #USER_CODE_VPN_BASE
    move.l  r9, #MAX_TASKS
    move.l  r8, #USER_VPN_STRIDE
    mulu    r9, r9, r8
    add     r6, r6, r9                  ; end VPN
.bup_copy_user_loop:
    bge     r4, r6, .bup_add_user_pages
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r7
    store.q r3, (r9)
    add     r4, r4, #1
    bra     .bup_copy_user_loop

.bup_add_user_pages:
    ; Add 3 user-accessible entries for THIS task's pages
    pop     r10                         ; restore task_id
    move.l  r8, #USER_VPN_STRIDE
    mulu    r8, r10, r8
    add     r8, r8, #USER_CODE_VPN_BASE ; r8 = code VPN
    ; Code: P|X|U = 0x19
    lsl     r3, r8, #13
    or      r3, r3, #0x19
    lsl     r5, r8, #3
    add     r5, r5, r7
    store.q r3, (r5)
    ; Stack (VPN+1): P|R|W|U = 0x17
    add     r8, r8, #1
    lsl     r3, r8, #13
    or      r3, r3, #0x17
    lsl     r5, r8, #3
    add     r5, r5, r7
    store.q r3, (r5)
    ; Data (VPN+2): P|R|W|U = 0x17
    add     r8, r8, #1
    lsl     r3, r8, #13
    or      r3, r3, #0x17
    lsl     r5, r8, #3
    add     r5, r5, r7
    store.q r3, (r5)
    rts

; ============================================================================
; Trap Handler
; ============================================================================

trap_handler:
    mfcr    r10, cr2

    move.l  r11, #FAULT_SYSCALL
    beq     r10, r11, .syscall_dispatch

    ; --- Fault reporting ---
    ; Check previous privilege mode via CR13 (PREV_MODE): 1=supervisor, 0=user
    mfcr    r11, cr13                   ; PREV_MODE
    bnez    r11, .fault_supervisor      ; supervisor fault → kernel panic

    ; --- User-mode fault: kill the task and continue ---
    la      r8, fault_msg_prefix
    jsr     kern_puts
    move.q  r8, r10
    jsr     kern_put_hex
    la      r8, fault_msg_pc
    jsr     kern_puts
    mfcr    r8, cr3
    jsr     kern_put_hex
    la      r8, fault_msg_addr
    jsr     kern_puts
    mfcr    r8, cr1
    jsr     kern_put_hex
    la      r8, fault_msg_task
    jsr     kern_puts
    move.l  r12, #KERN_DATA_BASE
    load.q  r8, (r12)
    jsr     kern_put_hex
    move.q  r8, #0x0A
    jsr     kern_put_char
    ; Kill the faulting task
    load.q  r13, (r12)
    jsr     kill_task_cleanup
    jsr     find_next_runnable
    store.q r13, (r12)
    bra     restore_task

.fault_supervisor:
    ; --- Supervisor fault: KERNEL PANIC ---
    la      r8, panic_msg
    jsr     kern_puts
    la      r8, fault_msg_prefix
    jsr     kern_puts
    move.q  r8, r10
    jsr     kern_put_hex
    la      r8, fault_msg_pc
    jsr     kern_puts
    mfcr    r8, cr3
    jsr     kern_put_hex
    la      r8, fault_msg_addr
    jsr     kern_puts
    mfcr    r8, cr1
    jsr     kern_put_hex
    move.q  r8, #0x0A
    jsr     kern_put_char
    halt

.syscall_dispatch:
    mfcr    r10, cr1

    move.l  r11, #SYS_YIELD
    beq     r10, r11, .do_yield

    move.l  r11, #SYS_GET_SYS_INFO
    beq     r10, r11, .do_getsysinfo

    move.l  r11, #SYS_DEBUG_PUTCHAR
    beq     r10, r11, .do_debug_putchar

    move.l  r11, #SYS_ALLOC_SIGNAL
    beq     r10, r11, .do_alloc_signal

    move.l  r11, #SYS_FREE_SIGNAL
    beq     r10, r11, .do_free_signal

    move.l  r11, #SYS_SIGNAL
    beq     r10, r11, .do_signal

    move.l  r11, #SYS_WAIT
    beq     r10, r11, .do_wait

    move.l  r11, #SYS_CREATE_PORT
    beq     r10, r11, .do_create_port

    move.l  r11, #SYS_PUT_MSG
    beq     r10, r11, .do_put_msg

    move.l  r11, #SYS_GET_MSG
    beq     r10, r11, .do_get_msg

    move.l  r11, #SYS_WAIT_PORT
    beq     r10, r11, .do_wait_port

    move.l  r11, #SYS_CREATE_TASK
    beq     r10, r11, .do_create_task

    move.l  r11, #SYS_EXIT_TASK
    beq     r10, r11, .do_exit_task

    move.q  r2, #ERR_BADARG
    eret

; --- DebugPutChar ---
.do_debug_putchar:
    ; R1 = character (preserved from user context, not clobbered by dispatch)
    la      r28, TERM_OUT
    store.b r1, (r28)
    move.q  r2, #ERR_OK
    eret

; --- Yield ---
.do_yield:
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)              ; current_task

    ; Save current task's state
    lsl     r15, r13, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12
    mfcr    r14, cr3
    store.q r14, KD_TASK_PC(r15)
    mfcr    r14, cr12
    store.q r14, KD_TASK_USP(r15)

    ; Find next runnable task (round-robin scan)
    jsr     find_next_runnable          ; R13 = next runnable (or halts)
    ; If find_next_runnable returned same task, just stay
    move.l  r12, #KERN_DATA_BASE
    load.q  r16, (r12)                  ; original current_task
    beq     r13, r16, .yield_stay

    ; Switch to next
    store.q r13, (r12)
    bra     restore_task

.yield_stay:
    eret

; --- GetSysInfo ---
.do_getsysinfo:
    move.l  r11, #SYSINFO_TICK_COUNT
    beq     r1, r11, .info_ticks
    move.q  r1, #0
    move.q  r2, #ERR_OK
    eret
.info_ticks:
    move.l  r11, #KERN_DATA_BASE
    load.q  r1, KD_TICK_COUNT(r11)
    move.q  r2, #ERR_OK
    eret

; --- AllocSignal ---
; R1 = requested bit (-1 for auto), returns R1 = bit number, R2 = err
.do_alloc_signal:
    ; Get current task's TCB address
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)              ; current_task
    lsl     r15, r13, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12           ; R15 = &TCB[current]

    ; Load current sig_alloc
    load.l  r20, KD_TASK_SIG_ALLOC(r15)

    ; If R1 == -1 (0xFFFFFFFF as unsigned), auto-allocate
    move.l  r21, #0xFFFFFFFF
    bne     r1, r21, .alloc_specific

    ; Auto-allocate: scan bits 16-31 for a free bit (bit=0 in sig_alloc)
    move.l  r22, #16               ; start from bit 16
.alloc_scan:
    move.l  r23, #32
    bge     r22, r23, .alloc_exhausted
    ; Check if bit r22 is free in sig_alloc
    move.q  r23, #1
    lsl     r23, r23, r22          ; mask = 1 << bit
    and     r24, r20, r23
    bnez    r24, .alloc_next       ; bit already allocated
    ; Found free bit — allocate it
    or      r20, r20, r23          ; set bit in sig_alloc
    store.l r20, KD_TASK_SIG_ALLOC(r15)
    move.q  r1, r22                ; return bit number
    move.q  r2, #ERR_OK
    eret
.alloc_next:
    add     r22, r22, #1
    bra     .alloc_scan
.alloc_exhausted:
    move.q  r1, #0xFFFFFFFF        ; -1 = failure
    move.q  r2, #ERR_NOMEM
    eret

.alloc_specific:
    ; R1 = specific bit number requested
    ; Check range 16-31
    move.l  r22, #16
    blt     r1, r22, .alloc_badarg
    move.l  r22, #32
    bge     r1, r22, .alloc_badarg
    ; Check if already allocated
    move.q  r23, #1
    lsl     r23, r23, r1
    and     r24, r20, r23
    bnez    r24, .alloc_badarg     ; already allocated
    ; Allocate
    or      r20, r20, r23
    store.l r20, KD_TASK_SIG_ALLOC(r15)
    move.q  r2, #ERR_OK
    eret
.alloc_badarg:
    move.q  r2, #ERR_BADARG
    eret

; --- FreeSignal ---
; R1 = bit number to free, returns R2 = err
.do_free_signal:
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)
    lsl     r15, r13, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12

    ; Check range 16-31
    move.l  r22, #16
    blt     r1, r22, .free_badarg
    move.l  r22, #32
    bge     r1, r22, .free_badarg

    ; Check bit is actually allocated
    load.l  r20, KD_TASK_SIG_ALLOC(r15)
    move.q  r23, #1
    lsl     r23, r23, r1
    and     r24, r20, r23
    beqz    r24, .free_badarg      ; not allocated → ERR_BADARG

    ; Clear bit from sig_alloc
    not     r23, r23
    and     r20, r20, r23
    store.l r20, KD_TASK_SIG_ALLOC(r15)
    move.q  r2, #ERR_OK
    eret
.free_badarg:
    move.q  r2, #ERR_BADARG
    eret

; --- Signal ---
; R1 = target taskID, R2 = signal mask, returns R2 = err
.do_signal:
    ; Validate taskID (must be 0..MAX_TASKS-1 and not FREE)
    move.l  r22, #MAX_TASKS
    bge     r1, r22, .signal_badarg

    ; Get target TCB
    move.l  r12, #KERN_DATA_BASE
    lsl     r15, r1, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12           ; R15 = &TCB[target]

    ; Reject if target is FREE
    load.b  r22, KD_TASK_STATE(r15)
    move.l  r23, #TASK_FREE
    beq     r22, r23, .signal_badarg

    ; Set bits in target's signal_recv
    load.l  r20, KD_TASK_SIG_RECV(r15)
    or      r20, r20, r2            ; recv |= mask
    store.l r20, KD_TASK_SIG_RECV(r15)

    ; If target is WAITING and (recv & wait) != 0, set to READY
    load.b  r21, KD_TASK_STATE(r15)
    move.l  r22, #TASK_WAITING
    bne     r21, r22, .signal_done

    load.l  r23, KD_TASK_SIG_WAIT(r15)
    and     r24, r20, r23           ; recv & wait
    beqz    r24, .signal_done

    ; Wake the target: set state to READY
    move.b  r21, #TASK_READY
    store.b r21, KD_TASK_STATE(r15)

.signal_done:
    move.q  r2, #ERR_OK
    eret
.signal_badarg:
    move.q  r2, #ERR_BADARG
    eret

; --- Wait ---
; R1 = signal mask to wait for, returns R1 = received signals
.do_wait:
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)              ; current_task
    lsl     r15, r13, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12           ; R15 = &TCB[current]

    ; Check if any waited signals are already pending
    load.l  r20, KD_TASK_SIG_RECV(r15)
    and     r21, r20, r1            ; matched = recv & mask
    beqz    r21, .wait_block

    ; Immediate return: clear consumed signals
    not     r22, r21
    and     r20, r20, r22
    store.l r20, KD_TASK_SIG_RECV(r15)
    move.q  r1, r21                 ; R1 = matched signals
    move.q  r2, #ERR_OK
    eret

.wait_block:
    ; No signals ready — block this task
    store.l r1, KD_TASK_SIG_WAIT(r15)   ; save wait mask
    move.b  r22, #TASK_WAITING
    store.b r22, KD_TASK_STATE(r15)     ; state = WAITING

    ; Fall through to context switch (same as yield but skips WAITING tasks)
    ; Save current task's PC and USP
    mfcr    r14, cr3
    store.q r14, KD_TASK_PC(r15)
    mfcr    r14, cr12
    store.q r14, KD_TASK_USP(r15)

    ; Find next runnable task (round-robin)
    jsr     find_next_runnable          ; R13 = next (or halts if deadlock)

    ; Switch to next task
    store.q r13, (r12)              ; current_task = next
    bra     restore_task           ; shared restore path

; ============================================================================
; Message Port Syscalls
; ============================================================================

; --- CreatePort ---
; Returns R1=portID (0-3), R2=err
.do_create_port:
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)              ; current_task
    ; Scan for unused port slot
    move.l  r20, #0                 ; port index
    move.l  r21, #KERN_DATA_BASE
    add     r21, r21, #KD_PORT_BASE ; R21 = &port[0]
.create_scan:
    move.l  r22, #KD_PORT_MAX
    bge     r20, r22, .create_full
    load.b  r23, KD_PORT_VALID(r21)
    beqz    r23, .create_found
    add     r21, r21, #KD_PORT_STRIDE
    add     r20, r20, #1
    bra     .create_scan
.create_found:
    move.b  r23, #1
    store.b r23, KD_PORT_VALID(r21)     ; valid = 1
    store.b r13, KD_PORT_OWNER(r21)     ; owner = current_task
    store.b r0, KD_PORT_COUNT(r21)      ; count = 0
    store.b r0, KD_PORT_HEAD(r21)       ; head = 0
    store.b r0, KD_PORT_TAIL(r21)       ; tail = 0
    move.q  r1, r20                      ; R1 = portID
    move.q  r2, #ERR_OK
    eret
.create_full:
    move.q  r2, #ERR_NOMEM
    eret

; --- PutMsg ---
; R1=portID, R2=msg_type, R3=msg_data → R2=err
.do_put_msg:
    ; Validate portID
    move.l  r22, #KD_PORT_MAX
    bge     r1, r22, .putmsg_badarg
    ; Compute port address: R21 = KERN_DATA_BASE + KD_PORT_BASE + portID * KD_PORT_STRIDE
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)                  ; current_task (for src_task)
    move.l  r21, #KERN_DATA_BASE
    add     r21, r21, #KD_PORT_BASE
    move.l  r20, #KD_PORT_STRIDE
    mulu    r20, r1, r20
    add     r21, r21, r20               ; R21 = &port[portID]
    ; Check valid
    load.b  r23, KD_PORT_VALID(r21)
    beqz    r23, .putmsg_badarg
    ; Check FIFO not full (count < 4)
    load.b  r24, KD_PORT_COUNT(r21)
    move.l  r25, #KD_PORT_FIFO_SIZE
    bge     r24, r25, .putmsg_full
    ; Enqueue: compute message slot address
    ; slot = port_base + KD_PORT_MSGS + tail * KD_MSG_SIZE
    load.b  r25, KD_PORT_TAIL(r21)
    move.l  r26, #KD_MSG_SIZE
    mulu    r26, r25, r26               ; tail * 16
    add     r26, r26, r21
    add     r26, r26, #KD_PORT_MSGS     ; R26 = &msg_slot
    ; Write message fields
    store.l r2, KD_MSG_TYPE(r26)        ; mn_Type = R2
    store.l r13, KD_MSG_SRC(r26)        ; mn_SrcTask = current_task
    store.q r3, KD_MSG_DATA(r26)        ; mn_Data = R3
    ; Advance tail (mod 4)
    add     r25, r25, #1
    and     r25, r25, #3               ; tail = (tail + 1) & 3
    store.b r25, KD_PORT_TAIL(r21)
    ; Increment count
    add     r24, r24, #1
    store.b r24, KD_PORT_COUNT(r21)
    ; Signal port owner with SIGF_PORT
    load.b  r27, KD_PORT_OWNER(r21)    ; owner task ID
    lsl     r28, r27, #5
    add     r28, r28, #KD_TASK_BASE
    add     r28, r28, r12              ; R28 = &TCB[owner]
    load.l  r29, KD_TASK_SIG_RECV(r28)
    or      r29, r29, #SIGF_PORT
    store.l r29, KD_TASK_SIG_RECV(r28)
    ; If owner is WAITING on SIGF_PORT, set READY
    load.b  r30, KD_TASK_STATE(r28)
    move.l  r20, #TASK_WAITING
    bne     r30, r20, .putmsg_done
    load.l  r20, KD_TASK_SIG_WAIT(r28)
    and     r20, r20, #SIGF_PORT
    beqz    r20, .putmsg_done
    move.b  r30, #TASK_READY
    store.b r30, KD_TASK_STATE(r28)
.putmsg_done:
    move.q  r2, #ERR_OK
    eret
.putmsg_badarg:
    move.q  r2, #ERR_BADARG
    eret
.putmsg_full:
    move.q  r2, #ERR_AGAIN
    eret

; --- GetMsg ---
; R1=portID → R1=msg_type, R2=msg_data, R3=err
.do_get_msg:
    move.l  r22, #KD_PORT_MAX
    bge     r1, r22, .getmsg_badarg
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)
    ; Compute port address
    move.l  r21, #KERN_DATA_BASE
    add     r21, r21, #KD_PORT_BASE
    move.l  r20, #KD_PORT_STRIDE
    mulu    r20, r1, r20
    add     r21, r21, r20
    ; Check valid
    load.b  r23, KD_PORT_VALID(r21)
    beqz    r23, .getmsg_badarg
    ; Check ownership
    load.b  r24, KD_PORT_OWNER(r21)
    bne     r13, r24, .getmsg_perm
    ; Check count > 0
    load.b  r25, KD_PORT_COUNT(r21)
    beqz    r25, .getmsg_empty
    ; Dequeue: read message at head
    load.b  r26, KD_PORT_HEAD(r21)
    move.l  r27, #KD_MSG_SIZE
    mulu    r27, r26, r27
    add     r27, r27, r21
    add     r27, r27, #KD_PORT_MSGS     ; R27 = &msg_slot
    load.l  r1, KD_MSG_TYPE(r27)        ; R1 = msg_type
    load.q  r2, KD_MSG_DATA(r27)        ; R2 = msg_data
    ; Advance head (mod 4)
    add     r26, r26, #1
    and     r26, r26, #3
    store.b r26, KD_PORT_HEAD(r21)
    ; Decrement count
    sub     r25, r25, #1
    store.b r25, KD_PORT_COUNT(r21)
    move.q  r3, #ERR_OK
    eret
.getmsg_badarg:
    move.q  r3, #ERR_BADARG
    eret
.getmsg_perm:
    move.q  r3, #ERR_PERM
    eret
.getmsg_empty:
    move.q  r3, #ERR_AGAIN
    eret

; --- WaitPort ---
; R1=portID → R1=msg_type, R2=msg_data, R3=err
; Loop: check port, if empty Wait(SIGF_PORT), recheck.
.do_wait_port:
    ; Save portID in R5 (not clobbered by Wait internals)
    move.q  r5, r1
.waitport_loop:
    ; Compute port address
    move.l  r22, #KD_PORT_MAX
    bge     r5, r22, .waitport_badarg
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)
    move.l  r21, #KERN_DATA_BASE
    add     r21, r21, #KD_PORT_BASE
    move.l  r20, #KD_PORT_STRIDE
    mulu    r20, r5, r20
    add     r21, r21, r20
    ; Check valid + ownership
    load.b  r23, KD_PORT_VALID(r21)
    beqz    r23, .waitport_badarg
    load.b  r24, KD_PORT_OWNER(r21)
    bne     r13, r24, .waitport_perm
    ; Check count > 0
    load.b  r25, KD_PORT_COUNT(r21)
    bnez    r25, .waitport_dequeue
    ; Empty — block on SIGF_PORT with WaitPort flag
    lsl     r15, r13, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12
    move.l  r22, #SIGF_PORT
    store.l r22, KD_TASK_SIG_WAIT(r15)
    store.b r5, KD_TASK_WAITPORT(r15)      ; mark as WaitPort (portID in R5)
    move.b  r22, #TASK_WAITING
    store.b r22, KD_TASK_STATE(r15)
    mfcr    r14, cr3
    store.q r14, KD_TASK_PC(r15)
    mfcr    r14, cr12
    store.q r14, KD_TASK_USP(r15)
    ; Find next runnable task (round-robin)
    jsr     find_next_runnable          ; R13 = next (or halts if deadlock)
    store.q r13, (r12)
    bra     restore_task
.waitport_dequeue:
    ; Dequeue (same as GetMsg)
    load.b  r26, KD_PORT_HEAD(r21)
    move.l  r27, #KD_MSG_SIZE
    mulu    r27, r26, r27
    add     r27, r27, r21
    add     r27, r27, #KD_PORT_MSGS
    load.l  r1, KD_MSG_TYPE(r27)
    load.q  r2, KD_MSG_DATA(r27)
    add     r26, r26, #1
    and     r26, r26, #3
    store.b r26, KD_PORT_HEAD(r21)
    load.b  r25, KD_PORT_COUNT(r21)
    sub     r25, r25, #1
    store.b r25, KD_PORT_COUNT(r21)
    move.q  r3, #ERR_OK
    eret
.waitport_badarg:
    move.q  r3, #ERR_BADARG
    eret
.waitport_perm:
    move.q  r3, #ERR_PERM
    eret

; ============================================================================
; CreateTask (SYS_CREATE_TASK = 5)
; ============================================================================
; R1 = source_ptr, R2 = code_size (max 4096), R3 = arg0
; Returns R1 = task_id, R2 = err

.do_create_task:
    ; Save syscall args to high registers (build_user_pt clobbers R1-R9)
    move.q  r24, r1                     ; r24 = source_ptr
    move.q  r25, r2                     ; r25 = code_size
    move.q  r26, r3                     ; r26 = arg0

    ; Validate code_size: must be > 0 and <= MMU_PAGE_SIZE (4096)
    beqz    r25, .ct_badarg
    move.l  r11, #MMU_PAGE_SIZE
    bgt     r25, r11, .ct_badarg

    ; Round code_size up to 8-byte alignment for the 8-byte copy loop.
    ; This ensures the source range validation covers the actual bytes read.
    add     r25, r25, #7
    and     r25, r25, #0xFFFFFFF8       ; r25 = aligned code_size

    ; Validate source_ptr range: must be within caller's user region (3 pages)
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)                  ; current_task
    move.l  r11, #USER_SLOT_STRIDE
    mulu    r11, r13, r11
    move.l  r14, #USER_CODE_BASE
    add     r14, r14, r11               ; r14 = caller's user region base
    blt     r24, r14, .ct_badarg        ; source_ptr < base
    add     r16, r24, r25
    sub     r16, r16, #1                ; source_ptr + code_size - 1
    move.l  r17, #0x3000                ; 3 pages
    add     r17, r14, r17               ; base + 0x3000
    bge     r16, r17, .ct_badarg        ; end >= base + 3 pages

    ; Find free TCB slot
    move.l  r20, #0                     ; candidate child_id
.ct_scan:
    move.l  r11, #MAX_TASKS
    bge     r20, r11, .ct_nomem
    lsl     r15, r20, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12
    load.b  r16, KD_TASK_STATE(r15)
    move.l  r11, #TASK_FREE
    beq     r16, r11, .ct_found
    add     r20, r20, #1
    bra     .ct_scan
.ct_found:
    ; r20 = child_id

    ; Save caller's PTBR from CR0
    mfcr    r27, cr0                    ; r27 = saved caller PTBR

    ; Switch to kernel page table for cross-task memory access
    move.l  r11, #KERN_PAGE_TABLE
    mtcr    cr0, r11
    tlbflush

    ; Compute child's code page address
    move.l  r11, #USER_SLOT_STRIDE
    mulu    r11, r20, r11
    move.l  r21, #USER_CODE_BASE
    add     r21, r21, r11               ; r21 = child code addr

    ; Zero child's code page (512 iterations of 8 bytes = 4096)
    move.l  r4, #0
    move.l  r6, #MMU_PAGE_SIZE
.ct_zero:
    bge     r4, r6, .ct_zero_done
    add     r5, r21, r4
    store.q r0, (r5)
    add     r4, r4, #8
    bra     .ct_zero
.ct_zero_done:

    ; Copy code_size bytes from source_ptr to child code page
    ; r24 = source_ptr, r25 = code_size, r21 = dest
    move.l  r4, #0
.ct_copy:
    bge     r4, r25, .ct_copy_done
    add     r5, r24, r4
    load.q  r6, (r5)
    add     r5, r21, r4
    store.q r6, (r5)
    add     r4, r4, #8
    bra     .ct_copy
.ct_copy_done:

    ; Write arg0 to child's data page at offset 0
    move.l  r11, #USER_SLOT_STRIDE
    mulu    r11, r20, r11
    move.l  r22, #USER_DATA_BASE
    add     r22, r22, r11               ; r22 = child data addr
    store.q r26, (r22)                  ; data[0] = arg0

    ; Build child's page table (r10 = child_id)
    move.q  r10, r20
    jsr     build_user_pt

    ; Restore caller's PTBR
    mtcr    cr0, r27
    tlbflush

    ; Init child TCB
    move.l  r12, #KERN_DATA_BASE
    lsl     r15, r20, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12               ; r15 = &TCB[child]

    ; PC = USER_CODE_BASE + child_id * USER_SLOT_STRIDE
    move.l  r11, #USER_SLOT_STRIDE
    mulu    r11, r20, r11
    move.l  r14, #USER_CODE_BASE
    add     r14, r14, r11
    store.q r14, KD_TASK_PC(r15)

    ; USP = USER_STACK_BASE + child_id * USER_SLOT_STRIDE + MMU_PAGE_SIZE
    move.l  r14, #USER_STACK_BASE
    add     r14, r14, r11
    add     r14, r14, #MMU_PAGE_SIZE
    store.q r14, KD_TASK_USP(r15)

    ; Signals
    move.l  r14, #SIG_SYSTEM_MASK
    store.l r14, KD_TASK_SIG_ALLOC(r15)
    store.l r0, KD_TASK_SIG_WAIT(r15)
    store.l r0, KD_TASK_SIG_RECV(r15)

    ; State = READY
    store.b r0, KD_TASK_STATE(r15)

    ; waitport_id = WAITPORT_NONE
    move.b  r14, #WAITPORT_NONE
    store.b r14, KD_TASK_WAITPORT(r15)

    ; Set PTBR[child_id] = USER_PT_BASE + child_id * USER_SLOT_STRIDE
    move.l  r11, #USER_SLOT_STRIDE
    mulu    r11, r20, r11
    move.l  r14, #USER_PT_BASE
    add     r14, r14, r11
    lsl     r16, r20, #3
    add     r16, r16, #KD_PTBR_BASE
    add     r16, r16, r12
    store.q r14, (r16)

    ; Return child_id in R1, ERR_OK in R2
    move.q  r1, r20
    move.q  r2, #ERR_OK
    eret

.ct_badarg:
    move.q  r2, #ERR_BADARG
    eret
.ct_nomem:
    move.q  r2, #ERR_NOMEM
    eret

; ============================================================================
; ExitTask (SYS_EXIT_TASK = 34)
; ============================================================================

.do_exit_task:
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)              ; current_task
    jsr     kill_task_cleanup       ; shared cleanup subroutine
    jsr     find_next_runnable      ; R13 = next (or halts)
    store.q r13, (r12)
    bra     restore_task

; ============================================================================
; kill_task_cleanup: clean up task R13, mark FREE.
; Input: R13 = task to kill, R12 = KERN_DATA_BASE
; Clobbers: R15-R24
; ============================================================================

kill_task_cleanup:
    lsl     r15, r13, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12
    ; Mark FREE
    move.b  r16, #TASK_FREE
    store.b r16, KD_TASK_STATE(r15)
    ; Clear signals
    store.l r0, KD_TASK_SIG_ALLOC(r15)
    store.l r0, KD_TASK_SIG_WAIT(r15)
    store.l r0, KD_TASK_SIG_RECV(r15)
    move.b  r16, #WAITPORT_NONE
    store.b r16, KD_TASK_WAITPORT(r15)
    ; Invalidate ports owned by this task
    move.l  r20, #KERN_DATA_BASE
    add     r20, r20, #KD_PORT_BASE
    move.l  r21, #0
.ktc_port_scan:
    move.l  r22, #KD_PORT_MAX
    bge     r21, r22, .ktc_done
    load.b  r23, KD_PORT_VALID(r20)
    beqz    r23, .ktc_port_next
    load.b  r24, KD_PORT_OWNER(r20)
    bne     r13, r24, .ktc_port_next
    store.b r0, KD_PORT_VALID(r20)
    store.b r0, KD_PORT_COUNT(r20)
.ktc_port_next:
    add     r20, r20, #KD_PORT_STRIDE
    add     r21, r21, #1
    bra     .ktc_port_scan
.ktc_done:
    rts

; ============================================================================
; Shared Task Restore (used by yield, wait, and interrupt handlers)
; ============================================================================
; Entry: R13 = next task index, R12 = KERN_DATA_BASE
; Loads PC, USP, PTBR from TCB. Checks signal_wait for Wait/WaitPort delivery.

restore_task:
    lsl     r15, r13, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12

    ; Load PC
    load.q  r14, KD_TASK_PC(r15)
    mtcr    cr3, r14

    ; Load USP
    load.q  r14, KD_TASK_USP(r15)
    mtcr    cr12, r14

    ; Load PTBR
    lsl     r17, r13, #3
    add     r17, r17, #KD_PTBR_BASE
    add     r17, r17, r12
    load.q  r14, (r17)
    mtcr    cr0, r14
    tlbflush

    ; Check if this task was blocked in Wait (signal_wait != 0)
    load.l  r18, KD_TASK_SIG_WAIT(r15)
    beqz    r18, .restore_normal

    ; Check if this was a WaitPort block (waitport_id != WAITPORT_NONE)
    load.b  r23, KD_TASK_WAITPORT(r15)
    move.l  r24, #WAITPORT_NONE
    bne     r23, r24, .restore_waitport

    ; Plain Wait delivery: compute matched signals
    load.l  r20, KD_TASK_SIG_RECV(r15)
    and     r21, r20, r18           ; matched = recv & wait

    ; Clear consumed signals
    not     r22, r21
    and     r20, r20, r22
    store.l r20, KD_TASK_SIG_RECV(r15)

    ; Clear wait mask
    store.l r0, KD_TASK_SIG_WAIT(r15)

    ; Set state to RUNNING
    move.b  r22, #TASK_RUNNING
    store.b r22, KD_TASK_STATE(r15)

    ; Deliver result in R1
    move.q  r1, r21
    move.q  r2, #ERR_OK

.restore_normal:
    eret

; --- WaitPort resume: dequeue message from port r23 ---
.restore_waitport:
    ; Compute port address
    move.l  r21, #KERN_DATA_BASE
    add     r21, r21, #KD_PORT_BASE
    move.l  r20, #KD_PORT_STRIDE
    mulu    r20, r23, r20
    add     r21, r21, r20               ; R21 = &port[waitport_id]

    ; Check count > 0 (spurious wake if another port got SIGF_PORT)
    load.b  r25, KD_PORT_COUNT(r21)
    beqz    r25, .restore_waitport_spurious

    ; Message available — dequeue
    load.b  r26, KD_PORT_HEAD(r21)
    move.l  r27, #KD_MSG_SIZE
    mulu    r27, r26, r27
    add     r27, r27, r21
    add     r27, r27, #KD_PORT_MSGS
    load.l  r1, KD_MSG_TYPE(r27)
    load.q  r2, KD_MSG_DATA(r27)
    add     r26, r26, #1
    and     r26, r26, #3
    store.b r26, KD_PORT_HEAD(r21)
    sub     r25, r25, #1
    store.b r25, KD_PORT_COUNT(r21)

    ; Clear wait state
    store.l r0, KD_TASK_SIG_WAIT(r15)
    move.b  r24, #WAITPORT_NONE
    store.b r24, KD_TASK_WAITPORT(r15)
    move.b  r22, #TASK_RUNNING
    store.b r22, KD_TASK_STATE(r15)

    ; Clear consumed SIGF_PORT from recv
    load.l  r20, KD_TASK_SIG_RECV(r15)
    move.l  r24, #SIGF_PORT
    not     r22, r24
    and     r20, r20, r22
    store.l r20, KD_TASK_SIG_RECV(r15)

    move.q  r3, #ERR_OK
    eret

.restore_waitport_spurious:
    ; Port empty — spurious wake from different port sharing SIGF_PORT.
    ; Clear consumed signal, re-block, switch to other task.
    load.l  r20, KD_TASK_SIG_RECV(r15)
    move.l  r24, #SIGF_PORT
    not     r22, r24
    and     r20, r20, r22
    store.l r20, KD_TASK_SIG_RECV(r15)

    ; Re-block (state=WAITING; signal_wait and waitport_id remain set)
    move.b  r22, #TASK_WAITING
    store.b r22, KD_TASK_STATE(r15)

    ; Find next runnable task (round-robin)
    jsr     find_next_runnable          ; R13 = next (or halts if deadlock)
    store.q r13, (r12)
    bra     restore_task

; ============================================================================
; Interrupt Handler (Timer Preemption)
; ============================================================================

intr_handler:
    move.l  r12, #KERN_DATA_BASE

    ; Increment tick count
    load.q  r10, KD_TICK_COUNT(r12)
    add     r10, r10, #1
    store.q r10, KD_TICK_COUNT(r12)

    ; Heartbeat: print '.' every IEXEC_HEARTBEAT_INTERVAL ticks
    and     r11, r10, #IEXEC_HEARTBEAT_INTERVAL-1
    bnez    r11, .no_heartbeat
    move.q  r8, #0x2E              ; '.'
    jsr     kern_put_char
.no_heartbeat:

    ; Context switch
    load.q  r13, (r12)              ; current_task

    ; Save current task
    lsl     r15, r13, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12
    mfcr    r14, cr3
    store.q r14, KD_TASK_PC(r15)
    mfcr    r14, cr12
    store.q r14, KD_TASK_USP(r15)

    ; Find next runnable task (round-robin)
    jsr     find_next_runnable          ; R13 = next (or halts if deadlock)
    ; If same task, stay on current
    load.q  r16, (r12)                  ; original current_task
    beq     r13, r16, .intr_stay

    ; Switch to next
    store.q r13, (r12)
    bra     restore_task

.intr_stay:
    eret

; ============================================================================
; User Task Templates (copied to user code pages during init)
; ============================================================================

; Task 0 (M5 demo): Create port, create child task, print 'A' + yield loop.
; Child code is pre-loaded into task 0's data page during boot init.
; 12 instructions = 96 bytes. 'A' marker at instruction 5 (byte 40).
user_task0_template:
    syscall #SYS_CREATE_PORT        ; 0: create port (gets port 0)
    move.l  r1, #USER_DATA_BASE    ; 1: source_ptr = data page (child code)
    move.l  r2, #32                ; 2: code_size = 4 instructions
    move.l  r3, #0                 ; 3: arg0 = 0
    syscall #SYS_CREATE_TASK       ; 4: create child task
.t0_loop:
    move.l  r1, #0x41              ; 5: 'A' ← findTaskTemplates marker
    syscall #SYS_DEBUG_PUTCHAR     ; 6
    syscall #SYS_YIELD             ; 7
    bra     .t0_loop               ; 8
    move.q  r0, r0                 ; 9: pad
    move.q  r0, r0                 ; 10: pad
    move.q  r0, r0                 ; 11: pad
user_task0_template_end:

; Task 1 (M5 demo): simple yield loop printing 'B'.
; 12 instructions = 96 bytes.
user_task1_template:
.t1_loop:
    move.l  r1, #0x42              ; 0: 'B'
    syscall #SYS_DEBUG_PUTCHAR     ; 1
    syscall #SYS_YIELD             ; 2
    bra     .t1_loop               ; 3
    move.q  r0, r0                 ; 4-11: pad
    move.q  r0, r0
    move.q  r0, r0
    move.q  r0, r0
    move.q  r0, r0
    move.q  r0, r0
    move.q  r0, r0
    move.q  r0, r0
user_task1_template_end:

; Child task code (copied to task 0's data page during boot init).
; CreateTask copies this from the data page to the child's code page.
; 4 instructions = 32 bytes. Prints 'C', yields, loops.
child_task_template:
.child_loop:
    move.l  r1, #0x43              ; 'C'
    syscall #SYS_DEBUG_PUTCHAR
    syscall #SYS_YIELD
    bra     .child_loop             ; loop back to start
child_task_template_end:

; ============================================================================
; Data: Strings
; ============================================================================

boot_banner:
    dc.b    "IExec M5 boot", 0x0A, 0
    align   4

fault_msg_prefix:
    dc.b    "FAULT cause=", 0
    align   4

fault_msg_pc:
    dc.b    " PC=", 0
    align   4

fault_msg_addr:
    dc.b    " ADDR=", 0
    align   4

deadlock_msg:
    dc.b    "DEADLOCK: no runnable tasks", 0x0A, 0
    align   4

fault_msg_task:
    dc.b    " task=", 0
    align   4

panic_msg:
    dc.b    "KERNEL PANIC: ", 0
    align   4

; ============================================================================
; End of IExec M5
; ============================================================================
