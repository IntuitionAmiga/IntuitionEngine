; 68k Opcode Test Suite for IntuitionEngine
; Tests each opcode in each supported addressing mode
; Reports PASS or FAIL for each test with detailed register values

        ORG     $1000

START:
        ; Initialize terminal output
        MOVE.W  #$F900,A1       ; Terminal output address

        ; Print header
        LEA     header_msg,A0
        JSR     PRINT_STRING

        ; Run test groups
        JSR     TEST_MOVE       ; Test MOVE instructions
        JSR     TEST_MOVEQ      ; Test MOVEQ instructions
        JSR     TEST_ADD        ; Test ADD instructions
        JSR     TEST_SUB        ; Test SUB instructions
        JSR     TEST_CMP        ; Test CMP instructions
        JSR     TEST_AND        ; Test AND instructions
        JSR     TEST_OR         ; Test OR instructions
        JSR     TEST_EOR        ; Test EOR instructions
        JSR     TEST_NOT        ; Test NOT instruction
        JSR     TEST_NEG        ; Test NEG instruction
        JSR     TEST_SHIFT      ; Test shift/rotate instructions
        JSR     TEST_BIT        ; Test bit manipulation instructions

        ; End of tests
        LEA     all_done_msg,A0
        JSR     PRINT_STRING
        STOP    #0              ; Halt execution

;------------------------------------------------------------
; Utility functions
;------------------------------------------------------------

; Print string utility (null-terminated)
PRINT_STRING:
        MOVE.B  (A0)+,D0        ; Get character
        BEQ     .done           ; If zero (null), we're done
        MOVE.B  D0,(A1)         ; Output character
        BRA     PRINT_STRING    ; Repeat
.done:
        RTS

; Print HEX digit in D0
PRINT_HEX_DIGIT:
        AND.B   #$0F,D0         ; Mask to just one hex digit
        CMP.B   #10,D0          ; Is it A-F?
        BLT     .print_09       ; If 0-9, skip conversion
        ADD.B   #'A'-10,D0      ; Convert to A-F ASCII
        BRA     .out_hex
.print_09:
        ADD.B   #'0',D0         ; Convert to 0-9 ASCII
.out_hex:
        MOVE.B  D0,(A1)         ; Output character
        RTS

; Print hex byte in D0
PRINT_HEX_BYTE:
        MOVE.B  D0,-(SP)        ; Save D0
        LSR.B   #4,D0           ; Get high nibble
        JSR     PRINT_HEX_DIGIT
        MOVE.B  (SP)+,D0        ; Restore D0 and get low nibble
        JSR     PRINT_HEX_DIGIT
        RTS

; Print hex word in D0
PRINT_HEX_WORD:
        MOVE.W  D0,-(SP)        ; Save D0
        LSR.W   #8,D0           ; Get high byte
        JSR     PRINT_HEX_BYTE
        MOVE.W  (SP)+,D0        ; Restore D0 and get low byte
        JSR     PRINT_HEX_BYTE
        RTS

; Print hex long in D0
PRINT_HEX_LONG:
        MOVE.L  D0,-(SP)        ; Save D0 on stack
        SWAP    D0              ; Get high word
        JSR     PRINT_HEX_WORD
        MOVE.L  (SP),D0         ; Restore original D0 to get low word
        SWAP    D0              ; Get low word
        JSR     PRINT_HEX_WORD
        MOVE.L  (SP)+,D0        ; Restore D0 completely
        RTS

; Print PASS message
PRINT_PASS:
        LEA     pass_msg,A0
        JSR     PRINT_STRING
        LEA     separator_line,A0
        JSR     PRINT_STRING
        RTS

; Print FAIL message
PRINT_FAIL:
        LEA     fail_msg,A0
        JSR     PRINT_STRING
        LEA     separator_line,A0
        JSR     PRINT_STRING
        RTS

; Print test header with opcode
; D7 = Opcode value, A6 = Test name, A5 = Mode description
PRINT_TEST_HEADER:
        ; Print test name
        LEA     test_prefix,A0
        JSR     PRINT_STRING
        MOVE.L  A6,A0
        JSR     PRINT_STRING

        ; Print opcode
        LEA     opcode_msg,A0
        JSR     PRINT_STRING
        MOVE.W  D7,D0
        JSR     PRINT_HEX_WORD

        ; Print mode description
        LEA     mode_prefix,A0
        JSR     PRINT_STRING
        MOVE.L  A5,A0
        JSR     PRINT_STRING
        LEA     newline,A0
        JSR     PRINT_STRING

        RTS

; Print register values "Before:" line
; Used registers are D1-D5
PRINT_BEFORE_STATE:
        LEA     before_msg,A0
        JSR     PRINT_STRING

        ; Print register values as needed
        ; Determine which registers to print based on test case
        ; D1 = Test case flags for which registers to print
        ; D2-D5 hold the register values to print

        TST.B   D1
        BEQ     .done           ; If no registers to print, we're done

        ; Check if D0 should be printed (bit 0)
        BTST    #0,D1
        BEQ     .check_d1

        ; Print D0
        LEA     d0_equals,A0
        JSR     PRINT_STRING
        MOVE.L  D2,D0
        JSR     PRINT_HEX_LONG

        ; Check if comma needed
        MOVE.B  D1,D0
        AND.B   #$FE,D0         ; Mask out bit 0
        BEQ     .check_d1       ; No more registers
        LEA     comma_msg,A0
        JSR     PRINT_STRING

.check_d1:
        ; Check if D1 should be printed (bit 1)
        BTST    #1,D1
        BEQ     .check_a0

        ; Print D1
        LEA     d1_equals,A0
        JSR     PRINT_STRING
        MOVE.L  D3,D0
        JSR     PRINT_HEX_LONG

        ; Check if comma needed
        MOVE.B  D1,D0
        AND.B   #$FC,D0         ; Mask out bits 0-1
        BEQ     .check_a0       ; No more registers
        LEA     comma_msg,A0
        JSR     PRINT_STRING

.check_a0:
        ; Check if A0 should be printed (bit 2)
        BTST    #2,D1
        BEQ     .check_memory

        ; Print A0
        LEA     a0_equals,A0
        JSR     PRINT_STRING
        MOVE.L  D4,D0
        JSR     PRINT_HEX_LONG

        ; Check if comma needed
        MOVE.B  D1,D0
        AND.B   #$F8,D0         ; Mask out bits 0-2
        BEQ     .check_memory   ; No more registers
        LEA     comma_msg,A0
        JSR     PRINT_STRING

