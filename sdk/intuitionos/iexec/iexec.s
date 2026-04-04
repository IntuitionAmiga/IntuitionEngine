; ============================================================================
; IExec.library - IE64 Microkernel Nucleus (M7: Named Ports + Reply Protocol)
; ============================================================================
;
; Amiga Exec-inspired protected microkernel for the IE64 CPU.
; M7: Named public MsgPorts, FindPort discovery, request/reply messaging,
;     shared-memory handoff through messages.
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
    move.l  r3, #88                      ; 11 instructions = 88 bytes
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
    store.q r0, KD_NONCE_COUNTER(r12)   ; nonce counter = 0

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
    ; 6b. Initialize port slots (all invalid, M7: 8 ports x 160 bytes)
    ; ---------------------------------------------------------------
    move.l  r2, #KERN_DATA_BASE
    add     r2, r2, #KD_PORT_BASE       ; R2 = &port[0]
    move.l  r4, #0
.port_init_loop:
    store.b r0, KD_PORT_VALID(r2)       ; valid = 0
    store.b r0, KD_PORT_FLAGS(r2)       ; flags = 0
    ; Zero name field (16 bytes = 2 quad-words)
    store.q r0, KD_PORT_NAME(r2)
    add     r6, r2, #KD_PORT_NAME
    add     r6, r6, #8
    store.q r0, (r6)
    add     r2, r2, #KD_PORT_STRIDE
    add     r4, r4, #1
    move.l  r5, #KD_PORT_MAX
    blt     r4, r5, .port_init_loop

    ; ---------------------------------------------------------------
    ; 6c. Zero M6 kernel data structures (bitmap, region table, shmem table)
    ; ---------------------------------------------------------------
    ; Zero page bitmap (800 bytes at KD_PAGE_BITMAP)
    move.l  r2, #KERN_DATA_BASE
    add     r2, r2, #KD_PAGE_BITMAP     ; r2 = bitmap start
    move.l  r3, #KD_PAGE_BITMAP_SZ
    lsr     r3, r3, #3                  ; r3 = 800/8 = 100 quad-words
    move.l  r4, #0
.zero_bitmap:
    store.q r0, (r2)
    add     r2, r2, #8
    add     r4, r4, #1
    blt     r4, r3, .zero_bitmap

    ; Zero region table (1024 bytes at KD_REGION_TABLE)
    move.l  r2, #KERN_DATA_BASE
    add     r2, r2, #KD_REGION_TABLE
    move.l  r3, #128                    ; 1024/8 = 128 quad-words
    move.l  r4, #0
.zero_regions:
    store.q r0, (r2)
    add     r2, r2, #8
    add     r4, r4, #1
    blt     r4, r3, .zero_regions

    ; Zero shared object table (128 bytes at KD_SHMEM_TABLE)
    move.l  r2, #KERN_DATA_BASE
    add     r2, r2, #KD_SHMEM_TABLE
    move.l  r3, #16                     ; 128/8 = 16 quad-words
    move.l  r4, #0
.zero_shmem:
    store.q r0, (r2)
    add     r2, r2, #8
    add     r4, r4, #1
    blt     r4, r3, .zero_shmem

    ; ---------------------------------------------------------------
    ; 6d. Extend kernel PT: map allocation pool pages (0x700-0x1FFF)
    ;     as supervisor P|R|W for MEMF_CLEAR zeroing
    ; ---------------------------------------------------------------
    move.l  r2, #KERN_PAGE_TABLE
    move.l  r4, #ALLOC_POOL_BASE        ; start page = 0x700
    move.l  r6, #ALLOC_POOL_BASE
    move.l  r7, #ALLOC_POOL_PAGES
    add     r6, r6, r7                  ; end page = 0x700 + 6400 = 0x2000
.kern_pool_map:
    lsl     r3, r4, #13                 ; PPN << 13
    or      r3, r3, #0x07              ; P|R|W (supervisor only)
    lsl     r5, r4, #3                  ; offset in PT = page * 8
    add     r5, r5, r2
    store.q r3, (r5)
    add     r4, r4, #1
    blt     r4, r6, .kern_pool_map

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
    ; Scan all MAX_TASKS slots starting from (current+1), wrapping around.
    ; If none found (including current), deadlock. The scan covers current+1
    ; through current (inclusive) = all MAX_TASKS slots.
    move.q  r16, r13                    ; save original
    move.l  r17, #0                     ; iteration counter
    add     r13, r13, #1               ; start from next slot
    move.l  r18, #MAX_TASKS
    blt     r13, r18, .fnr_check
    move.l  r13, #0                     ; wrap
.fnr_check:
    ; Check task[r13].state
    lsl     r19, r13, #5
    add     r19, r19, #KD_TASK_BASE
    add     r19, r19, r12
    load.b  r20, KD_TASK_STATE(r19)
    move.l  r21, #TASK_WAITING
    beq     r20, r21, .fnr_next         ; skip WAITING
    move.l  r21, #TASK_FREE
    beq     r20, r21, .fnr_next         ; skip FREE
    rts                                 ; R13 = runnable task (may be same as original)
.fnr_next:
    add     r17, r17, #1
    move.l  r18, #MAX_TASKS
    bge     r17, r18, .fnr_deadlock     ; checked all MAX_TASKS slots — none runnable
    add     r13, r13, #1
    move.l  r18, #MAX_TASKS
    blt     r13, r18, .fnr_check
    move.l  r13, #0                     ; wrap
    bra     .fnr_check
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
; Page Allocator (M6)
; ============================================================================

; alloc_pages: allocate N contiguous physical pages from the pool.
; Input:  R1 = number of pages requested
; Output: R1 = base PPN (absolute page number, not pool-relative), R2 = ERR_OK
;         On failure: R1 = 0, R2 = ERR_NOMEM
; Clobbers: R3-R9
alloc_pages:
    move.l  r3, #KERN_DATA_BASE
    add     r3, r3, #KD_PAGE_BITMAP    ; r3 = bitmap base addr
    move.l  r4, #0                     ; r4 = bit index (0..ALLOC_POOL_PAGES-1)
    move.l  r5, #ALLOC_POOL_PAGES      ; r5 = total bits
.ap_scan:
    bge     r4, r5, .ap_fail           ; exhausted bitmap
    ; Check if N contiguous bits starting at r4 are all free (0)
    move.q  r6, r4                     ; r6 = candidate start
    move.q  r7, r4                     ; r7 = scan cursor
    add     r8, r4, r1                 ; r8 = candidate end (exclusive)
    bgt     r8, r5, .ap_fail           ; would exceed pool
.ap_check_bit:
    bge     r7, r8, .ap_found          ; all bits free
    ; Read bit r7: byte = bitmap[r7/8], bit = r7 % 8
    lsr     r9, r7, #3                 ; r9 = byte index
    add     r9, r9, r3                 ; r9 = &bitmap[byte]
    load.b  r9, (r9)                   ; r9 = bitmap byte
    and     r2, r7, #7                 ; r2 = bit position (0-7)
    lsr     r9, r9, r2                 ; shift target bit to position 0
    and     r9, r9, #1
    bnez    r9, .ap_skip               ; bit is set (page in use)
    add     r7, r7, #1
    bra     .ap_check_bit
.ap_skip:
    ; Skip to bit after the occupied one
    add     r4, r7, #1
    bra     .ap_scan
.ap_found:
    ; Mark bits r6..r6+r1-1 as used
    move.q  r4, r6                     ; r4 = start bit
    add     r8, r6, r1                 ; r8 = end bit (exclusive)
