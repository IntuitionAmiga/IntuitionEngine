; ============================================================================
; exec.library - IE64 Microkernel Nucleus (M10: DOS-loaded programs + assigns + Startup-Sequence)
; ============================================================================
;
; Amiga Exec-inspired protected microkernel for the IE64 CPU.
; M10: DOS namespace execution, assigns (C:, S:, RAM:), Startup-Sequence.
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

    ; 3b. Add supervisor-only mappings for all user pages (8 tasks × USER_VPN_STRIDE pages)
    ; This lets the kernel access any task's code/stack/data for CreateTask etc.
    move.l  r4, #0                      ; task counter
.kern_user_map:
    move.l  r6, #MAX_TASKS
    bge     r4, r6, .kern_user_map_done
    ; Compute base VPN for task i: USER_CODE_VPN_BASE + i * USER_VPN_STRIDE
    move.l  r7, #USER_VPN_STRIDE
    mulu    r7, r4, r7
    add     r7, r7, #USER_CODE_VPN_BASE ; r7 = base VPN for this task
    ; Map all USER_VPN_STRIDE pages per task
    move.l  r9, #0
.kern_slot_map:
    move.l  r11, #USER_VPN_STRIDE
    bge     r9, r11, .kern_slot_done
    add     r8, r7, r9
    lsl     r3, r8, #13
    or      r3, r3, #0x07
    lsl     r5, r8, #3
    add     r5, r5, r2
    store.q r3, (r5)
    add     r9, r9, #1
    bra     .kern_slot_map
.kern_slot_done:
    add     r4, r4, #1
    bra     .kern_user_map
.kern_user_map_done:

    ; 3b'. Extend kernel PT: identity-map task PT region
    ;      (USER_PT_BASE .. USER_PT_BASE + MAX_TASKS * USER_SLOT_STRIDE)
    ;      as supervisor P|R|W. Required because USER_PT_BASE = $680000 is
    ;      outside the kernel-mapped range (pages 0..KERN_PAGES-1, phase 3a)
    ;      AND outside the user-slot range (pages USER_CODE_VPN_BASE..,
    ;      phase 3b), so it must be mapped explicitly here so the kernel
    ;      can read/write task page tables after the MMU is enabled.
    move.l  r2, #KERN_PAGE_TABLE
    move.l  r4, #USER_PT_PAGE_BASE       ; start page (= USER_PT_BASE >> 12)
    move.l  r6, #USER_PT_PAGE_END        ; end page exclusive
.kern_userpt_map:
    lsl     r3, r4, #13                  ; PPN << 13
    or      r3, r3, #0x07                ; P|R|W (supervisor only)
    lsl     r5, r4, #3                   ; offset in PT = page * 8
    add     r5, r5, r2
    store.q r3, (r5)
    add     r4, r4, #1
    blt     r4, r6, .kern_userpt_map

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
    ; 6b. Initialize port slots (all invalid, M12: 8 ports x 168 bytes)
    ; ---------------------------------------------------------------
    move.l  r2, #KERN_DATA_BASE
    add     r2, r2, #KD_PORT_BASE       ; R2 = &port[0]
    move.l  r4, #0
.port_init_loop:
    store.b r0, KD_PORT_VALID(r2)       ; valid = 0
    store.b r0, KD_PORT_FLAGS(r2)       ; flags = 0
    ; Zero name field (PORT_NAME_LEN = 32 bytes = 4 quad-words)
    store.q r0, KD_PORT_NAME(r2)
    add     r6, r2, #KD_PORT_NAME
    add     r6, r6, #8
    store.q r0, (r6)
    add     r6, r6, #8
    store.q r0, (r6)
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

    ; Zero region table (MAX_TASKS * KD_REGION_TASK_SZ bytes at
    ; KD_REGION_TABLE — 16 tasks * 128 bytes = 2048 bytes after M12).
    ; Loop count is derived from the structural caps so future bumps
    ; cannot drift relative to the actual table size.
    move.l  r2, #KERN_DATA_BASE
    add     r2, r2, #KD_REGION_TABLE
    move.l  r3, #(MAX_TASKS * KD_REGION_TASK_SZ / 8)   ; quad-words
    move.l  r4, #0
.zero_regions:
    store.q r0, (r2)
    add     r2, r2, #8
    add     r4, r4, #1
    blt     r4, r3, .zero_regions

    ; Zero shared object table (KD_SHMEM_MAX * KD_SHMEM_STRIDE bytes
    ; at KD_SHMEM_TABLE — 16 entries * 16 bytes = 256 bytes after M12).
    move.l  r2, #KERN_DATA_BASE
    add     r2, r2, #KD_SHMEM_TABLE
    move.l  r3, #(KD_SHMEM_MAX * KD_SHMEM_STRIDE / 8) ; quad-words
    move.l  r4, #0
.zero_shmem:
    store.q r0, (r2)
    add     r2, r2, #8
    add     r4, r4, #1
    blt     r4, r3, .zero_shmem

    ; ---------------------------------------------------------------
    ; M12.5: Initialize hardware.resource state — broker identity
    ; sentinel and grant-table chain header. Both live in the kernel
    ; data page so they are accessible without PT switching. The first
    ; chain page is allocated lazily during the boot-load loop when
    ; the first bootstrap grant is inserted (task IDs do not exist
    ; yet at kern_init).
    ; ---------------------------------------------------------------
    move.l  r2, #KERN_DATA_BASE
    move.l  r4, #0xFF
    store.b r4, KD_HWRES_TASK(r2)       ; broker = unclaimed sentinel
    move.l  r2, #KERN_DATA_BASE
    add     r2, r2, #KD_GRANT_TABLE_HDR
    store.q r0, (r2)                    ; zero the 8-byte header

    ; Zero the per-task region overflow chain headers (16 tasks * 8 bytes)
    move.l  r2, #KERN_DATA_BASE
    add     r2, r2, #KD_REGION_OVERFLOW_HEAD
    move.l  r3, #(MAX_TASKS * KD_REGION_OFLOW_STRIDE / 8)
    move.l  r4, #0
.zero_oflow:
    store.q r0, (r2)
    add     r2, r2, #8
    add     r4, r4, #1
    blt     r4, r3, .zero_oflow

    ; M12.6 Phase B: zero the shmem overflow chain header (8 bytes).
    ; Lazy allocation — first chain page is allocated on demand by
    ; kern_shmem_alloc_slot when the inline 16 rows are exhausted.
    move.l  r2, #KERN_DATA_BASE
    add     r2, r2, #KD_SHMEM_OFLOW_HDR
    store.q r0, (r2)

    ; M12.6 Phase C: zero the port overflow chain header (8 bytes).
    ; Lazy allocation — first chain page is allocated on demand by
    ; kern_port_alloc_slot when the inline 32 ports are exhausted.
    move.l  r2, #KERN_DATA_BASE
    add     r2, r2, #KD_PORT_OFLOW_HDR
    store.q r0, (r2)

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

    ; --- M12.5: insert bootstrap grants for this boot entry ---
    ; r1 already holds the assigned task ID from load_program. Pass
    ; (boot_idx=r29, task_id=r1) to kern_bootstrap_grant_for_program,
    ; which scans bootstrap_grant_table for matching rows and writes
    ; live grant entries via kern_grant_create_row. Failure to insert
    ; a bootstrap grant is a hard boot failure (panic) — without the
    ; grant, the affected task's first SYS_MAP_IO call would return
    ; ERR_PERM and the boot path could not come up.
    push    r30
    push    r29
    move.q  r2, r1                      ; r2 = task_id
    move.q  r1, r29                     ; r1 = boot entry index
    jsr     kern_bootstrap_grant_for_program ; R2 = err
    pop     r29
    pop     r30
    bnez    r2, .boot_load_fail

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
; Input: r10 = task_id, r9 = code_pages, r11 = data_pages
; Clobbers: r2-r9, r10, r11

build_user_pt:
    push    r10
    push    r9
    push    r11
    ; Compute PT base address: USER_PT_BASE + task_id * USER_SLOT_STRIDE
    move.l  r7, #USER_SLOT_STRIDE
    mulu    r7, r10, r7
    add     r7, r7, #USER_PT_BASE      ; r7 = this task's PT base

    ; Copy kernel PT entries for pages 0..KERN_PAGES-1
    move.l  r2, #KERN_PAGE_TABLE
    move.l  r4, #0
.bup_copy_kern:
    move.l  r6, #KERN_PAGES
    bge     r4, r6, .bup_copy_pool
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r7
    store.q r3, (r9)
    add     r4, r4, #1
    bra     .bup_copy_kern

    ; M12.6 Phase C: copy allocator pool entries (supervisor-only) so the
    ; kernel can access chain pages from a user PT without switching to the
    ; kernel PT for every chain walker invocation. The pool entries were
    ; written to the kernel PT at boot by kern_init's kern_pool_map loop;
    ; here we copy them across into this task's PT so the supervisor-only
    ; flag is preserved (user code still cannot read them).
.bup_copy_pool:
    move.l  r4, #ALLOC_POOL_BASE
    move.l  r6, #ALLOC_POOL_PAGES
    add     r6, r6, r4                  ; r6 = pool end VPN (exclusive)
.bup_copy_pool_loop:
    bge     r4, r6, .bup_copy_user
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r7
    store.q r3, (r9)
    add     r4, r4, #1
    bra     .bup_copy_pool_loop

.bup_copy_user:
    ; Copy supervisor-only user page entries (VPN 0x600..0x600+MAX_TASKS*0x10)
    move.l  r4, #USER_CODE_VPN_BASE
    move.l  r6, #USER_CODE_VPN_BASE
    move.l  r9, #MAX_TASKS
    move.l  r8, #USER_VPN_STRIDE
    mulu    r9, r9, r8
    add     r6, r6, r9                  ; end VPN
.bup_copy_user_loop:
    bge     r4, r6, .bup_copy_userpt
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r7
    store.q r3, (r9)
    add     r4, r4, #1
    bra     .bup_copy_user_loop

.bup_copy_userpt:
    ; Copy supervisor-only user-PT region entries (VPN USER_PT_PAGE_BASE..
    ; USER_PT_PAGE_END). Required because the kernel walks each task's PT
    ; from kernel-supervisor mode while running on the task's own PTBR
    ; (e.g. safe_copy_user_name does load.q on PTBR + VPN*8). Without
    ; copying these mappings, the kernel faults the moment it touches a
    ; task PT after the user-PT region was relocated to $680000.
    move.l  r4, #USER_PT_PAGE_BASE
    move.l  r6, #USER_PT_PAGE_END
.bup_copy_userpt_loop:
    bge     r4, r6, .bup_add_user_pages
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r7
    store.q r3, (r9)
    add     r4, r4, #1
    bra     .bup_copy_userpt_loop

.bup_add_user_pages:
    ; Add user-accessible entries for THIS task's pages
    pop     r11                         ; restore data_pages
    pop     r9                          ; restore code_pages
    pop     r10                         ; restore task_id
    move.l  r8, #USER_VPN_STRIDE
    mulu    r8, r10, r8
    add     r8, r8, #USER_CODE_VPN_BASE ; r8 = base code VPN
    ; Code pages: P|X|U = 0x19 (loop r9 pages)
    move.l  r4, #0
.bup_code_loop:
    bge     r4, r9, .bup_code_done
    add     r6, r8, r4                  ; VPN = base + i
    lsl     r3, r6, #13
    or      r3, r3, #0x19
    lsl     r5, r6, #3
    add     r5, r5, r7
    store.q r3, (r5)
    add     r4, r4, #1
    bra     .bup_code_loop
.bup_code_done:
    ; Stack page (VPN = base + code_pages): P|R|W|U = 0x17
    add     r6, r8, r9                  ; VPN = base + code_pages
    lsl     r3, r6, #13
    or      r3, r3, #0x17
    lsl     r5, r6, #3
    add     r5, r5, r7
    store.q r3, (r5)
    ; Data pages: P|R|W|U = 0x17 (loop r11 pages)
    add     r6, r8, r9
    add     r6, r6, #1                  ; VPN = base + code_pages + 1
    move.l  r4, #0
.bup_data_loop:
    bge     r4, r11, .bup_data_done
    add     r2, r6, r4                  ; VPN = data_base_vpn + i
    lsl     r3, r2, #13
    or      r3, r3, #0x17
    lsl     r5, r2, #3
    add     r5, r5, r7
    store.q r3, (r5)
    add     r4, r4, #1
    bra     .bup_data_loop
.bup_data_done:
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

    ; 3. Load code_size, validate > 0 and 8-byte aligned.
    ;    M12.8 prerequisite: the prior arbitrary `code_size <= 8192` cap
    ;    was a bucket-C product limit, not an architectural bound. It has
    ;    been removed; the actual constraint is the per-task slot fit
    ;    check at step 5d below.
    load.l  r20, IMG_OFF_CODE_SIZE(r7)  ; r20 = code_size
    beqz    r20, .lp_badarg
    and     r14, r20, #7                ; check 8-byte alignment
    bnez    r14, .lp_badarg

    ; 4. Load data_size. M12.8 prerequisite: the prior arbitrary
    ;    `data_size <= 49152` cap (bumped from M5 → M12 to make room for
    ;    embedded service images) was the same kind of bucket-C product
    ;    limit. It has been removed; the slot fit check at step 5d is
    ;    the real constraint.
    load.l  r21, IMG_OFF_DATA_SIZE(r7)  ; r21 = data_size

    ; 5. Check image_size >= 32 + code_size + data_size (truncated image).
    ;    Performed in 64-bit; load.l zero-extends so r20/r21 are in [0,2^32).
    move.l  r14, #IMG_HEADER_SIZE
    add     r14, r14, r20
    add     r14, r14, r21               ; r14 = required size
    bgt     r14, r8, .lp_badarg         ; truncated image

    ; 5b. Compute code_pages = ceil(code_size / 4096).
    ;     Safe vs hostile inputs: r20 ∈ [0,2^32), so r26 ≤ 2^20, well
    ;     under any signed-bgt overflow boundary used at step 5d.
    move.q  r26, r20
    add     r26, r26, #0xFFF
    lsr     r26, r26, #12               ; r26 = code_pages

    ; 5c. Compute data_pages (default 1, or ceil(data_size / 4096))
    move.l  r27, #1                     ; default data_pages = 1
    beqz    r21, .lp_skip_data_pages
    move.q  r27, r21
    add     r27, r27, #0xFFF
    lsr     r27, r27, #12               ; r27 = data_pages
.lp_skip_data_pages:

    ; 5d. Architectural slot-fit check (M12.8 prerequisite).
    ;     Each task slot is USER_VPN_STRIDE pages wide and is laid out as
    ;       code_pages | 1 stack page | data_pages
    ;     so the only real ceiling on an image is
    ;       code_pages + 1 + data_pages <= USER_VPN_STRIDE.
    ;     This single bound replaces the two prior arbitrary caps and is
    ;     the *honest* architectural limit — the layout itself enforces
    ;     it. A hostile image declaring code_size = 0xFFFFFFFF computes
    ;     code_pages ≈ 2^20 here and is correctly rejected by this bound.
    add     r14, r26, r27
    add     r14, r14, #1                ; +1 for the stack page
    move.l  r11, #USER_VPN_STRIDE
    bgt     r14, r11, .lp_badarg

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

    ; 8. Zero child's code pages (code_pages * 4096 bytes)
    move.l  r11, #USER_SLOT_STRIDE
    mulu    r11, r22, r11               ; r11 = task_id * stride
    move.l  r23, #USER_CODE_BASE
    add     r23, r23, r11               ; r23 = child code page addr
    move.l  r4, #0
    lsl     r6, r26, #12                ; r6 = code_pages * 4096
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

    ; 10. If data_size > 0: zero data pages, copy data
    beqz    r21, .lp_skip_data

    ; Zero data pages (data_pages * 4096 bytes)
    ; Data base address = code_base + (code_pages + 1) * 4096
    add     r11, r26, #1               ; code_pages + 1 (skip stack page)
    lsl     r11, r11, #12
    add     r24, r23, r11               ; r24 = child data base
    move.l  r4, #0
    lsl     r6, r27, #12               ; r6 = data_pages * 4096
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
    move.q  r9, r26                     ; r9 = code_pages
    move.q  r11, r27                    ; r11 = data_pages
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

    ; USP = USER_CODE_BASE + task_id * stride + (code_pages+1)*4096 + PAGE_SIZE
    add     r9, r26, #1
    lsl     r9, r9, #12
    move.l  r14, #USER_CODE_BASE
    add     r14, r14, r11
    add     r14, r14, r9
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

; count_free_pages: count the number of free (0) bits in the page bitmap,
; counting only the first ALLOC_POOL_PAGES bits (the bitmap is sized to a
; round 800 bytes = 6400 bits, but ALLOC_POOL_PAGES may be < 6400 so the
; trailing bits are unused and must NOT be counted as free).
; Output: R1 = number of free pages
; Clobbers: R2-R7
count_free_pages:
    move.l  r2, #KERN_DATA_BASE
    add     r2, r2, #KD_PAGE_BITMAP    ; r2 = bitmap base
    move.l  r3, #ALLOC_POOL_PAGES      ; r3 = total bits to scan (M12: not bitmap byte size)
    move.q  r1, #0                     ; r1 = free count
    move.l  r4, #0                     ; r4 = bit index
.cfp_bit_loop:
    bge     r4, r3, .cfp_done
    ; Read bit r4: byte = bitmap[r4/8], bit = r4 % 8
    lsr     r5, r4, #3                 ; r5 = byte index
    add     r5, r5, r2                 ; r5 = &bitmap[byte]
    load.b  r5, (r5)                   ; r5 = bitmap byte
    and     r6, r4, #7                 ; r6 = bit position
    lsr     r5, r5, r6                 ; shift target bit to position 0
    and     r5, r5, #1
    bnez    r5, .cfp_skip
    add     r1, r1, #1                 ; free bit
.cfp_skip:
    add     r4, r4, #1
    bra     .cfp_bit_loop
.cfp_done:
    rts

; ============================================================================
; M12.5: hardware.resource grant-table helpers
; ============================================================================
; The grant table is a chain-linked list of allocator-backed pages. Each page
; holds 255 grant rows (16 bytes each) plus a 16-byte header whose first two
; bytes are the next-page PPN. Existing pages NEVER move when the chain
; grows — only the tail's next-pointer is updated. This is what makes the
; design safe even though the broker walks the chain through these helpers
; (it never holds a stale row pointer).
;
; Header sits in the kernel data page at KD_GRANT_TABLE_HDR and is always
; accessible without PT switching (kernel data is in the kernel-mapped range
; that every user PT inherits as supervisor mappings). Chain pages live in
; the allocator pool (PPN >= ALLOC_POOL_BASE = 0x800), so accessing them
; requires the kernel PT to be active (kern_pool_map at boot installs them
; as supervisor R/W in KERN_PAGE_TABLE only). The helpers below handle the
; PT switch internally so callers from any context just call them directly.

; ----------------------------------------------------------------------------
; kern_grant_chain_alloc_page: allocate one fresh chain page, link to tail.
; ----------------------------------------------------------------------------
; Inputs:  none
; Outputs: R1 = new chain page PPN (absolute), R2 = ERR_OK or ERR_NOMEM
; Clobbers: R3..R16, R28
;
; Steps:
;   1. Switch to kernel PT (saved in R28).
;   2. alloc_pages(1) -> R1=PPN, R2=err. Bail out on err.
;   3. Zero the entire 4 KiB page.
;   4. Set every row's KD_GRANT_TASK_ID byte to GRANT_TASK_FREE (0xFF).
;   5. Walk the existing chain header. If empty, set FIRST_PPN to new page.
;      Otherwise walk to the tail page and store the new PPN into its
;      KD_GRANT_PAGE_NEXT field.
;   6. Bump KD_GRANT_HDR_PAGES.
;   7. Restore PTBR. Return R1=new PPN, R2=ERR_OK.
kern_grant_chain_alloc_page:
    mfcr    r28, cr0                    ; save current PTBR
    move.l  r3, #KERN_PAGE_TABLE
    mtcr    cr0, r3
    tlbflush

    move.l  r1, #1                      ; allocate 1 page
    jsr     alloc_pages                 ; R1 = PPN, R2 = err
    bnez    r2, .gcap_fail

    move.q  r29, r1                     ; r29 = new chain page PPN (preserve)

    ; Zero the entire page (4096 bytes = 512 quadwords)
    lsl     r3, r1, #12                 ; r3 = byte addr of page
    move.q  r4, r3
    move.l  r5, #4096
    add     r5, r5, r3                  ; r5 = end addr
.gcap_zero:
    bge     r4, r5, .gcap_zero_done
    store.q r0, (r4)
    add     r4, r4, #8
    bra     .gcap_zero
.gcap_zero_done:

    ; Mark all 255 rows as free (KD_GRANT_TASK_ID = 0xFF at each row offset).
    ; Row 0 is at page+KD_GRANT_PAGE_HDR_SZ, row stride = KD_GRANT_ROW_SIZE.
    move.l  r4, #KD_GRANT_PAGE_HDR_SZ
    add     r4, r4, r3                  ; r4 = &row[0]
    move.l  r5, #0                      ; row index
    move.l  r6, #0xFF
.gcap_mark:
    move.l  r7, #KD_GRANT_ROWS_PER_PG
    bge     r5, r7, .gcap_mark_done
    store.b r6, KD_GRANT_TASK_ID(r4)
    add     r4, r4, #KD_GRANT_ROW_SIZE
    add     r5, r5, #1
    bra     .gcap_mark
.gcap_mark_done:

    ; Link onto the chain. Header is in kernel data page, always accessible.
    move.l  r3, #KERN_DATA_BASE
    add     r3, r3, #KD_GRANT_TABLE_HDR ; r3 = &header
    load.w  r4, KD_GRANT_HDR_FIRST_PPN(r3)
    bnez    r4, .gcap_walk_tail

    ; Empty chain — set FIRST_PPN
    store.w r29, KD_GRANT_HDR_FIRST_PPN(r3)
    bra     .gcap_link_done

.gcap_walk_tail:
    ; r4 = current PPN. Walk pages until next == 0.
.gcap_walk_loop:
    lsl     r5, r4, #12                 ; r5 = byte addr of current page
    load.w  r6, KD_GRANT_PAGE_NEXT(r5)  ; r6 = next page PPN
    beqz    r6, .gcap_at_tail
    move.q  r4, r6
    bra     .gcap_walk_loop
.gcap_at_tail:
    ; r5 = byte addr of tail page; store new PPN into its NEXT field
    store.w r29, KD_GRANT_PAGE_NEXT(r5)

.gcap_link_done:
    ; Bump KD_GRANT_HDR_PAGES
    load.w  r4, KD_GRANT_HDR_PAGES(r3)
    add     r4, r4, #1
    store.w r4, KD_GRANT_HDR_PAGES(r3)

    ; Restore PTBR and return new PPN.
    mtcr    cr0, r28
    tlbflush
    move.q  r1, r29
    move.q  r2, #ERR_OK
    rts

.gcap_fail:
    ; alloc_pages already loaded R1=0 R2=ERR_NOMEM. Restore PTBR and return.
    mtcr    cr0, r28
    tlbflush
    rts

; ----------------------------------------------------------------------------
; kern_grant_create_row: write a grant row, allocating a new chain page if
; the existing chain is empty or every row in every page is occupied.
; ----------------------------------------------------------------------------
; Inputs:  R1 = target task ID (low byte used)
;          R2 = region tag (low 32 bits used)
;          R3 = PPN low (inclusive, low 16 bits used)
;          R4 = PPN high (inclusive, low 16 bits used)
; Outputs: R1 = unused, R2 = ERR_OK or ERR_NOMEM
; Clobbers: R3..R19, R28, R29
;
; Walks every chain page; for each, scans rows 0..254 looking for one whose
; KD_GRANT_TASK_ID == 0xFF. Writes the row in place. If no chain page has a
; free row (or chain is empty), allocates a new page via
; kern_grant_chain_alloc_page and writes row 0 of the new page.
kern_grant_create_row:
    ; Save inputs across helper calls.
    push    r1                          ; saved task_id
    push    r2                          ; saved tag
    push    r3                          ; saved ppn_lo
    push    r4                          ; saved ppn_hi

    ; Switch to kernel PT to walk chain pages. Save the previous PTBR on
    ; the STACK (not r28) because kern_grant_chain_alloc_page may also
    ; clobber r28 with its own PT save/restore — keeping the saved PT in
    ; r28 across that nested call would corrupt it. The stack copy is the
    ; canonical reference for the restore at the bottom of this routine.
    mfcr    r28, cr0
    push    r28
    move.l  r3, #KERN_PAGE_TABLE
    mtcr    cr0, r3
    tlbflush

    ; Load chain header.
    move.l  r3, #KERN_DATA_BASE
    add     r3, r3, #KD_GRANT_TABLE_HDR ; r3 = &header
    load.w  r4, KD_GRANT_HDR_FIRST_PPN(r3)
    beqz    r4, .gcr_alloc_first

.gcr_walk_pages:
    ; r4 = current chain page PPN, r3 = &header
    lsl     r5, r4, #12                 ; r5 = byte addr of page
    move.q  r6, r5
    add     r6, r6, #KD_GRANT_PAGE_HDR_SZ ; r6 = &row[0]
    move.l  r7, #0                      ; row index
.gcr_scan_rows:
    move.l  r8, #KD_GRANT_ROWS_PER_PG
    bge     r7, r8, .gcr_next_page
    load.b  r8, KD_GRANT_TASK_ID(r6)
    move.l  r9, #0xFF
    beq     r8, r9, .gcr_found_row
    add     r6, r6, #KD_GRANT_ROW_SIZE
    add     r7, r7, #1
    bra     .gcr_scan_rows

.gcr_next_page:
    ; r5 = byte addr of current page; load its NEXT field.
    load.w  r4, KD_GRANT_PAGE_NEXT(r5)
    beqz    r4, .gcr_alloc_more
    bra     .gcr_walk_pages

.gcr_alloc_first:
.gcr_alloc_more:
    ; Need a new chain page. (gcr_alloc_first and gcr_alloc_more share code.)
    jsr     kern_grant_chain_alloc_page ; R1 = new PPN, R2 = err
    bnez    r2, .gcr_nomem

    ; New page: row 0 is at PPN<<12 + KD_GRANT_PAGE_HDR_SZ
    lsl     r5, r1, #12
    move.q  r6, r5
    add     r6, r6, #KD_GRANT_PAGE_HDR_SZ
    ; r6 = &row[0] of new chain page; fall through to .gcr_found_row

.gcr_found_row:
    ; r6 = address of free row. Pop saved values from the stack.
    ; Stack layout (top first): saved-PT (r28), ppn_hi (r4), ppn_lo (r3),
    ; tag (r2), task_id (r1). Pop the PT first since it's the topmost,
    ; then the args in reverse-push order.
    pop     r28                         ; saved user PT
    pop     r4                          ; ppn_hi
    pop     r3                          ; ppn_lo
    pop     r2                          ; tag
    pop     r1                          ; task_id

    store.b r1, KD_GRANT_TASK_ID(r6)
    store.l r2, KD_GRANT_REGION(r6)
    store.w r3, KD_GRANT_PPN_LO(r6)
    store.w r4, KD_GRANT_PPN_HI(r6)
    store.l r0, KD_GRANT_FLAGS(r6)

    ; Bump KD_GRANT_HDR_TOTAL
    move.l  r5, #KERN_DATA_BASE
    add     r5, r5, #KD_GRANT_TABLE_HDR
    load.w  r7, KD_GRANT_HDR_TOTAL(r5)
    add     r7, r7, #1
    store.w r7, KD_GRANT_HDR_TOTAL(r5)

    ; Restore PTBR.
    mtcr    cr0, r28
    tlbflush
    move.q  r2, #ERR_OK
    rts

.gcr_nomem:
    ; kern_grant_chain_alloc_page already restored PTBR on its failure path,
    ; but we did our own switch above and need to pop the saved PT and
    ; restore. Pop in stack order: saved-PT first, then the four args.
    pop     r28                         ; saved user PT
    pop     r4                          ; discard saved inputs
    pop     r3
    pop     r2
    pop     r1
    mtcr    cr0, r28
    tlbflush
    move.q  r2, #ERR_NOMEM
    rts

; ----------------------------------------------------------------------------
; kern_grant_check: check whether the calling task has a grant covering a
; given PPN range. Used by SYS_MAP_IO before installing PTEs.
; ----------------------------------------------------------------------------
; Inputs:  R1 = task ID (low byte)
;          R2 = requested PPN low (inclusive)
;          R3 = requested page count (>= 1)
; Outputs: R1 = 1 if a covering grant exists, 0 otherwise; R2 = ERR_OK
; Clobbers: R3..R19, R28
kern_grant_check:
    ; Compute requested PPN high (inclusive) = req_lo + count - 1
    add     r4, r2, r3
    sub     r4, r4, #1                  ; r4 = req_hi
    move.q  r5, r2                      ; r5 = req_lo
    move.q  r6, r1                      ; r6 = task_id

    ; Switch to kernel PT to walk chain pages.
    mfcr    r28, cr0
    move.l  r7, #KERN_PAGE_TABLE
    mtcr    cr0, r7
    tlbflush

    move.l  r7, #KERN_DATA_BASE
    add     r7, r7, #KD_GRANT_TABLE_HDR
    load.w  r8, KD_GRANT_HDR_FIRST_PPN(r7)
    beqz    r8, .gck_nope

.gck_page:
    lsl     r9, r8, #12                 ; r9 = page byte addr
    move.q  r10, r9
    add     r10, r10, #KD_GRANT_PAGE_HDR_SZ ; r10 = &row[0]
    move.l  r11, #0                     ; row idx
.gck_row:
    move.l  r12, #KD_GRANT_ROWS_PER_PG
    bge     r11, r12, .gck_next_pg
    load.b  r12, KD_GRANT_TASK_ID(r10)
    bne     r12, r6, .gck_row_next
    ; Task matches; check PPN range
    load.w  r13, KD_GRANT_PPN_LO(r10)
    load.w  r14, KD_GRANT_PPN_HI(r10)
    blt     r5, r13, .gck_row_next      ; req_lo < grant_lo? skip
    bgt     r4, r14, .gck_row_next      ; req_hi > grant_hi? skip
    ; Match!
    mtcr    cr0, r28
    tlbflush
    move.q  r1, #1
    move.q  r2, #ERR_OK
    rts
.gck_row_next:
    add     r10, r10, #KD_GRANT_ROW_SIZE
    add     r11, r11, #1
    bra     .gck_row
.gck_next_pg:
    load.w  r8, KD_GRANT_PAGE_NEXT(r9)
    bnez    r8, .gck_page
.gck_nope:
    mtcr    cr0, r28
    tlbflush
    move.q  r1, #0
    move.q  r2, #ERR_OK
    rts