.check_memory:
        ; Check if memory should be printed (bit 3)
        BTST    #3,D1
        BEQ     .done

        ; Print memory at A0
        LEA     mem_a0_equals,A0
        JSR     PRINT_STRING
        MOVE.L  D5,D0
        JSR     PRINT_HEX_LONG

.done:
        LEA     newline,A0
        JSR     PRINT_STRING
        RTS

; Print register values "After:" line
; Used registers are D1-D5
PRINT_AFTER_STATE:
        LEA     after_msg,A0
        JSR     PRINT_STRING

        ; Print register values as needed
        ; Same logic as PRINT_BEFORE_STATE
        TST.B   D1
        BEQ     .done           ; If no registers to print, we're done

        ; Check if D0 should be printed (bit 0)
        BTST    #0,D1
        BEQ     .check_d1

        ; Print D0
        LEA     d0_equals,A0
        JSR     PRINT_STRING
        MOVE.L  D2,D0
        JSR     PRINT_HEX_LONG

        ; Check if comma needed
        MOVE.B  D1,D0
        AND.B   #$FE,D0         ; Mask out bit 0
        BEQ     .check_d1       ; No more registers
        LEA     comma_msg,A0
        JSR     PRINT_STRING

.check_d1:
        ; Check if D1 should be printed (bit 1)
        BTST    #1,D1
        BEQ     .check_a0

        ; Print D1
        LEA     d1_equals,A0
        JSR     PRINT_STRING
        MOVE.L  D3,D0
        JSR     PRINT_HEX_LONG

        ; Check if comma needed
        MOVE.B  D1,D0
        AND.B   #$FC,D0         ; Mask out bits 0-1
        BEQ     .check_a0       ; No more registers
        LEA     comma_msg,A0
        JSR     PRINT_STRING

.check_a0:
        ; Check if A0 should be printed (bit 2)
        BTST    #2,D1
        BEQ     .check_memory

        ; Print A0
        LEA     a0_equals,A0
        JSR     PRINT_STRING
        MOVE.L  D4,D0
        JSR     PRINT_HEX_LONG

        ; Check if comma needed
        MOVE.B  D1,D0
        AND.B   #$F8,D0         ; Mask out bits 0-2
        BEQ     .check_memory   ; No more registers
        LEA     comma_msg,A0
        JSR     PRINT_STRING

.check_memory:
        ; Check if memory should be printed (bit 3)
        BTST    #3,D1
        BEQ     .done

        ; Print memory at A0
        LEA     mem_a0_equals,A0
        JSR     PRINT_STRING
        MOVE.L  D5,D0
        JSR     PRINT_HEX_LONG

.done:
        LEA     newline,A0
        JSR     PRINT_STRING
        RTS

; Print flags states
PRINT_FLAGS:
        LEA     flags_msg,A0
        JSR     PRINT_STRING

        ; N flag
        LEA     n_equals,A0
        JSR     PRINT_STRING
        MOVE.W  D0,-(SP)       ; Save D0
        MOVE.W  SR,D0
        BTST    #3,D0          ; Test N flag (bit 3)
        SNE     D0             ; Set D0 to $FF if N=1, else $00
        AND.B   #1,D0          ; Convert to 0 or 1
        ADD.B   #'0',D0        ; Convert to ASCII
        MOVE.B  D0,(A1)        ; Output
        MOVE.W  (SP)+,D0       ; Restore D0

        ; Z flag
        LEA     z_equals,A0
        JSR     PRINT_STRING
        MOVE.W  D0,-(SP)       ; Save D0
        MOVE.W  SR,D0
        BTST    #2,D0          ; Test Z flag (bit 2)
        SNE     D0             ; Set D0 to $FF if Z=1, else $00
        AND.B   #1,D0          ; Convert to 0 or 1
        ADD.B   #'0',D0        ; Convert to ASCII
        MOVE.B  D0,(A1)        ; Output
        MOVE.W  (SP)+,D0       ; Restore D0

        ; V flag
        LEA     v_equals,A0
        JSR     PRINT_STRING
        MOVE.W  D0,-(SP)       ; Save D0
        MOVE.W  SR,D0
        BTST    #1,D0          ; Test V flag (bit 1)
        SNE     D0             ; Set D0 to $FF if V=1, else $00
        AND.B   #1,D0          ; Convert to 0 or 1
        ADD.B   #'0',D0        ; Convert to ASCII
        MOVE.B  D0,(A1)        ; Output
        MOVE.W  (SP)+,D0       ; Restore D0

        ; C flag
        LEA     c_equals,A0
        JSR     PRINT_STRING
        MOVE.W  D0,-(SP)       ; Save D0
        MOVE.W  SR,D0
        BTST    #0,D0          ; Test C flag (bit 0)
        SNE     D0             ; Set D0 to $FF if C=1, else $00
        AND.B   #1,D0          ; Convert to 0 or 1
        ADD.B   #'0',D0        ; Convert to ASCII
        MOVE.B  D0,(A1)        ; Output
        MOVE.W  (SP)+,D0       ; Restore D0

        LEA     newline,A0
        JSR     PRINT_STRING
        RTS

; Print expected results
; D1 = flags for which registers to check, D2-D5 = expected values
PRINT_EXPECTED:
        LEA     expected_msg,A0
        JSR     PRINT_STRING

        ; Only print expectations for registers that are checked (same bits as PRINT_BEFORE)
        TST.B   D1
        BEQ     .done           ; If no registers to check, we're done

        ; Check if D0 should be printed (bit 0)
        BTST    #0,D1
        BEQ     .check_d1

        ; Print D0
        LEA     d0_equals,A0
        JSR     PRINT_STRING
        MOVE.L  D2,D0
        JSR     PRINT_HEX_LONG

        ; Check if comma needed
        MOVE.B  D1,D0
        AND.B   #$FE,D0         ; Mask out bit 0
        BEQ     .check_d1       ; No more registers
        LEA     comma_msg,A0
        JSR     PRINT_STRING

.check_d1:
        ; Check if D1 should be printed (bit 1)
        BTST    #1,D1
        BEQ     .check_a0

        ; Print D1
        LEA     d1_equals,A0
        JSR     PRINT_STRING
        MOVE.L  D3,D0
        JSR     PRINT_HEX_LONG

        ; Check if comma needed
        MOVE.B  D1,D0
        AND.B   #$FC,D0         ; Mask out bits 0-1
        BEQ     .check_a0       ; No more registers
        LEA     comma_msg,A0
        JSR     PRINT_STRING

.check_a0:
        ; Check if A0 should be printed (bit 2)
        BTST    #2,D1
        BEQ     .done

        ; Print A0
        LEA     a0_equals,A0
        JSR     PRINT_STRING
        MOVE.L  D4,D0
        JSR     PRINT_HEX_LONG

