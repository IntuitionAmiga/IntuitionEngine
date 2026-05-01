// jit_ie64_turbo.go - IE64 turbo-tier policy, planning, and diagnostics.

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
)

const (
	ie64JITTier1     = 0
	ie64JITTierTurbo = 1
)

// ie64TurboEnabled reports whether hot IE64 region promotion is allowed.
// Region promotion already ships as the IE64 high tier; keep it enabled by
// default and provide an explicit kill switch for parity/bisect runs.
func ie64TurboEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("IE64_JIT_TURBO"))) {
	case "0", "false", "off", "no":
		return false
	default:
		return true
	}
}

func ie64TurboStatsEnabled() bool {
	return os.Getenv("IE64_JIT_STATS") == "1"
}

type ie64TurboStats struct {
	tier1Blocks     atomic.Uint64
	turboCandidates atomic.Uint64
	turboRegions    atomic.Uint64
	turboRejected   atomic.Uint64
	spills          atomic.Uint64
	fpuSpills       atomic.Uint64
	directRAMProofs atomic.Uint64
	inlinedCalls    atomic.Uint64
	ioBails         atomic.Uint64
	invalidations   atomic.Uint64
}

var globalIE64TurboStats ie64TurboStats

type ie64TurboStatsSnapshot struct {
	tier1Blocks     uint64
	turboCandidates uint64
	turboRegions    uint64
	turboRejected   uint64
	spills          uint64
	fpuSpills       uint64
	directRAMProofs uint64
	inlinedCalls    uint64
	ioBails         uint64
	invalidations   uint64
}

func ie64TurboStatsLoad() ie64TurboStatsSnapshot {
	return ie64TurboStatsSnapshot{
		tier1Blocks:     globalIE64TurboStats.tier1Blocks.Load(),
		turboCandidates: globalIE64TurboStats.turboCandidates.Load(),
		turboRegions:    globalIE64TurboStats.turboRegions.Load(),
		turboRejected:   globalIE64TurboStats.turboRejected.Load(),
		spills:          globalIE64TurboStats.spills.Load(),
		fpuSpills:       globalIE64TurboStats.fpuSpills.Load(),
		directRAMProofs: globalIE64TurboStats.directRAMProofs.Load(),
		inlinedCalls:    globalIE64TurboStats.inlinedCalls.Load(),
		ioBails:         globalIE64TurboStats.ioBails.Load(),
		invalidations:   globalIE64TurboStats.invalidations.Load(),
	}
}

func (s ie64TurboStatsSnapshot) Sub(base ie64TurboStatsSnapshot) ie64TurboStatsSnapshot {
	return ie64TurboStatsSnapshot{
		tier1Blocks:     s.tier1Blocks - base.tier1Blocks,
		turboCandidates: s.turboCandidates - base.turboCandidates,
		turboRegions:    s.turboRegions - base.turboRegions,
		turboRejected:   s.turboRejected - base.turboRejected,
		spills:          s.spills - base.spills,
		fpuSpills:       s.fpuSpills - base.fpuSpills,
		directRAMProofs: s.directRAMProofs - base.directRAMProofs,
		inlinedCalls:    s.inlinedCalls - base.inlinedCalls,
		ioBails:         s.ioBails - base.ioBails,
		invalidations:   s.invalidations - base.invalidations,
	}
}

func (s ie64TurboStatsSnapshot) Print() {
	fmt.Printf("IE64 JIT stats: tier1=%d turbo_regions=%d turbo_candidates=%d turbo_rejected=%d spills=%d fpu_spills=%d direct_ram_proofs=%d inlined_calls=%d io_bails=%d invalidations=%d\n",
		s.tier1Blocks,
		s.turboRegions,
		s.turboCandidates,
		s.turboRejected,
		s.spills,
		s.fpuSpills,
		s.directRAMProofs,
		s.inlinedCalls,
		s.ioBails,
		s.invalidations,
	)
}

// ie64TurboPlan is analysis metadata for the IE64 turbo region tier. The
// current native emitter still uses the fixed Tier-1 mapping, but the plan
// records the dynamic register choices and spill pressure at region scope so
// the scratch-allocator refactor can consume a tested contract.
type ie64TurboPlan struct {
	residentGuestRegs []byte
	residentHostRegs  []byte
	spillOps          int
	fpuSpillOps       int
}

var ie64TurboHostRegs = []byte{
	amd64RBX, amd64RBP, amd64R12, amd64R13, amd64R10, amd64R11,
}

func ie64PlanTurboRegion(region *ie64Region) ie64TurboPlan {
	if region == nil {
		return ie64TurboPlan{}
	}
	var weights [32]int
	var spillOps int
	for _, block := range region.blocks {
		for i := range block {
			ji := &block[i]
			ie64AccumulateTurboWeights(ji, &weights)
			spillOps += ie64EstimatedSpillOps(ji)
		}
	}

	// R0 is hardwired zero and R31/SP remains pinned to R14 by ABI.
	weights[0] = 0
	weights[31] = 0

	plan := ie64TurboPlan{}
	for len(plan.residentGuestRegs) < len(ie64TurboHostRegs) {
		bestReg := byte(0)
		bestWeight := 0
		for reg := 1; reg < 31; reg++ {
			if weights[reg] > bestWeight {
				bestReg = byte(reg)
				bestWeight = weights[reg]
			}
		}
		if bestWeight == 0 {
			break
		}
		plan.residentGuestRegs = append(plan.residentGuestRegs, bestReg)
		plan.residentHostRegs = append(plan.residentHostRegs, ie64TurboHostRegs[len(plan.residentHostRegs)])
		weights[bestReg] = 0
	}
	plan.spillOps = spillOps
	return plan
}