.ap_mark:
    bge     r4, r8, .ap_done
    ; Set bit r4: bitmap[r4/8] |= (1 << (r4 % 8))
    lsr     r9, r4, #3                 ; byte index
    add     r7, r9, r3                 ; r7 = &bitmap[byte]
    load.b  r9, (r7)                   ; current byte
    and     r2, r4, #7                 ; bit position
    move.l  r6, #1
    lsl     r6, r6, r2                 ; mask = 1 << bit
    or      r9, r9, r6                 ; set bit
    store.b r9, (r7)
    add     r4, r4, #1
    ; Restore r6 = original start bit (it was clobbered)
    sub     r6, r8, r1
    bra     .ap_mark
.ap_done:
    ; Return base PPN = ALLOC_POOL_BASE + start_bit
    sub     r6, r8, r1                 ; r6 = start bit (re-derive)
    move.l  r1, #ALLOC_POOL_BASE
    add     r1, r1, r6                 ; R1 = absolute PPN
    move.q  r2, #ERR_OK
    rts
.ap_fail:
    move.q  r1, #0
    move.q  r2, #ERR_NOMEM
    rts

; free_pages: release N contiguous physical pages back to the pool.
; Input:  R1 = base PPN (absolute page number)
;         R2 = number of pages to free
; Clobbers: R3-R7
free_pages:
    move.l  r3, #KERN_DATA_BASE
    add     r3, r3, #KD_PAGE_BITMAP    ; r3 = bitmap base addr
    ; Convert absolute PPN to pool-relative bit index
    move.l  r4, #ALLOC_POOL_BASE
    sub     r4, r1, r4                 ; r4 = start bit
    add     r5, r4, r2                 ; r5 = end bit (exclusive)
.fp_loop:
    bge     r4, r5, .fp_done
    ; Clear bit r4: bitmap[r4/8] &= ~(1 << (r4 % 8))
    lsr     r6, r4, #3                 ; byte index
    add     r7, r6, r3                 ; r7 = &bitmap[byte]
    load.b  r6, (r7)                   ; current byte
    and     r2, r4, #7                 ; bit position (clobbers R2 but we saved count in R5)
    move.l  r1, #1
    lsl     r1, r1, r2                 ; mask = 1 << bit
    move.l  r2, #0xFF
    sub     r2, r2, r1                 ; NOT mask (complement via 0xFF - mask, works for single byte)
    and     r6, r6, r2                 ; clear bit
    store.b r6, (r7)
    add     r4, r4, #1
    bra     .fp_loop
.fp_done:
    rts

; count_free_pages: count the number of free (0) bits in the page bitmap.
; Output: R1 = number of free pages
; Clobbers: R2-R6
count_free_pages:
    move.l  r2, #KERN_DATA_BASE
    add     r2, r2, #KD_PAGE_BITMAP    ; r2 = bitmap base
    move.l  r3, #KD_PAGE_BITMAP_SZ     ; r3 = 800 bytes
    move.q  r1, #0                     ; r1 = free count
    move.l  r4, #0                     ; r4 = byte index
.cfp_byte:
    bge     r4, r3, .cfp_done
    add     r5, r2, r4
    load.b  r5, (r5)                   ; r5 = bitmap byte
    ; Count zero bits = 8 - popcount(byte)
    move.l  r6, #0                     ; bit counter
.cfp_bit:
    move.l  r7, #8
    bge     r6, r7, .cfp_next_byte
    move.l  r7, #1
    lsl     r7, r7, r6
    and     r7, r7, r5
    bnez    r7, .cfp_bit_set
    add     r1, r1, #1                 ; free bit found
.cfp_bit_set:
    add     r6, r6, #1
    bra     .cfp_bit
.cfp_next_byte:
    add     r4, r4, #1
    bra     .cfp_byte
.cfp_done:
    ; Adjust: last 0 bits past ALLOC_POOL_PAGES (6400 bits = 800 bytes exact, no adjustment needed)
    rts

; ============================================================================
; Memory Mapping Helpers (M6)
; ============================================================================

; map_pages: write PTEs for contiguous pages into a task's page table.
; Input:  R1 = PT base address (physical)
;         R2 = VA of first page
;         R3 = PPN of first physical page
;         R4 = number of pages
;         R5 = PTE flags (e.g. P|R|W|U = 0x17)
; Clobbers: R6-R9
map_pages:
    move.l  r6, #0                     ; counter
.mp_loop:
    bge     r6, r4, .mp_done
    ; Compute VPN = VA >> 12
    add     r7, r3, r6                 ; r7 = PPN + i
    lsl     r8, r7, #13               ; PPN << 13
    or      r8, r8, r5                 ; PTE = (PPN << 13) | flags
    ; Compute PTE offset: (VA >> 12 + i) * 8
    lsr     r9, r2, #12               ; base VPN
    add     r9, r9, r6                 ; VPN + i
    lsl     r9, r9, #3                ; * 8
    add     r9, r9, r1                 ; &PT[VPN+i]
    store.q r8, (r9)
    add     r6, r6, #1
    bra     .mp_loop
.mp_done:
    rts

; unmap_pages: clear PTEs for contiguous pages in a task's page table.
; Input:  R1 = PT base address (physical)
;         R2 = VA of first page
;         R3 = number of pages
; Clobbers: R4-R6
unmap_pages:
    move.l  r4, #0                     ; counter
.ump_loop:
    bge     r4, r3, .ump_done
    lsr     r5, r2, #12               ; base VPN
    add     r5, r5, r4                 ; VPN + i
    lsl     r5, r5, #3                ; * 8
    add     r5, r5, r1                 ; &PT[VPN+i]
    store.q r0, (r5)                   ; clear PTE
    add     r4, r4, #1
    bra     .ump_loop
.ump_done:
    rts

; find_free_va: find a gap in the task's dynamic VA window for N pages.
; Input:  R1 = task_id
;         R2 = number of pages needed
; Output: R1 = VA of the gap, R2 = ERR_OK
;         On failure: R1 = 0, R2 = ERR_NOMEM
; Clobbers: R3-R16
;
; Algorithm: scan region table, collect used VA ranges, find first gap.
; Simple approach: try each page-aligned VA in the task's window,
; check if it overlaps any existing region.
find_free_va:
    ; Compute task's dynamic VA window
    move.l  r3, #USER_DYN_STRIDE
    mulu    r3, r1, r3                 ; r3 = task_id * stride
    move.l  r4, #USER_DYN_BASE
    add     r4, r4, r3                 ; r4 = window start VA
    move.l  r5, #USER_DYN_PAGES
    lsl     r5, r5, #12               ; r5 = window size in bytes
    add     r5, r5, r4                 ; r5 = window end VA (exclusive)
    ; Compute task's region table base
    move.l  r6, #KERN_DATA_BASE
    move.l  r7, #KD_REGION_TASK_SZ
    mulu    r7, r1, r7                 ; r7 = task_id * region_task_sz
    add     r6, r6, #KD_REGION_TABLE
    add     r6, r6, r7                 ; r6 = &region_table[task_id][0]
    ; r2 = pages needed, compute size in bytes
    lsl     r7, r2, #12               ; r7 = needed_bytes
    ; Try candidate VA = window start, stepping by page size
    move.q  r8, r4                     ; r8 = candidate VA
.ffv_try:
    ; Would candidate + needed exceed window?
    add     r9, r8, r7                 ; r9 = candidate end
    bgt     r9, r5, .ffv_fail          ; exceeds window
    ; Check if candidate overlaps any active region
    move.l  r10, #0                    ; region index
