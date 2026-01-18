// sndh_68k_player.go - SNDH playback runner using 68K CPU emulation.
//
// This module handles frame-based execution of SNDH music files:
// 1. Calls INIT routine once with subsong number in D0
// 2. Calls PLAY routine at the configured frame rate (usually 50Hz)
// 3. Captures YM2149 writes as PSGEvents with cycle-accurate timing

package main

import (
	"fmt"
)

const (
	// Atari ST 68000 clock speed (8 MHz)
	M68K_CLOCK_ATARI_ST = 8000000

	// Sentinel address pushed to stack to detect routine return
	SNDH_RETURN_SENTINEL = 0x00000000

	// Maximum instructions per frame to prevent infinite loops
	SNDH_MAX_INSTRUCTIONS_PER_FRAME = 1000000

	// Stack address for SNDH playback
	SNDH_STACK_ADDR = 0x3FF00
)

// sndh68KPlayer handles SNDH playback using 68K CPU emulation
type sndh68KPlayer struct {
	cpu               *M68KCPU
	bus               *sndh68KBus
	file              *SNDHFile
	clockHz           uint32
	frameRate         uint16
	sampleRate        int
	cyclesPerFrame    uint64
	currentSample     uint64
	frameAcc          uint64
	initDone          bool
	timerVector       uint32 // Timer interrupt vector (if timer-based replay)
	timerCallsPerPlay int    // Number of timer calls per PLAY frame

	// SID emulation timer handling - these are called independently
	timerBVector uint32 // Timer B interrupt handler
	timerDVector uint32 // Timer D interrupt handler
}

// newSNDH68KPlayer creates a new SNDH player
func newSNDH68KPlayer(file *SNDHFile, subsong int, sampleRate int) (*sndh68KPlayer, error) {
	if file == nil {
		return nil, fmt.Errorf("SNDH file is nil")
	}
	if subsong < 1 || subsong > file.Header.SubSongCount {
		subsong = file.Header.DefaultSong
	}
	if sampleRate <= 0 {
		return nil, fmt.Errorf("invalid sample rate: %d", sampleRate)
	}

	// Create bus and load SNDH data
	bus := newSNDH68KBus()
	bus.LoadSNDH(file.Data)

	// Create CPU with this bus
	cpu := NewM68KCPU(bus)

	// Determine frame rate from file header
	frameRate := uint16(file.Header.TimerFreq)
	if frameRate == 0 {
		frameRate = 50 // Default to PAL VBL
	}

	clockHz := uint32(M68K_CLOCK_ATARI_ST)
	cyclesPerFrame := uint64(clockHz) / uint64(frameRate)

	player := &sndh68KPlayer{
		cpu:            cpu,
		bus:            bus,
		file:           file,
		clockHz:        clockHz,
		frameRate:      frameRate,
		sampleRate:     sampleRate,
		cyclesPerFrame: cyclesPerFrame,
	}

	// Initialize the subsong
	if err := player.callInit(subsong); err != nil {
		return nil, fmt.Errorf("INIT failed: %w", err)
	}

	return player, nil
}

// Timer vector addresses in 68000 exception table
const (
	TIMER_A_VECTOR_ADDR = 0x134 // MFP Timer A vector
	TIMER_B_VECTOR_ADDR = 0x120 // MFP Timer B vector
	TIMER_C_VECTOR_ADDR = 0x114 // MFP Timer C vector
	TIMER_D_VECTOR_ADDR = 0x110 // MFP Timer D vector
)

