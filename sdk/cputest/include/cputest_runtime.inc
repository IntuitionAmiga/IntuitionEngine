; Common runtime for the M68K CPU test harness.

_LVOOpenLibrary     equ -552
_LVOCloseLibrary    equ -414

_LVOOpen            equ -30
_LVOClose           equ -36
_LVOWrite           equ -48

MODE_NEWFILE        equ 1006

                xdef    ct_init
                xdef    ct_finish
                xdef    ct_enter_shard
                xdef    ct_fail_reg_sr
                xdef    ct_fail_reg_only
                xdef    ct_fail_fp64
                xdef    ct_pass_count
                xdef    ct_fail_count
                xdef    ct_fp_scratch
                xdef    ct_fpsr_shadow
                xdef    ct_fail_mem32
                xdef    ct_fail_multi_reg
                xdef    ct_install_trap_handler
                xdef    ct_restore_trap_handler
                xdef    ct_check_trap_taken
                xdef    ct_trap_taken
                xdef    ct_trap_vector
                xdef    ct_trap_resume
                xdef    ct_expected_total

; ---------------------------------------------------------------------------
; Text
; ---------------------------------------------------------------------------

                section cputest_text,code

ct_init:
                movea.l 4.w,a6
                move.l  a6,ct_exec_base

                lea     ct_dos_name(pc),a1
                moveq   #0,d0
                jsr     _LVOOpenLibrary(a6)
                move.l  d0,ct_dos_base
                beq.s   .done

                movea.l d0,a6
                lea     ct_report_path(pc),a0
                move.l  a0,d1
                move.l  #MODE_NEWFILE,d2
                jsr     _LVOOpen(a6)
                move.l  d0,ct_report_fh

                lea     ct_banner(pc),a0
                bsr     ct_write_cstr
.done:
                clr.l   ct_pass_count
                clr.l   ct_fail_count
                rts

ct_finish:
                lea     ct_summary_prefix(pc),a0
                bsr     ct_write_cstr
                move.l  ct_pass_count,d0
                bsr     ct_write_hex32
                lea     ct_summary_mid(pc),a0
                bsr     ct_write_cstr
                move.l  ct_fail_count,d0
                bsr     ct_write_hex32
                lea     ct_newline(pc),a0
                bsr     ct_write_cstr

                ; Verify total
                move.l  ct_expected_total,d3
                beq.s   .skip_total     ; 0 means not set, skip check
                move.l  ct_pass_count,d4
                add.l   ct_fail_count,d4
                cmp.l   d3,d4
                beq.s   .skip_total
                lea     ct_mismatch_prefix(pc),a0
                bsr     ct_write_cstr
                move.l  d3,d0
                bsr     ct_write_hex32
                lea     ct_mismatch_mid(pc),a0
                bsr     ct_write_cstr
                move.l  d4,d0
                bsr     ct_write_hex32
                lea     ct_newline(pc),a0
                bsr     ct_write_cstr
.skip_total:

                move.l  ct_report_fh,d1
                beq.s   .close_lib
                movea.l ct_dos_base,a6
                jsr     _LVOClose(a6)
                clr.l   ct_report_fh

.close_lib:
                move.l  ct_dos_base,d0
                beq.s   .done
                movea.l d0,a1
                movea.l ct_exec_base,a6
                jsr     _LVOCloseLibrary(a6)
                clr.l   ct_dos_base
.done:
                rts

ct_enter_shard:
                lea     ct_shard_prefix(pc),a1
                bsr     ct_write_cstr_a1
                bsr     ct_write_cstr
                lea     ct_newline(pc),a0
                bsr     ct_write_cstr
                rts

ct_fail_reg_sr:
                move.l  d0,d6
                move.w  d1,d7
                bsr     ct_begin_failure
                lea     ct_actual_prefix(pc),a0
                bsr     ct_write_cstr
                lea     ct_reg_prefix(pc),a0
                bsr     ct_write_cstr
                move.l  d6,d0
                bsr     ct_write_hex32
                lea     ct_sr_prefix(pc),a0
                bsr     ct_write_cstr
                moveq   #0,d0
                move.w  d7,d0
                bsr     ct_write_hex32
                lea     ct_newline(pc),a0
                bsr     ct_write_cstr
                rts