; ----------------------------------------------------------------------------
; kern_bootstrap_grant_for_program: walk the bootstrap_grant_table for any
; rows whose program index matches the just-loaded boot entry, and insert
; each as a live grant tied to the assigned task ID. Called from the boot-
; load loop immediately after load_program returns.
; ----------------------------------------------------------------------------
; Inputs:  R1 = boot entry index (0..PROGTAB_BOOT_COUNT-1)
;          R2 = assigned task ID for that program
; Outputs: R2 = ERR_OK or ERR_NOMEM
; Clobbers: R3..R19, R20-R22, R28, R29, R30
;
; Strategy: copy the inputs into call-preserved registers (R20=boot idx,
; R21=task_id) and walk the table cursor in R22. Each iteration that
; matches calls kern_grant_create_row with R1=task_id, R2=tag, R3=ppn_lo,
; R4=ppn_hi. R20/R21/R22 are pushed/popped around the call because the
; helper documents clobbering R3..R19 + R28/R29 — R20+ are not in its
; clobber list but defensive save/restore makes the call site robust.
kern_bootstrap_grant_for_program:
    move.q  r20, r1                     ; r20 = boot entry index (preserved)
    move.q  r21, r2                     ; r21 = task_id (preserved)
    la      r22, bootstrap_grant_table  ; r22 = table cursor
.bgfp_loop:
    load.b  r3, BOOTSTRAP_GRANT_OFF_PROG_IDX(r22)
    move.l  r4, #0xFF
    beq     r3, r4, .bgfp_done          ; sentinel = end of table
    bne     r3, r20, .bgfp_advance      ; row's program idx != ours, skip
    ; Match — load tag/ppn_lo/ppn_hi from the row and call create_row.
    load.l  r5, BOOTSTRAP_GRANT_OFF_TAG(r22)
    load.w  r6, BOOTSTRAP_GRANT_OFF_PPN_LO(r22)
    load.w  r7, BOOTSTRAP_GRANT_OFF_PPN_HI(r22)
    push    r20
    push    r21
    push    r22
    move.q  r1, r21                     ; r1 = task_id
    move.q  r2, r5                      ; r2 = tag
    move.q  r3, r6                      ; r3 = ppn_lo
    move.q  r4, r7                      ; r4 = ppn_hi
    jsr     kern_grant_create_row       ; R2 = err
    pop     r22
    pop     r21
    pop     r20
    bnez    r2, .bgfp_fail
.bgfp_advance:
    add     r22, r22, #BOOTSTRAP_GRANT_ROW_SZ
    bra     .bgfp_loop
.bgfp_done:
    move.q  r2, #ERR_OK
    rts
.bgfp_fail:
    move.q  r2, #ERR_NOMEM
    rts

; ----------------------------------------------------------------------------
; kern_grant_release_for_task: walk the entire grant chain and mark every row
; whose task_id matches the input as free (KD_GRANT_TASK_ID = GRANT_TASK_FREE).
; Called from kill_task_cleanup so that a recycled task slot cannot inherit
; the previous occupant's MMIO grants.
; ----------------------------------------------------------------------------
; Inputs:  R1 = task_id (low byte)
; Outputs: none
; Clobbers: R3..R15
;
; Caller must already be on kernel PT (chain pages live in the allocator pool).
kern_grant_release_for_task:
    move.q  r6, r1                      ; r6 = task_id to clear
    move.l  r3, #KERN_DATA_BASE
    add     r3, r3, #KD_GRANT_TABLE_HDR
    load.w  r4, KD_GRANT_HDR_FIRST_PPN(r3)
    beqz    r4, .grft_done

.grft_page:
    lsl     r5, r4, #12                 ; r5 = page byte addr
    move.q  r7, r5
    add     r7, r7, #KD_GRANT_PAGE_HDR_SZ ; r7 = first row
    move.l  r8, #0                      ; row idx
.grft_row:
    move.l  r9, #KD_GRANT_ROWS_PER_PG
    bge     r8, r9, .grft_next_page
    load.b  r9, KD_GRANT_TASK_ID(r7)
    bne     r9, r6, .grft_row_skip
    ; Match — mark free and bump down the header TOTAL counter.
    move.l  r10, #0xFF
    store.b r10, KD_GRANT_TASK_ID(r7)
    load.w  r11, KD_GRANT_HDR_TOTAL(r3)
    sub     r11, r11, #1
    store.w r11, KD_GRANT_HDR_TOTAL(r3)
.grft_row_skip:
    add     r7, r7, #KD_GRANT_ROW_SIZE
    add     r8, r8, #1
    bra     .grft_row
.grft_next_page:
    load.w  r4, KD_GRANT_PAGE_NEXT(r5)
    bnez    r4, .grft_page
.grft_done:
    rts

; ============================================================================
; M12.5: per-task region OVERFLOW chain helpers
; ============================================================================
; M12.5 removes the per-task KD_REGION_MAX = 8 cap by adding an overflow
; chain. The first 8 rows still live inline at the original KD_REGION_TABLE
; location (preserving the fast path and layout offsets); rows 9+ live in
; allocator-backed chain pages reached through a per-task overflow header
; at KD_REGION_OVERFLOW_HEAD. Walkers iterate inline first, then walk the
; overflow chain. The "find free row" allocators look at inline first, then
; in the overflow chain, then allocate a new chain page when both are full.
;
; Helpers below assume the caller is already on the kernel PT (chain pages
; live in the allocator pool which is only mapped in the kernel PT). Each
; walker site that uses overflow takes care of the PT switching itself.

; ----------------------------------------------------------------------------
; kern_region_oflow_head: return pointer to a task's overflow chain header.
; Inputs:  R1 = task_id
; Outputs: R1 = &header (kernel data offset, always accessible)
; Clobbers: R3, R4
kern_region_oflow_head:
    move.l  r3, #KERN_DATA_BASE
    add     r3, r3, #KD_REGION_OVERFLOW_HEAD
    move.l  r4, #KD_REGION_OFLOW_STRIDE
    mulu    r4, r1, r4
    add     r1, r3, r4
    rts

; ----------------------------------------------------------------------------
; kern_region_oflow_alloc_row: find a free row in the task's overflow chain,
; allocating a new chain page if every existing row is occupied. Caller must
; already be on the kernel PT.
; ----------------------------------------------------------------------------
; Inputs:  R1 = task_id
; Outputs: R1 = row addr, R2 = ERR_OK or ERR_NOMEM
; Clobbers: R3..R19
kern_region_oflow_alloc_row:
    push    r1
    jsr     kern_region_oflow_head      ; R1 = &header
    move.q  r10, r1

    load.w  r4, KD_REGION_OFLOW_FIRST_PPN(r10)
    beqz    r4, .roar_alloc_first

.roar_walk:
    lsl     r5, r4, #12                 ; r5 = page byte addr
    move.q  r6, r5
    add     r6, r6, #KD_REGION_PAGE_HDR_SZ
    move.l  r7, #0
.roar_scan:
    move.l  r8, #KD_REGION_ROWS_PER_PG
    bge     r7, r8, .roar_next_page
    load.b  r8, KD_REG_TYPE(r6)
    beqz    r8, .roar_found             ; REGION_FREE
    add     r6, r6, #KD_REGION_STRIDE
    add     r7, r7, #1
    bra     .roar_scan
.roar_next_page:
    load.w  r4, KD_REGION_PAGE_NEXT(r5)
    bnez    r4, .roar_walk

.roar_alloc_first:
    ; Need a new overflow chain page.
    move.l  r1, #1
    jsr     alloc_pages                 ; R1=PPN, R2=err
    bnez    r2, .roar_nomem
    move.q  r29, r1                     ; r29 = new page PPN

    ; Zero entire 4 KiB page (REGION_FREE = 0)
    lsl     r3, r1, #12
    move.q  r4, r3
    move.l  r5, #4096
    add     r5, r5, r3
.roar_zero:
    bge     r4, r5, .roar_zdone
    store.q r0, (r4)
    add     r4, r4, #8
    bra     .roar_zero
.roar_zdone:

    ; Re-derive header pointer (r10 may have been clobbered by alloc_pages).
    pop     r1                          ; task_id
    push    r1
    jsr     kern_region_oflow_head      ; R1 = &header
    move.q  r10, r1

    ; Link new page onto the chain.
    load.w  r4, KD_REGION_OFLOW_FIRST_PPN(r10)
    bnez    r4, .roar_walk_tail
    store.w r29, KD_REGION_OFLOW_FIRST_PPN(r10)
    bra     .roar_link_done

.roar_walk_tail:
    lsl     r5, r4, #12
    load.w  r6, KD_REGION_PAGE_NEXT(r5)
    beqz    r6, .roar_at_tail
    move.q  r4, r6
    bra     .roar_walk_tail
.roar_at_tail:
    store.w r29, KD_REGION_PAGE_NEXT(r5)

.roar_link_done:
    load.w  r4, KD_REGION_OFLOW_PAGES(r10)
    add     r4, r4, #1
    store.w r4, KD_REGION_OFLOW_PAGES(r10)

    ; Use row 0 of the new page.
    lsl     r5, r29, #12
    move.q  r6, r5
    add     r6, r6, #KD_REGION_PAGE_HDR_SZ

.roar_found:
    ; r6 = row address. Bump TOTAL counter.
    pop     r1                          ; task_id
    push    r6
    jsr     kern_region_oflow_head      ; R1 = &header
    pop     r6
    load.w  r7, KD_REGION_OFLOW_TOTAL(r1)
    add     r7, r7, #1
    store.w r7, KD_REGION_OFLOW_TOTAL(r1)
    move.q  r1, r6
    move.q  r2, #ERR_OK
    rts

.roar_nomem:
    pop     r1                          ; discard saved task_id
    move.q  r1, #0
    move.q  r2, #ERR_NOMEM
    rts

; ============================================================================
; M12.6 Phase B: shmem overflow chain helpers
; ============================================================================
; Same chain pattern as the M12.5 region overflow chain, but with a single
; global header (KD_SHMEM_OFLOW_HDR) instead of a per-task array. The shmem
; table is system-wide (one shared object pool for all tasks).
;
; Both helpers assume the caller is already on the kernel PT — chain pages
; live in the allocator pool which is only mapped in the kernel PT. Every
; existing shmem walker call site is inside a syscall handler that has done
; an outer PT switch (.do_alloc_mem at iexec.s:2686, .do_free_mem at
; iexec.s:2923, .do_map_shared at iexec.s:3064) or inside kill_task_cleanup
; which is called only after switching to kernel PT.
;
; Slot ID encoding:
;   id 0..KD_SHMEM_INLINE_MAX-1 → inline row at KD_SHMEM_TABLE + id*16
;   id KD_SHMEM_INLINE_MAX..    → overflow chain row
;     overflow_index = id - KD_SHMEM_INLINE_MAX
;     page_index     = overflow_index / KD_SHMEM_ROWS_PER_PG
;     slot_in_page   = overflow_index % KD_SHMEM_ROWS_PER_PG

; ----------------------------------------------------------------------------
; kern_shmem_alloc_slot: walk inline + overflow chain looking for a free
; slot (KD_SHM_VALID == 0). Allocate a new chain page if none found.
; ----------------------------------------------------------------------------
; In:  none (caller must already be on the kernel PT)
; Out: R1 = slot addr (kernel data VA, or chain page VA; 0 on ERR_NOMEM)
;      R2 = slot id
; Clobbers: R4..R19, R28
;
; Note: r3 is intentionally NOT touched. Callers in syscall handlers may have
; user-visible state in r3 (e.g. .do_alloc_mem returns r3 = share_handle, but
; some callers like .do_create_port leave r3 holding a user pointer that the
; user-space test relies on surviving the syscall). Use `beqz r1` to detect
; NOMEM rather than checking an err code in r3.
kern_shmem_alloc_slot:
    ; --- Scan inline rows 0..KD_SHMEM_INLINE_MAX-1 ---
    move.l  r4, #KERN_DATA_BASE
    add     r4, r4, #KD_SHMEM_TABLE     ; r4 = inline base
    move.l  r5, #0                       ; r5 = inline slot id
.ksas_inline_scan:
    move.l  r6, #KD_SHMEM_INLINE_MAX
    bge     r5, r6, .ksas_oflow
    move.l  r6, #KD_SHMEM_STRIDE
    mulu    r6, r5, r6
    add     r7, r4, r6                   ; r7 = &inline[slot]
    load.b  r8, KD_SHM_VALID(r7)
    beqz    r8, .ksas_inline_found
    add     r5, r5, #1
    bra     .ksas_inline_scan
.ksas_inline_found:
    move.q  r1, r7
    move.q  r2, r5
    rts

.ksas_oflow:
    ; --- Walk overflow chain ---
    ; r6 will track the slot id as we walk (starts at INLINE_MAX).
    move.l  r4, #KERN_DATA_BASE
    add     r4, r4, #KD_SHMEM_OFLOW_HDR  ; r4 = &header
    load.w  r5, KD_SHMEM_OFLOW_FIRST_PPN(r4)
    move.l  r6, #KD_SHMEM_INLINE_MAX     ; r6 = current slot id
    beqz    r5, .ksas_alloc_new

.ksas_walk_page:
    lsl     r7, r5, #12                  ; r7 = page byte addr
    move.q  r8, r7
    add     r8, r8, #KD_SHMEM_PAGE_HDR_SZ ; r8 = first row in page
    move.l  r9, #0                        ; r9 = row idx in this page
.ksas_walk_row:
    move.l  r10, #KD_SHMEM_ROWS_PER_PG
    bge     r9, r10, .ksas_walk_next_page
    load.b  r11, KD_SHM_VALID(r8)
    beqz    r11, .ksas_oflow_found
    add     r8, r8, #KD_SHMEM_STRIDE
    add     r9, r9, #1
    add     r6, r6, #1
    bra     .ksas_walk_row
.ksas_walk_next_page:
    load.w  r5, KD_SHMEM_PAGE_NEXT(r7)
    bnez    r5, .ksas_walk_page
    bra     .ksas_alloc_new
.ksas_oflow_found:
    ; M12.6 Phase E: enforce the 1-byte shmem ID ABI ceiling. The shmem
    ; handle encodes the slot in the low 8 bits and the share-handle
    ; decoder rejects 0xFF (which collides with the sentinel reserved
    ; by the inline shmem ID bytes). Reject any allocation at id >= 0xFF
    ; before returning so we never hand out an unrepresentable ID.
    move.l  r10, #0xFF
    bge     r6, r10, .ksas_nomem
    move.q  r1, r8
    move.q  r2, r6
    rts

.ksas_alloc_new:
    ; All existing rows occupied (or chain empty). r6 holds the slot id
    ; we would assign. M12.6 Phase E: reject before allocating a new chain
    ; page if that id is at or past the 1-byte shmem ID ABI ceiling — no
    ; point burning an allocator pool page on a slot we cannot represent.
    move.l  r10, #0xFF
    bge     r6, r10, .ksas_nomem
    ; Allocate a new chain page and use slot 0 of it. Note: alloc_pages
    ; clobbers a lot of regs; r6 must be preserved across the call.
    push    r6
    move.l  r1, #1
    jsr     alloc_pages                  ; r1 = PPN, r2 = err
    pop     r6
    bnez    r2, .ksas_nomem
    move.q  r12, r1                      ; r12 = new page PPN

    ; Zero entire 4 KiB page (so all KD_SHM_VALID start at 0)
    lsl     r7, r1, #12
    move.q  r8, r7
    move.l  r9, #4096
    add     r9, r9, r7
.ksas_zero:
    bge     r8, r9, .ksas_zdone
    store.q r0, (r8)
    add     r8, r8, #8
    bra     .ksas_zero
.ksas_zdone:

    ; Link the new page onto the chain.
    move.l  r4, #KERN_DATA_BASE
    add     r4, r4, #KD_SHMEM_OFLOW_HDR
    load.w  r5, KD_SHMEM_OFLOW_FIRST_PPN(r4)
    bnez    r5, .ksas_walk_tail
    store.w r12, KD_SHMEM_OFLOW_FIRST_PPN(r4)
    bra     .ksas_link_done
.ksas_walk_tail:
    lsl     r7, r5, #12
    load.w  r8, KD_SHMEM_PAGE_NEXT(r7)
    beqz    r8, .ksas_at_tail
    move.q  r5, r8
    bra     .ksas_walk_tail
.ksas_at_tail:
    store.w r12, KD_SHMEM_PAGE_NEXT(r7)
.ksas_link_done:
    ; Bump page count.
    load.w  r8, KD_SHMEM_OFLOW_PAGES(r4)
    add     r8, r8, #1
    store.w r8, KD_SHMEM_OFLOW_PAGES(r4)

    ; Use slot 0 of the new page; r6 = slot id (preserved across alloc)
    lsl     r7, r12, #12
    add     r1, r7, #KD_SHMEM_PAGE_HDR_SZ
    move.q  r2, r6
    rts

.ksas_nomem:
    move.q  r1, r0
    move.q  r2, r0
    rts

; ----------------------------------------------------------------------------
; kern_shmem_addr_for_id: convert a 1-byte slot id back to a slot address.
; ----------------------------------------------------------------------------
; In:  R1 = slot id (low byte significant; 0..254)
; Out: R1 = slot addr (kernel VA), or 0 if id is past end of allocated chain
; Clobbers: R3..R9
;
; Caller must already be on the kernel PT (overflow chain pages live in
; the allocator pool).
kern_shmem_addr_for_id:
    move.l  r3, #KD_SHMEM_INLINE_MAX
    bge     r1, r3, .ksai_oflow
    ; Inline path: addr = TABLE + id * STRIDE
    move.l  r3, #KD_SHMEM_STRIDE
    mulu    r3, r1, r3
    move.l  r4, #KERN_DATA_BASE
    add     r4, r4, #KD_SHMEM_TABLE
    add     r1, r4, r3
    rts
.ksai_oflow:
    ; Convert id → (page_index, slot_in_page)
    sub     r1, r1, #KD_SHMEM_INLINE_MAX  ; overflow index
    move.l  r3, #KD_SHMEM_ROWS_PER_PG
    divu    r4, r1, r3                    ; r4 = page index
    mulu    r5, r4, r3
    sub     r5, r1, r5                    ; r5 = slot in page
    ; Walk chain `r4` pages forward.
    move.l  r6, #KERN_DATA_BASE
    add     r6, r6, #KD_SHMEM_OFLOW_HDR
    load.w  r7, KD_SHMEM_OFLOW_FIRST_PPN(r6)
.ksai_walk:
    beqz    r7, .ksai_notfound
    beqz    r4, .ksai_at_page
    lsl     r8, r7, #12
    load.w  r7, KD_SHMEM_PAGE_NEXT(r8)
    sub     r4, r4, #1
    bra     .ksai_walk
.ksai_at_page:
    lsl     r8, r7, #12                  ; r8 = page byte addr
    move.l  r9, #KD_SHMEM_STRIDE
    mulu    r9, r5, r9
    add     r1, r8, #KD_SHMEM_PAGE_HDR_SZ
    add     r1, r1, r9
    rts
.ksai_notfound:
    move.q  r1, r0
    rts

; ============================================================================
; M12.6 Phase C: port overflow chain helpers
; ============================================================================
; Same chain pattern as the M12.5 region overflow chain and M12.6 Phase B
; shmem overflow chain, but with port-sized rows (KD_PORT_STRIDE = 168, so
; 24 rows per chain page).
;
; Port table is system-wide (single global header). Port IDs map to slots:
;   id 0..KD_PORT_INLINE_MAX-1   → inline at KD_PORT_BASE + id*KD_PORT_STRIDE
;   id KD_PORT_INLINE_MAX..      → overflow chain
;     overflow_index = id - KD_PORT_INLINE_MAX
;     page_index     = overflow_index / KD_PORT_ROWS_PER_PG
;     slot_in_page   = overflow_index % KD_PORT_ROWS_PER_PG
;
; All helpers assume the caller is on the kernel PT. Every port handler
; (.do_create_port, .do_find_port, .do_put_msg, .do_get_msg, .do_wait_port,
; .do_reply_msg) runs from the trap dispatcher, which is already on kernel
; PT throughout (no syscall handler ever leaves kernel PT for a port op).
; kill_task_cleanup is also called from kernel PT (the timer/exit paths
; switch first; see iexec.s:1767, 2657).

; ----------------------------------------------------------------------------
; kern_port_alloc_slot: walk inline + overflow chain looking for a free
; port (KD_PORT_VALID == 0). Allocate a new chain page if none found.
; ----------------------------------------------------------------------------
; In:  none
; Out: R1 = port addr (0 on ERR_NOMEM), R2 = port id
; Clobbers: R4..R19, R28
;
; Note: r3 is intentionally NOT touched. Callers like .do_create_port leave
; r3 holding a user-space data pointer that the caller's user-space code
; relies on surviving the syscall. Use `beqz r1` to detect NOMEM rather than
; checking an err code in r3.
kern_port_alloc_slot:
    ; --- Scan inline ports 0..KD_PORT_INLINE_MAX-1 ---
    move.l  r4, #KERN_DATA_BASE
    add     r4, r4, #KD_PORT_BASE       ; r4 = inline base
    move.l  r5, #0                       ; r5 = inline port id
.kpas_inline_scan:
    move.l  r6, #KD_PORT_INLINE_MAX
    bge     r5, r6, .kpas_oflow
    move.l  r6, #KD_PORT_STRIDE
    mulu    r6, r5, r6
    add     r7, r4, r6                   ; r7 = &port[id]
    load.b  r8, KD_PORT_VALID(r7)
    beqz    r8, .kpas_inline_found
    add     r5, r5, #1
    bra     .kpas_inline_scan
.kpas_inline_found:
    move.q  r1, r7
    move.q  r2, r5
    rts

.kpas_oflow:
    ; --- Walk overflow chain ---
    move.l  r4, #KERN_DATA_BASE
    add     r4, r4, #KD_PORT_OFLOW_HDR
    load.w  r5, KD_PORT_OFLOW_FIRST_PPN(r4)
    move.l  r6, #KD_PORT_INLINE_MAX      ; r6 = current port id
    beqz    r5, .kpas_alloc_new

.kpas_walk_page:
    lsl     r7, r5, #12                  ; r7 = page byte addr
    move.q  r8, r7
    add     r8, r8, #KD_PORT_PAGE_HDR_SZ ; r8 = first row
    move.l  r9, #0                        ; r9 = row idx in this page
.kpas_walk_row:
    move.l  r10, #KD_PORT_ROWS_PER_PG
    bge     r9, r10, .kpas_walk_next_page
    load.b  r11, KD_PORT_VALID(r8)
    beqz    r11, .kpas_oflow_found
    add     r8, r8, #KD_PORT_STRIDE
    add     r9, r9, #1
    add     r6, r6, #1
    bra     .kpas_walk_row
.kpas_walk_next_page:
    load.w  r5, KD_PORT_PAGE_NEXT(r7)
    bnez    r5, .kpas_walk_page
    bra     .kpas_alloc_new
.kpas_oflow_found:
    ; M12.6 Phase E: enforce the 1-byte port ID ABI ceiling. WAITPORT_NONE
    ; (0xFF) is the sentinel reserved by KD_TASK_WAITPORT and the port
    ; lookup helper rejects ids >= 0xFF. Reject any allocation at id >= 0xFF
    ; before returning so we never hand out an unrepresentable ID.
    ; Note: r3 is intentionally NOT touched (helper clobber contract — see
    ; the .do_create_port r3 user-space scratch reliance). Use r10 instead.
    move.l  r10, #0xFF
    bge     r6, r10, .kpas_nomem
    move.q  r1, r8
    move.q  r2, r6
    rts

.kpas_alloc_new:
    ; All existing rows occupied (or chain empty). r6 holds the port id we
    ; would assign. M12.6 Phase E: reject before allocating a new chain page
    ; if that id is at or past the 1-byte port ID ABI ceiling — no point
    ; burning an allocator pool page on a slot we cannot represent.
    move.l  r10, #0xFF
    bge     r6, r10, .kpas_nomem
    push    r6
    move.l  r1, #1
    jsr     alloc_pages                  ; r1 = PPN, r2 = err
    pop     r6
    bnez    r2, .kpas_nomem
    move.q  r12, r1                      ; r12 = new page PPN

    ; Zero entire 4 KiB page
    lsl     r7, r1, #12
    move.q  r8, r7
    move.l  r9, #4096
    add     r9, r9, r7
.kpas_zero:
    bge     r8, r9, .kpas_zdone
    store.q r0, (r8)
    add     r8, r8, #8
    bra     .kpas_zero
.kpas_zdone:

    ; Link new page onto chain.
    move.l  r4, #KERN_DATA_BASE
    add     r4, r4, #KD_PORT_OFLOW_HDR
    load.w  r5, KD_PORT_OFLOW_FIRST_PPN(r4)
    bnez    r5, .kpas_walk_tail
    store.w r12, KD_PORT_OFLOW_FIRST_PPN(r4)
    bra     .kpas_link_done
.kpas_walk_tail:
    lsl     r7, r5, #12
    load.w  r8, KD_PORT_PAGE_NEXT(r7)
    beqz    r8, .kpas_at_tail
    move.q  r5, r8
    bra     .kpas_walk_tail
.kpas_at_tail:
    store.w r12, KD_PORT_PAGE_NEXT(r7)
.kpas_link_done:
    load.w  r8, KD_PORT_OFLOW_PAGES(r4)
    add     r8, r8, #1
    store.w r8, KD_PORT_OFLOW_PAGES(r4)

    ; Use slot 0 of new page.
    lsl     r7, r12, #12
    add     r1, r7, #KD_PORT_PAGE_HDR_SZ
    move.q  r2, r6
    rts

.kpas_nomem:
    move.q  r1, r0
    move.q  r2, r0
    rts

; ----------------------------------------------------------------------------
; kern_port_addr_for_id: convert a port id to its slot address.
; ----------------------------------------------------------------------------
; In:  R1 = port id (0..254; 0xFF reserved as WAITPORT_NONE)
; Out: R1 = port addr (kernel VA), or 0 if id is past end of chain / 0xFF
; Clobbers: R3..R9
kern_port_addr_for_id:
    ; Reject the WAITPORT_NONE sentinel and any value above it.
    move.l  r3, #0xFF
    bge     r1, r3, .kpai_notfound
    move.l  r3, #KD_PORT_INLINE_MAX
    bge     r1, r3, .kpai_oflow
    ; Inline path
    move.l  r3, #KD_PORT_STRIDE
    mulu    r3, r1, r3
    move.l  r4, #KERN_DATA_BASE
    add     r4, r4, #KD_PORT_BASE
    add     r1, r4, r3
    rts
.kpai_oflow:
    sub     r1, r1, #KD_PORT_INLINE_MAX  ; overflow index
    move.l  r3, #KD_PORT_ROWS_PER_PG
    divu    r4, r1, r3                    ; r4 = page index
    mulu    r5, r4, r3
    sub     r5, r1, r5                    ; r5 = slot in page
    move.l  r6, #KERN_DATA_BASE
    add     r6, r6, #KD_PORT_OFLOW_HDR
    load.w  r7, KD_PORT_OFLOW_FIRST_PPN(r6)
.kpai_walk:
    beqz    r7, .kpai_notfound
    beqz    r4, .kpai_at_page
    lsl     r8, r7, #12
    load.w  r7, KD_PORT_PAGE_NEXT(r8)
    sub     r4, r4, #1
    bra     .kpai_walk
.kpai_at_page:
    lsl     r8, r7, #12
    move.l  r9, #KD_PORT_STRIDE
    mulu    r9, r5, r9
    add     r1, r8, #KD_PORT_PAGE_HDR_SZ
    add     r1, r1, r9
    rts
.kpai_notfound:
    move.q  r1, r0
    rts

; ----------------------------------------------------------------------------
; kern_port_find_public: walk inline + overflow chain looking for a valid
; PUBLIC port whose name matches the scratch buffer at KD_NAME_SCRATCH.
; ----------------------------------------------------------------------------
; In:  none (caller must have populated KD_NAME_SCRATCH via safe_copy_user_name)
; Out: R1 = port addr (or 0 if not found)
;      R2 = port id  (or 0 if not found)
; Clobbers: R3..R19
kern_port_find_public:
    ; --- Scan inline ---
    move.l  r4, #KERN_DATA_BASE
    add     r4, r4, #KD_PORT_BASE
    move.l  r5, #0                       ; r5 = port id
.kpfp_inline_scan:
    move.l  r6, #KD_PORT_INLINE_MAX
    bge     r5, r6, .kpfp_oflow
    move.l  r6, #KD_PORT_STRIDE
    mulu    r6, r5, r6
    add     r7, r4, r6                   ; r7 = &port[id]
    load.b  r8, KD_PORT_VALID(r7)
    beqz    r8, .kpfp_inline_next
    load.b  r8, KD_PORT_FLAGS(r7)
    and     r8, r8, #PF_PUBLIC
    beqz    r8, .kpfp_inline_next
    ; Compare port name vs scratch
    move.q  r24, r7
    add     r24, r24, #KD_PORT_NAME       ; r24 = port name ptr
    move.l  r25, #KERN_DATA_BASE
    add     r25, r25, #KD_NAME_SCRATCH    ; r25 = scratch ptr
    push    r4
    push    r5
    push    r7
    jsr     case_insensitive_cmp          ; R23 = 0 if match
    pop     r7
    pop     r5
    pop     r4
    beqz    r23, .kpfp_inline_match
.kpfp_inline_next:
    add     r5, r5, #1
    bra     .kpfp_inline_scan
.kpfp_inline_match:
    move.q  r1, r7
    move.q  r2, r5
    rts

.kpfp_oflow:
    ; --- Walk overflow chain ---
    move.l  r4, #KERN_DATA_BASE
    add     r4, r4, #KD_PORT_OFLOW_HDR
    load.w  r10, KD_PORT_OFLOW_FIRST_PPN(r4)
    beqz    r10, .kpfp_notfound
    move.l  r11, #KD_PORT_INLINE_MAX     ; r11 = current port id
.kpfp_walk_page:
    lsl     r12, r10, #12                 ; r12 = page byte addr
    move.q  r7, r12
    add     r7, r7, #KD_PORT_PAGE_HDR_SZ
    move.l  r9, #0                        ; row idx
.kpfp_walk_row:
    move.l  r6, #KD_PORT_ROWS_PER_PG
    bge     r9, r6, .kpfp_walk_next_page
    load.b  r8, KD_PORT_VALID(r7)
    beqz    r8, .kpfp_walk_skip
    load.b  r8, KD_PORT_FLAGS(r7)
    and     r8, r8, #PF_PUBLIC
    beqz    r8, .kpfp_walk_skip
    move.q  r24, r7
    add     r24, r24, #KD_PORT_NAME
    move.l  r25, #KERN_DATA_BASE
    add     r25, r25, #KD_NAME_SCRATCH
    push    r7
    push    r9
    push    r10
    push    r11
    push    r12
    jsr     case_insensitive_cmp
    pop     r12
    pop     r11
    pop     r10
    pop     r9
    pop     r7
    beqz    r23, .kpfp_oflow_match
.kpfp_walk_skip:
    add     r7, r7, #KD_PORT_STRIDE
    add     r9, r9, #1
    add     r11, r11, #1
    bra     .kpfp_walk_row
.kpfp_walk_next_page:
    load.w  r10, KD_PORT_PAGE_NEXT(r12)
    bnez    r10, .kpfp_walk_page
    bra     .kpfp_notfound
