// jit_m68k_exec.go - M68020 JIT execution loop

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

// M68K JIT configuration
const (
	m68kJitExecMemSize      = 512 * 1024 * 1024 // large enough for long-running high-RAM M68K workloads
	m68kJitFallbackBurstMax = 512
	m68kJitWarmupDefault    = 2
)

func m68kJITFallbackBurstMax() int {
	if raw := os.Getenv("IE_M68K_JIT_FALLBACK_BURST"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
	}
	return m68kJitFallbackBurstMax
}

func m68kFallbackBurstUntilInterruptSample(instructionCount uint64) int {
	max := m68kJITFallbackBurstMax()
	untilSample := 256 - int(instructionCount&0xFF)
	if untilSample <= 0 || untilSample > 256 {
		untilSample = 256
	}
	if max > untilSample {
		return untilSample
	}
	return max
}

func m68kJITDisableChains() bool {
	return os.Getenv("IE_M68K_JIT_DISABLE_CHAINS") == "1"
}

func m68kJITDisableRTSCache() bool {
	if os.Getenv("IE_M68K_JIT_DISABLE_RTS_CACHE") == "1" {
		return true
	}
	return os.Getenv("IE_M68K_JIT_ENABLE_RTS_CACHE") != "1"
}

// m68kJITStrictMode reports whether the JIT must fail loudly on a compiler
// error instead of falling back to one interpreter instruction. Used to catch
// emitter bugs during development (M68K_JIT_FALLBACK_REMOVAL_PLAN.md).
func m68kJITStrictMode() bool {
	return os.Getenv("IE_M68K_JIT_STRICT") == "1"
}

// m68kJITDiagBurstFallback re-enables the legacy multi-instruction interpreter
// burst for the production blocked/compile-failure paths. Default off: normal
// production fallback executes exactly one instruction and returns to the JIT
// dispatcher. This switch exists only for diagnostics/A-B comparison; no normal
// run should rely on it.
func m68kJITDiagBurstFallback() bool {
	return os.Getenv("IE_M68K_JIT_DIAG_BURST_FALLBACK") == "1"
}

func m68kJITDisableRegions() bool {
	return os.Getenv("IE_M68K_JIT_DISABLE_REGIONS") == "1"
}

func m68kJITDisableStaticJMPChase() bool {
	return os.Getenv("IE_M68K_JIT_DISABLE_STATIC_JMP_CHASE") == "1"
}

func m68kJITInterruptSampleInterval() uint32 {
	if raw := os.Getenv("IE_M68K_JIT_IRQ_SAMPLE_INTERVAL"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			if n < 1 {
				return 1
			}
			if n > 256 {
				return 256
			}
			return uint32(n)
		}
	}
	return 256
}

func m68kJITTraceExtensionPC() bool {
	return os.Getenv("IE_M68K_JIT_TRACE_EXTENSION_PC") == "1"
}

func m68kJITCompileWarmupLimit() uint8 {
	if raw := os.Getenv("IE_M68K_JIT_COMPILE_WARMUP"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			switch {
			case n <= 1:
				return 1
			case n > 255:
				return 255
			default:
				return uint8(n)
			}
		}
	}
	return m68kJitWarmupDefault
}

func m68kJITDiagnosticDisabledPCs() map[uint32]struct{} {
	raw := os.Getenv("IE_M68K_JIT_DISABLE_PC")
	if raw == "" {
		return nil
	}
	disabled := make(map[uint32]struct{})
	for _, field := range strings.Split(raw, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		value, err := strconv.ParseUint(field, 0, 32)
		if err != nil {
			continue
		}
		disabled[uint32(value)] = struct{}{}
	}
	return disabled
}

type m68kJITDiagnosticPCRange struct {
	lo uint32
	hi uint32
}

func m68kJITDiagnosticDisabledPCRanges() []m68kJITDiagnosticPCRange {
	raw := os.Getenv("IE_M68K_JIT_DISABLE_PC_RANGE")
	if raw == "" {
		return nil
	}
	var ranges []m68kJITDiagnosticPCRange
	for _, field := range strings.Split(raw, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		parts := strings.Split(field, "-")
		if len(parts) != 2 {
			continue
		}
		lo, errLo := strconv.ParseUint(strings.TrimSpace(parts[0]), 0, 32)
		hi, errHi := strconv.ParseUint(strings.TrimSpace(parts[1]), 0, 32)
		if errLo != nil || errHi != nil {
			continue
		}
		if hi < lo {
			lo, hi = hi, lo
		}
		ranges = append(ranges, m68kJITDiagnosticPCRange{lo: uint32(lo), hi: uint32(hi)})
	}
	return ranges
}

func m68kJITDiagnosticPCInRanges(pc uint32, ranges []m68kJITDiagnosticPCRange) bool {
	for _, r := range ranges {
		if pc >= r.lo && pc <= r.hi {
			return true
		}
	}
	return false
}

func m68kJITDiagnosticInstrsTouchRanges(startPC uint32, instrs []M68KJITInstr, ranges []m68kJITDiagnosticPCRange) bool {
	if len(ranges) == 0 {
		return false
	}
	for i := range instrs {
		if m68kJITDiagnosticPCInRanges(startPC+uint32(instrs[i].pcOffset), ranges) {
			return true
		}
	}
	return false
}

func (cpu *M68KCPU) m68kJITTraceSuspiciousExtensionPC(pc uint32, stage string) bool {
	if cpu == nil || !m68kJITTraceExtensionPC() || pc < 2 || int(pc+1) >= len(cpu.memory) {
		return false
	}
	prev := uint16(cpu.memory[pc-2])<<8 | uint16(cpu.memory[pc-1])
	if prev&0xFFC0 != 0x4E80 && prev&0xFFC0 != 0x4EC0 {
		return false
	}
	for back := uint32(4); back <= 10 && pc >= back; back += 2 {
		if l := m68kInstrLength(cpu.memory, pc-back); l == int(back) {
			return false
		}
	}
	mode := (prev >> 3) & 7
	reg := prev & 7
	if mode == 2 || mode == 3 || mode == 4 || (mode == 7 && reg > 3) {
		return false
	}
	op := uint16(cpu.memory[pc])<<8 | uint16(cpu.memory[pc+1])
	prev2 := uint16(0)
	if pc >= 4 {
		prev2 = uint16(cpu.memory[pc-4])<<8 | uint16(cpu.memory[pc-3])
	}
	fmt.Printf("M68K JIT suspicious extension PC stage=%s pc=%08X op=%04X prev=%04X prev2=%04X A6=%08X A7=%08X D0=%08X D1=%08X stack=%04X %04X %04X %04X RTS0=%08X RTS1=%08X\n",
		stage, pc, op, prev, prev2, cpu.AddrRegs[6], cpu.AddrRegs[7], cpu.DataRegs[0], cpu.DataRegs[1],
		cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]),
		cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+2),
		cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+4),
		cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+6),
		uint32(cpu.m68kJitCtx.RTSCache0PC), uint32(cpu.m68kJitCtx.RTSCache1PC))
	return true
}

func m68kJITDiagnosticTracePCs() map[uint32]struct{} {
	raw := os.Getenv("IE_M68K_JIT_TRACE_PC")
	if raw == "" {
		return nil
	}
	traced := make(map[uint32]struct{})
	for _, field := range strings.Split(raw, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		value, err := strconv.ParseUint(field, 0, 32)
		if err != nil {
			continue
		}
		traced[uint32(value)] = struct{}{}
	}
	return traced
}

func m68kJITDiagnosticTraceRetRanges() []m68kJITDiagnosticPCRange {
	raw := os.Getenv("IE_M68K_JIT_TRACE_RET_RANGE")
	if raw == "" {
		return nil
	}
	var ranges []m68kJITDiagnosticPCRange
	for _, field := range strings.Split(raw, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		parts := strings.Split(field, "-")
		if len(parts) != 2 {
			continue
		}
		lo, errLo := strconv.ParseUint(strings.TrimSpace(parts[0]), 0, 32)
		hi, errHi := strconv.ParseUint(strings.TrimSpace(parts[1]), 0, 32)
		if errLo != nil || errHi != nil {
			continue
		}
		if hi < lo {
			lo, hi = hi, lo
		}
		ranges = append(ranges, m68kJITDiagnosticPCRange{lo: uint32(lo), hi: uint32(hi)})
	}
	return ranges
}

func m68kJITDiagnosticStopRetRanges() []m68kJITDiagnosticPCRange {
	raw := os.Getenv("IE_M68K_JIT_STOP_RET_RANGE")
	if raw == "" {
		return nil
	}
	var ranges []m68kJITDiagnosticPCRange
	for _, field := range strings.Split(raw, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		parts := strings.Split(field, "-")
		if len(parts) != 2 {
			continue
		}
		lo, errLo := strconv.ParseUint(strings.TrimSpace(parts[0]), 0, 32)
		hi, errHi := strconv.ParseUint(strings.TrimSpace(parts[1]), 0, 32)
		if errLo != nil || errHi != nil {
			continue
		}
		if hi < lo {
			lo, hi = hi, lo
		}
		ranges = append(ranges, m68kJITDiagnosticPCRange{lo: uint32(lo), hi: uint32(hi)})
	}
	return ranges
}

func m68kJITDiagnosticStopPCRanges() []m68kJITDiagnosticPCRange {
	raw := os.Getenv("IE_M68K_JIT_STOP_PC_RANGE")
	if raw == "" {
		return nil
	}
	var ranges []m68kJITDiagnosticPCRange
	for _, field := range strings.Split(raw, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		parts := strings.Split(field, "-")
		if len(parts) != 2 {
			continue
		}
		lo, errLo := strconv.ParseUint(strings.TrimSpace(parts[0]), 0, 32)
		hi, errHi := strconv.ParseUint(strings.TrimSpace(parts[1]), 0, 32)
		if errLo != nil || errHi != nil {
			continue
		}
		if hi < lo {
			lo, hi = hi, lo
		}
		ranges = append(ranges, m68kJITDiagnosticPCRange{lo: uint32(lo), hi: uint32(hi)})
	}
	return ranges
}

func m68kJITDiagnosticStopA6Value() (uint32, bool) {
	raw := strings.TrimSpace(os.Getenv("IE_M68K_JIT_STOP_ON_A6_VALUE"))
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseUint(raw, 0, 32)
	if err != nil {
		return 0, false
	}
	return uint32(value), true
}

func m68kJITDiagnosticTraceLowPC() bool {
	return os.Getenv("IE_M68K_JIT_TRACE_LOW_PC") == "1"
}

func (cpu *M68KCPU) m68kReportJITStopPCRange(pc uint32, block *JITBlock) {
	if cpu == nil {
		return
	}
	blockPC := uint32(0)
	if block != nil && block.startPC <= uint64(^uint32(0)) {
		blockPC = uint32(block.startPC)
	}
	fmt.Printf("M68K JIT stop pc-range pc=%08X op=%04X sr=%04X last_native=%08X block=%08X A0=%08X A1=%08X A2=%08X A3=%08X A4=%08X A5=%08X A6=%08X A7=%08X D0=%08X D1=%08X D2=%08X D3=%08X D4=%08X D5=%08X D6=%08X D7=%08X stack=%04X %04X %04X %04X\n",
		pc, cpu.m68kJITDiagnosticWord(pc), cpu.SR, cpu.m68kJitLastNativePC.Load(), blockPC,
		cpu.AddrRegs[0], cpu.AddrRegs[1], cpu.AddrRegs[2], cpu.AddrRegs[3],
		cpu.AddrRegs[4], cpu.AddrRegs[5], cpu.AddrRegs[6], cpu.AddrRegs[7],
		cpu.DataRegs[0], cpu.DataRegs[1], cpu.DataRegs[2], cpu.DataRegs[3],
		cpu.DataRegs[4], cpu.DataRegs[5], cpu.DataRegs[6], cpu.DataRegs[7],
		cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]),
		cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+2),
		cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+4),
		cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+6))
	readMem := func(addr uint64, size int) []byte {
		if addr+uint64(size) > uint64(len(cpu.memory)) {
			return nil
		}
		return cpu.memory[addr : addr+uint64(size)]
	}
	for _, line := range disassembleM68K(readMem, uint64(pc), 16) {
		fmt.Printf("  %08X: %-16s %s\n", line.Address, line.HexBytes, line.Mnemonic)
	}
	cpu.m68kDumpNativePCRing()
}

func (cpu *M68KCPU) m68kReportJITStopRetRange(block *JITBlock, ctx *M68KJITContext) {
	if cpu == nil || ctx == nil {
		return
	}
	blockPC := uint32(0)
	if block != nil && block.startPC <= uint64(^uint32(0)) {
		blockPC = uint32(block.startPC)
	}
	blockEnd := uint32(0)
	if block != nil && block.endPC <= uint64(^uint32(0)) {
		blockEnd = uint32(block.endPC)
	}
	fmt.Printf("M68K JIT stop ret-range block=%08X end=%08X ret=%08X op=%04X sr=%04X chain=%d retCount=%d NeedIO=%d Helper=%d Exception=%d Inval=%d A0=%08X A1=%08X A2=%08X A3=%08X A4=%08X A5=%08X A6=%08X A7=%08X D0=%08X D1=%08X D2=%08X D3=%08X D4=%08X D5=%08X D6=%08X D7=%08X stack=%04X %04X %04X %04X\n",
		blockPC, blockEnd, ctx.RetPC, cpu.m68kJITDiagnosticWord(ctx.RetPC), cpu.SR,
		ctx.ChainCount, ctx.RetCount, ctx.NeedIOFallback, ctx.NeedHelper, ctx.NativeException, ctx.NeedInval,
		cpu.AddrRegs[0], cpu.AddrRegs[1], cpu.AddrRegs[2], cpu.AddrRegs[3],
		cpu.AddrRegs[4], cpu.AddrRegs[5], cpu.AddrRegs[6], cpu.AddrRegs[7],
		cpu.DataRegs[0], cpu.DataRegs[1], cpu.DataRegs[2], cpu.DataRegs[3],
		cpu.DataRegs[4], cpu.DataRegs[5], cpu.DataRegs[6], cpu.DataRegs[7],
		cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]),
		cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+2),
		cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+4),
		cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+6))
	readMem := func(addr uint64, size int) []byte {
		if addr+uint64(size) > uint64(len(cpu.memory)) {
			return nil
		}
		return cpu.memory[addr : addr+uint64(size)]
	}
	if block != nil {
		fmt.Printf("  --- returning block disasm pc=%08X ---\n", blockPC)
		for _, line := range disassembleM68K(readMem, block.startPC, 24) {
			fmt.Printf("  %08X: %-16s %s\n", line.Address, line.HexBytes, line.Mnemonic)
		}
	}
	fmt.Printf("  --- ret pc disasm pc=%08X ---\n", ctx.RetPC)
	for _, line := range disassembleM68K(readMem, uint64(ctx.RetPC), 16) {
		fmt.Printf("  %08X: %-16s %s\n", line.Address, line.HexBytes, line.Mnemonic)
	}
	cpu.m68kDumpNativePCRing()
}

// m68kTierController is the shared Phase 3 promotion controller bound
// to M68K's reference RegPressureProfile. Cache-hit gate in the exec
// loop delegates to ShouldPromote so threshold tweaks apply uniformly.
//
// B.1.b uses this controller to drive region promotion only — no
// per-block Tier-2 reg-map promotion lands until B.1.c. The
// PromoteBlock allocator stub stays a no-op for now (see
// jit_tier_backends.go).
var m68kTierController = NewTierController(M68KRegProfile)

// m68kGetJITExecMem returns the typed *ExecMem from the cpu's any field.
func (cpu *M68KCPU) m68kGetJITExecMem() *ExecMem {
	if cpu.m68kJitExecMem == nil {
		return nil
	}
	return cpu.m68kJitExecMem.(*ExecMem)
}

