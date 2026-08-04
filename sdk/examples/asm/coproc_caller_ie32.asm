; Coprocessor mailbox caller: IE32 guest code that starts the matching worker, writes two operands to shared guest RAM, enqueues an add request and polls its ticket.
;
; The caller and service communicate through shared guest RAM and coprocessor MMIO.
; The Host SDK builds both guest binaries; it is not present at runtime. Read the
; mailbox constants, initialisation, descriptor exchange and terminal path in order.
; Run the caller with `go run . -ie32 -coproc-svc <service> <caller>`.
; Compare coproc_service_ie32.asm as its matching pair and the other CPU ports for their
; real addressing differences.
;


    .include "ie32.inc"

    .org 0x1000

; ============================================================================
; SHARED MEMORY LAYOUT
; ============================================================================
;
; The caller and service communicate through three shared memory regions:
;
;   0x400000: Service filename string (host pre-loads the binary path here)
;   0x410000: Request data  - two uint32 values (val1 at +0, val2 at +4)
;   0x410100: Response buffer - one uint32 result
;
; These addresses are arbitrary but must match between caller and service.
; They sit above VRAM (0x100000-0x22C000) to avoid any overlap.

; ============================================================================
; PHASE 1 - START THE COPROCESSOR WORKER
; ============================================================================
;
; Before we can send work, we must launch the service binary on a separate
; CPU core. We write the CPU type and binary path to the coprocessor control
; registers, then issue the START command. The host loads the service binary
; and begins executing it on a new IE32 core.

    LOAD    A, #COPROC_CPU_IE32
    STORE   A, COPROC_CPU_TYPE
    LOAD    A, #0x400000
    STORE   A, COPROC_NAME_PTR
    LOAD    A, #COPROC_CMD_START
    STORE   A, COPROC_CMD

    ; Check that the start succeeded - status 0 means OK
    LOAD    A, COPROC_CMD_STATUS
    JNZ     A, error                   ; if status != 0, start failed

; ============================================================================
; PHASE 2 - WRITE REQUEST DATA TO SHARED MEMORY
; ============================================================================
;
; We want the service to compute 10 + 20 = 30. The request format is
; simply two consecutive uint32 values at the request address. The service
; will read both values, add them, and write the sum to the response buffer.

    LOAD    A, #10
    STORE   A, 0x410000                ; val1 = 10
    LOAD    A, #20
    STORE   A, 0x410004                ; val2 = 20

; ============================================================================
; PHASE 3 - ENQUEUE THE REQUEST VIA MAILBOX REGISTERS
; ============================================================================
;
; The mailbox protocol requires us to set several parameters before
; issuing the ENQUEUE command:
;   - CPU type:     which coprocessor to target
;   - Operation:    what to do (1 = add)
;   - Request ptr:  where to find the input data
;   - Request len:  how many bytes of input (8 = two uint32s)
;   - Response ptr: where to write the output
;   - Response cap: how many bytes of output space (4 = one uint32)
;
; After ENQUEUE, the COPROC_TICKET register contains a unique ticket
; number that we use to track this specific request.

    LOAD    A, #COPROC_CPU_IE32
    STORE   A, COPROC_CPU_TYPE
    LOAD    A, #1                      ; op = add
    STORE   A, COPROC_OP
    LOAD    A, #0x410000
    STORE   A, COPROC_REQ_PTR
    LOAD    A, #8
    STORE   A, COPROC_REQ_LEN
    LOAD    A, #0x410100
    STORE   A, COPROC_RESP_PTR
    LOAD    A, #4
    STORE   A, COPROC_RESP_CAP
    LOAD    A, #COPROC_CMD_ENQUEUE
    STORE   A, COPROC_CMD

    ; Save the ticket - we need this to poll for completion
    LOAD    X, COPROC_TICKET           ; X = ticket number

; ============================================================================
; PHASE 4 - POLL FOR COMPLETION
; ============================================================================
;
; The caller has no way to know when the service will finish processing.
; We poll by writing our ticket to COPROC_TICKET, issuing a POLL command,
; and reading COPROC_TICKET_STATUS. When the status equals COPROC_ST_OK (2),
; the result is ready in the response buffer.
;
; This is analogous to GPU fence polling: submit work, get a fence value,
; spin until the fence signals completion. In production code you might
; yield or do other work between polls, but for this demo we busy-wait.

poll_loop:
    STORE   X, COPROC_TICKET
    LOAD    A, #COPROC_CMD_POLL
    STORE   A, COPROC_CMD
    LOAD    A, COPROC_TICKET_STATUS
    SUB     A, #COPROC_ST_OK           ; compare with OK (2)
    JZ      A, done
    LOAD    A, COPROC_TICKET_STATUS
    SUB     A, #COPROC_ST_ERROR        ; compare with ERROR (3)
    JZ      A, error
    JMP     poll_loop

; ============================================================================
; PHASE 5 - READ THE RESULT
; ============================================================================
;
; The service has written the sum (10 + 20 = 30) to address 0x410100.
; We load it into register A and halt. In a real application, this result
; would feed into further computation or be displayed to the user.

done:
    LOAD    A, 0x410100                ; A = result (should be 30)
    HALT

; ============================================================================
; ERROR HANDLER
; ============================================================================
;
; If the coprocessor START command fails (e.g. binary not found, CPU type
; unsupported), we read the error code and halt. The error code can be
; inspected in a debugger to diagnose the failure.

error:
    LOAD    A, COPROC_CMD_ERROR
    HALT
