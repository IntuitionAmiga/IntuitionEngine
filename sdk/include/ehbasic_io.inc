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
;                 Returns length in R8. Clobbers R1-R7, R10-R13.
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
    ; Character available - read it
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
; Clobbers: R1-R7, R10-R13
;
; Behaviour:
;   - Reads characters until CR (0x0D) or LF (0x0A) is received
;   - Supports cursor movement via arrow keys (ESC[C / ESC[D)
;   - Supports Home (ESC[H / ESC[1~) and End (ESC[F / ESC[4~)
;   - Handles backspace (0x08): deletes char before cursor
;   - Handles delete (ESC[3~): deletes char at cursor
;   - Mid-line insert/delete redraws the tail of the line
;   - Ignores characters when buffer is full (sends BEL)
;   - Discards unrecognised escape sequences
;   - Returns count of characters (not including CR/LF or null)
;
; Registers:
;   R3  = buffer base pointer
;   R4  = max length
;   R5  = current length (chars in buffer)
;   R6  = cursor position (0..R5)
;   R7  = escape state (0=normal, 1=ESC, 2=ESC[, 3=ESC[+digit, 4=consume)
;   R9  = echo flag (1=on, 0=off)
;   R12 = escape param digit
;   R26 = TERM_OUT (cached)
;   R27 = TERM_STATUS (cached)

read_line:
    la      r1, TERM_CTRL
    move.l  r2, #1
    store.l r2, (r1)               ; enter line-input mode

    move.q  r3, r8                 ; r3 = buffer base pointer
    move.q  r4, r9                 ; r4 = max length
    move.q  r5, r0                 ; r5 = length (0)
    move.q  r6, r0                 ; r6 = cursor position (0)
    move.q  r7, r0                 ; r7 = escape state (0)

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

    ; --- Escape sequence state machine ---
    move.q  r1, #1
    beq     r7, r1, .rl_esc1      ; state 1: saw ESC
    move.q  r1, #2
    beq     r7, r1, .rl_esc2      ; state 2: saw ESC[
    move.q  r1, #3
    beq     r7, r1, .rl_esc3      ; state 3: saw ESC[ + digit
    move.q  r1, #4
    beq     r7, r1, .rl_esc4      ; state 4: consuming sequence

    ; --- State 0: normal input ---

    ; Check for ESC
    move.q  r1, #0x1B
    beq     r2, r1, .rl_got_esc

    ; Check for CR (0x0D) or LF (0x0A) - end of line
    move.q  r1, #0x0D
    beq     r2, r1, .rl_done
    move.q  r1, #0x0A
    beq     r2, r1, .rl_done

    ; Check for backspace (0x08)
    move.q  r1, #0x08
    beq     r2, r1, .rl_backspace

    ; Regular character - insert at cursor position
    bge     r5, r4, .rl_full       ; buffer full?

    ; Shift chars from end down to cursor, right by 1
    move.q  r10, r5                ; i = length
.rl_ins_shift:
    bge     r6, r10, .rl_ins_store ; if cursor >= i, done shifting
    sub.q   r11, r10, #1           ; i-1
    add.q   r11, r3, r11           ; &buffer[i-1]
    load.b  r1, (r11)
    add.q   r11, r3, r10           ; &buffer[i]
    store.b r1, (r11)
    sub.q   r10, r10, #1
    bra     .rl_ins_shift

.rl_ins_store:
    add.q   r1, r3, r6             ; &buffer[cursor]
    store.b r2, (r1)
    add.q   r5, r5, #1             ; length++
    add.q   r6, r6, #1             ; cursor++

    ; Echo: redraw from insert point to end, cursor back
    beqz    r9, .rl_poll
    sub.q   r10, r6, #1            ; start from inserted char
    move.q  r13, r0                ; shrink=0 (buffer grew)
    bra     .rl_redraw

.rl_got_esc:
    move.q  r7, #1                 ; state = 1 (saw ESC)
    bra     .rl_poll

; --- Escape state 1: saw ESC ---
.rl_esc1:
    move.q  r7, r0                 ; reset state
    move.q  r1, #0x5B             ; '['
    beq     r2, r1, .rl_esc1_csi
    bra     .rl_poll               ; discard ESC + non-[
.rl_esc1_csi:
    move.q  r7, #2                 ; state = 2 (CSI)
    bra     .rl_poll

; --- Escape state 2: saw ESC[ ---
.rl_esc2:
    move.q  r7, r0                 ; reset state (default)

    ; Check for cursor movement letters
    move.q  r1, #0x43             ; 'C'
    beq     r2, r1, .rl_cursor_right
    move.q  r1, #0x44             ; 'D'
    beq     r2, r1, .rl_cursor_left
    move.q  r1, #0x48             ; 'H'
    beq     r2, r1, .rl_home
    move.q  r1, #0x46             ; 'F'
    beq     r2, r1, .rl_end

    ; Check for digit (parameter byte): '0'-'9'
    move.q  r1, #0x30             ; '0'
    blt     r2, r1, .rl_esc_consume ; < '0' → consume
    move.q  r1, #0x3A             ; '9' + 1
    blt     r2, r1, .rl_esc2_digit  ; '0'-'9' → digit

    ; Terminal byte (0x40-0x7E) ends unknown sequence
    move.q  r1, #0x40
    bge     r2, r1, .rl_poll       ; done, discard

    ; Otherwise consume until terminal byte
    bra     .rl_esc_consume

.rl_esc2_digit:
    move.q  r12, r2                ; save param digit
    move.q  r7, #3                 ; state = 3
    bra     .rl_poll

; --- Escape state 3: saw ESC[ + digit(s) ---
.rl_esc3:
    move.q  r7, r0                 ; reset state (default)

    ; Check for '~' (function key terminator)
    move.q  r1, #0x7E             ; '~'
    beq     r2, r1, .rl_esc3_tilde

    ; Check for another digit - multi-digit param
    move.q  r1, #0x30
    blt     r2, r1, .rl_esc3_other
    move.q  r1, #0x3A
    blt     r2, r1, .rl_esc3_more

.rl_esc3_other:
    ; Terminal byte ends sequence
    move.q  r1, #0x40
    bge     r2, r1, .rl_poll       ; done, discard
    bra     .rl_esc_consume        ; consume remaining

.rl_esc3_more:
    move.q  r12, r0                ; multi-digit: clear param
    move.q  r7, #3                 ; stay in state 3
    bra     .rl_poll

.rl_esc3_tilde:
    ; Delete key: ESC[3~
    move.q  r1, #0x33             ; '3'
    beq     r12, r1, .rl_delete
    ; Home: ESC[1~
    move.q  r1, #0x31             ; '1'
    beq     r12, r1, .rl_home
    ; End: ESC[4~
    move.q  r1, #0x34             ; '4'
    beq     r12, r1, .rl_end
    bra     .rl_poll               ; discard other function keys

; --- Escape state 4: consuming remaining CSI bytes ---
.rl_esc4:
    ; Wait for terminal byte (0x40-0x7E) to end sequence
    move.q  r1, #0x40
    blt     r2, r1, .rl_poll       ; keep consuming
    move.q  r7, r0                 ; done, reset state
    bra     .rl_poll

.rl_esc_consume:
    move.q  r7, #4                 ; enter consume state
    bra     .rl_poll

; --- Cursor right ---
.rl_cursor_right:
    bge     r6, r5, .rl_poll       ; already at end
    add.q   r6, r6, #1
    beqz    r9, .rl_poll
    move.q  r1, #0x1B
    store.b r1, (r26)
    move.q  r1, #0x5B
    store.b r1, (r26)
    move.q  r1, #0x43
    store.b r1, (r26)
    bra     .rl_poll

; --- Cursor left ---
.rl_cursor_left:
    beqz    r6, .rl_poll           ; already at start
    sub.q   r6, r6, #1
    beqz    r9, .rl_poll
    move.q  r1, #0x08
    store.b r1, (r26)              ; BS moves cursor left
    bra     .rl_poll

; --- Home (cursor to start) ---
.rl_home:
    beqz    r6, .rl_poll           ; already at start
    beqz    r9, .rl_home_set
    move.q  r10, r6
.rl_home_bs:
    beqz    r10, .rl_home_set
    move.q  r1, #0x08
    store.b r1, (r26)
    sub.q   r10, r10, #1
    bra     .rl_home_bs
.rl_home_set:
    move.q  r6, r0
    bra     .rl_poll

; --- End (cursor to end) ---
.rl_end:
    bge     r6, r5, .rl_poll       ; already at end
    beqz    r9, .rl_end_set
    sub.q   r10, r5, r6
.rl_end_fwd:
    beqz    r10, .rl_end_set
    move.q  r1, #0x1B
    store.b r1, (r26)
    move.q  r1, #0x5B
    store.b r1, (r26)
    move.q  r1, #0x43
    store.b r1, (r26)
    sub.q   r10, r10, #1
    bra     .rl_end_fwd
.rl_end_set:
    move.q  r6, r5
    bra     .rl_poll

; --- Delete (forward delete at cursor) ---
.rl_delete:
    bge     r6, r5, .rl_poll       ; at end, nothing to delete
    ; Shift chars left from cursor+1 to end
    move.q  r10, r6
.rl_del_shift:
    add.q   r11, r10, #1
    bge     r11, r5, .rl_del_done
    add.q   r11, r3, r11           ; &buffer[i+1]
    load.b  r1, (r11)
    add.q   r11, r3, r10           ; &buffer[i]
    store.b r1, (r11)
    add.q   r10, r10, #1
    bra     .rl_del_shift
.rl_del_done:
    sub.q   r5, r5, #1             ; length--
    beqz    r9, .rl_poll
    move.q  r10, r6                ; redraw from cursor
    move.q  r13, #1                ; shrink=1 (need trailing space)
    bra     .rl_redraw

; --- Backspace (delete before cursor) ---
.rl_backspace:
    beqz    r6, .rl_poll           ; cursor at start, nothing to delete
    sub.q   r6, r6, #1             ; cursor--
    ; Shift chars left from cursor+1 to end
    move.q  r10, r6
.rl_bs_shift:
    add.q   r11, r10, #1
    bge     r11, r5, .rl_bs_done
    add.q   r11, r3, r11           ; &buffer[i+1]
    load.b  r1, (r11)
    add.q   r11, r3, r10           ; &buffer[i]
    store.b r1, (r11)
    add.q   r10, r10, #1
    bra     .rl_bs_shift
.rl_bs_done:
    sub.q   r5, r5, #1             ; length--
    beqz    r9, .rl_poll
    ; Move terminal cursor left first
    move.q  r1, #0x08
    store.b r1, (r26)              ; BS
    move.q  r10, r6                ; redraw from new cursor position
    move.q  r13, #1                ; shrink=1 (need trailing space)
    bra     .rl_redraw

; --- Redraw tail (common echo for insert/delete/backspace) ---
; R10 = start position for redraw
; R13 = 1 if buffer shrank (need trailing space to erase old last char)
; Redraws chars from R10 to end, optionally adds space, moves cursor back to R6
.rl_redraw:
    move.q  r11, r10               ; save redraw start for BS count
.rl_redraw_loop:
    bge     r10, r5, .rl_redraw_pad
    add.q   r1, r3, r10            ; &buffer[r10]
    load.b  r1, (r1)
    store.b r1, (r26)
    add.q   r10, r10, #1
    bra     .rl_redraw_loop
.rl_redraw_pad:
    beqz    r13, .rl_redraw_back
    move.q  r1, #0x20
    store.b r1, (r26)              ; space to erase old last char
    add.q   r10, r10, #1           ; account for space in cursor position
.rl_redraw_back:
    ; Move cursor back from r10 to r6
    sub.q   r10, r10, r6
    beqz    r10, .rl_poll
.rl_redraw_bs:
    move.q  r1, #0x08
    store.b r1, (r26)
    sub.q   r10, r10, #1
    beqz    r10, .rl_poll
    bra     .rl_redraw_bs

.rl_full:
    ; Buffer full - send BEL (0x07)
    beqz    r9, .rl_poll
    move.q  r1, #0x07
    store.b r1, (r26)
    bra     .rl_poll

.rl_done:
    ; Exit line-input mode
    la      r1, TERM_CTRL
    store.l r0, (r1)

    ; Null-terminate the buffer
    add.q   r1, r3, r5             ; buffer[length]
    store.b r0, (r1)               ; write null terminator
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
