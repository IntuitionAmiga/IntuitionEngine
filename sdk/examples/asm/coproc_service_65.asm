; ============================================================================
; COPROCESSOR SERVICE - 6502 WORKER SIDE OF INTER-CPU COMMUNICATION
; ca65/ld65 syntax for IntuitionEngine - Headless coprocessor service
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    MOS 6502 (8-bit, 16-bit address bus via CoprocBus32)
; Video Chip:    None (headless coprocessor service)
; Audio Engine:  None
; Assembler:     ca65/ld65 (cc65 toolchain)
; Build:         make ie65asm SRC=sdk/examples/asm/coproc_service_65.asm
; Linker cfg:    ie65_service.cfg (auto-detected by SDK build)
; Run:           Used as -coproc argument with a matching caller binary
; Porting:       Same service implemented on all CPU cores. See
;                coproc_service_ie32.asm (reference), coproc_service_68k.asm,
;                coproc_service_x86.asm, coproc_service_z80.asm.
;
; === WHAT THIS DEMO DOES ===
; 1. Polls the shared-memory ring buffer for incoming request descriptors
; 2. Dispatches on the op field (currently supports op=1: 32-bit add)
; 3. Reads two uint32 operands from the request data pointer
; 4. Performs a 4-byte carry-chained addition (the 6502 has no 32-bit ALU)
; 5. Writes the 32-bit result to the response data pointer
; 6. Fills in the response descriptor (ticket echo, status, result length)
; 7. Advances the ring tail pointer and loops back to poll
;
; === WHY COPROCESSOR MAILBOX IPC ===
; This is the SERVICE side of the coprocessor mailbox protocol. The service
; runs on a separate CPU core -- in this case a 6502 -- and polls shared
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
;     coprocessors, which execute them independently from shared memory
;   - SNES: The SA-1 coprocessor (a second 65C816) runs its own program
;     from shared ROM, communicating via hardware registers
;   - Modern GPUs: The CPU submits command buffers to a ring; the GPU
;     processes them asynchronously and signals completion
;
; === 6502-SPECIFIC NOTES ===
; The 6502's 8-bit accumulator means every 32-bit operation becomes four
; separate byte operations with carry propagation. The CoprocBus32 maps
; the mailbox into the 6502's 16-bit address space starting at $2000,
; so all ring buffer addresses fit within the $0000-$FFFF range.
;
; Zero page ($00-$0F) is used as scratch storage for computed pointers,
; since the 6502's indirect addressing modes require zero-page operands.
; This is a standard 6502 idiom -- zero page acts as a register file
; extension for an architecture with very few registers.
;
; === MEMORY MAP ===
; $0000-$00FF    Zero page (scratch pointers and temporaries)
; $0000          Code entry point (reset vector targets here)
; $2000          Mailbox base (mapped to bus MAILBOX_BASE)
; $2300          Ring 1 head pointer (6502 is ring index 1)
; $2301          Ring 1 tail pointer
; $2308+tail*32  Request entry descriptors (32 bytes each)
; $2508+tail*16  Response descriptors (16 bytes each)
; $FFFC-$FFFD    Reset vector (points to $0000)
;
; === BUILD AND RUN ===
; make ie65asm SRC=sdk/examples/asm/coproc_service_65.asm
; (loaded by a caller binary via COPROC_CMD_START)
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

.include "ie65.inc"

.segment "CODE"
.org $0000

; ============================================================================
; CONSTANTS - Ring Buffer Addresses
; ============================================================================
;
; The ring buffer for 6502 is at mailbox offset 1*$300 = $0300 from the
; mailbox base at $2000. Each ring has a head pointer (written by the
; caller), a tail pointer (written by us, the service), and two arrays:
; 16 entry descriptors (32 bytes each) and 16 response descriptors
; (16 bytes each).

RING_HEAD   = $2300
RING_TAIL   = $2301
ENTRIES_LO  = $08          ; low byte of entries offset from ring base ($2308)
ENTRIES_HI  = $23          ; high byte
RESP_LO     = $08          ; low byte of responses offset ($2508)
RESP_HI     = $25          ; high byte

; ============================================================================
; ZERO PAGE SCRATCH - Pointer Storage
; ============================================================================
;
; WHY: The 6502 indirect addressing mode (LDA (zp),Y) requires the base
; address to live in zero page. We compute multi-byte addresses at runtime
; and store them here so we can use indirect-indexed addressing to reach
; ring buffer entries, request data, and response data.

ZP_TAIL     = $00
ZP_ENTL     = $02          ; entry addr low
ZP_ENTH     = $03          ; entry addr high
ZP_RESPL    = $04          ; response addr low
ZP_RESPH    = $05          ; response addr high
ZP_REQPL    = $06          ; reqPtr low
ZP_REQPH    = $07          ; reqPtr high
ZP_REQPL2   = $08          ; reqPtr+1
ZP_REQPH2   = $09
ZP_RSPL     = $0A          ; respPtr low
ZP_RSPH     = $0B          ; respPtr high
ZP_RSPL2    = $0C          ; respPtr+1
ZP_RSPH2    = $0D
ZP_TICKL    = $0E          ; ticket low bytes
ZP_TICKH    = $0F

