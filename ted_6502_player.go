// ted_6502_player.go - TED 6502 music player for .ted file playback

/*
TED6502Player executes Plus/4 6502 code and captures TED register writes.
It provides sample-accurate timing by tracking CPU cycles and converting
them to audio sample positions.

Usage:
  1. Create player with NewTED6502Player
  2. Load .ted file with LoadFromData
  3. Call RenderFrame() per frame to get TED events
  4. Feed events to TEDEngine for audio synthesis
*/

package main

import "fmt"

// TEDFileMetadata contains parsed metadata from a TED file
type TEDFileMetadata struct {
	Title    string
	Author   string
	Date     string
	Tool     string
	Subtunes int
}

// TED6502Player executes Plus/4 6502 code and captures TED register writes.
type TED6502Player struct {
	bus            *TED6502Bus
	cpu            *CPU_6502
	file           *TEDFile
	clockHz        uint32
	frameRate      int
	sampleRate     int
	cyclesPerFrame uint64
	totalCycles    uint64
	totalSamples   uint64
	engine         *TEDEngine
	initEvents     []TEDEvent
	initEmitted    bool
	continuousMode bool // True if player runs continuously (init==play)
	realTEDMode    bool // True for RealTED mode (PlayAddr==0, full raster emulation)
	currentSubtune int  // Currently selected subtune (0-based)
}

// NewTED6502Player creates a new TED 6502 player
func NewTED6502Player(engine *TEDEngine, sampleRate int) (*TED6502Player, error) {
	player := &TED6502Player{
		engine:     engine,
		sampleRate: sampleRate,
		clockHz:    TED_CLOCK_PAL,
		frameRate:  50,
	}
	player.cyclesPerFrame = uint64(player.clockHz) / uint64(player.frameRate)
	return player, nil
}

// LoadFromData loads a TED file from raw data
func (p *TED6502Player) LoadFromData(data []byte) error {
	file, err := parseTEDFile(data)
	if err != nil {
		return fmt.Errorf("failed to parse TED file: %v", err)
	}

	p.file = file
	p.realTEDMode = file.RealTEDMode
	p.currentSubtune = 0

	// Set clock based on NTSC flag
	if file.NTSC {
		p.clockHz = TED_CLOCK_NTSC
		p.frameRate = 60
	} else {
		p.clockHz = TED_CLOCK_PAL
		p.frameRate = 50
	}
	p.cyclesPerFrame = uint64(p.clockHz) / uint64(p.frameRate)

	// Create bus and load program
	p.bus = newTED6502Bus(file.NTSC)
	p.bus.LoadBinary(file.LoadAddr, file.Data)

	// Create CPU
	p.cpu = p.createCPU()

	// Handle RealTED mode (PlayAddr == 0)
	if p.realTEDMode {
		return p.initRealTEDMode()
	}

	// Determine if this is a packed file that needs unpacking
	// Check the SYS target ($100D for Plus/4 BASIC files):
	// - If SYS target is JMP, the music code is elsewhere - use continuous mode (raster waits)
	// - If SYS target is NOT JMP, it's an unpacker - run to completion and check for IRQ mode
	irqDrivenPlayer := false
	if file.InitAddr != 0 {
		sysTarget := uint16(0x100D) // Default SYS target for Plus/4
		sysOpcode := p.bus.Read(sysTarget)

		if sysOpcode == 0x4C { // JMP - direct to music
			// The JMP target is where music code starts
			// This type of player uses raster WAITS, not IRQ handlers
			p.cpu.PC = file.InitAddr
			p.continuousMode = true
			irqDrivenPlayer = false
		} else {
			// SYS target is an unpacker or IRQ setup - run to completion
			p.cpu.PC = sysTarget // Start from actual SYS target, not JMP-followed address
			if err := p.runInitToCompletion(); err != nil {
				return fmt.Errorf("init failed: %v", err)
			}
			// After init, check what kind of loop we're in:
			// - JMP-to-self: true IRQ wait, needs raster IRQs
			// - CMP $FF1D or DEY/BNE: raster polling, continuous mode but NO IRQs
			irqDrivenPlayer = p.isJMPWaitLoop()
			p.continuousMode = p.isInWaitLoop()
		}
	} else {
		p.continuousMode = (file.InitAddr == file.PlayAddr) && file.InitAddr != 0
	}

	// For IRQ-driven players (those sitting in a JMP-to-self wait loop),
	// enable the KERNAL-style timer. Many TED music players assume the
	// Plus/4 KERNAL has already set up Timer 1 to fire at 50Hz and just
	// hook their handler via $0314.
	if irqDrivenPlayer {
		p.bus.EnableKERNALTimer()
	}

	return nil
}

