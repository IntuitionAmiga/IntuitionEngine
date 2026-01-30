// cpu_x86_ops.go - x86 CPU Instruction Implementations
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

package main

// =============================================================================
// ADD Instructions
// =============================================================================

func (c *CPU_X86) opADD_Eb_Gb() {
	c.fetchModRM()
	a := c.readRM8()
	b := c.getReg8(c.getModRMReg())
	result := uint16(a) + uint16(b)
	c.setFlagsArith8(result, a, b, false)
	c.writeRM8(byte(result))
	c.Cycles += 2
}

func (c *CPU_X86) opADD_Ev_Gv() {
	c.fetchModRM()
	if c.prefixOpSize {
		a := c.readRM16()
		b := c.getReg16(c.getModRMReg())
		result := uint32(a) + uint32(b)
		c.setFlagsArith16(result, a, b, false)
		c.writeRM16(uint16(result))
	} else {
		a := c.readRM32()
		b := c.getReg32(c.getModRMReg())
		result := uint64(a) + uint64(b)
		c.setFlagsArith32(result, a, b, false)
		c.writeRM32(uint32(result))
	}
	c.Cycles += 2
}

func (c *CPU_X86) opADD_Gb_Eb() {
	c.fetchModRM()
	a := c.getReg8(c.getModRMReg())
	b := c.readRM8()
	result := uint16(a) + uint16(b)
	c.setFlagsArith8(result, a, b, false)
	c.setReg8(c.getModRMReg(), byte(result))
	c.Cycles += 2
}

func (c *CPU_X86) opADD_Gv_Ev() {
	c.fetchModRM()
	if c.prefixOpSize {
		a := c.getReg16(c.getModRMReg())
		b := c.readRM16()
		result := uint32(a) + uint32(b)
		c.setFlagsArith16(result, a, b, false)
		c.setReg16(c.getModRMReg(), uint16(result))
	} else {
		a := c.getReg32(c.getModRMReg())
		b := c.readRM32()
		result := uint64(a) + uint64(b)
		c.setFlagsArith32(result, a, b, false)
		c.setReg32(c.getModRMReg(), uint32(result))
	}
	c.Cycles += 2
}

func (c *CPU_X86) opADD_AL_Ib() {
	a := c.AL()
	b := c.fetch8()
	result := uint16(a) + uint16(b)
	c.setFlagsArith8(result, a, b, false)
	c.SetAL(byte(result))
	c.Cycles += 2
}

func (c *CPU_X86) opADD_AX_Iv() {
	if c.prefixOpSize {
		a := c.AX()
		b := c.fetch16()
		result := uint32(a) + uint32(b)
		c.setFlagsArith16(result, a, b, false)
		c.SetAX(uint16(result))
	} else {
		a := c.EAX
		b := c.fetch32()
		result := uint64(a) + uint64(b)
		c.setFlagsArith32(result, a, b, false)
		c.EAX = uint32(result)
	}
	c.Cycles += 2
}

// =============================================================================
// ADC Instructions (Add with Carry)
// =============================================================================

func (c *CPU_X86) opADC_Eb_Gb() {
	c.fetchModRM()
	a := c.readRM8()
	b := c.getReg8(c.getModRMReg())
	var carry byte
	if c.CF() {
		carry = 1
	}
	result := uint16(a) + uint16(b) + uint16(carry)
	c.setFlagsArith8(result, a, b+carry, false)
	c.writeRM8(byte(result))
	c.Cycles += 2
}

func (c *CPU_X86) opADC_Ev_Gv() {
	c.fetchModRM()
	var carry uint32
	if c.CF() {
		carry = 1
	}
	if c.prefixOpSize {
		a := c.readRM16()
		b := c.getReg16(c.getModRMReg())
		result := uint32(a) + uint32(b) + carry
		c.setFlagsArith16(result, a, b+uint16(carry), false)
		c.writeRM16(uint16(result))
	} else {
		a := c.readRM32()
		b := c.getReg32(c.getModRMReg())
		result := uint64(a) + uint64(b) + uint64(carry)
		c.setFlagsArith32(result, a, b+carry, false)
		c.writeRM32(uint32(result))
	}
	c.Cycles += 2
}

func (c *CPU_X86) opADC_Gb_Eb() {
	c.fetchModRM()
	a := c.getReg8(c.getModRMReg())
	b := c.readRM8()
	var carry byte
	if c.CF() {
		carry = 1
	}
	result := uint16(a) + uint16(b) + uint16(carry)
	c.setFlagsArith8(result, a, b+carry, false)
	c.setReg8(c.getModRMReg(), byte(result))
	c.Cycles += 2
}

func (c *CPU_X86) opADC_Gv_Ev() {
	c.fetchModRM()
	var carry uint32
	if c.CF() {
		carry = 1
	}
	if c.prefixOpSize {
		a := c.getReg16(c.getModRMReg())
		b := c.readRM16()
		result := uint32(a) + uint32(b) + carry
		c.setFlagsArith16(result, a, b+uint16(carry), false)
		c.setReg16(c.getModRMReg(), uint16(result))
	} else {
		a := c.getReg32(c.getModRMReg())
		b := c.readRM32()
		result := uint64(a) + uint64(b) + uint64(carry)
		c.setFlagsArith32(result, a, b+carry, false)
		c.setReg32(c.getModRMReg(), uint32(result))
	}
	c.Cycles += 2
}

func (c *CPU_X86) opADC_AL_Ib() {
	a := c.AL()
	b := c.fetch8()
	var carry byte
	if c.CF() {
		carry = 1
	}
	result := uint16(a) + uint16(b) + uint16(carry)
	c.setFlagsArith8(result, a, b+carry, false)
	c.SetAL(byte(result))
	c.Cycles += 2
}

func (c *CPU_X86) opADC_AX_Iv() {
	var carry uint32
	if c.CF() {
		carry = 1
	}
	if c.prefixOpSize {
		a := c.AX()
		b := c.fetch16()
		result := uint32(a) + uint32(b) + carry
		c.setFlagsArith16(result, a, b+uint16(carry), false)
		c.SetAX(uint16(result))
	} else {
		a := c.EAX
		b := c.fetch32()
		result := uint64(a) + uint64(b) + uint64(carry)
		c.setFlagsArith32(result, a, b+carry, false)
		c.EAX = uint32(result)
	}
	c.Cycles += 2
}

// =============================================================================
// SUB Instructions
// =============================================================================

func (c *CPU_X86) opSUB_Eb_Gb() {
	c.fetchModRM()
	a := c.readRM8()
	b := c.getReg8(c.getModRMReg())
	result := uint16(a) - uint16(b)
	c.setFlagsArith8(result, a, b, true)
	c.writeRM8(byte(result))
	c.Cycles += 2
}

func (c *CPU_X86) opSUB_Ev_Gv() {
	c.fetchModRM()
	if c.prefixOpSize {
		a := c.readRM16()
		b := c.getReg16(c.getModRMReg())
		result := uint32(a) - uint32(b)
		c.setFlagsArith16(result, a, b, true)
		c.writeRM16(uint16(result))
	} else {
		a := c.readRM32()
		b := c.getReg32(c.getModRMReg())
		result := uint64(a) - uint64(b)
		c.setFlagsArith32(result, a, b, true)
		c.writeRM32(uint32(result))
	}
	c.Cycles += 2
}

func (c *CPU_X86) opSUB_Gb_Eb() {
	c.fetchModRM()
	a := c.getReg8(c.getModRMReg())
	b := c.readRM8()
	result := uint16(a) - uint16(b)
	c.setFlagsArith8(result, a, b, true)
	c.setReg8(c.getModRMReg(), byte(result))
	c.Cycles += 2
}

func (c *CPU_X86) opSUB_Gv_Ev() {
	c.fetchModRM()
	if c.prefixOpSize {
		a := c.getReg16(c.getModRMReg())
		b := c.readRM16()
		result := uint32(a) - uint32(b)
		c.setFlagsArith16(result, a, b, true)
		c.setReg16(c.getModRMReg(), uint16(result))
	} else {
		a := c.getReg32(c.getModRMReg())
		b := c.readRM32()
		result := uint64(a) - uint64(b)
		c.setFlagsArith32(result, a, b, true)
		c.setReg32(c.getModRMReg(), uint32(result))
	}
	c.Cycles += 2
}

