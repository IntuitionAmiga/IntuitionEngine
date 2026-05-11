; Tier-3 fixture: ifd IS_IE / ifnd IS_IE — must stay byte-identical across all
; phases under Model A (gate-only) since the IS_IE seed and lowered if 1/if 0
; output shape is preserved.

start:
	ifd	IS_IE
	move.l	#1,d0
	endc
	ifnd	IS_IE
	move.l	#0,d0
	endc
	rts