// runInitToCompletion runs the init routine until it hits a stable wait loop
func (p *TED6502Player) runInitToCompletion() error {
	maxCycles := uint64(2000000) // Allow plenty of time for unpackers
	startCycles := p.cpu.Cycles
	lastPC := uint16(0)
	sameCount := 0

	for p.cpu.Running() && (p.cpu.Cycles-startCycles) < maxCycles {
		// Detect stable wait loop (same PC for 100+ iterations)
		if p.cpu.PC == lastPC {
			sameCount++
			if sameCount > 100 {
				return nil // Init complete, sitting in wait loop
			}
		} else {
			sameCount = 0
			lastPC = p.cpu.PC
		}

		p.cpu.Step()
		p.bus.AddCycles(1)
	}

	return nil // Ran to completion or timeout
}

// isJMPWaitLoop checks if the CPU is in a JMP-to-self wait loop
// This indicates a true IRQ-driven player that needs raster IRQs
func (p *TED6502Player) isJMPWaitLoop() bool {
	pc := p.cpu.PC
	opcode := p.bus.Read(pc)

	// Check for JMP $xxxx where target == pc
	if opcode == 0x4C { // JMP absolute
		target := uint16(p.bus.Read(pc+1)) | (uint16(p.bus.Read(pc+2)) << 8)
		return target == pc
	}
	return false
}

// isInWaitLoop checks if the CPU is in any kind of wait loop pattern
// This includes:
// - JMP $xxxx where target == pc (IRQ-driven wait)
// - CMP $FF1D / BNE pattern (raster polling)
// - DEY / BNE pattern (timing loop in raster polling)
func (p *TED6502Player) isInWaitLoop() bool {
	// JMP-to-self is also a wait loop
	if p.isJMPWaitLoop() {
		return true
	}

	pc := p.cpu.PC
	opcode := p.bus.Read(pc)

	// Check for BNE that branches backwards (tight loop)
	if opcode == 0xD0 { // BNE
		offset := int8(p.bus.Read(pc + 1))
		if offset < 0 { // Backwards branch = loop
			// Branch target is PC+2+offset (offset is relative to byte after instruction)
			branchTarget := uint16(int(pc) + 2 + int(offset))
			targetOp := p.bus.Read(branchTarget)
			if targetOp == 0xCD { // CMP absolute at branch target
				cmpAddr := uint16(p.bus.Read(branchTarget+1)) | (uint16(p.bus.Read(branchTarget+2)) << 8)
				if cmpAddr == 0xFF1D || cmpAddr == 0xFF1C { // Raster registers
					return true
				}
			}
			if targetOp == 0x88 || targetOp == 0xCA { // DEY or DEX (timing loop)
				return true
			}
		}
	}

	// Check if we're at CMP $FF1D (start of raster wait)
	if opcode == 0xCD { // CMP absolute
		target := uint16(p.bus.Read(pc+1)) | (uint16(p.bus.Read(pc+2)) << 8)
		if target == 0xFF1D || target == 0xFF1C {
			return true
		}
	}

	// Check if we're at DEY/DEX with BNE following (timing loop)
	if opcode == 0x88 || opcode == 0xCA { // DEY or DEX
		nextOp := p.bus.Read(pc + 1)
		if nextOp == 0xD0 { // BNE follows
			nextOffset := int8(p.bus.Read(pc + 2))
			if nextOffset < 0 { // Backwards branch = loop
				return true
			}
		}
	}

	return false
}

// LoadFile loads a TED file from disk
func (p *TED6502Player) LoadFile(path string) error {
	data, err := readFileBytes(path)
	if err != nil {
		return err
	}
	return p.LoadFromData(data)
}

// readFileBytes is a helper to read file contents
func readFileBytes(path string) ([]byte, error) {
	import_needed := struct{}{}
	_ = import_needed
	// This will be implemented with os.ReadFile
	return nil, fmt.Errorf("not implemented - use LoadFromData")
}

// GetMetadata returns file metadata
func (p *TED6502Player) GetMetadata() TEDFileMetadata {
	if p.file == nil {
		return TEDFileMetadata{}
	}
	return TEDFileMetadata{
		Title:    p.file.Title,
		Author:   p.file.Author,
		Date:     p.file.Date,
		Tool:     p.file.Tool,
		Subtunes: p.file.Subtunes,
	}
}

