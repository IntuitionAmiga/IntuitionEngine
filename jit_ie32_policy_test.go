package main

import (
	"fmt"
	"testing"
	"time"
)

func TestIE32JIT_DefaultPolicyMatchesBackendAvailability(t *testing.T) {
	cpu := NewCPU(NewMachineBus())
	if cpu.jitEnabled != ie32JITRuntimeAvailable() {
		t.Fatalf("NewCPU jitEnabled = %v, want runtime availability %v", cpu.jitEnabled, ie32JITRuntimeAvailable())
	}
	wantBackend := "none"
	if ie32JITRuntimeAvailable() {
		wantBackend = ie32JITBackend
	}
	if got, want := cpu.JITStats().Backend, wantBackend; got != want {
		t.Fatalf("initial backend = %q, want %q", got, want)
	}
}

func TestIE32JIT_ConfiguredConstructorAppliesOnlyStartupPolicy(t *testing.T) {
	for _, tc := range []struct {
		name       string
		disableJIT bool
	}{
		{name: "default", disableJIT: false},
		{name: "nojit", disableJIT: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cpu := newIE32CPUConfigured(NewMachineBus(), tc.disableJIT)
			want := ie32JITRuntimeAvailable() && !tc.disableJIT
			if cpu.jitEnabled != want {
				t.Fatalf("jitEnabled=%v, want %v", cpu.jitEnabled, want)
			}
		})
	}
}

func TestIE32JIT_WorkerFactoryPropagatesStartupPolicy(t *testing.T) {
	for _, disableJIT := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "nojit"}[disableJIT], func(t *testing.T) {
			bus := NewMachineBus()
			code := buildIE32ServiceBinary(ringBaseAddr(coprocRingIndex(EXEC_TYPE_IE32, 0)))
			worker, err := createIE32WorkerConfigured(bus, code, 0, disableJIT)
			if err != nil {
				t.Fatalf("create worker: %v", err)
			}
			defer func() {
				worker.stop()
				<-worker.done
			}()
			adapter, ok := worker.debugCPU.(*DebugIE32)
			if !ok || adapter.cpu == nil {
				t.Fatal("worker did not retain IE32 debug CPU")
			}
			want := ie32JITRuntimeAvailable() && !disableJIT
			if adapter.cpu.jitEnabled != want {
				t.Fatalf("worker jitEnabled=%v, want %v", adapter.cpu.jitEnabled, want)
			}
		})
	}
}

func TestIE32JIT_StoppedToggleRejectsUnavailableBackend(t *testing.T) {
	cpu := NewCPU(NewMachineBus())
	cpu.running.Store(false)
	if err := cpu.SetJITEnabled(true); ie32JITRuntimeAvailable() && err != nil {
		t.Fatalf("enabling available JIT: %v", err)
	} else if !ie32JITRuntimeAvailable() && err == nil {
		t.Fatal("enabling unavailable JIT succeeded")
	}
}

func TestIE32JIT_RunningToggleIsRejected(t *testing.T) {
	cpu := NewCPU(NewMachineBus())
	if err := cpu.SetJITEnabled(false); err == nil {
		t.Fatal("changed IE32 JIT state while CPU was running")
	}
}

func TestIE32JIT_ExecuteRoutesThroughJITEntryWhenEnabled(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable on this platform")
	}
	cpu := NewCPU(NewMachineBus())
	putIE32Instruction(cpu.memory, PROG_START, LDA, 0, ADDR_IMMEDIATE, 1)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if got := cpu.JITStats().NativeEntries; got == 0 {
		t.Fatal("default IE32 Execute did not enter generated code")
	}
	if ie32JITBackend == "native" && cpu.jitMarker == 0 {
		t.Fatal("default IE32 Execute did not install a generated entry")
	}
}

func TestIE32JIT_ResetReclaimsGeneratedEntry(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable on this platform")
	}
	cpu := NewCPU(NewMachineBus())
	cpu.memory[PROG_START] = HALT
	cpu.Execute()
	cpu.resetJITStats()
	if cpu.jitMarker != 0 || cpu.jit.execMem != nil {
		t.Fatal("reset retained IE32 generated entry memory")
	}
}

func TestIE32JIT_DirectImmediateLoadExecutesGeneratedCode(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable on this platform")
	}
	cpu := NewCPU(NewMachineBus())
	cpu.memory[PROG_START] = LDA
	cpu.memory[PROG_START+ADDRMODE_OFFSET] = ADDR_IMMEDIATE
	cpu.memory[PROG_START+OPERAND_OFFSET] = 0x78
	cpu.memory[PROG_START+OPERAND_OFFSET+1] = 0x56
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if got := cpu.A; got != 0x5678 {
		t.Fatalf("direct LDA result = %#x, want %#x", got, uint32(0x5678))
	}
	if got := cpu.JITStats().Instructions; got != 2 {
		t.Fatalf("JIT retired instructions = %d, want 2", got)
	}
	if got := cpu.JITStats().DirectInstructions; got != 1 {
		t.Fatalf("direct JIT instructions = %d, want 1", got)
	}
	if got := cpu.InstructionCount; got != 2 {
		t.Fatalf("architectural retired instructions = %d, want 2", got)
	}
}

func TestIE32JIT_DirectImmediateALUAndNamedLoad(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable on this platform")
	}
	cpu := NewCPU(NewMachineBus())
	putIE32Instruction(cpu.memory, PROG_START, LDX, 0, ADDR_IMMEDIATE, 0x10)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, ADD, 1, ADDR_IMMEDIATE, 5)
	putIE32Instruction(cpu.memory, PROG_START+2*INSTRUCTION_SIZE, XOR, 1, ADDR_IMMEDIATE, 3)
	putIE32Instruction(cpu.memory, PROG_START+3*INSTRUCTION_SIZE, MUL, 1, ADDR_IMMEDIATE, 3)
	putIE32Instruction(cpu.memory, PROG_START+4*INSTRUCTION_SIZE, SHL, 1, ADDR_IMMEDIATE, 1)
	putIE32Instruction(cpu.memory, PROG_START+5*INSTRUCTION_SIZE, NOT, 1, ADDR_IMMEDIATE, 0)
	putIE32Instruction(cpu.memory, PROG_START+6*INSTRUCTION_SIZE, HALT, 0, 0, 0)
	cpu.Execute()
	if got, want := cpu.X, ^uint32((((0x10+5)^3)*3)<<1); got != want {
		t.Fatalf("direct immediate sequence = %#x, want %#x", got, want)
	}
}

func TestIE32JIT_ResidentImmediateALURunPreservesArchitecturalState(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable on this platform")
	}
	jit := NewCPU(NewMachineBus())
	interpreter := newIE32CPUConfigured(NewMachineBus(), true)
	for _, cpu := range []*CPU{jit, interpreter} {
		cpu.A = 0x12345678
		putIE32Instruction(cpu.memory, PROG_START, ADD, REG_A, ADDR_IMMEDIATE, 0x10)
		putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, XOR, REG_A, ADDR_IMMEDIATE, 0xFF00FF00)
		putIE32Instruction(cpu.memory, PROG_START+2*INSTRUCTION_SIZE, MUL, REG_A, ADDR_IMMEDIATE, 3)
		putIE32Instruction(cpu.memory, PROG_START+3*INSTRUCTION_SIZE, SUB, REG_A, ADDR_IMMEDIATE, 7)
		putIE32Instruction(cpu.memory, PROG_START+4*INSTRUCTION_SIZE, HALT, 0, ADDR_IMMEDIATE, 0)
		cpu.Execute()
	}
	if jit.A != interpreter.A || jit.PC != interpreter.PC || jit.InstructionCount != interpreter.InstructionCount {
		t.Fatalf("resident immediate ALU state jit=%#x/%#x/%d interpreter=%#x/%#x/%d", jit.A, jit.PC, jit.InstructionCount, interpreter.A, interpreter.PC, interpreter.InstructionCount)
	}
	if got := jit.JITStats().DirectInstructions; got != 4 {
		t.Fatalf("resident immediate ALU direct instructions=%d, want 4", got)
	}
	if got := jit.JITStats().ResidentSpillsSaved; got != 3 {
		t.Fatalf("resident immediate ALU spills saved=%d, want 3", got)
	}
}

