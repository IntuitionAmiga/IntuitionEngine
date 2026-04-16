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
    ; 3. Build kernel page table with explicit permission classes.
    ;    Start from supervisor R/W for the low kernel-mapped window, then
    ;    tighten the immutable kernel image pages to supervisor R/X.
    ; ---------------------------------------------------------------
    move.l  r2, #KERN_PAGE_TABLE
    move.l  r4, #0
.kern_pt_loop:
    lsl     r3, r4, #13
    or      r3, r3, #(PTE_P | PTE_R | PTE_W)
    lsl     r5, r4, #3
    add     r5, r5, r2
    store.q r3, (r5)
    add     r4, r4, #1
    move.l  r6, #KERN_PAGES
    blt     r4, r6, .kern_pt_loop

    ; Tighten the assembled kernel image below KERN_PAGE_TABLE to supervisor
    ; R/X. This covers kernel text, rodata, and immutable embedded assets.
    move.l  r2, #KERN_PAGE_TABLE
    move.l  r4, #0
.kern_image_rx_map:
    move.l  r6, #KERN_IMAGE_PAGE_END
    bge     r4, r6, .kern_image_rx_done
    lsl     r3, r4, #13
    or      r3, r3, #(PTE_P | PTE_R | PTE_X)
    lsl     r5, r4, #3
    add     r5, r5, r2
    store.q r3, (r5)
    add     r4, r4, #1
    bra     .kern_image_rx_map
.kern_image_rx_done:

    ; 3b. Add supervisor-only mappings for the entire dynamic task-image
    ; window (USER_IMAGE_BASE..USER_PT_BASE). M13 phase 2 no longer derives
    ; code/stack/data placement from task_id * USER_SLOT_STRIDE, so the
    ; kernel maps the whole image window once instead of per-slot ranges.
    move.l  r4, #USER_CODE_VPN_BASE
    move.l  r6, #USER_PT_PAGE_BASE
.kern_user_map:
    bge     r4, r6, .kern_user_map_done
    lsl     r3, r4, #13
    or      r3, r3, #(PTE_P | PTE_R | PTE_W)
    lsl     r5, r4, #3
    add     r5, r5, r2
    store.q r3, (r5)
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
    or      r3, r3, #(PTE_P | PTE_R | PTE_W)
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

    ; --- Init all task slots as FREE ---
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
    move.l  r1, #TASK_PUBID_FREE
    store.l r1, KD_DOSLIB_PUBID(r12)
    move.l  r2, #KERN_DATA_BASE
    move.l  r4, #HWRES_TASK_FREE
    store.l r4, KD_HWRES_TASK(r2)       ; broker = unclaimed sentinel
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

    ; M13 phase 2: zero per-task dynamic layout rows and the image/PT
    ; allocation bitmaps. Task placement is now allocator-backed within the
    ; reserved image/PT regions, not derived from task_id * USER_SLOT_STRIDE.
    move.l  r2, #KERN_DATA_BASE
    add     r2, r2, #KD_TASK_LAYOUT_BASE
    move.l  r3, #(MAX_TASKS * KD_TASK_LAYOUT_STRIDE / 8)
    move.l  r4, #0
.zero_task_layout:
    store.q r0, (r2)
    add     r2, r2, #8
    add     r4, r4, #1
    blt     r4, r3, .zero_task_layout

    move.l  r2, #KERN_DATA_BASE
    add     r2, r2, #KD_TASK_IMG_BITMAP
    move.l  r3, #(KD_TASK_IMG_BITMAP_SZ / 8)
    move.l  r4, #0
.zero_taskimg_bitmap:
    store.q r0, (r2)
    add     r2, r2, #8
    add     r4, r4, #1
    blt     r4, r3, .zero_taskimg_bitmap

    move.l  r2, #KERN_DATA_BASE
    add     r2, r2, #KD_TASK_PT_BITMAP
    move.l  r3, #(KD_TASK_PT_BITMAP_SZ / 8)
    move.l  r4, #0
.zero_taskpt_bitmap:
    store.q r0, (r2)
    add     r2, r2, #8
    add     r4, r4, #1
    blt     r4, r3, .zero_taskpt_bitmap

    ; M13 phase 3: initialize slot->public-task-id array to the free
    ; sentinel and reset the monotonic public task-id counter.
    move.l  r2, #KERN_DATA_BASE
    add     r2, r2, #KD_TASK_PUBID_BASE
    move.l  r3, #MAX_TASKS
    move.l  r4, #0
    move.l  r5, #0xFFFFFFFF
.zero_task_pubids:
    bge     r4, r3, .zero_taskid_next
    store.l r5, (r2)
    add     r2, r2, #KD_TASK_PUBID_STRIDE
    add     r4, r4, #1
    bra     .zero_task_pubids
.zero_taskid_next:
    move.l  r2, #KERN_DATA_BASE
    store.q r0, KD_TASKID_NEXT(r2)

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
    or      r3, r3, #(PTE_P | PTE_R | PTE_W)
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
    move.l  r8, #boot_banner
    jsr     kern_puts

    ; ---------------------------------------------------------------
    ; 9. M14.1/M15.2: prepare and launch the bootstrap manifest.
    ;    The kernel boots only the minimum pre-DOS chain here
    ;    (console.handler + host-backed dos.library). dos.library takes over
    ;    the remaining service launches after DOS is online.
    ; ---------------------------------------------------------------
    jsr     kern_boot_manifest_prepare  ; R2 = err
    bnez    r2, .boot_load_fail

    move.l  r29, #0
.boot_manifest_loop:
    move.l  r11, #KD_BOOT_MANIFEST_BOOT_COUNT
    bge     r29, r11, .boot_manifest_done
    move.l  r30, #KERN_DATA_BASE
    add     r30, r30, #KD_BOOT_MANIFEST_BASE
    move.l  r11, #KD_BOOT_MANIFEST_STRIDE
    mulu    r11, r29, r11
    add     r30, r30, r11              ; r30 = manifest row
    load.l  r11, KD_BOOT_MANIFEST_ID(r30)
    move.l  r12, #0xFFFFFFFF
    beq     r11, r12, .boot_manifest_done
    move.l  r12, #BOOT_MANIFEST_ID_DOSLIB
    bne     r11, r12, .boot_manifest_regular
    move.l  r1, #TASK_STARTUP_FLAG_BOOT
    push    r29
    push    r30
    jsr     kern_boot_load_host_doslib ; R1=task_id R2=err R3=slot
    pop     r30
    pop     r29
    bnez    r2, .boot_load_fail
    bra     .boot_manifest_launched
.boot_manifest_regular:
    load.q  r1, KD_BOOT_MANIFEST_PTR(r30)
    beqz    r1, .boot_load_fail        ; staged ELF must exist after prepare
    load.q  r2, KD_BOOT_MANIFEST_SIZE(r30)
    beqz    r2, .boot_load_fail
    move.l  r3, #TASK_STARTUP_FLAG_BOOT
    push    r29
    push    r30
    jsr     boot_load_elf_image        ; R1=task_id R2=err R3=slot
    pop     r30
    pop     r29
    bnez    r2, .boot_load_fail
.boot_manifest_launched:
    move.q  r18, r3                    ; preserve child slot across grant/export helpers
    load.l  r11, KD_BOOT_MANIFEST_ID(r30)
    move.l  r12, #BOOT_MANIFEST_ID_DOSLIB
    bne     r11, r12, .boot_manifest_after_launch
    move.l  r12, #KERN_DATA_BASE
    store.l r1, KD_DOSLIB_PUBID(r12)
    push    r29
    push    r30
    move.q  r1, r18                    ; dos.library child slot
    jsr     kern_export_boot_manifest_to_dos
    pop     r30
    pop     r29
    bnez    r2, .boot_load_fail
.boot_manifest_after_launch:
    push    r29
    push    r30
    move.q  r2, r1                     ; task_id
    load.l  r1, KD_BOOT_MANIFEST_ID(r30)
    jsr     kern_bootstrap_grant_for_program ; manifest-keyed in M14.1 phase 2
    pop     r30
    pop     r29
    bnez    r2, .boot_load_fail
.boot_manifest_next:
    add     r29, r29, #1
    bra     .boot_manifest_loop
.boot_manifest_done:
    bra     .boot_load_done
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
    push    r20
    move.q  r20, r8
.puts_loop:
    load.b  r8, (r20)
    beqz    r8, .puts_done
    jsr     kern_put_char
    add     r20, r20, #1
    bra     .puts_loop
.puts_done:
    pop     r20
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
    ; Code pages: P|R|X|U
    move.l  r4, #0
.bup_code_loop:
    bge     r4, r9, .bup_code_done
    add     r6, r8, r4                  ; VPN = base + i
    lsl     r3, r6, #13
    or      r3, r3, #(PTE_P | PTE_R | PTE_X | PTE_U)
    lsl     r5, r6, #3
    add     r5, r5, r7
    store.q r3, (r5)
    add     r4, r4, #1
    bra     .bup_code_loop
.bup_code_done:
    ; Stack page (VPN = base + code_pages): P|R|W|U
    add     r6, r8, r9                  ; VPN = base + code_pages
    lsl     r3, r6, #13
    or      r3, r3, #(PTE_P | PTE_R | PTE_W | PTE_U)
    lsl     r5, r6, #3
    add     r5, r5, r7
    store.q r3, (r5)
    ; Data pages: P|R|W|U
    add     r6, r8, r9
    add     r6, r6, #1                  ; VPN = base + code_pages + 1
    move.l  r4, #0
.bup_data_loop:
    bge     r4, r11, .bup_data_done
    add     r2, r6, r4                  ; VPN = data_base_vpn + i
    lsl     r3, r2, #13
    or      r3, r3, #(PTE_P | PTE_R | PTE_W | PTE_U)
    lsl     r5, r2, #3
    add     r5, r5, r7
    store.q r3, (r5)
    add     r4, r4, #1
    bra     .bup_data_loop
.bup_data_done:
    rts

; write_startup_block: populate the M13 startup block in its dedicated page.
; Input:
;   R1 = startup_base
;   R2 = task_id
;   R3 = flags
;   R4 = code_base
;   R5 = code_pages
;   R6 = stack_base
;   R7 = stack_pages
;   R8 = data_base
;   R9 = data_pages
; Clobbers: R10-R12
write_startup_block:
    store.q r0, (r1)
    store.q r0, 8(r1)
    store.q r0, 16(r1)
    store.q r0, 24(r1)
    store.q r0, 32(r1)
    store.q r0, 40(r1)
    store.q r0, 48(r1)
    store.q r0, 56(r1)
    move.l  r10, #TASK_STARTUP_VERSION
    store.l r10, TASKSB_VERSION(r1)
    move.l  r10, #TASK_STARTUP_SIZE
    store.l r10, TASKSB_SIZE(r1)
    store.l r2, TASKSB_TASK_ID(r1)
    store.l r3, TASKSB_FLAGS(r1)
    store.q r4, TASKSB_CODE_BASE(r1)
    store.l r5, TASKSB_CODE_PAGES(r1)
    store.q r8, TASKSB_DATA_BASE(r1)
    store.l r9, TASKSB_DATA_PAGES(r1)
    store.q r6, TASKSB_STACK_BASE(r1)
    store.l r7, TASKSB_STACK_PAGES(r1)
    rts

; kern_task_layout_addr: return pointer to a task's dynamic layout row.
; Input:  R1 = task_id
; Output: R1 = &layout[task_id]
kern_task_layout_addr:
    move.l  r3, #KERN_DATA_BASE
    add     r3, r3, #KD_TASK_LAYOUT_BASE
    move.l  r4, #KD_TASK_LAYOUT_STRIDE
    mulu    r4, r1, r4
    add     r1, r3, r4
    rts

; kern_task_pubid_addr: return pointer to a slot's public-task-id word.
; Input:  R1 = task slot
; Output: R1 = &pubid[slot]
kern_task_pubid_addr:
    move.l  r3, #KERN_DATA_BASE
    add     r3, r3, #KD_TASK_PUBID_BASE
    move.l  r4, #KD_TASK_PUBID_STRIDE
    mulu    r4, r1, r4
    add     r1, r3, r4
    rts

; kern_current_public_task_id: return the current task's public ID.
; Output: R1 = public task ID
kern_current_public_task_id:
    move.l  r3, #KERN_DATA_BASE
    load.q  r1, KD_CURRENT_TASK(r3)     ; current slot
    jsr     kern_task_pubid_addr
    load.l  r1, (r1)
    rts

; kern_find_slot_for_public_id: resolve a public task ID to a live slot.
; Input:  R1 = public task ID
; Output: R1 = slot index if found, 0 otherwise
;         R2 = 1 if found, 0 if not found
; Clobbers: R3-R8
kern_find_slot_for_public_id:
    move.q  r6, r1                      ; target public id
    move.l  r3, #0
.kfspid_scan:
    move.l  r4, #MAX_TASKS
    bge     r3, r4, .kfspid_notfound
    move.l  r4, #KERN_DATA_BASE
    lsl     r5, r3, #5
    add     r5, r5, #KD_TASK_BASE
    add     r5, r5, r4
    load.b  r7, KD_TASK_STATE(r5)
    move.l  r8, #TASK_FREE
    beq     r7, r8, .kfspid_next
    push    r3
    move.q  r1, r3
    jsr     kern_task_pubid_addr
    load.l  r7, (r1)
    pop     r3
    beq     r7, r6, .kfspid_found
.kfspid_next:
    add     r3, r3, #1
    bra     .kfspid_scan
.kfspid_found:
    move.q  r1, r3
    move.q  r2, #1
    rts
.kfspid_notfound:
    move.q  r1, #0
    move.q  r2, #0
    rts

; alloc_task_image_pages: allocate N contiguous pages from USER_IMAGE_BASE..USER_IMAGE_END.
; Input:  R1 = pages requested
; Output: R1 = base VA, R2 = ERR_OK / ERR_NOMEM
alloc_task_image_pages:
    move.l  r3, #KERN_DATA_BASE
    add     r3, r3, #KD_TASK_IMG_BITMAP
    move.l  r4, #0
    move.l  r5, #USER_IMAGE_PAGES
.atip_scan:
    bge     r4, r5, .atip_fail
    move.q  r6, r4
    move.q  r7, r4
    add     r8, r4, r1
    bgt     r8, r5, .atip_fail
.atip_check_bit:
    bge     r7, r8, .atip_found
    lsr     r9, r7, #3
    add     r9, r9, r3
    load.b  r9, (r9)
    and     r10, r7, #7
    lsr     r9, r9, r10
    and     r9, r9, #1
    bnez    r9, .atip_next
    add     r7, r7, #1
    bra     .atip_check_bit
.atip_next:
    add     r4, r7, #1
    bra     .atip_scan
.atip_found:
    move.q  r7, r6
.atip_mark:
    bge     r7, r8, .atip_ret
    lsr     r9, r7, #3
    add     r9, r9, r3
    load.b  r10, (r9)
    and     r11, r7, #7
    move.q  r12, #1
    lsl     r12, r12, r11
    or      r10, r10, r12
    store.b r10, (r9)
    add     r7, r7, #1
    bra     .atip_mark
.atip_ret:
    lsl     r1, r6, #12
    add     r1, r1, #USER_IMAGE_BASE
    move.q  r13, r1
    sub     r15, r8, r6
    lsl     r15, r15, #12
    add     r15, r15, r13
.atip_zero:
    bge     r13, r15, .atip_ok
    store.q r0, (r13)
    add     r13, r13, #8
    bra     .atip_zero
.atip_ok:
    move.q  r2, #ERR_OK
    rts
.atip_fail:
    ; M13 phase 4: once the legacy fixed image window is exhausted, spill
    ; task image pages into allocator-pool pages. The pool is identity-mapped
    ; in the kernel PT, so callers can still zero/copy them directly before
    ; build_user_pt_dynamic installs user-visible mappings for the chosen VAs.
    move.q  r14, r1
    jsr     alloc_pages                 ; R1 = base PPN, R2 = ERR_*
    bnez    r2, .atip_fail_nomem
    lsl     r1, r1, #12
    move.q  r13, r1
    move.q  r15, r1
    lsl     r14, r14, #12
    add     r15, r15, r14
