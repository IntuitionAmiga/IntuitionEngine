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
    move.q  r2, #32
.clear_state:
    store.q r0, (r1)
    add.q   r1, r1, #8
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
    bnez    r8, repl_loop

    bra     repl_read               ; Back to input (no "Ready" prompt)

repl_delete_line:
    ; Delete line: call line_store with length 0
    move.q  r8, r22                 ; R8 = line number
    move.q  r9, r0                  ; R9 = unused
    move.q  r10, r0                 ; R10 = 0 (signals deletion)
    jsr     line_store
    bnez    r8, repl_loop
    bra     repl_read

repl_immediate:
    ; --- Immediate mode: check for special commands, then tokenise and execute ---

    ; Check for RUN AOT command (must precede plain RUN, which would also
    ; match the "RUN" prefix and run interpreted instead of compiling).
    la      r1, BASIC_LINE_BUF
    jsr     repl_check_run_aot
    bnez    r8, repl_do_run_aot

    ; Check for RUN command (before tokenising)
    la      r1, BASIC_LINE_BUF
    jsr     repl_check_run
    bnez    r8, repl_do_run

    ; Check for COMPILE command (direct-only AOT to standalone .ie64)
    la      r1, BASIC_LINE_BUF
    jsr     repl_check_compile
    bnez    r8, repl_do_compile

    ; Check for TRANSPILE command (direct-only: transpile to NAME.asm only,
    ; the first half of COMPILE without assembling or writing NAME.ie64).
    la      r1, BASIC_LINE_BUF
    jsr     repl_check_transpile
    bnez    r8, repl_do_transpile

    ; Check for ASSEMBLE command (direct-only: assemble NAME.asm from disk to
    ; NAME.ie64 with the in-guest assembler; general user-asm, not BASIC).
    la      r1, BASIC_LINE_BUF
    jsr     repl_check_assemble
    bnez    r8, repl_do_assemble

    ; Native CONT: only when a RUN AOT STOP left a pending continuation
    ; (AOT_CONT_PC != 0). A typed CONT then re-enters the compiled arena. With no
    ; pending continuation, CONT falls through to tokenise + interpreted exec_do_cont.
    la      r1, AOT_CONT_PC
    load.q  r2, (r1)
    beqz    r2, .no_aot_cont
    la      r1, BASIC_LINE_BUF
    jsr     repl_check_cont
    bnez    r8, repl_do_cont_aot
.no_aot_cont:

    ; Check for DIR command
    la      r1, BASIC_LINE_BUF
    jsr     repl_check_dir
    bnez    r8, repl_do_dir

    ; Check for TYPE command (direct-only: print a text file to the screen)
    la      r1, BASIC_LINE_BUF
    jsr     repl_check_type
    bnez    r8, repl_do_type

    ; Check for LIST command
    la      r1, BASIC_LINE_BUF
    jsr     repl_check_list
    bnez    r8, repl_do_list

    ; Check for NEW command
    la      r1, BASIC_LINE_BUF
    jsr     repl_check_new
    bnez    r8, repl_do_new

    ; Check for EMUTOS command
    la      r1, BASIC_LINE_BUF
    jsr     repl_check_emutos
    bnez    r8, repl_do_emutos

    ; Check for AROS command
    la      r1, BASIC_LINE_BUF
    jsr     repl_check_aros
    bnez    r8, repl_do_aros

    ; Check for INTUITIONOS command
    la      r1, BASIC_LINE_BUF
    jsr     repl_check_intuitionos
    bnez    r8, repl_do_intuitionos

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

    ; Successful direct commands clear the persistent last-error state.
    beqz    r8, repl_clear_error_state
    bra     repl_loop

repl_clear_error_state:
    add.q   r1, r16, #ST_ERROR_FLAG
    store.l r0, (r1)
    add.q   r1, r16, #ST_ERROR_LINE
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
    ; Interpreted RUN restarts the programme; any RUN AOT STOP continuation is stale.
    la      r1, AOT_CONT_PC
    store.q r0, (r1)
    jsr     exec_run
    bra     repl_loop

.run_file_error:
    la      r8, repl_msg_file_error
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop

; ============================================================================
; RUN AOT command handler - compile the stored programme to native IE64 code
; ============================================================================
; Direct-only. Prints the compile banner, then compiles the stored programme
; into a top-of-RAM arena and runs it in place (the native code ends with rts).

repl_do_run_aot:
    ; RUN AOT takes no arguments. Re-walk past "AOT" and reject any trailing
    ; token (the file form "RUN AOT \"file\"" is explicitly unsupported); only
    ; trailing spaces before end-of-line are allowed.
    la      r1, BASIC_LINE_BUF
    jsr     repl_skip_spaces
    add.q   r1, r1, #3              ; past "RUN"
    jsr     repl_skip_spaces
    add.q   r1, r1, #3              ; past "AOT"
    jsr     repl_skip_spaces
    load.b  r2, (r1)
    bnez    r2, .run_aot_extra

    la      r8, repl_msg_compiling
    jsr     print_string
    jsr     print_crlf
    jsr     aot_compile_check       ; reject direct-only/raw roots first
    bnez    r8, repl_loop           ; reasoned error already printed
    jsr     aot_do_run_aot          ; compile into the arena and execute it
    beqz    r8, repl_loop           ; 0 = success (native code ran)
    move.q  r3, #1
    beq     r8, r3, .run_aot_unsupported
    move.q  r3, #2
    beq     r8, r3, .run_aot_oom
    move.q  r3, #4
    beq     r8, r3, .run_aot_ret_no_gosub
    la      r8, repl_msg_aot_asm_err   ; 3 = assembler error
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop
.run_aot_ret_no_gosub:
    ; 4 = RETURN without GOSUB at runtime. The compiled code set ST_CURRENT_LINE
    ; to the offending line; raise_error persists ST_ERROR_FLAG/ST_ERROR_LINE and
    ; prints "?RETURN WITHOUT GOSUB ERROR IN <line>", matching exec_do_return.
    move.q  r8, #ERR_RET_NO_GOSUB
    la      r9, err_msg_ret_no_gosub
    jsr     raise_error
    bra     repl_loop
.run_aot_unsupported:
    la      r8, repl_msg_aot_stub
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop
.run_aot_oom:
    la      r8, repl_msg_aot_oom
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop

.run_aot_extra:
    la      r8, repl_msg_syntax_in0
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop

; ============================================================================
; CONT (native) - resume a RUN AOT programme stopped by STOP
; ============================================================================
; Reached only when AOT_CONT_PC != 0 (a prior RUN AOT hit STOP). Re-enters the
; compiled arena at the saved resume address with variables, DATA, FOR and GOSUB
; state preserved. The arena code is still resident: the compiler allocator is
; deterministic, so unless a fresh RUN AOT/COMPILE ran (which clears AOT_CONT_PC)
; the bytes are unchanged.

repl_do_cont_aot:
    la      r1, AOT_CONT_PC
    load.q  r8, (r1)               ; R8 = saved resume address
    store.q r0, (r1)               ; consume it (a fresh STOP re-arms CONT)
    la      r1, AOT_RT_ERR
    store.q r0, (r1)               ; clear the runtime-error slot
    ; Re-establish the register conventions the arena code and bundled helpers use.
    la      r26, TERM_OUT
    la      r27, TERM_STATUS
    ; Do NOT reset ST_GOSUB_SP / AOT_GOSUB_SP / variables / DATA: CONT continues.
    jsr     aot_cont_enter         ; saves the entry SP, then jumps to R8
    ; Returns here on END or another STOP (the arena unwinds to the saved SP).
    la      r1, AOT_RT_ERR
    load.q  r1, (r1)
    bnez    r1, .cont_runtime_err
    bra     repl_loop
.cont_runtime_err:
    ; A RETURN without GOSUB during the resumed run. The arena set ST_CURRENT_LINE;
    ; raise_error prints "?RETURN WITHOUT GOSUB ERROR IN <line>" like the interpreter.
    move.q  r8, #ERR_RET_NO_GOSUB
    la      r9, err_msg_ret_no_gosub
    jsr     raise_error
    bra     repl_loop