func TestIE32JIT_ResidentALUBranchPreservesState(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable on this platform")
	}
	jit := NewCPU(NewMachineBus())
	interpreter := newIE32CPUConfigured(NewMachineBus(), true)
	for _, cpu := range []*CPU{jit, interpreter} {
		cpu.A = 1
		putIE32Instruction(cpu.memory, PROG_START, ADD, REG_A, ADDR_IMMEDIATE, 1)
		putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, XOR, REG_A, ADDR_IMMEDIATE, 3)
		putIE32Instruction(cpu.memory, PROG_START+2*INSTRUCTION_SIZE, JNZ, REG_A, ADDR_IMMEDIATE, PROG_START+4*INSTRUCTION_SIZE)
		putIE32Instruction(cpu.memory, PROG_START+3*INSTRUCTION_SIZE, LDA, 0, ADDR_IMMEDIATE, 0xDEAD)
		putIE32Instruction(cpu.memory, PROG_START+4*INSTRUCTION_SIZE, HALT, 0, ADDR_IMMEDIATE, 0)
		cpu.Execute()
	}
	if jit.A != interpreter.A || jit.PC != interpreter.PC || jit.InstructionCount != interpreter.InstructionCount {
		t.Fatalf("resident branch state jit=%#x/%#x/%d interpreter=%#x/%#x/%d", jit.A, jit.PC, jit.InstructionCount, interpreter.A, interpreter.PC, interpreter.InstructionCount)
	}
	if got := jit.JITStats().DirectInstructions; got != 3 {
		t.Fatalf("resident branch direct instructions=%d, want 3", got)
	}
}

func TestIE32JIT_DirectGuardedRegisterIndirectALU(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable on this platform")
	}
	cpu := NewCPU(NewMachineBus())
	const operandAddr = uint32(0x400)
	cpu.A = 5
	cpu.X = operandAddr
	cpu.Write32(operandAddr, 7)
	putIE32Instruction(cpu.memory, PROG_START, ADD, REG_A, ADDR_REG_IND, REG_X)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if got := cpu.A; got != 12 {
		t.Fatalf("guarded register-indirect ADD A=%d, want 12", got)
	}
	if got := cpu.JITStats().DirectInstructions; got != 1 {
		t.Fatalf("direct guarded register-indirect instructions=%d, want 1", got)
	}
}

func TestIE32JIT_ConstantPointerSpecialisesLaterIndirectLoad(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable on this platform")
	}
	cpu := NewCPU(NewMachineBus())
	const address = uint32(0x400)
	cpu.Write32(address, 0xC0FFEE)
	putIE32Instruction(cpu.memory, PROG_START, LOAD, REG_X, ADDR_IMMEDIATE, address)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, LOAD, REG_A, ADDR_REG_IND, REG_X)
	cpu.memory[PROG_START+2*INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if got := cpu.A; got != 0xC0FFEE {
		t.Fatalf("constant pointer load A=%#x, want %#x", got, uint32(0xC0FFEE))
	}
	stats := cpu.JITStats()
	if stats.Blocks != 1 || stats.DirectInstructions != 2 {
		t.Fatalf("constant pointer provenance blocks=%d direct=%d, want 1/2", stats.Blocks, stats.DirectInstructions)
	}
}

func TestIE32JIT_DirectGuardedRegisterIndirectDivision(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable on this platform")
	}
	cpu := NewCPU(NewMachineBus())
	const operandAddr = uint32(0x400)
	cpu.A = 37
	cpu.X = operandAddr
	cpu.Write32(operandAddr, 5)
	putIE32Instruction(cpu.memory, PROG_START, DIV, REG_A, ADDR_REG_IND, REG_X)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if got := cpu.A; got != 7 {
		t.Fatalf("guarded register-indirect DIV A=%d, want 7", got)
	}
	if got := cpu.JITStats().DirectInstructions; got != 1 {
		t.Fatalf("direct guarded register-indirect instructions=%d, want 1", got)
	}
}

func TestIE32JIT_DirectGuardedRegisterShift(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	cpu.A, cpu.X = 3, 2
	putIE32Instruction(cpu.memory, PROG_START, SHL, REG_A, ADDR_REGISTER, REG_X)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if cpu.A != 12 || cpu.JITStats().DirectInstructions != 1 {
		t.Fatalf("shift A=%d direct=%d", cpu.A, cpu.JITStats().DirectInstructions)
	}
}

func TestIE32JIT_StaticForwardJumpRegion(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	start := uint32(PROG_START)
	target := start + 3*INSTRUCTION_SIZE
	putIE32Instruction(cpu.memory, start, JMP, 0, ADDR_IMMEDIATE, target)
	putIE32Instruction(cpu.memory, target, LDA, 0, ADDR_IMMEDIATE, 0xCAFE)
	cpu.memory[target+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	cpu.PC = start
	cpu.running.Store(true)
	cpu.Execute()
	if cpu.A != 0xCAFE {
		t.Fatalf("region result A=%#x, want %#x", cpu.A, uint32(0xCAFE))
	}
	stats := cpu.JITStats()
	if stats.NativeEntries == 0 || stats.Regions == 0 || stats.HotRecompilations == 0 {
		t.Fatalf("region provenance entries=%d regions=%d recompilations=%d, want hot generated region", stats.NativeEntries, stats.Regions, stats.HotRecompilations)
	}
}

func TestIE32JIT_ResidentALUCrossesStaticJumpRegion(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	start := uint32(PROG_START)
	target := start + 3*INSTRUCTION_SIZE
	putIE32Instruction(cpu.memory, start, ADD, REG_A, ADDR_IMMEDIATE, 1)
	putIE32Instruction(cpu.memory, start+INSTRUCTION_SIZE, JMP, 0, ADDR_IMMEDIATE, target)
	putIE32Instruction(cpu.memory, target, XOR, REG_A, ADDR_IMMEDIATE, 2)
	cpu.memory[target+INSTRUCTION_SIZE] = HALT
	cpu.Execute() // Establish the compact static-jump block.
	cpu.resetJITStats()
	cpu.PC = start
	cpu.running.Store(true)
	cpu.Execute() // Hot recompilation emits the chased region.
	if got, want := cpu.A, uint32(6); got != want {
		t.Fatalf("static-region resident ALU A=%#x, want %#x", got, want)
	}
	stats := cpu.JITStats()
	if stats.Regions == 0 || stats.ResidentSpillsSaved != 1 {
		t.Fatalf("static-region residency regions=%d spills=%d, want region/1", stats.Regions, stats.ResidentSpillsSaved)
	}
}

func TestIE32JIT_DeadImmediateLoadElisionPreservesRetirement(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	putIE32Instruction(cpu.memory, PROG_START, LOAD, REG_A, ADDR_IMMEDIATE, 1)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, LOAD, REG_A, ADDR_IMMEDIATE, 2)
	cpu.memory[PROG_START+2*INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if cpu.A != 2 {
		t.Fatalf("final immediate load A=%d, want 2", cpu.A)
	}
	if got := cpu.JITStats().DirectInstructions; got != 2 {
		t.Fatalf("direct retired instructions=%d, want 2", got)
	}
	if got := cpu.InstructionCount; got != 3 {
		t.Fatalf("architectural retired instructions=%d, want 3", got)
	}
}

func TestIE32JIT_ImmediateConstantFoldPreservesRetirement(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	putIE32Instruction(cpu.memory, PROG_START, LOAD, REG_A, ADDR_IMMEDIATE, 7)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, ADD, REG_A, ADDR_IMMEDIATE, 5)
	cpu.memory[PROG_START+2*INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if cpu.A != 12 || cpu.InstructionCount != 3 || cpu.JITStats().DirectInstructions != 2 {
		t.Fatalf("fold result A=%d retired=%d direct=%d", cpu.A, cpu.InstructionCount, cpu.JITStats().DirectInstructions)
	}
}

func TestIE32JIT_ImmediateNotFoldPreservesRetirement(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable on this platform")
	}
	cpu := NewCPU(NewMachineBus())
	putIE32Instruction(cpu.memory, PROG_START, LOAD, REG_A, ADDR_IMMEDIATE, 7)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, NOT, REG_A, ADDR_IMMEDIATE, 0)
	cpu.memory[PROG_START+2*INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if got, want := cpu.A, ^uint32(7); got != want {
		t.Fatalf("folded NOT result=%#x, want %#x", got, want)
	}
	if got := cpu.InstructionCount; got != 3 {
		t.Fatalf("architectural retired instructions=%d, want 3", got)
	}
	if got := cpu.JITStats().DirectInstructions; got != 2 {
		t.Fatalf("direct retired instructions=%d, want 2", got)
	}
}

