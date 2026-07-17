; ehbasic_compiler_driver.inc - RUN AOT, COMPILE, TRANSPILE and ASSEMBLE drivers
; ============================================================================
; Phase: compiler-time memory allocators.
;
; alloc64    - top-down retained AOT arena for compiler workspace, asm text,
;              symbols, fixups, literals, and generated AOT code.
; The public low32 scratch window [AOT_LOW32_BASE, AOT_LOW32_END) remains
; reserved for guest-visible contracts, but File I/O accepts 64-bit pointers,
; so the drivers allocate everything from the retained arena.
;
; The cursors are reset per RUN AOT / COMPILE attempt and live in the AOT half
; of BASIC_STATE. Retained buffers must never be allocated from the temporary
; file bridge.
;
; Self-contained (depends only on ie64.inc constants and the mfcr instruction)
; so it can be unit-tested in isolation before being wired into the pipeline.
;
; ARENA ADDRESSING RULE: the RUN AOT arena (alloc64) can load tens of GiB up on a
; large-RAM host. "la rN, label" assembles to a single 32-bit, sign-extending lea,
; so it TRUNCATES any arena address with bit 31 set. Arena codegen must reach code
; addresses only PC-relatively (bra / jsr label / rts) and capture a return/resume
; PC via the hardware stack (jsr+pop), NEVER "la rN, <arena label>". Standalone
; COMPILE images load at a fixed low address, so "la" is safe there. TestREPL_RunAOT_
; HighArena forces a bit-31 arena base and guards this rule.
; ============================================================================

; --- aot_alloc_reset --------------------------------------------------------
; Reset AOT retained, public low32, and private file-bridge cursors.
; Call once at the start of every RUN AOT / COMPILE attempt.
; Clobbers: R1-R5, R8
aot_alloc_reset:
    mfcr    r8, cr15                ; R8 = CR_RAM_SIZE_BYTES (total guest RAM)
    ; Bound the arena frontier to the dense 32-bit window: the retained
    ; text/code allocations hold compiled native code the JIT executes,
    ; which must stay where the fused low paths can reach it. cr15 can
    ; exceed the window when the host backs RAM above 4 GiB.
    la      r1, SYSINFO_LOW_WINDOW_LO
    load.l  r1, (r1)
    la      r2, SYSINFO_LOW_WINDOW_HI
    load.l  r3, (r2)
    lsl.q   r3, r3, #32
    or.q    r1, r1, r3
    beqz    r1, .aar_frontier_cr15  ; SYSINFO absent: trust cr15
    blt     r1, r8, .aar_frontier_ready
.aar_frontier_cr15:
    move.q  r1, r8
.aar_frontier_ready:
    and.q   r2, r1, #0xFFF
    sub.q   r1, r1, r2             ; R1 = frontier & ~0xFFF
    ; Keep retained allocations below the BASIC stacks only when the
    ; stacks actually sit inside the arena region. Split layout (stacks
    ; strictly below the heap top, heap above the 32-bit window): the
    ; arena keeps the top-of-RAM frontier. Legacy layouts publish
    ; heap top == ctrl low, so the stacks bound the arena exactly when
    ; the resident window tops out at RAM top; a zero heap top (reset
    ; without layout init) treats any live ctrl low as the bound.
    add.q   r3, r16, #ST_CTRL_LOW
    load.q  r4, (r3)
    beqz    r4, .aar_have_arena_top
    bge     r4, r1, .aar_have_arena_top
    add.q   r3, r16, #ST_HEAP_TOP
    load.q  r5, (r3)
    beqz    r5, .aar_clamp_ctrl_low
    blt     r4, r5, .aar_have_arena_top
    add.q   r3, r16, #ST_STACK_HIGH
    load.q  r5, (r3)
    add.q   r5, r5, #BASIC_STACK_GUARD_BYTES
    blt     r5, r1, .aar_have_arena_top
.aar_clamp_ctrl_low:
    move.q  r1, r4                  ; stacks inside the arena region: sit below them
.aar_have_arena_top:
    la      r3, AOT_ARENA_HIGH_NEXT
    store.q r1, (r3)
    move.q  r4, #AOT_LOW32_BASE
    la      r3, AOT_LOW32_NEXT
    store.q r4, (r3)
    move.q  r4, #AOT_LOW32_CAP
    blt     r1, r4, .aar_bridge_from_arena
    move.q  r2, r4
    bra     .aar_bridge_apply_ctrl_low
.aar_bridge_from_arena:
    move.q  r2, r1
.aar_bridge_apply_ctrl_low:
    add.q   r3, r16, #ST_CTRL_LOW
    load.q  r4, (r3)
    beqz    r4, .aar_bridge_top_ok
    bge     r4, r2, .aar_bridge_top_ok
    move.q  r2, r4                  ; file bridge must stay below BASIC stacks
.aar_bridge_top_ok:
    la      r3, AOT_FILE_BRIDGE_NEXT
    store.q r2, (r3)
    move.q  r4, #AOT_FILE_BRIDGE_LOW_DEFAULT
    la      r3, AOT_FILE_BRIDGE_LOW
    store.q r4, (r3)
    la      r3, AOT_RESET_GENERATION
    load.q  r4, (r3)
    add.q   r4, r4, #1
    store.q r4, (r3)
    rts

; --- aot_alloc64 / aot_alloc_low32 / aot_alloc_file_bridge -------------------
; Allocate a 4 KiB-aligned block from the requested AOT cursor.
; Input:  R8 = requested byte size
; Output: R8 = allocated base address (0 on failure)
;         R9 = 0 on success, 1 if exhausted
; Clobbers: R1-R5
aot_alloc64:
    la      r4, AOT_ARENA_HIGH_NEXT
    load.q  r2, (r4)               ; R2 = current frontier (no clamp for 64-bit)
    move.q  r5, #AOT_ALLOC_FLOOR
    bra     aot_alloc_topdown_finish

aot_alloc_topdown_finish:
    ; R2 = base (frontier top to allocate beneath), R4 = &cursor, R5 = floor.
    ; Reject absurd sizes before aligning: only requests with bit 63 set can
    ; make (size + 0xFFF) wrap, and those would otherwise align to a tiny value
    ; and falsely succeed. Any positive (signed) size is far below 2^63 and
    ; cannot overflow the alignment add.
    blt     r8, r0, aot_alloc_fail
    ; Round the request up to a 4 KiB multiple: (size + 0xFFF) & ~0xFFF.
    add.q   r1, r8, #0xFFF
    and.q   r3, r1, #0xFFF
    sub.q   r1, r1, r3             ; R1 = aligned size
    sub.q   r3, r2, r1             ; R3 = candidate base / new frontier
    ; Reject if it would drop below the floor. A request larger than the
    ; remaining window wraps R3 far above R2 (huge unsigned), which is also
    ; < FLOOR under signed comparison, so this single check covers both; the
    ; bgt is a defensive underflow guard.
    blt     r3, r5, aot_alloc_fail
    bgt     r3, r2, aot_alloc_fail
    store.q r3, (r4)               ; commit the requested frontier downward
    ; When active RAM is below the low32 bridge cap, the retained AOT arena and
    ; private file bridge share the same top-down window. Keep both cursors at
    ; the lowest successful allocation frontier so mixed COMPILE allocations do
    ; not overlap. On high-RAM hosts the retained arena lives above low32, so the
    ; bridge must not drag it down.
    la      r1, AOT_ARENA_HIGH_NEXT
    load.q  r2, (r1)
    move.q  r5, #AOT_LOW32_CAP
    bgt     r2, r5, .atd_done
    bge     r3, r2, .atd_bridge_sync
    store.q r3, (r1)
.atd_bridge_sync:
    la      r1, AOT_FILE_BRIDGE_NEXT
    load.q  r2, (r1)
    bge     r3, r2, .atd_done
    store.q r3, (r1)
.atd_done:
    move.q  r8, r3
    move.q  r9, r0
    rts
aot_alloc_fail:
    move.q  r8, r0
    move.q  r9, #1
    rts

; aot_transpile - lower a tokenised BASIC programme to IE64 asm text.
; First slice: supports the empty programme and programmes whose statements are
; only END / REM. Any other statement is not yet lowerable.
;
; The line list is the live IE64 BASIC format: records
; [next:q, lineNo:l, reserved:l, tokens..]
; chained by next; the terminator record has next == 0. A real line always has
; a non-zero next, so next == 0 marks the end without a body to lower.
;
; Input:  R8  = pointer to the first line record (may be the terminator)
;         R9  = output text buffer
;         R10 = end mode: 0 = arena (emit "rts", return to the REPL trampoline),
;               non-zero = standalone (emit "halt")
;         R11 = output buffer end limit (0 = unbounded; used by callers with a
;               fixed arena to fail safely instead of overrunning it)
; Output: R8  = 1 if fully lowered, 0 unsupported statement, 2 buffer overflow
;         R9  = length of the emitted asm text (valid on success)
; Clobbers: R1-R3, R8, R9, R17-R21
aot_str_movel_r1:
    dc.b    "move.l r1, #", 0
    align 4
aot_str_movel_r2:
    dc.b    "move.l r2, #", 0
    align 4
aot_str_movel_r3:
    dc.b    "move.l r3, #", 0
    align 4
aot_str_movel_r14:
    dc.b    "move.l r14, #", 0
    align 4
aot_str_lslq_r3_32:
    dc.b    "lsl.q r3, r3, #32", 0x0A, 0
    align 4
aot_str_orq_r2_r3:
    dc.b    "or.q r2, r2, r3", 0x0A, 0
    align 4
aot_str_moveq_r2_r8:
    dc.b    "move.q r2, r8", 0x0A, 0
    align 4
aot_str_moveq_r1_r8:
    dc.b    "move.q r1, r8", 0x0A, 0
    align 4
aot_str_loadq_r8_r8:
    dc.b    "load.q r8, (r8)", 0x0A, 0
    align 4
aot_str_load_var_value_r8_tag_r9:
    dc.b    "move.q r1, r8", 0x0A
    dc.b    "move.q r2, r1", 0x0A
    dc.b    "sub.q r2, r2, #8", 0x0A
    dc.b    "load.l r9, (r2)", 0x0A
    dc.b    "load.q r8, (r1)", 0x0A, 0
    align 4
aot_str_addq_r8_r1_r8:
    dc.b    "add.q r8, r1, r8", 0x0A, 0
    align 4
aot_str_lslq_r1_4:
    dc.b    "lsl.q r1, r1, #4", 0x0A, 0
    align 4
aot_str_addq_r8_r8_r1:
    dc.b    "add.q r8, r8, r1", 0x0A, 0
    align 4
aot_str_addq_r8_40:
    dc.b    "add.q r8, r8, #40", 0x0A, 0
    align 4
aot_str_subq_r8_r1_r8:
    dc.b    "sub.q r8, r1, r8", 0x0A, 0
    align 4
aot_str_mulsq_r8_r1_r8:
    dc.b    "muls.q r8, r1, r8", 0x0A, 0
    align 4
aot_str_divsq_r8_r1_r8:
    dc.b    "divs.q r8, r1, r8", 0x0A, 0
    align 4
aot_str_andq_r8_r1_r8:
    dc.b    "and.q r8, r1, r8", 0x0A, 0
    align 4
aot_str_negq_r8:
    dc.b    "neg.q r8, r8", 0x0A, 0
    align 4
aot_str_cmp_false:
    dc.b    "move.q r8, #0", 0x0A, 0
    align 4
aot_str_cmp_true_set:
    dc.b    "move.q r8, #1", 0x0A, 0
    align 4
aot_str_blt_ic:
    dc.b    "blt r1, r8, IC", 0
    align 4
aot_str_bgt_ic:
    dc.b    "bgt r1, r8, IC", 0
    align 4
aot_str_beq_ic:
    dc.b    "beq r1, r8, IC", 0
    align 4
aot_str_bne_ic:
    dc.b    "bne r1, r8, IC", 0
    align 4
aot_str_bra_id:
    dc.b    "bra ID", 0
    align 4
aot_str_lbl_ic:
    dc.b    "IC", 0
    align 4
aot_str_lbl_id:
    dc.b    "ID", 0
    align 4
aot_str_loadb_r8_r1:
    dc.b    "load.b r8, (r1)", 0x0A, 0
    align 4
aot_str_andl_r8_ff:
    dc.b    "and.l r8, r8, #0xFF", 0x0A, 0
    align 4
aot_str_loadl_r8_r1:
    dc.b    "load.l r8, (r1)", 0x0A, 0
    align 4
aot_str_push_r8:
    dc.b    "push r8", 0x0A, 0
    align 4
aot_str_push_r1:
    dc.b    "push r1", 0x0A, 0
    align 4
aot_str_pop_r1:
    dc.b    "pop r1", 0x0A, 0
    align 4
aot_str_storeq_r8_r1:
    dc.b    "store.q r8, (r1)", 0x0A, 0
    align 4
aot_str_storel_r9_tag_r1:
    dc.b    "move.q r2, r1", 0x0A
    dc.b    "sub.q r2, r2, #8", 0x0A
    dc.b    "store.l r9, (r2)", 0x0A, 0
    align 4
aot_str_store_b:
    dc.b    "store.b r2, (r1)", 0x0A, 0
    align 4
aot_str_store_w:
    dc.b    "store.w r2, (r1)", 0x0A, 0
    align 4
