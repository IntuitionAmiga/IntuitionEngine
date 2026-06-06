; ehbasic_exec.inc - IE64 BASIC Statement Executor
;
; Main interpreter loop and statement handlers for IE64 BASIC.
;
; The interpreter walks through stored program lines (linked list),
; tokenizes each statement, and dispatches to the appropriate handler.
;
; Register conventions (preserved across statements):
;   R16 = interpreter state base (BASIC_STATE)
;   R17 = text pointer (current position in tokenized line content)
;   R26 = cached TERM_OUT address
;   R27 = cached TERM_STATUS address
;   R31 = hardware stack pointer
;
; Key state block fields:
;   ST_CURRENT_LINE (+0x200) = current line number
;   ST_DIRECT_MODE  (+0x210) = 1 if immediate mode, 0 if running
;   ST_GOSUB_SP     (+0x060) = GOSUB stack cursor, grows upward from ST_CTRL_LOW
;   ST_FOR_SP       (+0x068) = loop stack cursor, grows downward from ST_CTRL_HIGH
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; basic_layout_init - publish dynamic BASIC stack/control/layout bounds
; ============================================================================
; Input:  R16 = BASIC_STATE base after the state page has been cleared.
; Output: R31 = dynamic hardware stack pointer.
; Clobbers: R1-R7.
; Notes:  May change R31. It preserves the JSR return PC across the stack move,
;         so cold_start can call it after seeding any temporary valid stack.

basic_layout_init:
    pop     r7                         ; preserve return PC before changing R31
    mfcr    r1, cr15                    ; CR_RAM_SIZE_BYTES
    bnez    r1, .bli_have_ram
    move.q  r1, #0x02000000             ; legacy harness fallback: 32 MiB
.bli_have_ram:
    lsr.q   r1, r1, #12
    lsl.q   r1, r1, #12                 ; page-align active RAM down
    ; Keep the resident interpreter stack/control area in BASIC's mapped
    ; low memory window. Production IE64 BASIC can expose sparse RAM above
    ; this, but the current BASIC/JIT resident paths still expect their own
    ; stack and scratch pointers to be directly low-window addressable.
    move.q  r5, #BASIC_RESIDENT_LOW32_CAP
    blt     r1, r5, .bli_stack_top_ready
    move.q  r1, r5
.bli_stack_top_ready:
    move.q  r2, #BASIC_STACK_GUARD_BYTES
    sub.q   r1, r1, r2                  ; first byte below guard page
    move.q  r31, r1

    add.q   r2, r16, #ST_STACK_HIGH
    store.q r31, (r2)
    move.q  r3, #BASIC_STACK_BYTES
    sub.q   r3, r31, r3                 ; stack_low
    add.q   r2, r16, #ST_STACK_LOW
    store.q r3, (r2)

    add.q   r2, r16, #ST_CTRL_HIGH
    store.q r3, (r2)
    move.q  r4, #BASIC_CTRL_BYTES
    sub.q   r4, r3, r4                  ; ctrl_low
    add.q   r2, r16, #ST_CTRL_LOW
    store.q r4, (r2)

    add.q   r2, r16, #ST_GOSUB_SP
    store.q r4, (r2)
    add.q   r2, r16, #ST_FOR_SP
    store.q r3, (r2)

    ; Private file-data bridge lives below both dynamic reservations and below the
    ; 32-bit MMIO ABI ceiling. The current low32 bridge implementation is still a
    ; later plan slice, but publishing the bounds now keeps all reset paths coherent.
    move.q  r5, #AOT_LOW32_CAP
    blt     r4, r5, .bli_bridge_top_ready
    move.q  r4, r5
.bli_bridge_top_ready:
    add.q   r2, r16, #ST_FILE_BRIDGE_NEXT
    store.q r4, (r2)
    move.q  r6, #BASIC_LOW32_INTERNAL_BASE
    add.q   r2, r16, #ST_FILE_BRIDGE_LOW
    store.q r6, (r2)
    add.q   r2, r16, #ST_SCRATCH_BASE
    store.q r6, (r2)
    add.q   r2, r16, #ST_LINE_BUF_PTR
    store.q r6, (r2)
    move.q  r6, #BASIC_LINE_BUF_BYTES
    add.q   r2, r16, #ST_LINE_BUF_CAP
    store.q r6, (r2)
    add.q   r6, r6, #BASIC_LOW32_INTERNAL_BASE
    add.q   r2, r16, #ST_SCRATCH_END
    store.q r6, (r2)
    add.q   r2, r16, #ST_FILE_BRIDGE_LOW
    store.q r6, (r2)
    add.q   r2, r16, #ST_HEAP_BOTTOM
    store.q r6, (r2)
    add.q   r2, r16, #ST_HEAP_TOP
    store.q r4, (r2)
    push    r7                         ; move return PC to the new stack
    rts

; ============================================================================
; exec_run - Execute stored BASIC program from beginning
; ============================================================================
; Input:  R16 = state base (program already stored via line_store)
; Output: Returns when END/STOP reached or program falls off last line
; Clobbers: All registers except R16, R26, R27, R31

exec_run:
    push    r14
    push    r15
    push    r22
    push    r23
    push    r24
    push    r25

    ; Clear direct mode
    add.q   r1, r16, #ST_DIRECT_MODE
    store.l r0, (r1)

    ; Initialise control-flow stack cursors
    jsr     exec_reset_control_stack

    ; Reset DATA/READ pointer
    add.q   r1, r16, #ST_DATA_PTR
    store.q r0, (r1)

    ; Trigger GC before main execution loop starts
    la      r1, SYS_GC_TRIGGER
    store.l r0, (r1)

    ; Start at first line
    load.q  r14, (r16)           ; R14 = address of first line
    ; R14 is our "current line pointer" throughout execution

; ============================================================================
; exec_next_line - Advance to the next line and execute it
; ============================================================================
; R14 = pointer to current line entry (header: next.q + line.l + reserved.l + content)

exec_next_line:
    ; Check end of program
    beqz    r14, .exec_done

    ; Read next-line pointer. If it's 0, R14 is the end terminator - stop.
    ; Invariant: line_rechain sets every real line's next-ptr to a valid address
    ; (either the next line or the terminator). Only the terminator itself has 0.
    load.q  r15, (r14)           ; R15 = next-line pointer
    beqz    r15, .exec_done      ; R14 is the terminator - done

    ; Valid line: read line number
    add.q   r1, r14, #8
    load.l  r1, (r1)            ; R1 = line number

    ; Store current line number in state
    add.q   r2, r16, #ST_CURRENT_LINE
    store.l r1, (r2)

    ; Set text pointer to start of content (offset +16)
    add.q   r17, r14, #16

    ; TRON trace: print [linenum] if enabled (R1 = line number)
    jsr     tron_check

    ; Execute the statements on this line
    jsr     exec_line

    ; If exec_line returns 0 in R8, continue to next line normally
    ; If R8 = 1, a GOTO/GOSUB changed R14 - use the new value
    ; If R8 = 2, END was hit - stop
    ; If R8 = 3, runtime error was reported - stop with stacks reset
    move.q  r1, #3
    beq     r8, r1, .exec_error
    move.q  r1, #2
    beq     r8, r1, .exec_done
    move.q  r1, #1
    beq     r8, r1, exec_next_line  ; R14 already updated by GOTO/GOSUB

    ; Normal: advance to next line
    ; Re-read next-ptr from R14 (R15 may have been clobbered by statement handlers)
    load.q  r14, (r14)          ; R14 = next-line pointer
    bra     exec_next_line

.exec_error:
    jsr     exec_reset_control_stack
    bra     .exec_done

.exec_done:
    pop     r25
    pop     r24
    pop     r23
    pop     r22
    pop     r15
    pop     r14
    rts

; ============================================================================
; exec_line - Execute all statements on the current line
; ============================================================================
; Input:  R17 = pointer to tokenized content (null-terminated)
;         R14 = current line pointer (for GOTO to update)
; Output: R8  = 0 (normal), 1 (GOTO/GOSUB changed R14), 2 (END/STOP)
; Clobbers: R1-R13, R15, R22-R25

exec_line:
    push    r28
    move.q  r28, r0              ; R28 = return code (0 = normal)
    la      r1, if_else_boundary
    store.q r0, (r1)

.el_next_stmt:
    ; Skip spaces
    jsr     exec_skip_spaces

    ; True IF clauses set an ELSE boundary so the statement loop stops before
    ; executing the ELSE clause.
    la      r1, if_else_boundary
    load.q  r1, (r1)
    beqz    r1, .el_no_else_boundary
    bge     r17, r1, .el_done
.el_no_else_boundary:

    ; Get current token/character
    load.b  r1, (r17)
    beqz    r1, .el_done         ; null = end of line

    ; Check for ':' statement separator
    move.q  r2, #0x3A
    beq     r1, r2, .el_colon

    ; Check if it's a token (>= 0x80)
    move.q  r2, #0x80
    blt     r1, r2, .el_implied_let

    ; Token dispatch
    ; Subtract 0x80 to get index
    sub.q   r1, r1, #0x80

    ; Dispatch to handler based on token index
    ; TK_END     = 0x80 → index 0
    ; TK_FOR     = 0x81 → index 1
    ; TK_NEXT    = 0x82 → index 2
    ; TK_DATA    = 0x83 → index 3
    ; TK_INPUT   = 0x84 → index 4
    ; TK_DIM     = 0x85 → index 5
    ; TK_READ    = 0x86 → index 6
    ; TK_LET     = 0x87 → index 7
    ; ...
    ; TK_GOTO    = 0x89 → index 9
    ; TK_RUN     = 0x8A → index 10
    ; TK_IF      = 0x8B → index 11
    ; TK_RESTORE = 0x8C → index 12
    ; TK_GOSUB   = 0x8D → index 13
    ; TK_RETURN  = 0x8E → index 14
    ; TK_REM     = 0x8F → index 15
    ; TK_STOP    = 0x90 → index 16
    ; TK_PRINT   = 0x9E → index 30

    add.q   r17, r17, #1        ; consume the token byte

    ; --- Jump table dispatch ---
    ; R1 = token index (0-127), guaranteed in range by load.b - 0x80
    la      r9, stmt_jump_table
    lsl.q   r2, r1, #2              ; index * 4
    add.q   r9, r9, r2
    load.l  r9, (r9)                ; handler address
    jsr     (r9)                    ; dispatch to handler
    ; fall through to check return code
.el_check_rc:
    ; After handler returns, check R28
    ; 0 = continue to next stmt, 1 = GOTO (R14 changed), 2 = END/STOP
    bnez    r28, .el_done
    bra     .el_next_stmt

.el_colon:
    ; Statement separator - advance and continue
    add.q   r17, r17, #1
    bra     .el_next_stmt

