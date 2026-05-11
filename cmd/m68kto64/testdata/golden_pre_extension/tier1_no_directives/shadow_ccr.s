; Shadow-CCR golden: separated CMP/TST + Bcc with intervening op.

start:
	move.l	#10,d0
	tst.l	d0
	move.l	#$1234,d1   ; intervening op — forces shadow-CCR path
	beq	target
	cmp.l	#5,d0
	move.l	#$5678,d2
	blt	other
target:
	rts
other:
	rts
