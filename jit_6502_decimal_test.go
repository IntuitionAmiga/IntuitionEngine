package main

import "testing"

func TestP65DecimalTablesMatchInterpreter(t *testing.T) {
	for carry := byte(0); carry < 2; carry++ {
		for a := 0; a < 256; a++ {
			for operand := 0; operand < 256; operand++ {
				index := a | operand<<8 | int(carry)<<16
				adc := &CPU_6502{A: byte(a), SR: DECIMAL_FLAG | carry}
				adc.adc(byte(operand))
				wantADC := p65DecimalResult{adc.A, adc.SR & (CARRY_FLAG | OVERFLOW_FLAG | NEGATIVE_FLAG | ZERO_FLAG)}
				if got := p65DecimalADC[index]; got != wantADC {
					t.Fatalf("ADC table a=%02X operand=%02X c=%d: got=%+v want=%+v", a, operand, carry, got, wantADC)
				}
				sbc := &CPU_6502{A: byte(a), SR: DECIMAL_FLAG | carry}
				sbc.sbc(byte(operand))
				wantSBC := p65DecimalResult{sbc.A, sbc.SR & (CARRY_FLAG | OVERFLOW_FLAG | NEGATIVE_FLAG | ZERO_FLAG)}
				if got := p65DecimalSBC[index]; got != wantSBC {
					t.Fatalf("SBC table a=%02X operand=%02X c=%d: got=%+v want=%+v", a, operand, carry, got, wantSBC)
				}
				binaryADC := &CPU_6502{A: byte(a), SR: carry}
				binaryADC.adc(byte(operand))
				wantBinaryADC := p65DecimalResult{binaryADC.A, binaryADC.SR & (CARRY_FLAG | OVERFLOW_FLAG | NEGATIVE_FLAG | ZERO_FLAG)}
				if got := p65BinaryADC[index]; got != wantBinaryADC {
					t.Fatalf("binary ADC table a=%02X operand=%02X c=%d: got=%+v want=%+v", a, operand, carry, got, wantBinaryADC)
				}
				binarySBC := &CPU_6502{A: byte(a), SR: carry}
				binarySBC.sbc(byte(operand))
				wantBinarySBC := p65DecimalResult{binarySBC.A, binarySBC.SR & (CARRY_FLAG | OVERFLOW_FLAG | NEGATIVE_FLAG | ZERO_FLAG)}
				if got := p65BinarySBC[index]; got != wantBinarySBC {
					t.Fatalf("binary SBC table a=%02X operand=%02X c=%d: got=%+v want=%+v", a, operand, carry, got, wantBinarySBC)
				}
			}
		}
	}
}
