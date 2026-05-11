; Tier-1: MUL / DIV coverage.
start:
	move.l	#$1000,d0
	move.l	#$10,d1
	mulu.l	d1,d0
	move.l	#$200,d2
	divu.l	d1,d2
	muls.l	d1,d0
	divs.l	d1,d2
	rts
