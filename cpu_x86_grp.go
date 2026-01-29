// cpu_x86_grp.go - x86 CPU Group Opcode Implementations (Grp1-5, shifts, multiply/divide)
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

package main

// =============================================================================
// Group 1 (ADD, OR, ADC, SBB, AND, SUB, XOR, CMP)
// =============================================================================

func (c *CPU_X86) opGrp1_Eb_Ib() {
	c.fetchModRM()
	op := c.getModRMReg()
	a := c.readRM8()
	b := c.fetch8()

	switch op {
	case 0: // ADD
		result := uint16(a) + uint16(b)
		c.setFlagsArith8(result, a, b, false)
		c.writeRM8(byte(result))
	case 1: // OR
		result := a | b
		c.setFlagsLogic8(result)
		c.writeRM8(result)
	case 2: // ADC
		var carry byte
		if c.CF() {
			carry = 1
		}
		result := uint16(a) + uint16(b) + uint16(carry)
		c.setFlagsArith8(result, a, b+carry, false)
		c.writeRM8(byte(result))
	case 3: // SBB
		var borrow byte
		if c.CF() {
			borrow = 1
		}
		result := uint16(a) - uint16(b) - uint16(borrow)
		c.setFlagsArith8(result, a, b+borrow, true)
		c.writeRM8(byte(result))
	case 4: // AND
		result := a & b
		c.setFlagsLogic8(result)
		c.writeRM8(result)
	case 5: // SUB
		result := uint16(a) - uint16(b)
		c.setFlagsArith8(result, a, b, true)
		c.writeRM8(byte(result))
	case 6: // XOR
		result := a ^ b
		c.setFlagsLogic8(result)
		c.writeRM8(result)
	case 7: // CMP
		result := uint16(a) - uint16(b)
		c.setFlagsArith8(result, a, b, true)
	}
	c.Cycles += 2
}

func (c *CPU_X86) opGrp1_Ev_Iv() {
	c.fetchModRM()
	op := c.getModRMReg()

	if c.prefixOpSize {
		a := c.readRM16()
		b := c.fetch16()

		switch op {
		case 0: // ADD
			result := uint32(a) + uint32(b)
			c.setFlagsArith16(result, a, b, false)
			c.writeRM16(uint16(result))
		case 1: // OR
			result := a | b
			c.setFlagsLogic16(result)
			c.writeRM16(result)
		case 2: // ADC
			var carry uint16
			if c.CF() {
				carry = 1
			}
			result := uint32(a) + uint32(b) + uint32(carry)
			c.setFlagsArith16(result, a, b+carry, false)
			c.writeRM16(uint16(result))
		case 3: // SBB
			var borrow uint16
			if c.CF() {
				borrow = 1
			}
			result := uint32(a) - uint32(b) - uint32(borrow)
			c.setFlagsArith16(result, a, b+borrow, true)
			c.writeRM16(uint16(result))
		case 4: // AND
			result := a & b
			c.setFlagsLogic16(result)
			c.writeRM16(result)
		case 5: // SUB
			result := uint32(a) - uint32(b)
			c.setFlagsArith16(result, a, b, true)
			c.writeRM16(uint16(result))
		case 6: // XOR
			result := a ^ b
			c.setFlagsLogic16(result)
			c.writeRM16(result)
		case 7: // CMP
			result := uint32(a) - uint32(b)
			c.setFlagsArith16(result, a, b, true)
		}
	} else {
		a := c.readRM32()
		b := c.fetch32()

		switch op {
		case 0: // ADD
			result := uint64(a) + uint64(b)
			c.setFlagsArith32(result, a, b, false)
			c.writeRM32(uint32(result))
		case 1: // OR
			result := a | b
			c.setFlagsLogic32(result)
			c.writeRM32(result)
		case 2: // ADC
			var carry uint32
			if c.CF() {
				carry = 1
			}
			result := uint64(a) + uint64(b) + uint64(carry)
			c.setFlagsArith32(result, a, b+carry, false)
			c.writeRM32(uint32(result))
		case 3: // SBB
			var borrow uint32
			if c.CF() {
				borrow = 1
			}
			result := uint64(a) - uint64(b) - uint64(borrow)
			c.setFlagsArith32(result, a, b+borrow, true)
			c.writeRM32(uint32(result))
		case 4: // AND
			result := a & b
			c.setFlagsLogic32(result)
			c.writeRM32(result)
		case 5: // SUB
			result := uint64(a) - uint64(b)
			c.setFlagsArith32(result, a, b, true)
			c.writeRM32(uint32(result))
		case 6: // XOR
			result := a ^ b
			c.setFlagsLogic32(result)
			c.writeRM32(result)
		case 7: // CMP
			result := uint64(a) - uint64(b)
			c.setFlagsArith32(result, a, b, true)
		}
	}
	c.Cycles += 2
}