// initM68KJIT initializes JIT state. Called once before execution.
func (cpu *M68KCPU) initM68KJIT() error {
	if cpu.m68kJitExecMem != nil {
		return nil // already initialized
	}
	execMem, err := AllocExecMem(m68kJitExecMemSize)
	if err != nil {
		return fmt.Errorf("M68K JIT init failed: %w", err)
	}
	cpu.m68kJitExecMem = execMem
	cpu.m68kJitCache = NewCodeCache()
	cpu.m68kJitWarmupCounts = make(map[uint32]uint8, 4096)
	if cpu.m68kJitWarmupLimit == 0 {
		cpu.m68kJitWarmupLimit = m68kJITCompileWarmupLimit()
	}
	cpu.m68kJitIOPageBitmap = nil
	if bus, ok := cpu.bus.(*MachineBus); ok && len(bus.ioPageBitmap) > 0 {
		cpu.m68kJitIOPageBitmap = append([]bool(nil), bus.ioPageBitmap...)
		if cpu.AmigaINTENA != nil {
			page := uint32(0xDFF09A >> 8)
			if page < uint32(len(cpu.m68kJitIOPageBitmap)) {
				cpu.m68kJitIOPageBitmap[page] = true
			}
		}
		if bus.videoStatusReader != nil {
			page := uint32(0xF0008 >> 8)
			if page < uint32(len(cpu.m68kJitIOPageBitmap)) {
				cpu.m68kJitIOPageBitmap[page] = true
			}
		}
	}
	pageCount := (uint32(len(cpu.memory)) + 4095) >> 12
	cpu.m68kJitCodeBitmap = make([]byte, pageCount)
	cpu.m68kJitCodePageMin = make([]uint16, pageCount)
	cpu.m68kJitCodePageMax = make([]uint16, pageCount)
	cpu.m68kJitCodePageBlocks = make([]map[*JITBlock]struct{}, pageCount)
	for i := range cpu.m68kJitCodePageMin {
		cpu.m68kJitCodePageMin[i] = 0xFFFF
	}
	cpu.m68kJitCtx = newM68KJITContext(cpu, cpu.m68kJitCodeBitmap, cpu.m68kJitCodePageMin, cpu.m68kJitCodePageMax)
	cpu.m68kJitNativeActive.Store(false)
	cpu.m68kJitDeferredInval.Store(false)
	return nil
}

// freeM68KJIT releases all JIT resources. If m68kJitPersist is set,
// the code cache and exec memory are kept alive for reuse across benchmark runs.
func (cpu *M68KCPU) freeM68KJIT() {
	if cpu.m68kJitPersist {
		return
	}
	if em := cpu.m68kGetJITExecMem(); em != nil {
		em.Free()
		cpu.m68kJitExecMem = nil
	}
	cpu.m68kJitCache = nil
	cpu.m68kJitCtx = nil
	cpu.m68kJitIOPageBitmap = nil
	cpu.m68kJitCodeBitmap = nil
	cpu.m68kJitCodePageMin = nil
	cpu.m68kJitCodePageMax = nil
	cpu.m68kJitCodePageBlocks = nil
	cpu.m68kJitNativeActive.Store(false)
	cpu.m68kJitDeferredInval.Store(false)
}

func (cpu *M68KCPU) m68kClearJITRTSCache() {
	if cpu == nil || cpu.m68kJitCtx == nil {
		return
	}
	ctx := cpu.m68kJitCtx
	ctx.RTSCache0PC = 0
	ctx.RTSCache0Addr = 0
	ctx.RTSCache1PC = 0
	ctx.RTSCache1Addr = 0
	ctx.RTSCache2PC = 0
	ctx.RTSCache2Addr = 0
	ctx.RTSCache3PC = 0
	ctx.RTSCache3Addr = 0
	ctx.RTSCache4PC = 0
	ctx.RTSCache4Addr = 0
	ctx.RTSCache5PC = 0
	ctx.RTSCache5Addr = 0
	ctx.RTSCache6PC = 0
	ctx.RTSCache6Addr = 0
	ctx.RTSCache7PC = 0
	ctx.RTSCache7Addr = 0
}

func (cpu *M68KCPU) m68kResetJITCodeCache() {
	if cpu == nil {
		return
	}
	if cpu.m68kJitCache != nil {
		cpu.m68kJitCache.Invalidate()
	}
	if execMem := cpu.m68kGetJITExecMem(); execMem != nil {
		execMem.Reset()
	}
	if cpu.m68kJitCodeBitmap != nil {
		clear(cpu.m68kJitCodeBitmap)
	}
	if cpu.m68kJitCodePageMin != nil {
		for i := range cpu.m68kJitCodePageMin {
			cpu.m68kJitCodePageMin[i] = 0xFFFF
		}
	}
	if cpu.m68kJitCodePageMax != nil {
		clear(cpu.m68kJitCodePageMax)
	}
	if cpu.m68kJitCodePageBlocks != nil {
		clear(cpu.m68kJitCodePageBlocks)
	}
	cpu.m68kClearJITRTSCache()
}

func (cpu *M68KCPU) m68kRebuildJITCodeMetadata() {
	if cpu == nil {
		return
	}
	if cpu.m68kJitCodeBitmap != nil {
		clear(cpu.m68kJitCodeBitmap)
	}
	if cpu.m68kJitCodePageMin != nil {
		for i := range cpu.m68kJitCodePageMin {
			cpu.m68kJitCodePageMin[i] = 0xFFFF
		}
	}
	if cpu.m68kJitCodePageMax != nil {
		clear(cpu.m68kJitCodePageMax)
	}
	if cpu.m68kJitCodePageBlocks != nil {
		clear(cpu.m68kJitCodePageBlocks)
	}
	if cpu.m68kJitCache == nil {
		return
	}
	for _, block := range cpu.m68kJitCache.blocks {
		cpu.m68kMarkJITCodeRanges(block)
	}
}

func (cpu *M68KCPU) m68kInvalidateJITCodeRange(lo, hi uint32) {
	if cpu == nil {
		return
	}
	if hi <= lo {
		cpu.m68kResetJITCodeCache()
		return
	}
	removed := cpu.m68kInvalidateJITCodeRangeIndexed(uint64(lo), uint64(hi))
	if removed != 0 {
		cpu.m68kClearJITRTSCache()
		cpu.m68kRebuildJITCodeMetadata()
		return
	}
}

func (cpu *M68KCPU) m68kInvalidateJITCodeRangeIndexed(lo, hi uint64) int {
	if cpu == nil || cpu.m68kJitCache == nil || hi <= lo {
		return 0
	}
	cpu.m68kJitCache.UnpatchChainsInRange(lo, hi)
	if cpu.m68kJitCodePageBlocks == nil || len(cpu.m68kJitCodePageBlocks) == 0 {
		return cpu.m68kJitCache.InvalidateRange(lo, hi)
	}
	startPage := lo >> 12
	endPage := (hi - 1) >> 12
	if startPage >= uint64(len(cpu.m68kJitCodePageBlocks)) {
		return 0
	}
	if endPage >= uint64(len(cpu.m68kJitCodePageBlocks)) {
		endPage = uint64(len(cpu.m68kJitCodePageBlocks) - 1)
	}
	candidates := make(map[*JITBlock]struct{}, 8)
	for page := startPage; page <= endPage; page++ {
		for block := range cpu.m68kJitCodePageBlocks[page] {
			candidates[block] = struct{}{}
		}
	}
	if len(candidates) == 0 {
		return cpu.m68kJitCache.InvalidateRange(lo, hi)
	}
	removed := 0
	for block := range candidates {
		for _, r := range JITBlockCoveredRanges(block) {
			if r[1] > lo && r[0] < hi {
				if cpu.m68kJitCache.RemoveBlock(block) {
					removed++
				}
				break
			}
		}
	}
	return removed
}

func (cpu *M68KCPU) m68kInvalidateJITCodeCacheSafely() {
	if cpu == nil {
		return
	}
	if cpu.m68kJitNativeActive.Load() {
		cpu.m68kJitDeferredInval.Store(true)
		return
	}
	cpu.m68kResetJITCodeCache()
}

func (cpu *M68KCPU) m68kApplyDeferredJITInvalidation() bool {
	if cpu == nil || cpu.m68kJitNativeActive.Load() || !cpu.m68kJitDeferredInval.Swap(false) {
		return false
	}
	cpu.m68kResetJITCodeCache()
	return true
}

// m68kPendingInvalMaxRanges bounds the cross-thread invalidation queue. If a
// host goroutine floods writes faster than the CPU thread drains, the queue
// collapses to a single full-cache reset instead of growing unbounded.
const m68kPendingInvalMaxRanges = 64

// m68kEnqueueJITInvalidation records a guest-write range to invalidate, to be
// applied by the CPU/dispatcher thread. SAFE TO CALL FROM ANY GOROUTINE: it
// only touches the pending-range list under m68kJitPendingInvalMu and never the
// JIT cache maps or code bitmap, which are owned by the CPU thread. This is the
// serialization point that closes the host-vs-CPU data race on the cache.
func (cpu *M68KCPU) m68kEnqueueJITInvalidation(addr, size uint32) {
	if cpu == nil || size == 0 {
		return
	}
	end := uint64(addr) + uint64(size)
	if end > uint64(^uint32(0)) {
		end = uint64(^uint32(0))
	}
	hi := uint32(end)
	cpu.m68kJitPendingInvalMu.Lock()
	if !cpu.m68kJitPendingInvalReset {
		if n := len(cpu.m68kJitPendingInvalRanges); n > 0 {
			// Coalesce with the last range when adjacent/overlapping (handles
			// the common sequential byte-write loaders without list growth).
			last := &cpu.m68kJitPendingInvalRanges[n-1]
			if addr <= last[1] && hi >= last[0] {
				if addr < last[0] {
					last[0] = addr
				}
				if hi > last[1] {
					last[1] = hi
				}
				cpu.m68kJitPendingInvalMu.Unlock()
				cpu.m68kJitHasPendingInval.Store(true)
				// Publish AFTER the range + flag so a dispatcher observing the
				// new generation also observes the queued work.
				cpu.m68kJitInvalGen.Add(1)
				return
			}
		}
		if len(cpu.m68kJitPendingInvalRanges) >= m68kPendingInvalMaxRanges {
			cpu.m68kJitPendingInvalReset = true
			cpu.m68kJitPendingInvalRanges = cpu.m68kJitPendingInvalRanges[:0]
		} else {
			cpu.m68kJitPendingInvalRanges = append(cpu.m68kJitPendingInvalRanges, [2]uint32{addr, hi})
		}
	}
	cpu.m68kJitPendingInvalMu.Unlock()
	cpu.m68kJitHasPendingInval.Store(true)
	// Publish AFTER the range + flag so a dispatcher observing the new
	// generation also observes the queued work.
	cpu.m68kJitInvalGen.Add(1)
}

// m68kDrainPendingJITInvalidations applies queued cross-thread invalidations.
// MUST be called only from the CPU/dispatcher goroutine (it mutates the cache).
func (cpu *M68KCPU) m68kDrainPendingJITInvalidations() {
	if cpu == nil || !cpu.m68kJitHasPendingInval.Load() {
		return
	}
	cpu.m68kJitPendingInvalMu.Lock()
	reset := cpu.m68kJitPendingInvalReset
	ranges := cpu.m68kJitPendingInvalRanges
	cpu.m68kJitPendingInvalRanges = nil
	cpu.m68kJitPendingInvalReset = false
	cpu.m68kJitHasPendingInval.Store(false)
	cpu.m68kJitPendingInvalMu.Unlock()
	if reset {
		cpu.m68kResetJITCodeCache()
		return
	}
	for _, r := range ranges {
		cpu.invalidateM68KJITForGuestWrite(r[0], r[1]-r[0])
	}
}

// m68kVerifyCaptureWrite logs old RAM bytes for the verifier's interpreter
// pre-pass so it can be undone. Returns true (abort) if the target is MMIO /
// VGA / terminal — those side-effects cannot be undone, and a block touching
// them would self-bail natively, so it is not a verify candidate.
func (cpu *M68KCPU) m68kVerifyCaptureWrite(addr uint32, size int) bool {
	if addr >= cpu.ProfileTopOfRAM() || (addr >= 0xA0000 && addr < 0xC0000) ||
		addr == TERM_OUT || addr == TERM_OUT_SIGNEXT {
		cpu.m68kVerifyAbort = true
		return true
	}
	for i := 0; i < size; i++ {
		a := addr + uint32(i)
		if a < uint32(len(cpu.memory)) {
			cpu.m68kVerifyWrites = append(cpu.m68kVerifyWrites, m68kVerifyWrite{addr: a, old: cpu.memory[a]})
		}
	}
	return false
}

// m68kVerifyInterpPrePass runs the interpreter forward from startPC, recording
// architectural state at the ENTRY of each instruction (first time each PC is
// seen), then undoes every RAM write and restores all CPU state. The recorded
// trace lets the caller compare native's result against the interpreter state at
// native's actual exit PC — robust to where native stops (chain budget, inline
// Bcc chain-exit, block end). Returns aborted=true if the block touched
// MMIO/terminal (not a verify candidate) or hit an exception.
func (cpu *M68KCPU) m68kVerifyInterpPrePass(startPC, endPC uint32) bool {
	sD := cpu.DataRegs
	sA := cpu.AddrRegs
	sSR := cpu.SR
	sPC := cpu.PC
	sInstr := cpu.InstructionCount
	sPendEx := cpu.pendingException.Load()

	cpu.m68kVerifyWrites = cpu.m68kVerifyWrites[:0]
	cpu.m68kVerifyTrace = cpu.m68kVerifyTrace[:0]
	cpu.m68kVerifyAbort = false
	cpu.m68kVerifyCapturing = true

	// Run a generous window: until control jumps far outside [startPC, endPC+4K)
	// (backward loop or chain target), or step/trace caps are hit.
	lo := startPC
	hi := endPC + 0x1000
	cpu.PC = startPC
	steps := 0
	for steps < 4096 && len(cpu.m68kVerifyTrace) < 512 {
		pc := cpu.PC
		if pc < lo || pc >= hi || pc&1 != 0 {
			break
		}
		cpu.m68kVerifyTrace = append(cpu.m68kVerifyTrace, m68kVerifyState{
			pc: pc, d: cpu.DataRegs, a: cpu.AddrRegs, sr: cpu.SR,
		})
		cpu.StepOne()
		steps++
		if cpu.m68kVerifyAbort || cpu.pendingException.Load() != 0 {
			cpu.m68kVerifyAbort = true
			break
		}
	}
	// Record the final exit state too (PC after the last step).
	if !cpu.m68kVerifyAbort {
		cpu.m68kVerifyTrace = append(cpu.m68kVerifyTrace, m68kVerifyState{
			pc: cpu.PC, d: cpu.DataRegs, a: cpu.AddrRegs, sr: cpu.SR,
		})
	}
	aborted := cpu.m68kVerifyAbort
	cpu.m68kVerifyCapturing = false

	for i := len(cpu.m68kVerifyWrites) - 1; i >= 0; i-- {
		w := cpu.m68kVerifyWrites[i]
		cpu.memory[w.addr] = w.old
	}
	cpu.m68kVerifyWrites = cpu.m68kVerifyWrites[:0]
	cpu.DataRegs = sD
	cpu.AddrRegs = sA
	cpu.SR = sSR
	cpu.PC = sPC
	cpu.InstructionCount = sInstr
	cpu.pendingException.Store(sPendEx)
	return aborted
}

// m68kVerifyLookup returns the interpreter pre-pass state after `idx` retired
// instructions (trace[idx]), or nil if out of range.
func (cpu *M68KCPU) m68kVerifyLookup(idx int) *m68kVerifyState {
	if idx < 0 || idx >= len(cpu.m68kVerifyTrace) {
		return nil
	}
	return &cpu.m68kVerifyTrace[idx]
}

// m68kReportVerifyDivergence prints the block whose native execution disagreed
// with the interpreter, with disassembly and the first differing register —
// pinpointing the buggy emitter.
func (cpu *M68KCPU) m68kReportVerifyDivergence(startPC, endPC uint32, exp *m68kVerifyState) {
	fmt.Printf("M68K JIT VERIFY DIVERGENCE block=%08X-%08X exitPC=%08X\n", startPC, endPC, cpu.PC)
	fmt.Printf("  native SR=%04X interp SR=%04X\n", cpu.SR&0x1F, exp.sr&0x1F)
	for i := 0; i < 8; i++ {
		if cpu.DataRegs[i] != exp.d[i] {
			fmt.Printf("  D%d native=%08X interp=%08X\n", i, cpu.DataRegs[i], exp.d[i])
		}
	}
	for i := 0; i < 8; i++ {
		if cpu.AddrRegs[i] != exp.a[i] {
			fmt.Printf("  A%d native=%08X interp=%08X\n", i, cpu.AddrRegs[i], exp.a[i])
		}
	}
	readMem := func(addr uint64, size int) []byte {
		if addr+uint64(size) > uint64(len(cpu.memory)) {
			return nil
		}
		return cpu.memory[addr : addr+uint64(size)]
	}
	n := int((endPC - startPC) / 2)
	if n > 24 {
		n = 24
	}
	if n < 1 {
		n = 1
	}
	for _, line := range disassembleM68K(readMem, uint64(startPC), n) {
		fmt.Printf("  %08X: %-18s %s\n", line.Address, line.HexBytes, line.Mnemonic)
	}
}

