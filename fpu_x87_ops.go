package main

import "math"

var x87BinaryOpTable = [8]func(a, b float64) float64{
	0: func(a, b float64) float64 { return a + b }, // FADD
	1: func(a, b float64) float64 { return a * b }, // FMUL
	4: func(a, b float64) float64 { return a - b }, // FSUB
	5: func(a, b float64) float64 { return b - a }, // FSUBR
	6: func(a, b float64) float64 { return a / b }, // FDIV
	7: func(a, b float64) float64 { return b / a }, // FDIVR
}

func x87CheckBinaryExceptions(f *FPU_X87, op int, r, a, b float64) {
	bits := math.Float64bits(r)
	exp := bits & 0x7FF0000000000000
	frac := bits & 0x000FFFFFFFFFFFFF
	if exp == 0x7FF0000000000000 {
		if frac != 0 {
			f.setException(x87FSW_IE) // NaN result
		} else {
			f.setException(x87FSW_OE) // Inf result
		}
	}
	if op >= 6 { // FDIV or FDIVR
		den := b
		if op == 7 {
			den = a
		}
		if den == 0 {
			f.setException(x87FSW_ZE)
		}
	}
}

func (c *CPU_X86) x87RegPair() (int, int) {
	return int(c.getModRMReg()), int(c.getModRMRM())
}

func (c *CPU_X86) x87FetchOp(esc byte) {
	if c.FPU == nil {
		c.Cycles += 1
		return
	}
	modrm := c.fetchModRM()
	c.FPU.captureOp(c, esc, modrm)
}

func (c *CPU_X86) x87MemAddr() uint32 {
	addr := c.getEffectiveAddress()
	c.FPU.captureMemAccess(c, addr)
	return addr
}

func (c *CPU_X86) x87MemAddrNoCapture() uint32 {
	return c.getEffectiveAddress()
}

func (c *CPU_X86) x87BinaryST0STi(op int, i int) {
	f := c.FPU
	t := f.top()
	p0 := t & 7
	pi := (t + i) & 7
	if f.getTag(p0) == x87TagEmpty || f.getTag(pi) == x87TagEmpty {
		f.setException(x87FSW_IE | x87FSW_SF)
		f.FSW &^= x87FSW_C1
		return
	}
	if op < 0 || op >= len(x87BinaryOpTable) {
		return
	}
	fn := x87BinaryOpTable[op]
	if fn == nil {
		return
	}
	a, b := f.regs[p0], f.regs[pi]
	r := fn(a, b)
	x87CheckBinaryExceptions(f, op, r, a, b)
	f.regs[p0] = r
	f.setTag(p0, f.classifyTag(r))
}

func (c *CPU_X86) x87BinarySTiST0(op int, i int) {
	f := c.FPU
	t := f.top()
	p0 := t & 7
	pi := (t + i) & 7
	if f.getTag(p0) == x87TagEmpty || f.getTag(pi) == x87TagEmpty {
		f.setException(x87FSW_IE | x87FSW_SF)
		f.FSW &^= x87FSW_C1
		return
	}
	if op < 0 || op >= len(x87BinaryOpTable) {
		return
	}
	fn := x87BinaryOpTable[op]
	if fn == nil {
		return
	}
	a, b := f.regs[pi], f.regs[p0]
	r := fn(a, b)
	x87CheckBinaryExceptions(f, op, r, a, b)
	f.regs[pi] = r
	f.setTag(pi, f.classifyTag(r))
}

func (c *CPU_X86) x87BinaryMem(op int, v float64) {
	f := c.FPU
	p0 := f.top() & 7
	if f.getTag(p0) == x87TagEmpty {
		f.setException(x87FSW_IE | x87FSW_SF)
		f.FSW &^= x87FSW_C1
		return
	}
	if op < 0 || op >= len(x87BinaryOpTable) {
		return
	}
	fn := x87BinaryOpTable[op]
	if fn == nil {
		return
	}
	a := f.regs[p0]
	r := fn(a, v)
	x87CheckBinaryExceptions(f, op, r, a, v)
	f.regs[p0] = r
	f.setTag(p0, f.classifyTag(r))
}

