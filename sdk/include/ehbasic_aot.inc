; ehbasic_aot.inc - EhBASIC IE64 AOT compiler support
; ============================================================================
; Phase: compiler-time memory allocators.
;
; alloc64    - top-down retained AOT arena for compiler workspace, asm text,
;              symbols, fixups, literals, and generated AOT code.
; allocLow32 - bottom-up AOT public low32 scratch in [AOT_LOW32_BASE,
;              AOT_LOW32_END). This is deliberately small and separate from
;              public MEMALLOC.
; fileBridge - top-down private low32 staging for File I/O calls, restored by
;              callers after single-operation use.
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
    and.q   r2, r8, #0xFFF
    sub.q   r1, r8, r2             ; R1 = RAM size & ~0xFFF
    ; Keep retained allocations below BASIC stacks when the stacks are in the
    ; same high arena window. Resident high-RAM BASIC caps its stacks into the
    ; low32 window, so those low32 stack cursors must not pull the retained RUN
    ; AOT arena down from a high/sparse RAM frontier.
    add.q   r3, r16, #ST_CTRL_LOW
    load.q  r4, (r3)
    beqz    r4, .aar_have_arena_top
    bge     r4, r1, .aar_have_arena_top
    move.q  r5, #AOT_LOW32_CAP
    blt     r1, r5, .aar_use_ctrl_low
    bgt     r4, r5, .aar_use_ctrl_low
    bra     .aar_have_arena_top
.aar_use_ctrl_low:
    move.q  r1, r4                  ; retained arena shares stack window
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

aot_alloc_file_bridge:
    la      r4, AOT_FILE_BRIDGE_NEXT
    load.q  r2, (r4)
    la      r1, AOT_ARENA_HIGH_NEXT
    load.q  r3, (r1)
    blt     r2, r3, .afb_top_ok
    move.q  r2, r3
.afb_top_ok:
    la      r1, AOT_FILE_BRIDGE_LOW
    load.q  r5, (r1)
    bra     aot_alloc_topdown_finish

aot_alloc_low32:
    blt     r8, r0, aot_alloc_fail
    add.q   r1, r8, #0xFFF
    and.q   r3, r1, #0xFFF
    sub.q   r1, r1, r3             ; R1 = aligned size
    la      r4, AOT_LOW32_NEXT
    load.q  r2, (r4)               ; R2 = current bottom-up cursor
    add.q   r3, r2, r1             ; R3 = new cursor
    move.q  r5, #AOT_LOW32_END
    bgt     r3, r5, aot_alloc_fail
    blt     r3, r2, aot_alloc_fail
    store.q r3, (r4)
    move.q  r8, r2                 ; return old cursor as base
    move.q  r9, r0
    rts

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

; ============================================================================
; Private assembler - instruction encoder
; ============================================================================
; aot_emit_instr - encode one IE64 instruction word and append it to a buffer.
;
; The IE64 instruction word is a fixed 8 bytes (see encodeInstruction in
; assembler/ie64asm.go):
;   [0]      opcode
;   [1]      (rd << 3) | (size << 1) | xbit
;   [2]      rs << 3
;   [3]      rt << 3
;   [4..7]   imm32, little-endian
;
; Input:  R8  = destination pointer
;         R9  = opcode
;         R10 = rd          (0-31)
;         R11 = size        (0=B, 1=W, 2=L, 3=Q)
;         R12 = xbit        (0 or 1)
;         R13 = rs          (0-31)
;         R14 = rt          (0-31)
;         R15 = imm32
; Output: R8  = destination pointer advanced by 8
; Clobbers: R1, R2
aot_emit_instr:
    store.b r9, (r8)              ; [0] opcode
    lsl.q   r1, r10, #3           ; rd << 3
    lsl.q   r2, r11, #1           ; size << 1
    or.q    r1, r1, r2
    or.q    r1, r1, r12           ; | xbit
    store.b r1, 1(r8)             ; [1]
    lsl.q   r1, r13, #3           ; rs << 3
    store.b r1, 2(r8)             ; [2]
    lsl.q   r1, r14, #3           ; rt << 3
    store.b r1, 3(r8)             ; [3]
    store.l r15, 4(r8)            ; [4..7] imm32 (store.l writes 32-bit LE)
    add.q   r8, r8, #8
    rts

; ============================================================================
; Private assembler - operand parsers
; ============================================================================
; aot_parse_reg - parse a register operand token ("rN" 0-31, or "sp" = 31).
; Leading spaces are skipped. The token ends at the first non-digit; a register
; immediately followed by a letter (e.g. "r3x") is rejected, matching the Go
; assembler's parseRegister.
;
; Input:  R8  = pointer to the operand text
; Output: R8  = register number (0-31) when valid
;         R9  = 1 if a valid register was parsed, 0 otherwise
;         R10 = pointer just past the register token (valid only when R9 = 1)
; Clobbers: R1-R5
aot_parse_reg:
.pr_sp:
    load.b  r1, (r8)
    move.q  r2, #0x20
    bne     r1, r2, .pr_start
    add.q   r8, r8, #1
    bra     .pr_sp
.pr_start:
    or.q    r2, r1, #0x20            ; fold case
    move.q  r3, #0x73               ; 's' -> "sp"
    beq     r2, r3, .pr_sp_reg
    move.q  r3, #0x72               ; 'r'
    bne     r2, r3, .pr_invalid
    add.q   r8, r8, #1              ; past 'r'
    move.q  r4, r0                  ; value
    move.q  r5, r0                  ; digit count
.pr_digit:
    load.b  r1, (r8)
    move.q  r2, #0x30
    blt     r1, r2, .pr_end
    move.q  r2, #0x39
    bgt     r1, r2, .pr_end
    ; value = value*10 + (c - '0')
    lsl.q   r2, r4, #3
    lsl.q   r3, r4, #1
    add.q   r4, r2, r3
    sub.q   r1, r1, #0x30
    add.q   r4, r4, r1
    add.q   r8, r8, #1
    add.q   r5, r5, #1
    bra     .pr_digit
.pr_end:
    beqz    r5, .pr_invalid         ; no digits
    move.q  r2, #31
    bgt     r4, r2, .pr_invalid     ; out of range
    ; reject a letter immediately after the digits ("r3x")
    load.b  r1, (r8)
    or.q    r1, r1, #0x20
    move.q  r2, #0x61
    blt     r1, r2, .pr_ok
    move.q  r2, #0x7A
    ble     r1, r2, .pr_invalid
.pr_ok:
    move.q  r10, r8
    move.q  r8, r4
    move.q  r9, #1
    rts
.pr_sp_reg:
    add.q   r8, r8, #1
    load.b  r1, (r8)
    or.q    r1, r1, #0x20
    move.q  r2, #0x70               ; 'p'
    bne     r1, r2, .pr_invalid
    add.q   r8, r8, #1
    move.q  r10, r8
    move.q  r8, #31
    move.q  r9, #1
    rts
.pr_invalid:
    move.q  r9, r0
    rts

; aot_parse_freg - parse an FPU register operand "fN" (f0..f15).
; Input:  R8  = pointer.   Output: R8 = register number, R9 = 1 ok / 0 invalid,
; R10 = pointer past. Clobbers R1-R5.
aot_parse_freg:
.pf_sp:
    load.b  r1, (r8)
    move.q  r2, #0x20
    bne     r1, r2, .pf_f
    add.q   r8, r8, #1
    bra     .pf_sp
.pf_f:
    or.q    r2, r1, #0x20           ; fold case
    move.q  r3, #0x66               ; 'f'
    bne     r2, r3, .pf_invalid
    add.q   r8, r8, #1
    move.q  r4, r0                  ; value
    move.q  r5, r0                  ; digit count
.pf_digit:
    load.b  r1, (r8)
    move.q  r2, #0x30
    blt     r1, r2, .pf_end
    move.q  r2, #0x39
    bgt     r1, r2, .pf_end
    lsl.q   r2, r4, #3
    lsl.q   r3, r4, #1
    add.q   r4, r2, r3             ; value *= 10
    sub.q   r1, r1, #0x30
    add.q   r4, r4, r1
    add.q   r8, r8, #1
    add.q   r5, r5, #1
    bra     .pf_digit
.pf_end:
    beqz    r5, .pf_invalid         ; no digits
    move.q  r2, #15
    bgt     r4, r2, .pf_invalid     ; f0..f15 only
    load.b  r1, (r8)               ; reject a trailing letter ("f3x")
    or.q    r1, r1, #0x20
    move.q  r2, #0x61
    blt     r1, r2, .pf_ok
    move.q  r2, #0x7A
    ble     r1, r2, .pf_invalid
.pf_ok:
    move.q  r10, r8
    move.q  r8, r4
    move.q  r9, #1
    rts
.pf_invalid:
    move.q  r9, r0
    rts

; aot_parse_uint - parse an unsigned integer literal. Accepts decimal, "0x"/"0X"
; hex, and "&H"/"&h" hex (the BASIC hex syntax). Leading spaces are skipped;
; parsing stops at the first non-digit. At least one digit is required.
;
; Input:  R8  = pointer to the number text
; Output: R8  = parsed value (64-bit; callers use the low 32 for imm32)
;         R9  = 1 if a number was parsed, 0 otherwise
;         R10 = pointer just past the number (valid only when R9 = 1)
; Clobbers: R1-R6
aot_parse_uint:
.pu_sp:
    load.b  r1, (r8)
    move.q  r2, #0x20
    bne     r1, r2, .pu_start
    add.q   r8, r8, #1
    bra     .pu_sp
.pu_start:
    move.q  r2, #0x30               ; '0'
    bne     r1, r2, .pu_check_amp
    load.b  r1, 1(r8)
    or.q    r1, r1, #0x20
    move.q  r2, #0x78               ; 'x'
    bne     r1, r2, .pu_dec         ; bare '0...' -> decimal
    add.q   r8, r8, #2              ; skip "0x"
    bra     .pu_hex
.pu_check_amp:
    move.q  r2, #0x26               ; '&'
    bne     r1, r2, .pu_dec
    load.b  r1, 1(r8)
    or.q    r1, r1, #0x20
    move.q  r2, #0x68               ; 'h'
    bne     r1, r2, .pu_invalid     ; '&' not followed by 'h'
    add.q   r8, r8, #2              ; skip "&H"
    bra     .pu_hex

.pu_dec:
    move.q  r4, r0                  ; value
    move.q  r5, r0                  ; digit count
.pu_dec_loop:
    load.b  r1, (r8)
    move.q  r2, #0x30
    blt     r1, r2, .pu_dec_done
    move.q  r2, #0x39
    bgt     r1, r2, .pu_dec_done
    lsl.q   r2, r4, #3
    lsl.q   r3, r4, #1
    add.q   r4, r2, r3             ; value *= 10
    sub.q   r1, r1, #0x30
    add.q   r4, r4, r1
    add.q   r8, r8, #1
    add.q   r5, r5, #1
    bra     .pu_dec_loop
.pu_dec_done:
    beqz    r5, .pu_invalid
    bra     .pu_ok

.pu_hex:
    move.q  r4, r0
    move.q  r5, r0
.pu_hex_loop:
    load.b  r1, (r8)
    move.q  r2, #0x30
    blt     r1, r2, .pu_hex_done
    move.q  r2, #0x39
    bgt     r1, r2, .pu_hex_alpha
    sub.q   r6, r1, #0x30          ; 0-9
    bra     .pu_hex_acc
.pu_hex_alpha:
    or.q    r1, r1, #0x20
    move.q  r2, #0x61               ; 'a'
    blt     r1, r2, .pu_hex_done
    move.q  r2, #0x66               ; 'f'
    bgt     r1, r2, .pu_hex_done
    sub.q   r6, r1, #0x61
    add.q   r6, r6, #10            ; a-f -> 10-15
.pu_hex_acc:
    lsl.q   r4, r4, #4
    or.q    r4, r4, r6
    add.q   r8, r8, #1
    add.q   r5, r5, #1
    bra     .pu_hex_loop
.pu_hex_done:
    beqz    r5, .pu_invalid

.pu_ok:
    move.q  r10, r8
    move.q  r8, r4
    move.q  r9, #1
    rts
.pu_invalid:
    move.q  r9, r0
    rts

; ============================================================================
; Private assembler - mnemonic + size tokenizer
; ============================================================================
; Instruction operand classes (drive operand parsing in the line assembler).
AOT_CLS_ALU3  equ 0      ; op rd, rs, rt|#imm
AOT_CLS_ALU2  equ 1      ; op rd, rs
AOT_CLS_MOVE  equ 2      ; move rd, rs|#imm
AOT_CLS_LOAD  equ 3      ; load  rd, disp(rs) | (rs)
AOT_CLS_STORE equ 4      ; store rd, disp(rs) | (rs)
AOT_CLS_BRA   equ 5      ; bra label
AOT_CLS_BCC   equ 6      ; b<cc> rs, rt, label
AOT_CLS_JSR   equ 7      ; jsr label | (rs)
AOT_CLS_JMP   equ 8      ; jmp (rs)
AOT_CLS_NOARG equ 9      ; rts / nop / halt (no operands)
AOT_CLS_LEA   equ 10     ; lea rd, disp(rs)
AOT_CLS_LA    equ 11     ; la  rd, addr|label  (-> lea rd, addr(r0))
AOT_CLS_PUSH  equ 12     ; push rs
AOT_CLS_POP   equ 13     ; pop  rd
AOT_CLS_BZ    equ 14     ; beqz/bnez rs, label  (rt = r0)
AOT_CLS_MFCR  equ 15     ; mfcr rd, crN
AOT_CLS_FP2   equ 16     ; 2-operand FPU op (dst, src); fp-ness per mnemtab flags
AOT_CLS_FP3   equ 17     ; 3-FP-register op (fd, fs, ft)
AOT_CLS_FCMP  equ 18     ; fcmp rd, fs, ft  (rd integer, fs/ft FP)
AOT_CLS_FPMEM equ 19     ; fload/fstore/dload/dstore fN, disp(rN)

; aot_parse_mnem - read a mnemonic and optional ".b/.w/.l/.q" size suffix, then
; resolve it through the mnemonic table.
;
; Input:  R8  = pointer to the line
; Output: R8  = opcode             (valid when R12 = 1)
;         R9  = operand class       (AOT_CLS_*)
;         R10 = size                (0=B,1=W,2=L,3=Q; default Q when no suffix)
;         R11 = pointer just past the mnemonic+suffix
;         R12 = 1 if recognised, 0 otherwise
; Clobbers: R1-R6
aot_parse_mnem:
.pm_sp:
    load.b  r1, (r8)
    move.q  r2, #0x20
    bne     r1, r2, .pm_copy
    add.q   r8, r8, #1
    bra     .pm_sp
.pm_copy:
    la      r3, EHBASIC_PRIV_MNEM_BUF
    store.q r0, (r3)              ; clear the 8-byte name field (null padding)
    move.q  r4, r0               ; letter count
.pm_cl:
    load.b  r1, (r8)
    or.q    r2, r1, #0x20         ; fold case
    move.q  r5, #0x61             ; 'a'
    blt     r2, r5, .pm_cl_end
    move.q  r5, #0x7A             ; 'z'
    bgt     r2, r5, .pm_cl_end
    move.q  r5, #8                ; cap at 8 chars (the full name word, e.g. "tlbflush")
    bge     r4, r5, .pm_cl_end
    add.q   r6, r3, r4
    store.b r2, (r6)
    add.q   r8, r8, #1
    add.q   r4, r4, #1
    bra     .pm_cl
.pm_cl_end:
    beqz    r4, .pm_notfound      ; no mnemonic letters
    move.q  r10, #3              ; default size .q
    load.b  r1, (r8)
    move.q  r2, #0x2E             ; '.'
    bne     r1, r2, .pm_lookup
    add.q   r8, r8, #1           ; past '.'
    load.b  r1, (r8)
    or.q    r1, r1, #0x20
    move.q  r2, #0x62             ; 'b'
    beq     r1, r2, .pm_sz0
    move.q  r2, #0x77             ; 'w'
    beq     r1, r2, .pm_sz1
    move.q  r2, #0x6C             ; 'l'
    beq     r1, r2, .pm_sz2
    move.q  r2, #0x71             ; 'q'
    beq     r1, r2, .pm_sz3
    bra     .pm_notfound          ; bad size suffix
.pm_sz0:
    move.q  r10, #0
    bra     .pm_sz_done
.pm_sz1:
    move.q  r10, #1
    bra     .pm_sz_done
.pm_sz2:
    move.q  r10, #2
    bra     .pm_sz_done
.pm_sz3:
    move.q  r10, #3
.pm_sz_done:
    add.q   r8, r8, #1           ; past the size letter
.pm_lookup:
    move.q  r11, r8              ; past-pointer
    ; The mnemonic token must end at a separator (whitespace, end-of-line, ';'
    ; comment, or null). Otherwise an operand is glued to the mnemonic, e.g.
    ; "move.qr1" or "bra.qtarget", which ie64asm rejects as a bad mnemonic.
    load.b  r1, (r8)
    beqz    r1, .pm_sep_ok
    move.q  r2, #0x20               ; space
    beq     r1, r2, .pm_sep_ok
    move.q  r2, #0x09               ; tab
    beq     r1, r2, .pm_sep_ok
    move.q  r2, #0x0A               ; '\n'
    beq     r1, r2, .pm_sep_ok
    move.q  r2, #0x0D               ; '\r'
    beq     r1, r2, .pm_sep_ok
    move.q  r2, #0x3B               ; ';'
    beq     r1, r2, .pm_sep_ok
    bra     .pm_notfound
.pm_sep_ok:
    la      r3, EHBASIC_PRIV_MNEM_BUF
    load.q  r6, (r3)             ; parsed name as one 8-byte word
    la      r5, aot_mnemtab
.pm_scan:
    load.q  r1, (r5)             ; record name (8 bytes, null-padded)
    beqz    r1, .pm_notfound      ; terminator record
    bne     r1, r6, .pm_next
    load.b  r8, 8(r5)            ; opcode
    load.b  r9, 9(r5)            ; class
    move.q  r13, r5             ; matched record pointer (FP2 reads pad flags)
    move.q  r12, #1
    rts
.pm_next:
    add.q   r5, r5, #16          ; record stride (8 name + opcode/class + pad)
    bra     .pm_scan
.pm_notfound:
    move.q  r12, r0
    rts

; Mnemonic table: 16-byte records, q-aligned so the name load.q never faults.
; Layout: [0..7] name (lowercase, null-padded), [8] opcode, [9] class, [10..15] pad.
    align 8
aot_mnemtab:
    dc.b "move", 0,0,0,0,   0x01, AOT_CLS_MOVE,  0,0,0,0,0,0
    dc.b "load", 0,0,0,0,   0x10, AOT_CLS_LOAD,  0,0,0,0,0,0
    dc.b "store",0,0,0,     0x11, AOT_CLS_STORE, 0,0,0,0,0,0
    dc.b "lea",  0,0,0,0,0, 0x04, AOT_CLS_LEA,   0,0,0,0,0,0
    dc.b "la",   0,0,0,0,0,0, 0x04, AOT_CLS_LA,  0,0,0,0,0,0
    dc.b "push", 0,0,0,0,   0x52, AOT_CLS_PUSH,  0,0,0,0,0,0
    dc.b "pop",  0,0,0,0,0, 0x53, AOT_CLS_POP,   0,0,0,0,0,0
    dc.b "beqz", 0,0,0,0,   0x41, AOT_CLS_BZ,    0,0,0,0,0,0
    dc.b "bnez", 0,0,0,0,   0x42, AOT_CLS_BZ,    0,0,0,0,0,0
    dc.b "mfcr", 0,0,0,0,   0xE7, AOT_CLS_MFCR,  0,0,0,0,0,0
    dc.b "tlbflush",        0xE9, AOT_CLS_NOARG, 0,0,0,0,0,0
    ; FPU (size .L). FP2 pad bytes: [10] = dst is FP, [11] = src is FP.
    dc.b "fmovi", 0,0,0,    0x6F, AOT_CLS_FP2,   1,0,0,0,0,0
    dc.b "fmovo", 0,0,0,    0x70, AOT_CLS_FP2,   0,1,0,0,0,0
    dc.b "fcvtif",0,0,      0x6D, AOT_CLS_FP2,   1,0,0,0,0,0
    dc.b "fcvtfi",0,0,      0x6E, AOT_CLS_FP2,   0,1,0,0,0,0
    dc.b "fabs", 0,0,0,0,   0x68, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "fneg", 0,0,0,0,   0x69, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "fsqrt",0,0,0,     0x6A, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "fint", 0,0,0,0,   0x6B, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "fsin", 0,0,0,0,   0x71, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "fcos", 0,0,0,0,   0x72, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "ftan", 0,0,0,0,   0x73, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "fatan",0,0,0,     0x74, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "flog", 0,0,0,0,   0x75, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "fexp", 0,0,0,0,   0x76, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "fadd", 0,0,0,0,   0x63, AOT_CLS_FP3,   0,0,0,0,0,0
    dc.b "fsub", 0,0,0,0,   0x64, AOT_CLS_FP3,   0,0,0,0,0,0
    dc.b "fmul", 0,0,0,0,   0x65, AOT_CLS_FP3,   0,0,0,0,0,0
    dc.b "fdiv", 0,0,0,0,   0x66, AOT_CLS_FP3,   0,0,0,0,0,0
    dc.b "fpow", 0,0,0,0,   0x77, AOT_CLS_FP3,   0,0,0,0,0,0
    dc.b "fcmp", 0,0,0,0,   0x6C, AOT_CLS_FCMP,  0,0,0,0,0,0
    dc.b "fload",0,0,0,     0x61, AOT_CLS_FPMEM, 0,0,0,0,0,0
    dc.b "fstore",0,0,      0x62, AOT_CLS_FPMEM, 0,0,0,0,0,0
    dc.b "dmov", 0,0,0,0,   0x80, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "dload",0,0,0,     0x81, AOT_CLS_FPMEM, 0,0,0,0,0,0
    dc.b "dstore",0,0,      0x82, AOT_CLS_FPMEM, 0,0,0,0,0,0
    dc.b "dadd", 0,0,0,0,   0x83, AOT_CLS_FP3,   0,0,0,0,0,0
    dc.b "dsub", 0,0,0,0,   0x84, AOT_CLS_FP3,   0,0,0,0,0,0
    dc.b "dmul", 0,0,0,0,   0x85, AOT_CLS_FP3,   0,0,0,0,0,0
    dc.b "ddiv", 0,0,0,0,   0x86, AOT_CLS_FP3,   0,0,0,0,0,0
    dc.b "dmod", 0,0,0,0,   0x87, AOT_CLS_FP3,   0,0,0,0,0,0
    dc.b "dabs", 0,0,0,0,   0x88, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "dneg", 0,0,0,0,   0x89, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "dsqrt",0,0,0,     0x8A, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "dint", 0,0,0,0,   0x8B, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "dcmp", 0,0,0,0,   0x8C, AOT_CLS_FCMP,  0,0,0,0,0,0
    dc.b "dcvtif",0,0,      0x8D, AOT_CLS_FP2,   1,0,0,0,0,0
    dc.b "dcvtfi",0,0,      0x8E, AOT_CLS_FP2,   0,1,0,0,0,0
    dc.b "fcvtsd",0,0,      0x8F, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "fcvtds",0,0,      0x90, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "dsin", 0,0,0,0,   0x91, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "dcos", 0,0,0,0,   0x92, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "dtan", 0,0,0,0,   0x93, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "datan",0,0,0,     0x94, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "dlog", 0,0,0,0,   0x95, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "dexp", 0,0,0,0,   0x96, AOT_CLS_FP2,   1,1,0,0,0,0
    dc.b "dpow", 0,0,0,0,   0x97, AOT_CLS_FP3,   0,0,0,0,0,0
    dc.b "add",  0,0,0,0,0, 0x20, AOT_CLS_ALU3,  0,0,0,0,0,0
    dc.b "sub",  0,0,0,0,0, 0x21, AOT_CLS_ALU3,  0,0,0,0,0,0
    dc.b "mulu", 0,0,0,0,   0x22, AOT_CLS_ALU3,  0,0,0,0,0,0
    dc.b "muls", 0,0,0,0,   0x23, AOT_CLS_ALU3,  0,0,0,0,0,0
    dc.b "divu", 0,0,0,0,   0x24, AOT_CLS_ALU3,  0,0,0,0,0,0
    dc.b "divs", 0,0,0,0,   0x25, AOT_CLS_ALU3,  0,0,0,0,0,0
    dc.b "mod",  0,0,0,0,0, 0x26, AOT_CLS_ALU3,  0,0,0,0,0,0
    dc.b "mods", 0,0,0,0,   0x28, AOT_CLS_ALU3,  0,0,0,0,0,0
    dc.b "neg",  0,0,0,0,0, 0x27, AOT_CLS_ALU2,  0,0,0,0,0,0
    dc.b "and",  0,0,0,0,0, 0x30, AOT_CLS_ALU3,  0,0,0,0,0,0
    dc.b "or",   0,0,0,0,0,0, 0x31, AOT_CLS_ALU3, 0,0,0,0,0,0
    dc.b "eor",  0,0,0,0,0, 0x32, AOT_CLS_ALU3,  0,0,0,0,0,0
    dc.b "not",  0,0,0,0,0, 0x33, AOT_CLS_ALU2,  0,0,0,0,0,0
    dc.b "lsl",  0,0,0,0,0, 0x34, AOT_CLS_ALU3,  0,0,0,0,0,0
    dc.b "lsr",  0,0,0,0,0, 0x35, AOT_CLS_ALU3,  0,0,0,0,0,0
    dc.b "asr",  0,0,0,0,0, 0x36, AOT_CLS_ALU3,  0,0,0,0,0,0
    dc.b "sext", 0,0,0,0,   0x38, AOT_CLS_ALU2,  0,0,0,0,0,0
    dc.b "rol",  0,0,0,0,0, 0x39, AOT_CLS_ALU3,  0,0,0,0,0,0
    dc.b "ror",  0,0,0,0,0, 0x3A, AOT_CLS_ALU3,  0,0,0,0,0,0
    dc.b "bra",  0,0,0,0,0, 0x40, AOT_CLS_BRA,   0,0,0,0,0,0
    dc.b "beq",  0,0,0,0,0, 0x41, AOT_CLS_BCC,   0,0,0,0,0,0
    dc.b "bne",  0,0,0,0,0, 0x42, AOT_CLS_BCC,   0,0,0,0,0,0
    dc.b "blt",  0,0,0,0,0, 0x43, AOT_CLS_BCC,   0,0,0,0,0,0
    dc.b "bge",  0,0,0,0,0, 0x44, AOT_CLS_BCC,   0,0,0,0,0,0
    dc.b "bgt",  0,0,0,0,0, 0x45, AOT_CLS_BCC,   0,0,0,0,0,0
    dc.b "ble",  0,0,0,0,0, 0x46, AOT_CLS_BCC,   0,0,0,0,0,0
    dc.b "jmp",  0,0,0,0,0, 0x49, AOT_CLS_JMP,   0,0,0,0,0,0
    dc.b "jsr",  0,0,0,0,0, 0x50, AOT_CLS_JSR,   0,0,0,0,0,0
    dc.b "rts",  0,0,0,0,0, 0x51, AOT_CLS_NOARG, 0,0,0,0,0,0
    dc.b "nop",  0,0,0,0,0, 0xE0, AOT_CLS_NOARG, 0,0,0,0,0,0
    dc.b "halt", 0,0,0,0,   0xE1, AOT_CLS_NOARG, 0,0,0,0,0,0
    dc.b 0,0,0,0,0,0,0,0,   0,0,  0,0,0,0,0,0      ; terminator