.kpfp_oflow_match:
    move.q  r1, r7
    move.q  r2, r11
    rts
.kpfp_notfound:
    move.q  r1, r0
    move.q  r2, r0
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
    ; M12: dynamic VA is GLOBAL — all tasks share USER_DYN_BASE..USER_DYN_END.
    ; Each task has its own page table so the same VA in two tasks maps to
    ; different physical pages with no conflict. The per-task region table
    ; tracks which VA gaps are used WITHIN this single shared range.
    ;
    ; M12.5: walks both the inline 8-row table and the per-task overflow
    ; chain. CALLER MUST BE ON KERNEL PT (overflow chain pages live in
    ; the allocator pool, only mapped by the kernel PT). The Phase-4
    ; outer PT switch in AllocMem/MapShared/SYS_MAP_IO satisfies this.
    push    r1                         ; save task_id (used by overflow walk)
    move.l  r4, #USER_DYN_BASE          ; r4 = window start VA (shared)
    move.l  r5, #USER_DYN_END            ; r5 = window end VA (exclusive, shared)
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
    ; --- Phase 1: scan inline rows 0..7 ---
    move.l  r10, #0                    ; region index
.ffv_check_region:
    move.l  r11, #KD_REGION_INLINE_MAX
    bge     r10, r11, .ffv_check_overflow
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
    bge     r8, r16, .ffv_next_region  ; candidate starts after region ends
    bge     r14, r9, .ffv_next_region  ; region starts after candidate ends
    ; Overlap detected — advance candidate past this region
    move.q  r8, r16                    ; candidate = region end
    bra     .ffv_try                   ; retry with new candidate
.ffv_next_region:
    add     r10, r10, #1
    bra     .ffv_check_region

.ffv_check_overflow:
    ; --- Phase 2: walk per-task overflow chain ---
    ; Look up overflow head pointer (kernel data, always accessible).
    load.q  r17, (sp)                  ; r17 = task_id (was pushed at entry)
    move.l  r10, #KERN_DATA_BASE
    add     r10, r10, #KD_REGION_OVERFLOW_HEAD
    move.l  r11, #KD_REGION_OFLOW_STRIDE
    mulu    r11, r17, r11
    add     r10, r10, r11               ; r10 = &task's overflow head
    load.w  r11, KD_REGION_OFLOW_FIRST_PPN(r10)
    beqz    r11, .ffv_found            ; no overflow rows for this task
.ffv_oflow_page:
    lsl     r12, r11, #12              ; r12 = page byte addr
    move.q  r13, r12
    add     r13, r13, #KD_REGION_PAGE_HDR_SZ ; r13 = first row in page
    move.l  r10, #0                    ; row idx in this page
.ffv_oflow_row:
    move.l  r17, #KD_REGION_ROWS_PER_PG
    bge     r10, r17, .ffv_oflow_next_page
    load.b  r17, KD_REG_TYPE(r13)
    beqz    r17, .ffv_oflow_skip       ; FREE, skip
    load.l  r14, KD_REG_VA(r13)
    load.w  r15, KD_REG_PAGES(r13)
    lsl     r15, r15, #12
    add     r16, r14, r15
    bge     r8, r16, .ffv_oflow_skip
    bge     r14, r9, .ffv_oflow_skip
    ; Overlap — advance candidate past this region
    move.q  r8, r16
    bra     .ffv_try
.ffv_oflow_skip:
    add     r13, r13, #KD_REGION_STRIDE
    add     r10, r10, #1
    bra     .ffv_oflow_row
.ffv_oflow_next_page:
    load.w  r11, KD_REGION_PAGE_NEXT(r12)
    bnez    r11, .ffv_oflow_page
    bra     .ffv_found

.ffv_found:
    pop     r2                         ; discard saved task_id
    move.q  r1, r8                     ; R1 = VA
    move.q  r2, #ERR_OK
    rts
.ffv_fail:
    pop     r2                         ; discard saved task_id
    move.q  r1, #0
    move.q  r2, #ERR_NOMEM
    rts

; ============================================================================
; M7: Port Name Helpers
; ============================================================================

; safe_copy_user_name: validate PTE and copy up to PORT_NAME_LEN bytes from
;   user VA to kernel scratch buffer at KERN_DATA_BASE + KD_NAME_SCRATCH.
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

    ; --- Validate PTE for last byte's page (name_ptr + PORT_NAME_LEN-1) ---
    pop     r1
    push    r1
    add     r14, r1, #(PORT_NAME_LEN-1)
    lsr     r14, r14, #12               ; VPN of name_ptr+(NAMELEN-1)
    lsl     r18, r14, #3
    add     r18, r18, r17
    load.q  r18, (r18)
    and     r19, r18, #0x13
    bne     r19, r24, .scun_bad

    ; --- Copy up to PORT_NAME_LEN bytes to scratch buffer ---
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

; case_insensitive_cmp: compare two PORT_NAME_LEN-byte buffers case-insensitively
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

; check_name_unique: scan ports for a duplicate public name.
; M12.6 Phase C: thin wrapper around kern_port_find_public — that helper
; walks both inline and overflow chain, so name uniqueness now spans the
; full system port table, not just the inline 32 rows.
; Input:  scratch buffer at KERN_DATA_BASE + KD_NAME_SCRATCH already filled
; Output: R23 = 0 if unique, 1 if duplicate found
; Clobbers: R14, R16, R17, R18, R19, R23, R24, R25, R26
;            (matches the M12 contract: callers like .do_create_port keep
;             relying on r7 = name_ptr and r13 = current_task surviving.)
check_name_unique:
    ; kern_port_find_public clobbers r3..r19 (uses r7 internally for the
    ; row pointer and r13 indirectly via case_insensitive_cmp). Save the
    ; registers .do_create_port still needs after the call.
    push    r7
    push    r12
    push    r13
    jsr     kern_port_find_public       ; R1 = port addr (0 if none), R2 = id
    pop     r13
    pop     r12
    pop     r7
    bnez    r1, .cnu_dup
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
    ; M12.5: kill_task_cleanup walks overflow chain pages — switch to kernel PT.
    mfcr    r19, cr0
    move.l  r11, #KERN_PAGE_TABLE
    mtcr    cr0, r11
    tlbflush
    jsr     kill_task_cleanup
    mtcr    cr0, r19
    tlbflush
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

    move.l  r11, #SYS_HWRES_OP
    beq     r10, r11, .do_hwres_op

    ; M11.5: SYS_READ_INPUT (slot 37) removed. Terminal MMIO is now mapped
    ; into console.handler via SYS_MAP_IO and the read loop is inlined in
    ; console.handler's CON_MSG_READLINE handler. Slot 37 falls through to
    ; ERR_BADARG below, same as any other unallocated number.
    ; M12.5: slot 37 stays a reserved hole; new syscalls use fresh slots
    ; (SYS_HWRES_OP at slot 38). TestIExec_HWRes_Slot37StillReserved is
    ; the executable contract.

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
    ; M12.6 Phase C: chain-aware slot allocator. Helper preserves r3..r6
    ; (which user-space CreatePort callers may use as scratch — see the
    ; r3 comment on kern_port_alloc_slot). r7 (name ptr), r8 (flags), and
    ; r13 (current task) are saved on the stack since the helper clobbers
    ; r4..r19 internally. Allocator pool pages (where the chain lives) are
    ; mapped supervisor-only in user PTs by build_user_pt, so the helper
    ; can read/write them on the user PT — no PT switch required.
    push    r13
    push    r7
    push    r8
    jsr     kern_port_alloc_slot    ; R1 = port addr (0 on NOMEM), R2 = port id
    pop     r8
    pop     r7
    pop     r13
    beqz    r1, .create_full
    move.q  r21, r1                 ; r21 = &port[id]
    move.q  r20, r2                 ; r20 = port id
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

    ; 4. If PF_PUBLIC, check name uniqueness.
    and     r23, r8, #PF_PUBLIC
    beqz    r23, .create_commit
    push    r20
    push    r21
    push    r8
    push    r13
    push    r7
    jsr     check_name_unique       ; R23 = 0 if unique, 1 if dup
    pop     r7
    pop     r13
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
    ; Zero name field (PORT_NAME_LEN = 32 bytes = 4 quads)
    store.q r0, KD_PORT_NAME(r21)
    add     r23, r21, #KD_PORT_NAME
    add     r23, r23, #8
    store.q r0, (r23)
    add     r23, r23, #8
    store.q r0, (r23)
    add     r23, r23, #8
    store.q r0, (r23)
    ; If named, copy from scratch buffer to port name (32 bytes = 4 quads)
    beqz    r7, .create_done
    move.l  r14, #KERN_DATA_BASE
    add     r14, r14, #KD_NAME_SCRATCH  ; R14 = &scratch
    add     r16, r21, #KD_PORT_NAME     ; R16 = &port.name
    load.q  r17, (r14)
    store.q r17, (r16)
    add     r14, r14, #8
    add     r16, r16, #8
    load.q  r17, (r14)
    store.q r17, (r16)
    add     r14, r14, #8
    add     r16, r16, #8
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
    jsr     kern_port_find_public   ; R1 = port addr, R2 = port id (0/0 if not found)
    beqz    r1, .findport_notfound
    move.q  r1, r2                  ; R1 = port ID
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
    ; M12.6 Phase C: chain-aware port lookup. Preserve r2..r6 (message
    ; fields) across the helper call; the helper clobbers r3..r9.
    push    r2
    push    r3
    push    r4
    push    r5
    push    r6
    jsr     kern_port_addr_for_id       ; R1 = port addr (or 0)
    pop     r6
    pop     r5
    pop     r4
    pop     r3
    pop     r2
    beqz    r1, .putmsg_badarg
    move.q  r21, r1                     ; R21 = &port[portID]
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)                  ; current_task (for src_task)
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
    ; M12.6 Phase C: chain-aware port lookup.
    jsr     kern_port_addr_for_id       ; R1 = port addr (or 0)
    beqz    r1, .getmsg_badarg
    move.q  r21, r1                     ; R21 = &port[portID]
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)
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
    load.l  r7, KD_MSG_SRC(r27)         ; R7 = sender task ID (M12.5)
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
    ; M12.6 Phase C: chain-aware port lookup.
    move.q  r1, r5
    jsr     kern_port_addr_for_id       ; R1 = port addr (or 0)
    beqz    r1, .waitport_badarg
    move.q  r21, r1
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)
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
    load.l  r7, KD_MSG_SRC(r27)         ; R7 = sender task ID (M12.5)
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
    move.l  r9, #1                      ; code_pages = 1
    move.l  r11, #1                     ; data_pages = 1
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
    ; M12.5: kill_task_cleanup walks overflow chain pages so it must run on
    ; kernel PT. Save user PT, switch, run cleanup, restore.
    mfcr    r19, cr0
    move.l  r11, #KERN_PAGE_TABLE
    mtcr    cr0, r11
    tlbflush
    jsr     kill_task_cleanup       ; shared cleanup subroutine
    mtcr    cr0, r19
    tlbflush
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

    ; M12.5: outer PT switch — run the handler on kernel PT throughout so
    ; overflow chain pages (allocator pool, only mapped in kernel PT) are
    ; accessible by both find_free_va and the region row writes. r19 holds
    ; the saved user PT for the entire handler; nothing else clobbers it
    ; (find_free_va says R3-R16 only, alloc_pages uses R3-R7, etc.).
    mfcr    r19, cr0
    move.l  r11, #KERN_PAGE_TABLE
    mtcr    cr0, r11
    tlbflush

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

    ; Step 4: If MEMF_PUBLIC, find free shared object slot via the chain
    ; allocator (M12.6 Phase B). Inline rows 0..15 first, then overflow.
    move.q  r26, #0                     ; r26 = shmem slot addr (0 = not public)
    and     r11, r25, #MEMF_PUBLIC
    beqz    r11, .am_skip_shmem_check
    jsr     kern_shmem_alloc_slot       ; R1 = addr (0 on NOMEM), R2 = id
    beqz    r1, .am_nomem
    move.q  r26, r1                     ; r26 = slot addr
    move.q  r27, r2                     ; r27 = slot id (used in handle)
.am_skip_shmem_check:

    ; Step 5: Find free region slot in caller's region table.
    ; M12.5: scan inline rows 0..7 first; if all occupied, fall through
    ; to the overflow chain via kern_region_oflow_alloc_row. The outer
    ; PT switch (right at the start of the handler) keeps us on kernel
    ; PT so overflow chain page accesses work directly.
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)                  ; r13 = current_task
    move.l  r14, #KERN_DATA_BASE
    add     r14, r14, #KD_REGION_TABLE
    move.l  r11, #KD_REGION_TASK_SZ
    mulu    r11, r13, r11
    add     r14, r14, r11               ; r14 = &region_table[task][0]
    move.l  r15, #0                     ; region slot index
.am_find_region:
    move.l  r11, #KD_REGION_INLINE_MAX
    bge     r15, r11, .am_inline_full   ; inline full → try overflow
    move.l  r11, #KD_REGION_STRIDE
    mulu    r11, r15, r11
    add     r16, r14, r11               ; r16 = &region[i]
    load.b  r11, KD_REG_TYPE(r16)
    bnez    r11, .am_find_region_next
    bra     .am_found_region
.am_find_region_next:
    add     r15, r15, #1
    bra     .am_find_region

.am_inline_full:
    ; Inline 8 rows all occupied — allocate a row in the overflow chain.
    ; We're already on kernel PT (handler-level switch in the entry).
    move.q  r1, r13                     ; task_id
    jsr     kern_region_oflow_alloc_row ; R1 = row addr, R2 = err
    bnez    r2, .am_nomem
    move.q  r16, r1                     ; r16 = row addr (in chain page)
    ; Fall through to .am_found_region

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

    ; M12.5: restore user PT before eret
    mtcr    cr0, r19
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
    mtcr    cr0, r19
    tlbflush
    move.q  r1, #0
    move.q  r2, #ERR_BADARG
    move.q  r3, #0
    eret

.am_toolarge:
    mtcr    cr0, r19
    tlbflush
    move.q  r1, #0
    move.q  r2, #ERR_TOOLARGE
    move.q  r3, #0
    eret

.am_nomem:
    mtcr    cr0, r19
    tlbflush
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

    ; M12.5: outer PT switch (kernel PT for whole handler).
    mfcr    r29, cr0
    move.l  r11, #KERN_PAGE_TABLE
    mtcr    cr0, r11
    tlbflush

    ; Compute expected page count
    add     r20, r25, #0xFFF
    lsr     r20, r20, #12               ; r20 = page count from size

    ; Find matching region in caller's table.
    ; M12.5: scan inline rows 0..7 first; if not found, walk overflow chain.
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)                  ; current_task
    move.l  r14, #KERN_DATA_BASE
    add     r14, r14, #KD_REGION_TABLE
    move.l  r11, #KD_REGION_TASK_SZ
    mulu    r11, r13, r11
    add     r14, r14, r11               ; r14 = &region_table[task][0]

    move.l  r15, #0                     ; region index
.fm_find:
    move.l  r11, #KD_REGION_INLINE_MAX
    bge     r15, r11, .fm_check_overflow
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

.fm_check_overflow:
    ; M12.5: walk per-task overflow chain looking for matching VA.
    move.l  r11, #KERN_DATA_BASE
    add     r11, r11, #KD_REGION_OVERFLOW_HEAD
    move.l  r12, #KD_REGION_OFLOW_STRIDE
    mulu    r12, r13, r12
    add     r11, r11, r12               ; r11 = &task's overflow head
    load.w  r12, KD_REGION_OFLOW_FIRST_PPN(r11)
    beqz    r12, .fm_badarg
.fm_oflow_page:
    lsl     r14, r12, #12               ; r14 = page byte addr
    move.q  r16, r14
    add     r16, r16, #KD_REGION_PAGE_HDR_SZ ; r16 = first row in page
    move.l  r15, #0                     ; row idx
.fm_oflow_row:
    move.l  r11, #KD_REGION_ROWS_PER_PG
    bge     r15, r11, .fm_oflow_next_page
    load.b  r11, KD_REG_TYPE(r16)
    beqz    r11, .fm_oflow_skip
    load.l  r17, KD_REG_VA(r16)
    beq     r17, r24, .fm_found         ; r16 is the matching row
.fm_oflow_skip:
    add     r16, r16, #KD_REGION_STRIDE
    add     r15, r15, #1
    bra     .fm_oflow_row
.fm_oflow_next_page:
    load.w  r12, KD_REGION_PAGE_NEXT(r14)
    bnez    r12, .fm_oflow_page
    bra     .fm_badarg                  ; not found anywhere

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
    ; SHARED: decrement refcount, maybe free backing pages.
    ; M12.6 Phase B: shmem slot may live in inline range OR overflow chain.
    load.b  r1, KD_REG_SHMID(r16)       ; slot id
    push    r16                         ; save region row addr (helper clobbers r3..r9)
    jsr     kern_shmem_addr_for_id      ; r1 = slot addr
    pop     r16
    move.q  r17, r1                     ; r17 = &shmem[id]
    beqz    r17, .fm_cleanup_region     ; defensive: bad id, just clear region

    load.b  r11, KD_SHM_REFCOUNT(r17)
    sub     r11, r11, #1
    store.b r11, KD_SHM_REFCOUNT(r17)
    bnez    r11, .fm_cleanup_region     ; refcount > 0, object survives

    ; Refcount reached 0: free backing pages and invalidate object
    load.w  r1, KD_SHM_PPN(r17)
    load.w  r2, KD_SHM_PAGES(r17)
    push    r17
    jsr     free_pages
    pop     r17
    store.b r0, KD_SHM_VALID(r17)       ; invalidate

.fm_cleanup_region:
    ; Mark region as FREE
    store.b r0, KD_REG_TYPE(r16)
    tlbflush
    mtcr    cr0, r29
    tlbflush
    move.q  r2, #ERR_OK
    eret

.fm_badarg:
    mtcr    cr0, r29
    tlbflush
    move.q  r2, #ERR_BADARG
    eret

; ============================================================================
; MapShared (SYS_MAP_SHARED = 4)
; ============================================================================
; R1 = share_handle (opaque 32-bit: nonce<<8 | slot)
; Returns: R1 = VA, R2 = err, R3 = page count of the share
;
; The page count is returned so user-space services (e.g. dos.library)
; can clamp byte_count parameters from clients to the actual mapped
; size, preventing reads/writes past the mapped region. Backwards
; compatible: M9/M10 callers that ignored R3 keep working.

.do_map_shared:
    move.q  r24, r1                     ; r24 = handle

    ; M12.5: outer PT switch (kernel PT for entire handler).
    mfcr    r19, cr0
    move.l  r11, #KERN_PAGE_TABLE
    mtcr    cr0, r11
    tlbflush

    ; Decode handle: slot = handle & 0xFF, nonce = (handle >> 8) & 0xFFFFFF
    and     r20, r24, #0xFF             ; r20 = slot id
    lsr     r21, r24, #8               ; r21 = nonce (upper 24 bits)

    ; M12.6 Phase B: 0xFF is the reserved sentinel; reject early.
    move.l  r11, #0xFF
    beq     r20, r11, .ms_badhandle

    ; Look up shared object via the chain-aware helper. Returns 0 if the
    ; id is past the end of any allocated overflow chain page.
    move.q  r1, r20
    jsr     kern_shmem_addr_for_id
    move.q  r14, r1                     ; r14 = &shmem[id]
    beqz    r14, .ms_badhandle

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
    move.l  r11, #KD_REGION_INLINE_MAX
    bge     r16, r11, .ms_inline_full
    move.l  r11, #KD_REGION_STRIDE
    mulu    r11, r16, r11
    add     r17, r15, r11
    load.b  r11, KD_REG_TYPE(r17)
    beqz    r11, .ms_found_region
    add     r16, r16, #1
    bra     .ms_find_region

.ms_inline_full:
    ; M12.5: fall through to overflow chain.
    move.q  r1, r13                     ; task_id
    jsr     kern_region_oflow_alloc_row ; R1 = row addr, R2 = err
    bnez    r2, .ms_nomem
    move.q  r17, r1                     ; r17 = row addr (overflow chain)
    ; Fall through

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
    mtcr    cr0, r19
    tlbflush
    move.q  r1, r25                     ; R1 = VA
    move.q  r2, #ERR_OK                 ; R2 = err
    move.q  r3, r23                     ; R3 = page count (saved at line ~2385)
    eret

.ms_badhandle:
    mtcr    cr0, r19
    tlbflush
    move.q  r1, #0
    move.q  r2, #ERR_BADHANDLE
    move.q  r3, #0
    eret

.ms_nomem:
    mtcr    cr0, r19
    tlbflush
    move.q  r1, #0
    move.q  r2, #ERR_NOMEM
    move.q  r3, #0
    eret

; ============================================================================
; SYS_READ_INPUT (slot 37) — REMOVED in M11.5
; ============================================================================
; The kernel-side line-input helper has been removed. console.handler now
; maps page 0xF0 directly via SYS_MAP_IO and inlines the terminal MMIO read
; loop in its CON_MSG_READLINE handler. Slot 37 falls through the dispatcher
; chain to ERR_BADARG. See sdk/docs/IntuitionOS/IExec.md "Exec Boundary"
; for the rationale; the regression guard is TestIExec_ReadInput_RemovedReturnsBadarg.

; ============================================================================
; SYS_OPEN_LIBRARY (slot 36) — binary-compat redirect to SYS_FIND_PORT (M11.5)
; ============================================================================
; R1 = name_ptr, R2 = version (ignored)
; Returns: R1 = library handle (port ID), R2 = error
;
; M11.5: This slot is no longer a distinct programming model. New IntuitionOS
; assembly source must call SYS_FIND_PORT directly. The slot is retained as a
; one-instruction redirect so that any out-of-tree binary or third-party tool
; hardcoded to raw syscall number 36 continues to work. Future milestones
; must NOT add new behavior here — see sdk/docs/IntuitionOS/IExec.md
; "Exec Boundary" for the rationale and the syscall admission rule.
;
; Regression guard: TestIExec_OpenLibrary_DispatcherCollapse pins the contract
; that slot 36 produces identical results to slot 16 for the same name.
.do_open_library:
    bra     .do_find_port               ; binary-compat redirect, version in R2 ignored

; ============================================================================
; SYS_MAP_IO (M9, extended in M11, gated by grant table in M12.5)
; ============================================================================
; R1 = base physical page number
; R2 = page count (0 is treated as 1 for M9/M10 backwards compatibility)
; Returns: R1 = mapped VA (base), R2 = error
;
; M12.5 authorization model: the calling task must hold a grant entry in
; the kernel grant chain whose PPN range covers [R1 .. R1+R2-1] inclusive.
; The only producers of grants are:
;   1. hardware.resource via SYS_HWRES_OP / HWRES_CREATE (the runtime path)
;   2. the immutable bootstrap_grant_table copied in during the boot-load
;      loop (the bootstrap path, breaks the chicken-and-egg for
;      console.handler at boot)
; Without a covering grant the call returns ERR_PERM. The legacy hardcoded
; allowlist (PPN 0xF0 + [0x100..0x5FF] VRAM range) was removed in M12.5;
; the same PPN ranges are now expressed as grant rows whose region tags
; ('CHIP', 'VRAM') hardware.resource resolves to those PPNs. This makes
; SYS_MAP_IO trusted-internal (gated by KD_GRANT_TABLE) rather than
; nucleus, and reclassifies it accordingly in IExec.md §5.11.
;
; Bookkeeping (region table walk, find_free_va, map_pages, region row
; install) is unchanged from M11 — the PT install path still goes through
; map_pages and the per-task region table still records the mapping.
; The KD_REGION_MAX cap removal (per-task region chain) lands in Phase 4
; of M12.5; this handler still uses the indexed walker for now.
.do_map_io:
    move.q  r24, r1                     ; r24 = requested base PPN
    move.q  r25, r2                     ; r25 = requested page count

    ; M12.5: outer PT switch (kernel PT for whole handler so the region
    ; overflow chain is accessible).
    mfcr    r19, cr0
    move.l  r11, #KERN_PAGE_TABLE
    mtcr    cr0, r11
    tlbflush

    ; Reject high-bit-set PPN or count up front. The bounds checks below
    ; use signed bgt/bge/blt; without these guards, a caller passing
    ; R2 = 0x8000_0000_0000_0000 would make the (PPN+count) sum interpret
    ; as a signed-negative value and bypass the bgt check.
    bltz    r24, .mio_badarg
    bltz    r25, .mio_badarg

    ; Backwards compat: page_count = 0 means 1 (M9/M10 callers)
    bnez    r25, .mio_count_set
    move.l  r25, #1
.mio_count_set:

    ; Cap page count to 0x500 (1280 pages = 5 MB) as a sanity bound on
    ; the request size. The grant check below is the actual authorization
    ; — this cap just keeps signed PPN+count arithmetic within positive
    ; int64 territory regardless of what the broker has put in the chain.
    move.l  r11, #0x500
    bgt     r25, r11, .mio_badarg

    ; M12.5: grant-table check.
    ; r1 = current task ID, r2 = req PPN low, r3 = req page count
    move.l  r12, #KERN_DATA_BASE
    load.q  r1, (r12)                   ; r1 = current_task
    move.q  r2, r24                     ; r2 = req PPN low
    move.q  r3, r25                     ; r3 = req page count
    jsr     kern_grant_check            ; R1 = match (0/1), R2 = ERR_OK
    beqz    r1, .mio_perm               ; no covering grant → ERR_PERM

.mio_validated:
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
    move.l  r11, #KD_REGION_INLINE_MAX
    bge     r15, r11, .mio_inline_full
    move.l  r11, #KD_REGION_STRIDE
    mulu    r11, r15, r11
    add     r16, r14, r11
    load.b  r11, KD_REG_TYPE(r16)
    beqz    r11, .mio_found_region
    add     r15, r15, #1
    bra     .mio_find_region

.mio_inline_full:
    ; M12.5: fall through to overflow chain.
    move.q  r1, r13                     ; task_id
    jsr     kern_region_oflow_alloc_row ; R1 = row addr, R2 = err
    bnez    r2, .mio_nomem
    move.q  r16, r1
    ; Fall through

.mio_found_region:
    move.q  r21, r16                    ; r21 = &region[slot]

    ; Find free VA range (r25 pages) in task's dynamic window
    push    r13
    move.q  r1, r13                     ; task_id
    move.q  r2, r25                     ; pages needed
    jsr     find_free_va                ; R1 = VA, R2 = err
    pop     r13
    bnez    r2, .mio_nomem
    move.q  r23, r1                     ; r23 = base VA

    ; Map the I/O pages: P|R|W|U = 0x17 (no X)
    move.l  r12, #KERN_DATA_BASE
    lsl     r1, r13, #3
    add     r1, r1, #KD_PTBR_BASE
    add     r1, r1, r12
    load.q  r1, (r1)                    ; R1 = PT base
    move.q  r2, r23                     ; R2 = base VA
    move.q  r3, r24                     ; R3 = base PPN
    move.q  r4, r25                     ; R4 = page count
    move.l  r5, #0x17                   ; R5 = P|R|W|U
    jsr     map_pages

    ; Fill region entry: type = REGION_IO (don't free pages on cleanup)
    store.l r23, KD_REG_VA(r21)
    store.w r24, KD_REG_PPN(r21)
    store.w r25, KD_REG_PAGES(r21)
    move.b  r11, #REGION_IO
    store.b r11, KD_REG_TYPE(r21)

    tlbflush
    mtcr    cr0, r19
    tlbflush
    move.q  r1, r23                     ; return base VA
    move.q  r2, #ERR_OK
    eret

.mio_badarg:
    mtcr    cr0, r19
    tlbflush
    move.q  r1, #0
    move.q  r2, #ERR_BADARG
    eret
.mio_nomem:
    mtcr    cr0, r19
    tlbflush
    move.q  r1, #0
    move.q  r2, #ERR_NOMEM
    eret
.mio_perm:
    mtcr    cr0, r19
    tlbflush
    move.q  r1, #0
    move.q  r2, #ERR_PERM
    eret

; ============================================================================
; SYS_HWRES_OP (M12.5) — hardware.resource broker primitive
; ============================================================================
; Verb-multiplexed trusted-internal syscall. R0 selects the operation.
; The kernel exposes exactly one slot at the ABI level (slot 38), pays
; +1 raw slot, and lets SYS_MAP_IO leave the public nucleus set in the
; same milestone. Net public ABI shrinks by one.
;
; Verbs:
;   HWRES_BECOME (0):
;     Inputs:  none
;     Effect:  if KD_HWRES_TASK == 0xFF, set it to current_task and return
;              ERR_OK. Otherwise return ERR_EXISTS.
;     Outputs: R1 = unused, R2 = err
;
;   HWRES_CREATE (1):
;     Inputs:  R1 = target task ID
;              R2 = 4-byte region tag (low 32 bits)
;              R3 = PPN low (inclusive)
;              R4 = PPN high (inclusive)
;     Guard:   current_task must equal KD_HWRES_TASK (else ERR_PERM).
;     Effect:  walks the grant chain for a free row and writes the new
;              grant entry. Allocates a new chain page if no free row
;              exists in any current chain page.
;     Outputs: R1 = unused, R2 = err
;
;   HWRES_REVOKE (2):
;     Reserved for M13. Returns ERR_BADARG in M12.5 so the slot is
;     documented and M13 can fill it without re-bikeshedding the layout.
;
;   HWRES_TASK_ALIVE (3):
;     Inputs:  R1 = target task ID
;     Guard:   current_task must equal KD_HWRES_TASK (broker-only)
;     Effect:  reads KD_TASK_STATE for the target task and reports
;              whether the slot is in use. Used by the broker to lazily
;              reclaim stale per-tag owner slots when a grantee has
;              exited (the kernel grant chain and KD_HWRES_TASK are
;              cleaned up automatically by kill_task_cleanup, but the
;              broker's private owner-list state is not — this verb
;              gives the broker enough kernel state to reconcile).
;     Outputs: R1 = 1 if alive (state != TASK_FREE), 0 if free; R2 = ERR_OK
.do_hwres_op:
    ; R0 was zeroed by the trap entry path (it's the syscall instruction's
    ; rd field, not a user-controlled register). The verb selector is
    ; encoded in the *low* bits of an arg register — by convention we
    ; place it in R6 so R1..R4 stay available for the per-verb args. The
    ; user-mode wrapper does: move r6, #verb; syscall #SYS_HWRES_OP.
    ; (We avoid R0 because the IE64 SYSCALL instruction does not pass R0
    ; through to the trap frame in a usable way.)
    move.q  r10, r6                     ; r10 = verb

    move.l  r11, #HWRES_BECOME
    beq     r10, r11, .hwres_become
    move.l  r11, #HWRES_CREATE
    beq     r10, r11, .hwres_create
    move.l  r11, #HWRES_REVOKE
    beq     r10, r11, .hwres_revoke
    move.l  r11, #HWRES_TASK_ALIVE
    beq     r10, r11, .hwres_task_alive
    bra     .hwres_badarg

.hwres_become:
    ; If KD_HWRES_TASK is still the unclaimed sentinel (0xFF), set it to
    ; the current task and return OK. Otherwise return EXISTS.
    move.l  r12, #KERN_DATA_BASE
    load.b  r13, KD_HWRES_TASK(r12)
    move.l  r14, #0xFF
    bne     r13, r14, .hwres_exists
    load.q  r15, (r12)                  ; r15 = current_task
    store.b r15, KD_HWRES_TASK(r12)
    move.q  r1, #0
    move.q  r2, #ERR_OK
    eret

.hwres_create:
    ; Guard: current_task == KD_HWRES_TASK
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)                  ; current_task
    load.b  r14, KD_HWRES_TASK(r12)
    bne     r13, r14, .hwres_perm
    ; Sanity-check inputs lightly:
    ;   target task id must be < MAX_TASKS
    ;   ppn_lo, ppn_hi must satisfy ppn_lo <= ppn_hi
    move.l  r15, #MAX_TASKS
    bge     r1, r15, .hwres_badarg
    bltz    r1, .hwres_badarg
    bgt     r3, r4, .hwres_badarg
    bltz    r3, .hwres_badarg
    bltz    r4, .hwres_badarg
    ; r1=task_id, r2=tag, r3=ppn_lo, r4=ppn_hi already in the right
    ; registers for kern_grant_create_row.
    jsr     kern_grant_create_row       ; R2 = err
    move.q  r1, #0
    eret