.ffv_check_region:
    move.l  r11, #KD_REGION_MAX
    bge     r10, r11, .ffv_found       ; no overlap with any region
    ; Compute region entry address
    move.l  r12, #KD_REGION_STRIDE
    mulu    r12, r10, r12
    add     r12, r12, r6               ; r12 = &region[i]
    ; Check type
    load.b  r13, KD_REG_TYPE(r12)
    beqz    r13, .ffv_next_region      ; REGION_FREE, skip
    ; Load region VA and size
    load.l  r14, KD_REG_VA(r12)        ; region VA
    load.w  r15, KD_REG_PAGES(r12)     ; region page count
    lsl     r15, r15, #12              ; region size in bytes
    add     r16, r14, r15              ; region end VA
    ; Check overlap: candidate [r8, r9) vs region [r14, r16)
    ; Overlap if: candidate_start < region_end AND region_start < candidate_end
    bge     r8, r16, .ffv_next_region  ; candidate starts after region ends
    bge     r14, r9, .ffv_next_region  ; region starts after candidate ends
    ; Overlap detected — advance candidate past this region
    move.q  r8, r16                    ; candidate = region end
    bra     .ffv_try                   ; retry with new candidate
.ffv_next_region:
    add     r10, r10, #1
    bra     .ffv_check_region
.ffv_found:
    move.q  r1, r8                     ; R1 = VA
    move.q  r2, #ERR_OK
    rts
.ffv_fail:
    move.q  r1, #0
    move.q  r2, #ERR_NOMEM
    rts

; ============================================================================
; M7: Port Name Helpers
; ============================================================================

; safe_copy_user_name: validate PTE and copy up to 16 bytes from user VA
;   to kernel scratch buffer at KERN_DATA_BASE + KD_NAME_SCRATCH.
; Input:  R1 = user VA (name pointer), R13 = current_task (for PTBR lookup)
; Output: R23 = 0 on success, ERR_BADARG on failure
; Clobbers: R14, R16, R17, R18, R19, R23, R24, R25, R26
safe_copy_user_name:
    push    r1
    ; --- Validate PTE for first page ---
    ; Compute VPN = VA >> 12
    lsr     r14, r1, #12               ; R14 = VPN of name_ptr
    ; Load caller's PTBR from PTBR array
    move.l  r16, #KERN_DATA_BASE
    lsl     r17, r13, #3               ; task_id * 8
    add     r17, r17, #KD_PTBR_BASE
    add     r17, r17, r16
    load.q  r17, (r17)                 ; R17 = caller's PT base address
    ; Load PTE at PT[VPN]
    lsl     r18, r14, #3               ; VPN * 8 (PTE offset)
    add     r18, r18, r17
    load.q  r18, (r18)                 ; R18 = PTE
    ; Check P|R|U (0x13)
    and     r19, r18, #0x13
    move.l  r24, #0x13
    bne     r19, r24, .scun_bad

    ; --- Validate PTE for last byte's page (name_ptr + 15) ---
    pop     r1
    push    r1
    add     r14, r1, #15
    lsr     r14, r14, #12               ; VPN of name_ptr+15
    lsl     r18, r14, #3
    add     r18, r18, r17
    load.q  r18, (r18)
    and     r19, r18, #0x13
    bne     r19, r24, .scun_bad

    ; --- Copy up to 16 bytes to scratch buffer ---
    pop     r1                          ; restore user VA
    move.l  r25, #KERN_DATA_BASE
    add     r25, r25, #KD_NAME_SCRATCH  ; R25 = &scratch
    move.l  r26, #0                     ; byte counter
.scun_copy:
    move.l  r24, #PORT_NAME_LEN
    bge     r26, r24, .scun_ok
    add     r14, r1, r26
    load.b  r14, (r14)                  ; read byte from user memory
    add     r18, r25, r26
    store.b r14, (r18)                  ; write to scratch
    beqz    r14, .scun_pad              ; NUL → zero-pad rest
    add     r26, r26, #1
    bra     .scun_copy
.scun_pad:
    ; Zero remaining bytes
    add     r26, r26, #1
.scun_pad_loop:
    move.l  r24, #PORT_NAME_LEN
    bge     r26, r24, .scun_ok
    add     r18, r25, r26
    store.b r0, (r18)
    add     r26, r26, #1
    bra     .scun_pad_loop
.scun_ok:
    move.q  r23, #0                     ; success
    rts
.scun_bad:
    pop     r1                          ; balance stack
    move.q  r23, #ERR_BADARG
    rts

; case_insensitive_cmp: compare two 16-byte buffers case-insensitively
; Input:  R24 = ptr_a, R25 = ptr_b
; Output: R23 = 0 if match, 1 if mismatch
; Clobbers: R14, R16, R17, R18, R23, R26
case_insensitive_cmp:
    move.l  r26, #0                     ; byte counter
.cic_loop:
    move.l  r18, #PORT_NAME_LEN
    bge     r26, r18, .cic_match
    add     r14, r24, r26
    load.b  r14, (r14)                  ; byte_a
    add     r16, r25, r26
    load.b  r16, (r16)                  ; byte_b
    ; To upper: if 0x61 <= b <= 0x7A, subtract 0x20
    move.l  r17, #0x61
    blt     r14, r17, .cic_a_done
    move.l  r17, #0x7B
    bge     r14, r17, .cic_a_done
    sub     r14, r14, #0x20
.cic_a_done:
    move.l  r17, #0x61
    blt     r16, r17, .cic_b_done
    move.l  r17, #0x7B
    bge     r16, r17, .cic_b_done
    sub     r16, r16, #0x20
.cic_b_done:
    bne     r14, r16, .cic_mismatch
    add     r26, r26, #1
    bra     .cic_loop
.cic_match:
    move.q  r23, #0
    rts
.cic_mismatch:
    move.q  r23, #1
    rts

; check_name_unique: scan ports for a duplicate public name
; Input:  scratch buffer at KERN_DATA_BASE + KD_NAME_SCRATCH already filled
; Output: R23 = 0 if unique, 1 if duplicate found
; Clobbers: R14, R16, R17, R18, R19, R20, R23, R24, R25, R26
check_name_unique:
    move.l  r19, #0                     ; port index
    move.l  r20, #KERN_DATA_BASE
    add     r20, r20, #KD_PORT_BASE     ; R20 = &port[0]
.cnu_loop:
    move.l  r18, #KD_PORT_MAX
    bge     r19, r18, .cnu_unique
    load.b  r14, KD_PORT_VALID(r20)
    beqz    r14, .cnu_next
    load.b  r14, KD_PORT_FLAGS(r20)
    and     r14, r14, #PF_PUBLIC
    beqz    r14, .cnu_next
    ; Compare: port name at R20+KD_PORT_NAME vs scratch buffer
    move.q  r24, r20
    add     r24, r24, #KD_PORT_NAME     ; ptr_a = port name
    move.l  r25, #KERN_DATA_BASE
    add     r25, r25, #KD_NAME_SCRATCH  ; ptr_b = scratch
    push    r19
    push    r20
    jsr     case_insensitive_cmp        ; R23 = 0 if match
    pop     r20
    pop     r19
    beqz    r23, .cnu_dup               ; match found → duplicate
.cnu_next:
    add     r20, r20, #KD_PORT_STRIDE
    add     r19, r19, #1
    bra     .cnu_loop
.cnu_unique:
    move.q  r23, #0
    rts
