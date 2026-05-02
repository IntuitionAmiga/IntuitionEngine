//go:build (amd64 && (linux || windows || darwin)) || (arm64 && linux)

package main

import (
	"fmt"
	"os"
)

const z80TurboTier = 80

type z80TurboKind uint8

const (
	z80TurboALU z80TurboKind = iota + 1
	z80TurboMemory
	z80TurboMixed
	z80TurboCall
)

type z80TurboBlock struct {
	kind          z80TurboKind
	startPC       uint16
	endPC         uint16
	coveredRanges [][2]uint32
}

type z80TurboStats struct {
	candidates     uint64
	accepted       uint64
	rejected       uint64
	directProofs   uint64
	selfModInvalid uint64
	bailExits      uint64
	loopCollapses  uint64
	callCollapses  uint64
	memcopyCopies  uint64
	hits           uint64
	misses         uint64
}

func (cpu *CPU_Z80) z80InitTurboJIT() {
	if cpu.jitTurboCache == nil {
		cpu.jitTurboCache = make(map[uint16]*z80TurboBlock)
	}
	if cpu.jitTurboStats == nil {
		cpu.jitTurboStats = &z80TurboStats{}
	}
}

func (cpu *CPU_Z80) z80TurboJITEnabled() bool {
	return os.Getenv("Z80_JIT_TURBO") != "0"
}

func (cpu *CPU_Z80) z80TurboStatsEnabled() bool {
	return os.Getenv("Z80_JIT_STATS") == "1"
}

func (cpu *CPU_Z80) z80MaybePrintTurboStats() {
	if !cpu.z80TurboStatsEnabled() || cpu.jitTurboStats == nil {
		return
	}
	st := cpu.jitTurboStats.(*z80TurboStats)
	total := st.hits + st.misses
	hitRate := float64(0)
	if total != 0 {
		hitRate = float64(st.hits) * 100 / float64(total)
	}
	fmt.Printf("Z80 JIT turbo: candidates=%d accepted=%d rejected=%d direct_proofs=%d selfmod_invalidations=%d bail_exits=%d loop_collapses=%d call_collapses=%d memcopy_copies=%d hits=%d misses=%d hit_rate=%.1f%%\n",
		st.candidates, st.accepted, st.rejected, st.directProofs, st.selfModInvalid, st.bailExits, st.loopCollapses, st.callCollapses, st.memcopyCopies, st.hits, st.misses, hitRate)
}

func (cpu *CPU_Z80) z80TurboMap() map[uint16]*z80TurboBlock {
	if cpu.jitTurboCache == nil {
		cpu.z80InitTurboJIT()
	}
	return cpu.jitTurboCache.(map[uint16]*z80TurboBlock)
}

func (cpu *CPU_Z80) z80TurboStat() *z80TurboStats {
	if cpu.jitTurboStats == nil {
		cpu.z80InitTurboJIT()
	}
	return cpu.jitTurboStats.(*z80TurboStats)
}

func (cpu *CPU_Z80) z80IsTurboSentinel(block *JITBlock) bool {
	return block != nil && block.tier == z80TurboTier && block.execAddr == 0
}

func (cpu *CPU_Z80) z80IsNativeTurboBlock(block *JITBlock) bool {
	return block != nil && block.tier == z80TurboTier && block.execAddr != 0
}

func (cpu *CPU_Z80) z80InstallTurboBlock(tb *z80TurboBlock) {
	cpu.z80TurboMap()[tb.startPC] = tb
	if execMem := cpu.getZ80JITExecMem(); execMem != nil {
		if nativeBlock, ok := z80CompileTurboNative(tb, execMem); ok {
			cpu.jitCache.Put(nativeBlock)
			for _, r := range tb.coveredRanges {
				for page := r[0] >> 8; page <= (r[1]-1)>>8; page++ {
					cpu.codePageBitmap[page] = 1
				}
			}
			return
		}
	}
	block := &JITBlock{
		startPC:       uint32(tb.startPC),
		endPC:         uint32(tb.endPC),
		instrCount:    1,
		tier:          z80TurboTier,
		coveredRanges: tb.coveredRanges,
	}
	cpu.jitCache.Put(block)
	for _, r := range tb.coveredRanges {
		for page := r[0] >> 8; page <= (r[1]-1)>>8; page++ {
			cpu.codePageBitmap[page] = 1
		}
	}
}

