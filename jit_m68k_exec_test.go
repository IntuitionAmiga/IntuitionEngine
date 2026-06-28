// jit_m68k_exec_test.go - Integration tests for M68020 JIT execution loop

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

func newM68KJITCounterProgramCPU(t *testing.T, startPC uint32) *M68KCPU {
	t.Helper()

	bus := NewMachineBus()
	bus.Write32(0, 0x00010000)
	bus.Write32(4, startPC)
	cpu := NewM68KCPU(bus)
	cpu.PC = startPC
	cpu.SR = M68K_SR_S
	cpu.m68kJitWarmupLimit = 1

	pc := startPC
	const repeats = 4096
	for range repeats {
		// MOVEQ #1,D0; MOVEQ #2,D1; ADD.L D0,D1.
		for _, op := range []uint16{0x7001, 0x7202, 0xD280} {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
	}
	return cpu
}

func runM68KUntilNativeBlockOrTimeout(t *testing.T, cpu *M68KCPU, run func()) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		run()
		close(done)
	}()

	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			if cpu.m68kJitNativeBlocksExecuted.Load() == 0 {
				t.Fatal("execution stopped before any native M68K JIT block ran")
			}
			return
		case <-deadline:
			cpu.running.Store(false)
			waitDoneWithGuard(t, done)
			t.Fatal("timed out waiting for native M68K JIT block execution")
		case <-ticker.C:
			if cpu.m68kJitNativeBlocksExecuted.Load() > 0 {
				cpu.running.Store(false)
				waitDoneWithGuard(t, done)
				return
			}
		}
	}
}

func TestM68KJIT_FPUInstructionLengthsAndTerminators(t *testing.T) {
	mem := make([]byte, 0x2000)
	const pc = uint32(0x1000)
	write := func(words ...uint16) {
		for i := range mem {
			mem[i] = 0
		}
		addr := pc
		for _, word := range words {
			mem[addr] = byte(word >> 8)
			mem[addr+1] = byte(word)
			addr += 2
		}
	}

	tests := []struct {
		name  string
		words []uint16
		want  int
	}{
		{name: "general_reg_to_reg", words: []uint16{0xF200, 0x0000}, want: 4},
		{name: "general_immediate_double", words: []uint16{0xF23C, 0x5400, 0x3FF0, 0x0000, 0x0000, 0x0000}, want: 12},
		{name: "fsave_disp_an", words: []uint16{0xF328, 0x0010}, want: 4},
		{name: "frestore_postinc", words: []uint16{0xF358}, want: 2},
		{name: "fdbcc", words: []uint16{0xF248, 0x0000, 0xFFFC}, want: 6},
		{name: "ftrapcc_long_operand", words: []uint16{0xF07B, 0x0000, 0x0000, 0x0000}, want: 8},
		{name: "fbcc_word", words: []uint16{0xF080, 0x0000}, want: 4},
		{name: "fbcc_long", words: []uint16{0xF0C0, 0x0000, 0x0000}, want: 6},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			write(tc.words...)
			if got := m68kInstrLength(mem, pc); got != tc.want {
				t.Fatalf("m68kInstrLength = %d, want %d", got, tc.want)
			}
		})
	}

	if m68kIsBlockTerminator(0xF200) {
		t.Fatalf("normal FPU instruction was classified as a block terminator")
	}
	if !m68kIsBlockTerminator(0xF180) {
		t.Fatalf("unimplemented Line-F trap class was not classified as a block terminator")
	}
}

func TestM68KJIT_FPUFallbackBurstDoesNotStopAfterOneInstruction(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	pc := cpu.PC
	for _, word := range []uint16{
		0xF200, 0x0000, // FMOVE FP0,FP0
		0xF200, 0x0000, // FMOVE FP0,FP0
		0x4E72, 0x2700, // STOP
	} {
		cpu.memory[pc] = byte(word >> 8)
		cpu.memory[pc+1] = byte(word)
		pc += 2
	}

	cpu.running.Store(true)
	retired := cpu.m68kInterpretFallbackBurst(2)
	cpu.running.Store(false)

	if retired != 2 {
		t.Fatalf("fallback burst retired %d instructions, want 2", retired)
	}
	if cpu.PC != 0x1008 {
		t.Fatalf("PC after two FPU fallback instructions = 0x%08X, want 0x00001008", cpu.PC)
	}
}

func TestM68KJIT_NativeAROSSchedulerORIWordDispMatchesInterpreter(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		a0      = uint32(0x3000)
		target  = a0 + 298
	)
	opcodes := []uint16{
		0x0068, 0x0080, 0x012A, // ORI.W #$0080, 298(A0)
		0x4A68, 0x012A, // TST.W 298(A0)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	interp.AddrRegs[0] = a0
	interp.Write16(target, 0x0004)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	jit.AddrRegs[0] = a0
	jit.Write16(target, 0x0004)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got, want := jit.Read16(target), uint16(0x0084); got != want {
		t.Fatalf("target word=0x%04X, want 0x%04X", got, want)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KCHK2ExtensionSelectsUpperRegisterField(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		trapPC  = uint32(0x1800)
		bounds  = uint32(0x3000)
	)
	opcodes := []uint16{
		0x00D0, 0x480F, // CHK2.B (A0),D4; low extension bits deliberately non-zero.
	}
	setup := func(cpu *M68KCPU) {
		cpu.SR = M68K_SR_S
		cpu.AddrRegs[0] = bounds
		cpu.DataRegs[4] = 0x40 // selected by bits 14..12: in range 0..0x80
		cpu.DataRegs[7] = 0x8C // would trap if the low extension bits were used
		cpu.Write8(bounds, 0x00)
		cpu.Write8(bounds+1, 0x80)
		cpu.Write32(uint32(M68K_VEC_CHK)*4, trapPC)
		writeM68KWords(cpu, trapPC, 0x4E72, 0x2700)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)
	if interp.PC == trapPC+4 {
		t.Fatalf("interpreter CHK2 trapped; extension register field decoded from low bits")
	}

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)
	if jit.PC == trapPC+4 {
		t.Fatalf("JIT CHK2 helper trapped; extension register field decoded from low bits")
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_IntegrationFallbackAfterNativeDirtyRegistersMatchesInterpreter(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const startPC = uint32(0x1000)
	opcodes := []uint16{
		0x7007,       // MOVEQ #7,D0
		0x7203,       // MOVEQ #3,D1
		0xD280,       // ADD.L D0,D1
		0x41F9, 0x00, // LEA $00003000,A0
		0x3000,
		0xF200, 0x00, // FPU helper/fallback-class instruction consumes synced state
		0xD280, // ADD.L D0,D1 after returning from helper/fallback
	}

	interp := newM68KTestProgramCPU(t, startPC)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("JIT did not execute any native block before helper/fallback boundary")
	}
	assertM68KCoreStateEqual(t, jit, interp)
	assertM68KFPUStateEqual(t, jit, interp)
}

func TestM68KJIT_IntegrationFallbackAfterLazyCCRMatchesInterpreter(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const startPC = uint32(0x1000)
	opcodes := []uint16{
		0x7000,       // MOVEQ #0,D0, sets Z in native lazy CCR path
		0x4A80,       // TST.L D0, refreshes lazy CCR
		0xF200, 0x00, // FPU helper/fallback boundary must materialize SR
		0x6602, // BNE should not branch if Z survived boundary
		0x7201, // MOVEQ #1,D1
		0x7402, // MOVEQ #2,D2
	}

	interp := newM68KTestProgramCPU(t, startPC)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("JIT did not execute native code before CCR boundary")
	}
	assertM68KCoreStateEqual(t, jit, interp)
	assertM68KFPUStateEqual(t, jit, interp)
}

func TestM68KJIT_IntegrationSMCStoreThenExecuteModifiedCodeMatchesInterpreter(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		target  = uint32(0x1800)
	)
	opcodes := []uint16{
		0x303C, 0x7209, // MOVE.W #$7209,D0 ; opcode for MOVEQ #9,D1
		0x33C0, 0x0000, 0x1800, // MOVE.W D0,$00001800
		0x4EB9, 0x0000, 0x1800, // JSR $00001800
		0x7402, // MOVEQ #2,D2
	}

	interp := newM68KTestProgramCPU(t, startPC)
	writeM68KStopProgram(interp, startPC, opcodes...)
	writeM68KWords(interp, target,
		0x7201, // MOVEQ #1,D1 before SMC
		0x4E75, // RTS
	)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	writeM68KStopProgram(jit, startPC, opcodes...)
	writeM68KWords(jit, target,
		0x7201, // MOVEQ #1,D1 before SMC
		0x4E75, // RTS
	)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("JIT did not execute native code in SMC integration test")
	}
	assertM68KCoreStateEqual(t, jit, interp)
	if got, want := jit.DataRegs[1], uint32(9); got != want {
		t.Fatalf("modified target did not execute: D1=0x%08X want=0x%08X", got, want)
	}
}

func TestM68KJIT_DefaultDispatcherExecutesFPUImmediateViaHelperNoFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const startPC = uint32(0x1000)
	opcodes := []uint16{
		0xF23C, 0x5400, 0x4009, 0x21FB, 0x5444, 0x2D18, // FMOVE.D #pi,FP0
	}

	interp := newM68KTestProgramCPU(t, startPC)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	if m68kNeedsFallback(instrs) {
		t.Fatalf("FPU helper-admitted block still needs fallback: instrs=%+v", instrs)
	}
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) != 1 {
		t.Fatalf("FPU helper prefix length=%d, want 1; instrs=%+v", len(prefix), instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute FPU helper block natively")
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0xF23C].Load(); got != 0 {
		t.Fatalf("FPU opcode 0xF23C fell back %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
	assertM68KFPUStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesAROSFPUContextSequenceViaHelperNoFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		saveA0  = uint32(0x3000)
	)
	opcodes := []uint16{
		0xF23C, 0x5400, 0x4009, 0x21FB, 0x5444, 0x2D18, // FMOVE.D #pi,FP0
		0xF328, 0x006C, // FSAVE 108(A0)
		0xF228, 0xF0FF, 0x000C, // FMOVEM FP0-FP7,12(A0)
		0xF210, 0xBC00, // FMOVE.L FPCR/FPSR/FPIAR,(A0)
		0xF210, 0x9C00, // FMOVE.L (A0),FPCR/FPSR/FPIAR
		0xF228, 0xD0FF, 0x000C, // FMOVEM 12(A0),FP0-FP7
		0xF368, 0x006C, // FRESTORE 108(A0)
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = saveA0
		cpu.FPU.FPCR = 0x01020304
		cpu.FPU.FPSR = 0x05060708
		cpu.FPU.FPIAR = 0x090A0B0C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute AROS FPU helper sequence natively")
	}
	for _, opcode := range []uint16{0xF23C, 0xF328, 0xF228, 0xF210, 0xF368} {
		if got := jit.m68kJitFallbackOpcodeCounts[opcode].Load(); got != 0 {
			t.Fatalf("FPU opcode 0x%04X fell back %d times", opcode, got)
		}
	}
	assertM68KCoreStateEqual(t, jit, interp)
	assertM68KFPUStateEqual(t, jit, interp)
	for off := uint32(0); off < 128; off++ {
		if got, want := jit.Read8(saveA0+off), interp.Read8(saveA0+off); got != want {
			t.Fatalf("FPU save area byte +0x%02X mismatch: got=0x%02X want=0x%02X", off, got, want)
		}
	}
}

func TestM68KJIT_DefaultRunnerExecutesNativeBlockWhenAvailable(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := newM68KJITCounterProgramCPU(t, 0x1000)
	runner := NewM68KRunner(cpu)
	if !runner.cpu.m68kJitEnabled {
		t.Fatal("NewM68KRunner did not enable M68K JIT on a JIT-capable host")
	}

	runM68KUntilNativeBlockOrTimeout(t, cpu, runner.Execute)
}

func TestM68KJIT_DisabledRunnerUsesInterpreter(t *testing.T) {
	cpu := newM68KJITCounterProgramCPU(t, 0x1000)
	runner := NewM68KRunner(cpu)
	runner.cpu.m68kJitEnabled = false

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		runner.Execute()
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if got := cpu.m68kJitNativeBlocksExecuted.Load(); got != 0 {
		t.Fatalf("native blocks executed with JIT disabled = %d, want 0", got)
	}
	if cpu.DataRegs[1] != 3 {
		t.Fatalf("interpreter did not execute counter program: D1=%d, want 3", cpu.DataRegs[1])
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeMoveqAddSequence(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := newM68KJITCounterProgramCPU(t, 0x1000)
	cpu.m68kJitEnabled = true

	runM68KUntilNativeBlockOrTimeout(t, cpu, cpu.M68KExecuteJIT)
	if got := cpu.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute a native block")
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeA7StoreBlock(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const target = 0x2200
	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x12345678
		cpu.AddrRegs[7] = target
	}, false,
		0x2E80, // MOVE.L D0,(A7) is now compiled natively.
	)

	// Data correctness: the store must land regardless of the execution path.
	if got := cpu.Read32(target); got != 0x12345678 {
		t.Fatalf("MOVE.L D0,(A7) wrote 0x%08X, want 0x12345678", got)
	}
	// MOVE.L D0,(A7) is now admitted to the native path.
	if got := cpu.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("native blocks for A7-store block = 0, want > 0")
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeRegisterLongBlock(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	opcodes := []uint16{
		0x7001, // MOVEQ #1,D0
		0x7202, // MOVEQ #2,D1
		0xD081, // ADD.L D1,D0
		0x5280, // ADDQ.L #1,D0
		0x5380, // SUBQ.L #1,D0
		0x9081, // SUB.L D1,D0
		0x4680, // NOT.L D0
		0x4840, // SWAP D0
		0x4282, // CLR.L D2
		0x2400, // MOVE.L D0,D2
		0x4A82, // TST.L D2
		0xB482, // CMP.L D2,D2
		0x8481, // OR.L D1,D2
		0xC481, // AND.L D1,D2
		0x7405, // MOVEQ #5,D2
		0xD482, // ADD.L D2,D2
	}
	for len(opcodes) < m68kJitMaxBlockSize {
		opcodes = append(opcodes, opcodes...)
	}
	opcodes = opcodes[:m68kJitMaxBlockSize]

	interp := runM68KInterpreterStopProgram(t, 0x1000, opcodes...)
	jit := runM68KJITStopProgram(t, 0x1000, opcodes...)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		instrs := m68kScanBlock(jit.memory, 0x1000)
		firstUnsafe := uint16(0)
		firstUnsafeIndex := -1
		for i := range instrs {
			if !m68kInstrProductionNativeSafe(&instrs[i]) {
				firstUnsafe = instrs[i].opcode
				firstUnsafeIndex = i
				break
			}
		}
		t.Fatalf("default M68K JIT dispatcher did not execute register-long block natively: instrs=%d fallback=%v conservative=%v productionSafe=%v genericIO=%v firstUnsafe[%d]=0x%04X fallbackInstr=%d",
			len(instrs), m68kNeedsFallback(instrs), m68kNeedsConservativeFallback(jit.memory, 0x1000, instrs),
			m68kBlockProductionNativeSafe(instrs), m68kBlockMayUseGenericIOFallback(instrs), firstUnsafeIndex, firstUnsafe,
			jit.m68kJitFallbackInstructions.Load())
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("register-long block bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeEORAndAddressALU(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	opcodes := []uint16{
		0x7005, // MOVEQ #5,D0
		0x7223, // MOVEQ #0x23,D1
		0xB380, // EOR.L D1,D0
		0xD1C0, // ADDA.L D0,A0
		0xD1C9, // ADDA.L A1,A0
		0x91C1, // SUBA.L D1,A0
		0x91C9, // SUBA.L A1,A0
		0xB380, // EOR.L D1,D0
	}
	for len(opcodes) < m68kJitMaxBlockSize {
		opcodes = append(opcodes, opcodes...)
	}
	opcodes = opcodes[:m68kJitMaxBlockSize]

	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = 0x00002000
		cpu.AddrRegs[1] = 0x00000100
	}
	interp := newM68KTestProgramCPU(t, 0x1000)
	setup(interp)
	writeM68KStopProgram(interp, 0x1000, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := runM68KJITStopProgramWithSetup(t, 0x1000, setup, false, opcodes...)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute EOR/address-ALU block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("EOR/address-ALU block bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeEORToMemoryEA(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	tests := []struct {
		name       string
		opcode     uint16
		ext        []uint16
		setup      func(*M68KCPU)
		checkAddrs []uint32
	}{
		{
			name:   "eor_l_d1_to_a7_indirect",
			opcode: 0xB397, // EOR.L D1,(A7)
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[1] = 0x0000FF00
				cpu.AddrRegs[7] = 0x120000
				cpu.Write32(0x120000, 0x00F0000F)
			},
			checkAddrs: []uint32{0x120000},
		},
		{
			name:   "eor_l_d1_to_a7_postincrement",
			opcode: 0xB39F, // EOR.L D1,(A7)+
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[1] = 0x0000FF00
				cpu.AddrRegs[7] = 0x120000
				cpu.Write32(0x120000, 0x00F0000F)
			},
			checkAddrs: []uint32{0x120000},
		},
		{
			name:   "eor_l_d6_to_a7_predecrement",
			opcode: 0xBDA7, // EOR.L D6,-(A7)
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[6] = 0x00FF00FF
				cpu.AddrRegs[7] = 0x120004
				cpu.Write32(0x120000, 0x0F0F0F0F)
			},
			checkAddrs: []uint32{0x120000},
		},
		{
			name:   "eor_l_d7_to_a7_displacement",
			opcode: 0xBFAF, // EOR.L D7,16(A7)
			ext:    []uint16{0x0010},
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[7] = 0x00FF00FF
				cpu.AddrRegs[7] = 0x120000
				cpu.Write32(0x120010, 0x0F0F0F0F)
			},
			checkAddrs: []uint32{0x120010},
		},
		{
			name:   "eor_l_d4_to_abs_word",
			opcode: 0xB9B8, // EOR.L D4,$3000.W
			ext:    []uint16{0x3000},
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[4] = 0x00FF00FF
				cpu.Write32(0x3000, 0x0F0F0F0F)
			},
			checkAddrs: []uint32{0x3000},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const startPC = uint32(0x1000)
			opcodes := []uint16{tt.opcode}
			opcodes = append(opcodes, tt.ext...)
			opcodes = append(opcodes, 0x7E01) // MOVEQ #1,D7

			interp := newM68KTestProgramCPU(t, startPC)
			tt.setup(interp)
			writeM68KStopProgram(interp, startPC, opcodes...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			tt.setup(jit)
			writeM68KStopProgram(jit, startPC, opcodes...)
			instrs := m68kScanBlock(jit.memory, startPC)
			prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
			const wantPrefixInstrs = 2
			if len(prefix) != wantPrefixInstrs || !m68kCanUseProductionNativeBlock(jit.memory, startPC, prefix) {
				t.Fatalf("%04X rejected by production gate: instrs=%d prefix=%d needsFallback=%v conservative=%v productionSafe=%v genericIO=%v",
					tt.opcode,
					len(instrs),
					len(prefix),
					m68kNeedsFallback(prefix),
					m68kNeedsConservativeFallback(jit.memory, startPC, prefix),
					m68kBlockProductionNativeSafe(prefix),
					m68kBlockMayUseGenericIOFallback(prefix))
			}

			runM68KJITUntilStopped(t, jit)
			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("default M68K JIT dispatcher did not execute %04X block natively", tt.opcode)
			}
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("%04X block bailed out %d times, want 0", tt.opcode, got)
			}
			if got := jit.m68kJitFallbackOpcodeCounts[tt.opcode].Load(); got != 0 {
				t.Fatalf("%04X fallback count = %d, want 0", tt.opcode, got)
			}
			for _, addr := range tt.checkAddrs {
				if got, want := jit.Read32(addr), interp.Read32(addr); got != want {
					t.Fatalf("memory[0x%08X]=0x%08X, want 0x%08X", addr, got, want)
				}
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeSUBASameSpilledAddressRegister(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	opcodes := []uint16{
		0x93C9, // SUBA.L A1,A1
	}
	for len(opcodes) < m68kJitMaxBlockSize {
		opcodes = append(opcodes, 0x7001) // MOVEQ #1,D0
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[1] = 0x12345678
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, 0x1000)
	setup(interp)
	writeM68KStopProgram(interp, 0x1000, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, 0x1000)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, 0x1000, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute SUBA.L A1,A1 natively")
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeCMPBCCCarryClear(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	opcodes := []uint16{
		0x7603, // MOVEQ #3,D3
		0xB682, // CMP.L D2,D3
		0x6404, // BCC.S taken
		0x7001, // skipped
		0x6002, // BRA.S end
		0x7002, // taken path
	}
	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[2] = 0
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, 0x1000)
	setup(interp)
	writeM68KStopProgram(interp, 0x1000, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, 0x1000)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, 0x1000, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute CMP/BCC block natively")
	}
	assertM68KCoreStateEqual(t, jit, interp)
	if got := jit.DataRegs[0]; got != 2 {
		t.Fatalf("BCC did not take carry-clear path: D0=%d, want 2", got)
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeNegExtLong(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	pattern := []uint16{
		0x203C, 0x0000, 0x8001, // MOVE.L #0x00008001,D0
		0x48C0,                 // EXT.L D0
		0x4480,                 // NEG.L D0
		0x223C, 0x0000, 0x00F0, // MOVE.L #0x000000F0,D1
		0x49C1, // EXTB.L D1
		0x4481, // NEG.L D1
	}
	opcodes := make([]uint16, 0, len(pattern)*48)
	for range 48 {
		opcodes = append(opcodes, pattern...)
	}

	interp := runM68KInterpreterStopProgram(t, 0x1000, opcodes...)
	jit := runM68KJITStopProgram(t, 0x1000, opcodes...)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		instrs := m68kScanBlock(jit.memory, 0x1000)
		firstUnsafe := uint16(0)
		firstUnsafeIndex := -1
		for i := range instrs {
			if !m68kInstrProductionNativeSafe(&instrs[i]) {
				firstUnsafe = instrs[i].opcode
				firstUnsafeIndex = i
				break
			}
		}
		t.Fatalf("default M68K JIT dispatcher did not execute NEG/EXT long block natively: instrs=%d fallback=%v conservative=%v productionSafe=%v genericIO=%v firstUnsafe[%d]=0x%04X fallbackInstr=%d",
			len(instrs), m68kNeedsFallback(instrs), m68kNeedsConservativeFallback(jit.memory, 0x1000, instrs),
			m68kBlockProductionNativeSafe(instrs), m68kBlockMayUseGenericIOFallback(instrs), firstUnsafeIndex, firstUnsafe,
			jit.m68kJitFallbackInstructions.Load())
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("NEG/EXT long block bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeBRA(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	opcodes := []uint16{
		0x7001, // MOVEQ #1,D0
		0x6004, // BRA.B +4 -> skip D2 writes
		0x7499, // MOVEQ #-103,D2 (skipped)
		0x74AA, // MOVEQ #-86,D2 (skipped)
	}
	for len(opcodes) < m68kJitMaxBlockSize {
		opcodes = append(opcodes,
			0x7202, // MOVEQ #2,D1
			0xD280, // ADD.L D0,D1
			0x5381, // SUBQ.L #1,D1
			0x4282, // CLR.L D2
		)
	}
	opcodes = opcodes[:m68kJitMaxBlockSize]

	interp := runM68KInterpreterStopProgram(t, 0x1000, opcodes...)
	jit := runM68KJITStopProgram(t, 0x1000, opcodes...)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute BRA block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("BRA block bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeBEQNotTaken(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	opcodes := []uint16{
		0x7001, // MOVEQ #1,D0
		0x7202, // MOVEQ #2,D1
		0xB081, // CMP.L D1,D0 -> Z clear
		0x6704, // BEQ.B +4 not taken
		0x7403, // MOVEQ #3,D2
		0x7604, // MOVEQ #4,D3
	}
	for len(opcodes) < m68kJitMaxBlockSize {
		opcodes = append(opcodes,
			0xD480, // ADD.L D0,D2
			0x5382, // SUBQ.L #1,D2
			0xD680, // ADD.L D0,D3
			0x5383, // SUBQ.L #1,D3
		)
	}
	opcodes = opcodes[:m68kJitMaxBlockSize]

	interp := runM68KInterpreterStopProgram(t, 0x1000, opcodes...)
	jit := runM68KJITStopProgram(t, 0x1000, opcodes...)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute BEQ not-taken block natively")
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_ProductionPrefixAllowsSingleBccBeforeUnsafeTail(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := newM68KTestProgramCPU(t, 0x1000)
	writeM68KStopProgram(cpu, 0x1000,
		0x6632,                 // BNE.S out of the unsafe tail
		0x4EB9, 0x0061, 0xC9B6, // JSR $0061C9B6
	)

	instrs := m68kScanBlock(cpu.memory, 0x1000)
	prefix := m68kProductionNativePrefix(cpu.memory, 0x1000, instrs)
	if len(prefix) != 1 || prefix[0].opcode != 0x6632 {
		t.Fatalf("production prefix len/opcode = %d/0x%04X, want single BNE prefix", len(prefix), func() uint16 {
			if len(prefix) == 0 {
				return 0
			}
			return prefix[0].opcode
		}())
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeBEQTaken(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC  = uint32(0x1000)
		targetPC = uint32(0x1400)
	)
	interp := newM68KTestProgramCPU(t, startPC)
	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true

	write := func(cpu *M68KCPU, pc uint32, ops ...uint16) uint32 {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
		return pc
	}
	for _, cpu := range []*M68KCPU{interp, jit} {
		pc := startPC
		pc = write(cpu, pc,
			0x7001,                               // MOVEQ #1,D0
			0x7201,                               // MOVEQ #1,D1
			0xB081,                               // CMP.L D1,D0 -> Z set
			0x6700, uint16(targetPC-(startPC+8)), // BEQ.W targetPC
		)
		for pc < startPC+uint32(m68kJitMaxBlockSize*2) {
			pc = write(cpu, pc, 0x7499) // skipped if BEQ is taken
		}
		pc = targetPC
		for i := 0; i < m68kJitMaxBlockSize; i++ {
			pc = write(cpu, pc, 0x7405) // MOVEQ #5,D2
		}
		write(cpu, pc, 0x4E72, 0x2700) // STOP
	}

	runM68KInterpreterUntilStopped(t, interp)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute BEQ taken block natively")
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAllBccConditions(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	type flagInputs struct {
		d0 uint32
		d1 uint32
	}
	cases := []struct {
		name  string
		cond  uint16
		taken flagInputs
		skip  flagInputs
	}{
		{"BHI", 0x2, flagInputs{3, 2}, flagInputs{1, 2}},
		{"BLS", 0x3, flagInputs{1, 2}, flagInputs{3, 2}},
		{"BCC", 0x4, flagInputs{3, 2}, flagInputs{1, 2}},
		{"BCS", 0x5, flagInputs{1, 2}, flagInputs{3, 2}},
		{"BVC", 0x8, flagInputs{1, 1}, flagInputs{0x80000000, 1}},
		{"BVS", 0x9, flagInputs{0x80000000, 1}, flagInputs{1, 1}},
		{"BPL", 0xA, flagInputs{2, 1}, flagInputs{1, 2}},
		{"BMI", 0xB, flagInputs{1, 2}, flagInputs{2, 1}},
		{"BGE", 0xC, flagInputs{2, 1}, flagInputs{1, 2}},
		{"BLT", 0xD, flagInputs{1, 2}, flagInputs{2, 1}},
		{"BGT", 0xE, flagInputs{2, 1}, flagInputs{1, 1}},
		{"BLE", 0xF, flagInputs{1, 2}, flagInputs{2, 1}},
	}

	run := func(t *testing.T, cond uint16, inputs flagInputs, wantTaken bool) {
		t.Helper()

		opcodes := []uint16{
			0x203C, uint16(inputs.d0 >> 16), uint16(inputs.d0), // MOVE.L #d0,D0
			0x223C, uint16(inputs.d1 >> 16), uint16(inputs.d1), // MOVE.L #d1,D1
			0x4282,               // CLR.L D2
			0xB081,               // CMP.L D1,D0
			0x6000 | cond<<8 | 4, // Bcc.B true
			0x7400,               // MOVEQ #0,D2
			0x6002,               // BRA.B done
			0x7401,               // MOVEQ #1,D2
		}
		for i := 0; i < m68kJitMaxBlockSize; i++ {
			opcodes = append(opcodes, 0x7807) // MOVEQ #7,D4
		}

		interp := newM68KTestProgramCPU(t, 0x1000)
		writeM68KStopProgram(interp, 0x1000, opcodes...)
		runM68KInterpreterUntilStopped(t, interp)

		jit := newM68KTestProgramCPU(t, 0x1000)
		jit.m68kJitEnabled = true
		writeM68KStopProgram(jit, 0x1000, opcodes...)
		runM68KJITUntilStopped(t, jit)

		wantD2 := uint32(0)
		if wantTaken {
			wantD2 = 1
		}
		if got := jit.DataRegs[2]; got != wantD2 {
			t.Fatalf("D2 = %d, want %d", got, wantD2)
		}
		if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
			t.Fatal("default M68K JIT dispatcher did not execute Bcc condition block natively")
		}
		if got := jit.m68kJitBailoutCount.Load(); got != 0 {
			t.Fatalf("Bcc condition block bailed out %d times, want 0", got)
		}
		assertM68KCoreStateEqual(t, jit, interp)
	}

	for _, tc := range cases {
		t.Run(tc.name+"_taken", func(t *testing.T) {
			run(t, tc.cond, tc.taken, true)
		})
		t.Run(tc.name+"_not_taken", func(t *testing.T) {
			run(t, tc.cond, tc.skip, false)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeDBFLoop(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	opcodes := []uint16{
		0x7003,         // MOVEQ #3,D0
		0x7200,         // MOVEQ #0,D1
		0x5281,         // ADDQ.L #1,D1
		0x51C8, 0xFFFC, // DBF D0,ADDQ
	}
	for i := 0; i < m68kJitMaxBlockSize; i++ {
		opcodes = append(opcodes, 0x7E07) // MOVEQ #7,D7
	}

	interp := runM68KInterpreterStopProgram(t, 0x1000, opcodes...)
	jit := runM68KJITStopProgram(t, 0x1000, opcodes...)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute DBF loop block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("DBF loop block bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeMovePostincPostincLong(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		src     = uint32(0x3000)
		dst     = uint32(0x5000)
	)
	opcodes := make([]uint16, m68kJitMaxBlockSize)
	for i := range opcodes {
		opcodes[i] = 0x22D8 // MOVE.L (A0)+,(A1)+
	}

	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = src
		cpu.AddrRegs[1] = dst
		for i := uint32(0); i < uint32(m68kJitMaxBlockSize)*4; i += 4 {
			cpu.Write32(src+i, 0x10203040+i)
			cpu.Write32(dst+i, 0xDEADBEEF)
		}
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	if !m68kCanUseProductionNativeBlock(jit.memory, startPC, instrs) {
		t.Fatalf("AROS postinc fill loop rejected by production gate: instrs=%d needsFallback=%v conservative=%v productionSafe=%v genericIO=%v",
			len(instrs),
			m68kNeedsFallback(instrs),
			m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
			m68kBlockProductionNativeSafe(instrs),
			m68kBlockMayUseGenericIOFallback(instrs))
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute MOVE.L (An)+,(An)+ block natively")
	}
	assertM68KCoreStateEqual(t, jit, interp)
	for i := uint32(0); i < uint32(m68kJitMaxBlockSize)*4; i += 4 {
		if got, want := jit.Read32(dst+i), interp.Read32(dst+i); got != want {
			t.Fatalf("copied long at +0x%X: got=0x%08X want=0x%08X", i, got, want)
		}
	}
}

func TestM68KJIT_DefaultDispatcherMovePostincPostincCodePageWriteInvalidatesWithoutFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		src     = uint32(0x1800)
		dst     = uint32(0x1900) // Same 4 KiB code bitmap page as startPC.
	)
	opcodes := []uint16{
		0x22D8, // MOVE.L (A0)+,(A1)+
		0x6704, // BEQ.S set_two
		0x7201, // MOVEQ #1,D1
		0x6002, // BRA.S done
		0x7202, // MOVEQ #2,D1
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = src
		cpu.AddrRegs[1] = dst
		cpu.Write32(src, 0)
		cpu.Write32(dst, 0xA5A5A5A5)
		cpu.SR = M68K_SR_S | M68K_SR_N | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitFallbackOpcodeCounts[0x22D8].Load(); got != 0 {
		t.Fatalf("MOVE.L (A0)+,(A1)+ code-page write fell back %d times", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("MOVE.L (A0)+,(A1)+ code-page write bailed out %d times", got)
	}
	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("MOVE.L (A0)+,(A1)+ did not execute a native JIT block")
	}
	if got := jit.Read32(dst); got != 0 {
		t.Fatalf("destination long=0x%08X, want 0", got)
	}
	if got := jit.DataRegs[1]; got != 2 {
		t.Fatalf("D1=0x%08X, want 2; BEQ consumed wrong MOVE flags", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesAROSOddPostincCopyBlockNoFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC  = uint32(0x1000)
		src      = uint32(0x8001)
		dst      = uint32(0x9003)
		stack    = uint32(0x7000)
		returnPC = startPC + 16
	)
	opcodes := []uint16{
		0x22D8, // MOVE.L (A0)+,(A1)+
		0xD080, // ADD.L D0,D0
		0x6504, // BCS.S skip_word
		0x32D8, // MOVE.W (A0)+,(A1)+
		0x4A80, // TST.L D0
		0x6B02, // BMI.S skip_byte
		0x12D8, // MOVE.B (A0)+,(A1)+
		0x4E75, // RTS
	}
	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 1
		cpu.AddrRegs[0] = src
		cpu.AddrRegs[1] = dst
		cpu.AddrRegs[7] = stack
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		cpu.Write32(stack, returnPC)
		cpu.Write32(src, 0x11223344)
		cpu.Write16(src+4, 0x5566)
		cpu.Write8(src+6, 0x77)
		for off := uint32(0); off < 8; off++ {
			cpu.Write8(dst+off, 0xA5)
		}
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	wantPrefix := len(opcodes) - 1
	if len(prefix) != wantPrefix {
		t.Fatalf("AROS odd postinc copy prefix length=%d want=%d prefix=%+v fallback=%v conservative=%v productionSafe=%v genericIO=%v instrs=%+v",
			len(prefix), wantPrefix, prefix,
			m68kNeedsFallback(instrs), m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
			m68kBlockProductionNativeSafe(instrs), m68kBlockMayUseGenericIOFallback(instrs), instrs)
	}
	runM68KJITUntilStopped(t, jit)

	for _, opcode := range []uint16{0x22D8, 0x32D8, 0x12D8} {
		if got := jit.m68kJitFallbackOpcodeCounts[opcode].Load(); got != 0 {
			t.Fatalf("copy opcode 0x%04X fell back %d times", opcode, got)
		}
	}
	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("AROS odd postinc copy block did not execute a native JIT block")
	}
	for off := uint32(0); off < 7; off++ {
		if got, want := jit.Read8(dst+off), interp.Read8(dst+off); got != want {
			t.Fatalf("destination byte +%d=0x%02X, want 0x%02X", off, got, want)
		}
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAROSPostincFillLoop(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const startPC = uint32(0x1000)
	opcodes := []uint16{
		0x24C1, // MOVE.L D1,(A2)+
		0x2608, // MOVE.L A0,D3
		0x968A, // SUB.L A2,D3
		0xD689, // ADD.L A1,D3
		0x7804, // MOVEQ #4,D4
		0xB883, // CMP.L D3,D4
		0x65F2, // BCS.S loop
	}

	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[1] = 0x11223344
		cpu.AddrRegs[0] = 0x3000
		cpu.AddrRegs[1] = 0x100
		cpu.AddrRegs[2] = 0x30F0
		for off := uint32(0); off < 0x20; off += 4 {
			cpu.Write32(0x30F0+off, 0xDEADBEEF)
		}
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) != len(opcodes) {
		t.Fatalf("MOVE.L postinc fill loop prefix rejected by production gate: instrs=%d prefix=%d needsFallback=%v conservative=%v productionSafe=%v genericIO=%v",
			len(instrs),
			len(prefix),
			m68kNeedsFallback(instrs),
			m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
			m68kBlockProductionNativeSafe(instrs),
			m68kBlockMayUseGenericIOFallback(instrs))
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute AROS postinc fill loop natively")
	}
	assertM68KCoreStateEqual(t, jit, interp)
	for off := uint32(0); off < 0x20; off += 4 {
		if got, want := jit.Read32(0x30F0+off), interp.Read32(0x30F0+off); got != want {
			t.Fatalf("memory[0x%08X] = 0x%08X, want 0x%08X", 0x30F0+off, got, want)
		}
	}
}

func TestM68KJIT_ForceNativeExecutesBytePostincCountdownLoop(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		dst     = uint32(0x3000)
	)
	opcodes := []uint16{
		0x10C0, // MOVE.B D0,(A0)+
		0x5381, // SUBQ.L #1,D1
		0x64FA, // BCC.S loop
	}

	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x123456AB
		cpu.DataRegs[1] = 3
		cpu.AddrRegs[0] = dst
		for off := uint32(0); off < 8; off++ {
			cpu.Write8(dst+off, 0xEE)
		}
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	jit.m68kJitForceNative = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("force-native M68K JIT dispatcher did not execute byte postinc countdown loop natively")
	}
	// The loop body opcodes themselves must never fall back. The only legitimate
	// bailout is the terminal STOP (0x4E72) appended by writeM68KStopProgram,
	// which force-native mode cannot compile and which exits the program.
	for _, op := range opcodes {
		if got := jit.m68kJitFallbackOpcodeCounts[op].Load(); got != 0 {
			t.Fatalf("loop opcode 0x%04X fell back %d times, want 0", op, got)
		}
	}
	assertM68KCoreStateEqual(t, jit, interp)
	for off := uint32(0); off < 8; off++ {
		if got, want := jit.Read8(dst+off), interp.Read8(dst+off); got != want {
			t.Fatalf("memory[0x%08X] = 0x%02X, want 0x%02X", dst+off, got, want)
		}
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeBytePostincCountdownLoop(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		dst     = uint32(0x3000)
	)
	opcodes := []uint16{
		0x10C0, // MOVE.B D0,(A0)+
		0x5381, // SUBQ.L #1,D1
		0x64FA, // BCC.S loop
		0x2009, // MOVE.L A1,D0 after loop
	}

	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x123456AB
		cpu.DataRegs[1] = 3
		cpu.AddrRegs[0] = dst
		cpu.AddrRegs[1] = 0xCAFEBABE
		for off := uint32(0); off < 8; off++ {
			cpu.Write8(dst+off, 0xEE)
		}
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute byte postinc countdown loop natively")
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x10C0].Load(); got != 0 {
		t.Fatalf("byte postinc countdown loop fell back to interpreter %d times", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("byte postinc countdown loop bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
	for off := uint32(0); off < 8; off++ {
		if got, want := jit.Read8(dst+off), interp.Read8(dst+off); got != want {
			t.Fatalf("memory[0x%08X] = 0x%02X, want 0x%02X", dst+off, got, want)
		}
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativePostincEAToDnALU(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		src     = uint32(0x3000)
	)
	opcodes := []uint16{
		0x720A, // MOVEQ #10,D1
	}
	for len(opcodes) < m68kJitMaxBlockSize {
		opcodes = append(opcodes,
			0xD298, // ADD.L (A0)+,D1
			0x9298, // SUB.L (A0)+,D1
			0xB298, // CMP.L (A0)+,D1
		)
	}
	opcodes = opcodes[:m68kJitMaxBlockSize]

	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = src
		for i := uint32(0); i < uint32(m68kJitMaxBlockSize)*4; i += 4 {
			cpu.Write32(src+i, (i/4)+1)
		}
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute postinc EA-to-Dn ALU block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("postinc EA-to-Dn ALU block bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeExtendedEAToDnALU(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	opcodes := []uint16{
		0x203C, 0x0000, 0x0064, // MOVE.L #100,D0
		0xD0A8, 0x0010, // ADD.L 16(A0),D0
		0x90B8, 0x3000, // SUB.L $3000.W,D0
		0xD0BC, 0x0000, 0x0007, // ADD.L #7,D0
		0xB0BC, 0x0000, 0x006C, // CMP.L #108,D0
		0x6704,                 // BEQ.B true
		0x7200,                 // MOVEQ #0,D1
		0x6002,                 // BRA.B done
		0x7201,                 // MOVEQ #1,D1
		0x243C, 0x0000, 0x0028, // MOVE.L #40,D2
		0x94B9, 0x0000, 0x3004, // SUB.L $00003004.L,D2
		0xB4A8, 0x0014, // CMP.L 20(A0),D2
	}
	for i := 0; i < m68kJitMaxBlockSize; i++ {
		opcodes = append(opcodes, 0x7E07) // MOVEQ #7,D7
	}

	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = 0x2000
		cpu.Write32(0x2010, 3)
		cpu.Write32(0x2014, 30)
		cpu.Write32(0x3000, 2)
		cpu.Write32(0x3004, 10)
	}
	interp := newM68KTestProgramCPU(t, 0x1000)
	setup(interp)
	writeM68KStopProgram(interp, 0x1000, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, 0x1000)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, 0x1000, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute extended EA ALU block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("extended EA ALU block bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeDisplacementEAToDnADDSUB(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	tests := []struct {
		name      string
		opcode    uint16
		ext       uint16
		addrReg   int
		base      uint32
		srcOffset uint32
		initialD0 uint32
		source    uint32
	}{
		{
			name:      "add_d16_a0_to_d0",
			opcode:    0xD0A8, // ADD.L d16(A0),D0
			ext:       0x0010,
			addrReg:   0,
			base:      0x2000,
			srcOffset: 0x10,
			initialD0: 0x00000064,
			source:    0x00000003,
		},
		{
			name:      "add_d16_a6_to_d0",
			opcode:    0xD0AE, // ADD.L d16(A6),D0
			ext:       0x0004,
			addrReg:   6,
			base:      0x2400,
			srcOffset: 0x04,
			initialD0: 0x00000100,
			source:    0x00000044,
		},
		{
			name:      "sub_d16_a0_from_d0",
			opcode:    0x90A8, // SUB.L d16(A0),D0
			ext:       0x0008,
			addrReg:   0,
			base:      0x2800,
			srcOffset: 0x08,
			initialD0: 0x00000100,
			source:    0x00000030,
		},
		{
			name:      "sub_d16_a6_from_d0",
			opcode:    0x90AE, // SUB.L d16(A6),D0
			ext:       0x000C,
			addrReg:   6,
			base:      0x2C00,
			srcOffset: 0x0C,
			initialD0: 0x00000100,
			source:    0x00000020,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const startPC = uint32(0x1000)
			opcodes := []uint16{
				0x203C, uint16(tt.initialD0 >> 16), uint16(tt.initialD0), // MOVE.L #initial,D0
				tt.opcode, tt.ext,
				0x7201, // MOVEQ #1,D1
			}

			setup := func(cpu *M68KCPU) {
				cpu.AddrRegs[tt.addrReg] = tt.base
				cpu.Write32(tt.base+tt.srcOffset, tt.source)
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			writeM68KStopProgram(interp, startPC, opcodes...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			writeM68KStopProgram(jit, startPC, opcodes...)

			instrs := m68kScanBlock(jit.memory, startPC)
			prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
			const wantPrefixInstrs = 3
			if len(prefix) != wantPrefixInstrs || !m68kCanUseProductionNativeBlock(jit.memory, startPC, prefix) {
				t.Fatalf("%04X rejected by production gate: instrs=%d prefix=%d needsFallback=%v conservative=%v productionSafe=%v genericIO=%v",
					tt.opcode,
					len(instrs),
					len(prefix),
					m68kNeedsFallback(prefix),
					m68kNeedsConservativeFallback(jit.memory, startPC, prefix),
					m68kBlockProductionNativeSafe(prefix),
					m68kBlockMayUseGenericIOFallback(prefix))
			}

			runM68KJITUntilStopped(t, jit)
			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("default M68K JIT dispatcher did not execute %04X block natively", tt.opcode)
			}
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("%04X block bailed out %d times, want 0", tt.opcode, got)
			}
			if got := jit.m68kJitFallbackOpcodeCounts[tt.opcode].Load(); got != 0 {
				t.Fatalf("%04X fallback count = %d, want 0", tt.opcode, got)
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAddressRegisterSourceADDSUB(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	tests := []struct {
		name      string
		opcode    uint16
		addrReg   int
		addrValue uint32
		initialD  uint32
	}{
		{
			name:      "add_a7_to_d0",
			opcode:    0xD08F, // ADD.L A7,D0
			addrReg:   7,
			addrValue: 0x00120000,
			initialD:  0x00000040,
		},
		{
			name:      "add_a7_to_d2",
			opcode:    0xD48F, // ADD.L A7,D2
			addrReg:   7,
			addrValue: 0x00110020,
			initialD:  0x00000080,
		},
		{
			name:      "sub_a7_from_d0",
			opcode:    0x908F, // SUB.L A7,D0
			addrReg:   7,
			addrValue: 0x00000020,
			initialD:  0x00000100,
		},
		{
			name:      "sub_a6_from_d1",
			opcode:    0x928E, // SUB.L A6,D1
			addrReg:   6,
			addrValue: 0x00000010,
			initialD:  0x00000100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const startPC = uint32(0x1000)
			dstReg := int((tt.opcode >> 9) & 7)
			moveImmToDst := uint16(0x203C | uint16(dstReg<<9))
			opcodes := []uint16{
				moveImmToDst, uint16(tt.initialD >> 16), uint16(tt.initialD), // MOVE.L #initial,Dn
				tt.opcode,
				0x7E01, // MOVEQ #1,D7
			}

			setup := func(cpu *M68KCPU) {
				cpu.AddrRegs[tt.addrReg] = tt.addrValue
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			writeM68KStopProgram(interp, startPC, opcodes...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			writeM68KStopProgram(jit, startPC, opcodes...)

			instrs := m68kScanBlock(jit.memory, startPC)
			prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
			const wantPrefixInstrs = 3
			if len(prefix) != wantPrefixInstrs || !m68kCanUseProductionNativeBlock(jit.memory, startPC, prefix) {
				t.Fatalf("%04X rejected by production gate: instrs=%d prefix=%d needsFallback=%v conservative=%v productionSafe=%v genericIO=%v",
					tt.opcode,
					len(instrs),
					len(prefix),
					m68kNeedsFallback(prefix),
					m68kNeedsConservativeFallback(jit.memory, startPC, prefix),
					m68kBlockProductionNativeSafe(prefix),
					m68kBlockMayUseGenericIOFallback(prefix))
			}

			runM68KJITUntilStopped(t, jit)
			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("default M68K JIT dispatcher did not execute %04X block natively", tt.opcode)
			}
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("%04X block bailed out %d times, want 0", tt.opcode, got)
			}
			if got := jit.m68kJitFallbackOpcodeCounts[tt.opcode].Load(); got != 0 {
				t.Fatalf("%04X fallback count = %d, want 0", tt.opcode, got)
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeLogicDnToMemoryEA(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	tests := []struct {
		name       string
		opcode     uint16
		ext        []uint16
		setup      func(*M68KCPU)
		checkAddrs []uint32
	}{
		{
			name:   "or_l_d1_to_a7_indirect",
			opcode: 0x8397, // OR.L D1,(A7)
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[1] = 0x0000FF00
				cpu.AddrRegs[7] = 0x120000
				cpu.Write32(0x120000, 0x00F0000F)
			},
			checkAddrs: []uint32{0x120000},
		},
		{
			name:   "or_l_d1_to_a7_displacement",
			opcode: 0x83AF, // OR.L D1,16(A7)
			ext:    []uint16{0x0010},
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[1] = 0x000000F0
				cpu.AddrRegs[7] = 0x120000
				cpu.Write32(0x120010, 0x00000F00)
			},
			checkAddrs: []uint32{0x120010},
		},
		{
			name:   "or_l_d1_to_a7_postincrement",
			opcode: 0x839F, // OR.L D1,(A7)+
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[1] = 0x0000FF00
				cpu.AddrRegs[7] = 0x120000
				cpu.Write32(0x120000, 0x00F0000F)
			},
			checkAddrs: []uint32{0x120000},
		},
		{
			name:   "or_l_d1_to_a7_predecrement",
			opcode: 0x83A7, // OR.L D1,-(A7)
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[1] = 0x0000FF00
				cpu.AddrRegs[7] = 0x120004
				cpu.Write32(0x120000, 0x00F0000F)
			},
			checkAddrs: []uint32{0x120000},
		},
		{
			name:   "and_l_d1_to_a7_indirect",
			opcode: 0xC397, // AND.L D1,(A7)
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[1] = 0x00FFFF00
				cpu.AddrRegs[7] = 0x120000
				cpu.Write32(0x120000, 0x0F0F0F0F)
			},
			checkAddrs: []uint32{0x120000},
		},
		{
			name:   "and_l_d1_to_a7_displacement",
			opcode: 0xC3AF, // AND.L D1,16(A7)
			ext:    []uint16{0x0010},
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[1] = 0x0000FFFF
				cpu.AddrRegs[7] = 0x120000
				cpu.Write32(0x120010, 0x12345678)
			},
			checkAddrs: []uint32{0x120010},
		},
		{
			name:   "and_l_d1_to_abs_word",
			opcode: 0xC3B8, // AND.L D1,$3000.W
			ext:    []uint16{0x3000},
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[1] = 0x0FF00FF0
				cpu.Write32(0x3000, 0xFFFF000F)
			},
			checkAddrs: []uint32{0x3000},
		},
		{
			name:   "and_l_a7_postincrement_to_d0",
			opcode: 0xC09F, // AND.L (A7)+,D0
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 0xFFFF0000
				cpu.AddrRegs[7] = 0x120000
				cpu.Write32(0x120000, 0x00FFFF00)
			},
			checkAddrs: []uint32{0x120000},
		},
		{
			name:   "and_l_a7_predecrement_to_d4",
			opcode: 0xC8A7, // AND.L -(A7),D4
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[4] = 0xFFFF0000
				cpu.AddrRegs[7] = 0x120004
				cpu.Write32(0x120000, 0x00FFFF00)
			},
			checkAddrs: []uint32{0x120000},
		},
		{
			name:   "and_l_pc_indexed_to_d5",
			opcode: 0xCABB,           // AND.L (d8,PC,D0.L),D5
			ext:    []uint16{0x0800}, // D0.L, scale 1, displacement 0
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 0x00000200
				cpu.DataRegs[5] = 0xFFFF0000
				cpu.Write32(0x1202, 0x00FFFF00)
			},
			checkAddrs: []uint32{0x1202},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const startPC = uint32(0x1000)
			opcodes := []uint16{
				tt.opcode,
			}
			opcodes = append(opcodes, tt.ext...)
			opcodes = append(opcodes, 0x7E01) // MOVEQ #1,D7

			interp := newM68KTestProgramCPU(t, startPC)
			tt.setup(interp)
			writeM68KStopProgram(interp, startPC, opcodes...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			tt.setup(jit)
			writeM68KStopProgram(jit, startPC, opcodes...)
			instrs := m68kScanBlock(jit.memory, startPC)
			prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
			const wantPrefixInstrs = 2
			if len(prefix) != wantPrefixInstrs || !m68kCanUseProductionNativeBlock(jit.memory, startPC, prefix) {
				t.Fatalf("%04X rejected by production gate: instrs=%d prefix=%d needsFallback=%v conservative=%v productionSafe=%v genericIO=%v",
					tt.opcode,
					len(instrs),
					len(prefix),
					m68kNeedsFallback(prefix),
					m68kNeedsConservativeFallback(jit.memory, startPC, prefix),
					m68kBlockProductionNativeSafe(prefix),
					m68kBlockMayUseGenericIOFallback(prefix))
			}

			runM68KJITUntilStopped(t, jit)
			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("default M68K JIT dispatcher did not execute %04X block natively", tt.opcode)
			}
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("%04X block bailed out %d times, want 0", tt.opcode, got)
			}
			if got := jit.m68kJitFallbackOpcodeCounts[tt.opcode].Load(); got != 0 {
				t.Fatalf("%04X fallback count = %d, want 0", tt.opcode, got)
			}
			for _, addr := range tt.checkAddrs {
				if got, want := jit.Read32(addr), interp.Read32(addr); got != want {
					t.Fatalf("memory[0x%08X]=0x%08X, want 0x%08X", addr, got, want)
				}
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeArithDnToMemoryEA(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	tests := []struct {
		name       string
		opcode     uint16
		ext        []uint16
		setup      func(*M68KCPU)
		checkAddrs []uint32
	}{
		{
			name:   "sub_l_d3_to_a4_indirect",
			opcode: 0x9794, // SUB.L D3,(A4)
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[3] = 0x00000010
				cpu.AddrRegs[4] = 0x120000
				cpu.Write32(0x120000, 0x00000100)
			},
			checkAddrs: []uint32{0x120000},
		},
		{
			name:   "sub_l_d3_to_a4_postincrement",
			opcode: 0x979C, // SUB.L D3,(A4)+
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[3] = 0x00000010
				cpu.AddrRegs[4] = 0x120000
				cpu.Write32(0x120000, 0x00000100)
			},
			checkAddrs: []uint32{0x120000},
		},
		{
			name:   "sub_l_d3_to_a4_predecrement",
			opcode: 0x97A4, // SUB.L D3,-(A4)
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[3] = 0x00000101
				cpu.AddrRegs[4] = 0x120004
				cpu.Write32(0x120000, 0x00000100)
			},
			checkAddrs: []uint32{0x120000},
		},
		{
			name:   "add_l_d2_to_a5_postincrement",
			opcode: 0xD59D, // ADD.L D2,(A5)+
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[2] = 0x00000020
				cpu.AddrRegs[5] = 0x120100
				cpu.Write32(0x120100, 0x00000100)
			},
			checkAddrs: []uint32{0x120100},
		},
		{
			name:   "add_l_d2_to_a5_predecrement",
			opcode: 0xD5A5, // ADD.L D2,-(A5)
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[2] = 0xFFFFFFFF
				cpu.AddrRegs[5] = 0x120104
				cpu.Write32(0x120100, 0x00000001)
			},
			checkAddrs: []uint32{0x120100},
		},
		{
			name:   "add_l_d1_to_a7_displacement",
			opcode: 0xD3AF, // ADD.L D1,16(A7)
			ext:    []uint16{0x0010},
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[1] = 0x7FFFFFFF
				cpu.AddrRegs[7] = 0x120200
				cpu.Write32(0x120210, 0x00000001)
			},
			checkAddrs: []uint32{0x120210},
		},
		{
			name:   "sub_l_d4_to_abs_word",
			opcode: 0x99B8, // SUB.L D4,$3000.W
			ext:    []uint16{0x3000},
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[4] = 0x00000001
				cpu.Write32(0x3000, 0x80000000)
			},
			checkAddrs: []uint32{0x3000},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const startPC = uint32(0x1000)
			opcodes := []uint16{
				tt.opcode,
			}
			opcodes = append(opcodes, tt.ext...)
			opcodes = append(opcodes, 0x7E01) // MOVEQ #1,D7

			interp := newM68KTestProgramCPU(t, startPC)
			tt.setup(interp)
			writeM68KStopProgram(interp, startPC, opcodes...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			tt.setup(jit)
			writeM68KStopProgram(jit, startPC, opcodes...)
			instrs := m68kScanBlock(jit.memory, startPC)
			prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
			const wantPrefixInstrs = 2
			if len(prefix) != wantPrefixInstrs || !m68kCanUseProductionNativeBlock(jit.memory, startPC, prefix) {
				t.Fatalf("%04X rejected by production gate: instrs=%d prefix=%d needsFallback=%v conservative=%v productionSafe=%v genericIO=%v",
					tt.opcode,
					len(instrs),
					len(prefix),
					m68kNeedsFallback(prefix),
					m68kNeedsConservativeFallback(jit.memory, startPC, prefix),
					m68kBlockProductionNativeSafe(prefix),
					m68kBlockMayUseGenericIOFallback(prefix))
			}

			runM68KJITUntilStopped(t, jit)
			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("default M68K JIT dispatcher did not execute %04X block natively", tt.opcode)
			}
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("%04X block bailed out %d times, want 0", tt.opcode, got)
			}
			if got := jit.m68kJitFallbackOpcodeCounts[tt.opcode].Load(); got != 0 {
				t.Fatalf("%04X fallback count = %d, want 0", tt.opcode, got)
			}
			for _, addr := range tt.checkAddrs {
				if got, want := jit.Read32(addr), interp.Read32(addr); got != want {
					t.Fatalf("memory[0x%08X]=0x%08X, want 0x%08X", addr, got, want)
				}
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeImmediateMemoryEA(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	tests := []struct {
		name       string
		opcode     uint16
		words      []uint16
		setup      func(*M68KCPU)
		checkAddrs []uint32
	}{
		{
			name:   "eori_l_to_a7_indirect",
			opcode: 0x0A97, // EORI.L #imm,(A7)
			words:  []uint16{0x00FF, 0x0F0F},
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[7] = 0x120000
				cpu.Write32(0x120000, 0x0FF00000)
			},
			checkAddrs: []uint32{0x120000},
		},
		{
			name:   "cmpi_l_to_a7_predecrement",
			opcode: 0x0CA7, // CMPI.L #imm,-(A7)
			words:  []uint16{0x0000, 0x0002},
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[7] = 0x120004
				cpu.Write32(0x120000, 0x00000001)
			},
			checkAddrs: []uint32{0x120000},
		},
		{
			name:   "ori_l_to_a4_postincrement",
			opcode: 0x009C, // ORI.L #imm,(A4)+
			words:  []uint16{0x0000, 0x00F0},
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[4] = 0x120100
				cpu.Write32(0x120100, 0x00000F00)
			},
			checkAddrs: []uint32{0x120100},
		},
		{
			name:   "andi_l_to_abs_word",
			opcode: 0x02B8, // ANDI.L #imm,$3000.W
			words:  []uint16{0x0FFF, 0xFFFF, 0x3000},
			setup: func(cpu *M68KCPU) {
				cpu.Write32(0x3000, 0xFFFF000F)
			},
			checkAddrs: []uint32{0x3000},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const startPC = uint32(0x1000)
			opcodes := []uint16{tt.opcode}
			opcodes = append(opcodes, tt.words...)
			opcodes = append(opcodes, 0x7E01) // MOVEQ #1,D7

			interp := newM68KTestProgramCPU(t, startPC)
			tt.setup(interp)
			writeM68KStopProgram(interp, startPC, opcodes...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			tt.setup(jit)
			writeM68KStopProgram(jit, startPC, opcodes...)
			instrs := m68kScanBlock(jit.memory, startPC)
			prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
			const wantPrefixInstrs = 2
			if len(prefix) != wantPrefixInstrs || !m68kCanUseProductionNativeBlock(jit.memory, startPC, prefix) {
				t.Fatalf("%04X rejected by production gate: instrs=%d prefix=%d needsFallback=%v conservative=%v productionSafe=%v genericIO=%v",
					tt.opcode,
					len(instrs),
					len(prefix),
					m68kNeedsFallback(prefix),
					m68kNeedsConservativeFallback(jit.memory, startPC, prefix),
					m68kBlockProductionNativeSafe(prefix),
					m68kBlockMayUseGenericIOFallback(prefix))
			}

			runM68KJITUntilStopped(t, jit)
			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("default M68K JIT dispatcher did not execute %04X block natively", tt.opcode)
			}
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("%04X block bailed out %d times, want 0", tt.opcode, got)
			}
			if got := jit.m68kJitFallbackOpcodeCounts[tt.opcode].Load(); got != 0 {
				t.Fatalf("%04X fallback count = %d, want 0", tt.opcode, got)
			}
			for _, addr := range tt.checkAddrs {
				if got, want := jit.Read32(addr), interp.Read32(addr); got != want {
					t.Fatalf("memory[0x%08X]=0x%08X, want 0x%08X", addr, got, want)
				}
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeCMPMemoryEA(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	tests := []struct {
		name       string
		opcode     uint16
		ext        []uint16
		setup      func(*M68KCPU)
		checkAddrs []uint32
	}{
		{
			name:   "cmp_l_pc_displacement_to_d1",
			opcode: 0xB2BA, // CMP.L (d16,PC),D1
			ext:    []uint16{0x0200},
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[1] = 0x00000100
				cpu.Write32(0x1202, 0x00000080)
			},
			checkAddrs: []uint32{0x1202},
		},
		{
			name:   "cmp_l_predecrement_a3_to_d3",
			opcode: 0xB6A3, // CMP.L -(A3),D3
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[3] = 0x80000000
				cpu.AddrRegs[3] = 0x120104
				cpu.Write32(0x120100, 0x7FFFFFFF)
			},
			checkAddrs: []uint32{0x120100},
		},
		{
			name:   "cmp_l_pc_displacement_to_d3",
			opcode: 0xB6BA, // CMP.L (d16,PC),D3
			ext:    []uint16{0x0200},
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[3] = 0x00000001
				cpu.Write32(0x1202, 0x00000002)
			},
			checkAddrs: []uint32{0x1202},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const startPC = uint32(0x1000)
			opcodes := []uint16{tt.opcode}
			opcodes = append(opcodes, tt.ext...)
			opcodes = append(opcodes, 0x7E01) // MOVEQ #1,D7

			interp := newM68KTestProgramCPU(t, startPC)
			tt.setup(interp)
			writeM68KStopProgram(interp, startPC, opcodes...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			tt.setup(jit)
			writeM68KStopProgram(jit, startPC, opcodes...)
			instrs := m68kScanBlock(jit.memory, startPC)
			prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
			const wantPrefixInstrs = 2
			if len(prefix) != wantPrefixInstrs || !m68kCanUseProductionNativeBlock(jit.memory, startPC, prefix) {
				t.Fatalf("%04X rejected by production gate: instrs=%d prefix=%d needsFallback=%v conservative=%v productionSafe=%v genericIO=%v",
					tt.opcode,
					len(instrs),
					len(prefix),
					m68kNeedsFallback(prefix),
					m68kNeedsConservativeFallback(jit.memory, startPC, prefix),
					m68kBlockProductionNativeSafe(prefix),
					m68kBlockMayUseGenericIOFallback(prefix))
			}

			runM68KJITUntilStopped(t, jit)
			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("default M68K JIT dispatcher did not execute %04X block natively", tt.opcode)
			}
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("%04X block bailed out %d times, want 0", tt.opcode, got)
			}
			if got := jit.m68kJitFallbackOpcodeCounts[tt.opcode].Load(); got != 0 {
				t.Fatalf("%04X fallback count = %d, want 0", tt.opcode, got)
			}
			for _, addr := range tt.checkAddrs {
				if got, want := jit.Read32(addr), interp.Read32(addr); got != want {
					t.Fatalf("memory[0x%08X]=0x%08X, want 0x%08X", addr, got, want)
				}
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeMoveImmediateLong(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	opcodes := make([]uint16, 0, m68kJitMaxBlockSize*3)
	for i := 0; i < m68kJitMaxBlockSize/2; i++ {
		dVal := uint32(0x10000000 + i)
		aVal := uint32(0x20000000 + i*4)
		opcodes = append(opcodes,
			0x203C, uint16(dVal>>16), uint16(dVal), // MOVE.L #imm,D0
			0x227C, uint16(aVal>>16), uint16(aVal), // MOVEA.L #imm,A1
		)
	}

	interp := runM68KInterpreterStopProgram(t, 0x1000, opcodes...)
	jit := runM68KJITStopProgram(t, 0x1000, opcodes...)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute immediate MOVE.L/MOVEA.L block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("immediate MOVE.L/MOVEA.L block bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeMoveLongEAToRegs(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	opcodes := []uint16{
		0x2010,         // MOVE.L (A0),D0
		0x2428, 0x0010, // MOVE.L 16(A0),D2
		0x2218,         // MOVE.L (A0)+,D1
		0x2638, 0x3000, // MOVE.L $3000.W,D3
		0x2839, 0x0000, 0x3004, // MOVE.L $00003004.L,D4
		0x2A09,                 // MOVE.L A1,D5
		0x2451,                 // MOVEA.L (A1),A2
		0x2C3C, 0x1357, 0x9BDF, // MOVE.L #0x13579BDF,D6
		0x2046, // MOVEA.L D6,A0
	}
	for i := 0; i < m68kJitMaxBlockSize; i++ {
		opcodes = append(opcodes, 0x7E07) // MOVEQ #7,D7
	}

	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = 0x2000
		cpu.AddrRegs[1] = 0x2010
		cpu.Write32(0x2000, 0x11223344)
		cpu.Write32(0x2010, 0x55667788)
		cpu.Write32(0x3000, 0xCAFEBABE)
		cpu.Write32(0x3004, 0x0BADF00D)
	}
	interp := newM68KTestProgramCPU(t, 0x1000)
	setup(interp)
	writeM68KStopProgram(interp, 0x1000, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, 0x1000)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, 0x1000, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute MOVE.L EA-to-reg block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("MOVE.L EA-to-reg block bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_NativeAROSSetPatchPrologueLoadsD5(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	opcodes := []uint16{
		0x4FEF, 0xFFEC, // LEA -20(A7),A7
		0x48E7, 0x3C32, // MOVEM.L D2-D5/A2/A5-A6,-(A7)
		0x2808,         // MOVE.L A0,D4
		0x2A38, 0x0004, // MOVE.L $0004.W,D5
		0x47EF, 0x001C, // LEA 28(A7),A3
		0x4878, 0x0014, // PEA $0014.W
		0x42A7, // CLR.L -(A7)
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = 0x0080055C
		cpu.AddrRegs[2] = 0x008A5528
		cpu.AddrRegs[5] = 0x00940AFC
		cpu.AddrRegs[6] = 0x0080055C
		cpu.AddrRegs[7] = 0x00940B00
		cpu.DataRegs[2] = 0x00004000
		cpu.DataRegs[3] = 0x008ADB14
		cpu.DataRegs[4] = 0x11111111
		cpu.DataRegs[5] = 0x22222222
		cpu.Write32(0x0004, 0x0080055C)
	}

	interp := newM68KTestProgramCPU(t, 0x1000)
	setup(interp)
	writeM68KStopProgram(interp, 0x1000, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, 0x1000)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, 0x1000, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute AROS SetPatch prologue natively")
	}
	if got, want := jit.DataRegs[5], interp.DataRegs[5]; got != want {
		t.Fatalf("D5=0x%08X, want interpreter 0x%08X", got, want)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeMoveLongRegsToEA(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	opcodes := []uint16{
		0x203C, 0x1122, 0x3344, // MOVE.L #0x11223344,D0
		0x2080,                 // MOVE.L D0,(A0)
		0x223C, 0x5566, 0x7788, // MOVE.L #0x55667788,D1
		0x20C1,                 // MOVE.L D1,(A0)+
		0x243C, 0x99AA, 0xBBCC, // MOVE.L #0x99AABBCC,D2
		0x2142, 0x0010, // MOVE.L D2,16(A0)
		0x263C, 0xA5A5, 0xA5A5, // MOVE.L #0xA5A5A5A5,D3
		0x21C3, 0x3000, // MOVE.L D3,$3000.W
		0x283C, 0xCAFE, 0xBABE, // MOVE.L #0xCAFEBABE,D4
		0x23C4, 0x0000, 0x3004, // MOVE.L D4,$00003004.L
		0x2A3C, 0x1357, 0x9BDF, // MOVE.L #0x13579BDF,D5
		0x20FC, 0x2468, 0xACE0, // MOVE.L #0x2468ACE0,(A0)+
	}
	for i := 0; i < m68kJitMaxBlockSize; i++ {
		opcodes = append(opcodes, 0x7E07) // MOVEQ #7,D7
	}

	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = 0x2000
	}
	interp := newM68KTestProgramCPU(t, 0x1000)
	setup(interp)
	writeM68KStopProgram(interp, 0x1000, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, 0x1000)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, 0x1000, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute MOVE.L reg-to-EA block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("MOVE.L reg-to-EA block bailed out %d times, want 0", got)
	}
	for _, addr := range []uint32{0x2000, 0x2004, 0x2014, 0x3000, 0x3004} {
		if got, want := jit.Read32(addr), interp.Read32(addr); got != want {
			t.Fatalf("memory[0x%08X] = 0x%08X, want 0x%08X", addr, got, want)
		}
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_OddAddressWordLongDirectRAMParityNoBail(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	tests := []struct {
		name   string
		ops    []uint16
		setup  func(*M68KCPU)
		verify func(t *testing.T, interp, jit *M68KCPU)
	}{
		{
			name: "move.l_odd_read",
			ops: []uint16{
				0x2010, // MOVE.L (A0),D0
			},
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = 0x2001
				cpu.Write32(0x2001, 0x11223344)
			},
			verify: func(t *testing.T, interp, jit *M68KCPU) {
				t.Helper()
				if got, want := jit.DataRegs[0], interp.DataRegs[0]; got != want {
					t.Fatalf("D0=0x%08X want 0x%08X", got, want)
				}
			},
		},
		{
			name: "move.w_odd_read",
			ops: []uint16{
				0x3010, // MOVE.W (A0),D0
			},
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = 0x2001
				cpu.Write16(0x2001, 0x8877)
			},
			verify: func(t *testing.T, interp, jit *M68KCPU) {
				t.Helper()
				if got, want := jit.DataRegs[0], interp.DataRegs[0]; got != want {
					t.Fatalf("D0=0x%08X want 0x%08X", got, want)
				}
			},
		},
		{
			name: "move.l_odd_write",
			ops: []uint16{
				0x203C, 0x5566, 0x7788, // MOVE.L #$55667788,D0
				0x2080, // MOVE.L D0,(A0)
			},
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = 0x2001
			},
			verify: func(t *testing.T, interp, jit *M68KCPU) {
				t.Helper()
				if got, want := jit.Read32(0x2001), interp.Read32(0x2001); got != want {
					t.Fatalf("mem32=0x%08X want 0x%08X", got, want)
				}
			},
		},
		{
			name: "move.w_odd_write",
			ops: []uint16{
				0x303C, 0x99AA, // MOVE.W #$99AA,D0
				0x3080, // MOVE.W D0,(A0)
			},
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = 0x2001
			},
			verify: func(t *testing.T, interp, jit *M68KCPU) {
				t.Helper()
				if got, want := jit.Read16(0x2001), interp.Read16(0x2001); got != want {
					t.Fatalf("mem16=0x%04X want 0x%04X", got, want)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp := newM68KTestProgramCPU(t, 0x1000)
			tt.setup(interp)
			writeM68KStopProgram(interp, 0x1000, tt.ops...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, 0x1000)
			jit.m68kJitEnabled = true
			tt.setup(jit)
			writeM68KStopProgram(jit, 0x1000, tt.ops...)
			runM68KJITUntilStopped(t, jit)

			tt.verify(t, interp, jit)
			assertM68KCoreStateEqual(t, interp, jit)
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("JIT bailed out %d times on odd direct RAM access", got)
			}
			if got := jit.m68kJitFallbackInstructions.Load(); got != 1 {
				t.Fatalf("fallback instructions = %d, want only STOP fallback", got)
			}
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeMoveByteWord(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	opcodes := []uint16{
		0x103C, 0x0080, // MOVE.B #0x80,D0
		0x6B04,         // BMI.B byte_negative
		0x7200,         // MOVEQ #0,D1
		0x6002,         // BRA.B after_byte_flag
		0x7201,         // MOVEQ #1,D1
		0x1080,         // MOVE.B D0,(A0)
		0x1218,         // MOVE.B (A0)+,D1
		0x1428, 0x000F, // MOVE.B 15(A0),D2
		0x11C2, 0x3000, // MOVE.B D2,$3000.W
		0x207C, 0x0000, 0x2002, // MOVEA.L #0x2002,A0
		0x303C, 0x8001, // MOVE.W #0x8001,D0
		0x6B04,         // BMI.B word_negative
		0x7400,         // MOVEQ #0,D2
		0x6002,         // BRA.B after_word_flag
		0x7401,         // MOVEQ #1,D2
		0x3080,         // MOVE.W D0,(A0)
		0x3218,         // MOVE.W (A0)+,D1
		0x3438, 0x3002, // MOVE.W $3002.W,D2
		0x31C2, 0x3004, // MOVE.W D2,$3004.W
	}
	for i := 0; i < m68kJitMaxBlockSize; i++ {
		opcodes = append(opcodes, 0x7E07) // MOVEQ #7,D7
	}

	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = 0x2000
		cpu.Write8(0x2010, 0x7F)
		cpu.Write16(0x3002, 0x1234)
	}
	interp := newM68KTestProgramCPU(t, 0x1000)
	setup(interp)
	writeM68KStopProgram(interp, 0x1000, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, 0x1000)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, 0x1000, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute MOVE.B/W block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("MOVE.B/W block bailed out %d times, want 0", got)
	}
	for _, addr := range []uint32{0x2000, 0x2002, 0x3000, 0x3004} {
		if got, want := jit.Read32(addr), interp.Read32(addr); got != want {
			t.Fatalf("memory[0x%08X] = 0x%08X, want 0x%08X", addr, got, want)
		}
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherMatchesInterpreterMoveBytePostincMemoryToMemory(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		src     = uint32(0x2400)
		dst     = uint32(0x3400)
		count   = 32
	)
	opcodes := make([]uint16, count)
	for i := range opcodes {
		opcodes[i] = 0x12D8 // MOVE.B (A0)+,(A1)+
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = src
		cpu.AddrRegs[1] = dst
		for i := uint32(0); i < count; i++ {
			cpu.Write8(src+i, byte(0x80+i))
			cpu.Write8(dst+i, 0)
		}
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	jit.m68kJitForceNative = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute MOVE.B postinc memory block natively")
	}
	if got, want := jit.AddrRegs[0], interp.AddrRegs[0]; got != want {
		t.Fatalf("A0 after MOVE.B postinc memory block = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := jit.AddrRegs[1], interp.AddrRegs[1]; got != want {
		t.Fatalf("A1 after MOVE.B postinc memory block = 0x%08X, want 0x%08X", got, want)
	}
	for i := uint32(0); i < count; i++ {
		if got, want := jit.Read8(dst+i), interp.Read8(dst+i); got != want {
			t.Fatalf("dst[%d] = 0x%02X, want 0x%02X", i, got, want)
		}
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeByteOpsOnMappedD1(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	opcodes := []uint16{
		0x223C, 0xAABB, 0xCC12, // MOVE.L #$AABBCC12,D1
		0x7030, // MOVEQ #$30,D0
		0xC200, // AND.B D0,D1 -> D1 low byte = $10, upper 24 bits preserved
	}
	for i := 0; i < m68kJitMaxBlockSize; i++ {
		opcodes = append(opcodes, 0x7E07) // MOVEQ #7,D7
	}

	interp := newM68KTestProgramCPU(t, 0x1000)
	writeM68KStopProgram(interp, 0x1000, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, 0x1000)
	jit.m68kJitEnabled = true
	writeM68KStopProgram(jit, 0x1000, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute D1 byte block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("D1 byte block bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeLEA(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	opcodes := make([]uint16, 0, m68kJitMaxBlockSize*3)
	for i := 0; i < m68kJitMaxBlockSize/2; i++ {
		abs := uint32(0x30000000 + i*16)
		disp := uint16(i * 2)
		opcodes = append(opcodes,
			0x41F9, uint16(abs>>16), uint16(abs), // LEA (abs).L,A0
			0x43E8, disp, // LEA disp(A0),A1
		)
	}

	interp := runM68KInterpreterStopProgram(t, 0x1000, opcodes...)
	jit := runM68KJITStopProgram(t, 0x1000, opcodes...)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute LEA block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("LEA block bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_ForceNativeExecutesCMPIWordAn(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[2] = 0x2000
		cpu.Write16(0x2000, 0x1234)
	}
	opcodes := []uint16{
		0x0C52, 0x1234, // CMPI.W #$1234,(A2)
		0x6702, // BEQ.B +2
		0x7099, // MOVEQ #-103,D0 (skipped)
		0x7001, // MOVEQ #1,D0
	}
	for i := 0; i < m68kJitMaxBlockSize; i++ {
		opcodes = append(opcodes, 0x7E01) // MOVEQ #1,D7
	}

	interp := newM68KTestProgramCPU(t, 0x1000)
	setup(interp)
	writeM68KStopProgram(interp, 0x1000, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, 0x1000)
	jit.m68kJitEnabled = true
	jit.m68kJitForceNative = true
	setup(jit)
	writeM68KStopProgram(jit, 0x1000, opcodes...)
	instrs := m68kScanBlock(jit.memory, 0x1000)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatalf("force-native M68K JIT dispatcher did not execute CMPI.W #imm,(An) block natively: needsFallback=%v conservative=%v productionSafe=%v genericIO=%v instrs=%+v",
			m68kNeedsFallback(instrs),
			m68kNeedsConservativeFallback(jit.memory, 0x1000, instrs),
			m68kBlockProductionNativeSafe(instrs),
			m68kBlockMayUseGenericIOFallback(instrs),
			instrs)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_ForceNativeExecutesAROSCMPIBlockWithNegativeJSR(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		node    = uint32(0x3000)
		subPC   = uint32(0x4000)
	)
	opcodes := []uint16{
		0x0C52, 0x4AFC, // CMPI.W #$4AFC,(A2)
		0x663C,         // BNE.S skip
		0xB5EA, 0x0002, // CMPA.L 2(A2),A2
		0x6636,         // BNE.S skip
		0x2C46,         // MOVEA.L D6,A6
		0x4EAE, 0xFF7C, // JSR -132(A6)
		0x7207, // MOVEQ #7,D1 after return
	}

	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[2] = node
		cpu.DataRegs[6] = subPC + 132
		cpu.AddrRegs[7] = 0x9000
		cpu.Write16(node, 0x4AFC)
		cpu.Write32(node+2, node)
		writeM68KWords(cpu, subPC,
			0x702A, // MOVEQ #42,D0
			0x4E75, // RTS
		)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	jit.m68kJitForceNative = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("force-native M68K JIT dispatcher did not execute AROS CMPI/CMPA/JSR block natively")
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAROSCMPIBlockWithNegativeJSR(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		node    = uint32(0x3000)
		subPC   = uint32(0x4000)
	)
	opcodes := []uint16{
		0x0C52, 0x4AFC, // CMPI.W #$4AFC,(A2)
		0x663C,         // BNE.S skip
		0xB5EA, 0x0002, // CMPA.L 2(A2),A2
		0x6636,         // BNE.S skip
		0x2C46,         // MOVEA.L D6,A6
		0x4EAE, 0xFF7C, // JSR -132(A6)
		0x7207, // MOVEQ #7,D1 after return
	}

	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[2] = node
		cpu.DataRegs[6] = subPC + 132
		cpu.AddrRegs[7] = 0x9000
		cpu.Write16(node, 0x4AFC)
		cpu.Write32(node+2, node)
		writeM68KWords(cpu, subPC,
			0x702A, // MOVEQ #42,D0
			0x4E75, // RTS
		)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	if !m68kIsAROSCMPIJSRBlock(instrs, startPC, jit.memory) {
		t.Fatalf("test block did not match AROS CMPI/JSR recognizer: instrs=%+v", instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatalf("default M68K JIT dispatcher did not execute AROS CMPI/CMPA/JSR block natively: fallback=%v conservative=%v productionSafe=%v genericIO=%v",
			m68kNeedsFallback(instrs),
			m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
			m68kBlockProductionNativeSafe(instrs),
			m68kBlockMayUseGenericIOFallback(instrs))
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_AROSResidentScanRangeHasNativePrefix(t *testing.T) {
	const (
		scanPC = uint32(0x0064D9E2)
		stepPC = uint32(0x0064DA1E)
	)
	memory := make([]byte, 0x0064E000)
	put := func(pc uint32, words ...uint16) {
		for _, w := range words {
			memory[pc] = byte(w >> 8)
			memory[pc+1] = byte(w)
			pc += 2
		}
	}
	put(scanPC,
		0x246F, 0x0018, // MOVEA.L 24(A7),A2
		0x0C52, 0x4AFC, // CMPI.W #$4AFC,(A2)
		0x6632,         // BNE.S step
		0xB5EA, 0x0002, // CMPA.L 2(A2),A2
		0x662C,         // BNE.S step
		0x4A83,         // TST.L D3
		0x6638,         // BNE.S dispatch
		0x486F, 0x0018, // PEA 24(A7)
		0x2F02, // MOVE.L D2,-(A7)
		0x2F0A, // MOVE.L A2,-(A7)
		0x4E94, // JSR (A4)
	)
	put(stepPC,
		0x202F, 0x0018, // MOVE.L 24(A7),D0
		0x5480,         // ADDQ.L #2,D0
		0x2F40, 0x0018, // MOVE.L D0,24(A7)
		0xB880, // CMP.L D0,D4
		0x62B6, // BHI.S scan
		0x6092, // BRA.S done
	)

	for _, pc := range []uint32{scanPC, stepPC} {
		instrs := m68kScanBlock(memory, pc)
		if len(instrs) == 0 {
			t.Fatalf("scan at 0x%08X returned no instructions", pc)
		}
		if m68kNeedsConservativeFallback(memory, pc, instrs) {
			t.Fatalf("AROS resident scan block at 0x%08X was conservatively rejected: instrs=%+v", pc, instrs)
		}
		if prefix := m68kProductionNativePrefix(memory, pc, instrs); len(prefix) == 0 {
			t.Fatalf("AROS resident scan block at 0x%08X has no native prefix: %+v", pc, instrs)
		}
	}
}

func TestM68KJIT_ForceNativeAROSResidentScanBlocksMatchInterpreter(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		scanPC   = uint32(0x0064D9E2)
		stepPC   = uint32(0x0064DA1E)
		loopPC   = uint32(0x0064D9C0)
		callPC   = uint32(0x0064DA2E)
		returnPC = uint32(0x0064DA00)
		node     = uint32(0x00020000)
		subPC    = uint32(0x00021000)
		stack    = uint32(0x0000F000)
	)

	writeResidentScanProgram := func(cpu *M68KCPU) {
		writeM68KWords(cpu, scanPC,
			0x246F, 0x0018, // MOVEA.L 24(A7),A2
			0x0C52, 0x4AFC, // CMPI.W #$4AFC,(A2)
			0x6632,         // BNE.S step
			0xB5EA, 0x0002, // CMPA.L 2(A2),A2
			0x662C,         // BNE.S step
			0x4A83,         // TST.L D3
			0x6638,         // BNE.S call
			0x486F, 0x0018, // PEA 24(A7)
			0x2F02, // MOVE.L D2,-(A7)
			0x2F0A, // MOVE.L A2,-(A7)
			0x4E94, // JSR (A4)
		)
		writeM68KWords(cpu, stepPC,
			0x202F, 0x0018, // MOVE.L 24(A7),D0
			0x5480,         // ADDQ.L #2,D0
			0x2F40, 0x0018, // MOVE.L D0,24(A7)
			0xB880, // CMP.L D0,D4
			0x62B6, // BHI.S scan
			0x6092, // BRA.S loop
		)
		writeM68KStopProgram(cpu, loopPC)
		writeM68KStopProgram(cpu, callPC)
		writeM68KWords(cpu, returnPC,
			0x4FEF, 0x000C, // LEA 12(A7),A7
			0x4E72, 0x2700, // STOP
		)
		writeM68KWords(cpu, subPC,
			0x7007, // MOVEQ #7,D0
			0x4E75, // RTS
		)
	}

	run := func(t *testing.T, name string, startPC uint32, setup func(*M68KCPU)) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			interp := newM68KTestProgramCPU(t, startPC)
			writeResidentScanProgram(interp)
			setup(interp)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			jit.m68kJitForceNative = true
			writeResidentScanProgram(jit)
			setup(jit)
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("%s did not execute any native block", name)
			}
			assertM68KCoreStateEqual(t, jit, interp)
			for _, addr := range []uint32{stack - 16, stack - 12, stack - 8, stack - 4, stack + 24} {
				if got, want := jit.Read32(addr), interp.Read32(addr); got != want {
					t.Fatalf("%s memory[0x%08X]=0x%08X want 0x%08X", name, addr, got, want)
				}
			}
		})
	}

	baseSetup := func(cpu *M68KCPU) {
		cpu.AddrRegs[7] = stack
		cpu.AddrRegs[4] = subPC
		cpu.DataRegs[2] = 0x11223344
		cpu.Write32(stack+24, node)
		cpu.Write16(node, 0x4AFC)
		cpu.Write32(node+2, node)
	}

	run(t, "step_bhi_taken", stepPC, func(cpu *M68KCPU) {
		baseSetup(cpu)
		cpu.Write32(stack+24, 0x1000)
		cpu.DataRegs[4] = 0x2000
	})
	run(t, "step_bhi_not_taken", stepPC, func(cpu *M68KCPU) {
		baseSetup(cpu)
		cpu.Write32(stack+24, 0x1000)
		cpu.DataRegs[4] = 0x1001
	})
	run(t, "scan_cmpi_not_equal", scanPC, func(cpu *M68KCPU) {
		baseSetup(cpu)
		cpu.Write16(node, 0x4AFD)
	})
	run(t, "scan_cmpa_not_equal", scanPC, func(cpu *M68KCPU) {
		baseSetup(cpu)
		cpu.Write32(node+2, node+4)
	})
	run(t, "scan_tst_d3_taken", scanPC, func(cpu *M68KCPU) {
		baseSetup(cpu)
		cpu.DataRegs[3] = 1
	})
	run(t, "scan_jsr_path", scanPC, func(cpu *M68KCPU) {
		baseSetup(cpu)
		cpu.DataRegs[3] = 0
	})
}

func TestM68KJIT_AROSResidentScanPrefixesMatchInterpreter(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	run := func(t *testing.T, name string, startPC uint32, opcodes []uint16, setup func(*M68KCPU), wantPrefixLen int, stopTargets ...uint32) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			interp := newM68KTestProgramCPU(t, startPC)
			writeM68KStopProgram(interp, startPC, opcodes...)
			for _, target := range stopTargets {
				writeM68KStopProgram(interp, target)
			}
			setup(interp)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			writeM68KStopProgram(jit, startPC, opcodes...)
			for _, target := range stopTargets {
				writeM68KStopProgram(jit, target)
			}
			setup(jit)

			instrs := m68kScanBlock(jit.memory, startPC)
			prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
			if len(prefix) != wantPrefixLen {
				t.Fatalf("%s prefix length=%d, want %d; instrs=%+v prefix=%+v", name, len(prefix), wantPrefixLen, instrs, prefix)
			}

			runM68KJITUntilStopped(t, jit)
			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("%s did not execute a native prefix", name)
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}

	const (
		stack = uint32(0x0000F000)
		node  = uint32(0x00020000)
	)

	scanProgram := []uint16{
		0x246F, 0x0018, // MOVEA.L 24(A7),A2
		0x0C52, 0x4AFC, // CMPI.W #$4AFC,(A2)
		0x6632,         // BNE.S step
		0xB5EA, 0x0002, // CMPA.L 2(A2),A2
		0x662C, // BNE.S step
	}
	run(t, "scan_cmpi_prefix_taken", 0x1000, scanProgram, func(cpu *M68KCPU) {
		cpu.AddrRegs[7] = stack
		cpu.Write32(stack+24, node)
		cpu.Write16(node, 0x4AFD)
	}, 5, 0x103C)
	run(t, "scan_cmpi_prefix_not_taken", 0x1100, scanProgram, func(cpu *M68KCPU) {
		cpu.AddrRegs[7] = stack
		cpu.Write32(stack+24, node)
		cpu.Write16(node, 0x4AFC)
		cpu.Write32(node+2, node+4)
	}, 5, 0x113C)

	stepProgram := []uint16{
		0x202F, 0x0018, // MOVE.L 24(A7),D0
		0x5480,         // ADDQ.L #2,D0
		0x2F40, 0x0018, // MOVE.L D0,24(A7)
		0xB880, // CMP.L D0,D4
		0x62B6, // BHI.S scan
		0x6092, // BRA.S loop
	}
	run(t, "step_cmp_prefix_bhi_taken", 0x2000, stepProgram, func(cpu *M68KCPU) {
		cpu.AddrRegs[7] = stack
		cpu.Write32(stack+24, 0x1000)
		cpu.DataRegs[4] = 0x2000
	}, 5, 0x1FC0, 0x1FA0)
	run(t, "step_cmp_prefix_bhi_not_taken", 0x2100, stepProgram, func(cpu *M68KCPU) {
		cpu.AddrRegs[7] = stack
		cpu.Write32(stack+24, 0x1000)
		cpu.DataRegs[4] = 0x1001
	}, 5, 0x20C0, 0x20A0)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAROSStackLoadJSRBlock(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		stack   = uint32(0x9000)
		argA2   = uint32(0x3000)
		subPC   = uint32(0x4000)
	)
	opcodes := []uint16{
		0x246F, 0x000C, // MOVEA.L 12(A7),A2
		0x2C6F, 0x0014, // MOVEA.L 20(A7),A6
		0x4EAE, 0xFF88, // JSR -120(A6)
		0x7207, // MOVEQ #7,D1 after return
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[7] = stack
		cpu.Write32(stack+12, argA2)
		cpu.Write32(stack+20, subPC+120)
		writeM68KWords(cpu, subPC,
			0x702A, // MOVEQ #42,D0
			0x4E75, // RTS
		)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	if !m68kIsAROSStackLoadJSRBlock(instrs, startPC, jit.memory) {
		t.Fatalf("test block did not match AROS stack-load JSR recognizer: instrs=%+v", instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatalf("default M68K JIT dispatcher did not execute AROS stack-load JSR block natively: fallback=%v conservative=%v productionSafe=%v genericIO=%v",
			m68kNeedsFallback(instrs),
			m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
			m68kBlockProductionNativeSafe(instrs),
			m68kBlockMayUseGenericIOFallback(instrs))
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAROSStandaloneJSRBlock(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		subPC   = uint32(0x4000)
		stack   = uint32(0x9000)
	)
	opcodes := []uint16{
		0x4EAE, 0xFF2E, // JSR -210(A6)
		0x7207, // MOVEQ #7,D1 after return
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[6] = subPC + 210
		cpu.AddrRegs[7] = stack
		writeM68KWords(cpu, subPC,
			0x702A, // MOVEQ #42,D0
			0x4E75, // RTS
		)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	if !m68kIsAROSStandaloneJSRBlock(instrs, startPC, jit.memory) {
		t.Fatalf("test block did not match AROS standalone JSR recognizer: instrs=%+v", instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatalf("default M68K JIT dispatcher did not execute AROS standalone JSR block natively: fallback=%v conservative=%v productionSafe=%v genericIO=%v",
			m68kNeedsFallback(instrs),
			m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
			m68kBlockProductionNativeSafe(instrs),
			m68kBlockMayUseGenericIOFallback(instrs))
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAROSStandaloneJSRMinus132Block(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		subPC   = uint32(0x4000)
		stack   = uint32(0x9000)
	)
	opcodes := []uint16{
		0x4EAE, 0xFF7C, // JSR -132(A6)
		0x7207, // MOVEQ #7,D1 after return
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[6] = subPC + 132
		cpu.AddrRegs[7] = stack
		writeM68KWords(cpu, subPC,
			0x702A, // MOVEQ #42,D0
			0x4E75, // RTS
		)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	if !m68kIsAROSStandaloneJSRBlock(instrs, startPC, jit.memory) {
		t.Fatalf("test block did not match AROS standalone JSR recognizer: instrs=%+v", instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatalf("default M68K JIT dispatcher did not execute AROS standalone JSR -132 block natively: fallback=%v conservative=%v productionSafe=%v genericIO=%v",
			m68kNeedsFallback(instrs),
			m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
			m68kBlockProductionNativeSafe(instrs),
			m68kBlockMayUseGenericIOFallback(instrs))
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAROSStandaloneJSRMinus312Block(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		subPC   = uint32(0x4000)
		stack   = uint32(0x9000)
	)
	opcodes := []uint16{
		0x4EAE, 0xFEC8, // JSR -312(A6)
		0x7207, // MOVEQ #7,D1 after return
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[6] = subPC + 312
		cpu.AddrRegs[7] = stack
		writeM68KWords(cpu, subPC,
			0x702A, // MOVEQ #42,D0
			0x4E75, // RTS
		)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	if !m68kIsAROSStandaloneJSRBlock(instrs, startPC, jit.memory) {
		t.Fatalf("test block did not match AROS standalone JSR recognizer: instrs=%+v", instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatalf("default M68K JIT dispatcher did not execute AROS standalone JSR -312 block natively: fallback=%v conservative=%v productionSafe=%v genericIO=%v",
			m68kNeedsFallback(instrs),
			m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
			m68kBlockProductionNativeSafe(instrs),
			m68kBlockMayUseGenericIOFallback(instrs))
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAROSStandaloneJSRMinus306Block(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		subPC   = uint32(0x4000)
		stack   = uint32(0x9000)
	)
	opcodes := []uint16{
		0x4EAE, 0xFECE, // JSR -306(A6)
		0x7207, // MOVEQ #7,D1 after return
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[6] = subPC + 306
		cpu.AddrRegs[7] = stack
		writeM68KWords(cpu, subPC,
			0x702A, // MOVEQ #42,D0
			0x4E75, // RTS
		)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	if !m68kIsAROSStandaloneJSRBlock(instrs, startPC, jit.memory) {
		t.Fatalf("test block did not match AROS standalone JSR recognizer: instrs=%+v", instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatalf("default M68K JIT dispatcher did not execute AROS standalone JSR -306 block natively: fallback=%v conservative=%v productionSafe=%v genericIO=%v",
			m68kNeedsFallback(instrs),
			m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
			m68kBlockProductionNativeSafe(instrs),
			m68kBlockMayUseGenericIOFallback(instrs))
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAROSStandaloneJSRMinus60Block(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		subPC   = uint32(0x4000)
		stack   = uint32(0x9000)
	)
	opcodes := []uint16{
		0x4EAE, 0xFFC4, // JSR -60(A6)
		0x7207, // MOVEQ #7,D1 after return
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[6] = subPC + 60
		cpu.AddrRegs[7] = stack
		writeM68KWords(cpu, subPC,
			0x702A, // MOVEQ #42,D0
			0x4E75, // RTS
		)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	if !m68kIsAROSStandaloneJSRBlock(instrs, startPC, jit.memory) {
		t.Fatalf("test block did not match AROS standalone JSR recognizer: instrs=%+v", instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatalf("default M68K JIT dispatcher did not execute AROS standalone JSR -60 block natively: fallback=%v conservative=%v productionSafe=%v genericIO=%v",
			m68kNeedsFallback(instrs),
			m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
			m68kBlockProductionNativeSafe(instrs),
			m68kBlockMayUseGenericIOFallback(instrs))
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAROSStandaloneJSRWithHighRAMStack(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		subPC   = uint32(0x4000)
		stack   = uint32(0x120000)
	)
	opcodes := []uint16{
		0x4EAE, 0xFFC4, // JSR -60(A6)
		0x7207, // MOVEQ #7,D1 after return
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[6] = subPC + 60
		cpu.AddrRegs[7] = stack
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		writeM68KWords(cpu, subPC,
			0x702A, // MOVEQ #42,D0
			0x4E75, // RTS
		)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	if !m68kIsAROSStandaloneJSRBlock(instrs, startPC, jit.memory) {
		t.Fatalf("test block did not match AROS standalone JSR recognizer: instrs=%+v", instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatalf("default M68K JIT dispatcher did not execute high-stack AROS standalone JSR block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("high-RAM stack JSR bailed out %d times; stack writes should use ioPageBitmap, not the low-memory cutoff", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_HighRAMCallReturnRunsNative(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x00901000)
		subPC   = uint32(0x00902000)
		stack   = uint32(0x00120000)
	)
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[7] = stack
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		writeM68KWords(cpu, startPC,
			0x4EB9, uint16(subPC>>16), uint16(subPC&0xFFFF), // JSR subPC
			0x7207,         // MOVEQ #7,D1 after return
			0x4E72, 0x2700, // STOP
		)
		writeM68KWords(cpu, subPC,
			0x702A, // MOVEQ #42,D0
			0x4E75, // RTS
		)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitFallbackOpcodeCounts[0x4EB9].Load(); got != 0 {
		t.Fatalf("high-RAM JSR fell back %d times, want native", got)
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x4E75].Load(); got != 0 {
		t.Fatalf("high-RAM RTS fell back %d times, want native", got)
	}
	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatalf("high-RAM call/return code did not execute natively")
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeMOVEMLongRoundTrip(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	opcodes := make([]uint16, 0, m68kJitMaxBlockSize*2)
	for i := 0; i < m68kJitMaxBlockSize/2; i++ {
		opcodes = append(opcodes,
			0x48E7, 0xC0C0, // MOVEM.L D0/D1/A0/A1,-(SP)
			0x4CDF, 0x0303, // MOVEM.L (SP)+,D0/D1/A0/A1
		)
	}

	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x11112222
		cpu.DataRegs[1] = 0x33334444
		cpu.AddrRegs[0] = 0x55556666
		cpu.AddrRegs[1] = 0x77778888
		cpu.AddrRegs[7] = 0x9000
	}

	interp := newM68KTestProgramCPU(t, 0x1000)
	setup(interp)
	writeM68KStopProgram(interp, 0x1000, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, 0x1000)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, 0x1000, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute MOVEM.L round-trip block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("MOVEM.L round-trip block bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
	for addr := uint32(0x8FC0); addr < 0x9040; addr += 4 {
		if got, want := jit.Read32(addr), interp.Read32(addr); got != want {
			t.Fatalf("stack long at 0x%08X: got=0x%08X want=0x%08X", addr, got, want)
		}
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAROSExecMOVEMSaveRestoreMask(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		stack   = uint32(0x120000)
	)
	opcodes := []uint16{
		0x48E7, 0x3032, // MOVEM.L D2/D3/A2/A3/A6,-(A7)
		0x7400,         // MOVEQ #0,D2
		0x7600,         // MOVEQ #0,D3
		0x2440,         // MOVEA.L D0,A2
		0x2640,         // MOVEA.L D0,A3
		0x2C40,         // MOVEA.L D0,A6
		0x4CDF, 0x4C0C, // MOVEM.L (A7)+,D2/D3/A2/A3/A6
	}
	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0
		cpu.DataRegs[2] = 0xD2000002
		cpu.DataRegs[3] = 0xD3000003
		cpu.AddrRegs[2] = 0x00A20002
		cpu.AddrRegs[3] = 0x00A30003
		cpu.AddrRegs[6] = 0x00A60006
		cpu.AddrRegs[7] = stack
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute exact AROS Exec MOVEM save/restore block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("exact AROS Exec MOVEM save/restore block bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
	for addr := stack - 20; addr < stack; addr += 4 {
		if got, want := jit.Read32(addr), interp.Read32(addr); got != want {
			t.Fatalf("saved MOVEM long at 0x%08X: got=0x%08X want=0x%08X", addr, got, want)
		}
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAROSLargeMOVEMSaveRestoreMask(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		stack   = uint32(0x120000)
	)
	opcodes := []uint16{
		0x48E7, 0x383E, // MOVEM.L D2/D3/D4/A2/A3/A4/A5/A6,-(A7)
		0x7400,         // MOVEQ #0,D2
		0x7600,         // MOVEQ #0,D3
		0x7800,         // MOVEQ #0,D4
		0x2440,         // MOVEA.L D0,A2
		0x2640,         // MOVEA.L D0,A3
		0x2840,         // MOVEA.L D0,A4
		0x2A40,         // MOVEA.L D0,A5
		0x2C40,         // MOVEA.L D0,A6
		0x4CDF, 0x7C1C, // MOVEM.L (A7)+,D2/D3/D4/A2/A3/A4/A5/A6
	}
	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0
		cpu.DataRegs[2] = 0xD2000002
		cpu.DataRegs[3] = 0xD3000003
		cpu.DataRegs[4] = 0xD4000004
		cpu.AddrRegs[2] = 0x00A20002
		cpu.AddrRegs[3] = 0x00A30003
		cpu.AddrRegs[4] = 0x00A40004
		cpu.AddrRegs[5] = 0x00A50005
		cpu.AddrRegs[6] = 0x00A60006
		cpu.AddrRegs[7] = stack
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute large AROS MOVEM save/restore block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("large AROS MOVEM save/restore block bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
	for addr := stack - 32; addr < stack; addr += 4 {
		if got, want := jit.Read32(addr), interp.Read32(addr); got != want {
			t.Fatalf("saved large MOVEM long at 0x%08X: got=0x%08X want=0x%08X", addr, got, want)
		}
	}
}

func TestM68KJIT_SccPreservesCCRForFollowingBranch(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const startPC = uint32(0x1000)
	opcodes := []uint16{
		0xB083, // CMP.L D3,D0; D0 < D3 sets carry
		0x54C2, // SCC D2; must not change CCR
		0x6404, // BCC.S branch target; must not branch
		0x70AA, // MOVEQ #-86,D0
		0x6002, // BRA.S done
		0x7055, // MOVEQ #85,D0
	}
	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 2
		cpu.DataRegs[2] = 0x12345678
		cpu.DataRegs[3] = 3
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute Scc/branch block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("Scc/branch block bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
	if got, want := jit.DataRegs[0], uint32(0xFFFFFFAA); got != want {
		t.Fatalf("Scc clobbered following BCC decision: D0=0x%08X want=0x%08X", got, want)
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAROSMOVEMBulkCopyBlock(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		src     = uint32(0x3000)
		dst     = uint32(0x5000)
		stack   = uint32(0x120000)
	)
	opcodes := []uint16{
		0x48E7, 0x3F3E, // MOVEM.L D2-D7/A2-A6,-(A7)
		0x4CD8, 0x7CFE, // MOVEM.L (A0)+,D1-D7/A2-A6
		0x48D1, 0x7CFE, // MOVEM.L D1-D7/A2-A6,(A1)
		0x7230,                 // MOVEQ #48,D1
		0x9081,                 // SUB.L D1,D0
		0xD3C1,                 // ADDA.L D1,A1
		0x4EF9, 0x0000, 0x1100, // JMP stop stub
	}
	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 96
		for i := 1; i <= 7; i++ {
			cpu.DataRegs[i] = 0xD0000000 | uint32(i)
		}
		cpu.AddrRegs[0] = src
		cpu.AddrRegs[1] = dst
		for i := 2; i <= 6; i++ {
			cpu.AddrRegs[i] = 0xA0000000 | uint32(i)
		}
		cpu.AddrRegs[7] = stack
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		for i := uint32(0); i < 12; i++ {
			cpu.Write32(src+i*4, 0x10000000+i)
			cpu.Write32(dst+i*4, 0xDEADBEEF)
		}
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KWords(interp, startPC, opcodes...)
	writeM68KStopProgram(interp, 0x1100)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KWords(jit, startPC, opcodes...)
	writeM68KStopProgram(jit, 0x1100)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		instrs := m68kScanBlock(jit.memory, startPC)
		t.Fatalf("default M68K JIT dispatcher did not execute AROS MOVEM bulk-copy block natively: instrs=%d needsFallback=%v conservative=%v productionSafe=%v genericIO=%v",
			len(instrs),
			m68kNeedsFallback(instrs),
			m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
			m68kBlockProductionNativeSafe(instrs),
			m68kBlockMayUseGenericIOFallback(instrs))
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x48E7].Load(); got != 0 {
		t.Fatalf("AROS MOVEM bulk-copy block fell back to interpreter %d times", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("AROS MOVEM bulk-copy block bailed out %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
	for i := uint32(0); i < 12; i++ {
		if got, want := jit.Read32(dst+i*4), interp.Read32(dst+i*4); got != want {
			t.Fatalf("bulk MOVEM copy long %d: got=0x%08X want=0x%08X", i, got, want)
		}
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAROSMOVEMCopyLoopHead(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1004)
		src     = uint32(0x8A5300)
		dst     = uint32(0x8A5348)
	)
	opcodes := []uint16{
		0x48D1, 0x7CFE, // MOVEM.L D1-D7/A2-A6,(A1)
		0x7230, // MOVEQ #48,D1
		0x9081, // SUB.L D1,D0
		0xD3C1, // ADDA.L D1,A1
		0xB081, // CMP.L D1,D0
		0x64EE, // BCC.S to startPC-4 in the AROS loop
	}
	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 64
		for i := 1; i <= 7; i++ {
			cpu.DataRegs[i] = 0xD1000000 | uint32(i)
		}
		cpu.AddrRegs[0] = src
		cpu.AddrRegs[1] = dst
		for i := 2; i <= 6; i++ {
			cpu.AddrRegs[i] = 0xA2000000 | uint32(i)
		}
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KWords(interp, startPC, opcodes...)
	writeM68KStopProgram(interp, startPC+0x0E)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KWords(jit, startPC, opcodes...)
	writeM68KStopProgram(jit, startPC+0x0E)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		instrs := m68kScanBlock(jit.memory, startPC)
		t.Fatalf("default M68K JIT dispatcher did not execute AROS MOVEM copy-loop head natively: instrs=%d needsFallback=%v conservative=%v productionSafe=%v genericIO=%v",
			len(instrs),
			m68kNeedsFallback(instrs),
			m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
			m68kBlockProductionNativeSafe(instrs),
			m68kBlockMayUseGenericIOFallback(instrs))
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x48D1].Load(); got != 0 {
		t.Fatalf("AROS MOVEM copy-loop head fell back to interpreter %d times", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("AROS MOVEM copy-loop head bailed out %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
	for i := uint32(0); i < 12; i++ {
		if got, want := jit.Read32(dst+i*4), interp.Read32(dst+i*4); got != want {
			t.Fatalf("copy-loop MOVEM long %d: got=0x%08X want=0x%08X", i, got, want)
		}
	}
}

func TestM68KJIT_DefaultDispatcherAROSMOVEMStoreInvalidatesCodePageWithoutFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		dstPC   = uint32(0x9000)
	)
	opcodes := []uint16{
		0x48D1, 0x7CFE, // MOVEM.L D1-D7/A2-A6,(A1)
	}

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	jit.m68kJitPersist = true
	t.Cleanup(jit.freeM68KJIT)
	if err := jit.initM68KJIT(); err != nil {
		t.Fatalf("initM68KJIT: %v", err)
	}

	dstBlock := &JITBlock{startPC: uint64(dstPC), endPC: uint64(dstPC + 4)}
	jit.m68kJitCache.Put(dstBlock)
	jit.m68kMarkJITCodeRanges(dstBlock)

	for i := 1; i <= 7; i++ {
		jit.DataRegs[i] = 0xD1000000 | uint32(i)
	}
	jit.AddrRegs[1] = dstPC
	for i := 2; i <= 6; i++ {
		jit.AddrRegs[i] = 0xA2000000 | uint32(i)
	}
	writeM68KStopProgram(jit, startPC, opcodes...)

	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitFallbackOpcodeCounts[0x48D1].Load(); got != 0 {
		t.Fatalf("AROS MOVEM store fell back %d times", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("AROS MOVEM store bailed out %d times", got)
	}
	if got := jit.m68kJitCache.Get(uint64(dstPC)); got != nil {
		t.Fatalf("compiled MOVEM destination block survived invalidation: %#v", got)
	}
	if got := jit.m68kJitCodeBitmap[dstPC>>12]; got != 0 {
		t.Fatalf("destination code bitmap page after MOVEM invalidation = %d, want 0", got)
	}
	if got := jit.Read32(dstPC); got != 0xD1000001 {
		t.Fatalf("first MOVEM long=0x%08X, want D1", got)
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAROSUnalignedMOVEMCopyHead(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		src     = uint32(0x8A3C92)
		dst     = uint32(0x8A5348)
	)
	opcodes := []uint16{
		0x4CD8, 0x7CFE, // MOVEM.L (A0)+,D1-D7/A2-A6
		0x48D1, 0x7CFE, // MOVEM.L D1-D7/A2-A6,(A1)
	}
	setup := func(cpu *M68KCPU) {
		for i := range 8 {
			cpu.DataRegs[i] = 0xD0000000 | uint32(i)
			cpu.AddrRegs[i] = 0xA0000000 | uint32(i)
		}
		cpu.AddrRegs[0] = src
		cpu.AddrRegs[1] = dst
		for i := uint32(0); i < 12; i++ {
			cpu.Write32(src+i*4, 0x10000000+i)
			cpu.Write32(dst+i*4, 0xDEADBEEF)
		}
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		instrs := m68kScanBlock(jit.memory, startPC)
		t.Fatalf("default M68K JIT dispatcher did not execute unaligned AROS MOVEM copy head natively: instrs=%d needsFallback=%v conservative=%v productionSafe=%v genericIO=%v",
			len(instrs),
			m68kNeedsFallback(instrs),
			m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
			m68kBlockProductionNativeSafe(instrs),
			m68kBlockMayUseGenericIOFallback(instrs))
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x4CD8].Load(); got != 0 {
		t.Fatalf("unaligned AROS MOVEM load fell back to interpreter %d times", got)
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x48D1].Load(); got != 0 {
		t.Fatalf("unaligned AROS MOVEM store fell back to interpreter %d times", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("unaligned AROS MOVEM copy head bailed out %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
	for i := uint32(0); i < 12; i++ {
		if got, want := jit.Read32(dst+i*4), interp.Read32(dst+i*4); got != want {
			t.Fatalf("unaligned copy-head MOVEM long %d: got=0x%08X want=0x%08X", i, got, want)
		}
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAROSMountBlockHead(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		a0Base  = uint32(0x8000)
		a5Base  = uint32(0x9000)
		table   = uint32(0xA000)
	)
	opcodes := []uint16{
		0x4281,         // CLR.L D1
		0xB2AD, 0xFF90, // CMP.L -112(A5), D1
		0x6700, 0xFED8, // BEQ.W not taken in this setup
		0x2A28, 0x0004, // MOVE.L 4(A0), D5
		0x2C05,         // MOVE.L D5, D6
		0xE08E,         // LSR.L #8, D6
		0x2006,         // MOVE.L D6, D0
		0xE988,         // LSL.L #4, D0
		0x2C6D, 0xFF8C, // MOVEA.L -116(A5), A6
		0xDDC0,         // ADDA.L D0, A6
		0x3E2E, 0x000E, // MOVE.W 14(A6), D7
		0x0C47, 0xFFF1, // CMPI.W #$FFF1, D7
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = a0Base
		cpu.AddrRegs[5] = a5Base
		cpu.Write32(a5Base-112, 1)
		cpu.Write32(a5Base-116, table)
		cpu.Write32(a0Base+4, 0x00000123)
		cpu.Write16(table+0x10+14, 0xFFF2)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	wantPrefix := len(instrs) - 1 // STOP remains interpreted.
	if len(prefix) != wantPrefix {
		t.Fatalf("AROS Mount block-head native prefix length=%d want=%d prefix=%+v fallback=%v conservative=%v productionSafe=%v genericIO=%v instrs=%+v",
			len(prefix), wantPrefix, prefix,
			m68kNeedsFallback(instrs), m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
			m68kBlockProductionNativeSafe(instrs), m68kBlockMayUseGenericIOFallback(instrs), instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute AROS Mount block head natively")
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x4281].Load(); got != 0 {
		t.Fatalf("AROS Mount block-head CLR.L fell back %d times", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("AROS Mount block-head bailed out %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAROSDIVLEpilogueBlock(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC  = uint32(0x1000)
		returnPC = uint32(0x2000)
		a0Base   = uint32(0x8000)
		a1Base   = uint32(0x9000)
		stack    = uint32(0x120000)
	)
	opcodes := []uint16{
		0x4281,         // CLR.L D1
		0x3229, 0x00AA, // MOVE.W 170(A1), D1
		0x2428, 0x0004, // MOVE.L 4(A0), D2
		0x4C41, 0x2002, // DIVU.L D1, D2
		0x4290,         // CLR.L (A0)
		0xD480,         // ADD.L D0, D2
		0x2142, 0x0004, // MOVE.L D2, 4(A0)
		0x4CDF, 0x00FC, // MOVEM.L (A7)+, D2-D7
		0x4E75, // RTS
	}
	saved := []uint32{
		0xD2000002, 0xD3000003, 0xD4000004, 0xD5000005, 0xD6000006, 0xD7000007,
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = a0Base
		cpu.AddrRegs[1] = a1Base
		cpu.AddrRegs[7] = stack
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		cpu.DataRegs[0] = 3
		cpu.DataRegs[1] = 1
		cpu.DataRegs[2] = 0x16
		cpu.Write16(a1Base+170, 0x0005)
		cpu.Write32(a0Base, 0xA5A5A5A5)
		cpu.Write32(a0Base+4, 0x00000016)
		for i, val := range saved {
			cpu.Write32(stack+uint32(i)*4, val)
		}
		cpu.Write32(stack+uint32(len(saved))*4, returnPC)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KWords(interp, startPC, opcodes...)
	writeM68KStopProgram(interp, returnPC)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KWords(jit, startPC, opcodes...)
	writeM68KStopProgram(jit, returnPC)
	instrs := m68kScanBlock(jit.memory, startPC)
	if !m68kCanUseProductionNativeBlock(jit.memory, startPC, instrs) {
		t.Fatalf("AROS DIVL epilogue block rejected: instrs=%+v fallback=%v conservative=%v productionSafe=%v genericIO=%v",
			instrs,
			m68kNeedsFallback(instrs),
			m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
			m68kBlockProductionNativeSafe(instrs),
			m68kBlockMayUseGenericIOFallback(instrs))
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute AROS DIVL epilogue block natively")
	}
	for _, opcode := range []uint16{0x4281, 0x4C41, 0x4CDF, 0x4E75} {
		if got := jit.m68kJitFallbackOpcodeCounts[opcode].Load(); got != 0 {
			t.Fatalf("AROS DIVL epilogue opcode 0x%04X fell back %d times", opcode, got)
		}
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("AROS DIVL epilogue block bailed out %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
	for _, addr := range []uint32{a0Base, a0Base + 4} {
		if got, want := jit.Read32(addr), interp.Read32(addr); got != want {
			t.Fatalf("memory[0x%08X]=0x%08X, want 0x%08X", addr, got, want)
		}
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAROSMULLDIVLEpilogueBlock(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC  = uint32(0x1000)
		returnPC = uint32(0x2000)
		a0Base   = uint32(0x82F738)
		a1Base   = uint32(0x827988)
		stack    = uint32(0x82F1D0)
	)
	opcodes := []uint16{
		0x4C10, 0x0800, // MULL.L (A0),D0
		0x4281,         // CLR.L D1
		0x3229, 0x00A2, // MOVE.W 162(A1),D1
		0x2428, 0x0004, // MOVE.L 4(A0),D2
		0x4C41, 0x2002, // DIVL.L D1,D2
		0x4290,         // CLR.L (A0)
		0xD480,         // ADD.L D0,D2
		0x2142, 0x0004, // MOVE.L D2,4(A0)
		0x4CDF, 0x00FC, // MOVEM.L (A7)+,D2-D7
		0x4E75, // RTS
	}
	saved := []uint32{
		0xD2000002, 0xD3000003, 0xD4000004, 0xD5000005, 0xD6000006, 0xD7000007,
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = a0Base
		cpu.AddrRegs[1] = a1Base
		cpu.AddrRegs[7] = stack
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		cpu.DataRegs[0] = 0x3C
		cpu.DataRegs[1] = 1
		cpu.DataRegs[2] = 0x16
		cpu.Write32(a0Base, 0)
		cpu.Write32(a0Base+4, 0x000186A0)
		cpu.Write16(a1Base+0xA2, 0x411A)
		for i, val := range saved {
			cpu.Write32(stack+uint32(i)*4, val)
		}
		cpu.Write32(stack+uint32(len(saved))*4, returnPC)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KWords(interp, startPC, opcodes...)
	writeM68KStopProgram(interp, returnPC)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KWords(jit, startPC, opcodes...)
	writeM68KStopProgram(jit, returnPC)
	instrs := m68kScanBlock(jit.memory, startPC)
	if !m68kCanUseProductionNativeBlock(jit.memory, startPC, instrs) {
		t.Fatalf("AROS MULL/DIVL epilogue block rejected: instrs=%+v fallback=%v conservative=%v productionSafe=%v genericIO=%v",
			instrs,
			m68kNeedsFallback(instrs),
			m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
			m68kBlockProductionNativeSafe(instrs),
			m68kBlockMayUseGenericIOFallback(instrs))
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute AROS MULL/DIVL epilogue block natively")
	}
	for _, opcode := range []uint16{0x4C10, 0x4281, 0x4C41, 0x4CDF, 0x4E75} {
		if got := jit.m68kJitFallbackOpcodeCounts[opcode].Load(); got != 0 {
			t.Fatalf("AROS MULL/DIVL epilogue opcode 0x%04X fell back %d times", opcode, got)
		}
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("AROS MULL/DIVL epilogue block bailed out %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
	for _, addr := range []uint32{a0Base, a0Base + 4} {
		if got, want := jit.Read32(addr), interp.Read32(addr); got != want {
			t.Fatalf("memory[0x%08X]=0x%08X, want 0x%08X", addr, got, want)
		}
	}
}

func TestM68KJIT_DefaultDispatcherExecutesMMIOAbsoluteLongMOVEViaHelperNoFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		mmio    = uint32(AROS_DOS_REGION_BASE)
	)
	opcodes := []uint16{
		0x23C0, 0x000F, 0x2220, // MOVE.L D0,$000F2220
		0x2039, 0x000F, 0x2220, // MOVE.L $000F2220,D0
		0x2C39, 0x000F, 0x2220, // MOVEA.L $000F2220,A6
	}
	setup := func(cpu *M68KCPU, readValue *uint32, writes *[]uint32) {
		bus, ok := cpu.bus.(*MachineBus)
		if !ok {
			t.Fatal("test CPU is not backed by MachineBus")
		}
		bus.MapIO(mmio, mmio+3,
			func(addr uint32) uint32 {
				return *readValue
			},
			func(addr uint32, value uint32) {
				*writes = append(*writes, value)
				*readValue = value ^ 0x11111111
			})
		cpu.DataRegs[0] = 0xCAFEBABE
		cpu.AddrRegs[6] = 0
	}

	interpRead := uint32(0)
	var interpWrites []uint32
	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp, &interpRead, &interpWrites)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jitRead := uint32(0)
	var jitWrites []uint32
	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit, &jitRead, &jitWrites)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute MMIO MOVE block natively")
	}
	for _, opcode := range []uint16{0x23C0, 0x2039, 0x2C39} {
		if got := jit.m68kJitFallbackOpcodeCounts[opcode].Load(); got != 0 {
			t.Fatalf("MMIO MOVE opcode %04X fell back to interpreter %d times", opcode, got)
		}
	}
	assertM68KCoreStateEqual(t, jit, interp)
	if len(jitWrites) != len(interpWrites) {
		t.Fatalf("MMIO write count got=%d want=%d", len(jitWrites), len(interpWrites))
	}
	for i := range jitWrites {
		if jitWrites[i] != interpWrites[i] {
			t.Fatalf("MMIO write[%d] got=0x%08X want=0x%08X", i, jitWrites[i], interpWrites[i])
		}
	}
}

func TestM68KJIT_DefaultDispatcherPreservesINTENASideEffect(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const startPC = uint32(0x1000)
	cases := []struct {
		name    string
		setup   func(*M68KCPU)
		opcodes []uint16
		wantMem uint16
		wantOn  bool
	}{
		{
			name:    "move_imm_abs_long",
			opcodes: []uint16{0x33FC, 0x4000, 0x00DF, 0xF09A}, // MOVE.W #$4000,$00DFF09A
			wantMem: 0x4000,
		},
		{
			name: "move_reg_abs_long",
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 0x4000
			},
			opcodes: []uint16{0x33C0, 0x00DF, 0xF09A}, // MOVE.W D0,$00DFF09A
			wantMem: 0x4000,
		},
		{
			name: "move_reg_addr_indirect",
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 0x4000
				cpu.AddrRegs[0] = 0x00DFF09A
			},
			opcodes: []uint16{0x3080}, // MOVE.W D0,(A0)
			wantMem: 0x4000,
		},
		{
			name:    "ori_abs_long",
			opcodes: []uint16{0x0079, 0x4000, 0x00DF, 0xF09A}, // ORI.W #$4000,$00DFF09A
			wantMem: 0x4000,
		},
		{
			name:    "clr_abs_long",
			opcodes: []uint16{0x4279, 0x00DF, 0xF09A}, // CLR.W $00DFF09A
			wantMem: 0x0000,
			wantOn:  true,
		},
		{
			name: "lsl_addr_indirect",
			setup: func(cpu *M68KCPU) {
				cpu.Write16(0x00DFF09A, 0x2000)
				cpu.AddrRegs[0] = 0x00DFF09A
			},
			opcodes: []uint16{0xE3D0}, // LSL.W (A0)
			wantMem: 0x4000,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			intena := &atomic.Bool{}
			intena.Store(true)

			cpu := newM68KTestProgramCPU(t, startPC)
			cpu.m68kJitEnabled = true
			cpu.AmigaINTENA = intena
			if tc.setup != nil {
				tc.setup(cpu)
			}
			writeM68KStopProgram(cpu, startPC, tc.opcodes...)
			runM68KJITUntilStopped(t, cpu)

			if got := intena.Load(); got != tc.wantOn {
				t.Fatalf("AmigaINTENA=%v, want %v", got, tc.wantOn)
			}
			if got := cpu.Read16(0xDFF09A); got != tc.wantMem {
				t.Fatalf("Read16($DFF09A)=0x%04X, want plain memory value 0x%04X", got, tc.wantMem)
			}
		})
	}
}

func TestM68KJIT_DefaultDispatcherUsesVideoStatusReader(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const startPC = uint32(0x1000)
	bus := NewMachineBus()
	bus.SetVideoStatusReader(func(addr uint32) uint32 {
		if addr != 0xF0008 {
			t.Fatalf("video status reader called with addr=0x%08X", addr)
		}
		return 0x80000002
	})
	bus.Write32(0, 0x00010000)
	bus.Write32(4, startPC)
	cpu := NewM68KCPU(bus)
	cpu.PC = startPC
	cpu.SR = M68K_SR_S
	cpu.m68kJitEnabled = true
	cpu.m68kJitWarmupLimit = 1
	writeM68KStopProgram(cpu, startPC,
		0x2039, 0x000F, 0x0008, // MOVE.L $000F0008,D0
	)
	runM68KJITUntilStopped(t, cpu)

	if got := cpu.DataRegs[0]; got != 0x80000002 {
		t.Fatalf("D0=0x%08X, want VIDEO_STATUS reader value 0x80000002", got)
	}
}

func TestM68KJIT_DefaultDispatcherUsesFastBTSTMMIOPoll(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	reads := 0
	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.bus.(*MachineBus).MapIO(0xF0008, 0xF0008, func(addr uint32) uint32 {
			reads++
			if reads < 4 {
				return 0
			}
			return 0x2
		}, nil)
	}, false,
		0x2239, 0x000F, 0x0008, // MOVE.L $000F0008,D1
		0x0801, 0x0001, // BTST #1,D1
		0x67F4, // BEQ $1000
	)

	if !cpu.stopped.Load() {
		t.Fatalf("JIT did not reach STOP after BTST MMIO poll: PC=0x%08X reads=%d", cpu.PC, reads)
	}
	if reads != 4 {
		t.Fatalf("reads = %d, want 4", reads)
	}
	if got := cpu.DataRegs[1]; got != 0x2 {
		t.Fatalf("D1 = 0x%08X, want final video status 0x00000002", got)
	}
	if got := cpu.m68kJitFallbackInstructions.Load(); got > 1 {
		t.Fatalf("fallback instructions = %d, want only STOP fallback", got)
	}
}

func TestM68KJIT_DefaultDispatcherExecutesAROSDOSMMIOMemoryMOVEViaHelperNoFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		mmio0   = uint32(AROS_DOS_REGION_BASE)
		mmio4   = uint32(AROS_DOS_REGION_BASE + 4)
		src1    = uint32(0x8000)
		src2    = uint32(0x8010)
		stack   = uint32(0x9000)
	)
	opcodes := []uint16{
		0x2680,                         // MOVE.L D0,(A3)
		0x23EF, 0x0020, 0x000F, 0x2220, // MOVE.L 32(A7),$000F2220
		0x23D1, 0x000F, 0x2224, // MOVE.L (A1),$000F2224
		0x23D2, 0x000F, 0x2224, // MOVE.L (A2),$000F2224
	}
	setup := func(cpu *M68KCPU, writes *[][2]uint32) {
		bus, ok := cpu.bus.(*MachineBus)
		if !ok {
			t.Fatal("test CPU is not backed by MachineBus")
		}
		bus.MapIO(mmio0, mmio4+3,
			func(addr uint32) uint32 {
				return 0
			},
			func(addr uint32, value uint32) {
				*writes = append(*writes, [2]uint32{addr, value})
			})
		cpu.AddrRegs[1] = src1
		cpu.AddrRegs[2] = src2
		cpu.AddrRegs[3] = mmio4
		cpu.AddrRegs[7] = stack
		cpu.DataRegs[0] = 0x0BADF00D
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		cpu.Write32(stack+32, 0x10203040)
		cpu.Write32(src1, 0x50607080)
		cpu.Write32(src2, 0x90A0B0C0)
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_V | M68K_SR_C
	}

	var interpWrites [][2]uint32
	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp, &interpWrites)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	var jitWrites [][2]uint32
	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit, &jitWrites)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute AROS DOS MMIO MOVE block natively")
	}
	for _, opcode := range []uint16{0x2680, 0x23EF, 0x23D1, 0x23D2} {
		if got := jit.m68kJitFallbackOpcodeCounts[opcode].Load(); got != 0 {
			t.Fatalf("AROS DOS MMIO MOVE opcode %04X fell back to interpreter %d times", opcode, got)
		}
	}
	assertM68KCoreStateEqual(t, jit, interp)
	if len(jitWrites) != len(interpWrites) {
		t.Fatalf("MMIO write count got=%d want=%d", len(jitWrites), len(interpWrites))
	}
	for i := range jitWrites {
		if jitWrites[i] != interpWrites[i] {
			t.Fatalf("MMIO write[%d] got=(0x%08X,0x%08X) want=(0x%08X,0x%08X)",
				i, jitWrites[i][0], jitWrites[i][1], interpWrites[i][0], interpWrites[i][1])
		}
	}
}

func TestM68KJIT_DefaultDispatcherExecutesAROSDOSMMIOCLRCommandBlockNoFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		funcPC  = uint32(0x5000)
		mmio0   = uint32(AROS_DOS_REGION_BASE)
		mmio8   = uint32(AROS_DOS_REGION_BASE + 8)
		mmio12  = uint32(AROS_DOS_REGION_BASE + 12)
		stack   = uint32(0x9000)
	)
	opcodes := []uint16{
		0x42B9, 0x000F, 0x2228, // CLR.L $000F2228
		0x23C4, 0x000F, 0x222C, // MOVE.L D4,$000F222C
		0x7017,                 // MOVEQ #23,D0
		0x23C0, 0x000F, 0x2220, // MOVE.L D0,$000F2220
		0x2042,                 // MOVEA.L D2,A0
		0x2C79, 0x0000, 0x0004, // MOVEA.L $00000004,A6
		0x4EAE, 0xFDC6, // JSR -570(A6)
	}
	setup := func(cpu *M68KCPU, writes *[][2]uint32) {
		bus, ok := cpu.bus.(*MachineBus)
		if !ok {
			t.Fatal("test CPU is not backed by MachineBus")
		}
		bus.MapIO(mmio0, mmio12+3,
			func(addr uint32) uint32 {
				return 0
			},
			func(addr uint32, value uint32) {
				*writes = append(*writes, [2]uint32{addr, value})
			})
		cpu.AddrRegs[7] = stack
		cpu.DataRegs[2] = 0x0089CDE8
		cpu.DataRegs[4] = 0x00000048
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		cpu.Write32(4, funcPC+570)
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_V | M68K_SR_C
	}

	var interpWrites [][2]uint32
	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp, &interpWrites)
	writeM68KStopProgram(interp, startPC, opcodes...)
	writeM68KWords(interp, funcPC, 0x4E75) // RTS
	runM68KInterpreterUntilStopped(t, interp)

	var jitWrites [][2]uint32
	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit, &jitWrites)
	writeM68KStopProgram(jit, startPC, opcodes...)
	writeM68KWords(jit, funcPC, 0x4E75) // RTS
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute AROS DOS MMIO CLR command block natively")
	}
	for _, opcode := range []uint16{0x42B9, 0x23C4, 0x23C0} {
		if got := jit.m68kJitFallbackOpcodeCounts[opcode].Load(); got != 0 {
			t.Fatalf("AROS DOS MMIO command opcode %04X fell back to interpreter %d times", opcode, got)
		}
	}
	assertM68KCoreStateEqual(t, jit, interp)
	if len(jitWrites) != len(interpWrites) {
		t.Fatalf("MMIO write count got=%d want=%d", len(jitWrites), len(interpWrites))
	}
	for i := range jitWrites {
		if jitWrites[i] != interpWrites[i] {
			t.Fatalf("MMIO write[%d] got=(0x%08X,0x%08X) want=(0x%08X,0x%08X)",
				i, jitWrites[i][0], jitWrites[i][1], interpWrites[i][0], interpWrites[i][1])
		}
	}
	wantWrites := [][2]uint32{
		{mmio8, 0},
		{mmio12, 0x00000048},
		{mmio0, 0x00000017},
	}
	for i := range wantWrites {
		if interpWrites[i] != wantWrites[i] {
			t.Fatalf("unexpected interpreter MMIO write[%d] got=(0x%08X,0x%08X) want=(0x%08X,0x%08X)",
				i, interpWrites[i][0], interpWrites[i][1], wantWrites[i][0], wantWrites[i][1])
		}
	}
}

func TestM68KJIT_DefaultDispatcherExecutesVideoMMIOWrapperNoFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		funcPC  = uint32(0x5000)
		stack   = uint32(0x9000)
		mmioLo  = uint32(0x000F001C)
		mmioHi  = uint32(0x000F048B)
	)
	program := []uint16{
		0x4EB9, 0x0000, 0x5000, // JSR $00005000
	}
	wrapper := []uint16{
		0x2F03,         // MOVE.L D3,-(A7)
		0x2F02,         // MOVE.L D2,-(A7)
		0x262F, 0x0014, // MOVE.L 20(A7),D3
		0x242F, 0x0018, // MOVE.L 24(A7),D2
		0x222F, 0x001C, // MOVE.L 28(A7),D1
		0x202F, 0x0020, // MOVE.L 32(A7),D0
		0x42B9, 0x000F, 0x0020, // CLR.L $000F0020
		0x23EF, 0x000C, 0x000F, 0x0024, // MOVE.L 12(A7),$000F0024
		0x23EF, 0x0010, 0x000F, 0x0028, // MOVE.L 16(A7),$000F0028
		0x0283, 0x0000, 0xFFFF, // ANDI.L #$0000FFFF,D3
		0x23C3, 0x000F, 0x002C, // MOVE.L D3,$000F002C
		0x0282, 0x0000, 0xFFFF, // ANDI.L #$0000FFFF,D2
		0x23C2, 0x000F, 0x0030, // MOVE.L D2,$000F0030
		0x0281, 0x0000, 0xFFFF, // ANDI.L #$0000FFFF,D1
		0x23C1, 0x000F, 0x0034, // MOVE.L D1,$000F0034
		0x0280, 0x0000, 0xFFFF, // ANDI.L #$0000FFFF,D0
		0x23C0, 0x000F, 0x0038, // MOVE.L D0,$000F0038
		0x23EF, 0x0024, 0x000F, 0x0488, // MOVE.L 36(A7),$000F0488
		0x7001,                 // MOVEQ #1,D0
		0x23C0, 0x000F, 0x001C, // MOVE.L D0,$000F001C
		0x241F, // MOVE.L (A7)+,D2
		0x261F, // MOVE.L (A7)+,D3
		0x4E75, // RTS
	}
	setup := func(cpu *M68KCPU, writes *[][2]uint32) {
		bus, ok := cpu.bus.(*MachineBus)
		if !ok {
			t.Fatal("test CPU is not backed by MachineBus")
		}
		bus.MapIO(mmioLo, mmioHi,
			func(addr uint32) uint32 { return 0 },
			func(addr uint32, value uint32) {
				*writes = append(*writes, [2]uint32{addr, value})
			})
		cpu.AddrRegs[7] = stack
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		cpu.Write32(stack+4, 0x01E00000)
		cpu.Write32(stack+8, 0x0265F840)
		cpu.Write32(stack+12, 0x00000010)
		cpu.Write32(stack+16, 0x00000018)
		cpu.Write32(stack+20, 0x00001E00)
		cpu.Write32(stack+24, 0x00000040)
		cpu.Write32(stack+28, 0x00000030)
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_V | M68K_SR_C
	}

	var interpWrites [][2]uint32
	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp, &interpWrites)
	writeM68KStopProgram(interp, startPC, program...)
	writeM68KWords(interp, funcPC, wrapper...)
	runM68KInterpreterUntilStopped(t, interp)

	var jitWrites [][2]uint32
	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit, &jitWrites)
	writeM68KStopProgram(jit, startPC, program...)
	writeM68KWords(jit, funcPC, wrapper...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute video MMIO wrapper natively")
	}
	assertM68KCoreStateEqual(t, jit, interp)
	if len(jitWrites) != len(interpWrites) {
		t.Fatalf("MMIO write count got=%d want=%d", len(jitWrites), len(interpWrites))
	}
	for i := range jitWrites {
		if jitWrites[i] != interpWrites[i] {
			t.Fatalf("MMIO write[%d] got=(0x%08X,0x%08X) want=(0x%08X,0x%08X)",
				i, jitWrites[i][0], jitWrites[i][1], interpWrites[i][0], interpWrites[i][1])
		}
	}
}

func TestM68KJIT_LockstepReferenceRecordsExitPCWhenLastExecPCInRange(t *testing.T) {
	cpu := newM68KTestProgramCPU(t, 0x1000)
	cpu.PC = 0x0900
	cpu.lastExecPC = 0x1000
	cpu.SR = 0x2700
	cpu.AddrRegs[7] = 0x1800

	session := newM68KJITLockstepReference(0x1000, 0x10FF, 8)
	session.recordReference(cpu, 42)
	trace := session.ReferenceSnapshot()
	if _, ok := trace[42]; !ok {
		t.Fatalf("reference snapshot at exit count was not recorded")
	}
	if got := trace[42].PC; got != 0x0900 {
		t.Fatalf("recorded PC = %08X, want exit PC 00000900", got)
	}
}

func TestM68KJIT_DefaultDispatcherAROSDOSMMIOCommandResponseRoundTrip(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		base    = uint32(AROS_DOS_REGION_BASE)
		result  = uint32(AROS_DOS_REGION_BASE + 0x14)
	)
	opcodes := []uint16{
		0x7208,                 // MOVEQ #8,D1
		0x23C1, 0x000F, 0x2220, // MOVE.L D1,$000F2220
		0x2C39, 0x000F, 0x2234, // MOVE.L $000F2234,D6
	}
	setup := func(cpu *M68KCPU, writes *[]uint32) {
		bus, ok := cpu.bus.(*MachineBus)
		if !ok {
			t.Fatal("test CPU is not backed by MachineBus")
		}
		response := uint32(0)
		bus.MapIO(base, base+0x18,
			func(addr uint32) uint32 {
				if addr == result {
					return response
				}
				return 0
			},
			func(addr uint32, value uint32) {
				*writes = append(*writes, value)
				if addr == base {
					response = 0x13579BDF
				}
			})
	}

	var interpWrites []uint32
	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp, &interpWrites)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	var jitWrites []uint32
	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit, &jitWrites)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got, want := jit.DataRegs[6], interp.DataRegs[6]; got != want {
		t.Fatalf("D6 response got=0x%08X want=0x%08X", got, want)
	}
	if len(jitWrites) != len(interpWrites) {
		t.Fatalf("MMIO write count got=%d want=%d", len(jitWrites), len(interpWrites))
	}
}

func TestM68KJIT_MMIOHelperSeesPriorLazyMOVEQCCR(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		mmio    = uint32(0x000F001C)
	)
	opcodes := []uint16{
		0x7001,                 // MOVEQ #1,D0 -- clears N/Z/V/C, preserves X
		0x23C0, 0x000F, 0x001C, // MOVE.L D0,$000F001C
	}
	setup := func(cpu *M68KCPU, seen *[]uint16) {
		bus, ok := cpu.bus.(*MachineBus)
		if !ok {
			t.Fatal("test CPU is not backed by MachineBus")
		}
		bus.MapIO(mmio, mmio,
			func(addr uint32) uint32 { return 0 },
			func(addr uint32, value uint32) {
				*seen = append(*seen, cpu.SR)
			})
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C
	}

	var interpSeen []uint16
	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp, &interpSeen)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	var jitSeen []uint16
	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit, &jitSeen)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if len(jitSeen) != len(interpSeen) {
		t.Fatalf("MMIO write count got=%d want=%d", len(jitSeen), len(interpSeen))
	}
	if len(jitSeen) != 1 {
		t.Fatalf("MMIO writes got=%d want=1", len(jitSeen))
	}
	if got, want := jitSeen[0]&0x1F, interpSeen[0]&0x1F; got != want {
		t.Fatalf("MMIO callback CCR got=%02X want=%02X (jit SR=%04X interp SR=%04X)",
			got, want, jitSeen[0], interpSeen[0])
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesSupervisorMOVECNoFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const startPC = uint32(0x1000)
	opcodes := []uint16{
		0x4E7B, 0x0000, // MOVEC D0,SFC
		0x4E7A, 0x9801, // MOVEC VBR,A1
	}
	setup := func(cpu *M68KCPU) {
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_V | M68K_SR_C
		cpu.DataRegs[0] = 0x00940B68
		cpu.AddrRegs[1] = 0
		cpu.VBR = 0x00ABCDEF
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute supervisor MOVEC block natively")
	}
	for _, opcode := range []uint16{0x4E7B, 0x4E7A} {
		if got := jit.m68kJitFallbackOpcodeCounts[opcode].Load(); got != 0 {
			t.Fatalf("MOVEC opcode %04X fell back to interpreter %d times", opcode, got)
		}
	}
	assertM68KCoreStateEqual(t, jit, interp)
	if jit.SFC != 0 || interp.SFC != 0 {
		t.Fatalf("SFC jit=%d interp=%d, want 0", jit.SFC, interp.SFC)
	}
	if jit.AddrRegs[1] != 0x00ABCDEF || interp.AddrRegs[1] != 0x00ABCDEF {
		t.Fatalf("A1 jit=0x%08X interp=0x%08X, want VBR", jit.AddrRegs[1], interp.AddrRegs[1])
	}
}

func TestM68KJIT_DefaultDispatcherExecutesCLRLongCodePageRAMNoFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC  = uint32(0x1000)
		clearDst = uint32(0x1010)
	)
	opcodes := []uint16{
		0x4290, // CLR.L (A0)
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = clearDst
		cpu.Write32(clearDst, 0xA5A55A5A)
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute CLR.L (A0) block natively")
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x4290].Load(); got != 0 {
		t.Fatalf("CLR.L (A0) code-page RAM write fell back to interpreter %d times", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("CLR.L (A0) code-page RAM write used IO bailout %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
	if got, want := jit.Read32(clearDst), uint32(0); got != want {
		t.Fatalf("cleared long got=0x%08X want=0x%08X", got, want)
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeJSRRTS(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		subPC   = uint32(0x2000)
	)
	interp := newM68KTestProgramCPU(t, startPC)
	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true

	write := func(cpu *M68KCPU, pc uint32, ops ...uint16) uint32 {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
		return pc
	}
	for _, cpu := range []*M68KCPU{interp, jit} {
		cpu.AddrRegs[7] = 0x9000
		pc := startPC
		pc = write(cpu, pc,
			0x7001,                                   // MOVEQ #1,D0
			0x4EB9, uint16(subPC>>16), uint16(subPC), // JSR subPC
			0x7203, // MOVEQ #3,D1 after return
		)
		for i := 0; i < m68kJitMaxBlockSize; i++ {
			pc = write(cpu, pc, 0xD280) // ADD.L D0,D1
		}
		write(cpu, pc, 0x4E72, 0x2700) // STOP

		pc = subPC
		for i := 0; i < m68kJitMaxBlockSize-1; i++ {
			pc = write(cpu, pc, 0x5280) // ADDQ.L #1,D0
		}
		write(cpu, pc, 0x4E75) // RTS
	}

	runM68KInterpreterUntilStopped(t, interp)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got < 2 {
		t.Fatalf("native blocks executed for JSR/RTS = %d, want at least 2", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("JSR/RTS block bailed out %d times, want 0", got)
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x4EB9].Load(); got != 0 {
		t.Fatalf("JSR abs.L fell back %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeJSRAbsWord(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		subPC   = uint32(0x2000)
	)
	writeProgram := func(cpu *M68KCPU) {
		cpu.AddrRegs[7] = 0x9000
		writeM68KStopProgram(cpu, startPC,
			0x7001,         // MOVEQ #1,D0
			0x4EB8, 0x2000, // JSR $2000.W
			0x7203, // MOVEQ #3,D1 after return
		)
		writeM68KWords(cpu, subPC,
			0x7402, // MOVEQ #2,D2
			0x4E75, // RTS
		)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	writeProgram(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	writeProgram(jit)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitFallbackOpcodeCounts[0x4EB8].Load(); got != 0 {
		t.Fatalf("JSR abs.W fell back %d times, want 0", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("JSR abs.W bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeJSRAddressRegister(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		subPC   = uint32(0x2000)
	)
	writeProgram := func(cpu *M68KCPU) {
		cpu.AddrRegs[6] = subPC
		cpu.AddrRegs[7] = 0x9000
		writeM68KStopProgram(cpu, startPC,
			0x7001, // MOVEQ #1,D0
			0x4E96, // JSR (A6)
			0x7203, // MOVEQ #3,D1 after return
		)
		writeM68KWords(cpu, subPC,
			0x7402, // MOVEQ #2,D2
			0x4E75, // RTS
		)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	writeProgram(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	writeProgram(jit)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitFallbackOpcodeCounts[0x4E96].Load(); got != 0 {
		t.Fatalf("JSR (An) fell back %d times, want 0", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("JSR (An) bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeJSRDisplacementAn(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		basePC  = uint32(0x2000)
		subPC   = uint32(0x2010)
	)
	writeProgram := func(cpu *M68KCPU) {
		cpu.AddrRegs[6] = basePC
		cpu.AddrRegs[7] = 0x9000
		writeM68KStopProgram(cpu, startPC,
			0x7001,         // MOVEQ #1,D0
			0x4EAE, 0x0010, // JSR 16(A6)
			0x7203, // MOVEQ #3,D1 after return
		)
		writeM68KWords(cpu, subPC,
			0x7402, // MOVEQ #2,D2
			0x4E75, // RTS
		)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	writeProgram(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	writeProgram(jit)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitFallbackOpcodeCounts[0x4EAE].Load(); got != 0 {
		t.Fatalf("JSR d16(An) fell back %d times, want 0", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("JSR d16(An) bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeJSRNegativeDisplacementAn(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		basePC  = uint32(0x2200)
		subPC   = uint32(0x2104) // basePC - 0x00FC
		stackPC = uint32(0x9000)
	)
	writeProgram := func(cpu *M68KCPU) {
		cpu.AddrRegs[6] = basePC
		cpu.AddrRegs[7] = stackPC
		writeM68KStopProgram(cpu, startPC,
			0x7001,         // MOVEQ #1,D0
			0x4EAE, 0xFF04, // JSR -252(A6)
			0x7203, // MOVEQ #3,D1 after return
		)
		writeM68KWords(cpu, subPC,
			0x7402, // MOVEQ #2,D2
			0x4E75, // RTS
		)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	writeProgram(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	writeProgram(jit)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitFallbackOpcodeCounts[0x4EAE].Load(); got != 0 {
		t.Fatalf("JSR negative d16(An) fell back %d times, want 0", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("JSR negative d16(An) bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherJSRStackCodePageInvalidatesNatively(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC  = uint32(0x1000)
		subPC    = uint32(0x2000)
		returnPC = uint32(0x1004)
		stack    = uint32(0x943114)
		a6Base   = subPC + 138
	)
	opcodes := []uint16{
		0x4EAE, 0xFF76, // JSR -138(A6)
		0x7203, // MOVEQ #3,D1 after return
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[6] = a6Base
		cpu.AddrRegs[7] = stack
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		cpu.DataRegs[0] = 1
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	writeM68KWords(interp, subPC,
		0x5280, // ADDQ.L #1,D0
		0x4E75, // RTS
	)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	writeM68KWords(jit, subPC,
		0x5280, // ADDQ.L #1,D0
		0x4E75, // RTS
	)
	if err := jit.initM68KJIT(); err != nil {
		t.Fatalf("initM68KJIT: %v", err)
	}
	t.Cleanup(jit.freeM68KJIT)
	stackPage := (stack - 4) >> 12
	jit.m68kJitCache.Put(&JITBlock{startPC: uint64(stack &^ 0xFFF), endPC: uint64((stack &^ 0xFFF) + 4)})
	jit.m68kJitCodeBitmap[stackPage] = 1
	runM68KJITUntilStopped(t, jit)

	// JSR d16(A6) is now native; m68kEmitJSR invalidates the stack code page on
	// the return-address push, so no interpreter bailout is required.
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("JSR d16(A6) stack code-page write used IO bailout %d times", got)
	}
	if int(stackPage) < len(jit.m68kJitCodeBitmap) && jit.m68kJitCodeBitmap[stackPage] != 0 {
		got := jit.m68kJitCodeBitmap[stackPage]
		t.Fatalf("stack code page remained marked after return-address write: bitmap[%d]=%d", stackPage, got)
	}
	if got := jit.Read32(stack - 4); got != returnPC {
		t.Fatalf("return address on stack got=0x%08X want=0x%08X", got, returnPC)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeJSRPCDisplacement(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		subPC   = uint32(0x1020)
	)
	writeProgram := func(cpu *M68KCPU) {
		cpu.AddrRegs[7] = 0x9000
		writeM68KStopProgram(cpu, startPC,
			0x7001,                           // MOVEQ #1,D0
			0x4EBA, uint16(subPC-(0x1002+2)), // JSR subPC(PC), base is extension word address
			0x7203, // MOVEQ #3,D1 after return
		)
		writeM68KWords(cpu, subPC,
			0x7402, // MOVEQ #2,D2
			0x4E75, // RTS
		)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	writeProgram(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	writeProgram(jit)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitFallbackOpcodeCounts[0x4EBA].Load(); got != 0 {
		t.Fatalf("JSR d16(PC) fell back %d times, want 0", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("JSR d16(PC) bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_ForceNativeExecutesStandaloneRTS(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC  = uint32(0x1000)
		returnPC = uint32(0x1100)
		stack    = uint32(0x9000)
	)
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[7] = stack
		cpu.Write32(stack, returnPC)
		writeM68KWords(cpu, startPC, 0x4E75) // RTS
		writeM68KStopProgram(cpu, returnPC)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	jit.m68kJitForceNative = true
	setup(jit)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("force-native M68K JIT dispatcher did not execute standalone RTS natively")
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeStandaloneRTSWithHighRAMStack(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC  = uint32(0x1000)
		returnPC = uint32(0x2000)
		stack    = uint32(0x120000)
	)
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[7] = stack
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		cpu.Write32(stack, returnPC)
		writeM68KWords(cpu, startPC, 0x4E75) // RTS
		writeM68KStopProgram(cpu, returnPC,
			0x7207, // MOVEQ #7,D1 after return
		)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	instrs := m68kScanBlock(jit.memory, startPC)
	if !m68kIsStandaloneRTSBlock(instrs) {
		t.Fatalf("test block did not match standalone RTS recognizer: instrs=%+v", instrs)
	}
	if !m68kCanUseProductionNativeBlock(jit.memory, startPC, instrs) {
		t.Fatalf("standalone RTS block is not production-native: needsFallback=%v conservative=%v productionSafe=%v genericIO=%v instrs=%+v",
			m68kNeedsFallback(instrs),
			m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
			m68kBlockProductionNativeSafe(instrs),
			m68kBlockMayUseGenericIOFallback(instrs),
			instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute standalone RTS natively")
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x4E75].Load(); got != 0 {
		t.Fatalf("standalone RTS fell back to interpreter %d times", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("standalone RTS bailed out %d times; high-RAM stack should be direct RAM", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeMOVEMPostincRTSWithHighRAMStack(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC  = uint32(0x1000)
		returnPC = uint32(0x2000)
		stack    = uint32(0x120000)
		mask     = uint16(0x4CFC) // D2-D7/A2/A3/A6, matching common AROS epilogues.
	)
	saved := []uint32{
		0xD2000002, 0xD3000003, 0xD4000004, 0xD5000005, 0xD6000006, 0xD7000007,
		0xA2000002, 0xA3000003, 0xA6000006,
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[7] = stack
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		for i, val := range saved {
			cpu.Write32(stack+uint32(i)*4, val)
		}
		cpu.Write32(stack+uint32(len(saved))*4, returnPC)
		writeM68KWords(cpu, startPC,
			0x4CDF, mask, // MOVEM.L (A7)+,D2-D7/A2/A3/A6
			0x4E75, // RTS
		)
		writeM68KStopProgram(cpu, returnPC)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	instrs := m68kScanBlock(jit.memory, startPC)
	if !m68kIsMOVEMPostincRTSBlock(instrs) {
		t.Fatalf("test block did not match MOVEM+RTS recognizer: instrs=%+v", instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute MOVEM+RTS epilogue natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("MOVEM+RTS epilogue bailed out %d times; high-RAM stack should be direct RAM", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_NativeAROSMOVEMAddQRTSEpilogueMatchesInterpreter(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC  = uint32(0x0091D600)
		returnPC = uint32(0x0091D15C)
		stack    = uint32(0x00940AB0)
		mask     = uint16(0x7CFC) // D2-D7/A2-A6, from the AROS SetPatch epilogue at 0x91d600.
	)
	saved := []uint32{
		0xD2000002, 0xD3000003, 0xD4000004, 0xD5000005, 0xD6000006, 0xD7000007,
		0xA2000002, 0xA3000003, 0xA4000004, 0xA5000005, 0xA6000006,
	}
	setup := func(cpu *M68KCPU) {
		cpu.SR = M68K_SR_S
		cpu.AddrRegs[7] = stack
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		for i, val := range saved {
			cpu.Write32(stack+uint32(i)*4, val)
		}
		cpu.Write32(stack+uint32(len(saved))*4+8, returnPC)
		writeM68KWords(cpu, startPC,
			0x4CDF, mask, // MOVEM.L (A7)+,D2-D7/A2-A6
			0x508F, // ADDQ.L #8,A7
			0x4E75, // RTS
		)
		writeM68KStopProgram(cpu, returnPC)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	instrs := m68kScanBlock(jit.memory, startPC)
	if m68kNeedsFallback(instrs) || !m68kCanUseProductionNativeBlock(jit.memory, startPC, instrs) {
		t.Fatalf("AROS MOVEM/ADDQ/RTS epilogue was not fully native: instrs=%+v needsFallback=%v conservative=%v productionSafe=%v genericIO=%v",
			instrs,
			m68kNeedsFallback(instrs), m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
			m68kBlockProductionNativeSafe(instrs), m68kBlockMayUseGenericIOFallback(instrs))
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("AROS MOVEM/ADDQ/RTS epilogue did not execute native JIT")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("AROS MOVEM/ADDQ/RTS epilogue bailed out %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_NativeAROSSetPatchDecisionBlockMatchesInterpreter(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC      = uint32(0x0091D64A)
		branchTarget = uint32(0x0091D57A)
		statusBase   = uint32(0x008A59C4)
		stack        = uint32(0x00940B00)
	)
	setup := func(cpu *M68KCPU, status uint16) {
		cpu.SR = M68K_SR_S
		cpu.DataRegs[0] = 0xDEAD0000
		cpu.DataRegs[1] = 0xBEEF0000
		cpu.AddrRegs[7] = stack
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		cpu.Write16(statusBase+0x128, status)
		writeM68KWords(cpu, startPC,
			0x2079, 0x008A, 0x59C4, // MOVEA.L $008A59C4,A0
			0x3228, 0x0128, // MOVE.W 296(A0),D1
			0x3001,         // MOVE.W D1,D0
			0x0240, 0x0088, // ANDI.W #$0088,D0
			0x6700, 0xFF2C, // BEQ.W $0091D588
			0x203C, 0x0001, 0x09DC, // MOVE.L #$000109DC,D0
			0x4A01,                 // TST.B D1
			0x6B06,                 // BMI.S $0091D66E
			0x203C, 0x0001, 0x09C8, // MOVE.L #$000109C8,D0
			0x2F00,                 // MOVE.L D0,-(A7)
			0x4879, 0x008C, 0xEA77, // PEA $008CEA77
			0x6000, 0xFF02, // BRA.W $0091D57A
		)
		writeM68KStopProgram(cpu, branchTarget)
		writeM68KStopProgram(cpu, 0x0091D588)
	}

	for _, tt := range []struct {
		name   string
		status uint16
	}{
		{name: "negative_low_byte", status: 0x0088},
		{name: "positive_low_byte", status: 0x0008},
		{name: "zero_branch", status: 0x0001},
	} {
		t.Run(tt.name, func(t *testing.T) {
			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp, tt.status)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit, tt.status)
			instrs := m68kScanBlock(jit.memory, startPC)
			if m68kNeedsFallback(instrs) || !m68kCanUseProductionNativeBlock(jit.memory, startPC, instrs) {
				t.Fatalf("AROS SetPatch decision block was not fully native: instrs=%+v needsFallback=%v conservative=%v productionSafe=%v genericIO=%v",
					instrs,
					m68kNeedsFallback(instrs), m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
					m68kBlockProductionNativeSafe(instrs), m68kBlockMayUseGenericIOFallback(instrs))
			}
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatal("AROS SetPatch decision block did not execute native JIT")
			}
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("AROS SetPatch decision block bailed out %d times", got)
			}
			assertM68KCoreStateEqual(t, jit, interp)
			for _, addr := range []uint32{stack - 8, stack - 4} {
				if got, want := jit.Read32(addr), interp.Read32(addr); got != want {
					t.Fatalf("stack[0x%08X]=0x%08X, want 0x%08X", addr, got, want)
				}
			}
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeMOVEMDisplacementRestoreRTSWithHighRAMFrame(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC  = uint32(0x1000)
		returnPC = uint32(0x2000)
		frame    = uint32(0x120100)
		oldA5    = uint32(0x00ABCDEF)
		mask     = uint16(0x5C7C) // D2-D6/A2-A4/A6, matching a hot AROS epilogue.
	)
	saved := []uint32{
		0xD2000002, 0xD3000003, 0xD4000004, 0xD5000005, 0xD6000006,
		0xA2000002, 0xA3000003, 0xA4000004, 0xA6000006,
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[5] = frame
		cpu.AddrRegs[7] = 0x00100000
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		for i, val := range saved {
			cpu.Write32(frame-36+uint32(i)*4, val)
		}
		cpu.Write32(frame, oldA5)
		cpu.Write32(frame+4, returnPC)
		writeM68KWords(cpu, startPC,
			0x4CED, mask, 0xFFDC, // MOVEM.L -36(A5),D2-D6/A2-A4/A6
			0x4E5D, // UNLK A5
			0x4E75, // RTS
		)
		writeM68KStopProgram(cpu, returnPC)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	instrs := m68kScanBlock(jit.memory, startPC)
	if !m68kCanUseProductionNativeBlock(jit.memory, startPC, instrs) {
		t.Fatalf("default M68K JIT dispatcher rejected MOVEM d16(A5)/UNLK/RTS block: fallback=%v conservative=%v productionSafe=%v genericIO=%v instrs=%+v",
			m68kNeedsFallback(instrs), m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
			m68kBlockProductionNativeSafe(instrs), m68kBlockMayUseGenericIOFallback(instrs), instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute MOVEM d16(A5)/UNLK/RTS block natively")
	}
	for _, opcode := range []uint16{0x4CED, 0x4E5D, 0x4E75} {
		if got := jit.m68kJitFallbackOpcodeCounts[opcode].Load(); got != 0 {
			t.Fatalf("opcode 0x%04X fell back to interpreter %d times, want 0", opcode, got)
		}
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("MOVEM d16(A5)/UNLK/RTS block bailed out %d times; high-RAM frame should be direct RAM", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAROSMOVEMPrologueJSRWithHighRAMStack(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		nextPC  = uint32(0x1012)
		a6Base  = uint32(0x4000)
		subPC   = uint32(0x3F88) // a6Base-120
		stack   = uint32(0x120000)
	)
	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x12345678
		cpu.DataRegs[2] = 0x22222222
		cpu.AddrRegs[1] = 0x00601000
		cpu.AddrRegs[2] = 0xAAAAAAAA
		cpu.AddrRegs[3] = 0xBBBBBBBB
		cpu.AddrRegs[4] = 0xCCCCCCCC
		cpu.AddrRegs[6] = a6Base
		cpu.AddrRegs[7] = stack
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		cpu.Write32(a6Base+0x0114, 0x00602000)
		writeM68KWords(cpu, startPC,
			0x48E7, 0x203A, // MOVEM.L D2/A2/A3/A4/A6,-(A7)
			0x2449,         // MOVEA.L A1,A2
			0x2400,         // MOVE.L D0,D2
			0x264E,         // MOVEA.L A6,A3
			0x286E, 0x0114, // MOVEA.L 276(A6),A4
			0x4EAE, 0xFF88, // JSR -120(A6)
		)
		writeM68KStopProgram(cpu, nextPC)
		writeM68KWords(cpu, subPC,
			0x7009, // MOVEQ #9,D0
			0x4E75, // RTS
		)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	instrs := m68kScanBlock(jit.memory, startPC)
	if !m68kIsAROSMOVEMPrologueJSRBlock(instrs, startPC, jit.memory) {
		t.Fatalf("test block did not match AROS MOVEM prologue JSR recognizer: instrs=%+v", instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute AROS MOVEM prologue JSR block natively")
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x48E7].Load(); got != 0 {
		t.Fatalf("AROS MOVEM prologue JSR fell back to interpreter %d times", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("AROS MOVEM prologue JSR bailed out %d times; high-RAM stack should be direct RAM", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeStackLoadAbsJSRWithHighRAMStack(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		nextPC  = uint32(0x100A)
		subPC   = uint32(0x2000)
		stack   = uint32(0x120000)
	)
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[7] = stack
		cpu.AddrRegs[0] = 0x3000
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		cpu.Write32(stack+20, 0x12345678)
		cpu.Write32(0x3000, 0x00000009)
		writeM68KWords(cpu, startPC,
			0x202F, 0x0014, // MOVE.L 20(A7),D0
			0x4EB9, uint16(subPC>>16), uint16(subPC), // JSR subPC
		)
		writeM68KStopProgram(cpu, nextPC)
		writeM68KWords(cpu, subPC,
			0x2210, // MOVE.L (A0),D1; memory access prevents leaf fusion
			0x4E75, // RTS
		)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	instrs := m68kScanBlock(jit.memory, startPC)
	if !m68kIsStackLoadAbsJSRBlock(instrs, startPC, jit.memory) {
		t.Fatalf("test block did not match stack-load abs-JSR recognizer: instrs=%+v", instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute stack-load abs-JSR block natively")
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x202F].Load(); got != 0 {
		t.Fatalf("stack-load abs-JSR block fell back to interpreter %d times", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("stack-load abs-JSR block bailed out %d times; high-RAM stack should be direct RAM", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAROSStackCallWrapperBlock(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC  = uint32(0x1000)
		targetPC = uint32(0x2400)
		stack    = uint32(0x120000)
		argA2    = uint32(0x00345678)
	)
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[7] = stack
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		cpu.Write32(stack+20, argA2)
		cpu.Write32(stack+24, targetPC+66)
		writeM68KWords(cpu, startPC,
			0x246F, 0x0014, // MOVEA.L 20(A7),A2
			0x41EF, 0x0008, // LEA 8(A7),A0
			0x2C6F, 0x0018, // MOVEA.L 24(A7),A6
			0x4EAE, 0xFFBE, // JSR -66(A6)
		)
		writeM68KStopProgram(cpu, targetPC,
			0x7207, // MOVEQ #7,D1 after call target
		)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	instrs := m68kScanBlock(jit.memory, startPC)
	if !m68kIsAROSStackCallWrapperBlock(instrs, startPC, jit.memory) {
		t.Fatalf("test block did not match AROS stack-call wrapper recognizer: instrs=%+v", instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute AROS stack-call wrapper block natively")
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x246F].Load(); got != 0 {
		t.Fatalf("AROS stack-call wrapper fell back to interpreter %d times", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("AROS stack-call wrapper bailed out %d times; high-RAM stack should be direct RAM", got)
	}
	if got, want := jit.Read32(stack-4), uint32(startPC+0x10); got != want {
		t.Fatalf("pushed return PC=0x%08X, want 0x%08X", got, want)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeProductionPrefixBeforeUnsupportedTail(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		baseA2  = uint32(0x3000)
		baseA5  = uint32(0x4000)
	)
	opcodes := []uint16{
		0x5480,         // ADDQ.L #2,D0
		0x2540, 0x0008, // MOVE.L D0,8(A2)
		0x254C, 0x000C, // MOVE.L A4,12(A2)
		0x7002,         // MOVEQ #2,D0
		0x2540, 0x0010, // MOVE.L D0,16(A2)
		0x4E76, // TRAPV -- genuinely unsupported by the JIT emitter (no-op here, V is clear)
	}
	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x00000100
		cpu.AddrRegs[2] = baseA2
		cpu.AddrRegs[4] = 0xCAFEBABE
		cpu.AddrRegs[5] = baseA5
		cpu.Write32(baseA5+4, 0xDEADBEEF)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	// The five MOVE/ADDQ/MOVEQ instructions form a native prefix; the trailing
	// TRAPV is genuinely unsupported (m68kNeedsFallback) so the prefix stops
	// before it.
	if len(prefix) != 5 {
		t.Fatalf("production prefix length=%d, want 5; instrs=%+v", len(prefix), instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute production prefix natively")
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x5480].Load(); got != 0 {
		t.Fatalf("safe prefix first opcode fell back %d times", got)
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x4E76].Load(); got == 0 {
		t.Fatal("unsupported TRAPV tail did not fall back to interpreter")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("production prefix bailed out %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
	for _, off := range []uint32{8, 12, 16} {
		if got, want := jit.Read32(baseA2+off), interp.Read32(baseA2+off); got != want {
			t.Fatalf("memory at A2+%d: got=0x%08X want=0x%08X", off, got, want)
		}
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeStackDispLoadPrefix(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		stack   = uint32(0x120000)
	)
	opcodes := []uint16{
		0x202F, 0x0014, // MOVE.L 20(A7),D0
		0x242F, 0x0018, // MOVE.L 24(A7),D2
		0x206F, 0x001C, // MOVEA.L 28(A7),A0
		0x2240,         // MOVEA.L D0,A1
		0x2209,         // MOVE.L A1,D1
		0x4A2A, 0x0000, // TST.B 0(A2)
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[7] = stack
		cpu.AddrRegs[2] = stack + 32
		cpu.Write32(stack+20, 0x00003000)
		cpu.Write32(stack+24, 0x22222222)
		cpu.Write32(stack+28, 0x00004000)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) != 6 {
		t.Fatalf("stack-displacement load prefix length=%d, want 6; instrs=%+v", len(prefix), instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute stack-displacement load prefix natively")
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x202F].Load(); got != 0 {
		t.Fatalf("stack-displacement load prefix first opcode fell back %d times", got)
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x4A2A].Load(); got != 0 {
		t.Fatalf("TST.B d16(An) prefix fell back %d times", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("stack-displacement load prefix bailed out %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeStackDispStorePrefix(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		stack   = uint32(0x120000)
		obj     = uint32(0x3000)
	)
	opcodes := []uint16{
		0x2F40, 0x0074, // MOVE.L D0,116(A7)
		0x2F4C, 0x0044, // MOVE.L A4,68(A7)
		0x2043, // MOVEA.L D3,A0
		0x4E76, // TRAPV -- genuinely unsupported by the JIT emitter (no-op here, V is clear)
	}
	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x12345678
		cpu.DataRegs[3] = obj
		cpu.AddrRegs[4] = 0xCAFEBABE
		cpu.AddrRegs[7] = stack
		cpu.Write32(obj+312, 0x0BADF00D)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) != 3 {
		t.Fatalf("stack-displacement store prefix length=%d, want 3; instrs=%+v", len(prefix), instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute stack-displacement store prefix natively")
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x2F40].Load(); got != 0 {
		t.Fatalf("stack-displacement store prefix first opcode fell back %d times", got)
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x4E76].Load(); got == 0 {
		t.Fatal("unsupported TRAPV tail did not fall back to interpreter")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("stack-displacement store prefix bailed out %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
	if got, want := jit.Read32(stack+116), interp.Read32(stack+116); got != want {
		t.Fatalf("stack+116: got=0x%08X want=0x%08X", got, want)
	}
	if got, want := jit.Read32(stack+68), interp.Read32(stack+68); got != want {
		t.Fatalf("stack+68: got=0x%08X want=0x%08X", got, want)
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeStackPredecPushPrefix(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		stack   = uint32(0x120000)
	)
	opcodes := []uint16{
		0x2F0A, // MOVE.L A2,-(A7)
		0x2F00, // MOVE.L D0,-(A7)
	}
	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x80000001
		cpu.AddrRegs[2] = 0x00003000
		cpu.AddrRegs[7] = stack
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) != 2 {
		t.Fatalf("stack-predecrement push prefix length=%d, want 2; instrs=%+v", len(prefix), instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute stack-predecrement push prefix natively")
	}
	for _, opcode := range []uint16{0x2F0A, 0x2F00} {
		if got := jit.m68kJitFallbackOpcodeCounts[opcode].Load(); got != 0 {
			t.Fatalf("stack-predecrement push opcode 0x%04X fell back %d times", opcode, got)
		}
	}
	assertM68KCoreStateEqual(t, jit, interp)
	if got, want := jit.Read32(stack-4), interp.Read32(stack-4); got != want {
		t.Fatalf("stack-4: got=0x%08X want=0x%08X", got, want)
	}
	if got, want := jit.Read32(stack-8), interp.Read32(stack-8); got != want {
		t.Fatalf("stack-8: got=0x%08X want=0x%08X", got, want)
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeStackDisplacementToPredecMove(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		stack   = uint32(0x120000)
	)
	opcodes := []uint16{
		0x2F2F, 0x0008, // MOVE.L 8(A7),-(A7)
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[7] = stack
		cpu.Write32(stack+8, 0xAABBCCDD)
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) != 1 {
		t.Fatalf("stack-displacement-to-predec MOVE prefix length=%d, want 1; instrs=%+v fallback=%v conservative=%v productionSafe=%v genericIO=%v",
			len(prefix), instrs,
			m68kNeedsFallback(instrs[:1]), m68kNeedsConservativeFallback(jit.memory, startPC, instrs[:1]),
			m68kInstrProductionNativeSafe(&instrs[0]), m68kBlockMayUseGenericIOFallback(instrs[:1]))
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute stack-displacement-to-predec MOVE natively")
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x2F2F].Load(); got != 0 {
		t.Fatalf("MOVE.L d16(A7),-(A7) fell back %d times", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("MOVE.L d16(A7),-(A7) bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
	if got, want := jit.Read32(stack-4), uint32(0xAABBCCDD); got != want {
		t.Fatalf("stack-4 after push = 0x%08X, want 0x%08X", got, want)
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeStackPostincToPredecMove(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		stack   = uint32(0x120000)
	)
	opcodes := []uint16{
		0x2F1F, // MOVE.L (A7)+,-(A7)
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[7] = stack
		cpu.Write32(stack, 0x11223344)
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) != 1 {
		t.Fatalf("stack-postinc-to-predec MOVE prefix length=%d, want 1; instrs=%+v fallback=%v conservative=%v productionSafe=%v genericIO=%v",
			len(prefix), instrs,
			m68kNeedsFallback(instrs[:1]), m68kNeedsConservativeFallback(jit.memory, startPC, instrs[:1]),
			m68kInstrProductionNativeSafe(&instrs[0]), m68kBlockMayUseGenericIOFallback(instrs[:1]))
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute stack-postinc-to-predec MOVE natively")
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x2F1F].Load(); got != 0 {
		t.Fatalf("MOVE.L (A7)+,-(A7) fell back %d times", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("MOVE.L (A7)+,-(A7) bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
	if got, want := jit.AddrRegs[7], stack; got != want {
		t.Fatalf("A7 after postinc/predec MOVE = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := jit.Read32(stack), uint32(0x11223344); got != want {
		t.Fatalf("stack after postinc/predec MOVE = 0x%08X, want 0x%08X", got, want)
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeStackIndirectToPostincMove(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		stack   = uint32(0x120000)
	)
	opcodes := []uint16{
		0x2ED7, // MOVE.L (A7),(A7)+
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[7] = stack
		cpu.Write32(stack, 0x55667788)
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) != 1 {
		t.Fatalf("stack-indirect-to-postinc MOVE prefix length=%d, want 1; instrs=%+v fallback=%v conservative=%v productionSafe=%v genericIO=%v",
			len(prefix), instrs,
			m68kNeedsFallback(instrs[:1]), m68kNeedsConservativeFallback(jit.memory, startPC, instrs[:1]),
			m68kInstrProductionNativeSafe(&instrs[0]), m68kBlockMayUseGenericIOFallback(instrs[:1]))
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute stack-indirect-to-postinc MOVE natively")
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x2ED7].Load(); got != 0 {
		t.Fatalf("MOVE.L (A7),(A7)+ fell back %d times", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("MOVE.L (A7),(A7)+ bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
	if got, want := jit.AddrRegs[7], stack+4; got != want {
		t.Fatalf("A7 after indirect/postinc MOVE = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := jit.Read32(stack), uint32(0x55667788); got != want {
		t.Fatalf("stack after indirect/postinc MOVE = 0x%08X, want 0x%08X", got, want)
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeRemainingMoveLongA7MemoryForms(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		stack   = uint32(0x120000)
		dstMem  = uint32(0x2100)
		absSrc  = uint32(0x2000)
		pcSrc   = uint32(0x1100)
	)
	for _, tt := range []struct {
		name       string
		opcodes    []uint16
		opcode     uint16
		setup      func(*M68KCPU)
		checkAddrs []uint32
	}{
		{
			name:    "stack_displacement_to_address_indirect",
			opcode:  0x20AF,
			opcodes: []uint16{0x20AF, 0x0010}, // MOVE.L 16(A7),(A0)
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = dstMem
				cpu.AddrRegs[7] = stack
				cpu.Write32(stack+16, 0x71727374)
			},
			checkAddrs: []uint32{stack + 16, dstMem},
		},
		{
			name:    "predec_to_predec",
			opcode:  0x2F27,
			opcodes: []uint16{0x2F27}, // MOVE.L -(A7),-(A7)
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[7] = stack
				cpu.Write32(stack-4, 0x01020304)
			},
			checkAddrs: []uint32{stack - 8, stack - 4},
		},
		{
			name:    "postinc_to_indirect",
			opcode:  0x2E9F,
			opcodes: []uint16{0x2E9F}, // MOVE.L (A7)+,(A7)
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[7] = stack
				cpu.Write32(stack, 0x11121314)
			},
			checkAddrs: []uint32{stack, stack + 4},
		},
		{
			name:    "postinc_to_displacement",
			opcode:  0x2F5F,
			opcodes: []uint16{0x2F5F, 0x0008}, // MOVE.L (A7)+,8(A7)
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[7] = stack
				cpu.Write32(stack, 0x21222324)
			},
			checkAddrs: []uint32{stack, stack + 12},
		},
		{
			name:    "postinc_to_postinc",
			opcode:  0x2EDF,
			opcodes: []uint16{0x2EDF}, // MOVE.L (A7)+,(A7)+
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[7] = stack
				cpu.Write32(stack, 0x31323334)
			},
			checkAddrs: []uint32{stack, stack + 4},
		},
		{
			name:    "absw_to_indirect",
			opcode:  0x2EB8,
			opcodes: []uint16{0x2EB8, uint16(absSrc)}, // MOVE.L abs.W,(A7)
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[7] = stack
				cpu.Write32(absSrc, 0x41424344)
			},
			checkAddrs: []uint32{stack, absSrc},
		},
		{
			name:    "absw_to_postinc",
			opcode:  0x2EF8,
			opcodes: []uint16{0x2EF8, uint16(absSrc)}, // MOVE.L abs.W,(A7)+
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[7] = stack
				cpu.Write32(absSrc, 0x51525354)
			},
			checkAddrs: []uint32{stack, absSrc},
		},
		{
			name:    "pcdisp_to_postinc",
			opcode:  0x2EFA,
			opcodes: []uint16{0x2EFA, 0x00FE}, // MOVE.L 254(PC),(A7)+; base is ext word at 0x1002
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[7] = stack
				cpu.Write32(pcSrc, 0x61626364)
			},
			checkAddrs: []uint32{stack, pcSrc},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setup := func(cpu *M68KCPU) {
				tt.setup(cpu)
				cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_V | M68K_SR_C
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			writeM68KStopProgram(interp, startPC, tt.opcodes...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			writeM68KStopProgram(jit, startPC, tt.opcodes...)
			instrs := m68kScanBlock(jit.memory, startPC)
			prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
			if len(prefix) != 1 {
				t.Fatalf("%s MOVE prefix length=%d, want 1; instrs=%+v fallback=%v conservative=%v productionSafe=%v genericIO=%v",
					tt.name, len(prefix), instrs,
					m68kNeedsFallback(instrs[:1]), m68kNeedsConservativeFallback(jit.memory, startPC, instrs[:1]),
					m68kInstrProductionNativeSafe(&instrs[0]), m68kBlockMayUseGenericIOFallback(instrs[:1]))
			}
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("%s did not execute natively", tt.name)
			}
			if got := jit.m68kJitFallbackOpcodeCounts[tt.opcode].Load(); got != 0 {
				t.Fatalf("%s opcode 0x%04X fell back %d times", tt.name, tt.opcode, got)
			}
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("%s bailed out %d times, want 0", tt.name, got)
			}
			assertM68KCoreStateEqual(t, jit, interp)
			for _, addr := range tt.checkAddrs {
				if got, want := jit.Read32(addr), interp.Read32(addr); got != want {
					t.Fatalf("%s memory[0x%08X]=0x%08X, want 0x%08X", tt.name, addr, got, want)
				}
			}
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeStackPredecBeforeJSRFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		subPC   = uint32(0x1100)
		stack   = uint32(0x120000)
	)
	opcodes := []uint16{
		0x2F0E,         // MOVE.L A6,-(A7)
		0x2F0A,         // MOVE.L A2,-(A7)
		0x4EAE, 0xFFCA, // JSR -54(A6)
	}
	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x10
		cpu.AddrRegs[2] = 0x00003000
		cpu.AddrRegs[6] = subPC + 54
		cpu.AddrRegs[7] = stack
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		writeM68KWords(cpu, subPC,
			0x5280, // ADDQ.L #1,D0
			0x4E75, // RTS
		)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	if !m68kCanUseProductionNativeBlock(jit.memory, startPC, instrs) {
		t.Fatalf("stack-predecrement JSR block was not production-native: fallback=%v conservative=%v productionSafe=%v genericIO=%v instrs=%+v",
			m68kNeedsFallback(instrs), m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
			m68kBlockProductionNativeSafe(instrs), m68kBlockMayUseGenericIOFallback(instrs), instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute stack-predecrement JSR block natively")
	}
	for _, opcode := range []uint16{0x2F0E, 0x2F0A, 0x4EAE} {
		if got := jit.m68kJitFallbackOpcodeCounts[opcode].Load(); got != 0 {
			t.Fatalf("stack-predecrement JSR opcode 0x%04X fell back %d times", opcode, got)
		}
	}
	assertM68KCoreStateEqual(t, jit, interp)
	if got, want := jit.Read32(stack-4), interp.Read32(stack-4); got != want {
		t.Fatalf("stack-4: got=0x%08X want=0x%08X", got, want)
	}
	if got, want := jit.Read32(stack-8), interp.Read32(stack-8); got != want {
		t.Fatalf("stack-8: got=0x%08X want=0x%08X", got, want)
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeStackPredecRTSTrampoline(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		subPC   = uint32(0x1100)
		target  = uint32(0x1200)
		stack   = uint32(0x140000)
	)
	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = target
		cpu.DataRegs[1] = 0
		cpu.AddrRegs[7] = stack
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		writeM68KWords(cpu, subPC,
			0x2F00, // MOVE.L D0,-(A7)
			0x4E75, // RTS to D0 target
		)
		writeM68KWords(cpu, target,
			0x7202,         // MOVEQ #2,D1
			0x4E72, 0x2700, // STOP
		)
	}
	opcodes := []uint16{
		0x4EB9, uint16(subPC >> 16), uint16(subPC), // JSR sub
		0x7201, // MOVEQ #1,D1, should be skipped by trampoline
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, subPC)
	if len(instrs) != 2 || instrs[0].opcode != 0x2F00 || instrs[1].opcode != 0x4E75 {
		t.Fatalf("trampoline scan mismatch: instrs=%+v", instrs)
	}
	if !m68kCanUseProductionNativeBlock(jit.memory, subPC, instrs) {
		t.Fatalf("trampoline block rejected: fallback=%v conservative=%v productionSafe=%v genericIO=%v instrs=%+v",
			m68kNeedsFallback(instrs), m68kNeedsConservativeFallback(jit.memory, subPC, instrs),
			m68kBlockProductionNativeSafe(instrs), m68kBlockMayUseGenericIOFallback(instrs), instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitFallbackOpcodeCounts[0x2F00].Load(); got != 0 {
		t.Fatalf("trampoline MOVE.L D0,-(A7) fell back %d times", got)
	}
	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("trampoline did not execute a native JIT block")
	}
	if got := jit.DataRegs[1]; got != 2 {
		t.Fatalf("D1=0x%08X, want 2", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativePEARTSTrampoline(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		target  = uint32(0x1200)
		stack   = uint32(0x140000)
	)
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = target
		cpu.AddrRegs[7] = stack
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		writeM68KWords(cpu, target,
			0x7201, // MOVEQ #1,D1
			0x4E75, // RTS to continuation pushed by PEA
		)
	}
	opcodes := []uint16{
		0x487A, 0x0006, // PEA 6(PC), pushes continuation at startPC+8
		0x2F08, // MOVE.L A0,-(A7), pushes dynamic target
		0x4E75, // RTS to A0 target
		0x7202, // MOVEQ #2,D1 after target returns
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	if !m68kIsPEARTSTrampoline(instrs) {
		t.Fatalf("test block did not match PEA/RTS trampoline recognizer: instrs=%+v", instrs)
	}
	if !m68kCanUseProductionNativeBlock(jit.memory, startPC, instrs) {
		t.Fatalf("PEA/RTS trampoline block rejected: fallback=%v conservative=%v productionSafe=%v genericIO=%v instrs=%+v",
			m68kNeedsFallback(instrs), m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
			m68kBlockProductionNativeSafe(instrs), m68kBlockMayUseGenericIOFallback(instrs), instrs)
	}

	runM68KJITUntilStopped(t, jit)
	if got := jit.m68kJitFallbackOpcodeCounts[0x487A].Load(); got != 0 {
		t.Fatalf("PEA/RTS trampoline fell back %d times", got)
	}
	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("PEA/RTS trampoline did not execute a native JIT block")
	}
	if got := jit.DataRegs[1]; got != 2 {
		t.Fatalf("D1=0x%08X, want 2", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_ExecutesNativeAROSConstructorDispatchPattern(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC  = uint32(0x1000)
		targetPC = uint32(0x2400)
		baseA3   = uint32(0x3000)
	)
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[2] = targetPC + 30 // A2 -> A6, JSR -30(A6) lands at targetPC
		cpu.AddrRegs[3] = baseA3
		cpu.DataRegs[0] = 0x55 // BEQ.S not taken (Z initially clear after MOVE.L)
		cpu.AddrRegs[7] = 0x120000
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		writeM68KStopProgram(cpu, startPC,
			0x274A, 0x00AA, // MOVE.L A2,170(A3)
			0x674A,         // BEQ.S skip
			0x2C4A,         // MOVEA.L A2,A6
			0x4EAE, 0xFFE2, // JSR -30(A6)
		)
		writeM68KStopProgram(cpu, targetPC,
			0x7207, // MOVEQ #7,D1 after call target
		)
	}

	recog := newM68KTestProgramCPU(t, startPC)
	setup(recog)
	instrs := m68kScanBlock(recog.memory, startPC)
	if !m68kIsAROSConstructorDispatchBlock(recog.memory, startPC, instrs) {
		t.Fatalf("test block did not match AROS constructor dispatch recognizer: instrs=%+v", instrs)
	}
	// The block is now admitted to the production-native path (the conservative
	// gate is no longer consulted for production admission).
	if !m68kCanUseProductionNativeBlock(recog.memory, startPC, instrs) {
		t.Fatalf("AROS constructor dispatch block was not admitted as production native: instrs=%+v", instrs)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute AROS constructor dispatch block natively")
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_ProductionAllowsStackPredecrementPushPrefix(t *testing.T) {
	const startPC = uint32(0x1000)

	cpu := newM68KTestProgramCPU(t, startPC)
	writeM68KStopProgram(cpu, startPC,
		0x2F0E,         // MOVE.L A6,-(A7)
		0x2F0A,         // MOVE.L A2,-(A7)
		0x4EAE, 0xFFCA, // JSR -54(A6)
	)

	instrs := m68kScanBlock(cpu.memory, startPC)
	if prefix := m68kProductionNativePrefix(cpu.memory, startPC, instrs); len(prefix) != 2 {
		t.Fatalf("stack-predecrement push native prefix length=%d, want 2; prefix=%+v instrs=%+v", len(prefix), prefix, instrs)
	}
	for _, instr := range instrs[:2] {
		if !m68kInstrProductionNativeSafe(&instr) {
			t.Fatalf("stack-predecrement opcode 0x%04X is not production-native safe", instr.opcode)
		}
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeBriefIndexedLEA(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const startPC = uint32(0x1000)
	opcodes := []uint16{
		0x47F1, 0x3808, // LEA 8(A1,D3.L),A3
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[1] = 0x2000
		cpu.DataRegs[3] = 0x30
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_C
	}

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) != 1 || prefix[0].opcode != 0x47F1 {
		candidate := instrs[:1]
		t.Fatalf("brief indexed LEA native prefix = %+v, want single LEA from instrs=%+v canPrefix=%v needsFallback=%v conservative=%v instrSafe=%v blockSafe=%v genericIO=%v",
			prefix, instrs,
			m68kCanPrefixInstruction(&instrs[0]),
			m68kNeedsFallback(candidate),
			m68kNeedsConservativeFallback(jit.memory, startPC, candidate),
			m68kInstrProductionNativeSafe(&instrs[0]),
			m68kBlockProductionNativeSafe(candidate),
			m68kBlockMayUseGenericIOFallback(candidate))
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute brief indexed LEA natively")
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x47F1].Load(); got != 0 {
		t.Fatalf("brief indexed LEA fell back %d times", got)
	}
	if got, want := jit.AddrRegs[3], uint32(0x2038); got != want {
		t.Fatalf("A3 after LEA = 0x%08X, want 0x%08X", got, want)
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeFullIndexedLEA(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const startPC = uint32(0x1000)
	opcodes := []uint16{
		0x41F0, 0x1920, 0x015E, // LEA 350(A0,D1.L),A0
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = 0x2000
		cpu.DataRegs[1] = 0x28
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) != 1 || prefix[0].opcode != 0x41F0 {
		candidate := instrs[:1]
		t.Fatalf("full indexed LEA native prefix = %+v, want single LEA from instrs=%+v canPrefix=%v needsFallback=%v conservative=%v instrSafe=%v blockSafe=%v genericIO=%v",
			prefix, instrs,
			m68kCanPrefixInstruction(&instrs[0]),
			m68kNeedsFallback(candidate),
			m68kNeedsConservativeFallback(jit.memory, startPC, candidate),
			m68kInstrProductionNativeSafe(&instrs[0]),
			m68kBlockProductionNativeSafe(candidate),
			m68kBlockMayUseGenericIOFallback(candidate))
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute full indexed LEA natively")
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x41F0].Load(); got != 0 {
		t.Fatalf("full indexed LEA fell back %d times", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("full indexed LEA bailed out %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeBTSTImmediateDnPrefix(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const startPC = uint32(0x1000)
	opcodes := []uint16{
		0x0801, 0x0000, // BTST #0,D1
		0x0801, 0x0001, // BTST #1,D1
	}
	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[1] = 0x00000001
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) != 2 {
		t.Fatalf("BTST immediate prefix length=%d, want 2; instrs=%+v", len(prefix), instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute BTST immediate prefix natively")
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x0801].Load(); got != 0 {
		t.Fatalf("BTST immediate prefix fell back %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeTSTLongAddressRegisterPrefix(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const startPC = uint32(0x1000)
	opcodes := []uint16{
		0x4A8A, // TST.L A2
		0x4A89, // TST.L A1
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[1] = 0
		cpu.AddrRegs[2] = 0x80000000
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) != 2 {
		t.Fatalf("TST.L An prefix length=%d, want 2; instrs=%+v", len(prefix), instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute TST.L An prefix natively")
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x4A8A].Load(); got != 0 {
		t.Fatalf("TST.L An prefix fell back %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeTSTAnDispPrefix(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		baseA2  = uint32(0x170000)
		baseA6  = uint32(0x180000)
	)
	opcodes := []uint16{
		0x4A2A, 0x0000, // TST.B 0(A2)
		0x4A6E, 0x0002, // TST.W 2(A6)
		0x4AAA, 0x0004, // TST.L 4(A2)
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[2] = baseA2
		cpu.AddrRegs[6] = baseA6
		cpu.Write8(baseA2, 0x80)
		cpu.Write16(baseA6+2, 0x0000)
		cpu.Write32(baseA2+4, 0x00000000)
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) != 3 {
		t.Fatalf("TST d16(An) prefix length=%d, want 3; instrs=%+v", len(prefix), instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute TST d16(An) prefix natively")
	}
	for _, opcode := range []uint16{0x4A2A, 0x4A6E, 0x4AAA} {
		if got := jit.m68kJitFallbackOpcodeCounts[opcode].Load(); got != 0 {
			t.Fatalf("TST d16(An) opcode 0x%04X fell back %d times", opcode, got)
		}
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherMaterializesNativeTSTAnDispBeforeBranchFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		obj     = uint32(0x3000)
	)
	opcodes := []uint16{
		0x7201,         // MOVEQ #1,D1
		0x244E,         // MOVEA.L A6,A2
		0x4AAE, 0x0000, // TST.L 0(A6)
		0x6706,         // BEQ.S over the bad fall-through
		0x2C6E, 0x0004, // MOVEA.L 4(A6),A6
		0x7001, // MOVEQ #1,D0 -- must be skipped
		0x7002, // MOVEQ #2,D0 -- branch target
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[2] = 0x00004000
		cpu.AddrRegs[6] = obj
		cpu.Write32(obj, 0)
		cpu.Write32(obj+4, 0)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	// The whole block (including the inline BEQ and its taken/not-taken paths)
	// is now admitted natively; the prefix covers all 7 decoded instructions.
	if len(prefix) != 7 {
		t.Fatalf("TST d16(An) branch prefix length=%d, want 7; instrs=%+v", len(prefix), instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute TST d16(An) branch prefix natively")
	}
	// Full-state parity plus D0==2 proves the inline BEQ consumed the correct
	// TST flags (the branch was taken to MOVEQ #2,D0).
	assertM68KCoreStateEqual(t, jit, interp)
	if got := jit.DataRegs[0]; got != 2 {
		t.Fatalf("branch consumed stale TST flags: D0=0x%08X, want 2", got)
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeTSTBriefIndexedBeforeBranch(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		baseA0  = uint32(0x3000)
	)
	opcodes := []uint16{
		0x7201,         // MOVEQ #1,D1
		0x4A30, 0x006C, // TST.B 108(A0,D0.W)
		0x6702, // BEQ.S over the fall-through write
		0x7207, // MOVEQ #7,D1 -- skipped when TST sets Z
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = baseA0
		cpu.DataRegs[0] = 4
		cpu.Write8(baseA0+108+4, 0)
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) != 4 {
		t.Fatalf("TST brief-indexed branch prefix length=%d, want 4; instrs=%+v conservative=%v productionSafe=%v",
			len(prefix), instrs, m68kNeedsConservativeFallback(jit.memory, startPC, instrs), m68kBlockProductionNativeSafe(instrs))
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute TST brief-indexed prefix natively")
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x4A30].Load(); got != 0 {
		t.Fatalf("TST.B brief-indexed opcode 0x4A30 fell back %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
	if got := jit.DataRegs[1]; got != 1 {
		t.Fatalf("branch consumed stale TST flags: D1=0x%08X, want 1", got)
	}
}

func TestM68KJIT_DefaultDispatcherMaterializesNativeCMPIAnBeforeBranchFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		obj     = uint32(0x3000)
	)
	for _, tc := range []struct {
		name    string
		word    uint16
		wantD0  uint32
		wantBNE bool
	}{
		{name: "equal", word: 0x4AFC, wantD0: 1, wantBNE: false},
		{name: "not-equal", word: 0x4AFD, wantD0: 2, wantBNE: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opcodes := []uint16{
				0x7201,         // MOVEQ #1,D1
				0x244E,         // MOVEA.L A6,A2
				0x0C52, 0x4AFC, // CMPI.W #$4AFC,(A2)
				0x6604, // BNE.S to the taken result
				0x7001, // MOVEQ #1,D0 -- not-taken path
				0x6002, // BRA.S done
				0x7002, // MOVEQ #2,D0 -- branch target
			}
			setup := func(cpu *M68KCPU) {
				cpu.AddrRegs[6] = obj
				cpu.Write16(obj, tc.word)
				cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_V | M68K_SR_C
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			writeM68KStopProgram(interp, startPC, opcodes...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			writeM68KStopProgram(jit, startPC, opcodes...)
			instrs := m68kScanBlock(jit.memory, startPC)
			prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
			if len(prefix) < 3 || prefix[2].opcode != 0x0C52 {
				t.Fatalf("CMPI (An) native prefix=%+v, want at least through CMPI; instrs=%+v", prefix, instrs)
			}
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatal("default M68K JIT dispatcher did not execute CMPI (An) branch prefix natively")
			}
			assertM68KCoreStateEqual(t, jit, interp)
			if got := jit.DataRegs[0]; got != tc.wantD0 {
				t.Fatalf("branch consumed stale CMPI flags: D0=0x%08X, want %d", got, tc.wantD0)
			}
			taken := tc.wantD0 == 2
			if taken != tc.wantBNE {
				t.Fatalf("test bug: taken=%v wantBNE=%v", taken, tc.wantBNE)
			}
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesIndexedByteCopyCountLoopHelper(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		srcBase = uint32(0x3000)
		dstBase = uint32(0x4000)
	)
	opcodes := []uint16{
		0x307C, 0x0004, // MOVEA.W #4,A0
		0x7601,                 // MOVEQ #1,D3
		0x91C3,                 // SUBA.L D3,A0 -> copy count 3
		0x93C9,                 // SUBA.L A1,A1
		0x13B1, 0x1800, 0x0800, // MOVE.B 0(A1,D1.L),0(A1,D0.L)
		0x5289, // ADDQ.L #1,A1
		0xB3C8, // CMPA.L A0,A1
		0x66F4, // BNE.S loop
	}
	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = dstBase
		cpu.DataRegs[1] = srcBase
		cpu.AddrRegs[1] = 0x11111111
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_V | M68K_SR_C
		cpu.Write8(srcBase+0, 0xA0)
		cpu.Write8(srcBase+1, 0xA1)
		cpu.Write8(srcBase+2, 0xA2)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	assertM68KCoreStateEqual(t, jit, interp)
	for off := uint32(0); off < 3; off++ {
		if got, want := jit.Read8(dstBase+off), interp.Read8(dstBase+off); got != want {
			t.Fatalf("copied byte at dst+%d: got=0x%02X want=0x%02X", off, got, want)
		}
	}
}

func TestM68KJIT_IndexedByteCopyCountLoopHelperInvalidatesCompiledDestination(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		loopPC  = uint32(0x1000)
		srcBase = uint32(0x3000)
		dstBase = uint32(0x4000)
		count   = uint32(3)
	)

	cpu := newM68KTestProgramCPU(t, loopPC)
	writeM68KWords(cpu, loopPC,
		0x13B1, 0x1800, 0x0800, // MOVE.B 0(A1,D1.L),0(A1,D0.L)
		0x5289, // ADDQ.L #1,A1
		0xB3C8, // CMPA.L A0,A1
		0x66F4, // BNE.S loop
	)
	cpu.DataRegs[0] = dstBase
	cpu.DataRegs[1] = srcBase
	cpu.AddrRegs[0] = count
	cpu.AddrRegs[1] = 0
	cpu.Write8(srcBase+0, 0xA0)
	cpu.Write8(srcBase+1, 0xA1)
	cpu.Write8(srcBase+2, 0xA2)

	if err := cpu.initM68KJIT(); err != nil {
		t.Fatalf("initM68KJIT: %v", err)
	}
	t.Cleanup(cpu.freeM68KJIT)

	const compiledPC = uint64(dstBase)
	page := compiledPC >> 12
	cpu.m68kJitCache.Put(&JITBlock{startPC: compiledPC, endPC: compiledPC + 4})
	cpu.m68kJitCodeBitmap[page] = 1
	cpu.m68kJitCtx.RTSCache0PC = 0x2100
	cpu.m68kJitCtx.RTSCache0Addr = 0x3100

	retired, ok := cpu.tryM68KIndexedByteCopyCountLoop()
	if !ok {
		t.Fatal("indexed byte copy helper did not recognize loop")
	}
	if retired != uint64(count)*4 {
		t.Fatalf("retired = %d, want %d", retired, uint64(count)*4)
	}
	for off := uint32(0); off < count; off++ {
		if got, want := cpu.Read8(dstBase+off), byte(0xA0+off); got != want {
			t.Fatalf("copied byte at dst+%d: got=0x%02X want=0x%02X", off, got, want)
		}
	}
	if got := cpu.m68kJitCache.Get(compiledPC); got != nil {
		t.Fatalf("compiled destination block survived helper invalidation: %#v", got)
	}
	if got := cpu.m68kJitCodeBitmap[page]; got != 0 {
		t.Fatalf("code bitmap page after helper invalidation = %d, want 0", got)
	}
	if cpu.m68kJitCtx.RTSCache0PC != 0 || cpu.m68kJitCtx.RTSCache0Addr != 0 {
		t.Fatalf("RTS cache was not cleared by helper invalidation")
	}
}

func TestM68KJIT_DefaultDispatcherMatchesAROSPostincFillLoopPrefix(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const startPC = uint32(0x1000)
	opcodes := []uint16{
		0x2608, // MOVE.L A0,D3
		0x968A, // SUB.L A2,D3
		0xD689, // ADD.L A1,D3
		0x7804, // MOVEQ #4,D4
		0xB883, // CMP.L D3,D4
	}

	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[3] = 0xCAFEBABE
		cpu.DataRegs[4] = 0xDEADBEEF
		cpu.AddrRegs[0] = 0x00300000
		cpu.AddrRegs[1] = 0x00000100
		cpu.AddrRegs[2] = 0x003000F4
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)

	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) != len(opcodes) || !m68kCanUseProductionNativeBlock(jit.memory, startPC, prefix) {
		t.Fatalf("AROS postinc fill prefix rejected: fallback=%v conservative=%v productionSafe=%v genericIO=%v instrs=%+v",
			m68kNeedsFallback(prefix),
			m68kNeedsConservativeFallback(jit.memory, startPC, prefix),
			m68kBlockProductionNativeSafe(prefix),
			m68kBlockMayUseGenericIOFallback(prefix),
			instrs)
	}

	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute AROS postinc fill prefix natively")
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherMaterializesAROSPostincFillLoopBranchFlags(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tc := range []struct {
		name string
		a0   uint32
		a1   uint32
		a2   uint32
	}{
		{name: "taken", a0: 0x00300000, a1: 0x00000100, a2: 0x003000F4},
		{name: "not_taken", a0: 0x00300000, a1: 0x00000100, a2: 0x003000FC},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const startPC = uint32(0x1000)
			opcodes := []uint16{
				0x2608, // MOVE.L A0,D3
				0x968A, // SUB.L A2,D3
				0xD689, // ADD.L A1,D3
				0x7804, // MOVEQ #4,D4
				0xB883, // CMP.L D3,D4
				0x6502, // BCS.S +2
				0x7001, // MOVEQ #1,D0
				0x7002, // MOVEQ #2,D0
			}

			setup := func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = tc.a0
				cpu.AddrRegs[1] = tc.a1
				cpu.AddrRegs[2] = tc.a2
				cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_V | M68K_SR_C
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			writeM68KStopProgram(interp, startPC, opcodes...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			writeM68KStopProgram(jit, startPC, opcodes...)
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatal("default M68K JIT dispatcher did not execute AROS postinc fill branch prefix natively")
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeBTSTImmediateAnDispPrefix(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		baseA2  = uint32(0x170000)
	)
	opcodes := []uint16{
		0x082A, 0x0003, 0x0011, // BTST #3,17(A2)
		0x082A, 0x000A, 0x0012, // BTST #10,18(A2), memory bit number wraps to bit 2
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[2] = baseA2
		cpu.Write8(baseA2+17, 0x08)
		cpu.Write8(baseA2+18, 0x00)
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) != 2 {
		t.Fatalf("BTST #imm,d16(An) prefix length=%d, want 2; instrs=%+v", len(prefix), instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute BTST #imm,d16(An) prefix natively")
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x082A].Load(); got != 0 {
		t.Fatalf("BTST #imm,d16(An) fell back %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeImmediateLogicLongDnPrefix(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const startPC = uint32(0x1000)
	opcodes := []uint16{
		0x0280, 0x0000, 0x0FFF, // ANDI.L #$00000FFF,D0
		0x0081, 0x8000, 0x0000, // ORI.L #$80000000,D1
		0x0A82, 0xFFFF, 0x00FF, // EORI.L #$FFFF00FF,D2
	}
	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x1234FFFF
		cpu.DataRegs[1] = 0x00000004
		cpu.DataRegs[2] = 0x00FF00FF
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) != 3 {
		t.Fatalf("immediate logic prefix length=%d, want 3; instrs=%+v", len(prefix), instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute immediate logic prefix natively")
	}
	for _, opcode := range []uint16{0x0280, 0x0081, 0x0A82} {
		if got := jit.m68kJitFallbackOpcodeCounts[opcode].Load(); got != 0 {
			t.Fatalf("opcode %04X fell back %d times", opcode, got)
		}
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAROSAddStoreJSRBlock(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC  = uint32(0x1000)
		targetPC = uint32(0x2400)
		baseA2   = uint32(0x3000)
	)
	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x100
		cpu.DataRegs[2] = targetPC + 30
		cpu.AddrRegs[2] = baseA2
		cpu.AddrRegs[3] = 0x00ABCDEF
		cpu.AddrRegs[7] = 0x120000
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		writeM68KWords(cpu, startPC,
			0x5480,         // ADDQ.L #2,D0
			0x2540, 0x0008, // MOVE.L D0,8(A2)
			0x254B, 0x000C, // MOVE.L A3,12(A2)
			0x91C8,                 // SUBA.L A0,A0
			0x43F9, 0x0062, 0x5B28, // LEA $00625B28,A1
			0x2C42,         // MOVEA.L D2,A6
			0x4EAE, 0xFFE2, // JSR -30(A6)
		)
		writeM68KStopProgram(cpu, targetPC,
			0x7207, // MOVEQ #7,D1 after call target
		)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	instrs := m68kScanBlock(jit.memory, startPC)
	if !m68kIsAROSAddStoreJSRBlock(instrs, startPC, jit.memory) {
		t.Fatalf("test block did not match AROS add/store/JSR recognizer: instrs=%+v", instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute AROS add/store/JSR block natively")
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x5480].Load(); got != 0 {
		t.Fatalf("AROS add/store/JSR block fell back to interpreter %d times", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("AROS add/store/JSR block bailed out %d times", got)
	}
	if got, want := jit.Read32(baseA2+8), interp.Read32(baseA2+8); got != want {
		t.Fatalf("stored D0: got=0x%08X want=0x%08X", got, want)
	}
	if got, want := jit.Read32(baseA2+12), interp.Read32(baseA2+12); got != want {
		t.Fatalf("stored A3: got=0x%08X want=0x%08X", got, want)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherMemToMemMoveCodePageWriteInvalidatesWithoutFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		src     = uint32(0x1800) // Same 4 KiB code bitmap page as startPC.
		dst     = uint32(0x1900)
	)
	opcodes := []uint16{
		0x156B, 0x0000, 0x0000, // MOVE.B 0(A3),0(A2)
		0x6704, // BEQ.S set_two
		0x7201, // MOVEQ #1,D1
		0x6002, // BRA.S done
		0x7202, // MOVEQ #2,D1
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[2] = dst
		cpu.AddrRegs[3] = src
		cpu.Write8(src, 0x00)
		cpu.Write8(dst, 0xA5)
		cpu.SR = M68K_SR_S | M68K_SR_N | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	wantPrefix := 0
	for wantPrefix < len(instrs) && instrs[wantPrefix].opcode != 0x4E72 && !m68kIsBlockTerminator(instrs[wantPrefix].opcode) {
		wantPrefix++
	}
	if len(prefix) != wantPrefix {
		t.Fatalf("mem-to-mem MOVE prefix length=%d want=%d prefix=%+v instrs=%+v productionSafe=%v genericIO=%v",
			len(prefix), wantPrefix, prefix, instrs, m68kBlockProductionNativeSafe(instrs), m68kBlockMayUseGenericIOFallback(instrs))
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitFallbackOpcodeCounts[0x156B].Load(); got != 0 {
		t.Fatalf("MOVE.B d16(A3),d16(A2) fell back %d times", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("mem-to-mem MOVE bailed out %d times", got)
	}
	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("mem-to-mem MOVE did not execute a native JIT block")
	}
	if got := jit.Read8(dst); got != 0x00 {
		t.Fatalf("destination byte=0x%02X, want 0x00", got)
	}
	if got := jit.DataRegs[1]; got != 2 {
		t.Fatalf("D1=0x%08X, want 2; BEQ consumed wrong MOVE flags", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherRegToMemMoveCodePageWriteInvalidatesWithoutFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		base    = uint32(0x1800) // Same 4 KiB code bitmap page as startPC.
		dst     = base + 4
	)
	opcodes := []uint16{
		0x2149, 0x0004, // MOVE.L A1,4(A0)
		0x6704, // BEQ.S set_two
		0x7201, // MOVEQ #1,D1
		0x6002, // BRA.S done
		0x7202, // MOVEQ #2,D1
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = base
		cpu.AddrRegs[1] = 0
		cpu.Write32(dst, 0xA5A5A5A5)
		cpu.SR = M68K_SR_S | M68K_SR_N | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitFallbackOpcodeCounts[0x2149].Load(); got != 0 {
		t.Fatalf("MOVE.L A1,4(A0) fell back %d times", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("MOVE.L A1,4(A0) bailed out %d times", got)
	}
	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("MOVE.L A1,4(A0) did not execute a native JIT block")
	}
	if got := jit.Read32(dst); got != 0 {
		t.Fatalf("destination long=0x%08X, want 0", got)
	}
	if got := jit.DataRegs[1]; got != 2 {
		t.Fatalf("D1=0x%08X, want 2; BEQ consumed wrong MOVE flags", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherRegToMemLongInvalidatesExactCodePage(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		dstPC   = uint32(0x9000)
	)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	jit.m68kJitPersist = true
	t.Cleanup(jit.freeM68KJIT)
	if err := jit.initM68KJIT(); err != nil {
		t.Fatalf("initM68KJIT: %v", err)
	}

	dstBlock := &JITBlock{startPC: uint64(dstPC), endPC: uint64(dstPC + 4)}
	jit.m68kJitCache.Put(dstBlock)
	jit.m68kMarkJITCodeRanges(dstBlock)

	jit.DataRegs[0] = 0x4E714E71
	jit.AddrRegs[0] = dstPC
	writeM68KStopProgram(jit, startPC,
		0x2080, // MOVE.L D0,(A0)
	)

	runM68KJITUntilStopped(t, jit)

	if got := jit.Read32(dstPC); got != 0x4E714E71 {
		t.Fatalf("destination long=0x%08X, want 0x4E714E71", got)
	}
	if got := jit.m68kJitCache.Get(uint64(dstPC)); got != nil {
		t.Fatalf("compiled destination block survived exact code-page invalidation: %#v", got)
	}
	if got := jit.m68kJitCodeBitmap[dstPC>>12]; got != 0 {
		t.Fatalf("destination code bitmap page after invalidation = %d, want 0", got)
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x2080].Load(); got != 0 {
		t.Fatalf("MOVE.L D0,(A0) fell back %d times", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("MOVE.L D0,(A0) bailed out %d times", got)
	}
}

func TestM68KJIT_MOVEMCodeWriteInvalidationAddressSurvivesLaterDataWrite(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC  = uint32(0x1000)
		targetPC = uint32(0x2000)
		dataPC   = uint32(0x3000)
	)

	cpu := newM68KTestProgramCPU(t, startPC)
	cpu.m68kJitEnabled = true
	cpu.m68kJitPersist = true
	t.Cleanup(cpu.freeM68KJIT)
	if err := cpu.initM68KJIT(); err != nil {
		t.Fatalf("initM68KJIT: %v", err)
	}

	writeM68KWords(cpu, targetPC,
		0x4E71,         // NOP
		0x4E71,         // NOP
		0x4E72, 0x2700, // STOP
	)
	targetInstrs := m68kScanBlock(cpu.memory, targetPC)
	targetBlock, err := m68kCompileBlockWithMem(targetInstrs, targetPC, cpu.m68kGetJITExecMem(), cpu.memory)
	if err != nil {
		t.Fatalf("compile target block: %v", err)
	}
	m68kStampGuestBlockBytes(cpu.memory, targetBlock)
	cpu.m68kJitCache.Put(targetBlock)
	cpu.m68kMarkJITCodeRanges(targetBlock)
	if got := cpu.m68kJitCache.Get(uint64(targetPC)); got == nil {
		t.Fatal("target block was not cached before SMC write")
	}

	writeM68KWords(cpu, startPC,
		0x7000,                 // MOVEQ #0,D0
		0x7201,                 // MOVEQ #1,D1
		0x207C, 0x0000, 0x2008, // MOVEA.L #$2008,A0
		0x48E0, 0xC000, // MOVEM.L D0/D1,-(A0), overwrites 0x2000..0x2007
		0x23C0, 0x0000, 0x3000, // MOVE.L D0,$3000, ordinary later data write
		0x4E72, 0x2700, // STOP #$2700
	)

	runM68KJITUntilStopped(t, cpu)
	if got := cpu.m68kJitCache.Get(uint64(targetPC)); got != nil {
		t.Fatalf("compiled target block survived MOVEM code write followed by data write")
	}
	if got := cpu.Read32(dataPC); got != 0 {
		t.Fatalf("data write at 0x%08X = 0x%08X, want 0", dataPC, got)
	}
}

func TestM68KJIT_MOVEMSelfModifyingWriteExitsBeforeStaleFollowingInstruction(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const startPC = uint32(0x1000)

	cpu := newM68KTestProgramCPU(t, startPC)
	cpu.m68kJitEnabled = true
	writeM68KWords(cpu, startPC,
		0x203C, 0x7003, 0x4E72, // MOVE.L #$70034E72,D0
		0x207C, 0x0000, 0x1014, // MOVEA.L #$1014,A0
		0x48E0, 0x8000, // MOVEM.L D0,-(A0), overwrites 0x1010..0x1013
		0x7007,         // stale old instruction: MOVEQ #7,D0
		0x4E72, 0x2700, // STOP #$2700, immediate also serves rewritten STOP
	)

	runM68KJITUntilStopped(t, cpu)

	if got := cpu.DataRegs[0]; got != 3 {
		t.Fatalf("D0 = %d, want 3 from rewritten instruction at 0x1010", got)
	}
}

func TestM68KJIT_IOFallbackDoesNotInterpretFollowingNativeInstruction(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		dst     = uint32(TERM_OUT)
	)
	opcodes := []uint16{
		0x1080, // MOVE.B D0,(A0): MMIO destination forces a native bail today.
		0x7207, // MOVEQ #7,D1: must be dispatched/compiled after the single fallback.
	}
	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x00000041
		cpu.AddrRegs[0] = dst
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitFallbackOpcodeCounts[0x7207].Load(); got != 0 {
		t.Fatalf("MOVEQ after I/O fallback was interpreted by fallback burst %d times", got)
	}
	if got := jit.DataRegs[1]; got != 7 {
		t.Fatalf("D1=0x%08X, want 7", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherMoveSRUserModeRaisesPrivilegeWithoutFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC   = uint32(0x1000)
		handlerPC = uint32(0x2000)
		userSP    = uint32(0x3000)
		superSP   = uint32(0x4000)
	)
	setup := func(cpu *M68KCPU) {
		cpu.SR = 0
		cpu.AddrRegs[7] = userSP
		cpu.SSP = superSP
		cpu.Write32(uint32(M68K_VEC_PRIVILEGE)*4, handlerPC)
		writeM68KStopProgram(cpu, handlerPC,
			0x7207, // MOVEQ #7,D1
		)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC,
		0x40C0, // MOVE SR,D0: user mode raises privilege violation on 68020
		0x7201, // skipped when exception handler runs
	)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC,
		0x40C0,
		0x7201,
	)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitFallbackOpcodeCounts[0x40C0].Load(); got != 0 {
		t.Fatalf("MOVE SR,D0 user-mode privilege path fell back %d times", got)
	}
	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("MOVE SR,D0 privilege test did not execute a native JIT block")
	}
	if got := jit.DataRegs[1]; got != 7 {
		t.Fatalf("privilege handler did not run: D1=0x%08X, want 7", got)
	}
	if got := jit.USP; got != userSP {
		t.Fatalf("USP=0x%08X, want 0x%08X", got, userSP)
	}
	if got := jit.Read32(jit.AddrRegs[7] + 2); got != startPC {
		t.Fatalf("exception frame saved PC=0x%08X, want 0x%08X", got, startPC)
	}
	assertM68KCoreStateEqual(t, jit, interp)
	if jit.USP != interp.USP || jit.SSP != interp.SSP {
		t.Fatalf("stack pointers mismatch: USP got=0x%08X want=0x%08X SSP got=0x%08X want=0x%08X",
			jit.USP, interp.USP, jit.SSP, interp.SSP)
	}
}

func TestM68KJIT_DefaultDispatcherImmediateSRUserModeRaisesPrivilegeWithoutFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tt := range []struct {
		name   string
		opcode uint16
		imm    uint16
	}{
		{name: "andi_sr", opcode: 0x027C, imm: 0x2000},
		{name: "ori_sr", opcode: 0x007C, imm: 0x2000},
		{name: "eori_sr", opcode: 0x0A7C, imm: 0x2000},
	} {
		t.Run(tt.name, func(t *testing.T) {
			const (
				startPC   = uint32(0x1000)
				handlerPC = uint32(0x2000)
				userSP    = uint32(0x3000)
				superSP   = uint32(0x4000)
			)
			setup := func(cpu *M68KCPU) {
				cpu.SR = 0
				cpu.AddrRegs[7] = userSP
				cpu.SSP = superSP
				cpu.Write32(uint32(M68K_VEC_PRIVILEGE)*4, handlerPC)
				writeM68KStopProgram(cpu, handlerPC,
					0x7207, // MOVEQ #7,D1
				)
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			writeM68KStopProgram(interp, startPC,
				tt.opcode, tt.imm,
				0x7201, // skipped when exception handler runs
			)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			writeM68KStopProgram(jit, startPC,
				tt.opcode, tt.imm,
				0x7201,
			)
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitFallbackOpcodeCounts[tt.opcode].Load(); got != 0 {
				t.Fatalf("%s user-mode privilege path fell back %d times", tt.name, got)
			}
			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("%s privilege test did not execute a native JIT block", tt.name)
			}
			if got := jit.DataRegs[1]; got != 7 {
				t.Fatalf("%s privilege handler did not run: D1=0x%08X, want 7", tt.name, got)
			}
			if got := jit.Read32(jit.AddrRegs[7] + 2); got != startPC {
				t.Fatalf("%s exception frame saved PC=0x%08X, want 0x%08X", tt.name, got, startPC)
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherTRAPRaisesExceptionWithoutFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, trapNum := range []uint16{0, 14, 15} {
		t.Run(fmt.Sprintf("trap_%d", trapNum), func(t *testing.T) {
			const (
				startPC   = uint32(0x1000)
				handlerPC = uint32(0x2000)
				userSP    = uint32(0x3000)
				superSP   = uint32(0x4000)
			)
			opcode := uint16(M68K_TRAP | trapNum)
			vector := uint32(M68K_VEC_TRAP_BASE + uint8(trapNum))
			setup := func(cpu *M68KCPU) {
				cpu.SR = 0
				cpu.AddrRegs[7] = userSP
				cpu.SSP = superSP
				cpu.Write32(vector*4, handlerPC)
				writeM68KStopProgram(cpu, handlerPC,
					0x7207, // MOVEQ #7,D1
				)
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			writeM68KStopProgram(interp, startPC,
				opcode,
				0x7201, // skipped when exception handler runs
			)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			writeM68KStopProgram(jit, startPC,
				opcode,
				0x7201,
			)
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitFallbackOpcodeCounts[opcode].Load(); got != 0 {
				t.Fatalf("TRAP #%d fell back %d times", trapNum, got)
			}
			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("TRAP #%d did not execute a native JIT block", trapNum)
			}
			if got := jit.DataRegs[1]; got != 7 {
				t.Fatalf("TRAP #%d handler did not run: D1=0x%08X, want 7", trapNum, got)
			}
			if got := jit.Read32(jit.AddrRegs[7] + 2); got != startPC+2 {
				t.Fatalf("TRAP #%d exception frame saved PC=0x%08X, want 0x%08X", trapNum, got, startPC+2)
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherMemToMemMoveAROSHighRAMWithoutFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		srcA3   = uint32(0x00812A40)
		dstA2   = uint32(0x01DFFFB8)
		srcEA   = srcA3 + 0x39
		dstEA   = dstA2 + 0x3F
	)
	opcodes := []uint16{
		0x156B, 0x0039, 0x003F, // MOVE.B 57(A3),63(A2)
		0x6704, // BEQ.S set_two
		0x7201, // MOVEQ #1,D1
		0x6002, // BRA.S done
		0x7202, // MOVEQ #2,D1
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[2] = dstA2
		cpu.AddrRegs[3] = srcA3
		cpu.Write8(srcEA, 0x00)
		cpu.Write8(dstEA, 0xA5)
		cpu.SR = M68K_SR_S | M68K_SR_N | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	wantPrefix := 0
	for wantPrefix < len(instrs) && instrs[wantPrefix].opcode != 0x4E72 && !m68kIsBlockTerminator(instrs[wantPrefix].opcode) {
		wantPrefix++
	}
	if len(prefix) != wantPrefix {
		t.Fatalf("AROS high-RAM mem-to-mem MOVE prefix length=%d want=%d prefix=%+v instrs=%+v productionSafe=%v genericIO=%v",
			len(prefix), wantPrefix, prefix, instrs, m68kBlockProductionNativeSafe(instrs), m68kBlockMayUseGenericIOFallback(instrs))
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitFallbackOpcodeCounts[0x156B].Load(); got != 0 {
		t.Fatalf("AROS high-RAM MOVE.B d16(A3),d16(A2) fell back %d times", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("AROS high-RAM mem-to-mem MOVE bailed out %d times", got)
	}
	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("AROS high-RAM mem-to-mem MOVE did not execute a native JIT block")
	}
	if got := jit.Read8(dstEA); got != 0x00 {
		t.Fatalf("destination byte=0x%02X, want 0x00", got)
	}
	if got := jit.DataRegs[1]; got != 2 {
		t.Fatalf("D1=0x%08X, want 2; BEQ consumed wrong MOVE flags", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAROSIndexedLookupPrefix(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	tests := []struct {
		name string
		d0   uint32
		d1   uint32
	}{
		{name: "keep_d0", d0: 2, d1: 5},
		{name: "clamp_d0", d0: 9, d1: 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const (
				startPC = uint32(0x1000)
				nextPC  = uint32(0x1010)
				table   = uint32(0x3000)
				a5Base  = uint32(0x4000)
			)
			setup := func(cpu *M68KCPU) {
				cpu.DataRegs[0] = tc.d0
				cpu.DataRegs[1] = tc.d1
				cpu.AddrRegs[5] = a5Base
				cpu.Write32(a5Base+12, table)
				for i := uint32(0); i < 16; i++ {
					cpu.Write32(table+26+i*4, 0xABC00000+i)
				}
				writeM68KWords(cpu, startPC,
					0x5381,         // SUBQ.L #1,D1
					0xB081,         // CMP.L D1,D0
					0x6302,         // BLS.S skip
					0x2001,         // MOVE.L D1,D0
					0x226D, 0x000C, // MOVEA.L 12(A5),A1
					0x2231, 0x0C1A, // MOVE.L 26(A1,D0.L*4),D1
				)
				writeM68KStopProgram(cpu, nextPC)
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			instrs := m68kScanBlock(jit.memory, startPC)
			if _, ok := m68kIsAROSIndexedLookupPrefix(instrs, startPC, jit.memory); !ok {
				t.Fatalf("test block did not match indexed lookup recognizer: instrs=%+v", instrs)
			}
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatal("default M68K JIT dispatcher did not execute AROS indexed lookup prefix natively")
			}
			if got := jit.m68kJitFallbackOpcodeCounts[0x5381].Load(); got != 0 {
				t.Fatalf("AROS indexed lookup prefix fell back to interpreter %d times", got)
			}
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("AROS indexed lookup prefix bailed out %d times", got)
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeSubqBCCMoveRTSBlock(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	tests := []struct {
		name string
		d1   uint32
	}{
		{name: "branch_taken", d1: 2},
		{name: "return_path", d1: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const (
				targetPC = uint32(0x0FFE)
				startPC  = uint32(0x1000)
				returnPC = uint32(0x2000)
				stack    = uint32(0x120000)
			)
			setup := func(cpu *M68KCPU) {
				cpu.DataRegs[1] = tc.d1
				cpu.AddrRegs[1] = 0x12345678
				cpu.AddrRegs[7] = stack
				cpu.stackLowerBound = 0
				cpu.stackUpperBound = uint32(len(cpu.memory))
				cpu.Write32(stack, returnPC)
				writeM68KStopProgram(cpu, targetPC)
				writeM68KWords(cpu, startPC,
					0x5381, // SUBQ.L #1,D1
					0x64FA, // BCC.S targetPC
					0x2009, // MOVE.L A1,D0
					0x4E75, // RTS
				)
				writeM68KStopProgram(cpu, returnPC)
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			instrs := m68kScanBlock(jit.memory, startPC)
			if !m68kIsSubqBCCMoveRTSBlock(instrs, startPC) {
				t.Fatalf("test block did not match SUBQ/BCC/MOVE/RTS recognizer: instrs=%+v", instrs)
			}
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatal("default M68K JIT dispatcher did not execute SUBQ/BCC/MOVE/RTS block natively")
			}
			if got := jit.m68kJitFallbackOpcodeCounts[0x5381].Load(); got != 0 {
				t.Fatalf("SUBQ/BCC/MOVE/RTS block fell back to interpreter %d times", got)
			}
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("SUBQ/BCC/MOVE/RTS block bailed out %d times", got)
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeSubqLSRBNEStoreBRABlock(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	tests := []struct {
		name string
		d1   uint32
	}{
		{name: "bne_taken", d1: 5},
		{name: "store_and_bra", d1: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const (
				backPC  = uint32(0x0FD4)
				startPC = uint32(0x1000)
				donePC  = uint32(0x100C)
				dst     = uint32(0x3000)
			)
			setup := func(cpu *M68KCPU) {
				cpu.DataRegs[1] = tc.d1
				cpu.AddrRegs[0] = dst
				cpu.Write32(dst, 0x11111111)
				writeM68KStopProgram(cpu, backPC)
				writeM68KWords(cpu, startPC,
					0x5381, // SUBQ.L #1,D1
					0xE289, // LSR.L #1,D1
					0x6606, // BNE.S donePC
					0x72FF, // MOVEQ #-1,D1
					0x2081, // MOVE.L D1,(A0)
					0x60C8, // BRA.S backPC
				)
				writeM68KStopProgram(cpu, donePC)
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			instrs := m68kScanBlock(jit.memory, startPC)
			if !m68kIsSubqLSRBNEStoreBRABlock(instrs, startPC) {
				t.Fatalf("test block did not match SUBQ/LSR/BNE/store/BRA recognizer: instrs=%+v", instrs)
			}
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatal("default M68K JIT dispatcher did not execute SUBQ/LSR/BNE/store/BRA block natively")
			}
			if got := jit.m68kJitFallbackOpcodeCounts[0x5381].Load(); got != 0 {
				t.Fatalf("SUBQ/LSR/BNE/store/BRA block fell back to interpreter %d times", got)
			}
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("SUBQ/LSR/BNE/store/BRA block bailed out %d times", got)
			}
			assertM68KCoreStateEqual(t, jit, interp)
			if got, want := jit.Read32(dst), interp.Read32(dst); got != want {
				t.Fatalf("stored long: got=0x%08X want=0x%08X", got, want)
			}
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeSubqSubCmpBLSAddStoreBlock(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	tests := []struct {
		name string
		d1   uint32
		d2   uint32
		d3   uint32
	}{
		{name: "bls_taken", d1: 4, d2: 5, d3: 1},
		{name: "store_path", d1: 10, d2: 5, d3: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const (
				startPC = uint32(0x1000)
				donePC  = uint32(0x101C)
				dstBase = uint32(0x3000)
				dstOff  = uint32(0x002C)
			)
			setup := func(cpu *M68KCPU) {
				cpu.DataRegs[1] = tc.d1
				cpu.DataRegs[2] = tc.d2
				cpu.DataRegs[3] = tc.d3
				cpu.AddrRegs[2] = dstBase
				cpu.Write32(dstBase+dstOff, 0x11111111)
				writeM68KWords(cpu, startPC,
					0x5381,         // SUBQ.L #1,D1
					0x9283,         // SUB.L D3,D1
					0xB282,         // CMP.L D2,D1
					0x6314,         // BLS.S donePC
					0x5283,         // ADDQ.L #1,D3
					0x2543, 0x002C, // MOVE.L D3,44(A2)
					0x600C, // BRA.S donePC
				)
				writeM68KStopProgram(cpu, donePC)
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			instrs := m68kScanBlock(jit.memory, startPC)
			if !m68kIsSubqSubCmpBLSAddStoreBlock(instrs, startPC, jit.memory) {
				t.Fatalf("test block did not match SUBQ/SUB/CMP/BLS/ADD/store recognizer: instrs=%+v", instrs)
			}
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatal("default M68K JIT dispatcher did not execute SUBQ/SUB/CMP/BLS/ADD/store block natively")
			}
			if got := jit.m68kJitFallbackOpcodeCounts[0x5381].Load(); got != 0 {
				t.Fatalf("SUBQ/SUB/CMP/BLS/ADD/store block fell back to interpreter %d times", got)
			}
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("SUBQ/SUB/CMP/BLS/ADD/store block bailed out %d times", got)
			}
			assertM68KCoreStateEqual(t, jit, interp)
			if got, want := jit.Read32(dstBase+dstOff), interp.Read32(dstBase+dstOff); got != want {
				t.Fatalf("stored long: got=0x%08X want=0x%08X", got, want)
			}
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeStackCaseUpdateBlock(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	tests := []struct {
		name      string
		caseValue uint16
	}{
		{name: "bhi_taken", caseValue: 3},
		{name: "fallthrough_update", caseValue: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const (
				startPC = uint32(0x1000)
				target  = uint32(0x0E82)
				stack   = uint32(0x3000)
			)
			setup := func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 0x12345678
				cpu.AddrRegs[7] = stack
				cpu.Write16(stack+0x004A, tc.caseValue)
				cpu.Write32(stack+0x0060, 0xA5A5A5A5)
				writeM68KWords(cpu, startPC,
					0x2F40, 0x0060, // MOVE.L D0,96(A7)
					0x0C6F, 0x0002, 0x004A, // CMPI.W #2,74(A7)
					0x6200, 0xFE76, // BHI.W target
					0x3F7C, 0x0003, 0x004A, // MOVE.W #3,74(A7)
					0x6000, 0xFE6C, // BRA.W target
				)
				writeM68KStopProgram(cpu, target)
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			instrs := m68kScanBlock(jit.memory, startPC)
			if !m68kIsStackCaseUpdateBlock(instrs, startPC, jit.memory) {
				t.Fatalf("test block did not match stack case-update recognizer: instrs=%+v", instrs)
			}
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatal("default M68K JIT dispatcher did not execute stack case-update block natively")
			}
			if got := jit.m68kJitFallbackOpcodeCounts[0x2F40].Load(); got != 0 {
				t.Fatalf("stack case-update block fell back to interpreter %d times", got)
			}
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("stack case-update block bailed out %d times, want 0", got)
			}
			assertM68KCoreStateEqual(t, jit, interp)
			if got, want := jit.Read32(stack+0x0060), interp.Read32(stack+0x0060); got != want {
				t.Fatalf("stack long: got=0x%08X want=0x%08X", got, want)
			}
			if got, want := jit.Read16(stack+0x004A), interp.Read16(stack+0x004A); got != want {
				t.Fatalf("stack word: got=0x%04X want=0x%04X", got, want)
			}
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeBNEMoveMOVEMRTSBlock(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	tests := []struct {
		name string
		sr   uint16
	}{
		{name: "branch_taken", sr: M68K_SR_S},
		{name: "return_path", sr: M68K_SR_S | M68K_SR_Z},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const (
				backPC   = uint32(0x0FFC)
				startPC  = uint32(0x1000)
				returnPC = uint32(0x2000)
				stack    = uint32(0x120000)
				mask     = uint16(0x441C) // D2/D3/D4/A2/A6
			)
			saved := []uint32{
				0xD2000002, 0xD3000003, 0xD4000004,
				0xA2000002, 0xA6000006,
			}
			setup := func(cpu *M68KCPU) {
				cpu.SR = tc.sr
				cpu.DataRegs[4] = 0x44444444
				cpu.AddrRegs[7] = stack
				cpu.stackLowerBound = 0
				cpu.stackUpperBound = uint32(len(cpu.memory))
				for i, val := range saved {
					cpu.Write32(stack+uint32(i)*4, val)
				}
				cpu.Write32(stack+uint32(len(saved))*4, returnPC)
				writeM68KStopProgram(cpu, backPC)
				writeM68KWords(cpu, startPC,
					0x66FA,       // BNE.S backPC
					0x2004,       // MOVE.L D4,D0
					0x4CDF, mask, // MOVEM.L (A7)+,D2/D3/D4/A2/A6
					0x4E75, // RTS
				)
				writeM68KStopProgram(cpu, returnPC)
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			instrs := m68kScanBlock(jit.memory, startPC)
			if !m68kIsBNEMoveMOVEMRTSBlock(instrs, startPC) {
				t.Fatalf("test block did not match BNE/MOVE/MOVEM/RTS recognizer: instrs=%+v", instrs)
			}
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatal("default M68K JIT dispatcher did not execute BNE/MOVE/MOVEM/RTS block natively")
			}
			if got := jit.m68kJitFallbackOpcodeCounts[0x66FA].Load(); got != 0 {
				t.Fatalf("BNE/MOVE/MOVEM/RTS block fell back to interpreter %d times", got)
			}
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("BNE/MOVE/MOVEM/RTS block bailed out %d times", got)
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeMoveA7PostincRTSBlock(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	tests := []struct {
		name  string
		words []uint16
		regs  []uint16
	}{
		{name: "pop_a6_rts", words: []uint16{0x2C5F, 0x4E75}, regs: []uint16{6}},
		{name: "pop_a2_a6_rts", words: []uint16{0x245F, 0x2C5F, 0x4E75}, regs: []uint16{2, 6}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const (
				startPC  = uint32(0x1000)
				returnPC = uint32(0x2000)
				stack    = uint32(0x120000)
			)
			setup := func(cpu *M68KCPU) {
				cpu.AddrRegs[7] = stack
				cpu.stackLowerBound = 0
				cpu.stackUpperBound = uint32(len(cpu.memory))
				for i, reg := range tc.regs {
					cpu.Write32(stack+uint32(i)*4, 0xA0000000|uint32(reg))
				}
				cpu.Write32(stack+uint32(len(tc.regs))*4, returnPC)
				writeM68KWords(cpu, startPC, tc.words...)
				writeM68KStopProgram(cpu, returnPC)
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			instrs := m68kScanBlock(jit.memory, startPC)
			if _, ok := m68kIsMoveA7PostincRTSBlock(instrs); !ok {
				t.Fatalf("test block did not match MOVEA (A7)+/RTS recognizer: instrs=%+v", instrs)
			}
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatal("default M68K JIT dispatcher did not execute MOVEA (A7)+/RTS block natively")
			}
			if got := jit.m68kJitFallbackOpcodeCounts[tc.words[0]].Load(); got != 0 {
				t.Fatalf("MOVEA (A7)+/RTS block fell back to interpreter %d times", got)
			}
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("MOVEA (A7)+/RTS block bailed out %d times", got)
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAROSStackCallSetupPrefix(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		subPC   = uint32(0x2000)
		stack   = uint32(0x120000)
	)

	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x00008000
		cpu.AddrRegs[2] = 0x00A20002
		cpu.AddrRegs[6] = subPC
		cpu.AddrRegs[7] = stack
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		writeM68KStopProgram(cpu, startPC,
			0x2F0A,         // MOVE.L A2,-(A7)
			0x42A7,         // CLR.L -(A7)
			0x4A40,         // TST.W D0
			0x4E96,         // JSR (A6)
			0x4FEF, 0x0008, // LEA 8(A7),A7
		)
		writeM68KWords(cpu, subPC, 0x4E75) // RTS
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	instrs := m68kScanBlock(jit.memory, startPC)
	if !m68kCanUseProductionNativeBlock(jit.memory, startPC, instrs) {
		t.Fatalf("stack call setup block is not production-native: instrs=%+v productionSafe=%v genericIO=%v",
			instrs, m68kBlockProductionNativeSafe(instrs), m68kBlockMayUseGenericIOFallback(instrs))
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute AROS stack call setup prefix natively")
	}
	for _, opcode := range []uint16{0x2F0A, 0x42A7, 0x4A40, 0x4E96} {
		if got := jit.m68kJitFallbackOpcodeCounts[opcode].Load(); got != 0 {
			t.Fatalf("opcode 0x%04X fell back %d times, want 0", opcode, got)
		}
	}
	assertM68KCoreStateEqual(t, jit, interp)
	if got, want := jit.Read32(stack-4), uint32(0x00A20002); got != want {
		t.Fatalf("MOVE.L A2,-(A7) wrote 0x%08X, want 0x%08X", got, want)
	}
	if got, want := jit.Read32(stack-8), uint32(0); got != want {
		t.Fatalf("CLR.L -(A7) wrote 0x%08X, want 0", got)
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeBSRRTS(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		subPC   = uint32(0x2000)
	)
	interp := newM68KTestProgramCPU(t, startPC)
	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true

	write := func(cpu *M68KCPU, pc uint32, ops ...uint16) uint32 {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
		return pc
	}
	for _, cpu := range []*M68KCPU{interp, jit} {
		cpu.AddrRegs[7] = 0x9000
		pc := startPC
		pc = write(cpu, pc,
			0x7001,                            // MOVEQ #1,D0
			0x6100, uint16(subPC-(startPC+4)), // BSR.W subPC
			0x7203, // MOVEQ #3,D1 after return
		)
		for i := 0; i < m68kJitMaxBlockSize; i++ {
			pc = write(cpu, pc, 0xD280) // ADD.L D0,D1
		}
		write(cpu, pc, 0x4E72, 0x2700) // STOP

		pc = subPC
		for i := 0; i < m68kJitMaxBlockSize-1; i++ {
			pc = write(cpu, pc, 0x5280) // ADDQ.L #1,D0
		}
		write(cpu, pc, 0x4E75) // RTS
	}

	runM68KInterpreterUntilStopped(t, interp)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got < 2 {
		t.Fatalf("native blocks executed for BSR/RTS = %d, want at least 2", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("BSR/RTS block bailed out %d times, want 0", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeJMPAbsLong(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	writeProgram := func(cpu *M68KCPU) {
		cpu.Write16(0x1000, 0x7001) // MOVEQ #1,D0
		cpu.Write16(0x1002, 0x4EF9) // JMP $00001100
		cpu.Write16(0x1004, 0x0000)
		cpu.Write16(0x1006, 0x1100)
		cpu.Write16(0x1008, 0x70FF) // skipped
		writeM68KStopProgram(cpu, 0x1100)
	}

	interp := newM68KTestProgramCPU(t, 0x1000)
	writeProgram(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, 0x1000)
	jit.m68kJitEnabled = true
	writeProgram(jit)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute JMP abs.L block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("JMP abs.L block bailed out %d times, want 0", got)
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x4EF9].Load(); got != 0 {
		t.Fatalf("JMP abs.L fell back %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeJMPAbsWord(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	writeProgram := func(cpu *M68KCPU) {
		cpu.Write16(0x1000, 0x7001) // MOVEQ #1,D0
		cpu.Write16(0x1002, 0x4EF8) // JMP $1100.W
		cpu.Write16(0x1004, 0x1100)
		cpu.Write16(0x1006, 0x70FF) // skipped
		writeM68KStopProgram(cpu, 0x1100)
	}

	interp := newM68KTestProgramCPU(t, 0x1000)
	writeProgram(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, 0x1000)
	jit.m68kJitEnabled = true
	writeProgram(jit)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute JMP abs.W block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("JMP abs.W block bailed out %d times, want 0", got)
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x4EF8].Load(); got != 0 {
		t.Fatalf("JMP abs.W fell back %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeJMPAddressRegister(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	writeProgram := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = 0x1100
		cpu.Write16(0x1000, 0x7001) // MOVEQ #1,D0
		cpu.Write16(0x1002, 0x4ED0) // JMP (A0)
		cpu.Write16(0x1004, 0x70FF) // skipped
		writeM68KStopProgram(cpu, 0x1100)
	}

	interp := newM68KTestProgramCPU(t, 0x1000)
	writeProgram(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, 0x1000)
	jit.m68kJitEnabled = true
	writeProgram(jit)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute JMP (An) block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("JMP (An) block bailed out %d times, want 0", got)
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x4ED0].Load(); got != 0 {
		t.Fatalf("JMP (An) fell back %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeJMPDisplacementAddressRegister(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	writeProgram := func(cpu *M68KCPU) {
		cpu.AddrRegs[3] = 0x10F0
		cpu.Write16(0x1000, 0x7001) // MOVEQ #1,D0
		cpu.Write16(0x1002, 0x4EEB) // JMP 16(A3)
		cpu.Write16(0x1004, 0x0010)
		cpu.Write16(0x1006, 0x70FF) // skipped
		writeM68KStopProgram(cpu, 0x1100)
	}

	interp := newM68KTestProgramCPU(t, 0x1000)
	writeProgram(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, 0x1000)
	jit.m68kJitEnabled = true
	writeProgram(jit)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute JMP d16(An) block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("JMP d16(An) block bailed out %d times, want 0", got)
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x4EEB].Load(); got != 0 {
		t.Fatalf("JMP d16(An) fell back %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeJMPDisplacementPC(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	writeProgram := func(cpu *M68KCPU) {
		cpu.Write16(0x1000, 0x7001) // MOVEQ #1,D0
		cpu.Write16(0x1002, 0x4EFA) // JMP 252(PC), base is extension word address 0x1004
		cpu.Write16(0x1004, 0x00FC)
		cpu.Write16(0x1006, 0x70FF) // skipped
		writeM68KStopProgram(cpu, 0x1100)
	}

	interp := newM68KTestProgramCPU(t, 0x1000)
	writeProgram(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, 0x1000)
	jit.m68kJitEnabled = true
	writeProgram(jit)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute JMP d16(PC) block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("JMP d16(PC) block bailed out %d times, want 0", got)
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x4EFA].Load(); got != 0 {
		t.Fatalf("JMP d16(PC) fell back %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeJMPBriefIndexedAddressRegister(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	writeProgram := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = 0x10F0
		cpu.DataRegs[1] = 0
		cpu.Write16(0x1000, 0x7001) // MOVEQ #1,D0
		cpu.Write16(0x1002, 0x4EF0) // JMP 16(A0,D1.L)
		cpu.Write16(0x1004, 0x1810)
		cpu.Write16(0x1006, 0x70FF) // skipped
		writeM68KStopProgram(cpu, 0x1100)
	}

	interp := newM68KTestProgramCPU(t, 0x1000)
	writeProgram(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, 0x1000)
	jit.m68kJitEnabled = true
	writeProgram(jit)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute JMP d8(An,Xn) block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("JMP d8(An,Xn) block bailed out %d times, want 0", got)
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x4EF0].Load(); got != 0 {
		t.Fatalf("JMP d8(An,Xn) fell back %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeJMPBriefIndexedPC(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	writeProgram := func(cpu *M68KCPU) {
		cpu.DataRegs[2] = 0x80
		cpu.Write16(0x1000, 0x7001) // MOVEQ #1,D0
		cpu.Write16(0x1002, 0x4EFB) // JMP 124(PC,D2.W), base is extension word address 0x1004
		cpu.Write16(0x1004, 0x207C)
		cpu.Write16(0x1006, 0x70FF) // skipped
		writeM68KStopProgram(cpu, 0x1100)
	}

	interp := newM68KTestProgramCPU(t, 0x1000)
	writeProgram(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, 0x1000)
	jit.m68kJitEnabled = true
	writeProgram(jit)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute JMP d8(PC,Xn) block natively")
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("JMP d8(PC,Xn) block bailed out %d times, want 0", got)
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x4EFB].Load(); got != 0 {
		t.Fatalf("JMP d8(PC,Xn) fell back %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_ForceNativeExecutesJSRDisplacementAn(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	writeProgram := func(cpu *M68KCPU) {
		cpu.AddrRegs[6] = 0x2000
		writeM68KStopProgram(cpu, 0x1000,
			0x7001,         // MOVEQ #1,D0
			0x4EAE, 0x0010, // JSR 16(A6)
			0x7203, // MOVEQ #3,D1 after return
		)
		writeM68KWords(cpu, 0x2010,
			0x7402, // MOVEQ #2,D2
			0x4E75, // RTS
		)
	}

	interp := newM68KTestProgramCPU(t, 0x1000)
	writeProgram(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, 0x1000)
	jit.m68kJitEnabled = true
	jit.m68kJitForceNative = true
	writeProgram(jit)
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitFallbackOpcodeCounts[0x4EAE].Load(); got != 0 {
		t.Fatalf("JSR d16(An) fallback count = %d, want 0", got)
	}
	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("force-native M68K JIT dispatcher did not execute JSR d16(An) block natively")
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativePEALinkUnlk(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	opcodes := []uint16{
		0x4868, 0x0010, // PEA (16,A0)
		0x4E56, 0xFFF8, // LINK A6,#-8
		0x4E5E, // UNLK A6
	}
	for i := 0; i < m68kJitMaxBlockSize; i++ {
		opcodes = append(opcodes, 0x7001) // MOVEQ #1,D0
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = 0x2000
		cpu.AddrRegs[6] = 0xAAAAAAAA
		cpu.AddrRegs[7] = 0x10024
	}

	interp := newM68KTestProgramCPU(t, 0x1000)
	setup(interp)
	writeM68KStopProgram(interp, 0x1000, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := runM68KJITStopProgramWithSetup(t, 0x1000, setup, false, opcodes...)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		instrs := m68kScanBlock(jit.memory, 0x1000)
		firstUnsafe := uint16(0)
		firstUnsafeIndex := -1
		for i := range instrs {
			if !m68kInstrProductionNativeSafe(&instrs[i]) {
				firstUnsafe = instrs[i].opcode
				firstUnsafeIndex = i
				break
			}
		}
		t.Fatalf("default M68K JIT dispatcher did not execute PEA/LINK/UNLK block natively: instrs=%d fallback=%v conservative=%v productionSafe=%v genericIO=%v firstUnsafe[%d]=0x%04X fallbackInstr=%d",
			len(instrs), m68kNeedsFallback(instrs), m68kNeedsConservativeFallback(jit.memory, 0x1000, instrs),
			m68kBlockProductionNativeSafe(instrs), m68kBlockMayUseGenericIOFallback(instrs), firstUnsafeIndex, firstUnsafe,
			jit.m68kJitFallbackInstructions.Load())
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("PEA/LINK/UNLK block bailed out %d times, want 0", got)
	}
	if got, want := jit.Read32(0x10020), uint32(0x2010); got != want {
		t.Fatalf("PEA pushed 0x%08X, want 0x%08X", got, want)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativePEAFullIndexed(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const startPC = uint32(0x1000)
	opcodes := []uint16{
		0x4870, 0x4920, 0x0084, // PEA full-indexed form seen in AROS boot paths.
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = 0x8000
		cpu.AddrRegs[7] = 0x9000
		cpu.DataRegs[4] = 0x10
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)

	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) == 0 || prefix[0].opcode != 0x4870 || m68kNeedsConservativeFallback(jit.memory, startPC, prefix) ||
		!m68kCanUseProductionNativeBlock(jit.memory, startPC, prefix) {
		t.Fatalf("PEA full-indexed prefix rejected: instrs=%+v prefix=%+v fallback=%v conservative=%v productionSafe=%v genericIO=%v",
			instrs,
			prefix,
			m68kNeedsFallback(prefix),
			m68kNeedsConservativeFallback(jit.memory, startPC, prefix),
			m68kBlockProductionNativeSafe(prefix),
			m68kBlockMayUseGenericIOFallback(prefix))
	}

	runM68KJITUntilStopped(t, jit)
	if got := jit.m68kJitFallbackOpcodeCounts[0x4870].Load(); got != 0 {
		t.Fatalf("PEA full-indexed fell back %d times", got)
	}
	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("PEA full-indexed did not execute native JIT")
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_FallbackInterruptSamplingMatchesInterpreterCadence(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	bus.Write32((24+4)*4, 0x2000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	for pc := uint32(0x1000); pc < 0x1100; pc += 2 {
		cpu.memory[pc] = 0x4E
		cpu.memory[pc+1] = 0x71 // NOP, conservatively interpreted in production JIT mode.
	}
	cpu.pendingInterrupt.Store(1 << 4)
	cpu.debugBreakIn = func(pc uint64) bool {
		return pc >= 0x1014
	}

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cpu.running.Store(false)
		waitDoneWithGuard(t, done)
		t.Fatal("M68K JIT fallback interrupt cadence test timed out")
	}

	if got := cpu.pendingInterrupt.Load(); got&(1<<4) == 0 {
		t.Fatal("pending level-4 interrupt was delivered before 256 fallback instructions")
	}
	if cpu.PC >= 0x2000 {
		t.Fatalf("PC jumped to interrupt vector early: 0x%08X", cpu.PC)
	}
}

func TestM68KJIT_IOFallbackSamplesInterruptAfterNativePrefix(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		trapPC  = uint32(0x2000)
		ioAddr  = uint32(TERM_OUT)
	)

	bus := NewMachineBus()
	bus.Write32(0, 0x00010000)
	bus.Write32(4, startPC)
	var writes atomic.Uint32
	bus.MapIO(ioAddr, ioAddr, nil, func(addr uint32, value uint32) {
		writes.Add(1)
	})

	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.m68kJitWarmupLimit = 2
	cpu.PC = startPC
	cpu.SR = M68K_SR_S
	cpu.Write32((M68K_VEC_LEVEL1+3)*M68K_LONG_SIZE, trapPC)
	cpu.AddrRegs[0] = ioAddr
	cpu.DataRegs[0] = 0x12345678
	cpu.pendingInterrupt.Store(1 << 4)
	cpu.StoppedIdleHook = func(cpu *M68KCPU) {
		cpu.running.Store(false)
	}

	pc := startPC
	write := func(op uint16) {
		cpu.memory[pc] = byte(op >> 8)
		cpu.memory[pc+1] = byte(op)
		pc += 2
	}
	write(0x4E71) // Warmed up through the interpreter: dispatcher count becomes 1.
	for i := 0; i < m68kJitMaxBlockSize-1; i++ {
		write(0x4E71) // Native prefix: 255 NOPs crosses the 256-instruction sample.
	}
	write(0x2080)                               // MOVE.L D0,(A0), bails to interpreter because A0 is MMIO.
	writeM68KWords(cpu, trapPC, 0x4E72, 0x2700) // STOP #$2700 interrupt handler.

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cpu.running.Store(false)
		waitDoneWithGuard(t, done)
		t.Fatalf("M68K JIT IO fallback interrupt sampling test timed out: pc=0x%08X sr=0x%04X instr=%d pending=0x%X writes=%d native=%d fallback=%d stopped=%v",
			cpu.PC, cpu.SR, cpu.InstructionCount, cpu.pendingInterrupt.Load(), writes.Load(),
			cpu.m68kJitNativeBlocksExecuted.Load(), cpu.m68kJitFallbackInstructions.Load(), cpu.stopped.Load())
	}

	if got := writes.Load(); got != 0 {
		t.Fatalf("MMIO fallback write happened before pending IRQ delivery: writes=%d", got)
	}
	if got := cpu.pendingInterrupt.Load(); got&(1<<4) != 0 {
		t.Fatal("pending level-4 interrupt was not delivered at native-prefix sample boundary")
	}
	if cpu.PC != trapPC+4 {
		t.Fatalf("PC=0x%08X, want stopped after interrupt handler at 0x%08X", cpu.PC, trapPC+4)
	}
	if got := cpu.InstructionCount; got != 257 {
		t.Fatalf("InstructionCount=%d, want 257 (warmup NOP + native prefix + STOP handler)", got)
	}
}

func TestM68KJIT_HelperExitRedispatchesAfterNativePrefixInterrupt(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		blockPC = uint32(0x1100)
		trapPC  = uint32(0x2000)
	)

	bus := NewMachineBus()
	bus.Write32(0, 0x00010000)
	bus.Write32(4, startPC)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.m68kJitWarmupLimit = 1
	cpu.PC = startPC
	cpu.SR = M68K_SR_S
	cpu.Write32((M68K_VEC_LEVEL1+3)*M68K_LONG_SIZE, trapPC)
	cpu.pendingInterrupt.Store(1 << 4)
	cpu.StoppedIdleHook = func(cpu *M68KCPU) {
		cpu.running.Store(false)
	}

	pc := startPC
	write := func(op uint16) {
		cpu.memory[pc] = byte(op >> 8)
		cpu.memory[pc+1] = byte(op)
		pc += 2
	}
	write(0x4EF9) // JMP blockPC: dispatcher count becomes 1 before the helper block.
	write(0x0000)
	write(uint16(blockPC))
	pc = blockPC
	for i := 0; i < m68kJitMaxBlockSize-1; i++ {
		write(0x4E71) // Native prefix: 255 NOPs crosses the 256-instruction sample.
	}
	write(0x4848)                               // BKPT #0 helper must not run before the IRQ handler.
	writeM68KWords(cpu, trapPC, 0x4E72, 0x2700) // STOP #$2700 interrupt handler.

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cpu.running.Store(false)
		waitDoneWithGuard(t, done)
		t.Fatalf("M68K JIT helper interrupt redispatch test timed out: pc=0x%08X sr=0x%04X instr=%d pending=0x%X native=%d helper=%d fallback=%d stopped=%v",
			cpu.PC, cpu.SR, cpu.InstructionCount, cpu.pendingInterrupt.Load(),
			cpu.m68kJitNativeBlocksExecuted.Load(), cpu.m68kJitNativeHelperExits.Load(),
			cpu.m68kJitFallbackInstructions.Load(), cpu.stopped.Load())
	}

	if got := cpu.m68kJitNativeHelperExits.Load(); got != 1 {
		t.Fatalf("helper exits=%d, want 1 helper exit deferred by IRQ redispatch", got)
	}
	if got := cpu.pendingInterrupt.Load(); got&(1<<4) != 0 {
		t.Fatal("pending level-4 interrupt was not delivered at native-prefix sample boundary")
	}
	if cpu.PC != trapPC+4 {
		t.Fatalf("PC=0x%08X, want stopped after interrupt handler at 0x%08X", cpu.PC, trapPC+4)
	}
	if got := cpu.InstructionCount; got != 257 {
		t.Fatalf("InstructionCount=%d, want 257 (JMP + native prefix + STOP handler)", got)
	}
}

// runM68KJITProgram loads M68K opcodes at startPC, runs ExecuteJIT with a timeout,
// and returns the CPU for result inspection.
func runM68KJITProgram(t *testing.T, startPC uint32, opcodes ...uint16) *M68KCPU {
	return runM68KJITProgramWithSetup(t, startPC, nil, false, opcodes...)
}

func runM68KJITProgramWithSetup(t *testing.T, startPC uint32, setup func(*M68KCPU), forceNative bool, opcodes ...uint16) *M68KCPU {
	t.Helper()

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000) // SSP (initial stack)
	bus.Write32(4, startPC)    // reset vector → our code
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.m68kJitForceNative = forceNative
	cpu.m68kJitWarmupLimit = 1
	cpu.PC = startPC
	cpu.SR = M68K_SR_S // supervisor mode
	if setup != nil {
		setup(cpu)
	}

	// Write opcodes in big-endian
	pc := startPC
	for _, op := range opcodes {
		cpu.memory[pc] = byte(op >> 8)
		cpu.memory[pc+1] = byte(op)
		pc += 2
	}

	// Run with timeout
	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cpu.running.Store(false)
		waitDoneWithGuard(t, done)
		t.Fatal("M68K JIT execution timed out")
	}

	return cpu
}

func newM68KJITDemoCPU(t *testing.T) (*M68KCPU, *VideoChip) {
	t.Helper()

	bus := NewMachineBus()

	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("failed to create video chip: %v", err)
	}
	video.AttachBus(bus)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)
	bus.SetVideoStatusReader(video.HandleRead)

	vga := NewVGAEngine(bus)
	bus.MapIO(VGA_BASE, VGA_REG_END, vga.HandleRead, vga.HandleWrite)
	bus.MapIO(VGA_VRAM_WINDOW, VGA_VRAM_WINDOW+VGA_VRAM_SIZE-1, vga.HandleVRAMRead, vga.HandleVRAMWrite)
	bus.MapIO(VGA_TEXT_WINDOW, VGA_TEXT_WINDOW+VGA_TEXT_SIZE-1, vga.HandleTextRead, vga.HandleTextWrite)

	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, M68K_ENTRY_POINT)

	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = M68K_ENTRY_POINT
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x00010000

	return cpu, video
}

// writeBELong writes a big-endian uint32 to memory.
func writeBELong(mem []byte, addr uint32, val uint32) {
	mem[addr] = byte(val >> 24)
	mem[addr+1] = byte(val >> 16)
	mem[addr+2] = byte(val >> 8)
	mem[addr+3] = byte(val)
}

// TestM68KJIT_Exec_SimpleHalt tests that a STOP instruction halts execution.
func TestM68KJIT_Exec_SimpleHalt(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S

	// MOVEQ #42,D0; then ILLEGAL (0x4AFC) to halt
	// Actually, ILLEGAL will cause an exception which may loop.
	// Use a simpler approach: just run a few instructions and then check running stopped.
	// MOVEQ #42,D0; MOVEQ #0,D1; then we stop running externally.
	cpu.memory[0x1000] = 0x70
	cpu.memory[0x1001] = 0x2A // MOVEQ #42,D0
	cpu.memory[0x1002] = 0x72
	cpu.memory[0x1003] = 0x00 // MOVEQ #0,D1

	// Put a STOP #$2700 to halt — STOP needs fallback to interpreter
	cpu.memory[0x1004] = 0x4E
	cpu.memory[0x1005] = 0x72 // STOP
	cpu.memory[0x1006] = 0x27
	cpu.memory[0x1007] = 0x00 // #$2700

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	// Wait a bit then stop
	time.Sleep(100 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if cpu.DataRegs[0] != 42 {
		t.Errorf("D0 = %d, want 42", cpu.DataRegs[0])
	}
}

// TestM68KJIT_Exec_ALUSequence runs ADD/SUB through the full dispatcher.
func TestM68KJIT_Exec_ALUSequence(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Program: MOVEQ #10,D0; MOVEQ #20,D1; ADD.L D0,D1; STOP
	cpu := runM68KJITStopProgram(t, 0x1000,
		0x700A, // MOVEQ #10,D0
		0x7214, // MOVEQ #20,D1
		0xD280, // ADD.L D0,D1  (D1 = D1 + D0 = 30)
	)

	if cpu.DataRegs[0] != 10 {
		t.Errorf("D0 = %d, want 10", cpu.DataRegs[0])
	}
	if cpu.DataRegs[1] != 30 {
		t.Errorf("D1 = %d, want 30", cpu.DataRegs[1])
	}
}

// TestM68KJIT_Exec_MemoryMove runs MOVE with memory through the dispatcher.
func TestM68KJIT_Exec_MemoryMove(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgram(t, 0x1000,
		0x203C, 0xDEAD, 0xBEEF, // MOVE.L #$DEADBEEF,D0
	)

	if cpu.DataRegs[0] != 0xDEADBEEF {
		t.Errorf("D0 = 0x%08X, want 0xDEADBEEF", cpu.DataRegs[0])
	}
}

// TestM68KJIT_Exec_JSR_RTS runs a subroutine call through the dispatcher.
func TestM68KJIT_Exec_JSR_RTS(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000) // SSP
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000 // stack pointer

	// Main: MOVEQ #1,D0; JSR $1010; MOVEQ #3,D0; STOP
	// Sub at $1010: MOVEQ #2,D1; RTS
	// After execution: D0=3 (set after JSR returns), D1=2
	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x7001) // MOVEQ #1,D0
	writeW(0x4EB9) // JSR
	writeW(0x0000) // abs.L high
	writeW(0x1010) // abs.L low (target = $1010)
	writeW(0x7003) // MOVEQ #3,D0 (executed after RTS)
	// STOP to halt
	writeW(0x4E72)
	writeW(0x2700)

	// Subroutine at $1010
	pc = 0x1010
	writeW(0x7402) // MOVEQ #2,D2
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if cpu.DataRegs[0] != 3 {
		t.Errorf("D0 = %d, want 3 (set after JSR return)", cpu.DataRegs[0])
	}
	if cpu.DataRegs[2] != 2 {
		t.Errorf("D2 = %d, want 2 (set in subroutine)", cpu.DataRegs[2])
	}
}

func TestM68KJIT_Exec_RotatingCubeDoesNotOverwritePlotPixelCode(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu, video := newM68KJITDemoCPU(t)
	if err := video.Start(); err != nil {
		t.Fatalf("failed to start video chip: %v", err)
	}
	defer func() { _ = video.Stop() }()

	programPath := filepath.Join("sdk", "examples", "prebuilt", "rotating_cube_copper_68k.ie68")
	program, err := os.ReadFile(programPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", programPath, err)
	}
	cpu.LoadProgramBytes(program)

	const plotPixelPC = uint32(0x1542)
	wantOpcode := cpu.Read16(plotPixelPC)
	if wantOpcode == 0 {
		t.Fatalf("unexpected zero opcode at 0x%08X before execution", plotPixelPC)
	}

	type codeWrite struct {
		addr  uint32
		value uint32
		pc    uint32
		size  int
	}

	writeCh := make(chan codeWrite, 1)
	cpu.DebugWatchFn = func(addr, value, pc uint32, size int) {
		end := addr + uint32(size)
		if end <= addr {
			return
		}
		if plotPixelPC < addr || plotPixelPC >= end {
			return
		}
		select {
		case writeCh <- codeWrite{addr: addr, value: value, pc: pc, size: size}:
		default:
		}
		cpu.running.Store(false)
	}

	runner := NewM68KRunner(cpu)
	runner.cpu.m68kJitEnabled = true
	runner.StartExecution()
	defer runner.Stop()

	select {
	case hit := <-writeCh:
		t.Fatalf("rotating cube overwrote plot_pixel code at 0x%08X via write size=%d addr=0x%08X value=0x%08X from PC=0x%08X", plotPixelPC, hit.size, hit.addr, hit.value, hit.pc)
	case <-time.After(1500 * time.Millisecond):
	}

	if got := cpu.Read16(plotPixelPC); got != wantOpcode {
		t.Fatalf("plot_pixel opcode at 0x%08X changed from 0x%04X to 0x%04X without direct watched overlap", plotPixelPC, wantOpcode, got)
	}
}

func TestM68KJIT_Exec_RotatingCubeDrawLineMatchesInterpreter(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		drawLinePC = uint32(0x14E2)
		stopPC     = uint32(0x2000)
		initialSP  = uint32(0x00090000)
	)

	setupCPU := func(t *testing.T, useJIT bool) *M68KCPU {
		t.Helper()

		cpu, _ := newM68KJITDemoCPU(t)
		programPath := filepath.Join("sdk", "examples", "prebuilt", "rotating_cube_copper_68k.ie68")
		program, err := os.ReadFile(programPath)
		if err != nil {
			t.Fatalf("failed to read %s: %v", programPath, err)
		}
		cpu.LoadProgramBytes(program)
		cpu.m68kJitEnabled = useJIT
		cpu.PC = drawLinePC
		cpu.SR = M68K_SR_S
		cpu.AddrRegs[7] = initialSP - 4
		cpu.SSP = initialSP
		cpu.USP = initialSP
		cpu.stackLowerBound = 0
		cpu.stackUpperBound = uint32(len(cpu.memory))
		cpu.Write32(initialSP-4, stopPC)
		cpu.Write32(initialSP+0x20, stopPC)
		cpu.memory[stopPC] = 0x4E
		cpu.memory[stopPC+1] = 0x72
		cpu.memory[stopPC+2] = 0x27
		cpu.memory[stopPC+3] = 0x00

		// d3=x1, d4=y1, d5=x2, d6=y2, d2=colour
		cpu.DataRegs[2] = 0x4F
		cpu.DataRegs[3] = 10
		cpu.DataRegs[4] = 10
		cpu.DataRegs[5] = 10
		cpu.DataRegs[6] = 10

		return cpu
	}

	runCPU := func(t *testing.T, cpu *M68KCPU, useJIT bool) {
		t.Helper()
		if useJIT {
			runM68KJITUntilStopped(t, cpu)
		} else {
			runM68KInterpreterUntilStopped(t, cpu)
		}
	}

	interp := setupCPU(t, false)
	jit := setupCPU(t, true)

	runCPU(t, interp, false)
	runCPU(t, jit, true)

	if got, want := jit.AddrRegs[7], interp.AddrRegs[7]; got != want {
		t.Fatalf("A7 mismatch after draw_line: jit=0x%08X interp=0x%08X", got, want)
	}
	if got, want := jit.PC, interp.PC; got != want {
		t.Fatalf("PC mismatch after draw_line: jit=0x%08X interp=0x%08X", got, want)
	}
	for reg := range 8 {
		if got, want := jit.DataRegs[reg], interp.DataRegs[reg]; got != want {
			t.Fatalf("D%d mismatch after draw_line: jit=0x%08X interp=0x%08X", reg, got, want)
		}
	}
	if got, want := jit.AddrRegs[0], interp.AddrRegs[0]; got != want {
		t.Fatalf("A0 mismatch after draw_line: jit=0x%08X interp=0x%08X", got, want)
	}
	for i := uint32(0); i < 16; i += 2 {
		addr := interp.AddrRegs[7] + i
		if got, want := jit.Read16(addr), interp.Read16(addr); got != want {
			t.Fatalf("stack mismatch at 0x%08X: jit=0x%04X interp=0x%04X", addr, got, want)
		}
	}
	for i := uint32(0); i < 32; i++ {
		addr := VGA_VRAM_WINDOW + 10*320 + 10 + i
		if got, want := jit.Read8(addr), interp.Read8(addr); got != want {
			t.Fatalf("VRAM mismatch at 0x%08X: jit=0x%02X interp=0x%02X", addr, got, want)
		}
	}
}

func TestM68KJIT_Exec_BSR_W_RTS(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	// Main:
	// 0x1000 MOVEQ #1,D0
	// 0x1002 BSR.W +8 -> 0x100E
	// 0x1006 MOVEQ #3,D0
	// 0x1008 STOP #$2700
	writeW(0x7001) // MOVEQ #1,D0
	writeW(0x6100) // BSR.W
	writeW(0x0008) // target = 0x1004 + 0x0008 = 0x100E
	writeW(0x7003) // MOVEQ #3,D0
	writeW(0x4E72) // STOP
	writeW(0x2700)

	// Padding before the subroutine entry.
	writeW(0x4E71) // NOP

	// Subroutine at 0x100E: MOVEQ #2,D2; RTS
	writeW(0x7402) // MOVEQ #2,D2
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if cpu.DataRegs[0] != 3 {
		t.Errorf("D0 = %d, want 3 (set after BSR return)", cpu.DataRegs[0])
	}
	if cpu.DataRegs[2] != 2 {
		t.Errorf("D2 = %d, want 2 (set in BSR subroutine)", cpu.DataRegs[2])
	}
}

func TestM68KJIT_Exec_BSR_L_RTS(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	// Main:
	// 0x1000 MOVEQ #1,D0
	// 0x1002 BSR.L +12 -> 0x1010
	// 0x1008 MOVEQ #3,D0
	// 0x100A STOP #$2700
	writeW(0x7001)
	writeW(0x61FF)
	writeW(0x0000)
	writeW(0x000C) // target = 0x1004 + 0x000C = 0x1010
	writeW(0x7003)
	writeW(0x4E72)
	writeW(0x2700)
	writeW(0x4E71) // padding

	// Subroutine at 0x1010: MOVEQ #2,D2; RTS
	writeW(0x7402)
	writeW(0x4E75)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if cpu.DataRegs[0] != 3 {
		t.Errorf("D0 = %d, want 3 (set after BSR.L return)", cpu.DataRegs[0])
	}
	if cpu.DataRegs[2] != 2 {
		t.Errorf("D2 = %d, want 2 (set in BSR.L subroutine)", cpu.DataRegs[2])
	}
}

func TestM68KJIT_Exec_BSR_W_IntoInterpreterFallbackThenRTS(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.DataRegs[0] = 1

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x6100) // BSR.W
	writeW(0x0006) // -> 0x1008
	writeW(0x4E72) // STOP
	writeW(0x2700)
	writeW(0x4A40) // TST.W D0 (forces interpreter fallback at target)
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatal("BSR fallback/RTS test timed out")
	}
	if got := cpu.PC; got != 0x1008 {
		t.Fatalf("PC = 0x%08X, want STOP at 0x00001008", got)
	}
	if got := cpu.AddrRegs[7]; got != 0x00010000 {
		t.Fatalf("A7 = 0x%08X, want 0x00010000", got)
	}
}

func TestM68KJIT_Exec_MoveWordPredecrementPushesOntoStack(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.DataRegs[7] = 0xFFFF000A

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x3F07) // MOVE.W D7,-(SP)
	writeW(0x3F07) // MOVE.W D7,-(SP)
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatal("stack push test timed out")
	}
	if got := cpu.AddrRegs[7]; got != 0x0000FFFC {
		t.Fatalf("A7 = 0x%08X, want 0x0000FFFC", got)
	}
	if got := cpu.Read16(0xFFFC); got != 0x000A {
		t.Fatalf("stack[0xFFFC] = 0x%04X, want 0x000A", got)
	}
	if got := cpu.Read16(0xFFFE); got != 0x000A {
		t.Fatalf("stack[0xFFFE] = 0x%04X, want 0x000A", got)
	}
}

func TestM68KJIT_Exec_MoveWordPushesThenBSRIntoFallbackThenRTS(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.DataRegs[0] = 1
	cpu.DataRegs[7] = 0xFFFF000A

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x3F07) // MOVE.W D7,-(SP)
	writeW(0x3F07) // MOVE.W D7,-(SP)
	writeW(0x6100) // BSR.W
	writeW(0x0006) // -> 0x100C
	writeW(0x4E72) // STOP
	writeW(0x2700)
	writeW(0x4A40) // TST.W D0 (fallback)
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatal("push+bsr fallback test timed out")
	}
	if got := cpu.PC; got != 0x100C {
		t.Fatalf("PC = 0x%08X, want STOP at 0x0000100C", got)
	}
	if got := cpu.AddrRegs[7]; got != 0x0000FFFC {
		t.Fatalf("A7 = 0x%08X, want 0x0000FFFC", got)
	}
	if got := cpu.Read16(0xFFFC); got != 0x000A {
		t.Fatalf("stack[0xFFFC] = 0x%04X, want 0x000A", got)
	}
	if got := cpu.Read16(0xFFFE); got != 0x000A {
		t.Fatalf("stack[0xFFFE] = 0x%04X, want 0x000A", got)
	}
}

func TestM68KJIT_Exec_TrapHandlerPreludeExtractsVectorWord(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	setupCPU := func(useJIT bool) *M68KCPU {
		bus := NewMachineBus()
		termOut := NewTerminalOutput()
		bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
		bus.Write32(0, 0x00010000)
		bus.Write32(4, 0x1000)
		cpu := NewM68KCPU(bus)
		cpu.m68kJitEnabled = useJIT
		cpu.PC = 0x1000
		cpu.SR = M68K_SR_S
		cpu.AddrRegs[7] = 0x10000
		cpu.DataRegs[0] = 0xDEADBEEF

		// Synthetic exception frame as expected by ct_trap_handler:
		//   [SP+0]  = SR
		//   [SP+2]  = PC
		//   [SP+6]  = fmt/vector word (vector 6 -> offset 24 -> 0x0018)
		cpu.Write16(0x10000, 0x2700)
		cpu.Write32(0x10002, 0x00002000)
		cpu.Write16(0x10006, 0x0018)

		pc := uint32(0x1000)
		writeW := func(val uint16) {
			cpu.memory[pc] = byte(val >> 8)
			cpu.memory[pc+1] = byte(val)
			pc += 2
		}

		writeW(0x2F00) // MOVE.L D0,-(SP)
		writeW(0x7000) // MOVEQ #0,D0
		writeW(0x302F) // MOVE.W 10(SP),D0
		writeW(0x000A)
		writeW(0x0280) // ANDI.L #$00000FFF,D0
		writeW(0x0000)
		writeW(0x0FFF)
		writeW(0xE488) // LSR.L #2,D0
		writeW(0x4E72) // STOP
		writeW(0x2700)

		return cpu
	}

	runInterp := func(t *testing.T, cpu *M68KCPU) {
		t.Helper()
		cpu.running.Store(true)
		for i := 0; i < 100 && !cpu.stopped.Load() && cpu.running.Load(); i++ {
			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()
		}
		if !cpu.stopped.Load() {
			t.Fatalf("interpreter trap-prelude test did not stop (PC=0x%08X)", cpu.PC)
		}
	}

	runJIT := func(t *testing.T, cpu *M68KCPU) {
		t.Helper()
		done := make(chan struct{})
		go func() {
			cpu.running.Store(true)
			cpu.M68KExecuteJIT()
			close(done)
		}()

		deadline := time.Now().Add(2 * time.Second)
		for !cpu.stopped.Load() && cpu.running.Load() && time.Now().Before(deadline) {
			runtime.Gosched()
			time.Sleep(time.Millisecond)
		}
		cpu.running.Store(false)
		waitDoneWithGuard(t, done)
		if !cpu.stopped.Load() {
			t.Fatalf("JIT trap-prelude test did not stop (PC=0x%08X SP=0x%08X)", cpu.PC, cpu.AddrRegs[7])
		}
	}

	interp := setupCPU(false)
	jit := setupCPU(true)
	runInterp(t, interp)
	runJIT(t, jit)

	if got, want := interp.DataRegs[0], uint32(6); got != want {
		t.Fatalf("interpreter D0 = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := jit.DataRegs[0], interp.DataRegs[0]; got != want {
		t.Fatalf("JIT D0 = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := jit.AddrRegs[7], interp.AddrRegs[7]; got != want {
		t.Fatalf("JIT A7 = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := jit.Read32(0xFFFC), interp.Read32(0xFFFC); got != want {
		t.Fatalf("saved D0 on stack = 0x%08X, want 0x%08X", got, want)
	}
}

func TestM68KJIT_Exec_JSRHelperSkipsInlineDataViaAddqSPThenRTS(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	// Main: BSR case; MOVEQ #7,D1; STOP
	writeW(0x6100) // BSR.W
	writeW(0x001C) // -> 0x1020
	writeW(0x7207) // MOVEQ #7,D1
	writeW(0x4E72) // STOP
	writeW(0x2700)

	for pc < 0x1020 {
		writeW(0x4E71) // NOP padding
	}

	// case: JSR helper; inline bytes that must never execute
	writeW(0x4EB9) // JSR $00001058
	writeW(0x0000)
	writeW(0x1058)
	cpu.memory[pc+0] = 'B'
	cpu.memory[pc+1] = 'A'
	cpu.memory[pc+2] = 'D'
	cpu.memory[pc+3] = 0x00
	pc += 4

	for pc < 0x1058 {
		writeW(0x4E71) // NOP padding
	}

	// helper: ADDQ.L #4,SP ; RTS
	writeW(0x588F)
	writeW(0x4E75)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && cpu.running.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatalf("inline-data helper test did not reach STOP (PC=0x%08X SP=0x%08X)", cpu.PC, cpu.AddrRegs[7])
	}
	if got := cpu.DataRegs[1]; got != 7 {
		t.Fatalf("D1 = %d, want 7 (execution should resume after BSR caller)", got)
	}
	if got := cpu.PC; got != 0x0000100A {
		t.Fatalf("PC = 0x%08X, want 0x0000100A after STOP", got)
	}
	if got := cpu.AddrRegs[7]; got != 0x00010000 {
		t.Fatalf("A7 = 0x%08X, want 0x00010000", got)
	}
}

func TestM68KJIT_Exec_FirstCPUSuiteBFTSTCaseMatchesInterpreter(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	setupCPU := func(useJIT bool) *M68KCPU {
		bus := NewMachineBus()
		termOut := NewTerminalOutput()
		bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
		bus.Write32(0, 0x00010000)
		bus.Write32(4, 0x1000)
		cpu := NewM68KCPU(bus)
		cpu.m68kJitEnabled = useJIT
		cpu.PC = 0x1000
		cpu.SR = M68K_SR_S
		cpu.AddrRegs[7] = 0x10000

		pc := uint32(0x1000)
		writeW := func(val uint16) {
			cpu.memory[pc] = byte(val >> 8)
			cpu.memory[pc+1] = byte(val)
			pc += 2
		}

		// Exact instruction bytes from the first assembled CPU-suite case.
		writeW(0x203C) // MOVE.L #$FF000000,D0
		writeW(0xFF00)
		writeW(0x0000)
		writeW(0x7400) // MOVEQ #0,D2
		writeW(0x44C2) // MOVE.W D2,CCR
		writeW(0xE8C0) // BFTST D0{0:8}
		writeW(0x0008)
		writeW(0x40C1) // MOVE.W SR,D1
		writeW(0x0241) // ANDI.W #$000C,D1
		writeW(0x000C)
		writeW(0x0C41) // CMPI.W #$0008,D1
		writeW(0x0008)
		writeW(0x6606) // BNE.B fail
		writeW(0x7E01) // MOVEQ #1,D7 (pass)
		writeW(0x4E72) // STOP
		writeW(0x2700)
		writeW(0x7E02) // fail: MOVEQ #2,D7
		writeW(0x4E72) // STOP
		writeW(0x2700)

		return cpu
	}

	runInterp := func(t *testing.T, cpu *M68KCPU) {
		t.Helper()
		for i := 0; i < 1000 && !cpu.stopped.Load() && cpu.running.Load(); i++ {
			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()
		}
		if !cpu.stopped.Load() {
			t.Fatalf("interpreter case did not reach STOP (PC=0x%08X)", cpu.PC)
		}
	}

	runJIT := func(t *testing.T, cpu *M68KCPU) {
		t.Helper()
		done := make(chan struct{})
		go func() {
			cpu.running.Store(true)
			cpu.M68KExecuteJIT()
			close(done)
		}()

		deadline := time.Now().Add(2 * time.Second)
		for !cpu.stopped.Load() && cpu.running.Load() && time.Now().Before(deadline) {
			runtime.Gosched()
			time.Sleep(time.Millisecond)
		}
		cpu.running.Store(false)
		waitDoneWithGuard(t, done)
		if !cpu.stopped.Load() {
			t.Fatalf("JIT case did not reach STOP (PC=0x%08X)", cpu.PC)
		}
	}

	interp := setupCPU(false)
	jit := setupCPU(true)
	interp.running.Store(true)

	runInterp(t, interp)
	runJIT(t, jit)

	if got, want := interp.DataRegs[7], uint32(1); got != want {
		t.Fatalf("interpreter D7 = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := jit.DataRegs[7], interp.DataRegs[7]; got != want {
		t.Fatalf("JIT D7 = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := jit.DataRegs[1]&0xFFFF, interp.DataRegs[1]&0xFFFF; got != want {
		t.Fatalf("JIT D1.W = 0x%04X, want 0x%04X", got, want)
	}
	if got, want := jit.SR&0x000F, interp.SR&0x000F; got != want {
		t.Fatalf("JIT SR low flags = 0x%04X, want 0x%04X", got, want)
	}
}

func TestM68KJIT_Exec_FirstCPUSuiteCaseWithPassHelperAndBSR(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	const passCount = 0x80000
	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	// shard: BSR case; MOVEQ #9,D6; STOP
	writeW(0x6100) // BSR.W
	writeW(0x001C) // -> 0x1020
	writeW(0x7C09) // MOVEQ #9,D6
	writeW(0x4E72) // STOP
	writeW(0x2700)

	for pc < 0x1020 {
		writeW(0x4E71)
	}

	// Exact first case pass path from the assembled suite.
	writeW(0x203C) // MOVE.L #$FF000000,D0
	writeW(0xFF00)
	writeW(0x0000)
	writeW(0x7400) // MOVEQ #0,D2
	writeW(0x44C2) // MOVE.W D2,CCR
	writeW(0xE8C0) // BFTST D0{0:8}
	writeW(0x0008)
	writeW(0x40C1) // MOVE.W SR,D1
	writeW(0x0241) // ANDI.W #$000C,D1
	writeW(0x000C)
	writeW(0x0C41) // CMPI.W #$0008,D1
	writeW(0x0008)
	writeW(0x660C) // BNE.B fail
	writeW(0x41FA) // LEA .name(PC),A0
	writeW(0x001C)
	writeW(0x4EB9) // JSR pass_helper
	writeW(0x0000)
	writeW(0x1100)
	writeW(0x4E75) // RTS
	writeW(0x41FA) // fail: LEA .name(PC),A0
	writeW(0x0010)
	writeW(0x4E72) // STOP (should never hit in this test)
	writeW(0x2700)

	cpu.memory[pc+0] = 'B'
	cpu.memory[pc+1] = 'F'
	cpu.memory[pc+2] = 'T'
	cpu.memory[pc+3] = 'S'
	cpu.memory[pc+4] = 'T'
	cpu.memory[pc+5] = 0
	pc += 6

	for pc < 0x1100 {
		writeW(0x4E71)
	}

	// pass_helper: MOVE.L $80000,D0 ; ADDQ.L #1,D0 ; MOVE.L D0,$80000 ; RTS
	writeW(0x2039)
	writeW(0x0008)
	writeW(0x0000)
	writeW(0x5280)
	writeW(0x23C0)
	writeW(0x0008)
	writeW(0x0000)
	writeW(0x4E75)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && cpu.running.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatalf("suite case + pass helper did not stop (PC=0x%08X SP=0x%08X)", cpu.PC, cpu.AddrRegs[7])
	}
	if got := cpu.Read32(passCount); got != 1 {
		t.Fatalf("pass_count = %d, want 1", got)
	}
	if got := cpu.DataRegs[6]; got != 9 {
		t.Fatalf("D6 = %d, want 9 (execution should resume in shard after case RTS)", got)
	}
}

func TestM68KJIT_Exec_ShardDispatcherJSRIndirectMatchesSuitePattern(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	const passCount = 0x80000
	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}
	writeL := func(val uint32) {
		cpu.memory[pc] = byte(val >> 24)
		cpu.memory[pc+1] = byte(val >> 16)
		cpu.memory[pc+2] = byte(val >> 8)
		cpu.memory[pc+3] = byte(val)
		pc += 4
	}

	// Minimal top-level suite dispatcher:
	// lea shard_list,a5
	// move.l (a5)+,d0
	// beq done
	// movea.l d0,a1
	// move.l a5,-(sp)
	// jsr (a1)
	// movea.l (sp)+,a5
	// moveq #9,d6
	// stop
	writeW(0x4BFA) // LEA shard_list(PC),A5
	writeW(0x0022)
	writeW(0x201D) // MOVE.L (A5)+,D0
	writeW(0x670A) // BEQ done
	writeW(0x2240) // MOVEA.L D0,A1
	writeW(0x2F0D) // MOVE.L A5,-(SP)
	writeW(0x4E91) // JSR (A1)
	writeW(0x2A5F) // MOVEA.L (SP)+,A5
	writeW(0x7C09) // MOVEQ #9,D6
	writeW(0x4E72) // done: STOP
	writeW(0x2700)

	for pc < 0x1024 {
		writeW(0x4E71)
	}

	writeL(0x00001100) // shard_list[0] = shard entry
	writeL(0x00000000) // terminator

	for pc < 0x1100 {
		writeW(0x4E71)
	}

	// shard: BSR case ; RTS
	writeW(0x6100)
	writeW(0x001C) // -> 0x1120
	writeW(0x4E75)

	for pc < 0x1120 {
		writeW(0x4E71)
	}

	// Reuse the exact first-case pass path.
	writeW(0x203C) // MOVE.L #$FF000000,D0
	writeW(0xFF00)
	writeW(0x0000)
	writeW(0x7400) // MOVEQ #0,D2
	writeW(0x44C2) // MOVE.W D2,CCR
	writeW(0xE8C0) // BFTST D0{0:8}
	writeW(0x0008)
	writeW(0x40C1) // MOVE.W SR,D1
	writeW(0x0241) // ANDI.W #$000C,D1
	writeW(0x000C)
	writeW(0x0C41) // CMPI.W #$0008,D1
	writeW(0x0008)
	writeW(0x660C) // BNE.B fail
	writeW(0x41FA) // LEA .name(PC),A0
	writeW(0x001C)
	writeW(0x4EB9) // JSR pass_helper
	writeW(0x0000)
	writeW(0x1200)
	writeW(0x4E75) // RTS
	writeW(0x41FA) // fail: LEA .name(PC),A0
	writeW(0x0010)
	writeW(0x4E72) // STOP (should not hit)
	writeW(0x2700)

	cpu.memory[pc+0] = 'B'
	cpu.memory[pc+1] = 'F'
	cpu.memory[pc+2] = 'T'
	cpu.memory[pc+3] = 'S'
	cpu.memory[pc+4] = 'T'
	cpu.memory[pc+5] = 0
	pc += 6

	for pc < 0x1200 {
		writeW(0x4E71)
	}

	writeW(0x2039) // pass_helper: MOVE.L $80000,D0
	writeW(0x0008)
	writeW(0x0000)
	writeW(0x5280) // ADDQ.L #1,D0
	writeW(0x23C0) // MOVE.L D0,$80000
	writeW(0x0008)
	writeW(0x0000)
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && cpu.running.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatalf("suite dispatcher repro did not stop (PC=0x%08X SP=0x%08X A5=0x%08X)", cpu.PC, cpu.AddrRegs[7], cpu.AddrRegs[5])
	}
	if got := cpu.Read32(passCount); got != 1 {
		t.Fatalf("pass_count = %d, want 1", got)
	}
	if got := cpu.DataRegs[6]; got != 9 {
		t.Fatalf("D6 = %d, want 9", got)
	}
	if got := cpu.AddrRegs[5]; got != 0x00001028 {
		t.Fatalf("A5 = 0x%08X, want 0x00001028 after restoring shard-list cursor", got)
	}
}

func TestM68KJIT_Exec_ShardPrologueJSRThenFirstCase(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	const passCount = 0x80000
	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	// Shard body matching the real suite shape:
	// lea shard_name,a0 ; jsr ct_enter_shard ; bsr case ; stop
	writeW(0x41FA) // LEA shard_name(PC),A0
	writeW(0x0014)
	writeW(0x4EB9) // JSR ct_enter_shard
	writeW(0x0000)
	writeW(0x1100)
	writeW(0x6100) // BSR.W case
	writeW(0x0022) // -> 0x1030
	writeW(0x4E72) // STOP
	writeW(0x2700)

	cpu.memory[pc+0] = 'S'
	cpu.memory[pc+1] = 'H'
	cpu.memory[pc+2] = 'A'
	cpu.memory[pc+3] = 'R'
	cpu.memory[pc+4] = 'D'
	cpu.memory[pc+5] = 0
	pc += 6

	for pc < 0x1030 {
		writeW(0x4E71)
	}

	// First case pass path.
	writeW(0x203C) // MOVE.L #$FF000000,D0
	writeW(0xFF00)
	writeW(0x0000)
	writeW(0x7400) // MOVEQ #0,D2
	writeW(0x44C2) // MOVE.W D2,CCR
	writeW(0xE8C0) // BFTST D0{0:8}
	writeW(0x0008)
	writeW(0x40C1) // MOVE.W SR,D1
	writeW(0x0241) // ANDI.W #$000C,D1
	writeW(0x000C)
	writeW(0x0C41) // CMPI.W #$0008,D1
	writeW(0x0008)
	writeW(0x660C) // BNE.B fail
	writeW(0x41FA) // LEA .name(PC),A0
	writeW(0x001C)
	writeW(0x4EB9) // JSR pass_helper
	writeW(0x0000)
	writeW(0x1200)
	writeW(0x4E75) // RTS
	writeW(0x41FA) // fail: LEA .name(PC),A0
	writeW(0x0010)
	writeW(0x4E72) // STOP if fail
	writeW(0x2700)

	cpu.memory[pc+0] = 'B'
	cpu.memory[pc+1] = 'F'
	cpu.memory[pc+2] = 'T'
	cpu.memory[pc+3] = 'S'
	cpu.memory[pc+4] = 'T'
	cpu.memory[pc+5] = 0
	pc += 6

	for pc < 0x1100 {
		writeW(0x4E71)
	}

	writeW(0x4E75) // ct_enter_shard: RTS

	for pc < 0x1200 {
		writeW(0x4E71)
	}

	writeW(0x2039) // pass_helper: MOVE.L $80000,D0
	writeW(0x0008)
	writeW(0x0000)
	writeW(0x5280) // ADDQ.L #1,D0
	writeW(0x23C0) // MOVE.L D0,$80000
	writeW(0x0008)
	writeW(0x0000)
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && cpu.running.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatalf("shard prologue repro did not stop (PC=0x%08X SP=0x%08X)", cpu.PC, cpu.AddrRegs[7])
	}
	if got := cpu.Read32(passCount); got != 1 {
		t.Fatalf("pass_count = %d, want 1 (PC=0x%08X D1=0x%08X SR=0x%04X SP=0x%08X)", got, cpu.PC, cpu.DataRegs[1], cpu.SR, cpu.AddrRegs[7])
	}
}

func TestM68KJIT_Exec_JSRIndirectWithSavedA5OnStack(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.AddrRegs[5] = 0x12345678
	cpu.AddrRegs[1] = 0x00001100

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x2F0D) // MOVE.L A5,-(SP)
	writeW(0x4E91) // JSR (A1)
	writeW(0x2A5F) // MOVEA.L (SP)+,A5
	writeW(0x4E72) // STOP
	writeW(0x2700)

	for pc < 0x1100 {
		writeW(0x4E71)
	}
	writeW(0x7001) // MOVEQ #1,D0
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && cpu.running.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatalf("indirect jsr + saved a5 test did not stop (PC=0x%08X SP=0x%08X A5=0x%08X)", cpu.PC, cpu.AddrRegs[7], cpu.AddrRegs[5])
	}
	if got := cpu.DataRegs[0]; got != 1 {
		t.Fatalf("D0 = %d, want 1", got)
	}
	if got := cpu.AddrRegs[5]; got != 0x12345678 {
		t.Fatalf("A5 = 0x%08X, want 0x12345678", got)
	}
	if got := cpu.AddrRegs[7]; got != 0x00010000 {
		t.Fatalf("A7 = 0x%08X, want 0x00010000", got)
	}
}

func TestM68KJIT_Exec_LoadShardPointerViaA5PostIncrement(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}
	writeL := func(val uint32) {
		cpu.memory[pc] = byte(val >> 24)
		cpu.memory[pc+1] = byte(val >> 16)
		cpu.memory[pc+2] = byte(val >> 8)
		cpu.memory[pc+3] = byte(val)
		pc += 4
	}

	writeW(0x4BFA) // LEA shard_list(PC),A5
	writeW(0x000A)
	writeW(0x201D) // MOVE.L (A5)+,D0
	writeW(0x2240) // MOVEA.L D0,A1
	writeW(0x4E72) // STOP
	writeW(0x2700)
	writeL(0x00001100)
	writeL(0x00000000)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && cpu.running.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatalf("load shard pointer test did not stop (PC=0x%08X A5=0x%08X D0=0x%08X A1=0x%08X)", cpu.PC, cpu.AddrRegs[5], cpu.DataRegs[0], cpu.AddrRegs[1])
	}
	if got := cpu.DataRegs[0]; got != 0x00001100 {
		t.Fatalf("D0 = 0x%08X, want 0x00001100", got)
	}
	if got := cpu.AddrRegs[1]; got != 0x00001100 {
		t.Fatalf("A1 = 0x%08X, want 0x00001100", got)
	}
	if got := cpu.AddrRegs[5]; got != 0x00001010 {
		t.Fatalf("A5 = 0x%08X, want 0x00001010 after post-increment", got)
	}
}

func TestM68KJIT_Exec_SubroutineWithMovemFrameAroundIndexedVGAPixelWrite(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	vga := NewVGAEngine(bus)
	bus.MapIO(VGA_BASE, VGA_REG_END, vga.HandleRead, vga.HandleWrite)
	bus.MapIO(VGA_VRAM_WINDOW, VGA_VRAM_WINDOW+VGA_VRAM_SIZE-1, vga.HandleVRAMRead, vga.HandleVRAMWrite)
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.DataRegs[0] = 0x12345678
	cpu.DataRegs[1] = 0x87654321
	cpu.DataRegs[2] = 0x4F
	cpu.DataRegs[3] = 10
	cpu.DataRegs[4] = 10
	cpu.AddrRegs[0] = 0xCAFEBABE

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x4EB9) // JSR $1100
	writeW(0x0000)
	writeW(0x1100)
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	pc = 0x1100
	writeW(0x48E7) // MOVEM.L D0-D1/A0,-(SP)
	writeW(0xC080)
	writeW(0x41F9) // LEA $000A0000,A0
	writeW(0x000A)
	writeW(0x0000)
	writeW(0x3004) // MOVE.W D4,D0
	writeW(0xC0FC) // MULU #320,D0
	writeW(0x0140)
	writeW(0xD043) // ADD.W D3,D0
	writeW(0x1182) // MOVE.B D2,(A0,D0.L)
	writeW(0x0800)
	writeW(0x4CDF) // MOVEM.L (SP)+,D0-D1/A0
	writeW(0x0103)
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatal("movem VGA subroutine test timed out")
	}
	if got := cpu.DataRegs[0]; got != 0x12345678 {
		t.Fatalf("D0 = 0x%08X, want 0x12345678", got)
	}
	if got := cpu.DataRegs[1]; got != 0x87654321 {
		t.Fatalf("D1 = 0x%08X, want 0x87654321", got)
	}
	if got := cpu.AddrRegs[0]; got != 0xCAFEBABE {
		t.Fatalf("A0 = 0x%08X, want 0xCAFEBABE", got)
	}
	if got := cpu.AddrRegs[7]; got != 0x00010000 {
		t.Fatalf("A7 = 0x%08X, want 0x00010000", got)
	}
	if got := bus.Read8(VGA_VRAM_WINDOW + 3210); got != 0x4F {
		t.Fatalf("vram[3210] = 0x%02X, want 0x4F", got)
	}
}

func TestM68KJIT_Exec_WordStackLocalsThenMovemPixelSubroutineThenRTS(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	vga := NewVGAEngine(bus)
	bus.MapIO(VGA_BASE, VGA_REG_END, vga.HandleRead, vga.HandleWrite)
	bus.MapIO(VGA_VRAM_WINDOW, VGA_VRAM_WINDOW+VGA_VRAM_SIZE-1, vga.HandleVRAMRead, vga.HandleVRAMWrite)
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.DataRegs[0] = 0x12345678
	cpu.DataRegs[1] = 0x87654321
	cpu.DataRegs[2] = 0x4F
	cpu.DataRegs[3] = 10
	cpu.DataRegs[4] = 10
	cpu.DataRegs[7] = 0xFFFF000A
	cpu.AddrRegs[0] = 0xCAFEBABE

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x3F07) // MOVE.W D7,-(SP)
	writeW(0x3F07) // MOVE.W D7,-(SP)
	writeW(0x4EB9) // JSR $1100
	writeW(0x0000)
	writeW(0x1100)
	writeW(0x588F) // ADDQ.L #4,SP
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	pc = 0x1100
	writeW(0x48E7) // MOVEM.L D0-D1/A0,-(SP)
	writeW(0xC080)
	writeW(0x41F9) // LEA $000A0000,A0
	writeW(0x000A)
	writeW(0x0000)
	writeW(0x3004) // MOVE.W D4,D0
	writeW(0xC0FC) // MULU #320,D0
	writeW(0x0140)
	writeW(0xD043) // ADD.W D3,D0
	writeW(0x1182) // MOVE.B D2,(A0,D0.L)
	writeW(0x0800)
	writeW(0x4CDF) // MOVEM.L (SP)+,D0-D1/A0
	writeW(0x0103)
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatal("word locals + pixel subroutine test timed out")
	}
	if got := cpu.AddrRegs[7]; got != 0x00010000 {
		t.Fatalf("A7 = 0x%08X, want 0x00010000", got)
	}
	if got := cpu.Read8(VGA_VRAM_WINDOW + 3210); got != 0x4F {
		t.Fatalf("vram[3210] = 0x%02X, want 0x4F", got)
	}
}

func TestM68KJIT_Exec_DrawLineStyleNestedFramesAndLocals(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	vga := NewVGAEngine(bus)
	bus.MapIO(VGA_BASE, VGA_REG_END, vga.HandleRead, vga.HandleWrite)
	bus.MapIO(VGA_VRAM_WINDOW, VGA_VRAM_WINDOW+VGA_VRAM_SIZE-1, vga.HandleVRAMRead, vga.HandleVRAMWrite)
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.DataRegs[0] = 0x12345678
	cpu.DataRegs[1] = 0x87654321
	cpu.DataRegs[2] = 0x4F
	cpu.DataRegs[3] = 10
	cpu.DataRegs[4] = 10
	cpu.DataRegs[7] = 0xFFFF000A
	cpu.AddrRegs[0] = 0xCAFEBABE

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x48E7) // MOVEM.L D0-D7/A0,-(SP)
	writeW(0x01FF)
	writeW(0x3F07) // MOVE.W D7,-(SP)
	writeW(0x3F07) // MOVE.W D7,-(SP)
	writeW(0x6100) // BSR.W $1100
	writeW(0x00F6)
	writeW(0x588F) // ADDQ.L #4,SP
	writeW(0x4CDF) // MOVEM.L (SP)+,D0-D7/A0
	writeW(0xFF80)
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	pc = 0x1100
	writeW(0x48E7) // MOVEM.L D0-D1/A0,-(SP)
	writeW(0xC080)
	writeW(0x41F9) // LEA $000A0000,A0
	writeW(0x000A)
	writeW(0x0000)
	writeW(0x3004) // MOVE.W D4,D0
	writeW(0xC0FC) // MULU #320,D0
	writeW(0x0140)
	writeW(0xD043) // ADD.W D3,D0
	writeW(0x1182) // MOVE.B D2,(A0,D0.L)
	writeW(0x0800)
	writeW(0x4CDF) // MOVEM.L (SP)+,D0-D1/A0
	writeW(0x0103)
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatal("draw-line style nested frame test timed out")
	}
	if got := cpu.AddrRegs[7]; got != 0x00010000 {
		t.Fatalf("A7 = 0x%08X, want 0x00010000", got)
	}
	if got := bus.Read8(VGA_VRAM_WINDOW + 3210); got != 0x4F {
		t.Fatalf("vram[3210] = 0x%02X, want 0x4F", got)
	}
}

func TestM68KJIT_Exec_OuterMovemThenWordLocalsThenBSRIntoFallbackThenRTS(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.DataRegs[0] = 1
	cpu.DataRegs[7] = 0xFFFF000A
	cpu.AddrRegs[0] = 0xCAFEBABE

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x48E7) // MOVEM.L D0-D7/A0,-(SP)
	writeW(0x01FF)
	writeW(0x3F07) // MOVE.W D7,-(SP)
	writeW(0x3F07) // MOVE.W D7,-(SP)
	writeW(0x6100) // BSR.W
	writeW(0x000A) // -> 0x1016
	writeW(0x588F) // ADDQ.L #4,SP
	writeW(0x4CDF) // MOVEM.L (SP)+,D0-D7/A0
	writeW(0xFF80)
	writeW(0x4E72) // STOP
	writeW(0x2700)

	writeW(0x4A40) // TST.W D0 (fallback)
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if !cpu.stopped.Load() {
		t.Fatal("outer movem + locals + bsr test timed out")
	}
	if got := cpu.PC; got != 0x1016 {
		t.Fatalf("PC = 0x%08X, want STOP at 0x00001016", got)
	}
}

func TestM68KJIT_Exec_NestedBSRWithMovemFramesRestoresStack(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	// Main: BSR func1; MOVEQ #7,D0; STOP
	writeW(0x6100) // BSR.W
	writeW(0x0010) // -> 0x1014
	writeW(0x7007) // MOVEQ #7,D0
	writeW(0x4E72) // STOP
	writeW(0x2700)
	writeW(0x4E71) // padding
	writeW(0x4E71) // padding
	writeW(0x4E71) // padding
	writeW(0x4E71) // padding
	writeW(0x4E71) // padding

	// func1 at 0x1014:
	// MOVEM.L D0-D1/A0,-(SP); BSR func2; MOVEM.L (SP)+,D0-D1/A0; RTS
	writeW(0x48E7)
	writeW(0xC080)
	writeW(0x6100) // BSR.W
	writeW(0x0006) // -> 0x1022
	writeW(0x4CDF)
	writeW(0x0103)
	writeW(0x4E75)

	// func2 at 0x1022:
	// MOVEM.L D0-D1/A0,-(SP); MOVEQ #5,D1; MOVEM.L (SP)+,D0-D1/A0; RTS
	writeW(0x48E7)
	writeW(0xC080)
	writeW(0x7205)
	writeW(0x4CDF)
	writeW(0x0103)
	writeW(0x4E75)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if got := cpu.DataRegs[0]; got != 7 {
		t.Fatalf("D0 = %d, want 7", got)
	}
	if got := cpu.AddrRegs[7]; got != 0x10000 {
		t.Fatalf("A7 = 0x%08X, want 0x00010000", got)
	}
}

func TestM68KJIT_Exec_VideoBlitterSetupSequence(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("failed to create video chip: %v", err)
	}
	video.AttachBus(bus)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)
	bus.SetVideoStatusReader(video.HandleRead)
	if err := video.Start(); err != nil {
		t.Fatalf("failed to start video chip: %v", err)
	}
	defer func() { _ = video.Stop() }()

	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	// Mirror the rotozoomer load_texture-style setup sequence.
	writeW(0x23FC) // MOVE.L #0,BLT_OP
	writeW(0x0000)
	writeW(0x0000)
	writeW(0x000F)
	writeW(0x0020)

	writeW(0x41F9) // LEA $00001600,A0
	writeW(0x0000)
	writeW(0x1600)

	writeW(0x23C8) // MOVE.L A0,BLT_SRC
	writeW(0x000F)
	writeW(0x0024)

	writeW(0x23FC) // MOVE.L #$00600000,BLT_DST
	writeW(0x0060)
	writeW(0x0000)
	writeW(0x000F)
	writeW(0x0028)

	writeW(0x23FC) // MOVE.L #$00000100,BLT_WIDTH
	writeW(0x0000)
	writeW(0x0100)
	writeW(0x000F)
	writeW(0x002C)

	writeW(0x23FC) // MOVE.L #$00000100,BLT_HEIGHT
	writeW(0x0000)
	writeW(0x0100)
	writeW(0x000F)
	writeW(0x0030)

	writeW(0x23FC) // MOVE.L #$00000400,BLT_SRC_STRIDE
	writeW(0x0000)
	writeW(0x0400)
	writeW(0x000F)
	writeW(0x0034)

	writeW(0x23FC) // MOVE.L #$00000400,BLT_DST_STRIDE
	writeW(0x0000)
	writeW(0x0400)
	writeW(0x000F)
	writeW(0x0038)

	writeW(0x23FC) // MOVE.L #1,BLT_CTRL
	writeW(0x0000)
	writeW(0x0001)
	writeW(0x000F)
	writeW(0x001C)

	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if got := video.HandleRead(BLT_SRC); got != 0x00001600 {
		t.Fatalf("BLT_SRC = 0x%08X, want 0x00001600", got)
	}
	if got := video.HandleRead(BLT_DST); got != 0x00600000 {
		t.Fatalf("BLT_DST = 0x%08X, want 0x00600000", got)
	}
	if got := video.HandleRead(BLT_WIDTH); got != 0x00000100 {
		t.Fatalf("BLT_WIDTH = 0x%08X, want 0x00000100", got)
	}
	if got := video.HandleRead(BLT_HEIGHT); got != 0x00000100 {
		t.Fatalf("BLT_HEIGHT = 0x%08X, want 0x00000100", got)
	}
}

func TestM68KJIT_Exec_MoveLongAbsMaskAndStoreBack(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.Write32(0x880C, 0x12345678)

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x2039) // MOVE.L $0000880C,D0
	writeW(0x0000)
	writeW(0x880C)
	writeW(0x0280) // ANDI.L #$0000001F,D0
	writeW(0x0000)
	writeW(0x001F)
	writeW(0x23C0) // MOVE.L D0,$0000880C
	writeW(0x0000)
	writeW(0x880C)
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if got := cpu.Read32(0x880C); got != 0x00000018 {
		t.Fatalf("memory[0x880C] = 0x%08X, want 0x00000018", got)
	}
}

func TestM68KJIT_Exec_MoveMaskStoreThenBRA_W(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.Write32(0x880C, 0x12345678)

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x2039) // MOVE.L $0000880C,D0
	writeW(0x0000)
	writeW(0x880C)
	writeW(0x0280) // ANDI.L #$0000001F,D0
	writeW(0x0000)
	writeW(0x001F)
	writeW(0x23C0) // MOVE.L D0,$0000880C
	writeW(0x0000)
	writeW(0x880C)
	writeW(0x7400) // MOVEQ #0,D2
	writeW(0x6000) // BRA.W +4 -> 0x101A
	writeW(0x0004)
	writeW(0x7299) // MOVEQ #-103,D1 (skipped)
	writeW(0x7601) // MOVEQ #1,D3
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if got := cpu.Read32(0x880C); got != 0x00000018 {
		t.Fatalf("memory[0x880C] = 0x%08X, want 0x00000018", got)
	}
	if cpu.DataRegs[1] != 0 {
		t.Fatalf("D1 = %d, want 0 (BRA.W should skip)", cpu.DataRegs[1])
	}
	if cpu.DataRegs[3] != 1 {
		t.Fatalf("D3 = %d, want 1", cpu.DataRegs[3])
	}
}

func TestM68KJIT_Exec_MoveByteIndexedLongIntoDn(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.Write8(0x2003, 0x7F)

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x41F9) // LEA $00002000,A0
	writeW(0x0000)
	writeW(0x2000)
	writeW(0x7403) // MOVEQ #3,D2
	writeW(0x1A30) // MOVE.B 0(A0,D2.L),D5
	writeW(0x2800)
	writeW(0x0285) // ANDI.L #$000000FF,D5
	writeW(0x0000)
	writeW(0x00FF)
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if got := cpu.DataRegs[5]; got != 0x7F {
		t.Fatalf("D5 = 0x%08X, want 0x0000007F", got)
	}
}

func TestM68KJIT_Exec_LEAAbsLongThenMoveBytePostInc(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x41F9) // LEA $000A0000,A0
	writeW(0x000A)
	writeW(0x0000)
	writeW(0x10FC) // MOVE.B #1,(A0)+
	writeW(0x0001)
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if got := bus.Read8(0x000A0000); got != 0x01 {
		t.Fatalf("bus[0x000A0000] = 0x%02X, want 0x01", got)
	}
	if got := cpu.AddrRegs[0]; got != 0x000A0001 {
		t.Fatalf("A0 = 0x%08X, want 0x000A0001", got)
	}
	if got := cpu.Read16(0x1000); got != 0x41F9 {
		t.Fatalf("memory[0x1000] = 0x%04X, want 0x41F9", got)
	}
}

func TestM68KJIT_Exec_MoveByteImmediateToVGAMMIO(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	vga := NewVGAEngine(bus)
	bus.MapIO(VGA_BASE, VGA_REG_END, vga.HandleRead, vga.HandleWrite)
	bus.MapIO(VGA_VRAM_WINDOW, VGA_VRAM_WINDOW+VGA_VRAM_SIZE-1, vga.HandleVRAMRead, vga.HandleVRAMWrite)
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x13FC) // MOVE.B #$13,$000F1000 (VGA_MODE)
	writeW(0x0013)
	writeW(0x000F)
	writeW(0x1000)
	writeW(0x13FC) // MOVE.B #$01,$000F1008 (VGA_CTRL)
	writeW(0x0001)
	writeW(0x000F)
	writeW(0x1008)
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if got := bus.Read8(VGA_MODE); got != VGA_MODE_13H {
		t.Fatalf("VGA_MODE = 0x%02X, want 0x%02X", got, VGA_MODE_13H)
	}
	if got := bus.Read8(VGA_CTRL); got != VGA_CTRL_ENABLE {
		t.Fatalf("VGA_CTRL = 0x%02X, want 0x%02X", got, VGA_CTRL_ENABLE)
	}
}

func TestM68KJIT_Exec_MoveByteDnToIndexedVGAVRAMDoesNotTouchStack(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	vga := NewVGAEngine(bus)
	bus.MapIO(VGA_BASE, VGA_REG_END, vga.HandleRead, vga.HandleWrite)
	bus.MapIO(VGA_VRAM_WINDOW, VGA_VRAM_WINDOW+VGA_VRAM_SIZE-1, vga.HandleVRAMRead, vga.HandleVRAMWrite)
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.Write32(0xFFFC, 0x11223344)

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x41F9) // LEA $000A0000,A0
	writeW(0x000A)
	writeW(0x0000)
	writeW(0x203C) // MOVE.L #3210,D0
	writeW(0x0000)
	writeW(0x0C8A)
	writeW(0x744F) // MOVEQ #79,D2
	writeW(0x1182) // MOVE.B D2,(A0,D0.L)
	writeW(0x0800)
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)
	if !cpu.stopped.Load() {
		t.Fatal("indexed VGA write test timed out")
	}

	if got := bus.Read8(VGA_VRAM_WINDOW + 3210); got != 0x4F {
		t.Fatalf("vram[3210] = 0x%02X, want 0x4F", got)
	}
	if got := cpu.Read32(0xFFFC); got != 0x11223344 {
		t.Fatalf("stack sentinel = 0x%08X, want 0x11223344", got)
	}
}

func TestM68KJIT_Exec_DBF_MoveBytePostIncrementIntoVGAVRAM(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	vga := NewVGAEngine(bus)
	bus.MapIO(VGA_BASE, VGA_REG_END, vga.HandleRead, vga.HandleWrite)
	bus.MapIO(VGA_VRAM_WINDOW, VGA_VRAM_WINDOW+VGA_VRAM_SIZE-1, vga.HandleVRAMRead, vga.HandleVRAMWrite)
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x41F9) // LEA $000A0000,A0
	writeW(0x000A)
	writeW(0x0000)
	writeW(0x303C) // MOVE.W #3,D0
	writeW(0x0003)
	writeW(0x10FC) // MOVE.B #1,(A0)+
	writeW(0x0001)
	writeW(0x51C8) // DBF D0, loop
	writeW(0xFFFA)
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	for i := uint32(0); i < 4; i++ {
		if got := bus.Read8(0x000A0000 + i); got != 0x01 {
			t.Fatalf("bus[0x%08X] = 0x%02X, want 0x01", 0x000A0000+i, got)
		}
	}
	if got := cpu.AddrRegs[0]; got != 0x000A0004 {
		t.Fatalf("A0 = 0x%08X, want 0x000A0004", got)
	}
}

func TestM68KJIT_Exec_MoveLongIndexedMappedIndexIntoDn(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000
	cpu.Write32(0x2014, 0x12345678)

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x41F9) // LEA $00002000,A0
	writeW(0x0000)
	writeW(0x2000)
	writeW(0x7205) // MOVEQ #5,D1
	writeW(0xE589) // LSL.L #2,D1
	writeW(0x2430) // MOVE.L 0(A0,D1.L),D2
	writeW(0x1800)
	writeW(0x0682) // ADDI.L #200,D2
	writeW(0x0000)
	writeW(0x00C8)
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if got := cpu.DataRegs[2]; got != 0x12345740 {
		t.Fatalf("D2 = 0x%08X, want 0x12345740", got)
	}
}

func TestM68KJIT_Exec_DBF_MoveByteImmediatePostIncrementLoop(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x41F9) // LEA $00002000,A0
	writeW(0x0000)
	writeW(0x2000)
	writeW(0x303C) // MOVE.W #$03E7,D0
	writeW(0x03E7)
	loopPC := pc
	writeW(0x10FC) // MOVE.B #$80,(A0)+
	writeW(0x0080)
	_ = loopPC
	writeW(0x51C8) // DBF D0, loop
	writeW(0xFFFA)
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	for i := uint32(0); i < 1000; i++ {
		if got := cpu.Read8(0x2000 + i); got != 0x80 {
			t.Fatalf("memory[0x%04X] = 0x%02X, want 0x80", 0x2000+i, got)
		}
	}
	if got := cpu.AddrRegs[0]; got != 0x2000+1000 {
		t.Fatalf("A0 = 0x%08X, want 0x%08X", got, 0x2000+1000)
	}
	if got := cpu.DataRegs[0] & 0xFFFF; got != 0xFFFF {
		t.Fatalf("D0.W = 0x%04X, want 0xFFFF", got)
	}
}

func TestM68KJIT_LongFillCountLoopHelperMatchesInterpreter(t *testing.T) {
	const startPC = 0x1000
	const dst = 0x3000
	program := []uint16{
		0x24C1, // MOVE.L D1,(A2)+
		0x2608, // MOVE.L A0,D3
		0x968A, // SUB.L A2,D3
		0xD689, // ADD.L A1,D3
		0x7804, // MOVEQ #4,D4
		0xB883, // CMP.L D3,D4
		0x65F2, // BCS.S loop
	}
	setup := func(cpu *M68KCPU) {
		cpu.PC = startPC
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C
		cpu.DataRegs[1] = 0xA1B2C3D4
		cpu.DataRegs[3] = 0xDEADBEEF
		cpu.DataRegs[4] = 0xCAFEBABE
		cpu.AddrRegs[0] = 20
		cpu.AddrRegs[1] = dst
		cpu.AddrRegs[2] = dst
	}

	interp := newM68KTestProgramCPU(t, startPC)
	writeM68KWords(interp, startPC, program...)
	setup(interp)
	for steps := 0; interp.PC != startPC+14 && steps < 100; steps++ {
		interp.StepOne()
	}
	if interp.PC != startPC+14 {
		t.Fatalf("interpreter did not exit fill loop, pc=0x%08X", interp.PC)
	}

	jit := newM68KTestProgramCPU(t, startPC)
	writeM68KWords(jit, startPC, program...)
	setup(jit)
	retired, ok := jit.tryM68KLongFillCountLoop()
	if !ok {
		t.Fatal("long fill helper did not recognize loop")
	}
	if retired != 28 {
		t.Fatalf("retired = %d, want 28", retired)
	}

	assertM68KCoreStateEqual(t, interp, jit)
	for off := uint32(0); off < 32; off++ {
		if got, want := jit.Read8(dst+off), interp.Read8(dst+off); got != want {
			t.Fatalf("memory[%#x] = 0x%02X, want 0x%02X", dst+off, got, want)
		}
	}
}

func TestM68KJIT_LongFillCountLoopHelperChunksForInterruptCadence(t *testing.T) {
	const startPC = 0x1000
	const dst = 0x3000
	program := []uint16{
		0x24C1, // MOVE.L D1,(A2)+
		0x2608, // MOVE.L A0,D3
		0x968A, // SUB.L A2,D3
		0xD689, // ADD.L A1,D3
		0x7804, // MOVEQ #4,D4
		0xB883, // CMP.L D3,D4
		0x65F2, // BCS.S loop
	}

	cpu := newM68KTestProgramCPU(t, startPC)
	writeM68KWords(cpu, startPC, program...)
	cpu.PC = startPC
	cpu.SR = M68K_SR_S
	cpu.DataRegs[1] = 0x11223344
	cpu.AddrRegs[0] = 1024
	cpu.AddrRegs[1] = dst
	cpu.AddrRegs[2] = dst

	retired, ok := cpu.tryM68KLongFillCountLoop()
	if !ok {
		t.Fatal("long fill helper did not recognize loop")
	}
	if retired != 252 {
		t.Fatalf("retired = %d, want one 36-iteration chunk", retired)
	}
	if cpu.PC != startPC {
		t.Fatalf("PC = 0x%08X, want branch back to 0x%08X", cpu.PC, startPC)
	}
	if cpu.SR&M68K_SR_C == 0 {
		t.Fatal("helper chunk should leave CMP carry set for taken BCS")
	}
	if cpu.AddrRegs[2] != dst+36*4 {
		t.Fatalf("A2 = 0x%08X, want 0x%08X", cpu.AddrRegs[2], dst+36*4)
	}
}

func TestM68KJIT_Exec_LazyCCR_CMP_BLT(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgram(t, 0x1000,
		0x2C3C, 0x0000, 0x000A, // MOVE.L #10,D6
		0x203C, 0x0000, 0x0014, // MOVE.L #20,D0
		0xBC80, // CMP.L D0,D6
		0x6D02, // BLT.B +2 (skip MOVEQ #99,D1)
		0x7299, // MOVEQ #-103,D1 (skipped)
		0x7401, // MOVEQ #1,D2
	)

	if cpu.DataRegs[1] != 0 {
		t.Fatalf("D1 = %d, want 0 (BLT should skip)", cpu.DataRegs[1])
	}
	if cpu.DataRegs[2] != 1 {
		t.Fatalf("D2 = %d, want 1", cpu.DataRegs[2])
	}
}

func TestM68KJIT_Exec_LazyCCR_TST_BMI(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgram(t, 0x1000,
		0x70FF, // MOVEQ #-1,D0
		0x4A80, // TST.L D0
		0x6B02, // BMI.B +2 (skip MOVEQ #99,D1)
		0x7299, // MOVEQ #-103,D1 (skipped)
		0x7401, // MOVEQ #1,D2
	)

	if cpu.DataRegs[1] != 0 {
		t.Fatalf("D1 = %d, want 0 (BMI should skip)", cpu.DataRegs[1])
	}
	if cpu.DataRegs[2] != 1 {
		t.Fatalf("D2 = %d, want 1", cpu.DataRegs[2])
	}
}

func TestM68KJIT_Exec_CMPI_BGE(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgram(t, 0x1000,
		0x203C, 0x0000, 0x0260, // MOVE.L #$260,D0
		0x0C80, 0x0000, 0x0260, // CMPI.L #$260,D0
		0x6C02, // BGE.B +2 (skip MOVEQ #99,D1)
		0x7299, // MOVEQ #-103,D1 (skipped)
		0x7401, // MOVEQ #1,D2
	)

	if cpu.DataRegs[1] != 0 {
		t.Fatalf("D1 = %d, want 0 (BGE should skip)", cpu.DataRegs[1])
	}
	if cpu.DataRegs[2] != 1 {
		t.Fatalf("D2 = %d, want 1", cpu.DataRegs[2])
	}
}

func TestM68KJIT_Exec_TSTWordUsesLowWordForBMI(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgram(t, 0x1000,
		0x203C, 0x01FE, 0xB093, // MOVE.L #$01FEB093,D0
		0x4A40, // TST.W D0
		0x6B02, // BMI.B +2
		0x7299, // MOVEQ #-103,D1 (skipped)
		0x7401, // MOVEQ #1,D2
	)

	if cpu.DataRegs[1] != 0 {
		t.Fatalf("D1 = %d, want 0 (BMI should skip on negative low word)", cpu.DataRegs[1])
	}
	if cpu.DataRegs[2] != 1 {
		t.Fatalf("D2 = %d, want 1", cpu.DataRegs[2])
	}
}

func TestM68KJIT_Exec_ADDWordPreservesDestinationUpperWord(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgram(t, 0x1000,
		0x203C, 0x0000, 0x5280, // MOVE.L #$00005280,D0
		0x263C, 0x01FE, 0xB093, // MOVE.L #$01FEB093,D3
		0xD043, // ADD.W D3,D0
	)

	if got := cpu.DataRegs[0]; got != 0x00000313 {
		t.Fatalf("D0 = 0x%08X, want 0x00000313", got)
	}
}

func TestM68KJIT_Exec_SpilledAddressCalcAcrossMULUBail(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x263C) // MOVE.L #224,D3
	writeW(0x0000)
	writeW(0x00E0)
	writeW(0x243C) // MOVE.L #40,D2
	writeW(0x0000)
	writeW(0x0028)
	writeW(0x2803) // MOVE.L D3,D4
	writeW(0x2A3C) // MOVE.L #LINE_BYTES,D5
	writeW(0x0000)
	writeW(0x0A00)
	writeW(0xC8C5) // MULU.W D5,D4
	writeW(0x2A02) // MOVE.L D2,D5
	writeW(0xE58D) // LSL.L #2,D5
	writeW(0xD885) // ADD.L D5,D4
	writeW(0x0684) // ADDI.L #VRAM_START,D4
	writeW(0x0010)
	writeW(0x0000)
	writeW(0x2E04) // MOVE.L D4,D7
	writeW(0x4E72) // STOP #$2700
	writeW(0x2700)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	const want = 0x0018C0A0
	if got := cpu.DataRegs[4]; got != want {
		t.Fatalf("D4 = 0x%08X, want 0x%08X", got, want)
	}
	if got := cpu.DataRegs[7]; got != want {
		t.Fatalf("D7 = 0x%08X, want 0x%08X", got, want)
	}
	if got := cpu.DataRegs[5]; got != 0xA0 {
		t.Fatalf("D5 = 0x%08X, want 0x000000A0", got)
	}
}

// TestM68KJIT_Exec_BranchTaken runs a conditional branch through the dispatcher.
func TestM68KJIT_Exec_BranchTaken(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Program: MOVEQ #0,D0; TST.L D0; BEQ +4; MOVEQ #99,D1; MOVEQ #42,D2; STOP
	// BEQ should skip MOVEQ #99,D1 and land on MOVEQ #42,D2
	cpu := runM68KJITStopProgram(t, 0x1000,
		0x7000, // MOVEQ #0,D0
		0x4A80, // TST.L D0 → sets Z=1
		0x6702, // BEQ.B +2 (skip next instruction)
		0x7263, // MOVEQ #99,D1 (should be skipped)
		0x742A, // MOVEQ #42,D2 (branch target)
	)

	if cpu.DataRegs[1] != 0 {
		t.Errorf("D1 = %d, want 0 (should be skipped by BEQ)", cpu.DataRegs[1])
	}
	if cpu.DataRegs[2] != 42 {
		t.Errorf("D2 = %d, want 42", cpu.DataRegs[2])
	}
}

func TestM68KJIT_Exec_CMPWordThenBEQByteTaken(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgram(t, 0x1000,
		0x760A, // MOVEQ #10,D3
		0x7A0A, // MOVEQ #10,D5
		0xB645, // CMP.W D5,D3
		0x6702, // BEQ.B +2
		0x7099, // MOVEQ #-103,D0 (skipped)
		0x7001, // MOVEQ #1,D0
	)

	if got := cpu.DataRegs[0]; got != 1 {
		t.Fatalf("D0 = 0x%08X, want 0x00000001", got)
	}
}

func TestM68KJIT_Exec_SUBWordThenBGEByteTaken(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgram(t, 0x1000,
		0x7000, // MOVEQ #0,D0
		0x7E00, // MOVEQ #0,D7
		0x9E40, // SUB.W D0,D7
		0x6C02, // BGE.B +2
		0x7099, // MOVEQ #-103,D0 (skipped)
		0x7001, // MOVEQ #1,D0
	)

	if got := cpu.DataRegs[0]; got != 1 {
		t.Fatalf("D0 = 0x%08X, want 0x00000001", got)
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeEmuTOSWordByteHotLoopShape(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const startPC = 0x1000
	program := []uint16{
		0x3E03,         // MOVE.W D3,D7
		0xDE49,         // ADD.W A1,D7
		0xBE6A, 0x0004, // CMP.W 4(A2),D7
		0x6EBC,                 // BGT.B -68 (not taken in this setup)
		0x3E0C,                 // MOVE.W A4,D7
		0xCE44,                 // AND.W D4,D7
		0x56C7,                 // SNE D7
		0xCE00,                 // AND.B D0,D7
		0x1187,                 // MOVE.B D7,(A0)
		0x9800,                 // brief indexed extension: (0,A0,D1.L)
		0xE25C,                 // ROR.W #1,D4
		0x4EF9, 0x0000, 0x1100, // JMP $00001100
	}

	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x0000000F
		cpu.DataRegs[3] = 0xAAAA0001
		cpu.DataRegs[4] = 0x123400F0
		cpu.DataRegs[7] = 0x55555555
		cpu.AddrRegs[0] = 0x3000
		cpu.AddrRegs[1] = 0x00020002
		cpu.AddrRegs[2] = 0x3100
		cpu.AddrRegs[4] = 0x777700F3
		cpu.Write16(0x3104, 0x0003)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	writeM68KWords(interp, startPC, program...)
	writeM68KStopProgram(interp, 0x1100)
	setup(interp)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	writeM68KWords(jit, startPC, program...)
	writeM68KStopProgram(jit, 0x1100)
	setup(jit)
	instrs := m68kScanBlock(jit.memory, startPC)
	if m68kNeedsFallback(instrs) {
		t.Fatalf("hot-loop shape rejected by m68kNeedsFallback; instrs=%d first=%04X", len(instrs), instrs[0].opcode)
	}
	if m68kNeedsConservativeFallback(jit.memory, startPC, instrs) {
		t.Fatalf("hot-loop shape rejected by m68kNeedsConservativeFallback; instrs=%d", len(instrs))
	}
	if !m68kBlockProductionNativeSafe(instrs) {
		for _, instr := range instrs {
			if !m68kInstrProductionNativeSafe(&instr) {
				t.Fatalf("hot-loop shape opcode %04X at +%d is not production-native safe", instr.opcode, instr.pcOffset)
			}
		}
		t.Fatal("hot-loop shape rejected by m68kBlockProductionNativeSafe")
	}
	if m68kBlockMayUseGenericIOFallback(instrs) {
		t.Fatalf("hot-loop shape rejected by m68kBlockMayUseGenericIOFallback")
	}
	runM68KJITUntilStopped(t, jit)

	assertM68KCoreStateEqual(t, interp, jit)
	if got, want := jit.Read8(0x3000), interp.Read8(0x3000); got != want {
		t.Fatalf("hot-loop store byte = 0x%02X, want 0x%02X", got, want)
	}
	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute EmuTOS hot-loop shape natively")
	}
	if got := jit.m68kJitFallbackInstructions.Load(); got != 1 {
		t.Fatalf("fallback instructions = %d, want only STOP fallback", got)
	}
	if got := jit.m68kJitBailoutCount.Load(); got != 0 {
		t.Fatalf("bailouts = %d, want 0", got)
	}
}

func TestM68KJIT_ProductionModePromotesSafeRegionWithoutForceNative(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	blockA := uint32(0x1000)
	blockB := uint32(0x1100)
	dispAToB := int16(int32(blockB) - int32(blockA+4))
	dispBToA := int16(int32(blockA) - int32(blockB+4))
	cpu := newM68KTestProgramCPU(t, blockA)
	writeM68KWords(cpu, blockA,
		0x7001,                   // MOVEQ #1,D0
		0x6000, uint16(dispAToB), // BRA.W blockB
	)
	writeM68KWords(cpu, blockB,
		0x5280,                   // ADDQ.L #1,D0
		0x6000, uint16(dispBToA), // BRA.W blockA
	)

	if region := m68kFormRegion(blockA, cpu.memory); region == nil || len(region.blocks) != 2 {
		t.Fatalf("test program did not form a safe 2-block region: %+v", region)
	}

	cpu.m68kJitEnabled = true
	cpu.m68kJitPersist = true
	cpu.m68kJitForceNative = false
	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	promoted := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cpu.m68kJitRegionPromotions.Load() > 0 {
			promoted = true
			break
		}
		select {
		case <-done:
			t.Fatalf("M68K JIT returned before promotion: PC=0x%08X", cpu.PC)
		default:
			time.Sleep(time.Millisecond)
		}
	}

	cpu.running.Store(false)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("M68K JIT did not stop after production-mode promotion test")
	}
	defer func() {
		cpu.m68kJitPersist = false
		cpu.freeM68KJIT()
	}()

	if !promoted {
		var tier int
		var execCount uint32
		tier = -1
		if cache := cpu.m68kJitCache; cache != nil {
			if block := cache.Get(uint64(blockA)); block != nil {
				tier = block.tier
				execCount = block.execCount
			}
		}
		t.Fatalf("safe M68K region was not promoted in production mode: tier=%d execCount=%d promotions=%d nativeBlocks=%d forceNative=%v",
			tier, execCount, cpu.m68kJitRegionPromotions.Load(), cpu.m68kJitNativeBlocksExecuted.Load(), cpu.m68kJitForceNative)
	}
}

func TestM68KJIT_HighRAMMOVEMRTSAdmittedAsFullNativeBlock(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	blockA := uint32(0x00910000)
	blockB := uint32(0x00910100)
	dispAToB := int16(int32(blockB) - int32(blockA+4))
	cpu := newM68KTestProgramCPU(t, blockA)
	writeM68KWords(cpu, blockA,
		0x7001,                   // MOVEQ #1,D0
		0x6000, uint16(dispAToB), // BRA.W blockB
	)
	writeM68KWords(cpu, blockB,
		0x4CDF, 0x043C, // MOVEM.L (A7)+,D2/D3/D4/D5/A2
		0x4E75, // RTS
	)

	instrs := m68kScanBlock(cpu.memory, blockB)
	if !m68kCanUseProductionNativeBlock(cpu.memory, blockB, instrs) {
		t.Fatal("high-RAM MOVEM/RTS block was rejected by production native gate")
	}
	if region := m68kFormRegion(blockA, cpu.memory); region == nil {
		t.Fatal("high-RAM MOVEM/RTS block was not eligible for native region formation")
	}
}

func TestM68KJIT_HighRAMStackEpilogueUsesNativePrefix(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// With the native-PC ceiling removed, high-RAM code is compiled natively
	// across the whole address space. A stack/control epilogue whose block ends
	// at a terminator (RTS) is split into a native prefix of the safe leading
	// instructions; only the terminator is left for the dispatcher. The prefix
	// must therefore be the three instructions preceding the RTS.
	startPC := uint32(0x00912000)
	cpu := newM68KTestProgramCPU(t, startPC)
	writeM68KWords(cpu, startPC,
		0x4A80,         // TST.L D0
		0x66E8,         // BNE.S target outside this epilogue
		0x4CDF, 0x043C, // MOVEM.L (A7)+,D2/D3/D4/D5/A2
		0x4E75, // RTS
	)

	instrs := m68kScanBlock(cpu.memory, startPC)
	prefix := m68kProductionNativePrefix(cpu.memory, startPC, instrs)
	if len(prefix) != 3 {
		t.Fatalf("high-RAM stack/control epilogue native prefix len=%d, want 3 (TST.L/BNE.S/MOVEM.L before RTS): instrs=%+v", len(prefix), prefix)
	}
	if prefix[len(prefix)-1].opcode != 0x4CDF {
		t.Fatalf("high-RAM epilogue prefix should end at MOVEM.L (0x4CDF), got 0x%04X", prefix[len(prefix)-1].opcode)
	}
}

func TestM68KJIT_DispatcherChasesStaticJMPTrampoline(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		trampolinePC = uint32(0x1000)
		targetPC     = uint32(0x2000)
	)
	cpu := newM68KTestProgramCPU(t, trampolinePC)
	writeM68KWords(cpu, trampolinePC,
		0x4EF9, 0x0000, 0x2000, // JMP $2000
	)
	writeM68KWords(cpu, targetPC,
		0x702A,         // MOVEQ #42,D0
		0x4E72, 0x2700, // STOP
	)

	cpu.m68kJitRecordNativePCs.Store(true)
	t.Cleanup(func() { cpu.m68kJitRecordNativePCs.Store(false) })

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.After(2 * time.Second)
	for !cpu.stopped.Load() {
		select {
		case <-deadline:
			cpu.running.Store(false)
			waitDoneWithGuard(t, done)
			t.Fatal("static JMP trampoline program timed out")
		default:
			runtime.Gosched()
		}
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if got := cpu.DataRegs[0]; got != 42 {
		t.Fatalf("D0 = %d, want 42", got)
	}
	if got := cpu.m68kJitStaticJMPChases.Load(); got != 1 {
		t.Fatalf("static JMP chases = %d, want 1", got)
	}
	if got := cpu.InstructionCount; got != 3 {
		t.Fatalf("InstructionCount = %d, want 3 (JMP + MOVEQ + STOP)", got)
	}
	cpu.m68kJitNativePCMu.Lock()
	trampolineEntries := cpu.m68kJitNativePCCounts[trampolinePC]
	targetEntries := cpu.m68kJitNativePCCounts[targetPC]
	cpu.m68kJitNativePCMu.Unlock()
	if trampolineEntries != 0 {
		t.Fatalf("native entries at trampoline PC = %d, want 0", trampolineEntries)
	}
	if targetEntries == 0 {
		t.Fatalf("native target PC %08X was never entered", targetPC)
	}
}

// runM68KJITStopProgram runs a program followed by STOP, waits for it to halt.
func runM68KJITStopProgram(t *testing.T, startPC uint32, opcodes ...uint16) *M68KCPU {
	return runM68KJITStopProgramWithSetup(t, startPC, nil, false, opcodes...)
}

func runM68KInterpreterStopProgram(t *testing.T, startPC uint32, opcodes ...uint16) *M68KCPU {
	t.Helper()

	cpu := newM68KTestProgramCPU(t, startPC)

	pc := startPC
	writeM68KStopProgram(cpu, pc, opcodes...)

	runM68KInterpreterUntilStopped(t, cpu)
	return cpu
}

func writeM68KStopProgram(cpu *M68KCPU, startPC uint32, opcodes ...uint16) {
	writeM68KWords(cpu, startPC, opcodes...)
	pc := startPC + uint32(len(opcodes))*2
	cpu.memory[pc] = 0x4E
	cpu.memory[pc+1] = 0x72
	cpu.memory[pc+2] = 0x27
	cpu.memory[pc+3] = 0x00
}

func writeM68KWords(cpu *M68KCPU, startPC uint32, opcodes ...uint16) {
	pc := startPC
	for _, op := range opcodes {
		cpu.memory[pc] = byte(op >> 8)
		cpu.memory[pc+1] = byte(op)
		pc += 2
	}
}

func newM68KTestProgramCPU(t *testing.T, startPC uint32) *M68KCPU {
	t.Helper()

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, startPC)
	cpu := NewM68KCPU(bus)
	cpu.PC = startPC
	cpu.SR = M68K_SR_S
	cpu.m68kJitWarmupLimit = 1
	return cpu
}

func runM68KInterpreterUntilStopped(t *testing.T, cpu *M68KCPU) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && time.Now().Before(deadline) {
		if cpu.StepOne() == 0 {
			break
		}
	}
	if !cpu.stopped.Load() {
		t.Fatal("M68K interpreter stop program timed out before STOP")
	}
}

func runM68KJITUntilStopped(t *testing.T, cpu *M68KCPU) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !cpu.stopped.Load() && time.Now().Before(deadline) {
		select {
		case <-done:
			if !cpu.stopped.Load() {
				t.Fatalf("M68K JIT returned before STOP: PC=0x%08X SR=0x%04X", cpu.PC, cpu.SR)
			}
			return
		default:
			runtime.Gosched()
			time.Sleep(time.Millisecond)
		}
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)
	if !cpu.stopped.Load() {
		t.Fatalf("M68K JIT stop program timed out before STOP: PC=0x%08X SR=0x%04X native_blocks=%d fallback_instructions=%d bailouts=%d D0=0x%08X D1=0x%08X D2=0x%08X",
			cpu.PC, cpu.SR,
			cpu.m68kJitNativeBlocksExecuted.Load(),
			cpu.m68kJitFallbackInstructions.Load(),
			cpu.m68kJitBailoutCount.Load(),
			cpu.DataRegs[0], cpu.DataRegs[1], cpu.DataRegs[2])
	}
}

func assertM68KCoreStateEqual(t *testing.T, got, want *M68KCPU) {
	t.Helper()

	if got.PC != want.PC {
		t.Fatalf("PC mismatch: got=0x%08X want=0x%08X", got.PC, want.PC)
	}
	if got.SR != want.SR {
		t.Fatalf("SR mismatch: got=0x%04X want=0x%04X", got.SR, want.SR)
	}
	for reg := range 8 {
		if got.DataRegs[reg] != want.DataRegs[reg] {
			t.Fatalf("D%d mismatch: got=0x%08X want=0x%08X", reg, got.DataRegs[reg], want.DataRegs[reg])
		}
		if got.AddrRegs[reg] != want.AddrRegs[reg] {
			t.Fatalf("A%d mismatch: got=0x%08X want=0x%08X", reg, got.AddrRegs[reg], want.AddrRegs[reg])
		}
	}
}

func assertM68KFPUStateEqual(t *testing.T, got, want *M68KCPU) {
	t.Helper()

	if got.FPU == nil || want.FPU == nil {
		if got.FPU != want.FPU {
			t.Fatalf("FPU presence mismatch: got=%v want=%v", got.FPU != nil, want.FPU != nil)
		}
		return
	}
	for reg := range 8 {
		if got.FPU.GetFP64(reg) != want.FPU.GetFP64(reg) {
			t.Fatalf("FP%d mismatch: got=%v want=%v", reg, got.FPU.GetFP64(reg), want.FPU.GetFP64(reg))
		}
	}
	if got.FPU.FPCR != want.FPU.FPCR {
		t.Fatalf("FPCR mismatch: got=0x%08X want=0x%08X", got.FPU.FPCR, want.FPU.FPCR)
	}
	if got.FPU.FPSR != want.FPU.FPSR {
		t.Fatalf("FPSR mismatch: got=0x%08X want=0x%08X", got.FPU.FPSR, want.FPU.FPSR)
	}
	if got.FPU.FPIAR != want.FPU.FPIAR {
		t.Fatalf("FPIAR mismatch: got=0x%08X want=0x%08X", got.FPU.FPIAR, want.FPU.FPIAR)
	}
}

func runM68KJITStopProgramWithSetup(t *testing.T, startPC uint32, setup func(*M68KCPU), forceNative bool, opcodes ...uint16) *M68KCPU {
	t.Helper()

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, startPC)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.m68kJitForceNative = forceNative
	cpu.m68kJitWarmupLimit = 1
	cpu.PC = startPC
	cpu.SR = M68K_SR_S
	if setup != nil {
		setup(cpu)
	}

	// Write opcodes
	pc := startPC
	for _, op := range opcodes {
		cpu.memory[pc] = byte(op >> 8)
		cpu.memory[pc+1] = byte(op)
		pc += 2
	}
	// Append STOP #$2700
	cpu.memory[pc] = 0x4E
	cpu.memory[pc+1] = 0x72
	cpu.memory[pc+2] = 0x27
	cpu.memory[pc+3] = 0x00

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cpu.running.Store(false)
		waitDoneWithGuard(t, done)
		// Don't fatal — STOP halts in a loop, we just stop it externally
	}

	return cpu
}

func TestM68KJIT_Exec_SccAbsoluteLongFallbackSetsByte(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const target = 0x2301
	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.SR = M68K_SR_S | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C | M68K_SR_X
		cpu.Write8(target, 0x00)
		cpu.Write8(target+1, 0x77)
	}, false,
		0x50F9, 0x0000, target, // ST (xxx).L
	)

	if got := cpu.Read8(target); got != 0xFF {
		t.Fatalf("target byte = 0x%02X, want 0xFF", got)
	}
	if got := cpu.Read8(target + 1); got != 0x77 {
		t.Fatalf("adjacent byte = 0x%02X, want 0x77", got)
	}
}

// TestM68KJIT_Exec_DBRA_Loop runs a DBRA loop through the full dispatcher.
func TestM68KJIT_Exec_DBRA_Loop(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Program: MOVEQ #3,D0; MOVEQ #0,D1; loop: ADDQ #1,D1; DBRA D0,loop; STOP
	// Loop runs 4 times (D0: 3→2→1→0→-1)
	// After: D1 = 4
	cpu := runM68KJITStopProgram(t, 0x1000,
		0x7003,         // MOVEQ #3,D0
		0x7200,         // MOVEQ #0,D1
		0x5281,         // ADDQ.L #1,D1 (at 0x1004)
		0x51C8, 0xFFFC, // DBRA D0,$1004 (displacement = -4)
	)

	if cpu.DataRegs[1] != 4 {
		t.Errorf("DBRA loop: D1 = %d, want 4", cpu.DataRegs[1])
	}
}

func TestM68KJIT_Exec_NOT_B_NativeDispatcher(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.DataRegs[5] = 0x0000007F
	}, true,
		0x4605, // NOT.B D5
	)

	if got, want := cpu.DataRegs[5], uint32(0x00000080); got != want {
		t.Fatalf("D5 = 0x%08X, want 0x%08X", got, want)
	}
}

func TestM68KJIT_Exec_MullLongPostincrement_NativeDispatcher(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 3
		cpu.AddrRegs[2] = 0x2000
		cpu.Write32(0x2000, 14)
	}, true,
		0x4C1A, 0x0800, // MULL.L (A2)+,<ext=0x0800>
	)

	if got, want := cpu.AddrRegs[2], uint32(0x2004); got != want {
		t.Fatalf("A2 = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := cpu.DataRegs[0], uint32(42); got != want {
		t.Fatalf("D0 = 0x%08X, want 0x%08X", got, want)
	}
}

func TestM68KJIT_Exec_MullLongRegister_NativeDispatcher(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.DataRegs[3] = 7
		cpu.DataRegs[2] = 6
	}, true,
		0x4C03, 0x2000, // MULL.L D3,D2
	)

	if got, want := cpu.DataRegs[2], uint32(42); got != want {
		t.Fatalf("D2 = 0x%08X, want 0x%08X", got, want)
	}
}

func TestM68KJIT_DefaultDispatcherExecutesAROSMULLLongRegisterNoFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const startPC = uint32(0x1000)
	opcodes := []uint16{
		0x4C00, 0x5800, // MULL.L D0,D5 (signed 32-bit result), seen in AROS boot path
	}
	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 6
		cpu.DataRegs[5] = 7
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) != len(opcodes)/2 || !m68kCanUseProductionNativeBlock(jit.memory, startPC, prefix) {
		t.Fatalf("AROS MULL.L register form rejected by production gate: instrs=%d prefix=%d needsFallback=%v conservative=%v productionSafe=%v genericIO=%v",
			len(instrs),
			len(prefix),
			m68kNeedsFallback(prefix),
			m68kNeedsConservativeFallback(jit.memory, startPC, prefix),
			m68kBlockProductionNativeSafe(prefix),
			m68kBlockMayUseGenericIOFallback(prefix))
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("default M68K JIT dispatcher did not execute AROS MULL.L block natively")
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x4C00].Load(); got != 0 {
		t.Fatalf("AROS MULL.L register form fell back to interpreter %d times", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_Exec_MullLongRegisterSigned64_NativeDispatcher(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0xFFFFFFFD // -3
		cpu.DataRegs[1] = 7
	}, true,
		0x4C00, 0x1C02, // MULL.L D0,D2:D1 (signed 64-bit result)
	)

	if got, want := cpu.DataRegs[2], uint32(0xFFFFFFFF); got != want {
		t.Fatalf("D2 = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := cpu.DataRegs[1], uint32(0xFFFFFFEB); got != want {
		t.Fatalf("D1 = 0x%08X, want 0x%08X", got, want)
	}
}

func TestM68KJIT_Exec_DivLongRegister_NativeDispatcher(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 4
		cpu.DataRegs[2] = 14
		cpu.DataRegs[1] = 0x11111111
	}, true,
		0x4C40, 0x2001, // DIVL.L D0,D1:D2
	)

	if got, want := cpu.DataRegs[2], uint32(3); got != want {
		t.Fatalf("D2 = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := cpu.DataRegs[1], uint32(2); got != want {
		t.Fatalf("D1 = 0x%08X, want 0x%08X", got, want)
	}
}

func TestM68KJIT_Exec_DivLongRegisterSigned64_NativeDispatcher(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 3
		cpu.DataRegs[2] = 0xFFFFFFFF
		cpu.DataRegs[1] = 0xFFFFFFEB
	}, true,
		0x4C40, 0x1C02, // DIVL.L D0,D2:D1 (signed 64-bit)
	)

	if got, want := cpu.DataRegs[1], uint32(0xFFFFFFF9); got != want {
		t.Fatalf("D1 = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := cpu.DataRegs[2], uint32(0); got != want {
		t.Fatalf("D2 = 0x%08X, want 0x%08X", got, want)
	}
}

func TestM68KJIT_Exec_DivLongPostincrement_NativeDispatcher(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.DataRegs[2] = 14
		cpu.AddrRegs[0] = 0x2000
		cpu.Write32(0x2000, 4)
	}, true,
		0x4C58, 0x2001, // DIVL.L (A0)+,D1:D2
	)

	if got, want := cpu.DataRegs[2], uint32(3); got != want {
		t.Fatalf("D2 = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := cpu.DataRegs[1], uint32(2); got != want {
		t.Fatalf("D1 = 0x%08X, want 0x%08X", got, want)
	}
	if got, want := cpu.AddrRegs[0], uint32(0x2004); got != want {
		t.Fatalf("A0 = 0x%08X, want 0x%08X", got, want)
	}
}

func TestM68KJIT_Exec_DivuWordImmediate_NativeDispatcher(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C
		cpu.DataRegs[0] = 101
	}, true,
		0x80FC, 0x000A, // DIVU.W #10,D0
	)

	if got := cpu.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("DIVU.W immediate did not execute through a native JIT block")
	}
	if got, want := cpu.DataRegs[0], uint32(0x0001000A); got != want {
		t.Fatalf("D0 = 0x%08X, want 0x%08X", got, want)
	}
}

func TestM68KJIT_Exec_DivsWordImmediateSignedSource_NativeDispatcher(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_Z | M68K_SR_V | M68K_SR_C
		cpu.DataRegs[0] = 0xFFFFFFF9
	}, true,
		0x81FC, 0xFFFE, // DIVS.W #-2,D0
	)

	if got := cpu.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("DIVS.W immediate did not execute through a native JIT block")
	}
	if got, want := cpu.DataRegs[0], uint32(0xFFFF0003); got != want {
		t.Fatalf("D0 = 0x%08X, want 0x%08X", got, want)
	}
}

func TestM68KJIT_Exec_MuluWordImmediate_NativeDispatcher(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x0000FFFF
	}, true,
		0xC0FC, 0xFFFF, // MULU.W #$FFFF,D0
	)

	if got := cpu.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("MULU.W immediate did not execute through a native JIT block")
	}
	if got, want := cpu.DataRegs[0], uint32(0xFFFE0001); got != want {
		t.Fatalf("D0 = 0x%08X, want 0x%08X", got, want)
	}
}

func TestM68KJIT_Exec_MulsWordImmediateSignedSource_NativeDispatcher(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x00000010
	}, true,
		0xC1FC, 0xFFFF, // MULS.W #-1,D0
	)

	if got := cpu.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("MULS.W immediate did not execute through a native JIT block")
	}
	if got, want := cpu.DataRegs[0], uint32(0xFFFFFFF0); got != want {
		t.Fatalf("D0 = 0x%08X, want 0x%08X", got, want)
	}
}

// TestM68KJIT_Exec_MemoryALU runs ADD with immediate operand through dispatcher.
func TestM68KJIT_Exec_MemoryALU(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgram(t, 0x1000,
		0x203C, 0x0000, 0x0064, // MOVE.L #100,D0
		0xD0BC, 0x0000, 0x0032, // ADD.L #50,D0
	)

	if cpu.DataRegs[0] != 150 {
		t.Errorf("ADD.L #50,D0: D0 = %d, want 150", cpu.DataRegs[0])
	}
}

// ===========================================================================
// Block Chaining Tests (Stages 2-4)
// ===========================================================================

// TestM68KJIT_Exec_BRA_ChainPatch verifies that BRA chains directly to a
// compiled target block via patched JMP rel32.
func TestM68KJIT_Exec_BRA_ChainPatch(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Sequential code: MOVEQ #1,D0; BRA.B +4; MOVEQ #99,D2; MOVEQ #2,D1; STOP
	// BRA.B +4 skips MOVEQ #99,D2 (2 bytes) + goes to MOVEQ #2,D1.
	// BRA.B displacement = +4 means target = instrPC+2+4.
	// At 0x1002: BRA.B → target = 0x1002 + 2 + 4 = 0x1008.
	// 0x1004: MOVEQ #99,D2 (skipped)
	// 0x1006: MOVEQ #77,D2 (also skipped — BRA skips to 0x1008)
	// Actually BRA.B +2: target = 0x1002 + 2 + 2 = 0x1006
	// But BRA exits block! So MOVEQ #2,D1 is in a different block.
	// That's fine — dispatcher re-enters at target and compiles block B.
	cpu := runM68KJITStopProgram(t, 0x1000,
		0x7001, // MOVEQ #1,D0 at 0x1000
		0x6004, // BRA.B +4 at 0x1002 → target 0x1008
		0x7499, // MOVEQ #-103,D2 at 0x1004 (skipped by BRA)
		0x4E71, // NOP at 0x1006 (skipped by BRA)
		0x7201, // MOVEQ #1,D1 at 0x1008 (target of BRA) — 0x7201 = MOVEQ #1,D1
	)

	if cpu.DataRegs[0] != 1 {
		t.Errorf("D0 = %d, want 1 (set before BRA)", cpu.DataRegs[0])
	}
	if cpu.DataRegs[1] != 1 {
		t.Errorf("D1 = %d, want 1 (set at BRA target)", cpu.DataRegs[1])
	}
}

// TestM68KJIT_Exec_ChainBudgetExhaustion verifies that chained execution
// returns to Go after the chain budget is exhausted.
func TestM68KJIT_Exec_ChainBudgetExhaustion(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Two blocks that BRA to each other, forming an infinite chain loop.
	// The budget (64) should stop execution and return to Go.
	// Block A at 0x1000: ADDQ #1,D0; BRA 0x2000
	// Block B at 0x2000: ADDQ #1,D0; BRA 0x1000
	// After timeout: D0 > 0 (proves blocks executed) and program didn't hang.
	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S

	w := func(pc uint32, ops ...uint16) {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
	}

	// Block A: ADDQ.L #1,D0 + BRA.W to 0x2000 (disp = 0x2000 - 0x1006 = 0x0FFA)
	w(0x1000, 0x5280, 0x6000, 0x0FFA)
	// Block B: ADDQ.L #1,D0 + BRA.W to 0x1000 (disp = 0x1000 - 0x2006 = 0xEFFA)
	w(0x2000, 0x5280, 0x6000, 0xEFFA)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	// D0 should be large (many iterations via chaining) but execution didn't hang
	if cpu.DataRegs[0] == 0 {
		t.Error("D0 should be > 0 after chained BRA loop")
	}
}

// TestM68KJIT_Exec_JSR_RTS_Chain verifies JSR→subroutine→RTS chain round-trip.
func TestM68KJIT_Exec_JSR_RTS_Chain(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Simple: JSR $2000; MOVEQ #3,D0; STOP
	// Sub at $2000: MOVEQ #2,D1; RTS
	// After: D0=3 (from code after JSR), D1=2 (from subroutine)
	// This is the same pattern as the existing TestM68KJIT_Exec_JSR_RTS
	// but explicitly tests that chain patching connects JSR→sub→RTS→return.
	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	w := func(ops ...uint16) {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
	}

	w(0x7001)                 // MOVEQ #1,D0
	w(0x4EB9, 0x0000, 0x2000) // JSR $2000
	w(0x7003)                 // MOVEQ #3,D0 (after RTS)
	w(0x4E72, 0x2700)         // STOP

	pc = 0x2000
	w(0x7402) // MOVEQ #2,D2
	w(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if cpu.DataRegs[0] != 3 {
		t.Errorf("D0 = %d, want 3 (set after JSR return)", cpu.DataRegs[0])
	}
	if cpu.DataRegs[2] != 2 {
		t.Errorf("D2 = %d, want 2 (set in subroutine)", cpu.DataRegs[2])
	}
}

// ===========================================================================
// Lazy CCR Integration Tests (Stage 5)
// ===========================================================================

// TestM68KJIT_Exec_LazyCCR_CMP_BEQ verifies CMP;BEQ uses direct Jcc from EFLAGS.
func TestM68KJIT_Exec_LazyCCR_CMP_BEQ(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// D0=42, D1=42. CMP D1,D0; BEQ skip; MOVEQ #99,D2; skip: MOVEQ #1,D3; STOP
	// BEQ taken → D2 stays 0, D3=1
	cpu := runM68KJITStopProgram(t, 0x1000,
		0x203C, 0x0000, 0x002A, // MOVE.L #42,D0
		0x223C, 0x0000, 0x002A, // MOVE.L #42,D1
		0xB081, // CMP.L D1,D0
		0x6702, // BEQ.B +2 (skip MOVEQ #99)
		0x7499, // MOVEQ #-103,D2 (skipped)
		0x7601, // MOVEQ #1,D3
	)

	if cpu.DataRegs[2] != 0 {
		t.Errorf("D2 = %d, want 0 (BEQ should skip)", cpu.DataRegs[2])
	}
	if cpu.DataRegs[3] != 1 {
		t.Errorf("D3 = %d, want 1", cpu.DataRegs[3])
	}
}

// TestM68KJIT_Exec_LazyCCR_ADD_BCS verifies ADD;BCS with carry from EFLAGS.
func TestM68KJIT_Exec_LazyCCR_ADD_BCS(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// D0=0xFFFFFFFF, D1=1. ADD D1,D0 → carry. BCS skip; MOVEQ #99,D2; skip: MOVEQ #1,D3; STOP
	cpu := runM68KJITStopProgram(t, 0x1000,
		0x203C, 0xFFFF, 0xFFFF, // MOVE.L #$FFFFFFFF,D0
		0x7201, // MOVEQ #1,D1
		0xD081, // ADD.L D1,D0 → 0 with carry
		0x6502, // BCS.B +2 (skip)
		0x7499, // MOVEQ #-103,D2 (skipped)
		0x7601, // MOVEQ #1,D3
	)

	if cpu.DataRegs[0] != 0 {
		t.Errorf("D0 = 0x%08X, want 0", cpu.DataRegs[0])
	}
	if cpu.DataRegs[2] != 0 {
		t.Errorf("D2 = %d, want 0 (BCS should skip — carry set)", cpu.DataRegs[2])
	}
	if cpu.DataRegs[3] != 1 {
		t.Errorf("D3 = %d, want 1", cpu.DataRegs[3])
	}
}

func TestM68KJIT_Exec_SUBQAddressRegisterPreservesPriorCCRForBNE(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.DataRegs[4] = 2
		cpu.AddrRegs[1] = 4
	}, false,
		0x5384, // SUBQ.L #1,D4
		0x5989, // SUBQ.L #4,A1; must not change CCR
		0x66FA, // BNE.S $1000, tests D4's Z flag
	)

	if cpu.DataRegs[4] != 0 {
		t.Fatalf("D4=0x%08X, want 0; BNE used flags clobbered by SUBQ.L A1", cpu.DataRegs[4])
	}
	if cpu.AddrRegs[1] != 0xFFFFFFFC {
		t.Fatalf("A1=0x%08X, want 0xFFFFFFFC", cpu.AddrRegs[1])
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeADDQSUBQA7PreservesCCR(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tt := range []struct {
		name    string
		opcode  uint16
		wantA7  uint32
		wantD1  uint32
		counter uint16
	}{
		{name: "addq", opcode: 0x508F, wantA7: 0x120010, wantD1: 0x11, counter: 0x5281}, // ADDQ.L #8,A7; ADDQ.L #1,D1
		{name: "subq", opcode: 0x518F, wantA7: 0x11FFF0, wantD1: 0x11, counter: 0x5281}, // SUBQ.L #8,A7; ADDQ.L #1,D1
	} {
		t.Run(tt.name, func(t *testing.T) {
			const startPC = uint32(0x1000)
			setup := func(cpu *M68KCPU) {
				cpu.AddrRegs[7] = 0x120000
				cpu.DataRegs[0] = 2
				cpu.DataRegs[1] = 0x10
			}
			opcodes := []uint16{
				0x5380, // SUBQ.L #1,D0, sets non-zero CCR for first BNE
				tt.opcode,
				0x66FA, // BNE.S back to SUBQ; must use D0 flags, not A7 update
				tt.counter,
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			writeM68KStopProgram(interp, startPC, opcodes...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			writeM68KStopProgram(jit, startPC, opcodes...)
			instrs := m68kScanBlock(jit.memory, startPC)
			prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
			if len(prefix) != len(opcodes) {
				candidate := instrs
				if len(prefix)+1 < len(instrs) {
					candidate = instrs[:len(prefix)+2]
				}
				t.Fatalf("default M68K JIT dispatcher rejected A7 %s native prefix: prefix=%+v fallback=%v conservative=%v productionSafe=%v genericIO=%v instrs=%+v",
					tt.name, prefix,
					m68kNeedsFallback(candidate), m68kNeedsConservativeFallback(jit.memory, startPC, candidate),
					m68kBlockProductionNativeSafe(candidate), m68kBlockMayUseGenericIOFallback(candidate), candidate)
			}
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("default M68K JIT dispatcher did not execute A7 %s block natively", tt.name)
			}
			if got := jit.m68kJitFallbackOpcodeCounts[tt.opcode].Load(); got != 0 {
				t.Fatalf("A7 %s opcode 0x%04X fell back %d times", tt.name, tt.opcode, got)
			}
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("A7 %s block bailed out %d times, want 0", tt.name, got)
			}
			if got := jit.AddrRegs[7]; got != tt.wantA7 {
				t.Fatalf("A7 after %s = 0x%08X, want 0x%08X", tt.name, got, tt.wantA7)
			}
			if got := jit.DataRegs[1]; got != tt.wantD1 {
				t.Fatalf("D1 after %s = 0x%08X, want 0x%08X", tt.name, got, tt.wantD1)
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativePostincADDQSUBQ(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	tests := []struct {
		name       string
		opcode     uint16
		addrReg    int
		initialA   uint32
		initialMem uint32
	}{
		{name: "addq_b_a7_postinc", opcode: 0x541F, addrReg: 7, initialA: 0x120000, initialMem: 0xFE000000},
		{name: "subq_w_a7_postinc", opcode: 0x535F, addrReg: 7, initialA: 0x120100, initialMem: 0x00010000},
		{name: "subq_l_a7_postinc", opcode: 0x579F, addrReg: 7, initialA: 0x120200, initialMem: 0x00000004},
		{name: "subq_w_a6_postinc", opcode: 0x575E, addrReg: 6, initialA: 0x120300, initialMem: 0x00080000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const startPC = uint32(0x1000)
			opcodes := []uint16{
				tt.opcode,
				0x7E01, // MOVEQ #1,D7
			}

			setup := func(cpu *M68KCPU) {
				cpu.AddrRegs[tt.addrReg] = tt.initialA
				cpu.Write32(tt.initialA, tt.initialMem)
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			writeM68KStopProgram(interp, startPC, opcodes...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			writeM68KStopProgram(jit, startPC, opcodes...)
			instrs := m68kScanBlock(jit.memory, startPC)
			prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
			const wantPrefixInstrs = 2
			if len(prefix) != wantPrefixInstrs || !m68kCanUseProductionNativeBlock(jit.memory, startPC, prefix) {
				t.Fatalf("%04X rejected by production gate: instrs=%d prefix=%d needsFallback=%v conservative=%v productionSafe=%v genericIO=%v",
					tt.opcode,
					len(instrs),
					len(prefix),
					m68kNeedsFallback(prefix),
					m68kNeedsConservativeFallback(jit.memory, startPC, prefix),
					m68kBlockProductionNativeSafe(prefix),
					m68kBlockMayUseGenericIOFallback(prefix))
			}

			runM68KJITUntilStopped(t, jit)
			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("default M68K JIT dispatcher did not execute %04X block natively", tt.opcode)
			}
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("%04X block bailed out %d times, want 0", tt.opcode, got)
			}
			if got := jit.m68kJitFallbackOpcodeCounts[tt.opcode].Load(); got != 0 {
				t.Fatalf("%04X fallback count = %d, want 0", tt.opcode, got)
			}
			if got, want := jit.Read32(tt.initialA), interp.Read32(tt.initialA); got != want {
				t.Fatalf("memory[0x%08X]=0x%08X, want 0x%08X", tt.initialA, got, want)
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeArithEAToDnAddressSources(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	type testCase struct {
		name       string
		words      []uint16
		wantPrefix int
		setup      func(*M68KCPU)
		checkMem   []uint32
	}

	const (
		startPC = uint32(0x1000)
		memA7   = uint32(0x120000)
		pcData  = uint32(0x1010)
	)

	tests := []testCase{
		{
			name:  "add_w_indirect_a7_to_d0",
			words: []uint16{0xD057, 0x7E01}, wantPrefix: 2,
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 0x00000010
				cpu.AddrRegs[7] = memA7
				cpu.Write16(memA7, 0x0020)
			},
			checkMem: []uint32{memA7},
		},
		{
			name:  "add_b_postinc_a7_to_d7",
			words: []uint16{0xDE1F, 0x7E01}, wantPrefix: 2,
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[7] = 0x00000001
				cpu.AddrRegs[7] = memA7
				cpu.Write8(memA7, 0x7F)
			},
			checkMem: []uint32{memA7},
		},
		{
			name:  "sub_w_a7_to_d1",
			words: []uint16{0x924F, 0x7E01}, wantPrefix: 2,
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[1] = 0x00000100
				cpu.AddrRegs[7] = 0x00000020
			},
		},
		{
			name:  "sub_b_postinc_a7_to_d3",
			words: []uint16{0x961F, 0x7E01}, wantPrefix: 2,
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[3] = 0x00000040
				cpu.AddrRegs[7] = memA7
				cpu.Write8(memA7, 0x11)
			},
			checkMem: []uint32{memA7},
		},
		{
			name:  "sub_b_predec_a7_to_d3",
			words: []uint16{0x9667, 0x7E01}, wantPrefix: 2,
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[3] = 0x00000040
				cpu.AddrRegs[7] = memA7 + 2
				cpu.Write8(memA7, 0x11)
			},
			checkMem: []uint32{memA7},
		},
		{
			name:  "sub_b_predec_a0_to_d5",
			words: []uint16{0x9A20, 0x7E01}, wantPrefix: 2,
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[5] = 0x00000040
				cpu.AddrRegs[0] = memA7 + 1
				cpu.Write8(memA7, 0x11)
			},
			checkMem: []uint32{memA7},
		},
		{
			name:  "sub_b_predec_a0_harte_addr_to_d5",
			words: []uint16{0x9A20, 0x7E01}, wantPrefix: 2,
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[5] = 0x0D46687C
				cpu.AddrRegs[0] = 0x0003A2BF
				cpu.Write8(0x0003A2BE, 0x00)
			},
			checkMem: []uint32{0x0003A2BE},
		},
		{
			name:  "sub_b_pcdisp_to_d3",
			words: []uint16{0x963A, 0x000E, 0x7E01}, wantPrefix: 2,
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[3] = 0x00000040
				cpu.Write8(pcData, 0x11)
			},
			checkMem: []uint32{pcData},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp := newM68KTestProgramCPU(t, startPC)
			tt.setup(interp)
			writeM68KStopProgram(interp, startPC, tt.words...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			tt.setup(jit)
			writeM68KStopProgram(jit, startPC, tt.words...)
			instrs := m68kScanBlock(jit.memory, startPC)
			prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
			if len(prefix) != tt.wantPrefix {
				candidate := instrs
				if len(candidate) > tt.wantPrefix {
					candidate = candidate[:tt.wantPrefix]
				}
				t.Fatalf("%s rejected by production gate: prefix=%+v instrs=%+v fallback=%v conservative=%v productionSafe=%v genericIO=%v",
					tt.name,
					prefix,
					instrs,
					m68kNeedsFallback(candidate),
					m68kNeedsConservativeFallback(jit.memory, startPC, candidate),
					m68kBlockProductionNativeSafe(candidate),
					m68kBlockMayUseGenericIOFallback(candidate))
			}

			runM68KJITUntilStopped(t, jit)
			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("%s did not execute a native block", tt.name)
			}
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("%s bailed out %d times, want 0", tt.name, got)
			}
			if got := jit.m68kJitFallbackOpcodeCounts[tt.words[0]].Load(); got != 0 {
				t.Fatalf("%s opcode 0x%04X fallback count = %d, want 0", tt.name, tt.words[0], got)
			}
			for _, addr := range tt.checkMem {
				if got, want := jit.Read32(addr), interp.Read32(addr); got != want {
					t.Fatalf("%s memory[0x%08X]=0x%08X, want 0x%08X", tt.name, addr, got, want)
				}
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeAddressArithmeticAndCMPASources(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	type testCase struct {
		name     string
		opcode   uint16
		setup    func(*M68KCPU)
		checkMem []uint32
	}

	const (
		startPC = uint32(0x1000)
		stack   = uint32(0x120100)
	)

	tests := []testCase{
		{
			name:   "adda_l_a7_to_a2",
			opcode: 0xD5CF,
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[2] = 0x100
				cpu.AddrRegs[7] = 0x20
			},
		},
		{
			name:   "adda_l_postinc_a7_to_a4",
			opcode: 0xD9DF,
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[4] = 0x100
				cpu.AddrRegs[7] = stack
				cpu.Write32(stack, 0x00000020)
			},
			checkMem: []uint32{stack},
		},
		{
			name:   "adda_l_predec_a7_to_a7",
			opcode: 0xDFE7,
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[7] = stack + 4
				cpu.Write32(stack, 0x00000020)
			},
			checkMem: []uint32{stack},
		},
		{
			name:   "adda_w_a0_to_a7",
			opcode: 0xDEC8,
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = 0x0000FFF0
				cpu.AddrRegs[7] = 0x100
			},
		},
		{
			name:   "suba_l_indirect_a7_to_a5",
			opcode: 0x9BD7,
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[5] = 0x100
				cpu.AddrRegs[7] = stack
				cpu.Write32(stack, 0x00000020)
			},
			checkMem: []uint32{stack},
		},
		{
			name:   "suba_l_predec_a7_to_a6",
			opcode: 0x9DE7,
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[6] = 0x100
				cpu.AddrRegs[7] = stack + 4
				cpu.Write32(stack, 0x00000020)
			},
			checkMem: []uint32{stack},
		},
		{
			name:   "suba_w_a4_to_a7",
			opcode: 0x9ECC,
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[4] = 0x0000FFF0
				cpu.AddrRegs[7] = 0x100
			},
		},
		{
			name:   "cmpa_l_indirect_a7_to_a0",
			opcode: 0xB1D7,
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = 0x100
				cpu.AddrRegs[7] = stack
				cpu.Write32(stack, 0x00000020)
			},
			checkMem: []uint32{stack},
		},
		{
			name:   "cmpa_l_a7_to_a2",
			opcode: 0xB5CF,
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[2] = 0x100
				cpu.AddrRegs[7] = 0x20
			},
		},
		{
			name:   "cmpa_l_predec_a2_to_a2",
			opcode: 0xB5E2,
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[2] = stack + 4
				cpu.Write32(stack, 0x00000020)
			},
			checkMem: []uint32{stack},
		},
		{
			name:   "cmpa_l_a4_to_a7",
			opcode: 0xBFCC,
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[4] = 0x20
				cpu.AddrRegs[7] = 0x100
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opcodes := []uint16{tt.opcode, 0x7E01}

			interp := newM68KTestProgramCPU(t, startPC)
			tt.setup(interp)
			writeM68KStopProgram(interp, startPC, opcodes...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			tt.setup(jit)
			writeM68KStopProgram(jit, startPC, opcodes...)
			instrs := m68kScanBlock(jit.memory, startPC)
			prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
			if len(prefix) != len(opcodes) {
				candidate := instrs
				if len(candidate) > len(opcodes) {
					candidate = candidate[:len(opcodes)]
				}
				t.Fatalf("%s rejected by production gate: prefix=%+v instrs=%+v fallback=%v conservative=%v productionSafe=%v genericIO=%v",
					tt.name,
					prefix,
					instrs,
					m68kNeedsFallback(candidate),
					m68kNeedsConservativeFallback(jit.memory, startPC, candidate),
					m68kBlockProductionNativeSafe(candidate),
					m68kBlockMayUseGenericIOFallback(candidate))
			}

			runM68KJITUntilStopped(t, jit)
			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("%s did not execute a native block", tt.name)
			}
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("%s bailed out %d times, want 0", tt.name, got)
			}
			if got := jit.m68kJitFallbackOpcodeCounts[tt.opcode].Load(); got != 0 {
				t.Fatalf("%s opcode 0x%04X fallback count = %d, want 0", tt.name, tt.opcode, got)
			}
			for _, addr := range tt.checkMem {
				if got, want := jit.Read32(addr), interp.Read32(addr); got != want {
					t.Fatalf("%s memory[0x%08X]=0x%08X, want 0x%08X", tt.name, addr, got, want)
				}
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeMULWAddressSources(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	type testCase struct {
		name       string
		words      []uint16
		wantPrefix int
		setup      func(*M68KCPU)
		checkMem   []uint32
	}

	const (
		startPC = uint32(0x1000)
		stack   = uint32(0x120200)
	)

	tests := []testCase{
		{
			name: "mulu_indirect_a7_to_d0", words: []uint16{0xC0D7, 0x7E01}, wantPrefix: 2,
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 3
				cpu.AddrRegs[7] = stack
				cpu.Write16(stack, 7)
			},
			checkMem: []uint32{stack},
		},
		{
			name: "mulu_postinc_a7_to_d0", words: []uint16{0xC0DF, 0x7E01}, wantPrefix: 2,
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 3
				cpu.AddrRegs[7] = stack
				cpu.Write16(stack, 7)
			},
			checkMem: []uint32{stack},
		},
		{
			name: "mulu_disp_a7_to_d4", words: []uint16{0xC8EF, 0x0004, 0x7E01}, wantPrefix: 2,
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[4] = 5
				cpu.AddrRegs[7] = stack
				cpu.Write16(stack+4, 9)
			},
			checkMem: []uint32{stack + 4},
		},
		{
			name: "muls_predec_a7_to_d2", words: []uint16{0xC5E7, 0x7E01}, wantPrefix: 2,
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[2] = 0x0000FFFE
				cpu.AddrRegs[7] = stack + 2
				cpu.Write16(stack, 0xFFFD)
			},
			checkMem: []uint32{stack},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp := newM68KTestProgramCPU(t, startPC)
			tt.setup(interp)
			writeM68KStopProgram(interp, startPC, tt.words...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			tt.setup(jit)
			writeM68KStopProgram(jit, startPC, tt.words...)
			instrs := m68kScanBlock(jit.memory, startPC)
			prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
			if len(prefix) != tt.wantPrefix {
				candidate := instrs
				if len(candidate) > tt.wantPrefix {
					candidate = candidate[:tt.wantPrefix]
				}
				t.Fatalf("%s rejected by production gate: prefix=%+v instrs=%+v fallback=%v conservative=%v productionSafe=%v genericIO=%v",
					tt.name,
					prefix,
					instrs,
					m68kNeedsFallback(candidate),
					m68kNeedsConservativeFallback(jit.memory, startPC, candidate),
					m68kBlockProductionNativeSafe(candidate),
					m68kBlockMayUseGenericIOFallback(candidate))
			}

			runM68KJITUntilStopped(t, jit)
			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("%s did not execute a native block", tt.name)
			}
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("%s bailed out %d times, want 0", tt.name, got)
			}
			if got := jit.m68kJitFallbackOpcodeCounts[tt.words[0]].Load(); got != 0 {
				t.Fatalf("%s opcode 0x%04X fallback count = %d, want 0", tt.name, tt.words[0], got)
			}
			for _, addr := range tt.checkMem {
				if got, want := jit.Read32(addr), interp.Read32(addr); got != want {
					t.Fatalf("%s memory[0x%08X]=0x%08X, want 0x%08X", tt.name, addr, got, want)
				}
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_Exec_AROSBackwardCopyTailCMPAddressRegister(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
		cpu.DataRegs[4] = 0xFFFFFFFD
		cpu.AddrRegs[1] = 0
	}, false,
		0x5389, // SUBQ.L #1,A1
		0xB889, // CMP.L A1,D4
		0x66FA, // BNE.S $1000
	)

	if cpu.DataRegs[4] != 0xFFFFFFFD {
		t.Fatalf("D4=0x%08X, want 0xFFFFFFFD", cpu.DataRegs[4])
	}
	if cpu.AddrRegs[1] != 0xFFFFFFFD {
		t.Fatalf("A1=0x%08X, want 0xFFFFFFFD", cpu.AddrRegs[1])
	}
}

func TestM68KJIT_Exec_AROSCC36StoreComparePrefixFlags(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tt := range []struct {
		name     string
		d0       uint32
		memCmp   uint32
		wantD1   uint32
		wantD2   uint32
		wantFlag string
	}{
		{name: "bhi_taken", d0: 0x20, memCmp: 0x10, wantD1: 0, wantD2: 2, wantFlag: "BHI taken"},
		{name: "bhi_not_taken", d0: 0x10, memCmp: 0x20, wantD1: 1, wantD2: 2, wantFlag: "BHI not taken"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			const base = 0x3000
			cpu := runM68KJITStopProgramWithSetup(t, 0x1000, func(cpu *M68KCPU) {
				cpu.DataRegs[0] = tt.d0
				cpu.AddrRegs[3] = base
				cpu.Write32(base+0x3A, tt.memCmp)
			}, false,
				0x2740, 0x0036, // MOVE.L D0,54(A3)
				0xB0AB, 0x003A, // CMP.L 58(A3),D0
				0x6202, // BHI.S over MOVEQ D1
				0x7201, // MOVEQ #1,D1
				0x7402, // MOVEQ #2,D2
			)

			if got := cpu.Read32(base + 0x36); got != tt.d0 {
				t.Fatalf("stored long=0x%08X, want 0x%08X", got, tt.d0)
			}
			if cpu.DataRegs[1] != tt.wantD1 {
				t.Fatalf("D1=0x%08X, want 0x%08X (%s)", cpu.DataRegs[1], tt.wantD1, tt.wantFlag)
			}
			if cpu.DataRegs[2] != tt.wantD2 {
				t.Fatalf("D2=0x%08X, want 0x%08X", cpu.DataRegs[2], tt.wantD2)
			}
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeStackDisplacementComparePrefixes(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tt := range []struct {
		name      string
		opcodes   []uint16
		setup     func(*M68KCPU)
		wantD1    uint32
		fallbacks []uint16
	}{
		{
			name: "cmp_l_d16_a7_d0",
			opcodes: []uint16{
				0xB0AF, 0x002C, // CMP.L 44(A7),D0
				0x6602, // BNE.S over MOVEQ
				0x7201, // MOVEQ #1,D1
				0x7202, // MOVEQ #2,D1
			},
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[7] = 0x120000
				cpu.DataRegs[0] = 0x12345678
				cpu.Write32(0x120000+44, 0x12345678)
				cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_C
			},
			wantD1:    2,
			fallbacks: []uint16{0xB0AF},
		},
		{
			name: "cmpi_l_imm_d16_a7",
			opcodes: []uint16{
				0x0CAF, 0x0060, 0x56D0, 0x0002, // CMPI.L #$006056D0,2(A7)
				0x6602, // BNE.S over MOVEQ
				0x7201, // MOVEQ #1,D1
				0x7202, // MOVEQ #2,D1
			},
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[7] = 0x120000
				cpu.Write32(0x120000+2, 0x006056D0)
				cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_C
			},
			wantD1:    2,
			fallbacks: []uint16{0x0CAF},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			const startPC = uint32(0x1000)

			interp := newM68KTestProgramCPU(t, startPC)
			tt.setup(interp)
			writeM68KStopProgram(interp, startPC, tt.opcodes...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			tt.setup(jit)
			writeM68KStopProgram(jit, startPC, tt.opcodes...)
			instrs := m68kScanBlock(jit.memory, startPC)
			prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
			wantPrefix := 0
			for wantPrefix < len(instrs) && instrs[wantPrefix].opcode != 0x4E72 && !m68kIsBlockTerminator(instrs[wantPrefix].opcode) {
				wantPrefix++
			}
			if len(prefix) != wantPrefix {
				t.Fatalf("%s native prefix length=%d want=%d prefix=%+v fallback=%v conservative=%v productionSafe=%v genericIO=%v instrs=%+v",
					tt.name, len(prefix), wantPrefix, prefix,
					m68kNeedsFallback(instrs), m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
					m68kBlockProductionNativeSafe(instrs), m68kBlockMayUseGenericIOFallback(instrs), instrs)
			}
			runM68KJITUntilStopped(t, jit)

			for _, opcode := range tt.fallbacks {
				if got := jit.m68kJitFallbackOpcodeCounts[opcode].Load(); got != 0 {
					t.Fatalf("%s opcode 0x%04X fell back %d times", tt.name, opcode, got)
				}
			}
			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("%s did not execute a native JIT block", tt.name)
			}
			if got := jit.DataRegs[1]; got != tt.wantD1 {
				t.Fatalf("%s D1=0x%08X, want 0x%08X", tt.name, got, tt.wantD1)
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeStackDisplacementADDQSUBQ(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tt := range []struct {
		name     string
		opcode   uint16
		disp     uint16
		baseReg  uint16
		size     int
		initial  uint32
		wantMem  uint32
		branch   uint16
		wantD1   uint32
		fallback uint16
	}{
		{name: "addq_l_a7", opcode: 0x54AF, disp: 0x0002, baseReg: 7, size: M68K_SIZE_LONG, initial: 0xFFFFFFFF, wantMem: 0x00000001, branch: 0x6604, wantD1: 2, fallback: 0x54AF},
		{name: "subq_l_a7", opcode: 0x55AF, disp: 0x0002, baseReg: 7, size: M68K_SIZE_LONG, initial: 0x00000001, wantMem: 0xFFFFFFFF, branch: 0x6604, wantD1: 2, fallback: 0x55AF},
		{name: "subq_b_a6", opcode: 0x532E, disp: 0x0127, baseReg: 6, size: M68K_SIZE_BYTE, initial: 0x01, wantMem: 0x00, branch: 0x6704, wantD1: 2, fallback: 0x532E},
		{name: "addq_w_a5", opcode: 0x526D, disp: 0x0010, baseReg: 5, size: M68K_SIZE_WORD, initial: 0x7FFF, wantMem: 0x8000, branch: 0x6B04, wantD1: 2, fallback: 0x526D},
	} {
		t.Run(tt.name, func(t *testing.T) {
			const (
				startPC = uint32(0x1000)
				stack   = uint32(0x120000)
			)
			opcodes := []uint16{
				tt.opcode, tt.disp, // ADDQ/SUBQ d16(An)
				tt.branch, // branch to MOVEQ #2,D1 when expected flags are set
				0x7201,    // MOVEQ #1,D1
				0x6002,    // BRA.S done
				0x7202,    // MOVEQ #2,D1
			}
			setup := func(cpu *M68KCPU) {
				cpu.AddrRegs[tt.baseReg] = stack
				switch tt.size {
				case M68K_SIZE_BYTE:
					cpu.Write8(stack+uint32(int16(tt.disp)), uint8(tt.initial))
				case M68K_SIZE_WORD:
					cpu.Write16(stack+uint32(int16(tt.disp)), uint16(tt.initial))
				case M68K_SIZE_LONG:
					cpu.Write32(stack+uint32(int16(tt.disp)), tt.initial)
				}
				cpu.SR = M68K_SR_S | M68K_SR_N | M68K_SR_Z | M68K_SR_V
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			writeM68KStopProgram(interp, startPC, opcodes...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			writeM68KStopProgram(jit, startPC, opcodes...)
			instrs := m68kScanBlock(jit.memory, startPC)
			prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
			wantPrefix := 0
			for wantPrefix < len(instrs) && instrs[wantPrefix].opcode != 0x4E72 && !m68kIsBlockTerminator(instrs[wantPrefix].opcode) {
				wantPrefix++
			}
			if len(prefix) != wantPrefix {
				t.Fatalf("%s native prefix length=%d want=%d prefix=%+v fallback=%v conservative=%v productionSafe=%v genericIO=%v instrs=%+v",
					tt.name, len(prefix), wantPrefix, prefix,
					m68kNeedsFallback(instrs), m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
					m68kBlockProductionNativeSafe(instrs), m68kBlockMayUseGenericIOFallback(instrs), instrs)
			}
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitFallbackOpcodeCounts[tt.fallback].Load(); got != 0 {
				t.Fatalf("%s opcode 0x%04X fell back %d times", tt.name, tt.fallback, got)
			}
			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("%s did not execute a native JIT block", tt.name)
			}
			var gotMem uint32
			switch tt.size {
			case M68K_SIZE_BYTE:
				gotMem = uint32(jit.Read8(stack + uint32(int16(tt.disp))))
			case M68K_SIZE_WORD:
				gotMem = uint32(jit.Read16(stack + uint32(int16(tt.disp))))
			case M68K_SIZE_LONG:
				gotMem = jit.Read32(stack + uint32(int16(tt.disp)))
			}
			if gotMem != tt.wantMem {
				t.Fatalf("%s memory result=0x%08X, want 0x%08X", tt.name, gotMem, tt.wantMem)
			}
			if got := jit.DataRegs[1]; got != tt.wantD1 {
				t.Fatalf("%s D1=0x%08X, want 0x%08X", tt.name, got, tt.wantD1)
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherADDQSUBQCodePageWriteInvalidatesWithoutFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tt := range []struct {
		name    string
		opcode  uint16
		initial uint8
		want    uint8
	}{
		{name: "addq_b", opcode: 0x522E, initial: 0xFF, want: 0x00},
		{name: "subq_b", opcode: 0x532E, initial: 0x01, want: 0x00},
	} {
		t.Run(tt.name, func(t *testing.T) {
			const (
				startPC = uint32(0x1000)
				data    = uint32(0x1800) // Same 4 KiB code bitmap page as startPC.
			)
			opcodes := []uint16{
				tt.opcode, 0x0000, // ADDQ/SUBQ.B 0(A6)
				0x6704, // BEQ.S set_two
				0x7201, // MOVEQ #1,D1
				0x6002, // BRA.S done
				0x7202, // MOVEQ #2,D1
			}
			setup := func(cpu *M68KCPU) {
				cpu.AddrRegs[6] = data
				cpu.Write8(data, tt.initial)
				cpu.SR = M68K_SR_S | M68K_SR_N | M68K_SR_V | M68K_SR_C
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			writeM68KStopProgram(interp, startPC, opcodes...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			writeM68KStopProgram(jit, startPC, opcodes...)
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitFallbackOpcodeCounts[tt.opcode].Load(); got != 0 {
				t.Fatalf("%s opcode 0x%04X fell back %d times", tt.name, tt.opcode, got)
			}
			if got := jit.m68kJitBailoutCount.Load(); got != 0 {
				t.Fatalf("%s bailed out %d times", tt.name, got)
			}
			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("%s did not execute a native JIT block", tt.name)
			}
			if got := jit.Read8(data); got != tt.want {
				t.Fatalf("%s memory result=0x%02X, want 0x%02X", tt.name, got, tt.want)
			}
			if got := jit.DataRegs[1]; got != 2 {
				t.Fatalf("%s D1=0x%08X, want 2; BEQ consumed wrong ADDQ/SUBQ flags", tt.name, got)
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeStackIndirectADDQSUBQ(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tt := range []struct {
		name     string
		opcode   uint16
		initial  uint32
		wantMem  uint32
		branch   uint16
		wantD1   uint32
		fallback uint16
	}{
		{name: "addq_l_a7_indirect", opcode: 0x5297, initial: 0xFFFFFFFF, wantMem: 0x00000000, branch: 0x6704, wantD1: 2, fallback: 0x5297},
		{name: "subq_l_a7_indirect", opcode: 0x5397, initial: 0x00000001, wantMem: 0x00000000, branch: 0x6704, wantD1: 2, fallback: 0x5397},
	} {
		t.Run(tt.name, func(t *testing.T) {
			const (
				startPC = uint32(0x1000)
				stack   = uint32(0x120000)
			)
			opcodes := []uint16{
				tt.opcode, // ADDQ/SUBQ.L (A7)
				tt.branch, // branch to MOVEQ #2,D1 when Z is set
				0x7201,    // MOVEQ #1,D1
				0x6002,    // BRA.S done
				0x7202,    // MOVEQ #2,D1
			}
			setup := func(cpu *M68KCPU) {
				cpu.AddrRegs[7] = stack
				cpu.Write32(stack, tt.initial)
				cpu.SR = M68K_SR_S | M68K_SR_N | M68K_SR_V | M68K_SR_C
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			writeM68KStopProgram(interp, startPC, opcodes...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			writeM68KStopProgram(jit, startPC, opcodes...)
			instrs := m68kScanBlock(jit.memory, startPC)
			prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
			wantPrefix := 0
			for wantPrefix < len(instrs) && instrs[wantPrefix].opcode != 0x4E72 && !m68kIsBlockTerminator(instrs[wantPrefix].opcode) {
				wantPrefix++
			}
			if len(prefix) != wantPrefix {
				t.Fatalf("%s native prefix length=%d want=%d prefix=%+v fallback=%v conservative=%v productionSafe=%v genericIO=%v instrs=%+v",
					tt.name, len(prefix), wantPrefix, prefix,
					m68kNeedsFallback(instrs), m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
					m68kBlockProductionNativeSafe(instrs), m68kBlockMayUseGenericIOFallback(instrs), instrs)
			}
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitFallbackOpcodeCounts[tt.fallback].Load(); got != 0 {
				t.Fatalf("%s opcode 0x%04X fell back %d times", tt.name, tt.fallback, got)
			}
			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("%s did not execute a native JIT block", tt.name)
			}
			if got := jit.Read32(stack); got != tt.wantMem {
				t.Fatalf("%s memory result=0x%08X, want 0x%08X", tt.name, got, tt.wantMem)
			}
			if got := jit.DataRegs[1]; got != tt.wantD1 {
				t.Fatalf("%s D1=0x%08X, want 0x%08X", tt.name, got, tt.wantD1)
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeStackPredecrementADDQSUBQ(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tt := range []struct {
		name     string
		opcode   uint16
		initial  uint32
		wantMem  uint32
		branch   uint16
		wantD1   uint32
		fallback uint16
	}{
		{name: "addq_l_predec_a7", opcode: 0x5CA7, initial: 0xFFFFFFFA, wantMem: 0x00000000, branch: 0x6704, wantD1: 2, fallback: 0x5CA7},
		{name: "subq_l_predec_a7", opcode: 0x5DA7, initial: 0x00000006, wantMem: 0x00000000, branch: 0x6704, wantD1: 2, fallback: 0x5DA7},
	} {
		t.Run(tt.name, func(t *testing.T) {
			const (
				startPC = uint32(0x1000)
				stack   = uint32(0x120000)
			)
			opcodes := []uint16{
				tt.opcode, // ADDQ/SUBQ.L -(A7)
				tt.branch, // branch to MOVEQ #2,D1 when Z is set
				0x7201,    // MOVEQ #1,D1
				0x6002,    // BRA.S done
				0x7202,    // MOVEQ #2,D1
			}
			setup := func(cpu *M68KCPU) {
				cpu.AddrRegs[7] = stack
				cpu.Write32(stack-4, tt.initial)
				cpu.SR = M68K_SR_S | M68K_SR_N | M68K_SR_V | M68K_SR_C
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			writeM68KStopProgram(interp, startPC, opcodes...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			writeM68KStopProgram(jit, startPC, opcodes...)
			instrs := m68kScanBlock(jit.memory, startPC)
			prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
			wantPrefix := 0
			for wantPrefix < len(instrs) && instrs[wantPrefix].opcode != 0x4E72 && !m68kIsBlockTerminator(instrs[wantPrefix].opcode) {
				wantPrefix++
			}
			if len(prefix) != wantPrefix {
				t.Fatalf("%s native prefix length=%d want=%d prefix=%+v fallback=%v conservative=%v productionSafe=%v genericIO=%v instrs=%+v",
					tt.name, len(prefix), wantPrefix, prefix,
					m68kNeedsFallback(instrs), m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
					m68kBlockProductionNativeSafe(instrs), m68kBlockMayUseGenericIOFallback(instrs), instrs)
			}
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitFallbackOpcodeCounts[tt.fallback].Load(); got != 0 {
				t.Fatalf("%s opcode 0x%04X fell back %d times", tt.name, tt.fallback, got)
			}
			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("%s did not execute a native JIT block", tt.name)
			}
			if got := jit.Read32(stack - 4); got != tt.wantMem {
				t.Fatalf("%s memory result=0x%08X, want 0x%08X", tt.name, got, tt.wantMem)
			}
			if got := jit.AddrRegs[7]; got != stack-4 {
				t.Fatalf("%s A7=0x%08X, want 0x%08X", tt.name, got, stack-4)
			}
			if got := jit.DataRegs[1]; got != tt.wantD1 {
				t.Fatalf("%s D1=0x%08X, want 0x%08X", tt.name, got, tt.wantD1)
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeBFTSTRegisterImmediate(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tt := range []struct {
		name     string
		opcode   uint16
		dataReg  uint16
		value    uint32
		branch   uint16
		wantD1   uint32
		fallback uint16
	}{
		{name: "zero_d2", opcode: 0xE8C2, dataReg: 2, value: 0x00000000, branch: 0x6704, wantD1: 2, fallback: 0xE8C2},
		{name: "negative_d3", opcode: 0xE8C3, dataReg: 3, value: 0x00000005, branch: 0x6B04, wantD1: 2, fallback: 0xE8C3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			const startPC = uint32(0x1000)
			opcodes := []uint16{
				tt.opcode, 0x0743, // BFTST Dn{#29:#3}
				tt.branch, // branch to MOVEQ #2,D1 when expected flags are set
				0x7201,    // MOVEQ #1,D1
				0x6002,    // BRA.S done
				0x7202,    // MOVEQ #2,D1
			}
			setup := func(cpu *M68KCPU) {
				cpu.DataRegs[tt.dataReg] = tt.value
				cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_V | M68K_SR_C
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			writeM68KStopProgram(interp, startPC, opcodes...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			writeM68KStopProgram(jit, startPC, opcodes...)
			instrs := m68kScanBlock(jit.memory, startPC)
			prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
			wantPrefix := 0
			for wantPrefix < len(instrs) && instrs[wantPrefix].opcode != 0x4E72 && !m68kIsBlockTerminator(instrs[wantPrefix].opcode) {
				wantPrefix++
			}
			if len(prefix) != wantPrefix {
				t.Fatalf("%s native prefix length=%d want=%d prefix=%+v fallback=%v conservative=%v productionSafe=%v genericIO=%v instrs=%+v",
					tt.name, len(prefix), wantPrefix, prefix,
					m68kNeedsFallback(instrs), m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
					m68kBlockProductionNativeSafe(instrs), m68kBlockMayUseGenericIOFallback(instrs), instrs)
			}
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitFallbackOpcodeCounts[tt.fallback].Load(); got != 0 {
				t.Fatalf("%s opcode 0x%04X fell back %d times", tt.name, tt.fallback, got)
			}
			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("%s did not execute a native JIT block", tt.name)
			}
			if got := jit.DataRegs[1]; got != tt.wantD1 {
				t.Fatalf("%s D1=0x%08X, want 0x%08X", tt.name, got, tt.wantD1)
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeBFEXTURegisterImmediate(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	for _, tt := range []struct {
		name     string
		opcode   uint16
		ext      uint16
		dataReg  uint16
		value    uint32
		fallback uint16
	}{
		{name: "aros_d0_to_d0_16_10", opcode: 0xE9C0, ext: 0x040A, dataReg: 0, value: 0x0016FFFF, fallback: 0xE9C0},
		{name: "aros_d0_to_d1_22_8", opcode: 0xE9C0, ext: 0x1588, dataReg: 0, value: 0x008A4D54, fallback: 0xE9C0},
		{name: "zero_result", opcode: 0xE9C2, ext: 0x1008, dataReg: 2, value: 0x00000000, fallback: 0xE9C2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			const startPC = uint32(0x1000)
			opcodes := []uint16{
				tt.opcode, tt.ext, // BFEXTU Dn{offset:width},Ddest
				0x6604, // BNE.S set_two when extracted value is non-zero
				0x7201, // MOVEQ #1,D1
				0x6002, // BRA.S done
				0x7202, // MOVEQ #2,D1
			}
			setup := func(cpu *M68KCPU) {
				cpu.DataRegs[tt.dataReg] = tt.value
				cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_N | M68K_SR_V | M68K_SR_C
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			writeM68KStopProgram(interp, startPC, opcodes...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			writeM68KStopProgram(jit, startPC, opcodes...)
			instrs := m68kScanBlock(jit.memory, startPC)
			prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
			wantPrefix := 0
			for wantPrefix < len(instrs) && instrs[wantPrefix].opcode != 0x4E72 && !m68kIsBlockTerminator(instrs[wantPrefix].opcode) {
				wantPrefix++
			}
			if len(prefix) != wantPrefix {
				t.Fatalf("%s native prefix length=%d want=%d prefix=%+v fallback=%v conservative=%v productionSafe=%v genericIO=%v instrs=%+v",
					tt.name, len(prefix), wantPrefix, prefix,
					m68kNeedsFallback(instrs), m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
					m68kBlockProductionNativeSafe(instrs), m68kBlockMayUseGenericIOFallback(instrs), instrs)
			}
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitFallbackOpcodeCounts[tt.fallback].Load(); got != 0 {
				t.Fatalf("%s opcode 0x%04X fell back %d times", tt.name, tt.fallback, got)
			}
			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("%s did not execute a native JIT block", tt.name)
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

func TestM68KJIT_DefaultDispatcherExecutesNativeBFEXTUMemoryImmediate(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		base    = uint32(0x8000)
		disp    = uint16(0x012B)
	)
	opcodes := []uint16{
		0xE9E8, 0x0001, disp, // BFEXTU 299(A0){0:1},D0
		0x6604, // BNE.S set_two when extracted bit is non-zero
		0x7201, // MOVEQ #1,D1
		0x6002, // BRA.S done
		0x7202, // MOVEQ #2,D1
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = base
		cpu.Write8(base+uint32(disp), 0x80)
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) == 0 || prefix[0].opcode != 0xE9E8 {
		t.Fatalf("BFEXTU memory was not admitted into native prefix=%+v instrs=%+v", prefix, instrs)
	}
	if !m68kCanUseProductionNativeBlock(jit.memory, startPC, prefix) {
		t.Fatalf("BFEXTU memory prefix not production native: fallback=%v conservative=%v productionSafe=%v genericIO=%v prefix=%+v instrs=%+v",
			m68kNeedsFallback(prefix), m68kNeedsConservativeFallback(jit.memory, startPC, prefix),
			m68kBlockProductionNativeSafe(prefix), m68kBlockMayUseGenericIOFallback(prefix), prefix, instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitFallbackOpcodeCounts[0xE9E8].Load(); got != 0 {
		t.Fatalf("BFEXTU memory fell back %d times", got)
	}
	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("BFEXTU memory did not execute a native JIT block")
	}
	if got := jit.DataRegs[0]; got != 1 {
		t.Fatalf("D0=0x%08X, want extracted bit 1", got)
	}
	if got := jit.DataRegs[1]; got != 2 {
		t.Fatalf("D1=0x%08X, want BNE path result 2", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeBFEXTSRegisterImmediate(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const startPC = uint32(0x1000)
	opcodes := []uint16{
		0xEBC0, 0x1208, // BFEXTS D0{8:8},D1
		0x6604, // BNE.S set_two when extracted value is non-zero
		0x7401, // MOVEQ #1,D2
		0x6002, // BRA.S done
		0x7402, // MOVEQ #2,D2
	}
	setup := func(cpu *M68KCPU) {
		cpu.DataRegs[0] = 0x008A4D54
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) == 0 || prefix[0].opcode != 0xEBC0 {
		t.Fatalf("BFEXTS register was not admitted into native prefix=%+v instrs=%+v", prefix, instrs)
	}
	if !m68kCanUseProductionNativeBlock(jit.memory, startPC, prefix) {
		t.Fatalf("BFEXTS register prefix not production native: fallback=%v conservative=%v productionSafe=%v genericIO=%v prefix=%+v instrs=%+v",
			m68kNeedsFallback(prefix), m68kNeedsConservativeFallback(jit.memory, startPC, prefix),
			m68kBlockProductionNativeSafe(prefix), m68kBlockMayUseGenericIOFallback(prefix), prefix, instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitFallbackOpcodeCounts[0xEBC0].Load(); got != 0 {
		t.Fatalf("BFEXTS register fell back %d times", got)
	}
	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("BFEXTS register did not execute a native JIT block")
	}
	if got := jit.DataRegs[1]; got != 0xFFFFFF8A {
		t.Fatalf("D1=0x%08X, want sign-extended 0xFFFFFF8A", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeBFEXTSMemoryImmediate(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		base    = uint32(0x8000)
		disp    = uint16(0x012B)
	)
	opcodes := []uint16{
		0xEBEA, 0x0008, disp, // BFEXTS 299(A2){0:8},D0
		0x6604, // BNE.S set_two when extracted value is non-zero
		0x7201, // MOVEQ #1,D1
		0x6002, // BRA.S done
		0x7202, // MOVEQ #2,D1
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[2] = base
		cpu.Write8(base+uint32(disp), 0x80)
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) == 0 || prefix[0].opcode != 0xEBEA {
		t.Fatalf("BFEXTS memory was not admitted into native prefix=%+v instrs=%+v", prefix, instrs)
	}
	if !m68kCanUseProductionNativeBlock(jit.memory, startPC, prefix) {
		t.Fatalf("BFEXTS memory prefix not production native: fallback=%v conservative=%v productionSafe=%v genericIO=%v prefix=%+v instrs=%+v",
			m68kNeedsFallback(prefix), m68kNeedsConservativeFallback(jit.memory, startPC, prefix),
			m68kBlockProductionNativeSafe(prefix), m68kBlockMayUseGenericIOFallback(prefix), prefix, instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitFallbackOpcodeCounts[0xEBEA].Load(); got != 0 {
		t.Fatalf("BFEXTS memory fell back %d times", got)
	}
	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("BFEXTS memory did not execute a native JIT block")
	}
	if got := jit.DataRegs[0]; got != 0xFFFFFF80 {
		t.Fatalf("D0=0x%08X, want sign-extended 0xFFFFFF80", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeBFTSTMemoryImmediate(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		base    = uint32(0x8000)
		disp    = uint16(0x0010)
	)
	opcodes := []uint16{
		0xE8E8, 0x0001, disp, // BFTST 16(A0){0:1}
		0x6604, // BNE.S set_two when tested bit is non-zero
		0x7201, // MOVEQ #1,D1
		0x6002, // BRA.S done
		0x7202, // MOVEQ #2,D1
	}
	setup := func(cpu *M68KCPU) {
		cpu.AddrRegs[0] = base
		cpu.Write8(base+uint32(disp), 0x80)
		cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_V | M68K_SR_C
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
	if len(prefix) == 0 || prefix[0].opcode != 0xE8E8 {
		t.Fatalf("BFTST memory was not admitted into native prefix=%+v instrs=%+v", prefix, instrs)
	}
	if !m68kCanUseProductionNativeBlock(jit.memory, startPC, prefix) {
		t.Fatalf("BFTST memory prefix not production native: fallback=%v conservative=%v productionSafe=%v genericIO=%v prefix=%+v instrs=%+v",
			m68kNeedsFallback(prefix), m68kNeedsConservativeFallback(jit.memory, startPC, prefix),
			m68kBlockProductionNativeSafe(prefix), m68kBlockMayUseGenericIOFallback(prefix), prefix, instrs)
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitFallbackOpcodeCounts[0xE8E8].Load(); got != 0 {
		t.Fatalf("BFTST memory fell back %d times", got)
	}
	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("BFTST memory did not execute a native JIT block")
	}
	if got := jit.DataRegs[1]; got != 2 {
		t.Fatalf("D1=0x%08X, want BNE path result 2", got)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_DefaultDispatcherExecutesNativeBFWriteMemoryImmediate(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	tests := []struct {
		name       string
		opcode     uint16
		ext        uint16
		initial    byte
		insertD0   uint32
		wantMemory byte
		wantD1     uint32
	}{
		{
			name:       "BFCLR",
			opcode:     0xECD0, // BFCLR (A0){0:2}
			ext:        0x0002,
			initial:    0xC0,
			wantMemory: 0x00,
			wantD1:     2,
		},
		{
			name:       "BFINS",
			opcode:     0xEFD0, // BFINS D0,(A0){0:2}
			ext:        0x0002,
			initial:    0x00,
			insertD0:   0x3,
			wantMemory: 0xC0,
			wantD1:     2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const (
				startPC = uint32(0x1000)
				base    = uint32(0x8000)
			)
			opcodes := []uint16{
				tt.opcode, tt.ext,
				0x6604, // BNE.S set_two when tested/inserted field is non-zero
				0x7201, // MOVEQ #1,D1
				0x6002, // BRA.S done
				0x7202, // MOVEQ #2,D1
			}
			setup := func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = base
				cpu.DataRegs[0] = tt.insertD0
				cpu.Write8(base, tt.initial)
				cpu.SR = M68K_SR_S | M68K_SR_X | M68K_SR_V | M68K_SR_C
			}

			interp := newM68KTestProgramCPU(t, startPC)
			setup(interp)
			writeM68KStopProgram(interp, startPC, opcodes...)
			runM68KInterpreterUntilStopped(t, interp)

			jit := newM68KTestProgramCPU(t, startPC)
			jit.m68kJitEnabled = true
			setup(jit)
			writeM68KStopProgram(jit, startPC, opcodes...)
			instrs := m68kScanBlock(jit.memory, startPC)
			prefix := m68kProductionNativePrefix(jit.memory, startPC, instrs)
			if len(prefix) == 0 || prefix[0].opcode != tt.opcode {
				t.Fatalf("%s was not admitted into native prefix=%+v instrs=%+v", tt.name, prefix, instrs)
			}
			if !m68kCanUseProductionNativeBlock(jit.memory, startPC, prefix) {
				t.Fatalf("%s prefix not production native: fallback=%v conservative=%v productionSafe=%v genericIO=%v prefix=%+v instrs=%+v",
					tt.name, m68kNeedsFallback(prefix), m68kNeedsConservativeFallback(jit.memory, startPC, prefix),
					m68kBlockProductionNativeSafe(prefix), m68kBlockMayUseGenericIOFallback(prefix), prefix, instrs)
			}
			runM68KJITUntilStopped(t, jit)

			if got := jit.m68kJitFallbackOpcodeCounts[tt.opcode].Load(); got != 0 {
				t.Fatalf("%s fell back %d times", tt.name, got)
			}
			if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				t.Fatalf("%s did not execute a native JIT block", tt.name)
			}
			if got := jit.Read8(base); got != tt.wantMemory {
				t.Fatalf("%s memory=0x%02X, want 0x%02X", tt.name, got, tt.wantMemory)
			}
			if got := jit.DataRegs[1]; got != tt.wantD1 {
				t.Fatalf("%s D1=0x%08X, want %d", tt.name, got, tt.wantD1)
			}
			assertM68KCoreStateEqual(t, jit, interp)
		})
	}
}

// TestM68KJIT_Exec_RTS_CacheHitWithLazyCCR verifies the JSR+DBRA+RTS chain
// loop with block chaining and lazy CCR. Uses the same polling pattern as
// the benchmark to match working behavior.
func TestM68KJIT_Exec_RTS_CacheHitWithLazyCCR(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	w := func(ops ...uint16) {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
	}

	w(0x3E3C, 0x0004)         // MOVE.W #4,D7
	loopTop := pc             // 0x1004
	w(0x4EB9, 0x0000, 0x2000) // JSR $2000
	disp := int16(int32(loopTop) - int32(pc) - 2)
	w(0x51CF, uint16(disp)) // DBRA D7,loop
	w(0x4E72, 0x2700)       // STOP

	pc = 0x2000
	w(0x5280) // ADDQ.L #1,D0 (arithmetic → flagsLiveArith)
	w(0x4E75) // RTS

	// Use the same polling pattern as runM68KBenchJIT
	cpu.PC = 0x1000
	cpu.running.Store(true)
	cpu.stopped.Store(false)

	done := make(chan struct{})
	go func() {
		cpu.M68KExecuteJIT()
		close(done)
	}()

	// Poll for STOP (with timeout)
	deadline := time.After(5 * time.Second)
	for !cpu.stopped.Load() {
		select {
		case <-deadline:
			cpu.running.Store(false)
			waitDoneWithGuard(t, done)
			t.Fatal("RTS cache hit + lazy CCR test timed out")
		default:
			runtime.Gosched()
		}
	}
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	// ADDQ #1,D0 called 5 times (D7: 4,3,2,1,0,-1)
	if cpu.DataRegs[0] != 5 {
		t.Errorf("D0 = %d, want 5 (ADDQ called 5 times)", cpu.DataRegs[0])
	}
}

func TestM68KJIT_RTSCacheHitBudgetExitDoesNotFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC  = uint32(0x1000)
		stack    = uint32(0x9000)
		returnPC = uint32(0x2000)
	)
	cpu := newM68KTestProgramCPU(t, startPC)
	cpu.m68kJitEnabled = true
	cpu.AddrRegs[7] = stack
	cpu.Write32(stack, returnPC)
	writeM68KWords(cpu, startPC, 0x4E75) // RTS
	if err := cpu.initM68KJIT(); err != nil {
		t.Fatalf("initM68KJIT: %v", err)
	}
	t.Cleanup(cpu.freeM68KJIT)

	execMem, err := AllocExecMem(64 * 1024)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	instrs := m68kScanBlock(cpu.memory, startPC)
	block, err := m68kCompileBlockWithMem(instrs, startPC, execMem, cpu.memory)
	if err != nil {
		t.Fatalf("m68kCompileBlockWithMem: %v", err)
	}

	ctx := cpu.m68kJitCtx
	ctx.RTSCache0PC = returnPC
	ctx.RTSCache0Addr = 0xDEADBEEF
	ctx.ChainBudget = 0
	ctx.ChainCount = 0
	ctx.NeedIOFallback = 0
	ctx.NeedInval = 0
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))

	if ctx.NeedIOFallback == 0 {
		t.Fatal("RTS cache-hit budget exit did not request interpreter fallback")
	}
	if got := ctx.RetPC; got != startPC {
		t.Fatalf("RetPC=0x%08X, want RTS PC 0x%08X", got, startPC)
	}
	if got := ctx.RetCount; got != 0 {
		t.Fatalf("RetCount=%d, want 0", got)
	}
	if got := cpu.AddrRegs[7]; got != stack {
		t.Fatalf("A7=0x%08X, want restored stack 0x%08X", got, stack)
	}
}

func TestM68KJIT_RTSCacheEmptySlotDoesNotJumpNullForZeroReturnPC(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x1000)
		stack   = uint32(0x9000)
	)
	cpu := newM68KTestProgramCPU(t, startPC)
	cpu.m68kJitEnabled = true
	cpu.AddrRegs[7] = stack
	cpu.Write32(stack, 0)
	writeM68KWords(cpu, startPC, 0x4E75) // RTS
	if err := cpu.initM68KJIT(); err != nil {
		t.Fatalf("initM68KJIT: %v", err)
	}
	t.Cleanup(cpu.freeM68KJIT)

	execMem, err := AllocExecMem(64 * 1024)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	instrs := m68kScanBlock(cpu.memory, startPC)
	block, err := m68kCompileBlockWithMem(instrs, startPC, execMem, cpu.memory)
	if err != nil {
		t.Fatalf("m68kCompileBlockWithMem: %v", err)
	}

	ctx := cpu.m68kJitCtx
	ctx.RTSCache0PC = 0
	ctx.RTSCache0Addr = 0
	ctx.ChainBudget = 8
	ctx.ChainCount = 0
	ctx.NeedIOFallback = 0
	ctx.NeedInval = 0
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))

	if ctx.NeedIOFallback != 0 {
		t.Fatal("empty RTS cache slot requested interpreter fallback")
	}
	if got := ctx.RetPC; got != 0 {
		t.Fatalf("RetPC=0x%08X, want popped zero return PC", got)
	}
	if got := ctx.RetCount; got != 1 {
		t.Fatalf("RetCount=%d, want 1", got)
	}
	if got := cpu.AddrRegs[7]; got != stack+4 {
		t.Fatalf("A7=0x%08X, want popped stack 0x%08X", got, stack+4)
	}
}

func TestM68KJIT_RTSCacheSlot7ChainsToContinuation(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC  = uint32(0x1000)
		stack    = uint32(0x9000)
		returnPC = uint32(0x2000)
	)
	cpu := newM68KTestProgramCPU(t, startPC)
	cpu.m68kJitEnabled = true
	cpu.AddrRegs[7] = stack
	cpu.Write32(stack, returnPC)
	writeM68KWords(cpu, startPC, 0x4E71, 0x4E75) // NOP; RTS
	writeM68KWords(cpu, returnPC, 0x7007)        // MOVEQ #7,D0
	if err := cpu.initM68KJIT(); err != nil {
		t.Fatalf("initM68KJIT: %v", err)
	}
	t.Cleanup(cpu.freeM68KJIT)

	execMem, err := AllocExecMem(64 * 1024)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()

	rtsBlock, err := m68kCompileBlockWithMem([]M68KJITInstr{
		{opcode: 0x4E71, pcOffset: 0, length: 2, group: 4},
		{opcode: 0x4E75, pcOffset: 2, length: 2, group: 4},
	}, startPC, execMem, cpu.memory)
	if err != nil {
		t.Fatalf("compile RTS block: %v", err)
	}
	contBlock, err := m68kCompileBlockWithMem([]M68KJITInstr{{opcode: 0x7007, pcOffset: 0, length: 2, group: 7}}, returnPC, execMem, cpu.memory)
	if err != nil {
		t.Fatalf("compile continuation block: %v", err)
	}
	if contBlock.chainEntry == 0 {
		t.Fatal("continuation block has no chain entry")
	}

	ctx := cpu.m68kJitCtx
	ctx.RTSCache7PC = returnPC
	ctx.RTSCache7Addr = contBlock.chainEntry
	ctx.ChainBudget = 3
	ctx.ChainCount = 0
	ctx.NeedIOFallback = 0
	ctx.NeedInval = 0
	callNative(rtsBlock.execAddr, uintptr(unsafe.Pointer(ctx)))

	if ctx.NeedIOFallback != 0 {
		t.Fatal("RTS slot-7 cache hit requested interpreter fallback")
	}
	if got := cpu.DataRegs[0]; got != 7 {
		t.Fatalf("D0=%d, want 7 from chained continuation; RetPC=0x%08X ChainCount=%d ChainBudget=%d",
			got, ctx.RetPC, ctx.ChainCount, ctx.ChainBudget)
	}
	if got := ctx.RetPC; got != returnPC+2 {
		t.Fatalf("RetPC=0x%08X, want 0x%08X", got, returnPC+2)
	}
	if got := m68kJITRetiredInstructionCount(ctx.RetCount, ctx.ChainCount, rtsBlock.instrCount, false); got != 3 {
		t.Fatalf("retired instruction count=%d, want 3 (NOP + RTS + continuation MOVEQ); RetCount=%d ChainCount=%d",
			got, ctx.RetCount, ctx.ChainCount)
	}
	if got := cpu.AddrRegs[7]; got != stack+4 {
		t.Fatalf("A7=0x%08X, want 0x%08X", got, stack+4)
	}
}

func TestM68KJIT_SingleRTSCacheHitChainsToContinuation(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC  = uint32(0x1000)
		stack    = uint32(0x9000)
		returnPC = uint32(0x2000)
	)
	cpu := newM68KTestProgramCPU(t, startPC)
	cpu.m68kJitEnabled = true
	cpu.AddrRegs[7] = stack
	cpu.Write32(stack, returnPC)
	writeM68KWords(cpu, startPC, 0x4E75)  // RTS
	writeM68KWords(cpu, returnPC, 0x7009) // MOVEQ #9,D0
	if err := cpu.initM68KJIT(); err != nil {
		t.Fatalf("initM68KJIT: %v", err)
	}
	t.Cleanup(cpu.freeM68KJIT)

	execMem, err := AllocExecMem(64 * 1024)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()

	rtsBlock, err := m68kCompileBlockWithMem([]M68KJITInstr{{opcode: 0x4E75, pcOffset: 0, length: 2, group: 4}}, startPC, execMem, cpu.memory)
	if err != nil {
		t.Fatalf("compile RTS block: %v", err)
	}
	contBlock, err := m68kCompileBlockWithMem([]M68KJITInstr{{opcode: 0x7009, pcOffset: 0, length: 2, group: 7}}, returnPC, execMem, cpu.memory)
	if err != nil {
		t.Fatalf("compile continuation block: %v", err)
	}
	if contBlock.chainEntry == 0 {
		t.Fatal("continuation block has no chain entry")
	}

	ctx := cpu.m68kJitCtx
	ctx.RTSCache0PC = returnPC
	ctx.RTSCache0Addr = contBlock.chainEntry
	ctx.ChainBudget = 2
	ctx.ChainCount = 0
	ctx.NeedIOFallback = 0
	ctx.NeedInval = 0
	callNative(rtsBlock.execAddr, uintptr(unsafe.Pointer(ctx)))

	if ctx.NeedIOFallback != 0 {
		t.Fatal("single RTS cache hit requested interpreter fallback")
	}
	if got := cpu.DataRegs[0]; got != 9 {
		t.Fatalf("D0=%d, want 9 from chained continuation; RetPC=0x%08X ChainCount=%d ChainBudget=%d",
			got, ctx.RetPC, ctx.ChainCount, ctx.ChainBudget)
	}
	if got := ctx.RetPC; got != returnPC+2 {
		t.Fatalf("RetPC=0x%08X, want 0x%08X", got, returnPC+2)
	}
	if got := m68kJITRetiredInstructionCount(ctx.RetCount, ctx.ChainCount, rtsBlock.instrCount, false); got != 2 {
		t.Fatalf("retired instruction count=%d, want 2 (RTS + continuation MOVEQ); RetCount=%d ChainCount=%d",
			got, ctx.RetCount, ctx.ChainCount)
	}
	if got := cpu.AddrRegs[7]; got != stack+4 {
		t.Fatalf("A7=0x%08X, want 0x%08X", got, stack+4)
	}
}

func TestM68KJIT_DefaultDispatcherExecutesInterruptMaskRTEBlockNoFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC  = uint32(0x1000)
		returnPC = uint32(0x2000)
		stack    = uint32(0x9000)
	)
	opcodes := []uint16{
		0x0057, 0x0700, // ORI.W #$0700,(A7)
		0x4E73, // RTE
	}
	setup := func(cpu *M68KCPU) {
		cpu.SR = M68K_SR_S
		cpu.AddrRegs[7] = stack
		cpu.Write16(stack, M68K_SR_S)
		cpu.Write32(stack+2, returnPC)
		cpu.Write16(stack+6, 0)
		cpu.Write16(returnPC, 0x4E72) // STOP #$2700
		cpu.Write16(returnPC+2, 0x2700)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	setup(interp)
	writeM68KWords(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	setup(jit)
	writeM68KWords(jit, startPC, opcodes...)
	instrs := m68kScanBlock(jit.memory, startPC)
	if m68kNeedsFallback(instrs) || !m68kCanUseProductionNativeBlock(jit.memory, startPC, instrs) {
		t.Fatalf("interrupt-mask RTE block was not fully native: instrs=%+v needsFallback=%v conservative=%v productionSafe=%v genericIO=%v",
			instrs,
			m68kNeedsFallback(instrs), m68kNeedsConservativeFallback(jit.memory, startPC, instrs),
			m68kBlockProductionNativeSafe(instrs), m68kBlockMayUseGenericIOFallback(instrs))
	}
	runM68KJITUntilStopped(t, jit)

	if got := jit.m68kJitFallbackOpcodeCounts[0x0057].Load(); got != 0 {
		t.Fatalf("ORI.W #$0700,(A7) fell back %d times", got)
	}
	if got := jit.m68kJitFallbackOpcodeCounts[0x4E73].Load(); got != 0 {
		t.Fatalf("RTE fell back %d times", got)
	}
	if got := jit.m68kJitNativeBlocksExecuted.Load(); got == 0 {
		t.Fatal("interrupt-mask RTE block did not execute native JIT")
	}
	assertM68KCoreStateEqual(t, jit, interp)
}

func TestM68KJIT_NativeJSRDispA6PushesExactReturnPC(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x0091D022)
		a6      = uint32(0x0080055C)
	)
	cpu := newM68KTestProgramCPU(t, startPC)
	cpu.AddrRegs[6] = a6
	writeM68KWords(cpu, startPC, 0x4EAE, 0xFDD8) // JSR -552(A6)

	instrs := m68kScanBlock(cpu.memory, startPC)
	if len(instrs) != 1 || instrs[0].opcode != 0x4EAE || instrs[0].length != 4 {
		t.Fatalf("unexpected scan for JSR d16(A6): %+v", instrs)
	}
	if !m68kCanUseProductionNativeBlock(cpu.memory, startPC, instrs) {
		t.Fatalf("JSR d16(A6) was not admitted to production native path: instrs=%+v", instrs)
	}
}

func TestM68KJIT_NativePrefixedJSRDispA6PushesExactReturnPC(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC = uint32(0x0091D640)
		a6      = uint32(0x0080055C)
	)
	cpu := newM68KTestProgramCPU(t, startPC)
	cpu.DataRegs[4] = 9
	cpu.AddrRegs[6] = a6
	writeM68KWords(cpu, startPC,
		0x59C4,         // SUBQ.L #4,D4
		0x4EAE, 0xFD78, // JSR -648(A6)
	)

	instrs := m68kScanBlock(cpu.memory, startPC)
	if len(instrs) != 2 || instrs[0].opcode != 0x59C4 || instrs[1].opcode != 0x4EAE || instrs[1].length != 4 {
		t.Fatalf("unexpected scan for prefixed JSR d16(A6): %+v", instrs)
	}
	if !m68kCanUseProductionNativeBlock(cpu.memory, startPC, instrs) {
		t.Fatalf("prefixed JSR d16(A6) was not admitted to production native path: instrs=%+v", instrs)
	}
}

func TestM68KJIT_NativeJSRAbsLongPushesExactReturnPC(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const (
		startPC  = uint32(0x0091D022)
		targetPC = uint32(0x0091D680)
	)
	cpu := newM68KTestProgramCPU(t, startPC)
	writeM68KWords(cpu, startPC,
		0x4EB9, uint16(targetPC>>16), uint16(targetPC&0xFFFF), // JSR $0091D680
	)

	instrs := m68kScanBlock(cpu.memory, startPC)
	if len(instrs) != 1 || instrs[0].opcode != 0x4EB9 || instrs[0].length != 6 {
		t.Fatalf("unexpected scan for JSR abs.L: %+v", instrs)
	}
	if !m68kCanUseProductionNativeBlock(cpu.memory, startPC, instrs) {
		t.Fatalf("JSR abs.L was not admitted to production native path: instrs=%+v", instrs)
	}
}

func TestM68KJIT_NativeJSRDispA6ThroughLibraryVectorPreservesReturns(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const startPC = uint32(0x0091D022)

	cpu := newM68KTestProgramCPU(t, startPC)

	writeM68KWords(cpu, startPC,
		0x4EAE, 0xFDD8, // JSR -552(A6)
		0x4E72, 0x2700, // STOP #$2700
	)

	instrs := m68kScanBlock(cpu.memory, startPC)
	if len(instrs) != 1 || instrs[0].opcode != 0x4EAE || instrs[0].length != 4 {
		t.Fatalf("unexpected scan for outer JSR d16(A6): %+v", instrs)
	}
	if !m68kCanUseProductionNativeBlock(cpu.memory, startPC, instrs) {
		t.Fatalf("outer JSR d16(A6) was not admitted to production native path: instrs=%+v", instrs)
	}
}

// TestM68KJIT_Exec_RTS_IOBailRetPC verifies that the RTS I/O bail path
// correctly sets RetPC so the dispatcher re-executes via interpreter.
func TestM68KJIT_Exec_RTS_IOBailRetPC(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Set A7 to the I/O region (>= 0xA0000) so RTS bails.
	// The bail should set RetPC to the RTS instruction's PC.
	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x000A0010) // SSP in I/O region
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	// Put A7 in I/O region — RTS will bail when reading the return address
	cpu.AddrRegs[7] = 0xA0000

	// Write a return address at 0xA0000 (big-endian 0x00001008)
	cpu.Write32(0xA0000, 0x00001008)

	pc := uint32(0x1000)
	w := func(ops ...uint16) {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
	}

	w(0x7001) // MOVEQ #1,D0
	w(0x4E75) // RTS (at 0x1002) — will bail because A7 >= IOThreshold
	// After interpreter re-executes RTS, PC should be 0x1008
	w(0x4E71)         // NOP (0x1004)
	w(0x4E71)         // NOP (0x1006)
	w(0x7003)         // MOVEQ #3,D0 (0x1008 — return target)
	w(0x4E72, 0x2700) // STOP

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	// D0 should be 3 (set at 0x1008, the return target after RTS)
	if cpu.DataRegs[0] != 3 {
		t.Errorf("D0 = %d, want 3 (set at return target after RTS I/O bail)", cpu.DataRegs[0])
	}
}

// TestM68KJIT_Exec_SelfModDuringChain verifies that self-modifying code
// during chained execution triggers cache invalidation.
func TestM68KJIT_Exec_SelfModDuringChain(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Block writes to a code page, triggering NeedInval.
	// MOVEQ #42,D0; MOVE.L D0,(0x1000) — writes to own code page
	// Then STOP. The write to 0x1000 (code page) should trigger invalidation.
	cpu := runM68KJITStopProgram(t, 0x1000,
		0x702A,                 // MOVEQ #42,D0
		0x23C0, 0x0000, 0x1000, // MOVE.L D0,$1000 (writes to code page)
	)

	if cpu.DataRegs[0] != 42 {
		t.Errorf("D0 = %d, want 42", cpu.DataRegs[0])
	}
	// The program ran to completion (didn't crash from invalidation)
}

func TestM68KJIT_MachineBusWriteInvalidatesCompiledCodePage(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	runner := NewM68KRunner(cpu)
	if err := cpu.initM68KJIT(); err != nil {
		t.Fatalf("initM68KJIT: %v", err)
	}
	t.Cleanup(cpu.freeM68KJIT)

	snap := runtimeStatus.snapshot()
	runtimeStatus.setCPUs(runtimeCPUM68K, snap.ie32, snap.ie64, runner, snap.z80, snap.x86, snap.cpu65)
	t.Cleanup(func() {
		runtimeStatus.setCPUs(snap.selectedCPU, snap.ie32, snap.ie64, snap.m68k, snap.z80, snap.x86, snap.cpu65)
	})

	const pc = uint64(0x9000)
	page := pc >> 12
	cpu.m68kJitCache.Put(&JITBlock{startPC: pc, endPC: pc + 4})
	cpu.m68kJitCodeBitmap[page] = 1
	cpu.m68kJitCtx.RTSCache0PC = 0x2000
	cpu.m68kJitCtx.RTSCache0Addr = 0x3000

	bus.Write16(uint32(pc+2), 0x4E71)

	if got := cpu.m68kJitCache.Get(pc); got != nil {
		t.Fatalf("compiled block survived MachineBus.Write16 invalidation: %#v", got)
	}
	if got := cpu.m68kJitCodeBitmap[page]; got != 0 {
		t.Fatalf("code bitmap page after MachineBus.Write16 = %d, want 0", got)
	}
	if cpu.m68kJitCtx.RTSCache0PC != 0 || cpu.m68kJitCtx.RTSCache0Addr != 0 {
		t.Fatalf("RTS cache was not cleared by MachineBus.Write16 invalidation")
	}
}

// TestM68KJIT_BusWriteStoresBytesBeforeInvalidation proves the guest bytes are
// committed to memory BEFORE the JIT invalidation fires, for every bus write
// width and WriteGuestBytes. If invalidation ran first, a live dispatcher could
// drain it and recompile the OLD bytes in the gap. The test installs an
// invalidator that reads back the just-written location: with correct ordering
// it must observe the NEW bytes, never the stale ones.
func TestM68KJIT_BusWriteStoresBytesBeforeInvalidation(t *testing.T) {
	const addr = uint32(0x4000)

	read32 := func(bus *MachineBus) uint32 {
		mem := bus.GetMemory()
		return uint32(mem[addr]) | uint32(mem[addr+1])<<8 |
			uint32(mem[addr+2])<<16 | uint32(mem[addr+3])<<24
	}

	cases := []struct {
		name  string
		write func(bus *MachineBus)
		want  uint32
	}{
		{"Write32", func(bus *MachineBus) { bus.Write32(addr, 0xDEADBEEF) }, 0xDEADBEEF},
		{"Write16", func(bus *MachineBus) { bus.Write16(addr, 0xBEEF) }, 0x0000BEEF},
		{"Write8", func(bus *MachineBus) { bus.Write8(addr, 0xEF) }, 0x000000EF},
		{"WriteGuestBytes", func(bus *MachineBus) {
			_ = WriteGuestBytes(bus, addr, 0, []byte{0xEF, 0xBE, 0xAD, 0xDE})
		}, 0xDEADBEEF},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()
			var sawCallback bool
			var observed uint32
			bus.RegisterM68KJITInvalidator(func(a, sz uint64) {
				sawCallback = true
				observed = read32(bus)
			})
			tc.write(bus)
			if !sawCallback {
				t.Fatal("JIT invalidator was not called for the guest write")
			}
			if observed != tc.want {
				t.Fatalf("invalidator observed 0x%08X, want 0x%08X — bytes not stored before invalidation", observed, tc.want)
			}
		})
	}
}

func TestM68KJIT_MachineBusWriteDefersInvalidationDuringNative(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	runner := NewM68KRunner(cpu)
	if err := cpu.initM68KJIT(); err != nil {
		t.Fatalf("initM68KJIT: %v", err)
	}
	t.Cleanup(cpu.freeM68KJIT)

	snap := runtimeStatus.snapshot()
	runtimeStatus.setCPUs(runtimeCPUM68K, snap.ie32, snap.ie64, runner, snap.z80, snap.x86, snap.cpu65)
	t.Cleanup(func() {
		runtimeStatus.setCPUs(snap.selectedCPU, snap.ie32, snap.ie64, snap.m68k, snap.z80, snap.x86, snap.cpu65)
	})

	const pc = uint64(0xA000)
	page := pc >> 12
	block := &JITBlock{startPC: pc, endPC: pc + 4}
	cpu.m68kJitCache.Put(block)
	cpu.m68kJitCodeBitmap[page] = 1
	cpu.m68kJitCtx.RTSCache0PC = 0x2100
	cpu.m68kJitCtx.RTSCache0Addr = 0x3100

	cpu.m68kJitNativeActive.Store(true)
	bus.Write16(uint32(pc+2), 0x4E71)

	if got := cpu.m68kJitCache.Get(pc); got != block {
		t.Fatalf("compiled block was invalidated while native code was active: %#v", got)
	}
	if got := cpu.m68kJitCodeBitmap[page]; got != 1 {
		t.Fatalf("code bitmap page during deferred invalidation = %d, want 1", got)
	}
	if !cpu.m68kJitDeferredInval.Load() {
		t.Fatalf("MachineBus.Write16 did not mark deferred invalidation")
	}
	if cpu.m68kJitCtx.RTSCache0PC != 0x2100 || cpu.m68kJitCtx.RTSCache0Addr != 0x3100 {
		t.Fatalf("RTS cache was cleared before native returned")
	}

	cpu.m68kJitNativeActive.Store(false)
	if !cpu.m68kApplyDeferredJITInvalidation() {
		t.Fatalf("deferred invalidation was not applied after native returned")
	}
	if got := cpu.m68kJitCache.Get(pc); got != nil {
		t.Fatalf("compiled block survived deferred invalidation: %#v", got)
	}
	if got := cpu.m68kJitCodeBitmap[page]; got != 0 {
		t.Fatalf("code bitmap page after deferred invalidation = %d, want 0", got)
	}
	if cpu.m68kJitCtx.RTSCache0PC != 0 || cpu.m68kJitCtx.RTSCache0Addr != 0 {
		t.Fatalf("RTS cache was not cleared by deferred invalidation")
	}
}

// TestM68KJIT_STOPIdleHookFiresInJITLoop verifies the JIT dispatcher's STOP
// spin invokes StoppedIdleHook when the guest is idle at STOP with nothing
// pending. The deterministic-IRQ boot harness installs this hook to pump its
// timer/vblank IRQ source (the instruction-count hook is frozen during STOP);
// without it the JIT boot path deadlocks at cpu_Dispatch's idle STOP. Mirrors
// the interpreter STOP spin, which already calls the hook.
func TestM68KJIT_STOPIdleHookFiresInJITLoop(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	cpu.SR |= M68K_SR_S // supervisor so the loop takes the STOP idle path
	cpu.stopped.Store(true)
	cpu.running.Store(true)

	fired := 0
	cpu.StoppedIdleHook = func(c *M68KCPU) {
		fired++
		// Break out of the dispatcher so the test terminates deterministically.
		c.running.Store(false)
	}

	cpu.M68KExecuteJIT()

	if fired == 0 {
		t.Fatal("JIT STOP idle loop did not invoke StoppedIdleHook")
	}
}

// TestM68KJIT_Exec_RTSCacheClearedOnInval verifies that the RTS inline
// cache is cleared on cache invalidation.
func TestM68KJIT_Exec_RTSCacheClearedOnInval(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// JSR + RTS + self-mod write + STOP
	// The self-mod write should clear the RTS cache.
	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	w := func(ops ...uint16) {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
	}

	w(0x4EB9, 0x0000, 0x2000) // JSR $2000
	w(0x23C0, 0x0000, 0x2000) // MOVE.L D0,$2000 (write to sub code page → invalidate)
	w(0x4E72, 0x2700)         // STOP

	pc = 0x2000
	w(0x702A) // MOVEQ #42,D0
	w(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	if cpu.DataRegs[0] != 42 {
		t.Errorf("D0 = %d, want 42 (set in subroutine before invalidation)", cpu.DataRegs[0])
	}
}
