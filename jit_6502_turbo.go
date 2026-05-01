// jit_6502_turbo.go - 6502 turbo-tier policy, analysis, and diagnostics.

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
)

const (
	p65JITTier1     = 0
	p65JITTierTurbo = 1

	// The current generic promoted-block emitter is kept for diagnostics and
	// follow-up region work, but the measured wins in this pass come from the
	// bounded-loop fast tier. Leave generic promotion off until it beats tier 1.
	p65TurboRegionPromotion = false
)

var p65TierController = func() *TierController {
	c := NewTierController(P65RegProfile)
	// 6502 blocks are small and hot loops amortize quickly.
	c.Thresholds.PromoteAtExecCount = 32
	return c
}()

func p65TurboEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("P65_JIT_TURBO"))) {
	case "0", "false", "off", "no":
		return false
	default:
		return true
	}
}

func p65TurboStatsEnabled() bool {
	return os.Getenv("P65_JIT_STATS") == "1"
}

type p65TurboStats struct {
	tier1Blocks         atomic.Uint64
	turboCandidates     atomic.Uint64
	turboRegions        atomic.Uint64
	turboRejected       atomic.Uint64
	rejectUnsupported   atomic.Uint64
	rejectDynamicJump   atomic.Uint64
	rejectTrap          atomic.Uint64
	rejectDecimal       atomic.Uint64
	rejectMemory        atomic.Uint64
	rejectCall          atomic.Uint64
	directMemoryProofs  atomic.Uint64
	loopSpecializations atomic.Uint64
	inlinedCalls        atomic.Uint64
	bails               atomic.Uint64
	invalidations       atomic.Uint64
	chainExits          atomic.Uint64
}

var globalP65TurboStats p65TurboStats

type p65TurboStatsSnapshot struct {
	tier1Blocks         uint64
	turboCandidates     uint64
	turboRegions        uint64
	turboRejected       uint64
	rejectUnsupported   uint64
	rejectDynamicJump   uint64
	rejectTrap          uint64
	rejectDecimal       uint64
	rejectMemory        uint64
	rejectCall          uint64
	directMemoryProofs  uint64
	loopSpecializations uint64
	inlinedCalls        uint64
	bails               uint64
	invalidations       uint64
	chainExits          uint64
}

func p65TurboStatsLoad() p65TurboStatsSnapshot {
	return p65TurboStatsSnapshot{
		tier1Blocks:         globalP65TurboStats.tier1Blocks.Load(),
		turboCandidates:     globalP65TurboStats.turboCandidates.Load(),
		turboRegions:        globalP65TurboStats.turboRegions.Load(),
		turboRejected:       globalP65TurboStats.turboRejected.Load(),
		rejectUnsupported:   globalP65TurboStats.rejectUnsupported.Load(),
		rejectDynamicJump:   globalP65TurboStats.rejectDynamicJump.Load(),
		rejectTrap:          globalP65TurboStats.rejectTrap.Load(),
		rejectDecimal:       globalP65TurboStats.rejectDecimal.Load(),
		rejectMemory:        globalP65TurboStats.rejectMemory.Load(),
		rejectCall:          globalP65TurboStats.rejectCall.Load(),
		directMemoryProofs:  globalP65TurboStats.directMemoryProofs.Load(),
		loopSpecializations: globalP65TurboStats.loopSpecializations.Load(),
		inlinedCalls:        globalP65TurboStats.inlinedCalls.Load(),
		bails:               globalP65TurboStats.bails.Load(),
		invalidations:       globalP65TurboStats.invalidations.Load(),
		chainExits:          globalP65TurboStats.chainExits.Load(),
	}
}

func (s p65TurboStatsSnapshot) Sub(base p65TurboStatsSnapshot) p65TurboStatsSnapshot {
	return p65TurboStatsSnapshot{
		tier1Blocks:         s.tier1Blocks - base.tier1Blocks,
		turboCandidates:     s.turboCandidates - base.turboCandidates,
		turboRegions:        s.turboRegions - base.turboRegions,
		turboRejected:       s.turboRejected - base.turboRejected,
		rejectUnsupported:   s.rejectUnsupported - base.rejectUnsupported,
		rejectDynamicJump:   s.rejectDynamicJump - base.rejectDynamicJump,
		rejectTrap:          s.rejectTrap - base.rejectTrap,
		rejectDecimal:       s.rejectDecimal - base.rejectDecimal,
		rejectMemory:        s.rejectMemory - base.rejectMemory,
		rejectCall:          s.rejectCall - base.rejectCall,
		directMemoryProofs:  s.directMemoryProofs - base.directMemoryProofs,
		loopSpecializations: s.loopSpecializations - base.loopSpecializations,
		inlinedCalls:        s.inlinedCalls - base.inlinedCalls,
		bails:               s.bails - base.bails,
		invalidations:       s.invalidations - base.invalidations,
		chainExits:          s.chainExits - base.chainExits,
	}
}