; aot_cont_enter - establish a call frame whose return lands back in
; repl_do_cont_aot, record that frame's stack pointer as the arena unwind target
; (AOT_SAVED_SP), then tail-jump to the CONT resume address in R8. This mirrors the
; RUN AOT entry (jsr (r8) -> the prologue saves r31), so a later END/STOP
; "load.q r31, (AOT_SAVED_SP) ; rts" returns cleanly to the REPL.
aot_cont_enter:
    la      r1, AOT_SAVED_SP
    store.q r31, (r1)              ; SP here = inside this frame (after the jsr)
    jmp     (r8)

; ============================================================================
; COMPILE command handler - compile the stored programme to a standalone .ie64
; ============================================================================
; Direct-only. COMPILE "name" validates the filename and (later) writes a flat
; standalone image. Missing argument is a syntax error; empty, absolute, "..",
; or separator-containing names raise ?FC ERROR IN 0. A ".ie64" suffix is
; appended case-insensitively when absent.

repl_do_compile:
    move.q  r7, #7                      ; skip "COMPILE" in repl_parse_compile_name
    jsr     repl_parse_compile_name     ; R8: 0=syntax, 1=ok, 2=bad name
    beqz    r8, .compile_syntax
    move.q  r3, #2
    beq     r8, r3, .compile_fc
    ; R8 == 1: FILE_NAME_BUF holds the validated output name.
    jsr     aot_compile_check       ; reject direct-only/raw roots first
    bnez    r8, repl_loop           ; reasoned error already printed (no banner)
    jsr     aot_do_compile          ; transpile + assemble + write .ie64 and .asm
    beqz    r8, repl_loop           ; 0 = success (files written)
    move.q  r3, #1
    beq     r8, r3, .compile_unsupported
    move.q  r3, #2
    beq     r8, r3, .compile_oom
    move.q  r3, #3
    beq     r8, r3, .compile_asmerr
    la      r8, repl_msg_fileerr_in0   ; 4 = file write error
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop
.compile_unsupported:
    la      r8, repl_msg_aot_stub
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop
.compile_oom:
    la      r8, repl_msg_aot_oom
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop
.compile_asmerr:
    la      r8, repl_msg_aot_asm_err
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop

.compile_syntax:
    la      r8, repl_msg_syntax_in0
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop

.compile_fc:
    la      r8, repl_msg_fc_in0
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop

; ============================================================================
; TRANSPILE command handler - transpile the stored programme to NAME.asm
; ============================================================================
; Direct-only. TRANSPILE "name" does the first half of COMPILE: it transpiles
; the stored programme to IE64 assembly text and writes NAME.asm, without
; assembling or producing NAME.ie64. The validated name (with its ".ie64"
; suffix) is reused so aot_make_asm_name derives NAME.asm exactly as COMPILE
; would, leaving both commands' sidecar names in step.
repl_do_transpile:
    move.q  r7, #9                      ; skip "TRANSPILE" in repl_parse_compile_name
    jsr     repl_parse_compile_name     ; R8: 0=syntax, 1=ok, 2=bad name
    beqz    r8, .transpile_syntax
    move.q  r3, #2
    beq     r8, r3, .transpile_fc
    ; R8 == 1: FILE_NAME_BUF holds the validated output name.
    jsr     aot_compile_check       ; reject direct-only/raw roots first
    bnez    r8, repl_loop           ; reasoned error already printed
    jsr     aot_do_transpile        ; transpile + write NAME.asm
    beqz    r8, repl_loop           ; 0 = success (NAME.asm written)
    move.q  r3, #1
    beq     r8, r3, .transpile_unsupported
    move.q  r3, #2
    beq     r8, r3, .transpile_oom
    la      r8, repl_msg_fileerr_in0   ; 4 = file write error
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop
.transpile_unsupported:
    la      r8, repl_msg_aot_stub
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop
.transpile_oom:
    la      r8, repl_msg_aot_oom
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop
.transpile_syntax:
    la      r8, repl_msg_syntax_in0
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop
.transpile_fc:
    la      r8, repl_msg_fc_in0
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop

; ============================================================================
; ASSEMBLE command handler - assemble NAME.asm from disk to NAME.ie64
; ============================================================================
; Direct-only. ASSEMBLE "name" reads name.asm (beside the most recently LOADed
; programme, or the File I/O root), assembles it at PROGRAM_START with the
; in-guest private assembler and writes name.ie64. The source is general
; user-written IE64 assembly, independent of any stored BASIC programme, so the
; stored programme is left untouched and aot_compile_check is not run.
repl_do_assemble:
    move.q  r7, #8                      ; skip "ASSEMBLE" in repl_parse_compile_name
    jsr     repl_parse_compile_name     ; R8: 0=syntax, 1=ok, 2=bad name
    beqz    r8, .assemble_syntax
    move.q  r3, #2
    beq     r8, r3, .assemble_fc
    ; R8 == 1: FILE_NAME_BUF holds the validated name.ie64 output name.
    jsr     aot_do_assemble         ; read name.asm, assemble, write name.ie64
    beqz    r8, repl_loop           ; 0 = success (name.ie64 written)
    move.q  r3, #2
    beq     r8, r3, .assemble_oom
    move.q  r3, #3
    beq     r8, r3, .assemble_asmerr
    la      r8, repl_msg_fileerr_in0   ; 4 = file read/write error
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop
.assemble_oom:
    la      r8, repl_msg_aot_oom
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop
.assemble_asmerr:
    la      r8, repl_msg_aot_asm_err
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop
.assemble_syntax:
    la      r8, repl_msg_syntax_in0
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop
.assemble_fc:
    la      r8, repl_msg_fc_in0
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop

; ============================================================================
; EMUTOS command handler - boot EmuTOS ROM
; ============================================================================
; Writes EXEC_OP_EMUTOS (2) to EXEC_CTRL, then polls EXEC_SESSION/EXEC_STATUS
; with bounded loops. On error, prints ?EMUTOS NOT AVAILABLE.

repl_do_emutos:
    ; Read current session value before triggering executor
    la      r1, EXEC_SESSION
    load.l  r21, (r1)

    ; Trigger EmuTOS boot (no filename needed)
    la      r1, EXEC_CTRL
    move.q  r2, #2
    store.l r2, (r1)

    ; Wait for EXEC_SESSION to advance (bounded poll)
    move.q  r25, #0x200000
.emu_wait_session:
    la      r1, EXEC_SESSION
    load.l  r22, (r1)
    bne     r22, r21, .emu_wait_status
    sub.q   r25, r25, #1
    bnez    r25, .emu_wait_session
    bra     .emu_error

    ; Poll status for the new session (bounded)
.emu_wait_status:
    move.q  r25, #0x400000
.emu_status_loop:
    la      r1, EXEC_STATUS
    load.l  r22, (r1)
    move.q  r23, #1
    beq     r22, r23, .emu_status_waiting
    move.q  r23, #3
    beq     r22, r23, .emu_error
    move.q  r23, #2
    beq     r22, r23, .emu_ok
    bra     .emu_error

.emu_status_waiting:
    sub.q   r25, r25, #1
    bnez    r25, .emu_status_loop
    bra     .emu_error

.emu_ok:
    bra     repl_loop

.emu_error:
    la      r8, repl_msg_emutos_error
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop

; ============================================================================
; AROS command handler - boot AROS
; ============================================================================
; Writes EXEC_OP_AROS (3) to EXEC_CTRL, then polls EXEC_SESSION/EXEC_STATUS
; with bounded loops. On error, prints ?AROS NOT AVAILABLE.

repl_do_aros:
    ; Read current session value before triggering executor
    la      r1, EXEC_SESSION
    load.l  r21, (r1)

    ; Trigger AROS boot (no filename needed)
    la      r1, EXEC_CTRL
    move.q  r2, #3
    store.l r2, (r1)

    ; Wait for EXEC_SESSION to advance (bounded poll)
    move.q  r25, #0x200000