func TestIE32JIT_TimerBoundaryPreservesSemanticsAndGeneratedProvenance(t *testing.T) {
	cpu := NewCPU(NewMachineBus())
	putIE32Instruction(cpu.memory, PROG_START, LDA, 0, ADDR_IMMEDIATE, 0x55)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.timerEnabled.Store(true)
	cpu.timerCount.Store(1)
	cpu.timerPeriod.Store(7)
	cpu.cycleCounter = SAMPLE_RATE - 1
	cpu.Execute()
	if got := cpu.timerCount.Load(); got != 7 {
		t.Fatalf("timer reload = %d, want 7", got)
	}
	if ie32JITRuntimeAvailable() {
		if got := cpu.JITStats().Blocks; got == 0 {
			t.Fatal("timer boundary did not lower the eligible instruction")
		}
		if got := cpu.JITStats().DirectInstructions; got != 1 {
			t.Fatalf("timer boundary direct instructions = %d, want 1", got)
		}
	}
}

func TestIE32JIT_TimerExpiryMatchesInterpreterAtEachBlockPosition(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	for position := uint32(0); position < 3; position++ {
		t.Run(fmt.Sprintf("instruction-%d", position+1), func(t *testing.T) {
			interpreter := newIE32TimedParityCPU(position)
			interpreter.running.Store(false)
			if err := interpreter.SetJITEnabled(false); err != nil {
				t.Fatalf("disable JIT: %v", err)
			}
			interpreter.running.Store(true)
			jit := newIE32TimedParityCPU(position)
			interpreter.Execute()
			jit.Execute()
			if interpreter.PC != jit.PC || interpreter.A != jit.A || interpreter.X != jit.X ||
				interpreter.timerCount.Load() != jit.timerCount.Load() || interpreter.timerState.Load() != jit.timerState.Load() ||
				interpreter.cycleCounter != jit.cycleCounter || interpreter.inInterrupt.Load() != jit.inInterrupt.Load() {
				t.Fatalf("timer state differs at position %d: interpreter count=%d cycle=%d PC=%#x, JIT count=%d cycle=%d PC=%#x", position+1, interpreter.timerCount.Load(), interpreter.cycleCounter, interpreter.PC, jit.timerCount.Load(), jit.cycleCounter, jit.PC)
			}
			if got := jit.JITStats().DirectInstructions; got != 3 {
				t.Fatalf("timer run direct instructions = %d, want 3", got)
			}
		})
	}
}

func TestIE32JIT_TimerInterruptInConditionalLoopMatchesInterpreter(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	jit := newIE32TimerInterruptLoopCPU(false)
	interpreter := newIE32TimerInterruptLoopCPU(true)
	jit.Execute()
	interpreter.Execute()
	if jit.A != interpreter.A || jit.PC != interpreter.PC || jit.SP != interpreter.SP || jit.InstructionCount != interpreter.InstructionCount || jit.inInterrupt.Load() != interpreter.inInterrupt.Load() || jit.timerCount.Load() != interpreter.timerCount.Load() {
		t.Fatalf("timer-loop state jit A=%#x PC=%#x SP=%#x retired=%d interrupt=%t timer=%d, interpreter A=%#x PC=%#x SP=%#x retired=%d interrupt=%t timer=%d", jit.A, jit.PC, jit.SP, jit.InstructionCount, jit.inInterrupt.Load(), jit.timerCount.Load(), interpreter.A, interpreter.PC, interpreter.SP, interpreter.InstructionCount, interpreter.inInterrupt.Load(), interpreter.timerCount.Load())
	}
	if jit.A != 0 {
		t.Fatalf("timer ISR/loop final A=%d, want 0", jit.A)
	}
	if got := jit.JITStats().DirectInstructions; got != 7 {
		t.Fatalf("timer-loop direct instructions=%d, want 7", got)
	}
}

func TestIE32JIT_TimerInterruptSelectsSameInstructionAsInterpreter(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	newCPU := func(disableJIT bool) *CPU {
		cpu := newIE32CPUConfigured(NewMachineBus(), disableJIT)
		const handler = uint32(PROG_START + 0x100)
		putIE32Instruction(cpu.memory, PROG_START, LDA, 0, ADDR_IMMEDIATE, 0x11)
		putIE32Instruction(cpu.memory, handler, LDA, 0, ADDR_IMMEDIATE, 0x22)
		cpu.memory[handler+INSTRUCTION_SIZE] = HALT
		cpu.Write32(VECTOR_TABLE, handler)
		cpu.timerEnabled.Store(true)
		cpu.timerCount.Store(1)
		cpu.timerPeriod.Store(100)
		cpu.interruptEnabled.Store(true)
		cpu.cycleCounter = SAMPLE_RATE - 1
		return cpu
	}
	interpreter := newCPU(true)
	jit := newCPU(false)
	interpreter.Execute()
	jit.Execute()
	if interpreter.A != jit.A || interpreter.PC != jit.PC || interpreter.SP != jit.SP || interpreter.InstructionCount != jit.InstructionCount {
		t.Fatalf("timer interrupt selection diverged: interpreter A=%#x PC=%#x SP=%#x retired=%d; JIT A=%#x PC=%#x SP=%#x retired=%d", interpreter.A, interpreter.PC, interpreter.SP, interpreter.InstructionCount, jit.A, jit.PC, jit.SP, jit.InstructionCount)
	}
	if got := jit.A; got != 0x22 {
		t.Fatalf("JIT executed interrupted instruction A=%#x, want ISR value %#x", got, uint32(0x22))
	}
}

func newIE32TimerInterruptLoopCPU(disableJIT bool) *CPU {
	cpu := newIE32CPUConfigured(NewMachineBus(), disableJIT)
	const handler = uint32(PROG_START + 0x100)
	loop := uint32(PROG_START + INSTRUCTION_SIZE)
	cpu.Write32(VECTOR_TABLE, handler)
	putIE32Instruction(cpu.memory, handler, LDA, 0, ADDR_IMMEDIATE, 0xBEEF)
	putIE32Instruction(cpu.memory, handler+INSTRUCTION_SIZE, RTI, 0, ADDR_IMMEDIATE, 0)
	putIE32Instruction(cpu.memory, PROG_START, LDA, 0, ADDR_IMMEDIATE, 3)
	putIE32Instruction(cpu.memory, loop, DEC, 0, ADDR_REGISTER, REG_A)
	putIE32Instruction(cpu.memory, loop+INSTRUCTION_SIZE, JNZ, REG_A, ADDR_IMMEDIATE, loop)
	cpu.memory[loop+2*INSTRUCTION_SIZE] = HALT
	cpu.timerEnabled.Store(true)
	cpu.timerCount.Store(1)
	cpu.timerPeriod.Store(100)
	cpu.interruptEnabled.Store(true)
	cpu.cycleCounter = SAMPLE_RATE - 1
	return cpu
}

func newIE32TimedParityCPU(expiryPosition uint32) *CPU {
	cpu := NewCPU(NewMachineBus())
	putIE32Instruction(cpu.memory, PROG_START, LDA, 0, ADDR_IMMEDIATE, 1)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, LDX, 0, ADDR_IMMEDIATE, 2)
	putIE32Instruction(cpu.memory, PROG_START+2*INSTRUCTION_SIZE, ADD, REG_A, ADDR_IMMEDIATE, 3)
	cpu.memory[PROG_START+3*INSTRUCTION_SIZE] = HALT
	cpu.timerEnabled.Store(true)
	cpu.timerCount.Store(1)
	cpu.timerPeriod.Store(7)
	cpu.cycleCounter = SAMPLE_RATE - 1 - expiryPosition
	return cpu
}

func TestIE32JIT_HelperResumesGeneratedExecution(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	putIE32Instruction(cpu.memory, PROG_START, WAIT, 0, ADDR_IMMEDIATE, 0)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, LDA, 0, ADDR_IMMEDIATE, 0x55)
	cpu.memory[PROG_START+2*INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if got := cpu.A; got != 0x55 {
		t.Fatalf("post-helper direct load A=%#x, want %#x", got, uint32(0x55))
	}
	if got := cpu.JITStats().DirectInstructions; got != 1 {
		t.Fatalf("post-helper direct instructions = %d, want 1", got)
	}
	if got := cpu.InstructionCount; got != 3 {
		t.Fatalf("architectural retired instructions = %d, want 3", got)
	}
}

