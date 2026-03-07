; Standalone core ALU shard.

                include "cputest_runtime.inc"

                xref    run_core_alu_shard

                section text,code

start:
                bsr     ct_init
                bsr     run_core_alu_shard
                bsr     ct_finish
                moveq   #0,d0
                rts

                include "../cputest/generated/core_alu_cases.inc"