func (cpu *M68KCPU) invalidateM68KJITForGuestWrite(addr uint32, size uint32) {
	// During the verifier's interpreter pre-pass, writes are temporary and will
	// be undone; suppress cache invalidation so the block we are about to run
	// natively is not evicted out from under us.
	if cpu.m68kVerifyCapturing {
		return
	}
	if cpu == nil || cpu.m68kJitCache == nil || cpu.m68kJitCodeBitmap == nil || size == 0 {
		return
	}
	if len(cpu.m68kJitCodeBitmap) == 0 {
		return
	}
	startPage := addr >> 12
	end := uint64(addr) + uint64(size) - 1
	endPage := uint32(end >> 12)
	if uint64(endPage) >= uint64(len(cpu.m68kJitCodeBitmap)) {
		endPage = uint32(len(cpu.m68kJitCodeBitmap) - 1)
	}
	for page := startPage; page <= endPage; page++ {
		if cpu.m68kJitCodeBitmap[page] == 0 {
			continue
		}
		if !cpu.m68kGuestWriteOverlapsJITCode(addr, size) && !cpu.m68kJITCacheOverlapsGuestRange(uint64(addr), uint64(addr)+uint64(size)) {
			continue
		}
		if cpu.m68kJitNativeActive.Load() {
			cpu.m68kJitDeferredInval.Store(true)
		} else {
			end := uint64(addr) + uint64(size)
			if end > uint64(^uint32(0)) {
				end = uint64(^uint32(0))
			}
			cpu.m68kInvalidateJITCodeRange(addr, uint32(end))
		}
		return
	}
}

func (cpu *M68KCPU) m68kJITCacheOverlapsGuestRange(lo, hi uint64) bool {
	if cpu == nil || cpu.m68kJitCache == nil || hi <= lo {
		return false
	}
	for _, block := range cpu.m68kJitCache.blocks {
		for _, r := range JITBlockCoveredRanges(block) {
			if r[1] > lo && r[0] < hi {
				return true
			}
		}
	}
	for _, block := range cpu.m68kJitCache.mmuBlocks {
		for _, r := range JITBlockCoveredRanges(block) {
			if r[1] > lo && r[0] < hi {
				return true
			}
		}
	}
	return false
}

func (cpu *M68KCPU) m68kGuestWriteOverlapsJITCode(addr uint32, size uint32) bool {
	if cpu == nil || size == 0 {
		return false
	}
	if cpu.m68kJitCodePageMin == nil || cpu.m68kJitCodePageMax == nil {
		return true
	}
	end := uint64(addr) + uint64(size)
	if end == 0 {
		return false
	}
	startPage := uint64(addr >> 12)
	endPage := (end - 1) >> 12
	if endPage >= uint64(len(cpu.m68kJitCodePageMin)) {
		endPage = uint64(len(cpu.m68kJitCodePageMin) - 1)
	}
	for page := startPage; page <= endPage; page++ {
		minOff := cpu.m68kJitCodePageMin[page]
		if minOff == 0xFFFF {
			continue
		}
		maxOff := cpu.m68kJitCodePageMax[page]
		writeStart := uint16(0)
		if page == startPage {
			writeStart = uint16(addr & 0xFFF)
		}
		writeEnd := uint16(0x1000)
		if page == endPage {
			writeEnd = uint16(((end - 1) & 0xFFF) + 1)
		}
		if writeEnd > minOff && writeStart < maxOff {
			return true
		}
	}
	return false
}

func (cpu *M68KCPU) m68kRebuildJITCodeMetadataRange(lo, hi uint64) {
	if cpu == nil || hi <= lo {
		return
	}
	startPage := lo >> 12
	endPage := (hi - 1) >> 12
	if cpu.m68kJitCodeBitmap != nil {
		if startPage >= uint64(len(cpu.m68kJitCodeBitmap)) {
			return
		}
		if endPage >= uint64(len(cpu.m68kJitCodeBitmap)) {
			endPage = uint64(len(cpu.m68kJitCodeBitmap) - 1)
		}
		for page := startPage; page <= endPage; page++ {
			cpu.m68kJitCodeBitmap[page] = 0
		}
	}
	if cpu.m68kJitCodePageBlocks != nil {
		if startPage >= uint64(len(cpu.m68kJitCodePageBlocks)) {
			return
		}
		idxEndPage := endPage
		if idxEndPage >= uint64(len(cpu.m68kJitCodePageBlocks)) {
			idxEndPage = uint64(len(cpu.m68kJitCodePageBlocks) - 1)
		}
		for page := startPage; page <= idxEndPage; page++ {
			cpu.m68kJitCodePageBlocks[page] = nil
		}
	}
	if cpu.m68kJitCodePageMin != nil && cpu.m68kJitCodePageMax != nil {
		if startPage >= uint64(len(cpu.m68kJitCodePageMin)) {
			return
		}
		if endPage >= uint64(len(cpu.m68kJitCodePageMin)) {
			endPage = uint64(len(cpu.m68kJitCodePageMin) - 1)
		}
		for page := startPage; page <= endPage; page++ {
			cpu.m68kJitCodePageMin[page] = 0xFFFF
			cpu.m68kJitCodePageMax[page] = 0
		}
	}
	pageLo := startPage << 12
	pageHi := (endPage + 1) << 12
	if cpu.m68kJitCache == nil {
		return
	}
	markOverlappingBlock := func(block *JITBlock) {
		for _, r := range JITBlockCoveredRanges(block) {
			if r[1] <= pageLo || r[0] >= pageHi {
				continue
			}
			cpu.m68kMarkJITCodeRanges(block)
			return
		}
	}
	for _, block := range cpu.m68kJitCache.blocks {
		markOverlappingBlock(block)
	}
	for _, block := range cpu.m68kJitCache.mmuBlocks {
		markOverlappingBlock(block)
	}
}

func (cpu *M68KCPU) m68kMarkJITCodeRanges(block *JITBlock) {
	if cpu == nil || block == nil {
		return
	}
	for _, r := range JITBlockCoveredRanges(block) {
		if r[1] <= r[0] {
			continue
		}
		if cpu.m68kJitCodeBitmap != nil {
			startPage := r[0] >> 12
			endPage := (r[1] - 1) >> 12
			for p := startPage; p <= endPage; p++ {
				if p < uint64(len(cpu.m68kJitCodeBitmap)) {
					cpu.m68kJitCodeBitmap[p] = 1
				}
			}
		}
		if cpu.m68kJitCodePageBlocks != nil {
			startPage := r[0] >> 12
			endPage := (r[1] - 1) >> 12
			for p := startPage; p <= endPage; p++ {
				if p < uint64(len(cpu.m68kJitCodePageBlocks)) {
					if cpu.m68kJitCodePageBlocks[p] == nil {
						cpu.m68kJitCodePageBlocks[p] = make(map[*JITBlock]struct{}, 1)
					}
					cpu.m68kJitCodePageBlocks[p][block] = struct{}{}
				}
			}
		}
		if cpu.m68kJitCodePageMin != nil && cpu.m68kJitCodePageMax != nil {
			start := r[0]
			end := r[1]
			if end <= start {
				continue
			}
			startPage := start >> 12
			endPage := (end - 1) >> 12
			if startPage >= uint64(len(cpu.m68kJitCodePageMin)) {
				continue
			}
			if endPage >= uint64(len(cpu.m68kJitCodePageMin)) {
				endPage = uint64(len(cpu.m68kJitCodePageMin) - 1)
			}
			for page := startPage; page <= endPage; page++ {
				minOff := uint16(0)
				if page == startPage {
					minOff = uint16(start & 0xFFF)
				}
				maxOff := uint16(0x1000)
				if page == endPage {
					maxOff = uint16(((end - 1) & 0xFFF) + 1)
				}
				if minOff < cpu.m68kJitCodePageMin[page] {
					cpu.m68kJitCodePageMin[page] = minOff
				}
				if maxOff > cpu.m68kJitCodePageMax[page] {
					cpu.m68kJitCodePageMax[page] = maxOff
				}
			}
		}
	}
}

func invalidateM68KJITForGuestWrite(bus Bus32, addr uint64, size uint64) {
	if bus == nil || size == 0 || addr > uint64(^uint32(0)) {
		return
	}
	if mb, ok := bus.(*MachineBus); ok && mb.m68kJITInvalidator != nil {
		mb.m68kJITInvalidator(addr, size)
		return
	}
	snap := runtimeStatus.snapshot()
	if snap.m68k == nil || snap.m68k.cpu == nil || snap.m68k.cpu.bus != bus {
		return
	}
	if size > uint64(^uint32(0)) {
		size = uint64(^uint32(0))
	}
	// Fallback (non-MachineBus) path is also reachable from host goroutines;
	// enqueue rather than touch the cache maps off the CPU thread.
	snap.m68k.cpu.m68kEnqueueJITInvalidation(uint32(addr), uint32(size))
}

// m68kInterpretOne executes one M68K instruction at cpu.PC using the interpreter.
func (cpu *M68KCPU) m68kInterpretOne() {
	cpu.m68kRecordJITFallbackOpcode()
	cpu.StepOne()
}

// m68kFallbackOneInstruction executes exactly one guest instruction through the
// interpreter and returns control to the JIT dispatcher. This is the only
// production interpreter fallback path: after the single instruction retires the
// dispatcher re-enters and may immediately compile/run native code at the new
// PC. Broad interpreter bursts are no longer a production execution strategy
// (M68K_JIT_FALLBACK_REMOVAL_PLAN.md); m68kInterpretFallbackBurst* remain only
// for explicit diagnostic modes.
func (cpu *M68KCPU) m68kFallbackOneInstruction() uint64 {
	cpu.m68kInterpretOne()
	return 1
}

// m68kReportJITStrictFailure prints PC, opcode, disassembly, and the compiler
// error for an emitter/compiler failure under IE_M68K_JIT_STRICT=1.
func (cpu *M68KCPU) m68kReportJITStrictFailure(pc uint32, err error) {
	opcode := uint32(0xFFFFFFFF)
	if pc&1 == 0 && uint64(pc)+2 <= uint64(len(cpu.memory)) {
		opcode = uint32(cpu.memory[pc])<<8 | uint32(cpu.memory[pc+1])
	}
	disasm := "?"
	readMem := func(addr uint64, size int) []byte {
		if addr+uint64(size) > uint64(len(cpu.memory)) {
			return nil
		}
		return cpu.memory[addr : addr+uint64(size)]
	}
	if lines := disassembleM68K(readMem, uint64(pc), 1); len(lines) > 0 {
		disasm = lines[0].Mnemonic
	}
	fmt.Printf("M68K JIT strict: compile failure pc=%08X opcode=%04X disasm=%q err=%v\n",
		pc, opcode, disasm, err)
}

func (cpu *M68KCPU) m68kInterpretFallbackBurst(max int) uint64 {
	return cpu.m68kInterpretFallbackBurstMode(max, true)
}

func (cpu *M68KCPU) m68kInterpretFallbackBurstThroughBranches(max int) uint64 {
	return cpu.m68kInterpretFallbackBurstMode(max, false)
}

func (cpu *M68KCPU) m68kExecuteJITFPUHelper(pc uint32) uint64 {
	cpu.PC = pc
	cpu.lastExecPC = pc
	if pc&1 != 0 {
		cpu.ProcessException(M68K_VEC_ADDRESS_ERROR)
		return 1
	}
	if pc >= cpu.ProfileTopOfRAM()-M68K_WORD_SIZE {
		cpu.recordFault(pc, 2, false, 0)
		cpu.ProcessException(M68K_VEC_BUS_ERROR)
		return 1
	}
	opcode := cpu.Fetch16()
	cpu.currentIR = opcode
	cpu.lastExecOpcode = opcode
	cpu.ExecFPUInstruction(opcode)
	return 1
}

func (cpu *M68KCPU) m68kExecuteJITInterpreterHelper(pc uint32) uint64 {
	cpu.PC = pc
	cpu.lastExecPC = pc
	cpu.StepOne()
	return 1
}

func (cpu *M68KCPU) m68kExecuteJITMMIOMOVEHelper(pc uint32) uint64 {
	cpu.PC = pc
	cpu.lastExecPC = pc
	if pc&1 != 0 {
		cpu.ProcessException(M68K_VEC_ADDRESS_ERROR)
		return 1
	}
	if pc >= cpu.ProfileTopOfRAM()-M68K_WORD_SIZE {
		cpu.recordFault(pc, 2, false, 0)
		cpu.ProcessException(M68K_VEC_BUS_ERROR)
		return 1
	}
	opcode := cpu.Fetch16()
	cpu.currentIR = opcode
	cpu.lastExecOpcode = opcode

	group := opcode >> 12
	size := M68K_SIZE_LONG
	if group == 0x1 {
		size = M68K_SIZE_BYTE
	} else if group == 0x3 {
		size = M68K_SIZE_WORD
	} else if group != 0x2 {
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return 1
	}

	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstMode := (opcode >> 6) & 7
	dstReg := (opcode >> 9) & 7

	if m68kIsNativeSupportedMOVEA(opcode) {
		if srcMode != 7 || srcReg != 1 || dstMode != 1 {
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return 1
		}
		addr := cpu.Fetch32()
		if !isNativeNumericMMIOAddr(addr) {
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return 1
		}
		value := cpu.m68kReadJITMMIOMOVEValue(addr, size)
		if size == M68K_SIZE_WORD {
			value = uint32(int32(int16(value)))
		}
		cpu.AddrRegs[dstReg] = value
		return 1
	}

	memToMemMMIO := dstMode == 7 && dstReg == 1 && m68kIsNativeSupportedMOVEMemToMemGuarded(opcode)
	dstMem := m68kMoveMemToMemEASupported(dstMode, dstReg, false)
	if !memToMemMMIO && !m68kIsNativeSupportedMOVEGuarded(opcode) {
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return 1
	}

	srcRegOrImm := srcMode == 0 || (group != 0x1 && srcMode == 1) || (srcMode == 7 && srcReg == 4)
	switch {
	case srcMode == 7 && srcReg == 1 && dstMode == 0:
		addr := cpu.Fetch32()
		if !isNativeNumericMMIOAddr(addr) {
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return 1
		}
		value := cpu.m68kReadJITMMIOMOVEValue(addr, size)
		cpu.m68kStoreJITMMIOMOVEDataReg(dstReg, value, size)
		cpu.SetFlags(value, size)
	case srcRegOrImm && dstMem:
		value := uint32(0)
		switch srcMode {
		case 0:
			value = cpu.DataRegs[srcReg]
		case 1:
			value = cpu.AddrRegs[srcReg]
		case 7:
			switch size {
			case M68K_SIZE_BYTE:
				value = uint32(cpu.Fetch16() & 0x00FF)
			case M68K_SIZE_WORD:
				value = uint32(cpu.Fetch16())
			default:
				value = cpu.Fetch32()
			}
		default:
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return 1
		}
		addr, postIncrement := cpu.resolveEAWithPrePost(dstMode, dstReg, size)
		cpu.m68kWriteJITMMIOMOVEValue(addr, value, size)
		if postIncrement != 0 {
			cpu.AddrRegs[dstReg] += postIncrement
		}
		cpu.SetFlags(value, size)
	case memToMemMMIO:
		srcAddr, postIncrement := cpu.resolveEAWithPrePost(srcMode, srcReg, size)
		addr := cpu.GetEffectiveAddress(dstMode, dstReg)
		if !isNativeNumericMMIOAddr(addr) {
			cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
			return 1
		}
		value := cpu.m68kReadJITMMIOMOVEValue(srcAddr, size)
		if postIncrement != 0 {
			cpu.AddrRegs[srcReg] += postIncrement
		}
		cpu.m68kWriteJITMMIOMOVEValue(addr, value, size)
		cpu.SetFlags(value, size)
	default:
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
	}
	return 1
}

func (cpu *M68KCPU) m68kExecuteJITMMIOCLRHelper(pc uint32) uint64 {
	cpu.PC = pc
	cpu.lastExecPC = pc
	if pc&1 != 0 {
		cpu.ProcessException(M68K_VEC_ADDRESS_ERROR)
		return 1
	}
	if pc >= cpu.ProfileTopOfRAM()-M68K_WORD_SIZE {
		cpu.recordFault(pc, 2, false, 0)
		cpu.ProcessException(M68K_VEC_BUS_ERROR)
		return 1
	}
	opcode := cpu.Fetch16()
	cpu.currentIR = opcode
	cpu.lastExecOpcode = opcode
	if !m68kIsNativeSupportedCLR(opcode) {
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return 1
	}

	size := int((opcode >> 6) & 3)
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	if mode == 0 || size == 3 {
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return 1
	}

	addr, postIncrement := cpu.resolveEAWithPrePost(mode, reg, size)
	cpu.m68kWriteJITMMIOMOVEValue(addr, 0, size)
	if postIncrement != 0 {
		cpu.AddrRegs[reg] += postIncrement
	}
	cpu.SetFlags(0, size)
	return 1
}

