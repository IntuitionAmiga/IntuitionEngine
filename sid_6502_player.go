package main

import "fmt"

// SID6502Player executes PSID 6502 code and captures SID register writes.
type SID6502Player struct {
	bus           *SID6502Bus
	cpu           *CPU_6502
	file          *SIDFile
	subsong       int
	clockHz       uint32
	sampleRate    int
	cyclesPerTick int
	totalCycles   uint64
	totalSamples  uint64
	interruptMode bool
	initEvents    []SIDEvent
	initEmitted   bool
}

func newSID6502Player(file *SIDFile, subsong, sampleRate int) (*SID6502Player, error) {
	if file == nil {
		return nil, fmt.Errorf("SID file is nil")
	}
	if file.Header.IsRSID {
		return nil, fmt.Errorf("RSID is not supported")
	}
	if file.Header.Songs == 0 {
		return nil, fmt.Errorf("invalid SID header: songs=0")
	}

	if subsong <= 0 {
		subsong = int(file.Header.StartSong)
	}
	if subsong <= 0 || subsong > int(file.Header.Songs) {
		return nil, fmt.Errorf("invalid subsong %d", subsong)
	}

	ntsc := sidIsNTSC(file.Header)
	clockHz := uint32(SID_CLOCK_PAL)
	if ntsc {
		clockHz = uint32(SID_CLOCK_NTSC)
	}

	interruptMode := file.Header.PlayAddress == 0
	cyclesPerTick := sidCyclesPerTick(clockHz, ntsc, interruptMode, file.Header.Speed, subsong)

	bus := newSID6502Bus(ntsc)
	bus.LoadBinary(file.Header.LoadAddress, file.Data)

	player := &SID6502Player{
		bus:           bus,
		file:          file,
		subsong:       subsong,
		clockHz:       clockHz,
		sampleRate:    sampleRate,
		cyclesPerTick: cyclesPerTick,
		interruptMode: interruptMode,
	}

	player.cpu = player.createCPU()

	if file.Header.InitAddress != 0 {
		player.bus.StartFrame()
		if err := player.callRoutine(file.Header.InitAddress, uint8(subsong)); err != nil {
			return nil, fmt.Errorf("INIT routine failed: %v", err)
		}
		player.initEvents = player.bus.CollectEvents()
	}

	return player, nil
}

func sidIsNTSC(header SIDHeader) bool {
	video := header.Flags & 0x03
	return video == 0x02
}

func sidSpeedIsCIA(speed uint32, subsong int) bool {
	idx := subsong - 1
	if idx < 0 || idx >= 32 {
		return false
	}
	return (speed>>uint(idx))&1 != 0
}

func sidCyclesPerTick(clockHz uint32, ntsc bool, interruptMode bool, speed uint32, subsong int) int {
	tickHz := sidTickHz(clockHz, ntsc, interruptMode, speed, subsong)
	if tickHz == 0 {
		return 0
	}
	return int(clockHz / uint32(tickHz))
}

func sidTickHz(clockHz uint32, ntsc bool, interruptMode bool, speed uint32, subsong int) int {
	var tickHz uint32
	if interruptMode {
		tickHz = 60
	} else if sidSpeedIsCIA(speed, subsong) {
		tickHz = 60
	} else if ntsc {
		tickHz = 60
	} else {
		tickHz = 50
	}
	if tickHz == 0 {
		return 0
	}
	return int(tickHz)
}

func (p *SID6502Player) createCPU() *CPU_6502 {
	cpu := &CPU_6502{
		memory:        p.bus,
		SP:            0xFF,
		SR:            UNUSED_FLAG,
		Running:       true,
		rdyLine:       true,
		breakpoints:   make(map[uint16]bool),
		breakpointHit: make(chan uint16, 1),
	}
	return cpu
}