func (c *CPU_X86) opSUB_AL_Ib() {
	a := c.AL()
	b := c.fetch8()
	result := uint16(a) - uint16(b)
	c.setFlagsArith8(result, a, b, true)
	c.SetAL(byte(result))
	c.Cycles += 2
}

func (c *CPU_X86) opSUB_AX_Iv() {
	if c.prefixOpSize {
		a := c.AX()
		b := c.fetch16()
		result := uint32(a) - uint32(b)
		c.setFlagsArith16(result, a, b, true)
		c.SetAX(uint16(result))
	} else {
		a := c.EAX
		b := c.fetch32()
		result := uint64(a) - uint64(b)
		c.setFlagsArith32(result, a, b, true)
		c.EAX = uint32(result)
	}
	c.Cycles += 2
}

// =============================================================================
// SBB Instructions (Subtract with Borrow)
// =============================================================================

func (c *CPU_X86) opSBB_Eb_Gb() {
	c.fetchModRM()
	a := c.readRM8()
	b := c.getReg8(c.getModRMReg())
	var borrow byte
	if c.CF() {
		borrow = 1
	}
	result := uint16(a) - uint16(b) - uint16(borrow)
	c.setFlagsArith8(result, a, b+borrow, true)
	c.writeRM8(byte(result))
	c.Cycles += 2
}

func (c *CPU_X86) opSBB_Ev_Gv() {
	c.fetchModRM()
	var borrow uint32
	if c.CF() {
		borrow = 1
	}
	if c.prefixOpSize {
		a := c.readRM16()
		b := c.getReg16(c.getModRMReg())
		result := uint32(a) - uint32(b) - borrow
		c.setFlagsArith16(result, a, b+uint16(borrow), true)
		c.writeRM16(uint16(result))
	} else {
		a := c.readRM32()
		b := c.getReg32(c.getModRMReg())
		result := uint64(a) - uint64(b) - uint64(borrow)
		c.setFlagsArith32(result, a, b+borrow, true)
		c.writeRM32(uint32(result))
	}
	c.Cycles += 2
}

func (c *CPU_X86) opSBB_Gb_Eb() {
	c.fetchModRM()
	a := c.getReg8(c.getModRMReg())
	b := c.readRM8()
	var borrow byte
	if c.CF() {
		borrow = 1
	}
	result := uint16(a) - uint16(b) - uint16(borrow)
	c.setFlagsArith8(result, a, b+borrow, true)
	c.setReg8(c.getModRMReg(), byte(result))
	c.Cycles += 2
}

func (c *CPU_X86) opSBB_Gv_Ev() {
	c.fetchModRM()
	var borrow uint32
	if c.CF() {
		borrow = 1
	}
	if c.prefixOpSize {
		a := c.getReg16(c.getModRMReg())
		b := c.readRM16()
		result := uint32(a) - uint32(b) - borrow
		c.setFlagsArith16(result, a, b+uint16(borrow), true)
		c.setReg16(c.getModRMReg(), uint16(result))
	} else {
		a := c.getReg32(c.getModRMReg())
		b := c.readRM32()
		result := uint64(a) - uint64(b) - uint64(borrow)
		c.setFlagsArith32(result, a, b+borrow, true)
		c.setReg32(c.getModRMReg(), uint32(result))
	}
	c.Cycles += 2
}

func (c *CPU_X86) opSBB_AL_Ib() {
	a := c.AL()
	b := c.fetch8()
	var borrow byte
	if c.CF() {
		borrow = 1
	}
	result := uint16(a) - uint16(b) - uint16(borrow)
	c.setFlagsArith8(result, a, b+borrow, true)
	c.SetAL(byte(result))
	c.Cycles += 2
}

func (c *CPU_X86) opSBB_AX_Iv() {
	var borrow uint32
	if c.CF() {
		borrow = 1
	}
	if c.prefixOpSize {
		a := c.AX()
		b := c.fetch16()
		result := uint32(a) - uint32(b) - borrow
		c.setFlagsArith16(result, a, b+uint16(borrow), true)
		c.SetAX(uint16(result))
	} else {
		a := c.EAX
		b := c.fetch32()
		result := uint64(a) - uint64(b) - uint64(borrow)
		c.setFlagsArith32(result, a, b+borrow, true)
		c.EAX = uint32(result)
	}
	c.Cycles += 2
}

// =============================================================================
// CMP Instructions
// =============================================================================

func (c *CPU_X86) opCMP_Eb_Gb() {
	c.fetchModRM()
	a := c.readRM8()
	b := c.getReg8(c.getModRMReg())
	result := uint16(a) - uint16(b)
	c.setFlagsArith8(result, a, b, true)
	c.Cycles += 2
}

func (c *CPU_X86) opCMP_Ev_Gv() {
	c.fetchModRM()
	if c.prefixOpSize {
		a := c.readRM16()
		b := c.getReg16(c.getModRMReg())
		result := uint32(a) - uint32(b)
		c.setFlagsArith16(result, a, b, true)
	} else {
		a := c.readRM32()
		b := c.getReg32(c.getModRMReg())
		result := uint64(a) - uint64(b)
		c.setFlagsArith32(result, a, b, true)
	}
	c.Cycles += 2
}

func (c *CPU_X86) opCMP_Gb_Eb() {
	c.fetchModRM()
	a := c.getReg8(c.getModRMReg())
	b := c.readRM8()
	result := uint16(a) - uint16(b)
	c.setFlagsArith8(result, a, b, true)
	c.Cycles += 2
}

func (c *CPU_X86) opCMP_Gv_Ev() {
	c.fetchModRM()
	if c.prefixOpSize {
		a := c.getReg16(c.getModRMReg())
		b := c.readRM16()
		result := uint32(a) - uint32(b)
		c.setFlagsArith16(result, a, b, true)
	} else {
		a := c.getReg32(c.getModRMReg())
		b := c.readRM32()
		result := uint64(a) - uint64(b)
		c.setFlagsArith32(result, a, b, true)
	}
	c.Cycles += 2
}

func (c *CPU_X86) opCMP_AL_Ib() {
	a := c.AL()
	b := c.fetch8()
	result := uint16(a) - uint16(b)
	c.setFlagsArith8(result, a, b, true)
	c.Cycles += 2
}

func (c *CPU_X86) opCMP_AX_Iv() {
	if c.prefixOpSize {
		a := c.AX()
		b := c.fetch16()
		result := uint32(a) - uint32(b)
		c.setFlagsArith16(result, a, b, true)
	} else {
		a := c.EAX
		b := c.fetch32()
		result := uint64(a) - uint64(b)
		c.setFlagsArith32(result, a, b, true)
	}
	c.Cycles += 2
}

// =============================================================================
// AND Instructions
// =============================================================================

func (c *CPU_X86) opAND_Eb_Gb() {
	c.fetchModRM()
	a := c.readRM8()
	b := c.getReg8(c.getModRMReg())
	result := a & b
	c.setFlagsLogic8(result)
	c.writeRM8(result)
	c.Cycles += 2
}

func (c *CPU_X86) opAND_Ev_Gv() {
	c.fetchModRM()
	if c.prefixOpSize {
		a := c.readRM16()
		b := c.getReg16(c.getModRMReg())
		result := a & b
		c.setFlagsLogic16(result)
		c.writeRM16(result)
	} else {
		a := c.readRM32()
		b := c.getReg32(c.getModRMReg())
		result := a & b
		c.setFlagsLogic32(result)
		c.writeRM32(result)
	}
	c.Cycles += 2
}

func (c *CPU_X86) opAND_Gb_Eb() {
	c.fetchModRM()
	a := c.getReg8(c.getModRMReg())
	b := c.readRM8()
	result := a & b
	c.setFlagsLogic8(result)
	c.setReg8(c.getModRMReg(), result)
	c.Cycles += 2
}

func (c *CPU_X86) opAND_Gv_Ev() {
	c.fetchModRM()
	if c.prefixOpSize {
		a := c.getReg16(c.getModRMReg())
		b := c.readRM16()
		result := a & b
		c.setFlagsLogic16(result)
		c.setReg16(c.getModRMReg(), result)
	} else {
		a := c.getReg32(c.getModRMReg())
		b := c.readRM32()
		result := a & b
		c.setFlagsLogic32(result)
		c.setReg32(c.getModRMReg(), result)
	}
	c.Cycles += 2
}

