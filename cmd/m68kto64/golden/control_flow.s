; Control-flow golden: BRA/BSR/RTS/JMP/labels + adjacent-fuse Bcc.

start:
	move.l	#10,d0
loop:
	sub.l	#1,d0
	cmp.l	#0,d0
	bne	loop
	bsr	helper
	bra	done
helper:
	move.l	#1,d1
	rts
done:
	rts
