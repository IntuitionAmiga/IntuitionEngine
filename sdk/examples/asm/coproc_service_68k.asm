; ============================================================================
; COPROCESSOR SERVICE - M68K WORKER SIDE OF INTER-CPU COMMUNICATION
; Motorola 68020 syntax for IntuitionEngine - Headless coprocessor service
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    Motorola 68020 (32-bit, big-endian, register-rich)
; Video Chip:    None (headless coprocessor service)
; Audio Engine:  None
; Assembler:     vasmm68k_mot (Motorola syntax, DevPac compatible)
; Build:         vasmm68k_mot -Fbin -m68020 -devpac -o coproc_service_68k.ie68 coproc_service_68k.asm
; Run:           Used as -coproc argument with a matching caller binary
; Porting:       Same service implemented on all CPU cores. See
;                coproc_service_ie32.asm (reference), coproc_service_65.asm,
;                coproc_service_x86.asm, coproc_service_z80.asm.
;
; === WHAT THIS DEMO DOES ===
; 1. Polls the shared-memory ring buffer for incoming request descriptors
; 2. Dispatches on the op field (currently supports op=1: 32-bit add)
; 3. Reads two uint32 operands from the request data pointer
; 4. Performs a single 32-bit ADD.L instruction (native word size)
; 5. Writes the 32-bit result to the response data pointer
; 6. Fills in the response descriptor (ticket echo, status, result length)
; 7. Advances the ring tail pointer and loops back to poll
;
; === WHY COPROCESSOR MAILBOX IPC ===
; This is the SERVICE side of the coprocessor mailbox protocol. The service
; runs on a separate CPU core -- in this case a Motorola 68020 -- and polls
; shared memory for incoming commands from a caller running on a different
; CPU. When a new request appears (head != tail), the service processes it
; and writes the result back, then advances the tail to signal completion.
;
; The ring buffer acts as a hardware-neutral IPC channel: any CPU can be
; the caller and any CPU can be the service, because they communicate
; solely through memory-mapped addresses. This mirrors real coprocessor
; architectures throughout computing history:
;
;   - Amiga: The 68000 was the main CPU, but the copper and blitter acted
;     as coprocessors executing command lists from shared chip RAM. This
;     M68K service inverts that relationship -- here the 68K IS the
;     coprocessor, serving requests from another CPU
;   - SNES: The SA-1 coprocessor (a second 65C816) ran its own program
;     from shared ROM, communicating via hardware registers
;   - Modern GPUs: The CPU submits command buffers to a ring; the GPU
;     processes them asynchronously and signals completion
;
; === M68K-SPECIFIC NOTES ===
; The 68020's 32-bit registers and rich addressing modes make this service
; significantly more compact than the 8-bit implementations. The entire
; add operation is a single ADD.L instruction, versus four carry-chained
; byte adds on the 6502 or Z80. Address registers (A0-A3) hold pointers
; directly, eliminating the zero-page indirection needed on the 6502.
;
; The 68020 can also use displacement addressing (e.g., 8(a0) for
; entry+8) which lets us access entry fields without separate pointer
; arithmetic. This is why the M68K version has noticeably fewer
; instructions than the other ports.
;
; === MEMORY MAP ===
; $280000          Code entry point (WORKER_M68K_BASE)
; $820000          Mailbox base (MAILBOX_BASE)
; $820600          Ring 2 head pointer (M68K is ring index 2)
; $820601          Ring 2 tail pointer
; $820608+tail*32  Request entry descriptors (32 bytes each)
; $820808+tail*16  Response descriptors (16 bytes each)
;
; === BUILD AND RUN ===
; vasmm68k_mot -Fbin -m68020 -devpac -o coproc_service_68k.ie68 coproc_service_68k.asm
; (loaded by a caller binary via COPROC_CMD_START)
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

    include "ie68.inc"

    org $280000

; ============================================================================
; CONSTANTS - Ring Buffer Addresses
; ============================================================================
;
; The M68K is assigned ring index 2, so its ring lives at mailbox base
; + 2 * $300 = $820600. Each ring has 16 entry slots (32 bytes each)
; starting at offset +8, and 16 response slots (16 bytes each) starting
; at offset +$208.

RING_BASE   equ $820600
ENTRIES     equ RING_BASE+8
RESPONSES   equ RING_BASE+$208

; ============================================================================
; MAIN POLL LOOP - Wait for Requests
; ============================================================================
;
; WHY: The service spins on head != tail. The caller advances head when
; it enqueues a request; we advance tail when we finish processing one.
; Both are single bytes, so reads are atomic on any architecture.
;
; Register usage in the main loop:
;   D0 = tail (zero-extended from byte)
;   D1 = head (zero-extended from byte)
;   D2 = scratch for address computation
;   D3 = op field from entry
;   D4 = ticket from entry
;   D5 = computation result
;   A0 = entry descriptor pointer
;   A1 = request data pointer (reqPtr)
;   A2 = response data pointer (respPtr)
;   A3 = response descriptor pointer

