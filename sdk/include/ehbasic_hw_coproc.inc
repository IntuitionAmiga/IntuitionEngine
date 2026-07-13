; ehbasic_hw_coproc.inc - IE64 BASIC Coprocessor Commands
;
; Statement handlers:
;   COSTART cpuType, "serviceFile" - Start coprocessor worker
;   COSTOP cpuType                 - Stop coprocessor worker
;   COWAIT ticket [, timeoutMs]    - Wait for ticket completion
;
; Function handlers (called from expr_atom):
;   COCALL(cpuType, op, reqPtr, reqLen, respPtr, respCap) - Enqueue request
;   COSTATUS(ticket)                                       - Poll ticket status
;   COSTATS(field)                                          - Read coproc stats register
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; exec_do_costart - COSTART cpuType, "serviceFile"
; ============================================================================
; Starts a coprocessor worker for the given CPU type.
; cpuType: 1=IE32, 2=IE64, 3=6502, 4=M68K, 5=Z80, 6=x86
; serviceFile: path to service binary (string expression)

exec_do_costart:
    jsr     exec_skip_spaces
    ; Parse cpuType expression, write it before further evals clobber regs
    jsr     coproc_eval_int
    la      r1, COPROC_CPU_TYPE
    store.l r8, (r1)
    ; Default instance 0 (written explicitly so a prior POKE cannot redirect)
    la      r1, COPROC_INSTANCE
    store.l r0, (r1)

    ; Expect comma
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C            ; ','
    bne     r1, r2, .costart_done
    add.q   r17, r17, #1         ; skip comma
    jsr     exec_skip_spaces

    ; Optional instance: a string-literal filename starts with '"'. If the next
    ; token is not a string, it is a numeric instance expression followed by a
    ; second comma before the filename.
    load.b  r1, (r17)
    move.q  r2, #0x22            ; '"'
    beq     r1, r2, .costart_filename
    jsr     coproc_eval_int
    la      r1, COPROC_INSTANCE
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C            ; ','
    bne     r1, r2, .costart_done
    add.q   r17, r17, #1         ; skip comma
    jsr     exec_skip_spaces

.costart_filename:
    ; Parse filename string
    jsr     str_eval             ; R8 = filename string pointer
    move.q  r10, r8

    ; Copy filename to EHBASIC_PRIV_NAME_BUF (reuse existing buffer)
    la      r1, EHBASIC_PRIV_NAME_BUF
.costart_copy:
    load.b  r2, (r10)
    store.b r2, (r1)
    beqz    r2, .costart_copied
    add.q   r10, r10, #1
    add.q   r1, r1, #1
    bra     .costart_copy
.costart_copied:

    ; Write filename pointer to COPROC_NAME_PTR
    la      r1, COPROC_NAME_PTR
    la      r2, EHBASIC_PRIV_NAME_BUF
    store.l r2, (r1)

    ; Trigger START command
    la      r1, COPROC_CMD
    move.l  r2, #COPROC_CMD_START
    store.l r2, (r1)

    ; Check status
    la      r1, COPROC_CMD_STATUS
    load.l  r1, (r1)
    beqz    r1, .costart_done

    ; Error: set R28 for error signaling
    move.q  r28, #2              ; signal END
.costart_done:
    rts

; ============================================================================
; exec_do_costop - COSTOP cpuType
; ============================================================================

exec_do_costop:
    jsr     exec_skip_spaces
    ; Parse cpuType expression, write it before any further eval clobbers regs
    jsr     coproc_eval_int
    la      r1, COPROC_CPU_TYPE
    store.l r8, (r1)
    ; Default instance 0 (written explicitly so a prior POKE cannot redirect)
    la      r1, COPROC_INSTANCE
    store.l r0, (r1)

    ; Optional ", instance"
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C            ; ','
    bne     r1, r2, .costop_go
    add.q   r17, r17, #1         ; skip comma
    jsr     exec_skip_spaces
    jsr     coproc_eval_int
    la      r1, COPROC_INSTANCE
    store.l r8, (r1)

.costop_go:
    ; Trigger STOP command
    la      r1, COPROC_CMD
    move.l  r2, #COPROC_CMD_STOP
    store.l r2, (r1)

    rts

; ============================================================================
; exec_do_cowait - COWAIT ticket [, timeoutMs]
; ============================================================================
; Blocks until ticket completes or timeout expires.
; Default timeout: 1000ms.
; After COWAIT, use COSTATUS(ticket) to check outcome.

