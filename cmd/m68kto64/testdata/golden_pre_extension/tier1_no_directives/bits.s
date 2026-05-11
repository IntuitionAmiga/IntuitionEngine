; Tier-1: bit-test coverage. (bset/bclr/bchg deliberately omitted — they are
; recognised by the lexer but have no lowering, so ConvertSource passthrough
; and ConvertFile Werror error path diverge.)
start:
	move.l	#$80,d0
	btst	#3,d0
	btst	#5,d0
	btst	#0,d0
	rts
