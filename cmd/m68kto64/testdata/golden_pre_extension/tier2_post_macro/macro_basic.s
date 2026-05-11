; Tier-2 fixture: macro/endm/rept/endr — Phase A→D byte-identity guard.
; Expected to drift in Phase E when macros are transpile-time-expanded.

PUSHALL macro
	movem.l	d0-d7/a0-a6,-(sp)
	endm

POPALL macro
	movem.l	(sp)+,d0-d7/a0-a6
	endm

start:
	PUSHALL
	rept 3
	nop
	endr
	POPALL
	rts