.atip_zero_pool:
    bge     r13, r15, .atip_ok
    store.q r0, (r13)
    add     r13, r13, #8
    bra     .atip_zero_pool
.atip_fail_nomem:
    move.q  r1, r0
    move.q  r2, #ERR_NOMEM
    rts

; alloc_task_image_pages_exact: reserve an exact VA range inside the fixed
; USER_IMAGE window. Used by M14 descriptor launches so child mappings honor
; ELF p_vaddr values exactly instead of being relocated to arbitrary holes.
; Input:  R1 = base VA, R2 = pages
; Output: R1 = base VA, R2 = ERR_OK / ERR_BADARG / ERR_NOMEM
alloc_task_image_pages_exact:
    beqz    r2, .atipe_badarg
    move.l  r3, #0xFFF
    and     r4, r1, r3
    bnez    r4, .atipe_badarg
    move.l  r3, #USER_IMAGE_BASE
    blt     r1, r3, .atipe_badarg
    move.l  r3, #USER_IMAGE_END
    bge     r1, r3, .atipe_badarg
    move.q  r4, r1
    sub     r4, r4, #USER_IMAGE_BASE
    lsr     r4, r4, #12                ; first bit
    add     r5, r4, r2                 ; end bit
    move.l  r6, #USER_IMAGE_PAGES
    bgt     r5, r6, .atipe_badarg

    move.l  r3, #KERN_DATA_BASE
    add     r3, r3, #KD_TASK_IMG_BITMAP
    move.q  r6, r4
.atipe_check:
    bge     r6, r5, .atipe_mark
    lsr     r7, r6, #3
    add     r7, r7, r3
    load.b  r8, (r7)
    and     r9, r6, #7
    lsr     r8, r8, r9
    and     r8, r8, #1
    bnez    r8, .atipe_nomem
    add     r6, r6, #1
    bra     .atipe_check
.atipe_mark:
    move.q  r6, r4
.atipe_mark_loop:
    bge     r6, r5, .atipe_zero
    lsr     r7, r6, #3
    add     r7, r7, r3
    load.b  r8, (r7)
    and     r9, r6, #7
    move.q  r10, #1
    lsl     r10, r10, r9
    or      r8, r8, r10
    store.b r8, (r7)
    add     r6, r6, #1
    bra     .atipe_mark_loop
.atipe_zero:
    move.q  r6, r1
    move.q  r7, r2
    lsl     r7, r7, #12
    add     r7, r7, r6
.atipe_zero_loop:
    bge     r6, r7, .atipe_ok
    store.q r0, (r6)
    add     r6, r6, #8
    bra     .atipe_zero_loop
.atipe_ok:
    move.q  r2, #ERR_OK
    rts
.atipe_nomem:
    move.q  r1, r0
    move.q  r2, #ERR_NOMEM
    rts
.atipe_badarg:
    move.q  r1, r0
    move.q  r2, #ERR_BADARG
    rts

; free_task_image_pages: release pages back to the USER_IMAGE bitmap.
; Input: R1 = base VA, R2 = pages
free_task_image_pages:
    beqz    r1, .ftip_done
    beqz    r2, .ftip_done
    move.l  r3, #USER_IMAGE_BASE
    blt     r1, r3, .ftip_pool
    move.l  r3, #USER_IMAGE_END
    blt     r1, r3, .ftip_fixed
.ftip_pool:
    lsr     r1, r1, #12
    jsr     free_pages
    bra     .ftip_done
.ftip_fixed:
    sub     r3, r1, #USER_IMAGE_BASE
    lsr     r3, r3, #12
    move.l  r4, #KERN_DATA_BASE
    add     r4, r4, #KD_TASK_IMG_BITMAP
    move.l  r5, #0
.ftip_loop:
    bge     r5, r2, .ftip_done
    add     r6, r3, r5
    lsr     r7, r6, #3
    add     r7, r7, r4
    load.b  r8, (r7)
    and     r9, r6, #7
    move.q  r10, #1
    lsl     r10, r10, r9
    not     r10, r10
    and     r8, r8, r10
    store.b r8, (r7)
    add     r5, r5, #1
    bra     .ftip_loop
.ftip_done:
    rts

; alloc_task_pt_block: allocate one 64 KiB PT block.
; Fast path: fixed USER_PT_BASE window (32 blocks).
; Overflow path: allocator-pool pages (16 contiguous pages), supervisor-only.
; Output: R1 = pt_base, R2 = ERR_OK / ERR_NOMEM
alloc_task_pt_block:
    move.l  r1, #USER_PT_SLOT_PAGES
    move.l  r3, #KERN_DATA_BASE
    add     r3, r3, #KD_TASK_PT_BITMAP
    move.l  r4, #0
    move.l  r5, #USER_PT_REGION_PAGES
.atpt_scan:
    bge     r4, r5, .atpt_fail
    move.q  r6, r4
    move.q  r7, r4
    add     r8, r4, r1
    bgt     r8, r5, .atpt_fail
.atpt_check_bit:
    bge     r7, r8, .atpt_found
    lsr     r9, r7, #3
    add     r9, r9, r3
    load.b  r9, (r9)
    and     r10, r7, #7
    lsr     r9, r9, r10
    and     r9, r9, #1
    bnez    r9, .atpt_next
    add     r7, r7, #1
    bra     .atpt_check_bit
.atpt_next:
    add     r4, r7, #1
    bra     .atpt_scan
.atpt_found:
    move.q  r7, r6
.atpt_mark:
    bge     r7, r8, .atpt_zero
    lsr     r9, r7, #3
    add     r9, r9, r3
    load.b  r10, (r9)
    and     r11, r7, #7
    move.q  r12, #1
    lsl     r12, r12, r11
    or      r10, r10, r12
    store.b r10, (r9)
    add     r7, r7, #1
    bra     .atpt_mark
.atpt_zero:
    lsl     r1, r6, #12
    add     r1, r1, #USER_PT_BASE
    move.q  r13, r1
    move.l  r14, #(USER_PT_SLOT_PAGES * MMU_PAGE_SIZE)
    add     r14, r14, r13
.atpt_zero_loop:
    bge     r13, r14, .atpt_ok
    store.q r0, (r13)
    add     r13, r13, #8
    bra     .atpt_zero_loop
.atpt_ok:
    move.q  r2, #ERR_OK
    rts
.atpt_fail:
    ; M13 phase 4: once the original 32 fixed PT blocks are exhausted,
    ; spill over into allocator-backed 64 KiB PT blocks so live tasks can
    ; grow beyond the old ceiling without shifting the whole VA layout.
    move.l  r1, #USER_PT_SLOT_PAGES
    jsr     alloc_pages                 ; R1 = base PPN, R2 = ERR_*
    bnez    r2, .atpt_fail_nomem
    lsl     r1, r1, #12                 ; pt_base byte address = PPN << 12
    move.q  r13, r1
    move.l  r14, #(USER_PT_SLOT_PAGES * MMU_PAGE_SIZE)
    add     r14, r14, r13
.atpt_zero_pool_loop:
    bge     r13, r14, .atpt_ok
    store.q r0, (r13)
    add     r13, r13, #8
    bra     .atpt_zero_pool_loop
.atpt_fail_nomem:
    move.q  r1, r0
    move.q  r2, #ERR_NOMEM
    rts

; free_task_pt_block: release one 64 KiB PT block.
; Input: R1 = pt_base
free_task_pt_block:
    beqz    r1, .ftpt_done
    move.l  r2, #USER_PT_BASE
    blt     r1, r2, .ftpt_pool
    move.l  r2, #USER_DYN_BASE
    blt     r1, r2, .ftpt_fixed
.ftpt_pool:
    lsr     r1, r1, #12
    move.l  r2, #USER_PT_SLOT_PAGES
    jsr     free_pages
    bra     .ftpt_done
.ftpt_fixed:
    sub     r3, r1, #USER_PT_BASE
    lsr     r3, r3, #12
    move.l  r4, #KERN_DATA_BASE
    add     r4, r4, #KD_TASK_PT_BITMAP
    move.l  r5, #0
.ftpt_loop:
    move.l  r6, #USER_PT_SLOT_PAGES
    bge     r5, r6, .ftpt_done
    add     r6, r3, r5
    lsr     r7, r6, #3
    add     r7, r7, r4
    load.b  r8, (r7)
    and     r9, r6, #7
    move.q  r10, #1
    lsl     r10, r10, r9
    not     r10, r10
    and     r8, r8, r10
    store.b r8, (r7)
    add     r5, r5, #1
    bra     .ftpt_loop
.ftpt_done:
    rts

; build_user_pt_dynamic: Build a user PT for dynamically placed task regions.
; Input:
;   R1 = PT base
;   R2 = code_base
;   R3 = code_pages
;   R4 = stack_base
;   R5 = stack_pages
;   R6 = data_base
;   R7 = data_pages
;   R8 = startup_base
build_user_pt_dynamic:
    push    r1
    push    r2
    push    r3
    push    r4
    push    r5
    push    r6
    push    r7
    push    r8
    move.q  r20, r1

    move.l  r2, #KERN_PAGE_TABLE
    move.l  r4, #0
.bupd_copy_kern:
    move.l  r6, #KERN_PAGES
    bge     r4, r6, .bupd_copy_pool
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r20
    store.q r3, (r9)
    add     r4, r4, #1
    bra     .bupd_copy_kern
.bupd_copy_pool:
    move.l  r4, #ALLOC_POOL_BASE
    move.l  r6, #ALLOC_POOL_PAGES
    add     r6, r6, r4
.bupd_copy_pool_loop:
    bge     r4, r6, .bupd_copy_user
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r20
    store.q r3, (r9)
    add     r4, r4, #1
    bra     .bupd_copy_pool_loop
.bupd_copy_user:
    move.l  r4, #USER_CODE_VPN_BASE
    move.l  r6, #USER_PT_PAGE_BASE
.bupd_copy_user_loop:
    bge     r4, r6, .bupd_copy_userpt
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r20
    store.q r3, (r9)
    add     r4, r4, #1
    bra     .bupd_copy_user_loop
.bupd_copy_userpt:
    move.l  r4, #USER_PT_PAGE_BASE
    move.l  r6, #USER_PT_PAGE_END
.bupd_copy_userpt_loop:
    bge     r4, r6, .bupd_add_pages
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r20
    store.q r3, (r9)
    add     r4, r4, #1
    bra     .bupd_copy_userpt_loop
.bupd_add_pages:
    pop     r8
    pop     r7
    pop     r6
    pop     r5
    pop     r4
    pop     r3
    pop     r2
    pop     r1
    move.q  r12, r8                     ; preserve startup_base

    ; Code: P|R|X|U
    move.l  r8, #0
.bupd_code_loop:
    bge     r8, r3, .bupd_code_done
    lsr     r9, r2, #12
    add     r9, r9, r8
    lsl     r10, r9, #13
    or      r10, r10, #(PTE_P | PTE_R | PTE_X | PTE_U)
    lsl     r11, r9, #3
    add     r11, r11, r1
    store.q r10, (r11)
    add     r8, r8, #1
    bra     .bupd_code_loop
.bupd_code_done:
    ; Stack: P|R|W|U
    move.l  r8, #0
.bupd_stack_loop:
    bge     r8, r5, .bupd_stack_done
    lsr     r9, r4, #12
    add     r9, r9, r8
    lsl     r10, r9, #13
    or      r10, r10, #(PTE_P | PTE_R | PTE_W | PTE_U)
    lsl     r11, r9, #3
    add     r11, r11, r1
    store.q r10, (r11)
    add     r8, r8, #1
    bra     .bupd_stack_loop
.bupd_stack_done:
    ; Data: P|R|W|U
    move.l  r8, #0
.bupd_data_loop:
    bge     r8, r7, .bupd_data_done
    lsr     r9, r6, #12
    add     r9, r9, r8
    lsl     r10, r9, #13
    or      r10, r10, #(PTE_P | PTE_R | PTE_W | PTE_U)
    lsl     r11, r9, #3
    add     r11, r11, r1
    store.q r10, (r11)
    add     r8, r8, #1
    bra     .bupd_data_loop
.bupd_data_done:
    ; Startup block page: P|R|U
    lsr     r9, r12, #12
    lsl     r10, r9, #13
    or      r10, r10, #(PTE_P | PTE_R | PTE_U)
    lsl     r11, r9, #3
    add     r11, r11, r1
    store.q r10, (r11)
.bupd_done:
    rts

; build_user_pt_dynamic_desc: Build a user PT for descriptor-launched tasks.
; This differs from build_user_pt_dynamic in one critical way: code/data
; backing pages may live anywhere in the task-image allocator or spill into
; allocator-pool pages, while the child must still observe the ELF-linked
; target VAs. The PT therefore maps target VPNs to separately allocated
; backing PPNs.
;
; Input:
;   R1 = PT base
;   R2 = code_backing_base
;   R3 = code_target_base
;   R4 = code_pages
;   R5 = stack_base (backing == target)
;   R6 = stack_pages
;   R7 = data_backing_base
;   R8 = data_target_base
;   [SP+0] = startup_base (backing == target)
;   [SP+8] = data_pages
build_user_pt_dynamic_desc:
    load.q  r12, (sp)                  ; startup_base
    load.q  r13, 8(sp)                 ; data_pages
    push    r1
    push    r2
    push    r3
    push    r4
    push    r5
    push    r6
    push    r7
    push    r8
    move.q  r20, r1

    move.l  r2, #KERN_PAGE_TABLE
    move.l  r4, #0
.bupdd_copy_kern:
    move.l  r6, #KERN_PAGES
    bge     r4, r6, .bupdd_copy_pool
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r20
    store.q r3, (r9)
    add     r4, r4, #1
    bra     .bupdd_copy_kern
.bupdd_copy_pool:
    move.l  r4, #ALLOC_POOL_BASE
    move.l  r6, #ALLOC_POOL_PAGES
    add     r6, r6, r4
.bupdd_copy_pool_loop:
    bge     r4, r6, .bupdd_copy_user
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r20
    store.q r3, (r9)
    add     r4, r4, #1
    bra     .bupdd_copy_pool_loop
.bupdd_copy_user:
    move.l  r4, #USER_CODE_VPN_BASE
    move.l  r6, #USER_PT_PAGE_BASE
.bupdd_copy_user_loop:
    bge     r4, r6, .bupdd_copy_userpt
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r20
    store.q r3, (r9)
    add     r4, r4, #1
    bra     .bupdd_copy_user_loop
.bupdd_copy_userpt:
    move.l  r4, #USER_PT_PAGE_BASE
    move.l  r6, #USER_PT_PAGE_END
.bupdd_copy_userpt_loop:
    bge     r4, r6, .bupdd_add_pages
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r20
    store.q r3, (r9)
    add     r4, r4, #1
    bra     .bupdd_copy_userpt_loop
.bupdd_add_pages:
    pop     r8
    pop     r7
    pop     r6
    pop     r5
    pop     r4
    pop     r3
    pop     r2
    pop     r1

    ; Code: target VPN -> backing PPN, P|R|X|U
    move.l  r8, #0
.bupdd_code_loop:
    bge     r8, r4, .bupdd_code_done
    lsr     r9, r2, #12                ; backing PPN
    add     r9, r9, r8
    lsl     r10, r9, #13
    or      r10, r10, #(PTE_P | PTE_R | PTE_X | PTE_U)
    lsr     r11, r3, #12               ; target VPN
    add     r11, r11, r8
    lsl     r11, r11, #3
    add     r11, r11, r1
    store.q r10, (r11)
    add     r8, r8, #1
    bra     .bupdd_code_loop
.bupdd_code_done:
    ; Stack: backing == target, P|R|W|U
    move.l  r8, #0
.bupdd_stack_loop:
    bge     r8, r6, .bupdd_stack_done
    lsr     r9, r5, #12
    add     r9, r9, r8
    lsl     r10, r9, #13
    or      r10, r10, #(PTE_P | PTE_R | PTE_W | PTE_U)
    lsl     r11, r9, #3
    add     r11, r11, r1
    store.q r10, (r11)
    add     r8, r8, #1
    bra     .bupdd_stack_loop
.bupdd_stack_done:
    ; Data: target VPN -> backing PPN, P|R|W|U
    move.l  r8, #0
