; m68k → IE64 transpiler golden test: straight-line arithmetic.
; Self-contained: no labels referenced from outside, no macro expansion.

start:
	move.l	#0,d0
	move.l	#5,d1
	add.l	d1,d0
	sub.l	#1,d0
	and.l	#$FF,d0
	or.l	#$80,d0
	eor.l	d1,d0
	neg.l	d0
	not.l	d0
	clr.l	d0
done:
	rts