// RenderFrame renders one frame of audio and returns TED events
func (p *TED6502Player) RenderFrame() ([]TEDEvent, error) {
	if p.file == nil {
		return nil, fmt.Errorf("no file loaded")
	}

	var events []TEDEvent

	// Emit init events for non-continuous mode
	if !p.initEmitted {
		if !p.continuousMode && len(p.initEvents) > 0 {
			for _, ev := range p.initEvents {
				events = append(events, TEDEvent{
					Cycle:  0,
					Sample: 0,
					Reg:    ev.Reg,
					Value:  ev.Value,
				})
			}
		}
		p.initEmitted = true
	}

	// Start new frame
	p.bus.StartFrame()

	if p.continuousMode {
		// Continuous mode: just run the CPU for one frame's worth of cycles
		// The code is already in its main loop from init
		if err := p.runContinuous(); err != nil {
			return nil, fmt.Errorf("continuous execution failed: %v", err)
		}
	} else if p.file.PlayAddr != 0 {
		// Traditional mode: call play routine each frame
		if err := p.callRoutine(p.file.PlayAddr, 0); err != nil {
			return nil, fmt.Errorf("play routine failed: %v", err)
		}
	}

	// Collect events from this frame
	frameEvents := p.bus.CollectEvents()
	for _, ev := range frameEvents {
		// Calculate cycle delta within this frame
		cycleDelta := ev.Cycle - p.bus.frameCycle
		eventCycle := p.totalCycles + cycleDelta
		sample := p.cyclesToSamples(eventCycle)
		events = append(events, TEDEvent{
			Cycle:  eventCycle,
			Sample: sample,
			Reg:    ev.Reg,
			Value:  ev.Value,
		})
	}

	// Advance time
	p.totalCycles += p.cyclesPerFrame
	p.totalSamples += p.getSamplesPerFrame()

	return events, nil
}

// createCPU creates a 6502 CPU for this player
func (p *TED6502Player) createCPU() *CPU_6502 {
	cpu := &CPU_6502{
		memory:        p.bus,
		SP:            0xFF,
		SR:            UNUSED_FLAG,
		breakpoints:   make(map[uint16]bool),
		breakpointHit: make(chan uint16, 1),
	}
	cpu.rdyLine.Store(true)
	cpu.running.Store(true)
	return cpu
}

// runContinuous runs the CPU for one frame's worth of cycles without resetting PC
// Used for players that have an internal infinite loop
func (p *TED6502Player) runContinuous() error {
	maxCycles := p.cyclesPerFrame
	startCycles := p.cpu.Cycles

	for p.cpu.Running() && (p.cpu.Cycles-startCycles) < maxCycles {
		// Check for pending IRQ from TED timer
		if p.bus.CheckIRQ() {
			p.cpu.irqPending.Store(true)
		}

		cycles := p.cpu.Step()
		p.bus.AddCycles(cycles)
	}

	return nil
}

// callRoutine calls a 6502 subroutine and runs for one frame's worth of cycles
// Many TED players run continuously and don't return, so we run for a fixed time
func (p *TED6502Player) callRoutine(addr uint16, aReg uint8) error {
	p.cpu.A = aReg
	p.cpu.X = 0
	p.cpu.Y = 0
	p.cpu.SR = UNUSED_FLAG
	p.cpu.SetRunning(true)

	// Set up stub: JSR addr; JMP (infinite loop)
	stubAddr := uint16(0xFFF0)
	returnAddr := stubAddr + 3
	p.bus.Write(stubAddr, 0x20) // JSR
	p.bus.Write(stubAddr+1, byte(addr))
	p.bus.Write(stubAddr+2, byte(addr>>8))
	p.bus.Write(returnAddr, 0x4C) // JMP returnAddr
	p.bus.Write(returnAddr+1, byte(returnAddr))
	p.bus.Write(returnAddr+2, byte(returnAddr>>8))

	p.cpu.PC = stubAddr

	// Run for exactly one frame's worth of cycles
	// TED players often loop forever, so we can't wait for return
	maxCycles := p.cyclesPerFrame
	if maxCycles == 0 {
		maxCycles = 100000 // Fallback for init routine
	}
	startCycles := p.cpu.Cycles

	for p.cpu.Running() && (p.cpu.Cycles-startCycles) < maxCycles {
		if p.cpu.PC == returnAddr {
			break
		}

		// Check for pending IRQ from TED timer
		if p.bus.CheckIRQ() {
			p.cpu.irqPending.Store(true)
		}

		cycles := p.cpu.Step()
		p.bus.AddCycles(cycles)
	}

	return nil
}