.el_implied_let:
    ; Not a token and not ':' - must be variable assignment (implied LET)
    ; Check if it's a letter (A-Z or a-z)
    move.q  r2, #0x41            ; 'A'
    blt     r1, r2, .el_done
    move.q  r2, #0x7A            ; 'z'
    bgt     r1, r2, .el_done

    ; --- Intercept coprocessor statements (COSTART, COSTOP, COWAIT)
    ;     and the HOST statement. These are recognized as raw text in the
    ;     line buffer because they are not tokenized. ---
    or.q    r3, r1, #0x20        ; normalize to lowercase
    move.q  r2, #0x68            ; 'h' → HOST
    beq     r3, r2, .el_check_host
    move.q  r2, #0x63            ; 'c'
    bne     r3, r2, .el_do_let
    load.b  r3, 1(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x6F            ; 'o'
    bne     r3, r2, .el_do_let
    ; "CO..." - check third char
    load.b  r3, 2(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x73            ; 's' → COSTART or COSTOP
    beq     r3, r2, .el_check_cos
    move.q  r2, #0x77            ; 'w' → COWAIT
    beq     r3, r2, .el_check_cowait
    bra     .el_do_let

.el_check_cos:
    ; Check for COSTART (7 chars) or COSTOP (6 chars)
    load.b  r3, 3(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x74            ; 't'
    bne     r3, r2, .el_do_let
    load.b  r3, 4(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x61            ; 'a' → COSTART
    beq     r3, r2, .el_match_costart
    move.q  r2, #0x6F            ; 'o' → COSTOP
    beq     r3, r2, .el_match_costop
    bra     .el_do_let

.el_match_costart:
    ; Verify remaining chars: R, T + word boundary
    load.b  r3, 5(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x72            ; 'r'
    bne     r3, r2, .el_do_let
    load.b  r3, 6(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x74            ; 't'
    bne     r3, r2, .el_do_let
    ; Word boundary: next char must not be alpha, digit, or '$'
    load.b  r3, 7(r17)
    move.q  r2, #0x24            ; '$'
    beq     r3, r2, .el_do_let   ; string suffix → variable
    or.q    r3, r3, #0x20
    move.q  r2, #0x30            ; '0'
    blt     r3, r2, .el_is_costart
    move.q  r2, #0x39            ; '9'
    ble     r3, r2, .el_do_let   ; digit → variable
    move.q  r2, #0x61            ; 'a'
    blt     r3, r2, .el_is_costart
    move.q  r2, #0x7A            ; 'z'
    ble     r3, r2, .el_do_let   ; alpha → variable
.el_is_costart:
    add.q   r17, r17, #7         ; consume "COSTART"
    jsr     exec_do_costart
    bra     .el_check_rc

.el_match_costop:
    ; Verify remaining char: P + word boundary
    load.b  r3, 5(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x70            ; 'p'
    bne     r3, r2, .el_do_let
    ; Word boundary: next char must not be alpha, digit, or '$'
    load.b  r3, 6(r17)
    move.q  r2, #0x24            ; '$'
    beq     r3, r2, .el_do_let   ; string suffix → variable
    or.q    r3, r3, #0x20
    move.q  r2, #0x30            ; '0'
    blt     r3, r2, .el_is_costop
    move.q  r2, #0x39            ; '9'
    ble     r3, r2, .el_do_let   ; digit → variable
    move.q  r2, #0x61            ; 'a'
    blt     r3, r2, .el_is_costop
    move.q  r2, #0x7A            ; 'z'
    ble     r3, r2, .el_do_let
.el_is_costop:
    add.q   r17, r17, #6         ; consume "COSTOP"
    jsr     exec_do_costop
    bra     .el_check_rc

.el_check_cowait:
    ; Match "COWAIT" (6 chars)
    load.b  r3, 3(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x61            ; 'a'
    bne     r3, r2, .el_do_let
    load.b  r3, 4(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x69            ; 'i'
    bne     r3, r2, .el_do_let
    load.b  r3, 5(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x74            ; 't'
    bne     r3, r2, .el_do_let
    ; Word boundary: next char must not be alpha, digit, or '$'
    load.b  r3, 6(r17)
    move.q  r2, #0x24            ; '$'
    beq     r3, r2, .el_do_let   ; string suffix → variable
    or.q    r3, r3, #0x20
    move.q  r2, #0x30            ; '0'
    blt     r3, r2, .el_is_cowait
    move.q  r2, #0x39            ; '9'
    ble     r3, r2, .el_do_let   ; digit → variable
    move.q  r2, #0x61            ; 'a'
    blt     r3, r2, .el_is_cowait
    move.q  r2, #0x7A            ; 'z'
    ble     r3, r2, .el_do_let
.el_is_cowait:
    add.q   r17, r17, #6         ; consume "COWAIT"
    jsr     exec_do_cowait
    bra     .el_check_rc

.el_check_host:
    ; Match "HOST" (4 chars) starting at r17, with word boundary.
    ; First char was already verified to be 'h'/'H' by the caller.
    load.b  r3, 1(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x6F            ; 'o'
    bne     r3, r2, .el_do_let
    load.b  r3, 2(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x73            ; 's'
    bne     r3, r2, .el_do_let
    load.b  r3, 3(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x74            ; 't'
    bne     r3, r2, .el_do_let
    ; Word boundary: char after "HOST" must not be alpha, digit, or '$'
    load.b  r3, 4(r17)
    move.q  r2, #0x24            ; '$'
    beq     r3, r2, .el_do_let   ; string suffix → variable
    or.q    r3, r3, #0x20
    move.q  r2, #0x30            ; '0'
    blt     r3, r2, .el_is_host
    move.q  r2, #0x39            ; '9'
    ble     r3, r2, .el_do_let   ; digit → variable
    move.q  r2, #0x61            ; 'a'
    blt     r3, r2, .el_is_host
    move.q  r2, #0x7A            ; 'z'
    ble     r3, r2, .el_do_let   ; alpha → variable
.el_is_host:
    add.q   r17, r17, #4         ; consume "HOST"
    jsr     exec_do_host
    bra     .el_check_rc

.el_do_let:
    ; It's a letter - implied LET (don't consume, var_lookup will read name)
    jsr     exec_do_let
    bra     .el_check_rc

.el_done:
    move.q  r8, r28
    pop     r28
    rts

; ============================================================================
; Statement handlers
; ============================================================================

; --- END ---
exec_do_end:
    move.q  r28, #2
    rts
    ; === END exec_do_end ===

; --- STOP ---
exec_do_stop:
    ; Save state for CONT
    add.q   r1, r16, #ST_CONT_LINE_PTR
    store.q r14, (r1)
    add.q   r1, r16, #ST_CONT_TEXT_PTR
    store.q r17, (r1)
    move.q  r28, #2
    rts
    ; === END exec_do_stop ===

; --- CONT ---
exec_do_cont:
    ; Restore saved execution position from STOP
    add.q   r1, r16, #ST_CONT_LINE_PTR
    load.q  r14, (r1)
    beqz    r14, .cont_done      ; no saved state → ignore
    add.q   r1, r16, #ST_CONT_TEXT_PTR
    load.q  r17, (r1)
    ; Clear saved state
    add.q   r1, r16, #ST_CONT_LINE_PTR
    store.l r0, (r1)
    ; Signal exec_run to use updated R14
    move.q  r28, #1
.cont_done:
    rts
    ; === END exec_do_cont ===

; --- REM ---
exec_do_rem:
    ; Skip rest of line (everything after REM is a comment)
    ; Advance R17 to end of line (null terminator)
.rem_skip:
    load.b  r1, (r17)
    beqz    r1, .rem_done
    add.q   r17, r17, #1
    bra     .rem_skip
.rem_done:
    rts
    ; === END exec_do_rem ===

; --- DATA ---
exec_do_data:
    ; Skip rest of DATA statement (skip to colon or EOL)
.data_skip:
    load.b  r1, (r17)
    beqz    r1, .data_done
    move.q  r2, #0x3A            ; ':'
    beq     r1, r2, .data_done
    add.q   r17, r17, #1
    bra     .data_skip
.data_done:
    rts
    ; === END exec_do_data ===

; --- LET ---
exec_do_let:
    jsr     exec_skip_spaces
    ; Parse variable name and get pointer to value slot
    jsr     var_lookup           ; R8 = value slot, R9 = 0(num)/1(str)
    bnez    r28, .let_done
    beqz    r8, .let_done        ; error: no variable name
    move.q  r22, r8              ; R22 = save value slot pointer
    move.q  r23, r9              ; R23 = type flag (0=num, 1=str)

    jsr     exec_skip_spaces

    ; Expect '=' token (TK_EQUAL = 0xBC)
    load.b  r1, (r17)
    move.q  r2, #TK_EQUAL
    beq     r1, r2, .let_has_eq
    ; Also check raw '='
    move.q  r2, #0x3D
    bne     r1, r2, .let_done   ; no '=' → syntax error (silently skip)
.let_has_eq:
    add.q   r17, r17, #1        ; consume '='
    jsr     exec_skip_spaces

    ; Branch on type
    bnez    r23, .let_str_rhs

    ; Numeric RHS.
    jsr     expr_eval            ; R8 = payload, R9 = tag
    bnez    r28, .let_done
    jsr     exec_store_expr_r22
    rts

.let_str_rhs:
    ; String RHS
    jsr     str_eval             ; R8 = pointer to string data
    bnez    r28, .let_done
    store.q r8, (r22)
.let_done:
    rts
    ; === END exec_do_let ===

exec_store_fp64_r22:
    store.q r8, (r22)
exec_clear_qword_marker:
    load.l  r1, -8(r22)
    move.q  r2, #VAL_F64
    beq     r1, r2, .ecqm_scalar
    move.q  r2, #VAL_I64
    bne     r1, r2, .ecqm_done
.ecqm_scalar:
    move.q  r1, #VAL_F64
    store.l r1, -8(r22)
.ecqm_done:
    rts

; Store a tagged numeric expression result into a variable slot. Simple scalar
; variables and array elements preserve VAL_I64 or VAL_F64 via the public cell
; tag at value_slot-8.
; Input: R8=payload, R9=tag, R22=value slot pointer.
exec_store_expr_r22:
    load.l  r1, -8(r22)
    move.q  r2, #VAL_F64
    beq     r1, r2, .eser_store_tagged
    move.q  r2, #VAL_I64
    bne     r1, r2, .eser_store_fp64
.eser_store_tagged:
    store.q r8, (r22)
    store.l r9, -8(r22)
    rts
.eser_store_fp64:
    bra     exec_store_fp64_r22

; Coerce a tagged expression result to FP64 bits for statement internals
; that have not yet moved to tagged frames.
exec_coerce_fp64:
    move.q  r1, #VAL_I64
    bne     r9, r1, .ecf_done
    jsr     fp_float
    move.q  r9, #VAL_F64
.ecf_done:
    rts

; --- PRINT ---
exec_do_print:
    jsr     exec_skip_spaces
    load.b  r1, (r17)

    ; PRINT with nothing → just CRLF
    beqz    r1, .print_crlf
    move.q  r2, #0x3A            ; ':'
    beq     r1, r2, .print_crlf

.print_loop:
    la      r1, if_else_boundary
    load.q  r1, (r1)
    beqz    r1, .print_no_boundary
    bge     r17, r1, .print_crlf
.print_no_boundary:
    load.b  r1, (r17)
    move.q  r2, #0x20
    beq     r1, r2, .print_space
    beqz    r1, .print_crlf      ; end of line → CRLF
    move.q  r2, #0x3A            ; ':'
    beq     r1, r2, .print_crlf

    ; Check for semicolon (suppress CRLF)
    move.q  r2, #0x3B            ; ';'
    beq     r1, r2, .print_semi

    ; Check for comma (tab to next zone)
    move.q  r2, #0x2C            ; ','
    beq     r1, r2, .print_comma

    ; Check for string literal (quote)
    move.q  r2, #0x22            ; '"'
    beq     r1, r2, .print_string

    ; Check for string expression before numeric
    jsr     str_detect           ; R8 = 1 if string, 0 if not
    bnez    r8, .print_str_expr

    ; Numeric: evaluate and print
    jsr     expr_eval            ; R8 = payload, R9 = tag
    bnez    r28, .print_no_crlf
    move.q  r1, #VAL_I64
    beq     r9, r1, .print_i64
    push    r14                  ; fp_print clobbers R14/R15
    push    r15
    jsr     fp_print             ; print the number
    pop     r15
    pop     r14
    bra     .print_loop

.print_i64:
    jsr     io_print_int64
    bra     .print_loop

.print_str_expr:
    jsr     str_eval             ; R8 = pointer to string data
    bnez    r28, .print_no_crlf
    move.q  r3, r8
.print_str_chars:
    load.b  r2, (r3)
    beqz    r2, .print_loop
    push    r3
    move.q  r8, r2
    jsr     term_putc
    pop     r3
    add.q   r3, r3, #1
    bra     .print_str_chars

.print_semi:
    add.q   r17, r17, #1        ; consume ';'
    ; Check if end of line after semicolon → suppress CRLF
    load.b  r1, (r17)
    beqz    r1, .print_no_crlf
    move.q  r2, #0x3A
    beq     r1, r2, .print_no_crlf
    bra     .print_loop

.print_space:
    add.q   r17, r17, #1
    bra     .print_loop

.print_comma:
    add.q   r17, r17, #1        ; consume ','
    ; Print tab (spaces to next tab stop)
    move.q  r8, #0x09            ; TAB character
    jsr     putchar
    bra     .print_loop

.print_string:
    ; Print quoted string literal
    add.q   r17, r17, #1        ; skip opening '"'
.print_str_loop:
    load.b  r1, (r17)
    beqz    r1, .print_no_crlf   ; unterminated string → end
    move.q  r2, #0x22            ; closing '"'
    beq     r1, r2, .print_str_end
    ; Output the character
    move.q  r8, r1
    jsr     term_putc
    add.q   r17, r17, #1
    bra     .print_str_loop
.print_str_end:
    add.q   r17, r17, #1        ; skip closing '"'
    bra     .print_loop

.print_crlf:
    jsr     print_crlf
.print_no_crlf:
    rts
    ; === END exec_do_print ===

; --- GOTO ---
exec_do_goto:
    jsr     exec_skip_spaces
    ; Parse line number (numeric literal)
    jsr     exec_parse_linenum   ; R8 = line number
    ; Search for the target line
    jsr     exec_find_line       ; R14 = new line pointer (or 0)
    beqz    r14, .goto_missing
    move.q  r28, #1              ; signal: R14 was changed
.goto_done:
    rts
.goto_missing:
    move.q  r8, #ERR_UNDEF_LINE
    la      r9, err_msg_undef_line
    jsr     raise_error
    rts
    ; === END exec_do_goto ===

; --- GOSUB ---
exec_do_gosub:
    jsr     exec_skip_spaces
    ; Save return context on GOSUB stack
    ; Stack frame: [marker (4)] [reserved (4)] [return line ptr (8)]
    ;              [return text ptr offset (8)] = 24 bytes.
    add.q   r1, r16, #ST_GOSUB_SP
    load.q  r2, (r1)            ; R2 = current GOSUB stack pointer
    add.q   r3, r2, #24
    add.q   r4, r16, #ST_FOR_SP
    load.q  r4, (r4)
    bgt     r3, r4, .gosub_overflow

    ; Push marker (0x47535542 = "GSUB")
    move.l  r4, #0x47535542
    store.l r4, (r2)
    add.q   r2, r2, #4
    store.l r0, (r2)
    add.q   r2, r2, #4
    ; Push return line pointer (R14 = current line)
    store.q r14, (r2)
    add.q   r2, r2, #8
    ; Push return text offset (R17 - current line = offset within record)
    sub.q   r3, r17, r14
    store.q r3, (r2)
    add.q   r2, r2, #8
    ; Update GOSUB SP
    store.q r2, (r1)

    ; Now do the GOTO part
    jsr     exec_parse_linenum   ; R8 = target line number
    jsr     exec_find_line       ; R14 = target line pointer
    beqz    r14, .gosub_missing
    move.q  r28, #1              ; R14 changed
.gosub_done:
    rts
.gosub_missing:
    move.q  r8, #ERR_UNDEF_LINE
    la      r9, err_msg_undef_line
    jsr     raise_error
    rts
.gosub_overflow:
    move.q  r28, #2
    rts
    ; === END exec_do_gosub ===

; --- RETURN ---
exec_do_return:
    ; Pop GOSUB stack
    add.q   r1, r16, #ST_GOSUB_SP
    load.q  r2, (r1)            ; R2 = GOSUB stack pointer
    add.q   r3, r16, #ST_CTRL_LOW
    load.q  r3, (r3)
    ble     r2, r3, .return_no_gosub

    ; Pop text offset
    sub.q   r2, r2, #8
    load.q  r3, (r2)            ; R3 = return text offset
    ; Pop line pointer
    sub.q   r2, r2, #8
    load.q  r14, (r2)           ; R14 = return line pointer
    ; Pop reserved
    sub.q   r2, r2, #4
    ; Pop marker
    sub.q   r2, r2, #4
    load.l  r4, (r2)            ; R4 = marker (should be "GSUB")
    move.l  r5, #0x47535542
    bne     r4, r5, .return_no_gosub
    ; Update GOSUB SP
    store.q r2, (r1)

    ; Restore R17 to the saved position
    add.q   r17, r14, r3        ; R17 = line ptr + text offset

    ; After RETURN, execution continues on the NEXT statement
    ; The text pointer is pointing just past the GOSUB call's line number
    ; We need to advance to the next statement
    ; Skip to ':' or EOL
.return_skip:
    load.b  r1, (r17)
    beqz    r1, .return_next_line
    move.q  r2, #0x3A
    beq     r1, r2, .return_at_colon
    add.q   r17, r17, #1
    bra     .return_skip

.return_at_colon:
    ; Found colon - R28=0 means continue, caller will see .el_next_stmt
    rts

.return_next_line:
    ; End of the GOSUB's line - advance to next line
    load.q  r15, (r14)           ; R15 = next line pointer from saved line
    move.q  r14, r15
    move.q  r28, #1              ; R14 changed
    rts
.return_no_gosub:
    move.q  r8, #ERR_RET_NO_GOSUB
    la      r9, err_msg_ret_no_gosub
    jsr     raise_error
    rts
    ; === END exec_do_return ===

; --- IF ---
exec_do_if:
    jsr     exec_skip_spaces
    ; Evaluate condition expression
    jsr     expr_eval            ; R8 = condition payload, R9 = tag

    jsr     expr_truthy
    beqz    r8, .if_false
.if_condition_true:

    ; Condition is TRUE - look for THEN and execute what follows
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_THEN
    bne     r1, r2, .if_done     ; no THEN → just continue (R28=0)
    add.q   r17, r17, #1        ; consume THEN

    ; Check if what follows THEN is a line number (GOTO shorthand)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x30            ; '0'
    blt     r1, r2, .if_true_scan_else ; not a digit → execute as statement
    move.q  r2, #0x39            ; '9'
    bgt     r1, r2, .if_true_scan_else ; not a digit → execute as statement
    ; It's a line number - implicit GOTO
    bra     exec_do_goto

.if_true_scan_else:
    move.q  r3, r17
.if_true_else_loop:
    load.b  r1, (r3)
    beqz    r1, .if_done
    move.q  r2, #TK_ELSE
    beq     r1, r2, .if_true_else_found
    add.q   r3, r3, #1
    bra     .if_true_else_loop
.if_true_else_found:
    la      r1, if_else_boundary
    store.q r3, (r1)
    bra     .if_done

.if_false:
    ; Condition is FALSE.
    ; First, skip forward to the THEN token (the one for THEN, not ELSE).
.if_find_then:
    load.b  r1, (r17)
    beqz    r1, .if_done          ; no THEN found → end of line
    move.q  r2, #TK_THEN
    beq     r1, r2, .if_past_then
    add.q   r17, r17, #1
    bra     .if_find_then

.if_past_then:
    add.q   r17, r17, #1         ; consume the THEN token
    ; Now scan for ELSE or end of line. Everything between here and ELSE is
    ; the THEN-clause (skipped).
.if_seek_else:
    load.b  r1, (r17)
    beqz    r1, .if_done          ; no ELSE → done, produce no output
    move.q  r2, #TK_ELSE
    beq     r1, r2, .if_else_found
    add.q   r17, r17, #1
    bra     .if_seek_else

.if_else_found:
    add.q   r17, r17, #1         ; consume ELSE marker (TK_THEN)
    ; R28=0, return to dispatch loop which will continue to .el_next_stmt
.if_done:
    rts
    ; === END exec_do_if ===

; --- ELSE ---
exec_do_else:
    move.q  r8, #ERR_SYNTAX
    la      r9, err_msg_syntax
    jsr     raise_error
    rts

; --- FOR ---
exec_do_for:
    jsr     exec_skip_spaces

    ; Get loop variable
    jsr     var_lookup           ; R8 = pointer to value slot
    beqz    r8, .for_done
    move.q  r22, r8              ; R22 = var value slot pointer

    jsr     exec_skip_spaces

    ; Expect '=' (TK_EQUAL or raw '=')
    load.b  r1, (r17)
    move.q  r2, #TK_EQUAL
    beq     r1, r2, .for_eq
    move.q  r2, #0x3D
    bne     r1, r2, .for_done
.for_eq:
    add.q   r17, r17, #1

    ; Evaluate initial value
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = initial value
    jsr     exec_coerce_fp64
    jsr     exec_store_fp64_r22  ; store in loop variable

    jsr     exec_skip_spaces

    ; Expect TO token
    load.b  r1, (r17)
    move.q  r2, #TK_TO
    bne     r1, r2, .for_done
    add.q   r17, r17, #1

    ; Evaluate limit value
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = limit
    jsr     exec_coerce_fp64
    move.q  r23, r8              ; R23 = limit value

    ; Check for STEP
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_STEP
    bne     r1, r2, .for_default_step

    ; Has STEP
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = step value
    jsr     exec_coerce_fp64
    move.q  r24, r8              ; R24 = step
    bra     .for_push

.for_default_step:
    move.q  r8, #1
    jsr     fp_float             ; default STEP = 1.0 as FP64
    move.q  r24, r8

.for_push:
    ; Push FOR frame on the loop stack.
    ; Frame: [marker (4)] [reserved (4)] [var ptr (8)] [limit FP64 (8)]
    ;        [step FP64 (8)] [line ptr (8)] [text ptr (8)] = 48 bytes.
    add.q   r1, r16, #ST_FOR_SP
    load.q  r2, (r1)
    sub.q   r2, r2, #48
    add.q   r4, r16, #ST_GOSUB_SP
    load.q  r4, (r4)
    blt     r2, r4, .for_overflow

    move.l  r3, #0x464F5220      ; "FOR "
    store.l r3, (r2)
    add.q   r2, r2, #4
    store.l r0, (r2)
    add.q   r2, r2, #4
    store.q r22, (r2)             ; var pointer
    add.q   r2, r2, #8
    store.q r23, (r2)             ; limit
    add.q   r2, r2, #8
    store.q r24, (r2)             ; step
    add.q   r2, r2, #8
    store.q r14, (r2)             ; line pointer (for loop back)
    add.q   r2, r2, #8
    ; Save text ptr: we need the position AFTER the entire FOR statement
    ; R17 currently points past the FOR line. When NEXT loops back,
    ; we advance to the next line (R14's next pointer).
    store.q r17, (r2)

    ; Update loop SP
    sub.q   r2, r2, #40
    store.q r2, (r1)

    ; Continue execution normally (body of loop follows)
.for_done:
    rts
.for_overflow:
    move.q  r28, #2
    rts
    ; === END exec_do_for ===

; --- NEXT ---
exec_do_next:
    ; Find the most recent FOR frame on the loop stack
    add.q   r1, r16, #ST_FOR_SP
    load.q  r2, (r1)            ; R2 = top loop frame
    add.q   r3, r16, #ST_CTRL_HIGH
    load.q  r3, (r3)
    bge     r2, r3, .next_err

    load.l  r3, (r2)            ; should be "FOR " marker
    move.l  r4, #0x464F5220
    bne     r3, r4, .next_err   ; not FOR → error (silently)

    ; Read FOR frame
    add.q   r3, r2, #8
    load.q  r22, (r3)           ; R22 = var pointer
    add.q   r3, r3, #8
    load.q  r23, (r3)           ; R23 = limit (FP64)
    add.q   r3, r3, #8
    load.q  r24, (r3)           ; R24 = step (FP64)
    add.q   r3, r3, #8
    load.q  r25, (r3)           ; R25 = loop line pointer
    add.q   r3, r3, #8
    load.q  r15, (r3)           ; R15 = saved text ptr

    ; Add STEP to loop variable
    load.q  r8, (r22)           ; current value
    move.q  r9, r24             ; step
    push    r22                 ; preserve loop variable pointer across FP helper
    push    r23                 ; preserve loop limit across FP helper
    push    r24                 ; preserve loop step across FP helper
    jsr     fp_add              ; R8 = value + step
    pop     r24
    pop     r23
    pop     r22
    jsr     exec_store_fp64_r22 ; store back

    ; Compare with limit
    ; If step > 0: continue while value <= limit
    ; If step < 0: continue while value >= limit
    move.q  r9, r23             ; R9 = limit
    push    r24                 ; preserve step sign across FP helper
    jsr     fp_cmp              ; R8 = -1/0/1
    pop     r24

    ; Check step sign
    lsr.q   r3, r24, #63        ; R3 = sign bit of step (0=positive, 1=negative)
    bnez    r3, .next_neg_step

    ; Positive step: loop while value <= limit (cmp returns -1 or 0)
    ; R8 = 1 means value > limit → done
    move.q  r3, #1
    beq     r8, r3, .next_done
    bra     .next_loop_back

.next_neg_step:
    ; Negative step: loop while value >= limit (cmp returns 1 or 0)
    ; The fp_cmp less sentinel is the documented low-32 0xFFFFFFFF value.
    move.l  r3, #0xFFFFFFFF
    beq     r8, r3, .next_done

.next_loop_back:
    ; Loop continues: restore execution to the body of the FOR.
    ;
    ; R25 = the FOR statement's BASIC line pointer.
    ; R15 = saved text ptr (the byte immediately after the FOR/TO/STEP
    ;       clause). For one-line FOR bodies this is the ':' before the
    ;       body, so resume in the current exec_line call. For multi-line
    ;       FOR bodies this is the null terminator, so jump the outer
    ;       line walker to the line after the FOR header.
    load.b  r1, (r15)
    beqz    r1, .next_loop_multiline
    move.q  r14, r25            ; current line = FOR line
    move.q  r17, r15            ; text ptr  = body start
    move.q  r28, r0             ; R28 = 0: continue exec_line normally
    rts
.next_loop_multiline:
    load.q  r14, (r25)          ; current line = first body line
    move.q  r28, #1             ; R28 = 1: R14 already updated
    rts

.next_done:
    ; Loop complete: pop FOR frame from loop stack
    add.q   r1, r16, #ST_FOR_SP
    load.q  r2, (r1)
    add.q   r2, r2, #48
    store.q r2, (r1)
    ; R28=0, continue to next statement
    rts
.next_err:
    move.q  r8, #ERR_NEXT_NO_FOR
    la      r9, err_msg_next_no_for
    jsr     raise_error
    rts
    ; === END exec_do_next ===

; --- WHILE ---
exec_do_while:
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = condition
    jsr     expr_truthy
    beqz    r8, .while_false

    ; TRUE: push WHILE frame [marker (4)] [reserved (4)] [line ptr (8)]
    add.q   r1, r16, #ST_FOR_SP
    load.q  r2, (r1)
    sub.q   r2, r2, #16
    add.q   r4, r16, #ST_GOSUB_SP
    load.q  r4, (r4)
    blt     r2, r4, .while_overflow
    move.l  r3, #0x5748494C      ; "WHIL"
    store.l r3, (r2)
    add.q   r2, r2, #4
    store.l r0, (r2)
    add.q   r2, r2, #4
    store.q r14, (r2)            ; line pointer of WHILE
    sub.q   r2, r2, #8
    store.q r2, (r1)
    rts                          ; R28=0, continue

.while_overflow:
    move.q  r28, #2
    rts

.while_false:
    ; FALSE: scan forward to matching WEND (nested - tracks WHILE/WEND depth)
    move.q  r7, #1               ; nesting depth = 1 (we are inside this WHILE)
    load.q  r2, (r14)            ; start from next line
.while_scan_lines:
    beqz    r2, .while_done
    load.q  r3, (r2)             ; next pointer
    beqz    r3, .while_done      ; terminator
    add.q   r4, r2, #16          ; content ptr

    ; skip leading spaces
.while_lead_space:
    load.b  r5, (r4)
    move.q  r6, #0x20
    bne     r5, r6, .while_check_token
    add.q   r4, r4, #1
    bra     .while_lead_space

.while_check_token:
    ; Check for nested WHILE (increases depth)
    move.q  r6, #TK_WHILE
    bne     r5, r6, .while_check_wend
    add.q   r7, r7, #1
    bra     .while_next_line

.while_check_wend:
    move.q  r6, #TK_UNTIL        ; WEND token
    bne     r5, r6, .while_next_line
    sub.q   r7, r7, #1
    bnez    r7, .while_next_line  ; not our WEND yet

    ; depth=0 → found matching WEND. Continue after this line.
    load.q  r14, (r2)
    move.q  r28, #1
    rts

.while_next_line:
    move.q  r2, r3
    bra     .while_scan_lines
.while_done:
    rts
    ; === END exec_do_while ===

; --- WEND ---
exec_do_wend:
    ; Pop latest WHILE frame and jump back to WHILE line.
    add.q   r1, r16, #ST_FOR_SP
    load.q  r2, (r1)
    add.q   r3, r16, #ST_CTRL_HIGH
    load.q  r3, (r3)
    bge     r2, r3, .wend_done
    load.l  r3, (r2)             ; marker
    move.l  r4, #0x5748494C      ; "WHIL"
    bne     r3, r4, .wend_done
    add.q   r5, r2, #8
    load.q  r14, (r5)            ; WHILE line pointer
    add.q   r2, r2, #16
    store.q r2, (r1)             ; pop frame
    move.q  r28, #1
.wend_done:
    rts
    ; === END exec_do_wend ===

; --- RESTORE ---
exec_do_restore:
    add.q   r1, r16, #ST_DATA_PTR
    store.q r0, (r1)            ; reset DATA pointer to 0
    rts
    ; === END exec_do_restore ===

; --- INC ---
exec_do_inc:
    jsr     exec_skip_spaces
    jsr     var_lookup           ; R8 = pointer to value slot
    beqz    r8, .inc_done
    move.q  r22, r8
    load.l  r1, -8(r22)
    move.q  r2, #VAL_I64
    bne     r1, r2, .inc_fp64
    load.q  r8, (r22)
    add.q   r8, r8, #1
    store.q r8, (r22)
    bra     .inc_done
.inc_fp64:
    load.q  r8, (r22)           ; current FP64 value
    push    r8
    move.q  r8, #1
    jsr     fp_float
    move.q  r9, r8
    pop     r8
    jsr     fp_add
    jsr     exec_store_fp64_r22
.inc_done:
    rts
    ; === END exec_do_inc ===

; --- DEC ---
exec_do_dec:
    jsr     exec_skip_spaces
    jsr     var_lookup           ; R8 = pointer to value slot
    beqz    r8, .dec_done
    move.q  r22, r8
    load.l  r1, -8(r22)
    move.q  r2, #VAL_I64
    bne     r1, r2, .dec_fp64
    load.q  r8, (r22)
    sub.q   r8, r8, #1
    store.q r8, (r22)
    bra     .dec_done
.dec_fp64:
    load.q  r8, (r22)           ; current FP64 value
    push    r8
    move.q  r8, #1
    jsr     fp_float
    move.q  r9, r8
    pop     r8
    jsr     fp_sub
    jsr     exec_store_fp64_r22
.dec_done:
    rts
    ; === END exec_do_dec ===

; --- CLEAR ---
exec_do_clear:
    jsr     var_init             ; reset all variables
    add.q   r1, r16, #ST_PROG_END
    load.q  r2, (r1)
    add.q   r1, r16, #ST_HEAP_BOTTOM
    store.q r2, (r1)
    jsr     basic_memalloc_reset
    ; Reset DATA pointer
    add.q   r1, r16, #ST_DATA_PTR
    store.q r0, (r1)
    ; Reset control-flow stacks
    jsr     exec_reset_control_stack
    rts
    ; === END exec_do_clear ===

; --- RUN ---
exec_do_run:
    ; Restart stored program from the first line. Variables are preserved, while
    ; control/data stacks reset like a fresh exec_run.
    add.q   r1, r16, #ST_DIRECT_MODE
    store.l r0, (r1)
    add.q   r1, r16, #ST_DATA_PTR
    store.q r0, (r1)
    jsr     exec_reset_control_stack
    load.q  r14, (r16)
    move.q  r28, #1
    rts

; --- LIST ---
exec_do_list:
    push    r14
    move.q  r8, r0
    move.q  r9, #0xFFFFFF
    jsr     line_list
    pop     r14
    rts

; --- NEW ---
exec_do_new:
    jsr     line_new
    jsr     var_init
    jsr     basic_memalloc_reset
    add.q   r1, r16, #ST_DATA_PTR
    store.q r0, (r1)
    jsr     exec_reset_control_stack
    move.q  r28, #2
    rts

; --- SWAP ---
exec_do_swap:
    jsr     exec_skip_spaces
    jsr     var_lookup           ; R8 = ptr to first var
    beqz    r8, .swap_done
    move.q  r22, r8              ; R22 = first var ptr
    move.q  r24, r9              ; R24 = first var type flag
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .swap_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     var_lookup           ; R8 = ptr to second var
    beqz    r8, .swap_done
    move.q  r23, r8              ; R23 = second var ptr
    move.q  r25, r9              ; R25 = second var type flag
    ; Scalar numeric variables carry a public value tag immediately before the
    ; value payload. String variables carry a full 64-bit internal pointer.
    bnez    r24, .swap_first_string
    bnez    r25, .swap_low32_no_marker
    load.l  r1, -8(r22)
    load.l  r2, -8(r23)
    move.q  r3, #VAL_F64
    beq     r1, r3, .swap_first_scalar
    move.q  r3, #VAL_I64
    bne     r1, r3, .swap_low32
.swap_first_scalar:
    move.q  r3, #VAL_F64
    beq     r2, r3, .swap_both_scalar
    move.q  r3, #VAL_I64
    bne     r2, r3, .swap_low32
.swap_both_scalar:
    load.q  r4, (r22)
    load.q  r5, (r23)
    store.l r2, -8(r22)
    store.l r1, -8(r23)
    store.q r5, (r22)
    store.q r4, (r23)
    bra     .swap_done
.swap_low32:
    load.l  r1, (r22)
    load.l  r2, (r23)
    store.l r2, (r22)
    store.l r1, (r23)
    jsr     exec_clear_qword_marker
    move.q  r22, r23
    jsr     exec_clear_qword_marker
    bra     .swap_done
.swap_first_string:
    beqz    r25, .swap_low32_no_marker
    load.q  r1, (r22)
    load.q  r2, (r23)
    store.q r2, (r22)
    store.q r1, (r23)
    bra     .swap_done
.swap_low32_no_marker:
    load.l  r1, (r22)
    load.l  r2, (r23)
    store.l r2, (r22)
    store.l r1, (r23)
.swap_done:
    rts
    ; === END exec_do_swap ===

; --- BITSET ---
exec_do_bitset:
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = address
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = address
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .bitset_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = bit number (0-7)
    jsr     fp_int
    jsr     fp_fix
    ; Create mask: 1 << bit
    move.q  r1, #1
    lsl.l   r1, r1, r8
    ; Set bit
    load.b  r2, (r22)
    or.l    r2, r2, r1
    store.b r2, (r22)
.bitset_done:
    rts
    ; === END exec_do_bitset ===

; --- BITCLR ---
exec_do_bitclr:
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = address
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = address
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .bitclr_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = bit number (0-7)
    jsr     fp_int
    jsr     fp_fix
    ; Create mask: ~(1 << bit)
    move.q  r1, #1
    lsl.l   r1, r1, r8
    move.q  r3, #0xFF
    eor.l   r1, r1, r3           ; invert within byte
    ; Clear bit
    load.b  r2, (r22)
    and.l   r2, r2, r1
    store.b r2, (r22)
.bitclr_done:
    rts
    ; === END exec_do_bitclr ===

; --- GET ---
exec_do_get:
    jsr     exec_skip_spaces
    jsr     var_lookup           ; R8 = ptr to value slot, R9 = type
    beqz    r8, .get_done
    move.q  r22, r8              ; R22 = value slot
    move.q  r23, r9              ; R23 = type (0=num, 1=str)
    ; Check if key available
    la      r1, TERM_KEY_STATUS
    load.l  r1, (r1)
    and.l   r1, r1, #1          ; bit 0 = char available
    beqz    r1, .get_empty
    ; Read char
    la      r3, TERM_KEY_IN
    load.b  r8, (r3)
    and.l   r8, r8, #0xFF
    ; If string var, store as single-char string
    bnez    r23, .get_str
    ; Numeric: store ASCII code as FP64
    jsr     fp_float
    jsr     exec_store_fp64_r22
    rts
.get_str:
    ; Allocate a 2-byte string (char + null)
    push    r8
    move.q  r8, #1
    jsr     str_alloc
    move.q  r3, r8
    pop     r8
    beqz    r3, .get_done
    store.b r8, (r3)             ; the character
    add.q   r4, r3, #1
    store.b r0, (r4)             ; null terminator
    store.q r3, (r22)            ; store pointer in var
    rts
.get_empty:
    ; No char available: store 0 (num) or "" (str)
    bnez    r23, .get_empty_str
    store.l r0, (r22)            ; 0
    jsr     exec_clear_qword_marker
    rts
.get_empty_str:
    move.q  r8, r0
    jsr     str_alloc
    move.q  r3, r8
    beqz    r3, .get_done
    store.b r0, (r3)             ; empty string (just null)
    store.q r3, (r22)
.get_done:
    rts
    ; === END exec_do_get ===

; ============================================================================
; exec_reset_control_stack - reset dynamic control-flow stack cursors
; ============================================================================
; Uses ST_CTRL_LOW/ST_CTRL_HIGH when already initialised. Older focused harnesses
; may enter before those bounds are populated, so seed a reduced dynamic low32
; control stack instead of the retired fixed BASIC stack symbols.
; Input:  R16 = state base
; Clobbers: R1-R4

exec_reset_control_stack:
    add.q   r1, r16, #ST_CTRL_LOW
    load.q  r2, (r1)
    bnez    r2, .ctrl_low_ready
    move.q  r2, #(BASIC_LOW32_INTERNAL_BASE + 0x00800000 - BASIC_CTRL_BYTES)
    store.q r2, (r1)
.ctrl_low_ready:
    add.q   r3, r16, #ST_CTRL_HIGH
    load.q  r4, (r3)
    bnez    r4, .ctrl_high_ready
    move.q  r4, #(BASIC_LOW32_INTERNAL_BASE + 0x00800000)
    store.q r4, (r3)
.ctrl_high_ready:
    add.q   r1, r16, #ST_GOSUB_SP
    store.q r2, (r1)
    add.q   r1, r16, #ST_FOR_SP
    store.q r4, (r1)
    rts

; --- ON ---
exec_do_on:
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = selector (FP64)
    jsr     fp_int
    jsr     fp_fix               ; R8 = integer selector (1-based)
    move.q  r22, r8              ; R22 = selector
    jsr     exec_skip_spaces
    ; Check for GOTO or GOSUB token
    load.b  r1, (r17)
    move.q  r2, #TK_GOTO
    beq     r1, r2, .on_goto
    move.q  r2, #TK_GOSUB
    beq     r1, r2, .on_gosub
    ; Neither → error
    rts
.on_goto:
    add.q   r17, r17, #1         ; consume GOTO token
    jsr     exec_skip_spaces
    move.q  r23, #1              ; R23 = current count
.on_goto_scan:
    beq     r23, r22, .on_goto_target
    jsr     exec_skip_number
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C            ; ','
    bne     r1, r2, .on_done     ; out of range → fall through
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    add.q   r23, r23, #1
    bra     .on_goto_scan
.on_goto_target:
    bra     exec_do_goto         ; parse line number and GOTO
.on_gosub:
    add.q   r17, r17, #1         ; consume GOSUB token
    jsr     exec_skip_spaces
    move.q  r23, #1
.on_gosub_scan:
    beq     r23, r22, .on_gosub_target
    jsr     exec_skip_number
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .on_done     ; out of range → fall through
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    add.q   r23, r23, #1
    bra     .on_gosub_scan
.on_gosub_target:
    bra     exec_do_gosub        ; parse line number and GOSUB
.on_done:
    rts
    ; === END exec_do_on ===

; --- DO ---
exec_do_do:
    ; Push DO frame on loop stack: [marker (4)] [reserved (4)] [line ptr (8)]
    add.q   r1, r16, #ST_FOR_SP
    load.q  r2, (r1)
    sub.q   r2, r2, #16
    add.q   r4, r16, #ST_GOSUB_SP
    load.q  r4, (r4)
    blt     r2, r4, .do_overflow
    move.l  r3, #0x444F2020      ; "DO  "
    store.l r3, (r2)
    add.q   r2, r2, #4
    store.l r0, (r2)
    add.q   r2, r2, #4
    store.q r14, (r2)            ; line pointer of DO statement
    sub.q   r2, r2, #8
    store.q r2, (r1)             ; update SP
    rts
.do_overflow:
    move.q  r28, #2
    rts
    ; === END exec_do_do ===

; --- LOOP ---
exec_do_loop:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_WHILE
    beq     r1, r2, .loop_while
    move.q  r2, #TK_UNTIL
    beq     r1, r2, .loop_until
    ; Bare LOOP - unconditional loop back to DO
    bra     .loop_back
.loop_while:
    add.q   r17, r17, #1         ; consume WHILE
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = condition
    jsr     expr_truthy
    beqz    r8, .loop_exit       ; false → exit
    bra     .loop_back           ; true → loop
.loop_until:
    add.q   r17, r17, #1         ; consume UNTIL
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = condition
    jsr     expr_truthy
    bnez    r8, .loop_exit       ; true → exit
    ; false → loop
.loop_back:
    ; Pop DO frame, jump back to DO line (which re-pushes)
    add.q   r1, r16, #ST_FOR_SP
    load.q  r2, (r1)
    add.q   r3, r16, #ST_CTRL_HIGH
    load.q  r3, (r3)
    bge     r2, r3, .loop_done
    load.l  r3, (r2)             ; marker
    move.l  r4, #0x444F2020
    bne     r3, r4, .loop_done   ; not DO → error
    add.q   r5, r2, #8
    load.q  r14, (r5)            ; DO line pointer
    add.q   r2, r2, #16
    store.q r2, (r1)             ; pop frame
    move.q  r28, #1              ; R14 changed
    rts
.loop_exit:
    ; Pop DO frame and continue
    add.q   r1, r16, #ST_FOR_SP
    load.q  r2, (r1)
    add.q   r2, r2, #16
    store.q r2, (r1)
.loop_done:
    rts
    ; === END exec_do_loop ===

; --- INPUT ---
exec_do_input:
    ; INPUT ["prompt";] var[,var...]
    ; Reads one line per variable from terminal.
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x22            ; optional quoted prompt
    bne     r1, r2, .input_loop
    add.q   r17, r17, #1         ; consume opening quote
.input_prompt_loop:
    load.b  r1, (r17)
    beqz    r1, .input_done
    move.q  r2, #0x22
    beq     r1, r2, .input_prompt_end
    move.q  r8, r1
    jsr     putchar
    add.q   r17, r17, #1
    bra     .input_prompt_loop
.input_prompt_end:
    add.q   r17, r17, #1         ; consume closing quote
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x3B            ; ';'
    beq     r1, r2, .input_prompt_sep
    move.q  r2, #0x2C            ; ','
    bne     r1, r2, .input_loop
.input_prompt_sep:
    add.q   r17, r17, #1
.input_loop:
    jsr     exec_skip_spaces
    jsr     var_lookup           ; R8 = value slot pointer, R9 = type
    beqz    r8, .input_done
    move.q  r22, r8              ; save value slot pointer
    move.q  r24, r9              ; save type flag
    move.q  r23, r17             ; save statement cursor

    ; Read line into the BASIC-owned dynamic line buffer.
    add.q   r8, r16, #ST_LINE_BUF_PTR
    load.q  r8, (r8)
    move.q  r9, #80
    jsr     read_line
    bnez    r24, .input_store_string

    ; Numeric variable: parse line as a tagged expression.
    add.q   r17, r16, #ST_LINE_BUF_PTR
    load.q  r17, (r17)
    jsr     expr_eval            ; R8 = payload, R9 = tag
    jsr     exec_store_expr_r22
    bra     .input_after_store

.input_store_string:
    add.q   r8, r16, #ST_LINE_BUF_PTR
    load.q  r8, (r8)
    jsr     str_len
    jsr     str_alloc
    bnez    r28, .input_done
    beqz    r8, .input_done
    move.q  r3, r8               ; allocated string pointer
    add.q   r9, r16, #ST_LINE_BUF_PTR
    load.q  r9, (r9)
    jsr     str_copy
    store.q r3, (r22)

    ; Restore statement cursor and continue list parsing
.input_after_store:
    move.q  r17, r23
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C            ; ','
    bne     r1, r2, .input_done
    add.q   r17, r17, #1
    bra     .input_loop
.input_done:
    rts
    ; === END exec_do_input ===

; --- DIM ---
exec_do_dim:
    bra     arr_dim
    ; === END exec_do_dim ===

; --- POKE ---
; POKE addr, value  - byte alias for POKE8.
; This is an intentional historical nod to 8-bit home computer BASICs:
; bare POKE writes one byte, and wider stores must use POKE16/32/64.
exec_do_poke:
    move.q  r23, #1              ; byte width
    bra     exec_do_poke_width

; R23 = width in bytes (1, 2, 4, or 8)
exec_do_poke_width:
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = address payload, R9 = tag
    bnez    r28, .poke_done
    move.q  r1, #VAL_I64
    beq     r9, r1, .poke_addr_i64
    jsr     fp_int               ; R8 = INT(addr) FP64
    jsr     fp_fix               ; R8 = integer
.poke_addr_i64:
    move.q  r22, r8              ; R22 = address
    move.q  r1, #1
    beq     r23, r1, .poke_addr_ok
    move.q  r1, #2
    bne     r23, r1, .poke_check32
    and.l   r1, r22, #1
    bnez    r1, .poke_fc_error
    bra     .poke_addr_ok
.poke_check32:
    move.q  r1, #4
    bne     r23, r1, .poke_check64
    and.l   r1, r22, #3
    bnez    r1, .poke_fc_error
    bra     .poke_addr_ok
.poke_check64:
    and.l   r1, r22, #7
    bnez    r1, .poke_fc_error
.poke_addr_ok:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C            ; ','
    bne     r1, r2, .poke_done
    add.q   r17, r17, #1         ; consume ','
    jsr     exec_skip_spaces
    move.q  r1, #8
    bne     r23, r1, .poke_eval_value
    move.q  r24, r17             ; save value cursor in case this is not an I64 expression
    jsr     exec_eval_i64
    beqz    r9, .poke_restore_eval_value
    move.q  r25, r8
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    beqz    r1, .poke_restore_qword
    move.q  r2, #0x3A            ; ':'
    beq     r1, r2, .poke_restore_qword
.poke_restore_eval_value:
    move.q  r17, r24
    bra     .poke_eval_value
.poke_restore_qword:
    move.q  r8, r25
    bra     .poke_store_qword
.poke_eval_value:
    jsr     expr_eval            ; R8 = value payload, R9 = tag
    bnez    r28, .poke_done
    move.q  r1, #VAL_I64
    beq     r9, r1, .poke_value_i64
    jsr     fp_int               ; R8 = INT(value) FP64
    jsr     fp_fix               ; R8 = integer
.poke_value_i64:
    move.q  r1, #1
    beq     r23, r1, .poke_8bit
    move.q  r1, #2
    beq     r23, r1, .poke_16bit
    move.q  r1, #4
    beq     r23, r1, .poke_32bit
.poke_store_qword:
    store.q r8, (r22)            ; write 64-bit to address
    rts
.poke_8bit:
    store.b r8, (r22)            ; write byte to address
    rts
.poke_16bit:
    store.w r8, (r22)            ; write 16-bit to address
    rts
.poke_32bit:
    store.l r8, (r22)            ; write 32-bit to address
.poke_done:
    rts
.poke_fc_error:
    move.q  r8, #ERR_FC
    la      r9, err_msg_fc
    jsr     raise_error
    rts
    ; === END exec_do_poke ===

; Parse a plain &H hex literal into a full 64-bit payload.
; Input:  R17 = '&'
; Output: R8 = qword value, R9 = 1 on success, 0 on malformed/no digits.
;         R17 advanced past the literal on success; unspecified on failure.
; Clobbers: R1-R7
exec_parse_hex_u64:
    move.q  r9, r0
    load.b  r1, (r17)
    move.q  r2, #0x26            ; '&'
    bne     r1, r2, .eph_fail
    add.q   r17, r17, #1
    load.b  r1, (r17)
    or.q    r1, r1, #0x20        ; normalize H/h
    move.q  r2, #0x68            ; 'h'
    bne     r1, r2, .eph_fail
.eph_start:
    add.q   r17, r17, #1
    move.q  r8, r0
    move.q  r7, r0               ; digit count
.eph_loop:
    load.b  r1, (r17)
    move.q  r2, #0x30
    blt     r1, r2, .eph_done
    move.q  r2, #0x39
    ble     r1, r2, .eph_digit_09
    move.q  r2, #0x41
    blt     r1, r2, .eph_done
    move.q  r2, #0x46
    ble     r1, r2, .eph_digit_af_upper
    move.q  r2, #0x61
    blt     r1, r2, .eph_done
    move.q  r2, #0x66
    ble     r1, r2, .eph_digit_af_lower
    bra     .eph_done
.eph_digit_09:
    sub.q   r1, r1, #0x30
    bra     .eph_accum
.eph_digit_af_upper:
    sub.q   r1, r1, #0x37
    bra     .eph_accum
.eph_digit_af_lower:
    sub.q   r1, r1, #0x57
.eph_accum:
    move.q  r2, #16
    bge     r7, r2, .eph_fail
    lsl.q   r8, r8, #4
    or.q    r8, r8, r1
    add.q   r7, r7, #1
    add.q   r17, r17, #1
    bra     .eph_loop
.eph_done:
    beqz    r7, .eph_fail
    move.q  r9, #1
    rts
.eph_fail:
    move.q  r9, r0
    rts

; Evaluate an integer-compatible expression into a full qword.
; This is a scoped bridge for POKE64 while the main expression engine is still
; FP64. It supports integer literals, simple numeric variables, and tokenised
; + and - operators.
; Input:  R17 = expression cursor
; Output: R8 = qword payload, R9 = 1 on success, 0 on unsupported syntax.
;         R17 advanced past the parsed expression on success.
; Clobbers: R1-R15. R22 is preserved for statement callers.
exec_eval_i64:
    push    r22
    push    r23
    jsr     exec_i64_or
    pop     r23
    pop     r22
    rts

exec_i64_or:
    jsr     exec_i64_and
    beqz    r9, .ei64_or_done
    move.q  r22, r8
.ei64_or_loop:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_OR
    beq     r1, r2, .ei64_do_or
    move.q  r2, #TK_EOR
    beq     r1, r2, .ei64_do_eor
    move.q  r8, r22
    move.q  r9, #1
.ei64_or_done:
    rts
.ei64_do_or:
    add.q   r17, r17, #1
    push    r22
    jsr     exec_i64_and
    pop     r22
    beqz    r9, .ei64_or_done
    or.q    r22, r22, r8
    bra     .ei64_or_loop
.ei64_do_eor:
    add.q   r17, r17, #1
    push    r22
    jsr     exec_i64_and
    pop     r22
    beqz    r9, .ei64_or_done
    eor.q   r22, r22, r8
    bra     .ei64_or_loop

exec_i64_and:
    jsr     exec_i64_not
    beqz    r9, .ei64_and_done
    move.q  r22, r8
.ei64_and_loop:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_AND
    bne     r1, r2, .ei64_and_return
    add.q   r17, r17, #1
    push    r22
    jsr     exec_i64_not
    pop     r22
    beqz    r9, .ei64_and_done
    and.q   r22, r22, r8
    bra     .ei64_and_loop
.ei64_and_return:
    move.q  r8, r22
    move.q  r9, #1
.ei64_and_done:
    rts

exec_i64_not:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_NOT
    bne     r1, r2, exec_i64_compare
    add.q   r17, r17, #1
    jsr     exec_i64_not
    beqz    r9, .ei64_not_done
    not.q   r8, r8
    move.q  r9, #1
.ei64_not_done:
    rts

exec_i64_compare:
    jsr     exec_i64_add
    beqz    r9, .ei64_cmp_done
    move.q  r22, r8
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_LT
    beq     r1, r2, .ei64_cmp_lt
    move.q  r2, #TK_GT
    beq     r1, r2, .ei64_cmp_gt
    move.q  r2, #TK_EQUAL
    beq     r1, r2, .ei64_cmp_eq
    move.q  r8, r22
    move.q  r9, #1
.ei64_cmp_done:
    rts
.ei64_cmp_lt:
    move.q  r23, r0              ; <
    add.q   r17, r17, #1
    load.b  r1, (r17)
    move.q  r2, #0x3D
    beq     r1, r2, .ei64_cmp_set_le
    move.q  r2, #TK_EQUAL
    beq     r1, r2, .ei64_cmp_set_le
    move.q  r2, #0x3E
    beq     r1, r2, .ei64_cmp_set_ne
    move.q  r2, #TK_GT
    beq     r1, r2, .ei64_cmp_set_ne
    bra     .ei64_cmp_rhs
.ei64_cmp_set_le:
    move.q  r23, #1              ; <=
    add.q   r17, r17, #1
    bra     .ei64_cmp_rhs
.ei64_cmp_set_ne:
    move.q  r23, #2              ; <>
    add.q   r17, r17, #1
    bra     .ei64_cmp_rhs
.ei64_cmp_gt:
    move.q  r23, #3              ; >
    add.q   r17, r17, #1
    load.b  r1, (r17)
    move.q  r2, #0x3D
    beq     r1, r2, .ei64_cmp_set_ge
    move.q  r2, #TK_EQUAL
    beq     r1, r2, .ei64_cmp_set_ge
    bra     .ei64_cmp_rhs
.ei64_cmp_set_ge:
    move.q  r23, #4              ; >=
    add.q   r17, r17, #1
    bra     .ei64_cmp_rhs
.ei64_cmp_eq:
    move.q  r23, #5              ; =
    add.q   r17, r17, #1
.ei64_cmp_rhs:
    push    r22
    push    r23
    jsr     exec_i64_add
    pop     r23
    pop     r22
    beqz    r9, .ei64_cmp_done
    move.q  r1, r0               ; false
    move.q  r2, #5
    beq     r23, r2, .ei64_cmp_do_eq
    move.q  r2, #4
    beq     r23, r2, .ei64_cmp_do_ge
    move.q  r2, #3
    beq     r23, r2, .ei64_cmp_do_gt
    move.q  r2, #2
    beq     r23, r2, .ei64_cmp_do_ne
    move.q  r2, #1
    beq     r23, r2, .ei64_cmp_do_le
    blt     r22, r8, .ei64_cmp_true
    bra     .ei64_cmp_result
.ei64_cmp_do_le:
    blt     r22, r8, .ei64_cmp_true
    beq     r22, r8, .ei64_cmp_true
    bra     .ei64_cmp_result
.ei64_cmp_do_ne:
    bne     r22, r8, .ei64_cmp_true
    bra     .ei64_cmp_result
.ei64_cmp_do_gt:
    bgt     r22, r8, .ei64_cmp_true
    bra     .ei64_cmp_result
.ei64_cmp_do_ge:
    bgt     r22, r8, .ei64_cmp_true
    beq     r22, r8, .ei64_cmp_true
    bra     .ei64_cmp_result
.ei64_cmp_do_eq:
    beq     r22, r8, .ei64_cmp_true
    bra     .ei64_cmp_result
.ei64_cmp_true:
    move.q  r1, r0
    not.q   r1, r1
.ei64_cmp_result:
    move.q  r8, r1
    move.q  r9, #1
    rts

exec_i64_add:
    jsr     exec_i64_mul
    beqz    r9, .ei64_add_done
    move.q  r22, r8
.ei64_add_loop:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_PLUS
    beq     r1, r2, .ei64_do_add
    move.q  r2, #TK_MINUS
    beq     r1, r2, .ei64_do_sub
    move.q  r2, #TK_LSHIFT
    beq     r1, r2, .ei64_do_lshift
    move.q  r2, #TK_RSHIFT
    beq     r1, r2, .ei64_do_rshift
    move.q  r8, r22
    move.q  r9, #1
.ei64_add_done:
    rts
.ei64_do_add:
    add.q   r17, r17, #1
    push    r22
    jsr     exec_i64_mul
    pop     r22
    beqz    r9, .ei64_add_done
    add.q   r22, r22, r8
    bra     .ei64_add_loop
.ei64_do_sub:
    add.q   r17, r17, #1
    push    r22
    jsr     exec_i64_mul
    pop     r22
    beqz    r9, .ei64_add_done
    sub.q   r22, r22, r8
    bra     .ei64_add_loop
.ei64_do_lshift:
    add.q   r17, r17, #1
    push    r22
    jsr     exec_i64_mul
    pop     r22
    beqz    r9, .ei64_add_done
    and.q   r8, r8, #63
    lsl.q   r22, r22, r8
    bra     .ei64_add_loop
.ei64_do_rshift:
    add.q   r17, r17, #1
    push    r22
    jsr     exec_i64_mul
    pop     r22
    beqz    r9, .ei64_add_done
    and.q   r8, r8, #63
    lsr.q   r22, r22, r8
    bra     .ei64_add_loop

exec_i64_mul:
    jsr     exec_i64_primary
    beqz    r9, .ei64_mul_done
    move.q  r22, r8
.ei64_mul_loop:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_MULT
    bne     r1, r2, .ei64_mul_return
    add.q   r17, r17, #1
    push    r22
    jsr     exec_i64_primary
    pop     r22
    beqz    r9, .ei64_mul_done
    mulu.q  r22, r22, r8
    bra     .ei64_mul_loop
.ei64_mul_return:
    move.q  r8, r22
    move.q  r9, #1
.ei64_mul_done:
    rts

exec_i64_primary:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_MINUS
    beq     r1, r2, .ei64_unary_minus
    move.q  r2, #0x2D            ; '-'
    beq     r1, r2, .ei64_unary_minus
    move.q  r2, #TK_PLUS
    beq     r1, r2, .ei64_unary_plus
    move.q  r2, #0x2B            ; '+'
    beq     r1, r2, .ei64_unary_plus
    move.q  r2, #TK_EXT
    beq     r1, r2, .ei64_ext
    move.q  r2, #TK_VAL
    beq     r1, r2, .ei64_fn_val
    move.q  r2, #TK_INT
    beq     r1, r2, .ei64_fn_int
    move.q  r2, #TK_ABS
    beq     r1, r2, .ei64_fn_abs
    move.q  r2, #TK_SGN
    beq     r1, r2, .ei64_fn_sgn
    move.q  r2, #0x28            ; '('
    beq     r1, r2, .ei64_paren
    move.q  r2, #0x26            ; '&'
    beq     r1, r2, .ei64_hex
    move.q  r2, #0x30
    blt     r1, r2, .ei64_not_decimal
    move.q  r2, #0x39
    ble     r1, r2, .ei64_decimal
.ei64_not_decimal:
    move.q  r2, #0x41
    blt     r1, r2, .ei64_fail
    bra     .ei64_var            ; var_parse_name validates A-Z/a-z
.ei64_ext:
    add.q   r17, r17, #1         ; consume TK_EXT
    load.b  r1, (r17)
    move.q  r2, #EXT_PEEK64
    bne     r1, r2, .ei64_fail
    add.q   r17, r17, #1         ; consume EXT_PEEK64
    bra     exec_i64_peek64
.ei64_hex:
    bra     exec_parse_hex_u64
.ei64_decimal:
    bra     exec_parse_dec_u64
.ei64_unary_minus:
    add.q   r17, r17, #1
    jsr     exec_i64_primary
    beqz    r9, .ei64_fail
    neg.q   r8, r8
    move.q  r9, #1
    rts
.ei64_unary_plus:
    add.q   r17, r17, #1
    bra     exec_i64_primary
.ei64_fn_val:
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x28             ; '('
    bne     r1, r2, .ei64_fail
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x22             ; '"'
    bne     r1, r2, .ei64_fail
    add.q   r17, r17, #1
    move.q  r8, r17
    jsr     val_parse_i64_string
    beqz    r9, .ei64_fail
.ei64_fn_val_seek_quote:
    load.b  r1, (r17)
    beqz    r1, .ei64_fail
    move.q  r2, #0x22
    beq     r1, r2, .ei64_fn_val_quote
    add.q   r17, r17, #1
    bra     .ei64_fn_val_seek_quote
.ei64_fn_val_quote:
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x29             ; ')'
    bne     r1, r2, .ei64_fail
    add.q   r17, r17, #1
    move.q  r9, #1
    rts
.ei64_fn_int:
    add.q   r17, r17, #1
    jsr     exec_i64_call_arg
    beqz    r9, .ei64_fail
    move.q  r9, #1
    rts
.ei64_fn_abs:
    add.q   r17, r17, #1
    jsr     exec_i64_call_arg
    beqz    r9, .ei64_fail
    bgez    r8, .ei64_fn_abs_done
    neg.q   r8, r8
.ei64_fn_abs_done:
    move.q  r9, #1
    rts
.ei64_fn_sgn:
    add.q   r17, r17, #1
    jsr     exec_i64_call_arg
    beqz    r9, .ei64_fail
    beqz    r8, .ei64_fn_sgn_zero
    bgez    r8, .ei64_fn_sgn_pos
    move.q  r8, r0
    not.q   r8, r8
    move.q  r9, #1
    rts
.ei64_fn_sgn_pos:
    move.q  r8, #1
    move.q  r9, #1
    rts
.ei64_fn_sgn_zero:
    move.q  r8, r0
    move.q  r9, #1
    rts
.ei64_paren:
    add.q   r17, r17, #1
    jsr     exec_i64_or
    beqz    r9, .ei64_fail
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x29            ; ')'
    bne     r1, r2, .ei64_fail
    add.q   r17, r17, #1
    move.q  r9, #1
    rts
.ei64_var:
    jsr     var_parse_name       ; R14=name, R15=count
    beqz    r15, .ei64_fail
    jsr     var_lookup_tag
    beqz    r8, .ei64_fail
    move.q  r11, r8
    move.q  r1, r8
    sub.q   r1, r1, #8
    load.l  r1, (r1)
    move.q  r2, #VAL_I64
    bne     r1, r2, .ei64_fail
.ei64_var_qword:
    load.q  r8, (r11)
    move.q  r9, #1
.ei64_primary_done:
    rts
.ei64_fail:
    move.q  r9, r0
    rts

exec_i64_call_arg:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x28
    bne     r1, r2, .ei64_call_fail
    add.q   r17, r17, #1
    jsr     exec_i64_or
    beqz    r9, .ei64_call_fail
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x29
    bne     r1, r2, .ei64_call_fail
    add.q   r17, r17, #1
    move.q  r9, #1
    rts
.ei64_call_fail:
    move.q  r9, r0
    rts

; Evaluate PEEK64(address) as a qword primary for POKE64 value expressions.
; General BASIC expressions are still FP64; this bridge preserves qword memory
; copies such as POKE64 dst, PEEK64(src).
exec_i64_peek64:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x28            ; '('
    bne     r1, r2, .ei64_peek64_fail
    add.q   r17, r17, #1
    jsr     expr_eval            ; R8 = address payload, R9 = tag
    bnez    r28, .ei64_peek64_fail
    load.b  r1, (r17)
    move.q  r2, #0x29            ; ')'
    bne     r1, r2, .ei64_peek64_fail
    add.q   r17, r17, #1
    move.q  r1, #VAL_I64
    beq     r9, r1, .ei64_peek64_addr_i64
    jsr     fp_int
    jsr     fp_fix               ; R8 = integer address
.ei64_peek64_addr_i64:
    and.l   r1, r8, #7
    bnez    r1, .ei64_peek64_fc
    load.q  r8, (r8)
    move.q  r9, #1
    rts
.ei64_peek64_fc:
    move.q  r8, #ERR_FC
    la      r9, err_msg_fc
    jsr     raise_error
.ei64_peek64_fail:
    move.q  r9, r0
    rts

; Parse an unsigned decimal integer into a qword. Overflow wraps; callers only
; use this for integer-compatible qword payloads.
exec_parse_dec_u64:
    move.q  r8, r0
    move.q  r9, r0
.epd_loop:
    load.b  r1, (r17)
    move.q  r2, #0x30
    blt     r1, r2, .epd_done
    move.q  r2, #0x39
    bgt     r1, r2, .epd_done
    sub.q   r1, r1, #0x30
    mulu.q  r8, r8, #10
    add.q   r8, r8, r1
    add.q   r17, r17, #1
    move.q  r9, #1
    bra     .epd_loop
.epd_done:
    move.q  r2, #0x2E             ; decimal point means this is not I64
    beq     r1, r2, .epd_fail
    move.q  r2, #0x45             ; E
    beq     r1, r2, .epd_fail
    move.q  r2, #0x65             ; e
    beq     r1, r2, .epd_fail
    rts
.epd_fail:
    move.q  r9, r0
    rts

; --- READ ---
exec_do_read:
    ; READ A[,B,...] from DATA statements in program text
.read_loop:
    jsr     exec_skip_spaces
    jsr     var_lookup           ; R8 = value slot pointer
    beqz    r8, .read_done
    move.q  r22, r8
    push    r17                  ; save text pointer (clobbered by data scanner)
    jsr     exec_read_next_data_value ; R8 = FP64 value
    pop     r17                  ; restore text pointer
    jsr     exec_store_fp64_r22
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C            ; ','
    bne     r1, r2, .read_done
    add.q   r17, r17, #1
    bra     .read_loop
.read_done:
    rts
    ; === END exec_do_read ===

; --- Hardware command wrappers ---
; Each wraps a hw_xxx call, then returns. Since the hw_xxx handlers
; all return via rts, we can tail-call them with bra instead of jsr+rts.
exec_do_screen:   bra hw_screen
exec_do_cls:      bra hw_cls
exec_do_plot:     bra hw_plot
exec_do_palette:  bra hw_palette
exec_do_vsync:    bra hw_vsync
exec_do_locate:   bra hw_locate
exec_do_color:    bra hw_color
exec_do_line:     bra hw_line
exec_do_circle:   bra hw_circle
exec_do_box:      bra hw_box
exec_do_scroll:   bra hw_scroll
exec_do_sound:    bra hw_sound
exec_do_envelope: bra hw_envelope
exec_do_gate:     bra hw_gate
exec_do_wait:     bra hw_wait
exec_do_copper:   bra hw_copper
exec_do_blit:     bra hw_blit
exec_do_ula:      bra hw_ula
exec_do_ted_cmd:  bra hw_ted_cmd
exec_do_antic:    bra hw_antic
exec_do_gtia:     bra hw_gtia
exec_do_voodoo:   bra hw_voodoo
exec_do_psg:      bra hw_psg
exec_do_sid:      bra hw_sid
exec_do_pokey:    bra hw_pokey
exec_do_ahx:      bra hw_ahx
exec_do_sap:      bra hw_sap
exec_do_zbuffer:  bra hw_zbuffer
exec_do_vertex:   bra hw_vertex
exec_do_triangle: bra hw_triangle
exec_do_texture:  bra hw_texture

; --- CALL ---
exec_do_call:
    ; CALL addr - JSR to machine code at given address
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = FP64 address
    jsr     fp_int
    jsr     fp_fix               ; R8 = integer address
    move.q  r9, r8               ; R9 = call target
    ; Save BASIC state registers across machine-language call
    push    r14
    push    r16
    push    r17
    push    r26
    push    r28
    jsr     (r9)                 ; register-indirect JSR
    pop     r28
    pop     r26
    pop     r17
    pop     r16
    pop     r14
    rts
    ; === END exec_do_call ===

; --- TRON ---
exec_do_tron:
    ; TRON - enable trace mode (print line numbers during execution)
    add.q   r1, r16, #ST_TRACE_FLAG
    move.q  r2, #1
    store.l r2, (r1)
    rts
    ; === END exec_do_tron ===

; --- LOAD and SAVE are implemented in ehbasic_file_io.inc ---

exec_do_troff:
    ; TROFF - disable trace mode
    add.q   r1, r16, #ST_TRACE_FLAG
    store.l r0, (r1)
    rts
    ; === END exec_do_troff ===

; --- EXTENDED STATEMENTS ---
exec_do_ext:
    load.b  r1, (r17)
    add.q   r17, r17, #1
    move.q  r2, #EXT_TRON
    beq     r1, r2, exec_do_tron
    move.q  r2, #EXT_TROFF
    beq     r1, r2, exec_do_troff
    move.q  r2, #EXT_POKE8
    beq     r1, r2, .ext_poke8
    move.q  r2, #EXT_POKE16
    beq     r1, r2, .ext_poke16
    move.q  r2, #EXT_POKE32
    beq     r1, r2, .ext_poke32
    move.q  r2, #EXT_POKE64
    beq     r1, r2, .ext_poke64
    bra     exec_do_unknown
.ext_poke8:
    move.q  r23, #1
    bra     exec_do_poke_width
.ext_poke16:
    move.q  r23, #2
    bra     exec_do_poke_width
.ext_poke32:
    move.q  r23, #4
    bra     exec_do_poke_width
.ext_poke64:
    move.q  r23, #8
    bra     exec_do_poke_width

; --- DEF FN ---
exec_do_def:
    push    r14
    push    r15
    push    r22
    push    r23
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_FN
    beq     r1, r2, .def_consume_fn_token
    move.q  r2, #0x46            ; raw "FN" prefix in names like FND
    bne     r1, r2, .def_troff   ; legacy TROFF token alias
    load.b  r1, 1(r17)
    move.q  r2, #0x4E
    bne     r1, r2, .def_troff
    add.q   r17, r17, #2
    bra     .def_after_fn
.def_consume_fn_token:
    add.q   r17, r17, #1
.def_after_fn:
    jsr     exec_skip_spaces
    jsr     var_parse_name       ; R14 = function name tag
    move.q  r22, r14
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x28            ; '('
    bne     r1, r2, .def_syntax
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     var_parse_name       ; R14 = parameter tag
    move.q  r23, r14
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x29            ; ')'
    bne     r1, r2, .def_syntax
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_EQUAL
    beq     r1, r2, .def_has_eq
    move.q  r2, #0x3D
    bne     r1, r2, .def_syntax
.def_has_eq:
    add.q   r17, r17, #1
    la      r1, fn_table
    move.q  r2, #8
    move.q  r3, r0               ; first empty slot
.def_find_slot:
    beqz    r2, .def_store_slot
    load.l  r4, (r1)             ; function name tag
    beq     r4, r22, .def_use_slot
    bnez    r4, .def_next_slot
    bnez    r3, .def_next_slot
    move.q  r3, r1               ; remember empty slot
.def_next_slot:
    add.q   r1, r1, #24
    sub.q   r2, r2, #1
    bra     .def_find_slot
.def_store_slot:
    beqz    r3, .def_syntax
    move.q  r1, r3
.def_use_slot:
    store.l r22, (r1)
    store.l r23, 4(r1)
    store.q r17, 8(r1)
    store.l r0, 16(r1)
    store.l r0, 20(r1)
.def_skip:
    load.b  r1, (r17)
    beqz    r1, .def_done
    add.q   r17, r17, #1
    bra     .def_skip
.def_done:
    pop     r23
    pop     r22
    pop     r15
    pop     r14
    rts
.def_syntax:
    move.q  r8, #ERR_SYNTAX
    la      r9, err_msg_syntax
    jsr     raise_error
    pop     r23
    pop     r22
    pop     r15
    pop     r14
    rts
.def_troff:
    pop     r23
    pop     r22
    pop     r15
    pop     r14
    bra     exec_do_troff

; --- Unknown token handler ---
exec_do_unknown:
    move.q  r8, #ERR_SYNTAX
    la      r9, err_msg_syntax
    jsr     raise_error
    rts
    ; === END exec_do_unknown ===

; ============================================================================
; Statement Jump Table (128 entries, indexed by token - 0x80)
; ============================================================================
; Indices: 0=END, 1=FOR, 2=NEXT, 3=DATA, 4=INPUT, 5=DIM, 6=READ, 7=LET,
;          8=DEC, 9=GOTO, 10=unused(RUN), 11=IF, 12=RESTORE, 13=GOSUB,
;          14=RETURN, 15=REM, 16=STOP, 17=ON, 18=extended, 19=INC,
;          20=WAIT, 21=LOAD, 22=SAVE, 23=DEF, 24=POKE, 25-26=removed,
;          27=CALL, 28=DO, 29=LOOP, 30=PRINT, 31=CONT
;          32-46=mixed (LIST/NEW/BLOAD and parser keywords), 47=WEND, 48=WHILE, 49-97=unused
;          98=SCREEN..127=TEXTURE

    align 4
stmt_jump_table:
    dc.l    exec_do_end          ;  0 = TK_END      (0x80)
    dc.l    exec_do_for          ;  1 = TK_FOR      (0x81)
    dc.l    exec_do_next         ;  2 = TK_NEXT     (0x82)
    dc.l    exec_do_data         ;  3 = TK_DATA     (0x83)
    dc.l    exec_do_input        ;  4 = TK_INPUT    (0x84)
    dc.l    exec_do_dim          ;  5 = TK_DIM      (0x85)
    dc.l    exec_do_read         ;  6 = TK_READ     (0x86)
    dc.l    exec_do_let          ;  7 = TK_LET      (0x87)
    dc.l    exec_do_dec          ;  8 = TK_DEC      (0x88)
    dc.l    exec_do_goto         ;  9 = TK_GOTO     (0x89)
    dc.l    exec_do_run          ; 10 = TK_RUN      (0x8A)
    dc.l    exec_do_if           ; 11 = TK_IF       (0x8B)
    dc.l    exec_do_restore      ; 12 = TK_RESTORE  (0x8C)
    dc.l    exec_do_gosub        ; 13 = TK_GOSUB    (0x8D)
    dc.l    exec_do_return       ; 14 = TK_RETURN   (0x8E)
    dc.l    exec_do_rem          ; 15 = TK_REM      (0x8F)
    dc.l    exec_do_stop         ; 16 = TK_STOP     (0x90)
    dc.l    exec_do_on           ; 17 = TK_ON       (0x91)
    dc.l    exec_do_ext          ; 18 = TK_EXT      (0x92)
    dc.l    exec_do_inc          ; 19 = TK_INC      (0x93)
    dc.l    exec_do_wait         ; 20 = TK_WAIT     (0x94)
    dc.l    exec_do_load         ; 21 = TK_LOAD     (0x95)
    dc.l    exec_do_save         ; 22 = TK_SAVE     (0x96)
    dc.l    exec_do_def          ; 23 = TK_DEF      (0x97)
    dc.l    exec_do_poke         ; 24 = TK_POKE     (0x98)
    dc.l    exec_do_unknown      ; 25 = removed DOKE (0x99)
    dc.l    exec_do_unknown      ; 26 = removed LOKE (0x9A)
    dc.l    exec_do_call         ; 27 = TK_CALL     (0x9B)
    dc.l    exec_do_do           ; 28 = TK_DO       (0x9C)
    dc.l    exec_do_loop         ; 29 = TK_LOOP     (0x9D)
    dc.l    exec_do_print        ; 30 = TK_PRINT    (0x9E)
    dc.l    exec_do_cont         ; 31 = TK_CONT     (0x9F)
    dc.l    exec_do_list         ; 32 = TK_LIST     (0xA0)
    dc.l    exec_do_clear        ; 33 = TK_CLEAR    (0xA1)
    dc.l    exec_do_new          ; 34 = TK_NEW      (0xA2)
    dc.l    exec_do_bload        ; 35 = TK_WIDTH→BLOAD (0xA3)
    dc.l    exec_do_get          ; 36 = TK_GET      (0xA4)
    dc.l    exec_do_swap         ; 37 = TK_SWAP     (0xA5)
    dc.l    exec_do_bitset       ; 38 = TK_BITSET   (0xA6)
    dc.l    exec_do_bitclr       ; 39 = TK_BITCLR   (0xA7)
    dc.l    exec_do_unknown      ; 40 = TK_THEN     (0xA8)
    dc.l    exec_do_unknown      ; 41 = TK_TO       (0xA9)
    dc.l    exec_do_unknown      ; 42 = TK_STEP     (0xAA)
    dc.l    exec_do_else         ; 43 = TK_ELSE     (0xAB)
    dc.l    exec_do_unknown      ; 44 = unused      (0xAC)
    dc.l    exec_do_unknown      ; 45 = TK_NOT      (0xAD)
    dc.l    exec_do_unknown      ; 46 = unused      (0xAE)
    dc.l    exec_do_wend         ; 47 = TK_UNTIL    (0xAF) → WEND
    dc.l    exec_do_while        ; 48 = TK_WHILE    (0xB0)
    ; 49-97: function/operator tokens - not statements
    dc.l    exec_do_unknown      ; 49  (0xB1) TK_PLUS
    dc.l    exec_do_unknown      ; 50  (0xB2) TK_MINUS
    dc.l    exec_do_unknown      ; 51  (0xB3) TK_MULT
    dc.l    exec_do_unknown      ; 52  (0xB4) TK_DIV
    dc.l    exec_do_unknown      ; 53  (0xB5) TK_POWER
    dc.l    exec_do_unknown      ; 54  (0xB6) TK_AND
    dc.l    exec_do_unknown      ; 55  (0xB7) TK_EOR
    dc.l    exec_do_unknown      ; 56  (0xB8) TK_OR
    dc.l    exec_do_unknown      ; 57  (0xB9)
    dc.l    exec_do_unknown      ; 58  (0xBA)
    dc.l    exec_do_unknown      ; 59  (0xBB) TK_GT
    dc.l    exec_do_unknown      ; 60  (0xBC) TK_EQUAL
    dc.l    exec_do_unknown      ; 61  (0xBD) TK_LT
    dc.l    exec_do_unknown      ; 62  (0xBE) TK_SGN
    dc.l    exec_do_unknown      ; 63  (0xBF) TK_INT
    dc.l    exec_do_unknown      ; 64  (0xC0) TK_ABS
    dc.l    exec_do_unknown      ; 65  (0xC1) TK_USR
    dc.l    exec_do_unknown      ; 66  (0xC2) TK_FRE
    dc.l    exec_do_unknown      ; 67  (0xC3)
    dc.l    exec_do_unknown      ; 68  (0xC4) TK_SQR
    dc.l    exec_do_unknown      ; 69  (0xC5) TK_RND
    dc.l    exec_do_unknown      ; 70  (0xC6) TK_SIN
    dc.l    exec_do_unknown      ; 71  (0xC7) TK_LOG
    dc.l    exec_do_unknown      ; 72  (0xC8) TK_EXP
    dc.l    exec_do_unknown      ; 73  (0xC9) TK_COS
    dc.l    exec_do_unknown      ; 74  (0xCA) TK_TAN
    dc.l    exec_do_unknown      ; 75  (0xCB) TK_ATN
    dc.l    exec_do_unknown      ; 76  (0xCC) TK_PEEK
    dc.l    exec_do_unknown      ; 77  (0xCD) removed DEEK
    dc.l    exec_do_unknown      ; 78  (0xCE) removed LEEK
    dc.l    exec_do_unknown      ; 79  (0xCF) TK_LEN
    dc.l    exec_do_unknown      ; 80  (0xD0) TK_STR
    dc.l    exec_do_unknown      ; 81  (0xD1) TK_VAL
    dc.l    exec_do_unknown      ; 82  (0xD2) TK_ASC
    dc.l    exec_do_unknown      ; 83  (0xD3) TK_CHR
    dc.l    exec_do_unknown      ; 84  (0xD4) TK_LEFTS
    dc.l    exec_do_unknown      ; 85  (0xD5) TK_RIGHTS
    dc.l    exec_do_unknown      ; 86  (0xD6) TK_MIDS
    dc.l    exec_do_unknown      ; 87  (0xD7) TK_PI
    dc.l    exec_do_unknown      ; 88  (0xD8) TK_TWOPI
    dc.l    exec_do_unknown      ; 89  (0xD9) TK_HEXS
    dc.l    exec_do_unknown      ; 90  (0xDA) TK_BINS
    dc.l    exec_do_unknown      ; 91  (0xDB) TK_BITTST
    dc.l    exec_do_unknown      ; 92  (0xDC) TK_MAX
    dc.l    exec_do_unknown      ; 93  (0xDD) TK_MIN
    dc.l    exec_do_unknown      ; 94  (0xDE) TK_VPTR — VARPTR is an
                                  ;     expression-context function, not a
                                  ;     statement. Tokenized VARPTR used as
                                  ;     a statement falls through to here.
                                  ;     HOST is intercepted as a raw
                                  ;     statement in exec_line, not via this
                                  ;     dispatch slot.
    dc.l    exec_do_unknown      ; 95  (0xDF)
    dc.l    exec_do_unknown      ; 96  (0xE0)
    dc.l    exec_do_unknown      ; 97  (0xE1)
    ; 98-127: Hardware extension tokens (0xE2-0xFF)
    dc.l    exec_do_screen       ; 98  (0xE2) TK_SCREEN
    dc.l    exec_do_cls          ; 99  (0xE3) TK_CLS
    dc.l    exec_do_plot         ;100  (0xE4) TK_PLOT
    dc.l    exec_do_palette      ;101  (0xE5) TK_PALETTE
    dc.l    exec_do_vsync        ;102  (0xE6) TK_VSYNC
    dc.l    exec_do_locate       ;103  (0xE7) TK_LOCATE
    dc.l    exec_do_color        ;104  (0xE8) TK_COLOR
    dc.l    exec_do_line         ;105  (0xE9) TK_LINE_CMD
    dc.l    exec_do_circle       ;106  (0xEA) TK_CIRCLE
    dc.l    exec_do_box          ;107  (0xEB) TK_BOX
    dc.l    exec_do_scroll       ;108  (0xEC) TK_SCROLL_CMD
    dc.l    exec_do_copper       ;109  (0xED) TK_COPPER
    dc.l    exec_do_blit         ;110  (0xEE) TK_BLIT
    dc.l    exec_do_sound        ;111  (0xEF) TK_SOUND
    dc.l    exec_do_envelope     ;112  (0xF0) TK_ENVELOPE
    dc.l    exec_do_gate         ;113  (0xF1) TK_GATE
    dc.l    exec_do_ula          ;114  (0xF2) TK_ULA
    dc.l    exec_do_ted_cmd      ;115  (0xF3) TK_TED_CMD
    dc.l    exec_do_antic        ;116  (0xF4) TK_ANTIC
    dc.l    exec_do_gtia         ;117  (0xF5) TK_GTIA
    dc.l    exec_do_voodoo       ;118  (0xF6) TK_VOODOO
    dc.l    exec_do_psg          ;119  (0xF7) TK_PSG_CMD
    dc.l    exec_do_sid          ;120  (0xF8) TK_SID_CMD
    dc.l    exec_do_pokey        ;121  (0xF9) TK_POKEY_CMD
    dc.l    exec_do_ahx          ;122  (0xFA) TK_AHX
    dc.l    exec_do_sap          ;123  (0xFB) TK_SAP
    dc.l    exec_do_zbuffer      ;124  (0xFC) TK_ZBUFFER
    dc.l    exec_do_vertex       ;125  (0xFD) TK_VERTEX
    dc.l    exec_do_triangle     ;126  (0xFE) TK_TRIANGLE
    dc.l    exec_do_texture      ;127  (0xFF) TK_TEXTURE

    align 8
if_else_boundary:
    dc.q    0
; DEF FN table: 8 entries x [name_tag.l, param_tag.l, body_ptr.q, active.l, reserved.l]
fn_table:
    dc.l    0,0,0,0,0,0
    dc.l    0,0,0,0,0,0
    dc.l    0,0,0,0,0,0
    dc.l    0,0,0,0,0,0
    dc.l    0,0,0,0,0,0
    dc.l    0,0,0,0,0,0
    dc.l    0,0,0,0,0,0
    dc.l    0,0,0,0,0,0

; ============================================================================
; Helper: exec_skip_spaces
; ============================================================================
exec_skip_spaces:
    load.b  r1, (r17)
    move.q  r2, #0x20            ; space
    bne     r1, r2, .ess_done
    add.q   r17, r17, #1
    bra     exec_skip_spaces
.ess_done:
    rts

; ============================================================================
; Helper: exec_parse_linenum - Parse a decimal line number from token stream
; ============================================================================
; Input:  R17 = text pointer
; Output: R8  = line number (integer)
;         R17 = advanced past number
; Clobbers: R1-R3

exec_parse_linenum:
    move.q  r8, r0               ; accumulator = 0

.epln_loop:
    load.b  r1, (r17)
    move.q  r2, #0x30            ; '0'
    blt     r1, r2, .epln_done
    move.q  r2, #0x39            ; '9'
    bgt     r1, r2, .epln_done
    sub.q   r1, r1, #0x30       ; digit value
    mulu.l  r8, r8, #10
    add.l   r8, r8, r1
    add.q   r17, r17, #1
    bra     .epln_loop

.epln_done:
    rts

; ============================================================================
; Helper: exec_skip_number - Skip decimal digits at R17
; ============================================================================
; Input:  R17 = text pointer
; Output: R17 = advanced past digits
; Clobbers: R1, R2

exec_skip_number:
    load.b  r1, (r17)
    move.q  r2, #0x30            ; '0'
    blt     r1, r2, .esn_done
    move.q  r2, #0x39            ; '9'
    bgt     r1, r2, .esn_done
    add.q   r17, r17, #1
    bra     exec_skip_number
.esn_done:
    rts

; ============================================================================
; Helper: exec_find_line - Find a line by number in the program
; ============================================================================
; Input:  R8 = line number to find
; Output: R14 = pointer to the line (0 if not found)
; Clobbers: R1-R3

exec_find_line:
    ; Start from beginning of program
    load.q  r1, (r16)           ; R1 = first line pointer

.efl_loop:
    beqz    r1, .efl_not_found
    ; Read line number at R1+8
    add.q   r2, r1, #8
    load.l  r2, (r2)
    beq     r2, r8, .efl_found
    bgt     r2, r8, .efl_not_found  ; passed it
    ; Advance to next line
    load.q  r1, (r1)            ; follow next pointer
    bra     .efl_loop

.efl_found:
    move.q  r14, r1
    rts

.efl_not_found:
    move.q  r14, r0
    rts

; ============================================================================
; Helper: exec_read_next_data_value
; ============================================================================
; Uses ST_DATA_PTR as the count of values already consumed by READ.
; Input:  R16 = state base
; Output: R8  = next DATA value as FP64 (0 if unavailable)
; Clobbers: R1-R15, R22-R25

exec_read_next_data_value:
    ; target index = state[ST_DATA_PTR]
    add.q   r1, r16, #ST_DATA_PTR
    load.l  r6, (r1)             ; R6 = target data index
    push    r6                   ; save target index
    jsr     exec_find_data_value_by_index
    ; consume one slot regardless (keeps READ monotonic)
    pop     r6                   ; restore target index
    add.q   r6, r6, #1
    add.q   r1, r16, #ST_DATA_PTR  ; recompute (R1 was clobbered)
    store.l r6, (r1)
    rts

; ============================================================================
; Helper: exec_find_data_value_by_index
; ============================================================================
; Input:  R6 = target DATA element index (0-based)
; Output: R8 = FP64 value (0 if not found)
; Clobbers: R1-R15, R22-R25

exec_find_data_value_by_index:
    push    r14
    push    r15

    move.q  r7, r0               ; R7 = current data index
    load.q  r10, (r16)           ; R10 = first line pointer

.fd_line_loop:
    beqz    r10, .fd_not_found
    load.q  r11, (r10)           ; next line pointer

    add.q   r12, r10, #16        ; R12 = content pointer
.fd_stmt_loop:
    load.b  r1, (r12)
    beqz    r1, .fd_next_line
    move.q  r2, #0x3A            ; ':'
    beq     r1, r2, .fd_after_colon
    move.q  r2, #TK_DATA
    beq     r1, r2, .fd_data_stmt
    add.q   r12, r12, #1
    bra     .fd_stmt_loop

.fd_after_colon:
    add.q   r12, r12, #1
    bra     .fd_stmt_loop

.fd_data_stmt:
    add.q   r12, r12, #1         ; skip TK_DATA

.fd_values_loop:
    ; skip spaces
.fd_skip_spaces:
    load.b  r1, (r12)
    move.q  r2, #0x20
    bne     r1, r2, .fd_value_start
    add.q   r12, r12, #1
    bra     .fd_skip_spaces

.fd_value_start:
    load.b  r1, (r12)
    beqz    r1, .fd_next_line
    move.q  r2, #0x3A
    beq     r1, r2, .fd_after_colon

    ; Optional sign for numeric literal
    move.q  r13, r0              ; sign flag (0=+, 1=-)
    move.q  r2, #0x2D            ; '-'
    beq     r1, r2, .fd_neg_sign
    move.q  r2, #TK_MINUS
    beq     r1, r2, .fd_neg_sign
    move.q  r2, #0x2B            ; '+'
    beq     r1, r2, .fd_pos_sign
    move.q  r2, #TK_PLUS
    bne     r1, r2, .fd_parse_value
.fd_pos_sign:
    add.q   r12, r12, #1
    bra     .fd_parse_value
.fd_neg_sign:
    add.q   r12, r12, #1
    move.q  r13, #1

.fd_parse_value:
    ; parse_number clobbers R1-R7, so preserve DATA indices (R6=target, R7=current)
    push    r6
    push    r7
    move.q  r17, r12
    jsr     parse_number          ; R8 = FP64, R17 advanced
    move.q  r12, r17

    ; apply sign if needed
    beqz    r13, .fd_restore_idx
    jsr     fp_neg

.fd_restore_idx:
    pop     r7
    pop     r6

.fd_check_index:
    beq     r7, r6, .fd_found
    add.q   r7, r7, #1

    ; skip spaces after value
.fd_post_spaces:
    load.b  r1, (r12)
    move.q  r2, #0x20
    bne     r1, r2, .fd_after_value
    add.q   r12, r12, #1
    bra     .fd_post_spaces

.fd_after_value:
    load.b  r1, (r12)
    move.q  r2, #0x2C            ; ','
    bne     r1, r2, .fd_stmt_end
    add.q   r12, r12, #1
    bra     .fd_values_loop

.fd_stmt_end:
    ; End of DATA statement segment; continue scanning line for next statements.
    bra     .fd_stmt_loop

.fd_next_line:
    move.q  r10, r11
    bra     .fd_line_loop

.fd_found:
    pop     r15
    pop     r14
    rts

.fd_not_found:
    move.q  r8, r0
    pop     r15
    pop     r14
    rts

; ============================================================================
; EOF
; ============================================================================
