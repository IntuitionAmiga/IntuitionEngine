//go:build arm64 && linux

package main

import (
	"bytes"
	"fmt"
	"testing"
)

// TestJIT6502_ARM64_ManifestNativeExecution is the ARM64 counterpart to the
// AMD64 manifest gate. A direct opcode may not merely compile to a bailout:
// it must retire through emitted ARM64 code and match one interpreter step.
func TestJIT6502_ARM64_ManifestNativeExecution(t *testing.T) {
	// QEMU user mode can retain a translated block after its dual-mapped RX
	// address is unmapped, even though hardware and the kernel have completed
	// the required cache sequence. Keep this suite's RX mappings live until it
	// ends so each native block has a distinct address without weakening the
	// production allocator or its lifecycle contract.
	// Earlier emitter tests intentionally allocate and free small code maps.
	// Reserve enough distinct RX aliases to move this manifest beyond those
	// addresses as well; QEMU's stale translated block is keyed by the old RX
	// virtual address rather than the current memfd backing.
	reservations := make([]*ExecMem, 0, 128)
	for range 128 {
		reservation, err := AllocExecMem(jit6502ExecMemSize)
		if err != nil {
			for _, held := range reservations {
				held.Free()
			}
			t.Fatalf("reserve ARM64 JIT address: %v", err)
		}
		reservations = append(reservations, reservation)
	}
	defer func() {
		for _, held := range reservations {
			held.Free()
		}
	}()
	var retained []*CPU_6502
	defer func() {
		for _, cpu := range retained {
			cpu.jitPersist = false
			cpu.freeJIT6502()
		}
	}()

	for _, entry := range P65OpcodeManifest {
		if entry.Decision != p65OpcodeDirect {
			continue
		}
		t.Run(fmt.Sprintf("opcode_%02X", entry.Opcode), func(t *testing.T) {
			interp := newARM64Manifest6502CPU(entry)
			// Branch handlers return only their dynamic cycle penalties, so zero
			// is a valid Step result for an executed, not-taken branch.
			interp.Step()
			// The JIT control stops exactly at the one-instruction checkpoint;
			// mirror that external stop in the interpreter control before comparing
			// architectural stop state.
			interp.SetRunning(false)

			jit := newARM64Manifest6502CPU(entry)
			jit.jitEnabled = true
			jit.jitPersist = true
			jit.jitTestStopAfter = 1
			jit.jitTestBlockLimit = 1
			jit.ExecuteJIT6502()
			if jit.jitTestRetired != 1 {
				t.Fatalf("opcode %02X retired %d instructions, want 1", entry.Opcode, jit.jitTestRetired)
			}
			if bails := jit.jit6502StatsSnapshot().bails; bails != 0 {
				t.Fatalf("opcode %02X reached the interpreter after %d native bailout(s)", entry.Opcode, bails)
			}
			assertARM64Manifest6502StateEqual(t, entry.Opcode, interp, jit)
			retained = append(retained, jit)
		})
	}
}

func newARM64Manifest6502CPU(entry P65OpcodeManifestEntry) *CPU_6502 {
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

func assertARM64Manifest6502StateEqual(t *testing.T, opcode byte, want, got *CPU_6502) {
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