func (c *CPU_X86) opFPU_D8() {
	c.x87FetchOp(0xD8)
	if c.FPU == nil {
		return
	}
	reg, rm := c.x87RegPair()
	if c.getModRMMod() == 3 {
		switch reg {
		case 0, 1, 4, 5, 6, 7:
			c.x87BinaryST0STi(reg, rm)
		case 2:
			if !c.FPU.checkStackUnderflow(0) && !c.FPU.checkStackUnderflow(rm) {
				c.FPU.doCompare(c.FPU.ST(0), c.FPU.ST(rm), true)
			}
		case 3:
			if !c.FPU.checkStackUnderflow(0) && !c.FPU.checkStackUnderflow(rm) {
				c.FPU.doCompare(c.FPU.ST(0), c.FPU.ST(rm), true)
				c.FPU.pop()
			}
		}
	} else {
		addr := c.x87MemAddr()
		v := c.FPU.loadFloat32(c.bus, addr)
		switch reg {
		case 0, 1, 4, 5, 6, 7:
			c.x87BinaryMem(reg, v)
		case 2:
			if !c.FPU.checkStackUnderflow(0) {
				c.FPU.doCompare(c.FPU.ST(0), v, true)
			}
		case 3:
			if !c.FPU.checkStackUnderflow(0) {
				c.FPU.doCompare(c.FPU.ST(0), v, true)
				c.FPU.pop()
			}
		}
	}
	c.Cycles += 1
}

var x87D9RegOps [64]func(*CPU_X86)

