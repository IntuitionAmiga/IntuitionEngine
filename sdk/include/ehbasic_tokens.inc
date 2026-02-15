; ehbasic_tokens.inc - EhBASIC IE64 Token Definitions
;
; Token values and keyword tables for the EhBASIC tokeniser.
; Ported from ehbasic68k.asm token tables.
;
; Token values $80-$E1 map 1:1 with the original EhBASIC 68K.
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; Token Constants ($80-$E1)
; ============================================================================

TK_END        equ 0x80
TK_FOR        equ 0x81
TK_NEXT       equ 0x82
TK_DATA       equ 0x83
TK_INPUT      equ 0x84
TK_DIM        equ 0x85
TK_READ       equ 0x86
TK_LET        equ 0x87
TK_DEC        equ 0x88
TK_GOTO       equ 0x89
TK_RUN        equ 0x8A
TK_IF         equ 0x8B
TK_RESTORE    equ 0x8C
TK_GOSUB      equ 0x8D
TK_RETURN     equ 0x8E
TK_REM        equ 0x8F
TK_STOP       equ 0x90
TK_ON         equ 0x91
TK_NULL       equ 0x92
TK_INC        equ 0x93
TK_WAIT       equ 0x94
TK_LOAD       equ 0x95
TK_SAVE       equ 0x96
TK_DEF        equ 0x97
TK_POKE       equ 0x98
TK_DOKE       equ 0x99
TK_LOKE       equ 0x9A
TK_CALL       equ 0x9B
TK_DO         equ 0x9C
TK_LOOP       equ 0x9D
TK_PRINT      equ 0x9E
TK_CONT       equ 0x9F
TK_LIST       equ 0xA0
TK_CLEAR      equ 0xA1
TK_NEW        equ 0xA2
TK_WIDTH      equ 0xA3
TK_GET        equ 0xA4
TK_SWAP       equ 0xA5
TK_BITSET     equ 0xA6
TK_BITCLR     equ 0xA7
TK_TAB        equ 0xA8
TK_TO         equ 0xA9
TK_FN         equ 0xAA
TK_SPC        equ 0xAB
TK_THEN       equ 0xAC
TK_NOT        equ 0xAD
TK_STEP       equ 0xAE
TK_UNTIL      equ 0xAF
TK_WHILE      equ 0xB0
TK_PLUS       equ 0xB1
TK_MINUS      equ 0xB2
TK_MULT       equ 0xB3
TK_DIV        equ 0xB4
TK_POWER      equ 0xB5
TK_AND        equ 0xB6
TK_EOR        equ 0xB7
TK_OR         equ 0xB8
TK_RSHIFT     equ 0xB9
TK_LSHIFT     equ 0xBA
TK_GT         equ 0xBB
TK_EQUAL      equ 0xBC
TK_LT         equ 0xBD
TK_SGN        equ 0xBE
TK_INT        equ 0xBF
TK_ABS        equ 0xC0
TK_USR        equ 0xC1
TK_FRE        equ 0xC2
TK_POS        equ 0xC3
TK_SQR        equ 0xC4
TK_RND        equ 0xC5
TK_LOG        equ 0xC6
TK_EXP        equ 0xC7
TK_COS        equ 0xC8
TK_SIN        equ 0xC9
TK_TAN        equ 0xCA
TK_ATN        equ 0xCB
TK_PEEK       equ 0xCC
TK_DEEK       equ 0xCD
TK_LEEK       equ 0xCE
TK_SADD       equ 0xCF
TK_LEN        equ 0xD0
TK_STRS       equ 0xD1
TK_VAL        equ 0xD2
TK_ASC        equ 0xD3
TK_UCASES     equ 0xD4
TK_LCASES     equ 0xD5
TK_CHRS       equ 0xD6
TK_HEXS       equ 0xD7
TK_BINS       equ 0xD8
TK_BITTST     equ 0xD9
TK_MAX        equ 0xDA
TK_MIN        equ 0xDB
TK_PI         equ 0xDC
TK_TWOPI      equ 0xDD
TK_VPTR       equ 0xDE
TK_LEFTS      equ 0xDF
TK_RIGHTS     equ 0xE0
TK_MIDS       equ 0xE1

; ============================================================================
; Hardware Extension Tokens ($E2-$FF)
; ============================================================================

; VGA video commands
TK_SCREEN     equ 0xE2
TK_CLS        equ 0xE3
TK_PLOT       equ 0xE4
TK_PALETTE    equ 0xE5
TK_VSYNC_CMD  equ 0xE6
TK_LOCATE     equ 0xE7
TK_COLOR      equ 0xE8
TK_LINE_CMD   equ 0xE9
TK_CIRCLE     equ 0xEA
TK_BOX        equ 0xEB
TK_SCROLL_CMD equ 0xEC

; Copper/blitter commands
TK_COPPER     equ 0xED
TK_BLIT       equ 0xEE

; Sound commands
TK_SOUND      equ 0xEF
TK_ENVELOPE   equ 0xF0
TK_GATE       equ 0xF1

; Chip-specific commands
TK_ULA        equ 0xF2
TK_TED_CMD    equ 0xF3
TK_ANTIC      equ 0xF4
TK_GTIA       equ 0xF5
TK_VOODOO     equ 0xF6
TK_PSG_CMD    equ 0xF7
TK_SID_CMD    equ 0xF8
TK_POKEY_CMD  equ 0xF9
TK_AHX        equ 0xFA
TK_SAP        equ 0xFB

; Voodoo sub-commands
TK_ZBUFFER    equ 0xFC
TK_VERTEX     equ 0xFD
TK_TRIANGLE   equ 0xFE
TK_TEXTURE    equ 0xFF

; ============================================================================
; EOF
; ============================================================================
