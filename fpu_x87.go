package main

import (
	"math"
)

var x87SmallestNormal = math.Float64frombits(0x0010000000000000)

const (
	x87TagValid   = uint16(0)
	x87TagZero    = uint16(1)
	x87TagSpecial = uint16(2)
	x87TagEmpty   = uint16(3)
)

const (
	x87FSW_IE       = uint16(1 << 0)
	x87FSW_DE       = uint16(1 << 1)
	x87FSW_ZE       = uint16(1 << 2)
	x87FSW_OE       = uint16(1 << 3)
	x87FSW_UE       = uint16(1 << 4)
	x87FSW_PE       = uint16(1 << 5)
	x87FSW_SF       = uint16(1 << 6)
	x87FSW_ES       = uint16(1 << 7)
	x87FSW_C0       = uint16(1 << 8)
	x87FSW_C1       = uint16(1 << 9)
	x87FSW_C2       = uint16(1 << 10)
	x87FSW_TOPMask  = uint16(7 << 11)
	x87FSW_TOPShift = 11
	x87FSW_C3       = uint16(1 << 14)
	x87FSW_B        = uint16(1 << 15)
)

const (
	x87FCW_PCShift = 8
	x87FCW_RCShift = 10
	x87FCW_RCMask  = uint16(3 << x87FCW_RCShift)
)

const (
	x87FCW_RCNearest = uint16(0)
	x87FCW_RCDown    = uint16(1)
	x87FCW_RCUp      = uint16(2)
	x87FCW_RCChop    = uint16(3)
)

const (
	x87IndefInt16 = int16(-32768)
	x87IndefInt32 = int32(-2147483648)
	x87IndefInt64 = int64(-9223372036854775808)
)

type FPU_X87 struct {
	regs [8]float64

	FCW uint16
	FSW uint16
	FTW uint16

	FIP uint32
	FCS uint16
	FDP uint32
	FDS uint16
	FOP uint16
}

type x87Bus interface {
	Read(addr uint32) byte
	Write(addr uint32, value byte)
}

func NewFPU_X87() *FPU_X87 {
	f := &FPU_X87{}
	f.Reset()
	return f
}

func (f *FPU_X87) Reset() {
	for i := range f.regs {
		f.regs[i] = 0
	}
	f.FCW = 0x037F
	f.FSW = 0
	f.FTW = 0xFFFF
	f.FIP = 0
	f.FCS = 0
	f.FDP = 0
	f.FDS = 0
	f.FOP = 0
}

func (f *FPU_X87) top() int {
	return int((f.FSW & x87FSW_TOPMask) >> x87FSW_TOPShift)
}

func (f *FPU_X87) setTop(top int) {
	f.FSW = (f.FSW &^ x87FSW_TOPMask) | (uint16(top&7) << x87FSW_TOPShift)
}

func (f *FPU_X87) physReg(stIdx int) int {
	return (f.top() + stIdx) & 7
}

func (f *FPU_X87) ST(i int) float64 {
	return f.regs[f.physReg(i)]
}

func (f *FPU_X87) setST(i int, v float64) {
	phys := f.physReg(i)
	f.regs[phys] = v
	f.setTag(phys, f.classifyTag(v))
}

func (f *FPU_X87) getTag(phys int) uint16 {
	shift := uint((phys & 7) * 2)
	return (f.FTW >> shift) & 0x3
}

func (f *FPU_X87) setTag(phys int, tag uint16) {
	shift := uint((phys & 7) * 2)
	f.FTW &^= 0x3 << shift
	f.FTW |= (tag & 0x3) << shift
}

func (f *FPU_X87) classifyTag(v float64) uint16 {
	if v == 0 {
		return x87TagZero
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return x87TagSpecial
	}
	if math.Abs(v) < x87SmallestNormal {
		return x87TagSpecial
	}
	return x87TagValid
}

func (f *FPU_X87) setException(mask uint16) {
	f.FSW |= mask
	if (f.FCW & mask) == 0 {
		f.FSW |= x87FSW_ES
	}
}

func (f *FPU_X87) clearCond() {
	f.FSW &^= x87FSW_C0 | x87FSW_C1 | x87FSW_C2 | x87FSW_C3
}