func init() {
	// FLD ST(i): 0xC0-0xC7 → indices 0x00-0x07
	for i := range 8 {
		idx := i
		x87D9RegOps[idx] = func(c *CPU_X86) {
			f := c.FPU
			if !f.checkStackUnderflow(idx) {
				f.push(f.ST(idx))
			}
		}
	}
	// FXCH ST(i): 0xC8-0xCF → indices 0x08-0x0F
	for i := range 8 {
		idx := i
		x87D9RegOps[0x08+idx] = func(c *CPU_X86) {
			f := c.FPU
			if !f.checkStackUnderflow(0) && !f.checkStackUnderflow(idx) {
				a := f.ST(0)
				b := f.ST(idx)
				f.setST(0, b)
				f.setST(idx, a)
			}
		}
	}
	// FNOP: 0xD0 → index 0x10
	x87D9RegOps[0x10] = func(c *CPU_X86) {}
	// FCHS: 0xE0 → index 0x20
	x87D9RegOps[0x20] = func(c *CPU_X86) {
		f := c.FPU
		if !f.checkStackUnderflow(0) {
			f.setST(0, -f.ST(0))
		}
	}
	// FABS: 0xE1 → index 0x21
	x87D9RegOps[0x21] = func(c *CPU_X86) {
		f := c.FPU
		if !f.checkStackUnderflow(0) {
			f.setST(0, math.Abs(f.ST(0)))
		}
	}
	// FTST: 0xE4 → index 0x24
	x87D9RegOps[0x24] = func(c *CPU_X86) {
		f := c.FPU
		if !f.checkStackUnderflow(0) {
			f.doCompare(f.ST(0), 0, true)
		}
	}
	// FXAM: 0xE5 → index 0x25
	x87D9RegOps[0x25] = func(c *CPU_X86) {
		f := c.FPU
		top := f.top()
		f.xam(f.regs[top], f.getTag(top) == x87TagEmpty)
	}
	// Constants: 0xE8-0xEE → indices 0x28-0x2E
	for i := range 7 {
		idx := i
		x87D9RegOps[0x28+idx] = func(c *CPU_X86) {
			c.FPU.push(x87ConstTable[idx])
		}
	}
	// F2XM1: 0xF0 → index 0x30
	x87D9RegOps[0x30] = func(c *CPU_X86) {
		f := c.FPU
		if !f.checkStackUnderflow(0) {
			f.setST(0, math.Exp2(f.ST(0))-1.0)
		}
	}
	// FYL2X: 0xF1 → index 0x31
	x87D9RegOps[0x31] = func(c *CPU_X86) {
		f := c.FPU
		if !f.checkStackUnderflow(0) && !f.checkStackUnderflow(1) {
			x := f.ST(0)
			y := f.ST(1)
			if x < 0 {
				f.setException(x87FSW_IE)
			} else if x == 0 && !math.IsNaN(y) && !math.IsInf(y, 0) && y != 0 {
				f.setException(x87FSW_ZE)
			}
			f.setST(1, y*math.Log2(x))
			f.pop()
		}
	}
	// FPTAN: 0xF2 → index 0x32
	x87D9RegOps[0x32] = func(c *CPU_X86) {
		f := c.FPU
		if !f.checkStackUnderflow(0) {
			f.FSW &^= x87FSW_C2
			f.setST(0, math.Tan(f.ST(0)))
			f.push(1.0)
		}
	}
	// FPATAN: 0xF3 → index 0x33
	x87D9RegOps[0x33] = func(c *CPU_X86) {
		f := c.FPU
		if !f.checkStackUnderflow(0) && !f.checkStackUnderflow(1) {
			f.setST(1, math.Atan2(f.ST(1), f.ST(0)))
			f.pop()
		}
	}
	// FXTRACT: 0xF4 → index 0x34
	x87D9RegOps[0x34] = func(c *CPU_X86) {
		f := c.FPU
		if !f.checkStackUnderflow(0) {
			x := f.ST(0)
			if x == 0 {
				f.push(math.Inf(-1))
				f.setST(1, 0)
			} else {
				frac, exp := math.Frexp(x)
				f.setST(0, frac*2)
				f.push(float64(exp - 1))
			}
		}
	}
	// FPREM1: 0xF5 → index 0x35
	x87D9RegOps[0x35] = func(c *CPU_X86) {
		f := c.FPU
		if !f.checkStackUnderflow(0) && !f.checkStackUnderflow(1) {
			a := f.ST(0)
			b := f.ST(1)
			q := int64(math.RoundToEven(a / b))
			f.setST(0, math.Remainder(a, b))
			f.FSW &^= x87FSW_C2
			f.setQuotientFlags(q)
		}
	}
	// FDECSTP: 0xF6 → index 0x36
	x87D9RegOps[0x36] = func(c *CPU_X86) {
		f := c.FPU
		f.setTop((f.top() - 1) & 7)
	}
	// FINCSTP: 0xF7 → index 0x37
	x87D9RegOps[0x37] = func(c *CPU_X86) {
		f := c.FPU
		f.setTop((f.top() + 1) & 7)
	}
	// FPREM: 0xF8 → index 0x38
	x87D9RegOps[0x38] = func(c *CPU_X86) {
		f := c.FPU
		if !f.checkStackUnderflow(0) && !f.checkStackUnderflow(1) {
			a := f.ST(0)
			b := f.ST(1)
			q := int64(math.Trunc(a / b))
			f.setST(0, a-float64(q)*b)
			f.FSW &^= x87FSW_C2
			f.setQuotientFlags(q)
		}
	}
	// FYL2XP1: 0xF9 → index 0x39
	x87D9RegOps[0x39] = func(c *CPU_X86) {
		f := c.FPU
		if !f.checkStackUnderflow(0) && !f.checkStackUnderflow(1) {
			x := f.ST(0)
			y := f.ST(1)
			if x <= -1 {
				f.setException(x87FSW_IE)
			}
			f.setST(1, y*math.Log1p(x)/math.Ln2)
			f.pop()
		}
	}
	// FSQRT: 0xFA → index 0x3A
	x87D9RegOps[0x3A] = func(c *CPU_X86) {
		f := c.FPU
		if !f.checkStackUnderflow(0) {
			x := f.ST(0)
			if x < 0 {
				f.setException(x87FSW_IE)
			}
			f.setST(0, math.Sqrt(x))
		}
	}
	// FSINCOS: 0xFB → index 0x3B
	x87D9RegOps[0x3B] = func(c *CPU_X86) {
		f := c.FPU
		if !f.checkStackUnderflow(0) {
			x := f.ST(0)
			s := math.Sin(x)
			co := math.Cos(x)
			f.setST(0, s)
			f.push(co)
			f.FSW &^= x87FSW_C2
		}
	}
	// FRNDINT: 0xFC → index 0x3C
	x87D9RegOps[0x3C] = func(c *CPU_X86) {
		f := c.FPU
		if !f.checkStackUnderflow(0) {
			f.setST(0, f.roundPerFCW(f.ST(0)))
		}
	}
	// FSCALE: 0xFD → index 0x3D
	x87D9RegOps[0x3D] = func(c *CPU_X86) {
		f := c.FPU
		if !f.checkStackUnderflow(0) && !f.checkStackUnderflow(1) {
			scale := int(f.ST(1))
			f.setST(0, math.Ldexp(f.ST(0), scale))
		}
	}
	// FSIN: 0xFE → index 0x3E
	x87D9RegOps[0x3E] = func(c *CPU_X86) {
		f := c.FPU
		if !f.checkStackUnderflow(0) {
			f.setST(0, math.Sin(f.ST(0)))
			f.FSW &^= x87FSW_C2
		}
	}
	// FCOS: 0xFF → index 0x3F
	x87D9RegOps[0x3F] = func(c *CPU_X86) {
		f := c.FPU
		if !f.checkStackUnderflow(0) {
			f.setST(0, math.Cos(f.ST(0)))
			f.FSW &^= x87FSW_C2
		}
	}
}

