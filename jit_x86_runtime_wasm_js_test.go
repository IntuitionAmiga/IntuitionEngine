//go:build js && wasm

package main

import (
	"testing"
	"time"
)

func runX86WasmNodeInterpreter(t *testing.T, startPC uint32, setup func(*CPU_X86), code []byte) *CPU_X86 {
	t.Helper()
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	copy(cpu.memory[startPC:], code)
	cpu.EIP = startPC
	cpu.running.Store(true)
	cpu.Halted = false
	if setup != nil {
		setup(cpu)
	}
	for cpu.Running() && !cpu.Halted {
		cpu.Step()
	}
	return cpu
}

func runX86WasmNodeJIT(t *testing.T, startPC uint32, setup func(*CPU_X86), code []byte) (*CPU_X86, *x86WasmJITRuntime) {
	t.Helper()
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	cpu.x86JitEnabled = true
	cpu.x86JitPersist = true
	copy(cpu.memory[startPC:], code)
	cpu.EIP = startPC
	cpu.running.Store(true)
	cpu.Halted = false
	if setup != nil {
		setup(cpu)
	}
	done := make(chan struct{})
	go func() {
		cpu.X86ExecuteJIT()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		rt := cpu.x86GetWasmRuntime()
		t.Fatalf("x86 wasm jit timed out (EIP=%#x EAX=%#x ECX=%#x EDX=%#x Flags=%#x blocks=%d)",
			cpu.EIP, cpu.EAX, cpu.ECX, cpu.EDX, cpu.Flags, len(rt.blocks))
	}
	rt := cpu.x86GetWasmRuntime()
	if rt == nil {
		t.Fatal("x86 wasm runtime was not retained")
	}
	return cpu, rt
}

func TestX86WasmJIT_Node_EndToEndParity(t *testing.T) {
	const startPC = uint32(0x1000)
	code := []byte{
		0xBB, 0x06, 0x00, 0x00, 0x00, // MOV EBX,6
		0xEB, 0x09, // JMP 0x1010
	}
	code = append(code, make([]byte, 0x10-int(startPC+uint32(len(code))-startPC))...)
	code = append(code,
		0x40,       // INC EAX
		0x4B,       // DEC EBX
		0x75, 0xFC, // JNZ 0x1010
		0xF4, // HLT
	)
	setup := func(cpu *CPU_X86) {
		cpu.EAX = 7
	}
	interp := runX86WasmNodeInterpreter(t, startPC, setup, code)
	jit, rt := runX86WasmNodeJIT(t, startPC, setup, code)
	if jit.EAX != interp.EAX || jit.EBX != interp.EBX || jit.EIP != interp.EIP || jit.Flags != interp.Flags || jit.Cycles != interp.Cycles {
		t.Fatalf("jit state EAX/EBX/EIP/Flags/Cycles = %#x/%#x/%#x/%#x/%d, interpreter %#x/%#x/%#x/%#x/%d",
			jit.EAX, jit.EBX, jit.EIP, jit.Flags, jit.Cycles,
			interp.EAX, interp.EBX, interp.EIP, interp.Flags, interp.Cycles)
	}
	if len(rt.blocks) < 2 {
		t.Fatalf("compiled blocks=%d, want at least entry and loop body", len(rt.blocks))
	}
	if rt.blocks[startPC] == nil || rt.blocks[0x1010] == nil {
		t.Fatalf("missing compiled entry/loop blocks: have=%v", rt.blocks)
	}
}

func TestX86WasmJIT_Node_BoundedDispatchHonoursInstructionBudget(t *testing.T) {
	const startPC = uint32(0x1000)
	code := []byte{
		0xB8, 0x01, 0x00, 0x00, 0x00, // MOV EAX,1
		0xBB, 0x02, 0x00, 0x00, 0x00, // MOV EBX,2
		0xF4,
	}

	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	cpu.x86JitEnabled = true
	cpu.x86JitPersist = true
	copy(cpu.memory[startPC:], code)
	cpu.EIP = startPC
	cpu.running.Store(true)
	cpu.Halted = false
	cpu.x86BudgetActive = true
	cpu.x86InstrBudget = 1

	done := make(chan struct{})
	go func() {
		cpu.X86ExecuteJIT()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("bounded x86 wasm jit timed out (EIP=%#x EAX=%#x EBX=%#x budget=%d)",
			cpu.EIP, cpu.EAX, cpu.EBX, cpu.x86InstrBudget)
	}

	if got := cpu.x86InstrBudget; got != 0 {
		t.Fatalf("budget = %d, want 0", got)
	}
	if got := cpu.EAX; got != 1 {
		t.Fatalf("EAX = %#x, want 1", got)
	}
	if got := cpu.EBX; got != 0 {
		t.Fatalf("EBX = %#x, want 0 after one retirement", got)
	}
	if cpu.Halted {
		t.Fatal("bounded execution ran through HLT")
	}
}

func TestX86WasmJIT_Node_RegionPromotionParity(t *testing.T) {
	const startPC = uint32(0x1000)
	mem := make([]byte, 0x1100)
	copy(mem[startPC:], []byte{
		0x40,       // INC EAX
		0xEB, 0x0D, // -> 0x1010
	})
	copy(mem[0x1010:], []byte{
		0x41,       // INC ECX
		0xEB, 0x0D, // -> 0x1020
	})
	copy(mem[0x1020:], []byte{
		0x42,       // INC EDX
		0xEB, 0x0D, // -> 0x1030
	})
	copy(mem[0x1030:], []byte{
		0xF4, // HLT
	})
	code := mem[startPC:0x1031]
	setup := func(cpu *CPU_X86) {
		cpu.EAX = 1
		cpu.ECX = 2
		cpu.EDX = 3
	}

	interp := runX86WasmNodeInterpreter(t, startPC, setup, code)
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	cpu.x86JitEnabled = true
	cpu.x86JitPersist = true
	copy(cpu.memory[startPC:], code)
	cpu.EIP = startPC
	cpu.running.Store(true)
	cpu.Halted = false
	setup(cpu)
	if err := cpu.initX86JIT(); err != nil {
		t.Fatalf("init x86 wasm jit: %v", err)
	}
	rt := cpu.x86GetWasmRuntime()
	if rt == nil {
		t.Fatal("x86 wasm runtime was not retained")
	}
	promoted := rt.promoteRegion(startPC)
	if promoted == nil {
		t.Fatal("promoted entry block missing")
	}
	cpu.X86ExecuteJIT()
	jit := cpu
	entry := rt.blocks[startPC]
	if entry == nil {
		t.Fatal("promoted entry block missing")
	}
	if entry.meta == nil || entry.meta.tier != 2 {
		t.Fatalf("entry tier=%v want promoted tier 2", entry.meta)
	}
	if jit.EAX != interp.EAX || jit.ECX != interp.ECX || jit.EDX != interp.EDX || jit.EIP != interp.EIP || jit.Flags != interp.Flags {
		t.Fatalf("jit state EAX/ECX/EDX/EIP/Flags = %#x/%#x/%#x/%#x/%#x, interpreter %#x/%#x/%#x/%#x/%#x",
			jit.EAX, jit.ECX, jit.EDX, jit.EIP, jit.Flags,
			interp.EAX, interp.ECX, interp.EDX, interp.EIP, interp.Flags)
	}
}