func (c *CPU_X86) opGrp1_Ev_Ib() {
	c.fetchModRM()
	op := c.getModRMReg()
	// Sign-extended immediate
	imm := int8(c.fetch8())

	if c.prefixOpSize {
		a := c.readRM16()
		b := uint16(int16(imm))

		switch op {
		case 0: // ADD
			result := uint32(a) + uint32(b)
			c.setFlagsArith16(result, a, b, false)
			c.writeRM16(uint16(result))
		case 1: // OR
			result := a | b
			c.setFlagsLogic16(result)
			c.writeRM16(result)
		case 2: // ADC
			var carry uint16
			if c.CF() {
				carry = 1
			}
			result := uint32(a) + uint32(b) + uint32(carry)
			c.setFlagsArith16(result, a, b+carry, false)
			c.writeRM16(uint16(result))
		case 3: // SBB
			var borrow uint16
			if c.CF() {
				borrow = 1
			}
			result := uint32(a) - uint32(b) - uint32(borrow)
			c.setFlagsArith16(result, a, b+borrow, true)
			c.writeRM16(uint16(result))
		case 4: // AND
			result := a & b
			c.setFlagsLogic16(result)
			c.writeRM16(result)
		case 5: // SUB
			result := uint32(a) - uint32(b)
			c.setFlagsArith16(result, a, b, true)
			c.writeRM16(uint16(result))
		case 6: // XOR
			result := a ^ b
			c.setFlagsLogic16(result)
			c.writeRM16(result)
		case 7: // CMP
			result := uint32(a) - uint32(b)
			c.setFlagsArith16(result, a, b, true)
		}
	} else {
		a := c.readRM32()
		b := uint32(int32(imm))

		switch op {
		case 0: // ADD
			result := uint64(a) + uint64(b)
			c.setFlagsArith32(result, a, b, false)
			c.writeRM32(uint32(result))
		case 1: // OR
			result := a | b
			c.setFlagsLogic32(result)
			c.writeRM32(result)
		case 2: // ADC
			var carry uint32
			if c.CF() {
				carry = 1
			}
			result := uint64(a) + uint64(b) + uint64(carry)
			c.setFlagsArith32(result, a, b+carry, false)
			c.writeRM32(uint32(result))
		case 3: // SBB
			var borrow uint32
			if c.CF() {
				borrow = 1
			}
			result := uint64(a) - uint64(b) - uint64(borrow)
			c.setFlagsArith32(result, a, b+borrow, true)
			c.writeRM32(uint32(result))
		case 4: // AND
			result := a & b
			c.setFlagsLogic32(result)
			c.writeRM32(result)
		case 5: // SUB
			result := uint64(a) - uint64(b)
			c.setFlagsArith32(result, a, b, true)
			c.writeRM32(uint32(result))
		case 6: // XOR
			result := a ^ b
			c.setFlagsLogic32(result)
			c.writeRM32(result)
		case 7: // CMP
			result := uint64(a) - uint64(b)
			c.setFlagsArith32(result, a, b, true)
		}
	}
	c.Cycles += 2
}

// =============================================================================
// Group 2 (ROL, ROR, RCL, RCR, SHL, SHR, SAL, SAR)
// =============================================================================

func (c *CPU_X86) shiftRotate8(val byte, count byte, op byte) byte {
	if count == 0 {
		return val
	}

	count &= 0x1F // Mask to 5 bits
	if count == 0 {
		return val
	}

	var result byte
	switch op {
	case 0: // ROL
		count &= 7
		result = (val << count) | (val >> (8 - count))
		c.setFlag(x86FlagCF, (result&1) != 0)
		if count == 1 {
			c.setFlag(x86FlagOF, ((result>>7)^(result&1)) != 0)
		}
	case 1: // ROR
		count &= 7
		result = (val >> count) | (val << (8 - count))
		c.setFlag(x86FlagCF, (result&0x80) != 0)
		if count == 1 {
			c.setFlag(x86FlagOF, ((result>>7)^((result>>6)&1)) != 0)
		}
	case 2: // RCL
		count %= 9
		var cf byte
		if c.CF() {
			cf = 1
		}
		for i := byte(0); i < count; i++ {
			newCF := val >> 7
			val = (val << 1) | cf
			cf = newCF
		}
		result = val
		c.setFlag(x86FlagCF, cf != 0)
		if count == 1 {
			c.setFlag(x86FlagOF, ((result>>7)^cf) != 0)
		}
	case 3: // RCR
		count %= 9
		var cf byte
		if c.CF() {
			cf = 1
		}
		for i := byte(0); i < count; i++ {
			newCF := val & 1
			val = (val >> 1) | (cf << 7)
			cf = newCF
		}
		result = val
		c.setFlag(x86FlagCF, cf != 0)
		if count == 1 {
			c.setFlag(x86FlagOF, ((result>>7)^((result>>6)&1)) != 0)
		}
	case 4, 6: // SHL/SAL
		c.setFlag(x86FlagCF, ((val>>(8-count))&1) != 0)
		result = val << count
		if count == 1 {
			c.setFlag(x86FlagOF, ((result>>7)^(val>>7)) != 0)
		}
		c.setFlag(x86FlagSF, (result&0x80) != 0)
		c.setFlag(x86FlagZF, result == 0)
		c.setFlag(x86FlagPF, parity(result))
	case 5: // SHR
		c.setFlag(x86FlagCF, ((val>>(count-1))&1) != 0)
		result = val >> count
		if count == 1 {
			c.setFlag(x86FlagOF, (val&0x80) != 0)
		}
		c.setFlag(x86FlagSF, (result&0x80) != 0)
		c.setFlag(x86FlagZF, result == 0)
		c.setFlag(x86FlagPF, parity(result))
	case 7: // SAR
		c.setFlag(x86FlagCF, ((val>>(count-1))&1) != 0)
		result = byte(int8(val) >> count)
		if count == 1 {
			c.setFlag(x86FlagOF, false)
		}
		c.setFlag(x86FlagSF, (result&0x80) != 0)
		c.setFlag(x86FlagZF, result == 0)
		c.setFlag(x86FlagPF, parity(result))
	}
	return result
}