func (c *CPU_X86) opFPU_D9() {
	c.x87FetchOp(0xD9)
	if c.FPU == nil {
		return
	}
	f := c.FPU

	if c.getModRMMod() == 3 {
		if fn := x87D9RegOps[c.modrm-0xC0]; fn != nil {
			fn(c)
		}
	} else {
		reg := int(c.getModRMReg())
		switch reg {
		case 0: // FLD m32
			addr := c.x87MemAddr()
			f.push(f.loadFloat32(c.bus, addr))
		case 2: // FST m32
			addr := c.x87MemAddr()
			if !f.checkStackUnderflow(0) {
				f.storeFloat32(c.bus, addr, f.ST(0))
			}
		case 3: // FSTP m32
			addr := c.x87MemAddr()
			if !f.checkStackUnderflow(0) {
				f.storeFloat32(c.bus, addr, f.ST(0))
				f.pop()
			}
		case 4: // FLDENV
			addr := c.x87MemAddrNoCapture()
			f.fldenv32(c.bus, addr)
		case 5: // FLDCW
			addr := c.x87MemAddrNoCapture()
			f.FCW = uint16(c.read16(addr))
		case 6: // FNSTENV
			addr := c.x87MemAddrNoCapture()
			f.fnstenv32(c.bus, addr)
		case 7: // FNSTCW
			addr := c.x87MemAddrNoCapture()
			c.write16(addr, f.FCW)
		}
	}
	c.Cycles += 1
}

func (c *CPU_X86) opFPU_DA() {
	c.x87FetchOp(0xDA)
	if c.FPU == nil {
		return
	}
	if c.getModRMMod() == 3 {
		c.Cycles += 1
		return
	}
	reg := int(c.getModRMReg())
	addr := c.x87MemAddr()
	v := c.FPU.loadInt32(c.bus, addr)
	switch reg {
	case 0, 1, 4, 5, 6, 7:
		c.x87BinaryMem(reg, v)
	case 2:
		if !c.FPU.checkStackUnderflow(0) {
			c.FPU.doCompare(c.FPU.ST(0), v, true)
		}
	case 3:
		if !c.FPU.checkStackUnderflow(0) {
			c.FPU.doCompare(c.FPU.ST(0), v, true)
			c.FPU.pop()
		}
	}
	c.Cycles += 1
}