func (s p65TurboStatsSnapshot) Print() {
	fmt.Printf("6502 JIT stats: tier1=%d turbo_candidates=%d turbo_regions=%d turbo_rejected=%d rejects={unsupported:%d dynamic_jump:%d trap:%d decimal:%d memory:%d call:%d} direct_memory_proofs=%d loop_specializations=%d inlined_calls=%d bails=%d invalidations=%d chain_exits=%d\n",
		s.tier1Blocks,
		s.turboCandidates,
		s.turboRegions,
		s.turboRejected,
		s.rejectUnsupported,
		s.rejectDynamicJump,
		s.rejectTrap,
		s.rejectDecimal,
		s.rejectMemory,
		s.rejectCall,
		s.directMemoryProofs,
		s.loopSpecializations,
		s.inlinedCalls,
		s.bails,
		s.invalidations,
		s.chainExits,
	)
}

type p65TurboRejectReason uint8

const (
	p65TurboRejectNone p65TurboRejectReason = iota
	p65TurboRejectUnsupported
	p65TurboRejectDynamicJump
	p65TurboRejectTrap
	p65TurboRejectDecimal
	p65TurboRejectMemory
	p65TurboRejectCall
)

func p65RecordTurboReject(reason p65TurboRejectReason) {
	globalP65TurboStats.turboRejected.Add(1)
	switch reason {
	case p65TurboRejectUnsupported:
		globalP65TurboStats.rejectUnsupported.Add(1)
	case p65TurboRejectDynamicJump:
		globalP65TurboStats.rejectDynamicJump.Add(1)
	case p65TurboRejectTrap:
		globalP65TurboStats.rejectTrap.Add(1)
	case p65TurboRejectDecimal:
		globalP65TurboStats.rejectDecimal.Add(1)
	case p65TurboRejectMemory:
		globalP65TurboStats.rejectMemory.Add(1)
	case p65TurboRejectCall:
		globalP65TurboStats.rejectCall.Add(1)
	}
}

type p65TurboPlan struct {
	directMemoryProofs int
	loopSpecialized    bool
	inlinedCalls       int
}

func p65AnalyzeTurboRegion(cpu *CPU_6502, instrs []JIT6502Instr, startPC uint16) (p65TurboPlan, p65TurboRejectReason) {
	if len(instrs) == 0 {
		return p65TurboPlan{}, p65TurboRejectUnsupported
	}
	plan := p65TurboPlan{}
	decimalKnownClear := cpu.SR&DECIMAL_FLAG == 0
	for i := range instrs {
		ji := instrs[i]
		if !jit6502IsCompilable[ji.opcode] {
			return p65TurboPlan{}, p65TurboRejectUnsupported
		}
		switch ji.opcode {
		case 0x00, 0x40:
			return p65TurboPlan{}, p65TurboRejectTrap
		case 0x02, 0x12, 0x22, 0x32, 0x42, 0x52, 0x62, 0x72, 0x92, 0xB2, 0xD2, 0xF2:
			return p65TurboPlan{}, p65TurboRejectTrap
		case 0x6C:
			return p65TurboPlan{}, p65TurboRejectDynamicJump
		case 0x20:
			if ji.fused&p65FusedJSRLeafCall == 0 {
				return p65TurboPlan{}, p65TurboRejectCall
			}
			plan.inlinedCalls++
		case 0x60:
			if ji.fused&p65FusedRTSLeafReturn == 0 {
				return p65TurboPlan{}, p65TurboRejectCall
			}
			return p65TurboPlan{}, p65TurboRejectCall
		case 0xF8:
			decimalKnownClear = false
		case 0xD8:
			decimalKnownClear = true
		case 0x69, 0x65, 0x75, 0x6D, 0x7D, 0x79, 0x61, 0x71,
			0xE9, 0xE5, 0xF5, 0xED, 0xFD, 0xF9, 0xE1, 0xF1:
			if !decimalKnownClear {
				return p65TurboPlan{}, p65TurboRejectDecimal
			}
		}
		proofs, ok := p65ProveTurboMemory(cpu, ji)
		if !ok {
			return p65TurboPlan{}, p65TurboRejectMemory
		}
		plan.directMemoryProofs += proofs

		if ji.opcode == 0xD0 {
			branchPC := startPC + ji.pcOffset + 2
			targetPC := uint16(int(branchPC) + int(int8(ji.operand&0xFF)))
			targetOff := int(targetPC) - int(startPC)
			if targetOff >= 0 {
				targetIdx := findInstrByPC(instrs, uint16(targetOff))
				if targetIdx >= 0 && targetIdx <= i && p65IsBoundedCounterBranchTurbo(instrs, i, targetIdx) {
					plan.loopSpecialized = true
				}
			}
		}
	}
	return plan, p65TurboRejectNone
}