aot_str_store_l:
    dc.b    "store.l r2, (r1)", 0x0A, 0
    align 4
aot_str_store_q:
    dc.b    "store.q r2, (r1)", 0x0A, 0
    align 4
aot_str_loadb_r2:
    dc.b    "load.b r2, (r1)", 0x0A, 0
    align 4
aot_str_jsr_r1:
    dc.b    "jsr (r1)", 0x0A, 0
    align 4
aot_str_orl_r2:
    dc.b    "or.l r2, r2, #", 0
    align 4
aot_str_andl_r2:
    dc.b    "and.l r2, r2, #", 0
    align 4
aot_str_nl:
    dc.b    0x0A, 0
    align 4
aot_str_label_l:
    dc.b    "L", 0
    align 4
aot_str_bra_l:
    dc.b    "bra L", 0
    align 4
aot_str_jsr_l:
    dc.b    "jsr L", 0
    align 4
; WAIT/VSYNC poll-loop fragments. Registers: r1=addr, r2=value, r3=mask,
; r4=xor, r5=timeout. Loop label "W<id>", done label "X<id>".
aot_str_movel_r4:
    dc.b    "move.l r4, #", 0
    align 4
aot_str_wait_to:
    dc.b    "move.l r5, #1048576", 0x0A, 0
    align 4
aot_str_loadl_r2:
    dc.b    "load.l r2, (r1)", 0x0A, 0
    align 4
aot_str_eorl_r4:
    dc.b    "eor.l r2, r2, r4", 0x0A, 0
    align 4
aot_str_andl_r3:
    dc.b    "and.l r2, r2, r3", 0x0A, 0
    align 4
aot_str_subq_r5:
    dc.b    "sub.q r5, r5, #1", 0x0A, 0
    align 4
aot_str_bne_done:
    dc.b    "bne r2, r0, X", 0
    align 4
aot_str_bne_top:
    dc.b    "bne r5, r0, W", 0
    align 4
aot_str_lbl_w:
    dc.b    "W", 0
    align 4
aot_str_lbl_x:
    dc.b    "X", 0
    align 4
; Arena (RUN AOT) entry-SP save/restore. END/STOP must unwind any GOSUB nesting
; back to the entry stack pointer before rts, or rts would pop a GOSUB return
; address and resume after the call instead of terminating the programme.
aot_str_movel_r6:
    dc.b    "move.l r6, #", 0
    align 4
; Standalone (COMPILE) bootstrap prologue: a flat .ie64 boots with no resident
; setup, so the image establishes its own dynamic stack/control reservations and
; the TERM_OUT pointer that the bundled fp_print / aot_print_str helpers write
; through (R26), plus the state base (R16) and TERM_STATUS pointer (R27) for
; parity with the resident runtime.
aot_str_basic_layout_init:
    dc.b    "mfcr r1, cr15", 0x0A
    dc.b    "bnez r1, B1", 0x0A
    dc.b    "move.q r1, #0x02000000", 0x0A
    dc.b    "B1:", 0x0A
    dc.b    "lsr.q r1, r1, #12", 0x0A
    dc.b    "lsl.q r1, r1, #12", 0x0A
    dc.b    "sub.q r1, r1, #0x1000", 0x0A
    dc.b    "move.q r31, r1", 0x0A
    dc.b    "move.l r2, #0x420A8", 0x0A
    dc.b    "store.q r31, (r2)", 0x0A
    dc.b    "move.q r3, r31", 0x0A
    dc.b    "sub.q r3, r3, #0x10000", 0x0A
    dc.b    "move.l r2, #0x420A0", 0x0A
    dc.b    "store.q r3, (r2)", 0x0A
    dc.b    "move.l r2, #0x420B8", 0x0A
    dc.b    "store.q r3, (r2)", 0x0A
    dc.b    "move.q r4, r3", 0x0A
    dc.b    "sub.q r4, r4, #0x10000", 0x0A
    dc.b    "move.l r2, #0x420B0", 0x0A
    dc.b    "store.q r4, (r2)", 0x0A
    dc.b    "move.l r2, #0x42060", 0x0A
    dc.b    "store.q r4, (r2)", 0x0A
    dc.b    "move.l r2, #0x42068", 0x0A
    dc.b    "store.q r3, (r2)", 0x0A
    dc.b    "move.q r5, #0xFFFF0000", 0x0A
    dc.b    "blt r4, r5, B2", 0x0A
    dc.b    "move.q r4, r5", 0x0A
    dc.b    "B2:", 0x0A
    dc.b    "move.l r2, #0x420C0", 0x0A
    dc.b    "store.q r4, (r2)", 0x0A
    dc.b    "move.q r6, #0x01000000", 0x0A
    dc.b    "move.l r2, #0x420C8", 0x0A
    dc.b    "store.q r6, (r2)", 0x0A
    dc.b    "move.l r2, #0x420D0", 0x0A
    dc.b    "store.q r6, (r2)", 0x0A
    dc.b    "move.l r2, #0x420E0", 0x0A
    dc.b    "store.q r6, (r2)", 0x0A
    dc.b    "move.q r6, #0x400", 0x0A
    dc.b    "move.l r2, #0x420E8", 0x0A
    dc.b    "store.q r6, (r2)", 0x0A
    dc.b    "move.q r7, #0x01000000", 0x0A
    dc.b    "add.q r6, r6, r7", 0x0A
    dc.b    "move.l r2, #0x420D8", 0x0A
    dc.b    "store.q r6, (r2)", 0x0A
    dc.b    "move.l r2, #0x420C8", 0x0A
    dc.b    "store.q r6, (r2)", 0x0A
    dc.b    "move.l r2, #0x42038", 0x0A
    dc.b    "store.q r6, (r2)", 0x0A
    dc.b    "move.l r2, #0x42030", 0x0A
    dc.b    "store.q r4, (r2)", 0x0A, 0
    align 4
aot_str_movel_r31:
    dc.b    "move.l r31, #", 0
    align 4
aot_str_movel_r26:
    dc.b    "move.l r26, #", 0
    align 4
aot_str_movel_r27:
    dc.b    "move.l r27, #", 0
    align 4
aot_str_movel_r16:
    dc.b    "move.l r16, #", 0
    align 4
aot_str_save_sp:
    dc.b    "store.q r31, (r6)", 0x0A, 0
    align 4
aot_str_load_sp:
    dc.b    "load.q r31, (r6)", 0x0A, 0
    align 4
; RETURN-without-GOSUB guard (arena mode): compare SP to the saved entry SP.
aot_str_load_r6:
    dc.b    "load.q r6, (r6)", 0x0A, 0
    align 4
aot_str_blt_g:
    dc.b    "blt r31, r6, G", 0
    align 4
aot_str_movel_r5:
    dc.b    "move.l r5, #", 0
    align 4
aot_str_store_r5:
    dc.b    "store.q r5, (r6)", 0x0A, 0
    align 4
aot_str_store_r5_l:
    dc.b    "store.l r5, (r6)", 0x0A, 0
    align 4
aot_str_addq_r6_r16:
    dc.b    "add.q r6, r16, #", 0
    align 4
aot_str_lbl_g:
    dc.b    "G", 0
    align 4
; PRINT "literal": inline string data (jumped over) printed via the bundled
; aot_print_str helper, then CRLF (unless a trailing ';').
aot_str_bra_p:
    dc.b    "bra P", 0
    align 4
aot_str_lbl_q:
    dc.b    "Q", 0
    align 4
aot_str_lbl_p:
    dc.b    "P", 0
    align 4
aot_str_dcb:
    dc.b    "dc.b ", 0
    align 4
aot_str_comma_sp:
    dc.b    ", ", 0
    align 4
aot_str_zero_nl:
    dc.b    "0", 0x0A, 0
    align 4
aot_str_la_r8_q:
    dc.b    "la r8, Q", 0
    align 4
; --- Arena STOP (CONT continuation) ---
; GOSUB/RETURN use the hardware stack (jsr L<line> / rts): PC-relative, so the full
; 64-bit return PC is pushed by the hardware regardless of where the arena loads
; (it can sit above 4 GiB on a large-RAM host). STOP cannot use "la" to capture the
; resume address - "la" is a 32-bit lea and would truncate an arena address above
; 4 GiB - so it captures the full 64-bit resume PC the same way: a "jsr" into a tiny
; capture helper pushes PC+8 (the resume label C<id>), which the helper pops into r5
; and stores in AOT_CONT_PC before unwinding to the REPL.
aot_str_lbl_c:
    dc.b    "C", 0
    align 4
aot_str_lbl_v:
    dc.b    "V", 0
    align 4
aot_str_jsr_v:
    dc.b    "jsr V", 0
    align 4
aot_str_pop_r5:
    dc.b    "pop r5", 0x0A, 0
    align 4
aot_str_jsr_print:
    dc.b    "jsr aot_print_str", 0x0A, 0
    align 4
aot_str_crlf:
    dc.b    "move.q r1, #13", 0x0A, "store.b r1, (r26)", 0x0A
    dc.b    "move.q r1, #10", 0x0A, "store.b r1, (r26)", 0x0A, 0
    align 4