.hwres_revoke:
    ; M12.5 stub — slot reserved for M13.
    move.q  r1, #0
    move.q  r2, #ERR_BADARG
    eret

.hwres_task_alive:
    ; Guard: only the broker may query task liveness.
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)                  ; current_task
    load.b  r14, KD_HWRES_TASK(r12)
    bne     r13, r14, .hwres_perm
    ; Sanity-check target task ID
    move.l  r15, #MAX_TASKS
    bge     r1, r15, .hwres_badarg
    bltz    r1, .hwres_badarg
    ; Read TCB state byte: KD_TASK_BASE + task * KD_TASK_STRIDE + KD_TASK_STATE
    lsl     r15, r1, #5                 ; r15 = task * 32
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12               ; r15 = &TCB[task]
    load.b  r16, KD_TASK_STATE(r15)
    move.l  r17, #TASK_FREE
    beq     r16, r17, .hwres_alive_no
    move.q  r1, #1                      ; alive
    move.q  r2, #ERR_OK
    eret
.hwres_alive_no:
    move.q  r1, #0                      ; FREE = dead
    move.q  r2, #ERR_OK
    eret

.hwres_perm:
    move.q  r1, #0
    move.q  r2, #ERR_PERM
    eret
.hwres_exists:
    move.q  r1, #0
    move.q  r2, #ERR_EXISTS
    eret
.hwres_badarg:
    move.q  r1, #0
    move.q  r2, #ERR_BADARG
    eret

; ============================================================================
; SYS_EXEC_PROGRAM (M10 / M11.6) — Launch a program from a user image pointer
; ============================================================================
; ABI: R1 = image_ptr (must be user VA, >= USER_CODE_BASE)
;      R2 = image_size, R3 = args_ptr, R4 = args_len
; Returns: R1 = new task_id, R2 = error
;
; Runs entirely under the caller's PT (no PT switching).
; Caller's PT has: user-accessible mappings for caller's own pages (incl.
; AllocMem'd storage), and supervisor-only mappings for all task slot pages.
;
; M11.6: the legacy `R1 < USER_CODE_BASE` built-in-program-table index branch
; was removed. Sub-USER_CODE_BASE values now hard-fail with ERR_BADARG so the
; only path through this handler is the validated image-pointer ABI.
.do_exec_program:
    ; M11.6: reject any R1 below USER_CODE_BASE — the legacy index path is gone.
    move.l  r11, #USER_CODE_BASE
    blt     r1, r11, .ep_badarg_norestore

    move.q  r24, r1                     ; r24 = image_ptr (user VA)
    move.q  r25, r2                     ; r25 = image_size
    move.q  r26, r3                     ; r26 = args_ptr
    move.q  r27, r4                     ; r27 = args_len

    ; 1. Validate image_size range: 32 (header) .. 24608 (header + 8KB code + 16KB data)
    ;    Matches load_program's max (2 code pages + 4 data pages + 32-byte header).
    move.l  r11, #IMG_HEADER_SIZE
    blt     r25, r11, .ep_badarg_norestore
    move.l  r11, #24608
    bgt     r25, r11, .ep_badarg_norestore

    ; 2. Validate args_len
    move.l  r11, #DATA_ARGS_MAX
    bgt     r27, r11, .ep_badarg_norestore

    ; 3. Overflow check
    add     r11, r24, r25
    blt     r11, r24, .ep_badarg_norestore

    ; 4. Validate image range: walk caller's PT, check P+U for each page
    mfcr    r28, cr0                    ; r28 = caller's PTBR
    move.q  r1, r24
    move.q  r2, r25
    move.q  r3, r28
    jsr     validate_user_range
    bnez    r1, .ep_badarg_norestore

    ; 5. Validate args range if present
    beqz    r27, .ep_args_valid
    beqz    r26, .ep_args_valid
    add     r11, r26, r27
    blt     r11, r26, .ep_badarg_norestore
    move.q  r1, r26
    move.q  r2, r27
    move.q  r3, r28
    jsr     validate_user_range
    bnez    r1, .ep_badarg_norestore
.ep_args_valid:

    ; 6. Call load_program UNDER CALLER'S PT (no switching!)
    move.q  r7, r24                     ; image_ptr
    move.q  r8, r25                     ; image_size
    push    r24                         ; save image_ptr
    push    r26                         ; save args_ptr
    push    r27                         ; save args_len
    jsr     load_program                ; R1 = task_id, R2 = err
    pop     r27
    pop     r26
    pop     r24
    bnez    r2, .ep_new_fail

    ; 7. Copy args directly (no PT switching needed)
    move.q  r22, r1                     ; r22 = new task_id
    beqz    r27, .ep_new_no_args
    beqz    r26, .ep_new_no_args

    ; Compute destination: new task data page + DATA_ARGS_OFFSET
    ; Re-read code_size from image to compute data offset
    load.l  r14, IMG_OFF_CODE_SIZE(r24)
    add     r14, r14, #0xFFF
    lsr     r14, r14, #12              ; code_pages
    add     r14, r14, #1               ; +1 for stack
    lsl     r14, r14, #12              ; (code_pages+1)*4096
    move.l  r11, #USER_SLOT_STRIDE
    mulu    r11, r22, r11
    move.l  r15, #USER_CODE_BASE
    add     r15, r15, r11
    add     r15, r15, r14
    add     r15, r15, #DATA_ARGS_OFFSET ; r15 = dest (supervisor-only mapped)

    ; Direct copy: read from caller VA, write to new task pages
    move.l  r4, #0
.ep_new_copy_args:
    bge     r4, r27, .ep_new_args_term
    add     r5, r26, r4
    load.b  r6, (r5)                    ; read from caller VA (user-accessible)
    add     r5, r15, r4
    store.b r6, (r5)                    ; write to dest (supervisor-only)
    add     r4, r4, #1
    bra     .ep_new_copy_args
.ep_new_args_term:
    add     r5, r15, r27
    store.b r0, (r5)

.ep_new_no_args:
    move.q  r1, r22
    move.q  r2, #ERR_OK
    eret

.ep_new_fail:
    move.q  r1, #0
    eret

.ep_badarg_norestore:
    move.q  r1, #0
    move.q  r2, #ERR_BADARG
    eret

; ============================================================================
; validate_user_range: check every page in [start_va, start_va+byte_count)
; has both P (present) and U (user-accessible) bits set in the given PT.
; Input:  R1 = start_va, R2 = byte_count, R3 = PTBR (page table base)
; Output: R1 = 0 (OK) or 1 (BADARG)
; Clobbers: R1-R6
; ============================================================================
validate_user_range:
    beqz    r2, .vur_ok
    lsr     r4, r1, #12                 ; start VPN
    add     r5, r1, r2
    sub     r5, r5, #1
    lsr     r5, r5, #12                 ; end VPN (inclusive)
.vur_check:
    bgt     r4, r5, .vur_ok
    lsl     r6, r4, #3
    add     r6, r6, r3
    load.q  r6, (r6)                    ; PTE
    and     r6, r6, #0x11              ; check P + U
    move.l  r1, #0x11
    bne     r6, r1, .vur_fail
    add     r4, r4, #1
    bra     .vur_check
.vur_ok:
    move.q  r1, #0
    rts
.vur_fail:
    move.q  r1, #1
    rts

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
    ; Invalidate ports owned by this task.
    ; M12.6 Phase C: walk inline first, then walk every overflow chain
    ; page. The chain may have zero or many pages — we visit them all.
    ; r13 (current task being killed) MUST be preserved across the walk.

    ; --- Walk inline ports 0..KD_PORT_INLINE_MAX-1 ---
    move.l  r20, #KERN_DATA_BASE
    add     r20, r20, #KD_PORT_BASE
    move.l  r21, #0
.ktc_port_scan:
    move.l  r22, #KD_PORT_INLINE_MAX
    bge     r21, r22, .ktc_port_inline_done
    load.b  r23, KD_PORT_VALID(r20)
    beqz    r23, .ktc_port_next
    load.b  r24, KD_PORT_OWNER(r20)
    bne     r13, r24, .ktc_port_next
    jsr     .ktc_port_clear            ; clear port at r20
.ktc_port_next:
    add     r20, r20, #KD_PORT_STRIDE
    add     r21, r21, #1
    bra     .ktc_port_scan

.ktc_port_inline_done:
    ; --- Walk overflow chain pages ---
    move.l  r22, #KERN_DATA_BASE
    add     r22, r22, #KD_PORT_OFLOW_HDR
    load.w  r25, KD_PORT_OFLOW_FIRST_PPN(r22)
    beqz    r25, .ktc_done
.ktc_port_oflow_page:
    lsl     r26, r25, #12               ; r26 = page byte addr
    move.q  r20, r26
    add     r20, r20, #KD_PORT_PAGE_HDR_SZ ; r20 = first row in page
    move.l  r21, #0                     ; row idx
.ktc_port_oflow_row:
    move.l  r22, #KD_PORT_ROWS_PER_PG
    bge     r21, r22, .ktc_port_oflow_next_page
    load.b  r23, KD_PORT_VALID(r20)
    beqz    r23, .ktc_port_oflow_skip
    load.b  r24, KD_PORT_OWNER(r20)
    bne     r13, r24, .ktc_port_oflow_skip
    push    r25
    push    r26
    jsr     .ktc_port_clear
    pop     r26
    pop     r25
.ktc_port_oflow_skip:
    add     r20, r20, #KD_PORT_STRIDE
    add     r21, r21, #1
    bra     .ktc_port_oflow_row
.ktc_port_oflow_next_page:
    load.w  r25, KD_PORT_PAGE_NEXT(r26)
    bnez    r25, .ktc_port_oflow_page
    bra     .ktc_done

; .ktc_port_clear: clear the port at R20 (set valid/count/flags = 0,
; zero the 32-byte name field). Preserves R13, R20, R21, R25, R26.
; Clobbers R22.
.ktc_port_clear:
    store.b r0, KD_PORT_VALID(r20)
    store.b r0, KD_PORT_COUNT(r20)
    store.b r0, KD_PORT_FLAGS(r20)
    store.q r0, KD_PORT_NAME(r20)        ; clear name bytes 0-7
    add     r22, r20, #KD_PORT_NAME
    add     r22, r22, #8
    store.q r0, (r22)                    ; bytes 8-15
    add     r22, r22, #8
    store.q r0, (r22)                    ; bytes 16-23
    add     r22, r22, #8
    store.q r0, (r22)                    ; bytes 24-31
    rts

.ktc_done:
    ; M6: Clean up memory regions for this task. M12.5: walks inline rows
    ; first, then walks the per-task overflow chain (rows 9+), and finally
    ; frees the overflow chain pages themselves so the page allocator pool
    ; isn't leaked. Caller is responsible for kernel-PT context (every
    ; caller switches before invoking).
    move.l  r20, #KERN_DATA_BASE
    add     r20, r20, #KD_REGION_TABLE
    move.l  r21, #KD_REGION_TASK_SZ
    mulu    r21, r13, r21
    add     r20, r20, r21               ; r20 = &region_table[task][0]
    move.l  r21, #0                     ; region index
.ktc_region_scan:
    move.l  r22, #KD_REGION_INLINE_MAX
    bge     r21, r22, .ktc_inline_done
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
    ; SHARED: decrement refcount.
    ; M12.6 Phase B: shmem slot may live in inline range OR overflow chain.
    load.b  r1, KD_REG_SHMID(r23)
    push    r23                         ; save region row addr (helper clobbers r3..r9)
    jsr     kern_shmem_addr_for_id      ; r1 = &shmem[id], or 0
    pop     r23
    move.q  r4, r1                      ; r4 = shmem ptr (preserved across free_pages)
    beqz    r4, .ktc_region_clear       ; defensive: bad id, just clear region
    load.b  r22, KD_SHM_REFCOUNT(r4)
    sub     r22, r22, #1
    store.b r22, KD_SHM_REFCOUNT(r4)
    bnez    r22, .ktc_region_clear      ; refcount > 0

    ; Last reference: free backing pages and invalidate.
    load.w  r1, KD_SHM_PPN(r4)
    load.w  r2, KD_SHM_PAGES(r4)
    push    r4
    push    r23
    jsr     free_pages
    pop     r23
    pop     r4
    store.b r0, KD_SHM_VALID(r4)        ; invalidate

.ktc_region_clear:
    store.b r0, KD_REG_TYPE(r23)        ; mark region FREE

.ktc_region_next:
    add     r21, r21, #1
    bra     .ktc_region_scan

.ktc_inline_done:
    ; --- M12.5: walk overflow chain rows for the same task ---
    move.l  r20, #KERN_DATA_BASE
    add     r20, r20, #KD_REGION_OVERFLOW_HEAD
    move.l  r22, #KD_REGION_OFLOW_STRIDE
    mulu    r22, r13, r22
    add     r20, r20, r22               ; r20 = &task's overflow head
    load.w  r25, KD_REGION_OFLOW_FIRST_PPN(r20)
    beqz    r25, .ktc_mem_done

.ktc_oflow_page:
    lsl     r26, r25, #12               ; r26 = page byte addr
    move.q  r23, r26
    add     r23, r23, #KD_REGION_PAGE_HDR_SZ ; r23 = first row
    move.l  r21, #0                     ; row idx
.ktc_oflow_row:
    move.l  r22, #KD_REGION_ROWS_PER_PG
    bge     r21, r22, .ktc_oflow_next_page
    load.b  r22, KD_REG_TYPE(r23)
    beqz    r22, .ktc_oflow_skip
    ; Unmap, free pages, decrement shmem refcount — same logic as inline path.
    lsl     r1, r13, #3
    add     r1, r1, #KD_PTBR_BASE
    add     r1, r1, r12
    load.q  r1, (r1)
    load.l  r2, KD_REG_VA(r23)
    load.w  r3, KD_REG_PAGES(r23)
    jsr     unmap_pages
    load.b  r22, KD_REG_TYPE(r23)
    move.l  r24, #REGION_IO
    beq     r22, r24, .ktc_oflow_clear
    move.l  r24, #REGION_PRIVATE
    bne     r22, r24, .ktc_oflow_shared
    load.w  r1, KD_REG_PPN(r23)
    load.w  r2, KD_REG_PAGES(r23)
    jsr     free_pages
    bra     .ktc_oflow_clear
.ktc_oflow_shared:
    ; M12.6 Phase B: chain-aware shmem slot lookup.
    load.b  r1, KD_REG_SHMID(r23)
    push    r23                         ; save current overflow row addr
    push    r25                         ; r25 = overflow chain head PPN (preserve)
    push    r26                         ; r26 = current overflow page byte addr (preserve)
    jsr     kern_shmem_addr_for_id      ; r1 = &shmem[id], or 0
    pop     r26
    pop     r25
    pop     r23
    move.q  r4, r1
    beqz    r4, .ktc_oflow_clear
    load.b  r22, KD_SHM_REFCOUNT(r4)
    sub     r22, r22, #1
    store.b r22, KD_SHM_REFCOUNT(r4)
    bnez    r22, .ktc_oflow_clear
    load.w  r1, KD_SHM_PPN(r4)
    load.w  r2, KD_SHM_PAGES(r4)
    push    r4
    push    r23
    push    r25
    push    r26
    jsr     free_pages
    pop     r26
    pop     r25
    pop     r23
    pop     r4
    store.b r0, KD_SHM_VALID(r4)
.ktc_oflow_clear:
    store.b r0, KD_REG_TYPE(r23)
.ktc_oflow_skip:
    add     r23, r23, #KD_REGION_STRIDE
    add     r21, r21, #1
    bra     .ktc_oflow_row
.ktc_oflow_next_page:
    load.w  r25, KD_REGION_PAGE_NEXT(r26)
    bnez    r25, .ktc_oflow_page

    ; --- All overflow rows processed; now free the chain pages themselves ---
    ; Re-derive overflow head pointer.
    move.l  r20, #KERN_DATA_BASE
    add     r20, r20, #KD_REGION_OVERFLOW_HEAD
    move.l  r22, #KD_REGION_OFLOW_STRIDE
    mulu    r22, r13, r22
    add     r20, r20, r22               ; r20 = &head
    load.w  r25, KD_REGION_OFLOW_FIRST_PPN(r20)
.ktc_oflow_free_chain:
    beqz    r25, .ktc_oflow_reset_head
    lsl     r26, r25, #12               ; r26 = page byte addr
    load.w  r24, KD_REGION_PAGE_NEXT(r26) ; save next BEFORE freeing
    move.q  r1, r25                     ; PPN to free
    move.l  r2, #1                      ; one page
    jsr     free_pages
    move.q  r25, r24
    bra     .ktc_oflow_free_chain

.ktc_oflow_reset_head:
    ; Re-derive head and reset its 8 bytes.
    move.l  r20, #KERN_DATA_BASE
    add     r20, r20, #KD_REGION_OVERFLOW_HEAD
    move.l  r22, #KD_REGION_OFLOW_STRIDE
    mulu    r22, r13, r22
    add     r20, r20, r22
    store.q r0, (r20)