func (c *CPU_X86) opAND_AL_Ib() {
	a := c.AL()
	b := c.fetch8()
	result := a & b
	c.setFlagsLogic8(result)
	c.SetAL(result)
	c.Cycles += 2
}

func (c *CPU_X86) opAND_AX_Iv() {
	if c.prefixOpSize {
		a := c.AX()
		b := c.fetch16()
		result := a & b
		c.setFlagsLogic16(result)
		c.SetAX(result)
	} else {
		a := c.EAX
		b := c.fetch32()
		result := a & b
		c.setFlagsLogic32(result)
		c.EAX = result
	}
	c.Cycles += 2
}

// =============================================================================
// OR Instructions
// =============================================================================

func (c *CPU_X86) opOR_Eb_Gb() {
	c.fetchModRM()
	a := c.readRM8()
	b := c.getReg8(c.getModRMReg())
	result := a | b
	c.setFlagsLogic8(result)
	c.writeRM8(result)
	c.Cycles += 2
}

func (c *CPU_X86) opOR_Ev_Gv() {
	c.fetchModRM()
	if c.prefixOpSize {
		a := c.readRM16()
		b := c.getReg16(c.getModRMReg())
		result := a | b
		c.setFlagsLogic16(result)
		c.writeRM16(result)
	} else {
		a := c.readRM32()
		b := c.getReg32(c.getModRMReg())
		result := a | b
		c.setFlagsLogic32(result)
		c.writeRM32(result)
	}
	c.Cycles += 2
}

func (c *CPU_X86) opOR_Gb_Eb() {
	c.fetchModRM()
	a := c.getReg8(c.getModRMReg())
	b := c.readRM8()
	result := a | b
	c.setFlagsLogic8(result)
	c.setReg8(c.getModRMReg(), result)
	c.Cycles += 2
}

func (c *CPU_X86) opOR_Gv_Ev() {
	c.fetchModRM()
	if c.prefixOpSize {
		a := c.getReg16(c.getModRMReg())
		b := c.readRM16()
		result := a | b
		c.setFlagsLogic16(result)
		c.setReg16(c.getModRMReg(), result)
	} else {
		a := c.getReg32(c.getModRMReg())
		b := c.readRM32()
		result := a | b
		c.setFlagsLogic32(result)
		c.setReg32(c.getModRMReg(), result)
	}
	c.Cycles += 2
}

func (c *CPU_X86) opOR_AL_Ib() {
	a := c.AL()
	b := c.fetch8()
	result := a | b
	c.setFlagsLogic8(result)
	c.SetAL(result)
	c.Cycles += 2
}

func (c *CPU_X86) opOR_AX_Iv() {
	if c.prefixOpSize {
		a := c.AX()
		b := c.fetch16()
		result := a | b
		c.setFlagsLogic16(result)
		c.SetAX(result)
	} else {
		a := c.EAX
		b := c.fetch32()
		result := a | b
		c.setFlagsLogic32(result)
		c.EAX = result
	}
	c.Cycles += 2
}

// =============================================================================
// XOR Instructions
// =============================================================================

func (c *CPU_X86) opXOR_Eb_Gb() {
	c.fetchModRM()
	a := c.readRM8()
	b := c.getReg8(c.getModRMReg())
	result := a ^ b
	c.setFlagsLogic8(result)
	c.writeRM8(result)
	c.Cycles += 2
}

func (c *CPU_X86) opXOR_Ev_Gv() {
	c.fetchModRM()
	if c.prefixOpSize {
		a := c.readRM16()
		b := c.getReg16(c.getModRMReg())
		result := a ^ b
		c.setFlagsLogic16(result)
		c.writeRM16(result)
	} else {
		a := c.readRM32()
		b := c.getReg32(c.getModRMReg())
		result := a ^ b
		c.setFlagsLogic32(result)
		c.writeRM32(result)
	}
	c.Cycles += 2
}

func (c *CPU_X86) opXOR_Gb_Eb() {
	c.fetchModRM()
	a := c.getReg8(c.getModRMReg())
	b := c.readRM8()
	result := a ^ b
	c.setFlagsLogic8(result)
	c.setReg8(c.getModRMReg(), result)
	c.Cycles += 2
}

func (c *CPU_X86) opXOR_Gv_Ev() {
	c.fetchModRM()
	if c.prefixOpSize {
		a := c.getReg16(c.getModRMReg())
		b := c.readRM16()
		result := a ^ b
		c.setFlagsLogic16(result)
		c.setReg16(c.getModRMReg(), result)
	} else {
		a := c.getReg32(c.getModRMReg())
		b := c.readRM32()
		result := a ^ b
		c.setFlagsLogic32(result)
		c.setReg32(c.getModRMReg(), result)
	}
	c.Cycles += 2
}

func (c *CPU_X86) opXOR_AL_Ib() {
	a := c.AL()
	b := c.fetch8()
	result := a ^ b
	c.setFlagsLogic8(result)
	c.SetAL(result)
	c.Cycles += 2
}

func (c *CPU_X86) opXOR_AX_Iv() {
	if c.prefixOpSize {
		a := c.AX()
		b := c.fetch16()
		result := a ^ b
		c.setFlagsLogic16(result)
		c.SetAX(result)
	} else {
		a := c.EAX
		b := c.fetch32()
		result := a ^ b
		c.setFlagsLogic32(result)
		c.EAX = result
	}
	c.Cycles += 2
}

// =============================================================================
// TEST Instructions
// =============================================================================

func (c *CPU_X86) opTEST_Eb_Gb() {
	c.fetchModRM()
	a := c.readRM8()
	b := c.getReg8(c.getModRMReg())
	result := a & b
	c.setFlagsLogic8(result)
	c.Cycles += 2
}

func (c *CPU_X86) opTEST_Ev_Gv() {
	c.fetchModRM()
	if c.prefixOpSize {
		a := c.readRM16()
		b := c.getReg16(c.getModRMReg())
		result := a & b
		c.setFlagsLogic16(result)
	} else {
		a := c.readRM32()
		b := c.getReg32(c.getModRMReg())
		result := a & b
		c.setFlagsLogic32(result)
	}
	c.Cycles += 2
}

func (c *CPU_X86) opTEST_AL_Ib() {
	a := c.AL()
	b := c.fetch8()
	result := a & b
	c.setFlagsLogic8(result)
	c.Cycles += 2
}

func (c *CPU_X86) opTEST_AX_Iv() {
	if c.prefixOpSize {
		a := c.AX()
		b := c.fetch16()
		result := a & b
		c.setFlagsLogic16(result)
	} else {
		a := c.EAX
		b := c.fetch32()
		result := a & b
		c.setFlagsLogic32(result)
	}
	c.Cycles += 2
}

// =============================================================================
// INC/DEC Instructions
// =============================================================================

func (c *CPU_X86) opINC_reg(idx byte) {
	// Save CF as INC doesn't affect it
	cf := c.CF()
	if c.prefixOpSize {
		v := c.getReg16(idx)
		result := uint32(v) + 1
		c.setFlagsArith16(result, v, 1, false)
		c.setReg16(idx, uint16(result))
	} else {
		v := c.getReg32(idx)
		result := uint64(v) + 1
		c.setFlagsArith32(result, v, 1, false)
		c.setReg32(idx, uint32(result))
	}
	c.setFlag(x86FlagCF, cf) // Restore CF
	c.Cycles += 1
}

func (c *CPU_X86) opDEC_reg(idx byte) {
	// Save CF as DEC doesn't affect it
	cf := c.CF()
	if c.prefixOpSize {
		v := c.getReg16(idx)
		result := uint32(v) - 1
		c.setFlagsArith16(result, v, 1, true)
		c.setReg16(idx, uint16(result))
	} else {
		v := c.getReg32(idx)
		result := uint64(v) - 1
		c.setFlagsArith32(result, v, 1, true)
		c.setReg32(idx, uint32(result))
	}
	c.setFlag(x86FlagCF, cf) // Restore CF
	c.Cycles += 1
}

// =============================================================================
// PUSH/POP Instructions
// =============================================================================

func (c *CPU_X86) opPUSH_reg(idx byte) {
	if c.prefixOpSize {
		c.push16(c.getReg16(idx))
	} else {
		c.push32(c.getReg32(idx))
	}
	c.Cycles += 1
}

