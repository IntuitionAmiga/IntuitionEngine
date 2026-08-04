; Coprocessor mailbox caller: Z80 guest code that starts the matching worker, writes two operands to shared guest RAM, enqueues an add request and polls its ticket.
;
; The caller and service communicate through shared guest RAM and coprocessor MMIO.
; The Host SDK builds both guest binaries; it is not present at runtime. Read the
; mailbox constants, initialisation, descriptor exchange and terminal path in order.
; Run the caller with `go run . -z80 -coproc-svc <service> <caller>`.
; Compare coproc_service_z80.asm as its matching pair and the other CPU ports for their
; real addressing differences.
;


    .include "ie80.inc"

    .org 0x0000

; ============================================================================
; GATEWAY WINDOW OVERVIEW
; ============================================================================
; The gateway at 0xF200-0xF23F maps to bus 0xF2340-0xF237F.
; All COPROC registers are accessed via byte-level writes using STORE32.
; Data buffers use bank windows to access bus memory above 0xFFFF.

; ============================================================================
; PHASE 1 - START THE M68K WORKER
; ============================================================================
;
; Before we can enqueue any work, the coprocessor controller needs
; to know which CPU type to spawn and where to find the service binary.
; COPROC_CPU_TYPE selects the worker architecture (Z80 in this case);
; COPROC_NAME_PTR points to a null-terminated filename string in bus
; memory. Writing COPROC_CMD with COPROC_CMD_START triggers the launch.
; We then check CMD_STATUS to confirm the worker started without error.
;
; Note: this Z80 caller launches a matching Z80 worker by convention.

    ; Select Z80 as the worker CPU type
    STORE32 COPROC_CPU_TYPE, COPROC_CPU_Z80

    ; Point to the service binary filename in bus memory
    ; (must be pre-loaded into bus memory by the host before execution)
    STORE32 COPROC_NAME_PTR, 0x400000

    ; Trigger the START command
    STORE32 COPROC_CMD, COPROC_CMD_START

    ; Check whether the start succeeded (0 = OK, non-zero = error)
    ; Z80 idiom: `or a` sets the zero flag if A == 0
    ld a,(COPROC_CMD_STATUS)
    or a
    jp nz,error

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
; The request buffer at 0x410000 contains two uint32 values (the operands).
; The response buffer at 0x410100 will receive one uint32 (the sum).
; These buffers must be pre-populated by the host environment.

    STORE32 COPROC_CPU_TYPE, COPROC_CPU_Z80
    STORE32 COPROC_OP, 1                ; op = add
    STORE32 COPROC_REQ_PTR, 0x410000    ; request data at bus 0x410000
    STORE32 COPROC_REQ_LEN, 8           ; two uint32 = 8 bytes
    STORE32 COPROC_RESP_PTR, 0x410100   ; response buffer
    STORE32 COPROC_RESP_CAP, 4          ; capacity: 4 bytes (one uint32)
    STORE32 COPROC_CMD, COPROC_CMD_ENQUEUE

    ; Save the returned ticket number into register B
    ; (tickets are small integers, so only byte 0 matters on the Z80)
    ld a,(COPROC_TICKET)
    ld b,a                             ; B = ticket

; ============================================================================
; PHASE 3 - POLL UNTIL COMPLETE
; ============================================================================
;
; The worker processes the request asynchronously. We must poll the
; ticket status until it transitions from PENDING (0) or RUNNING (1) to
; a terminal state. COPROC_ST_OK (2) means the response buffer is valid.
;
; To poll, we write the ticket number back into the COPROC_TICKET register
; (all 4 bytes, clearing the upper 3 since the Z80 stored only byte 0),
; then issue COPROC_CMD_POLL. The result appears in COPROC_TICKET_STATUS.

poll_loop:
    ; Restore the full 32-bit ticket value (byte 0 from B, bytes 1-3 = 0)
    ld a,b
    ld (COPROC_TICKET),a
    ld a,0
    ld (COPROC_TICKET+1),a
    ld (COPROC_TICKET+2),a
    ld (COPROC_TICKET+3),a

    ; Issue the POLL command
    STORE32 COPROC_CMD, COPROC_CMD_POLL

    ; Check the ticket status
    ld a,(COPROC_TICKET_STATUS)
    cp COPROC_ST_OK                    ; 2 = completed successfully
    jr z,done
    cp COPROC_ST_ERROR                 ; 3 = service reported an error
    jr z,error
    jr poll_loop

; ============================================================================
; PHASE 4 - COMPLETION
; ============================================================================
;
; The result now sits at bus 0x410100. On the Z80, reading 32-bit
; bus memory requires a bank window -- but for this demo we simply halt,
; and the test harness verifies the response buffer contents directly.

done:
    halt

; ============================================================================
; ERROR HANDLER
; ============================================================================
;
; If the START command failed, COPROC_CMD_ERROR contains a diagnostic
; error code. We load it into A for inspection and halt.

error:
    ld a,(COPROC_CMD_ERROR)
    halt
