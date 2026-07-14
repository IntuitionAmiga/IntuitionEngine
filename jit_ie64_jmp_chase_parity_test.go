//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

import (
	"testing"
	"time"
)

// runToHaltAt loads instr words placed by build into a fresh machine, sets PC
// to PROG_START, and runs either the JIT dispatcher or the interpreter to halt.
func runToHaltAt(t *testing.T, jit bool, build func(mem []byte)) *CPU64 {
	t.Helper()
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.jitEnabled = jit
	cpu.PerfEnabled = true // count retired instructions for parity comparison
	build(cpu.memory)
	cpu.PC = uint64(PROG_START)

	done := make(chan struct{})
	go func() {
		if jit {
			cpu.ExecuteJIT()
		} else {
			cpu.Execute()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cpu.running.Store(false)
		waitDoneWithGuard(t, done)
		t.Fatal("execution timed out")
	}
	return cpu
}

// TestJIT_vs_Interpreter_StaticJumpChase verifies the dispatcher static-JMP
// chase (Technique 1) lands on the same PC, produces the same register state,
// and retires the same instruction count as the interpreter for a run of
// static unconditional jumps (BRA and JMP R0) forming a trampoline chain.
func TestJIT_vs_Interpreter_StaticJumpChase(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	base := uint64(PROG_START)
	build := func(mem []byte) {
		// base+0x00 : BRA +0x40            -> base+0x40
		copy(mem[base:], ie64Instr(OP_BRA, 0, 0, 0, 0, 0, uint32(0x40)))
		// base+0x40 : JMP R0, base+0x80    -> base+0x80
		copy(mem[base+0x40:], ie64Instr(OP_JMP, 0, 0, 0, 0, 0, uint32(base+0x80)))
		// base+0x80 : BRA +0x80            -> base+0x100
		copy(mem[base+0x80:], ie64Instr(OP_BRA, 0, 0, 0, 0, 0, uint32(0x80)))
		// base+0x100: MOVE.Q R1, #0x42 ; HALT
		copy(mem[base+0x100:], ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0x42))
		copy(mem[base+0x108:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	}

	jitCPU := runToHaltAt(t, true, build)
	interpCPU := runToHaltAt(t, false, build)

	if jitCPU.regs[1] != 0x42 {
		t.Fatalf("JIT R1 = 0x%X, want 0x42", jitCPU.regs[1])
	}
	if jitCPU.PC != interpCPU.PC {
		t.Fatalf("PC mismatch: JIT 0x%X, interp 0x%X", jitCPU.PC, interpCPU.PC)
	}
	if jitCPU.InstructionCount != interpCPU.InstructionCount {
		t.Fatalf("retired count mismatch: JIT %d, interp %d",
			jitCPU.InstructionCount, interpCPU.InstructionCount)
	}
	for i := range jitCPU.regs {
		if jitCPU.regs[i] != interpCPU.regs[i] {
			t.Fatalf("R%d mismatch: JIT 0x%X, interp 0x%X", i, jitCPU.regs[i], interpCPU.regs[i])
		}
	}
}
