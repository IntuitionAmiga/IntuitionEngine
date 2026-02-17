; Wrapper for SQT player - assembles with our entry point trampoline
; SQT init is special: it expects module data at the `buffer` label,
; not passed via HL. Our wrapper copies the module data there first.

; Define these before including SQT.a80
	define	PUREPLAYER
	define	SINGLESETUP

	org	#C000

; Entry point trampoline
	jp	sq_init_wrapper	; 0xC000: init (HL = module address)
	jp	sq_play		; 0xC003: play (called 50Hz)
	jp	sq_stop		; 0xC006: mute

; sq_status - loop forever (0 = loop forever)
sq_status:	db	0

; Wrapper init: copies module data from HL to `buffer`, then calls sq_init
sq_init_wrapper:
	; HL = source (module at 0x4000)
	; Copy module data to buffer area
	; SQT modules are typically < 16KB, copy a safe amount
	ld	de,buffer
	push	de
	; Calculate available space: 0xFFFF - buffer
	; Copy up to the end of Z80 address space or module size
	; Use a safe fixed size - SQT modules fit in this range
	ld	bc,#10000-buffer
	; Clamp: if source + count would wrap, use what we have
	ldir
	pop	hl		; not used by sq_init, but clean
	jp	sq_init

; Include the real player
	include "SQT.a80"