.cnu_dup:
    move.q  r23, #1
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

    move.l  r11, #SYS_FIND_PORT
    beq     r10, r11, .do_find_port

    move.l  r11, #SYS_PUT_MSG
    beq     r10, r11, .do_put_msg

    move.l  r11, #SYS_GET_MSG
    beq     r10, r11, .do_get_msg

    move.l  r11, #SYS_WAIT_PORT
    beq     r10, r11, .do_wait_port

    move.l  r11, #SYS_REPLY_MSG
    beq     r10, r11, .do_reply_msg

    move.l  r11, #SYS_CREATE_TASK
    beq     r10, r11, .do_create_task

    move.l  r11, #SYS_EXIT_TASK
    beq     r10, r11, .do_exit_task

    move.l  r11, #SYS_ALLOC_MEM
    beq     r10, r11, .do_alloc_mem

    move.l  r11, #SYS_FREE_MEM
    beq     r10, r11, .do_free_mem

    move.l  r11, #SYS_MAP_SHARED
    beq     r10, r11, .do_map_shared

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
    move.l  r11, #SYSINFO_TOTAL_PAGES
    beq     r1, r11, .info_total_pages
    move.l  r11, #SYSINFO_FREE_PAGES
    beq     r1, r11, .info_free_pages
    move.l  r11, #SYSINFO_TICK_COUNT
    beq     r1, r11, .info_ticks
    move.l  r11, #SYSINFO_CURRENT_TASK
    beq     r1, r11, .info_current_task
    move.q  r1, #0
    move.q  r2, #ERR_OK
    eret
.info_total_pages:
    move.l  r1, #ALLOC_POOL_PAGES
    move.q  r2, #ERR_OK
    eret
.info_free_pages:
    jsr     count_free_pages        ; R1 = free count
    move.q  r2, #ERR_OK
    eret
.info_current_task:
    move.l  r11, #KERN_DATA_BASE
    load.q  r1, (r11)                  ; current_task
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

; --- CreatePort (M7) ---
; R1=name_ptr (0=anonymous), R2=flags → R1=portID (0-7), R2=err
.do_create_port:
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)              ; current_task
    ; Save inputs
    move.q  r7, r1                  ; R7 = name_ptr
    move.q  r8, r2                  ; R8 = flags

    ; --- Validation phase (no side effects) ---
    ; 1. Find free port slot
    move.l  r20, #0                 ; port index
    move.l  r21, #KERN_DATA_BASE
    add     r21, r21, #KD_PORT_BASE ; R21 = &port[0]
.create_scan:
    move.l  r22, #KD_PORT_MAX
    bge     r20, r22, .create_full
    load.b  r23, KD_PORT_VALID(r21)
    beqz    r23, .create_slot_found
    add     r21, r21, #KD_PORT_STRIDE
    add     r20, r20, #1
    bra     .create_scan
.create_slot_found:
    ; R20 = slot index, R21 = &port[slot]. Do NOT write valid=1 yet.
    ; 2. If name_ptr == 0, force-clear PF_PUBLIC and skip name validation
    ;    (anonymous ports are always private per the documented model)
    bnez    r7, .create_named
    move.l  r23, #PF_PUBLIC
    not     r23, r23
    and     r8, r8, r23             ; clear PF_PUBLIC from flags
    bra     .create_commit
.create_named:

    ; 3. Validate user pointer and copy name to scratch buffer
    move.q  r1, r7                  ; R1 = name_ptr for safe_copy_user_name
    push    r20
    push    r21
    push    r8
    jsr     safe_copy_user_name     ; R23 = 0 on success, ERR_BADARG on failure
    pop     r8
    pop     r21
    pop     r20
    bnez    r23, .create_badarg

    ; 4. If PF_PUBLIC, check name uniqueness
    and     r23, r8, #PF_PUBLIC
    beqz    r23, .create_commit
    push    r20
    push    r21
    push    r8
    jsr     check_name_unique       ; R23 = 0 if unique, 1 if dup
    pop     r8
    pop     r21
    pop     r20
    bnez    r23, .create_exists

    ; --- Commit phase (no failures possible) ---
.create_commit:
    move.b  r23, #1
    store.b r23, KD_PORT_VALID(r21)     ; valid = 1
    store.b r13, KD_PORT_OWNER(r21)     ; owner = current_task
    store.b r0, KD_PORT_COUNT(r21)      ; count = 0
    store.b r0, KD_PORT_HEAD(r21)       ; head = 0
    store.b r0, KD_PORT_TAIL(r21)       ; tail = 0
    store.b r8, KD_PORT_FLAGS(r21)      ; flags
    ; Zero name field
    store.q r0, KD_PORT_NAME(r21)
    add     r23, r21, #KD_PORT_NAME
    add     r23, r23, #8
    store.q r0, (r23)
    ; If named, copy from scratch buffer to port name
    beqz    r7, .create_done
    move.l  r14, #KERN_DATA_BASE
    add     r14, r14, #KD_NAME_SCRATCH  ; R14 = &scratch
    add     r16, r21, #KD_PORT_NAME     ; R16 = &port.name
    ; Copy 16 bytes (2 quad-words)
    load.q  r17, (r14)
    store.q r17, (r16)
    add     r14, r14, #8
    add     r16, r16, #8
    load.q  r17, (r14)
    store.q r17, (r16)
.create_done:
    move.q  r1, r20                      ; R1 = portID
    move.q  r2, #ERR_OK
    eret
.create_full:
    move.q  r2, #ERR_NOMEM
    eret
.create_badarg:
    move.q  r2, #ERR_BADARG
    eret
.create_exists:
    move.q  r2, #ERR_EXISTS
    eret

; --- FindPort (M7) ---
; R1=name_ptr → R1=portID, R2=err
.do_find_port:
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)              ; current_task
    ; Validate and copy user name to scratch buffer
    push    r1
    jsr     safe_copy_user_name     ; R23 = 0 on success
    pop     r1
    bnez    r23, .findport_badarg
    ; Scan all ports for public name match
    move.l  r19, #0                 ; port index
    move.l  r20, #KERN_DATA_BASE
    add     r20, r20, #KD_PORT_BASE
.findport_loop:
    move.l  r18, #KD_PORT_MAX
    bge     r19, r18, .findport_notfound
    load.b  r14, KD_PORT_VALID(r20)
    beqz    r14, .findport_next
    load.b  r14, KD_PORT_FLAGS(r20)
    and     r14, r14, #PF_PUBLIC
    beqz    r14, .findport_next
    ; Compare port name against scratch
    move.q  r24, r20
    add     r24, r24, #KD_PORT_NAME
    move.l  r25, #KERN_DATA_BASE
    add     r25, r25, #KD_NAME_SCRATCH
    push    r19
    push    r20
    jsr     case_insensitive_cmp    ; R23 = 0 if match
    pop     r20
    pop     r19
    beqz    r23, .findport_found
.findport_next:
    add     r20, r20, #KD_PORT_STRIDE
    add     r19, r19, #1
    bra     .findport_loop
.findport_found:
    move.q  r1, r19                 ; R1 = portID
    move.q  r2, #ERR_OK
    eret
.findport_notfound:
    move.q  r2, #ERR_NOTFOUND
    eret
.findport_badarg:
    move.q  r2, #ERR_BADARG
    eret

; --- ReplyMsg (M7) ---
; R1=reply_port_id, R2=msg_type, R3=data0, R4=data1, R5=share_handle → R2=err
; Implemented as PutMsg with share_handle moved to R6 and reply_port set to NONE
.do_reply_msg:
    move.q  r6, r5                  ; R6 = share_handle (was R5)
    move.l  r5, #REPLY_PORT_NONE    ; R5 = 0xFFFF (no reply-to-reply)
    ; Fall through to PutMsg

