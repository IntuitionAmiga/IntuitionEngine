; ehbasic_ie64.asm - EhBASIC IE64 Main Entry Point and REPL
;
; This is the main file for the EhBASIC IE64 interpreter. It provides:
;   - Cold start: memory initialisation, boot banner
;   - Warm start: reset state, show "Ready" prompt
;   - REPL loop: read line, detect line number vs immediate, dispatch
;
; Usage:
;   bin/ie64asm assembler/ehbasic_ie64.asm
;   bin/IntuitionEngine -ie64 assembler/ehbasic_ie64.iex
;
; Register conventions (global, preserved across calls):
;   R16 = interpreter state base (BASIC_STATE)
;   R17 = text pointer (current position in tokenized content)
;   R26 = cached TERM_OUT address
;   R27 = cached TERM_STATUS address
;   R31 = hardware stack pointer
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

include "ie64.inc"
include "ehbasic_tokens.inc"

    org 0x1000

; ============================================================================
; Cold Start — one-time initialisation
; ============================================================================

cold_start:
    ; Set up hardware stack
    la      r31, STACK_TOP

    ; Initialise terminal I/O (caches R26, R27)
    jsr     io_init

    ; Initialise interpreter state base
    la      r16, BASIC_STATE

    ; Clear state block (256 bytes)
    move.q  r1, r16
    move.q  r2, #256
.clear_state:
    store.b r0, (r1)
    add.q   r1, r1, #1
    sub.q   r2, r2, #1
    bnez    r2, .clear_state

    ; Initialise line storage (empty program)
    jsr     line_init

    ; Initialise variable storage
    jsr     var_init

    ; Seed RNG
    move.q  r1, #12345
    add.q   r2, r16, #ST_RANDOM_SEED
    store.l r1, (r2)

    ; Disable MMIO echo — EhBASIC's read_line handles echo itself
    la      r1, TERM_ECHO
    move.q  r2, #0
    store.l r2, (r1)

    ; Print boot banner
    la      r8, repl_str_banner
    jsr     print_string
    jsr     print_crlf

; ============================================================================
; Warm Start — reset execution state, enter REPL
; ============================================================================

warm_start:
    ; Reset stack pointer (in case of error recovery)
    la      r31, STACK_TOP

    ; Clear direct mode
    add.q   r1, r16, #ST_DIRECT_MODE
    store.l r0, (r1)

    ; Clear error flag
    add.q   r1, r16, #ST_ERROR_FLAG
    store.l r0, (r1)

; ============================================================================
; REPL — Read-Eval-Print Loop
; ============================================================================

repl_loop:
    ; Print "Ready" prompt
    la      r8, repl_str_ready
    jsr     print_string
    jsr     print_crlf

repl_read:
    ; Read a line of input
    la      r8, BASIC_LINE_BUF
    move.q  r9, #BASIC_LINE_BUFLEN
    jsr     read_line
    ; R8 = number of characters read
    beqz    r8, repl_read           ; empty line — try again

    ; --- Check if line starts with a digit (line number) ---
    la      r1, BASIC_LINE_BUF
    load.b  r2, (r1)

    ; Is first char a digit (0x30..0x39)?
    move.q  r3, #0x30
    blt     r2, r3, repl_immediate
    move.q  r3, #0x3A
    bge     r2, r3, repl_immediate

    ; --- Line number present: parse it, tokenize, and store ---
    jsr     repl_parse_linenum      ; R8 = line number, R9 = pointer past digits

    ; Save line number
    move.q  r22, r8

    ; Check if rest of line is empty (delete line)
    ; Skip spaces after line number
    move.q  r1, r9
.skip_sp:
    load.b  r2, (r1)
    move.q  r3, #0x20
    bne     r2, r3, .after_sp
    add.q   r1, r1, #1
    bra     .skip_sp
.after_sp:
    beqz    r2, repl_delete_line

    ; Tokenize the line content (after the line number)
    move.q  r8, r9                  ; R8 = pointer to content after line number
    la      r9, 0x021100            ; R9 = tokenize output buffer
    jsr     tokenize
    ; R8 = tokenized length

    ; Store the line
    move.q  r10, r8                 ; R10 = tokenized length
    move.q  r8, r22                 ; R8 = line number
    la      r9, 0x021100            ; R9 = tokenized content
    jsr     line_store

    bra     repl_read               ; back to input (no "Ready" prompt)

repl_delete_line:
    ; Delete line: line_store with length 0
    move.q  r8, r22                 ; R8 = line number
    move.q  r9, r0                  ; R9 = unused
    move.q  r10, r0                 ; R10 = 0 (delete)
    jsr     line_store
    bra     repl_read

repl_immediate:
    ; --- Immediate mode: tokenize and execute directly ---

    ; Check for RUN command (before tokenizing)
    la      r1, BASIC_LINE_BUF
    jsr     repl_check_run
    bnez    r8, repl_do_run

    ; Check for LIST command
    la      r1, BASIC_LINE_BUF
    jsr     repl_check_list
    bnez    r8, repl_do_list

    ; Check for NEW command
    la      r1, BASIC_LINE_BUF
    jsr     repl_check_new
    bnez    r8, repl_do_new

    ; Tokenize the input line
    la      r8, BASIC_LINE_BUF
    la      r9, 0x021100
    jsr     tokenize
    ; R8 = tokenized length
    beqz    r8, repl_read           ; empty after tokenize

    ; Set direct mode flag
    add.q   r1, r16, #ST_DIRECT_MODE
    move.q  r2, #1
    store.l r2, (r1)

    ; Set text pointer to tokenized content
    la      r17, 0x021100

    ; Execute the tokenized statement(s)
    ; In direct mode, R14 = 0 (no current line)
    move.q  r14, r0
    jsr     exec_line
    ; exec_line return code in R8 (0=normal, 1=goto, 2=end)

    ; Clear direct mode
    add.q   r1, r16, #ST_DIRECT_MODE
    store.l r0, (r1)

    bra     repl_loop