func p65ProveTurboMemory(cpu *CPU_6502, ji JIT6502Instr) (int, bool) {
	switch ji.opcode {
	case 0xA9, 0xA2, 0xA0, 0xEA, 0xE8, 0xC8, 0xCA, 0x88,
		0xAA, 0x8A, 0xA8, 0x98, 0xBA, 0x9A,
		0x18, 0x38, 0x58, 0x78, 0xD8, 0xF8, 0xB8,
		0x0A, 0x4A, 0x2A, 0x6A,
		0x08, 0x28, 0x48, 0x68,
		0x90, 0xB0, 0xF0, 0xD0, 0x30, 0x10, 0x70, 0x50,
		0x4C:
		return 0, true
	}
	if p65OpcodeUsesIndirectAddressing(ji.opcode) {
		return 0, false
	}
	if p65OpcodeUsesZeroPage(ji.opcode) {
		if cpu.directPageBitmap[0] != 0 {
			return 0, false
		}
		return 1, true
	}
	if p65OpcodeUsesAbsolute(ji.opcode) {
		if p65DirectPageRange(cpu, uint32(ji.operand), p65AbsoluteMaxOffset(ji.opcode)) {
			return 1, true
		}
		return 0, false
	}
	return 0, true
}

func p65OpcodeUsesZeroPage(op byte) bool {
	switch op {
	case 0xA5, 0xA6, 0xA4, 0xB5, 0xB4, 0xB6,
		0x85, 0x86, 0x84, 0x95, 0x94, 0x96,
		0x65, 0x75, 0xE5, 0xF5, 0x25, 0x35, 0x05, 0x15, 0x45, 0x55,
		0xC5, 0xD5, 0xE4, 0xC4, 0x24,
		0xE6, 0xF6, 0xC6, 0xD6,
		0x06, 0x16, 0x46, 0x56, 0x26, 0x36, 0x66, 0x76:
		return true
	default:
		return false
	}
}

func p65OpcodeUsesAbsolute(op byte) bool {
	switch op {
	case 0xAD, 0xAE, 0xAC, 0xBD, 0xBC, 0xB9, 0xBE,
		0x8D, 0x8E, 0x8C, 0x9D, 0x99,
		0x6D, 0x7D, 0x79, 0xED, 0xFD, 0xF9,
		0x2D, 0x3D, 0x39, 0x0D, 0x1D, 0x19, 0x4D, 0x5D, 0x59,
		0xCD, 0xDD, 0xD9, 0xEC, 0xCC, 0x2C,
		0xEE, 0xFE, 0xCE, 0xDE,
		0x0E, 0x1E, 0x4E, 0x5E, 0x2E, 0x3E, 0x6E, 0x7E:
		return true
	default:
		return false
	}
}

func p65OpcodeUsesIndirectAddressing(op byte) bool {
	switch op {
	case 0xA1, 0xB1, 0x81, 0x91,
		0x61, 0x71, 0xE1, 0xF1,
		0x21, 0x31, 0x01, 0x11, 0x41, 0x51,
		0xC1, 0xD1:
		return true
	default:
		return false
	}
}

func p65AbsoluteMaxOffset(op byte) uint32 {
	switch op {
	case 0xBD, 0xBC, 0x9D, 0x7D, 0xFD, 0x3D, 0x1D, 0x5D, 0xDD, 0xFE, 0xDE, 0x1E, 0x5E, 0x3E, 0x7E:
		return 0xFF
	case 0xB9, 0xBE, 0x99, 0x79, 0xF9, 0x39, 0x19, 0x59, 0xD9:
		return 0xFF
	default:
		return 0
	}
}

func p65DirectPageRange(cpu *CPU_6502, start, maxOffset uint32) bool {
	if start > 0xFFFF {
		return false
	}
	end := start + maxOffset
	if end > 0xFFFF {
		end = 0xFFFF
	}
	for page := start >> 8; page <= end>>8; page++ {
		if cpu.directPageBitmap[page&0xFF] != 0 {
			return false
		}
	}
	return true
}

func p65WritesX(op byte) bool {
	switch op {
	case 0xA2, 0xA6, 0xB6, 0xAE, 0xBE, // LDX
		0xAA,       // TAX
		0xBA,       // TSX
		0xE8, 0xCA: // INX, DEX
		return true
	default:
		return false
	}
}

