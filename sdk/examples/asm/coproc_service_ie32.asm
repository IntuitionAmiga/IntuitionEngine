; Coprocessor mailbox service: IE32 guest code that waits for mailbox entries,
; reads an add request from shared guest RAM, writes the response and advances
; the ring tail.
;
; It shares the descriptor layout with every coprocessor port. The Host SDK builds
; the service binary, but the running guest communicates only through shared RAM
; and coprocessor MMIO. Read constants, the poll loop, response path and error
; path in order. Run a caller with `go run . -ie32 -coproc-svc <service> <caller>`.
; Compare coproc_caller_ie32.asm as its matching pair.

    .org 0x200000

; ============================================================================
; CONSTANTS - Ring Buffer Addresses
; ============================================================================
;
; The IE32 is assigned ring index 0, so its ring lives directly at the
; mailbox base ($790000). Each ring has 16 entry slots (32 bytes each)
; starting at offset +8, and 16 response slots (16 bytes each) starting
; at offset +$208.

.equ RING_HEAD   0x790000
.equ RING_TAIL   0x790001
.equ RING_ACK    0x790004
.equ ENTRIES     0x790008
.equ RESPONSES   0x790208

; Mailbox layout version echoed back at startup (COPROC_LAYOUT_VERSION).
.equ LAYOUT_VER  1

; ============================================================================
; REGISTER ALLOCATION
; ============================================================================
;
; On a register-starved RISC like the IE32 (only 7 GPRs), careful
; allocation avoids unnecessary spills to memory. This mapping holds
; throughout the main processing path:
;
;   A (r0) = scratch / computation result
;   X (r1) = tail (preserved across the entire request)
;   Y (r2) = head / scratch
;   Z (r3) = scratch for pointer arithmetic
;   B (r4) = entry base address
;   C (r5) = scratch
;   D (r6) = response data pointer (respPtr)

; ============================================================================
; MAIN POLL LOOP - Wait for Requests
; ============================================================================
;
; The service spins on head != tail. When the caller enqueues a
; request, it advances head. We detect this by comparing our cached tail
; against the current head value. Both are single bytes masked to 8 bits,
; so reads are effectively atomic.

    ; Version-gate handshake: echo the mailbox layout version into the ring
    ; header so the host routes work to this service. A stale image built for
    ; a different layout never reaches this address and is refused at START.
    LDA #LAYOUT_VER
    STA @RING_ACK

main_loop:
    ; Read tail
    LDA @RING_TAIL                  ; A = mem[tail_addr]
    AND A, #0xFF                    ; mask to byte
    LDX A                           ; X = tail

    ; Read head
    LDA @RING_HEAD                  ; A = mem[head_addr]
    AND A, #0xFF                    ; mask to byte

    ; Compare: if tail == head, ring empty
    SUB A, X                        ; A = head - tail
    JZ  A, main_loop                ; if equal, poll again

; ============================================================================
; ENTRY ADDRESS COMPUTATION - Locate the Request Descriptor
; ============================================================================
;
; Each entry is 32 bytes, so entry address = ENTRIES + tail * 32.
; SHL #5 computes the multiply. The result is held in B (r4) for the
; duration of request processing, since we need to read multiple fields
; from the entry at different offsets.

    ; Compute entry address: ENTRIES + tail * 32
    LDB X                           ; B = tail
    SHL B, #5                       ; B = tail * 32
    ADD B, #ENTRIES                  ; B = entry base

; ============================================================================
; OPCODE DISPATCH - Check Which Operation Was Requested
; ============================================================================
;
; The op field at entry+8 tells us what computation to perform.
; On the IE32 (load-store architecture), we must compute the field
; address in a scratch register, then load through it with [Z] syntax.
; Currently only op=1 (add two uint32 values) is supported.
;
; GOTCHA: We cannot use register-indirect with offset 8 directly because
; the IE32 reg_ind encoding requires offset >= 16 in the high bits.
; Instead we copy B to Z, add the offset manually, then load via [Z].

    ; Read op: entry + 8
    LDZ B                           ; Z = entry addr
    ADD Z, #8                       ; Z = entry + 8 (op offset)
    LDC [Z]                         ; C = op

    ; Check op == 1 (add)
    LDA C
    SUB A, #1
    JNZ A, write_error              ; unsupported op