ct_fail_reg_only:
                move.l  d0,d6
                bsr     ct_begin_failure
                lea     ct_actual_prefix(pc),a0
                bsr     ct_write_cstr
                lea     ct_reg_prefix(pc),a0
                bsr     ct_write_cstr
                move.l  d6,d0
                bsr     ct_write_hex32
                lea     ct_newline(pc),a0
                bsr     ct_write_cstr
                rts

ct_fail_fp64:
                move.l  d0,d4
                move.l  d1,d5
                move.l  d2,d6
                bsr     ct_begin_failure
                lea     ct_actual_prefix(pc),a0
                bsr     ct_write_cstr
                lea     ct_fp_prefix(pc),a0
                bsr     ct_write_cstr
                move.l  d4,d0
                bsr     ct_write_hex32
                move.l  d5,d0
                bsr     ct_write_hex32
                lea     ct_fpsr_prefix(pc),a0
                bsr     ct_write_cstr
                move.l  d6,d0
                bsr     ct_write_hex32
                lea     ct_newline(pc),a0
                bsr     ct_write_cstr
                rts

ct_begin_failure:
                addq.l  #1,ct_fail_count
                move.l  a0,-(sp)
                move.l  a1,-(sp)
                move.l  a2,-(sp)
                lea     ct_fail_prefix(pc),a0
                bsr     ct_write_cstr
                movea.l 8(sp),a0
                bsr     ct_write_cstr
                lea     ct_newline(pc),a0
                bsr     ct_write_cstr
                lea     ct_input_prefix(pc),a0
                bsr     ct_write_cstr
                movea.l 4(sp),a0
                bsr     ct_write_cstr
                lea     ct_newline(pc),a0
                bsr     ct_write_cstr
                lea     ct_expected_prefix(pc),a0
                bsr     ct_write_cstr
                movea.l (sp),a0
                bsr     ct_write_cstr
                lea     ct_newline(pc),a0
                bsr     ct_write_cstr
                addq.l  #4,sp
                addq.l  #4,sp
                addq.l  #4,sp
                rts

ct_write_cstr_a1:
                movea.l a1,a0

ct_write_cstr:
                movem.l d0-d3/a0-a2,-(sp)
                movea.l a0,a1
                moveq   #0,d3
.len_loop:
                tst.b   (a1)+
                beq.s   .len_done
                addq.l  #1,d3
                bra.s   .len_loop
.len_done:
                tst.l   d3
                beq.s   .out
                move.l  ct_report_fh,d1
                beq.s   .out
                move.l  a0,d2
                move.l  d3,d3
                movea.l ct_dos_base,a6
                jsr     _LVOWrite(a6)
.out:
                movem.l (sp)+,d0-d3/a0-a2
                rts

ct_write_hex32:
                movem.l d1-d3/a0,-(sp)
                lea     ct_hex_buf+7(pc),a0
                moveq   #8-1,d3
.loop:
                move.l  d0,d1
                andi.l  #$F,d1
                lea     ct_hex_digits(pc),a1
                move.b  0(a1,d1.w),(a0)
                lsr.l   #4,d0
                subq.l  #1,a0
                dbra    d3,.loop
                lea     ct_hex_buf(pc),a0
                bsr     ct_write_cstr
                movem.l (sp)+,d1-d3/a0
                rts

ct_fail_mem32:
                move.l  d0,d6
                bsr     ct_begin_failure
                lea     ct_actual_prefix(pc),a0
                bsr     ct_write_cstr
                lea     ct_mem_prefix(pc),a0
                bsr     ct_write_cstr
                move.l  d6,d0
                bsr     ct_write_hex32
                lea     ct_newline(pc),a0
                bsr     ct_write_cstr
                rts

ct_fail_multi_reg:
                move.l  d0,d6
                move.l  d1,d7
                bsr     ct_begin_failure
                lea     ct_actual_prefix(pc),a0
                bsr     ct_write_cstr
                lea     ct_r0_prefix(pc),a0
                bsr     ct_write_cstr
                move.l  d6,d0
                bsr     ct_write_hex32
                lea     ct_r1_prefix(pc),a0
                bsr     ct_write_cstr
                move.l  d7,d0
                bsr     ct_write_hex32
                lea     ct_newline(pc),a0
                bsr     ct_write_cstr
                rts

; ---------------------------------------------------------------------------
; Trap handler — records vector number, supports resume for faulting exceptions
; ---------------------------------------------------------------------------

