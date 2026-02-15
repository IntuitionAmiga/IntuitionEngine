; ============================================================================
; COPROCESSOR SERVICE - WORKER SIDE OF INTER-CPU COMMUNICATION
; IE32 Assembly for IntuitionEngine - Ring Buffer Service Template
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    IE32 (custom 32-bit RISC) - worker/service side
; Video Chip:    None (headless worker)
; Audio Engine:  None
; Assembler:     ie32asm (built-in IE32 assembler)
; Build:         ./bin/ie32asm sdk/examples/asm/coproc_service_ie32.asm
; Run:           (launched by caller via COPROC_CMD_START)
; Porting:       Mailbox protocol is CPU-agnostic. Any CPU core can implement
;                the service side using the same memory-mapped ring buffer.
;
; === SERVICE BINARY CONTRACT ===
;   1. Poll ring buffer for requests
;   2. Dispatch on op field
;   3. Op=1 (add): read two uint32 from reqPtr, add, write to respPtr
;   4. Write response descriptor (status=2/ok, respLen=4)
;   5. Advance tail, loop
;
; === MEMORY MAP ===
;   Code loaded at 0x200000 (WORKER_IE32_BASE)
;   Mailbox at 0x820000 (MAILBOX_BASE)
;   Ring 0 (IE32): 0x820000 + 0*0x300 = 0x820000
;     head:     0x820000 (uint8)
;     tail:     0x820001 (uint8)
;     entries:  0x820008 + tail*32 (request descriptors)
;     responses:0x820208 + tail*16 (response descriptors)
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

    org 0x200000

; Constants
RING_BASE   equ 0x820000
ENTRIES     equ RING_BASE + 0x08
RESPONSES   equ RING_BASE + 0x208

; Register usage:
;   A (r0) = scratch
;   X (r1) = tail
;   Y (r2) = head / scratch
;   Z (r3) = scratch
;   B (r4) = entry address
;   C (r5) = scratch
;   D (r6) = response address

main_loop:
    ; Read tail
    load    A, RING_BASE + 1        ; A = mem[tail_addr]
    and     A, #0xFF                ; mask to byte
    move    X, A                    ; X = tail

    ; Read head
    load    A, RING_BASE + 0        ; A = mem[head_addr]
    and     A, #0xFF                ; mask to byte

    ; Compare: if tail == head, ring empty
    sub     A, X                    ; A = head - tail
    jz      A, main_loop            ; if equal, poll again

    ; Compute entry address: ENTRIES + tail * 32
    move    B, X                    ; B = tail
    shl     B, #5                   ; B = tail * 32
    add     B, #ENTRIES              ; B = entry base

    ; Read request fields using direct addressing
    ; ticket at B+0, op at B+8, reqPtr at B+16, reqLen at B+20
    ; respPtr at B+24, respCap at B+28

    ; For IE32, we need register-indirect addressing.
    ; ADDR_REG_IND: operand = (offset & ~0xF) | regIdx
    ; B = register 4

    ; Read op: entry + 8 → C
    ; reg_ind: reg=B(4), offset=8 → operand = (8 & ~0xF) | 4 = 0x04
    ; But offset 8 & ~0xF = 0, so we lose the offset!
    ; IE32 reg_ind encoding: offset = operand & 0xFFFFFFF0
    ; For offset 8, we need operand with bits[31:4] = 8
    ; But 8 in bits[31:4] means operand = 0x80 | 4 = 0x84
    ; Actually: (operand & 0xFFFFFFF0) = offset, (operand & 0x0F) = reg
    ; So offset=8: operand = 0x08 | 4 = 0x0C → but 0x0C & 0xFFFFFFF0 = 0x00, not 8!
    ; This means we can't address offsets < 16 with reg_ind.
    ; Workaround: add offset to B directly.

    ; Read op
    move    Z, B                    ; Z = entry addr
    add     Z, #8                   ; Z = entry + 8 (op offset)
    load    C, [Z]                  ; C = op

    ; Check op == 1 (add)
    move    A, C
    sub     A, #1
    jnz     A, write_error          ; unsupported op

    ; Read reqPtr: entry + 16
    move    Z, B
    add     Z, #16
    load    C, [Z]                  ; C = reqPtr

    ; Read respPtr: entry + 24
    move    Z, B
    add     Z, #24
    load    D, [Z]                  ; D = respPtr

    ; Read ticket: entry + 0
    load    Z, [B]                  ; Z = ticket

    ; Op=1: add two uint32 from reqPtr
    ; val1 = mem[reqPtr], val2 = mem[reqPtr+4]
    load    A, [C]                  ; A = val1
    move    Y, C
    add     Y, #4
    load    Y, [Y]                  ; Y = val2
    add     A, Y                    ; A = val1 + val2

    ; Write result to respPtr
    store   A, [D]

    ; Write response descriptor: RESPONSES + tail * 16
    move    A, X                    ; A = tail
    shl     A, #4                   ; A = tail * 16
    add     A, #RESPONSES            ; A = response addr

    ; response.ticket = Z (ticket)
    store   Z, [A]

    ; response.status = 2 (ok)
    move    Y, A
    add     Y, #4
    move    C, #2
    store   C, [Y]

    ; response.resultCode = 0
    add     Y, #4
    move    C, #0
    store   C, [Y]

    ; response.respLen = 4
    add     Y, #4
    move    C, #4
    store   C, [Y]

    ; Advance tail: (tail + 1) & 15
    move    A, X
    add     A, #1
    and     A, #0x0F
    store   A, RING_BASE + 1

    jmp     main_loop

write_error:
    ; Unsupported op — write error response
    move    A, X                    ; A = tail
    shl     A, #4
    add     A, #RESPONSES

    ; ticket
    load    Z, [B]
    store   Z, [A]

    ; status = 3 (error)
    move    Y, A
    add     Y, #4
    move    C, #3
    store   C, [Y]

    ; resultCode = 1 (unknown op)
    add     Y, #4
    move    C, #1
    store   C, [Y]

    ; respLen = 0
    add     Y, #4
    move    C, #0
    store   C, [Y]

    ; Advance tail
    move    A, X
    add     A, #1
    and     A, #0x0F
    store   A, RING_BASE + 1

    jmp     main_loop
