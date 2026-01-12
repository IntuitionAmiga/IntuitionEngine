; m68k_test_suite.asm - Systematic test suite for 68000-68060 CPUs
; Minimizes memory accesses and uses direct terminal output

TERM_OUT    equ $F900         ; Terminal output register

    ORG     $1000

start:
    ; Initialize stack
    move.l  #$8000,sp

    ; Print banner
    lea     banner_msg,a0
    bsr     print_string

    ; Initialize test counters
    moveq   #0,d7             ; Total tests
    moveq   #0,d6             ; Passed tests

    ; Execute each test group
    bsr     test_68000_core_instructions
    bsr     test_68000_addressing_modes
    bsr     test_68020_extensions

    ; Print summary
    lea     summary_pre_msg,a0
    bsr     print_string

    move.l  d6,d0             ; Passed tests
    bsr     print_decimal

    lea     summary_mid_msg,a0
    bsr     print_string

    move.l  d7,d0             ; Total tests
    bsr     print_decimal

    lea     summary_post_msg,a0
    bsr     print_string

    stop    #$2700

;-------------------------------------------------------------------------------
; Safe terminal output functions
;-------------------------------------------------------------------------------
; Print character in d0
print_char:
    movem.l d0-d1/a0-a1,-(sp)
    move.b  d0,TERM_OUT
    movem.l (sp)+,d0-d1/a0-a1
    rts

; Print newline
print_newline:
    movem.l d0/a0,-(sp)
    move.b  #13,d0
    bsr     print_char
    move.b  #10,d0
    bsr     print_char
    movem.l (sp)+,d0/a0
    rts

; Print string in a0
print_string:
    movem.l d0-d1/a0-a1,-(sp)
    move.l  a0,a1
.loop:
    move.b  (a1)+,d0
    beq     .done
    bsr     print_char
    bra     .loop
.done:
    movem.l (sp)+,d0-d1/a0-a1
    rts

; Print decimal in d0
print_decimal:
    movem.l d0-d2/a0-a1,-(sp)

    ; Handle zero specially
    tst.l   d0
    bne     .not_zero

    move.b  #'0',d0
    bsr     print_char
    bra     .done

.not_zero:
    ; Build digits in reverse on stack
    moveq   #0,d2             ; Digit count
    move.l  d0,d1             ; Working copy

.loop:
    divul   #10,d1:d1         ; Divide by 10: quotient in d1, remainder in d1 high word
    swap    d1                ; Get remainder
    add.b   #'0',d1           ; Convert to ASCII
    move.b  d1,-(sp)          ; Push digit
    addq.l  #1,d2             ; Count digit
    swap    d1                ; Back to quotient
    tst.l   d1                ; Check if done
    bne     .loop

    ; Print digits from stack
.print_loop:
    move.b  (sp)+,d0          ; Pop digit
    bsr     print_char
    subq.l  #1,d2
    bne     .print_loop

.done:
    movem.l (sp)+,d0-d2/a0-a1
    rts

; Record test result
; Input: a0 = test name, d0 = result (0=pass, nonzero=fail)
record_test:
    ; Increment counters
    addq.l  #1,d7             ; Total tests
    tst.l   d0
    bne     .failed
    addq.l  #1,d6             ; Passed tests
.failed:

    ; Print test name
    bsr     print_string

    ; Print separator
    lea     dots_msg,a0
    bsr     print_string

    ; Print result
    tst.l   d0
    bne     .print_fail

    lea     pass_msg,a0
    bra     .print_result

.print_fail:
    lea     fail_msg,a0

.print_result:
    bsr     print_string
    bsr     print_newline
    rts

;-------------------------------------------------------------------------------
; 68000 Core Instruction Tests
;-------------------------------------------------------------------------------
test_68000_core_instructions:
    lea     header_68000_msg,a0
    bsr     print_string

    ; Test basic register data movement
    lea     move_b_msg,a0
    moveq   #0,d0

    ; Test MOVE.B
    move.b  #$12,d1
    move.b  d1,d2
    cmp.b   d1,d2
    beq     .move_b_pass
    moveq   #1,d0
.move_b_pass:
    bsr     record_test

    ; Test MOVE.W
    lea     move_w_msg,a0
    moveq   #0,d0

    move.w  #$1234,d1
    move.w  d1,d2
    cmp.w   d1,d2
    beq     .move_w_pass
    moveq   #1,d0
.move_w_pass:
    bsr     record_test

    ; Test MOVE.L
    lea     move_l_msg,a0
    moveq   #0,d0

    move.l  #$12345678,d1
    move.l  d1,d2
    cmp.l   d1,d2
    beq     .move_l_pass
    moveq   #1,d0
.move_l_pass:
    bsr     record_test

    ; Test MOVEQ
    lea     moveq_msg,a0
    moveq   #0,d0

    moveq   #127,d1
    cmp.l   #127,d1
    bne     .moveq_fail

    moveq   #-128,d1
    cmp.l   #-128,d1
    beq     .moveq_pass

.moveq_fail:
    moveq   #1,d0
.moveq_pass:
    bsr     record_test

    ; Test ADD
    lea     add_msg,a0
    moveq   #0,d0

    move.l  #$100,d1
    move.l  #$200,d2
    add.l   d1,d2
    cmp.l   #$300,d2
    beq     .add_pass
    moveq   #1,d0
