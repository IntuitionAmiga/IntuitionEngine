; Bare-metal runtime for the M68K CPU test harness.
; No AmigaOS dependencies — reports results via memory-mapped mailbox at $80000.

; ---------------------------------------------------------------------------
; Mailbox layout at $80000
; ---------------------------------------------------------------------------

ct_mailbox          equ $80000
ct_pass_count       equ ct_mailbox+0
ct_fail_count       equ ct_mailbox+4
ct_expected_total   equ ct_mailbox+8
ct_suite_done       equ ct_mailbox+12
ct_fail_log_pos     equ ct_mailbox+16
ct_case_log_pos     equ ct_mailbox+20
ct_trap_taken       equ ct_mailbox+24
ct_trap_vector      equ ct_mailbox+28
ct_trap_resume      equ ct_mailbox+32
ct_trap_old_vector  equ ct_mailbox+36
ct_trap_installed_vec equ ct_mailbox+40
ct_fpsr_shadow      equ ct_mailbox+44
ct_fp_scratch       equ ct_mailbox+48
; +56..+63 reserved
ct_case_log         equ ct_mailbox+64
ct_case_log_size    equ 4096
ct_fail_log         equ ct_case_log+ct_case_log_size
ct_fail_log_size    equ 16384
ct_mailbox_end      equ ct_fail_log+ct_fail_log_size

; Fail record size: 28 bytes
ct_fail_rec_size    equ 28
; Case record size: 8 bytes
ct_case_rec_size    equ 8

; Fail type codes
ct_ft_reg_sr        equ 1
ct_ft_reg_only      equ 2
ct_ft_fp64          equ 3
ct_ft_mem32         equ 4
ct_ft_multi_reg     equ 5
ct_ft_exception     equ 6

; ---------------------------------------------------------------------------
; ct_init — clear mailbox region
; ---------------------------------------------------------------------------

; ct_init — mailbox is pre-zeroed by the Go test driver before execution.
; This just ensures pass/fail counts are zero in case of re-entry.
ct_init:
                clr.l   ct_pass_count
                clr.l   ct_fail_count
                clr.l   ct_suite_done
                clr.l   ct_fail_log_pos
                clr.l   ct_case_log_pos
                rts

; ---------------------------------------------------------------------------
; ct_finish — signal completion
; ---------------------------------------------------------------------------

ct_finish:
                move.l  #1,ct_suite_done
                rts

; ---------------------------------------------------------------------------
; ct_enter_shard — no-op in bare-metal mode
; ---------------------------------------------------------------------------

ct_enter_shard:
                rts

; ---------------------------------------------------------------------------
; ct_log_pass — log a passing case
; Input: a0 = name_ptr
; Normal rts (returns to instruction after jsr).
; ---------------------------------------------------------------------------

ct_log_pass:
                addq.l  #1,ct_pass_count
                ; Append case log record: {name_ptr, result=1}
                move.l  ct_case_log_pos,d0
                lea     ct_case_log,a1
                move.l  a0,0(a1,d0.l)          ; name_ptr
                move.l  #1,4(a1,d0.l)          ; result = pass
                addq.l  #8,ct_case_log_pos
                rts

; ---------------------------------------------------------------------------
; ct_fail_reg_sr — fail with register + SR
; Input: a0=name, a1=input, a2=expected, d0=reg, d1=SR
; Terminal: discards JSR return, returns to shard.
; ---------------------------------------------------------------------------

ct_fail_reg_sr:
                move.l  d0,d4
                move.l  d1,d5
                moveq   #0,d6                   ; d2 unused
                moveq   #ct_ft_reg_sr,d3
                bra     ct_write_fail_record

; ---------------------------------------------------------------------------
; ct_fail_reg_only — fail with register only
; Input: a0=name, a1=input, a2=expected, d0=reg
; Terminal.
; ---------------------------------------------------------------------------

ct_fail_reg_only:
                move.l  d0,d4
                moveq   #0,d5
                moveq   #0,d6
                moveq   #ct_ft_reg_only,d3
                bra     ct_write_fail_record

; ---------------------------------------------------------------------------
; ct_fail_fp64 — fail with FP64 value + FPSR
; Input: a0=name, a1=input, a2=expected, d0=fp_hi, d1=fp_lo, d2=fpsr
; Terminal.
; ---------------------------------------------------------------------------

ct_fail_fp64:
                move.l  d0,d4
                move.l  d1,d5
                move.l  d2,d6
                moveq   #ct_ft_fp64,d3
                bra     ct_write_fail_record