func (c *CPU_X86) opPOP_reg(idx byte) {
	if c.prefixOpSize {
		c.setReg16(idx, c.pop16())
	} else {
		c.setReg32(idx, c.pop32())
	}
	c.Cycles += 1
}

func (c *CPU_X86) opPUSH_ES() {
	c.push16(c.ES)
	c.Cycles += 1
}

func (c *CPU_X86) opPOP_ES() {
	c.ES = c.pop16()
	c.Cycles += 1
}

func (c *CPU_X86) opPUSH_CS() {
	c.push16(c.CS)
	c.Cycles += 1
}

func (c *CPU_X86) opPUSH_SS() {
	c.push16(c.SS)
	c.Cycles += 1
}

func (c *CPU_X86) opPOP_SS() {
	c.SS = c.pop16()
	c.Cycles += 1
}

func (c *CPU_X86) opPUSH_DS() {
	c.push16(c.DS)
	c.Cycles += 1
}

func (c *CPU_X86) opPOP_DS() {
	c.DS = c.pop16()
	c.Cycles += 1
}

func (c *CPU_X86) opPUSH_FS() {
	c.push16(c.FS)
	c.Cycles += 1
}

func (c *CPU_X86) opPOP_FS() {
	c.FS = c.pop16()
	c.Cycles += 1
}

func (c *CPU_X86) opPUSH_GS() {
	c.push16(c.GS)
	c.Cycles += 1
}

func (c *CPU_X86) opPOP_GS() {
	c.GS = c.pop16()
	c.Cycles += 1
}

func (c *CPU_X86) opPUSH_Iv() {
	if c.prefixOpSize {
		c.push16(c.fetch16())
	} else {
		c.push32(c.fetch32())
	}
	c.Cycles += 1
}

func (c *CPU_X86) opPUSH_Ib() {
	// Sign-extended push
	v := int8(c.fetch8())
	if c.prefixOpSize {
		c.push16(uint16(int16(v)))
	} else {
		c.push32(uint32(int32(v)))
	}
	c.Cycles += 1
}

func (c *CPU_X86) opPUSHA() {
	temp := c.ESP
	if c.prefixOpSize {
		c.push16(c.AX())
		c.push16(c.CX())
		c.push16(c.DX())
		c.push16(c.BX())
		c.push16(uint16(temp))
		c.push16(c.BP())
		c.push16(c.SI())
		c.push16(c.DI())
	} else {
		c.push32(c.EAX)
		c.push32(c.ECX)
		c.push32(c.EDX)
		c.push32(c.EBX)
		c.push32(temp)
		c.push32(c.EBP)
		c.push32(c.ESI)
		c.push32(c.EDI)
	}
	c.Cycles += 5
}

func (c *CPU_X86) opPOPA() {
	if c.prefixOpSize {
		c.SetDI(c.pop16())
		c.SetSI(c.pop16())
		c.SetBP(c.pop16())
		c.pop16() // Skip SP
		c.SetBX(c.pop16())
		c.SetDX(c.pop16())
		c.SetCX(c.pop16())
		c.SetAX(c.pop16())
	} else {
		c.EDI = c.pop32()
		c.ESI = c.pop32()
		c.EBP = c.pop32()
		c.pop32() // Skip ESP
		c.EBX = c.pop32()
		c.EDX = c.pop32()
		c.ECX = c.pop32()
		c.EAX = c.pop32()
	}
	c.Cycles += 5
}

func (c *CPU_X86) opPUSHF() {
	if c.prefixOpSize {
		c.push16(uint16(c.Flags))
	} else {
		c.push32(c.Flags)
	}
	c.Cycles += 2
}

func (c *CPU_X86) opPOPF() {
	if c.prefixOpSize {
		c.Flags = (c.Flags & 0xFFFF0000) | uint32(c.pop16())
	} else {
		c.Flags = c.pop32()
	}
	c.Cycles += 2
}

func (c *CPU_X86) opPOP_Ev() {
	c.fetchModRM()
	if c.prefixOpSize {
		c.writeRM16(c.pop16())
	} else {
		c.writeRM32(c.pop32())
	}
	c.Cycles += 2
}

// =============================================================================
// MOV Instructions
// =============================================================================

func (c *CPU_X86) opMOV_Eb_Gb() {
	c.fetchModRM()
	c.writeRM8(c.getReg8(c.getModRMReg()))
	c.Cycles += 1
}

func (c *CPU_X86) opMOV_Ev_Gv() {
	c.fetchModRM()
	if c.prefixOpSize {
		c.writeRM16(c.getReg16(c.getModRMReg()))
	} else {
		c.writeRM32(c.getReg32(c.getModRMReg()))
	}
	c.Cycles += 1
}

func (c *CPU_X86) opMOV_Gb_Eb() {
	c.fetchModRM()
	c.setReg8(c.getModRMReg(), c.readRM8())
	c.Cycles += 1
}

func (c *CPU_X86) opMOV_Gv_Ev() {
	c.fetchModRM()
	if c.prefixOpSize {
		c.setReg16(c.getModRMReg(), c.readRM16())
	} else {
		c.setReg32(c.getModRMReg(), c.readRM32())
	}
	c.Cycles += 1
}

func (c *CPU_X86) opMOV_Ev_Sw() {
	c.fetchModRM()
	seg := c.getSeg(int(c.getModRMReg() & 7))
	if c.prefixOpSize || c.getModRMMod() == 3 {
		c.writeRM16(seg)
	} else {
		c.writeRM32(uint32(seg))
	}
	c.Cycles += 1
}

func (c *CPU_X86) opMOV_Sw_Ew() {
	c.fetchModRM()
	if c.prefixOpSize || c.getModRMMod() == 3 {
		c.setSeg(int(c.getModRMReg()&7), c.readRM16())
	} else {
		c.setSeg(int(c.getModRMReg()&7), uint16(c.readRM32()))
	}
	c.Cycles += 1
}

func (c *CPU_X86) opMOV_r8_imm8(idx byte) {
	c.setReg8(idx, c.fetch8())
	c.Cycles += 1
}

func (c *CPU_X86) opMOV_r_imm(idx byte) {
	if c.prefixOpSize {
		c.setReg16(idx, c.fetch16())
	} else {
		c.setReg32(idx, c.fetch32())
	}
	c.Cycles += 1
}

func (c *CPU_X86) opMOV_Eb_Ib() {
	c.fetchModRM()
	// Must calculate effective address BEFORE fetching immediate
	// because displacement bytes come before immediate in encoding:
	// [opcode] [ModR/M] [displacement] [immediate]
	if c.getModRMMod() == 3 {
		// Register operand - no displacement
		c.setReg8(c.getModRMRM(), c.fetch8())
	} else {
		addr := c.getEffectiveAddress() // Fetch displacement first
		imm := c.fetch8()               // Then fetch immediate
		c.write8(addr, imm)
	}
	c.Cycles += 1
}

func (c *CPU_X86) opMOV_Ev_Iv() {
	c.fetchModRM()
	// Must calculate effective address BEFORE fetching immediate
	if c.getModRMMod() == 3 {
		// Register operand
		if c.prefixOpSize {
			c.setReg16(c.getModRMRM(), c.fetch16())
		} else {
			c.setReg32(c.getModRMRM(), c.fetch32())
		}
	} else {
		// Memory operand - fetch displacement first, then immediate
		addr := c.getEffectiveAddress()
		if c.prefixOpSize {
			c.write16(addr, c.fetch16())
		} else {
			c.write32(addr, c.fetch32())
		}
	}
	c.Cycles += 1
}

func (c *CPU_X86) opMOV_AL_moffs() {
	var addr uint32
	if c.prefixAddrSize {
		addr = uint32(c.fetch16())
	} else {
		addr = c.fetch32()
	}
	c.SetAL(c.read8(addr))
	c.Cycles += 1
}

func (c *CPU_X86) opMOV_AX_moffs() {
	var addr uint32
	if c.prefixAddrSize {
		addr = uint32(c.fetch16())
	} else {
		addr = c.fetch32()
	}
	if c.prefixOpSize {
		c.SetAX(c.read16(addr))
	} else {
		c.EAX = c.read32(addr)
	}
	c.Cycles += 1
}