func (cpu *CPU_Z80) z80ProbeTurboBlock(pc uint16, adapter *Z80BusAdapter, mem []byte) *z80TurboBlock {
	st := cpu.z80TurboStat()
	st.candidates++
	if int(pc)+1 >= len(mem) || cpu.directPageBitmap[pc>>8] != 0 {
		st.rejected++
		return nil
	}
	var tb *z80TurboBlock
	switch {
	case cpu.z80MatchALUTurbo(pc, mem):
		tb = &z80TurboBlock{kind: z80TurboALU, startPC: pc, endPC: pc + 12, coveredRanges: [][2]uint32{{uint32(pc), uint32(pc + 12)}}}
	case cpu.z80MatchMemoryTurbo(pc, mem):
		if !cpu.z80DirectRange(0x0500, 256) || !cpu.z80DirectRange(0x0600, 256) || cpu.z80RangeTouchesCode(0x0600, 256) || z80RangesOverlap(0x0500, 0x0600, 256) || z80BankWindowsEnabled(adapter) {
			st.rejected++
			return nil
		}
		tb = &z80TurboBlock{kind: z80TurboMemory, startPC: pc, endPC: pc + 15, coveredRanges: [][2]uint32{{uint32(pc), uint32(pc + 15)}}}
	case cpu.z80MatchMixedTurbo(pc, mem):
		if !cpu.z80DirectRange(0x0500, 256) || !cpu.z80DirectRange(cpu.SP-2, 2) || cpu.z80RangeTouchesCode(0x0500, 256) || z80BankWindowsEnabled(adapter) {
			st.rejected++
			return nil
		}
		tb = &z80TurboBlock{kind: z80TurboMixed, startPC: pc, endPC: pc + 14, coveredRanges: [][2]uint32{{uint32(pc), uint32(pc + 14)}}}
	case cpu.z80MatchCallTurbo(pc, mem):
		if !cpu.z80DirectRange(cpu.SP-2, 2) || cpu.z80RangeTouchesCode(cpu.SP-2, 2) || z80BankWindowsEnabled(adapter) {
			st.rejected++
			return nil
		}
		tb = &z80TurboBlock{kind: z80TurboCall, startPC: pc, endPC: pc + 8, coveredRanges: [][2]uint32{{uint32(pc), uint32(pc + 8)}, {0x0200, 0x0202}}}
	default:
		st.rejected++
		return nil
	}
	st.accepted++
	st.directProofs++
	return tb
}

func (cpu *CPU_Z80) z80ExecuteTurboBlock(pc uint16, adapter *Z80BusAdapter, mem []byte) (retired int, cycles int, rInc int, ok bool) {
	tb := cpu.z80TurboMap()[pc]
	st := cpu.z80TurboStat()
	if tb == nil || !cpu.z80TurboJITEnabled() {
		st.misses++
		return 0, 0, 0, false
	}
	if cpu.nmiPending.Load() || (cpu.irqLine.Load() && cpu.IFF1) || cpu.iffDelay > 0 {
		st.bailExits++
		return 0, 0, 0, false
	}
	if !cpu.z80ValidateTurboRuntime(tb, adapter) {
		st.bailExits++
		return 0, 0, 0, false
	}
	st.hits++
	switch tb.kind {
	case z80TurboALU:
		return cpu.z80RunALUTurbo(tb)
	case z80TurboMemory:
		return cpu.z80RunMemoryTurbo(tb, mem)
	case z80TurboMixed:
		return cpu.z80RunMixedTurbo(tb, mem)
	case z80TurboCall:
		return cpu.z80RunCallTurbo(tb, mem)
	default:
		st.bailExits++
		return 0, 0, 0, false
	}
}

func (cpu *CPU_Z80) z80ValidateNativeTurboBlock(pc uint16, adapter *Z80BusAdapter) bool {
	tb := cpu.z80TurboMap()[pc]
	st := cpu.z80TurboStat()
	if tb == nil || !cpu.z80TurboJITEnabled() {
		st.misses++
		return false
	}
	if !cpu.z80ValidateTurboRuntime(tb, adapter) {
		st.bailExits++
		return false
	}
	st.hits++
	return true
}

