//go:build js && wasm

package main

import (
	"bytes"
	"fmt"
	"testing"
)

// TestP65WasmJIT_ManifestNativeExecution prevents an unsupported direct form
// from appearing to pass by silently falling back to the interpreter.
func TestP65WasmJIT_ManifestNativeExecution(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	for _, entry := range P65OpcodeManifest {
		if entry.Decision != p65OpcodeDirect {
			continue
		}
		t.Run(fmt.Sprintf("opcode_%02X", entry.Opcode), func(t *testing.T) {
			interp := newWasmManifest6502CPU(entry)
			if cycles := interp.Step(); cycles == 0 {
				t.Fatalf("interpreter did not retire opcode %02X", entry.Opcode)
			}

			jit := newWasmManifest6502CPU(entry)
			jit.jitEnabled = true
			jit.jitTestStopAfter = 1
			jit.jitTestBlockLimit = 1
			jit.jit6502Execute()
			if jit.jitTestRetired != 1 {
				t.Fatalf("opcode %02X retired %d instructions, want 1", entry.Opcode, jit.jitTestRetired)
			}
			if bails := jit.jit6502StatsSnapshot().bails; bails != 0 {
				t.Fatalf("opcode %02X reached the interpreter after %d wasm bailout(s)", entry.Opcode, bails)
			}
			assertWasmManifest6502StateEqual(t, entry.Opcode, interp, jit)
		})
	}
}

func newWasmManifest6502CPU(entry P65OpcodeManifestEntry) *CPU_6502 {
	bus := NewMachineBus()
	for index := 0; index < int(entry.Length); index++ {
		bus.Write8(0x0600+uint32(index), entry.Representative[index])
	}
	bus.Write8(0x00FF, 0x21)
	bus.Write8(IRQ_VECTOR, 0x10)
	bus.Write8(IRQ_VECTOR+1, 0x07)
	cpu := NewCPU_6502(bus)
	cpu.PC = 0x0600
	cpu.A, cpu.X, cpu.Y, cpu.SP, cpu.SR = 0x53, 0x00, 0x00, 0xFF, UNUSED_FLAG|CARRY_FLAG
	cpu.SetRDYLine(true)
	cpu.SetRunning(true)
	return cpu
}

func assertWasmManifest6502StateEqual(t *testing.T, opcode byte, want, got *CPU_6502) {
	t.Helper()
	if want.PC != got.PC || want.SP != got.SP || want.A != got.A || want.X != got.X || want.Y != got.Y || want.SR != got.SR || want.Cycles != got.Cycles || want.Running() != got.Running() {
		t.Fatalf("opcode %02X state mismatch: want %s, got %s", opcode, p65FixtureCPUState(want), p65FixtureCPUState(got))
	}
	wantBus := want.memory.(*Bus6502Adapter).bus.(*MachineBus)
	gotBus := got.memory.(*Bus6502Adapter).bus.(*MachineBus)
	const addressSpace = 1 << 16
	if !bytes.Equal(wantBus.memory[:addressSpace], gotBus.memory[:addressSpace]) {
		for address := 0; address < addressSpace; address++ {
			if wantBus.memory[address] != gotBus.memory[address] {
				t.Fatalf("opcode %02X RAM mismatch at $%04X: want $%02X, got $%02X", opcode, address, wantBus.memory[address], gotBus.memory[address])
			}
		}
		t.Fatal("RAM mismatch")
	}
}
