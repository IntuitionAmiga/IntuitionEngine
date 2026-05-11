; Tier-1: AND/OR/EOR/NOT/NEG.
start:
	move.l	#$ff00,d0
	and.l	#$f0f0,d0
	or.l	#$0001,d0
	eor.l	#$ffff,d0
	not.l	d0
	neg.l	d0
	rts