.aros_wait_session:
    la      r1, EXEC_SESSION
    load.l  r22, (r1)
    bne     r22, r21, .aros_wait_status
    sub.q   r25, r25, #1
    bnez    r25, .aros_wait_session
    bra     .aros_error

    ; Poll status for the new session (bounded)
.aros_wait_status:
    move.q  r25, #0x400000
.aros_status_loop:
    la      r1, EXEC_STATUS
    load.l  r22, (r1)
    move.q  r23, #1
    beq     r22, r23, .aros_status_waiting
    move.q  r23, #3
    beq     r22, r23, .aros_error
    move.q  r23, #2
    beq     r22, r23, .aros_ok
    bra     .aros_error

.aros_status_waiting:
    sub.q   r25, r25, #1
    bnez    r25, .aros_status_loop
    bra     .aros_error

.aros_ok:
    bra     repl_loop

.aros_error:
    la      r8, repl_msg_aros_error
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop

; ----------------------------------------------------------------------------
; INTUITIONOS command handler
; Writes EXEC_OP_IEXEC (4) to EXEC_CTRL, then polls status.
; ----------------------------------------------------------------------------

repl_do_intuitionos:
    ; Read current session value before triggering executor
    la      r1, EXEC_SESSION
    load.l  r21, (r1)

    ; Trigger IntuitionOS boot
    la      r1, EXEC_CTRL
    move.q  r2, #4
    store.l r2, (r1)

    ; Wait for EXEC_SESSION to advance (bounded poll)
    move.q  r25, #0x200000
.ios_wait_session:
    la      r1, EXEC_SESSION
    load.l  r22, (r1)
    bne     r22, r21, .ios_wait_status
    sub.q   r25, r25, #1
    bnez    r25, .ios_wait_session
    bra     .ios_error

    ; Poll status for the new session (bounded)
.ios_wait_status:
    move.q  r25, #0x400000
.ios_status_loop:
    la      r1, EXEC_STATUS
    load.l  r22, (r1)
    move.q  r23, #1
    beq     r22, r23, .ios_status_waiting
    move.q  r23, #3
    beq     r22, r23, .ios_error
    move.q  r23, #2
    beq     r22, r23, .ios_ok
    bra     .ios_error

.ios_status_waiting:
    sub.q   r25, r25, #1
    bnez    r25, .ios_status_loop
    bra     .ios_error

.ios_ok:
    bra     repl_loop

.ios_error:
    la      r8, repl_msg_intuitionos_error
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
; DIR command handler - display host directory listing through File I/O
; ============================================================================
; Supports:
;   DIR
;   DIR "path"

repl_do_dir:
    jsr     repl_parse_dir_path
    beqz    r8, .dir_file_error

    ; Setup MMIO
    la      r1, FILE_NAME_PTR
    la      r2, FILE_NAME_BUF
    store.l r2, (r1)

    la      r1, FILE_DATA_PTR
    la      r2, FILE_DATA_BUF
    store.l r2, (r1)

    la      r1, FILE_CTRL
    li      r2, #3                  ; OP_LIST
    store.l r2, (r1)

    ; Check status
    la      r1, FILE_STATUS
    load.l  r1, (r1)
    beqz    r1, .dir_success

    la      r1, FILE_ERROR_CODE
    load.l  r1, (r1)
    move.q  r2, #1                  ; FILE_ERR_NOT_FOUND
    beq     r1, r2, .dir_not_found
    bra     .dir_file_error

.dir_success:
    la      r8, FILE_DATA_BUF
    jsr     print_string
    bra     repl_loop

.dir_not_found:
    la      r8, repl_msg_file_not_found
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop

.dir_file_error:
    la      r8, repl_msg_file_error
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop

; ============================================================================
; TYPE command handler - print a text file to the screen (MSDOS-style)
; ============================================================================
; Direct-only. TYPE "path" reads the file (relative to the File I/O root, path
; separators allowed) into the resident File I/O buffer FILE_DATA_BUF, refuses
; to print it unless the whole file is valid ASCII/UTF-8, and writes it to the
; terminal. Binary files are rejected so their control bytes never reach the
; screen. The read is capped at the device to the buffer's usable span less one
; byte (room for a null terminator); a larger file is refused as ?FILE TOO LARGE
; before a single byte is staged. No allocation, no compiler state touched.

repl_do_type:
    jsr     repl_parse_type_path        ; R8: 1 = ok (FILE_NAME_BUF = path), 0 = syntax
    beqz    r8, .type_syntax

    ; Cap the read so an over-large file is refused before any byte is staged. The
    ; buffer spans FILE_DATA_BUF .. AOT_ALLOC_FLOOR; keep one byte for the null
    ; terminator. The cap is one-shot (consumed by the device per read).
    la      r1, FILE_READ_MAX
    move.q  r2, #(AOT_ALLOC_FLOOR - FILE_DATA_BUF - 1)
    store.l r2, (r1)

    ; OP_READ "path" -> FILE_DATA_BUF.
    la      r1, FILE_NAME_PTR
    la      r2, FILE_NAME_BUF
    store.l r2, (r1)
    la      r1, FILE_DATA_PTR
    la      r2, FILE_DATA_BUF
    store.l r2, (r1)
    la      r1, FILE_CTRL
    move.q  r2, #FILE_OP_READ
    store.l r2, (r1)

    la      r1, FILE_STATUS
    load.l  r1, (r1)
    bnez    r1, .type_read_err

    ; Validate the bytes as text before anything reaches the terminal.
    la      r8, FILE_DATA_BUF
    la      r1, FILE_RESULT_LEN
    load.l  r9, (r1)
    jsr     type_is_text                ; R8 = 1 text, 0 binary
    beqz    r8, .type_binary

    ; Print the file, normalising line endings to CR+LF (the terminal needs a CR
    ; to return to column 0; a Unix file carries bare LFs which would otherwise
    ; staircase across the screen).
    la      r8, FILE_DATA_BUF
    la      r1, FILE_RESULT_LEN
    load.l  r9, (r1)
    jsr     type_print

    ; Resume the prompt on a fresh line unless the file already ended with a line
    ; break (type_print emitted the CRLF). An empty file (length 0) also gets one.
    la      r1, FILE_RESULT_LEN
    load.l  r2, (r1)
    beqz    r2, .type_crlf
    la      r1, FILE_DATA_BUF
    add.q   r1, r1, r2
    sub.q   r1, r1, #1
    load.b  r1, (r1)
    move.q  r3, #0x0A
    beq     r1, r3, repl_loop
    move.q  r3, #0x0D
    beq     r1, r3, repl_loop
.type_crlf:
    jsr     print_crlf
    bra     repl_loop

.type_read_err:
    la      r1, FILE_ERROR_CODE
    load.l  r1, (r1)
    move.q  r2, #1                      ; FILE_ERR_NOT_FOUND
    beq     r1, r2, .type_not_found
    move.q  r2, #4                      ; FILE_ERR_RANGE (over the size cap)
    beq     r1, r2, .type_too_large
    bra     .type_file_err
.type_not_found:
    la      r8, repl_msg_file_not_found
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop
.type_too_large:
    la      r8, repl_msg_file_too_large
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop
.type_file_err:
    la      r8, repl_msg_file_error
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop
.type_binary:
    la      r8, repl_msg_not_text
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop
.type_syntax:
    la      r8, repl_msg_syntax_in0
    jsr     print_string
    jsr     print_crlf
    bra     repl_loop

; ============================================================================
; type_print - print a byte range, normalising line endings to CR+LF
; ============================================================================
; The terminal needs a CR to return to column 0, but a text file may use bare
; LF (Unix), bare CR (classic Mac) or CR+LF (DOS). Each of those is emitted as a
; single CR+LF, so the file always renders left-aligned regardless of origin.
; Input:  R8 = buffer base, R9 = byte length
; Clobbers: R1-R6, R8 (preserves nothing; caller reloads as needed)

type_print:
    move.q  r3, r8                      ; R3 = cursor
    move.q  r4, r9                      ; R4 = remaining bytes
