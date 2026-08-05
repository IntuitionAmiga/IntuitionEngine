//go:build js && wasm

package main

import (
	"runtime"
	"testing"
	"time"
)

func TestP65WasmJIT_NodeYieldsAndAcknowledgesReset(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}

	t.Run("compiled_block_yields", func(t *testing.T) {
		savedLast, savedSkip, savedCalls := lastWasmYield, yieldSkipLeft, yieldCallsSince
		defer func() {
			lastWasmYield, yieldSkipLeft, yieldCallsSince = savedLast, savedSkip, savedCalls
		}()
		wasmResetYieldThrottle()

		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		bus.Write8(0x0600, 0xEA) // NOP
		bus.Write8(0x0601, 0x02) // JAM fallback
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		cpu.jitEnabled = true
		cpu.jitTestStopAfter = 1
		cpu.jit6502Execute()

		if yieldCallsSince == 0 {
			t.Fatal("compiled wasm block did not reach the cooperative-yield throttle")
		}
	})

	t.Run("reset_handshake", func(t *testing.T) {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		bus.Write8(0x0600, 0xEA) // NOP
		bus.Write8(0x0601, 0x02) // JAM fallback
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		cpu.jitEnabled = true
		cpu.jitTestStopAfter = 1
		cpu.resetting.Store(true)
		done := make(chan struct{})
		go func() {
			cpu.jit6502Execute()
			close(done)
		}()

		deadline := time.Now().Add(time.Second)
		for !cpu.resetAck.Load() {
			select {
			case <-done:
				t.Fatal("wasm dispatcher exited before acknowledging reset")
			default:
			}
			if time.Now().After(deadline) {
				t.Fatal("wasm dispatcher did not acknowledge reset")
			}
			runtime.Gosched()
		}
		if !cpu.executing.Load() {
			t.Fatal("wasm dispatcher acknowledged reset without marking itself executing")
		}
		cpu.resetting.Store(false)
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("wasm dispatcher did not resume after reset")
		}
	})
}

func TestP65WasmJIT_NodeImmediateParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{
		0xA9, 0x80, // LDA #$80
		0xAA,       // TAX
		0xE8,       // INX
		0xCA,       // DEX
		0x29, 0xF0, // AND #$F0
		0x09, 0x03, // ORA #$03
		0x49, 0x0F, // EOR #$0F
		0x38, // SEC
		0x02, // trailing JAM
	}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT PC=%04X A=%02X X=%02X Y=%02X SR=%02X cycles=%d; interpreter PC=%04X A=%02X X=%02X Y=%02X SR=%02X cycles=%d",
			jit.PC, jit.A, jit.X, jit.Y, jit.SR, jit.Cycles, interp.PC, interp.A, interp.X, interp.Y, interp.SR, interp.Cycles)
	}
	if got := jit.jit6502StatsSnapshot().nativeEntries; got == 0 {
		t.Fatal("wasm dispatcher did not execute a compiled block")
	}
}

