; ============================================================================
; exec.library - IE64 Microkernel Nucleus (M9: dos.library + RAM: + Shell)
; ============================================================================
;
; Amiga Exec-inspired protected microkernel for the IE64 CPU.
; M9: dos.library, RAM: filesystem, interactive shell, external commands.
;     OpenLibrary discovery, console.handler CON:, all commands as user tasks.
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
    ; 4. Initialize kernel data (MMU still off)
    ; ---------------------------------------------------------------
    move.l  r12, #KERN_DATA_BASE
    store.q r0, (r12)                   ; current_task = 0
    store.q r0, KD_TICK_COUNT(r12)
    store.q r0, KD_NUM_TASKS(r12)       ; 0 tasks (loader increments)
    store.q r0, KD_NONCE_COUNTER(r12)   ; nonce counter = 0

    ; --- Init all 8 task slots as FREE ---
    move.l  r4, #0
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
    store.b r0, KD_TASK_GPR_SAVED(r5)   ; no GPRs on stack initially
    add     r4, r4, #1
    bra     .init_free_tasks
.init_free_done:

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
    ; 9. Load boot programs from program_table (M9: strict for first N)
    ; ---------------------------------------------------------------
    la      r30, program_table          ; r30 = table cursor
    move.l  r29, #0                     ; boot entry counter
.boot_load_loop:
    move.l  r11, #PROGTAB_BOOT_COUNT
    bge     r29, r11, .boot_load_done   ; loaded all required boot programs
    load.q  r7, PROGTAB_OFF_PTR(r30)   ; r7 = image_ptr
    beqz    r7, .boot_load_fail         ; sentinel before boot count → fail
    load.q  r8, PROGTAB_OFF_SIZE(r30)   ; r8 = image_size
    push    r30                         ; save table cursor (load_program clobbers R14-R27)
    push    r29                         ; save counter
    jsr     load_program                ; → R1=task_id, R2=err
    pop     r29                         ; restore counter
    pop     r30                         ; restore cursor
    bnez    r2, .boot_load_fail         ; strict: any failure → panic
    add     r30, r30, #PROGTAB_ENTRY_SIZE
    add     r29, r29, #1
    bra     .boot_load_loop
.boot_load_fail:
    la      r8, boot_fail_msg
    jsr     kern_puts
    halt
.boot_load_done:

    ; ---------------------------------------------------------------
    ; 9b. Enable terminal line input mode
    ; ---------------------------------------------------------------
    la      r28, TERM_CTRL
    move.l  r1, #1
    store.l r1, (r28)                  ; line mode = 1

    ; ---------------------------------------------------------------
    ; 10. Program timer
    ; ---------------------------------------------------------------
    move.l  r1, #10000
    mtcr    cr9, r1
    move.l  r1, #10000
    mtcr    cr10, r1
    move.q  r1, #3
    mtcr    cr11, r1

    ; ---------------------------------------------------------------
    ; 11. Enter first user task (task 0 = first loaded program)
    ; ---------------------------------------------------------------
    move.l  r12, #KERN_DATA_BASE
    load.q  r1, KD_NUM_TASKS(r12)
    beqz    r1, .boot_no_tasks          ; no programs loaded → panic

    move.l  r15, #KERN_DATA_BASE
    add     r15, r15, #KD_TASK_BASE     ; &TCB[0]
    load.q  r1, KD_TASK_USP(r15)
    mtcr    cr12, r1
    load.q  r1, KD_TASK_PC(r15)
    mtcr    cr3, r1
    load.q  r1, KD_PTBR_BASE(r12)
    mtcr    cr0, r1
    tlbflush
    eret

.boot_no_tasks:
    la      r8, no_tasks_msg
    jsr     kern_puts
    halt

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
; Program Loader (M8: boot-time only, not a syscall)
; ============================================================================
; load_program: Load a bundled IE64 program image into a free task slot.
; Input:  R7 = image_ptr (address of image in kernel memory)
;         R8 = image_size (total bytes: header + code + data)
; Output: R1 = task_id, R2 = ERR_OK
;         On failure: R1 = 0, R2 = ERR_BADARG or ERR_NOMEM
; Clobbers: R1-R9, R14-R27
; Must be called with kernel PT active (boot context).

load_program:
    push    r7
    push    r8

    ; --- Validation (no side effects) ---

    ; 1. Check image_size >= IMG_HEADER_SIZE (32)
    move.l  r11, #IMG_HEADER_SIZE
    blt     r8, r11, .lp_badarg

    ; 2. Check magic (8 bytes at image_ptr)
    load.l  r14, (r7)                   ; low 32 bits
    move.l  r15, #IMG_MAGIC_LO
    bne     r14, r15, .lp_badarg
    load.l  r14, 4(r7)                  ; high 32 bits
    move.l  r15, #IMG_MAGIC_HI
    bne     r14, r15, .lp_badarg

    ; 3. Load code_size, validate > 0, <= 4096, 8-byte aligned
    load.l  r20, IMG_OFF_CODE_SIZE(r7)  ; r20 = code_size
    beqz    r20, .lp_badarg
    move.l  r11, #MMU_PAGE_SIZE
    bgt     r20, r11, .lp_badarg
    and     r14, r20, #7                ; check 8-byte alignment
    bnez    r14, .lp_badarg

    ; 4. Load data_size, validate <= 4096
    load.l  r21, IMG_OFF_DATA_SIZE(r7)  ; r21 = data_size
    bgt     r21, r11, .lp_badarg

    ; 5. Check image_size >= 32 + code_size + data_size
    move.l  r14, #IMG_HEADER_SIZE
    add     r14, r14, r20
    add     r14, r14, r21               ; r14 = required size
    bgt     r14, r8, .lp_badarg         ; truncated image

    ; 6. Find free TCB slot
    move.l  r22, #0                     ; r22 = candidate slot
    move.l  r12, #KERN_DATA_BASE
.lp_scan:
    move.l  r11, #MAX_TASKS
    bge     r22, r11, .lp_nomem
    lsl     r15, r22, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12
    load.b  r16, KD_TASK_STATE(r15)
    move.l  r11, #TASK_FREE
    beq     r16, r11, .lp_slot_found
    add     r22, r22, #1
    bra     .lp_scan
.lp_slot_found:
    ; r22 = task_id, r15 = &TCB[task_id], r20 = code_size, r21 = data_size

    ; --- Commit (no failures past this point) ---

    ; 7. Already on kernel PT (boot context), no switch needed.

    ; 8. Zero child's code page (4096 bytes)
    move.l  r11, #USER_SLOT_STRIDE
    mulu    r11, r22, r11               ; r11 = task_id * stride
    move.l  r23, #USER_CODE_BASE
    add     r23, r23, r11               ; r23 = child code page addr
    move.l  r4, #0
    move.l  r6, #MMU_PAGE_SIZE
.lp_zero_code:
    bge     r4, r6, .lp_zero_code_done
    add     r5, r23, r4
    store.q r0, (r5)
    add     r4, r4, #8
    bra     .lp_zero_code
.lp_zero_code_done:

    ; 9. Copy code_size bytes from image_ptr+32 to code page
    pop     r8                          ; restore image_size (needed later)
    pop     r7                          ; restore image_ptr
    push    r7                          ; re-save for later
    push    r8
    add     r14, r7, #IMG_HEADER_SIZE   ; r14 = code source addr
    move.l  r4, #0
.lp_copy_code:
    bge     r4, r20, .lp_copy_code_done
    add     r5, r14, r4
    load.q  r6, (r5)
    add     r5, r23, r4
    store.q r6, (r5)
    add     r4, r4, #8
    bra     .lp_copy_code
.lp_copy_code_done:

    ; 10. If data_size > 0: zero data page, copy data
    beqz    r21, .lp_skip_data

    ; Zero data page
    move.l  r11, #USER_SLOT_STRIDE
    mulu    r11, r22, r11
    move.l  r24, #USER_DATA_BASE
    add     r24, r24, r11               ; r24 = child data page addr
    move.l  r4, #0
    move.l  r6, #MMU_PAGE_SIZE
.lp_zero_data:
    bge     r4, r6, .lp_zero_data_done
    add     r5, r24, r4
    store.q r0, (r5)
    add     r4, r4, #8
    bra     .lp_zero_data
.lp_zero_data_done:

    ; Copy data_size bytes from image_ptr+32+code_size to data page (byte-by-byte,
    ; because data_size is not required to be 8-byte aligned)
    add     r14, r7, #IMG_HEADER_SIZE
    add     r14, r14, r20               ; r14 = data source addr
    move.l  r4, #0
.lp_copy_data:
    bge     r4, r21, .lp_copy_data_done
    add     r5, r14, r4
    load.b  r6, (r5)
    add     r5, r24, r4
    store.b r6, (r5)
    add     r4, r4, #1
    bra     .lp_copy_data
.lp_copy_data_done:

.lp_skip_data:
    ; 11. Build child's page table
    move.q  r10, r22                    ; r10 = task_id
    jsr     build_user_pt

    ; 12. Initialize TCB
    move.l  r12, #KERN_DATA_BASE
    lsl     r15, r22, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12               ; r15 = &TCB[task_id]

    ; PC = USER_CODE_BASE + task_id * stride
    move.l  r11, #USER_SLOT_STRIDE
    mulu    r11, r22, r11
    move.l  r14, #USER_CODE_BASE
    add     r14, r14, r11
    store.q r14, KD_TASK_PC(r15)

    ; USP = USER_STACK_BASE + task_id * stride + PAGE_SIZE
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

    ; WaitPort = NONE
    move.b  r14, #WAITPORT_NONE
    store.b r14, KD_TASK_WAITPORT(r15)
    store.b r0, KD_TASK_GPR_SAVED(r15)  ; no GPRs saved initially

    ; 13. Set PTBR[task_id]
    move.l  r11, #USER_SLOT_STRIDE
    mulu    r11, r22, r11
    move.l  r14, #USER_PT_BASE
    add     r14, r14, r11               ; r14 = task PT base
    lsl     r16, r22, #3
    add     r16, r16, #KD_PTBR_BASE
    add     r16, r16, r12
    store.q r14, (r16)

    ; 14. Increment num_tasks
    load.q  r14, KD_NUM_TASKS(r12)
    add     r14, r14, #1
    store.q r14, KD_NUM_TASKS(r12)

    ; 15. Return success
    pop     r8
    pop     r7
    move.q  r1, r22                     ; R1 = task_id
    move.l  r2, #ERR_OK
    rts

.lp_badarg:
    pop     r8
    pop     r7
    move.q  r1, r0
    move.l  r2, #ERR_BADARG
    rts

.lp_nomem:
    pop     r8
    pop     r7
    move.q  r1, r0
    move.l  r2, #ERR_NOMEM
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
    move.q  r8, #0x0D
    jsr     kern_put_char
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
    move.q  r8, #0x0D
    jsr     kern_put_char
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

    move.l  r11, #SYS_EXEC_PROGRAM
    beq     r10, r11, .do_exec_program

    move.l  r11, #SYS_OPEN_LIBRARY
    beq     r10, r11, .do_open_library

    move.l  r11, #SYS_MAP_IO
    beq     r10, r11, .do_map_io

    move.l  r11, #SYS_READ_INPUT
    beq     r10, r11, .do_read_input

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
    store.b r0, KD_TASK_GPR_SAVED(r15)  ; M9: clear stale GPR frame flag

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
    store.b r0, KD_TASK_GPR_SAVED(r15)  ; M9: clear stale GPR frame flag

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
    store.b r0, KD_TASK_GPR_SAVED(r15)  ; M9: clear stale GPR frame flag
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
    ; NOTE: find_free_va clobbers R3-R16, so save R13 (current_task)
    push    r13
    move.q  r1, r13                     ; R1 = task_id
    move.q  r2, r20                     ; R2 = pages needed
    jsr     find_free_va                ; R1 = VA, R2 = err
    pop     r13                         ; restore current_task
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
; SYS_READ_INPUT (M9) — Read line from terminal input buffer
; ============================================================================
; R1 = destination buffer (user VA), R2 = max_len
; Returns: R1 = bytes_read, R2 = ERR_OK (line ready) or ERR_AGAIN (no line)
; Reads directly from TERM_IN/TERM_LINE_STATUS (kernel-mode I/O access).
; This bypasses the MMU/MAP_IO path which has dispatch issues.
.do_read_input:
    move.q  r20, r1                     ; r20 = user buffer ptr
    move.q  r21, r2                     ; r21 = max_len

    ; Validate buffer ptr is in user address space
    move.l  r11, #0x600000              ; USER_CODE_BASE
    blt     r20, r11, .ri_badarg        ; below user space

    ; Check TERM_LINE_STATUS — direct read from physical I/O register
    la      r28, TERM_LINE_STATUS       ; r28 = 0xF070C
    load.l  r14, (r28)
    and     r14, r14, #1
    beqz    r14, .ri_no_line            ; no complete line

    ; Line is ready — read chars from TERM_IN into user buffer
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)                  ; current_task
    move.l  r22, #0                     ; byte count
    la      r23, TERM_IN                ; TERM_IN physical addr (0xF0708)
    la      r24, TERM_STATUS            ; TERM_STATUS physical addr (0xF0704)

.ri_read_loop:
    ; Check max_len
    bge     r22, r21, .ri_done

    ; Check TERM_STATUS bit 0 = data available
    load.l  r14, (r24)
    and     r14, r14, #1
    beqz    r14, .ri_done               ; no more chars

    ; Read char from TERM_IN
    load.l  r14, (r23)                  ; dequeue char

    ; Skip \r
    move.l  r15, #0x0D
    beq     r14, r15, .ri_read_loop

    ; Check for \n = end of line
    move.l  r15, #0x0A
    beq     r14, r15, .ri_done

    ; Write char to user buffer (using user PT for addressing)
    add     r15, r20, r22              ; dest addr = user_buf + count
    store.b r14, (r15)                  ; write to user space
    add     r22, r22, #1
    bra     .ri_read_loop

.ri_done:
    ; Null-terminate
    add     r15, r20, r22
    store.b r0, (r15)
    move.q  r1, r22                     ; R1 = bytes_read
    move.q  r2, #ERR_OK
    eret