exec_do_cowait:
    jsr     exec_skip_spaces
    ; Parse ticket expression
    jsr     coproc_eval_int
    move.q  r22, r8              ; R22 = ticket

    ; Write ticket to COPROC_TICKET
    la      r1, COPROC_TICKET
    store.l r22, (r1)

    ; Default timeout = 1000ms
    move.l  r23, #1000

    ; Check for optional comma + timeout
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C            ; ','
    bne     r1, r2, .cowait_do
    add.q   r17, r17, #1         ; skip comma
    jsr     exec_skip_spaces
    jsr     coproc_eval_int
    move.q  r23, r8              ; R23 = timeout

.cowait_do:
    ; Write timeout to COPROC_TIMEOUT
    la      r1, COPROC_TIMEOUT
    store.l r23, (r1)

    ; Trigger WAIT command (blocks in Go until completion or timeout)
    la      r1, COPROC_CMD
    move.l  r2, #COPROC_CMD_WAIT_CMD
    store.l r2, (r1)

    rts

; ============================================================================
; fn_cocall - COCALL(cpuType, op, reqPtr, reqLen, respPtr, respCap)
; ============================================================================
; Enqueues an async coprocessor request.
; Returns ticket number as FP64 in R8.

; Two forms, disambiguated structurally (expr_eval clobbers r20-r27, so no live
; arg counter can survive across evals):
;   6 args: COCALL(cpuType, op, reqPtr, reqLen, respPtr, respCap)      instance 0
;   7 args: COCALL(cpuType, instance, op, reqPtr, reqLen, respPtr, respCap)
; The two forms share the first six parse positions; after parsing six args a
; trailing ',' means the seven-arg form (a seventh arg follows), a ')' the
; six-arg form. Any other count raises ?FC.
fn_cocall:
    ; '(' already consumed by caller - skip spaces
    jsr     exec_skip_spaces

    ; Shared arg 1
    jsr     coproc_eval_int
    push    r8
    jsr     .cocall_want_comma
    beqz    r8, .cocall_pop1

    ; Shared arg 2
    jsr     exec_skip_spaces
    jsr     coproc_eval_int
    push    r8
    jsr     .cocall_want_comma
    beqz    r8, .cocall_pop2

    ; Shared arg 3
    jsr     exec_skip_spaces
    jsr     coproc_eval_int
    push    r8
    jsr     .cocall_want_comma
    beqz    r8, .cocall_pop3

    ; Shared arg 4
    jsr     exec_skip_spaces
    jsr     coproc_eval_int
    push    r8
    jsr     .cocall_want_comma
    beqz    r8, .cocall_pop4

    ; Shared arg 5
    jsr     exec_skip_spaces
    jsr     coproc_eval_int
    push    r8
    jsr     .cocall_want_comma
    beqz    r8, .cocall_pop5

    ; Shared arg 6
    jsr     exec_skip_spaces
    jsr     coproc_eval_int
    push    r8

    ; Peek: ',' => seven-arg form, ')' => six-arg form.
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C            ; ','
    beq     r1, r2, .cocall_seven

    ; Six-arg form: stack = cpuType, op, reqPtr, reqLen, respPtr, respCap
    pop     r25                  ; respCap
    pop     r24                  ; respPtr
    pop     r23                  ; reqLen
    pop     r22                  ; reqPtr
    pop     r21                  ; op
    pop     r20                  ; cpuType
    move.q  r27, r0              ; instance = 0
    bra     .cocall_dispatch

.cocall_seven:
    add.q   r17, r17, #1         ; skip comma
    jsr     exec_skip_spaces
    jsr     coproc_eval_int      ; arg 7 = respCap
    move.q  r25, r8              ; respCap
    ; stack = cpuType, instance, op, reqPtr, reqLen, respPtr
    pop     r24                  ; respPtr
    pop     r23                  ; reqLen
    pop     r22                  ; reqPtr
    pop     r21                  ; op
    pop     r27                  ; instance
    pop     r20                  ; cpuType

