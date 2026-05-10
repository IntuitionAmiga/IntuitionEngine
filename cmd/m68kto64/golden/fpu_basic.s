; m68k → IE64 transpiler golden test: 68881/68882 FPU basics.
;
; Exercises FMOVE.X / FMOVE.D / FADD / FSUB / FMUL / FDIV / FNEG / FABS /
; FSQRT / FINT / FINTRZ / FCMP / FBEQ / FSIN / FCOS. End-to-end transpile
; → ie64asm conformance gate for Phase 7.

start:
	; Load doubles from memory into FP regs.
	fmove.d	(a0),fp0
	fmove.d	(a1),fp1

	; Arithmetic chain.
	fadd.x	fp1,fp0
	fmul.x	fp1,fp0
	fsub.x	fp1,fp0
	fdiv.x	fp1,fp0
	fneg.x	fp0
	fabs.x	fp0
	fsqrt.x	fp0
	fint.x	fp0
	fintrz.x	fp0

	; Transcendentals (single-precision native, doubled-precision wrap).
	fsin.x	fp0,fp2
	fcos.x	fp0,fp3
	fmul.x	fp3,fp2
	fadd.x	fp2,fp0

	; Comparison + FBEQ.
	fcmp.x	fp1,fp0
	fbeq	done

	; FMOVE back to memory.
	fmove.d	fp0,(a0)
done:
	rts