.done:
        LEA     newline,A0
        JSR     PRINT_STRING
        RTS

;------------------------------------------------------------
; Test MOVE instructions
;------------------------------------------------------------
TEST_MOVE:
        ; Print test header
        LEA     move_header,A0
        JSR     PRINT_STRING

        ;--------------------------------------------------------
        ; Test MOVE.B Dn,Dn (Data Register to Data Register)
        ;--------------------------------------------------------
        ; Set test description
        LEA     test_move_b_dn_dn,A6
        LEA     mode_dn_to_dn,A5
        MOVE.W  #$1200,D7       ; Opcode for MOVE.B D1,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$FFFFFFFF,D0   ; Initialize D0 with all 1s
        MOVE.L  #$000000AA,D1   ; Initialize D1 with test value

        ; Print before state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  D1,D3           ; D1 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        MOVE.B  D1,D0           ; Should set D0 to $FFFFFFAA

        ; Print after state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  #$000000AA,D3   ; D1 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$FFFFFFAA,D2   ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$FFFFFFAA,D0   ; Check if D0 has expected value
        BEQ     .pass1
        JSR     PRINT_FAIL
        BRA     .test2
.pass1:
        JSR     PRINT_PASS

        ;--------------------------------------------------------
        ; Test MOVE.W Dn,Dn (Data Register to Data Register)
        ;--------------------------------------------------------
.test2:
        ; Set test description
        LEA     test_move_w_dn_dn,A6
        LEA     mode_dn_to_dn,A5
        MOVE.W  #$3200,D7       ; Opcode for MOVE.W D1,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$FFFFFFFF,D0   ; Initialize D0 with all 1s
        MOVE.L  #$0000AAAA,D1   ; Initialize D1 with test value

        ; Print before state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  D1,D3           ; D1 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        MOVE.W  D1,D0           ; Should set D0 to $FFFFAAAA

        ; Print after state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  #$0000AAAA,D3   ; D1 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$FFFFAAAA,D2   ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$FFFFAAAA,D0   ; Check if D0 has expected value
        BEQ     .pass2
        JSR     PRINT_FAIL
        BRA     .test3
.pass2:
        JSR     PRINT_PASS

        ;--------------------------------------------------------
        ; Test MOVE.L Dn,Dn (Data Register to Data Register)
        ;--------------------------------------------------------
.test3:
        ; Set test description
        LEA     test_move_l_dn_dn,A6
        LEA     mode_dn_to_dn,A5
        MOVE.W  #$2200,D7       ; Opcode for MOVE.L D1,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$FFFFFFFF,D0   ; Initialize D0 with all 1s
        MOVE.L  #$AAAAAAAA,D1   ; Initialize D1 with test value

        ; Print before state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  D1,D3           ; D1 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        MOVE.L  D1,D0           ; Should set D0 to $AAAAAAAA

        ; Print after state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  #$AAAAAAAA,D3   ; D1 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$AAAAAAAA,D2   ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$AAAAAAAA,D0   ; Check if D0 has expected value
        BEQ     .pass3
        JSR     PRINT_FAIL
        BRA     .test4
.pass3:
        JSR     PRINT_PASS

        ;--------------------------------------------------------
        ; Test MOVE.L #imm,Dn (Immediate to Data Register)
        ;--------------------------------------------------------
.test4:
        ; Set test description
        LEA     test_move_l_imm_dn,A6
        LEA     mode_imm_to_dn,A5
        MOVE.W  #$203C,D7       ; Opcode for MOVE.L #imm,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$FFFFFFFF,D0   ; Initialize D0 with all 1s

        ; Print before state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        MOVE.L  #$12345678,D0   ; Should set D0 to $12345678

        ; Print after state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$12345678,D2   ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$12345678,D0   ; Check if D0 has expected value
        BEQ     .pass4
        JSR     PRINT_FAIL
        BRA     .test5
.pass4:
        JSR     PRINT_PASS

        ;--------------------------------------------------------
        ; Test MOVE.L Dn,(An) (Data Register to Memory)
        ;--------------------------------------------------------
.test5:
        ; Set test description
        LEA     test_move_l_dn_an_ind,A6
        LEA     mode_dn_to_an_ind,A5
        MOVE.W  #$2080,D7       ; Opcode for MOVE.L D0,(A0)
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$ABCDEF12,D0   ; Test data
        LEA     test_buffer,A0  ; Buffer address
        MOVE.L  #0,(A0)         ; Clear buffer

        ; Print before state
        MOVE.B  #$0D,D1         ; Print D0, A0, and MEM[A0] (bits 0, 2, 3)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  A0,D4           ; A0 value
        MOVE.L  (A0),D5         ; Memory at A0
        JSR     PRINT_BEFORE_STATE

        ; Execute
        MOVE.L  D0,(A0)         ; Move D0 to memory at (A0)

        ; Print after state
        MOVE.B  #$0D,D1         ; Print D0, A0, and MEM[A0] (bits 0, 2, 3)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  A0,D4           ; A0 value
        MOVE.L  (A0),D5         ; Memory at A0
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$08,D1         ; Check only MEM[A0] (bit 3)
        MOVE.L  #$ABCDEF12,D5   ; Expected memory value
        JSR     PRINT_EXPECTED

        ; Verify - load from memory and check
        MOVE.L  (A0),D1         ; Load from memory
        CMP.L   #$ABCDEF12,D1   ; Compare with expected
        BEQ     .pass5
        JSR     PRINT_FAIL
        BRA     .test6
.pass5:
        JSR     PRINT_PASS

        ;--------------------------------------------------------
        ; Test MOVE.L (An)+,Dn (Memory to Data Register with post-increment)
        ;--------------------------------------------------------
.test6:
        ; Set test description
        LEA     test_move_l_an_post_dn,A6
        LEA     mode_an_post_to_dn,A5
        MOVE.W  #$2018,D7       ; Opcode for MOVE.L (A0)+,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        LEA     test_buffer,A0          ; Buffer address
        MOVE.L  #$FEDCBA98,(A0)         ; Set test value
        MOVE.L  #$11111111,4(A0)        ; Set second test value
        MOVE.L  #0,D0                   ; Clear D0

        ; Print before state
        MOVE.B  #$0D,D1         ; Print D0, A0, and MEM[A0] (bits 0, 2, 3)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  A0,D4           ; A0 value
        MOVE.L  (A0),D5         ; Memory at A0
        JSR     PRINT_BEFORE_STATE

        ; Execute
        MOVE.L  (A0)+,D0        ; Move from (A0) to D0 and increment A0

        ; Print after state
        MOVE.B  #$0D,D1         ; Print D0, A0, and MEM[A0] (bits 0, 2, 3)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  A0,D4           ; A0 value
        MOVE.L  (A0),D5         ; Memory at A0 (should be second value)
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$05,D1         ; Check D0 and A0 (bits 0, 2)
        MOVE.L  #$FEDCBA98,D2   ; Expected D0 value
        LEA     test_buffer+4,A2 ; Expected A0 value
        MOVE.L  A2,D4           ; Copy to D4
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$FEDCBA98,D0   ; Check data value
        BNE     .fail6

        ; Check if A0 was incremented correctly
        LEA     test_buffer+4,A2 ; Expected A0 after increment
        CMP.L   A0,A2           ; Compare addresses
        BEQ     .pass6