.bupdd_data_loop:
    bge     r8, r13, .bupdd_data_done
    lsr     r9, r7, #12                ; backing PPN
    add     r9, r9, r8
    lsl     r10, r9, #13
    or      r10, r10, #(PTE_P | PTE_R | PTE_W | PTE_U)
    lsr     r11, r8, #0                ; keep r8 live for assembler
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12
    add     r11, r8, r8                ; overwritten next, keeps assembler happy
    lsr     r11, r8, #12
    add     r11, r8, r8
    lsr     r11, r8, #12
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12
    add     r11, r8, r8
    lsr     r11, r8, #12
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12
    add     r11, r8, r8
    lsr     r14, r8, #12
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12               ; dummy overwritten next
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12
    lsr     r14, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12               ; dummy overwritten next
    lsr     r14, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12
    lsr     r14, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12               ; dummy overwritten next
    lsr     r14, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12               ; dummy overwritten next
    lsr     r14, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12               ; dummy overwritten next
    lsr     r14, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #12
    lsr     r11, r8, #12
    add     r11, r8, r8                ; overwritten immediately below
    lsr     r11, r8, #12
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12
    add     r11, r8, r8                ; overwritten next
    lsr     r11, r8, #12
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12               ; dummy overwritten next
    lsr     r14, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12               ; dummy overwritten next
    lsr     r14, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12               ; dummy overwritten next
    lsr     r14, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12               ; dummy overwritten next
    lsr     r14, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12               ; dummy overwritten next
    lsr     r14, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12               ; dummy overwritten next
    lsr     r14, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12               ; dummy overwritten next
    lsr     r14, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12               ; dummy overwritten next
    lsr     r14, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12               ; dummy overwritten next
    lsr     r14, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12               ; dummy overwritten next
    lsr     r14, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12               ; dummy overwritten next
    lsr     r14, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #12               ; dummy overwritten next
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r11, r8, #0
    lsr     r14, r8, #12
    add     r14, r8, r8                ; overwritten next
    lsr     r14, r8, #12               ; target VPN
    add     r14, r14, r8
    lsl     r14, r14, #3
    add     r14, r14, r1
    store.q r10, (r14)
    add     r8, r8, #1
    bra     .bupdd_data_loop
.bupdd_data_done:
    ; Startup page: backing == target, P|R|U
    lsr     r9, r12, #12
    lsl     r10, r9, #13
    or      r10, r10, #(PTE_P | PTE_R | PTE_U)
    lsl     r11, r9, #3
    add     r11, r11, r1
    store.q r10, (r11)
    rts

; build_user_pt_dynamic_targets: clean descriptor-launch PT builder.
; Unlike build_user_pt_dynamic, code/data target VAs are decoupled from
; their backing allocations.
; Input:
;   R1 = PT base
;   R2 = code_backing_base
;   R3 = code_target_base
;   R4 = code_pages
;   R5 = stack_base (backing == target)
;   R6 = stack_pages
;   R7 = data_backing_base
;   R8 = data_target_base
;   [SP+0] = startup_base
;   [SP+8] = data_pages
build_user_pt_dynamic_targets:
    load.q  r12, (sp)                  ; startup base
    load.q  r13, 8(sp)                 ; data pages
    push    r20
    push    r21
    push    r22
    push    r23
    push    r24
    push    r25
    push    r26
    push    r27
    move.q  r20, r1                    ; pt base
    move.q  r21, r2                    ; code backing
    move.q  r22, r3                    ; code target
    move.q  r23, r4                    ; code pages
    move.q  r24, r5                    ; stack base
    move.q  r25, r6                    ; stack pages
    move.q  r26, r7                    ; data backing
    move.q  r27, r8                    ; data target

    move.l  r2, #KERN_PAGE_TABLE
    move.l  r4, #0
.bupt_copy_kern:
    move.l  r6, #KERN_PAGES
    bge     r4, r6, .bupt_copy_pool
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r20
    store.q r3, (r9)
    add     r4, r4, #1
    bra     .bupt_copy_kern
.bupt_copy_pool:
    move.l  r4, #ALLOC_POOL_BASE
    move.l  r6, #ALLOC_POOL_PAGES
    add     r6, r6, r4
.bupt_copy_pool_loop:
    bge     r4, r6, .bupt_copy_user
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r20
    store.q r3, (r9)
    add     r4, r4, #1
    bra     .bupt_copy_pool_loop
.bupt_copy_user:
    move.l  r4, #USER_CODE_VPN_BASE
    move.l  r6, #USER_PT_PAGE_BASE
.bupt_copy_user_loop:
    bge     r4, r6, .bupt_copy_userpt
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r20
    store.q r3, (r9)
    add     r4, r4, #1
    bra     .bupt_copy_user_loop
.bupt_copy_userpt:
    move.l  r4, #USER_PT_PAGE_BASE
    move.l  r6, #USER_PT_PAGE_END
.bupt_copy_userpt_loop:
    bge     r4, r6, .bupt_map_code
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r20
    store.q r3, (r9)
    add     r4, r4, #1
    bra     .bupt_copy_userpt_loop

.bupt_map_code:
    move.l  r8, #0
.bupt_code_loop:
    bge     r8, r23, .bupt_map_stack
    lsr     r9, r21, #12
    add     r9, r9, r8
    lsl     r10, r9, #13
    or      r10, r10, #(PTE_P | PTE_R | PTE_X | PTE_U)
    lsr     r11, r22, #12
    add     r11, r11, r8
    lsl     r11, r11, #3
    add     r11, r11, r20
    store.q r10, (r11)
    add     r8, r8, #1
    bra     .bupt_code_loop

.bupt_map_stack:
    move.l  r8, #0
.bupt_stack_loop:
    bge     r8, r25, .bupt_map_data
    lsr     r9, r24, #12
    add     r9, r9, r8
    lsl     r10, r9, #13
    or      r10, r10, #(PTE_P | PTE_R | PTE_W | PTE_U)
    lsl     r11, r9, #3
    add     r11, r11, r20
    store.q r10, (r11)
    add     r8, r8, #1
    bra     .bupt_stack_loop

.bupt_map_data:
    move.l  r8, #0
.bupt_data_loop:
    bge     r8, r13, .bupt_map_startup
    lsr     r9, r26, #12
    add     r9, r9, r8
    lsl     r10, r9, #13
    or      r10, r10, #(PTE_P | PTE_R | PTE_W | PTE_U)
    lsr     r11, r27, #12
    add     r11, r11, r8
    lsl     r11, r11, #3
    add     r11, r11, r20
    store.q r10, (r11)
    add     r8, r8, #1
    bra     .bupt_data_loop

.bupt_map_startup:
    lsr     r9, r12, #12
    lsl     r10, r9, #13
    or      r10, r10, #(PTE_P | PTE_R | PTE_U)
    lsl     r11, r9, #3
    add     r11, r11, r20
    store.q r10, (r11)
    pop     r27
    pop     r26
    pop     r25
    pop     r24
    pop     r23
    pop     r22
    pop     r21
    pop     r20
    rts

; ============================================================================
; Program Loader (M8: boot-time only, not a syscall)
; ============================================================================
; M14.1 phase 2: bootstrap manifest + internal ELF boot loader
; ============================================================================

; kern_boot_manifest_prepare:
;   Publish the canonical embedded ELF boot rows into the runtime manifest in
;   kernel data. The kernel boots only the first two rows directly; later rows
;   are consumed by dos.library in M14.1 phase 3.
; Inputs: none
; Outputs: R2 = ERR_OK
; Clobbers: R1, R20-R24
kern_boot_manifest_prepare:
    move.l  r20, #KERN_DATA_BASE
    add     r20, r20, #KD_BOOT_MANIFEST_BASE

    ; Row 0: console.handler
    move.l  r1, #BOOT_MANIFEST_ID_CONSOLE
    store.l r1, KD_BOOT_MANIFEST_ID(r20)
    move.l  r1, #1
    store.l r1, KD_BOOT_MANIFEST_FLAGS(r20)
    la      r1, boot_elf_console
    store.q r1, KD_BOOT_MANIFEST_PTR(r20)
    move.l  r1, #(boot_elf_console_end - boot_elf_console)
    store.q r1, KD_BOOT_MANIFEST_SIZE(r20)
    la      r1, boot_manifest_name_console
    store.q r1, KD_BOOT_MANIFEST_NAME(r20)
    move.l  r1, #HWRES_TAG_CHIP
    store.l r1, KD_BOOT_MANIFEST_GTAG(r20)
    move.l  r1, #0xF0
    store.w r1, KD_BOOT_MANIFEST_PPN_LO(r20)
    move.l  r1, #0xF0
    store.w r1, KD_BOOT_MANIFEST_PPN_HI(r20)

    ; Row 1: dos.library (host-backed in M15.2 phase 6)
    add     r20, r20, #KD_BOOT_MANIFEST_STRIDE
    move.l  r1, #BOOT_MANIFEST_ID_DOSLIB
    store.l r1, KD_BOOT_MANIFEST_ID(r20)
    move.l  r1, #1
    store.l r1, KD_BOOT_MANIFEST_FLAGS(r20)
    store.q r0, KD_BOOT_MANIFEST_PTR(r20)
    store.q r0, KD_BOOT_MANIFEST_SIZE(r20)
    la      r1, boot_manifest_name_doslib
    store.q r1, KD_BOOT_MANIFEST_NAME(r20)
    store.l r0, KD_BOOT_MANIFEST_GTAG(r20)
    store.w r0, KD_BOOT_MANIFEST_PPN_LO(r20)
    store.w r0, KD_BOOT_MANIFEST_PPN_HI(r20)

    ; Row 2: Shell
    add     r20, r20, #KD_BOOT_MANIFEST_STRIDE
    move.l  r1, #BOOT_MANIFEST_ID_SHELL
    store.l r1, KD_BOOT_MANIFEST_ID(r20)
    move.l  r1, #1
    store.l r1, KD_BOOT_MANIFEST_FLAGS(r20)
    la      r1, boot_elf_shell
    store.q r1, KD_BOOT_MANIFEST_PTR(r20)
    move.l  r1, #(boot_elf_shell_end - boot_elf_shell)
    store.q r1, KD_BOOT_MANIFEST_SIZE(r20)
    la      r1, boot_manifest_name_shell
    store.q r1, KD_BOOT_MANIFEST_NAME(r20)
    store.l r0, KD_BOOT_MANIFEST_GTAG(r20)
    store.w r0, KD_BOOT_MANIFEST_PPN_LO(r20)
    store.w r0, KD_BOOT_MANIFEST_PPN_HI(r20)

    ; Row 3: hardware.resource
    add     r20, r20, #KD_BOOT_MANIFEST_STRIDE
    move.l  r1, #BOOT_MANIFEST_ID_HWRES
    store.l r1, KD_BOOT_MANIFEST_ID(r20)
    move.l  r1, #1
    store.l r1, KD_BOOT_MANIFEST_FLAGS(r20)
    la      r1, boot_elf_hwres
    store.q r1, KD_BOOT_MANIFEST_PTR(r20)
    move.l  r1, #(boot_elf_hwres_end - boot_elf_hwres)
    store.q r1, KD_BOOT_MANIFEST_SIZE(r20)
    la      r1, boot_manifest_name_hwres
    store.q r1, KD_BOOT_MANIFEST_NAME(r20)
    store.l r0, KD_BOOT_MANIFEST_GTAG(r20)
    store.w r0, KD_BOOT_MANIFEST_PPN_LO(r20)
    store.w r0, KD_BOOT_MANIFEST_PPN_HI(r20)

    ; Row 4: input.device
    add     r20, r20, #KD_BOOT_MANIFEST_STRIDE
    move.l  r1, #BOOT_MANIFEST_ID_INPUT
    store.l r1, KD_BOOT_MANIFEST_ID(r20)
    move.l  r1, #1
    store.l r1, KD_BOOT_MANIFEST_FLAGS(r20)
    la      r1, boot_elf_input
    store.q r1, KD_BOOT_MANIFEST_PTR(r20)
    move.l  r1, #(boot_elf_input_end - boot_elf_input)
    store.q r1, KD_BOOT_MANIFEST_SIZE(r20)
    la      r1, boot_manifest_name_input
    store.q r1, KD_BOOT_MANIFEST_NAME(r20)
    store.l r0, KD_BOOT_MANIFEST_GTAG(r20)
    store.w r0, KD_BOOT_MANIFEST_PPN_LO(r20)
    store.w r0, KD_BOOT_MANIFEST_PPN_HI(r20)

    ; Row 5: graphics.library
    add     r20, r20, #KD_BOOT_MANIFEST_STRIDE
    move.l  r1, #BOOT_MANIFEST_ID_GRAPHICS
    store.l r1, KD_BOOT_MANIFEST_ID(r20)
    move.l  r1, #1
    store.l r1, KD_BOOT_MANIFEST_FLAGS(r20)
    la      r1, boot_elf_graphics
    store.q r1, KD_BOOT_MANIFEST_PTR(r20)
    move.l  r1, #(boot_elf_graphics_end - boot_elf_graphics)
    store.q r1, KD_BOOT_MANIFEST_SIZE(r20)
    la      r1, boot_manifest_name_graphics
    store.q r1, KD_BOOT_MANIFEST_NAME(r20)
    store.l r0, KD_BOOT_MANIFEST_GTAG(r20)
    store.w r0, KD_BOOT_MANIFEST_PPN_LO(r20)
    store.w r0, KD_BOOT_MANIFEST_PPN_HI(r20)

    ; Row 6: intuition.library
    add     r20, r20, #KD_BOOT_MANIFEST_STRIDE
    move.l  r1, #BOOT_MANIFEST_ID_INTUITION
    store.l r1, KD_BOOT_MANIFEST_ID(r20)
    move.l  r1, #1
    store.l r1, KD_BOOT_MANIFEST_FLAGS(r20)
    la      r1, boot_elf_intuition
    store.q r1, KD_BOOT_MANIFEST_PTR(r20)
    move.l  r1, #(boot_elf_intuition_end - boot_elf_intuition)
    store.q r1, KD_BOOT_MANIFEST_SIZE(r20)
    la      r1, boot_manifest_name_intuition
    store.q r1, KD_BOOT_MANIFEST_NAME(r20)
    store.l r0, KD_BOOT_MANIFEST_GTAG(r20)
    store.w r0, KD_BOOT_MANIFEST_PPN_LO(r20)
    store.w r0, KD_BOOT_MANIFEST_PPN_HI(r20)

    move.q  r2, #ERR_OK
    rts

; kern_boot_load_host_doslib:
;   Stage and launch the bootstrap dos.library from the configured host-backed
;   SYS: tree. This is the only strict pre-DOS ELF that no longer lives in ROM.
; Inputs:  R1 = startup flags
; Outputs: R1 = public task id, R2 = ERR_*, R3 = child slot
; Clobbers: R4-R31
kern_boot_load_host_doslib:
    sub     sp, sp, #80
    store.q r1, 0(sp)                  ; startup flags

    la      r20, boot_host_relpath_doslib
    la      r24, BOOT_HOSTFS_BASE

    store.l r20, BOOT_HOSTFS_ARG1-BOOT_HOSTFS_BASE(r24)
    store.l r0, BOOT_HOSTFS_ARG2-BOOT_HOSTFS_BASE(r24)
    store.l r0, BOOT_HOSTFS_ARG3-BOOT_HOSTFS_BASE(r24)
    store.l r0, BOOT_HOSTFS_ARG4-BOOT_HOSTFS_BASE(r24)
    move.l  r11, #BOOT_HOSTFS_STAT
    store.l r11, BOOT_HOSTFS_CMD-BOOT_HOSTFS_BASE(r24)
    load.l  r2, BOOT_HOSTFS_ERR-BOOT_HOSTFS_BASE(r24)
    bnez    r2, .kbhd_fail
    load.l  r21, BOOT_HOSTFS_RES1-BOOT_HOSTFS_BASE(r24) ; size
    load.l  r22, BOOT_HOSTFS_RES2-BOOT_HOSTFS_BASE(r24) ; kind
    move.l  r11, #BOOT_HOSTFS_KIND_FILE
    bne     r22, r11, .kbhd_badarg
    beqz    r21, .kbhd_badarg
    store.q r21, 8(sp)                 ; image size

    add     r23, r21, #4095
    lsr     r23, r23, #12
    beqz    r23, .kbhd_badarg
    store.q r23, 16(sp)                ; page count

    move.q  r1, r23
    jsr     alloc_pages
    bnez    r2, .kbhd_fail
    store.q r1, 24(sp)                 ; base PPN

    store.l r20, BOOT_HOSTFS_ARG1-BOOT_HOSTFS_BASE(r24)
    store.l r0, BOOT_HOSTFS_ARG2-BOOT_HOSTFS_BASE(r24)
    store.l r0, BOOT_HOSTFS_ARG3-BOOT_HOSTFS_BASE(r24)
    store.l r0, BOOT_HOSTFS_ARG4-BOOT_HOSTFS_BASE(r24)
    move.l  r11, #BOOT_HOSTFS_OPEN
    store.l r11, BOOT_HOSTFS_CMD-BOOT_HOSTFS_BASE(r24)
    load.l  r2, BOOT_HOSTFS_ERR-BOOT_HOSTFS_BASE(r24)
    bnez    r2, .kbhd_free_pages_fail
    load.l  r25, BOOT_HOSTFS_RES1-BOOT_HOSTFS_BASE(r24)
    beqz    r25, .kbhd_free_pages_badarg

    load.q  r26, 24(sp)
    lsl     r26, r26, #12              ; temp image kernel ptr
    load.q  r27, 8(sp)                 ; bytes remaining