func (cpu *M68KCPU) m68kReadJITMMIOMOVEValue(addr uint32, size int) uint32 {
	switch size {
	case M68K_SIZE_BYTE:
		return uint32(cpu.Read8(addr))
	case M68K_SIZE_WORD:
		return uint32(cpu.Read16(addr))
	default:
		return cpu.Read32(addr)
	}
}

func (cpu *M68KCPU) m68kWriteJITMMIOMOVEValue(addr, value uint32, size int) {
	switch size {
	case M68K_SIZE_BYTE:
		cpu.Write8(addr, uint8(value))
	case M68K_SIZE_WORD:
		cpu.Write16(addr, uint16(value))
	default:
		cpu.Write32(addr, value)
	}
}

func (cpu *M68KCPU) m68kStoreJITMMIOMOVEDataReg(reg uint16, value uint32, size int) {
	switch size {
	case M68K_SIZE_BYTE:
		cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFFFF00) | (value & 0xFF)
	case M68K_SIZE_WORD:
		cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFF0000) | (value & 0xFFFF)
	default:
		cpu.DataRegs[reg] = value
	}
}

func (cpu *M68KCPU) m68kHandleJITHelper(ctx *M68KJITContext) (uint64, bool) {
	if ctx == nil || ctx.NeedHelper == m68kJITHelperNone {
		return 0, false
	}
	helper := ctx.NeedHelper
	helperPC := ctx.HelperPC
	ctx.NeedHelper = m68kJITHelperNone
	switch helper {
	case m68kJITHelperFPU:
		return cpu.m68kExecuteJITFPUHelper(helperPC), true
	case m68kJITHelperMMIOMOVE:
		return cpu.m68kExecuteJITMMIOMOVEHelper(helperPC), true
	case m68kJITHelperMMIOCLR:
		return cpu.m68kExecuteJITMMIOCLRHelper(helperPC), true
	case m68kJITHelperCHK2CMP2:
		return cpu.m68kExecuteJITInterpreterHelper(helperPC), true
	case m68kJITHelperCASCAS2:
		return cpu.m68kExecuteJITInterpreterHelper(helperPC), true
	case m68kJITHelperMOVES:
		return cpu.m68kExecuteJITInterpreterHelper(helperPC), true
	case m68kJITHelperTRAPcc:
		return cpu.m68kExecuteJITInterpreterHelper(helperPC), true
	case m68kJITHelperBKPT:
		return cpu.m68kExecuteJITInterpreterHelper(helperPC), true
	case m68kJITHelperCALLM:
		return cpu.m68kExecuteJITInterpreterHelper(helperPC), true
	case m68kJITHelperRTM:
		return cpu.m68kExecuteJITInterpreterHelper(helperPC), true
	default:
		cpu.PC = helperPC
		cpu.ProcessException(M68K_VEC_ILLEGAL_INSTR)
		return 1, true
	}
}

func (cpu *M68KCPU) m68kInterpretFallbackBurstMode(max int, stopOnFlowChange bool) uint64 {
	if max <= 0 {
		return 0
	}
	var retired uint64
	for retired < uint64(max) && cpu.running.Load() && !cpu.stopped.Load() {
		if cpu.debugHandleBreakIn(uint64(cpu.PC)) {
			cpu.running.Store(false)
			break
		}
		pc := cpu.PC
		if pc&1 != 0 || pc >= cpu.ProfileTopOfRAM()-M68K_WORD_SIZE {
			cpu.m68kInterpretOne()
			retired++
			break
		}
		if !stopOnFlowChange && cpu.m68kJitCache != nil && cpu.m68kJitCache.Get(uint64(pc)) != nil {
			break
		}
		opcode := uint16(cpu.memory[pc])<<8 | uint16(cpu.memory[pc+1])
		nextPC := pc + uint32(m68kInstrLength(cpu.memory, pc))
		cpu.m68kInterpretOne()
		retired++
		if !cpu.running.Load() || cpu.stopped.Load() {
			break
		}
		if stopOnFlowChange {
			if m68kIsBlockTerminator(opcode) || cpu.PC != nextPC {
				break
			}
			if cpu.m68kJitCache != nil && cpu.m68kJitCache.Get(uint64(cpu.PC)) != nil {
				break
			}
		}
		if cpu.pendingException.Load() != 0 {
			break
		}
	}
	return retired
}

func (cpu *M68KCPU) tryM68KIndexedByteCopyCountLoop() (uint64, bool) {
	const loopLen = 12
	pc := cpu.PC
	if uint64(pc)+loopLen > uint64(len(cpu.memory)) {
		return 0, false
	}
	if (uint16(cpu.memory[pc])<<8|uint16(cpu.memory[pc+1])) != 0x13B1 ||
		(uint16(cpu.memory[pc+2])<<8|uint16(cpu.memory[pc+3])) != 0x1800 ||
		(uint16(cpu.memory[pc+4])<<8|uint16(cpu.memory[pc+5])) != 0x0800 ||
		(uint16(cpu.memory[pc+6])<<8|uint16(cpu.memory[pc+7])) != 0x5289 ||
		(uint16(cpu.memory[pc+8])<<8|uint16(cpu.memory[pc+9])) != 0xB3C8 ||
		(uint16(cpu.memory[pc+10])<<8|uint16(cpu.memory[pc+11])) != 0x66F4 {
		return 0, false
	}

	start := cpu.AddrRegs[1]
	target := cpu.AddrRegs[0]
	if target <= start {
		return 0, false
	}
	count := target - start
	if count > 1<<20 {
		return 0, false
	}
	srcBase := uint64(start) + uint64(cpu.DataRegs[1])
	dstBase := uint64(start) + uint64(cpu.DataRegs[0])
	if srcBase+uint64(count) > uint64(len(cpu.memory)) ||
		dstBase+uint64(count) > uint64(len(cpu.memory)) {
		return 0, false
	}

	for off := uint32(0); off < count; off++ {
		srcAddr := uint32(srcBase) + off
		dstAddr := uint32(dstBase) + off
		cpu.memory[dstAddr] = cpu.memory[srcAddr]
	}
	cpu.invalidateM68KJITForGuestWrite(uint32(dstBase), count)

	cpu.AddrRegs[1] = target
	result := cpu.AddrRegs[1] - cpu.AddrRegs[0]
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)
	if cpu.AddrRegs[0] > cpu.AddrRegs[1] {
		cpu.SR |= M68K_SR_C
	}
	if ((cpu.AddrRegs[1] ^ cpu.AddrRegs[0]) & (cpu.AddrRegs[1] ^ result) & 0x80000000) != 0 {
		cpu.SR |= M68K_SR_V
	}
	if result == 0 {
		cpu.SR |= M68K_SR_Z
	}
	if result&0x80000000 != 0 {
		cpu.SR |= M68K_SR_N
	}
	cpu.PC = pc + loopLen
	return uint64(count) * 4, true
}

func (cpu *M68KCPU) tryM68KLongFillCountLoop() (uint64, bool) {
	const loopLen = 14
	pc := cpu.PC
	if uint64(pc)+loopLen > uint64(len(cpu.memory)) {
		return 0, false
	}
	if (uint16(cpu.memory[pc])<<8|uint16(cpu.memory[pc+1])) != 0x24C1 ||
		(uint16(cpu.memory[pc+2])<<8|uint16(cpu.memory[pc+3])) != 0x2608 ||
		(uint16(cpu.memory[pc+4])<<8|uint16(cpu.memory[pc+5])) != 0x968A ||
		(uint16(cpu.memory[pc+6])<<8|uint16(cpu.memory[pc+7])) != 0xD689 ||
		(uint16(cpu.memory[pc+8])<<8|uint16(cpu.memory[pc+9])) != 0x7804 ||
		(uint16(cpu.memory[pc+10])<<8|uint16(cpu.memory[pc+11])) != 0xB883 ||
		(uint16(cpu.memory[pc+12])<<8|uint16(cpu.memory[pc+13])) != 0x65F2 {
		return 0, false
	}

	a0 := cpu.AddrRegs[0]
	a1 := cpu.AddrRegs[1]
	a2 := cpu.AddrRegs[2]
	remaining := a0 + a1 - a2
	if remaining <= 4 {
		return 0, false
	}
	count := (remaining - 1) / 4
	if count == 0 || count > 16<<20 {
		return 0, false
	}
	const maxLongFillLoopChunk = 36
	fullCount := count
	if count > maxLongFillLoopChunk {
		count = maxLongFillLoopChunk
	}
	writeStart := a2
	writeSize := uint64(count) * 4
	if uint64(writeStart)+writeSize > uint64(len(cpu.memory)) {
		return 0, false
	}

	value := cpu.DataRegs[1]
	for i := uint32(0); i < count; i++ {
		addr := writeStart + i*4
		cpu.memory[addr] = byte(value >> 24)
		cpu.memory[addr+1] = byte(value >> 16)
		cpu.memory[addr+2] = byte(value >> 8)
		cpu.memory[addr+3] = byte(value)
	}
	cpu.invalidateM68KJITForGuestWrite(writeStart, uint32(writeSize))

	cpu.AddrRegs[2] = writeStart + count*4
	cpu.DataRegs[3] = a0 - cpu.AddrRegs[2] + a1
	cpu.DataRegs[4] = 4

	src := cpu.DataRegs[3]
	dst := cpu.DataRegs[4]
	result := dst - src
	cpu.SR &= ^uint16(M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C)
	if result&0x80000000 != 0 {
		cpu.SR |= M68K_SR_N
	}
	if result == 0 {
		cpu.SR |= M68K_SR_Z
	}
	if ((dst ^ src) & (dst ^ result) & 0x80000000) != 0 {
		cpu.SR |= M68K_SR_V
	}
	if src > dst {
		cpu.SR |= M68K_SR_C
	}
	if count == fullCount {
		cpu.PC = pc + loopLen
	} else {
		cpu.PC = pc
	}
	return uint64(count) * 7, true
}

func (cpu *M68KCPU) m68kResetJITDiagnostics() {
	if os.Getenv("IE_M68K_JIT_DUMP_RING") == "1" {
		cpu.m68kJitRecordNativePCs.Store(true)
	}
	cpu.m68kJitNativeBlocksExecuted.Store(0)
	cpu.m68kJitRegionPromotions.Store(0)
	cpu.m68kJitStaticJMPChases.Store(0)
	cpu.m68kJitNativeRetCountSum.Store(0)
	cpu.m68kJitNativeChainCountSum.Store(0)
	cpu.m68kJitNativeNoChainReturns.Store(0)
	cpu.m68kJitNativeHelperExits.Store(0)
	cpu.m68kJitNativeExceptionExits.Store(0)
	cpu.m68kJitNativeInvalExits.Store(0)
	cpu.m68kJitMMIOGuardExits.Store(0)
	cpu.m68kJitUnsupportedOneExits.Store(0)
	cpu.m68kJitCompileFailureExits.Store(0)
	cpu.m68kJitWarmupInstructions.Store(0)
	cpu.m68kJitLastNativePC.Store(0)
	cpu.m68kJitNativePCMu.Lock()
	if cpu.m68kJitRecordNativePCs.Load() && cpu.m68kJitNativePCCounts == nil {
		cpu.m68kJitNativePCCounts = make(map[uint32]uint64, 128)
	} else if cpu.m68kJitNativePCCounts != nil {
		clear(cpu.m68kJitNativePCCounts)
	}
	if cpu.m68kJitRecordNativePCs.Load() && cpu.m68kJitNativePCRetCounts == nil {
		cpu.m68kJitNativePCRetCounts = make(map[uint32]uint64, 128)
	} else if cpu.m68kJitNativePCRetCounts != nil {
		clear(cpu.m68kJitNativePCRetCounts)
	}
	if cpu.m68kJitRecordNativePCs.Load() && cpu.m68kJitNativeInvalPCCounts == nil {
		cpu.m68kJitNativeInvalPCCounts = make(map[uint32]uint64, 128)
	} else if cpu.m68kJitNativeInvalPCCounts != nil {
		clear(cpu.m68kJitNativeInvalPCCounts)
	}
	clear(cpu.m68kJitNativePCRing[:])
	cpu.m68kJitNativePCRingIdx = 0
	cpu.m68kJitNativePCMu.Unlock()
	cpu.m68kJitFallbackInstructions.Store(0)
	cpu.m68kJitBailoutCount.Store(0)
	cpu.m68kJitLastFallbackPC.Store(0)
	cpu.m68kJitLastFallbackOpcode.Store(0)
	for _, opcode := range cpu.m68kJitFallbackTouched {
		cpu.m68kJitFallbackOpcodeCounts[opcode].Store(0)
		cpu.m68kJitFallbackOpcodePCs[opcode].Store(0)
	}
	cpu.m68kJitFallbackTouched = cpu.m68kJitFallbackTouched[:0]
	if cpu.m68kJitRecordFallbackSnapshots {
		cpu.m68kJitFallbackSnapshotMu.Lock()
		clear(cpu.m68kJitFallbackFirstSnapshots)
		cpu.m68kJitFallbackSnapshotMu.Unlock()
	}
	cpu.m68kJitCompileFailMu.Lock()
	if cpu.m68kJitCompileFailCounts != nil {
		clear(cpu.m68kJitCompileFailCounts)
	}
	if cpu.m68kJitCompileFailErrors != nil {
		clear(cpu.m68kJitCompileFailErrors)
	}
	cpu.m68kJitCompileFailMu.Unlock()
}

func m68kBumpJITBlockHotness(block *JITBlock, increment uint32) {
	if block == nil {
		return
	}
	if increment == 0 {
		increment = 1
	}
	if ^uint32(0)-block.execCount < increment {
		block.execCount = ^uint32(0)
		return
	}
	block.execCount += increment
}

func m68kNativeHotnessIncrement(block *JITBlock, chainCount uint32) uint32 {
	if block == nil || chainCount == 0 || block.instrCount <= 0 {
		return 1
	}
	perEntry := uint32(block.instrCount)
	return (chainCount + perEntry - 1) / perEntry
}

// m68kJITRetiredInstructionCount computes the number of guest instructions a
// native execution retired. The accounting contract is uniform:
//   - ChainCount holds instructions retired in every block chained THROUGH,
//     plus in-block loop re-executions (accumulated by the chain-exit prologue
//     and the within-block loop emitters).
//   - RetCount holds the final (returning) block's own linear instruction count.
//   - Total retired = ChainCount + RetCount.
//
// Earlier code special-cased `retCount <= blockInstrCount` to add the two, else
// returned retCount alone. That dropped ChainCount whenever the final chained
// block was larger than the entry block (blockInstrCount is the ENTRY block's
// size, unrelated to the final block's retCount) — a distributed under-count
// that desynchronized the instruction-count-keyed interrupt cadence from the
// interpreter. The sum is always correct given the contract above.
func m68kJITRetiredInstructionCount(retCount, chainCount uint32, blockInstrCount int, exitSignal bool) uint64 {
	if retCount == 0 {
		if chainCount > 0 {
			return uint64(chainCount)
		}
		if !exitSignal && blockInstrCount > 0 {
			return uint64(blockInstrCount)
		}
		return 0
	}
	return uint64(chainCount) + uint64(retCount)
}

func (cpu *M68KCPU) m68kJITShouldWarmupInterpret(pc uint32) bool {
	if cpu == nil || cpu.m68kJitForceNative || cpu.m68kJitWarmupLimit <= 1 {
		return false
	}
	if cpu.m68kJitWarmupCounts == nil {
		cpu.m68kJitWarmupCounts = make(map[uint32]uint8, 4096)
	}
	count := cpu.m68kJitWarmupCounts[pc]
	if count+1 >= cpu.m68kJitWarmupLimit {
		cpu.m68kJitWarmupCounts[pc] = cpu.m68kJitWarmupLimit
		return false
	}
	cpu.m68kJitWarmupCounts[pc] = count + 1
	return true
}

// m68kCanCompileNativePC gates native compilation by the configured PC ceiling
// (m68kJitNativeMaxPC). A zero ceiling means "no limit", so dynamically loaded
// high-RAM code can remain native.
func (cpu *M68KCPU) m68kCanCompileNativePC(pc uint32) bool {
	if cpu == nil || cpu.m68kJitForceNative {
		return true
	}
	if m68kJitDiagExcludeLo != 0 || m68kJitDiagExcludeHi != 0 {
		if pc >= m68kJitDiagExcludeLo && pc < m68kJitDiagExcludeHi {
			return false
		}
	}
	return cpu.m68kJitNativeMaxPC == 0 || pc <= cpu.m68kJitNativeMaxPC
}