; --- PutMsg ---
; R1=portID, R2=msg_type, R3=data0, R4=data1, R5=reply_port, R6=share_handle → R2=err
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
    ; Write message fields (32 bytes)
    store.l r2, KD_MSG_TYPE(r26)        ; mn_Type = R2
    store.l r13, KD_MSG_SRC(r26)        ; mn_SrcTask = current_task
    store.q r3, KD_MSG_DATA0(r26)       ; mn_Data0 = R3
    store.q r4, KD_MSG_DATA1(r26)       ; mn_Data1 = R4
    store.w r5, KD_MSG_REPLY_PORT(r26)  ; mn_ReplyPort = R5
    store.l r6, KD_MSG_SHARE_HDL(r26)   ; mn_ShareHandle = R6
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
    move.q  r2, #ERR_FULL
    eret

; --- GetMsg ---
; R1=portID → R1=msg_type, R2=data0, R3=err, R4=data1, R5=reply_port, R6=share_handle
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
    load.q  r2, KD_MSG_DATA0(r27)       ; R2 = data0
    load.q  r4, KD_MSG_DATA1(r27)       ; R4 = data1
    load.w  r5, KD_MSG_REPLY_PORT(r27)  ; R5 = reply_port
    load.l  r6, KD_MSG_SHARE_HDL(r27)   ; R6 = share_handle
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
; R1=portID → R1=msg_type, R2=data0, R3=err, R4=data1, R5=reply_port, R6=share_handle
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
    ; Dequeue (same as GetMsg, 32-byte message)
    load.b  r26, KD_PORT_HEAD(r21)
    move.l  r27, #KD_MSG_SIZE
    mulu    r27, r26, r27
    add     r27, r27, r21
    add     r27, r27, #KD_PORT_MSGS
    load.l  r1, KD_MSG_TYPE(r27)
    load.q  r2, KD_MSG_DATA0(r27)
    load.q  r4, KD_MSG_DATA1(r27)
    load.w  r5, KD_MSG_REPLY_PORT(r27)
    load.l  r6, KD_MSG_SHARE_HDL(r27)
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
; AllocMem (SYS_ALLOC_MEM = 1)
; ============================================================================
; R1 = size (bytes), R2 = flags (MEMF_PUBLIC | MEMF_CLEAR, or 0)
; Returns: R1 = VA, R2 = err, R3 = share_handle (if MEMF_PUBLIC)

.do_alloc_mem:
    ; Save user args
    move.q  r24, r1                     ; r24 = size
    move.q  r25, r2                     ; r25 = flags

    ; Step 1: Validate size > 0
    beqz    r24, .am_badarg

    ; Step 2: Compute page count = (size + 0xFFF) >> 12
    ; Guard against overflow: if size > 0xFFFFF000, the add wraps and produces
    ; a small/zero page count. Reject sizes above 1 MiB (USER_DYN_PAGES * PAGE_SIZE).
    move.l  r11, #USER_DYN_PAGES
    lsl     r11, r11, #12               ; r11 = max bytes (256 * 4096 = 1 MiB)
    bgt     r24, r11, .am_toolarge      ; reject before add can overflow
    add     r20, r24, #0xFFF
    lsr     r20, r20, #12               ; r20 = page count (safe, no overflow possible)

    ; Step 3: Validate pages <= USER_DYN_PAGES (256) — redundant but defense-in-depth
    move.l  r11, #USER_DYN_PAGES
    bgt     r20, r11, .am_toolarge

    ; Step 4: If MEMF_PUBLIC, find free shared object slot (check only)
    move.q  r26, #0                     ; r26 = shmem slot (-1 = not public)
    and     r11, r25, #MEMF_PUBLIC
    beqz    r11, .am_skip_shmem_check
    move.l  r14, #KERN_DATA_BASE
    add     r14, r14, #KD_SHMEM_TABLE
    move.l  r15, #0
.am_find_shmem:
    move.l  r11, #KD_SHMEM_MAX
    bge     r15, r11, .am_nomem         ; no free shmem slot
    move.l  r11, #KD_SHMEM_STRIDE
    mulu    r11, r15, r11
    add     r16, r14, r11
    load.b  r11, KD_SHM_VALID(r16)
    beqz    r11, .am_found_shmem
    add     r15, r15, #1
    bra     .am_find_shmem
.am_found_shmem:
    move.q  r26, r16                    ; r26 = &shmem[slot]
    move.q  r27, r15                    ; r27 = slot index (save for handle)
.am_skip_shmem_check:

    ; Step 5: Find free region slot in caller's region table
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)                  ; r13 = current_task
    move.l  r14, #KERN_DATA_BASE
    add     r14, r14, #KD_REGION_TABLE
    move.l  r11, #KD_REGION_TASK_SZ
    mulu    r11, r13, r11
    add     r14, r14, r11               ; r14 = &region_table[task][0]
    move.l  r15, #0                     ; region slot index
.am_find_region:
    move.l  r11, #KD_REGION_MAX
    bge     r15, r11, .am_nomem         ; no free slot
    move.l  r11, #KD_REGION_STRIDE
    mulu    r11, r15, r11
    add     r16, r14, r11               ; r16 = &region[i]
    load.b  r11, KD_REG_TYPE(r16)
    bnez    r11, .am_find_region_next
    bra     .am_found_region
.am_find_region_next:
    add     r15, r15, #1
    bra     .am_find_region
.am_found_region:
    ; r15 = free slot index, r16 = &region[slot]
    ; Save region pointer for commit phase
    move.q  r21, r16                    ; r21 = &region[slot]

    ; Step 6: Allocate physical pages
    move.q  r1, r20                     ; R1 = page count
    jsr     alloc_pages                 ; R1 = base PPN, R2 = err
    bnez    r2, .am_nomem               ; alloc failed
    move.q  r22, r1                     ; r22 = base PPN

    ; Step 7: Find free VA gap
    move.q  r1, r13                     ; R1 = task_id
    move.q  r2, r20                     ; R2 = pages needed
    jsr     find_free_va                ; R1 = VA, R2 = err
    bnez    r2, .am_rollback_pages      ; VA search failed → free pages
    move.q  r23, r1                     ; r23 = VA

    ; === COMMIT PHASE (no failures past this point) ===

    ; Step 8: Map pages into caller's page table as P|R|W|U
    move.l  r12, #KERN_DATA_BASE
    ; Get caller's PTBR from the PTBR array
    lsl     r1, r13, #3                 ; task * 8
    add     r1, r1, #KD_PTBR_BASE
    add     r1, r1, r12
    load.q  r1, (r1)                    ; R1 = PT base
    move.q  r2, r23                     ; R2 = VA
    move.q  r3, r22                     ; R3 = PPN
    move.q  r4, r20                     ; R4 = page count
    move.l  r5, #0x17                   ; R5 = P|R|W|U (no X — W^X)
    jsr     map_pages

    ; Step 9: If MEMF_CLEAR, zero the pages
    lsr     r11, r25, #16               ; r11 = flags >> 16
    and     r11, r11, #1                ; bit 0 of upper half = MEMF_CLEAR
    beqz    r11, .am_skip_clear

    ; Switch to kernel PT for zeroing (pool pages only mapped in kernel PT)
    ; Use R28 for PTBR save (R27 holds shmem slot index, needed for MEMF_PUBLIC)
    mfcr    r28, cr0                    ; save current PTBR
    move.l  r1, #KERN_PAGE_TABLE
    mtcr    cr0, r1
    tlbflush

    ; Zero pages: base physical addr = PPN * 4096
    lsl     r1, r22, #12               ; r1 = physical address
    lsl     r2, r20, #12               ; r2 = total bytes
    add     r2, r2, r1                  ; r2 = end address
.am_zero:
    bge     r1, r2, .am_zero_done
    store.q r0, (r1)
    add     r1, r1, #8
    bra     .am_zero