func ie64AccumulateTurboWeights(ji *JITInstr, weights *[32]int) {
	read := func(reg byte, weight int) {
		if reg != 0 && reg < 32 {
			weights[reg] += weight
		}
	}
	write := func(reg byte, weight int) {
		if reg != 0 && reg < 32 {
			weights[reg] += weight
		}
	}

	switch ji.opcode {
	case OP_MOVE:
		if ji.xbit == 0 {
			read(ji.rs, 2)
		}
		write(ji.rd, 2)
	case OP_MOVT, OP_MOVEQ:
		write(ji.rd, 2)
	case OP_LEA:
		read(ji.rs, 3)
		write(ji.rd, 3)
	case OP_ADD, OP_SUB, OP_AND64, OP_OR64, OP_EOR,
		OP_MULU, OP_MULS, OP_DIVU, OP_DIVS, OP_MOD64, OP_MODS, OP_MULHU, OP_MULHS,
		OP_LSL, OP_LSR, OP_ASR, OP_ROL, OP_ROR:
		read(ji.rs, 3)
		if ji.xbit == 0 {
			read(ji.rt, 3)
		}
		write(ji.rd, 3)
	case OP_NEG, OP_NOT64, OP_CLZ, OP_SEXT, OP_CTZ, OP_POPCNT, OP_BSWAP:
		read(ji.rs, 3)
		write(ji.rd, 3)
	case OP_LOAD:
		read(ji.rs, 5)
		write(ji.rd, 4)
	case OP_STORE:
		read(ji.rs, 5)
		read(ji.rd, 4)
	case OP_BEQ, OP_BNE, OP_BLT, OP_BGE, OP_BGT, OP_BLE, OP_BHI, OP_BLS:
		read(ji.rs, 4)
		read(ji.rt, 4)
	case OP_JMP, OP_JSR_IND:
		read(ji.rs, 4)
	case OP_JSR64, OP_RTS64, OP_PUSH64, OP_POP64:
		read(31, 5)
		write(31, 5)
	case OP_FMOVI, OP_FCVTIF, OP_FMOVSC, OP_FMOVCC:
		read(ji.rs, 2)
	case OP_FMOVO, OP_FCMP, OP_FCVTFI, OP_FMOVSR, OP_FMOVCR, OP_DCMP, OP_DCVTFI:
		write(ji.rd, 2)
	case OP_FLOAD, OP_FSTORE, OP_DLOAD, OP_DSTORE:
		read(ji.rs, 4)
	}
}

func ie64EstimatedSpillOps(ji *JITInstr) int {
	ops := 0
	isSpilled := func(reg byte) bool {
		if reg == 0 || reg == 31 {
			return false
		}
		_, mapped := ie64ToAMD64Reg(reg)
		return !mapped
	}
	countRead := func(reg byte) {
		if isSpilled(reg) {
			ops++
		}
	}
	countWrite := func(reg byte) {
		if isSpilled(reg) {
			ops++
		}
	}

	switch ji.opcode {
	case OP_MOVE:
		if ji.xbit == 0 {
			countRead(ji.rs)
		}
		countWrite(ji.rd)
	case OP_MOVT:
		countRead(ji.rd)
		countWrite(ji.rd)
	case OP_MOVEQ:
		countWrite(ji.rd)
	case OP_LEA:
		countRead(ji.rs)
		countWrite(ji.rd)
	case OP_ADD, OP_SUB, OP_AND64, OP_OR64, OP_EOR,
		OP_MULU, OP_MULS, OP_DIVU, OP_DIVS, OP_MOD64, OP_MODS, OP_MULHU, OP_MULHS,
		OP_LSL, OP_LSR, OP_ASR, OP_ROL, OP_ROR:
		countRead(ji.rs)
		if ji.xbit == 0 {
			countRead(ji.rt)
		}
		countWrite(ji.rd)
	case OP_NEG, OP_NOT64, OP_CLZ, OP_SEXT, OP_CTZ, OP_POPCNT, OP_BSWAP:
		countRead(ji.rs)
		countWrite(ji.rd)
	case OP_LOAD:
		countRead(ji.rs)
		countWrite(ji.rd)
	case OP_STORE:
		countRead(ji.rs)
		countRead(ji.rd)
	case OP_BEQ, OP_BNE, OP_BLT, OP_BGE, OP_BGT, OP_BLE, OP_BHI, OP_BLS:
		countRead(ji.rs)
		countRead(ji.rt)
	case OP_JMP, OP_JSR_IND:
		countRead(ji.rs)
	case OP_FMOVI, OP_FCVTIF, OP_FMOVSC, OP_FMOVCC:
		countRead(ji.rs)
	case OP_FMOVO, OP_FCMP, OP_FCVTFI, OP_FMOVSR, OP_FMOVCR, OP_DCMP, OP_DCVTFI:
		countWrite(ji.rd)
	case OP_FLOAD, OP_FSTORE, OP_DLOAD, OP_DSTORE:
		countRead(ji.rs)
	}
	return ops
}

func ie64CountFusedLeafCalls(instrs []JITInstr) int {
	n := 0
	for i := range instrs {
		if instrs[i].fusedFlag&ie64FusedJSRLeafCall != 0 {
			n++
		}
	}
	return n
}