// m68kJitDiagExcludeLo/Hi force the interpreter for blocks whose start PC falls in
// [lo,hi) via IE_M68K_JIT_EXCLUDE_LO / _HI (hex). Diagnostic bisection only.
var m68kJitDiagExcludeLo = m68kJitDiagEnvUint32("IE_M68K_JIT_EXCLUDE_LO")
var m68kJitDiagExcludeHi = m68kJitDiagEnvUint32("IE_M68K_JIT_EXCLUDE_HI")

// m68kJitDiagA0WatchEnabled gates the A0-across-IRQ corruption tracer in the JIT
// dispatcher (IE_M68K_JIT_DIAG_A0WATCH=1). Diagnostic only.
var m68kJitDiagA0WatchEnabled = os.Getenv("IE_M68K_JIT_DIAG_A0WATCH") == "1"

func m68kJitDiagEnvUint32(name string) uint32 {
	raw := os.Getenv(name)
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseUint(raw, 0, 32)
	if err != nil {
		return 0
	}
	return uint32(v)
}

func m68kHashGuestBlockBytes(memory []byte, block *JITBlock) (uint64, bool) {
	if block == nil {
		return 0, false
	}
	const (
		fnvOffset = uint64(1469598103934665603)
		fnvPrime  = uint64(1099511628211)
	)
	hash := fnvOffset
	for _, r := range JITBlockCoveredRanges(block) {
		if r[1] < r[0] || r[1] > uint64(len(memory)) {
			return 0, false
		}
		hash ^= r[0]
		hash *= fnvPrime
		hash ^= r[1]
		hash *= fnvPrime
		for _, b := range memory[int(r[0]):int(r[1])] {
			hash ^= uint64(b)
			hash *= fnvPrime
		}
	}
	return hash, true
}

func m68kStampGuestBlockBytes(memory []byte, block *JITBlock) {
	hash, ok := m68kHashGuestBlockBytes(memory, block)
	if !ok {
		block.guestHash = 0
		block.guestHashValid = false
		return
	}
	block.guestHash = hash
	block.guestHashValid = true
}

func m68kGuestBlockBytesStillMatch(memory []byte, block *JITBlock) bool {
	if block == nil || !block.guestHashValid {
		return true
	}
	hash, ok := m68kHashGuestBlockBytes(memory, block)
	return ok && hash == block.guestHash
}

func (cpu *M68KCPU) m68kTryPromoteJITRegion(block *JITBlock, execMem *ExecMem, memory []byte, disableChains bool) *JITBlock {
	if cpu == nil || cpu.m68kJitCache == nil || block == nil || block.startPC > uint64(^uint32(0)) {
		return block
	}
	if !cpu.m68kCanCompileNativePC(uint32(block.startPC)) {
		return block
	}
	if !m68kTierController.ShouldPromote(block.tier, block.execCount, block.ioBails, block.lastPromoteAt) {
		return block
	}
	block.lastPromoteAt = block.execCount
	region := m68kFormRegion(uint32(block.startPC), memory)
	if region == nil || len(region.blocks) < 2 {
		return block
	}
	if disabledPCs := m68kJITDiagnosticDisabledPCs(); len(disabledPCs) != 0 {
		for _, pc := range region.blockPCs {
			if _, disabled := disabledPCs[pc]; disabled {
				return block
			}
		}
	}
	if disabledRanges := m68kJITDiagnosticDisabledPCRanges(); len(disabledRanges) != 0 {
		for i, instrs := range region.blocks {
			if i < len(region.blockPCs) && m68kJITDiagnosticInstrsTouchRanges(region.blockPCs[i], instrs, disabledRanges) {
				return block
			}
		}
	}
	for _, pc := range region.blockPCs {
		if !cpu.m68kCanCompileNativePC(pc) {
			return block
		}
	}
	newBlock, err := m68kCompileRegion(region, execMem, memory)
	if err != nil {
		return block
	}
	newBlock.execCount = block.execCount
	newBlock.tier = 1
	m68kStampGuestBlockBytes(memory, newBlock)
	cpu.m68kJitCache.Put(newBlock)
	if !disableChains && newBlock.chainEntry != 0 {
		cpu.m68kJitCache.PatchChainsTo(newBlock.startPC, newBlock.chainEntry)
	}
	if !disableChains {
		for i := range newBlock.chainSlots {
			slot := &newBlock.chainSlots[i]
			target := cpu.m68kJitCache.Get(slot.targetPC)
			if target == nil || target.chainEntry == 0 {
				continue
			}
			// Never chain into a target whose guest bytes were overwritten since
			// it was compiled; the direct native jump would bypass the dispatcher's
			// byte-stamp revalidation. Evict and recompile on next dispatch.
			if !m68kGuestBlockBytesStillMatch(cpu.memory, target) {
				cpu.m68kInvalidateJITCodeRange(uint32(target.startPC), uint32(target.endPC))
				continue
			}
			PatchRel32At(slot.patchAddr, target.chainEntry)
		}
	}
	cpu.m68kMarkJITCodeRanges(newBlock)
	cpu.m68kJitRegionPromotions.Add(1)
	return newBlock
}

func (cpu *M68KCPU) m68kRecordJITNativePC(pc uint32) {
	cpu.m68kJitLastNativePC.Store(pc)
	if !cpu.m68kJitRecordNativePCs.Load() {
		return
	}
	cpu.m68kJitNativePCMu.Lock()
	if cpu.m68kJitNativePCCounts == nil {
		cpu.m68kJitNativePCCounts = make(map[uint32]uint64, 128)
	}
	cpu.m68kJitNativePCCounts[pc]++
	cpu.m68kJitNativePCRing[cpu.m68kJitNativePCRingIdx%uint32(len(cpu.m68kJitNativePCRing))] = pc
	cpu.m68kJitNativePCRingIdx++
	cpu.m68kJitNativePCMu.Unlock()
}

func (cpu *M68KCPU) m68kRecordJITNativeRetired(pc uint32, retired uint64) {
	if !cpu.m68kJitRecordNativePCs.Load() {
		return
	}
	cpu.m68kJitNativePCMu.Lock()
	if cpu.m68kJitNativePCRetCounts == nil {
		cpu.m68kJitNativePCRetCounts = make(map[uint32]uint64, 128)
	}
	cpu.m68kJitNativePCRetCounts[pc] += retired
	cpu.m68kJitNativePCMu.Unlock()
}

func (cpu *M68KCPU) m68kRecordJITNativeInvalPC(pc uint32) {
	if !cpu.m68kJitRecordNativePCs.Load() {
		return
	}
	cpu.m68kJitNativePCMu.Lock()
	if cpu.m68kJitNativeInvalPCCounts == nil {
		cpu.m68kJitNativeInvalPCCounts = make(map[uint32]uint64, 128)
	}
	cpu.m68kJitNativeInvalPCCounts[pc]++
	cpu.m68kJitNativePCMu.Unlock()
}

func (cpu *M68KCPU) m68kRecordJITFallbackOpcode() {
	pc := cpu.PC
	cpu.m68kJitLastFallbackPC.Store(pc)
	if pc&1 != 0 || uint64(pc)+2 > uint64(len(cpu.memory)) {
		cpu.m68kJitLastFallbackOpcode.Store(0xFFFF_FFFF)
		return
	}
	opcode := uint32(cpu.memory[pc])<<8 | uint32(cpu.memory[pc+1])
	cpu.m68kJitLastFallbackOpcode.Store(opcode)
	if cpu.m68kJitRecordFallbackSnapshots {
		cpu.m68kRecordJITFallbackSnapshot(pc, uint16(opcode))
	}
	if cpu.m68kJitFallbackOpcodeCounts[opcode].Add(1) == 1 {
		cpu.m68kJitFallbackTouched = append(cpu.m68kJitFallbackTouched, uint16(opcode))
	}
	cpu.m68kJitFallbackOpcodePCs[opcode].Store(pc)
}

func (cpu *M68KCPU) m68kRecordJITFallbackSnapshot(pc uint32, opcode uint16) {
	cpu.m68kJitFallbackSnapshotMu.Lock()
	defer cpu.m68kJitFallbackSnapshotMu.Unlock()
	if cpu.m68kJitFallbackFirstSnapshots == nil {
		cpu.m68kJitFallbackFirstSnapshots = make(map[uint32]m68kJITFallbackSnapshot, 64)
	}
	if _, exists := cpu.m68kJitFallbackFirstSnapshots[pc]; exists {
		return
	}
	var snap m68kJITFallbackSnapshot
	snap.pc = pc
	snap.opcode = opcode
	snap.sr = cpu.SR
	copy(snap.data[:], cpu.DataRegs[:])
	copy(snap.addr[:], cpu.AddrRegs[:])
	cpu.m68kJitFallbackFirstSnapshots[pc] = snap
}

func (cpu *M68KCPU) m68kRecordJITCompileFailure(pc uint32, err error) {
	if cpu == nil || err == nil {
		return
	}
	cpu.m68kJitCompileFailMu.Lock()
	defer cpu.m68kJitCompileFailMu.Unlock()
	if cpu.m68kJitCompileFailCounts == nil {
		cpu.m68kJitCompileFailCounts = make(map[uint32]uint64, 64)
	}
	if cpu.m68kJitCompileFailErrors == nil {
		cpu.m68kJitCompileFailErrors = make(map[uint32]string, 64)
	}
	cpu.m68kJitCompileFailCounts[pc]++
	cpu.m68kJitCompileFailErrors[pc] = err.Error()
}

func m68kBlockMayUseGenericIOFallback(instrs []M68KJITInstr) bool {
	for i := range instrs {
		if m68kInstrGenericIOFallbackUnsafe(&instrs[i]) {
			return true
		}
	}
	return false
}

func m68kInstrGenericIOFallbackUnsafe(ji *M68KJITInstr) bool {
	if !m68kInstrMaySetGenericIOFallback(ji) {
		return false
	}

	opcode := ji.opcode
	group := opcode >> 12
	switch group {
	case 0x0: // CMPI is safe to re-execute only for non-mutating EAs.
		if m68kIsImmediateLogicDn(opcode) {
			return false
		}
		if m68kIsNativeSupportedImmediateLogicEA(opcode) {
			return false
		}
		if m68kIsBTSTImmAnDisp(opcode) {
			return false
		}
		if m68kIsNativeSupportedCMPI(opcode) {
			return false
		}
		if opcode&0xFF00 == 0x0C00 && (opcode>>6)&3 != 3 {
			mode := (opcode >> 3) & 7
			return mode == 3 || mode == 4
		}
		return true
	case 0x1, 0x2, 0x3: // MOVE
		if m68kIsNativeSupportedMOVEA(opcode) {
			return false
		}
		if m68kIsNativeSupportedMOVEGuarded(opcode) {
			return false
		}
		if m68kIsNativeSupportedMOVEMemToMemGuarded(opcode) {
			return false
		}
		if m68kIsMoveLongStackDispToReg(opcode) || m68kIsMoveLongRegToStackDisp(opcode) ||
			m68kIsMoveLongRegToStackPredec(opcode) || m68kIsMoveLongStackDispToStackPredec(opcode) ||
			m68kIsMoveLongStackPostincToStackPredec(opcode) || m68kIsMoveLongStackIndirectToStackPostinc(opcode) ||
			m68kIsMoveLongStackDispToAddressIndirect(opcode) || m68kIsMoveLongAuditedStackMemory(opcode) ||
			m68kIsMovePostincPostinc(opcode) {
			return false
		}
		srcMode := (opcode >> 3) & 7
		srcReg := opcode & 7
		dstMode := (opcode >> 6) & 7
		dstReg := (opcode >> 9) & 7
		if m68kEAMayUseMemHelper(srcMode, srcReg, false) {
			if dstMode == 0 || dstMode == 1 {
				return false
			}
			if srcMode != 3 && srcMode != 4 && dstMode != 4 {
				return false
			}
			return true
		}
		if m68kEAMayUseMemHelper(dstMode, dstReg, true) {
			return dstMode == 4
		}
		return false
	case 0x5: // ADDQ/SUBQ Dn/An/(An)/-(An)/d16(An) are audited native paths.
		if m68kIsNativeSupportedDBcc(opcode) {
			return false
		}
		if opcode&0xF0C0 == 0x50C0 && opcode&0xF0F8 != 0x50C8 && opcode&0xF0F8 != 0x50F8 {
			return !m68kIsNativeSupportedScc(opcode)
		}
		if opcode&0x00C0 != 0x00C0 {
			mode := (opcode >> 3) & 7
			return mode != 0 && mode != 1 && mode != 2 && mode != 3 && mode != 4 && mode != 5
		}
		return true
	case 0xB: // CMP <ea>,Dn is replay-safe; audited EOR Dn,<ea> owns memory writes.
		opmode := (opcode >> 6) & 7
		if m68kIsNativeSupportedCMPM(opcode) {
			return false
		}
		if (opmode == 3 || opmode == 7) && m68kIsNativeSupportedCMPA(opcode) {
			return false
		}
		if opmode <= 2 && m68kIsNativeSupportedCMPToDn(opcode) {
			return false
		}
		if opmode >= 4 && opmode <= 6 && m68kIsNativeSupportedLogicDnToEA(opcode) {
			return false
		}
		return opmode > 2
	case 0x8: // OR <ea>,Dn is safe only when the source EA read can be replayed.
		opmode := (opcode >> 6) & 7
		if opmode <= 2 {
			if m68kIsNativeSupportedLogicEAToDn(opcode) {
				return false
			}
			return m68kEAToDnALUSourceFallbackUnsafe(opcode)
		}
		if m68kIsNativeSupportedLogicDnToEA(opcode) {
			return false
		}
		return true
	case 0xC: // AND <ea>,Dn / AND Dn,<ea>
		if opcode&0xF0C0 == 0xC0C0 && m68kIsNativeSupportedMULW(opcode) {
			return m68kEAToDnALUSourceFallbackUnsafe(opcode)
		}
		opmode := (opcode >> 6) & 7
		if opmode <= 2 {
			if m68kIsNativeSupportedLogicEAToDn(opcode) {
				return false
			}
			return m68kEAToDnALUSourceFallbackUnsafe(opcode)
		}
		if m68kIsNativeSupportedLogicDnToEA(opcode) {
			return false
		}
		return true
	case 0x9, 0xD: // SUB/ADD <ea>,Dn write Dn after a source read.
		opmode := (opcode >> 6) & 7
		if opmode <= 2 {
			return m68kEAToDnALUSourceFallbackUnsafe(opcode)
		}
		if (opmode == 3 || opmode == 7) && m68kIsNativeSupportedAddrArithA(opcode) {
			return false
		}
		if m68kIsNativeSupportedArithDnToEA(opcode) {
			return false
		}
		return true
	case 0x4:
		if m68kIsNativeSupportedPEA(opcode) || m68kIsNativeSupportedCLR(opcode) ||
			m68kIsNativeSupportedTST(opcode) || m68kIsNativeSupportedNOT(opcode) ||
			m68kIsNativeSupportedNEG(opcode) || m68kIsNativeSupportedNEGX(opcode) ||
			m68kIsNativeSupportedTAS(opcode) || m68kIsNativeSupportedNBCD(opcode) ||
			m68kIsNativeSupportedMOVEFromStatus(opcode) || m68kIsNativeSupportedMOVEToCCR(opcode) {
			return false
		}
		if m68kIsNativeSupportedCHK(opcode) {
			return false
		}
		if opcode&0xFF80 == 0x4C00 && m68kIsNativeSupportedMULLDIVL(opcode) {
			return false
		}
		if m68kIsNativeSupportedMOVEM(opcode) {
			return false
		}
		if m68kIsNativeSupportedMOVEToSR(opcode) {
			return false
		}
		if m68kIsTSTAnDisp(opcode) {
			return false
		}
	}
	return true
}

func m68kBlockProductionNativeSafe(instrs []M68KJITInstr) bool {
	if len(instrs) == 0 {
		return false
	}
	for i := range instrs {
		if !m68kInstrProductionNativeSafe(&instrs[i]) {
			return false
		}
	}
	return true
}