; ============================================================================
; EXTRACT REQUEST FIELDS - Read Pointers from the Entry Descriptor
; ============================================================================
;
; reqPtr (entry+16) points to the input operands. respPtr (entry+24)
; points to where we write the result. ticket (entry+0) is an opaque
; value echoed back in the response for caller correlation.
;
; Each field requires a fresh pointer computation because the IE32 has
; no displacement addressing mode -- we must manually ADD the offset
; to the base address each time.

    ; Read reqPtr: entry + 16
    LDZ B
    ADD Z, #16
    LDC [Z]                         ; C = reqPtr

    ; Read respPtr: entry + 24
    LDZ B
    ADD Z, #24
    LDD [Z]                         ; D = respPtr

    ; Read ticket: entry + 0
    LDZ [B]                         ; Z = ticket

; ============================================================================
; OP=1: 32-BIT ADDITION - Native Word-Size Operation
; ============================================================================
;
; The IE32 is a 32-bit CPU, so the addition is a single ADD
; instruction. We load val1 from [reqPtr], val2 from [reqPtr+4],
; add them, and store the result to [respPtr]. The register-indirect
; loads ([C] and [Y]) dereference the pointers we extracted above.

    ; Op=1: add two uint32 from reqPtr
    ; val1 = mem[reqPtr], val2 = mem[reqPtr+4]
    LDA [C]                         ; A = val1
    LDY C
    ADD Y, #4
    LDY [Y]                         ; Y = val2
    ADD A, Y                        ; A = val1 + val2

    ; Write result to respPtr
    STA [D]

; ============================================================================
; WRITE RESPONSE DESCRIPTOR - Signal Completion to the Caller
; ============================================================================
;
; The response descriptor tells the caller what happened. It lives
; in a separate array from the request entries (RESPONSES + tail*16).
; We fill in four fields, each a uint32:
;   +0  ticket    -- echoed from the request for correlation
;   +4  status    -- 2 = success, 3 = error
;   +8  resultCode -- 0 for success
;   +12 respLen   -- number of valid bytes in the response data buffer
;
; On the IE32, each field write requires computing the target address
; by incrementing Y (our running pointer into the response descriptor).

    ; Write response descriptor: RESPONSES + tail * 16
    LDA X                           ; A = tail
    SHL A, #4                       ; A = tail * 16
    ADD A, #RESPONSES                ; A = response addr

    ; response.ticket = Z (ticket)
    STZ [A]

    ; response.status = 2 (ok)
    LDY A
    ADD Y, #4
    LDC #2
    STC [Y]

    ; response.resultCode = 0
    ADD Y, #4
    LDC #0
    STC [Y]

    ; response.respLen = 4
    ADD Y, #4
    LDC #4
    STC [Y]

; ============================================================================
; ADVANCE TAIL - Mark This Slot as Consumed
; ============================================================================
;
; Incrementing tail modulo 16 (AND #0x0F) frees this ring slot
; for reuse by the caller. The 16-entry ring wraps naturally with a
; 4-bit mask.

    ; Advance tail: (tail + 1) & 15
    LDA X
    ADD A, #1
    AND A, #0x0F
    STA @RING_TAIL

    JMP main_loop

; ============================================================================
; ERROR RESPONSE - Unknown Opcode Handler
; ============================================================================
;
; If the caller sends an opcode we do not recognise, we must still
; advance the tail (otherwise the ring would stall forever). We write
; status=3 (error) and resultCode=1 (unknown op) so the caller knows
; the request was rejected rather than silently lost.

write_error:
    ; Unsupported op -- write error response
    LDA X                           ; A = tail
    SHL A, #4
    ADD A, #RESPONSES

    ; ticket
    LDZ [B]
    STZ [A]

    ; status = 3 (error)
    LDY A
    ADD Y, #4
    LDC #3
    STC [Y]

    ; resultCode = 1 (unknown op)
    ADD Y, #4
    LDC #1
    STC [Y]

    ; respLen = 0
    ADD Y, #4
    LDC #0
    STC [Y]

    ; Advance tail
    LDA X
    ADD A, #1
    AND A, #0x0F
    STA @RING_TAIL

    JMP main_loop