func (c *CPU_X86) shiftRotate16(val uint16, count byte, op byte) uint16 {
	if count == 0 {
		return val
	}

	count &= 0x1F
	if count == 0 {
		return val
	}

	var result uint16
	switch op {
	case 0: // ROL
		count &= 15
		result = (val << count) | (val >> (16 - count))
		c.setFlag(x86FlagCF, (result&1) != 0)
		if count == 1 {
			c.setFlag(x86FlagOF, ((result>>15)^(result&1)) != 0)
		}
	case 1: // ROR
		count &= 15
		result = (val >> count) | (val << (16 - count))
		c.setFlag(x86FlagCF, (result&0x8000) != 0)
		if count == 1 {
			c.setFlag(x86FlagOF, ((result>>15)^((result>>14)&1)) != 0)
		}
	case 2: // RCL
		count %= 17
		var cf uint16
		if c.CF() {
			cf = 1
		}
		for i := byte(0); i < count; i++ {
			newCF := val >> 15
			val = (val << 1) | cf
			cf = newCF
		}
		result = val
		c.setFlag(x86FlagCF, cf != 0)
		if count == 1 {
			c.setFlag(x86FlagOF, ((result>>15)^cf) != 0)
		}
	case 3: // RCR
		count %= 17
		var cf uint16
		if c.CF() {
			cf = 1
		}
		for i := byte(0); i < count; i++ {
			newCF := val & 1
			val = (val >> 1) | (cf << 15)
			cf = newCF
		}
		result = val
		c.setFlag(x86FlagCF, cf != 0)
		if count == 1 {
			c.setFlag(x86FlagOF, ((result>>15)^((result>>14)&1)) != 0)
		}
	case 4, 6: // SHL/SAL
		if count >= 16 {
			c.setFlag(x86FlagCF, false)
			result = 0
		} else {
			c.setFlag(x86FlagCF, ((val>>(16-count))&1) != 0)
			result = val << count
		}
		if count == 1 {
			c.setFlag(x86FlagOF, ((result>>15)^(val>>15)) != 0)
		}
		c.setFlag(x86FlagSF, (result&0x8000) != 0)
		c.setFlag(x86FlagZF, result == 0)
		c.setFlag(x86FlagPF, parity(byte(result)))
	case 5: // SHR
		if count >= 16 {
			c.setFlag(x86FlagCF, false)
			result = 0
		} else {
			c.setFlag(x86FlagCF, ((val>>(count-1))&1) != 0)
			result = val >> count
		}
		if count == 1 {
			c.setFlag(x86FlagOF, (val&0x8000) != 0)
		}
		c.setFlag(x86FlagSF, (result&0x8000) != 0)
		c.setFlag(x86FlagZF, result == 0)
		c.setFlag(x86FlagPF, parity(byte(result)))
	case 7: // SAR
		if count >= 16 {
			if val&0x8000 != 0 {
				result = 0xFFFF
				c.setFlag(x86FlagCF, true)
			} else {
				result = 0
				c.setFlag(x86FlagCF, false)
			}
		} else {
			c.setFlag(x86FlagCF, ((val>>(count-1))&1) != 0)
			result = uint16(int16(val) >> count)
		}
		if count == 1 {
			c.setFlag(x86FlagOF, false)
		}
		c.setFlag(x86FlagSF, (result&0x8000) != 0)
		c.setFlag(x86FlagZF, result == 0)
		c.setFlag(x86FlagPF, parity(byte(result)))
	}
	return result
}

func (c *CPU_X86) shiftRotate32(val uint32, count byte, op byte) uint32 {
	if count == 0 {
		return val
	}

	count &= 0x1F
	if count == 0 {
		return val
	}

	var result uint32
	switch op {
	case 0: // ROL
		result = (val << count) | (val >> (32 - count))
		c.setFlag(x86FlagCF, (result&1) != 0)
		if count == 1 {
			c.setFlag(x86FlagOF, ((result>>31)^(result&1)) != 0)
		}
	case 1: // ROR
		result = (val >> count) | (val << (32 - count))
		c.setFlag(x86FlagCF, (result&0x80000000) != 0)
		if count == 1 {
			c.setFlag(x86FlagOF, ((result>>31)^((result>>30)&1)) != 0)
		}
	case 2: // RCL
		var cf uint32
		if c.CF() {
			cf = 1
		}
		for i := byte(0); i < count; i++ {
			newCF := val >> 31
			val = (val << 1) | cf
			cf = newCF
		}
		result = val
		c.setFlag(x86FlagCF, cf != 0)
		if count == 1 {
			c.setFlag(x86FlagOF, ((result>>31)^cf) != 0)
		}
	case 3: // RCR
		var cf uint32
		if c.CF() {
			cf = 1
		}
		for i := byte(0); i < count; i++ {
			newCF := val & 1
			val = (val >> 1) | (cf << 31)
			cf = newCF
		}
		result = val
		c.setFlag(x86FlagCF, cf != 0)
		if count == 1 {
			c.setFlag(x86FlagOF, ((result>>31)^((result>>30)&1)) != 0)
		}
	case 4, 6: // SHL/SAL
		c.setFlag(x86FlagCF, ((val>>(32-count))&1) != 0)
		result = val << count
		if count == 1 {
			c.setFlag(x86FlagOF, ((result>>31)^(val>>31)) != 0)
		}
		c.setFlag(x86FlagSF, (result&0x80000000) != 0)
		c.setFlag(x86FlagZF, result == 0)
		c.setFlag(x86FlagPF, parity(byte(result)))
	case 5: // SHR
		c.setFlag(x86FlagCF, ((val>>(count-1))&1) != 0)
		result = val >> count
		if count == 1 {
			c.setFlag(x86FlagOF, (val&0x80000000) != 0)
		}
		c.setFlag(x86FlagSF, (result&0x80000000) != 0)
		c.setFlag(x86FlagZF, result == 0)
		c.setFlag(x86FlagPF, parity(byte(result)))
	case 7: // SAR
		c.setFlag(x86FlagCF, ((val>>(count-1))&1) != 0)
		result = uint32(int32(val) >> count)
		if count == 1 {
			c.setFlag(x86FlagOF, false)
		}
		c.setFlag(x86FlagSF, (result&0x80000000) != 0)
		c.setFlag(x86FlagZF, result == 0)
		c.setFlag(x86FlagPF, parity(byte(result)))
	}
	return result
}

