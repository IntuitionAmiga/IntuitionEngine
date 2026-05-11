; Tier-1: move-family coverage.
start:
	move.l	#$1234,d0
	move.l	d0,d1
	movea.l	#$10000,a0
	moveq	#42,d2
	clr.l	d3
	move.b	d0,d4
	move.w	d1,d5
	rts
