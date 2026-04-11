// cpu_six5go2_fast.go - 6502 fast interpreter path
//
// ExecuteFast() implements the fast interpreter loop from the 6502
// optimization plan. The loop shadows hot CPU state in local variables, uses
// the direct-page bitmap for translation-safe memory fast paths, and inlines
// the full validation-subset opcodes directly inside the dispatch switch.
// Opcodes outside the validation subset (unofficial/undocumented opcodes,
// anything the plan explicitly leaves behind) spill the shadow state, call
// the existing opcodeTable entry, then reload from the CPU struct.
//
// Correctness invariants (see the plan for details):
//   - Instruction-boundary polling of resetting, rdyLine, nmiPending,
//     irqPending and running is preserved exactly.
//   - Cycle counts — including page-cross penalties and branch-taken cycles —
//     match the legacy Execute() byte-for-byte (the legacy branch() helper
//     intentionally charges no base cycles; only +1 taken / +1 page-cross).
//   - Binary-mode ADC/SBC is inlined; decimal-mode ADC/SBC still defers to
//     the existing cpu.adc()/cpu.sbc() helpers for byte-for-byte compat with
//     the legacy BCD handling.
//   - Debug mode falls through to the legacy executeLegacy() path.
//   - Memory translation is never bypassed: non-direct pages always go
//     through adapter.Read()/adapter.Write(). The directPageBitmap is the
//     single source of truth for "may read memDirect directly" decisions.
//   - Every 6502 read-modify-write instruction issues the spurious write of
//     the original value before writing the modified value. That second bus
//     write is observable to MMIO devices mapped into the target page and is
//     required for parity with the legacy rmw() helper.
//   - Absolute operand fetches use per-byte bitmap checks so that an
//     instruction straddling a page boundary (pc == 0xXXFF) still honors
//     per-page translation rules for the high operand byte.
//
// Note on zero-page and stack accessors (plan deviation):
//
// The plan recommends "treat ReadZP() / WriteZP() / ReadStack() / WriteStack()
// as primary hot-path accessors for zero-page and stack traffic when those
// pages remain direct." We instead keep an inline `dpb[0/1] == 0` bitmap
// check against the loop-local `memDirect` slice header and `dpb` pointer
// captured at function entry. The resulting code is semantically equivalent
// to the adapter helpers — page 0 / page 1 never have adapter ioTable
// entries, so `directPageBitmap[0/1] == 0` iff `!ioPageBitmap[0/1]` — but is
// measurably faster in practice for two reasons:
//
//  1. ReadStack / WriteStack have body cost 82 / 84 which exceeds the Go
//     inliner's default budget of 80. They are therefore compiled as real
//     method calls. Every JSR / RTS / BRK / RTI / PHA / PLA / PHP / PLP pays
//     call overhead.
//  2. ReadZP / WriteZP do inline, but the expanded body still references
//     `adapter.ioPageBitmap` and `adapter.memDirect` — both are pointer-chased
//     loads through the `adapter` struct each call site. The inline
//     `dpb[0] == 0` / `memDirect[zp]` form uses the loop-local array pointer
//     and slice header which the Go SSA backend keeps in registers.
//
// Measured impact of a full refactor to the adapter helpers on this host
// (Intel i5-8365U, Go benchmark with 2s per case, 3 runs):
//
//     Workload  | helpers  | inline   | delta
//     ----------+----------+----------+---------
//     ALU       | 192 MIPS | 186 MIPS | +3%
//     Memory    | 177 MIPS | 202 MIPS | -12%
//     Call      | 160 MIPS | 191 MIPS | -16%
//     Branch    | 210 MIPS | 207 MIPS | +1%
//     Mixed     | 170 MIPS | 193 MIPS | -12%
//
// Memory / Call / Mixed regressed materially under the helpers because their
// hot paths are dominated by zero-page reads/writes and stack push/pop. The
// inline pattern recovers those losses without changing observable semantics.
// The plan's directPageBitmap-driven invariant is still satisfied — only the
// source-level spelling differs.

package main

import (
	"fmt"
	"runtime"
	"time"
)

// ===========================================================================
// Pure ALU/flag helpers — value in, value out, no escaping state.
// These are kept small and argument-only so the Go compiler's inliner can
// fold them into the dispatch switch.
// ===========================================================================

// adc6502Binary performs a binary-mode ADC. Decimal mode is handled by the
// existing cpu.adc() helper via fallback.
func adc6502Binary(a, sr, v byte) (newA, newSR byte) {
	var carry uint16
	if sr&CARRY_FLAG != 0 {
		carry = 1
	}
	temp := uint16(a) + uint16(v) + carry
	newA = byte(temp)
	sr &^= CARRY_FLAG | OVERFLOW_FLAG
	if temp > 0xFF {
		sr |= CARRY_FLAG
	}
	if (a^v)&0x80 == 0 && (a^newA)&0x80 != 0 {
		sr |= OVERFLOW_FLAG
	}
	newSR = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[newA]
	return
}

// sbc6502Binary performs a binary-mode SBC.
func sbc6502Binary(a, sr, v byte) (newA, newSR byte) {
	temp := uint16(a) - uint16(v)
	if sr&CARRY_FLAG == 0 {
		temp--
	}
	newA = byte(temp)
	sr &^= CARRY_FLAG | OVERFLOW_FLAG
	if temp < 0x100 {
		sr |= CARRY_FLAG
	}
	if (a^v)&0x80 != 0 && (a^newA)&0x80 != 0 {
		sr |= OVERFLOW_FLAG
	}
	newSR = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[newA]
	return
}