func TestP65WasmJIT_NodeStructuredPrefixAndSMCBoundary(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	t.Run("straight_line_prefix", func(t *testing.T) {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		for i, value := range []byte{0xA9, 0x80, 0xAA, 0xE8, 0x02} { // LDA; TAX; INX; JAM
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		cpu.jitEnabled = true
		cpu.jitTestStopAfter = 3
		cpu.jit6502Execute()
		if got := cpu.jitTestRetired; got != 3 {
			t.Fatalf("retired=%d, want 3", got)
		}
		if got := cpu.jit6502StatsSnapshot().nativeEntries; got != 1 {
			t.Fatalf("native entries=%d, want one compiled straight-line prefix", got)
		}
		if cpu.A != 0x80 || cpu.X != 0x81 || cpu.PC != 0x0604 {
			t.Fatalf("state A=%02X X=%02X PC=%04X", cpu.A, cpu.X, cpu.PC)
		}
	})
	t.Run("store_ends_prefix_before_modified_code", func(t *testing.T) {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		// STA overwrites the following NOP at $0606 with JAM. A module that
		// continued beyond STA would execute stale precompiled code instead.
		for i, value := range []byte{0xA9, 0x02, 0x8D, 0x06, 0x06, 0xEA, 0xEA, 0x00} {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		cpu.jitEnabled = true
		cpu.jit6502Execute()
		if cpu.Running() {
			t.Fatal("self-modified JAM did not stop execution")
		}
		if got := bus.Read8(0x0606); got != 0x02 {
			t.Fatalf("modified opcode=%02X, want JAM", got)
		}
	})
	t.Run("absolute_x_store_ends_prefix_before_modified_code", func(t *testing.T) {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		// STA $0608,X overwrites the second NOP with JAM. The compiled prefix
		// must end at the store, so source validation sees the replacement
		// before the old NOP can execute.
		for i, value := range []byte{0xA9, 0x02, 0xA2, 0x00, 0x9D, 0x08, 0x06, 0xEA, 0xEA, 0xEA} {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		cpu.jitEnabled = true
		cpu.jit6502Execute()
		if cpu.Running() {
			t.Fatal("self-modified JAM did not stop execution")
		}
		if got := bus.Read8(0x0608); got != 0x02 {
			t.Fatalf("modified opcode=%02X, want JAM", got)
		}
		if cpu.PC != 0x0608 {
			t.Fatalf("stale compiled instruction ran: PC=%04X, want 0608", cpu.PC)
		}
	})
}

func TestP65WasmJIT_NodeDecimalImmediateParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0xF8, 0xA9, 0x45, 0x38, 0x69, 0x55, 0x02}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.A != interp.A || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.SR, jit.PC, jit.Cycles, interp.A, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestP65WasmJIT_NodeBinaryImmediateParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	// ADC stays in binary mode and must still execute through the result table.
	program := []byte{0xA9, 0x45, 0x38, 0x69, 0x55, 0x02}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.A != interp.A || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.SR, jit.PC, jit.Cycles, interp.A, interp.SR, interp.PC, interp.Cycles)
	}
	if got := jit.jit6502StatsSnapshot().nativeEntries; got == 0 {
		t.Fatal("binary ADC did not execute through wasm native code")
	}
}

func TestP65WasmJIT_NodeConditionalBranchParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	// First BNE is taken, second falls through after LDA #0 sets Z.
	program := []byte{0xD0, 0x01, 0xEA, 0xA9, 0x00, 0xD0, 0x01, 0x02}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.A != interp.A || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.SR, jit.PC, jit.Cycles, interp.A, interp.SR, interp.PC, interp.Cycles)
	}
	if got := jit.jit6502StatsSnapshot().nativeEntries; got < 2 {
		t.Fatalf("native entries=%d, want wasm branch execution", got)
	}
}

func TestP65WasmJIT_NodeStackParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0xA9, 0x80, 0x48, 0xA9, 0x00, 0x68, 0x08, 0x28, 0x02}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SP = 0
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.A != interp.A || jit.SP != interp.SP || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jitBus.Read8(0x0100) != interpBus.Read8(0x0100) {
		t.Fatalf("JIT A=%02X SP=%02X SR=%02X PC=%04X cycles=%d stack=%02X; interpreter A=%02X SP=%02X SR=%02X PC=%04X cycles=%d stack=%02X",
			jit.A, jit.SP, jit.SR, jit.PC, jit.Cycles, jitBus.Read8(0x0100), interp.A, interp.SP, interp.SR, interp.PC, interp.Cycles, interpBus.Read8(0x0100))
	}
}