func (c *CPU_X86) opMOV_moffs_AL() {
	var addr uint32
	if c.prefixAddrSize {
		addr = uint32(c.fetch16())
	} else {
		addr = c.fetch32()
	}
	c.write8(addr, c.AL())
	c.Cycles += 1
}

func (c *CPU_X86) opMOV_moffs_AX() {
	var addr uint32
	if c.prefixAddrSize {
		addr = uint32(c.fetch16())
	} else {
		addr = c.fetch32()
	}
	if c.prefixOpSize {
		c.write16(addr, c.AX())
	} else {
		c.write32(addr, c.EAX)
	}
	c.Cycles += 1
}

// =============================================================================
// LEA, LES, LDS Instructions
// =============================================================================

func (c *CPU_X86) opLEA() {
	c.fetchModRM()
	addr := c.getEffectiveAddress()
	if c.prefixOpSize {
		c.setReg16(c.getModRMReg(), uint16(addr))
	} else {
		c.setReg32(c.getModRMReg(), addr)
	}
	c.Cycles += 1
}

func (c *CPU_X86) opLES() {
	c.fetchModRM()
	addr := c.getEffectiveAddress()
	if c.prefixOpSize {
		c.setReg16(c.getModRMReg(), c.read16(addr))
		c.ES = c.read16(addr + 2)
	} else {
		c.setReg32(c.getModRMReg(), c.read32(addr))
		c.ES = c.read16(addr + 4)
	}
	c.Cycles += 4
}

func (c *CPU_X86) opLDS() {
	c.fetchModRM()
	addr := c.getEffectiveAddress()
	if c.prefixOpSize {
		c.setReg16(c.getModRMReg(), c.read16(addr))
		c.DS = c.read16(addr + 2)
	} else {
		c.setReg32(c.getModRMReg(), c.read32(addr))
		c.DS = c.read16(addr + 4)
	}
	c.Cycles += 4
}

// =============================================================================
// XCHG Instructions
// =============================================================================

func (c *CPU_X86) opXCHG_Eb_Gb() {
	c.fetchModRM()
	a := c.readRM8()
	b := c.getReg8(c.getModRMReg())
	c.writeRM8(b)
	c.setReg8(c.getModRMReg(), a)
	c.Cycles += 3
}

func (c *CPU_X86) opXCHG_Ev_Gv() {
	c.fetchModRM()
	if c.prefixOpSize {
		a := c.readRM16()
		b := c.getReg16(c.getModRMReg())
		c.writeRM16(b)
		c.setReg16(c.getModRMReg(), a)
	} else {
		a := c.readRM32()
		b := c.getReg32(c.getModRMReg())
		c.writeRM32(b)
		c.setReg32(c.getModRMReg(), a)
	}
	c.Cycles += 3
}

func (c *CPU_X86) opXCHG_AX_reg(idx byte) {
	if c.prefixOpSize {
		a := c.AX()
		b := c.getReg16(idx)
		c.SetAX(b)
		c.setReg16(idx, a)
	} else {
		a := c.EAX
		b := c.getReg32(idx)
		c.EAX = b
		c.setReg32(idx, a)
	}
	c.Cycles += 3
}

func (c *CPU_X86) opNOP() {
	c.Cycles += 1
}

// =============================================================================
// CBW/CWD Instructions
// =============================================================================

func (c *CPU_X86) opCBW() {
	if c.prefixOpSize {
		// CBW: AL -> AX
		c.SetAX(uint16(int16(int8(c.AL()))))
	} else {
		// CWDE: AX -> EAX
		c.EAX = uint32(int32(int16(c.AX())))
	}
	c.Cycles += 1
}

func (c *CPU_X86) opCWD() {
	if c.prefixOpSize {
		// CWD: AX -> DX:AX
		if c.AX()&0x8000 != 0 {
			c.SetDX(0xFFFF)
		} else {
			c.SetDX(0)
		}
	} else {
		// CDQ: EAX -> EDX:EAX
		if c.EAX&0x80000000 != 0 {
			c.EDX = 0xFFFFFFFF
		} else {
			c.EDX = 0
		}
	}
	c.Cycles += 1
}

// =============================================================================
// Flag manipulation
// =============================================================================

func (c *CPU_X86) opSAHF() {
	// Load AH into low byte of flags
	c.Flags = (c.Flags & 0xFFFFFF00) | uint32(c.AH())
	c.Cycles += 1
}

func (c *CPU_X86) opLAHF() {
	// Load low byte of flags into AH
	c.SetAH(byte(c.Flags))
	c.Cycles += 1
}

func (c *CPU_X86) opCLC() {
	c.setFlag(x86FlagCF, false)
	c.Cycles += 1
}

func (c *CPU_X86) opSTC() {
	c.setFlag(x86FlagCF, true)
	c.Cycles += 1
}

func (c *CPU_X86) opCLI() {
	c.setFlag(x86FlagIF, false)
	c.Cycles += 1
}

func (c *CPU_X86) opSTI() {
	c.setFlag(x86FlagIF, true)
	c.Cycles += 1
}

func (c *CPU_X86) opCLD() {
	c.setFlag(x86FlagDF, false)
	c.Cycles += 1
}

func (c *CPU_X86) opSTD() {
	c.setFlag(x86FlagDF, true)
	c.Cycles += 1
}

func (c *CPU_X86) opCMC() {
	c.setFlag(x86FlagCF, !c.CF())
	c.Cycles += 1
}

// =============================================================================
// BCD Instructions
// =============================================================================

func (c *CPU_X86) opDAA() {
	al := c.AL()
	cf := c.CF()
	af := c.AF()

	if (al&0x0F) > 9 || af {
		al += 6
		c.setFlag(x86FlagAF, true)
	}
	if al > 0x9F || cf {
		al += 0x60
		c.setFlag(x86FlagCF, true)
	}

	c.SetAL(al)
	c.setFlag(x86FlagSF, (al&0x80) != 0)
	c.setFlag(x86FlagZF, al == 0)
	c.setFlag(x86FlagPF, parity(al))
	c.Cycles += 4
}

func (c *CPU_X86) opDAS() {
	al := c.AL()
	cf := c.CF()
	af := c.AF()

	if (al&0x0F) > 9 || af {
		al -= 6
		c.setFlag(x86FlagAF, true)
	}
	if al > 0x9F || cf {
		al -= 0x60
		c.setFlag(x86FlagCF, true)
	}

	c.SetAL(al)
	c.setFlag(x86FlagSF, (al&0x80) != 0)
	c.setFlag(x86FlagZF, al == 0)
	c.setFlag(x86FlagPF, parity(al))
	c.Cycles += 4
}

func (c *CPU_X86) opAAA() {
	if (c.AL()&0x0F) > 9 || c.AF() {
		c.SetAL(c.AL() + 6)
		c.SetAH(c.AH() + 1)
		c.setFlag(x86FlagAF, true)
		c.setFlag(x86FlagCF, true)
	} else {
		c.setFlag(x86FlagAF, false)
		c.setFlag(x86FlagCF, false)
	}
	c.SetAL(c.AL() & 0x0F)
	c.Cycles += 4
}

func (c *CPU_X86) opAAS() {
	if (c.AL()&0x0F) > 9 || c.AF() {
		c.SetAL(c.AL() - 6)
		c.SetAH(c.AH() - 1)
		c.setFlag(x86FlagAF, true)
		c.setFlag(x86FlagCF, true)
	} else {
		c.setFlag(x86FlagAF, false)
		c.setFlag(x86FlagCF, false)
	}
	c.SetAL(c.AL() & 0x0F)
	c.Cycles += 4
}

func (c *CPU_X86) opAAM() {
	base := c.fetch8()
	if base == 0 {
		// Division by zero - generate exception
		c.handleInterrupt(0)
		return
	}
	ah := c.AL() / base
	al := c.AL() % base
	c.SetAH(ah)
	c.SetAL(al)
	c.setFlag(x86FlagSF, (al&0x80) != 0)
	c.setFlag(x86FlagZF, al == 0)
	c.setFlag(x86FlagPF, parity(al))
	c.Cycles += 17
}

func (c *CPU_X86) opAAD() {
	base := c.fetch8()
	al := c.AH()*base + c.AL()
	c.SetAL(al)
	c.SetAH(0)
	c.setFlag(x86FlagSF, (al&0x80) != 0)
	c.setFlag(x86FlagZF, al == 0)
	c.setFlag(x86FlagPF, parity(al))
	c.Cycles += 19
}

