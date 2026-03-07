; Bare-metal M68K CPU test suite (no AmigaOS dependencies).
; Runs on raw M68K emulator via Go test driver.
; Assembled with: vasmm68k_mot -Fbin -m68020 -m68881 -devpac -I include -o cputest_suite.bin cputest_suite_bare.asm

                org     $1000
                bra     start

                include "cputest_runtime_bare.inc"
                include "cputest_manifest.inc"

start:
                jsr     ct_init
                jsr     ct_set_expected_total
                lea     ct_suite_shards,a5
.next:
                move.l  (a5)+,d0
                beq.s   .done
                movea.l d0,a1
                move.l  a5,-(sp)
                jsr     (a1)
                movea.l (sp)+,a5
                bra.s   .next
.done:
                jsr     ct_finish
.halt:          bra.s   .halt

                include "../cputest/generated/bf_mem_cases.inc"
                include "../cputest/generated/bf_reg_cases.inc"
                include "../cputest/generated/cas_ops_cases.inc"
                include "../cputest/generated/chk2_cmp2_cases.inc"
                include "../cputest/generated/core_020_cases.inc"
                include "../cputest/generated/core_alu_cases.inc"
                include "../cputest/generated/core_bcd_cases.inc"
                include "../cputest/generated/core_flow_cases.inc"
                include "../cputest/generated/core_misc_cases.inc"
                include "../cputest/generated/core_move_ctrl_cases.inc"
                include "../cputest/generated/core_move_ea_cases.inc"
                include "../cputest/generated/core_shift_bit_cases.inc"
                include "../cputest/generated/ea_020_brief_cases.inc"
                include "../cputest/generated/ea_020_full_cases.inc"
                include "../cputest/generated/ea_020_memindir_cases.inc"
                include "../cputest/generated/ea_control_cases.inc"
                include "../cputest/generated/ea_read_ops_cases.inc"
                include "../cputest/generated/ea_write_ops_cases.inc"
                include "../cputest/generated/exception_return_cases.inc"
                include "../cputest/generated/fpu_arith_cases.inc"
                include "../cputest/generated/fpu_cond_cases.inc"
                include "../cputest/generated/fpu_ctrl_state_cases.inc"
                include "../cputest/generated/fpu_data_cases.inc"
                include "../cputest/generated/fpu_formats_cases.inc"
                include "../cputest/generated/fpu_trans_cases.inc"
                include "../cputest/generated/fpu_unary_cases.inc"
                include "../cputest/generated/muldiv_020_cases.inc"
                include "../cputest/generated/supervisor_ctrl_cases.inc"
                include "../cputest/generated/trap_basic_cases.inc"