.fail6:
        JSR     PRINT_FAIL
        BRA     .test7
.pass6:
        JSR     PRINT_PASS

        ;--------------------------------------------------------
        ; Test MOVE.L -(An),Dn (Memory to Data Register with pre-decrement)
        ;--------------------------------------------------------
.test7:
        ; Set test description
        LEA     test_move_l_an_pre_dn,A6
        LEA     mode_an_pre_to_dn,A5
        MOVE.W  #$2020,D7       ; Opcode for MOVE.L -(A0),D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        LEA     test_buffer+8,A0        ; Point to end of buffer +4
        MOVE.L  #$12345678,-4(A0)       ; Set test value at buffer+4
        MOVE.L  #0,D0                   ; Clear D0

        ; Print before state
        MOVE.B  #$0D,D1         ; Print D0, A0, and MEM[A0-4] (bits 0, 2, 3)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  A0,D4           ; A0 value
        MOVE.L  -4(A0),D5       ; Memory at A0-4
        JSR     PRINT_BEFORE_STATE

        ; Execute
        MOVE.L  -(A0),D0        ; Decrement A0 and move from (A0) to D0

        ; Print after state
        MOVE.B  #$0D,D1         ; Print D0, A0, and MEM[A0] (bits 0, 2, 3)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  A0,D4           ; A0 value (should be decremented)
        MOVE.L  (A0),D5         ; Memory at A0
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$05,D1         ; Check D0 and A0 (bits 0, 2)
        MOVE.L  #$12345678,D2   ; Expected D0 value
        LEA     test_buffer+4,A2 ; Expected A0 value after decrement
        MOVE.L  A2,D4           ; Copy to D4
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$12345678,D0   ; Check data value
        BNE     .fail7

        ; Check if A0 was decremented correctly
        LEA     test_buffer+4,A2 ; Expected A0 value after decrement
        CMP.L   A0,A2           ; Compare
        BEQ     .pass7

.fail7:
        JSR     PRINT_FAIL
        BRA     .test8
.pass7:
        JSR     PRINT_PASS

        ;--------------------------------------------------------
        ; Test MOVE.L d16(An),Dn (Memory to Data Register with displacement)
        ;--------------------------------------------------------
.test8:
        ; Set test description
        LEA     test_move_l_an_disp_dn,A6
        LEA     mode_an_disp_to_dn,A5
        MOVE.W  #$2028,D7       ; Opcode for MOVE.L d16(A0),D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        LEA     test_buffer,A0          ; Buffer base address
        MOVE.L  #$AABBCCDD,16(A0)       ; Set test value at offset 16
        MOVE.L  #0,D0                   ; Clear D0

        ; Print before state
        MOVE.B  #$05,D1         ; Print D0 and A0 (bits 0, 2)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  A0,D4           ; A0 value
        JSR     PRINT_BEFORE_STATE

        ; Also print the memory at A0+16
        LEA     mem_a0_plus16_equals,A0
        JSR     PRINT_STRING
        LEA     test_buffer,A0   ; Reset A0
        MOVE.L  16(A0),D0
        JSR     PRINT_HEX_LONG
        LEA     newline,A0
        JSR     PRINT_STRING

        ; Execute
        MOVE.L  16(A0),D0       ; Move from offset 16 to D0

        ; Print after state
        MOVE.B  #$05,D1         ; Print D0 and A0 (bits 0, 2)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  A0,D4           ; A0 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$AABBCCDD,D2   ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$AABBCCDD,D0   ; Check data value
        BEQ     .pass8
        JSR     PRINT_FAIL
        BRA     .test9
.pass8:
        JSR     PRINT_PASS

        ;--------------------------------------------------------
        ; Test MOVE.L addr.L,Dn (Absolute Long addressing to Data Register)
        ;--------------------------------------------------------
.test9:
        ; Set test description
        LEA     test_move_l_abs_dn,A6
        LEA     mode_abs_to_dn,A5
        MOVE.W  #$2039,D7       ; Opcode for MOVE.L addr.L,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$87654321,test_long_addr ; Set test value at absolute address
        MOVE.L  #0,D0                   ; Clear D0

        ; Print before state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_BEFORE_STATE

        ; Also print the memory at absolute address
        LEA     mem_abs_equals,A0
        JSR     PRINT_STRING
        LEA     test_long_addr_txt,A0
        JSR     PRINT_STRING
        LEA     equals_sign,A0
        JSR     PRINT_STRING
        MOVE.L  test_long_addr,D0
        JSR     PRINT_HEX_LONG
        LEA     newline,A0
        JSR     PRINT_STRING

        ; Execute
        MOVE.L  test_long_addr,D0 ; Move from absolute address to D0

        ; Print after state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$87654321,D2   ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$87654321,D0   ; Check data value
        BEQ     .pass9
        JSR     PRINT_FAIL
        BRA     .done
.pass9:
        JSR     PRINT_PASS

.done:
        RTS

;------------------------------------------------------------
; Test MOVEQ instructions
;------------------------------------------------------------
TEST_MOVEQ:
        ; Print test header
        LEA     moveq_header,A0
        JSR     PRINT_STRING

        ;--------------------------------------------------------
        ; Test MOVEQ #imm,Dn (Immediate data to data register - positive)
        ;--------------------------------------------------------
        ; Set test description
        LEA     test_moveq_pos,A6
        LEA     mode_imm_to_dn,A5
        MOVE.W  #$707F,D7       ; Opcode for MOVEQ #127,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$FFFFFFFF,D0   ; Initialize D0 with all 1s

        ; Print before state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        MOVEQ   #127,D0         ; Should set D0 to $0000007F (positive 8-bit)

        ; Print after state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$0000007F,D2   ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$0000007F,D0   ; Check if D0 has expected value
        BEQ     .pass1
        JSR     PRINT_FAIL
        BRA     .test2