.add_pass:
    bsr     record_test

    ; Test SUB
    lea     sub_msg,a0
    moveq   #0,d0

    move.l  #$300,d1
    move.l  #$100,d2
    sub.l   d2,d1
    cmp.l   #$200,d1
    beq     .sub_pass
    moveq   #1,d0
.sub_pass:
    bsr     record_test

    ; Test AND
    lea     and_msg,a0
    moveq   #0,d0

    move.l  #$AAAA5555,d1
    move.l  #$5555AAAA,d2
    and.l   d2,d1
    cmp.l   #$00000000,d1
    beq     .and_pass
    moveq   #1,d0
.and_pass:
    bsr     record_test

    ; Test OR
    lea     or_msg,a0
    moveq   #0,d0

    move.l  #$AAAA0000,d1
    move.l  #$00005555,d2
    or.l    d2,d1
    cmp.l   #$AAAA5555,d1
    beq     .or_pass
    moveq   #1,d0
.or_pass:
    bsr     record_test

    ; Test LSL
    lea     lsl_msg,a0
    moveq   #0,d0

    move.l  #$12345678,d1
    lsl.l   #4,d1
    cmp.l   #$23456780,d1
    beq     .lsl_pass
    moveq   #1,d0
.lsl_pass:
    bsr     record_test

    ; Test LSR
    lea     lsr_msg,a0
    moveq   #0,d0

    move.l  #$12345678,d1
    lsr.l   #4,d1
    cmp.l   #$01234567,d1
    beq     .lsr_pass
    moveq   #1,d0
.lsr_pass:
    bsr     record_test

    ; Test more instructions here...

    rts

;-------------------------------------------------------------------------------
; 68000 Addressing Mode Tests
;-------------------------------------------------------------------------------
test_68000_addressing_modes:
    lea     header_addr_msg,a0
    bsr     print_string

    ; Test register direct
    lea     addr_reg_direct_msg,a0
    moveq   #0,d0

    move.l  #$12345678,d1
    move.l  d1,d2
    cmp.l   #$12345678,d2
    beq     .reg_direct_pass
    moveq   #1,d0
.reg_direct_pass:
    bsr     record_test

    ; Test address register indirect
    lea     addr_reg_indirect_msg,a0
    moveq   #0,d0

    lea     test_data,a0
    move.l  (a0),d1
    cmp.l   #$ABCDEF01,d1
    beq     .reg_indirect_pass
    moveq   #1,d0
.reg_indirect_pass:
    bsr     record_test

    ; Test address register with postincrement
    lea     addr_postinc_msg,a0
    moveq   #0,d0

    lea     test_data,a0
    move.l  (a0)+,d1
    cmp.l   #$ABCDEF01,d1
    bne     .postinc_fail

    cmp.l   #test_data+4,a0
    beq     .postinc_pass

.postinc_fail:
    moveq   #1,d0
.postinc_pass:
    bsr     record_test

    ; Test more addressing modes...

    rts

;-------------------------------------------------------------------------------
; 68020 Extension Tests
;-------------------------------------------------------------------------------
test_68020_extensions:
    lea     header_68020_msg,a0
    bsr     print_string

    ; Test CHK2 (if available)
    lea     chk2_msg,a0
    moveq   #0,d0

    ; Skip for now - not testing 68020 specifics yet
    moveq   #1,d0  ; Skip
    bsr     record_test

    ; Test other 68020 features...

    rts

;-------------------------------------------------------------------------------
; Data section
;-------------------------------------------------------------------------------
    ORG     $4000

; Test data
test_data:    dc.l    $ABCDEF01, $23456789
test_buffer:  ds.l    16

; Messages
banner_msg:      dc.b    "M68K CPU Test Suite",13,10
                 dc.b    "---------------------",13,10,0

summary_pre_msg: dc.b    13,10,"Tests passed: ",0
summary_mid_msg: dc.b    " of ",0
summary_post_msg:dc.b    " total tests",13,10,0

dots_msg:        dc.b    " .......... ",0
pass_msg:        dc.b    "PASS",0
fail_msg:        dc.b    "FAIL",0

; Headers
header_68000_msg:dc.b    13,10,"68000 CORE INSTRUCTION TESTS:",13,10,0
header_addr_msg: dc.b    13,10,"68000 ADDRESSING MODE TESTS:",13,10,0
header_68020_msg:dc.b    13,10,"68020 INSTRUCTION TESTS:",13,10,0

; Test messages
move_b_msg:      dc.b    "MOVE.B instruction",0
move_w_msg:      dc.b    "MOVE.W instruction",0
move_l_msg:      dc.b    "MOVE.L instruction",0
moveq_msg:       dc.b    "MOVEQ instruction",0
add_msg:         dc.b    "ADD instruction",0
sub_msg:         dc.b    "SUB instruction",0
and_msg:         dc.b    "AND instruction",0
or_msg:          dc.b    "OR instruction",0
lsl_msg:         dc.b    "LSL instruction",0
lsr_msg:         dc.b    "LSR instruction",0

addr_reg_direct_msg:   dc.b    "Register direct addressing",0
addr_reg_indirect_msg: dc.b    "Register indirect addressing",0
addr_postinc_msg:      dc.b    "Register indirect with postincrement",0

chk2_msg:        dc.b    "CHK2 instruction (68020)",0