func TestP65WasmJIT_NodeAccumulatorShiftParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0xA9, 0x81, 0x38, 0x0A, 0x2A, 0x38, 0x6A, 0x4A, 0x02}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.A != interp.A || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.SR, jit.PC, jit.Cycles, interp.A, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestP65WasmJIT_NodeImmediateCompareParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0xA9, 0x10, 0xA2, 0x80, 0xA0, 0x10, 0xC9, 0x10, 0xE0, 0x81, 0xC0, 0x11, 0x02}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.X, jit.Y, jit.SR, jit.PC, jit.Cycles, interp.A, interp.X, interp.Y, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestP65WasmJIT_NodeDirectINCDECParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0xE6, 0x10, 0xCE, 0x11, 0x00, 0xA2, 0x02, 0xF6, 0xFF, 0xD6, 0xFE, 0x02}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		bus.Write8(0x0010, 0xFF)
		bus.Write8(0x0011, 0x00)
		bus.Write8(0x0001, 0xFF)
		bus.Write8(0x0000, 0x00)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jitBus.Read8(0x10) != interpBus.Read8(0x10) || jitBus.Read8(0x11) != interpBus.Read8(0x11) || jitBus.Read8(0x00) != interpBus.Read8(0x00) || jitBus.Read8(0x01) != interpBus.Read8(0x01) {
		t.Fatalf("JIT SR=%02X PC=%04X cycles=%d mem=%02X/%02X/%02X/%02X; interpreter SR=%02X PC=%04X cycles=%d mem=%02X/%02X/%02X/%02X",
			jit.SR, jit.PC, jit.Cycles, jitBus.Read8(0x10), jitBus.Read8(0x11), jitBus.Read8(0x00), jitBus.Read8(0x01), interp.SR, interp.PC, interp.Cycles, interpBus.Read8(0x10), interpBus.Read8(0x11), interpBus.Read8(0x00), interpBus.Read8(0x01))
	}
}

func TestP65WasmJIT_NodeZeroPageIndexedLogicParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0xA2, 0x02, 0xA9, 0xF0, 0x35, 0xFE, 0x15, 0xFD, 0x55, 0xFC, 0x02}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x0000, 0xCC)
		bus.Write8(0x00FF, 0x03)
		bus.Write8(0x00FE, 0x0F)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.A != interp.A || jit.X != interp.X || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X X=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X X=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.X, jit.SR, jit.PC, jit.Cycles, interp.A, interp.X, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestP65WasmJIT_NodeJSRRTSParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0x20, 0x06, 0x06, 0x02, 0xEA, 0xEA, 0x60}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SP = 0
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.SP != interp.SP || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jitBus.Read8(0x0100) != interpBus.Read8(0x0100) || jitBus.Read8(0x01FF) != interpBus.Read8(0x01FF) {
		t.Fatalf("JIT SP=%02X SR=%02X PC=%04X cycles=%d stack=%02X/%02X; interpreter SP=%02X SR=%02X PC=%04X cycles=%d stack=%02X/%02X",
			jit.SP, jit.SR, jit.PC, jit.Cycles, jitBus.Read8(0x0100), jitBus.Read8(0x01FF), interp.SP, interp.SR, interp.PC, interp.Cycles, interpBus.Read8(0x0100), interpBus.Read8(0x01FF))
	}
}

func TestP65WasmJIT_NodeJMPIndirectPageWrapParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0x6C, 0xFF, 0x10, 0xEA, 0xEA, 0xEA, 0xEA, 0x02}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x10FF, 0x07)
		bus.Write8(0x1000, 0x06)
		bus.Write8(0x1100, 0x05)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.PC != interp.PC || jit.Cycles != interp.Cycles || jit.Running() != interp.Running() {
		t.Fatalf("JIT PC=%04X cycles=%d running=%v; interpreter PC=%04X cycles=%d running=%v",
			jit.PC, jit.Cycles, jit.Running(), interp.PC, interp.Cycles, interp.Running())
	}
}

