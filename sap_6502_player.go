// sap_6502_player.go - 6502 CPU player for SAP file playback
//
// This player executes 6502 code from SAP files to capture POKEY register
// writes with cycle-accurate timing. It supports TYPE B SAP files where:
// - INIT routine is called once with subsong in A register
// - PLAYER routine is called once per frame at the FASTPLAY rate

package main

import (
	"fmt"
)

// SAP6502Player executes SAP 6502 code and captures POKEY events
type SAP6502Player struct {
	bus     *SAP6502Bus
	cpu     *CPU_6502
	file    *SAPFile
	subsong int

	// Timing configuration
	clockHz           uint32
	sampleRate        int
	scanlinesPerFrame int
	cyclesPerFrame    int

	// Playback state
	totalCycles      uint64
	totalSamples     uint64
	stereo           bool
	sampleMultiplier uint64          // Pre-computed: (sampleRate << 32) / clockHz for fast conversion
	eventBuffer      []SAPPOKEYEvent // Pre-allocated buffer for frame events (zero-allocation path)
}

// maxSAPEventsPerFrame is the initial capacity for the event buffer.
const maxSAPEventsPerFrame = 512

// sapBusAdapter adapts SAP6502Bus to MemoryBus_6502 interface
type sapBusAdapter struct {
	bus *SAP6502Bus
}

func (a *sapBusAdapter) Read(addr uint16) byte {
	return a.bus.Read(addr)
}

func (a *sapBusAdapter) Write(addr uint16, value byte) {
	a.bus.Write(addr, value)
}

// newSAP6502Player creates a new SAP player for the given file and subsong
func newSAP6502Player(file *SAPFile, subsong, sampleRate int) (*SAP6502Player, error) {
	// Validate file type
	if file.Header.Type != 'B' && file.Header.Type != 'C' {
		return nil, fmt.Errorf("unsupported SAP TYPE %c (only B and C supported)", file.Header.Type)
	}

	// Create bus
	bus := newSAP6502Bus(file.Header.Stereo, file.Header.NTSC)

	// Load binary blocks into memory
	bus.LoadBlocks(file.Blocks)

	// Determine clock frequency
	var clockHz uint32
	if file.Header.NTSC {
		clockHz = POKEY_CLOCK_NTSC
	} else {
		clockHz = POKEY_CLOCK_PAL
	}

	// Calculate cycles per frame
	cyclesPerFrame := file.Header.FastPlay * atariCyclesPerScanline

	// Pre-compute sample multiplier for fast cycle-to-sample conversion
	// Using 32.32 fixed-point: (sampleRate << 32) / clockHz
	sampleMultiplier := (uint64(sampleRate) << 32) / uint64(clockHz)

	player := &SAP6502Player{
		bus:               bus,
		file:              file,
		subsong:           subsong,
		clockHz:           clockHz,
		sampleRate:        sampleRate,
		scanlinesPerFrame: file.Header.FastPlay,
		cyclesPerFrame:    cyclesPerFrame,
		stereo:            file.Header.Stereo,
		sampleMultiplier:  sampleMultiplier,
		eventBuffer:       make([]SAPPOKEYEvent, 0, maxSAPEventsPerFrame),
	}

	// Create CPU with bus adapter
	player.cpu = player.createCPU()

	// Run INIT routine
	if err := player.runInit(); err != nil {
		return nil, fmt.Errorf("INIT routine failed: %v", err)
	}

	return player, nil
}

// createCPU creates a 6502 CPU with our custom bus
func (p *SAP6502Player) createCPU() *CPU_6502 {
	// Create CPU with direct memory access
	cpu := &CPU_6502{
		memory:        &sapBusAdapter{p.bus},
		SP:            0xFF,
		SR:            UNUSED_FLAG,
		breakpoints:   make(map[uint16]bool),
		breakpointHit: make(chan uint16, 1),
	}
	cpu.rdyLine.Store(true) // RDY line must be high for CPU to run
	cpu.running.Store(true)
	return cpu
}

// runInit executes the INIT routine (TYPE B: JSR to INIT address with subsong in A)
func (p *SAP6502Player) runInit() error {
	if p.file.Header.Type == 'B' {
		// TYPE B: Call INIT with subsong number in A register
		return p.callRoutine(p.file.Header.Init, uint8(p.subsong))
	} else if p.file.Header.Type == 'C' {
		// TYPE C: Different initialization sequence
		// For now, just call PLAYER with the music address
		return nil
	}
	return nil
}