func (c *CPU_X86) opSALC() {
	// Undocumented: Set AL to 0xFF if CF=1, else 0
	if c.CF() {
		c.SetAL(0xFF)
	} else {
		c.SetAL(0)
	}
	c.Cycles += 2
}

func (c *CPU_X86) opXLAT() {
	addr := c.EBX + uint32(c.AL())
	c.SetAL(c.read8(addr))
	c.Cycles += 5
}

// =============================================================================
// Jump Instructions
// =============================================================================

func (c *CPU_X86) opJMP_rel8() {
	offset := int8(c.fetch8())
	c.EIP = uint32(int32(c.EIP) + int32(offset))
	c.Cycles += 2
}

func (c *CPU_X86) opJMP_rel() {
	if c.prefixOpSize {
		offset := int16(c.fetch16())
		c.EIP = uint32(int32(c.EIP) + int32(offset))
	} else {
		offset := int32(c.fetch32())
		c.EIP = uint32(int32(c.EIP) + offset)
	}
	c.Cycles += 2
}

func (c *CPU_X86) opJMP_far() {
	if c.prefixOpSize {
		offset := c.fetch16()
		seg := c.fetch16()
		c.SetIP(offset)
		c.CS = seg
	} else {
		offset := c.fetch32()
		seg := c.fetch16()
		c.EIP = offset
		c.CS = seg
	}
	c.Cycles += 4
}

// Conditional jumps helper
func (c *CPU_X86) jccRel8(cond bool) {
	offset := int8(c.fetch8())
	if cond {
		c.EIP = uint32(int32(c.EIP) + int32(offset))
	}
	c.Cycles += 2
}

func (c *CPU_X86) jccRel16(cond bool) {
	if c.prefixOpSize {
		offset := int16(c.fetch16())
		if cond {
			c.EIP = uint32(int32(c.EIP) + int32(offset))
		}
	} else {
		offset := int32(c.fetch32())
		if cond {
			c.EIP = uint32(int32(c.EIP) + offset)
		}
	}
	c.Cycles += 2
}

func (c *CPU_X86) opJO_rel8()   { c.jccRel8(c.OF()) }
func (c *CPU_X86) opJNO_rel8()  { c.jccRel8(!c.OF()) }
func (c *CPU_X86) opJB_rel8()   { c.jccRel8(c.CF()) }
func (c *CPU_X86) opJNB_rel8()  { c.jccRel8(!c.CF()) }
func (c *CPU_X86) opJZ_rel8()   { c.jccRel8(c.ZF()) }
func (c *CPU_X86) opJNZ_rel8()  { c.jccRel8(!c.ZF()) }
func (c *CPU_X86) opJBE_rel8()  { c.jccRel8(c.CF() || c.ZF()) }
func (c *CPU_X86) opJNBE_rel8() { c.jccRel8(!c.CF() && !c.ZF()) }
func (c *CPU_X86) opJS_rel8()   { c.jccRel8(c.SF()) }
func (c *CPU_X86) opJNS_rel8()  { c.jccRel8(!c.SF()) }
func (c *CPU_X86) opJP_rel8()   { c.jccRel8(c.PF()) }
func (c *CPU_X86) opJNP_rel8()  { c.jccRel8(!c.PF()) }
func (c *CPU_X86) opJL_rel8()   { c.jccRel8(c.SF() != c.OF()) }
func (c *CPU_X86) opJNL_rel8()  { c.jccRel8(c.SF() == c.OF()) }
func (c *CPU_X86) opJLE_rel8()  { c.jccRel8(c.ZF() || c.SF() != c.OF()) }
func (c *CPU_X86) opJNLE_rel8() { c.jccRel8(!c.ZF() && c.SF() == c.OF()) }

func (c *CPU_X86) opJO_rel16()   { c.jccRel16(c.OF()) }
func (c *CPU_X86) opJNO_rel16()  { c.jccRel16(!c.OF()) }
func (c *CPU_X86) opJB_rel16()   { c.jccRel16(c.CF()) }
func (c *CPU_X86) opJNB_rel16()  { c.jccRel16(!c.CF()) }
func (c *CPU_X86) opJZ_rel16()   { c.jccRel16(c.ZF()) }
func (c *CPU_X86) opJNZ_rel16()  { c.jccRel16(!c.ZF()) }
func (c *CPU_X86) opJBE_rel16()  { c.jccRel16(c.CF() || c.ZF()) }
func (c *CPU_X86) opJNBE_rel16() { c.jccRel16(!c.CF() && !c.ZF()) }
func (c *CPU_X86) opJS_rel16()   { c.jccRel16(c.SF()) }
func (c *CPU_X86) opJNS_rel16()  { c.jccRel16(!c.SF()) }
func (c *CPU_X86) opJP_rel16()   { c.jccRel16(c.PF()) }
func (c *CPU_X86) opJNP_rel16()  { c.jccRel16(!c.PF()) }
func (c *CPU_X86) opJL_rel16()   { c.jccRel16(c.SF() != c.OF()) }
func (c *CPU_X86) opJNL_rel16()  { c.jccRel16(c.SF() == c.OF()) }
func (c *CPU_X86) opJLE_rel16()  { c.jccRel16(c.ZF() || c.SF() != c.OF()) }
func (c *CPU_X86) opJNLE_rel16() { c.jccRel16(!c.ZF() && c.SF() == c.OF()) }

func (c *CPU_X86) opJCXZ() {
	offset := int8(c.fetch8())
	cond := false
	if c.prefixAddrSize {
		cond = c.CX() == 0
	} else {
		cond = c.ECX == 0
	}
	if cond {
		c.EIP = uint32(int32(c.EIP) + int32(offset))
	}
	c.Cycles += 5
}

// =============================================================================
// CALL/RET Instructions
// =============================================================================

func (c *CPU_X86) opCALL_rel() {
	if c.prefixOpSize {
		offset := int16(c.fetch16())
		c.push16(c.IP())
		c.EIP = uint32(int32(c.EIP) + int32(offset))
	} else {
		offset := int32(c.fetch32())
		c.push32(c.EIP)
		c.EIP = uint32(int32(c.EIP) + offset)
	}
	c.Cycles += 3
}

func (c *CPU_X86) opCALL_far() {
	if c.prefixOpSize {
		offset := c.fetch16()
		seg := c.fetch16()
		c.push16(c.CS)
		c.push16(c.IP())
		c.SetIP(offset)
		c.CS = seg
	} else {
		offset := c.fetch32()
		seg := c.fetch16()
		c.push16(c.CS)
		c.push32(c.EIP)
		c.EIP = offset
		c.CS = seg
	}
	c.Cycles += 6
}

func (c *CPU_X86) opRET() {
	if c.prefixOpSize {
		c.SetIP(c.pop16())
	} else {
		c.EIP = c.pop32()
	}
	c.Cycles += 2
}

func (c *CPU_X86) opRET_imm16() {
	disp := c.fetch16()
	if c.prefixOpSize {
		c.SetIP(c.pop16())
	} else {
		c.EIP = c.pop32()
	}
	c.ESP += uint32(disp)
	c.Cycles += 3
}

func (c *CPU_X86) opRETF() {
	if c.prefixOpSize {
		c.SetIP(c.pop16())
		c.CS = c.pop16()
	} else {
		c.EIP = c.pop32()
		c.CS = c.pop16()
	}
	c.Cycles += 4
}

func (c *CPU_X86) opRETF_imm16() {
	disp := c.fetch16()
	if c.prefixOpSize {
		c.SetIP(c.pop16())
		c.CS = c.pop16()
	} else {
		c.EIP = c.pop32()
		c.CS = c.pop16()
	}
	c.ESP += uint32(disp)
	c.Cycles += 5
}

// =============================================================================
// LOOP Instructions
// =============================================================================

func (c *CPU_X86) opLOOP() {
	offset := int8(c.fetch8())
	if c.prefixAddrSize {
		c.SetCX(c.CX() - 1)
		if c.CX() != 0 {
			c.EIP = uint32(int32(c.EIP) + int32(offset))
		}
	} else {
		c.ECX--
		if c.ECX != 0 {
			c.EIP = uint32(int32(c.EIP) + int32(offset))
		}
	}
	c.Cycles += 5
}

