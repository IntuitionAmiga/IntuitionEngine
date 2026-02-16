// tracker_z80_render.go - Generic tracker format → Z80 renderer.
//
// Tracker formats (PT3, PT2, PT1, STC, SQT, ASC, FTC) use Z80 player routines
// to sequence their module data. This file provides the common rendering
// infrastructure: load player + module into Z80 RAM, run the Z80 with IRQ-based
// frame timing, and capture AY register writes as PSGEvents.

package main

import (
	"fmt"
	"time"
)

// trackerFormatConfig describes how to load and run a specific tracker format's
// Z80 player routine.
type trackerFormatConfig struct {
	name         string // Format name (e.g., "PT3", "STC")
	playerBinary []byte // Compiled Z80 player routine
	playerBase   uint16 // Load address for player (typically 0xC000)
	moduleBase   uint16 // Load address for module data (typically 0x4000)
	initEntry    uint16 // Entry point for init routine
	playEntry    uint16 // Entry point for play routine (called each frame)
	system       byte   // System type for AY port mapping
	clockHz      uint32 // PSG clock frequency
	z80ClockHz   uint32 // Z80 CPU clock frequency
	frameRate    uint16 // Player frame rate (typically 50 Hz)
}

// renderTrackerZ80 runs a tracker module through its Z80 player routine,
// capturing AY register writes as PSGEvents.
func renderTrackerZ80(config trackerFormatConfig, moduleData []byte, sampleRate int, maxFrames int) (PSGMetadata, []PSGEvent, uint64, error) {
	if config.playerBinary == nil || len(config.playerBinary) == 0 {
		return PSGMetadata{}, nil, 0, fmt.Errorf("tracker: %s player binary not available", config.name)
	}
	if maxFrames <= 0 {
		maxFrames = 15000 // ~5 minutes at 50 Hz
	}

	ram, err := buildTrackerZ80RAM(config, moduleData)
	if err != nil {
		return PSGMetadata{}, nil, 0, fmt.Errorf("tracker: %s RAM setup failed: %w", config.name, err)
	}

	bus := newAyPlaybackBusZ80(&ram, config.system, nil)
	cpu := NewCPU_Z80(bus)

	// Initialize CPU state
	cpu.SP = 0x3FFF
	cpu.I = 3
	cpu.IM = 0
	cpu.IFF1 = false
	cpu.IFF2 = false
	cpu.PC = 0x0000
	cpu.SetIRQVector(0x00)

	// Pre-compute sample multiplier for fast cycle-to-sample conversion
	sampleMultiplier := (uint64(sampleRate) << 32) / uint64(config.z80ClockHz)
	cyclesPerFrame := uint64(config.z80ClockHz) / uint64(config.frameRate)

	events := make([]PSGEvent, 0, maxFrames*32)
	var samplePos uint64
	samplesPerFrameNum := uint64(sampleRate)
	samplesPerFrameDen := uint64(config.frameRate)
	acc := uint64(0)

	var totalInstr uint64
	var totalNanos uint64

	for frame := 0; frame < maxFrames; frame++ {
		startCycle := bus.cycles
		startIndex := len(bus.writes)

		instrCount, nanos := trackerRunIRQFrame(cpu, bus, cyclesPerFrame)
		totalInstr += instrCount
		totalNanos += nanos

		// Collect events from this frame
		for _, write := range bus.writes[startIndex:] {
			cycleDelta := write.Cycle - startCycle
			sampleOffset := (cycleDelta * sampleMultiplier) >> 32
			events = append(events, PSGEvent{
				Sample: samplePos + sampleOffset,
				Reg:    write.Reg,
				Value:  write.Value,
			})
		}

		// Advance sample position using accumulator to prevent drift
		acc += samplesPerFrameNum
		step := acc / samplesPerFrameDen
		samplePos += step
		acc -= step * samplesPerFrameDen
	}

	meta := PSGMetadata{
		System: trackerSystemName(config.system),
	}

	return meta, events, samplePos, nil
}

