// jit_ie64_region_policy.go - IE64 region-tier policy, planning, and diagnostics.

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
)

const (
	ie64JITTier1      = 0
	ie64JITTierRegion = 1
)

// ie64RegionPromotionEnabled reports whether hot IE64 region promotion is allowed.
// Region promotion already ships as the IE64 high tier; keep it enabled by
// default and provide an explicit kill switch for parity/bisect runs.
func ie64RegionPromotionEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("IE64_JIT_REGIONS"))) {
	case "0", "false", "off", "no":
		return false
	default:
		return true
	}
}

func ie64RegionMMUEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("IE64_JIT_REGION_MMU"))) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

func ie64JITStatsEnabled() bool {
	return os.Getenv("IE64_JIT_STATS") == "1"
}

type ie64JITStats struct {
	tier1Blocks         atomic.Uint64
	regionCandidates    atomic.Uint64
	regions             atomic.Uint64
	regionRejected      atomic.Uint64
	spills              atomic.Uint64
	fpuSpills           atomic.Uint64
	directRAMProofs     atomic.Uint64
	inlinedCalls        atomic.Uint64
	ioBails             atomic.Uint64
	ioBailOpcodes       [256]atomic.Uint64
	invalidations       atomic.Uint64
	helperExits         [HELPER_DTRANS + 1]atomic.Uint64
	helperResumes       atomic.Uint64
	helperResumeCancels atomic.Uint64
}

var globalIE64JITStats ie64JITStats

type ie64JITStatsSnapshot struct {
	tier1Blocks         uint64
	regionCandidates    uint64
	regions             uint64
	regionRejected      uint64
	spills              uint64
	fpuSpills           uint64
	directRAMProofs     uint64
	inlinedCalls        uint64
	ioBails             uint64
	ioBailOpcodes       [256]uint64
	invalidations       uint64
	helperExits         [HELPER_DTRANS + 1]uint64
	helperResumes       uint64
	helperResumeCancels uint64
}

func ie64JITStatsLoad() ie64JITStatsSnapshot {
	snap := ie64JITStatsSnapshot{
		tier1Blocks:         globalIE64JITStats.tier1Blocks.Load(),
		regionCandidates:    globalIE64JITStats.regionCandidates.Load(),
		regions:             globalIE64JITStats.regions.Load(),
		regionRejected:      globalIE64JITStats.regionRejected.Load(),
		spills:              globalIE64JITStats.spills.Load(),
		fpuSpills:           globalIE64JITStats.fpuSpills.Load(),
		directRAMProofs:     globalIE64JITStats.directRAMProofs.Load(),
		inlinedCalls:        globalIE64JITStats.inlinedCalls.Load(),
		ioBails:             globalIE64JITStats.ioBails.Load(),
		invalidations:       globalIE64JITStats.invalidations.Load(),
		helperResumes:       globalIE64JITStats.helperResumes.Load(),
		helperResumeCancels: globalIE64JITStats.helperResumeCancels.Load(),
	}
	for i := range snap.helperExits {
		snap.helperExits[i] = globalIE64JITStats.helperExits[i].Load()
	}
	for i := range snap.ioBailOpcodes {
		snap.ioBailOpcodes[i] = globalIE64JITStats.ioBailOpcodes[i].Load()
	}
	return snap
}

func (s ie64JITStatsSnapshot) Sub(base ie64JITStatsSnapshot) ie64JITStatsSnapshot {
	out := ie64JITStatsSnapshot{
		tier1Blocks:         s.tier1Blocks - base.tier1Blocks,
		regionCandidates:    s.regionCandidates - base.regionCandidates,
		regions:             s.regions - base.regions,
		regionRejected:      s.regionRejected - base.regionRejected,
		spills:              s.spills - base.spills,
		fpuSpills:           s.fpuSpills - base.fpuSpills,
		directRAMProofs:     s.directRAMProofs - base.directRAMProofs,
		inlinedCalls:        s.inlinedCalls - base.inlinedCalls,
		ioBails:             s.ioBails - base.ioBails,
		invalidations:       s.invalidations - base.invalidations,
		helperResumes:       s.helperResumes - base.helperResumes,
		helperResumeCancels: s.helperResumeCancels - base.helperResumeCancels,
	}
	for i := range out.helperExits {
		out.helperExits[i] = s.helperExits[i] - base.helperExits[i]
	}
	for i := range out.ioBailOpcodes {
		out.ioBailOpcodes[i] = s.ioBailOpcodes[i] - base.ioBailOpcodes[i]
	}
	return out
}

