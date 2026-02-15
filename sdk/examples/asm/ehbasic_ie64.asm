; ============================================================================
; EHBASIC IE64 - BASIC INTERPRETER ENTRY POINT AND REPL
; IE64 Assembly for IntuitionEngine - Terminal I/O (serial console)
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    IE64 (custom 64-bit RISC)
; Video Chip:    None (terminal I/O via serial MMIO)
; Audio Engine:  All engines accessible via BASIC SOUND commands
; Assembler:     ie64asm (Intuition Engine IE64 assembler)
; Build:         sdk/bin/ie64asm -I sdk/include sdk/examples/asm/ehbasic_ie64.asm
; Run:           bin/IntuitionEngine -ie64 ehbasic_ie64.ie64
; Shortcut:      make basic (assembles, embeds, and builds with -basic flag)
; Porting:       EhBASIC is tightly coupled to IE64. The interpreter's
;                register conventions and MMIO addresses are IE64-specific.
;                Porting would require rewriting the entire codebase.
;
; === WHAT THIS DEMO DOES ===
; 1. Performs one-time cold start: initialises memory, variables, and the FPU
; 2. Prints the EhBASIC boot banner with version and copyright
; 3. Enters a Read-Eval-Print Loop (REPL) that accepts typed commands
; 4. Lines beginning with a digit are stored as numbered programme lines
; 5. Lines without a number are tokenised and executed immediately
; 6. Recognises RUN, LIST, and NEW as special immediate commands
; 7. Supports RUN "filename" to load and execute external BASIC programmes
;
; === WHY BASIC INTERPRETERS ===
;
; BASIC (Beginner's All-purpose Symbolic Instruction Code) was the lingua
; franca of home computing from 1975 to 1995. Nearly every 8-bit computer
; shipped with a BASIC interpreter in ROM:
;
;   - Commodore 64: Commodore BASIC 2.0 (by Microsoft)
;   - ZX Spectrum:  Sinclair BASIC
;   - Apple II:     Applesoft BASIC (by Microsoft)
;   - BBC Micro:    BBC BASIC (by Acorn)
;   - Atari 800:    Atari BASIC
;
; EhBASIC (Enhanced BASIC) was written by Lee Davison as a portable BASIC
; interpreter for the 6502 processor. This port adapts it to the IE64
; architecture, preserving the original language semantics while adding
; hardware extension commands for the Intuition Engine's video, audio,
; and blitter subsystems.
;
; === WHY TOKENISATION ===
;
; When the user types a line like PRINT "HELLO", the interpreter does not
; store it as raw ASCII text. Instead, the tokeniser converts keywords into
; single-byte tokens (e.g., PRINT becomes 0x99). This has two benefits:
;
;   1. Memory savings: a 5-letter keyword becomes 1 byte
;   2. Faster execution: the executor can dispatch on a single byte
;      via a jump table instead of string-comparing every keyword
;
; The REPL loop below orchestrates this: input -> tokenise -> store/execute.
;
; === WHY THE REPL LOOP ===
;
; The REPL pattern (Read-Eval-Print Loop) is the fundamental interaction
; model for interactive interpreters. Each iteration:
;
;   READ:  Accept a line of text from the terminal
;   EVAL:  Tokenise it, then either store it (if numbered) or execute it
;   PRINT: Display results or error messages
;   LOOP:  Show the "Ready" prompt and repeat
;
; This gives the user immediate feedback - type PRINT 2+2 and see 4 at
; once, without any compile-link-run cycle.
;
; === REGISTER CONVENTIONS (global, preserved across calls) ===
;   R16 = interpreter state base address (BASIC_STATE)
;   R17 = text pointer (current position in tokenised content)
;   R26 = cached TERM_OUT MMIO address (avoids repeated LA instructions)
;   R27 = cached TERM_STATUS MMIO address
;   R31 = hardware stack pointer
;
; === MEMORY MAP ===
;   0x001000          Programme code entry point (this file + includes)
;   0x021100          Tokeniser output buffer
;   0x050000          Simple variable storage
;   0x058000          Array variable storage
;   0x060000          String variable storage
;   0x070000          BASIC_STATE (interpreter state block, 256 bytes)
;   0x0FF000          STACK_TOP (hardware stack, grows downward)
;
; === BUILD AND RUN ===
;   sdk/bin/ie64asm -I sdk/include sdk/examples/asm/ehbasic_ie64.asm
;   bin/IntuitionEngine -ie64 ehbasic_ie64.ie64
;
; Or use the embedded build:
;   make basic
;   bin/IntuitionEngine -basic
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

include "ie64.inc"
include "ehbasic_tokens.inc"

    org 0x1000

; ============================================================================
; COLD START - One-time initialisation
; ============================================================================
; Runs once at power-on. Sets up the stack, terminal I/O, interpreter state,
; variable storage, FPU rounding mode, RNG seed, and terminal echo. After
; printing the boot banner, execution falls through to warm_start.

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

    ; Initialise line storage (empty programme)
    jsr     line_init

    ; Initialise variable storage
    jsr     var_init

    ; Initialise FPU rounding mode to truncate toward zero (fp_int compatibility)
    move.l  r1, #1                ; RND_ZERO = 1
    fmovcc  r1

    ; Seed RNG
    move.q  r1, #12345
    add.q   r2, r16, #ST_RANDOM_SEED
    store.l r1, (r2)

    ; Enable terminal echo (read_line checks this flag)
    la      r1, TERM_ECHO
    move.q  r2, #1
    store.l r2, (r1)

    ; Print boot banner
    la      r8, repl_str_banner
    jsr     print_string
    jsr     print_crlf

; ============================================================================
; WARM START - Reset execution state, enter REPL
; ============================================================================
; Called after errors or STOP to recover. Resets the stack pointer and
; clears runtime flags, then falls through to the REPL loop.

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
; REPL - Read-Eval-Print Loop
; ============================================================================
; The core interaction loop. Prints "Ready", reads a line, determines
; whether it is a numbered programme line or an immediate command, and
; dispatches accordingly.

repl_loop:
    ; Print "Ready" prompt
    la      r8, repl_str_ready
    jsr     print_string
    jsr     print_crlf

repl_read:
    ; Read a line of input from the terminal
    la      r8, BASIC_LINE_BUF
    move.q  r9, #BASIC_LINE_BUFLEN
    jsr     read_line
    ; R8 = number of characters read
    beqz    r8, repl_read           ; Empty line - try again

    ; --- Determine whether the line starts with a digit (line number) ---
    la      r1, BASIC_LINE_BUF
    load.b  r2, (r1)

    ; ASCII digits are 0x30..0x39
    move.q  r3, #0x30
    blt     r2, r3, repl_immediate
    move.q  r3, #0x3A
    bge     r2, r3, repl_immediate

    ; --- Numbered line: parse number, tokenise content, store ---
    jsr     repl_parse_linenum      ; R8 = line number, R9 = pointer past digits

    ; Save line number
    move.q  r22, r8

    ; Check if the rest of the line is empty (which means: delete this line)
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

    ; Tokenise the line content (after the line number)
    move.q  r8, r9                  ; R8 = pointer to content after line number
    la      r9, 0x021100            ; R9 = tokenise output buffer
    jsr     tokenize
    ; R8 = tokenised length

    ; Store the line in programme memory
    move.q  r10, r8                 ; R10 = tokenised length
    move.q  r8, r22                 ; R8 = line number
    la      r9, 0x021100            ; R9 = tokenised content
    jsr     line_store

    bra     repl_read               ; Back to input (no "Ready" prompt)

repl_delete_line:
    ; Delete line: call line_store with length 0
    move.q  r8, r22                 ; R8 = line number
    move.q  r9, r0                  ; R9 = unused
    move.q  r10, r0                 ; R10 = 0 (signals deletion)
    jsr     line_store
    bra     repl_read

repl_immediate:
    ; --- Immediate mode: check for special commands, then tokenise and execute ---

    ; Check for RUN command (before tokenising)
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

    ; Tokenise the input line
    la      r8, BASIC_LINE_BUF
    la      r9, 0x021100
    jsr     tokenize
    ; R8 = tokenised length
    beqz    r8, repl_read           ; Empty after tokenise

    ; Set direct mode flag
    add.q   r1, r16, #ST_DIRECT_MODE
    move.q  r2, #1
    store.l r2, (r1)

    ; Set text pointer to tokenised content
    la      r17, 0x021100

    ; Execute the tokenised statement(s)
    ; In direct mode, R14 = 0 (no current line)
    move.q  r14, r0
    jsr     exec_line

    ; Clear direct mode
    add.q   r1, r16, #ST_DIRECT_MODE
    store.l r0, (r1)

    bra     repl_loop

; ============================================================================
; RUN command handler
; ============================================================================
; Supports two forms:
;   RUN          - execute the stored programme from the first line
;   RUN "file"   - load and execute an external BASIC programme file

repl_do_run:
    la      r1, BASIC_LINE_BUF
    jsr     repl_parse_run_filename
    beqz    r8, .run_internal

    ; --- External file execution via EXEC handoff ---
    ; Read current session value before triggering executor
    la      r1, EXEC_SESSION
    load.l  r21, (r1)

    ; Point executor at the filename buffer
    la      r1, EXEC_NAME_PTR
    la      r2, FILE_NAME_BUF
    store.l r2, (r1)

    ; Trigger execution
    la      r1, EXEC_CTRL
    move.q  r2, #1
    store.l r2, (r1)

    ; Wait for EXEC_SESSION to advance (bounded poll)
    move.q  r25, #0x200000
.run_wait_session:
    la      r1, EXEC_SESSION
    load.l  r22, (r1)
    bne     r22, r21, .run_wait_status
    sub.q   r25, r25, #1
    bnez    r25, .run_wait_session
    bra     .run_file_error

    ; Poll status for the new session (bounded)
.run_wait_status:
    move.q  r25, #0x400000
.run_status_loop:
    la      r1, EXEC_STATUS
    load.l  r22, (r1)
    move.q  r23, #1
    beq     r22, r23, .run_status_waiting
    move.q  r23, #3
    beq     r22, r23, .run_file_error
    move.q  r23, #2
    beq     r22, r23, .run_external_ok
    bra     .run_file_error

.run_status_waiting:
    sub.q   r25, r25, #1
    bnez    r25, .run_status_loop
    bra     .run_file_error

.run_external_ok:
    bra     repl_loop

.run_internal:
    jsr     exec_run
    bra     repl_loop

.run_file_error:
    la      r8, repl_msg_file_error
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop

; ============================================================================
; LIST command handler - display stored programme
; ============================================================================

repl_do_list:
    move.q  r8, r0                  ; Start from line 0
    move.q  r9, #0xFFFFFF           ; To end
    jsr     line_list
    bra     repl_loop

; ============================================================================
; NEW command handler - clear programme and variables
; ============================================================================

repl_do_new:
    jsr     line_init
    jsr     var_init
    ; Reset DATA pointer
    add.q   r1, r16, #ST_DATA_PTR
    store.l r0, (r1)
    bra     repl_loop

; ============================================================================
; repl_parse_linenum - Extract line number from input buffer
; ============================================================================
; Parses a sequence of ASCII digits into a 32-bit unsigned integer.
;
; Input:  BASIC_LINE_BUF contains the line
; Output: R8 = line number, R9 = pointer to first non-digit character
; Clobbers: R1-R5

repl_parse_linenum:
    la      r1, BASIC_LINE_BUF
    move.q  r8, r0                  ; Accumulator = 0
.loop:
    load.b  r2, (r1)
    ; Check digit range (0x30..0x39)
    move.q  r3, #0x30
    blt     r2, r3, .done
    move.q  r3, #0x3A
    bge     r2, r3, .done
    ; accumulator = accumulator * 10 + (char - '0')
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
; repl_check_run - Check if input is "RUN" (case-insensitive)
; ============================================================================
; Input:  R1 = pointer to input buffer
; Output: R8 = 1 if RUN, 0 otherwise
; Clobbers: R2-R5

repl_check_run:
    jsr     repl_skip_spaces

    load.b  r2, (r1)
    or.l    r2, r2, #0x20           ; Force lowercase
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
; repl_parse_run_filename - Parse RUN "file" from input line
; ============================================================================
; Extracts a quoted filename from a RUN command. If no quote follows RUN,
; returns 0 (indicating a plain RUN of the stored programme).
;
; Input:  BASIC_LINE_BUF contains the line
; Output: R8 = 1 if FILE_NAME_BUF was populated, 0 if no quoted filename
; Clobbers: R1-R3, R10-R11

repl_parse_run_filename:
    la      r1, BASIC_LINE_BUF
    jsr     repl_skip_spaces

    ; Match "RUN" prefix
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x72               ; 'r'
    bne     r2, r3, .no_file

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x75               ; 'u'
    bne     r2, r3, .no_file

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x6E               ; 'n'
    bne     r2, r3, .no_file

    add.q   r1, r1, #1
    jsr     repl_skip_spaces

    ; Require opening quote for external RUN
    load.b  r2, (r1)
    move.q  r3, #0x22               ; '"'
    bne     r2, r3, .no_file
    add.q   r1, r1, #1

    ; Copy quoted filename into FILE_NAME_BUF
    la      r10, FILE_NAME_BUF
    move.q  r11, #255              ; Max chars to prevent buffer overflow
.copy_name_loop:
    load.b  r2, (r1)
    beqz    r2, .no_file            ; Unterminated quote
    move.q  r3, #0x22               ; Closing quote
    beq     r2, r3, .copy_done
    beqz    r11, .no_file
    store.b r2, (r10)
    add.q   r1, r1, #1
    add.q   r10, r10, #1
    sub.q   r11, r11, #1
    bra     .copy_name_loop

.copy_done:
    store.b r0, (r10)
    move.q  r8, #1
    rts

.no_file:
    move.q  r8, r0
    rts

; ============================================================================
; repl_check_list - Check if input is "LIST" (case-insensitive)
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
; repl_check_new - Check if input is "NEW" (case-insensitive)
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
; repl_skip_spaces - Advance R1 past any space characters
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
; STRING DATA
; ============================================================================

repl_str_banner:
    dc.b    "EhBASIC IE64 v1.0", 0x0D, 0x0A
    dc.b    "(c) Zayn Otley, 2024-2026", 0x0D, 0x0A
    dc.b    "Based on EhBASIC by Lee Davison", 0
    align 4

repl_str_ready:
    dc.b    "Ready", 0
    align 4

repl_msg_file_error:
    dc.b    "?FILE ERROR", 0
    align 4

; ============================================================================
; INCLUDE ALL INTERPRETER MODULES
; ============================================================================
; Each module handles a specific area of the interpreter:
;   io           - Terminal input/output (serial MMIO)
;   tokenizer    - Keyword-to-token conversion
;   lineeditor   - Numbered line storage and retrieval
;   expr         - Expression evaluator (arithmetic, functions, strings)
;   vars         - Variable and array storage
;   strings      - String manipulation functions (LEFT$, MID$, CHR$, etc.)
;   exec         - Statement executor and control flow
;   hw_video     - SCREEN, CLS, PLOT, PALETTE, VSYNC commands
;   hw_audio     - SOUND, ENVELOPE, GATE, REVERB, OVERDRIVE commands
;   hw_system    - WAIT, POKE, PEEK, and system-level commands
;   hw_voodoo    - VOODOO 3D graphics commands (TRIANGLE, TEXTURE, etc.)
;   file_io      - BLOAD/BSAVE file operations
;   hw_coproc    - Coprocessor management commands
;   ie64_fp      - IEEE 754 FP32 soft-float library

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
include "ehbasic_file_io.inc"
include "ehbasic_hw_coproc.inc"
include "ie64_fp.inc"