// callRoutine calls a subroutine at the given address with A register set
func (p *SAP6502Player) callRoutine(addr uint16, aReg uint8) error {
	// Set up CPU state
	p.cpu.A = aReg
	p.cpu.X = 0
	p.cpu.Y = 0
	p.cpu.SR = UNUSED_FLAG
	p.cpu.SetRunning(true)

	// Create a JSR/RTS stub at a safe location
	// We'll use $FFF0-$FFF5 as our stub area
	// The stub is: JSR addr; JMP $FFF3 (infinite loop as return sentinel)
	stubAddr := uint16(0xFFF0)
	returnAddr := stubAddr + 3
	p.bus.Write(stubAddr, 0x20)            // JSR opcode
	p.bus.Write(stubAddr+1, byte(addr))    // Low byte of target
	p.bus.Write(stubAddr+2, byte(addr>>8)) // High byte of target
	// Put an infinite loop at the return point: JMP $FFF3
	p.bus.Write(returnAddr, 0x4C)                  // JMP opcode
	p.bus.Write(returnAddr+1, byte(returnAddr))    // Low byte
	p.bus.Write(returnAddr+2, byte(returnAddr>>8)) // High byte

	// Start execution at stub
	p.cpu.PC = stubAddr

	// Execute until we return to the sentinel loop or max cycles
	maxCycles := uint64(1000000) // Safety limit
	startCycles := p.cpu.Cycles

	for p.cpu.Running() && (p.cpu.Cycles-startCycles) < maxCycles {
		// Check if we've returned to the sentinel loop BEFORE executing
		if p.cpu.PC == returnAddr {
			break
		}

		// Execute one instruction
		p.executeInstruction()
	}

	// Sync bus cycles with CPU cycles
	p.bus.cycles = p.cpu.Cycles

	return nil
}

// executeInstruction executes a single CPU instruction using the proven 6502 emulator
func (p *SAP6502Player) executeInstruction() {
	// Use the proven CPU_6502.Step() method that passes Klaus's tests
	cycles := p.cpu.Step()
	p.bus.AddCycles(cycles)
}

// RenderFrames renders N frames of audio and returns POKEY events
func (p *SAP6502Player) RenderFrames(numFrames int) ([]SAPPOKEYEvent, uint64) {
	// Reuse pre-allocated buffer, reset length but keep capacity
	p.eventBuffer = p.eventBuffer[:0]

	for frame := 0; frame < numFrames; frame++ {
		// Start new frame for event collection
		p.bus.StartFrame()

		// Call PLAYER routine
		p.callRoutine(p.file.Header.Player, 0)

		// Zero-allocation path: read events directly without copy
		frameEvents := p.bus.GetEvents()
		frameCycle := p.bus.GetFrameCycleStart()

		// Convert cycle timestamps to sample timestamps
		for i := range frameEvents {
			// Calculate sample position within the song
			eventCycle := p.totalCycles + frameEvents[i].Cycle - frameCycle

			// Convert to samples
			sample := p.cyclesToSamples(eventCycle)
			p.eventBuffer = append(p.eventBuffer, SAPPOKEYEvent{
				Cycle:  eventCycle,
				Sample: sample,
				Reg:    frameEvents[i].Reg,
				Value:  frameEvents[i].Value,
				Chip:   frameEvents[i].Chip,
			})
		}
		p.bus.ClearEvents()

		// Advance total counters
		p.totalCycles += uint64(p.cyclesPerFrame)
		p.totalSamples += uint64(p.getSamplesPerFrame())
	}

	return p.eventBuffer, p.totalSamples
}

// cyclesToSamples converts CPU cycles to audio samples
func (p *SAP6502Player) cyclesToSamples(cycles uint64) uint64 {
	// Fast conversion using pre-computed 32.32 fixed-point multiplier
	// Equivalent to: cycles * sampleRate / clockHz
	// But uses shift instead of division (15x faster)
	return (cycles * p.sampleMultiplier) >> 32
}

// getSamplesPerFrame returns the number of audio samples per frame
func (p *SAP6502Player) getSamplesPerFrame() int {
	// Frame rate = clockHz / (scanlinesPerFrame * cyclesPerScanline)
	// Samples per frame = sampleRate / frameRate
	// = sampleRate * scanlinesPerFrame * cyclesPerScanline / clockHz
	return p.sampleRate * p.cyclesPerFrame / int(p.clockHz)
}

// GetClockHz returns the CPU/POKEY clock frequency
func (p *SAP6502Player) GetClockHz() uint32 {
	return p.clockHz
}

// IsStereo returns true if the SAP file uses stereo POKEY
func (p *SAP6502Player) IsStereo() bool {
	return p.stereo
}

// GetTotalSamples returns the total samples rendered so far
func (p *SAP6502Player) GetTotalSamples() uint64 {
	return p.totalSamples
}

// GetTotalCycles returns the total CPU cycles executed
func (p *SAP6502Player) GetTotalCycles() uint64 {
	return p.totalCycles
}

// Reset resets the player to initial state
func (p *SAP6502Player) Reset() {
	p.bus.Reset()
	p.totalCycles = 0
	p.totalSamples = 0
	p.cpu = p.createCPU()
	p.runInit()
}

// executeOpcode executes a single 6502 opcode
// This is a simplified version focused on the instructions commonly used in SAP players