; ============================================================================
; MAIN POLL LOOP - Wait for Requests
; ============================================================================
;
; WHY: The service spins on head != tail. When the caller enqueues a
; request, it advances head. We detect this by comparing our cached tail
; against the current head value. This is a classic producer-consumer
; pattern where head is written by the producer (caller) and tail is
; written by the consumer (us).

main_loop:
    ; Read tail
    LDA RING_TAIL
    STA ZP_TAIL

    ; Read head
    LDA RING_HEAD

    ; Compare: if tail == head, ring empty
    CMP ZP_TAIL
    BEQ main_loop           ; empty, poll again

; ============================================================================
; ENTRY ADDRESS COMPUTATION - Locate the Request Descriptor
; ============================================================================
;
; WHY: Each entry is 32 bytes, so entry address = ENTRIES_BASE + tail * 32.
; The 6502 has no multiply instruction, so we shift left 5 times (ASL A)
; to compute tail * 32. The result is a 16-bit address built from the
; shifted value plus the entries base offset.

    ; Compute entry address: $2308 + tail*32
    ; tail*32 = tail << 5
    LDA ZP_TAIL
    ASL A                   ; *2
    ASL A                   ; *4
    ASL A                   ; *8
    ASL A                   ; *16
    ASL A                   ; *32
    CLC
    ADC #ENTRIES_LO         ; + $08
    STA ZP_ENTL
    LDA #ENTRIES_HI         ; $23
    ADC #$00                ; carry from low add
    STA ZP_ENTH

; ============================================================================
; OPCODE DISPATCH - Check Which Operation Was Requested
; ============================================================================
;
; WHY: The op field at entry+8 tells us what computation to perform.
; Currently only op=1 (add two uint32 values) is supported. Any other
; opcode triggers the error response path.

    ; Read op at entry+8
    LDY #8
    LDA (ZP_ENTL),Y         ; op low byte
    CMP #1                  ; op == 1?
    BEQ op_ok
    JMP error_resp
op_ok:

; ============================================================================
; EXTRACT REQUEST FIELDS - Read Pointers from the Entry Descriptor
; ============================================================================
;
; WHY: The entry descriptor contains pointers to the actual data buffers.
; reqPtr (entry+16) points to the input operands. respPtr (entry+24)
; points to where we write the result. ticket (entry+0) is an opaque
; value we must echo back in the response so the caller can match
; responses to requests.
;
; Register usage after this section:
;   ZP_REQPL/H  = request data pointer (where operands live)
;   ZP_RSPL/H   = response data pointer (where result goes)
;   ZP_TICKL/H  = ticket value to echo back

    ; Read reqPtr at entry+16 (little-endian uint32, we use low 16 bits)
    LDY #16
    LDA (ZP_ENTL),Y
    STA ZP_REQPL
    INY
    LDA (ZP_ENTL),Y
    STA ZP_REQPH

    ; Read respPtr at entry+24
    LDY #24
    LDA (ZP_ENTL),Y
    STA ZP_RSPL
    INY
    LDA (ZP_ENTL),Y
    STA ZP_RSPH

    ; Read ticket at entry+0 (low 2 bytes for response)
    LDY #0
    LDA (ZP_ENTL),Y
    STA ZP_TICKL
    INY
    LDA (ZP_ENTL),Y
    STA ZP_TICKH

; ============================================================================
; OP=1: 32-BIT ADDITION - Carry-Chained Byte-by-Byte Add
; ============================================================================
;
; WHY: The 6502 is an 8-bit CPU with no 32-bit arithmetic. To add two
; uint32 values, we must add four pairs of bytes with carry propagation.
; CLC clears carry before byte 0, then ADC (add with carry) automatically
; chains the carry through bytes 1-3. This is the fundamental pattern for
; multi-precision arithmetic on 8-bit processors.
;
; Memory layout at reqPtr:
;   reqPtr+0..3  = val1 (little-endian uint32)
;   reqPtr+4..7  = val2 (little-endian uint32)
; Result written to respPtr+0..3

    ; Op=1: add two uint32 from reqPtr
    ; For simplicity, do 32-bit add (4 bytes each)
    LDY #0
    CLC
    LDA (ZP_REQPL),Y        ; val1 byte 0
    LDY #4
    ADC (ZP_REQPL),Y        ; + val2 byte 0
    LDY #0
    STA (ZP_RSPL),Y         ; result byte 0

    LDY #1
    LDA (ZP_REQPL),Y        ; val1 byte 1
    LDY #5
    ADC (ZP_REQPL),Y        ; + val2 byte 1
    LDY #1
    STA (ZP_RSPL),Y         ; result byte 1

    LDY #2
    LDA (ZP_REQPL),Y        ; val1 byte 2
    LDY #6
    ADC (ZP_REQPL),Y        ; + val2 byte 2
    LDY #2
    STA (ZP_RSPL),Y         ; result byte 2

    LDY #3
    LDA (ZP_REQPL),Y        ; val1 byte 3
    LDY #7
    ADC (ZP_REQPL),Y        ; + val2 byte 3
    LDY #3
    STA (ZP_RSPL),Y         ; result byte 3

