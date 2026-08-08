//go:build js && wasm

package main

import "testing"

func TestP65WasmJIT_MMIOPollParksBetweenVBlankReads(t *testing.T) {
	bus := NewMachineBus()
	values := []uint32{0, 0, videoStatusVBlank}
	bus.MapIO(VIDEO_STATUS, VIDEO_STATUS, func(uint32) uint32 {
		value := values[0]
		values = values[1:]
		return value
	}, nil)
	cpu := NewCPU_6502(bus)
	adapter, ok := cpu.memory.(*Bus6502Adapter)
	if !ok {
		t.Fatal("6502 CPU did not install Bus6502Adapter")
	}
	copy(cpu.fastAdapter.memDirect[0x0600:], []byte{
		0xAD, 0x08, 0xF0, // LDA $F008
		0x29, byte(videoStatusVBlank), // AND #$02
		0xF0, 0xF9, // BEQ $0600
	})
	cpu.PC = 0x0600
	cpu.Cycles = 0
	cpu.SetRunning(true)

	previousPark := wasm8BitPollPark
	parks := 0
	wasm8BitPollPark = func() { parks++ }
	t.Cleanup(func() { wasm8BitPollPark = previousPark })

	matched, retired := cpu.wasmRun6502MMIOPollLoop(adapter)
	if !matched {
		t.Fatal("wasm 6502 VBlank poll did not match")
	}
	if got, want := retired, uint32(9); got != want {
		t.Fatalf("retired=%d, want %d", got, want)
	}
	if got, want := parks, 2; got != want {
		t.Fatalf("parks=%d, want %d", got, want)
	}
	if got, want := cpu.PC, uint16(0x0607); got != want {
		t.Fatalf("PC=%04X, want %04X", got, want)
	}
	if got, want := cpu.A, byte(videoStatusVBlank); got != want {
		t.Fatalf("A=%02X, want %02X", got, want)
	}
	if got, want := cpu.Cycles, uint64(26); got != want {
		t.Fatalf("cycles=%d, want %d", got, want)
	}
}

func TestP65WasmJIT_DispatchUsesMMIOPollService(t *testing.T) {
	if !p65WasmJITEnabled() {
		t.Skip("P65 wasm JIT runtime unavailable")
	}
	bus := NewMachineBus()
	values := []uint32{0, 0, videoStatusVBlank}
	bus.MapIO(VIDEO_STATUS, VIDEO_STATUS, func(uint32) uint32 {
		value := values[0]
		values = values[1:]
		return value
	}, nil)
	cpu := NewCPU_6502(bus)
	copy(cpu.fastAdapter.memDirect[0x0600:], []byte{
		0xAD, 0x08, 0xF0, // LDA $F008
		0x29, byte(videoStatusVBlank), // AND #$02
		0xF0, 0xF9, // BEQ $0600
		0x02, // JAM
	})
	cpu.PC = 0x0600
	cpu.SetRDYLine(true)
	cpu.SetRunning(true)
	cpu.jitEnabled = true

	previousPark := wasm8BitPollPark
	parks := 0
	wasm8BitPollPark = func() { parks++ }
	t.Cleanup(func() { wasm8BitPollPark = previousPark })

	cpu.jit6502Execute()
	if got, want := parks, 2; got != want {
		t.Fatalf("parks=%d, want %d", got, want)
	}
	if cpu.Running() {
		t.Fatal("JAM after VBlank poll did not stop the 6502")
	}
}

func TestZ80WasmMMIOPollParksBetweenVBlankReads(t *testing.T) {
	bus := NewMachineBus()
	values := []uint32{0, 0, videoStatusVBlank}
	bus.MapIO(VIDEO_STATUS, VIDEO_STATUS, func(uint32) uint32 {
		value := values[0]
		values = values[1:]
		return value
	}, nil)
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	copy(bus.memory[0x0600:], []byte{
		0x3A, 0x08, 0xF0, // LD A,(F008)
		0xE6, byte(videoStatusVBlank), // AND $02
		0x28, 0xF9, // JR Z,$0600
	})
	cpu.PC = 0x0600
	cpu.F = z80FlagC
	cpu.Cycles = 0
	cpu.SetRunning(true)

	previousPark := wasm8BitPollPark
	parks := 0
	wasm8BitPollPark = func() { parks++ }
	t.Cleanup(func() { wasm8BitPollPark = previousPark })

	matched, retired, rIncrements := cpu.wasmRunZ80MMIOPollLoop(adapter)
	if !matched {
		t.Fatal("wasm Z80 VBlank poll did not match")
	}
	if got, want := retired, uint32(9); got != want {
		t.Fatalf("retired=%d, want %d", got, want)
	}
	if got, want := rIncrements, uint32(3); got != want {
		t.Fatalf("R increments=%d, want %d", got, want)
	}
	if got, want := parks, 2; got != want {
		t.Fatalf("parks=%d, want %d", got, want)
	}
	if got, want := cpu.PC, uint16(0x0607); got != want {
		t.Fatalf("PC=%04X, want %04X", got, want)
	}
	if got, want := cpu.A, byte(videoStatusVBlank); got != want {
		t.Fatalf("A=%02X, want %02X", got, want)
	}
	if got, want := cpu.F, byte(z80FlagH); got != want {
		t.Fatalf("F=%02X, want %02X after AND", got, want)
	}
	if got, want := cpu.Cycles, uint64(91); got != want {
		t.Fatalf("cycles=%d, want %d", got, want)
	}
}

func TestZ80WasmDispatchUsesMMIOPollService(t *testing.T) {
	if !z80WasmJITEnabled() {
		t.Skip("Z80 wasm JIT runtime unavailable")
	}
	bus := NewMachineBus()
	values := []uint32{0, 0, videoStatusVBlank}
	bus.MapIO(VIDEO_STATUS, VIDEO_STATUS, func(uint32) uint32 {
		value := values[0]
		values = values[1:]
		return value
	}, nil)
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	copy(bus.memory[0x0600:], []byte{
		0x3A, 0x08, 0xF0, // LD A,(F008)
		0xE6, byte(videoStatusVBlank), // AND $02
		0x28, 0xF9, // JR Z,$0600
		0x76, // HALT
	})
	cpu.PC = 0x0600
	cpu.SetRunning(true)
	cpu.jitEnabled = true
	cpu.executionBoundary = func() {
		if cpu.PC == 0x0607 {
			cpu.SetRunning(false)
		}
	}

	previousPark := wasm8BitPollPark
	parks := 0
	wasm8BitPollPark = func() { parks++ }
	t.Cleanup(func() { wasm8BitPollPark = previousPark })

	cpu.z80JitExecute()
	if got, want := parks, 2; got != want {
		t.Fatalf("parks=%d, want %d", got, want)
	}
	if got, want := cpu.R&0x7F, byte(3); got != want {
		t.Fatalf("R=%02X, want %02X", got, want)
	}
}