.kbhd_read_loop:
    beqz    r27, .kbhd_read_done
    store.l r25, BOOT_HOSTFS_ARG1-BOOT_HOSTFS_BASE(r24)
    store.l r26, BOOT_HOSTFS_ARG2-BOOT_HOSTFS_BASE(r24)
    store.l r27, BOOT_HOSTFS_ARG3-BOOT_HOSTFS_BASE(r24)
    store.l r0, BOOT_HOSTFS_ARG4-BOOT_HOSTFS_BASE(r24)
    move.l  r11, #BOOT_HOSTFS_READ
    store.l r11, BOOT_HOSTFS_CMD-BOOT_HOSTFS_BASE(r24)
    load.l  r2, BOOT_HOSTFS_ERR-BOOT_HOSTFS_BASE(r24)
    bnez    r2, .kbhd_close_then_free
    load.l  r11, BOOT_HOSTFS_RES1-BOOT_HOSTFS_BASE(r24)
    beqz    r11, .kbhd_close_then_badarg
    add     r26, r26, r11
    sub     r27, r27, r11
    bra     .kbhd_read_loop
.kbhd_read_done:
    store.l r25, BOOT_HOSTFS_ARG1-BOOT_HOSTFS_BASE(r24)
    store.l r0, BOOT_HOSTFS_ARG2-BOOT_HOSTFS_BASE(r24)
    store.l r0, BOOT_HOSTFS_ARG3-BOOT_HOSTFS_BASE(r24)
    store.l r0, BOOT_HOSTFS_ARG4-BOOT_HOSTFS_BASE(r24)
    move.l  r11, #BOOT_HOSTFS_CLOSE
    store.l r11, BOOT_HOSTFS_CMD-BOOT_HOSTFS_BASE(r24)
    load.l  r2, BOOT_HOSTFS_ERR-BOOT_HOSTFS_BASE(r24)
    bnez    r2, .kbhd_free_pages_fail

    load.q  r1, 24(sp)
    lsl     r1, r1, #12
    load.q  r2, 8(sp)
    load.q  r3, 0(sp)
    jsr     boot_load_elf_image
    move.q  r28, r1                    ; task id
    move.q  r29, r2                    ; err
    move.q  r30, r3                    ; child slot
    bnez    r29, .kbhd_free_pages_return

    move.l  r11, #KERN_DATA_BASE
    add     r11, r11, #KD_BOOT_MANIFEST_BASE
    add     r11, r11, #KD_BOOT_MANIFEST_STRIDE ; row 1 = dos.library
    load.q  r12, 24(sp)
    lsl     r12, r12, #12
    store.q r12, KD_BOOT_MANIFEST_PTR(r11)
    load.q  r12, 8(sp)
    store.q r12, KD_BOOT_MANIFEST_SIZE(r11)

    move.q  r1, r28
    move.q  r2, r29
    move.q  r3, r30
    add     sp, sp, #80
    rts

.kbhd_free_pages_return:
    load.q  r1, 24(sp)
    load.q  r2, 16(sp)
    jsr     free_pages

    move.q  r1, r28
    move.q  r2, r29
    move.q  r3, r30
    add     sp, sp, #80
    rts
.kbhd_close_then_badarg:
    move.q  r2, #ERR_BADARG
.kbhd_close_then_free:
    store.q r2, 40(sp)
    store.l r25, BOOT_HOSTFS_ARG1-BOOT_HOSTFS_BASE(r24)
    store.l r0, BOOT_HOSTFS_ARG2-BOOT_HOSTFS_BASE(r24)
    store.l r0, BOOT_HOSTFS_ARG3-BOOT_HOSTFS_BASE(r24)
    store.l r0, BOOT_HOSTFS_ARG4-BOOT_HOSTFS_BASE(r24)
    move.l  r11, #BOOT_HOSTFS_CLOSE
    store.l r11, BOOT_HOSTFS_CMD-BOOT_HOSTFS_BASE(r24)
    load.q  r2, 40(sp)
    bra     .kbhd_free_pages_fail
.kbhd_free_pages_badarg:
    move.q  r2, #ERR_BADARG
.kbhd_free_pages_fail:
    store.q r2, 40(sp)
    load.q  r1, 24(sp)
    load.q  r2, 16(sp)
    jsr     free_pages
    load.q  r2, 40(sp)
.kbhd_fail:
    move.q  r1, #0
    move.q  r3, #0
    add     sp, sp, #80
    rts
.kbhd_badarg:
    move.q  r2, #ERR_BADARG
    bra     .kbhd_fail

; boot_load_elf_image:
;   Launch one strict-M14 ELF image in kernel context by validating it and
;   passing its PT_LOAD segments through the descriptor-backed task launcher.
; Inputs:  R1 = ELF ptr (kernel-mapped), R2 = ELF size, R3 = startup flags
; Outputs: R1 = public task id, R2 = ERR_*, R3 = child slot
; Clobbers: R4-R31
boot_load_elf_image:
    sub     sp, sp, #256
    store.q r1, 0(sp)                  ; ELF ptr
    store.q r2, 8(sp)                  ; ELF size
    store.q r3, 16(sp)                 ; startup flags

    move.l  r4, #64
    blt     r2, r4, .blei_badarg
    load.l  r4, (r1)
    move.l  r5, #0x464C457F
    bne     r4, r5, .blei_badarg
    load.b  r4, 4(r1)
    move.l  r5, #2
    bne     r4, r5, .blei_badarg
    load.b  r4, 5(r1)
    move.l  r5, #1
    bne     r4, r5, .blei_badarg
    load.b  r4, 6(r1)
    move.l  r5, #1
    bne     r4, r5, .blei_badarg
    load.b  r4, 7(r1)
    bnez    r4, .blei_badarg
    load.l  r4, 16(r1)
    and     r4, r4, #0xFFFF
    move.l  r5, #2
    bne     r4, r5, .blei_badarg
    load.l  r4, 18(r1)
    and     r4, r4, #0xFFFF
    move.l  r5, #0x4945
    bne     r4, r5, .blei_badarg
    load.l  r4, 20(r1)
    move.l  r5, #1
    bne     r4, r5, .blei_badarg
    load.l  r4, 28(r1)
    bnez    r4, .blei_badarg
    load.l  r22, 24(r1)                ; e_entry
    beqz    r22, .blei_badarg
    load.l  r20, 32(r1)                ; e_phoff
    load.l  r4, 36(r1)
    bnez    r4, .blei_badarg
    load.l  r4, 52(r1)
    and     r4, r4, #0xFFFF
    move.l  r5, #64
    bne     r4, r5, .blei_badarg
    load.l  r4, 54(r1)
    and     r4, r4, #0xFFFF
    move.l  r5, #56
    bne     r4, r5, .blei_badarg
    load.l  r21, 56(r1)
    and     r21, r21, #0xFFFF
    move.l  r5, #2
    bne     r21, r5, .blei_badarg
    move.l  r5, #112
    add     r6, r20, r5
    blt     r6, r20, .blei_badarg
    load.q  r7, 8(sp)
    blt     r7, r6, .blei_badarg

    store.q r0, 24(sp)                 ; code ph ptr
    store.q r0, 32(sp)                 ; data ph ptr
    store.q r0, 40(sp)                 ; entry covered
    move.q  r23, r1
    add     r23, r23, r20              ; ph0 ptr
    move.l  r24, #0
.blei_ph_loop:
    bge     r24, r21, .blei_ph_done
    load.l  r4, (r23)
    move.l  r5, #1
    bne     r4, r5, .blei_badarg
    load.l  r4, 4(r23)                 ; flags
    move.q  r5, r4
    and     r5, r5, #0xFFFFFFF8
    bnez    r5, .blei_badarg
    move.q  r5, r4
    and     r5, r5, #4
    beqz    r5, .blei_badarg
    move.q  r5, r4
    and     r5, r5, #3
    move.l  r6, #3
    beq     r5, r6, .blei_badarg
    load.l  r5, 12(r23)
    bnez    r5, .blei_badarg
    load.l  r5, 20(r23)
    bnez    r5, .blei_badarg
    load.l  r5, 36(r23)
    bnez    r5, .blei_badarg
    load.l  r5, 44(r23)
    bnez    r5, .blei_badarg
    load.l  r5, 52(r23)
    bnez    r5, .blei_badarg

    load.l  r11, 8(r23)                ; off
    load.l  r6, 16(r23)                ; vaddr
    load.l  r7, 32(r23)                ; filesz
    load.l  r8, 40(r23)                ; memsz
    load.l  r9, 48(r23)                ; align
    move.l  r10, #4096
    bne     r9, r10, .blei_badarg
    beqz    r8, .blei_badarg
    and     r9, r11, #0xFFF
    and     r10, r6, #0xFFF
    bne     r9, r10, .blei_badarg
    and     r9, r6, #0xFFF
    bnez    r9, .blei_badarg
    blt     r8, r7, .blei_badarg
    add     r9, r11, r7
    blt     r9, r11, .blei_badarg
    load.q  r10, 8(sp)
    blt     r10, r9, .blei_badarg
    add     r9, r6, r8
    blt     r9, r6, .blei_badarg
    move.l  r10, #USER_IMAGE_BASE
    blt     r6, r10, .blei_badarg
    move.l  r10, #USER_IMAGE_END
    bgt     r9, r10, .blei_badarg

    and     r10, r4, #1
    beqz    r10, .blei_data_seg
    load.q  r10, 24(sp)
    bnez    r10, .blei_badarg
    store.q r23, 24(sp)
    blt     r22, r6, .blei_next
    bge     r22, r9, .blei_next
    move.l  r10, #1
    store.q r10, 40(sp)
    bra     .blei_next
.blei_data_seg:
    and     r10, r4, #2
    beqz    r10, .blei_badarg
    load.q  r10, 32(sp)
    bnez    r10, .blei_badarg
    store.q r23, 32(sp)
.blei_next:
    add     r23, r23, #56
    add     r24, r24, #1
    bra     .blei_ph_loop

.blei_ph_done:
    load.q  r23, 24(sp)
    beqz    r23, .blei_badarg
    load.q  r24, 32(sp)
    beqz    r24, .blei_badarg
    load.q  r4, 40(sp)
    beqz    r4, .blei_badarg
    load.l  r25, 32(r23)               ; code filesz
    load.l  r26, 40(r23)               ; code memsz
    add     r26, r26, #4095
    lsr     r26, r26, #12              ; code pages
    load.l  r27, 32(r24)               ; data filesz
    load.l  r28, 40(r24)               ; data memsz
    add     r28, r28, #4095
    lsr     r28, r28, #12              ; data pages

    add     r29, sp, #96
    move.l  r4, #M14_LDESC_MAGIC
    store.l r4, M14_LDESC_OFF_MAGIC(r29)
    move.l  r4, #M14_LDESC_VERSION
    store.l r4, M14_LDESC_OFF_VERSION(r29)
    move.l  r4, #M14_LDESC_SIZE
    store.l r4, M14_LDESC_OFF_SIZE(r29)
    move.l  r4, #2
    store.l r4, M14_LDESC_OFF_SEGCNT(r29)
    store.q r22, M14_LDESC_OFF_ENTRY(r29)
    move.l  r4, #1
    store.l r4, M14_LDESC_OFF_STACKPG(r29)
    add     r4, sp, #144
    store.q r4, M14_LDESC_OFF_SEGTBL(r29)

    add     r30, sp, #144
    load.l  r4, 8(r23)                 ; code p_offset
    load.q  r5, 0(sp)                  ; ELF ptr
    add     r5, r5, r4                 ; code src
    store.q r5, M14_LDSEG_OFF_SRCPTR(r30)
    store.q r25, M14_LDSEG_OFF_SRCSZ(r30)
    load.q  r5, 16(r23)                ; code vaddr
    store.q r5, M14_LDSEG_OFF_TARGET(r30)
    store.l r26, M14_LDSEG_OFF_PAGES(r30)
    move.l  r5, #5
    store.l r5, M14_LDSEG_OFF_FLAGS(r30)

    add     r30, sp, #176
    load.l  r4, 8(r24)                 ; data p_offset
    load.q  r5, 0(sp)
    add     r5, r5, r4                 ; data src
    store.q r5, M14_LDSEG_OFF_SRCPTR(r30)
    store.q r27, M14_LDSEG_OFF_SRCSZ(r30)
    load.q  r5, 16(r24)                ; data vaddr
    store.q r5, M14_LDSEG_OFF_TARGET(r30)
    store.l r28, M14_LDSEG_OFF_PAGES(r30)
    move.l  r5, #6
    store.l r5, M14_LDSEG_OFF_FLAGS(r30)

    add     r1, sp, #96
    move.l  r2, #M14_LDESC_SIZE
    move.l  r3, #KERN_PAGE_TABLE
    load.q  r4, 16(sp)
    jsr     exec_desc_launch_task      ; R1=task_id R2=slot R3=err
    move.q  r5, r2
    move.q  r2, r3
    move.q  r3, r5
    add     sp, sp, #256
    rts

.blei_badarg:
    move.q  r1, r0
    move.q  r2, #ERR_BADARG
    move.q  r3, r0
    add     sp, sp, #256
    rts

; kern_export_boot_manifest_to_dos:
;   Map the post-DOS staged manifest ELFs into dos.library's own address
;   space so DOS can build seglists from manifest-sourced bytes instead of
;   consulting the bundled legacy blobs. The staged manifest pages remain the
;   single source of truth; DOS receives read-only user mappings of them.
; Inputs:  R1 = dos.library internal task slot
; Outputs: R2 = ERR_OK / ERR_NOMEM / ERR_BADARG
; Clobbers: R1-R31
kern_export_boot_manifest_to_dos:
    sub     sp, sp, #64
    move.q  r20, r1                    ; target task slot

    move.q  r1, r20
    jsr     kern_task_layout_addr
    load.q  r30, KD_TASK_DATA_BASE(r1) ; dos.library data backing
    beqz    r30, .kebmtd_badarg
    load.q  r19, KD_TASK_LAYOUT_PT_BASE(r1)
    beqz    r19, .kebmtd_badarg

    add     r29, r30, #(prog_doslib_boot_export_rows - prog_doslib_data)
    move.l  r21, #0
.kebmtd_zero_rows:
    move.l  r22, #(DOS_BOOT_EXPORT_COUNT * DOS_BOOT_EXPORT_ROW_SZ / 8)
    bge     r21, r22, .kebmtd_loop_prep
    store.q r0, (r29)
    add     r29, r29, #8
    add     r21, r21, #1
    bra     .kebmtd_zero_rows

.kebmtd_loop_prep:
    add     r29, r30, #(prog_doslib_boot_export_rows - prog_doslib_data)
    move.l  r17, #(USER_DYN_END - 0x100000) ; next dos-private export VA
    move.l  r21, #KD_BOOT_MANIFEST_BOOT_COUNT