func (c *CPU_X86) opGrp2_Eb_1() {
	c.fetchModRM()
	op := c.getModRMReg()
	val := c.readRM8()
	c.writeRM8(c.shiftRotate8(val, 1, op))
	c.Cycles += 2
}

func (c *CPU_X86) opGrp2_Ev_1() {
	c.fetchModRM()
	op := c.getModRMReg()
	if c.prefixOpSize {
		val := c.readRM16()
		c.writeRM16(c.shiftRotate16(val, 1, op))
	} else {
		val := c.readRM32()
		c.writeRM32(c.shiftRotate32(val, 1, op))
	}
	c.Cycles += 2
}

func (c *CPU_X86) opGrp2_Eb_CL() {
	c.fetchModRM()
	op := c.getModRMReg()
	val := c.readRM8()
	c.writeRM8(c.shiftRotate8(val, c.CL(), op))
	c.Cycles += 3
}

func (c *CPU_X86) opGrp2_Ev_CL() {
	c.fetchModRM()
	op := c.getModRMReg()
	if c.prefixOpSize {
		val := c.readRM16()
		c.writeRM16(c.shiftRotate16(val, c.CL(), op))
	} else {
		val := c.readRM32()
		c.writeRM32(c.shiftRotate32(val, c.CL(), op))
	}
	c.Cycles += 3
}

func (c *CPU_X86) opGrp2_Eb_Ib() {
	c.fetchModRM()
	op := c.getModRMReg()
	val := c.readRM8()
	count := c.fetch8()
	c.writeRM8(c.shiftRotate8(val, count, op))
	c.Cycles += 3
}

func (c *CPU_X86) opGrp2_Ev_Ib() {
	c.fetchModRM()
	op := c.getModRMReg()
	if c.prefixOpSize {
		val := c.readRM16()
		count := c.fetch8()
		c.writeRM16(c.shiftRotate16(val, count, op))
	} else {
		val := c.readRM32()
		count := c.fetch8()
		c.writeRM32(c.shiftRotate32(val, count, op))
	}
	c.Cycles += 3
}

// =============================================================================
// Group 3 (TEST, NOT, NEG, MUL, IMUL, DIV, IDIV)
// =============================================================================

func (c *CPU_X86) opGrp3_Eb() {
	c.fetchModRM()
	op := c.getModRMReg()
	val := c.readRM8()

	switch op {
	case 0, 1: // TEST Eb,Ib
		imm := c.fetch8()
		result := val & imm
		c.setFlagsLogic8(result)
	case 2: // NOT
		c.writeRM8(^val)
	case 3: // NEG
		result := uint16(0) - uint16(val)
		c.setFlagsArith8(result, 0, val, true)
		c.setFlag(x86FlagCF, val != 0)
		c.writeRM8(byte(result))
	case 4: // MUL
		result := uint16(c.AL()) * uint16(val)
		c.SetAX(result)
		c.setFlag(x86FlagCF, c.AH() != 0)
		c.setFlag(x86FlagOF, c.AH() != 0)
	case 5: // IMUL
		result := int16(int8(c.AL())) * int16(int8(val))
		c.SetAX(uint16(result))
		// CF/OF set if high byte is not sign extension of low byte
		signExt := int16(int8(byte(result)))
		c.setFlag(x86FlagCF, result != signExt)
		c.setFlag(x86FlagOF, result != signExt)
	case 6: // DIV
		if val == 0 {
			c.handleInterrupt(0) // Division by zero
			return
		}
		dividend := c.AX()
		quotient := dividend / uint16(val)
		remainder := dividend % uint16(val)
		if quotient > 0xFF {
			c.handleInterrupt(0) // Overflow
			return
		}
		c.SetAL(byte(quotient))
		c.SetAH(byte(remainder))
	case 7: // IDIV
		if val == 0 {
			c.handleInterrupt(0)
			return
		}
		dividend := int16(c.AX())
		divisor := int16(int8(val))
		quotient := dividend / divisor
		remainder := dividend % divisor
		if quotient > 127 || quotient < -128 {
			c.handleInterrupt(0)
			return
		}
		c.SetAL(byte(quotient))
		c.SetAH(byte(remainder))
	}
	c.Cycles += 5
}