.ri_no_line:
    move.q  r1, #0
    move.q  r2, #ERR_AGAIN
    eret

.ri_badarg:
    move.q  r1, #0
    move.q  r2, #ERR_BADARG
    eret

; ============================================================================
; SYS_OPEN_LIBRARY (M9) — AmigaOS-style OpenLibrary
; ============================================================================
; R1 = name_ptr, R2 = version (ignored in M9)
; Returns: R1 = library handle (port ID), R2 = error
; Implementation: identical to FindPort (searches public ports by name).
.do_open_library:
    bra     .do_find_port               ; same logic, version in R2 ignored

; ============================================================================
; SYS_MAP_IO (M9) — Map I/O page into user space
; ============================================================================
; R1 = physical page number (only 0xF0 allowed for M9)
; Returns: R1 = mapped VA, R2 = error
.do_map_io:
    move.q  r24, r1                     ; r24 = requested physical page

    ; Validate: only terminal I/O page (0xF0) allowed for M9
    move.l  r11, #TERM_IO_PAGE
    bne     r24, r11, .mio_badarg

    ; Get current task
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)

    ; Find free region slot for this task
    move.l  r14, #KERN_DATA_BASE
    add     r14, r14, #KD_REGION_TABLE
    move.l  r11, #KD_REGION_TASK_SZ
    mulu    r11, r13, r11
    add     r14, r14, r11               ; r14 = &region_table[task][0]
    move.l  r15, #0
.mio_find_region:
    move.l  r11, #KD_REGION_MAX
    bge     r15, r11, .mio_nomem
    move.l  r11, #KD_REGION_STRIDE
    mulu    r11, r15, r11
    add     r16, r14, r11
    load.b  r11, KD_REG_TYPE(r16)
    beqz    r11, .mio_found_region
    add     r15, r15, #1
    bra     .mio_find_region