func TestIE32JIT_HelperOnlyRunDoesNotFabricateNativeEntry(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	putIE32Instruction(cpu.memory, PROG_START, WAIT, 0, ADDR_IMMEDIATE, 0)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if got := cpu.JITStats().NativeEntries; got != 0 {
		t.Fatalf("helper-only run fabricated %d native entries", got)
	}
	if got := cpu.JITStats().DirectInstructions; got != 0 {
		t.Fatalf("helper-only run directly lowered %d instructions", got)
	}
	stats := cpu.JITStats()
	if stats.HelperInstructions != 2 || stats.Deoptimizations != 2 {
		t.Fatalf("helper-only counters = helpers:%d deopts:%d, want 2 each", stats.HelperInstructions, stats.Deoptimizations)
	}
	if stats.HelperDeopts != 2 || stats.SourceStampDeopts != 0 {
		t.Fatalf("helper-only deopt reasons = helper:%d stamp:%d", stats.HelperDeopts, stats.SourceStampDeopts)
	}
}

func TestIE32JIT_PureBlockCacheReusesGeneratedCode(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	putIE32Instruction(cpu.memory, PROG_START, LDA, 0, ADDR_IMMEDIATE, 0x5A3C)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	first := cpu.JITStats()
	if first.Blocks == 0 {
		t.Fatalf("first compiled blocks=%d, want 1", first.Blocks)
	}
	cpu.PC = PROG_START
	cpu.running.Store(true)
	cpu.Execute()
	second := cpu.JITStats()
	if second.CacheHits <= first.CacheHits {
		t.Fatalf("cache hits did not increase: first=%d second=%d", first.CacheHits, second.CacheHits)
	}
	if second.NativeEntries != first.NativeEntries+1 {
		t.Fatalf("native entries=%d, want %d", second.NativeEntries, first.NativeEntries+1)
	}
	if ie32JITBackend == "native" && second.Blocks != first.Blocks {
		t.Fatalf("native cache miss recompiled block: first=%d second=%d", first.Blocks, second.Blocks)
	}
}

func TestIE32JIT_DynamicNativeGuardsAreNotCacheable(t *testing.T) {
	tests := []ie32DecodedInstruction{
		{Opcode: DIV, AddrMode: ADDR_DIRECT, Operand: 0x400},
		{Opcode: MOD, AddrMode: ADDR_DIRECT, Operand: 0x400},
		{Opcode: SHL, AddrMode: ADDR_REGISTER, Operand: REG_B},
		{Opcode: SHR, AddrMode: ADDR_REGISTER, Operand: REG_B},
	}
	for _, in := range tests {
		t.Run(fmt.Sprintf("opcode_%02x", in.Opcode), func(t *testing.T) {
			if ie32CacheableNativeBlock([]ie32DecodedInstruction{in}, 1) {
				t.Fatalf("dynamically guarded %#v must not enter native cache", in)
			}
		})
	}
}

func TestIE32JIT_DynamicGuardedFormsAreNotRetained(t *testing.T) {
	if !ie32JITRuntimeAvailable() || ie32JITBackend != "native" {
		t.Skip("native IE32 JIT unavailable")
	}
	for _, tc := range []struct {
		name string
		op   byte
		mode byte
		init func(*CPU)
	}{
		{"direct_divisor", DIV, ADDR_DIRECT, func(cpu *CPU) { cpu.A = 12; cpu.Write32(0x400, 3) }},
		{"direct_modulus", MOD, ADDR_DIRECT, func(cpu *CPU) { cpu.A = 13; cpu.Write32(0x400, 3) }},
		{"register_shift_left", SHL, ADDR_REGISTER, func(cpu *CPU) { cpu.A = 3; cpu.B = 1 }},
		{"register_shift_right", SHR, ADDR_REGISTER, func(cpu *CPU) { cpu.A = 8; cpu.B = 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cpu := NewCPU(NewMachineBus())
			tc.init(cpu)
			operand := uint32(0x400)
			if tc.mode == ADDR_REGISTER {
				operand = REG_B
			}
			putIE32Instruction(cpu.memory, PROG_START, tc.op, REG_A, tc.mode, operand)
			cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
			cpu.Execute()
			if len(cpu.jit.nativeCache) != 0 {
				t.Fatalf("dynamically guarded %s was retained in native cache", tc.name)
			}
		})
	}
}

func TestIE32CPUDisposeUnregistersBusInvalidator(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU(bus)
	if got := bus.ie32JITInvalidatorCnt.Load(); got != 1 {
		t.Fatalf("invalidator count after construction=%d, want 1", got)
	}
	cpu.Dispose()
	if got := bus.ie32JITInvalidatorCnt.Load(); got != 0 {
		t.Fatalf("invalidator count after disposal=%d, want 0", got)
	}
	cpu.Dispose()
	if got := bus.ie32JITInvalidatorCnt.Load(); got != 0 {
		t.Fatalf("invalidator count after repeated disposal=%d, want 0", got)
	}
}

func TestIE32JIT_DebugInstrumentationUsesStepThenResumesJIT(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	access := NewDebugAccessService()
	access.EnableHistory(8)
	cpu.debugAccess = access
	cpu.debugCPUID = -1
	cpu.debugBreakIn = func(uint64) bool {
		access.DisableHistory()
		return false
	}
	putIE32Instruction(cpu.memory, PROG_START, NOP, 0, ADDR_IMMEDIATE, 0)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, LDA, 0, ADDR_IMMEDIATE, 0x55)
	cpu.memory[PROG_START+2*INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if got := cpu.A; got != 0x55 {
		t.Fatalf("resumed JIT result A=%#x, want %#x", got, uint32(0x55))
	}
	if got := cpu.JITStats().DirectInstructions; got != 1 {
		t.Fatalf("resumed JIT direct instructions = %d, want 1", got)
	}
}

func TestIE32JIT_WatchpointUsesStepThenResumesGeneratedRouting(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	const watched = uint32(0x400)
	access := NewDebugAccessService()
	cpu.debugAccess = access
	cpu.debugCPUID = 0
	access.Watch(cpu.debugCPUID, uint64(watched), WORD_SIZE, WatchWrite)
	putIE32Instruction(cpu.memory, PROG_START, LDA, 0, ADDR_IMMEDIATE, 0x55)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, STORE, REG_A, ADDR_DIRECT, watched)
	cpu.memory[PROG_START+2*INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if got := cpu.Read32(watched); got != 0x55 {
		t.Fatalf("watchpoint-step store=%#x, want %#x", got, uint32(0x55))
	}
	if got := cpu.JITStats().DirectInstructions; got != 0 {
		t.Fatalf("watchpoint active direct instructions=%d, want 0", got)
	}
	if !cpu.jitEnabled {
		t.Fatal("watchpoint changed selected JIT policy")
	}
	access.ClearWatch(cpu.debugCPUID, uint64(watched))
	cpu.resetJITStats()
	cpu.PC = PROG_START
	cpu.running.Store(true)
	cpu.Execute()
	if got := cpu.JITStats().DirectInstructions; got != 2 {
		t.Fatalf("cleared watchpoint direct instructions=%d, want 2", got)
	}
}

func TestIE32JIT_DirectImmediateDivisionAndRemainder(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	putIE32Instruction(cpu.memory, PROG_START, LOAD, REG_A, ADDR_IMMEDIATE, 100)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, DIV, REG_A, ADDR_IMMEDIATE, 9)
	putIE32Instruction(cpu.memory, PROG_START+2*INSTRUCTION_SIZE, LOAD, REG_X, ADDR_IMMEDIATE, 100)
	putIE32Instruction(cpu.memory, PROG_START+3*INSTRUCTION_SIZE, MOD, REG_X, ADDR_IMMEDIATE, 9)
	putIE32Instruction(cpu.memory, PROG_START+4*INSTRUCTION_SIZE, HALT, 0, 0, 0)
	cpu.Execute()
	if cpu.A != 11 || cpu.X != 1 {
		t.Fatalf("DIV/MOD = %d/%d, want 11/1", cpu.A, cpu.X)
	}
}

func TestIE32JIT_ZeroDivisorRemainsFaultBoundary(t *testing.T) {
	cpu := NewCPU(NewMachineBus())
	putIE32Instruction(cpu.memory, PROG_START, LOAD, REG_A, ADDR_IMMEDIATE, 100)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, DIV, REG_A, ADDR_IMMEDIATE, 0)
	cpu.Execute()
	if cpu.IsRunning() {
		t.Fatal("zero divisor left CPU running")
	}
	if cpu.A != 100 {
		t.Fatalf("zero divisor changed accumulator to %d", cpu.A)
	}
}