// cmp6502 updates C/N/Z for a CMP/CPX/CPY against the supplied register.
func cmp6502(reg, sr, v byte) byte {
	r := reg - v
	if reg >= v {
		sr |= CARRY_FLAG
	} else {
		sr &^= CARRY_FLAG
	}
	return (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[r]
}

// asl6502 performs an ASL on v and updates C/N/Z.
func asl6502(sr, v byte) (newSR, result byte) {
	if v&0x80 != 0 {
		sr |= CARRY_FLAG
	} else {
		sr &^= CARRY_FLAG
	}
	result = v << 1
	newSR = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[result]
	return
}

// lsr6502 performs an LSR on v and updates C/N/Z.
func lsr6502(sr, v byte) (newSR, result byte) {
	if v&0x01 != 0 {
		sr |= CARRY_FLAG
	} else {
		sr &^= CARRY_FLAG
	}
	result = v >> 1
	newSR = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[result]
	return
}

// rol6502 performs a ROL on v through carry and updates C/N/Z.
func rol6502(sr, v byte) (newSR, result byte) {
	var cin byte
	if sr&CARRY_FLAG != 0 {
		cin = 1
	}
	if v&0x80 != 0 {
		sr |= CARRY_FLAG
	} else {
		sr &^= CARRY_FLAG
	}
	result = (v << 1) | cin
	newSR = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[result]
	return
}

// ror6502 performs a ROR on v through carry and updates C/N/Z.
func ror6502(sr, v byte) (newSR, result byte) {
	var cin byte
	if sr&CARRY_FLAG != 0 {
		cin = 1
	}
	if v&0x01 != 0 {
		sr |= CARRY_FLAG
	} else {
		sr &^= CARRY_FLAG
	}
	result = (v >> 1) | (cin << 7)
	newSR = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[result]
	return
}

// inc6502 increments v and updates N/Z.
func inc6502(sr, v byte) (newSR, result byte) {
	result = v + 1
	newSR = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[result]
	return
}

// dec6502 decrements v and updates N/Z.
func dec6502(sr, v byte) (newSR, result byte) {
	result = v - 1
	newSR = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[result]
	return
}

// bit6502 performs the BIT test between a and v, updating N/V/Z.
func bit6502(a, sr, v byte) byte {
	sr &^= ZERO_FLAG | OVERFLOW_FLAG | NEGATIVE_FLAG
	if a&v == 0 {
		sr |= ZERO_FLAG
	}
	sr |= v & (OVERFLOW_FLAG | NEGATIVE_FLAG)
	return sr
}

// ===========================================================================
// Fast-path memory helpers on *CPU_6502.
// ===========================================================================

// fastReadByte reads a byte honoring the direct-page bitmap. Non-direct pages
// always go through adapter.Read() so MMIO translation is preserved.
func (cpu *CPU_6502) fastReadByte(addr uint16) byte {
	if cpu.directPageBitmap[addr>>8] == 0 {
		return cpu.fastAdapter.memDirect[addr]
	}
	return cpu.fastAdapter.Read(addr)
}

// fastWriteByte writes a byte honoring the direct-page bitmap.
func (cpu *CPU_6502) fastWriteByte(addr uint16, val byte) {
	if cpu.directPageBitmap[addr>>8] == 0 {
		cpu.fastAdapter.memDirect[addr] = val
		return
	}
	cpu.fastAdapter.Write(addr, val)
}

// ensureDirectPageBitmap seals MachineBus mappings (if the adapter is backed
// by one) and computes the direct-page bitmap on first use. Subsequent calls
// are no-ops. Safe to call repeatedly from stepFast() and ExecuteFast().
func (cpu_6502 *CPU_6502) ensureDirectPageBitmap() {
	if cpu_6502.directPageReady {
		return
	}
	if cpu_6502.fastAdapter == nil {
		return
	}
	if mb, ok := cpu_6502.fastAdapter.bus.(*MachineBus); ok {
		mb.SealMappings()
	}
	cpu_6502.initDirectPageBitmap()
}

// ===========================================================================
// ExecuteFast — the main dispatch loop.
//
// Every addressing-mode fetch consults dpb[pc>>8] per byte so that operands
// straddling a page boundary honor the correct translation rule for each
// byte. The fallback default case spills every local, invokes the generic
// opcodeTable entry, and reloads.
// ===========================================================================

// ExecuteFast is the fast interpreter entry point. Execute() routes here
// when fastAdapter != nil and Debug == false. Any other configuration falls
// back to the legacy executeLegacy() path.
func (cpu_6502 *CPU_6502) ExecuteFast() {
	if cpu_6502.fastAdapter == nil || cpu_6502.Debug {
		cpu_6502.executeLegacy()
		return
	}

	cpu_6502.ensureDirectPageBitmap()
	adapter := cpu_6502.fastAdapter
	memDirect := adapter.memDirect
	dpb := &cpu_6502.directPageBitmap

	if cpu_6502.PerfEnabled {
		cpu_6502.perfStartTime = time.Now()
		cpu_6502.lastPerfReport = cpu_6502.perfStartTime
		cpu_6502.InstructionCount = 0
	}

	cpu_6502.ensureOpcodeTableReady()
	cpu_6502.executing.Store(true)
	defer cpu_6502.executing.Store(false)

	// Local shadow state. Spilled to cpu fields only at helper, interrupt,
	// reset, loop-exit, and fallback boundaries.
	pc := cpu_6502.PC
	sp := cpu_6502.SP
	a := cpu_6502.A
	x := cpu_6502.X
	y := cpu_6502.Y
	sr := cpu_6502.SR
	cycles := cpu_6502.Cycles

	for cpu_6502.running.Load() {
		for range 4096 {
			// Reset handshake at instruction boundary.
			if cpu_6502.resetting.Load() {
				cpu_6502.PC = pc
				cpu_6502.SP = sp
				cpu_6502.A = a
				cpu_6502.X = x
				cpu_6502.Y = y
				cpu_6502.SR = sr
				cpu_6502.Cycles = cycles
				cpu_6502.resetAck.Store(true)
				for cpu_6502.resetting.Load() {
					runtime.Gosched()
				}
				cpu_6502.resetAck.Store(false)
				pc = cpu_6502.PC
				sp = cpu_6502.SP
				a = cpu_6502.A
				x = cpu_6502.X
				y = cpu_6502.Y
				sr = cpu_6502.SR
				cycles = cpu_6502.Cycles
				break
			}

			// RDY line hold
			if !cpu_6502.rdyLine.Load() {
				cpu_6502.rdyHold = true
				break
			}
			cpu_6502.rdyHold = false

			// Instruction-boundary interrupt delivery.
			if cpu_6502.nmiPending.Load() {
				cpu_6502.PC = pc
				cpu_6502.SP = sp
				cpu_6502.A = a
				cpu_6502.X = x
				cpu_6502.Y = y
				cpu_6502.SR = sr
				cpu_6502.Cycles = cycles
				cpu_6502.handleInterrupt(NMI_VECTOR, true)
				cpu_6502.nmiPending.Store(false)
				pc = cpu_6502.PC
				sp = cpu_6502.SP
				sr = cpu_6502.SR
				cycles = cpu_6502.Cycles
			} else if cpu_6502.irqPending.Load() && sr&INTERRUPT_FLAG == 0 {
				cpu_6502.PC = pc
				cpu_6502.SP = sp
				cpu_6502.A = a
				cpu_6502.X = x
				cpu_6502.Y = y
				cpu_6502.SR = sr
				cpu_6502.Cycles = cycles
				cpu_6502.handleInterrupt(IRQ_VECTOR, false)
				cpu_6502.irqPending.Store(false)
				pc = cpu_6502.PC
				sp = cpu_6502.SP
				sr = cpu_6502.SR
				cycles = cpu_6502.Cycles
			}

			// Fetch opcode via direct bitmap fast path.
			var opcode byte
			if dpb[pc>>8] == 0 {
				opcode = memDirect[pc]
			} else {
				opcode = adapter.Read(pc)
			}
			pc++

			switch opcode {

			// ================================================================
			// Loads
			// ================================================================

			case 0xA9: // LDA imm
				if dpb[pc>>8] == 0 {
					a = memDirect[pc]
				} else {
					a = adapter.Read(pc)
				}
				pc++
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 2

			case 0xA2: // LDX imm
				if dpb[pc>>8] == 0 {
					x = memDirect[pc]
				} else {
					x = adapter.Read(pc)
				}
				pc++
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[x]
				cycles += 2

			case 0xA0: // LDY imm
				if dpb[pc>>8] == 0 {
					y = memDirect[pc]
				} else {
					y = adapter.Read(pc)
				}
				pc++
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[y]
				cycles += 2

			case 0xA5: // LDA zp
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				if dpb[0] == 0 {
					a = memDirect[zp]
				} else {
					a = adapter.Read(uint16(zp))
				}
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 3

			case 0xA6: // LDX zp
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				if dpb[0] == 0 {
					x = memDirect[zp]
				} else {
					x = adapter.Read(uint16(zp))
				}
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[x]
				cycles += 3

			case 0xA4: // LDY zp
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				if dpb[0] == 0 {
					y = memDirect[zp]
				} else {
					y = adapter.Read(uint16(zp))
				}
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[y]
				cycles += 3

			case 0xB5: // LDA zp,X
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				addr := uint16(byte(zp + x))
				if dpb[0] == 0 {
					a = memDirect[addr]
				} else {
					a = adapter.Read(addr)
				}
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 4

			case 0xB6: // LDX zp,Y
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				addr := uint16(byte(zp + y))
				if dpb[0] == 0 {
					x = memDirect[addr]
				} else {
					x = adapter.Read(addr)
				}
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[x]
				cycles += 4

			case 0xB4: // LDY zp,X
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				addr := uint16(byte(zp + x))
				if dpb[0] == 0 {
					y = memDirect[addr]
				} else {
					y = adapter.Read(addr)
				}
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[y]
				cycles += 4

			case 0xAD: // LDA abs
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := uint16(lo) | uint16(hi)<<8
				if dpb[addr>>8] == 0 {
					a = memDirect[addr]
				} else {
					a = adapter.Read(addr)
				}
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 4

			case 0xAE: // LDX abs
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := uint16(lo) | uint16(hi)<<8
				if dpb[addr>>8] == 0 {
					x = memDirect[addr]
				} else {
					x = adapter.Read(addr)
				}
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[x]
				cycles += 4

			case 0xAC: // LDY abs
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := uint16(lo) | uint16(hi)<<8
				if dpb[addr>>8] == 0 {
					y = memDirect[addr]
				} else {
					y = adapter.Read(addr)
				}
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[y]
				cycles += 4

			case 0xBD: // LDA abs,X
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				base := uint16(lo) | uint16(hi)<<8
				addr := base + uint16(x)
				if dpb[addr>>8] == 0 {
					a = memDirect[addr]
				} else {
					a = adapter.Read(addr)
				}
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 4
				if (base & 0xFF00) != (addr & 0xFF00) {
					cycles++
				}

			case 0xB9: // LDA abs,Y
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				base := uint16(lo) | uint16(hi)<<8
				addr := base + uint16(y)
				if dpb[addr>>8] == 0 {
					a = memDirect[addr]
				} else {
					a = adapter.Read(addr)
				}
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 4
				if (base & 0xFF00) != (addr & 0xFF00) {
					cycles++
				}

			case 0xBE: // LDX abs,Y
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				base := uint16(lo) | uint16(hi)<<8
				addr := base + uint16(y)
				if dpb[addr>>8] == 0 {
					x = memDirect[addr]
				} else {
					x = adapter.Read(addr)
				}
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[x]
				cycles += 4
				if (base & 0xFF00) != (addr & 0xFF00) {
					cycles++
				}

			case 0xBC: // LDY abs,X
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				base := uint16(lo) | uint16(hi)<<8
				addr := base + uint16(x)
				if dpb[addr>>8] == 0 {
					y = memDirect[addr]
				} else {
					y = adapter.Read(addr)
				}
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[y]
				cycles += 4
				if (base & 0xFF00) != (addr & 0xFF00) {
					cycles++
				}

			case 0xA1: // LDA (ind,X)
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				ptr := byte(zp + x)
				var lo, hi byte
				if dpb[0] == 0 {
					lo = memDirect[ptr]
					hi = memDirect[byte(ptr+1)]
				} else {
					lo = adapter.Read(uint16(ptr))
					hi = adapter.Read(uint16(byte(ptr + 1)))
				}
				addr := uint16(lo) | uint16(hi)<<8
				if dpb[addr>>8] == 0 {
					a = memDirect[addr]
				} else {
					a = adapter.Read(addr)
				}
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 6

			case 0xB1: // LDA (ind),Y
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				var lo, hi byte
				if dpb[0] == 0 {
					lo = memDirect[zp]
					hi = memDirect[byte(zp+1)]
				} else {
					lo = adapter.Read(uint16(zp))
					hi = adapter.Read(uint16(byte(zp + 1)))
				}
				base := uint16(lo) | uint16(hi)<<8
				addr := base + uint16(y)
				if dpb[addr>>8] == 0 {
					a = memDirect[addr]
				} else {
					a = adapter.Read(addr)
				}
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 5
				if (base & 0xFF00) != (addr & 0xFF00) {
					cycles++
				}

			// ================================================================
			// Stores
			// ================================================================

			case 0x85: // STA zp
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				if dpb[0] == 0 {
					memDirect[zp] = a
				} else {
					adapter.Write(uint16(zp), a)
				}
				cycles += 3

			case 0x86: // STX zp
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				if dpb[0] == 0 {
					memDirect[zp] = x
				} else {
					adapter.Write(uint16(zp), x)
				}
				cycles += 3

			case 0x84: // STY zp
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				if dpb[0] == 0 {
					memDirect[zp] = y
				} else {
					adapter.Write(uint16(zp), y)
				}
				cycles += 3

			case 0x95: // STA zp,X
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				addr := uint16(byte(zp + x))
				if dpb[0] == 0 {
					memDirect[addr] = a
				} else {
					adapter.Write(addr, a)
				}
				cycles += 4

			case 0x96: // STX zp,Y
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				addr := uint16(byte(zp + y))
				if dpb[0] == 0 {
					memDirect[addr] = x
				} else {
					adapter.Write(addr, x)
				}
				cycles += 4

			case 0x94: // STY zp,X
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				addr := uint16(byte(zp + x))
				if dpb[0] == 0 {
					memDirect[addr] = y
				} else {
					adapter.Write(addr, y)
				}
				cycles += 4

			case 0x8D: // STA abs
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := uint16(lo) | uint16(hi)<<8
				if dpb[addr>>8] == 0 {
					memDirect[addr] = a
				} else {
					adapter.Write(addr, a)
				}
				cycles += 4

			case 0x8E: // STX abs
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := uint16(lo) | uint16(hi)<<8
				if dpb[addr>>8] == 0 {
					memDirect[addr] = x
				} else {
					adapter.Write(addr, x)
				}
				cycles += 4

			case 0x8C: // STY abs
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := uint16(lo) | uint16(hi)<<8
				if dpb[addr>>8] == 0 {
					memDirect[addr] = y
				} else {
					adapter.Write(addr, y)
				}
				cycles += 4

			case 0x9D: // STA abs,X (5 cycles — no page-cross penalty on stores)
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := (uint16(lo) | uint16(hi)<<8) + uint16(x)
				if dpb[addr>>8] == 0 {
					memDirect[addr] = a
				} else {
					adapter.Write(addr, a)
				}
				cycles += 5

			case 0x99: // STA abs,Y
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := (uint16(lo) | uint16(hi)<<8) + uint16(y)
				if dpb[addr>>8] == 0 {
					memDirect[addr] = a
				} else {
					adapter.Write(addr, a)
				}
				cycles += 5

			case 0x81: // STA (ind,X)
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				ptr := byte(zp + x)
				var lo, hi byte
				if dpb[0] == 0 {
					lo = memDirect[ptr]
					hi = memDirect[byte(ptr+1)]
				} else {
					lo = adapter.Read(uint16(ptr))
					hi = adapter.Read(uint16(byte(ptr + 1)))
				}
				addr := uint16(lo) | uint16(hi)<<8
				if dpb[addr>>8] == 0 {
					memDirect[addr] = a
				} else {
					adapter.Write(addr, a)
				}
				cycles += 6

			case 0x91: // STA (ind),Y
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				var lo, hi byte
				if dpb[0] == 0 {
					lo = memDirect[zp]
					hi = memDirect[byte(zp+1)]
				} else {
					lo = adapter.Read(uint16(zp))
					hi = adapter.Read(uint16(byte(zp + 1)))
				}
				addr := (uint16(lo) | uint16(hi)<<8) + uint16(y)
				if dpb[addr>>8] == 0 {
					memDirect[addr] = a
				} else {
					adapter.Write(addr, a)
				}
				cycles += 6

			// ================================================================
			// Register transfers
			// ================================================================

			case 0xAA: // TAX
				x = a
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[x]
				cycles += 2
			case 0x8A: // TXA
				a = x
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 2
			case 0xA8: // TAY
				y = a
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[y]
				cycles += 2
			case 0x98: // TYA
				a = y
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 2
			case 0xBA: // TSX
				x = sp
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[x]
				cycles += 2
			case 0x9A: // TXS
				sp = x
				cycles += 2

			// ================================================================
			// Register increment/decrement
			// ================================================================

			case 0xE8: // INX
				x++
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[x]
				cycles += 2
			case 0xC8: // INY
				y++
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[y]
				cycles += 2
			case 0xCA: // DEX
				x--
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[x]
				cycles += 2
			case 0x88: // DEY
				y--
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[y]
				cycles += 2

			// ================================================================
			// Logical ops — AND
			// ================================================================

			case 0x29: // AND imm
				var v byte
				if dpb[pc>>8] == 0 {
					v = memDirect[pc]
				} else {
					v = adapter.Read(pc)
				}
				pc++
				a &= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 2

			case 0x25: // AND zp
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				var v byte
				if dpb[0] == 0 {
					v = memDirect[zp]
				} else {
					v = adapter.Read(uint16(zp))
				}
				a &= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 3

			case 0x35: // AND zp,X
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				addr := uint16(byte(zp + x))
				var v byte
				if dpb[0] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				a &= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 4

			case 0x2D: // AND abs
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := uint16(lo) | uint16(hi)<<8
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				a &= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 4

			case 0x3D: // AND abs,X
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				base := uint16(lo) | uint16(hi)<<8
				addr := base + uint16(x)
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				a &= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 4
				if (base & 0xFF00) != (addr & 0xFF00) {
					cycles++
				}

			case 0x39: // AND abs,Y
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				base := uint16(lo) | uint16(hi)<<8
				addr := base + uint16(y)
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				a &= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 4
				if (base & 0xFF00) != (addr & 0xFF00) {
					cycles++
				}

			case 0x21: // AND (ind,X)
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				ptr := byte(zp + x)
				var lo, hi byte
				if dpb[0] == 0 {
					lo = memDirect[ptr]
					hi = memDirect[byte(ptr+1)]
				} else {
					lo = adapter.Read(uint16(ptr))
					hi = adapter.Read(uint16(byte(ptr + 1)))
				}
				addr := uint16(lo) | uint16(hi)<<8
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				a &= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 6

			case 0x31: // AND (ind),Y
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				var lo, hi byte
				if dpb[0] == 0 {
					lo = memDirect[zp]
					hi = memDirect[byte(zp+1)]
				} else {
					lo = adapter.Read(uint16(zp))
					hi = adapter.Read(uint16(byte(zp + 1)))
				}
				base := uint16(lo) | uint16(hi)<<8
				addr := base + uint16(y)
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				a &= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 5
				if (base & 0xFF00) != (addr & 0xFF00) {
					cycles++
				}

			// ================================================================
			// Logical ops — ORA
			// ================================================================

			case 0x09: // ORA imm
				var v byte
				if dpb[pc>>8] == 0 {
					v = memDirect[pc]
				} else {
					v = adapter.Read(pc)
				}
				pc++
				a |= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 2

			case 0x05: // ORA zp
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				var v byte
				if dpb[0] == 0 {
					v = memDirect[zp]
				} else {
					v = adapter.Read(uint16(zp))
				}
				a |= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 3

			case 0x15: // ORA zp,X
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				addr := uint16(byte(zp + x))
				var v byte
				if dpb[0] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				a |= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 4

			case 0x0D: // ORA abs
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := uint16(lo) | uint16(hi)<<8
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				a |= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 4

			case 0x1D: // ORA abs,X
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				base := uint16(lo) | uint16(hi)<<8
				addr := base + uint16(x)
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				a |= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 4
				if (base & 0xFF00) != (addr & 0xFF00) {
					cycles++
				}

			case 0x19: // ORA abs,Y
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				base := uint16(lo) | uint16(hi)<<8
				addr := base + uint16(y)
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				a |= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 4
				if (base & 0xFF00) != (addr & 0xFF00) {
					cycles++
				}

			case 0x01: // ORA (ind,X)
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				ptr := byte(zp + x)
				var lo, hi byte
				if dpb[0] == 0 {
					lo = memDirect[ptr]
					hi = memDirect[byte(ptr+1)]
				} else {
					lo = adapter.Read(uint16(ptr))
					hi = adapter.Read(uint16(byte(ptr + 1)))
				}
				addr := uint16(lo) | uint16(hi)<<8
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				a |= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 6

			case 0x11: // ORA (ind),Y
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				var lo, hi byte
				if dpb[0] == 0 {
					lo = memDirect[zp]
					hi = memDirect[byte(zp+1)]
				} else {
					lo = adapter.Read(uint16(zp))
					hi = adapter.Read(uint16(byte(zp + 1)))
				}
				base := uint16(lo) | uint16(hi)<<8
				addr := base + uint16(y)
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				a |= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 5
				if (base & 0xFF00) != (addr & 0xFF00) {
					cycles++
				}

			// ================================================================
			// Logical ops — EOR
			// ================================================================

			case 0x49: // EOR imm
				var v byte
				if dpb[pc>>8] == 0 {
					v = memDirect[pc]
				} else {
					v = adapter.Read(pc)
				}
				pc++
				a ^= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 2

			case 0x45: // EOR zp
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				var v byte
				if dpb[0] == 0 {
					v = memDirect[zp]
				} else {
					v = adapter.Read(uint16(zp))
				}
				a ^= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 3

			case 0x55: // EOR zp,X
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				addr := uint16(byte(zp + x))
				var v byte
				if dpb[0] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				a ^= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 4

			case 0x4D: // EOR abs
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := uint16(lo) | uint16(hi)<<8
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				a ^= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 4

			case 0x5D: // EOR abs,X
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				base := uint16(lo) | uint16(hi)<<8
				addr := base + uint16(x)
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				a ^= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 4
				if (base & 0xFF00) != (addr & 0xFF00) {
					cycles++
				}

			case 0x59: // EOR abs,Y
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				base := uint16(lo) | uint16(hi)<<8
				addr := base + uint16(y)
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				a ^= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 4
				if (base & 0xFF00) != (addr & 0xFF00) {
					cycles++
				}

			case 0x41: // EOR (ind,X)
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				ptr := byte(zp + x)
				var lo, hi byte
				if dpb[0] == 0 {
					lo = memDirect[ptr]
					hi = memDirect[byte(ptr+1)]
				} else {
					lo = adapter.Read(uint16(ptr))
					hi = adapter.Read(uint16(byte(ptr + 1)))
				}
				addr := uint16(lo) | uint16(hi)<<8
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				a ^= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 6

			case 0x51: // EOR (ind),Y
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				var lo, hi byte
				if dpb[0] == 0 {
					lo = memDirect[zp]
					hi = memDirect[byte(zp+1)]
				} else {
					lo = adapter.Read(uint16(zp))
					hi = adapter.Read(uint16(byte(zp + 1)))
				}
				base := uint16(lo) | uint16(hi)<<8
				addr := base + uint16(y)
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				a ^= v
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 5
				if (base & 0xFF00) != (addr & 0xFF00) {
					cycles++
				}

			// ================================================================
			// ADC (binary mode inlined; decimal defers to cpu.adc via fallback)
			// ================================================================

			case 0x69: // ADC imm
				if sr&DECIMAL_FLAG != 0 {
					cpu_6502.PC = pc
					cpu_6502.SP = sp
					cpu_6502.A = a
					cpu_6502.X = x
					cpu_6502.Y = y
					cpu_6502.SR = sr
					cpu_6502.Cycles = cycles
					cpu_6502.opcodeTable[opcode](cpu_6502)
					pc = cpu_6502.PC
					sp = cpu_6502.SP
					a = cpu_6502.A
					x = cpu_6502.X
					y = cpu_6502.Y
					sr = cpu_6502.SR
					cycles = cpu_6502.Cycles
				} else {
					var v byte
					if dpb[pc>>8] == 0 {
						v = memDirect[pc]
					} else {
						v = adapter.Read(pc)
					}
					pc++
					a, sr = adc6502Binary(a, sr, v)
					cycles += 2
				}

			case 0x65: // ADC zp
				if sr&DECIMAL_FLAG != 0 {
					cpu_6502.PC = pc
					cpu_6502.SP = sp
					cpu_6502.A = a
					cpu_6502.X = x
					cpu_6502.Y = y
					cpu_6502.SR = sr
					cpu_6502.Cycles = cycles
					cpu_6502.opcodeTable[opcode](cpu_6502)
					pc = cpu_6502.PC
					sp = cpu_6502.SP
					a = cpu_6502.A
					x = cpu_6502.X
					y = cpu_6502.Y
					sr = cpu_6502.SR
					cycles = cpu_6502.Cycles
				} else {
					var zp byte
					if dpb[pc>>8] == 0 {
						zp = memDirect[pc]
					} else {
						zp = adapter.Read(pc)
					}
					pc++
					var v byte
					if dpb[0] == 0 {
						v = memDirect[zp]
					} else {
						v = adapter.Read(uint16(zp))
					}
					a, sr = adc6502Binary(a, sr, v)
					cycles += 3
				}

			case 0x75: // ADC zp,X
				if sr&DECIMAL_FLAG != 0 {
					cpu_6502.PC = pc
					cpu_6502.SP = sp
					cpu_6502.A = a
					cpu_6502.X = x
					cpu_6502.Y = y
					cpu_6502.SR = sr
					cpu_6502.Cycles = cycles
					cpu_6502.opcodeTable[opcode](cpu_6502)
					pc = cpu_6502.PC
					sp = cpu_6502.SP
					a = cpu_6502.A
					x = cpu_6502.X
					y = cpu_6502.Y
					sr = cpu_6502.SR
					cycles = cpu_6502.Cycles
				} else {
					var zp byte
					if dpb[pc>>8] == 0 {
						zp = memDirect[pc]
					} else {
						zp = adapter.Read(pc)
					}
					pc++
					addr := uint16(byte(zp + x))
					var v byte
					if dpb[0] == 0 {
						v = memDirect[addr]
					} else {
						v = adapter.Read(addr)
					}
					a, sr = adc6502Binary(a, sr, v)
					cycles += 4
				}

			case 0x6D: // ADC abs
				if sr&DECIMAL_FLAG != 0 {
					cpu_6502.PC = pc
					cpu_6502.SP = sp
					cpu_6502.A = a
					cpu_6502.X = x
					cpu_6502.Y = y
					cpu_6502.SR = sr
					cpu_6502.Cycles = cycles
					cpu_6502.opcodeTable[opcode](cpu_6502)
					pc = cpu_6502.PC
					sp = cpu_6502.SP
					a = cpu_6502.A
					x = cpu_6502.X
					y = cpu_6502.Y
					sr = cpu_6502.SR
					cycles = cpu_6502.Cycles
				} else {
					var lo byte
					if dpb[pc>>8] == 0 {
						lo = memDirect[pc]
					} else {
						lo = adapter.Read(pc)
					}
					pc++
					var hi byte
					if dpb[pc>>8] == 0 {
						hi = memDirect[pc]
					} else {
						hi = adapter.Read(pc)
					}
					pc++
					addr := uint16(lo) | uint16(hi)<<8
					var v byte
					if dpb[addr>>8] == 0 {
						v = memDirect[addr]
					} else {
						v = adapter.Read(addr)
					}
					a, sr = adc6502Binary(a, sr, v)
					cycles += 4
				}

			case 0x7D: // ADC abs,X
				if sr&DECIMAL_FLAG != 0 {
					cpu_6502.PC = pc
					cpu_6502.SP = sp
					cpu_6502.A = a
					cpu_6502.X = x
					cpu_6502.Y = y
					cpu_6502.SR = sr
					cpu_6502.Cycles = cycles
					cpu_6502.opcodeTable[opcode](cpu_6502)
					pc = cpu_6502.PC
					sp = cpu_6502.SP
					a = cpu_6502.A
					x = cpu_6502.X
					y = cpu_6502.Y
					sr = cpu_6502.SR
					cycles = cpu_6502.Cycles
				} else {
					var lo byte
					if dpb[pc>>8] == 0 {
						lo = memDirect[pc]
					} else {
						lo = adapter.Read(pc)
					}
					pc++
					var hi byte
					if dpb[pc>>8] == 0 {
						hi = memDirect[pc]
					} else {
						hi = adapter.Read(pc)
					}
					pc++
					base := uint16(lo) | uint16(hi)<<8
					addr := base + uint16(x)
					var v byte
					if dpb[addr>>8] == 0 {
						v = memDirect[addr]
					} else {
						v = adapter.Read(addr)
					}
					a, sr = adc6502Binary(a, sr, v)
					cycles += 4
					if (base & 0xFF00) != (addr & 0xFF00) {
						cycles++
					}
				}

			case 0x79: // ADC abs,Y
				if sr&DECIMAL_FLAG != 0 {
					cpu_6502.PC = pc
					cpu_6502.SP = sp
					cpu_6502.A = a
					cpu_6502.X = x
					cpu_6502.Y = y
					cpu_6502.SR = sr
					cpu_6502.Cycles = cycles
					cpu_6502.opcodeTable[opcode](cpu_6502)
					pc = cpu_6502.PC
					sp = cpu_6502.SP
					a = cpu_6502.A
					x = cpu_6502.X
					y = cpu_6502.Y
					sr = cpu_6502.SR
					cycles = cpu_6502.Cycles
				} else {
					var lo byte
					if dpb[pc>>8] == 0 {
						lo = memDirect[pc]
					} else {
						lo = adapter.Read(pc)
					}
					pc++
					var hi byte
					if dpb[pc>>8] == 0 {
						hi = memDirect[pc]
					} else {
						hi = adapter.Read(pc)
					}
					pc++
					base := uint16(lo) | uint16(hi)<<8
					addr := base + uint16(y)
					var v byte
					if dpb[addr>>8] == 0 {
						v = memDirect[addr]
					} else {
						v = adapter.Read(addr)
					}
					a, sr = adc6502Binary(a, sr, v)
					cycles += 4
					if (base & 0xFF00) != (addr & 0xFF00) {
						cycles++
					}
				}

			case 0x61: // ADC (ind,X)
				if sr&DECIMAL_FLAG != 0 {
					cpu_6502.PC = pc
					cpu_6502.SP = sp
					cpu_6502.A = a
					cpu_6502.X = x
					cpu_6502.Y = y
					cpu_6502.SR = sr
					cpu_6502.Cycles = cycles
					cpu_6502.opcodeTable[opcode](cpu_6502)
					pc = cpu_6502.PC
					sp = cpu_6502.SP
					a = cpu_6502.A
					x = cpu_6502.X
					y = cpu_6502.Y
					sr = cpu_6502.SR
					cycles = cpu_6502.Cycles
				} else {
					var zp byte
					if dpb[pc>>8] == 0 {
						zp = memDirect[pc]
					} else {
						zp = adapter.Read(pc)
					}
					pc++
					ptr := byte(zp + x)
					var lo, hi byte
					if dpb[0] == 0 {
						lo = memDirect[ptr]
						hi = memDirect[byte(ptr+1)]
					} else {
						lo = adapter.Read(uint16(ptr))
						hi = adapter.Read(uint16(byte(ptr + 1)))
					}
					addr := uint16(lo) | uint16(hi)<<8
					var v byte
					if dpb[addr>>8] == 0 {
						v = memDirect[addr]
					} else {
						v = adapter.Read(addr)
					}
					a, sr = adc6502Binary(a, sr, v)
					cycles += 6
				}

			case 0x71: // ADC (ind),Y
				if sr&DECIMAL_FLAG != 0 {
					cpu_6502.PC = pc
					cpu_6502.SP = sp
					cpu_6502.A = a
					cpu_6502.X = x
					cpu_6502.Y = y
					cpu_6502.SR = sr
					cpu_6502.Cycles = cycles
					cpu_6502.opcodeTable[opcode](cpu_6502)
					pc = cpu_6502.PC
					sp = cpu_6502.SP
					a = cpu_6502.A
					x = cpu_6502.X
					y = cpu_6502.Y
					sr = cpu_6502.SR
					cycles = cpu_6502.Cycles
				} else {
					var zp byte
					if dpb[pc>>8] == 0 {
						zp = memDirect[pc]
					} else {
						zp = adapter.Read(pc)
					}
					pc++
					var lo, hi byte
					if dpb[0] == 0 {
						lo = memDirect[zp]
						hi = memDirect[byte(zp+1)]
					} else {
						lo = adapter.Read(uint16(zp))
						hi = adapter.Read(uint16(byte(zp + 1)))
					}
					base := uint16(lo) | uint16(hi)<<8
					addr := base + uint16(y)
					var v byte
					if dpb[addr>>8] == 0 {
						v = memDirect[addr]
					} else {
						v = adapter.Read(addr)
					}
					a, sr = adc6502Binary(a, sr, v)
					cycles += 5
					if (base & 0xFF00) != (addr & 0xFF00) {
						cycles++
					}
				}

			// ================================================================
			// SBC (binary mode inlined; decimal defers to cpu.sbc via fallback)
			// ================================================================

			case 0xE9, 0xEB: // SBC imm (0xEB is unofficial alias)
				if sr&DECIMAL_FLAG != 0 {
					cpu_6502.PC = pc
					cpu_6502.SP = sp
					cpu_6502.A = a
					cpu_6502.X = x
					cpu_6502.Y = y
					cpu_6502.SR = sr
					cpu_6502.Cycles = cycles
					cpu_6502.opcodeTable[opcode](cpu_6502)
					pc = cpu_6502.PC
					sp = cpu_6502.SP
					a = cpu_6502.A
					x = cpu_6502.X
					y = cpu_6502.Y
					sr = cpu_6502.SR
					cycles = cpu_6502.Cycles
				} else {
					var v byte
					if dpb[pc>>8] == 0 {
						v = memDirect[pc]
					} else {
						v = adapter.Read(pc)
					}
					pc++
					a, sr = sbc6502Binary(a, sr, v)
					cycles += 2
				}

			case 0xE5: // SBC zp
				if sr&DECIMAL_FLAG != 0 {
					cpu_6502.PC = pc
					cpu_6502.SP = sp
					cpu_6502.A = a
					cpu_6502.X = x
					cpu_6502.Y = y
					cpu_6502.SR = sr
					cpu_6502.Cycles = cycles
					cpu_6502.opcodeTable[opcode](cpu_6502)
					pc = cpu_6502.PC
					sp = cpu_6502.SP
					a = cpu_6502.A
					x = cpu_6502.X
					y = cpu_6502.Y
					sr = cpu_6502.SR
					cycles = cpu_6502.Cycles
				} else {
					var zp byte
					if dpb[pc>>8] == 0 {
						zp = memDirect[pc]
					} else {
						zp = adapter.Read(pc)
					}
					pc++
					var v byte
					if dpb[0] == 0 {
						v = memDirect[zp]
					} else {
						v = adapter.Read(uint16(zp))
					}
					a, sr = sbc6502Binary(a, sr, v)
					cycles += 3
				}

			case 0xF5: // SBC zp,X
				if sr&DECIMAL_FLAG != 0 {
					cpu_6502.PC = pc
					cpu_6502.SP = sp
					cpu_6502.A = a
					cpu_6502.X = x
					cpu_6502.Y = y
					cpu_6502.SR = sr
					cpu_6502.Cycles = cycles
					cpu_6502.opcodeTable[opcode](cpu_6502)
					pc = cpu_6502.PC
					sp = cpu_6502.SP
					a = cpu_6502.A
					x = cpu_6502.X
					y = cpu_6502.Y
					sr = cpu_6502.SR
					cycles = cpu_6502.Cycles
				} else {
					var zp byte
					if dpb[pc>>8] == 0 {
						zp = memDirect[pc]
					} else {
						zp = adapter.Read(pc)
					}
					pc++
					addr := uint16(byte(zp + x))
					var v byte
					if dpb[0] == 0 {
						v = memDirect[addr]
					} else {
						v = adapter.Read(addr)
					}
					a, sr = sbc6502Binary(a, sr, v)
					cycles += 4
				}

			case 0xED: // SBC abs
				if sr&DECIMAL_FLAG != 0 {
					cpu_6502.PC = pc
					cpu_6502.SP = sp
					cpu_6502.A = a
					cpu_6502.X = x
					cpu_6502.Y = y
					cpu_6502.SR = sr
					cpu_6502.Cycles = cycles
					cpu_6502.opcodeTable[opcode](cpu_6502)
					pc = cpu_6502.PC
					sp = cpu_6502.SP
					a = cpu_6502.A
					x = cpu_6502.X
					y = cpu_6502.Y
					sr = cpu_6502.SR
					cycles = cpu_6502.Cycles
				} else {
					var lo byte
					if dpb[pc>>8] == 0 {
						lo = memDirect[pc]
					} else {
						lo = adapter.Read(pc)
					}
					pc++
					var hi byte
					if dpb[pc>>8] == 0 {
						hi = memDirect[pc]
					} else {
						hi = adapter.Read(pc)
					}
					pc++
					addr := uint16(lo) | uint16(hi)<<8
					var v byte
					if dpb[addr>>8] == 0 {
						v = memDirect[addr]
					} else {
						v = adapter.Read(addr)
					}
					a, sr = sbc6502Binary(a, sr, v)
					cycles += 4
				}

			case 0xFD: // SBC abs,X
				if sr&DECIMAL_FLAG != 0 {
					cpu_6502.PC = pc
					cpu_6502.SP = sp
					cpu_6502.A = a
					cpu_6502.X = x
					cpu_6502.Y = y
					cpu_6502.SR = sr
					cpu_6502.Cycles = cycles
					cpu_6502.opcodeTable[opcode](cpu_6502)
					pc = cpu_6502.PC
					sp = cpu_6502.SP
					a = cpu_6502.A
					x = cpu_6502.X
					y = cpu_6502.Y
					sr = cpu_6502.SR
					cycles = cpu_6502.Cycles
				} else {
					var lo byte
					if dpb[pc>>8] == 0 {
						lo = memDirect[pc]
					} else {
						lo = adapter.Read(pc)
					}
					pc++
					var hi byte
					if dpb[pc>>8] == 0 {
						hi = memDirect[pc]
					} else {
						hi = adapter.Read(pc)
					}
					pc++
					base := uint16(lo) | uint16(hi)<<8
					addr := base + uint16(x)
					var v byte
					if dpb[addr>>8] == 0 {
						v = memDirect[addr]
					} else {
						v = adapter.Read(addr)
					}
					a, sr = sbc6502Binary(a, sr, v)
					cycles += 4
					if (base & 0xFF00) != (addr & 0xFF00) {
						cycles++
					}
				}

			case 0xF9: // SBC abs,Y
				if sr&DECIMAL_FLAG != 0 {
					cpu_6502.PC = pc
					cpu_6502.SP = sp
					cpu_6502.A = a
					cpu_6502.X = x
					cpu_6502.Y = y
					cpu_6502.SR = sr
					cpu_6502.Cycles = cycles
					cpu_6502.opcodeTable[opcode](cpu_6502)
					pc = cpu_6502.PC
					sp = cpu_6502.SP
					a = cpu_6502.A
					x = cpu_6502.X
					y = cpu_6502.Y
					sr = cpu_6502.SR
					cycles = cpu_6502.Cycles
				} else {
					var lo byte
					if dpb[pc>>8] == 0 {
						lo = memDirect[pc]
					} else {
						lo = adapter.Read(pc)
					}
					pc++
					var hi byte
					if dpb[pc>>8] == 0 {
						hi = memDirect[pc]
					} else {
						hi = adapter.Read(pc)
					}
					pc++
					base := uint16(lo) | uint16(hi)<<8
					addr := base + uint16(y)
					var v byte
					if dpb[addr>>8] == 0 {
						v = memDirect[addr]
					} else {
						v = adapter.Read(addr)
					}
					a, sr = sbc6502Binary(a, sr, v)
					cycles += 4
					if (base & 0xFF00) != (addr & 0xFF00) {
						cycles++
					}
				}

			case 0xE1: // SBC (ind,X)
				if sr&DECIMAL_FLAG != 0 {
					cpu_6502.PC = pc
					cpu_6502.SP = sp
					cpu_6502.A = a
					cpu_6502.X = x
					cpu_6502.Y = y
					cpu_6502.SR = sr
					cpu_6502.Cycles = cycles
					cpu_6502.opcodeTable[opcode](cpu_6502)
					pc = cpu_6502.PC
					sp = cpu_6502.SP
					a = cpu_6502.A
					x = cpu_6502.X
					y = cpu_6502.Y
					sr = cpu_6502.SR
					cycles = cpu_6502.Cycles
				} else {
					var zp byte
					if dpb[pc>>8] == 0 {
						zp = memDirect[pc]
					} else {
						zp = adapter.Read(pc)
					}
					pc++
					ptr := byte(zp + x)
					var lo, hi byte
					if dpb[0] == 0 {
						lo = memDirect[ptr]
						hi = memDirect[byte(ptr+1)]
					} else {
						lo = adapter.Read(uint16(ptr))
						hi = adapter.Read(uint16(byte(ptr + 1)))
					}
					addr := uint16(lo) | uint16(hi)<<8
					var v byte
					if dpb[addr>>8] == 0 {
						v = memDirect[addr]
					} else {
						v = adapter.Read(addr)
					}
					a, sr = sbc6502Binary(a, sr, v)
					cycles += 6
				}

			case 0xF1: // SBC (ind),Y
				if sr&DECIMAL_FLAG != 0 {
					cpu_6502.PC = pc
					cpu_6502.SP = sp
					cpu_6502.A = a
					cpu_6502.X = x
					cpu_6502.Y = y
					cpu_6502.SR = sr
					cpu_6502.Cycles = cycles
					cpu_6502.opcodeTable[opcode](cpu_6502)
					pc = cpu_6502.PC
					sp = cpu_6502.SP
					a = cpu_6502.A
					x = cpu_6502.X
					y = cpu_6502.Y
					sr = cpu_6502.SR
					cycles = cpu_6502.Cycles
				} else {
					var zp byte
					if dpb[pc>>8] == 0 {
						zp = memDirect[pc]
					} else {
						zp = adapter.Read(pc)
					}
					pc++
					var lo, hi byte
					if dpb[0] == 0 {
						lo = memDirect[zp]
						hi = memDirect[byte(zp+1)]
					} else {
						lo = adapter.Read(uint16(zp))
						hi = adapter.Read(uint16(byte(zp + 1)))
					}
					base := uint16(lo) | uint16(hi)<<8
					addr := base + uint16(y)
					var v byte
					if dpb[addr>>8] == 0 {
						v = memDirect[addr]
					} else {
						v = adapter.Read(addr)
					}
					a, sr = sbc6502Binary(a, sr, v)
					cycles += 5
					if (base & 0xFF00) != (addr & 0xFF00) {
						cycles++
					}
				}

			// ================================================================
			// Compare — CMP / CPX / CPY
			// ================================================================

			case 0xC9: // CMP imm
				var v byte
				if dpb[pc>>8] == 0 {
					v = memDirect[pc]
				} else {
					v = adapter.Read(pc)
				}
				pc++
				sr = cmp6502(a, sr, v)
				cycles += 2

			case 0xC5: // CMP zp
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				var v byte
				if dpb[0] == 0 {
					v = memDirect[zp]
				} else {
					v = adapter.Read(uint16(zp))
				}
				sr = cmp6502(a, sr, v)
				cycles += 3

			case 0xD5: // CMP zp,X
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				addr := uint16(byte(zp + x))
				var v byte
				if dpb[0] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				sr = cmp6502(a, sr, v)
				cycles += 4

			case 0xCD: // CMP abs
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := uint16(lo) | uint16(hi)<<8
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				sr = cmp6502(a, sr, v)
				cycles += 4

			case 0xDD: // CMP abs,X
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				base := uint16(lo) | uint16(hi)<<8
				addr := base + uint16(x)
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				sr = cmp6502(a, sr, v)
				cycles += 4
				if (base & 0xFF00) != (addr & 0xFF00) {
					cycles++
				}

			case 0xD9: // CMP abs,Y
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				base := uint16(lo) | uint16(hi)<<8
				addr := base + uint16(y)
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				sr = cmp6502(a, sr, v)
				cycles += 4
				if (base & 0xFF00) != (addr & 0xFF00) {
					cycles++
				}

			case 0xC1: // CMP (ind,X)
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				ptr := byte(zp + x)
				var lo, hi byte
				if dpb[0] == 0 {
					lo = memDirect[ptr]
					hi = memDirect[byte(ptr+1)]
				} else {
					lo = adapter.Read(uint16(ptr))
					hi = adapter.Read(uint16(byte(ptr + 1)))
				}
				addr := uint16(lo) | uint16(hi)<<8
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				sr = cmp6502(a, sr, v)
				cycles += 6

			case 0xD1: // CMP (ind),Y
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				var lo, hi byte
				if dpb[0] == 0 {
					lo = memDirect[zp]
					hi = memDirect[byte(zp+1)]
				} else {
					lo = adapter.Read(uint16(zp))
					hi = adapter.Read(uint16(byte(zp + 1)))
				}
				base := uint16(lo) | uint16(hi)<<8
				addr := base + uint16(y)
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				sr = cmp6502(a, sr, v)
				cycles += 5
				if (base & 0xFF00) != (addr & 0xFF00) {
					cycles++
				}

			case 0xE0: // CPX imm
				var v byte
				if dpb[pc>>8] == 0 {
					v = memDirect[pc]
				} else {
					v = adapter.Read(pc)
				}
				pc++
				sr = cmp6502(x, sr, v)
				cycles += 2

			case 0xE4: // CPX zp
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				var v byte
				if dpb[0] == 0 {
					v = memDirect[zp]
				} else {
					v = adapter.Read(uint16(zp))
				}
				sr = cmp6502(x, sr, v)
				cycles += 3

			case 0xEC: // CPX abs
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := uint16(lo) | uint16(hi)<<8
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				sr = cmp6502(x, sr, v)
				cycles += 4

			case 0xC0: // CPY imm
				var v byte
				if dpb[pc>>8] == 0 {
					v = memDirect[pc]
				} else {
					v = adapter.Read(pc)
				}
				pc++
				sr = cmp6502(y, sr, v)
				cycles += 2

			case 0xC4: // CPY zp
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				var v byte
				if dpb[0] == 0 {
					v = memDirect[zp]
				} else {
					v = adapter.Read(uint16(zp))
				}
				sr = cmp6502(y, sr, v)
				cycles += 3

			case 0xCC: // CPY abs
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := uint16(lo) | uint16(hi)<<8
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				sr = cmp6502(y, sr, v)
				cycles += 4

			// ================================================================
			// BIT
			// ================================================================

			case 0x24: // BIT zp
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				var v byte
				if dpb[0] == 0 {
					v = memDirect[zp]
				} else {
					v = adapter.Read(uint16(zp))
				}
				sr = bit6502(a, sr, v)
				cycles += 3

			case 0x2C: // BIT abs
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := uint16(lo) | uint16(hi)<<8
				var v byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
				} else {
					v = adapter.Read(addr)
				}
				sr = bit6502(a, sr, v)
				cycles += 4

			// ================================================================
			// Shifts/rotates — accumulator
			// ================================================================

			case 0x0A: // ASL A
				sr, a = asl6502(sr, a)
				cycles += 2
			case 0x4A: // LSR A
				sr, a = lsr6502(sr, a)
				cycles += 2
			case 0x2A: // ROL A
				sr, a = rol6502(sr, a)
				cycles += 2
			case 0x6A: // ROR A
				sr, a = ror6502(sr, a)
				cycles += 2

			// ================================================================
			// Shifts/rotates — memory (all use spurious-write RMW discipline)
			// ================================================================

			case 0x06: // ASL zp
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				var v, r byte
				if dpb[0] == 0 {
					v = memDirect[zp]
					memDirect[zp] = v
					sr, r = asl6502(sr, v)
					memDirect[zp] = r
				} else {
					v = adapter.Read(uint16(zp))
					adapter.Write(uint16(zp), v)
					sr, r = asl6502(sr, v)
					adapter.Write(uint16(zp), r)
				}
				cycles += 5

			case 0x16: // ASL zp,X
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				addr := uint16(byte(zp + x))
				var v, r byte
				if dpb[0] == 0 {
					v = memDirect[addr]
					memDirect[addr] = v
					sr, r = asl6502(sr, v)
					memDirect[addr] = r
				} else {
					v = adapter.Read(addr)
					adapter.Write(addr, v)
					sr, r = asl6502(sr, v)
					adapter.Write(addr, r)
				}
				cycles += 6

			case 0x0E: // ASL abs
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := uint16(lo) | uint16(hi)<<8
				var v, r byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
					memDirect[addr] = v
					sr, r = asl6502(sr, v)
					memDirect[addr] = r
				} else {
					v = adapter.Read(addr)
					adapter.Write(addr, v)
					sr, r = asl6502(sr, v)
					adapter.Write(addr, r)
				}
				cycles += 6

			case 0x1E: // ASL abs,X
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := (uint16(lo) | uint16(hi)<<8) + uint16(x)
				var v, r byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
					memDirect[addr] = v
					sr, r = asl6502(sr, v)
					memDirect[addr] = r
				} else {
					v = adapter.Read(addr)
					adapter.Write(addr, v)
					sr, r = asl6502(sr, v)
					adapter.Write(addr, r)
				}
				cycles += 7

			case 0x46: // LSR zp
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				var v, r byte
				if dpb[0] == 0 {
					v = memDirect[zp]
					memDirect[zp] = v
					sr, r = lsr6502(sr, v)
					memDirect[zp] = r
				} else {
					v = adapter.Read(uint16(zp))
					adapter.Write(uint16(zp), v)
					sr, r = lsr6502(sr, v)
					adapter.Write(uint16(zp), r)
				}
				cycles += 5

			case 0x56: // LSR zp,X
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				addr := uint16(byte(zp + x))
				var v, r byte
				if dpb[0] == 0 {
					v = memDirect[addr]
					memDirect[addr] = v
					sr, r = lsr6502(sr, v)
					memDirect[addr] = r
				} else {
					v = adapter.Read(addr)
					adapter.Write(addr, v)
					sr, r = lsr6502(sr, v)
					adapter.Write(addr, r)
				}
				cycles += 6

			case 0x4E: // LSR abs
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := uint16(lo) | uint16(hi)<<8
				var v, r byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
					memDirect[addr] = v
					sr, r = lsr6502(sr, v)
					memDirect[addr] = r
				} else {
					v = adapter.Read(addr)
					adapter.Write(addr, v)
					sr, r = lsr6502(sr, v)
					adapter.Write(addr, r)
				}
				cycles += 6

			case 0x5E: // LSR abs,X
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := (uint16(lo) | uint16(hi)<<8) + uint16(x)
				var v, r byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
					memDirect[addr] = v
					sr, r = lsr6502(sr, v)
					memDirect[addr] = r
				} else {
					v = adapter.Read(addr)
					adapter.Write(addr, v)
					sr, r = lsr6502(sr, v)
					adapter.Write(addr, r)
				}
				cycles += 7

			case 0x26: // ROL zp
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				var v, r byte
				if dpb[0] == 0 {
					v = memDirect[zp]
					memDirect[zp] = v
					sr, r = rol6502(sr, v)
					memDirect[zp] = r
				} else {
					v = adapter.Read(uint16(zp))
					adapter.Write(uint16(zp), v)
					sr, r = rol6502(sr, v)
					adapter.Write(uint16(zp), r)
				}
				cycles += 5

			case 0x36: // ROL zp,X
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				addr := uint16(byte(zp + x))
				var v, r byte
				if dpb[0] == 0 {
					v = memDirect[addr]
					memDirect[addr] = v
					sr, r = rol6502(sr, v)
					memDirect[addr] = r
				} else {
					v = adapter.Read(addr)
					adapter.Write(addr, v)
					sr, r = rol6502(sr, v)
					adapter.Write(addr, r)
				}
				cycles += 6

			case 0x2E: // ROL abs
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := uint16(lo) | uint16(hi)<<8
				var v, r byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
					memDirect[addr] = v
					sr, r = rol6502(sr, v)
					memDirect[addr] = r
				} else {
					v = adapter.Read(addr)
					adapter.Write(addr, v)
					sr, r = rol6502(sr, v)
					adapter.Write(addr, r)
				}
				cycles += 6

			case 0x3E: // ROL abs,X
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := (uint16(lo) | uint16(hi)<<8) + uint16(x)
				var v, r byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
					memDirect[addr] = v
					sr, r = rol6502(sr, v)
					memDirect[addr] = r
				} else {
					v = adapter.Read(addr)
					adapter.Write(addr, v)
					sr, r = rol6502(sr, v)
					adapter.Write(addr, r)
				}
				cycles += 7

			case 0x66: // ROR zp
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				var v, r byte
				if dpb[0] == 0 {
					v = memDirect[zp]
					memDirect[zp] = v
					sr, r = ror6502(sr, v)
					memDirect[zp] = r
				} else {
					v = adapter.Read(uint16(zp))
					adapter.Write(uint16(zp), v)
					sr, r = ror6502(sr, v)
					adapter.Write(uint16(zp), r)
				}
				cycles += 5

			case 0x76: // ROR zp,X
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				addr := uint16(byte(zp + x))
				var v, r byte
				if dpb[0] == 0 {
					v = memDirect[addr]
					memDirect[addr] = v
					sr, r = ror6502(sr, v)
					memDirect[addr] = r
				} else {
					v = adapter.Read(addr)
					adapter.Write(addr, v)
					sr, r = ror6502(sr, v)
					adapter.Write(addr, r)
				}
				cycles += 6

			case 0x6E: // ROR abs
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := uint16(lo) | uint16(hi)<<8
				var v, r byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
					memDirect[addr] = v
					sr, r = ror6502(sr, v)
					memDirect[addr] = r
				} else {
					v = adapter.Read(addr)
					adapter.Write(addr, v)
					sr, r = ror6502(sr, v)
					adapter.Write(addr, r)
				}
				cycles += 6

			case 0x7E: // ROR abs,X
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := (uint16(lo) | uint16(hi)<<8) + uint16(x)
				var v, r byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
					memDirect[addr] = v
					sr, r = ror6502(sr, v)
					memDirect[addr] = r
				} else {
					v = adapter.Read(addr)
					adapter.Write(addr, v)
					sr, r = ror6502(sr, v)
					adapter.Write(addr, r)
				}
				cycles += 7

			// ================================================================
			// Memory INC/DEC — spurious-write RMW discipline
			// ================================================================

			case 0xE6: // INC zp
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				var v, r byte
				if dpb[0] == 0 {
					v = memDirect[zp]
					memDirect[zp] = v
					sr, r = inc6502(sr, v)
					memDirect[zp] = r
				} else {
					v = adapter.Read(uint16(zp))
					adapter.Write(uint16(zp), v)
					sr, r = inc6502(sr, v)
					adapter.Write(uint16(zp), r)
				}
				cycles += 5

			case 0xF6: // INC zp,X
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				addr := uint16(byte(zp + x))
				var v, r byte
				if dpb[0] == 0 {
					v = memDirect[addr]
					memDirect[addr] = v
					sr, r = inc6502(sr, v)
					memDirect[addr] = r
				} else {
					v = adapter.Read(addr)
					adapter.Write(addr, v)
					sr, r = inc6502(sr, v)
					adapter.Write(addr, r)
				}
				cycles += 6

			case 0xEE: // INC abs
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := uint16(lo) | uint16(hi)<<8
				var v, r byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
					memDirect[addr] = v
					sr, r = inc6502(sr, v)
					memDirect[addr] = r
				} else {
					v = adapter.Read(addr)
					adapter.Write(addr, v)
					sr, r = inc6502(sr, v)
					adapter.Write(addr, r)
				}
				cycles += 6

			case 0xFE: // INC abs,X
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := (uint16(lo) | uint16(hi)<<8) + uint16(x)
				var v, r byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
					memDirect[addr] = v
					sr, r = inc6502(sr, v)
					memDirect[addr] = r
				} else {
					v = adapter.Read(addr)
					adapter.Write(addr, v)
					sr, r = inc6502(sr, v)
					adapter.Write(addr, r)
				}
				cycles += 7

			case 0xC6: // DEC zp
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				var v, r byte
				if dpb[0] == 0 {
					v = memDirect[zp]
					memDirect[zp] = v
					sr, r = dec6502(sr, v)
					memDirect[zp] = r
				} else {
					v = adapter.Read(uint16(zp))
					adapter.Write(uint16(zp), v)
					sr, r = dec6502(sr, v)
					adapter.Write(uint16(zp), r)
				}
				cycles += 5

			case 0xD6: // DEC zp,X
				var zp byte
				if dpb[pc>>8] == 0 {
					zp = memDirect[pc]
				} else {
					zp = adapter.Read(pc)
				}
				pc++
				addr := uint16(byte(zp + x))
				var v, r byte
				if dpb[0] == 0 {
					v = memDirect[addr]
					memDirect[addr] = v
					sr, r = dec6502(sr, v)
					memDirect[addr] = r
				} else {
					v = adapter.Read(addr)
					adapter.Write(addr, v)
					sr, r = dec6502(sr, v)
					adapter.Write(addr, r)
				}
				cycles += 6

			case 0xCE: // DEC abs
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := uint16(lo) | uint16(hi)<<8
				var v, r byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
					memDirect[addr] = v
					sr, r = dec6502(sr, v)
					memDirect[addr] = r
				} else {
					v = adapter.Read(addr)
					adapter.Write(addr, v)
					sr, r = dec6502(sr, v)
					adapter.Write(addr, r)
				}
				cycles += 6

			case 0xDE: // DEC abs,X
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				addr := (uint16(lo) | uint16(hi)<<8) + uint16(x)
				var v, r byte
				if dpb[addr>>8] == 0 {
					v = memDirect[addr]
					memDirect[addr] = v
					sr, r = dec6502(sr, v)
					memDirect[addr] = r
				} else {
					v = adapter.Read(addr)
					adapter.Write(addr, v)
					sr, r = dec6502(sr, v)
					adapter.Write(addr, r)
				}
				cycles += 7

			// ================================================================
			// Branches (no base cycles; legacy branch() only charges
			// +1 taken and +1 page-cross)
			// ================================================================

			case 0x90: // BCC
				var off byte
				if dpb[pc>>8] == 0 {
					off = memDirect[pc]
				} else {
					off = adapter.Read(pc)
				}
				pc++
				if sr&CARRY_FLAG == 0 {
					oldPC := pc
					pc = uint16(int32(pc) + int32(int8(off)))
					cycles++
					if (oldPC & 0xFF00) != (pc & 0xFF00) {
						cycles++
					}
				}

			case 0xB0: // BCS
				var off byte
				if dpb[pc>>8] == 0 {
					off = memDirect[pc]
				} else {
					off = adapter.Read(pc)
				}
				pc++
				if sr&CARRY_FLAG != 0 {
					oldPC := pc
					pc = uint16(int32(pc) + int32(int8(off)))
					cycles++
					if (oldPC & 0xFF00) != (pc & 0xFF00) {
						cycles++
					}
				}

			case 0xF0: // BEQ
				var off byte
				if dpb[pc>>8] == 0 {
					off = memDirect[pc]
				} else {
					off = adapter.Read(pc)
				}
				pc++
				if sr&ZERO_FLAG != 0 {
					oldPC := pc
					pc = uint16(int32(pc) + int32(int8(off)))
					cycles++
					if (oldPC & 0xFF00) != (pc & 0xFF00) {
						cycles++
					}
				}

			case 0xD0: // BNE
				var off byte
				if dpb[pc>>8] == 0 {
					off = memDirect[pc]
				} else {
					off = adapter.Read(pc)
				}
				pc++
				if sr&ZERO_FLAG == 0 {
					oldPC := pc
					pc = uint16(int32(pc) + int32(int8(off)))
					cycles++
					if (oldPC & 0xFF00) != (pc & 0xFF00) {
						cycles++
					}
				}

			case 0x30: // BMI
				var off byte
				if dpb[pc>>8] == 0 {
					off = memDirect[pc]
				} else {
					off = adapter.Read(pc)
				}
				pc++
				if sr&NEGATIVE_FLAG != 0 {
					oldPC := pc
					pc = uint16(int32(pc) + int32(int8(off)))
					cycles++
					if (oldPC & 0xFF00) != (pc & 0xFF00) {
						cycles++
					}
				}

			case 0x10: // BPL
				var off byte
				if dpb[pc>>8] == 0 {
					off = memDirect[pc]
				} else {
					off = adapter.Read(pc)
				}
				pc++
				if sr&NEGATIVE_FLAG == 0 {
					oldPC := pc
					pc = uint16(int32(pc) + int32(int8(off)))
					cycles++
					if (oldPC & 0xFF00) != (pc & 0xFF00) {
						cycles++
					}
				}

			case 0x70: // BVS
				var off byte
				if dpb[pc>>8] == 0 {
					off = memDirect[pc]
				} else {
					off = adapter.Read(pc)
				}
				pc++
				if sr&OVERFLOW_FLAG != 0 {
					oldPC := pc
					pc = uint16(int32(pc) + int32(int8(off)))
					cycles++
					if (oldPC & 0xFF00) != (pc & 0xFF00) {
						cycles++
					}
				}

			case 0x50: // BVC
				var off byte
				if dpb[pc>>8] == 0 {
					off = memDirect[pc]
				} else {
					off = adapter.Read(pc)
				}
				pc++
				if sr&OVERFLOW_FLAG == 0 {
					oldPC := pc
					pc = uint16(int32(pc) + int32(int8(off)))
					cycles++
					if (oldPC & 0xFF00) != (pc & 0xFF00) {
						cycles++
					}
				}

			// ================================================================
			// Flag operations
			// ================================================================

			case 0x18: // CLC
				sr &^= CARRY_FLAG
				cycles += 2
			case 0x38: // SEC
				sr |= CARRY_FLAG
				cycles += 2
			case 0x58: // CLI
				sr &^= INTERRUPT_FLAG
				cycles += 2
			case 0x78: // SEI
				sr |= INTERRUPT_FLAG
				cycles += 2
			case 0xB8: // CLV
				sr &^= OVERFLOW_FLAG
				cycles += 2
			case 0xD8: // CLD
				sr &^= DECIMAL_FLAG
				cycles += 2
			case 0xF8: // SED
				sr |= DECIMAL_FLAG
				cycles += 2

			// ================================================================
			// Stack operations
			// ================================================================

			case 0x48: // PHA
				if dpb[1] == 0 {
					memDirect[0x0100|uint16(sp)] = a
				} else {
					adapter.Write(0x0100|uint16(sp), a)
				}
				sp--
				cycles += 3

			case 0x68: // PLA
				sp++
				if dpb[1] == 0 {
					a = memDirect[0x0100|uint16(sp)]
				} else {
					a = adapter.Read(0x0100 | uint16(sp))
				}
				sr = (sr &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[a]
				cycles += 4

			case 0x08: // PHP
				val := sr | BREAK_FLAG | UNUSED_FLAG
				if dpb[1] == 0 {
					memDirect[0x0100|uint16(sp)] = val
				} else {
					adapter.Write(0x0100|uint16(sp), val)
				}
				sp--
				cycles += 3

			case 0x28: // PLP
				sp++
				var v byte
				if dpb[1] == 0 {
					v = memDirect[0x0100|uint16(sp)]
				} else {
					v = adapter.Read(0x0100 | uint16(sp))
				}
				sr = (v & 0xEF) | UNUSED_FLAG
				cycles += 4

			// ================================================================
			// Control flow
			// ================================================================

			case 0x4C: // JMP abs
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc = uint16(lo) | uint16(hi)<<8
				cycles += 3

			case 0x6C: // JMP (ind) — 6502 page-wrap bug intact
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				ptr := uint16(lo) | uint16(hi)<<8
				var lo2, hi2 byte
				if dpb[ptr>>8] == 0 {
					lo2 = memDirect[ptr]
				} else {
					lo2 = adapter.Read(ptr)
				}
				wrapPtr := (ptr & 0xFF00) | ((ptr + 1) & 0x00FF)
				if dpb[wrapPtr>>8] == 0 {
					hi2 = memDirect[wrapPtr]
				} else {
					hi2 = adapter.Read(wrapPtr)
				}
				pc = uint16(lo2) | uint16(hi2)<<8
				cycles += 5

			case 0x20: // JSR abs
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				// legacy JSR pushes PC (which after fetching the two operand
				// bytes is already one past the hi byte) minus one via
				// push16(cpu.PC - 1). Our pc here points one past the hi
				// operand byte, so retPC = pc (without further increment).
				retPC := pc
				if dpb[1] == 0 {
					memDirect[0x0100|uint16(sp)] = byte(retPC >> 8)
				} else {
					adapter.Write(0x0100|uint16(sp), byte(retPC>>8))
				}
				sp--
				if dpb[1] == 0 {
					memDirect[0x0100|uint16(sp)] = byte(retPC)
				} else {
					adapter.Write(0x0100|uint16(sp), byte(retPC))
				}
				sp--
				pc = uint16(lo) | uint16(hi)<<8
				cycles += 6

			case 0x60: // RTS
				sp++
				var lo byte
				if dpb[1] == 0 {
					lo = memDirect[0x0100|uint16(sp)]
				} else {
					lo = adapter.Read(0x0100 | uint16(sp))
				}
				sp++
				var hi byte
				if dpb[1] == 0 {
					hi = memDirect[0x0100|uint16(sp)]
				} else {
					hi = adapter.Read(0x0100 | uint16(sp))
				}
				pc = (uint16(lo) | uint16(hi)<<8) + 1
				cycles += 6

			case 0x00: // BRK — matches legacy executeLegacy() byte for byte
				pc++ // legacy cpu_6502.PC++
				if dpb[1] == 0 {
					memDirect[0x0100|uint16(sp)] = byte(pc >> 8)
				} else {
					adapter.Write(0x0100|uint16(sp), byte(pc>>8))
				}
				sp--
				if dpb[1] == 0 {
					memDirect[0x0100|uint16(sp)] = byte(pc)
				} else {
					adapter.Write(0x0100|uint16(sp), byte(pc))
				}
				sp--
				if dpb[1] == 0 {
					memDirect[0x0100|uint16(sp)] = sr | BREAK_FLAG | UNUSED_FLAG
				} else {
					adapter.Write(0x0100|uint16(sp), sr|BREAK_FLAG|UNUSED_FLAG)
				}
				sp--
				sr |= INTERRUPT_FLAG
				sr &^= BREAK_FLAG
				pc = uint16(adapter.Read(IRQ_VECTOR)) | uint16(adapter.Read(IRQ_VECTOR+1))<<8
				cycles += 7

			case 0x40: // RTI
				sp++
				var v byte
				if dpb[1] == 0 {
					v = memDirect[0x0100|uint16(sp)]
				} else {
					v = adapter.Read(0x0100 | uint16(sp))
				}
				sr = (v & 0xEF) | UNUSED_FLAG
				sp++
				var pcLo byte
				if dpb[1] == 0 {
					pcLo = memDirect[0x0100|uint16(sp)]
				} else {
					pcLo = adapter.Read(0x0100 | uint16(sp))
				}
				sp++
				var pcHi byte
				if dpb[1] == 0 {
					pcHi = memDirect[0x0100|uint16(sp)]
				} else {
					pcHi = adapter.Read(0x0100 | uint16(sp))
				}
				pc = uint16(pcLo) | uint16(pcHi)<<8
				cycles += 6

			// ================================================================
			// NOP
			// ================================================================

			case 0xEA: // NOP
				cycles += 2
			case 0x1A, 0x3A, 0x5A, 0x7A, 0xDA, 0xFA: // unofficial NOP implied
				cycles += 2
			case 0x80, 0x82, 0x89, 0xC2, 0xE2: // unofficial NOP imm
				pc++
				cycles += 2
			case 0x04, 0x44, 0x64: // unofficial NOP zp
				pc++
				cycles += 3
			case 0x14, 0x34, 0x54, 0x74, 0xD4, 0xF4: // unofficial NOP zp,X
				pc++
				cycles += 4
			case 0x0C: // unofficial NOP abs
				pc += 2
				cycles += 4
			case 0x1C, 0x3C, 0x5C, 0x7C, 0xDC, 0xFC: // unofficial NOP abs,X
				var lo byte
				if dpb[pc>>8] == 0 {
					lo = memDirect[pc]
				} else {
					lo = adapter.Read(pc)
				}
				pc++
				var hi byte
				if dpb[pc>>8] == 0 {
					hi = memDirect[pc]
				} else {
					hi = adapter.Read(pc)
				}
				pc++
				base := uint16(lo) | uint16(hi)<<8
				addr := base + uint16(x)
				// Dummy read honors MMIO side effects on the target address.
				if dpb[addr>>8] == 0 {
					_ = memDirect[addr]
				} else {
					_ = adapter.Read(addr)
				}
				cycles += 4
				if (base & 0xFF00) != (addr & 0xFF00) {
					cycles++
				}

			// ================================================================
			// Fallback — unofficial/illegal opcodes and anything else not
			// in the validation subset go through the legacy opcode table.
			// ================================================================

			default:
				cpu_6502.PC = pc
				cpu_6502.SP = sp
				cpu_6502.A = a
				cpu_6502.X = x
				cpu_6502.Y = y
				cpu_6502.SR = sr
				cpu_6502.Cycles = cycles
				cpu_6502.opcodeTable[opcode](cpu_6502)
				pc = cpu_6502.PC
				sp = cpu_6502.SP
				a = cpu_6502.A
				x = cpu_6502.X
				y = cpu_6502.Y
				sr = cpu_6502.SR
				cycles = cpu_6502.Cycles
			}

			if !cpu_6502.running.Load() {
				break
			}
			if cpu_6502.PerfEnabled {
				cpu_6502.InstructionCount++
			}
		}

		if cpu_6502.PerfEnabled {
			now := time.Now()
			if now.Sub(cpu_6502.lastPerfReport) >= time.Second {
				elapsed := now.Sub(cpu_6502.perfStartTime).Seconds()
				if elapsed > 0 {
					ips := float64(cpu_6502.InstructionCount) / elapsed
					mips := ips / 1_000_000
					fmt.Printf("\r6502: %.2f MIPS (%.0f instructions in %.1fs)", mips, float64(cpu_6502.InstructionCount), elapsed)
				}
				cpu_6502.lastPerfReport = now
			}
		}
	}

	// Final spill on loop exit.
	cpu_6502.PC = pc
	cpu_6502.SP = sp
	cpu_6502.A = a
	cpu_6502.X = x
	cpu_6502.Y = y
	cpu_6502.SR = sr
	cpu_6502.Cycles = cycles
}