func (c *CPU_X86) opGrp3_Ev() {
	c.fetchModRM()
	op := c.getModRMReg()

	if c.prefixOpSize {
		val := c.readRM16()
		switch op {
		case 0, 1: // TEST Ev,Iv
			imm := c.fetch16()
			result := val & imm
			c.setFlagsLogic16(result)
		case 2: // NOT
			c.writeRM16(^val)
		case 3: // NEG
			result := uint32(0) - uint32(val)
			c.setFlagsArith16(result, 0, val, true)
			c.setFlag(x86FlagCF, val != 0)
			c.writeRM16(uint16(result))
		case 4: // MUL
			result := uint32(c.AX()) * uint32(val)
			c.SetAX(uint16(result))
			c.SetDX(uint16(result >> 16))
			c.setFlag(x86FlagCF, c.DX() != 0)
			c.setFlag(x86FlagOF, c.DX() != 0)
		case 5: // IMUL
			result := int32(int16(c.AX())) * int32(int16(val))
			c.SetAX(uint16(result))
			c.SetDX(uint16(result >> 16))
			signExt := int32(int16(uint16(result)))
			c.setFlag(x86FlagCF, result != signExt)
			c.setFlag(x86FlagOF, result != signExt)
		case 6: // DIV
			if val == 0 {
				c.handleInterrupt(0)
				return
			}
			dividend := (uint32(c.DX()) << 16) | uint32(c.AX())
			quotient := dividend / uint32(val)
			remainder := dividend % uint32(val)
			if quotient > 0xFFFF {
				c.handleInterrupt(0)
				return
			}
			c.SetAX(uint16(quotient))
			c.SetDX(uint16(remainder))
		case 7: // IDIV
			if val == 0 {
				c.handleInterrupt(0)
				return
			}
			dividend := int32((uint32(c.DX()) << 16) | uint32(c.AX()))
			divisor := int32(int16(val))
			quotient := dividend / divisor
			remainder := dividend % divisor
			if quotient > 32767 || quotient < -32768 {
				c.handleInterrupt(0)
				return
			}
			c.SetAX(uint16(quotient))
			c.SetDX(uint16(remainder))
		}
	} else {
		val := c.readRM32()
		switch op {
		case 0, 1: // TEST Ev,Iv
			imm := c.fetch32()
			result := val & imm
			c.setFlagsLogic32(result)
		case 2: // NOT
			c.writeRM32(^val)
		case 3: // NEG
			result := uint64(0) - uint64(val)
			c.setFlagsArith32(result, 0, val, true)
			c.setFlag(x86FlagCF, val != 0)
			c.writeRM32(uint32(result))
		case 4: // MUL
			result := uint64(c.EAX) * uint64(val)
			c.EAX = uint32(result)
			c.EDX = uint32(result >> 32)
			c.setFlag(x86FlagCF, c.EDX != 0)
			c.setFlag(x86FlagOF, c.EDX != 0)
		case 5: // IMUL
			result := int64(int32(c.EAX)) * int64(int32(val))
			c.EAX = uint32(result)
			c.EDX = uint32(result >> 32)
			signExt := int64(int32(uint32(result)))
			c.setFlag(x86FlagCF, result != signExt)
			c.setFlag(x86FlagOF, result != signExt)
		case 6: // DIV
			if val == 0 {
				c.handleInterrupt(0)
				return
			}
			dividend := (uint64(c.EDX) << 32) | uint64(c.EAX)
			quotient := dividend / uint64(val)
			remainder := dividend % uint64(val)
			if quotient > 0xFFFFFFFF {
				c.handleInterrupt(0)
				return
			}
			c.EAX = uint32(quotient)
			c.EDX = uint32(remainder)
		case 7: // IDIV
			if val == 0 {
				c.handleInterrupt(0)
				return
			}
			dividend := int64((uint64(c.EDX) << 32) | uint64(c.EAX))
			divisor := int64(int32(val))
			quotient := dividend / divisor
			remainder := dividend % divisor
			if quotient > 2147483647 || quotient < -2147483648 {
				c.handleInterrupt(0)
				return
			}
			c.EAX = uint32(quotient)
			c.EDX = uint32(remainder)
		}
	}
	c.Cycles += 10
}

// =============================================================================
// IMUL (multi-operand forms)
// =============================================================================

func (c *CPU_X86) opIMUL_Gv_Ev() {
	c.fetchModRM()
	if c.prefixOpSize {
		a := int32(int16(c.getReg16(c.getModRMReg())))
		b := int32(int16(c.readRM16()))
		result := a * b
		c.setReg16(c.getModRMReg(), uint16(result))
		signExt := int32(int16(uint16(result)))
		c.setFlag(x86FlagCF, result != signExt)
		c.setFlag(x86FlagOF, result != signExt)
	} else {
		a := int64(int32(c.getReg32(c.getModRMReg())))
		b := int64(int32(c.readRM32()))
		result := a * b
		c.setReg32(c.getModRMReg(), uint32(result))
		signExt := int64(int32(uint32(result)))
		c.setFlag(x86FlagCF, result != signExt)
		c.setFlag(x86FlagOF, result != signExt)
	}
	c.Cycles += 10
}

func (c *CPU_X86) opIMUL_Gv_Ev_Iv() {
	c.fetchModRM()
	if c.prefixOpSize {
		a := int32(int16(c.readRM16()))
		b := int32(int16(c.fetch16()))
		result := a * b
		c.setReg16(c.getModRMReg(), uint16(result))
		signExt := int32(int16(uint16(result)))
		c.setFlag(x86FlagCF, result != signExt)
		c.setFlag(x86FlagOF, result != signExt)
	} else {
		a := int64(int32(c.readRM32()))
		b := int64(int32(c.fetch32()))
		result := a * b
		c.setReg32(c.getModRMReg(), uint32(result))
		signExt := int64(int32(uint32(result)))
		c.setFlag(x86FlagCF, result != signExt)
		c.setFlag(x86FlagOF, result != signExt)
	}
	c.Cycles += 10
}

func (c *CPU_X86) opIMUL_Gv_Ev_Ib() {
	c.fetchModRM()
	if c.prefixOpSize {
		a := int32(int16(c.readRM16()))
		b := int32(int8(c.fetch8()))
		result := a * b
		c.setReg16(c.getModRMReg(), uint16(result))
		signExt := int32(int16(uint16(result)))
		c.setFlag(x86FlagCF, result != signExt)
		c.setFlag(x86FlagOF, result != signExt)
	} else {
		a := int64(int32(c.readRM32()))
		b := int64(int8(c.fetch8()))
		result := a * b
		c.setReg32(c.getModRMReg(), uint32(result))
		signExt := int64(int32(uint32(result)))
		c.setFlag(x86FlagCF, result != signExt)
		c.setFlag(x86FlagOF, result != signExt)
	}
	c.Cycles += 10
}

