package main

import "math"

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
	if f.checkStackUnderflow(0) || f.checkStackUnderflow(i) {
		return
	}
	a := f.ST(0)
	b := f.ST(i)
	var r float64
	den := b
	switch op {
	case 0:
		r = a + b
	case 1:
		r = a * b
	case 4:
		r = a - b
	case 5:
		r = b - a
	case 6:
		r = a / b
	case 7:
		r = b / a
		den = a
	}
	if math.IsInf(r, 0) {
		f.setException(x87FSW_OE)
	}
	if math.IsNaN(r) {
		f.setException(x87FSW_IE)
	}
	if (op == 6 || op == 7) && den == 0 {
		f.setException(x87FSW_ZE)
	}
	f.setST(0, r)
}

func (c *CPU_X86) x87BinarySTiST0(op int, i int) {
	f := c.FPU
	if f.checkStackUnderflow(0) || f.checkStackUnderflow(i) {
		return
	}
	a := f.ST(i)
	b := f.ST(0)
	var r float64
	den := b
	switch op {
	case 0:
		r = a + b
	case 1:
		r = a * b
	case 4:
		r = a - b
	case 5:
		r = b - a
	case 6:
		r = a / b
	case 7:
		r = b / a
		den = a
	}
	if math.IsInf(r, 0) {
		f.setException(x87FSW_OE)
	}
	if math.IsNaN(r) {
		f.setException(x87FSW_IE)
	}
	if (op == 6 || op == 7) && den == 0 {
		f.setException(x87FSW_ZE)
	}
	f.setST(i, r)
}