// callInit calls the SNDH INIT routine with subsong number in D0
func (p *sndh68KPlayer) callInit(subsong int) error {
	// Check if this tune explicitly uses timer-based replay
	// Timer types: "A", "B", "C", "D" = use timers, "V" or "" = VBL only
	useTimers := p.file.Header.TimerType == "A" ||
		p.file.Header.TimerType == "B" ||
		p.file.Header.TimerType == "C" ||
		p.file.Header.TimerType == "D"

	// Always save Timer B and D vectors - SID emulation tunes use these
	// for ADSR envelope processing even when declared as VBL-based
	timerBBefore := p.readVector(TIMER_B_VECTOR_ADDR)
	timerDBefore := p.readVector(TIMER_D_VECTOR_ADDR)

	// Save timer A/C vectors only if explicitly timer-based
	var timerABefore, timerCBefore uint32
	if useTimers {
		timerABefore = p.readVector(TIMER_A_VECTOR_ADDR)
		timerCBefore = p.readVector(TIMER_C_VECTOR_ADDR)
	}

	// Set up stack
	p.cpu.AddrRegs[7] = SNDH_STACK_ADDR

	// Push return sentinel to stack
	p.cpu.AddrRegs[7] -= 4
	p.bus.Write32(p.cpu.AddrRegs[7], SNDH_RETURN_SENTINEL)

	// Set D0 to subsong number (1-based)
	p.cpu.DataRegs[0] = uint32(subsong)

	// Set PC to INIT address
	p.cpu.PC = uint32(p.file.InitOffset)

	// Run until return
	if err := p.runUntilReturn(); err != nil {
		return err
	}

	// Check Timer B and D vectors - these are used by SID emulation
	timerBAfter := p.readVector(TIMER_B_VECTOR_ADDR)
	timerDAfter := p.readVector(TIMER_D_VECTOR_ADDR)

	if timerBAfter != timerBBefore && isValidVector(timerBAfter) {
		p.timerBVector = timerBAfter
	}
	if timerDAfter != timerDBefore && isValidVector(timerDAfter) {
		p.timerDVector = timerDAfter
	}

	// Check for explicit timer-based replay
	if useTimers {
		timerAAfter := p.readVector(TIMER_A_VECTOR_ADDR)
		timerCAfter := p.readVector(TIMER_C_VECTOR_ADDR)

		// Detect which timer was installed for explicit timer-based replay
		if timerAAfter != timerABefore && isValidVector(timerAAfter) {
			p.timerVector = timerAAfter
			p.timerCallsPerPlay = 4 // Timer A typically runs at ~200Hz vs 50Hz VBL
		} else if timerBAfter != timerBBefore && isValidVector(timerBAfter) {
			p.timerVector = timerBAfter
			p.timerCallsPerPlay = 4
		} else if timerCAfter != timerCBefore && isValidVector(timerCAfter) {
			p.timerVector = timerCAfter
			p.timerCallsPerPlay = 4
		}
	}

	p.initDone = true
	return nil
}

// isValidVector checks if a vector value looks like a valid handler address
func isValidVector(vec uint32) bool {
	// Must be non-zero and within memory
	if vec == 0 || vec >= SNDH_BUS_SIZE {
		return false
	}
	// High bytes should be zero for Atari ST addresses (24-bit address space)
	if vec&0xFF000000 != 0 {
		return false
	}
	// Should be even (68K requires word-aligned addresses)
	if vec&1 != 0 {
		return false
	}
	return true
}

// readVector reads a 32-bit exception vector from memory
func (p *sndh68KPlayer) readVector(addr uint32) uint32 {
	return uint32(p.bus.memory[addr])<<24 |
		uint32(p.bus.memory[addr+1])<<16 |
		uint32(p.bus.memory[addr+2])<<8 |
		uint32(p.bus.memory[addr+3])
}

// MFP prescaler values (index 0-7 maps to divider)
var mfpPrescalers = [8]int{0, 4, 10, 16, 50, 64, 100, 200}

// calcTimerInterruptsPerFrame calculates how many times a timer fires per VBL frame
func calcTimerInterruptsPerFrame(ctrl, data uint8, frameRate uint16) int {
	if ctrl == 0 || data == 0 {
		return 0
	}
	prescaler := mfpPrescalers[ctrl&0x07]
	if prescaler == 0 {
		return 0
	}
	// Timer frequency = MFP_CLOCK / prescaler / data
	// Interrupts per frame = timer_freq / frameRate
	timerFreq := MFP_CLOCK / prescaler / int(data)
	return timerFreq / int(frameRate)
}

