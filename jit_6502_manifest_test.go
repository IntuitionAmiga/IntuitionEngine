//go:build amd64 && linux

package main

import (
	"bytes"
	"fmt"
	"testing"
)

func TestJIT6502_ManifestInterpreterDispatchInventory(t *testing.T) {
	for opcode, entry := range P65OpcodeManifest {
		if entry.Opcode != byte(opcode) || entry.Length == 0 || entry.Length > 3 || entry.BackendPath == "" || entry.ProvingTest == "" {
			t.Fatalf("manifest entry %02X is incomplete: %+v", opcode, entry)
		}
		if entry.Representative[0] != byte(opcode) {
			t.Fatalf("manifest entry %02X representative=%02X", opcode, entry.Representative[0])
		}
		switch entry.Decision {
		case p65OpcodeDirect:
			if !jit6502IsCompilable[opcode] || entry.BackendPath != "native" {
				t.Fatalf("direct entry %02X does not match native admission", opcode)
			}
		case p65OpcodeHalt:
			if jit6502IsCompilable[opcode] || entry.BackendPath != "halt" {
				t.Fatalf("halt entry %02X does not match JAM policy", opcode)
			}
		case p65OpcodeInterpreterFallback:
			if jit6502IsCompilable[opcode] || entry.BackendPath != "interpreter-fallback" {
				t.Fatalf("fallback entry %02X does not match policy", opcode)
			}
		default:
			t.Fatalf("entry %02X has unknown decision %d", opcode, entry.Decision)
		}
	}
}

func TestJIT6502_ManifestNativeAdmission(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()
	for _, entry := range P65OpcodeManifest {
		if entry.Decision != p65OpcodeDirect {
			continue
		}
		instr := JIT6502Instr{opcode: entry.Opcode, length: entry.Length}
		if entry.Length >= 2 {
			instr.operand = uint16(entry.Representative[1])
		}
		if entry.Length == 3 {
			instr.operand |= uint16(entry.Representative[2]) << 8
		}
		if _, err := compileBlock6502([]JIT6502Instr{instr}, 0x0600, rig.execMem, &rig.cpu.codePageBitmap); err != nil {
			t.Fatalf("native admission opcode %02X: %v", entry.Opcode, err)
		}
	}
}

// TestJIT6502_ManifestNativeExecution turns the manifest into a semantic gate:
// every official opcode admitted by the AMD64 backend must retire natively and
// leave the same architectural state as one interpreter step.
func TestJIT6502_ManifestNativeExecution(t *testing.T) {
	for _, entry := range P65OpcodeManifest {
		if entry.Decision != p65OpcodeDirect {
			continue
		}
		t.Run(fmt.Sprintf("opcode_%02X", entry.Opcode), func(t *testing.T) {
			interp := newManifest6502CPU(entry)
			// Branch handlers return only dynamic penalties, so zero cycles does
			// not mean that a not-taken branch failed to retire.
			interp.Step()
			interp.SetRunning(false)

			jit := newManifest6502CPU(entry)
			jit.jitTestStopAfter = 1
			jit.jitTestBlockLimit = 1
			jit.ExecuteJIT6502()
			if jit.jitTestRetired != 1 {
				t.Fatalf("native opcode %02X retired %d instructions, want 1", entry.Opcode, jit.jitTestRetired)
			}
			assertManifest6502StateEqual(t, entry.Opcode, interp, jit)
		})
	}
}

func newManifest6502CPU(entry P65OpcodeManifestEntry) *CPU_6502 {
	bus := NewMachineBus()
	for index := 0; index < int(entry.Length); index++ {
		bus.Write8(0x0600+uint32(index), entry.Representative[index])
	}
	bus.Write8(0x00FF, 0x21) // deterministic PLP/RTI stack source
	bus.Write8(IRQ_VECTOR, 0x10)
	bus.Write8(IRQ_VECTOR+1, 0x07)
	cpu := NewCPU_6502(bus)
	cpu.PC = 0x0600
	cpu.A, cpu.X, cpu.Y, cpu.SP, cpu.SR = 0x53, 0x00, 0x00, 0xFF, UNUSED_FLAG|CARRY_FLAG
	cpu.SetRDYLine(true)
	cpu.SetRunning(true)
	return cpu
}

func assertManifest6502StateEqual(t *testing.T, opcode byte, want, got *CPU_6502) {
	t.Helper()
	if want.PC != got.PC || want.SP != got.SP || want.A != got.A || want.X != got.X ||
		want.Y != got.Y || want.SR != got.SR || want.Cycles != got.Cycles || want.Running() != got.Running() {
		t.Fatalf("opcode %02X state mismatch: want %s, got %s", opcode, p65FixtureCPUState(want), p65FixtureCPUState(got))
	}
	wantBus := want.memory.(*Bus6502Adapter).bus.(*MachineBus)
	gotBus := got.memory.(*Bus6502Adapter).bus.(*MachineBus)
	const p65AddressSpace = 1 << 16
	if !bytes.Equal(wantBus.memory[:p65AddressSpace], gotBus.memory[:p65AddressSpace]) {
		for address := 0; address < p65AddressSpace; address++ {
			if wantBus.memory[address] != gotBus.memory[address] {
				t.Fatalf("opcode %02X RAM mismatch at $%04X: want $%02X, got $%02X", opcode, address, wantBus.memory[address], gotBus.memory[address])
			}
		}
		t.Fatal("RAM mismatch")
	}
}