main_loop:
    ; Read tail (byte)
    move.b  (RING_BASE+1),d0
    andi.l  #$FF,d0             ; D0 = tail

    ; Read head (byte)
    move.b  (RING_BASE),d1
    andi.l  #$FF,d1             ; D1 = head

    ; Compare: if tail == head, ring empty
    cmp.l   d1,d0
    beq     main_loop

; ============================================================================
; ENTRY ADDRESS COMPUTATION - Locate the Request Descriptor
; ============================================================================
;
; WHY: Each entry is 32 bytes, so entry address = ENTRIES + tail * 32.
; The 68020's barrel shifter computes tail*32 in a single LSL.L #5
; instruction. LEA loads the entries base into A0, then ADDA adds the
; offset -- a clean two-instruction sequence.

    ; Compute entry address: ENTRIES + tail*32
    move.l  d0,d2
    lsl.l   #5,d2               ; D2 = tail*32
    lea     ENTRIES,a0
    adda.l  d2,a0               ; A0 = entry address

; ============================================================================
; OPCODE DISPATCH - Check Which Operation Was Requested
; ============================================================================

    ; Read op at entry+8
    move.l  8(a0),d3            ; D3 = op
    cmpi.l  #1,d3
    bne     error_resp

; ============================================================================
; EXTRACT REQUEST FIELDS AND COMPUTE - Read Pointers, Perform Addition
; ============================================================================
;
; WHY: The 68020's address registers can load pointers directly from
; memory with displacement addressing. A1 = reqPtr, A2 = respPtr,
; D4 = ticket -- all extracted from the entry descriptor in three
; instructions. The actual computation (ADD.L) is a single opcode
; because the 68020 is a native 32-bit CPU.

    ; Read reqPtr at entry+16
    move.l  16(a0),a1           ; A1 = reqPtr

    ; Read respPtr at entry+24
    move.l  24(a0),a2           ; A2 = respPtr

    ; Read ticket at entry+0
    move.l  (a0),d4             ; D4 = ticket

    ; Op=1: add two uint32
    move.l  (a1),d5             ; D5 = val1
    add.l   4(a1),d5            ; D5 = val1 + val2
    move.l  d5,(a2)             ; write result

; ============================================================================
; WRITE RESPONSE DESCRIPTOR - Signal Completion to the Caller
; ============================================================================
;
; WHY: The response descriptor tells the caller what happened. We echo
; back the ticket for correlation, set status=2 (success), resultCode=0,
; and respLen=4 (one uint32 of result data). The 68020 can write all
; four fields with immediate MOVE.L instructions using displacement
; addressing -- no pointer arithmetic needed between writes.

    ; Compute response address: RESPONSES + tail*16
    move.l  d0,d2
    lsl.l   #4,d2               ; D2 = tail*16
    lea     RESPONSES,a3
    adda.l  d2,a3               ; A3 = response address

    ; Write response descriptor
    move.l  d4,(a3)             ; ticket
    move.l  #2,4(a3)            ; status = 2 (ok)
    move.l  #0,8(a3)            ; resultCode = 0
    move.l  #4,12(a3)           ; respLen = 4

; ============================================================================
; ADVANCE TAIL - Mark This Slot as Consumed
; ============================================================================
;
; WHY: Incrementing tail modulo 16 frees this ring slot for reuse.
; The AND mask ($0F) provides the wrap-around for a 16-entry ring.

    ; Advance tail: (tail + 1) & 15
    move.l  d0,d2
    addq.l  #1,d2
    andi.l  #$0F,d2
    move.b  d2,(RING_BASE+1)

    bra     main_loop

; ============================================================================
; ERROR RESPONSE - Unknown Opcode Handler
; ============================================================================
;
; WHY: If the caller sends an opcode we do not recognise, we must still
; advance the tail (otherwise the ring would stall forever). We write
; status=3 (error) and resultCode=1 (unknown op) so the caller knows
; the request was rejected rather than lost.

error_resp:
    ; Compute response address
    move.l  d0,d2
    lsl.l   #4,d2
    lea     RESPONSES,a3
    adda.l  d2,a3

    ; Write error response
    move.l  (a0),d4             ; ticket
    move.l  d4,(a3)
    move.l  #3,4(a3)            ; status = 3 (error)
    move.l  #1,8(a3)            ; resultCode = 1
    move.l  #0,12(a3)           ; respLen = 0

    ; Advance tail
    move.l  d0,d2
    addq.l  #1,d2
    andi.l  #$0F,d2
    move.b  d2,(RING_BASE+1)

    bra     main_loop
