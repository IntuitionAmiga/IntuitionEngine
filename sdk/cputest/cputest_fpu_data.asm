; Standalone FPU data shard.

                include "cputest_runtime.inc"

                xref    run_fpu_data_shard

                section text,code

start:
                bsr     ct_init
                bsr     run_fpu_data_shard
                bsr     ct_finish
                moveq   #0,d0
                rts

                include "../cputest/generated/fpu_data_cases.inc"
