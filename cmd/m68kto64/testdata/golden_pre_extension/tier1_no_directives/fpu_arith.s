; Tier-1: FPU arithmetic coverage.
start:
	fmove.d	(a0),fp0
	fmove.d	(a1),fp1
	fadd.x	fp1,fp0
	fsub.x	fp1,fp0
	fmul.x	fp1,fp0
	fdiv.x	fp1,fp0
	fabs.x	fp0
	fneg.x	fp0
	fmove.d	fp0,(a0)
	rts