// RenderFrames renders the specified number of frames and returns PSGEvents
func (p *sndh68KPlayer) RenderFrames(frameCount int) ([]PSGEvent, uint64) {
	if frameCount <= 0 {
		return nil, p.currentSample
	}

	events := make([]PSGEvent, 0, frameCount*32)

	samplesPerFrameNum := uint64(p.sampleRate)
	samplesPerFrameDen := uint64(p.frameRate)
	acc := p.frameAcc
	samplePos := p.currentSample

	for frame := 0; frame < frameCount; frame++ {
		// Clear writes from previous frame
		p.bus.ResetWrites()
		startCycle := p.bus.GetCycles()

		// Call PLAY routine (advances frame counters)
		if err := p.callPlay(); err != nil {
			// Stop on error
			break
		}

		// For timer-based replay, also call the timer interrupt handler
		if p.timerVector != 0 {
			for t := 0; t < p.timerCallsPerPlay; t++ {
				if err := p.callTimer(); err != nil {
					break
				}
			}
		}

		// Call SID emulation timer handlers (Timer B and D)
		// These handle ADSR envelope processing for SID-style tunes
		if p.timerBVector != 0 {
			timerBCalls := calcTimerInterruptsPerFrame(
				p.bus.timerB.control, p.bus.timerB.data, p.frameRate)
			if timerBCalls > 100 {
				timerBCalls = 100 // Safety cap
			}
			for t := 0; t < timerBCalls; t++ {
				p.bus.ipra |= 0x01 // Set Timer B interrupt pending
				if err := p.callTimerHandler(p.timerBVector); err != nil {
					break
				}
			}
		}
		if p.timerDVector != 0 {
			timerDCalls := calcTimerInterruptsPerFrame(
				p.bus.timerD.control, p.bus.timerD.data, p.frameRate)
			if timerDCalls > 100 {
				timerDCalls = 100 // Safety cap
			}
			for t := 0; t < timerDCalls; t++ {
				p.bus.iprb |= 0x10 // Set Timer D interrupt pending
				if err := p.callTimerHandler(p.timerDVector); err != nil {
					break
				}
			}
		}

		// Collect events from this frame
		events = append(events, p.collectEvents(samplePos, startCycle)...)

		// Advance sample position
		acc += samplesPerFrameNum
		step := acc / samplesPerFrameDen
		samplePos += step
		acc -= step * samplesPerFrameDen
	}

	p.frameAcc = acc
	p.currentSample = samplePos
	return events, samplePos
}

// callPlay calls the SNDH PLAY routine
func (p *sndh68KPlayer) callPlay() error {
	// For SID emulation tunes that check timer control registers,
	// force timers to look "running" so envelope processing works.
	// The SID code checks TBCR/TCDCR (timer control) not IPRA/IPRB.
	if p.bus.timerB.data != 0 && p.bus.timerB.control == 0 {
		// Timer B has data but is stopped - enable it with prescaler /4
		p.bus.timerB.control = 1
	}
	if p.bus.timerD.data != 0 && p.bus.timerD.control == 0 {
		// Timer D has data but is stopped - enable it with prescaler /4
		p.bus.timerD.control = 1
	}

	// Set up stack
	p.cpu.AddrRegs[7] = SNDH_STACK_ADDR

	// Push return sentinel to stack
	p.cpu.AddrRegs[7] -= 4
	p.bus.Write32(p.cpu.AddrRegs[7], SNDH_RETURN_SENTINEL)

	// Set PC to PLAY address
	p.cpu.PC = uint32(p.file.PlayOffset)

	// Run until return
	return p.runUntilReturn()
}

// callTimer calls the timer interrupt handler (for timer-based replay)
func (p *sndh68KPlayer) callTimer() error {
	// Set up stack
	p.cpu.AddrRegs[7] = SNDH_STACK_ADDR

	// Push return sentinel to stack
	p.cpu.AddrRegs[7] -= 4
	p.bus.Write32(p.cpu.AddrRegs[7], SNDH_RETURN_SENTINEL)

	// Set PC to timer handler address
	p.cpu.PC = p.timerVector

	// Run until RTE or return sentinel
	return p.runUntilReturnOrRTE()
}