.tp_loop:
    beqz    r4, .tp_done
    load.b  r5, (r3)
    move.q  r6, #0x0D
    beq     r5, r6, .tp_cr
    move.q  r6, #0x0A
    beq     r5, r6, .tp_nl
    ; Ordinary byte: emit verbatim.
    push    r3
    push    r4
    move.q  r8, r5
    jsr     term_putc
    pop     r4
    pop     r3
    add.q   r3, r3, #1
    sub.q   r4, r4, #1
    bra     .tp_loop
.tp_cr:
    ; CR (optionally followed by LF): emit one CR+LF and swallow a paired LF.
    push    r3
    push    r4
    jsr     print_crlf
    pop     r4
    pop     r3
    add.q   r3, r3, #1
    sub.q   r4, r4, #1
    beqz    r4, .tp_loop
    load.b  r5, (r3)
    move.q  r6, #0x0A
    bne     r5, r6, .tp_loop
    add.q   r3, r3, #1                  ; consume the LF of a CR+LF pair
    sub.q   r4, r4, #1
    bra     .tp_loop
.tp_nl:
    ; Bare LF: emit CR+LF.
    push    r3
    push    r4
    jsr     print_crlf
    pop     r4
    pop     r3
    add.q   r3, r3, #1
    sub.q   r4, r4, #1
    bra     .tp_loop
.tp_done:
    rts

; ============================================================================
; type_is_text - classify a byte range as printable text
; ============================================================================
; Input:  R8 = buffer base, R9 = byte length
; Output: R8 = 1 if the whole range is printable text, 0 if any byte is binary
; A byte is binary only if it is a control char other than tab/LF/CR (this
; includes NUL and every other 0x00..0x1F code) or DEL (0x7F). Bytes 0x20..0x7E
; are ASCII; bytes 0x80..0xFF are accepted as printable extended characters, so
; both UTF-8 (multibyte) and legacy 8-bit encodings such as ISO-8859-1 (Latin-1,
; classic AmigaOS) and Windows-1252 render. No UTF-8 structure is required: the
; high range is intentionally permissive. Binary files are still caught because
; machine code and images almost always contain NUL or low control bytes.
; Clobbers: R3-R6

type_is_text:
    move.q  r3, r8                      ; R3 = cursor
    move.q  r4, r9                      ; R4 = remaining bytes
.it_loop:
    beqz    r4, .it_text
    load.b  r5, (r3)
    move.q  r6, #0x20
    bge     r5, r6, .it_geq20           ; >= 0x20: printable, DEL handled below
    ; control char < 0x20: allow tab / LF / CR only
    move.q  r6, #0x09
    beq     r5, r6, .it_adv
    move.q  r6, #0x0A
    beq     r5, r6, .it_adv
    move.q  r6, #0x0D
    beq     r5, r6, .it_adv
    bra     .it_binary
.it_geq20:
    move.q  r6, #0x7F
    beq     r5, r6, .it_binary          ; DEL (0x80..0xFF fall through as printable)
.it_adv:
    add.q   r3, r3, #1
    sub.q   r4, r4, #1
    bra     .it_loop

.it_text:
    move.q  r8, #1
    rts
.it_binary:
    move.q  r8, r0
    rts

; ============================================================================
; NEW command handler - clear programme and variables
; ============================================================================

repl_do_new:
    jsr     line_init
    jsr     var_init
    ; Reset DATA pointer
    add.q   r1, r16, #ST_DATA_PTR
    store.l r0, (r1)
    ; NEW clears the programme, so drop any pending RUN AOT STOP continuation.
    la      r1, AOT_CONT_PC
    store.q r0, (r1)
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
; repl_check_run_aot - Check if input is "RUN AOT" (case-insensitive)
; ============================================================================
; Matches "RUN" followed by at least one space and then "AOT", terminated by a
; space or end of line. Must be tried before repl_check_run.
; Input:  R1 = pointer to input buffer
; Output: R8 = 1 if RUN AOT, 0 otherwise
; Clobbers: R2, R3

repl_check_run_aot:
    jsr     repl_skip_spaces

    load.b  r2, (r1)
    or.l    r2, r2, #0x20
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
    ; "RUN" must be followed by a space to introduce the AOT argument
    load.b  r2, (r1)
    move.q  r3, #0x20
    bne     r2, r3, .no
    jsr     repl_skip_spaces

    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x61               ; 'a'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x6F               ; 'o'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x74               ; 't'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    ; Word boundary after "AOT": claim the line for end-of-line, space, or any
    ; punctuation (':', ',', '=', '"', ...) so the handler can reject the tail.
    ; Only an identifier continuation (alpha/digit/'$', i.e. "AOTx") is NOT the
    ; AOT command and falls through to the plain RUN check.
    move.q  r3, #0x24               ; '$' string-var suffix -> longer identifier
    beq     r2, r3, .no
    or.l    r2, r2, #0x20           ; fold case for the letter test
    move.q  r3, #0x30               ; '0'
    blt     r2, r3, .yes            ; null / space / punctuation below '0'
    move.q  r3, #0x39               ; '9'
    ble     r2, r3, .no             ; digit -> identifier
    move.q  r3, #0x61               ; 'a'
    blt     r2, r3, .yes            ; punctuation between '9' and 'a' (':', '=', ...)
    move.q  r3, #0x7A               ; 'z'
    ble     r2, r3, .no             ; letter -> identifier
    bra     .yes                    ; punctuation above 'z'

.yes:
    move.q  r8, #1
    rts
.no:
    move.q  r8, r0
    rts

; ============================================================================
; repl_check_compile - Check if input is "COMPILE" (case-insensitive)
; ============================================================================
; Input:  R1 = pointer to input buffer
; Output: R8 = 1 if COMPILE, 0 otherwise
; Clobbers: R2, R3

repl_check_compile:
    jsr     repl_skip_spaces

    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x63               ; 'c'
    bne     r2, r3, .no
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x6F               ; 'o'
    bne     r2, r3, .no
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x6D               ; 'm'
    bne     r2, r3, .no
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x70               ; 'p'
    bne     r2, r3, .no
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x69               ; 'i'
    bne     r2, r3, .no
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x6C               ; 'l'
    bne     r2, r3, .no
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x65               ; 'e'
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
; repl_check_transpile - Check if input is "TRANSPILE" (case-insensitive)
; ============================================================================
; Input:  R1 = pointer to input buffer
; Output: R8 = 1 if TRANSPILE, 0 otherwise
; Clobbers: R2, R3

repl_check_transpile:
    jsr     repl_skip_spaces

    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x74               ; 't'
    bne     r2, r3, .tno
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x72               ; 'r'
    bne     r2, r3, .tno
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x61               ; 'a'
    bne     r2, r3, .tno
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x6E               ; 'n'
    bne     r2, r3, .tno
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x73               ; 's'
    bne     r2, r3, .tno
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x70               ; 'p'
    bne     r2, r3, .tno
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x69               ; 'i'
    bne     r2, r3, .tno
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x6C               ; 'l'
    bne     r2, r3, .tno
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x65               ; 'e'
    bne     r2, r3, .tno
    add.q   r1, r1, #1
    load.b  r2, (r1)
    beqz    r2, .tyes
    move.q  r3, #0x20
    beq     r2, r3, .tyes
    bra     .tno

.tyes:
    move.q  r8, #1
    rts
.tno:
    move.q  r8, r0
    rts

; ============================================================================
; repl_check_assemble - Check if input is "ASSEMBLE" (case-insensitive)
; ============================================================================
; Input:  R1 = pointer to input buffer
; Output: R8 = 1 if ASSEMBLE, 0 otherwise
; Clobbers: R2, R3

