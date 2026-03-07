; IntuitionEngine M68K CPU test suite.

                include "cputest_runtime.inc"
                include "cputest_manifest.inc"

                xref    run_bf_mem_shard
                xref    run_bf_reg_shard
                xref    run_callm_rtm_shard
                xref    run_cas_ops_shard
                xref    run_chk2_cmp2_shard
                xref    run_core_020_shard
                xref    run_core_alu_shard
                xref    run_core_bcd_shard
                xref    run_core_flow_shard
                xref    run_core_misc_shard
                xref    run_core_move_ctrl_shard
                xref    run_core_move_ea_shard
                xref    run_core_shift_bit_shard
                xref    run_ea_020_brief_shard
                xref    run_ea_020_full_shard
                xref    run_ea_020_memindir_shard
                xref    run_ea_control_shard
                xref    run_ea_read_ops_shard
                xref    run_ea_write_ops_shard
                xref    run_exception_return_shard
                xref    run_fpu_arith_shard
                xref    run_fpu_cond_shard
                xref    run_fpu_ctrl_state_shard
                xref    run_fpu_data_shard
                xref    run_fpu_formats_shard
                xref    run_fpu_trans_shard
                xref    run_fpu_unary_shard
                xref    run_muldiv_020_shard
                xref    run_supervisor_ctrl_shard
                xref    run_trap_basic_shard
                xref    ct_set_expected_total

                section text,code

start:
                bsr     ct_set_expected_total
                bsr     ct_init
                lea     ct_suite_shards(pc),a0
.next:
                movea.l (a0)+,a1
                beq.s   .done
                jsr     (a1)
                bra.s   .next
.done:
                bsr     ct_finish
                moveq   #0,d0
                rts

                include "../cputest/generated/bf_mem_cases.inc"
                include "../cputest/generated/bf_reg_cases.inc"
                include "../cputest/generated/callm_rtm_cases.inc"
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