func (p *SID6502Player) callRoutine(addr uint16, aReg uint8) error {
	p.cpu.A = aReg
	p.cpu.X = 0
	p.cpu.Y = 0
	p.cpu.SR = UNUSED_FLAG
	p.cpu.Running = true

	stubAddr := uint16(0xFFF0)
	returnAddr := stubAddr + 3
	p.bus.Write(stubAddr, 0x20)
	p.bus.Write(stubAddr+1, byte(addr))
	p.bus.Write(stubAddr+2, byte(addr>>8))
	p.bus.Write(returnAddr, 0x4C)
	p.bus.Write(returnAddr+1, byte(returnAddr))
	p.bus.Write(returnAddr+2, byte(returnAddr>>8))

	p.cpu.PC = stubAddr

	maxCycles := uint64(1000000)
	startCycles := p.cpu.Cycles

	for p.cpu.Running && (p.cpu.Cycles-startCycles) < maxCycles {
		if p.cpu.PC == returnAddr {
			break
		}
		p.executeInstruction()
	}

	return nil
}

func (p *SID6502Player) executeInstruction() {
	cycles := p.cpu.Step()
	p.bus.AddCycles(cycles)
	if p.bus.irqPending {
		p.cpu.irqPending = true
		p.bus.irqPending = false
	}
}

func (p *SID6502Player) RenderFrames(numFrames int) ([]SIDEvent, uint64) {
	var allEvents []SIDEvent

	if !p.initEmitted && len(p.initEvents) > 0 {
		for i := range p.initEvents {
			eventCycle := p.totalCycles + p.initEvents[i].Cycle
			sample := p.cyclesToSamples(eventCycle)
			allEvents = append(allEvents, SIDEvent{
				Cycle:  eventCycle,
				Sample: sample,
				Reg:    p.initEvents[i].Reg,
				Value:  p.initEvents[i].Value,
			})
		}
		p.initEmitted = true
	}

	for frame := 0; frame < numFrames; frame++ {
		p.bus.StartFrame()

		if p.interruptMode {
			p.runForCycles(uint64(p.cyclesPerTick))
		} else {
			p.callRoutine(p.file.Header.PlayAddress, 0)
		}

		frameEvents := p.bus.CollectEvents()
		for i := range frameEvents {
			eventCycle := p.totalCycles + frameEvents[i].Cycle - p.bus.frameCycle
			sample := p.cyclesToSamples(eventCycle)
			allEvents = append(allEvents, SIDEvent{
				Cycle:  eventCycle,
				Sample: sample,
				Reg:    frameEvents[i].Reg,
				Value:  frameEvents[i].Value,
			})
		}

		p.totalCycles += uint64(p.cyclesPerTick)
		p.totalSamples += uint64(p.getSamplesPerTick())
	}

	return allEvents, p.totalSamples
}

func (p *SID6502Player) runForCycles(target uint64) {
	for p.bus.GetFrameCycles() < target && p.cpu.Running {
		p.executeInstruction()
	}
}

func (p *SID6502Player) cyclesToSamples(cycles uint64) uint64 {
	return cycles * uint64(p.sampleRate) / uint64(p.clockHz)
}

func (p *SID6502Player) getSamplesPerTick() int {
	return p.sampleRate * p.cyclesPerTick / int(p.clockHz)
}

func (p *SID6502Player) GetClockHz() uint32 {
	return p.clockHz
}

func (p *SID6502Player) GetTotalSamples() uint64 {
	return p.totalSamples
}

func (p *SID6502Player) GetTotalCycles() uint64 {
	return p.totalCycles
}

func (p *SID6502Player) Reset() {
	p.bus.Reset()
	p.totalCycles = 0
	p.totalSamples = 0
	p.initEmitted = false
	p.cpu = p.createCPU()
	if p.file.Header.InitAddress != 0 {
		p.bus.StartFrame()
		_ = p.callRoutine(p.file.Header.InitAddress, uint8(p.subsong))
		p.initEvents = p.bus.CollectEvents()
	}
}