func (c *CPU_X86) opLOOPE() {
	offset := int8(c.fetch8())
	if c.prefixAddrSize {
		c.SetCX(c.CX() - 1)
		if c.CX() != 0 && c.ZF() {
			c.EIP = uint32(int32(c.EIP) + int32(offset))
		}
	} else {
		c.ECX--
		if c.ECX != 0 && c.ZF() {
			c.EIP = uint32(int32(c.EIP) + int32(offset))
		}
	}
	c.Cycles += 5
}

func (c *CPU_X86) opLOOPNE() {
	offset := int8(c.fetch8())
	if c.prefixAddrSize {
		c.SetCX(c.CX() - 1)
		if c.CX() != 0 && !c.ZF() {
			c.EIP = uint32(int32(c.EIP) + int32(offset))
		}
	} else {
		c.ECX--
		if c.ECX != 0 && !c.ZF() {
			c.EIP = uint32(int32(c.EIP) + int32(offset))
		}
	}
	c.Cycles += 5
}

// =============================================================================
// Interrupt Instructions
// =============================================================================

func (c *CPU_X86) opINT3() {
	c.handleInterrupt(3)
	c.Cycles += 5
}

func (c *CPU_X86) opINT() {
	vector := c.fetch8()
	c.handleInterrupt(vector)
	c.Cycles += 5
}

func (c *CPU_X86) opINTO() {
	if c.OF() {
		c.handleInterrupt(4)
	}
	c.Cycles += 3
}

func (c *CPU_X86) opIRET() {
	if c.prefixOpSize {
		c.SetIP(c.pop16())
		c.CS = c.pop16()
		c.Flags = (c.Flags & 0xFFFF0000) | uint32(c.pop16())
	} else {
		c.EIP = c.pop32()
		c.CS = c.pop16()
		c.Flags = c.pop32()
	}
	c.Cycles += 4
}

// =============================================================================
// I/O Instructions
// =============================================================================

func (c *CPU_X86) opIN_AL_imm8() {
	port := uint16(c.fetch8())
	c.SetAL(c.bus.In(port))
	c.Cycles += 5
}

func (c *CPU_X86) opIN_AX_imm8() {
	port := uint16(c.fetch8())
	if c.prefixOpSize {
		lo := c.bus.In(port)
		hi := c.bus.In(port + 1)
		c.SetAX(uint16(lo) | (uint16(hi) << 8))
	} else {
		b0 := c.bus.In(port)
		b1 := c.bus.In(port + 1)
		b2 := c.bus.In(port + 2)
		b3 := c.bus.In(port + 3)
		c.EAX = uint32(b0) | (uint32(b1) << 8) | (uint32(b2) << 16) | (uint32(b3) << 24)
	}
	c.Cycles += 5
}

func (c *CPU_X86) opOUT_imm8_AL() {
	port := uint16(c.fetch8())
	c.bus.Out(port, c.AL())
	c.Cycles += 5
}

func (c *CPU_X86) opOUT_imm8_AX() {
	port := uint16(c.fetch8())
	if c.prefixOpSize {
		c.bus.Out(port, byte(c.AX()))
		c.bus.Out(port+1, byte(c.AX()>>8))
	} else {
		c.bus.Out(port, byte(c.EAX))
		c.bus.Out(port+1, byte(c.EAX>>8))
		c.bus.Out(port+2, byte(c.EAX>>16))
		c.bus.Out(port+3, byte(c.EAX>>24))
	}
	c.Cycles += 5
}

func (c *CPU_X86) opIN_AL_DX() {
	c.SetAL(c.bus.In(c.DX()))
	c.Cycles += 5
}

func (c *CPU_X86) opIN_AX_DX() {
	port := c.DX()
	if c.prefixOpSize {
		lo := c.bus.In(port)
		hi := c.bus.In(port + 1)
		c.SetAX(uint16(lo) | (uint16(hi) << 8))
	} else {
		b0 := c.bus.In(port)
		b1 := c.bus.In(port + 1)
		b2 := c.bus.In(port + 2)
		b3 := c.bus.In(port + 3)
		c.EAX = uint32(b0) | (uint32(b1) << 8) | (uint32(b2) << 16) | (uint32(b3) << 24)
	}
	c.Cycles += 5
}

func (c *CPU_X86) opOUT_DX_AL() {
	c.bus.Out(c.DX(), c.AL())
	c.Cycles += 5
}

func (c *CPU_X86) opOUT_DX_AX() {
	port := c.DX()
	if c.prefixOpSize {
		c.bus.Out(port, byte(c.AX()))
		c.bus.Out(port+1, byte(c.AX()>>8))
	} else {
		c.bus.Out(port, byte(c.EAX))
		c.bus.Out(port+1, byte(c.EAX>>8))
		c.bus.Out(port+2, byte(c.EAX>>16))
		c.bus.Out(port+3, byte(c.EAX>>24))
	}
	c.Cycles += 5
}

// =============================================================================
// String Instructions
// =============================================================================

func (c *CPU_X86) stringDirection() int32 {
	if c.DF() {
		return -1
	}
	return 1
}

func (c *CPU_X86) opMOVSB() {
	src := c.ESI
	dst := c.EDI
	delta := c.stringDirection()

	count := uint32(1)
	if c.prefixRep > 0 {
		count = c.ECX
	}

	for count > 0 {
		c.write8(dst, c.read8(src))
		src = uint32(int32(src) + delta)
		dst = uint32(int32(dst) + delta)
		count--
		c.Cycles++
	}

	c.ESI = src
	c.EDI = dst
	if c.prefixRep > 0 {
		c.ECX = 0
	}
}

func (c *CPU_X86) opMOVSW() {
	src := c.ESI
	dst := c.EDI
	var delta int32
	if c.prefixOpSize {
		delta = c.stringDirection() * 2
	} else {
		delta = c.stringDirection() * 4
	}

	count := uint32(1)
	if c.prefixRep > 0 {
		count = c.ECX
	}

	for count > 0 {
		if c.prefixOpSize {
			c.write16(dst, c.read16(src))
		} else {
			c.write32(dst, c.read32(src))
		}
		src = uint32(int32(src) + delta)
		dst = uint32(int32(dst) + delta)
		count--
		c.Cycles++
	}

	c.ESI = src
	c.EDI = dst
	if c.prefixRep > 0 {
		c.ECX = 0
	}
}

func (c *CPU_X86) opSTOSB() {
	dst := c.EDI
	delta := c.stringDirection()

	count := uint32(1)
	if c.prefixRep > 0 {
		count = c.ECX
	}

	for count > 0 {
		c.write8(dst, c.AL())
		dst = uint32(int32(dst) + delta)
		count--
		c.Cycles++
	}

	c.EDI = dst
	if c.prefixRep > 0 {
		c.ECX = 0
	}
}

func (c *CPU_X86) opSTOSW() {
	dst := c.EDI
	var delta int32
	if c.prefixOpSize {
		delta = c.stringDirection() * 2
	} else {
		delta = c.stringDirection() * 4
	}

	count := uint32(1)
	if c.prefixRep > 0 {
		count = c.ECX
	}

	for count > 0 {
		if c.prefixOpSize {
			c.write16(dst, c.AX())
		} else {
			c.write32(dst, c.EAX)
		}
		dst = uint32(int32(dst) + delta)
		count--
		c.Cycles++
	}

	c.EDI = dst
	if c.prefixRep > 0 {
		c.ECX = 0
	}
}

func (c *CPU_X86) opLODSB() {
	src := c.ESI
	delta := c.stringDirection()

	count := uint32(1)
	if c.prefixRep > 0 {
		count = c.ECX
	}

	for count > 0 {
		c.SetAL(c.read8(src))
		src = uint32(int32(src) + delta)
		count--
		c.Cycles++
	}

	c.ESI = src
	if c.prefixRep > 0 {
		c.ECX = 0
	}
}

func (c *CPU_X86) opLODSW() {
	src := c.ESI
	var delta int32
	if c.prefixOpSize {
		delta = c.stringDirection() * 2
	} else {
		delta = c.stringDirection() * 4
	}

	count := uint32(1)
	if c.prefixRep > 0 {
		count = c.ECX
	}

	for count > 0 {
		if c.prefixOpSize {
			c.SetAX(c.read16(src))
		} else {
			c.EAX = c.read32(src)
		}
		src = uint32(int32(src) + delta)
		count--
		c.Cycles++
	}

	c.ESI = src
	if c.prefixRep > 0 {
		c.ECX = 0
	}
}