func TestIE32JIT_InvalidOpcodeNeverGetsDirectProvenance(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	cpu.memory[PROG_START] = 0x00
	cpu.Execute()
	if cpu.IsRunning() {
		t.Fatal("invalid opcode left CPU running")
	}
	if got := cpu.JITStats().DirectInstructions; got != 0 {
		t.Fatalf("invalid opcode directly lowered %d instructions", got)
	}
	if got := cpu.InstructionCount; got != 1 {
		t.Fatalf("invalid opcode retirement = %d, want 1", got)
	}
}

func TestIE32JIT_ReservedAddressModesPreserveISAHelperSemantics(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	for _, tc := range []struct {
		name string
		op   byte
		reg  byte
		init uint32
		want uint32
	}{
		{name: "read-zero", op: LOAD, reg: REG_A, init: 0xDEADBEEF, want: 0},
		{name: "store-direct", op: STORE, reg: REG_A, init: 0xC001D00D, want: 0xC001D00D},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cpu := NewCPU(NewMachineBus())
			cpu.A = tc.init
			putIE32Instruction(cpu.memory, PROG_START, tc.op, tc.reg, 0xFF, 0x400)
			cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
			cpu.Execute()
			got := cpu.A
			if tc.op == STORE {
				got = cpu.Read32(0x400)
			}
			if got != tc.want {
				t.Fatalf("reserved mode result=%#x, want %#x", got, tc.want)
			}
			if got := cpu.JITStats().DirectInstructions; got != 0 {
				t.Fatalf("reserved mode direct instructions=%d, want 0", got)
			}
		})
	}
}

func TestIE32JIT_DirectRegisterIncrementAndDecrement(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	putIE32Instruction(cpu.memory, PROG_START, LOAD, REG_A, ADDR_IMMEDIATE, 0)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, INC, 0, ADDR_REGISTER, REG_A)
	putIE32Instruction(cpu.memory, PROG_START+2*INSTRUCTION_SIZE, INC, 0, ADDR_REGISTER, REG_A)
	putIE32Instruction(cpu.memory, PROG_START+3*INSTRUCTION_SIZE, DEC, 0, ADDR_REGISTER, REG_A)
	putIE32Instruction(cpu.memory, PROG_START+4*INSTRUCTION_SIZE, HALT, 0, 0, 0)
	cpu.Execute()
	if cpu.A != 1 {
		t.Fatalf("INC/DEC A=%d, want 1", cpu.A)
	}
}

func TestIE32JIT_DirectAbsoluteJump(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	target := uint32(PROG_START + 2*INSTRUCTION_SIZE)
	putIE32Instruction(cpu.memory, PROG_START, JMP, 0, ADDR_DIRECT, target)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, LDA, 0, ADDR_IMMEDIATE, 1)
	putIE32Instruction(cpu.memory, target, HALT, 0, 0, 0)
	cpu.Execute()
	if cpu.PC != target {
		t.Fatalf("JMP PC=%#x, want %#x", cpu.PC, target)
	}
	if cpu.A != 0 {
		t.Fatalf("JMP executed skipped instruction, A=%d", cpu.A)
	}
}

func TestIE32JIT_DirectConditionalBranches(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	for _, tc := range []struct {
		name       string
		opcode     byte
		value      uint32
		wantTarget bool
	}{
		{"JNZ taken", JNZ, 1, true}, {"JNZ fallthrough", JNZ, 0, false}, {"JZ taken", JZ, 0, true}, {"JZ fallthrough", JZ, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cpu := NewCPU(NewMachineBus())
			target := uint32(PROG_START + 3*INSTRUCTION_SIZE)
			putIE32Instruction(cpu.memory, PROG_START, LOAD, REG_A, ADDR_IMMEDIATE, tc.value)
			putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, tc.opcode, REG_A, ADDR_DIRECT, target)
			cpu.memory[PROG_START+2*INSTRUCTION_SIZE] = HALT
			cpu.memory[target] = HALT
			cpu.Execute()
			want := uint32(PROG_START + 2*INSTRUCTION_SIZE)
			if tc.wantTarget {
				want = target
			}
			if cpu.PC != uint32(want) {
				t.Fatalf("PC=%#x, want %#x", cpu.PC, want)
			}
		})
	}
}

func TestIE32JIT_DirectSignedConditionalBranches(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	for _, tc := range []struct {
		opcode byte
		value  uint32
		taken  bool
	}{
		{JGT, 1, true}, {JGT, 0, false}, {JGE, 0, true}, {JLT, 0xFFFFFFFF, true}, {JLE, 1, false}, {JLE, 0, true},
	} {
		cpu := NewCPU(NewMachineBus())
		target := uint32(PROG_START + 3*INSTRUCTION_SIZE)
		putIE32Instruction(cpu.memory, PROG_START, LOAD, REG_A, ADDR_IMMEDIATE, tc.value)
		putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, tc.opcode, REG_A, ADDR_DIRECT, target)
		cpu.memory[PROG_START+2*INSTRUCTION_SIZE], cpu.memory[target] = HALT, HALT
		cpu.Execute()
		want := uint32(PROG_START + 2*INSTRUCTION_SIZE)
		if tc.taken {
			want = target
		}
		if cpu.PC != want {
			t.Fatalf("opcode %#x value %#x PC=%#x want %#x", tc.opcode, tc.value, cpu.PC, want)
		}
	}
}

func TestIE32JIT_DirectAbsoluteRAMLoad(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	cpu.Write32(0x400, 0xC0FFEE)
	putIE32Instruction(cpu.memory, PROG_START, LOAD, REG_A, ADDR_DIRECT, 0x400)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if cpu.A != 0xC0FFEE {
		t.Fatalf("direct RAM load = %#x", cpu.A)
	}
	if got := cpu.JITStats().NativeEntries; got == 0 {
		t.Fatal("absolute RAM load did not enter generated code")
	}
}

func TestIE32JIT_VisibleRAMAndMMIOAdmissionBoundaries(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	for _, tc := range []struct {
		name       string
		address    uint32
		wantDirect uint64
	}{
		{name: "last-visible-word", address: 0x1FFC, wantDirect: 1},
		{name: "first-outside-visible-ram", address: 0x2000},
		{name: "unaligned", address: 0x1FFD},
		{name: "mmio", address: IO_REGION_START},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()
			bus.SetSizing(MemorySizing{ActiveVisibleRAM: 0x2000})
			cpu := NewCPU(bus)
			if tc.address < uint32(len(cpu.memory)-WORD_SIZE) {
				cpu.Write32(tc.address&^uint32(3), 0xC001D00D)
			}
			putIE32Instruction(cpu.memory, PROG_START, LOAD, REG_A, ADDR_DIRECT, tc.address)
			cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
			cpu.Execute()
			if got := cpu.JITStats().DirectInstructions; got != tc.wantDirect {
				t.Fatalf("direct instructions=%d, want %d", got, tc.wantDirect)
			}
		})
	}
}

func TestIE32JIT_DirectAbsoluteRAMNamedLoad(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	cpu.Write32(0x400, 0xFACECAFE)
	putIE32Instruction(cpu.memory, PROG_START, LDA, 0, ADDR_DIRECT, 0x400)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if cpu.A != 0xFACECAFE {
		t.Fatalf("direct named RAM load = %#x", cpu.A)
	}
}

func TestIE32JIT_DirectAbsoluteRAMStores(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	cpu.A = 0x11223344
	cpu.B = 0x55667788
	cpu.X = 0x99AABBCC
	putIE32Instruction(cpu.memory, PROG_START, STORE, REG_A, ADDR_DIRECT, 0x400)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, STB, 0, ADDR_DIRECT, 0x404)
	putIE32Instruction(cpu.memory, PROG_START+2*INSTRUCTION_SIZE, STX, 0, ADDR_DIRECT, 0x408)
	cpu.memory[PROG_START+3*INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if got := cpu.Read32(0x400); got != 0x11223344 {
		t.Fatalf("generic direct store = %#x", got)
	}
	if got := cpu.Read32(0x404); got != 0x55667788 {
		t.Fatalf("named direct store = %#x", got)
	}
	if got := cpu.Read32(0x408); got != 0x99AABBCC {
		t.Fatalf("named X direct store = %#x", got)
	}
	if got := cpu.JITStats().Blocks; got == 0 {
		t.Fatal("direct stores were not lowered into a generated block")
	}
}