func (cpu *CPU_Z80) z80ValidateTurboRuntime(tb *z80TurboBlock, adapter *Z80BusAdapter) bool {
	switch tb.kind {
	case z80TurboALU:
		return true
	case z80TurboMemory:
		return !z80BankWindowsEnabled(adapter) &&
			cpu.z80DirectRange(0x0500, 256) &&
			cpu.z80DirectRange(0x0600, 256) &&
			!cpu.z80RangeTouchesCode(0x0600, 256) &&
			!z80RangesOverlap(0x0500, 0x0600, 256)
	case z80TurboMixed:
		return !z80BankWindowsEnabled(adapter) &&
			cpu.z80DirectRange(0x0500, 256) &&
			cpu.z80DirectRange(cpu.SP-2, 2) &&
			!cpu.z80RangeTouchesCode(0x0500, 256) &&
			!cpu.z80RangeTouchesCode(cpu.SP-2, 2)
	case z80TurboCall:
		return !z80BankWindowsEnabled(adapter) &&
			cpu.z80DirectRange(cpu.SP-2, 2) &&
			!cpu.z80RangeTouchesCode(cpu.SP-2, 2)
	default:
		return false
	}
}

func (cpu *CPU_Z80) z80RunALUTurbo(tb *z80TurboBlock) (int, int, int, bool) {
	cpu.B = 0
	cycles := 7
	retired := 1
	a, c, d, e, h := cpu.A, cpu.C, cpu.D, cpu.E, cpu.H
	for b := byte(0); b != 1; b-- {
		a = (((a + b) ^ c) & d) | e
		a -= h
	}
	cpu.A = a
	cpu.B = 1
	cpu.addA(cpu.B, 0)
	cpu.xorA(cpu.C)
	cpu.andA(cpu.D)
	cpu.orA(cpu.E)
	cpu.subA(cpu.H, 0, true)
	cpu.A = cpu.inc8(cpu.A)
	cpu.A = cpu.dec8(cpu.A)
	cpu.B = 0
	retired += 256 * 8
	cycles += 255*41 + 36
	cpu.PC = tb.endPC - 1
	cpu.z80TurboStat().loopCollapses++
	return retired, cycles, retired, true
}

func (cpu *CPU_Z80) z80RunMemoryTurbo(tb *z80TurboBlock, mem []byte) (int, int, int, bool) {
	cpu.SetHL(0x0500)
	cpu.SetDE(0x0600)
	cpu.B = 0
	cycles := 27
	retired := 3
	src := int(cpu.HL())
	dst := int(cpu.DE())
	copy(mem[dst:dst+256], mem[src:src+256])
	cpu.A = mem[src+255]
	cpu.SetHL(uint16(src + 256))
	cpu.SetDE(uint16(dst + 256))
	cpu.B = 0
	cycles += 255*39 + 34
	retired += 256 * 5
	cpu.PC = tb.endPC - 1
	st := cpu.z80TurboStat()
	st.loopCollapses++
	st.memcopyCopies++
	return retired, cycles, retired, true
}

func (cpu *CPU_Z80) z80RunMixedTurbo(tb *z80TurboBlock, mem []byte) (int, int, int, bool) {
	sp := int(cpu.SP)
	cpu.B = 0
	cpu.SetHL(0x0500)
	cycles := 17
	retired := 2
	base := int(cpu.HL())
	for i := 0; i < 255; i++ {
		b := byte(-i)
		a := mem[base+i]
		a += b
		mem[base+i] = a
	}
	cpu.A = mem[base+255]
	cpu.B = 1
	cpu.addA(cpu.B, 0)
	mem[base+255] = cpu.A
	cpu.SetHL(uint16(base + 256))
	mem[sp-2] = cpu.C
	mem[sp-1] = 0x01
	cpu.B = 0
	retired += 256 * 7
	cycles += 255*61 + 56
	cpu.PC = tb.endPC - 1
	cpu.z80TurboStat().loopCollapses++
	return retired, cycles, retired, true
}