func (c *CPU_X86) opCMPSB() {
	src := c.ESI
	dst := c.EDI
	delta := c.stringDirection()

	for {
		a := c.read8(src)
		b := c.read8(dst)
		result := uint16(a) - uint16(b)
		c.setFlagsArith8(result, a, b, true)

		src = uint32(int32(src) + delta)
		dst = uint32(int32(dst) + delta)
		c.Cycles++

		if c.prefixRep == 0 {
			break
		}
		c.ECX--
		if c.ECX == 0 {
			break
		}
		// REPE (rep == 1): continue while ZF=1
		// REPNE (rep == 2): continue while ZF=0
		if (c.prefixRep == 1 && !c.ZF()) || (c.prefixRep == 2 && c.ZF()) {
			break
		}
	}

	c.ESI = src
	c.EDI = dst
}

func (c *CPU_X86) opCMPSW() {
	src := c.ESI
	dst := c.EDI
	var delta int32
	if c.prefixOpSize {
		delta = c.stringDirection() * 2
	} else {
		delta = c.stringDirection() * 4
	}

	for {
		if c.prefixOpSize {
			a := c.read16(src)
			b := c.read16(dst)
			result := uint32(a) - uint32(b)
			c.setFlagsArith16(result, a, b, true)
		} else {
			a := c.read32(src)
			b := c.read32(dst)
			result := uint64(a) - uint64(b)
			c.setFlagsArith32(result, a, b, true)
		}

		src = uint32(int32(src) + delta)
		dst = uint32(int32(dst) + delta)
		c.Cycles++

		if c.prefixRep == 0 {
			break
		}
		c.ECX--
		if c.ECX == 0 {
			break
		}
		if (c.prefixRep == 1 && !c.ZF()) || (c.prefixRep == 2 && c.ZF()) {
			break
		}
	}

	c.ESI = src
	c.EDI = dst
}

func (c *CPU_X86) opSCASB() {
	dst := c.EDI
	delta := c.stringDirection()

	for {
		a := c.AL()
		b := c.read8(dst)
		result := uint16(a) - uint16(b)
		c.setFlagsArith8(result, a, b, true)

		dst = uint32(int32(dst) + delta)
		c.Cycles++

		if c.prefixRep == 0 {
			break
		}
		c.ECX--
		if c.ECX == 0 {
			break
		}
		if (c.prefixRep == 1 && !c.ZF()) || (c.prefixRep == 2 && c.ZF()) {
			break
		}
	}

	c.EDI = dst
}

func (c *CPU_X86) opSCASW() {
	dst := c.EDI
	var delta int32
	if c.prefixOpSize {
		delta = c.stringDirection() * 2
	} else {
		delta = c.stringDirection() * 4
	}

	for {
		if c.prefixOpSize {
			a := c.AX()
			b := c.read16(dst)
			result := uint32(a) - uint32(b)
			c.setFlagsArith16(result, a, b, true)
		} else {
			a := c.EAX
			b := c.read32(dst)
			result := uint64(a) - uint64(b)
			c.setFlagsArith32(result, a, b, true)
		}

		dst = uint32(int32(dst) + delta)
		c.Cycles++

		if c.prefixRep == 0 {
			break
		}
		c.ECX--
		if c.ECX == 0 {
			break
		}
		if (c.prefixRep == 1 && !c.ZF()) || (c.prefixRep == 2 && c.ZF()) {
			break
		}
	}

	c.EDI = dst
}

func (c *CPU_X86) opINSB() {
	dst := c.EDI
	port := c.DX()
	delta := c.stringDirection()

	count := uint32(1)
	if c.prefixRep > 0 {
		count = c.ECX
	}

	for count > 0 {
		c.write8(dst, c.bus.In(port))
		dst = uint32(int32(dst) + delta)
		count--
		c.Cycles++
	}

	c.EDI = dst
	if c.prefixRep > 0 {
		c.ECX = 0
	}
}

func (c *CPU_X86) opINSW() {
	dst := c.EDI
	port := c.DX()
	var delta int32
	if c.prefixOpSize {
		delta = c.stringDirection() * 2
	} else {
		delta = c.stringDirection() * 4
	}

	count := uint32(1)
	if c.prefixRep > 0 {
		count = c.ECX
	}

	for count > 0 {
		if c.prefixOpSize {
			lo := c.bus.In(port)
			hi := c.bus.In(port + 1)
			c.write16(dst, uint16(lo)|(uint16(hi)<<8))
		} else {
			b0 := c.bus.In(port)
			b1 := c.bus.In(port + 1)
			b2 := c.bus.In(port + 2)
			b3 := c.bus.In(port + 3)
			c.write32(dst, uint32(b0)|(uint32(b1)<<8)|(uint32(b2)<<16)|(uint32(b3)<<24))
		}
		dst = uint32(int32(dst) + delta)
		count--
		c.Cycles++
	}

	c.EDI = dst
	if c.prefixRep > 0 {
		c.ECX = 0
	}
}

func (c *CPU_X86) opOUTSB() {
	src := c.ESI
	port := c.DX()
	delta := c.stringDirection()

	count := uint32(1)
	if c.prefixRep > 0 {
		count = c.ECX
	}

	for count > 0 {
		c.bus.Out(port, c.read8(src))
		src = uint32(int32(src) + delta)
		count--
		c.Cycles++
	}

	c.ESI = src
	if c.prefixRep > 0 {
		c.ECX = 0
	}
}

func (c *CPU_X86) opOUTSW() {
	src := c.ESI
	port := c.DX()
	var delta int32
	if c.prefixOpSize {
		delta = c.stringDirection() * 2
	} else {
		delta = c.stringDirection() * 4
	}

	count := uint32(1)
	if c.prefixRep > 0 {
		count = c.ECX
	}

	for count > 0 {
		if c.prefixOpSize {
			v := c.read16(src)
			c.bus.Out(port, byte(v))
			c.bus.Out(port+1, byte(v>>8))
		} else {
			v := c.read32(src)
			c.bus.Out(port, byte(v))
			c.bus.Out(port+1, byte(v>>8))
			c.bus.Out(port+2, byte(v>>16))
			c.bus.Out(port+3, byte(v>>24))
		}
		src = uint32(int32(src) + delta)
		count--
		c.Cycles++
	}

	c.ESI = src
	if c.prefixRep > 0 {
		c.ECX = 0
	}
}

// =============================================================================
// ENTER/LEAVE Instructions
// =============================================================================

func (c *CPU_X86) opENTER() {
	size := c.fetch16()
	level := c.fetch8() & 0x1F

	if c.prefixOpSize {
		c.push16(c.BP())
		framePtr := c.SP()
		if level > 0 {
			for i := byte(1); i < level; i++ {
				c.EBP -= 2
				c.push16(c.read16(c.EBP))
			}
			c.push16(framePtr)
		}
		c.SetBP(framePtr)
		c.SetSP(c.SP() - size)
	} else {
		c.push32(c.EBP)
		framePtr := c.ESP
		if level > 0 {
			for i := byte(1); i < level; i++ {
				c.EBP -= 4
				c.push32(c.read32(c.EBP))
			}
			c.push32(framePtr)
		}
		c.EBP = framePtr
		c.ESP -= uint32(size)
	}
	c.Cycles += 10
}

func (c *CPU_X86) opLEAVE() {
	if c.prefixOpSize {
		c.SetSP(c.BP())
		c.SetBP(c.pop16())
	} else {
		c.ESP = c.EBP
		c.EBP = c.pop32()
	}
	c.Cycles += 2
}

// =============================================================================
// Miscellaneous Instructions
// =============================================================================

func (c *CPU_X86) opHLT() {
	c.Halted = true
	c.Cycles += 1
}

func (c *CPU_X86) opWAIT() {
	// FPU wait - just a NOP for now
	c.Cycles += 1
}

func (c *CPU_X86) opFPU_escape() {
	// FPU opcodes - skip ModR/M and ignore
	c.fetchModRM()
	if c.getModRMMod() != 3 {
		// Memory operand - skip effective address calculation
		c.getEffectiveAddress()
	}
	c.Cycles += 1
}