; ============================================================================
; WRITE RESPONSE DESCRIPTOR - Signal Completion to the Caller
; ============================================================================
;
; WHY: The response descriptor tells the caller what happened. It lives
; in a separate array from the request entries (RESP_BASE + tail*16).
; We fill in four fields, each a little-endian uint32:
;   +0  ticket    -- echoed from the request so the caller can correlate
;   +4  status    -- 2 = success, 3 = error
;   +8  resultCode -- 0 for success
;   +12 respLen   -- number of valid bytes in the response data buffer

    ; Write response descriptor at $2508 + tail*16
    LDA ZP_TAIL
    ASL A                   ; *2
    ASL A                   ; *4
    ASL A                   ; *8
    ASL A                   ; *16
    CLC
    ADC #RESP_LO            ; + $08
    STA ZP_RESPL
    LDA #RESP_HI            ; $25
    ADC #$00
    STA ZP_RESPH

    ; response.ticket (4 bytes, low 2 from saved)
    LDY #0
    LDA ZP_TICKL
    STA (ZP_RESPL),Y
    INY
    LDA ZP_TICKH
    STA (ZP_RESPL),Y
    INY
    LDA #0
    STA (ZP_RESPL),Y
    INY
    STA (ZP_RESPL),Y

    ; response.status = 2 (ok)
    LDY #4
    LDA #2
    STA (ZP_RESPL),Y
    INY
    LDA #0
    STA (ZP_RESPL),Y
    INY
    STA (ZP_RESPL),Y
    INY
    STA (ZP_RESPL),Y

    ; response.resultCode = 0
    LDY #8
    LDA #0
    STA (ZP_RESPL),Y
    INY
    STA (ZP_RESPL),Y
    INY
    STA (ZP_RESPL),Y
    INY
    STA (ZP_RESPL),Y

    ; response.respLen = 4
    LDY #12
    LDA #4
    STA (ZP_RESPL),Y
    INY
    LDA #0
    STA (ZP_RESPL),Y
    INY
    STA (ZP_RESPL),Y
    INY
    STA (ZP_RESPL),Y

; ============================================================================
; ADVANCE TAIL - Mark This Slot as Consumed
; ============================================================================
;
; WHY: The tail pointer is our read cursor into the ring. By advancing
; it modulo 16 (AND #$0F), we tell the caller that this slot is now free
; for reuse. The ring has 16 entries, so the mask wraps naturally.

    ; Advance tail: (tail + 1) & 15
    LDA ZP_TAIL
    CLC
    ADC #1
    AND #$0F
    STA RING_TAIL

    JMP main_loop

; ============================================================================
; ERROR RESPONSE - Unknown Opcode Handler
; ============================================================================
;
; WHY: If the caller sends an opcode we do not recognise, we must still
; advance the tail (otherwise the ring would stall forever). We write
; status=3 (error) so the caller knows the request was rejected.

error_resp:
    ; Write error response (status=3)
    LDA ZP_TAIL
    ASL A
    ASL A
    ASL A
    ASL A
    CLC
    ADC #RESP_LO
    STA ZP_RESPL
    LDA #RESP_HI
    ADC #$00
    STA ZP_RESPH

    ; ticket
    LDY #0
    LDA (ZP_ENTL),Y
    STA (ZP_RESPL),Y
    INY
    LDA (ZP_ENTL),Y
    STA (ZP_RESPL),Y
    LDY #2
    LDA #0
    STA (ZP_RESPL),Y
    INY
    STA (ZP_RESPL),Y

    ; status = 3 (error)
    LDY #4
    LDA #3
    STA (ZP_RESPL),Y

    ; Advance tail
    LDA ZP_TAIL
    CLC
    ADC #1
    AND #$0F
    STA RING_TAIL

    JMP main_loop

; ============================================================================
; RESET AND INTERRUPT VECTORS
; ============================================================================
;
; WHY: The 6502 reads the reset vector at $FFFC-$FFFD on power-up.
; We point it to $0000 where our code begins. The IRQ/BRK vector is
; also set to $0000 since this service does not use interrupts.

.org $FFFC
.word $0000                 ; RESET -> $0000
.word $0000                 ; IRQ/BRK -> $0000