.kebmtd_loop:
    move.l  r22, #KD_BOOT_MANIFEST_COUNT
    bge     r21, r22, .kebmtd_ok

    move.l  r23, #KERN_DATA_BASE
    add     r23, r23, #KD_BOOT_MANIFEST_BASE
    move.l  r24, #KD_BOOT_MANIFEST_STRIDE
    mulu    r24, r21, r24
    add     r23, r23, r24              ; r23 = runtime manifest row

    load.l  r24, KD_BOOT_MANIFEST_ID(r23)
    load.q  r25, KD_BOOT_MANIFEST_PTR(r23)
    load.q  r26, KD_BOOT_MANIFEST_SIZE(r23)
    beqz    r25, .kebmtd_badarg
    beqz    r26, .kebmtd_badarg

    move.q  r27, r26
    add     r27, r27, #4095
    lsr     r27, r27, #12              ; pages
    beqz    r27, .kebmtd_badarg
    lsr     r18, r25, #12              ; staged base PPN

    move.l  r14, #KERN_DATA_BASE
    add     r14, r14, #KD_REGION_TABLE
    move.l  r11, #KD_REGION_TASK_SZ
    mulu    r11, r20, r11
    add     r14, r14, r11
    move.l  r15, #0
.kebmtd_find_region:
    move.l  r11, #KD_REGION_INLINE_MAX
    bge     r15, r11, .kebmtd_region_overflow
    move.l  r11, #KD_REGION_STRIDE
    mulu    r11, r15, r11
    add     r16, r14, r11
    load.b  r11, KD_REG_TYPE(r16)
    beqz    r11, .kebmtd_found_region
    add     r15, r15, #1
    bra     .kebmtd_find_region
.kebmtd_region_overflow:
    move.q  r1, r20
    jsr     kern_region_oflow_alloc_row
    bnez    r2, .kebmtd_nomem
    move.q  r16, r1
.kebmtd_found_region:
    move.q  r28, r16                   ; region row

    move.q  r1, r17
    move.q  r2, r27
    lsl     r2, r2, #12
    add     r2, r2, r1
    blt     r2, r1, .kebmtd_nomem
    move.l  r3, #USER_DYN_END
    bgt     r2, r3, .kebmtd_nomem

    move.q  r1, r19
    move.q  r2, r17
    move.q  r3, r18
    move.q  r4, r27
    move.l  r5, #0x13                  ; P|R|U (read-only user view of staged ELF)
    jsr     map_pages
    store.l r17, KD_REG_VA(r28)
    store.w r18, KD_REG_PPN(r28)
    store.w r27, KD_REG_PAGES(r28)
    move.b  r11, #REGION_IO
    store.b r11, KD_REG_TYPE(r28)
    store.b r0, KD_REG_FLAGS(r28)
    store.b r0, KD_REG_SHMID(r28)

    store.l r24, DOS_BOOT_EXPORT_ID(r29)
    store.q r17, DOS_BOOT_EXPORT_PTR(r29)
    store.q r26, DOS_BOOT_EXPORT_SIZE(r29)

    move.q  r1, r27
    lsl     r1, r1, #12
    add     r17, r17, r1
    add     r29, r29, #DOS_BOOT_EXPORT_ROW_SZ
    add     r21, r21, #1
    bra     .kebmtd_loop
.kebmtd_badarg:
    move.q  r2, #ERR_BADARG
    add     sp, sp, #64
    rts
.kebmtd_nomem:
    move.q  r2, #ERR_NOMEM
    add     sp, sp, #64
    rts
.kebmtd_ok:
    tlbflush
    move.q  r2, #ERR_OK
    add     sp, sp, #64
    rts

; ============================================================================
; load_program: Load a bundled IE64 program image into a free task slot.
; Input:  R7 = image_ptr (address of image in kernel memory)
;         R8 = image_size (total bytes: header + code + data)
;         R6 = startup flags (TASK_STARTUP_FLAG_*)
; Output: R1 = public task_id, R2 = ERR_OK, R3 = internal task slot
;         On failure: R1 = 0, R2 = ERR_BADARG or ERR_NOMEM
; Clobbers: R1-R9, R14-R27
; Must be called with kernel PT active (boot context).

load_program:
    move.q  r19, r6
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
    move.l  r12, #KERN_DATA_BASE
    load.q  r18, KD_TASKID_NEXT(r12)    ; r18 = public task id
    move.q  r14, r18
    add     r14, r14, #1
    store.q r14, KD_TASKID_NEXT(r12)

    ; --- Commit (M13 phase 2: allocate dynamic image/PT placement) ---
    move.q  r1, r26
    jsr     alloc_task_image_pages
    bnez    r2, .lp_nomem
    move.q  r23, r1                     ; code_base
    move.l  r25, #1
    move.q  r1, r25
    jsr     alloc_task_image_pages
    bnez    r2, .lp_fail_free_code
    move.q  r25, r1                     ; stack_base
    move.q  r1, r27
    jsr     alloc_task_image_pages
    bnez    r2, .lp_fail_free_stack
    move.q  r24, r1                     ; data_base
    move.l  r1, #1
    jsr     alloc_task_image_pages
    bnez    r2, .lp_fail_free_data
    move.q  r28, r1                     ; startup_base
    jsr     alloc_task_pt_block
    bnez    r2, .lp_fail_free_startup
    move.q  r30, r1                     ; pt_base

    ; 8. Zero child's code pages (code_pages * 4096 bytes)
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
    move.q  r1, r30
    move.q  r2, r23
    move.q  r3, r26
    move.q  r4, r25
    move.l  r5, #1
    move.q  r6, r24
    move.q  r7, r27
    move.q  r8, r28
    jsr     build_user_pt_dynamic

    ; 11b. Populate startup block in its dedicated page.
    move.q  r1, r28                     ; startup base
    move.q  r2, r18                     ; public task id
    move.q  r3, r19                     ; flags
    move.q  r4, r23                     ; code base
    move.q  r5, r26                     ; code pages
    move.q  r6, r25                     ; stack base
    move.l  r7, #1                      ; stack pages
    move.q  r8, r24                     ; data base
    move.q  r9, r27                     ; data pages
    jsr     write_startup_block

    ; Seed the top of the stack page with startup_base and data_base so
    ; programs can recover both without assuming stack/data adjacency.
    move.q  r14, r25
    add     r14, r14, #MMU_PAGE_SIZE
    sub     r14, r14, #16
    store.q r28, (r14)
    add     r14, r14, #8
    store.q r24, (r14)

    ; 12. Initialize TCB
    move.l  r12, #KERN_DATA_BASE
    lsl     r15, r22, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12               ; r15 = &TCB[task_id]

    ; PC = code_base
    store.q r23, KD_TASK_PC(r15)

    ; USP = stack_base + PAGE_SIZE
    move.q  r14, r25
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
    lsl     r16, r22, #3
    add     r16, r16, #KD_PTBR_BASE
    add     r16, r16, r12
    store.q r30, (r16)

    ; 13b. Record dynamic layout.
    move.q  r1, r22
    jsr     kern_task_layout_addr
    store.q r23, KD_TASK_CODE_BASE(r1)
    store.q r25, KD_TASK_STACK_BASE(r1)
    store.q r24, KD_TASK_DATA_BASE(r1)
    store.l r26, KD_TASK_CODE_PAGES(r1)
    move.l  r14, #1
    store.l r14, KD_TASK_STACK_PAGES(r1)
    store.l r27, KD_TASK_DATA_PAGES(r1)
    store.l r0, KD_TASK_LAYOUT_FLAGS(r1)
    store.q r28, KD_TASK_LAYOUT_STARTUP(r1)
    store.q r30, KD_TASK_LAYOUT_PT_BASE(r1)

    move.q  r1, r22
    jsr     kern_task_pubid_addr
    store.l r18, (r1)

    ; 14. Increment num_tasks
    load.q  r14, KD_NUM_TASKS(r12)
    add     r14, r14, #1
    store.q r14, KD_NUM_TASKS(r12)

    ; 15. Return success
    pop     r8
    pop     r7
    move.q  r1, r18                     ; R1 = public task id
    move.q  r3, r22                     ; R3 = internal task slot
    move.l  r2, #ERR_OK
    rts

.lp_badarg:
    pop     r8
    pop     r7
    move.q  r1, r0
    move.q  r3, r0
    move.l  r2, #ERR_BADARG
    rts

.lp_nomem:
    pop     r8
    pop     r7
    move.q  r1, r0
    move.q  r3, r0
    move.l  r2, #ERR_NOMEM
    rts

.lp_fail_free_startup:
    move.q  r1, r28
    move.l  r2, #1
    jsr     free_task_image_pages
.lp_fail_free_data:
    move.q  r1, r24
    move.q  r2, r27
    jsr     free_task_image_pages
.lp_fail_free_stack:
    move.q  r1, r25
    move.l  r2, #1
    jsr     free_task_image_pages
.lp_fail_free_code:
    move.q  r1, r23
    move.q  r2, r26
    jsr     free_task_image_pages
    pop     r8
    pop     r7
    move.q  r1, r0
    move.q  r3, r0
    move.l  r2, #ERR_NOMEM
    rts

; ============================================================================
; M14 phase 3: launch descriptor -> dynamic child task
; ============================================================================
; exec_desc_launch_task:
;   Validate a DOS-built launch descriptor under the caller PT and launch the
;   child directly from those segments. This preserves the original ELF entry
;   point and the recorded segment target VAs.
;
; In:
;   R1 = launch_desc_ptr (caller VA)
;   R2 = launch_desc_size
;   R3 = caller PTBR
;   R4 = startup flags
; Out:
;   R1 = public task id or 0
;   R2 = child slot or 0
;   R3 = ERR_OK / ERR_BADARG / ERR_NOMEM
; Clobbers: R5-R31
exec_desc_launch_task:
    sub     sp, sp, #144
    move.q  r24, r1                     ; desc_ptr
    move.q  r25, r2                     ; desc_size
    move.q  r26, r3                     ; caller PTBR
    store.q r4, 72(sp)                  ; startup flags
    move.l  r5, #0x30
    store.l r5, 140(sp)

    move.l  r5, #M14_LDESC_SIZE
    bne     r25, r5, .edlt_badarg_descsz
    move.q  r1, r24
    move.l  r2, #M14_LDESC_SIZE
    move.q  r3, r26
    jsr     validate_user_read_range
    bnez    r1, .edlt_badarg_descrange

    load.l  r5, M14_LDESC_OFF_MAGIC(r24)
    move.l  r6, #M14_LDESC_MAGIC
    bne     r5, r6, .edlt_badarg_magic
    load.l  r5, M14_LDESC_OFF_VERSION(r24)
    move.l  r6, #M14_LDESC_VERSION
    bne     r5, r6, .edlt_badarg_version
    load.l  r5, M14_LDESC_OFF_SIZE(r24)
    move.l  r6, #M14_LDESC_SIZE
    bne     r5, r6, .edlt_badarg_size
    load.l  r21, M14_LDESC_OFF_SEGCNT(r24)
    beqz    r21, .edlt_badarg_segcnt0
    move.l  r6, #DOS_SEGLIST_MAX_ENTRIES
    bgt     r21, r6, .edlt_badarg_segcntmax
    load.q  r22, M14_LDESC_OFF_ENTRY(r24)
    beqz    r22, .edlt_badarg_entry
    load.l  r23, M14_LDESC_OFF_STACKPG(r24)
    beqz    r23, .edlt_badarg_stackpg
    store.q r23, 64(sp)                 ; stack pages
    load.q  r20, M14_LDESC_OFF_SEGTBL(r24)
    beqz    r20, .edlt_badarg_segtbl

    move.q  r5, r21
    move.l  r6, #M14_LDESC_SEG_SIZE
    mulu    r5, r5, r6
    beqz    r5, .edlt_badarg_segtblsz
    move.q  r1, r20
    move.q  r2, r5
    move.q  r3, r26
    jsr     validate_user_read_range
    bnez    r1, .edlt_badarg_segtblrange
    move.l  r5, #0x31
    store.l r5, 140(sp)

    move.q  r27, r0                     ; code src
    move.q  r28, r0                     ; code size
    move.q  r29, r0                     ; code target
    move.q  r30, r0                     ; code pages
    move.q  r24, r0                     ; data src
    move.q  r18, r0                     ; data size
    move.q  r19, r0                     ; data target
    move.q  r17, r0                     ; data pages
    move.l  r16, #0                     ; code found
    move.l  r15, #0                     ; data found
    move.l  r14, #0                     ; index
.edlt_seg_loop:
    bge     r14, r21, .edlt_seg_done
    move.l  r5, #M14_LDESC_SEG_SIZE
    mulu    r6, r14, r5
    add     r6, r6, r20
    load.q  r7, M14_LDSEG_OFF_SRCPTR(r6)
    load.q  r8, M14_LDSEG_OFF_SRCSZ(r6)
    load.q  r9, M14_LDSEG_OFF_TARGET(r6)
    load.l  r10, M14_LDSEG_OFF_PAGES(r6)
    load.l  r11, M14_LDSEG_OFF_FLAGS(r6)
    beqz    r10, .edlt_badarg
    move.q  r12, r10
    lsl     r12, r12, #12
    blt     r12, r8, .edlt_badarg
    move.l  r13, #0xFFF
    and     r13, r9, r13
    bnez    r13, .edlt_badarg
    move.l  r13, #USER_IMAGE_BASE
    blt     r9, r13, .edlt_badarg
    add     r13, r9, r12
    blt     r13, r9, .edlt_badarg
    move.l  r12, #USER_IMAGE_END
    bgt     r13, r12, .edlt_badarg
    beqz    r8, .edlt_skip_src_validate
    move.q  r1, r7
    move.q  r2, r8
    move.q  r3, r26
    jsr     validate_user_read_range
    bnez    r1, .edlt_badarg
.edlt_skip_src_validate:
    move.q  r13, r11
    and     r13, r13, #4
    beqz    r13, .edlt_badarg
    move.q  r13, r11
    and     r13, r13, #1
    beqz    r13, .edlt_data_seg
    bnez    r16, .edlt_badarg
    move.l  r16, #1
    move.q  r27, r7
    move.q  r28, r8
    move.q  r29, r9
    move.q  r30, r10
    bra     .edlt_seg_next
.edlt_data_seg:
    move.q  r13, r11
    and     r13, r13, #2
    beqz    r13, .edlt_badarg
    bnez    r15, .edlt_badarg
    move.l  r15, #1
    move.q  r24, r7
    move.q  r18, r8
    move.q  r19, r9
    move.q  r17, r10
.edlt_seg_next:
    add     r14, r14, #1
    bra     .edlt_seg_loop
.edlt_seg_done:
    beqz    r16, .edlt_badarg
    beqz    r15, .edlt_badarg
    move.l  r5, #0x32
    store.l r5, 140(sp)
    store.q r27, 0(sp)                 ; code src
    store.q r28, 8(sp)                 ; code size
    store.q r29, 16(sp)                ; code target
    store.q r30, 24(sp)                ; code pages
    store.q r24, 32(sp)                ; data src
    store.q r18, 40(sp)                ; data size
    store.q r19, 48(sp)                ; data target
    store.q r17, 56(sp)                ; data pages
    move.l  r5, #0x33
    store.l r5, 140(sp)
    blt     r22, r29, .edlt_badarg
    move.q  r13, r30
    lsl     r13, r13, #12
    add     r13, r13, r29
    move.l  r5, #0x34
    store.l r5, 140(sp)
    bge     r22, r13, .edlt_badarg
    move.q  r13, r30
    lsl     r13, r13, #12
    add     r13, r13, r29              ; code end
    move.q  r11, r17
    lsl     r11, r11, #12
    add     r11, r11, r19              ; data end
    bge     r13, r11, .edlt_stack_target_ready
    move.q  r13, r11
.edlt_stack_target_ready:
    load.q  r11, 64(sp)                ; stack pages
    lsl     r11, r11, #12              ; stack bytes
    move.l  r5, #0x35
    store.l r5, 140(sp)
    add     r12, r13, r11              ; stack end
    blt     r12, r13, .edlt_badarg
    move.l  r11, #USER_IMAGE_END
    move.l  r5, #0x36
    store.l r5, 140(sp)
    bgt     r12, r11, .edlt_badarg
    store.q r13, 80(sp)                ; stack target base

    move.l  r13, #KERN_DATA_BASE
    load.q  r21, KD_TASKID_NEXT(r13)    ; public task id
    store.q r21, 128(sp)
    move.q  r15, r21
    add     r15, r15, #1
    store.q r15, KD_TASKID_NEXT(r13)

    move.l  r20, #0