; ============================================================================
; Private assembler - symbol table + label parser
; ============================================================================
; Labels are referenced by (source pointer, length): the source text is stable
; for the whole assembly, so names are compared in place with no copy or length
; limit. Each 16-byte record packs the name pointer in the low 56 bits and the
; length in the top 8 bits of word 0, with the address in word 1. Names are
; case-sensitive (matching ie64asm).

; aot_sym_reset - clear the symbol table. Clobbers R1.
aot_sym_reset:
    la      r1, AOT_SYMCNT
    store.q r0, (r1)
    rts

; aot_scope_reset - reset local-label scope tracking. Called at the start of each
; pass so both passes assign the same scope id to each global label. Clobbers R1.
aot_scope_reset:
    la      r1, AOT_CUR_SCOPE
    store.q r0, (r1)
    la      r1, AOT_SCOPE_NEXT
    store.q r0, (r1)
    rts

; aot_scope_enter - if the label at R8 is global (not '.'-prefixed) allocate a new
; scope id and make it current; a local label leaves the scope unchanged. Both
; passes call this on every label so local-label references resolve within the
; enclosing global label. Clobbers R1-R3.
aot_scope_enter:
    load.b  r1, (r8)
    move.q  r2, #0x2E               ; '.'
    beq     r1, r2, .se_done        ; local label: stay in the current scope
    la      r1, AOT_SCOPE_NEXT
    load.q  r2, (r1)
    add.q   r2, r2, #1
    store.q r2, (r1)
    la      r1, AOT_CUR_SCOPE
    store.q r2, (r1)
.se_done:
    rts

; aot_sym_add - define a label. Input R8 = name pointer, R9 = length,
; R10 = address. Output R11 = 1 on success, 0 if the table is full. The record
; packs name pointer+length in word 0 and address+scope (scope in the top 16
; bits) in word 1. A '.'-prefixed (local) name takes the current scope; a global
; name takes scope 0. Clobbers R1-R5.
aot_sym_add:
    la      r1, AOT_SYMCNT
    load.q  r2, (r1)
    move.q  r3, #AOT_SYM_MAX
    bge     r2, r3, .sa_full
    lsl.q   r4, r2, #4               ; count * 16
    la      r3, AOT_SYMTAB_PTR
    load.q  r3, (r3)               ; active symbol-table base (caller-provided)
    add.q   r3, r3, r4
    lsl.q   r5, r9, #56              ; length in the top byte
    or.q    r5, r5, r8              ; name pointer in the low 56 bits
    store.q r5, (r3)
    load.b  r1, (r8)               ; scope: local ('.') -> current, else 0
    move.q  r5, #0x2E
    bne     r1, r5, .sa_global
    la      r1, AOT_CUR_SCOPE
    load.q  r5, (r1)
    bra     .sa_packaddr
.sa_global:
    move.q  r5, r0
.sa_packaddr:
    lsl.q   r5, r5, #48            ; scope in the top 16 bits
    or.q    r5, r5, r10           ; address in the low 48 bits
    store.q r5, 8(r3)
    la      r1, AOT_SYMCNT
    add.q   r2, r2, #1
    store.q r2, (r1)
    move.q  r11, #1
    rts
.sa_full:
    move.q  r11, r0
    rts

; aot_sym_lookup - resolve a label. Input R8 = name pointer, R9 = length.
; Output R8 = address, R9 = 1 if found (0 otherwise). A '.'-prefixed name is
; resolved within the current scope (AOT_CUR_SCOPE); a global name uses scope 0.
; Preserves R10, R11 (callers rely on the past pointer / length). Clobbers R1-R8.
aot_sym_lookup:
    load.b  r1, (r8)               ; query scope from the name's first char
    move.q  r2, #0x2E
    bne     r1, r2, .sl_qglobal
    la      r1, AOT_CUR_SCOPE
    load.q  r2, (r1)
    bra     .sl_qstore
.sl_qglobal:
    move.q  r2, r0
.sl_qstore:
    la      r1, AOT_QSCOPE
    store.q r2, (r1)
    la      r1, AOT_SYMCNT
    load.q  r2, (r1)               ; remaining records
    la      r3, AOT_SYMTAB_PTR
    load.q  r3, (r3)               ; active symbol-table base
.sl_loop:
    beqz    r2, .sl_none
    load.q  r5, (r3)               ; packed nameref
    move.q  r6, r5
    lsr.q   r6, r6, #56            ; stored length
    bne     r6, r9, .sl_next
    lsl.q   r6, r5, #8
    lsr.q   r6, r6, #8             ; stored name pointer (top byte cleared)
    move.q  r4, r9                 ; bytes to compare
    move.q  r7, r8                 ; query cursor
.sl_cmp:
    beqz    r4, .sl_scope
    load.b  r1, (r7)
    load.b  r5, (r6)
    bne     r1, r5, .sl_next
    add.q   r7, r7, #1
    add.q   r6, r6, #1
    sub.q   r4, r4, #1
    bra     .sl_cmp
.sl_scope:
    load.q  r5, 8(r3)             ; word 1 = address | scope<<48
    move.q  r6, r5
    lsr.q   r6, r6, #48            ; stored scope
    la      r1, AOT_QSCOPE
    load.q  r1, (r1)
    bne     r6, r1, .sl_next        ; scope mismatch
    lsl.q   r8, r5, #16
    lsr.q   r8, r8, #16            ; address (top 16 scope bits cleared)
    move.q  r9, #1
    rts
.sl_next:
    add.q   r3, r3, #16
    sub.q   r2, r2, #1
    bra     .sl_loop
.sl_none:
    move.q  r9, r0
    rts

; aot_const_lookup - resolve a named assembler constant (an "equ" symbol) used as
; an operand value, e.g. "#ST_ERROR_FLAG" or "la r1, BASIC_STATE". The values are
; resolved by ie64asm when this image is built, so the in-guest assembler never
; needs include/equ support. Input R8 = name pointer, R9 = length. Output R8 =
; value, R9 = 1 if found (0 otherwise). Clobbers R1-R7.
;
; The table (aot_consttab) is build-time generated from ie64.inc by
; tools/gen_aot_consttab and pulled in via the include below.
; Records: [name length : 1][name bytes][value : 4 LE], 0-length terminated.
aot_const_lookup:
    la      r3, aot_consttab
.kl_loop:
    load.b  r4, (r3)               ; entry name length (0 = end of table)
    beqz    r4, .kl_none
    bne     r4, r9, .kl_skip       ; length mismatch
    add.q   r6, r3, #1            ; entry name pointer
    move.q  r5, r8                ; query cursor
    move.q  r7, r9                ; remaining bytes
.kl_cmp:
    beqz    r7, .kl_match
    load.b  r1, (r5)
    load.b  r2, (r6)
    bne     r1, r2, .kl_skip
    add.q   r5, r5, #1
    add.q   r6, r6, #1
    sub.q   r7, r7, #1
    bra     .kl_cmp
.kl_match:
    add.q   r6, r3, #1            ; value pointer = record + 1 + namelen
    add.q   r6, r6, r4
    load.b  r8, (r6)              ; 4-byte little-endian value (unaligned)
    load.b  r1, 1(r6)
    lsl.q   r1, r1, #8
    or.q    r8, r8, r1
    load.b  r1, 2(r6)
    lsl.q   r1, r1, #16
    or.q    r8, r8, r1
    load.b  r1, 3(r6)
    lsl.q   r1, r1, #24
    or.q    r8, r8, r1
    move.q  r9, #1
    rts
.kl_skip:
    add.q   r3, r3, r4            ; next record = record + 1 + namelen + 4
    add.q   r3, r3, #5
    bra     .kl_loop
.kl_none:
    move.q  r9, r0
    rts

; Build-time generated [name pointer, value] table (tools/gen_aot_consttab).
    include "aot_consttab.inc"

; aot_parse_label - read an identifier ([A-Za-z_][A-Za-z0-9_]*, any length).
; Names are referenced in place (pointer + length) rather than copied, so labels
; are not length-limited. Case-sensitive (matching ie64asm).
; Input  R8  = pointer (leading spaces skipped)
; Output R8  = pointer to the first character of the name
;        R9  = 1 if a valid identifier, 0 otherwise
;        R10 = pointer just past the name
;        R11 = name length in bytes
; Clobbers R1-R4
aot_parse_label:
.pl_sp:
    load.b  r1, (r8)
    move.q  r2, #0x20
    bne     r1, r2, .pl_first
    add.q   r8, r8, #1
    bra     .pl_sp
.pl_first:
    move.q  r2, #0x5F                ; '_'
    beq     r1, r2, .pl_begin
    move.q  r2, #0x2E                ; '.' (local label, e.g. ".loop")
    beq     r1, r2, .pl_begin
    or.q    r3, r1, #0x20            ; fold only for the letter test
    move.q  r2, #0x61
    blt     r3, r2, .pl_invalid
    move.q  r2, #0x7A
    bgt     r3, r2, .pl_invalid
.pl_begin:
    move.q  r4, r8                  ; name start
    add.q   r8, r8, #1             ; consume the already-validated first char
                                    ; (letter, '_' or a leading '.' for locals)
.pl_ch:
    load.b  r1, (r8)
    move.q  r2, #0x5F                ; '_'
    beq     r1, r2, .pl_adv
    move.q  r2, #0x30                ; '0'
    blt     r1, r2, .pl_done
    move.q  r2, #0x39                ; '9'
    ble     r1, r2, .pl_adv          ; digit
    or.q    r3, r1, #0x20
    move.q  r2, #0x61
    blt     r3, r2, .pl_done
    move.q  r2, #0x7A
    bgt     r3, r2, .pl_done         ; not a letter
.pl_adv:
    add.q   r8, r8, #1
    bra     .pl_ch
.pl_done:
    move.q  r10, r8                 ; past
    sub.q   r11, r8, r4            ; length
    move.q  r8, r4                  ; name start
    move.q  r9, #1
    rts
.pl_invalid:
    move.q  r9, r0
    rts

; ============================================================================
; Private assembler - single-line assembler
; ============================================================================
; aot_skip_comma - advance the R20 cursor past spaces, a required comma, and
; following spaces. Operands are comma-separated (matching ie64asm), so a
; missing comma is a malformed line.
; Output: R9 = 1 if a comma was consumed, 0 if none was present.
; Clobbers R1, R2.
aot_skip_comma:
.sc_pre:
    load.b  r1, (r20)
    move.q  r2, #0x20
    bne     r1, r2, .sc_comma
    add.q   r20, r20, #1
    bra     .sc_pre
.sc_comma:
    move.q  r2, #0x2C               ; ','
    bne     r1, r2, .sc_none
    add.q   r20, r20, #1
.sc_post:
    load.b  r1, (r20)
    move.q  r2, #0x20
    bne     r1, r2, .sc_yes
    add.q   r20, r20, #1
    bra     .sc_post
.sc_yes:
    move.q  r9, #1
    rts
.sc_none:
    move.q  r9, r0
    rts

; aot_parse_memref - parse "disp(rs)" or "(rs)" (load/store address operand).
; Input:  R8  = pointer
; Output: R8  = displacement, R9 = rs, R10 = 1 if valid (0 otherwise),
;         R11 = pointer past ')'
; Clobbers: R1-R7
aot_parse_memref:
.mr_sp:
    load.b  r1, (r8)
    move.q  r2, #0x20
    bne     r1, r2, .mr_chk
    add.q   r8, r8, #1
    bra     .mr_sp
.mr_chk:
    move.q  r2, #0x28               ; '('
    beq     r1, r2, .mr_nodisp
    jsr     aot_parse_uint          ; R8=disp, R9=valid, R10=past
    beqz    r9, .mr_bad
    move.q  r7, r8                  ; save disp (R7 survives parse_reg)
    move.q  r8, r10                 ; cursor past the displacement
    bra     .mr_open
.mr_nodisp:
    move.q  r7, r0                  ; disp = 0
.mr_open:
    load.b  r1, (r8)
    move.q  r2, #0x28               ; '('
    bne     r1, r2, .mr_bad
    add.q   r8, r8, #1
    jsr     aot_parse_reg           ; R8=rs, R9=valid, R10=past
    beqz    r9, .mr_bad
    move.q  r6, r8                  ; save rs
    move.q  r8, r10
    load.b  r1, (r8)
    move.q  r2, #0x29               ; ')'
    bne     r1, r2, .mr_bad
    add.q   r8, r8, #1
    move.q  r11, r8
    move.q  r8, r7                  ; disp
    move.q  r9, r6                  ; rs
    move.q  r10, #1
    rts
.mr_bad:
    move.q  r10, r0
    rts

; aot_asm_line - assemble one instruction line into the output buffer.
; Supports ALU3, ALU2, MOVE, LOAD, STORE, and the branch classes BRA, BCC,
; JSR, JMP. Branch/jsr-label operands are resolved through the symbol table;
; PC-relative offsets are computed against R10.
; Input:  R8  = source line pointer (one instruction)
;         R9  = destination buffer pointer
;         R10 = address (PC) this instruction will live at (for branch offsets)
; Output: R8  = 1 on success, 0 on parse error
;         R9  = destination advanced by 8 (only on success)
; Clobbers: R1-R15, R19-R29
aot_asm_line:
    move.q  r21, r9                 ; destination
    move.q  r19, r10                ; this instruction's PC
    jsr     aot_parse_mnem          ; R8=op R9=class R10=size R11=past R12=found
    beqz    r12, .al_err
    move.q  r22, r8                 ; opcode
    move.q  r23, r9                 ; class
    move.q  r24, r10                ; size
    move.q  r20, r11                ; cursor

    ; Default all instruction fields (branches leave rd = 0).
    move.q  r25, r0                 ; rd
    move.q  r26, r0                 ; xbit
    move.q  r27, r0                 ; rs
    move.q  r28, r0                 ; rt
    move.q  r29, r0                 ; imm

    ; Branch classes have no leading rd register; dispatch before parsing rd.
    move.q  r1, #AOT_CLS_BRA
    beq     r23, r1, .al_bra
    move.q  r1, #AOT_CLS_BCC
    beq     r23, r1, .al_bcc
    move.q  r1, #AOT_CLS_JSR
    beq     r23, r1, .al_jsr
    move.q  r1, #AOT_CLS_JMP
    beq     r23, r1, .al_jmp
    move.q  r1, #AOT_CLS_NOARG
    beq     r23, r1, .al_noarg
    move.q  r1, #AOT_CLS_PUSH
    beq     r23, r1, .al_push
    move.q  r1, #AOT_CLS_POP
    beq     r23, r1, .al_pop
    move.q  r1, #AOT_CLS_BZ
    beq     r23, r1, .al_bz
    move.q  r1, #AOT_CLS_MFCR
    beq     r23, r1, .al_mfcr
    move.q  r1, #AOT_CLS_FP2
    beq     r23, r1, .al_fp2
    move.q  r1, #AOT_CLS_FP3
    beq     r23, r1, .al_fp3
    move.q  r1, #AOT_CLS_FCMP
    beq     r23, r1, .al_fcmp
    move.q  r1, #AOT_CLS_FPMEM
    beq     r23, r1, .al_fpmem

    ; Non-branch: operand 1 = rd.
    move.q  r8, r20
    jsr     aot_parse_reg
    beqz    r9, .al_err
    move.q  r25, r8                 ; rd
    move.q  r20, r10

    move.q  r1, #AOT_CLS_ALU2
    beq     r23, r1, .al_alu2
    move.q  r1, #AOT_CLS_MOVE
    beq     r23, r1, .al_move
    move.q  r1, #AOT_CLS_LOAD
    beq     r23, r1, .al_mem
    move.q  r1, #AOT_CLS_STORE
    beq     r23, r1, .al_mem
    move.q  r1, #AOT_CLS_LEA
    beq     r23, r1, .al_lea
    move.q  r1, #AOT_CLS_LA
    beq     r23, r1, .al_la
    ; fall through: ALU3

.al_alu3:
    jsr     aot_skip_comma
    beqz    r9, .al_err             ; comma required between rd and rs
    move.q  r8, r20
    jsr     aot_parse_reg           ; rs
    beqz    r9, .al_err
    move.q  r27, r8
    move.q  r20, r10
    jsr     aot_skip_comma
    beqz    r9, .al_err             ; comma required before the third operand
    ; third operand: #imm or rt
    move.q  r8, r20
.al3_sp:
    load.b  r1, (r8)
    move.q  r2, #0x20
    bne     r1, r2, .al3_chk
    add.q   r8, r8, #1
    bra     .al3_sp
.al3_chk:
    move.q  r2, #0x23               ; '#'
    bne     r1, r2, .al3_reg
    add.q   r8, r8, #1
    jsr     aot_parse_value         ; number or named constant
    beqz    r9, .al_err
    move.q  r29, r8                 ; imm
    move.q  r26, #1                 ; xbit
    move.q  r20, r10
    bra     .al_endchk
.al3_reg:
    move.q  r8, r20
    jsr     aot_parse_reg
    beqz    r9, .al_err
    move.q  r28, r8                 ; rt
    move.q  r20, r10
    bra     .al_endchk

.al_alu2:
    jsr     aot_skip_comma
    beqz    r9, .al_err             ; comma required between rd and rs
    move.q  r8, r20
    jsr     aot_parse_reg
    beqz    r9, .al_err
    move.q  r27, r8                 ; rs
    move.q  r20, r10
    bra     .al_endchk

.al_move:
    jsr     aot_skip_comma
    beqz    r9, .al_err             ; comma required between rd and source
    move.q  r8, r20
.mv_sp:
    load.b  r1, (r8)
    move.q  r2, #0x20
    bne     r1, r2, .mv_chk
    add.q   r8, r8, #1
    bra     .mv_sp
.mv_chk:
    move.q  r2, #0x23               ; '#'
    bne     r1, r2, .mv_reg
    add.q   r8, r8, #1
    jsr     aot_parse_value         ; number or named constant
    beqz    r9, .al_err
    move.q  r29, r8                 ; imm
    move.q  r26, #1                 ; xbit
    move.q  r20, r10
    bra     .al_endchk
.mv_reg:
    move.q  r8, r20
    jsr     aot_parse_reg
    beqz    r9, .al_err
    move.q  r27, r8                 ; rs (source register)
    move.q  r20, r10
    bra     .al_endchk

.al_mem:
    jsr     aot_skip_comma
    beqz    r9, .al_err             ; comma required between rd and address
    move.q  r8, r20
    jsr     aot_parse_memref        ; R8=disp R9=rs R10=valid R11=past
    beqz    r10, .al_err
    move.q  r29, r8                 ; imm = displacement
    move.q  r27, r9                 ; rs
    move.q  r20, r11
    beqz    r29, .al_endchk
    move.q  r26, #1                 ; xbit when displacement != 0
    bra     .al_endchk

.al_bra:
    move.q  r24, #3                 ; branches are size .q
    move.q  r8, r20
    jsr     aot_parse_label
    beqz    r9, .al_err
    move.q  r20, r10
    move.q  r9, r11                ; R9 = name length for the lookup
    jsr     aot_sym_lookup          ; R8=nameptr,R9=len -> R8=addr, R9=found
    beqz    r9, .al_err
    sub.q   r29, r8, r19            ; imm = target - PC
    bra     .al_endchk

.al_bcc:
    move.q  r24, #3
    move.q  r8, r20
    jsr     aot_parse_reg           ; rs
    beqz    r9, .al_err
    move.q  r27, r8
    move.q  r20, r10
    jsr     aot_skip_comma
    beqz    r9, .al_err
    move.q  r8, r20
    jsr     aot_parse_reg           ; rt
    beqz    r9, .al_err
    move.q  r28, r8
    move.q  r20, r10
    jsr     aot_skip_comma
    beqz    r9, .al_err
    move.q  r8, r20
    jsr     aot_parse_label
    beqz    r9, .al_err
    move.q  r20, r10
    move.q  r9, r11
    jsr     aot_sym_lookup
    beqz    r9, .al_err
    sub.q   r29, r8, r19            ; imm = target - PC
    bra     .al_endchk

.al_jsr:
    ; "(rN)" / "disp(rN)" -> JSR_IND; otherwise a PC-relative label.
    move.q  r8, r20
.jsr_sp:
    load.b  r1, (r8)
    move.q  r2, #0x20
    bne     r1, r2, .jsr_chk
    add.q   r8, r8, #1
    bra     .jsr_sp
.jsr_chk:
    move.q  r2, #0x28               ; '('
    beq     r1, r2, .al_jmp_ind     ; share the indirect path (opcode set below)
    move.q  r2, #0x30
    blt     r1, r2, .jsr_label
    move.q  r2, #0x39
    ble     r1, r2, .al_jmp_ind     ; leading digit -> disp(rN)
.jsr_label:
    move.q  r24, #3                 ; jsr label is size .q
    move.q  r8, r20
    jsr     aot_parse_label
    beqz    r9, .al_err
    move.q  r20, r10
    move.q  r9, r11
    jsr     aot_sym_lookup
    beqz    r9, .al_err
    sub.q   r29, r8, r19            ; imm = target - PC (opcode 0x50 from table)
    bra     .al_endchk

.al_noarg:
    move.q  r24, r0                 ; rts/nop/halt/tlbflush: size 0, no operands
    bra     .al_endchk

; push rs -> reg in the rs field (rd = 0), matching ie64asm asmPushPop.
.al_push:
    move.q  r24, #3
    move.q  r8, r20
    jsr     aot_parse_reg
    beqz    r9, .al_err
    move.q  r27, r8                 ; rs = reg
    move.q  r20, r10
    bra     .al_endchk
; pop rd -> reg in the rd field.
.al_pop:
    move.q  r24, #3
    move.q  r8, r20
    jsr     aot_parse_reg
    beqz    r9, .al_err
    move.q  r25, r8                 ; rd = reg
    move.q  r20, r10
    bra     .al_endchk
; beqz/bnez rs, label -> beq/bne rs, r0, label (rt stays 0).
.al_bz:
    move.q  r24, #3
    move.q  r8, r20
    jsr     aot_parse_reg
    beqz    r9, .al_err
    move.q  r27, r8                 ; rs
    move.q  r20, r10
    jsr     aot_skip_comma
    beqz    r9, .al_err
    move.q  r8, r20
    jsr     aot_parse_label
    beqz    r9, .al_err
    move.q  r20, r10
    move.q  r9, r11
    jsr     aot_sym_lookup
    beqz    r9, .al_err
    sub.q   r29, r8, r19           ; imm = target - PC
    bra     .al_endchk
; mfcr rd, crN -> rd field, CR# in the rs field, size 0.
.al_mfcr:
    move.q  r24, r0
    move.q  r8, r20
    jsr     aot_parse_reg
    beqz    r9, .al_err
    move.q  r25, r8                 ; rd
    move.q  r20, r10
    jsr     aot_skip_comma
    beqz    r9, .al_err
    load.b  r1, (r20)
    or.q    r1, r1, #0x20
    move.q  r2, #0x63              ; 'c'
    bne     r1, r2, .al_err
    add.q   r20, r20, #1
    load.b  r1, (r20)
    or.q    r1, r1, #0x20
    move.q  r2, #0x72              ; 'r'
    bne     r1, r2, .al_err
    add.q   r20, r20, #1
    move.q  r8, r20
    jsr     aot_parse_uint         ; CR number
    beqz    r9, .al_err
    move.q  r27, r8                ; rs = CR#
    move.q  r20, r10
    bra     .al_endchk
; FPU 2-operand: dst then src; each FP (fN) or integer per the mnemtab flags at
; R13+10 (dst is FP) and R13+11 (src is FP). Size .L.
.al_fp2:
    move.q  r24, #2
    load.b  r1, 10(r13)            ; dst FP?
    move.q  r8, r20
    bnez    r1, .fp2_dfp
    jsr     aot_parse_reg
    bra     .fp2_dchk
.fp2_dfp:
    jsr     aot_parse_freg
.fp2_dchk:
    beqz    r9, .al_err
    move.q  r25, r8                ; rd / fd
    move.q  r20, r10
    jsr     aot_skip_comma
    beqz    r9, .al_err
    load.b  r1, 11(r13)            ; src FP?
    move.q  r8, r20
    bnez    r1, .fp2_sfp
    jsr     aot_parse_reg
    bra     .fp2_schk
.fp2_sfp:
    jsr     aot_parse_freg
.fp2_schk:
    beqz    r9, .al_err
    move.q  r27, r8                ; rs / fs
    move.q  r20, r10
    bra     .al_endchk
; FPU 3-operand: fd, fs, ft (all FP). Size .L.
.al_fp3:
    move.q  r24, #2
    move.q  r8, r20
    jsr     aot_parse_freg
    beqz    r9, .al_err
    move.q  r25, r8                ; fd
    move.q  r20, r10
    jsr     aot_skip_comma
    beqz    r9, .al_err
    move.q  r8, r20
    jsr     aot_parse_freg
    beqz    r9, .al_err
    move.q  r27, r8                ; fs
    move.q  r20, r10
    jsr     aot_skip_comma
    beqz    r9, .al_err
    move.q  r8, r20
    jsr     aot_parse_freg
    beqz    r9, .al_err
    move.q  r28, r8                ; ft
    move.q  r20, r10
    bra     .al_endchk
