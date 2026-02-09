; ehbasic_io.inc - EhBASIC IE64 I/O Layer
;
; Low-level terminal I/O routines for the EhBASIC IE64 port.
; These replace the M68K TRAP #15 I/O vectors from ehbasic68k.asm.
;
; Calling conventions:
;   putchar:      R8.b = character to output. Clobbers R1.
;   getchar:      Returns byte in R8.b, R9 = 1 if char available, 0 if not.
;                 Clobbers R1, R2.
;   getchar_wait: Blocks until a character is available. Returns in R8.b.
;                 Clobbers R1, R2.
;   print_string: R8 = pointer to null-terminated string. Clobbers R1, R2, R3.
;   print_crlf:   Outputs CR+LF. Clobbers R1, R8.
;   read_line:    Reads a line into buffer at R8, max length in R9.
;                 Returns length in R8. Clobbers R1-R5.
;
; Register conventions (preserved across calls):
;   R26 = cached TERM_OUT address (set up by io_init)
;   R27 = cached TERM_STATUS address (set up by io_init)
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; I/O Initialisation
; ============================================================================

io_init:
    ; Cache terminal register addresses in R26/R27 for fast access
    la      r26, TERM_OUT
    la      r27, TERM_STATUS
    rts

; ============================================================================
; putchar - Output a single character
; ============================================================================
; Input:  R8.b = character
; Output: none
; Clobbers: R1

putchar:
    store.b r8, (r26)               ; write byte to TERM_OUT
    rts

; ============================================================================
; getchar - Non-blocking character input
; ============================================================================
; Output: R8.b = character (if available)
;         R9   = 1 if character available, 0 if not
; Clobbers: R1, R2

getchar:
    load.l  r1, (r27)              ; read TERM_STATUS
    and.l   r2, r1, #1             ; bit 0 = input available
    beqz    r2, .gc_none
    ; Character available — read it
    la      r1, TERM_IN
    load.l  r8, (r1)               ; read and dequeue character
    and.l   r8, r8, #0xFF          ; mask to byte
    move.q  r9, #1                 ; flag: available
    rts
.gc_none:
    move.q  r8, r0                 ; no char
    move.q  r9, r0                 ; flag: not available
    rts

; ============================================================================
; getchar_wait - Blocking character input
; ============================================================================
; Output: R8.b = character
; Clobbers: R1, R2

getchar_wait:
.gcw_poll:
    load.l  r1, (r27)              ; read TERM_STATUS
    and.l   r2, r1, #1             ; bit 0 = input available
    beqz    r2, .gcw_poll          ; spin until available
    la      r1, TERM_IN
    load.l  r8, (r1)               ; read and dequeue
    and.l   r8, r8, #0xFF          ; mask to byte
    rts

; ============================================================================
; print_string - Print null-terminated string
; ============================================================================
; Input:  R8 = pointer to null-terminated string
; Output: none
; Clobbers: R1, R2, R3

print_string:
    move.q  r3, r8                 ; r3 = string pointer
.ps_loop:
    load.b  r2, (r3)              ; load byte from string
    beqz    r2, .ps_done           ; null terminator? done
    store.b r2, (r26)             ; write to TERM_OUT
    add.q   r3, r3, #1            ; advance pointer
    bra     .ps_loop
.ps_done:
    rts

; ============================================================================
; print_crlf - Output CR + LF
; ============================================================================
; Clobbers: R1, R8

print_crlf:
    move.q  r8, #0x0D              ; CR
    store.b r8, (r26)
    move.q  r8, #0x0A              ; LF
    store.b r8, (r26)
    rts

; ============================================================================
; read_line - Read a line of input into buffer
; ============================================================================
; Input:  R8 = pointer to buffer
;         R9 = max buffer length (excluding null terminator)
; Output: R8 = number of characters read (not including null terminator)
;         The buffer is null-terminated.
; Clobbers: R1-R5
;
; Behaviour:
;   - Reads characters until CR (0x0D) or LF (0x0A) is received
;   - Echoes each character back to terminal
;   - Handles backspace (0x08): deletes last char, echoes BS+space+BS
;   - Ignores characters when buffer is full (sends BEL)
;   - Returns count of characters (not including CR/LF or null)

read_line:
    move.q  r3, r8                 ; r3 = buffer base pointer
    move.q  r4, r9                 ; r4 = max length
    move.q  r5, r0                 ; r5 = current index (0)

    ; Cache echo flag: R9 = echo enabled (1) or disabled (0)
    la      r1, TERM_ECHO
    load.l  r9, (r1)
    and.l   r9, r9, #1

.rl_poll:
    ; Poll for input
    load.l  r1, (r27)             ; read TERM_STATUS
    and.l   r2, r1, #1            ; bit 0 = input available
    beqz    r2, .rl_poll           ; spin until available

    ; Read the character
    la      r1, TERM_IN
    load.l  r2, (r1)              ; read and dequeue
    and.l   r2, r2, #0xFF         ; mask to byte

    ; Check for CR (0x0D) or LF (0x0A) — end of line
    move.q  r1, #0x0D
    beq     r2, r1, .rl_done
    move.q  r1, #0x0A
    beq     r2, r1, .rl_done

    ; Check for backspace (0x08)
    move.q  r1, #0x08
    beq     r2, r1, .rl_backspace

    ; Regular character — check if buffer has space
    bge     r5, r4, .rl_full

    ; Store character in buffer
    add.q   r1, r3, r5            ; buffer[index]
    store.b r2, (r1)              ; store char
    add.q   r5, r5, #1            ; index++

    ; Echo character (if echo enabled)
    beqz    r9, .rl_poll
    store.b r2, (r26)             ; echo to TERM_OUT
    bra     .rl_poll

.rl_backspace:
    ; If buffer is empty, ignore
    beqz    r5, .rl_poll
    sub.q   r5, r5, #1            ; index--
    ; Echo: BS + space + BS to erase on terminal (if echo enabled)
    beqz    r9, .rl_poll
    move.q  r1, #0x08
    store.b r1, (r26)             ; BS
    move.q  r1, #0x20
    store.b r1, (r26)             ; space
    move.q  r1, #0x08
    store.b r1, (r26)             ; BS
    bra     .rl_poll

.rl_full:
    ; Buffer full — send BEL (0x07)
    beqz    r9, .rl_poll
    move.q  r1, #0x07
    store.b r1, (r26)
    bra     .rl_poll

.rl_done:
    ; Null-terminate the buffer
    add.q   r1, r3, r5            ; buffer[index]
    store.b r0, (r1)              ; write null terminator
    ; Echo CR+LF (if echo enabled)
    beqz    r9, .rl_no_echo_crlf
    move.q  r1, #0x0D
    store.b r1, (r26)
    move.q  r1, #0x0A
    store.b r1, (r26)
.rl_no_echo_crlf:
    ; Return count in R8
    move.q  r8, r5
    rts

; ============================================================================
; EOF
; ============================================================================