func (f *FPU_X87) checkStackOverflow() bool {
	nextTop := (f.top() - 1) & 7
	if f.getTag(nextTop) != x87TagEmpty {
		f.setException(x87FSW_IE | x87FSW_SF)
		f.FSW |= x87FSW_C1
		return true
	}
	return false
}

func (f *FPU_X87) checkStackUnderflow(i int) bool {
	if f.getTag(f.physReg(i)) == x87TagEmpty {
		f.setException(x87FSW_IE | x87FSW_SF)
		f.FSW &^= x87FSW_C1
		return true
	}
	return false
}

func (f *FPU_X87) push(v float64) {
	if f.checkStackOverflow() {
		return
	}
	nextTop := (f.top() - 1) & 7
	f.setTop(nextTop)
	f.regs[nextTop] = v
	f.setTag(nextTop, f.classifyTag(v))
}

func (f *FPU_X87) pop() float64 {
	if f.checkStackUnderflow(0) {
		return math.NaN()
	}
	top := f.top()
	v := f.regs[top]
	f.setTag(top, x87TagEmpty)
	f.setTop((top + 1) & 7)
	return v
}

func (f *FPU_X87) roundPerFCW(v float64) float64 {
	switch (f.FCW >> x87FCW_RCShift) & 0x3 {
	case x87FCW_RCDown:
		return math.Floor(v)
	case x87FCW_RCUp:
		return math.Ceil(v)
	case x87FCW_RCChop:
		return math.Trunc(v)
	default:
		return math.RoundToEven(v)
	}
}

func (f *FPU_X87) intFromFloat(v float64, bits int) int64 {
	r := f.roundPerFCW(v)
	if math.IsNaN(r) || math.IsInf(r, 0) {
		f.setException(x87FSW_IE)
		switch bits {
		case 16:
			return int64(x87IndefInt16)
		case 32:
			return int64(x87IndefInt32)
		default:
			return x87IndefInt64
		}
	}
	switch bits {
	case 16:
		if r < math.MinInt16 || r > math.MaxInt16 {
			f.setException(x87FSW_IE)
			return int64(x87IndefInt16)
		}
	case 32:
		if r < math.MinInt32 || r > math.MaxInt32 {
			f.setException(x87FSW_IE)
			return int64(x87IndefInt32)
		}
	case 64:
		if r < math.MinInt64 || r > math.MaxInt64 {
			f.setException(x87FSW_IE)
			return x87IndefInt64
		}
	}
	return int64(r)
}

func (f *FPU_X87) loadFloat32(bus x87Bus, addr uint32) float64 {
	bits := uint32(bus.Read(addr)) |
		(uint32(bus.Read(addr+1)) << 8) |
		(uint32(bus.Read(addr+2)) << 16) |
		(uint32(bus.Read(addr+3)) << 24)
	return float64(math.Float32frombits(bits))
}

func (f *FPU_X87) storeFloat32(bus x87Bus, addr uint32, v float64) {
	f32 := float32(v)
	if float64(f32) != v && !math.IsNaN(v) {
		f.setException(x87FSW_PE)
	}
	bits := math.Float32bits(f32)
	bus.Write(addr, byte(bits))
	bus.Write(addr+1, byte(bits>>8))
	bus.Write(addr+2, byte(bits>>16))
	bus.Write(addr+3, byte(bits>>24))
}

func (f *FPU_X87) loadFloat64(bus x87Bus, addr uint32) float64 {
	bits := uint64(bus.Read(addr)) |
		(uint64(bus.Read(addr+1)) << 8) |
		(uint64(bus.Read(addr+2)) << 16) |
		(uint64(bus.Read(addr+3)) << 24) |
		(uint64(bus.Read(addr+4)) << 32) |
		(uint64(bus.Read(addr+5)) << 40) |
		(uint64(bus.Read(addr+6)) << 48) |
		(uint64(bus.Read(addr+7)) << 56)
	return math.Float64frombits(bits)
}

func (f *FPU_X87) storeFloat64(bus x87Bus, addr uint32, v float64) {
	bits := math.Float64bits(v)
	for i := range 8 {
		bus.Write(addr+uint32(i), byte(bits>>(8*i)))
	}
}

