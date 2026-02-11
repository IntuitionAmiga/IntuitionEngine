; ehbasic_tokenizer.inc - EhBASIC IE64 Tokenizer
;
; Converts BASIC source text into compact token representation.
; Ported from ehbasic68k.asm LAB_13A6 ("crunch" routine).
;
; Uses a flat keyword table (tok_kw_table from ehbasic_tokens.inc).
; Each entry: null-terminated keyword string + token byte.
; Table ends with a lone 0x00 byte.
;
; Calling convention:
;   tokenize:
;     Input:  R8 = pointer to null-terminated input line
;             R9 = pointer to output buffer
;     Output: R8 = number of bytes written to output buffer
;     Clobbers: R1-R7, R10-R15
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; toupper - Convert byte to uppercase if lowercase
; ============================================================================
; Input:  R1 = byte
; Output: R1 = uppercased byte
; Clobbers: R2

toupper:
    move.q  r2, #0x61              ; 'a'
    blt     r1, r2, .tu_done
    move.q  r2, #0x7A              ; 'z'
    bgt     r1, r2, .tu_done
    sub.q   r1, r1, #0x20
.tu_done:
    rts

; ============================================================================
; tokenize - Convert BASIC source line to tokens
; ============================================================================
; Input:  R8 = source string pointer (null-terminated)
;         R9 = output buffer pointer
; Output: R8 = length of tokenized output (bytes)
; Clobbers: R1-R7, R10-R15

tokenize:
    push    r14
    push    r15

    move.q  r10, r8                 ; r10 = input pointer
    move.q  r11, r9                 ; r11 = output base pointer
    move.q  r12, r9                 ; r12 = output write pointer
    move.q  r13, r0                 ; r13 = mode: 0=normal, 1=DATA, 2=REM

.tok_main:
    load.b  r1, (r10)              ; get byte from input
    beqz    r1, .tok_eol            ; null = end of line

    ; If in REM mode, copy everything raw
    move.q  r2, #2
    beq     r13, r2, .tok_rem_copy

    ; Check for quote character
    move.q  r2, #0x22              ; '"'
    beq     r1, r2, .tok_quote

    ; If in DATA mode, copy raw but watch for ":"
    move.q  r2, #1
    beq     r13, r2, .tok_data_copy

    ; Try keyword matching
    ; First, check if this could be a keyword start character
    ; Keywords start with A-Z, a-z, or special chars (+, -, *, /, etc.)
    jsr     toupper                ; uppercase r1 for matching
    move.q  r14, r10               ; save input position

    ; Scan the flat keyword table
    la      r3, tok_kw_table       ; r3 = table pointer

.tok_scan_table:
    load.b  r4, (r3)               ; first byte of keyword
    beqz    r4, .tok_no_match      ; 0x00 = end of table

    ; Try to match this keyword against input at r14
    move.q  r5, r14                ; r5 = input scan pointer
    move.q  r6, r3                 ; r6 = keyword scan pointer

.tok_match_loop:
    load.b  r4, (r6)               ; keyword byte
    beqz    r4, .tok_matched       ; null = end of keyword, all matched!
    ; Get input byte and uppercase it
    load.b  r7, (r5)
    push    r1
    move.q  r1, r7
    jsr     toupper
    move.q  r7, r1
    pop     r1
    ; Compare
    bne     r4, r7, .tok_skip_entry
    add.q   r5, r5, #1             ; advance input
    add.q   r6, r6, #1             ; advance keyword
    bra     .tok_match_loop

.tok_matched:
    ; Keyword matched! r6 points to the null terminator.
    ; r3 = keyword start in table, r5 = input past keyword, r14 = match start
    ;
    ; Word boundary check: for alphabetic keywords, reject if the match
    ; is embedded inside a longer word. This prevents "OR" matching inside
    ; "BORDER", "POKE" matching inside "POKEY", etc.
    load.b  r15, (r3)              ; first char of keyword
    move.q  r2, #0x41              ; 'A'
    blt     r15, r2, .tok_emit     ; non-alpha keyword (operator), skip check
    move.q  r2, #0x5B              ; 'Z' + 1
    bge     r15, r2, .tok_emit     ; not uppercase letter, skip check

    ; Check char AFTER keyword — reject if alpha follows
    load.b  r7, (r5)
    ; Convert to uppercase for check
    move.q  r2, #0x61              ; 'a'
    blt     r7, r2, .tok_bnd_after_upper
    move.q  r2, #0x7B              ; 'z' + 1
    bge     r7, r2, .tok_bnd_after_upper
    sub.q   r7, r7, #0x20          ; to uppercase
