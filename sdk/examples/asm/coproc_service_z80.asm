; ============================================================================
; COPROCESSOR SERVICE - Z80 WORKER SIDE OF INTER-CPU COMMUNICATION
; Z80 syntax for IntuitionEngine - Headless coprocessor service
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    Zilog Z80 (8-bit, 16-bit address bus via CoprocZ80Bus)
; Video Chip:    None (headless coprocessor service)
; Audio Engine:  None
; Assembler:     vasmz80_std (standard Z80 syntax)
; Build:         vasmz80_std -Fbin -o coproc_service_z80.ie80 coproc_service_z80.asm
; Run:           Used as -coproc argument with a matching caller binary
; Porting:       Same service implemented on all CPU cores. See
;                coproc_service_ie32.asm (reference), coproc_service_65.asm,
;                coproc_service_68k.asm, coproc_service_x86.asm.
;
; === WHAT THIS DEMO DOES ===
; 1. Polls the shared-memory ring buffer for incoming request descriptors
; 2. Dispatches on the op field (currently supports op=1: 32-bit add)
; 3. Reads two uint32 operands from the request data pointer
; 4. Performs a 4-byte carry-chained addition (the Z80 has no 32-bit ALU)
; 5. Writes the 32-bit result to the response data pointer via IX
; 6. Fills in the response descriptor (ticket echo, status, result length)
; 7. Advances the ring tail pointer and loops back to poll
;
; === WHY COPROCESSOR MAILBOX IPC ===
; This is the SERVICE side of the coprocessor mailbox protocol. The service
; runs on a separate CPU core -- in this case a Z80 -- and polls shared
; memory for incoming commands from a caller running on a different CPU.
; When a new request appears (head != tail), the service processes it and
; writes the result back, then advances the tail to signal completion.
;
; The ring buffer acts as a hardware-neutral IPC channel: any CPU can be
; the caller and any CPU can be the service, because they communicate
; solely through memory-mapped addresses. This mirrors real coprocessor
; architectures throughout computing history:
;
;   - Amiga: The 68000 writes command lists for the copper and blitter
;     coprocessors, which execute them independently from shared chip RAM
;   - SNES: The SA-1 coprocessor runs its own program from shared ROM,
;     communicating via hardware registers
;   - Sinclair Spectrum: The Z80 was the sole CPU, but many Spectrum
;     clones and add-ons used a second Z80 or coprocessor for I/O and
;     sound -- communicating through shared memory in the same way
;   - Modern GPUs: The CPU submits command buffers to a ring; the GPU
;     processes them asynchronously and signals completion
;
; === Z80-SPECIFIC NOTES ===
; Like the 6502, the Z80 is an 8-bit CPU with a 16-bit address bus, so
; all 32-bit operations require four carry-chained byte operations. The
; Z80 has more registers than the 6502 (BC, DE, HL, IX, IY) but no
; indirect-indexed addressing via zero page, so pointer manipulation
; uses 16-bit register pairs instead.
;
; The CoprocZ80Bus maps the mailbox into the Z80's 16-bit address space
; at $2000, and the Z80 is assigned ring index 3 (offset 3*$300 = $0900),
; so the ring base is at $2900.
;
; IX (index register) is used as the response data pointer because it
; supports displacement addressing (LD (IX+n), value), which lets us
; write individual result bytes without separate INC instructions. This
; is a Z80-specific advantage over the 6502, which must reload Y for
; each byte offset.
;
; GOTCHA: The Z80's PUSH/POP stack management is critical here. We save
; the entry base address on the stack across the addition, then restore
; it to read the ticket for the response. Getting the push/pop balance
; wrong will corrupt the return addresses and crash.
;
; === MEMORY MAP ===
; $0000          Code entry point (loaded at CPU address $0000)
; $2000          Mailbox base (mapped to bus MAILBOX_BASE)
; $2900          Ring 3 head pointer (Z80 is ring index 3)
; $2901          Ring 3 tail pointer
; $2908+tail*32  Request entry descriptors (32 bytes each)
; $2B08+tail*16  Response descriptors (16 bytes each)
;
; === BUILD AND RUN ===
; vasmz80_std -Fbin -o coproc_service_z80.ie80 coproc_service_z80.asm
; (loaded by a caller binary via COPROC_CMD_START)
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

    .include "ie80.inc"

    .org 0x0000

; ============================================================================
; CONSTANTS - Ring Buffer Addresses
; ============================================================================
;
; The Z80 is assigned ring index 3, so its ring lives at mailbox base
; + 3 * $300 = $2900 (after the $2000 base mapping). Each ring has
; 16 entry slots (32 bytes each) starting at offset +8, and 16 response
; slots (16 bytes each) starting at offset +$208.