// callTimerHandler calls a specific timer interrupt handler
func (p *sndh68KPlayer) callTimerHandler(vector uint32) error {
	if vector == 0 || vector >= SNDH_BUS_SIZE {
		return nil
	}

	// Set up stack
	p.cpu.AddrRegs[7] = SNDH_STACK_ADDR

	// Push return sentinel to stack
	p.cpu.AddrRegs[7] -= 4
	p.bus.Write32(p.cpu.AddrRegs[7], SNDH_RETURN_SENTINEL)

	// Set PC to timer handler address
	p.cpu.PC = vector

	// Run until RTE or return sentinel
	return p.runUntilReturnOrRTE()
}

// debugSNDH enables verbose SNDH player debugging
var debugSNDH = false

// runUntilReturnOrRTE executes instructions until RTS/RTE returns to sentinel
func (p *sndh68KPlayer) runUntilReturnOrRTE() error {
	instructions := 0
	p.cpu.running.Store(true)
	startPC := p.cpu.PC

	if debugSNDH {
		println("runUntilReturnOrRTE: start PC=", p.cpu.PC, "SP=", p.cpu.AddrRegs[7])
	}

	for instructions < SNDH_MAX_INSTRUCTIONS_PER_FRAME {
		// Check if we've returned to sentinel
		if p.cpu.PC == SNDH_RETURN_SENTINEL {
			if debugSNDH {
				println("  -> returned to sentinel after", instructions, "instructions")
			}
			return nil
		}

		// Check for out-of-bounds PC
		if p.cpu.PC >= SNDH_BUS_SIZE-4 {
			return fmt.Errorf("PC out of bounds: 0x%08X", p.cpu.PC)
		}

		// Check for RTE instruction (timer handlers end with RTE)
		nextOp := uint16(p.bus.memory[p.cpu.PC])<<8 | uint16(p.bus.memory[p.cpu.PC+1])
		if nextOp == 0x4E73 { // RTE
			if debugSNDH {
				println("  -> hit RTE at PC=", p.cpu.PC, "after", instructions, "instructions")
			}
			return nil
		}

		// Execute one instruction
		p.cpu.currentIR = p.cpu.Fetch16()
		p.cpu.FetchAndDecodeInstruction()

		// Sync cycle counter to bus
		p.bus.AddCycles(int(p.cpu.cycleCounter))
		p.cpu.cycleCounter = 0

		instructions++
	}

	if debugSNDH {
		println("  -> exceeded max instructions, startPC=", startPC, "endPC=", p.cpu.PC)
	}
	return fmt.Errorf("exceeded max instructions per frame")
}

// runUntilReturn executes instructions until RTS returns to sentinel
func (p *sndh68KPlayer) runUntilReturn() error {
	instructions := 0
	p.cpu.running.Store(true)

	for instructions < SNDH_MAX_INSTRUCTIONS_PER_FRAME {
		// Check if we've returned to sentinel
		if p.cpu.PC == SNDH_RETURN_SENTINEL {
			return nil
		}

		// Check for out-of-bounds PC
		if p.cpu.PC >= SNDH_BUS_SIZE-4 {
			return fmt.Errorf("PC out of bounds: 0x%08X", p.cpu.PC)
		}

		// Execute one instruction
		p.cpu.currentIR = p.cpu.Fetch16()
		p.cpu.FetchAndDecodeInstruction()

		// Sync cycle counter to bus
		p.bus.AddCycles(int(p.cpu.cycleCounter))
		p.cpu.cycleCounter = 0

		instructions++
	}

	return fmt.Errorf("exceeded max instructions per frame")
}

// collectEvents converts YM2149 writes to PSGEvents
func (p *sndh68KPlayer) collectEvents(frameBaseSample uint64, startCycle uint64) []PSGEvent {
	writes := p.bus.GetWrites()
	if len(writes) == 0 {
		return nil
	}

	events := make([]PSGEvent, 0, len(writes))
	for _, write := range writes {
		// Calculate sample offset from cycle delta
		cycleDelta := write.Cycle - startCycle
		sampleOffset := (cycleDelta * uint64(p.sampleRate)) / uint64(p.clockHz)

		events = append(events, PSGEvent{
			Sample: frameBaseSample + sampleOffset,
			Reg:    write.Reg,
			Value:  write.Value,
		})
	}

	return events
}
