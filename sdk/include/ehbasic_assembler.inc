; ehbasic_assembler.inc - private in-guest IE64 assembler
; ============================================================================
; The ASSEMBLE command and both compiler drivers assemble through this unit.
; It accepts canonical host ie64asm syntax for the standalone contract and
; treats include "ie64.inc" as a no-op because its canonical constants are
; baked in through ehbasic_assembler_consttab.inc.

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
AOT_CLS_MOVT  equ 20     ; movt rd, #imm32

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
    dc.b "movt", 0,0,0,0,   0x02, AOT_CLS_MOVT,  0,0,0,0,0,0
    dc.b "load", 0,0,0,0,   0x10, AOT_CLS_LOAD,  0,0,0,0,0,0
    dc.b "store",0,0,0,     0x11, AOT_CLS_STORE, 0,0,0,0,0,0
    dc.b "lea",  0,0,0,0,0, 0x04, AOT_CLS_LEA,   0,0,0,0,0,0
    dc.b "la",   0,0,0,0,0,0, 0x04, AOT_CLS_LA,  0,0,0,0,0,0
    dc.b "push", 0,0,0,0,   0x52, AOT_CLS_PUSH,  0,0,0,0,0,0
    dc.b "pop",  0,0,0,0,0, 0x53, AOT_CLS_POP,   0,0,0,0,0,0
    dc.b "beqz", 0,0,0,0,   0x41, AOT_CLS_BZ,    0,0,0,0,0,0
    dc.b "bnez", 0,0,0,0,   0x42, AOT_CLS_BZ,    0,0,0,0,0,0
    dc.b "bltz", 0,0,0,0,   0x43, AOT_CLS_BZ,    0,0,0,0,0,0
    dc.b "bgez", 0,0,0,0,   0x44, AOT_CLS_BZ,    0,0,0,0,0,0
    dc.b "bgtz", 0,0,0,0,   0x45, AOT_CLS_BZ,    0,0,0,0,0,0
    dc.b "blez", 0,0,0,0,   0x46, AOT_CLS_BZ,    0,0,0,0,0,0
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
; The table (assembler_consttab) is build-time generated from ie64.inc by
; tools/gen_assembler_consttab and pulled in via the include below.
; Records: [name length : 1][name bytes][value : 4 LE], 0-length terminated.
aot_const_lookup:
    la      r3, assembler_consttab
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

; Build-time generated [name pointer, value] table (tools/gen_assembler_consttab).
    include "ehbasic_assembler_consttab.inc"

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
    move.q  r1, #AOT_CLS_MOVT
    beq     r23, r1, .al_movt
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

.al_movt:
    move.q  r24, #3                 ; movt is encoded as a qword-sized immediate op
    jsr     aot_skip_comma
    beqz    r9, .al_err
    move.q  r8, r20
.mt_sp:
    load.b  r1, (r8)
    move.q  r2, #0x20
    bne     r1, r2, .mt_hash
    add.q   r8, r8, #1
    bra     .mt_sp
.mt_hash:
    move.q  r2, #0x23               ; '#'
    bne     r1, r2, .al_err
    add.q   r8, r8, #1
    jsr     aot_parse_value
    beqz    r9, .al_err
    move.q  r29, r8
    move.q  r26, #1
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
; beqz/bnez/bltz/bgez/bgtz/blez rs, label -> b<cc> rs, r0, label (rt stays 0).
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
    move.q  r2, #0x73               ; 's' -> ds.q
    beq     r1, r2, .id_ds
    move.q  r2, #0x63               ; 'c'
    bne     r1, r2, .id_no
    load.b  r1, 2(r8)
    move.q  r2, #0x2E               ; '.'
    bne     r1, r2, .id_no
    move.q  r8, #1
    rts
.id_ds:
    load.b  r1, 2(r8)
    move.q  r2, #0x2E
    bne     r1, r2, .id_no
    load.b  r1, 3(r8)
    or.q    r1, r1, #0x20
    move.q  r2, #0x71               ; only ds.q is emitted by the compiler
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
    load.b  r1, 1(r13)
    or.q    r1, r1, #0x20
    move.q  r2, #0x73               ; 's' -> ds.q count
    beq     r1, r2, .ad_space_q
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

.ad_space_q:
    add.q   r13, r13, #4           ; past "ds.q"
    move.q  r8, r13
    jsr     aot_parse_value
    beqz    r9, .ad_err
    move.q  r13, r10
    lsl.q   r3, r8, #3
    lsr.q   r4, r3, #3
    bne     r4, r8, .ad_oflow
    beqz    r15, .adsq_zero
    add.q   r4, r12, r3
    blt     r4, r12, .ad_oflow
    bgt     r4, r15, .ad_oflow
.adsq_zero:
    beqz    r3, .ad_ok
    store.b r0, (r12)
    add.q   r12, r12, #1
    sub.q   r3, r3, #1
    bra     .adsq_zero

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
; baked in via assembler_consttab, so a user source that opens with `include "ie64.inc"`
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
    la      r3, aot_str_org_prog_start
.ic_org_cmp:
    load.b  r4, (r3)
    beqz    r4, .ic_org_tail
    load.b  r1, (r2)
    beq     r1, r4, .ic_org_char_ok
    or.q    r1, r1, #0x20
    bne     r1, r4, .ic_org_not
.ic_org_char_ok:
    add.q   r2, r2, #1
    add.q   r3, r3, #1
    bra     .ic_org_cmp
.ic_org_tail:
    ; Standalone compiler output uses the canonical fixed link origin. The
    ; private assembler already receives PROG_START as its link base, so this
    ; exact directive is a validated no-op in both layout passes.
    bra     .ic_tail
.ic_org_not:
    move.q  r2, r8
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
aot_str_org_prog_start:
    dc.b    "org prog_start", 0
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
    la      r1, AOT_ASM_ERROR_SRC
    store.q r17, (r1)              ; source cursor after the rejected line
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
    la      r1, AOT_EMIT_OVERFLOW
    load.q  r1, (r1)
    bnez    r1, .es_blocked
    beqz    r8, .es_blocked
.es_loop:
    load.b  r1, (r9)
    beqz    r1, .es_done
    la      r2, AOT_TEXT_END
    load.q  r2, (r2)
    beqz    r2, .es_store
    bge     r8, r2, .es_overflow
.es_store:
    store.b r1, (r8)
    add.q   r8, r8, #1
    add.q   r9, r9, #1
    bra     .es_loop
.es_done:
    rts
.es_overflow:
    la      r1, AOT_EMIT_OVERFLOW
    move.q  r2, #1
    store.q r2, (r1)
    move.q  r8, r0
.es_blocked:
    rts

; aot_text_room - R8 = 1 if the asm-text buffer has >= 256 bytes of headroom at
; the current cursor R17 (covers any single statement's emit, including the
; multi-line WAIT/VSYNC poll loops), or if AOT_TEXT_END is 0 (unbounded).
; Else R8 = 0. Clobbers R1-R3.
aot_text_room:
    la      r1, AOT_EMIT_OVERFLOW
    load.q  r1, (r1)
    bnez    r1, .room_no
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