.ktc_mem_done:
    ; ----------------------------------------------------------------
    ; M12.5 hardening: clear hardware.resource state for the exiting
    ; task so a recycled slot cannot inherit MMIO privilege.
    ; ----------------------------------------------------------------
    ;
    ; (a) Walk the grant chain and free every row whose task_id == r13
    ;     (so the new occupant of this slot can't reuse the old grants).
    move.q  r1, r13
    jsr     kern_grant_release_for_task
    ;
    ; (b) If the exiting task IS the broker (KD_HWRES_TASK == r13),
    ;     clear the broker identity. After this, a fresh task can claim
    ;     broker identity via SYS_HWRES_OP/HWRES_BECOME without inheriting
    ;     the dead broker's privilege.
    move.l  r12, #KERN_DATA_BASE
    load.b  r14, KD_HWRES_TASK(r12)
    bne     r14, r13, .ktc_hwres_done
    move.l  r14, #0xFF
    store.b r14, KD_HWRES_TASK(r12)
.ktc_hwres_done:
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
    ; M12.6 Phase C: chain-aware lookup. r23 holds the saved waitport id.
    ; The helper clobbers r3..r9; r23 is preserved (high register).
    move.q  r1, r23
    jsr     kern_port_addr_for_id
    move.q  r21, r1                     ; R21 = &port[waitport_id]
    beqz    r21, .restore_waitport_spurious

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
    load.l  r7, KD_MSG_SRC(r27)         ; R7 = sender task ID (M12.5)
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
; Bootstrap Grant Table (M12.5)
; ============================================================================
; Immutable list of (program_index, region_tag, ppn_lo, ppn_hi) rows that
; the boot-load loop translates into live grant entries after each
; load_program returns its assigned task ID. This is the only non-broker
; producer of grants and exists solely to break the chicken-and-egg of
; console.handler needing serial-port MMIO before hardware.resource is
; alive (hardware.resource is started by Startup-Sequence, which Shell
; executes, which depends on dos.library, which depends on console.handler).
;
; M12.5 ships exactly one bootstrap row: console.handler (boot index 0)
; gets a 'CHIP' grant for PPN 0xF0 (the chip register page that holds
; TERM_*, SCAN_*, MOUSE_*, video MMIO). Adding more rows is a code change.
;
; Row layout (16 bytes each, BOOTSTRAP_GRANT_ROW_SZ):
;   +0:  program index (1 byte) + 3 bytes padding
;   +4:  region tag (4 bytes, little-endian uint32)
;   +8:  ppn_lo (2 bytes, inclusive)
;   +10: ppn_hi (2 bytes, inclusive)
;   +12: reserved (4 bytes)
;
; Sentinel row: program index = 0xFF (= GRANT_TASK_FREE).

bootstrap_grant_table:
    ; Row 0: console.handler (boot index 0) — CHIP grant for PPN 0xF0
    dc.b    0, 0, 0, 0                  ; prog_idx=0 + 3 bytes padding
    dc.l    HWRES_TAG_CHIP              ; 'CHIP' (little-endian uint32)
    dc.w    0xF0                        ; ppn_lo
    dc.w    0xF0                        ; ppn_hi
    dc.l    0                           ; reserved
    ; Sentinel
    dc.b    0xFF
    ds.b    15

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

    ; === Map terminal MMIO page (M11.5: console.handler now owns terminal I/O) ===
    ; SYS_MAP_IO(0xF0, 1) maps physical page 0xF0 (TERM_*, SCAN_*, MOUSE_*, video MMIO)
    ; into our user address space. The returned VA is cached at data[144] and is the
    ; base for the inlined readline loop in .con_no_msg below. M11.5 removed the
    ; legacy SYS_READ_INPUT kernel helper; line input is now read from MMIO directly
    ; here in user mode.
    move.l  r1, #0xF0                   ; R1 = base PPN
    move.l  r2, #1                      ; R2 = page count
    syscall #SYS_MAP_IO                 ; R1 = mapped VA, R2 = err
    load.q  r29, (sp)
    bnez    r2, .con_mapio_failed       ; non-zero err is unrecoverable
    store.q r1, 144(r29)                ; data[144] = term_io_va
    bra     .con_after_mapio
.con_mapio_failed:
    ; Cannot recover — exit task. Subsequent FindPort retries by clients will
    ; eventually fail when no console.handler port exists.
    syscall #SYS_EXIT_TASK
.con_after_mapio:

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

    ; M11.5: Inline terminal MMIO read loop (formerly SYS_READ_INPUT in kernel).
    ; Reads from page 0xF0 via the cached VA at data[144]. Layout within the
    ; mapped page (matches the physical TERM_* register addresses):
    ;   +0x70C TERM_LINE_STATUS   (bit 0 = complete line ready)
    ;   +0x704 TERM_STATUS        (bit 0 = char available)
    ;   +0x708 TERM_IN            (read dequeues one char)
    ; Destination is the readline_mapped_va shared buffer at data[168].
    load.q  r28, 144(r29)             ; r28 = term_io_va (page 0xF0 base)
    load.q  r20, 168(r29)             ; r20 = dest buffer VA
    move.l  r21, #126                 ; r21 = max_len

    ; Check TERM_LINE_STATUS — bail with ERR_AGAIN if no complete line yet
    add     r24, r28, #0x70C           ; r24 = &TERM_LINE_STATUS
    load.l  r14, (r24)
    and     r14, r14, #1
    beqz    r14, .con_yield            ; no complete line — yield and retry

    ; Line is ready — drain TERM_IN until \n or buffer full
    add     r23, r28, #0x708           ; r23 = &TERM_IN
    add     r24, r28, #0x704           ; r24 = &TERM_STATUS
    move.l  r22, #0                    ; r22 = byte count

.con_ri_loop:
    bge     r22, r21, .con_ri_done    ; max_len reached

    load.l  r14, (r24)                ; TERM_STATUS
    and     r14, r14, #1
    beqz    r14, .con_ri_done          ; no more chars (shouldn't happen mid-line)

    load.l  r14, (r23)                ; dequeue char from TERM_IN

    move.l  r15, #0x0D                 ; '\r' — skip
    beq     r14, r15, .con_ri_loop

    move.l  r15, #0x0A                 ; '\n' — end of line
    beq     r14, r15, .con_ri_done

    add     r15, r20, r22              ; dest = buffer + count
    store.b r14, (r15)
    add     r22, r22, #1
    bra     .con_ri_loop

.con_ri_done:
    ; Null-terminate
    add     r15, r20, r22
    store.b r0, (r15)

    ; Restore data page pointer (r29) and use byte count from r22
    load.q  r29, (sp)
    move.q  r25, r22                   ; save byte count

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
; Data page layout (M12.6 Phase A):
;   0:   "console.handler\0"       (16 bytes, for OpenLibrary)
;   16:  "dos.library\0"           (16 bytes, port name for CreatePort)
;   32:  "dos.library ONLINE [Task\0" (22 bytes + 10 pad = 32 bytes, banner)
;   64:  padding (64 bytes)
;   128: task_id (8)
;   136: console_port (8)
;   144: dos_port (8)
;   152: meta_chain_head_va (8) — VA of first metadata chain page (M12.6 Phase A)
;   160: hnd_chain_head_va  (8) — VA of first handle chain page   (M12.6 Phase A)
;   168: caller_mapped_va (8, cached MapShared result)
;   176: reserved (8) — was open_handles[8]; gone in M12.6 Phase A
;   184: cached share_pages (8)
;   192..895: dead space (was: file_table 16 × 44). Reused freely later.
;   896: "readme\0" + pad (16 bytes, scratch for pre-create)
;   912: "Welcome to IntuitionOS M9\r\n\0" (29 bytes, pre-create content)
;   944: scratch: saved reply_port (8)
;   952: scratch: saved msg_type (8)
;   960: scratch: saved data0 (8)
;   968: scratch: saved data1 (8)
;   976: scratch: saved share_handle (8)
;   984: scratch: cached share_handle (4)

prog_doslib:
    ; Header
    dc.l    IMG_MAGIC_LO, IMG_MAGIC_HI
    dc.l    prog_doslib_code_end - prog_doslib_code
    dc.l    prog_doslib_data_end - prog_doslib_data
    dc.l    0
    ds.b    12
prog_doslib_code:

    ; =====================================================================
    ; Preamble: compute data page base (preemption-safe double-check).
    ;
    ; M12.8 prerequisite: the previous version added a hard-coded
    ; `#0x3000` offset to USER_CODE_BASE + task_id*stride. That offset
    ; assumed dos.library has exactly 2 code pages — wrong as soon as
    ; the code section grows past 8 KiB. The robust derivation: the
    ; kernel sets initial USP = data_base (see load_program at
    ; iexec.s ~line 747). After the entry-point `sub sp, sp, #16`,
    ; sp == data_base - 16, so `add r29, sp, #16` recovers data_base
    ; for ANY code_pages count. No magic offsets, no SYSINFO query.
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
    add     r29, sp, #16               ; r29 = data_base (= initial USP)
    store.q r29, (sp)
    load.q  r1, 8(sp)
    add     r28, sp, #16               ; recompute for double-check
    load.q  r29, (sp)
    bne     r28, r29, .dos_preamble
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
    syscall #SYS_FIND_PORT             ; R1 = handle (port_id), R2 = err
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
    ; M12.6 Phase A: initialize the metadata + handle chain heads.
    ; Each chain starts with one AllocMem'd 4 KiB page; further pages are
    ; allocated on demand by .dos_meta_alloc_page / .dos_hnd_alloc_page.
    ; CreatePort is still deferred until after seeding — boot race prevention.
    ; =====================================================================
    ; Allocate first metadata chain page
    move.l  r1, #4096
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM             ; R1 = VA, R2 = err
    load.q  r29, (sp)
    store.q r1, 152(r29)               ; data[152] = meta_chain_head_va
    ; Allocate first handle chain page
    move.l  r1, #4096
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM             ; R1 = VA, R2 = err
    load.q  r29, (sp)
    store.q r1, 160(r29)               ; data[160] = hnd_chain_head_va

    ; =====================================================================
    ; Pre-create "readme" file at metadata chain page slot 0.
    ; The first chain page is empty so the "find free slot" walker would
    ; trivially return slot 0; we just inline the same operations here
    ; for the boot path to keep boot init simple and dependency-free.
    ; M12.8 Phase 2: file body is now an extent chain, not a fixed
    ; AllocMem(DOS_FILE_SIZE) page.
    ; =====================================================================
    ; Allocate the readme body via the extent allocator (28 bytes).
    move.l  r1, #28
    jsr     .dos_extent_alloc           ; r1 = first extent VA, r2 = err
    bnez    r2, .dos_init_done          ; alloc failed → leave entry unset
    move.q  r24, r1                     ; r24 = readme_body_first_va

    ; Get the metadata chain head and address slot 0
    load.q  r25, 152(r29)              ; r25 = meta page VA
    add     r25, r25, #DOS_META_HDR_SZ ; r25 = &entries[0]

    ; Copy filename from data[896] = "readme" to entry.name (max 31 + NUL)
    add     r20, r29, #896             ; src = "readme"
    move.q  r21, r25                   ; dst = &entry.name
    move.l  r14, #0
.dos_cpname:
    load.b  r15, (r20)
    store.b r15, (r21)
    beqz    r15, .dos_cpname_done
    add     r20, r20, #1
    add     r21, r21, #1
    add     r14, r14, #1
    move.l  r28, #31
    blt     r14, r28, .dos_cpname
.dos_cpname_done:
    ; entry.file_va = r24 (readme_body_first_va, head of extent chain)
    store.q r24, DOS_META_OFF_VA(r25)
    ; entry.size = 28 (length of welcome message)
    move.l  r14, #28
    store.l r14, DOS_META_OFF_SIZE(r25)

    ; Copy welcome message from data[912] into the extent chain payload.
    move.q  r1, r24                    ; r1 = first extent VA
    add     r2, r29, #912              ; r2 = src (welcome message)
    move.l  r3, #28                    ; r3 = byte_count
    jsr     .dos_extent_write
    load.q  r29, (sp)
.dos_init_done:

    ; =====================================================================
    ; Seed RAM: with command images from embedded data pages (M10)
    ; =====================================================================
    ; Images are embedded contiguously at data offset 4096 (page 1+).
    ; Each has an IE64PROG header; Startup-Sequence is plain text.
    ; R22 = slot index (1..6), R24 = current image ptr (auto-advanced).
    ; The .dos_seed_one subroutine handles one file.
    load.q  r29, (sp)
    add     r24, r29, #4096             ; r24 = first embedded image (data page 1)

    ; Seed names at data offsets: C/Version(942), C/Avail(952), C/Dir(960),
    ;   C/Type(966), C/Echo(973), S/Startup-Sequence(980)
    ; Seed VERSION (slot 1)
    add     r20, r29, #942
    jsr     .dos_seed_one
    ; Seed AVAIL (slot 2)
    load.q  r29, (sp)
    add     r20, r29, #952
    jsr     .dos_seed_one
    ; Seed DIR (slot 3)
    load.q  r29, (sp)
    add     r20, r29, #960
    jsr     .dos_seed_one
    ; Seed TYPE (slot 4)
    load.q  r29, (sp)
    add     r20, r29, #966
    jsr     .dos_seed_one
    ; Seed ECHO (slot 5)
    load.q  r29, (sp)
    add     r20, r29, #973
    jsr     .dos_seed_one
    ; Seed S/Startup-Sequence (slot 6)
    load.q  r29, (sp)
    add     r20, r29, #980
    jsr     .dos_seed_one
    ; Seed DEVS/input.device (slot 7) — M11
    load.q  r29, (sp)
    add     r20, r29, #1032
    jsr     .dos_seed_one
    ; Seed RESOURCES/hardware.resource (slot 8) — M12.5
    ; (must come BEFORE graphics.library to match the embedded image order
    ; in this data section: prog_input_device, prog_hwres, prog_graphics_library)
    load.q  r29, (sp)
    add     r20, r29, #1113
    jsr     .dos_seed_one
    ; Seed LIBS/graphics.library (slot 9) — M11
    load.q  r29, (sp)
    add     r20, r29, #1050
    jsr     .dos_seed_one
    ; Seed C/GfxDemo (slot 10) — M11
    load.q  r29, (sp)
    add     r20, r29, #1072
    jsr     .dos_seed_one
    ; Seed LIBS/intuition.library (slot 11) — M12
    load.q  r29, (sp)
    add     r20, r29, #1082
    jsr     .dos_seed_one
    ; Seed C/About (slot 12) — M12
    load.q  r29, (sp)
    add     r20, r29, #1105
    jsr     .dos_seed_one
    bra     .dos_seed_done

    ; -----------------------------------------------------------------
    ; .dos_seed_one (M12.6 Phase A; M12.8 Phase 2 storage migration):
    ; seed one file from embedded data into the metadata chain. The
    ; file body is now an extent chain, not a fixed AllocMem(DOS_FILE_SIZE)
    ; page. Image size is computed from the IE64PROG header (or NUL scan
    ; for plain text), an extent chain is allocated to hold it, and the
    ; image bytes are copied via .dos_extent_write.
    ; Input:  r20 = name_ptr, r24 = image_ptr, r29 = data_base
    ; Output: r24 advanced past image (aligned to 8)
    ; Saves intermediate state in data[192..223] (former file_table region,
    ; now dead-space scratch reserved exclusively for boot init).
    ; Clobbers: r1-r18, r20, r21, r23, r25, r26
    ; -----------------------------------------------------------------
.dos_seed_one:
    ; NOTE: seed_one is itself called via JSR, so (sp) holds the return PC of
    ; seed_one — NOT a saved-r29 slot. r29 must be kept live in the register.
    ; Helpers preserve r29 internally; inline syscalls use push/pop.
    ; Stash name + image ptrs into data scratch (data[192..223], dead space).
    store.q r20, 192(r29)               ; saved name ptr
    store.q r24, 200(r29)               ; saved image ptr

    ; --- 1. Find/alloc free metadata entry (helper preserves r29) ---
    jsr     .dos_meta_alloc_entry       ; r1 = entry VA, r2 = err
    bnez    r2, .dso_done
    store.q r1, 208(r29)                ; saved entry VA

    ; --- 2. Compute image size from saved image ptr ---
    load.q  r24, 200(r29)
    load.l  r15, (r24)
    move.l  r18, #IMG_MAGIC_LO
    bne     r15, r18, .dso_text_size
    ; IE64PROG: size = 32 + code_size + data_size
    load.l  r23, 8(r24)
    load.l  r15, 12(r24)
    add     r23, r23, r15
    add     r23, r23, #IMG_HEADER_SIZE
    bra     .dso_have_size
.dso_text_size:
    ; Plain text: scan for NUL byte.
    ;
    ; M12.8 follow-up: this path seeds trusted embedded text assets
    ; (currently S:Startup-Sequence), so a hard-coded 4 KiB scan cap
    ; would be a lingering artificial per-file ceiling after the
    ; DOS_FILE_SIZE removal. The embedded data is kernel-controlled, not
    ; user input, so the correct behavior here is to scan to its real
    ; terminating NUL rather than imposing another product limit.
    move.q  r16, r24
    move.l  r23, #0
.dso_tscan:
    load.b  r15, (r16)
    beqz    r15, .dso_have_size
    add     r16, r16, #1
    add     r23, r23, #1
    bra     .dso_tscan
.dso_have_size:
    store.q r23, 216(r29)               ; saved image size

    ; --- 3. Allocate extent chain large enough for the image ---
    move.q  r1, r23                     ; r1 = byte_count
    jsr     .dos_extent_alloc           ; r1 = first extent VA, r2 = err
    bnez    r2, .dso_done
    store.q r1, 224(r29)                ; saved chain head VA

    ; --- 4. Copy name from saved name ptr to entry.name ---
    load.q  r20, 192(r29)
    load.q  r25, 208(r29)
    move.q  r16, r20
    move.q  r17, r25                    ; entry.name = entry+0
    move.l  r18, #0
.dso_cpname:
    load.b  r15, (r16)
    store.b r15, (r17)
    beqz    r15, .dso_cpname_done
    add     r16, r16, #1
    add     r17, r17, #1
    add     r18, r18, #1
    move.l  r28, #31
    blt     r18, r28, .dso_cpname
    store.b r0, (r17)
.dso_cpname_done:

    ; --- 5. entry.file_va = chain head, entry.size = image size ---
    load.q  r25, 208(r29)
    load.q  r1, 224(r29)
    store.q r1, DOS_META_OFF_VA(r25)
    load.q  r23, 216(r29)
    store.l r23, DOS_META_OFF_SIZE(r25)

    ; --- 6. Copy image bytes into the extent chain ---
    load.q  r1, 224(r29)                ; r1 = first extent VA
    load.q  r2, 200(r29)                ; r2 = src = image ptr
    load.q  r3, 216(r29)                ; r3 = byte_count
    jsr     .dos_extent_write

    ; --- 7. Advance r24 past image (aligned to 8) ---
    load.q  r24, 200(r29)
    load.q  r23, 216(r29)
    add     r24, r24, r23
    add     r24, r24, #7
    and     r24, r24, #0xFFFFFFF8
.dso_done:
    rts

.dos_seed_done:

    ; =====================================================================
    ; NOW create the DOS port (after seeding = readiness signal)
    ; =====================================================================
    load.q  r29, (sp)
    add     r1, r29, #16               ; R1 = &data[16] = "dos.library"
    move.l  r2, #PF_PUBLIC
    syscall #SYS_CREATE_PORT           ; R1 = port_id
    load.q  r29, (sp)
    store.q r1, 144(r29)               ; data[144] = dos_port

    ; =====================================================================
    ; Main loop: WaitPort(dos_port) → dispatch on message type
    ; =====================================================================
.dos_main_loop:
    load.q  r29, (sp)
    load.q  r1, 144(r29)               ; R1 = dos_port
    syscall #SYS_WAIT_PORT              ; R1=type R2=data0 R3=err R4=data1 R5=reply R6=share
    load.q  r29, (sp)

    ; Save message fields to data page scratch
    store.q r5, 944(r29)               ; saved reply_port
    store.q r1, 952(r29)               ; saved type (opcode)
    store.q r2, 960(r29)               ; saved data0
    store.q r4, 968(r29)               ; saved data1
    store.q r6, 976(r29)               ; saved share_handle

    ; --- Map caller's shared buffer (re-map if share_handle changed) ---
    load.l  r14, 984(r29)              ; cached share_handle
    load.l  r15, 976(r29)              ; incoming share_handle
    beq     r14, r15, .dos_have_buf    ; same handle → use cached VA
    ; Different handle or first time: do MapShared
    store.l r15, 984(r29)              ; update cached handle
    move.q  r1, r15                    ; R1 = new share_handle
    syscall #SYS_MAP_SHARED            ; R1=VA R2=err R3=share_pages
    move.q  r24, r3                    ; preserve R3 across r29 reload
    load.q  r29, (sp)
    beqz    r1, .dos_reply_err         ; MapShared failed
    ; Reject shares smaller than 1 page. Defense-in-depth: AllocMem rounds
    ; up to ≥1 page (4096B), so this never triggers in normal use. The
    ; DOS_READ/WRITE share-size clamps below also bound the copy by
    ; share_pages*4096 for additional safety.
    move.l  r11, #1                    ; min pages = 1
    blt     r24, r11, .dos_reply_badarg
    store.q r1, 168(r29)               ; update cached VA
    store.q r24, 184(r29)              ; cache share_pages
.dos_have_buf:

.dos_dispatch:
    load.q  r14, 952(r29)              ; r14 = opcode
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
    ; M12.6 Phase A: walks the metadata chain instead of a fixed file table.
    ; =================================================================
.dos_do_dir:
    load.q  r20, 168(r29)              ; r20 = dest (caller's shared buffer)
    move.q  r21, r0                     ; r21 = total bytes written
    ; Compute share_bytes - 56 reserve into r24 (max ~56 bytes per entry:
    ; 32 name pad + up to 10 size digits + 2 CRLF + 12 slack/NUL). Defense-in-depth
    ; against a small share that can't fit the full directory listing.
    load.q  r24, 184(r29)              ; cached share_pages
    lsl     r24, r24, #12              ; r24 = share_bytes
    sub     r24, r24, #56              ; r24 = safe write ceiling
    ; Walk the metadata chain. r25 = current chain page VA.
    load.q  r25, 152(r29)              ; meta chain head
.dos_dir_page_loop:
    beqz    r25, .dos_dir_done
    move.q  r22, r25
    add     r22, r22, #DOS_META_HDR_SZ ; r22 = &entries[0]
    move.l  r23, #0                    ; entry index in this page
.dos_dir_entry:
    ; Stop if buffer is nearly full
    bge     r21, r24, .dos_dir_done
    move.l  r28, #DOS_META_PER_PAGE
    bge     r23, r28, .dos_dir_next_page
    move.q  r14, r22                    ; r14 = entry VA
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
    move.l  r28, #32
    blt     r17, r28, .dos_dir_cpname
.dos_dir_pad:
    ; Pad with spaces to column 32
    move.l  r28, #32
    bge     r17, r28, .dos_dir_size
    move.l  r15, #0x20                  ; ' '
    store.b r15, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    add     r17, r17, #1
    bra     .dos_dir_pad
.dos_dir_size:
    ; Read file size from entry+DOS_META_OFF_SIZE and write it in decimal.
    ; Keep the historical minimum width of 4 digits for small files, but
    ; expand cleanly past 9999 now that M12.8 removed the old per-file cap.
    load.l  r15, DOS_META_OFF_SIZE(r14) ; r15 = remaining size
    move.l  r18, #0                     ; r18 = started flag

    ; 1,000,000,000
    move.l  r28, #1000000000
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    beqz    r18, .dds_skip_1g_chk
    bra     .dds_emit_1g
.dds_skip_1g_chk:
    beqz    r16, .dds_skip_1g
.dds_emit_1g:
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r18, #1
.dds_skip_1g:

    ; 100,000,000
    move.l  r28, #100000000
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    beqz    r18, .dds_skip_100m_chk
    bra     .dds_emit_100m
.dds_skip_100m_chk:
    beqz    r16, .dds_skip_100m
.dds_emit_100m:
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r18, #1
.dds_skip_100m:

    ; 10,000,000
    move.l  r28, #10000000
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    beqz    r18, .dds_skip_10m_chk
    bra     .dds_emit_10m
.dds_skip_10m_chk:
    beqz    r16, .dds_skip_10m
.dds_emit_10m:
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r18, #1
.dds_skip_10m:

    ; 1,000,000
    move.l  r28, #1000000
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    beqz    r18, .dds_skip_1m_chk
    bra     .dds_emit_1m
.dds_skip_1m_chk:
    beqz    r16, .dds_skip_1m
.dds_emit_1m:
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r18, #1
.dds_skip_1m:

    ; 100,000
    move.l  r28, #100000
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    beqz    r18, .dds_skip_100k_chk
    bra     .dds_emit_100k
.dds_skip_100k_chk:
    beqz    r16, .dds_skip_100k
.dds_emit_100k:
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r18, #1
.dds_skip_100k:

    ; 10,000
    move.l  r28, #10000
    divu    r16, r15, r28
    mulu    r17, r16, r28
    sub     r15, r15, r17
    beqz    r18, .dds_skip_10k_chk
    bra     .dds_emit_10k
.dds_skip_10k_chk:
    beqz    r16, .dds_skip_10k
.dds_emit_10k:
    add     r16, r16, #0x30
    store.b r16, (r20)
    add     r20, r20, #1
    add     r21, r21, #1
    move.l  r18, #1
.dds_skip_10k:

    ; 1,000 (always emit from here down for 4-digit minimum width)
    move.l  r28, #1000
    divu    r16, r15, r28
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
    add     r22, r22, #DOS_META_ENTRY_SZ
    add     r23, r23, #1
    bra     .dos_dir_entry
.dos_dir_next_page:
    load.q  r25, (r25)
    bra     .dos_dir_page_loop
.dos_dir_done:
    ; Null-terminate
    store.b r0, (r20)
    ; Reply with data0 = bytes written
    load.q  r1, 944(r29)               ; reply_port
    move.l  r2, #DOS_OK                 ; type = success
    move.q  r3, r21                     ; data0 = bytes written
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

    ; =================================================================
    ; DOS_OPEN (type=1): open file by name from shared buffer
    ; M12.6 Phase A: walks the metadata chain via .dos_meta_find_by_name;
    ; allocates a new entry via .dos_meta_alloc_entry on write-mode miss;
    ; allocates a handle slot via .dos_hnd_alloc.
    ; =================================================================
    ; data0 = mode (READ=0, WRITE=1), filename in caller's shared buffer
.dos_do_open:
    load.q  r20, 960(r29)              ; r20 = mode (0=READ, 1=WRITE)
    load.q  r23, 168(r29)              ; r23 = mapped VA (filename pointer)

    ; Resolve filename (strip RAM:, C:, S: prefixes)
    jsr     .dos_resolve_file

    ; --- Search metadata chain for matching name ---
    move.q  r1, r23                     ; r1 = request name ptr
    jsr     .dos_meta_find_by_name      ; r1 = entry VA (or 0)
    bnez    r1, .dos_open_have_entry
    ; Not found
    bnez    r20, .dos_open_create       ; WRITE → create new
    bra     .dos_reply_err              ; READ → error

.dos_open_create:
    ; M12.8 Phase 2: write-mode create no longer pre-allocates a body.
    ; entry.file_va starts as 0 (empty file). The first DOS_WRITE will
    ; allocate an extent chain via .dos_extent_alloc and link it in.
    ; Allocate a fresh metadata entry.
    jsr     .dos_meta_alloc_entry       ; r1 = entry VA, r2 = err
    bnez    r2, .dos_reply_full
    move.q  r25, r1                     ; r25 = entry VA
    ; entry.file_va = 0 (empty body)
    store.q r0, DOS_META_OFF_VA(r25)
    ; Copy filename from request buffer to entry.name (max 31 + NUL)
    move.q  r16, r23                    ; src = request name
    move.q  r17, r25                    ; dst = entry.name
    move.l  r18, #0
.dos_cpy_fname:
    load.b  r15, (r16)
    store.b r15, (r17)
    beqz    r15, .dos_cpy_fname_done
    add     r16, r16, #1
    add     r17, r17, #1
    add     r18, r18, #1
    move.l  r28, #31
    blt     r18, r28, .dos_cpy_fname
    store.b r0, (r17)
.dos_cpy_fname_done:
    ; entry.size = 0
    store.l r0, DOS_META_OFF_SIZE(r25)
    move.q  r1, r25                     ; r1 = entry VA for handle alloc
    bra     .dos_open_have_entry

.dos_open_have_entry:
    ; r1 = entry VA. Allocate a handle slot referencing this entry.
    jsr     .dos_hnd_alloc              ; r1 = handle_id, r2 = err
    bnez    r2, .dos_reply_full
    move.q  r18, r1                     ; r18 = handle_id

    ; Reply: type=DOS_OK, data0=handle_id
    load.q  r29, (sp)
    load.q  r1, 944(r29)
    move.l  r2, #DOS_OK
    move.q  r3, r18                     ; data0 = handle_id
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

    ; =================================================================
    ; DOS_READ (type=2): read file data into caller's shared buffer.
    ; M12.8 Phase 2: file body is now an extent chain. The body VA at
    ; DOS_META_OFF_VA is the first_extent_va of the chain (or 0 for an
    ; empty file). The clamped max_bytes is read via .dos_extent_walk.
    ; =================================================================
    ; data0 = handle, data1 = max_bytes
.dos_do_read:
    load.q  r1, 960(r29)               ; r1 = handle_id
    load.q  r19, 968(r29)              ; r19 = max_bytes
    jsr     .dos_hnd_lookup             ; r1 = entry VA (or 0)
    beqz    r1, .dos_read_badh
    move.q  r14, r1                     ; r14 = entry VA
    load.q  r29, (sp)
    load.q  r20, DOS_META_OFF_VA(r14)  ; r20 = first extent VA (or 0)
    load.l  r16, DOS_META_OFF_SIZE(r14)
    ; Clamp max_bytes to file size
    blt     r19, r16, .dos_read_clamp
    move.q  r19, r16
.dos_read_clamp:
    ; Clamp max_bytes to share size in bytes (share_pages << 12).
    load.q  r24, 184(r29)              ; cached share_pages
    lsl     r24, r24, #12              ; r24 = share_bytes
    blt     r19, r24, .dos_read_share_ok
    move.q  r19, r24
.dos_read_share_ok:
    ; Empty body shortcut: file_va == 0 → read 0 bytes (skip the walk).
    move.q  r17, r0                     ; bytes copied = 0
    beqz    r20, .dos_read_reply
    beqz    r19, .dos_read_reply
    ; Walk the extent chain into the caller's shared buffer.
    move.q  r1, r20                     ; r1 = first extent VA
    load.q  r2, 168(r29)                ; r2 = dst = caller's mapped buffer
    move.q  r3, r19                     ; r3 = byte_count
    jsr     .dos_extent_walk            ; r1 = bytes copied
    move.q  r17, r1
    load.q  r29, (sp)
.dos_read_reply:
    load.q  r1, 944(r29)
    move.l  r2, #DOS_OK
    move.q  r3, r17                     ; data0 = bytes read
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop
.dos_read_badh:
    load.q  r29, (sp)
    bra     .dos_reply_badh

    ; =================================================================
    ; DOS_WRITE (type=3): write data from caller's buffer to file.
    ; M12.8 Phase 2: file body is now an extent chain. The per-file
    ; DOS_FILE_SIZE cap has been removed; the only cap is share size.
    ;
    ; Atomic-swap-on-rewrite rule: allocate a NEW extent chain for the
    ; new content; on alloc failure, leave the OLD chain intact and
    ; reply DOS_ERR_FULL. Only after the new chain is fully allocated
    ; AND the new bytes are copied in do we (a) point entry.file_va at
    ; the new chain and (b) free the old chain. A failed write therefore
    ; never corrupts the previous file content.
    ;
    ; Handler scratch slots in dos.library data page:
    ;   256: entry_va        (8 bytes — survives helper JSRs)
    ;   264: old_first_va    (8 bytes — for atomic swap)
    ;   272: clamped_count   (8 bytes — clamped byte_count)
    ; =================================================================
    ; data0 = handle, data1 = byte_count
.dos_do_write:
    load.q  r1, 960(r29)               ; r1 = handle_id
    load.q  r19, 968(r29)              ; r19 = byte_count (raw)
    jsr     .dos_hnd_lookup             ; r1 = entry VA (or 0)
    beqz    r1, .dos_write_badh
    load.q  r29, (sp)
    store.q r1, 256(r29)                ; saved entry VA
    move.q  r14, r1                     ; r14 = entry VA
    load.q  r4, DOS_META_OFF_VA(r14)
    store.q r4, 264(r29)                ; saved old_first_va

    ; Clamp byte_count to share size in bytes (share_pages << 12).
    ; The previous DOS_FILE_SIZE cap is removed; the only remaining
    ; bound is the size of the caller's mapped share.
    load.q  r24, 184(r29)              ; cached share_pages
    lsl     r24, r24, #12              ; r24 = share_bytes
    blt     r19, r24, .dos_write_share_ok
    move.q  r19, r24
.dos_write_share_ok:
    store.q r19, 272(r29)               ; saved clamped byte_count

    ; ---- Allocate the new extent chain (size = clamped byte_count) ----
    ; .dos_extent_alloc returns r1=0 if byte_count==0 (legitimate empty
    ; write) and r2=ERR_OK in that case — handled below at .dwr_no_alloc.
    move.q  r1, r19
    jsr     .dos_extent_alloc           ; r1 = new_first_va, r2 = err
    bnez    r2, .dos_write_full
    load.q  r29, (sp)
    move.q  r25, r1                     ; r25 = new_first_va (may be 0)

    ; ---- Copy bytes from caller's share into the new chain ----
    beqz    r25, .dwr_no_alloc          ; empty write → skip extent_write
    move.q  r1, r25                     ; r1 = new_first_va
    load.q  r2, 168(r29)                ; r2 = src = caller's mapped buffer
    load.q  r3, 272(r29)                ; r3 = clamped byte_count
    jsr     .dos_extent_write
    load.q  r29, (sp)
.dwr_no_alloc:
    ; ---- Atomic swap: link new chain into entry, then free old ----
    load.q  r14, 256(r29)               ; reload entry VA
    store.q r25, DOS_META_OFF_VA(r14)   ; entry.file_va = new_first_va
    load.q  r19, 272(r29)               ; clamped byte_count
    store.l r19, DOS_META_OFF_SIZE(r14) ; entry.size = byte_count

    ; Free the old chain (no-op if old_first_va == 0)
    load.q  r1, 264(r29)
    beqz    r1, .dwr_done
    jsr     .dos_extent_free
    load.q  r29, (sp)
.dwr_done:
    ; Reply DOS_OK with bytes_written = clamped byte_count
    load.q  r19, 272(r29)
    load.q  r1, 944(r29)
    move.l  r2, #DOS_OK
    move.q  r3, r19                     ; data0 = bytes written
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

.dos_write_full:
    ; Allocation failed; .dos_extent_alloc has already freed any
    ; partially-allocated chain via its internal cleanup. The entry
    ; is untouched, so the previous file content is intact.
    load.q  r29, (sp)
    bra     .dos_reply_full
.dos_write_badh:
    load.q  r29, (sp)
    bra     .dos_reply_badh

    ; =================================================================
    ; DOS_CLOSE (type=4): close a file handle.
    ; M12.6 Phase A: walks the handle chain via .dos_hnd_lookup; clears
    ; the slot in place. The file body and metadata entry persist.
    ; =================================================================
    ; data0 = handle_id
.dos_do_close:
    load.q  r1, 960(r29)               ; r1 = handle_id
    jsr     .dos_hnd_lookup             ; r1 = entry VA (or 0), r2 = slot VA
    beqz    r1, .dos_close_badh
    ; Clear the slot
    store.q r0, (r2)
    load.q  r29, (sp)
    ; Reply success
    load.q  r1, 944(r29)
    move.l  r2, #DOS_OK
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop
.dos_close_badh:
    load.q  r29, (sp)
    bra     .dos_reply_badh

    ; =================================================================
    ; Name resolution subroutines
    ; =================================================================

    ; .dos_resolve_cmd: resolve command name (default prefix C:)
    ; Input: r23 = name pointer in shared buffer, r29 = data base
    ; Output: r23 = resolved name pointer (may be unchanged)
    ; Clobbers: r14, r15, r16, r17, r18
.dos_resolve_cmd:
    move.l  r18, #1                     ; default = C: volume
    bra     .dos_resolve_scan
    ; .dos_resolve_file: resolve filename (bare default)
    ; Input: r23 = name pointer in shared buffer, r29 = data base
    ; Output: r23 = resolved name pointer (may be unchanged)
.dos_resolve_file:
    move.l  r18, #0                     ; default = bare (no prefix)
.dos_resolve_scan:
    ; Scan for ':' in name
    move.q  r14, r23
    move.l  r15, #0                     ; char index
.dos_resolve_colon:
    load.b  r16, (r14)
    beqz    r16, .dos_resolve_no_colon  ; end of string, no colon found
    move.l  r17, #0x3A                  ; ':'
    beq     r16, r17, .dos_resolve_has_colon
    add     r14, r14, #1
    add     r15, r15, #1
    move.l  r17, #32
    blt     r15, r17, .dos_resolve_colon
.dos_resolve_no_colon:
    ; No colon found — check default mode
    beqz    r18, .dos_resolve_bare_ret  ; mode=0 → bare name, return unchanged
    ; mode=1 → prepend "C/" to name, write to scratch at data[1000].
    ; Reuses the bounded shared copy loop instead of an inline unbounded
    ; loop (the original was vulnerable to a long unprefixed name).
    add     r17, r29, #1000
    move.l  r16, #0x43                  ; 'C'
    store.b r16, (r17)
    add     r17, r17, #1
    move.l  r16, #0x2F                  ; '/'
    store.b r16, (r17)
    add     r17, r17, #1
    move.q  r14, r23                    ; src = original name
    move.l  r19, #29                    ; cap: 32 - 2 (prefix) - 1 (NUL)
    bra     .dos_resolve_copy_rest
.dos_resolve_bare_ret:
    rts
.dos_resolve_has_colon:
    ; Found colon at position r15. Check volume prefix.
    ; Check for "RAM:" (4 chars before colon = 3 chars + colon)
    move.l  r17, #3
    bne     r15, r17, .dos_resolve_check_c
    ; Check 'R'/'A'/'M' (case insensitive)
    load.b  r16, (r23)
    or      r16, r16, #0x20            ; lowercase
    move.l  r17, #0x72                  ; 'r'
    bne     r16, r17, .dos_resolve_check_c
    add     r14, r23, #1
    load.b  r16, (r14)
    or      r16, r16, #0x20
    move.l  r17, #0x61                  ; 'a'
    bne     r16, r17, .dos_resolve_check_c
    add     r14, r23, #2
    load.b  r16, (r14)
    or      r16, r16, #0x20
    move.l  r17, #0x6D                  ; 'm'
    bne     r16, r17, .dos_resolve_check_c
    ; RAM: prefix — strip it, return pointer past ':'
    add     r23, r23, #4               ; skip "RAM:"
    rts
.dos_resolve_check_c:
    ; Check for "C:" (1 char before colon)
    move.l  r17, #1
    bne     r15, r17, .dos_resolve_check_4ch     ; M11: chain to LIBS/DEVS
    load.b  r16, (r23)
    or      r16, r16, #0x20
    move.l  r17, #0x63                  ; 'c'
    beq     r16, r17, .dos_resolve_pfx_c
    move.l  r17, #0x73                  ; 's'
    beq     r16, r17, .dos_resolve_pfx_s
    bra     .dos_resolve_done           ; unknown 1-char volume

.dos_resolve_pfx_c:
    ; Write "C/" + (name after ':') to scratch buffer at data[1000]
    add     r17, r29, #1000
    move.l  r16, #0x43                  ; 'C'
    store.b r16, (r17)
    add     r17, r17, #1
    move.l  r16, #0x2F                  ; '/'
    store.b r16, (r17)
    add     r17, r17, #1
    add     r14, r23, #2                ; src = past "C:"
    move.l  r19, #29                    ; cap: 32 - 2 (prefix) - 1 (NUL)
    bra     .dos_resolve_copy_rest

.dos_resolve_pfx_s:
    ; Write "S/" + (name after ':') to scratch buffer
    add     r17, r29, #1000
    move.l  r16, #0x53                  ; 'S'
    store.b r16, (r17)
    add     r17, r17, #1
    move.l  r16, #0x2F                  ; '/'
    store.b r16, (r17)
    add     r17, r17, #1
    add     r14, r23, #2                ; src = past "S:"
    move.l  r19, #29                    ; cap: 32 - 2 (prefix) - 1 (NUL)
    ; Shared bounded copy loop. r14 = src, r17 = dst (in scratch),
    ; r19 = max remaining payload bytes (set by each prefix branch
    ; to (31 - prefix_len) so the buffer always has room for the
    ; trailing NUL terminator). On src NUL OR exhausted r19, write
    ; a NUL at r17 and return. The 32-byte scratch at data[1000]
    ; is the M10 resolver scratch; without this cap a long
    ; user-supplied name (e.g. "C:" + 200 'A' chars) would walk
    ; past the scratch into the M11 seed-name strings and beyond.
.dos_resolve_copy_rest:
    load.b  r16, (r14)
    beqz    r16, .dos_resolve_copy_term  ; src NUL → terminate
    beqz    r19, .dos_resolve_copy_term  ; out of room → truncate + terminate
    store.b r16, (r17)
    add     r14, r14, #1
    add     r17, r17, #1
    sub     r19, r19, #1
    bra     .dos_resolve_copy_rest
.dos_resolve_copy_term:
    store.b r0, (r17)                    ; write NUL terminator (r0 = hardwired 0)
.dos_resolve_copy_done:
    add     r23, r29, #1000              ; return scratch as resolved name
    rts

    ; ----------------------------------------------------------------
    ; M11: 4-char prefix check — handles LIBS: and DEVS:
    ; ----------------------------------------------------------------
.dos_resolve_check_4ch:
    move.l  r17, #4
    bne     r15, r17, .dos_resolve_check_9ch
    load.b  r16, (r23)
    or      r16, r16, #0x20
    move.l  r17, #0x6C                  ; 'l'
    beq     r16, r17, .dos_check_libs
    move.l  r17, #0x64                  ; 'd'
    beq     r16, r17, .dos_check_devs
    bra     .dos_resolve_check_9ch

.dos_check_libs:
    ; Verify "ibs"
    add     r14, r23, #1
    load.b  r16, (r14)
    or      r16, r16, #0x20
    move.l  r17, #0x69                  ; 'i'
    bne     r16, r17, .dos_resolve_check_9ch
    add     r14, r23, #2
    load.b  r16, (r14)
    or      r16, r16, #0x20
    move.l  r17, #0x62                  ; 'b'
    bne     r16, r17, .dos_resolve_check_9ch
    add     r14, r23, #3
    load.b  r16, (r14)
    or      r16, r16, #0x20
    move.l  r17, #0x73                  ; 's'
    bne     r16, r17, .dos_resolve_check_9ch
    ; Match: emit "LIBS/" + remainder
    add     r17, r29, #1000
    move.l  r16, #0x4C                  ; 'L'
    store.b r16, (r17)
    add     r17, r17, #1
    move.l  r16, #0x49                  ; 'I'
    store.b r16, (r17)
    add     r17, r17, #1
    move.l  r16, #0x42                  ; 'B'
    store.b r16, (r17)
    add     r17, r17, #1
    move.l  r16, #0x53                  ; 'S'
    store.b r16, (r17)
    add     r17, r17, #1
    move.l  r16, #0x2F                  ; '/'
    store.b r16, (r17)
    add     r17, r17, #1
    add     r14, r23, #5                ; src = past "LIBS:"
    move.l  r19, #26                    ; cap: 32 - 5 (prefix) - 1 (NUL)
    bra     .dos_resolve_copy_rest

.dos_check_devs:
    ; Verify "evs"
    add     r14, r23, #1
    load.b  r16, (r14)
    or      r16, r16, #0x20
    move.l  r17, #0x65                  ; 'e'
    bne     r16, r17, .dos_resolve_check_9ch
    add     r14, r23, #2
    load.b  r16, (r14)
    or      r16, r16, #0x20
    move.l  r17, #0x76                  ; 'v'
    bne     r16, r17, .dos_resolve_check_9ch
    add     r14, r23, #3
    load.b  r16, (r14)
    or      r16, r16, #0x20
    move.l  r17, #0x73                  ; 's'
    bne     r16, r17, .dos_resolve_check_9ch
    ; Match: emit "DEVS/" + remainder
    add     r17, r29, #1000
    move.l  r16, #0x44                  ; 'D'
    store.b r16, (r17)
    add     r17, r17, #1
    move.l  r16, #0x45                  ; 'E'
    store.b r16, (r17)
    add     r17, r17, #1
    move.l  r16, #0x56                  ; 'V'
    store.b r16, (r17)
    add     r17, r17, #1
    move.l  r16, #0x53                  ; 'S'
    store.b r16, (r17)
    add     r17, r17, #1
    move.l  r16, #0x2F                  ; '/'
    store.b r16, (r17)
    add     r17, r17, #1
    add     r14, r23, #5                ; src = past "DEVS:"
    move.l  r19, #26                    ; cap: 32 - 5 (prefix) - 1 (NUL)
    bra     .dos_resolve_copy_rest

    ; ----------------------------------------------------------------
    ; M11: 9-char prefix check — handles RESOURCES:
    ; ----------------------------------------------------------------
.dos_resolve_check_9ch:
    move.l  r17, #9
    bne     r15, r17, .dos_resolve_done
    ; Verify "esources" at offsets 1..8 (offset 0 = 'r' check first)
    load.b  r16, (r23)
    or      r16, r16, #0x20
    move.l  r17, #0x72                  ; 'r'
    bne     r16, r17, .dos_resolve_done
    add     r14, r23, #1
    load.b  r16, (r14)
    or      r16, r16, #0x20
    move.l  r17, #0x65                  ; 'e'
    bne     r16, r17, .dos_resolve_done
    add     r14, r23, #2
    load.b  r16, (r14)
    or      r16, r16, #0x20
    move.l  r17, #0x73                  ; 's'
    bne     r16, r17, .dos_resolve_done
    add     r14, r23, #3
    load.b  r16, (r14)
    or      r16, r16, #0x20
    move.l  r17, #0x6F                  ; 'o'
    bne     r16, r17, .dos_resolve_done
    add     r14, r23, #4
    load.b  r16, (r14)
    or      r16, r16, #0x20
    move.l  r17, #0x75                  ; 'u'
    bne     r16, r17, .dos_resolve_done
    add     r14, r23, #5
    load.b  r16, (r14)
    or      r16, r16, #0x20
    move.l  r17, #0x72                  ; 'r'
    bne     r16, r17, .dos_resolve_done
    add     r14, r23, #6
    load.b  r16, (r14)
    or      r16, r16, #0x20
    move.l  r17, #0x63                  ; 'c'
    bne     r16, r17, .dos_resolve_done
    add     r14, r23, #7
    load.b  r16, (r14)
    or      r16, r16, #0x20
    move.l  r17, #0x65                  ; 'e'
    bne     r16, r17, .dos_resolve_done
    add     r14, r23, #8
    load.b  r16, (r14)
    or      r16, r16, #0x20
    move.l  r17, #0x73                  ; 's'
    bne     r16, r17, .dos_resolve_done
    ; Match: emit "RESOURCES/" + remainder
    add     r17, r29, #1000
    move.l  r16, #0x52                  ; 'R'
    store.b r16, (r17)
    add     r17, r17, #1
    move.l  r16, #0x45                  ; 'E'
    store.b r16, (r17)
    add     r17, r17, #1
    move.l  r16, #0x53                  ; 'S'
    store.b r16, (r17)
    add     r17, r17, #1
    move.l  r16, #0x4F                  ; 'O'
    store.b r16, (r17)
    add     r17, r17, #1
    move.l  r16, #0x55                  ; 'U'
    store.b r16, (r17)
    add     r17, r17, #1
    move.l  r16, #0x52                  ; 'R'
    store.b r16, (r17)
    add     r17, r17, #1
    move.l  r16, #0x43                  ; 'C'
    store.b r16, (r17)
    add     r17, r17, #1
    move.l  r16, #0x45                  ; 'E'
    store.b r16, (r17)
    add     r17, r17, #1
    move.l  r16, #0x53                  ; 'S'
    store.b r16, (r17)
    add     r17, r17, #1
    move.l  r16, #0x2F                  ; '/'
    store.b r16, (r17)
    add     r17, r17, #1
    add     r14, r23, #10               ; src = past "RESOURCES:"
    move.l  r19, #21                    ; cap: 32 - 10 (prefix) - 1 (NUL)
    bra     .dos_resolve_copy_rest

.dos_resolve_done:
    ; Unknown volume — return unchanged
    rts

    ; =================================================================
    ; DOS_RUN (type=6): launch program by name (M10)
    ; =================================================================
    ; Shared buffer format: "command_name\0args_string\0"
    ; Resolves command name through C: assign, finds image in file table,
    ; launches via SYS_EXEC_PROGRAM (new ABI: image_ptr, image_size).
.dos_do_run:
    ; 1. Read command name from mapped shared buffer
    load.q  r23, 168(r29)              ; r23 = caller's mapped buffer (name ptr)

    ; 2. Resolve through C: assign (r23 in/out)
    jsr     .dos_resolve_cmd            ; r23 = resolved name (e.g. "C/Version")
    load.q  r29, (sp)

    ; 3. M12.6 Phase A: walk metadata chain by name
    move.q  r1, r23
    jsr     .dos_meta_find_by_name      ; r1 = entry VA (or 0)
    beqz    r1, .dos_run_notfound
    move.q  r14, r1                     ; r14 = entry VA
    load.q  r29, (sp)
    bra     .dos_run_found

.dos_run_notfound:
    load.q  r29, (sp)
    ; Reply with DOS_ERR_NOTFOUND
    load.q  r1, 944(r29)
    move.l  r2, #DOS_ERR_NOTFOUND
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

.dos_run_found:
    ; r14 = entry VA. M12.8 Phase 2: file body is an extent chain.
    ; SYS_EXEC_PROGRAM needs a contiguous image_ptr, so we:
    ;   1. AllocMem a temp contiguous buffer (image_size bytes)
    ;   2. Walk the extent chain into the temp buffer
    ;   3. Pass the temp buffer to SYS_EXEC_PROGRAM
    ;   4. FreeMem the temp after SYS_EXEC_PROGRAM returns (the kernel
    ;      has already copied the image into the new task's slot at
    ;      that point — see load_program in iexec.s ~line 700).
    ;
    ; Handler scratch slots in dos.library data page (DOS_RUN-specific,
    ; non-overlapping with DOS_WRITE's 256..279):
    ;   280: first_extent_va (8 bytes — for the walker call)
    ;   288: image_size      (8 bytes — for walker count + later FreeMem)
    ;   296: temp_buf_va     (8 bytes — for FreeMem after exec)
    load.q  r21, DOS_META_OFF_VA(r14)   ; r21 = first extent VA (or 0)
    load.l  r23, DOS_META_OFF_SIZE(r14) ; r23 = image size

    ; A zero-size or empty-body file cannot be a valid program.
    beqz    r21, .dos_run_notfound
    beqz    r23, .dos_run_notfound

    ; Save state to scratch slots BEFORE any syscall (syscalls clobber regs)
    store.q r21, 280(r29)               ; first_extent_va
    store.q r23, 288(r29)               ; image_size

    ; ---- AllocMem a temp contiguous buffer of size = image_size ----
    push    r29
    move.q  r1, r23                     ; r1 = image size
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM              ; r1 = temp buf VA, r2 = err
    pop     r29
    bnez    r2, .dos_run_notfound       ; AllocMem failure → treat as notfound
    store.q r1, 296(r29)                ; saved temp buf VA

    ; ---- Walk the extent chain into the temp buffer ----
    load.q  r1, 280(r29)                ; r1 = first_extent_va
    load.q  r2, 296(r29)                ; r2 = dst = temp buf VA
    load.q  r3, 288(r29)                ; r3 = byte_count = image size
    jsr     .dos_extent_walk
    load.q  r29, (sp)
    load.q  r21, 296(r29)               ; r21 = temp buf VA (image_ptr for exec)
    load.q  r23, 288(r29)               ; r23 = image size

    ; 5. Find args: scan past command name null in shared buffer.
    ; Bounded scan — without a length cap, a malicious caller could
    ; send a shared buffer with no terminator and walk dos.library
    ; off the mapped page (faulting the service). DATA_ARGS_MAX (256)
    ; is the same upper bound used by the args-length scan below; a
    ; command name longer than that is treated as malformed input
    ; and routed to the DOS_ERR_NOTFOUND reply.
    load.q  r20, 168(r29)              ; original shared buffer
    move.q  r16, r20
    move.l  r24, #0                    ; r24 = scan counter
.dos_run_skip_cmd:
    load.b  r15, (r16)
    beqz    r15, .dos_run_args_start
    add     r16, r16, #1
    add     r24, r24, #1
    move.l  r28, #DATA_ARGS_MAX
    blt     r24, r28, .dos_run_skip_cmd
    ; Hit cap without finding a NUL — malformed request, bail out.
    bra     .dos_run_notfound
.dos_run_args_start:
    add     r16, r16, #1               ; skip the null → args start
    ; Compute args_len (scan for second null)
    move.q  r17, r16                   ; args_ptr
    move.l  r18, #0                    ; args_len
.dos_run_arglen:
    load.b  r15, (r17)
    beqz    r15, .dos_run_launch
    add     r17, r17, #1
    add     r18, r18, #1
    move.l  r28, #DATA_ARGS_MAX
    blt     r18, r28, .dos_run_arglen

.dos_run_launch:
    ; 6. SYS_EXEC_PROGRAM (new ABI): R1=image_ptr, R2=size, R3=args_ptr, R4=args_len
    move.q  r1, r21                    ; image_ptr (temp buf populated by extent walker)
    move.q  r2, r23                    ; image_size
    move.q  r3, r16                    ; args_ptr (in shared buffer)
    move.q  r4, r18                    ; args_len
    syscall #SYS_EXEC_PROGRAM
    load.q  r29, (sp)
    ; Save task_id + err to scratch BEFORE FreeMem (FreeMem clobbers regs).
    store.q r1, 304(r29)               ; saved task_id
    store.q r2, 312(r29)               ; saved err

    ; M12.8 Phase 2: free the temp contiguous buffer. The kernel has
    ; already copied the image into the new task's slot during
    ; SYS_EXEC_PROGRAM, so the temp buffer is no longer needed.
    push    r29
    load.q  r1, 296(r29)               ; r1 = temp buf VA
    load.q  r2, 288(r29)               ; r2 = image size (matches AllocMem)
    syscall #SYS_FREE_MEM
    pop     r29

    ; Reply: type=err, data0=task_id
    load.q  r1, 944(r29)
    load.q  r2, 312(r29)               ; type = err
    load.q  r3, 304(r29)               ; data0 = task_id
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

    ; =================================================================
    ; Shared reply blocks (saves code space by consolidating duplicates)
    ; =================================================================
.dos_reply_badh:
    load.q  r1, 944(r29)
    move.l  r2, #DOS_ERR_BADHANDLE
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop
.dos_reply_full:
    load.q  r1, 944(r29)
    move.l  r2, #DOS_ERR_FULL
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop
.dos_reply_badarg:
    load.q  r1, 944(r29)
    move.l  r2, #DOS_ERR_BADARG
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop
.dos_reply_err:
    load.q  r1, 944(r29)
    move.l  r2, #DOS_ERR_NOTFOUND
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .dos_main_loop

    ; ==================================================================
    ; M12.6 Phase A: dos.library chain-allocator helpers
    ; ==================================================================
    ; The file metadata table and the open-handle table are no longer
    ; fixed-size inline arrays. Each is a singly-linked list of 4 KiB
    ; AllocMem'd "chain pages":
    ;
    ;   chain page (4 KiB):
    ;     [0..7]    next_va (8 bytes, 0 = end of chain)
    ;     [8..15]   reserved
    ;     [16..]    entries (per-table stride)
    ;
    ; Metadata entries are 48 bytes (DOS_META_PER_PAGE = 85 per page).
    ; Handle entries are 8 bytes (DOS_HND_PER_PAGE = 510 per page).
    ;
    ; Both helpers preserve r29 (data base) implicitly via the stack
    ; convention used by every other dos.library subroutine.

    ; ------------------------------------------------------------------
    ; .dos_meta_alloc_entry: walk the metadata chain looking for an
    ; entry whose name[0] == 0 (free). If none, allocate a new chain
    ; page and use entry 0 of the new page.
    ; In:  r29 = dos.library data base
    ; Out: r1  = entry VA (always non-zero on success)
    ;      r2  = ERR_OK or err code
    ; Clobbers: r3..r19, r25, r26
    ; ------------------------------------------------------------------
.dos_meta_alloc_entry:
    load.q  r25, 152(r29)              ; r25 = current chain page VA
.dmae_walk_pages:
    beqz    r25, .dmae_alloc_new       ; chain head not allocated yet
    move.q  r26, r25
    add     r26, r26, #DOS_META_HDR_SZ ; r26 = &entries[0]
    move.l  r3, #0                     ; entry index
.dmae_scan_rows:
    move.l  r4, #DOS_META_PER_PAGE
    bge     r3, r4, .dmae_next_page
    load.b  r5, DOS_META_OFF_NAME(r26) ; first byte of name
    beqz    r5, .dmae_found            ; free entry
    add     r26, r26, #DOS_META_ENTRY_SZ
    add     r3, r3, #1
    bra     .dmae_scan_rows
.dmae_next_page:
    load.q  r25, (r25)                 ; next page VA
    bra     .dmae_walk_pages
.dmae_found:
    move.q  r1, r26
    move.q  r2, r0                     ; ERR_OK = 0
    rts
.dmae_alloc_new:
    ; Save r29 across the syscall (AllocMem clobbers user regs).
    push    r29
    move.l  r1, #4096
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM             ; r1 = VA, r2 = err
    pop     r29
    bnez    r2, .dmae_fail
    move.q  r25, r1                    ; r25 = new page VA
    ; Walk to the tail of the existing chain and link the new page.
    load.q  r3, 152(r29)               ; head
    beqz    r3, .dmae_set_head
.dmae_link_walk:
    load.q  r4, (r3)
    beqz    r4, .dmae_at_tail
    move.q  r3, r4
    bra     .dmae_link_walk
.dmae_at_tail:
    store.q r25, (r3)                  ; tail.next_va = new page
    bra     .dmae_link_done
.dmae_set_head:
    store.q r25, 152(r29)              ; head = new page
.dmae_link_done:
    ; Use entry 0 of the new page (already zero from MEMF_CLEAR).
    add     r1, r25, #DOS_META_HDR_SZ
    move.q  r2, r0                     ; ERR_OK
    rts
.dmae_fail:
    move.q  r1, r0
    ; r2 already has the err code from AllocMem
    rts

    ; ------------------------------------------------------------------
    ; .dos_meta_find_by_name: walk the metadata chain comparing each
    ; entry's name to the request name (case-insensitive, max 32 chars).
    ; Returns the entry VA on match, or 0 if not found.
    ; In:  r1  = request name VA (NUL-terminated, in dos.library AS)
    ;      r29 = data base
    ; Out: r1  = entry VA (or 0 if not found)
    ; Clobbers: r3..r12, r25, r26, r27
    ; ------------------------------------------------------------------
.dos_meta_find_by_name:
    move.q  r27, r1                    ; r27 = request name (preserved)
    load.q  r25, 152(r29)              ; chain head
.dmfn_walk_pages:
    beqz    r25, .dmfn_not_found
    move.q  r26, r25
    add     r26, r26, #DOS_META_HDR_SZ
    move.l  r3, #0                     ; entry index in page
.dmfn_scan_rows:
    move.l  r4, #DOS_META_PER_PAGE
    bge     r3, r4, .dmfn_next_page
    load.b  r5, DOS_META_OFF_NAME(r26)
    beqz    r5, .dmfn_skip             ; empty entry → skip
    ; Case-insensitive name compare (max 32 bytes).
    move.q  r6, r26                    ; r6 = entry name ptr
    move.q  r7, r27                    ; r7 = request name ptr
    move.l  r8, #0                     ; char index
.dmfn_cmp:
    load.b  r9, (r6)
    load.b  r10, (r7)
    move.l  r11, #0x41                 ; 'A'
    blt     r9, r11, .dmfn_skip1
    move.l  r11, #0x5A                 ; 'Z'
    bgt     r9, r11, .dmfn_skip1
    or      r9, r9, #0x20
.dmfn_skip1:
    move.l  r11, #0x41
    blt     r10, r11, .dmfn_skip2
    move.l  r11, #0x5A
    bgt     r10, r11, .dmfn_skip2
    or      r10, r10, #0x20
.dmfn_skip2:
    bne     r9, r10, .dmfn_skip
    beqz    r9, .dmfn_match            ; both null → match
    add     r6, r6, #1
    add     r7, r7, #1
    add     r8, r8, #1
    move.l  r11, #32
    blt     r8, r11, .dmfn_cmp
    bra     .dmfn_match                ; reached 32 chars → treat as match
.dmfn_skip:
    add     r26, r26, #DOS_META_ENTRY_SZ
    add     r3, r3, #1
    bra     .dmfn_scan_rows
.dmfn_next_page:
    load.q  r25, (r25)
    bra     .dmfn_walk_pages
.dmfn_match:
    move.q  r1, r26
    rts
.dmfn_not_found:
    move.q  r1, r0
    rts

    ; ------------------------------------------------------------------
    ; .dos_hnd_alloc: walk the handle chain looking for an unused slot
    ; (entry == 0). If none, allocate a new chain page and use entry 0.
    ; Stores the supplied metadata entry VA in the slot and returns the
    ; integer handle_id (page_index * DOS_HND_PER_PAGE + slot_in_page).
    ; In:  r1  = metadata entry VA to store in the slot (must be non-zero)
    ;      r29 = data base
    ; Out: r1  = handle_id (>= 0 on success)
    ;      r2  = ERR_OK or err code
    ; Clobbers: r3..r19, r25, r26
    ; ------------------------------------------------------------------
.dos_hnd_alloc:
    move.q  r19, r1                    ; r19 = entry VA (preserved)
    load.q  r25, 160(r29)              ; r25 = current handle page VA
    move.l  r12, #0                    ; r12 = page index
.dha_walk_pages:
    beqz    r25, .dha_alloc_new
    move.q  r26, r25
    add     r26, r26, #DOS_HND_HDR_SZ  ; r26 = &slots[0]
    move.l  r3, #0                     ; slot index
.dha_scan_rows:
    move.l  r4, #DOS_HND_PER_PAGE
    bge     r3, r4, .dha_next_page
    load.q  r5, (r26)
    beqz    r5, .dha_found
    add     r26, r26, #DOS_HND_ENTRY_SZ
    add     r3, r3, #1
    bra     .dha_scan_rows
.dha_next_page:
    load.q  r25, (r25)
    add     r12, r12, #1
    bra     .dha_walk_pages
.dha_found:
    store.q r19, (r26)                 ; slot = entry VA
    ; handle_id = page_index * DOS_HND_PER_PAGE + slot_index
    move.l  r4, #DOS_HND_PER_PAGE
    mulu    r4, r12, r4
    add     r1, r4, r3
    move.q  r2, r0                     ; ERR_OK
    rts
.dha_alloc_new:
    push    r29
    push    r19
    push    r12
    move.l  r1, #4096
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM
    pop     r12
    pop     r19
    pop     r29
    bnez    r2, .dha_fail
    move.q  r25, r1                    ; new page VA
    ; Link onto chain.
    load.q  r3, 160(r29)               ; head
    beqz    r3, .dha_set_head
.dha_link_walk:
    load.q  r4, (r3)
    beqz    r4, .dha_at_tail
    move.q  r3, r4
    bra     .dha_link_walk
.dha_at_tail:
    store.q r25, (r3)
    bra     .dha_link_done
.dha_set_head:
    store.q r25, 160(r29)              ; head = new page
.dha_link_done:
    ; Use slot 0 of the new page.
    add     r26, r25, #DOS_HND_HDR_SZ
    store.q r19, (r26)
    move.l  r4, #DOS_HND_PER_PAGE
    mulu    r4, r12, r4
    move.q  r1, r4                     ; slot index = 0
    move.q  r2, r0                     ; ERR_OK
    rts
.dha_fail:
    move.q  r1, r0
    ; r2 has err code from AllocMem
    rts

    ; ------------------------------------------------------------------
    ; .dos_hnd_lookup: walk handle chain to slot at handle_id, return
    ; the metadata entry VA stored there. Returns 0 if handle_id is
    ; out of range or the slot is empty.
    ; In:  r1  = handle_id
    ;      r29 = data base
    ; Out: r1  = metadata entry VA (or 0)
    ;      r2  = slot VA inside the chain page (for callers that need
    ;            to clear the slot, e.g. DOS_CLOSE; 0 if not found)
    ; Clobbers: r3..r10, r25
    ; ------------------------------------------------------------------
.dos_hnd_lookup:
    bltz    r1, .dhl_not_found
    move.l  r3, #DOS_HND_PER_PAGE
    divu    r4, r1, r3                 ; r4 = page index
    mulu    r5, r4, r3
    sub     r5, r1, r5                 ; r5 = slot in page
    load.q  r25, 160(r29)              ; chain head
.dhl_walk:
    beqz    r25, .dhl_not_found
    beqz    r4, .dhl_at_page
    load.q  r25, (r25)
    sub     r4, r4, #1
    bra     .dhl_walk
.dhl_at_page:
    move.l  r6, #DOS_HND_ENTRY_SZ
    mulu    r6, r5, r6
    add     r6, r6, #DOS_HND_HDR_SZ
    add     r6, r25, r6                ; r6 = slot VA
    load.q  r1, (r6)
    beqz    r1, .dhl_empty
    move.q  r2, r6
    rts
.dhl_empty:
    move.q  r1, r0
    move.q  r2, r0
    rts
.dhl_not_found:
    move.q  r1, r0
    move.q  r2, r0
    rts

    ; ==================================================================
    ; M12.8 Phase 1 — file body extent allocator (DEAD CODE in Phase 1).
    ;
    ; These helpers allocate, free, and walk a chain of 4 KiB extents
    ; that will replace the fixed-size DOS_FILE_SIZE per-file body in
    ; Phase 2. They are wired into NO existing code path in Phase 1;
    ; they only need to assemble cleanly and not break any existing
    ; test. Phase 2 will switch DOS_OPEN/READ/WRITE over and remove
    ; the DOS_FILE_SIZE cap.
    ;
    ; Extent layout (one AllocMem'd 4 KiB page):
    ;   [0..7]    next_va (0 = end of chain)
    ;   [8..15]   reserved
    ;   [16..4095] payload (DOS_EXT_PAYLOAD = 4080 bytes)
    ; ==================================================================

    ; ------------------------------------------------------------------
    ; .dos_extent_alloc: allocate a chain of 4 KiB extents large enough
    ; to hold byte_count payload bytes.
    ; In:  r1  = byte_count (>=0; 0 means "no body, return first_va=0")
    ;      r29 = data base
    ; Out: r1  = first_extent_va (or 0 if byte_count==0 or alloc failed)
    ;      r2  = ERR_OK on success, AllocMem err code on failure
    ; Clobbers: r3, r17, r18, r19
    ; ------------------------------------------------------------------
.dos_extent_alloc:
    beqz    r1, .dea_zero_size
    ; n_extents = ceil(byte_count / DOS_EXT_PAYLOAD)
    add     r17, r1, #(DOS_EXT_PAYLOAD - 1)
    move.l  r3, #DOS_EXT_PAYLOAD
    divu    r17, r17, r3                ; r17 = remaining extents to allocate
    ; Allocate the first extent.
    push    r29
    push    r17
    move.l  r1, #4096
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM
    pop     r17
    pop     r29
    bnez    r2, .dea_fail_first
    move.q  r19, r1                     ; r19 = first_va (preserved)
    move.q  r18, r1                     ; r18 = tail_va (advances each loop)
    sub     r17, r17, #1
.dea_loop:
    beqz    r17, .dea_done
    push    r29
    push    r19
    push    r18
    push    r17
    move.l  r1, #4096
    move.l  r2, #MEMF_CLEAR
    syscall #SYS_ALLOC_MEM
    pop     r17
    pop     r18
    pop     r19
    pop     r29
    bnez    r2, .dea_fail_partial
    store.q r1, DOS_EXT_OFF_NEXT(r18)   ; tail.next_va = new extent
    move.q  r18, r1                     ; advance tail
    sub     r17, r17, #1
    bra     .dea_loop
.dea_done:
    move.q  r1, r19
    move.q  r2, r0                      ; ERR_OK
    rts
.dea_zero_size:
    move.q  r1, r0
    move.q  r2, r0
    rts
.dea_fail_first:
    move.q  r1, r0
    ; r2 already holds the AllocMem err code
    rts
.dea_fail_partial:
    ; r2 holds the AllocMem err code; preserve it across the free call.
    push    r2
    move.q  r1, r19                     ; head of partially-allocated chain
    jsr     .dos_extent_free            ; preserves r29 internally
    pop     r2
    move.q  r1, r0
    rts

    ; ------------------------------------------------------------------
    ; .dos_extent_free: walk an extent chain and FreeMem each page.
    ; Safe to call with r1 == 0 (no-op).
    ; In:  r1  = first_extent_va (or 0)
    ;      r29 = data base
    ; Out: r2  = ERR_OK
    ; Clobbers: r1, r18, r19
    ; ------------------------------------------------------------------
.dos_extent_free:
    move.q  r19, r1                     ; r19 = current extent
.def_loop:
    beqz    r19, .def_done
    load.q  r18, DOS_EXT_OFF_NEXT(r19)  ; r18 = next extent
    push    r29
    push    r18
    move.q  r1, r19
    move.l  r2, #4096
    syscall #SYS_FREE_MEM
    pop     r18
    pop     r29
    move.q  r19, r18
    bra     .def_loop
.def_done:
    move.q  r2, r0                      ; ERR_OK
    rts

    ; ------------------------------------------------------------------
    ; .dos_extent_walk: copy up to byte_count bytes from the start of an
    ; extent chain into a destination buffer. Returns the number of
    ; bytes actually copied — equals byte_count if the chain is long
    ; enough, otherwise the chain length in bytes.
    ; In:  r1  = first_extent_va (or 0)
    ;      r2  = dst VA
    ;      r3  = byte_count to copy
    ;      r29 = data base
    ; Out: r1  = bytes copied
    ; Clobbers: r4, r5, r6, r7, r8, r16, r17, r18, r19
    ; ------------------------------------------------------------------
.dos_extent_walk:
    move.q  r19, r1                     ; r19 = current extent VA
    move.q  r18, r2                     ; r18 = dst
    move.q  r17, r3                     ; r17 = remaining bytes to copy
    move.q  r16, r0                     ; r16 = total copied
.dew_extent_loop:
    beqz    r19, .dew_done
    beqz    r17, .dew_done
    ; n = min(r17, DOS_EXT_PAYLOAD)
    move.q  r4, r17
    move.l  r5, #DOS_EXT_PAYLOAD
    blt     r4, r5, .dew_have_n
    move.q  r4, r5
.dew_have_n:
    ; r4 = bytes to copy this extent
    add     r5, r19, #DOS_EXT_HDR_SZ    ; src = extent + header
    move.q  r6, r18                     ; dst
    move.q  r7, r0                      ; counter
.dew_byte_copy:
    bge     r7, r4, .dew_extent_done
    load.b  r8, (r5)
    store.b r8, (r6)
    add     r5, r5, #1
    add     r6, r6, #1
    add     r7, r7, #1
    bra     .dew_byte_copy
.dew_extent_done:
    add     r16, r16, r4                ; total += n
    add     r18, r18, r4                ; dst += n
    sub     r17, r17, r4                ; remaining -= n
    load.q  r19, DOS_EXT_OFF_NEXT(r19)  ; advance to next extent
    bra     .dew_extent_loop
.dew_done:
    move.q  r1, r16
    rts

    ; ------------------------------------------------------------------
    ; .dos_extent_write: copy up to byte_count bytes from a source
    ; buffer into the start of an extent chain. Symmetric counterpart
    ; to .dos_extent_walk. The extent chain MUST already have been
    ; allocated (via .dos_extent_alloc) with enough capacity; this
    ; helper does NOT grow the chain. Returns bytes actually written
    ; (= byte_count when the chain has enough capacity, otherwise the
    ; chain capacity in bytes).
    ; In:  r1  = first_extent_va (or 0)
    ;      r2  = src VA
    ;      r3  = byte_count to copy
    ;      r29 = data base
    ; Out: r1  = bytes written
    ; Clobbers: r4, r5, r6, r7, r8, r16, r17, r18, r19
    ; ------------------------------------------------------------------
.dos_extent_write:
    move.q  r19, r1                     ; r19 = current extent VA
    move.q  r18, r2                     ; r18 = src
    move.q  r17, r3                     ; r17 = remaining bytes to copy
    move.q  r16, r0                     ; r16 = total written
.dexw_extent_loop:
    beqz    r19, .dexw_done
    beqz    r17, .dexw_done
    ; n = min(r17, DOS_EXT_PAYLOAD)
    move.q  r4, r17
    move.l  r5, #DOS_EXT_PAYLOAD
    blt     r4, r5, .dexw_have_n
    move.q  r4, r5
.dexw_have_n:
    ; r4 = bytes to copy this extent
    add     r5, r19, #DOS_EXT_HDR_SZ    ; dst = extent + header
    move.q  r6, r18                     ; src = caller buffer
    move.q  r7, r0                      ; counter
.dexw_byte_copy:
    bge     r7, r4, .dexw_extent_done
    load.b  r8, (r6)
    store.b r8, (r5)
    add     r5, r5, #1
    add     r6, r6, #1
    add     r7, r7, #1
    bra     .dexw_byte_copy
.dexw_extent_done:
    add     r16, r16, r4                ; total += n
    add     r18, r18, r4                ; src += n
    sub     r17, r17, r4                ; remaining -= n
    load.q  r19, DOS_EXT_OFF_NEXT(r19)  ; advance to next extent
    bra     .dexw_extent_loop
.dexw_done:
    move.q  r1, r16
    rts

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
    ; --- Offset 152: meta_chain_head_va (8) — M12.6 Phase A ---
    ds.b    8
    ; --- Offset 160: hnd_chain_head_va  (8) — M12.6 Phase A ---
    ds.b    8
    ; --- Offset 168: caller_mapped_va (8) ---
    ds.b    8
    ; --- Offset 176: reserved (was open_handles[8] before M12.6 Phase A) ---
    ds.b    8
    ; --- Offset 184: cached share_pages (8) ---
    ds.b    8
    ; --- Offset 192..895: dead-space scratch (was: file_table 16×44 before M12.6 Phase A) ---
    ; .dos_seed_one uses 192..223 as save slots during boot.
    ds.b    704
    ; --- Offset 896: pre-create filename "readme\0" + pad to 16 ---
    dc.b    "readme", 0, 0, 0, 0, 0, 0, 0, 0, 0, 0
    ; --- Offset 912: pre-create content ---
    dc.b    "Welcome to IntuitionOS M11", 0x0D, 0x0A, 0
    ; --- Offset 941: pad 1 byte ---
    ds.b    1
    ; --- Offset 942: seed file names ---
    dc.b    "C/Version", 0              ; 942 (10 bytes)
    dc.b    "C/Avail", 0                ; 952 (8 bytes)
    dc.b    "C/Dir", 0                  ; 960 (6 bytes)
    dc.b    "C/Type", 0                 ; 966 (7 bytes)
    dc.b    "C/Echo", 0                 ; 973 (7 bytes)
    dc.b    "S/Startup-Sequence", 0     ; 980 (19 bytes) → 999
    ds.b    1                           ; pad to 1000
    ; --- Offset 1000: name resolution scratch buffer (32 bytes) ---
    ds.b    32
    ; --- Offset 1032: M11 seed names ---
    dc.b    "DEVS/input.device", 0      ; 1032 (18 bytes) → 1050
    dc.b    "LIBS/graphics.library", 0  ; 1050 (22 bytes) → 1072
    dc.b    "C/GfxDemo", 0              ; 1072 (10 bytes) → 1082
    ; --- M12 seed names ---
    dc.b    "LIBS/intuition.library", 0 ; 1082 (23 bytes) → 1105
    dc.b    "C/About", 0                ; 1105 (8 bytes) → 1113
    ; --- M12.5 seed names ---
    dc.b    "RESOURCES/hardware.resource", 0  ; 1113 (28 bytes) → 1141
    ; --- Pad to 4096 (page boundary) ---
    ; 1141 → 4096: 2955 bytes padding
    ds.b    2955

; ---------------------------------------------------------------------------
; Embedded command images (VERSION, AVAIL, DIR, TYPE, ECHO)
; ---------------------------------------------------------------------------

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
    syscall #SYS_FIND_PORT  
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
    dc.b    "IntuitionOS 0.15 (exec.library M11.6 / intuition.library M12 / hardware.resource M12.5 / cap sweep M12.6 / dos storage M12.8)", 0x0D, 0x0A, 0
prog_version_data_end:
    align   8
prog_version_end:

; ---------------------------------------------------------------------------
; AVAIL — display physical vs allocatable memory
; ---------------------------------------------------------------------------
; Opens console.handler and prints:
;   Phys  = total machine RAM (MMU_NUM_PAGES)
;   Alloc = Exec allocator-pool pages (SYSINFO_TOTAL_PAGES)
;   Free  = currently free allocator-pool pages (SYSINFO_FREE_PAGES)

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
    syscall #SYS_FIND_PORT  
    load.q  r29, (sp)
    beqz    r2, .av_open_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .av_open_retry
.av_open_ok:
    store.q r1, 64(r29)                ; data[64] = console_port

    ; === Send "Phys: " ===
    add     r20, r29, #16
    jsr     .av_send_string

    ; === Physical RAM = MMU_NUM_PAGES → multiply by 4 for KB ===
    move.l  r1, #MMU_NUM_PAGES
    lsl     r1, r1, #2                 ; pages * 4 = KB (4KB pages)
    store.q r1, 8(sp)                  ; save value
    jsr     .av_print_number

    ; === Send " KB  Alloc: " ===
    load.q  r29, (sp)
    add     r20, r29, #24
    jsr     .av_send_string

    ; === GetSysInfo(TOTAL_PAGES = allocator pool) → multiply by 4 for KB ===
    move.l  r1, #SYSINFO_TOTAL_PAGES
    syscall #SYS_GET_SYS_INFO
    load.q  r29, (sp)
    lsl     r1, r1, #2                 ; pages * 4 = KB (4KB pages)
    store.q r1, 8(sp)                  ; save value
    jsr     .av_print_number

    ; === Send " KB  Free: " ===
    load.q  r29, (sp)
    add     r20, r29, #40
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
    add     r20, r29, #52
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
    ; --- Offset 16: "Phys: \0" (8 bytes) ---
    dc.b    "Phys: ", 0, 0
    ; --- Offset 24: " KB  Alloc: \0" (16 bytes) ---
    dc.b    " KB  Alloc: ", 0, 0, 0, 0
    ; --- Offset 40: " KB  Free: \0" (12 bytes) ---
    dc.b    " KB  Free: ", 0
    ; --- Offset 52: " KB\r\n\0" + pad to 64 ---
    dc.b    " KB", 0x0D, 0x0A, 0
    ds.b    5                           ; pad to offset 64
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
    syscall #SYS_FIND_PORT  
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
    syscall #SYS_FIND_PORT  
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
    syscall #SYS_FIND_PORT  
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
    syscall #SYS_FIND_PORT  
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
    syscall #SYS_FIND_PORT  
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

seed_startup:
    dc.b    "RESOURCES:hardware.resource", 0x0A
    dc.b    "DEVS:input.device", 0x0A
    dc.b    "LIBS:graphics.library", 0x0A
    dc.b    "LIBS:intuition.library", 0x0A
    dc.b    "VERSION", 0x0A
    dc.b    "ECHO Core OS objects: fixed product limits removed where practical", 0x0A
    dc.b    "ECHO dos.library file storage: variable-size, no per-file cap", 0x0A
    dc.b    "ECHO IntuitionOS M12.8 ready", 0x0A
    dc.b    "ECHO All visible services are running in user space", 0x0A, 0
seed_startup_end:
    align   8

; ---------------------------------------------------------------------------
; input.device — keyboard/mouse event service (M11)
; ---------------------------------------------------------------------------
; Polls SCAN_*/MOUSE_* registers (mapped via SYS_MAP_IO with the M11
; range-aware extension), pushes INPUT_EVENT messages to a single registered
; subscriber port. Single subscriber for M11; multi-subscriber fan-out is M12
; work in intuition.library.
;
; Protocol: see iexec.inc INPUT_OPEN / INPUT_CLOSE / INPUT_EVENT.
; Data layout:
;   0:   "console.handler\0"  (16 bytes — unused, kept for standard layout)
;   16:  "input.device\0"     (16 bytes, padded — port name)
;   32:  banner string        (32 bytes — "input.device ONLINE [Task ")
;   64:  padding              (64 bytes)
;   128: task_id              (8 bytes)
;   136: (unused)             (8 bytes)
;   144: input_port           (8 bytes — own port_id)
;   152: chip_mmio_va         (8 bytes — from SYS_MAP_IO)
;   160: subscriber_port      (8 bytes, 0 = none)
;   168: last_mouse_x         (4 bytes)
;   172: last_mouse_y         (4 bytes)
;   176: last_mouse_buttons   (4 bytes)
;   180: event_seq            (4 bytes — monotonic event counter)
;   184: padding              (8 bytes)
prog_input_device:
    dc.l    IMG_MAGIC_LO, IMG_MAGIC_HI
    dc.l    prog_input_device_code_end - prog_input_device_code
    dc.l    prog_input_device_data_end - prog_input_device_data
    dc.l    0
    ds.b    12
prog_input_device_code:

    ; ===== Preamble: compute data page base (preempt-safe) =====
    sub     sp, sp, #16
.idev_preamble:
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    store.q r1, 8(sp)
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    load.q  r28, 8(sp)
    bne     r1, r28, .idev_preamble
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
    bne     r29, r28, .idev_preamble
    store.q r29, (sp)
    load.q  r1, 8(sp)
    store.q r1, 128(r29)               ; data[128] = task_id

    ; ===== M12.5: Request CHIP grant from hardware.resource =====
    ; SYS_MAP_IO is now gated by the kernel grant table; we must hold a
    ; grant for PPN 0xF0 before calling it. The broker is the only producer
    ; of grants for a non-bootstrap task. Spin on FindPort until the broker
    ; is up (boot launch order in S/Startup-Sequence puts hardware.resource
    ; first, but we still poll to be safe across launch-order edits).
.idev_find_hwres:
    add     r1, r29, #192              ; r1 = "hardware.resource" string
    syscall #SYS_FIND_PORT             ; R1=port_id, R2=err
    bnez    r2, .idev_hwres_retry
    bra     .idev_have_hwres
.idev_hwres_retry:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .idev_find_hwres
.idev_have_hwres:
    store.q r1, 224(r29)               ; data[224] = hwres_port

    ; Create anonymous reply port
    move.q  r1, r0
    move.q  r2, r0
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    bnez    r2, .idev_halt
    store.q r1, 232(r29)               ; data[232] = reply_port

    ; PutMsg(hwres_port, HWRES_MSG_REQUEST, tag=CHIP, data1=task_id, reply=reply_port)
    load.q  r1, 224(r29)               ; hwres_port
    move.l  r2, #HWRES_MSG_REQUEST
    move.l  r3, #HWRES_TAG_CHIP        ; data0 = tag
    load.q  r4, 128(r29)               ; data1 = my task_id
    load.q  r5, 232(r29)               ; reply_port
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    bnez    r2, .idev_halt

    ; Wait for reply (WaitPort returns message data directly)
    load.q  r1, 232(r29)
    syscall #SYS_WAIT_PORT             ; R1=type, R2=data0, R3=err
    load.q  r29, (sp)
    bnez    r3, .idev_halt
    move.l  r28, #HWRES_MSG_GRANTED
    bne     r1, r28, .idev_halt        ; broker denied → halt

    ; ===== SYS_MAP_IO(R1=0xF0, R2=1) =====
    ; Now we hold a CHIP grant; the kernel grant check will succeed.
    move.l  r1, #TERM_IO_PAGE
    move.l  r2, #1
    syscall #SYS_MAP_IO
    load.q  r29, (sp)
    bnez    r2, .idev_halt
    store.q r1, 152(r29)               ; data[152] = chip_mmio_va

    ; ===== CreatePort("input.device") =====
    add     r1, r29, #16
    move.l  r2, #PF_PUBLIC
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    bnez    r2, .idev_halt
    store.q r1, 144(r29)               ; data[144] = input_port

    ; ===== Print banner via SYS_DEBUG_PUTCHAR =====
    add     r20, r29, #32              ; r20 = &data[32] = banner
.idev_ban_loop:
    load.b  r1, (r20)
    beqz    r1, .idev_ban_id
    store.q r20, 8(sp)
    syscall #SYS_DEBUG_PUTCHAR
    load.q  r29, (sp)
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .idev_ban_loop
.idev_ban_id:
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

    ; ===== Main loop =====
.idev_main:
    load.q  r29, (sp)

    ; --- Try to get a message (non-blocking) ---
    load.q  r1, 144(r29)               ; input_port
    syscall #SYS_GET_MSG               ; R1=type R2=data0 R3=err R4=data1 R5=reply R6=share
    bnez    r3, .idev_poll             ; ERR_AGAIN → no msg

    ; --- Got message: dispatch ---
    move.q  r24, r2                    ; r24 = data0 (subscriber port for OPEN)
    move.q  r25, r5                    ; r25 = reply_port

    move.l  r28, #INPUT_OPEN
    beq     r1, r28, .idev_do_open
    move.l  r28, #INPUT_CLOSE
    beq     r1, r28, .idev_do_close
    bra     .idev_main                 ; unknown opcode, drop

.idev_do_open:
    load.q  r29, (sp)
    load.q  r14, 160(r29)              ; current subscriber
    bnez    r14, .idev_open_busy
    store.q r24, 160(r29)              ; subscriber = data0
    move.q  r1, r25                    ; reply_port
    move.l  r2, #INPUT_ERR_OK
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    bra     .idev_main

.idev_open_busy:
    move.q  r1, r25
    move.l  r2, #INPUT_ERR_BUSY
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    bra     .idev_main

.idev_do_close:
    load.q  r29, (sp)
    store.q r0, 160(r29)               ; clear subscriber
    move.q  r1, r25
    move.l  r2, #INPUT_ERR_OK
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    bra     .idev_main

    ; --- No message: poll input registers ---
.idev_poll:
    load.q  r29, (sp)
    load.q  r24, 160(r29)              ; subscriber
    beqz    r24, .idev_yield           ; no subscriber → just yield
    load.q  r25, 152(r29)              ; chip_mmio_va

    ; --- Drain keyboard scancodes ---
.idev_kbd_drain:
    add     r14, r25, #0x744           ; SCAN_STATUS
    load.l  r14, (r14)
    and     r14, r14, #1
    beqz    r14, .idev_kbd_done
    add     r15, r25, #0x740           ; SCAN_CODE (read auto-dequeues)
    load.l  r15, (r15)
    add     r16, r25, #0x748           ; SCAN_MODIFIERS
    load.l  r16, (r16)

    ; Build event word: (IE_KEY_DOWN<<24) | (scancode<<16) | (modifiers<<8)
    move.l  r17, #IE_KEY_DOWN
    lsl     r17, r17, #24
    and     r15, r15, #0xFF
    lsl     r15, r15, #16
    or      r17, r17, r15
    and     r16, r16, #0xFF
    lsl     r16, r16, #8
    or      r17, r17, r16

    ; Build mn_Data1: (mx16<<48) | (my16<<32) | event_seq32
    add     r18, r25, #0x730           ; MOUSE_X
    load.l  r18, (r18)
    and     r18, r18, #0xFFFF
    lsl     r18, r18, #48
    add     r19, r25, #0x734           ; MOUSE_Y
    load.l  r19, (r19)
    and     r19, r19, #0xFFFF
    lsl     r19, r19, #32
    or      r18, r18, r19
    load.l  r19, 180(r29)
    add     r19, r19, #1
    store.l r19, 180(r29)
    or      r18, r18, r19

    ; PutMsg(subscriber, INPUT_EVENT, r17, r18, NONE, 0)
    move.q  r1, r24
    move.l  r2, #INPUT_EVENT
    move.q  r3, r17
    move.q  r4, r18
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    load.q  r24, 160(r29)              ; reload subscriber
    beqz    r24, .idev_yield
    load.q  r25, 152(r29)
    bra     .idev_kbd_drain

.idev_kbd_done:
    ; --- Mouse: check status (reading clears change flag) ---
    add     r14, r25, #0x73C           ; MOUSE_STATUS
    load.l  r14, (r14)
    and     r14, r14, #1
    beqz    r14, .idev_yield

    add     r15, r25, #0x730           ; MOUSE_X
    load.l  r15, (r15)
    and     r15, r15, #0xFFFF
    add     r16, r25, #0x734           ; MOUSE_Y
    load.l  r16, (r16)
    and     r16, r16, #0xFFFF
    add     r17, r25, #0x738           ; MOUSE_BUTTONS
    load.l  r17, (r17)
    and     r17, r17, #0xFF

    load.l  r18, 168(r29)              ; last_mouse_x
    load.l  r19, 172(r29)              ; last_mouse_y
    load.l  r20, 176(r29)              ; last_mouse_buttons

    ; --- Position changed? Emit IE_MOUSE_MOVE ---
    bne     r15, r18, .idev_mv_emit
    bne     r16, r19, .idev_mv_emit
    bra     .idev_mv_check_btn

.idev_mv_emit:
    move.l  r21, #IE_MOUSE_MOVE
    lsl     r21, r21, #24
    move.q  r22, r15
    lsl     r22, r22, #48
    move.q  r23, r16
    lsl     r23, r23, #32
    or      r22, r22, r23
    load.l  r19, 180(r29)
    add     r19, r19, #1
    store.l r19, 180(r29)
    or      r22, r22, r19
    move.q  r1, r24
    move.l  r2, #INPUT_EVENT
    move.q  r3, r21
    move.q  r4, r22
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    load.q  r24, 160(r29)
    beqz    r24, .idev_save_state
    load.l  r20, 176(r29)              ; reload last_buttons (mouse coords r15/r16 unchanged in r registers)

.idev_mv_check_btn:
    ; --- Buttons changed? Emit IE_MOUSE_BTN ---
    beq     r17, r20, .idev_save_state
    move.l  r21, #IE_MOUSE_BTN
    lsl     r21, r21, #24
    move.q  r23, r17
    lsl     r23, r23, #16
    or      r21, r21, r23
    move.q  r22, r15
    lsl     r22, r22, #48
    move.q  r23, r16
    lsl     r23, r23, #32
    or      r22, r22, r23
    load.l  r19, 180(r29)
    add     r19, r19, #1
    store.l r19, 180(r29)
    or      r22, r22, r19
    move.q  r1, r24
    move.l  r2, #INPUT_EVENT
    move.q  r3, r21
    move.q  r4, r22
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)

.idev_save_state:
    store.l r15, 168(r29)
    store.l r16, 172(r29)
    store.l r17, 176(r29)

.idev_yield:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .idev_main

.idev_halt:
    syscall #SYS_YIELD
    bra     .idev_halt
prog_input_device_code_end:

prog_input_device_data:
    dc.b    "console.handler", 0
    dc.b    "input.device", 0, 0, 0, 0
    dc.b    "input.device ONLINE [Task ", 0
    ds.b    5                           ; pad to offset 64
    ds.b    64                          ; pad to offset 128
    ds.b    8                           ; 128: task_id
    ds.b    8                           ; 136: (unused)
    ds.b    8                           ; 144: input_port
    ds.b    8                           ; 152: chip_mmio_va
    ds.b    8                           ; 160: subscriber_port
    ds.b    4                           ; 168: last_mouse_x
    ds.b    4                           ; 172: last_mouse_y
    ds.b    4                           ; 176: last_mouse_buttons
    ds.b    4                           ; 180: event_seq
    ds.b    8                           ; 184: pad
    ; --- M12.5 additions ---
    dc.b    "hardware.resource", 0      ; 192: broker port name
    ds.b    14                          ; pad to offset 224 (192+32)
    ds.b    8                           ; 224: hwres_port
    ds.b    8                           ; 232: reply_port
prog_input_device_data_end:
    align   8
prog_input_device_end:

; ---------------------------------------------------------------------------
; hardware.resource — user-space MMIO arbiter (M12.5)
; ---------------------------------------------------------------------------
; The first user-space service to claim broker identity via SYS_HWRES_OP /
; HWRES_BECOME. Owns the policy mapping from 4-byte region tags ('CHIP',
; 'VRAM') to physical PPN ranges. Clients send HWRES_MSG_REQUEST naming a
; tag and their own task ID; the broker resolves the tag, calls
; SYS_HWRES_OP / HWRES_CREATE to write a grant row covering the right PPN
; range for the requesting task, and replies with HWRES_MSG_GRANTED whose
; data0 carries (ppn_base<<32) | page_count so the client can call
; SYS_MAP_IO with values it learned from the broker (no PPN literals
; baked into clients).
;
; Data layout (offsets relative to data page):
;   0..31:  port name "hardware.resource\0..." (32 bytes; PORT_NAME_LEN=32)
;   32..95: banner "hardware.resource ONLINE [Task " (variable, padded)
;   96..127: pad
;   128:    task_id (8 bytes)
;   136:    hwres_port (8 bytes)
;   144:    pad

prog_hwres:
    dc.l    IMG_MAGIC_LO, IMG_MAGIC_HI
    dc.l    prog_hwres_code_end - prog_hwres_code
    dc.l    prog_hwres_data_end - prog_hwres_data
    dc.l    0
    ds.b    12
prog_hwres_code:

    ; ===== Preamble: compute data page base (preempt-safe) =====
    sub     sp, sp, #16
.hwres_preamble:
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    store.q r1, 8(sp)
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    load.q  r28, 8(sp)
    bne     r1, r28, .hwres_preamble
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
    bne     r29, r28, .hwres_preamble
    store.q r29, (sp)
    load.q  r1, 8(sp)
    store.q r1, 128(r29)               ; data[128] = task_id

    ; ===== SYS_HWRES_OP / HWRES_BECOME =====
    move.l  r6, #HWRES_BECOME
    syscall #SYS_HWRES_OP              ; R2 = err
    load.q  r29, (sp)
    bnez    r2, .hwres_halt            ; can't claim broker → give up

    ; ===== CreatePort("hardware.resource", PF_PUBLIC) =====
    move.q  r1, r29                    ; r1 = &data[0] = port name
    move.l  r2, #PF_PUBLIC
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    bnez    r2, .hwres_halt
    store.q r1, 136(r29)               ; data[136] = hwres_port

    ; ===== Print banner =====
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

    ; r1 = msg type, r2 = tag, r5 = reply_port, r7 = sender (kernel-trusted)
    move.q  r24, r2                    ; r24 = tag
    move.q  r25, r7                    ; r25 = sender task ID (TRUSTED)
    move.q  r26, r5                    ; r26 = reply_port

    move.l  r28, #HWRES_MSG_REQUEST
    bne     r1, r28, .hwres_main       ; ignore unknown message types

    ; ===== Sanity-check sender task ID =====
    move.l  r28, #MAX_TASKS
    bge     r25, r28, .hwres_deny
    bltz    r25, .hwres_deny

    ; ===== M12.5 v2 hardening: scrub stale owner slots =====
    ; Walk the broker's per-tag owner lists and clear any slot whose task
    ; has exited (kernel reports state == TASK_FREE). Without this scrub a
    ; recycled task slot would be silently regranted (because the slot ID
    ; matches the dead owner) or a different task would be blocked forever
    ; (because the dead owner still occupies the slot). The kernel grant
    ; chain and KD_HWRES_TASK are cleaned up by kill_task_cleanup, but
    ; the broker's private owner state isn't visible to the kernel.
    ;
    ; CHIP slots: data[144..147] (4 entries)
    ; VRAM slot:  data[148]      (1 entry)
    add     r17, r29, #144              ; r17 = scan cursor
    move.l  r18, #5                     ; total slots to scrub (4 CHIP + 1 VRAM)
.hwres_scrub:
    beqz    r18, .hwres_scrub_done
    load.b  r14, (r17)
    move.l  r15, #0xFF
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
    move.l  r14, #0xFF
    store.b r14, (r17)                  ; dead — reclaim slot
.hwres_scrub_next:
    add     r17, r17, #1
    sub     r18, r18, #1
    bra     .hwres_scrub
.hwres_scrub_done:

    ; ===== Resolve tag in policy table =====
    ; M12.5 trust gating: each tag has a per-tag owner list. Sender either
    ; finds itself already in the list (idempotent re-grant) or claims a
    ; FREE slot. List full → DENY. Stale slots have already been reclaimed
    ; by the scrub pass above so dead owners do not block live requesters.
    ;
    ; CHIP owners: data[144..147] (4 slots; chip MMIO is shared)
    ; VRAM owner:  data[148]      (1 slot; framebuffer is monopolized)
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
    ; data[144..147] (4 slots, 0xFF = unclaimed). A request is granted
    ; if the sender is already in the list (idempotent re-grant) OR
    ; if there is a free slot to record them. Otherwise DENY.
    add     r28, r29, #144              ; r28 = &owner_list[0]
    move.l  r27, #4                     ; list size
    move.q  r17, r28                    ; r17 = scan cursor
    move.q  r16, r0                     ; r16 = first free slot ptr (0 = none)
.hwres_chip_scan:
    beqz    r27, .hwres_chip_check_free
    load.b  r15, (r17)
    move.l  r14, #0xFF
    beq     r15, r14, .hwres_chip_remember_free
    beq     r15, r25, .hwres_chip_set_range  ; sender already in list
    bra     .hwres_chip_scan_next
.hwres_chip_remember_free:
    bnez    r16, .hwres_chip_scan_next
    move.q  r16, r17
.hwres_chip_scan_next:
    add     r17, r17, #1
    sub     r27, r27, #1
    bra     .hwres_chip_scan
.hwres_chip_check_free:
    beqz    r16, .hwres_deny           ; list full → deny
    store.b r25, (r16)                  ; claim free slot
.hwres_chip_set_range:
    move.l  r17, #0xF0                 ; r17 = ppn_lo
    move.l  r18, #0xF0                 ; r18 = ppn_hi
    move.l  r19, #1                    ; r19 = count
    bra     .hwres_do_create

.hwres_grant_vram:
    ; VRAM is a MONOPOLY resource — only one task may own the framebuffer
    ; (M12 single-display-client rule). One owner slot at data[148].
    load.b  r28, 148(r29)              ; r28 = current VRAM owner
    move.l  r27, #0xFF
    beq     r28, r27, .hwres_vram_claim
    bne     r28, r25, .hwres_deny
    bra     .hwres_vram_set_range
.hwres_vram_claim:
    store.b r25, 148(r29)
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

.hwres_halt:
    syscall #SYS_YIELD
    bra     .hwres_halt
prog_hwres_code_end:

prog_hwres_data:
    dc.b    "hardware.resource", 0     ; 0..17 (port name, padded below)
    ds.b    14                          ; 18..31 (pad to PORT_NAME_LEN=32)
    dc.b    "hardware.resource ONLINE [Task ", 0   ; 32..63 (banner)
    ds.b    32                          ; 64..95 (pad)
    ds.b    32                          ; 96..127 (pad to data[128])
    ds.b    8                           ; 128: task_id
    ds.b    8                           ; 136: hwres_port
    ; --- M12.5 trust gating: per-tag owner slots ---
    ; 144..147: CHIP owner list (4 slots, 0xFF = unclaimed). CHIP is shared
    ;           because chip MMIO holds terminal/input/video registers all
    ;           in one physical page; multiple services need it.
    ; 148:      VRAM owner (1 slot, 0xFF = unclaimed). VRAM is a monopoly —
    ;           only one task may own the framebuffer per the M12
    ;           single-display-client rule.
    ; All slots default to 0xFF because 0 is a valid task ID (console.handler)
    ; and the broker must distinguish "unclaimed" from "owned by task 0".
    dc.b    0xFF, 0xFF, 0xFF, 0xFF      ; 144..147: CHIP owner list
    dc.b    0xFF                        ; 148: VRAM owner
    ds.b    11                          ; 149..159: pad
prog_hwres_data_end:
    align   8
prog_hwres_end:

; ---------------------------------------------------------------------------
; graphics.library — fullscreen RGBA32 display service (M11, M12: 800x600)
; ---------------------------------------------------------------------------
; Maps the chip register page (0xF0) and the 800x600x4 VRAM range
; (PPNs 0x100..0x2D5 = 470 pages = 1925120 bytes), creates the
; "graphics.library" port, then services requests synchronously.
;
; M12: bumped from 640x480 to 800x600 to match the chip's DEFAULT_VIDEO_MODE
; and give clients more screen real estate. The chip is left in mode 1 the
; whole time (the chip's DEFAULT_VIDEO_MODE = MODE_800x600), so a kernel-side
; VideoTerminal that started in 800x600 keeps the same framebuffer dimensions.
; The protocol still allows clients to request other modes — graphics.library
; just defaults to 800x600 in M12 because no other mode is enumerated yet.
;
; Single surface only (USER_DYN_PAGES=768 doesn't fit two persistent surface
; mappings + persistent VRAM). Client double-buffering remains deferred.
;
; Protocol: see iexec.inc GFX_* constants.
prog_graphics_library:
    dc.l    IMG_MAGIC_LO, IMG_MAGIC_HI
    dc.l    prog_gfxlib_code_end - prog_gfxlib_code
    dc.l    prog_gfxlib_data_end - prog_gfxlib_data
    dc.l    0
    ds.b    12
prog_gfxlib_code:

    ; ===== Preamble =====
    sub     sp, sp, #16
.gfx_preamble:
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    store.q r1, 8(sp)
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    load.q  r28, 8(sp)
    bne     r1, r28, .gfx_preamble
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
    bne     r29, r28, .gfx_preamble
    store.q r29, (sp)
    load.q  r1, 8(sp)
    store.q r1, 128(r29)               ; data[128] = task_id

    ; ===== M12.5: request CHIP and VRAM grants from hardware.resource =====
    ; Both SYS_MAP_IO calls below are now gated by the kernel grant table.
    ; Spin on FindPort until hardware.resource is up, then send two
    ; HWRES_MSG_REQUEST messages, then call SYS_MAP_IO twice.
.gfx_find_hwres:
    add     r1, r29, #256              ; r1 = "hardware.resource" string (data offset 256)
    syscall #SYS_FIND_PORT
    bnez    r2, .gfx_hwres_retry
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

    ; ===== CreatePort("graphics.library") =====
    add     r1, r29, #16
    move.l  r2, #PF_PUBLIC
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    bnez    r2, .gfx_halt
    store.q r1, 144(r29)               ; data[144] = port_id

    ; ===== Print banner via SYS_DEBUG_PUTCHAR =====
    add     r20, r29, #48              ; r20 = &data[48] = banner (M12: shifted from 32 after PORT_NAME_LEN bump)
.gfx_ban_loop:
    load.b  r1, (r20)
    beqz    r1, .gfx_ban_id
    store.q r20, 8(sp)
    syscall #SYS_DEBUG_PUTCHAR
    load.q  r29, (sp)
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .gfx_ban_loop
.gfx_ban_id:
    load.q  r29, (sp)
    load.q  r1, 128(r29)
    add     r1, r1, #0x30
    syscall #SYS_DEBUG_PUTCHAR
    move.l  r1, #0x5D                  ; ']'
    syscall #SYS_DEBUG_PUTCHAR
    move.l  r1, #0x0D
    syscall #SYS_DEBUG_PUTCHAR
    move.l  r1, #0x0A
    syscall #SYS_DEBUG_PUTCHAR

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
    ; offset 48: banner "graphics.library ONLINE [Task " + null + pad to 80
    dc.b    "graphics.library ONLINE [Task ", 0
    ds.b    1                           ; pad to offset 80
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
prog_gfxlib_data_end:
    align   8
prog_gfxlib_end:

; ---------------------------------------------------------------------------
; C/GfxDemo — minimal graphics.library client (M11)
; ---------------------------------------------------------------------------
; Opens graphics.library + input.device, allocates a 640x480 RGBA32 surface,
; registers it, fills with a solid color, presents once, then waits for
; Escape (scancode 0x01) and exits cleanly.
prog_gfxdemo:
    dc.l    IMG_MAGIC_LO, IMG_MAGIC_HI
    dc.l    prog_gfxdemo_code_end - prog_gfxdemo_code
    dc.l    prog_gfxdemo_data_end - prog_gfxdemo_data
    dc.l    0
    ds.b    12
prog_gfxdemo_code:

    ; ===== Preamble =====
    sub     sp, sp, #16
.gd_preamble:
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    store.q r1, 8(sp)
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    load.q  r28, 8(sp)
    bne     r1, r28, .gd_preamble
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
    bne     r29, r28, .gd_preamble
    store.q r29, (sp)
    load.q  r1, 8(sp)
    store.q r1, 128(r29)               ; data[128] = task_id

    ; ===== OpenLibrary("graphics.library") with retry =====
.gd_open_gfx:
    load.q  r29, (sp)
    add     r1, r29, #16               ; "graphics.library"
    move.l  r2, #0
    syscall #SYS_FIND_PORT  
    load.q  r29, (sp)
    beqz    r2, .gd_gfx_ok
    syscall #SYS_YIELD
    bra     .gd_open_gfx
.gd_gfx_ok:
    store.q r1, 136(r29)               ; data[136] = graphics_port

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

    move.q  r1, r0
    syscall #SYS_EXIT_TASK

.gd_halt:
    syscall #SYS_YIELD
    bra     .gd_halt
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
    ds.b    8                           ; 232: pad to 8-byte
prog_gfxdemo_data_end:
    align   8
prog_gfxdemo_end:

; ---------------------------------------------------------------------------
; intuition.library — single-window compositor + IDCMP delivery (M12)
; ---------------------------------------------------------------------------
; intuition.library is the sole graphics.library client. On the FIRST
; INTUITION_OPEN_WINDOW it lazily opens the display, allocates a fullscreen
; screen surface, registers it with graphics.library, and subscribes to
; input.device. From then on it composites the (single) window's backing
; surface into the screen surface and routes input as IDCMP-* messages to
; the window's idcmp_port. On CLOSE_WINDOW it tears down all of the above
; and returns to text mode.
;
; Protocol: see iexec.inc INTUITION_* / IDCMP_* constants.
; M12 ships single-window only — no z-order, no compositor overlap.
;
; Data layout:
;   0:    "console.handler\0"  (16) — convention slot, unused
;   16:   "intuition.library"  (16, exactly 16, NO null) — port name
;   32:   "graphics.library"   (16, exactly 16, NO null) — for FindPort
;   48:   "input.device", 0x00 (16) — for FindPort
;   64:   "intuition.library ONLINE [Task " + null (32) → ends at 96
;   96:   pad to 128 (32)
;   128:  task_id              (8)
;   136:  intuition_port       (8)  — public port
;   144:  graphics_port        (8)  — cached after first FindPort
;   152:  input_port           (8)  — cached after first FindPort
;   160:  reply_port           (8)  — anonymous, sync replies
;   168:  my_input_port        (8)  — anonymous, receives input events
;   176:  display_open         (1)  — 0 = text mode, 1 = graphics mode
;   177:  input_subscribed     (1)  — 1 if our INPUT_OPEN succeeded; close
;                                     skips INPUT_CLOSE if 0 so we don't
;                                     clobber another client's subscription
;   178..183: pad                  (6)
;   184:  display_handle       (8)  — graphics.library display handle
;   192:  surface_handle       (8)  — graphics.library surface handle
;   200:  screen_va            (8)  — own MEMF_PUBLIC screen buffer VA
;   208:  screen_share         (8)  — (4) own surface share handle + pad
;   216:  win_in_use           (8)  — (1) + pad (1=window open)
;   224:  win_x                (4)  — window origin x (signed)
;   228:  win_y                (4)  — window origin y (signed)
;   232:  win_w                (4)  — window width
;   236:  win_h                (4)  — window height
;   240:  win_share            (8)  — (4) app buffer share handle + pad
;   248:  win_mapped_va        (8)  — MapShared'd app buffer VA
;   256:  idcmp_port           (8)  — owner's IDCMP delivery port
;   264:  event_seq            (8)  — (4) monotonic seq + pad
;   272:  msg_type             (8)  — saved message fields (scratch)
;   280:  msg_data0            (8)
;   288:  msg_data1            (8)
;   296:  msg_reply            (8)
;   304:  msg_share            (8)
;   312:  pad                  (8)
prog_intuition_library:
    dc.l    IMG_MAGIC_LO, IMG_MAGIC_HI
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
    sub     sp, sp, #16
.intui_preamble:
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    store.q r1, 8(sp)
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    load.q  r28, 8(sp)
    bne     r1, r28, .intui_preamble
    move.l  r28, #USER_SLOT_STRIDE
    mulu    r28, r1, r28
    move.l  r29, #USER_CODE_BASE
    add     r29, r29, r28
    add     r29, r29, #0x3000          ; 2 code pages + 1 stack page = 3 pages
    store.q r29, (sp)
    load.q  r1, 8(sp)
    move.l  r28, #USER_SLOT_STRIDE
    mulu    r28, r1, r28
    move.l  r29, #USER_CODE_BASE
    add     r29, r29, r28
    add     r29, r29, #0x3000
    load.q  r28, (sp)
    bne     r29, r28, .intui_preamble
    store.q r29, (sp)
    load.q  r1, 8(sp)
    store.q r1, 128(r29)               ; data[128] = task_id

    ; ===== CreatePort("intuition.library") =====
    add     r1, r29, #320
    move.l  r2, #PF_PUBLIC
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    bnez    r2, .intui_halt
    store.q r1, 136(r29)               ; data[136] = intuition_port

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
    add     r20, r29, #416             ; r20 = &data[416] = banner
.intui_ban_loop:
    load.b  r1, (r20)
    beqz    r1, .intui_ban_id
    store.q r20, 8(sp)
    syscall #SYS_DEBUG_PUTCHAR
    load.q  r29, (sp)
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .intui_ban_loop
.intui_ban_id:
    load.q  r29, (sp)
    load.q  r1, 128(r29)
    add     r1, r1, #0x30
    syscall #SYS_DEBUG_PUTCHAR
    move.l  r1, #0x5D                  ; ']'
    syscall #SYS_DEBUG_PUTCHAR
    move.l  r1, #0x0D
    syscall #SYS_DEBUG_PUTCHAR
    move.l  r1, #0x0A
    syscall #SYS_DEBUG_PUTCHAR

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

    ; FindPort("graphics.library")
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
    bnez    r2, .intui_reply_nomem
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
    bnez    r2, .intui_reply_nomem
    load.q  r1, 160(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r3, .intui_reply_nomem
    bnez    r1, .intui_reply_nomem     ; r1 = err code from gfx
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
    bnez    r2, .intui_reply_nomem
    load.q  r1, 160(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r3, .intui_reply_nomem
    bnez    r1, .intui_reply_nomem
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
    bnez    r2, .intui_reply_nomem     ; kernel-level PutMsg failure
    load.q  r1, 160(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r3, .intui_reply_nomem     ; kernel-level WaitPort failure
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

    ; --- 5. FreeMem our own screen surface ---
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
.intui_reply_nomem:
    load.q  r1, 296(r29)
    move.l  r2, #INTUI_ERR_NOMEM
    move.q  r3, r0
    move.q  r4, r0
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
    ds.b    8                           ; 264: event_seq (4) + pad
    ds.b    8                           ; 272: msg_type
    ds.b    8                           ; 280: msg_data0
    ds.b    8                           ; 288: msg_data1
    ds.b    8                           ; 296: msg_reply
    ds.b    8                           ; 304: msg_share
    ds.b    8                           ; 312: pad
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
    ; offset 416: banner string "intuition.library ONLINE [Task" + null + pad
    dc.b    "intuition.library ONLINE [Task", 0
    ds.b    13                          ; pad to offset 460
    ; offset 460: window title text rendered by .intui_draw_string in the
    ; title bar block of the M12 decoration
    dc.b    "About IntuitionOS", 0
    ds.b    290                         ; pad to offset 768
    ; offset 768: embedded Topaz 8x16 bitmap font (256 glyphs x 16 bytes
    ; = 4096 bytes) — used by .intui_draw_char to render title text
    incbin  "topaz.raw"
prog_intui_data_end:
    align   8
prog_intui_end:

; ---------------------------------------------------------------------------
; About — intuition.library client with text rendering (M12)
; ---------------------------------------------------------------------------
; Allocates a 320x200 RGBA32 backing surface, opens an intuition.library
; window centered on the 800x600 screen, fills the content area with a
; teal backdrop, draws several lines of "About IntuitionOS" text using
; the embedded Topaz 8x16 bitmap font, sends DAMAGE, then waits on its
; IDCMP port. On IDCMP_CLOSEWINDOW (Esc key OR click on close gadget)
; it sends INTUITION_CLOSE_WINDOW and exits.
;
; Data layout:
;   0..127:  pad
;   128:  task_id              (8)
;   136:  intuition_port       (8)
;   144:  reply_port           (8)
;   152:  idcmp_port           (8)
;   160:  surface_va           (8)
;   168:  surface_share        (8) — (4) + pad
;   176:  window_handle        (8)
;   184:  pad
;   192:  "intuition.library"  (32, port name for FindPort)
;   224:  "About M12 ready"    (test marker)
;   256:  about text strings (each null-terminated)
;   ...
;   1024: topaz font (4096 bytes, full 256 chars × 16 bytes)
prog_about:
    dc.l    IMG_MAGIC_LO, IMG_MAGIC_HI
    dc.l    prog_about_code_end - prog_about_code
    dc.l    prog_about_data_end - prog_about_data
    dc.l    0
    ds.b    12
prog_about_code:
    sub     sp, sp, #16
.ab_preamble:
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    store.q r1, 8(sp)
    move.l  r1, #SYSINFO_CURRENT_TASK
    syscall #SYS_GET_SYS_INFO
    load.q  r28, 8(sp)
    bne     r1, r28, .ab_preamble
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
    bne     r29, r28, .ab_preamble
    store.q r29, (sp)
    load.q  r1, 8(sp)
    store.q r1, 128(r29)

    ; FindPort("intuition.library") with retry
.ab_findi:
    load.q  r29, (sp)
    add     r1, r29, #192
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    beqz    r2, .ab_findi_ok
    syscall #SYS_YIELD
    bra     .ab_findi
.ab_findi_ok:
    store.q r1, 136(r29)               ; intuition_port

    ; CreatePort(NULL) → reply_port
    move.q  r1, r0
    move.l  r2, #0
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    bnez    r2, .ab_halt
    store.q r1, 144(r29)               ; reply_port

    ; CreatePort(NULL) → idcmp_port
    move.q  r1, r0
    move.l  r2, #0
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    bnez    r2, .ab_halt
    store.q r1, 152(r29)               ; idcmp_port

    ; AllocMem(320*200*4 = 256000, MEMF_PUBLIC|MEMF_CLEAR)
    ; M12: window is 320x200 to fit nicely under the 16-pixel title bar
    ; with room for several lines of about text in the content area.
    move.l  r1, #256000
    move.l  r2, #0x10001
    syscall #SYS_ALLOC_MEM             ; R1=va R2=err R3=share
    load.q  r29, (sp)
    bnez    r2, .ab_halt
    store.q r1, 160(r29)               ; surface_va
    store.l r3, 168(r29)               ; surface_share

    ; Fill window backing buffer with the recessed-panel grey colour
    ; COL_PANEL_BG = 0xFFDCD8D0 (AmigaOS 3.9 / ReAction recessed panel).
    ; intuition.library overpaints the outer frame, blue title bar, and
    ; recessed-panel border on top, but the interior of the panel keeps
    ; these pixels — so the user sees a uniform recessed grey body with
    ; black Topaz text on top.
    load.q  r14, 160(r29)
    move.l  r15, #256000               ; bytes
    move.l  r16, #0xFFDCD8D0           ; COL_PANEL_BG
.ab_fill:
    beqz    r15, .ab_fill_done
    store.l r16, (r14)
    add     r14, r14, #4
    sub     r15, r15, #4
    bra     .ab_fill
.ab_fill_done:

    ; ===== Render about text into the window =====
    ; The window's content area starts below intuition.library's 16-pixel
    ; title bar plus the 1-pixel outer + 1-pixel inner frame, so visible
    ; window. The recessed content panel painted by intuition.library
    ; starts at window-local (8, 24) and extends to (312, 168). All text
    ; lives inside that panel area, indented 8 px (so x = 16) and spaced
    ; 18 px apart (16-px glyph height + 2 px leading).
    ;
    ; Text color = black (0xFF000000) on COL_PANEL_BG (0xFFDCD8D0).

    ; Line 1 (y=32): "About IntuitionOS"
    move.l  r10, #16                   ; x
    move.l  r11, #32                   ; y
    add     r12, r29, #256             ; r12 = string ptr (data offset 256)
    jsr     .ab_draw_string
    ; Line 2 (y=56): "Protected Exec-inspired kernel"
    move.l  r10, #16
    move.l  r11, #56
    add     r12, r29, #288             ; data offset 288
    jsr     .ab_draw_string
    ; Line 3 (y=80): "intuition.library demonstration"
    move.l  r10, #16
    move.l  r11, #80
    add     r12, r29, #320             ; data offset 320
    jsr     .ab_draw_string
    ; Line 4 (y=104): "All services run in user space"
    move.l  r10, #16
    move.l  r11, #104
    add     r12, r29, #352             ; data offset 352
    jsr     .ab_draw_string
    ; Line 5 (y=152): "Press Esc to close"
    move.l  r10, #16
    move.l  r11, #152
    add     r12, r29, #384             ; data offset 384
    jsr     .ab_draw_string

    ; Send INTUITION_OPEN_WINDOW
    ; data0 = (320<<48)|(200<<32)|(240<<16)|200  (w/h/x/y)
    ; M12: window centered on 800x600 screen at (240, 200), size 320x200.
    load.q  r1, 136(r29)               ; intuition_port
    move.l  r2, #INTUITION_OPEN_WINDOW
    move.q  r3, #320
    lsl     r3, r3, #48
    move.q  r14, #200
    lsl     r14, r14, #32
    or      r3, r3, r14
    move.q  r14, #240
    lsl     r14, r14, #16
    or      r3, r3, r14
    or      r3, r3, #200
    load.q  r4, 152(r29)               ; data1 = idcmp_port
    load.q  r5, 144(r29)               ; reply_port
    load.l  r6, 168(r29)               ; share = surface_share
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    bnez    r2, .ab_halt
    load.q  r1, 144(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r3, .ab_halt
    bnez    r1, .ab_halt               ; r1 = INTUI_ERR_*
    store.q r2, 176(r29)               ; window_handle

    ; Send INTUITION_DAMAGE (full window)
    load.q  r1, 136(r29)
    move.l  r2, #INTUITION_DAMAGE
    load.q  r3, 176(r29)
    move.q  r4, r0
    load.q  r5, 144(r29)
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    load.q  r1, 144(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)

    ; ----- IDCMP wait loop -----
.ab_idcmp:
    load.q  r29, (sp)
    load.q  r1, 152(r29)               ; idcmp_port
    syscall #SYS_WAIT_PORT             ; R1=type R2=data0 R3=err
    load.q  r29, (sp)
    bnez    r3, .ab_idcmp
    move.l  r28, #IDCMP_CLOSEWINDOW
    beq     r1, r28, .ab_close
    bra     .ab_idcmp                  ; ignore other classes for M12 demo

.ab_close:
    ; Send INTUITION_CLOSE_WINDOW
    load.q  r1, 136(r29)
    move.l  r2, #INTUITION_CLOSE_WINDOW
    load.q  r3, 176(r29)
    move.q  r4, r0
    load.q  r5, 144(r29)
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    load.q  r1, 144(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)

    move.q  r1, r0
    syscall #SYS_EXIT_TASK

.ab_halt:
    syscall #SYS_YIELD
    bra     .ab_halt

; ----------------------------------------------------------------
; .ab_draw_char — render a single 8x16 topaz glyph into the surface
; ----------------------------------------------------------------
; Inputs:
;   r10 = x (column in pixels, must be >= 0 and <= 312)
;   r11 = y (row in pixels, must be >= 0 and <= 184)
;   r3  = character byte (ASCII; lookup is `font[ch * 16 + row]`)
;   r29 = data base (so r29+1024 is font[0])
; Output: nothing
; Clobbers: r4..r9, r14..r19
; Preserves: r10, r11, r29
; ----------------------------------------------------------------
.ab_draw_char:
    ; Load surface_va, compute pixel base for this glyph cell:
    ;   base = surface_va + y*1280 + x*4    (320-stride = 320*4 = 1280)
    load.q  r4, 160(r29)               ; surface_va
    move.l  r14, #1280
    mulu    r14, r11, r14
    add     r4, r4, r14
    lsl     r14, r10, #2
    add     r4, r4, r14                ; r4 = top-left pixel of cell
    ; Compute glyph pointer: r5 = font_base + ch * 16
    add     r5, r29, #1024             ; font_base
    lsl     r6, r3, #4                 ; ch * 16
    add     r5, r5, r6                 ; r5 = &font[ch][0]
    ; Loop 16 rows
    move.l  r6, #0                     ; row index 0..15
.ab_dc_row:
    move.l  r14, #16
    bge     r6, r14, .ab_dc_done
    load.b  r7, (r5)                   ; r7 = glyph row bits
    add     r5, r5, #1
    ; Render 8 pixels left-to-right
    move.q  r8, r4                     ; pixel cursor
    move.l  r9, #0                     ; col index 0..7
.ab_dc_col:
    move.l  r14, #8
    bge     r9, r14, .ab_dc_col_done
    ; bit = (r7 >> (7 - col)) & 1
    move.l  r14, #7
    sub     r14, r14, r9
    lsr     r15, r7, r14
    and     r15, r15, #1
    beqz    r15, .ab_dc_skip
    ; M12 redesign: black text on grey panel (was white text on teal).
    ; COL_TEXT = 0xFF000000 (RGBA bytes 00,00,00,FF).
    move.l  r16, #0xFF000000
    store.l r16, (r8)
.ab_dc_skip:
    add     r8, r8, #4
    add     r9, r9, #1
    bra     .ab_dc_col
.ab_dc_col_done:
    add     r4, r4, #1280              ; next surface row
    add     r6, r6, #1
    bra     .ab_dc_row
.ab_dc_done:
    rts

; ----------------------------------------------------------------
; .ab_draw_string — render a null-terminated string at (x, y)
; ----------------------------------------------------------------
; Inputs:
;   r10 = x (initial column)
;   r11 = y (row)
;   r12 = string pointer
;   r29 = data base
; Output: r10 advanced past the last drawn glyph
; Clobbers: r3..r9, r14..r19
; ----------------------------------------------------------------
.ab_draw_string:
    sub     sp, sp, #24
    store.q r10, (sp)                  ; save x
    store.q r11, 8(sp)                 ; save y
    store.q r12, 16(sp)                ; save str ptr
.ab_ds_loop:
    load.q  r12, 16(sp)
    load.b  r3, (r12)
    beqz    r3, .ab_ds_done
    ; Draw the char
    load.q  r10, (sp)
    load.q  r11, 8(sp)
    jsr     .ab_draw_char
    ; Advance x by glyph width (8) and string ptr
    load.q  r10, (sp)
    add     r10, r10, #8
    store.q r10, (sp)
    load.q  r12, 16(sp)
    add     r12, r12, #1
    store.q r12, 16(sp)
    bra     .ab_ds_loop
.ab_ds_done:
    add     sp, sp, #24
    rts
prog_about_code_end:

prog_about_data:
    ; offsets 0..127: convention/scratch, unused. Strings live at 192+ to keep
    ; field offsets stable.
    ds.b    128                          ; pad 0..128
    ds.b    8                            ; 128: task_id
    ds.b    8                            ; 136: intuition_port
    ds.b    8                            ; 144: reply_port
    ds.b    8                            ; 152: idcmp_port
    ds.b    8                            ; 160: surface_va
    ds.b    8                            ; 168: surface_share
    ds.b    8                            ; 176: window_handle
    ds.b    8                            ; 184: pad
    ; offset 192: "intuition.library" + pad to 32 (port name for FindPort)
    dc.b    "intuition.library", 0
    ds.b    14
    ; offset 224: "About M12 ready" + pad to 32 (test marker)
    dc.b    "About M12 ready", 0
    ds.b    16
    ; offset 256: line 1 — "About IntuitionOS" + pad to 32
    dc.b    "About IntuitionOS", 0
    ds.b    14
    ; offset 288: line 2 — "Protected Exec-inspired kernel" + pad to 32
    dc.b    "Protected Exec-inspired kernel", 0
    ds.b    1
    ; offset 320: line 3 — "intuition.library demonstration" + pad to 32
    dc.b    "intuition.library demonstration", 0
    ; offset 352: line 4 — "All visible services run in user space" — too long
    ; for a 32-byte slot; truncate to fit. Use a 64-byte slot here.
    dc.b    "All services run in user space", 0
    ds.b    1
    ; offset 384: line 5 — "Press Esc to close" + pad to 32
    dc.b    "Press Esc to close", 0
    ds.b    13
    ; offset 416: pad to 1024 (font lives at 1024 for round offsets)
    ds.b    608
    ; offset 1024: embedded Topaz 8x16 font (256 glyphs × 16 bytes = 4096 bytes)
    incbin  "topaz.raw"
prog_about_data_end:
    align   8
prog_about_end:

prog_doslib_data_end:
    align   8
prog_doslib_end:

; ---------------------------------------------------------------------------
; SHELL — interactive command shell (M10)
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
    syscall #SYS_FIND_PORT  
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
    syscall #SYS_FIND_PORT  
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

    ; (Stale "IntuitionOS M11" banner removed: incorrect milestone label
    ;  and redundant — exec.library already prints the canonical version
    ;  banner. The data slot at offset 56 is preserved as padding to keep
    ;  the offsets of subsequent strings stable.)

    ; =====================================================================
    ; M10: Try to open and read S:Startup-Sequence
    ; =====================================================================
    load.q  r29, (sp)
    ; Initialize script_mode = 0, script_pos = 0, script_len = 0
    store.b r0, 176(r29)
    store.l r0, 184(r29)
    store.l r0, 188(r29)

    ; Write "S:Startup-Sequence" to shared buffer
    load.q  r14, 160(r29)             ; r14 = shared_buf_va
    add     r15, r29, #320             ; r15 = "S:Startup-Sequence" string source
    ; Inline string copy of "S:Startup-Sequence\0"
    move.l  r16, #0x53                 ; 'S'
    store.b r16, (r14)
    move.l  r16, #0x3A                 ; ':'
    store.b r16, 1(r14)
    move.l  r16, #0x53                 ; 'S'
    store.b r16, 2(r14)
    move.l  r16, #0x74                 ; 't'
    store.b r16, 3(r14)
    move.l  r16, #0x61                 ; 'a'
    store.b r16, 4(r14)
    move.l  r16, #0x72                 ; 'r'
    store.b r16, 5(r14)
    move.l  r16, #0x74                 ; 't'
    store.b r16, 6(r14)
    move.l  r16, #0x75                 ; 'u'
    store.b r16, 7(r14)
    move.l  r16, #0x70                 ; 'p'
    store.b r16, 8(r14)
    move.l  r16, #0x2D                 ; '-'
    store.b r16, 9(r14)
    move.l  r16, #0x53                 ; 'S'
    store.b r16, 10(r14)
    move.l  r16, #0x65                 ; 'e'
    store.b r16, 11(r14)
    move.l  r16, #0x71                 ; 'q'
    store.b r16, 12(r14)
    move.l  r16, #0x75                 ; 'u'
    store.b r16, 13(r14)
    move.l  r16, #0x65                 ; 'e'
    store.b r16, 14(r14)
    move.l  r16, #0x6E                 ; 'n'
    store.b r16, 15(r14)
    move.l  r16, #0x63                 ; 'c'
    store.b r16, 16(r14)
    move.l  r16, #0x65                 ; 'e'
    store.b r16, 17(r14)
    store.b r0, 18(r14)                ; null

    ; Send DOS_OPEN(mode=READ=0)
    load.q  r29, (sp)
    move.l  r2, #DOS_OPEN              ; type
    move.q  r3, r0                      ; data0 = mode (READ=0)
    move.q  r4, r0                      ; data1 = 0
    load.q  r5, 152(r29)               ; reply_port
    load.l  r6, 168(r29)               ; share_handle
    load.q  r1, 144(r29)               ; dos_port
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)

    ; WaitPort for DOS_OPEN reply
    load.q  r1, 152(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    ; R1 = type (0=OK, nonzero=error), R2 = data0 (file handle if OK)
    bnez    r1, .sh_no_script           ; not found → skip startup
    move.q  r17, r2                     ; r17 = file handle (save)
    store.q r17, 8(sp)                 ; save handle to scratch

    ; Send DOS_READ(handle, max=512). The script buffer at data[368] is
    ; 512 bytes (M12.8: bumped from 256 to absorb the new boot ECHO line
    ; without truncating earlier startup commands). The shell's share is
    ; 4 KiB so the share clamp doesn't kick in.
    load.q  r29, (sp)
    move.l  r2, #DOS_READ              ; type
    load.q  r3, 8(sp)                  ; data0 = handle
    move.l  r4, #512                   ; data1 = max bytes
    load.q  r5, 152(r29)
    load.l  r6, 168(r29)
    load.q  r1, 144(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)

    ; WaitPort for DOS_READ reply
    load.q  r1, 152(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    ; R1=type, R2=data0=bytes_read
    bnez    r1, .sh_close_script       ; read error → close + skip
    move.q  r18, r2                    ; r18 = bytes_read
    store.l r18, 188(r29)              ; save script_len

    ; Copy script content from shared_buf_va to data[368] (script buffer)
    load.q  r14, 160(r29)             ; src
    add     r15, r29, #368             ; dst
    move.l  r16, #0
.sh_copy_script:
    bge     r16, r18, .sh_copy_script_done
    add     r19, r14, r16
    load.b  r20, (r19)
    add     r19, r15, r16
    store.b r20, (r19)
    add     r16, r16, #1
    bra     .sh_copy_script
.sh_copy_script_done:
    ; Set script_mode = 1
    move.l  r14, #1
    store.b r14, 176(r29)

.sh_close_script:
    ; Send DOS_CLOSE(handle)
    load.q  r29, (sp)
    move.l  r2, #DOS_CLOSE
    load.q  r3, 8(sp)                  ; handle
    move.q  r4, r0
    load.q  r5, 152(r29)
    load.l  r6, 168(r29)
    load.q  r1, 144(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    load.q  r1, 152(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)

.sh_no_script:

    ; =====================================================================
    ; Script line reader: read next line from script buffer to line buffer
    ; (Defined before main loop so backward branch resolves)
    ; =====================================================================
.sh_script_line:
    load.q  r29, (sp)
    ; r14 = script_pos, r15 = script_len
    load.l  r14, 184(r29)
    load.l  r15, 188(r29)
    ; If script_pos >= script_len, end of script
    bge     r14, r15, .sh_script_done
    ; Copy bytes from script_buffer[script_pos] until '\n' or end
    add     r16, r29, #368             ; r16 = script buffer base
    add     r16, r16, r14              ; r16 = current read ptr
    add     r17, r29, #240             ; r17 = line buffer (dest)
    move.l  r18, #0                    ; chars copied
.sh_sl_copy:
    bge     r14, r15, .sh_sl_eol       ; reached script end
    move.l  r19, #126                  ; max line length
    bge     r18, r19, .sh_sl_eol
    load.b  r20, (r16)
    move.l  r19, #0x0A                 ; '\n'
    beq     r20, r19, .sh_sl_eol_inc
    store.b r20, (r17)
    add     r16, r16, #1
    add     r17, r17, #1
    add     r18, r18, #1
    add     r14, r14, #1
    bra     .sh_sl_copy
.sh_sl_eol_inc:
    add     r14, r14, #1               ; consume the '\n'
.sh_sl_eol:
    store.b r0, (r17)                  ; null-terminate line
    store.l r14, 184(r29)              ; save updated script_pos
    bra     .sh_line_ready

.sh_script_done:
    ; End of script: clear script_mode, fall through to interactive
    store.b r0, 176(r29)
    bra     .sh_main_loop

    ; =====================================================================
    ; Main loop
    ; =====================================================================
.sh_main_loop:
    load.q  r29, (sp)
    ; Check script mode
    load.b  r14, 176(r29)
    bnez    r14, .sh_script_line

    ; --- Interactive mode: print prompt + read line via console ---
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
.sh_line_ready:

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
    ; M10: Name-based command dispatch via DOS_RUN
    ; =====================================================================
    ; Write "command_name\0args\0" to shared buffer, send DOS_RUN.
    ; r15 = line start, r16 = end of first word, r17 = word length.
    ; If word length == 0, skip (empty line handled above).

    ; 1. Copy command name to shared buffer at offset 0
    load.q  r14, 160(r29)             ; r14 = shared_buf_va
    move.q  r18, r15                   ; src = line start
    move.q  r19, r14                   ; dst = shared buf
    move.l  r20, #0
.sh_cp_cmd:
    bge     r20, r17, .sh_cp_cmd_done
    load.b  r21, (r18)
    store.b r21, (r19)
    add     r18, r18, #1
    add     r19, r19, #1
    add     r20, r20, #1
    bra     .sh_cp_cmd
.sh_cp_cmd_done:
    store.b r0, (r19)                  ; null-terminate command name
    add     r19, r19, #1               ; advance past null

    ; 2. Copy args (everything after first word + space) after null
    ; r16 = end of word (space or null)
    load.b  r21, (r16)
    beqz    r21, .sh_no_args
    add     r16, r16, #1               ; skip space
.sh_no_args:
    move.l  r20, #0
.sh_cp_args:
    load.b  r21, (r16)
    store.b r21, (r19)
    beqz    r21, .sh_args_done
    add     r16, r16, #1
    add     r19, r19, #1
    add     r20, r20, #1
    move.l  r22, #DATA_ARGS_MAX
    blt     r20, r22, .sh_cp_args
    store.b r0, (r19)
.sh_args_done:

    ; 3. Send DOS_RUN to dos.library
    load.q  r29, (sp)
    move.l  r2, #DOS_RUN               ; type = DOS_RUN
    move.q  r3, r0                      ; data0 = 0 (unused in M10)
    move.q  r4, r0                      ; data1 = 0
    load.q  r5, 152(r29)               ; reply_port
    load.l  r6, 168(r29)               ; share_handle
    load.q  r1, 144(r29)               ; dos_port
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)

    ; 4. WaitPort(reply_port) for dos.library response
    load.q  r1, 152(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)

    ; 5. Check response: R1=type (error code), if NOTFOUND → "Unknown command"
    move.l  r14, #DOS_ERR_NOTFOUND
    bne     r1, r14, .sh_cmd_ok

    ; Unknown command
    load.q  r29, (sp)
    add     r20, r29, #88             ; "Unknown command\r\n"
    jsr     .sh_send_string
    bra     .sh_main_loop

.sh_cmd_ok:
    ; Command launched — yield 200 times to let it finish
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
    ; --- Offset 56: dead 24-byte slot (formerly "IntuitionOS M11\r\n\0",
    ;     printed by a stale shell banner removed in M12.8 Phase 1).
    ;     Kept as padding to preserve the offsets of the strings below. ---
    ds.b    24                          ; pad to offset 80
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
    ; --- Offset 176: script_mode (1 byte) + padding ---
    ds.b    8
    ; --- Offset 184: script_pos (4 bytes) + script_len (4 bytes) ---
    ds.b    8
    ; --- Offset 192: (M10: command table removed, space available) ---
    ds.b    48                          ; pad to offset 240
    ; --- Offset 240: line buffer (128 bytes) ---
    ds.b    128
    ; --- Offset 368: script buffer (512 bytes for Startup-Sequence —
    ;     M12.8: bumped from 256 to absorb new boot ECHO line) ---
    ds.b    512
prog_shell_data_end:
    align   8
prog_shell_end:

; ============================================================================
; Data: Strings
; ============================================================================

boot_banner:
    dc.b    "exec.library M11 boot", 0x0D, 0x0A, 0
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