.tok_bnd_after_upper:
    move.q  r2, #0x41              ; 'A'
    blt     r7, r2, .tok_bnd_before
    move.q  r2, #0x5B              ; 'Z' + 1
    blt     r7, r2, .tok_skip_entry ; letter follows → reject match

.tok_bnd_before:
    ; Check char BEFORE keyword — reject if alpha precedes
    beq     r14, r8, .tok_emit     ; at input start, no char before
    sub.q   r7, r14, #1
    load.b  r7, (r7)
    ; Convert to uppercase for check
    move.q  r2, #0x61              ; 'a'
    blt     r7, r2, .tok_bnd_pre_upper
    move.q  r2, #0x7B              ; 'z' + 1
    bge     r7, r2, .tok_bnd_pre_upper
    sub.q   r7, r7, #0x20
.tok_bnd_pre_upper:
    move.q  r2, #0x41              ; 'A'
    blt     r7, r2, .tok_emit
    move.q  r2, #0x5B              ; 'Z' + 1
    blt     r7, r2, .tok_skip_entry ; letter before → reject match

.tok_emit:
    ; The token byte is immediately after the null.
    add.q   r6, r6, #1            ; skip null
    load.b  r4, (r6)              ; read token byte
    store.b r4, (r12)             ; emit token
    add.q   r12, r12, #1
    move.q  r10, r5               ; advance input past matched keyword

    ; Check for REM or DATA
    move.q  r2, #TK_REM
    beq     r4, r2, .tok_set_rem
    move.q  r2, #TK_DATA
    beq     r4, r2, .tok_set_data
    bra     .tok_main

.tok_set_rem:
    move.q  r13, #2                ; REM mode
    bra     .tok_main

.tok_set_data:
    move.q  r13, #1                ; DATA mode
    bra     .tok_main

.tok_skip_entry:
    ; Skip to end of this keyword (find null terminator)
    load.b  r4, (r6)
    add.q   r6, r6, #1
    bnez    r4, .tok_skip_entry
    ; r6 now points past the null; next byte is the token
    add.q   r6, r6, #1            ; skip token byte
    move.q  r3, r6                ; update table pointer
    bra     .tok_scan_table

.tok_no_match:
    ; No keyword matched — copy the original byte
    load.b  r1, (r10)
    store.b r1, (r12)
    add.q   r12, r12, #1
    add.q   r10, r10, #1

    ; Check if this was ":" — reset DATA mode
    move.q  r2, #0x3A
    bne     r1, r2, .tok_main
    move.q  r13, r0                ; clear DATA mode
    bra     .tok_main

.tok_data_copy:
    ; DATA mode: copy raw, but ":" resets
    store.b r1, (r12)
    add.q   r12, r12, #1
    add.q   r10, r10, #1
    move.q  r2, #0x3A
    bne     r1, r2, .tok_main
    move.q  r13, r0
    bra     .tok_main

.tok_rem_copy:
    ; REM mode: copy everything until EOL
    store.b r1, (r12)
    add.q   r12, r12, #1
    add.q   r10, r10, #1
    bra     .tok_main

.tok_quote:
    ; Copy quoted string verbatim (including quotes)
    store.b r1, (r12)              ; emit opening quote
    add.q   r12, r12, #1
    add.q   r10, r10, #1
.tok_quote_loop:
    load.b  r1, (r10)
    beqz    r1, .tok_eol            ; unterminated string
    store.b r1, (r12)
    add.q   r12, r12, #1
    add.q   r10, r10, #1
    move.q  r2, #0x22              ; closing '"'
    bne     r1, r2, .tok_quote_loop
    bra     .tok_main

.tok_eol:
    ; Write null terminator
    store.b r0, (r12)
    ; Calculate length
    sub.q   r8, r12, r11
    pop     r15
    pop     r14
    rts