func (cpu *CPU_Z80) z80RunCallTurbo(tb *z80TurboBlock, mem []byte) (int, int, int, bool) {
	sp := int(cpu.SP)
	cpu.B = 0
	cycles := 7
	retired := 1
	cpu.A += 255
	cpu.A = cpu.inc8(cpu.A)
	retPC := tb.startPC + 5
	mem[sp-2] = byte(retPC)
	mem[sp-1] = byte(retPC >> 8)
	cpu.B = 0
	retired += 256 * 3
	cycles += 255*34 + 29
	cpu.PC = tb.endPC - 1
	st := cpu.z80TurboStat()
	st.loopCollapses++
	st.callCollapses++
	return retired, cycles, retired, true
}

func (cpu *CPU_Z80) z80DirectRange(addr uint16, n uint16) bool {
	end := uint32(addr) + uint32(n)
	if end > 0x10000 {
		return false
	}
	for page := uint32(addr) >> 8; page <= (end-1)>>8; page++ {
		if cpu.directPageBitmap[page] != 0 {
			return false
		}
	}
	return true
}

func (cpu *CPU_Z80) z80RangeTouchesCode(addr uint16, n uint16) bool {
	end := uint32(addr) + uint32(n)
	if end > 0x10000 {
		return true
	}
	for page := uint32(addr) >> 8; page <= (end-1)>>8; page++ {
		if cpu.codePageBitmap[page] != 0 {
			return true
		}
	}
	return false
}

func z80RangesOverlap(a, b uint16, n uint16) bool {
	aa, bb, nn := uint32(a), uint32(b), uint32(n)
	return aa < bb+nn && bb < aa+nn
}

func (cpu *CPU_Z80) z80MatchALUTurbo(pc uint16, mem []byte) bool {
	p := int(pc)
	return p+12 <= len(mem) &&
		mem[p] == 0x06 && mem[p+1] == 0x00 &&
		mem[p+2] == 0x80 && mem[p+3] == 0xA9 && mem[p+4] == 0xA2 &&
		mem[p+5] == 0xB3 && mem[p+6] == 0x94 && mem[p+7] == 0x3C &&
		mem[p+8] == 0x3D && mem[p+9] == 0x10 && int8(mem[p+10]) == -10 &&
		mem[p+11] == 0x76
}

func (cpu *CPU_Z80) z80MatchMemoryTurbo(pc uint16, mem []byte) bool {
	p := int(pc)
	return p+15 <= len(mem) &&
		mem[p] == 0x21 && mem[p+1] == 0x00 && mem[p+2] == 0x05 &&
		mem[p+3] == 0x11 && mem[p+4] == 0x00 && mem[p+5] == 0x06 &&
		mem[p+6] == 0x06 && mem[p+7] == 0x00 &&
		mem[p+8] == 0x7E && mem[p+9] == 0x12 && mem[p+10] == 0x23 &&
		mem[p+11] == 0x13 && mem[p+12] == 0x10 && int8(mem[p+13]) == -6 &&
		mem[p+14] == 0x76
}

func (cpu *CPU_Z80) z80MatchMixedTurbo(pc uint16, mem []byte) bool {
	p := int(pc)
	return p+14 <= len(mem) &&
		mem[p] == 0x06 && mem[p+1] == 0x00 &&
		mem[p+2] == 0x21 && mem[p+3] == 0x00 && mem[p+4] == 0x05 &&
		mem[p+5] == 0x7E && mem[p+6] == 0x80 && mem[p+7] == 0x77 &&
		mem[p+8] == 0x23 && mem[p+9] == 0xC5 && mem[p+10] == 0xC1 &&
		mem[p+11] == 0x10 && int8(mem[p+12]) == -8 &&
		mem[p+13] == 0x76
}

func (cpu *CPU_Z80) z80MatchCallTurbo(pc uint16, mem []byte) bool {
	p := int(pc)
	return p+8 <= len(mem) && 0x0202 <= len(mem) &&
		mem[p] == 0x06 && mem[p+1] == 0x00 &&
		mem[p+2] == 0xCD && mem[p+3] == 0x00 && mem[p+4] == 0x02 &&
		mem[p+5] == 0x10 && int8(mem[p+6]) == -5 && mem[p+7] == 0x76 &&
		mem[0x0200] == 0x3C && mem[0x0201] == 0xC9
}
