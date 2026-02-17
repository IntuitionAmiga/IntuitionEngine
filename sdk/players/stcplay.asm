; Wrapper for STC player - assembles with our entry point trampoline

; Define these before including STC.a80
	define	PUREPLAYER
	define	SINGLESETUP

	org	#C000

; Entry point trampoline
	jp	music_init0	; 0xC000: init (HL = module address)
	jp	music_play	; 0xC003: play (called 50Hz)
	jp	music_mute	; 0xC006: mute

; music_setup - loop forever (0 = loop forever)
music_setup:	db	0

; Dummy label for songdata (not used - we call music_init0 directly)
songdata	equ	#4000

; Include the real player
	include "STC.a80"