.am_zero_done:
    ; Restore caller's PTBR
    mtcr    cr0, r28
    tlbflush
.am_skip_clear:

    ; Step 10: MEMF_PUBLIC → create shared object with nonce handle
    move.q  r3, #0                      ; share_handle = 0 (default for private)
    and     r11, r25, #MEMF_PUBLIC
    beqz    r11, .am_fill_region        ; not public → skip

    ; Generate nonce from monotonic counter (guarantees uniqueness even within same tick)
    move.l  r12, #KERN_DATA_BASE
    load.q  r11, KD_NONCE_COUNTER(r12)
    add     r11, r11, #1               ; increment counter
    store.q r11, KD_NONCE_COUNTER(r12) ; write back
    eor     r11, r11, r27              ; XOR slot index (spread across slots)
    and     r11, r11, #0xFFFFFF        ; mask to 24 bits

    ; Fill shared object entry
    move.b  r9, #1
    store.b r9, KD_SHM_VALID(r26)       ; valid = 1
    store.b r9, KD_SHM_REFCOUNT(r26)    ; refcount = 1
    store.b r13, KD_SHM_CREATOR(r26)    ; creator = current_task
    store.w r22, KD_SHM_PPN(r26)        ; base PPN
    store.w r20, KD_SHM_PAGES(r26)      ; page count
    store.l r11, KD_SHM_NONCE(r26)      ; nonce

    ; Build handle = (nonce << 8) | slot
    lsl     r3, r11, #8
    or      r3, r3, r27                 ; R3 = share_handle

.am_fill_region:
    ; Step 11: Fill region table entry
    store.l r23, KD_REG_VA(r21)         ; base VA
    store.w r22, KD_REG_PPN(r21)        ; base PPN
    store.w r20, KD_REG_PAGES(r21)      ; page count
    ; Set type based on MEMF_PUBLIC
    and     r11, r25, #MEMF_PUBLIC
    bnez    r11, .am_type_shared
    move.b  r11, #REGION_PRIVATE
    bra     .am_store_type
.am_type_shared:
    move.b  r11, #REGION_SHARED
    store.b r27, KD_REG_SHMID(r21)      ; shmem slot index
.am_store_type:
    store.b r11, KD_REG_TYPE(r21)
    store.b r25, KD_REG_FLAGS(r21)      ; flags (low byte)

    ; Step 12: TLB flush
    tlbflush

    ; Return VA in R1, ERR_OK in R2, share_handle in R3
    move.q  r1, r23
    move.q  r2, #ERR_OK
    eret

.am_rollback_pages:
    move.q  r1, r22                     ; base PPN
    move.q  r2, r20                     ; page count
    jsr     free_pages
    bra     .am_nomem

.am_badarg:
    move.q  r1, #0
    move.q  r2, #ERR_BADARG
    move.q  r3, #0
    eret

.am_toolarge:
    move.q  r1, #0
    move.q  r2, #ERR_TOOLARGE
    move.q  r3, #0
    eret

.am_nomem:
    move.q  r1, #0
    move.q  r2, #ERR_NOMEM
    move.q  r3, #0
    eret

; ============================================================================
; FreeMem (SYS_FREE_MEM = 2)
; ============================================================================
; R1 = addr (VA), R2 = size (bytes, must match allocation)
; Returns: R2 = err

.do_free_mem:
    move.q  r24, r1                     ; r24 = addr
    move.q  r25, r2                     ; r25 = size

    ; Compute expected page count
    add     r20, r25, #0xFFF
    lsr     r20, r20, #12               ; r20 = page count from size

    ; Find matching region in caller's table
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)                  ; current_task
    move.l  r14, #KERN_DATA_BASE
    add     r14, r14, #KD_REGION_TABLE
    move.l  r11, #KD_REGION_TASK_SZ
    mulu    r11, r13, r11
    add     r14, r14, r11               ; r14 = &region_table[task][0]

    move.l  r15, #0                     ; region index
.fm_find:
    move.l  r11, #KD_REGION_MAX
    bge     r15, r11, .fm_badarg        ; not found
    move.l  r11, #KD_REGION_STRIDE
    mulu    r11, r15, r11
    add     r16, r14, r11               ; r16 = &region[i]
    load.b  r11, KD_REG_TYPE(r16)
    beqz    r11, .fm_find_next          ; skip FREE slots
    ; Check if VA matches
    load.l  r17, KD_REG_VA(r16)
    beq     r17, r24, .fm_found
.fm_find_next:
    add     r15, r15, #1
    bra     .fm_find

.fm_found:
    ; Validate size matches
    load.w  r18, KD_REG_PAGES(r16)      ; stored page count
    bne     r18, r20, .fm_badarg        ; size mismatch

    ; Unmap from caller's page table
    lsl     r1, r13, #3
    add     r1, r1, #KD_PTBR_BASE
    move.l  r11, #KERN_DATA_BASE
    add     r1, r1, r11
    load.q  r1, (r1)                    ; R1 = PT base
    move.q  r2, r24                     ; R2 = VA
    move.q  r3, r18                     ; R3 = page count
    jsr     unmap_pages

    ; Check region type
    load.b  r11, KD_REG_TYPE(r16)
    move.l  r17, #REGION_SHARED
    beq     r11, r17, .fm_shared

    ; PRIVATE: free physical pages
    load.w  r1, KD_REG_PPN(r16)         ; base PPN
    move.q  r2, r18                     ; page count
    jsr     free_pages
    bra     .fm_cleanup_region

.fm_shared:
    ; SHARED: decrement refcount, maybe free backing pages
    load.b  r19, KD_REG_SHMID(r16)     ; slot index
    move.l  r11, #KD_SHMEM_STRIDE
    mulu    r11, r19, r11
    move.l  r17, #KERN_DATA_BASE
    add     r17, r17, #KD_SHMEM_TABLE
    add     r17, r17, r11               ; r17 = &shmem[slot]

    load.b  r11, KD_SHM_REFCOUNT(r17)
    sub     r11, r11, #1
    store.b r11, KD_SHM_REFCOUNT(r17)
    bnez    r11, .fm_cleanup_region     ; refcount > 0, object survives

    ; Refcount reached 0: free backing pages and invalidate object
    load.w  r1, KD_SHM_PPN(r17)
    load.w  r2, KD_SHM_PAGES(r17)
    jsr     free_pages
    store.b r0, KD_SHM_VALID(r17)       ; invalidate

.fm_cleanup_region:
    ; Mark region as FREE
    store.b r0, KD_REG_TYPE(r16)
    tlbflush
    move.q  r2, #ERR_OK
    eret

.fm_badarg:
    move.q  r2, #ERR_BADARG
    eret

; ============================================================================
; MapShared (SYS_MAP_SHARED = 4)
; ============================================================================
; R1 = share_handle (opaque 32-bit: nonce<<8 | slot)
; Returns: R1 = VA, R2 = err

.do_map_shared:
    move.q  r24, r1                     ; r24 = handle

    ; Decode handle: slot = handle & 0xFF, nonce = (handle >> 8) & 0xFFFFFF
    and     r20, r24, #0xFF             ; r20 = slot
    lsr     r21, r24, #8               ; r21 = nonce (upper 24 bits)

    ; Validate slot < KD_SHMEM_MAX
    move.l  r11, #KD_SHMEM_MAX
    bge     r20, r11, .ms_badhandle

    ; Look up shared object
    move.l  r14, #KERN_DATA_BASE
    add     r14, r14, #KD_SHMEM_TABLE
    move.l  r11, #KD_SHMEM_STRIDE
    mulu    r11, r20, r11
    add     r14, r14, r11               ; r14 = &shmem[slot]

    ; Check valid
    load.b  r11, KD_SHM_VALID(r14)
    beqz    r11, .ms_badhandle

    ; Check nonce matches
    load.l  r11, KD_SHM_NONCE(r14)
    and     r11, r11, #0xFFFFFF         ; mask to 24 bits
    bne     r11, r21, .ms_badhandle

    ; Find free region slot in caller's table
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)                  ; current_task
    move.l  r15, #KERN_DATA_BASE
    add     r15, r15, #KD_REGION_TABLE
    move.l  r11, #KD_REGION_TASK_SZ
    mulu    r11, r13, r11
    add     r15, r15, r11               ; r15 = &region_table[task][0]
    move.l  r16, #0