.cocall_dispatch:
    move.q  r8, r22
    move.q  r9, r23
    jsr     coproc_validate_public_span
    beqz    r8, .cocall_fc
    move.q  r8, r24
    move.q  r9, r25
    jsr     coproc_validate_public_span
    beqz    r8, .cocall_fc

    ; Write MMIO registers (instance explicit, then the request descriptor)
    la      r1, COPROC_CPU_TYPE
    store.l r20, (r1)

    la      r1, COPROC_INSTANCE
    store.l r27, (r1)

    la      r1, COPROC_OP
    store.l r21, (r1)

    la      r1, COPROC_REQ_PTR
    store.l r22, (r1)

    la      r1, COPROC_REQ_LEN
    store.l r23, (r1)

    la      r1, COPROC_RESP_PTR
    store.l r24, (r1)

    la      r1, COPROC_RESP_CAP
    store.l r25, (r1)

    ; Trigger ENQUEUE command
    la      r1, COPROC_CMD
    move.l  r2, #COPROC_CMD_ENQUEUE
    store.l r2, (r1)

    ; Check command status - if error, return 0
    la      r1, COPROC_CMD_STATUS
    load.l  r1, (r1)
    bnez    r1, .cocall_fail

    ; Read ticket from COPROC_TICKET
    la      r1, COPROC_TICKET
    load.l  r8, (r1)

    ; Convert to FP64
    jsr     fp_float

    ; Skip closing ')'
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x29            ; ')'
    bne     r1, r2, .cocall_ret
    add.q   r17, r17, #1
.cocall_ret:
    rts

; .cocall_want_comma - after an arg, require a ',' separator. Returns R8=1 and
; advances past the comma, or R8=0 (leaving the cursor) if the next non-space
; character is not a comma.
.cocall_want_comma:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C            ; ','
    bne     r1, r2, .cwc_no
    add.q   r17, r17, #1
    move.q  r8, #1
    rts
.cwc_no:
    move.q  r8, r0
    rts

.cocall_fail:
    move.q  r8, r0               ; return 0 on enqueue failure
    ; Skip closing ')'
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x29            ; ')'
    bne     r1, r2, .cocall_fail_ret
    add.q   r17, r17, #1
.cocall_fail_ret:
    rts

.cocall_fc:
    move.q  r8, #ERR_FC
    la      r9, err_msg_fc
    jsr     raise_error
    move.q  r8, r0
    rts

; Malformed argument list: pop the args pushed so far, then raise ?FC.
.cocall_pop5:
    pop     r8
.cocall_pop4:
    pop     r8
.cocall_pop3:
    pop     r8
.cocall_pop2:
    pop     r8
.cocall_pop1:
    pop     r8
    move.q  r8, #ERR_FC
    la      r9, err_msg_fc
    jsr     raise_error
    move.q  r8, r0
    rts

; coproc_validate_public_span - validate a COCALL request/response span.
; Input: R8 = pointer, R9 = byte length/capacity.
; Output: R8 = 1 valid, 0 invalid. Zero-length spans are valid.
; Clobbers: R1-R4.
coproc_validate_public_span:
    beqz    r9, .cvs_ok
    beqz    r8, .cvs_bad
    add.q   r1, r8, r9
    blt     r1, r8, .cvs_bad
    move.q  r2, #BASIC_MEMALLOC_BASE0
    move.q  r3, #BASIC_MEMALLOC_END0
    jsr     .cvs_range
    bnez    r4, .cvs_ok
    move.q  r2, #BASIC_MEMALLOC_BASE1
    move.q  r3, #BASIC_MEMALLOC_END1
    jsr     .cvs_range
    bnez    r4, .cvs_ok
    move.q  r2, #BASIC_MEMALLOC_BASE2
    move.q  r3, #BASIC_MEMALLOC_END2
    jsr     .cvs_range
    bnez    r4, .cvs_ok
.cvs_bad:
    move.q  r8, r0
    rts
.cvs_ok:
    move.q  r8, #1
    rts
.cvs_range:
    move.q  r4, r0
    blt     r8, r2, .cvs_range_ret
    bgt     r1, r3, .cvs_range_ret
    move.q  r4, #1
.cvs_range_ret:
    rts

; coproc_eval_int - evaluate an integer-compatible expression.
; Output: R8 = integer payload. I64 expression values are used directly; FP64
; values are truncated through the existing FP integer path.
; Clobbers: expression evaluator registers.
coproc_eval_int:
    jsr     expr_eval
    move.q  r1, #VAL_I64
    beq     r9, r1, .cei_ret
    jsr     fp_int
    jsr     fp_fix
.cei_ret:
    rts

; ============================================================================
; fn_costatus - COSTATUS(ticket)
; ============================================================================
; Returns the status of a ticket as FP64 in R8.
; 0=pending, 1=running, 2=ok, 3=error, 4=timeout, 5=worker_down