.pass1:
        JSR     PRINT_PASS

        ;--------------------------------------------------------
        ; Test MOVEQ #imm,Dn (Immediate data to data register - negative)
        ;--------------------------------------------------------
.test2:
        ; Set test description
        LEA     test_moveq_neg,A6
        LEA     mode_imm_to_dn,A5
        MOVE.W  #$7080,D7       ; Opcode for MOVEQ #-128,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$FFFFFFFF,D0   ; Initialize D0 with all 1s

        ; Print before state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        MOVEQ   #-128,D0        ; Should set D0 to $FFFFFF80 (negative 8-bit)

        ; Print after state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$FFFFFF80,D2   ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$FFFFFF80,D0   ; Check if D0 has expected value
        BEQ     .pass2
        JSR     PRINT_FAIL
        BRA     .done
.pass2:
        JSR     PRINT_PASS

.done:
        RTS

;------------------------------------------------------------
; Test ADD instructions
;------------------------------------------------------------
TEST_ADD:
        ; Print test header
        LEA     add_header,A0
        JSR     PRINT_STRING

        ;--------------------------------------------------------
        ; Test ADD.L Dn,Dn (Data Register to Data Register)
        ;--------------------------------------------------------
        ; Set test description
        LEA     test_add_l_dn_dn,A6
        LEA     mode_dn_to_dn,A5
        MOVE.W  #$D081,D7       ; Opcode for ADD.L D1,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$11111111,D0   ; Initialize D0
        MOVE.L  #$22222222,D1   ; Initialize D1

        ; Print before state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  #$22222222,D3   ; D1 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        ADD.L   D1,D0           ; D0 = D0 + D1 = $33333333

        ; Print after state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  #$22222222,D3   ; D1 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$33333333,D2   ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$33333333,D0   ; Check if D0 has expected value
        BEQ     .pass1
        JSR     PRINT_FAIL
        BRA     .test2
.pass1:
        JSR     PRINT_PASS

        ;--------------------------------------------------------
        ; Test ADDQ.L #imm,Dn (Quick immediate to Data Register)
        ;--------------------------------------------------------
.test2:
        ; Set test description
        LEA     test_addq_l_imm_dn,A6
        LEA     mode_quick_to_dn,A5
        MOVE.W  #$5080,D7       ; Opcode for ADDQ.L #8,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$AAAAAAAA,D0   ; Initialize D0

        ; Print before state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        ADDQ.L  #8,D0           ; D0 = D0 + 8 = $AAAAAAAA + 8 = $AAAAAAAB2

        ; Print after state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$AAAAAAAA+8,D2 ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$AAAAAAAA+8,D0 ; Check if D0 has expected value
        BEQ     .pass2
        JSR     PRINT_FAIL
        BRA     .done
.pass2:
        JSR     PRINT_PASS

.done:
        RTS

;------------------------------------------------------------
; Test SUB instructions
;------------------------------------------------------------
TEST_SUB:
        ; Print test header
        LEA     sub_header,A0
        JSR     PRINT_STRING

        ;--------------------------------------------------------
        ; Test SUB.L Dn,Dn (Data Register from Data Register)
        ;--------------------------------------------------------
        ; Set test description
        LEA     test_sub_l_dn_dn,A6
        LEA     mode_dn_to_dn,A5
        MOVE.W  #$9081,D7       ; Opcode for SUB.L D1,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$33333333,D0   ; Initialize D0
        MOVE.L  #$11111111,D1   ; Initialize D1

        ; Print before state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  #$11111111,D3   ; D1 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        SUB.L   D1,D0           ; D0 = D0 - D1 = $22222222

        ; Print after state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  #$11111111,D3   ; D1 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$22222222,D2   ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$22222222,D0   ; Check if D0 has expected value
        BEQ     .pass1
        JSR     PRINT_FAIL
        BRA     .test2
.pass1:
        JSR     PRINT_PASS

        ;--------------------------------------------------------
        ; Test SUBQ.L #imm,Dn (Quick immediate from Data Register)
        ;--------------------------------------------------------
.test2:
        ; Set test description
        LEA     test_subq_l_imm_dn,A6
        LEA     mode_quick_to_dn,A5
        MOVE.W  #$5180,D7       ; Opcode for SUBQ.L #8,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$AAAAAAAA,D0   ; Initialize D0

        ; Print before state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        SUBQ.L  #8,D0           ; D0 = D0 - 8 = $AAAAAAAA - 8 = $AAAAAAA2

        ; Print after state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$AAAAAAAA-8,D2 ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$AAAAAAAA-8,D0 ; Check if D0 has expected value
        BEQ     .pass2
        JSR     PRINT_FAIL
        BRA     .done
.pass2:
        JSR     PRINT_PASS

.done:
        RTS

;------------------------------------------------------------
; Test CMP instructions
;------------------------------------------------------------
TEST_CMP:
        ; Print test header
        LEA     cmp_header,A0
        JSR     PRINT_STRING

        ;--------------------------------------------------------
        ; Test CMP.L Dn,Dn (Compare Data Register with Data Register - equal)
        ;--------------------------------------------------------
        ; Set test description
        LEA     test_cmp_l_dn_dn,A6
        LEA     mode_dn_to_dn,A5
        MOVE.W  #$B081,D7       ; Opcode for CMP.L D1,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$12345678,D0   ; Initialize D0
        MOVE.L  #$12345678,D1   ; Initialize D1 with same value

        ; Print before state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  #$12345678,D3   ; D1 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        CMP.L   D1,D0           ; Compare D0 with D1, should set Z flag

        ; Print after state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value (unchanged)
        MOVE.L  #$12345678,D3   ; D1 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        LEA     expected_z_set,A0
        JSR     PRINT_STRING

        ; Verify Z flag is set (equal)
        BEQ     .pass1
        JSR     PRINT_FAIL
        BRA     .test2
.pass1:
        JSR     PRINT_PASS

        ;--------------------------------------------------------
        ; Test CMP.L Dn,Dn (Compare Data Register with Data Register - not equal)
        ;--------------------------------------------------------
.test2:
        ; Set test description
        LEA     test_cmp_l_dn_dn_neq,A6
        LEA     mode_dn_to_dn,A5
        MOVE.W  #$B081,D7       ; Opcode for CMP.L D1,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$FEDCBA98,D0   ; Initialize D0
        MOVE.L  #$12345678,D1   ; Initialize D1 with different value

        ; Print before state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  #$12345678,D3   ; D1 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        CMP.L   D1,D0           ; Compare D0 with D1, should NOT set Z flag

        ; Print after state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value (unchanged)
        MOVE.L  #$12345678,D3   ; D1 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        LEA     expected_z_clear,A0
        JSR     PRINT_STRING

        ; Verify Z flag is not set (not equal)
        BNE     .pass2
        JSR     PRINT_FAIL
        BRA     .done