// =============================================================================
// Group 4 (INC/DEC Eb)
// =============================================================================

func (c *CPU_X86) opGrp4_Eb() {
	c.fetchModRM()
	op := c.getModRMReg()
	val := c.readRM8()

	cf := c.CF() // Save CF

	switch op {
	case 0: // INC
		result := uint16(val) + 1
		c.setFlagsArith8(result, val, 1, false)
		c.writeRM8(byte(result))
	case 1: // DEC
		result := uint16(val) - 1
		c.setFlagsArith8(result, val, 1, true)
		c.writeRM8(byte(result))
	default:
		c.Halted = true // Undefined
	}

	c.setFlag(x86FlagCF, cf) // Restore CF
	c.Cycles += 2
}

// =============================================================================
// Group 5 (INC, DEC, CALL, CALL far, JMP, JMP far, PUSH)
// =============================================================================

func (c *CPU_X86) opGrp5_Ev() {
	c.fetchModRM()
	op := c.getModRMReg()

	switch op {
	case 0: // INC Ev
		cf := c.CF()
		if c.prefixOpSize {
			val := c.readRM16()
			result := uint32(val) + 1
			c.setFlagsArith16(result, val, 1, false)
			c.writeRM16(uint16(result))
		} else {
			val := c.readRM32()
			result := uint64(val) + 1
			c.setFlagsArith32(result, val, 1, false)
			c.writeRM32(uint32(result))
		}
		c.setFlag(x86FlagCF, cf)
	case 1: // DEC Ev
		cf := c.CF()
		if c.prefixOpSize {
			val := c.readRM16()
			result := uint32(val) - 1
			c.setFlagsArith16(result, val, 1, true)
			c.writeRM16(uint16(result))
		} else {
			val := c.readRM32()
			result := uint64(val) - 1
			c.setFlagsArith32(result, val, 1, true)
			c.writeRM32(uint32(result))
		}
		c.setFlag(x86FlagCF, cf)
	case 2: // CALL Ev (near indirect)
		if c.prefixOpSize {
			target := c.readRM16()
			c.push16(c.IP())
			c.SetIP(target)
		} else {
			target := c.readRM32()
			c.push32(c.EIP)
			c.EIP = target
		}
	case 3: // CALL Mp (far indirect)
		addr := c.getEffectiveAddress()
		if c.prefixOpSize {
			offset := c.read16(addr)
			seg := c.read16(addr + 2)
			c.push16(c.CS)
			c.push16(c.IP())
			c.SetIP(offset)
			c.CS = seg
		} else {
			offset := c.read32(addr)
			seg := c.read16(addr + 4)
			c.push16(c.CS)
			c.push32(c.EIP)
			c.EIP = offset
			c.CS = seg
		}
	case 4: // JMP Ev (near indirect)
		if c.prefixOpSize {
			c.SetIP(c.readRM16())
		} else {
			c.EIP = c.readRM32()
		}
	case 5: // JMP Mp (far indirect)
		addr := c.getEffectiveAddress()
		if c.prefixOpSize {
			c.SetIP(c.read16(addr))
			c.CS = c.read16(addr + 2)
		} else {
			c.EIP = c.read32(addr)
			c.CS = c.read16(addr + 4)
		}
	case 6: // PUSH Ev
		if c.prefixOpSize {
			c.push16(c.readRM16())
		} else {
			c.push32(c.readRM32())
		}
	default:
		c.Halted = true // Undefined
	}
	c.Cycles += 2
}

// =============================================================================
// SETcc Instructions
// =============================================================================

func (c *CPU_X86) setcc(cond bool) {
	c.fetchModRM()
	if cond {
		c.writeRM8(1)
	} else {
		c.writeRM8(0)
	}
	c.Cycles += 2
}

func (c *CPU_X86) opSETO()   { c.setcc(c.OF()) }
func (c *CPU_X86) opSETNO()  { c.setcc(!c.OF()) }
func (c *CPU_X86) opSETB()   { c.setcc(c.CF()) }
func (c *CPU_X86) opSETNB()  { c.setcc(!c.CF()) }
func (c *CPU_X86) opSETZ()   { c.setcc(c.ZF()) }
func (c *CPU_X86) opSETNZ()  { c.setcc(!c.ZF()) }
func (c *CPU_X86) opSETBE()  { c.setcc(c.CF() || c.ZF()) }
func (c *CPU_X86) opSETNBE() { c.setcc(!c.CF() && !c.ZF()) }
func (c *CPU_X86) opSETS()   { c.setcc(c.SF()) }
func (c *CPU_X86) opSETNS()  { c.setcc(!c.SF()) }
func (c *CPU_X86) opSETP()   { c.setcc(c.PF()) }
func (c *CPU_X86) opSETNP()  { c.setcc(!c.PF()) }
func (c *CPU_X86) opSETL()   { c.setcc(c.SF() != c.OF()) }
func (c *CPU_X86) opSETNL()  { c.setcc(c.SF() == c.OF()) }
func (c *CPU_X86) opSETLE()  { c.setcc(c.ZF() || c.SF() != c.OF()) }
func (c *CPU_X86) opSETNLE() { c.setcc(!c.ZF() && c.SF() == c.OF()) }

// =============================================================================
// MOVZX/MOVSX Instructions
// =============================================================================

func (c *CPU_X86) opMOVZX_Gv_Eb() {
	c.fetchModRM()
	val := uint32(c.readRM8())
	if c.prefixOpSize {
		c.setReg16(c.getModRMReg(), uint16(val))
	} else {
		c.setReg32(c.getModRMReg(), val)
	}
	c.Cycles += 2
}