func (f *FPU_X87) loadExtended80(bus x87Bus, addr uint32) float64 {
	var mant uint64
	for i := range 8 {
		mant |= uint64(bus.Read(addr+uint32(i))) << (8 * i)
	}
	se := uint16(bus.Read(addr+8)) | (uint16(bus.Read(addr+9)) << 8)
	sign := uint8((se >> 15) & 0x1)
	exp := se & 0x7FFF
	ext := ExtendedReal{Sign: sign, Exp: exp, Mant: mant}
	return ext.ToFloat64()
}

func (f *FPU_X87) storeExtended80(bus x87Bus, addr uint32, v float64) {
	ext := ExtendedRealFromFloat64(v)
	for i := range 8 {
		bus.Write(addr+uint32(i), byte(ext.Mant>>(8*i)))
	}
	se := (uint16(ext.Sign&1) << 15) | (ext.Exp & 0x7FFF)
	bus.Write(addr+8, byte(se))
	bus.Write(addr+9, byte(se>>8))
}

func (f *FPU_X87) loadInt16(bus x87Bus, addr uint32) float64 {
	raw := uint16(bus.Read(addr)) | (uint16(bus.Read(addr+1)) << 8)
	return float64(int16(raw))
}

func (f *FPU_X87) storeInt16(bus x87Bus, addr uint32, v float64) {
	i := int16(f.intFromFloat(v, 16))
	bus.Write(addr, byte(i))
	bus.Write(addr+1, byte(uint16(i)>>8))
}

func (f *FPU_X87) loadInt32(bus x87Bus, addr uint32) float64 {
	raw := uint32(bus.Read(addr)) |
		(uint32(bus.Read(addr+1)) << 8) |
		(uint32(bus.Read(addr+2)) << 16) |
		(uint32(bus.Read(addr+3)) << 24)
	return float64(int32(raw))
}

func (f *FPU_X87) storeInt32(bus x87Bus, addr uint32, v float64) {
	i := int32(f.intFromFloat(v, 32))
	bus.Write(addr, byte(i))
	bus.Write(addr+1, byte(uint32(i)>>8))
	bus.Write(addr+2, byte(uint32(i)>>16))
	bus.Write(addr+3, byte(uint32(i)>>24))
}

func (f *FPU_X87) loadInt64(bus x87Bus, addr uint32) float64 {
	raw := uint64(bus.Read(addr)) |
		(uint64(bus.Read(addr+1)) << 8) |
		(uint64(bus.Read(addr+2)) << 16) |
		(uint64(bus.Read(addr+3)) << 24) |
		(uint64(bus.Read(addr+4)) << 32) |
		(uint64(bus.Read(addr+5)) << 40) |
		(uint64(bus.Read(addr+6)) << 48) |
		(uint64(bus.Read(addr+7)) << 56)
	return float64(int64(raw))
}

func (f *FPU_X87) storeInt64(bus x87Bus, addr uint32, v float64) {
	i := uint64(f.intFromFloat(v, 64))
	for j := range 8 {
		bus.Write(addr+uint32(j), byte(i>>(8*j)))
	}
}

func (f *FPU_X87) loadBCD(bus x87Bus, addr uint32) float64 {
	var val int64
	mul := int64(1)
	for i := range 9 {
		b := bus.Read(addr + uint32(i))
		d0 := int64(b & 0x0F)
		d1 := int64((b >> 4) & 0x0F)
		val += d0 * mul
		mul *= 10
		val += d1 * mul
		mul *= 10
	}
	sign := bus.Read(addr + 9)
	if sign&0x80 != 0 {
		val = -val
	}
	return float64(val)
}

func (f *FPU_X87) storeBCD(bus x87Bus, addr uint32, v float64) {
	r := int64(f.roundPerFCW(v))
	neg := r < 0
	if neg {
		r = -r
	}
	for i := range 9 {
		d0 := byte(r % 10)
		r /= 10
		d1 := byte(r % 10)
		r /= 10
		bus.Write(addr+uint32(i), d0|(d1<<4))
	}
	if neg {
		bus.Write(addr+9, 0x80)
	} else {
		bus.Write(addr+9, 0x00)
	}
}