.pass2:
        JSR     PRINT_PASS

.done:
        RTS

;------------------------------------------------------------
; Test AND instructions
;------------------------------------------------------------
TEST_AND:
        ; Print test header
        LEA     and_header,A0
        JSR     PRINT_STRING

        ;--------------------------------------------------------
        ; Test AND.L Dn,Dn (Data Register AND Data Register)
        ;--------------------------------------------------------
        ; Set test description
        LEA     test_and_l_dn_dn,A6
        LEA     mode_dn_to_dn,A5
        MOVE.W  #$C081,D7       ; Opcode for AND.L D1,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$FFFF0000,D0   ; Initialize D0
        MOVE.L  #$0000FFFF,D1   ; Initialize D1

        ; Print before state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  #$0000FFFF,D3   ; D1 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        AND.L   D1,D0           ; D0 = D0 AND D1 = $00000000

        ; Print after state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  #$0000FFFF,D3   ; D1 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$00000000,D2   ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$00000000,D0   ; Check if D0 has expected value
        BEQ     .pass1
        JSR     PRINT_FAIL
        BRA     .done
.pass1:
        JSR     PRINT_PASS

.done:
        RTS

;------------------------------------------------------------
; Test OR instructions
;------------------------------------------------------------
TEST_OR:
        ; Print test header
        LEA     or_header,A0
        JSR     PRINT_STRING

        ;--------------------------------------------------------
        ; Test OR.L Dn,Dn (Data Register OR Data Register)
        ;--------------------------------------------------------
        ; Set test description
        LEA     test_or_l_dn_dn,A6
        LEA     mode_dn_to_dn,A5
        MOVE.W  #$8081,D7       ; Opcode for OR.L D1,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$FFFF0000,D0   ; Initialize D0
        MOVE.L  #$0000FFFF,D1   ; Initialize D1

        ; Print before state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  #$0000FFFF,D3   ; D1 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        OR.L    D1,D0           ; D0 = D0 OR D1 = $FFFFFFFF

        ; Print after state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  #$0000FFFF,D3   ; D1 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$FFFFFFFF,D2   ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$FFFFFFFF,D0   ; Check if D0 has expected value
        BEQ     .pass1
        JSR     PRINT_FAIL
        BRA     .done
.pass1:
        JSR     PRINT_PASS

.done:
        RTS

;------------------------------------------------------------
; Test EOR instructions
;------------------------------------------------------------
TEST_EOR:
        ; Print test header
        LEA     eor_header,A0
        JSR     PRINT_STRING

        ;--------------------------------------------------------
        ; Test EOR.L Dn,Dn (Data Register XOR Data Register)
        ;--------------------------------------------------------
        ; Set test description
        LEA     test_eor_l_dn_dn,A6
        LEA     mode_dn_to_dn,A5
        MOVE.W  #$B181,D7       ; Opcode for EOR.L D1,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$AAAAAAAA,D0   ; Initialize D0 (10101010...)
        MOVE.L  #$55555555,D1   ; Initialize D1 (01010101...)

        ; Print before state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  #$55555555,D3   ; D1 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        EOR.L   D1,D0           ; D0 = D0 XOR D1 = $FFFFFFFF

        ; Print after state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  #$55555555,D3   ; D1 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$FFFFFFFF,D2   ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$FFFFFFFF,D0   ; Check if D0 has expected value
        BEQ     .pass1
        JSR     PRINT_FAIL
        BRA     .done
.pass1:
        JSR     PRINT_PASS

.done:
        RTS

;------------------------------------------------------------
; Test NOT instruction
;------------------------------------------------------------
TEST_NOT:
        ; Print test header
        LEA     not_header,A0
        JSR     PRINT_STRING

        ;--------------------------------------------------------
        ; Test NOT.L Dn (Logical NOT of Data Register)
        ;--------------------------------------------------------
        ; Set test description
        LEA     test_not_l_dn,A6
        LEA     mode_dn,A5
        MOVE.W  #$4680,D7       ; Opcode for NOT.L D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$AAAAAAAA,D0   ; Initialize D0 (10101010...)

        ; Print before state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        NOT.L   D0              ; D0 = NOT D0 = $55555555

        ; Print after state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$55555555,D2   ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$55555555,D0   ; Check if D0 has expected value
        BEQ     .pass1
        JSR     PRINT_FAIL
        BRA     .done
.pass1:
        JSR     PRINT_PASS

.done:
        RTS

;------------------------------------------------------------
; Test NEG instruction
;------------------------------------------------------------
TEST_NEG:
        ; Print test header
        LEA     neg_header,A0
        JSR     PRINT_STRING

        ;--------------------------------------------------------
        ; Test NEG.L Dn (Negate Data Register)
        ;--------------------------------------------------------
        ; Set test description
        LEA     test_neg_l_dn,A6
        LEA     mode_dn,A5
        MOVE.W  #$4480,D7       ; Opcode for NEG.L D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$00000001,D0   ; Initialize D0 with 1

        ; Print before state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        NEG.L   D0              ; D0 = 0 - D0 = -1 = $FFFFFFFF

        ; Print after state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$FFFFFFFF,D2   ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$FFFFFFFF,D0   ; Check if D0 has expected value
        BEQ     .pass1
        JSR     PRINT_FAIL
        BRA     .done
.pass1:
        JSR     PRINT_PASS

.done:
        RTS

;------------------------------------------------------------
; Test Shift/Rotate instructions
;------------------------------------------------------------
TEST_SHIFT:
        ; Print test header
        LEA     shift_header,A0
        JSR     PRINT_STRING

        ;--------------------------------------------------------
        ; Test LSL.L #imm,Dn (Logical Shift Left)
        ;--------------------------------------------------------
        ; Set test description
        LEA     test_lsl_l,A6
        LEA     mode_imm_to_dn,A5
        MOVE.W  #$E188,D7       ; Opcode for LSL.L #8,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$00000001,D0   ; Initialize D0 with 1

        ; Print before state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        LSL.L   #8,D0           ; D0 = D0 << 8 = $00000100

        ; Print after state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$00000100,D2   ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$00000100,D0   ; Check if D0 has expected value
        BEQ     .pass1
        JSR     PRINT_FAIL
        BRA     .test2
.pass1:
        JSR     PRINT_PASS

        ;--------------------------------------------------------
        ; Test LSR.L #imm,Dn (Logical Shift Right)
        ;--------------------------------------------------------
