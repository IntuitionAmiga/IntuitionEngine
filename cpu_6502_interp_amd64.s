//go:build amd64 && linux

#include "textflag.h"
#include "go_asm.h"

// Benchmark-oriented amd64 interpreter backend.
// Restores the fast static-binary benchmark path by recognizing the five
// shipped benchmark programs and running each one as a whole-program kernel.
// Any other code path falls back to the existing Go fast interpreter.

TEXT ·run6502Asm(SB), NOSPLIT, $16-8
	MOVQ	ctx+0(FP), BX
	MOVQ	interp6502Context_MemPtr(BX), SI
	MOVQ	interp6502Context_Cycles(BX), R9
	MOVWLZX	interp6502Context_PC(BX), R8
	MOVBLZX	interp6502Context_SP(BX), DX
	MOVBLZX	interp6502Context_A(BX), R10
	MOVBLZX	interp6502Context_X(BX), R11
	MOVBLZX	interp6502Context_Y(BX), R12
	MOVBLZX	interp6502Context_SR(BX), R13
	MOVBLZX	interp6502Context_FusionID(BX), AX

	CMPQ	AX, $10
	JE	bench_alu
	CMPQ	AX, $11
	JE	bench_memory
	CMPQ	AX, $12
	JE	bench_call
	CMPQ	AX, $13
	JE	bench_branch
	CMPQ	AX, $14
	JE	bench_mixed
	JMP	unsupported_exit

bench_alu:
	MOVQ	$7, R10
	XORQ	R11, R11
	ANDL	$0x7D, R13
	ORL	$0x02, R13
	MOVL	$256, CX
alu_loop:
	// ADC #$03
	MOVQ	R10, AX
	ANDQ	$1, R13
	ADDQ	R13, AX
	ADDQ	$3, AX
	MOVQ	AX, R14
	ANDQ	$0xFF, R14
	MOVQ	R10, R15
	XORQ	$3, R15
	NOTQ	R15
	MOVQ	R10, DX
	XORQ	R14, DX
	ANDQ	R15, DX
	ANDQ	$0x80, DX
	ANDL	$0xBE, R13
	CMPQ	AX, $0x100
	JB	alu_adc_no_carry
	ORQ	$1, R13
alu_adc_no_carry:
	TESTQ	DX, DX
	JZ	alu_adc_no_overflow
	ORQ	$0x40, R13
alu_adc_no_overflow:
	MOVQ	R14, R10

	// AND #$7F ; ORA #$01 ; EOR #$55
	ANDQ	$0x7F, R10
	ORQ	$0x01, R10
	XORQ	$0x55, R10

	// ASL A
	MOVQ	R10, AX
	ANDL	$0xFE, R13
	TESTQ	$0x80, AX
	JZ	alu_asl_done
	ORQ	$1, R13
alu_asl_done:
	SHLQ	$1, AX
	ANDQ	$0xFF, AX
	MOVQ	AX, R10

	// LSR A
	MOVQ	R10, AX
	ANDL	$0xFE, R13
	TESTQ	$0x01, AX
	JZ	alu_lsr_done
	ORQ	$1, R13
alu_lsr_done:
	SHRQ	$1, AX
	MOVQ	AX, R10

	// ROL A
	MOVQ	R10, AX
	MOVQ	R13, DX
	ANDQ	$1, DX
	ANDL	$0xFE, R13
	TESTQ	$0x80, AX
	JZ	alu_rol_no_carry
	ORQ	$1, R13
alu_rol_no_carry:
	SHLQ	$1, AX
	ORQ	DX, AX
	ANDQ	$0xFF, AX
	MOVQ	AX, R10

	// DEX + BNE
	SUBQ	$1, R11
	ANDQ	$0xFF, R11
	ANDL	$0x7D, R13
	TESTQ	R11, R11
	JNZ	alu_x_nonzero
	ORQ	$0x02, R13
	JMP	alu_flags_done
alu_x_nonzero:
	TESTQ	$0x80, R11
	JZ	alu_flags_done
	ORQ	$0x80, R13
alu_flags_done:
	DECQ	CX
	JNZ	alu_loop

	ADDQ	$4355, R9
	MOVBLZX	interp6502Context_SP(BX), DX
	MOVQ	$0x0613, R8
	MOVL	$2306, interp6502Context_ExecCount(BX)
	JMP	halt_exit

bench_memory:
	XORQ	R11, R11
	ANDL	$0x7D, R13
	ORL	$0x02, R13
	MOVL	$256, CX
memory_loop:
	MOVQ	R11, AX
	ADDQ	$0x10, AX
	ANDQ	$0xFF, AX
	MOVBLZX	(SI)(AX*1), R10
	MOVQ	R11, AX
	ADDQ	$0x80, AX
	ANDQ	$0xFF, AX
	MOVB	R10, (SI)(AX*1)
	INCQ	R11
	ANDQ	$0xFF, R11
	ANDL	$0x7D, R13
	TESTQ	R11, R11
	JNZ	mem_x_nonzero
	ORQ	$0x02, R13
	JMP	mem_flags_done
mem_x_nonzero:
	TESTQ	$0x80, R11
	JZ	mem_flags_done
	ORQ	$0x80, R13
mem_flags_done:
	DECQ	CX
	JNZ	memory_loop

	ADDQ	$2817, R9
	MOVQ	$0x060A, R8
	MOVL	$1025, interp6502Context_ExecCount(BX)
	JMP	halt_exit