; ---------------------------------------------------------------------------
; ct_fail_mem32 — fail with memory value
; Input: a0=name, a1=input, a2=expected, d0=mem
; Terminal.
; ---------------------------------------------------------------------------

ct_fail_mem32:
                move.l  d0,d4
                moveq   #0,d5
                moveq   #0,d6
                moveq   #ct_ft_mem32,d3
                bra     ct_write_fail_record

; ---------------------------------------------------------------------------
; ct_fail_multi_reg — fail with two register values
; Input: a0=name, a1=input, a2=expected, d0=r0, d1=r1
; Terminal.
; ---------------------------------------------------------------------------

ct_fail_multi_reg:
                move.l  d0,d4
                move.l  d1,d5
                moveq   #0,d6
                moveq   #ct_ft_multi_reg,d3
                bra     ct_write_fail_record

; ---------------------------------------------------------------------------
; ct_fail_exception — fail with trap metadata
; Input: a0=name, a1=input, a2=expected, d0=trap_taken, d1=trap_vector
; Terminal.
; ---------------------------------------------------------------------------

ct_fail_exception:
                move.l  d0,d4
                move.l  d1,d5
                moveq   #0,d6
                moveq   #ct_ft_exception,d3
                bra     ct_write_fail_record

; ---------------------------------------------------------------------------
; ct_write_fail_record — common fail record writer
; Input: a0=name, a1=input, a2=expected, d3=fail_type, d4=actual_d0,
;        d5=actual_d1, d6=actual_d2
; Terminal: discards JSR return, returns to shard.
; ---------------------------------------------------------------------------

ct_write_fail_record:
                addq.l  #1,ct_fail_count

                ; Write 28-byte fail log record
                move.l  ct_fail_log_pos,d7
                lea     ct_fail_log,a3
                move.l  a0,0(a3,d7.l)          ; name_ptr
                move.l  a1,4(a3,d7.l)          ; input_ptr
                move.l  a2,8(a3,d7.l)          ; expect_ptr
                move.l  d4,12(a3,d7.l)         ; actual_d0
                move.l  d5,16(a3,d7.l)         ; actual_d1
                move.l  d6,20(a3,d7.l)         ; actual_d2
                move.l  d3,24(a3,d7.l)         ; fail_type (byte + 3 pad)
                add.l   #ct_fail_rec_size,d7
                move.l  d7,ct_fail_log_pos

                ; Append case log record: {name_ptr, result=0}
                move.l  ct_case_log_pos,d7
                lea     ct_case_log,a3
                move.l  a0,0(a3,d7.l)          ; name_ptr
                clr.l   4(a3,d7.l)             ; result = fail
                addq.l  #8,ct_case_log_pos

                ; Discard JSR return address (points to inline .name: data)
                addq.l  #4,sp
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
                move.w  10(sp),d0               ; format/vector word
                andi.l  #$0FFF,d0               ; mask to vector offset (low 12 bits)
                lsr.l   #2,d0                   ; offset / 4 = vector number
                move.l  d0,ct_trap_vector
                ; If a resume address is set, overwrite the stacked PC so RTE does
                ; not re-execute the faulting instruction.
                move.l  ct_trap_resume,d0
                beq.s   .no_resume
                move.l  d0,6(sp)                ; overwrite stacked PC
                ori.w   #$2000,4(sp)            ; ensure supervisor mode in stacked SR
                clr.l   ct_trap_resume          ; one-shot
.no_resume:
                move.l  (sp)+,d0
                rte

; ---------------------------------------------------------------------------
; ct_install_trap_handler — D0 = vector number
; Patches the VBR vector table entry; saves old vector for restore.
; Clobbers a0, a1. Preserves d1-d7, a2-a6.
; ---------------------------------------------------------------------------

ct_install_trap_handler:
                move.l  d0,ct_trap_installed_vec
                lsl.l   #2,d0                   ; vector * 4
                movec   vbr,a0                  ; get VBR (68020)
                move.l  0(a0,d0.l),ct_trap_old_vector
                lea     ct_trap_handler(pc),a1
                move.l  a1,0(a0,d0.l)
                clr.l   ct_trap_taken
                clr.l   ct_trap_vector
                clr.l   ct_trap_resume
                rts

; ---------------------------------------------------------------------------
; ct_restore_trap_handler — restores the VBR vector patched by install.
; Clobbers d0, a0. Preserves d1-d7, a1-a6.
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
                even