func TestIE32JIT_DirectImmediateStoreUsesArchitecturalDestination(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	cpu.A = 0xA5A55A5A
	putIE32Instruction(cpu.memory, PROG_START, STORE, REG_A, ADDR_IMMEDIATE, 0x400)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if got := cpu.Read32(0x400); got != 0xA5A55A5A {
		t.Fatalf("immediate store = %#x, want %#x", got, uint32(0xA5A55A5A))
	}
	if got := cpu.JITStats().DirectInstructions; got != 1 {
		t.Fatalf("immediate store was not directly lowered: %d instructions", got)
	}
}

func TestIE32JIT_SelfModifyingStoreResumesBeforeStaleInstruction(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	cpu.A = HALT
	putIE32Instruction(cpu.memory, PROG_START, STORE, REG_A, ADDR_DIRECT, PROG_START+INSTRUCTION_SIZE)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, LDA, 0, ADDR_IMMEDIATE, 0x12345678)
	cpu.Execute()
	if cpu.A != HALT {
		t.Fatalf("stale instruction executed after self-modifying write, A=%#x", cpu.A)
	}
	if got := cpu.JITStats().DirectInstructions; got != 0 {
		t.Fatalf("self-modifying store directly executed %d instructions", got)
	}
}

func TestIE32JIT_DirectStackPushAndPop(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	putIE32Instruction(cpu.memory, PROG_START, LOAD, REG_A, ADDR_IMMEDIATE, 0xA5A55A5A)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, PUSH, REG_A, ADDR_DIRECT, 0)
	putIE32Instruction(cpu.memory, PROG_START+2*INSTRUCTION_SIZE, POP, REG_B, ADDR_DIRECT, 0)
	cpu.memory[PROG_START+3*INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if cpu.B != 0xA5A55A5A || cpu.SP != STACK_START {
		t.Fatalf("stack result B=%#x SP=%#x", cpu.B, cpu.SP)
	}
	if got := cpu.JITStats().DirectInstructions; got < 3 {
		t.Fatalf("direct stack instructions = %d, want at least 3", got)
	}
}

func TestIE32JIT_DirectStackPopUsesCurrentStackPointer(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	cpu.SP = STACK_START - WORD_SIZE
	cpu.Write32(cpu.SP, 0xC001D00D)
	putIE32Instruction(cpu.memory, PROG_START, POP, REG_C, ADDR_DIRECT, 0)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if cpu.C != 0xC001D00D || cpu.SP != STACK_START {
		t.Fatalf("pop C=%#x SP=%#x", cpu.C, cpu.SP)
	}
	if got := cpu.JITStats().DirectInstructions; got != 1 {
		t.Fatalf("direct stack pop = %d, want 1", got)
	}
}

func TestIE32JIT_DirectJSRAndRTS(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	target := uint32(PROG_START + 3*INSTRUCTION_SIZE)
	putIE32Instruction(cpu.memory, PROG_START, JSR, 0, ADDR_DIRECT, target)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	putIE32Instruction(cpu.memory, target, RTS, 0, ADDR_DIRECT, 0)
	cpu.Execute()
	if cpu.PC != PROG_START+2*INSTRUCTION_SIZE || cpu.SP != STACK_START {
		t.Fatalf("JSR/RTS PC=%#x SP=%#x", cpu.PC, cpu.SP)
	}
	if got := cpu.JITStats().DirectInstructions; got != 2 {
		t.Fatalf("direct JSR/RTS chain = %d, want 2", got)
	}
}

func TestIE32JIT_FusedLeafCallPreservesStackAndRetirement(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	leaf := uint32(PROG_START + 4*INSTRUCTION_SIZE)
	putIE32Instruction(cpu.memory, PROG_START, JSR, 0, ADDR_IMMEDIATE, leaf)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, LDA, 0, ADDR_IMMEDIATE, 9)
	cpu.memory[PROG_START+2*INSTRUCTION_SIZE] = HALT
	putIE32Instruction(cpu.memory, leaf, ADD, REG_A, ADDR_IMMEDIATE, 3)
	putIE32Instruction(cpu.memory, leaf+INSTRUCTION_SIZE, RTS, 0, ADDR_IMMEDIATE, 0)
	cpu.Execute()
	if cpu.A != 9 || cpu.SP != STACK_START {
		t.Fatalf("fused leaf result A=%d SP=%#x", cpu.A, cpu.SP)
	}
	if got := cpu.JITStats().DirectInstructions; got != 4 {
		t.Fatalf("fused leaf direct retirements=%d, want 4", got)
	}
}

func TestIE32JIT_KnownImmediateBranchFusion(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	target := uint32(PROG_START + 3*INSTRUCTION_SIZE)
	putIE32Instruction(cpu.memory, PROG_START, LOAD, REG_A, ADDR_IMMEDIATE, 0)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, JZ, REG_A, ADDR_IMMEDIATE, target)
	putIE32Instruction(cpu.memory, PROG_START+2*INSTRUCTION_SIZE, LDA, 0, ADDR_IMMEDIATE, 1)
	putIE32Instruction(cpu.memory, target, LDA, 0, ADDR_IMMEDIATE, 2)
	cpu.memory[target+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if cpu.A != 2 {
		t.Fatalf("known branch result A=%d, want 2", cpu.A)
	}
	if got := cpu.JITStats().DirectInstructions; got < 3 {
		t.Fatalf("known branch direct retirements=%d, want at least 3", got)
	}
}

func TestIE32JIT_AcceleratesMMIOPollLoop(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	bus := NewMachineBus()
	values := []uint32{0, 0, 1}
	bus.SetVideoStatusReader(func(uint32) uint32 {
		value := values[0]
		values = values[1:]
		return value
	})
	previousYield := ie32MMIOPollYield
	yieldCalls := 0
	ie32MMIOPollYield = func() { yieldCalls++ }
	t.Cleanup(func() { ie32MMIOPollYield = previousYield })
	cpu := NewCPU(bus)
	putIE32Instruction(cpu.memory, PROG_START, LOAD, REG_A, ADDR_DIRECT, VIDEO_STATUS)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, JZ, REG_A, ADDR_IMMEDIATE, PROG_START)
	cpu.memory[PROG_START+2*INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if cpu.A != 1 {
		t.Fatalf("poll result A=%d, want 1", cpu.A)
	}
	if got := cpu.JITStats().MMIOPollIterations; got != 3 {
		t.Fatalf("poll iterations=%d, want 3", got)
	}
	if yieldCalls != 1 {
		t.Fatalf("poll scheduler yields=%d, want 1", yieldCalls)
	}
}

func TestIE32JIT_ParksOnExhaustedVBlankPollAndResumesJIT(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	bus := NewMachineBus()
	vblank := false
	bus.SetVideoStatusReader(func(uint32) uint32 {
		if vblank {
			return videoStatusVBlank
		}
		return 0
	})
	waits := 0
	bus.SetVideoVBlankWaiter(func(time.Duration) bool {
		waits++
		vblank = true
		return true
	})
	cpu := NewCPU(bus)
	putIE32Instruction(cpu.memory, PROG_START, LOAD, REG_A, ADDR_DIRECT, VIDEO_STATUS)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, JZ, REG_A, ADDR_IMMEDIATE, PROG_START)
	putIE32Instruction(cpu.memory, PROG_START+2*INSTRUCTION_SIZE, LDA, 0, ADDR_IMMEDIATE, 7)
	cpu.memory[PROG_START+3*INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if cpu.A != 7 {
		t.Fatalf("post-VBlank native result A=%d, want 7", cpu.A)
	}
	if waits != 1 {
		t.Fatalf("VBlank waits=%d, want 1", waits)
	}
	if got := cpu.JITStats().MMIOPollParks; got != 1 {
		t.Fatalf("MMIO poll parks=%d, want 1", got)
	}
}

