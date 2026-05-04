package main

import (
	"math/rand"
)

const (
	XBIOS_PHYSBASE   = 2
	XBIOS_LOGBASE    = 3
	XBIOS_SETSCREEN  = 5
	XBIOS_SETPALETTE = 6
	XBIOS_SETCOLOR   = 7
	XBIOS_RANDOM     = 17
	XBIOS_DOSOUND    = 32
	XBIOS_KBRATE     = 35
)

type XBIOSInterceptor struct {
	cpu       *M68KCPU
	bus       *MachineBus
	videoChip *VideoChip
	psg       *PSGEngine

	logBase uint32
	rng     *rand.Rand
	kbrate  uint32

	stPalette [256]uint16
}

func NewXBIOSInterceptor(cpu *M68KCPU, bus *MachineBus, videoChip *VideoChip, psg *PSGEngine) *XBIOSInterceptor {
	return &XBIOSInterceptor{
		cpu:       cpu,
		bus:       bus,
		videoChip: videoChip,
		psg:       psg,
		logBase:   VRAM_START,
		rng:       rand.New(rand.NewSource(1)),
	}
}

func (x *XBIOSInterceptor) HandleTrap14() bool {
	sp := x.cpu.AddrRegs[7]
	funcNum := x.cpu.Read16(sp)

	switch funcNum {
	case XBIOS_PHYSBASE:
		x.setD0(VRAM_START)
	case XBIOS_LOGBASE:
		x.setD0(x.logBase)
	case XBIOS_SETSCREEN:
		x.handleSetscreen(sp)
	case XBIOS_SETPALETTE:
		x.handleSetpalette(sp)
	case XBIOS_SETCOLOR:
		x.handleSetcolor(sp)
	case XBIOS_DOSOUND:
		x.handleDosound(sp)
	case XBIOS_RANDOM:
		x.setD0(uint32(x.rng.Int31()))
	case XBIOS_KBRATE:
		x.handleKbrate(sp)
	default:
		return false
	}
	return true
}

func (x *XBIOSInterceptor) handleSetscreen(sp uint32) {
	log := x.cpu.Read32(sp + 2)
	if log != 0xFFFFFFFF && x.vramAddrValid(log) {
		x.logBase = log
	}
	x.setD0(x.logBase)
}

func (x *XBIOSInterceptor) handleSetpalette(sp uint32) {
	addr := x.cpu.Read32(sp + 2)
	for i := 0; i < 16; i++ {
		color := x.cpu.Read16(addr + uint32(i*2))
		x.stPalette[i] = color
		x.videoChip.SetPaletteEntry(uint8(i), stColorToRGB(color))
	}
	x.setD0(0)
}

func (x *XBIOSInterceptor) handleSetcolor(sp uint32) {
	index := x.cpu.Read16(sp + 2)
	color := x.cpu.Read16(sp + 4)
	if index >= 256 {
		x.setD0(0xFFFFFFFF)
		return
	}
	old := x.stPalette[index]
	x.stPalette[index] = color
	x.videoChip.SetPaletteEntry(uint8(index), stColorToRGB(color))
	x.setD0(uint32(old))
}

func (x *XBIOSInterceptor) handleDosound(sp uint32) {
	addr := x.cpu.Read32(sp + 2)
	for i := uint32(0); i < 512; i += 2 {
		reg := x.bus.Read8(addr + i)
		if reg == 0xFF {
			break
		}
		val := x.bus.Read8(addr + i + 1)
		if reg < PSG_REG_COUNT {
			if x.psg != nil {
				x.psg.WriteRegister(reg, val)
			} else {
				x.bus.Write8(PSG_BASE+uint32(reg), val)
			}
		}
	}
	x.setD0(0)
}

func (x *XBIOSInterceptor) handleKbrate(sp uint32) {
	initial := uint32(x.cpu.Read16(sp+2) & 0xFF)
	repeat := uint32(x.cpu.Read16(sp+4) & 0xFF)
	old := x.kbrate
	x.kbrate = initial<<8 | repeat
	x.setD0(old)
}

func (x *XBIOSInterceptor) setD0(val uint32) {
	x.cpu.DataRegs[0] = val
}

func (x *XBIOSInterceptor) vramAddrValid(addr uint32) bool {
	if addr < VRAM_START || addr >= VRAM_START+VRAM_SIZE {
		return false
	}
	return uint64(addr) < x.bus.ProfileMemoryCap() && addr < EmuTOS_PROFILE_TOP
}

func stColorToRGB(color uint16) uint32 {
	r := uint32((color >> 8) & 0x7)
	g := uint32((color >> 4) & 0x7)
	b := uint32(color & 0x7)
	r = (r << 5) | (r << 2) | (r >> 1)
	g = (g << 5) | (g << 2) | (g >> 1)
	b = (b << 5) | (b << 2) | (b >> 1)
	return r<<16 | g<<8 | b
}
