; ehbasic_hw_coproc.inc - EhBASIC IE64 Coprocessor Commands
;
; Statement handlers:
;   COSTART cpuType, "serviceFile" - Start coprocessor worker
;   COSTOP cpuType                 - Stop coprocessor worker
;   COWAIT ticket [, timeoutMs]    - Wait for ticket completion
;
; Function handlers (called from expr_atom):
;   COCALL(cpuType, op, reqPtr, reqLen, respPtr, respCap) - Enqueue request
;   COSTATUS(ticket)                                       - Poll ticket status
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; exec_do_costart - COSTART cpuType, "serviceFile"
; ============================================================================
; Starts a coprocessor worker for the given CPU type.
; cpuType: 1=IE32, 3=6502, 4=M68K, 5=Z80, 6=x86
; serviceFile: path to service binary (string expression)

exec_do_costart:
    jsr     exec_skip_spaces
    ; Parse cpuType expression
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = cpuType

    ; Expect comma
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C            ; ','
    bne     r1, r2, .costart_done
    add.q   r17, r17, #1         ; skip comma

    ; Parse filename string
    jsr     exec_skip_spaces
    jsr     str_eval             ; R8 = filename string pointer
    move.q  r10, r8

    ; Copy filename to FILE_NAME_BUF (reuse existing buffer)
    la      r1, FILE_NAME_BUF
.costart_copy:
    load.b  r2, (r10)
    store.b r2, (r1)
    beqz    r2, .costart_copied
    add.q   r10, r10, #1
    add.q   r1, r1, #1
    bra     .costart_copy
.costart_copied:

    ; Write cpuType to COPROC_CPU_TYPE
    la      r1, COPROC_CPU_TYPE
    store.l r22, (r1)

    ; Write filename pointer to COPROC_NAME_PTR
    la      r1, COPROC_NAME_PTR
    la      r2, FILE_NAME_BUF
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
    ; Parse cpuType expression
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = cpuType

    ; Write cpuType to COPROC_CPU_TYPE
    la      r1, COPROC_CPU_TYPE
    store.l r22, (r1)

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
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
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
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
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
; Returns ticket number as FP32 in R8.

fn_cocall:
    ; '(' already consumed by caller — skip spaces
    jsr     exec_skip_spaces

    ; Arg 1: cpuType
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    push    r8                   ; save cpuType
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .cocall_err
    add.q   r17, r17, #1

    ; Arg 2: op
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    push    r8                   ; save op
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .cocall_err2
    add.q   r17, r17, #1

    ; Arg 3: reqPtr
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    push    r8                   ; save reqPtr
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .cocall_err3
    add.q   r17, r17, #1

    ; Arg 4: reqLen
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    push    r8                   ; save reqLen
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .cocall_err4
    add.q   r17, r17, #1

    ; Arg 5: respPtr
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    push    r8                   ; save respPtr
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .cocall_err5
    add.q   r17, r17, #1

    ; Arg 6: respCap
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; R8 = respCap (keep in R8)
    move.q  r25, r8              ; R25 = respCap

    ; Pop args in reverse: respPtr, reqLen, reqPtr, op, cpuType
    pop     r24                  ; R24 = respPtr
    pop     r23                  ; R23 = reqLen
    pop     r22                  ; R22 = reqPtr
    pop     r21                  ; R21 = op    (R21 = register T=12)
    pop     r20                  ; R20 = cpuType (R20 = register S=11)

    ; Write MMIO registers
    la      r1, COPROC_CPU_TYPE
    store.l r20, (r1)

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

    ; Check command status — if error, return 0
    la      r1, COPROC_CMD_STATUS
    load.l  r1, (r1)
    bnez    r1, .cocall_fail

    ; Read ticket from COPROC_TICKET
    la      r1, COPROC_TICKET
    load.l  r8, (r1)

    ; Convert to FP32
    jsr     fp_float

    ; Skip closing ')'
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x29            ; ')'
    bne     r1, r2, .cocall_ret
    add.q   r17, r17, #1
.cocall_ret:
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

.cocall_err5:
    pop     r8                   ; discard respPtr
.cocall_err4:
    pop     r8                   ; discard reqLen
.cocall_err3:
    pop     r8                   ; discard reqPtr
.cocall_err2:
    pop     r8                   ; discard op
.cocall_err:
    pop     r8                   ; discard cpuType
    move.q  r8, r0               ; return 0
    rts

; ============================================================================
; fn_costatus - COSTATUS(ticket)
; ============================================================================
; Returns the status of a ticket as FP32 in R8.
; 0=pending, 1=running, 2=ok, 3=error, 4=timeout, 5=worker_down

fn_costatus:
    ; '(' already consumed by caller — skip spaces
    jsr     exec_skip_spaces

    ; Parse ticket expression
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
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

    ; Convert to FP32
    jsr     fp_float

    ; Skip closing ')'
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x29            ; ')'
    bne     r1, r2, .costatus_ret
    add.q   r17, r17, #1
.costatus_ret:
    rts
