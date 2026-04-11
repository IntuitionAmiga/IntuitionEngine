// cpu_six5go2_fast_test.go - regression tests for ExecuteFast()
//
// These tests exercise the 6502 fast interpreter against scenarios that the
// legacy interpreter path already handled correctly, so they act as
// guard-rails against future performance work that might accidentally drop
// observable behavior.

package main

import (
	"testing"
)

// TestExecuteFast_ZeroPageRMWSpuriousWrite verifies that every read-modify-
// write instruction targeting a zero-page address issues the 6502 "dummy"
// write of the original value BEFORE writing the modified value. This is
// required for parity with the legacy rmw() helper and is observable to
// MMIO devices mapped into page 0.
//
// The test maps a byte-level I/O region at $0050 with read and write
// callbacks, loads a single RMW instruction at $0600, runs through
// cpu.Execute() (which routes to ExecuteFast), and asserts that exactly
// two writes were observed at the target address — the first equal to the
// original value (spurious) and the second equal to the modified value.
func TestExecuteFast_ZeroPageRMWSpuriousWrite(t *testing.T) {
	// Initial carry state used for ROL/ROR so that the expected result is
	// deterministic (C = 0 → rotate in 0).
	const initial byte = 0x42

	cases := []struct {
		name     string
		opcode   byte
		expected byte // result byte after applying the operation to `initial`
	}{
		{"INC zp", 0xE6, 0x43},
		{"DEC zp", 0xC6, 0x41},
		{"ASL zp", 0x06, 0x84},
		{"LSR zp", 0x46, 0x21},
		{"ROL zp", 0x26, 0x84},
		{"ROR zp", 0x66, 0x21},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()

			const mmioAddr uint32 = 0x0050
			var writes []byte
			readValue := initial
			bus.MapIO(mmioAddr, mmioAddr,
				func(addr uint32) uint32 {
					return uint32(readValue)
				},
				func(addr uint32, val uint32) {
					writes = append(writes, byte(val))
				},
			)

			cpu := NewCPU_6502(bus)
			cpu.SetRDYLine(true)

			// Program at $0600: <opcode> $50 ; halt
			bus.Write8(0x0600, tc.opcode)
			bus.Write8(0x0601, 0x50)
			bus.Write8(0x0602, 0x02) // JAM — halts ExecuteFast's loop

			// Clear carry so ROL/ROR rotate in a known 0 bit.
			cpu.SR = UNUSED_FLAG
			cpu.PC = 0x0600
			cpu.SetRunning(true)
			cpu.Execute()

			if len(writes) != 2 {
				t.Fatalf("%s: expected 2 writes (spurious + modified) at $%04X, got %d: %v",
					tc.name, mmioAddr, len(writes), writes)
			}
			if writes[0] != initial {
				t.Errorf("%s: first (spurious) write = 0x%02X, want 0x%02X (original value)",
					tc.name, writes[0], initial)
			}
			if writes[1] != tc.expected {
				t.Errorf("%s: second (modified) write = 0x%02X, want 0x%02X",
					tc.name, writes[1], tc.expected)
			}
		})
	}
}

// TestExecuteFast_ZeroPageIndexedRMWSpuriousWrite covers the zp,X variants
// of the same read-modify-write discipline.
func TestExecuteFast_ZeroPageIndexedRMWSpuriousWrite(t *testing.T) {
	const initial byte = 0x42

	cases := []struct {
		name     string
		opcode   byte
		expected byte
	}{
		{"INC zp,X", 0xF6, 0x43},
		{"DEC zp,X", 0xD6, 0x41},
		{"ASL zp,X", 0x16, 0x84},
		{"LSR zp,X", 0x56, 0x21},
		{"ROL zp,X", 0x36, 0x84},
		{"ROR zp,X", 0x76, 0x21},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()

			const targetAddr uint32 = 0x0060
			var writes []byte
			readValue := initial
			bus.MapIO(targetAddr, targetAddr,
				func(addr uint32) uint32 { return uint32(readValue) },
				func(addr uint32, val uint32) { writes = append(writes, byte(val)) },
			)

			cpu := NewCPU_6502(bus)
			cpu.SetRDYLine(true)

			// Program: LDX #$10 ; <opcode> $50 ; JAM.
			// $50 + X(=0x10) → $60.
			bus.Write8(0x0600, 0xA2)
			bus.Write8(0x0601, 0x10)
			bus.Write8(0x0602, tc.opcode)
			bus.Write8(0x0603, 0x50)
			bus.Write8(0x0604, 0x02)

			cpu.SR = UNUSED_FLAG
			cpu.PC = 0x0600
			cpu.SetRunning(true)
			cpu.Execute()

			if len(writes) != 2 {
				t.Fatalf("%s: expected 2 writes at $%04X, got %d: %v",
					tc.name, targetAddr, len(writes), writes)
			}
			if writes[0] != initial {
				t.Errorf("%s: first write = 0x%02X, want 0x%02X", tc.name, writes[0], initial)
			}
			if writes[1] != tc.expected {
				t.Errorf("%s: second write = 0x%02X, want 0x%02X", tc.name, writes[1], tc.expected)
			}
		})
	}
}

// TestExecuteFast_AbsoluteRMWSpuriousWrite proves the spurious-write
// discipline also holds for absolute-mode RMW instructions when the target
// page is MMIO-backed.
func TestExecuteFast_AbsoluteRMWSpuriousWrite(t *testing.T) {
	const initial byte = 0x42

	cases := []struct {
		name     string
		opcode   byte
		expected byte
	}{
		{"INC abs", 0xEE, 0x43},
		{"DEC abs", 0xCE, 0x41},
		{"ASL abs", 0x0E, 0x84},
		{"LSR abs", 0x4E, 0x21},
		{"ROL abs", 0x2E, 0x84},
		{"ROR abs", 0x6E, 0x21},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()

			// Use a page that won't collide with page 0/1 or the standard
			// 6502 adapter I/O handlers (VGA/PSG/SID/TED/ULA live in page
			// ranges $D2-$D8). Page $30 is free.
			const targetAddr uint32 = 0x3000
			var writes []byte
			readValue := initial
			bus.MapIO(targetAddr, targetAddr,
				func(addr uint32) uint32 { return uint32(readValue) },
				func(addr uint32, val uint32) { writes = append(writes, byte(val)) },
			)

			cpu := NewCPU_6502(bus)
			cpu.SetRDYLine(true)

			// Program: <opcode> $3000 ; JAM
			bus.Write8(0x0600, tc.opcode)
			bus.Write8(0x0601, 0x00)
			bus.Write8(0x0602, 0x30)
			bus.Write8(0x0603, 0x02)

			cpu.SR = UNUSED_FLAG
			cpu.PC = 0x0600
			cpu.SetRunning(true)
			cpu.Execute()

			if len(writes) != 2 {
				t.Fatalf("%s: expected 2 writes at $%04X, got %d: %v",
					tc.name, targetAddr, len(writes), writes)
			}
			if writes[0] != initial {
				t.Errorf("%s: first write = 0x%02X, want 0x%02X", tc.name, writes[0], initial)
			}
			if writes[1] != tc.expected {
				t.Errorf("%s: second write = 0x%02X, want 0x%02X", tc.name, writes[1], tc.expected)
			}
		})
	}
}