func (c *CPU_X86) x87BinaryMem(op int, v float64) {
	f := c.FPU
	if f.checkStackUnderflow(0) {
		return
	}
	a := f.ST(0)
	var r float64
	den := v
	switch op {
	case 0:
		r = a + v
	case 1:
		r = a * v
	case 4:
		r = a - v
	case 5:
		r = v - a
	case 6:
		r = a / v
	case 7:
		r = v / a
		den = a
	}
	if math.IsNaN(r) {
		f.setException(x87FSW_IE)
	}
	if math.IsInf(r, 0) {
		f.setException(x87FSW_OE)
	}
	if (op == 6 || op == 7) && den == 0 {
		f.setException(x87FSW_ZE)
	}
	f.setST(0, r)
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

func (c *CPU_X86) opFPU_D9() {
	c.x87FetchOp(0xD9)
	if c.FPU == nil {
		return
	}
	mod := c.getModRMMod()
	reg, rm := c.x87RegPair()
	f := c.FPU

	if mod == 3 {
		switch {
		case c.modrm >= 0xC0 && c.modrm <= 0xC7: // FLD ST(i)
			if !f.checkStackUnderflow(rm) {
				f.push(f.ST(rm))
			}
		case c.modrm >= 0xC8 && c.modrm <= 0xCF: // FXCH ST(i)
			if !f.checkStackUnderflow(0) && !f.checkStackUnderflow(rm) {
				a := f.ST(0)
				b := f.ST(rm)
				f.setST(0, b)
				f.setST(rm, a)
			}
		case c.modrm == 0xD0: // FNOP
		case c.modrm == 0xE0: // FCHS
			if !f.checkStackUnderflow(0) {
				f.setST(0, -f.ST(0))
			}
		case c.modrm == 0xE1: // FABS
			if !f.checkStackUnderflow(0) {
				f.setST(0, math.Abs(f.ST(0)))
			}
		case c.modrm == 0xE4: // FTST
			if !f.checkStackUnderflow(0) {
				f.doCompare(f.ST(0), 0, true)
			}
		case c.modrm == 0xE5: // FXAM
			top := f.top()
			f.xam(f.regs[top], f.getTag(top) == x87TagEmpty)
		case c.modrm >= 0xE8 && c.modrm <= 0xEE: // constants
			f.push(x87ConstTable[c.modrm-0xE8])
		case c.modrm == 0xF0: // F2XM1
			if !f.checkStackUnderflow(0) {
				f.setST(0, math.Exp2(f.ST(0))-1.0)
			}
		case c.modrm == 0xF1: // FYL2X
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
		case c.modrm == 0xF2: // FPTAN
			if !f.checkStackUnderflow(0) {
				f.FSW &^= x87FSW_C2
				f.setST(0, math.Tan(f.ST(0)))
				f.push(1.0)
			}
		case c.modrm == 0xF3: // FPATAN
			if !f.checkStackUnderflow(0) && !f.checkStackUnderflow(1) {
				f.setST(1, math.Atan2(f.ST(1), f.ST(0)))
				f.pop()
			}
		case c.modrm == 0xF4: // FXTRACT
			if !f.checkStackUnderflow(0) {
				x := f.ST(0)
				if x == 0 {
					f.push(math.Inf(-1))
					f.setST(1, 0)
				} else {
					frac, exp := math.Frexp(x)
					sig := frac * 2
					f.setST(0, sig)
					f.push(float64(exp - 1))
				}
			}
		case c.modrm == 0xF5: // FPREM1
			if !f.checkStackUnderflow(0) && !f.checkStackUnderflow(1) {
				a := f.ST(0)
				b := f.ST(1)
				q := int64(math.RoundToEven(a / b))
				f.setST(0, math.Remainder(a, b))
				f.FSW &^= x87FSW_C2
				f.setQuotientFlags(q)
			}
		case c.modrm == 0xF6: // FDECSTP
			f.setTop((f.top() - 1) & 7)
		case c.modrm == 0xF7: // FINCSTP
			f.setTop((f.top() + 1) & 7)
		case c.modrm == 0xF8: // FPREM
			if !f.checkStackUnderflow(0) && !f.checkStackUnderflow(1) {
				a := f.ST(0)
				b := f.ST(1)
				q := int64(math.Trunc(a / b))
				f.setST(0, a-float64(q)*b)
				f.FSW &^= x87FSW_C2
				f.setQuotientFlags(q)
			}
		case c.modrm == 0xF9: // FYL2XP1
			if !f.checkStackUnderflow(0) && !f.checkStackUnderflow(1) {
				x := f.ST(0)
				y := f.ST(1)
				if x <= -1 {
					f.setException(x87FSW_IE)
				}
				f.setST(1, y*math.Log1p(x)/math.Ln2)
				f.pop()
			}
		case c.modrm == 0xFA: // FSQRT
			if !f.checkStackUnderflow(0) {
				x := f.ST(0)
				if x < 0 {
					f.setException(x87FSW_IE)
				}
				f.setST(0, math.Sqrt(x))
			}
		case c.modrm == 0xFB: // FSINCOS
			if !f.checkStackUnderflow(0) {
				x := f.ST(0)
				s := math.Sin(x)
				co := math.Cos(x)
				f.setST(0, s)
				f.push(co)
				f.FSW &^= x87FSW_C2
			}
		case c.modrm == 0xFC: // FRNDINT
			if !f.checkStackUnderflow(0) {
				f.setST(0, f.roundPerFCW(f.ST(0)))
			}
		case c.modrm == 0xFD: // FSCALE
			if !f.checkStackUnderflow(0) && !f.checkStackUnderflow(1) {
				scale := int(f.ST(1))
				f.setST(0, math.Ldexp(f.ST(0), scale))
			}
		case c.modrm == 0xFE: // FSIN
			if !f.checkStackUnderflow(0) {
				f.setST(0, math.Sin(f.ST(0)))
				f.FSW &^= x87FSW_C2
			}
		case c.modrm == 0xFF: // FCOS
			if !f.checkStackUnderflow(0) {
				f.setST(0, math.Cos(f.ST(0)))
				f.FSW &^= x87FSW_C2
			}
		}
	} else {
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