func (c *CPU_X86) opMOVZX_Gv_Ew() {
	c.fetchModRM()
	val := uint32(c.readRM16())
	c.setReg32(c.getModRMReg(), val)
	c.Cycles += 2
}

func (c *CPU_X86) opMOVSX_Gv_Eb() {
	c.fetchModRM()
	val := int8(c.readRM8())
	if c.prefixOpSize {
		c.setReg16(c.getModRMReg(), uint16(int16(val)))
	} else {
		c.setReg32(c.getModRMReg(), uint32(int32(val)))
	}
	c.Cycles += 2
}

func (c *CPU_X86) opMOVSX_Gv_Ew() {
	c.fetchModRM()
	val := int16(c.readRM16())
	c.setReg32(c.getModRMReg(), uint32(int32(val)))
	c.Cycles += 2
}

// =============================================================================
// Bit Test Instructions (BT, BTS, BTR, BTC)
// =============================================================================

func (c *CPU_X86) opBT_Ev_Gv() {
	c.fetchModRM()
	if c.prefixOpSize {
		val := c.readRM16()
		bit := c.getReg16(c.getModRMReg()) & 15
		c.setFlag(x86FlagCF, (val>>bit)&1 != 0)
	} else {
		val := c.readRM32()
		bit := c.getReg32(c.getModRMReg()) & 31
		c.setFlag(x86FlagCF, (val>>bit)&1 != 0)
	}
	c.Cycles += 3
}

func (c *CPU_X86) opBTS_Ev_Gv() {
	c.fetchModRM()
	if c.prefixOpSize {
		val := c.readRM16()
		bit := c.getReg16(c.getModRMReg()) & 15
		c.setFlag(x86FlagCF, (val>>bit)&1 != 0)
		c.writeRM16(val | (1 << bit))
	} else {
		val := c.readRM32()
		bit := c.getReg32(c.getModRMReg()) & 31
		c.setFlag(x86FlagCF, (val>>bit)&1 != 0)
		c.writeRM32(val | (1 << bit))
	}
	c.Cycles += 3
}

func (c *CPU_X86) opBTR_Ev_Gv() {
	c.fetchModRM()
	if c.prefixOpSize {
		val := c.readRM16()
		bit := c.getReg16(c.getModRMReg()) & 15
		c.setFlag(x86FlagCF, (val>>bit)&1 != 0)
		c.writeRM16(val &^ (1 << bit))
	} else {
		val := c.readRM32()
		bit := c.getReg32(c.getModRMReg()) & 31
		c.setFlag(x86FlagCF, (val>>bit)&1 != 0)
		c.writeRM32(val &^ (1 << bit))
	}
	c.Cycles += 3
}

func (c *CPU_X86) opBTC_Ev_Gv() {
	c.fetchModRM()
	if c.prefixOpSize {
		val := c.readRM16()
		bit := c.getReg16(c.getModRMReg()) & 15
		c.setFlag(x86FlagCF, (val>>bit)&1 != 0)
		c.writeRM16(val ^ (1 << bit))
	} else {
		val := c.readRM32()
		bit := c.getReg32(c.getModRMReg()) & 31
		c.setFlag(x86FlagCF, (val>>bit)&1 != 0)
		c.writeRM32(val ^ (1 << bit))
	}
	c.Cycles += 3
}

func (c *CPU_X86) opGrp8_Ev_Ib() {
	c.fetchModRM()
	op := c.getModRMReg()
	bit := c.fetch8()

	if c.prefixOpSize {
		val := c.readRM16()
		bit &= 15
		c.setFlag(x86FlagCF, (val>>bit)&1 != 0)
		switch op {
		case 4: // BT
			// Just test
		case 5: // BTS
			c.writeRM16(val | (1 << bit))
		case 6: // BTR
			c.writeRM16(val &^ (1 << bit))
		case 7: // BTC
			c.writeRM16(val ^ (1 << bit))
		}
	} else {
		val := c.readRM32()
		bit &= 31
		c.setFlag(x86FlagCF, (val>>bit)&1 != 0)
		switch op {
		case 4: // BT
			// Just test
		case 5: // BTS
			c.writeRM32(val | (1 << bit))
		case 6: // BTR
			c.writeRM32(val &^ (1 << bit))
		case 7: // BTC
			c.writeRM32(val ^ (1 << bit))
		}
	}
	c.Cycles += 3
}

// =============================================================================
// BSF/BSR Instructions
// =============================================================================

func (c *CPU_X86) opBSF_Gv_Ev() {
	c.fetchModRM()
	if c.prefixOpSize {
		val := c.readRM16()
		if val == 0 {
			c.setFlag(x86FlagZF, true)
		} else {
			c.setFlag(x86FlagZF, false)
			var i uint16
			for i = 0; i < 16; i++ {
				if (val>>i)&1 != 0 {
					break
				}
			}
			c.setReg16(c.getModRMReg(), i)
		}
	} else {
		val := c.readRM32()
		if val == 0 {
			c.setFlag(x86FlagZF, true)
		} else {
			c.setFlag(x86FlagZF, false)
			var i uint32
			for i = 0; i < 32; i++ {
				if (val>>i)&1 != 0 {
					break
				}
			}
			c.setReg32(c.getModRMReg(), i)
		}
	}
	c.Cycles += 10
}

func (c *CPU_X86) opBSR_Gv_Ev() {
	c.fetchModRM()
	if c.prefixOpSize {
		val := c.readRM16()
		if val == 0 {
			c.setFlag(x86FlagZF, true)
		} else {
			c.setFlag(x86FlagZF, false)
			var i uint16
			for i = 15; ; i-- {
				if (val>>i)&1 != 0 {
					break
				}
				if i == 0 {
					break
				}
			}
			c.setReg16(c.getModRMReg(), i)
		}
	} else {
		val := c.readRM32()
		if val == 0 {
			c.setFlag(x86FlagZF, true)
		} else {
			c.setFlag(x86FlagZF, false)
			var i uint32
			for i = 31; ; i-- {
				if (val>>i)&1 != 0 {
					break
				}
				if i == 0 {
					break
				}
			}
			c.setReg32(c.getModRMReg(), i)
		}
	}
	c.Cycles += 10
}