ct_trap_handler:
                move.l  d0,-(sp)
                move.l  #1,ct_trap_taken
                ; Extract vector number from format/vector word in exception frame.
                ; Stack after push: [saved_d0](SP) [SR](SP+4) [PC](SP+6) [fmt/vec](SP+10)
                moveq   #0,d0
                move.w  10(sp),d0       ; format/vector word
                andi.l  #$0FFF,d0       ; mask to vector offset (low 12 bits)
                lsr.l   #2,d0           ; offset / 4 = vector number
                move.l  d0,ct_trap_vector
                ; If a resume address is set, overwrite the stacked PC so RTE does
                ; not re-execute the faulting instruction (privilege violations,
                ; illegal/unimplemented opcodes stack the faulting PC).
                move.l  ct_trap_resume,d0
                beq.s   .no_resume
                move.l  d0,6(sp)        ; overwrite stacked PC
                ori.w   #$2000,4(sp)    ; ensure supervisor mode in stacked SR
                clr.l   ct_trap_resume  ; one-shot
.no_resume:
                move.l  (sp)+,d0
                rte

; ---------------------------------------------------------------------------
; ct_install_trap_handler — D0 = vector number
; Patches the VBR vector table entry; saves old vector for restore.
; ---------------------------------------------------------------------------

ct_install_trap_handler:
                move.l  d0,ct_trap_installed_vec
                lsl.l   #2,d0           ; vector * 4
                movec   vbr,a0          ; get VBR (68020)
                move.l  0(a0,d0.l),ct_trap_old_vector
                lea     ct_trap_handler(pc),a1
                move.l  a1,0(a0,d0.l)
                clr.l   ct_trap_taken
                clr.l   ct_trap_vector
                clr.l   ct_trap_resume
                rts

; ---------------------------------------------------------------------------
; ct_restore_trap_handler — restores the VBR vector patched by install.
; Clobbers D0, A0.
; ---------------------------------------------------------------------------

ct_restore_trap_handler:
                move.l  ct_trap_installed_vec,d0
                lsl.l   #2,d0
                movec   vbr,a0
                move.l  ct_trap_old_vector,0(a0,d0.l)
                rts

ct_check_trap_taken:
                move.l  ct_trap_taken,d0
                rts

; ---------------------------------------------------------------------------
; Data
; ---------------------------------------------------------------------------

                section cputest_data,data

ct_exec_base:        dc.l    0
ct_dos_base:         dc.l    0
ct_report_fh:        dc.l    0
ct_pass_count:       dc.l    0
ct_fail_count:       dc.l    0
ct_fpsr_shadow:      dc.l    0
ct_fp_scratch:       dc.l    0,0
ct_hex_buf:          dc.b    "00000000",0
ct_hex_digits:       dc.b    "0123456789ABCDEF",0
ct_dos_name:         dc.b    "dos.library",0
ct_report_path:      dc.b    "RAM:cputest",0
ct_banner:           dc.b    "IntuitionEngine M68K CPU test report",10,0
ct_shard_prefix:     dc.b    10,"[shard] ",0
ct_fail_prefix:      dc.b    "FAIL ",0
ct_input_prefix:     dc.b    "input: ",0
ct_expected_prefix:  dc.b    "expected: ",0
ct_actual_prefix:    dc.b    "actual: ",0
ct_reg_prefix:       dc.b    "reg=$",0
ct_sr_prefix:        dc.b    " sr=$",0
ct_fp_prefix:        dc.b    "fp0=$",0
ct_fpsr_prefix:      dc.b    " fpsr=$",0
ct_summary_prefix:   dc.b    10,"summary pass=$",0
ct_summary_mid:      dc.b    " fail=$",0
ct_newline:          dc.b    10,0
ct_trap_taken:       dc.l    0
ct_trap_vector:      dc.l    0
ct_trap_resume:      dc.l    0
ct_trap_old_vector:  dc.l    0
ct_trap_installed_vec: dc.l  0
ct_expected_total:   dc.l    0
ct_mem_prefix:       dc.b    "mem=$",0
ct_r0_prefix:        dc.b    "r0=$",0
ct_r1_prefix:        dc.b    " r1=$",0
ct_mismatch_prefix:  dc.b    10,"MISMATCH expected_total=$",0
ct_mismatch_mid:     dc.b    " actual_total=$",0
                even