func TestP65WasmJIT_NodeRTIParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0x40, 0xEA, 0xEA, 0xEA, 0xEA, 0xEA, 0x02}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x01FD, BREAK_FLAG|CARRY_FLAG)
		bus.Write8(0x01FE, 0x06)
		bus.Write8(0x01FF, 0x06)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC, cpu.SP = 0x0600, 0xFC
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.SP != interp.SP || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jit.Running() != interp.Running() {
		t.Fatalf("JIT SP=%02X SR=%02X PC=%04X cycles=%d running=%v; interpreter SP=%02X SR=%02X PC=%04X cycles=%d running=%v",
			jit.SP, jit.SR, jit.PC, jit.Cycles, jit.Running(), interp.SP, interp.SR, interp.PC, interp.Cycles, interp.Running())
	}
}

func TestP65WasmJIT_NodeBRKParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		bus.Write8(IRQ_VECTOR, 0x06)
		bus.Write8(IRQ_VECTOR+1, 0x06)
		bus.Write8(0x0600, 0x00)
		bus.Write8(0x0601, 0xEA)
		bus.Write8(0x0606, 0x02)
		cpu := NewCPU_6502(bus)
		cpu.PC, cpu.SP = 0x0600, 0
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.SP != interp.SP || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jit.Running() != interp.Running() || jitBus.Read8(0x0100) != interpBus.Read8(0x0100) || jitBus.Read8(0x01FF) != interpBus.Read8(0x01FF) || jitBus.Read8(0x01FE) != interpBus.Read8(0x01FE) {
		t.Fatalf("JIT SP=%02X SR=%02X PC=%04X cycles=%d stack=%02X/%02X/%02X; interpreter SP=%02X SR=%02X PC=%04X cycles=%d stack=%02X/%02X/%02X",
			jit.SP, jit.SR, jit.PC, jit.Cycles, jitBus.Read8(0x0100), jitBus.Read8(0x01FF), jitBus.Read8(0x01FE), interp.SP, interp.SR, interp.PC, interp.Cycles, interpBus.Read8(0x0100), interpBus.Read8(0x01FF), interpBus.Read8(0x01FE))
	}
}

func TestP65WasmJIT_NodeAbsoluteIndexedStoreParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0xA9, 0x5A, 0xA2, 0x02, 0x9D, 0xFE, 0x01, 0xA0, 0x03, 0x99, 0xFD, 0x01, 0x02}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jitBus.Read8(0x0200) != interpBus.Read8(0x0200) {
		t.Fatalf("JIT A=%02X X=%02X Y=%02X PC=%04X cycles=%d mem=%02X; interpreter A=%02X X=%02X Y=%02X PC=%04X cycles=%d mem=%02X",
			jit.A, jit.X, jit.Y, jit.PC, jit.Cycles, jitBus.Read8(0x0200), interp.A, interp.X, interp.Y, interp.PC, interp.Cycles, interpBus.Read8(0x0200))
	}
}

func TestP65WasmJIT_NodeIndirectStoreParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0xA9, 0x5A, 0xA2, 0x02, 0x81, 0xFE, 0xA0, 0x03, 0x91, 0x10, 0x02}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		bus.Write8(0x0000, 0x00)
		bus.Write8(0x0001, 0x02)
		bus.Write8(0x0010, 0xFD)
		bus.Write8(0x0011, 0x01)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jitBus.Read8(0x0200) != interpBus.Read8(0x0200) {
		t.Fatalf("JIT A=%02X X=%02X Y=%02X PC=%04X cycles=%d mem=%02X; interpreter A=%02X X=%02X Y=%02X PC=%04X cycles=%d mem=%02X",
			jit.A, jit.X, jit.Y, jit.PC, jit.Cycles, jitBus.Read8(0x0200), interp.A, interp.X, interp.Y, interp.PC, interp.Cycles, interpBus.Read8(0x0200))
	}
}