.mio_found_region:
    move.q  r21, r16                    ; r21 = &region[slot]

    ; Find free VA in task's dynamic window
    push    r13
    move.q  r1, r13                     ; task_id
    move.l  r2, #1                      ; 1 page
    jsr     find_free_va                ; R1 = VA, R2 = err
    pop     r13
    bnez    r2, .mio_nomem
    move.q  r23, r1                     ; r23 = VA

    ; Map the I/O page: P|R|W|U = 0x17 (no X)
    move.l  r12, #KERN_DATA_BASE
    lsl     r1, r13, #3
    add     r1, r1, #KD_PTBR_BASE
    add     r1, r1, r12
    load.q  r1, (r1)                    ; R1 = PT base
    move.q  r2, r23                     ; R2 = VA
    move.q  r3, r24                     ; R3 = PPN (0xF0)
    move.l  r4, #1                      ; R4 = 1 page
    move.l  r5, #0x17                   ; R5 = P|R|W|U
    jsr     map_pages

    ; Fill region entry: type = REGION_IO (don't free on cleanup)
    store.l r23, KD_REG_VA(r21)
    store.w r24, KD_REG_PPN(r21)
    move.w  r11, #1
    store.w r11, KD_REG_PAGES(r21)
    move.b  r11, #REGION_IO
    store.b r11, KD_REG_TYPE(r21)

    tlbflush
    move.q  r1, r23                     ; return VA
    move.q  r2, #ERR_OK
    eret

.mio_badarg:
    move.q  r1, #0
    move.q  r2, #ERR_BADARG
    eret
.mio_nomem:
    move.q  r1, #0
    move.q  r2, #ERR_NOMEM
    eret

; ============================================================================
; SYS_EXEC_PROGRAM (M9) — Launch bundled program with optional args
; ============================================================================
; R1 = program table index (0-based)
; R2 = args_ptr (in caller's space, 0 = no args)
; R3 = args_len (max DATA_ARGS_MAX, 0 = no args)
; Returns: R1 = new task_id, R2 = error
.do_exec_program:
    move.q  r24, r1                     ; r24 = requested index
    move.q  r25, r2                     ; r25 = args_ptr (user VA)
    move.q  r26, r3                     ; r26 = args_len

    ; Validate args_ptr is in user address space
    beqz    r25, .ep_args_ok            ; null ptr = no args (handled later)
    move.l  r11, #0x600000              ; USER_CODE_BASE (lowest user addr)
    blt     r25, r11, .ep_badarg_norestore ; below user space
.ep_args_ok:

    ; Validate args_len <= DATA_ARGS_MAX
    move.l  r11, #DATA_ARGS_MAX
    bgt     r26, r11, .ep_badarg

    ; Save caller's PTBR (we need to switch to kernel PT for load_program,
    ; and also need caller's PT to read args from user space)
    mfcr    r28, cr0                    ; r28 = caller PTBR

    ; Walk program table to find the entry at index r24
    la      r30, program_table
    move.l  r20, #0
.ep_scan:
    load.q  r7, PROGTAB_OFF_PTR(r30)
    beqz    r7, .ep_badarg_norestore    ; hit sentinel before index
    beq     r20, r24, .ep_found
    add     r30, r30, #PROGTAB_ENTRY_SIZE
    add     r20, r20, #1
    bra     .ep_scan
.ep_found:
    ; r7 = image_ptr, load size
    load.q  r8, PROGTAB_OFF_SIZE(r30)

    ; Switch to kernel PT for load_program
    move.l  r11, #KERN_PAGE_TABLE
    mtcr    cr0, r11
    tlbflush

    ; Save args info on stack before load_program (it clobbers everything)
    push    r25                         ; args_ptr
    push    r26                         ; args_len
    push    r28                         ; caller PTBR

    jsr     load_program                ; R1 = task_id, R2 = err

    pop     r28                         ; caller PTBR
    pop     r26                         ; args_len
    pop     r25                         ; args_ptr

    ; Check if load_program failed
    bnez    r2, .ep_fail

    ; R1 = new task_id. Copy args if present.
    move.q  r22, r1                     ; r22 = new task_id (save)
    beqz    r26, .ep_no_args            ; no args to copy
    beqz    r25, .ep_no_args            ; null ptr

    ; Compute destination: new task's data page + DATA_ARGS_OFFSET
    move.l  r11, #USER_SLOT_STRIDE
    mulu    r11, r22, r11
    move.l  r14, #USER_DATA_BASE
    add     r14, r14, r11
    add     r14, r14, #DATA_ARGS_OFFSET ; r14 = dest addr (phys, kernel-mapped)

    ; Phase 1: Read args from caller's space into kernel stack buffer
    ; Switch to caller's PT once, read all bytes, switch back
    mtcr    cr0, r28                    ; caller's PT
    tlbflush
    ; Use kernel stack as temp buffer (below current KSP)
    ; r31 = kernel SP. Write args at r31-256..r31-1
    sub     r15, r31, #256              ; r15 = temp buffer base on kernel stack
    move.l  r4, #0
.ep_read_args:
    bge     r4, r26, .ep_read_done
    add     r5, r25, r4
    load.b  r6, (r5)                    ; read from caller's VA
    add     r5, r15, r4
    store.b r6, (r5)                    ; write to kernel stack buffer
    add     r4, r4, #1
    bra     .ep_read_args
.ep_read_done:

    ; Phase 2: Write from kernel stack to new task's data page (kernel PT)
    move.l  r11, #KERN_PAGE_TABLE
    mtcr    cr0, r11
    tlbflush
    move.l  r4, #0
.ep_write_args:
    bge     r4, r26, .ep_args_term
    add     r5, r15, r4
    load.b  r6, (r5)                    ; read from kernel stack buffer
    add     r5, r14, r4
    store.b r6, (r5)                    ; write to dest
    add     r4, r4, #1
    bra     .ep_write_args
.ep_args_term:
    ; Null-terminate
    add     r5, r14, r26
    store.b r0, (r5)                    ; null terminator

.ep_no_args:
    ; Restore caller's PTBR
    mtcr    cr0, r28
    tlbflush
    move.q  r1, r22                     ; R1 = new task_id
    move.q  r2, #ERR_OK
    eret

.ep_fail:
    ; load_program failed, R2 already has error
    mtcr    cr0, r28
    tlbflush
    move.q  r1, #0
    eret

.ep_badarg:
    ; Need to restore PTBR if we saved it
    mtcr    cr0, r28
    tlbflush
.ep_badarg_norestore:
    move.q  r1, #0
    move.q  r2, #ERR_BADARG
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
    move.l  r24, #REGION_IO
    beq     r22, r24, .ktc_region_clear ; I/O: unmap only, don't free pages
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
    ; Check if GPRs were saved on user stack (timer preemption)
    load.b  r14, KD_TASK_GPR_SAVED(r15)
    beqz    r14, .restore_no_gprs   ; no GPR frame → just eret

    ; Clear the flag
    store.b r0, KD_TASK_GPR_SAVED(r15)

    ; Restore ALL GPRs from user stack (M9 preemption safety)
    ; USP in cr12 points to GPR frame base (adjusted USP from save)
    mfcr    r14, cr12               ; r14 = GPR frame base
    load.q  r1,   0(r14)
    load.q  r2,   8(r14)
    load.q  r3,  16(r14)
    load.q  r4,  24(r14)
    load.q  r5,  32(r14)
    load.q  r6,  40(r14)
    load.q  r7,  48(r14)
    load.q  r8,  56(r14)
    load.q  r9,  64(r14)
    load.q  r10, 72(r14)
    load.q  r11, 80(r14)
    load.q  r12, 88(r14)
    load.q  r13, 96(r14)
    ; skip r14 for now
    load.q  r15, 112(r14)
    load.q  r16, 120(r14)
    load.q  r17, 128(r14)
    load.q  r18, 136(r14)
    load.q  r19, 144(r14)
    load.q  r20, 152(r14)
    load.q  r21, 160(r14)
    load.q  r22, 168(r14)
    load.q  r23, 176(r14)
    load.q  r24, 184(r14)
    load.q  r25, 192(r14)
    load.q  r26, 200(r14)
    load.q  r27, 208(r14)
    load.q  r28, 216(r14)
    load.q  r29, 224(r14)
    load.q  r30, 232(r14)
    load.q  r31, 240(r14)
    ; Restore original USP (frame base + 248) and R14
    push    r14                     ; save frame base on kernel stack
    add     r14, r14, #248          ; r14 = original USP
    mtcr    cr12, r14               ; restore original USP
    pop     r14                     ; frame base back in r14
    load.q  r14, 104(r14)          ; restore user R14
    eret

.restore_no_gprs:
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
    ; --- Save user GPRs on user stack FIRST (M9: preemption safety) ---
    ; We need R14 for addressing. Use kernel stack to save R14 temporarily.
    push    r14                     ; save user R14 on kernel stack
    mfcr    r14, cr12               ; r14 = user SP
    sub     r14, r14, #248          ; frame for R1-R31 (31 × 8)
    store.q r1,   0(r14)
    store.q r2,   8(r14)
    store.q r3,  16(r14)
    store.q r4,  24(r14)
    store.q r5,  32(r14)
    store.q r6,  40(r14)
    store.q r7,  48(r14)
    store.q r8,  56(r14)
    store.q r9,  64(r14)
    store.q r10, 72(r14)
    store.q r11, 80(r14)
    store.q r12, 88(r14)
    store.q r13, 96(r14)
    pop     r1                      ; r1 = user R14 (saved on kernel stack)
    store.q r1, 104(r14)            ; save user R14
    store.q r15, 112(r14)
    store.q r16, 120(r14)
    store.q r17, 128(r14)
    store.q r18, 136(r14)
    store.q r19, 144(r14)
    store.q r20, 152(r14)
    store.q r21, 160(r14)
    store.q r22, 168(r14)
    store.q r23, 176(r14)
    store.q r24, 184(r14)
    store.q r25, 192(r14)
    store.q r26, 200(r14)
    store.q r27, 208(r14)
    store.q r28, 216(r14)
    store.q r29, 224(r14)
    store.q r30, 232(r14)
    store.q r31, 240(r14)
    ; r14 = adjusted user SP (with GPR frame). Save for later.
    move.q  r28, r14                ; r28 = adjusted user SP

    ; --- Now proceed with normal interrupt handling ---
    move.l  r12, #KERN_DATA_BASE

    ; Increment tick count
    load.q  r10, KD_TICK_COUNT(r12)
    add     r10, r10, #1
    store.q r10, KD_TICK_COUNT(r12)

    ; Context switch
    load.q  r13, (r12)              ; current_task

    ; Save current task PC and adjusted USP
    lsl     r15, r13, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12
    mfcr    r14, cr3
    store.q r14, KD_TASK_PC(r15)
    store.q r28, KD_TASK_USP(r15)   ; save adjusted USP (GPR frame below)
    move.b  r14, #1
    store.b r14, KD_TASK_GPR_SAVED(r15)  ; flag: GPRs on stack

    ; Find next runnable task (round-robin)
    jsr     find_next_runnable          ; R13 = next (or halts if deadlock)
    ; If same task, stay on current
    load.q  r16, (r12)                  ; original current_task
    beq     r13, r16, .intr_stay

    ; Switch to next
    store.q r13, (r12)
    bra     restore_task

.intr_stay:
    ; Same task continues — restore ALL GPRs from user stack
    ; R28 has the adjusted USP (frame base)
    move.q  r14, r28                ; r14 = GPR frame base
    load.q  r1,   0(r14)
    load.q  r2,   8(r14)
    load.q  r3,  16(r14)
    load.q  r4,  24(r14)
    load.q  r5,  32(r14)
    load.q  r6,  40(r14)
    load.q  r7,  48(r14)
    load.q  r8,  56(r14)
    load.q  r9,  64(r14)
    load.q  r10, 72(r14)
    load.q  r11, 80(r14)
    load.q  r12, 88(r14)
    load.q  r13, 96(r14)
    ; skip r14 for now (we're using it)
    load.q  r15, 112(r14)
    load.q  r16, 120(r14)
    load.q  r17, 128(r14)
    load.q  r18, 136(r14)
    load.q  r19, 144(r14)
    load.q  r20, 152(r14)
    load.q  r21, 160(r14)
    load.q  r22, 168(r14)
    load.q  r23, 176(r14)
    load.q  r24, 184(r14)
    load.q  r25, 192(r14)
    load.q  r26, 200(r14)
    load.q  r27, 208(r14)
    load.q  r28, 216(r14)
    load.q  r29, 224(r14)
    load.q  r30, 232(r14)
    load.q  r31, 240(r14)
    load.q  r14, 104(r14)          ; restore R14 last (clobbers our frame pointer)
    ; CR12 (USP) stays at original value — no adjustment needed
    eret

; ============================================================================
; Program Table (M8: static list of bundled program images)
; ============================================================================
; Each entry: 24 bytes (image_ptr, image_size, reserved).
; Sentinel: image_ptr = 0.
; Order determines launch order (and thus task slot assignment).

program_table:
    dc.q    prog_console
    dc.q    prog_console_end - prog_console
    dc.q    0
    dc.q    prog_doslib
    dc.q    prog_doslib_end - prog_doslib
    dc.q    0
    dc.q    prog_shell
    dc.q    prog_shell_end - prog_shell
    dc.q    0
    ; --- On-demand programs (launched via SYS_EXEC_PROGRAM) ---
    dc.q    prog_version
    dc.q    prog_version_end - prog_version
    dc.q    0
    dc.q    prog_avail
    dc.q    prog_avail_end - prog_avail
    dc.q    0
    dc.q    prog_dir
    dc.q    prog_dir_end - prog_dir
    dc.q    0
    dc.q    prog_type
    dc.q    prog_type_end - prog_type
    dc.q    0
    dc.q    prog_echo_cmd
    dc.q    prog_echo_cmd_end - prog_echo_cmd
    dc.q    0
    ; sentinel
    dc.q    0
    dc.q    0
    dc.q    0

; ============================================================================
; Bundled Program Images (M8)
; ============================================================================
; Each image: 32-byte header + code + optional data.
; Programs use the standard preamble to compute their own base addresses.
;
; Data section layout per program (copied to data page offset 0 by loader):
;   +0:   port name strings and other initial data
;   +128: scratch area (saved registers, port IDs, etc.)

; ---------------------------------------------------------------------------
; console.handler — CON: handler (M9)
; ---------------------------------------------------------------------------
; AmigaOS-style CON: handler. Owns keyboard input AND screen output.
; Registers as "console.handler" port. Maps terminal I/O via SYS_MAP_IO.
; Polling loop: GetMsg (non-blocking) for output + readline requests,
; polls keyboard when a readline is pending.
;
; Data page layout:
;   0:   "console.handler\0" (16 bytes, port name)
;   16:  "console.handler ONLINE [Task\0" (banner)
;   128: task_id (8 bytes)
;   136: console_port (8 bytes)
;   144: term_io_va (8 bytes, from MAP_IO)
;   152: readline_reply_port (8 bytes)
;   160: readline_share_handle (4 bytes + padding)
;   168: readline_mapped_va (8 bytes, cached MapShared)
;   176: readline_pending (1 byte)

prog_console:
    ; Header
    dc.l    IMG_MAGIC_LO, IMG_MAGIC_HI
    dc.l    prog_console_code_end - prog_console_code   ; code_size
    dc.l    prog_console_data_end - prog_console_data   ; data_size
    dc.l    0                           ; flags
    ds.b    12                          ; reserved
prog_console_code:
    ; === Preamble: compute data page base (preemption-safe) ===
    sub     sp, sp, #16                 ; reserve [sp]=R29, [sp+8]=scratch
.con_preamble:
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO           ; R1 = task_id
    store.q r1, 8(sp)                   ; save task_id
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO           ; R1 = task_id (verify)
    load.q  r28, 8(sp)
    bne     r1, r28, .con_preamble      ; mismatch → retry
    move.l  r28, #USER_SLOT_STRIDE
    mulu    r28, r1, r28
    move.l  r29, #USER_DATA_BASE
    add     r29, r29, r28
    store.q r29, (sp)                   ; save R29 first computation
    load.q  r1, 8(sp)
    move.l  r28, #USER_SLOT_STRIDE
    mulu    r28, r1, r28
    move.l  r29, #USER_DATA_BASE
    add     r29, r29, r28
    load.q  r28, (sp)
    bne     r29, r28, .con_preamble     ; mismatch → retry
    store.q r29, (sp)                   ; confirmed correct
    load.q  r1, 8(sp)
    store.q r1, 128(r29)                ; data[128] = task_id

    ; === Terminal line mode ===
    ; SYS_READ_INPUT handles terminal I/O in kernel mode, no MAP_IO needed.
    ; Line mode is already the default for the terminal.

    ; === Create "console.handler" port ===
    load.q  r29, (sp)
    move.q  r1, r29                     ; R1 = name_ptr (data[0])
    move.l  r2, #PF_PUBLIC
    syscall #SYS_CREATE_PORT            ; R1 = port_id
    load.q  r29, (sp)
    store.q r1, 136(r29)                ; data[136] = console_port

    ; === Clear readline_pending flag ===
    store.b r0, 176(r29)

    ; === Print "console.handler ONLINE [Taskn]\r\n" via DebugPutChar ===
    add     r20, r29, #16               ; r20 = &data[16] = banner string
.con_banner_loop:
    load.b  r1, (r20)
    beqz    r1, .con_banner_id
    store.q r20, 8(sp)
    syscall #SYS_DEBUG_PUTCHAR
    load.q  r29, (sp)
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .con_banner_loop
.con_banner_id:
    load.q  r29, (sp)
    load.q  r1, 128(r29)               ; task_id
    add     r1, r1, #0x30              ; ASCII '0'
    syscall #SYS_DEBUG_PUTCHAR
    move.l  r1, #0x5D                   ; ']'
    syscall #SYS_DEBUG_PUTCHAR
    move.l  r1, #0x0D                   ; '\r'
    syscall #SYS_DEBUG_PUTCHAR
    move.l  r1, #0x0A                   ; '\n'
    syscall #SYS_DEBUG_PUTCHAR

    ; === Main polling loop ===
.con_poll_loop:
    ; --- Try to get a message (non-blocking) ---
    load.q  r29, (sp)
    load.q  r1, 136(r29)               ; console_port
    syscall #SYS_GET_MSG                ; R1=type, R2=data0, R3=err, R4=data1, R5=reply_port, R6=share_handle
    load.q  r29, (sp)
    bnez    r3, .con_no_msg             ; R3 != ERR_OK → no message (ERR_AGAIN)

    ; --- Got a message. Dispatch on type. ---
    ; R1=type, R2=data0, R5=reply_port, R6=share_handle
    beqz    r1, .con_print_char         ; type 0 = CON_MSG_CHAR

    move.l  r11, #CON_MSG_READLINE
    beq     r1, r11, .con_readline_req

    ; Unknown type — ignore, loop back
    bra     .con_poll_loop

    ; --- CON_MSG_CHAR: print data0 low byte ---
.con_print_char:
    move.q  r1, r2                      ; char from data0
    syscall #SYS_DEBUG_PUTCHAR
    load.q  r29, (sp)
    bra     .con_poll_loop

    ; --- CON_MSG_READLINE request ---
.con_readline_req:
    ; Save reply_port and share_handle before checking pending flag
    load.q  r29, (sp)
    store.q r5, 8(sp)                   ; save reply_port to stack scratch
    store.l r6, 160(r29)                ; save share_handle to data[160]

    ; Check if readline is already pending
    load.b  r20, 176(r29)              ; readline_pending
    bnez    r20, .con_readline_busy     ; already pending → reject

    ; Accept readline request
    load.q  r5, 8(sp)                  ; restore reply_port
    store.q r5, 152(r29)               ; data[152] = readline_reply_port
    ; share_handle already saved at data[160]

    ; MapShared on first use (check if cached VA is 0)
    load.q  r20, 168(r29)             ; readline_mapped_va
    bnez    r20, .con_readline_mapped  ; already mapped

    ; First time: MapShared to get VA
    load.l  r1, 160(r29)              ; share_handle
    syscall #SYS_MAP_SHARED            ; R1 = VA, R2 = err
    load.q  r29, (sp)
    beqz    r1, .con_poll_loop         ; MapShared failed, drop request
    store.q r1, 168(r29)              ; cache mapped VA

.con_readline_mapped:
    ; Set readline_pending = 1
    move.b  r20, #1
    store.b r20, 176(r29)
    bra     .con_poll_loop

    ; --- Reject: readline already pending → reply with ERR_AGAIN ---
.con_readline_busy:
    load.q  r5, 8(sp)                 ; reply_port from stack
    move.q  r1, r5                     ; R1 = reply_port_id
    move.l  r2, #ERR_AGAIN             ; R2 = type = error code
    move.q  r3, r0                     ; R3 = data0 = 0
    move.q  r4, r0                     ; R4 = data1 = 0
    move.q  r5, r0                     ; R5 = share_handle = 0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .con_poll_loop

    ; --- No message: check if readline pending + keyboard ready ---
.con_no_msg:
    load.b  r20, 176(r29)             ; readline_pending
    beqz    r20, .con_yield            ; not pending → just yield

    ; Use SYS_READ_INPUT to read a line from terminal (kernel-mode I/O)
    ; Read directly into the shared buffer (readline_mapped_va)
    load.q  r1, 168(r29)              ; R1 = readline_mapped_va (shared buffer)
    move.l  r2, #126                   ; R2 = max_len
    syscall #SYS_READ_INPUT            ; R1=bytes_read, R2=err
    load.q  r29, (sp)
    bnez    r2, .con_yield             ; ERR_AGAIN = no line ready yet

    ; Line was read into shared buffer. R1 = byte count.
    move.q  r25, r1                    ; save byte count

    ; Reply to readline requester
    load.q  r1, 152(r29)              ; readline_reply_port
    move.l  r2, #0                     ; type = 0 (success)
    move.q  r3, r25                    ; data0 = byte count
    move.q  r4, r0                     ; data1 = 0
    move.q  r5, r0                     ; share_handle = 0
    syscall #SYS_REPLY_MSG

    ; Clear readline_pending
    load.q  r29, (sp)
    store.b r0, 176(r29)
    bra     .con_poll_loop

.con_yield:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .con_poll_loop

prog_console_code_end:

prog_console_data:
    dc.b    "console.handler", 0       ; offset 0: port name (16 bytes exactly)
    dc.b    "console.handler ONLINE [Task ", 0  ; offset 16: banner string (29 bytes)
    ; offset 128+ is scratch (task_id, port_id, etc.) — zeroed by loader
prog_console_data_end:
    align   8
prog_console_end:

; ---------------------------------------------------------------------------
; dos.library — RAM: filesystem service (M9)
; ---------------------------------------------------------------------------
; Amiga-style dos.library: in-memory filesystem with Open/Read/Write/Close/Dir.
; Registers as "dos.library" public port. Discovers console.handler via
; OpenLibrary. Handles DOS_OPEN, DOS_READ, DOS_WRITE, DOS_CLOSE, DOS_DIR,
; DOS_RUN requests from any task via shared-memory message passing.
;
; Data page layout:
;   0:   "console.handler\0"       (16 bytes, for OpenLibrary)
;   16:  "dos.library\0"           (16 bytes, port name for CreatePort)
;   32:  "dos.library ONLINE [Task\0" (22 bytes + 10 pad = 32 bytes, banner)
;   64:  padding (64 bytes)
;   128: task_id (8)
;   136: console_port (8)
;   144: dos_port (8)
;   152: storage_va (8)    — 64KB AllocMem for file data (16 × 4KB slots)
;   160: caller_share_handle (8, cached)
;   168: caller_mapped_va (8, cached MapShared result)
;   176: open_handles[8] (1 byte each: file_index, 0xFF=unused)
;   184: reserved (8)
;   192: File table: 16 entries × 28 bytes = 448 bytes (ends at 640)
;         Each entry: name[16], offset(4), size(4), capacity(4)
;   640: "readme\0" + pad (16 bytes, scratch for pre-create)
;   656: "Welcome to IntuitionOS M9\r\n\0" (29 bytes, pre-create content)
;   688: scratch: saved reply_port (8)
;   696: scratch: saved msg_type (8)
;   704: scratch: saved data0 (8)
;   712: scratch: saved data1 (8)
;   720: scratch: saved share_handle (8)

prog_doslib:
    ; Header
    dc.l    IMG_MAGIC_LO, IMG_MAGIC_HI
    dc.l    prog_doslib_code_end - prog_doslib_code
    dc.l    prog_doslib_data_end - prog_doslib_data
    dc.l    0
    ds.b    12
prog_doslib_code:

    ; =====================================================================
    ; Preamble: compute data page base (preemption-safe double-check)
    ; =====================================================================
    sub     sp, sp, #16
.dos_preamble:
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO          ; R1 = task_id
    store.q r1, 8(sp)
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO          ; R1 = task_id (verify)
    load.q  r28, 8(sp)
    bne     r1, r28, .dos_preamble
    move.l  r28, #USER_SLOT_STRIDE
    mulu    r28, r1, r28
    move.l  r29, #USER_DATA_BASE
    add     r29, r29, r28
    store.q r29, (sp)
    load.q  r1, 8(sp)
    move.l  r28, #USER_SLOT_STRIDE
    mulu    r28, r1, r28
    move.l  r29, #USER_DATA_BASE
    add     r29, r29, r28
    load.q  r28, (sp)
    bne     r29, r28, .dos_preamble
    store.q r29, (sp)
    load.q  r1, 8(sp)
    store.q r1, 128(r29)               ; data[128] = task_id

    ; =====================================================================
    ; OpenLibrary("console.handler", 0) with retry until found
    ; =====================================================================
.dos_openlib_retry:
    load.q  r29, (sp)
    move.q  r1, r29                     ; R1 = &data[0] = "console.handler"
    move.q  r2, r0                      ; R2 = version 0
    syscall #SYS_OPEN_LIBRARY           ; R1 = handle (port_id), R2 = err
    load.q  r29, (sp)
    bnez    r2, .dos_openlib_wait
    store.q r1, 136(r29)               ; data[136] = console_port
    bra     .dos_send_banner
.dos_openlib_wait:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .dos_openlib_retry

    ; =====================================================================
    ; Send "dos.library ONLINE [Taskn]\r\n" banner to console.handler
    ; =====================================================================
.dos_send_banner:
    add     r20, r29, #32              ; r20 = &data[32] = banner string
.dos_ban_loop:
    load.b  r1, (r20)
    beqz    r1, .dos_ban_id
    store.q r20, 8(sp)
.dos_ban_retry:
    load.q  r20, 8(sp)
    load.b  r3, (r20)                   ; data0 = char
    move.l  r2, #0                      ; type = CON_MSG_CHAR
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 136(r29)               ; console_port
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .dos_ban_full
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .dos_ban_loop
.dos_ban_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .dos_ban_retry
.dos_ban_id:
    ; Send task_id digit + "]\r\n"
.dos_bid_retry:
    load.q  r29, (sp)
    load.q  r3, 128(r29)
    add     r3, r3, #0x30              ; ASCII '0'+id
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r1, 136(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .dos_bid_full
    bra     .dos_bid_bracket
.dos_bid_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .dos_bid_retry
.dos_bid_bracket:
.dos_brk_retry:
    move.l  r3, #0x5D                   ; ']'
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 136(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .dos_brk_full
    bra     .dos_cr
.dos_brk_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .dos_brk_retry
.dos_cr:
.dos_cr_retry:
    move.l  r3, #0x0D
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 136(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .dos_cr_full
    bra     .dos_lf
.dos_cr_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .dos_cr_retry
.dos_lf:
.dos_lf_retry:
    move.l  r3, #0x0A
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 136(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .dos_lf_full
    bra     .dos_banner_done
.dos_lf_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .dos_lf_retry
.dos_banner_done:

    ; =====================================================================
    ; CreatePort("dos.library", PF_PUBLIC)
    ; =====================================================================
    load.q  r29, (sp)
    add     r1, r29, #16               ; R1 = &data[16] = "dos.library"
    move.l  r2, #PF_PUBLIC
    syscall #SYS_CREATE_PORT           ; R1 = port_id
    load.q  r29, (sp)
    store.q r1, 144(r29)               ; data[144] = dos_port

    ; =====================================================================
    ; AllocMem(0x10000, MEMF_CLEAR) — 64KB for file storage (16 × 4KB)
    ; =====================================================================
    move.l  r1, #0x10000               ; 64KB
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM             ; R1 = VA, R2 = err, R3 = share_handle
    load.q  r29, (sp)
    store.q r1, 152(r29)               ; data[152] = storage_va

    ; =====================================================================
    ; Initialize open_handles[0..7] = 0xFF (all unused)
    ; =====================================================================
    move.l  r14, #0xFF
    store.b r14, 176(r29)
    store.b r14, 177(r29)
    store.b r14, 178(r29)
    store.b r14, 179(r29)
    store.b r14, 180(r29)
    store.b r14, 181(r29)
    store.b r14, 182(r29)
    store.b r14, 183(r29)

    ; =====================================================================
    ; Pre-create "readme" file at file table entry 0
    ; =====================================================================
    ; Copy filename from data[640] to file_table[0].name (data[192])
    add     r20, r29, #640             ; src = &data[640] = "readme"
    add     r21, r29, #192             ; dst = &file_table[0].name
    move.l  r14, #0
.dos_cpname:
    load.b  r15, (r20)
    store.b r15, (r21)
    beqz    r15, .dos_cpname_done
    add     r20, r20, #1
    add     r21, r21, #1
    add     r14, r14, #1
    move.l  r28, #15
    blt     r14, r28, .dos_cpname
.dos_cpname_done:
    ; Set file_table[0].offset = 0 (already zero from init)
    ; Set file_table[0].size = 28 (length of welcome message)
    move.l  r14, #28
    store.l r14, 212(r29)              ; data[192+20] = size = 28
    ; Set file_table[0].capacity = 4096
    move.l  r14, #4096
    store.l r14, 216(r29)              ; data[192+24] = capacity = 4096

    ; Copy welcome message from data[656] to storage_va+0
    add     r20, r29, #656             ; src = welcome message
    load.q  r21, 152(r29)              ; dst = storage_va
.dos_cpwelcome:
    load.b  r15, (r20)
    store.b r15, (r21)
    beqz    r15, .dos_init_done
    add     r20, r20, #1
    add     r21, r21, #1
    bra     .dos_cpwelcome
.dos_init_done:

    ; =====================================================================
    ; Main loop: WaitPort(dos_port) → dispatch on message type
    ; =====================================================================
.dos_main_loop:
    load.q  r29, (sp)
    load.q  r1, 144(r29)               ; R1 = dos_port
    syscall #SYS_WAIT_PORT              ; R1=type R2=data0 R3=err R4=data1 R5=reply R6=share
    load.q  r29, (sp)

    ; Save message fields to data page scratch
    store.q r5, 688(r29)               ; saved reply_port
    store.q r1, 696(r29)               ; saved type (opcode)
    store.q r2, 704(r29)               ; saved data0
    store.q r4, 712(r29)               ; saved data1
    store.q r6, 720(r29)               ; saved share_handle

    ; --- Map caller's shared buffer (re-map if share_handle changed) ---
    load.l  r14, 728(r29)              ; cached share_handle
    load.l  r15, 720(r29)              ; incoming share_handle
    beq     r14, r15, .dos_have_buf    ; same handle → use cached VA
    ; Different handle or first time: do MapShared
    store.l r15, 728(r29)              ; update cached handle
    move.q  r1, r15                    ; R1 = new share_handle
    syscall #SYS_MAP_SHARED            ; R1 = VA, R2 = err
    load.q  r29, (sp)
    beqz    r1, .dos_reply_err         ; MapShared failed
    store.q r1, 168(r29)              ; update cached VA
.dos_have_buf:

.dos_dispatch:
    load.q  r14, 696(r29)              ; r14 = opcode
    move.l  r28, #DOS_DIR
    beq     r14, r28, .dos_do_dir
    move.l  r28, #DOS_OPEN
    beq     r14, r28, .dos_do_open
    move.l  r28, #DOS_READ
    beq     r14, r28, .dos_do_read
    move.l  r28, #DOS_WRITE
    beq     r14, r28, .dos_do_write
    move.l  r28, #DOS_CLOSE
    beq     r14, r28, .dos_do_close
    move.l  r28, #DOS_RUN
    beq     r14, r28, .dos_do_run
    ; Unknown opcode → reply with error and loop
    bra     .dos_reply_err

    ; =================================================================
    ; DOS_DIR (type=5): format directory listing into caller's buffer
    ; =================================================================
.dos_do_dir:
    load.q  r20, 168(r29)              ; r20 = dest (caller's shared buffer)
    move.q  r21, r0                     ; r21 = total bytes written
    move.l  r22, #0                     ; r22 = file table index
.dos_dir_entry:
    move.l  r28, #DOS_MAX_FILES
    bge     r22, r28, .dos_dir_done
    ; Compute entry base: data[192] + index * 28
    move.l  r14, #28
    mulu    r14, r22, r14
    add     r14, r14, #192
    add     r14, r29, r14               ; r14 = &file_table[index]
    load.b  r15, (r14)                  ; first byte of name
    beqz    r15, .dos_dir_next          ; empty entry → skip
    ; Copy name chars until null (max 16)
    move.q  r16, r14                    ; r16 = name pointer
    move.l  r17, #0                     ; name char count
.dos_dir_cpname:
    load.b  r15, (r16)
    beqz    r15, .dos_dir_pad
    store.b r15, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    add     r16, r16, #1
    add     r17, r17, #1
    move.l  r28, #16
    blt     r17, r28, .dos_dir_cpname
.dos_dir_pad:
    ; Pad with spaces to column 16
    move.l  r28, #16
    bge     r17, r28, .dos_dir_size
    move.l  r15, #0x20                  ; ' '
    store.b r15, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    add     r17, r17, #1
    bra     .dos_dir_pad
.dos_dir_size:
    ; Read file size from entry+20, write decimal digits
    load.l  r15, 20(r14)               ; r15 = file size
    ; Simple decimal: divide by powers of 10 (max 4096, so 4 digits)
    ; Write thousands digit
    move.l  r28, #1000
    divu    r16, r15, r28               ; r16 = thousands
    mulu    r17, r16, r28
    sub     r15, r15, r17
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    ; Hundreds
    move.l  r28, #100
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    ; Tens
    move.l  r28, #10
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    ; Ones
    add     r15, r15, #0x30
    store.b r15, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    ; Write "\r\n"
    move.l  r15, #0x0D
    store.b r15, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r15, #0x0A
    store.b r15, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
.dos_dir_next:
    add     r22, r22, #1
    bra     .dos_dir_entry
.dos_dir_done:
    ; Null-terminate
    store.b r0, (r20)
    ; Reply with data0 = bytes written
    load.q  r1, 688(r29)               ; reply_port
    move.l  r2, #DOS_OK                 ; type = success
    move.q  r3, r21                     ; data0 = bytes written
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

    ; =================================================================
    ; DOS_OPEN (type=1): open file by name from shared buffer
    ; =================================================================
    ; data0 = mode (READ=0, WRITE=1), filename in caller's shared buffer
.dos_do_open:
    load.q  r20, 704(r29)              ; r20 = mode
    load.q  r23, 168(r29)              ; r23 = mapped VA (filename pointer)

    ; --- Search file table for matching name (case-insensitive) ---
    move.l  r22, #0                     ; r22 = file table index
.dos_open_search:
    move.l  r28, #DOS_MAX_FILES
    bge     r22, r28, .dos_open_notfound
    ; Compute entry base
    move.l  r14, #28
    mulu    r14, r22, r14
    add     r14, r14, #192
    add     r14, r29, r14               ; r14 = &file_table[index]
    load.b  r15, (r14)
    beqz    r15, .dos_open_snext        ; empty entry → skip
    ; Case-insensitive compare: r14=table name, r23=request name
    move.q  r16, r14                    ; r16 = table name ptr
    move.q  r17, r23                    ; r17 = request name ptr
    move.l  r18, #0                     ; char index
.dos_open_cmp:
    load.b  r24, (r16)
    load.b  r25, (r17)
    ; Lowercase both if A-Z
    move.l  r28, #0x41
    blt     r24, r28, .dos_ocmp_skip1
    move.l  r28, #0x5A
    bgt     r24, r28, .dos_ocmp_skip1
    or      r24, r24, #0x20
.dos_ocmp_skip1:
    move.l  r28, #0x41
    blt     r25, r28, .dos_ocmp_skip2
    move.l  r28, #0x5A
    bgt     r25, r28, .dos_ocmp_skip2
    or      r25, r25, #0x20
.dos_ocmp_skip2:
    bne     r24, r25, .dos_open_snext   ; mismatch → try next
    beqz    r24, .dos_open_found        ; both null → match
    add     r16, r16, #1
    add     r17, r17, #1
    add     r18, r18, #1
    move.l  r28, #16
    blt     r18, r28, .dos_open_cmp
    ; Reached 16 chars without null → treat as match
    bra     .dos_open_found
.dos_open_snext:
    add     r22, r22, #1
    bra     .dos_open_search

.dos_open_notfound:
    ; If mode == WRITE, create new file entry
    bnez    r20, .dos_open_create
    ; Mode READ, file not found → error
    bra     .dos_reply_err

.dos_open_create:
    ; Find first empty file table slot
    move.l  r22, #0
.dos_create_scan:
    move.l  r28, #DOS_MAX_FILES
    bge     r22, r28, .dos_open_full
    move.l  r14, #28
    mulu    r14, r22, r14
    add     r14, r14, #192
    add     r14, r29, r14
    load.b  r15, (r14)
    beqz    r15, .dos_create_slot       ; found empty slot
    add     r22, r22, #1
    bra     .dos_create_scan
.dos_open_full:
    bra     .dos_reply_full

.dos_create_slot:
    ; r14 = entry base, r22 = slot index
    ; Copy filename from shared buffer to entry name[16]
    move.q  r16, r23                    ; src = request name
    move.q  r17, r14                    ; dst = entry name
    move.l  r18, #0
.dos_cpy_fname:
    load.b  r15, (r16)
    store.b r15, (r17)
    beqz    r15, .dos_cpy_fname_done
    add     r16, r16, #1
    add     r17, r17, #1
    add     r18, r18, #1
    move.l  r28, #15
    blt     r18, r28, .dos_cpy_fname
    store.b r0, (r17)                   ; force null at position 15
.dos_cpy_fname_done:
    ; Set offset = slot * 4096
    move.l  r15, #4096
    mulu    r15, r22, r15
    store.l r15, 16(r14)               ; entry.offset = slot * 4096
    ; Set size = 0 (new file)
    store.l r0, 20(r14)                ; entry.size = 0
    ; Set capacity = 4096
    move.l  r15, #4096
    store.l r15, 24(r14)               ; entry.capacity = 4096

.dos_open_found:
    ; r22 = file table index of found/created entry
    ; Find free handle slot
    move.l  r18, #0
.dos_find_handle:
    move.l  r28, #DOS_MAX_HANDLES
    bge     r18, r28, .dos_open_full_h
    add     r14, r29, #176
    add     r14, r14, r18
    load.b  r15, (r14)
    move.l  r28, #0xFF
    beq     r15, r28, .dos_got_handle
    add     r18, r18, #1
    bra     .dos_find_handle
.dos_open_full_h:
    bra     .dos_reply_full

.dos_got_handle:
    ; r18 = handle index, r22 = file index
    ; Store file_index in handles[handle]
    add     r14, r29, #176
    add     r14, r14, r18
    store.b r22, (r14)
    ; Reply: type=DOS_OK, data0=handle
    load.q  r1, 688(r29)
    move.l  r2, #DOS_OK
    move.q  r3, r18                     ; data0 = handle
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

    ; =================================================================
    ; DOS_READ (type=2): read file data into caller's shared buffer
    ; =================================================================
    ; data0 = handle, data1 = max_bytes
.dos_do_read:
    load.q  r18, 704(r29)              ; r18 = handle
    load.q  r19, 712(r29)              ; r19 = max_bytes
    ; Validate handle
    move.l  r28, #DOS_MAX_HANDLES
    bge     r18, r28, .dos_read_badh
    add     r14, r29, #176
    add     r14, r14, r18
    load.b  r22, (r14)                  ; r22 = file_index
    move.l  r28, #0xFF
    beq     r22, r28, .dos_read_badh
    ; Get file entry
    move.l  r14, #28
    mulu    r14, r22, r14
    add     r14, r14, #192
    add     r14, r29, r14               ; r14 = &file_table[file_index]
    load.l  r15, 16(r14)               ; r15 = offset within storage
    load.l  r16, 20(r14)               ; r16 = file size
    ; Clamp max_bytes to file size
    blt     r19, r16, .dos_read_clamp
    move.q  r19, r16
.dos_read_clamp:
    ; Copy from storage_va + offset to caller's shared buffer
    load.q  r20, 152(r29)              ; storage_va
    add     r20, r20, r15               ; src = storage_va + file_offset
    load.q  r21, 168(r29)              ; dst = caller's mapped buffer
    move.q  r17, r0                     ; bytes copied = 0
.dos_read_copy:
    bge     r17, r19, .dos_read_reply
    load.b  r15, (r20)
    store.b r15, (r21)
    add     r20, r20, #1
    add     r21, r21, #1
    add     r17, r17, #1
    bra     .dos_read_copy
.dos_read_reply:
    load.q  r1, 688(r29)
    move.l  r2, #DOS_OK
    move.q  r3, r17                     ; data0 = bytes read
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop
.dos_read_badh:
    bra     .dos_reply_badh

    ; =================================================================
    ; DOS_WRITE (type=3): write data from caller's buffer to file
    ; =================================================================
    ; data0 = handle, data1 = byte_count
.dos_do_write:
    load.q  r18, 704(r29)              ; r18 = handle
    load.q  r19, 712(r29)              ; r19 = byte_count
    ; Validate handle
    move.l  r28, #DOS_MAX_HANDLES
    bge     r18, r28, .dos_write_badh
    add     r14, r29, #176
    add     r14, r14, r18
    load.b  r22, (r14)                  ; r22 = file_index
    move.l  r28, #0xFF
    beq     r22, r28, .dos_write_badh
    ; Get file entry
    move.l  r14, #28
    mulu    r14, r22, r14
    add     r14, r14, #192
    add     r14, r29, r14               ; r14 = &file_table[file_index]
    load.l  r15, 16(r14)               ; r15 = offset within storage
    load.l  r16, 24(r14)               ; r16 = capacity
    ; Clamp byte_count to capacity
    blt     r19, r16, .dos_write_clamp
    move.q  r19, r16
.dos_write_clamp:
    ; Copy from caller's shared buffer to storage_va + offset
    load.q  r20, 168(r29)              ; src = caller's mapped buffer
    load.q  r21, 152(r29)              ; storage_va
    add     r21, r21, r15               ; dst = storage_va + file_offset
    move.q  r17, r0                     ; bytes copied = 0
.dos_write_copy:
    bge     r17, r19, .dos_write_done
    load.b  r15, (r20)
    store.b r15, (r21)
    add     r20, r20, #1
    add     r21, r21, #1
    add     r17, r17, #1
    bra     .dos_write_copy
.dos_write_done:
    ; Update file size
    move.l  r14, #28
    mulu    r14, r22, r14
    add     r14, r14, #192
    add     r14, r29, r14
    store.l r19, 20(r14)               ; entry.size = byte_count
    ; Reply
    load.q  r1, 688(r29)
    move.l  r2, #DOS_OK
    move.q  r3, r17                     ; data0 = bytes written
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop
.dos_write_badh:
    bra     .dos_reply_badh

    ; =================================================================
    ; DOS_CLOSE (type=4): close a file handle
    ; =================================================================
    ; data0 = handle
.dos_do_close:
    load.q  r18, 704(r29)              ; r18 = handle
    move.l  r28, #DOS_MAX_HANDLES
    bge     r18, r28, .dos_close_badh
    add     r14, r29, #176
    add     r14, r14, r18
    load.b  r15, (r14)
    move.l  r28, #0xFF
    beq     r15, r28, .dos_close_badh
    ; Mark handle as unused
    move.l  r15, #0xFF
    store.b r15, (r14)
    ; Reply success
    load.q  r1, 688(r29)
    move.l  r2, #DOS_OK
    move.q  r3, r0                      ; data0 = 0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop
.dos_close_badh:
    bra     .dos_reply_badh

    ; =================================================================
    ; DOS_RUN (type=6): launch a bundled program with args
    ; =================================================================
    ; data0 = prog_table_index, args in caller's shared buffer
.dos_do_run:
    load.q  r18, 704(r29)              ; r18 = prog_table_index
    load.q  r20, 168(r29)              ; r20 = caller's mapped buffer (args)
    ; Compute args length (scan for null)
    move.q  r21, r20
    move.q  r22, r0                     ; length counter
.dos_run_arglen:
    load.b  r15, (r21)
    beqz    r15, .dos_run_exec
    add     r21, r21, #1
    add     r22, r22, #1
    move.l  r28, #DATA_ARGS_MAX
    blt     r22, r28, .dos_run_arglen
.dos_run_exec:
    move.q  r1, r18                     ; R1 = prog index
    move.q  r2, r20                     ; R2 = args_ptr (in our mapped buf)
    move.q  r3, r22                     ; R3 = args_len
    syscall #SYS_EXEC_PROGRAM           ; R1 = task_id, R2 = err
    load.q  r29, (sp)
    move.q  r14, r1                     ; save task_id
    move.q  r15, r2                     ; save err
    ; Reply: type=err, data0=task_id
    load.q  r1, 688(r29)
    move.q  r2, r15                     ; type = err code (0=ok)
    move.q  r3, r14                     ; data0 = task_id
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

    ; =================================================================
    ; Shared reply blocks (saves code space by consolidating duplicates)
    ; =================================================================
.dos_reply_badh:
    load.q  r1, 688(r29)
    move.l  r2, #DOS_ERR_BADHANDLE
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop
.dos_reply_full:
    load.q  r1, 688(r29)
    move.l  r2, #DOS_ERR_FULL
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop
.dos_reply_err:
    load.q  r1, 688(r29)
    move.l  r2, #DOS_ERR_NOTFOUND
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

prog_doslib_code_end:

prog_doslib_data:
    ; --- Offset 0: "console.handler\0" (16 bytes) ---
    dc.b    "console.handler", 0
    ; --- Offset 16: "dos.library\0" + pad to 16 bytes ---
    dc.b    "dos.library", 0, 0, 0, 0, 0
    ; --- Offset 32: banner "dos.library ONLINE [Task \0" (26 bytes, ends at 58) ---
    dc.b    "dos.library ONLINE [Task ", 0
    ds.b    6                           ; pad to offset 64
    ; --- Offset 64: padding to 128 ---
    ds.b    64
    ; --- Offset 128: task_id (8) ---
    ds.b    8
    ; --- Offset 136: console_port (8) ---
    ds.b    8
    ; --- Offset 144: dos_port (8) ---
    ds.b    8
    ; --- Offset 152: storage_va (8) ---
    ds.b    8
    ; --- Offset 160: caller_share_handle (8) ---
    ds.b    8
    ; --- Offset 168: caller_mapped_va (8) ---
    ds.b    8
    ; --- Offset 176: open_handles[8] ---
    ds.b    8
    ; --- Offset 184: reserved (8) ---
    ds.b    8
    ; --- Offset 192: file table (16 entries × 28 bytes = 448 bytes, ends at 640) ---
    ds.b    448
    ; --- Offset 640: pre-create filename "readme\0" + pad to 16 ---
    dc.b    "readme", 0, 0, 0, 0, 0, 0, 0, 0, 0, 0
    ; --- Offset 656: pre-create content ---
    dc.b    "Welcome to IntuitionOS M9", 0x0D, 0x0A, 0
prog_doslib_data_end:
    align   8
prog_doslib_end:

; ---------------------------------------------------------------------------
; SHELL — interactive command shell (M9)
; ---------------------------------------------------------------------------
; Opens console.handler and dos.library via OpenLibrary.
; Reads lines from console, parses command, launches external programs via
; DOS_RUN, or prints "Unknown command\r\n".
;
; Data page layout:
;   0:   "console.handler\0"   (16 bytes)
;   16:  "dos.library\0"       (16 bytes, padded)
;   32:  "Shell ONLINE [Task\0"   (16 bytes, banner prefix)
;   48:  "IntuitionOS M9\r\n\0" (17 bytes + pad = 32 bytes)
;   80:  "1> \0"               (4 bytes + pad = 8 bytes)
;   88:  "Unknown command\r\n\0" (18 bytes + pad)
;   128: task_id               (8 bytes)
;   136: console_port          (8 bytes)
;   144: dos_port              (8 bytes)
;   152: reply_port            (8 bytes)
;   160: shared_buf_va         (8 bytes)
;   168: shared_buf_handle     (4 bytes + pad)
;   192: command name table    (5 x 8 bytes = 40 bytes)
;   232: command index table   (5 bytes)
;   240: line buffer           (128 bytes)

prog_shell:
    ; Header
    dc.l    IMG_MAGIC_LO, IMG_MAGIC_HI
    dc.l    prog_shell_code_end - prog_shell_code
    dc.l    prog_shell_data_end - prog_shell_data
    dc.l    0
    ds.b    12
prog_shell_code:

    ; =====================================================================
    ; Preamble: compute data page base (preemption-safe double-check)
    ; =====================================================================
    sub     sp, sp, #16
.sh_preamble:
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    store.q r1, 8(sp)
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    load.q  r28, 8(sp)
    bne     r1, r28, .sh_preamble
    move.l  r28, #USER_SLOT_STRIDE
    mulu    r28, r1, r28
    move.l  r29, #USER_DATA_BASE
    add     r29, r29, r28
    store.q r29, (sp)
    load.q  r1, 8(sp)
    move.l  r28, #USER_SLOT_STRIDE
    mulu    r28, r1, r28
    move.l  r29, #USER_DATA_BASE
    add     r29, r29, r28
    load.q  r28, (sp)
    bne     r29, r28, .sh_preamble
    store.q r29, (sp)
    load.q  r1, 8(sp)
    store.q r1, 128(r29)               ; data[128] = task_id

    ; =====================================================================
    ; OpenLibrary("console.handler", 0) with retry
    ; =====================================================================
.sh_open_con_retry:
    load.q  r29, (sp)
    move.q  r1, r29                     ; R1 = &data[0] = "console.handler"
    move.q  r2, r0                      ; version 0
    syscall #SYS_OPEN_LIBRARY
    load.q  r29, (sp)
    beqz    r2, .sh_open_con_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .sh_open_con_retry
.sh_open_con_ok:
    store.q r1, 136(r29)               ; data[136] = console_port

    ; =====================================================================
    ; OpenLibrary("dos.library", 0) with retry
    ; =====================================================================
.sh_open_dos_retry:
    load.q  r29, (sp)
    add     r1, r29, #16                ; R1 = &data[16] = "dos.library"
    move.q  r2, r0
    syscall #SYS_OPEN_LIBRARY
    load.q  r29, (sp)
    beqz    r2, .sh_open_dos_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .sh_open_dos_retry
.sh_open_dos_ok:
    store.q r1, 144(r29)               ; data[144] = dos_port

    ; =====================================================================
    ; CreatePort(anonymous, flags=0)
    ; =====================================================================
    move.q  r1, r0
    move.q  r2, r0
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    store.q r1, 152(r29)               ; data[152] = reply_port

    ; =====================================================================
    ; AllocMem(0x1000, MEMF_PUBLIC | MEMF_CLEAR)
    ; =====================================================================
    move.l  r1, #0x1000
    move.l  r2, #0x10001               ; MEMF_PUBLIC | MEMF_CLEAR
    syscall #SYS_ALLOC_MEM             ; R1=VA, R2=err, R3=share_handle
    load.q  r29, (sp)
    store.q r1, 160(r29)               ; data[160] = shared_buf_va
    store.l r3, 168(r29)               ; data[168] = shared_buf_handle

    ; =====================================================================
    ; Send "Shell ONLINE [Taskn]\r\n" banner
    ; =====================================================================
    add     r20, r29, #32              ; &data[32] = "Shell ONLINE [Task"
    jsr     .sh_send_string
    load.q  r29, (sp)
    ; task_id digit
.sh_ban_id_retry:
    load.q  r29, (sp)
    load.q  r3, 128(r29)
    add     r3, r3, #0x30               ; ASCII digit
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r1, 136(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .sh_ban_id_full
    bra     .sh_ban_bracket
.sh_ban_id_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .sh_ban_id_retry
.sh_ban_bracket:
    ; ']'
.sh_ban_brk_retry:
    move.l  r3, #0x5D
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 136(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .sh_ban_brk_full
    bra     .sh_ban_cr
.sh_ban_brk_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .sh_ban_brk_retry
.sh_ban_cr:
    ; '\r'
.sh_ban_cr_retry:
    move.l  r3, #0x0D
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 136(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .sh_ban_cr_full
    bra     .sh_ban_lf
.sh_ban_cr_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .sh_ban_cr_retry
.sh_ban_lf:
    ; '\n'
.sh_ban_lf_retry:
    move.l  r3, #0x0A
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 136(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .sh_ban_lf_full
    bra     .sh_ban_done
.sh_ban_lf_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .sh_ban_lf_retry
.sh_ban_done:

    ; =====================================================================
    ; Send "IntuitionOS M9\r\n" to console
    ; =====================================================================
    load.q  r29, (sp)
    add     r20, r29, #56              ; "IntuitionOS M9\r\n" at offset 56
    jsr     .sh_send_string

    ; =====================================================================
    ; Main loop
    ; =====================================================================
.sh_main_loop:
    ; Send prompt "1> "
    load.q  r29, (sp)
    add     r20, r29, #80
    jsr     .sh_send_string

    ; Send CON_READLINE request to console.handler (with ERR_FULL retry)
.sh_readline_retry:
    load.q  r29, (sp)
    move.l  r2, #CON_MSG_READLINE       ; type
    move.q  r3, r0                      ; data0 = 0
    move.q  r4, r0                      ; data1 = 0
    load.q  r5, 152(r29)               ; reply_port
    load.l  r6, 168(r29)               ; share_handle
    load.q  r1, 136(r29)               ; console_port
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .sh_readline_full
    bra     .sh_readline_sent
.sh_readline_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .sh_readline_retry
.sh_readline_sent:

    ; WaitPort(reply_port) → R2=data0=byte_count
    load.q  r1, 152(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    ; R2 = byte_count
    store.q r2, 8(sp)                  ; save byte_count to scratch

    ; Copy line from shared_buf_va to data[240..366], null-terminate
    load.q  r14, 160(r29)             ; r14 = shared_buf_va (source)
    add     r15, r29, #240             ; r15 = &data[240] (dest)
    load.q  r16, 8(sp)                ; r16 = byte_count
    move.l  r17, #0                    ; counter
.sh_copy_line:
    bge     r17, r16, .sh_copy_done
    move.l  r18, #126
    bge     r17, r18, .sh_copy_done
    add     r19, r14, r17
    load.b  r20, (r19)
    add     r19, r15, r17
    store.b r20, (r19)
    add     r17, r17, #1
    bra     .sh_copy_line
.sh_copy_done:
    ; Null-terminate
    add     r19, r15, r17
    store.b r0, (r19)

    ; Strip trailing CR/LF from line buffer
    add     r15, r29, #240             ; r15 = &data[240]
.sh_strip_trail:
    beqz    r17, .sh_strip_done
    sub     r18, r17, #1
    add     r19, r15, r18
    load.b  r20, (r19)
    move.l  r21, #0x0D
    beq     r20, r21, .sh_strip_char
    move.l  r21, #0x0A
    beq     r20, r21, .sh_strip_char
    bra     .sh_strip_done
.sh_strip_char:
    store.b r0, (r19)
    move.q  r17, r18
    bra     .sh_strip_trail
.sh_strip_done:

    ; Strip prompt "1> " from line if present (GUI terminal includes it)
    add     r15, r29, #240
    load.b  r20, (r15)
    move.l  r21, #0x31                  ; '1'
    bne     r20, r21, .sh_no_prompt_strip
    load.b  r20, 1(r15)
    move.l  r21, #0x3E                  ; '>'
    bne     r20, r21, .sh_no_prompt_strip
    load.b  r20, 2(r15)
    move.l  r21, #0x20                  ; ' '
    bne     r20, r21, .sh_no_prompt_strip
    ; Line starts with "1> " — skip 3 chars
    add     r15, r29, #243              ; data[240+3]
    ; Copy remainder back to data[240] (shift left by 3)
    add     r14, r29, #240
.sh_prompt_copy:
    load.b  r20, (r15)
    store.b r20, (r14)
    beqz    r20, .sh_no_prompt_strip
    add     r15, r15, #1
    add     r14, r14, #1
    bra     .sh_prompt_copy
.sh_no_prompt_strip:

    ; If line is empty, re-prompt
    add     r15, r29, #240
    load.b  r20, (r15)
    beqz    r20, .sh_main_loop

    ; =====================================================================
    ; Parse first word: scan for space or null in line buffer
    ; =====================================================================
    add     r15, r29, #240             ; r15 = line buffer start
    move.q  r16, r15                   ; r16 = scan pointer
.sh_scan_word:
    load.b  r17, (r16)
    beqz    r17, .sh_word_end
    move.l  r18, #0x20                 ; space
    beq     r17, r18, .sh_word_end
    add     r16, r16, #1
    bra     .sh_scan_word
.sh_word_end:
    ; r15 = start of word, r16 = end of word (points to space or null)
    sub     r17, r16, r15              ; r17 = word length

    ; =====================================================================
    ; Compare against command table (5 entries, case-insensitive)
    ; =====================================================================
    move.l  r18, #0                    ; r18 = command index (0..4)
.sh_cmd_loop:
    move.l  r19, #5
    bge     r18, r19, .sh_cmd_unknown

    ; Compute command name addr: data[192 + r18 * 8]
    lsl     r19, r18, #3
    add     r19, r19, #192
    add     r19, r19, r29              ; r19 = &cmd_name[i]

    ; Compare word_len chars case-insensitively
    ; First check cmd name length matches word length
    move.l  r20, #0                    ; char index
.sh_cmp_char:
    bge     r20, r17, .sh_cmp_end_check ; checked all word chars
    add     r21, r15, r20              ; &line[i]
    load.b  r22, (r21)                 ; line char
    add     r21, r19, r20              ; &cmd[i]
    load.b  r23, (r21)                 ; cmd char
    beqz    r23, .sh_cmd_next          ; cmd shorter than word → no match
    ; Uppercase both: if in 'a'-'z' range, clear bit 5 (AND with 0xDF)
    move.l  r24, #0x61                 ; 'a'
    move.l  r25, #0x7A                 ; 'z'
    blt     r22, r24, .sh_cmp_skip_upper1
    bgt     r22, r25, .sh_cmp_skip_upper1
    and     r22, r22, #0xDF
.sh_cmp_skip_upper1:
    blt     r23, r24, .sh_cmp_skip_upper2
    bgt     r23, r25, .sh_cmp_skip_upper2
    and     r23, r23, #0xDF
.sh_cmp_skip_upper2:
    bne     r22, r23, .sh_cmd_next     ; mismatch
    add     r20, r20, #1
    bra     .sh_cmp_char
.sh_cmp_end_check:
    ; Word matches so far; check that cmd_name[word_len] == 0
    add     r21, r19, r17
    load.b  r22, (r21)
    bnez    r22, .sh_cmd_next          ; cmd has more chars → no match
    bra     .sh_cmd_found
.sh_cmd_next:
    add     r18, r18, #1
    bra     .sh_cmd_loop

.sh_cmd_unknown:
    ; Send "Unknown command\r\n"
    load.q  r29, (sp)
    add     r20, r29, #88
    jsr     .sh_send_string
    bra     .sh_main_loop

.sh_cmd_found:
    ; r18 = command table index (0..4)
    ; Look up program_table index from data[232 + r18]
    add     r19, r29, #232
    add     r19, r19, r18
    load.b  r20, (r19)                 ; r20 = prog_table_index
    store.q r20, 8(sp)                 ; save prog index to scratch

    ; Copy args (rest of line after command+space) to shared_buf_va
    ; r16 points to space or null after command word
    load.b  r17, (r16)
    beqz    r17, .sh_no_args
    add     r16, r16, #1               ; skip space
.sh_no_args:
    ; r16 = start of args (or points to null)
    load.q  r14, 160(r29)             ; r14 = shared_buf_va
    move.l  r17, #0
.sh_copy_args:
    load.b  r18, (r16)
    store.b r18, (r14)
    beqz    r18, .sh_args_done
    add     r16, r16, #1
    add     r14, r14, #1
    add     r17, r17, #1
    move.l  r19, #DATA_ARGS_MAX
    blt     r17, r19, .sh_copy_args
    store.b r0, (r14)                  ; force null-terminate
.sh_args_done:

    ; Send DOS_RUN to dos.library
    load.q  r29, (sp)
    load.q  r3, 8(sp)                 ; data0 = prog_index
    move.l  r2, #DOS_RUN               ; type = DOS_RUN
    move.q  r4, r0                      ; data1 = 0
    load.q  r5, 152(r29)               ; reply_port
    load.l  r6, 168(r29)               ; share_handle
    load.q  r1, 144(r29)               ; dos_port
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)

    ; WaitPort(reply_port) for dos.library acknowledgement
    load.q  r1, 152(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)

    ; Yield 200 times to let command finish
    move.l  r20, #200
    store.q r20, 8(sp)
.sh_delay:
    load.q  r20, 8(sp)
    beqz    r20, .sh_delay_done
    sub     r20, r20, #1
    store.q r20, 8(sp)
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .sh_delay
.sh_delay_done:
    bra     .sh_main_loop

    ; =====================================================================
    ; send_string subroutine: R20 = string addr, console port from data[136]
    ; Clobbers R1-R6, R20. After return, R29 is valid (reloaded from sp).
    ; =====================================================================
; .sh_send_string: send null-terminated string at R20 to console.handler.
; Called via jsr — has its own 16-byte stack frame.
; Stack layout: [sp]=local R29, [sp+8]=R20 save, [sp+16]=return addr, [sp+24]=caller R29
; Clobbers R1-R6, R20, R28. R29 reloaded from caller frame.
.sh_send_string:
    sub     sp, sp, #16                 ; subroutine frame: [sp]=R29, [sp+8]=R20
    load.q  r29, 24(sp)                ; load R29 from caller's [sp] (skip 16 local + 8 retaddr)
    store.q r29, (sp)                   ; cache R29 in our frame
.sh_ss_loop:
    ; R20 must be valid here (set by caller or by post-PutMsg reload)
    ; Save R20 immediately, then re-read from saved copy for safety
    store.q r20, 8(sp)                  ; save R20 to scratch
    load.q  r20, 8(sp)                  ; reload (survives context switch at store.q boundary)
    load.b  r1, (r20)
    beqz    r1, .sh_ss_done
.sh_ss_retry:
    load.q  r20, 8(sp)
    load.b  r3, (r20)                   ; data0 = char
    move.l  r2, #0                      ; type = CON_MSG_CHAR
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 136(r29)               ; console_port
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .sh_ss_full
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .sh_ss_loop
.sh_ss_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .sh_ss_retry
.sh_ss_done:
    add     sp, sp, #16                 ; tear down subroutine frame
    rts

prog_shell_code_end:

prog_shell_data:
    ; --- Offset 0: "console.handler\0" (16 bytes) ---
    dc.b    "console.handler", 0
    ; --- Offset 16: "dos.library\0" + pad to 32 ---
    dc.b    "dos.library", 0, 0, 0, 0, 0
    ; --- Offset 32: "Shell ONLINE [Task \0" (20 bytes, ends at 52) ---
    dc.b    "Shell ONLINE [Task ", 0
    ds.b    4                           ; pad to offset 56
    ; --- Offset 56: "IntuitionOS M9\r\n\0" (17 bytes, ends at 73) ---
    dc.b    "IntuitionOS M9", 0x0D, 0x0A, 0
    ds.b    7                           ; pad to offset 80
    ; --- Offset 80: "1> \0" (4 bytes) + pad to 88 ---
    dc.b    "1> ", 0
    ds.b    4                           ; pad to offset 88
    ; --- Offset 88: "Unknown command\r\n\0" (18 bytes, ends at 106) + pad to 128 ---
    dc.b    "Unknown command", 0x0D, 0x0A, 0
    ds.b    22                          ; pad to offset 128
    ; --- Offset 128: task_id (8 bytes) ---
    ds.b    8
    ; --- Offset 136: console_port (8 bytes) ---
    ds.b    8
    ; --- Offset 144: dos_port (8 bytes) ---
    ds.b    8
    ; --- Offset 152: reply_port (8 bytes) ---
    ds.b    8
    ; --- Offset 160: shared_buf_va (8 bytes) ---
    ds.b    8
    ; --- Offset 168: shared_buf_handle (4 bytes) + pad ---
    ds.b    8
    ; --- Offset 176: padding to 192 ---
    ds.b    16
    ; --- Offset 192: command name table (5 x 8 bytes) ---
    dc.b    "VERSION", 0               ; 192 (8 bytes)
    dc.b    "AVAIL", 0, 0, 0           ; 200 (8 bytes)
    dc.b    "DIR", 0, 0, 0, 0, 0       ; 208 (8 bytes)
    dc.b    "TYPE", 0, 0, 0, 0         ; 216 (8 bytes)
    dc.b    "ECHO", 0, 0, 0, 0         ; 224 (8 bytes)
    ; --- Offset 232: command index table (5 bytes) ---
    dc.b    3                           ; VERSION = prog_table_index 3
    dc.b    4                           ; AVAIL = prog_table_index 4
    dc.b    5                           ; DIR = prog_table_index 5
    dc.b    6                           ; TYPE = prog_table_index 6
    dc.b    7                           ; ECHO = prog_table_index 7
    ds.b    3                           ; pad to offset 240
    ; --- Offset 240: line buffer (128 bytes) ---
    ds.b    128
prog_shell_data_end:
    align   8
prog_shell_end:

; ---------------------------------------------------------------------------
; VERSION — display system version string
; ---------------------------------------------------------------------------
; Opens console.handler, sends version string, exits.

prog_version:
    ; Header
    dc.l    IMG_MAGIC_LO, IMG_MAGIC_HI
    dc.l    prog_version_code_end - prog_version_code
    dc.l    prog_version_data_end - prog_version_data
    dc.l    0
    ds.b    12
prog_version_code:

    ; === DEBUG: emit 'V' to confirm task launched ===
    move.l  r1, #0x56
    syscall #SYS_DEBUG_PUTCHAR

    ; === Preamble ===
    sub     sp, sp, #16
.ver_preamble:
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    store.q r1, 8(sp)
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    load.q  r28, 8(sp)
    bne     r1, r28, .ver_preamble
    move.l  r28, #USER_SLOT_STRIDE
    mulu    r28, r1, r28
    move.l  r29, #USER_DATA_BASE
    add     r29, r29, r28
    store.q r29, (sp)
    load.q  r1, 8(sp)
    move.l  r28, #USER_SLOT_STRIDE
    mulu    r28, r1, r28
    move.l  r29, #USER_DATA_BASE
    add     r29, r29, r28
    load.q  r28, (sp)
    bne     r29, r28, .ver_preamble
    store.q r29, (sp)

    ; === OpenLibrary("console.handler", 0) ===
.ver_open_retry:
    load.q  r29, (sp)
    move.q  r1, r29                     ; &data[0] = "console.handler"
    move.l  r2, #0
    syscall #SYS_OPEN_LIBRARY
    load.q  r29, (sp)
    beqz    r2, .ver_open_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .ver_open_retry
.ver_open_ok:
    store.q r1, 16(r29)                ; data[16] = console_port

    ; === Send version string ===
    add     r20, r29, #32              ; &data[32] = "IntuitionOS 0.9 ..."
    jsr     .ver_send_string

    ; === ExitTask ===
    move.q  r1, r0
    syscall #SYS_EXIT_TASK

    ; === send_string subroutine ===
.ver_send_string:
    sub     sp, sp, #16                 ; subroutine frame: [sp]=R29, [sp+8]=R20
    load.q  r29, 24(sp)                ; load R29 from caller's [sp] (skip 16 local + 8 retaddr)
    store.q r29, (sp)                   ; cache R29 in our frame
.ver_ss_loop:
    load.b  r1, (r20)
    beqz    r1, .ver_ss_done
    store.q r20, 8(sp)
.ver_ss_retry:
    load.q  r20, 8(sp)
    load.b  r3, (r20)
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 16(r29)               ; console_port
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .ver_ss_full
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .ver_ss_loop
.ver_ss_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .ver_ss_retry
.ver_ss_done:
    add     sp, sp, #16                 ; tear down subroutine frame
    rts

prog_version_code_end:

prog_version_data:
    ; --- Offset 0: "console.handler\0" (16 bytes) ---
    dc.b    "console.handler", 0
    ; --- Offset 16: console_port (8 bytes) ---
    ds.b    8
    ; --- Offset 24: padding to 32 ---
    ds.b    8
    ; --- Offset 32: version string ---
    dc.b    "IntuitionOS 0.9 (exec.library M9)", 0x0D, 0x0A, 0
prog_version_data_end:
    align   8
prog_version_end:

; ---------------------------------------------------------------------------
; AVAIL — display memory availability
; ---------------------------------------------------------------------------
; Opens console.handler, queries total/free pages, prints KB values.

prog_avail:
    ; Header
    dc.l    IMG_MAGIC_LO, IMG_MAGIC_HI
    dc.l    prog_avail_code_end - prog_avail_code
    dc.l    prog_avail_data_end - prog_avail_data
    dc.l    0
    ds.b    12
prog_avail_code:

    ; === Preamble ===
    sub     sp, sp, #16
.av_preamble:
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    store.q r1, 8(sp)
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    load.q  r28, 8(sp)
    bne     r1, r28, .av_preamble
    move.l  r28, #USER_SLOT_STRIDE
    mulu    r28, r1, r28
    move.l  r29, #USER_DATA_BASE
    add     r29, r29, r28
    store.q r29, (sp)
    load.q  r1, 8(sp)
    move.l  r28, #USER_SLOT_STRIDE
    mulu    r28, r1, r28
    move.l  r29, #USER_DATA_BASE
    add     r29, r29, r28
    load.q  r28, (sp)
    bne     r29, r28, .av_preamble
    store.q r29, (sp)

    ; === OpenLibrary("console.handler", 0) ===
.av_open_retry:
    load.q  r29, (sp)
    move.q  r1, r29
    move.l  r2, #0
    syscall #SYS_OPEN_LIBRARY
    load.q  r29, (sp)
    beqz    r2, .av_open_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .av_open_retry
.av_open_ok:
    store.q r1, 64(r29)                ; data[64] = console_port

    ; === Send "Total: " ===
    add     r20, r29, #16
    jsr     .av_send_string

    ; === GetSysInfo(TOTAL_PAGES) → multiply by 4 for KB ===
    move.l  r1, #SYSINFO_TOTAL_PAGES
    syscall #SYS_GET_SYS_INFO
    load.q  r29, (sp)
    lsl     r1, r1, #2                 ; pages * 4 = KB (4KB pages)
    ; Format and send decimal number
    store.q r1, 8(sp)                  ; save value
    jsr     .av_print_number

    ; === Send " KB  Free: " ===
    load.q  r29, (sp)
    add     r20, r29, #24
    jsr     .av_send_string

    ; === GetSysInfo(FREE_PAGES) → multiply by 4 for KB ===
    move.l  r1, #SYSINFO_FREE_PAGES
    syscall #SYS_GET_SYS_INFO
    load.q  r29, (sp)
    lsl     r1, r1, #2                 ; pages * 4 = KB
    store.q r1, 8(sp)
    jsr     .av_print_number

    ; === Send " KB\r\n" ===
    load.q  r29, (sp)
    add     r20, r29, #36
    jsr     .av_send_string

    ; === ExitTask ===
    move.q  r1, r0
    syscall #SYS_EXIT_TASK

    ; =================================================================
    ; print_number: print decimal value from 8(sp) to console
    ; Uses data[80..95] as digit scratch buffer
    ; =================================================================
.av_print_number:
    sub     sp, sp, #16                 ; subroutine frame: [sp]=R29, [sp+8]=scratch
    load.q  r29, 24(sp)                ; load R29 from caller's [sp] (skip 16 local + 8 retaddr)
    store.q r29, (sp)                   ; cache R29 in our frame
    load.q  r14, 32(sp)                ; r14 = value (caller's 8(sp) = local+16 + retaddr+8 + 8 = 32)
    add     r15, r29, #80              ; r15 = scratch buffer base
    add     r16, r15, #15             ; r16 = write pointer (end of buffer)
    store.b r0, (r16)                  ; null-terminate
    ; Special case: value == 0
    bnez    r14, .av_divloop
    sub     r16, r16, #1
    move.l  r17, #0x30
    store.b r17, (r16)
    bra     .av_send_digits
.av_divloop:
    beqz    r14, .av_send_digits
    ; r14 / 10: repeated subtraction (simple, small numbers)
    move.q  r17, r0                    ; quotient
    move.q  r18, r14                   ; remainder
.av_div10:
    move.l  r19, #10
    blt     r18, r19, .av_div10_done
    sub     r18, r18, #10
    add     r17, r17, #1
    bra     .av_div10
.av_div10_done:
    ; r17 = quotient, r18 = remainder (digit)
    add     r18, r18, #0x30            ; ASCII digit
    sub     r16, r16, #1
    store.b r18, (r16)
    move.q  r14, r17                   ; value = quotient
    bra     .av_divloop
.av_send_digits:
    ; r16 points to first digit
    move.q  r20, r16
    jsr     .av_send_string
    add     sp, sp, #16                 ; tear down subroutine frame
    rts

    ; === send_string subroutine ===
.av_send_string:
    sub     sp, sp, #16                 ; subroutine frame: [sp]=R29, [sp+8]=R20
    load.q  r29, 24(sp)                ; load R29 from caller's [sp] (skip 16 local + 8 retaddr)
    store.q r29, (sp)                   ; cache R29 in our frame
.av_ss_loop:
    load.b  r1, (r20)
    beqz    r1, .av_ss_done
    store.q r20, 8(sp)
.av_ss_retry:
    load.q  r20, 8(sp)
    load.b  r3, (r20)
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 64(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .av_ss_full
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .av_ss_loop
.av_ss_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .av_ss_retry
.av_ss_done:
    add     sp, sp, #16                 ; tear down subroutine frame
    rts

prog_avail_code_end:

prog_avail_data:
    ; --- Offset 0: "console.handler\0" (16 bytes) ---
    dc.b    "console.handler", 0
    ; --- Offset 16: "Total: \0" (8 bytes) ---
    dc.b    "Total: ", 0
    ; --- Offset 24: " KB  Free: \0" (12 bytes) ---
    dc.b    " KB  Free: ", 0
    ; --- Offset 36: " KB\r\n\0" + pad to 64 ---
    dc.b    " KB", 0x0D, 0x0A, 0
    ds.b    21                          ; pad to offset 64
    ; --- Offset 64: console_port (8 bytes) ---
    ds.b    8
    ; --- Offset 72: padding to 80 ---
    ds.b    8
    ; --- Offset 80: digit scratch buffer (16 bytes) ---
    ds.b    16
prog_avail_data_end:
    align   8
prog_avail_end:

; ---------------------------------------------------------------------------
; DIR — list RAM: filesystem contents
; ---------------------------------------------------------------------------
; Opens console + dos, sends DOS_DIR, prints result or "RAM: is empty".

prog_dir:
    ; Header
    dc.l    IMG_MAGIC_LO, IMG_MAGIC_HI
    dc.l    prog_dir_code_end - prog_dir_code
    dc.l    prog_dir_data_end - prog_dir_data
    dc.l    0
    ds.b    12
prog_dir_code:

    ; === Preamble ===
    sub     sp, sp, #16
.dir_preamble:
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    store.q r1, 8(sp)
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    load.q  r28, 8(sp)
    bne     r1, r28, .dir_preamble
    move.l  r28, #USER_SLOT_STRIDE
    mulu    r28, r1, r28
    move.l  r29, #USER_DATA_BASE
    add     r29, r29, r28
    store.q r29, (sp)
    load.q  r1, 8(sp)
    move.l  r28, #USER_SLOT_STRIDE
    mulu    r28, r1, r28
    move.l  r29, #USER_DATA_BASE
    add     r29, r29, r28
    load.q  r28, (sp)
    bne     r29, r28, .dir_preamble
    store.q r29, (sp)

    ; === OpenLibrary("console.handler", 0) ===
.dir_open_con_retry:
    load.q  r29, (sp)
    move.q  r1, r29
    move.l  r2, #0
    syscall #SYS_OPEN_LIBRARY
    load.q  r29, (sp)
    beqz    r2, .dir_open_con_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .dir_open_con_retry
.dir_open_con_ok:
    store.q r1, 64(r29)                ; data[64] = console_port

    ; === OpenLibrary("dos.library", 0) ===
.dir_open_dos_retry:
    load.q  r29, (sp)
    add     r1, r29, #16
    move.l  r2, #0
    syscall #SYS_OPEN_LIBRARY
    load.q  r29, (sp)
    beqz    r2, .dir_open_dos_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .dir_open_dos_retry
.dir_open_dos_ok:
    store.q r1, 72(r29)                ; data[72] = dos_port

    ; === CreatePort(anonymous) ===
    move.q  r1, r0
    move.q  r2, r0
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    store.q r1, 80(r29)                ; data[80] = reply_port

    ; === AllocMem(0x1000, MEMF_PUBLIC | MEMF_CLEAR) ===
    move.l  r1, #0x1000
    move.l  r2, #0x10001
    syscall #SYS_ALLOC_MEM
    load.q  r29, (sp)
    store.q r1, 88(r29)                ; data[88] = shared_buf_va
    store.l r3, 96(r29)                ; data[96] = share_handle

    ; === Send DOS_DIR to dos.library ===
    move.l  r2, #DOS_DIR                ; type = DOS_DIR
    move.q  r3, r0                      ; data0 = 0
    move.q  r4, r0                      ; data1 = 0
    load.q  r5, 80(r29)                ; reply_port
    load.l  r6, 96(r29)                ; share_handle
    load.q  r1, 72(r29)                ; dos_port
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)

    ; === WaitPort(reply_port) → R2=data0=bytes_written ===
    load.q  r1, 80(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    ; R2 = bytes_written
    store.q r2, 8(sp)                  ; save bytes_written

    beqz    r2, .dir_empty

    ; === Send bytes_written chars from shared_buf_va ===
    load.q  r14, 88(r29)              ; r14 = shared_buf_va
    load.q  r15, 8(sp)                ; r15 = bytes_written
    move.l  r16, #0
.dir_print_loop:
    bge     r16, r15, .dir_done
    add     r17, r14, r16
    load.b  r18, (r17)
    store.q r16, 8(sp)                 ; save counter
    ; Send char
.dir_char_retry:
    load.q  r29, (sp)
    load.q  r16, 8(sp)
    load.q  r14, 88(r29)
    add     r17, r14, r16
    load.b  r3, (r17)                  ; data0 = char
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r1, 64(r29)               ; console_port
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .dir_char_full
    load.q  r16, 8(sp)
    add     r16, r16, #1
    store.q r16, 8(sp)
    load.q  r29, (sp)
    load.q  r15, 88(r29)              ; reload bytes count — use scratch differently
    ; Actually we need bytes_written still, recalculate:
    ; We stored bytes_written earlier but clobbered 8(sp) with counter.
    ; Let's use the fact that shared_buf ends with null; just check char != 0
    load.b  r18, (r17)
    bnez    r18, .dir_print_loop
    bra     .dir_done
.dir_char_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .dir_char_retry

.dir_empty:
    ; Send "RAM: is empty\r\n"
    load.q  r29, (sp)
    add     r20, r29, #32
    jsr     .dir_send_string
    bra     .dir_done

.dir_done:
    move.q  r1, r0
    syscall #SYS_EXIT_TASK

    ; === send_string subroutine ===
.dir_send_string:
    sub     sp, sp, #16                 ; subroutine frame: [sp]=R29, [sp+8]=R20
    load.q  r29, 24(sp)                ; load R29 from caller's [sp] (skip 16 local + 8 retaddr)
    store.q r29, (sp)                   ; cache R29 in our frame
.dir_ss_loop:
    load.b  r1, (r20)
    beqz    r1, .dir_ss_done
    store.q r20, 8(sp)
.dir_ss_retry:
    load.q  r20, 8(sp)
    load.b  r3, (r20)
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 64(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .dir_ss_full
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .dir_ss_loop
.dir_ss_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .dir_ss_retry
.dir_ss_done:
    add     sp, sp, #16                 ; tear down subroutine frame
    rts

prog_dir_code_end:

prog_dir_data:
    ; --- Offset 0: "console.handler\0" (16 bytes) ---
    dc.b    "console.handler", 0
    ; --- Offset 16: "dos.library\0" + pad to 32 ---
    dc.b    "dos.library", 0, 0, 0, 0, 0
    ; --- Offset 32: "RAM: is empty\r\n\0" + pad to 64 ---
    dc.b    "RAM: is empty", 0x0D, 0x0A, 0
    ds.b    16                          ; pad to offset 64
    ; --- Offset 64: console_port (8 bytes) ---
    ds.b    8
    ; --- Offset 72: dos_port (8 bytes) ---
    ds.b    8
    ; --- Offset 80: reply_port (8 bytes) ---
    ds.b    8
    ; --- Offset 88: shared_buf_va (8 bytes) ---
    ds.b    8
    ; --- Offset 96: share_handle (4 bytes) + pad ---
    ds.b    8
prog_dir_data_end:
    align   8
prog_dir_end:

; ---------------------------------------------------------------------------
; TYPE — display contents of a RAM: file
; ---------------------------------------------------------------------------
; Opens console + dos, strips "RAM:" prefix, opens/reads/prints file.

prog_type:
    ; Header
    dc.l    IMG_MAGIC_LO, IMG_MAGIC_HI
    dc.l    prog_type_code_end - prog_type_code
    dc.l    prog_type_data_end - prog_type_data
    dc.l    0
    ds.b    12
prog_type_code:

    ; === Preamble ===
    sub     sp, sp, #16
.typ_preamble:
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    store.q r1, 8(sp)
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    load.q  r28, 8(sp)
    bne     r1, r28, .typ_preamble
    move.l  r28, #USER_SLOT_STRIDE
    mulu    r28, r1, r28
    move.l  r29, #USER_DATA_BASE
    add     r29, r29, r28
    store.q r29, (sp)
    load.q  r1, 8(sp)
    move.l  r28, #USER_SLOT_STRIDE
    mulu    r28, r1, r28
    move.l  r29, #USER_DATA_BASE
    add     r29, r29, r28
    load.q  r28, (sp)
    bne     r29, r28, .typ_preamble
    store.q r29, (sp)

    ; === Read args from data[DATA_ARGS_OFFSET] ===
    add     r14, r29, #DATA_ARGS_OFFSET ; r14 = args pointer
    ; Check if args is empty
    load.b  r15, (r14)
    beqz    r15, .typ_no_file

    ; === Strip "RAM:" prefix (case-insensitive) ===
    ; Compare first 4 bytes against "RAM:"
    load.b  r15, (r14)
    ; Uppercase it
    move.l  r16, #0x61
    move.l  r17, #0x7A
    blt     r15, r16, .typ_chk_r
    bgt     r15, r17, .typ_chk_r
    and     r15, r15, #0xDF
.typ_chk_r:
    move.l  r16, #0x52                 ; 'R'
    bne     r15, r16, .typ_no_strip
    add     r18, r14, #1
    load.b  r15, (r18)
    move.l  r16, #0x61
    move.l  r17, #0x7A
    blt     r15, r16, .typ_chk_a
    bgt     r15, r17, .typ_chk_a
    and     r15, r15, #0xDF
.typ_chk_a:
    move.l  r16, #0x41                 ; 'A'
    bne     r15, r16, .typ_no_strip
    add     r18, r14, #2
    load.b  r15, (r18)
    move.l  r16, #0x61
    move.l  r17, #0x7A
    blt     r15, r16, .typ_chk_m
    bgt     r15, r17, .typ_chk_m
    and     r15, r15, #0xDF
.typ_chk_m:
    move.l  r16, #0x4D                 ; 'M'
    bne     r15, r16, .typ_no_strip
    add     r18, r14, #3
    load.b  r15, (r18)
    move.l  r16, #0x3A                 ; ':'
    bne     r15, r16, .typ_no_strip
    add     r14, r14, #4               ; strip "RAM:" prefix
.typ_no_strip:
    ; r14 = filename pointer (in data page args area)
    ; Save filename pointer
    store.q r14, 8(sp)

    ; === OpenLibrary("console.handler", 0) ===
.typ_open_con_retry:
    load.q  r29, (sp)
    move.q  r1, r29
    move.l  r2, #0
    syscall #SYS_OPEN_LIBRARY
    load.q  r29, (sp)
    beqz    r2, .typ_open_con_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .typ_open_con_retry
.typ_open_con_ok:
    store.q r1, 64(r29)                ; data[64] = console_port

    ; === OpenLibrary("dos.library", 0) ===
.typ_open_dos_retry:
    load.q  r29, (sp)
    add     r1, r29, #16
    move.l  r2, #0
    syscall #SYS_OPEN_LIBRARY
    load.q  r29, (sp)
    beqz    r2, .typ_open_dos_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .typ_open_dos_retry
.typ_open_dos_ok:
    store.q r1, 72(r29)                ; data[72] = dos_port

    ; === CreatePort(anonymous) ===
    move.q  r1, r0
    move.q  r2, r0
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    store.q r1, 80(r29)                ; data[80] = reply_port

    ; === AllocMem(0x1000, MEMF_PUBLIC | MEMF_CLEAR) ===
    move.l  r1, #0x1000
    move.l  r2, #0x10001
    syscall #SYS_ALLOC_MEM
    load.q  r29, (sp)
    store.q r1, 88(r29)                ; data[88] = shared_buf_va
    store.l r3, 96(r29)                ; data[96] = share_handle

    ; === Copy bare filename to shared_buf_va ===
    load.q  r14, 8(sp)                ; filename pointer (saved earlier)
    load.q  r15, 88(r29)              ; shared_buf_va
    move.l  r16, #0
.typ_copy_name:
    add     r17, r14, r16
    load.b  r18, (r17)
    add     r17, r15, r16
    store.b r18, (r17)
    beqz    r18, .typ_copy_name_done
    add     r16, r16, #1
    move.l  r19, #255
    blt     r16, r19, .typ_copy_name
    add     r17, r15, r16
    store.b r0, (r17)                  ; force null-terminate
.typ_copy_name_done:

    ; === DOS_OPEN(READ) ===
    load.q  r29, (sp)
    move.l  r2, #DOS_OPEN               ; type = DOS_OPEN
    move.l  r3, #DOS_MODE_READ           ; data0 = mode READ
    move.q  r4, r0                      ; data1 = 0
    load.q  r5, 80(r29)                ; reply_port
    load.l  r6, 96(r29)                ; share_handle (has filename)
    load.q  r1, 72(r29)                ; dos_port
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)

    ; === WaitPort → R1=type(error), R2=data0(handle) ===
    load.q  r1, 80(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    ; R1 = type (0=ok, non-zero=error), R2 = data0 (handle or error code)
    bnez    r1, .typ_not_found
    ; Save file handle
    store.q r2, 104(r29)               ; data[104] = file_handle

    ; === DOS_READ ===
    move.l  r2, #DOS_READ               ; type = DOS_READ
    load.q  r3, 104(r29)               ; data0 = handle
    move.l  r4, #4096                   ; data1 = max bytes
    load.q  r5, 80(r29)                ; reply_port
    load.l  r6, 96(r29)                ; share_handle (buf)
    load.q  r1, 72(r29)                ; dos_port
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)

    ; === WaitPort → R2=data0=bytes_read ===
    load.q  r1, 80(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    store.q r2, 112(r29)               ; save bytes_read to data[112]

    ; === Send bytes from shared_buf_va to console ===
    move.l  r16, #0
    store.q r16, 8(sp)                 ; loop counter in 8(sp)
.typ_print_loop:
    load.q  r29, (sp)
    load.q  r16, 8(sp)                ; reload counter
    load.q  r15, 112(r29)             ; reload bytes_read from data[112]
    bge     r16, r15, .typ_close
.typ_print_retry:
    load.q  r29, (sp)
    load.q  r16, 8(sp)
    load.q  r14, 88(r29)
    add     r17, r14, r16
    load.b  r3, (r17)
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r1, 64(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .typ_print_full
    load.q  r16, 8(sp)
    add     r16, r16, #1
    store.q r16, 8(sp)
    bra     .typ_print_loop
.typ_print_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .typ_print_retry

.typ_close:
    ; === DOS_CLOSE ===
    load.q  r29, (sp)
    move.l  r2, #DOS_CLOSE
    load.q  r3, 104(r29)               ; data0 = handle
    move.q  r4, r0
    load.q  r5, 80(r29)                ; reply_port
    move.q  r6, r0                      ; no share needed
    load.q  r1, 72(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)

    load.q  r1, 80(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)

    ; === ExitTask ===
    move.q  r1, r0
    syscall #SYS_EXIT_TASK

.typ_not_found:
    ; Send "File not found\r\n"
    load.q  r29, (sp)
    add     r20, r29, #40
    jsr     .typ_send_string
    move.q  r1, r0
    syscall #SYS_EXIT_TASK

.typ_no_file:
    ; No filename given — just exit
    move.q  r1, r0
    syscall #SYS_EXIT_TASK

    ; === send_string subroutine ===
.typ_send_string:
    sub     sp, sp, #16                 ; subroutine frame: [sp]=R29, [sp+8]=R20
    load.q  r29, 24(sp)                ; load R29 from caller's [sp] (skip 16 local + 8 retaddr)
    store.q r29, (sp)                   ; cache R29 in our frame
.typ_ss_loop:
    load.b  r1, (r20)
    beqz    r1, .typ_ss_done
    store.q r20, 8(sp)
.typ_ss_retry:
    load.q  r20, 8(sp)
    load.b  r3, (r20)
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 64(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .typ_ss_full
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .typ_ss_loop
.typ_ss_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .typ_ss_retry
.typ_ss_done:
    add     sp, sp, #16                 ; tear down subroutine frame
    rts

prog_type_code_end:

prog_type_data:
    ; --- Offset 0: "console.handler\0" (16 bytes) ---
    dc.b    "console.handler", 0
    ; --- Offset 16: "dos.library\0" + pad to 32 ---
    dc.b    "dos.library", 0, 0, 0, 0, 0
    ; --- Offset 32: "RAM:\0" + pad to 40 ---
    dc.b    "RAM:", 0, 0, 0, 0
    ; --- Offset 40: "File not found\r\n\0" + pad to 64 ---
    dc.b    "File not found", 0x0D, 0x0A, 0
    ds.b    7                           ; pad to offset 64
    ; --- Offset 64: console_port (8 bytes) ---
    ds.b    8
    ; --- Offset 72: dos_port (8 bytes) ---
    ds.b    8
    ; --- Offset 80: reply_port (8 bytes) ---
    ds.b    8
    ; --- Offset 88: shared_buf_va (8 bytes) ---
    ds.b    8
    ; --- Offset 96: share_handle (4 bytes) + pad ---
    ds.b    8
    ; --- Offset 104: file_handle (8 bytes) ---
    ds.b    8
    ; --- Offset 112: bytes_read (8 bytes) ---
    ds.b    8
prog_type_data_end:
    align   8
prog_type_end:

; ---------------------------------------------------------------------------
; ECHO — echo arguments to console
; ---------------------------------------------------------------------------
; Opens console.handler, reads args from data[DATA_ARGS_OFFSET], sends to
; console, appends \r\n, exits.

prog_echo_cmd:
    ; Header
    dc.l    IMG_MAGIC_LO, IMG_MAGIC_HI
    dc.l    prog_echo_cmd_code_end - prog_echo_cmd_code
    dc.l    prog_echo_cmd_data_end - prog_echo_cmd_data
    dc.l    0
    ds.b    12
prog_echo_cmd_code:

    ; === Preamble ===
    sub     sp, sp, #16
.echo_preamble:
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    store.q r1, 8(sp)
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    load.q  r28, 8(sp)
    bne     r1, r28, .echo_preamble
    move.l  r28, #USER_SLOT_STRIDE
    mulu    r28, r1, r28
    move.l  r29, #USER_DATA_BASE
    add     r29, r29, r28
    store.q r29, (sp)
    load.q  r1, 8(sp)
    move.l  r28, #USER_SLOT_STRIDE
    mulu    r28, r1, r28
    move.l  r29, #USER_DATA_BASE
    add     r29, r29, r28
    load.q  r28, (sp)
    bne     r29, r28, .echo_preamble
    store.q r29, (sp)

    ; === OpenLibrary("console.handler", 0) ===
.echo_open_retry:
    load.q  r29, (sp)
    move.q  r1, r29
    move.l  r2, #0
    syscall #SYS_OPEN_LIBRARY
    load.q  r29, (sp)
    beqz    r2, .echo_open_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .echo_open_retry
.echo_open_ok:
    store.q r1, 16(r29)                ; data[16] = console_port

    ; === Read args from data[DATA_ARGS_OFFSET] and send to console ===
    add     r20, r29, #DATA_ARGS_OFFSET
    jsr     .echo_send_string

    ; === Send \r\n ===
    load.q  r29, (sp)
.echo_cr_retry:
    move.l  r3, #0x0D
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 16(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .echo_cr_full
    bra     .echo_lf
.echo_cr_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .echo_cr_retry
.echo_lf:
.echo_lf_retry:
    move.l  r3, #0x0A
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 16(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .echo_lf_full
    bra     .echo_exit
.echo_lf_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .echo_lf_retry

.echo_exit:
    move.q  r1, r0
    syscall #SYS_EXIT_TASK

    ; === send_string subroutine ===
.echo_send_string:
    sub     sp, sp, #16                 ; subroutine frame: [sp]=R29, [sp+8]=R20
    load.q  r29, 24(sp)                ; load R29 from caller's [sp] (skip 16 local + 8 retaddr)
    store.q r29, (sp)                   ; cache R29 in our frame
.echo_ss_loop:
    load.b  r1, (r20)
    beqz    r1, .echo_ss_done
    store.q r20, 8(sp)
.echo_ss_retry:
    load.q  r20, 8(sp)
    load.b  r3, (r20)
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 16(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .echo_ss_full
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .echo_ss_loop
.echo_ss_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .echo_ss_retry
.echo_ss_done:
    add     sp, sp, #16                 ; tear down subroutine frame
    rts

prog_echo_cmd_code_end:

prog_echo_cmd_data:
    ; --- Offset 0: "console.handler\0" (16 bytes) ---
    dc.b    "console.handler", 0
    ; --- Offset 16: console_port (8 bytes) ---
    ds.b    8
prog_echo_cmd_data_end:
    align   8
prog_echo_cmd_end:

; ============================================================================
; Data: Strings
; ============================================================================

boot_banner:
    dc.b    "exec.library M9 boot", 0x0D, 0x0A, 0
    align   4

fault_msg_prefix:
    dc.b    "GURU MEDITATION cause=", 0
    align   4

fault_msg_pc:
    dc.b    " PC=", 0
    align   4

fault_msg_addr:
    dc.b    " ADDR=", 0
    align   4

deadlock_msg:
    dc.b    "DEADLOCK: no runnable tasks", 0x0D, 0x0A, 0
    align   4

fault_msg_task:
    dc.b    " task=", 0
    align   4

panic_msg:
    dc.b    "KERNEL PANIC: ", 0
    align   4

no_tasks_msg:
    dc.b    "PANIC: no programs loaded", 0x0D, 0x0A, 0
    align   4

boot_fail_msg:
    dc.b    "PANIC: boot program failed", 0x0D, 0x0A, 0
    align   4

; ============================================================================
; End of IExec M8
; ============================================================================