repl_check_assemble:
    jsr     repl_skip_spaces

    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x61               ; 'a'
    bne     r2, r3, .ano
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x73               ; 's'
    bne     r2, r3, .ano
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x73               ; 's'
    bne     r2, r3, .ano
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x65               ; 'e'
    bne     r2, r3, .ano
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x6D               ; 'm'
    bne     r2, r3, .ano
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x62               ; 'b'
    bne     r2, r3, .ano
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x6C               ; 'l'
    bne     r2, r3, .ano
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x65               ; 'e'
    bne     r2, r3, .ano
    add.q   r1, r1, #1
    load.b  r2, (r1)
    beqz    r2, .ayes
    move.q  r3, #0x20
    beq     r2, r3, .ayes
    bra     .ano

.ayes:
    move.q  r8, #1
    rts
.ano:
    move.q  r8, r0
    rts

; ============================================================================
; repl_parse_compile_name - Parse and validate COMPILE/TRANSPILE "name"
; ============================================================================
; Copies the quoted filename into FILE_NAME_BUF and validates it. A ".ie64"
; suffix is appended (case-insensitively) when absent.
;
; Input:  BASIC_LINE_BUF contains the COMPILE/TRANSPILE line
;         R7 = length of the matched leading keyword to skip (7 for COMPILE,
;              9 for TRANSPILE, 8 for ASSEMBLE)
; Output: R8 = 0 missing argument (syntax error)
;              1 valid (FILE_NAME_BUF populated)
;              2 bad name (?FC ERROR): empty, absolute, "..", separator, or
;                unterminated/over-length
; Clobbers: R1-R5, R10-R12, R20

repl_parse_compile_name:
    la      r1, BASIC_LINE_BUF
    jsr     repl_skip_spaces
    add.q   r1, r1, r7             ; skip the matched keyword (COMPILE=7/TRANSPILE=9)
    jsr     repl_skip_spaces

    ; Require an opening quote; anything else is a missing-argument syntax error
    load.b  r2, (r1)
    move.q  r3, #0x22               ; '"'
    bne     r2, r3, .syntax
    add.q   r1, r1, #1              ; past opening quote

    la      r10, FILE_NAME_BUF
    move.q  r11, #240               ; cap chars (room for ".ie64" + null)
    move.q  r12, r0                 ; copied length
    move.q  r20, r0                 ; previous char (for ".." detection)
