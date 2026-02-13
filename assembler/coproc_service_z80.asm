; coproc_service_z80.asm - Z80 Coprocessor Service Template
;
; Service binary contract:
;   1. Poll ring buffer for requests
;   2. Dispatch on op field
;   3. Op=1 (add): read two uint32 from reqPtr, add, write to respPtr
;   4. Write response descriptor (status=2/ok, respLen=4)
;   5. Advance tail, loop
;
; Memory map (Z80 address space via CoprocZ80Bus):
;   Code loaded at CPU addr 0x0000
;   Mailbox at CPU addr 0x2000 (mapped to bus MAILBOX_BASE)
;   Ring 3 (Z80): mailbox offset = 3*0x300 = 0x0900
;     ring base:  0x2900
;     head:       0x2900
;     tail:       0x2901
;     entries:    0x2908 + tail*32
;     responses:  0x2B08 + tail*16
;
; Assemble: vasmz80_std -Fbin -o coproc_service_z80.ie80 coproc_service_z80.asm
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

    .org 0x0000

.set RING_HEAD,    0x2900
.set RING_TAIL,    0x2901
.set ENTRIES_BASE, 0x2908
.set RESP_BASE,    0x2B08

main_loop:
    ; Read tail
    LD A, (RING_TAIL)
    LD B, A                 ; B = tail

    ; Read head
    LD A, (RING_HEAD)

    ; Compare: if tail == head, ring empty
    CP B
    JR Z, main_loop         ; empty, poll

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

    ; Read op at entry+8 (low byte only)
    PUSH HL
    LD DE, 8
    ADD HL, DE              ; HL = entry+8
    LD A, (HL)              ; A = op low byte
    POP HL
    CP 1
    JP NZ, error_resp

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

    ; Advance tail
    LD A, (RING_TAIL)
    INC A
    AND 0x0F
    LD (RING_TAIL), A

    JP main_loop

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