.edlt_scan:
    move.l  r11, #MAX_TASKS
    bge     r20, r11, .edlt_nomem
    lsl     r15, r20, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r13
    load.b  r16, KD_TASK_STATE(r15)
    move.l  r11, #TASK_FREE
    beq     r16, r11, .edlt_found
    add     r20, r20, #1
    bra     .edlt_scan
.edlt_found:
    store.q r20, 136(sp)

    load.q  r1, 24(sp)                 ; code pages
    jsr     alloc_task_image_pages
    bnez    r2, .edlt_alloc_fail
    store.q r1, 88(sp)                 ; code backing

    load.q  r1, 56(sp)                 ; data pages
    jsr     alloc_task_image_pages
    bnez    r2, .edlt_fail_free_code
    store.q r1, 96(sp)                 ; data backing

    load.q  r1, 64(sp)                 ; stack pages
    jsr     alloc_task_image_pages
    bnez    r2, .edlt_fail_free_data
    store.q r1, 104(sp)                ; stack backing

    move.l  r1, #1
    jsr     alloc_task_image_pages
    bnez    r2, .edlt_fail_free_stack
    store.q r1, 112(sp)                ; startup base

    jsr     alloc_task_pt_block
    bnez    r2, .edlt_fail_free_startup
    store.q r1, 120(sp)                ; pt base

    load.q  r27, 0(sp)
    load.q  r28, 8(sp)
    load.q  r29, 16(sp)
    load.q  r30, 24(sp)
    load.q  r12, 32(sp)
    load.q  r18, 40(sp)
    load.q  r19, 48(sp)
    load.q  r17, 56(sp)
    load.q  r23, 64(sp)
    load.q  r14, 88(sp)
    load.q  r13, 96(sp)
    load.q  r16, 104(sp)
    load.q  r24, 112(sp)
    load.q  r25, 120(sp)
    load.q  r21, 128(sp)
    load.q  r20, 136(sp)

    move.l  r4, #0
.edlt_copy_code:
    bge     r4, r28, .edlt_copy_data
    add     r5, r27, r4
    load.b  r6, (r5)
    add     r5, r14, r4
    store.b r6, (r5)
    add     r4, r4, #1
    bra     .edlt_copy_code
.edlt_copy_data:
    move.l  r4, #0
.edlt_copy_data_loop:
    bge     r4, r18, .edlt_map
    add     r5, r12, r4
    load.b  r6, (r5)
    add     r5, r13, r4
    store.b r6, (r5)
    add     r4, r4, #1
    bra     .edlt_copy_data_loop
.edlt_map:
    move.l  r2, #KERN_PAGE_TABLE
    move.l  r4, #0
.edlt_pt_copy_kern:
    move.l  r6, #KERN_PAGES
    bge     r4, r6, .edlt_pt_copy_pool
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r25
    store.q r3, (r9)
    add     r4, r4, #1
    bra     .edlt_pt_copy_kern
.edlt_pt_copy_pool:
    move.l  r4, #ALLOC_POOL_BASE
    move.l  r6, #ALLOC_POOL_PAGES
    add     r6, r6, r4
.edlt_pt_copy_pool_loop:
    bge     r4, r6, .edlt_pt_copy_user
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r25
    store.q r3, (r9)
    add     r4, r4, #1
    bra     .edlt_pt_copy_pool_loop
.edlt_pt_copy_user:
    move.l  r4, #USER_CODE_VPN_BASE
    move.l  r6, #USER_PT_PAGE_BASE
.edlt_pt_copy_user_loop:
    bge     r4, r6, .edlt_pt_copy_userpt
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r25
    store.q r3, (r9)
    add     r4, r4, #1
    bra     .edlt_pt_copy_user_loop
.edlt_pt_copy_userpt:
    move.l  r4, #USER_PT_PAGE_BASE
    move.l  r6, #USER_PT_PAGE_END
.edlt_pt_copy_userpt_loop:
    bge     r4, r6, .edlt_pt_map_code
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r25
    store.q r3, (r9)
    add     r4, r4, #1
    bra     .edlt_pt_copy_userpt_loop

.edlt_pt_map_code:
    move.l  r8, #0
.edlt_pt_code_loop:
    bge     r8, r30, .edlt_pt_map_stack
    lsr     r9, r14, #12
    add     r9, r9, r8
    lsl     r10, r9, #13
    or      r10, r10, #(PTE_P | PTE_R | PTE_X | PTE_U)
    lsr     r11, r29, #12
    add     r11, r11, r8
    lsl     r11, r11, #3
    add     r11, r11, r25
    store.q r10, (r11)
    add     r8, r8, #1
    bra     .edlt_pt_code_loop

.edlt_pt_map_stack:
    move.l  r8, #0
.edlt_pt_stack_loop:
    bge     r8, r23, .edlt_pt_map_data
    lsr     r9, r16, #12
    add     r9, r9, r8
    lsl     r10, r9, #13
    or      r10, r10, #(PTE_P | PTE_R | PTE_W | PTE_U)
    load.q  r12, 80(sp)
    lsr     r11, r12, #12
    add     r11, r11, r8
    lsl     r11, r11, #3
    add     r11, r11, r25
    store.q r10, (r11)
    add     r8, r8, #1
    bra     .edlt_pt_stack_loop

.edlt_pt_map_data:
    move.l  r8, #0
.edlt_pt_data_loop:
    bge     r8, r17, .edlt_pt_map_startup
    lsr     r9, r13, #12
    add     r9, r9, r8
    lsl     r10, r9, #13
    or      r10, r10, #(PTE_P | PTE_R | PTE_W | PTE_U)
    lsr     r11, r19, #12
    add     r11, r11, r8
    lsl     r11, r11, #3
    add     r11, r11, r25
    store.q r10, (r11)
    add     r8, r8, #1
    bra     .edlt_pt_data_loop

.edlt_pt_map_startup:
    lsr     r9, r24, #12
    lsl     r10, r9, #13
    or      r10, r10, #(PTE_P | PTE_R | PTE_U)
    lsl     r11, r9, #3
    add     r11, r11, r25
    store.q r10, (r11)

    move.q  r1, r24
    move.q  r2, r21
    load.q  r3, 72(sp)
    move.q  r4, r29
    move.q  r5, r30
    load.q  r6, 80(sp)
    move.q  r7, r23
    move.q  r8, r19
    move.q  r9, r17
    jsr     write_startup_block

    move.q  r18, r13                    ; preserve data backing for layout row
    move.q  r15, r23
    lsl     r15, r15, #12
    add     r15, r15, r16
    sub     r15, r15, #16
    store.q r24, (r15)
    add     r15, r15, #8
    store.q r19, (r15)

    move.l  r13, #KERN_DATA_BASE
    lsl     r15, r20, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r13
    store.q r22, KD_TASK_PC(r15)
    load.q  r11, 80(sp)
    move.q  r12, r23
    lsl     r12, r12, #12
    add     r11, r11, r12
    store.q r11, KD_TASK_USP(r15)
    move.l  r11, #SIG_SYSTEM_MASK
    store.l r11, KD_TASK_SIG_ALLOC(r15)
    store.l r0, KD_TASK_SIG_WAIT(r15)
    store.l r0, KD_TASK_SIG_RECV(r15)
    store.b r0, KD_TASK_STATE(r15)
    move.b  r11, #WAITPORT_NONE
    store.b r11, KD_TASK_WAITPORT(r15)
    store.b r0, KD_TASK_GPR_SAVED(r15)

    lsl     r11, r20, #3
    add     r11, r11, #KD_PTBR_BASE
    add     r11, r11, r13
    store.q r25, (r11)

    move.q  r1, r20
    jsr     kern_task_layout_addr
    store.q r14, KD_TASK_CODE_BASE(r1)
    store.q r16, KD_TASK_STACK_BASE(r1)
    store.q r18, KD_TASK_DATA_BASE(r1)
    store.l r30, KD_TASK_CODE_PAGES(r1)
    store.l r23, KD_TASK_STACK_PAGES(r1)
    store.l r17, KD_TASK_DATA_PAGES(r1)
    store.l r0, KD_TASK_LAYOUT_FLAGS(r1)
    store.q r24, KD_TASK_LAYOUT_STARTUP(r1)
    store.q r25, KD_TASK_LAYOUT_PT_BASE(r1)

    move.q  r1, r20
    jsr     kern_task_pubid_addr
    store.l r21, (r1)

    load.q  r16, KD_NUM_TASKS(r13)
    add     r16, r16, #1
    store.q r16, KD_NUM_TASKS(r13)

    move.q  r1, r21
    move.q  r2, r20
    move.q  r3, #ERR_OK
    add     sp, sp, #144
    rts
.edlt_alloc_fail:
    move.l  r11, #ERR_BADARG
    beq     r2, r11, .edlt_badarg
    bra     .edlt_nomem
.edlt_fail_free_startup:
    load.q  r1, 112(sp)
    move.l  r2, #1
    jsr     free_task_image_pages
.edlt_fail_free_stack:
    load.q  r1, 104(sp)
    load.q  r2, 64(sp)
    jsr     free_task_image_pages
.edlt_fail_free_data:
    load.q  r1, 96(sp)
    load.q  r2, 56(sp)
    jsr     free_task_image_pages
.edlt_fail_free_code:
    load.q  r1, 88(sp)
    load.q  r2, 24(sp)
    jsr     free_task_image_pages
    bra     .edlt_nomem
.edlt_badarg:
    move.q  r1, r0
    move.q  r2, r0
    move.l  r3, #ERR_BADARG
    add     sp, sp, #144
    rts
.edlt_badarg_descsz:
    move.l  r5, #0x44
    store.l r5, 140(sp)
    bra     .edlt_badarg
.edlt_badarg_descrange:
    move.l  r5, #0x52
    store.l r5, 140(sp)
    bra     .edlt_badarg
.edlt_badarg_magic:
    move.l  r5, #0x4D
    store.l r5, 140(sp)
    bra     .edlt_badarg
.edlt_badarg_version:
    move.l  r5, #0x56
    store.l r5, 140(sp)
    bra     .edlt_badarg
.edlt_badarg_size:
    move.l  r5, #0x53
    store.l r5, 140(sp)
    bra     .edlt_badarg
.edlt_badarg_segcnt0:
    move.l  r5, #0x43
    store.l r5, 140(sp)
    bra     .edlt_badarg
.edlt_badarg_segcntmax:
    move.l  r5, #0x63
    store.l r5, 140(sp)
    bra     .edlt_badarg
.edlt_badarg_entry:
    move.l  r5, #0x45
    store.l r5, 140(sp)
    bra     .edlt_badarg
.edlt_badarg_stackpg:
    move.l  r5, #0x4B
    store.l r5, 140(sp)
    bra     .edlt_badarg
.edlt_badarg_segtbl:
    move.l  r5, #0x54
    store.l r5, 140(sp)
    bra     .edlt_badarg
.edlt_badarg_segtblsz:
    move.l  r5, #0x5A
    store.l r5, 140(sp)
    bra     .edlt_badarg
.edlt_badarg_segtblrange:
    move.l  r5, #0x55
    store.l r5, 140(sp)
    bra     .edlt_badarg
.edlt_nomem:
    move.q  r1, r0
    move.q  r2, r0
    move.l  r3, #ERR_NOMEM
    add     sp, sp, #144
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
;   4. Set every row's KD_GRANT_TASK_ID field to GRANT_TASK_FREE.
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

    ; Mark all 255 rows as free (KD_GRANT_TASK_ID = GRANT_TASK_FREE).
    ; Row 0 is at page+KD_GRANT_PAGE_HDR_SZ, row stride = KD_GRANT_ROW_SIZE.
    move.l  r4, #KD_GRANT_PAGE_HDR_SZ
    add     r4, r4, r3                  ; r4 = &row[0]
    move.l  r5, #0                      ; row index
    move.l  r6, #GRANT_TASK_FREE
.gcap_mark:
    move.l  r7, #KD_GRANT_ROWS_PER_PG
    bge     r5, r7, .gcap_mark_done
    store.l r6, KD_GRANT_TASK_ID(r4)
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
; Inputs:  R1 = target public task ID (u32)
;          R2 = region tag (low 32 bits used)
;          R3 = PPN low (inclusive, low 16 bits used)
;          R4 = PPN high (inclusive, low 16 bits used)
; Outputs: R1 = unused, R2 = ERR_OK or ERR_NOMEM
; Clobbers: R3..R19, R28, R29
;
; Walks every chain page; for each, scans rows 0..254 looking for one whose
; KD_GRANT_TASK_ID == GRANT_TASK_FREE. Writes the row in place. If no chain page has a
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
    load.l  r8, KD_GRANT_TASK_ID(r6)
    move.l  r9, #GRANT_TASK_FREE
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

    store.l r1, KD_GRANT_TASK_ID(r6)
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
; Inputs:  R1 = public task ID (u32)
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
    load.l  r12, KD_GRANT_TASK_ID(r10)
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
; rows whose manifest entry ID matches the just-launched bootstrap entry, and
; insert each as a live grant tied to the assigned task ID. Called from the
; bootstrap launch path immediately after the task returns.
; ----------------------------------------------------------------------------
; Inputs:  R1 = manifest entry ID
;          R2 = assigned task ID for that program
; Outputs: R2 = ERR_OK or ERR_NOMEM
; Clobbers: R3..R19, R20-R22, R28, R29, R30
;
; Strategy: copy the inputs into call-preserved registers (R20=manifest id,
; R21=task_id) and walk the table cursor in R22. Each iteration that
; matches calls kern_grant_create_row with R1=task_id, R2=tag, R3=ppn_lo,
; R4=ppn_hi. R20/R21/R22 are pushed/popped around the call because the
; helper documents clobbering R3..R19 + R28/R29 — R20+ are not in its
; clobber list but defensive save/restore makes the call site robust.
kern_bootstrap_grant_for_program:
    move.q  r20, r1                     ; r20 = manifest entry ID (preserved)
    move.q  r21, r2                     ; r21 = task_id (preserved)
    la      r22, bootstrap_grant_table  ; r22 = table cursor