.ms_find_region:
    move.l  r11, #KD_REGION_MAX
    bge     r16, r11, .ms_nomem
    move.l  r11, #KD_REGION_STRIDE
    mulu    r11, r16, r11
    add     r17, r15, r11
    load.b  r11, KD_REG_TYPE(r17)
    beqz    r11, .ms_found_region
    add     r16, r16, #1
    bra     .ms_find_region
.ms_found_region:
    ; r17 = &region[free_slot]
    move.q  r22, r17                    ; save for commit

    ; Save values that find_free_va clobbers (R3-R16)
    move.q  r28, r14                    ; r28 = shmem ptr (save R14)
    move.q  r29, r13                    ; r29 = current_task (save R13)

    ; Find free VA gap
    move.q  r1, r13                     ; task_id
    load.w  r2, KD_SHM_PAGES(r14)      ; pages from shared object
    move.q  r23, r2                     ; save page count
    jsr     find_free_va
    bnez    r2, .ms_nomem
    move.q  r25, r1                     ; r25 = VA

    ; Restore saved values
    move.q  r14, r28                    ; restore shmem ptr
    move.q  r13, r29                    ; restore current_task

    ; === COMMIT ===

    ; Map shared pages into caller's PT
    lsl     r1, r13, #3
    add     r1, r1, #KD_PTBR_BASE
    move.l  r11, #KERN_DATA_BASE
    add     r1, r1, r11
    load.q  r1, (r1)                    ; R1 = PT base
    move.q  r2, r25                     ; R2 = VA
    load.w  r3, KD_SHM_PPN(r14)        ; R3 = PPN from shared object
    move.q  r4, r23                     ; R4 = page count
    move.l  r5, #0x17                   ; R5 = P|R|W|U
    jsr     map_pages

    ; Increment refcount
    load.b  r11, KD_SHM_REFCOUNT(r14)
    add     r11, r11, #1
    store.b r11, KD_SHM_REFCOUNT(r14)

    ; Fill region entry
    store.l r25, KD_REG_VA(r22)
    load.w  r11, KD_SHM_PPN(r14)
    store.w r11, KD_REG_PPN(r22)
    store.w r23, KD_REG_PAGES(r22)
    move.b  r11, #REGION_SHARED
    store.b r11, KD_REG_TYPE(r22)
    store.b r20, KD_REG_SHMID(r22)      ; slot index
    move.b  r11, #MEMF_PUBLIC
    store.b r11, KD_REG_FLAGS(r22)

    tlbflush
    move.q  r1, r25                     ; VA
    move.q  r2, #ERR_OK
    eret

.ms_badhandle:
    move.q  r1, #0
    move.q  r2, #ERR_BADHANDLE
    eret

.ms_nomem:
    move.q  r1, #0
    move.q  r2, #ERR_NOMEM
    eret

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
    store.b r0, KD_PORT_FLAGS(r20)       ; M7: clear flags (removes from FindPort)
    store.q r0, KD_PORT_NAME(r20)        ; M7: clear name (bytes 0-7)
    add     r22, r20, #KD_PORT_NAME
    add     r22, r22, #8
    store.q r0, (r22)                    ; M7: clear name (bytes 8-15)
.ktc_port_next:
    add     r20, r20, #KD_PORT_STRIDE
    add     r21, r21, #1
    bra     .ktc_port_scan
.ktc_done:
    ; M6: Clean up memory regions for this task
    move.l  r20, #KERN_DATA_BASE
    add     r20, r20, #KD_REGION_TABLE
    move.l  r21, #KD_REGION_TASK_SZ
    mulu    r21, r13, r21
    add     r20, r20, r21               ; r20 = &region_table[task][0]
    move.l  r21, #0                     ; region index
.ktc_region_scan:
    move.l  r22, #KD_REGION_MAX
    bge     r21, r22, .ktc_mem_done
    move.l  r22, #KD_REGION_STRIDE
    mulu    r22, r21, r22
    add     r23, r20, r22               ; r23 = &region[i]
    load.b  r22, KD_REG_TYPE(r23)
    beqz    r22, .ktc_region_next       ; skip FREE

    ; Unmap from task's page table
    ; Need PTBR for this task
    lsl     r1, r13, #3
    add     r1, r1, #KD_PTBR_BASE
    add     r1, r1, r12
    load.q  r1, (r1)                    ; PT base
    load.l  r2, KD_REG_VA(r23)          ; VA
    load.w  r3, KD_REG_PAGES(r23)       ; page count
    jsr     unmap_pages

    ; Check type for page freeing
    load.b  r22, KD_REG_TYPE(r23)
    move.l  r24, #REGION_PRIVATE
    bne     r22, r24, .ktc_region_shared

    ; PRIVATE: free physical pages
    load.w  r1, KD_REG_PPN(r23)
    load.w  r2, KD_REG_PAGES(r23)
    jsr     free_pages
    bra     .ktc_region_clear

.ktc_region_shared:
    ; SHARED: decrement refcount
    load.b  r24, KD_REG_SHMID(r23)
    move.l  r22, #KD_SHMEM_STRIDE
    mulu    r22, r24, r22
    move.l  r1, #KERN_DATA_BASE
    add     r1, r1, #KD_SHMEM_TABLE
    add     r1, r1, r22                 ; r1 = &shmem[slot]
    load.b  r22, KD_SHM_REFCOUNT(r1)
    sub     r22, r22, #1
    store.b r22, KD_SHM_REFCOUNT(r1)
    bnez    r22, .ktc_region_clear      ; refcount > 0

    ; Last reference: free backing pages and invalidate
    load.w  r2, KD_SHM_PAGES(r1)
    load.w  r1, KD_SHM_PPN(r1)         ; NOTE: clobbers r1, but we saved shmem addr... need to re-derive
    ; Actually r1 was the shmem addr, load PPN first
    ; Let me fix: save shmem ptr
    ; Re-derive: r24 = slot, compute shmem addr again
    move.l  r22, #KD_SHMEM_STRIDE
    mulu    r22, r24, r22
    move.l  r1, #KERN_DATA_BASE
    add     r1, r1, #KD_SHMEM_TABLE
    add     r1, r1, r22                 ; r1 = &shmem[slot] again
    load.w  r2, KD_SHM_PAGES(r1)       ; page count (save first)
    move.q  r3, r2                      ; r3 = pages
    load.w  r2, KD_SHM_PPN(r1)         ; PPN
    move.q  r4, r1                      ; r4 = shmem ptr (save)
    move.q  r1, r2                      ; R1 = PPN
    move.q  r2, r3                      ; R2 = pages
    jsr     free_pages
    store.b r0, KD_SHM_VALID(r4)        ; invalidate

.ktc_region_clear:
    store.b r0, KD_REG_TYPE(r23)        ; mark region FREE

.ktc_region_next:
    add     r21, r21, #1
    bra     .ktc_region_scan