func TestIE32JIT_ReturnCacheHitsAfterRTS(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	target := uint32(PROG_START + 4*INSTRUCTION_SIZE)
	putIE32Instruction(cpu.memory, PROG_START, JSR, 0, ADDR_DIRECT, target)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, LDA, 0, ADDR_IMMEDIATE, 7)
	cpu.memory[PROG_START+2*INSTRUCTION_SIZE] = HALT
	putIE32Instruction(cpu.memory, target, RTS, 0, ADDR_DIRECT, 0)
	cpu.Execute() // Populate the returned-to block cache.
	cpu.PC, cpu.SP = PROG_START, STACK_START
	cpu.running.Store(true)
	cpu.Execute()
	if cpu.A != 7 {
		t.Fatalf("returned-to result A=%d, want 7", cpu.A)
	}
	if got := cpu.JITStats().ReturnCacheHits; got == 0 {
		t.Fatal("RTS did not enter the return cache")
	}
}

func TestIE32JIT_ChainBudgetStopsAtExactRetirementBoundary(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	putIE32Instruction(cpu.memory, PROG_START, JMP, 0, ADDR_DIRECT, PROG_START)
	cpu.jit.testStopAfter = ie32JITChainBlockBudget
	cpu.Execute()
	stats := cpu.JITStats()
	if got := cpu.InstructionCount; got != uint64(ie32JITChainBlockBudget) {
		t.Fatalf("retired instructions=%d, want %d", got, ie32JITChainBlockBudget)
	}
	if got := stats.DirectInstructions; got != uint64(ie32JITChainBlockBudget) {
		t.Fatalf("direct instructions=%d, want %d", got, ie32JITChainBlockBudget)
	}
	if got := stats.Chains; got != uint64(ie32JITChainBlockBudget-1) {
		t.Fatalf("chain links=%d, want %d", got, ie32JITChainBlockBudget-1)
	}
	if got := stats.ChainBudgetExits; got != 1 {
		t.Fatalf("chain budget exits=%d, want 1", got)
	}
}

func TestIE32JIT_BoundedConditionalLoopMatchesInterpreter(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	jit := NewCPU(NewMachineBus())
	interpreter := newIE32CPUConfigured(NewMachineBus(), true)
	for _, cpu := range []*CPU{jit, interpreter} {
		loop := uint32(PROG_START + INSTRUCTION_SIZE)
		putIE32Instruction(cpu.memory, PROG_START, LDA, 0, ADDR_IMMEDIATE, 3)
		putIE32Instruction(cpu.memory, loop, DEC, 0, ADDR_REGISTER, REG_A)
		putIE32Instruction(cpu.memory, loop+INSTRUCTION_SIZE, JNZ, REG_A, ADDR_IMMEDIATE, loop)
		cpu.memory[loop+2*INSTRUCTION_SIZE] = HALT
		cpu.Execute()
	}
	if jit.A != interpreter.A || jit.PC != interpreter.PC || jit.InstructionCount != interpreter.InstructionCount || jit.running.Load() != interpreter.running.Load() {
		t.Fatalf("conditional loop state jit A=%d PC=%#x retired=%d running=%t, interpreter A=%d PC=%#x retired=%d running=%t", jit.A, jit.PC, jit.InstructionCount, jit.running.Load(), interpreter.A, interpreter.PC, interpreter.InstructionCount, interpreter.running.Load())
	}
	if got := jit.JITStats().DirectInstructions; got != 7 {
		t.Fatalf("conditional loop direct instructions=%d, want 7", got)
	}
	if got := jit.JITStats().Chains; got == 0 {
		t.Fatal("conditional loop did not use bounded generated chaining")
	}
}