func TestP65WasmJIT_NodeIndirectLoadParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0xA2, 0x02, 0xA1, 0xFE, 0xA0, 0x03, 0xB1, 0x10, 0x02}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		bus.Write8(0x0000, 0x00)
		bus.Write8(0x0001, 0x02)
		bus.Write8(0x0010, 0xFD)
		bus.Write8(0x0011, 0x01)
		bus.Write8(0x0200, 0x80)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jitBus.Read8(0x0200) != interpBus.Read8(0x0200) {
		t.Fatalf("JIT A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.X, jit.Y, jit.SR, jit.PC, jit.Cycles, interp.A, interp.X, interp.Y, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestP65WasmJIT_NodeIndirectLogicParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0xA9, 0xF0, 0xA2, 0x02, 0x01, 0xFE, 0x21, 0xFE, 0x41, 0xFE, 0xA0, 0x03, 0x11, 0x10, 0x31, 0x10, 0x51, 0x10, 0xA9, 0x55, 0xC1, 0xFE, 0xA9, 0x54, 0xD1, 0x10, 0x02}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		bus.Write8(0x0000, 0x00)
		bus.Write8(0x0001, 0x02)
		bus.Write8(0x0010, 0xFD)
		bus.Write8(0x0011, 0x01)
		bus.Write8(0x0200, 0x0F)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	_, interp := newCPU()
	interp.Execute()
	_, jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.X, jit.Y, jit.SR, jit.PC, jit.Cycles, interp.A, interp.X, interp.Y, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestP65WasmJIT_NodeAbsoluteIndexedLoadParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0xA2, 0x02, 0xBD, 0x00, 0x02, 0xA0, 0x03, 0xBE, 0xFD, 0x01, 0x02}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x0202, 0x80)
		bus.Write8(0x0200, 0x7F)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.X, jit.Y, jit.SR, jit.PC, jit.Cycles, interp.A, interp.X, interp.Y, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestP65WasmJIT_NodeDirectCompareParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0xA9, 0x10, 0xA2, 0x80, 0xA0, 0x10, 0xC5, 0x10, 0xEC, 0x11, 0x00, 0xCC, 0x12, 0x00, 0x02}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x0010, 0x10)
		bus.Write8(0x0011, 0x81)
		bus.Write8(0x0012, 0x11)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.X, jit.Y, jit.SR, jit.PC, jit.Cycles, interp.A, interp.X, interp.Y, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestP65WasmJIT_NodeDirectArithmeticParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0xA9, 0x45, 0x38, 0x65, 0x10, 0xF8, 0xED, 0x11, 0x00, 0x02}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x0010, 0x55)
		bus.Write8(0x0011, 0x01)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.A != interp.A || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.SR, jit.PC, jit.Cycles, interp.A, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestP65WasmJIT_NodeIndirectArithmeticParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0xA2, 0x02, 0xA0, 0x03, 0xA9, 0x45, 0x38, 0x61, 0xFE, 0xF8, 0xA9, 0x45, 0x38, 0x71, 0x10, 0xA9, 0x45, 0x38, 0xE1, 0xFE, 0xD8, 0xA9, 0x45, 0x38, 0xF1, 0x10, 0x02}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x0000, 0x00)
		bus.Write8(0x0001, 0x02)
		bus.Write8(0x0010, 0xFD)
		bus.Write8(0x0011, 0x01)
		bus.Write8(0x0200, 0x55)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.X, jit.Y, jit.SR, jit.PC, jit.Cycles, interp.A, interp.X, interp.Y, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestP65WasmJIT_NodeZeroPageIndexedArithmeticParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0xA2, 0x02, 0xA9, 0x45, 0x38, 0x75, 0xFE, 0xF8, 0xA9, 0x45, 0x38, 0xF5, 0xFE, 0xA9, 0x54, 0xD5, 0xFE, 0x02}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x0000, 0x55)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.A != interp.A || jit.X != interp.X || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X X=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X X=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.X, jit.SR, jit.PC, jit.Cycles, interp.A, interp.X, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestP65WasmJIT_NodeAbsoluteIndexedArithmeticParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0xA2, 0x02, 0xA0, 0x03, 0xA9, 0x45, 0x38, 0x7D, 0xFE, 0x01, 0xF8, 0xA9, 0x45, 0x38, 0x79, 0xFD, 0x01, 0xA9, 0x45, 0x38, 0xFD, 0xFE, 0x01, 0xD8, 0xA9, 0x45, 0x38, 0xF9, 0xFD, 0x01, 0x02}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x0200, 0x55)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d",
			jit.A, jit.X, jit.Y, jit.SR, jit.PC, jit.Cycles, interp.A, interp.X, interp.Y, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestP65WasmJIT_NodeAbsoluteIndexedLogicParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0xA2, 0x02, 0xA0, 0x03, 0xA9, 0xF0, 0x1D, 0xFE, 0x01, 0x3D, 0xFE, 0x01, 0x5D, 0xFE, 0x01, 0x19, 0xFD, 0x01, 0x39, 0xFD, 0x01, 0x59, 0xFD, 0x01, 0x02}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x0200, 0x0F)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d", jit.A, jit.X, jit.Y, jit.SR, jit.PC, jit.Cycles, interp.A, interp.X, interp.Y, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestP65WasmJIT_NodeAbsoluteIndexedCompareParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	// Both CMP forms cross from $01FE into $0200, proving the result is still
	// applied and that the page-cross cycle is retained.
	program := []byte{0xA2, 0x02, 0xA0, 0x03, 0xA9, 0x55, 0xDD, 0xFE, 0x01, 0xD9, 0xFD, 0x01, 0x02}
	newCPU := func() *CPU_6502 {
		bus := NewMachineBus()
		bus.Write8(0x0200, 0x55)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return cpu
	}
	interp := newCPU()
	interp.Execute()
	jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.A != interp.A || jit.X != interp.X || jit.Y != interp.Y || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d; interpreter A=%02X X=%02X Y=%02X SR=%02X PC=%04X cycles=%d", jit.A, jit.X, jit.Y, jit.SR, jit.PC, jit.Cycles, interp.A, interp.X, interp.Y, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestP65WasmJIT_NodeDirectShiftParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0x06, 0x10, 0x46, 0x11, 0x38, 0x26, 0x12, 0x6E, 0x13, 0x00, 0x02}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		bus.Write8(0x0010, 0x80)
		bus.Write8(0x0011, 0x01)
		bus.Write8(0x0012, 0x80)
		bus.Write8(0x0013, 0x01)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	for _, address := range []uint32{0x10, 0x11, 0x12, 0x13} {
		if got, want := jitBus.Read8(address), interpBus.Read8(address); got != want {
			t.Fatalf("memory[$%04X]=%02X, want %02X", address, got, want)
		}
	}
	if jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles {
		t.Fatalf("JIT SR=%02X PC=%04X cycles=%d; interpreter SR=%02X PC=%04X cycles=%d", jit.SR, jit.PC, jit.Cycles, interp.SR, interp.PC, interp.Cycles)
	}
}