func m68kInstrTouchesA7OrControl(ji *M68KJITInstr) bool {
	if ji == nil {
		return false
	}
	opcode := ji.opcode
	group := opcode >> 12
	if group == 0x1 || group == 0x2 || group == 0x3 {
		srcMode := (opcode >> 3) & 7
		srcReg := opcode & 7
		dstMode := (opcode >> 6) & 7
		dstReg := (opcode >> 9) & 7
		return m68kModeTouchesSP(srcMode, srcReg) || m68kModeTouchesSP(dstMode, dstReg)
	}
	if group == 0x5 {
		mode := (opcode >> 3) & 7
		reg := opcode & 7
		return opcode&0x00C0 != 0x00C0 && m68kModeTouchesSP(mode, reg)
	}
	if group == 0x6 {
		return opcode&0xFF00 == 0x6100 // BSR
	}
	if group == 0x9 || group == 0xD {
		opmode := (opcode >> 6) & 7
		dstReg := (opcode >> 9) & 7
		return (opmode == 3 || opmode == 7) && dstReg == 7 // SUBA/ADDA ...,A7
	}
	if group != 0x4 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch {
	case opcode == 0x4E70 || opcode&0xFFF0 == 0x4E40 || opcode&0xFFF0 == 0x4E60 || opcode&0xFFFE == 0x4E7A: // RESET/TRAP/MOVE USP/MOVEC
		return true
	case opcode == 0x4E75 || opcode == 0x4E73 || opcode == 0x4E77: // RTS/RTE/RTR
		return true
	case opcode&0xFFC0 == 0x4E80 || opcode&0xFFC0 == 0x4EC0: // JSR/JMP
		return true
	case opcode&0xFFF8 == 0x4E50 || opcode&0xFFF8 == 0x4808 || opcode&0xFFF8 == 0x4E58: // LINK/UNLK
		return true
	case opcode&0xF1C0 == 0x41C0 && ((opcode>>9)&7) == 7: // LEA ...,A7
		return true
	case opcode&0xFFC0 == 0x4840: // PEA
		return true
	case opcode&0xFB80 == 0x4880 && m68kModeTouchesSP(mode, reg): // MOVEM with A7 EA
		return true
	default:
		return m68kModeTouchesSP(mode, reg)
	}
}

func m68kBlockTouchesA7OrControl(instrs []M68KJITInstr) bool {
	for i := range instrs {
		if m68kInstrTouchesA7OrControl(&instrs[i]) {
			return true
		}
	}
	return false
}

// m68kCanUseProductionNativeBlock decides whether a block runs as native code.
// Per M68K_JIT_FALLBACK_REMOVAL_PLAN.md the broad conservative gate
// (m68kNeedsConservativeFallback) no longer decides production admission: a
// block is native iff every instruction has an emitter (!m68kNeedsFallback) and
// is individually production-native-safe (m68kBlockProductionNativeSafe), with
// MMIO/guarded forms handled by per-instruction self-bail at run time rather
// than blanket block rejection. m68kNeedsConservativeFallback remains only as a
// diagnostic/reporting predicate (see m68kConservativeFallbackDiagnostic and the
// AROS diagnostic tests); it is intentionally NOT consulted here.
func m68kCanUseProductionNativeBlock(memory []byte, startPC uint32, instrs []M68KJITInstr) bool {
	return len(instrs) > 0 &&
		!m68kNeedsFallback(instrs) &&
		m68kBlockProductionNativeSafe(instrs) &&
		!m68kBlockMayUseGenericIOFallback(instrs)
}

// m68kConservativeFallbackDiagnostic exposes the retired conservative gate for
// diagnostics/reporting only. It must never gate production native admission.
func m68kConservativeFallbackDiagnostic(memory []byte, startPC uint32, instrs []M68KJITInstr) bool {
	return m68kNeedsConservativeFallback(memory, startPC, instrs)
}

func m68kCanPrefixInstruction(ji *M68KJITInstr) bool {
	if ji == nil || ji.fusedFlag != 0 {
		return false
	}
	opcode := ji.opcode
	if m68kIsBlockTerminator(opcode) {
		return false
	}
	if opcode>>12 == 0x6 {
		return m68kInstrProductionNativeSafe(ji)
	}
	if opcode&0xF0F8 == 0x50C8 {
		return m68kInstrProductionNativeSafe(ji)
	}
	return true
}

// m68kInstrWritesAddrReg reports (conservatively) whether the instruction writes
// address register An. Used to decide whether a native prefix may precede a
// register-indirect JSR/JMP whose An base it would mutate. Over-approximates:
// when unsure for a shape that could target An, returns true so the prefix
// bails rather than risk the interpreter fallback seeing an uncommitted base.
func m68kInstrWritesAddrReg(ji *M68KJITInstr, an uint16) bool {
	op := ji.opcode
	dst := (op >> 9) & 7
	switch op >> 12 {
	case 0x2, 0x3: // MOVEA.L / MOVEA.W: dstMode == 001
		if (op>>6)&7 == 1 && dst == an {
			return true
		}
	case 0x9, 0xD: // SUBA / ADDA: opmode 011 (word) or 111 (long)
		if om := (op >> 6) & 7; (om == 3 || om == 7) && dst == an {
			return true
		}
	}
	if op&0xF1C0 == 0x41C0 && dst == an { // LEA An
		return true
	}
	if op&0xFB80 == 0x4880 { // MOVEM (restore may load An); be conservative
		return true
	}
	return false
}

func m68kProductionNativePrefix(memory []byte, startPC uint32, instrs []M68KJITInstr) []M68KJITInstr {
	if m68kIsPEARTSTrampoline(instrs) {
		return nil
	}
	// A register-indirect JSR/JMP terminator whose An base is produced by a
	// preceding prefix instruction cannot be split: the interpreter fallback for
	// the call must observe the prefix's register writes, and the cached-block /
	// stack-codepage invariants require the whole setup to fall back together.
	// Other terminators (RTS/RTE/RTR, abs/PC-relative JSR, register-indirect JSR
	// whose base the prefix never touches) impose no such constraint — the prefix
	// loop below already stops at any block terminator, so a safe prefix that
	// merely PRECEDES such a terminator is retained instead of discarded.
	if n := len(instrs); n > 0 {
		term := instrs[n-1].opcode
		if term&0xFFC0 == 0x4E80 || term&0xFFC0 == 0x4EC0 { // JSR / JMP
			if mode := (term >> 3) & 7; mode == 2 || mode == 5 || mode == 6 {
				base := term & 7
				for i := 0; i < n-1; i++ {
					if m68kInstrWritesAddrReg(&instrs[i], base) {
						return nil
					}
				}
			}
		}
	}
	var best []M68KJITInstr
	for i := range instrs {
		if !m68kCanPrefixInstruction(&instrs[i]) {
			break
		}
		candidate := instrs[:i+1]
		if !m68kCanUseProductionNativeBlock(memory, startPC, candidate) {
			break
		}
		best = candidate
	}
	if len(best) == len(instrs) {
		return nil
	}
	if len(best) < 2 {
		if len(best) == 1 && m68kInstrProductionNativeSafe(&best[0]) {
			return best
		}
		return nil
	}
	return best
}