.set RING_HEAD,    0x2900
.set RING_TAIL,    0x2901
.set ENTRIES_BASE, 0x2908
.set RESP_BASE,    0x2B08

; ============================================================================
; MAIN POLL LOOP - Wait for Requests
; ============================================================================
;
; WHY: The service spins on head != tail. B holds the tail value for
; comparison against A (head). CP B sets the zero flag if they match,
; and JR Z loops back to poll. This is the tightest possible poll loop
; on the Z80 -- just three instructions before the branch.

main_loop:
    ; Read tail
    LD A, (RING_TAIL)
    LD B, A                 ; B = tail

    ; Read head
    LD A, (RING_HEAD)

    ; Compare: if tail == head, ring empty
    CP B
    JR Z, main_loop         ; empty, poll

; ============================================================================
; ENTRY ADDRESS COMPUTATION - Locate the Request Descriptor
; ============================================================================
;
; WHY: Each entry is 32 bytes, so entry address = ENTRIES_BASE + tail*32.
; The Z80 has no multiply or barrel shifter, so we compute tail*32 by
; loading tail into HL and doubling it five times with ADD HL, HL (the
; Z80's only 16-bit addition instruction). This is the same technique
; as the 6502's five ASL shifts, but operating on a 16-bit register pair.

    ; Compute entry address: ENTRIES_BASE + tail*32
    ; HL = ENTRIES_BASE + B*32
    LD H, 0
    LD L, B
    ; *32 = shift left 5
    ADD HL, HL              ; *2
    ADD HL, HL              ; *4
    ADD HL, HL              ; *8
    ADD HL, HL              ; *16
    ADD HL, HL              ; *32
    LD DE, ENTRIES_BASE
    ADD HL, DE              ; HL = entry address

; ============================================================================
; OPCODE DISPATCH - Check Which Operation Was Requested
; ============================================================================

    ; Read op at entry+8 (low byte only)
    PUSH HL
    LD DE, 8
    ADD HL, DE              ; HL = entry+8
    LD A, (HL)              ; A = op low byte
    POP HL
    CP 1
    JP NZ, error_resp

; ============================================================================
; EXTRACT REQUEST FIELDS - Read Pointers from the Entry Descriptor
; ============================================================================
;
; WHY: We need three values from the entry: reqPtr (entry+16),
; respPtr (entry+24), and ticket (entry+0). The Z80 cannot hold all
; these in registers simultaneously, so we use the stack to juggle them.
;
; The pointer extraction sequence is:
;   1. Read reqPtr into DE (entry+16, low 16 bits)
;   2. Save reqPtr on stack, read respPtr into DE (entry+24)
;   3. Restore reqPtr into BC (from stack)
;   4. Load respPtr into IX (for displacement-addressed result writes)
;
; GOTCHA: The push/pop sequence here is delicate. BC ends up holding
; reqPtr (swapped from DE via stack), IX holds respPtr, and HL holds
; the entry base. A mismatched push/pop will corrupt everything.

    ; Re-read reqPtr: entry + 16
    PUSH HL                 ; save entry base
    LD DE, 16
    ADD HL, DE
    LD E, (HL)
    INC HL
    LD D, (HL)              ; DE = reqPtr
    POP HL
    PUSH HL

    ; Get respPtr: entry + 24
    PUSH DE                 ; save reqPtr
    PUSH HL
    LD DE, 24
    ADD HL, DE
    LD E, (HL)
    INC HL
    LD D, (HL)              ; DE = respPtr
    POP HL
    POP BC                  ; BC = reqPtr (was in DE)
    PUSH DE                 ; save respPtr

; ============================================================================
; OP=1: 32-BIT ADDITION - Carry-Chained Byte-by-Byte Add via IX
; ============================================================================
;
; WHY: Like the 6502, the Z80 is an 8-bit CPU and must chain four byte
; additions with carry. However, the Z80 has an advantage: IX supports
; displacement addressing (LD (IX+n), A), so we can write result bytes
; at specific offsets without manually incrementing the pointer between
; each byte. DE points to val1, HL points to val2 (reqPtr+4), and IX
; holds the response data pointer.
;
; The carry chain works identically to the 6502 version:
;   Byte 0: ADD (no carry in)
;   Byte 1: ADC (carry from byte 0)
;   Byte 2: ADC (carry from byte 1)
;   Byte 3: ADC (carry from byte 2)

    ; Now: BC = reqPtr, stack has respPtr and entry base
    ; Set up for 32-bit add: val1 at (BC), val2 at (BC+4), result to respPtr
    POP IX                  ; IX = respPtr
    POP HL                  ; HL = entry base
    PUSH HL                 ; save entry base again

    ; Load reqPtr into DE (from BC)
    LD D, B
    LD E, C                 ; DE = reqPtr

    ; Compute reqPtr+4 in HL
    LD H, B
    LD L, C
    PUSH DE
    LD DE, 4
    ADD HL, DE              ; HL = reqPtr+4
    POP DE

    ; Byte 0
    LD A, (DE)              ; val1[0]
    ADD A, (HL)             ; + val2[0]
    LD (IX+0), A

    ; Byte 1
    INC DE
    INC HL
    LD A, (DE)
    ADC A, (HL)
    LD (IX+1), A

    ; Byte 2
    INC DE
    INC HL
    LD A, (DE)
    ADC A, (HL)
    LD (IX+2), A

    ; Byte 3
    INC DE
    INC HL
    LD A, (DE)
    ADC A, (HL)
    LD (IX+3), A

    POP HL                  ; restore entry base

; ============================================================================
; WRITE RESPONSE DESCRIPTOR - Signal Completion to the Caller
; ============================================================================
;
; WHY: The response descriptor tells the caller what happened. We must
; compute RESP_BASE + tail*16 (same shift-by-4 technique as the entry
; address, but only four doublings instead of five).
;
; The ticket is copied byte-by-byte from the entry descriptor (pointed
; to by DE after we restore entry base). Then status, resultCode, and
; respLen are written as little-endian uint32 values, one byte at a time.
; Each field is 4 bytes, with only the low byte non-zero in our case.

    ; Write response descriptor at RESP_BASE + tail*16
    PUSH HL                 ; save entry base
    LD A, (RING_TAIL)
    LD L, A
    LD H, 0
    ADD HL, HL              ; *2
    ADD HL, HL              ; *4
    ADD HL, HL              ; *8
    ADD HL, HL              ; *16
    LD DE, RESP_BASE
    ADD HL, DE              ; HL = response addr

    ; Read ticket from entry (we need entry base from stack)
    POP DE                  ; DE = entry base
    PUSH HL                 ; save response addr
    LD A, (DE)              ; ticket byte 0
    LD (HL), A
    INC HL
    INC DE
    LD A, (DE)              ; ticket byte 1
    LD (HL), A
    INC HL
    LD (HL), 0              ; ticket byte 2
    INC HL
    LD (HL), 0              ; ticket byte 3
    INC HL

    ; status = 2 (ok)
    LD (HL), 2
    INC HL
    LD (HL), 0
    INC HL
    LD (HL), 0
    INC HL
    LD (HL), 0
    INC HL

    ; resultCode = 0
    LD (HL), 0
    INC HL
    LD (HL), 0
    INC HL
    LD (HL), 0
    INC HL
    LD (HL), 0
    INC HL

    ; respLen = 4
    LD (HL), 4
    INC HL
    LD (HL), 0
    INC HL
    LD (HL), 0
    INC HL
    LD (HL), 0

    POP HL                  ; discard saved response addr

; ============================================================================
; ADVANCE TAIL - Mark This Slot as Consumed
; ============================================================================
;
; WHY: Incrementing tail modulo 16 frees this ring slot for reuse.
; AND 0x0F provides the wrap-around for the 16-entry ring.

    ; Advance tail
    LD A, (RING_TAIL)
    INC A
    AND 0x0F
    LD (RING_TAIL), A

    JP main_loop

; ============================================================================
; ERROR RESPONSE - Unknown Opcode Handler
; ============================================================================
;
; WHY: If the caller sends an opcode we do not recognise, we must still
; advance the tail (otherwise the ring would stall forever). We write
; status=3 (error) so the caller knows the request was rejected. Only
; the ticket and status fields are filled in for brevity -- the caller
; checks status first and ignores the other fields on error.

error_resp:
    ; Write error response (status=3)
    PUSH HL                 ; save entry base
    LD A, (RING_TAIL)
    LD L, A
    LD H, 0
    ADD HL, HL
    ADD HL, HL
    ADD HL, HL
    ADD HL, HL
    LD DE, RESP_BASE
    ADD HL, DE

    POP DE                  ; DE = entry base

    ; ticket
    LD A, (DE)
    LD (HL), A
    INC HL
    INC DE
    LD A, (DE)
    LD (HL), A
    INC HL
    LD (HL), 0
    INC HL
    LD (HL), 0
    INC HL

    ; status = 3
    LD (HL), 3
    INC HL
    LD (HL), 0
    INC HL
    LD (HL), 0
    INC HL
    LD (HL), 0

    ; Advance tail
    LD A, (RING_TAIL)
    INC A
    AND 0x0F
    LD (RING_TAIL), A

    JP main_loop