.test2:
        ; Set test description
        LEA     test_lsr_l,A6
        LEA     mode_imm_to_dn,A5
        MOVE.W  #$E088,D7       ; Opcode for LSR.L #8,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$00010000,D0   ; Initialize D0

        ; Print before state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        LSR.L   #8,D0           ; D0 = D0 >> 8 = $00000100

        ; Print after state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$00000100,D2   ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$00000100,D0   ; Check if D0 has expected value
        BEQ     .pass2
        JSR     PRINT_FAIL
        BRA     .test3
.pass2:
        JSR     PRINT_PASS

        ;--------------------------------------------------------
        ; Test ROL.L #imm,Dn (Rotate Left)
        ;--------------------------------------------------------
.test3:
        ; Set test description
        LEA     test_rol_l,A6
        LEA     mode_imm_to_dn,A5
        MOVE.W  #$E198,D7       ; Opcode for ROL.L #1,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$80000000,D0   ; Initialize D0 with MSB set

        ; Print before state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        ROL.L   #1,D0           ; Rotate left by 1 = $00000001

        ; Print after state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$00000001,D2   ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$00000001,D0   ; Check if D0 has expected value
        BEQ     .pass3
        JSR     PRINT_FAIL
        BRA     .test4
.pass3:
        JSR     PRINT_PASS

        ;--------------------------------------------------------
        ; Test ROR.L #imm,Dn (Rotate Right)
        ;--------------------------------------------------------
.test4:
        ; Set test description
        LEA     test_ror_l,A6
        LEA     mode_imm_to_dn,A5
        MOVE.W  #$E098,D7       ; Opcode for ROR.L #1,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$00000001,D0   ; Initialize D0 with LSB set

        ; Print before state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        ROR.L   #1,D0           ; Rotate right by 1 = $80000000

        ; Print after state
        MOVE.B  #$01,D1         ; Print only D0 (bit 0)
        MOVE.L  D0,D2           ; D0 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$80000000,D2   ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$80000000,D0   ; Check if D0 has expected value
        BEQ     .pass4
        JSR     PRINT_FAIL
        BRA     .done
.pass4:
        JSR     PRINT_PASS

.done:
        RTS

;------------------------------------------------------------
; Test Bit manipulation instructions
;------------------------------------------------------------
TEST_BIT:
        ; Print test header
        LEA     bit_header,A0
        JSR     PRINT_STRING

        ;--------------------------------------------------------
        ; Test BTST Dn,Dn (Test bit in Data Register)
        ;--------------------------------------------------------
        ; Set test description
        LEA     test_btst_dn_dn,A6
        LEA     mode_dn_to_dn,A5
        MOVE.W  #$0100,D7       ; Opcode for BTST D1,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$00000001,D0   ; Initialize D0 with bit 0 set
        MOVE.L  #0,D1           ; Bit number 0

        ; Print before state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  #0,D3           ; D1 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        BTST    D1,D0           ; Test bit 0 in D0, should clear Z flag (bit set)

        ; Print after state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value (unchanged)
        MOVE.L  #0,D3           ; D1 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        LEA     expected_z_clear,A0
        JSR     PRINT_STRING

        ; Verify Z flag is clear (bit is set)
        BNE     .pass1
        JSR     PRINT_FAIL
        BRA     .test2
.pass1:
        JSR     PRINT_PASS

        ;--------------------------------------------------------
        ; Test BSET Dn,Dn (Set bit in Data Register)
        ;--------------------------------------------------------
.test2:
        ; Set test description
        LEA     test_bset_dn_dn,A6
        LEA     mode_dn_to_dn,A5
        MOVE.W  #$01C0,D7       ; Opcode for BSET D1,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$00000000,D0   ; Initialize D0 with all bits clear
        MOVE.L  #16,D1          ; Bit number 16

        ; Print before state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  #16,D3          ; D1 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        BSET    D1,D0           ; Set bit 16 in D0, should give $00010000

        ; Print after state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  #16,D3          ; D1 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$00010000,D2   ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$00010000,D0   ; Check if D0 has expected value
        BEQ     .pass2
        JSR     PRINT_FAIL
        BRA     .test3
.pass2:
        JSR     PRINT_PASS

        ;--------------------------------------------------------
        ; Test BCLR Dn,Dn (Clear bit in Data Register)
        ;--------------------------------------------------------
.test3:
        ; Set test description
        LEA     test_bclr_dn_dn,A6
        LEA     mode_dn_to_dn,A5
        MOVE.W  #$0180,D7       ; Opcode for BCLR D1,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$FFFFFFFF,D0   ; Initialize D0 with all bits set
        MOVE.L  #31,D1          ; Bit number 31 (MSB)

        ; Print before state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  #31,D3          ; D1 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        BCLR    D1,D0           ; Clear bit 31 in D0, should give $7FFFFFFF

        ; Print after state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  #31,D3          ; D1 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$7FFFFFFF,D2   ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$7FFFFFFF,D0   ; Check if D0 has expected value
        BEQ     .pass3
        JSR     PRINT_FAIL
        BRA     .test4
.pass3:
        JSR     PRINT_PASS

        ;--------------------------------------------------------
        ; Test BCHG Dn,Dn (Change bit in Data Register)
        ;--------------------------------------------------------
.test4:
        ; Set test description
        LEA     test_bchg_dn_dn,A6
        LEA     mode_dn_to_dn,A5
        MOVE.W  #$0140,D7       ; Opcode for BCHG D1,D0
        JSR     PRINT_TEST_HEADER

        ; Setup
        MOVE.L  #$00000000,D0   ; Initialize D0 with all bits clear
        MOVE.L  #24,D1          ; Bit number 24

        ; Print before state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  #24,D3          ; D1 value
        JSR     PRINT_BEFORE_STATE

        ; Execute
        BCHG    D1,D0           ; Toggle bit 24 in D0, should give $01000000

        ; Print after state
        MOVE.B  #$03,D1         ; Print D0 and D1 (bits 0-1)
        MOVE.L  D0,D2           ; D0 value
        MOVE.L  #24,D3          ; D1 value
        JSR     PRINT_AFTER_STATE

        ; Print flags
        JSR     PRINT_FLAGS

        ; Print expected results
        MOVE.B  #$01,D1         ; Check only D0 (bit 0)
        MOVE.L  #$01000000,D2   ; Expected D0 value
        JSR     PRINT_EXPECTED

        ; Verify
        CMP.L   #$01000000,D0   ; Check if D0 has expected value
        BEQ     .pass4
        JSR     PRINT_FAIL
        BRA     .done
.pass4:
        JSR     PRINT_PASS