; PRINT <integer literal>: convert to FP64 at runtime and call the bundled
; fp_print (so formatting matches the interpreter's expr_eval -> fp_print path).
aot_str_movel_r8:
    dc.b    "move.l r8, #", 0
    align 4
aot_str_moveq_r8_imm:
    dc.b    "move.q r8, #", 0
    align 4
aot_str_lslq_r8_32:
    dc.b    "lsl.q r8, r8, #32", 0x0A, 0
    align 4
aot_str_orq_r8_r8_r1:
    dc.b    "or.q r8, r8, r1", 0x0A, 0
    align 4
aot_str_fp_seq:
    dc.b    "jsr fp_float", 0x0A, "jsr fp_print", 0x0A, 0
    align 4
; Generic leaf-statement delegation: point R17 at the statement tokens and call
; the resident handler via the interpreter's stmt_jump_table.
aot_str_movel_r17:
    dc.b    "move.l r17, #", 0
    align 4
aot_str_movel_r17_q:
    dc.b    "move.l r17, #Q", 0
    align 4
aot_str_jsr_fp_print:
    dc.b    "jsr fp_print", 0x0A, 0
    align 4
aot_str_movel_r9:
    dc.b    "move.l r9, #", 0
    align 4
aot_str_load_jsr_r9:
    dc.b    "load.l r9, (r9)", 0x0A, "jsr (r9)", 0x0A, 0
    align 4
; IF <expr> THEN: evaluate the condition via resident expr_eval, then branch.
aot_str_jsr_r6:
    dc.b    "jsr (r6)", 0x0A, 0
    align 4
; --- Standalone runtime-blob bootstrap + blob-call templates ----------------
aot_str_loadq_r6_r6:
    dc.b    "load.q r6, (r6)", 0x0A, 0
    align 4
; Post-call error check: a bundled runtime call that raised an error left a non-zero
; state[ST_ERROR_FLAG] (raise_error persisted it and printed the message). A standalone
; image has no REPL to unwind to, so halt rather than run on into corrupt state. E<id>
; skips the halt on the no-error path.
aot_str_loadl_r6_r6:
    dc.b    "load.l r6, (r6)", 0x0A, 0
    align 4
aot_str_beqz_r6_e:
    dc.b    "beqz r6, E", 0
    align 4
aot_str_beq_r9_r1_e:
    dc.b    "beq r9, r1, E", 0
    align 4
aot_str_lbl_e:
    dc.b    "E", 0
    align 4
aot_str_store_r2_r1:
    dc.b    "store.l r2, (r1)", 0x0A, 0
    align 4
aot_str_beqz_ie:
    dc.b    "beqz r8, IE", 0
    align 4
aot_str_lbl_ie:
    dc.b    "IE", 0
    align 4
; IF ... ELSE: false branches to EL<id> (the ELSE clause); the THEN clause ends
; with "bra IE<id>" to skip over the ELSE clause to the line-end join.
aot_str_beqz_el:
    dc.b    "beqz r8, EL", 0
    align 4
aot_str_bra_ie:
    dc.b    "bra IE", 0
    align 4
aot_str_lbl_el:
    dc.b    "EL", 0
    align 4
aot_str_moveq_r1:
    dc.b    "move.q r1, #", 0
    align 4
aot_str_beq_r1_l:
    dc.b    "beq r8, r1, L", 0
    align 4
aot_str_bne_r1_s:
    dc.b    "bne r8, r1, S", 0
    align 4
aot_str_bra_d:
    dc.b    "bra D", 0
    align 4
aot_str_lbl_s:
    dc.b    "S", 0
    align 4
aot_str_lbl_d:
    dc.b    "D", 0
    align 4
; FOR/NEXT labels. Simple scalar integer loops use F<id> as the loop top and
; FD<id> as the done label; delegated loops still use F<id> for their loop-back.
aot_str_lbl_f:
    dc.b    "F", 0
    align 4
aot_str_lbl_fd:
    dc.b    "FD", 0
    align 4
aot_str_bra_f:
    dc.b    "bra F", 0
    align 4
aot_str_bgt_fd:
    dc.b    "bgt r1, r8, FD", 0
    align 4
aot_str_blt_fd:
    dc.b    "blt r1, r8, FD", 0
    align 4
aot_str_addq_r1_imm:
    dc.b    "add.q r1, r1, #", 0
    align 4
aot_str_subq_r1_imm:
    dc.b    "sub.q r1, r1, #", 0
    align 4
aot_str_for_next_load:
    dc.b    "move.q r3, r8", 0x0A
    dc.b    "load.q r1, (r3)", 0x0A, 0
    align 4
aot_str_for_next_store:
    dc.b    "move.q r4, r3", 0x0A
    dc.b    "sub.q r4, r4, #8", 0x0A
    dc.b    "move.l r2, #2", 0x0A
    dc.b    "store.l r2, (r4)", 0x0A
    dc.b    "store.q r1, (r3)", 0x0A, 0
    align 4
aot_str_addq_r1_gosub:
    dc.b    "add.q r1, r16, #", 0
    align 4
aot_str_load_r1_r1:
    dc.b    "load.q r1, (r1)", 0x0A, 0
    align 4
aot_str_store_r1_r2:
    dc.b    "store.q r1, (r2)", 0x0A, 0
    align 4
aot_str_load_r2_r2:
    dc.b    "load.q r2, (r2)", 0x0A, 0
    align 4
aot_str_beq_r1_r2_f:
    dc.b    "beq r1, r2, F", 0
    align 4
; WHILE/WEND and DO/LOOP (fully native: expr_eval condition + native branches).
; (aot_str_lbl_w "W" is shared with the WAIT poll-loop strings.)
aot_str_lbl_we:
    dc.b    "WE", 0
    align 4
aot_str_beqz_we:
    dc.b    "beqz r8, WE", 0
    align 4
aot_str_bra_w:
    dc.b    "bra W", 0
    align 4
aot_str_beqz_d:
    dc.b    "beqz r8, D", 0
    align 4
aot_str_bnez_d:
    dc.b    "bnez r8, D", 0
    align 4


; aot_emit_movel - emit "<prefix><uint>\n" to the text cursor: the move.l
; prefix string at R9 followed by the value in R10. Clobbers R1-R3, R9.
aot_emit_movel:
    jsr     aot_emit_str            ; prefix
    move.q  r9, r10
    jsr     aot_emit_uint           ; value
    la      r9, aot_str_nl
    jsr     aot_emit_str
    rts

; aot_emit_delegate - emit a call to a resident statement handler:
;   move.l r17, #<token pointer> / move.l r9, #<jump-table slot> /
;   load.l r9, (r9) / jsr (r9)
; The handler executes one statement using the live interpreter state (R16) and
; the resident token stream. R8 = cursor, R9 = R17 target (resident token
; address), R10 = jump-table slot address. Returns R8 advanced. Clobbers
; R1-R10, R12, R13.
aot_skip_spaces:
.ass_loop:
    load.b  r1, (r20)
    move.q  r2, #0x20
    bne     r1, r2, .ass_done
    add.q   r20, r20, #1
    bra     .ass_loop
.ass_done:
    rts

; aot_skip_var_name - advance R20 over a BASIC numeric variable name.
; Mirrors var_parse_name's character class: first char must be a letter, following
; chars may be letters or digits.
aot_skip_var_name:
    load.b  r1, (r20)
    or.q    r2, r1, #0x20
    move.q  r3, #0x61
    blt     r2, r3, .asv_done
    move.q  r3, #0x7A
    bgt     r2, r3, .asv_done
    add.q   r20, r20, #1
.asv_loop:
    load.b  r1, (r20)
    or.q    r2, r1, #0x20
    move.q  r3, #0x61
    blt     r2, r3, .asv_digit
    move.q  r3, #0x7A
    ble     r2, r3, .asv_adv
.asv_digit:
    move.q  r3, #0x30
    blt     r1, r3, .asv_done
    move.q  r3, #0x39
    bgt     r1, r3, .asv_done
.asv_adv:
    add.q   r20, r20, #1
    bra     .asv_loop
.asv_done:
    rts

; aot_parse_one_uint - parse "<uint>" at R20. R22 = value, R20 advanced.
; R8 = 1 on success, 0 on failure. Clobbers R1-R6, R8-R10.
aot_parse_one_uint:
    move.q  r8, r20
    jsr     aot_parse_uint
    beqz    r9, .p1u_fail
    move.q  r22, r8
    move.q  r20, r10
    move.q  r8, #1
    rts
.p1u_fail:
    move.q  r8, r0
    rts

; aot_emit_uint - append R9 as decimal ASCII to the buffer at R8.
; Output R8 = cursor advanced. Clobbers R1-R7 (uses the hardware stack).
aot_emit_uint:
    bnez    r9, .eu_conv
    move.q  r1, #0x30               ; "0"
    store.b r1, (r8)
    add.q   r8, r8, #1
    rts
.eu_conv:
    move.q  r2, r0                  ; digit count
    move.q  r3, r9                  ; remaining value
.eu_div:
    beqz    r3, .eu_emit
    move.q  r4, r3
    move.q  r5, #10
    divu.q  r4, r4, r5             ; r4 = r3 / 10 (full 64-bit value)
    mulu.q  r6, r4, r5             ; r6 = (r3/10) * 10
    sub.q   r7, r3, r6            ; r7 = r3 % 10
    add.q   r7, r7, #0x30          ; -> ASCII digit
    push    r7
    add.q   r2, r2, #1
    move.q  r3, r4
    bra     .eu_div
.eu_emit:
    beqz    r2, .eu_done
    pop     r1
    store.b r1, (r8)
    add.q   r8, r8, #1
    sub.q   r2, r2, #1
    bra     .eu_emit
.eu_done:
    rts

; aot_emit_dcb - append the R10 bytes at R9 to the asm text at R8 as `dc.b` lines
; (16 decimal values per line), so a binary payload (the fp_print closure, the
; runtime blob, the tokenised programme) becomes self-contained assembler source.
; Keeps the text null-terminated. Bounds-checked against AOT_TEXT_END (0 =
; unbounded). R8 = text cursor (advanced and returned). R9 = source. R10 = count.
; Output R11 = 1 ok, 0 text overflow. Clobbers R1-R7 (via aot_emit_uint), R13-R15.
aot_file_write:
    la      r1, FILE_NAME_PTR
    store.l r8, (r1)
    la      r1, FILE_DATA_PTR64
    store.q r9, (r1)
    la      r1, FILE_DATA_LEN
    store.l r10, (r1)
    la      r1, FILE_CTRL
    move.q  r2, #FILE_OP_WRITE
    store.l r2, (r1)               ; write triggers on FILE_CTRL
    la      r1, FILE_STATUS
    load.l  r8, (r1)
    rts

aot_native_vars_alloc:
    ; Name-tag table (AOT_NATIVE_VAR_MAX * uint32) plus one known-I64
    ; flag byte per scalar at offset 0x4000.
    move.q  r8, #0x5000
    jsr     aot_alloc64
    bnez    r9, .anva_fail
    beqz    r8, .anva_fail
    la      r1, AOT_NATIVE_VAR_TAB
    store.q r8, (r1)
    la      r1, AOT_NATIVE_VAR_COUNT
    store.q r0, (r1)
    move.q  r9, #1
    rts
.anva_fail:
    move.q  r9, r0
    rts

; Input R20 = programme first line record. Output R9 = 1 ok, 0 overflow.
aot_native_vars_prescan:
    move.q  r6, r20
.anvp_line:
    load.q  r7, (r6)
    beqz    r7, .anvp_done
    add.q   r3, r6, #16
.anvp_tok:
    load.b  r1, (r3)
    beqz    r1, .anvp_next_line
    move.q  r2, #0x8F              ; TK_REM: rest of line is comment text
    beq     r1, r2, .anvp_next_line
    move.q  r2, #0x83              ; TK_DATA: rest of line is literal DATA text
    beq     r1, r2, .anvp_next_line
    move.q  r2, #0x22
    beq     r1, r2, .anvp_string
    or.q    r2, r1, #0x20
    move.q  r4, #0x61
    blt     r2, r4, .anvp_adv
    move.q  r4, #0x7A
    bgt     r2, r4, .anvp_adv
    move.q  r20, r3
    push    r6
    push    r7
    jsr     aot_parse_compiler_var_name
    beqz    r15, .anvp_restore_adv
    load.b  r1, (r22)
    move.q  r2, #0x24
    beq     r1, r2, .anvp_restore_skip_name
    move.q  r2, #0x28
    beq     r1, r2, .anvp_restore_skip_name
    jsr     aot_native_var_intern
    move.q  r5, r22
    pop     r7
    pop     r6
    beqz    r9, .anvp_fail
    move.q  r3, r5
    bra     .anvp_tok
.anvp_restore_skip_name:
    move.q  r5, r22
    pop     r7
    pop     r6
    move.q  r3, r5
    bra     .anvp_tok
.anvp_restore_adv:
    pop     r7
    pop     r6
    bra     .anvp_adv
.anvp_string:
    add.q   r3, r3, #1
.anvp_string_loop:
    load.b  r1, (r3)
    beqz    r1, .anvp_next_line
    move.q  r2, #0x22
    beq     r1, r2, .anvp_string_end
    add.q   r3, r3, #1
    bra     .anvp_string_loop
.anvp_string_end:
    add.q   r3, r3, #1
    bra     .anvp_tok
.anvp_adv:
    add.q   r3, r3, #1
    bra     .anvp_tok
.anvp_next_line:
    move.q  r6, r7
    bra     .anvp_line
.anvp_done:
    move.q  r9, #1
    rts
.anvp_fail:
    move.q  r9, r0
    rts

; Input R20 = variable token pointer.
; Output R14 = tag, R15 = char count, R22 = pointer past name.
aot_parse_compiler_var_name:
    move.q  r14, r0
    move.q  r15, r0
    move.q  r22, r20
.apcv_collect:
    load.b  r1, (r22)
    move.q  r2, #0x41
    blt     r1, r2, .apcv_check_digit
    move.q  r2, #0x5A
    ble     r1, r2, .apcv_is_alpha
    move.q  r2, #0x61
    blt     r1, r2, .apcv_check_digit
    move.q  r2, #0x7A
    ble     r1, r2, .apcv_is_alpha
    bra     .apcv_check_digit
.apcv_is_alpha:
    move.q  r2, #0x61
    blt     r1, r2, .apcv_store_char
    sub.q   r1, r1, #0x20
.apcv_store_char:
    move.q  r2, #4
    bge     r15, r2, .apcv_mix_extra
    lsl.l   r14, r14, #8
    or.l    r14, r14, r1
    add.q   r15, r15, #1
    bra     .apcv_advance
.apcv_mix_extra:
    mulu.l  r14, r14, #33
    add.l   r14, r14, r1
.apcv_advance:
    add.q   r22, r22, #1
    bra     .apcv_collect
.apcv_check_digit:
    move.q  r2, #0x30
    blt     r1, r2, .apcv_done
    move.q  r2, #0x39
    bgt     r1, r2, .apcv_done
    beqz    r15, .apcv_done
    bra     .apcv_store_char
.apcv_done:
    rts

; Intern the simple scalar variable at R20.
; Output R8 = native payload address, R9 = 1 ok, R11 = table index,
;        R22 = pointer past name. A newly added scalar starts known-I64;
;        the type scan clears the flag for non-native writes.
aot_native_var_intern:
    jsr     aot_parse_compiler_var_name
    beqz    r15, .anvi_fail
    load.b  r1, (r22)
    move.q  r2, #0x24
    beq     r1, r2, .anvi_fail
    move.q  r2, #0x28
    beq     r1, r2, .anvi_fail
    la      r1, AOT_NATIVE_VAR_TAB
    load.q  r3, (r1)
    la      r1, AOT_NATIVE_VAR_COUNT
    load.q  r4, (r1)
    move.q  r5, r0
.anvi_search:
    bge     r5, r4, .anvi_add
    lsl.q   r6, r5, #2
    add.q   r6, r3, r6
    load.l  r7, (r6)
    beq     r7, r14, .anvi_found
    add.q   r5, r5, #1
    bra     .anvi_search
.anvi_add:
    move.q  r6, #AOT_NATIVE_VAR_MAX
    bge     r4, r6, .anvi_fail
    lsl.q   r6, r4, #2
    add.q   r6, r3, r6
    store.l r14, (r6)
    add.q   r6, r3, #0x4000
    add.q   r6, r6, r4
    move.q  r7, #1
    store.b r7, (r6)               ; known-I64 until a non-native write clears it
    move.q  r5, r4
    add.q   r4, r4, #1
    la      r1, AOT_NATIVE_VAR_COUNT
    store.q r4, (r1)
.anvi_found:
    move.q  r11, r5
    move.q  r8, r5
    mulu.q  r8, r8, #24
    add.q   r8, r8, #(AOT_NATIVE_VAR_SEG + 32)
    move.q  r9, #1
    rts
.anvi_fail:
    move.q  r8, r0
    move.q  r9, r0
    rts

; --- compile-time scalar typing ----------------------------------------------
; A native scalar is "known-I64" when every write the programme can make to it
; provably stores an I64-tagged value: a division-free native integer LET, or a
; native FOR/NEXT pair. Interning seeds the flag, the type scan below clears it
; at each non-native write, and aot_emit_var_read_atom skips the runtime tag
; guard exactly for flagged scalars. CLEAR/NEW/LOAD rebuild variables at run
; time, so their presence anywhere disables the typing pass wholesale.

; aot_int_expr_supported_nodiv - aot_int_expr_supported minus division.
; Integer division truncates, so a quotient RHS stays on the tagged evaluator
; to keep BASIC's fractional semantics; every other supported form stores I64.
; Input R20. Output R9 = 1 supported. Preserves R20. Clobbers R1-R7.
aot_int_expr_supported_nodiv:
    jsr     aot_int_expr_supported
    beqz    r9, .iesn_ret
    move.q  r3, r20
    move.q  r4, r0                 ; parenthesis depth
.iesn_loop:
    load.b  r1, (r3)
    beqz    r1, .iesn_ok
    move.q  r2, #0x3A              ; ':'
    beq     r1, r2, .iesn_ok
    move.q  r2, #0x2C              ; ','
    beq     r1, r2, .iesn_ok
    move.q  r2, #0xAC              ; THEN
    beq     r1, r2, .iesn_ok
    move.q  r2, #0xAB              ; ELSE
    beq     r1, r2, .iesn_ok
    move.q  r2, #0xB4              ; division
    beq     r1, r2, .iesn_fail
    move.q  r2, #0xBF              ; INT(...)
    beq     r1, r2, .iesn_skip_int
    move.q  r2, #0x28
    bne     r1, r2, .iesn_not_lparen
    add.q   r4, r4, #1
    bra     .iesn_next
.iesn_not_lparen:
    move.q  r2, #0x29
    bne     r1, r2, .iesn_next
    beqz    r4, .iesn_ok
    sub.q   r4, r4, #1
.iesn_next:
    add.q   r3, r3, #1
    bra     .iesn_loop
.iesn_skip_int:
    add.q   r3, r3, #1
.iesn_int_sp:
    load.b  r1, (r3)
    move.q  r2, #0x20
    bne     r1, r2, .iesn_int_lparen
    add.q   r3, r3, #1
    bra     .iesn_int_sp
.iesn_int_lparen:
    move.q  r2, #0x28              ; '('
    bne     r1, r2, .iesn_fail
    move.q  r5, #1
    add.q   r3, r3, #1
.iesn_int_loop:
    load.b  r1, (r3)
    beqz    r1, .iesn_fail
    move.q  r2, #0x28
    bne     r1, r2, .iesn_int_not_lparen
    add.q   r5, r5, #1
    bra     .iesn_int_next
.iesn_int_not_lparen:
    move.q  r2, #0x29
    bne     r1, r2, .iesn_int_next
    sub.q   r5, r5, #1
    add.q   r3, r3, #1
    beqz    r5, .iesn_loop
    bra     .iesn_int_loop
.iesn_int_next:
    add.q   r3, r3, #1
    bra     .iesn_int_loop
.iesn_ok:
    move.q  r9, #1
    rts
.iesn_fail:
    move.q  r9, r0
.iesn_ret:
    rts

; aot_native_var_flag_clear_r20 - clear the known-I64 flag of the scalar at
; R20 (leading spaces skipped). String and array names are left alone; R22
; always ends past the parsed name so list walkers can advance.
; Clobbers R1-R11, R14, R15, R20, R22.
aot_native_var_flag_clear_r20:
    jsr     aot_skip_spaces
    jsr     aot_native_var_intern
    beqz    r9, .nvfc_ret
    la      r1, AOT_NATIVE_VAR_TAB
    load.q  r1, (r1)
    add.q   r1, r1, #0x4000
    add.q   r1, r1, r11
    load.b  r2, (r1)
    beqz    r2, .nvfc_ret          ; already clear: no change to latch
    store.b r0, (r1)
    la      r2, AOT_NATIVE_TYPES_CHANGED
    move.q  r3, #1
    store.q r3, (r2)
.nvfc_ret:
    rts

; aot_rhs_scalars_all_known_i64 - R9=1 iff every scalar operand in the RHS
; expression at R20 is currently known-I64. A syntactically integer RHS that
; reads an FP scalar would truncate it, so LET (and the type scan's keep
; decision) must fall back to the tagged evaluator in that case. The RHS has
; already passed aot_int_expr_supported_nodiv, so it holds only scalars,
; unsigned literals, +, -, *, comparisons, and parentheses. Stops at a
; statement separator. Preserves R20. Clobbers R1-R11, R14, R15, R22-R24.
aot_rhs_scalars_all_known_i64:
	push    r20
	la      r1, AOT_NATIVE_TYPES_OFF
	load.q  r1, (r1)
	bnez    r1, .rsak_no           ; typing disabled: no scalar is proven I64
.rsak_loop:
    load.b  r1, (r20)
    beqz    r1, .rsak_yes          ; end of statement text
    move.q  r2, #0x3A              ; ':'
    beq     r1, r2, .rsak_yes
    move.q  r2, #0x2C              ; ','
    beq     r1, r2, .rsak_yes
    move.q  r2, #0xAC              ; THEN
    beq     r1, r2, .rsak_yes
    move.q  r2, #0xAB              ; ELSE
    beq     r1, r2, .rsak_yes
    or.q    r2, r1, #0x20
    move.q  r3, #0x61              ; 'a'
    blt     r2, r3, .rsak_adv
    move.q  r3, #0x7A              ; 'z'
    bgt     r2, r3, .rsak_adv
    ; A scalar name begins here. Intern it and read its known-I64 flag.
    move.q  r24, r20               ; remember, aot_native_var_intern preserves R20
    jsr     aot_native_var_intern
    beqz    r9, .rsak_no           ; unexpected non-scalar: be conservative
    la      r1, AOT_NATIVE_VAR_TAB
    load.q  r1, (r1)
    add.q   r1, r1, #0x4000
    add.q   r1, r1, r11
    load.b  r1, (r1)
    beqz    r1, .rsak_no           ; this operand is not proven I64
    move.q  r20, r24
    jsr     aot_skip_var_name
    bra     .rsak_loop
.rsak_adv:
    add.q   r20, r20, #1
    bra     .rsak_loop
.rsak_yes:
    pop     r20
    move.q  r9, #1
    rts
.rsak_no:
    pop     r20
    move.q  r9, r0
    rts

; aot_expr_contains_int - R9=1 iff the expression at R20 contains TK_INT before
; its top-level boundary. Preserves R20.
aot_native_vars_type_scan:
    move.q  r13, r20               ; token cursor
.tvs_line:
    beqz    r13, .tvs_done
    load.q  r12, (r13)             ; next line record
    beqz    r12, .tvs_done         ; terminator
    add.q   r13, r13, #16          ; first statement token
.tvs_stmt:
    load.b  r1, (r13)
    beqz    r1, .tvs_line_next
    move.q  r2, #0x20
    beq     r1, r2, .tvs_adv
    move.q  r2, #0x3A              ; ':'
    beq     r1, r2, .tvs_adv
    move.q  r2, #0xAC              ; THEN opens a nested statement
    beq     r1, r2, .tvs_adv
    move.q  r2, #0xAB              ; ELSE opens a nested statement
    beq     r1, r2, .tvs_adv
    move.q  r2, #0x8F              ; REM: comment to end of line
    beq     r1, r2, .tvs_line_next
    move.q  r2, #0x83              ; DATA: literal text to end of line
    beq     r1, r2, .tvs_line_next
    move.q  r2, #0x81              ; FOR
    beq     r1, r2, .tvs_for
    move.q  r2, #0x86              ; READ writes its targets via the evaluator
    beq     r1, r2, .tvs_clear_list
    move.q  r2, #0x84              ; INPUT likewise
    beq     r1, r2, .tvs_clear_list
    move.q  r2, #0xA4              ; GET likewise
    beq     r1, r2, .tvs_clear_list
    move.q  r2, #0xA5              ; SWAP moves payloads between records
    beq     r1, r2, .tvs_clear_list
    move.q  r2, #0xA1              ; CLEAR rebuilds variables at run time
    beq     r1, r2, .tvs_types_off
    move.q  r2, #0xA2              ; NEW likewise
    beq     r1, r2, .tvs_types_off
    move.q  r2, #0x95              ; LOAD likewise
    beq     r1, r2, .tvs_types_off
    move.q  r2, #0x87              ; LET keyword: assignment follows
    bne     r1, r2, .tvs_check_letter
    add.q   r13, r13, #1
.tvs_let_spaces:
    load.b  r1, (r13)
    move.q  r2, #0x20
    bne     r1, r2, .tvs_check_letter
    add.q   r13, r13, #1
    bra     .tvs_let_spaces
.tvs_check_letter:
    or.q    r2, r1, #0x20
    move.q  r3, #0x61
    blt     r2, r3, .tvs_skip_stmt
    move.q  r3, #0x7A
    bgt     r2, r3, .tvs_skip_stmt
    ; NAME = RHS candidate (an IF condition starting with a comparison
    ; never reaches the '=' check, and a spurious keep is harmless for a
    ; statement that writes nothing)
    move.q  r20, r13
    push    r12
    push    r13
    jsr     aot_parse_compiler_var_name
    pop     r13
    pop     r12
    beqz    r15, .tvs_skip_stmt
    load.b  r1, (r22)
    move.q  r2, #0x24              ; string variables have their own records
    beq     r1, r2, .tvs_skip_stmt
    move.q  r2, #0x28              ; array elements are not native scalars
    beq     r1, r2, .tvs_skip_stmt
.tvs_eq_spaces:
    load.b  r1, (r22)
    move.q  r2, #0x20
    bne     r1, r2, .tvs_eq_check
    add.q   r22, r22, #1
    bra     .tvs_eq_spaces
.tvs_eq_check:
    move.q  r2, #0x3D              ; '='
    beq     r1, r2, .tvs_have_assign
    move.q  r2, #0xBC              ; TK_EQUAL
    bne     r1, r2, .tvs_skip_stmt
.tvs_have_assign:
    add.q   r22, r22, #1
    move.q  r20, r22
    move.q  r23, r22               ; remember RHS start for the second check
    push    r12
    push    r13
    jsr     aot_int_expr_supported_nodiv
    pop     r13
    pop     r12
    beqz    r9, .tvs_clear_target
    move.q  r20, r23
    push    r12
    push    r13
    jsr     aot_rhs_scalars_all_known_i64
    pop     r13
    pop     r12
    bnez    r9, .tvs_skip_stmt     ; native I64 store from proven-I64 operands
.tvs_clear_target:
    move.q  r20, r13
    push    r12
    push    r13
    jsr     aot_native_var_flag_clear_r20
    pop     r13
    pop     r12
    bra     .tvs_skip_stmt
.tvs_for:
    add.q   r13, r13, #1
    move.q  r20, r13
    push    r12
    push    r13
    jsr     aot_try_native_for     ; parse only; R23 = 1 for the native form
    pop     r13
    pop     r12
    move.q  r1, #1
    beq     r23, r1, .tvs_skip_stmt ; native FOR/NEXT keeps the loop scalar I64
    move.q  r20, r13
    push    r12
    push    r13
    jsr     aot_native_var_flag_clear_r20
    pop     r13
    pop     r12
    bra     .tvs_skip_stmt
.tvs_clear_list:
    add.q   r13, r13, #1
.tvs_cl_loop:
    load.b  r1, (r13)
    beqz    r1, .tvs_line_next
    move.q  r2, #0x3A              ; ':' ends the target list
    beq     r1, r2, .tvs_stmt
    move.q  r2, #0x22              ; skip string literals (INPUT prompts)
    beq     r1, r2, .tvs_cl_str
    or.q    r2, r1, #0x20
    move.q  r3, #0x61
    blt     r2, r3, .tvs_cl_adv
    move.q  r3, #0x7A
    bgt     r2, r3, .tvs_cl_adv
    move.q  r20, r13
    push    r12
    push    r13
    jsr     aot_native_var_flag_clear_r20
    pop     r13
    pop     r12
    move.q  r13, r22               ; past the name (subscript contents are
    bra     .tvs_cl_loop           ; then walked as reads-become-clears, safe)
.tvs_cl_str:
    add.q   r13, r13, #1
.tvs_cl_str_loop:
    load.b  r1, (r13)
    beqz    r1, .tvs_line_next
    move.q  r2, #0x22
    beq     r1, r2, .tvs_cl_adv
    add.q   r13, r13, #1
    bra     .tvs_cl_str_loop
.tvs_cl_adv:
    add.q   r13, r13, #1
    bra     .tvs_cl_loop
.tvs_types_off:
    la      r1, AOT_NATIVE_TYPES_OFF
    move.q  r2, #1
    store.q r2, (r1)
    bra     .tvs_done              ; typing is off for the whole compile
.tvs_skip_stmt:
    load.b  r1, (r13)
    beqz    r1, .tvs_line_next
    move.q  r2, #0x3A
    beq     r1, r2, .tvs_stmt
    move.q  r2, #0xAC              ; THEN: classify the nested statement
    beq     r1, r2, .tvs_stmt
    move.q  r2, #0xAB              ; ELSE: classify the nested statement
    beq     r1, r2, .tvs_stmt
    move.q  r2, #0x22
    bne     r1, r2, .tvs_ss_adv
    add.q   r13, r13, #1
.tvs_ss_str:
    load.b  r1, (r13)
    beqz    r1, .tvs_line_next
    move.q  r2, #0x22
    beq     r1, r2, .tvs_ss_adv
    add.q   r13, r13, #1
    bra     .tvs_ss_str
.tvs_ss_adv:
    add.q   r13, r13, #1
    bra     .tvs_skip_stmt
.tvs_adv:
    add.q   r13, r13, #1
    bra     .tvs_stmt
.tvs_line_next:
    move.q  r13, r12
    bra     .tvs_line
.tvs_done:
    move.q  r9, #1
    rts

aot_emit_native_vars_init:
    ; No native scalars: keep the interpreter's own variable list, so
    ; fully delegated programmes behave exactly like interpreted RUN.
    la      r1, AOT_NATIVE_VAR_COUNT
    load.q  r1, (r1)
    bnez    r1, .aenvi_emit
    move.q  r9, #1
    rts
.aenvi_emit:
    move.q  r17, r8
    jsr     aot_text_room
    beqz    r8, .aenvi_fail
    move.q  r8, r17
    la      r9, aot_str_movel_r1
    move.q  r10, #AOT_NATIVE_VAR_SEG
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r2
    move.q  r10, r0
    jsr     aot_emit_movel
    la      r9, aot_str_store_q
    jsr     aot_emit_str
    la      r9, aot_str_movel_r1
    move.q  r10, #(AOT_NATIVE_VAR_SEG + 8)
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r2
    la      r1, AOT_NATIVE_VAR_COUNT
    load.q  r10, (r1)
    jsr     aot_emit_movel
    la      r9, aot_str_store_r2_r1
    jsr     aot_emit_str
    la      r9, aot_str_movel_r1
    move.q  r10, #(AOT_NATIVE_VAR_SEG + 12)
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r2
    move.q  r10, #AOT_NATIVE_VAR_MAX
    jsr     aot_emit_movel
    la      r9, aot_str_store_r2_r1
    jsr     aot_emit_str
    la      r9, aot_str_movel_r1
    move.q  r10, #(BASIC_STATE + ST_VAR_START)
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r2
    move.q  r10, #AOT_NATIVE_VAR_SEG
    jsr     aot_emit_movel
    la      r9, aot_str_store_q
    jsr     aot_emit_str
    la      r9, aot_str_movel_r1
    move.q  r10, #(BASIC_STATE + ST_VAR_END)
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r2
    move.q  r10, #AOT_NATIVE_VAR_SEG
    jsr     aot_emit_movel
    la      r9, aot_str_store_q
    jsr     aot_emit_str
    move.q  r17, r8
    la      r28, AOT_NATIVE_VAR_TAB
    load.q  r28, (r28)
    la      r1, AOT_NATIVE_VAR_COUNT
    load.q  r29, (r1)
    move.q  r14, r0
.aenvi_rec:
    bge     r14, r29, .aenvi_done
    jsr     aot_text_room
    beqz    r8, .aenvi_fail
    move.q  r8, r17
    lsl.q   r1, r14, #2
    add.q   r1, r28, r1
    load.l  r15, (r1)
    move.q  r13, r14
    mulu.q  r13, r13, #24
    add.q   r13, r13, #(AOT_NATIVE_VAR_SEG + 16)
    la      r9, aot_str_movel_r1
    move.q  r10, r13
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r2
    move.q  r10, r15
    jsr     aot_emit_movel
    la      r9, aot_str_store_r2_r1
    jsr     aot_emit_str
    la      r9, aot_str_movel_r1
    add.q   r10, r13, #4
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r2
    move.q  r10, r0
    jsr     aot_emit_movel
    la      r9, aot_str_store_r2_r1
    jsr     aot_emit_str
    la      r9, aot_str_movel_r1
    add.q   r10, r13, #8
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r2
    move.q  r10, #VAL_I64
    jsr     aot_emit_movel
    la      r9, aot_str_store_r2_r1
    jsr     aot_emit_str
    la      r9, aot_str_movel_r1
    add.q   r10, r13, #12
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r2
    move.q  r10, r0
    jsr     aot_emit_movel
    la      r9, aot_str_store_r2_r1
    jsr     aot_emit_str
    la      r9, aot_str_movel_r1
    add.q   r10, r13, #16
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r2
    move.q  r10, r0
    jsr     aot_emit_movel
    la      r9, aot_str_store_q
    jsr     aot_emit_str
    move.q  r17, r8
    add.q   r14, r14, #1
    bra     .aenvi_rec
.aenvi_done:
    move.q  r17, r8
    move.q  r9, #1
    rts
.aenvi_fail:
    move.q  r9, r0
    rts

; aot_emit_var_lookup - emit a numeric variable lookup for a pending assignment.
; RUN AOT calls resident var_lookup; standalone calls the bundled runtime slot.
; Runtime input R17 points at the variable token stream. Runtime output R8 is the
; value payload pointer. R8 = output cursor (in/out). Output R9 = 1 ok.
aot_try_native_for:
    move.q  r23, r0
    jsr     aot_skip_spaces
    load.b  r1, (r20)
    or.q    r2, r1, #0x20
    move.q  r3, #0x61
    blt     r2, r3, .tnf_no
    move.q  r3, #0x7A
    bgt     r2, r3, .tnf_no
    la      r3, AOT_FOR_TMP_VAR
    store.q r20, (r3)
    jsr     aot_skip_var_name
    load.b  r1, (r20)
    move.q  r2, #0x24              ; '$'
    beq     r1, r2, .tnf_no
    move.q  r2, #0x28              ; arrays are delegated
    beq     r1, r2, .tnf_no
    jsr     aot_skip_spaces
    load.b  r1, (r20)
    move.q  r2, #0xBC              ; TK_EQUAL
    beq     r1, r2, .tnf_eq
    move.q  r2, #0x3D
    bne     r1, r2, .tnf_no
.tnf_eq:
    add.q   r20, r20, #1
    jsr     aot_skip_spaces
    la      r3, AOT_FOR_TMP_START
    store.q r20, (r3)
    jsr     aot_find_for_to
    beqz    r9, .tnf_no
    move.q  r24, r22
    la      r3, AOT_FOR_TMP_START
    load.q  r20, (r3)
    jsr     aot_int_expr_supported_to
    beqz    r9, .tnf_no
    add.q   r20, r24, #1
    jsr     aot_skip_spaces
    la      r3, AOT_FOR_TMP_END
    store.q r20, (r3)
    jsr     aot_find_for_end_or_step
    beqz    r9, .tnf_no
    push    r23                    ; step expression start, or 0
    push    r22                    ; header boundary
    move.q  r24, r22
    la      r3, AOT_FOR_TMP_END
    load.q  r20, (r3)
    jsr     aot_int_expr_supported_to
    pop     r22
    pop     r23
    beqz    r9, .tnf_no
    beqz    r23, .tnf_default_step
    move.q  r20, r23
    jsr     aot_parse_for_step_const
    beqz    r9, .tnf_no
    bra     .tnf_yes
.tnf_default_step:
    la      r3, AOT_FOR_TMP_AFTER
    store.q r22, (r3)
    la      r3, AOT_FOR_TMP_STEP
    move.q  r1, #1
    store.q r1, (r3)
    la      r3, AOT_FOR_TMP_SIGN
    store.q r0, (r3)
.tnf_yes:
    la      r3, AOT_FOR_TMP_AFTER
    load.q  r20, (r3)
    move.q  r23, #1
    move.q  r9, #1
    rts
.tnf_no:
    move.q  r23, r0
    move.q  r9, #1
    rts

aot_find_for_to:
    move.q  r3, r20
    move.q  r4, r0
.fft_loop:
    load.b  r1, (r3)
    beqz    r1, .fft_fail
    move.q  r2, #0x3A
    beq     r1, r2, .fft_fail
    move.q  r2, #0x28
    bne     r1, r2, .fft_not_lparen
    add.q   r4, r4, #1
    bra     .fft_next
.fft_not_lparen:
    move.q  r2, #0x29
    bne     r1, r2, .fft_not_rparen
    beqz    r4, .fft_fail
    sub.q   r4, r4, #1
    bra     .fft_next
.fft_not_rparen:
    bnez    r4, .fft_next
    move.q  r2, #0xA9              ; TK_TO
    beq     r1, r2, .fft_found
.fft_next:
    add.q   r3, r3, #1
    bra     .fft_loop
.fft_found:
    move.q  r22, r3
    move.q  r9, #1
    rts
.fft_fail:
    move.q  r9, r0
    rts

; Output R22 = header boundary, R23 = STEP expression start or 0.
aot_find_for_end_or_step:
    move.q  r3, r20
    move.q  r4, r0
.ffes_loop:
    load.b  r1, (r3)
    beqz    r1, .ffes_boundary
    move.q  r2, #0x3A
    beq     r1, r2, .ffes_boundary
    move.q  r2, #0x28
    bne     r1, r2, .ffes_not_lparen
    add.q   r4, r4, #1
    bra     .ffes_next
.ffes_not_lparen:
    move.q  r2, #0x29
    bne     r1, r2, .ffes_not_rparen
    beqz    r4, .ffes_boundary
    sub.q   r4, r4, #1
    bra     .ffes_next
.ffes_not_rparen:
    bnez    r4, .ffes_next
    move.q  r2, #0xAE              ; TK_STEP
    beq     r1, r2, .ffes_step
.ffes_next:
    add.q   r3, r3, #1
    bra     .ffes_loop
.ffes_step:
    move.q  r22, r3
    add.q   r23, r3, #1
    move.q  r5, r23
.ffes_step_boundary_loop:
    load.b  r1, (r5)
    beqz    r1, .ffes_step_boundary
    move.q  r2, #0x3A
    beq     r1, r2, .ffes_step_boundary
    add.q   r5, r5, #1
    bra     .ffes_step_boundary_loop
.ffes_step_boundary:
    la      r3, AOT_FOR_TMP_AFTER
    store.q r5, (r3)
    move.q  r9, #1
    rts
.ffes_boundary:
    bnez    r4, .ffes_fail
    move.q  r22, r3
    move.q  r23, r0
    move.q  r9, #1
    rts
.ffes_fail:
    move.q  r9, r0
    rts

aot_parse_for_step_const:
    jsr     aot_skip_spaces
    move.q  r6, r0
    load.b  r1, (r20)
    move.q  r2, #0xB2              ; TK_MINUS
    beq     r1, r2, .pfsc_neg
    move.q  r2, #0x2D
    bne     r1, r2, .pfsc_digits
.pfsc_neg:
    move.q  r6, #1
    add.q   r20, r20, #1
    jsr     aot_skip_spaces
.pfsc_digits:
    jsr     aot_parse_one_uint
    beqz    r8, .pfsc_fail
    beqz    r22, .pfsc_fail
    jsr     aot_skip_spaces
    la      r3, AOT_FOR_TMP_AFTER
    load.q  r1, (r3)
    bne     r20, r1, .pfsc_fail
    la      r3, AOT_FOR_TMP_STEP
    store.q r22, (r3)
    la      r3, AOT_FOR_TMP_SIGN
    store.q r6, (r3)
    move.q  r9, #1
    rts
.pfsc_fail:
    move.q  r9, r0
    rts

aot_int_expr_supported_to:
    move.q  r3, r20
    move.q  r4, r0
    move.q  r5, r0
.iest_loop:
    beq     r3, r24, .iest_done
    load.b  r1, (r3)
    move.q  r2, #0x20
    beq     r1, r2, .iest_next
    move.q  r2, #0x29
    bne     r1, r2, .iest_not_rparen
    beqz    r4, .iest_fail
    sub.q   r4, r4, #1
    move.q  r5, #1
    bra     .iest_next
.iest_not_rparen:
    move.q  r2, #0x28
    bne     r1, r2, .iest_not_lparen
    add.q   r4, r4, #1
    move.q  r5, #1
    bra     .iest_next
.iest_not_lparen:
    move.q  r2, #0x30
    blt     r1, r2, .iest_ops
    move.q  r2, #0x39
    ble     r1, r2, .iest_digits
.iest_ops:
    move.q  r2, #0xB1
    beq     r1, r2, .iest_supported_one
    move.q  r2, #0xB2
    beq     r1, r2, .iest_supported_one
    move.q  r2, #0xB3
    beq     r1, r2, .iest_supported_one
    move.q  r2, #0xB4
    beq     r1, r2, .iest_supported_one
    move.q  r2, #0xB6
    beq     r1, r2, .iest_supported_one
    move.q  r2, #0xBF
    beq     r1, r2, .iest_supported_one
    move.q  r2, #0xCC
    beq     r1, r2, .iest_supported_one
    move.q  r2, #0x92
    bne     r1, r2, .iest_var
    add.q   r6, r3, #1
    beq     r6, r24, .iest_fail
    load.b  r1, (r6)
    move.q  r2, #0x07
    beq     r1, r2, .iest_ext_ok
    move.q  r2, #0x09
    bne     r1, r2, .iest_fail
.iest_ext_ok:
    add.q   r3, r3, #1
    move.q  r5, #1
    bra     .iest_next
.iest_var:
    or.q    r2, r1, #0x20
    move.q  r6, #0x61
    blt     r2, r6, .iest_fail
    move.q  r6, #0x7A
    bgt     r2, r6, .iest_fail
    move.q  r6, r3
.iest_var_scan:
    beq     r6, r24, .iest_var_ok
    load.b  r1, (r6)
    or.q    r7, r1, #0x20
    move.q  r2, #0x61
    blt     r7, r2, .iest_var_digit
    move.q  r2, #0x7A
    ble     r7, r2, .iest_var_adv
.iest_var_digit:
    move.q  r2, #0x30
    blt     r1, r2, .iest_var_end
    move.q  r2, #0x39
    bgt     r1, r2, .iest_var_end
.iest_var_adv:
    add.q   r6, r6, #1
    bra     .iest_var_scan
.iest_var_end:
    move.q  r2, #0x24
    beq     r1, r2, .iest_fail
    move.q  r2, #0x28
    beq     r1, r2, .iest_array
.iest_var_ok:
    move.q  r3, r6
    move.q  r5, #1
    bra     .iest_loop
.iest_array:
    move.q  r7, #1
    add.q   r6, r6, #1
.iest_array_loop:
    beq     r6, r24, .iest_fail
    load.b  r1, (r6)
    move.q  r2, #0x28
    beq     r1, r2, .iest_array_lparen
    move.q  r2, #0x29
    beq     r1, r2, .iest_array_rparen
    add.q   r6, r6, #1
    bra     .iest_array_loop
.iest_array_lparen:
    add.q   r7, r7, #1
    add.q   r6, r6, #1
    bra     .iest_array_loop
.iest_array_rparen:
    sub.q   r7, r7, #1
    add.q   r6, r6, #1
    bnez    r7, .iest_array_loop
    bra     .iest_var_ok
.iest_digits:
.iest_digit_loop:
    beq     r3, r24, .iest_digit_done
    load.b  r1, (r3)
    move.q  r2, #0x30
    blt     r1, r2, .iest_digit_done
    move.q  r2, #0x39
    bgt     r1, r2, .iest_digit_done
    add.q   r3, r3, #1
    bra     .iest_digit_loop
.iest_digit_done:
    move.q  r5, #1
    bra     .iest_loop
.iest_supported_one:
    move.q  r5, #1
.iest_next:
    add.q   r3, r3, #1
    bra     .iest_loop
.iest_done:
    bnez    r4, .iest_fail
    move.q  r9, r5
    rts
.iest_fail:
    move.q  r9, r0
    rts

aot_int_expr_supported:
    move.q  r3, r20
    move.q  r4, r0                  ; paren depth
    move.q  r5, r0                  ; saw any token
.ies_loop:
    load.b  r1, (r3)
    beqz    r1, .ies_done
    move.q  r2, #0x3A               ; ':'
    beq     r1, r2, .ies_done
    move.q  r2, #0x2C               ; ','
    beq     r1, r2, .ies_done
    move.q  r2, #0xAC
    beq     r1, r2, .ies_done
    move.q  r2, #0xAB
    beq     r1, r2, .ies_done
    move.q  r2, #0x20
    beq     r1, r2, .ies_next
    move.q  r2, #0x29               ; ')'
    bne     r1, r2, .ies_not_rparen
    beqz    r4, .ies_done
    sub.q   r4, r4, #1
    move.q  r5, #1
    bra     .ies_next
.ies_not_rparen:
    move.q  r2, #0x28               ; '('
    bne     r1, r2, .ies_not_lparen
    add.q   r4, r4, #1
    move.q  r5, #1
    bra     .ies_next
.ies_not_lparen:
    move.q  r2, #0x26               ; '&H' hex literal
    beq     r1, r2, .ies_uint_parse
    move.q  r2, #0x30
    blt     r1, r2, .ies_ops
    move.q  r2, #0x39
    ble     r1, r2, .ies_digits
.ies_ops:
    move.q  r2, #0xB1
    beq     r1, r2, .ies_supported_one
    move.q  r2, #0xB2
    beq     r1, r2, .ies_supported_one
    move.q  r2, #0xB3
    beq     r1, r2, .ies_supported_one
    move.q  r2, #0xB4
    beq     r1, r2, .ies_supported_one
    move.q  r2, #0xB6
    beq     r1, r2, .ies_supported_one
    move.q  r2, #0xBF
    beq     r1, r2, .ies_supported_one
    move.q  r2, #0xCC
    beq     r1, r2, .ies_supported_one
    move.q  r2, #0x92
    bne     r1, r2, .ies_var
    load.b  r1, 1(r3)
    move.q  r2, #0x07
    beq     r1, r2, .ies_ext_ok
    move.q  r2, #0x09
    bne     r1, r2, .ies_fail
.ies_ext_ok:
    add.q   r3, r3, #1
    move.q  r5, #1
    bra     .ies_next
.ies_var:
    or.q    r2, r1, #0x20
    move.q  r6, #0x61
    blt     r2, r6, .ies_fail
    move.q  r6, #0x7A
    bgt     r2, r6, .ies_fail
    move.q  r6, r3
.ies_var_scan:
    load.b  r1, (r6)
    or.q    r7, r1, #0x20
    move.q  r2, #0x61
    blt     r7, r2, .ies_var_digit
    move.q  r2, #0x7A
    ble     r7, r2, .ies_var_adv
.ies_var_digit:
    move.q  r2, #0x30
    blt     r1, r2, .ies_var_end
    move.q  r2, #0x39
    bgt     r1, r2, .ies_var_end
.ies_var_adv:
    add.q   r6, r6, #1
    bra     .ies_var_scan
.ies_var_end:
    move.q  r2, #0x24
    beq     r1, r2, .ies_fail
    move.q  r2, #0x28
    beq     r1, r2, .ies_array
    move.q  r3, r6
    move.q  r5, #1
    bra     .ies_loop
.ies_array:
    move.q  r7, #1
    add.q   r6, r6, #1
.ies_array_loop:
    load.b  r1, (r6)
    beqz    r1, .ies_fail
    move.q  r2, #0x28
    beq     r1, r2, .ies_array_lparen
    move.q  r2, #0x29
    beq     r1, r2, .ies_array_rparen
    add.q   r6, r6, #1
    bra     .ies_array_loop
.ies_array_lparen:
    add.q   r7, r7, #1
    add.q   r6, r6, #1
    bra     .ies_array_loop
.ies_array_rparen:
    sub.q   r7, r7, #1
    add.q   r6, r6, #1
    bnez    r7, .ies_array_loop
    move.q  r3, r6
    move.q  r5, #1
    bra     .ies_loop
.ies_digits:
.ies_digit_loop:
    load.b  r1, (r3)
    move.q  r2, #0x30
    blt     r1, r2, .ies_digit_done
    move.q  r2, #0x39
    bgt     r1, r2, .ies_digit_done
    add.q   r3, r3, #1
    bra     .ies_digit_loop
.ies_digit_done:
    move.q  r5, #1
    bra     .ies_loop
.ies_uint_parse:
    push    r4
    move.q  r8, r3
    jsr     aot_parse_uint
    pop     r4
    beqz    r9, .ies_fail
    move.q  r3, r10
    move.q  r5, #1
    bra     .ies_loop
.ies_supported_one:
    move.q  r5, #1
.ies_next:
    add.q   r3, r3, #1
    bra     .ies_loop
.ies_done:
    bnez    r4, .ies_fail
    move.q  r9, r5
    rts
.ies_fail:
    move.q  r9, r0
    rts

; Recursive-descent emitter for the supported integer subset.
aot_apply_src_dir:
    la      r1, EHBASIC_PRIV_LOAD_DIR_BUF
    load.b  r2, (r1)
    beqz    r2, .asd_done          ; empty -> File I/O root, no change
    move.q  r3, r0                 ; R3 = dirLen
.asd_dlen:
    add.q   r4, r1, r3
    load.b  r5, (r4)
    beqz    r5, .asd_dlen_done
    add.q   r3, r3, #1
    bra     .asd_dlen
.asd_dlen_done:
    la      r6, EHBASIC_PRIV_NAME_BUF
    move.q  r4, r0                 ; R4 = nameLen (excluding the null)
.asd_nlen:
    add.q   r7, r6, r4
    load.b  r5, (r7)
    beqz    r5, .asd_nlen_done
    add.q   r4, r4, #1
    bra     .asd_nlen
.asd_nlen_done:
    add.q   r5, r3, r4             ; dirLen + nameLen + 1 (null)
    add.q   r5, r5, #1
    move.q  r7, #256
    bgt     r5, r7, .asd_done      ; would overflow the buffer -> use bare name
    ; Shift [0..nameLen] (incl null) right by dirLen, copying high index first so
    ; the overlapping source bytes are not clobbered before they are read.
    move.q  r7, r4                 ; i = nameLen (null position)
.asd_shift:
    add.q   r1, r6, r7
    load.b  r5, (r1)
    add.q   r2, r7, r3
    add.q   r2, r6, r2
    store.b r5, (r2)
    beqz    r7, .asd_prepend
    sub.q   r7, r7, #1
    bra     .asd_shift
.asd_prepend:
    la      r1, EHBASIC_PRIV_LOAD_DIR_BUF
    move.q  r7, r0
.asd_pre_loop:
    beq     r7, r3, .asd_done
    add.q   r2, r1, r7
    load.b  r5, (r2)
    add.q   r4, r6, r7
    store.b r5, (r4)
    add.q   r7, r7, #1
    bra     .asd_pre_loop
.asd_done:
    rts

; aot_make_asm_name - derive EHBASIC_PRIV_ASM_NAME_BUF = "NAME.asm" from EHBASIC_PRIV_NAME_BUF
; ("NAME.ie64"). The validated COMPILE name always ends in ".ie64" (case-
; insensitive), so the last five characters are stripped and ".asm" appended.
; Clobbers R1-R7
aot_make_asm_name:
    la      r1, EHBASIC_PRIV_NAME_BUF
    move.q  r2, r0                  ; length
.mn_len:
    load.b  r3, (r1)
    beqz    r3, .mn_copy
    add.q   r1, r1, #1
    add.q   r2, r2, #1
    bra     .mn_len
.mn_copy:
    sub.q   r4, r2, #5             ; characters to copy (strip ".ie64")
    la      r5, EHBASIC_PRIV_NAME_BUF
    la      r6, EHBASIC_PRIV_ASM_NAME_BUF
    move.q  r7, r0
.mn_cp:
    bge     r7, r4, .mn_ext
    load.b  r3, (r5)
    store.b r3, (r6)
    add.q   r5, r5, #1
    add.q   r6, r6, #1
    add.q   r7, r7, #1
    bra     .mn_cp
.mn_ext:
    la      r5, aot_str_asmext
.mn_app:
    load.b  r3, (r5)
    store.b r3, (r6)               ; copies the trailing null too
    beqz    r3, .mn_done
    add.q   r5, r5, #1
    add.q   r6, r6, #1
    bra     .mn_app
.mn_done:
    rts

aot_str_asmext:
    dc.b    ".asm", 0
    align 4

; aot_do_assemble - the ASSEMBLE "name" backend: read NAME.asm, assemble it at
; PROGRAM_START with the in-guest private assembler, and write NAME.ie64. This is
; a general user-assembly assembler: it assembles self-contained IE64 source
; (labels, dc.b/w/l/q, align, and ie64.inc named constants resolved through the
; baked assembler_consttab). `include "ie64.inc"` is accepted as a no-op because
; those constants are already available; any other include, or an unknown
; directive/mnemonic, is a clean assembler error. It performs no bundling of its
; own, so it only assembles what the source defines. COMPILE and TRANSPILE emit
; fully self-contained source, including every required typed runtime helper, so
; ASSEMBLE reassembles a transpiled .asm into the same image COMPILE writes
; directly. Both files sit beside the most recently
; LOADed programme (or the File I/O root). EHBASIC_PRIV_NAME_BUF holds the validated
; ".ie64" output name on entry. Uses R16; saves/restores callee-saved registers.
; Source files are limited by the retained AOT arena available under
; CR_RAM_SIZE_BYTES. A larger file is refused as a file error, never staged into
; the compiler workspace.
; Output R8 = status: 0 success, 2 out of memory, 3 assembler error,
;                     4 file error (input unreadable, over-large, or unwritable).
aot_do_assemble:
    push r14
    push r17
    push r18
    push r19
    push r20
    push r21
    push r22
    push r23
    push r24
    push r25
    push r26
    push r27
    push r28
    push r29
    jsr     aot_do_assemble_inner
    pop r29
    pop r28
    pop r27
    pop r26
    pop r25
    pop r24
    pop r23
    pop r22
    pop r21
    pop r20
    pop r19
    pop r18
    pop r17
    pop r14
    rts
; RUN AOT, TRANSPILE, COMPILE and ASSEMBLE size their generated text and native
; code buffers dynamically from the retained top-of-RAM AOT arena. These legacy
; constants remain only so older standalone unit snippets that refer to them
; still assemble.
AOT_ASM_SRC_CAP equ 0x20000000
AOT_GEN_TEXT_BYTES equ 0x20000000
AOT_RUN_CODE_BYTES equ 0x8000000
AOT_COMPILE_CODE_BYTES equ AOT_RUN_CODE_BYTES
AOT_DYNAMIC_RESERVE_BYTES equ 0x400000

aot_dynamic_text_code_sizes:
    la      r1, AOT_ARENA_HIGH_NEXT
    load.q  r2, (r1)
    ; Size within the retained top-of-RAM AOT reserve (2 GiB, shift-built
    ; because 32-bit immediates sign-extend), not the whole span down to
    ; the allocation floor: sizing from the full span parks the code
    ; buffer just above the floor, far below the frontier, and on split
    ; layouts inside the interpreter heap. Hosts whose frontier is below
    ; the reserve keep the legacy floor and see identical sizes.
    move.q  r3, #1
    lsl.q   r3, r3, #31            ; retained AOT reserve
    sub.q   r3, r2, r3             ; frontier - reserve = sizing floor
    move.q  r4, #AOT_ALLOC_FLOOR
    bge     r3, r4, .dtcs_have_floor
    move.q  r3, r4
.dtcs_have_floor:
    sub.q   r2, r2, r3             ; retained arena bytes remaining
    move.q  r3, #(AOT_SYMTAB_BYTES + AOT_DYNAMIC_RESERVE_BYTES)
    ble     r2, r3, .dtcs_fail
    sub.q   r2, r2, r3             ; bytes available for generated text + code
    lsr.q   r25, r2, #1            ; native-code buffer
    sub.q   r24, r2, r25           ; generated assembly-text buffer
    move.q  r9, #1
    rts
.dtcs_fail:
    move.q  r9, r0
    rts

aot_dynamic_text_scratch_sizes:
    la      r1, AOT_ARENA_HIGH_NEXT
    load.q  r2, (r1)
    ; Same reserve-bounded window as aot_dynamic_text_code_sizes.
    move.q  r3, #1
    lsl.q   r3, r3, #31            ; retained AOT reserve
    sub.q   r3, r2, r3             ; frontier - reserve = sizing floor
    move.q  r4, #AOT_ALLOC_FLOOR
    bge     r3, r4, .dtss_have_floor
    move.q  r3, r4
.dtss_have_floor:
    sub.q   r2, r2, r3             ; retained arena bytes remaining
    move.q  r3, #(0x18000 + AOT_DYNAMIC_RESERVE_BYTES)
    ble     r2, r3, .dtss_fail
    sub.q   r24, r2, r3            ; generated assembly-text buffer
    move.q  r25, #0x18000          ; programme relocation scratch
    move.q  r9, #1
    rts
.dtss_fail:
    move.q  r9, r0
    rts

aot_do_assemble_inner:
    jsr     aot_alloc_reset
    jsr     aot_dynamic_text_code_sizes
    beqz    r9, .as_oom
    ; Allocate the source-text buffer from the retained 64-bit arena and read it
    ; through FILE_DATA_PTR64. FILE_READ_MAX is set to the allocated size before
    ; OP_READ, so an over-large source is refused before any byte is staged.
    move.q  r8, r24
    jsr     aot_alloc64
    bnez    r9, .as_oom
    la      r1, AOT_DC_TEXT
    store.q r8, (r1)
    la      r1, AOT_DC_TLEN
    store.q r24, (r1)              ; temporary: source buffer capacity
    move.q  r8, r25                ; machine-code output buffer
    jsr     aot_alloc64
    bnez    r9, .as_oom
    la      r1, AOT_DC_CODE
    store.q r8, (r1)
    add.q   r2, r8, r25
    la      r1, AOT_CODE_END
    store.q r2, (r1)               ; bound the buffer so a big source reports OOM
    move.q  r8, #AOT_SYMTAB_BYTES   ; private-assembler symbol table
    jsr     aot_alloc64
    bnez    r9, .as_oom
    la      r1, AOT_SYMTAB_PTR
    store.q r8, (r1)

    ; Derive NAME.asm (the input) from EHBASIC_PRIV_NAME_BUF (NAME.ie64), in the source
    ; directory, exactly where COMPILE/TRANSPILE place their .asm sidecar.
    jsr     aot_apply_src_dir
    jsr     aot_make_asm_name       ; EHBASIC_PRIV_ASM_NAME_BUF = "...asm"

    ; Cap the read at the device BEFORE it copies anything: FILE_READ_MAX makes the
    ; File I/O device refuse a file larger than the source buffer without staging a
    ; single byte into guest memory. The cap is one-shot.
    la      r1, FILE_READ_MAX
    la      r2, AOT_DC_TLEN
    load.q  r2, (r2)
    sub.q   r2, r2, #1
    store.l r2, (r1)

    ; Read NAME.asm into the source buffer via the IE64 64-bit File I/O extension.
    la      r1, FILE_NAME_PTR
    la      r2, EHBASIC_PRIV_ASM_NAME_BUF
    store.l r2, (r1)
    la      r1, FILE_DATA_PTR64
    la      r2, AOT_DC_TEXT
    load.q  r2, (r2)
    store.q r2, (r1)
    la      r1, FILE_CTRL
    move.q  r2, #FILE_OP_READ
    store.l r2, (r1)
    la      r1, FILE_STATUS
    load.l  r1, (r1)
    bnez    r1, .as_file_err        ; not found, or over the FILE_READ_MAX cap

    ; Defence in depth: FILE_READ_MAX already rejected anything >= the cap, so this
    ; never trips, but it keeps the terminator write below provably in-bounds.
    la      r1, FILE_RESULT_LEN
    load.l  r2, (r1)
    la      r4, AOT_DC_TLEN
    load.q  r4, (r4)
    bge     r2, r4, .as_file_err

    ; Null-terminate the source text at buffer + bytesRead so the assembler line
    ; scanner stops at the end of file.
    la      r3, AOT_DC_TEXT
    load.q  r3, (r3)
    add.q   r3, r3, r2
    store.b r0, (r3)

    ; Assemble at PROGRAM_START into the code buffer.
    la      r8, AOT_DC_TEXT
    load.q  r8, (r8)
    move.q  r9, #PROGRAM_START
    la      r1, AOT_DC_CODE
    load.q  r10, (r1)
    jsr     aot_asm_program         ; R8=status R9=codeLen
    move.q  r1, #2
    beq     r8, r1, .as_oom         ; output overflow
    beqz    r8, .as_asm_err         ; unknown mnemonic / unresolved symbol / bad operand
    la      r1, AOT_DC_CLEN
    store.q r9, (r1)

    ; Write NAME.ie64 (machine code).
    la      r8, EHBASIC_PRIV_NAME_BUF
    la      r1, AOT_DC_CODE
    load.q  r9, (r1)
    la      r1, AOT_DC_CLEN
    load.q  r10, (r1)
    jsr     aot_file_write
    bnez    r8, .as_file_err
    move.q  r8, r0                  ; 0 = success
    rts
.as_oom:
    move.q  r8, #2
    rts
.as_asm_err:
    move.q  r8, #3
    rts
.as_file_err:
    move.q  r8, #4
    rts

; aot_do_transpile - the TRANSPILE "name" backend: transpile the stored
; programme to IE64 assembly text and write NAME.asm, the first half of COMPILE
; with no assembly or NAME.ie64. EHBASIC_PRIV_NAME_BUF holds the validated ".ie64" name
; (aot_make_asm_name derives NAME.asm from it, identical to COMPILE).
; Uses R16 (the live interpreter state base); does not clobber it. Saves/restores
; the interpreter's callee-saved registers so the REPL survives.
; Output R8 = status: 0 success, 1 unsupported, 2 out of memory, 4 file error.
aot_do_transpile:
    push r14
    push r17
    push r18
    push r19
    push r20
    push r21
    push r22
    push r23
    push r24
    push r25
    push r26
    push r27
    push r28
    push r29
    jsr     aot_do_transpile_inner
    pop r29
    pop r28
    pop r27
    pop r26
    pop r25
    pop r24
    pop r23
    pop r22
    pop r21
    pop r20
    pop r19
    pop r18
    pop r17
    pop r14
    rts
aot_do_transpile_inner:
    jsr     aot_alloc_reset
    jsr     aot_dynamic_text_scratch_sizes
    beqz    r9, .tp_oom
    move.q  r8, r24                ; asm-text buffer (holds the self-contained
                                   ; dc.b'd closure/blob/programme, like COMPILE)
    jsr     aot_alloc64
    bnez    r9, .tp_oom
    la      r1, AOT_DC_TEXT
    store.q r8, (r1)
    la      r1, AOT_DC_TLEN
    store.q r24, (r1)              ; temporary: asm-text capacity
    move.q  r8, r25                ; programme relocation scratch buffer
    jsr     aot_alloc64            ; (aot_emit_bundled_data uses AOT_DC_CODE as scratch)
    bnez    r9, .tp_oom
    la      r1, AOT_DC_CODE
    store.q r8, (r1)
    add.q   r2, r8, #0x18000
    la      r1, AOT_CODE_END
    store.q r2, (r1)               ; bound the scratch so a big programme reports OOM

    ; Transpile the live programme in standalone mode (ends with halt), exactly
    ; as COMPILE does, so the written asm matches COMPILE's NAME.asm sidecar.
    load.q  r8, (r16)              ; first line record (state[ST_PROG_START])
    la      r1, AOT_DC_TEXT
    load.q  r9, (r1)
    move.q  r10, #1                ; standalone
    la      r11, AOT_DC_TLEN
    load.q  r11, (r11)
    add.q   r11, r9, r11           ; text buffer end
    jsr     compiler_transpile_adapter ; replacement typed compiler
    move.q  r1, #2
    beq     r8, r1, .tp_oom        ; text buffer overflow
    beqz    r8, .tp_unsupported    ; unsupported statement

    ; The replacement compiler emits every required helper into the text, so
    ; the generated .asm is already self-contained: record its length.
    la      r1, AOT_DC_TLEN
    store.q r9, (r1)

    ; Write NAME.asm beside the most recently LOADed programme (or the File I/O
    ; root if none), identical to COMPILE's sidecar placement.
    jsr     aot_apply_src_dir
    jsr     aot_make_asm_name
    la      r8, EHBASIC_PRIV_ASM_NAME_BUF
    la      r1, AOT_DC_TEXT
    load.q  r9, (r1)
    la      r1, AOT_DC_TLEN
    load.q  r10, (r1)
    jsr     aot_file_write
    bnez    r8, .tp_file_err
    move.q  r8, r0                 ; 0 = success
    rts
.tp_unsupported:
    move.q  r8, #1
    rts
.tp_oom:
    move.q  r8, #2
    rts
.tp_file_err:
    move.q  r8, #4
    rts

; aot_do_compile - the COMPILE "name" backend: transpile the stored programme,
; assemble a standalone image at PROGRAM_START, and write both NAME.ie64 (the
; machine code) and NAME.asm (the transpiled source). EHBASIC_PRIV_NAME_BUF holds the
; validated ".ie64" output name. Prints its own diagnostics on failure.
; Uses R16 (the live interpreter state base); does not clobber it.
; Saves/restores the interpreter's callee-saved registers (see aot_do_run_aot)
; so the REPL survives after COMPILE returns.
aot_do_compile:
    push r14
    push r17
    push r18
    push r19
    push r20
    push r21
    push r22
    push r23
    push r24
    push r25
    push r26
    push r27
    push r28
    push r29
    jsr     aot_do_compile_inner
    pop r29
    pop r28
    pop r27
    pop r26
    pop r25
    pop r24
    pop r23
    pop r22
    pop r21
    pop r20
    pop r19
    pop r18
    pop r17
    pop r14
    rts
aot_do_compile_inner:
    jsr     aot_alloc_reset
    jsr     aot_dynamic_text_code_sizes
    beqz    r9, .dc_oom
    move.q  r8, r24                ; asm-text buffer: holds
                                   ; the user code plus the fp_print closure, runtime
                                   ; blob (~53 KiB -> over 300 KiB as dc.b) and programme
                                   ; emitted as self-contained dc.b data.
    jsr     aot_alloc64
    bnez    r9, .dc_oom
    la      r1, AOT_DC_TEXT
    store.q r8, (r1)
    la      r1, AOT_DC_TLEN
    store.q r24, (r1)              ; temporary: asm-text capacity
    move.q  r8, r25                ; machine-code buffer:
                                   ; holds user code + fp_print closure + runtime
                                   ; bootstrap and bundled payload copies.
    jsr     aot_alloc64
    bnez    r9, .dc_oom
    la      r1, AOT_DC_CODE
    store.q r8, (r1)
    move.q  r8, #AOT_SYMTAB_BYTES   ; private-assembler symbol table (64-bit workspace)
    jsr     aot_alloc64
    bnez    r9, .dc_oom
    la      r1, AOT_SYMTAB_PTR
    store.q r8, (r1)

    ; Bound the code buffer so a large programme reports OOM, not corruption.
    la      r1, AOT_DC_CODE
    load.q  r2, (r1)
    add.q   r2, r2, r25
    la      r1, AOT_CODE_END
    store.q r2, (r1)

    ; Transpile the live programme in standalone mode (ends with halt).
    load.q  r8, (r16)              ; first line record (state[ST_PROG_START])
    la      r1, AOT_DC_TEXT
    load.q  r9, (r1)
    move.q  r10, #1                ; standalone
    la      r11, AOT_DC_TLEN
    load.q  r11, (r11)
    add.q   r11, r9, r11           ; text buffer end
    jsr     compiler_transpile_adapter ; replacement typed compiler
    move.q  r1, #2
    beq     r8, r1, .dc_oom        ; text buffer overflow
    beqz    r8, .dc_unsupported    ; unsupported statement
    ; The replacement compiler emits every required helper into the text, so
    ; the generated .asm is already self-contained: record its length.
    la      r1, AOT_DC_TLEN
    store.q r9, (r1)

    ; Assemble once at link base PROGRAM_START (the standalone image entry point).
    ; The returned length already covers the user code plus the dc.b'd closure,
    ; blob and programme, so it is the complete image length.
    la      r1, AOT_DC_TEXT
    load.q  r8, (r1)
    move.q  r9, #PROGRAM_START
    la      r1, AOT_DC_CODE
    load.q  r10, (r1)
    jsr     aot_asm_program        ; R8=status R9=imageLen
    move.q  r1, #2
    beq     r8, r1, .dc_oom        ; code buffer overflow
    beqz    r8, .dc_asm_err
    la      r1, AOT_DC_CLEN
    store.q r9, (r1)

    ; Write the output beside the most recently LOADed programme (or the File I/O
    ; root if none): prepend the tracked source directory to the validated name.
    jsr     aot_apply_src_dir

    ; Write NAME.ie64 (machine code).
    la      r8, EHBASIC_PRIV_NAME_BUF
    la      r1, AOT_DC_CODE
    load.q  r9, (r1)
    la      r1, AOT_DC_CLEN
    load.q  r10, (r1)
    jsr     aot_file_write
    bnez    r8, .dc_file_err

    ; Write NAME.asm (transpiled source) beside it.
    jsr     aot_make_asm_name
    la      r8, EHBASIC_PRIV_ASM_NAME_BUF
    la      r1, AOT_DC_TEXT
    load.q  r9, (r1)
    la      r1, AOT_DC_TLEN
    load.q  r10, (r1)
    jsr     aot_file_write
    bnez    r8, .dc_file_err
    move.q  r8, r0                  ; 0 = success
    rts

; Status codes (the caller prints the matching message; keeps this module free
; of REPL message dependencies so it can be unit-tested standalone):
;   1 = unsupported statement, 2 = out of memory, 3 = assembler error,
;   4 = file write error.
.dc_unsupported:
    move.q  r8, #1
    rts
.dc_oom:
    move.q  r8, #2
    rts
.dc_asm_err:
    move.q  r8, #3
    rts
.dc_file_err:
    move.q  r8, #4
    rts

; ============================================================================
; RUN AOT path
; ============================================================================
; aot_do_run_aot - compile the stored programme into a top-of-RAM arena and run
; it in place. The compiled code ends with rts and returns here. Reuses the
; COMPILE scratch slots (the two commands never run concurrently).
; Uses R16 (live interpreter state base); does not clobber it.
; Output R8 = status: 0 success, 1 unsupported, 2 out of memory, 3 asm error.
; The transpiler/assembler use R17-R29 as working state, but the interpreter
; (read_line, the line editor, exec) keeps persistent state in those callee-
; saved registers, so RUN AOT / COMPILE must save and restore them or the REPL
; breaks on the next command. R16 (the state-block base) is never clobbered.
; The wrapper does this once around the whole compile so the inner routines can
; use the registers freely. RUN AOT / COMPILE are one-shot, so the extra
; push/pop is not on any hot path.
aot_do_run_aot:
    push r14
    push r17
    push r18
    push r19
    push r20
    push r21
    push r22
    push r23
    push r24
    push r25
    push r26
    push r27
    push r28
    push r29
    jsr     aot_do_run_aot_inner
    pop r29
    pop r28
    pop r27
    pop r26
    pop r25
    pop r24
    pop r23
    pop r22
    pop r21
    pop r20
    pop r19
    pop r18
    pop r17
    pop r14
    rts
aot_do_run_aot_inner:
    jsr     aot_alloc_reset
    jsr     aot_native_vars_alloc
    beqz    r9, .ra_oom
    load.q  r20, (r16)
    jsr     aot_native_vars_prescan
    beqz    r9, .ra_oom
    load.q  r20, (r16)
    jsr     aot_native_vars_type_scan
    beqz    r9, .ra_oom
    la      r1, AOT_LAST_OOM_STAGE
    store.q r0, (r1)
    jsr     aot_dynamic_text_code_sizes
    beqz    r9, .ra_oom
    move.q  r8, r24                ; asm-text buffer
    jsr     aot_alloc64
    la      r1, AOT_LAST_OOM_STAGE
    move.q  r2, #1                 ; RUN AOT asm-text allocation
    store.q r2, (r1)
    bnez    r9, .ra_oom
    la      r1, AOT_DC_TEXT
    store.q r8, (r1)
    la      r1, AOT_DC_TLEN
    store.q r24, (r1)              ; temporary: asm-text capacity
    move.q  r8, r25                ; code arena (executed in place)
    jsr     aot_alloc64
    la      r1, AOT_LAST_OOM_STAGE
    move.q  r2, #2                 ; RUN AOT code allocation
    store.q r2, (r1)
    bnez    r9, .ra_oom
    la      r1, AOT_DC_CODE
    store.q r8, (r1)
    move.q  r8, #AOT_SYMTAB_BYTES   ; private-assembler symbol table
    jsr     aot_alloc64
    la      r1, AOT_LAST_OOM_STAGE
    move.q  r2, #3                 ; RUN AOT symbol-table allocation
    store.q r2, (r1)
    bnez    r9, .ra_oom
    la      r1, AOT_SYMTAB_PTR
    store.q r8, (r1)

    ; Bound the code arena so a large programme reports OOM, not corruption.
    la      r1, AOT_DC_CODE
    load.q  r8, (r1)
    add.q   r2, r8, r25
    la      r1, AOT_CODE_END
    store.q r2, (r1)

    ; Transpile in arena mode (ends with rts so control returns here).
    load.q  r8, (r16)              ; first line record
    la      r1, AOT_DC_TEXT
    load.q  r9, (r1)
    move.q  r10, r0                ; arena mode
    la      r11, AOT_DC_TLEN
    load.q  r11, (r11)
    add.q   r11, r9, r11           ; text buffer end
    la      r1, AOT_LAST_OOM_STAGE
    move.q  r2, #4                 ; RUN AOT transpiler text overflow
    store.q r2, (r1)
    jsr     compiler_transpile_adapter
    move.q  r1, #2
    beq     r8, r1, .ra_oom        ; text buffer overflow
    beqz    r8, .ra_unsupported
    ; The replacement compiler emits every required helper into the text, so
    ; the arena assembly is already self-contained: record its length.
    la      r1, AOT_DC_TLEN
    store.q r9, (r1)              ; replace temporary capacity with text length

    ; Assemble at link base = the arena address.
    la      r1, AOT_DC_TEXT
    load.q  r8, (r1)
    la      r1, AOT_DC_CODE
    load.q  r9, (r1)
    move.q  r10, r9                ; destination = arena
    la      r1, AOT_LAST_OOM_STAGE
    move.q  r2, #5                 ; RUN AOT assembler code overflow
    store.q r2, (r1)
    jsr     aot_asm_program
    move.q  r1, #2
    beq     r8, r1, .ra_oom        ; code buffer overflow
    beqz    r8, .ra_asm_err
    la      r1, AOT_DC_CLEN
    store.q r9, (r1)

    ; Invalidate the IE64 JIT cache before executing. The arena address is
    ; reused on every RUN AOT (the allocator resets to the same top of RAM), but
    ; the freshly assembled bytes were written with ordinary stores, which do not
    ; invalidate JIT-translated blocks. Without this, a second RUN AOT after
    ; editing the programme would run the previous compilation. tlbflush also
    ; sets jitNeedInval so the dispatcher drops the stale block.
    tlbflush

    ; Clear the runtime-error slot; compiled code sets it (e.g. RETURN without a
    ; matching GOSUB) so a runtime fault is reported instead of silent success.
    la      r1, AOT_RT_ERR
    store.q r0, (r1)

    ; Re-establish the interpreter register conventions the arena code and bundled
    ; helpers expect: R16 (state base) is never clobbered, but the compiler used
    ; R26/R27 as scratch, so refresh the cached TERM_OUT/TERM_STATUS pointers.
    la      r26, TERM_OUT
    la      r27, TERM_STATUS

    ; Initialise the interpreter run-state that delegated leaf handlers rely on,
    ; mirroring exec_run (a RUN AOT restarts the programme): running mode, empty
    ; GOSUB/FOR stack, reset DATA pointer, cleared IF/ELSE boundary, and R14 at the
    ; first line for error reporting.
    add.q   r1, r16, #ST_DIRECT_MODE
    store.l r0, (r1)
    jsr     exec_reset_control_stack
    add.q   r1, r16, #ST_DATA_PTR
    store.q r0, (r1)
    ; The arena prologue republishes ST_VAR_START at the fresh native
    ; variable segment; drop cached variable-record pointers so resident
    ; lookups cannot keep serving records from the previous segment.
    la      r1, var_cache
    move.q  r2, #8
.ra_var_cache_clear:
    store.q r0, (r1)
    add.q   r1, r1, #8
    sub.q   r2, r2, #1
    bnez    r2, .ra_var_cache_clear
    la      r1, if_else_boundary
    store.q r0, (r1)
    load.q  r14, (r16)             ; R14 = first line record
    ; Clear any pending native continuation: a fresh RUN AOT restarts the programme,
    ; so a stale STOP resume address from a previous run must not be honoured by CONT.
    la      r1, AOT_CONT_PC
    store.q r0, (r1)

    ; Execute the native code from the arena base.
    ; Its trailing rts returns control here.
    la      r1, AOT_DC_CODE
    load.q  r8, (r1)
    la      r2, AOT_SAVED_SP
    store.q r31, (r2)
    jsr     (r8)
    la      r1, AOT_RT_ERR
    load.q  r1, (r1)
    bnez    r1, .ra_runtime_err
    move.q  r8, r0                 ; success
    rts
.ra_runtime_err:
    move.q  r8, #4                 ; runtime fault (currently RETURN without GOSUB)
    rts
.ra_unsupported:
    move.q  r8, #1
    rts
.ra_oom:
    move.q  r8, #2
    rts
.ra_asm_err:
    move.q  r8, #3
    rts
; Compatibility adapter for RUN AOT, TRANSPILE and COMPILE. It replaces the
; legacy token transpiler while retaining the established allocation, private
; assembly and file orchestration around it. Inputs and return values match
; aot_transpile's historical contract.
compiler_transpile_adapter:
    move.q  r18, r8
    move.q  r19, r9
    move.q  r20, r11
    move.q  r21, r10
    ; Each invocation owns a fresh output span. Do not inherit a prior
    ; overflow latch or bound from RUN AOT, TRANSPILE or COMPILE.
    la      r1, AOT_EMIT_OVERFLOW
    store.q r0, (r1)
    la      r1, AOT_TEXT_END
    store.q r20, (r1)
    add.q   r1, r16, #ST_PROG_END
    load.q  r22, (r1)
    sub.q   r23, r22, r18
    lsl.q   r23, r23, #6
    move.q  r1, #0x20000
    bge     r23, r1, .cta_ir_size
    move.q  r23, r1
.cta_ir_size:
    move.q  r8, r23
    jsr     aot_alloc64
    bnez    r9, .cta_oom
    move.q  r24, r8
    add.q   r25, r24, r23
    lsr.q   r26, r23, #1
    move.q  r8, r26
    jsr     aot_alloc64
    bnez    r9, .cta_oom
    move.q  r27, r8
    add.q   r28, r27, r26
    move.q  r8, r24
    move.q  r9, r25
    jsr     compiler_ir_init
    bnez    r8, .cta_unsupported
    move.q  r8, r18
    move.q  r9, r22
    move.q  r10, r24
    jsr     compiler_parse_program
    bnez    r8, .cta_unsupported
    move.q  r8, r24
    jsr     compiler_optimise_ir
    bnez    r8, .cta_unsupported
    move.q  r29, #COMPILER_TARGET_ARENA
    beqz    r21, .cta_target_ready
    move.q  r29, #COMPILER_TARGET_STANDALONE
.cta_target_ready:
    move.q  r8, r24
    move.q  r9, r27
    move.q  r10, r28
    move.q  r11, r29
    jsr     compiler_lower_ir
    move.q  r1, #1
    beq     r8, r1, .cta_oom
    bnez    r8, .cta_unsupported
    move.q  r25, r9
    move.q  r13, #PROG_START
    move.q  r1, #COMPILER_TARGET_STANDALONE
    beq     r29, r1, .cta_emit
    la      r1, AOT_DC_CODE
    load.q  r13, (r1)
.cta_emit:
    move.q  r1, #COMPILER_TARGET_ARENA
    bne     r29, r1, .cta_emit_compiler
    push    r18
    push    r19
    push    r20
    push    r21
    push    r22
    push    r23
    push    r24
    push    r25
    push    r26
    push    r27
    push    r28
    push    r29
    move.q  r8, r19
    jsr     aot_emit_native_vars_init
    move.q  r1, r8
    move.q  r2, r9
    pop     r29
    pop     r28
    pop     r27
    pop     r26
    pop     r25
    pop     r24
    pop     r23
    pop     r22
    pop     r21
    pop     r20
    pop     r19
    pop     r18
    beqz    r2, .cta_native_init_oom
    move.q  r19, r1
.cta_emit_compiler:
    move.q  r8, r27
    move.q  r9, r25
    move.q  r10, r19
    move.q  r11, r20
    move.q  r12, r29
    jsr     compiler_emit_assembly
    move.q  r1, #1
    beq     r8, r1, .cta_emit_oom
    bnez    r8, .cta_unsupported
    move.q  r8, #1
    rts
.cta_unsupported:
    la      r1, AOT_LAST_LINE
    store.q r9, (r1)
    la      r1, AOT_LAST_TOKEN
    store.q r10, (r1)
    move.q  r8, r0
    move.q  r9, r0
    rts
.cta_native_init_oom:
    bra     .cta_oom
.cta_emit_oom:
.cta_oom:
    move.q  r8, #2
    move.q  r9, r0
    rts
