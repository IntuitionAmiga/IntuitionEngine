
				output	/system.gs

				include	funcdef.i







				include	lvo/dos_lib.i
				include	lvo/exec_lib.i
				include	lvo/graphics_lib.i
				include	lvo/intuition_lib.i
				include	lvo/misc_lib.i
				include	lvo/potgo_lib.i


				include	utility/tagitem.i

				include	workbench/startup.i

CALLEXEC		MACRO
				move.l	4.w,a6
				jsr		_LVO\1(a6)
				ENDM

CALLINT			MACRO
				move.l	_IntuitionBase,a6
				jsr		_LVO\1(a6)
				ENDM

INTNAME			MACRO
				dc.b	'intuition.library',0
				ENDM

CALLGRAF		MACRO
				move.l	_GfxBase,a6
				jsr		_LVO\1(a6)
				ENDM

GRAFNAME		MACRO
				dc.b	'graphics.library',0
				ENDM

CALLDOS			MACRO
				move.l	_DOSBase,a6
				jsr		_LVO\1(a6)
				ENDM

CALLMISC		MACRO
				move.l	_MiscBase,a6
				jsr		_LVO\1(a6)
				ENDM

CALLPOTGO		MACRO
				move.l	_PotgoBase,a6
				jsr		_LVO\1(a6)
				ENDM