; fcmp rd, fs, ft -> rd integer, fs/ft FP. Size .L.
.al_fcmp:
    move.q  r24, #2
    move.q  r8, r20
    jsr     aot_parse_reg
    beqz    r9, .al_err
    move.q  r25, r8                ; rd (int)
    move.q  r20, r10
    jsr     aot_skip_comma
    beqz    r9, .al_err
    move.q  r8, r20
    jsr     aot_parse_freg
    beqz    r9, .al_err
    move.q  r27, r8                ; fs
    move.q  r20, r10
    jsr     aot_skip_comma
    beqz    r9, .al_err
    move.q  r8, r20
    jsr     aot_parse_freg
    beqz    r9, .al_err
    move.q  r28, r8                ; ft
    move.q  r20, r10
    bra     .al_endchk

; FPU memory: fload/fstore/dload/dstore fN, disp(rN). Size .L, FP operand in rd,
; memory base in rs, displacement in imm.
.al_fpmem:
    move.q  r24, #2
    move.q  r8, r20
    jsr     aot_parse_freg
    beqz    r9, .al_err
    move.q  r25, r8                ; FP register in rd
    move.q  r20, r10
    jsr     aot_skip_comma
    beqz    r9, .al_err
    move.q  r8, r20
    jsr     aot_parse_memref
    beqz    r10, .al_err
    move.q  r29, r8                ; displacement
    move.q  r27, r9                ; memory base
    move.q  r20, r11
    beqz    r29, .al_endchk
    move.q  r26, #1
    bra     .al_endchk

; lea rd, disp(rs) -> OP 0x04, size .q, xbit 1 (matches ie64asm asmLea).
.al_lea:
    move.q  r24, #3                 ; lea is always size .q
    jsr     aot_skip_comma
    beqz    r9, .al_err             ; comma required between rd and address
    move.q  r8, r20
    jsr     aot_parse_memref        ; R8=disp R9=rs R10=valid R11=past
    beqz    r10, .al_err
    move.q  r29, r8                 ; imm = displacement
    move.q  r27, r9                 ; rs
    move.q  r20, r11
    move.q  r26, #1                 ; xbit always 1 for lea
    bra     .al_endchk

; la rd, addr|label -> lea rd, addr(r0). The operand is a label (resolved at the
; link base through the symbol table) or a numeric address.
.al_la:
    move.q  r24, #3                 ; size .q
    move.q  r27, r0                 ; rs = r0
    move.q  r26, #1                 ; xbit always 1
    jsr     aot_skip_comma
    beqz    r9, .al_err             ; comma required between rd and address
    move.q  r8, r20
    jsr     aot_parse_value         ; label / named constant / numeric address
    beqz    r9, .al_err
    move.q  r29, r8                 ; imm = resolved address
    move.q  r20, r10
    bra     .al_endchk

.al_jmp:
    move.q  r22, #0x49              ; JMP (table already has 0x49, kept explicit)
.al_jmp_ind:
    ; Register-indirect jmp/jsr: opcode already set (0x49 jmp, or 0x50->0x54
    ; jsr_ind below), size 0, xbit 0 (matching ie64asm).
    move.q  r1, #0x50               ; if we arrived from .al_jsr the opcode is JSR
    bne     r22, r1, .ji_go
    move.q  r22, #0x54              ; JSR_IND
.ji_go:
    move.q  r24, r0                 ; size 0
    move.q  r8, r20
    jsr     aot_parse_memref        ; R8=disp R9=rs R10=valid R11=past
    beqz    r10, .al_err
    move.q  r29, r8                 ; displacement
    move.q  r27, r9                 ; rs
    move.q  r20, r11
    bra     .al_endchk

.al_endchk:
    ; The cursor (R20) must be at end-of-line after optional trailing spaces;
    ; any leftover token means the line is malformed (reject, do not emit).
    ; End-of-line is null, newline (\n / \r), or a ';' comment.
    load.b  r1, (r20)
    move.q  r2, #0x20
    bne     r1, r2, .al_endchk_done
    add.q   r20, r20, #1
    bra     .al_endchk
.al_endchk_done:
    beqz    r1, .al_emit
    move.q  r2, #0x0A               ; '\n'
    beq     r1, r2, .al_emit
    move.q  r2, #0x0D               ; '\r'
    beq     r1, r2, .al_emit
    move.q  r2, #0x3B               ; ';' comment
    beq     r1, r2, .al_emit
    bra     .al_err

.al_emit:
    move.q  r8, r21                 ; dest
    move.q  r9, r22                 ; opcode
    move.q  r10, r25                ; rd
    move.q  r11, r24                ; size
    move.q  r12, r26                ; xbit
    move.q  r13, r27                ; rs
    move.q  r14, r28                ; rt
    move.q  r15, r29                ; imm
    jsr     aot_emit_instr          ; R8 = dest + 8
    move.q  r9, r8                  ; new destination
    move.q  r8, #1
    rts
.al_err:
    move.q  r8, r0
    rts

; ============================================================================
; Private assembler - data directives (dc.b/w/l/q, align)
; ============================================================================
; aot_is_directive - R8 = 1 if the line at R8 (already at its first significant
; char) begins a "dc." or "align" directive, else 0. Clobbers R1, R2.
aot_is_directive:
    load.b  r1, (r8)
    or.q    r1, r1, #0x20
    move.q  r2, #0x64               ; 'd' -> maybe "dc."
    beq     r1, r2, .id_dc
    move.q  r2, #0x61               ; 'a' -> maybe "align"
    beq     r1, r2, .id_align
    move.q  r8, r0
    rts
.id_dc:
    load.b  r1, 1(r8)
    or.q    r1, r1, #0x20
    move.q  r2, #0x63               ; 'c'
    bne     r1, r2, .id_no
    load.b  r1, 2(r8)
    move.q  r2, #0x2E               ; '.'
    bne     r1, r2, .id_no
    move.q  r8, #1
    rts
.id_align:
    load.b  r1, 1(r8)
    or.q    r1, r1, #0x20
    move.q  r2, #0x6C               ; 'l'
    bne     r1, r2, .id_no
    load.b  r1, 4(r8)
    or.q    r1, r1, #0x20
    move.q  r2, #0x6E               ; 'n' (align)
    bne     r1, r2, .id_no
    move.q  r8, #1
    rts
.id_no:
    move.q  r8, r0
    rts

; aot_parse_value - parse a numeric literal or a label reference at R8.
; Output: R8 = value, R9 = 1 on success / 0 on failure, R10 = pointer past.
; A label is resolved through the symbol table. When AOT_LENIENT is non-zero an
; unknown label resolves to 0 (used by pass 1, where forward labels are not yet
; defined and only the operand length matters). Clobbers R1-R11 (R11 via the
; label length from aot_parse_label).
aot_parse_value:
.pv_sp:
    load.b  r1, (r8)
    move.q  r2, #0x20
    bne     r1, r2, .pv_try
    add.q   r8, r8, #1
    bra     .pv_sp
.pv_try:
    move.q  r7, r8                  ; save start (survives aot_parse_label)
    jsr     aot_parse_label         ; R8=nameptr R9=valid R10=past R11=len
    beqz    r9, .pv_num
    la      r1, EHBASIC_PRIV_MNEM_BUF
    store.q r8, (r1)               ; save name pointer (operand phase: buffer free)
    move.q  r9, r11                ; R9 = name length for the lookup
    jsr     aot_sym_lookup          ; R8=addr R9=found (R10, R11 preserved)
    bnez    r9, .pv_ok
    ; not a program label -> try a named assembler constant
    la      r1, EHBASIC_PRIV_MNEM_BUF
    load.q  r8, (r1)
    move.q  r9, r11
    jsr     aot_const_lookup        ; R8=value R9=found (R10 preserved)
    bnez    r9, .pv_ok
    ; neither label nor constant: pass 1 (lenient) treats it as 0; pass 2 fails.
    la      r1, AOT_LENIENT
    load.q  r1, (r1)
    beqz    r1, .pv_fail
    move.q  r8, r0
.pv_ok:
    move.q  r9, #1
    rts
.pv_num:
    move.q  r8, r7
    jsr     aot_parse_uint          ; R8=val R9=ok R10=past
    rts
.pv_fail:
    move.q  r9, r0
    rts