.ktc_mem_done:
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

    ; Message available — dequeue (32-byte message)
    load.b  r26, KD_PORT_HEAD(r21)
    move.l  r27, #KD_MSG_SIZE
    mulu    r27, r26, r27
    add     r27, r27, r21
    add     r27, r27, #KD_PORT_MSGS
    load.l  r1, KD_MSG_TYPE(r27)
    load.q  r2, KD_MSG_DATA0(r27)
    load.q  r4, KD_MSG_DATA1(r27)
    load.w  r5, KD_MSG_REPLY_PORT(r27)
    load.l  r6, KD_MSG_SHARE_HDL(r27)
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

; Task 0 (M6 demo): AllocMem(MEMF_PUBLIC|MEMF_CLEAR), write 'S' to shared page,
; CreateTask with share_handle as arg0, print 'A' + yield loop.
; Child code is pre-loaded into task 0's data page during boot init.
; 12 instructions = 96 bytes. 'A' marker at instruction 5 (byte 40).
; Task 0 (M7 demo): ECHO service with shared-memory reply.
; Creates named "ECHO" port, allocates MEMF_PUBLIC shared memory, writes "HI",
; waits for request, replies with share_handle. Proves named ports + request/reply
; + share-handle-in-message handoff.
; 24 instructions = 192 bytes.
user_task0_template:
    ; Write "ECHO\0" to data page at offset 128
    move.l  r7, #USER_DATA_BASE
    add     r7, r7, #128            ; 0-1: R7 = &data[128]
    move.l  r8, #0x4F484345         ; 2: "ECHO" in little-endian
    store.l r8, (r7)                ; 3: write "ECHO"
    store.b r0, 4(r7)               ; 4: NUL terminator
    move.l  r1, #0x41               ; 5: 'A' ← findTaskTemplates marker
    ; CreatePort("ECHO", PF_PUBLIC)
    move.l  r1, r7                  ; 6: R1 = name_ptr
    move.l  r2, #PF_PUBLIC          ; 7: R2 = flags
    syscall #SYS_CREATE_PORT        ; 8: → R1=portID (always 0, first port)
    ; AllocMem(4096, MEMF_PUBLIC|MEMF_CLEAR)
    move.l  r1, #0x1000             ; 9: size = 4096
    move.l  r2, #0x10001            ; 10: MEMF_PUBLIC | MEMF_CLEAR
    syscall #SYS_ALLOC_MEM          ; 11: → R1=VA, R3=share_handle
    ; Write "HI" to shared memory (MEMF_CLEAR ensures NUL at +2)
    move.l  r8, #0x4948             ; 12: "HI" in LE
    store.w r8, (r1)                ; 13: write "HI"
    ; Save share_handle to data page (survives WaitPort context switch)
    move.l  r8, #USER_DATA_BASE     ; 14
    store.q r3, 256(r8)             ; 15: data[256] = share_handle
    ; WaitPort(port 0) → receives request with R5=reply_port
    move.q  r1, r0                  ; 16: R1 = 0 (echo_port is always port 0)
    syscall #SYS_WAIT_PORT          ; 17: blocks → R5=reply_port
    ; ReplyMsg(reply_port, type='!', share_handle from data page)
    move.q  r1, r5                  ; 18: R1 = reply_port
    move.l  r2, #0x21               ; 19: type = '!'
    move.l  r8, #USER_DATA_BASE     ; 20
    load.q  r5, 256(r8)             ; 21: R5 = share_handle
    syscall #SYS_REPLY_MSG          ; 22: reply with handle
    syscall #SYS_EXIT_TASK          ; 23: done
user_task0_template_end:

; Task 1 (M7 demo): ECHO client with shared-memory read.
; Finds "ECHO" port, sends request with reply port, receives reply with
; share_handle, maps shared memory, reads and prints greeting string.
; Proves FindPort + request/reply + MapShared from message handle.
; 24 instructions = 192 bytes.
user_task1_template:
    ; Write "ECHO\0" to data page at offset 128 (task 1 data = $612000)
    move.l  r7, #0x612080              ; 0: R7 = task1_data + 128
    move.l  r8, #0x4F484345            ; 1: "ECHO"
    store.l r8, (r7)                   ; 2: write name
    store.b r0, 4(r7)                  ; 3: NUL
    ; CreatePort(anonymous) → reply port
    move.q  r1, r0                     ; 4: R1=0 (anonymous)
    move.q  r2, r0                     ; 5: R2=0 (no flags)
    syscall #SYS_CREATE_PORT           ; 6: → R1=reply_portID
    move.q  r9, r1                     ; 7: save reply_port in R9
    ; FindPort("ECHO")
    move.q  r1, r7                     ; 8: R1 = name_ptr
    syscall #SYS_FIND_PORT             ; 9: → R1=echo_portID
    ; PutMsg(echo_port, type='Q', reply_port=R9)
    move.l  r2, #0x51                  ; 10: type = 'Q' (R1 = echo_port from FindPort)
    move.q  r5, r9                     ; 11: R5 = reply_port
    move.q  r6, r0                     ; 12: R6 = 0 (no share_handle outbound)
    syscall #SYS_PUT_MSG               ; 13: send request
    ; WaitPort(reply_port) → R6=share_handle from reply
    move.q  r1, r9                     ; 14: R1 = reply_port
    syscall #SYS_WAIT_PORT             ; 15: → R6=share_handle
    ; MapShared(share_handle) → R1=mapped_VA
    move.q  r1, r6                     ; 16: R1 = share_handle from reply
    syscall #SYS_MAP_SHARED            ; 17: → R1=mapped_VA
    ; Read first byte from shared memory and print it
    load.b  r1, (r1)                   ; 18: R1 = 'H' (from "HI" in shared mem)
    syscall #SYS_DEBUG_PUTCHAR         ; 19: print 'H'
    ; Idle loop (keeps a runnable task so the kernel doesn't deadlock)
.t1_idle:
    syscall #SYS_YIELD                 ; 20
    bra     .t1_idle                   ; 21
    ; Pad to 24 instructions
    move.q  r0, r0                     ; 22-23: nop
    move.q  r0, r0
user_task1_template_end:

; Child task code (copied to task 0's data page during boot init).
; CreateTask copies this from the data page to the child's code page.
; 11 instructions = 88 bytes. Maps shared memory via arg0 handle, reads 'S', prints it, exits.
child_task_template:
    ; Get own task ID to compute data page VA
    move.l  r1, #SYSINFO_CURRENT_TASK  ; 0: info_id = 3
    syscall #SYS_GET_SYS_INFO         ; 1: R1 = task_id
    ; Compute data page VA = USER_DATA_BASE + task_id * USER_SLOT_STRIDE
    move.l  r5, #USER_SLOT_STRIDE     ; 2
    mulu    r5, r1, r5                ; 3: R5 = task_id * stride
    move.l  r6, #USER_DATA_BASE       ; 4
    add     r5, r5, r6                ; 5: R5 = data page VA
    ; Load arg0 (share_handle) from data page offset 0
    load.q  r1, (r5)                  ; 6: R1 = share_handle
    ; MapShared(handle) → R1 = mapped VA
    syscall #SYS_MAP_SHARED           ; 7
    ; Read 'S' from shared memory and print it
    load.b  r1, (r1)                  ; 8: R1 = [mapped VA] = 'S'
    syscall #SYS_DEBUG_PUTCHAR        ; 9: print 'S'
    syscall #SYS_EXIT_TASK            ; 10: exit cleanly
child_task_template_end:

; ============================================================================
; Data: Strings
; ============================================================================

boot_banner:
    dc.b    "IExec M7 boot", 0x0A, 0
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
; End of IExec M6
; ============================================================================
