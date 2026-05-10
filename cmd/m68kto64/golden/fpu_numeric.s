; m68k FPU numeric-differential program. Computes several FP results and
; stores them to memory at well-known offsets so a runtime harness can
; read them back and compare to a host-math reference.
;
; Layout (8 bytes per slot, stored as IEEE 754 double):
;   $1000: sin(1.0)
;   $1008: cos(0.5)
;   $1010: exp(1.0)
;   $1018: log(2.0)
;   $1020: sqrt(2.0)

start:
	; sin(1.0) → $1000
	lea     one,a0
	fmove.d (a0),fp0
	fsin.x  fp0
	lea     $1000,a1
	fmove.d fp0,(a1)

	; cos(0.5) → $1008
	lea     half,a0
	fmove.d (a0),fp0
	fcos.x  fp0
	lea     $1008,a1
	fmove.d fp0,(a1)

	; exp(1.0) → $1010
	lea     one,a0
	fmove.d (a0),fp0
	fetox.x fp0
	lea     $1010,a1
	fmove.d fp0,(a1)

	; log(2.0) → $1018
	lea     two,a0
	fmove.d (a0),fp0
	flogn.x fp0
	lea     $1018,a1
	fmove.d fp0,(a1)

	; sqrt(2.0) → $1020
	lea     two,a0
	fmove.d (a0),fp0
	fsqrt.x fp0
	lea     $1020,a1
	fmove.d fp0,(a1)

	rts

one:	dc.d	1.0
half:	dc.d	0.5
two:	dc.d	2.0