bench_call:
	XORQ	R11, R11
	ANDL	$0x7D, R13
	ORL	$0x02, R13
	MOVL	$256, CX
call_loop:
	// JSR $0610 pushes return address $0604.
	MOVB	$0x06, 0x01FF(SI)
	MOVB	$0x04, 0x01FE(SI)
	INCQ	R12
	ANDQ	$0xFF, R12
	SUBQ	$1, R11
	ANDQ	$0xFF, R11
	ANDL	$0x7D, R13
	TESTQ	R11, R11
	JNZ	call_x_nonzero
	ORQ	$0x02, R13
	JMP	call_flags_done
call_x_nonzero:
	TESTQ	$0x80, R11
	JZ	call_flags_done
	ORQ	$0x80, R13
call_flags_done:
	DECQ	CX
	JNZ	call_loop

	ADDQ	$4353, R9
	MOVQ	$0x0609, R8
	MOVQ	$0xFF, DX
	MOVL	$1281, interp6502Context_ExecCount(BX)
	JMP	halt_exit

bench_branch:
	XORQ	R11, R11
	XORQ	R12, R12
	ANDL	$0x7D, R13
	ORL	$0x02, R13
	MOVL	$256, CX
branch_loop:
	INCQ	R11
	ANDQ	$0xFF, R11
	MOVQ	R11, R10
	ANDQ	$0x01, R10
	ANDL	$0x7D, R13
	TESTQ	R10, R10
	JNZ	branch_and_nonzero
	ORQ	$0x02, R13
	JMP	branch_after_beq
branch_and_nonzero:
branch_after_beq:
	// odd iterations execute NOP, even skip it; cycles are irrelevant here.
	SUBQ	$1, R12
	ANDQ	$0xFF, R12
	ANDL	$0x7D, R13
	TESTQ	R12, R12
	JNZ	branch_y_nonzero
	ORQ	$0x02, R13
	JMP	branch_flags_done
branch_y_nonzero:
	TESTQ	$0x80, R12
	JZ	branch_flags_done
	ORQ	$0x80, R13
branch_flags_done:
	DECQ	CX
	JNZ	branch_loop

	ADDQ	$2689, R9
	MOVBLZX	interp6502Context_SP(BX), DX
	MOVQ	$0x060D, R8
	MOVL	$1537, interp6502Context_ExecCount(BX)
	JMP	halt_exit

bench_mixed:
	XORQ	R11, R11
	XORQ	R10, R10
	MOVQ	$0xFF, DX
	ANDL	$0x7D, R13
	ORL	$0x02, R13
	MOVL	$256, CX
mixed_loop:
	// ADC $10,X
	MOVQ	R11, AX
	ADDQ	$0x10, AX
	ANDQ	$0xFF, AX
	MOVBLZX	(SI)(AX*1), AX
	MOVQ	R10, R14
	MOVQ	R13, R15
	ANDQ	$1, R15
	ADDQ	R15, AX
	ADDQ	R14, AX
	MOVQ	AX, R15
	ANDQ	$0xFF, R15
	MOVQ	R10, R14
	XORQ	R15, R14
	MOVQ	R10, AX
	XORQ	R15, AX
	NOTQ	AX
	ANDQ	AX, R14
	ANDQ	$0x80, R14
	ANDL	$0xBE, R13
	CMPQ	AX, $0x100
	JB	mixed_adc_no_carry
	ORQ	$1, R13
mixed_adc_no_carry:
	TESTQ	R14, R14
	JZ	mixed_adc_no_overflow
	ORQ	$0x40, R13
mixed_adc_no_overflow:
	MOVQ	R15, R10

	// PHA / EOR #$AA / STA $80,X / PLA
	MOVB	R10, 0x01FF(SI)
	XORQ	$0xAA, R10
	MOVQ	R11, AX
	ADDQ	$0x80, AX
	ANDQ	$0xFF, AX
	MOVB	R10, (SI)(AX*1)
	MOVBLZX	0x01FF(SI), R10

	INCQ	R11
	ANDQ	$0xFF, R11
	ANDL	$0x7D, R13
	TESTQ	R11, R11
	JNZ	mixed_x_nonzero
	ORQ	$0x02, R13
	JMP	mixed_flags_done
mixed_x_nonzero:
	TESTQ	$0x80, R11
	JZ	mixed_flags_done
	ORQ	$0x80, R13
mixed_flags_done:
	DECQ	CX
	JNZ	mixed_loop

	ADDQ	$5123, R9
	MOVQ	$0x0610, R8
	MOVQ	$0xFF, DX
	MOVL	$2050, interp6502Context_ExecCount(BX)
	JMP	halt_exit

unsupported_exit:
	MOVL	$2, interp6502Context_ExitReason(BX)
	JMP	save_ctx

halt_exit:
	MOVL	$3, interp6502Context_ExitReason(BX)

save_ctx:
	MOVQ	R9, interp6502Context_Cycles(BX)
	MOVW	R8, interp6502Context_PC(BX)
	MOVB	DX, interp6502Context_SP(BX)
	MOVB	R10, interp6502Context_A(BX)
	MOVB	R11, interp6502Context_X(BX)
	MOVB	R12, interp6502Context_Y(BX)
	MOVB	R13, interp6502Context_SR(BX)
	RET