func p65IsBoundedCounterBranchTurbo(instrs []JIT6502Instr, branchIdx, targetIdx int) bool {
	if branchIdx <= 0 || branchIdx >= len(instrs) || targetIdx < 0 || targetIdx >= branchIdx {
		return false
	}
	if instrs[branchIdx].opcode != 0xD0 {
		return false
	}
	counterOp := instrs[branchIdx-1].opcode
	var writesCounter func(byte) bool
	switch counterOp {
	case 0xC8, 0x88:
		writesCounter = p65WritesY
	case 0xE8, 0xCA:
		writesCounter = p65WritesX
	default:
		return false
	}
	for i := targetIdx; i < branchIdx-1; i++ {
		op := instrs[i].opcode
		if p65CanSkipCounterUpdate(op) || writesCounter(op) {
			return false
		}
	}
	return true
}

func p65ScanTurboBlock(cpu *CPU_6502, mem []byte, startPC uint16, memSize int) []JIT6502Instr {
	instrs := make([]JIT6502Instr, 0, 64)
	pc := startPC
	for len(instrs) < jit6502MaxBlockSize {
		if int(pc) >= memSize {
			break
		}
		opcode := mem[pc]
		length := jit6502InstrLengths[opcode]
		if int(pc)+int(length) > memSize {
			break
		}
		if p65TurboTrapOpcode(opcode) && len(instrs) > 0 {
			break
		}
		if !jit6502IsCompilable[opcode] && !jit6502IsBlockTerminator(opcode) {
			break
		}
		ji := JIT6502Instr{opcode: opcode, length: length, pcOffset: pc - startPC}
		switch length {
		case 2:
			ji.operand = uint16(mem[pc+1])
		case 3:
			ji.operand = uint16(mem[pc+1]) | uint16(mem[pc+2])<<8
		}
		if opcode == 0x20 {
			returnPC := pc + 3
			if leaf, ok := p65AnalyzeLeafCall(cpu, mem, ji.operand, startPC, memSize); ok && len(instrs)+len(leaf)+1 < jit6502MaxBlockSize {
				ji.fused = p65FusedJSRLeafCall
				instrs = append(instrs, ji)
				instrs = append(instrs, leaf...)
				pc = returnPC
				continue
			}
		}
		instrs = append(instrs, ji)
		pc += uint16(length)
		if jit6502IsBlockTerminator(opcode) {
			break
		}
	}
	return instrs
}

func p65TurboTrapOpcode(opcode byte) bool {
	switch opcode {
	case 0x00, 0x40,
		0x02, 0x12, 0x22, 0x32, 0x42, 0x52, 0x62, 0x72, 0x92, 0xB2, 0xD2, 0xF2:
		return true
	default:
		return false
	}
}

func p65AnalyzeLeafCall(cpu *CPU_6502, mem []byte, targetPC uint16, regionStart uint16, memSize int) ([]JIT6502Instr, bool) {
	if int(targetPC) >= memSize {
		return nil, false
	}
	leaf := make([]JIT6502Instr, 0, 8)
	pc := targetPC
	for len(leaf) < 8 {
		if int(pc) >= memSize {
			return nil, false
		}
		opcode := mem[pc]
		length := jit6502InstrLengths[opcode]
		if int(pc)+int(length) > memSize || !jit6502IsCompilable[opcode] {
			return nil, false
		}
		ji := JIT6502Instr{opcode: opcode, length: length, pcOffset: pc - regionStart}
		switch length {
		case 2:
			ji.operand = uint16(mem[pc+1])
		case 3:
			ji.operand = uint16(mem[pc+1]) | uint16(mem[pc+2])<<8
		}
		switch opcode {
		case 0x20, 0x4C, 0x6C, 0x40, 0x00:
			return nil, false
		case 0x60:
			ji.fused = p65FusedRTSLeafReturn
			leaf = append(leaf, ji)
			plan, reason := p65AnalyzeTurboRegion(cpu, leaf, regionStart)
			return leaf, reason == p65TurboRejectNone && plan.inlinedCalls == 0
		}
		if _, ok := p65ProveTurboMemory(cpu, ji); !ok {
			return nil, false
		}
		leaf = append(leaf, ji)
		pc += uint16(length)
	}
	return nil, false
}

func compileBlock6502Turbo(cpu *CPU_6502, instrs []JIT6502Instr, startPC uint16, execMem *ExecMem, codePageBitmap *[256]byte) (*JITBlock, p65TurboPlan, p65TurboRejectReason, error) {
	plan, reason := p65AnalyzeTurboRegion(cpu, instrs, startPC)
	if reason != p65TurboRejectNone {
		return nil, plan, reason, nil
	}
	if plan.inlinedCalls == 0 {
		return nil, plan, p65TurboRejectUnsupported, nil
	}
	block, err := compileBlock6502WithOptions(instrs, startPC, execMem, codePageBitmap, p65CompileOptions{
		tier:              p65JITTierTurbo,
		turboCounterLoops: true,
		turboDirectMemory: true,
	})
	if err != nil {
		return nil, plan, p65TurboRejectNone, err
	}
	return block, plan, p65TurboRejectNone, nil
}