fn_costatus:
    ; '(' already consumed by caller - skip spaces
    jsr     exec_skip_spaces

    ; Parse ticket expression
    jsr     coproc_eval_int
    move.q  r22, r8              ; R22 = ticket

    ; Write ticket to COPROC_TICKET
    la      r1, COPROC_TICKET
    store.l r22, (r1)

    ; Trigger POLL command
    la      r1, COPROC_CMD
    move.l  r2, #COPROC_CMD_POLL
    store.l r2, (r1)

    ; Read ticket status from COPROC_TICKET_STATUS
    la      r1, COPROC_TICKET_STATUS
    load.l  r8, (r1)

    ; Convert to FP64
    jsr     fp_float

    ; Skip closing ')'
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x29            ; ')'
    bne     r1, r2, .costatus_ret
    add.q   r17, r17, #1
.costatus_ret:
    rts

; ============================================================================
; fn_costats - COSTATS(field)
; ============================================================================
; Reads a coprocessor stats register and returns the value as FP64.
; field: 0=ops dispatched, 1=bytes processed, 2=dispatch overhead (ns),
;        3=completed ticket, 4=IRQ enabled
;
; MMIO registers:
;   COPROC_STATS_OPS         (0xF2378)  field 0
;   COPROC_STATS_BYTES       (0xF237C)  field 1
;   COPROC_DISPATCH_OVERHEAD (0xF2384)  field 2
;   COPROC_COMPLETED_TICKET  (0xF2388)  field 3
;   COPROC_IRQ_CTRL          (0xF2380)  field 4

fn_costats:
    ; '(' already consumed by caller - skip spaces
    jsr     exec_skip_spaces

    ; Parse field expression
    jsr     coproc_eval_int
    ; R8 = field number

    ; Dispatch by field number
    beqz    r8, .costats_ops
    move.l  r1, #1
    beq     r8, r1, .costats_bytes
    move.l  r1, #2
    beq     r8, r1, .costats_overhead
    move.l  r1, #3
    beq     r8, r1, .costats_completed
    move.l  r1, #4
    beq     r8, r1, .costats_irq

    ; Invalid field — return 0
    move.q  r8, r0
    bra     .costats_close

.costats_ops:
    la      r1, COPROC_STATS_OPS
    load.l  r8, (r1)
    bra     .costats_convert

.costats_bytes:
    la      r1, COPROC_STATS_BYTES
    load.l  r8, (r1)
    bra     .costats_convert

.costats_overhead:
    la      r1, COPROC_DISPATCH_OVERHEAD
    load.l  r8, (r1)
    bra     .costats_convert

.costats_completed:
    la      r1, COPROC_COMPLETED_TICKET
    load.l  r8, (r1)
    bra     .costats_convert

.costats_irq:
    la      r1, COPROC_IRQ_CTRL
    load.l  r8, (r1)

.costats_convert:
    ; Convert to FP64
    jsr     fp_float

.costats_close:
    ; Skip closing ')'
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x29            ; ')'
    bne     r1, r2, .costats_ret
    add.q   r17, r17, #1
.costats_ret:
    rts

; ============================================================================
; fn_cocaps - COCAPS(cpuType)
; ============================================================================
; Returns the number of worker instances a CPU type supports (COPROC_INSTANCE_LIMIT).

fn_cocaps:
    ; '(' already consumed by caller
    jsr     exec_skip_spaces
    jsr     coproc_eval_int      ; R8 = cpuType
    la      r1, COPROC_CPU_TYPE
    store.l r8, (r1)
    la      r1, COPROC_INSTANCE_LIMIT
    load.l  r8, (r1)
    jsr     fp_float
    ; Skip closing ')'
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x29            ; ')'
    bne     r1, r2, .cocaps_ret
    add.q   r17, r17, #1
.cocaps_ret:
    rts

; ============================================================================
; fn_coinstance - COINSTANCE()
; ============================================================================
; Returns the currently selected worker instance (COPROC_INSTANCE shadow).

fn_coinstance:
    ; '(' already consumed by caller
    jsr     exec_skip_spaces
    la      r1, COPROC_INSTANCE
    load.l  r8, (r1)
    jsr     fp_float
    ; Skip closing ')'
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x29            ; ')'
    bne     r1, r2, .coinst_ret
    add.q   r17, r17, #1
.coinst_ret:
    rts

; ============================================================================
; fn_coselstate - COSELSTATE()
; ============================================================================
; Returns the running state of the selected (COPROC_CPU_TYPE, COPROC_INSTANCE)
; worker (COPROC_SELECTED_STATE): 1 online, 0 offline.

fn_coselstate:
    ; '(' already consumed by caller
    jsr     exec_skip_spaces
    la      r1, COPROC_SELECTED_STATE
    load.l  r8, (r1)
    jsr     fp_float
    ; Skip closing ')'
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x29            ; ')'
    bne     r1, r2, .cosel_ret
    add.q   r17, r17, #1
.cosel_ret:
    rts
