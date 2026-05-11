; Tier-1: shift / rotate coverage.
start:
	move.l	#$80,d0
	lsl.l	#1,d0
	lsr.l	#2,d0
	asl.l	#1,d0
	asr.l	#1,d0
	rol.l	#4,d0
	ror.l	#4,d0
	rts