.cpy:
    load.b  r2, (r1)
    beqz    r2, .fc                 ; unterminated quote
    move.q  r3, #0x22
    beq     r2, r3, .cpy_done       ; closing quote
    beqz    r11, .fc                ; over-length
    ; reject path separators (covers absolute "/..." and "sub/..." and "\")
    move.q  r3, #0x2F               ; '/'
    beq     r2, r3, .fc
    move.q  r3, #0x5C               ; '\'
    beq     r2, r3, .fc
    ; reject ".." (previous char and current char both '.')
    move.q  r3, #0x2E               ; '.'
    bne     r2, r3, .store
    move.q  r3, #0x2E
    beq     r20, r3, .fc
.store:
    store.b r2, (r10)
    add.q   r10, r10, #1
    add.q   r12, r12, #1
    sub.q   r11, r11, #1
    move.q  r20, r2
    add.q   r1, r1, #1
    bra     .cpy

.cpy_done:
    store.b r0, (r10)              ; null-terminate
    beqz    r12, .fc               ; empty name
    ; Only trailing spaces may follow the closing quote; anything else is a
    ; syntax error (COMPILE "name" takes no further arguments).
    add.q   r1, r1, #1             ; advance past the closing quote
.cd_tail:
    load.b  r2, (r1)
    move.q  r3, #0x20
    bne     r2, r3, .cd_tail_end
    add.q   r1, r1, #1
    bra     .cd_tail
.cd_tail_end:
    bnez    r2, .syntax            ; trailing junk after the filename
    jsr     aot_append_ie64        ; append ".ie64" if absent (R12 = length)
    move.q  r8, #1
    rts

.syntax:
    move.q  r8, r0
    rts
.fc:
    move.q  r8, #2
    rts

; ============================================================================
; aot_append_ie64 - Append ".ie64" to FILE_NAME_BUF unless already present
; ============================================================================
; The suffix check is case-insensitive, so "DEMO.IE64" is left unchanged.
; Input:  FILE_NAME_BUF null-terminated, R12 = current length
; Clobbers: R2-R5

aot_append_ie64:
    move.q  r3, #5
    blt     r12, r3, .append       ; too short to already carry ".ie64"

    la      r4, FILE_NAME_BUF
    add.q   r4, r4, r12
    sub.q   r4, r4, #5             ; R4 -> last 5 chars
    la      r5, aot_str_ie64ext
.cmp_loop:
    load.b  r3, (r5)
    beqz    r3, .already           ; matched all 5 -> suffix present
    load.b  r2, (r4)
    or.l    r2, r2, #0x20          ; lower-case the filename char
    bne     r2, r3, .append
    add.q   r4, r4, #1
    add.q   r5, r5, #1
    bra     .cmp_loop
.already:
    rts

.append:
    la      r4, FILE_NAME_BUF
    add.q   r4, r4, r12
    la      r5, aot_str_ie64ext
.app_loop:
    load.b  r2, (r5)
    beqz    r2, .app_done
    store.b r2, (r4)
    add.q   r4, r4, #1
    add.q   r5, r5, #1
    bra     .app_loop
.app_done:
    store.b r0, (r4)
    rts

aot_str_ie64ext:
    dc.b    ".ie64", 0
    align 4

; ============================================================================
; aot_compile_check - Reject direct-only/raw-root statements before compiling
; ============================================================================
; Walks the stored programme and rejects any statement whose root is a
; direct-only or non-compilable raw keyword: DIR, HOST, COSTART, COSTOP,
; COWAIT, COCALL, COSTATUS, COMPILE, and RUN AOT. Tokenised roots (PRINT, FOR,
; SOUND, ...) are accepted, including their documented raw subverbs such as
; SOUND PLAY. On the first offending statement it prints the canonical
; ?COMPILE ERROR IN <line>: <reason> and returns.
;
; Output: R8 = 0 if all statements are compilable, 1 if rejected (error printed)
; Clobbers: R1-R5, R8, R20

aot_compile_check:
    push    r14                     ; current line pointer
    push    r15                     ; next-line pointer
    push    r17                     ; statement cursor
    push    r18                     ; current line number

    load.l  r14, (r16)              ; first line

.acc_line:
    beqz    r14, .acc_ok
    ; Stop on the in-memory empty/end sentinel (mirror line_list).
    add.q   r1, r16, #4
    load.l  r1, (r1)
    sub.q   r1, r1, #4
    bne     r14, r1, .acc_real
    load.l  r15, (r14)
    beqz    r15, .acc_ok
.acc_real:
    load.l  r15, (r14)              ; next-line pointer
    add.q   r1, r14, #4
    load.l  r18, (r1)               ; R18 = line number
    add.q   r17, r14, #8            ; R17 = tokenised content

.acc_stmt:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    beqz    r1, .acc_next_line
    ; ':' and the IF clause keywords THEN/ELSE introduce a fresh statement
    ; root, so reclassify after each (the interpreter executes THEN/ELSE tails
    ; as statements). The IF condition itself contains no separators and is
    ; skipped by aot_scan_stmt_body up to THEN.
    move.q  r2, #0x3A               ; ':'
    beq     r1, r2, .acc_sep
    move.q  r2, #TK_THEN
    beq     r1, r2, .acc_sep
    move.q  r2, #TK_ELSE
    beq     r1, r2, .acc_sep
    bra     .acc_classify
.acc_sep:
    add.q   r17, r17, #1
    bra     .acc_stmt

.acc_classify:
    move.q  r2, #0x80
    bge     r1, r2, .acc_token_root

    ; Raw root: match against the rejected raw keywords.
    jsr     aot_check_raw_root      ; R8 = reason ptr (0 if none)
    bnez    r8, .acc_reject
    bra     .acc_allowed

.acc_token_root:
    move.q  r2, #TK_RUN
    bne     r1, r2, .acc_check_data
    ; RUN token: reject only the "RUN AOT" form; bare RUN is allowed.
    jsr     aot_run_is_aot          ; R8 = 1 if RUN AOT
    beqz    r8, .acc_allowed
    la      r8, aot_rsn_runaot
    bra     .acc_reject

.acc_check_data:
    ; DATA payload is literal data, not an expression. Skip it like the
    ; interpreter (to ':' or end-of-line) so values such as DATA COCALL(1) are
    ; not mistaken for a coprocessor call.
    move.q  r2, #TK_DATA
    bne     r1, r2, .acc_allowed
    jsr     aot_skip_data
    bra     .acc_stmt

.acc_allowed:
    jsr     aot_scan_stmt_body      ; advance R17 to separator; reject banned fns
    bnez    r8, .acc_reject         ; COCALL/COSTATUS in an expression
    bra     .acc_stmt

.acc_next_line:
    move.q  r14, r15
    bra     .acc_line

.acc_reject:
    jsr     aot_reject              ; R8 = reason ptr, R18 = line; prints error
    bra     .acc_epilogue           ; R8 = 1

.acc_ok:
    move.q  r8, r0

.acc_epilogue:
    pop     r18
    pop     r17
    pop     r15
    pop     r14
    rts

; aot_reject - print "?COMPILE ERROR IN <line>: <reason>" (R8=reason, R18=line)
aot_reject:
    move.q  r20, r8                 ; save reason pointer
    la      r8, aot_err_prefix
    jsr     print_string
    move.q  r8, r18
    jsr     io_print_uint32
    la      r8, aot_err_colon
    jsr     print_string
    move.q  r8, r20
    jsr     print_string
    jsr     print_crlf
    move.q  r8, #1
    rts

; aot_scan_stmt_body - advance R17 to the next statement separator (':' or the
; IF keywords THEN/ELSE) or null, skipping string literals and REM bodies. At
; each identifier boundary it also rejects the coprocessor functions COCALL and
; COSTATUS, which are non-compilable even when used inside an expression
; (X=COCALL(...), PRINT COSTATUS(1)). String/REM contents are not inspected, so
; PRINT "COCALL" is fine.
; Output: R8 = 0 (reached separator, R17 left at it) or reason ptr if rejected.
; Clobbers: R1-R5, R20
aot_scan_stmt_body:
    move.q  r20, r0                 ; previous char = 0 (treated as a boundary)
.sts_loop:
    load.b  r1, (r17)
    beqz    r1, .sts_sep
    move.q  r2, #0x3A               ; ':'
    beq     r1, r2, .sts_sep
    move.q  r2, #TK_THEN            ; end of IF condition / before THEN body
    beq     r1, r2, .sts_sep
    move.q  r2, #TK_ELSE            ; before ELSE body
    beq     r1, r2, .sts_sep
    move.q  r2, #TK_REM
    beq     r1, r2, .sts_rem
    move.q  r2, #0x22               ; '"'
    beq     r1, r2, .sts_str

    ; Only test for a function name at an identifier boundary: the previous
    ; char must not be alpha/digit/'$' (else we are mid-identifier).
    move.q  r2, r20
    move.q  r3, #0x24               ; '$'
    beq     r2, r3, .sts_advance
    or.q    r2, r2, #0x20
    move.q  r3, #0x30               ; '0'
    blt     r2, r3, .sts_try
    move.q  r3, #0x39               ; '9'
    ble     r2, r3, .sts_advance    ; prev digit
    move.q  r3, #0x61               ; 'a'
    blt     r2, r3, .sts_try
    move.q  r3, #0x7A               ; 'z'
    ble     r2, r3, .sts_advance    ; prev alpha
.sts_try:
    or.q    r2, r1, #0x20
    move.q  r3, #0x63               ; 'c' - COCALL / COSTATUS both start with C
    bne     r2, r3, .sts_advance
    ; Require call syntax: the interpreter only treats these names as
    ; coprocessor functions when followed by '(' (so a variable COCALL is fine).
    la      r5, aot_kw_costatus
    jsr     aot_match_fn
    bnez    r8, .sts_costatus
    la      r5, aot_kw_cocall
    jsr     aot_match_fn
    bnez    r8, .sts_cocall

.sts_advance:
    move.q  r20, r1
    add.q   r17, r17, #1
    bra     .sts_loop

.sts_str:
    add.q   r17, r17, #1
.sts_str_loop:
    load.b  r1, (r17)
    beqz    r1, .sts_sep
    move.q  r2, #0x22
    beq     r1, r2, .sts_str_close
    add.q   r17, r17, #1
    bra     .sts_str_loop
.sts_str_close:
    add.q   r17, r17, #1
    move.q  r20, #0x22              ; prev = '"' (a boundary)
    bra     .sts_loop

.sts_rem:
    load.b  r1, (r17)
    beqz    r1, .sts_sep
    add.q   r17, r17, #1
    bra     .sts_rem

.sts_sep:
    move.q  r8, r0
    rts
.sts_costatus:
    la      r8, aot_rsn_costatus
    rts
.sts_cocall:
    la      r8, aot_rsn_cocall
    rts

; aot_run_is_aot - R17 points at TK_RUN. Returns R8=1 if "RUN AOT". Clobbers R2-R4.
aot_run_is_aot:
    add.q   r4, r17, #1            ; past the RUN token
.ria_sp:
    load.b  r2, (r4)
    move.q  r3, #0x20
    bne     r2, r3, .ria_chk
    add.q   r4, r4, #1
    bra     .ria_sp
.ria_chk:
    or.q    r2, r2, #0x20
    move.q  r3, #0x61              ; 'a'
    bne     r2, r3, .ria_no
    load.b  r2, 1(r4)
    or.q    r2, r2, #0x20
    move.q  r3, #0x6F              ; 'o'
    bne     r2, r3, .ria_no
    load.b  r2, 2(r4)
    or.q    r2, r2, #0x20
    move.q  r3, #0x74              ; 't'
    bne     r2, r3, .ria_no
    ; Word boundary after "AOT": anything that is not an identifier continuation
    ; (alpha/digit/'$') is the direct-only RUN AOT form. Mirrors the immediate
    ; matcher repl_check_run_aot so punctuation tails (RUN AOT,1 / =1 / "...")
    ; are rejected, not treated as a plain RUN statement.
    load.b  r2, 3(r4)
    move.q  r3, #0x24              ; '$' -> identifier suffix
    beq     r2, r3, .ria_no
    or.q    r2, r2, #0x20
    move.q  r3, #0x30              ; '0'
    blt     r2, r3, .ria_yes       ; null / space / punctuation below '0'
    move.q  r3, #0x39              ; '9'
    ble     r2, r3, .ria_no        ; digit
    move.q  r3, #0x61              ; 'a'
    blt     r2, r3, .ria_yes       ; punctuation between '9' and 'a'
    move.q  r3, #0x7A              ; 'z'
    ble     r2, r3, .ria_no        ; letter
    bra     .ria_yes               ; punctuation above 'z' (incl. tokens >= 0x80)
.ria_yes:
    move.q  r8, #1
    rts
.ria_no:
    move.q  r8, r0
    rts

; aot_match_kw - match lowercase keyword at R5 against text at R17 with a word
; boundary. Returns R8=1 on match. Does not advance R17. Clobbers R2-R5.
aot_match_kw:
    move.q  r4, r17
.mk_loop:
    load.b  r3, (r5)
    beqz    r3, .mk_boundary       ; consumed whole keyword
    load.b  r2, (r4)
    or.q    r2, r2, #0x20
    bne     r2, r3, .mk_no
    add.q   r4, r4, #1
    add.q   r5, r5, #1
    bra     .mk_loop
.mk_boundary:
    load.b  r2, (r4)
    move.q  r3, #0x24              ; '$'
    beq     r2, r3, .mk_no
    or.q    r2, r2, #0x20
    move.q  r3, #0x30              ; '0'
    blt     r2, r3, .mk_yes        ; null/space/':'/etc
    move.q  r3, #0x39              ; '9'
    ble     r2, r3, .mk_no         ; digit
    move.q  r3, #0x61              ; 'a'
    blt     r2, r3, .mk_yes
    move.q  r3, #0x7A              ; 'z'
    ble     r2, r3, .mk_no         ; alpha
.mk_yes:
    move.q  r8, #1
    rts
.mk_no:
    move.q  r8, r0
    rts

; aot_match_fn - like aot_match_kw, but only succeeds when the matched keyword
; is in function-call form: the next non-space char must be '('. This matches
; the interpreter, which treats COCALL/COSTATUS as coprocessor functions only
; when called, leaving plain identifiers (X=COCALL+1) as ordinary variables.
; Input: R5 = lowercase keyword, R17 = text. Output: R8 = 1 on a call. Clobbers R2-R5.
aot_match_fn:
    move.q  r4, r17
.mf_loop:
    load.b  r3, (r5)
    beqz    r3, .mf_paren
    load.b  r2, (r4)
    or.q    r2, r2, #0x20
    bne     r2, r3, .mf_no
    add.q   r4, r4, #1
    add.q   r5, r5, #1
    bra     .mf_loop
.mf_paren:
    load.b  r2, (r4)
    move.q  r3, #0x20               ; skip spaces between name and '('
    bne     r2, r3, .mf_chk
    add.q   r4, r4, #1
    bra     .mf_paren
.mf_chk:
    move.q  r3, #0x28               ; '('
    beq     r2, r3, .mf_yes
.mf_no:
    move.q  r8, r0
    rts
.mf_yes:
    move.q  r8, #1
    rts

; aot_skip_data - advance R17 past a DATA payload to the next ':' or end of
; line, mirroring exec_do_data exactly (no string awareness). Clobbers R1, R2.
aot_skip_data:
.skd_loop:
    load.b  r1, (r17)
    beqz    r1, .skd_end
    move.q  r2, #0x3A               ; ':'
    beq     r1, r2, .skd_end
    add.q   r17, r17, #1
    bra     .skd_loop
.skd_end:
    rts

; aot_check_raw_root - R17 at a raw alpha root. Returns R8 = reason string ptr
; for a rejected keyword, or 0 if the root is an allowed statement (variable /
; implied LET). Clobbers R2-R5.
; COCALL and COSTATUS are not checked here: they are functions, valid as
; variable names without a following '(', so the call-syntax-aware body scan
; (aot_match_fn) handles them whether they appear as a root or in an expression.
aot_check_raw_root:
    la      r5, aot_kw_compile
    jsr     aot_match_kw
    bnez    r8, .crr_compile
    la      r5, aot_kw_costart
    jsr     aot_match_kw
    bnez    r8, .crr_costart
    la      r5, aot_kw_costop
    jsr     aot_match_kw
    bnez    r8, .crr_costop
    la      r5, aot_kw_cowait
    jsr     aot_match_kw
    bnez    r8, .crr_cowait
    la      r5, aot_kw_host
    jsr     aot_match_kw
    bnez    r8, .crr_host
    la      r5, aot_kw_dir
    jsr     aot_match_kw
    bnez    r8, .crr_dir
    la      r5, aot_kw_type
    jsr     aot_match_kw
    bnez    r8, .crr_type
    move.q  r8, r0
    rts
.crr_compile:
    ; DIR and COMPILE are not intercepted by exec_line, so the interpreter
    ; accepts COMPILE=... / COMPILE(...) as an implied-LET variable or array.
    ; Only flag them as direct-only when not used in assignment/array form.
    ; (HOST/COSTART/COSTOP/COWAIT are intercepted as commands regardless, so
    ; they are not exempted.)
    jsr     aot_root_is_var
    bnez    r8, .crr_allow
    la      r8, aot_rsn_compile
    rts
.crr_costart:
    la      r8, aot_rsn_costart
    rts
.crr_costop:
    la      r8, aot_rsn_costop
    rts
.crr_cowait:
    la      r8, aot_rsn_cowait
    rts
.crr_host:
    la      r8, aot_rsn_host
    rts
.crr_dir:
    jsr     aot_root_is_var
    bnez    r8, .crr_allow
    la      r8, aot_rsn_dir
    rts
.crr_type:
    ; TYPE, like DIR/COMPILE, is direct-only and not intercepted by exec_line, so
    ; the interpreter accepts TYPE=... / TYPE(...) as an implied-LET variable.
    jsr     aot_root_is_var
    bnez    r8, .crr_allow
    la      r8, aot_rsn_type
    rts
.crr_allow:
    move.q  r8, r0
    rts

; aot_root_is_var - R4 = pointer just past a matched DIR/COMPILE root keyword
; (left there by aot_match_kw). Returns R8=1 if the keyword is an implied-LET
; variable: the next non-space token is '=' (raw or TK_EQUAL) or '(' (array).
; Clobbers R2, R3, R4.
aot_root_is_var:
.riv_sp:
    load.b  r2, (r4)
    move.q  r3, #0x20               ; space
    bne     r2, r3, .riv_chk
    add.q   r4, r4, #1
    bra     .riv_sp
.riv_chk:
    move.q  r3, #0x3D               ; raw '='
    beq     r2, r3, .riv_yes
    move.q  r3, #TK_EQUAL           ; tokenised '='
    beq     r2, r3, .riv_yes
    move.q  r3, #0x28               ; '(' array subscript
    beq     r2, r3, .riv_yes
    move.q  r8, r0
    rts
.riv_yes:
    move.q  r8, #1
    rts

aot_err_prefix:
    dc.b    "?COMPILE ERROR IN ", 0
    align 4
aot_err_colon:
    dc.b    ": ", 0
    align 4

aot_kw_dir:      dc.b "dir", 0
aot_kw_host:     dc.b "host", 0
aot_kw_costart:  dc.b "costart", 0
aot_kw_costop:   dc.b "costop", 0
aot_kw_cowait:   dc.b "cowait", 0
aot_kw_cocall:   dc.b "cocall", 0
aot_kw_costatus: dc.b "costatus", 0
aot_kw_compile:  dc.b "compile", 0
aot_kw_type:     dc.b "type", 0
    align 4

aot_rsn_dir:      dc.b "DIR is direct-only", 0
aot_rsn_host:     dc.b "HOST cannot be compiled", 0
aot_rsn_costart:  dc.b "COSTART cannot be compiled", 0
aot_rsn_costop:   dc.b "COSTOP cannot be compiled", 0
aot_rsn_cowait:   dc.b "COWAIT cannot be compiled", 0
aot_rsn_cocall:   dc.b "COCALL cannot be compiled", 0
aot_rsn_costatus: dc.b "COSTATUS cannot be compiled", 0
aot_rsn_compile:  dc.b "COMPILE is direct-only", 0
aot_rsn_type:     dc.b "TYPE is direct-only", 0
aot_rsn_runaot:   dc.b "RUN AOT is direct-only", 0
    align 4

; ============================================================================
; repl_check_dir - Check if input is "DIR" (case-insensitive)
; ============================================================================
; Input:  R1 = pointer to input buffer
; Output: R8 = 1 if DIR, 0 otherwise
; Clobbers: R2-R5

repl_check_dir:
    jsr     repl_skip_spaces

    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x64               ; 'd'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x69               ; 'i'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x72               ; 'r'
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
; repl_parse_dir_path - Parse optional DIR "path" argument
; ============================================================================
; Input:  BASIC_LINE_BUF contains the line
; Output: R8 = 1 if FILE_NAME_BUF was populated, 0 on malformed input
; Clobbers: R1-R3, R10-R11

repl_parse_dir_path:
    la      r1, BASIC_LINE_BUF
    jsr     repl_skip_spaces

    ; Match "DIR" prefix
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x64               ; 'd'
    bne     r2, r3, .bad

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x69               ; 'i'
    bne     r2, r3, .bad

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x72               ; 'r'
    bne     r2, r3, .bad

    add.q   r1, r1, #1
    jsr     repl_skip_spaces

    ; No argument: list base directory
    load.b  r2, (r1)
    bnez    r2, .maybe_quote
    la      r10, FILE_NAME_BUF
    store.b r0, (r10)
    move.q  r8, #1
    rts

.maybe_quote:
    move.q  r3, #0x22               ; '"'
    bne     r2, r3, .bad
    add.q   r1, r1, #1

    ; Copy quoted path into FILE_NAME_BUF
    la      r10, FILE_NAME_BUF
    move.q  r11, #255
.copy_path_loop:
    load.b  r2, (r1)
    beqz    r2, .bad                ; Unterminated quote
    move.q  r3, #0x22
    beq     r2, r3, .copy_done
    beqz    r11, .bad
    store.b r2, (r10)
    add.q   r1, r1, #1
    add.q   r10, r10, #1
    sub.q   r11, r11, #1
    bra     .copy_path_loop

.copy_done:
    store.b r0, (r10)
    move.q  r8, #1
    rts

.bad:
    move.q  r8, r0
    rts

; ============================================================================
; repl_check_type - Check if input is "TYPE" (case-insensitive)
; ============================================================================
; Input:  R1 = pointer to input buffer
; Output: R8 = 1 if TYPE, 0 otherwise
; Clobbers: R2, R3

repl_check_type:
    jsr     repl_skip_spaces

    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x74               ; 't'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x79               ; 'y'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x70               ; 'p'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x65               ; 'e'
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
; repl_parse_type_path - Parse the required TYPE "path" argument
; ============================================================================
; Copies the quoted path into FILE_NAME_BUF. Unlike COMPILE/TRANSPILE/ASSEMBLE
; the path may contain separators (TYPE views a file anywhere under the File I/O
; root); the device enforces traversal protection. The argument is mandatory.
; Input:  BASIC_LINE_BUF contains the TYPE line
; Output: R8 = 1 if FILE_NAME_BUF was populated, 0 on syntax error
;              (missing/unquoted/empty/over-length path, or trailing junk)
; Clobbers: R1-R3, R10-R11

repl_parse_type_path:
    la      r1, BASIC_LINE_BUF
    jsr     repl_skip_spaces

    ; Match "TYPE" prefix
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x74               ; 't'
    bne     r2, r3, .bad
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x79               ; 'y'
    bne     r2, r3, .bad
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x70               ; 'p'
    bne     r2, r3, .bad
    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x65               ; 'e'
    bne     r2, r3, .bad
    add.q   r1, r1, #1
    jsr     repl_skip_spaces

    ; Require an opening quote (the argument is mandatory).
    load.b  r2, (r1)
    move.q  r3, #0x22               ; '"'
    bne     r2, r3, .bad
    add.q   r1, r1, #1

    ; Copy the quoted path into FILE_NAME_BUF.
    la      r10, FILE_NAME_BUF
    move.q  r11, #255
.copy_loop:
    load.b  r2, (r1)
    beqz    r2, .bad                ; unterminated quote
    move.q  r3, #0x22
    beq     r2, r3, .copy_done
    beqz    r11, .bad               ; over-length
    store.b r2, (r10)
    add.q   r1, r1, #1
    add.q   r10, r10, #1
    sub.q   r11, r11, #1
    bra     .copy_loop

.copy_done:
    store.b r0, (r10)              ; null-terminate
    ; Reject an empty path ("").
    la      r3, FILE_NAME_BUF
    load.b  r3, (r3)
    beqz    r3, .bad
    ; Only trailing spaces may follow the closing quote.
    add.q   r1, r1, #1
.tail:
    load.b  r2, (r1)
    move.q  r3, #0x20
    bne     r2, r3, .tail_end
    add.q   r1, r1, #1
    bra     .tail
.tail_end:
    bnez    r2, .bad               ; trailing junk after the path
    move.q  r8, #1
    rts

.bad:
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
; repl_check_cont - Check if input is "CONT" (case-insensitive)
; ============================================================================
; Input:  R1 = pointer to input buffer
; Output: R8 = 1 if CONT, 0 otherwise
; Clobbers: R2-R5

repl_check_cont:
    jsr     repl_skip_spaces

    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x63               ; 'c'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x6F               ; 'o'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x6E               ; 'n'
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
; repl_check_emutos - Check if input is "EMUTOS" (case-insensitive)
; ============================================================================
; Input:  R1 = pointer to input buffer
; Output: R8 = 1 if EMUTOS, 0 otherwise
; Clobbers: R2-R5

repl_check_emutos:
    jsr     repl_skip_spaces

    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x65               ; 'e'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x6D               ; 'm'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x75               ; 'u'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x74               ; 't'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x6F               ; 'o'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x73               ; 's'
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
; repl_check_aros - Check if input is "AROS" (case-insensitive)
; ============================================================================
; Input:  R1 = pointer to input buffer
; Output: R8 = 1 if AROS, 0 otherwise
; Clobbers: R2-R5

repl_check_aros:
    jsr     repl_skip_spaces

    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x61               ; 'a'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x72               ; 'r'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x6F               ; 'o'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x73               ; 's'
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
; repl_check_intuitionos - Check if input is "INTUITIONOS" (case-insensitive)
; ============================================================================
; Input:  R1 = pointer to input buffer
; Output: R8 = 1 if INTUITIONOS, 0 otherwise
; Clobbers: R2-R5

repl_check_intuitionos:
    jsr     repl_skip_spaces

    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x69               ; 'i'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x6E               ; 'n'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x74               ; 't'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x75               ; 'u'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x69               ; 'i'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x74               ; 't'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x69               ; 'i'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x6F               ; 'o'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x6E               ; 'n'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x6F               ; 'o'
    bne     r2, r3, .no

    add.q   r1, r1, #1
    load.b  r2, (r1)
    or.l    r2, r2, #0x20
    move.q  r3, #0x73               ; 's'
    bne     r2, r3, .no

    ; Check word boundary (next char must be NUL or space)
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
    dc.b    "EhBASIC IE64 v3.1", 0x0D, 0x0A
    dc.b    "(c) Zayn Otley, 2024-2026", 0x0D, 0x0A
    dc.b    "Based on EhBASIC by Lee Davison", 0
    align 4

repl_str_ready:
    dc.b    "Ready", 0
    align 4

repl_msg_file_error:
    dc.b    "?FILE ERROR", 0
    align 4

repl_msg_file_not_found:
    dc.b    "?FILE NOT FOUND", 0
    align 4

repl_msg_file_too_large:
    dc.b    "?FILE TOO LARGE", 0
    align 4

repl_msg_not_text:
    dc.b    "?NOT A TEXT FILE", 0
    align 4

repl_msg_emutos_error:
    dc.b    "?EMUTOS NOT AVAILABLE", 0
    align 4

repl_msg_aros_error:
    dc.b    "?AROS NOT AVAILABLE", 0
    align 4

repl_msg_intuitionos_error:
    dc.b    "?INTUITIONOS NOT AVAILABLE", 0
    align 4

repl_msg_compiling:
    dc.b    "Compiling to native code...", 0
    align 4

repl_msg_syntax_in0:
    dc.b    "?SYNTAX ERROR IN 0", 0
    align 4

repl_msg_fc_in0:
    dc.b    "?FC ERROR IN 0", 0
    align 4

; Temporary front-end placeholder for statements the transpiler cannot yet
; lower. RUN AOT still uses it; COMPILE reaches it only for unsupported programs.
repl_msg_aot_stub:
    dc.b    "?COMPILE ERROR IN 0: native code generation not yet implemented", 0
    align 4

repl_msg_aot_oom:
    dc.b    "?OUT OF MEMORY ERROR IN 0: out of compiler memory", 0
    align 4

repl_msg_fileerr_in0:
    dc.b    "?FILE ERROR IN 0", 0
    align 4

repl_msg_aot_asm_err:
    dc.b    "?COMPILE ERROR IN 0: internal assembler error", 0
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
;   hw_host      - HOST NET/UPDATE/REBOOT/POWEROFF bridge
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
include "ehbasic_hw_host.inc"
include "ehbasic_hw_voodoo.inc"
include "ehbasic_file_io.inc"
include "ehbasic_hw_coproc.inc"
include "ie64_fp.inc"
include "ehbasic_aot.inc"