func (c *CPU_X86) opFPU_DB() {
	c.x87FetchOp(0xDB)
	if c.FPU == nil {
		return
	}
	f := c.FPU
	if c.getModRMMod() == 3 {
		switch c.modrm {
		case 0xE2: // FNCLEX
			f.FSW &^= 0x80FF
		case 0xE3: // FNINIT
			f.Reset()
		}
		c.Cycles += 1
		return
	}
	reg := int(c.getModRMReg())
	addr := c.x87MemAddr()
	switch reg {
	case 0: // FILD m32int
		f.push(f.loadInt32(c.bus, addr))
	case 2: // FIST m32int
		if !f.checkStackUnderflow(0) {
			f.storeInt32(c.bus, addr, f.ST(0))
		}
	case 3: // FISTP m32int
		if !f.checkStackUnderflow(0) {
			f.storeInt32(c.bus, addr, f.ST(0))
			f.pop()
		}
	case 5: // FLD m80
		f.push(f.loadExtended80(c.bus, addr))
	case 7: // FSTP m80
		if !f.checkStackUnderflow(0) {
			f.storeExtended80(c.bus, addr, f.ST(0))
			f.pop()
		}
	}
	c.Cycles += 1
}

func (c *CPU_X86) opFPU_DC() {
	c.x87FetchOp(0xDC)
	if c.FPU == nil {
		return
	}
	reg, rm := c.x87RegPair()
	if c.getModRMMod() == 3 {
		switch reg {
		case 0, 1:
			c.x87BinarySTiST0(reg, rm)
		case 4:
			c.x87BinarySTiST0(5, rm) // FSUBR encoding is swapped in DC register form
		case 5:
			c.x87BinarySTiST0(4, rm) // FSUB encoding is swapped in DC register form
		case 6:
			c.x87BinarySTiST0(7, rm) // FDIVR encoding is swapped in DC register form
		case 7:
			c.x87BinarySTiST0(6, rm) // FDIV encoding is swapped in DC register form
		case 2:
			if !c.FPU.checkStackUnderflow(0) && !c.FPU.checkStackUnderflow(rm) {
				c.FPU.doCompare(c.FPU.ST(rm), c.FPU.ST(0), true)
			}
		case 3:
			if !c.FPU.checkStackUnderflow(0) && !c.FPU.checkStackUnderflow(rm) {
				c.FPU.doCompare(c.FPU.ST(rm), c.FPU.ST(0), true)
				c.FPU.pop()
			}
		}
	} else {
		addr := c.x87MemAddr()
		v := c.FPU.loadFloat64(c.bus, addr)
		switch reg {
		case 0, 1, 4, 5, 6, 7:
			c.x87BinaryMem(reg, v)
		case 2:
			if !c.FPU.checkStackUnderflow(0) {
				c.FPU.doCompare(c.FPU.ST(0), v, true)
			}
		case 3:
			if !c.FPU.checkStackUnderflow(0) {
				c.FPU.doCompare(c.FPU.ST(0), v, true)
				c.FPU.pop()
			}
		}
	}
	c.Cycles += 1
}

func (c *CPU_X86) opFPU_DD() {
	c.x87FetchOp(0xDD)
	if c.FPU == nil {
		return
	}
	f := c.FPU
	if c.getModRMMod() == 3 {
		reg, rm := c.x87RegPair()
		switch reg {
		case 0: // FFREE
			f.setTag(f.physReg(rm), x87TagEmpty)
		case 3: // FSTP ST(i)
			if !f.checkStackUnderflow(0) && !f.checkStackUnderflow(rm) {
				f.setST(rm, f.ST(0))
				f.pop()
			}
		case 4: // FUCOM
			if !f.checkStackUnderflow(0) && !f.checkStackUnderflow(rm) {
				f.doCompare(f.ST(0), f.ST(rm), false)
			}
		case 5: // FUCOMP
			if !f.checkStackUnderflow(0) && !f.checkStackUnderflow(rm) {
				f.doCompare(f.ST(0), f.ST(rm), false)
				f.pop()
			}
		}
		c.Cycles += 1
		return
	}
	reg := int(c.getModRMReg())
	switch reg {
	case 0: // FLD m64
		addr := c.x87MemAddr()
		f.push(f.loadFloat64(c.bus, addr))
	case 2: // FST m64
		addr := c.x87MemAddr()
		if !f.checkStackUnderflow(0) {
			f.storeFloat64(c.bus, addr, f.ST(0))
		}
	case 3: // FSTP m64
		addr := c.x87MemAddr()
		if !f.checkStackUnderflow(0) {
			f.storeFloat64(c.bus, addr, f.ST(0))
			f.pop()
		}
	case 4: // FRSTOR
		addr := c.x87MemAddrNoCapture()
		f.frstor32(c.bus, addr)
	case 6: // FNSAVE
		addr := c.x87MemAddrNoCapture()
		f.fsave32(c.bus, addr)
	case 7: // FNSTSW m16
		addr := c.x87MemAddrNoCapture()
		c.write16(addr, f.FSW)
	}
	c.Cycles += 1
}