func TestP65WasmJIT_NodeZeroPageIndexedShiftParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0xA2, 0x02, 0x16, 0xFE, 0x38, 0x36, 0xFF, 0x56, 0xFE, 0x38, 0x76, 0xFF, 0x02}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		bus.Write8(0x0000, 0x80)
		bus.Write8(0x0001, 0x80)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jitBus.Read8(0) != interpBus.Read8(0) || jitBus.Read8(1) != interpBus.Read8(1) {
		t.Fatalf("JIT SR=%02X PC=%04X cycles=%d mem=%02X/%02X; interpreter SR=%02X PC=%04X cycles=%d mem=%02X/%02X", jit.SR, jit.PC, jit.Cycles, jitBus.Read8(0), jitBus.Read8(1), interp.SR, interp.PC, interp.Cycles, interpBus.Read8(0), interpBus.Read8(1))
	}
}

func TestP65WasmJIT_NodeAbsoluteIndexedINCDECParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0xA2, 0x02, 0xFE, 0xFE, 0x01, 0xDE, 0xFE, 0x01, 0x02}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		bus.Write8(0x0200, 0xFF)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jitBus.Read8(0x0200) != interpBus.Read8(0x0200) {
		t.Fatalf("JIT SR=%02X PC=%04X cycles=%d mem=%02X; interpreter SR=%02X PC=%04X cycles=%d mem=%02X", jit.SR, jit.PC, jit.Cycles, jitBus.Read8(0x0200), interp.SR, interp.PC, interp.Cycles, interpBus.Read8(0x0200))
	}
}