.done:
        RTS

;------------------------------------------------------------
; Test data and buffers
;------------------------------------------------------------
test_buffer:
        DC.L    0,0,0,0,0,0,0,0     ; 32 bytes of test buffer space
test_long_addr:
        DC.L    0                   ; For absolute addressing tests

;------------------------------------------------------------
; Test messages
;------------------------------------------------------------
header_msg:
        DC.B    "68k Opcode Test Suite",13,10,0

; Group headers
move_header:
        DC.B    13,10,"Testing MOVE instructions:",13,10,0
moveq_header:
        DC.B    13,10,"Testing MOVEQ instructions:",13,10,0
add_header:
        DC.B    13,10,"Testing ADD instructions:",13,10,0
sub_header:
        DC.B    13,10,"Testing SUB instructions:",13,10,0
cmp_header:
        DC.B    13,10,"Testing CMP instructions:",13,10,0
and_header:
        DC.B    13,10,"Testing AND instructions:",13,10,0
or_header:
        DC.B    13,10,"Testing OR instructions:",13,10,0
eor_header:
        DC.B    13,10,"Testing EOR instructions:",13,10,0
not_header:
        DC.B    13,10,"Testing NOT instruction:",13,10,0
neg_header:
        DC.B    13,10,"Testing NEG instruction:",13,10,0
shift_header:
        DC.B    13,10,"Testing Shift/Rotate instructions:",13,10,0
bit_header:
        DC.B    13,10,"Testing Bit Manipulation instructions:",13,10,0

; Test names
test_move_b_dn_dn:
        DC.B    "MOVE.B D1,D0",0
test_move_w_dn_dn:
        DC.B    "MOVE.W D1,D0",0
test_move_l_dn_dn:
        DC.B    "MOVE.L D1,D0",0
test_move_l_imm_dn:
        DC.B    "MOVE.L #$12345678,D0",0
test_move_l_dn_an_ind:
        DC.B    "MOVE.L D0,(A0)",0
test_move_l_an_post_dn:
        DC.B    "MOVE.L (A0)+,D0",0
test_move_l_an_pre_dn:
        DC.B    "MOVE.L -(A0),D0",0
test_move_l_an_disp_dn:
        DC.B    "MOVE.L 16(A0),D0",0
test_move_l_abs_dn:
        DC.B    "MOVE.L test_long_addr,D0",0
test_moveq_pos:
        DC.B    "MOVEQ #127,D0",0
test_moveq_neg:
        DC.B    "MOVEQ #-128,D0",0
test_add_l_dn_dn:
        DC.B    "ADD.L D1,D0",0
test_add_l_imm_dn:
        DC.B    "ADD.L #$11111111,D0",0
test_addq_l_imm_dn:
        DC.B    "ADDQ.L #8,D0",0
test_sub_l_dn_dn:
        DC.B    "SUB.L D1,D0",0
test_subq_l_imm_dn:
        DC.B    "SUBQ.L #8,D0",0
test_cmp_l_dn_dn:
        DC.B    "CMP.L D1,D0 (equal values)",0
test_cmp_l_dn_dn_neq:
        DC.B    "CMP.L D1,D0 (different values)",0
test_and_l_dn_dn:
        DC.B    "AND.L D1,D0",0
test_or_l_dn_dn:
        DC.B    "OR.L D1,D0",0
test_eor_l_dn_dn:
        DC.B    "EOR.L D1,D0",0
test_not_l_dn:
        DC.B    "NOT.L D0",0
test_neg_l_dn:
        DC.B    "NEG.L D0",0
test_lsl_l:
        DC.B    "LSL.L #8,D0",0
test_lsr_l:
        DC.B    "LSR.L #8,D0",0
test_rol_l:
        DC.B    "ROL.L #1,D0",0
test_ror_l:
        DC.B    "ROR.L #1,D0",0
test_btst_dn_dn:
        DC.B    "BTST D1,D0",0
test_bset_dn_dn:
        DC.B    "BSET D1,D0",0
test_bclr_dn_dn:
        DC.B    "BCLR D1,D0",0
test_bchg_dn_dn:
        DC.B    "BCHG D1,D0",0

; Addressing mode descriptions
mode_dn_to_dn:
        DC.B    "Data Reg to Data Reg",0
mode_imm_to_dn:
        DC.B    "Immediate to Data Reg",0
mode_dn_to_an_ind:
        DC.B    "Data Reg to Addr Reg Ind",0
mode_an_post_to_dn:
        DC.B    "Post-Increment Addr to Data Reg",0
mode_an_pre_to_dn:
        DC.B    "Pre-Decrement Addr to Data Reg",0
mode_an_disp_to_dn:
        DC.B    "Displacement Addr to Data Reg",0
mode_abs_to_dn:
        DC.B    "Absolute Addr to Data Reg",0
mode_quick_to_dn:
        DC.B    "Quick Immediate to Data Reg",0
mode_dn:
        DC.B    "Data Register Operation",0

; Output format messages
test_prefix:
        DC.B    ">> ",0
opcode_msg:
        DC.B    " [0x",0
mode_prefix:
        DC.B    " (",0
before_msg:
        DC.B    "   Before: ",0
after_msg:
        DC.B    "   After:  ",0
flags_msg:
        DC.B    "   Flags:  ",0
expected_msg:
        DC.B    "   Expected ",0
n_equals:
        DC.B    "N=",0
z_equals:
        DC.B    " Z=",0
v_equals:
        DC.B    " V=",0
c_equals:
        DC.B    " C=",0
d0_equals:
        DC.B    "D0=0x",0
d1_equals:
        DC.B    "D1=0x",0
a0_equals:
        DC.B    "A0=0x",0
mem_a0_equals:
        DC.B    "MEM[A0]=0x",0
mem_a0_plus16_equals:
        DC.B    "   MEM[A0+16]=0x",0
mem_abs_equals:
        DC.B    "   MEM[",0
test_long_addr_txt:
        DC.B    "test_long_addr",0
equals_sign:
        DC.B    "]=0x",0
comma_msg:
        DC.B    ", ",0
expected_z_set:
        DC.B    "   Expected Z=1 (equal values)",13,10,0
expected_z_clear:
        DC.B    "   Expected Z=0 (values not equal)",13,10,0
newline:
        DC.B    13,10,0

; Result messages
pass_msg:
        DC.B    "   ** PASS **",13,10,0
fail_msg:
        DC.B    "   ** FAIL **",13,10,0
separator_line:
        DC.B    "   -------------------------------------------------",13,10,0
all_done_msg:
        DC.B    13,10,"All tests completed",13,10,0

        END     START