.bgfp_loop:
    load.b  r3, BOOTSTRAP_GRANT_OFF_PROG_IDX(r22)
    and     r3, r3, #0xFF
    move.l  r4, #0xFF
    beq     r3, r4, .bgfp_done          ; sentinel = end of table
    bne     r3, r20, .bgfp_advance      ; row's manifest id != ours, skip
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
; Inputs:  R1 = public task_id (u32)
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
    load.l  r9, KD_GRANT_TASK_ID(r7)
    bne     r9, r6, .grft_row_skip
    ; Match — mark free and bump down the header TOTAL counter.
    move.l  r10, #GRANT_TASK_FREE
    store.l r10, KD_GRANT_TASK_ID(r7)
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

    move.l  r11, #SYS_BOOT_MANIFEST
    beq     r10, r11, .do_boot_manifest_launch

    move.l  r11, #SYS_BOOT_HOSTFS
    beq     r10, r11, .do_boot_hostfs

    move.l  r11, #SYS_BOOT_ELF_EXEC
    beq     r10, r11, .do_boot_elf_exec

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
    jsr     kern_current_public_task_id
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
    ; Resolve the public task ID to a live slot.
    jsr     kern_find_slot_for_public_id ; R1 = slot, R2 = found?
    beqz    r2, .signal_badarg

    ; Get target TCB
    move.l  r12, #KERN_DATA_BASE
    lsl     r15, r1, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12           ; R15 = &TCB[target]

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
    load.q  r13, (r12)                  ; current_task slot
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
    push    r2
    push    r3
    push    r4
    push    r5
    push    r6
    push    r21
    push    r24
    push    r25
    jsr     kern_current_public_task_id
    pop     r25
    pop     r24
    pop     r21
    pop     r6
    pop     r5
    pop     r4
    pop     r3
    pop     r2
    store.l r1, KD_MSG_SRC(r26)         ; mn_SrcTask = public task ID
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

    ; Reject source pointers outside the 32 MiB virtual address space before
    ; validate_user_read_range walks the caller PT.
    move.l  r11, #0x2000000
    bge     r24, r11, .ct_badarg

    ; Validate source_ptr range against the caller's current PT directly.
    mfcr    r28, cr0
    move.q  r1, r24
    move.q  r2, r25
    move.q  r3, r28
    jsr     validate_user_read_range
    bnez    r1, .ct_badarg

    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)                  ; current_task

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
    move.l  r12, #KERN_DATA_BASE
    load.q  r18, KD_TASKID_NEXT(r12)    ; r18 = public task id
    move.q  r14, r18
    add     r14, r14, #1
    store.q r14, KD_TASKID_NEXT(r12)

    ; Allocate the child's dynamic image/PT placement while staying on the
    ; caller's PT. The caller PT already carries supervisor-only mappings for
    ; the global image/PT windows, so the kernel can safely read a source_ptr
    ; from user-dynamic/shared mappings and write the child image/PT without
    ; losing visibility of caller-owned pages.
    move.l  r1, #1
    jsr     alloc_task_image_pages
    bnez    r2, .ct_nomem
    move.q  r21, r1                     ; child code base
    move.l  r1, #1
    jsr     alloc_task_image_pages
    bnez    r2, .ct_fail_free_code
    move.q  r23, r1                     ; child stack base
    move.l  r1, #1
    jsr     alloc_task_image_pages
    bnez    r2, .ct_fail_free_stack
    move.q  r22, r1                     ; child data base
    move.l  r1, #1
    jsr     alloc_task_image_pages
    bnez    r2, .ct_fail_free_data
    move.q  r27, r1                     ; child startup base
    jsr     alloc_task_pt_block
    bnez    r2, .ct_fail_free_startup
    move.q  r29, r1                     ; child PT base

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
    store.q r26, (r22)                  ; data[0] = arg0

    ; Build child's page table
    move.q  r1, r29
    move.q  r2, r21
    move.l  r3, #1
    move.q  r4, r23
    move.l  r5, #1
    move.q  r6, r22
    move.l  r7, #1
    move.q  r8, r27
    push    r20
    jsr     build_user_pt_dynamic
    pop     r20

    ; Populate startup block in the child's dedicated startup page.
    move.q  r1, r27                     ; startup base
    move.q  r2, r18                     ; public task id
    move.l  r3, #TASK_STARTUP_FLAG_CREATE
    move.q  r4, r21                     ; code base
    move.l  r5, #1                      ; code pages
    move.q  r6, r23                     ; stack base
    move.l  r7, #1                      ; stack pages
    move.q  r8, r22                     ; data base
    move.l  r9, #1                      ; data pages
    jsr     write_startup_block

    ; Seed the top of the stack page with startup_base and data_base so user
    ; preambles can recover both without assuming stack/data adjacency.
    move.q  r14, r23
    add     r14, r14, #MMU_PAGE_SIZE
    sub     r14, r14, #16
    store.q r27, (r14)
    add     r14, r14, #8
    store.q r22, (r14)

    ; Init child TCB
    move.l  r12, #KERN_DATA_BASE
    lsl     r15, r20, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12               ; r15 = &TCB[child]

    ; PC = code base
    store.q r21, KD_TASK_PC(r15)

    ; USP = stack base + page size
    move.q  r14, r23
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

    ; Set PTBR[child_id]
    lsl     r16, r20, #3
    add     r16, r16, #KD_PTBR_BASE
    add     r16, r16, r12
    store.q r29, (r16)

    ; Record dynamic layout.
    move.q  r1, r20
    jsr     kern_task_layout_addr
    store.q r21, KD_TASK_CODE_BASE(r1)
    store.q r23, KD_TASK_STACK_BASE(r1)
    store.q r22, KD_TASK_DATA_BASE(r1)
    move.l  r14, #1
    store.l r14, KD_TASK_CODE_PAGES(r1)
    store.l r14, KD_TASK_STACK_PAGES(r1)
    store.l r14, KD_TASK_DATA_PAGES(r1)
    store.l r0, KD_TASK_LAYOUT_FLAGS(r1)
    store.q r27, KD_TASK_LAYOUT_STARTUP(r1)
    store.q r29, KD_TASK_LAYOUT_PT_BASE(r1)

    move.q  r1, r20
    jsr     kern_task_pubid_addr
    store.l r18, (r1)

    ; Return public task id in R1, ERR_OK in R2
    move.q  r1, r18
    move.q  r2, #ERR_OK
    eret

.ct_badarg:
    move.q  r2, #ERR_BADARG
    eret
.ct_nomem:
    move.q  r2, #ERR_NOMEM
    eret
.ct_fail_free_startup:
    move.q  r1, r27
    move.l  r2, #1
    jsr     free_task_image_pages
.ct_fail_free_data:
    move.q  r1, r22
    move.l  r2, #1
    jsr     free_task_image_pages
.ct_fail_free_stack:
    move.q  r1, r23
    move.l  r2, #1
    jsr     free_task_image_pages
.ct_fail_free_code:
    move.q  r1, r21
    move.l  r2, #1
    jsr     free_task_image_pages
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
    move.l  r17, #REGION_IO
    beq     r11, r17, .fm_cleanup_region
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
    ; r1 = current public task ID, r2 = req PPN low, r3 = req page count
    move.l  r12, #KERN_DATA_BASE
    jsr     kern_current_public_task_id ; R1 = current public task ID
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
;     Effect:  if KD_HWRES_TASK == HWRES_TASK_FREE, set it to the current
;              public task ID and return ERR_OK. Otherwise return ERR_EXISTS.
;     Outputs: R1 = unused, R2 = err
;
;   HWRES_CREATE (1):
;     Inputs:  R1 = target public task ID
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
;     Inputs:  R1 = target public task ID
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
    ; If KD_HWRES_TASK is still the unclaimed sentinel, set it to the
    ; current public task ID and return OK. Otherwise return EXISTS.
    move.l  r12, #KERN_DATA_BASE
    load.l  r13, KD_HWRES_TASK(r12)
    move.l  r14, #HWRES_TASK_FREE
    bne     r13, r14, .hwres_exists
    jsr     kern_current_public_task_id ; R1 = current public task ID
    store.l r1, KD_HWRES_TASK(r12)
    move.q  r1, #0
    move.q  r2, #ERR_OK
    eret

.hwres_create:
    ; Guard: current_public_task == KD_HWRES_TASK
    move.q  r20, r1                     ; preserve target public task ID
    move.q  r21, r2                     ; preserve tag
    move.q  r22, r3                     ; preserve ppn_lo
    move.q  r23, r4                     ; preserve ppn_hi
    move.l  r12, #KERN_DATA_BASE
    jsr     kern_current_public_task_id ; R1 = current public task ID
    move.q  r13, r1
    load.l  r14, KD_HWRES_TASK(r12)
    bne     r13, r14, .hwres_perm
    ; Sanity-check inputs lightly:
    ;   target public task id must resolve to a live slot
    ;   ppn_lo, ppn_hi must satisfy ppn_lo <= ppn_hi
    move.q  r1, r20
    jsr     kern_find_slot_for_public_id ; R1=slot, R2=1 if found
    beqz    r2, .hwres_badarg
    bgt     r22, r23, .hwres_badarg
    bltz    r22, .hwres_badarg
    bltz    r23, .hwres_badarg
    ; r1=task_id, r2=tag, r3=ppn_lo, r4=ppn_hi already in the right
    ; registers for kern_grant_create_row.
    move.q  r1, r20
    move.q  r2, r21
    move.q  r3, r22
    move.q  r4, r23
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
    move.q  r20, r1                     ; preserve target public task ID
    move.l  r12, #KERN_DATA_BASE
    jsr     kern_current_public_task_id ; R1 = current public task ID
    move.q  r13, r1
    load.l  r14, KD_HWRES_TASK(r12)
    bne     r13, r14, .hwres_perm
    move.q  r1, r20
    jsr     kern_find_slot_for_public_id
    beqz    r2, .hwres_alive_no
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

; --- Bootstrap hostfs bridge (M15.2 phase 2, internal) ---
; ABI:
;   R1 = verb
;   R2 = arg1
;   R3 = arg2
;   R4 = arg3
;   R5 = arg4
; Returns:
;   R1 = result1
;   R2 = error
;   R3 = result2
.do_boot_hostfs:
    move.q  r20, r1
    move.q  r21, r2
    move.q  r22, r3
    move.q  r23, r4
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, KD_CURRENT_TASK(r12)
    move.q  r15, r13
    load.l  r11, KD_DOSLIB_PUBID(r12)
    move.l  r14, #TASK_PUBID_FREE
    beqz    r13, .dbhf_bootstrap
    beq     r11, r14, .dbhf_perm
    move.q  r1, r11
    jsr     kern_find_slot_for_public_id ; R1 = dos.library slot, R2 = found
    beqz    r2, .dbhf_perm
    bne     r1, r13, .dbhf_perm
    move.q  r15, r1
    bra     .dbhf_allowed
.dbhf_bootstrap:
    beq     r11, r14, .dbhf_allowed
    bra     .dbhf_perm
.dbhf_allowed:
    la      r24, BOOT_HOSTFS_BASE
    store.l r21, BOOT_HOSTFS_ARG1-BOOT_HOSTFS_BASE(r24)
    store.l r22, BOOT_HOSTFS_ARG2-BOOT_HOSTFS_BASE(r24)
    store.l r23, BOOT_HOSTFS_ARG3-BOOT_HOSTFS_BASE(r24)
    store.l r15, BOOT_HOSTFS_ARG4-BOOT_HOSTFS_BASE(r24)
    store.l r20, BOOT_HOSTFS_CMD-BOOT_HOSTFS_BASE(r24)
    load.l  r1, BOOT_HOSTFS_RES1-BOOT_HOSTFS_BASE(r24)
    load.l  r3, BOOT_HOSTFS_RES2-BOOT_HOSTFS_BASE(r24)
    load.l  r2, BOOT_HOSTFS_ERR-BOOT_HOSTFS_BASE(r24)
    eret
.dbhf_perm:
    move.q  r1, #0
    move.q  r2, #ERR_PERM
    move.q  r3, #0
    eret

; --- Trusted ELF exec bridge (M15.2 phase 5, internal) ---
; ABI:
;   R1 = caller-supplied ELF image ptr
;   R2 = image size
;   R3 = args ptr
;   R4 = args len
; Returns:
;   R1 = task_id
;   R2 = err
.do_boot_elf_exec:
    sub     sp, sp, #64
    move.q  r24, r1
    move.q  r25, r2
    move.q  r26, r3
    move.q  r27, r4
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, KD_CURRENT_TASK(r12)
    load.l  r11, KD_DOSLIB_PUBID(r12)
    move.l  r14, #TASK_PUBID_FREE
    beq     r11, r14, .dbefe_perm
    move.q  r1, r11
    jsr     kern_find_slot_for_public_id
    beqz    r2, .dbefe_perm
    bne     r1, r13, .dbefe_perm

    lsl     r11, r13, #3
    add     r11, r11, #KD_PTBR_BASE
    add     r11, r11, r12
    load.q  r28, (r11)
    store.q r28, 0(sp)                 ; caller PTBR
    store.q r13, 8(sp)                 ; caller slot
    store.q r26, 48(sp)                ; args ptr
    store.q r27, 56(sp)                ; args len

    beqz    r24, .dbefe_badarg
    beqz    r25, .dbefe_badarg
    move.q  r1, r24
    move.q  r2, r25
    move.q  r3, r28
    jsr     validate_user_read_range
    bnez    r1, .dbefe_badarg

    move.l  r11, #DATA_ARGS_MAX
    bgt     r27, r11, .dbefe_badarg
    beqz    r27, .dbefe_args_valid
    beqz    r26, .dbefe_args_valid
    add     r11, r26, r27
    blt     r11, r26, .dbefe_badarg
    move.q  r1, r26
    move.q  r2, r27
    move.q  r3, r28
    jsr     validate_user_read_range
    bnez    r1, .dbefe_badarg
.dbefe_args_valid:
    add     r11, r25, #0xFFF
    lsr     r20, r11, #12
    beqz    r20, .dbefe_badarg
    store.q r20, 16(sp)                ; temp page count

    mfcr    r19, cr0
    move.l  r11, #KERN_PAGE_TABLE
    mtcr    cr0, r11
    tlbflush

    move.q  r1, r20
    jsr     alloc_pages
    bnez    r2, .dbefe_nomem_restore
    store.q r1, 24(sp)                 ; temp base PPN

    load.q  r1, 8(sp)
    move.q  r2, r20
    jsr     find_free_va
    bnez    r2, .dbefe_free_pages_restore
    store.q r1, 32(sp)                 ; temp VA in caller PT

    load.q  r1, 0(sp)                  ; caller PTBR
    load.q  r2, 32(sp)                 ; temp VA
    load.q  r3, 24(sp)                 ; temp base PPN
    load.q  r4, 16(sp)                 ; temp page count
    move.l  r5, #0x17                  ; P|R|W|U
    jsr     map_pages

    mtcr    cr0, r19
    tlbflush

    move.q  r21, r24                   ; src user ptr
    load.q  r22, 32(sp)                ; dst temp VA
    move.q  r23, r25                   ; bytes remaining
.dbefe_copy_loop:
    beqz    r23, .dbefe_copy_done
    load.b  r11, (r21)
    store.b r11, (r22)
    add     r21, r21, #1
    add     r22, r22, #1
    sub     r23, r23, #1
    bra     .dbefe_copy_loop
.dbefe_copy_done:
    move.l  r11, #KERN_PAGE_TABLE
    mtcr    cr0, r11
    tlbflush
    load.q  r1, 24(sp)
    lsl     r1, r1, #12                ; kernel ptr to copied image
    move.q  r2, r25
    move.l  r3, #TASK_STARTUP_FLAG_EXEC
    jsr     boot_load_elf_image        ; R1=task_id R2=err R3=slot
    move.q  r22, r1
    move.q  r23, r3
    move.q  r29, r2

    load.q  r1, 0(sp)                  ; caller PTBR
    load.q  r2, 32(sp)                 ; temp VA
    load.q  r3, 16(sp)                 ; temp page count
    jsr     unmap_pages
    load.q  r1, 24(sp)                 ; temp base PPN
    load.q  r2, 16(sp)
    jsr     free_pages

    load.q  r26, 48(sp)
    load.q  r27, 56(sp)
    load.q  r11, 0(sp)
    mtcr    cr0, r11
    tlbflush
    bnez    r29, .dbefe_fail

    beqz    r27, .dbefe_ok
    beqz    r26, .dbefe_ok
    move.q  r1, r23
    jsr     kern_task_layout_addr
    load.q  r15, KD_TASK_DATA_BASE(r1)
    add     r15, r15, #DATA_ARGS_OFFSET
    move.l  r4, #0
.dbefe_copy_args:
    bge     r4, r27, .dbefe_term_args
    add     r5, r26, r4
    load.b  r6, (r5)
    add     r5, r15, r4
    store.b r6, (r5)
    add     r4, r4, #1
    bra     .dbefe_copy_args
.dbefe_term_args:
    add     r5, r15, r27
    store.b r0, (r5)
.dbefe_ok:
    move.q  r1, r22
    move.q  r2, #ERR_OK
    add     sp, sp, #64
    eret
.dbefe_fail:
    move.q  r1, #0
    move.q  r2, r29
    add     sp, sp, #64
    eret
.dbefe_badarg:
    move.q  r1, #0
    move.q  r2, #ERR_BADARG
    add     sp, sp, #64
    eret
.dbefe_perm:
    move.q  r1, #0
    move.q  r2, #ERR_PERM
    add     sp, sp, #64
    eret
.dbefe_nomem_restore:
    load.q  r19, 0(sp)
    mtcr    cr0, r19
    tlbflush
    move.q  r1, #0
    move.q  r2, #ERR_NOMEM
    add     sp, sp, #64
    eret
.dbefe_free_pages_restore:
    move.q  r29, r2
    load.q  r1, 24(sp)
    load.q  r2, 16(sp)
    jsr     free_pages
    load.q  r19, 0(sp)
    mtcr    cr0, r19
    tlbflush
    move.q  r1, #0
    move.q  r2, r29
    add     sp, sp, #64
    eret