func (f *FPU_X87) doCompare(a, b float64, signalNaN bool) {
	f.clearCond()
	if math.IsNaN(a) || math.IsNaN(b) {
		f.FSW |= x87FSW_C0 | x87FSW_C2 | x87FSW_C3
		if signalNaN {
			f.setException(x87FSW_IE)
		}
		return
	}
	switch {
	case a > b:
		// all clear
	case a < b:
		f.FSW |= x87FSW_C0
	default:
		f.FSW |= x87FSW_C3
	}
}

func (f *FPU_X87) setQuotientFlags(q int64) {
	f.FSW &^= x87FSW_C0 | x87FSW_C1 | x87FSW_C3
	if (q & 0x4) != 0 {
		f.FSW |= x87FSW_C0
	}
	if (q & 0x2) != 0 {
		f.FSW |= x87FSW_C3
	}
	if (q & 0x1) != 0 {
		f.FSW |= x87FSW_C1
	}
}

func (f *FPU_X87) captureOp(cpu *CPU_X86, esc byte, modrm byte) {
	f.FIP = cpu.EIP - 2
	f.FCS = cpu.CS
	f.FOP = (uint16(esc&0x7) << 8) | uint16(modrm)
}

func (f *FPU_X87) captureMemAccess(cpu *CPU_X86, addr uint32) {
	f.FDP = addr
	f.FDS = cpu.getSeg(cpu.lastEASeg)
}

func (f *FPU_X87) fnstenv32(bus x87Bus, addr uint32) {
	store32 := func(off uint32, v uint32) {
		bus.Write(addr+off, byte(v))
		bus.Write(addr+off+1, byte(v>>8))
		bus.Write(addr+off+2, byte(v>>16))
		bus.Write(addr+off+3, byte(v>>24))
	}
	store32(0, uint32(f.FCW))
	store32(4, uint32(f.FSW))
	store32(8, uint32(f.FTW))
	store32(12, f.FIP)
	store32(16, uint32(f.FCS)|(uint32(f.FOP&0x7FF)<<16))
	store32(20, f.FDP)
	store32(24, uint32(f.FDS))
	f.FCW |= 0x003F
}

func (f *FPU_X87) fldenv32(bus x87Bus, addr uint32) {
	load32 := func(off uint32) uint32 {
		return uint32(bus.Read(addr+off)) |
			(uint32(bus.Read(addr+off+1)) << 8) |
			(uint32(bus.Read(addr+off+2)) << 16) |
			(uint32(bus.Read(addr+off+3)) << 24)
	}
	f.FCW = uint16(load32(0))
	f.FSW = uint16(load32(4))
	f.FTW = uint16(load32(8))
	f.FIP = load32(12)
	mix := load32(16)
	f.FCS = uint16(mix)
	f.FOP = uint16((mix >> 16) & 0x7FF)
	f.FDP = load32(20)
	f.FDS = uint16(load32(24))
}

func (f *FPU_X87) fsave32(bus x87Bus, addr uint32) {
	f.fnstenv32(bus, addr)
	base := addr + 28
	for i := range 8 {
		f.storeExtended80(bus, base+uint32(i*10), f.regs[i])
	}
	f.Reset()
}

func (f *FPU_X87) frstor32(bus x87Bus, addr uint32) {
	f.fldenv32(bus, addr)
	base := addr + 28
	for i := range 8 {
		f.regs[i] = f.loadExtended80(bus, base+uint32(i*10))
	}
}

func (f *FPU_X87) xam(v float64, empty bool) {
	f.FSW &^= x87FSW_C0 | x87FSW_C1 | x87FSW_C2 | x87FSW_C3
	if empty {
		f.FSW |= x87FSW_C0 | x87FSW_C3
		return
	}
	if math.Signbit(v) {
		f.FSW |= x87FSW_C1
	}
	if math.IsNaN(v) {
		f.FSW |= x87FSW_C0
		return
	}
	if math.IsInf(v, 0) {
		f.FSW |= x87FSW_C0 | x87FSW_C2
		return
	}
	if v == 0 {
		f.FSW |= x87FSW_C3
		return
	}
	if math.Abs(v) < x87SmallestNormal {
		f.FSW |= x87FSW_C2 | x87FSW_C3
		return
	}
	f.FSW |= x87FSW_C2
}

var x87ConstTable = [7]float64{
	1.0,
	math.Log2(10),
	math.Log2E,
	math.Pi,
	math.Log10(2),
	math.Ln2,
	0.0,
}
