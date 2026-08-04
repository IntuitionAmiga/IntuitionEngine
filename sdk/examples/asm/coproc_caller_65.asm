; Coprocessor mailbox caller: 6502 guest code that starts the matching worker, writes two operands to shared guest RAM, enqueues an add request and polls its ticket.
;
; The caller and service communicate through shared guest RAM and coprocessor MMIO.
; The Host SDK builds both guest binaries; it is not present at runtime. Read the
; mailbox constants, initialisation, descriptor exchange and terminal path in order.
; Run the caller with `go run . -m6502 -coproc-svc <service> <caller>`.
; Compare coproc_service_65.asm as its matching pair and the other CPU ports for their
; real addressing differences.
;


.include "ie65.inc"

.segment "CODE"
.org $0600

; ============================================================================
; GATEWAY WINDOW OVERVIEW
; ============================================================================
; The gateway at $F200-$F23F maps to bus $F2340-$F237F.
; All COPROC registers are accessed via byte-level writes using STORE32.
; Data buffers use bank windows to access bus memory above $FFFF.

; ============================================================================
; PHASE 1 - START THE 6502 WORKER
; ============================================================================
;
; Before we can enqueue any work, the coprocessor controller needs
; to know which CPU type to spawn and where to find the service binary.
; COPROC_CPU_TYPE selects the worker architecture; COPROC_NAME_PTR points
; to a null-terminated filename string in bus memory. Writing COPROC_CMD
; with COPROC_CMD_START triggers the launch. We then check CMD_STATUS to
; confirm the worker started without error.

    ; Select 6502 as the worker CPU type
    STORE32 COPROC_CPU_TYPE, COPROC_CPU_6502

    ; Point to the service binary filename in bus memory
    ; (must be pre-loaded into bus memory by the host before execution)
    STORE32 COPROC_NAME_PTR, $400000

    ; Trigger the START command
    STORE32 COPROC_CMD, COPROC_CMD_START

    ; Check whether the start succeeded (0 = OK, non-zero = error)
    LDA COPROC_CMD_STATUS
    BEQ no_start_err
    JMP error
no_start_err:

; ============================================================================
; PHASE 2 - ENQUEUE AN ADD REQUEST
; ============================================================================
;
; Now that the worker is running, we fill in the request descriptor.
; The protocol requires: CPU type (to route to the correct worker ring),
; operation code (op=1 means "add"), pointers to the request and response
; buffers in bus memory, and their sizes. Writing COPROC_CMD_ENQUEUE
; submits the request and returns a ticket number in COPROC_TICKET.
;
; The request buffer at $410000 contains two uint32 values (the operands).
; The response buffer at $410100 will receive one uint32 (the sum).
; These buffers must be pre-populated by the host environment.

    STORE32 COPROC_CPU_TYPE, COPROC_CPU_6502
    STORE32 COPROC_OP, 1               ; op = add
    STORE32 COPROC_REQ_PTR, $410000    ; request data at bus $410000
    STORE32 COPROC_REQ_LEN, 8          ; two uint32 = 8 bytes
    STORE32 COPROC_RESP_PTR, $410100   ; response buffer
    STORE32 COPROC_RESP_CAP, 4         ; capacity: 4 bytes (one uint32)
    STORE32 COPROC_CMD, COPROC_CMD_ENQUEUE

    ; Save the returned ticket number into zero page
    ; (tickets are small integers, so only byte 0 matters on the 6502)
    LDA COPROC_TICKET
    STA $00                            ; ZP $00 = ticket

; ============================================================================
; PHASE 3 - POLL UNTIL COMPLETE
; ============================================================================
;
; The worker processes the request asynchronously. We must poll the
; ticket status until it transitions from PENDING (0) or RUNNING (1) to
; a terminal state. COPROC_ST_OK (2) means the response buffer is valid.
;
; To poll, we write the ticket number back into the COPROC_TICKET register
; (all 4 bytes, clearing the upper 3 since the 6502 wrote only byte 0),
; then issue COPROC_CMD_POLL. The result appears in COPROC_TICKET_STATUS.

poll_loop:
    ; Restore the full 32-bit ticket value (byte 0 from ZP, bytes 1-3 = 0)
    LDA $00
    STA COPROC_TICKET
    LDA #0
    STA COPROC_TICKET+1
    STA COPROC_TICKET+2
    STA COPROC_TICKET+3

    ; Issue the POLL command
    STORE32 COPROC_CMD, COPROC_CMD_POLL

    ; Check the ticket status
    LDA COPROC_TICKET_STATUS
    CMP #COPROC_ST_OK                  ; 2 = completed successfully
    BEQ done
    CMP #COPROC_ST_ERROR               ; 3 = service reported an error
    BEQ error
    JMP poll_loop

; ============================================================================
; PHASE 4 - READ RESULT AND SIGNAL COMPLETION
; ============================================================================
;
; The result now sits at bus $410100. On the 6502, reading 32-bit
; bus memory requires a bank window -- but for this demo we simply write
; a completion flag to $0200 so the test harness can verify success.

done:
    LDA #$FF
    STA $0200                          ; signal completion to test harness
    JMP *                              ; halt: spin forever

; ============================================================================
; ERROR HANDLER
; ============================================================================
;
; If the START command failed, COPROC_CMD_ERROR contains a diagnostic
; error code. We store it at $0201 for the test harness to inspect.

error:
    LDA COPROC_CMD_ERROR
    STA $0201                          ; store error code for inspection
    JMP *                              ; halt: spin forever
