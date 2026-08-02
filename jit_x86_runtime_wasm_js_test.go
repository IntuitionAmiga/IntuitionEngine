//go:build js && wasm

package main

import "testing"

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
	cpu.X86ExecuteJIT()
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
		0x4A,       // DEC EDX
		0x75, 0xDD, // -> 0x1000
		0xF4, // HLT
	})
	code := mem[startPC : 0x1023+1]
	setup := func(cpu *CPU_X86) {
		cpu.EAX = 1
		cpu.ECX = 2
		cpu.EDX = 5
	}

	oldRegions := x86RegionPromotionEnabled
	oldThresholds := x86TierController.Thresholds
	x86RegionPromotionEnabled = true
	x86TierController.Thresholds.PromoteAtExecCount = 1
	x86TierController.Thresholds.RegionMinBlocks = 3
	t.Cleanup(func() {
		x86RegionPromotionEnabled = oldRegions
		x86TierController.Thresholds = oldThresholds
	})

	interp := runX86WasmNodeInterpreter(t, startPC, setup, code)
	jit, rt := runX86WasmNodeJIT(t, startPC, setup, code)
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