; ============================================================================
; Keyword Table (flat format)
; ============================================================================
;
; Format: Each entry is the FULL keyword as null-terminated uppercase ASCII,
; immediately followed by the token byte. The table ends with a 0x00 byte
; where a keyword would start.
;
; Keywords are sorted longest-first within the table to ensure greedy
; matching (e.g. RESTORE before REM, GOSUB before GOTO, etc.).
;
; The tokeniser scans this table linearly for each non-quoted, non-DATA byte.

tok_kw_table:
    ; Operators (single/double character)
    dc.b    "<<", 0, TK_LSHIFT
    dc.b    ">>", 0, TK_RSHIFT
    dc.b    "+", 0, TK_PLUS
    dc.b    "-", 0, TK_MINUS
    dc.b    "*", 0, TK_MULT
    dc.b    "/", 0, TK_DIV
    dc.b    "^", 0, TK_POWER
    dc.b    "=", 0, TK_EQUAL
    dc.b    ">", 0, TK_GT
    dc.b    "<", 0, TK_LT
    dc.b    "?", 0, TK_PRINT

    ; Multi-char keywords (longest first for greedy matching)
    dc.b    "RESTORE", 0, TK_RESTORE
    dc.b    "RETURN", 0, TK_RETURN
    dc.b    "VARPTR", 0, TK_VPTR
    dc.b    "UCASE$", 0, TK_UCASES
    dc.b    "LCASE$", 0, TK_LCASES
    dc.b    "RIGHT$", 0, TK_RIGHTS
    dc.b    "BITTST", 0, TK_BITTST
    dc.b    "BITSET", 0, TK_BITSET
    dc.b    "BITCLR", 0, TK_BITCLR
    dc.b    "CLEAR", 0, TK_CLEAR
    dc.b    "TROFF", 0, TK_DEF
    dc.b    "GOSUB", 0, TK_GOSUB
    dc.b    "INPUT", 0, TK_INPUT
    dc.b    "PRINT", 0, TK_PRINT
    dc.b    "UNTIL", 0, TK_UNTIL
    dc.b    "WHILE", 0, TK_WHILE
    dc.b    "BLOAD", 0, TK_WIDTH
    dc.b    "TWOPI", 0, TK_TWOPI
    dc.b    "LEFT$", 0, TK_LEFTS
    dc.b    "CALL", 0, TK_CALL
    dc.b    "CHR$", 0, TK_CHRS
    dc.b    "CONT", 0, TK_CONT
    dc.b    "DATA", 0, TK_DATA
    dc.b    "DEEK", 0, TK_DEEK
    dc.b    "DOKE", 0, TK_DOKE
    dc.b    "ELSE", 0, TK_THEN    ; Note: ELSE handled differently in full interpreter
    dc.b    "GOTO", 0, TK_GOTO
    dc.b    "HEX$", 0, TK_HEXS
    dc.b    "BIN$", 0, TK_BINS
    dc.b    "LEEK", 0, TK_LEEK
    dc.b    "LIST", 0, TK_LIST
    dc.b    "LOAD", 0, TK_LOAD
    dc.b    "LOKE", 0, TK_LOKE
    dc.b    "LOOP", 0, TK_LOOP
    dc.b    "NEXT", 0, TK_NEXT
    dc.b    "TRON", 0, TK_NULL
    dc.b    "PEEK", 0, TK_PEEK
    dc.b    "POKE", 0, TK_POKE
    dc.b    "READ", 0, TK_READ
    dc.b    "SADD", 0, TK_SADD
    dc.b    "SAVE", 0, TK_SAVE
    dc.b    "STEP", 0, TK_STEP
    dc.b    "STOP", 0, TK_STOP
    dc.b    "STR$", 0, TK_STRS
    dc.b    "SWAP", 0, TK_SWAP
    dc.b    "THEN", 0, TK_THEN
    dc.b    "WAIT", 0, TK_WAIT
    dc.b    "MID$", 0, TK_MIDS
    dc.b    "ABS", 0, TK_ABS
    dc.b    "AND", 0, TK_AND
    dc.b    "ASC", 0, TK_ASC
    dc.b    "ATN", 0, TK_ATN
    dc.b    "COS", 0, TK_COS
    dc.b    "DEC", 0, TK_DEC
    dc.b    "DEF", 0, TK_DEF
    dc.b    "DIM", 0, TK_DIM
    dc.b    "END", 0, TK_END
    dc.b    "EOR", 0, TK_EOR
    dc.b    "EXP", 0, TK_EXP
    dc.b    "FOR", 0, TK_FOR
    dc.b    "FRE", 0, TK_FRE
    dc.b    "GET", 0, TK_GET
    dc.b    "INC", 0, TK_INC
    dc.b    "INT", 0, TK_INT
    dc.b    "LEN", 0, TK_LEN
    dc.b    "LET", 0, TK_LET
    dc.b    "LOG", 0, TK_LOG
    dc.b    "MAX", 0, TK_MAX
    dc.b    "MIN", 0, TK_MIN
    dc.b    "NEW", 0, TK_NEW
    dc.b    "NOT", 0, TK_NOT
    dc.b    "POS", 0, TK_POS
    dc.b    "REM", 0, TK_REM
    dc.b    "RND", 0, TK_RND
    dc.b    "RUN", 0, TK_RUN
    dc.b    "SGN", 0, TK_SGN
    dc.b    "SIN", 0, TK_SIN
    dc.b    "SPC", 0, TK_SPC
    dc.b    "SQR", 0, TK_SQR
    dc.b    "TAB", 0, TK_TAB
    dc.b    "TAN", 0, TK_TAN
    dc.b    "WEND", 0, TK_UNTIL
    dc.b    "USR", 0, TK_USR
    dc.b    "VAL", 0, TK_VAL
    dc.b    "DO", 0, TK_DO
    dc.b    "FN", 0, TK_FN
    dc.b    "IF", 0, TK_IF
    dc.b    "ON", 0, TK_ON
    dc.b    "OR", 0, TK_OR
    dc.b    "PI", 0, TK_PI
    dc.b    "TO", 0, TK_TO

    ; Hardware extension keywords (longest first)
    dc.b    "TRIANGLE", 0, TK_TRIANGLE
    ; NOTE: TRICOLOR is NOT tokenized — left as literal chars so hw_voodoo
    ; dispatch can match on 'T'. Word boundary check prevents COLOR from
    ; matching inside TRICOLOR since 'I' precedes it.
    dc.b    "ENVELOPE", 0, TK_ENVELOPE
    dc.b    "ZBUFFER", 0, TK_ZBUFFER
    dc.b    "TEXTURE", 0, TK_TEXTURE
    dc.b    "PALETTE", 0, TK_PALETTE
    dc.b    "SCREEN", 0, TK_SCREEN
    dc.b    "SCROLL", 0, TK_SCROLL_CMD
    dc.b    "COPPER", 0, TK_COPPER
    dc.b    "CIRCLE", 0, TK_CIRCLE
    dc.b    "VERTEX", 0, TK_VERTEX
    dc.b    "VOODOO", 0, TK_VOODOO
    dc.b    "LOCATE", 0, TK_LOCATE
    dc.b    "POKEY", 0, TK_POKEY_CMD
    dc.b    "ANTIC", 0, TK_ANTIC
    dc.b    "SOUND", 0, TK_SOUND
    dc.b    "COLOR", 0, TK_COLOR
    dc.b    "VSYNC", 0, TK_VSYNC_CMD
    dc.b    "PLOT", 0, TK_PLOT
    dc.b    "LINE", 0, TK_LINE_CMD
    dc.b    "BLIT", 0, TK_BLIT
    dc.b    "GATE", 0, TK_GATE
    dc.b    "GTIA", 0, TK_GTIA
    dc.b    "CLS", 0, TK_CLS
    dc.b    "BOX", 0, TK_BOX
    dc.b    "ULA", 0, TK_ULA
    dc.b    "TED", 0, TK_TED_CMD
    dc.b    "PSG", 0, TK_PSG_CMD
    dc.b    "SID", 0, TK_SID_CMD
    dc.b    "AHX", 0, TK_AHX
    dc.b    "SAP", 0, TK_SAP

    ; End of table marker
    dc.b    0

    align 8
tok_kw_table_end:

; ============================================================================
; EOF
; ============================================================================