// stepFast performs a single-instruction step sharing the fast-path memory
// rules used by ExecuteFast. It is used by Step() when fastAdapter != nil and
// Debug == false. Dispatch itself still goes through opcodeTable; the
// performance win here comes from the directPageBitmap-backed opcode fetch
// and not from inlined case bodies. Step() is called at most a few thousand
// times per audio frame, so full inline dispatch is not worth the code size.
func (cpu_6502 *CPU_6502) stepFast() int {
	cpu_6502.ensureOpcodeTableReady()
	cpu_6502.ensureDirectPageBitmap()

	if !cpu_6502.running.Load() || cpu_6502.resetting.Load() {
		return 0
	}
	if !cpu_6502.rdyLine.Load() {
		cpu_6502.rdyHold = true
		return 0
	}
	cpu_6502.rdyHold = false

	if cpu_6502.nmiPending.Load() {
		cpu_6502.handleInterrupt(NMI_VECTOR, true)
		cpu_6502.nmiPending.Store(false)
	} else if cpu_6502.irqPending.Load() && cpu_6502.SR&INTERRUPT_FLAG == 0 {
		cpu_6502.handleInterrupt(IRQ_VECTOR, false)
		cpu_6502.irqPending.Store(false)
	}

	startCycles := cpu_6502.Cycles

	// Direct bitmap-aware opcode fetch.
	adapter := cpu_6502.fastAdapter
	var opcode byte
	if cpu_6502.directPageBitmap[cpu_6502.PC>>8] == 0 {
		opcode = adapter.memDirect[cpu_6502.PC]
	} else {
		opcode = adapter.Read(cpu_6502.PC)
	}
	cpu_6502.PC++

	cpu_6502.opcodeTable[opcode](cpu_6502)

	return int(cpu_6502.Cycles - startCycles)
}