// cyclesToSamples converts CPU cycles to audio sample position
func (p *TED6502Player) cyclesToSamples(cycles uint64) uint64 {
	return cycles * uint64(p.sampleRate) / uint64(p.clockHz)
}

// getSamplesPerFrame returns the number of audio samples per video frame
func (p *TED6502Player) getSamplesPerFrame() uint64 {
	return uint64(p.sampleRate) / uint64(p.frameRate)
}

// GetClockHz returns the TED clock frequency
func (p *TED6502Player) GetClockHz() uint32 {
	return p.clockHz
}

// GetFrameRate returns the playback frame rate (50 or 60 Hz)
func (p *TED6502Player) GetFrameRate() int {
	return p.frameRate
}

// GetTotalSamples returns the total samples rendered
func (p *TED6502Player) GetTotalSamples() uint64 {
	return p.totalSamples
}

// GetTotalCycles returns the total CPU cycles executed
func (p *TED6502Player) GetTotalCycles() uint64 {
	return p.totalCycles
}

// Reset resets the player to initial state
func (p *TED6502Player) Reset() {
	if p.bus != nil {
		p.bus.Reset()
	}
	p.totalCycles = 0
	p.totalSamples = 0
	p.initEmitted = false

	if p.file != nil {
		p.cpu = p.createCPU()
		p.bus.LoadBinary(p.file.LoadAddr, p.file.Data)

		if p.realTEDMode {
			_ = p.initRealTEDMode()
		} else if p.file.InitAddr != 0 {
			p.bus.StartFrame()
			_ = p.callRoutine(p.file.InitAddr, 0)
			p.initEvents = p.bus.CollectEvents()
		}
	}
}

// initRealTEDMode initializes the player for RealTED mode
// RealTED mode is used when PlayAddr==0, requiring full raster-based emulation
func (p *TED6502Player) initRealTEDMode() error {
	p.continuousMode = true

	// Enable raster timer IRQ for RealTED mode
	// This simulates the Plus/4 raster interrupt system
	p.bus.EnableKERNALTimer()

	// Set the CPU to start execution from InitAddr
	if p.file.InitAddr != 0 {
		p.cpu.PC = p.file.InitAddr
	} else {
		// Fallback to SYS address if InitAddr not set
		p.cpu.PC = findSYSAddress(p.file.Data)
	}

	// In RealTED mode, the init routine doesn't return - it sets up
	// an infinite loop with raster-synchronized timing. We run it
	// continuously without expecting a return.
	p.cpu.A = uint8(p.currentSubtune)
	p.cpu.X = 0
	p.cpu.Y = 0
	p.cpu.SR = UNUSED_FLAG
	p.cpu.SetRunning(true)

	return nil
}

// SelectSubtune selects a subtune for playback
// Subtune numbers are 0-based (0 = first subtune)
func (p *TED6502Player) SelectSubtune(n int) error {
	if p.file == nil {
		return fmt.Errorf("no file loaded")
	}

	if n < 0 || n >= p.file.Subtunes {
		return fmt.Errorf("subtune %d out of range (0-%d)", n, p.file.Subtunes-1)
	}

	p.currentSubtune = n

	// Reset CPU state
	if p.bus != nil {
		p.bus.Reset()
	}
	p.totalCycles = 0
	p.totalSamples = 0
	p.initEmitted = false

	// Reload program data
	p.bus.LoadBinary(p.file.LoadAddr, p.file.Data)
	p.cpu = p.createCPU()

	if p.realTEDMode {
		return p.initRealTEDMode()
	}

	// For standard mode, call init routine with subtune number in A register
	if p.file.InitAddr != 0 {
		p.bus.StartFrame()
		if err := p.callRoutine(p.file.InitAddr, uint8(n)); err != nil {
			return fmt.Errorf("init routine failed: %v", err)
		}
		p.initEvents = p.bus.CollectEvents()
	}

	return nil
}

// GetCurrentSubtune returns the currently selected subtune number (0-based)
func (p *TED6502Player) GetCurrentSubtune() int {
	return p.currentSubtune
}

// GetSubtuneCount returns the number of available subtunes
func (p *TED6502Player) GetSubtuneCount() int {
	if p.file == nil {
		return 0
	}
	return p.file.Subtunes
}

// IsRealTEDMode returns true if the player is in RealTED mode
func (p *TED6502Player) IsRealTEDMode() bool {
	return p.realTEDMode
}

// GetFormatType returns the detected format type of the loaded file
func (p *TED6502Player) GetFormatType() TEDFormat {
	if p.file == nil {
		return TEDFormatRaw
	}
	return p.file.FormatType
}