// =============================================================================
// SHLD/SHRD Instructions
// =============================================================================

func (c *CPU_X86) opSHLD_Ev_Gv_Ib() {
	c.fetchModRM()
	count := c.fetch8() & 0x1F
	if count == 0 {
		return
	}

	if c.prefixOpSize {
		dst := c.readRM16()
		src := c.getReg16(c.getModRMReg())
		result := (uint32(dst) << count) | (uint32(src) >> (16 - count))
		c.setFlag(x86FlagCF, ((dst>>(16-count))&1) != 0)
		r16 := uint16(result)
		c.setFlag(x86FlagSF, (r16&0x8000) != 0)
		c.setFlag(x86FlagZF, r16 == 0)
		c.setFlag(x86FlagPF, parity(byte(r16)))
		c.writeRM16(r16)
	} else {
		dst := c.readRM32()
		src := c.getReg32(c.getModRMReg())
		result := (uint64(dst) << count) | (uint64(src) >> (32 - count))
		c.setFlag(x86FlagCF, ((dst>>(32-count))&1) != 0)
		r32 := uint32(result)
		c.setFlag(x86FlagSF, (r32&0x80000000) != 0)
		c.setFlag(x86FlagZF, r32 == 0)
		c.setFlag(x86FlagPF, parity(byte(r32)))
		c.writeRM32(r32)
	}
	c.Cycles += 3
}

func (c *CPU_X86) opSHLD_Ev_Gv_CL() {
	c.fetchModRM()
	count := c.CL() & 0x1F
	if count == 0 {
		return
	}

	if c.prefixOpSize {
		dst := c.readRM16()
		src := c.getReg16(c.getModRMReg())
		result := (uint32(dst) << count) | (uint32(src) >> (16 - count))
		c.setFlag(x86FlagCF, ((dst>>(16-count))&1) != 0)
		r16 := uint16(result)
		c.setFlag(x86FlagSF, (r16&0x8000) != 0)
		c.setFlag(x86FlagZF, r16 == 0)
		c.setFlag(x86FlagPF, parity(byte(r16)))
		c.writeRM16(r16)
	} else {
		dst := c.readRM32()
		src := c.getReg32(c.getModRMReg())
		result := (uint64(dst) << count) | (uint64(src) >> (32 - count))
		c.setFlag(x86FlagCF, ((dst>>(32-count))&1) != 0)
		r32 := uint32(result)
		c.setFlag(x86FlagSF, (r32&0x80000000) != 0)
		c.setFlag(x86FlagZF, r32 == 0)
		c.setFlag(x86FlagPF, parity(byte(r32)))
		c.writeRM32(r32)
	}
	c.Cycles += 3
}

func (c *CPU_X86) opSHRD_Ev_Gv_Ib() {
	c.fetchModRM()
	count := c.fetch8() & 0x1F
	if count == 0 {
		return
	}

	if c.prefixOpSize {
		dst := c.readRM16()
		src := c.getReg16(c.getModRMReg())
		result := (uint32(dst) >> count) | (uint32(src) << (16 - count))
		c.setFlag(x86FlagCF, ((dst>>(count-1))&1) != 0)
		r16 := uint16(result)
		c.setFlag(x86FlagSF, (r16&0x8000) != 0)
		c.setFlag(x86FlagZF, r16 == 0)
		c.setFlag(x86FlagPF, parity(byte(r16)))
		c.writeRM16(r16)
	} else {
		dst := c.readRM32()
		src := c.getReg32(c.getModRMReg())
		result := (uint64(dst) >> count) | (uint64(src) << (32 - count))
		c.setFlag(x86FlagCF, ((dst>>(count-1))&1) != 0)
		r32 := uint32(result)
		c.setFlag(x86FlagSF, (r32&0x80000000) != 0)
		c.setFlag(x86FlagZF, r32 == 0)
		c.setFlag(x86FlagPF, parity(byte(r32)))
		c.writeRM32(r32)
	}
	c.Cycles += 3
}

func (c *CPU_X86) opSHRD_Ev_Gv_CL() {
	c.fetchModRM()
	count := c.CL() & 0x1F
	if count == 0 {
		return
	}

	if c.prefixOpSize {
		dst := c.readRM16()
		src := c.getReg16(c.getModRMReg())
		result := (uint32(dst) >> count) | (uint32(src) << (16 - count))
		c.setFlag(x86FlagCF, ((dst>>(count-1))&1) != 0)
		r16 := uint16(result)
		c.setFlag(x86FlagSF, (r16&0x8000) != 0)
		c.setFlag(x86FlagZF, r16 == 0)
		c.setFlag(x86FlagPF, parity(byte(r16)))
		c.writeRM16(r16)
	} else {
		dst := c.readRM32()
		src := c.getReg32(c.getModRMReg())
		result := (uint64(dst) >> count) | (uint64(src) << (32 - count))
		c.setFlag(x86FlagCF, ((dst>>(count-1))&1) != 0)
		r32 := uint32(result)
		c.setFlag(x86FlagSF, (r32&0x80000000) != 0)
		c.setFlag(x86FlagZF, r32 == 0)
		c.setFlag(x86FlagPF, parity(byte(r32)))
		c.writeRM32(r32)
	}
	c.Cycles += 3
}