func TestIE32JIT_NativeCountedLoopReturnsExactRetirement(t *testing.T) {
	if !ie32JITRuntimeAvailable() || ie32JITBackend != "native" {
		t.Skip("native IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	body := uint32(PROG_START + INSTRUCTION_SIZE)
	putIE32Instruction(cpu.memory, PROG_START, LOAD, REG_B, ADDR_IMMEDIATE, 3)
	putIE32Instruction(cpu.memory, body, ADD, REG_A, ADDR_IMMEDIATE, 1)
	putIE32Instruction(cpu.memory, body+INSTRUCTION_SIZE, SUB, REG_B, ADDR_IMMEDIATE, 1)
	putIE32Instruction(cpu.memory, body+2*INSTRUCTION_SIZE, JNZ, REG_B, ADDR_IMMEDIATE, body)
	cpu.memory[body+3*INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if cpu.A != 3 || cpu.B != 0 || cpu.InstructionCount != 11 {
		t.Fatalf("counted loop state A=%d B=%d retired=%d, want 3/0/11", cpu.A, cpu.B, cpu.InstructionCount)
	}
	stats := cpu.JITStats()
	if stats.Blocks != 1 || stats.DirectInstructions != 10 || stats.NativeEntries != 1 || stats.CountedLoops != 1 {
		t.Fatalf("counted loop provenance blocks=%d direct=%d entries=%d loops=%d, want 1/10/1/1", stats.Blocks, stats.DirectInstructions, stats.NativeEntries, stats.CountedLoops)
	}
}

func TestIE32JIT_GuardedRAMCountedLoopPublishesExactWriteRange(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	const scratch = uint32(0x400)
	body := uint32(PROG_START + INSTRUCTION_SIZE)
	cpu.Write32(scratch, 9)
	putIE32Instruction(cpu.memory, PROG_START, LOAD, REG_B, ADDR_IMMEDIATE, 3)
	putIE32Instruction(cpu.memory, body, LOAD, REG_A, ADDR_DIRECT, scratch)
	putIE32Instruction(cpu.memory, body+INSTRUCTION_SIZE, STORE, REG_A, ADDR_DIRECT, scratch)
	putIE32Instruction(cpu.memory, body+2*INSTRUCTION_SIZE, SUB, REG_B, ADDR_IMMEDIATE, 1)
	putIE32Instruction(cpu.memory, body+3*INSTRUCTION_SIZE, JNZ, REG_B, ADDR_IMMEDIATE, body)
	cpu.memory[body+4*INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if cpu.A != 9 || cpu.B != 0 || cpu.Read32(scratch) != 9 || cpu.InstructionCount != 14 {
		t.Fatalf("RAM counted loop A=%d B=%d RAM=%d retired=%d", cpu.A, cpu.B, cpu.Read32(scratch), cpu.InstructionCount)
	}
	stats := cpu.JITStats()
	if stats.Blocks != 1 || stats.DirectInstructions != 13 || stats.Invalidations == 0 {
		t.Fatalf("RAM counted loop blocks=%d direct=%d invalidations=%d, want 1/13/>0", stats.Blocks, stats.DirectInstructions, stats.Invalidations)
	}
}

func TestIE32JIT_FusedLeafCountedLoopReturnsExactRetirement(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	loop := uint32(PROG_START + INSTRUCTION_SIZE)
	callee := uint32(PROG_START + 5*INSTRUCTION_SIZE)
	putIE32Instruction(cpu.memory, PROG_START, LOAD, REG_B, ADDR_IMMEDIATE, 3)
	putIE32Instruction(cpu.memory, loop, JSR, 0, ADDR_IMMEDIATE, callee)
	putIE32Instruction(cpu.memory, loop+INSTRUCTION_SIZE, SUB, REG_B, ADDR_IMMEDIATE, 1)
	putIE32Instruction(cpu.memory, loop+2*INSTRUCTION_SIZE, JNZ, REG_B, ADDR_IMMEDIATE, loop)
	cpu.memory[loop+3*INSTRUCTION_SIZE] = HALT
	putIE32Instruction(cpu.memory, callee, ADD, REG_A, ADDR_IMMEDIATE, 1)
	putIE32Instruction(cpu.memory, callee+INSTRUCTION_SIZE, RTS, 0, ADDR_IMMEDIATE, 0)
	cpu.Execute()
	if cpu.A != 3 || cpu.B != 0 || cpu.SP != STACK_START || cpu.InstructionCount != 17 {
		t.Fatalf("fused-loop state A=%d B=%d SP=%#x retired=%d", cpu.A, cpu.B, cpu.SP, cpu.InstructionCount)
	}
	stats := cpu.JITStats()
	if stats.Blocks != 1 || stats.DirectInstructions != 16 || stats.NativeEntries != 1 {
		t.Fatalf("fused-loop provenance blocks=%d direct=%d entries=%d, want 1/16/1", stats.Blocks, stats.DirectInstructions, stats.NativeEntries)
	}
}

func TestIE32JIT_ExactCheckpointStopsBeforeAnotherChain(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	putIE32Instruction(cpu.memory, PROG_START, JMP, 0, ADDR_DIRECT, PROG_START)
	cpu.jit.testStopAfter = 7
	cpu.jit.testExactRetirement = true
	cpu.Execute()
	if got := cpu.jit.testRetired; got != 7 {
		t.Fatalf("test checkpoint retired=%d, want 7", got)
	}
	if got := cpu.InstructionCount; got != 7 {
		t.Fatalf("instruction count=%d, want 7", got)
	}
}

func TestIE32JIT_InterruptControlUsesCanonicalHelper(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	putIE32Instruction(cpu.memory, PROG_START, SEI, 0, ADDR_DIRECT, 0)
	putIE32Instruction(cpu.memory, PROG_START+INSTRUCTION_SIZE, CLI, 0, ADDR_DIRECT, 0)
	cpu.memory[PROG_START+2*INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if cpu.interruptEnabled.Load() {
		t.Fatal("CLI did not clear interrupt enable")
	}
	if got := cpu.JITStats().DirectInstructions; got != 0 {
		t.Fatalf("interrupt helper directly lowered %d instructions", got)
	}
}

func TestIE32JIT_RTIUsesCanonicalHelper(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	cpu.SP = STACK_START - WORD_SIZE
	cpu.Write32(cpu.SP, PROG_START+INSTRUCTION_SIZE)
	cpu.inInterrupt.Store(true)
	putIE32Instruction(cpu.memory, PROG_START, RTI, 0, ADDR_DIRECT, 0)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if cpu.inInterrupt.Load() || cpu.SP != STACK_START {
		t.Fatalf("RTI state interrupt=%v SP=%#x", cpu.inInterrupt.Load(), cpu.SP)
	}
	if got := cpu.JITStats().DirectInstructions; got != 0 {
		t.Fatalf("RTI helper directly lowered %d instructions", got)
	}
}

func TestIE32JIT_DirectRegisterLoad(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	cpu.X = 0x12345678
	putIE32Instruction(cpu.memory, PROG_START, LOAD, REG_A, ADDR_REGISTER, REG_X)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if cpu.A != cpu.X {
		t.Fatalf("register load A=%#x X=%#x", cpu.A, cpu.X)
	}
	if got := cpu.JITStats().DirectInstructions; got != 1 {
		t.Fatalf("direct register load = %d", got)
	}
}

func TestIE32JIT_DirectNamedRegisterLoad(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	cpu.B = 0xDEADBEEF
	putIE32Instruction(cpu.memory, PROG_START, LDX, 0, ADDR_REGISTER, REG_B)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if cpu.X != cpu.B {
		t.Fatalf("named register load X=%#x B=%#x", cpu.X, cpu.B)
	}
}

func TestIE32JIT_DirectRegisterALU(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	cpu.A, cpu.X = 7, 5
	putIE32Instruction(cpu.memory, PROG_START, ADD, REG_A, ADDR_REGISTER, REG_X)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if cpu.A != 12 {
		t.Fatalf("register ADD A=%d", cpu.A)
	}
}

func TestIE32JIT_DirectRegisterIndirectLoad(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	cpu.X = 0x400
	cpu.Write32(0x408, 0xF00DBABE)
	putIE32Instruction(cpu.memory, PROG_START, LOAD, REG_A, ADDR_REG_IND, 0x8|REG_X)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if cpu.A != 0xF00DBABE {
		t.Fatalf("register indirect A=%#x", cpu.A)
	}
}

func TestIE32JIT_DirectMemoryIndirectLoad(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	cpu.Write32(0x400, 0xCAFEBABE)
	putIE32Instruction(cpu.memory, PROG_START, LOAD, REG_A, ADDR_MEM_IND, 0x400)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if cpu.A != 0xCAFEBABE {
		t.Fatalf("memory indirect A=%#x", cpu.A)
	}
	if got := cpu.JITStats().DirectInstructions; got != 1 {
		t.Fatalf("memory indirect direct instructions=%d, want 1", got)
	}
}

func TestIE32JIT_FirstMemoryIndirectOperationMatchesInterpreter(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	for _, tc := range []struct {
		name    string
		opcode  byte
		reg     byte
		initial uint32
	}{
		{name: "load", opcode: LOAD, reg: REG_A},
		{name: "lda", opcode: LDA},
		{name: "add", opcode: ADD, reg: REG_A, initial: 7},
		{name: "store", opcode: STORE, reg: REG_A, initial: 0x13579BDF},
		{name: "increment", opcode: INC},
		{name: "decrement", opcode: DEC},
	} {
		t.Run(tc.name, func(t *testing.T) {
			interpreter := NewCPU(NewMachineBus())
			jit := NewCPU(NewMachineBus())
			for _, cpu := range []*CPU{interpreter, jit} {
				cpu.A = tc.initial
				if ie32MemoryIndirectWrites(tc.opcode) {
					cpu.Write32(0x400, 0x404)
					cpu.Write32(0x404, 5)
				} else {
					cpu.Write32(0x400, 5)
				}
				putIE32Instruction(cpu.memory, PROG_START, tc.opcode, tc.reg, ADDR_MEM_IND, 0x400)
				cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
			}
			interpreter.running.Store(false)
			if err := interpreter.SetJITEnabled(false); err != nil {
				t.Fatalf("disable interpreter JIT: %v", err)
			}
			interpreter.running.Store(true)
			interpreter.Execute()
			jit.Execute()
			if interpreter.A != jit.A || interpreter.PC != jit.PC || interpreter.SP != jit.SP || interpreter.Read32(0x404) != jit.Read32(0x404) {
				t.Fatalf("memory-indirect state differs: interpreter A=%#x memory=%#x, JIT A=%#x memory=%#x", interpreter.A, interpreter.Read32(0x404), jit.A, jit.Read32(0x404))
			}
			if got := jit.JITStats().DirectInstructions; got != 1 {
				t.Fatalf("direct instructions=%d, want 1", got)
			}
		})
	}
}

func TestIE32JIT_DirectRegisterModeStore(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable")
	}
	cpu := NewCPU(NewMachineBus())
	cpu.A = 0x13579BDF
	putIE32Instruction(cpu.memory, PROG_START, STORE, REG_A, ADDR_REGISTER, 0x400)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if got := cpu.Read32(0x400); got != cpu.A {
		t.Fatalf("register-mode store=%#x", got)
	}
}

func putIE32Instruction(mem []byte, pc uint32, opcode, reg, mode byte, operand uint32) {
	mem[pc] = opcode
	mem[pc+REG_OFFSET] = reg
	mem[pc+ADDRMODE_OFFSET] = mode
	mem[pc+OPERAND_OFFSET] = byte(operand)
	mem[pc+OPERAND_OFFSET+1] = byte(operand >> 8)
	mem[pc+OPERAND_OFFSET+2] = byte(operand >> 16)
	mem[pc+OPERAND_OFFSET+3] = byte(operand >> 24)
}

func TestIE32StepOneHALTStopsProcessor(t *testing.T) {
	cpu := NewCPU(NewMachineBus())
	cpu.jitEnabled = false
	cpu.memory[PROG_START] = HALT
	cpu.StepOne()
	if cpu.IsRunning() {
		t.Fatal("StepOne HALT left the processor running")
	}
}

func TestIE32RetiredInstructionAccountingDoesNotRequirePerformanceMode(t *testing.T) {
	cpu := NewCPU(NewMachineBus())
	cpu.jitEnabled = false
	cpu.memory[PROG_START] = NOP
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	cpu.Execute()
	if got := cpu.InstructionCount; got != 2 {
		t.Fatalf("retired instructions = %d, want 2", got)
	}
}

func TestIE32JIT_IEScriptControlsAndReportsSelectedCPU(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU(bus)
	cpu.running.Store(false)
	runtimeStatus.setCPUs(runtimeCPUIE32, cpu, nil, nil, nil, nil, nil)
	t.Cleanup(func() { runtimeStatus.setCPUs(runtimeCPUNone, nil, nil, nil, nil, nil, nil) })

	se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())
	if err := se.RunString(`
		cpu.set_jit_enabled(false)
		if cpu.jit_enabled() then error("IE32 JIT still enabled") end
		if cpu.execution_mode() ~= "interpreter" then error("wrong IE32 execution mode") end
		local stats = cpu.jit_stats()
		if stats.backend == nil then error("missing IE32 backend") end
		if stats.direct_instructions == nil then error("missing IE32 direct instruction counter") end
	`, "ie32_jit_controls"); err != nil {
		t.Fatalf("RunString: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
}