func (s ie64JITStatsSnapshot) Print() {
	fmt.Printf("IE64 JIT stats: tier1=%d regions=%d region_candidates=%d region_rejected=%d spills=%d fpu_spills=%d direct_ram_proofs=%d inlined_calls=%d io_bails=%d invalidations=%d helper_resumes=%d helper_resume_cancels=%d\n",
		s.tier1Blocks,
		s.regions,
		s.regionCandidates,
		s.regionRejected,
		s.spills,
		s.fpuSpills,
		s.directRAMProofs,
		s.inlinedCalls,
		s.ioBails,
		s.invalidations,
		s.helperResumes,
		s.helperResumeCancels,
	)
}

// ie64RegionPlan is analysis metadata for the IE64 region tier. The
// current native emitter still uses the fixed Tier-1 mapping, but the plan
// records the dynamic register choices and spill pressure at region scope so
// the scratch-allocator refactor can consume a tested contract.
type ie64RegionPlan struct {
	residentGuestRegs []byte
	residentHostRegs  []byte
	spillOps          int
	fpuSpillOps       int
}

var errIE64RegionSpillPressure = fmt.Errorf("ie64CompileRegion: spill pressure exceeds IE64_JIT_REGION_MAX_SPILLS")

var ie64RegionHostRegs = []byte{
	amd64RBX, amd64RBP, amd64R12, amd64R13, amd64R10, amd64R11,
}

// ie64RegionResidentHostRegs are the host registers a promoted region may bind
// to its hottest guest registers (Technique 2). Only the four callee-saved
// GPRs are usable: R14 stays pinned to SP, and R10/R11 are reserved as JIT
// scratch (epilogue ctx pointer, micro-TLB probe) and must never hold guest
// state across an instruction boundary. Ordered hottest-first.
var ie64RegionResidentHostRegs = []byte{amd64RBX, amd64RBP, amd64R12, amd64R13}

// ie64ResidentBinding pins one guest register to a host register for the
// duration of a single region compilation. SP (R31 -> R14) is always resident
// and is not represented here.
type ie64ResidentBinding struct {
	guest byte
	host  byte
}

// ie64ActiveRegionMap, when non-nil, overrides the fixed Tier-1 GPR mapping for
// the region currently being compiled. ie64ToAMD64Reg, the block prologue and
// every register spill site (epilogue, lightweight chain-exit store) consult it
// so the loaded, spilled and in-body mappings stay coherent. IE64 code
// generation is single-threaded, so a package global is sound; ie64CompileRegion
// sets it on entry and clears it on return.
var ie64ActiveRegionMap []ie64ResidentBinding

// ie64BuildRegionRegMap selects the region's resident guest->host bindings from
// the planner's weighted ranking, capped at the four usable callee-saved hosts.
// Returns nil when the planner chose nothing (region falls back to the fixed
// Tier-1 mapping).
func ie64BuildRegionRegMap(plan ie64RegionPlan) []ie64ResidentBinding {
	n := len(plan.residentGuestRegs)
	if n > len(ie64RegionResidentHostRegs) {
		n = len(ie64RegionResidentHostRegs)
	}
	if n == 0 {
		return nil
	}
	bindings := make([]ie64ResidentBinding, 0, n)
	for i := 0; i < n; i++ {
		g := plan.residentGuestRegs[i]
		if g == 0 || g == 31 {
			continue // never remap the zero register or SP
		}
		bindings = append(bindings, ie64ResidentBinding{guest: g, host: ie64RegionResidentHostRegs[i]})
	}
	if len(bindings) == 0 {
		return nil
	}
	return bindings
}

func (p ie64RegionPlan) residentMask() uint32 {
	var mask uint32
	for _, reg := range p.residentGuestRegs {
		if reg < 32 {
			mask |= 1 << reg
		}
	}
	return mask
}

func ie64RegionMaxSpillOps() int {
	raw := strings.TrimSpace(os.Getenv("IE64_JIT_REGION_MAX_SPILLS"))
	if raw == "" {
		return -1
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return -1
	}
	return n
}

func ie64RegionPlanAccepted(plan ie64RegionPlan) bool {
	limit := ie64RegionMaxSpillOps()
	return limit < 0 || plan.spillOps <= limit
}

func ie64PlanRegion(region *ie64Region) ie64RegionPlan {
	if region == nil {
		return ie64RegionPlan{}
	}
	var weights [32]int
	var spillOps int
	for _, block := range region.blocks {
		for i := range block {
			ji := &block[i]
			ie64AccumulateRegionWeights(ji, &weights)
			spillOps += ie64EstimatedSpillOps(ji)
		}
	}

	// R0 is hardwired zero and R31/SP remains pinned to R14 by ABI.
	weights[0] = 0
	weights[31] = 0

	plan := ie64RegionPlan{}
	for len(plan.residentGuestRegs) < len(ie64RegionHostRegs) {
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
		plan.residentHostRegs = append(plan.residentHostRegs, ie64RegionHostRegs[len(plan.residentHostRegs)])
		weights[bestReg] = 0
	}
	plan.spillOps = spillOps
	return plan
}

func ie64AccumulateRegionWeights(ji *JITInstr, weights *[32]int) {
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