repl_do_run:
    ; Execute stored program
    jsr     exec_run
    bra     repl_loop

repl_do_list:
    ; List program
    move.q  r8, r0                  ; start from line 0
    move.q  r9, #0xFFFFFF           ; to end
    jsr     line_list
    bra     repl_loop

repl_do_new:
    ; Clear program and variables
    jsr     line_init
    jsr     var_init
    ; Reset DATA pointer
    add.q   r1, r16, #ST_DATA_PTR
    store.l r0, (r1)
    bra     repl_loop

; ============================================================================
; repl_parse_linenum — Extract line number from input buffer
; ============================================================================
; Input:  BASIC_LINE_BUF contains the line
; Output: R8 = line number (unsigned 32-bit)
;         R9 = pointer to first non-digit character after the number
; Clobbers: R1-R5

repl_parse_linenum:
    la      r1, BASIC_LINE_BUF
    move.q  r8, r0                  ; accumulator = 0
.loop:
    load.b  r2, (r1)
    ; Check digit range
    move.q  r3, #0x30
    blt     r2, r3, .done
    move.q  r3, #0x3A
    bge     r2, r3, .done
    ; digit: acc = acc * 10 + (char - '0')
    move.q  r4, #10
    mulu.l  r8, r8, r4
    sub.q   r2, r2, #0x30
    add.l   r8, r8, r2
    add.q   r1, r1, #1
    bra     .loop
.done:
    move.q  r9, r1                  ; R9 = pointer past digits
    rts

; ============================================================================
; repl_check_run — Check if input is "RUN" (case-insensitive)
; ============================================================================
; Input:  R1 = pointer to input buffer
; Output: R8 = 1 if RUN, 0 otherwise
; Clobbers: R2-R5

repl_check_run:
    jsr     repl_skip_spaces

    load.b  r2, (r1)
    or.l    r2, r2, #0x20           ; lowercase
    move.q  r3, #0x72               ; 'r'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x75               ; 'u'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x6E               ; 'n'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    beqz    r2, .yes
    move.q  r3, #0x20
    beq     r2, r3, .yes
    bra     .no

.yes:
    move.q  r8, #1
    rts
.no:
    move.q  r8, r0
    rts

; ============================================================================
; repl_check_list — Check if input is "LIST" (case-insensitive)
; ============================================================================
; Input:  R1 = pointer to input buffer
; Output: R8 = 1 if LIST, 0 otherwise
; Clobbers: R2-R5

repl_check_list:
    jsr     repl_skip_spaces

    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x6C               ; 'l'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x69               ; 'i'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x73               ; 's'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x74               ; 't'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    beqz    r2, .yes
    move.q  r3, #0x20
    beq     r2, r3, .yes
    bra     .no

.yes:
    move.q  r8, #1
    rts
.no:
    move.q  r8, r0
    rts

; ============================================================================
; repl_check_new — Check if input is "NEW" (case-insensitive)
; ============================================================================
; Input:  R1 = pointer to input buffer
; Output: R8 = 1 if NEW, 0 otherwise
; Clobbers: R2-R5

repl_check_new:
    jsr     repl_skip_spaces

    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x6E               ; 'n'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x65               ; 'e'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x77               ; 'w'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    beqz    r2, .yes
    move.q  r3, #0x20
    beq     r2, r3, .yes
    bra     .no

.yes:
    move.q  r8, #1
    rts
.no:
    move.q  r8, r0
    rts

; ============================================================================
; repl_skip_spaces — Skip spaces at R1
; ============================================================================
; Input/Output: R1 = pointer (advanced past spaces)
; Clobbers: R2, R3

repl_skip_spaces:
    load.b  r2, (r1)
    move.q  r3, #0x20
    bne     r2, r3, .done
    add.q   r1, r1, #1
    bra     repl_skip_spaces
.done:
    rts

; ============================================================================
; String Data
; ============================================================================

repl_str_banner:
    dc.b    "EhBASIC IE64 v1.0", 0x0D, 0x0A
    dc.b    "Zayn Otley, 2024-2026", 0x0D, 0x0A
    dc.b    "Based on EhBASIC by Lee Davison", 0
    align 4

repl_str_ready:
    dc.b    "Ready", 0
    align 4

; ============================================================================
; Include all interpreter modules
; ============================================================================

include "ehbasic_io.inc"
include "ehbasic_tokenizer.inc"
include "ehbasic_lineeditor.inc"
include "ehbasic_expr.inc"
include "ehbasic_vars.inc"
include "ehbasic_strings.inc"
include "ehbasic_exec.inc"
include "ehbasic_hw_video.inc"
include "ehbasic_hw_audio.inc"
include "ehbasic_hw_system.inc"
include "ehbasic_hw_voodoo.inc"
include "ie64_fp.inc"