func (c *CPU_X86) opFPU_DE() {
	c.x87FetchOp(0xDE)
	if c.FPU == nil {
		return
	}
	if c.getModRMMod() == 3 {
		reg, rm := c.x87RegPair()
		switch reg {
		case 0, 1:
			c.x87BinarySTiST0(reg, rm)
			c.FPU.pop()
		case 4:
			c.x87BinarySTiST0(5, rm) // FSUBRP swapped encoding
			c.FPU.pop()
		case 5:
			c.x87BinarySTiST0(4, rm) // FSUBP swapped encoding
			c.FPU.pop()
		case 6:
			c.x87BinarySTiST0(7, rm) // FDIVRP swapped encoding
			c.FPU.pop()
		case 7:
			c.x87BinarySTiST0(6, rm) // FDIVP swapped encoding
			c.FPU.pop()
		case 3: // FCOMPP
			if !c.FPU.checkStackUnderflow(0) && !c.FPU.checkStackUnderflow(1) {
				c.FPU.doCompare(c.FPU.ST(0), c.FPU.ST(1), true)
				c.FPU.pop()
				c.FPU.pop()
			}
		}
		c.Cycles += 1
		return
	}
	reg := int(c.getModRMReg())
	addr := c.x87MemAddr()
	v := c.FPU.loadInt16(c.bus, addr)
	switch reg {
	case 0, 1, 4, 5, 6, 7:
		c.x87BinaryMem(reg, v)
	case 2:
		if !c.FPU.checkStackUnderflow(0) {
			c.FPU.doCompare(c.FPU.ST(0), v, true)
		}
	case 3:
		if !c.FPU.checkStackUnderflow(0) {
			c.FPU.doCompare(c.FPU.ST(0), v, true)
			c.FPU.pop()
		}
	}
	c.Cycles += 1
}

func (c *CPU_X86) opFPU_DF() {
	c.x87FetchOp(0xDF)
	if c.FPU == nil {
		return
	}
	f := c.FPU
	if c.getModRMMod() == 3 {
		if c.modrm == 0xE0 { // FNSTSW AX
			c.SetAX(f.FSW)
		}
		c.Cycles += 1
		return
	}
	reg := int(c.getModRMReg())
	addr := c.x87MemAddr()
	switch reg {
	case 0: // FILD m16
		f.push(f.loadInt16(c.bus, addr))
	case 2: // FIST m16
		if !f.checkStackUnderflow(0) {
			f.storeInt16(c.bus, addr, f.ST(0))
		}
	case 3: // FISTP m16
		if !f.checkStackUnderflow(0) {
			f.storeInt16(c.bus, addr, f.ST(0))
			f.pop()
		}
	case 4: // FBLD
		f.push(f.loadBCD(c.bus, addr))
	case 5: // FILD m64
		f.push(f.loadInt64(c.bus, addr))
	case 6: // FBSTP
		if !f.checkStackUnderflow(0) {
			f.storeBCD(c.bus, addr, f.ST(0))
			f.pop()
		}
	case 7: // FISTP m64
		if !f.checkStackUnderflow(0) {
			f.storeInt64(c.bus, addr, f.ST(0))
			f.pop()
		}
	}
	c.Cycles += 1
}