func TestP65WasmJIT_NodeAbsoluteIndexedShiftParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0xA2, 0x02, 0x1E, 0xFE, 0x01, 0x38, 0x3E, 0xFF, 0x01, 0x5E, 0xFE, 0x01, 0x38, 0x7E, 0xFF, 0x01, 0x02}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		bus.Write8(0x0200, 0x80)
		bus.Write8(0x0201, 0x80)
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jitBus.Read8(0x0200) != interpBus.Read8(0x0200) || jitBus.Read8(0x0201) != interpBus.Read8(0x0201) {
		t.Fatalf("JIT SR=%02X PC=%04X cycles=%d mem=%02X/%02X; interpreter SR=%02X PC=%04X cycles=%d mem=%02X/%02X", jit.SR, jit.PC, jit.Cycles, jitBus.Read8(0x0200), jitBus.Read8(0x0201), interp.SR, interp.PC, interp.Cycles, interpBus.Read8(0x0200), interpBus.Read8(0x0201))
	}
}

func TestP65WasmJIT_NodeDirectRAMLoadStoreParity(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	program := []byte{0xA9, 0x42, 0x85, 0x10, 0x24, 0x10, 0xA2, 0x02, 0x95, 0xFF, 0xB5, 0x0E, 0x02}
	newCPU := func() (*MachineBus, *CPU_6502) {
		bus := NewMachineBus()
		cpu := NewCPU_6502(bus)
		for i, value := range program {
			bus.Write8(0x0600+uint32(i), value)
		}
		cpu.PC = 0x0600
		cpu.SetRDYLine(true)
		cpu.SetRunning(true)
		return bus, cpu
	}
	interpBus, interp := newCPU()
	interp.Execute()
	jitBus, jit := newCPU()
	jit.jitEnabled = true
	jit.jit6502Execute()
	if jit.A != interp.A || jit.SR != interp.SR || jit.PC != interp.PC || jit.Cycles != interp.Cycles || jitBus.Read8(0x10) != interpBus.Read8(0x10) || jitBus.Read8(0x01) != interpBus.Read8(0x01) {
		t.Fatalf("JIT A=%02X SR=%02X PC=%04X cycles=%d mem10=%02X mem01=%02X; interpreter A=%02X SR=%02X PC=%04X cycles=%d mem10=%02X mem01=%02X",
			jit.A, jit.SR, jit.PC, jit.Cycles, jitBus.Read8(0x10), jitBus.Read8(0x01), interp.A, interp.SR, interp.PC, interp.Cycles, interpBus.Read8(0x10), interpBus.Read8(0x01))
	}
}

func TestP65WasmJIT_NodeMappedLoadBailsBeforeSideEffect(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	bus := NewMachineBus()
	reads := 0
	bus.MapIO(0x0010, 0x0010, func(uint32) uint32 {
		reads++
		return 0x37
	}, nil)
	cpu := NewCPU_6502(bus)
	bus.Write8(0x0600, 0xA5) // LDA $10
	bus.Write8(0x0601, 0x10)
	bus.Write8(0x0602, 0x02)
	cpu.PC = 0x0600
	cpu.SetRDYLine(true)
	cpu.SetRunning(true)
	cpu.jitEnabled = true
	cpu.jit6502Execute()
	if got, want := reads, 1; got != want {
		t.Fatalf("mapped reads=%d, want %d", got, want)
	}
	if got, want := cpu.A, byte(0x37); got != want {
		t.Fatalf("A=%02X, want %02X", got, want)
	}
}