func m68kStaticJMPTrampolineTarget(memory []byte, pc uint32, topOfRAM uint32) (uint32, bool) {
	if pc+2 > uint32(len(memory)) {
		return 0, false
	}
	opcode := uint16(memory[pc])<<8 | uint16(memory[pc+1])
	if opcode&0xFFC0 != 0x4EC0 {
		return 0, false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	var target uint32
	switch {
	case mode == 7 && reg == 1: // JMP abs.L
		if pc+6 > uint32(len(memory)) {
			return 0, false
		}
		target = uint32(memory[pc+2])<<24 | uint32(memory[pc+3])<<16 |
			uint32(memory[pc+4])<<8 | uint32(memory[pc+5])
	case mode == 7 && reg == 0: // JMP abs.W
		if pc+4 > uint32(len(memory)) {
			return 0, false
		}
		w := uint16(memory[pc+2])<<8 | uint16(memory[pc+3])
		target = uint32(int32(int16(w)))
	case mode == 7 && reg == 2: // JMP d16(PC)
		if pc+4 > uint32(len(memory)) {
			return 0, false
		}
		w := uint16(memory[pc+2])<<8 | uint16(memory[pc+3])
		target = uint32(int64(pc+2) + int64(int16(w)))
	default:
		return 0, false
	}
	targetEnd := uint64(target) + M68K_WORD_SIZE
	if target&1 != 0 || targetEnd > uint64(topOfRAM) || targetEnd > uint64(len(memory)) {
		return 0, false
	}
	return target, true
}

func (cpu *M68KCPU) m68kChaseStaticJMPTrampolines(pc uint32) (uint32, uint32) {
	const chaseLimit = 8
	topOfRAM := cpu.ProfileTopOfRAM()
	visited := [chaseLimit]uint32{pc}
	visitedCount := 1
	retired := uint32(0)
	for retired < chaseLimit {
		target, ok := m68kStaticJMPTrampolineTarget(cpu.memory, pc, topOfRAM)
		if !ok || target == pc {
			break
		}
		seen := false
		for i := 0; i < visitedCount; i++ {
			if visited[i] == target {
				seen = true
				break
			}
		}
		if seen {
			break
		}
		pc = target
		retired++
		if visitedCount < chaseLimit {
			visited[visitedCount] = pc
			visitedCount++
		}
	}
	return pc, retired
}

// M68KExecuteJIT is the main JIT execution loop for the M68020.
// It replaces ExecuteInstruction() when JIT compilation is enabled.
func (cpu *M68KCPU) M68KExecuteJIT() {
	cpu.m68kResetJITDiagnostics()

	if err := cpu.initM68KJIT(); err != nil {
		fmt.Printf("M68K JIT: %v, falling back to interpreter\n", err)
		cpu.m68kJitFallbackInstructions.Add(1)
		cpu.ExecuteInstruction()
		return
	}
	defer cpu.freeM68KJIT()

	// Mark the dispatcher live so cross-thread bus invalidations enqueue (and
	// are drained on this goroutine) instead of mutating the cache maps from a
	// host goroutine. Cleared on exit so post-run writes invalidate in place.
	cpu.m68kJitDispatchActive.Store(true)
	defer cpu.m68kJitDispatchActive.Store(false)

	enableM68KPollWiring(cpu)

	execMem := cpu.m68kGetJITExecMem()
	ctx := cpu.m68kJitCtx
	disableChains := m68kJITDisableChains()
	disableRTSCache := m68kJITDisableRTSCache()
	disableRegions := m68kJITDisableRegions()
	disableStaticJMPChase := m68kJITDisableStaticJMPChase()
	irqSampleInterval := m68kJITInterruptSampleInterval()
	diagnosticDisabledPCs := m68kJITDiagnosticDisabledPCs()
	diagnosticDisabledPCRanges := m68kJITDiagnosticDisabledPCRanges()
	diagnosticTracePCs := m68kJITDiagnosticTracePCs()
	diagnosticTraceRetRanges := m68kJITDiagnosticTraceRetRanges()
	diagnosticStopRetRanges := m68kJITDiagnosticStopRetRanges()
	diagnosticStopPCRanges := m68kJITDiagnosticStopPCRanges()
	diagnosticTraceLowPC := m68kJITDiagnosticTraceLowPC()
	diagnosticStopA6, diagnosticStopA6OK := m68kJITDiagnosticStopA6Value()
	diagnosticLowPCCount := 0
	verifyMode := os.Getenv("IE_M68K_JIT_VERIFY") == "1"
	verifyStopOnDiverge := os.Getenv("IE_M68K_JIT_VERIFY_STOP") == "1"
	var verifyReported int

	// Initialize performance measurement
	instructionCount := uint64(0)
	if cpu.PerfEnabled {
		cpu.perfStartTime = time.Now()
		cpu.lastPerfReport = cpu.perfStartTime
		cpu.InstructionCount = 0
	}

	// Diagnostic counters
	var diagCacheHits uint64
	var diagCacheMisses uint64
	var diagFallbackInstr uint64
	var diagIOBails uint64

	// A0/R12-across-IRQ corruption tracer (IE_M68K_JIT_DIAG_A0WATCH=1). Tracks the
	// A0 register file slot at each interrupt-sample boundary in the crash window
	// and logs when it flips from a sane small value to garbage, plus each IRQ
	// delivery's context — to localize the async-IRQ/exception-resume corruption
	// of the chained high-RAM memset loop.
	diagA0Watch := m68kJitDiagA0WatchEnabled
	var diagPrevA0 uint32
	var diagPrevPC uint32
	var diagA0Init bool
	var diagA0Dumped bool

	checkPending := func(prevInstructionCount uint64) {
		if diagA0Watch && instructionCount >= 195000000 && instructionCount <= 199000000 {
			a0 := cpu.AddrRegs[0]
			a1 := cpu.AddrRegs[1]
			// Fine per-block trace in the tight window bracketing the first
			// corruption (A0/A1 at every block boundary).
			if instructionCount >= 195179000 && instructionCount <= 195180200 {
				fmt.Printf("[A0WATCH-FINE] @instr=%d PC=%08X A0=%08X A1=%08X SP=%08X SR=%04X\n",
					instructionCount, cpu.PC, a0, a1, cpu.AddrRegs[7], cpu.SR)
			}
			if diagA0Init && a0 != diagPrevA0 && (a0 >= 0x01000000) && diagPrevA0 < 0x01000000 {
				fmt.Printf("[A0WATCH] A0 %08X -> %08X between samples ending @instr=%d PC=%08X(prev=%08X) A1=%08X SP=%08X pending=%X SR=%04X\n",
					diagPrevA0, a0, instructionCount, cpu.PC, diagPrevPC, a1, cpu.AddrRegs[7], cpu.pendingInterrupt.Load(), cpu.SR)
				if !diagA0Dumped && a0 >= 0x5D000000 && a0 < 0x5E000000 {
					diagA0Dumped = true
					fmt.Printf("[A0WATCH] *** first crash-garbage transition; native block ring (newest = corrupting block):\n")
					cpu.m68kDumpNativePCRing()
				}
			}
			diagPrevA0 = a0
			diagPrevPC = cpu.PC
			diagA0Init = true
		}
		if cpu.stopped.Load() || instructionCount/uint64(irqSampleInterval) == prevInstructionCount/uint64(irqSampleInterval) {
			return
		}
		cpu.runInstructionCountHook(instructionCount)
		pendingException := cpu.pendingException.Load()
		pending := cpu.pendingInterrupt.Load()
		if pendingException == 0 && pending == 0 {
			return
		}
		if pendingException != 0 {
			cpu.pendingException.Store(0)
			cpu.ProcessException(uint8(pendingException))
		}

		if pending != 0 {
			ipl := uint32((cpu.SR & M68K_SR_IPL) >> M68K_SR_SHIFT)
			for level := uint32(7); level >= 1; level-- {
				if pending&(1<<level) != 0 && (level > ipl || level == 7) {
					if diagA0Watch && instructionCount >= 195000000 && instructionCount <= 199000000 {
						fmt.Printf("[A0WATCH] deliver L%d @instr=%d PC=%08X A0=%08X SP=%08X SR=%04X\n",
							level, instructionCount, cpu.PC, cpu.AddrRegs[0], cpu.AddrRegs[7], cpu.SR)
					}
					if cpu.ProcessInterrupt(uint8(level)) {
						if diagA0Watch && instructionCount >= 195000000 && instructionCount <= 199000000 {
							fmt.Printf("[A0WATCH] post-deliver L%d PC=%08X A0=%08X SP=%08X SR=%04X\n",
								level, cpu.PC, cpu.AddrRegs[0], cpu.AddrRegs[7], cpu.SR)
						}
						for {
							old := cpu.pendingInterrupt.Load()
							if cpu.pendingInterrupt.CompareAndSwap(old, old&^(1<<level)) {
								break
							}
						}
					}
					break
				}
			}
		}
	}

	for cpu.running.Load() {
		if cpu.debugHandleBreakIn(uint64(cpu.PC)) {
			break
		}
		// ── STOP state handling (replicates cpu_m68k.go:2358-2405) ──
		if cpu.stopped.Load() {
			pendingException := cpu.pendingException.Load()
			if pendingException != 0 {
				cpu.pendingException.Store(0)
				cpu.stopped.Store(false)
				cpu.stopSpinCount.Store(0)
				cpu.ProcessException(uint8(pendingException))
			}

			woke := false
			pending := cpu.pendingInterrupt.Load()
			if pending != 0 {
				ipl := uint32((cpu.SR & M68K_SR_IPL) >> M68K_SR_SHIFT)
				for level := uint32(7); level >= 1; level-- {
					if pending&(1<<level) != 0 && (level > ipl || level == 7) {
						cpu.stopped.Store(false)
						cpu.stopSpinCount.Store(0)
						if cpu.ProcessInterrupt(uint8(level)) {
							woke = true
							for {
								old := cpu.pendingInterrupt.Load()
								if cpu.pendingInterrupt.CompareAndSwap(old, old&^(1<<level)) {
									break
								}
							}
						} else {
							cpu.stopped.Store(true)
						}
						break
					}
				}
			}

			if !woke {
				// Wake STOP-idle: the instruction-count hook is frozen while the
				// guest sits at STOP with nothing pending, so the deterministic
				// IRQ source must be pumped here too. Mirrors the interpreter
				// STOP spin (cpu_m68k.go) — without this the JIT boot path can
				// deadlock at cpu_Dispatch's idle STOP that this hook wakes.
				if cpu.StoppedIdleHook != nil {
					cpu.StoppedIdleHook(cpu)
				}
				spins := cpu.stopSpinCount.Add(1)
				if spins >= 5000 && spins%5000 == 0 {
					cpu.stopWatchdogHits.Add(1)
				}
				// Park the idle CPU instead of busy-spinning Gosched, so the
				// wall-clock VBL ticker and compositor goroutines are not
				// starved during a real AROS boot. Deterministic harness keeps
				// a tight spin (delay 0).
				if d := stopIdleBackoffDelay(spins, cpu.StoppedIdleHook != nil); d > 0 {
					time.Sleep(d)
				} else {
					runtime.Gosched()
				}
			} else {
				runtime.Gosched()
			}
			continue // back to top — do NOT fall through to fetch
		}

		// ── Normal instruction execution ──
		pc := cpu.PC
		if diagnosticTraceLowPC && pc < 0x3000 && diagnosticLowPCCount < 256 {
			fmt.Printf("M68K JIT low pc=%08X op=%04X sr=%04X A0=%08X A1=%08X A2=%08X A3=%08X A4=%08X A5=%08X A6=%08X A7=%08X D0=%08X D1=%08X D2=%08X D3=%08X stack=%04X %04X %04X %04X\n",
				pc, cpu.m68kJITDiagnosticWord(pc), cpu.SR,
				cpu.AddrRegs[0], cpu.AddrRegs[1], cpu.AddrRegs[2], cpu.AddrRegs[3],
				cpu.AddrRegs[4], cpu.AddrRegs[5], cpu.AddrRegs[6], cpu.AddrRegs[7],
				cpu.DataRegs[0], cpu.DataRegs[1], cpu.DataRegs[2], cpu.DataRegs[3],
				cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]),
				cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+2),
				cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+4),
				cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+6))
			diagnosticLowPCCount++
		}

		// Odd PC check
		if pc&1 != 0 {
			cpu.ProcessException(M68K_VEC_ADDRESS_ERROR)
			continue
		}

		if pc >= cpu.ProfileTopOfRAM()-M68K_WORD_SIZE {
			cpu.recordFault(pc, 2, false, 0)
			cpu.ProcessException(M68K_VEC_BUS_ERROR)
			continue
		}

		if cpu.m68kJITTraceSuspiciousExtensionPC(pc, "dispatch") {
			cpu.running.Store(false)
			break
		}
		if len(diagnosticStopPCRanges) != 0 && m68kJITDiagnosticPCInRanges(pc, diagnosticStopPCRanges) {
			cpu.m68kReportJITStopPCRange(pc, nil)
			cpu.running.Store(false)
			break
		}

		if matched, retired := cpu.tryFastM68KMMIOPollLoop(); matched {
			prevInstructionCount := instructionCount
			instructionCount += uint64(retired)
			cpu.InstructionCount = instructionCount
			if !cpu.running.Load() {
				break
			}
			checkPending(prevInstructionCount)
			continue
		}

		if !disableStaticJMPChase {
			if chasedPC, retired := cpu.m68kChaseStaticJMPTrampolines(pc); retired != 0 {
				cpu.PC = chasedPC
				cpu.m68kJitStaticJMPChases.Add(uint64(retired))
				prevInstructionCount := instructionCount
				instructionCount += uint64(retired)
				cpu.InstructionCount = instructionCount
				checkPending(prevInstructionCount)
				if !cpu.running.Load() || cpu.stopped.Load() || cpu.PC != chasedPC {
					continue
				}
				pc = chasedPC
			}
		}

		// Apply any cross-thread (host-goroutine) cache invalidations before
		// looking up / running a block, so we never execute a stale native
		// block whose guest code a host write just changed.
		cpu.m68kDrainPendingJITInvalidations()
		// Snapshot the invalidation generation immediately after draining. If a
		// host goroutine queues a new invalidation while we look up / compile /
		// set up this block, the generation changes and we re-loop (re-drain)
		// instead of entering a block whose guest bytes were just overwritten.
		genAtDispatch := cpu.m68kJitInvalGen.Load()

		// Try cache lookup
		block := cpu.m68kJitCache.Get(uint64(pc))
		if block == nil {
			// Scan block
			instrs := m68kScanBlock(cpu.memory, pc)
			if len(instrs) == 0 {
				cpu.m68kInterpretOne()
				instructionCount++
				cpu.InstructionCount = instructionCount
				diagFallbackInstr++
				cpu.m68kJitFallbackInstructions.Add(1)
				if cpu.m68kJitLockstep != nil {
					cpu.m68kJitLockstep.recordReference(cpu, instructionCount)
				}
				continue
			}

			compileInstrs := instrs
			fullBlockAllowed := !m68kNeedsFallback(instrs) &&
				(cpu.m68kJitForceNative || m68kCanUseProductionNativeBlock(cpu.memory, pc, instrs))
			if !cpu.m68kCanCompileNativePC(pc) {
				fullBlockAllowed = false
			}
			if _, disabled := diagnosticDisabledPCs[pc]; disabled {
				fullBlockAllowed = false
			}
			if m68kJITDiagnosticInstrsTouchRanges(pc, instrs, diagnosticDisabledPCRanges) {
				fullBlockAllowed = false
			}
			if !fullBlockAllowed && !cpu.m68kJitForceNative {
				if prefix := m68kProductionNativePrefix(cpu.memory, pc, instrs); len(prefix) > 0 {
					compileInstrs = prefix
					fullBlockAllowed = true
				}
			}
			if !cpu.m68kCanCompileNativePC(pc) {
				fullBlockAllowed = false
			}
			if _, disabled := diagnosticDisabledPCs[pc]; disabled {
				fullBlockAllowed = false
			}
			if m68kJITDiagnosticInstrsTouchRanges(pc, compileInstrs, diagnosticDisabledPCRanges) {
				fullBlockAllowed = false
			}
			if fullBlockAllowed && irqSampleInterval > 0 && len(compileInstrs) > int(irqSampleInterval) {
				compileInstrs = compileInstrs[:int(irqSampleInterval)]
				fullBlockAllowed = !m68kNeedsFallback(compileInstrs) &&
					(cpu.m68kJitForceNative || m68kCanUseProductionNativeBlock(cpu.memory, pc, compileInstrs))
			}

			// Genuinely unsupported leading instruction (no native prefix was
			// possible). Execute exactly ONE instruction through the interpreter,
			// classify it as unsupported_one, then return to JIT dispatch. Broad
			// interpreter bursts are no longer a production path; the diagnostic
			// burst is opt-in only.
			if !fullBlockAllowed {
				var retired uint64
				if m68kJITDiagBurstFallback() {
					retired = cpu.m68kInterpretFallbackBurst(m68kFallbackBurstUntilInterruptSample(instructionCount))
				} else {
					retired = cpu.m68kFallbackOneInstruction()
					cpu.m68kJitUnsupportedOneExits.Add(1)
				}
				prevInstructionCount := instructionCount
				instructionCount += retired
				cpu.InstructionCount = instructionCount
				diagFallbackInstr += retired
				cpu.m68kJitFallbackInstructions.Add(retired)
				if cpu.m68kJitLockstep != nil && retired == 1 {
					cpu.m68kJitLockstep.recordReference(cpu, instructionCount)
				}
				if !cpu.running.Load() {
					break
				}
				checkPending(prevInstructionCount)
				continue
			}

			if cpu.m68kJITShouldWarmupInterpret(pc) {
				cpu.StepOne()
				prevInstructionCount := instructionCount
				instructionCount++
				cpu.InstructionCount = instructionCount
				cpu.m68kJitWarmupInstructions.Add(1)
				if cpu.m68kJitLockstep != nil {
					cpu.m68kJitLockstep.recordReference(cpu, instructionCount)
				}
				if !cpu.running.Load() {
					break
				}
				checkPending(prevInstructionCount)
				continue
			}

			// Compile block
			var err error
			block, err = m68kCompileBlockWithMem(compileInstrs, pc, execMem, cpu.memory)
			if err != nil {
				cpu.m68kRecordJITCompileFailure(pc, err)
				// Strict mode: emitter/compiler bugs must surface, not hide
				// behind the interpreter. Report and stop the CPU.
				if m68kJITStrictMode() {
					cpu.m68kReportJITStrictFailure(pc, err)
					cpu.running.Store(false)
					break
				}
				// Normal mode: execute exactly one interpreter instruction,
				// record compile_failure, return to JIT dispatch. Never blacklist
				// the block or fall back to an interpreter burst.
				var retired uint64
				if m68kJITDiagBurstFallback() {
					retired = cpu.m68kInterpretFallbackBurst(m68kFallbackBurstUntilInterruptSample(instructionCount))
				} else {
					retired = cpu.m68kFallbackOneInstruction()
					cpu.m68kJitCompileFailureExits.Add(1)
				}
				prevInstructionCount := instructionCount
				instructionCount += retired
				cpu.InstructionCount = instructionCount
				diagFallbackInstr += retired
				cpu.m68kJitFallbackInstructions.Add(retired)
				if cpu.m68kJitLockstep != nil && retired == 1 {
					cpu.m68kJitLockstep.recordReference(cpu, instructionCount)
				}
				if !cpu.running.Load() {
					break
				}
				checkPending(prevInstructionCount)
				continue
			}

			// Cache block and mark code pages
			m68kStampGuestBlockBytes(cpu.memory, block)
			cpu.m68kJitCache.Put(block)
			cpu.m68kMarkJITCodeRanges(block)

			// Patch chain slots bidirectionally:
			// 1. Existing blocks exiting to this block → patch their slots to our chainEntry
			if !disableChains && block.chainEntry != 0 {
				cpu.m68kJitCache.PatchChainsTo(block.startPC, block.chainEntry)
			}
			// 2. This block's exits targeting already-cached blocks → patch our slots.
			//    Chained successors are reached by a direct native jump that bypasses
			//    the dispatcher's per-block byte-stamp revalidation (see the
			//    m68kGuestBlockBytesStillMatch guard before callNative). A chain edge
			//    must therefore never be patched to a target whose guest bytes have
			//    since been overwritten (e.g. AROS relocating code over a previously
			//    compiled address): doing so would execute a stale translation of
			//    unrelated code. Validate the target's stamp here and evict-instead-of-
			//    patch when it no longer matches, so the next dispatch recompiles it.
			if !disableChains {
				for i := range block.chainSlots {
					slot := &block.chainSlots[i]
					target := cpu.m68kJitCache.Get(slot.targetPC)
					if target == nil || target.chainEntry == 0 {
						continue
					}
					if !m68kGuestBlockBytesStillMatch(cpu.memory, target) {
						cpu.m68kInvalidateJITCodeRange(uint32(target.startPC), uint32(target.endPC))
						continue
					}
					PatchRel32At(slot.patchAddr, target.chainEntry)
				}
			}
			diagCacheMisses++
		} else {
			diagCacheHits++
		}

		// Update 8-entry MRU RTS cache: shift entries down and write new
		// to entry 0. When RTS pops a return address, it probes all eight
		// entries for a chain hit before bailing to the Go dispatcher.
		if !disableChains && !disableRTSCache && block.chainEntry != 0 {
			ctx.RTSCache7PC = ctx.RTSCache6PC
			ctx.RTSCache7Addr = ctx.RTSCache6Addr
			ctx.RTSCache6PC = ctx.RTSCache5PC
			ctx.RTSCache6Addr = ctx.RTSCache5Addr
			ctx.RTSCache5PC = ctx.RTSCache4PC
			ctx.RTSCache5Addr = ctx.RTSCache4Addr
			ctx.RTSCache4PC = ctx.RTSCache3PC
			ctx.RTSCache4Addr = ctx.RTSCache3Addr
			ctx.RTSCache3PC = ctx.RTSCache2PC
			ctx.RTSCache3Addr = ctx.RTSCache2Addr
			ctx.RTSCache2PC = ctx.RTSCache1PC
			ctx.RTSCache2Addr = ctx.RTSCache1Addr
			ctx.RTSCache1PC = ctx.RTSCache0PC
			ctx.RTSCache1Addr = ctx.RTSCache0Addr
			ctx.RTSCache0PC = uint32(block.startPC)
			ctx.RTSCache0Addr = block.chainEntry
		}

		// Execute native code block
		ctx.NeedInval = 0
		ctx.InvalAddr = 0
		ctx.InvalSize = 0
		ctx.NeedIOFallback = 0
		ctx.NativeException = 0
		ctx.NeedHelper = m68kJITHelperNone
		remainingToSample := irqSampleInterval - uint32(instructionCount%uint64(irqSampleInterval))
		if remainingToSample == 0 {
			remainingToSample = irqSampleInterval
		}
		ctx.ChainBudget = remainingToSample
		ctx.ChainCount = 0

		// Verifier: run the interpreter over this single block first (then undo),
		// capturing its register result. We force an unchained native run
		// (ChainBudget=0) so native executes exactly this block and we can
		// compare like-for-like.
		verifyActive := false
		// Verify only contiguous per-block (tier==0) blocks. Tier-2 region
		// blocks are non-contiguous (endPC may precede startPC) and are not
		// amenable to the range-bounded interpreter pre-pass.
		if verifyMode && block.tier == 0 && block.startPC <= uint64(^uint32(0)) &&
			block.endPC > block.startPC && block.endPC-block.startPC <= 0x1000 {
			aborted := cpu.m68kVerifyInterpPrePass(uint32(block.startPC), uint32(block.endPC))
			verifyActive = !aborted
			ctx.ChainBudget = 0 // native runs exactly this block
		}
		if _, ok := diagnosticTracePCs[uint32(block.startPC)]; ok {
			fmt.Printf("M68K JIT trace enter pc=%08X A0=%08X A1=%08X A2=%08X A3=%08X A4=%08X A5=%08X A6=%08X A7=%08X D0=%08X D1=%08X D2=%08X D3=%08X D4=%08X D5=%08X D6=%08X D7=%08X\n",
				uint32(block.startPC),
				cpu.AddrRegs[0], cpu.AddrRegs[1], cpu.AddrRegs[2], cpu.AddrRegs[3],
				cpu.AddrRegs[4], cpu.AddrRegs[5], cpu.AddrRegs[6], cpu.AddrRegs[7],
				cpu.DataRegs[0], cpu.DataRegs[1], cpu.DataRegs[2], cpu.DataRegs[3],
				cpu.DataRegs[4], cpu.DataRegs[5], cpu.DataRegs[6], cpu.DataRegs[7])
		}
		// Final cross-thread SMC guard: if a host goroutine queued an
		// invalidation since the post-drain snapshot, the guest bytes backing
		// this block may have changed. Re-loop to drain (which evicts the stale
		// block) and recompile from current memory rather than run it. Closes
		// the store-before-enqueue window down to the few instructions between
		// here and the native entry below.
		if cpu.m68kJitInvalGen.Load() != genAtDispatch {
			continue
		}
		if !m68kGuestBlockBytesStillMatch(cpu.memory, block) {
			cpu.m68kInvalidateJITCodeRange(uint32(block.startPC), uint32(block.endPC))
			continue
		}
		ctx.InvalGenSnapshot = genAtDispatch
		cpu.m68kRecordJITNativePC(uint32(block.startPC))
		cpu.m68kJitNativeActive.Store(true)
		callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
		cpu.m68kJitNativeActive.Store(false)
		cpu.m68kJitNativeBlocksExecuted.Add(1)

		// Read return values from context
		cpu.PC = ctx.RetPC
		if len(diagnosticStopRetRanges) != 0 && m68kJITDiagnosticPCInRanges(ctx.RetPC, diagnosticStopRetRanges) {
			cpu.m68kReportJITStopRetRange(block, ctx)
			cpu.running.Store(false)
			break
		}
		if os.Getenv("IE_M68K_JIT_STOP_ON_BAD_RETPC") == "1" && cpu.PC >= cpu.ProfileTopOfRAM() {
			fmt.Printf("M68K JIT bad RetPC after native block start=%08X ret=%08X top=%08X chain=%d retCount=%d NeedIO=%d Helper=%d Exception=%d Inval=%d A0=%08X A1=%08X A2=%08X A3=%08X A4=%08X A5=%08X A6=%08X A7=%08X D0=%08X D1=%08X D2=%08X D3=%08X D4=%08X D5=%08X D6=%08X D7=%08X stack=%04X %04X %04X %04X\n",
				uint32(block.startPC), ctx.RetPC, cpu.ProfileTopOfRAM(), ctx.ChainCount, ctx.RetCount,
				ctx.NeedIOFallback, ctx.NeedHelper, ctx.NativeException, ctx.NeedInval,
				cpu.AddrRegs[0], cpu.AddrRegs[1], cpu.AddrRegs[2], cpu.AddrRegs[3],
				cpu.AddrRegs[4], cpu.AddrRegs[5], cpu.AddrRegs[6], cpu.AddrRegs[7],
				cpu.DataRegs[0], cpu.DataRegs[1], cpu.DataRegs[2], cpu.DataRegs[3],
				cpu.DataRegs[4], cpu.DataRegs[5], cpu.DataRegs[6], cpu.DataRegs[7],
				cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]),
				cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+2),
				cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+4),
				cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+6))
			readMem := func(addr uint64, size int) []byte {
				if addr+uint64(size) > uint64(len(cpu.memory)) {
					return nil
				}
				return cpu.memory[addr : addr+uint64(size)]
			}
			fmt.Printf("  --- bad-ret block disasm pc=%08X ---\n", uint32(block.startPC))
			for _, line := range disassembleM68K(readMem, block.startPC, 24) {
				fmt.Printf("  %08X: %-16s %s\n", line.Address, line.HexBytes, line.Mnemonic)
			}
			cpu.m68kDumpNativePCRing()
			cpu.running.Store(false)
			break
		}

		// Verifier: compare native result to the interpreter pre-pass. Only when
		// native ran the block to completion (no bail/exception/helper/inval) —
		// those exit paths are not single-block-comparable.
		if verifyActive && ctx.NeedIOFallback == 0 && ctx.NativeException == 0 &&
			ctx.NeedHelper == m68kJITHelperNone && ctx.NeedInval == 0 && verifyReported < 64 {
			// Compare native's result against the interpreter state after the
			// SAME number of retired instructions. Indexing by count (not PC) is
			// robust to loops, where native's exit PC can equal the loop top.
			retired := int(m68kJITRetiredInstructionCount(ctx.RetCount, ctx.ChainCount, block.instrCount, false))
			if exp := cpu.m68kVerifyLookup(retired); exp != nil && exp.pc == cpu.PC {
				diverged := (cpu.SR & 0x1F) != (exp.sr & 0x1F)
				for i := 0; i < 8 && !diverged; i++ {
					if cpu.DataRegs[i] != exp.d[i] || cpu.AddrRegs[i] != exp.a[i] {
						diverged = true
					}
				}
				if diverged {
					verifyReported++
					cpu.m68kReportVerifyDivergence(uint32(block.startPC), uint32(block.endPC), exp)
					if verifyStopOnDiverge {
						cpu.running.Store(false)
						break
					}
				}
			}
		}
		if len(diagnosticTraceRetRanges) != 0 && m68kJITDiagnosticPCInRanges(ctx.RetPC, diagnosticTraceRetRanges) {
			fmt.Printf("M68K JIT trace ret-range block=%08X ret=%08X chain=%d retCount=%d NeedIO=%d Helper=%d Exception=%d Inval=%d A0=%08X A1=%08X A2=%08X A3=%08X A4=%08X A5=%08X A6=%08X A7=%08X D0=%08X D1=%08X D2=%08X D3=%08X D4=%08X D5=%08X D6=%08X D7=%08X stack=%04X %04X %04X %04X\n",
				uint32(block.startPC), ctx.RetPC, ctx.ChainCount, ctx.RetCount,
				ctx.NeedIOFallback, ctx.NeedHelper, ctx.NativeException, ctx.NeedInval,
				cpu.AddrRegs[0], cpu.AddrRegs[1], cpu.AddrRegs[2], cpu.AddrRegs[3],
				cpu.AddrRegs[4], cpu.AddrRegs[5], cpu.AddrRegs[6], cpu.AddrRegs[7],
				cpu.DataRegs[0], cpu.DataRegs[1], cpu.DataRegs[2], cpu.DataRegs[3],
				cpu.DataRegs[4], cpu.DataRegs[5], cpu.DataRegs[6], cpu.DataRegs[7],
				cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]),
				cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+2),
				cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+4),
				cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+6))
		}
		if _, ok := diagnosticTracePCs[uint32(block.startPC)]; ok {
			fmt.Printf("M68K JIT trace exit pc=%08X ret=%08X chain=%d retCount=%d A0=%08X A1=%08X A2=%08X A3=%08X A4=%08X A5=%08X A6=%08X A7=%08X D0=%08X D1=%08X D2=%08X D3=%08X D4=%08X D5=%08X D6=%08X D7=%08X stack=%04X %04X %04X %04X\n",
				uint32(block.startPC), ctx.RetPC, ctx.ChainCount, ctx.RetCount,
				cpu.AddrRegs[0], cpu.AddrRegs[1], cpu.AddrRegs[2], cpu.AddrRegs[3],
				cpu.AddrRegs[4], cpu.AddrRegs[5], cpu.AddrRegs[6], cpu.AddrRegs[7],
				cpu.DataRegs[0], cpu.DataRegs[1], cpu.DataRegs[2], cpu.DataRegs[3],
				cpu.DataRegs[4], cpu.DataRegs[5], cpu.DataRegs[6], cpu.DataRegs[7],
				cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]),
				cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+2),
				cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+4),
				cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+6))
		}
		if os.Getenv("IE_M68K_JIT_STOP_ON_BAD_A3") == "1" {
			a3 := cpu.AddrRegs[3]
			if a3 >= cpu.ProfileTopOfRAM() {
				fmt.Printf("M68K JIT bad A3 after native block start=%08X ret=%08X chain=%d retCount=%d A3=%08X A0=%08X A1=%08X A2=%08X A4=%08X A5=%08X A6=%08X A7=%08X D0=%08X D1=%08X D2=%08X D3=%08X\n",
					uint32(block.startPC), ctx.RetPC, ctx.ChainCount, ctx.RetCount,
					a3, cpu.AddrRegs[0], cpu.AddrRegs[1], cpu.AddrRegs[2], cpu.AddrRegs[4], cpu.AddrRegs[5], cpu.AddrRegs[6], cpu.AddrRegs[7],
					cpu.DataRegs[0], cpu.DataRegs[1], cpu.DataRegs[2], cpu.DataRegs[3])
				cpu.running.Store(false)
				break
			}
		}
		if os.Getenv("IE_M68K_JIT_STOP_ON_BAD_ADDRREG") == "1" {
			top := cpu.ProfileTopOfRAM()
			for _, i := range []int{0, 2} {
				a := cpu.AddrRegs[i]
				if a >= top && a < 0x80000000 {
					fmt.Printf("M68K JIT bad A%d after native block start=%08X ret=%08X chain=%d retCount=%d A0=%08X A1=%08X A2=%08X A3=%08X A4=%08X A5=%08X A6=%08X A7=%08X D0=%08X D1=%08X D2=%08X D3=%08X D4=%08X D5=%08X D6=%08X D7=%08X\n",
						i, uint32(block.startPC), ctx.RetPC, ctx.ChainCount, ctx.RetCount,
						cpu.AddrRegs[0], cpu.AddrRegs[1], cpu.AddrRegs[2], cpu.AddrRegs[3], cpu.AddrRegs[4], cpu.AddrRegs[5], cpu.AddrRegs[6], cpu.AddrRegs[7],
						cpu.DataRegs[0], cpu.DataRegs[1], cpu.DataRegs[2], cpu.DataRegs[3], cpu.DataRegs[4], cpu.DataRegs[5], cpu.DataRegs[6], cpu.DataRegs[7])
					if cpu.AddrRegs[3]+0x90 < cpu.ProfileTopOfRAM() {
						fmt.Printf("  A3 data 80=%08X 84=%08X 88=%08X 8C=%08X\n",
							cpu.Read32(cpu.AddrRegs[3]+0x80),
							cpu.Read32(cpu.AddrRegs[3]+0x84),
							cpu.Read32(cpu.AddrRegs[3]+0x88),
							cpu.Read32(cpu.AddrRegs[3]+0x8C))
					}
					readMem := func(addr uint64, size int) []byte {
						if addr+uint64(size) > uint64(len(cpu.memory)) {
							return nil
						}
						return cpu.memory[addr : addr+uint64(size)]
					}
					instrs := m68kScanBlock(cpu.memory, uint32(block.startPC))
					limit := len(instrs)
					if limit < 16 {
						limit = 16
					}
					fmt.Printf("  --- bad block disasm pc=%08X instrs=%d ---\n", uint32(block.startPC), len(instrs))
					for _, line := range disassembleM68K(readMem, block.startPC, limit) {
						fmt.Printf("  %08X: %-16s %s\n", line.Address, line.HexBytes, line.Mnemonic)
					}
					cpu.m68kDumpNativePCRing()
					cpu.running.Store(false)
					break
				}
			}
			if !cpu.running.Load() {
				break
			}
		}
		if diagnosticStopA6OK && cpu.AddrRegs[6] == diagnosticStopA6 {
			fmt.Printf("M68K JIT stop A6 after native block start=%08X ret=%08X chain=%d retCount=%d A6=%08X A0=%08X A1=%08X A2=%08X A3=%08X A4=%08X A5=%08X A7=%08X D0=%08X D1=%08X D2=%08X D3=%08X D4=%08X D5=%08X D6=%08X D7=%08X stack=%04X %04X %04X %04X\n",
				uint32(block.startPC), ctx.RetPC, ctx.ChainCount, ctx.RetCount,
				cpu.AddrRegs[6], cpu.AddrRegs[0], cpu.AddrRegs[1], cpu.AddrRegs[2], cpu.AddrRegs[3], cpu.AddrRegs[4], cpu.AddrRegs[5], cpu.AddrRegs[7],
				cpu.DataRegs[0], cpu.DataRegs[1], cpu.DataRegs[2], cpu.DataRegs[3], cpu.DataRegs[4], cpu.DataRegs[5], cpu.DataRegs[6], cpu.DataRegs[7],
				cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]),
				cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+2),
				cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+4),
				cpu.m68kJITDiagnosticWord(cpu.AddrRegs[7]+6))
			cpu.running.Store(false)
			break
		}
		exitSignal := ctx.NativeException != 0 || ctx.NeedInval != 0 || ctx.NeedIOFallback != 0 || ctx.NeedHelper != 0
		executed := m68kJITRetiredInstructionCount(ctx.RetCount, ctx.ChainCount, block.instrCount, exitSignal)
		if cpu.m68kJitLockstep != nil {
			boundary := m68kJITLockstepBoundary{
				Count:       instructionCount + executed,
				BlockPC:     uint32(block.startPC),
				RetPC:       ctx.RetPC,
				RetCount:    ctx.RetCount,
				ChainCount:  ctx.ChainCount,
				ChainBudget: ctx.ChainBudget,
				ExitSignal:  exitSignal,
				NeedIO:      ctx.NeedIOFallback,
				NeedHelper:  ctx.NeedHelper,
				Exception:   ctx.NativeException,
				NeedInval:   ctx.NeedInval,
			}
			if !cpu.m68kJitLockstep.compareCandidate(cpu, boundary) {
				if mismatch := cpu.m68kJitLockstep.Mismatch(); mismatch != nil {
					fmt.Print(mismatch.String())
				}
				cpu.running.Store(false)
				break
			}
		}
		cpu.m68kRecordJITNativeRetired(uint32(block.startPC), executed)
		cpu.m68kJitNativeRetCountSum.Add(executed)
		cpu.m68kJitNativeChainCountSum.Add(uint64(ctx.ChainCount))
		if ctx.ChainCount == 0 {
			cpu.m68kJitNativeNoChainReturns.Add(1)
		}

		if ctx.NativeException != 0 {
			cpu.m68kJitNativeExceptionExits.Add(1)
			vector := uint8(ctx.NativeException)
			ctx.NativeException = 0
			cpu.lastExecPC = ctx.NativeExceptionPC
			cpu.lastExecOpcode = uint16(ctx.NativeExceptionIR)
			prevInstructionCount := instructionCount
			instructionCount += executed
			cpu.InstructionCount = instructionCount
			if vector >= M68K_VEC_TRAP_BASE && vector < M68K_VEC_TRAP_BASE+16 {
				cpu.currentIR = uint16(M68K_TRAP | uint16(vector-M68K_VEC_TRAP_BASE))
				cpu.ExecTRAP(cpu.currentIR)
			} else {
				cpu.ProcessException(vector)
			}
			checkPending(prevInstructionCount)
			continue
		}

		didHelper := false
		if ctx.NeedHelper != m68kJITHelperNone {
			cpu.m68kJitNativeHelperExits.Add(1)
			didHelper = true
			if executed != 0 {
				prevInstructionCount := instructionCount
				instructionCount += executed
				executed = 0
				cpu.InstructionCount = instructionCount
				checkPending(prevInstructionCount)
				if !cpu.running.Load() || cpu.stopped.Load() || cpu.PC != ctx.RetPC {
					continue
				}
			}
			prevInstructionCount := instructionCount
			retired, _ := cpu.m68kHandleJITHelper(ctx)
			instructionCount += retired
			cpu.InstructionCount = instructionCount
			if !cpu.running.Load() {
				break
			}
			checkPending(prevInstructionCount)
		}

		// Self-modifying code or external code writes: invalidate cache.
		invalidated := false
		deferredInval := cpu.m68kJitDeferredInval.Swap(false)
		if ctx.NeedInval != 0 || deferredInval {
			cpu.m68kJitNativeInvalExits.Add(1)
			cpu.m68kRecordJITNativeInvalPC(uint32(block.startPC))
			invalAddr := ctx.InvalAddr
			invalSize := ctx.InvalSize
			ctx.NeedInval = 0
			ctx.InvalAddr = 0
			ctx.InvalSize = 0
			if !deferredInval && invalSize != 0 {
				end := uint64(invalAddr) + uint64(invalSize)
				if end > uint64(^uint32(0)) {
					end = uint64(^uint32(0))
				}
				cpu.m68kInvalidateJITCodeRange(invalAddr, uint32(end))
			} else {
				cpu.m68kResetJITCodeCache()
			}
			invalidated = true
		}

		// I/O fallback: re-execute the bailing instruction via interpreter
		didIOBail := false
		if ctx.NeedIOFallback != 0 {
			ctx.NeedIOFallback = 0
			didIOBail = true
			block.ioBails++
			cpu.m68kJitMMIOGuardExits.Add(1)
			if executed != 0 {
				prevInstructionCount := instructionCount
				instructionCount += executed
				executed = 0
				cpu.InstructionCount = instructionCount
				checkPending(prevInstructionCount)
				if !cpu.running.Load() || cpu.stopped.Load() || cpu.PC != ctx.RetPC {
					continue
				}
			}
			prevInstructionCount := instructionCount
			cpu.m68kInterpretOne()
			retired := uint64(1)
			instructionCount += retired // count the re-executed instruction.
			cpu.InstructionCount = instructionCount
			diagIOBails++
			diagFallbackInstr += retired
			cpu.m68kJitBailoutCount.Add(1)
			cpu.m68kJitFallbackInstructions.Add(retired)
			if !cpu.running.Load() {
				break
			}
			checkPending(prevInstructionCount)
		}

		if !invalidated && !didIOBail && !didHelper {
			m68kBumpJITBlockHotness(block, m68kNativeHotnessIncrement(block, ctx.ChainCount))
			if !disableRegions {
				block = cpu.m68kTryPromoteJITRegion(block, execMem, cpu.memory, disableChains)
			}
		}

		prevInstructionCount := instructionCount
		instructionCount += executed
		cpu.InstructionCount = instructionCount

		// ── Interrupt/exception checking (replicates cpu_m68k.go:2457-2485) ──
		if cpu.stopped.Load() {
			continue // defer to STOP handler at top
		}
		checkPending(prevInstructionCount)

		// Performance monitoring
		if cpu.PerfEnabled {
			cpu.InstructionCount = instructionCount
			now := time.Now()
			if now.Sub(cpu.lastPerfReport) >= time.Second {
				elapsed := now.Sub(cpu.perfStartTime).Seconds()
				mips := float64(instructionCount) / elapsed / 1_000_000
				hitRate := float64(0)
				if diagCacheHits+diagCacheMisses > 0 {
					hitRate = float64(diagCacheHits) / float64(diagCacheHits+diagCacheMisses) * 100
				}
				fallbackPct := float64(0)
				if instructionCount > 0 {
					fallbackPct = float64(diagFallbackInstr) / float64(instructionCount) * 100
				}
				fmt.Printf("\rM68K JIT: %.2f MIPS | cache %.0f%% | fallback %.1f%% | io %d   ",
					mips, hitRate, fallbackPct, diagIOBails)
				cpu.lastPerfReport = now
			}
		}
	}

	if cpu.PerfEnabled {
		cpu.InstructionCount = instructionCount
	}
}