// buildTrackerZ80RAM initializes 64K Z80 RAM with player routine and module data.
//
// Memory layout:
//
//	0x0000-0x00FF: Bootstrap stub (DI / CALL init / loop: IM1 / EI / HALT / CALL play / JR loop)
//	0x0100-0x3FFF: Scratch / stack area (SP starts at 0x3FFF)
//	moduleBase:    Module data
//	playerBase:    Player routine
func buildTrackerZ80RAM(config trackerFormatConfig, moduleData []byte) ([0x10000]byte, error) {
	var ram [0x10000]byte

	// Fill low page with RET for safe default interrupt handling
	for i := range 0x0100 {
		ram[i] = 0xC9 // RET
	}
	// Fill scratch area
	for i := 0x0100; i < 0x4000; i++ {
		ram[i] = 0xFF
	}

	// Install IM1 interrupt handler at 0x0038 (RST 56)
	ram[0x0038] = 0xFB // EI
	ram[0x0039] = 0xC9 // RET

	// Copy module data
	if len(moduleData) > 0 {
		modEnd := min(int(config.moduleBase)+len(moduleData), 0x10000)
		copy(ram[config.moduleBase:modEnd], moduleData)
	}

	// Copy player binary
	if len(config.playerBinary) > 0 {
		playerEnd := min(int(config.playerBase)+len(config.playerBinary), 0x10000)
		copy(ram[config.playerBase:playerEnd], config.playerBinary)
	}

	// Build and install bootstrap stub
	stub := buildTrackerStub(config.initEntry, config.playEntry, config.moduleBase)
	copy(ram[:], stub)

	return ram, nil
}

// buildTrackerStub generates the Z80 bootstrap code.
// The stub initializes the player then loops: wait for IRQ, call play routine.
//
// Most tracker players expect HL to point to the module data on init.
func buildTrackerStub(initAddr, playAddr, moduleBase uint16) []byte {
	code := make([]byte, 0, 32)
	code = append(code, 0xF3) // DI

	// Load HL with module base address (for player init)
	code = append(code, 0x21, byte(moduleBase&0xFF), byte(moduleBase>>8)) // LD HL, moduleBase

	// Call init
	if initAddr != 0 {
		code = appendCall(code, initAddr) // CALL initAddr
	}

	// Main loop
	loopPos := len(code)
	code = append(code, 0xED, 0x56) // IM 1
	code = append(code, 0xFB, 0x76) // EI, HALT

	// Call play routine
	if playAddr != 0 {
		code = appendCall(code, playAddr) // CALL playAddr
	}

	// Jump back to loop
	rel := loopPos - (len(code) + 2)
	code = append(code, 0x18, byte(int8(rel))) // JR loop

	return code
}

// trackerRunIRQFrame executes one frame of Z80 code using IRQ-driven timing.
func trackerRunIRQFrame(cpu *CPU_Z80, bus *ayPlaybackBusZ80, budget uint64) (uint64, uint64) {
	start := time.Now()
	idlePC := cpu.PC
	startCycles := bus.cycles
	irqAsserted := false
	irqServiced := false
	executed := false
	var instrCount uint64

	for bus.cycles-startCycles < budget {
		if cpu.Halted && !irqAsserted {
			cpu.SetIRQLine(true)
			irqAsserted = true
		}

		prevIFF1 := cpu.IFF1
		cpu.Step()
		instrCount++
		executed = true

		if irqAsserted && prevIFF1 && !cpu.IFF1 && !irqServiced {
			irqServiced = true
			cpu.SetIRQLine(false)
		}
		if executed && cpu.PC == idlePC && irqServiced {
			break
		}
	}

	if irqAsserted && !irqServiced {
		cpu.SetIRQLine(false)
	}

	return instrCount, uint64(time.Since(start).Nanoseconds())
}

func trackerSystemName(system byte) string {
	switch system {
	case ayZXSystemCPC:
		return "Amstrad CPC"
	case ayZXSystemMSX:
		return "MSX"
	default:
		return "ZX Spectrum"
	}
}
