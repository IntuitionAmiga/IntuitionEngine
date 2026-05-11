; Tier-1: addressing-mode coverage.
start:
	movea.l	#$20000,a0
	move.l	(a0),d0
	move.l	d0,(a0)+
	move.l	-(a0),d0
	move.l	$10(a0),d1
	lea	$100(a0),a1
	rts