; aot_asm_data - assemble one data directive (dc.b/w/l/q or align) into the
; output buffer. R8 = directive text (at the keyword), R9 = destination,
; R10 = the PC this data lives at (for align padding). Output: R8 = 1 ok /
; 0 error / 2 overflow, R9 = destination advanced past the emitted bytes.
; Preserves R16-R18 (the program loop's cursor/offset). Clobbers R1-R15.
; Cross-call state lives in R12-R15 (dest, src, elemsize, buffer-end) because
; aot_parse_value clobbers R1-R11; R11 holds the PC, used only by align (which
; calls aot_parse_uint, not aot_parse_value).
aot_asm_data:
    move.q  r13, r8                 ; source cursor
    move.q  r12, r9                 ; destination cursor
    move.q  r11, r10               ; pc (align only)
    la      r1, AOT_CODE_END
    load.q  r15, (r1)              ; r15 = buffer end (0 = unbounded)
    load.b  r1, (r13)
    or.q    r1, r1, #0x20
    move.q  r2, #0x61               ; 'a' -> align
    beq     r1, r2, .ad_align
    load.b  r1, 3(r13)             ; the size letter in dc.X
    or.q    r1, r1, #0x20
    add.q   r13, r13, #4           ; past "dc.X"
    move.q  r2, #0x62               ; 'b'
    beq     r1, r2, .ad_b
    move.q  r14, #2
    move.q  r2, #0x77               ; 'w'
    beq     r1, r2, .ad_num
    move.q  r14, #4
    move.q  r2, #0x6C               ; 'l'
    beq     r1, r2, .ad_num
    move.q  r14, #8
    move.q  r2, #0x71               ; 'q'
    beq     r1, r2, .ad_num
    move.q  r8, r0                 ; unknown dc size
    rts

; dc.w/dc.l/dc.q: comma-separated numeric/label list, little-endian, r14 bytes.
.ad_num:
.adn_loop:
    load.b  r1, (r13)
    move.q  r2, #0x20
    bne     r1, r2, .adn_chk
    add.q   r13, r13, #1
    bra     .adn_loop
.adn_chk:
    beqz    r1, .ad_ok
    move.q  r2, #0x0A
    beq     r1, r2, .ad_ok
    move.q  r2, #0x0D
    beq     r1, r2, .ad_ok
    move.q  r2, #0x3B
    beq     r1, r2, .ad_ok
    beqz    r15, .adn_val          ; bounds: r14 bytes must fit
    add.q   r3, r12, r14
    bgt     r3, r15, .ad_oflow
.adn_val:
    move.q  r8, r13
    jsr     aot_parse_value         ; R8=val R9=ok R10=past (clobbers R1-R11)
    beqz    r9, .ad_err
    move.q  r13, r10               ; advance source
    move.q  r3, r14                ; byte count
    move.q  r4, r8                 ; value
.adn_wr:
    store.b r4, (r12)
    add.q   r12, r12, #1
    lsr.q   r4, r4, #8
    sub.q   r3, r3, #1
    bnez    r3, .adn_wr
.adn_aft:
    load.b  r1, (r13)
    move.q  r2, #0x20
    bne     r1, r2, .adn_aft2
    add.q   r13, r13, #1
    bra     .adn_aft
.adn_aft2:
    move.q  r2, #0x2C               ; ','
    bne     r1, r2, .ad_ok
    add.q   r13, r13, #1
    bra     .adn_loop

; dc.b: comma-separated bytes and "quoted strings".
.ad_b:
.adb_loop:
    load.b  r1, (r13)
    move.q  r2, #0x20
    bne     r1, r2, .adb_chk
    add.q   r13, r13, #1
    bra     .adb_loop
.adb_chk:
    beqz    r1, .ad_ok
    move.q  r2, #0x0A
    beq     r1, r2, .ad_ok
    move.q  r2, #0x0D
    beq     r1, r2, .ad_ok
    move.q  r2, #0x3B
    beq     r1, r2, .ad_ok
    move.q  r2, #0x22               ; '"'
    beq     r1, r2, .adb_str
    beqz    r15, .adb_num
    add.q   r3, r12, #1
    bgt     r3, r15, .ad_oflow
.adb_num:
    move.q  r8, r13
    jsr     aot_parse_value
    beqz    r9, .ad_err
    move.q  r13, r10
    store.b r8, (r12)
    add.q   r12, r12, #1
    bra     .adb_aft
.adb_str:
    add.q   r13, r13, #1           ; past opening quote
.adb_strch:
    load.b  r1, (r13)
    beqz    r1, .ad_err            ; unterminated string
    move.q  r2, #0x22
    beq     r1, r2, .adb_strend
    beqz    r15, .adb_put
    bge     r12, r15, .ad_oflow
.adb_put:
    store.b r1, (r12)
    add.q   r12, r12, #1
    add.q   r13, r13, #1
    bra     .adb_strch
.adb_strend:
    add.q   r13, r13, #1           ; past closing quote
.adb_aft:
    load.b  r1, (r13)
    move.q  r2, #0x20
    bne     r1, r2, .adb_aft2
    add.q   r13, r13, #1
    bra     .adb_aft
.adb_aft2:
    move.q  r2, #0x2C
    bne     r1, r2, .ad_ok
    add.q   r13, r13, #1
    bra     .adb_loop

; align N: pad with zero bytes to the next N-byte boundary of the PC.
.ad_align:
    add.q   r13, r13, #5           ; past "align"
.ada_sp:
    load.b  r1, (r13)
    move.q  r2, #0x20
    bne     r1, r2, .ada_n
    add.q   r13, r13, #1
    bra     .ada_sp
.ada_n:
    move.q  r8, r13
    jsr     aot_parse_uint          ; R8=N (clobbers R1-R6)
    beqz    r9, .ad_err
    move.q  r10, r8                 ; N
    beqz    r10, .ad_ok
    move.q  r3, r11                ; pc
    divu.l  r4, r3, r10
    mulu.l  r4, r4, r10
    sub.q   r3, r3, r4            ; pc mod N
    beqz    r3, .ad_ok            ; already aligned
    sub.q   r3, r10, r3          ; pad = N - (pc mod N)
.ada_fill:
    beqz    r15, .ada_put
    bge     r12, r15, .ad_oflow
.ada_put:
    store.b r0, (r12)
    add.q   r12, r12, #1
    sub.q   r3, r3, #1
    bnez    r3, .ada_fill
    bra     .ad_ok

.ad_ok:
    move.q  r9, r12                 ; new destination
    move.q  r8, #1
    rts
.ad_err:
    move.q  r8, r0
    rts
.ad_oflow:
    move.q  r8, #2
    rts

; ============================================================================
; Private assembler - two-pass program assembler
; ============================================================================
; aot_find_nextline - advance to the start of the next line.
; Input  R9  = any pointer within the current line
; Output R10 = pointer to the first char after the next '\n' (or the null)
; Clobbers R1, R2
aot_find_nextline:
    move.q  r10, r9
.fn_loop:
    load.b  r1, (r10)
    beqz    r1, .fn_end             ; null: stay at end
    move.q  r2, #0x0A               ; '\n'
    beq     r1, r2, .fn_nl
    add.q   r10, r10, #1
    bra     .fn_loop
.fn_nl:
    add.q   r10, r10, #1            ; past the newline
.fn_end:
    rts

; aot_line_class - classify one source line.
; Input  R8  = line-start pointer
; Output R8  = kind: 0 end-of-source, 1 blank/comment, 2 label only,
;                    3 instruction only, 4 label + instruction
;        R9  = instruction-start pointer (kinds 3 and 4)
;        R10 = pointer to the start of the next line
;        R11 = label name pointer (kinds 2 and 4)
;        R12 = label name length (kinds 2 and 4)
; Clobbers R1-R7
aot_line_class:
    move.q  r7, r8                  ; working cursor (survives aot_parse_label)
.lc_ws:
    load.b  r1, (r7)
    move.q  r2, #0x20               ; space
    beq     r1, r2, .lc_ws_adv
    move.q  r2, #0x09               ; tab
    beq     r1, r2, .lc_ws_adv
    bra     .lc_sig
.lc_ws_adv:
    add.q   r7, r7, #1
    bra     .lc_ws
.lc_sig:
    beqz    r1, .lc_end
    move.q  r2, #0x0A               ; '\n'
    beq     r1, r2, .lc_blank
    move.q  r2, #0x0D               ; '\r'
    beq     r1, r2, .lc_blank
    move.q  r2, #0x3B               ; ';' comment
    beq     r1, r2, .lc_blank
    ; Try a label: identifier immediately followed by ':'.
    move.q  r8, r7
    jsr     aot_parse_label         ; R8=nameptr R9=valid R10=past R11=len (R5-R7 safe)
    beqz    r9, .lc_instr           ; not an identifier -> instruction
    load.b  r1, (r10)
    move.q  r2, #0x3A               ; ':'
    bne     r1, r2, .lc_instr       ; identifier with no ':' -> mnemonic
    move.q  r5, r8                  ; save label name pointer
    move.q  r6, r11                ; save label name length
    add.q   r7, r10, #1            ; past ':'
.lc_l_ws:
    load.b  r1, (r7)
    move.q  r2, #0x20
    beq     r1, r2, .lc_l_adv
    move.q  r2, #0x09
    beq     r1, r2, .lc_l_adv
    bra     .lc_l_sig
.lc_l_adv:
    add.q   r7, r7, #1
    bra     .lc_l_ws
.lc_l_sig:
    beqz    r1, .lc_label_only
    move.q  r2, #0x0A
    beq     r1, r2, .lc_label_only
    move.q  r2, #0x0D
    beq     r1, r2, .lc_label_only
    move.q  r2, #0x3B
    beq     r1, r2, .lc_label_only
    ; Label followed by an instruction on the same line.
    move.q  r9, r7                  ; instruction pointer
    move.q  r11, r5                 ; name pointer
    move.q  r12, r6                ; name length
    jsr     aot_find_nextline       ; R10 = next line
    move.q  r8, #4
    rts
.lc_label_only:
    move.q  r9, r7
    move.q  r11, r5
    move.q  r12, r6
    jsr     aot_find_nextline
    move.q  r8, #2
    rts
.lc_instr:
    move.q  r9, r7                  ; instruction starts at first significant char
    jsr     aot_find_nextline
    move.q  r8, #3
    rts
.lc_blank:
    move.q  r9, r7
    jsr     aot_find_nextline
    move.q  r8, #1
    rts
.lc_end:
    move.q  r10, r7
    move.q  r8, r0
    rts

; aot_inc_class - classify a possible "include" pseudo-op at R8 (the first
; significant char of a line). The in-guest assembler has the ie64.inc constants
; baked in via aot_consttab, so a user source that opens with `include "ie64.inc"`
; is honoured as a no-op (emit nothing); any other include is rejected. Every
; other directive/mnemonic (incbin, equ, org, macros, conditionals, ...) is not
; recognised by the assembler and already fails as an unknown mnemonic, so only
; include needs special handling here.
; Output: R8 = 0 not an include (assemble the line normally)
;              1 include "ie64.inc" (recognised no-op)
;              2 any other / malformed include (assembler error)
; Clobbers: R1-R4. Preserves R5-R31 (notably R17/R18/R19 in the pass loops).
aot_inc_class:
    move.q  r2, r8                 ; working cursor (R8 reused for the result)
    la      r3, aot_str_kw_include
.ic_kw:
    load.b  r4, (r3)
    beqz    r4, .ic_kw_done
    load.b  r1, (r2)
    or.q    r1, r1, #0x20          ; case-insensitive
    bne     r1, r4, .ic_not
    add.q   r2, r2, #1
    add.q   r3, r3, #1
    bra     .ic_kw
.ic_kw_done:
    ; "include" must be followed by whitespace, else it is some other token.
    load.b  r1, (r2)
    move.q  r4, #0x20
    beq     r1, r4, .ic_sp
    move.q  r4, #0x09
    beq     r1, r4, .ic_sp
    bra     .ic_not
.ic_sp:
    load.b  r1, (r2)
    move.q  r4, #0x20
    beq     r1, r4, .ic_sp_adv
    move.q  r4, #0x09
    beq     r1, r4, .ic_sp_adv
    bra     .ic_q
.ic_sp_adv:
    add.q   r2, r2, #1
    bra     .ic_sp
.ic_q:
    move.q  r4, #0x22              ; opening '"'
    bne     r1, r4, .ic_reject
    add.q   r2, r2, #1
    la      r3, aot_str_ie64inc
.ic_cmp:
    load.b  r4, (r3)
    beqz    r4, .ic_cmp_end
    load.b  r1, (r2)
    or.q    r1, r1, #0x20
    bne     r1, r4, .ic_reject
    add.q   r2, r2, #1
    add.q   r3, r3, #1
    bra     .ic_cmp
.ic_cmp_end:
    load.b  r1, (r2)               ; must be the closing '"'
    move.q  r4, #0x22
    bne     r1, r4, .ic_reject
    add.q   r2, r2, #1             ; past the closing quote
.ic_tail:
    load.b  r1, (r2)               ; only whitespace then end-of-line/comment may follow
    move.q  r4, #0x20
    beq     r1, r4, .ic_tail_adv
    move.q  r4, #0x09
    beq     r1, r4, .ic_tail_adv
    beqz    r1, .ic_ok             ; end of text
    move.q  r4, #0x0A
    beq     r1, r4, .ic_ok         ; end of line
    move.q  r4, #0x0D
    beq     r1, r4, .ic_ok
    move.q  r4, #0x3B              ; ';' comment
    beq     r1, r4, .ic_ok
    bra     .ic_reject             ; trailing junk after include "ie64.inc"
.ic_tail_adv:
    add.q   r2, r2, #1
    bra     .ic_tail
.ic_ok:
    move.q  r8, #1
    rts
.ic_reject:
    move.q  r8, #2
    rts
.ic_not:
    move.q  r8, r0
    rts

aot_str_kw_include:
    dc.b    "include", 0
aot_str_ie64inc:
    dc.b    "ie64.inc", 0
    align 4

; aot_asm_program - assemble multi-line asm text into machine code.
; Lines are '\n'-separated; the text is null-terminated. Labels ("name:") may
; stand alone or precede an instruction. Pass 1 records label addresses; pass 2
; emits instructions, resolving branch/jsr labels PC-relative to the link base.
;
; Input:  R8  = source text pointer
;         R9  = link base address (where the code will live)
;         R10 = destination buffer pointer
;         AOT_CODE_END = byte after the destination buffer (0 = unbounded)
; Output: R8  = 1 on success, 0 on error, 2 on output overflow
;         R9  = total code length in bytes (valid on success)
; Clobbers: R1-R15, R17-R29
aot_asm_program:
    la      r1, AOT_ASM_SRC
    store.q r8, (r1)
    la      r1, AOT_ASM_BASE
    store.q r9, (r1)
    la      r1, AOT_ASM_DEST
    store.q r10, (r1)

    ; Lines are 8-byte instructions or variable-length data directives (dc.*,
    ; align), so both passes track a running byte offset. Pass 1 records label
    ; addresses and sizes directives leniently (forward labels resolve to 0,
    ; which does not affect length); pass 2 emits with labels resolved.

    ; --- Pass 1: collect label addresses, compute the layout ---
    jsr     aot_sym_reset
    jsr     aot_scope_reset
    ; Inject bundled-helper symbols so the user code can call them. The helper
    ; machine code is copied ahead of the user code at a known address.
    la      r1, AOT_HELPER_FP
    load.q  r1, (r1)
    beqz    r1, .ap_no_fp
    la      r8, aot_str_fp_print_name
    move.q  r9, #8                  ; len("fp_print")
    la      r1, AOT_HELPER_FP_ADDR
    load.q  r10, (r1)
    jsr     aot_sym_add
.ap_no_fp:
    ; Inject __rtpay = the bundled runtime-blob payload address (set by COMPILE before
    ; the real assembly), so the standalone bootstrap's copy resolves to the blob bytes
    ; appended after the user code. 0 during the sizing pass (lenient).
    la      r1, AOT_USE_RT
    load.q  r1, (r1)
    beqz    r1, .ap_no_helpers
    la      r8, aot_str_rtpay_name
    move.q  r9, #7                  ; len("__rtpay")
    la      r1, AOT_RT_PAYLOAD
    load.q  r10, (r1)
    jsr     aot_sym_add
    ; Inject __rtprog = the bundled tokenised-programme payload address (READ/DATA).
    la      r1, AOT_USE_PROG
    load.q  r1, (r1)
    beqz    r1, .ap_no_helpers
    la      r8, aot_str_rtprog_name
    move.q  r9, #8                  ; len("__rtprog")
    la      r1, AOT_RT_PROG_PAY
    load.q  r10, (r1)
    jsr     aot_sym_add
.ap_no_helpers:
    la      r1, AOT_LENIENT
    move.q  r2, #1
    store.q r2, (r1)               ; unknown labels -> 0 while sizing
    move.q  r18, r0                 ; offset (r16/r17/r18 survive the emitters)
    la      r1, AOT_ASM_SRC
    load.q  r17, (r1)               ; cursor
.p1_loop:
    move.q  r8, r17
    jsr     aot_line_class          ; R8=kind R9=instr R10=next R11=nameptr R12=len
    move.q  r17, r10
    move.q  r19, r9                 ; instr/directive pointer (kinds 3/4)
    move.q  r20, r11                ; label name pointer (kinds 2/4)
    move.q  r21, r12               ; label name length (kinds 2/4)
    beqz    r8, .p1_done
    move.q  r1, #2
    beq     r8, r1, .p1_label
    move.q  r1, #4
    beq     r8, r1, .p1_label_instr
    move.q  r1, #3
    beq     r8, r1, .p1_instr
    bra     .p1_loop                ; blank/comment
.p1_label:
    move.q  r8, r20                ; name pointer
    jsr     aot_scope_enter        ; global label -> new scope
    move.q  r8, r20
    move.q  r9, r21               ; name length
    la      r1, AOT_ASM_BASE
    load.q  r10, (r1)
    add.q   r10, r10, r18          ; address = base + offset
    jsr     aot_sym_add             ; R11 = 1 ok / 0 full
    beqz    r11, .p1_err
    bra     .p1_loop
.p1_label_instr:
    move.q  r8, r20
    jsr     aot_scope_enter
    move.q  r8, r20
    move.q  r9, r21
    la      r1, AOT_ASM_BASE
    load.q  r10, (r1)
    add.q   r10, r10, r18
    jsr     aot_sym_add
    beqz    r11, .p1_err
    ; fall through to size the instruction/directive
.p1_instr:
    move.q  r8, r19
    jsr     aot_inc_class           ; R8: 0 normal, 1 include-noop, 2 reject
    move.q  r1, #1
    beq     r8, r1, .p1_loop        ; include "ie64.inc" -> emit nothing
    move.q  r1, #2
    beq     r8, r1, .p1_err         ; any other include -> assembler error
    move.q  r8, r19
    jsr     aot_is_directive        ; R8 = 1 if dc/align
    beqz    r8, .p1_fixed
    ; size the directive by emitting it (lenient) into its real slot
    la      r1, AOT_ASM_DEST
    load.q  r2, (r1)
    add.q   r9, r2, r18            ; dest
    la      r1, AOT_ASM_BASE
    load.q  r2, (r1)
    add.q   r10, r2, r18           ; pc
    move.q  r8, r19
    jsr     aot_asm_data            ; R8=status R9=newdest
    move.q  r1, #2
    beq     r8, r1, .p1_oflow
    beqz    r8, .p1_err
    la      r1, AOT_ASM_DEST
    load.q  r2, (r1)
    add.q   r2, r2, r18            ; dest
    sub.q   r9, r9, r2            ; emitted length
    add.q   r18, r18, r9
    bra     .p1_loop
.p1_fixed:
    add.q   r18, r18, #8
    bra     .p1_loop
.p1_err:
    la      r1, AOT_LENIENT
    store.q r0, (r1)
    bra     .p_err
.p1_oflow:
    la      r1, AOT_LENIENT
    store.q r0, (r1)
    bra     .p_oflow
.p1_done:
    la      r1, AOT_LENIENT
    store.q r0, (r1)               ; pass 2 resolves labels strictly

    ; --- Pass 2: assemble, resolving labels ---
    jsr     aot_scope_reset         ; re-walk globals so scope ids match pass 1
    move.q  r18, r0                 ; offset
    la      r1, AOT_ASM_SRC
    load.q  r17, (r1)               ; cursor
.p2_loop:
    move.q  r8, r17
    jsr     aot_line_class          ; R8=kind R9=instr R10=next R11=nameptr
    move.q  r17, r10                ; advance cursor (survives the emitters)
    move.q  r19, r9                 ; instr/directive pointer
    move.q  r23, r11               ; label name pointer (kinds 2/4)
    beqz    r8, .p2_done
    move.q  r1, #2
    beq     r8, r1, .p2_label
    move.q  r1, #3
    beq     r8, r1, .p2_instr
    move.q  r1, #4
    beq     r8, r1, .p2_label_instr
    bra     .p2_loop                ; blank/comment
.p2_label:
    move.q  r8, r23
    jsr     aot_scope_enter         ; track scope for following local references
    bra     .p2_loop
.p2_label_instr:
    move.q  r8, r23
    jsr     aot_scope_enter
    ; fall through to assemble the instruction
.p2_instr:
    move.q  r8, r19
    jsr     aot_inc_class           ; R8: 0 normal, 1 include-noop, 2 reject
    move.q  r1, #1
    beq     r8, r1, .p2_loop        ; include "ie64.inc" -> emit nothing
    move.q  r1, #2
    beq     r8, r1, .p_err          ; any other include -> assembler error
    move.q  r8, r19
    jsr     aot_is_directive
    move.q  r20, r8                 ; isDirective
    la      r1, AOT_ASM_DEST
    load.q  r2, (r1)
    add.q   r21, r2, r18           ; dest = destbase + offset
    la      r1, AOT_ASM_BASE
    load.q  r2, (r1)
    add.q   r22, r2, r18           ; pc = base + offset
    ; overflow pre-check for the fixed-size instruction path (data self-checks)
    bnez    r20, .p2_emit
    la      r1, AOT_CODE_END
    load.q  r3, (r1)
    beqz    r3, .p2_emit
    add.q   r4, r21, #8
    bgt     r4, r3, .p_oflow
.p2_emit:
    move.q  r8, r19                ; src
    move.q  r9, r21                ; dest
    move.q  r10, r22               ; pc
    bnez    r20, .p2_data
    jsr     aot_asm_line            ; R8=status R9=newdest
    bra     .p2_after
.p2_data:
    jsr     aot_asm_data            ; R8=status R9=newdest
.p2_after:
    move.q  r1, #2
    beq     r8, r1, .p_oflow
    beqz    r8, .p_err
    la      r1, AOT_ASM_DEST
    load.q  r2, (r1)
    add.q   r2, r2, r18            ; dest (r18 preserved across the emitters)
    sub.q   r9, r9, r2            ; emitted length = newdest - dest
    add.q   r18, r18, r9
    bra     .p2_loop
.p2_done:
    move.q  r9, r18                 ; total code length
    move.q  r8, #1
    rts
.p_err:
    move.q  r8, r0
    rts
.p_oflow:
    move.q  r8, #2                  ; code buffer overflow
    rts

; ============================================================================
; Transpiler - BASIC tokens to IE64 asm text
; ============================================================================
; aot_emit_str - append a null-terminated string to a text buffer.
; Input  R8 = destination cursor, R9 = source string pointer
; Output R8 = destination cursor advanced past the copied bytes
; Clobbers R1, R9
aot_emit_str:
.es_loop:
    load.b  r1, (r9)
    beqz    r1, .es_done
    store.b r1, (r8)
    add.q   r8, r8, #1
    add.q   r9, r9, #1
    bra     .es_loop
.es_done:
    rts

; aot_text_room - R8 = 1 if the asm-text buffer has >= 256 bytes of headroom at
; the current cursor R17 (covers any single statement's emit, including the
; multi-line WAIT/VSYNC poll loops), or if AOT_TEXT_END is 0 (unbounded).
; Else R8 = 0. Clobbers R1-R3.
aot_text_room:
    la      r1, AOT_TEXT_END
    load.q  r2, (r1)
    beqz    r2, .room_ok           ; unbounded (standalone unit tests)
    add.q   r3, r17, #256
    bgt     r3, r2, .room_no
.room_ok:
    move.q  r8, #1
    rts
.room_no:
    move.q  r8, r0
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
aot_transpile:
    move.q  r17, r9                 ; output cursor (survives aot_emit_str)
    move.q  r19, r9                 ; output start (for the length)
    move.q  r18, r8                 ; current line record
    move.q  r21, r10                ; end mode
    la      r1, AOT_TEXT_END
    store.q r11, (r1)               ; output limit for aot_text_room
    la      r1, AOT_LBLCNT
    store.q r0, (r1)               ; reset the per-compile branch-label counter
    la      r1, AOT_USE_PRINT
    store.q r0, (r1)               ; reset the bundled-helper request flags
    la      r1, AOT_USE_FP
    store.q r0, (r1)
    la      r1, AOT_FOR_SP
    store.q r0, (r1)               ; reset the compile-time loop stacks
    la      r1, AOT_WHILE_SP
    store.q r0, (r1)
    la      r1, AOT_DO_SP
    store.q r0, (r1)
    la      r1, AOT_USE_RT
    store.q r0, (r1)               ; reset runtime-blob-needed flag
    la      r1, AOT_USE_PROG
    store.q r0, (r1)               ; reset programme-bundling flag
    ; Standalone only: pre-scan the whole programme. Any expression/control-flow token
    ; sets AOT_USE_RT (needs the runtime blob); READ/DATA/RESTORE additionally set
    ; AOT_USE_PROG (the tokenised programme must be bundled for the blob's DATA reader).
    ; The scan runs to the end so a READ/DATA anywhere is caught.
    beqz    r21, .tr_no_prescan
    move.q  r1, r18                ; first line record
.tr_ps_line:
    load.q  r2, (r1)               ; next pointer
    beqz    r2, .tr_ps_done
    add.q   r3, r1, #16            ; statement tokens
.tr_ps_tok:
    load.b  r4, (r3)
    beqz    r4, .tr_ps_next
    move.q  r5, #0x83              ; TK_DATA
    beq     r4, r5, .tr_ps_prog
    move.q  r5, #0x86              ; TK_READ
    beq     r4, r5, .tr_ps_prog
    move.q  r5, #0x8C              ; TK_RESTORE
    beq     r4, r5, .tr_ps_prog
    move.q  r5, #0xA0              ; TK_LIST -> lists the bundled programme
    beq     r4, r5, .tr_ps_prog
    move.q  r5, #0x96              ; TK_SAVE -> serialises the bundled programme
    beq     r4, r5, .tr_ps_prog
    move.q  r5, #0x84              ; TK_INPUT -> needs the runtime blob (read_line/expr)
    beq     r4, r5, .tr_ps_rt
    move.q  r5, #0xA3              ; TK_WIDTH->BLOAD -> needs the blob (exec_do_bload)
    beq     r4, r5, .tr_ps_rt
    move.q  r5, #0x8B              ; TK_IF
    beq     r4, r5, .tr_ps_rt
    move.q  r5, #0x91              ; TK_ON
    beq     r4, r5, .tr_ps_rt
    move.q  r5, #0x81              ; TK_FOR
    beq     r4, r5, .tr_ps_rt
    move.q  r5, #0x82              ; TK_NEXT
    beq     r4, r5, .tr_ps_rt
    move.q  r5, #0x9C              ; TK_DO
    beq     r4, r5, .tr_ps_rt
    move.q  r5, #0x9D              ; TK_LOOP
    beq     r4, r5, .tr_ps_rt
    ; DEF FN is rejected at the statement level (TK_DEF reaches .tr_unsup). FN() in an
    ; expression is not a distinct token (the tokeniser keeps "FN" as literal letters),
    ; so it cannot be pattern-rejected here; without a DEF FN binding the bundled
    ; expr_eval raises a clean error at run time (caught by the post-call error halt),
    ; matching the interpreter, so no extra rejection is needed.
    move.q  r5, #0xA9              ; operator/function tokens [0xA9, 0xE2)
    blt     r4, r5, .tr_ps_letter
    move.q  r5, #0xE2
    blt     r4, r5, .tr_ps_rt
.tr_ps_letter:
    or.q    r6, r4, #0x20          ; fold case: ASCII letter -> variable reference
    move.q  r5, #0x61
    blt     r6, r5, .tr_ps_strchk
    move.q  r5, #0x7A
    ble     r6, r5, .tr_ps_rt
.tr_ps_strchk:
    move.q  r5, #0x22              ; '"' -> skip a string literal (ignore tokens inside)
    bne     r4, r5, .tr_ps_adv
    add.q   r3, r3, #1
.tr_ps_str:
    load.b  r4, (r3)
    beqz    r4, .tr_ps_next
    move.q  r5, #0x22
    beq     r4, r5, .tr_ps_stradv
    add.q   r3, r3, #1
    bra     .tr_ps_str
.tr_ps_stradv:
    add.q   r3, r3, #1
    bra     .tr_ps_tok
.tr_ps_rt:
    la      r6, AOT_USE_RT
    move.q  r7, #1
    store.q r7, (r6)
    bra     .tr_ps_adv
.tr_ps_prog:
    la      r6, AOT_USE_RT
    move.q  r7, #1
    store.q r7, (r6)
    la      r6, AOT_USE_PROG
    store.q r7, (r6)
.tr_ps_adv:
    add.q   r3, r3, #1
    bra     .tr_ps_tok
.tr_ps_next:
    move.q  r1, r2
    bra     .tr_ps_line
.tr_ps_done:
    la      r1, AOT_USE_RT
    load.q  r1, (r1)
    beqz    r1, .tr_no_prescan
    ; Read the runtime blob now (compile time) so AOT_RT_LEN is known when the prologue
    ; bakes the copy length and the bytes are available to bundle. A missing/unreadable
    ; blob leaves AOT_RT_LEN = 0; the compile path reports a file error only for an
    ; otherwise-supported programme (an unsupported statement is reported first).
    jsr     aot_read_rt_blob
    ; If the tokenised programme is bundled (READ/DATA), measure its byte length now
    ; (state[ST_PROG_END] - state[ST_PROG_START]) so the bootstrap copy length baked during
    ; transpile is correct. The sizing phase recomputes the same value for the append.
    la      r1, AOT_USE_PROG
    load.q  r1, (r1)
    beqz    r1, .tr_no_prescan
    move.q  r2, #(BASIC_STATE + ST_PROG_END)
    load.q  r2, (r2)
    move.q  r3, #(BASIC_STATE + ST_PROG_START)
    load.q  r3, (r3)
    sub.q   r2, r2, r3
    la      r1, AOT_RT_PROG_LEN
    store.q r2, (r1)
.tr_no_prescan:
    ; Standalone (COMPILE) bootstrap: a flat .ie64 boots with no resident setup, so
    ; establish the stack, the TERM_OUT pointer (R26) the bundled print helpers use,
    ; the state base (R16) and TERM_STATUS (R27). Emitted before the first line
    ; label so a GOTO back to the first line does not re-run the bootstrap.
    beqz    r21, .tr_arena_prologue
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    move.q  r8, r17
    la      r9, aot_str_movel_r16  ; move.l r16, #BASIC_STATE
    move.q  r10, #BASIC_STATE
    jsr     aot_emit_movel
    la      r9, aot_str_basic_layout_init
    jsr     aot_emit_str
    la      r9, aot_str_movel_r26  ; move.l r26, #TERM_OUT
    move.q  r10, #TERM_OUT
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r27  ; move.l r27, #TERM_STATUS
    move.q  r10, #TERM_STATUS
    jsr     aot_emit_movel
    ; If the programme needs the runtime blob, read it into AOT_RT_BASE and run
    ; var_init (R16/R26/R27/R31 are set above). aot_emit_rt_bootstrap threads R8.
    la      r1, AOT_USE_RT
    load.q  r1, (r1)
    beqz    r1, .tr_prologue_done
    jsr     aot_emit_rt_bootstrap
.tr_prologue_done:
    move.q  r17, r8
    bra     .tr_line
    ; Arena prologue: save the entry stack pointer so END/STOP can unwind a
    ; compiled GOSUB back to the REPL trampoline. Emitted before the first line
    ; label so a GOTO back to the first line does not re-run it.
.tr_arena_prologue:
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    move.q  r8, r17
    la      r9, aot_str_movel_r6   ; move.l r6, #AOT_SAVED_SP
    move.q  r10, #AOT_SAVED_SP
    jsr     aot_emit_movel
    la      r9, aot_str_save_sp    ; store.q r31, (r6)
    jsr     aot_emit_str
    move.q  r17, r8
.tr_line:
    load.q  r1, (r18)              ; next pointer
    beqz    r1, .tr_finish          ; next == 0 -> terminator, no body
    la      r1, AOT_IF_LABEL       ; no pending IF end/else labels at the line start
    store.q r0, (r1)
    la      r1, AOT_IF_ELSE
    store.q r0, (r1)
    ; Emit "L<lineNo>:" so GOTO/GOSUB targets resolve to this line's code.
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    move.q  r8, r17
    load.l  r9, 8(r18)             ; line number
    jsr     aot_emit_label_def
    move.q  r17, r8
    add.q   r20, r18, #16          ; statement cursor = tokens
.tr_stmt:
    load.b  r1, (r20)
    move.q  r2, #0x20
    bne     r1, r2, .tr_chk
    add.q   r20, r20, #1
    bra     .tr_stmt
.tr_chk:
    beqz    r1, .tr_advance
    move.q  r2, #0x3A               ; ':'
    beq     r1, r2, .tr_sep
    move.q  r2, #0x80               ; TK_END
    beq     r1, r2, .tr_end_tok
    move.q  r2, #0x90               ; TK_STOP
    beq     r1, r2, .tr_stop        ; arena: save CONT continuation; standalone: halt
    move.q  r2, #0x8F               ; TK_REM
    beq     r1, r2, .tr_advance     ; REM: rest of line is comment
    move.q  r2, #0x89               ; TK_GOTO
    beq     r1, r2, .tr_goto
    move.q  r2, #0x8D               ; TK_GOSUB
    beq     r1, r2, .tr_gosub
    move.q  r2, #0x8E               ; TK_RETURN
    beq     r1, r2, .tr_return
    move.q  r2, #0x92               ; TK_EXT
    beq     r1, r2, .tr_ext
    move.q  r2, #0x98               ; TK_POKE
    beq     r1, r2, .tr_poke
    move.q  r2, #0x9B               ; TK_CALL
    beq     r1, r2, .tr_call
    move.q  r2, #0xA6               ; TK_BITSET
    beq     r1, r2, .tr_bitset
    move.q  r2, #0xA7               ; TK_BITCLR
    beq     r1, r2, .tr_bitclr
    move.q  r2, #0x94               ; TK_WAIT
    beq     r1, r2, .tr_wait
    move.q  r2, #0xE6               ; TK_VSYNC_CMD
    beq     r1, r2, .tr_vsync
    move.q  r2, #0x9E               ; TK_PRINT
    beq     r1, r2, .tr_print
    move.q  r2, #0x8B               ; TK_IF
    beq     r1, r2, .tr_if
    move.q  r2, #0xAB               ; TK_ELSE
    beq     r1, r2, .tr_else
    move.q  r2, #0x91               ; TK_ON
    beq     r1, r2, .tr_on
    move.q  r2, #0x81               ; TK_FOR
    beq     r1, r2, .tr_for
    move.q  r2, #0x82               ; TK_NEXT
    beq     r1, r2, .tr_next
    move.q  r2, #0xB0               ; TK_WHILE
    beq     r1, r2, .tr_while
    move.q  r2, #0xAF               ; TK_UNTIL (= WEND at statement start)
    beq     r1, r2, .tr_wend
    move.q  r2, #0x9C               ; TK_DO
    beq     r1, r2, .tr_do
    move.q  r2, #0x9D               ; TK_LOOP
    beq     r1, r2, .tr_loop
; No native handler: delegate leaf statements to their resident handler via the
; interpreter's stmt_jump_table. Control-flow / REPL tokens cannot be delegated
; (their R28/R14 effects need the interpreter's execution loop) -> real stub.
.tr_try_delegate:
    ; Delegation calls resident handlers via stmt_jump_table, which only exist
    ; inside the interpreter (RUN AOT). A standalone COMPILE image has neither the
    ; resident handlers nor the resident token addresses. Standalone instead supports
    ; an implied LET (variable assignment) by bundling the statement tokens and calling
    ; the bundled exec_do_let; anything else delegated is unsupported.
    beqz    r21, .tr_arena_delegate
    load.b  r1, (r20)
    move.q  r2, #0x85             ; TK_DIM -> bundle + bundled exec_do_dim
    beq     r1, r2, .tr_dim_standalone
    move.q  r2, #0x83             ; TK_DATA -> data only, emit no code
    beq     r1, r2, .tr_skip_stmt
    move.q  r2, #0x86             ; TK_READ -> bundle + bundled exec_do_read
    beq     r1, r2, .tr_read_standalone
    move.q  r2, #0x8C             ; TK_RESTORE -> bundle + bundled exec_do_restore
    beq     r1, r2, .tr_restore_standalone
    move.q  r2, #0x84             ; TK_INPUT -> bundle + bundled exec_do_input
    beq     r1, r2, .tr_input_standalone
    move.q  r2, #0xA0             ; TK_LIST -> bundled exec_do_list (lists bundled prog)
    beq     r1, r2, .tr_list_standalone
    move.q  r2, #0x96             ; TK_SAVE -> bundle filename + bundled exec_do_save
    beq     r1, r2, .tr_save_standalone
    move.q  r2, #0xA3             ; TK_WIDTH->BLOAD -> bundle + bundled exec_do_bload
    beq     r1, r2, .tr_bload_standalone
    ; TK_LOAD (0x95) reconstructs a tokenised programme, which needs the resident
    ; tokeniser and a REPL loop to run it; reject (falls to .tr_unsup). BLOAD is a
    ; raw binary load (File I/O MMIO), so it is bundled above like SAVE.
    or.q    r2, r1, #0x20         ; fold case: a letter starts an implied LET
    move.q  r3, #0x61
    blt     r2, r3, .tr_unsup
    move.q  r3, #0x7A
    bgt     r2, r3, .tr_unsup
    bra     .tr_let_standalone
.tr_read_standalone:
    add.q   r20, r20, #1           ; consume TK_READ -> R20 at the variable list
    jsr     aot_emit_cond_span     ; bundle the variable list -> move.l r17, #Q<id>
    beqz    r9, .tr_oflow
    move.q  r8, r17
    move.q  r10, #RT_EXEC_DO_READ  ; call exec_do_read (checked: halt on error)
    jsr     aot_emit_rt_call
    move.q  r17, r8
    bra     .tr_skip_stmt
.tr_restore_standalone:
    add.q   r20, r20, #1           ; consume TK_RESTORE
    jsr     aot_emit_cond_span     ; bundle any RESTORE argument -> move.l r17, #Q<id>
    beqz    r9, .tr_oflow
    move.q  r8, r17
    move.q  r10, #RT_EXEC_DO_RESTORE ; call exec_do_restore (checked: halt on error)
    jsr     aot_emit_rt_call
    move.q  r17, r8
    bra     .tr_skip_stmt
.tr_dim_standalone:
    add.q   r20, r20, #1           ; consume TK_DIM -> R20 at the array spec
    jsr     aot_emit_cond_span     ; bundle the DIM spec -> move.l r17, #Q<id>
    beqz    r9, .tr_oflow
    move.q  r8, r17
    move.q  r10, #RT_EXEC_DO_DIM   ; call exec_do_dim (checked: halt on error)
    jsr     aot_emit_rt_call
    move.q  r17, r8
    bra     .tr_skip_stmt
; INPUT: bundle the operand span (optional quoted prompt, variable list) and call the
; bundled exec_do_input, which prints the prompt, reads a line per variable via
; read_line, and stores through var_lookup, all inside the runtime blob.
.tr_input_standalone:
    add.q   r20, r20, #1           ; consume TK_INPUT -> R20 at the prompt/var list
    jsr     aot_emit_cond_span     ; bundle the operands -> move.l r17, #Q<id>
    beqz    r9, .tr_oflow
    move.q  r8, r17
    move.q  r10, #RT_EXEC_DO_INPUT ; call exec_do_input (checked: halt on error)
    jsr     aot_emit_rt_call
    move.q  r17, r8
    bra     .tr_skip_stmt
; LIST: call the bundled exec_do_list, which detokenises and prints the bundled
; tokenised programme (walked from state[ST_PROG_START] = AOT_RT_PROG). Any line-range
; argument is bundled and ignored, matching the interpreter (it always lists all).
.tr_list_standalone:
    add.q   r20, r20, #1           ; consume TK_LIST -> R20 at any line-range argument
    jsr     aot_emit_cond_span     ; bundle (and skip) the range -> move.l r17, #Q<id>
    beqz    r9, .tr_oflow
    move.q  r8, r17
    move.q  r10, #RT_EXEC_DO_LIST  ; call exec_do_list (checked: halt on error)
    jsr     aot_emit_rt_call
    move.q  r17, r8
    bra     .tr_skip_stmt
; SAVE: bundle the filename string expression and call the bundled exec_do_save, which
; detokenises the bundled programme to a transient low32 bridge and writes it via the File I/O ABI.
.tr_save_standalone:
    add.q   r20, r20, #1           ; consume TK_SAVE -> R20 at the filename expression
    jsr     aot_emit_cond_span     ; bundle the filename -> move.l r17, #Q<id>
    beqz    r9, .tr_oflow
    move.q  r8, r17
    move.q  r10, #RT_EXEC_DO_SAVE  ; call exec_do_save (checked: halt on error)
    jsr     aot_emit_rt_call
    move.q  r17, r8
    bra     .tr_skip_stmt
; BLOAD "file", addr: bundle the operand span (filename string, comma, destination
; expression) and call the bundled exec_do_bload, which loads raw bytes to the
; destination through the File I/O MMIO (FILE_NAME_PTR / FILE_DATA_PTR / FILE_CTRL =
; OP_READ), identical to the interpreter. The destination 2^32 range check and the
; ?FILE NOT FOUND / ?FC ERROR reporting come with the bundled handler.
.tr_bload_standalone:
    add.q   r20, r20, #1           ; consume TK_BLOAD -> R20 at the operands
    jsr     aot_emit_cond_span     ; bundle "file", addr -> move.l r17, #Q<id>
    beqz    r9, .tr_oflow
    move.q  r8, r17
    move.q  r10, #RT_EXEC_DO_BLOAD ; call exec_do_bload (checked: halt on error)
    jsr     aot_emit_rt_call
    move.q  r17, r8
    bra     .tr_skip_stmt
.tr_arena_delegate:
    load.b  r1, (r20)
    move.q  r2, #0x80
    blt     r1, r2, .tr_maybe_let   ; < 0x80: a variable letter -> implied LET
    move.q  r2, #0x81               ; FOR
    beq     r1, r2, .tr_unsup
    move.q  r2, #0x82               ; NEXT
    beq     r1, r2, .tr_unsup
    move.q  r2, #0x8A               ; RUN
    beq     r1, r2, .tr_unsup
    move.q  r2, #0x8B               ; IF
    beq     r1, r2, .tr_unsup
    move.q  r2, #0x91               ; ON
    beq     r1, r2, .tr_unsup
    move.q  r2, #0x9C               ; DO
    beq     r1, r2, .tr_unsup
    move.q  r2, #0x9D               ; LOOP
    beq     r1, r2, .tr_unsup
    move.q  r2, #0x9F               ; CONT
    beq     r1, r2, .tr_unsup
    move.q  r2, #0xA0               ; LIST
    beq     r1, r2, .tr_unsup
    move.q  r2, #0xA2               ; NEW
    beq     r1, r2, .tr_unsup
    move.q  r2, #0xAF               ; UNTIL
    beq     r1, r2, .tr_unsup
    move.q  r2, #0xB0               ; WHILE
    beq     r1, r2, .tr_unsup
    ; Statement tokens are <= 0xA8 (END..TAB) or the hardware range >= 0xE2;
    ; syntactic tokens (TO/ELSE/THEN/STEP), operators and functions (0xA9..0xE1)
    ; cannot start a statement.
    move.q  r2, #0xA9
    blt     r1, r2, .tr_delegate_token
    move.q  r2, #0xE2
    blt     r1, r2, .tr_unsup
.tr_delegate_token:
    add.q   r9, r20, #1            ; R17 target = past the token byte
    move.q  r2, #0x80
    sub.q   r2, r1, r2            ; token index
    lsl.q   r2, r2, #2
    move.q  r3, #stmt_jump_table
    add.q   r10, r3, r2          ; jump-table slot address
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    move.q  r8, r17
    jsr     aot_emit_delegate
    move.q  r17, r8
    bra     .tr_skip_stmt
.tr_maybe_let:
    or.q    r2, r1, #0x20          ; a letter -> implied LET, else junk
    move.q  r3, #0x61
    blt     r2, r3, .tr_unsup
    move.q  r3, #0x7A
    bgt     r2, r3, .tr_unsup
    move.q  r9, r20                ; R17 target = the variable
    move.q  r10, #(stmt_jump_table + 28)  ; TK_LET handler slot (index 7 * 4)
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    move.q  r8, r17
    jsr     aot_emit_delegate
    move.q  r17, r8
    bra     .tr_skip_stmt
; Standalone implied LET: bundle the statement tokens (the variable, '=', and the
; RHS expression) and call the bundled exec_do_let, which parses the assignment from
; R17 (var_lookup + expr_eval/str_eval + store), all inside the runtime blob.
.tr_let_standalone:
    jsr     aot_emit_cond_span     ; bundle [R20..sep) -> move.l r17, #Q<id>
    beqz    r9, .tr_oflow
    move.q  r8, r17
    move.q  r10, #RT_EXEC_DO_LET   ; call exec_do_let (checked: halt on error)
    jsr     aot_emit_rt_call
    move.q  r17, r8
    bra     .tr_skip_stmt
; Skip the delegated statement's tokens to the next ':' or end of line, leaving
; the runtime R17 (advanced by the handler) and the compile cursor R20 in sync.
; Quote-aware so a ':' inside a string literal does not split the statement.
.tr_skip_stmt:
    load.b  r1, (r20)
    beqz    r1, .tr_advance
    move.q  r2, #0x3A               ; ':' -> separator
    beq     r1, r2, .tr_stmt
    move.q  r2, #0xAB               ; TK_ELSE -> hand to .tr_else (closes THEN clause)
    beq     r1, r2, .tr_stmt
    move.q  r2, #0x22               ; '"' -> skip the string literal
    bne     r1, r2, .tr_skip_adv
    add.q   r20, r20, #1
.tr_skip_str:
    load.b  r1, (r20)
    beqz    r1, .tr_advance
    move.q  r2, #0x22
    beq     r1, r2, .tr_skip_str_end
    add.q   r20, r20, #1
    bra     .tr_skip_str
.tr_skip_str_end:
    add.q   r20, r20, #1
    bra     .tr_skip_stmt
.tr_skip_adv:
    add.q   r20, r20, #1
    bra     .tr_skip_stmt
.tr_unsup:
    move.q  r8, r0                  ; unsupported statement
    rts
.tr_oflow:
    move.q  r8, #2                  ; output buffer overflow
    rts

; POKE addr,val is intentionally a byte alias, matching the 8-bit home computer
; BASIC convention; wider native stores must use explicit POKE16/32/64.
; With complete integer
; literal operands the fast path emits the store directly. POKE64 materialises
; the value from low/high 32-bit immediates because the private decimal emitter
; and move.l immediates are intentionally 32-bit.
; With expression operands (variables/arithmetic), including arithmetic that starts
; with a numeric literal, the arena delegates the whole statement to the resident
; statement handler, keeping RUN AOT's "every tokenised statement runs" contract. R25
; holds the statement start for that fallback; it survives the parse.
.tr_poke:
    move.q  r25, r20               ; save the statement start for the delegate fallback
    add.q   r20, r20, #1           ; consume TK_POKE
    la      r24, aot_str_store_b
    move.q  r26, r0
    bra     .tr_store2

.tr_ext:
    move.q  r25, r20               ; save the statement start
    add.q   r20, r20, #1           ; consume TK_EXT
    load.b  r1, (r20)              ; subtoken
    move.q  r2, #0x03               ; EXT_POKE8
    beq     r1, r2, .tr_ext_poke8
    move.q  r2, #0x04               ; EXT_POKE16
    beq     r1, r2, .tr_ext_poke16
    move.q  r2, #0x05               ; EXT_POKE32
    beq     r1, r2, .tr_ext_poke32
    move.q  r2, #0x06               ; EXT_POKE64
    beq     r1, r2, .tr_ext_poke64
    sub.q   r20, r20, #1           ; rewind to TK_EXT for generic delegation
    load.b  r1, (r20)
    bra     .tr_delegate_token
.tr_ext_poke8:
    add.q   r20, r20, #1           ; consume subtoken
    la      r24, aot_str_store_b
    move.q  r26, r0
    bra     .tr_store2
.tr_ext_poke16:
    add.q   r20, r20, #1
    la      r24, aot_str_store_w
    move.q  r26, r0
    bra     .tr_store2
.tr_ext_poke32:
    add.q   r20, r20, #1
    la      r24, aot_str_store_l
    move.q  r26, r0
    bra     .tr_store2
.tr_ext_poke64:
    add.q   r20, r20, #1
    la      r24, aot_str_store_q
    move.q  r26, #1
.tr_store2:
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    jsr     aot_parse_two_uints     ; R22=addr, R23=val (R24-R26 survive)
    beqz    r8, .tr_poke_expr       ; non-literal operands -> delegate (arena)
    jsr     aot_at_stmt_boundary
    beqz    r8, .tr_poke_expr       ; numeric-prefix expression -> delegate
    move.q  r8, r17
    la      r9, aot_str_movel_r1
    move.q  r10, r22
    jsr     aot_emit_movel
    bnez    r26, .tr_store2_val64
    la      r9, aot_str_movel_r2
    move.q  r10, r23
    jsr     aot_emit_movel
    bra     .tr_store2_store
.tr_store2_val64:
    move.q  r10, r23               ; low 32
    lsl.q   r10, r10, #32
    lsr.q   r10, r10, #32
    la      r9, aot_str_movel_r2
    jsr     aot_emit_movel
    move.q  r10, r23               ; high 32
    lsr.q   r10, r10, #32
    beqz    r10, .tr_store2_store
    la      r9, aot_str_movel_r3
    jsr     aot_emit_movel
    la      r9, aot_str_lslq_r3_32
    jsr     aot_emit_str
    la      r9, aot_str_orq_r2_r3
    jsr     aot_emit_str
.tr_store2_store:
    move.q  r9, r24
    jsr     aot_emit_str
    move.q  r17, r8
    bra     .tr_stmt
.tr_poke_expr:
    ; Expression operands: standalone has no resident handler to delegate to, so it
    ; reports the statement unsupported; the arena delegates to the resident handler.
    bnez    r21, .tr_unsup
    move.q  r20, r25                ; rewind to the POKE/TK_EXT token
    load.b  r1, (r20)               ; token byte (drives the jump-table index)
    bra     .tr_delegate_token

; CALL addr -> move.l r1, #addr / jsr (r1)
.tr_call:
    add.q   r20, r20, #1
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    jsr     aot_parse_one_uint      ; R22=addr
    beqz    r8, .tr_unsup
    move.q  r8, r17
    la      r9, aot_str_movel_r1
    move.q  r10, r22
    jsr     aot_emit_movel
    la      r9, aot_str_jsr_r1
    jsr     aot_emit_str
    move.q  r17, r8
    bra     .tr_stmt

; BITSET/BITCLR addr, bit -> read-modify-write a byte.
;   move.l r1, #addr / load.b r2, (r1) / or.l|and.l r2, r2, #mask / store.b r2, (r1)
.tr_bitset:
    add.q   r20, r20, #1
    move.q  r26, #1                 ; 1 = set
    bra     .tr_bit
.tr_bitclr:
    add.q   r20, r20, #1
    move.q  r26, r0                 ; 0 = clear
.tr_bit:
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    jsr     aot_parse_two_uints     ; R22=addr, R23=bit
    beqz    r8, .tr_unsup
    move.q  r25, #1
    lsl.q   r25, r25, r23           ; mask = 1 << bit
    move.q  r8, r17
    la      r9, aot_str_movel_r1
    move.q  r10, r22
    jsr     aot_emit_movel
    la      r9, aot_str_loadb_r2
    jsr     aot_emit_str
    bnez    r26, .tr_bit_set
    la      r9, aot_str_andl_r2     ; clear: AND with ~mask (low byte)
    move.q  r10, r25
    not.q   r10, r10
    and.q   r10, r10, #0xFF
    bra     .tr_bit_mask
.tr_bit_set:
    la      r9, aot_str_orl_r2      ; set: OR with mask
    move.q  r10, r25
.tr_bit_mask:
    jsr     aot_emit_str            ; op prefix
    move.q  r9, r10
    jsr     aot_emit_uint           ; mask value
    la      r9, aot_str_nl
    jsr     aot_emit_str
    la      r9, aot_str_store_b
    jsr     aot_emit_str
    move.q  r17, r8
    bra     .tr_stmt
; GOTO line -> bra L<line>; GOSUB line -> jsr L<line>; RETURN -> rts.
; Every BASIC line is labelled L<lineNo> at .tr_line, so these resolve through
; the private assembler's symbol table. GOSUB/RETURN use the hardware call stack.
.tr_goto:
    add.q   r20, r20, #1           ; consume TK_GOTO
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    jsr     aot_parse_one_uint      ; R22 = target line number
    beqz    r8, .tr_unsup
    move.q  r8, r17
    la      r9, aot_str_bra_l
    move.q  r10, r22
    jsr     aot_emit_movel          ; "bra L<line>\n"
    move.q  r17, r8
    bra     .tr_stmt
; GOSUB line -> hardware "jsr L<line>" (PC-relative, pushes the full 64-bit return
; PC). Arena and standalone share this; RETURN unwinds it (arena guards a missing
; frame against the saved arena SP, standalone emits a bare rts).
.tr_gosub:
    add.q   r20, r20, #1           ; consume TK_GOSUB
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    jsr     aot_parse_one_uint      ; R22 = target line number
    beqz    r8, .tr_unsup
    move.q  r8, r17
    la      r9, aot_str_jsr_l
    move.q  r10, r22
    jsr     aot_emit_movel          ; "jsr L<line>\n"
    move.q  r17, r8
    bra     .tr_stmt
; STOP: arena captures a native continuation (the full 64-bit resume address in
; AOT_CONT_PC) and unwinds to the REPL so a typed CONT can re-enter the compiled
; code; standalone halts like END (no REPL to return to). The C<id> resume label
; lands inline so the statements following STOP run when CONT jumps back.
.tr_stop:
    bnez    r21, .tr_end_tok       ; standalone: STOP halts like END (consumes there)
    add.q   r20, r20, #1           ; arena: consume TK_STOP
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    move.q  r8, r17
    jsr     aot_emit_stop
    move.q  r17, r8
    bra     .tr_stmt
.tr_return:
    add.q   r20, r20, #1           ; consume TK_RETURN
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    ; Arena mode guards against RETURN without a matching GOSUB (the interpreter
    ; raises ERR_RET_NO_GOSUB). Standalone mode emits a bare rts for now; full
    ; standalone RETURN semantics arrive with the bundled runtime.
    bnez    r21, .tr_return_bare
    move.q  r8, r17
    load.l  r9, 8(r18)             ; current line number (for the error report)
    jsr     aot_emit_return
    move.q  r17, r8
    bra     .tr_stmt
.tr_return_bare:
    move.q  r8, r17
    la      r9, aot_str_rts
    jsr     aot_emit_str
    move.q  r17, r8
    bra     .tr_stmt
; IF <expr> THEN <statements|line>. The condition is evaluated by the resident
; expr_eval (RUN AOT only); a false result branches to the per-line end label so
; the rest of the line (the THEN clause) is skipped. THEN <line> becomes a GOTO.
; ELSE is not yet supported. Nested IFs on one line share the end label (a false
; outer or inner both skip to end of line).
.tr_if:
    add.q   r20, r20, #1           ; consume TK_IF
    ; Standalone (COMPILE) lowers the condition through the bundled expr_eval in the
    ; runtime blob (the pre-scan flagged AOT_USE_RT and the bootstrap loaded it);
    ; arena (RUN AOT) uses the resident expr_eval. Both share the lowering below.
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    la      r1, AOT_IF_LABEL       ; allocate / reuse the per-line end label id
    load.q  r25, (r1)
    bnez    r25, .tr_if_id
    la      r1, AOT_LBLCNT         ; ids start at 1 (0 = "no pending label")
    load.q  r25, (r1)
    add.q   r25, r25, #1
    store.q r25, (r1)
    la      r1, AOT_IF_LABEL
    store.q r25, (r1)
.tr_if_id:
    move.q  r8, r17
    bnez    r21, .tr_if_cond_bundle ; standalone: bundle the condition tokens inline
    la      r9, aot_str_movel_r17  ; arena: move.l r17, #<resident condition tokens>
    move.q  r10, r20
    jsr     aot_emit_movel
    bra     .tr_if_cond_ready
.tr_if_cond_bundle:
    ; Standalone: the program text is absent and the blob occupies
    ; the live program-text segment, so bundle the condition tokens inline and point R17 at
    ; them (R20 preserved for the THEN seek below).
    jsr     aot_emit_cond_span
    beqz    r9, .tr_oflow
    move.q  r8, r17
.tr_if_cond_ready:
    bnez    r21, .tr_if_eval_blob  ; standalone -> blob jump-table expr_eval
    la      r9, aot_str_movel_r6   ; move.l r6, #expr_eval (resident, RUN AOT)
    move.q  r10, #expr_eval
    jsr     aot_emit_movel
    la      r9, aot_str_jsr_r6     ; jsr (r6)  -> R8 payload, R9 tag
    jsr     aot_emit_str
    bra     .tr_if_eval_done
.tr_if_eval_blob:
    jsr     aot_emit_eval_call     ; move.l r6, #RT_EXPR_EVAL / load.q / jsr (r6)
.tr_if_eval_done:
    jsr     aot_emit_truthy_call    ; tagged condition -> R8=0/1
    beqz    r9, .tr_oflow
    move.q  r17, r8
    ; skip the condition tokens to TK_THEN (quote-aware) and consume it
.tr_if_seek_then:
    load.b  r1, (r20)
    beqz    r1, .tr_stmt           ; no THEN -> let .tr_stmt end the line
    move.q  r2, #0xAC              ; TK_THEN
    beq     r1, r2, .tr_if_then
    move.q  r2, #0x22              ; '"' -> skip a string literal
    bne     r1, r2, .tr_if_seek_adv
    add.q   r20, r20, #1
.tr_if_seek_str:
    load.b  r1, (r20)
    beqz    r1, .tr_stmt
    move.q  r2, #0x22
    beq     r1, r2, .tr_if_seek_str_end
    add.q   r20, r20, #1
    bra     .tr_if_seek_str
.tr_if_seek_str_end:
    add.q   r20, r20, #1
    bra     .tr_if_seek_then
.tr_if_seek_adv:
    add.q   r20, r20, #1
    bra     .tr_if_seek_then
.tr_if_then:
    add.q   r20, r20, #1           ; consume TK_THEN
    ; Scan the THEN clause for a string-aware ELSE. R23 = 1 if an ELSE is found.
    ; R4 = 1 if a nested IF appears in the THEN clause before that ELSE: this
    ; single-level lowering cannot bind the ELSE to the inner IF (BASIC binds ELSE to
    ; the nearest IF), so that shape is rejected as unsupported at .tr_if_has_else
    ; rather than emitting mis-associated labels (which fault the private assembler).
    move.q  r23, r0
    move.q  r4, r0                 ; nested-IF-before-ELSE flag
    move.q  r3, r20                ; scan cursor (preserves R20)
.tr_if_else_scan:
    load.b  r1, (r3)
    beqz    r1, .tr_if_emit_beqz   ; end of line
    move.q  r2, #0xAB              ; TK_ELSE
    beq     r1, r2, .tr_if_has_else
    move.q  r2, #0x8B              ; TK_IF -> a nested IF inside the THEN clause
    bne     r1, r2, .tr_if_else_chkstr
    move.q  r4, #1                 ; remember it; an ELSE after this is the inner IF's
.tr_if_else_chkstr:
    move.q  r2, #0x22              ; skip string literals
    bne     r1, r2, .tr_if_else_adv
    add.q   r3, r3, #1
.tr_if_else_str:
    load.b  r1, (r3)
    beqz    r1, .tr_if_emit_beqz
    move.q  r2, #0x22
    beq     r1, r2, .tr_if_else_strend
    add.q   r3, r3, #1
    bra     .tr_if_else_str
.tr_if_else_strend:
    add.q   r3, r3, #1
    bra     .tr_if_else_scan
.tr_if_else_adv:
    add.q   r3, r3, #1
    bra     .tr_if_else_scan
.tr_if_has_else:
    bnez    r4, .tr_unsup          ; nested IF before ELSE: ELSE binds to the inner
                                   ; IF, which this single-level lowering cannot
                                   ; represent -> reject as unsupported, not broken code
    move.q  r23, #1
    move.q  r22, r3                ; runtime address of the TK_ELSE byte
.tr_if_emit_beqz:
    ; False-branch target: EL<id> (the ELSE clause) if there is an ELSE, else the
    ; line-end IE<id> (skip the THEN clause).
    beqz    r23, .tr_if_beqz_ie
    la      r1, AOT_LBLCNT         ; allocate the ELSE-clause label id
    load.q  r24, (r1)
    add.q   r24, r24, #1
    store.q r24, (r1)
    la      r1, AOT_IF_ELSE
    store.q r24, (r1)
    move.q  r8, r17
    la      r9, aot_str_beqz_el    ; beqz r8, EL<id>
    move.q  r10, r24
    jsr     aot_emit_movel
    ; Make delegated THEN-clause handlers (PRINT, ...) stop at ELSE by pointing the
    ; resident if_else_boundary at the TK_ELSE byte address.
    la      r9, aot_str_movel_r1   ; move.l r1, #if_else_boundary
    move.q  r10, #if_else_boundary
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r2   ; move.l r2, #<else addr>
    move.q  r10, r22
    jsr     aot_emit_movel
    la      r9, aot_str_store_l    ; store.l r2, (r1)
    jsr     aot_emit_str
    move.q  r17, r8
    bra     .tr_if_then_sp
.tr_if_beqz_ie:
    move.q  r8, r17
    la      r9, aot_str_beqz_ie    ; beqz r8, IE<id>
    move.q  r10, r25
    jsr     aot_emit_movel
    move.q  r17, r8
.tr_if_then_sp:
    load.b  r1, (r20)
    move.q  r2, #0x20
    bne     r1, r2, .tr_if_then_chk
    add.q   r20, r20, #1
    bra     .tr_if_then_sp
.tr_if_then_chk:
    move.q  r2, #0x30              ; digit -> THEN <line> = GOTO
    blt     r1, r2, .tr_stmt
    move.q  r2, #0x39
    bgt     r1, r2, .tr_stmt       ; THEN <statements> -> transpile inline
    ; THEN <line number> -> bra L<line>
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    jsr     aot_parse_one_uint     ; R22 = target line
    beqz    r8, .tr_unsup
    move.q  r8, r17
    la      r9, aot_str_bra_l      ; bra L<line>
    move.q  r10, r22
    jsr     aot_emit_movel
    move.q  r17, r8
    bra     .tr_stmt
; ELSE: end the THEN clause (bra to the line-end join IE<id>), then open the ELSE
; clause label EL<id>. Only valid inside an IF that recorded a pending ELSE.
.tr_else:
    add.q   r20, r20, #1           ; consume TK_ELSE
    bnez    r21, .tr_unsup
    la      r1, AOT_IF_ELSE
    load.q  r24, (r1)
    beqz    r24, .tr_unsup         ; ELSE with no matching IF
    store.q r0, (r1)               ; consume it
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    la      r1, AOT_IF_LABEL
    load.q  r25, (r1)              ; line-end (join) id
    move.q  r8, r17
    la      r9, aot_str_bra_ie     ; bra IE<id>  (THEN done -> skip ELSE)
    move.q  r10, r25
    jsr     aot_emit_movel
    la      r9, aot_str_lbl_el     ; EL<id>:  (ELSE clause start)
    move.q  r10, r24
    jsr     aot_emit_label_named
    ; Clear if_else_boundary so ELSE-clause / later statements run to end of line.
    la      r9, aot_str_movel_r1   ; move.l r1, #if_else_boundary
    move.q  r10, #if_else_boundary
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r2   ; move.l r2, #0
    move.q  r10, #0
    jsr     aot_emit_movel
    la      r9, aot_str_store_l    ; store.l r2, (r1)
    jsr     aot_emit_str
    move.q  r17, r8
    bra     .tr_stmt
; ON <expr> GOTO|GOSUB l1,l2,...: evaluate the 1-based index, then for target i
; compare and branch to L<li> (GOTO) or jsr L<li> then continue (GOSUB). An index
; out of range falls through. RUN AOT only (expr_eval is resident).
.tr_on:
    add.q   r20, r20, #1           ; consume TK_ON
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    move.q  r8, r17
    jsr     aot_emit_expr          ; index expr -> R8 (resident or blob expr_eval)
    beqz    r9, .tr_oflow
    jsr     aot_emit_expr_to_i64
    move.q  r17, r8
.on_seek:
    load.b  r1, (r20)
    beqz    r1, .tr_unsup          ; no GOTO/GOSUB after the expression
    move.q  r2, #0x89              ; TK_GOTO
    beq     r1, r2, .on_goto_mode
    move.q  r2, #0x8D              ; TK_GOSUB
    beq     r1, r2, .on_gosub_mode
    add.q   r20, r20, #1
    bra     .on_seek
.on_goto_mode:
    add.q   r20, r20, #1
    move.q  r26, r0                ; mode 0 = GOTO
    bra     .on_targets
.on_gosub_mode:
    add.q   r20, r20, #1
    move.q  r26, #1                ; mode 1 = GOSUB
    la      r1, AOT_LBLCNT         ; shared done label id
    load.q  r28, (r1)
    add.q   r28, r28, #1
    store.q r28, (r1)
.on_targets:
    move.q  r24, #1                ; 1-based index counter
.on_one:
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    load.b  r1, (r20)
    move.q  r2, #0x20
    bne     r1, r2, .on_chk
    add.q   r20, r20, #1
    bra     .on_one
.on_chk:
    move.q  r2, #0x30
    blt     r1, r2, .on_done
    move.q  r2, #0x39
    bgt     r1, r2, .on_done
    jsr     aot_parse_one_uint     ; R22 = target line
    beqz    r8, .tr_unsup
    move.q  r8, r17
    la      r9, aot_str_moveq_r1   ; move.q r1, #<index>
    move.q  r10, r24
    jsr     aot_emit_movel
    bnez    r26, .on_emit_gosub
    la      r9, aot_str_beq_r1_l   ; beq r8, r1, L<line>
    move.q  r10, r22
    jsr     aot_emit_movel
    move.q  r17, r8
    bra     .on_next
.on_emit_gosub:
    la      r1, AOT_LBLCNT         ; per-target skip label id
    load.q  r25, (r1)
    add.q   r25, r25, #1
    store.q r25, (r1)
    la      r9, aot_str_bne_r1_s   ; bne r8, r1, S<sid>
    move.q  r10, r25
    jsr     aot_emit_movel
    la      r9, aot_str_jsr_l      ; jsr L<line>  (hardware GOSUB, PC-relative)
    move.q  r10, r22
    jsr     aot_emit_movel
    la      r9, aot_str_bra_d      ; bra D<did>
    move.q  r10, r28
    jsr     aot_emit_movel
    la      r9, aot_str_lbl_s      ; S<sid>:
    move.q  r10, r25
    jsr     aot_emit_label_named
    move.q  r17, r8
.on_next:
    add.q   r24, r24, #1
.on_comma_sp:
    load.b  r1, (r20)
    move.q  r2, #0x20
    bne     r1, r2, .on_comma_chk
    add.q   r20, r20, #1
    bra     .on_comma_sp
.on_comma_chk:
    move.q  r2, #0x2C              ; ','
    bne     r1, r2, .on_done
    add.q   r20, r20, #1
    bra     .on_one
.on_done:
    beqz    r26, .tr_stmt          ; GOTO: out of range -> fall through
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    move.q  r8, r17
    la      r9, aot_str_lbl_d      ; D<did>:  (GOSUB join point)
    move.q  r10, r28
    jsr     aot_emit_label_named
    move.q  r17, r8
    bra     .tr_stmt
; FOR <header>: delegate to resident exec_do_for (sets up the loop frame), then
; mark the loop top F<id>. NEXT delegates to exec_do_next (increment + compare)
; and branches back to F<id> when it reports "loop" (R28 == 1). RUN AOT only.
.tr_for:
    add.q   r20, r20, #1           ; consume TK_FOR
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    la      r1, AOT_LBLCNT         ; loop-label id (ids start at 1)
    load.q  r25, (r1)
    add.q   r25, r25, #1
    store.q r25, (r1)
    la      r1, AOT_FOR_SP         ; push onto the compile-time FOR stack
    load.q  r2, (r1)
    move.q  r3, #8
    bge     r2, r3, .tr_unsup      ; > 8 nested FORs not supported
    la      r3, AOT_FOR_STACK
    lsl.q   r4, r2, #3
    add.q   r3, r3, r4
    store.q r25, (r3)
    add.q   r2, r2, #1
    store.q r2, (r1)
    move.q  r8, r17
    bnez    r21, .tr_for_blob
    la      r9, aot_str_movel_r17  ; arena: move.l r17, #<resident FOR header>
    move.q  r10, r20
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r6   ; move.l r6, #exec_do_for
    move.q  r10, #exec_do_for
    jsr     aot_emit_movel
    la      r9, aot_str_jsr_r6     ; jsr (r6)
    jsr     aot_emit_str
    bra     .tr_for_called
.tr_for_blob:
    move.q  r17, r8                ; sync cursor for the span bundler
    jsr     aot_emit_cond_span     ; bundle the FOR header -> move.l r17, #Q<id>
    beqz    r9, .tr_oflow
    move.q  r8, r17
    move.q  r10, #RT_EXEC_DO_FOR   ; call exec_do_for (checked: halt on error)
    jsr     aot_emit_rt_call
.tr_for_called:
    la      r9, aot_str_lbl_f      ; F<id>:  (loop top)
    move.q  r10, r25
    jsr     aot_emit_label_named
    move.q  r17, r8
    bra     .tr_skip_stmt
.tr_next:
    add.q   r20, r20, #1           ; consume TK_NEXT
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    la      r1, AOT_FOR_SP         ; pop the matching FOR (innermost)
    load.q  r2, (r1)
    beqz    r2, .tr_unsup          ; NEXT without FOR
    sub.q   r2, r2, #1
    store.q r2, (r1)
    la      r3, AOT_FOR_STACK
    lsl.q   r4, r2, #3
    add.q   r3, r3, r4
    load.q  r25, (r3)              ; loop-label id
    move.q  r8, r17
    ; save ST_FOR_SP (exec_do_next pops the FOR frame only when the loop ends)
    la      r9, aot_str_addq_r1_gosub
    move.q  r10, #ST_FOR_SP
    jsr     aot_emit_movel
    la      r9, aot_str_load_r1_r1
    jsr     aot_emit_str
    la      r9, aot_str_movel_r2
    move.q  r10, #AOT_FOR_SP_SAVE
    jsr     aot_emit_movel
    la      r9, aot_str_store_r1_r2
    jsr     aot_emit_str
    ; jsr exec_do_next  (increment + compare; FP arithmetic with parity)
    bnez    r21, .tr_next_blob
    la      r9, aot_str_movel_r17
    move.q  r10, r20
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r6
    move.q  r10, #exec_do_next
    jsr     aot_emit_movel
    la      r9, aot_str_jsr_r6
    jsr     aot_emit_str
    bra     .tr_next_called
.tr_next_blob:
    move.q  r17, r8                ; sync cursor for the span bundler
    jsr     aot_emit_cond_span     ; bundle the NEXT variable (if any) -> R17
    beqz    r9, .tr_oflow
    move.q  r8, r17
    move.q  r10, #RT_EXEC_DO_NEXT  ; call exec_do_next (checked: halt on error)
    jsr     aot_emit_rt_call
.tr_next_called:
    ; reload ST_FOR_SP; if unchanged the frame is still active -> loop to F<id>
    la      r9, aot_str_movel_r2
    move.q  r10, #AOT_FOR_SP_SAVE
    jsr     aot_emit_movel
    la      r9, aot_str_load_r2_r2
    jsr     aot_emit_str
    la      r9, aot_str_addq_r1_gosub
    move.q  r10, #ST_FOR_SP
    jsr     aot_emit_movel
    la      r9, aot_str_load_r1_r1
    jsr     aot_emit_str
    la      r9, aot_str_beq_r1_r2_f  ; beq r1, r2, F<id>  (sp unchanged -> loop)
    move.q  r10, r25
    jsr     aot_emit_movel
    move.q  r17, r8
    bra     .tr_skip_stmt
; WHILE <expr> ... WEND: native loop. WHILE evaluates the condition at the top
; (false skips past WEND); WEND branches back to re-check. RUN AOT only.
.tr_while:
    add.q   r20, r20, #1           ; consume TK_WHILE
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    la      r1, AOT_LBLCNT         ; loop-label id
    load.q  r25, (r1)
    add.q   r25, r25, #1
    store.q r25, (r1)
    la      r1, AOT_WHILE_SP       ; push
    load.q  r2, (r1)
    move.q  r3, #8
    bge     r2, r3, .tr_unsup
    la      r3, AOT_WHILE_STACK
    lsl.q   r4, r2, #3
    add.q   r3, r3, r4
    store.q r25, (r3)
    add.q   r2, r2, #1
    store.q r2, (r1)
    move.q  r8, r17
    la      r9, aot_str_lbl_w      ; W<id>:  (loop top - condition re-checked)
    move.q  r10, r25
    jsr     aot_emit_label_named
    jsr     aot_emit_expr          ; condition -> R8 (resident or blob expr_eval)
    beqz    r9, .tr_oflow
    jsr     aot_emit_truthy_call
    beqz    r9, .tr_oflow
    la      r9, aot_str_beqz_we    ; beqz r8, WE<id>  (false -> exit loop)
    move.q  r10, r25
    jsr     aot_emit_movel
    move.q  r17, r8
    bra     .tr_skip_stmt          ; skip the condition tokens
.tr_wend:
    add.q   r20, r20, #1           ; consume TK_UNTIL (WEND)
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    la      r1, AOT_WHILE_SP       ; pop the matching WHILE
    load.q  r2, (r1)
    beqz    r2, .tr_unsup          ; WEND without WHILE
    sub.q   r2, r2, #1
    store.q r2, (r1)
    la      r3, AOT_WHILE_STACK
    lsl.q   r4, r2, #3
    add.q   r3, r3, r4
    load.q  r25, (r3)
    move.q  r8, r17
    la      r9, aot_str_bra_w      ; bra W<id>  (re-check the condition)
    move.q  r10, r25
    jsr     aot_emit_movel
    la      r9, aot_str_lbl_we     ; WE<id>:  (exit target)
    move.q  r10, r25
    jsr     aot_emit_label_named
    move.q  r17, r8
    bra     .tr_stmt
; DO ... LOOP [WHILE|UNTIL <expr>]: native loop, condition (if any) at the bottom.
; DO with a top WHILE/UNTIL is not supported (reported unsupported).
.tr_do:
    add.q   r20, r20, #1           ; consume TK_DO
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    ; reject a top condition (DO WHILE/UNTIL)
.tr_do_sp:
    load.b  r1, (r20)
    move.q  r2, #0x20
    bne     r1, r2, .tr_do_chk
    add.q   r20, r20, #1
    bra     .tr_do_sp
.tr_do_chk:
    move.q  r2, #0xB0              ; TK_WHILE
    beq     r1, r2, .tr_unsup
    move.q  r2, #0xAF              ; TK_UNTIL
    beq     r1, r2, .tr_unsup
    la      r1, AOT_LBLCNT
    load.q  r25, (r1)
    add.q   r25, r25, #1
    store.q r25, (r1)
    la      r1, AOT_DO_SP          ; push
    load.q  r2, (r1)
    move.q  r3, #8
    bge     r2, r3, .tr_unsup
    la      r3, AOT_DO_STACK
    lsl.q   r4, r2, #3
    add.q   r3, r3, r4
    store.q r25, (r3)
    add.q   r2, r2, #1
    store.q r2, (r1)
    move.q  r8, r17
    la      r9, aot_str_lbl_d      ; D<id>:  (loop top)
    move.q  r10, r25
    jsr     aot_emit_label_named
    move.q  r17, r8
    bra     .tr_stmt
.tr_loop:
    add.q   r20, r20, #1           ; consume TK_LOOP
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    la      r1, AOT_DO_SP          ; pop the matching DO
    load.q  r2, (r1)
    beqz    r2, .tr_unsup          ; LOOP without DO
    sub.q   r2, r2, #1
    store.q r2, (r1)
    la      r3, AOT_DO_STACK
    lsl.q   r4, r2, #3
    add.q   r3, r3, r4
    load.q  r24, (r3)              ; r24 = loop id
.tr_loop_sp:
    load.b  r1, (r20)
    move.q  r2, #0x20
    bne     r1, r2, .tr_loop_chk
    add.q   r20, r20, #1
    bra     .tr_loop_sp
.tr_loop_chk:
    move.q  r2, #0xB0              ; TK_WHILE -> loop while true
    beq     r1, r2, .tr_loop_while
    move.q  r2, #0xAF              ; TK_UNTIL -> loop until true
    beq     r1, r2, .tr_loop_until
    ; bare LOOP -> unconditional back branch
    move.q  r8, r17
    la      r9, aot_str_bra_d      ; bra D<id>
    move.q  r10, r24
    jsr     aot_emit_movel
    move.q  r17, r8
    bra     .tr_stmt
.tr_loop_while:
    add.q   r20, r20, #1           ; consume WHILE
    move.q  r26, r0                ; 0 = WHILE (loop if true -> bnez)
    bra     .tr_loop_cond
.tr_loop_until:
    add.q   r20, r20, #1           ; consume UNTIL
    move.q  r26, #1                ; 1 = UNTIL (loop if false -> beqz)
.tr_loop_cond:
    move.q  r8, r17
    jsr     aot_emit_expr          ; condition -> R8 (resident or blob expr_eval)
    beqz    r9, .tr_oflow
    jsr     aot_emit_truthy_call
    beqz    r9, .tr_oflow
    bnez    r26, .tr_loop_until_br
    la      r9, aot_str_bnez_d     ; LOOP WHILE: true -> bnez r8, D<id>
    bra     .tr_loop_emit
.tr_loop_until_br:
    la      r9, aot_str_beqz_d     ; LOOP UNTIL: false -> beqz r8, D<id>
.tr_loop_emit:
    move.q  r10, r24
    jsr     aot_emit_movel
    move.q  r17, r8
    bra     .tr_skip_stmt          ; skip the condition tokens
; WAIT addr, mask [, xor] -> poll [addr] until ((v EOR xor) AND mask) != 0, with
; a ~1M-iteration timeout (matching the interpreter's hw_wait). VSYNC -> the same
; loop with addr = VIDEO_STATUS, mask = STATUS_VBLANK, xor = 0.
.tr_wait:
    add.q   r20, r20, #1           ; consume TK_WAIT
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    jsr     aot_parse_two_uints     ; R22 = addr, R23 = mask
    beqz    r8, .tr_unsup
    move.q  r24, r0                ; optional xor, default 0
.tr_wait_xsp:
    load.b  r1, (r20)
    move.q  r2, #0x20
    bne     r1, r2, .tr_wait_xc
    add.q   r20, r20, #1
    bra     .tr_wait_xsp
.tr_wait_xc:
    move.q  r2, #0x2C               ; ','
    bne     r1, r2, .tr_wait_emit
    add.q   r20, r20, #1
    move.q  r8, r20
    jsr     aot_parse_uint          ; R8 = xor value
    beqz    r9, .tr_unsup
    move.q  r24, r8
    move.q  r20, r10
.tr_wait_emit:
    move.q  r8, r17
    jsr     aot_emit_wait_loop      ; R22/R23/R24 -> poll loop
    move.q  r17, r8
    bra     .tr_stmt
.tr_vsync:
    add.q   r20, r20, #1           ; consume TK_VSYNC_CMD
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    move.q  r22, #VIDEO_STATUS
    move.q  r23, #STATUS_VBLANK
    move.q  r24, r0
    move.q  r8, r17
    jsr     aot_emit_wait_loop
    move.q  r17, r8
    bra     .tr_stmt
; PRINT: in arena mode (RUN AOT) delegate to the resident exec_do_print for full
; parity (expressions, variables, strings, commas, multiple items). In standalone
; mode (COMPILE) resident handlers do not exist, so use the self-contained native
; bundled string/number lowering instead.
.tr_print:
    beqz    r21, .tr_print_arena   ; arena: delegate to the resident exec_do_print
    ; Standalone: if the runtime blob is present, delegate the whole PRINT (strings,
    ; numbers, expressions, commas, semicolons) to the bundled exec_do_print; with no
    ; blob (literal-only programme) use the self-contained native lowering below.
    la      r1, AOT_USE_RT
    load.q  r1, (r1)
    bnez    r1, .tr_print_blob
    bra     .tr_print_native
.tr_print_arena:
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    add.q   r9, r20, #1            ; R17 target = past TK_PRINT
    move.q  r10, #(stmt_jump_table + 0x78)  ; PRINT handler slot (index 0x1E * 4)
    move.q  r8, r17
    jsr     aot_emit_delegate
    move.q  r17, r8
    bra     .tr_skip_stmt
.tr_print_blob:
    add.q   r20, r20, #1           ; consume TK_PRINT -> R20 at the print list
    move.q  r8, r17
    jsr     aot_emit_cond_span     ; bundle the whole print list -> move.l r17, #Q<id>
    beqz    r9, .tr_oflow
    move.q  r8, r17
    move.q  r10, #RT_EXEC_DO_PRINT ; call exec_do_print (checked: halt on error)
    jsr     aot_emit_rt_call
    move.q  r17, r8
    bra     .tr_skip_stmt
.tr_print_native:
    add.q   r20, r20, #1           ; consume TK_PRINT
.tr_pr_sp:
    load.b  r1, (r20)
    move.q  r2, #0x20
    bne     r1, r2, .tr_pr_chk
    add.q   r20, r20, #1
    bra     .tr_pr_sp
.tr_pr_chk:
    beqz    r1, .tr_pr_crlf        ; PRINT <eol> -> CRLF only
    move.q  r2, #0x3A              ; ':'
    beq     r1, r2, .tr_pr_crlf
    move.q  r2, #0x3B              ; ';' -> print nothing, suppress CRLF
    beq     r1, r2, .tr_pr_semi_only
    ; The native PRINT path handles only literals (string/integer); it is reached only
    ; without the runtime blob. Blob-present standalone PRINT delegates to the bundled
    ; exec_do_print (.tr_print_blob), which handles variables/expressions/separators.
    move.q  r2, #0x30              ; digit -> integer literal
    blt     r1, r2, .tr_pr_str_chk
    move.q  r2, #0x39
    ble     r1, r2, .tr_pr_num
.tr_pr_str_chk:
    move.q  r2, #0x22              ; '"'
    bne     r1, r2, .tr_unsup      ; only string / integer literals supported
    add.q   r20, r20, #1           ; skip the opening quote
    la      r1, AOT_LBLCNT         ; allocate a label id for the inline string
    load.q  r25, (r1)
    add.q   r2, r25, #1
    store.q r2, (r1)
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    move.q  r8, r17
    la      r9, aot_str_bra_p      ; bra P<id>  (skip over the inline data)
    move.q  r10, r25
    jsr     aot_emit_movel
    la      r9, aot_str_lbl_q      ; Q<id>:
    move.q  r10, r25
    jsr     aot_emit_label_named
    la      r9, aot_str_dcb        ; dc.b
    jsr     aot_emit_str
    move.q  r17, r8
.tr_pr_ch:
    jsr     aot_text_room          ; room for one "NNN, "
    beqz    r8, .tr_oflow
    load.b  r1, (r20)
    beqz    r1, .tr_pr_strend      ; unterminated string
    move.q  r2, #0x22
    beq     r1, r2, .tr_pr_strend
    move.q  r8, r17
    move.q  r9, r1                 ; character byte
    jsr     aot_emit_uint
    la      r9, aot_str_comma_sp
    jsr     aot_emit_str
    move.q  r17, r8
    add.q   r20, r20, #1
    bra     .tr_pr_ch
.tr_pr_strend:
    load.b  r1, (r20)              ; consume the closing quote if present
    move.q  r2, #0x22
    bne     r1, r2, .tr_pr_emit
    add.q   r20, r20, #1
.tr_pr_emit:
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    move.q  r8, r17
    la      r9, aot_str_zero_nl    ; "0\n" terminates the dc.b
    jsr     aot_emit_str
    la      r9, aot_str_lbl_p      ; P<id>:
    move.q  r10, r25
    jsr     aot_emit_label_named
    la      r9, aot_str_la_r8_q    ; la r8, Q<id>
    move.q  r10, r25
    jsr     aot_emit_movel
    la      r9, aot_str_jsr_print  ; jsr aot_print_str
    jsr     aot_emit_str
    move.q  r17, r8
    la      r1, AOT_USE_PRINT      ; request the bundled helper
    move.q  r2, #1
    store.q r2, (r1)
    bra     .tr_pr_semi_sp
; PRINT <integer literal>: load the int, convert to FP64, call bundled fp_print.
.tr_pr_num:
    move.q  r8, r20
    jsr     aot_parse_uint         ; R8 = value, R10 = past
    beqz    r9, .tr_unsup
    move.q  r24, r8                ; save value
    move.q  r20, r10
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    move.q  r8, r17
    la      r9, aot_str_movel_r8   ; move.l r8, #<value>
    move.q  r10, r24
    jsr     aot_emit_movel
    la      r9, aot_str_fp_seq     ; jsr fp_float / jsr fp_print
    jsr     aot_emit_str
    move.q  r17, r8
    la      r1, AOT_USE_FP         ; request the bundled fp_print closure
    move.q  r2, #1
    store.q r2, (r1)
.tr_pr_semi_sp:
    load.b  r1, (r20)
    move.q  r2, #0x20
    bne     r1, r2, .tr_pr_semi_chk
    add.q   r20, r20, #1
    bra     .tr_pr_semi_sp
.tr_pr_semi_chk:
    move.q  r2, #0x3B              ; ';'
    bne     r1, r2, .tr_pr_crlf
    add.q   r20, r20, #1           ; consume ';' -> suppress CRLF
    bra     .tr_stmt
.tr_pr_semi_only:
    add.q   r20, r20, #1           ; bare ';' after PRINT: no output, no CRLF
    bra     .tr_stmt
.tr_pr_crlf:
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    move.q  r8, r17
    la      r9, aot_str_crlf
    jsr     aot_emit_str
    move.q  r17, r8
    bra     .tr_stmt
.tr_sep:
    add.q   r20, r20, #1
    bra     .tr_stmt
.tr_end_tok:
    add.q   r20, r20, #1           ; consume END/STOP
    ; Emit an explicit terminator so execution stops here instead of falling
    ; through into a later line (matters once GOTO/GOSUB can jump around).
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    move.q  r8, r17
    jsr     aot_emit_terminator
    move.q  r17, r8
.tr_end_skip:
    load.b  r1, (r20)
    beqz    r1, .tr_advance
    move.q  r2, #0x3A
    beq     r1, r2, .tr_stmt        ; let .tr_stmt re-handle the ':'
    add.q   r20, r20, #1
    bra     .tr_end_skip
.tr_advance:
    ; Emit the pending IF end label (skip target) before leaving the line.
    la      r1, AOT_IF_LABEL
    load.q  r25, (r1)
    beqz    r25, .tr_advance_next
    jsr     aot_text_room
    beqz    r8, .tr_oflow
    move.q  r8, r17
    la      r9, aot_str_lbl_ie     ; IE<id>:
    move.q  r10, r25
    jsr     aot_emit_label_named
    ; Both IF arms join here; clear any if_else_boundary the THEN clause set (the
    ; THEN path skips the .tr_else reset via "bra IE").
    la      r9, aot_str_movel_r1   ; move.l r1, #if_else_boundary
    move.q  r10, #if_else_boundary
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r2   ; move.l r2, #0
    move.q  r10, #0
    jsr     aot_emit_movel
    la      r9, aot_str_store_l    ; store.l r2, (r1)
    jsr     aot_emit_str
    move.q  r17, r8
    la      r1, AOT_IF_LABEL
    store.q r0, (r1)
.tr_advance_next:
    load.q  r18, (r18)             ; follow the next-line pointer
    bra     .tr_line
.tr_finish:
    jsr     aot_text_room          ; room for the trailing rts/halt
    beqz    r8, .tr_oflow
    move.q  r8, r17
    jsr     aot_emit_terminator    ; rts (arena) / halt (standalone)
    move.q  r17, r8
    sub.q   r9, r17, r19           ; emitted length (excludes the terminator)
    ; Null-terminate the buffer so aot_asm_program stops at end-of-text instead
    ; of running into stale bytes. The terminator is past the reported length,
    ; so file writes (.asm) still emit only the text.
    store.b r0, (r17)
    move.q  r8, #1
    rts

aot_str_rts:
    dc.b    "rts", 0x0A, 0
    align 4
aot_str_halt:
    dc.b    "halt", 0x0A, 0
    align 4
aot_str_movel_r1:
    dc.b    "move.l r1, #", 0
    align 4
aot_str_movel_r1_rtpay:
    dc.b    "move.l r1, #__rtpay", 0x0A, 0
    align 4
aot_str_rt_copy:
    dc.b    "RTC:", 0x0A
    dc.b    "beqz r3, RTD", 0x0A
    dc.b    "load.b r4, (r1)", 0x0A
    dc.b    "store.b r4, (r2)", 0x0A
    dc.b    "add.q r1, r1, #1", 0x0A
    dc.b    "add.q r2, r2, #1", 0x0A
    dc.b    "sub.q r3, r3, #1", 0x0A
    dc.b    "bra RTC", 0x0A
    dc.b    "RTD:", 0x0A, 0
    align 4
aot_str_rt_prog_copy:
    dc.b    "PRC:", 0x0A
    dc.b    "beqz r3, PRD", 0x0A
    dc.b    "load.b r4, (r1)", 0x0A
    dc.b    "store.b r4, (r2)", 0x0A
    dc.b    "add.q r1, r1, #1", 0x0A
    dc.b    "add.q r2, r2, #1", 0x0A
    dc.b    "sub.q r3, r3, #1", 0x0A
    dc.b    "bra PRC", 0x0A
    dc.b    "PRD:", 0x0A, 0
    align 4
aot_str_rtpay_name:
    dc.b    "__rtpay"
    align 4
aot_str_rtprog_name:
    dc.b    "__rtprog"
    align 4
aot_str_movel_r1_rtprog:
    dc.b    "move.l r1, #__rtprog", 0x0A, 0
    align 4
aot_str_movel_r2:
    dc.b    "move.l r2, #", 0
    align 4
aot_str_movel_r3:
    dc.b    "move.l r3, #", 0
    align 4
aot_str_lslq_r3_32:
    dc.b    "lsl.q r3, r3, #32", 0x0A, 0
    align 4
aot_str_orq_r2_r3:
    dc.b    "or.q r2, r2, r3", 0x0A, 0
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
aot_str_fp_seq:
    dc.b    "jsr fp_float", 0x0A, "jsr fp_print", 0x0A, 0
    align 4
aot_str_fp_print_name:
    dc.b    "fp_print"
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
aot_str_lbl_e:
    dc.b    "E", 0
    align 4
aot_str_store_r2_r1:
    dc.b    "store.l r2, (r1)", 0x0A, 0
    align 4
aot_str_movel_r2_rtname:
    dc.b    "move.l r2, #__rtname", 0x0A, 0
    align 4
aot_str_rtname_data:
    dc.b    "__rtname:", 0x0A, "    dc.b ", 0x22, "aot_runtime_blob.bin", 0x22, ", 0", 0x0A, 0
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
; FOR/NEXT: delegate setup/increment to resident exec_do_for/exec_do_next; the
; loop-back branch is native (F<id> is the loop top; NEXT branches back if R28==1).
aot_str_lbl_f:
    dc.b    "F", 0
    align 4
aot_str_addq_r1_gosub:
    dc.b    "add.q r1, r16, #", 0
    align 4
aot_str_load_r1_r1:
    dc.b    "load.l r1, (r1)", 0x0A, 0
    align 4
aot_str_store_r1_r2:
    dc.b    "store.l r1, (r2)", 0x0A, 0
    align 4
aot_str_load_r2_r2:
    dc.b    "load.l r2, (r2)", 0x0A, 0
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

; aot_rt_print_src - source text of the bundled string-print helper. The compiler
; appends this after the transpiled programme (past its terminator, so it is only
; reached via the jsr) when AOT_USE_PRINT is set, then the private assembler
; assembles it at the link base so PRINT works in both RUN AOT and standalone
; COMPILE output without depending on resident interpreter addresses.
aot_rt_print_src:
    dc.b    "aot_print_str:", 0x0A
    dc.b    ".aps_loop:", 0x0A
    dc.b    "load.b r1, (r8)", 0x0A
    dc.b    "beqz r1, .aps_done", 0x0A
    dc.b    "store.b r1, (r26)", 0x0A
    dc.b    "add.q r8, r8, #1", 0x0A
    dc.b    "bra .aps_loop", 0x0A
    dc.b    ".aps_done:", 0x0A
    dc.b    "rts", 0x0A, 0
    align 4

; aot_append_helpers - append any required bundled helper source to the
; transpiled text. Input R8 = text buffer, R9 = current text length (the buffer
; is null-terminated at R8+R9). Output R8 = new length. Clobbers R1-R4.
aot_append_helpers:
    la      r1, AOT_USE_PRINT
    load.q  r1, (r1)
    beqz    r1, .ah_none
    add.q   r2, r8, r9             ; destination = end of the current text
    la      r3, aot_rt_print_src
.ah_copy:
    load.b  r4, (r3)
    beqz    r4, .ah_term
    store.b r4, (r2)
    add.q   r2, r2, #1
    add.q   r3, r3, #1
    bra     .ah_copy
.ah_term:
    store.b r0, (r2)               ; re-null-terminate
    sub.q   r8, r2, r8             ; new length
    rts
.ah_none:
    move.q  r8, r9
    rts

; aot_bundle_fp - copy the resident fp_print closure (fp_neg..fp_print_to_buf,
; position-independent: no la, all branches PC-relative) to the start of the code
; buffer at R8, so it runs at the link base in both RUN AOT and standalone output.
; Records the bundled fp_print address for symbol injection. Input R8 = code
; buffer base. Output R8 = closure byte length (where the user code starts).
; Clobbers R1-R6.
aot_bundle_fp:
    move.q  r6, r8                  ; dest base
    move.q  r5, #fp_neg            ; resident source start
    move.q  r3, #(fp_print_to_buf - fp_neg)  ; closure length
    move.q  r4, r6                 ; dest cursor
.bf_copy:
    beqz    r3, .bf_done
    load.b  r1, (r5)
    store.b r1, (r4)
    add.q   r4, r4, #1
    add.q   r5, r5, #1
    sub.q   r3, r3, #1
    bra     .bf_copy
.bf_done:
    move.q  r1, #(fp_print - fp_neg)
    add.q   r1, r1, r6             ; bundled fp_print address = base + offset
    la      r2, AOT_HELPER_FP_ADDR
    store.q r1, (r2)
    la      r2, AOT_HELPER_FP
    move.q  r1, #1
    store.q r1, (r2)               ; request fp_print symbol injection
    move.q  r8, #(fp_print_to_buf - fp_neg)
    rts

; aot_emit_terminator - emit the end-of-execution sequence for END/STOP and the
; fall-off-the-end safety net. Standalone (R21 != 0) halts. Arena (R21 == 0)
; restores the entry stack pointer saved by the prologue, then rts, so an END or
; STOP reached inside a compiled GOSUB unwinds the whole call stack instead of
; returning to the caller. R8 = cursor, returns R8 advanced. Clobbers R1-R7, R9.
aot_emit_terminator:
    beqz    r21, .et_arena
    la      r9, aot_str_halt           ; standalone image halts at the end
    jsr     aot_emit_str
    rts
.et_arena:
    la      r9, aot_str_movel_r6       ; move.l r6, #AOT_SAVED_SP
    move.q  r10, #AOT_SAVED_SP
    jsr     aot_emit_movel
    la      r9, aot_str_load_sp        ; load.q r31, (r6)  -> restore entry SP
    jsr     aot_emit_str
    la      r9, aot_str_rts            ; rts -> return to the REPL trampoline
    jsr     aot_emit_str
    rts

; aot_emit_label_def - emit "L<lineNo>:\n" as a label definition line. In a standalone
; image that uses the runtime blob, it also emits a store of the line number into
; state[ST_CURRENT_LINE], so a runtime error (the bundled raise_error reads that field)
; reports the offending line exactly as the interpreter does.
; R8 = cursor, R9 = line number. Returns R8 advanced. Clobbers R1-R7, R9, R10, R12.
aot_emit_label_def:
    move.q  r10, r9                 ; save line number
    la      r9, aot_str_label_l
    jsr     aot_emit_str           ; "L"
    move.q  r9, r10
    jsr     aot_emit_uint          ; line number
    move.q  r1, #0x3A              ; ':'
    store.b r1, (r8)
    add.q   r8, r8, #1
    la      r9, aot_str_nl
    jsr     aot_emit_str           ; "\n"
    beqz    r21, .ald_ret          ; arena (RUN AOT): the resident loop tracks the line
    la      r1, AOT_USE_RT
    load.q  r1, (r1)
    beqz    r1, .ald_ret           ; no runtime blob -> ST_CURRENT_LINE is never read
    move.q  r12, r10               ; preserve the line number across the movel emits
    la      r9, aot_str_movel_r1   ; move.l r1, #(BASIC_STATE + ST_CURRENT_LINE)
    move.q  r10, #(BASIC_STATE + ST_CURRENT_LINE)
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r2   ; move.l r2, #<lineNo>
    move.q  r10, r12
    jsr     aot_emit_movel
    la      r9, aot_str_store_r2_r1 ; store.l r2, (r1) -> ST_CURRENT_LINE
    jsr     aot_emit_str
.ald_ret:
    rts

; aot_emit_movel - emit "<prefix>#value\n". R8=dest, R9=prefix, R10=value.
; Output R8 advanced. Clobbers R1-R7, R9.
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
aot_emit_delegate:
    move.q  r12, r9                ; R17 target
    move.q  r13, r10               ; jump-table slot
    la      r9, aot_str_movel_r17
    move.q  r10, r12
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r9
    move.q  r10, r13
    jsr     aot_emit_movel
    la      r9, aot_str_load_jsr_r9
    jsr     aot_emit_str
    rts

; aot_emit_label_named - emit "<prefix><id>:\n" as a label definition line.
; R8 = cursor, R9 = prefix string, R10 = numeric id. Returns R8 advanced.
; Clobbers R1-R7, R9, R10, R12.
aot_emit_label_named:
    move.q  r12, r10               ; save id (emit_str/emit_uint clobber R1-R7,R9)
    jsr     aot_emit_str           ; prefix
    move.q  r9, r12
    jsr     aot_emit_uint          ; id
    move.q  r1, #0x3A              ; ':'
    store.b r1, (r8)
    add.q   r8, r8, #1
    la      r9, aot_str_nl
    jsr     aot_emit_str           ; "\n"
    rts

; aot_emit_stop - emit an arena STOP. Captures the full 64-bit resume address into
; AOT_CONT_PC (so a typed CONT can re-enter the compiled code) and unwinds to the
; REPL trampoline like END. It must NOT use "la C<id>" to materialise the resume
; address: "la" is a 32-bit lea and would truncate an arena that loads above 4 GiB.
; Instead a "jsr V<id>" into a tiny helper pushes PC+8 (the resume label C<id>); the
; helper pops that full 64-bit address into r5 and stores it. Variables/DATA persist
; in BASIC_STATE; open GOSUB frames live on the hardware stack and are unwound here,
; so a CONT after a STOP that was inside a GOSUB cannot resume that call (top-level
; STOP/CONT is the supported case). RUN AOT only.
; R8 = cursor. Returns R8 advanced. Clobbers R1-R7, R9, R10, R25.
aot_emit_stop:
    la      r1, AOT_LBLCNT         ; fresh id (shared by the V/P/C labels)
    load.q  r25, (r1)
    add.q   r2, r25, #1
    store.q r2, (r1)
    la      r9, aot_str_bra_p      ; bra P<id>  (normal flow skips the helper)
    move.q  r10, r25
    jsr     aot_emit_movel
    la      r9, aot_str_lbl_v      ; V<id>:  (capture helper)
    move.q  r10, r25
    jsr     aot_emit_label_named
    la      r9, aot_str_pop_r5     ; pop r5  -> r5 = C<id> (full 64-bit resume PC)
    jsr     aot_emit_str
    la      r9, aot_str_movel_r6   ; move.l r6, #AOT_CONT_PC
    move.q  r10, #AOT_CONT_PC
    jsr     aot_emit_movel
    la      r9, aot_str_store_r5   ; store.q r5, (r6)  -> AOT_CONT_PC = resume
    jsr     aot_emit_str
    la      r9, aot_str_movel_r6   ; move.l r6, #AOT_SAVED_SP
    move.q  r10, #AOT_SAVED_SP
    jsr     aot_emit_movel
    la      r9, aot_str_load_sp    ; load.q r31, (r6)  -> unwind to entry SP
    jsr     aot_emit_str
    la      r9, aot_str_rts        ; rts -> return to the REPL trampoline
    jsr     aot_emit_str
    la      r9, aot_str_lbl_p      ; P<id>:  (normal flow lands here)
    move.q  r10, r25
    jsr     aot_emit_label_named
    la      r9, aot_str_jsr_v      ; jsr V<id>  (push C<id>, run the capture helper)
    move.q  r10, r25
    jsr     aot_emit_movel
    la      r9, aot_str_lbl_c      ; C<id>:  (CONT resume point)
    move.q  r10, r25
    jsr     aot_emit_label_named
    rts

; aot_emit_return - emit an arena RETURN using the hardware stack. A compiled GOSUB
; is "jsr L<line>" (hardware), which pushes the full 64-bit return PC and drops the
; stack pointer below the entry SP saved by the prologue (AOT_SAVED_SP). If SP is
; still at or above the entry SP there is no matching GOSUB frame: record the line in
; ST_CURRENT_LINE, set AOT_RT_ERR to ERR_RET_NO_GOSUB, and unwind to the trampoline,
; where the REPL prints "?RETURN WITHOUT GOSUB ERROR IN <line>" like exec_do_return.
; Otherwise emit "rts", which returns to the caller at the full 64-bit address - so
; this works regardless of where the arena loads (it can sit above 4 GiB).
; R8 = cursor, R9 = line number. Clobbers R1-R12, R24, R25.
aot_emit_return:
    move.q  r24, r9                ; save line number (survives the emit calls)
    la      r1, AOT_LBLCNT
    load.q  r25, (r1)              ; fresh label id
    add.q   r2, r25, #1
    store.q r2, (r1)
    la      r9, aot_str_movel_r6   ; move.l r6, #AOT_SAVED_SP
    move.q  r10, #AOT_SAVED_SP
    jsr     aot_emit_movel
    la      r9, aot_str_load_r6    ; load.q r6, (r6)  -> r6 = entry SP
    jsr     aot_emit_str
    la      r9, aot_str_blt_g      ; blt r31, r6, G<id>  (frame present -> return)
    move.q  r10, r25
    jsr     aot_emit_movel
    ; No matching GOSUB frame: set ST_CURRENT_LINE so raise_error reports it.
    la      r9, aot_str_addq_r6_r16 ; add.q r6, r16, #ST_CURRENT_LINE
    move.q  r10, #ST_CURRENT_LINE
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r5   ; move.l r5, #<line>
    move.q  r10, r24
    jsr     aot_emit_movel
    la      r9, aot_str_store_r5_l ; store.l r5, (r6)
    jsr     aot_emit_str
    la      r9, aot_str_movel_r6   ; move.l r6, #AOT_RT_ERR
    move.q  r10, #AOT_RT_ERR
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r5   ; move.l r5, #ERR_RET_NO_GOSUB
    move.q  r10, #ERR_RET_NO_GOSUB
    jsr     aot_emit_movel
    la      r9, aot_str_store_r5   ; store.q r5, (r6)
    jsr     aot_emit_str
    la      r9, aot_str_movel_r6   ; move.l r6, #AOT_SAVED_SP
    move.q  r10, #AOT_SAVED_SP
    jsr     aot_emit_movel
    la      r9, aot_str_load_sp    ; load.q r31, (r6)  -> unwind to entry SP
    jsr     aot_emit_str
    la      r9, aot_str_lbl_g      ; G<id>:
    move.q  r10, r25
    jsr     aot_emit_label_named
    la      r9, aot_str_rts        ; rts -> hardware return to the GOSUB caller
    jsr     aot_emit_str
    rts

; aot_emit_wait_loop - emit a polling loop equivalent to the interpreter's
; hw_wait: read [R22], EOR R24, AND R23, repeat until non-zero or a ~1M-iteration
; timeout expires. Each call consumes a fresh label id (AOT_LBLCNT) so multiple
; WAIT/VSYNC statements get distinct loop/done labels. R8 = cursor in/out.
; Clobbers R1-R12, R25.
aot_emit_wait_loop:
    la      r1, AOT_LBLCNT
    load.q  r25, (r1)              ; r25 = this loop's id (survives the emit calls)
    add.q   r2, r25, #1
    store.q r2, (r1)
    la      r9, aot_str_movel_r1   ; move.l r1, #addr
    move.q  r10, r22
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r3   ; move.l r3, #mask
    move.q  r10, r23
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r4   ; move.l r4, #xor
    move.q  r10, r24
    jsr     aot_emit_movel
    la      r9, aot_str_wait_to    ; move.l r5, #1048576
    jsr     aot_emit_str
    la      r9, aot_str_lbl_w      ; W<id>:
    move.q  r10, r25
    jsr     aot_emit_label_named
    la      r9, aot_str_loadl_r2   ; load.l r2, (r1)
    jsr     aot_emit_str
    la      r9, aot_str_eorl_r4    ; eor.l r2, r2, r4
    jsr     aot_emit_str
    la      r9, aot_str_andl_r3    ; and.l r2, r2, r3
    jsr     aot_emit_str
    la      r9, aot_str_bne_done   ; bne r2, r0, X<id>
    move.q  r10, r25
    jsr     aot_emit_movel
    la      r9, aot_str_subq_r5    ; sub.q r5, r5, #1
    jsr     aot_emit_str
    la      r9, aot_str_bne_top    ; bne r5, r0, W<id>
    move.q  r10, r25
    jsr     aot_emit_movel
    la      r9, aot_str_lbl_x      ; X<id>:
    move.q  r10, r25
    jsr     aot_emit_label_named
    rts

; aot_parse_two_uints - parse "<uint> , <uint>" at R20. R22, R23 = values, R20
; advanced. R8 = 1 on success, 0 on failure. Clobbers R1-R10.
aot_parse_two_uints:
    move.q  r8, r20
    jsr     aot_parse_uint
    beqz    r9, .p2u_fail
    move.q  r22, r8
    move.q  r20, r10
.p2u_sp:
    load.b  r1, (r20)
    move.q  r2, #0x20
    bne     r1, r2, .p2u_comma
    add.q   r20, r20, #1
    bra     .p2u_sp
.p2u_comma:
    move.q  r2, #0x2C               ; ','
    bne     r1, r2, .p2u_fail
    add.q   r20, r20, #1
    move.q  r8, r20
    jsr     aot_parse_uint
    beqz    r9, .p2u_fail
    move.q  r23, r8
    move.q  r20, r10
    move.q  r8, #1
    rts
.p2u_fail:
    move.q  r8, r0
    rts

; aot_at_stmt_boundary - after a literal fast-path parse, verify the cursor is at
; a real statement boundary, not at the tail of an expression such as "1+2".
; Input: R20 = cursor after parsed operands.
; Output: R8 = 1 at end / ':' / TK_ELSE after optional spaces, else 0.
;         R20 advances over spaces to the boundary/tail byte.
; Clobbers R1-R2.
aot_at_stmt_boundary:
.asb_sp:
    load.b  r1, (r20)
    move.q  r2, #0x20
    bne     r1, r2, .asb_check
    add.q   r20, r20, #1
    bra     .asb_sp
.asb_check:
    beqz    r1, .asb_ok
    move.q  r2, #0x3A               ; ':'
    beq     r1, r2, .asb_ok
    move.q  r2, #0xAB               ; TK_ELSE
    beq     r1, r2, .asb_ok
    move.q  r8, r0
    rts
.asb_ok:
    move.q  r8, #1
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
    divu.l  r4, r4, r5             ; r4 = r3 / 10
    mulu.l  r6, r4, r5             ; r6 = (r3/10) * 10
    sub.l   r7, r3, r6            ; r7 = r3 % 10
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
aot_emit_dcb:
    move.q  r13, r9                ; source cursor
    move.q  r14, r10               ; remaining bytes
    move.q  r15, r0                ; values emitted on the current line
    move.q  r11, #1
.ed_loop:
    beqz    r14, .ed_done
    ; Headroom: a value never needs more than "    dc.b " + "255, " + "\n" + null.
    la      r1, AOT_TEXT_END
    load.q  r1, (r1)
    beqz    r1, .ed_room_ok
    add.q   r2, r8, #16
    bgt     r2, r1, .ed_oflow
.ed_room_ok:
    bnez    r15, .ed_sep           ; mid-line -> separate from the previous value
    la      r9, aot_str_dcb_kw     ; start a new "    dc.b " line
    jsr     aot_emit_str
    bra     .ed_num
.ed_sep:
    move.q  r1, #0x2C              ; ','
    store.b r1, (r8)
    add.q   r8, r8, #1
    move.q  r1, #0x20              ; ' '
    store.b r1, (r8)
    add.q   r8, r8, #1
.ed_num:
    load.b  r9, (r13)              ; R9 = value for aot_emit_uint
    jsr     aot_emit_uint
    add.q   r13, r13, #1
    sub.q   r14, r14, #1
    add.q   r15, r15, #1
    move.q  r1, #16
    blt     r15, r1, .ed_loop      ; line not yet full
    move.q  r1, #0x0A              ; end the line
    store.b r1, (r8)
    add.q   r8, r8, #1
    move.q  r15, r0
    bra     .ed_loop
.ed_done:
    beqz    r15, .ed_term          ; terminate a partial final line
    move.q  r1, #0x0A
    store.b r1, (r8)
    add.q   r8, r8, #1
.ed_term:
    store.b r0, (r8)               ; keep the text null-terminated
    rts
.ed_oflow:
    move.q  r11, r0
    store.b r0, (r8)
    rts

; aot_emit_bundled_data - append the standalone bundled payloads to the
; transpiled text at R8 as labelled `dc.b` data, in the order the bytes occupy
; the image: the fp_print closure (PRINT of a number), the runtime blob, then the
; tokenised programme (READ/DATA). Each section is gated on its AOT_USE_* flag, so
; a literal-only programme emits nothing. The assembler then resolves fp_print/
; __rtpay/__rtprog as ordinary forward labels, making the .asm fully self-contained
; and eliminating COMPILE's sizing pass and address injection. The programme is
; relocated (next-pointers -> AOT_RT_PROG) into the code buffer (used as scratch
; before assembly overwrites it) prior to emission. R8 = text cursor in/out.
; AOT_RT_PROG_LEN must already hold the programme length. AOT_TEXT_END bounds it.
; Output R11 = 1 ok, 0 text overflow. Clobbers R1-R15.
aot_emit_bundled_data:
    move.q  r11, #1
    ; --- fp_print closure (split so the fp_print label lands at its entry) ---
    la      r1, AOT_USE_FP
    load.q  r1, (r1)
    beqz    r1, .bd_no_fp
    move.q  r9, #fp_neg
    move.q  r10, #(fp_print - fp_neg)
    jsr     aot_emit_dcb
    beqz    r11, .bd_done
    la      r9, aot_str_lbl_fp_print
    jsr     aot_emit_label_chk
    beqz    r11, .bd_done
    move.q  r9, #fp_print
    move.q  r10, #(fp_print_to_buf - fp_print)
    jsr     aot_emit_dcb
    beqz    r11, .bd_done
.bd_no_fp:
    ; --- runtime blob ---
    la      r1, AOT_USE_RT
    load.q  r1, (r1)
    beqz    r1, .bd_no_rt
    la      r9, aot_str_lbl_rtpay
    jsr     aot_emit_label_chk
    beqz    r11, .bd_done
    la      r1, AOT_RT_BUF
    load.q  r9, (r1)
    la      r1, AOT_RT_LEN
    load.q  r10, (r1)
    jsr     aot_emit_dcb
    beqz    r11, .bd_done
.bd_no_rt:
    ; --- tokenised programme (READ/DATA): relocate next-pointers into the code
    ;     buffer (scratch), then emit ---
    la      r1, AOT_USE_PROG
    load.q  r1, (r1)
    beqz    r1, .bd_done
    la      r1, AOT_RT_PROG_LEN
    load.q  r12, (r1)              ; programme length
    la      r1, AOT_DC_CODE
    load.q  r13, (r1)              ; scratch base (overwritten by assembly later)
    la      r1, AOT_CODE_END       ; bound the scratch copy (0 = unbounded)
    load.q  r2, (r1)
    beqz    r2, .bd_scratch_ok
    sub.q   r2, r2, r13            ; scratch capacity
    bgt     r12, r2, .bd_oflow
.bd_scratch_ok:
    move.q  r4, r13
    load.q  r5, (r16)              ; live ST_PROG_START
    move.q  r6, r12
.bd_copy:
    beqz    r6, .bd_reloc
    load.b  r1, (r5)
    store.b r1, (r4)
    add.q   r4, r4, #1
    add.q   r5, r5, #1
    sub.q   r6, r6, #1
    bra     .bd_copy
.bd_reloc:
    move.q  r6, #AOT_RT_PROG
    load.q  r5, (r16)              ; live ST_PROG_START
    sub.q   r6, r6, r5             ; relocation delta
    move.q  r4, r13                ; walk = scratch base
.bd_reloc_loop:
    load.q  r1, (r4)               ; original next-pointer (0 = end)
    beqz    r1, .bd_reloc_done
    add.q   r2, r1, r6
    store.q r2, (r4)
    load.q  r5, (r16)              ; live ST_PROG_START
    sub.q   r1, r1, r5             ; offset into the programme
    add.q   r4, r13, r1            ; scratch address of the next record
    bra     .bd_reloc_loop
.bd_reloc_done:
    la      r9, aot_str_lbl_rtprog
    jsr     aot_emit_label_chk
    beqz    r11, .bd_done
    move.q  r9, r13                ; scratch base
    la      r1, AOT_RT_PROG_LEN
    load.q  r10, (r1)
    jsr     aot_emit_dcb
.bd_done:
    rts
.bd_oflow:
    move.q  r11, r0
    rts

; aot_emit_label_chk - emit the null-terminated label at R9 to the text at R8,
; but only if there are >= 16 bytes of headroom before AOT_TEXT_END (0 =
; unbounded). Used for the bundled-data labels (fp_print/__rtpay/__rtprog), whose
; aot_emit_str writes would otherwise be unbounded between checked dc.b blocks.
; R8 advanced. Output R11 = 1 emitted, 0 overflow (nothing emitted). Clobbers R1,R2,R9.
aot_emit_label_chk:
    la      r1, AOT_TEXT_END
    load.q  r1, (r1)
    beqz    r1, .elc_emit
    add     r2, r8, #16
    bgt     r2, r1, .elc_oflow
.elc_emit:
    jsr     aot_emit_str
    move.q  r11, #1
    rts
.elc_oflow:
    move.q  r11, r0
    store.b r0, (r8)               ; keep the text null-terminated
    rts

aot_str_dcb_kw:
    dc.b    "    dc.b ", 0
aot_str_lbl_fp_print:
    dc.b    "fp_print:", 0x0A, 0
aot_str_lbl_rtpay:
    dc.b    "__rtpay:", 0x0A, 0
aot_str_lbl_rtprog:
    dc.b    "__rtprog:", 0x0A, 0
    align 4

; ============================================================================
; COMPILE write path
; ============================================================================
; aot_file_write - write a buffer to a file through the File I/O MMIO.
; Input  R8 = name pointer, R9 = data pointer, R10 = byte length
; Output R8 = 0 on success, non-zero on error (FILE_STATUS)
; Clobbers R1, R2
aot_file_write:
    la      r1, FILE_NAME_PTR
    store.l r8, (r1)
    la      r1, FILE_DATA_PTR
    store.l r9, (r1)
    la      r1, FILE_DATA_LEN
    store.l r10, (r1)
    la      r1, FILE_CTRL
    move.q  r2, #FILE_OP_WRITE
    store.l r2, (r1)               ; write triggers on FILE_CTRL
    la      r1, FILE_STATUS
    load.l  r8, (r1)
    rts

; aot_read_rt_blob - read aot_runtime_blob.bin (the standalone COMPILE runtime blob)
; from the File I/O root into a retained AOT arena buffer, recording the buffer
; pointer (AOT_RT_BUF) and byte length (AOT_RT_LEN). File MMIO still receives a
; private low32 bridge pointer; the retained blob bytes are copied out before the
; bridge cursor may be reset by the next AOT file operation.
; Output R8 = 0 success, 2 out of memory (no buffer/bridge), 4 file error (missing
; or unreadable .bin). Clobbers R1-R3, R14.
aot_read_rt_blob:
    la      r1, AOT_RT_LEN
    store.q r0, (r1)               ; 0 until a successful read sets it
    move.q  r8, #AOT_RT_BLOB_MAX   ; retained buffer = enforced max blob size
    jsr     aot_alloc64
    bnez    r9, .rrb_oom
    la      r1, AOT_RT_BUF
    store.q r8, (r1)
    move.q  r14, r8                ; R14 = retained blob buffer
    move.q  r8, #AOT_RT_BLOB_MAX
    jsr     aot_alloc_file_bridge
    bnez    r9, .rrb_oom
    move.q  r3, r8                 ; R3 = low32 file bridge staging buffer
    la      r1, FILE_NAME_PTR
    move.q  r2, #aot_rt_blob_name
    store.l r2, (r1)
    la      r1, FILE_DATA_PTR
    store.l r3, (r1)
    la      r1, FILE_CTRL
    move.q  r2, #FILE_OP_READ
    store.l r2, (r1)               ; read triggers on FILE_CTRL
    la      r1, FILE_STATUS
    load.l  r2, (r1)
    bnez    r2, .rrb_err
    la      r1, FILE_RESULT_LEN
    load.l  r2, (r1)
    ; Defensive: the generator/tests cap the blob at AOT_RT_BLOB_MAX (= the staging
    ; size), so a valid blob always fits. Reject an over-large blob rather than trust
    ; a tampered/misgenerated one (the read has already staged it, but this stops a
    ; corrupt image from being written).
    move.q  r3, #AOT_RT_BLOB_MAX
    bgt     r2, r3, .rrb_err
    la      r1, FILE_DATA_PTR
    load.l  r3, (r1)               ; R3 = bridge base used by the read
    move.q  r1, r14                ; R1 = retained destination
    move.q  r14, r2                ; R14 = bytes remaining
.rrb_copy:
    beqz    r14, .rrb_copied
    load.b  r8, (r3)
    store.b r8, (r1)
    add.q   r3, r3, #1
    add.q   r1, r1, #1
    sub.q   r14, r14, #1
    bra     .rrb_copy
.rrb_copied:
    la      r1, AOT_RT_LEN
    store.q r2, (r1)
    move.q  r8, r0                 ; success
    rts
.rrb_oom:
    move.q  r8, #2                 ; AOT_RT_LEN already 0
    rts
.rrb_err:
    move.q  r8, #4                 ; AOT_RT_LEN already 0
    rts
aot_rt_blob_name:
    dc.b    "aot_runtime_blob.bin", 0
    align 4

; aot_emit_rt_bootstrap - emit the standalone runtime-blob bootstrap into the
; transpiled text (R8 = cursor in/out). The standalone image, at boot, reads the
; runtime blob from the File I/O root straight into AOT_RT_BASE (its link address;
; AOT_RT_BASE fits FILE_DATA_PTR and clears the File I/O range guard), then calls the
; bundled var_init through the jump table to set up variable/array/string state.
; Emitted only when the programme needs the blob (AOT_USE_RT), after the register
; bootstrap (R16/R26/R27/R31), before the first line. __rtname is a forward label to
; the bundled filename emitted at the image end (aot_str_rtname_data).
; Clobbers R9, R10 (R8 threaded).
aot_emit_rt_bootstrap:
    la      r9, aot_str_movel_r1_rtpay  ; move.l r1, #__rtpay (bundled blob payload)
    jsr     aot_emit_str
    la      r9, aot_str_movel_r2        ; move.l r2, #AOT_RT_BASE (copy destination)
    move.q  r10, #AOT_RT_BASE
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r3        ; move.l r3, #<blobLen>
    la      r1, AOT_RT_LEN
    load.q  r10, (r1)
    jsr     aot_emit_movel
    la      r9, aot_str_rt_copy         ; copy [r1, r1+r3) -> [r2, ...)
    jsr     aot_emit_str
    la      r9, aot_str_movel_r6        ; move.l r6, #RT_VAR_INIT
    move.q  r10, #RT_VAR_INIT
    jsr     aot_emit_movel
    la      r9, aot_str_loadq_r6_r6     ; load.q r6, (r6)
    jsr     aot_emit_str
    la      r9, aot_str_jsr_r6          ; jsr (r6)
    jsr     aot_emit_str
    ; Initialise the resident control-flow stack for delegated FOR/GOSUB helpers.
    la      r9, aot_str_movel_r6        ; move.l r6, #RT_EXEC_RESET_CONTROL_STACK
    move.q  r10, #RT_EXEC_RESET_CONTROL_STACK
    jsr     aot_emit_movel
    la      r9, aot_str_loadq_r6_r6     ; load.q r6, (r6)
    jsr     aot_emit_str
    la      r9, aot_str_jsr_r6          ; jsr (r6)
    jsr     aot_emit_str
    ; Clear state[ST_ERROR_FLAG] so the post-runtime-call error check (aot_emit_rt_call)
    ; starts from a clean slate; only the bundled raise_error writes a non-zero code there.
    la      r9, aot_str_movel_r1        ; move.l r1, #(BASIC_STATE + ST_ERROR_FLAG)
    move.q  r10, #(BASIC_STATE + ST_ERROR_FLAG)
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r2        ; move.l r2, #0
    move.q  r10, r0
    jsr     aot_emit_movel
    la      r9, aot_str_store_r2_r1     ; store.l r2, (r1)
    jsr     aot_emit_str
    ; If the programme bundles its tokenised source (READ/DATA), copy it to AOT_RT_PROG
    ; and point ST_PROG_START (state[0]) at it, with ST_DATA_PTR = 0. The bundled DATA
    ; reader then scans the copied programme.
    la      r1, AOT_USE_PROG
    load.q  r1, (r1)
    beqz    r1, .aerb_no_prog
    la      r9, aot_str_movel_r1_rtprog ; move.l r1, #__rtprog (bundled programme payload)
    jsr     aot_emit_str
    la      r9, aot_str_movel_r2        ; move.l r2, #AOT_RT_PROG
    move.q  r10, #AOT_RT_PROG
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r3        ; move.l r3, #<progLen>
    la      r1, AOT_RT_PROG_LEN
    load.q  r10, (r1)
    jsr     aot_emit_movel
    la      r9, aot_str_rt_prog_copy    ; copy programme bytes
    jsr     aot_emit_str
    la      r9, aot_str_movel_r1        ; move.l r1, #BASIC_STATE (= &state[ST_PROG_START])
    move.q  r10, #BASIC_STATE
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r2        ; move.l r2, #AOT_RT_PROG
    move.q  r10, #AOT_RT_PROG
    jsr     aot_emit_movel
    la      r9, aot_str_store_q         ; store.q r2, (r1)  -> ST_PROG_START
    jsr     aot_emit_str
    ; ST_PROG_END = AOT_RT_PROG + progLen, so the bundled LIST's end-of-programme
    ; sentinel (line_list compares the walk pointer against state[ST_PROG_END] - 8)
    ; matches the relocated terminator. READ/SAVE walk the next-pointer chain and do
    ; not need it, but setting it is harmless for them.
    la      r9, aot_str_movel_r1        ; move.l r1, #(BASIC_STATE + ST_PROG_END)
    move.q  r10, #(BASIC_STATE + ST_PROG_END)
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r2        ; move.l r2, #(AOT_RT_PROG + progLen)
    la      r1, AOT_RT_PROG_LEN
    load.q  r10, (r1)
    add.q   r10, r10, #AOT_RT_PROG
    jsr     aot_emit_movel
    la      r9, aot_str_store_q         ; store.q r2, (r1)  -> ST_PROG_END
    jsr     aot_emit_str
    la      r9, aot_str_movel_r1        ; move.l r1, #(BASIC_STATE + ST_DATA_PTR)
    move.q  r10, #(BASIC_STATE + ST_DATA_PTR)
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r2        ; move.l r2, #0
    move.q  r10, r0
    jsr     aot_emit_movel
    la      r9, aot_str_store_q         ; store.q r2, (r1)  -> ST_DATA_PTR = 0
    jsr     aot_emit_str
.aerb_no_prog:
    rts

; aot_emit_rt_call - emit a checked call through the blob jump table:
;   move.l r6, #<rt> / load.q r6, (r6) / jsr (r6)
; followed by an error check: if the call set state[ST_ERROR_FLAG] (raise_error fired),
; halt. The bundled raise_error already printed "?<msg> ERROR IN <line>"; a standalone
; image has no REPL to unwind to, so halting matches the interpreter's "do not run the
; following statements after an error" behaviour. The runtime check touches only R6, so
; a returned value in R8 (for example an expr_eval condition) survives it.
; R8 = output cursor (in/out), R10 = RT_ jump-table slot address.
; Clobbers R1-R7, R9, R10, R12, R14.
aot_emit_rt_call:
    la      r9, aot_str_movel_r6   ; move.l r6, #<rt>
    jsr     aot_emit_movel
    la      r9, aot_str_loadq_r6_r6 ; load.q r6, (r6)
    jsr     aot_emit_str
    la      r9, aot_str_jsr_r6     ; jsr (r6)
    jsr     aot_emit_str
    la      r1, AOT_LBLCNT         ; fresh E<id> for the no-error skip
    load.q  r14, (r1)
    add.q   r2, r14, #1
    store.q r2, (r1)
    la      r9, aot_str_movel_r6   ; move.l r6, #(BASIC_STATE + ST_ERROR_FLAG)
    move.q  r10, #(BASIC_STATE + ST_ERROR_FLAG)
    jsr     aot_emit_movel
    la      r9, aot_str_loadl_r6_r6 ; load.l r6, (r6)
    jsr     aot_emit_str
    la      r9, aot_str_beqz_r6_e  ; beqz r6, E<id>
    move.q  r10, r14
    jsr     aot_emit_movel
    la      r9, aot_str_halt       ; halt  (error already reported by raise_error)
    jsr     aot_emit_str
    la      r9, aot_str_lbl_e      ; E<id>:
    move.q  r10, r14
    jsr     aot_emit_label_named
    rts

; aot_emit_eval_call - emit a checked call to the bundled expr_eval through the blob
; jump table (standalone). R8 = cursor in/out. Clobbers R1-R7, R9, R10, R12, R14.
aot_emit_eval_call:
    move.q  r10, #RT_EXPR_EVAL
    jsr     aot_emit_rt_call
    rts

; aot_emit_truthy_call - emit tagged BASIC truthiness for the last expr_eval result.
; RUN AOT calls resident expr_truthy; standalone calls RT_EXPR_TRUTHY in the blob.
; R8 = output cursor (in/out). Output R9 = 1 ok / 0 buffer overflow.
aot_emit_truthy_call:
    bnez    r21, .etc_blob
    la      r9, aot_str_movel_r6   ; move.l r6, #expr_truthy
    move.q  r10, #expr_truthy
    jsr     aot_emit_movel
    la      r9, aot_str_jsr_r6     ; jsr (r6)
    jsr     aot_emit_str
    move.q  r9, #1
    rts
.etc_blob:
    move.q  r10, #RT_EXPR_TRUTHY
    jsr     aot_emit_rt_call
    move.q  r9, #1
    rts

; aot_emit_expr_to_i64 - convert the last tagged expression result to an I64
; selector. RUN AOT calls resident fp_int/fp_fix; standalone calls the blob slot.
; R8 = output cursor (in/out). Output R9 = 1 ok.
aot_emit_expr_to_i64:
    bnez    r21, .eti_blob
    la      r9, aot_str_movel_r6
    move.q  r10, #fp_int
    jsr     aot_emit_movel
    la      r9, aot_str_jsr_r6
    jsr     aot_emit_str
    la      r9, aot_str_movel_r6
    move.q  r10, #fp_fix
    jsr     aot_emit_movel
    la      r9, aot_str_jsr_r6
    jsr     aot_emit_str
    move.q  r9, #1
    rts
.eti_blob:
    move.q  r10, #RT_EXPR_TO_I64
    jsr     aot_emit_rt_call
    move.q  r9, #1
    rts

; aot_emit_expr - emit an expression evaluation that leaves the tagged numeric payload in R8.
; Arena (RUN AOT): move.l r17, #<resident tokens> / move.l r6, #expr_eval / jsr (r6).
; Standalone (COMPILE): bundle the tokens (aot_emit_cond_span) and call expr_eval
; through the blob jump table. R8 = output cursor (in/out, == R17 at entry), R20 =
; expression tokens (preserved). Output R9 = 1 ok / 0 buffer overflow.
aot_emit_expr:
    bnez    r21, .ee_blob
    la      r9, aot_str_movel_r17  ; move.l r17, #<resident condition tokens>
    move.q  r10, r20
    jsr     aot_emit_movel
    la      r9, aot_str_movel_r6   ; move.l r6, #expr_eval
    move.q  r10, #expr_eval
    jsr     aot_emit_movel
    la      r9, aot_str_jsr_r6     ; jsr (r6)
    jsr     aot_emit_str
    move.q  r9, #1
    rts
.ee_blob:
    move.q  r17, r8                ; cond_span tracks the cursor through R17
    jsr     aot_emit_cond_span     ; bundle -> move.l r17, #Q<id>; R9 = status
    move.q  r8, r17
    beqz    r9, .ee_ret            ; overflow -> propagate
    jsr     aot_emit_eval_call     ; expr_eval through the blob -> R8 payload, R9 tag
    move.q  r9, #1
.ee_ret:
    rts

; aot_emit_cond_span - bundle the expression token span at R20 as inline image data
; and emit "move.l r17, #Q<id>" so the bundled expr_eval reads it (standalone: the
; program text is absent and the blob occupies the private standalone runtime area). The span runs from
; R20 up to the first separator (null / THEN / ':' / ';' / ','), quote-aware so a
; string literal copies verbatim. R20 is preserved (the caller still scans the line).
; R17 is the output cursor (threaded). Output R9 = 1 ok / 0 buffer overflow.
; Uses R13 (byte), R14 (label id), R15 (scan cursor) - all survive the emit helpers.
aot_emit_cond_span:
    la      r1, AOT_LBLCNT
    load.q  r14, (r1)
    add.q   r2, r14, #1
    store.q r2, (r1)
    move.q  r8, r17
    la      r9, aot_str_bra_p      ; bra P<id> (skip the inline data)
    move.q  r10, r14
    jsr     aot_emit_movel
    la      r9, aot_str_lbl_q      ; Q<id>:
    move.q  r10, r14
    jsr     aot_emit_label_named
    la      r9, aot_str_dcb        ; dc.b
    jsr     aot_emit_str
    move.q  r17, r8
    move.q  r15, r20
.ecs_ch:
    jsr     aot_text_room
    beqz    r8, .ecs_oflow
    load.b  r13, (r15)
    beqz    r13, .ecs_end
    move.q  r2, #0xAC              ; THEN ends an IF condition
    beq     r13, r2, .ecs_end
    move.q  r2, #0x3A              ; ':' ends the statement
    beq     r13, r2, .ecs_end
    move.q  r8, r17
    move.q  r9, r13
    jsr     aot_emit_uint
    la      r9, aot_str_comma_sp
    jsr     aot_emit_str
    move.q  r17, r8
    move.q  r2, #0x22              ; '"' -> copy the string literal verbatim
    bne     r13, r2, .ecs_adv
.ecs_str:
    add.q   r15, r15, #1
    jsr     aot_text_room
    beqz    r8, .ecs_oflow
    load.b  r13, (r15)
    beqz    r13, .ecs_end
    move.q  r8, r17
    move.q  r9, r13
    jsr     aot_emit_uint
    la      r9, aot_str_comma_sp
    jsr     aot_emit_str
    move.q  r17, r8
    move.q  r2, #0x22
    bne     r13, r2, .ecs_str
.ecs_adv:
    add.q   r15, r15, #1
    bra     .ecs_ch
.ecs_end:
    jsr     aot_text_room
    beqz    r8, .ecs_oflow
    move.q  r8, r17
    la      r9, aot_str_zero_nl    ; "0\n" terminates the dc.b (null for expr_eval)
    jsr     aot_emit_str
    la      r9, aot_str_lbl_p      ; P<id>:
    move.q  r10, r14
    jsr     aot_emit_label_named
    la      r9, aot_str_movel_r17_q ; move.l r17, #Q<id>
    move.q  r10, r14
    jsr     aot_emit_movel
    move.q  r17, r8
    move.q  r9, #1
    rts
.ecs_oflow:
    move.q  r9, r0
    rts

; aot_apply_src_dir - prepend the tracked LOAD source directory (EHBASIC_PRIV_LOAD_DIR_BUF) to
; the validated output name in EHBASIC_PRIV_NAME_BUF, so COMPILE writes beside the most
; recently LOADed programme. An empty EHBASIC_PRIV_LOAD_DIR_BUF (no LOAD, or after NEW) leaves
; the name unchanged, so output goes to the File I/O root. The name is shifted
; right in place; if the combined path would not fit EHBASIC_PRIV_NAME_BUF (256 bytes)
; the prefix is skipped and the bare name is used. Clobbers R1-R7.
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
; baked aot_consttab). `include "ie64.inc"` is accepted as a no-op because those
; constants are already available; any other include, or an unknown directive/
; mnemonic, is a clean assembler error. It performs no bundling of its own, so it
; only assembles what the source defines - but because COMPILE/TRANSPILE now emit
; the runtime blob, fp_print closure and programme as in-source dc.b data (with
; the fp_print/__rtpay/__rtprog labels), ASSEMBLE reassembles a transpiled .asm
; into the same image COMPILE writes directly. A handwritten source that merely
; references __rtpay/__rtprog/fp_print without defining them still fails as an
; unresolved symbol, like any other. Both files sit beside the most recently
; LOADed programme (or the File I/O root). EHBASIC_PRIV_NAME_BUF holds the validated
; ".ie64" output name on entry. Uses R16; saves/restores callee-saved registers.
; Source files are limited to just under 1 MiB (AOT_ASM_SRC_CAP); a larger file
; is refused as a file error, never staged into the compiler workspace.
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
; Maximum ASSEMBLE source size (just under 1 MiB; the source buffer is sized to
; this and is the topmost low32 allocation, so the File I/O range guard rejects
; any larger file - see the staging comment below).
AOT_ASM_SRC_CAP equ 0x100000

aot_do_assemble_inner:
    jsr     aot_alloc_reset
    ; Allocate the source-text buffer FIRST so it is the topmost low32 allocation;
    ; the code and symbol buffers go BELOW it. The read into this buffer is bounded
    ; at the device by FILE_READ_MAX (set before the OP_READ below), so an over-large
    ; file is refused before a single byte is staged - that is the real cap. The
    ; topmost placement is secondary defence: even if the device cap were ever
    ; bypassed, the read only writes UPWARD from the buffer base and all compiler
    ; workspace sits strictly BELOW it, so workspace can never be corrupted.
    move.q  r8, #AOT_ASM_SRC_CAP
    jsr     aot_alloc_file_bridge
    bnez    r9, .as_oom
    la      r1, AOT_DC_TEXT
    store.q r8, (r1)
    move.q  r8, #0x18000           ; 96 KiB machine-code buffer for File I/O
    jsr     aot_alloc_file_bridge
    bnez    r9, .as_oom
    la      r1, AOT_DC_CODE
    store.q r8, (r1)
    add.q   r2, r8, #0x18000
    la      r1, AOT_CODE_END
    store.q r2, (r1)               ; bound the buffer so a big source reports OOM
    move.q  r8, #AOT_SYMTAB_BYTES   ; private-assembler symbol table
    jsr     aot_alloc64
    bnez    r9, .as_oom
    la      r1, AOT_SYMTAB_PTR
    store.q r8, (r1)

    ; Clear COMPILE bundling state so aot_asm_program injects no helper symbols:
    ; ASSEMBLE source is self-contained, with no blob, fp_print closure or
    ; programme appended. __rtpay/__rtprog/fp_print therefore stay unresolved
    ; unless the source defines them.
    la      r1, AOT_HELPER_FP
    store.q r0, (r1)
    la      r1, AOT_USE_RT
    store.q r0, (r1)
    la      r1, AOT_USE_PROG
    store.q r0, (r1)

    ; Derive NAME.asm (the input) from EHBASIC_PRIV_NAME_BUF (NAME.ie64), in the source
    ; directory, exactly where COMPILE/TRANSPILE place their .asm sidecar.
    jsr     aot_apply_src_dir
    jsr     aot_make_asm_name       ; EHBASIC_PRIV_ASM_NAME_BUF = "...asm"

    ; Cap the read at the device BEFORE it copies anything: FILE_READ_MAX makes the
    ; File I/O device refuse a file larger than the source buffer (room kept for the
    ; null terminator) without staging a single byte into guest memory, so an
    ; over-large source cannot overrun the buffer regardless of the active/backing
    ; RAM relationship. The cap is one-shot (the device consumes it per read).
    la      r1, FILE_READ_MAX
    move.q  r2, #(AOT_ASM_SRC_CAP - 1)
    store.l r2, (r1)

    ; Read NAME.asm into the source buffer via the File I/O MMIO (OP_READ). The
    ; data pointer is the 32-bit File I/O ABI field; the buffer base is a low32
    ; allocation (< AOT_LOW32_CAP), so it fits.
    la      r1, FILE_NAME_PTR
    la      r2, EHBASIC_PRIV_ASM_NAME_BUF
    store.l r2, (r1)
    la      r1, FILE_DATA_PTR
    la      r2, AOT_DC_TEXT
    load.q  r2, (r2)
    store.l r2, (r1)
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
    move.q  r4, #AOT_ASM_SRC_CAP
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
    move.q  r8, #0x40000           ; 256 KiB asm-text buffer (holds the self-contained
                                   ; dc.b'd closure/blob/programme, like COMPILE)
    jsr     aot_alloc_file_bridge
    bnez    r9, .tp_oom
    la      r1, AOT_DC_TEXT
    store.q r8, (r1)
    move.q  r8, #0x18000           ; 96 KiB scratch buffer for programme relocation
    jsr     aot_alloc_file_bridge  ; (aot_emit_bundled_data uses AOT_DC_CODE as scratch)
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
    add.q   r11, r9, #0x40000      ; text buffer end (256 KiB)
    jsr     aot_transpile          ; R8=status R9=textLen
    move.q  r1, #2
    beq     r8, r1, .tp_oom        ; text buffer overflow
    beqz    r8, .tp_unsupported    ; unsupported statement

    ; Match COMPILE's failure modes: if the programme needs the standalone runtime
    ; blob but it could not be read at compile time (missing/unreadable sidecar in
    ; the File I/O root), report a file error and write nothing, exactly as COMPILE
    ; does, rather than emitting an asm for a programme COMPILE would have rejected.
    la      r1, AOT_USE_RT
    load.q  r1, (r1)
    beqz    r1, .tp_rt_present
    la      r1, AOT_RT_LEN
    load.q  r1, (r1)
    beqz    r1, .tp_file_err
.tp_rt_present:

    ; Append any source-bundled runtime helpers the programme requested.
    la      r1, AOT_DC_TEXT
    load.q  r8, (r1)
    jsr     aot_append_helpers     ; R8=buf, R9=len -> R8=new len
    la      r1, AOT_DC_TLEN
    store.q r8, (r1)

    ; Compute the bundled programme length (READ/DATA) before emitting it.
    la      r1, AOT_RT_PROG_LEN
    store.q r0, (r1)
    la      r1, AOT_USE_PROG
    load.q  r1, (r1)
    beqz    r1, .tp_proglen_done
    add.q   r1, r16, #ST_PROG_END
    load.q  r2, (r1)
    load.q  r3, (r16)              ; live ST_PROG_START
    sub.q   r2, r2, r3
    la      r1, AOT_RT_PROG_LEN
    store.q r2, (r1)
.tp_proglen_done:

    ; Emit the bundled payloads (fp_print closure, runtime blob, programme) as
    ; labelled dc.b data, so the .asm is fully self-contained and reassembles to a
    ; working image via ASSEMBLE - identical to COMPILE's sidecar.
    la      r1, AOT_DC_TEXT
    load.q  r8, (r1)
    la      r1, AOT_DC_TLEN
    load.q  r9, (r1)
    add.q   r8, r8, r9
    jsr     aot_emit_bundled_data  ; R8 advanced, R11 = 1 ok / 0 overflow
    beqz    r11, .tp_oom
    la      r1, AOT_DC_TEXT
    load.q  r1, (r1)
    sub.q   r8, r8, r1
    la      r1, AOT_DC_TLEN
    store.q r8, (r1)

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
    move.q  r8, #0x40000           ; 256 KiB asm-text buffer (low32, writable): holds
                                   ; the user code plus the fp_print closure, runtime
                                   ; blob (~34 KiB -> ~190 KiB as dc.b) and programme
                                   ; emitted as self-contained dc.b data.
    jsr     aot_alloc_file_bridge
    bnez    r9, .dc_oom
    la      r1, AOT_DC_TEXT
    store.q r8, (r1)
    move.q  r8, #0x18000           ; 96 KiB machine-code buffer (low32, writable):
                                   ; holds user code + fp_print closure + the ~34 KiB
                                   ; bundled runtime blob payload.
    jsr     aot_alloc_file_bridge
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
    add.q   r2, r2, #0x18000
    la      r1, AOT_CODE_END
    store.q r2, (r1)

    ; Transpile the live programme in standalone mode (ends with halt).
    load.q  r8, (r16)              ; first line record (state[ST_PROG_START])
    la      r1, AOT_DC_TEXT
    load.q  r9, (r1)
    move.q  r10, #1                ; standalone
    add.q   r11, r9, #0x40000      ; text buffer end (256 KiB)
    jsr     aot_transpile          ; R8=status R9=textLen
    move.q  r1, #2
    beq     r8, r1, .dc_oom        ; text buffer overflow
    beqz    r8, .dc_unsupported    ; unsupported statement (reported before any blob issue)
    ; Supported programme: if it needs the runtime blob but the blob could not be read
    ; at compile time (missing sidecar in the File I/O root), report a file error.
    la      r1, AOT_USE_RT
    load.q  r1, (r1)
    beqz    r1, .dc_rt_present
    la      r1, AOT_RT_LEN
    load.q  r1, (r1)
    beqz    r1, .dc_file_err
.dc_rt_present:
    ; Append any source-bundled runtime helpers the programme requested.
    la      r1, AOT_DC_TEXT
    load.q  r8, (r1)
    jsr     aot_append_helpers     ; R8=buf, R9=len -> R8=new len
    la      r1, AOT_DC_TLEN
    store.q r8, (r1)

    ; Reset closure-last bundling state. fp_print (PRINT number) and the runtime blob
    ; are both appended after the user code, so each needs the user-code length before
    ; the real assembly. A provisional (sizing) assembly measures it (jsr/move are
    ; fixed size, so the placeholder addresses do not change the length); the closure
    ; and blob are then placed after the user code and their link addresses injected
    ; for the real assembly.
    la      r1, AOT_HELPER_FP
    store.q r0, (r1)
    la      r1, AOT_HELPER_LEN
    store.q r0, (r1)
    la      r1, AOT_RT_PAYLOAD
    store.q r0, (r1)
    la      r1, AOT_RT_PROG_PAY
    store.q r0, (r1)
    ; Programme bundling (READ/DATA): measure the tokenised programme length now
    ; (state[ST_PROG_END] - state[ST_PROG_START]); it is appended after the blob and its
    ; line-record next-pointers are relocated when copied.
    la      r1, AOT_RT_PROG_LEN
    store.q r0, (r1)
    la      r1, AOT_USE_PROG
    load.q  r1, (r1)
    beqz    r1, .dc_proglen_done
    add.q   r1, r16, #ST_PROG_END
    load.q  r2, (r1)
    load.q  r3, (r16)              ; live ST_PROG_START
    sub.q   r2, r2, r3             ; programme byte length
    la      r1, AOT_RT_PROG_LEN
    store.q r2, (r1)
.dc_proglen_done:
    ; Emit the bundled payloads (fp_print closure, runtime blob, tokenised
    ; programme) into the text as labelled dc.b data, so the assembler resolves
    ; fp_print/__rtpay/__rtprog as ordinary forward labels. This makes the .asm
    ; fully self-contained and removes the old sizing pass + address injection.
    la      r1, AOT_DC_TEXT
    load.q  r8, (r1)
    la      r1, AOT_DC_TLEN
    load.q  r9, (r1)
    add.q   r8, r8, r9             ; cursor = end of the helper-appended text
    jsr     aot_emit_bundled_data  ; R8 advanced, R11 = 1 ok / 0 overflow
    beqz    r11, .dc_oom
    la      r1, AOT_DC_TEXT
    load.q  r1, (r1)
    sub.q   r8, r8, r1
    la      r1, AOT_DC_TLEN
    store.q r8, (r1)               ; full self-contained text length (for the .asm)

    ; The text now defines fp_print/__rtpay/__rtprog, so disable the assembler's
    ; symbol injection (it would otherwise duplicate those labels).
    la      r1, AOT_HELPER_FP
    store.q r0, (r1)
    la      r1, AOT_USE_RT
    store.q r0, (r1)
    la      r1, AOT_USE_PROG
    store.q r0, (r1)

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
    move.q  r8, #0x10000           ; asm-text buffer
    jsr     aot_alloc64
    bnez    r9, .ra_oom
    la      r1, AOT_DC_TEXT
    store.q r8, (r1)
    move.q  r8, #0x10000           ; code arena (executed in place)
    jsr     aot_alloc64
    bnez    r9, .ra_oom
    la      r1, AOT_DC_CODE
    store.q r8, (r1)
    move.q  r8, #AOT_SYMTAB_BYTES   ; private-assembler symbol table
    jsr     aot_alloc64
    bnez    r9, .ra_oom
    la      r1, AOT_SYMTAB_PTR
    store.q r8, (r1)

    ; Bound the code arena so a large programme reports OOM, not corruption.
    la      r1, AOT_DC_CODE
    load.q  r8, (r1)
    add.q   r2, r8, #0x10000
    la      r1, AOT_CODE_END
    store.q r2, (r1)

    ; Transpile in arena mode (ends with rts so control returns here).
    load.q  r8, (r16)              ; first line record
    la      r1, AOT_DC_TEXT
    load.q  r9, (r1)
    move.q  r10, r0                ; arena mode
    add.q   r11, r9, #0x10000      ; text buffer end (64 KiB)
    jsr     aot_transpile
    move.q  r1, #2
    beq     r8, r1, .ra_oom        ; text buffer overflow
    beqz    r8, .ra_unsupported
    ; Append any source-bundled runtime helpers the programme requested.
    la      r1, AOT_DC_TEXT
    load.q  r8, (r1)
    jsr     aot_append_helpers     ; R8=buf, R9=len -> R8=new len

    ; Machine-code-bundle the fp_print closure ahead of the user code (if used),
    ; so the user code assembles after it and "jsr fp_print" resolves into it.
    la      r1, AOT_HELPER_FP
    store.q r0, (r1)
    la      r1, AOT_HELPER_LEN
    store.q r0, (r1)
    la      r1, AOT_USE_FP
    load.q  r1, (r1)
    beqz    r1, .ra_no_fp
    la      r1, AOT_DC_CODE
    load.q  r8, (r1)
    jsr     aot_bundle_fp          ; R8 = helper byte length
    la      r1, AOT_HELPER_LEN
    store.q r8, (r1)
.ra_no_fp:

    ; Assemble at link base = arena address + helper length (helpers come first).
    la      r1, AOT_DC_TEXT
    load.q  r8, (r1)
    la      r1, AOT_HELPER_LEN
    load.q  r3, (r1)
    la      r1, AOT_DC_CODE
    load.q  r9, (r1)
    add.q   r9, r9, r3             ; link base = arena + helper length
    move.q  r10, r9                ; destination = arena + helper length
    jsr     aot_asm_program
    move.q  r1, #2
    beq     r8, r1, .ra_oom        ; code buffer overflow
    beqz    r8, .ra_asm_err

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
    la      r1, if_else_boundary
    store.l r0, (r1)
    load.q  r14, (r16)             ; R14 = first line record
    ; Clear any pending native continuation: a fresh RUN AOT restarts the programme,
    ; so a stale STOP resume address from a previous run must not be honoured by CONT.
    la      r1, AOT_CONT_PC
    store.q r0, (r1)

    ; Execute the native code; entry is past the bundled helpers (user code).
    ; Its trailing rts returns control here.
    la      r1, AOT_DC_CODE
    load.q  r8, (r1)
    la      r1, AOT_HELPER_LEN
    load.q  r1, (r1)
    add.q   r8, r8, r1
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