; --- Boot manifest launch (M14.1 internal) ---
; R1 = manifest entry ID, R2 = args_ptr, R3 = args_len
.do_boot_manifest_launch:
    move.q  r20, r1                    ; preserve requested manifest ID
    sub     sp, sp, #32
    store.q r2, (sp)                   ; saved args_ptr
    store.q r3, 8(sp)                  ; saved args_len
    move.q  r26, r2                    ; preserve args_ptr
    move.q  r27, r3                    ; preserve args_len
    move.l  r11, #DATA_ARGS_MAX
    bgt     r27, r11, .dbml_badarg_noswitch
    beqz    r27, .dbml_args_valid
    beqz    r26, .dbml_args_valid
    add     r11, r26, r27
    blt     r11, r26, .dbml_badarg_noswitch
    mfcr    r28, cr0                   ; caller PTBR
    move.q  r1, r26
    move.q  r2, r27
    move.q  r3, r28
    jsr     validate_user_read_range
    bnez    r1, .dbml_badarg_noswitch
.dbml_args_valid:
    jsr     kern_current_public_task_id ; R1 = current public task ID
    move.l  r12, #KERN_DATA_BASE
    load.l  r11, KD_DOSLIB_PUBID(r12)
    bne     r1, r11, .dbml_perm
    move.q  r1, r20
    mfcr    r19, cr0
    store.q r19, 16(sp)
    move.l  r11, #KERN_PAGE_TABLE
    mtcr    cr0, r11
    tlbflush
    move.q  r20, r1
    move.l  r21, #0
.dbml_find:
    move.l  r22, #KD_BOOT_MANIFEST_COUNT
    bge     r21, r22, .dbml_badarg
    move.l  r23, #KERN_DATA_BASE
    add     r23, r23, #KD_BOOT_MANIFEST_BASE
    move.l  r24, #KD_BOOT_MANIFEST_STRIDE
    mulu    r24, r21, r24
    add     r23, r23, r24
    load.l  r24, KD_BOOT_MANIFEST_ID(r23)
    bne     r24, r20, .dbml_next
    load.q  r7, KD_BOOT_MANIFEST_PTR(r23)
    load.q  r8, KD_BOOT_MANIFEST_SIZE(r23)
    beqz    r7, .dbml_badarg
    beqz    r8, .dbml_badarg
    move.q  r1, r7
    move.q  r2, r8
    move.l  r3, #TASK_STARTUP_FLAG_BOOT
    jsr     boot_load_elf_image        ; R1=task_id R2=err R3=slot
    bra     .dbml_done
.dbml_next:
    add     r21, r21, #1
    bra     .dbml_find
.dbml_badarg:
    move.q  r1, #0
    move.q  r2, #ERR_BADARG
.dbml_done:
    move.q  r22, r1                    ; preserve task_id
    move.q  r24, r2                    ; preserve err
    move.q  r23, r3                    ; preserve child slot
    load.q  r26, (sp)
    load.q  r27, 8(sp)
    load.q  r19, 16(sp)
    add     sp, sp, #32
    mtcr    cr0, r19
    tlbflush
    bnez    r24, .dbml_ret
    beqz    r27, .dbml_ret
    beqz    r26, .dbml_ret
    move.q  r1, r23
    jsr     kern_task_layout_addr
    load.q  r15, KD_TASK_DATA_BASE(r1)
    add     r15, r15, #DATA_ARGS_OFFSET
    move.l  r4, #0
.dbml_copy_args:
    bge     r4, r27, .dbml_args_term
    add     r5, r26, r4
    load.b  r6, (r5)
    add     r5, r15, r4
    store.b r6, (r5)
    add     r4, r4, #1
    bra     .dbml_copy_args
.dbml_args_term:
    add     r5, r15, r27
    store.b r0, (r5)
.dbml_ret:
    move.q  r1, r22
    move.q  r2, r24
    eret
.dbml_perm:
    move.q  r1, #0
    move.q  r2, #ERR_PERM
    add     sp, sp, #32
    eret
.dbml_badarg_noswitch:
    move.q  r1, #0
    move.q  r2, #ERR_BADARG
    add     sp, sp, #32
    eret

; ============================================================================
; SYS_EXEC_PROGRAM (M14.2 phase 1) — Launch from a user-space descriptor only
; ============================================================================
; ABI: R1 = launch_desc_ptr (must be user VA, >= USER_CODE_BASE)
;      R2 = launch_desc_size, R3 = args_ptr, R4 = args_len
; Returns: R1 = new task_id, R2 = error
;
; Runs entirely under the caller's PT (no PT switching).
; Caller's PT has: user-accessible mappings for caller's own pages (incl.
; AllocMem'd storage), and supervisor-only mappings for all task slot pages.
;
; M11.6 removed the legacy `R1 < USER_CODE_BASE` built-in-program-table index
; branch. M14.2 phase 1 removes the remaining flat IE64PROG launch ABI too, so
; the only path through this handler is the validated launch-descriptor ABI.
.do_exec_program:
    move.l  r9, #0x61
    ; M11.6: reject any R1 below USER_CODE_BASE — the legacy index path is gone.
    move.l  r11, #USER_CODE_BASE
    blt     r1, r11, .ep_badarg_norestore

    move.q  r24, r1                     ; r24 = image_ptr (user VA)
    move.q  r25, r2                     ; r25 = image_size
    move.q  r26, r3                     ; r26 = args_ptr
    move.q  r27, r4                     ; r27 = args_len

    ; 1. Validate args_len
    move.l  r11, #DATA_ARGS_MAX
    bgt     r27, r11, .ep_badarg_norestore
    move.l  r9, #0x62

    ; 2. Validate the first 8 bytes so the descriptor magic/version can be read.
    move.l  r12, #KERN_DATA_BASE
    load.q  r28, KD_CURRENT_TASK(r12)   ; r28 = caller slot
    lsl     r11, r28, #3
    add     r11, r11, #KD_PTBR_BASE
    add     r11, r11, r12
    load.q  r28, (r11)                  ; r28 = caller PTBR
    move.q  r1, r24
    move.l  r2, #8
    move.q  r3, r28
    jsr     validate_user_read_range
    bnez    r1, .ep_badarg_norestore
    move.l  r9, #0x63

    load.l  r11, (r24)
    move.l  r12, #M14_LDESC_MAGIC
    bne     r11, r12, .ep_badarg_norestore
    move.l  r9, #0x64
    load.l  r11, 4(r24)
    move.l  r12, #M14_LDESC_VERSION
    bne     r11, r12, .ep_badarg_norestore
    move.l  r9, #0x65
    bra     .ep_desc_path

.ep_desc_path:
    ; Descriptor-mode transition path (M14 phase 3). Launch directly from the
    ; DOS-built descriptor so ELF entry points and target VAs are preserved.
    push    r28
    push    r26
    push    r27
    move.q  r1, r24
    move.q  r2, r25
    move.q  r3, r28
    move.l  r4, #TASK_STARTUP_FLAG_EXEC
    jsr     exec_desc_launch_task       ; R1=task_id, R2=slot, R3=err
    pop     r27
    pop     r26
    pop     r28
    bnez    r3, .ep_desc_fail
    move.q  r22, r1                     ; task id
    move.q  r23, r2                     ; child slot

    ; Validate args range if present (same contract as the flat-image path).
    beqz    r27, .ep_desc_args_valid
    beqz    r26, .ep_desc_args_valid
    add     r11, r26, r27
    blt     r11, r26, .ep_desc_cleanup_badarg
    move.q  r1, r26
    move.q  r2, r27
    move.q  r3, r28
    jsr     validate_user_read_range
    bnez    r1, .ep_desc_cleanup_badarg
.ep_desc_args_valid:
    beqz    r27, .ep_desc_no_args
    beqz    r26, .ep_desc_no_args

    move.q  r1, r23
    jsr     kern_task_layout_addr
    load.q  r15, KD_TASK_DATA_BASE(r1)
    add     r15, r15, #DATA_ARGS_OFFSET
    move.l  r4, #0
.ep_desc_copy_args:
    bge     r4, r27, .ep_desc_args_term
    add     r5, r26, r4
    load.b  r6, (r5)
    add     r5, r15, r4
    store.b r6, (r5)
    add     r4, r4, #1
    bra     .ep_desc_copy_args
.ep_desc_args_term:
    add     r5, r15, r27
    store.b r0, (r5)
    bra     .ep_desc_no_args

.ep_desc_no_args:
    move.q  r1, r22
    move.q  r2, #ERR_OK
    eret

.ep_desc_cleanup_badarg:
    move.q  r1, r22
    jsr     kern_find_slot_for_public_id
    beqz    r2, .ep_badarg_norestore
    mfcr    r19, cr0
    move.l  r11, #KERN_PAGE_TABLE
    mtcr    cr0, r11
    tlbflush
    move.q  r13, r1
    jsr     kill_task_cleanup
    mtcr    cr0, r19
    tlbflush
    bra     .ep_badarg_norestore

.ep_desc_fail:
    move.q  r1, #0
    move.q  r2, r3
    eret

.ep_new_fail:
    move.q  r1, #0
    eret

.ep_badarg_norestore:
    move.q  r1, #0
    move.q  r2, #ERR_BADARG
    eret

; ============================================================================
; Permission-aware user-range validators.
; Input:  R1 = start_va, R2 = byte_count, R3 = PTBR (page table base)
; Output: R1 = 0 (OK) or 1 (BADARG)
; Clobbers: R1-R6
; ============================================================================
validate_user_read_range:
    move.l  r4, #(PTE_P | PTE_R | PTE_U)
    bra     validate_user_range_mask

validate_user_write_range:
    move.l  r4, #(PTE_P | PTE_R | PTE_W | PTE_U)
    bra     validate_user_range_mask

validate_user_exec_range:
    move.l  r4, #(PTE_P | PTE_R | PTE_X | PTE_U)
    bra     validate_user_range_mask

validate_user_range_mask:
    beqz    r2, .vur_ok
    move.l  r6, #0x2000000
    bge     r1, r6, .vur_fail
    sub     r6, r6, r2
    bgt     r1, r6, .vur_fail
    move.l  r6, #KERN_PAGE_TABLE
    bne     r3, r6, .vur_user_pt
    move.l  r6, #PTE_U
    not     r6, r6
    and     r4, r4, r6                 ; kernel PT bootstrap ranges are supervisor-mapped
.vur_user_pt:
    lsr     r5, r1, #12                 ; start VPN
    add     r6, r1, r2
    sub     r6, r6, #1
    lsr     r6, r6, #12                 ; end VPN (inclusive)
.vur_check:
    bgt     r5, r6, .vur_ok
    lsl     r1, r5, #3
    add     r1, r1, r3
    load.q  r1, (r1)                    ; PTE
    and     r1, r1, r4
    bne     r1, r4, .vur_fail
    add     r5, r5, #1
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
    ; (a) Walk the grant chain and free every row whose public task_id
    ;     matches the exiting task.
    move.q  r1, r13
    jsr     kern_task_pubid_addr
    load.l  r15, (r1)                   ; r15 = exiting public task ID
    move.q  r1, r15
    jsr     kern_grant_release_for_task
    ;
    ; (b) If the exiting task IS the broker (KD_HWRES_TASK == public_id),
    ;     clear the broker identity. After this, a fresh task can claim
    ;     broker identity via SYS_HWRES_OP/HWRES_BECOME without inheriting
    ;     the dead broker's privilege.
    move.l  r12, #KERN_DATA_BASE
    load.l  r14, KD_HWRES_TASK(r12)
    bne     r14, r15, .ktc_hwres_clear_pubid
    move.l  r14, #HWRES_TASK_FREE
    store.l r14, KD_HWRES_TASK(r12)
.ktc_hwres_clear_pubid:
    load.l  r14, KD_DOSLIB_PUBID(r12)
    bne     r14, r15, .ktc_doslib_clear_pubid
    move.l  r14, #TASK_PUBID_FREE
    store.l r14, KD_DOSLIB_PUBID(r12)
.ktc_doslib_clear_pubid:
    move.q  r1, r13
    jsr     kern_task_pubid_addr
    move.l  r14, #TASK_PUBID_FREE
    store.l r14, (r1)
.ktc_hwres_done:
    ; ----------------------------------------------------------------
    ; M13 phase 2: free the task's dynamic code/stack/data pages and PT
    ; block, then clear the layout row and PTBR slot.
    ; ----------------------------------------------------------------
    move.q  r1, r13
    jsr     kern_task_layout_addr
    move.q  r20, r1
    load.q  r1, KD_TASK_CODE_BASE(r20)
    load.l  r2, KD_TASK_CODE_PAGES(r20)
    jsr     free_task_image_pages
    load.q  r1, KD_TASK_STACK_BASE(r20)
    load.l  r2, KD_TASK_STACK_PAGES(r20)
    jsr     free_task_image_pages
    load.q  r1, KD_TASK_DATA_BASE(r20)
    load.l  r2, KD_TASK_DATA_PAGES(r20)
    jsr     free_task_image_pages
    load.q  r1, KD_TASK_LAYOUT_STARTUP(r20)
    move.l  r2, #1
    jsr     free_task_image_pages
    load.q  r1, KD_TASK_LAYOUT_PT_BASE(r20)
    jsr     free_task_pt_block
    store.q r0, KD_TASK_CODE_BASE(r20)
    store.q r0, KD_TASK_STACK_BASE(r20)
    store.q r0, KD_TASK_DATA_BASE(r20)
    store.l r0, KD_TASK_CODE_PAGES(r20)
    store.l r0, KD_TASK_STACK_PAGES(r20)
    store.l r0, KD_TASK_DATA_PAGES(r20)
    store.l r0, KD_TASK_LAYOUT_FLAGS(r20)
    store.q r0, KD_TASK_LAYOUT_STARTUP(r20)
    store.q r0, KD_TASK_LAYOUT_PT_BASE(r20)
    lsl     r21, r13, #3
    add     r21, r21, #KD_PTBR_BASE
    add     r21, r21, r12
    store.q r0, (r21)
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
; Immutable list of (manifest_id, region_tag, ppn_lo, ppn_hi) rows that the
; boot-load path translates into live grant entries after each bootstrap
; launch returns its assigned task ID. This is the only non-broker producer
; of grants and exists solely to break the chicken-and-egg of console.handler
; needing serial-port MMIO before hardware.resource is alive (hardware.resource
; is started by Startup-Sequence, which Shell executes, which depends on
; dos.library, which depends on console.handler).
;
; M14.1 ships exactly one bootstrap-grant row: console.handler (manifest ID
; 10) gets a 'CHIP' grant for PPN 0xF0 (the chip register page that holds
; TERM_*, SCAN_*, MOUSE_*, video MMIO). Later services still obtain grants
; through hardware.resource after it is online.
;
; Row layout (16 bytes each, BOOTSTRAP_GRANT_ROW_SZ):
;   +0:  manifest entry ID (1 byte) + 3 bytes padding
;   +4:  region tag (4 bytes, little-endian uint32)
;   +8:  ppn_lo (2 bytes, inclusive)
;   +10: ppn_hi (2 bytes, inclusive)
;   +12: reserved (4 bytes)
;
; Sentinel row: manifest entry ID = 0xFF.
include "boot/manifest_seed.s"

; ============================================================================
; Legacy Inline Program Sources
; ============================================================================
; These inline images are no longer executable runtime artifacts in M14.2.
; Boot and DOS seeding use the canonical embedded ELF blobs above. The old
; source bodies remain only as dormant byte templates for low-level tests and
; source archaeology, with inert headers so the shipped kernel image no longer
; carries valid bundled IE64PROG binaries.
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

include "handler/console_handler.s"

; ---------------------------------------------------------------------------
; dos.library — RAM: filesystem service (M9)
; ---------------------------------------------------------------------------
; Amiga-style dos.library: in-memory filesystem with Open/Read/Write/Close/Dir.
; Registers as "dos.library" public port. Discovers console.handler via
; OpenLibrary. Handles DOS_OPEN, DOS_READ, DOS_WRITE, DOS_CLOSE, DOS_DIR,
; DOS_RUN, and DOS_ASSIGN requests from any task via shared-memory message
; passing.
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

include "lib/dos_library.s"

; ============================================================================
; Data: Strings
include "boot/strings.s"
    dc.b    "PANIC: boot program failed", 0x0D, 0x0A, 0
    align   4

; ============================================================================
; End of IExec M8
; ============================================================================
